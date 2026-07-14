package resources

import (
	"errors"
	"testing"

	"access-workspace/backend/internal/auth"
)

// accessDenied hides personal resources from non-owners as 404 rather than
// 403, so a resource id can't confirm someone owns a personal secret; shared
// resources keep 403 (their existence is not sensitive).
func TestAccessDeniedHidesPersonalFromNonOwners(t *testing.T) {
	owner := auth.User{ID: "owner-1"}
	other := auth.User{ID: "other-2", IsAdmin: true} // admin included on purpose

	personal := ResourceSummary{Personal: true, OwnerUserID: "owner-1"}
	shared := ResourceSummary{Personal: false, OwnerUserID: "owner-1"}

	// Non-owner (even admin) on a personal resource → 404, not 403.
	if err := accessDenied(other, personal); !errors.Is(err, ErrNotFound) {
		t.Fatalf("personal + non-owner: expected ErrNotFound, got %v", err)
	}
	// The owner mapping is 403 (in practice owners pass the gate and never
	// reach this), never 404 — we never hide a resource from its owner.
	if err := accessDenied(owner, personal); !errors.Is(err, ErrForbidden) {
		t.Fatalf("personal + owner: expected ErrForbidden, got %v", err)
	}
	// Shared resources keep 403 for anyone denied.
	if err := accessDenied(other, shared); !errors.Is(err, ErrForbidden) {
		t.Fatalf("shared: expected ErrForbidden, got %v", err)
	}
}
