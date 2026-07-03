$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

$versionFile = Join-Path $scriptDir "internal\\launcherinfo\\launcherinfo.go"
$match = Select-String -Path $versionFile -Pattern 'Version\s*=\s*"([^"]+)"'
if (-not $match) {
  throw "Unable to resolve launcher version from $versionFile"
}

$version = $match.Matches[0].Groups[1].Value
$downloadsDir = Join-Path $scriptDir "..\\frontend\\public\\downloads"
New-Item -ItemType Directory -Path $downloadsDir -Force | Out-Null

$versioned = Join-Path $downloadsDir ("access-workspace-launcher-windows-amd64-v{0}.exe" -f $version)

go build -ldflags="-H=windowsgui" -o $versioned .\cmd\access-workspace-launcher

Write-Host ("Built launcher {0}" -f $version)
Write-Host ("Versioned artifact: {0}" -f $versioned)
