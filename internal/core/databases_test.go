package core

import (
	"fmt"
	"strings"
	"testing"
)

func TestDatabaseQueriesUseProfileConnectionAndFixedReadOnlySQL(t *testing.T) {
	p := testProfile()
	p.Database, p.SSL = "unavailable_default", true
	cmd, err := BuildDatabaseList(p, Config{}, "catalog")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, "\n")
	for _, flag := range []string{"--protocol=TCP", "--database=information_schema", "--default-character-set=utf8mb4", "--ssl-mode=REQUIRED", "--execute=" + databaseListSQL} {
		if !strings.Contains(args, flag) {
			t.Fatalf("missing %s", flag)
		}
	}
	if !cmd.StdoutData || strings.Contains(args, p.Database) {
		t.Fatal("catalog depends on default database or logs its rows")
	}
	cmd.DockerIdentity = "pinned"
	groups := GroupListCommand(cmd)
	companies := CompanyListCommand(cmd)
	if !strings.Contains(strings.Join(companies.Args, "\n"), "--execute="+companyListSQL) || companies.ContainerName != cmd.ContainerName || companies.DockerIdentity != "pinned" {
		t.Fatal("company lookup lost connection or Docker identity")
	}
	for _, clause := range []string{"sommusgestor.empresa", "e.excluido = 0", "g.excluido = 0", "g.instancia_banco_dados_id = 1", "g.grupo_empresa_id = e.grupo_empresa_id", "LIMIT 10001"} {
		if !strings.Contains(companyListSQL, clause) {
			t.Fatal("company scope changed")
		}
	}
	if !strings.Contains(strings.Join(groups.Args, "\n"), "--execute="+groupListSQL) || groups.ContainerName != cmd.ContainerName || groups.DockerIdentity != "pinned" {
		t.Fatal("group lookup lost connection or Docker identity")
	}
	if !strings.Contains(strings.Join(cmd.Args, "\n"), databaseListSQL) {
		t.Fatal("group lookup mutated primary query")
	}
	for _, clause := range []string{"sommusgestor.grupo_empresa", "excluido = 0", "instancia_banco_dados_id = 1", "LIMIT 10001"} {
		if !strings.Contains(groupListSQL, clause) {
			t.Fatal("group scope changed")
		}
	}
}

func TestCompanyRowsValidateNamesAndMembership(t *testing.T) {
	valid := `{"company_id":2,"group_id":12,"legal_name":"São José Ltda","trade_name":""}`
	if info, err := ParseCompanyInfo(valid); err != nil || info.ID != 2 || info.GroupID != 12 || info.LegalName != "São José Ltda" {
		t.Fatal(info, err)
	}
	for _, line := range []string{
		strings.Replace(valid, `"company_id":2`, `"company_id":0`, 1),
		strings.Replace(valid, `"group_id":12`, `"group_id":null`, 1),
		strings.Replace(valid, `"São José Ltda"`, `" "`, 1),
		strings.Replace(valid, `"trade_name":""`, `"trade_name":null`, 1),
		strings.Replace(valid, `"São José Ltda"`, fmt.Sprintf("%q", strings.Repeat("a", 151)), 1),
		valid + valid,
	} {
		if _, err := ParseCompanyInfo(line); err == nil {
			t.Fatal("invalid company accepted", line)
		}
	}
}

func TestDatabaseAndGroupRowsRejectMalformedResponses(t *testing.T) {
	if info, err := ParseDatabaseInfo(`{"name":"sommusgestor_12"}`); err != nil || info.Name != "sommusgestor_12" {
		t.Fatal(info, err)
	}
	if info, err := ParseGroupInfo(`{"group_id":12,"group_name":"Comércio São José"}`); err != nil || info.Name != "Comércio São José" {
		t.Fatal(info, err)
	}
	for _, line := range []string{`null`, `{}`, `{"name":null}`, `{"name":""}`, `{"name":"a","name":"b"}`, `{"name":"a"}{"name":"b"}`, `{"name":"a","extra":1}`, `{"name":"a\n"}`} {
		if _, err := ParseDatabaseInfo(line); err == nil {
			t.Fatalf("invalid database row accepted: %s", line)
		}
	}
	for _, line := range []string{`{}`, `{"group_id":0,"group_name":"Grupo"}`, `{"group_id":1,"group_name":""}`, `{"group_id":"1","group_name":"Grupo"}`, `{"group_id":1,"group_name":"Grupo","extra":true}`} {
		if _, err := ParseGroupInfo(line); err == nil {
			t.Fatalf("invalid group row accepted: %s", line)
		}
	}
}
