package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

func tableLine(name string, size uint64) string {
	row := core.TableInfo{Name: name, SizeBytes: size, Rows: 12, TableType: "BASE TABLE"}
	data, err := json.Marshal(row)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestTableCatalogRetainsMoreThanEventsAndDoesNotLogData(t *testing.T) {
	dir := t.TempDir()
	m := New(newRepo(), executorFunc(func(_ context.Context, cmd core.Command, log func(string, string)) error {
		if !cmd.StdoutData {
			return errors.New("structured stdout not requested")
		}
		log("warning", "connection warning top-secret")
		for i := range 750 {
			log("data", tableLine(fmt.Sprintf("table-%04d", i), uint64(i%7)))
		}
		log("data", tableLine("top-secret_table", 99))
		return nil
	}), core.Config{LogDir: dir})
	job, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "succeeded" || final.Kind != "table_list" || final.Database != "production" {
		t.Fatalf("catalog failed: %+v", final)
	}
	tables, err := m.Tables(context.Background(), job.ID)
	if err != nil || len(tables) != 751 {
		t.Fatalf("catalog truncated to event retention: %d %v", len(tables), err)
	}
	if tables[0].Name != "top-secret_table" || tables[0].SizeBytes != 99 {
		t.Fatalf("raw name lost or sorting incorrect: %+v", tables[0])
	}
	for i := 1; i < len(tables); i++ {
		if tables[i].SizeBytes > tables[i-1].SizeBytes || (tables[i].SizeBytes == tables[i-1].SizeBytes && tables[i].Name < tables[i-1].Name) {
			t.Fatal("catalog order incorrect")
		}
	}
	tables[0].Name = "caller mutation"
	again, err := m.Tables(context.Background(), job.ID)
	if err != nil || again[0].Name != "top-secret_table" {
		t.Fatal("caller mutated retained catalog")
	}
	events, err := m.Events(context.Background(), job.ID, 0)
	if err != nil || len(events) > 5 {
		t.Fatalf("JSON rows became events: %d %v", len(events), err)
	}
	for _, event := range events {
		if strings.Contains(event.Message, "table-") || strings.Contains(event.Message, "top-secret") || strings.Contains(event.Message, "size_bytes") {
			t.Fatal("structured output leaked to events")
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, job.ID+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "table-") || strings.Contains(string(data), "top-secret") || strings.Contains(string(data), "size_bytes") {
		t.Fatal("structured output leaked to log")
	}
}

func TestMalformedAndFailedCatalogsNeverReturnPartialData(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		failure     error
	}{
		{"malformed", "not JSON", nil},
		{"missing fields", `{"name":"invalid"}`, nil},
		{"negative size", `{"name":"invalid","size_bytes":-1,"rows":0,"table_type":"VIEW"}`, nil},
		{"duplicate", tableLine("valid", 10), nil},
		{"oversized", strings.Repeat("x", core.MaxTableJSONBytes+1), nil},
		{"CLI failure", "", errors.New("query failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error {
				log("data", tableLine("valid", 10))
				if tc.extra != "" {
					log("data", tc.extra)
				}
				return tc.failure
			}), core.Config{LogDir: t.TempDir()})
			job, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
			if err != nil {
				t.Fatal(err)
			}
			final := waitFinal(t, m, job.ID)
			if final.Status != "failed" {
				t.Fatalf("malformed response succeeded: %+v", final)
			}
			if tables, err := m.Tables(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) || tables != nil {
				t.Fatalf("partial results returned: %v %v", tables, err)
			}
		})
	}
}

