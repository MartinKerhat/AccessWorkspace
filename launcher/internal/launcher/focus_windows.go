//go:build windows

package launcher

import (
	"syscall"
	"time"
	"unsafe"
)

func focusLaunchedWindow(pid int) {
	if pid <= 0 {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	isWindowVisible := user32.NewProc("IsWindowVisible")
	getWindowThreadProcessID := user32.NewProc("GetWindowThreadProcessId")
	showWindowAsync := user32.NewProc("ShowWindowAsync")
	setForegroundWindow := user32.NewProc("SetForegroundWindow")

	enumProc := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var windowPID uint32
		getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if int(windowPID) != pid {
			return 1
		}
		visible, _, _ := isWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		showWindowAsync.Call(hwnd, 9)
		setForegroundWindow.Call(hwnd)
		return 0
	})

	for attempt := 0; attempt < 12; attempt++ {
		enumWindows.Call(enumProc, 0)
		time.Sleep(400 * time.Millisecond)
	}
}
