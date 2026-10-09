package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"

	"dumpersg/internal/core"
)

func restartManager(t *testing.T) *Manager {
	t.Helper()
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, _ func(string, string)) error {
		<-ctx.Done()
		return ctx.Err()
	}), core.Config{LogDir: t.TempDir(), MySQLImage: "test"})
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

func TestRestartReservesIdleManagerAndRejectsNewJobs(t *testing.T) {
	m := restartManager(t)
	if err := m.PrepareRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("job started after restart: %v", err)
	}
	if err := m.PrepareRestart(context.Background()); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate restart: %v", err)
	}
}

func TestRestartRejectsActiveOperation(t *testing.T) {
	m := restartManager(t)
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareRestart(context.Background()); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("active job interrupted: %v", err)
	}
	if m.closed {
		t.Fatal("rejected restart closed manager")
	}
	if _, err := m.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, job.ID)
	if err := m.PrepareRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRestartPreservesPendingCleanupAndItsExecutionGuard(t *testing.T) {
	repo := newRepo()
	ctx := context.Background()
	if err := repo.SaveJob(ctx, core.Job{ID: "pending", Status: "failed", CleanupRequired: true}); err != nil {
		t.Fatal(err)
	}
	executor := executorFunc(func(context.Context, core.Command, func(string, string)) error {
		t.Fatal("pending operation guard was released")
		return nil
	})
	cfg := core.Config{LogDir: t.TempDir(), MySQLImage: "test"}
	m := New(repo, executor, cfg)
	if err := m.PrepareRestart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(ctx); !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("pending cleanup was not reported: %v", err)
	}
	job, err := repo.GetJob(ctx, "pending")
	if err != nil || !job.CleanupRequired {
		t.Fatal("persisted cleanup guard changed")
	}
	restarted := New(repo, executor, cfg)
	if _, err := restarted.StartTest(ctx, 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("new instance released pending job: %v", err)
	}
}

func TestRestartAndJobCreationAreMutuallyExclusive(t *testing.T) {
	for i := 0; i < 20; i++ {
		m := restartManager(t)
		begin := make(chan struct{})
		var restartErr, jobErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-begin; restartErr = m.PrepareRestart(context.Background()) }()
		go func() { defer wg.Done(); <-begin; _, jobErr = m.StartTest(context.Background(), 1) }()
		close(begin)
		wg.Wait()
		if (restartErr == nil) == (jobErr == nil) {
			t.Fatalf("restart=%v job=%v; only one may succeed", restartErr, jobErr)
		}
		if err := m.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
