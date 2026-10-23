//go:build darwin

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// macOS install follows the platform's own model instead of copying itself
// like Windows and Linux: the user drags the .app from the .dmg into
// Applications and opens it once. That first open registers a per-user
// LaunchAgent for the localhost bridge. The access-workspace:// scheme comes
// from the bundle's Info.plist, which LaunchServices registers by itself.
// Upgrading is the same gesture: replace the .app, open it once.

// launchAgentLabel matches CFBundleIdentifier in launcher/macos/Info.plist.
const launchAgentLabel = "io.github.martinkerhat.accessworkspace.launcher"

func InstallOrUpgrade() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	bundlePath, err := enclosingAppBundle(exePath)
	if err != nil {
		return "", err
	}
	if !inApplicationsFolder(bundlePath) {
		// Also catches the opened-from-the-.dmg case (/Volumes/...) and
		// Gatekeeper App Translocation (a random read-only copy): a LaunchAgent
		// pointing at either would break as soon as the user ejects or moves on.
		return "", fmt.Errorf("move %q to the Applications folder, then open it from there", filepath.Base(bundlePath))
	}

	plistPath, err := writeLaunchAgent(exePath)
	if err != nil {
		return "", err
	}
	if err := reloadLaunchAgent(plistPath); err != nil {
		return "", err
	}
	if err := waitForBackgroundLauncher(); err != nil {
		return "", err
	}
	return bundlePath, nil
}

// enclosingAppBundle returns X.app for X.app/Contents/MacOS/<binary>.
func enclosingAppBundle(exePath string) (string, error) {
	bundlePath := filepath.Dir(filepath.Dir(filepath.Dir(exePath)))
	if !strings.HasSuffix(bundlePath, ".app") || filepath.Base(filepath.Dir(exePath)) != "MacOS" {
		return "", fmt.Errorf("the launcher must run from its .app bundle (running from %s)", exePath)
	}
	return bundlePath, nil
}

func inApplicationsFolder(bundlePath string) bool {
	parent := filepath.Dir(bundlePath)
	if parent == "/Applications" {
		return true
	}
	home, err := os.UserHomeDir()
	return err == nil && parent == filepath.Join(home, "Applications")
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

// writeLaunchAgent makes launchd start the localhost bridge at login and
// restart it if it crashes. A clean exit (port already taken by another
// instance) is not restarted.
func writeLaunchAgent(exePath string) (string, error) {
	plistPath, err := launchAgentPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return "", fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	content := strings.Join([]string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
		`<plist version="1.0">`,
		`<dict>`,
		`  <key>Label</key>`,
		`  <string>` + xmlEscape(launchAgentLabel) + `</string>`,
		`  <key>ProgramArguments</key>`,
		`  <array>`,
		`    <string>` + xmlEscape(exePath) + `</string>`,
		`    <string>--background</string>`,
		`  </array>`,
		`  <key>RunAtLoad</key>`,
		`  <true/>`,
		`  <key>KeepAlive</key>`,
		`  <dict>`,
		`    <key>SuccessfulExit</key>`,
		`    <false/>`,
		`  </dict>`,
		`  <key>LimitLoadToSessionType</key>`,
		`  <string>Aqua</string>`,
		`  <key>ProcessType</key>`,
		`  <string>Interactive</string>`,
		`</dict>`,
		`</plist>`,
	}, "\n") + "\n"
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write LaunchAgent: %w", err)
	}
	return plistPath, nil
}

// reloadLaunchAgent swaps the running bridge for the one this bundle ships:
// bootout stops a previous version (an upgrade), bootstrap starts this one.
func reloadLaunchAgent(plistPath string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	// Fails when the agent is not loaded yet (first install) — expected.
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchAgentLabel).Run()

	// bootout returns before the old instance is fully gone; bootstrap reports
	// "service already loaded" until it is, so retry briefly.
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		output, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("start the launcher bridge with launchd: %w", lastErr)
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}

// The app is opened from Finder, so a dialog is the only visible feedback.
func ShowInstallSuccess(installedPath string) {
	showDialog("Access Workspace Launcher is ready.\n\nThe local bridge is running and starts automatically when you log in. You can now return to the web app and use Connect.", "note")
}

func ShowInstallFailure(err error) {
	showDialog("Access Workspace Launcher could not be set up.\n\n"+err.Error(), "caution")
}

// showDialog blocks until the user dismisses it, so the app does not quit
// from under its own message.
func showDialog(message string, icon string) {
	script := fmt.Sprintf(`display dialog %s with title "Access Workspace Launcher" buttons {"OK"} default button "OK" with icon %s`, appleScriptString(message), icon)
	_ = exec.Command("osascript", "-e", script).Run()
}

// appleScriptString renders value as an AppleScript string expression.
func appleScriptString(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + strings.ReplaceAll(escaped, "\n", `" & linefeed & "`) + `"`
}
