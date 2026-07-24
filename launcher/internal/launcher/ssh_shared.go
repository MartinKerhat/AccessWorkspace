package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"access-workspace/launcher/internal/payload"
)

// Shared pieces of the launcher-managed SSH session (the in-process
// golang.org/x/crypto/ssh client): auth methods, console sizing, and the
// payload-file handoff used to re-enter the launcher inside a real terminal.

func buildSSHAuthMethods(secret string) []ssh.AuthMethod {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}

	return []ssh.AuthMethod{
		ssh.Password(secret),
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for index := range questions {
				answers[index] = secret
			}
			return answers, nil
		}),
	}
}

func sshConsoleSize() (int, int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return 120, 40
	}
	return width, height
}

// writeSSHSessionPayload persists the launch payload for the terminal-hosted
// session process (`launcher --ssh-session-file <file>`), which deletes it on
// start. 0600 + the private data dir keep the transient secret owner-only.
func writeSSHSessionPayload(item payload.LaunchPayload) (string, error) {
	baseDir, err := launcherDataDir()
	if err != nil {
		return "", err
	}
	payloadDir := filepath.Join(baseDir, "payloads")
	if err := os.MkdirAll(payloadDir, 0o700); err != nil {
		return "", fmt.Errorf("create ssh payload directory: %w", err)
	}

	file, err := os.CreateTemp(payloadDir, "ssh-session-*.json")
	if err != nil {
		return "", fmt.Errorf("create ssh payload file: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(item); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write ssh payload file: %w", err)
	}
	return file.Name(), nil
}

func startSSHSessionProcess(args ...string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve launcher executable: %w", err)
	}

	command := exec.Command(exePath, args...)
	return command.Start()
}
