package launcherinfo

const (
	Version      = "0.5.7"
	ListenURL    = "127.0.0.1:47654"
	DownloadFile = "access-workspace-launcher-windows-amd64-v0.5.7.exe"

	// RDPTrustPath is the workspace API path exposing the public RDP publisher
	// trust configuration. It is joined onto the workspace base URL the launcher
	// learns at runtime from launch payloads (no host is hardcoded).
	RDPTrustPath = "/api/launcher/rdp-signing/public"
)
