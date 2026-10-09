// Package sqlite persists local profiles, settings and operation history.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dumpersg/internal/core"
	modernsqlite "modernc.org/sqlite"
)

const schemaVersion = 2

type Store struct {
	db   *sql.DB
	path string
}

var _ core.Repository = (*Store)(nil)

func fileDSN(path string, readOnly bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "foreign_keys(1)")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("caminho SQLite vazio")
	}
	dsn := path
	if path == ":memory:" {
		dsn += "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	} else {
		var err error
		dsn, err = fileDSN(path, false)
		if err != nil {
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection also preserves in-memory databases and serializes writers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, path: path}
	if err := s.initialize(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("inicializar SQLite: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initialize(ctx context.Context) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		var version int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return err
		}
		if version > schemaVersion {
			return fmt.Errorf("versão SQLite %d não suportada", version)
		}
		if version == 0 {
			_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS profiles (
 id INTEGER PRIMARY KEY AUTOINCREMENT, nome TEXT NOT NULL UNIQUE,
 host TEXT NOT NULL, porta INTEGER NOT NULL, usuario TEXT NOT NULL,
 senha TEXT NOT NULL, database_name TEXT NOT NULL DEFAULT '',
 ssl INTEGER NOT NULL DEFAULT 0, threads_default INTEGER NOT NULL DEFAULT 8,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, started_at TEXT NOT NULL, payload TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS jobs_recent ON jobs(started_at DESC, id DESC);
PRAGMA user_version = 1;`)
			if err != nil {
				return err
			}
			version = 1
		}
		if version == 1 {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE profiles ADD COLUMN table_presets TEXT NOT NULL DEFAULT '[]';
PRAGMA user_version = 2;`); err != nil {
				return err
			}
		}
		rows, err := tx.QueryContext(ctx, "SELECT id, started_at, payload FROM jobs")
		if err != nil {
			return err
		}
		interrupted := []core.Job{}
		keys := []struct{ id, value string }{}
		for rows.Next() {
			var id, storedKey, payload string
			if err := rows.Scan(&id, &storedKey, &payload); err != nil {
				rows.Close()
				return err
			}
			var job core.Job
			if err := json.Unmarshal([]byte(payload), &job); err != nil {
				rows.Close()
				return err
			}
			key, err := jobTimeKey(job.StartedAt)
			if err != nil {
				rows.Close()
				return err
			}
			if job.Status == "running" || job.Status == "cancel_requested" {
				job.Status = "failed"
				job.CleanupRequired = true
				job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
				job.Message = "Operação interrompida pelo encerramento do aplicativo; não foi retomada. Verifique o container Docker antes de repetir."
				interrupted = append(interrupted, job)
			} else if key != storedKey {
				keys = append(keys, struct{ id, value string }{id, key})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := tx.ExecContext(ctx, "UPDATE jobs SET started_at=? WHERE id=?", key.value, key.id); err != nil {
				return err
			}
		}
		for _, job := range interrupted {
			if err := writeJob(ctx, tx, job, false); err != nil {
				return err
			}
		}
		return nil
	})
}

// A fixed UTC precision sorts lexically in chronological order, including
// integral seconds, RFC3339Nano fractions and timestamps with explicit offsets.
func jobTimeKey(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", fmt.Errorf("data da operação inválida: %w", err)
	}
	return parsed.UTC().Format("2006-01-02T15:04:05.000000000Z"), nil
}

func (s *Store) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func profileError(err error) error {
	var se *modernsqlite.Error
	if errors.As(err, &se) && (se.Code() == 2067 || se.Code() == 1555) {
		return fmt.Errorf("%w: nome de perfil já cadastrado", core.ErrConflict)
	}
	return err
}

const legacyProfileColumns = "id, nome, host, porta, usuario, senha, database_name, ssl, threads_default"
const profileColumns = legacyProfileColumns + ", table_presets"

type scanner interface{ Scan(...any) error }

