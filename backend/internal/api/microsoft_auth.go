package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"access-workspace/backend/internal/auth"
	"github.com/google/uuid"
)

type microsoftTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type microsoftProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

type microsoftGroupsResponse struct {
	Value []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
	} `json:"value"`
}

type microsoftIDTokenClaims struct {
	OID               string   `json:"oid"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
}

func exchangeMicrosoftCode(ctx context.Context, config any, code string) (microsoftTokenResponse, error) {
	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/token",
		strings.TrimRight(adminString(config, "Authority", "entraAuthority"), "/"),
		adminString(config, "TenantID", "entraTenantId"),
	)

	form := url.Values{}
	form.Set("client_id", adminString(config, "ClientID", "entraClientId"))
	form.Set("client_secret", adminString(config, "ClientSecret", "entraClientSecret"))
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", adminString(config, "RedirectURI", "entraRedirectUri"))
	form.Set("scope", microsoftScopes(config))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return microsoftTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return microsoftTokenResponse{}, err
	}
	defer response.Body.Close()

	var payload microsoftTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return microsoftTokenResponse{}, err
	}
	if response.StatusCode >= 400 {
		if payload.Description != "" {
			return microsoftTokenResponse{}, fmt.Errorf("microsoft token exchange failed: %s", payload.Description)
		}
		if payload.Error != "" {
			return microsoftTokenResponse{}, fmt.Errorf("microsoft token exchange failed: %s", payload.Error)
		}
		return microsoftTokenResponse{}, fmt.Errorf("microsoft token exchange failed with status %d", response.StatusCode)
	}
	return payload, nil
}

func resolveMicrosoftUser(ctx context.Context, config any, tokens microsoftTokenResponse) (auth.User, error) {
	profile, err := fetchMicrosoftProfile(ctx, tokens.AccessToken)
	if err != nil {
		return auth.User{}, err
	}

	groupSource := adminString(config, "GroupSource", "entraGroupSource")
	groups := []string{}
	if groupSource == "graph" {
		groups, err = fetchMicrosoftGroups(ctx, tokens.AccessToken)
		if err != nil {
			return auth.User{}, err
		}
	} else {
		groups, err = groupsFromIDToken(tokens.IDToken)
		if err != nil {
			return auth.User{}, err
		}
	}

	email := profile.Mail
	if email == "" {
		email = profile.UserPrincipalName
	}

	user := auth.User{
		ID:      profile.ID,
		Name:    profile.DisplayName,
		Email:   email,
		Groups:  groups,
		IsAdmin: slicesContains(groups, "ops-admins"),
	}
	if user.Name == "" || user.Email == "" {
		claims, err := parseMicrosoftIDToken(tokens.IDToken)
		if err == nil {
			if user.Name == "" {
				user.Name = claims.Name
			}
			if user.Email == "" {
				user.Email = claims.Email
				if user.Email == "" {
					user.Email = claims.PreferredUsername
				}
			}
			if user.ID == "" {
				user.ID = claims.OID
			}
		}
	}

	if user.ID == "" {
		user.ID = user.Email
	}
	return user, nil
}

func fetchMicrosoftProfile(ctx context.Context, accessToken string) (microsoftProfile, error) {
	return callMicrosoftJSON[microsoftProfile](ctx, "https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName", accessToken)
}

func fetchMicrosoftGroups(ctx context.Context, accessToken string) ([]string, error) {
	payload, err := callMicrosoftJSON[microsoftGroupsResponse](ctx, "https://graph.microsoft.com/v1.0/me/transitiveMemberOf/microsoft.graph.group?$select=id,displayName", accessToken)
	if err != nil {
		return nil, err
	}

	groups := make([]string, 0, len(payload.Value))
	for _, item := range payload.Value {
		if item.ID != "" {
			groups = append(groups, item.ID)
			continue
		}
		if item.DisplayName != "" {
			groups = append(groups, item.DisplayName)
		}
	}
	return groups, nil
}

func callMicrosoftJSON[T any](ctx context.Context, endpoint string, accessToken string) (T, error) {
	var zero T

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return zero, fmt.Errorf("microsoft graph request failed: %s", strings.TrimSpace(string(body)))
	}

	var payload T
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return zero, err
	}
	return payload, nil
}

func parseMicrosoftIDToken(idToken string) (microsoftIDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return microsoftIDTokenClaims{}, fmt.Errorf("invalid id token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return microsoftIDTokenClaims{}, err
	}

	var claims microsoftIDTokenClaims
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&claims); err != nil {
		return microsoftIDTokenClaims{}, err
	}
	return claims, nil
}

func groupsFromIDToken(idToken string) ([]string, error) {
	claims, err := parseMicrosoftIDToken(idToken)
	if err != nil {
		return nil, err
	}
	return claims.Groups, nil
}

func microsoftScopes(config any) string {
	base := []string{"openid", "profile", "email", "offline_access", "User.Read"}
	if adminString(config, "GroupSource", "entraGroupSource") == "graph" {
		base = append(base, "GroupMember.Read.All")
	}
	return strings.Join(base, " ")
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func (s *Server) handleMicrosoftStart(w http.ResponseWriter, r *http.Request) {
	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		log.Printf("microsoft auth start: load config failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=config_load_failed", http.StatusFound)
		return
	}

	enabled := adminBool(configAny, "Enabled", "entraEnabled")
	configured := adminBool(configAny, "Configured", "entraConfigured")
	if !enabled || !configured {
		log.Printf("microsoft auth start: unavailable enabled=%t configured=%t", enabled, configured)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_login_not_available", http.StatusFound)
		return
	}

	authority := strings.TrimRight(adminString(configAny, "Authority", "entraAuthority"), "/")
	tenantID := adminString(configAny, "TenantID", "entraTenantId")
	clientID := adminString(configAny, "ClientID", "entraClientId")
	redirectURI := adminString(configAny, "RedirectURI", "entraRedirectUri")

	if authority == "" || tenantID == "" || clientID == "" || redirectURI == "" {
		log.Printf("microsoft auth start: incomplete config authority=%t tenant=%t client=%t redirect=%t",
			authority != "", tenantID != "", clientID != "", redirectURI != "")
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_login_not_configured", http.StatusFound)
		return
	}

	state := uuid.NewString()
	authURL, err := url.Parse(fmt.Sprintf("%s/%s/oauth2/v2.0/authorize", authority, tenantID))
	if err != nil {
		log.Printf("microsoft auth start: invalid authority %q: %v", authority, err)
		http.Redirect(w, r, s.frontendURL+"?authError=invalid_microsoft_authority", http.StatusFound)
		return
	}

	query := authURL.Query()
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("response_mode", "query")
	query.Set("scope", microsoftScopes(configAny))
	query.Set("state", state)
	authURL.RawQuery = query.Encode()

	http.SetCookie(w, &http.Cookie{
		Name:     "aw_ms_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	log.Printf("microsoft auth start: redirecting to authorize endpoint tenant=%s redirect_uri=%s group_source=%s",
		tenantID, redirectURI, adminString(configAny, "GroupSource", "entraGroupSource"))
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func (s *Server) handleMicrosoftCallback(w http.ResponseWriter, r *http.Request) {
	if errorCode := r.URL.Query().Get("error"); errorCode != "" {
		log.Printf("microsoft auth callback: provider returned error=%s description=%s",
			errorCode, r.URL.Query().Get("error_description"))
		http.Redirect(w, r, s.frontendURL+"?authError="+url.QueryEscape(errorCode), http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie("aw_ms_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != state {
		log.Printf("microsoft auth callback: invalid state cookie_present=%t state_match=%t err=%v",
			err == nil && stateCookie != nil,
			err == nil && stateCookie != nil && stateCookie.Value == state,
			err)
		http.Redirect(w, r, s.frontendURL+"?authError=invalid_microsoft_state", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "aw_ms_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	if code == "" {
		log.Printf("microsoft auth callback: missing authorization code")
		http.Redirect(w, r, s.frontendURL+"?authError=missing_microsoft_code", http.StatusFound)
		return
	}

	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		log.Printf("microsoft auth callback: load config failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=config_load_failed", http.StatusFound)
		return
	}

	tokens, err := exchangeMicrosoftCode(r.Context(), configAny, code)
	if err != nil {
		log.Printf("microsoft auth callback: token exchange failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_token_exchange_failed", http.StatusFound)
		return
	}

	user, err := resolveMicrosoftUser(r.Context(), configAny, tokens)
	if err != nil {
		log.Printf("microsoft auth callback: user resolution failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_user_resolution_failed", http.StatusFound)
		return
	}

	result, err := s.authenticator.IssueSession(r.Context(), user, auth.ModeEntra)
	if err != nil {
		log.Printf("microsoft auth callback: session creation failed for user=%s: %v", user.ID, err)
		if errors.Is(err, auth.ErrBlocked) {
			http.Redirect(w, r, s.frontendURL+"?authError=user_blocked", http.StatusFound)
			return
		}
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_session_failed", http.StatusFound)
		return
	}

	log.Printf("microsoft auth callback: signed in user=%s email=%s groups=%d", user.ID, user.Email, len(user.Groups))
	// The session travels in the httpOnly cookie — never in the redirect URL
	// (query tokens land in browser history and proxy logs).
	s.setSessionCookie(w, result.Token)
	http.Redirect(w, r, s.frontendURL, http.StatusFound)
}
