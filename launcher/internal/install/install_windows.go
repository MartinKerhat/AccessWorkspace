//go:build windows

package install

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"access-workspace/launcher/internal/launcherinfo"
)

const (
	installDirName        = "AccessWorkspaceLauncher"
	installFileName       = "access-workspace-launcher.exe"
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
)

type RDPMachineTrustPackage struct {
	Thumbprint     string `json:"thumbprint"`
	LeafCertBase64 string `json:"leafCertBase64"`
	RootCertBase64 string `json:"rootCertBase64"`
}

func InstallOrUpgrade() (string, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not available")
	}

	installDir := filepath.Join(localAppData, installDirName)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create launcher install directory: %w", err)
	}

	targetExe := filepath.Join(installDir, installFileName)
	if err := stopInstalledLauncher(targetExe); err != nil {
		return "", err
	}
	if err := copyFile(currentExe, targetExe); err != nil {
		return "", err
	}
	if err := registerProtocol(targetExe); err != nil {
		return "", err
	}
	if err := registerAutorun(targetExe); err != nil {
		return "", err
	}
	// Best-effort: RDP publisher trust targets the workspace the launcher learns
	// from launch payloads, which isn't known at first install. It is ensured
	// lazily on the first RDP launch, so never fail install on it.
	_ = synchronizeLauncherPrerequisites(targetExe)
	if err := startBackgroundLauncher(targetExe); err != nil {
		return "", err
	}
	if err := waitForBackgroundLauncher(); err != nil {
		return "", err
	}
	return targetExe, nil
}

func stopInstalledLauncher(exePath string) error {
	for attempt := 0; attempt < 3; attempt++ {
		taskkill := exec.Command("taskkill.exe", "/IM", installFileName, "/F", "/T")
		taskkill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_, _ = taskkill.CombinedOutput()
		time.Sleep(900 * time.Millisecond)
	}
	if err := waitForBackgroundLauncherShutdown(); err != nil {
		return err
	}
	if err := waitForWritableTarget(exePath); err != nil {
		return err
	}
	return nil
}

func registerProtocol(exePath string) error {
	commandValue := fmt.Sprintf("\"%s\" \"%%1\"", exePath)
	commands := [][]string{
		{"add", `HKCU\Software\Classes\access-workspace`, "/ve", "/d", "URL:Access Workspace Launcher", "/f"},
		{"add", `HKCU\Software\Classes\access-workspace`, "/v", "URL Protocol", "/d", "", "/f"},
		{"add", `HKCU\Software\Classes\access-workspace\DefaultIcon`, "/ve", "/d", exePath, "/f"},
		{"add", `HKCU\Software\Classes\access-workspace\shell\open\command`, "/ve", "/d", commandValue, "/f"},
	}
	for _, args := range commands {
		command := exec.Command("reg.exe", args...)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("register launcher protocol: %w (%s)", err, string(output))
		}
	}
	return nil
}

func registerAutorun(exePath string) error {
	commandValue := fmt.Sprintf("\"%s\" --background", exePath)
	command := exec.Command(
		"reg.exe",
		"add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v",
		"AccessWorkspaceLauncher",
		"/d",
		commandValue,
		"/f",
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("register launcher autorun: %w (%s)", err, string(output))
	}
	return nil
}

func ShowInstallSuccess(installedPath string) {
	showMessageBox(
		fmt.Sprintf("Access Workspace Launcher %s installed.\n\nLocation:\n%s\n\nYou can now return to the web app and use Connect.", launcherinfo.Version, installedPath),
		"Access Workspace Launcher",
	)
}

func ShowInstallFailure(err error) {
	showMessageBox(
		fmt.Sprintf("Access Workspace Launcher installation failed.\n\n%s", err),
		"Access Workspace Launcher",
	)
}

func showMessageBox(message string, title string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		0,
	)
}

func copyFile(source string, target string) error {
	input, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read launcher executable: %w", err)
	}

	tempTarget := target + ".new"
	backupTarget := target + ".previous"

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		_ = os.Remove(tempTarget)
		if err := os.WriteFile(tempTarget, input, 0o755); err != nil {
			lastErr = err
			time.Sleep(700 * time.Millisecond)
			continue
		}

		_ = os.Remove(backupTarget)
		if _, err := os.Stat(target); err == nil {
			if err := os.Rename(target, backupTarget); err != nil {
				lastErr = err
				_ = os.Remove(tempTarget)
				time.Sleep(700 * time.Millisecond)
				continue
			}
		}

		if err := os.Rename(tempTarget, target); err != nil {
			lastErr = err
			if _, backupErr := os.Stat(backupTarget); backupErr == nil {
				_ = os.Rename(backupTarget, target)
			}
			_ = os.Remove(tempTarget)
			time.Sleep(700 * time.Millisecond)
			continue
		}

		_ = os.Remove(backupTarget)
		return nil
	}
	return fmt.Errorf("write launcher executable: %w", lastErr)
}

