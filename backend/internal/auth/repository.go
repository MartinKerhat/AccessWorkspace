package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const externalPasswordPlaceholder = "external-auth-managed"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Authenticate(ctx context.Context, username, password string) (User, error) {
	var user User
	var passwordHash string
	var blocked bool
	err := r.db.QueryRow(ctx, `
		select id, display_name, email, groups, is_admin, workspace_blocked, direct_rights, password_hash
		from app_users
		where lower(username) = lower($1)
	`, strings.TrimSpace(username)).Scan(&user.ID, &user.Name, &user.Email, &user.Groups, &user.IsAdmin, &blocked, &user.DirectRights, &passwordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUnauthenticated
		}
		return User{}, err
	}
	if blocked {
		return User{}, ErrBlocked
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return User{}, ErrUnauthenticated
	}
	return user, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token := uuid.NewString()
	_, err := r.db.Exec(ctx, `
		insert into auth_sessions (token, user_id, expires_at)
		values ($1, $2, now() + ($3 * interval '1 second'))
	`, token, userID, int(ttl.Seconds()))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (r *Repository) CreateBrowserExtensionConnectToken(ctx context.Context, userID string, mode Mode, ttl time.Duration) (string, time.Time, error) {
	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(ttl)
	_, err := r.db.Exec(ctx, `
		insert into browser_extension_connect_tokens (token, user_id, auth_mode, expires_at)
		values ($1, $2, $3, $4)
	`, token, strings.TrimSpace(userID), string(mode), expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (r *Repository) ExchangeBrowserExtensionConnectToken(ctx context.Context, token string) (string, Mode, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var userID string
	var authMode string
	err = tx.QueryRow(ctx, `
		delete from browser_extension_connect_tokens
		where token = $1 and expires_at > now()
		returning user_id, auth_mode
	`, strings.TrimSpace(token)).Scan(&userID, &authMode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrUnauthenticated
		}
		return "", "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, Mode(authMode), nil
}

func (r *Repository) UpsertBrowserExtensionSession(ctx context.Context, userID string, installationID string, ttl time.Duration) (string, error) {
	token := uuid.NewString()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		delete from browser_extension_sessions
		where installation_id = $1
	`, strings.TrimSpace(installationID)); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		insert into browser_extension_sessions (token, user_id, installation_id, expires_at)
		values ($1, $2, $3, now() + ($4 * interval '1 second'))
	`, token, strings.TrimSpace(userID), strings.TrimSpace(installationID), int(ttl.Seconds())); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, nil
}

func (r *Repository) UserByToken(ctx context.Context, token string) (User, error) {
	var user User
	var blocked bool
	err := r.db.QueryRow(ctx, `
		select u.id, u.display_name, u.email, u.groups, u.is_admin, u.workspace_blocked, u.direct_rights
		from auth_sessions s
		join app_users u on u.id = s.user_id
		where s.token = $1 and s.expires_at > now()
	`, token).Scan(&user.ID, &user.Name, &user.Email, &user.Groups, &user.IsAdmin, &blocked, &user.DirectRights)
	if err == nil {
		if blocked {
			return User{}, ErrBlocked
		}
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}

	err = r.db.QueryRow(ctx, `
		select u.id, u.display_name, u.email, u.groups, u.is_admin, u.workspace_blocked, u.direct_rights
		from browser_extension_sessions s
		join app_users u on u.id = s.user_id
		where s.token = $1 and s.expires_at > now()
	`, token).Scan(&user.ID, &user.Name, &user.Email, &user.Groups, &user.IsAdmin, &blocked, &user.DirectRights)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUnauthenticated
		}
		return User{}, err
	}
	if _, touchErr := r.db.Exec(ctx, `
		update browser_extension_sessions
		set last_used_at = now()
		where token = $1
	`, token); touchErr != nil {
		return User{}, touchErr
	}
	if blocked {
		return User{}, ErrBlocked
	}
	return user, nil
}

