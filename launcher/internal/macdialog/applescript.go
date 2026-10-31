// Package macdialog shows the launcher's macOS alerts. `display dialog` runs
// inside osascript itself, so it needs no automation permission; the launcher
// log stays the source of truth for what went wrong.
package macdialog

import "strings"

// AppleScriptString renders value as an AppleScript string literal. Dialog
// text embeds server-provided error messages, so every backslash and quote is
// escaped and line breaks become explicit concatenations.
func AppleScriptString(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + strings.ReplaceAll(escaped, "\n", `" & linefeed & "`) + `"`
}
