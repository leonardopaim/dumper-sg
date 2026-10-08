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
		return ValidateDatabase(p.Database)
	}
	return nil
}

func ValidateThreads(n int) error {
	if n < 1 || n > 256 {
		return fmt.Errorf("threads deve estar entre 1 e 256")
	}
	return nil
}

func ValidateDatabase(name string) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
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
	if strings.TrimSpace(req.IgnoreRegex) != "" {
		cmd.Args = append(cmd.Args, "--regex=^(?!.*("+req.IgnoreRegex+")).*")
	}
	return cmd, filepath.Join(dir, name), nil
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
	return cmd, dir, nil
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
