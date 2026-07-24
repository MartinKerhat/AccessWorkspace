package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Launch diagnostics land in %LOCALAPPDATA%\AccessWorkspaceLauncher\logs\launcher.log.
// Logging must never break or slow a launch, so every failure here is ignored.
// Secrets (passwords, PFX material) must never be passed to Logf.

const launcherLogMaxBytes = 1 << 20

var logMu sync.Mutex

func Logf(format string, args ...interface{}) {
	baseDir, err := launcherDataDir()
	if err != nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	logsDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return
	}
	logPath := filepath.Join(logsDir, "launcher.log")
	rotateLauncherLog(logPath)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s pid=%d %s\r\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), fmt.Sprintf(format, args...))
}

func rotateLauncherLog(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < launcherLogMaxBytes {
		return
	}
	_ = os.Remove(logPath + ".old")
	_ = os.Rename(logPath, logPath+".old")
}
