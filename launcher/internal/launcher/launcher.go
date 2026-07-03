package launcher

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
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"access-workspace/launcher/internal/launcherinfo"
	"access-workspace/launcher/internal/payload"
)

func ShowLaunchFailure(err error) {
	if err == nil {
		return
	}
	showLauncherMessageBox("Access Workspace launch failed.\n\n"+err.Error(), "Access Workspace Launcher")
}

func Run(item payload.LaunchPayload) error {
	resolved, err := resolveLaunchPayload(item)
	if err != nil {
		return err
	}
	item = resolved

	switch item.ResourceType {
	case "ssh":
		return runSSH(item)
	case "rdp":
		return runRDP(item)
	case "web_portal":
		return openURL(item.URL)
	default:
		return fmt.Errorf("unsupported resource type %q", item.ResourceType)
	}
}

func resolveLaunchPayload(item payload.LaunchPayload) (payload.LaunchPayload, error) {
	ticket := payload.MetadataString(item.Metadata, "launcherTicket")
	resolveURL := payload.MetadataString(item.Metadata, "launcherResolveUrl")
	if ticket == "" || resolveURL == "" {
		return item, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest(http.MethodGet, resolveURL, nil)
	if err != nil {
		return payload.LaunchPayload{}, fmt.Errorf("build launcher ticket request: %w", err)
	}
	request.Header.Set("X-Access-Workspace-Launcher-Version", launcherinfo.Version)
	response, err := client.Do(request)
	if err != nil {
		return payload.LaunchPayload{}, fmt.Errorf("resolve launcher ticket: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return payload.LaunchPayload{}, fmt.Errorf("resolve launcher ticket: unexpected status %d", response.StatusCode)
	}

	var resolved payload.LaunchPayload
	if err := json.NewDecoder(response.Body).Decode(&resolved); err != nil {
		return payload.LaunchPayload{}, fmt.Errorf("decode launcher ticket: %w", err)
	}
	if resolved.Metadata == nil {
		resolved.Metadata = map[string]interface{}{}
	}
	return resolved, nil
}

func runSSH(item payload.LaunchPayload) error {
	return runSSHPlatform(item)
}

func runRDP(item payload.LaunchPayload) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("rdp launcher preview currently supports Windows only")
	}

	host := strings.TrimSpace(item.Target)
	if host == "" {
		return fmt.Errorf("rdp payload is missing target host")
	}

	port := payload.MetadataString(item.Metadata, "port")
	if port == "" {
		port = "3389"
	}

	args := buildRDPArgs(host, port, item.Metadata)

	login := windowsConnectionIdentity(
		payload.MetadataString(item.Metadata, "connectionDomain"),
		payload.MetadataString(item.Metadata, "username"),
	)
	secret := payload.MetadataString(item.Metadata, "secretValue")
	cmdkeyTargets := []string{}
	if login != "" && secret != "" {
		cmdkeyTargets = buildRDPStoredCredentialTargets(host, port)
		for _, target := range cmdkeyTargets {
			cmdkey := exec.Command("cmdkey.exe", "/generic:"+target, "/user:"+login, "/pass:"+secret)
			hideWindow(cmdkey)
			if output, err := cmdkey.CombinedOutput(); err != nil {
				return fmt.Errorf("store rdp credentials: %w (%s)", err, strings.TrimSpace(string(output)))
			}
		}
	}

	tempFile := ""
	if login != "" {
		var err error
		var signRequired bool
		tempFile, signRequired, err = writeRDPProfile(item, host, port, item.Metadata)
		if err != nil {
			return err
		}
		if err := ensureRDPSigningTrust(item.Metadata); err != nil {
			return err
		}
		if signRequired {
			if err := signRDPProfile(item.Metadata, tempFile); err != nil {
				return err
			}
		}
		args = []string{tempFile}
	}

	command := exec.Command("mstsc.exe", args...)
	if err := command.Start(); err != nil {
		return err
	}
	go focusLaunchedWindow(command.Process.Pid)
	if len(cmdkeyTargets) > 0 {
		go clearRDPCredentialsLater(cmdkeyTargets)
	}
	return nil
}

func openURL(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("web payload is missing target url")
	}

	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}

func windowsConnectionIdentity(domain string, username string) string {
	username = strings.TrimSpace(username)
	domain = strings.TrimSpace(domain)
	if username == "" {
		return ""
	}
	if domain == "" {
		return username
	}
	if strings.Contains(username, "\\") || strings.Contains(username, "@") {
		return username
	}
	return domain + "\\" + username
}

func buildRDPStoredCredentialTargets(host string, port string) []string {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	targets := []string{}
	if host != "" {
		targets = append(targets, "TERMSRV/"+host)
	}
	port = strings.TrimSpace(port)
	if host != "" && port != "" {
		targets = append(targets, "TERMSRV/"+host+":"+port)
	}
	return targets
}

