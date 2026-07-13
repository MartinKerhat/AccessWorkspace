package launcherinfo

// RequiredVersion is only the FALLBACK minimum launcher version, used when no
// launcher artifact is published. Normally the requirement is derived at
// runtime from the newest artifact in the store (see api.requiredLauncherVersion),
// so shipping a new build there rolls the requirement without a backend rebuild.
// StatusURL/LaunchURL are the launcher's local loopback endpoints.
const (
	RequiredVersion = "0.5.8"
	StatusURL       = "http://127.0.0.1:47654/status"
	LaunchURL       = "http://127.0.0.1:47654/launch"
)
