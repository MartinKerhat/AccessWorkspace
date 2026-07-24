//go:build windows

package launcher

// Capabilities reports what this launcher build supports on this machine, so
// the web app can set expectations before a launch instead of users
// discovering platform gaps mid-connect.
func Capabilities() map[string]any {
	return map[string]any{
		"ssh":                  true,
		"sshCredentialHandoff": true,
		"rdp":                  true,
		"rdpCredentialHandoff": true,
		"rdpGateway":           true,
		"webPortal":            true,
	}
}
