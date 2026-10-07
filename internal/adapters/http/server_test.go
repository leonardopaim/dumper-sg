package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/core"
	"dumpersg/internal/jobs"
)

type blockedExecutor struct{}

func (blockedExecutor) Run(ctx context.Context, _ core.Command, emit func(string, string)) error {
	emit("INFO", "iniciado")
	<-ctx.Done()
	return ctx.Err()
}
func fixture(t *testing.T) (*Server, *sqlite.Store) {
	t.Helper()
	dir := t.TempDir()
	repo, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	manager := jobs.New(repo, blockedExecutor{}, core.Config{BackupDir: dir, LogDir: dir, DockerImage: "test", MySQLImage: "test"})
	t.Cleanup(func() { _ = manager.Close(context.Background()); _ = repo.Close() })
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>DumperSG</html>")}, "assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")}}
	s, err := New(repo, manager, Options{AllowedHosts: []string{"localhost:8787"}, AllowedOrigins: []string{"http://localhost:8787"}, BackupDir: dir, Assets: assets, Importer: repo})
	if err != nil {
		t.Fatal(err)
	}
	return s, repo
}
func request(s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:8787"+path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("X-DumperSG-Token", token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestSecurityRejectsExternalPagesAndMissingTokens(t *testing.T) {
	s, _ := fixture(t)
	for _, tc := range []struct{ host, origin, site string }{{"evil.example:8787", "", ""}, {"localhost:8787", "https://evil.example", ""}, {"localhost:8787", "", "cross-site"}} {
		r := httptest.NewRequest("GET", "http://localhost:8787/api/v1/session", nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("externo recebeu %d", w.Code)
		}
	}
	if w := request(s, "POST", "/api/v1/profiles", `{"name":"dev"}`, ""); w.Code != 403 {
		t.Fatalf("sem token: %d", w.Code)
	}
	w := request(s, "GET", "/api/v1/session", "", "")
	var v map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v["token"]) < 32 {
		t.Fatal("token ausente")
	}
}
func TestProfileAPIHidesSecretAndPatchPreservesIt(t *testing.T) {
	s, repo := fixture(t)
	w := request(s, "POST", "/api/v1/profiles", `{"name":"dev","host":"localhost","user":"root","password":"super-secret","database":"production"}`, s.token)
	if w.Code != 201 {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "super-secret") || strings.Contains(w.Body.String(), `"password"`) {
		t.Fatal("senha exposta")
	}
	var p core.Profile
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if !p.HasPassword || p.Port != 3306 || p.Threads != 8 {
		t.Fatalf("defaults: %+v", p)
	}
	w = request(s, "PATCH", "/api/v1/profiles/1", `{"name":"renamed"}`, s.token)
	if w.Code != 200 {
		t.Fatalf("patch %d: %s", w.Code, w.Body)
	}
	saved, err := repo.GetProfile(context.Background(), p.ID)
	if err != nil || saved.Password != "super-secret" {
		t.Fatal("patch apagou senha")
	}
	w = request(s, "GET", "/api/v1/profiles", "", "")
	if strings.Contains(w.Body.String(), "super-secret") {
		t.Fatal("list expôs senha")
	}
	w = request(s, "PATCH", "/api/v1/profiles/1", `{"password":""}`, s.token)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	saved, _ = repo.GetProfile(context.Background(), p.ID)
	if saved.Password != "" {
		t.Fatal("não limpou senha")
	}
}
func TestRestoreGuardsApplyToDirectAPIClients(t *testing.T) {
	s, _ := fixture(t)
	w := request(s, "POST", "/api/v1/profiles", `{"name":"remote","host":"db.example.com","user":"root","database":"prod"}`, s.token)
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	w = request(s, "POST", "/api/v1/restores", `{"profile_id":1,"backup_dir":"not-found","target_database":"isolated"}`, s.token)
	if w.Code != 400 {
		t.Fatalf("restore remoto %d: %s", w.Code, w.Body)
	}
	w = request(s, "POST", "/api/v1/databases", `{"profile_id":1,"database":"isolated"}`, s.token)
	if w.Code != 400 {
		t.Fatalf("create remoto %d", w.Code)
	}
	request(s, "PATCH", "/api/v1/profiles/1", `{"host":"localhost"}`, s.token)
	w = request(s, "POST", "/api/v1/databases", `{"profile_id":1,"database":"PROD"}`, s.token)
	if w.Code != 400 {
		t.Fatalf("create default %d", w.Code)
	}
}
func TestJobOutlivesCreatingRequestAndBlocksConflicts(t *testing.T) {
	s, _ := fixture(t)
	request(s, "POST", "/api/v1/profiles", `{"name":"local","host":"localhost","user":"root"}`, s.token)
	w := request(s, "POST", "/api/v1/profiles/1/test", "", s.token)
	if w.Code != 202 {
		t.Fatalf("job %d: %s", w.Code, w.Body)
	}
	var job core.Job
	_ = json.Unmarshal(w.Body.Bytes(), &job)
	w = request(s, "GET", "/api/v1/jobs/"+job.ID, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "running") {
		t.Fatalf("job encerrado com request: %s", w.Body)
	}
	w = request(s, "POST", "/api/v1/profiles/1/test", "", s.token)
	if w.Code != 409 {
		t.Fatalf("conflito %d", w.Code)
	}
	w = request(s, "DELETE", "/api/v1/profiles/1", "", s.token)
	if w.Code != 409 {
		t.Fatalf("delete em uso %d", w.Code)
	}
	w = request(s, "POST", "/api/v1/jobs/"+job.ID+"/cancel", "", s.token)
	if w.Code != 200 {
		t.Fatalf("cancel %d", w.Code)
	}
}
func TestJSONLimitsSettingsAndSPAFallback(t *testing.T) {
	s, _ := fixture(t)
	for _, body := range []string{`{"unknown":true}`, `{} {}`, strings.Repeat("x", 70000)} {
		w := request(s, "POST", "/api/v1/profiles", body, s.token)
		if w.Code != 400 {
			t.Fatalf("JSON inválido %d", w.Code)
		}
	}
	w := request(s, "PATCH", "/api/v1/settings", `{"default_backup_dir":"relative"}`, s.token)
	if w.Code != 400 {
		t.Fatal("aceitou caminho relativo")
	}
	w = request(s, "GET", "/profiles", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "DumperSG") {
		t.Fatalf("SPA: %d", w.Code)
	}
	w = request(s, "GET", "/assets/missing.js", "", "")
	if w.Code != 404 {
		t.Fatal("asset ausente devolveu HTML")
	}
	w = request(s, "GET", "/api/v1/unknown", "", "")
	if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal("API desconhecida não JSON")
	}
	w = request(s, "GET", "/api/v1/history?limit=999999", "", "")
	if w.Code != 400 {
		t.Fatal("limite ilimitado")
	}
}
func TestOptionsAndEmptyLists(t *testing.T) {
	s, _ := fixture(t)
	w := request(s, http.MethodOptions, "/api/v1/profiles", "", "")
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	w = request(s, "GET", "/api/v1/profiles", "", "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("list vazia não array")
	}
}

func TestExplicitFrontendOriginCanConnect(t *testing.T) {
	s, _ := fixture(t)
	s.options.AllowedOrigins = append(s.options.AllowedOrigins, "http://127.0.0.1:5173")
	r := httptest.NewRequest("GET", "http://localhost:8787/api/v1/session", nil)
	r.Header.Set("Origin", "http://127.0.0.1:5173")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("frontend explicitamente autorizado bloqueado: %d", w.Code)
	}
}
