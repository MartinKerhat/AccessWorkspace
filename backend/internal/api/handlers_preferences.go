package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/tools"
)

// GeneratorPreferenceStore keeps each user's last-used generator settings.
type GeneratorPreferenceStore interface {
	List(ctx context.Context, userID string) (map[string]json.RawMessage, error)
	Save(ctx context.Context, userID, part string, settings json.RawMessage) error
}

// GET /api/me/generator-preferences
func (s *Server) handleListGeneratorPreferences(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.generatorPreferences == nil {
		writeJSON(w, http.StatusOK, map[string]any{"preferences": map[string]any{}})
		return
	}
	items, err := s.generatorPreferences.List(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": items})
}

// PUT /api/me/generator-preferences/{part} — body is the settings object.
func (s *Server) handleSaveGeneratorPreference(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.generatorPreferences == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	part, _ := url.PathUnescape(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/generator-preferences/"), "/"))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body too large"})
		return
	}
	if err := s.generatorPreferences.Save(r.Context(), user.ID, part, json.RawMessage(body)); err != nil {
		if errors.Is(err, tools.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
