//go:build windows

package main

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed tray.ico
var trayIconBytes []byte

var trayUser32 = windows.NewLazySystemDLL("user32.dll")
var trayShell32 = windows.NewLazySystemDLL("shell32.dll")
var traySequence atomic.Uint64

const (
	trayCallback = 0x8001
	trayNotice   = 0x8002
	trayOpen     = 1
	trayExit     = 2
	wmClose      = 0x0010
	wmDestroy    = 0x0002
	wmCommand    = 0x0111
)

// Win32 layouts use pointer-sized HWND/HICON fields and Windows UTF-16 strings.
type trayNotifyIcon struct {
	Size            uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            windows.GUID
	BalloonIcon     uintptr
}

type trayWindowClass struct {
	Size, Style                   uint32
	Procedure                     uintptr
	ClassExtra, WindowExtra       int32
	Instance, Icon, Cursor, Brush uintptr
	MenuName, ClassName           *uint16
	SmallIcon                     uintptr
}

type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          trayPoint
	Private        uint32
}

type windowsTray struct {
	window         uintptr
	icon           trayNotifyIcon
	taskbarCreated uint32
	open, exit     func() error
	notices        chan error
	done           chan struct{}
	closing        sync.Once
	actionBusy     atomic.Bool
}

func startTray(url string, exit func() error) (func(), error) {
	tray, err := newWindowsTray(func() error { return openTrayBrowser(url) }, exit)
	if err != nil {
		return nil, err
	}
	return tray.close, nil
}

func newWindowsTray(open, exit func() error) (*windowsTray, error) {
	t := &windowsTray{open: open, exit: exit, notices: make(chan error, 1), done: make(chan struct{})}
	ready := make(chan error, 1)
	go t.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return t, nil
}

func (t *windowsTray) run(ready chan<- error) {
	// The hidden window and its message pump must remain on the creating thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(t.done)
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	className, _ := windows.UTF16PtrFromString(fmt.Sprintf("DumperSG.Tray.%d.%d", os.Getpid(), traySequence.Add(1)))
	wc := trayWindowClass{Instance: instance, ClassName: className, Procedure: syscall.NewCallback(t.windowProcedure)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	registered, _, err := trayUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if registered == 0 {
		ready <- fmt.Errorf("registrar ícone na bandeja: %v", err)
		return
	}
	defer trayUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
	t.window, _, err = trayUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if t.window == 0 {
		ready <- fmt.Errorf("criar janela da bandeja: %v", err)
		return
	}
	defer trayUser32.NewProc("DestroyWindow").Call(t.window)
	icon, err := loadTrayIcon()
	if err != nil {
		ready <- err
		return
	}
	defer trayUser32.NewProc("DestroyIcon").Call(icon)
	t.icon = trayNotifyIcon{Window: t.window, ID: 1, Flags: 0x87, CallbackMessage: trayCallback, Icon: icon}
	t.icon.Size = uint32(unsafe.Sizeof(t.icon))
	copy(t.icon.Tip[:], windows.StringToUTF16("DumperSG — clique para abrir; botão direito para encerrar"))
	message, _ := windows.UTF16PtrFromString("TaskbarCreated")
	created, _, _ := trayUser32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(message)))
	t.taskbarCreated = uint32(created)
	if err := t.addIcon(); err != nil {
		ready <- err
		return
	}
	defer trayShell32.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&t.icon)))
	ready <- nil
	var msg trayMessage
	for {
		result, _, _ := trayUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		trayUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		trayUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *windowsTray) addIcon() error {
	added, _, _ := trayShell32.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&t.icon)))
	if added == 0 {
		return errors.New("o Windows não conseguiu mostrar o ícone na bandeja")
	}
	t.icon.Version = 4
	versioned, _, _ := trayShell32.NewProc("Shell_NotifyIconW").Call(4, uintptr(unsafe.Pointer(&t.icon)))
	if versioned == 0 {
		trayShell32.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&t.icon)))
		return errors.New("o Windows não conseguiu configurar o menu da bandeja")
	}
	return nil
}

