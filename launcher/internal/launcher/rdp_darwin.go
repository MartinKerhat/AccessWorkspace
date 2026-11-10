//go:build darwin

package launcher

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

// macOS RDP uses FreeRDP's native Cocoa client (MacFreeRDP), which ships
// INSIDE the launcher bundle: Mac users get one .dmg with everything in it,
// nothing to find and install separately (the opposite of the Linux model,
// where the distribution's FreeRDP package is the user's job). The client is
// built by scripts/build_freerdp_macos.sh and placed by scripts/build_macos.sh
// at Contents/Helpers/MacFreeRDP.app; it shares the command-line parser of the
// other FreeRDP clients, with the two differences below.

const bundledFreeRDPRelativePath = "Contents/Helpers/MacFreeRDP.app/Contents/MacOS/MacFreeRDP"

// freeRDPStdinPasswordArg must be the ":force" form here. Plain /from-stdin
// only tells the client to read the password from stdin when the server asks
// for it, and that read lives in the command-line clients' authentication
// callback; the Cocoa client replaces the callback with its own password
// window and never looks at stdin, so the stored password was silently
// dropped. ":force" makes the shared parser read it at startup, before any
// client-specific code runs.
const freeRDPStdinPasswordArg = "/from-stdin:force"

// freeRDPGatewayArg uses the FreeRDP 3 spelling: the short /g: is a
// deprecated alias that only exists in builds configured with
// WITH_FREERDP_DEPRECATED_COMMANDLINE, which build_freerdp_macos.sh does not
// set — the bundled client rejects it and quits. Given the host alone, the
// gateway hop reuses the connection's credentials.
func freeRDPGatewayArg(gatewayHost string) string {
	return "/gateway:g:" + gatewayHost
}

// freeRDPTLSSecLevelArg lets the bundled OpenSSL 3 complete the TLS handshake
// with Windows hosts that sign it with SHA-1 (rsa_pkcs1_sha1) — older or
// unpatched servers with their default self-signed RDP certificate. From
// security level 1 up OpenSSL 3 no longer offers SHA-1 signatures, such a
// server drops the connection, and the client reports a generic TLS failure
// before ever asking for a password. mstsc accepts these hosts; level 0 gives
// parity. The negotiated protocol is still the best both sides support
// (TLS 1.2 with ECDHE on those hosts), and the server certificate is still
// pinned on first use (/cert:tofu).
const freeRDPTLSSecLevelArg = "/tls:seclevel:0"

// findFreeRDPClient returns the bundled client's executable. For development
// builds that do not run from a .app bundle, ACCESS_WORKSPACE_MACFREERDP may
// point at a MacFreeRDP.app (or its executable) instead.
func findFreeRDPClient() (string, error) {
	if override := strings.TrimSpace(os.Getenv("ACCESS_WORKSPACE_MACFREERDP")); override != "" {
		candidate := override
		if strings.HasSuffix(candidate, ".app") {
			candidate = filepath.Join(candidate, "Contents", "MacOS", "MacFreeRDP")
		}
		if isExecutableFile(candidate) {
			return candidate, nil
		}
		return "", fmt.Errorf("ACCESS_WORKSPACE_MACFREERDP points at %s, which is not an executable MacFreeRDP", override)
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve launcher executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	// <bundle>.app/Contents/MacOS/<exe> -> <bundle>.app
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exePath)))
	candidate := filepath.Join(bundle, filepath.FromSlash(bundledFreeRDPRelativePath))
	if strings.HasSuffix(bundle, ".app") && isExecutableFile(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf("the bundled remote desktop client is missing from this launcher build — reinstall Access Workspace Launcher from the download dialog")
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func runRDPPlatform(item payload.LaunchPayload, host string, port string, gatewayHost string) error {
	client, err := findFreeRDPClient()
	if err != nil {
		Logf("rdp: %v", err)
		return err
	}

	// A window sized to this display rather than fullscreen: on macOS
	// fullscreen means its own Space, which is a bigger commitment than a
	// Windows user's fullscreen session, and the green button gets there in
	// one click anyway. /smart-sizing scales the session with the window
	// (the Cocoa client does not do dynamic resolution).
	width, height := preferredRDPDesktopSize()
	args := append(buildFreeRDPArgs(item, host, port, gatewayHost),
		fmt.Sprintf("/size:%dx%d", width, height), "/smart-sizing", freeRDPTLSSecLevelArg)
	secret := payload.MetadataSecret(item.Metadata, "secretValue")
	if strings.TrimSpace(secret) == "" {
		// No stored credential: the Cocoa client has its own password dialog,
		// so it runs directly — no terminal needed as on Linux.
		secret = ""
		Logf("rdp: no stored credentials, starting %s with its own prompt", client)
	}
	run, err := startMacRDPClient(client, args, secret)
	if err != nil {
		return err
	}
	if secret != "" {
		// With a stored credential nothing is asked first, so an exit within
		// this window means no session window ever appeared.
		select {
		case <-run.done:
			if !run.rejected.Seen() {
				return fmt.Errorf("the remote desktop client exited immediately (code %d, err=%v) — the connection was refused; details in logs/rdp-client.log", exitCodeOf(run.command), run.waitErr)
			}
		case <-time.After(2500 * time.Millisecond):
		}
	}
	backgroundWork.Add(1)
	go func() {
		defer backgroundWork.Done()
		superviseMacRDP(client, args, run)
	}()
	return nil
}

// maxRDPCredentialAttempts bounds how often the password prompt is offered
// for one launch, counting the first attempt.
const maxRDPCredentialAttempts = 3

// freeRDPCredentialFailureMarkers are the FreeRDP last-error names that mean
// the server rejected the user name, password or domain. FreeRDP logs the
// name of every last error at ERROR level, which the client log keeps. Older
// Windows hosts (CredSSP 5 and below) send no error code and just close the
// connection with a TLS "access denied" alert, which FreeRDP reports as
// AUTHENTICATION_FAILED. Deliberately absent: CONNECT_CANCELLED (the user
// pressed Cancel), locked or disabled accounts, expired passwords and network
// failures — asking for the password again would not help with those.
var freeRDPCredentialFailureMarkers = []string{
	"ERRCONNECT_AUTHENTICATION_FAILED",
	"ERRCONNECT_LOGON_FAILURE",
	"ERRCONNECT_WRONG_PASSWORD",
}

// credentialFailureDetector watches the client's output for a credential
// rejection. It keeps a short tail across writes, so a marker split between
// two writes still counts.
type credentialFailureDetector struct {
	mu   sync.Mutex
	tail []byte
	seen bool
}

func (d *credentialFailureDetector) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tail = append(d.tail, p...)
	for _, marker := range freeRDPCredentialFailureMarkers {
		if bytes.Contains(d.tail, []byte(marker)) {
			d.seen = true
		}
	}
	const keep = 64 // longer than any marker
	if len(d.tail) > keep {
		d.tail = append(d.tail[:0], d.tail[len(d.tail)-keep:]...)
	}
	return len(p), nil
}

