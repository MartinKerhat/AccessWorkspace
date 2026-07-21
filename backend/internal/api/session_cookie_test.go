package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"access-workspace/backend/internal/auth"
)

// fakeSessionAuth only needs SessionTTL; embedding the interface satisfies
// the rest (calling anything else panics, which would fail the test loudly).
type fakeSessionAuth struct {
	auth.Authenticator
}

func (fakeSessionAuth) SessionTTL() time.Duration { return 24 * time.Hour }

func newCookieTestServer() *Server {
	return &Server{
		authenticator: fakeSessionAuth{},
		frontendURL:   "https://workspace.example.com",
	}
}

func requestWithSessionCookie(method, origin string) *http.Request {
	r := httptest.NewRequest(method, "/api/resources", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "tok"})
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestOriginAllowed(t *testing.T) {
	s := newCookieTestServer()

	cases := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"GET always allowed", requestWithSessionCookie(http.MethodGet, "https://evil.example.com"), true},
		{"POST without cookie allowed (bearer clients)", httptest.NewRequest(http.MethodPost, "/api/resources", nil), true},
		{"POST cookie no origin allowed", requestWithSessionCookie(http.MethodPost, ""), true},
		{"POST cookie matching origin allowed", requestWithSessionCookie(http.MethodPost, "https://workspace.example.com"), true},
		{"POST cookie matching origin case-insensitive", requestWithSessionCookie(http.MethodPost, "https://WORKSPACE.example.com"), true},
		{"POST cookie foreign origin rejected", requestWithSessionCookie(http.MethodPost, "https://evil.example.com"), false},
		{"DELETE cookie foreign origin rejected", requestWithSessionCookie(http.MethodDelete, "https://evil.example.com"), false},
	}
	for _, tc := range cases {
		if got := s.originAllowed(tc.req); got != tc.want {
			t.Errorf("%s: originAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestOriginAllowedTrailingSlashFrontendURL(t *testing.T) {
	s := newCookieTestServer()
	s.frontendURL = "https://workspace.example.com/"
	if !s.originAllowed(requestWithSessionCookie(http.MethodPost, "https://workspace.example.com")) {
		t.Fatal("origin should match frontendURL with trailing slash")
	}
}

func TestSetSessionCookieAttributes(t *testing.T) {
	s := newCookieTestServer()
	rec := httptest.NewRecorder()
	s.setSessionCookie(rec, "raw-token")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != auth.SessionCookieName || c.Value != "raw-token" {
		t.Fatalf("unexpected cookie identity: %s=%s", c.Name, c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie must be HttpOnly+Secure+Lax, got HttpOnly=%v Secure=%v SameSite=%v", c.HttpOnly, c.Secure, c.SameSite)
	}
	if c.Path != "/api" {
		t.Fatalf("cookie path = %q, want /api", c.Path)
	}
	if c.MaxAge != int((24 * time.Hour).Seconds()) {
		t.Fatalf("cookie MaxAge = %d, want session TTL seconds", c.MaxAge)
	}
}

func TestClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	clearSessionCookie(rec)

	header := rec.Header().Get("Set-Cookie")
	if !strings.Contains(header, auth.SessionCookieName+"=") {
		t.Fatalf("expected clearing Set-Cookie, got %q", header)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected MaxAge<0 clearing cookie, got %+v", cookies)
	}
}
