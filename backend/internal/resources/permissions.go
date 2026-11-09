package resources

import (
	"slices"
	"strings"
)

type Viewer interface {
	GetID() string
	GetLocalGroups() []string
	GetIsAdmin() bool
}

// IsOwnerUser reports whether the viewer is the object's individual owner.
func IsOwnerUser(user Viewer, resource ResourceSummary) bool {
	return strings.TrimSpace(resource.OwnerUserID) != "" && resource.OwnerUserID == user.GetID()
}

// IsOwnerTeamMember reports whether the viewer belongs to the object's owner
// team. Personal objects have no owner team: they belong to one person only,
// whatever a stale ownerTeam value says.
func IsOwnerTeamMember(user Viewer, resource ResourceSummary) bool {
	team := strings.TrimSpace(resource.OwnerTeam)
	if resource.Personal || team == "" {
		return false
	}
	return slices.Contains(user.GetLocalGroups(), team)
}

// OwnsResource is the single "is this viewer an owner" rule: the owner user,
// or — on shared objects — any member of the owner team. Owner-team members
// get the same object-level rights as the owner user (full edit, archive,
// reveal); only admins reassign the owner user, and only the owner user may
// convert a shared object to personal.
func OwnsResource(user Viewer, resource ResourceSummary) bool {
	return IsOwnerUser(user, resource) || IsOwnerTeamMember(user, resource)
}

func CanAccess(user Viewer, resource ResourceSummary) bool {
	// Personal resources are visible ONLY to their owner — not admins, not groups,
	// not anyone else. This deliberately overrides the admin bypass below.
	if resource.Personal {
		return IsOwnerUser(user, resource)
	}
	if user.GetIsAdmin() {
		return true
	}
	if OwnsResource(user, resource) {
		return true
	}
	// No group and no user restriction means everyone with category access.
	if len(resource.AllowedGroups) == 0 && len(resource.AllowedUsers) == 0 {
		return true
	}
	if IsAllowedUser(user, resource) {
		return true
	}
	for _, group := range resource.AllowedGroups {
		if slices.Contains(user.GetLocalGroups(), group) {
			return true
		}
	}
	return false
}

func IsAllowedUser(user Viewer, resource ResourceSummary) bool {
	return slices.Contains(resource.AllowedUsers, user.GetID())
}

func MatchedAllowedGroups(user Viewer, resource ResourceSummary) []string {
	if user.GetIsAdmin() || resource.Personal || len(resource.AllowedGroups) == 0 {
		return nil
	}
	matches := make([]string, 0, len(resource.AllowedGroups))
	for _, group := range resource.AllowedGroups {
		if slices.Contains(user.GetLocalGroups(), group) {
			matches = append(matches, group)
		}
	}
	return matches
}

func FilterAllowed[T Viewer](user T, resources []ResourceSummary) []ResourceSummary {
	out := make([]ResourceSummary, 0, len(resources))
	for _, resource := range resources {
		if CanAccess(user, resource) {
			out = append(out, resource)
		}
	}
	return out
}
