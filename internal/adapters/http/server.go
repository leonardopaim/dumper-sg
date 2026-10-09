package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	store "dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/core"
)

type Operations interface {
	StartBackup(context.Context, core.BackupRequest) (core.Job, error)
	StartRestore(context.Context, core.RestoreRequest) (core.Job, error)
	StartTest(context.Context, int64) (core.Job, error)
	StartCreateDatabase(context.Context, core.DatabaseRequest) (core.Job, error)
	StartTableList(context.Context, core.TableListRequest) (core.Job, error)
	Tables(context.Context, string) ([]core.TableInfo, error)
	Get(context.Context, string) (core.Job, error)
	List(context.Context, int) ([]core.Job, error)
	Events(context.Context, string, int64) ([]core.Event, error)
	Cancel(context.Context, string) (core.Job, error)
}
type Importer interface {
	ImportLegacy(context.Context, string) (store.ImportResult, error)
}
type Options struct {
	AllowedHosts     []string
	AllowedOrigins   []string
	Assets           fs.FS
	BackupDir        string
	Diagnostics      func(context.Context) core.Diagnostics
	Importer         Importer
	Restart          func(context.Context) error
	Shutdown         func(context.Context) error
	InstanceID       string
	OpenBackupFolder func(string) error
}
type Server struct {
	repo     core.Repository
	ops      Operations
	options  Options
	token    string
	instance string
	handler  http.Handler
}

func New(repo core.Repository, ops Operations, options Options) (*Server, error) {
	secret := make([]byte, 48)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	s := &Server{repo: repo, ops: ops, options: options, token: hex.EncodeToString(secret[:32]), instance: hex.EncodeToString(secret[32:])}
	if options.InstanceID != "" {
		if len(options.InstanceID) != 32 || strings.Trim(options.InstanceID, "0123456789abcdef") != "" {
			return nil, errors.New("identificação da instância inválida")
		}
		s.instance = options.InstanceID
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/session", s.session)
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": "1.0.0"})
	})
	mux.HandleFunc("GET /api/v1/diagnostics", s.diagnostics)
	mux.HandleFunc("GET /api/v1/application", s.application)
	mux.HandleFunc("POST /api/v1/application/restart", s.restart)
	mux.HandleFunc("POST /api/v1/application/shutdown", s.shutdown)
	mux.HandleFunc("GET /api/v1/profiles", s.profiles)
	mux.HandleFunc("POST /api/v1/profiles", s.createProfile)
	mux.HandleFunc("GET /api/v1/profiles/{id}", s.profile)
	mux.HandleFunc("PATCH /api/v1/profiles/{id}", s.updateProfile)
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", s.deleteProfile)
	mux.HandleFunc("POST /api/v1/profiles/{id}/test", s.testConnection)
	mux.HandleFunc("POST /api/v1/backups", s.backup)
	mux.HandleFunc("POST /api/v1/backups/tables", s.backupTables)
	mux.HandleFunc("POST /api/v1/tables", s.listTables)
	mux.HandleFunc("GET /api/v1/backups", s.backups)
	mux.HandleFunc("DELETE /api/v1/backups/{id}", s.deleteBackup)
	mux.HandleFunc("POST /api/v1/backups/{id}/open", s.openBackup)
	mux.HandleFunc("POST /api/v1/restores", s.restore)
	mux.HandleFunc("POST /api/v1/databases", s.database)
	mux.HandleFunc("GET /api/v1/jobs", s.history)
	mux.HandleFunc("GET /api/v1/history", s.history)
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.job)
	mux.HandleFunc("GET /api/v1/jobs/{id}/events", s.events)
	mux.HandleFunc("GET /api/v1/jobs/{id}/tables", s.tableResults)
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.cancel)
	mux.HandleFunc("GET /api/v1/settings", s.settings)
	mux.HandleFunc("PATCH /api/v1/settings", s.updateSettings)
	mux.HandleFunc("POST /api/v1/import/legacy", s.importLegacy)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]string{"error": "rota não encontrada"})
	})
	mux.HandleFunc("/", s.assets)
	s.handler = s.security(mux)
	return s, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }
func contains(items []string, value string) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if !contains(s.options.AllowedHosts, r.Host) {
			writeJSON(w, 403, map[string]string{"error": "host não autorizado"})
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !contains(s.options.AllowedOrigins, origin) {
			writeJSON(w, 403, map[string]string{"error": "origem não autorizada"})
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" && origin == "" {
			writeJSON(w, 403, map[string]string{"error": "requisição externa bloqueada"})
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-DumperSG-Token")
			w.WriteHeader(204)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-DumperSG-Token")), []byte(s.token)) != 1 {
				writeJSON(w, 403, map[string]string{"error": "sessão inválida; atualize a página"})
				return
			}
			if r.ContentLength > 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeJSON(w, 415, map[string]string{"error": "use application/json"})
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, core.ErrNotFound) {
		status = 404
	}
	if errors.Is(err, core.ErrConflict) {
		status = 409
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return errors.New("JSON inválido ou campos desconhecidos")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("envie apenas um objeto JSON")
	}
	return nil
}
func id(r *http.Request) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || n <= 0 {
		return 0, core.ErrNotFound
	}
	return n, nil
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"token": s.token})
}
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if s.options.Diagnostics == nil {
		writeJSON(w, 200, core.Diagnostics{Message: "Diagnóstico indisponível"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, 200, s.options.Diagnostics(ctx))
}
func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.repo.ListProfiles(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]core.Profile, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Public())
	}
	writeJSON(w, 200, out)
}
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	n, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	p, err := s.repo.GetProfile(r.Context(), n)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, p.Public())
}
func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	var p core.Profile
	if err := decode(w, r, &p); err != nil {
		fail(w, err)
		return
	}
	p.ID = 0
	if p.Port == 0 {
		p.Port = 3306
	}
	if p.Threads == 0 {
		p.Threads = 8
	}
	if err := core.ValidateProfile(p); err != nil {
		fail(w, err)
		return
	}
	p, err := s.repo.SaveProfile(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, p.Public())
}

