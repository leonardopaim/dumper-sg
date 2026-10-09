//go:build windows

package main

import (
	"errors"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsTrayNativeLifecycle(t *testing.T) {
	class, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	explorer, _, _ := trayUser32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), 0)
	if explorer == 0 {
		t.Skip("Windows notification area unavailable in this session")
	}
	opened := make(chan struct{}, 1)
	exited := make(chan struct{}, 1)
	tray, err := newWindowsTray(func() error { opened <- struct{}{}; return nil }, func() error {
		exited <- struct{}{}
		return errors.New("Operação em andamento: encerramento bloqueado (teste)")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tray.close)
	post := func(command uintptr, signal <-chan struct{}) {
		t.Helper()
		result, _, err := trayUser32.NewProc("PostMessageW").Call(tray.window, wmCommand, command, 0)
		if result == 0 {
			t.Fatal(err)
		}
		select {
		case <-signal:
		case <-time.After(3 * time.Second):
			t.Fatal("tray action did not run")
		}
		deadline := time.Now().Add(3 * time.Second)
		for tray.actionBusy.Load() {
			if time.Now().After(deadline) {
				t.Fatal("tray action remained busy")
			}
			time.Sleep(time.Millisecond)
		}
	}
	post(trayExit, exited)
	select {
	case <-tray.done:
		t.Fatal("rejected exit removed the tray")
	default:
	}
	post(trayOpen, opened)
	tray.close()
	tray.close()
	select {
	case <-tray.done:
	default:
		t.Fatal("closing did not stop the native message loop")
	}
	if alive, _, _ := trayUser32.NewProc("IsWindow").Call(tray.window); alive != 0 {
		t.Fatal("tray left a native window behind")
	}
}
