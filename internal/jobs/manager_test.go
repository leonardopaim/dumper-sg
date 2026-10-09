package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"dumpersg/internal/core"
)

type memoryRepo struct {
	mu      sync.Mutex
	jobs    map[string]core.Job
	profile core.Profile
}

func newRepo() *memoryRepo {
	return &memoryRepo{jobs: map[string]core.Job{}, profile: core.Profile{ID: 1, Name: "local", Host: "localhost", Port: 3306, User: "root", Password: "top-secret", Database: "production", Threads: 8}}
}
func (r *memoryRepo) ListProfiles(context.Context) ([]core.Profile, error) {
	return []core.Profile{r.profile}, nil
}
func (r *memoryRepo) GetProfile(_ context.Context, id int64) (core.Profile, error) {
	if id != 1 {
		return core.Profile{}, core.ErrNotFound
	}
	return r.profile, nil
}
func (r *memoryRepo) SaveProfile(_ context.Context, p core.Profile) (core.Profile, error) {
	return p, nil
}
func (r *memoryRepo) DeleteProfile(context.Context, int64) error { return nil }
func (r *memoryRepo) GetSettings(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *memoryRepo) SaveSettings(context.Context, map[string]string) error { return nil }
func (r *memoryRepo) SaveJob(_ context.Context, j core.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[j.ID] = j
	return nil
}
func (r *memoryRepo) GetJob(_ context.Context, id string) (core.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return j, core.ErrNotFound
	}
	return j, nil
}
func (r *memoryRepo) ListJobs(_ context.Context, limit int) ([]core.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]core.Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		result = append(result, j)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt > result[j].StartedAt })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *memoryRepo) BackupJobs(ctx context.Context) ([]core.Job, error) {
	jobs, err := r.ListJobs(ctx, 0)
	result := []core.Job{}
	for _, job := range jobs {
		if job.Kind == "backup" {
			result = append(result, job)
		}
	}
	return result, err
}

func (r *memoryRepo) PendingJobs(context.Context) ([]core.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []core.Job{}
	for _, job := range r.jobs {
		if job.CleanupRequired || job.Status == "running" || job.Status == "cancel_requested" {
			result = append(result, job)
		}
	}
	return result, nil
}

type executorFunc func(context.Context, core.Command, func(string, string)) error

func (f executorFunc) Run(ctx context.Context, cmd core.Command, log func(string, string)) error {
	return f(ctx, cmd, log)
}

type identifiedExecutor struct{ identity string }

func (e identifiedExecutor) ExecutionIdentity(context.Context) (string, error) {
	return e.identity, nil
}
func (e identifiedExecutor) Run(_ context.Context, cmd core.Command, _ func(string, string)) error {
	if cmd.DockerIdentity != e.identity {
		return fmt.Errorf("command did not carry daemon identity")
	}
	return nil
}
func (e identifiedExecutor) VerifyTermination(_ context.Context, cmd core.Command) error {
	if cmd.DockerIdentity != e.identity {
		return core.ErrTerminationUnconfirmed
	}
	return nil
}

func TestDaemonIdentityPersistsAndPreventsRecoveryOnAnotherDaemon(t *testing.T) {
	repo := newRepo()
	m := New(repo, identifiedExecutor{"WSL:Ubuntu:daemon-one"}, core.Config{LogDir: t.TempDir()})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "succeeded" || final.DockerIdentity != "WSL:Ubuntu:daemon-one" {
		t.Fatalf("identity lost: %#v", final)
	}
	final.Status, final.CleanupRequired = "failed", true
	if err := repo.SaveJob(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	restarted := New(repo, identifiedExecutor{"WSL:Ubuntu:daemon-two"}, core.Config{LogDir: t.TempDir()})
	if _, err := restarted.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("different daemon released guard: %v", err)
	}
	stored, err := repo.GetJob(context.Background(), final.ID)
	if err != nil || !stored.CleanupRequired {
		t.Fatal("guard was cleared")
	}
	same := New(repo, identifiedExecutor{"WSL:Ubuntu:daemon-one"}, core.Config{LogDir: t.TempDir()})
	job, err = same.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, same, job.ID)
}

