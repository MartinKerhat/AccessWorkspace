//go:build darwin

package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// spawnInTerminal opens a Terminal.app window running argv. The command goes
// through a throwaway .command script opened with `open -a Terminal` rather
// than AppleScript (`tell application "Terminal" to do script`), which would
// make macOS ask the user to let the launcher control Terminal. The script
// deletes itself before running the command.
func spawnInTerminal(argv ...string) error {
	baseDir, err := launcherDataDir()
	if err != nil {
		return err
	}
	scriptDir := filepath.Join(baseDir, "payloads")
	if err := os.MkdirAll(scriptDir, 0o700); err != nil {
		return fmt.Errorf("create terminal script directory: %w", err)
	}
	file, err := os.CreateTemp(scriptDir, "terminal-*.command")
	if err != nil {
		return fmt.Errorf("create terminal script: %w", err)
	}
	scriptPath := file.Name()

	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	script := "#!/bin/sh\nrm -f \"$0\"\nexec " + strings.Join(quoted, " ") + "\n"
	_, writeErr := file.WriteString(script)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("write terminal script: %v", firstError(writeErr, closeErr))
	}
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("make terminal script executable: %w", err)
	}

	if output, err := exec.Command("open", "-a", "Terminal", scriptPath).CombinedOutput(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("open Terminal: %v: %s", err, strings.TrimSpace(string(output)))
	}
	Logf("terminal: Terminal.app opened %s", filepath.Base(scriptPath))
	return nil
}

// shellQuote wraps a value in single quotes for /bin/sh.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
