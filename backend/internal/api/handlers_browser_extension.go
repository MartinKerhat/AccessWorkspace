package api

import (
	"encoding/json"
	"net/http"

	"access-workspace/backend/internal/artifacts"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/browserextinfo"
)

func (s *Server) handleBrowserExtensionRuntime(w http.ResponseWriter, r *http.Request) {
	packages, err := s.artifacts.ExtensionPackages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	defaultDownloadURL := ""
	var allFiles []artifacts.Artifact
	for _, pkg := range packages {
		allFiles = append(allFiles, pkg.Files...)
		if defaultDownloadURL == "" && pkg.Status == "available" && pkg.DownloadURL != "" {
			defaultDownloadURL = pkg.DownloadURL
		}
	}
	requiredVersion := artifacts.NewestVersion(allFiles)
	if requiredVersion == "" {
		requiredVersion = browserextinfo.RequiredVersion
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requiredVersion": requiredVersion,
		"browser":         "chromium",
		"downloadUrl":     defaultDownloadURL,
		"packages":        packages,
	})
}

func (s *Server) handleBrowserExtensionPortalMatch(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input browserExtensionPortalURLInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	items, err := s.resources.ListPortalCredentialMatches(r.Context(), user, input.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBrowserExtensionPortalFill(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input browserExtensionPortalFillInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.resources.FillPortalCredential(r.Context(), user, input.ResourceID, input.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleBrowserExtensionSession(w http.ResponseWriter, r *http.Request, user auth.User) {
	result, err := s.authenticator.IssueSession(r.Context(), user, s.authenticator.Bootstrap().AuthMode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        result.Token,
		"user":         result.User,
		"authMode":     result.AuthMode,
		"capabilities": result.Capabilities,
	})
}

func (s *Server) handleBrowserExtensionConnectToken(w http.ResponseWriter, r *http.Request, user auth.User) {
	result, err := s.authenticator.IssueBrowserExtensionConnectToken(r.Context(), user, s.authenticator.Bootstrap().AuthMode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleBrowserExtensionConnectExchange(w http.ResponseWriter, r *http.Request) {
	var input browserExtensionConnectExchangeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.ExchangeBrowserExtensionConnectToken(r.Context(), input.Token, input.InstallationID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        result.Token,
		"user":         result.User,
		"authMode":     result.AuthMode,
		"capabilities": result.Capabilities,
	})
}
