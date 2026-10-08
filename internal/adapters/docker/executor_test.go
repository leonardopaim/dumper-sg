package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dumpersg/internal/core"
)

func TestLineWriterBoundsOutputAndRedactsSplitWrites(t *testing.T) {
	var lines []string
	w := &lineWriter{secrets: []string{"password"}, emit: func(_, s string) { lines = append(lines, s) }}
	w.Write([]byte("pass"))
	w.Write([]byte("word\r\n"))
	w.Write([]byte(strings.Repeat("x", 64<<10) + "password\n"))
	w.Write([]byte("tail password"))
	w.flush()
	if len(lines) != 3 || lines[0] != "***" || lines[1] != "[linha maior que 64 KiB omitida]" || lines[2] != "tail ***" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestGLibMessagesUseDeclaredSeverityInsteadOfStderr(t *testing.T) {
	for _, tc := range []struct{ text, stream, expected string }{
		{"** Message: 03:19:47.055: MyDumper restore version: 1.0.3-1", "error", "info"},
		{"** Message: 03:19:47.127: Fast index creation will be used for table", "error", "info"},
		{"** (myloader:1): WARNING **: 03:19:47: Table already exists", "error", "warning"},
		{"** (myloader:1): CRITICAL **: 03:24:47: Unknown option", "error", "error"},
		{"** (myloader:1): ERROR **: Error restoring schema", "info", "error"},
		{"docker: daemon failed", "error", "error"},
		{"message body mentions ERROR but has no log prefix", "info", "info"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			var level, message string
			writer := &lineWriter{level: tc.stream, secrets: []string{"1.0.3-1"}, emit: func(l, s string) { level, message = l, s }}
			// Prefix/secrets may cross writes; the final partial line is also parsed.
			writer.Write([]byte(tc.text[:5]))
			writer.Write([]byte(tc.text[5:]))
			writer.flush()
			if level != tc.expected {
				t.Fatalf("level=%s expected=%s", level, tc.expected)
			}
			if strings.Contains(message, "1.0.3-1") {
				t.Fatal("redaction regressed")
			}
		})
	}
}

func helperExecutor(t *testing.T, mode string) (*Executor, string) {
	t.Helper()
	dir := t.TempDir()
	e := &Executor{commandFactory: func(ctx context.Context, args ...string) *exec.Cmd {
		process := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDockerHelperProcess$", "--"}, args...)...)
		process.Env = append(os.Environ(), "DUMPERSG_TEST_HELPER="+dir, "DUMPERSG_TEST_MODE="+mode)
		return process
	}}
	return e, dir
}

func helperCommand() core.Command {
	return core.Command{Program: "docker", Args: []string{"run", "--rm", "--name", "dumpersg-test", "fake-image", "mysql", "--password=top-secret"}, ContainerName: "dumpersg-test", Secrets: []string{"top-secret"}}
}

func awaitFile(t *testing.T, filename string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filename); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("helper did not reach expected stage")
}

func TestCancelDuringCreationWaitsThenRemovesWithoutStarting(t *testing.T) {
	e, dir := helperExecutor(t, "create-wait")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- e.Run(ctx, helperCommand(), nil) }()
	awaitFile(t, filepath.Join(dir, "creating"))
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not finish")
	}
	if _, err := os.Stat(filepath.Join(dir, "state")); !os.IsNotExist(err) {
		t.Fatal("container was not removed")
	}
	trace, err := os.ReadFile(filepath.Join(dir, "trace"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(trace), "start") || !strings.Contains(string(trace), "stop\nrm\ncontainer\n") {
		t.Fatalf("unexpected control sequence: %s", trace)
	}
}

