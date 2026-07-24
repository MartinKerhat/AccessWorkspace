//go:build !windows

package launcher

import "os/exec"

// hideWindow is a Windows concept (suppressing helper console windows); on
// other platforms child processes have no implicit window to hide.
func hideWindow(command *exec.Cmd) {}
