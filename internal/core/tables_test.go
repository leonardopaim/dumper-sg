package core

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestTableListSQLAndConnectionFlags(t *testing.T) {
	database := "db'\\名"
	cmd, err := BuildTableList(testProfile(), TableListRequest{Database: database, SSL: true}, Config{UseHostNetwork: true}, "tables")
	if err != nil {
		t.Fatal(err)
	}
	if !cmd.StdoutData || cmd.ContainerName != "dumpersg-tables" || cmd.Secrets[0] != "secret" {
		t.Fatal("missing data channel or execution metadata")
	}
	args := strings.Join(cmd.Args, "\n")
	for _, flag := range []string{"--protocol=TCP", "--database=" + database, "--connect-timeout=8", "--batch", "--raw", "--skip-column-names", "--default-character-set=utf8mb4", "--ssl-mode=REQUIRED", "--network\nhost"} {
		if !strings.Contains(args, flag) {
			t.Fatalf("missing %s", flag)
		}
	}
	sql := ""
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "--execute=") {
			sql = strings.TrimPrefix(arg, "--execute=")
		}
	}
	if !strings.Contains(sql, "CONVERT(0x"+hex.EncodeToString([]byte(database))+" USING utf8mb4)") || strings.Contains(sql, database) || !strings.Contains(sql, "LIMIT 10001") || !strings.Contains(sql, "'BASE TABLE','VIEW'") {
		t.Fatalf("unsafe/incomplete query: %s", sql)
	}
	p := testProfile()
	p.SSL = true
	cmd, err = BuildTableList(p, TableListRequest{}, Config{}, "default")
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Join(cmd.Args, "\n")
	if !strings.Contains(args, "--database=production") || strings.Contains(args, "--ssl-mode") {
		t.Fatal("request database/SSL defaults changed")
	}
	p.Database = ""
	if _, err = BuildTableList(p, TableListRequest{}, Config{}, "missing"); err == nil {
		t.Fatal("missing database allowed")
	}
}

func TestTableJSONStrictValidation(t *testing.T) {
	valid := `{"name":"café名,.'\"","size_bytes":18446744073709551615,"rows":0,"table_type":"VIEW"}`
	table, err := ParseTableInfo(valid)
	if err != nil || table.SizeBytes != ^uint64(0) || table.TableType != "VIEW" {
		t.Fatalf("valid row rejected: %+v %v", table, err)
	}
	for _, line := range []string{
		"not JSON", `[]`, `null`, `{}`, valid + valid,
		`{"name":"t","size_bytes":1,"rows":0}`,
		`{"name":"t","size_bytes":null,"rows":0,"table_type":"BASE TABLE"}`,
		`{"name":"t","size_bytes":-1,"rows":0,"table_type":"BASE TABLE"}`,
		`{"name":"t","size_bytes":18446744073709551616,"rows":0,"table_type":"BASE TABLE"}`,
		`{"name":"t","size_bytes":1,"rows":1.5,"table_type":"BASE TABLE"}`,
		`{"name":"t","size_bytes":"1","rows":0,"table_type":"BASE TABLE"}`,
		`{"name":"t","size_bytes":1,"rows":0,"table_type":"SYSTEM VIEW"}`,
		`{"name":"t\n","size_bytes":1,"rows":0,"table_type":"VIEW"}`,
		`{"name":"t","name":"u","size_bytes":1,"rows":0,"table_type":"VIEW"}`,
		`{"Name":"t","size_bytes":1,"rows":0,"table_type":"VIEW"}`,
		strings.Repeat(" ", MaxTableJSONBytes) + valid,
	} {
		if _, err := ParseTableInfo(line); err == nil {
			t.Fatalf("malformed row accepted: %s", line)
		}
	}
}
