package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dumpersg/internal/core"
)

func TestCachedImageDoesNotPullAndMissingImagePullsBeforeCreate(t *testing.T) {
	for _, mode := range []string{"image-cached", "image-missing", "pull-fail"} {
		t.Run(mode, func(t *testing.T) {
			e, dir := helperExecutor(t, mode)
			cmd := helperCommand()
			cmd.Image = "fake-image"
			var logs []string
			err := e.Run(context.Background(), cmd, func(_, message string) { logs = append(logs, message) })
			trace, _ := os.ReadFile(filepath.Join(dir, "trace"))
			if mode == "pull-fail" {
				if err == nil || strings.Contains(string(trace), "create") || errors.Is(err, core.ErrTerminationUnconfirmed) {
					t.Fatalf("failed pull: %v %s", err, trace)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "image-cached" && strings.Contains(string(trace), "pull") {
				t.Fatal("cached image downloaded")
			}
			if mode == "image-missing" && (!strings.HasPrefix(string(trace), "image\npull\ncreate\n") || !strings.Contains(strings.Join(logs, "\n"), "Primeiro uso")) {
				t.Fatalf("pull order: %s logs=%v", trace, logs)
			}
		})
	}
}

func TestCancelledImagePullNeverCreatesOrCleansAContainer(t *testing.T) {
	e, dir := helperExecutor(t, "pull-wait")
	cmd := helperCommand()
	cmd.Image = "fake-image"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, cmd, nil) }()
	awaitFile(t, filepath.Join(dir, "pulling"))
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pull did not cancel")
	}
	trace, _ := os.ReadFile(filepath.Join(dir, "trace"))
	if string(trace) != "image\npull\n" {
		t.Fatalf("container controls used after pull cancellation: %s", trace)
	}
}
