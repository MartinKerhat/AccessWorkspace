package audit

import "time"

type EventType string

const (
	EventResourceViewed     EventType = "resource_viewed"
	EventResourceRevealed   EventType = "resource_revealed"
	EventResourceFilled     EventType = "resource_filled"
	EventResourceLaunched   EventType = "resource_launched"
	EventResourceCreated    EventType = "resource_created"
	EventResourceUpdated    EventType = "resource_updated"
	EventResourceArchived   EventType = "resource_archived"
	EventResourceDeleted    EventType = "resource_deleted"
	EventResourceRestored   EventType = "resource_restored"
	EventUserAccessUpdated  EventType = "user_access_updated"
	EventUserDeleted        EventType = "user_deleted"
	EventLoginSucceeded     EventType = "login_succeeded"
	EventLoginFailed        EventType = "login_failed"
	EventLogout             EventType = "logout"
	EventVaultUnlocked      EventType = "vault_unlocked"
	EventVaultSetup         EventType = "vault_setup"
	EventVaultLocked        EventType = "vault_locked"
	EventVaultMethodAdded   EventType = "vault_method_added"
	EventVaultMethodRemoved EventType = "vault_method_removed"
	// Session revocation: a user ending one of their own sessions, a user
	// signing out everywhere else (also emitted by a password change), and the
	// admin equivalents against another user.
	EventSessionRevoked          EventType = "session_revoked"
	EventSessionsRevokedAll      EventType = "sessions_revoked_all"
	EventAdminSessionRevoked     EventType = "admin_session_revoked"
	EventAdminSessionsRevokedAll EventType = "admin_sessions_revoked_all"
	// Generator page: a self-signed certificate was produced server-side
	// (subject/profile only — the material itself is never stored or logged).
	EventToolCertificateGenerated EventType = "tool_certificate_generated"
)

type Event struct {
	ID           string         `json:"id"`
	EventType    EventType      `json:"eventType"`
	UserID       string         `json:"userId"`
	UserName     string         `json:"userName"`
	ResourceID   *string        `json:"resourceId,omitempty"`
	ResourceName *string        `json:"resourceName,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}