func (d *credentialFailureDetector) Seen() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seen
}

// macRDPRun is one run of the bundled client. done is closed once it exited;
// waitErr is set before that.
type macRDPRun struct {
	command  *exec.Cmd
	rejected *credentialFailureDetector
	done     chan struct{}
	waitErr  error
}

// startMacRDPClient starts the client with its output going both to
// logs/rdp-client.log and to a credential-failure detector. A non-empty
// secret is handed over on stdin (freeRDPStdinPasswordArg) so it never
// appears in the process list the way /p: would; without one the client asks
// in its own dialog.
func startMacRDPClient(client string, args []string, secret string) (*macRDPRun, error) {
	if secret != "" {
		args = append(append([]string{}, args...), freeRDPStdinPasswordArg)
	}
	command := exec.Command(client, args...)
	var stdin io.WriteCloser
	if secret != "" {
		var err error
		if stdin, err = command.StdinPipe(); err != nil {
			return nil, fmt.Errorf("prepare rdp credential handoff: %w", err)
		}
	}
	run := &macRDPRun{command: command, rejected: &credentialFailureDetector{}, done: make(chan struct{})}
	var output io.Writer = run.rejected
	logFile := openRDPClientLog()
	if logFile != nil {
		output = io.MultiWriter(logFile, run.rejected)
	}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, fmt.Errorf("start %s: %w", client, err)
	}
	if stdin != nil {
		go func() {
			_, _ = io.WriteString(stdin, secret+"\n")
			_ = stdin.Close()
		}()
	}
	Logf("rdp: %s started (pid=%d, args=%v)", client, command.Process.Pid, sanitizeRDPArgsForLog(args))
	go func() {
		// The output is copied through a pipe, so the log file stays open
		// until Wait has drained it.
		run.waitErr = command.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		close(run.done)
	}()
	return run, nil
}

// superviseMacRDP waits for the client to exit and, when the server rejected
// the credentials, starts it again without a stored password so its dialog
// asks for them — the Cocoa client itself only shows "Authentication
// failure" and quits, where mstsc would simply ask again. A retry never
// reuses a stored password: it was just rejected.
func superviseMacRDP(client string, args []string, run *macRDPRun) {
	for attempt := 1; ; attempt++ {
		<-run.done
		Logf("rdp: %s pid=%d exited (code=%d, err=%v)", filepath.Base(client), run.command.Process.Pid, exitCodeOf(run.command), run.waitErr)
		if !run.rejected.Seen() {
			return
		}
		if attempt >= maxRDPCredentialAttempts {
			Logf("rdp: credentials rejected %d times, not asking again", attempt)
			return
		}
		Logf("rdp: credentials rejected (attempt %d of %d), asking again", attempt, maxRDPCredentialAttempts)
		next, err := startMacRDPClient(client, args, "")
		if err != nil {
			Logf("rdp: %v", err)
			return
		}
		run = next
	}
}

func exitCodeOf(command *exec.Cmd) int {
	if command.ProcessState == nil {
		return 0
	}
	return command.ProcessState.ExitCode()
}