func waitFinal(t *testing.T, m *Manager, id string) core.Job {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		job, err := m.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.FinishedAt != "" {
			return job
		}
		select {
		case <-deadline:
			t.Fatal("job never finished")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestExclusiveExecutionAndIndependentContext(t *testing.T) {
	repo := newRepo()
	entered := make(chan struct{})
	release := make(chan struct{})
	m := New(repo, executorFunc(func(ctx context.Context, _ core.Command, _ func(string, string)) error {
		close(entered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}), core.Config{LogDir: t.TempDir()})
	requestCtx, cancel := context.WithCancel(context.Background())
	job, err := m.StartTest(requestCtx, 1)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	<-entered
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
				t.Errorf("expected conflict: %v", err)
			}
		}()
	}
	wg.Wait()
	if current, _ := m.Get(context.Background(), job.ID); current.Status != "running" {
		t.Fatal("request cancellation stopped job")
	}
	close(release)
	if final := waitFinal(t, m, job.ID); final.Status != "succeeded" || final.Progress != 100 {
		t.Fatalf("unexpected result: %+v", final)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancelWaitsForConfirmedTerminationAndIsIdempotent(t *testing.T) {
	entered := make(chan struct{})
	cleanup := make(chan struct{})
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, _ func(string, string)) error {
		close(entered)
		<-ctx.Done()
		<-cleanup
		return context.Canceled
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	for range 3 {
		j, err := m.Cancel(context.Background(), job.ID)
		if err != nil || j.Status != "cancel_requested" {
			t.Fatalf("cancel result: %+v %v", j, err)
		}
	}
	if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatal("slot freed before termination")
	}
	close(cleanup)
	final := waitFinal(t, m, job.ID)
	if final.Status != "cancelled" {
		t.Fatalf("unexpected result %+v", final)
	}
	if final, err = m.Cancel(context.Background(), job.ID); err != nil || final.Status != "cancelled" {
		t.Fatal("final cancel is not idempotent")
	}
	if _, err = m.Cancel(context.Background(), "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatal("missing job error lost")
	}
}

func TestCleanupFailureIsNotReportedCancelled(t *testing.T) {
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, _ func(string, string)) error {
		<-ctx.Done()
		return errors.New("cleanup not confirmed top-secret")
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "failed" || strings.Contains(final.Message, "top-secret") {
		t.Fatalf("unexpected result %+v", final)
	}
}

func TestBoundedEventsRedactedLogsAndPersistence(t *testing.T) {
	dir := t.TempDir()
	repo := newRepo()
	m := New(repo, executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error {
		for range 1200 {
			log("info", "password top-secret "+strings.Repeat("x", 10000))
		}
		return errors.New("failed with top-secret")
	}), core.Config{LogDir: dir})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "failed" || strings.Contains(final.Message, "top-secret") {
		t.Fatal("secret in final result")
	}
	events, err := m.Events(context.Background(), job.ID, 0)
	if err != nil || len(events) != maxEvents {
		t.Fatalf("event retention: %d %v", len(events), err)
	}
	for _, event := range events {
		if strings.Contains(event.Message, "top-secret") || len(event.Message) > maxMessage+32 {
			t.Fatal("event leaked or exceeded limit")
		}
	}
	after, err := m.Events(context.Background(), job.ID, events[len(events)-1].Sequence)
	if err != nil || len(after) != 0 {
		t.Fatal("incremental events failed")
	}
	data, err := os.ReadFile(filepath.Join(dir, job.ID+".log"))
	if err != nil || strings.Contains(string(data), "top-secret") || len(data) > maxLogBytes {
		t.Fatal("log leaked or exceeded limit")
	}
	persisted, err := repo.GetJob(context.Background(), job.ID)
	if err != nil || persisted.Status != "failed" || persisted.FinishedAt == "" {
		t.Fatal("result not persisted")
	}
}

func TestReconcilesInterruptedJobsWithoutExecution(t *testing.T) {
	repo := newRepo()
	repo.jobs["old"] = core.Job{ID: "old", Status: "running"}
	m := New(repo, executorFunc(func(context.Context, core.Command, func(string, string)) error { t.Fatal("reexecuted job"); return nil }), core.Config{LogDir: t.TempDir()})
	job, err := m.Get(context.Background(), "old")
	if err != nil || job.Status != "failed" || job.FinishedAt == "" || !job.CleanupRequired {
		t.Fatalf("reconciliation failed %+v %v", job, err)
	}
	events, err := m.Events(context.Background(), "old", 0)
	if err != nil || len(events) != 0 {
		t.Fatal("historical event response failed")
	}
}

type guardedExecutor struct {
	mu        sync.Mutex
	confirmed bool
	runs      int
	repo      *memoryRepo
}

func (e *guardedExecutor) Run(context.Context, core.Command, func(string, string)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runs++
	if e.runs == 1 {
		return fmt.Errorf("cleanup failed: %w", core.ErrTerminationUnconfirmed)
	}
	pending, err := e.repo.PendingJobs(context.Background())
	if err != nil {
		return err
	}
	for _, job := range pending {
		if job.CleanupRequired {
			return errors.New("new execution began before confirmation persisted")
		}
	}
	return nil
}

func (e *guardedExecutor) VerifyTermination(_ context.Context, cmd core.Command) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cmd.Program != "docker" || cmd.ContainerName == "" || len(cmd.Args) > 0 || len(cmd.Secrets) > 0 {
		return errors.New("guard retained sensitive execution arguments")
	}
	if !e.confirmed {
		return core.ErrTerminationUnconfirmed
	}
	return nil
}

