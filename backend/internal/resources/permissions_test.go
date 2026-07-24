package resources

import "testing"

type fakeUser struct {
	id     string
	groups []string
	admin  bool
}

func (u fakeUser) GetID() string            { return u.id }
func (u fakeUser) GetLocalGroups() []string { return u.groups }
func (u fakeUser) GetIsAdmin() bool         { return u.admin }

func TestFilterAllowedReturnsMatchingGroups(t *testing.T) {
	items := []ResourceSummary{
		{ID: "1", AllowedGroups: []string{"platform"}},
		{ID: "2", AllowedGroups: []string{"support"}},
	}

	got := FilterAllowed(fakeUser{groups: []string{"support"}}, items)

	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("expected only support resource, got %#v", got)
	}
}

func TestCanAccessAllowsAdmin(t *testing.T) {
	resource := ResourceSummary{AllowedGroups: []string{"restricted"}}
	if !CanAccess(fakeUser{admin: true}, resource) {
		t.Fatalf("expected admin to access resource")
	}
}

func TestCanAccessAllowsEveryoneWhenNoGroupsAreConfigured(t *testing.T) {
	resource := ResourceSummary{AllowedGroups: []string{}}
	if !CanAccess(fakeUser{}, resource) {
		t.Fatalf("expected empty allowed groups to be visible to everyone")
	}
}

func TestCanAccessAllowsDirectlySharedUser(t *testing.T) {
	resource := ResourceSummary{AllowedUsers: []string{"alice"}}
	if !CanAccess(fakeUser{id: "alice"}, resource) {
		t.Fatalf("expected directly shared user to access resource")
	}
	if CanAccess(fakeUser{id: "bob"}, resource) {
		t.Fatalf("expected non-listed user to be denied when only user sharing is configured")
	}
}

func TestCanAccessMatchesGroupOrUser(t *testing.T) {
	resource := ResourceSummary{AllowedGroups: []string{"platform"}, AllowedUsers: []string{"alice"}}
	if !CanAccess(fakeUser{id: "bob", groups: []string{"platform"}}, resource) {
		t.Fatalf("expected group member to access resource with combined sharing")
	}
	if !CanAccess(fakeUser{id: "alice"}, resource) {
		t.Fatalf("expected listed user to access resource with combined sharing")
	}
	if CanAccess(fakeUser{id: "carol", groups: []string{"support"}}, resource) {
		t.Fatalf("expected user matching neither list to be denied")
	}
}

func TestCanAccessDeniesAdminOnPersonalResource(t *testing.T) {
	resource := ResourceSummary{Personal: true, OwnerUserID: "alice"}
	if CanAccess(fakeUser{id: "martin", admin: true}, resource) {
		t.Fatalf("expected admin to be denied access to another user's personal resource")
	}
}

func TestCanAccessAllowsOwnerOnPersonalResource(t *testing.T) {
	resource := ResourceSummary{Personal: true, OwnerUserID: "alice"}
	if !CanAccess(fakeUser{id: "alice"}, resource) {
		t.Fatalf("expected owner to access their own personal resource")
	}
}
