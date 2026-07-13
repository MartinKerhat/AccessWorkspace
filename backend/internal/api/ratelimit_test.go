package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiterFixedWindow(t *testing.T) {
	base := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	limiter := newIPRateLimiter(3, time.Minute)

	// First 3 hits in the window pass; the 4th is blocked.
	for i := 0; i < 3; i++ {
		if !limiter.allow("1.2.3.4", base) {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	if limiter.allow("1.2.3.4", base) {
		t.Fatalf("4th hit in window must be blocked")
	}

	// A different key is independent.
	if !limiter.allow("5.6.7.8", base) {
		t.Fatalf("distinct key must have its own budget")
	}

	// After the window elapses, the key resets.
	if !limiter.allow("1.2.3.4", base.Add(time.Minute+time.Second)) {
		t.Fatalf("window should reset after it elapses")
	}
}

func TestClientIPForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	if got := clientIP(r); got != "10.0.0.1" {
		t.Fatalf("expected host from RemoteAddr, got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("expected left-most XFF entry, got %q", got)
	}
}
