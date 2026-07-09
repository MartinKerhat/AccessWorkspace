package resources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// UpgradeSecretEncryption rewrites stored secret values in older formats
// (pre-encryption plaintext or v1 single-key ciphertext) as v2 envelopes, and
// fully heals rows corrupted by the historical double-encryption bug —
// including v2 envelopes whose inner content is still ciphertext. Runs at
// startup; healthy v2 rows are left untouched.
func (r *Repository) UpgradeSecretEncryption(ctx context.Context, cipher *SecretCipher) error {
	rows, err := r.db.Query(ctx, `
		select resource_id, secret_value
		from resource_secrets
		where secret_value <> ''
	`)
	if err != nil {
		return err
	}
	pending := map[string]string{}
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			rows.Close()
			return err
		}
		pending[id] = value
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for id, value := range pending {
		plain, layers, err := cipher.DecryptFully(ctx, value)
		if err != nil {
			return err
		}
		// Healthy v2 row: single layer, current format — leave untouched.
		if layers == 1 && !NeedsEncryptionUpgrade(value) {
			continue
		}
		encrypted, err := cipher.EncryptForStorage(ctx, plain, SecretClassShared)
		if err != nil {
			return err
		}
		// Guard on the old value so a concurrent update is not clobbered.
		if _, err := r.db.Exec(ctx, `
			update resource_secrets
			set secret_value = $2, updated_at = now()
			where resource_id = $1 and secret_value = $3
		`, id, encrypted, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter Filter) ([]ResourceSummary, error) {
	rows, err := r.db.Query(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain,
			r.vault_name, r.object_name, r.provider, r.application_id, r.credential_expires_at, r.expires_at,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at
		from resources r
		where r.archived_at is null
			and ($1 = '' or
				r.name ilike '%' || $1 || '%' or
				r.description ilike '%' || $1 || '%' or
				r.folder_path ilike '%' || $1 || '%' or
				r.target_host ilike '%' || $1 || '%' or
				r.target_url ilike '%' || $1 || '%' or
				r.target_system ilike '%' || $1 || '%' or
				r.vault_name ilike '%' || $1 || '%' or
				r.object_name ilike '%' || $1 || '%' or
				r.provider ilike '%' || $1 || '%' or
				r.application_id ilike '%' || $1 || '%')
			and ($2 = '' or r.type = $2)
			and ($3 = '' or r.target_host ilike '%' || $3 || '%' or r.target_url ilike '%' || $3 || '%' or r.target_system ilike '%' || $3 || '%' or r.folder_path ilike '%' || $3 || '%')
		order by r.folder_path asc, r.name asc
	`, strings.TrimSpace(filter.Query), filter.Type, strings.TrimSpace(filter.Host))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ResourceSummary{}
	for rows.Next() {
		item, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Resource, error) {
	row := r.db.QueryRow(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.source_object_id, r.last_synced_at, r.notes,
			r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain, r.connection_admin_session, r.connection_automatic_logon, r.connection_window_mode,
			r.connection_use_multiple_monitors, r.connection_show_connection_bar, r.connection_screen_mode, r.connection_mac_address,
			r.vault_name, r.object_name, r.object_type, r.object_version, r.content_type, r.expires_at,
			r.provider, r.application_id, r.tenant_id, r.client_id, r.credential_type, r.credential_expires_at,
			r.display_name_external, r.linked_secret_ref,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at,
			rs.secret_mode, rs.secret_value, rs.secret_reference
		from resources r
		join resource_secrets rs on rs.resource_id = r.id
		where r.id = $1 and r.archived_at is null
	`, id)
	item, err := scanResource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	if err := r.hydrateAppRegistrationSnapshot(ctx, &item); err != nil {
		return Resource{}, err
	}
	return item, nil
}

func (r *Repository) GetAny(ctx context.Context, id string) (Resource, error) {
	row := r.db.QueryRow(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.source_object_id, r.last_synced_at, r.notes,
			r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain, r.connection_admin_session, r.connection_automatic_logon, r.connection_window_mode,
			r.connection_use_multiple_monitors, r.connection_show_connection_bar, r.connection_screen_mode, r.connection_mac_address,
			r.vault_name, r.object_name, r.object_type, r.object_version, r.content_type, r.expires_at,
			r.provider, r.application_id, r.tenant_id, r.client_id, r.credential_type, r.credential_expires_at,
			r.display_name_external, r.linked_secret_ref,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at,
			rs.secret_mode, rs.secret_value, rs.secret_reference
		from resources r
		join resource_secrets rs on rs.resource_id = r.id
		where r.id = $1
	`, id)
	item, err := scanResource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	if err := r.hydrateAppRegistrationSnapshot(ctx, &item); err != nil {
		return Resource{}, err
	}
	return item, nil
}

func (r *Repository) ListManagedKeyVault(ctx context.Context) ([]Resource, error) {
	rows, err := r.db.Query(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.source_object_id, r.last_synced_at, r.notes,
			r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain, r.connection_admin_session, r.connection_automatic_logon, r.connection_window_mode,
			r.connection_use_multiple_monitors, r.connection_show_connection_bar, r.connection_screen_mode, r.connection_mac_address,
			r.vault_name, r.object_name, r.object_type, r.object_version, r.content_type, r.expires_at,
			r.provider, r.application_id, r.tenant_id, r.client_id, r.credential_type, r.credential_expires_at,
			r.display_name_external, r.linked_secret_ref,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at,
			rs.secret_mode, rs.secret_value, rs.secret_reference
		from resources r
		join resource_secrets rs on rs.resource_id = r.id
		where r.archived_at is null
			and r.type = $1
			and r.source_kind = $2
		order by r.name asc
	`, TypeKeyVaultSecret, SourceKindAzureKeyVault)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Resource{}
	for rows.Next() {
		item, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListManagedAppRegistrations(ctx context.Context) ([]Resource, error) {
	rows, err := r.db.Query(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.source_object_id, r.last_synced_at, r.notes,
			r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain, r.connection_admin_session, r.connection_automatic_logon, r.connection_window_mode,
			r.connection_use_multiple_monitors, r.connection_show_connection_bar, r.connection_screen_mode, r.connection_mac_address,
			r.vault_name, r.object_name, r.object_type, r.object_version, r.content_type, r.expires_at,
			r.provider, r.application_id, r.tenant_id, r.client_id, r.credential_type, r.credential_expires_at,
			r.display_name_external, r.linked_secret_ref,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at,
			rs.secret_mode, rs.secret_value, rs.secret_reference
		from resources r
		join resource_secrets rs on rs.resource_id = r.id
		where r.archived_at is null
			and r.type = $1
			and r.source_kind = $2
		order by r.name asc
	`, TypeAppRegistration, SourceKindEntraAppRegistration)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Resource{}
	for rows.Next() {
		item, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		if err := r.hydrateAppRegistrationSnapshot(ctx, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListArchived(ctx context.Context) ([]ArchivedResourceSummary, error) {
	rows, err := r.db.Query(ctx, `
		select
			r.id, r.name, r.type, r.personal, r.description, r.owner, r.owner_user_id, r.owner_team, r.environment, r.status,
			r.folder_path, r.launch_mode, r.source_kind, r.target_host, r.target_port, r.target_url, r.target_system, r.username,
			r.connection_domain,
			r.vault_name, r.object_name, r.provider, r.application_id, r.credential_expires_at, r.expires_at,
			r.launch_allowed, r.reveal_allowed, r.copy_allowed, r.allowed_groups, r.created_at, r.updated_at, r.archived_at,
			coalesce(a.user_name, ''), coalesce(a.metadata ->> 'reason', ''), a.created_at
		from resources r
		left join lateral (
			select event_type, user_name, metadata, created_at
			from audit_events
			where resource_id = r.id
				and event_type = 'resource_archived'
			order by created_at desc
			limit 1
		) a on true
		where r.archived_at is not null
		order by r.archived_at desc, r.name asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ArchivedResourceSummary{}
	for rows.Next() {
		item, err := scanArchivedSummary(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateResourceInput) (Resource, error) {
	id := uuid.NewString()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Resource{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		insert into resources (
			id, name, type, personal, description, owner, owner_user_id, owner_team, environment, status, folder_path, launch_mode, source_kind, source_object_id,
			last_synced_at, notes, target_host, target_port, target_url, target_system, username,
			connection_domain, connection_admin_session, connection_automatic_logon, connection_window_mode,
			connection_use_multiple_monitors, connection_show_connection_bar, connection_screen_mode, connection_mac_address,
			vault_name, object_name, object_type, object_version, content_type, expires_at,
			provider, application_id, tenant_id, client_id, credential_type, credential_expires_at,
			display_name_external, linked_secret_ref, launch_allowed, reveal_allowed, copy_allowed, allowed_groups
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21,
			$22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33, $34, $35,
			$36, $37, $38, $39, $40, $41,
			$42, $43, $44, $45, $46, $47
		)
	`, id, input.Name, input.Type, input.Personal, input.Description, input.Owner, input.OwnerUserID, input.OwnerTeam, input.Environment, input.Status, input.FolderPath, input.LaunchMode, input.SourceKind, input.SourceObjectID,
		input.LastSyncedAt, input.Notes, input.TargetHost, input.TargetPort, input.TargetURL, input.TargetSystem, input.Username,
		input.ConnectionDomain, input.ConnectionAdminSession, input.ConnectionAutomaticLogon, input.ConnectionWindowMode,
		input.ConnectionUseMultipleMonitors, input.ConnectionShowConnectionBar, input.ConnectionScreenMode, input.ConnectionMacAddress,
		input.VaultName, input.ObjectName, input.ObjectType, input.ObjectVersion, input.ContentType, input.ExpiresAt,
		input.Provider, input.ApplicationID, input.TenantID, input.ClientID, input.CredentialType, input.CredentialExpiresAt,
		input.DisplayNameExternal, input.LinkedSecretRef, input.LaunchAllowed, input.RevealAllowed, input.CopyAllowed, input.AllowedGroups)
	if err != nil {
		return Resource{}, err
	}

	_, err = tx.Exec(ctx, `
		insert into resource_secrets (resource_id, secret_mode, secret_value, secret_reference)
		values ($1, $2, $3, $4)
	`, id, input.SecretMode, input.SecretValue, input.SecretReference)
	if err != nil {
		return Resource{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Resource{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateResourceInput) (Resource, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Resource{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		update resources
		set
			name = $2,
			type = $3,
			personal = $4,
			description = $5,
			owner = $6,
			owner_user_id = $7,
			owner_team = $8,
			environment = $9,
			status = $10,
			folder_path = $11,
			launch_mode = $12,
			source_kind = $13,
			source_object_id = $14,
			last_synced_at = $15,
			notes = $16,
			target_host = $17,
			target_port = $18,
			target_url = $19,
			target_system = $20,
			username = $21,
			connection_domain = $22,
			connection_admin_session = $23,
			connection_automatic_logon = $24,
			connection_window_mode = $25,
			connection_use_multiple_monitors = $26,
			connection_show_connection_bar = $27,
			connection_screen_mode = $28,
			connection_mac_address = $29,
			vault_name = $30,
			object_name = $31,
			object_type = $32,
			object_version = $33,
			content_type = $34,
			expires_at = $35,
			provider = $36,
			application_id = $37,
			tenant_id = $38,
			client_id = $39,
			credential_type = $40,
			credential_expires_at = $41,
			display_name_external = $42,
			linked_secret_ref = $43,
			launch_allowed = $44,
			reveal_allowed = $45,
			copy_allowed = $46,
			allowed_groups = $47,
			updated_at = now()
		where id = $1
	`, id, input.Name, input.Type, input.Personal, input.Description, input.Owner, input.OwnerUserID, input.OwnerTeam, input.Environment, input.Status, input.FolderPath, input.LaunchMode, input.SourceKind,
		input.SourceObjectID, input.LastSyncedAt, input.Notes, input.TargetHost, input.TargetPort, input.TargetURL, input.TargetSystem,
		input.Username, input.ConnectionDomain, input.ConnectionAdminSession, input.ConnectionAutomaticLogon, input.ConnectionWindowMode,
		input.ConnectionUseMultipleMonitors, input.ConnectionShowConnectionBar, input.ConnectionScreenMode, input.ConnectionMacAddress,
		input.VaultName, input.ObjectName, input.ObjectType, input.ObjectVersion, input.ContentType, input.ExpiresAt,
		input.Provider, input.ApplicationID, input.TenantID, input.ClientID, input.CredentialType, input.CredentialExpiresAt,
		input.DisplayNameExternal, input.LinkedSecretRef, input.LaunchAllowed, input.RevealAllowed, input.CopyAllowed, input.AllowedGroups)
	if err != nil {
		return Resource{}, err
	}

	_, err = tx.Exec(ctx, `
		update resource_secrets
		set
			secret_mode = $2,
			secret_value = case when $3 = '' then secret_value else $3 end,
			secret_reference = case when $4 = '' then secret_reference else $4 end,
			updated_at = now()
		where resource_id = $1
	`, id, input.SecretMode, input.SecretValue, input.SecretReference)
	if err != nil {
		return Resource{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Resource{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `update resources set archived_at = now(), updated_at = now() where id = $1`, id)
	return err
}

func (r *Repository) Restore(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `update resources set archived_at = null, updated_at = now() where id = $1`, id)
	return err
}

func (r *Repository) GetConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string) (ConnectionCredentialOverride, error) {
	row := r.db.QueryRow(ctx, `
		select connection_id, password_resource_id, updated_at
		from connection_user_password_overrides
		where connection_id = $1 and user_id = $2
	`, connectionID, userID)

	var item ConnectionCredentialOverride
	var updatedAt pgtype.Timestamptz
	if err := row.Scan(&item.ConnectionID, &item.PasswordResourceID, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConnectionCredentialOverride{}, ErrNotFound
		}
		return ConnectionCredentialOverride{}, err
	}
	item.UpdatedAt = timeFromPg(updatedAt)
	return item, nil
}

func (r *Repository) UpsertConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string, passwordResourceID string) error {
	_, err := r.db.Exec(ctx, `
		insert into connection_user_password_overrides (connection_id, user_id, password_resource_id)
		values ($1, $2, $3)
		on conflict (connection_id, user_id) do update
		set password_resource_id = excluded.password_resource_id,
			updated_at = now()
	`, connectionID, userID, passwordResourceID)
	return err
}

func (r *Repository) DeleteConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string) error {
	_, err := r.db.Exec(ctx, `
		delete from connection_user_password_overrides
		where connection_id = $1 and user_id = $2
	`, connectionID, userID)
	return err
}

func (r *Repository) ReplaceAppRegistrationSnapshot(ctx context.Context, resourceID string, credentials []AppRegistrationCredential, owners []AppRegistrationOwner) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from app_registration_credentials where resource_id = $1`, resourceID); err != nil {
		return err
	}
	for _, credential := range credentials {
		if strings.TrimSpace(credential.KeyID) == "" || strings.TrimSpace(credential.CredentialType) == "" {
			continue
		}
		_, err := tx.Exec(ctx, `
			insert into app_registration_credentials (
				resource_id, key_id, credential_type, display_name, start_date_time, end_date_time, hint, usage, last_synced_at
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, resourceID, credential.KeyID, credential.CredentialType, credential.DisplayName,
			credential.StartDateTime, credential.EndDateTime, credential.Hint, credential.Usage, credential.LastSyncedAt)
		if err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `delete from app_registration_owners where resource_id = $1`, resourceID); err != nil {
		return err
	}
	for _, owner := range owners {
		if strings.TrimSpace(owner.OwnerID) == "" {
			continue
		}
		_, err := tx.Exec(ctx, `
			insert into app_registration_owners (
				resource_id, owner_id, owner_type, display_name, email
			) values ($1, $2, $3, $4, $5)
		`, resourceID, owner.OwnerID, owner.OwnerType, owner.DisplayName, owner.Email)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) ReplaceAppRegistrationNotificationPolicies(ctx context.Context, resourceID string, resourcePolicy *AppRegistrationNotificationPolicy, credentialPolicies []AppRegistrationCredentialPolicyInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from app_registration_notification_policies where resource_id = $1`, resourceID); err != nil {
		return err
	}

	if resourcePolicy != nil {
		if _, err := tx.Exec(ctx, `
			insert into app_registration_notification_policies (
				resource_id, credential_key_id, enabled, reminder_days, channels, updated_at
			) values ($1, '', $2, $3, $4, now())
		`, resourceID, resourcePolicy.Enabled, resourcePolicy.ReminderDays, notificationChannelsToStrings(resourcePolicy.Channels)); err != nil {
			return err
		}
	}

	for _, item := range credentialPolicies {
		if item.Policy == nil || strings.TrimSpace(item.KeyID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			insert into app_registration_notification_policies (
				resource_id, credential_key_id, enabled, reminder_days, channels, updated_at
			) values ($1, $2, $3, $4, $5, now())
		`, resourceID, strings.TrimSpace(item.KeyID), item.Policy.Enabled, item.Policy.ReminderDays, notificationChannelsToStrings(item.Policy.Channels)); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) hydrateAppRegistrationSnapshot(ctx context.Context, item *Resource) error {
	if item.Type != TypeAppRegistration {
		return nil
	}

	credentials, err := r.listAppRegistrationCredentials(ctx, item.ID)
	if err != nil {
		return err
	}
	owners, err := r.listAppRegistrationOwners(ctx, item.ID)
	if err != nil {
		return err
	}
	resourcePolicy, credentialPolicies, err := r.listAppRegistrationNotificationPolicies(ctx, item.ID)
	if err != nil {
		return err
	}
	item.AppCredentials = credentials
	item.AppOwners = owners
	item.AppNotificationPolicyOverride = resourcePolicy
	for i := range item.AppCredentials {
		if policy, ok := credentialPolicies[item.AppCredentials[i].KeyID]; ok {
			item.AppCredentials[i].NotificationPolicyOverride = policy
		}
	}
	return nil
}

func (r *Repository) listAppRegistrationCredentials(ctx context.Context, resourceID string) ([]AppRegistrationCredential, error) {
	rows, err := r.db.Query(ctx, `
		select resource_id, key_id, credential_type, display_name, start_date_time, end_date_time, hint, usage, last_synced_at
		from app_registration_credentials
		where resource_id = $1
		order by end_date_time asc nulls last, credential_type asc, display_name asc
	`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AppRegistrationCredential{}
	for rows.Next() {
		var item AppRegistrationCredential
		var startDateTime pgtype.Timestamptz
		var endDateTime pgtype.Timestamptz
		var lastSyncedAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ResourceID, &item.KeyID, &item.CredentialType, &item.DisplayName, &startDateTime, &endDateTime,
			&item.Hint, &item.Usage, &lastSyncedAt,
		); err != nil {
			return nil, err
		}
		item.StartDateTime = timeFromPg(startDateTime)
		item.EndDateTime = timeFromPg(endDateTime)
		item.LastSyncedAt = timeFromPg(lastSyncedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) listAppRegistrationOwners(ctx context.Context, resourceID string) ([]AppRegistrationOwner, error) {
	rows, err := r.db.Query(ctx, `
		select resource_id, owner_id, owner_type, display_name, email
		from app_registration_owners
		where resource_id = $1
		order by display_name asc, email asc
	`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AppRegistrationOwner{}
	for rows.Next() {
		var item AppRegistrationOwner
		if err := rows.Scan(&item.ResourceID, &item.OwnerID, &item.OwnerType, &item.DisplayName, &item.Email); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) listAppRegistrationNotificationPolicies(ctx context.Context, resourceID string) (*AppRegistrationNotificationPolicy, map[string]*AppRegistrationNotificationPolicy, error) {
	rows, err := r.db.Query(ctx, `
		select credential_key_id, enabled, reminder_days, channels
		from app_registration_notification_policies
		where resource_id = $1
	`, resourceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var resourcePolicy *AppRegistrationNotificationPolicy
	credentialPolicies := map[string]*AppRegistrationNotificationPolicy{}
	for rows.Next() {
		var credentialKeyID string
		var enabled bool
		var reminderDays []int
		var channels []string
		if err := rows.Scan(&credentialKeyID, &enabled, &reminderDays, &channels); err != nil {
			return nil, nil, err
		}
		policy := &AppRegistrationNotificationPolicy{
			Enabled:      enabled,
			ReminderDays: append([]int{}, reminderDays...),
			Channels:     notificationChannelsFromStrings(channels),
		}
		if strings.TrimSpace(credentialKeyID) == "" {
			resourcePolicy = policy
			continue
		}
		credentialPolicies[credentialKeyID] = policy
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return resourcePolicy, credentialPolicies, nil
}

type summaryScanner interface {
	Scan(dest ...any) error
}

func scanSummary(scanner summaryScanner) (ResourceSummary, error) {
	var item ResourceSummary
	var targetPort pgtype.Int4
	var credentialExpiresAt pgtype.Timestamptz
	var expiresAt pgtype.Timestamptz
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Type, &item.Personal, &item.Description, &item.Owner, &item.OwnerUserID, &item.OwnerTeam, &item.Environment, &item.Status,
		&item.FolderPath, &item.LaunchMode, &item.SourceKind, &item.TargetHost, &targetPort, &item.TargetURL, &item.TargetSystem, &item.Username,
		&item.ConnectionDomain, &item.VaultName, &item.ObjectName, &item.Provider, &item.ApplicationID, &credentialExpiresAt, &expiresAt,
		&item.LaunchAllowed, &item.RevealAllowed, &item.CopyAllowed, &item.AllowedGroups, &item.CreatedAt, &item.UpdatedAt, &item.ArchivedAt,
	); err != nil {
		return ResourceSummary{}, err
	}
	item.Category = CategoryForType(item.Type)
	if targetPort.Valid {
		port := int(targetPort.Int32)
		item.TargetPort = &port
	}
	item.CredentialExpiresAt = timeFromPg(credentialExpiresAt)
	item.ExpiresAt = timeFromPg(expiresAt)
	return item, nil
}

func scanResource(scanner summaryScanner) (Resource, error) {
	var item Resource
	var targetPort pgtype.Int4
	var lastSyncedAt pgtype.Timestamptz
	var expiresAt pgtype.Timestamptz
	var credentialExpiresAt pgtype.Timestamptz
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Type, &item.Personal, &item.Description, &item.Owner, &item.OwnerUserID, &item.OwnerTeam, &item.Environment, &item.Status,
		&item.FolderPath, &item.LaunchMode, &item.SourceKind, &item.SourceObjectID, &lastSyncedAt, &item.Notes,
		&item.TargetHost, &targetPort, &item.TargetURL, &item.TargetSystem, &item.Username,
		&item.ConnectionDomain, &item.ConnectionAdminSession, &item.ConnectionAutomaticLogon, &item.ConnectionWindowMode,
		&item.ConnectionUseMultipleMonitors, &item.ConnectionShowConnectionBar, &item.ConnectionScreenMode, &item.ConnectionMacAddress,
		&item.VaultName, &item.ObjectName, &item.ObjectType, &item.ObjectVersion, &item.ContentType, &expiresAt,
		&item.Provider, &item.ApplicationID, &item.TenantID, &item.ClientID, &item.CredentialType, &credentialExpiresAt,
		&item.DisplayNameExternal, &item.LinkedSecretRef,
		&item.LaunchAllowed, &item.RevealAllowed, &item.CopyAllowed, &item.AllowedGroups, &item.CreatedAt, &item.UpdatedAt, &item.ArchivedAt,
		&item.Secret.Mode, &item.Secret.Value, &item.Secret.Reference,
	); err != nil {
		return Resource{}, err
	}
	item.Category = CategoryForType(item.Type)
	if targetPort.Valid {
		port := int(targetPort.Int32)
		item.TargetPort = &port
	}
	item.LastSyncedAt = timeFromPg(lastSyncedAt)
	item.ExpiresAt = timeFromPg(expiresAt)
	item.CredentialExpiresAt = timeFromPg(credentialExpiresAt)
	return item, nil
}

func scanArchivedSummary(scanner summaryScanner) (ArchivedResourceSummary, error) {
	var item ArchivedResourceSummary
	var targetPort pgtype.Int4
	var credentialExpiresAt pgtype.Timestamptz
	var expiresAt pgtype.Timestamptz
	var archivedEventAt pgtype.Timestamptz
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Type, &item.Personal, &item.Description, &item.Owner, &item.OwnerUserID, &item.OwnerTeam, &item.Environment, &item.Status,
		&item.FolderPath, &item.LaunchMode, &item.SourceKind, &item.TargetHost, &targetPort, &item.TargetURL, &item.TargetSystem, &item.Username,
		&item.ConnectionDomain, &item.VaultName, &item.ObjectName, &item.Provider, &item.ApplicationID, &credentialExpiresAt, &expiresAt,
		&item.LaunchAllowed, &item.RevealAllowed, &item.CopyAllowed, &item.AllowedGroups, &item.CreatedAt, &item.UpdatedAt, &item.ArchivedAt,
		&item.ArchivedBy, &item.ArchivedReason, &archivedEventAt,
	); err != nil {
		return ArchivedResourceSummary{}, err
	}
	item.Category = CategoryForType(item.Type)
	if targetPort.Valid {
		port := int(targetPort.Int32)
		item.TargetPort = &port
	}
	item.CredentialExpiresAt = timeFromPg(credentialExpiresAt)
	item.ExpiresAt = timeFromPg(expiresAt)
	item.ArchivedEventAt = timeFromPg(archivedEventAt)
	return item, nil
}

func timeFromPg(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func notificationChannelsToStrings(channels []NotificationChannel) []string {
	items := make([]string, 0, len(channels))
	for _, channel := range channels {
		items = append(items, string(channel))
	}
	return items
}

func notificationChannelsFromStrings(channels []string) []NotificationChannel {
	items := make([]NotificationChannel, 0, len(channels))
	for _, channel := range channels {
		trimmed := strings.TrimSpace(channel)
		if trimmed == "" {
			continue
		}
		items = append(items, NotificationChannel(trimmed))
	}
	return items
}
