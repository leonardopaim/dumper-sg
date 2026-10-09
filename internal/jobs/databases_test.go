package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

func TestDatabaseCatalogMatchesOnlyAvailableDatabasesAndDoesNotLogNames(t *testing.T) {
	var commands []core.Command
	m := New(newRepo(), executorFunc(func(_ context.Context, cmd core.Command, emit func(string, string)) error {
		commands = append(commands, cmd)
		if len(commands) == 1 {
			for _, name := range []string{"sommusgestor_12", "other", "sommusgestor_13", "sommusgestor_012"} {
				emit("data", fmt.Sprintf(`{"name":%q}`, name))
			}
		} else if len(commands) == 2 {
			emit("data", `{"group_id":12,"group_name":"Comércio São José"}`)
			emit("data", `{"group_id":999,"group_name":"Banco ausente"}`)
		} else {
			emit("data", `{"company_id":1,"group_id":12,"legal_name":"Mercado José Ltda","trade_name":"Mercado São José"}`)
			emit("data", `{"company_id":2,"group_id":12,"legal_name":"Filial José Ltda","trade_name":""}`)
			emit("data", `{"company_id":3,"group_id":999,"legal_name":"Banco ausente Ltda","trade_name":""}`)
		}
		return nil
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartDatabaseList(context.Background(), core.DatabaseListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if final := waitFinal(t, m, job.ID); final.Status != "succeeded" {
		t.Fatal(final)
	}
	result, err := m.Databases(context.Background(), job.ID)
	if err != nil || len(result.Databases) != 4 || result.Warning != "" {
		t.Fatal(result, err)
	}
	for _, row := range result.Databases {
		if row.Name == "sommusgestor_12" {
			if row.GroupName != "Comércio São José" || row.GroupID != 12 || len(row.Companies) != 2 || row.Companies[0].TradeName != "Mercado São José" {
				t.Fatal(row)
			}
		} else if row.GroupName != "" || row.GroupID != 0 || len(row.Companies) != 0 {
			t.Fatal("incorrect group match", row)
		}
	}
	result.Databases[0].Name = "mutated"
	result.Databases[2].Companies[0].LegalName = "mutated"
	again, _ := m.Databases(context.Background(), job.ID)
	if again.Databases[0].Name == "mutated" || again.Databases[2].Companies[0].LegalName == "mutated" {
		t.Fatal("caller mutated catalog")
	}
	if len(commands) != 3 || commands[0].ContainerName != commands[1].ContainerName || commands[0].DockerIdentity != commands[1].DockerIdentity || commands[0].ContainerName != commands[2].ContainerName || commands[0].DockerIdentity != commands[2].DockerIdentity {
		t.Fatal("connection changed")
	}
	events, _ := m.Events(context.Background(), job.ID, 0)
	for _, event := range events {
		if strings.Contains(event.Message, "Comércio") || strings.Contains(event.Message, "sommusgestor_12") || strings.Contains(event.Message, "Mercado") {
			t.Fatal("catalog leaked to logs")
		}
	}
}

func TestCompanyLookupFailurePreservesGroupsAndCancellationGuards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		invalid string
		count   int
		status  string
	}{
		{"permission denied", errors.New("SELECT denied"), "", 0, "succeeded"},
		{"malformed", nil, `{"company_id":1,"group_id":12,"legal_name":"","trade_name":""}`, 0, "succeeded"},
		{"duplicate", nil, "", 2, "succeeded"},
		{"oversized", nil, "", core.MaxCatalogDatabases + 1, "succeeded"},
		{"cancelled", context.Canceled, "", 0, "cancelled"},
		{"unconfirmed cleanup", core.ErrTerminationUnconfirmed, "", 0, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, emit func(string, string)) error {
				calls++
				switch calls {
				case 1:
					emit("data", `{"name":"sommusgestor_12"}`)
				case 2:
					emit("data", `{"group_id":12,"group_name":"Grupo"}`)
				case 3:
					for i := range tc.count {
						id := i + 1
						if tc.count == 2 {
							id = 1
						}
						emit("data", fmt.Sprintf(`{"company_id":%d,"group_id":12,"legal_name":"Empresa","trade_name":""}`, id))
					}
					if tc.invalid != "" {
						emit("data", tc.invalid)
					}
					return tc.failure
				}
				return nil
			}), core.Config{LogDir: t.TempDir()})
			job, err := m.StartDatabaseList(context.Background(), core.DatabaseListRequest{ProfileID: 1})
			if err != nil {
				t.Fatal(err)
			}
			final := waitFinal(t, m, job.ID)
			if final.Status != tc.status {
				t.Fatal(final)
			}
			result, err := m.Databases(context.Background(), job.ID)
			if tc.status == "succeeded" {
				if err != nil || result.Warning == "" || result.Databases[0].GroupName != "Grupo" || len(result.Databases[0].Companies) != 0 {
					t.Fatal(result, err)
				}
			} else if !errors.Is(err, core.ErrConflict) {
				t.Fatal("partial catalog returned", err)
			}
			if errors.Is(tc.failure, core.ErrTerminationUnconfirmed) && !final.CleanupRequired {
				t.Fatal("guard lost")
			}
		})
	}
}

