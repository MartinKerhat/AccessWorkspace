//go:build darwin

package launcher

import (
	"fmt"
	"os"
	"path/filepath"
)

// launcherDataDir is the per-user state directory (logs, payloads). macOS
// keeps per-user app state in ~/Library/Application Support.
func launcherDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "access-workspace-launcher"), nil
}

// LauncherLogHint points users at the log location in error dialogs.
func LauncherLogHint() string {
	return "Details: ~/Library/Application Support/access-workspace-launcher/logs/launcher.log"
}
