package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"dumpersg/internal/core"
)

type ImportResult struct {
	Profiles int `json:"profiles"`
	Jobs     int `json:"jobs"`
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve aliases when possible, making repeated imports through a symlink idempotent.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

// ImportLegacy reads a consistent snapshot of the source without changing its schema or pragmas.
// All destination changes commit together; a malformed legacy table rolls back the entire import.
func (s *Store) ImportLegacy(ctx context.Context, path string) (ImportResult, error) {
	result := ImportResult{}
	if path == "" || path == ":memory:" {
		return result, errors.New("caminho do banco legado inválido")
	}
	sourcePath, err := canonicalPath(path)
	if err != nil {
		return result, err
	}
	if s.path != ":memory:" {
		destPath, err := canonicalPath(s.path)
		if err != nil {
			return result, err
		}
		if sourcePath == destPath {
			return result, errors.New("origem e destino da importação são iguais")
		}
	}
	dsn, err := fileDSN(sourcePath, true)
	if err != nil {
		return result, err
	}
	source, err := sql.Open("sqlite", dsn)
	if err != nil {
		return result, err
	}
	defer source.Close()
	source.SetMaxOpenConns(1)
	read, err := source.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("abrir legado: %w", err)
	}
	defer read.Rollback()
	tables := map[string]bool{}
	rows, err := read.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return result, err
		}
		tables[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if !tables["profiles"] {
		return result, errors.New("banco não contém o schema legado de perfis")
	}
	hash := sha256.Sum256([]byte(sourcePath))
	prefix := fmt.Sprintf("legacy-%x", hash[:12])
	err = s.transaction(ctx, func(tx *sql.Tx) error {
		ids := map[int64]int64{}
		secrets := []string{}
		rows, err := read.QueryContext(ctx, "SELECT "+legacyProfileColumns+", '[]' FROM profiles ORDER BY id")
		if err != nil {
			return err
		}
		for rows.Next() {
			p, err := scanProfile(rows)
			if err != nil {
				rows.Close()
				return err
			}
			if p.Password != "" {
				secrets = append(secrets, p.Password)
			}
			r, err := tx.ExecContext(ctx, `INSERT INTO profiles
(nome,host,porta,usuario,senha,database_name,ssl,threads_default) VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(nome) DO NOTHING`, p.Name, p.Host, p.Port, p.User, p.Password, p.Database, p.SSL, p.Threads)
			if err != nil {
				rows.Close()
				return err
			}
			n, err := r.RowsAffected()
			if err != nil {
				rows.Close()
				return err
			}
			result.Profiles += int(n)
			var id int64
			if err := tx.QueryRowContext(ctx, "SELECT id FROM profiles WHERE nome=?", p.Name).Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids[p.ID] = id
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if tables["app_settings"] {
			rows, err := read.QueryContext(ctx, "SELECT key,value FROM app_settings ORDER BY key")
			if err != nil {
				return err
			}
			for rows.Next() {
				var key, value string
				if err := rows.Scan(&key, &value); err != nil {
					rows.Close()
					return err
				}
				if _, err := tx.ExecContext(ctx, "INSERT INTO app_settings(key,value) VALUES (?,?) ON CONFLICT(key) DO NOTHING", key, value); err != nil {
					rows.Close()
					return err
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		for _, history := range []struct{ table, kind, path string }{
			{"backup_history", "backup", "destination_dir"},
			{"restore_history", "restore", "source_dir"},
		} {
			if !tables[history.table] {
				continue
			}
			rows, err := read.QueryContext(ctx, "SELECT id,profile_id,profile_name,database_name,"+history.path+",status,started_at,finished_at,message FROM "+history.table+" ORDER BY id")
			if err != nil {
				return err
			}
			for rows.Next() {
				var oldID int64
				var oldProfile sql.NullInt64
				var finished, message sql.NullString
				job := core.Job{Kind: history.kind}
				if err := rows.Scan(&oldID, &oldProfile, &job.ProfileName, &job.Database, &job.Path, &job.Status, &job.StartedAt, &finished, &message); err != nil {
					rows.Close()
					return err
				}
				job.ID = fmt.Sprintf("%s-%s-%d", prefix, history.kind, oldID)
				job.ProfileID = ids[oldProfile.Int64]
				if job.ProfileID == 0 {
					err := tx.QueryRowContext(ctx, "SELECT id FROM profiles WHERE nome=?", job.ProfileName).Scan(&job.ProfileID)
					if err != nil && !errors.Is(err, sql.ErrNoRows) {
						rows.Close()
						return err
					}
				}
				job.Status = legacyStatus(job.Status)
				job.StartedAt, err = legacyTime(job.StartedAt)
				if err != nil {
					rows.Close()
					return err
				}
				job.FinishedAt, err = legacyTime(finished.String)
				if err != nil {
					rows.Close()
					return err
				}
				job.Message = message.String
				for _, secret := range secrets {
					job.Message = strings.ReplaceAll(job.Message, secret, "[REDACTED]")
				}
				if job.Status == "succeeded" {
					job.Progress = 100
				}
				if job.Status == "failed" && job.FinishedAt == "" {
					job.FinishedAt = job.StartedAt
					job.Message = "Histórico legado interrompido ou sem estado final reconhecido. " + job.Message
				}
				var exists bool
				if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE id=?)", job.ID).Scan(&exists); err != nil {
					rows.Close()
					return err
				}
				if !exists {
					if err := writeJob(ctx, tx, job, true); err != nil {
						rows.Close()
						return err
					}
					result.Jobs++
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("importar legado: %w", err)
	}
	return result, nil
}

func legacyStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		return "succeeded"
	case "CANCELLED", "CANCELED":
		return "cancelled"
	default:
		return "failed"
	}
}

func legacyTime(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}
	return "", fmt.Errorf("data do histórico legado inválida: %q", value)
}
