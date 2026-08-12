package notifications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"slices"
	"strings"
	"time"

	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResourceStore interface {
	Get(ctx context.Context, id string) (resources.Resource, error)
}

type UserDirectory interface {
	ListUsers(ctx context.Context) ([]auth.UserSummary, error)
}

type PolicyStore interface {
	GetAppRegistrationNotificationPolicy(ctx context.Context) (resources.ExpiryNotificationPolicy, error)
	GetKeyVaultNotificationPolicy(ctx context.Context) (resources.ExpiryNotificationPolicy, error)
	GetNotificationEmailRuntime(ctx context.Context) (NotificationEmailRuntimeConfig, error)
}

// expiringItem is the one shape both notifiable sources reduce to: an app
// registration contributes one per synced credential, a Key Vault secret
// contributes exactly one (its own expiry). Everything downstream — reminder
// days, dedupe, superseding, the email body — works on this and does not care
// which category it came from.
type expiringItem struct {
	keyID       string
	displayName string
	kind        string
	expiresAt   *time.Time
	policy      resources.ExpiryNotificationPolicy
}

type NotificationEmailRuntimeConfig struct {
	Enabled    bool
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	Configured bool
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type Service struct {
	repo      *Repository
	resources ResourceStore
	users     UserDirectory
	policies  PolicyStore
}

func NewService(repo *Repository, resources ResourceStore, users UserDirectory, policies PolicyStore) *Service {
	return &Service{repo: repo, resources: resources, users: users, policies: policies}
}

func (s *Service) EvaluateResource(ctx context.Context, resourceID string) error {
	resource, err := s.resources.Get(ctx, resourceID)
	if err != nil {
		return err
	}

	items, err := s.expiringItems(ctx, resource)
	if err != nil {
		return err
	}
	if items == nil {
		return nil
	}

	// Superseding runs before anything else and regardless of whether a
	// reminder is due: a rotated credential (or a new Key Vault secret version)
	// moves the expiry date, and the reminders written for the OLD date would
	// otherwise sit in the notification centre forever claiming an expiry that
	// no longer exists — loudest exactly when the new credential has no expiry
	// at all and nothing new is ever written to replace them.
	if err := s.repo.supersedeStaleReminders(ctx, resource.ID, items); err != nil {
		return err
	}

	recipients, err := s.resolveRecipients(ctx, resource.Owner, resource.OwnerTeam)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		log.Printf("expiry notification: resource=%s owner=%q team=%q no recipients resolved", resource.ID, resource.Owner, resource.OwnerTeam)
		return nil
	}

	now := time.Now().UTC()
	for _, item := range items {
		if item.expiresAt == nil {
			continue
		}
		policy := item.policy
		if !policy.Enabled || len(policy.ReminderDays) == 0 || len(policy.Channels) == 0 {
			continue
		}
		daysRemaining := calendarDayDistanceUTC(now, *item.expiresAt)
		if !slices.Contains(policy.ReminderDays, daysRemaining) {
			continue
		}
		for _, recipient := range recipients {
			if strings.TrimSpace(recipient.ID) == "" {
				continue
			}
			notification, created, err := s.repo.ensureReminder(ctx, reminderNotification(resource, item, recipient, daysRemaining))
			if err != nil {
				return err
			}
			if !created && notification.EmailStatus == "sent" {
				continue
			}
			if slices.Contains(policy.Channels, resources.NotificationChannelEmail) {
				if err := s.sendEmail(ctx, recipient, notification); err != nil {
					log.Printf("expiry notification email failed: resource=%s recipient=%s email=%s reminder_day=%d error=%v", resource.ID, recipient.ID, recipient.Email, daysRemaining, err)
					_ = s.repo.updateEmailStatus(ctx, notification.ID, "failed", nil, err.Error())
					continue
				}
				sentAt := time.Now().UTC()
				log.Printf("expiry notification email sent: resource=%s recipient=%s email=%s reminder_day=%d", resource.ID, recipient.ID, recipient.Email, daysRemaining)
				_ = s.repo.updateEmailStatus(ctx, notification.ID, "sent", &sentAt, "")
			}
		}
	}
	return nil
}

