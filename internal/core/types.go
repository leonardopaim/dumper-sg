package core

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("registro não encontrado")
var ErrConflict = errors.New("operação em conflito")
var ErrTerminationUnconfirmed = errors.New("término do container não confirmado")

type Profile struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	User         string        `json:"user"`
	Password     string        `json:"password,omitempty"`
	Database     string        `json:"database"`
	SSL          bool          `json:"ssl"`
	Threads      int           `json:"threads"`
	HasPassword  bool          `json:"has_password"`
	TablePresets []TablePreset `json:"table_presets"`
}

type TablePreset struct {
	Name     string   `json:"name"`
	Database string   `json:"database"`
	Tables   []string `json:"tables"`
}

type TableReference struct {
	Database string `json:"database"`
	Name     string `json:"name"`
}

type BackupTableInfo struct {
	fileDatabase string
	fileTable    string
	Database     string `json:"database"`
	Name         string `json:"name"`
	SizeBytes    uint64 `json:"size_bytes"`
	Rows         uint64 `json:"rows"`
	TableType    string `json:"table_type"`
}

func (p Profile) Public() Profile {
	p.HasPassword = p.Password != ""
	p.Password = ""
	if p.TablePresets == nil {
		p.TablePresets = []TablePreset{}
	}
	return p
}

type BackupRequest struct {
	ProfileID      int64    `json:"profile_id"`
	Database       string   `json:"database"`
	DestinationDir string   `json:"destination_dir"`
	Threads        int      `json:"threads"`
	Compress       bool     `json:"compress"`
	SSL            bool     `json:"ssl"`
	NonLocking     *bool    `json:"non_locking,omitempty"`
	IgnoreRegex    string   `json:"ignore_regex"`
	Tables         []string `json:"tables"`
}
type TableListRequest struct {
	ProfileID int64  `json:"profile_id"`
	Database  string `json:"database"`
	SSL       bool   `json:"ssl"`
}
type TableInfo struct {
	Name      string `json:"name"`
	SizeBytes uint64 `json:"size_bytes"`
	Rows      uint64 `json:"rows"`
	TableType string `json:"table_type"`
}
type RestoreRequest struct {
	ProfileID       int64            `json:"profile_id"`
	BackupDir       string           `json:"backup_dir"`
	TargetDatabase  string           `json:"target_database"`
	Threads         int              `json:"threads"`
	OverwriteTables bool             `json:"overwrite_tables"`
	Tables          []TableReference `json:"tables"`
}
type DatabaseRequest struct {
	ProfileID int64  `json:"profile_id"`
	Database  string `json:"database"`
}
type Job struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	ProfileID       int64  `json:"profile_id"`
	ProfileName     string `json:"profile_name"`
	Database        string `json:"database"`
	Path            string `json:"path"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
	Progress        int    `json:"progress"`
	Message         string `json:"message"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	CleanupRequired bool   `json:"cleanup_required,omitempty"`
	DockerIdentity  string `json:"docker_identity,omitempty"`
}
type Event struct {
	Sequence int64  `json:"sequence"`
	Time     string `json:"time"`
	Level    string `json:"level"`
	Message  string `json:"message"`
}
type Command struct {
	// StdoutData routes structured output to the collector; never to job logs.
	StdoutData     bool
	Program        string
	Image          string
	Args           []string
	Secrets        []string
	ContainerName  string
	DockerIdentity string
}
type Diagnostics struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Message   string `json:"message"`
}
type Config struct {
	BackupDir      string
	LogDir         string
	DockerImage    string
	MySQLImage     string
	UseHostNetwork bool
}
type Repository interface {
	ListProfiles(context.Context) ([]Profile, error)
	GetProfile(context.Context, int64) (Profile, error)
	SaveProfile(context.Context, Profile) (Profile, error)
	DeleteProfile(context.Context, int64) error
	GetSettings(context.Context) (map[string]string, error)
	SaveSettings(context.Context, map[string]string) error
	SaveJob(context.Context, Job) error
	GetJob(context.Context, string) (Job, error)
	ListJobs(context.Context, int) ([]Job, error)
	BackupJobs(context.Context) ([]Job, error)
	PendingJobs(context.Context) ([]Job, error)
}
type Executor interface {
	Run(context.Context, Command, func(string, string)) error
}

type TerminationVerifier interface {
	VerifyTermination(context.Context, Command) error
}

type ExecutionIdentityProvider interface {
	ExecutionIdentity(context.Context) (string, error)
}
