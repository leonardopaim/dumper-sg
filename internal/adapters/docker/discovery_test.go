package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type discoveryScenario struct {
	Native, WSL bool
	OS, ID      string
}

func discoveryFactory(s *discoveryScenario, calls *[][]string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, program string, args ...string) *exec.Cmd {
		*calls = append(*calls, append([]string{program}, args...))
		data, _ := json.Marshal(s)
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDiscoveryHelper$", "--", program}, args...)...)
		cmd.Env = append(os.Environ(), "DUMPERSG_DISCOVERY="+string(data))
		return cmd
	}
}

func TestAutoRetriesUntilDaemonRespondsThenPinsTransport(t *testing.T) {
	s := discoveryScenario{OS: "linux", ID: "desktop-id"}
	var calls [][]string
	e, err := detect(context.Background(), Options{Runtime: "auto"}, "windows", discoveryFactory(&s, &calls))
	if err != nil || !e.autoPending {
		t.Fatalf("unavailable Docker prevented opening app: %v", err)
	}
	if d := e.Diagnostics(context.Background()); d.Available || !strings.Contains(d.Message, "Abra o Docker Desktop") {
		t.Fatalf("diagnostic: %+v", d)
	}
	s.Native = true
	d := e.Diagnostics(context.Background())
	if !d.Available || !strings.Contains(d.Message, "Docker Desktop") || e.autoPending {
		t.Fatalf("late Desktop not detected: %+v", d)
	}
	id, err := e.ExecutionIdentity(context.Background())
	if err != nil || id != "CLI nativa:desktop-id" {
		t.Fatalf("identity %q %v", id, err)
	}
	s.Native, s.WSL = false, true
	calls = nil
	if _, err := e.ExecutionIdentity(context.Background()); err == nil {
		t.Fatal("silently switched to WSL")
	}
	for _, call := range calls {
		if call[0] == "wsl.exe" {
			t.Fatal("pinned transport fell back to WSL")
		}
	}
}

func TestDesktopLinuxRequirementAndWSLFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		s         discoveryScenario
		want      string
		available bool
	}{
		{"Desktop", discoveryScenario{true, true, "linux", "desktop-id"}, "Docker Desktop", true},
		{"WSL Engine", discoveryScenario{false, true, "linux", "wsl-id"}, "Docker Engine", true},
		{"Windows containers", discoveryScenario{true, true, "windows", "desktop-id"}, "Linux containers", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			e, err := detect(context.Background(), Options{Runtime: "auto"}, "windows", discoveryFactory(&tc.s, &calls))
			if err != nil {
				t.Fatal(err)
			}
			d := e.Diagnostics(context.Background())
			if d.Available != tc.available || !strings.Contains(d.Message, tc.want) {
				t.Fatalf("diagnostic: %+v", d)
			}
			if tc.s.OS == "windows" {
				for _, call := range calls {
					if call[0] == "wsl.exe" {
						t.Fatal("Windows containers error hidden by fallback")
					}
				}
			}
		})
	}
}

func TestDesktopNetworkingInExecutor(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "host.docker.internal", "db.example"} {
		t.Run(host, func(t *testing.T) {
			s := discoveryScenario{Native: true, OS: "linux", ID: "desktop-id"}
			var calls [][]string
			e, err := detect(context.Background(), Options{Runtime: "native"}, "windows", discoveryFactory(&s, &calls))
			if err != nil {
				t.Fatal(err)
			}
			cmd := helperCommand()
			cmd.DockerIdentity, err = e.ExecutionIdentity(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "--rm", "--name", cmd.ContainerName, "--network", "host", "-v", `C:\Test User\backup:/backup`, "mysql:test", "mysql", "--host=" + host, "--password=--host=localhost", "--execute=SELECT '--network host'"}
			cmd.Args = append([]string(nil), args...)
			if err := e.Run(context.Background(), cmd, nil); err != nil {
				t.Fatal(err)
			}
			var create []string
			for _, call := range calls {
				if len(call) > 1 && call[1] == "create" {
					create = call
				}
			}
			if len(create) == 0 {
				t.Fatal("no container created")
			}
			want := host
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				want = "host.docker.internal"
			}
			joined := strings.Join(create, "\n")
			if strings.Contains(joined, "\n--network\n") || !strings.Contains(joined, "\n--host="+want+"\n") || !strings.Contains(joined, `C:\Test User\backup:/backup`) || !strings.Contains(joined, "--password=--host=localhost") || !strings.Contains(joined, "--execute=SELECT '--network host'") {
				t.Fatalf("unexpected arguments: %v", create)
			}
			if !reflect.DeepEqual(cmd.Args, args) {
				t.Fatal("core command mutated")
			}
		})
	}
}

func TestEngineKeepsHostNetworkAndLoopback(t *testing.T) {
	e := &Executor{runtime: "wsl", distribution: "Ubuntu"}
	args := []string{"create", "--network=host", "image", "mysql", "--host=localhost"}
	if got := e.desktopNetworking(args); !reflect.DeepEqual(got, args) {
		t.Fatalf("Engine command changed: %v", got)
	}
	e.desktop = true
	if got := e.desktopNetworking(args); !reflect.DeepEqual(got, []string{"create", "image", "mysql", "--host=host.docker.internal"}) {
		t.Fatalf("Desktop via WSL not adapted: %v", got)
	}
}

func TestDiscoveryHelper(t *testing.T) {
	raw := os.Getenv("DUMPERSG_DISCOVERY")
	if raw == "" {
		return
	}
	var s discoveryScenario
	if json.Unmarshal([]byte(raw), &s) != nil {
		os.Exit(2)
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	native := args[0] == "docker"
	if (native && !s.Native) || (!native && !s.WSL) {
		os.Exit(1)
	}
	args = args[1:]
	if !native {
		for len(args) > 0 && args[0] != "--exec" {
			args = args[1:]
		}
		args = args[1:]
		if args[0] == "printenv" {
			fmt.Println("Ubuntu")
			os.Exit(0)
		}
		args = args[1:]
	}
	switch args[0] {
	case "info":
		operatingSystem, name := "Ubuntu", "ubuntu"
		if native {
			operatingSystem, name = "Docker Desktop", "docker-desktop"
		}
		data, _ := json.Marshal(daemonDetails{ID: s.ID, OSType: s.OS, OperatingSystem: operatingSystem, Name: name})
		fmt.Println(string(data))
	case "version":
		fmt.Println("28.2.2")
	case "create":
		fmt.Println("fake-container")
	case "start", "container", "stop", "rm":
	default:
		os.Exit(2)
	}
	os.Exit(0)
}
