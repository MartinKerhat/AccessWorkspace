//go:build linux || darwin

package launcher

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

// Linux and macOS both drive FreeRDP. Unlike mstsc there is no profile file,
// no Credential Manager and no profile signing: everything — including the
// credential — is passed as arguments/stdin from this trusted local process,
// which is exactly the semantic-payload model the launcher uses. The
// platforms differ only in where the client comes from (a system package on
// Linux, a client bundled inside the launcher .app on macOS) and in a few
// display flags; see rdp_linux.go and rdp_darwin.go.

// buildFreeRDPArgs maps the connection fields — target, credentials, domain,
// Remote Desktop Gateway, admin session, window title — to FreeRDP arguments.
// The password is deliberately not among them: it travels on stdin.
func buildFreeRDPArgs(item payload.LaunchPayload, host string, port string, gatewayHost string) []string {
	username := strings.TrimSpace(payload.MetadataString(item.Metadata, "username"))
	domain := strings.TrimSpace(payload.MetadataString(item.Metadata, "connectionDomain"))

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
	// Title the session window after the connection so several open sessions
	// stay tellable-apart in the taskbar / Dock.
	if name := strings.TrimSpace(payload.MetadataString(item.Metadata, "connectionName")); name != "" {
		args = append(args, "/t:"+name)
	}
	// Parity with the Windows baseline: clipboard on, and accept the server
	// certificate on first use (mstsc profiles ship "authentication level:i:0").
	return append(args, "+clipboard", "/cert:tofu")
}

// startFreeRDPWithStoredCredential launches the client with the password
// handed over on stdin (/from-stdin) so it never appears in the process list
// the way /p: would, then watches for an early exit: the client dying within
// the window means no session window ever appeared (bad credentials, refused
// certificate, ...).
func startFreeRDPWithStoredCredential(client string, args []string, secret string) error {
	args = append(append([]string{}, args...), "/from-stdin")
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
	return waitForEarlyRDPExit(command, 2500*time.Millisecond)
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
