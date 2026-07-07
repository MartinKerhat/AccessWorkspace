package launcher

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The launcher is a single, deployment-agnostic artifact: the same binary is
// downloaded from any Access Workspace deployment. It therefore learns which
// workspace to talk to at runtime rather than having a URL baked in.
//
// Every launch payload carries an absolute launcherResolveUrl pointing at the
// deployment that issued it. We derive the workspace origin from that URL and
// persist it here, so deployment-wide prerequisites (RDP publisher trust) can
// be fetched from the same deployment on the next RDP launch — no hardcoded URL.

type workspaceConfig struct {
	BaseURL string `json:"baseUrl"`
}

func workspaceConfigPath() (string, error) {
	dir, err := launcherDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspace.json"), nil
}

// rememberWorkspaceBaseURL persists the workspace origin (scheme://host) derived
// from a launch resolve URL. Best-effort: any failure is ignored, since this is
// a convenience cache, not a hard dependency.
func rememberWorkspaceBaseURL(resolveURL string) {
	origin := originFromURL(resolveURL)
	if origin == "" || origin == loadWorkspaceBaseURL() {
		return
	}
	dir, err := launcherDataDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path, err := workspaceConfigPath()
	if err != nil {
		return
	}
	bytes, err := json.Marshal(workspaceConfig{BaseURL: origin})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, bytes, 0o600)
}

// loadWorkspaceBaseURL returns the last persisted workspace origin, or "" if the
// launcher has not handled a launch from any deployment yet.
func loadWorkspaceBaseURL() string {
	path, err := workspaceConfigPath()
	if err != nil {
		return ""
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var config workspaceConfig
	if err := json.Unmarshal(bytes, &config); err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
