package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func ValidateProfile(p Profile) error {
	if strings.ContainsAny(p.Password, "\x00\r\n") {
		return fmt.Errorf("senha não pode conter NUL ou quebras de linha")
	}
	for name, value := range map[string]string{"Nome": p.Name, "Host": p.Host, "Usuário": p.User} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("%s é obrigatório e não pode conter quebras de linha", name)
		}
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("porta deve estar entre 1 e 65535")
	}
	if err := ValidateThreads(p.Threads); err != nil {
		return err
	}
	if p.Database != "" {
		if err := ValidateDatabase(p.Database); err != nil {
			return err
		}
	}
	return ValidateTablePresets(p.TablePresets)
}

func ValidateThreads(n int) error {
	if n < 1 || n > 256 {
		return fmt.Errorf("threads deve estar entre 1 e 256")
	}
	return nil
}

func ValidateDatabase(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || len([]rune(name)) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("database deve conter entre 1 e 64 caracteres sem controles")
	}
	return nil
}

func ValidateLocalTarget(p Profile, database string) error {
	switch strings.ToLower(strings.TrimSpace(p.Host)) {
	case "localhost", "127.0.0.1", "::1", "host.docker.internal":
	default:
		return fmt.Errorf("destino permitido somente em MySQL local (localhost, 127.0.0.1, ::1 ou host.docker.internal)")
	}
	if err := ValidateDatabase(database); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(database), strings.TrimSpace(p.Database)) {
		return fmt.Errorf("database destino deve ser diferente do database padrão do perfil")
	}
	return nil
}

// ExistingDirectory resolves links before constructing the Docker bind mount.
func ExistingDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("diretório é obrigatório")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("diretório inválido: %w", err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("diretório não existe: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("caminho deve ser um diretório existente")
	}
	if strings.ContainsAny(abs, "\x00\r\n") {
		return "", fmt.Errorf("diretório inválido")
	}
	return abs, nil
}

func baseCommand(p Profile, cfg Config, id, mount, image string) Command {
	args := []string{"run", "--rm", "--name", "dumpersg-" + id}
	if cfg.UseHostNetwork {
		args = append(args, "--network", "host")
	}
	if mount != "" {
		args = append(args, "-v", mount+":/backup")
	}
	args = append(args, image)
	return Command{Program: "docker", Args: args, Secrets: []string{p.Password}, ContainerName: "dumpersg-" + id}
}

func connectionArgs(p Profile) []string {
	return []string{"--host=" + strings.TrimSpace(p.Host), "--port=" + strconv.Itoa(p.Port), "--user=" + p.User, "--password=" + p.Password}
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func BuildBackup(p Profile, req BackupRequest, cfg Config, id string) (Command, string, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, "", err
	}
	if req.Database == "" {
		req.Database = p.Database
	}
	if err := ValidateDatabase(req.Database); err != nil {
		return Command{}, "", err
	}
	if req.Threads == 0 {
		req.Threads = p.Threads
	}
	if err := ValidateThreads(req.Threads); err != nil {
		return Command{}, "", err
	}
	if req.DestinationDir == "" {
		req.DestinationDir = cfg.BackupDir
	}
	dir, err := ExistingDirectory(req.DestinationDir)
	if err != nil {
		return Command{}, "", err
	}
	if len(req.IgnoreRegex) > 4096 || strings.ContainsAny(req.IgnoreRegex, "\x00\r\n") {
		return Command{}, "", fmt.Errorf("regex inválido ou maior que 4096 bytes")
	}
	filter, err := backupRegex(req.Database, req.Tables, req.IgnoreRegex)
	if err != nil {
		return Command{}, "", err
	}
	// Regex is passed to MyDumper's PCRE engine, including its negative lookahead.
	// Go regexp cannot validate PCRE and must not silently change its semantics.
	name := strings.Trim(unsafeFilename.ReplaceAllString(req.Database, "_"), "_.")
	if name == "" {
		name = "database"
	}
	name += "_" + time.Now().Format("20060102_150405") + "_" + id
	image := cfg.DockerImage
	if image == "" {
		image = "mydumper/mydumper:latest"
	}
	cmd := baseCommand(p, cfg, id, dir, image)
	cmd.Args = append(cmd.Args, "mydumper", "--protocol=tcp")
	cmd.Args = append(cmd.Args, connectionArgs(p)...)
	cmd.Args = append(cmd.Args, "--database="+req.Database, "--outputdir=/backup/"+name, "--threads="+strconv.Itoa(req.Threads), "--verbose=3")
	if req.Compress {
		cmd.Args = append(cmd.Args, "--compress")
	}
	if req.SSL {
		cmd.Args = append(cmd.Args, "--ssl")
	}
	if req.NonLocking == nil || *req.NonLocking {
		cmd.Args = append(cmd.Args, "--sync-thread-lock-mode=NO_LOCK", "--trx-tables", "--skip-ddl-locks", "--no-backup-locks")
	}
	if filter != "" {
		cmd.Args = append(cmd.Args, "--regex="+filter)
	}
	return cmd, filepath.Join(dir, name), nil
}

