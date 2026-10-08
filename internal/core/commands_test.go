package core

import (
	"os"
	"path/filepath"
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
