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

func CanAccess(user Viewer, resource ResourceSummary) bool {
	if user.GetIsAdmin() {
		return true
	}
	if strings.TrimSpace(resource.OwnerUserID) != "" && resource.OwnerUserID == user.GetID() {
		return true
	}
	if resource.Personal {
		return false
	}
	if len(resource.AllowedGroups) == 0 {
		return true
	}
	for _, group := range resource.AllowedGroups {
		if slices.Contains(user.GetLocalGroups(), group) {
			return true
		}
	}
	return false
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
