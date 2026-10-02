package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

// Session revocation — the user's own sessions. Revocation is immediate:
// every request looks its session up in the database, and deleting the row
// also destroys that session's copy of the vault key.

func (s *Server) handleListOwnSessions(w http.ResponseWriter, r *http.Request, user auth.User) {
	sessions, err := s.authenticator.ListOwnSessions(r.Context(), user, requestSessionToken(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// handleRevokeOwnSession handles DELETE /api/auth/sessions/{id}. Ending the
// session that makes the request behaves like a logout.
func (s *Server) handleRevokeOwnSession(w http.ResponseWriter, r *http.Request, user auth.User) {
	id, _ := url.PathUnescape(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/sessions/"), "/"))
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	kind, wasCurrent, err := s.authenticator.RevokeOwnSession(r.Context(), user, id, requestSessionToken(r))
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventSessionRevoked,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata:  map[string]any{"sessionKind": kind, "current": wasCurrent},
	})
	if wasCurrent {
		clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]any{"status": "signed_out", "current": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "kind": kind})
}

// handleRevokeOtherSessions is "sign out everywhere else".
func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request, user auth.User) {
	revoked, err := s.authenticator.RevokeOtherSessions(r.Context(), user, requestSessionToken(r))
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventSessionsRevokedAll,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata:  map[string]any{"revoked": revoked, "reason": "user_request"},
	})
	writeJSON(w, http.StatusOK, map[string]any{"revoked": revoked})
}

// handleAdminUserSessions serves /api/admin/users/{id}/sessions[/{sid}]. The
// caller is already verified to be an admin by handleAdminUserRoutes.
func (s *Server) handleAdminUserSessions(w http.ResponseWriter, r *http.Request, admin auth.User, userID string, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		sessions, err := s.localGroups.ListUserSessions(r.Context(), userID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case len(rest) == 0 && r.Method == http.MethodDelete:
		revoked, err := s.localGroups.RevokeAllUserSessions(r.Context(), userID)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventAdminSessionsRevokedAll,
			UserID:    admin.ID,
			UserName:  admin.Name,
			Metadata:  map[string]any{"targetUserId": userID, "revoked": revoked},
		})
		writeJSON(w, http.StatusOK, map[string]any{"revoked": revoked})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		sessionID, err := url.PathUnescape(strings.TrimSpace(rest[0]))
		if err != nil || sessionID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		kind, err := s.localGroups.RevokeUserSession(r.Context(), userID, sessionID)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventAdminSessionRevoked,
			UserID:    admin.ID,
			UserName:  admin.Name,
			Metadata:  map[string]any{"targetUserId": userID, "sessionKind": kind},
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "kind": kind})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}
