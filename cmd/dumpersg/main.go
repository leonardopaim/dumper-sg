package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"dumpersg/internal/adapters/docker"
	httpapi "dumpersg/internal/adapters/http"
	"dumpersg/internal/adapters/sqlite"
	"dumpersg/internal/core"
	"dumpersg/internal/jobs"
	"dumpersg/web"
)

var errRestart = errors.New("reinício solicitado")

const restartExitCode = 75

func main() {
	if err := run(); err != nil {
		if errors.Is(err, errRestart) {
			os.Exit(restartExitCode)
		}
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	addr := flag.String("addr", "127.0.0.1:8787", "endereço HTTP local (somente loopback)")
	data := flag.String("data-dir", defaultDataDir(), "diretório de dados da aplicação web")
	origins := flag.String("origins", "http://127.0.0.1:5173,http://localhost:5173", "origens adicionais exatas para frontends de desenvolvimento")
	image := flag.String("docker-image", "mydumper/mydumper:latest", "imagem MyDumper/MyLoader")
	mysql := flag.String("mysql-image", "mysql:8.4", "imagem cliente MySQL")
	hostNetwork := flag.Bool("host-network", true, "usar rede host do Docker (como o legado)")
	dockerRuntime := flag.String("docker-runtime", "auto", "execução Docker: auto, native ou wsl")
	wslDistro := flag.String("wsl-distro", "", "distribuição WSL do Docker (padrão do WSL quando omitida)")
	restartEnabled := flag.Bool("restart-enabled", false, "habilitar reinício quando iniciado por um launcher supervisor")
	shutdownEnabled := flag.Bool("shutdown-enabled", false, "habilitar encerramento ocioso para o launcher instalado")
	noTray := flag.Bool("no-tray", false, "não mostrar ícone na bandeja do Windows")
	instanceID := flag.String("instance-id", "", "identificação pública definida pelo launcher")
	flag.Parse()
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("endereço inválido: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("o servidor deve usar um IP loopback explícito, por exemplo 127.0.0.1:8787")
	}
	dataPath, err := filepath.Abs(*data)
	if err != nil {
		return err
	}
	cfg := core.Config{BackupDir: filepath.Join(dataPath, "backups"), LogDir: filepath.Join(dataPath, "logs"), DockerImage: *image, MySQLImage: *mysql, UseHostNetwork: *hostNetwork}
	for _, dir := range []string{dataPath, cfg.BackupDir, cfg.LogDir} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	repo, err := sqlite.Open(filepath.Join(dataPath, "dumper_sg.sqlite3"))
	if err != nil {
		return err
	}
	defer repo.Close()
	executor, err := docker.Detect(context.Background(), docker.Options{Runtime: *dockerRuntime, Distribution: *wslDistro})
	if err != nil {
		return err
	}
	manager := jobs.New(repo, executor, cfg)
	assets, err := web.FS()
	if err != nil {
		return fmt.Errorf("frontend: %w", err)
	}
	allowedHosts := []string{*addr, net.JoinHostPort("localhost", port)}
	allowedOrigins := []string{"http://" + *addr, "http://" + net.JoinHostPort("localhost", port)}
	for _, origin := range strings.Split(*origins, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowedOrigins = append(allowedOrigins, origin)
		}
	}
	restartRequested := make(chan struct{}, 1)
	shutdownRequested := make(chan struct{}, 1)
	options := httpapi.Options{AllowedHosts: allowedHosts, AllowedOrigins: allowedOrigins, Assets: assets, BackupDir: cfg.BackupDir, Diagnostics: executor.Diagnostics, Importer: repo, InstanceID: *instanceID}
	if *restartEnabled {
		options.Restart = func(ctx context.Context) error {
			if err := manager.PrepareRestart(ctx); err != nil {
				return err
			}
			restartRequested <- struct{}{}
			return nil
		}
	}
	requestShutdown := func(ctx context.Context) error {
		return prepareIdleShutdown(ctx, manager, shutdownRequested)
	}
	if *shutdownEnabled {
		options.Shutdown = requestShutdown
	}
	handler, err := httpapi.New(repo, manager, options)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("não foi possível iniciar em %s: %w", *addr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	if !*noTray {
		closeTray, trayErr := startTray("http://"+*addr, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return requestShutdown(ctx)
		})
		if trayErr != nil {
			log.Print("Ícone da bandeja indisponível: ", trayErr)
		} else {
			defer closeTray()
		}
	}
	fmt.Printf("DumperSG disponível em http://%s\nDados: %s\nCtrl+C encerra o serviço e cancela a operação ativa.\n", *addr, dataPath)
	restarting := false
	requestedShutdown := false
	select {
	case err = <-finished:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	case <-restartRequested:
		restarting = true
	case <-shutdownRequested:
		requestedShutdown = true
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	closeErr := manager.Close(shutdown)
	serverErr := server.Shutdown(shutdown)
	// Restarting the HTTP backend does not terminate unconfirmed containers.
	// Their persisted guards are recovered by the next manager instance.
	if (restarting || requestedShutdown) && errors.Is(closeErr, core.ErrTerminationUnconfirmed) {
		log.Print("Encerramento mantendo pendências de containers: ", closeErr)
		closeErr = nil
	}
	if err := errors.Join(closeErr, serverErr); err != nil {
		return err
	}
	if restarting {
		return errRestart
	}
	return nil
}
func defaultDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "./data"
	}
	return filepath.Join(base, "DumperSG", "web")
}

func prepareIdleShutdown(ctx context.Context, manager *jobs.Manager, requested chan<- struct{}) error {
	if err := manager.PrepareRestart(ctx); err != nil {
		if errors.Is(err, core.ErrConflict) {
			return fmt.Errorf("conclua ou cancele a operação em andamento antes de encerrar; verifique também se um reinício ou encerramento já foi solicitado: %w", core.ErrConflict)
		}
		return err
	}
	requested <- struct{}{}
	return nil
}
