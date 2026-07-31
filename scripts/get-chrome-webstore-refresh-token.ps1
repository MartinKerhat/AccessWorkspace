# One-time helper: obtains the Chrome Web Store API refresh token for CI.
#
# Prerequisites (Google Cloud console, same Google account as the CWS
# publisher): a project with the "Chrome Web Store API" enabled and a
# "Desktop app" OAuth client — see .dev-notes/browser-extension-store-plan.md
# section C.
#
# Run it interactively:
#   .\scripts\get-chrome-webstore-refresh-token.ps1
# It opens the Google consent page, catches the redirect on a local port,
# exchanges the code, prints the refresh token, and offers to store all
# three CHROME_WEB_STORE_* secrets in the GitHub repo via `gh`.
#
# Fallback if the local catch fails: copy the `code` query parameter from the
# browser's address bar and re-run with -Code <value>.

param(
  [string]$ClientId = "",
  [string]$ClientSecret = "",
  [string]$Code = "",
  [int]$Port = 8818
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ClientId)) {
  $ClientId = Read-Host "OAuth client ID (ends with .apps.googleusercontent.com)"
}
if ([string]::IsNullOrWhiteSpace($ClientSecret)) {
  $ClientSecret = Read-Host "OAuth client secret"
}

$redirectUri = "http://localhost:$Port"
$scope = "https://www.googleapis.com/auth/chromewebstore"

if ([string]::IsNullOrWhiteSpace($Code)) {
  $authUrl = "https://accounts.google.com/o/oauth2/v2/auth" +
    "?client_id=$([uri]::EscapeDataString($ClientId))" +
    "&redirect_uri=$([uri]::EscapeDataString($redirectUri))" +
    "&response_type=code" +
    "&scope=$([uri]::EscapeDataString($scope))" +
    "&access_type=offline&prompt=consent"

  # Plain TCP listener (no admin rights needed, unlike HttpListener) to catch
  # the one redirect Google sends after consent.
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
  $listener.Start()
  Write-Host ""
  Write-Host "Opening the Google consent page. Sign in with the CHROME WEB STORE PUBLISHER account."
  Write-Host "If an 'unverified app' warning appears: Advanced -> Continue (only you use this client)."
  Write-Host ""
  Write-Host "If the browser shows an error (e.g. Google 500), paste this URL into another"
  Write-Host "browser window/incognito yourself - the redirect lands back here either way:"
  Write-Host $authUrl
  Write-Host ""
  Start-Process $authUrl

  try {
    $client = $listener.AcceptTcpClient()
    $stream = $client.GetStream()
    $reader = [System.IO.StreamReader]::new($stream)
    $requestLine = $reader.ReadLine()   # e.g. GET /?code=4/0Af...&scope=... HTTP/1.1

    $response = "HTTP/1.1 200 OK`r`nContent-Type: text/html`r`nConnection: close`r`n`r`n" +
      "<html><body style='font-family:sans-serif'><h2>Done - you can close this tab</h2>" +
      "<p>Return to the PowerShell window.</p></body></html>"
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($response)
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Flush()
    $client.Close()
  } finally {
    $listener.Stop()
  }

  if ($requestLine -notmatch "code=([^&\s]+)") {
    throw "No authorization code in the redirect ($requestLine). Google may have shown an error - check the browser tab."
  }
  $Code = [uri]::UnescapeDataString($Matches[1])
  Write-Host "Authorization code captured."
}

$tokenResponse = Invoke-RestMethod -Method Post -Uri "https://oauth2.googleapis.com/token" -ContentType "application/x-www-form-urlencoded" -Body @{
  client_id     = $ClientId
  client_secret = $ClientSecret
  code          = $Code
  grant_type    = "authorization_code"
  redirect_uri  = $redirectUri
}

$refreshToken = [string]$tokenResponse.refresh_token
if ([string]::IsNullOrWhiteSpace($refreshToken)) {
  throw "Token exchange succeeded but returned no refresh_token. Re-run - the consent must be a fresh approval (prompt=consent)."
}

Write-Host ""
Write-Host "CHROME_WEB_STORE_CLIENT_ID     = $ClientId"
Write-Host "CHROME_WEB_STORE_CLIENT_SECRET = $ClientSecret"
Write-Host "CHROME_WEB_STORE_REFRESH_TOKEN = $refreshToken"
Write-Host ""

# Sanity check: mint an access token with the refresh token right away.
$check = Invoke-RestMethod -Method Post -Uri "https://oauth2.googleapis.com/token" -ContentType "application/x-www-form-urlencoded" -Body @{
  client_id     = $ClientId
  client_secret = $ClientSecret
  refresh_token = $refreshToken
  grant_type    = "refresh_token"
}
if ([string]::IsNullOrWhiteSpace([string]$check.access_token)) {
  throw "Verification failed: refresh token did not yield an access token."
}
Write-Host "Verified: refresh token successfully minted an access token."

$gh = Get-Command gh -ErrorAction SilentlyContinue
if ($gh) {
  $answer = Read-Host "Store all three as GitHub repository secrets via gh now? (y/n)"
  if ($answer -eq "y") {
    gh secret set CHROME_WEB_STORE_CLIENT_ID --body $ClientId
    gh secret set CHROME_WEB_STORE_CLIENT_SECRET --body $ClientSecret
    gh secret set CHROME_WEB_STORE_REFRESH_TOKEN --body $refreshToken
    Write-Host "Secrets stored."
  }
} else {
  Write-Host "gh CLI not found - add the three values in GitHub -> Settings -> Secrets and variables -> Actions."
}