func clearRDPCredentialsLater(targets []string) {
	time.Sleep(10 * time.Minute)
	for _, target := range targets {
		command := exec.Command("cmdkey.exe", "/delete:"+target)
		hideWindow(command)
		_ = command.Run()
	}
}

func buildRDPArgs(host string, port string, metadata map[string]interface{}) []string {
	args := []string{fmt.Sprintf("/v:%s:%s", host, port)}
	if payload.MetadataBool(metadata, "connectionAdminSession") {
		args = append(args, "/admin")
	}
	args = append(args, "/f")
	return args
}

func writeRDPProfile(item payload.LaunchPayload, host string, port string, metadata map[string]interface{}) (string, bool, error) {
	profilePath, err := rdpProfilePath(item)
	if err != nil {
		return "", false, err
	}
	username := strings.TrimSpace(payload.MetadataString(metadata, "username"))
	domain := strings.TrimSpace(payload.MetadataString(metadata, "connectionDomain"))
	lines := buildRDPProfileLines(host, port, metadata, username, domain)
	content := strings.Join(lines, "\r\n") + "\r\n"

	if existing, err := os.ReadFile(profilePath); err == nil {
		if rdpProfileMatches(content, string(existing)) {
			signRequired := !rdpProfileHasSignature(string(existing))
			if !signRequired {
				signRequired = rdpProfileSignerChanged(profilePath, metadata)
			}
			return profilePath, signRequired, nil
		}
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("read rdp profile: %w", err)
	}

	tempPath := profilePath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content), 0o600); err != nil {
		return "", false, fmt.Errorf("write rdp profile: %w", err)
	}
	if err := os.Rename(tempPath, profilePath); err != nil {
		return "", false, fmt.Errorf("replace rdp profile: %w", err)
	}
	return profilePath, true, nil
}

func buildRDPProfileLines(host string, port string, metadata map[string]interface{}, username string, domain string) []string {
	lines := []string{
		fmt.Sprintf("full address:s:%s:%s", strings.TrimSpace(host), strings.TrimSpace(port)),
		"prompt for credentials:i:0",
		"promptcredentialonce:i:1",
		"session bpp:i:32",
		"smart sizing:i:1",
		"negotiate security layer:i:1",
		"enablecredsspsupport:i:1",
		"authentication level:i:0",
	}
	if username != "" {
		lines = append(lines, fmt.Sprintf("username:s:%s", username))
	}
	if domain != "" {
		lines = append(lines, fmt.Sprintf("domain:s:%s", domain))
	}
	width, height := preferredRDPDesktopSize()
	lines = append(lines,
		fmt.Sprintf("desktopwidth:i:%d", width),
		fmt.Sprintf("desktopheight:i:%d", height),
	)
	if payload.MetadataBool(metadata, "connectionAdminSession") {
		lines = append(lines, "administrative session:i:1")
	}
	lines = append(lines, "screen mode id:i:2")
	lines = append(lines, "displayconnectionbar:i:1")
	return lines
}

func rdpProfileMatches(expectedContent string, existingContent string) bool {
	expected := normalizeRDPProfileLines(expectedContent)
	existing := normalizeRDPProfileLines(existingContent)
	if len(existing) < len(expected) {
		return false
	}
	for index, line := range expected {
		if existing[index] != line {
			return false
		}
	}
	return true
}

func rdpProfileHasSignature(content string) bool {
	for _, line := range normalizeRDPProfileLines(content) {
		if strings.HasPrefix(line, "signscope:s:") || strings.HasPrefix(line, "signature:s:") {
			return true
		}
	}
	return false
}

func normalizeRDPProfileLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	rawLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func rdpProfilePath(item payload.LaunchPayload) (string, error) {
	baseDir, err := launcherDataDir()
	if err != nil {
		return "", err
	}
	profilesDir := filepath.Join(baseDir, "profiles", "rdp")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		return "", fmt.Errorf("create rdp profiles directory: %w", err)
	}
	profileName := stableProfileName(item) + ".rdp"
	return filepath.Join(profilesDir, profileName), nil
}

func launcherDataDir() (string, error) {
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not available")
	}
	return filepath.Join(localAppData, "AccessWorkspaceLauncher"), nil
}

var invalidProfileNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func stableProfileName(item payload.LaunchPayload) string {
	base := strings.TrimSpace(item.ResourceID)
	if base == "" {
		base = strings.TrimSpace(item.Target)
	}
	if base == "" {
		base = "connection"
	}
	base = strings.ReplaceAll(base, ":", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, "/", "_")
	base = invalidProfileNameChars.ReplaceAllString(base, "_")
	base = strings.Trim(base, "._-")
	if base == "" {
		base = "connection"
	}
	return base
}

