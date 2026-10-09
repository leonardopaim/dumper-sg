package core

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func writeCatalogFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func catalogMetadata(groups string) string {
	return "# Started dump at: 2026-10-08\n[config]\nquote-character=BACKTICK\n" + groups + "\n# Finished dump at: 2026-10-08\n"
}

func selectedBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", catalogMetadata("[`source`.`sample`]\nreal_table_name=sample\nrows=2\n[`source`.`mydumper_0`]\nreal_table_name=literal,+.[x]\nrows=1\n[`source`]\nschema_checksum=a\n"))
	writeCatalogFile(t, dir, "source-schema-create.sql.zst", "db")
	writeCatalogFile(t, dir, "source.sample-schema.sql.zst", "schema")
	writeCatalogFile(t, dir, "source.sample.00000.sql.zst", "two rows")
	writeCatalogFile(t, dir, "source.mydumper_0-schema.sql.zst", "special schema")
	writeCatalogFile(t, dir, "source.mydumper_0.00000.00001.sql.zst", "one row")
	return dir
}

func TestBackupCatalogAliasesCompressedSizesAndRows(t *testing.T) {
	dir := selectedBackup(t)
	tables, err := ListBackupTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 || tables[0].Name != "literal,+.[x]" || tables[0].Database != "source" || tables[0].Rows != 1 || tables[0].SizeBytes != 21 || tables[1].Name != "sample" || tables[1].Rows != 2 || tables[1].SizeBytes != 14 {
		t.Fatalf("incorrect catalog: %+v", tables)
	}
}

func TestBackupCatalogQuotedIdentifiersAndViews(t *testing.T) {
	dir := t.TempDir()
	ids, err := quotedBackupIdentifiers(`"db"."view""name"`)
	if err != nil || len(ids) != 2 || ids[1] != `view"name` {
		t.Fatalf("ANSI identifier escaping: %+v %v", ids, err)
	}
	writeCatalogFile(t, dir, "metadata", catalogMetadata("[`db.with``quote`.`table.with.dot`]\nreal_table_name= table.with.dot \nrows=0\n[\"db.with`quote\".\"mydumper_1\"]\nreal_table_name=view\"name\nrows=0\nis_view=1\n"))
	writeCatalogFile(t, dir, "db.with`quote.table.with.dot-schema.sql", "123")
	writeCatalogFile(t, dir, "db.with`quote.mydumper_1-schema-view.sql", "12345")
	tables, err := ListBackupTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 || tables[0].TableType != "VIEW" || tables[0].Name != "view\"name" || tables[1].Name != " table.with.dot " || tables[1].Database != "db.with`quote" {
		t.Fatalf("quoted mapping lost: %+v", tables)
	}
}

func TestBackupCatalogFailuresNeverReturnPartial(t *testing.T) {
	valid := "[`db`.`t`]\nreal_table_name=t\nrows=1\n"
	for _, tc := range []struct {
		name, metadata string
		schema         bool
	}{
		{"truncated", "[config]\n" + valid, true},
		{"no catalog", catalogMetadata("[config]\nx=y\n"), true},
		{"bad section", catalogMetadata("[`db`.`t]\nreal_table_name=t\nrows=1\n"), true},
		{"no rows", catalogMetadata("[`db`.`t`]\nreal_table_name=t\n"), true},
		{"negative rows", catalogMetadata(strings.ReplaceAll(valid, "rows=1", "rows=-1")), true},
		{"duplicate table", catalogMetadata(valid + valid), true},
		{"missing schema", catalogMetadata(valid), false},
		{"oversized", strings.Repeat("x", maxBackupMetadataBytes+1), true},
		{"invalid utf8", catalogMetadata(strings.ReplaceAll(valid, "name=t", "name=\xff")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCatalogFile(t, dir, "metadata", tc.metadata)
			if tc.schema {
				writeCatalogFile(t, dir, "db.t-schema.sql", "schema")
			}
			tables, err := ListBackupTables(dir)
			if err == nil || tables != nil {
				t.Fatalf("partial/silent success: %+v %v", tables, err)
			}
		})
	}
	dir := selectedBackup(t)
	writeCatalogFile(t, dir, "source.unknown-schema.sql", "schema")
	if tables, err := ListBackupTables(dir); err == nil || tables != nil {
		t.Fatal("unmapped file silently omitted")
	}
}

