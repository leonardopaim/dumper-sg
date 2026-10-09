package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func testProfile() Profile {
	return Profile{ID: 1, Name: "local", Host: "localhost", Port: 3306, User: "root", Password: "secret", Database: "production", Threads: 8}
}

func TestLocalTargets(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "host.docker.internal", " LOCALHOST "} {
		p := testProfile()
		p.Host = host
		if err := ValidateLocalTarget(p, "production_restore"); err != nil {
			t.Fatal(err)
		}
		if err := ValidateLocalTarget(p, " PRODUCTION "); err == nil {
			t.Fatal("default database allowed")
		}
	}
	for _, host := range []string{"remote.example", "localhost.example", "127.0.0.2", "[::1]", "localhost."} {
		p := testProfile()
		p.Host = host
		if err := ValidateLocalTarget(p, "isolated"); err == nil {
			t.Fatalf("remote host allowed: %s", host)
		}
	}
}

func TestMultilineSecretsRejectedAndMySQLSSLMode(t *testing.T) {
	for _, secret := range []string{"first\nsecond", "first\rsecond", "first\x00second"} {
		p := testProfile()
		p.Password = secret
		if _, err := BuildTest(p, Config{}, "abc"); err == nil {
			t.Fatal("multiline secret allowed to leak fragments")
		}
	}
	p := testProfile()
	p.SSL = true
	for _, build := range []func() (Command, error){func() (Command, error) { return BuildTest(p, Config{}, "abc") }, func() (Command, error) { return BuildCreateDatabase(p, "isolated", Config{}, "abc") }} {
		cmd, err := build()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(cmd.Args, " "), "--ssl-mode=REQUIRED") {
			t.Fatal("MySQL 8.4 encryption flag missing")
		}
	}
}

func TestBackupPreservesLegacyFlagsAndPCRE(t *testing.T) {
	dir := t.TempDir()
	req := BackupRequest{DestinationDir: dir, Database: "db with spaces", Compress: true, SSL: true, IgnoreRegex: `audit(?=_)`}
	cmd, path, err := BuildBackup(testProfile(), req, Config{UseHostNetwork: true}, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, "\n")
	for _, expected := range []string{"--network\nhost", "mydumper/mydumper:latest\nmydumper", "--threads=8", "--compress", "--ssl", "--sync-thread-lock-mode=NO_LOCK", "--trx-tables", "--skip-ddl-locks", "--no-backup-locks", "--regex=^(?!.*(audit(?=_))).*"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("missing argument %s", expected)
		}
	}
	if filepath.Dir(path) != dir || strings.Contains(filepath.Base(path), " ") {
		t.Fatalf("invalid output path: %s", path)
	}
	if cmd.ContainerName != "dumpersg-abcd" || cmd.Secrets[0] != "secret" {
		t.Fatal("missing execution identity or secret")
	}
	no := false
	req.NonLocking = &no
	cmd, _, err = BuildBackup(testProfile(), req, Config{}, "next")
	if err != nil || strings.Contains(strings.Join(cmd.Args, " "), "--no-backup-locks") {
		t.Fatal("explicit locking option ignored")
	}
}

func TestRestoreRequiresMetadataAndEscapesSQL(t *testing.T) {
	dir := t.TempDir()
	req := RestoreRequest{BackupDir: dir, TargetDatabase: "isolated", OverwriteTables: true}
	if _, _, err := BuildRestore(testProfile(), req, Config{}, "abc"); err == nil {
		t.Fatal("missing metadata allowed")
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, canonical, err := BuildRestore(testProfile(), req, Config{}, "abc")
	if err != nil || canonical != dir {
		t.Fatalf("restore failed: %v", err)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), "--drop-table=DROP") || strings.Contains(strings.Join(cmd.Args, " "), "--overwrite-tables") {
		t.Fatal("overwrite flag missing")
	}
	req.OverwriteTables = false
	withoutDrop, _, err := BuildRestore(testProfile(), req, Config{}, "plain")
	if err != nil || strings.Contains(strings.Join(withoutDrop.Args, " "), "--drop-table") {
		t.Fatal("table removal enabled without explicit overwrite")
	}
	cmd, err = BuildCreateDatabase(testProfile(), "test`; DROP DATABASE production; --", Config{}, "abc")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "--execute=CREATE DATABASE IF NOT EXISTS `test``; DROP DATABASE production; --` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" {
			found = true
		}
	}
	if !found {
		t.Fatal("identifier was not safely quoted as a single argument")
	}
}

