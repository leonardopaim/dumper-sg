package httpapi

import (
	"context"
	"dumpersg/internal/core"
	"strings"
	"testing"
)

func TestEnvironmentReturnsSnapshotWithoutStartingJob(t *testing.T) {
	s, repo := fixture(t)
	s.options.Environment = func(context.Context) (core.EnvironmentReport, error) {
		return core.EnvironmentReport{APIAddress: "127.0.0.1:8787", Docker: core.DockerEnvironment{Runtime: "wsl", Distribution: "Ubuntu", DaemonID: "selected"}}, nil
	}
	w := request(s, "GET", "/api/v1/environment", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ubuntu") || !strings.Contains(w.Body.String(), "selected") {
		t.Fatalf("snapshot: %d %s", w.Code, w.Body.String())
	}
	jobs, _ := repo.ListJobs(context.Background(), 10)
	if len(jobs) != 0 {
		t.Fatal("diagnostic started job")
	}
}
