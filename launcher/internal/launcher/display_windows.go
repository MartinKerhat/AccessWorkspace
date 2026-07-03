//go:build windows

package launcher

import "syscall"

const (
	smCxScreen = 0
	smCyScreen = 1
)

func currentDisplaySize() (int, int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	width, _, _ := getSystemMetrics.Call(smCxScreen)
	height, _, _ := getSystemMetrics.Call(smCyScreen)
	return int(width), int(height)
}
