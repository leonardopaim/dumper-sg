package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPersistenceAndInterruptedJobs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s := openTestStore(t, path)
	profiles, err := s.ListProfiles(ctx)
	if err != nil || profiles == nil || len(profiles) != 0 {
		t.Fatalf("empty profiles: %v %v", profiles, err)
	}
	jobs, err := s.ListJobs(ctx, 10)
	if err != nil || jobs == nil || len(jobs) != 0 {
		t.Fatalf("empty jobs: %v %v", jobs, err)
	}
	p, err := s.SaveProfile(ctx, core.Profile{Name: "local", Host: "localhost", Port: 3306, User: "user", Password: "test-secret", Database: "source", SSL: true, Threads: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, map[string]string{"default_backup_dir": "C:/backup", "other": "keep"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, map[string]string{"default_backup_dir": "C:/new"}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"running", "cancel_requested", "succeeded"} {
		if err := s.SaveJob(ctx, core.Job{ID: status, Kind: "backup", Status: status, ProfileID: p.ID, StartedAt: "2026-10-07T01:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	loaded, err := s.GetProfile(ctx, p.ID)
	if err != nil || loaded.Password != p.Password || !loaded.SSL || !loaded.HasPassword {
		t.Fatalf("profile persistence: %+v %v", loaded, err)
	}
	public, err := json.Marshal(loaded.Public())
	if err != nil || strings.Contains(string(public), "test-secret") || !strings.Contains(string(public), `"has_password":true`) {
		t.Fatalf("public profile leaked: %s %v", public, err)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil || settings["default_backup_dir"] != "C:/new" || settings["other"] != "keep" {
		t.Fatalf("partial settings: %v %v", settings, err)
	}
	for _, id := range []string{"running", "cancel_requested"} {
		job, err := s.GetJob(ctx, id)
		if err != nil || job.Status != "failed" || !job.CleanupRequired || job.FinishedAt == "" || !strings.Contains(job.Message, "interrompida") || !strings.Contains(job.Message, "container Docker") {
			t.Fatalf("interrupted job: %+v %v", job, err)
		}
	}
	pending, err := s.PendingJobs(ctx)
	if err != nil || len(pending) != 2 {
		t.Fatalf("interrupted guards: %+v %v", pending, err)
	}
	job, err := s.GetJob(ctx, "succeeded")
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("terminal job changed: %+v %v", job, err)
	}
	var version int
	var journal string
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("schema version: %d %v", version, err)
	}
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal: %s %v", journal, err)
	}
}

func TestProfileCRUDConflictsAndNotFound(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ":memory:")
	p, err := s.SaveProfile(ctx, core.Profile{Name: "one", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveProfile(ctx, core.Profile{Name: "one"}); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	second, err := s.SaveProfile(ctx, core.Profile{Name: "two"})
	if err != nil {
		t.Fatal(err)
	}
	second.Name = "one"
	if _, err := s.SaveProfile(ctx, second); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate update: %v", err)
	}
	second, err = s.GetProfile(ctx, second.ID)
	if err != nil || second.Name != "two" {
		t.Fatalf("failed update changed row: %+v %v", second, err)
	}
	p.Name = "renamed"
	p.Password = ""
	p, err = s.SaveProfile(ctx, p)
	if err != nil || p.HasPassword {
		t.Fatalf("update profile: %+v %v", p, err)
	}
	if err := s.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProfile(ctx, p.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("get deleted: %v", err)
	}
	if err := s.DeleteProfile(ctx, p.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if _, err := s.SaveProfile(ctx, p); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if _, err := s.GetJob(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("job missing: %v", err)
	}
}

func TestJobOrderingAndUpsert(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ":memory:")
	for _, job := range []core.Job{
		{ID: "older", StartedAt: "2026-10-06T23:00:00Z", Status: "succeeded"},
		{ID: "a", StartedAt: "2026-10-07T01:00:00Z", Status: "running"},
		{ID: "b", StartedAt: "2026-10-07T01:00:00Z", Status: "failed"},
	} {
		if err := s.SaveJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := s.ListJobs(ctx, 2)
	if err != nil || len(jobs) != 2 || jobs[0].ID != "b" || jobs[1].ID != "a" {
		t.Fatalf("ordering/limit: %+v %v", jobs, err)
	}
	jobs[1].Status = "cancelled"
	if err := s.SaveJob(ctx, jobs[1]); err != nil {
		t.Fatal(err)
	}
	jobs, err = s.ListJobs(ctx, 0)
	if err != nil || len(jobs) != 3 || jobs[1].Status != "cancelled" {
		t.Fatalf("upsert/default limit: %+v %v", jobs, err)
	}
}

func legacyFixture(t *testing.T, broken bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	dsn, err := fileDSN(path, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE profiles(id INTEGER PRIMARY KEY, nome TEXT UNIQUE, host TEXT, porta INTEGER, usuario TEXT, senha TEXT, database_name TEXT, ssl INTEGER, threads_default INTEGER,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
INSERT INTO profiles(id,nome,host,porta,usuario,senha,database_name,ssl,threads_default) VALUES(7,'existing','legacy-host',3306,'user','fixture-password','db',0,8);
INSERT INTO profiles(id,nome,host,porta,usuario,senha,database_name,ssl,threads_default) VALUES(8,'new','localhost',3307,'user','other-secret','db',1,4);
CREATE TABLE app_settings(key TEXT PRIMARY KEY,value TEXT);
INSERT INTO app_settings VALUES('default_backup_dir','legacy-backups'),('other','imported');
CREATE TABLE backup_history(id INTEGER PRIMARY KEY,profile_id INTEGER,profile_name TEXT,database_name TEXT,destination_dir TEXT,status TEXT,started_at TEXT,finished_at TEXT,message TEXT);
INSERT INTO backup_history VALUES(2,7,'existing','db','backup','SUCCESS','2026-10-07 01:00:00','2026-10-07 01:01:00','done fixture-password');
INSERT INTO backup_history VALUES(3,8,'new','db','backup2','RUNNING','2026-10-07 02:00:00',NULL,NULL);
CREATE TABLE restore_history(id INTEGER PRIMARY KEY,profile_id INTEGER,profile_name TEXT,database_name TEXT,source_dir TEXT,status TEXT,started_at TEXT,finished_at TEXT,message TEXT);
INSERT INTO restore_history VALUES(2,8,'new','restored','backup','CANCELLED','2026-10-07 03:00:00','2026-10-07 03:01:00','cancelled');`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if broken {
		if _, err := db.Exec("ALTER TABLE restore_history RENAME COLUMN source_dir TO wrong_column"); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportIdempotentReadOnlyAndRemapsIDs(t *testing.T) {
	ctx := context.Background()
	path := legacyFixture(t, false)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, filepath.Join(t.TempDir(), "state.sqlite"))
	p, err := s.SaveProfile(ctx, core.Profile{Name: "existing", Host: "retain", Password: "retain-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, map[string]string{"default_backup_dir": "explicit-backups"}); err != nil {
		t.Fatal(err)
	}
	r, err := s.ImportLegacy(ctx, path)
	if err != nil || r.Profiles != 1 || r.Jobs != 3 {
		t.Fatalf("first import: %+v %v", r, err)
	}
	r, err = s.ImportLegacy(ctx, path)
	if err != nil || r != (ImportResult{}) {
		t.Fatalf("second import: %+v %v", r, err)
	}
	profiles, err := s.ListProfiles(ctx)
	if err != nil || len(profiles) != 2 || profiles[0].Host != "retain" || profiles[0].Password != "retain-password" {
		t.Fatalf("conflict preservation: %+v %v", profiles, err)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil || settings["default_backup_dir"] != "explicit-backups" || settings["other"] != "imported" {
		t.Fatalf("settings import: %+v %v", settings, err)
	}
	jobs, err := s.ListJobs(ctx, 10)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("jobs import: %+v %v", jobs, err)
	}
	if jobs[0].Kind != "restore" || jobs[0].Status != "cancelled" || jobs[0].ProfileID != profiles[1].ID {
		t.Fatalf("restore remap: %+v", jobs[0])
	}
	if jobs[1].Status != "failed" || jobs[1].FinishedAt == "" {
		t.Fatalf("running import: %+v", jobs[1])
	}
	for _, job := range jobs {
		if job.CleanupRequired {
			t.Fatalf("legacy history should not guard modern containers: %+v", job)
		}
	}
	if jobs[2].Status != "succeeded" || jobs[2].ProfileID != p.ID || jobs[2].Progress != 100 || jobs[2].StartedAt != "2026-10-07T01:00:00Z" || strings.Contains(jobs[2].Message, "fixture-password") {
		t.Fatalf("backup remap: %+v", jobs[2])
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatalf("source changed: %v", err)
	}
	otherSource := legacyFixture(t, false)
	r, err = s.ImportLegacy(ctx, otherSource)
	if err != nil || r.Profiles != 0 || r.Jobs != 3 {
		t.Fatalf("distinct source IDs: %+v %v", r, err)
	}
}

func TestImportRollbackAndMissingSource(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, filepath.Join(t.TempDir(), "state.sqlite"))
	r, err := s.ImportLegacy(ctx, legacyFixture(t, true))
	if err == nil || r != (ImportResult{}) {
		t.Fatalf("broken import succeeded: %+v %v", r, err)
	}
	profiles, err := s.ListProfiles(ctx)
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profile rollback: %+v %v", profiles, err)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil || len(settings) != 0 {
		t.Fatalf("settings rollback: %+v %v", settings, err)
	}
	jobs, err := s.ListJobs(ctx, 10)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("job rollback: %+v %v", jobs, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := s.ImportLegacy(ctx, missing); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("read-only import created source: %v", err)
	}
	if _, err := s.ImportLegacy(ctx, s.path); err == nil {
		t.Fatal("self import accepted")
	}
}

func TestRejectFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	dsn, err := fileDSN(path, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("future schema accepted")
	}
}

func TestImportDefaultSettingAndMigrateLegacyDestination(t *testing.T) {
	ctx := context.Background()
	source := legacyFixture(t, false)
	destination := legacyFixture(t, false)
	s := openTestStore(t, destination)
	// Opening a version-zero legacy database preserves its compatible profile/settings tables.
	profiles, err := s.ListProfiles(ctx)
	if err != nil || len(profiles) != 2 || profiles[0].ID != 7 {
		t.Fatalf("legacy schema migration: %+v %v", profiles, err)
	}
	if _, err := s.db.Exec("DELETE FROM app_settings"); err != nil {
		t.Fatal(err)
	}
	r, err := s.ImportLegacy(ctx, source)
	if err != nil || r.Profiles != 0 || r.Jobs != 3 {
		t.Fatalf("migrated destination import: %+v %v", r, err)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil || settings["default_backup_dir"] != "legacy-backups" {
		t.Fatalf("import missing default setting: %v %v", settings, err)
	}
}

func TestNanosecondOrderingAndNormalizeExistingKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s := openTestStore(t, path)
	for _, job := range []core.Job{
		{ID: "a", StartedAt: "2026-10-07T01:00:00Z"},
		{ID: "b", StartedAt: "2026-10-07T01:00:00.1Z"},
		{ID: "c", StartedAt: "2026-10-07T01:00:00.100000001Z"},
		{ID: "d", StartedAt: "2026-10-07T01:00:00.100000001Z"},
		{ID: "e", StartedAt: "2026-10-07T03:00:00.100000002+02:00"},
	} {
		if err := s.SaveJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	assertOrder := func() {
		t.Helper()
		jobs, err := s.ListJobs(ctx, 10)
		if err != nil || len(jobs) != 5 {
			t.Fatalf("jobs: %+v %v", jobs, err)
		}
		for i, id := range []string{"e", "d", "c", "b", "a"} {
			if jobs[i].ID != id {
				t.Fatalf("chronological order: %+v", jobs)
			}
		}
		if jobs[4].StartedAt != "2026-10-07T01:00:00Z" || jobs[0].StartedAt != "2026-10-07T03:00:00.100000002+02:00" {
			t.Fatalf("normalizing the index changed payload dates: %+v", jobs)
		}
		var firstKey, lastKey string
		if err := s.db.QueryRow("SELECT started_at FROM jobs WHERE id='a'").Scan(&firstKey); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow("SELECT started_at FROM jobs WHERE id='e'").Scan(&lastKey); err != nil {
			t.Fatal(err)
		}
		if firstKey != "2026-10-07T01:00:00.000000000Z" || lastKey != "2026-10-07T01:00:00.100000002Z" {
			t.Fatalf("fixed UTC keys: %s %s", firstKey, lastKey)
		}
	}
	assertOrder()
	// Simulate keys persisted before the ordering fix without changing job payloads.
	if _, err := s.db.Exec("UPDATE jobs SET started_at=json_extract(payload, '$.started_at')"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	assertOrder()
}

func TestImportNanosecondOrderingSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	source := legacyFixture(t, false)
	dsn, err := fileDSN(source, false)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.Exec(`UPDATE backup_history SET status='SUCCESS', started_at=
CASE id WHEN 2 THEN '2026-10-07T03:00:00.000000001Z' ELSE '2026-10-07 03:00:00.000000002' END`)
	if err != nil {
		fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s := openTestStore(t, path)
	if r, err := s.ImportLegacy(ctx, source); err != nil || r.Jobs != 3 {
		t.Fatalf("import: %+v %v", r, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	jobs, err := s.ListJobs(ctx, 10)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	want := []string{"2026-10-07T03:00:00.000000002Z", "2026-10-07T03:00:00.000000001Z", "2026-10-07T03:00:00Z"}
	for i := range want {
		if jobs[i].StartedAt != want[i] {
			t.Fatalf("import precision/order: %+v", jobs)
		}
	}
	if r, err := s.ImportLegacy(ctx, source); err != nil || r != (ImportResult{}) {
		t.Fatalf("idempotent reimport: %+v %v", r, err)
	}
}

func TestPendingJobsPersistsBeyondHistoryLimit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s := openTestStore(t, path)
	pending, err := s.PendingJobs(ctx)
	if err != nil || pending == nil || len(pending) != 0 {
		t.Fatalf("empty guards: %+v %v", pending, err)
	}
	guard := core.Job{ID: "old-pending", Status: "failed", StartedAt: "2026-10-06T01:00:00Z", CleanupRequired: true}
	if err := s.SaveJob(ctx, guard); err != nil {
		t.Fatal(err)
	}
	for i := range 105 {
		if err := s.SaveJob(ctx, core.Job{ID: fmt.Sprintf("recent-%03d", i), Status: "succeeded", StartedAt: "2026-10-07T01:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	jobs, err := s.ListJobs(ctx, 100)
	if err != nil || len(jobs) != 100 {
		t.Fatalf("history: %+v %v", jobs, err)
	}
	for _, job := range jobs {
		if job.ID == guard.ID {
			t.Fatal("guard fixture is inside recent history")
		}
	}
	pending, err = s.PendingJobs(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != guard.ID {
		t.Fatalf("unbounded guards: %+v %v", pending, err)
	}
	guard.CleanupRequired = false
	if err := s.SaveJob(ctx, guard); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	pending, err = s.PendingJobs(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("resolved guard persisted: %+v %v", pending, err)
	}
}
