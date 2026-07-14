package auth

import "slices"

const (
	LocalGroupAssignmentDirect                 = "direct_assignment"
	LocalGroupAssignmentMappedExternalGroup    = "mapped_external_group"
	LocalGroupAssignmentMatchingExternalGroup  = "matching_external_group_name"
)

type localGroupMatch struct {
	Group                LocalGroup
	AssignmentSource     string
	MatchedExternalGroup string
}

func resolveAuthorizationFromInputs(user User, localGroups []LocalGroup, directRights map[string][]string) User {
	matches := resolveLocalGroupMatches(user, localGroups)
	resolvedLocalGroups := make([]string, 0, len(matches))
	resolvedRights := []string{}

	for _, match := range matches {
		resolvedLocalGroups = append(resolvedLocalGroups, match.Group.Name)
		resolvedRights = append(resolvedRights, match.Group.Rights...)
	}

	for _, externalGroupID := range user.Groups {
		resolvedRights = append(resolvedRights, directRights[externalGroupID]...)
	}

	resolvedRights = append(resolvedRights, user.DirectRights...)

	if user.IsAdmin {
		resolvedRights = append(resolvedRights,
			"connections.read", "connections.edit", "connections.create",
			"keyvault.read", "keyvault.edit",
			"appregistrations.read", "appregistrations.edit",
			"passwords.read", "passwords.edit", "passwords.create",
			"audit.read", "admin.access",
		)
	}

	user.LocalGroups = uniqueNormalized(resolvedLocalGroups)
	user.Rights = uniqueNormalized(resolvedRights)
	user.IsAdmin = user.IsAdmin || slices.Contains(user.Rights, "admin.access")
	return user
}

func resolveLocalGroupMatches(user User, localGroups []LocalGroup) []localGroupMatch {
	items := make([]localGroupMatch, 0, len(localGroups))
	for _, group := range localGroups {
		if source, matchedExternalGroup, ok := assignmentSourceForUser(user, group); ok {
			items = append(items, localGroupMatch{
				Group:                group,
				AssignmentSource:     source,
				MatchedExternalGroup: matchedExternalGroup,
			})
		}
	}
	return items
}

func assignmentSourceForUser(user User, group LocalGroup) (string, string, bool) {
	if slices.Contains(group.AssignedUserIDs, user.ID) || slices.Contains(group.AssignedUserIDs, user.Email) {
		return LocalGroupAssignmentDirect, "", true
	}

	for _, mappedExternalGroup := range group.MappedExternalGroups {
		if slices.Contains(user.Groups, mappedExternalGroup) {
			return LocalGroupAssignmentMappedExternalGroup, mappedExternalGroup, true
		}
	}

	if slices.Contains(user.Groups, group.Name) {
		return LocalGroupAssignmentMatchingExternalGroup, group.Name, true
	}

	return "", "", false
}

func directAssignedLocalGroupNames(user User, localGroups []LocalGroup) []string {
	names := make([]string, 0, len(localGroups))
	for _, group := range localGroups {
		if slices.Contains(group.AssignedUserIDs, user.ID) || slices.Contains(group.AssignedUserIDs, user.Email) {
			names = append(names, group.Name)
		}
	}
	return uniqueNormalized(names)
}
