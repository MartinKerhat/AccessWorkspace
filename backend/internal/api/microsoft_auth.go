package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"access-workspace/backend/internal/auth"
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
