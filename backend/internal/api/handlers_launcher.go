package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"access-workspace/backend/internal/artifacts"
	"access-workspace/backend/internal/launcherinfo"
)

// requiredLauncherVersion is the version the app demands from installed
// launchers. It is derived from the newest published launcher artifact, so
// dropping a new build into the artifact store rolls the requirement forward
// without rebuilding the backend; the compiled constant is only the fallback
// when no artifact is available. Cached briefly so per-launch ticket
// resolution does not re-list a blob-backed store.
func (s *Server) requiredLauncherVersion(ctx context.Context) string {
	s.launcherVersionMu.Lock()
	defer s.launcherVersionMu.Unlock()
	if time.Now().Before(s.launcherVersionExpires) {
		return s.launcherVersionCached
	}
	version := launcherinfo.RequiredVersion
	if downloads, err := s.artifacts.LauncherDownloads(ctx); err == nil {
		if newest := artifacts.NewestVersion(downloads); newest != "" {
			version = newest
		}
	}
	s.launcherVersionCached = version
	s.launcherVersionExpires = time.Now().Add(time.Minute)
	return version
}

func (s *Server) handleLaunchTicketResolve(w http.ResponseWriter, r *http.Request) {
	required := s.requiredLauncherVersion(r.Context())
	// Older launchers must upgrade; a launcher newer than the published
	// artifacts (e.g. a dev build) is fine.
	if version := strings.TrimSpace(r.Header.Get("X-Access-Workspace-Launcher-Version")); version != "" && artifacts.CompareVersions(version, required) < 0 {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error":           "launcher upgrade required",
			"requiredVersion": required,
		})
		return
	}
	ticket := strings.TrimPrefix(r.URL.Path, "/api/launcher/tickets/")
	ticket, _ = url.PathUnescape(ticket)
	if strings.TrimSpace(ticket) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	result, err := s.resources.ResolveLaunchTicket(r.Context(), ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLauncherRuntime(w http.ResponseWriter, r *http.Request) {
	downloads, err := s.artifacts.LauncherDownloads(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	recommended := ""
	if len(downloads) > 0 {
		recommended = downloads[0].DownloadURL
	}
	requiredVersion := artifacts.NewestVersion(downloads)
	if requiredVersion == "" {
		requiredVersion = launcherinfo.RequiredVersion
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requiredVersion": requiredVersion,
		"statusUrl":       launcherinfo.StatusURL,
		"launchUrl":       launcherinfo.LaunchURL,
		"downloadUrl":     recommended,
		"downloads":       downloads,
	})
}

// handleArtifactDownload streams an artifact (launcher / browser extension) from
// the private store through the backend, so the store never needs to be reachable
// by the browser. Path: /api/artifacts/download/<category>/<name>.
func (s *Server) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/artifacts/download/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	category := parts[0]
	name, err := url.PathUnescape(parts[1])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	body, info, err := s.artifacts.Open(r.Context(), category, name)
	if err != nil {
		if errors.Is(err, artifacts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "download failed"})
		return
	}
	defer body.Close()

	contentType := "application/octet-stream"
	if info != nil && info.ContentType != "" {
		contentType = info.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	if info != nil && info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (s *Server) handleRDPSigningPublic(w http.ResponseWriter, r *http.Request) {
	config, err := s.adminConfig.GetRDPSigningPublic(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}
