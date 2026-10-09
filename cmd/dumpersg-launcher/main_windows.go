//go:build windows

// The installed launcher keeps the local core running without a console window.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const installedMutex = `Local\DumperSG.Installed.Running`
const dumperImage = "mydumper/mydumper@sha256:ed4257555d34ed1a92287838c1a300afa9358d8325d2aa98ca2a393c3f0f43c3"

type launchState struct {
	Instance string `json:"instance_id"`
	Address  string `json:"address"`
	DataDir  string `json:"data_dir"`
	Core     string `json:"core_executable"`
}

var client = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}

func main() {
	background := flag.Bool("background", false, "não abrir navegador")
	stop := flag.Bool("shutdown", false, "encerrar esta instalação quando ociosa")
	quiet := flag.Bool("quiet", false, "registrar erros sem abrir mensagem")
	address := flag.String("addr", "127.0.0.1:8787", "endereço HTTP local")
	config, err := os.UserConfigDir()
	if err != nil {
		report(err, *quiet)
		return
	}
	defaultData := filepath.Join(config, "DumperSG", "web")
	data := flag.String("data-dir", defaultData, "dados locais")
	flag.Parse()
	if err := launch(*address, *data, defaultData, *background, *stop); err != nil {
		logLifecycle(*data, "launcher: "+err.Error())
		report(err, *quiet)
		os.Exit(1)
	}
}

func report(err error, quiet bool) {
	fmt.Fprintln(os.Stderr, err)
	if quiet {
		return
	}
	text, _ := windows.UTF16PtrFromString("DumperSG\n\n" + err.Error())
	title, _ := windows.UTF16PtrFromString("DumperSG")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}

func localBase(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || port == "" {
		return "", errors.New("O endereço precisa ser um IP local explícito.")
	}
	return "http://" + address, nil
}

