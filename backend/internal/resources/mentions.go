package resources

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"access-workspace/backend/internal/auth"
)

// Object mentions let a note reference a stored credential inline: the author
// types "@", picks an object, and a token is written into the notes text. The
// point is connection notes that say "log in with @[db-admin](passwords:...)"
// instead of repeating the credential, so the note stays correct when the
// credential changes and the reader's own permissions decide what they can do
// with it.
//
// Notes stay a plain text column — the token is the whole storage format, so
// there is no migration and existing prose keeps working as prose.

// mentionTokenPattern matches @[Display name](category:id). Anything that does
// not match is literal text: a stray "@" in prose and a hand-mangled token must
// both survive untouched rather than erroring.
//
// The id is deliberately NOT constrained to a uuid shape — resource ids are
// arbitrary strings here (seeded objects use slugs like "res-web"), so a
// uuid-only pattern silently refused to render half the real data. Excluding
// ")" and whitespace is enough to keep the token unambiguous.
var mentionTokenPattern = regexp.MustCompile(`@\[([^\]]*)\]\((passwords|keyvault):([^)\s]{1,128})\)`)

// MentionState is what a specific viewer may do with a mentioned object.
type MentionState string

const (
	// MentionAccessible — the viewer may open it; name and username are returned.
	MentionAccessible MentionState = "accessible"
	// MentionDenied — the viewer may not open it, but learns it exists and what
	// it is called. Deliberate: a broadly shared connection can reference
	// credentials only a smaller group may read, and saying so is useful. Matches
	// this codebase's stance that shared objects 403 rather than 404.
	MentionDenied MentionState = "denied"
	// MentionHidden — nothing may be disclosed, not even the name. Used for
	// personal objects (which 404 precisely because their existence is sensitive)
	// and for targets that no longer exist. Reachable only through drift: a
	// personal object cannot be picked in the first place.
	MentionHidden MentionState = "hidden"
)

type MentionTarget struct {
	ResourceID string       `json:"resourceId"`
	Category   string       `json:"category"`
	State      MentionState `json:"state"`
	Name       string       `json:"name,omitempty"`
	Username   string       `json:"username,omitempty"`
	Type       ResourceType `json:"type,omitempty"`
}

type MentionCandidate struct {
	ResourceID string       `json:"resourceId"`
	Category   string       `json:"category"`
	Type       ResourceType `json:"type"`
	Name       string       `json:"name"`
	Username   string       `json:"username,omitempty"`
	FolderPath string       `json:"folderPath,omitempty"`
}

// mentionableCategories are the modules whose objects may be mentioned. Only
// these two hold a credential worth pointing at from a note; app registrations
// hold no secret value at all, and connections are not credentials.
var mentionableCategories = map[string]bool{"passwords": true, "keyvault": true}

// ParseMentionIDs returns the resource ids referenced by a notes body, in order
// of appearance and de-duplicated.
func ParseMentionIDs(notes string) []string {
	matches := mentionTokenPattern.FindAllStringSubmatch(notes, -1)
	seen := make(map[string]bool, len(matches))
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		// Not lowercased: ids are opaque and looked up by exact match.
		id := match[3]
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// ListMentionCandidates is the "@" picker's source: the shared Passwords and Key
// Vault objects THIS caller can see, optionally narrowed by a name prefix.
//
// Scoped to the caller on purpose — an admin gets a large list and a non-admin
// colleague editing the same note gets a much smaller one. Personal objects are
// excluded: they are invisible to non-owners, so mentioning one would either
// leak that it exists or render dead for everyone but the owner.
//
// There is deliberately no comparison against the mentioning object's own
// sharing. A broadly shared connection referencing credentials that only a
// smaller group may open is a normal, wanted case; enforcement happens per
// viewer in ResolveMentions instead.
func (s *Service) ListMentionCandidates(ctx context.Context, user auth.User, query string) ([]MentionCandidate, error) {
	items, err := s.repo.List(ctx, Filter{})
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	visible := explainVisibleResourcesForUser(user, items)
	candidates := make([]MentionCandidate, 0, len(visible))
	for _, item := range visible {
		summary := item.ResourceSummary
		if summary.Personal || !mentionableCategories[summary.Category] {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(summary.Name+" "+summary.Username), needle) {
			continue
		}
		candidates = append(candidates, MentionCandidate{
			ResourceID: summary.ID,
			Category:   summary.Category,
			Type:       summary.Type,
			Name:       summary.Name,
			Username:   summary.Username,
			FolderPath: summary.FolderPath,
		})
	}
	sort.Slice(candidates, func(a, b int) bool {
		return strings.ToLower(candidates[a].Name) < strings.ToLower(candidates[b].Name)
	})
	return candidates, nil
}

// ResolveMentions is the single enforcement point, evaluated per viewer on every
// read. A verdict cached at write time would drift: sharing changes after a note
// is written, and module view capability comes from the viewer's role rather
// than from the object, so no authoring-time check could ever have been a
// guarantee.
func (s *Service) ResolveMentions(ctx context.Context, user auth.User, ids []string) ([]MentionTarget, error) {
	targets := make([]MentionTarget, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targets = append(targets, s.resolveMention(ctx, user, id))
	}
	return targets, nil
}

func (s *Service) resolveMention(ctx context.Context, user auth.User, id string) MentionTarget {
	hidden := MentionTarget{ResourceID: id, State: MentionHidden}

	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return hidden
	}
	// Personal and archived targets disclose nothing at all — not even the name.
	// Neither can be picked; both are reachable only by an object changing after
	// it was mentioned.
	if resource.Personal || resource.ArchivedAt != nil {
		return hidden
	}
	if !mentionableCategories[resource.Category] {
		return hidden
	}

	target := MentionTarget{
		ResourceID: resource.ID,
		Category:   resource.Category,
		Type:       resource.Type,
		Name:       resource.Name,
		Username:   resource.Username,
		State:      MentionDenied,
	}
	if canViewResource(user, resource.Summary()) {
		target.State = MentionAccessible
	} else {
		// Denied carries the name — that is the deliberate disclosure — and
		// nothing else. The username in particular stays hidden: it is closer to
		// the credential than a label is.
		target.Username = ""
	}
	return target
}
