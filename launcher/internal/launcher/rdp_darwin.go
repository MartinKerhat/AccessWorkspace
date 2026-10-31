//go:build darwin

package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

// macOS RDP uses FreeRDP's native Cocoa client (MacFreeRDP), which ships
// INSIDE the launcher bundle: Mac users get one .dmg with everything in it,
// nothing to find and install separately (the opposite of the Linux model,
// where the distribution's FreeRDP package is the user's job). The client is
// built by scripts/build_freerdp_macos.sh and placed by scripts/build_macos.sh
// at Contents/Helpers/MacFreeRDP.app; it accepts the same arguments as the
// other FreeRDP clients, including /from-stdin for the password.

const bundledFreeRDPRelativePath = "Contents/Helpers/MacFreeRDP.app/Contents/MacOS/MacFreeRDP"

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
		fmt.Sprintf("/size:%dx%d", width, height), "/smart-sizing")
	secret := payload.MetadataSecret(item.Metadata, "secretValue")
	if strings.TrimSpace(secret) == "" {
		// No stored credential: the Cocoa client has its own password dialog,
		// so it runs directly — no terminal needed as on Linux.
		Logf("rdp: no stored credentials, starting %s with its own prompt", client)
		command := exec.Command(client, args...)
		if err := command.Start(); err != nil {
			return fmt.Errorf("start %s: %w", client, err)
		}
		Logf("rdp: %s started (pid=%d, args=%v)", client, command.Process.Pid, sanitizeRDPArgsForLog(args))
		go func() { _ = command.Wait() }()
		return nil
	}
	return startFreeRDPWithStoredCredential(client, args, secret)
}
