//go:build !windows && !linux && !darwin

package launcher

func showLauncherMessageBox(message string, title string) {}
