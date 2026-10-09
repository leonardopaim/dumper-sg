package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"dumpersg/internal/core"
)

func TestBackupLocationsRemainVisibleBeyondPaginatedHistory(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "backups.db"))
	ctx := context.Background()
	if err := s.SaveJob(ctx, core.Job{ID: "old-backup", Kind: "backup", Status: "failed", Path: "custom-backup", StartedAt: "2020-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	// Fill the history without repeatedly syncing a thousand individual writes.
	_, err := s.db.ExecContext(ctx, `WITH RECURSIVE n(v) AS (VALUES(1) UNION ALL SELECT v+1 FROM n WHERE v<1001)
INSERT INTO jobs(id,started_at,payload) SELECT printf('query-%d',v),'2026-10-08T00:00:00Z',json_object('id',printf('query-%d',v),'kind','table_list','status','succeeded') FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.ListJobs(ctx, 1000)
	if err != nil || len(page) != 1000 {
		t.Fatalf("page: %d %v", len(page), err)
	}
	backups, err := s.BackupJobs(ctx)
	if err != nil || len(backups) != 1 || backups[0].ID != "old-backup" {
		t.Fatalf("backups: %+v %v", backups, err)
	}
}
