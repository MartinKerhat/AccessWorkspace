package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SessionClient is what the server knows about the client that opened a
// session, captured once at issue time (login, SSO callback, invite accept,
// extension connect-exchange). It travels on the request context so the
// session-issuing paths pick it up without threading it through every
// Authenticator signature.
type SessionClient struct {
	IP        string
	UserAgent string
}

type sessionClientKey struct{}

// WithSessionClient attaches the calling client's details to ctx.
func WithSessionClient(ctx context.Context, client SessionClient) context.Context {
	return context.WithValue(ctx, sessionClientKey{}, client)
}

func sessionClientFrom(ctx context.Context) SessionClient {
	client, _ := ctx.Value(sessionClientKey{}).(SessionClient)
	client.IP = strings.TrimSpace(client.IP)
	client.UserAgent = strings.TrimSpace(client.UserAgent)
	// Bound what we store; a user agent is never legitimately this long.
	if len(client.UserAgent) > 512 {
		client.UserAgent = client.UserAgent[:512]
	}
	return client
}

// Session kinds as exposed by the API.
const (
	SessionKindWeb       = "web"
	SessionKindExtension = "extension"
)

// SessionInfo is one live session as shown to its owner or to an admin. The
// raw user agent stays server-side; Client is a short derived label.
type SessionInfo struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"createdAt"`
	LastUsedAt    time.Time `json:"lastUsedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	IP            string    `json:"ip"`
	Client        string    `json:"client"`
	Current       bool      `json:"current"`
	VaultUnlocked bool      `json:"vaultUnlocked"`
}

// clientLabel reduces a user agent to "Browser on OS" (plus an extension
// marker). It is a convenience for recognising a row in a list, not
// fingerprinting — unknown agents simply read as unknown.
func clientLabel(kind, userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	browser := ""
	switch {
	case ua == "":
		browser = ""
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "EdgA/") || strings.Contains(ua, "EdgiOS/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	default:
		browser = "Unknown browser"
	}
	os := ""
	switch {
	case ua == "":
		os = ""
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "CrOS"):
		os = "ChromeOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	label := browser
	if os != "" {
		if label == "" {
			label = os
		} else {
			label += " on " + os
		}
	}
	if kind == SessionKindExtension {
		if label == "" {
			return "Browser extension"
		}
		return "Browser extension · " + label
	}
	if label == "" {
		return "Unknown client"
	}
	return label
}

// noCurrentSession is a token-hash sentinel that can never equal a stored
// hash (all stored hashes carry the sha256: prefix), used when the caller
// has no "current" session to mark — admin views.
const noCurrentSession = "-"

func currentSessionHash(token string) string {
	if strings.TrimSpace(token) == "" {
		return noCurrentSession
	}
	return hashToken(token)
}

// ListSessions returns the user's live sessions across both tables, most
// recently used first. currentToken (raw) marks the caller's own row.
func (r *Repository) ListSessions(ctx context.Context, userID string, currentToken string) ([]SessionInfo, error) {
	rows, err := r.db.Query(ctx, `
		select s.id::text, 'web', s.created_at, s.last_used_at, s.expires_at, s.created_ip, s.user_agent,
		       s.vault_private_key <> '', s.token = $2
		from auth_sessions s
		where s.user_id = $1 and s.expires_at > now()
		union all
		select s.id::text, 'extension', s.created_at, s.last_used_at, s.expires_at, s.created_ip, s.user_agent,
		       s.vault_private_key <> '', s.token = $2
		from browser_extension_sessions s
		where s.user_id = $1 and s.expires_at > now()
		order by 4 desc
	`, strings.TrimSpace(userID), currentSessionHash(currentToken))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SessionInfo, 0)
	for rows.Next() {
		var item SessionInfo
		var userAgent string
		if err := rows.Scan(&item.ID, &item.Kind, &item.CreatedAt, &item.LastUsedAt, &item.ExpiresAt, &item.IP, &userAgent, &item.VaultUnlocked, &item.Current); err != nil {
			return nil, err
		}
		item.Client = clientLabel(item.Kind, userAgent)
		items = append(items, item)
	}
	return items, rows.Err()
}

