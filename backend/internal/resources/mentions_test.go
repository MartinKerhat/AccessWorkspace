package resources

import (
	"context"
	"testing"
	"time"

	"access-workspace/backend/internal/auth"
)

func TestParseMentionIDsIgnoresProseAndBrokenTokens(t *testing.T) {
	// Both id shapes in use must parse: uuids in real deployments and slugs in
	// seeded data. Constraining this to uuids once left every seeded mention
	// rendering as raw text.
	notes := "VPN user: @[Heimstaden VPN MK](passwords:11111111-1111-1111-1111-111111111111)\n" +
		"SQL: @[Prod SQL](keyvault:res-kv)\n" +
		"mail me @ someone, @[missing paren](passwords:res-web, " +
		"@[dup](passwords:11111111-1111-1111-1111-111111111111)"

	ids := ParseMentionIDs(notes)
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique ids, got %#v", ids)
	}
	if ids[0] != "11111111-1111-1111-1111-111111111111" || ids[1] != "res-kv" {
		t.Fatalf("expected ids in order of appearance, got %#v", ids)
	}
	if got := ParseMentionIDs("no mentions here at all @ x"); len(got) != 0 {
		t.Fatalf("expected no ids, got %#v", got)
	}
	// Ids are opaque: case must survive, because lookup is an exact match.
	mixed := ParseMentionIDs("@[Mixed](passwords:Res-WEB)")
	if len(mixed) != 1 || mixed[0] != "Res-WEB" {
		t.Fatalf("expected the id case to be preserved, got %#v", mixed)
	}
}

