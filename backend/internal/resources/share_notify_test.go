package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

func shareTestUser(id string, groups ...string) auth.User {
	return auth.User{
		ID:          id,
		Name:        "User " + id,
		Email:       id + "@example.test",
		LocalGroups: groups,
		Rights:      []string{"passwords.read"},
	}
}

func sharedPassword(users []string, groups []string) Resource {
	return Resource{
		ID:            "res-1",
		Name:          "Backup share",
		Type:          TypeSharedSecret,
		Category:      "passwords",
		OwnerUserID:   "actor",
		AllowedUsers:  users,
		AllowedGroups: groups,
	}
}

func recipientsByID(recipients []ShareRecipient) map[string]ShareRecipient {
	out := map[string]ShareRecipient{}
	for _, recipient := range recipients {
		out[recipient.User.ID] = recipient
	}
	return out
}

func TestShareRecipientsDirectUserOnCreate(t *testing.T) {
	actor := shareTestUser("actor")
	users := []auth.User{actor, shareTestUser("martin"), shareTestUser("other")}

	got := recipientsByID(shareRecipients(actor, nil, sharedPassword([]string{"martin"}, nil), users))

	if len(got) != 1 || !got["martin"].Direct {
		t.Fatalf("expected only martin, directly; got %+v", got)
	}
}

func TestShareRecipientsGroupMembersInAppAndDirectWins(t *testing.T) {
	actor := shareTestUser("actor", "Admins")
	users := []auth.User{actor, shareTestUser("a", "Admins"), shareTestUser("b", "Admins"), shareTestUser("c", "Support")}

	got := recipientsByID(shareRecipients(actor, nil, sharedPassword([]string{"b"}, []string{"Admins"}), users))

	if len(got) != 2 {
		t.Fatalf("expected a and b, got %+v", got)
	}
	if got["a"].Direct || got["a"].Group != "Admins" {
		t.Fatalf("a should be notified through the group, got %+v", got["a"])
	}
	if !got["b"].Direct {
		t.Fatalf("b is named directly and in the group: the direct notification wins, got %+v", got["b"])
	}
	if _, ok := got["actor"]; ok {
		t.Fatalf("the person sharing is never notified")
	}
}

func TestShareRecipientsEveryoneAndPersonalAreSilent(t *testing.T) {
	actor := shareTestUser("actor")
	users := []auth.User{actor, shareTestUser("martin")}

	if got := shareRecipients(actor, nil, sharedPassword(nil, nil), users); len(got) != 0 {
		t.Fatalf("shared with everyone must not notify, got %+v", got)
	}
	personal := sharedPassword([]string{"martin"}, nil)
	personal.Personal = true
	if got := shareRecipients(actor, nil, personal, users); len(got) != 0 {
		t.Fatalf("personal objects must not notify, got %+v", got)
	}
}

func TestShareRecipientsUpdateOnlyNotifiesAdditions(t *testing.T) {
	actor := shareTestUser("actor")
	users := []auth.User{actor, shareTestUser("martin"), shareTestUser("new"), shareTestUser("old-group-member", "Ops", "Admins"), shareTestUser("fresh", "Admins")}
	before := sharedPassword([]string{"martin"}, []string{"Ops"})

	unchanged := sharedPassword([]string{"martin"}, []string{"Ops"})
	unchanged.Description = "edited"
	if got := shareRecipients(actor, &before, unchanged, users); len(got) != 0 {
		t.Fatalf("a save that adds nobody must notify nobody, got %+v", got)
	}

	after := sharedPassword([]string{"martin", "new"}, []string{"Ops", "Admins"})
	got := recipientsByID(shareRecipients(actor, &before, after, users))
	if len(got) != 2 || !got["new"].Direct || got["fresh"].Group != "Admins" {
		t.Fatalf("expected new (direct) and fresh (Admins), got %+v", got)
	}
	if _, ok := got["old-group-member"]; ok {
		t.Fatalf("a group member who could already open the object gains nothing and is not notified")
	}
}

func TestShareRecipientsRestrictingEveryoneToAGroupIsSilent(t *testing.T) {
	actor := shareTestUser("actor")
	users := []auth.User{actor, shareTestUser("a", "Admins")}
	before := sharedPassword(nil, nil)

	if got := shareRecipients(actor, &before, sharedPassword(nil, []string{"Admins"}), users); len(got) != 0 {
		t.Fatalf("narrowing everyone to a group gives nobody new access, got %+v", got)
	}
}

func TestShareRecipientsPersonalBecomingSharedNotifies(t *testing.T) {
	actor := shareTestUser("actor")
	users := []auth.User{actor, shareTestUser("martin")}
	before := sharedPassword([]string{"martin"}, nil)
	before.Personal = true

	got := recipientsByID(shareRecipients(actor, &before, sharedPassword([]string{"martin"}, nil), users))
	if !got["martin"].Direct {
		t.Fatalf("a personal object turned shared is new access, got %+v", got)
	}
}

func TestShareRecipientsSkipsUsersWhoCannotOpenIt(t *testing.T) {
	actor := shareTestUser("actor")
	noRight := shareTestUser("no-right")
	noRight.Rights = nil
	blocked := shareTestUser("blocked")
	blocked.Blocked = true
	users := []auth.User{actor, noRight, blocked}

	if got := shareRecipients(actor, nil, sharedPassword([]string{"no-right", "blocked"}, nil), users); len(got) != 0 {
		t.Fatalf("users without category access or blocked must not be told the object exists, got %+v", got)
	}
}

type fakeShareDirectory struct {
	users []auth.User
	calls int
}

func (d *fakeShareDirectory) ListResolvedUsers(context.Context) ([]auth.User, error) {
	d.calls++
	return d.users, nil
}

type fakeShareNotifier struct {
	recipients []ShareRecipient
	err        error
}

func (n *fakeShareNotifier) NotifyShared(_ context.Context, _ auth.User, _ Resource, recipients []ShareRecipient) error {
	n.recipients = append(n.recipients, recipients...)
	return n.err
}

func TestNotifyNewSharesSkipsDirectoryWhenNothingAdded(t *testing.T) {
	service := NewService(&archiveTestStore{}, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	directory := &fakeShareDirectory{users: []auth.User{shareTestUser("martin")}}
	notifier := &fakeShareNotifier{}
	service.ConfigureShareNotifications(directory, notifier)

	before := sharedPassword([]string{"martin"}, nil)
	service.notifyNewShares(context.Background(), shareTestUser("actor"), &before, sharedPassword([]string{"martin"}, nil))

	if directory.calls != 0 || len(notifier.recipients) != 0 {
		t.Fatalf("an unchanged share list should not even load users: calls=%d recipients=%d", directory.calls, len(notifier.recipients))
	}
}

func TestNotifyNewSharesDeliversAndSwallowsErrors(t *testing.T) {
	service := NewService(&archiveTestStore{}, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	directory := &fakeShareDirectory{users: []auth.User{shareTestUser("actor"), shareTestUser("martin")}}
	notifier := &fakeShareNotifier{err: errors.New("smtp down")}
	service.ConfigureShareNotifications(directory, notifier)

	// Must not panic or surface the error: the save already succeeded.
	service.notifyNewShares(context.Background(), shareTestUser("actor"), nil, sharedPassword([]string{"martin"}, nil))

	if len(notifier.recipients) != 1 || notifier.recipients[0].User.ID != "martin" {
		t.Fatalf("expected martin to be handed to the notifier, got %+v", notifier.recipients)
	}
}
