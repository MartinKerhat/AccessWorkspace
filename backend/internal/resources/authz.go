package resources

import (
	"context"
	"slices"
	"strings"

	"access-workspace/backend/internal/auth"
)

func (s *Service) ExplainVisibleResources(ctx context.Context, user auth.User, filter Filter) ([]VisibleResourceSummary, error) {
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return explainVisibleResourcesForUser(user, items), nil
}

// isPersonalPasswordOverrideOption gates what the override picker offers:
// only the caller's personal saved passwords, never shared or web-portal
// credentials. Existing overrides that predate this rule keep working via
// the more lenient isPasswordOverrideCandidate.
func isPersonalPasswordOverrideOption(resource ResourceSummary) bool {
	return isPasswordOverrideCandidateSummary(resource) && resource.Personal && resource.Type == TypeSharedSecret
}

type devViewer auth.User

func (u devViewer) GetID() string            { return u.ID }
func (u devViewer) GetLocalGroups() []string { return u.LocalGroups }
func (u devViewer) GetIsAdmin() bool         { return u.IsAdmin }

func canViewResource(user auth.User, resource ResourceSummary) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].View && CanAccess(devViewer(user), resource)
}

// accessDenied is the error for a resource the user may not act on. Personal
// resources are hidden from non-owners entirely (404, not 403), so holding a
// resource id cannot be used to confirm that someone owns a personal secret —
// consistent with "personal is invisible to everyone but its owner". Shared
// resources keep 403: their existence is not sensitive.
func accessDenied(user auth.User, resource ResourceSummary) error {
	if resource.Personal && resource.OwnerUserID != user.ID {
		return ErrNotFound
	}
	return ErrForbidden
}

func canRevealResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal && CanAccess(devViewer(user), resource.Summary())
}

func canLaunchResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Launch && CanAccess(devViewer(user), resource.Summary())
}

func canFillPasswordResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal &&
		resource.CopyAllowed &&
		CanAccess(devViewer(user), resource.Summary())
}

func canRevealStoredPassword(user auth.User, resource Resource) bool {
	if CategoryForType(resource.Type) != "passwords" {
		return false
	}
	if !auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal ||
		!CanAccess(devViewer(user), resource.Summary()) {
		return false
	}
	// Owners and admins can always reveal — they control the policy flags anyway.
	if resource.OwnerUserID == user.ID || (user.IsAdmin && !resource.Personal) {
		return true
	}
	// Web portals separate seeing the password (revealAllowed) from letting the
	// browser extension fill it (copyAllowed), so shared portal credentials can
	// be usable without being readable. Saved passwords keep one copy flag.
	if resource.Type == TypeWebPortal {
		return resource.RevealAllowed
	}
	return resource.CopyAllowed
}

func explainVisibleResourcesForUser(user auth.User, items []ResourceSummary) []VisibleResourceSummary {
	capabilities := auth.CapabilitiesForUser(user)
	viewer := devViewer(user)
	out := make([]VisibleResourceSummary, 0, len(items))
	for _, item := range items {
		category := capabilities.Categories[item.Category]
		if !category.View {
			continue
		}
		scope, matchedGroups, ok := visibilityScopeForResource(viewer, item)
		if !ok {
			continue
		}
		out = append(out, VisibleResourceSummary{
			ResourceSummary:     item,
			CategoryAccessRight: categoryAccessRight(user, item.Category),
			VisibilityScope:     scope,
			MatchedLocalGroups:  matchedGroups,
		})
	}
	return out
}

func visibilityScopeForResource(user devViewer, item ResourceSummary) (string, []string, bool) {
	if item.Personal {
		// Personal resources are visible ONLY to their owner — admins included.
		if item.OwnerUserID == user.ID {
			return "personal", nil, true
		}
		return "", nil, false
	}
	if item.OwnerUserID == user.ID {
		return "owner", nil, true
	}
	if user.GetIsAdmin() {
		return "administrator", nil, true
	}
	if len(item.AllowedGroups) == 0 {
		return "everyone", nil, true
	}
	matched := MatchedAllowedGroups(user, item)
	if len(matched) == 0 {
		return "", nil, false
	}
	return "matched_groups", matched, true
}

