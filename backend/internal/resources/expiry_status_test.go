package resources

import (
	"testing"
	"time"
)

func TestExpiryStatus(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		expiresAt *time.Time
		want      string
	}{
		// No expiry is the common case for Key Vault secrets here. Treating it
		// as "expiring" would bury the handful that actually have a date.
		{"no expiry", nil, "active"},
		{"expired yesterday", timePtr(now.Add(-24 * time.Hour)), "expired"},
		{"expires in an hour", timePtr(now.Add(time.Hour)), "expiring"},
		{"expires inside the window", timePtr(now.Add(29 * 24 * time.Hour)), "expiring"},
		{"expires beyond the window", timePtr(now.Add(31 * 24 * time.Hour)), "active"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := expiryStatus(testCase.expiresAt, now); got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

// Disabled beats expiry: an unusable secret is the more urgent fact, and the
// expiry is still on the detail view.
func TestKeyVaultStatusDisabledWinsOverExpiry(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)

	if got := keyVaultStatus(false, &expired, now); got != "disabled" {
		t.Fatalf("expected disabled, got %q", got)
	}
	if got := keyVaultStatus(true, &expired, now); got != "expired" {
		t.Fatalf("expected expired, got %q", got)
	}
	if got := keyVaultStatus(true, nil, now); got != "active" {
		t.Fatalf("expected active, got %q", got)
	}
}

// The catalog badge renders any non-active status, so app registrations and Key
// Vault secrets must agree on the vocabulary and on the window.
func TestKeyVaultStatusSharesAppRegistrationVocabulary(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	insideWindow := now.Add(ExpiryWarningWindow - time.Hour)

	if got := keyVaultStatus(true, &insideWindow, now); got != "expiring" {
		t.Fatalf("expected expiring, got %q", got)
	}
	if got := expiryStatus(&insideWindow, now); got != "expiring" {
		t.Fatalf("app registration path disagrees: %q", got)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
