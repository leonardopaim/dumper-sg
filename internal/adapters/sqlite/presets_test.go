package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

const versionOneJobPayload = `{"id":"existing-job","kind":"backup","status":"succeeded","profile_id":7,"database":"source","started_at":"2026-10-07T01:00:00Z","message":"preserved history"}`

func versionOneFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "version-one.sqlite")
	dsn, err := fileDSN(path, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Build the actual old schema directly; opening a current Store would already migrate it.
	_, err = db.Exec(`CREATE TABLE profiles (
 id INTEGER PRIMARY KEY AUTOINCREMENT, nome TEXT NOT NULL UNIQUE,
 host TEXT NOT NULL, porta INTEGER NOT NULL, usuario TEXT NOT NULL,
 senha TEXT NOT NULL, database_name TEXT NOT NULL DEFAULT '',
 ssl INTEGER NOT NULL DEFAULT 0, threads_default INTEGER NOT NULL DEFAULT 8,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE jobs (id TEXT PRIMARY KEY, started_at TEXT NOT NULL, payload TEXT NOT NULL);
CREATE INDEX jobs_recent ON jobs(started_at DESC, id DESC);
INSERT INTO profiles VALUES (7,'existing','legacy-host',3307,'legacy-user','saved-password','source',1,12,'2026-10-01 01:00:00','2026-10-02 02:00:00');
INSERT INTO profiles VALUES (9,'other','other-host',3306,'other-user','','other-db',0,8,'2026-10-03 03:00:00','2026-10-04 04:00:00');
INSERT INTO app_settings VALUES ('default_backup_dir','C:/backups'),('other','keep');
PRAGMA user_version = 1;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO jobs VALUES (?,?,?)", "existing-job", "2026-10-07T01:00:00.000000000Z", versionOneJobPayload); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTablePresetsMigrateVersionOneAndPersistLifecycle(t *testing.T) {
	ctx := context.Background()
	path := versionOneFixture(t)
	s := openTestStore(t, path)
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatalf("migration version: %d %v", version, err)
	}
	profiles, err := s.ListProfiles(ctx)
	if err != nil || len(profiles) != 2 {
		t.Fatalf("migrated profiles: %+v %v", profiles, err)
	}
	expected := core.Profile{ID: 7, Name: "existing", Host: "legacy-host", Port: 3307, User: "legacy-user", Password: "saved-password", HasPassword: true, Database: "source", SSL: true, Threads: 12, TablePresets: []core.TablePreset{}}
	if !reflect.DeepEqual(profiles[0], expected) || profiles[1].ID != 9 || profiles[1].Password != "" || profiles[1].TablePresets == nil || len(profiles[1].TablePresets) != 0 {
		t.Fatalf("migration changed profile data: %+v", profiles)
	}
	var created, updated, presets, started, payload string
	if err := s.db.QueryRow("SELECT created_at, updated_at, table_presets FROM profiles WHERE id=7").Scan(&created, &updated, &presets); err != nil {
		t.Fatal(err)
	}
	if created != "2026-10-01 01:00:00" || updated != "2026-10-02 02:00:00" || presets != "[]" {
		t.Fatalf("migration changed metadata: %q %q %q", created, updated, presets)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(settings, map[string]string{"default_backup_dir": "C:/backups", "other": "keep"}) {
		t.Fatalf("migration changed settings: %v %v", settings, err)
	}
	if err := s.db.QueryRow("SELECT started_at,payload FROM jobs WHERE id='existing-job'").Scan(&started, &payload); err != nil {
		t.Fatal(err)
	}
	if started != "2026-10-07T01:00:00.000000000Z" || payload != versionOneJobPayload {
		t.Fatalf("migration changed history: %q %q", started, payload)
	}
	expected.TablePresets = []core.TablePreset{{Name: "Financeiro ação", Database: "source", Tables: []string{"source.clientes", "source.pedidos"}}, {Name: "Outro banco", Database: "other-db", Tables: []string{"other-db.items"}}}
	saved, err := s.SaveProfile(ctx, expected)
	if err != nil || !reflect.DeepEqual(saved, expected) {
		t.Fatalf("save migrated profile: %+v %v", saved, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	loaded, err := s.GetProfile(ctx, expected.ID)
	if err != nil || !reflect.DeepEqual(loaded, expected) {
		t.Fatalf("reopened presets: %+v %v", loaded, err)
	}
	loaded.Password = "updated-password"
	loaded.TablePresets[0].Tables = []string{"source.new_table"}
	if _, err := s.SaveProfile(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	profiles, err = s.ListProfiles(ctx)
	if err != nil || !reflect.DeepEqual(profiles[0], loaded) {
		t.Fatalf("updated presets/password: %+v %v", profiles, err)
	}
	loaded.TablePresets = nil
	saved, err = s.SaveProfile(ctx, loaded)
	if err != nil || saved.TablePresets == nil || len(saved.TablePresets) != 0 {
		t.Fatalf("clear presets: %+v %v", saved, err)
	}
	if err := s.db.QueryRow("SELECT table_presets FROM profiles WHERE id=7").Scan(&presets); err != nil || presets != "[]" {
		t.Fatalf("cleared presets storage: %q %v", presets, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	loaded, err = s.GetProfile(ctx, expected.ID)
	if err != nil || loaded.TablePresets == nil || len(loaded.TablePresets) != 0 || loaded.Password != "updated-password" {
		t.Fatalf("reopened cleared presets: %+v %v", loaded, err)
	}
}

func TestTablePresetsFreshProfilesDoNotAliasCallerOrReads(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ":memory:")
	p := core.Profile{Name: "one", TablePresets: []core.TablePreset{{Name: "Selection", Database: "db", Tables: []string{"db.one"}}}}
	saved, err := s.SaveProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.TablePresets[0].Name = "caller change"
	p.TablePresets[0].Tables[0] = "db.caller"
	if saved.TablePresets[0].Name != "Selection" || saved.TablePresets[0].Tables[0] != "db.one" {
		t.Fatalf("returned profile aliases caller: %+v", saved)
	}
	saved.TablePresets[0].Tables[0] = "db.returned"
	profiles, err := s.ListProfiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].TablePresets[0].Tables[0] != "db.one" {
		t.Fatalf("stored presets changed through return: %+v %v", profiles, err)
	}
	profiles[0].TablePresets[0].Tables[0] = "db.list"
	loaded, err := s.GetProfile(ctx, saved.ID)
	if err != nil || loaded.TablePresets[0].Tables[0] != "db.one" {
		t.Fatalf("stored presets changed through list: %+v %v", loaded, err)
	}
}

func TestTablePresetsMalformedJSONIsReported(t *testing.T) {
	for _, payload := range []string{"", "[", `{}`, `[{"tables":7}]`} {
		t.Run(payload, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t, ":memory:")
			p, err := s.SaveProfile(ctx, core.Profile{Name: "one"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec("UPDATE profiles SET table_presets=? WHERE id=?", payload, p.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetProfile(ctx, p.ID); err == nil || !strings.Contains(err.Error(), "seleções de tabelas") {
				t.Fatalf("get accepted corrupt presets: %v", err)
			}
			if _, err := s.ListProfiles(ctx); err == nil {
				t.Fatal("list accepted corrupt presets")
			}
		})
	}
}

func TestVersionTwoMigrationRollsBackOnInitializationFailure(t *testing.T) {
	path := versionOneFixture(t)
	dsn, err := fileDSN(path, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("UPDATE jobs SET payload='broken-json'"); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("malformed history accepted")
	}
	var version, columns int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("failed migration changed version: %d %v", version, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('profiles') WHERE name='table_presets'").Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("failed migration retained new column: %d %v", columns, err)
	}
}