func categoryAccessRight(user auth.User, category string) string {
	if user.IsAdmin {
		return "admin.access"
	}

	switch category {
	case "connections":
		if slices.Contains(user.Rights, "connections.edit") {
			return "connections.edit"
		}
		if slices.Contains(user.Rights, "connections.read") {
			return "connections.read"
		}
	case "keyvault":
		if slices.Contains(user.Rights, "keyvault.edit") {
			return "keyvault.edit"
		}
		if slices.Contains(user.Rights, "keyvault.read") {
			return "keyvault.read"
		}
	case "appregistrations":
		if slices.Contains(user.Rights, "appregistrations.edit") {
			return "appregistrations.edit"
		}
		if slices.Contains(user.Rights, "appregistrations.read") {
			return "appregistrations.read"
		}
	case "passwords":
		if slices.Contains(user.Rights, "passwords.edit") {
			return "passwords.edit"
		}
		if slices.Contains(user.Rights, "passwords.read") {
			return "passwords.read"
		}
	}

	return ""
}

// canCreatePassword allows holders of the passwords Create capability to
// create both personal and shared password objects. Shared creations are
// forced to be owned by their creator (see enforceCreatorOwnership).
func canCreatePassword(user auth.User, input CreateResourceInput) bool {
	return CategoryForType(input.Type) == "passwords" &&
		auth.CapabilitiesForUser(user).Categories["passwords"].Create
}

func canUpdateResource(user auth.User, existing Resource) bool {
	// Ownership alone grants full edit on the object — the creator keeps control
	// even without the category edit right (e.g. a connections.create-only user).
	isOwner := existing.OwnerUserID == user.ID
	// Personal resources can only be managed by their owner. Admins are excluded
	// so they cannot flip a personal secret to shared and then reveal it.
	if existing.Personal {
		return isOwner
	}
	if user.IsAdmin {
		return true
	}
	if isOwner {
		return true
	}
	// Non-owners holding the category edit right may update shared objects they
	// can see — but only descriptive metadata (see restrictToSharedMetadata).
	capabilities := auth.CapabilitiesForUser(user).Categories[existing.Category]
	return capabilities.Edit && CanAccess(devViewer(user), existing.Summary())
}

// isSharedMetadataEditor identifies an editor of a shared object who is
// neither its owner nor an admin — allowed to touch descriptive metadata only.
func isSharedMetadataEditor(user auth.User, existing Resource) bool {
	return !user.IsAdmin && !existing.Personal && existing.OwnerUserID != user.ID
}

// restrictToSharedMetadata keeps everything crucial (identity, ownership,
// visibility, secret, targets, connection and policy settings) from the stored
// object, letting a non-owner change only description, notes, folder path and
// environment.
func restrictToSharedMetadata(existing Resource, input UpdateResourceInput) UpdateResourceInput {
	restricted := resourceToUpdateInput(existing)
	restricted.Description = input.Description
	restricted.Notes = input.Notes
	restricted.FolderPath = input.FolderPath
	restricted.Environment = input.Environment
	// Blank secret value means "keep the stored secret" downstream.
	restricted.SecretValue = ""
	return restricted
}

func canCreateConnection(user auth.User, input CreateResourceInput) bool {
	return CategoryForType(input.Type) == "connections" &&
		auth.CapabilitiesForUser(user).Categories["connections"].Create
}

// enforceCreatorOwnership decides who owns a newly created object. Non-admins
// always own what they create — the request body cannot pick someone else.
// Admins may assign a real owner via ownerUserId; when they don't, the admin
// becomes the owner so every user-created object has an accountable owner.
func enforceCreatorOwnership(user auth.User, input CreateResourceInput) CreateResourceInput {
	if user.IsAdmin {
		if strings.TrimSpace(input.OwnerUserID) == "" {
			input.OwnerUserID = user.ID
			if strings.TrimSpace(input.Owner) == "" {
				input.Owner = user.Name
			}
		}
		return input
	}
	input.Owner = user.Name
	input.OwnerUserID = user.ID
	if CategoryForType(input.Type) == "connections" {
		input.Personal = false
	}
	return input
}

func canArchiveResource(user auth.User, resource Resource) bool {
	// Ownership alone is enough — see canUpdateResource.
	isOwner := resource.OwnerUserID == user.ID
	// Personal resources can only be managed by their owner — admins included.
	if resource.Personal {
		return isOwner
	}
	if user.IsAdmin {
		return true
	}
	return isOwner
}

func enforcePersonalPasswordOwnership(user auth.User, input CreateResourceInput) CreateResourceInput {
	if !input.Personal || CategoryForType(input.Type) != "passwords" {
		return input
	}
	input.Owner = user.Name
	input.OwnerUserID = user.ID
	input.OwnerTeam = ""
	input.AllowedGroups = []string{}
	return input
}

func isPasswordOverrideCandidate(resource Resource) bool {
	if CategoryForType(resource.Type) != "passwords" {
		return false
	}
	if resource.SourceKind != SourceKindManual {
		return false
	}
	if strings.TrimSpace(resource.Username) == "" {
		return false
	}
	return resource.Type == TypeSharedSecret || resource.Type == TypeWebPortal
}
