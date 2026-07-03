param(
  [string]$OutputPath = "frontend/public/downloads/access-workspace-browser-extension-chrome-v0.2.5.zip"
)

$ErrorActionPreference = "Stop"

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) ("access-workspace-chrome-" + [System.Guid]::NewGuid().ToString("N") + ".zip")

try {
  Compress-Archive -Path "browser-extension/chrome/*" -DestinationPath $tempZip
  Copy-Item -LiteralPath $tempZip -Destination $OutputPath -Force
} finally {
  Remove-Item -LiteralPath $tempZip -Force -ErrorAction SilentlyContinue
}

Write-Host "Chrome ZIP built at $OutputPath"