func (e *guardedExecutor) confirm() { e.mu.Lock(); e.confirmed = true; e.mu.Unlock() }

func TestUnconfirmedTerminationBlocksThenVerifiedRecoveryAllowsNextJob(t *testing.T) {
	repo := newRepo()
	executor := &guardedExecutor{repo: repo}
	m := New(repo, executor, core.Config{LogDir: t.TempDir()})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "failed" || !final.CleanupRequired || !strings.Contains(final.Message, "bloqueadas") {
		t.Fatalf("missing quarantine: %+v", final)
	}
	if _, err = m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("started concurrent operation: %v", err)
	}
	if _, err = m.Cancel(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("failed verification accepted: %v", err)
	}
	executor.confirm()
	next, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if final = waitFinal(t, m, next.ID); final.Status != "succeeded" {
		t.Fatalf("new operation failed: %+v", final)
	}
	old, err := repo.GetJob(context.Background(), job.ID)
	if err != nil || old.CleanupRequired || old.Status != "failed" {
		t.Fatalf("confirmation not persisted: %+v %v", old, err)
	}
}

func TestCloseDoesNotRewriteFinishedQuarantine(t *testing.T) {
	repo := newRepo()
	executor := &guardedExecutor{repo: repo}
	m := New(repo, executor, core.Config{LogDir: t.TempDir()})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, job.ID)
	if err = m.Close(context.Background()); !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("close falsely confirmed termination: %v", err)
	}
	final, err := m.Get(context.Background(), job.ID)
	if err != nil || final.Status != "failed" || !final.CleanupRequired {
		t.Fatalf("close changed completed job: %+v %v", final, err)
	}
}

func TestRestartRecoversPendingGuardBeyondHistoryLimit(t *testing.T) {
	repo := newRepo()
	repo.jobs["old-guard"] = core.Job{ID: "old-guard", Status: "failed", CleanupRequired: true, StartedAt: "2020", FinishedAt: "2020"}
	repo.jobs["second-old-guard"] = core.Job{ID: "second-old-guard", Status: "failed", CleanupRequired: true, StartedAt: "2020", FinishedAt: "2020"}
	for i := range 150 {
		id := fmt.Sprintf("new-history-%03d", i)
		repo.jobs[id] = core.Job{ID: id, Status: "succeeded", StartedAt: "2026", FinishedAt: "2026"}
	}
	executor := &guardedExecutor{repo: repo, runs: 1}
	m := New(repo, executor, core.Config{LogDir: t.TempDir()})
	if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("restart lost guard: %v", err)
	}
	executor.confirm()
	job, err := m.Cancel(context.Background(), "old-guard")
	if err != nil || job.CleanupRequired || job.Status != "failed" {
		t.Fatalf("cancel reconciliation failed: %+v %v", job, err)
	}
	next, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if final := waitFinal(t, m, next.ID); final.Status != "succeeded" {
		t.Fatalf("recovered operation failed: %+v", final)
	}
}

type confirmationFailRepo struct {
	*memoryRepo
	rejectConfirmation bool
}

func (r *confirmationFailRepo) SaveJob(ctx context.Context, job core.Job) error {
	previous, _ := r.memoryRepo.GetJob(ctx, job.ID)
	if r.rejectConfirmation && previous.CleanupRequired && !job.CleanupRequired {
		return errors.New("cannot persist verification")
	}
	return r.memoryRepo.SaveJob(ctx, job)
}

func TestConfirmationMustPersistBeforeReleasingGuard(t *testing.T) {
	base := newRepo()
	repo := &confirmationFailRepo{memoryRepo: base, rejectConfirmation: true}
	executor := &guardedExecutor{repo: base}
	m := New(repo, executor, core.Config{LogDir: t.TempDir()})
	first, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, first.ID)
	executor.confirm()
	if _, err = m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("unpersisted confirmation released guard: %v", err)
	}
	old, err := repo.GetJob(context.Background(), first.ID)
	if err != nil || !old.CleanupRequired {
		t.Fatal("guard cleared despite persistence failure")
	}
	repo.rejectConfirmation = false
	second, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if final := waitFinal(t, m, second.ID); final.Status != "succeeded" {
		t.Fatalf("recovered operation failed: %+v", final)
	}
}

func TestOrdinaryFailureReleasesCapacity(t *testing.T) {
	m := New(newRepo(), executorFunc(func(context.Context, core.Command, func(string, string)) error {
		return errors.New("ordinary exit failure")
	}), core.Config{LogDir: t.TempDir()})
	first, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, first.ID)
	second, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatalf("ordinary failure quarantined: %v", err)
	}
	waitFinal(t, m, second.ID)
}