func ensureRDPSigningTrust(metadata map[string]interface{}) error {
	if !payload.MetadataBool(metadata, "rdpSigningEnabled") {
		return nil
	}
	thumbprint := strings.ToUpper(strings.TrimSpace(payload.MetadataString(metadata, "rdpSigningThumbprintSha256")))
	pfxBase64 := strings.TrimSpace(payload.MetadataString(metadata, "rdpSigningPfxBase64"))
	pfxPassword := payload.MetadataString(metadata, "rdpSigningPfxPassword")
	if thumbprint == "" || pfxBase64 == "" {
		return fmt.Errorf("rdp signing is enabled but the private signing package is incomplete")
	}
	return ensureRDPSigningPrivateKeyImported(thumbprint, pfxBase64, pfxPassword)
}

func signRDPProfile(metadata map[string]interface{}, profilePath string) error {
	if !payload.MetadataBool(metadata, "rdpSigningEnabled") {
		return nil
	}
	thumbprint := strings.ToUpper(strings.TrimSpace(payload.MetadataString(metadata, "rdpSigningThumbprintSha256")))
	if thumbprint == "" {
		return fmt.Errorf("rdp signing is enabled but the signing thumbprint is missing")
	}

	command := exec.Command("rdpsign.exe", "/sha256", thumbprint, profilePath)
	hideWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("sign rdp profile: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if err := writeRDPProfileSignerThumbprint(profilePath, thumbprint); err != nil {
		return err
	}
	return nil
}

func ensureRDPSigningPrivateKeyImported(thumbprint string, pfxBase64 string, pfxPassword string) error {
	installed, err := certificateStoreContains("My", thumbprint)
	if err != nil {
		return err
	}
	if !installed {
		baseDir, err := launcherDataDir()
		if err != nil {
			return err
		}
		certDir := filepath.Join(baseDir, "certificates")
		if err := os.MkdirAll(certDir, 0o755); err != nil {
			return fmt.Errorf("create certificates directory: %w", err)
		}

		pfxPath := filepath.Join(certDir, "rdp-signing-test.pfx")

		if err := writeBase64File(pfxPath, pfxBase64); err != nil {
			return err
		}
		if err := importPFXToCurrentUserStore(pfxPath, pfxPassword); err != nil {
			return err
		}
	}
	return nil
}

func rdpProfileSignerChanged(profilePath string, metadata map[string]interface{}) bool {
	if !payload.MetadataBool(metadata, "rdpSigningEnabled") {
		return false
	}
	currentThumbprint := strings.ToUpper(strings.TrimSpace(payload.MetadataString(metadata, "rdpSigningThumbprintSha256")))
	if currentThumbprint == "" {
		return true
	}
	storedThumbprint, err := readRDPProfileSignerThumbprint(profilePath)
	if err != nil {
		return true
	}
	return storedThumbprint != currentThumbprint
}

func rdpProfileSignerThumbprintPath(profilePath string) string {
	return profilePath + ".signer-thumbprint"
}

func readRDPProfileSignerThumbprint(profilePath string) (string, error) {
	bytes, err := os.ReadFile(rdpProfileSignerThumbprintPath(profilePath))
	if err != nil {
		return "", err
	}
	return strings.ToUpper(strings.TrimSpace(string(bytes))), nil
}

func writeRDPProfileSignerThumbprint(profilePath string, thumbprint string) error {
	thumbprint = strings.ToUpper(strings.TrimSpace(thumbprint))
	if thumbprint == "" {
		return nil
	}
	if err := os.WriteFile(rdpProfileSignerThumbprintPath(profilePath), []byte(thumbprint), 0o600); err != nil {
		return fmt.Errorf("write rdp signer thumbprint marker: %w", err)
	}
	return nil
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

func cleanupCurrentUserTestRDPSigningCertificates(currentLeafThumbprint string, currentRootThumbprint string) error {
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$targets = @("+
				"@{ Store = 'My'; Path = 'Cert:\\CurrentUser\\My'; Subject = 'CN=Access Workspace Test RDP Publisher, O=Access Workspace'; Keep = '%s' },"+
				"@{ Store = 'TrustedPublisher'; Path = 'Cert:\\CurrentUser\\TrustedPublisher'; Subject = 'CN=Access Workspace Test RDP Publisher, O=Access Workspace'; Keep = '%s' },"+
				"@{ Store = 'Root'; Path = 'Cert:\\CurrentUser\\Root'; Subject = 'CN=Access Workspace Test RDP Root, O=Access Workspace'; Keep = '%s' }"+
				");"+
				"$results = @(); "+
				"foreach ($target in $targets) { "+
				"  $items = Get-ChildItem $target.Path -ErrorAction SilentlyContinue | Where-Object { $_.Subject -eq $target.Subject -and $_.Thumbprint.ToUpper() -ne $target.Keep }; "+
				"  foreach ($item in $items) { $results += ($target.Store + '|' + $item.Thumbprint.ToUpper()) } "+
				"}; "+
				"$results -join \"`n\"",
			strings.ToUpper(strings.TrimSpace(currentLeafThumbprint)),
			strings.ToUpper(strings.TrimSpace(currentLeafThumbprint)),
			strings.ToUpper(strings.TrimSpace(currentRootThumbprint)),
		),
	)
	hideWindow(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cleanup stale current-user rdp signing certificates: %w (%s)", err, strings.TrimSpace(string(output)))
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
		if err := deleteCertificateFromCurrentUserStore(parts[0], parts[1]); err != nil {
			return err
		}
	}
	return nil
}

