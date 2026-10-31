//go:build darwin

package launcher

import "os/exec"

// Capabilities reports what this launcher build supports on this machine.
// SSH runs in Terminal.app next to the system OpenSSH client, both part of
// macOS. RDP uses the FreeRDP client bundled inside the launcher .app, so it
// is only ever missing from a broken or development build.
func Capabilities() map[string]any {
	_, sshErr := exec.LookPath("ssh")
	rdpClient, rdpErr := findFreeRDPClient()
	capabilities := map[string]any{
		"ssh":                  sshErr == nil,
		"sshCredentialHandoff": sshErr == nil,
		"rdp":                  rdpErr == nil,
		"rdpCredentialHandoff": rdpErr == nil,
		"rdpGateway":           rdpErr == nil,
		"webPortal":            true,
	}
	if rdpErr == nil {
		capabilities["rdpClient"] = rdpClient
	}
	return capabilities
}
