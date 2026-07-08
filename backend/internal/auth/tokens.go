package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Session and connect tokens are bearer credentials: anyone holding one can
// act as the user. Only their SHA-256 hash is stored so a database leak does
// not expose live sessions; clients keep presenting the raw token, which is
// re-hashed on every lookup.
const hashedTokenPrefix = "sha256:"

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hashedTokenPrefix + hex.EncodeToString(sum[:])
}

var tokenTables = []string{
	"auth_sessions",
	"browser_extension_sessions",
	"browser_extension_connect_tokens",
}

// HashPlaintextSessionTokens rewrites legacy plaintext tokens as hashes so
// existing sessions survive the upgrade. Runs at startup; already-hashed rows
// are left untouched.
func (r *Repository) HashPlaintextSessionTokens(ctx context.Context) error {
	for _, table := range tokenTables {
		rows, err := r.db.Query(ctx, `select token from `+table+` where token not like 'sha256:%'`)
		if err != nil {
			return err
		}
		var pending []string
		for rows.Next() {
			var token string
			if err := rows.Scan(&token); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, token)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, token := range pending {
			if _, err := r.db.Exec(ctx, `update `+table+` set token = $2 where token = $1`, token, hashToken(token)); err != nil {
				return err
			}
		}
	}
	return nil
}