func deleteCertificateFromCurrentUserStore(store string, thumbprint string) error {
	command := exec.Command("certutil.exe", "-user", "-delstore", store, strings.ToUpper(strings.TrimSpace(thumbprint)))
	hideWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		text := strings.ToLower(strings.TrimSpace(string(output)))
		if strings.Contains(text, "cannot find") || strings.Contains(text, "not found") {
			return nil
		}
		return fmt.Errorf("remove stale certificate from current user %s store: %w (%s)", store, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeBase64File(path string, encoded string) error {
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode certificate package: %w", err)
	}
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return fmt.Errorf("write certificate file: %w", err)
	}
	return nil
}

func importPFXToCurrentUserStore(path string, password string) error {
	command := exec.Command("certutil.exe", "-user", "-f", "-p", password, "-importpfx", "My", path)
	hideWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("import pfx into current user store: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func addCertificateToCurrentUserStore(store string, path string) error {
	command := exec.Command("certutil.exe", "-user", "-addstore", store, path)
	hideWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		text := strings.ToLower(strings.TrimSpace(string(output)))
		if strings.Contains(text, "already") {
			return nil
		}
		return fmt.Errorf("add certificate to %s store: %w (%s)", store, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func certificateStoreContains(store string, thumbprint string) (bool, error) {
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf("$item = Get-ChildItem Cert:\\CurrentUser\\%s | Where-Object { $_.Thumbprint -eq '%s' }; if ($item) { 'present' }", store, thumbprint),
	)
	hideWindow(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("check %s certificate store: %w (%s)", store, err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToLower(string(output)), "present"), nil
}

func addTrustedRDPPublisherThumbprint(thumbprint string) error {
	thumbprint = strings.ToUpper(strings.TrimSpace(thumbprint))
	if thumbprint == "" {
		return nil
	}

	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$path = 'HKCU:\\Software\\Policies\\Microsoft\\Windows NT\\Terminal Services'; "+
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
	hideWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("add trusted rdp publisher thumbprint: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	configured, current, err := trustedRDPPublisherThumbprintPresentAtPath(`HKCU:\Software\Policies\Microsoft\Windows NT\Terminal Services`, thumbprint)
	if err != nil {
		return err
	}
	if !configured {
		return fmt.Errorf("trusted rdp publisher thumbprint was not persisted to Windows policy (current=%s)", current)
	}
	return nil
}

func trustedRDPPublisherThumbprintPresentAtPath(registryPath string, thumbprint string) (bool, string, error) {
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf(
			"$path = '%s'; "+
				"$name = 'TrustedCertThumbprints'; "+
				"$current = (Get-ItemProperty -Path $path -Name $name -ErrorAction SilentlyContinue).$name; "+
				"if ($null -eq $current) { Write-Output '__missing__'; exit 0 }; "+
				"$value = [string]$current; "+
				"Write-Output $value; "+
				"$items = $value -split ',' | ForEach-Object { $_.Trim().ToUpper() } | Where-Object { $_ -ne '' }; "+
				"if ($items -contains '%s') { exit 0 } else { exit 3 }",
			registryPath,
			thumbprint,
		),
	)
	hideWindow(command)
	output, err := command.CombinedOutput()
	current := strings.TrimSpace(string(output))
	if current == "__missing__" {
		return false, current, nil
	}
	if err == nil {
		return true, current, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
		return false, current, nil
	}
	return false, current, fmt.Errorf("verify trusted rdp publisher thumbprint: %w (%s)", err, current)
}

func preferredRDPDesktopSize() (int, int) {
	screenWidth, screenHeight := currentDisplaySize()
	if screenWidth <= 0 || screenHeight <= 0 {
		screenWidth, screenHeight = 1600, 900
	}
	return screenWidth, screenHeight
}

func hideWindow(command *exec.Cmd) {
	if runtime.GOOS != "windows" || command == nil {
		return
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