func startBackgroundLauncher(exePath string) error {
	command := exec.Command(exePath, "--background")
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup | createNoWindow,
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start background launcher: %w", err)
	}
	_ = command.Process.Release()
	return nil
}

func synchronizeLauncherPrerequisites(exePath string) error {
	command := exec.Command(exePath, "--sync-agent-prereqs")
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("synchronize launcher prerequisites: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func waitForBackgroundLauncher() error {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		response, err := client.Get("http://127.0.0.1:47654/status")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("unexpected launcher status code %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("background launcher did not become ready: %w", lastErr)
}

func waitForBackgroundLauncherShutdown() error {
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	for attempt := 0; attempt < 15; attempt++ {
		response, err := client.Get("http://127.0.0.1:47654/status")
		if err != nil {
			return nil
		}
		_ = response.Body.Close()
		time.Sleep(350 * time.Millisecond)
	}
	return fmt.Errorf("installed launcher did not stop cleanly before upgrade")
}

func waitForWritableTarget(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt < 15; attempt++ {
		handle, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err == nil {
			_ = handle.Close()
			return nil
		}
		lastErr = err
		time.Sleep(350 * time.Millisecond)
	}
	return fmt.Errorf("installed launcher executable is still locked: %w", lastErr)
}

func WriteMachineTrustPackage(pkg RDPMachineTrustPackage) (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not available")
	}
	baseDir := filepath.Join(localAppData, installDirName, "trust")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create machine trust directory: %w", err)
	}
	packagePath := filepath.Join(baseDir, "rdp-machine-trust.json")
	bytes, err := json.Marshal(pkg)
	if err != nil {
		return "", fmt.Errorf("encode machine trust package: %w", err)
	}
	if err := os.WriteFile(packagePath, bytes, 0o600); err != nil {
		return "", fmt.Errorf("write machine trust package: %w", err)
	}
	return packagePath, nil
}

func EnsureMachineRDPPublisherTrust(pkg RDPMachineTrustPackage) error {
	packagePath, err := WriteMachineTrustPackage(pkg)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve launcher executable: %w", err)
	}
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$p = Start-Process -FilePath '%s' -ArgumentList @('--install-machine-rdp-trust','%s') -Verb RunAs -Wait -PassThru; exit $p.ExitCode",
			escapePowerShellSingleQuoted(exePath),
			escapePowerShellSingleQuoted(packagePath),
		),
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		lower := strings.ToLower(text + " " + err.Error())
		if strings.Contains(lower, "canceled") || strings.Contains(lower, "cancelled") {
			return fmt.Errorf("administrator approval is required to trust the RDP publisher on this PC")
		}
		return fmt.Errorf("run elevated machine trust installer: %w (%s)", err, text)
	}
	return nil
}

func InstallMachineRDPPublisherTrustFromFile(path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read machine trust package: %w", err)
	}
	var pkg RDPMachineTrustPackage
	if err := json.Unmarshal(bytes, &pkg); err != nil {
		return fmt.Errorf("decode machine trust package: %w", err)
	}
	if strings.TrimSpace(pkg.Thumbprint) == "" || strings.TrimSpace(pkg.LeafCertBase64) == "" || strings.TrimSpace(pkg.RootCertBase64) == "" {
		return fmt.Errorf("machine trust package is incomplete")
	}
	leafThumbprint, err := certificateThumbprintFromBase64(pkg.LeafCertBase64)
	if err != nil {
		return err
	}
	rootThumbprint, err := certificateThumbprintFromBase64(pkg.RootCertBase64)
	if err != nil {
		return err
	}

	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	certDir := filepath.Join(programData, installDirName, "certificates")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return fmt.Errorf("create machine certificate directory: %w", err)
	}
	leafCertPath := filepath.Join(certDir, "rdp-signing-leaf.cer")
	rootCertPath := filepath.Join(certDir, "rdp-signing-root.cer")
	if err := writeBase64File(leafCertPath, pkg.LeafCertBase64); err != nil {
		return err
	}
	if err := writeBase64File(rootCertPath, pkg.RootCertBase64); err != nil {
		return err
	}
	if err := cleanupMachineTestRDPSigningCertificates(leafThumbprint, rootThumbprint); err != nil {
		return err
	}
	if err := addCertificateToMachineStore("TrustedPublisher", leafCertPath); err != nil {
		return err
	}
	if err := addCertificateToMachineStore("Root", rootCertPath); err != nil {
		return err
	}
	if err := addTrustedRDPPublisherThumbprintMachine(pkg.Thumbprint); err != nil {
		return err
	}
	return nil
}

