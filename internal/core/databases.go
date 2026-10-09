package core

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const MaxCatalogDatabases = 10000

const databaseListSQL = "SELECT JSON_OBJECT('name',SCHEMA_NAME) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY SCHEMA_NAME LIMIT 10001;"
const groupListSQL = "SELECT JSON_OBJECT('group_id',grupo_empresa_id,'group_name',nome) FROM sommusgestor.grupo_empresa WHERE excluido = 0 AND instancia_banco_dados_id = 1 ORDER BY grupo_empresa_id LIMIT 10001;"
const companyListSQL = "SELECT JSON_OBJECT('company_id',e.empresa_id,'group_id',e.grupo_empresa_id,'legal_name',e.razao_social,'trade_name',e.nome_fantasia) FROM sommusgestor.empresa e INNER JOIN sommusgestor.grupo_empresa g ON g.grupo_empresa_id = e.grupo_empresa_id WHERE e.excluido = 0 AND g.excluido = 0 AND g.instancia_banco_dados_id = 1 ORDER BY e.empresa_id LIMIT 10001;"

type DatabaseListRequest struct {
	ProfileID int64 `json:"profile_id"`
}

type DatabaseInfo struct {
	Name      string        `json:"name"`
	GroupID   int64         `json:"group_id,omitempty"`
	GroupName string        `json:"group_name,omitempty"`
	Companies []CompanyInfo `json:"companies,omitempty"`
}

type CompanyInfo struct {
	ID        int64  `json:"company_id"`
	GroupID   int64  `json:"group_id"`
	LegalName string `json:"legal_name"`
	TradeName string `json:"trade_name"`
}

type DatabaseCatalog struct {
	Databases []DatabaseInfo `json:"databases"`
	Warning   string         `json:"warning,omitempty"`
}

type GroupInfo struct {
	ID   int64  `json:"group_id"`
	Name string `json:"group_name"`
}

func BuildDatabaseList(p Profile, cfg Config, id string) (Command, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, err
	}
	cmd := mysqlCommand(p, cfg, id)
	cmd.StdoutData = true
	cmd.Args = append(cmd.Args, "--database=information_schema", "--connect-timeout=8", "--batch", "--raw", "--skip-column-names", "--default-character-set=utf8mb4", "--execute="+databaseListSQL)
	if p.SSL {
		cmd.Args = append(cmd.Args, "--ssl-mode=REQUIRED")
	}
	return cmd, nil
}

// Reuse the same connection, container name and pinned Docker identity.
func GroupListCommand(cmd Command) Command {
	return catalogLookupCommand(cmd, groupListSQL)
}

func CompanyListCommand(cmd Command) Command {
	return catalogLookupCommand(cmd, companyListSQL)
}

func catalogLookupCommand(cmd Command, query string) Command {
	cmd.Args = append([]string(nil), cmd.Args...)
	for i, arg := range cmd.Args {
		if strings.HasPrefix(arg, "--execute=") {
			cmd.Args[i] = "--execute=" + query
		}
	}
	return cmd
}

func catalogRow(line string, fields int) (map[string]json.RawMessage, error) {
	invalid := fmt.Errorf("resposta do catálogo de bancos inválida")
	if len(line) > MaxTableJSONBytes || !utf8.ValidString(line) {
		return nil, invalid
	}
	decoder := json.NewDecoder(strings.NewReader(line))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, invalid
	}
	row := make(map[string]json.RawMessage, fields)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || row[key] != nil {
			return nil, invalid
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || string(value) == "null" {
			return nil, invalid
		}
		row[key] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') || len(row) != fields {
		return nil, invalid
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, invalid
	}
	return row, nil
}

func ParseDatabaseInfo(line string) (DatabaseInfo, error) {
	row, err := catalogRow(line, 1)
	if err != nil {
		return DatabaseInfo{}, err
	}
	var info DatabaseInfo
	if err := json.Unmarshal(row["name"], &info.Name); err != nil || ValidateDatabase(info.Name) != nil {
		return DatabaseInfo{}, fmt.Errorf("resposta do catálogo de bancos inválida: nome")
	}
	return info, nil
}

func ParseGroupInfo(line string) (GroupInfo, error) {
	row, err := catalogRow(line, 2)
	if err != nil {
		return GroupInfo{}, err
	}
	var info GroupInfo
	if err := json.Unmarshal(row["group_id"], &info.ID); err != nil || info.ID <= 0 {
		return GroupInfo{}, fmt.Errorf("grupo com ID inválido")
	}
	if err := json.Unmarshal(row["group_name"], &info.Name); err != nil || strings.TrimSpace(info.Name) == "" || utf8.RuneCountInString(info.Name) > 100 {
		return GroupInfo{}, fmt.Errorf("grupo com nome inválido")
	}
	return info, nil
}

func ParseCompanyInfo(line string) (CompanyInfo, error) {
	row, err := catalogRow(line, 4)
	if err != nil {
		return CompanyInfo{}, err
	}
	var info CompanyInfo
	if err := json.Unmarshal(row["company_id"], &info.ID); err != nil || info.ID <= 0 {
		return CompanyInfo{}, fmt.Errorf("empresa com ID inválido")
	}
	if err := json.Unmarshal(row["group_id"], &info.GroupID); err != nil || info.GroupID <= 0 {
		return CompanyInfo{}, fmt.Errorf("empresa com grupo inválido")
	}
	if err := json.Unmarshal(row["legal_name"], &info.LegalName); err != nil || strings.TrimSpace(info.LegalName) == "" || utf8.RuneCountInString(info.LegalName) > 150 {
		return CompanyInfo{}, fmt.Errorf("empresa com razão social inválida")
	}
	if err := json.Unmarshal(row["trade_name"], &info.TradeName); err != nil || utf8.RuneCountInString(info.TradeName) > 150 {
		return CompanyInfo{}, fmt.Errorf("empresa com nome fantasia inválido")
	}
	return info, nil
}
