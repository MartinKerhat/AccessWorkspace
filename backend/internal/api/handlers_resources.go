package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
)

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.List(r.Context(), user, resources.Filter{
		Query: r.URL.Query().Get("q"),
		Type:  r.URL.Query().Get("type"),
		Host:  r.URL.Query().Get("host"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input resources.CreateResourceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.resources.Create(r.Context(), user, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handlePasswordOptions(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.ListPasswordOptions(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleResourceRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/resources/")
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := s.resources.Get(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodPut:
			var input resources.UpdateResourceInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			item, err := s.resources.Update(r.Context(), user, id, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPut && parts[1] == "app-registration-notifications" {
		var input resources.AppRegistrationNotificationPolicyUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		item, err := s.resources.UpdateAppRegistrationNotificationPolicies(r.Context(), user, id, input)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 2 && parts[1] == "connection-override" {
		switch r.Method {
		case http.MethodGet:
			item, err := s.resources.GetConnectionCredentialOverride(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodPut:
			var input resources.ConnectionCredentialOverrideInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			item, err := s.resources.SetConnectionCredentialOverride(r.Context(), user, id, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodDelete:
			if err := s.resources.ClearConnectionCredentialOverride(r.Context(), user, id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "archive":
			if err := s.resources.Archive(r.Context(), user, id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
			return
		case "reveal":
			result, err := s.resources.Reveal(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		case "launch":
			result, err := s.resources.Launch(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !auth.CapabilitiesForUser(user).CanViewAudit {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	filter := audit.ListFilter{
		Query:     strings.TrimSpace(r.URL.Query().Get("q")),
		EventType: strings.TrimSpace(r.URL.Query().Get("eventType")),
		Limit:     queryLimit(r, 100),
		Offset:    queryOffset(r),
	}
	items, total, err := s.audit.List(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	eventTypes, err := s.audit.ListEventTypes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"total":      total,
		"eventTypes": eventTypes,
	})
}

func (s *Server) handleRecentActivity(w http.ResponseWriter, r *http.Request, user auth.User) {
	limit := queryLimit(r, 20)
	items, err := s.audit.RecentForUser(r.Context(), user.ID, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUserNotifications(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.notifications == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []resources.UserNotification{}})
		return
	}
	items, err := s.notifications.ListForUser(r.Context(), user.ID, queryLimit(r, 25))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleListArchivedResources(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.ListArchived(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleArchivedResourceRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/archived-resources/")
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "restore" {
		if err := s.resources.Restore(r.Context(), user, id); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleUserNotificationRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.notifications == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/notifications/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "read" || r.Method != http.MethodPost {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.notifications.MarkRead(r.Context(), user.ID, parts[0]); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
