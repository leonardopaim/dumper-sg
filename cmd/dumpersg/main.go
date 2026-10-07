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

func main() {
	if err := run(); err != nil {
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
	handler, err := httpapi.New(repo, manager, httpapi.Options{AllowedHosts: allowedHosts, AllowedOrigins: allowedOrigins, Assets: assets, BackupDir: cfg.BackupDir, Diagnostics: executor.Diagnostics, Importer: repo})
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
	fmt.Printf("DumperSG disponível em http://%s\nDados: %s\nCtrl+C encerra o serviço e cancela a operação ativa.\n", *addr, dataPath)
	select {
	case err = <-finished:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	closeErr := manager.Close(shutdown)
	serverErr := server.Shutdown(shutdown)
	return errors.Join(closeErr, serverErr)
}
func defaultDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "./data"
	}
	return filepath.Join(base, "DumperSG", "web")
}
