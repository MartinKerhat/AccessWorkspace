package auth

import "testing"

func TestResolveLocalGroupMatchesPreferDirectAssignment(t *testing.T) {
	user := User{
		ID:    "sam",
		Email: "sam@example.internal",
		Groups: []string{
			"external-support",
		},
	}
	localGroups := []LocalGroup{
		{
			Name:                 "support",
			AssignedUserIDs:      []string{"sam"},
			MappedExternalGroups: []string{"external-support"},
			Rights:               []string{"passwords.read"},
		},
	}

	matches := resolveLocalGroupMatches(user, localGroups)
	if len(matches) != 1 {
		t.Fatalf("expected one local group match, got %#v", matches)
	}
	if matches[0].AssignmentSource != LocalGroupAssignmentDirect {
		t.Fatalf("expected direct assignment to win, got %q", matches[0].AssignmentSource)
	}
	if matches[0].MatchedExternalGroup != "" {
		t.Fatalf("expected no external match marker for direct assignment, got %q", matches[0].MatchedExternalGroup)
	}
}

func TestDirectAssignedLocalGroupNamesUsesUserIDAndEmail(t *testing.T) {
	user := User{
		ID:    "sam",
		Email: "sam@example.internal",
	}
	localGroups := []LocalGroup{
		{Name: "support", AssignedUserIDs: []string{"sam"}},
		{Name: "web", AssignedUserIDs: []string{"sam@example.internal"}},
		{Name: "network", AssignedUserIDs: []string{"nina"}},
	}

	names := directAssignedLocalGroupNames(user, localGroups)
	if len(names) != 2 || names[0] != "support" || names[1] != "web" {
		t.Fatalf("expected support and web direct assignments, got %#v", names)
	}
}

func TestResolveAuthorizationFromInputsCombinesLocalGroupsAndDirectRights(t *testing.T) {
	user := User{
		ID:     "nina",
		Email:  "nina@example.internal",
		Groups: []string{"network-external"},
	}
	localGroups := []LocalGroup{
		{
			Name:                 "network",
			MappedExternalGroups: []string{"network-external"},
			Rights:               []string{"connections.read", "passwords.read"},
		},
	}
	directRights := map[string][]string{
		"network-external": {"keyvault.read"},
	}

	resolved := resolveAuthorizationFromInputs(user, localGroups, directRights)
	if len(resolved.LocalGroups) != 1 || resolved.LocalGroups[0] != "network" {
		t.Fatalf("expected resolved network local group, got %#v", resolved.LocalGroups)
	}
	if len(resolved.Rights) != 3 {
		t.Fatalf("expected combined rights from local group and direct rules, got %#v", resolved.Rights)
	}
}
