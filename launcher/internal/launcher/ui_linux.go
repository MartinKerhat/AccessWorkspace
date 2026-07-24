//go:build linux

package launcher

import "os/exec"

// showLauncherMessageBox surfaces a launch failure to the user. Best effort:
// zenity (GTK dialog) if present, then a desktop notification; the log
// remains the source of truth either way.
func showLauncherMessageBox(message string, title string) {
	if path, err := exec.LookPath("zenity"); err == nil {
		if err := exec.Command(path, "--error", "--title", title, "--text", message).Start(); err == nil {
			return
		}
	}
	if path, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command(path, "--urgency=critical", title, message).Start()
	}
}
