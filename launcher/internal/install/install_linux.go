//go:build linux

package install

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Linux self-install follows the XDG conventions the Windows build mirrors
// with the registry: binary in ~/.local/bin, a .desktop entry registering the
// access-workspace:// URI handler, and an autostart entry for the localhost
// bridge. Everything is per-user; nothing needs root.

const (
	installBinaryName  = "access-workspace-launcher"
	desktopHandlerName = "access-workspace-launcher.desktop"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create launcher install directory: %w", err)
	}
	targetExe := filepath.Join(binDir, installBinaryName)

	stopInstalledLauncher()
	if err := copyExecutable(currentExe, targetExe); err != nil {
		return "", err
	}
	if err := registerProtocolHandler(targetExe); err != nil {
		return "", err
	}
	if err := registerAutostart(targetExe); err != nil {
		return "", err
	}
	if err := startBackgroundLauncher(targetExe); err != nil {
		return "", err
	}
	if err := waitForBackgroundLauncher(); err != nil {
		return "", err
	}
	return targetExe, nil
}

// stopInstalledLauncher asks a running bridge instance to make way for the
// upgrade. There is no service manager to ask, so this is a best-effort
// pkill of the background invocation (never this install process — the
// argument filter only matches `--background`).
func stopInstalledLauncher() {
	if path, err := exec.LookPath("pkill"); err == nil {
		_ = exec.Command(path, "-f", installBinaryName+" --background").Run()
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	for attempt := 0; attempt < 15; attempt++ {
		response, err := client.Get("http://127.0.0.1:47654/status")
		if err != nil {
			return
		}
		_ = response.Body.Close()
		time.Sleep(350 * time.Millisecond)
	}
}

func copyExecutable(source string, target string) error {
	input, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read launcher executable: %w", err)
	}
	tempTarget := target + ".new"
	if err := os.WriteFile(tempTarget, input, 0o755); err != nil {
		return fmt.Errorf("write launcher executable: %w", err)
	}
	if err := os.Rename(tempTarget, target); err != nil {
		_ = os.Remove(tempTarget)
		return fmt.Errorf("replace launcher executable: %w", err)
	}
	return nil
}

// registerProtocolHandler writes the freedesktop .desktop entry that binds
// the access-workspace:// scheme to the installed binary.
func registerProtocolHandler(exePath string) error {
	applicationsDir, err := xdgDataSubdir("applications")
	if err != nil {
		return err
	}
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Access Workspace Launcher",
		"Comment=Opens SSH and RDP connections from the Access Workspace web app",
		fmt.Sprintf("Exec=%q --uri %%u", exePath),
		"Terminal=false",
		"NoDisplay=true",
		"MimeType=x-scheme-handler/access-workspace;",
	}, "\n") + "\n"
	desktopPath := filepath.Join(applicationsDir, desktopHandlerName)
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write protocol handler desktop entry: %w", err)
	}

	// Best effort: refresh the desktop database and claim the scheme default.
	if path, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(path, applicationsDir).Run()
	}
	if path, err := exec.LookPath("xdg-mime"); err == nil {
		_ = exec.Command(path, "default", desktopHandlerName, "x-scheme-handler/access-workspace").Run()
	}
	return nil
}

// registerAutostart makes the localhost bridge start with the desktop session.
func registerAutostart(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	configBase := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configBase == "" {
		configBase = filepath.Join(home, ".config")
	}
	autostartDir := filepath.Join(configBase, "autostart")
	if err := os.MkdirAll(autostartDir, 0o755); err != nil {
		return fmt.Errorf("create autostart directory: %w", err)
	}
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Access Workspace Launcher",
		"Comment=Local launcher bridge for the Access Workspace web app",
		fmt.Sprintf("Exec=%q --background", exePath),
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
	}, "\n") + "\n"
	autostartPath := filepath.Join(autostartDir, desktopHandlerName)
	if err := os.WriteFile(autostartPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write autostart desktop entry: %w", err)
	}
	return nil
}

func xdgDataSubdir(name string) (string, error) {
	base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s directory: %w", dir, err)
	}
	return dir, nil
}

func startBackgroundLauncher(exePath string) error {
	command := exec.Command(exePath, "--background")
	if err := command.Start(); err != nil {
		return fmt.Errorf("start background launcher: %w", err)
	}
	_ = command.Process.Release()
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

// The install runs from a terminal on Linux (extracted tarball), so plain
// stdout/stderr is the right feedback channel.
func ShowInstallSuccess(installedPath string) {
	fmt.Printf("Access Workspace Launcher installed.\n\nLocation: %s\n\nThe local bridge is running and starts automatically with your session.\nYou can now return to the web app and use Connect.\n", installedPath)
}

func ShowInstallFailure(err error) {
	fmt.Fprintf(os.Stderr, "Access Workspace Launcher installation failed: %v\n", err)
}

// RDP publisher trust is an mstsc concept; FreeRDP launches need none of it.
func WriteMachineTrustPackage(pkg RDPMachineTrustPackage) (string, error) {
	return "", fmt.Errorf("machine trust packages are only used on Windows")
}

func InstallMachineRDPPublisherTrustFromFile(path string) error {
	return fmt.Errorf("machine trust installation is only used on Windows")
}

func EnsureMachineRDPPublisherTrust(pkg RDPMachineTrustPackage) error {
	// No-op by design: Linux RDP (FreeRDP) has no profile-signing trust model.
	return nil
}
