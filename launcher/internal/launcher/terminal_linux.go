//go:build linux

package launcher

import (
	"fmt"
	"os/exec"
	"strings"
)

// terminalCandidate describes one terminal emulator and how it accepts a
// command to run ("prefix" flags before the argv). Ordered by how well they
// behave with plain `term <prefix> cmd args...` invocation; the freedesktop
// xdg-terminal-exec shim goes first because it resolves the user's actual
// default when present.
type terminalCandidate struct {
	binary string
	prefix []string
}

var terminalCandidates = []terminalCandidate{
	{binary: "xdg-terminal-exec"},
	{binary: "gnome-terminal", prefix: []string{"--"}},
	{binary: "konsole", prefix: []string{"-e"}},
	{binary: "xfce4-terminal", prefix: []string{"-x"}},
	{binary: "kitty"},
	{binary: "alacritty", prefix: []string{"-e"}},
	{binary: "foot"},
	{binary: "x-terminal-emulator", prefix: []string{"-e"}},
	{binary: "xterm", prefix: []string{"-e"}},
}

// findTerminal returns the first available terminal emulator.
func findTerminal() (terminalCandidate, error) {
	for _, candidate := range terminalCandidates {
		if path, err := exec.LookPath(candidate.binary); err == nil {
			return terminalCandidate{binary: path, prefix: candidate.prefix}, nil
		}
	}
	return terminalCandidate{}, fmt.Errorf("no terminal emulator found (looked for %s)", strings.Join(terminalNames(), ", "))
}

func terminalNames() []string {
	names := make([]string, 0, len(terminalCandidates))
	for _, candidate := range terminalCandidates {
		names = append(names, candidate.binary)
	}
	return names
}

// spawnInTerminal opens a visible terminal window running argv.
func spawnInTerminal(argv ...string) error {
	terminal, err := findTerminal()
	if err != nil {
		return err
	}
	command := exec.Command(terminal.binary, append(append([]string{}, terminal.prefix...), argv...)...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start terminal %s: %w", terminal.binary, err)
	}
	Logf("terminal: %s started (pid=%d)", terminal.binary, command.Process.Pid)
	return nil
}
