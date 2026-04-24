//go:build windows

package app

import (
	"context"
	"syscall"
	"unsafe"
)

var (
	_user32   = syscall.NewLazyDLL("user32.dll")
	_kernel32 = syscall.NewLazyDLL("kernel32.dll")

	_loadImage       = _user32.NewProc("LoadImageW")
	_sendMessage     = _user32.NewProc("SendMessageW")
	_findWindow      = _user32.NewProc("FindWindowW")
	_getModuleHandle = _kernel32.NewProc("GetModuleHandleW")
)

// SetWindowIcon applies the executable's embedded icon (resource ID 1, written
// by rsrc_windows_amd64.syso via go-winres) to the Wails main window via
// WM_SETICON. Wails' WebView2 host registers a window class with a null hIcon,
// so Windows falls back to the system default for the title bar even though the
// PE resource is correct. Sending WM_SETICON from OnDomReady fixes this.
func (a *App) SetWindowIcon(_ context.Context) {
	hInst, _, _ := _getModuleHandle.Call(0)
	if hInst == 0 {
		return
	}

	const (
		imageIcon = 1    // IMAGE_ICON
		wmSetIcon = 0x0080
		iconSmall = 0    // ICON_SMALL — title bar
		iconBig   = 1    // ICON_BIG   — taskbar / Alt+Tab
	)

	// Resource ID 1 is the group icon created by "go-winres simply".
	// Passing uintptr(1) is equivalent to MAKEINTRESOURCE(1) in Win32.
	hBig, _, _   := _loadImage.Call(hInst, 1, imageIcon, 32, 32, 0)
	hSmall, _, _ := _loadImage.Call(hInst, 1, imageIcon, 16, 16, 0)

	titlePtr, _ := syscall.UTF16PtrFromString("TokenTally")
	hwnd, _, _  := _findWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}

	if hSmall != 0 {
		_sendMessage.Call(hwnd, wmSetIcon, iconSmall, hSmall)
	}
	if hBig != 0 {
		_sendMessage.Call(hwnd, wmSetIcon, iconBig, hBig)
	}
}
