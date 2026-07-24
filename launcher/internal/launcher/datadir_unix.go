//go:build !windows

package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// launcherDataDir is the per-user state directory (profiles, logs, payloads).
// XDG data dir, so it survives cache cleaning: ~/.local/share by default.
func launcherDataDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "access-workspace-launcher"), nil
}

// LauncherLogHint points users at the log location in error dialogs.
func LauncherLogHint() string {
	return "Details: ~/.local/share/access-workspace-launcher/logs/launcher.log"
}
