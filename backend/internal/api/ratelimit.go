package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipRateLimiter is a small fixed-window per-key limiter guarding the sensitive
// auth endpoints against online guessing from a single source. It is
// in-memory, so the limit is per replica — the persistent account lockout in
// the auth layer is the cross-replica guarantee; this is defense-in-depth and
// a cheap DoS brake. Keyed by client IP.
type ipRateLimiter struct {
	mu        sync.Mutex
	windows   map[string]*rateWindow
	limit     int
	window    time.Duration
	lastSwept time.Time
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		windows: map[string]*rateWindow{},
		limit:   limit,
		window:  window,
	}
}

// allow records a hit for key and reports whether it is within the limit.
func (l *ipRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic sweep of expired windows so the map cannot grow without
	// bound under a spray of distinct source IPs.
	if now.Sub(l.lastSwept) > l.window {
		for k, w := range l.windows {
			if now.After(w.resetAt) {
				delete(l.windows, k)
			}
		}
		l.lastSwept = now
	}

	w, ok := l.windows[key]
	if !ok || now.After(w.resetAt) {
		l.windows[key] = &rateWindow{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// clientIP extracts the caller's address, honoring a single X-Forwarded-For
// hop (the ingress) when present. Behind a trusted proxy the left-most entry
// is the real client; we take it best-effort for throttling purposes only.
func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
