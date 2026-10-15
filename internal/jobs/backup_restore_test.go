package jobs

import (
	"context"
	"dumpersg/internal/core"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type chainRepo struct {
	*memoryRepo
	target core.Profile
}

func (r *chainRepo) GetProfile(ctx context.Context, id int64) (core.Profile, error) {
	if id == 2 {
		return r.target, nil
	}
	return r.memoryRepo.GetProfile(ctx, id)
}
func chainFixture() (*chainRepo, core.BackupRequest) {
	repo := &chainRepo{memoryRepo: newRepo(), target: core.Profile{ID: 2, Name: "Destino", Host: "127.0.0.1", Port: 3306, User: "qa", Password: "target-secret", Database: "mysql", Threads: 4}}
	repo.profile.Host, repo.profile.Threads = "db.sommusgestor.com", 2
	return repo, core.BackupRequest{ProfileID: 1, Database: "production", Restore: &core.RestoreDestination{ProfileID: 2, TargetDatabase: "isolated", OverwriteTables: true}}
}
func writeChainDump(cmd core.Command, complete bool) error {
	var parent, name string
	for _, arg := range cmd.Args {
		if strings.HasSuffix(arg, ":/backup") {
			parent = strings.TrimSuffix(arg, ":/backup")
		}
		if strings.HasPrefix(arg, "--outputdir=/backup/") {
			name = strings.TrimPrefix(arg, "--outputdir=/backup/")
		}
	}
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	metadata := "# Started dump at: now\n[config]\nquote-character=BACKTICK\n[`production`.`sample`]\nreal_table_name=sample\nrows=1\n[`production`]\nschema_checksum=a\n"
	if complete {
		metadata += "# Finished dump at: now\n"
	}
	for file, data := range map[string]string{"metadata": metadata, "production-schema-create.sql": "CREATE DATABASE `production`;", "production.sample-schema.sql": "CREATE TABLE sample (id INT);", "production.sample.00000.sql": "INSERT INTO sample VALUES (1);"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0600); err != nil {
			return err
		}
	}
	return nil
}
func TestBackupRestoreRunsSequentiallyAndPreservesBothSecrets(t *testing.T) {
	repo, req := chainFixture()
	var commands []core.Command
	manager := New(repo, executorFunc(func(_ context.Context, cmd core.Command, emit func(string, string)) error {
		commands = append(commands, cmd)
		emit("info", "top-secret target-secret")
		if len(commands) == 1 {
			return writeChainDump(cmd, true)
		}
		return nil
	}), core.Config{BackupDir: t.TempDir(), LogDir: t.TempDir()})
	job, err := manager.StartBackup(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, manager, job.ID)
	if final.Status != "succeeded" || final.Kind != "backup_restore" || !final.BackupCompleted || final.TargetDatabase != "isolated" || len(commands) != 3 {
		t.Fatalf("chain result: %+v / %d", final, len(commands))
	}
	if !strings.Contains(strings.Join(commands[2].Args, " "), "--threads=4") || !strings.Contains(strings.Join(commands[2].Args, " "), "--drop-table=DROP") {
		t.Fatal("restore did not inherit destination options")
	}
	for _, cmd := range commands {
		if cmd.ContainerName != commands[0].ContainerName {
			t.Fatal("recovery cannot identify stage container")
		}
	}
	events, _ := manager.Events(context.Background(), job.ID, 0)
	for _, event := range events {
		if strings.Contains(event.Message, "top-secret") || strings.Contains(event.Message, "target-secret") {
			t.Fatal("stage credentials leaked")
		}
	}
	if _, err := core.ListBackupTables(final.Path); err != nil {
		t.Fatal("backup was not preserved", err)
	}
}
func TestBackupRestoreStopsOnFailedOrIncompleteDump(t *testing.T) {
	for _, stageErr := range []error{errors.New("dump failed"), nil} {
		t.Run(fmtName(stageErr), func(t *testing.T) {
			repo, req := chainFixture()
			calls := 0
			manager := New(repo, executorFunc(func(_ context.Context, cmd core.Command, _ func(string, string)) error {
				calls++
				if stageErr != nil {
					return stageErr
				}
				return writeChainDump(cmd, false)
			}), core.Config{BackupDir: t.TempDir(), LogDir: t.TempDir()})
			job, err := manager.StartBackup(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			final := waitFinal(t, manager, job.ID)
			if final.Status != "failed" || final.BackupCompleted || calls != 1 {
				t.Fatalf("destination touched after unsafe dump: %+v calls=%d", final, calls)
			}
		})
	}
}
func fmtName(err error) string {
	if err != nil {
		return "failed"
	}
	return "incomplete"
}
func TestBackupRestoreFailureKeepsPartialBackupAndCancellationReservesCapacity(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore failure", true: "cancel restore"}[cancel], func(t *testing.T) {
			repo, req := chainFixture()
			entered := make(chan struct{})
			calls := 0
			manager := New(repo, executorFunc(func(ctx context.Context, cmd core.Command, _ func(string, string)) error {
				calls++
				if calls == 1 {
					return writeChainDump(cmd, true)
				}
				if calls == 2 {
					return nil
				}
				close(entered)
				if cancel {
					<-ctx.Done()
					return ctx.Err()
				}
				return errors.New("restore failed target-secret")
			}), core.Config{BackupDir: t.TempDir(), LogDir: t.TempDir()})
			job, err := manager.StartBackup(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			<-entered
			if cancel {
				if _, err := manager.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
					t.Fatal("capacity released between stages", err)
				}
				if _, err := manager.Cancel(context.Background(), job.ID); err != nil {
					t.Fatal(err)
				}
			}
			final := waitFinal(t, manager, job.ID)
			expected := "failed"
			if cancel {
				expected = "cancelled"
			}
			if final.Status != expected || !final.PartialResult || !final.BackupCompleted || strings.Contains(final.Message, "target-secret") {
				t.Fatalf("partial result: %+v", final)
			}
			saved, _ := repo.GetJob(context.Background(), job.ID)
			if !saved.PartialResult || !saved.BackupCompleted {
				t.Fatal("partial result not persisted")
			}
			if _, err := core.ListBackupTables(final.Path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestBackupRestoreRejectsUnsafeDestinationAndProductionThreadsBeforeExecution(t *testing.T) {
	for _, mode := range []string{"remote", "default database", "same source", "production threads", "invalid restore threads"} {
		t.Run(mode, func(t *testing.T) {
			repo, req := chainFixture()
			switch mode {
			case "remote":
				repo.target.Host = "remote.example"
			case "default database":
				req.Restore.TargetDatabase = "mysql"
			case "same source":
				repo.profile.Host = "localhost"
				req.Restore.TargetDatabase = "production"
			case "production threads":
				req.Threads = 3
			case "invalid restore threads":
				req.Restore.Threads = -1
			}
			manager := New(repo, executorFunc(func(context.Context, core.Command, func(string, string)) error {
				t.Fatal("unsafe request executed")
				return nil
			}), core.Config{BackupDir: t.TempDir(), LogDir: t.TempDir()})
			if _, err := manager.StartBackup(context.Background(), req); err == nil {
				t.Fatal("unsafe request accepted")
			}
			if len(repo.jobs) != 0 {
				t.Fatal("unsafe request created operation")
			}
		})
	}
}
