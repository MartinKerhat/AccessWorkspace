package notifications

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/resources"
	"github.com/google/uuid"
)

// NotifyShared records a "shared with you" notification for each recipient and
// emails the ones named directly. Who qualifies is decided by the resources
// package (it owns the access rules); this only delivers.
//
// Email goes out at once, one message per recipient, rather than through the
// expiry digest: a share is an event someone is waiting on ("I shared it with
// you, have a look"), not a date to batch. Email is offered only when SMTP is
// switched on and configured — otherwise every share would show "email failed"
// in the recipient's notification centre on installs that never set up mail.
func (s *Service) NotifyShared(ctx context.Context, actor auth.User, resource resources.Resource, recipients []resources.ShareRecipient) error {
	enabled, err := s.policies.GetShareNotificationsEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled || len(recipients) == 0 {
		return nil
	}
	emailRuntime, err := s.policies.GetNotificationEmailRuntime(ctx)
	if err != nil {
		return err
	}
	emailAvailable := emailRuntime.Enabled && emailRuntime.Configured

	for _, recipient := range recipients {
		notification := shareNotification(actor, resource, recipient, emailAvailable)
		if err := s.repo.insertShareNotification(ctx, notification); err != nil {
			return err
		}
		if !recipient.Direct || !emailAvailable {
			continue
		}
		s.sendShareEmail(ctx, notification, recipient.User.Email)
	}
	return nil
}

func (s *Service) sendShareEmail(ctx context.Context, notification resources.UserNotification, email string) {
	ids := []string{notification.ID}
	if strings.TrimSpace(email) == "" {
		_ = s.repo.markEmailStatus(ctx, ids, "failed", nil, "recipient has no email address")
		return
	}
	body := notification.Body
	if link := s.resourceLink(notification.ResourceID); link != "" {
		body += "\n\n" + link
	}
	if err := s.SendPlainEmail(ctx, email, notification.Title, body); err != nil {
		log.Printf("share notification email failed: recipient=%s resource=%s error=%v", notification.UserID, notification.ResourceID, err)
		_ = s.repo.markEmailStatus(ctx, ids, "failed", nil, err.Error())
		return
	}
	sentAt := time.Now().UTC()
	_ = s.repo.markEmailStatus(ctx, ids, "sent", &sentAt, "")
}

// shareNotification is the row and the email in one: the email subject is the
// title, the body the body. Deliberately only the object's name and type —
// never the username, password, description or notes.
func shareNotification(actor auth.User, resource resources.Resource, recipient resources.ShareRecipient, emailAvailable bool) resources.UserNotification {
	actorName := strings.TrimSpace(actor.Name)
	if actorName == "" {
		actorName = "Someone"
	}
	channels := []resources.NotificationChannel{resources.NotificationChannelInApp}
	if recipient.Direct && emailAvailable {
		channels = append(channels, resources.NotificationChannelEmail)
	}
	how := "shared with you directly"
	if !recipient.Direct {
		how = fmt.Sprintf("shared with your group %s", recipient.Group)
	}
	return resources.UserNotification{
		ID:           uuid.NewString(),
		Kind:         resources.NotificationKindShare,
		UserID:       recipient.User.ID,
		ResourceID:   resource.ID,
		ResourceName: resource.Name,
		Title:        fmt.Sprintf("%s shared %s with you", actorName, resource.Name),
		Body:         fmt.Sprintf("%s · %s.", resourceTypeLabel(resource.Type), how),
		Channels:     channels,
	}
}

func resourceTypeLabel(resourceType resources.ResourceType) string {
	switch resourceType {
	case resources.TypeSharedSecret:
		return "Saved password"
	case resources.TypeWebPortal:
		return "Web portal login"
	case resources.TypeSSH:
		return "SSH connection"
	case resources.TypeRDP:
		return "RDP connection"
	case resources.TypeKeyVaultSecret:
		return "Key Vault secret"
	case resources.TypeAppRegistration:
		return "App registration"
	default:
		return "Object"
	}
}

// insertShareNotification writes one share row. Unlike expiry reminders there
// is no dedupe lookup: every save that adds someone is its own event, and the
// caller only passes people who were added in that save.
func (r *Repository) insertShareNotification(ctx context.Context, notification resources.UserNotification) error {
	_, err := r.db.Exec(ctx, `
		insert into expiry_notifications (
			id, kind, user_id, resource_id, resource_name, credential_key_id, credential_display_name,
			credential_type, credential_end_date_time, reminder_day, title, body, channels,
			email_status, email_error
		) values (
			$1, $2, $3, $4, $5, '', '',
			'', null, 0, $6, $7, $8,
			'', ''
		)
	`, notification.ID, resources.NotificationKindShare, notification.UserID, notification.ResourceID, notification.ResourceName,
		notification.Title, notification.Body, notificationChannelsToStrings(notification.Channels))
	return err
}