// Include the database-only candidate so MyDumper still exports schema-create.
// Every table is quoted literally; commas and dots are not list separators.
func backupRegex(database string, tables []string, ignore string) (string, error) {
	if tables == nil {
		if strings.TrimSpace(ignore) == "" {
			return "", nil
		}
		return "^(?!.*(" + ignore + ")).*", nil
	}
	if len(tables) == 0 {
		return "", fmt.Errorf("selecione ao menos uma tabela ou omita tables para incluir todas")
	}
	seen := make(map[string]struct{})
	quoted := make([]string, 0)
	for _, table := range tables {
		if err := ValidateTableName(table); err != nil {
			return "", err
		}
		if _, exists := seen[table]; exists {
			continue
		}
		seen[table] = struct{}{}
		quoted = append(quoted, regexp.QuoteMeta(table))
	}
	filter := "^"
	if strings.TrimSpace(ignore) != "" {
		filter += "(?!.*(" + ignore + "))"
	}
	filter += "(?:" + regexp.QuoteMeta(database) + "(?:\\z|\\.(?:" + strings.Join(quoted, "|") + ")\\z))"
	if len(filter) > 16384 {
		return "", fmt.Errorf("seleção de tabelas gera regex maior que 16 KiB; reduza a seleção")
	}
	return filter, nil
}

func BuildRestore(p Profile, req RestoreRequest, cfg Config, id string) (Command, string, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, "", err
	}
	if err := ValidateLocalTarget(p, req.TargetDatabase); err != nil {
		return Command{}, "", err
	}
	if req.Threads == 0 {
		req.Threads = p.Threads
	}
	if err := ValidateThreads(req.Threads); err != nil {
		return Command{}, "", err
	}
	dir, err := ExistingDirectory(req.BackupDir)
	if err != nil {
		return Command{}, "", err
	}
	meta, err := os.Stat(filepath.Join(dir, "metadata"))
	if err != nil || !meta.Mode().IsRegular() {
		return Command{}, "", fmt.Errorf("backup deve conter arquivo metadata")
	}
	filter, err := restoreRegex(dir, req.Tables)
	if err != nil {
		return Command{}, "", err
	}
	image := cfg.DockerImage
	if image == "" {
		image = "mydumper/mydumper:latest"
	}
	cmd := baseCommand(p, cfg, id, filepath.Dir(dir), image)
	cmd.Args = append(cmd.Args, "myloader", "--protocol=tcp")
	cmd.Args = append(cmd.Args, connectionArgs(p)...)
	cmd.Args = append(cmd.Args, "--database="+req.TargetDatabase, "--directory=/backup/"+filepath.Base(dir), "--threads="+strconv.Itoa(req.Threads), "--verbose=3")
	if p.SSL {
		cmd.Args = append(cmd.Args, "--ssl")
	}
	if req.OverwriteTables {
		cmd.Args = append(cmd.Args, "--drop-table=DROP")
	}
	if filter != "" {
		// Post-schema files contain database-wide routines and events.
		cmd.Args = append(cmd.Args, "--regex="+filter, "--skip-post")
	}
	return cmd, dir, nil
}