// expiringItems reduces a resource to the things that can expire on it, with
// the policy that governs each already resolved. A nil slice means "this type
// does not participate in expiry reminders at all" and stops evaluation; an
// empty (non-nil) slice means "participates, but has nothing to expire right
// now" and still runs the supersede pass.
func (s *Service) expiringItems(ctx context.Context, resource resources.Resource) ([]expiringItem, error) {
	switch resource.Type {
	case resources.TypeAppRegistration:
		globalPolicy, err := s.policies.GetAppRegistrationNotificationPolicy(ctx)
		if err != nil {
			return nil, err
		}
		resourcePolicy := globalPolicy
		if resource.AppNotificationPolicyOverride != nil {
			resourcePolicy = *resource.AppNotificationPolicyOverride
		}
		items := make([]expiringItem, 0, len(resource.AppCredentials))
		for _, credential := range resource.AppCredentials {
			policy := resourcePolicy
			if credential.NotificationPolicyOverride != nil {
				policy = *credential.NotificationPolicyOverride
			}
			displayName := strings.TrimSpace(credential.DisplayName)
			if displayName == "" {
				displayName = credential.KeyID
			}
			items = append(items, expiringItem{
				keyID:       credential.KeyID,
				displayName: displayName,
				kind:        credential.CredentialType,
				expiresAt:   credential.EndDateTime,
				policy:      policy,
			})
		}
		return items, nil

	case resources.TypeKeyVaultSecret:
		// Only the version the workspace record points at matters: a
		// versionless reference tracks the current version, a pinned reference
		// tracks that one, and Key Vault refuses to serve an expired version —
		// so a superseded version's expiry is not actionable and is
		// deliberately not modelled. Per-secret overrides are not read here
		// because they are not hydrated for this type; the global Key Vault
		// policy is the whole story today.
		policy, err := s.policies.GetKeyVaultNotificationPolicy(ctx)
		if err != nil {
			return nil, err
		}
		keyID := strings.TrimSpace(resource.ObjectName)
		if keyID == "" {
			keyID = resource.ID
		}
		return []expiringItem{{
			keyID:       keyID,
			displayName: keyID,
			kind:        "secret",
			expiresAt:   resource.ExpiresAt,
			policy:      policy,
		}}, nil

	default:
		return nil, nil
	}
}

func (s *Service) ListForUser(ctx context.Context, userID string, limit int) ([]resources.UserNotification, error) {
	return s.repo.listForUser(ctx, userID, limit)
}

func (s *Service) ListRecentEmailDeliveries(ctx context.Context, limit int) ([]resources.NotificationDeliveryRecord, error) {
	return s.repo.listRecentEmailDeliveries(ctx, limit)
}

func (s *Service) MarkRead(ctx context.Context, userID string, notificationID string) error {
	return s.repo.markRead(ctx, userID, notificationID)
}

func (s *Service) resolveRecipients(ctx context.Context, owner string, ownerTeam string) ([]auth.UserSummary, error) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	owner = strings.TrimSpace(owner)
	ownerTeam = strings.TrimSpace(ownerTeam)

	seen := map[string]struct{}{}
	recipients := make([]auth.UserSummary, 0, len(users))
	for _, user := range users {
		// Blocked users keep their rows but have no workspace access, so
		// mailing them credential expiry detail would be a small leak to an
		// account that was deliberately shut out.
		if user.Blocked {
			continue
		}
		// Admins are always in scope: they are the fallback owner of anything
		// nobody claimed, and without them a record with an unset or
		// unmatchable owner resolves to nobody and expires in silence.
		if user.IsAdmin || matchesOwner(user, owner) || matchesTeam(user, ownerTeam) {
			if _, ok := seen[user.ID]; ok {
				continue
			}
			seen[user.ID] = struct{}{}
			recipients = append(recipients, user)
		}
	}
	return recipients, nil
}

func matchesOwner(user auth.UserSummary, owner string) bool {
	if owner == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(user.ID), owner) ||
		strings.EqualFold(strings.TrimSpace(user.Name), owner) ||
		strings.EqualFold(strings.TrimSpace(user.Email), owner)
}

func matchesTeam(user auth.UserSummary, ownerTeam string) bool {
	if ownerTeam == "" {
		return false
	}
	for _, group := range user.LocalGroups {
		if strings.EqualFold(strings.TrimSpace(group), ownerTeam) {
			return true
		}
	}
	return false
}

func reminderNotification(resource resources.Resource, item expiringItem, recipient auth.UserSummary, reminderDay int) resources.UserNotification {
	title := fmt.Sprintf("%s credential expires", resource.Name)
	if reminderDay > 0 {
		title = fmt.Sprintf("%s expires in %d days", resource.Name, reminderDay)
	}
	// A Key Vault secret's item name is the resource name, so naming both would
	// read "secret db-password for db-password"; app registrations carry a
	// distinct credential name that has to stay in the text.
	subject := fmt.Sprintf("%s %s for %s", item.kind, item.displayName, resource.Name)
	if strings.EqualFold(strings.TrimSpace(item.displayName), strings.TrimSpace(resource.Name)) {
		subject = fmt.Sprintf("%s %s", item.kind, resource.Name)
	}
	body := fmt.Sprintf("%s expires on %s.", subject, item.expiresAt.Local().Format("02.01.2006 15:04:05"))
	if reminderDay == 0 {
		body = fmt.Sprintf("%s expires today at %s.", subject, item.expiresAt.Local().Format("15:04:05"))
	}
	return resources.UserNotification{
		ID:                    uuid.NewString(),
		UserID:                recipient.ID,
		ResourceID:            resource.ID,
		ResourceName:          resource.Name,
		CredentialKeyID:       item.keyID,
		CredentialDisplayName: item.displayName,
		CredentialType:        item.kind,
		CredentialEndDateTime: item.expiresAt,
		ReminderDay:           reminderDay,
		Title:                 title,
		Body:                  body,
		Channels:              append([]resources.NotificationChannel{}, item.policy.Channels...),
	}
}

