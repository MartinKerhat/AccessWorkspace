//go:build linux

package launcher

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"access-workspace/launcher/internal/payload"
)

// Linux RDP goes through FreeRDP (the de-facto standard client, packaged by
// every major distribution). Unlike mstsc there is no profile file, no
// Credential Manager and no profile signing: everything — including the
// credential — is passed as arguments/stdin from this trusted local process,
// which is exactly the semantic-payload model the launcher uses.
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

	username := strings.TrimSpace(payload.MetadataString(item.Metadata, "username"))
	domain := strings.TrimSpace(payload.MetadataString(item.Metadata, "connectionDomain"))
	secret := payload.MetadataString(item.Metadata, "secretValue")

	args := []string{fmt.Sprintf("/v:%s:%s", host, port)}
	if username != "" {
		args = append(args, "/u:"+username)
	}
	if domain != "" {
		args = append(args, "/d:"+domain)
	}
	if gatewayHost != "" {
		// FreeRDP reuses the connection credentials for the gateway hop when no
		// separate /gu:/gp: are given — same behavior the Windows flow arranges
		// via cmdkey (promptcredentialonce).
		args = append(args, "/g:"+gatewayHost)
	}
	if payload.MetadataBool(item.Metadata, "connectionAdminSession") {
		args = append(args, "/admin")
	}
	// Parity with the Windows baseline: fullscreen, resize with the window,
	// clipboard on, and accept the server certificate on first use (mstsc
	// profiles ship "authentication level:i:0").
	args = append(args, "/f", "/dynamic-resolution", "+clipboard", "/cert:tofu")

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

	// Stored credential: hand the password over stdin (/from-stdin) so it never
	// appears in the process list the way /p: would.
	args = append(args, "/from-stdin")
	command := exec.Command(client, args...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("prepare rdp credential handoff: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start %s: %w", client, err)
	}
	go func() {
		_, _ = io.WriteString(stdin, secret+"\n")
		_ = stdin.Close()
	}()
	Logf("rdp: %s started (pid=%d, args=%v)", client, command.Process.Pid, sanitizeRDPArgsForLog(args))

	// Same early-exit watch as Windows: the client dying within the window
	// means no session window ever appeared (bad credentials, refused cert...).
	if err := waitForEarlyRDPExit(command, 2500*time.Millisecond); err != nil {
		return err
	}
	return nil
}

// sanitizeRDPArgsForLog keeps credentials out of the launcher log.
func sanitizeRDPArgsForLog(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "/p:") || strings.HasPrefix(arg, "/gp:") {
			out = append(out, "/p:***")
			continue
		}
		out = append(out, arg)
	}
	return out
}
