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
