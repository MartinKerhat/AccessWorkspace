param(
  [string]$SourceDir = "browser-extension/firefox",
  [string]$Channel = "unlisted"
)

$ErrorActionPreference = "Stop"

$webExt = Get-Command web-ext -ErrorAction SilentlyContinue
if (-not $webExt) {
  throw "web-ext is not installed. Install it first with: npm install -g web-ext"
}

$issuer = [Environment]::GetEnvironmentVariable("FIREFOX_AMO_JWT_ISSUER")
$secret = [Environment]::GetEnvironmentVariable("FIREFOX_AMO_JWT_SECRET")

if ([string]::IsNullOrWhiteSpace($issuer) -or [string]::IsNullOrWhiteSpace($secret)) {
  throw "Missing FIREFOX_AMO_JWT_ISSUER or FIREFOX_AMO_JWT_SECRET."
}

& $webExt.Source sign `
  --source-dir $SourceDir `
  --channel $Channel `
  --api-key $issuer `
  --api-secret $secret
