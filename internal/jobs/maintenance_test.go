package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"dumpersg/internal/core"
)

func TestMaintenanceExcludesConcurrentJobCreation(t *testing.T) {
	dir := t.TempDir()
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, _ func(string, string)) error {
		<-ctx.Done()
		return ctx.Err()
	}), core.Config{LogDir: filepath.Join(dir, "logs"), DockerImage: "test", MySQLImage: "test"})
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.WithIdleMaintenance(context.Background(), func() error { close(entered); <-release; return nil })
	}()
	<-entered
	started := make(chan error, 1)
	go func() { _, err := m.StartTest(context.Background(), 1); started <- err }()
	select {
	case err := <-started:
		close(release)
		t.Fatalf("job escaped maintenance: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	called := false
	if err := m.WithIdleMaintenance(context.Background(), func() error { called = true; return nil }); !errors.Is(err, core.ErrConflict) || called {
		t.Fatalf("active maintenance: %v called=%v", err, called)
	}
}

func TestMaintenanceRefusesUnconfirmedContainer(t *testing.T) {
	repo := newRepo()
	repo.jobs["pending"] = core.Job{ID: "pending", Kind: "backup", Status: "failed", CleanupRequired: true}
	m := New(repo, executorFunc(func(context.Context, core.Command, func(string, string)) error { return nil }), core.Config{})
	called := false
	if err := m.WithIdleMaintenance(context.Background(), func() error { called = true; return nil }); !errors.Is(err, core.ErrConflict) || called {
		t.Fatalf("pending maintenance: %v called=%v", err, called)
	}
}

func TestMaintenanceReleasesOnlyAfterReadOnlyConfirmationIsPersisted(t *testing.T) {
	repo := newRepo()
	repo.jobs["pending"] = core.Job{ID: "pending", Kind: "backup", Status: "failed", CleanupRequired: true}
	executor := &guardedExecutor{repo: repo}
	m := New(repo, executor, core.Config{})
	called := false
	action := func() error {
		job, err := repo.GetJob(context.Background(), "pending")
		if err != nil || job.CleanupRequired {
			t.Fatal("confirmation was not persisted")
		}
		called = true
		return nil
	}
	if err := m.WithIdleMaintenance(context.Background(), action); !errors.Is(err, core.ErrConflict) || called {
		t.Fatalf("unconfirmed: %v called=%v", err, called)
	}
	executor.confirm()
	if err := m.WithIdleMaintenance(context.Background(), action); err != nil || !called {
		t.Fatalf("confirmed: %v called=%v", err, called)
	}
	if executor.runs != 0 {
		t.Fatal("maintenance invoked Docker operation")
	}
}
