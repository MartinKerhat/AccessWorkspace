package notifications

import (
	"context"
	"strings"
	"testing"
	"time"

	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
)

type stubResourceStore struct {
	resource resources.Resource
}

func (s stubResourceStore) Get(context.Context, string) (resources.Resource, error) {
	return s.resource, nil
}

type stubUserDirectory struct {
	users []auth.UserSummary
}

func (s stubUserDirectory) ListUsers(context.Context) ([]auth.UserSummary, error) {
	return s.users, nil
}

type stubPolicyStore struct {
	appRegistration resources.ExpiryNotificationPolicy
	keyVault        resources.ExpiryNotificationPolicy
}

func (s stubPolicyStore) GetAppRegistrationNotificationPolicy(context.Context) (resources.ExpiryNotificationPolicy, error) {
	return s.appRegistration, nil
}

func (s stubPolicyStore) GetKeyVaultNotificationPolicy(context.Context) (resources.ExpiryNotificationPolicy, error) {
	return s.keyVault, nil
}

func (s stubPolicyStore) GetNotificationEmailRuntime(context.Context) (NotificationEmailRuntimeConfig, error) {
	return NotificationEmailRuntimeConfig{}, nil
}

func testPolicies() stubPolicyStore {
	return stubPolicyStore{
		appRegistration: resources.ExpiryNotificationPolicy{
			Enabled:      true,
			ReminderDays: []int{30, 14, 7},
			Channels:     []resources.NotificationChannel{resources.NotificationChannelInApp},
		},
		keyVault: resources.ExpiryNotificationPolicy{
			Enabled:      true,
			ReminderDays: []int{30, 7},
			Channels:     []resources.NotificationChannel{resources.NotificationChannelInApp},
		},
	}
}

func TestExpiringItemsKeyVaultSecret(t *testing.T) {
	expiry := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{}, testPolicies())

	items, err := service.expiringItems(context.Background(), resources.Resource{
		ID:         "res-1",
		Name:       "billing-api-key",
		Type:       resources.TypeKeyVaultSecret,
		ObjectName: "billing-api-key",
		ExpiresAt:  &expiry,
	})
	if err != nil {
		t.Fatalf("expiringItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].keyID != "billing-api-key" || items[0].kind != "secret" {
		t.Fatalf("unexpected item identity: %+v", items[0])
	}
	if items[0].expiresAt == nil || !items[0].expiresAt.Equal(expiry) {
		t.Fatalf("expected the resource expiry to carry through, got %v", items[0].expiresAt)
	}
	// The Key Vault policy must win — reusing the app registration cadence
	// would make the two categories impossible to tune apart.
	if len(items[0].policy.ReminderDays) != 2 {
		t.Fatalf("expected the key vault policy, got %+v", items[0].policy)
	}
}

// A secret with no expiry still yields an item so the supersede pass can clean
// up reminders written before the expiry was removed; it just has no date, so
// no reminder is ever due for it.
func TestExpiringItemsKeyVaultSecretWithoutExpiry(t *testing.T) {
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{}, testPolicies())

	items, err := service.expiringItems(context.Background(), resources.Resource{
		ID:         "res-2",
		Name:       "no-expiry",
		Type:       resources.TypeKeyVaultSecret,
		ObjectName: "no-expiry",
	})
	if err != nil {
		t.Fatalf("expiringItems: %v", err)
	}
	if len(items) != 1 || items[0].expiresAt != nil {
		t.Fatalf("expected a single dateless item, got %+v", items)
	}
}

// A nil slice is the "does not participate" signal that stops evaluation before
// anything is written — including the supersede pass.
func TestExpiringItemsIgnoresOtherTypes(t *testing.T) {
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{}, testPolicies())

	for _, resourceType := range []resources.ResourceType{resources.TypeRDP, resources.TypeSSH, resources.TypeSharedSecret, resources.TypeWebPortal} {
		items, err := service.expiringItems(context.Background(), resources.Resource{ID: "res", Type: resourceType})
		if err != nil {
			t.Fatalf("expiringItems(%s): %v", resourceType, err)
		}
		if items != nil {
			t.Fatalf("expected %s to be skipped, got %+v", resourceType, items)
		}
	}
}

