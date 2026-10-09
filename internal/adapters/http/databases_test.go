package httpapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/jobs"

	"dumpersg/internal/core"
)

func TestDatabaseCatalogRequiresSessionAndKeepsExclusiveOperation(t *testing.T) {
	s, repo := fixture(t)
	if _, err := repo.SaveProfile(context.Background(), core.Profile{Name: "QA", Host: "localhost", Port: 3306, User: "qa", Threads: 2}); err != nil {
		t.Fatal(err)
	}
	body := `{"profile_id":1}`
	if w := request(s, "POST", "/api/v1/databases/catalog", body, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/v1/databases/catalog", `{"profile_id":1,"sql":"DROP DATABASE x"}`, s.token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/v1/databases/catalog", `{"profile_id":0}`, s.token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w := request(s, "POST", "/api/v1/databases/catalog", body, s.token)
	var job core.Job
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &job) != nil || job.Kind != "database_list" {
		t.Fatal(w.Code, w.Body)
	}
	if w := request(s, "GET", "/api/v1/jobs/"+job.ID+"/databases", "", ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/v1/jobs/"+job.ID+"/tables", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/v1/databases/catalog", body, s.token); w.Code != 409 {
		t.Fatal(w.Code)
	}
}

type databaseAPIExecutor struct{}

func (databaseAPIExecutor) Run(_ context.Context, cmd core.Command, emit func(string, string)) error {
	if strings.Contains(strings.Join(cmd.Args, " "), "sommusgestor.empresa") {
		emit("data", `{"company_id":1,"group_id":12,"legal_name":"Empresa Ltda","trade_name":"Loja"}`)
	} else if strings.Contains(strings.Join(cmd.Args, " "), "sommusgestor.grupo_empresa") {
		emit("data", `{"group_id":12,"group_name":"Comércio São José"}`)
	} else {
		emit("data", `{"name":"sommusgestor_12"}`)
	}
	return nil
}

func TestDatabaseAPIProvidesCatalogAndGroupNamesWithoutCredentials(t *testing.T) {
	dir := t.TempDir()
	repo, err := sqlite.Open(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := jobs.New(repo, databaseAPIExecutor{}, core.Config{LogDir: dir})
	t.Cleanup(func() { _ = m.Close(context.Background()); _ = repo.Close() })
	if _, err := repo.SaveProfile(context.Background(), core.Profile{Name: "QA", Host: "localhost", Port: 3306, User: "qa", Password: "private-password", Threads: 2}); err != nil {
		t.Fatal(err)
	}
	s, err := New(repo, m, Options{AllowedHosts: []string{"localhost:8787"}})
	if err != nil {
		t.Fatal(err)
	}
	w := request(s, "POST", "/api/v1/databases/catalog", `{"profile_id":1}`, s.token)
	var job core.Job
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &job) != nil {
		t.Fatal(w.Code, w.Body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err = m.Get(context.Background(), job.ID)
		if err != nil || job.FinishedAt != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil || job.Status != "succeeded" {
		t.Fatal(job, err)
	}
	w = request(s, "GET", "/api/v1/jobs/"+job.ID+"/databases", "", "")
	var result core.DatabaseCatalog
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Databases) != 1 || result.Databases[0].GroupName != "Comércio São José" || result.Databases[0].GroupID != 12 {
		t.Fatal(w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "private-password") {
		t.Fatal("credentials leaked")
	}
	if len(result.Databases[0].Companies) != 1 || result.Databases[0].Companies[0].TradeName != "Loja" || result.Warning != "" {
		t.Fatal(w.Body)
	}
	if w := request(s, "GET", "/api/v1/jobs/missing/databases", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