func scanProfile(row scanner) (core.Profile, error) {
	var p core.Profile
	var presets string
	err := row.Scan(&p.ID, &p.Name, &p.Host, &p.Port, &p.User, &p.Password, &p.Database, &p.SSL, &p.Threads, &presets)
	if errors.Is(err, sql.ErrNoRows) {
		err = core.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(presets), &p.TablePresets); err != nil {
		return p, fmt.Errorf("seleções de tabelas do perfil %d inválidas: %w", p.ID, err)
	}
	if p.TablePresets == nil {
		p.TablePresets = []core.TablePreset{}
	}
	p.HasPassword = p.Password != ""
	return p, nil
}

func (s *Store) ListProfiles(ctx context.Context) ([]core.Profile, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+profileColumns+" FROM profiles ORDER BY nome, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []core.Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (s *Store) GetProfile(ctx context.Context, id int64) (core.Profile, error) {
	return scanProfile(s.db.QueryRowContext(ctx, "SELECT "+profileColumns+" FROM profiles WHERE id = ?", id))
}

func (s *Store) SaveProfile(ctx context.Context, p core.Profile) (core.Profile, error) {
	if p.TablePresets == nil {
		p.TablePresets = []core.TablePreset{}
	}
	presets, err := json.Marshal(p.TablePresets)
	if err != nil {
		return p, err
	}
	// Keep the returned profile independent of the caller's mutable selections.
	p.TablePresets = nil
	if err := json.Unmarshal(presets, &p.TablePresets); err != nil {
		return p, err
	}
	err = s.transaction(ctx, func(tx *sql.Tx) error {
		args := []any{p.Name, p.Host, p.Port, p.User, p.Password, p.Database, p.SSL, p.Threads, string(presets)}
		if p.ID == 0 {
			r, err := tx.ExecContext(ctx, `INSERT INTO profiles
(nome, host, porta, usuario, senha, database_name, ssl, threads_default, table_presets) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
			if err != nil {
				return profileError(err)
			}
			p.ID, err = r.LastInsertId()
			return err
		}
		args = append(args, p.ID)
		r, err := tx.ExecContext(ctx, `UPDATE profiles SET nome=?, host=?, porta=?, usuario=?, senha=?,
database_name=?, ssl=?, threads_default=?, table_presets=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, args...)
		if err != nil {
			return profileError(err)
		}
		n, err := r.RowsAffected()
		if err == nil && n == 0 {
			return core.ErrNotFound
		}
		return err
	})
	p.HasPassword = p.Password != ""
	return p, err
}

func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, "DELETE FROM profiles WHERE id=?", id)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err == nil && n == 0 {
			return core.ErrNotFound
		}
		return err
	})
}

func (s *Store) GetSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM app_settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, settings map[string]string) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		for k, v := range settings {
			if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key,value) VALUES (?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeJob(ctx context.Context, tx *sql.Tx, job core.Job, onlyNew bool) error {
	if job.ID == "" {
		return errors.New("ID de operação vazio")
	}
	key, err := jobTimeKey(job.StartedAt)
	if err != nil {
		return err
	}
	// Marshaling a typed Job prevents profile credentials or command secrets from being stored.
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	conflict := "DO UPDATE SET started_at=excluded.started_at, payload=excluded.payload"
	if onlyNew {
		conflict = "DO NOTHING"
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO jobs(id, started_at, payload) VALUES (?,?,?) ON CONFLICT(id) "+conflict,
		job.ID, key, string(payload))
	return err
}

func (s *Store) SaveJob(ctx context.Context, job core.Job) error {
	return s.transaction(ctx, func(tx *sql.Tx) error { return writeJob(ctx, tx, job, false) })
}

func scanJob(row scanner) (core.Job, error) {
	var payload string
	var job core.Job
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job, core.ErrNotFound
		}
		return job, err
	}
	err := json.Unmarshal([]byte(payload), &job)
	return job, err
}

func (s *Store) GetJob(ctx context.Context, id string) (core.Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id))
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]core.Job, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM jobs ORDER BY started_at DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []core.Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// BackupJobs includes older backup locations outside the paginated history.
func (s *Store) BackupJobs(ctx context.Context) ([]core.Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM jobs WHERE json_extract(payload, '$.kind')='backup' ORDER BY started_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []core.Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

// PendingJobs returns every job whose external termination still needs to be
// verified, including jobs older than the paginated history window.
func (s *Store) PendingJobs(ctx context.Context) ([]core.Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM jobs
WHERE json_extract(payload, '$.cleanup_required')=1 ORDER BY started_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []core.Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
