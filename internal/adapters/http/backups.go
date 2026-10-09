package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"dumpersg/internal/core"
)

type backupEntry struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	SizeBytes  int64  `json:"size_bytes"`
	FileCount  int    `json:"file_count"`
	Complete   bool   `json:"complete"`
	Problem    string `json:"problem,omitempty"`
	parent     string
	info       os.FileInfo
}

var backupJobSuffix = regexp.MustCompile(`_[0-9]{8}_[0-9]{6}_[a-f0-9]{32}$`)

func (s *Server) backupCatalog(ctx context.Context) ([]backupEntry, error) {
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	base := settings["default_backup_dir"]
	if base == "" {
		base = s.options.BackupDir
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return nil, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	candidates := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			candidates[filepath.Join(base, entry.Name())] = false
		}
	}
	// Include incomplete backups and custom destinations registered by this app.
	jobs, err := s.repo.BackupJobs(ctx)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		name := filepath.Base(job.Path)
		if job.Kind != "backup" || !filepath.IsAbs(job.Path) || !backupJobSuffix.MatchString(name) || !strings.HasSuffix(name, "_"+job.ID) {
			continue
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(job.Path))
		if err != nil || !samePath(parent, filepath.Dir(job.Path)) {
			continue
		}
		candidates[filepath.Clean(job.Path)] = true
	}
	out := []backupEntry{}
	seen := map[string]bool{}
	for path, registered := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := inspectBackup(ctx, path, registered)
		if err != nil {
			continue
		} // Removed folders and unrelated directories are not backups.
		if !seen[entry.ID] {
			out = append(out, entry)
			seen[entry.ID] = true
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModifiedAt == out[j].ModifiedAt {
			return out[i].Path < out[j].Path
		}
		return out[i].ModifiedAt > out[j].ModifiedAt
	})
	return out, nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func inspectBackup(ctx context.Context, path string, registered bool) (backupEntry, error) {
	parent, name := filepath.Dir(path), filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || !filepath.IsLocal(name) {
		return backupEntry{}, core.ErrNotFound
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return backupEntry{}, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return backupEntry{}, core.ErrNotFound
	}
	folder, err := root.OpenRoot(name)
	if err != nil {
		return backupEntry{}, err
	}
	defer folder.Close()
	metadata, metaErr := folder.Lstat("metadata")
	hasMetadata := metaErr == nil && metadata.Mode().IsRegular()
	if !registered && !hasMetadata {
		return backupEntry{}, core.ErrNotFound
	}
	identity := path
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	entry := backupEntry{Path: path, Name: name, ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), parent: parent, info: info}
	if hasMetadata {
		if metadata.Size() > 8<<20 {
			entry.Problem = "Metadata excede o limite de 8 MiB."
		} else {
			f, err := folder.Open("metadata")
			if err == nil {
				data, readErr := io.ReadAll(io.LimitReader(f, (8<<20)+1))
				f.Close()
				if readErr == nil && len(data) <= 8<<20 {
					for _, line := range strings.Split(string(data), "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "# Finished dump at:") || strings.HasPrefix(line, "Finished dump at:") {
							entry.Complete = true
							break
						}
					}
				} else {
					entry.Problem = "Não foi possível ler o metadata."
				}
			} else {
				entry.Problem = "Não foi possível ler o metadata."
			}
		}
	}
	count := 0
	err = fs.WalkDir(folder.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 100000 {
			return errors.New("limite de 100.000 arquivos excedido")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			entry.SizeBytes += info.Size()
			entry.FileCount++
		}
		return nil
	})
	if err != nil {
		entry.Problem = "Tamanho parcial: " + err.Error()
	}
	// Expire stale selections when files, size or completion change.
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%d\n%d\n%t", identity, entry.ModifiedAt, entry.SizeBytes, entry.FileCount, entry.Complete)))
	entry.ID = hex.EncodeToString(digest[:])
	return entry, nil
}

func (s *Server) findBackup(ctx context.Context, id string) (backupEntry, error) {
	if len(id) != 64 {
		return backupEntry{}, core.ErrNotFound
	}
	entries, err := s.backupCatalog(ctx)
	if err != nil {
		return backupEntry{}, err
	}
	for _, entry := range entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return backupEntry{}, core.ErrNotFound
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	guard, ok := s.ops.(interface {
		WithIdleMaintenance(context.Context, func() error) error
	})
	if !ok {
		writeJSON(w, 501, map[string]string{"error": "exclusão indisponível"})
		return
	}
	err := guard.WithIdleMaintenance(r.Context(), func() error {
		entry, err := s.findBackup(r.Context(), r.PathValue("id"))
		if err != nil {
			return err
		}
		root, err := os.OpenRoot(entry.parent)
		if err != nil {
			return err
		}
		defer root.Close()
		info, err := root.Lstat(entry.Name)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, entry.info) {
			return fmt.Errorf("a pasta mudou; atualize o catálogo: %w", core.ErrConflict)
		}
		// Root confines traversal; RemoveAll removes links without following their targets.
		return root.RemoveAll(entry.Name)
	})
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) openBackup(w http.ResponseWriter, r *http.Request) {
	entry, err := s.findBackup(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	opener := s.options.OpenBackupFolder
	if opener == nil {
		opener = openBackupFolder
	}
	if err := opener(entry.Path); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func openBackupFolder(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("não foi possível abrir a pasta: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
