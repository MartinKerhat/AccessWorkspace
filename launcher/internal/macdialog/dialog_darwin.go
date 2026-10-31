//go:build darwin

package macdialog

import (
	"fmt"
	"os/exec"
)

// script builds the `display dialog` command. icon is one of AppleScript's
// stock icons: "note", "caution" or "stop".
func script(message string, title string, icon string) string {
	return fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button "OK" with icon %s`,
		AppleScriptString(message), AppleScriptString(title), icon)
}

// Show blocks until the user dismisses the alert, so a process that is about
// to quit does not pull the dialog from under its own message.
func Show(message string, title string, icon string) error {
	return exec.Command("osascript", "-e", script(message, title, icon)).Run()
}

// Start shows the alert and returns at once.
func Start(message string, title string, icon string) error {
	return exec.Command("osascript", "-e", script(message, title, icon)).Start()
}