func TestBackupCatalogMetadataAndTableLinksRejected(t *testing.T) {
	for _, name := range []string{"metadata", "source.sample.00000.sql.zst"} {
		t.Run(name, func(t *testing.T) {
			dir := selectedBackup(t)
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, []byte("outside"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if tables, err := ListBackupTables(dir); err == nil || tables != nil {
				t.Fatal("symlink followed")
			}
		})
	}
}

func TestBackupCatalogLegacyGzipMappingAndInvalidZstd(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", "Started dump at: date\nFinished dump at: date\n")
	writeCatalogFile(t, dir, "mydumper_1-schema-create.sql", "CREATE DATABASE IF NOT EXISTS `db.with.dot`;")
	f, err := os.Create(filepath.Join(dir, "mydumper_1.mydumper_2-schema.sql.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	_, err = gz.Write([]byte("CREATE TABLE `table``with.dot` (id INT);"))
	if err != nil {
		t.Fatal(err)
	}
	if err = gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	tables, err := ListBackupTables(dir)
	if err != nil || len(tables) != 1 || tables[0].Database != "db.with.dot" || tables[0].Name != "table`with.dot" {
		t.Fatalf("legacy mapping: %+v %v", tables, err)
	}
	dir = t.TempDir()
	writeCatalogFile(t, dir, "metadata", "Started dump at: date\nFinished dump at: date\n")
	writeCatalogFile(t, dir, "mydumper_1-schema-create.sql", "CREATE DATABASE `db.with.dot`;")
	writeCatalogFile(t, dir, "mydumper_1.mydumper_2-schema.sql.zst", "unsupported")
	if tables, err := ListBackupTables(dir); err == nil || tables != nil || !strings.Contains(err.Error(), "zstd") {
		t.Fatalf("unsupported mapping did not fail explicitly: %v", err)
	}
}

func TestBackupCatalogLimitAndNumericTableName(t *testing.T) {
	var metadata strings.Builder
	for i := 0; i <= MaxCatalogTables; i++ {
		fmt.Fprintf(&metadata, "[`db`.`t%d`]\nreal_table_name=t%d\nrows=0\n", i, i)
	}
	if _, _, err := parseBackupMetadata(catalogMetadata(metadata.String())); err == nil {
		t.Fatal("table limit ignored")
	}
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", catalogMetadata("[`db`.`12345`]\nreal_table_name=12345\nrows=1\n"))
	writeCatalogFile(t, dir, "db.12345-schema.sql", "schema")
	writeCatalogFile(t, dir, "db.12345.00000.sql", "row")
	if tables, err := ListBackupTables(dir); err != nil || len(tables) != 1 || tables[0].SizeBytes != 9 {
		t.Fatalf("numeric table chunk mapping: %+v %v", tables, err)
	}
}

func TestRestoreSelectionUsesSourceLiteralAndSkipsGlobalObjects(t *testing.T) {
	dir := selectedBackup(t)
	req := RestoreRequest{BackupDir: dir, TargetDatabase: "isolated", OverwriteTables: true, Tables: []TableReference{{Database: "source", Name: "literal,+.[x]"}}}
	cmd, _, err := BuildRestore(testProfile(), req, Config{}, "selected")
	if err != nil {
		t.Fatal(err)
	}
	var filter string
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "--regex=") {
			filter = strings.TrimPrefix(arg, "--regex=")
		}
	}
	regex, err := regexp.Compile(filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source", "source.literal,+.[x]", "source.mydumper_0"} {
		if !regex.MatchString(name) {
			t.Fatalf("required source candidate omitted: %s", name)
		}
	}
	for _, name := range []string{"isolated.literal,+.[x]", "source.sample", "source.literal,+.[x]suffix", "other.literal,+.[x]", "source.literal,+.[x]\n"} {
		if regex.MatchString(name) {
			t.Fatalf("unselected candidate matched: %s", name)
		}
	}
	args := strings.Join(cmd.Args, "\n")
	for _, flag := range []string{"--database=isolated", "--skip-post", "--drop-table=DROP"} {
		if !strings.Contains(args, flag) {
			t.Fatalf("missing %s", flag)
		}
	}
	if strings.Contains(args, "--drop-database") {
		t.Fatal("destructive global option or filename alias filter")
	}
	for _, tables := range [][]TableReference{{}, {{Database: "source", Name: "fake"}}, {{Database: "source", Name: "sample\n"}}} {
		req.Tables = tables
		if _, _, err := BuildRestore(testProfile(), req, Config{}, "invalid"); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	req.Tables = nil
	writeCatalogFile(t, dir, "metadata", "old opaque metadata")
	cmd, _, err = BuildRestore(testProfile(), req, Config{}, "all")
	if err != nil || strings.Contains(strings.Join(cmd.Args, "\n"), "--regex=") || strings.Contains(strings.Join(cmd.Args, "\n"), "--skip-post") {
		t.Fatal("all-tables backwards behavior changed")
	}
	req.TargetDatabase = "production"
	if _, _, err := BuildRestore(testProfile(), req, Config{}, "unsafe"); err == nil {
		t.Fatal("default target accepted")
	}
}

func TestRestoreSelectionRejectsAmbiguousDestinationAndLargeFilter(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", catalogMetadata("[`first`.`same`]\nreal_table_name=same\nrows=0\n[`second`.`same`]\nreal_table_name=same\nrows=0\n"))
	writeCatalogFile(t, dir, "first.same-schema.sql", "schema")
	writeCatalogFile(t, dir, "second.same-schema.sql", "schema")
	if _, err := restoreRegex(dir, []TableReference{{Database: "first", Name: "same"}, {Database: "second", Name: "same"}}); err == nil {
		t.Fatal("two source tables overwrite the same destination")
	}
	var groups strings.Builder
	selection := make([]TableReference, 0, 300)
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("t%03d%s", i, strings.Repeat("名", 20))
		alias := fmt.Sprintf("mydumper_%d", i)
		fmt.Fprintf(&groups, "[`db`.`%s`]\nreal_table_name=%s\nrows=0\n", alias, name)
		selection = append(selection, TableReference{Database: "db", Name: name})
	}
	// Use a new backup so orphan files cannot mask the regex size validation.
	dir2 := t.TempDir()
	writeCatalogFile(t, dir2, "metadata", catalogMetadata(groups.String()))
	for i := 0; i < 300; i++ {
		writeCatalogFile(t, dir2, fmt.Sprintf("db.mydumper_%d-schema.sql", i), "schema")
	}
	if _, err := restoreRegex(dir2, selection); err == nil || !strings.Contains(err.Error(), "16 KiB") {
		t.Fatalf("large filter allowed: %v", err)
	}
}

func TestBackupCatalogZstdDatabaseAliasesAndDecodedLimit(t *testing.T) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	writeZstd := func(dir, name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), encoder.EncodeAll([]byte(data), nil), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_%v", legacy), func(t *testing.T) {
			dir := t.TempDir()
			metadata := catalogMetadata("[`mydumper_1`.`mydumper_2`]\nreal_table_name=table`with.dot\nrows=2\n[`mydumper_1`]\nschema_checksum=a\n")
			if legacy {
				metadata = "Started dump at: date\nFinished dump at: date\n"
			}
			writeCatalogFile(t, dir, "metadata", metadata)
			writeZstd(dir, "mydumper_1-schema-create.sql.zst", "CREATE DATABASE /*!32312 IF NOT EXISTS*/ `db.with.dot` /*!40100 DEFAULT CHARACTER SET utf8mb4 */;")
			writeZstd(dir, "mydumper_1.mydumper_2-schema.sql.zst", "CREATE TABLE `table``with.dot` (id INT);")
			tables, err := ListBackupTables(dir)
			if err != nil || len(tables) != 1 || tables[0].Database != "db.with.dot" || tables[0].Name != "table`with.dot" {
				t.Fatalf("zstd mapping: %+v %v", tables, err)
			}
		})
	}
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", "Started dump at: date\nFinished dump at: date\n")
	writeZstd(dir, "mydumper_1-schema-create.sql.zst", "CREATE DATABASE `db.with.dot`;"+strings.Repeat("x", maxSchemaBytes))
	if tables, err := ListBackupTables(dir); err == nil || tables != nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("decoded limit ignored: %+v %v", tables, err)
	}
}

func TestRestoreSelectionRejectsFilenameAliasCollision(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "metadata", catalogMetadata("[`db`.`mydumper_0`]\nreal_table_name=literal,+.[x]\nrows=1\n[`db`.`mydumper_1`]\nreal_table_name=mydumper_0\nrows=1\n"))
	writeCatalogFile(t, dir, "db.mydumper_0-schema.sql", "schema")
	writeCatalogFile(t, dir, "db.mydumper_1-schema.sql", "schema")
	if _, err := restoreRegex(dir, []TableReference{{Database: "db", Name: "literal,+.[x]"}}); err == nil || !strings.Contains(err.Error(), "ambíguos") {
		t.Fatalf("alias would restore an unselected object: %v", err)
	}
}
