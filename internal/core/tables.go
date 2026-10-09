package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxCatalogTables = 10000
const MaxTableJSONBytes = 4096

func ValidateTableName(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || len([]rune(name)) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("nome de tabela deve conter entre 1 e 64 caracteres sem controles")
	}
	return nil
}

func BuildTableList(p Profile, req TableListRequest, cfg Config, id string) (Command, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, err
	}
	if req.Database == "" {
		req.Database = p.Database
	}
	if err := ValidateDatabase(req.Database); err != nil {
		return Command{}, err
	}
	if !utf8.ValidString(req.Database) {
		return Command{}, fmt.Errorf("database deve conter UTF-8 válido")
	}
	// A hex literal avoids quote/backslash and SQL-mode-dependent escaping.
	database := "CONVERT(0x" + hex.EncodeToString([]byte(req.Database)) + " USING utf8mb4)"
	sql := "SELECT JSON_OBJECT('name',TABLE_NAME,'size_bytes',COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0),'rows',COALESCE(TABLE_ROWS,0),'table_type',TABLE_TYPE) FROM information_schema.TABLES WHERE TABLE_SCHEMA = " + database + " AND TABLE_TYPE IN ('BASE TABLE','VIEW') ORDER BY COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0) DESC,TABLE_NAME LIMIT 10001;"
	cmd := mysqlCommand(p, cfg, id)
	cmd.StdoutData = true
	cmd.Args = append(cmd.Args, "--database="+req.Database, "--connect-timeout=8", "--batch", "--raw", "--skip-column-names", "--default-character-set=utf8mb4", "--execute="+sql)
	if req.SSL {
		cmd.Args = append(cmd.Args, "--ssl-mode=REQUIRED")
	}
	return cmd, nil
}

func ParseTableInfo(line string) (TableInfo, error) {
	invalid := fmt.Errorf("resposta de tabelas inválida: esperado objeto JSON com nome, tamanho, linhas e tipo válidos")
	if len(line) > MaxTableJSONBytes || !utf8.ValidString(line) {
		return TableInfo{}, invalid
	}
	decoder := json.NewDecoder(strings.NewReader(line))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return TableInfo{}, invalid
	}
	var row TableInfo
	fields := make(map[string]bool, 4)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return TableInfo{}, invalid
		}
		name, ok := key.(string)
		if !ok || fields[name] {
			return TableInfo{}, invalid
		}
		fields[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || strings.TrimSpace(string(value)) == "null" {
			return TableInfo{}, invalid
		}
		var target any
		switch name {
		case "name":
			target = &row.Name
		case "size_bytes":
			target = &row.SizeBytes
		case "rows":
			target = &row.Rows
		case "table_type":
			target = &row.TableType
		default:
			return TableInfo{}, invalid
		}
		if err := json.Unmarshal(value, target); err != nil {
			return TableInfo{}, invalid
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return TableInfo{}, invalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return TableInfo{}, invalid
	}
	if len(fields) != 4 {
		return TableInfo{}, invalid
	}
	if err := ValidateTableName(row.Name); err != nil {
		return TableInfo{}, invalid
	}
	if row.TableType != "BASE TABLE" && row.TableType != "VIEW" {
		return TableInfo{}, invalid
	}
	return row, nil
}

func SortTables(tables []TableInfo) {
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].SizeBytes != tables[j].SizeBytes {
			return tables[i].SizeBytes > tables[j].SizeBytes
		}
		return tables[i].Name < tables[j].Name
	})
}
