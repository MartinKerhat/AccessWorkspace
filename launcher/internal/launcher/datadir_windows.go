//go:build windows

package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// launcherDataDir is the per-user state directory (profiles, logs, payloads).
func launcherDataDir() (string, error) {
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not available")
	}
	return filepath.Join(localAppData, "AccessWorkspaceLauncher"), nil
}

// LauncherLogHint points users at the log location in error dialogs.
func LauncherLogHint() string {
	return "Details: %LOCALAPPDATA%\\AccessWorkspaceLauncher\\logs\\launcher.log"
}