// The three disclosure states are the whole security surface of this feature:
// accessible reveals name+username, denied reveals ONLY the name (deliberate —
// a broadly shared connection may reference credentials a smaller group owns),
// and hidden reveals nothing at all.
func TestResolveMentionsDisclosurePerViewer(t *testing.T) {
	archivedAt := time.Now()
	store := &browserExtensionStore{
		items: map[string]Resource{
			"shared": {
				ID: "shared", Name: "Prod SQL", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", Username: "sa",
				AllowedGroups: []string{"sales"},
			},
			"other": {
				ID: "other", Name: "Ops Only SQL", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", Username: "sa-ops",
				AllowedGroups: []string{"ops"},
			},
			"personal": {
				ID: "personal", Name: "Alice Private", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", Personal: true,
			},
			"archived": {
				ID: "archived", Name: "Old SQL", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", ArchivedAt: &archivedAt,
			},
			"connection": {
				ID: "connection", Name: "Some RDP", Type: TypeRDP, Category: "connections",
				Owner: "Alice", OwnerUserID: "alice",
			},
			"kv-open": {
				ID: "kv-open", Name: "Payroll KV", Type: TypeKeyVaultSecret, Category: "keyvault",
				Owner: "Alice", OwnerUserID: "alice",
				VaultName: "kv-prod", ObjectName: "payroll-api-key", ObjectVersion: "abc123",
			},
			"kv-closed": {
				ID: "kv-closed", Name: "Ops KV", Type: TypeKeyVaultSecret, Category: "keyvault",
				Owner: "Alice", OwnerUserID: "alice", AllowedGroups: []string{"ops"},
				VaultName: "kv-ops", ObjectName: "ops-api-key",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	sales := auth.User{ID: "rita", Name: "Rita", Rights: []string{"passwords.read", "keyvault.read"}, LocalGroups: []string{"sales"}}

	targets, err := service.ResolveMentions(context.Background(), sales,
		[]string{"shared", "other", "personal", "archived", "connection", "missing"})
	if err != nil {
		t.Fatalf("resolve mentions: %v", err)
	}
	if len(targets) != 6 {
		t.Fatalf("expected one target per id, got %d", len(targets))
	}

	if targets[0].State != MentionAccessible || targets[0].Name != "Prod SQL" || targets[0].Username != "sa" {
		t.Fatalf("expected the shared-with-sales object to be accessible with name and username, got %#v", targets[0])
	}
	if targets[1].State != MentionDenied {
		t.Fatalf("expected an object shared with another group to be denied, got %#v", targets[1])
	}
	if targets[1].Name != "Ops Only SQL" {
		t.Fatalf("denied must still disclose the name, got %#v", targets[1])
	}
	if targets[1].Username != "" {
		t.Fatalf("denied must NOT disclose the username, got %#v", targets[1])
	}
	for index, label := range map[int]string{2: "personal", 3: "archived", 4: "connection", 5: "missing"} {
		if targets[index].State != MentionHidden {
			t.Fatalf("expected %s to be hidden, got %#v", label, targets[index])
		}
		if targets[index].Name != "" || targets[index].Username != "" {
			t.Fatalf("hidden must disclose nothing for %s, got %#v", label, targets[index])
		}
	}
}

// Key Vault secrets resolve like any other mention — the frontend routes an
// accessible one to the Key Vault module's own reveal modal, so nothing extra is
// returned here. What matters is that the states are right for both categories.
func TestResolveMentionsHandlesKeyVaultTargets(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"kv-open": {
				ID: "kv-open", Name: "Payroll KV", Type: TypeKeyVaultSecret, Category: "keyvault",
				Owner: "Alice", OwnerUserID: "alice",
				VaultName: "kv-prod", ObjectName: "payroll-api-key", ObjectVersion: "abc123",
			},
			"kv-closed": {
				ID: "kv-closed", Name: "Ops KV", Type: TypeKeyVaultSecret, Category: "keyvault",
				Owner: "Alice", OwnerUserID: "alice", AllowedGroups: []string{"ops"},
				VaultName: "kv-ops", ObjectName: "ops-api-key",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	viewer := auth.User{ID: "rita", Name: "Rita", Rights: []string{"keyvault.read"}, LocalGroups: []string{"sales"}}

	targets, err := service.ResolveMentions(context.Background(), viewer, []string{"kv-open", "kv-closed"})
	if err != nil {
		t.Fatalf("resolve mentions: %v", err)
	}

	open := targets[0]
	if open.State != MentionAccessible || open.Name != "Payroll KV" || open.Category != "keyvault" {
		t.Fatalf("expected the shared key vault secret to be accessible, got %#v", open)
	}

	closed := targets[1]
	if closed.State != MentionDenied || closed.Name != "Ops KV" {
		t.Fatalf("expected denied with the name disclosed, got %#v", closed)
	}
	if closed.Username != "" {
		t.Fatalf("denied must disclose only the name, got %#v", closed)
	}
}

// The picker is scoped to the caller: personal objects never appear, non-
// mentionable categories never appear, and an admin sees more than a colleague.
func TestListMentionCandidatesScopedToCaller(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"sales-pw": {
				ID: "sales-pw", Name: "Sales SQL", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", AllowedGroups: []string{"sales"},
			},
			"ops-pw": {
				ID: "ops-pw", Name: "Ops SQL", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Alice", OwnerUserID: "alice", AllowedGroups: []string{"ops"},
			},
			"vault": {
				ID: "vault", Name: "Vault Secret", Type: TypeKeyVaultSecret, Category: "keyvault",
				Owner: "Alice", OwnerUserID: "alice",
			},
			"mine": {
				ID: "mine", Name: "My Private", Type: TypeSharedSecret, Category: "passwords",
				Owner: "Rita", OwnerUserID: "rita", Personal: true,
			},
			"rdp": {
				ID: "rdp", Name: "Some RDP", Type: TypeRDP, Category: "connections",
				Owner: "Alice", OwnerUserID: "alice",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)

	sales := auth.User{ID: "rita", Name: "Rita", Rights: []string{"passwords.read", "keyvault.read"}, LocalGroups: []string{"sales"}}
	got, err := service.ListMentionCandidates(context.Background(), sales, "")
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	names := map[string]bool{}
	for _, item := range got {
		names[item.Name] = true
	}
	if !names["Sales SQL"] || !names["Vault Secret"] {
		t.Fatalf("expected the shared password and vault secret to be offered, got %#v", got)
	}
	if names["Ops SQL"] {
		t.Fatal("expected an object shared with another group not to be offered")
	}
	if names["My Private"] {
		t.Fatal("expected personal objects never to be offered")
	}
	if names["Some RDP"] {
		t.Fatal("expected connections not to be mentionable")
	}

	admin := auth.User{ID: "root", Name: "Root", IsAdmin: true}
	adminCandidates, err := service.ListMentionCandidates(context.Background(), admin, "")
	if err != nil {
		t.Fatalf("list candidates as admin: %v", err)
	}
	if len(adminCandidates) <= len(got) {
		t.Fatalf("expected an admin to see more candidates than a group member, admin=%d member=%d", len(adminCandidates), len(got))
	}

	filtered, err := service.ListMentionCandidates(context.Background(), admin, "vault")
	if err != nil {
		t.Fatalf("list filtered candidates: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Name != "Vault Secret" {
		t.Fatalf("expected the query to narrow by name, got %#v", filtered)
	}
}
