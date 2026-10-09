package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

func backupFolder(t *testing.T, parent, name string, complete bool) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	metadata := "# Started dump at: now\n"
	if complete {
		metadata += "# Finished dump at: later\n"
	}
	if err := os.WriteFile(filepath.Join(path, "metadata"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "db.table.sql"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func catalog(t *testing.T, s *Server) []backupEntry {
	t.Helper()
	w := request(s, "GET", "/api/v1/backups", "", "")
	if w.Code != 200 {
		t.Fatalf("catalog: %d %s", w.Code, w.Body)
	}
	var entries []backupEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestBackupManagementCatalogAndDeletion(t *testing.T) {
	s, repo := fixture(t)
	ready := backupFolder(t, s.options.BackupDir, "ready", true)
	other := backupFolder(t, s.options.BackupDir, "other", false)
	unrelated := filepath.Join(s.options.BackupDir, "unrelated")
	if err := os.Mkdir(unrelated, 0700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	id := strings.Repeat("a", 32)
	partial := filepath.Join(external, "db_20261008_120000_"+id)
	if err := os.Mkdir(partial, 0700); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveJob(context.Background(), core.Job{ID: id, Kind: "backup", Status: "failed", Path: partial, StartedAt: "2026-10-08T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	entries := catalog(t, s)
	if len(entries) != 3 {
		t.Fatalf("entries: %+v", entries)
	}
	var chosen backupEntry
	for _, entry := range entries {
		if entry.Path == ready {
			chosen = entry
		}
		if entry.Path == partial && (entry.Complete || entry.FileCount != 0) {
			t.Fatalf("partial: %+v", entry)
		}
	}
	metadata, _ := os.Stat(filepath.Join(ready, "metadata"))
	if !chosen.Complete || chosen.FileCount != 2 || chosen.SizeBytes != metadata.Size()+4 || len(chosen.ID) != 64 {
		t.Fatalf("ready: %+v", chosen)
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+chosen.ID, "", ""); w.Code != 403 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+strings.Repeat("f", 64), "", s.token); w.Code != 404 {
		t.Fatalf("unknown: %d", w.Code)
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+chosen.ID, "", s.token); w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatal("backup not removed")
	}
	for _, path := range []string{other, partial, unrelated, external, s.options.BackupDir} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("removed unrelated %s: %v", path, err)
		}
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+chosen.ID, "", s.token); w.Code != 404 {
		t.Fatalf("repeat: %d", w.Code)
	}
	for _, entry := range catalog(t, s) {
		if entry.Path == partial {
			if w := request(s, "DELETE", "/api/v1/backups/"+entry.ID, "", s.token); w.Code != 204 {
				t.Fatalf("incomplete delete: %d %s", w.Code, w.Body)
			}
		}
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("partial not removed")
	}
	job, err := repo.GetJob(context.Background(), id)
	if err != nil || job.Path != partial {
		t.Fatal("history changed")
	}
}

func TestBackupDeletionRefusesActiveOperationAndOpenUsesCatalogPath(t *testing.T) {
	s, _ := fixture(t)
	path := backupFolder(t, s.options.BackupDir, "backup", true)
	entry := catalog(t, s)[0]
	opened := ""
	s.options.OpenBackupFolder = func(path string) error { opened = path; return nil }
	if w := request(s, "POST", "/api/v1/backups/"+entry.ID+"/open", "", s.token); w.Code != 204 || opened != path {
		t.Fatalf("open: %d %s path=%s", w.Code, w.Body, opened)
	}
	w := request(s, "POST", "/api/v1/profiles", `{"name":"local","host":"localhost","user":"qa","database":"db"}`, s.token)
	if w.Code != 201 {
		t.Fatalf("profile: %s", w.Body)
	}
	if w := request(s, "POST", "/api/v1/profiles/1/test", "", s.token); w.Code != 202 {
		t.Fatalf("test: %d %s", w.Code, w.Body)
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+entry.ID, "", s.token); w.Code != 409 {
		t.Fatalf("active delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("active backup removed")
	}
}

func directoryLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); err == nil {
			return
		} else {
			t.Skipf("junction unavailable: %v %s", err, out)
		}
	}
	t.Skip("directory symlinks unavailable")
}

func TestBackupDeletionDoesNotFollowDirectoryLinks(t *testing.T) {
	s, _ := fixture(t)
	path := backupFolder(t, s.options.BackupDir, "backup", true)
	external := t.TempDir()
	protected := backupFolder(t, external, "protected", true)
	directoryLink(t, protected, filepath.Join(path, "linked"))
	directoryLink(t, protected, filepath.Join(s.options.BackupDir, "alias"))
	entries := catalog(t, s)
	if len(entries) != 1 || entries[0].Path != path {
		t.Fatalf("link listed: %+v", entries)
	}
	if entries[0].FileCount != 2 {
		t.Fatalf("followed link in size: %+v", entries[0])
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+entries[0].ID, "", s.token); w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(protected, "metadata")); err != nil {
		t.Fatal("link target removed", err)
	}
}

func TestBackupDeletionRejectsStaleSelection(t *testing.T) {
	s, _ := fixture(t)
	path := backupFolder(t, s.options.BackupDir, "backup", true)
	old := catalog(t, s)[0]
	if err := os.WriteFile(filepath.Join(path, "db.table.sql"), []byte("changed contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "DELETE", "/api/v1/backups/"+old.ID, "", s.token); w.Code != 404 {
		t.Fatalf("stale delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("changed backup removed")
	}
}
