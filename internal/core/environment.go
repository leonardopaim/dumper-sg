package core

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type DirectoryStatus struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
	Message  string `json:"message,omitempty"`
}
type ImageStatus struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
}
type DockerEnvironment struct {
	Available             bool          `json:"available"`
	Runtime               string        `json:"runtime"`
	Distribution          string        `json:"distribution,omitempty"`
	DiscoveryPending      bool          `json:"discovery_pending"`
	Engine                string        `json:"engine,omitempty"`
	Version               string        `json:"version,omitempty"`
	DaemonID              string        `json:"daemon_id,omitempty"`
	DaemonName            string        `json:"daemon_name,omitempty"`
	OperatingSystem       string        `json:"operating_system,omitempty"`
	ContainerOS           string        `json:"container_os,omitempty"`
	Network               string        `json:"network"`
	BackupMount           string        `json:"backup_mount,omitempty"`
	BackupMountAccessible bool          `json:"backup_mount_accessible"`
	Message               string        `json:"message"`
	Images                []ImageStatus `json:"images"`
}
type EnvironmentReport struct {
	CheckedAt    string            `json:"checked_at"`
	Platform     string            `json:"platform"`
	Architecture string            `json:"architecture"`
	GoVersion    string            `json:"go_version"`
	APIAddress   string            `json:"api_address"`
	DatabasePath string            `json:"database_path"`
	Directories  []DirectoryStatus `json:"directories"`
	Docker       DockerEnvironment `json:"docker"`
}

func InspectEnvironment(address, dataDir string, cfg Config) EnvironmentReport {
	result := EnvironmentReport{CheckedAt: time.Now().UTC().Format(time.RFC3339), Platform: runtime.GOOS,
		Architecture: runtime.GOARCH, GoVersion: runtime.Version(), APIAddress: address, DatabasePath: filepath.Join(dataDir, "dumper_sg.sqlite3")}
	for _, directory := range []struct{ name, path string }{{"Dados", dataDir}, {"Logs", cfg.LogDir}, {"Backups", cfg.BackupDir}} {
		status := DirectoryStatus{Name: directory.name, Path: directory.path}
		info, err := os.Stat(directory.path)
		status.Exists = err == nil && info.IsDir()
		if status.Exists {
			_, err = os.ReadDir(directory.path)
			status.Readable = err == nil
			file, writeErr := os.CreateTemp(directory.path, ".dumpersg-environment-*")
			if writeErr == nil {
				closeErr := file.Close()
				removeErr := os.Remove(file.Name())
				status.Writable = closeErr == nil && removeErr == nil
			}
		}
		if !status.Exists {
			status.Message = "Diretório ausente ou inacessível."
		} else if !status.Readable || !status.Writable {
			status.Message = "Verifique as permissões de leitura e escrita."
		}
		result.Directories = append(result.Directories, status)
	}
	return result
}
