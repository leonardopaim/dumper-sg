package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"dumpersg/internal/core"
)

// Explicit opt-in. Creates only a uniquely named disposable Alpine container.
func TestWSLCancellationIntegration(t *testing.T) {
	distro := os.Getenv("DUMPERSG_TEST_WSL_INTEGRATION")
	if distro == "" {
		t.Skip("set DUMPERSG_TEST_WSL_INTEGRATION to a WSL distro to opt in")
	}
	e, err := Detect(context.Background(), Options{Runtime: "wsl", Distribution: distro})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := e.ExecutionIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "dumpersg-test-cancel-" + hex.EncodeToString(random[:])
	command := core.Command{Program: "docker", Args: []string{"run", "--rm", "--name", name, "alpine:latest", "sleep", "60"}, ContainerName: name, DockerIdentity: identity}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- e.Run(ctx, command, nil) }()
	ready := false
	for start := time.Now(); time.Since(start) < 10*time.Second; {
		output, err := e.invoke(context.Background(), "inspect", "--format", "{{.State.Running}}", name)
		if err == nil && strings.TrimSpace(output) == "true" {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation result: %v", err)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("WSL cancellation did not finish")
	}
	if !ready {
		t.Fatal("container never reached running state")
	}
	if err := e.VerifyTermination(context.Background(), command); err != nil {
		t.Fatalf("container survived cancellation: %v", err)
	}
}
