package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/core"
	"dumpersg/internal/jobs"
)

type trayTestExecutor struct{ release <-chan struct{} }

func (e trayTestExecutor) Run(ctx context.Context, _ core.Command, _ func(string, string)) error {
	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestTrayShutdownPreservesActiveJobAndReservesIdleExit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repo, err := sqlite.Open(filepath.Join(dir, "tray.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	profile, err := repo.SaveProfile(ctx, core.Profile{Name: "tray", Host: "localhost", Port: 3306, User: "root", Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	manager := jobs.New(repo, trayTestExecutor{release}, core.Config{LogDir: dir, MySQLImage: "mysql:test"})
	defer manager.Close(ctx)
	job, err := manager.StartTest(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested := make(chan struct{}, 1)
	if err := prepareIdleShutdown(ctx, manager, requested); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("active operation allowed exit: %v", err)
	}
	if len(requested) != 0 {
		t.Fatal("blocked shutdown signaled exit")
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		finished, err := manager.Get(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finished.FinishedAt != "" {
			if finished.Status != "succeeded" {
				t.Fatalf("blocked shutdown interrupted the operation: %+v", finished)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operation never finished")
		}
		time.Sleep(time.Millisecond)
	}
	if err := prepareIdleShutdown(ctx, manager, requested); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 1 {
		t.Fatal("idle shutdown did not signal exit")
	}
	if _, err := manager.StartTest(ctx, profile.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("new operation accepted during shutdown: %v", err)
	}
	if err := prepareIdleShutdown(ctx, manager, requested); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate exit accepted: %v", err)
	}
}
