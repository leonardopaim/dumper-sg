package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dumpersg/internal/core"
)

const maxEvents = 500
const maxMessage = 8192
const maxLogBytes = 8 << 20
const maxRetainedJobs = 100

type execution struct {
	job      core.Job
	command  core.Command
	secrets  []string
	events   []core.Event
	sequence int64
	cancel   context.CancelFunc
	done     chan struct{}
	file     *os.File
	logBytes int
	err      error
	catalog  resultCatalog
}

type Manager struct {
	mu          sync.Mutex
	repo        core.Repository
	executor    core.Executor
	cfg         core.Config
	active      *execution
	quarantined map[string]*execution
	retained    map[string]*execution
	order       []string
	closed      bool
	initErr     error
}

func New(repo core.Repository, executor core.Executor, cfg core.Config) *Manager {
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	m := &Manager{repo: repo, executor: executor, cfg: cfg, retained: make(map[string]*execution), quarantined: make(map[string]*execution)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	previous, err := repo.PendingJobs(ctx)
	if err != nil {
		m.initErr = err
		return m
	}
	for _, job := range previous {
		if job.Status == "running" || job.Status == "cancel_requested" {
			job.Status, job.Message, job.FinishedAt = "failed", "Operação interrompida pelo encerramento anterior; verifique o container antes de repetir.", timestamp()
			job.CleanupRequired = true
			if err := repo.SaveJob(ctx, job); err != nil {
				m.initErr = err
				return m
			}
		}
		if job.CleanupRequired {
			x := &execution{job: job, command: core.Command{Program: "docker", ContainerName: "dumpersg-" + job.ID, DockerIdentity: job.DockerIdentity}, err: core.ErrTerminationUnconfirmed}
			m.quarantined[job.ID] = x
		}
	}
	return m
}

func identifier() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (m *Manager) profile(ctx context.Context, id int64) (core.Profile, string, error) {
	if id <= 0 {
		return core.Profile{}, "", fmt.Errorf("profile_id é obrigatório")
	}
	p, err := m.repo.GetProfile(ctx, id)
	if err != nil {
		return p, "", err
	}
	jobID, err := identifier()
	return p, jobID, err
}

func (m *Manager) StartBackup(ctx context.Context, req core.BackupRequest) (core.Job, error) {
	p, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	cfg := m.cfg
	if req.DestinationDir == "" {
		settings, err := m.repo.GetSettings(ctx)
		if err != nil {
			return core.Job{}, err
		}
		if settings["default_backup_dir"] != "" {
			cfg.BackupDir = settings["default_backup_dir"]
		}
	}
	cmd, path, err := core.BuildBackup(p, req, cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	database := req.Database
	if database == "" {
		database = p.Database
	}
	return m.start(ctx, p, cmd, core.Job{ID: id, Kind: "backup", Database: database, Path: path})
}

func (m *Manager) StartRestore(ctx context.Context, req core.RestoreRequest) (core.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	cmd, path, err := core.BuildRestore(p, req, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	return m.startCollectedLocked(ctx, p, cmd, core.Job{ID: id, Kind: "restore", Database: req.TargetDatabase, Path: path}, nil)
}

func (m *Manager) StartTest(ctx context.Context, profileID int64) (core.Job, error) {
	p, id, err := m.profile(ctx, profileID)
	if err != nil {
		return core.Job{}, err
	}
	cmd, err := core.BuildTest(p, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	return m.start(ctx, p, cmd, core.Job{ID: id, Kind: "connection_test"})
}

func (m *Manager) StartCreateDatabase(ctx context.Context, req core.DatabaseRequest) (core.Job, error) {
	p, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	cmd, err := core.BuildCreateDatabase(p, req.Database, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	return m.start(ctx, p, cmd, core.Job{ID: id, Kind: "create_database", Database: req.Database})
}

func (m *Manager) start(ctx context.Context, p core.Profile, cmd core.Command, job core.Job) (core.Job, error) {
	return m.startCollected(ctx, p, cmd, job, nil)
}

func (m *Manager) startCollected(ctx context.Context, p core.Profile, cmd core.Command, job core.Job, catalog resultCatalog) (core.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCollectedLocked(ctx, p, cmd, job, catalog)
}

func (m *Manager) startCollectedLocked(ctx context.Context, p core.Profile, cmd core.Command, job core.Job, catalog resultCatalog) (core.Job, error) {
	if m.initErr != nil {
		return core.Job{}, m.initErr
	}
	if m.closed {
		return core.Job{}, fmt.Errorf("aplicação encerrando ou reiniciando: %w", core.ErrConflict)
	}
	if m.active != nil {
		return core.Job{}, fmt.Errorf("já existe uma operação em execução: %w", core.ErrConflict)
	}
	if err := m.reconcileLocked(ctx); err != nil {
		return core.Job{}, fmt.Errorf("novas operações bloqueadas: %v: %w", err, core.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return core.Job{}, err
	}
	if provider, ok := m.executor.(core.ExecutionIdentityProvider); ok {
		identity, err := provider.ExecutionIdentity(ctx)
		if err != nil {
			return core.Job{}, err
		}
		job.DockerIdentity, cmd.DockerIdentity = identity, identity
	}
	if err := os.MkdirAll(m.cfg.LogDir, 0700); err != nil {
		return core.Job{}, err
	}
	f, err := os.OpenFile(filepath.Join(m.cfg.LogDir, job.ID+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return core.Job{}, err
	}
	job.Status, job.StartedAt, job.Message = "running", timestamp(), "Operação iniciada; progresso estimado."
	if catalog != nil {
		job.Message = "Consultando tabelas e views..."
		if job.Kind == "database_list" {
			job.Message = "Consultando bancos disponíveis..."
		}
	}
	job.ProfileID, job.ProfileName = p.ID, p.Name
	if err := m.repo.SaveJob(ctx, job); err != nil {
		f.Close()
		os.Remove(f.Name())
		return core.Job{}, err
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	x := &execution{job: job, command: core.Command{Program: cmd.Program, ContainerName: cmd.ContainerName, DockerIdentity: cmd.DockerIdentity}, secrets: append([]string(nil), cmd.Secrets...), cancel: cancel, done: make(chan struct{}), file: f, catalog: catalog}
	m.active, m.retained[job.ID] = x, x
	m.order = append(m.order, job.ID)
	for len(m.order) > maxRetainedJobs {
		delete(m.retained, m.order[0])
		m.order = m.order[1:]
	}
	m.eventLocked(x, "info", job.Message)
	go m.run(jobCtx, x, cmd)
	return job, nil
}

func redact(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	text = strings.ReplaceAll(text, "\x00", "")
	if len(text) > maxMessage {
		text = text[:maxMessage] + " [truncado]"
	}
	return strings.ToValidUTF8(text, "�")
}

func (m *Manager) eventLocked(x *execution, level, message string) {
	message = redact(message, x.secrets)
	if strings.EqualFold(level, "warning") || strings.EqualFold(level, "warn") {
		x.job.WarningCount++
		x.job.WarningMessage = message
	}
	x.sequence++
	event := core.Event{Sequence: x.sequence, Time: timestamp(), Level: level, Message: message}
	x.events = append(x.events, event)
	if len(x.events) > maxEvents {
		copy(x.events, x.events[len(x.events)-maxEvents:])
		x.events = x.events[:maxEvents]
	}
	line := fmt.Sprintf("%d %s %s %s\r\n", event.Sequence, event.Time, level, message)
	if x.file != nil && x.logBytes+len(line) <= maxLogBytes {
		if n, err := x.file.WriteString(line); err == nil {
			x.logBytes += n
		} else {
			x.file.Close()
			x.file = nil
		}
	}
}

func (m *Manager) run(ctx context.Context, x *execution, cmd core.Command) {
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				if x.job.Status == "running" && x.job.Progress < 95 {
					x.job.Progress++
					if err := m.save(x.job); err != nil {
						m.eventLocked(x, "error", "Falha ao salvar progresso: "+err.Error())
					}
				}
				m.mu.Unlock()
			}
		}
	}()
	err := m.executor.Run(ctx, cmd, func(level, text string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.active == x {
			if level == "data" {
				if x.catalog != nil {
					x.catalog.consume(text)
				}
			} else {
				if strings.HasPrefix(text, "Primeiro uso: baixando imagem ") || strings.HasPrefix(text, "Imagem disponível: ") {
					x.job.Message = text
					if err := m.save(x.job); err != nil {
						m.eventLocked(x, "error", "Falha ao salvar preparação: "+err.Error())
					}
				}
				m.eventLocked(x, level, text)
			}
		}
	})
	if err == nil && ctx.Err() == nil && x.catalog != nil {
		m.mu.Lock()
		err = x.catalog.failure()
		m.mu.Unlock()
		if optional, ok := x.catalog.(optionalCatalog); err == nil && ok {
			err = m.runOptionalCatalog(ctx, x, cmd, optional)
		}
	}
	close(progressDone)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil && ctx.Err() == nil && x.catalog != nil {
		err = x.catalog.failure()
	}
	x.err = err
	x.job.FinishedAt = timestamp()
	switch {
	case errors.Is(err, core.ErrTerminationUnconfirmed):
		x.err = fmt.Errorf("container %s: %w", cmd.ContainerName, err)
		x.job.Status, x.job.Message = "failed", "Novas operações bloqueadas até confirmar o término do container "+cmd.ContainerName+". "+redact(err.Error(), x.secrets)
		x.job.CleanupRequired = true
		m.quarantined[x.job.ID] = x
	case errors.Is(err, context.Canceled) || (err == nil && ctx.Err() != nil):
		x.job.Status, x.job.Message = "cancelled", "Operação cancelada; término externo confirmado."
	case err != nil:
		x.job.Status, x.job.Message = "failed", redact(err.Error(), x.secrets)
		var exit interface{ ExitCode() int }
		if errors.As(err, &exit) {
			n := exit.ExitCode()
			x.job.ExitCode = &n
		}
	default:
		x.job.Status, x.job.Message, x.job.Progress = "succeeded", "Operação concluída.", 100
		if x.catalog != nil {
			x.job.Message = x.catalog.finish()
			if catalog, ok := x.catalog.(*databaseCatalog); ok {
				x.job.PartialResult = catalog.result.Warning != ""
			}
		}
		n := 0
		x.job.ExitCode = &n
	}
	if saveErr := m.save(x.job); saveErr != nil {
		x.err = errors.Join(x.err, fmt.Errorf("falha ao persistir resultado: %w", saveErr))
		x.job.Status, x.job.Message = "failed", redact(x.err.Error(), x.secrets)
		if x.job.CleanupRequired {
			x.job.Message = "Novas operações bloqueadas até confirmar o término do container. " + x.job.Message
		}
	}
	if x.catalog != nil {
		x.catalog.release(x.job.Status == "succeeded")
	}
	level := "info"
	if x.job.Status == "failed" {
		level = "error"
	}
	m.eventLocked(x, level, x.job.Message)
	if x.file != nil {
		x.file.Close()
		x.file = nil
	}
	x.secrets = nil
	x.cancel()
	m.active = nil
	close(x.done)
}

// A completed CLI does not release capacity when external termination is unknown.
// Recovery verifies every outstanding container and persists that confirmation
// before another operation can start, including after an application restart.
func (m *Manager) reconcileLocked(ctx context.Context) error {
	if len(m.quarantined) == 0 {
		return nil
	}
	verifier, ok := m.executor.(core.TerminationVerifier)
	if !ok {
		for _, x := range m.quarantined {
			return fmt.Errorf("container %s: %w", x.command.ContainerName, core.ErrTerminationUnconfirmed)
		}
	}
	for id, x := range m.quarantined {
		verifyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := verifier.VerifyTermination(verifyCtx, x.command)
		cancel()
		if err != nil {
			return fmt.Errorf("container %s: %w", x.command.ContainerName, err)
		}
		updated := x.job
		updated.CleanupRequired = false
		updated.Message = "Término do container confirmado. Operação anterior falhou."
		if err := m.repo.SaveJob(ctx, updated); err != nil {
			return fmt.Errorf("não foi possível persistir confirmação de término: %w", err)
		}
		x.job = updated
		delete(m.quarantined, id)
		if retained := m.retained[id]; retained != nil {
			retained.job = updated
		}
	}
	return nil
}

func (m *Manager) save(job core.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.repo.SaveJob(ctx, job)
}

func (m *Manager) Get(ctx context.Context, id string) (core.Job, error) {
	m.mu.Lock()
	if x := m.retained[id]; x != nil {
		job := x.job
		m.mu.Unlock()
		return job, nil
	}
	m.mu.Unlock()
	return m.repo.GetJob(ctx, id)
}

func (m *Manager) List(ctx context.Context, limit int) ([]core.Job, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs, err := m.repo.ListJobs(ctx, limit)
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []core.Job{}
	}
	for i := range jobs {
		if x := m.retained[jobs[i].ID]; x != nil {
			jobs[i] = x.job
		}
	}
	return jobs, nil
}

func (m *Manager) Events(ctx context.Context, id string, after int64) ([]core.Event, error) {
	if after < 0 {
		return nil, fmt.Errorf("after deve ser positivo")
	}
	m.mu.Lock()
	if x := m.retained[id]; x != nil {
		events := make([]core.Event, 0)
		for _, event := range x.events {
			if event.Sequence > after {
				events = append(events, event)
			}
		}
		m.mu.Unlock()
		return events, nil
	}
	m.mu.Unlock()
	if _, err := m.repo.GetJob(ctx, id); err != nil {
		return nil, err
	}
	return []core.Event{}, nil
}

func (m *Manager) Cancel(ctx context.Context, id string) (core.Job, error) {
	m.mu.Lock()
	if x := m.quarantined[id]; x != nil {
		err := m.reconcileLocked(ctx)
		job := x.job
		m.mu.Unlock()
		if err != nil {
			return job, fmt.Errorf("novas operações bloqueadas: %v: %w", err, core.ErrConflict)
		}
		return job, nil
	}
	if x := m.active; x != nil && x.job.ID == id {
		if x.job.Status != "cancel_requested" {
			updated := x.job
			updated.Status, updated.Message = "cancel_requested", "Cancelamento solicitado; aguardando término externo."
			if err := m.repo.SaveJob(ctx, updated); err != nil {
				m.mu.Unlock()
				return core.Job{}, err
			}
			x.job = updated
			m.eventLocked(x, "info", updated.Message)
			x.cancel()
		}
		job := x.job
		m.mu.Unlock()
		return job, nil
	}
	m.mu.Unlock()
	return m.Get(ctx, id)
}

// WithIdleMaintenance serializes filesystem maintenance with operation creation.
// Unconfirmed containers may still have a backup mounted, so they also block it.
func (m *Manager) WithIdleMaintenance(ctx context.Context, action func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.initErr != nil {
		return m.initErr
	}
	if m.closed {
		return fmt.Errorf("aplicação encerrando ou reiniciando: %w", core.ErrConflict)
	}
	if m.active != nil {
		return fmt.Errorf("aguarde a operação em execução antes de excluir backups: %w", core.ErrConflict)
	}
	if err := m.reconcileLocked(ctx); err != nil {
		return fmt.Errorf("exclusão bloqueada: %v: %w", err, core.ErrConflict)
	}
	return action()
}

// Reserve shutdown under the same lock as job creation: no new operation may
// start between the idle check and a requested application restart.
func (m *Manager) PrepareRestart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.initErr != nil {
		return m.initErr
	}
	if m.closed {
		return fmt.Errorf("reinício já solicitado: %w", core.ErrConflict)
	}
	if m.active != nil {
		return fmt.Errorf("conclua ou cancele a operação em andamento antes de reiniciar: %w", core.ErrConflict)
	}
	m.closed = true
	return nil
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	x := m.active
	if x != nil {
		x.job.Status, x.job.Message = "cancel_requested", "Encerramento solicitado; aguardando término externo."
		if err := m.save(x.job); err != nil {
			m.eventLocked(x, "error", "Falha ao salvar encerramento: "+err.Error())
		}
		x.cancel()
	}
	m.mu.Unlock()
	if x == nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(m.quarantined) > 0 {
			for _, pending := range m.quarantined {
				return fmt.Errorf("container %s: %w", pending.command.ContainerName, core.ErrTerminationUnconfirmed)
			}
		}
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-x.done:
		m.mu.Lock()
		defer m.mu.Unlock()
		if x.job.Status == "failed" {
			return x.err
		}
		return nil
	}
}
