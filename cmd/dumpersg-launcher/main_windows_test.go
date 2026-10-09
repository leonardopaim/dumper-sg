//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLauncherOnlyAllowsLoopbackAndRecognizedAPI(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8787", "example.com:8787", "192.168.0.1:8787", "localhost:8787", "bad"} {
		if _, err := localBase(address); err == nil {
			t.Fatalf("remote accepted: %s", address)
		}
	}
	if _, err := localBase("127.0.0.1:8787"); err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"instance_id":"foreign"}`)) }))
	defer s.Close()
	if _, err := application(s.URL); err == nil {
		t.Fatal("foreign API accepted")
	}
}

func TestLauncherWaitDoesNotAdoptAnotherCore(t *testing.T) {
	done := make(chan error, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": strings.Repeat("a", 32)})
		done <- errors.New("child exited")
	}))
	defer s.Close()
	if _, err := waitForCore(s.URL, strings.Repeat("b", 32), done); err == nil {
		t.Fatal("adopted another process")
	}
}

func TestInstalledShutdownChecksOwnershipAndSession(t *testing.T) {
	instance := strings.Repeat("a", 32)
	posts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/application":
			if posts > 0 {
				w.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": instance})
		case "/api/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": strings.Repeat("b", 64)})
		case "/api/v1/application/shutdown":
			if r.Method != "POST" || r.Header.Get("X-DumperSG-Token") != strings.Repeat("b", 64) {
				t.Error("missing session")
			}
			posts++
			w.WriteHeader(202)
		}
	}))
	defer s.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "process.json")
	state := launchState{Instance: instance, Address: strings.TrimPrefix(s.URL, "http://"), Core: "owned.exe", DataDir: dir}
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	if err := shutdownInstalled(s.URL, path, "another.exe", dir); err == nil || posts != 0 {
		t.Fatal("other installation stopped")
	}
	state.Instance = strings.Repeat("c", 32)
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	if err := shutdownInstalled(s.URL, path, "owned.exe", dir); err == nil || posts != 0 {
		t.Fatal("changed instance stopped")
	}
	state.Instance = instance
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	if err := shutdownInstalled(s.URL, path, "owned.exe", dir); err != nil || posts != 1 {
		t.Fatalf("safe stop: %v posts=%d", err, posts)
	}
}
