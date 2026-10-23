//go:build darwin

package launcher

import (
	"fmt"
	"os/exec"
	"strings"
)

// showLauncherMessageBox surfaces a launch failure to the user with a
// standard alert. `display dialog` runs inside osascript itself, so it needs
// no automation permission; the log remains the source of truth either way.
func showLauncherMessageBox(message string, title string) {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button "OK" with icon caution`, appleScriptString(message), appleScriptString(title))
	_ = exec.Command("osascript", "-e", script).Start()
}

// appleScriptString renders value as an AppleScript string expression.
func appleScriptString(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + strings.ReplaceAll(escaped, "\n", `" & linefeed & "`) + `"`
}
