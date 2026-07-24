//go:build linux

package launcher

import (
	"fmt"
	"os"
	"strings"

	"access-workspace/launcher/internal/payload"
)

// runSSHPlatform opens the session in a visible terminal emulator by
// re-invoking the launcher with --ssh-session-file (same handoff the Windows
// build uses with its own console). The session process handles both the
// managed-password client and the native-ssh fallback.
func runSSHPlatform(item payload.LaunchPayload) error {
	if strings.TrimSpace(item.Target) == "" {
		return fmt.Errorf("ssh payload is missing target host")
	}

	sessionFile, err := writeSSHSessionPayload(item)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		_ = os.Remove(sessionFile)
		return fmt.Errorf("resolve launcher executable: %w", err)
	}
	if err := spawnInTerminal(exePath, "--ssh-session-file", sessionFile); err != nil {
		_ = os.Remove(sessionFile)
		return err
	}
	return nil
}
