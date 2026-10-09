package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/core"
	"dumpersg/internal/jobs"
)

func TestTableAPIRequiresSessionAndBlocksConcurrentOperations(t *testing.T) {
	s, repo := fixture(t)
	_, err := repo.SaveProfile(context.Background(), core.Profile{Name: "QA", Host: "localhost", Port: 3306, User: "qa", Database: "source", Threads: 2})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"profile_id":1,"database":"source","ssl":false}`
	if w := request(s, "POST", "/api/v1/tables", body, ""); w.Code != 403 {
		t.Fatalf("missing session: %d", w.Code)
	}
	if w := request(s, "POST", "/api/v1/tables", `{"profile_id":1,"command":"DROP DATABASE source"}`, s.token); w.Code != 400 {
		t.Fatalf("unknown fields accepted: %d", w.Code)
	}
	w := request(s, "POST", "/api/v1/tables", body, s.token)
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	var job core.Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil || job.Kind != "table_list" {
		t.Fatalf("job: %s %v", w.Body, err)
	}
	if w := request(s, "GET", "/api/v1/jobs/"+job.ID+"/tables", "", ""); w.Code != 409 {
		t.Fatalf("partial catalog returned: %d %s", w.Code, w.Body)
	}
	if w := request(s, "POST", "/api/v1/tables", body, s.token); w.Code != 409 {
		t.Fatalf("concurrent catalog allowed: %d", w.Code)
	}
	if w := request(s, "POST", "/api/v1/backups", `{"profile_id":1,"tables":[]}`, s.token); w.Code != 400 {
		t.Fatalf("empty selection accepted: %d %s", w.Code, w.Body)
	}
}

func TestProfilePresetsPatchPreservesCredentialsAndSupportsClearing(t *testing.T) {
	s, repo := fixture(t)
	w := request(s, "POST", "/api/v1/profiles", `{"name":"QA","host":"localhost","user":"qa","password":"secret","database":""}`, s.token)
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	body := `{"table_presets":[{"name":"Essenciais","database":"source","tables":["sample","literal,+.[x]"]}]}`
	w = request(s, "PATCH", "/api/v1/profiles/1", body, s.token)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("patch %d: %s", w.Code, w.Body)
	}
	p, err := repo.GetProfile(context.Background(), 1)
	if err != nil || p.Password != "secret" || p.Database != "" || len(p.TablePresets) != 1 || len(p.TablePresets[0].Tables) != 2 {
		t.Fatalf("profile changed: %v", err)
	}
	w = request(s, "PATCH", "/api/v1/profiles/1", `{"table_presets":[{"name":"Vazio","database":"source","tables":[]}]}`, s.token)
	if w.Code != 400 {
		t.Fatalf("empty preset accepted: %d", w.Code)
	}
	w = request(s, "PATCH", "/api/v1/profiles/1", `{"table_presets":[]}`, s.token)
	if w.Code != 200 {
		t.Fatalf("clear: %d %s", w.Code, w.Body)
	}
	p, err = repo.GetProfile(context.Background(), 1)
	if err != nil || len(p.TablePresets) != 0 || p.Password != "secret" {
		t.Fatal("clearing presets changed credentials")
	}
}

func TestBackupTableCatalogIsLocalAndRequiresSession(t *testing.T) {
	s, _ := fixture(t)
	dir := t.TempDir()
	metadata := "[config]\nquote-character = BACKTICK\n[`source`.`sample`]\nreal_table_name=sample\nrows = 2\n# Finished dump at: 2026-10-08 12:00:00\n"
	if err := os.WriteFile(filepath.Join(dir, "metadata"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source.sample-schema.sql"), []byte("CREATE TABLE `sample` (`id` int);"), 0600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"backup_dir": dir})
	if w := request(s, "POST", "/api/v1/backups/tables", string(body), ""); w.Code != 403 {
		t.Fatalf("missing session: %d", w.Code)
	}
	w := request(s, "POST", "/api/v1/backups/tables", string(body), s.token)
	var tables []core.BackupTableInfo
	if err := json.Unmarshal(w.Body.Bytes(), &tables); err != nil || w.Code != 200 || len(tables) != 1 || tables[0].Database != "source" || tables[0].Name != "sample" || tables[0].SizeBytes == 0 {
		t.Fatalf("catalog: %d %s %v", w.Code, w.Body, err)
	}
	if w := request(s, "POST", "/api/v1/backups/tables", `{"backup_dir":"missing"}`, s.token); w.Code != 400 {
		t.Fatalf("missing directory: %d", w.Code)
	}
	if w := request(s, "POST", "/api/v1/backups/tables", `{"backup_dir":"missing","command":"arbitrary"}`, s.token); w.Code != 400 {
		t.Fatalf("unknown field: %d", w.Code)
	}
}

type catalogExecutor struct{}

func (catalogExecutor) Run(_ context.Context, cmd core.Command, emit func(string, string)) error {
	if cmd.StdoutData {
		emit("data", `{"name":"small","size_bytes":10,"rows":1,"table_type":"BASE TABLE"}`)
		emit("data", `{"name":"large","size_bytes":999,"rows":2,"table_type":"BASE TABLE"}`)
	}
	return nil
}

func TestTableAPIProvidesCompleteOrderedCatalog(t *testing.T) {
	dir := t.TempDir()
	repo, err := sqlite.Open(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := jobs.New(repo, catalogExecutor{}, core.Config{LogDir: dir})
	t.Cleanup(func() { _ = m.Close(context.Background()); _ = repo.Close() })
	_, err = repo.SaveProfile(context.Background(), core.Profile{Name: "QA", Host: "localhost", Port: 3306, User: "qa", Database: "source", Threads: 2})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(repo, m, Options{AllowedHosts: []string{"localhost:8787"}})
	if err != nil {
		t.Fatal(err)
	}
	w := request(s, "POST", "/api/v1/tables", `{"profile_id":1,"database":"source"}`, s.token)
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	var job core.Job
	_ = json.Unmarshal(w.Body.Bytes(), &job)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err = m.Get(context.Background(), job.ID)
		if err != nil || job.FinishedAt != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("catalog job: %+v %v", job, err)
	}
	w = request(s, "GET", "/api/v1/jobs/"+job.ID+"/tables", "", "")
	var catalog []core.TableInfo
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil || w.Code != 200 || len(catalog) != 2 || catalog[0].Name != "large" {
		t.Fatalf("catalog: %d %s %v", w.Code, w.Body, err)
	}
	if w := request(s, "GET", "/api/v1/jobs/missing/tables", "", ""); w.Code != 404 {
		t.Fatalf("missing result: %d", w.Code)
	}
}
