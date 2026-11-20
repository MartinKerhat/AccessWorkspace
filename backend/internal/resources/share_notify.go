package resources

import (
	"context"
	"log"
	"slices"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

// ShareRecipient is one person who gained access to an object in a save, and
// how: named directly in "Shared with" (Direct), or as a member of a group that
// was added (Group). Delivery differs: a direct share is in-app plus email, a
// group share in-app only — a group can be large, and mailing all of it for
// every object is noise.
type ShareRecipient struct {
	User   auth.User
	Direct bool
	Group  string
}

// ShareNotifier delivers share notifications. Implemented by the
// notifications package; an interface here so resources does not import it.
type ShareNotifier interface {
	NotifyShared(ctx context.Context, actor auth.User, resource Resource, recipients []ShareRecipient) error
}

// ShareUserDirectory lists users with rights resolved, so recipients can be
// checked against the same view rule the object list uses.
type ShareUserDirectory interface {
	ListResolvedUsers(ctx context.Context) ([]auth.User, error)
}

// ConfigureShareNotifications turns on "shared with you" notifications for
// saves made by a person. Optional: without it, saves behave as before.
func (s *Service) ConfigureShareNotifications(users ShareUserDirectory, notifier ShareNotifier) {
	s.shareUsers = users
	s.shareNotifier = notifier
}

// notifyNewShares tells the people who just gained access to an object.
// Called only from the user-facing create/update paths — never from Key Vault
// sync or the imports, which apply default groups to many objects at once and
// would flood everyone in those groups. Best-effort: a failure is logged and
// never fails the save that triggered it.
func (s *Service) notifyNewShares(ctx context.Context, actor auth.User, before *Resource, after Resource) {
	if s.shareNotifier == nil || s.shareUsers == nil {
		return
	}
	if len(addedShareTargets(before, after)) == 0 {
		return
	}
	users, err := s.shareUsers.ListResolvedUsers(ctx)
	if err != nil {
		log.Printf("share notification: resource=%s listing users failed: %v", after.ID, err)
		return
	}
	recipients := shareRecipients(actor, before, after, users)
	if len(recipients) == 0 {
		return
	}
	if err := s.shareNotifier.NotifyShared(ctx, actor, after, recipients); err != nil {
		log.Printf("share notification: resource=%s recipients=%d failed: %v", after.ID, len(recipients), err)
	}
}

// addedShareTargets is the cheap pre-check: the users and groups present in
// "Shared with" after the save that were not there before. Empty means nothing
// to notify, without loading the user directory.
func addedShareTargets(before *Resource, after Resource) []string {
	if after.Personal || (len(after.AllowedUsers) == 0 && len(after.AllowedGroups) == 0) {
		// Personal objects are never shared; an empty list means everyone with
		// category access, which is deliberately silent.
		return nil
	}
	previousUsers, previousGroups := previousShareLists(before)
	added := newEntries(after.AllowedUsers, previousUsers)
	return append(added, newEntries(after.AllowedGroups, previousGroups)...)
}

// shareRecipients decides who is told about a save. The rules:
//   - only users and groups ADDED in this save count; re-saving an object that
//     is already shared notifies nobody;
//   - a user named directly gets the direct notification even when also in an
//     added group (one notification, the one with email);
//   - a group member is told only if they could not already open the object
//     before the save (or own it) — for them nothing actually changed;
//   - the actor, blocked users and anyone who cannot open the object are
//     never notified.
func shareRecipients(actor auth.User, before *Resource, after Resource, users []auth.User) []ShareRecipient {
	if len(addedShareTargets(before, after)) == 0 {
		return nil
	}
	previousUsers, previousGroups := previousShareLists(before)
	addedUsers := newEntries(after.AllowedUsers, previousUsers)
	addedGroups := newEntries(after.AllowedGroups, previousGroups)

	afterSummary := after.Summary()
	recipients := make([]ShareRecipient, 0)
	for _, user := range users {
		if strings.TrimSpace(user.ID) == "" || user.ID == actor.ID || user.Blocked {
			continue
		}
		if !canViewResource(user, afterSummary) {
			continue
		}
		if slices.Contains(addedUsers, user.ID) {
			recipients = append(recipients, ShareRecipient{User: user, Direct: true})
			continue
		}
		group := firstMatchingGroup(user.LocalGroups, addedGroups)
		if group == "" {
			continue
		}
		if ownsResource(user, afterSummary) {
			continue
		}
		if before != nil && canViewResource(user, before.Summary()) {
			continue
		}
		recipients = append(recipients, ShareRecipient{User: user, Group: group})
	}
	return recipients
}

// previousShareLists is what "Shared with" held before the save. A create has
// no before; neither does a personal object becoming shared, since a personal
// object's lists mean nothing.
func previousShareLists(before *Resource) ([]string, []string) {
	if before == nil || before.Personal {
		return nil, nil
	}
	return before.AllowedUsers, before.AllowedGroups
}

func newEntries(current []string, previous []string) []string {
	added := make([]string, 0)
	for _, item := range current {
		if strings.TrimSpace(item) == "" || slices.Contains(previous, item) || slices.Contains(added, item) {
			continue
		}
		added = append(added, item)
	}
	return added
}

// firstMatchingGroup names the group the notification credits, in the order
// the groups appear on the object.
func firstMatchingGroup(userGroups []string, addedGroups []string) string {
	for _, group := range addedGroups {
		if slices.Contains(userGroups, group) {
			return group
		}
	}
	return ""
}
