package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Generator parts a user can keep preferences for. The settings document is
// opaque to the server (the UI owns its shape); the server only bounds its
// size and makes sure it is a JSON object.
var preferenceParts = map[string]bool{
	"password":    true,
	"passphrase":  true,
	"key":         true,
	"token":       true,
	"rsa":         true,
	"ec":          true,
	"ssh":         true,
	"certificate": true,
}

const maxPreferenceBytes = 4096

// ValidatePreference checks the part name and the settings document.
func ValidatePreference(part string, settings json.RawMessage) error {
	if !preferenceParts[strings.TrimSpace(part)] {
		return fmt.Errorf("%w: unknown generator part %q", ErrInvalidInput, part)
	}
	if len(settings) == 0 || len(settings) > maxPreferenceBytes {
		return fmt.Errorf("%w: settings must be a JSON object of at most %d bytes", ErrInvalidInput, maxPreferenceBytes)
	}
	var object map[string]any
	if err := json.Unmarshal(settings, &object); err != nil || object == nil {
		return fmt.Errorf("%w: settings must be a JSON object", ErrInvalidInput)
	}
	return nil
}

// PreferenceStore persists per-user generator settings.
type PreferenceStore struct {
	db *pgxpool.Pool
}

func NewPreferenceStore(db *pgxpool.Pool) *PreferenceStore {
	return &PreferenceStore{db: db}
}

// List returns every stored part for the user, keyed by part name.
func (s *PreferenceStore) List(ctx context.Context, userID string) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, `select part, settings from user_generator_preferences where user_id = $1`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var part string
		var settings []byte
		if err := rows.Scan(&part, &settings); err != nil {
			return nil, err
		}
		out[part] = json.RawMessage(settings)
	}
	return out, rows.Err()
}

// Save upserts one part's settings for the user.
func (s *PreferenceStore) Save(ctx context.Context, userID, part string, settings json.RawMessage) error {
	if err := ValidatePreference(part, settings); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		insert into user_generator_preferences (user_id, part, settings, updated_at)
		values ($1, $2, $3::jsonb, now())
		on conflict (user_id, part) do update
		set settings = excluded.settings, updated_at = now()
	`, strings.TrimSpace(userID), strings.TrimSpace(part), string(settings))
	return err
}
