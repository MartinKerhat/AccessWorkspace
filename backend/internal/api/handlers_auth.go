package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

// setSessionCookie stores the raw session token in the httpOnly cookie.
// Secure is always set — browsers treat http://localhost as trustworthy, so
// dev still works. SameSite=Lax: invite/reset links from email are top-level
// cross-site navigations and must still carry the session.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/api",
		MaxAge:   int(s.authenticator.SessionTTL().Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/api",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// loginResponse is LoginResult without the token — the cookie carries it;
// the token must never reach page JavaScript.
func loginResponse(result auth.LoginResult) map[string]any {
	return map[string]any{
		"user":         result.User,
		"authMode":     result.AuthMode,
		"capabilities": result.Capabilities,
	}
}

func (s *Server) handleAuthBootstrap(w http.ResponseWriter, r *http.Request) {
	bootstrap := s.authenticator.Bootstrap()
	payload := map[string]any{
		"authMode":           bootstrap.AuthMode,
		"localLoginEnabled":  bootstrap.LocalLoginEnabled,
		"microsoftLoginHint": false,
	}

	if s.adminConfig == nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}

	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	payload["microsoftLoginHint"] = adminBool(configAny, "Enabled", "entraEnabled") &&
		adminBool(configAny, "Configured", "entraConfigured")

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.ChangeOwnPassword(r.Context(), user, input.CurrentPassword, input.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventUserAccessUpdated,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata:  map[string]any{"action": "password_changed"},
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.AcceptInvite(r.Context(), input.Token, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	s.setSessionCookie(w, result.Token)
	writeJSON(w, http.StatusOK, loginResponse(result))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		// Record the attempt (no user id — the account may not exist / the
		// password was wrong). The username is stored for spray detection.
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventLoginFailed,
			UserName:  strings.TrimSpace(input.Username),
			Metadata:  map[string]any{"ip": clientIP(r), "reason": loginFailureReason(err)},
		})
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventLoginSucceeded,
		UserID:    result.User.ID,
		UserName:  result.User.Name,
		Metadata:  map[string]any{"ip": clientIP(r)},
	})
	s.setSessionCookie(w, result.Token)
	writeJSON(w, http.StatusOK, loginResponse(result))
}

// loginFailureReason maps an auth error to a coarse, non-sensitive tag for the
// audit trail (never reveals whether the account exists).
func loginFailureReason(err error) string {
	switch {
	case errors.Is(err, auth.ErrLockedOut):
		return "locked_out"
	case errors.Is(err, auth.ErrBlocked):
		return "blocked"
	default:
		return "invalid_credentials"
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, user auth.User) {
	token := auth.SessionTokenFromRequest(r)
	if err := s.authenticator.Logout(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	if user.ID != "" {
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventLogout,
			UserID:    user.ID,
			UserName:  user.Name,
		})
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

// handleSessionCookieUpgrade migrates a live pre-cookie session: the frontend
// finds a token still in localStorage on boot, sends it as a bearer once, and
// gets it back as the httpOnly cookie. Requires bearer auth explicitly — a
// cookie-authenticated call has nothing to upgrade.
func (s *Server) handleSessionCookieUpgrade(w http.ResponseWriter, r *http.Request, _ auth.User) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bearer token required"})
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.authenticator.CurrentUser(r.Context(), r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         user,
		"authMode":     s.authenticator.Bootstrap().AuthMode,
		"capabilities": auth.CapabilitiesForUser(user),
	})
}
