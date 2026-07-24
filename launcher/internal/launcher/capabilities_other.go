//go:build !windows && !linux

package launcher

// Capabilities on platforms without a dedicated launcher flow yet (macOS):
// only browser hand-offs are honestly supported.
func Capabilities() map[string]any {
	return map[string]any{
		"ssh":                  false,
		"sshCredentialHandoff": false,
		"rdp":                  false,
		"rdpCredentialHandoff": false,
		"rdpGateway":           false,
		"webPortal":            true,
	}
}
