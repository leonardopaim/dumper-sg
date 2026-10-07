package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"dumpersg/internal/core"
)

func runtimeFactory(native bool, calls *[][]string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, program string, args ...string) *exec.Cmd {
		*calls = append(*calls, append([]string{program}, args...))
		process := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestRuntimeHelper$", "--", program}, args...)...)
		process.Env = append(os.Environ(), "DUMPERSG_RUNTIME_HELPER=1", fmt.Sprintf("DUMPERSG_NATIVE_UP=%t", native))
		return process
	}
}

func TestTransportSelection(t *testing.T) {
	for _, tc := range []struct {
		name, platform, mode, distro, expected, expectedDistro string
		native                                                 bool
	}{
		{"native available", "windows", "auto", "", "native", "", true},
		{"fallback WSL", "windows", "auto", "", "wsl", "Ubuntu", false},
		{"explicit WSL", "windows", "wsl", "Ubuntu Test", "wsl", "Ubuntu Test", true},
		{"distro overrides auto", "windows", "auto", "Ubuntu Test", "wsl", "Ubuntu Test", true},
		{"Linux native", "linux", "auto", "", "native", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			e, err := detect(context.Background(), Options{Runtime: tc.mode, Distribution: tc.distro}, tc.platform, runtimeFactory(tc.native, &calls))
			if err != nil || e.runtime != tc.expected || e.distribution != tc.expectedDistro {
				t.Fatalf("selection: %#v %v", e, err)
			}
			if tc.distro != "" && len(calls) != 0 {
				t.Fatalf("explicit distro must not probe native: %v", calls)
			}
		})
	}
	for _, tc := range []struct {
		platform string
		opts     Options
	}{
		{"windows", Options{Runtime: "bad"}}, {"linux", Options{Runtime: "wsl"}},
		{"windows", Options{Runtime: "native", Distribution: "Ubuntu"}}, {"windows", Options{Distribution: "--option"}},
	} {
		if _, err := detect(context.Background(), tc.opts, tc.platform, nil); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
}

func TestWSLMountsAndArguments(t *testing.T) {
	var calls [][]string
	e := &Executor{runtime: "wsl", distribution: "Ubuntu Test", processFactory: runtimeFactory(false, &calls)}
	source := `C:\Test User\backup € $(not-a-command)`
	original := []string{"create", "-v", source + ":/backup", "image", "mydumper", "--password=secret"}
	args, err := e.translateMounts(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if original[2] != source+":/backup" || args[2] != "/mnt/c/Test User/backup € $(not-a-command):/backup" {
		t.Fatalf("mount: %v", args)
	}
	expected := []string{"wsl.exe", "--distribution", "Ubuntu Test", "--exec", "wslpath", "-a", "-u", source}
	if !reflect.DeepEqual(calls[0], expected) {
		t.Fatalf("shell or source changed: %v", calls[0])
	}
	_ = e.command(context.Background(), args...)
	want := append([]string{"wsl.exe", "--distribution", "Ubuntu Test", "--exec", "docker"}, args...)
	if !reflect.DeepEqual(calls[len(calls)-1], want) {
		t.Fatalf("Docker transport: %v", calls)
	}
}

func TestWSLPathFailureDoesNotCreate(t *testing.T) {
	var calls [][]string
	e := &Executor{runtime: "wsl", distribution: "Ubuntu", processFactory: runtimeFactory(false, &calls)}
	cmd := helperCommand()
	cmd.Args = []string{"run", "--rm", "--name", cmd.ContainerName, "-v", `C:\unavailable:/backup`, "fake-image"}
	if err := e.Run(context.Background(), cmd, nil); err == nil {
		t.Fatal("failed path accepted")
	}
	if len(calls) != 1 || !strings.Contains(strings.Join(calls[0], " "), "wslpath") {
		t.Fatalf("Docker created after path failure: %v", calls)
	}
}

func TestRecoveryRetainsGuardOnDifferentDaemon(t *testing.T) {
	var calls [][]string
	e := &Executor{runtime: "wsl", distribution: "Ubuntu", processFactory: runtimeFactory(false, &calls)}
	cmd := helperCommand()
	cmd.DockerIdentity = "WSL (Ubuntu):different-daemon"
	if err := e.VerifyTermination(context.Background(), cmd); !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("wrong daemon accepted: %v", err)
	}
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), "container ls") {
			t.Fatal("queried wrong daemon")
		}
	}
	cmd.DockerIdentity = ""
	if err := e.VerifyTermination(context.Background(), cmd); !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("unknown legacy daemon accepted: %v", err)
	}
}

func TestUnresolvedWSLDistributionCannotStartOrIdentifyJobs(t *testing.T) {
	var calls [][]string
	e := &Executor{runtime: "wsl", processFactory: runtimeFactory(false, &calls)}
	if _, err := e.ExecutionIdentity(context.Background()); err == nil {
		t.Fatal("unresolved distro identified")
	}
	if err := e.Run(context.Background(), helperCommand(), nil); err == nil {
		t.Fatal("unresolved distro started")
	}
	if len(calls) != 0 {
		t.Fatalf("default distro used for operation: %v", calls)
	}
}

func TestCleanupRetainsGuardIfDaemonChanges(t *testing.T) {
	var calls [][]string
	e := &Executor{runtime: "wsl", distribution: "Ubuntu", processFactory: runtimeFactory(false, &calls)}
	err := e.cleanup("dumpersg-test", "owner", "WSL (Ubuntu):another-daemon")
	if !errors.Is(err, core.ErrTerminationUnconfirmed) {
		t.Fatalf("different daemon released guard: %v", err)
	}
	if len(calls) != 1 || !strings.Contains(strings.Join(calls[0], " "), "docker info") {
		t.Fatalf("cleanup touched wrong daemon: %v", calls)
	}
}

func TestRuntimeHelper(t *testing.T) {
	if os.Getenv("DUMPERSG_RUNTIME_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	if args[0] == "docker" {
		if os.Getenv("DUMPERSG_NATIVE_UP") != "true" {
			os.Exit(1)
		}
		fmt.Println("28.2.2")
		os.Exit(0)
	}
	for len(args) > 0 && args[0] != "--exec" {
		args = args[1:]
	}
	args = args[1:]
	switch args[0] {
	case "printenv":
		fmt.Println("Ubuntu")
	case "wslpath":
		if strings.Contains(args[len(args)-1], "unavailable") {
			os.Exit(2)
		}
		fmt.Println("/mnt/c/Test User/backup € $(not-a-command)")
	case "docker":
		if args[1] == "info" {
			fmt.Println("daemon-test")
		} else {
			fmt.Println("28.2.2")
		}
	case "test":
	default:
		os.Exit(2)
	}
	os.Exit(0)
}
