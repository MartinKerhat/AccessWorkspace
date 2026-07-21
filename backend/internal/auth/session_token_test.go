package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionTokenFromRequestCookieFirst(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-token"})
	r.Header.Set("Authorization", "Bearer header-token")

	if got := SessionTokenFromRequest(r); got != "cookie-token" {
		t.Fatalf("expected cookie to win, got %q", got)
	}
}

func TestSessionTokenFromRequestBearerFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	r.Header.Set("Authorization", "Bearer header-token")

	if got := SessionTokenFromRequest(r); got != "header-token" {
		t.Fatalf("expected bearer fallback, got %q", got)
	}
}

func TestSessionTokenFromRequestEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	if got := SessionTokenFromRequest(r); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}

func TestSessionTokenFromRequestIgnoresEmptyCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: ""})
	r.Header.Set("Authorization", "Bearer header-token")

	if got := SessionTokenFromRequest(r); got != "header-token" {
		t.Fatalf("expected bearer when cookie empty, got %q", got)
	}
}
