package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
)

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	config, err := s.adminConfig.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleUpdateAdminConfig(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	config, err := s.adminConfig.Update(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleGenerateRDPSigningTestCertificate(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	view, err := s.adminConfig.GenerateTestRDPSigningCertificate(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleListLocalGroups(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	items, err := s.localGroups.ListLocalGroups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	items, err := s.localGroups.ListUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input auth.CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.localGroups.CreateUser(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	if input.Invite {
		invite, err := s.localGroups.IssueUserInvite(r.Context(), user, item.ID, "invite")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user": item, "invite": invite})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleAdminNotificationDeliveries(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if s.notifications == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []resources.NotificationDeliveryRecord{}})
		return
	}
	items, err := s.notifications.ListRecentEmailDeliveries(r.Context(), queryLimit(r, 20))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminUserRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/")
	parts := strings.Split(path, "/")
	id, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil || id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "reset-password" {
		// Destroys the user's vault and personal secrets (unrecoverable by
		// design), kills their sessions, and returns a one-time reset link.
		invite, err := s.localGroups.ResetUserPassword(r.Context(), user, id)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserAccessUpdated,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"action":                   "password_reset_forced",
				"targetUserId":             id,
				"personalResourcesDeleted": invite.PersonalResourcesDeleted,
			},
		})
		writeJSON(w, http.StatusOK, invite)
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "invite" {
		// Re-issues the one-time invite link (invalidates any previous one).
		invite, err := s.localGroups.IssueUserInvite(r.Context(), user, id, "invite")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, invite)
		return
	}

	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "visible-resources" {
		item, err := s.localGroups.GetUserAccess(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		items, err := s.resources.ExplainVisibleResources(r.Context(), previewUserFromAccess(item), resources.Filter{
			Query: r.URL.Query().Get("q"),
			Type:  r.URL.Query().Get("type"),
			Host:  r.URL.Query().Get("host"),
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := s.localGroups.GetUserAccess(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	case http.MethodPut:
		var input auth.UserAccessUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		item, err := s.localGroups.UpdateUserAccess(r.Context(), id, input)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserAccessUpdated,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"targetUserId":      item.ID,
				"targetUserName":    item.Name,
				"blocked":           item.Blocked,
				"directLocalGroups": item.DirectAssignedLocalGroups,
				"directRights":      item.DirectRights,
			},
		})
		writeJSON(w, http.StatusOK, item)
		return
	case http.MethodDelete:
		if id == user.ID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "you cannot delete your own account"})
			return
		}
		result, err := s.localGroups.DeleteUser(r.Context(), user, id)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserDeleted,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"targetUserId":              id,
				"personalResourcesDeleted":  result.PersonalResourcesDeleted,
				"sharedResourcesReassigned": result.SharedResourcesReassigned,
			},
		})
		writeJSON(w, http.StatusOK, result)
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleCreateLocalGroup(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input auth.LocalGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.localGroups.CreateLocalGroup(r.Context(), input); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleLocalGroupRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/admin/local-groups/")
	name, _ = url.PathUnescape(name)
	if name == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var input auth.LocalGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.localGroups.UpdateLocalGroup(r.Context(), name, input); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func previewUserFromAccess(item auth.UserAccessDetail) auth.User {
	localGroups := make([]string, 0, len(item.ResolvedLocalGroups))
	for _, group := range item.ResolvedLocalGroups {
		localGroups = append(localGroups, group.Name)
	}
	return auth.User{
		ID:          item.ID,
		Name:        item.Name,
		Email:       item.Email,
		LocalGroups: localGroups,
		Rights:      append([]string{}, item.Rights...),
		IsAdmin:     item.IsAdmin,
		Blocked:     item.Blocked,
	}
}
