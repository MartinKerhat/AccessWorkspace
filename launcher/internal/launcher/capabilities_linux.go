//go:build linux

package launcher

import "os/exec"

// Capabilities reports what this launcher build supports on this machine.
// RDP depends on a FreeRDP client being installed; SSH needs the OpenSSH
// client plus some terminal emulator to show the session in.
func Capabilities() map[string]any {
	_, sshErr := exec.LookPath("ssh")
	_, terminalErr := findTerminal()
	rdpClient, rdpErr := findFreeRDPClient()

	capabilities := map[string]any{
		"ssh":                  sshErr == nil && terminalErr == nil,
		"sshCredentialHandoff": sshErr == nil && terminalErr == nil,
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