type profilePatch struct {
	Name         *string             `json:"name"`
	Host         *string             `json:"host"`
	Port         *int                `json:"port"`
	User         *string             `json:"user"`
	Password     *string             `json:"password"`
	Database     *string             `json:"database"`
	SSL          *bool               `json:"ssl"`
	Threads      *int                `json:"threads"`
	TablePresets *[]core.TablePreset `json:"table_presets"`
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	n, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	p, err := s.repo.GetProfile(r.Context(), n)
	if err != nil {
		fail(w, err)
		return
	}
	var v profilePatch
	if err = decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	if v.Name != nil {
		p.Name = *v.Name
	}
	if v.Host != nil {
		p.Host = *v.Host
	}
	if v.Port != nil {
		p.Port = *v.Port
	}
	if v.User != nil {
		p.User = *v.User
	}
	if v.Password != nil {
		p.Password = *v.Password
	}
	if v.Database != nil {
		p.Database = *v.Database
	}
	if v.SSL != nil {
		p.SSL = *v.SSL
	}
	if v.Threads != nil {
		p.Threads = *v.Threads
	}
	if v.TablePresets != nil {
		p.TablePresets = *v.TablePresets
	}
	if err = core.ValidateProfile(p); err != nil {
		fail(w, err)
		return
	}
	p, err = s.repo.SaveProfile(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, p.Public())
}
func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	n, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	js, err := s.ops.List(r.Context(), 100)
	if err != nil {
		fail(w, err)
		return
	}
	for _, j := range js {
		if j.ProfileID == n && (j.Status == "running" || j.Status == "cancel_requested") {
			fail(w, core.ErrConflict)
			return
		}
	}
	if err = s.repo.DeleteProfile(r.Context(), n); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func result(w http.ResponseWriter, j core.Job, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 202, j)
}
func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	n, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	j, err := s.ops.StartTest(r.Context(), n)
	result(w, j, err)
}
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	var v core.BackupRequest
	if err := decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	j, err := s.ops.StartBackup(r.Context(), v)
	result(w, j, err)
}
func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var v core.RestoreRequest
	if err := decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	j, err := s.ops.StartRestore(r.Context(), v)
	result(w, j, err)
}
func (s *Server) database(w http.ResponseWriter, r *http.Request) {
	var v core.DatabaseRequest
	if err := decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	j, err := s.ops.StartCreateDatabase(r.Context(), v)
	result(w, j, err)
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			fail(w, errors.New("limit deve estar entre 1 e 500"))
			return
		}
		limit = n
	}
	js, err := s.ops.List(r.Context(), limit)
	if err != nil {
		fail(w, err)
		return
	}
	if js == nil {
		js = []core.Job{}
	}
	writeJSON(w, 200, js)
}
func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	j, err := s.ops.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, j)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	j, err := s.ops.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, j)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			fail(w, errors.New("cursor inválido"))
			return
		}
		after = n
	}
	es, err := s.ops.Events(r.Context(), r.PathValue("id"), after)
	if err != nil {
		fail(w, err)
		return
	}
	if es == nil {
		es = []core.Event{}
	}
	writeJSON(w, 200, es)
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	v, err := s.repo.GetSettings(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if v == nil {
		v = map[string]string{}
	}
	if v["default_backup_dir"] == "" {
		v["default_backup_dir"] = s.options.BackupDir
	}
	writeJSON(w, 200, v)
}
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var v map[string]string
	if err := decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	for key, value := range v {
		if key != "default_backup_dir" {
			fail(w, errors.New("configuração desconhecida"))
			return
		}
		if !filepath.IsAbs(value) {
			fail(w, errors.New("diretório deve ser absoluto"))
			return
		}
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			fail(w, errors.New("diretório deve existir"))
			return
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			fail(w, err)
			return
		}
		v[key] = resolved
	}
	if err := s.repo.SaveSettings(r.Context(), v); err != nil {
		fail(w, err)
		return
	}
	s.settings(w, r)
}
func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	out, err := s.backupCatalog(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) importLegacy(w http.ResponseWriter, r *http.Request) {
	if s.options.Importer == nil {
		fail(w, errors.New("importação indisponível"))
		return
	}
	var v struct {
		Path string `json:"path"`
	}
	if err := decode(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	if !filepath.IsAbs(v.Path) {
		fail(w, errors.New("caminho SQLite deve ser absoluto"))
		return
	}
	js, err := s.ops.List(r.Context(), 100)
	if err != nil {
		fail(w, err)
		return
	}
	for _, j := range js {
		if j.Status == "running" || j.Status == "cancel_requested" {
			fail(w, core.ErrConflict)
			return
		}
	}
	imported, err := s.options.Importer.ImportLegacy(r.Context(), v.Path)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, imported)
}
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	if s.options.Assets == nil {
		http.Error(w, "Frontend não compilado. Execute scripts/build.ps1.", 503)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	file, err := s.options.Assets.Open(name)
	if err != nil {
		if strings.Contains(filepath.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		file, err = s.options.Assets.Open(name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = file.Close()
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		content, err := fs.ReadFile(s.options.Assets, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(content)
		}
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + name
	http.FileServer(http.FS(s.options.Assets)).ServeHTTP(w, r2)
}