// DeleteSessionByID removes one session that belongs to userID. The user
// filter is part of the delete itself, so a foreign id is simply not found.
// Returns the kind of the deleted row and whether it was the caller's own.
func (r *Repository) DeleteSessionByID(ctx context.Context, userID, sessionID, currentToken string) (string, bool, error) {
	var kind string
	var wasCurrent bool
	err := r.db.QueryRow(ctx, `
		with deleted_web as (
			delete from auth_sessions
			where user_id = $1 and id::text = $2
			returning 'web' as kind, token = $3 as was_current
		), deleted_ext as (
			delete from browser_extension_sessions
			where user_id = $1 and id::text = $2
			returning 'extension' as kind, token = $3 as was_current
		)
		select kind, was_current from deleted_web
		union all
		select kind, was_current from deleted_ext
		limit 1
	`, strings.TrimSpace(userID), strings.TrimSpace(sessionID), currentSessionHash(currentToken)).Scan(&kind, &wasCurrent)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, ErrNotFound
		}
		return "", false, err
	}
	return kind, wasCurrent, nil
}

// DeleteOtherSessions ends every session of the user except the one holding
// currentToken, plus any pending extension connect tokens (a connect token
// would otherwise turn into a fresh extension session a moment later).
func (r *Repository) DeleteOtherSessions(ctx context.Context, userID string, currentToken string) (int, error) {
	return r.deleteSessions(ctx, userID, currentSessionHash(currentToken))
}

// DeleteAllSessions ends every session and pending connect token of the user.
func (r *Repository) DeleteAllSessions(ctx context.Context, userID string) (int, error) {
	return r.deleteSessions(ctx, userID, noCurrentSession)
}

func (r *Repository) deleteSessions(ctx context.Context, userID string, keepHash string) (int, error) {
	userID = strings.TrimSpace(userID)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	total := 0
	for _, table := range []string{"auth_sessions", "browser_extension_sessions"} {
		tag, err := tx.Exec(ctx, `delete from `+table+` where user_id = $1 and token <> $2 and expires_at > now()`, userID, keepHash)
		if err != nil {
			return 0, err
		}
		total += int(tag.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `delete from browser_extension_connect_tokens where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return total, nil
}

// PurgeExpiredSessions drops session rows that expired more than a week ago.
// Expired rows were only ever filtered out, never removed; this keeps the
// tables small. Runs from the periodic sync tick.
func (r *Repository) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	var total int64
	for _, table := range []string{"auth_sessions", "browser_extension_sessions", "browser_extension_connect_tokens"} {
		tag, err := r.db.Exec(ctx, `delete from `+table+` where expires_at < now() - interval '7 days'`)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
	}
	return total, nil
}

// ---- service layer ----

// ListOwnSessions lists the caller's live sessions; currentToken marks the
// one making the request.
func (s *Service) ListOwnSessions(ctx context.Context, user User, currentToken string) ([]SessionInfo, error) {
	return s.repo.ListSessions(ctx, user.ID, currentToken)
}

// RevokeOwnSession ends one of the caller's sessions. Returns the kind and
// whether the caller just ended the session they are using (the handler then
// clears the cookie like a logout).
func (s *Service) RevokeOwnSession(ctx context.Context, user User, sessionID string, currentToken string) (string, bool, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", false, ErrNotFound
	}
	return s.repo.DeleteSessionByID(ctx, user.ID, sessionID, currentToken)
}

// RevokeOtherSessions is "sign out everywhere else".
func (s *Service) RevokeOtherSessions(ctx context.Context, user User, currentToken string) (int, error) {
	return s.repo.DeleteOtherSessions(ctx, user.ID, currentToken)
}

// ListUserSessions is the admin view of another user's sessions.
func (s *Service) ListUserSessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	if _, err := s.repo.userByID(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.ListSessions(ctx, userID, "")
}

// RevokeUserSession lets an admin end one session of a user.
func (s *Service) RevokeUserSession(ctx context.Context, userID, sessionID string) (string, error) {
	if _, err := s.repo.userByID(ctx, userID); err != nil {
		return "", err
	}
	kind, _, err := s.repo.DeleteSessionByID(ctx, userID, sessionID, "")
	return kind, err
}

// RevokeAllUserSessions lets an admin sign a user out everywhere without
// blocking them.
func (s *Service) RevokeAllUserSessions(ctx context.Context, userID string) (int, error) {
	if _, err := s.repo.userByID(ctx, userID); err != nil {
		return 0, err
	}
	return s.repo.DeleteAllSessions(ctx, userID)
}

// PurgeExpiredSessions removes long-expired session rows.
func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.PurgeExpiredSessions(ctx)
}
