//go:build darwin

package launcher

import "os/exec"

// Capabilities reports what this launcher build supports on this machine.
// SSH runs in Terminal.app, which ships with macOS, next to the system
// OpenSSH client. RDP waits for the bundled FreeRDP client.
func Capabilities() map[string]any {
	_, sshErr := exec.LookPath("ssh")
	return map[string]any{
		"ssh":                  sshErr == nil,
		"sshCredentialHandoff": sshErr == nil,
		"rdp":                  false,
		"rdpCredentialHandoff": false,
		"rdpGateway":           false,
		"webPortal":            true,
	}
}
