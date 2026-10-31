//go:build !windows && !linux && !darwin

package launcher

// Capabilities on platforms without a dedicated launcher flow (e.g. BSDs):
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
