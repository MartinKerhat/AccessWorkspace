package auth

import (
	"context"
	"strings"
	"testing"
)

func TestClientLabel(t *testing.T) {
	cases := []struct {
		name string
		kind string
		ua   string
		want string
	}{
		{"edge windows", SessionKindWeb, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0", "Edge on Windows"},
		{"chrome mac", SessionKindWeb, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36", "Chrome on macOS"},
		{"firefox linux", SessionKindWeb, "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0", "Firefox on Linux"},
		{"safari iphone", SessionKindWeb, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", "Safari on iOS"},
		{"chrome android", SessionKindWeb, "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36", "Chrome on Android"},
		{"empty web", SessionKindWeb, "", "Unknown client"},
		{"empty extension", SessionKindExtension, "", "Browser extension"},
		{"extension edge", SessionKindExtension, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0", "Browser extension · Edge on Windows"},
		{"opaque agent", SessionKindWeb, "curl/8.0", "Unknown browser"},
	}
	for _, tc := range cases {
		if got := clientLabel(tc.kind, tc.ua); got != tc.want {
			t.Errorf("%s: clientLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSessionClientContextRoundTrip(t *testing.T) {
	ctx := WithSessionClient(context.Background(), SessionClient{IP: " 10.0.0.7 ", UserAgent: " agent "})
	got := sessionClientFrom(ctx)
	if got.IP != "10.0.0.7" || got.UserAgent != "agent" {
		t.Fatalf("unexpected client %+v", got)
	}

	if empty := sessionClientFrom(context.Background()); empty.IP != "" || empty.UserAgent != "" {
		t.Fatalf("expected zero client without context value, got %+v", empty)
	}

	long := WithSessionClient(context.Background(), SessionClient{UserAgent: strings.Repeat("x", 2000)})
	if got := sessionClientFrom(long); len(got.UserAgent) != 512 {
		t.Fatalf("expected user agent bounded to 512, got %d", len(got.UserAgent))
	}
}

func TestCurrentSessionHashSentinel(t *testing.T) {
	if got := currentSessionHash(""); got != noCurrentSession {
		t.Fatalf("empty token must map to the sentinel, got %q", got)
	}
	if got := currentSessionHash("tok"); !strings.HasPrefix(got, hashedTokenPrefix) {
		t.Fatalf("real token must hash, got %q", got)
	}
	// The sentinel must never collide with a stored hash.
	if strings.HasPrefix(noCurrentSession, hashedTokenPrefix) {
		t.Fatal("sentinel collides with hash prefix")
	}
}