func TestAttachedCancellationConfirmsCleanupAndReportsFailure(t *testing.T) {
	for _, mode := range []string{"wait", "cleanup-fail"} {
		t.Run(mode, func(t *testing.T) {
			e, dir := helperExecutor(t, mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- e.Run(ctx, helperCommand(), nil) }()
			awaitFile(t, filepath.Join(dir, "running"))
			cancel()
			select {
			case err := <-result:
				if mode == "wait" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel failed: %v", err)
				}
				if mode == "cleanup-fail" && (err == nil || errors.Is(err, context.Canceled)) {
					t.Fatalf("cleanup failure falsely cancelled: %v", err)
				}
				if mode == "cleanup-fail" && !errors.Is(err, core.ErrTerminationUnconfirmed) {
					t.Fatalf("cleanup did not retain termination guard: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("executor did not finish")
			}
			if mode == "wait" {
				if _, err := os.Stat(filepath.Join(dir, "state")); !os.IsNotExist(err) {
					t.Fatal("container remained")
				}
			}
		})
	}
}

func TestAttachedExitCodeAndRedactedOutput(t *testing.T) {
	e, _ := helperExecutor(t, "exit7")
	var lines []string
	err := e.Run(context.Background(), helperCommand(), func(_, line string) { lines = append(lines, line) })
	var exit interface{ ExitCode() int }
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("lost container exit code: %v", err)
	}
	if strings.Contains(strings.Join(lines, "\n"), "top-secret") || !strings.Contains(strings.Join(lines, "\n"), "***") {
		t.Fatalf("secret output leaked: %v", lines)
	}
}

// This is a fake Docker CLI subprocess. It only writes under the test's temp
// directory; no Docker, network, MySQL, backup or restore is invoked.
func TestDockerHelperProcess(t *testing.T) {
	dir := os.Getenv("DUMPERSG_TEST_HELPER")
	if dir == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(2)
	}
	args = args[1:]
	mode := os.Getenv("DUMPERSG_TEST_MODE")
	trace, err := os.OpenFile(filepath.Join(dir, "trace"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(trace, args[0])
	trace.Close()
	state := filepath.Join(dir, "state")
	switch args[0] {
	case "create":
		if len(args) < 5 || args[1] != "--label" || strings.Contains(strings.Join(args, "\n"), "\n--rm\n") {
			os.Exit(3)
		}
		os.WriteFile(filepath.Join(dir, "creating"), []byte("ready"), 0600)
		if mode == "create-wait" {
			time.Sleep(150 * time.Millisecond)
		}
		os.WriteFile(state, []byte(args[2]), 0600)
		fmt.Println("container-id")
	case "start":
		os.WriteFile(filepath.Join(dir, "running"), []byte("ready"), 0600)
		fmt.Println("output top-secret")
		if mode == "wait" || mode == "cleanup-fail" {
			time.Sleep(10 * time.Second)
		}
		if mode == "exit7" {
			os.Exit(7)
		}
	case "container":
		if owner, err := os.ReadFile(state); err == nil {
			if strings.Contains(strings.Join(args, "\n"), "label=") && !strings.Contains(strings.Join(args, "\n"), "label="+string(owner)) {
				os.Exit(4)
			}
			fmt.Println("container-id")
		}
	case "stop":
	case "rm":
		if mode == "cleanup-fail" {
			os.Exit(1)
		}
		os.Remove(state)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func TestVerifyTerminationOnlyReleasesAbsentContainer(t *testing.T) {
	e, dir := helperExecutor(t, "cleanup-fail")
	if err := os.WriteFile(filepath.Join(dir, "state"), []byte("foreign-owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.VerifyTermination(context.Background(), helperCommand()); !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("existing container released: %v", err)
	}
	trace, err := os.ReadFile(filepath.Join(dir, "trace"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(trace), "stop") || strings.Contains(string(trace), "rm") {
		t.Fatal("verification changed an unknown container")
	}
	if err = os.Remove(filepath.Join(dir, "state")); err != nil {
		t.Fatal(err)
	}
	if err = e.VerifyTermination(context.Background(), helperCommand()); err != nil {
		t.Fatalf("absent container not confirmed: %v", err)
	}
}

func TestRunRejectsUnnamedAndArbitraryExecutables(t *testing.T) {
	for _, cmd := range []core.Command{{Program: "cmd", Args: []string{"run", "--rm", "--name", "safe"}, ContainerName: "safe"}, {Program: "docker", Args: []string{"run", "--rm", "image", "mysql"}, ContainerName: "safe"}, {Program: "docker", Args: []string{"run", "--rm", "--name", "other"}, ContainerName: "safe"}} {
		if err := New().Run(context.Background(), cmd, nil); err == nil {
			t.Fatal("invalid command accepted")
		}
	}
}
