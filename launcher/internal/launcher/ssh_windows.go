//go:build windows

package launcher

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/windows"
	"golang.org/x/term"

	"access-workspace/launcher/internal/payload"
)

const createNewConsole = 0x00000010

const (
	enableProcessedOutput      = 0x0001
	enableWrapAtEOLOutput      = 0x0002
	enableVirtualTerminal      = 0x0004
	enableVirtualTerminalInput = 0x0200
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

	secret := payload.MetadataString(item.Metadata, "secretValue")
	if strings.TrimSpace(secret) == "" {
		return startNativeSSHWindows(username, host, port)
	}

	sessionFile, err := writeSSHSessionPayload(item)
	if err != nil {
		return err
	}
	if err := startSSHSessionProcess("--ssh-session-file", sessionFile); err != nil {
		_ = os.Remove(sessionFile)
		return err
	}
	return nil
}

func RunSSHSession(item payload.LaunchPayload) error {
	restoreConsole, err := prepareSSHConsole()
	if err != nil {
		return fmt.Errorf("prepare ssh console: %w", err)
	}
	defer restoreConsole()

	host := strings.TrimSpace(item.Target)
	if host == "" {
		return fmt.Errorf("ssh payload is missing target host")
	}

	username := strings.TrimSpace(payload.MetadataString(item.Metadata, "username"))
	if username == "" {
		username = strings.TrimSpace(os.Getenv("USERNAME"))
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
		return startNativeSSHInCurrentConsole(username, host, port)
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

	restoreRaw := makeSSHConsoleRaw()
	defer restoreRaw()

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

func makeSSHConsoleRaw() func() {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}
	}
	stdinHandle := windows.Handle(os.Stdin.Fd())
	var rawMode uint32
	if err := windows.GetConsoleMode(stdinHandle, &rawMode); err == nil {
		_ = windows.SetConsoleMode(stdinHandle, rawMode|enableVirtualTerminalInput)
	}
	return func() {
		_ = term.Restore(fd, state)
	}
}

func startNativeSSHWindows(username string, host string, port string) error {
	target := host
	if strings.TrimSpace(username) != "" {
		target = fmt.Sprintf("%s@%s", strings.TrimSpace(username), host)
	}

	commandLine := fmt.Sprintf("ssh %s -p %s", target, port)
	command := exec.Command("cmd.exe", "/k", commandLine)
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewConsole,
	}
	return command.Start()
}

func startNativeSSHInCurrentConsole(username string, host string, port string) error {
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

func prepareSSHConsole() (func(), error) {
	restoreCodePages, err := ensureSSHConsole()
	if err != nil {
		return nil, err
	}
	restoreModes, err := enableSSHVirtualTerminal()
	if err != nil {
		restoreCodePages()
		return nil, err
	}
	return func() {
		restoreModes()
		restoreCodePages()
	}, nil
}

func ensureSSHConsole() (func(), error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	allocConsole := kernel32.NewProc("AllocConsole")
	setConsoleTitleW := kernel32.NewProc("SetConsoleTitleW")
	getConsoleCP := kernel32.NewProc("GetConsoleCP")
	getConsoleOutputCP := kernel32.NewProc("GetConsoleOutputCP")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")

	consoleWindow, _, _ := getConsoleWindow.Call()
	if consoleWindow == 0 {
		result, _, callErr := allocConsole.Call()
		if result == 0 {
			return nil, callErr
		}
	}

	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = input.Close()
		return nil, err
	}

	os.Stdin = input
	os.Stdout = output
	os.Stderr = output

	titlePtr, _ := syscall.UTF16PtrFromString("Access Workspace SSH")
	setConsoleTitleW.Call(uintptr(unsafe.Pointer(titlePtr)))

	originalInputCP, _, _ := getConsoleCP.Call()
	originalOutputCP, _, _ := getConsoleOutputCP.Call()
	setConsoleCP.Call(65001)
	setConsoleOutputCP.Call(65001)

	return func() {
		setConsoleCP.Call(originalInputCP)
		setConsoleOutputCP.Call(originalOutputCP)
	}, nil
}

func enableSSHVirtualTerminal() (func(), error) {
	stdoutHandle := windows.Handle(os.Stdout.Fd())
	stderrHandle := windows.Handle(os.Stderr.Fd())
	stdinHandle := windows.Handle(os.Stdin.Fd())

	var originalOutMode uint32
	if err := windows.GetConsoleMode(stdoutHandle, &originalOutMode); err != nil {
		return nil, err
	}
	var originalErrMode uint32
	if err := windows.GetConsoleMode(stderrHandle, &originalErrMode); err != nil {
		return nil, err
	}
	var originalInMode uint32
	if err := windows.GetConsoleMode(stdinHandle, &originalInMode); err != nil {
		return nil, err
	}

	newOutMode := originalOutMode | enableProcessedOutput | enableWrapAtEOLOutput | enableVirtualTerminal
	if err := windows.SetConsoleMode(stdoutHandle, newOutMode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(stderrHandle, newOutMode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(stdinHandle, originalInMode|enableVirtualTerminalInput); err != nil {
		return nil, err
	}

	return func() {
		_ = windows.SetConsoleMode(stdoutHandle, originalOutMode)
		_ = windows.SetConsoleMode(stderrHandle, originalErrMode)
		_ = windows.SetConsoleMode(stdinHandle, originalInMode)
	}, nil
}
