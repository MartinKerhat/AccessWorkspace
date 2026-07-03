//go:build !windows

package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"access-workspace/launcher/internal/payload"
)

func runSSHPlatform(item payload.LaunchPayload) error {
	host := strings.TrimSpace(item.Target)
	if host == "" {
		return fmt.Errorf("ssh payload is missing target host")
	}

	username := payload.MetadataString(item.Metadata, "username")
	port := "22"
	if raw := payload.MetadataString(item.Metadata, "port"); raw != "" {
		port = raw
	}

	target := host
	if username != "" {
		target = fmt.Sprintf("%s@%s", username, host)
	}

	command := exec.Command("ssh", target, "-p", port)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Start()
}

func RunSSHSession(item payload.LaunchPayload) error {
	return runSSHPlatform(item)
}