func restoreRegex(dir string, tables []TableReference) (string, error) {
	if tables == nil {
		return "", nil
	}
	if len(tables) == 0 || len(tables) > MaxCatalogTables {
		return "", fmt.Errorf("selecione entre 1 e 10000 tabelas ou omita tables para restaurar todas")
	}
	catalog, err := ListBackupTables(dir)
	if err != nil {
		return "", err
	}
	available := make(map[TableReference]BackupTableInfo, len(catalog))
	for _, table := range catalog {
		available[TableReference{Database: table.Database, Name: table.Name}] = table
	}
	seen := make(map[TableReference]bool)
	databases := make(map[string]bool)
	targetNames := make(map[string]string)
	parts := make([]string, 0, len(tables)+1)
	for _, table := range tables {
		if err := ValidateDatabase(table.Database); err != nil {
			return "", err
		}
		if err := ValidateTableName(table.Name); err != nil {
			return "", err
		}
		entry, exists := available[table]
		if !exists {
			return "", fmt.Errorf("tabela selecionada não pertence ao catálogo do backup: %s.%s", table.Database, table.Name)
		}
		if seen[table] {
			continue
		}
		if database, exists := targetNames[table.Name]; exists && database != table.Database {
			return "", fmt.Errorf("tabelas de bancos diferentes têm o mesmo nome no destino: %s", table.Name)
		}
		targetNames[table.Name] = table.Database
		seen[table] = true
		// MyLoader checks metadata with real names, but queues files with aliases.
		for _, database := range []string{table.Database, entry.fileDatabase} {
			if !databases[database] {
				databases[database] = true
				parts = append(parts, regexp.QuoteMeta(database))
			}
			for _, name := range []string{table.Name, entry.fileTable} {
				candidate := regexp.QuoteMeta(database) + `\.` + regexp.QuoteMeta(name)
				if !databases[candidate] {
					databases[candidate] = true
					parts = append(parts, candidate)
				}
			}
		}
	}
	filter := "^(?:" + strings.Join(parts, "|") + `)\z`
	if len(filter) > 16384 {
		return "", fmt.Errorf("seleção de tabelas gera regex maior que 16 KiB; reduza a seleção")
	}
	// Ambiguous aliases must never cause an unselected object to be restored.
	matcher, err := regexp.Compile(filter)
	if err != nil {
		return "", err
	}
	for reference, entry := range available {
		if seen[reference] {
			continue
		}
		for _, database := range []string{reference.Database, entry.fileDatabase} {
			for _, name := range []string{reference.Name, entry.fileTable} {
				if matcher.MatchString(database + "." + name) {
					return "", fmt.Errorf("aliases ambíguos entre tabelas selecionadas e não selecionadas: %s.%s", reference.Database, reference.Name)
				}
			}
		}
	}
	return filter, nil
}

func BuildTest(p Profile, cfg Config, id string) (Command, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, err
	}
	cmd := mysqlCommand(p, cfg, id)
	cmd.Args = append(cmd.Args, "--connect-timeout=8", "--execute=SELECT 1;")
	if p.SSL {
		cmd.Args = append(cmd.Args, "--ssl-mode=REQUIRED")
	}
	return cmd, nil
}

func BuildCreateDatabase(p Profile, database string, cfg Config, id string) (Command, error) {
	if err := ValidateProfile(p); err != nil {
		return Command{}, err
	}
	if err := ValidateLocalTarget(p, database); err != nil {
		return Command{}, err
	}
	cmd := mysqlCommand(p, cfg, id)
	cmd.Args = append(cmd.Args, "--execute=CREATE DATABASE IF NOT EXISTS `"+strings.ReplaceAll(database, "`", "``")+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;")
	if p.SSL {
		cmd.Args = append(cmd.Args, "--ssl-mode=REQUIRED")
	}
	return cmd, nil
}

func mysqlCommand(p Profile, cfg Config, id string) Command {
	image := cfg.MySQLImage
	if image == "" {
		image = "mysql:8.4"
	}
	cmd := baseCommand(p, cfg, id, "", image)
	cmd.Args = append(cmd.Args, "mysql")
	// A localhost profile addresses the daemon's TCP listener, not a Unix socket
	// inside the disposable client container.
	cmd.Args = append(cmd.Args, "--protocol=TCP")
	cmd.Args = append(cmd.Args, connectionArgs(p)...)
	return cmd
}
