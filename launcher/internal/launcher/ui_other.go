//go:build !windows && !linux

package launcher

func showLauncherMessageBox(message string, title string) {}
