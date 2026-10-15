package docker

import (
	"context"
	"dumpersg/internal/core"
	"strings"
	"time"
)

func (e *Executor) Environment(ctx context.Context, cfg core.Config) core.DockerEnvironment {
	result := core.DockerEnvironment{Images: []core.ImageStatus{}}
	details, daemonErr := e.inspectDaemon(ctx)
	e.selectionMu.Lock()
	result.Runtime, result.Distribution, result.DiscoveryPending = e.runtime, e.distribution, e.autoPending
	desktop := e.desktop
	e.selectionMu.Unlock()
	result.Network = "Bridge padrão do Docker"
	if cfg.UseHostNetwork {
		result.Network = "Rede host do Docker Engine"
	}
	if desktop {
		result.Network = "Bridge do Docker Desktop; conexões locais usam host.docker.internal"
	}
	if result.DiscoveryPending {
		result.Network = "A confirmar após detectar o Docker"
	}
	if daemonErr != nil {
		result.Message = daemonErr.Error()
	} else {
		result.Available, result.DaemonID, result.DaemonName = true, details.ID, details.Name
		result.OperatingSystem, result.ContainerOS = details.OperatingSystem, details.OSType
		result.Engine = "Docker Engine"
		if desktop {
			result.Engine = "Docker Desktop"
		}
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		version, err := e.invoke(probe, "version", "--format", "{{.Server.Version}}")
		cancel()
		if err == nil {
			result.Version = strings.TrimSpace(version)
		}
		result.Message = result.Engine + " disponível via " + e.runtimeDescription() + "."
		directory, err := core.ExistingDirectory(cfg.BackupDir)
		var args []string
		if err == nil {
			args, err = e.translateMounts(ctx, []string{"-v", directory + ":/backup"})
		}
		if err == nil {
			result.BackupMount = args[1]
			result.BackupMountAccessible = true
		} else {
			result.Message += " " + err.Error()
		}
	}
	dumper, mysql := cfg.DockerImage, cfg.MySQLImage
	if dumper == "" {
		dumper = "mydumper/mydumper:latest"
	}
	if mysql == "" {
		mysql = "mysql:8.4"
	}
	for _, image := range []string{dumper, mysql} {
		status := core.ImageStatus{Name: image, Message: "Não verificada: Docker indisponível."}
		if result.Available {
			probe, cancel := context.WithTimeout(ctx, 3*time.Second)
			id, err := e.invoke(probe, "image", "inspect", "--format", "{{.Id}}", image)
			cancel()
			if err == nil {
				status.Present, status.ID, status.Message = true, strings.TrimSpace(id), "Disponível no daemon selecionado."
			} else {
				status.Message = "Não localizada no daemon; será verificada e baixada ao iniciar uma operação."
			}
		}
		result.Images = append(result.Images, status)
	}
	return result
}