func calendarDayDistanceUTC(now time.Time, expiry time.Time) int {
	nowDay := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	expiryDay := time.Date(expiry.UTC().Year(), expiry.UTC().Month(), expiry.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(expiryDay.Sub(nowDay).Hours() / 24)
}

func (s *Service) sendEmail(ctx context.Context, recipient auth.UserSummary, notification resources.UserNotification) error {
	return s.SendPlainEmail(ctx, recipient.Email, notification.Title, notification.Body)
}

// SendPlainEmail delivers a plain-text email through the configured SMTP
// runtime. Errors when email delivery is not enabled/configured — callers
// decide whether that is fatal (reminders) or best-effort (invite links).
func (s *Service) SendPlainEmail(ctx context.Context, toEmail, subject, body string) error {
	config, err := s.policies.GetNotificationEmailRuntime(ctx)
	if err != nil {
		return err
	}
	if !config.Enabled || !config.Configured || strings.TrimSpace(toEmail) == "" {
		return errors.New("notification email delivery is not configured")
	}
	var message bytes.Buffer
	message.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	message.WriteString(fmt.Sprintf("From: %s\r\n", config.From))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("\r\n")
	message.WriteString(body)
	message.WriteString("\r\n")

	address := fmt.Sprintf("%s:%d", config.Host, config.Port)
	var authMethod smtp.Auth
	if strings.TrimSpace(config.Username) != "" || strings.TrimSpace(config.Password) != "" {
		authMethod = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}
	return smtp.SendMail(address, authMethod, config.From, []string{toEmail}, message.Bytes())
}

// supersedeStaleReminders marks read every unread reminder on this resource
// that no longer describes reality: the credential was rotated to a new expiry
// date, lost its expiry, or was deleted outright. It compares against the full
// current item set rather than a single row so that a deleted credential — one
// that no longer appears in items at all — is cleaned up too. Marking read (not
// deleting) keeps the row joined to its email delivery record.
func (r *Repository) supersedeStaleReminders(ctx context.Context, resourceID string, items []expiringItem) error {
	keyIDs := make([]string, 0, len(items))
	endDates := make([]*time.Time, 0, len(items))
	for _, item := range items {
		keyIDs = append(keyIDs, item.keyID)
		endDates = append(endDates, item.expiresAt)
	}

	_, err := r.db.Exec(ctx, `
		update expiry_notifications n
		set read_at = now(), updated_at = now()
		where n.resource_id = $1
		  and n.read_at is null
		  and not exists (
		      select 1
		      from unnest($2::text[], $3::timestamptz[]) as current(key_id, end_at)
		      where current.key_id = n.credential_key_id
		        and current.end_at is not distinct from n.credential_end_date_time
		  )
	`, resourceID, keyIDs, endDates)
	return err
}

func (r *Repository) ensureReminder(ctx context.Context, notification resources.UserNotification) (resources.UserNotification, bool, error) {
	row := r.db.QueryRow(ctx, `
		select id, user_id, resource_id, resource_name, credential_key_id, credential_display_name,
		       credential_type, credential_end_date_time, reminder_day, title, body, channels,
		       read_at, email_status, email_sent_at, email_error, created_at
		from expiry_notifications
		where user_id = $1
		  and resource_id = $2
		  and credential_key_id = $3
		  and credential_end_date_time is not distinct from $4
		  and reminder_day = $5
	`, notification.UserID, notification.ResourceID, notification.CredentialKeyID, notification.CredentialEndDateTime, notification.ReminderDay)

	existing, err := scanNotification(row)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return resources.UserNotification{}, false, err
	}

	inserted := notification
	if inserted.ID == "" {
		inserted.ID = uuid.NewString()
	}
	_, err = r.db.Exec(ctx, `
		insert into expiry_notifications (
			id, user_id, resource_id, resource_name, credential_key_id, credential_display_name,
			credential_type, credential_end_date_time, reminder_day, title, body, channels,
			email_status, email_error
		) values (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14
		)
	`, inserted.ID, inserted.UserID, inserted.ResourceID, inserted.ResourceName, inserted.CredentialKeyID, inserted.CredentialDisplayName,
		inserted.CredentialType, inserted.CredentialEndDateTime, inserted.ReminderDay, inserted.Title, inserted.Body, notificationChannelsToStrings(inserted.Channels),
		"", "")
	if err != nil {
		return resources.UserNotification{}, false, err
	}
	item, err := r.getByID(ctx, inserted.ID)
	return item, true, err
}

