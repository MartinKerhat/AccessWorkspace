package launcherinfo

// RequiredVersion gates the launcher protocol; StatusURL/LaunchURL are the
// launcher's local loopback endpoints. Downloadable launcher builds are
// enumerated dynamically by the artifacts package.
const (
	RequiredVersion = "0.5.6"
	StatusURL       = "http://127.0.0.1:47654/status"
	LaunchURL       = "http://127.0.0.1:47654/launch"
)
