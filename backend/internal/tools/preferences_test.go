package tools

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidatePreference(t *testing.T) {
	ok := json.RawMessage(`{"length":12,"symbols":false}`)
	for _, part := range []string{"password", "passphrase", "key", "token", "rsa", "ec", "ssh", "certificate"} {
		if err := ValidatePreference(part, ok); err != nil {
			t.Errorf("%s: unexpected error %v", part, err)
		}
	}
	bad := []struct {
		part     string
		settings string
	}{
		{"nope", `{}`},
		{"password", ``},
		{"password", `[]`},
		{"password", `"x"`},
		{"password", `null`},
		{"password", `{"a":"` + strings.Repeat("x", 5000) + `"}`},
	}
	for _, tc := range bad {
		if err := ValidatePreference(tc.part, json.RawMessage(tc.settings)); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s %q: expected ErrInvalidInput, got %v", tc.part, truncate(tc.settings), err)
		}
	}
}

func truncate(s string) string {
	if len(s) > 20 {
		return s[:20] + "…"
	}
	return s
}
