package db

import (
	"strings"
	"testing"
)

// Every file in the embedded migrations folder must follow the naming scheme
// and the baseline must be present — caught here, not at deploy time.
func TestEmbeddedMigrationNames(t *testing.T) {
	names, err := MigrationFileNames()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no migrations embedded")
	}
	if err := ValidateMigrationNames(names); err != nil {
		t.Fatal(err)
	}
	if names[0] != BaselineMigration {
		t.Fatalf("baseline must sort first, got %q", names[0])
	}
}

func TestValidateMigrationNamesRules(t *testing.T) {
	ok := []string{BaselineMigration, "202610071510_generator_preferences.sql"}
	if err := ValidateMigrationNames(ok); err != nil {
		t.Fatalf("valid set rejected: %v", err)
	}

	cases := []struct {
		names []string
		want  string
	}{
		{[]string{BaselineMigration, "021_next_thing.sql"}, "legacy NNN_ numbering is closed"},
		{[]string{BaselineMigration, "202610071510_Mixed-Case.sql"}, "name must be"},
		{[]string{BaselineMigration, "202610071510_a.sql", "202610071510_b.sql"}, "share the timestamp"},
		{[]string{"202610071510_generator_preferences.sql"}, "baseline migration"},
	}
	for _, tc := range cases {
		err := ValidateMigrationNames(tc.names)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: expected error containing %q, got %v", tc.names, tc.want, err)
		}
	}
}
