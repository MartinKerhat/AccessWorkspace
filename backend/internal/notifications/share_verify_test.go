package notifications

// End-to-end verification of share notifications against a throwaway
// database: a real object update through resources.Service, real users and
// local groups resolved by auth.Service, rows written by this package and the
// direct-share email handed to an SMTP catcher. Runs only when
// VERIFY_DATABASE_URL is set; the email part needs VERIFY_SMTP_ADDR
// (host:port of a catcher such as Mailpit) as well.

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/db"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

type verifySharePolicies struct {
	stubPolicyStore
	smtpHost string
	smtpPort int
}

func (p verifySharePolicies) GetNotificationEmailRuntime(context.Context) (NotificationEmailRuntimeConfig, error) {
	if p.smtpHost == "" {
		return NotificationEmailRuntimeConfig{}, nil
	}
	return NotificationEmailRuntimeConfig{
		Enabled:    true,
		Configured: true,
		Host:       p.smtpHost,
		Port:       p.smtpPort,
		From:       "workspace@example.test",
	}, nil
}

func TestShareNotificationsEndToEnd(t *testing.T) {
	dsn := os.Getenv("VERIFY_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFY_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	policies := verifySharePolicies{}
	if addr := os.Getenv("VERIFY_SMTP_ADDR"); addr != "" {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("VERIFY_SMTP_ADDR: %v", err)
		}
		policies.smtpHost = host
		policies.smtpPort, _ = strconv.Atoi(port)
	}

	authRepo := auth.NewRepository(pool)
	authService := auth.NewService(authRepo, auth.Mode("local"), "")
	for _, group := range []auth.LocalGroupInput{
		{Name: "VerifyShareEditors", Rights: []string{"passwords.read", "passwords.edit", "passwords.create"}},
		{Name: "VerifyShareReaders", Rights: []string{"passwords.read"}},
		{Name: "VerifyShareTeam"},
	} {
		if err := authRepo.CreateLocalGroup(ctx, group); err != nil {
			t.Fatalf("create group %s: %v", group.Name, err)
		}
	}
	defer pool.Exec(ctx, `delete from local_groups where name like 'VerifyShare%'`)

	newUser := func(username string, groups ...string) auth.User {
		id, err := authRepo.CreateUser(ctx, auth.CreateUserInput{
			Username:          username,
			DisplayName:       "Verify " + username,
			Email:             username + "@example.test",
			Password:          "verify-password-123",
			DirectLocalGroups: groups,
		})
		if err != nil {
			t.Fatalf("create user %s: %v", username, err)
		}
		return auth.User{ID: id, Name: "Verify " + username, Email: username + "@example.test", LocalGroups: groups}
	}
	actor := newUser("vshare-actor", "VerifyShareEditors")
	actor.Rights = []string{"passwords.read", "passwords.edit", "passwords.create"}
	actor.Name = "Verify Kerhát" // diacritics must survive the email subject and body
	direct := newUser("vshare-direct", "VerifyShareReaders")
	member := newUser("vshare-member", "VerifyShareReaders", "VerifyShareTeam")
	outsider := newUser("vshare-outsider") // no category right: must never hear of it
	defer pool.Exec(ctx, `delete from app_users where username like 'vshare-%'`)

	resourceRepo := resources.NewRepository(pool)
	notificationService := NewService(NewRepository(pool), resourceRepo, authService, policies)
	notificationService.ConfigureResourceLinks("https://workspace.example.test")
	resourceService := resources.NewService(resourceRepo, audit.NewService(audit.NewRepository(pool)), nil, nil, notificationService)
	resourceService.ConfigureShareNotifications(authService, notificationService)

	// Created through the service, the path the object form uses: shared with
	// one user directly, one by group, and one who has no access at all.
	created, err := resourceService.Create(ctx, actor, resources.CreateResourceInput{
		Name:          "Verify share portal",
		Type:          resources.TypeWebPortal,
		Status:        "active",
		TargetURL:     "https://portal.example.test",
		SourceKind:    resources.SourceKindManual,
		SecretMode:    resources.SecretModeNone,
		AllowedUsers:  []string{direct.ID, outsider.ID},
		AllowedGroups: []string{"VerifyShareTeam"},
	})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	defer resourceRepo.Delete(ctx, created.ID)

	share := func(users []string, groups []string) {
		existing, err := resourceRepo.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		input := resources.UpdateResourceInput{
			Name:          existing.Name,
			Type:          existing.Type,
			Owner:         existing.Owner,
			OwnerUserID:   existing.OwnerUserID,
			Status:        existing.Status,
			TargetURL:     existing.TargetURL,
			SourceKind:    existing.SourceKind,
			SecretMode:    resources.SecretModeNone,
			AllowedUsers:  users,
			AllowedGroups: groups,
		}
		if _, err := resourceService.Update(ctx, actor, created.ID, input); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	shareRows := func(user auth.User) []resources.UserNotification {
		items, err := notificationService.ListForUser(ctx, user.ID, 50)
		if err != nil {
			t.Fatalf("list for %s: %v", user.ID, err)
		}
		out := []resources.UserNotification{}
		for _, item := range items {
			if item.Kind == resources.NotificationKindShare && item.ResourceID == created.ID {
				out = append(out, item)
			}
		}
		return out
	}

	directRows := shareRows(direct)
	if len(directRows) != 1 {
		t.Fatalf("direct recipient: expected 1 share notification, got %d", len(directRows))
	}
	if directRows[0].Title != "Verify Kerhát shared Verify share portal with you" {
		t.Fatalf("unexpected title %q", directRows[0].Title)
	}
	if policies.smtpHost != "" {
		if directRows[0].EmailStatus != "sent" {
			t.Fatalf("direct recipient email: expected sent, got %q (%s)", directRows[0].EmailStatus, directRows[0].EmailError)
		}
	}
	memberRows := shareRows(member)
	if len(memberRows) != 1 || len(memberRows[0].Channels) != 1 || memberRows[0].Channels[0] != resources.NotificationChannelInApp {
		t.Fatalf("group member: expected 1 in-app-only notification, got %+v", memberRows)
	}
	if rows := shareRows(outsider); len(rows) != 0 {
		t.Fatalf("a user without access must not be notified, got %+v", rows)
	}
	if rows := shareRows(actor); len(rows) != 0 {
		t.Fatalf("the person sharing must not be notified, got %+v", rows)
	}

	// The expiry digest must never pick share rows up.
	pending, err := notificationService.repo.listPendingEmailReminders(ctx)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	for _, item := range pending {
		if item.Kind == resources.NotificationKindShare {
			t.Fatalf("share row leaked into the expiry digest: %+v", item)
		}
	}

	// Saving again with the same lists adds nobody and notifies nobody.
	share([]string{direct.ID, outsider.ID}, []string{"VerifyShareTeam"})
	if got := len(shareRows(direct)) + len(shareRows(member)); got != 2 {
		t.Fatalf("an unchanged re-save must not notify again, got %d rows", got)
	}

	// Removing the direct user and adding them back is new access again.
	share([]string{outsider.ID}, []string{"VerifyShareTeam"})
	share([]string{direct.ID, outsider.ID}, []string{"VerifyShareTeam"})
	if got := len(shareRows(direct)); got != 2 {
		t.Fatalf("re-adding a removed user should notify them again, got %d rows", got)
	}
}