func (r *Repository) IsBlocked(ctx context.Context, userID string) (bool, error) {
	var blocked bool
	err := r.db.QueryRow(ctx, `
		select workspace_blocked
		from app_users
		where id = $1
	`, strings.TrimSpace(userID)).Scan(&blocked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return blocked, nil
}

func (r *Repository) ResolveAuthorization(ctx context.Context, user User, defaultDirectRightsJSON string) (User, error) {
	if strings.TrimSpace(user.ID) != "" {
		if storedDirectRights, err := r.userDirectRightsByID(ctx, user.ID); err != nil {
			return User{}, err
		} else if len(storedDirectRights) > 0 {
			user.DirectRights = uniqueNormalized(append(user.DirectRights, storedDirectRights...))
		}
	}

	localGroups, err := r.ListLocalGroups(ctx)
	if err != nil {
		return User{}, err
	}

	directRights, err := r.loadDirectRightsMap(ctx, defaultDirectRightsJSON)
	if err != nil {
		return User{}, err
	}

	return resolveAuthorizationFromInputs(user, localGroups, directRights), nil
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `
		with deleted_auth as (
			delete from auth_sessions where token = $1
		)
		delete from browser_extension_sessions where token = $1
	`, token)
	return err
}

func (r *Repository) UpsertExternalUser(ctx context.Context, user User) error {
	passwordHash, err := HashPassword(externalPasswordPlaceholder)
	if err != nil {
		return err
	}

	if user.Groups == nil {
		user.Groups = []string{}
	}

	username := "entra:" + user.ID
	_, err = r.db.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash, groups, is_admin, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, now())
		on conflict (id) do update
		set display_name = excluded.display_name,
		    email = excluded.email,
		    groups = excluded.groups,
		    is_admin = excluded.is_admin,
		    updated_at = now()
	`, user.ID, username, user.Name, user.Email, passwordHash, user.Groups, user.IsAdmin)
	return err
}

func (r *Repository) ListLocalGroups(ctx context.Context) ([]LocalGroup, error) {
	rows, err := r.db.Query(ctx, `
		select name, description, rights, mapped_external_groups, assigned_user_ids
		from local_groups
		order by name asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []LocalGroup{}
	for rows.Next() {
		var item LocalGroup
		if err := rows.Scan(&item.Name, &item.Description, &item.Rights, &item.MappedExternalGroups, &item.AssignedUserIDs); err != nil {
			return nil, err
		}
		item.Rights = uniqueNormalized(item.Rights)
		item.MappedExternalGroups = uniqueNormalized(item.MappedExternalGroups)
		item.AssignedUserIDs = uniqueNormalized(item.AssignedUserIDs)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) listStoredUsers(ctx context.Context) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		select id, display_name, email, groups, is_admin, workspace_blocked, direct_rights
		from app_users
		order by display_name asc, email asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []User{}
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Groups, &item.IsAdmin, &item.Blocked, &item.DirectRights); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateUser(ctx context.Context, input CreateUserInput) (string, error) {
	input.Username = strings.TrimSpace(strings.ToLower(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Password = strings.TrimSpace(input.Password)
	input.DirectLocalGroups = uniqueNormalized(input.DirectLocalGroups)

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return "", err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		select name, description, rights, mapped_external_groups, assigned_user_ids
		from local_groups
		order by name asc
	`)
	if err != nil {
		return "", err
	}
	localGroups := []LocalGroup{}
	for rows.Next() {
		var group LocalGroup
		if err := rows.Scan(&group.Name, &group.Description, &group.Rights, &group.MappedExternalGroups, &group.AssignedUserIDs); err != nil {
			rows.Close()
			return "", err
		}
		group.Rights = uniqueNormalized(group.Rights)
		group.MappedExternalGroups = uniqueNormalized(group.MappedExternalGroups)
		group.AssignedUserIDs = uniqueNormalized(group.AssignedUserIDs)
		localGroups = append(localGroups, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	validGroups := map[string]struct{}{}
	for _, group := range localGroups {
		validGroups[group.Name] = struct{}{}
	}
	for _, name := range input.DirectLocalGroups {
		if _, ok := validGroups[name]; !ok {
			return "", fmt.Errorf("%w: one or more local groups do not exist", ErrInvalidInput)
		}
	}

	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash, groups, is_admin, workspace_blocked, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
	`, id, input.Username, input.DisplayName, input.Email, passwordHash, []string{}, input.IsAdmin, input.Blocked); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch {
			case strings.Contains(pgErr.ConstraintName, "username"):
				return "", fmt.Errorf("%w: username already exists", ErrInvalidInput)
			case strings.Contains(pgErr.ConstraintName, "email"):
				return "", fmt.Errorf("%w: email already exists", ErrInvalidInput)
			default:
				return "", fmt.Errorf("%w: user already exists", ErrInvalidInput)
			}
		}
		return "", err
	}

	selectedGroups := map[string]struct{}{}
	for _, name := range input.DirectLocalGroups {
		selectedGroups[name] = struct{}{}
	}
	for _, group := range localGroups {
		assignments := append([]string{}, group.AssignedUserIDs...)
		if _, ok := selectedGroups[group.Name]; ok {
			assignments = append(assignments, id)
		}
		assignments = uniqueNormalized(assignments)
		if _, err := tx.Exec(ctx, `
			update local_groups
			set assigned_user_ids = $2, updated_at = now()
			where name = $1
		`, group.Name, assignments); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) userByID(ctx context.Context, id string) (User, error) {
	var item User
	err := r.db.QueryRow(ctx, `
		select id, display_name, email, groups, is_admin, workspace_blocked, direct_rights
		from app_users
		where id = $1
	`, strings.TrimSpace(id)).Scan(&item.ID, &item.Name, &item.Email, &item.Groups, &item.IsAdmin, &item.Blocked, &item.DirectRights)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return item, nil
}

