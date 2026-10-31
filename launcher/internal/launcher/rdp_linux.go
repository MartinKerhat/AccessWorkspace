//go:build linux

package launcher

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

// Linux RDP goes through the system FreeRDP client — the de-facto standard,
// packaged by every major distribution — which the user installs; the
// capability report tells the web app when it is missing.
var freeRDPClientCandidates = []string{"xfreerdp3", "xfreerdp", "sdl-freerdp3", "sdl-freerdp", "wlfreerdp"}

// findFreeRDPClient returns the first FreeRDP client binary on PATH.
func findFreeRDPClient() (string, error) {
	for _, candidate := range freeRDPClientCandidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no RDP client found — install the FreeRDP package (e.g. `sudo apt install freerdp3-x11` or `sudo dnf install freerdp`) and try again")
}

func runRDPPlatform(item payload.LaunchPayload, host string, port string, gatewayHost string) error {
	client, err := findFreeRDPClient()
	if err != nil {
		Logf("rdp: %v", err)
		return err
	}

	// Fullscreen and resizing with the window, like the Windows profile.
	args := append(buildFreeRDPArgs(item, host, port, gatewayHost), "/f", "/dynamic-resolution")
	secret := payload.MetadataSecret(item.Metadata, "secretValue")
	if strings.TrimSpace(secret) == "" {
		// No stored credential: run inside a terminal so FreeRDP's interactive
		// password/certificate prompts are actually answerable (the launcher
		// bridge itself has no tty). Closest equivalent of mstsc's GUI prompt.
		Logf("rdp: no stored credentials, starting %s interactively", client)
		if err := spawnInTerminal(append([]string{client}, args...)...); err != nil {
			return fmt.Errorf("start %s in a terminal: %w", client, err)
		}
		return nil
	}
	return startFreeRDPWithStoredCredential(client, args, secret)
}
