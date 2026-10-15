package jobs

import (
	"context"
	"dumpersg/internal/core"
	"fmt"
	"strings"
)

type backupRestorePlan struct {
	target  core.Profile
	request core.RestoreRequest
	create  core.Command
}

func localHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1", "host.docker.internal":
		return true
	}
	return false
}

func (m *Manager) startBackupRestore(ctx context.Context, req core.BackupRequest) (core.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	source, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	destination := *req.Restore
	target, _, err := m.profile(ctx, destination.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	create, err := core.BuildCreateDatabase(target, destination.TargetDatabase, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	threads := destination.Threads
	if threads == 0 {
		threads = target.Threads
	}
	if err := core.ValidateThreads(threads); err != nil {
		return core.Job{}, err
	}
	database := req.Database
	if database == "" {
		database = source.Database
	}
	if localHost(source.Host) && source.Port == target.Port && strings.EqualFold(strings.TrimSpace(database), strings.TrimSpace(destination.TargetDatabase)) {
		return core.Job{}, fmt.Errorf("banco de destino deve ser diferente do banco de origem no mesmo servidor local")
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
	cmd, path, err := core.BuildBackup(source, req, cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	plan := &backupRestorePlan{target: target, create: create, request: core.RestoreRequest{
		ProfileID: target.ID, BackupDir: path, TargetDatabase: destination.TargetDatabase,
		Threads: threads, OverwriteTables: destination.OverwriteTables,
	}}
	job := core.Job{ID: id, Kind: "backup_restore", Phase: "backup", Database: database, Path: path,
		TargetProfileID: target.ID, TargetProfileName: target.Name, TargetDatabase: destination.TargetDatabase,
		TargetHost: target.Host, TargetPort: target.Port}
	return m.startPlannedLocked(ctx, source, cmd, job, nil, plan)
}

func (m *Manager) setPhase(x *execution, phase, message string, progress int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	x.job.Phase, x.job.Message, x.job.Progress = phase, message, progress
	m.eventLocked(x, "info", message)
	return m.save(x.job)
}

func (m *Manager) runBackupRestore(ctx context.Context, x *execution) error {
	// A successful CLI alone is insufficient: require a complete, readable catalog.
	if _, err := core.ListBackupTables(x.plan.request.BackupDir); err != nil {
		return fmt.Errorf("backup não validado; restauração não iniciada: %w", err)
	}
	m.mu.Lock()
	x.job.BackupCompleted = true
	err := m.save(x.job)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	restore, _, err := core.BuildRestore(x.plan.target, x.plan.request, m.cfg, x.job.ID)
	if err != nil {
		return fmt.Errorf("preparação da restauração falhou: %w", err)
	}
	if err := m.setPhase(x, "preparing_restore", "Backup concluído. Etapa 2 de 2: preparando banco local de destino.", 50); err != nil {
		return err
	}
	create := x.plan.create
	create.DockerIdentity = x.command.DockerIdentity
	if err := m.runCommand(ctx, x, create); err != nil {
		return fmt.Errorf("preparação do banco local falhou: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.setPhase(x, "restore", "Etapa 2 de 2: restaurando o backup no banco local.", 60); err != nil {
		return err
	}
	restore.DockerIdentity = x.command.DockerIdentity
	if err := m.runCommand(ctx, x, restore); err != nil {
		return fmt.Errorf("restauração falhou: %w", err)
	}
	return nil
}