func readJSON(base, path string, out any) error {
	res, err := client.Get(base + "/api/v1" + path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("API local retornou %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(out)
}

func application(base string) (string, error) {
	var v struct {
		Instance string `json:"instance_id"`
	}
	if err := readJSON(base, "/application", &v); err != nil {
		return "", err
	}
	if len(v.Instance) != 32 || strings.Trim(v.Instance, "0123456789abcdef") != "" {
		return "", errors.New("A porta está em uso por outra aplicação.")
	}
	return v.Instance, nil
}

func mutate(base, path string) error {
	var session struct {
		Token string `json:"token"`
	}
	if err := readJSON(base, "/session", &session); err != nil {
		return err
	}
	if len(session.Token) != 64 {
		return errors.New("Sessão local inválida.")
	}
	req, err := http.NewRequest("POST", base+"/api/v1"+path, bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DumperSG-Token", session.Token)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		var v struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&v)
		if v.Error == "" {
			v.Error = fmt.Sprintf("API local retornou %d", res.StatusCode)
		}
		return errors.New(v.Error)
	}
	return nil
}

func openBrowser(base string) error {
	url, _ := windows.UTF16PtrFromString(base)
	verb, _ := windows.UTF16PtrFromString("open")
	result, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(url)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("Abra %s no navegador. O Windows não conseguiu abrir o navegador padrão.", base)
	}
	return nil
}

func launch(address, data, defaultData string, background, stop bool) error {
	base, err := localBase(address)
	if err != nil {
		return err
	}
	data, err = filepath.Abs(data)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	core := filepath.Join(filepath.Dir(executable), "dumpersg-core.exe")
	statePath := filepath.Join(data, "installed-launcher", "process.json")
	if stop {
		return shutdownInstalled(base, statePath, core, data)
	}
	name := installedMutex
	if address != "127.0.0.1:8787" || !strings.EqualFold(filepath.Clean(data), filepath.Clean(defaultData)) {
		name = fmt.Sprintf(`Local\DumperSG.Installed.%x`, sha256.Sum256([]byte(strings.ToLower(data)+address)))
	}
	mutexName, _ := windows.UTF16PtrFromString(name)
	handle, mutexErr := windows.CreateMutex(nil, false, mutexName)
	if handle != 0 {
		defer windows.CloseHandle(handle)
	}
	if mutexErr != nil && !errors.Is(mutexErr, windows.ERROR_ALREADY_EXISTS) {
		return mutexErr
	}
	if errors.Is(mutexErr, windows.ERROR_ALREADY_EXISTS) {
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := application(base); err == nil {
				if !background {
					return openBrowser(base)
				}
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		return errors.New("A aplicação já está iniciando, mas não respondeu. Confira os logs de inicialização.")
	}
	// A development checkout may already serve this user's data. Reuse its UI.
	if _, err := application(base); err == nil {
		if !background {
			return openBrowser(base)
		}
		return nil
	}
	if _, err := os.Stat(core); err != nil {
		return errors.New("O pacote está incompleto. Reinstale o DumperSG.")
	}
	logDir := filepath.Dir(statePath)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	for {
		stdout, err := startupLog(logDir, "stdout.log")
		if err != nil {
			return err
		}
		stderr, err := startupLog(logDir, "stderr.log")
		if err != nil {
			stdout.Close()
			return err
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			stdout.Close()
			stderr.Close()
			return err
		}
		expected := hex.EncodeToString(nonce[:])
		cmd := exec.Command(core, "-addr", address, "-data-dir", data, "-docker-runtime", "auto", "-mysql-image", "mysql:8.4.3", "-docker-image", dumperImage, "-restart-enabled", "-shutdown-enabled", "-instance-id", expected)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		if err := cmd.Start(); err != nil {
			stdout.Close()
			stderr.Close()
			return err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		instance, readyErr := waitForCore(base, expected, done)
		if readyErr != nil {
			if exit, ok := readyErr.(*exec.ExitError); ok && exit.ExitCode() == 75 {
				stdout.Close()
				stderr.Close()
				continue // A previously open browser may request restart before readiness is recorded.
			}
			// Only terminate the child we just created, which never became ready.
			_ = cmd.Process.Kill()
			stdout.Close()
			stderr.Close()
			return fmt.Errorf("Não foi possível iniciar. Veja %s. %w", logDir, readyErr)
		}
		state := launchState{Instance: instance, Address: address, DataDir: data, Core: core}
		if err := writeState(statePath, state); err != nil {
			if stopErr := mutate(base, "/application/shutdown"); stopErr != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
			}
			stdout.Close()
			stderr.Close()
			return err
		}
		if !background {
			if err := openBrowser(base); err != nil {
				report(err, false)
			}
			background = true
		}
		waitErr := <-done
		logLifecycle(data, fmt.Sprintf("core pid=%d exit=%d error=%v", cmd.Process.Pid, cmd.ProcessState.ExitCode(), waitErr))
		stdout.Close()
		stderr.Close()
		if exit, ok := waitErr.(*exec.ExitError); ok && exit.ExitCode() == 75 {
			continue
		}
		_ = os.Remove(statePath)
		if waitErr != nil {
			return fmt.Errorf("A aplicação encerrou. Confira %s: %w", logDir, waitErr)
		}
		return nil
	}
}

func logLifecycle(data, message string) {
	dir := filepath.Join(data, "installed-launcher")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "launcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), message)
}

func waitForCore(base, expected string, done <-chan error) (string, error) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			if err == nil {
				err = errors.New("Processo encerrado antes da inicialização.")
			}
			return "", err
		default:
		}
		if instance, err := application(base); err == nil && instance == expected {
			return instance, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", errors.New("Tempo de inicialização excedido; verifique se a porta já está em uso.")
}

func startupLog(dir, name string) (*os.File, error) {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, path+".previous"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
}

func writeState(path string, state launchState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func shutdownInstalled(base, path, core, data string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return errors.New("Esta instalação não tem uma aplicação em execução registrada.")
	}
	var state launchState
	if err := json.Unmarshal(contents, &state); err != nil {
		return err
	}
	if !strings.EqualFold(state.Core, core) || !strings.EqualFold(state.DataDir, data) || "http://"+state.Address != base {
		return errors.New("Outra instalação está usando os dados. Encerramento recusado.")
	}
	instance, err := application(base)
	if err != nil {
		return err
	}
	if instance != state.Instance {
		return errors.New("A instância mudou. Aguarde a inicialização e tente novamente.")
	}
	if err := mutate(base, "/application/shutdown"); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := application(base); err != nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("O encerramento foi solicitado, mas a aplicação ainda não concluiu a saída.")
}