func (t *windowsTray) windowProcedure(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	if t.taskbarCreated != 0 && message == t.taskbarCreated {
		_ = t.addIcon() // Explorer recreated the notification area.
		return 0
	}
	switch message {
	case trayCallback:
		switch uint16(lparam) {
		case 0x0400, 0x0401: // NIN_SELECT, NIN_KEYSELECT (NOTIFYICON_VERSION_4).
			t.dispatch(trayOpen)
		case 0x007b: // WM_CONTEXTMENU: coordinates also work for keyboard activation.
			t.menu(trayPoint{X: int32(int16(uint16(wparam))), Y: int32(int16(uint16(wparam >> 16)))})
		}
		return 0
	case wmCommand:
		t.dispatch(uint16(wparam))
		return 0
	case trayNotice:
		select {
		case err := <-t.notices:
			notice := t.icon
			notice.Flags, notice.InfoFlags = 0x10, 0x12 // Warning, without sound.
			copy(notice.InfoTitle[:], windows.StringToUTF16("DumperSG"))
			text := windows.StringToUTF16(err.Error())
			if len(text) > len(notice.Info) {
				text = text[:len(notice.Info)-1]
			}
			copy(notice.Info[:], text)
			trayShell32.NewProc("Shell_NotifyIconW").Call(1, uintptr(unsafe.Pointer(&notice)))
		default:
		}
		return 0
	case wmClose:
		trayUser32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		trayUser32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := trayUser32.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wparam, lparam)
	return result
}

func (t *windowsTray) menu(point trayPoint) {
	menu, _, _ := trayUser32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer trayUser32.NewProc("DestroyMenu").Call(menu)
	for _, item := range []struct {
		id    uintptr
		label string
	}{{trayOpen, "Abrir DumperSG"}, {trayExit, "Encerrar DumperSG"}} {
		label, _ := windows.UTF16PtrFromString(item.label)
		trayUser32.NewProc("AppendMenuW").Call(menu, 0, item.id, uintptr(unsafe.Pointer(label)))
	}
	trayUser32.NewProc("SetForegroundWindow").Call(t.window)
	selected, _, _ := trayUser32.NewProc("TrackPopupMenu").Call(menu, 0x182, uintptr(point.X), uintptr(point.Y), 0, t.window, 0)
	trayUser32.NewProc("PostMessageW").Call(t.window, 0, 0, 0)
	t.dispatch(uint16(selected))
}

func (t *windowsTray) dispatch(command uint16) {
	var action func() error
	switch command {
	case trayOpen:
		action = t.open
	case trayExit:
		action = t.exit
	default:
		return
	}
	if !t.actionBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer t.actionBusy.Store(false)
		if err := action(); err != nil {
			select {
			case <-t.done:
				return
			default:
			}
			select {
			case t.notices <- err:
			default:
			}
			trayUser32.NewProc("PostMessageW").Call(t.window, trayNotice, 0, 0)
		}
	}()
}

func (t *windowsTray) close() {
	t.closing.Do(func() { trayUser32.NewProc("PostMessageW").Call(t.window, wmClose, 0, 0) })
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
	}
}

func openTrayBrowser(url string) error {
	address, _ := windows.UTF16PtrFromString(url)
	verb, _ := windows.UTF16PtrFromString("open")
	result, _, _ := trayShell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(address)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("o Windows não conseguiu abrir o navegador. Abra %s", url)
	}
	return nil
}

func loadTrayIcon() (uintptr, error) {
	if len(trayIconBytes) < 6 {
		return 0, errors.New("ícone da bandeja inválido")
	}
	count := int(binary.LittleEndian.Uint16(trayIconBytes[4:6]))
	for i := 0; i < count; i++ {
		entry := 6 + i*16
		if entry+16 > len(trayIconBytes) {
			break
		}
		if trayIconBytes[entry] != 32 || trayIconBytes[entry+1] != 32 {
			continue
		}
		size := int(binary.LittleEndian.Uint32(trayIconBytes[entry+8:]))
		offset := int(binary.LittleEndian.Uint32(trayIconBytes[entry+12:]))
		if size <= 0 || offset < 0 || offset > len(trayIconBytes) || size > len(trayIconBytes)-offset {
			break
		}
		// RT_ICON bits must be DWORD-aligned; ICO directory offsets need not be.
		resource := make([]uint32, (size+3)/4)
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&resource[0])), size), trayIconBytes[offset:offset+size])
		icon, _, err := trayUser32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&resource[0])), uintptr(size), 1, 0x30000, 32, 32, 0)
		runtime.KeepAlive(resource)
		if icon == 0 {
			return 0, fmt.Errorf("carregar ícone da bandeja: %v", err)
		}
		return icon, nil
	}
	return 0, errors.New("ícone da bandeja sem imagem de 32 pixels")
}