func TestCatalogLimitIsExplicitAndEmptyCatalogIsArray(t *testing.T) {
	for _, count := range []int{0, core.MaxCatalogTables, core.MaxCatalogTables + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error {
				for i := range count {
					log("data", tableLine(fmt.Sprintf("t%05d", i), 1))
				}
				return nil
			}), core.Config{LogDir: t.TempDir()})
			job, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
			if err != nil {
				t.Fatal(err)
			}
			final := waitFinal(t, m, job.ID)
			tables, err := m.Tables(context.Background(), job.ID)
			if count > core.MaxCatalogTables {
				if final.Status != "failed" || !strings.Contains(final.Message, "10000") || !errors.Is(err, core.ErrConflict) || tables != nil {
					t.Fatalf("oversized catalog silently truncated: %+v %v", final, err)
				}
			} else {
				if final.Status != "succeeded" || err != nil || len(tables) != count || tables == nil {
					t.Fatalf("catalog result: %+v %d %v", final, len(tables), err)
				}
			}
		})
	}
}

func TestCatalogCancellationKeepsExclusiveSlotUntilTermination(t *testing.T) {
	entered := make(chan struct{})
	termination := make(chan struct{})
	m := New(newRepo(), executorFunc(func(ctx context.Context, _ core.Command, log func(string, string)) error {
		log("data", tableLine("partial", 1))
		close(entered)
		<-ctx.Done()
		<-termination
		return ctx.Err()
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err = m.Tables(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatal("running tables available")
	}
	if _, err = m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatal("catalog did not own exclusive slot")
	}
	if _, err = m.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1}); !errors.Is(err, core.ErrConflict) {
		t.Fatal("cancel freed slot before external termination")
	}
	close(termination)
	final := waitFinal(t, m, job.ID)
	if final.Status != "cancelled" {
		t.Fatalf("cancel failed: %+v", final)
	}
	if tables, err := m.Tables(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) || tables != nil {
		t.Fatal("cancelled catalog returned partial data")
	}
}

func TestCatalogCleanupFailurePreservesQuarantine(t *testing.T) {
	m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error {
		log("data", tableLine("partial", 1))
		return core.ErrTerminationUnconfirmed
	}), core.Config{LogDir: t.TempDir()})
	job, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	final := waitFinal(t, m, job.ID)
	if final.Status != "failed" || !final.CleanupRequired {
		t.Fatalf("catalog released quarantine: %+v", final)
	}
	if tables, err := m.Tables(context.Background(), job.ID); !errors.Is(err, core.ErrConflict) || tables != nil {
		t.Fatal("unconfirmed catalog returned partial data")
	}
	if _, err = m.StartTest(context.Background(), 1); !errors.Is(err, core.ErrConflict) {
		t.Fatal("next job started before confirmed termination")
	}
}

func TestCatalogRetentionRestartAndWrongKindReturnNotFound(t *testing.T) {
	repo := newRepo()
	fake := executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error { return nil })
	m := New(repo, fake, core.Config{LogDir: t.TempDir()})
	first, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, first.ID)
	for range maxRetainedJobs {
		job, err := m.StartTest(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		waitFinal(t, m, job.ID)
		if _, err = m.Tables(context.Background(), job.ID); !errors.Is(err, core.ErrNotFound) {
			t.Fatal("wrong kind produced catalog")
		}
	}
	if _, err = m.Tables(context.Background(), first.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatal("evicted catalog remained available")
	}
	latest, err := m.StartTableList(context.Background(), core.TableListRequest{ProfileID: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, latest.ID)
	restarted := New(repo, fake, core.Config{LogDir: t.TempDir()})
	if _, err = restarted.Tables(context.Background(), latest.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatal("catalog appeared persisted after restart")
	}
}

func TestRawDataWithoutCatalogNeverBecomesLogOrEvent(t *testing.T) {
	dir := t.TempDir()
	m := New(newRepo(), executorFunc(func(_ context.Context, _ core.Command, log func(string, string)) error {
		log("data", `{"private":"raw secret content"}`)
		log("info", "ordinary top-secret output")
		return nil
	}), core.Config{LogDir: dir})
	job, err := m.StartTest(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitFinal(t, m, job.ID)
	events, err := m.Events(context.Background(), job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Level == "data" || strings.Contains(event.Message, "raw secret content") {
			t.Fatal("unhandled data reached events")
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, job.ID+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "raw secret content") || strings.Contains(string(data), "top-secret") {
		t.Fatal("unhandled data or credential reached log")
	}
}
