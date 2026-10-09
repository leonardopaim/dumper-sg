package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"dumpersg/internal/core"
)

type Executor struct {
	selectionMu    sync.Mutex
	autoPending    bool
	desktop        bool
	runtime        string
	distribution   string
	processFactory func(context.Context, string, ...string) *exec.Cmd
	// Tests substitute the CLI with an isolated helper process, never a database.
	commandFactory func(context.Context, ...string) *exec.Cmd
}

func New() *Executor { return &Executor{} }

func (e *Executor) Diagnostics(ctx context.Context) core.Diagnostics {
	if _, err := e.inspectDaemon(ctx); err != nil {
		return core.Diagnostics{Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := e.invoke(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return core.Diagnostics{Message: "Docker não respondeu via " + e.runtimeDescription() + ". Verifique o daemon e as permissões do usuário."}
	}
	e.selectionMu.Lock()
	desktop := e.desktop
	e.selectionMu.Unlock()
	engine := "Docker Engine"
	if desktop {
		engine = "Docker Desktop"
	}
	return core.Diagnostics{Available: true, Version: strings.TrimSpace(output), Message: engine + " disponível via " + e.runtimeDescription() + "."}
}

var validContainer = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{1,127}$`)

// Run creates before starting so cancellation always has an identifiable container.
// Docker control commands use a separate bounded context: killing the attached
// client alone does not stop a container on the Docker daemon.
func (e *Executor) Run(ctx context.Context, command core.Command, output func(string, string)) error {
	e.selectionMu.Lock()
	runMode, runDistro := e.runtime, e.distribution
	e.selectionMu.Unlock()
	if runMode == "wsl" && runDistro == "" {
		return fmt.Errorf("distribuição WSL deve ser fixada antes da execução")
	}
	if command.Program != "docker" || len(command.Args) < 4 || command.Args[0] != "run" || !validContainer.MatchString(command.ContainerName) {
		return fmt.Errorf("comando Docker inválido")
	}
	nameFound := false
	createArgs := []string{"create"}
	for i := 1; i < len(command.Args); i++ {
		if command.Args[i] == "--rm" {
			continue
		}
		if command.Args[i] == "--name" && i+1 < len(command.Args) && command.Args[i+1] == command.ContainerName {
			nameFound = true
		}
		createArgs = append(createArgs, command.Args[i])
	}
	if !nameFound {
		return fmt.Errorf("container deve ser nomeado explicitamente")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.verifyIdentity(ctx, command.DockerIdentity); err != nil {
		return err
	}
	if command.Image != "" {
		if err := e.ensureImage(ctx, command.Image, output); err != nil {
			return err
		}
	}
	var err error
	createArgs = e.desktopNetworking(createArgs)
	createArgs, err = e.translateMounts(ctx, createArgs)
	if err != nil {
		return err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	owner := "com.dumpersg.owner=" + hex.EncodeToString(nonce[:])
	createArgs = append([]string{"create", "--label", owner}, createArgs[1:]...)
	createCtx, createCancel := context.WithTimeout(context.Background(), 45*time.Second)
	_, createErr := e.invoke(createCtx, createArgs...)
	createCancel()
	if createErr != nil {
		if cleanupErr := e.cleanup(command.ContainerName, owner, command.DockerIdentity); cleanupErr != nil {
			return cleanupErr
		}
		return fmt.Errorf("Docker não criou o container: %w", createErr)
	}
	var runErr error
	if ctx.Err() == nil {
		process := e.command(ctx, "start", "--attach", command.ContainerName)
		stdout := &lineWriter{emit: output, level: "info", secrets: command.Secrets}
		if command.StdoutData {
			stdout.level, stdout.secrets = "data", nil
		}
		stderr := &lineWriter{emit: output, level: "error", secrets: command.Secrets}
		process.Stdout, process.Stderr = stdout, stderr
		// Bound waits if an inherited pipe remains open after the CLI exits.
		process.WaitDelay = 3 * time.Second
		runErr = process.Run()
		stdout.flush()
		stderr.flush()
	}
	if err := e.cleanup(command.ContainerName, owner, command.DockerIdentity); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil {
		return fmt.Errorf("Docker encerrou com falha: %w", runErr)
	}
	return nil
}

func (e *Executor) cleanup(name, owner, identity string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err := e.verifyIdentity(ctx, identity); err != nil {
		return err
	}
	ids, err := e.ownedContainers(ctx, name, owner)
	if err != nil {
		return fmt.Errorf("%w: não foi possível verificar término: %v", core.ErrTerminationUnconfirmed, err)
	}
	for _, id := range strings.Fields(ids) {
		// Stop is best effort; force-remove below is authoritative and also handles
		// a stopped container and a start racing with cancellation.
		_, _ = e.invoke(ctx, "stop", "--time", "8", id)
		if _, err := e.invoke(ctx, "rm", "--force", id); err != nil {
			remaining, checkErr := e.ownedContainers(ctx, name, owner)
			if checkErr != nil || strings.TrimSpace(remaining) != "" {
				return fmt.Errorf("%w: não foi possível remover o container: %v", core.ErrTerminationUnconfirmed, err)
			}
		}
	}
	remaining, err := e.ownedContainers(ctx, name, owner)
	if err != nil || strings.TrimSpace(remaining) != "" {
		if err == nil {
			err = errors.New("container ainda existe")
		}
		return fmt.Errorf("%w: limpeza não confirmada: %v", core.ErrTerminationUnconfirmed, err)
	}
	return e.verifyIdentity(ctx, identity)
}

// VerifyTermination is read-only and conservative: an existing named container
// (even stopped) retains the guard until the operator removes it. It never stops
// an unrecognized container discovered after a restart.
func (e *Executor) VerifyTermination(ctx context.Context, command core.Command) error {
	if !validContainer.MatchString(command.ContainerName) {
		return fmt.Errorf("%w: nome inválido", core.ErrTerminationUnconfirmed)
	}
	if command.DockerIdentity != "" {
		identity, err := e.ExecutionIdentity(ctx)
		if err != nil || identity != command.DockerIdentity {
			return fmt.Errorf("%w: use o mesmo daemon e distribuição WSL da operação pendente", core.ErrTerminationUnconfirmed)
		}
	} else if e.isWSL() {
		return fmt.Errorf("%w: operação antiga sem identidade do daemon; verifique no transporte nativo original", core.ErrTerminationUnconfirmed)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ids, err := e.invoke(ctx, "container", "ls", "--all", "--filter", "name=^/"+regexp.QuoteMeta(command.ContainerName)+"$", "--format", "{{.ID}}")
	if err != nil {
		return fmt.Errorf("%w: Docker não respondeu: %v", core.ErrTerminationUnconfirmed, err)
	}
	if strings.TrimSpace(ids) != "" {
		return fmt.Errorf("%w: remova o container %s após verificar seu estado", core.ErrTerminationUnconfirmed, command.ContainerName)
	}
	return nil
}

func (e *Executor) ownedContainers(ctx context.Context, name, owner string) (string, error) {
	return e.invoke(ctx, "container", "ls", "--all", "--filter", "name=^/"+regexp.QuoteMeta(name)+"$", "--filter", "label="+owner, "--format", "{{.ID}}")
}

type boundedWriter struct{ data []byte }

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if space := (64 << 10) - len(w.data); space > 0 {
		if len(p) > space {
			p = p[:space]
		}
		w.data = append(w.data, p...)
	}
	return n, nil
}

func (e *Executor) command(ctx context.Context, args ...string) *exec.Cmd {
	if e.commandFactory != nil {
		return e.commandFactory(ctx, args...)
	}
	if e.isWSL() {
		return e.hostCommand(ctx, "wsl.exe", e.wslArgs("docker", args...)...)
	}
	return e.hostCommand(ctx, "docker", args...)
}

func (e *Executor) invoke(ctx context.Context, args ...string) (string, error) {
	process := e.command(ctx, args...)
	var stdout, stderr boundedWriter
	process.Stdout, process.Stderr = &stdout, &stderr
	process.WaitDelay = 3 * time.Second
	err := process.Run()
	// Do not expose Docker's stderr here: it may repeat sensitive arguments.
	return string(stdout.data), err
}

type lineWriter struct {
	emit      func(string, string)
	level     string
	secrets   []string
	line      []byte
	oversized bool
}

// GLib/MyDumper writes every severity to stderr, including normal Message logs.
// Recognize its structured prefix; keep the stream level for unknown text.
var glibLevel = regexp.MustCompile(`(?i)^\s*\*\*\s+(?:\([^)]+\):\s*)?(message|info|debug|warning|critical|error)(?:\s*\*\*)?:`)
var mysqlWarning = regexp.MustCompile(`(?i)^\s*mysql(?:\.exe)?:\s*\[warning\]`)

func logLevel(line, fallback string) string {
	if mysqlWarning.MatchString(line) {
		return "warning"
	}
	match := glibLevel.FindStringSubmatch(line)
	if len(match) < 2 {
		return fallback
	}
	switch strings.ToLower(match[1]) {
	case "message", "info", "debug":
		return "info"
	case "warning":
		return "warning"
	default:
		return "error"
	}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			w.flush()
			continue
		}
		if w.oversized {
			continue
		}
		if len(w.line) >= 64<<10 {
			w.line = nil
			w.oversized = true
			continue
		}
		w.line = append(w.line, b)
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if w.oversized {
		if w.emit != nil {
			w.emit(w.level, "[linha maior que 64 KiB omitida]")
		}
	} else if len(w.line) > 0 {
		line := strings.TrimSuffix(string(w.line), "\r")
		level := logLevel(line, w.level)
		if w.level == "data" {
			level = "data"
		}
		for _, secret := range w.secrets {
			if secret != "" {
				line = strings.ReplaceAll(line, secret, "***")
			}
		}
		if w.emit != nil {
			w.emit(level, line)
		}
	}
	w.line, w.oversized = w.line[:0], false
}
