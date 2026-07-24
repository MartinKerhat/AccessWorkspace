//go:build !windows

package launcher

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"access-workspace/launcher/internal/payload"
)

// RunSSHSession is the terminal-hosted managed SSH session on Unix. Unlike
// Windows there is no console to allocate — the launcher is re-invoked
// INSIDE a terminal emulator (`--ssh-session-file`), so stdin/stdout already
// are the interactive tty.
func RunSSHSession(item payload.LaunchPayload) error {
	host := strings.TrimSpace(item.Target)
	if host == "" {
		return fmt.Errorf("ssh payload is missing target host")
	}

	username := strings.TrimSpace(payload.MetadataString(item.Metadata, "username"))
	if username == "" {
		username = strings.TrimSpace(os.Getenv("USER"))
	}
	if username == "" {
		return fmt.Errorf("ssh payload is missing username")
	}

	port := strings.TrimSpace(payload.MetadataString(item.Metadata, "port"))
	if port == "" {
		port = "22"
	}

	secret := payload.MetadataString(item.Metadata, "secretValue")
	if strings.TrimSpace(secret) == "" {
		// No stored password: hand over to the native ssh client in this
		// terminal (key-based setups, agent auth, interactive prompts).
		return startNativeSSHInCurrentTerminal(username, host, port)
	}

	address := net.JoinHostPort(host, port)
	clientConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            buildSSHAuthMethods(secret),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	client, err := ssh.Dial("tcp", address, clientConfig)
	if err != nil {
		return fmt.Errorf("connect ssh session: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	width, height := sshConsoleSize()
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return fmt.Errorf("request ssh pty: %w", err)
	}

	fd := int(os.Stdin.Fd())
	if state, err := term.MakeRaw(fd); err == nil {
		defer func() {
			_ = term.Restore(fd, state)
		}()
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start ssh shell: %w", err)
	}
	if err := session.Wait(); err != nil {
		return fmt.Errorf("wait for ssh shell: %w", err)
	}
	return nil
}

func startNativeSSHInCurrentTerminal(username string, host string, port string) error {
	target := host
	if strings.TrimSpace(username) != "" {
		target = fmt.Sprintf("%s@%s", strings.TrimSpace(username), host)
	}

	command := exec.Command("ssh", target, "-p", port)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