func TestUnavailableGroupNamesFallBackButCleanupFailuresRemainBlocking(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus string
		warning    bool
	}{
		{"missing table", errors.New("table does not exist"), "succeeded", true},
		{"permission denied", errors.New("SELECT denied"), "succeeded", true},
		{"cancelled", context.Canceled, "cancelled", false},
		{"cleanup unconfirmed", core.ErrTerminationUnconfirmed, "failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, emit func(string, string)) error {
				calls++
				if calls == 1 {
					emit("data", `{"name":"sommusgestor_12"}`)
					return nil
				}
				emit("data", `{"group_id":12,"group_name":"Partial"}`)
				return tc.err
			}), core.Config{LogDir: t.TempDir()})
			job, err := m.StartDatabaseList(context.Background(), core.DatabaseListRequest{ProfileID: 1})
			if err != nil {
				t.Fatal(err)
			}
			final := waitFinal(t, m, job.ID)
			if final.Status != tc.wantStatus {
				t.Fatal(final)
			}
			result, err := m.Databases(context.Background(), job.ID)
			if tc.warning {
				if !final.PartialResult || final.WarningCount == 0 || final.WarningMessage == "" {
					t.Fatal("partial result metadata missing", final)
				}
				if err != nil || result.Warning == "" || len(result.Databases) != 1 || result.Databases[0].GroupName != "" {
					t.Fatal(result, err)
				}
			} else if !errors.Is(err, core.ErrConflict) {
				t.Fatal("partial results returned", err)
			}
			if errors.Is(tc.err, core.ErrTerminationUnconfirmed) {
				if !final.CleanupRequired {
					t.Fatal("cleanup guard lost")
				}
				if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
					t.Fatal("guard released", err)
				}
			}
		})
	}
}

func TestDatabaseCatalogRejectsDuplicatesLimitsAndInvalidGroups(t *testing.T) {
	for _, tc := range []struct {
		name      string
		count     int
		duplicate bool
		badGroup  bool
		want      string
	}{
		{"empty", 0, false, false, "succeeded"},
		{"duplicate", 2, true, false, "failed"},
		{"oversized", core.MaxCatalogDatabases + 1, false, false, "failed"},
		{"invalid group", 1, false, true, "succeeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, emit func(string, string)) error {
				calls++
				if calls == 1 {
					for i := range tc.count {
						if tc.duplicate {
							i = 0
						}
						emit("data", fmt.Sprintf(`{"name":"sommusgestor_%d"}`, i+1))
					}
				} else if tc.badGroup {
					emit("data", "not JSON")
				}
				return nil
			}), core.Config{LogDir: t.TempDir()})
			job, err := m.StartDatabaseList(context.Background(), core.DatabaseListRequest{ProfileID: 1})
			if err != nil {
				t.Fatal(err)
			}
			if final := waitFinal(t, m, job.ID); final.Status != tc.want {
				t.Fatal(final)
			}
			result, err := m.Databases(context.Background(), job.ID)
			if tc.want == "failed" {
				if !errors.Is(err, core.ErrConflict) {
					t.Fatal(err)
				}
			} else if err != nil || result.Databases == nil {
				t.Fatal(result, err)
			}
			if tc.badGroup && result.Warning == "" {
				t.Fatal("bad groups silently accepted")
			}
		})
	}
}

func TestOptionalGroupLookupKeepsSlotUntilCancellationCompletes(t *testing.T) {
	entered, terminated := make(chan struct{}), make(chan struct{})
	calls := 0
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, emit func(string, string)) error {
		calls++
		if calls == 1 {
			emit("data", `{"name":"sommusgestor_12"}`)
			return nil
		}
		close(entered)
		<-ctx.Done()
		<-terminated
		return ctx.Err()
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartDatabaseList(context.Background(), core.DatabaseListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatal("optional lookup released slot", err)
	}
	if _, err := m.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatal("cancellation released slot too early", err)
	}
	close(terminated)
	if final := waitFinal(t, m, job.ID); final.Status != "cancelled" {
		t.Fatal(final)
	}
	if _, err := m.Databases(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatal("cancelled results returned", err)
	}
}
