package httpapi

import (
	"context"
	"dumpersg/internal/core"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationRestartSecurityAndCapabilities(t *testing.T) {
	s, _ := fixture(t)
	initial := request(s, "GET", "/api/v1/application", "", "")
	var status struct {
		Instance  string `json:"instance_id"`
		Available bool   `json:"restart_available"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &status); err != nil || status.Instance == "" || status.Available {
		t.Fatalf("status: %s %v", initial.Body, err)
	}
	if strings.Contains(initial.Body.String(), s.token) {
		t.Fatal("instance exposed session token")
	}
	if w := request(s, "POST", "/api/v1/application/restart", "{}", ""); w.Code != 403 {
		t.Fatalf("missing token: %d", w.Code)
	}
	if w := request(s, "POST", "/api/v1/application/restart", "{}", s.token); w.Code != 501 {
		t.Fatalf("unmanaged process: %d", w.Code)
	}
	called := 0
	s.options.Restart = func(context.Context) error { called++; return nil }
	for _, origin := range []string{"https://evil.example"} {
		r := httptest.NewRequest("POST", "http://localhost:8787/api/v1/application/restart", strings.NewReader("{}"))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-DumperSG-Token", s.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("external restart accepted")
		}
	}
	if w := request(s, "POST", "/api/v1/application/restart", `{"command":"arbitrary"}`, s.token); w.Code != 400 {
		t.Fatal("arbitrary body accepted")
	}
	if called != 0 {
		t.Fatal("invalid request triggered restart")
	}
	if w := request(s, "POST", "/api/v1/application/restart", "{}", s.token); w.Code != 202 || !strings.Contains(w.Body.String(), status.Instance) {
		t.Fatalf("accepted restart: %d %s", w.Code, w.Body)
	}
	if called != 1 {
		t.Fatalf("callback invoked %d times", called)
	}
	s.options.Restart = func(context.Context) error { return fmt.Errorf("operação ativa: %w", core.ErrConflict) }
	if w := request(s, "POST", "/api/v1/application/restart", "{}", s.token); w.Code != 409 {
		t.Fatalf("conflict: %d", w.Code)
	}
	other, _ := fixture(t)
	if other.instance == s.instance {
		t.Fatal("instance id did not change")
	}
}

func TestApplicationShutdownIsOptInAuthenticatedAndIdle(t *testing.T) {
	s, _ := fixture(t)
	if w := request(s, "POST", "/api/v1/application/shutdown", "{}", ""); w.Code != 403 {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := request(s, "POST", "/api/v1/application/shutdown", "{}", s.token); w.Code != 501 {
		t.Fatalf("disabled: %d", w.Code)
	}
	called := 0
	s.options.Shutdown = func(context.Context) error { called++; return nil }
	if w := request(s, "POST", "/api/v1/application/shutdown", `{"command":"anything"}`, s.token); w.Code != 400 || called != 0 {
		t.Fatal("arbitrary body accepted")
	}
	if w := request(s, "POST", "/api/v1/application/shutdown", "{}", s.token); w.Code != 202 || called != 1 {
		t.Fatalf("shutdown: %d %s", w.Code, w.Body)
	}
	s.options.Shutdown = func(context.Context) error { return core.ErrConflict }
	if w := request(s, "POST", "/api/v1/application/shutdown", "{}", s.token); w.Code != 409 {
		t.Fatalf("active: %d", w.Code)
	}
}