func (r *Repository) getByID(ctx context.Context, id string) (resources.UserNotification, error) {
	row := r.db.QueryRow(ctx, `
		select id, user_id, resource_id, resource_name, credential_key_id, credential_display_name,
		       credential_type, credential_end_date_time, reminder_day, title, body, channels,
		       read_at, email_status, email_sent_at, email_error, created_at
		from expiry_notifications
		where id = $1
	`, id)
	return scanNotification(row)
}

func (r *Repository) listForUser(ctx context.Context, userID string, limit int) ([]resources.UserNotification, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		select id, user_id, resource_id, resource_name, credential_key_id, credential_display_name,
		       credential_type, credential_end_date_time, reminder_day, title, body, channels,
		       read_at, email_status, email_sent_at, email_error, created_at
		from expiry_notifications
		where user_id = $1
		order by read_at asc nulls first, created_at desc
		limit $2
	`, strings.TrimSpace(userID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []resources.UserNotification{}
	for rows.Next() {
		item, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) markRead(ctx context.Context, userID string, notificationID string) error {
	_, err := r.db.Exec(ctx, `
		update expiry_notifications
		set read_at = coalesce(read_at, now()), updated_at = now()
		where id = $1 and user_id = $2
	`, strings.TrimSpace(notificationID), strings.TrimSpace(userID))
	return err
}

func (r *Repository) updateEmailStatus(ctx context.Context, notificationID string, status string, sentAt *time.Time, emailError string) error {
	_, err := r.db.Exec(ctx, `
		update expiry_notifications
		set email_status = $2,
		    email_sent_at = $3,
		    email_error = $4,
		    updated_at = now()
		where id = $1
	`, notificationID, strings.TrimSpace(status), sentAt, strings.TrimSpace(emailError))
	return err
}

func (r *Repository) listRecentEmailDeliveries(ctx context.Context, limit int) ([]resources.NotificationDeliveryRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(ctx, `
		select n.id, n.user_id, coalesce(u.display_name, n.user_id) as user_name, coalesce(u.email, '') as user_email,
		       n.resource_id, n.resource_name, n.credential_key_id, n.credential_display_name, n.credential_type,
		       n.reminder_day, n.title, n.email_status, n.email_sent_at, n.email_error, n.created_at
		from expiry_notifications n
		left join app_users u on u.id = n.user_id
		where cardinality(n.channels) > 0
		  and 'email' = any(n.channels)
		order by n.created_at desc
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []resources.NotificationDeliveryRecord{}
	for rows.Next() {
		var item resources.NotificationDeliveryRecord
		var emailSentAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.UserName, &item.UserEmail,
			&item.ResourceID, &item.ResourceName, &item.CredentialKeyID, &item.CredentialDisplayName, &item.CredentialType,
			&item.ReminderDay, &item.Title, &item.EmailStatus, &emailSentAt, &item.EmailError, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.EmailSentAt = timeFromPg(emailSentAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNotification(row scanner) (resources.UserNotification, error) {
	var item resources.UserNotification
	var credentialEnd pgtype.Timestamptz
	var readAt pgtype.Timestamptz
	var emailSentAt pgtype.Timestamptz
	var channels []string
	if err := row.Scan(
		&item.ID, &item.UserID, &item.ResourceID, &item.ResourceName, &item.CredentialKeyID, &item.CredentialDisplayName,
		&item.CredentialType, &credentialEnd, &item.ReminderDay, &item.Title, &item.Body, &channels,
		&readAt, &item.EmailStatus, &emailSentAt, &item.EmailError, &item.CreatedAt,
	); err != nil {
		return resources.UserNotification{}, err
	}
	item.CredentialEndDateTime = timeFromPg(credentialEnd)
	item.ReadAt = timeFromPg(readAt)
	item.EmailSentAt = timeFromPg(emailSentAt)
	item.Channels = notificationChannelsFromStrings(channels)
	return item, nil
}

func notificationChannelsToStrings(channels []resources.NotificationChannel) []string {
	items := make([]string, 0, len(channels))
	for _, channel := range channels {
		items = append(items, string(channel))
	}
	return items
}

func notificationChannelsFromStrings(channels []string) []resources.NotificationChannel {
	items := make([]resources.NotificationChannel, 0, len(channels))
	for _, channel := range channels {
		trimmed := strings.TrimSpace(channel)
		if trimmed == "" {
			continue
		}
		items = append(items, resources.NotificationChannel(trimmed))
	}
	return items
}

func timeFromPg(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