func TestRestoreHonorsProfileSSL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	p := testProfile()
	p.SSL = true
	cmd, _, err := BuildRestore(p, RestoreRequest{BackupDir: dir, TargetDatabase: "isolated"}, Config{}, "ssl")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "--ssl" {
			found = true
		}
	}
	if !found {
		t.Fatal("restore ignored profile encryption")
	}
}

func TestMySQLLocalhostForcesTCP(t *testing.T) {
	cmd, err := BuildTest(testProfile(), Config{}, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cmd.Args, "\n"), "\n--protocol=TCP\n") {
		t.Fatal("localhost can fall back to Unix socket")
	}
}

func TestBackupSelectionMatchesOnlyLiteralQualifiedNamesAndSchema(t *testing.T) {
	database := "db.main+$"
	tables := []string{"plain", "comma,name", "dot.name", "quote`'\"", "a|b[]()+?*", "café名"}
	filter, err := backupRegex(database, tables, "")
	if err != nil {
		t.Fatal(err)
	}
	pattern, err := regexp.Compile(filter)
	if err != nil {
		t.Fatal(err)
	}
	if !pattern.MatchString(database) {
		t.Fatal("database schema-create was excluded")
	}
	for _, table := range tables {
		if !pattern.MatchString(database + "." + table) {
			t.Fatalf("literal table excluded: %q", table)
		}
		for _, candidate := range []string{database + "." + table + "_copy", database + "." + table + "\n", "another." + table, "prefix" + database + "." + table} {
			if pattern.MatchString(candidate) {
				t.Fatalf("unselected candidate matched: %q", candidate)
			}
		}
	}
	for _, candidate := range []string{database + "x", database + ".absent", database + "\n", "dbXmain"} {
		if pattern.MatchString(candidate) {
			t.Fatalf("unselected candidate matched: %q", candidate)
		}
	}
}

func TestBackupSelectionCombinesIgnoreOnceAndRejectsEmpty(t *testing.T) {
	base := BackupRequest{Database: "production", DestinationDir: t.TempDir(), Tables: []string{"orders", "orders", "audit"}, IgnoreRegex: "audit"}
	cmd, _, err := BuildBackup(testProfile(), base, Config{}, "selected")
	if err != nil {
		t.Fatal(err)
	}
	filters := []string{}
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "--regex=") {
			filters = append(filters, arg)
		}
	}
	if len(filters) != 1 || strings.Count(filters[0], "orders") != 1 || !strings.Contains(filters[0], "(?!.*(audit))") {
		t.Fatalf("selection not cumulative/deduplicated: %v", filters)
	}
	base.Tables = []string{}
	if _, _, err := BuildBackup(testProfile(), base, Config{}, "empty"); err == nil {
		t.Fatal("empty selection became full backup")
	}
	base.Tables = nil
	cmd, _, err = BuildBackup(testProfile(), base, Config{}, "all")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "--regex=^(?!.*(audit)).*" {
			found = true
		}
	}
	if !found {
		t.Fatal("all-table legacy ignore changed")
	}
}

func TestBackupSelectionValidatesNamesAndRegexBudget(t *testing.T) {
	for _, table := range []string{"", " ", "line\nbreak", "null\x00", strings.Repeat("é", 65)} {
		if _, err := backupRegex("db", []string{table}, ""); err == nil {
			t.Fatalf("invalid table accepted %q", table)
		}
	}
	tables := make([]string, 300)
	for i := range tables {
		tables[i] = fmt.Sprintf("%04d%s", i, strings.Repeat("x", 60))
	}
	if _, err := backupRegex("db", tables, ""); err == nil {
		t.Fatal("oversized regex accepted")
	}
}

func TestProductionBackupThreadLimit(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, host                string
		profileThreads, requested int
		blocked                   bool
	}{
		{"two", "db.sommusgestor.com", 8, 2, false},
		{"one", "db.sommusgestor.com", 8, 1, false},
		{"explicit excess", "db.sommusgestor.com", 2, 3, true},
		{"inherited excess", "db.sommusgestor.com", 8, 0, true},
		{"safe inherited", "db.sommusgestor.com", 2, 0, false},
		{"normalized host", " DB.SOMMUSGESTOR.COM. ", 8, 3, true},
		{"other host", "another.example", 8, 8, false},
		{"lookalike", "db.sommusgestor.com.other", 8, 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProfile()
			p.Host, p.Threads = tc.host, tc.profileThreads
			cmd, _, err := BuildBackup(p, BackupRequest{Database: "production", DestinationDir: dir, Threads: tc.requested}, Config{}, "safe")
			if tc.blocked {
				if err == nil || !strings.Contains(err.Error(), "Cada thread aumenta") || len(cmd.Args) != 0 {
					t.Fatalf("unsafe backup allowed or missing alert: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
