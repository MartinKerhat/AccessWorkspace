//go:build windows

package launcher

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps helper processes (cmdkey, certutil, powershell) from
// flashing a console window. Windows-only concept; a no-op elsewhere.
func hideWindow(command *exec.Cmd) {
	if command == nil {
		return
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
