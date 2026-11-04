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

	if got := keyVaultStatus(false, nil, &expired, now); got != "disabled" {
		t.Fatalf("expected disabled, got %q", got)
	}
	if got := keyVaultStatus(true, nil, &expired, now); got != "expired" {
		t.Fatalf("expected expired, got %q", got)
	}
	if got := keyVaultStatus(true, nil, nil, now); got != "active" {
		t.Fatalf("expected active, got %q", got)
	}
}

// A future active-from date labels an enabled secret "inactive"; disabled still
// wins, and once the window has started the expiry vocabulary applies again.
func TestKeyVaultStatusInactiveBeforeActiveFrom(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)
	expired := now.Add(-time.Hour)

	if got := keyVaultStatus(true, &future, nil, now); got != "inactive" {
		t.Fatalf("expected inactive, got %q", got)
	}
	if got := keyVaultStatus(false, &future, nil, now); got != "disabled" {
		t.Fatalf("expected disabled to win over inactive, got %q", got)
	}
	if got := keyVaultStatus(true, &past, &expired, now); got != "expired" {
		t.Fatalf("expected expired once the window has started, got %q", got)
	}
}

// The catalog badge renders any non-active status, so app registrations and Key
// Vault secrets must agree on the vocabulary and on the window.
func TestKeyVaultStatusSharesAppRegistrationVocabulary(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	insideWindow := now.Add(ExpiryWarningWindow - time.Hour)

	if got := keyVaultStatus(true, nil, &insideWindow, now); got != "expiring" {
		t.Fatalf("expected expiring, got %q", got)
	}
	if got := expiryStatus(&insideWindow, now); got != "expiring" {
		t.Fatalf("app registration path disagrees: %q", got)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