func TestExpiringItemsAppRegistrationUsesOverrides(t *testing.T) {
	expiry := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	credentialOverride := resources.ExpiryNotificationPolicy{
		Enabled:      true,
		ReminderDays: []int{1},
		Channels:     []resources.NotificationChannel{resources.NotificationChannelEmail},
	}
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{}, testPolicies())

	items, err := service.expiringItems(context.Background(), resources.Resource{
		ID:   "res-3",
		Name: "Anytime OAuth",
		Type: resources.TypeAppRegistration,
		AppCredentials: []resources.AppRegistrationCredential{
			{KeyID: "key-1", DisplayName: "prod secret", CredentialType: "secret", EndDateTime: &expiry},
			{KeyID: "key-2", CredentialType: "certificate", EndDateTime: &expiry, NotificationPolicyOverride: &credentialOverride},
		},
	})
	if err != nil {
		t.Fatalf("expiringItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected one item per credential, got %d", len(items))
	}
	if items[0].displayName != "prod secret" {
		t.Fatalf("expected the credential display name, got %q", items[0].displayName)
	}
	// An unnamed credential must still be identifiable in the reminder text.
	if items[1].displayName != "key-2" {
		t.Fatalf("expected the key id as fallback name, got %q", items[1].displayName)
	}
	if len(items[1].policy.ReminderDays) != 1 || items[1].policy.ReminderDays[0] != 1 {
		t.Fatalf("expected the per-credential override to win, got %+v", items[1].policy)
	}
}

func TestResolveRecipientsIncludesAdminsAndSkipsBlocked(t *testing.T) {
	users := []auth.UserSummary{
		{ID: "u-owner", Name: "Martin Kerhát", Email: "martin@example.com"},
		{ID: "u-admin", Name: "Ops Admin", Email: "ops@example.com", IsAdmin: true},
		{ID: "u-team", Name: "Team Member", Email: "team@example.com", LocalGroups: []string{"platform"}},
		{ID: "u-other", Name: "Unrelated", Email: "other@example.com"},
		{ID: "u-blocked", Name: "Blocked Admin", Email: "blocked@example.com", IsAdmin: true, Blocked: true},
	}
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{users: users}, testPolicies())

	recipients, err := service.resolveRecipients(context.Background(), "Martin Kerhát", "platform")
	if err != nil {
		t.Fatalf("resolveRecipients: %v", err)
	}

	got := map[string]bool{}
	for _, recipient := range recipients {
		got[recipient.ID] = true
	}
	for _, id := range []string{"u-owner", "u-admin", "u-team"} {
		if !got[id] {
			t.Fatalf("expected %s among recipients, got %v", id, got)
		}
	}
	if got["u-other"] {
		t.Fatal("an unrelated non-admin user must not be notified")
	}
	if got["u-blocked"] {
		t.Fatal("a blocked user must not be notified even when admin")
	}
}

// With no owner and no team set, admins alone keep the record from expiring in
// silence — the failure mode a hand-imported secret used to have.
func TestResolveRecipientsFallsBackToAdmins(t *testing.T) {
	users := []auth.UserSummary{
		{ID: "u-admin", Name: "Ops Admin", Email: "ops@example.com", IsAdmin: true},
		{ID: "u-other", Name: "Unrelated", Email: "other@example.com"},
	}
	service := NewService(nil, stubResourceStore{}, stubUserDirectory{users: users}, testPolicies())

	recipients, err := service.resolveRecipients(context.Background(), "", "")
	if err != nil {
		t.Fatalf("resolveRecipients: %v", err)
	}
	if len(recipients) != 1 || recipients[0].ID != "u-admin" {
		t.Fatalf("expected only the admin, got %+v", recipients)
	}
}

func TestReminderNotificationBodyAvoidsDuplicateName(t *testing.T) {
	expiry := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	resource := resources.Resource{ID: "res-1", Name: "billing-api-key"}
	recipient := auth.UserSummary{ID: "u-1", Email: "u@example.com"}

	keyVault := reminderNotification(resource, expiringItem{
		keyID:       "billing-api-key",
		displayName: "billing-api-key",
		kind:        "secret",
		expiresAt:   &expiry,
	}, recipient, 7)
	if want := "secret billing-api-key expires on"; !strings.Contains(keyVault.Body, want) {
		t.Fatalf("expected %q in body, got %q", want, keyVault.Body)
	}

	appRegistration := reminderNotification(resources.Resource{ID: "res-2", Name: "Anytime OAuth"}, expiringItem{
		keyID:       "key-1",
		displayName: "prod secret",
		kind:        "secret",
		expiresAt:   &expiry,
	}, recipient, 7)
	if want := "secret prod secret for Anytime OAuth expires on"; !strings.Contains(appRegistration.Body, want) {
		t.Fatalf("expected %q in body, got %q", want, appRegistration.Body)
	}
}

func TestCalendarDayDistanceUTC(t *testing.T) {
	now := time.Date(2026, 8, 12, 23, 30, 0, 0, time.UTC)
	// Same calendar day but hours apart is still "expires today", which is what
	// the reminder-day match depends on.
	if got := calendarDayDistanceUTC(now, time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := calendarDayDistanceUTC(now, time.Date(2026, 8, 19, 0, 30, 0, 0, time.UTC)); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
	if got := calendarDayDistanceUTC(now, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)); got != -2 {
		t.Fatalf("expected -2, got %d", got)
	}
}
