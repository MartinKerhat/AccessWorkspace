param(
  [string]$OutputPath = "frontend/public/downloads/access-workspace-browser-extension-firefox-v0.2.5.xpi"
)

$ErrorActionPreference = "Stop"

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) ("access-workspace-firefox-" + [System.Guid]::NewGuid().ToString("N") + ".zip")

try {
  Compress-Archive -Path "browser-extension/firefox/*" -DestinationPath $tempZip
  Copy-Item -LiteralPath $tempZip -Destination $OutputPath -Force
} finally {
  Remove-Item -LiteralPath $tempZip -Force -ErrorAction SilentlyContinue
}

Write-Host "Firefox XPI built at $OutputPath"
