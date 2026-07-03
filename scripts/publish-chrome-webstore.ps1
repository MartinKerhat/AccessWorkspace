param(
  [string]$PackagePath = "frontend/public/downloads/access-workspace-browser-extension-chrome-v0.2.5.zip",
  [switch]$Publish
)

$ErrorActionPreference = "Stop"

function Require-Env([string]$name) {
  $value = [Environment]::GetEnvironmentVariable($name)
  if ([string]::IsNullOrWhiteSpace($value)) {
    throw "Missing required environment variable: $name"
  }
  return $value
}

$clientId = Require-Env "CHROME_WEB_STORE_CLIENT_ID"
$clientSecret = Require-Env "CHROME_WEB_STORE_CLIENT_SECRET"
$refreshToken = Require-Env "CHROME_WEB_STORE_REFRESH_TOKEN"
$publisherId = Require-Env "CHROME_WEB_STORE_PUBLISHER_ID"
$extensionId = Require-Env "CHROME_WEB_STORE_EXTENSION_ID"

if (-not (Test-Path $PackagePath)) {
  throw "Package not found: $PackagePath"
}

$tokenResponse = Invoke-RestMethod -Method Post -Uri "https://oauth2.googleapis.com/token" -ContentType "application/x-www-form-urlencoded" -Body @{
  client_id = $clientId
  client_secret = $clientSecret
  refresh_token = $refreshToken
  grant_type = "refresh_token"
}

$accessToken = [string]$tokenResponse.access_token
if ([string]::IsNullOrWhiteSpace($accessToken)) {
  throw "Chrome Web Store access token could not be obtained."
}

$uploadUrl = "https://chromewebstore.googleapis.com/upload/v2/publishers/$publisherId/items/$extensionId`:upload"
$headers = @{
  Authorization = "Bearer $accessToken"
}

Invoke-WebRequest -Method Post -Uri $uploadUrl -Headers $headers -InFile $PackagePath -ContentType "application/zip" | Out-Null
Write-Host "Chrome Web Store package uploaded for item $extensionId."

if ($Publish) {
  $publishUrl = "https://chromewebstore.googleapis.com/v2/publishers/$publisherId/items/$extensionId`:publish"
  Invoke-RestMethod -Method Post -Uri $publishUrl -Headers $headers | Out-Null
  Write-Host "Chrome Web Store publish request submitted."
} else {
  Write-Host "Upload finished. Re-run with -Publish to submit the uploaded version for review."
}
