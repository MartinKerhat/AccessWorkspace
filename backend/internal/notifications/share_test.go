package notifications

import (
	"slices"
	"strings"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/resources"
)

func TestShareNotificationDirectGetsEmailWhenAvailable(t *testing.T) {
	resource := resources.Resource{ID: "res-1", Name: "Backup share", Type: resources.TypeSharedSecret, Description: "secret detail", Username: "svc-user"}
	recipient := resources.ShareRecipient{User: auth.User{ID: "martin"}, Direct: true}

	item := shareNotification(auth.User{Name: "Alex Example"}, resource, recipient, true)

	if item.Kind != resources.NotificationKindShare || item.UserID != "martin" || item.ResourceID != "res-1" {
		t.Fatalf("unexpected identity: %+v", item)
	}
	if item.Title != "Alex Example shared Backup share with you" {
		t.Fatalf("unexpected title %q", item.Title)
	}
	if item.Body != "Saved password · shared with you directly." {
		t.Fatalf("unexpected body %q", item.Body)
	}
	if !slices.Contains(item.Channels, resources.NotificationChannelEmail) || !slices.Contains(item.Channels, resources.NotificationChannelInApp) {
		t.Fatalf("a direct share is in-app and email, got %v", item.Channels)
	}
	for _, leaked := range []string{"secret detail", "svc-user"} {
		if strings.Contains(item.Title+item.Body, leaked) {
			t.Fatalf("notification text must not carry object detail %q", leaked)
		}
	}
}

func TestShareNotificationGroupIsInAppOnly(t *testing.T) {
	resource := resources.Resource{ID: "res-1", Name: "Prod DB", Type: resources.TypeKeyVaultSecret}
	recipient := resources.ShareRecipient{User: auth.User{ID: "a"}, Group: "Admins"}

	item := shareNotification(auth.User{Name: "Alex Example"}, resource, recipient, true)

	if !slices.Equal(item.Channels, []resources.NotificationChannel{resources.NotificationChannelInApp}) {
		t.Fatalf("a group share is in-app only, got %v", item.Channels)
	}
	if item.Body != "Key Vault secret · shared with your group Admins." {
		t.Fatalf("unexpected body %q", item.Body)
	}
}

func TestShareNotificationDirectWithoutSMTPIsInAppOnly(t *testing.T) {
	recipient := resources.ShareRecipient{User: auth.User{ID: "martin"}, Direct: true}

	item := shareNotification(auth.User{}, resources.Resource{Name: "x", Type: resources.TypeRDP}, recipient, false)

	if slices.Contains(item.Channels, resources.NotificationChannelEmail) {
		t.Fatalf("without SMTP the email channel would only ever show as failed, got %v", item.Channels)
	}
	if !strings.HasPrefix(item.Title, "Someone shared") {
		t.Fatalf("a nameless actor still reads naturally, got %q", item.Title)
	}
}
