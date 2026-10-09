package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const daemonDetailsFormat = `{"id":{{json .ID}},"os_type":{{json .OSType}},"operating_system":{{json .OperatingSystem}},"name":{{json .Name}}}`

var errContainerOS = errors.New("DumperSG exige containers Linux; selecione Linux containers no Docker Desktop")

type daemonDetails struct {
	ID              string `json:"id"`
	OSType          string `json:"os_type"`
	OperatingSystem string `json:"operating_system"`
	Name            string `json:"name"`
}

func (e *Executor) probeDaemon(ctx context.Context) (daemonDetails, error) {
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := e.invoke(probe, "info", "--format", daemonDetailsFormat)
	if err != nil {
		return daemonDetails{}, fmt.Errorf("Docker não respondeu")
	}
	var details daemonDetails
	if err = json.Unmarshal([]byte(data), &details); err != nil || strings.TrimSpace(details.ID) == "" || strings.ContainsAny(details.ID, "\x00\r\n") {
		return daemonDetails{}, fmt.Errorf("Docker retornou identificação inválida")
	}
	if details.OSType != "linux" {
		return daemonDetails{}, errContainerOS
	}
	return details, nil
}

// Discovery can retry while no daemon has been selected. Once a daemon responds,
// its transport stays fixed for execution, cancellation and cleanup.
func (e *Executor) inspectDaemon(ctx context.Context) (daemonDetails, error) {
	e.selectionMu.Lock()
	defer e.selectionMu.Unlock()
	if e.autoPending {
		native := &Executor{runtime: "native", processFactory: e.processFactory}
		details, err := native.probeDaemon(ctx)
		if err == nil {
			e.runtime, e.distribution, e.autoPending = "native", "", false
			e.desktop = isDesktop(details)
			return details, nil
		}
		if errors.Is(err, errContainerOS) {
			return daemonDetails{}, err
		}
		wsl := &Executor{runtime: "wsl", processFactory: e.processFactory}
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		name, nameErr := wsl.invokeHost(probe, "wsl.exe", "--exec", "printenv", "WSL_DISTRO_NAME")
		cancel()
		name = strings.TrimSpace(name)
		if nameErr != nil || name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n") {
			return daemonDetails{}, fmt.Errorf("Docker indisponível. Abra o Docker Desktop ou inicie o Docker Engine no WSL e tente novamente")
		}
		wsl.distribution = name
		details, err = wsl.probeDaemon(ctx)
		if err != nil {
			return daemonDetails{}, fmt.Errorf("Docker indisponível. Abra o Docker Desktop ou inicie o Docker Engine no WSL (%s) e tente novamente", name)
		}
		e.runtime, e.distribution, e.autoPending = "wsl", name, false
		e.desktop = isDesktop(details)
		return details, nil
	}
	if e.runtime == "wsl" && e.distribution == "" {
		return daemonDetails{}, fmt.Errorf("distribuição WSL não identificada; reinicie com -wsl-distro explícito")
	}
	selected := &Executor{runtime: e.runtime, distribution: e.distribution, processFactory: e.processFactory, commandFactory: e.commandFactory}
	details, err := selected.probeDaemon(ctx)
	if err == nil {
		e.desktop = isDesktop(details)
	}
	return details, err
}

func isDesktop(details daemonDetails) bool {
	return strings.Contains(strings.ToLower(details.OperatingSystem), "docker desktop") || strings.EqualFold(details.Name, "docker-desktop")
}

func (e *Executor) desktopNetworking(args []string) []string {
	e.selectionMu.Lock()
	desktop := e.desktop
	e.selectionMu.Unlock()
	if !desktop {
		return args
	}
	result := make([]string, 0, len(args))
	imageSeen := false
	// Core commands put the image after Docker options. Stop treating arguments
	// as Docker flags there so SQL, passwords and tool options stay literal.
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !imageSeen && arg == "--network" && i+1 < len(args) && args[i+1] == "host" {
			i++
			continue
		}
		if !imageSeen && arg == "--network=host" {
			continue
		}
		if imageSeen && strings.HasPrefix(arg, "--host=") {
			host := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--host=")))
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				arg = "--host=host.docker.internal"
			}
		}
		result = append(result, arg)
		if i == 0 {
			continue
		} // create/run verb
		if !imageSeen {
			switch arg {
			case "--name", "-v", "--label", "--network":
				if i+1 < len(args) {
					i++
					result = append(result, args[i])
				}
			case "--rm":
			default:
				if !strings.HasPrefix(arg, "-") {
					imageSeen = true
				}
			}
		}
	}
	return result
}