// DeleteUser removes a user and cascades cleanup of everything tied to that
// user's identity. Personal resources (owner_user_id + personal) are deleted
// outright — they are secrets only the owner could ever see, so once the owner
// is gone they must not linger in the database. Sessions, tokens, and
// notifications are removed by ON DELETE CASCADE from app_users; personal
// resources, the user's connection credential overrides, and local-group
// assignments have no FK to app_users and are cleaned up explicitly here.
//
// Shared resources owned by the user are intentionally left in place (they are
// visible to others via allowed groups); only their now-dangling owner_user_id
// remains, which no longer grants anyone access.
func (r *Repository) DeleteUser(ctx context.Context, id string) (DeleteUserResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DeleteUserResult{}, ErrNotFound
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return DeleteUserResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var exists bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from app_users where id = $1)`, id).Scan(&exists); err != nil {
		return DeleteUserResult{}, err
	}
	if !exists {
		return DeleteUserResult{}, ErrNotFound
	}

	// Delete the user's personal resources. resource_secrets,
	// connection_user_password_overrides referencing them, and any notification
	// policies/rows keyed on the resource are removed via ON DELETE CASCADE.
	tag, err := tx.Exec(ctx, `delete from resources where owner_user_id = $1 and personal = true`, id)
	if err != nil {
		return DeleteUserResult{}, err
	}
	personalDeleted := int(tag.RowsAffected())

	// Remove the user's personal credential overrides on connections they don't own.
	if _, err := tx.Exec(ctx, `delete from connection_user_password_overrides where user_id = $1`, id); err != nil {
		return DeleteUserResult{}, err
	}

	// Drop the user from every local group's assignment list.
	if _, err := tx.Exec(ctx, `
		update local_groups
		set assigned_user_ids = array_remove(assigned_user_ids, $1), updated_at = now()
		where $1 = any(assigned_user_ids)
	`, id); err != nil {
		return DeleteUserResult{}, err
	}

	// Finally remove the account; FK cascades clear sessions, extension tokens,
	// and app-registration notifications.
	if _, err := tx.Exec(ctx, `delete from app_users where id = $1`, id); err != nil {
		return DeleteUserResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return DeleteUserResult{}, err
	}
	return DeleteUserResult{PersonalResourcesDeleted: personalDeleted}, nil
}

func (r *Repository) countAdmins(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx, `select count(*) from app_users where is_admin = true`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) CreateLocalGroup(ctx context.Context, input LocalGroupInput) error {
	input = normalizeLocalGroupInput(input)
	_, err := r.db.Exec(ctx, `
		insert into local_groups (name, description, rights, mapped_external_groups, assigned_user_ids, updated_at)
		values ($1, $2, $3, $4, $5, now())
	`, input.Name, input.Description, input.Rights, input.MappedExternalGroups, input.AssignedUserIDs)
	return err
}

func (r *Repository) UpdateLocalGroup(ctx context.Context, name string, input LocalGroupInput) error {
	input = normalizeLocalGroupInput(input)
	_, err := r.db.Exec(ctx, `
		update local_groups
		set description = $2,
		    rights = $3,
		    mapped_external_groups = $4,
		    assigned_user_ids = $5,
		    updated_at = now()
		where name = $1
	`, strings.TrimSpace(name), input.Description, input.Rights, input.MappedExternalGroups, input.AssignedUserIDs)
	return err
}

func (r *Repository) UpdateUserAccess(ctx context.Context, id string, input UserAccessUpdateInput) error {
	id = strings.TrimSpace(id)
	input.DirectLocalGroups = uniqueNormalized(input.DirectLocalGroups)
	input.DirectRights = uniqueNormalized(input.DirectRights)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var email string
	if err := tx.QueryRow(ctx, `select email from app_users where id = $1`, id).Scan(&email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	rows, err := tx.Query(ctx, `
		select name, description, rights, mapped_external_groups, assigned_user_ids
		from local_groups
		order by name asc
	`)
	if err != nil {
		return err
	}
	localGroups := []LocalGroup{}
	for rows.Next() {
		var group LocalGroup
		if err := rows.Scan(&group.Name, &group.Description, &group.Rights, &group.MappedExternalGroups, &group.AssignedUserIDs); err != nil {
			rows.Close()
			return err
		}
		group.Rights = uniqueNormalized(group.Rights)
		group.MappedExternalGroups = uniqueNormalized(group.MappedExternalGroups)
		group.AssignedUserIDs = uniqueNormalized(group.AssignedUserIDs)
		localGroups = append(localGroups, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	validGroups := map[string]struct{}{}
	for _, group := range localGroups {
		validGroups[group.Name] = struct{}{}
	}
	for _, name := range input.DirectLocalGroups {
		if _, ok := validGroups[name]; !ok {
			return ErrInvalidInput
		}
	}

	selectedGroups := map[string]struct{}{}
	for _, name := range input.DirectLocalGroups {
		selectedGroups[name] = struct{}{}
	}

	for _, group := range localGroups {
		assignments := make([]string, 0, len(group.AssignedUserIDs)+1)
		for _, assigned := range group.AssignedUserIDs {
			if assigned == id || assigned == email {
				continue
			}
			assignments = append(assignments, assigned)
		}
		if _, ok := selectedGroups[group.Name]; ok {
			assignments = append(assignments, id)
		}
		assignments = uniqueNormalized(assignments)
		if _, err := tx.Exec(ctx, `
			update local_groups
			set assigned_user_ids = $2, updated_at = now()
			where name = $1
		`, group.Name, assignments); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		update app_users
		set workspace_blocked = $2,
		    direct_rights = $3,
		    updated_at = now()
		where id = $1
	`, id, input.Blocked, input.DirectRights); err != nil {
		return err
	}

	if input.Blocked {
		if _, err := tx.Exec(ctx, `delete from auth_sessions where user_id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from browser_extension_sessions where user_id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from browser_extension_connect_tokens where user_id = $1`, id); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *Repository) userDirectRightsByID(ctx context.Context, id string) ([]string, error) {
	var directRights []string
	err := r.db.QueryRow(ctx, `
		select direct_rights
		from app_users
		where id = $1
	`, strings.TrimSpace(id)).Scan(&directRights)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return uniqueNormalized(directRights), nil
}

func (r *Repository) loadDirectRightsMap(ctx context.Context, fallback string) (map[string][]string, error) {
	value := strings.TrimSpace(fallback)
	if err := r.db.QueryRow(ctx, `select value from admin_settings where key = 'entra_direct_rights_json'`).Scan(&value); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return map[string][]string{}, nil
	}

	var parsed map[string][]string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, err
	}
	for key, rights := range parsed {
		parsed[key] = uniqueNormalized(rights)
	}
	return parsed, nil
}

func normalizeLocalGroupInput(input LocalGroupInput) LocalGroupInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Rights = uniqueNormalized(input.Rights)
	input.MappedExternalGroups = uniqueNormalized(input.MappedExternalGroups)
	input.AssignedUserIDs = uniqueNormalized(input.AssignedUserIDs)
	return input
}

func uniqueNormalized(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
