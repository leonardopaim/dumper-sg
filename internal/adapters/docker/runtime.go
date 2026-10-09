package docker

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"dumpersg/internal/core"
)

type Options struct {
	Runtime      string
	Distribution string
}

// Detect selects a transport once, so create/start/cleanup always reach the same
// daemon. An explicit WSL distribution takes precedence over native discovery.
// Failure to discover a daemon does not prevent opening the local application.
func Detect(ctx context.Context, opts Options) (*Executor, error) {
	return detect(ctx, opts, runtime.GOOS, nil)
}

func detect(ctx context.Context, opts Options, platform string, factory func(context.Context, string, ...string) *exec.Cmd) (*Executor, error) {
	mode := opts.Runtime
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "native" && mode != "wsl" {
		return nil, fmt.Errorf("docker-runtime deve ser auto, native ou wsl")
	}
	if strings.ContainsAny(opts.Distribution, "\x00\r\n") || strings.HasPrefix(opts.Distribution, "-") {
		return nil, fmt.Errorf("distribuição WSL inválida")
	}
	if platform != "windows" && (mode == "wsl" || opts.Distribution != "") {
		return nil, fmt.Errorf("execução via WSL disponível somente no Windows")
	}
	if mode == "native" && opts.Distribution != "" {
		return nil, fmt.Errorf("wsl-distro exige docker-runtime auto ou wsl")
	}
	e := &Executor{runtime: "native", processFactory: factory}
	if mode == "native" || platform != "windows" {
		return e, nil
	}
	if mode == "auto" && opts.Distribution == "" {
		e.autoPending = true
		_, _ = e.inspectDaemon(ctx)
		return e, nil
	}
	e.runtime, e.distribution = "wsl", opts.Distribution
	if e.distribution == "" {
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		name, err := e.invokeHost(probe, "wsl.exe", "--exec", "printenv", "WSL_DISTRO_NAME")
		cancel()
		if err == nil && strings.TrimSpace(name) != "" {
			e.distribution = strings.TrimSpace(name)
		}
	}
	return e, nil
}

func (e *Executor) runtimeDescription() string {
	e.selectionMu.Lock()
	defer e.selectionMu.Unlock()
	if e.runtime == "wsl" {
		if e.distribution != "" {
			return "WSL (" + e.distribution + ")"
		}
		return "WSL (distribuição padrão)"
	}
	return "CLI nativa"
}

func (e *Executor) ExecutionIdentity(ctx context.Context) (string, error) {
	details, err := e.inspectDaemon(ctx)
	if err != nil {
		return "", err
	}
	return e.runtimeDescription() + ":" + details.ID, nil
}

func (e *Executor) verifyIdentity(ctx context.Context, expected string) error {
	if expected == "" {
		return nil
	}
	actual, err := e.ExecutionIdentity(ctx)
	if err != nil || actual != expected {
		return fmt.Errorf("%w: daemon diferente ou indisponível durante a limpeza", core.ErrTerminationUnconfirmed)
	}
	return nil
}

func (e *Executor) wslArgs(program string, args ...string) []string {
	e.selectionMu.Lock()
	defer e.selectionMu.Unlock()
	prefix := []string{}
	if e.distribution != "" {
		prefix = append(prefix, "--distribution", e.distribution)
	}
	prefix = append(prefix, "--exec", program)
	return append(prefix, args...)
}

func (e *Executor) hostCommand(ctx context.Context, program string, args ...string) *exec.Cmd {
	if e.processFactory != nil {
		return e.processFactory(ctx, program, args...)
	}
	return exec.CommandContext(ctx, program, args...)
}

func (e *Executor) invokeHost(ctx context.Context, program string, args ...string) (string, error) {
	process := e.hostCommand(ctx, program, args...)
	var stdout, stderr boundedWriter
	process.Stdout, process.Stderr = &stdout, &stderr
	process.WaitDelay = 3 * time.Second
	err := process.Run()
	return string(stdout.data), err
}

// Bind mounts are interpreted by the Linux daemon, not by the Windows client.
// Ask the selected distribution to convert paths (including custom automount
// roots), and pass each argument directly through --exec without a shell.
func (e *Executor) translateMounts(ctx context.Context, args []string) ([]string, error) {
	if !e.isWSL() {
		return args, nil
	}
	translated := append([]string(nil), args...)
	for i := 0; i < len(translated); i++ {
		if translated[i] != "-v" {
			continue
		}
		if i+1 >= len(translated) {
			return nil, fmt.Errorf("montagem Docker inválida")
		}
		i++
		sep := strings.LastIndex(translated[i], ":/backup")
		if sep < 1 {
			return nil, fmt.Errorf("montagem de backup inválida")
		}
		source, suffix := translated[i][:sep], translated[i][sep:]
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		converted, err := e.invokeHost(probe, "wsl.exe", e.wslArgs("wslpath", "-a", "-u", source)...)
		cancel()
		converted = strings.TrimSpace(converted)
		if err != nil || !strings.HasPrefix(converted, "/") || strings.ContainsAny(converted, "\x00\r\n") {
			return nil, fmt.Errorf("não foi possível converter o diretório de backup para %s", e.runtimeDescription())
		}
		probe, cancel = context.WithTimeout(ctx, 8*time.Second)
		_, err = e.invokeHost(probe, "wsl.exe", e.wslArgs("test", "-d", converted)...)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("diretório de backup não está acessível em %s", e.runtimeDescription())
		}
		translated[i] = converted + suffix
	}
	return translated, nil
}

func (e *Executor) isWSL() bool {
	e.selectionMu.Lock()
	defer e.selectionMu.Unlock()
	return e.runtime == "wsl"
}