func addCertificateToMachineStore(store string, path string) error {
	command := exec.Command("certutil.exe", "-addstore", store, path)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		text := strings.ToLower(strings.TrimSpace(string(output)))
		if strings.Contains(text, "already") {
			return nil
		}
		return fmt.Errorf("add certificate to local machine %s store: %w (%s)", store, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func addTrustedRDPPublisherThumbprintMachine(thumbprint string) error {
	thumbprint = strings.ToUpper(strings.TrimSpace(thumbprint))
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$path = 'HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows NT\\Terminal Services'; "+
				"New-Item -Path $path -Force | Out-Null; "+
				"$name = 'TrustedCertThumbprints'; "+
				"$current = (Get-ItemProperty -Path $path -Name $name -ErrorAction SilentlyContinue).$name; "+
				"$items = @(); "+
				"if ($current) { $items = $current -split ',' | ForEach-Object { $_.Trim().ToUpper() } | Where-Object { $_ -ne '' } }; "+
				"if ($items -notcontains '%s') { $items = @($items + '%s') }; "+
				"Set-ItemProperty -Path $path -Name $name -Value ($items -join ',')",
			thumbprint,
			thumbprint,
		),
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add trusted rdp publisher thumbprint (machine): %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeBase64File(path string, encoded string) error {
	bytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return fmt.Errorf("decode certificate package: %w", err)
	}
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return fmt.Errorf("write certificate file: %w", err)
	}
	return nil
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func certificateThumbprintFromBase64(encoded string) (string, error) {
	bytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("decode certificate for thumbprint: %w", err)
	}
	certificate, err := x509.ParseCertificate(bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate for thumbprint: %w", err)
	}
	sum := sha1.Sum(certificate.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

func cleanupMachineTestRDPSigningCertificates(currentLeafThumbprint string, currentRootThumbprint string) error {
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$targets = @("+
				"@{ Store = 'TrustedPublisher'; Path = 'Cert:\\LocalMachine\\TrustedPublisher'; Subject = 'CN=Access Workspace Test RDP Publisher, O=Access Workspace'; Keep = '%s' },"+
				"@{ Store = 'Root'; Path = 'Cert:\\LocalMachine\\Root'; Subject = 'CN=Access Workspace Test RDP Root, O=Access Workspace'; Keep = '%s' }"+
				");"+
				"$results = @(); "+
				"foreach ($target in $targets) { "+
				"  $items = Get-ChildItem $target.Path -ErrorAction SilentlyContinue | Where-Object { $_.Subject -eq $target.Subject -and $_.Thumbprint.ToUpper() -ne $target.Keep }; "+
				"  foreach ($item in $items) { $results += ($target.Store + '|' + $item.Thumbprint.ToUpper()) } "+
				"}; "+
				"$results -join \"`n\"",
			strings.ToUpper(strings.TrimSpace(currentLeafThumbprint)),
			strings.ToUpper(strings.TrimSpace(currentRootThumbprint)),
		),
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cleanup stale machine rdp signing certificates: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		if err := deleteCertificateFromMachineStore(parts[0], parts[1]); err != nil {
			return err
		}
	}
	return nil
}

func deleteCertificateFromMachineStore(store string, thumbprint string) error {
	command := exec.Command("certutil.exe", "-delstore", store, strings.ToUpper(strings.TrimSpace(thumbprint)))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		text := strings.ToLower(strings.TrimSpace(string(output)))
		if strings.Contains(text, "cannot find") || strings.Contains(text, "not found") {
			return nil
		}
		return fmt.Errorf("remove stale certificate from local machine %s store: %w (%s)", store, err, strings.TrimSpace(string(output)))
	}
	return nil
}
