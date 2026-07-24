$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

$versionFile = Join-Path $scriptDir "internal\\launcherinfo\\launcherinfo.go"
$match = Select-String -Path $versionFile -Pattern 'Version\s*=\s*"([^"]+)"'
if (-not $match) {
  throw "Unable to resolve launcher version from $versionFile"
}

$version = $match.Matches[0].Groups[1].Value
# Artifacts live under artifacts/launcher/<platform> (served via the backend
# download proxy / bind-mounted in dev). See artifacts/README.md.

# --- Windows (amd64) ---
$windowsDir = Join-Path $scriptDir "..\\artifacts\\launcher\\windows"
New-Item -ItemType Directory -Path $windowsDir -Force | Out-Null
$windowsArtifact = Join-Path $windowsDir ("access-workspace-launcher-windows-amd64-v{0}.exe" -f $version)

# -H=windowsgui: build as a Windows GUI binary so no console window appears when
# the launcher runs (install, background bridge, and protocol launches).
go build -ldflags="-H=windowsgui" -o $windowsArtifact .\cmd\access-workspace-launcher
if ($LASTEXITCODE -ne 0) { throw "Windows launcher build failed" }

# --- Linux (amd64) ---
# Distributed as a tarball; the extracted binary self-installs on first run
# (~/.local/bin + XDG desktop entries). Console binary — Linux has no
# windowsgui concept and install feedback goes to stdout.
$linuxDir = Join-Path $scriptDir "..\\artifacts\\launcher\\linux"
New-Item -ItemType Directory -Path $linuxDir -Force | Out-Null
$linuxBinary = Join-Path $env:TEMP "access-workspace-launcher"
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
try {
  go build -o $linuxBinary .\cmd\access-workspace-launcher
  if ($LASTEXITCODE -ne 0) { throw "Linux launcher build failed" }
} finally {
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
$linuxArtifact = Join-Path $linuxDir ("access-workspace-launcher-linux-amd64-v{0}.tar.gz" -f $version)
# Packaged in Go so the 0755 executable bit survives (Windows tar.exe drops it).
go run .\scripts\package_linux_tar.go -binary $linuxBinary -name "access-workspace-launcher" -out $linuxArtifact
if ($LASTEXITCODE -ne 0) { throw "Linux launcher tarball failed" }
Remove-Item $linuxBinary -ErrorAction SilentlyContinue

Write-Host ("Built launcher {0}" -f $version)
Write-Host ("Windows artifact: {0}" -f $windowsArtifact)
Write-Host ("Linux artifact:   {0}" -f $linuxArtifact)
