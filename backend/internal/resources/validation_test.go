package resources

import (
	"errors"
	"testing"
)

func TestValidateInputRejectsMissingConnectionHost(t *testing.T) {
	input := CreateResourceInput{
		Name:            "SSH Bastion",
		Type:            TypeSSH,
		Owner:           "Platform",
		AllowedGroups:   []string{"platform"},
		SecretMode:      SecretModeExternal,
		SecretReference: "secret://ssh/bastion",
	}

	err := validateInput(normalizeInput(input))
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateInputAcceptsKeyVaultRecord(t *testing.T) {
	input := CreateResourceInput{
		Name:            "Payroll secret",
		Type:            TypeKeyVaultSecret,
		Owner:           "HR Systems",
		AllowedGroups:   []string{"ops-admins"},
		VaultName:       "payroll-vault",
		ObjectName:      "payroll-api-password",
		ObjectType:      "secret",
		SecretMode:      SecretModeExternal,
		SecretReference: "azure-key-vault://payroll/payroll-api-password",
	}

	if err := validateInput(normalizeInput(input)); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}

func TestValidateInputAcceptsEmptyAllowedGroups(t *testing.T) {
	input := CreateResourceInput{
		Name:            "Shared runbook",
		Type:            TypeWebPortal,
		Owner:           "Operations",
		TargetURL:       "https://runbooks.example.test",
		SecretMode:      SecretModeExternal,
		SecretReference: "secret://runbooks/shared",
	}

	if err := validateInput(normalizeInput(input)); err != nil {
		t.Fatalf("expected empty allowed groups to be valid, got %v", err)
	}
}

func TestValidateInputRejectsInvalidPortalURL(t *testing.T) {
	input := CreateResourceInput{
		Name:        "Shared portal login",
		Type:        TypeWebPortal,
		Owner:       "Operations",
		TargetURL:   "not a url",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	}

	err := validateInput(normalizeInput(input))
	if err == nil {
		t.Fatalf("expected invalid portal url to fail validation")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateInputAcceptsSharedPasswordWithoutTargetSystem(t *testing.T) {
	input := CreateResourceInput{
		Name:        "Shared SQL Login",
		Type:        TypeSharedSecret,
		Owner:       "Operations",
		Username:    "svc-sql",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	}

	if err := validateInput(normalizeInput(input)); err != nil {
		t.Fatalf("expected shared password without target system to be valid, got %v", err)
	}
}

func TestValidateInputAcceptsPasswordlessWebPortal(t *testing.T) {
	input := CreateResourceInput{
		Name:          "Claude shared account",
		Type:          TypeWebPortal,
		Owner:         "Operations",
		TargetURL:     "https://claude.ai",
		Username:      "info@insio.cz",
		RevealAllowed: true,
		SecretMode:    SecretModeNone,
	}

	normalized := normalizeInput(input)
	if err := validateInput(normalized); err != nil {
		t.Fatalf("expected passwordless portal to be valid, got %v", err)
	}
	if normalized.RevealAllowed {
		t.Fatalf("expected reveal flag to be cleared for passwordless portals")
	}
}

func TestValidateInputRejectsPasswordlessForSavedPassword(t *testing.T) {
	input := CreateResourceInput{
		Name:       "Shared SQL Login",
		Type:       TypeSharedSecret,
		Owner:      "Operations",
		Username:   "svc-sql",
		SecretMode: SecretModeNone,
	}

	err := validateInput(normalizeInput(input))
	if err == nil {
		t.Fatalf("expected passwordless saved password to fail validation")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateInputRejectsPasswordlessWithSecretValue(t *testing.T) {
	input := CreateResourceInput{
		Name:        "Claude shared account",
		Type:        TypeWebPortal,
		Owner:       "Operations",
		TargetURL:   "https://claude.ai",
		SecretMode:  SecretModeNone,
		SecretValue: "topsecret",
	}

	err := validateInput(normalizeInput(input))
	if err == nil {
		t.Fatalf("expected passwordless entry with secret value to fail validation")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestNormalizeInputDefaultsSourceKindByType(t *testing.T) {
	input := normalizeInput(CreateResourceInput{
		Name:          "Grafana app",
		Type:          TypeAppRegistration,
		Owner:         "Identity",
		AllowedGroups: []string{"platform"},
		Provider:      "entra",
		ApplicationID: "grafana-app",
		SecretMode:    SecretModeExternal,
	})

	if input.SourceKind != SourceKindEntraAppRegistration {
		t.Fatalf("expected app registrations to default to Entra source, got %q", input.SourceKind)
	}
}

func TestValidateInputAcceptsPromptOnLaunchForConnections(t *testing.T) {
	input := CreateResourceInput{
		Name:       "Alpha SSH",
		Type:       TypeSSH,
		Owner:      "Operations",
		TargetHost: "alpha.internal",
		SecretMode: SecretModePrompt,
	}

	if err := validateInput(normalizeInput(input)); err != nil {
		t.Fatalf("expected prompt-on-launch connection to be valid, got %v", err)
	}
}

func TestValidateInputRejectsFolderPathDeeperThanTwoLevels(t *testing.T) {
	input := CreateResourceInput{
		Name:       "Deep SSH",
		Type:       TypeSSH,
		Owner:      "Operations",
		TargetHost: "deep.internal",
		FolderPath: "workspace/infra/linux",
		SecretMode: SecretModePrompt,
	}

	err := validateInput(normalizeInput(input))
	if err == nil {
		t.Fatalf("expected deep folder path to fail validation")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}
