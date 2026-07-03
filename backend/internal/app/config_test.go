package app

import "testing"

func baseValidConfig() Config {
	return Config{
		AppEnv:              "",
		AuthMode:            "dev",
		SecretEncryptionKey: "a-unique-strong-key",
		ArtifactsSource:     "local",
		ArtifactsDir:        "/data/downloads",
	}
}

func TestValidate_OK(t *testing.T) {
	if err := baseValidConfig().Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidate_MissingEncryptionKey(t *testing.T) {
	cfg := baseValidConfig()
	cfg.SecretEncryptionKey = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when RESOURCE_SECRET_KEY is empty")
	}
}

func TestValidate_RejectsLegacyDevKey(t *testing.T) {
	cfg := baseValidConfig()
	cfg.SecretEncryptionKey = legacyDevSecretKey
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when using the legacy shared dev key")
	}
}

func TestValidate_NonProductionEnvsAllowSeedAndReset(t *testing.T) {
	// Anything that is not "production" runs unrestricted.
	for _, env := range []string{"", "development", "staging", "preprod", "ci", "test"} {
		cfg := baseValidConfig()
		cfg.AppEnv = env
		cfg.SeedOnStart = true
		cfg.ResetDBOnStart = true
		if err := cfg.Validate(); err != nil {
			t.Fatalf("APP_ENV=%q should allow seed/reset, got: %v", env, err)
		}
	}
}

func TestValidate_ProductionForbidsSeedAndReset(t *testing.T) {
	// Case-insensitive: "Production" must also trip the guard.
	for _, env := range []string{"production", "Production", "PRODUCTION"} {
		cfg := baseValidConfig()
		cfg.AppEnv = env

		cfg.SeedOnStart = true
		if err := cfg.Validate(); err == nil {
			t.Fatalf("APP_ENV=%q: expected error for SEED_ON_START", env)
		}

		cfg.SeedOnStart = false
		cfg.ResetDBOnStart = true
		if err := cfg.Validate(); err == nil {
			t.Fatalf("APP_ENV=%q: expected error for RESET_DB_ON_START", env)
		}

		cfg.ResetDBOnStart = false
		if err := cfg.Validate(); err != nil {
			t.Fatalf("APP_ENV=%q: expected valid production config, got: %v", env, err)
		}
	}
}

func TestValidate_BootstrapAdminConsistency(t *testing.T) {
	cfg := baseValidConfig()

	cfg.BootstrapAdminUser = "admin"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: username set without password")
	}

	cfg.BootstrapAdminPass = "short"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: password too short")
	}

	cfg.BootstrapAdminPass = "long-enough-password"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with bootstrap admin, got: %v", err)
	}
}

func TestValidate_ArtifactsSource(t *testing.T) {
	cfg := baseValidConfig()
	cfg.ArtifactsSource = "sftp"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown ARTIFACTS_SOURCE")
	}

	cfg = baseValidConfig()
	cfg.ArtifactsSource = "blob"
	cfg.ArtifactsBlobURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: blob source without container URL")
	}
	cfg.ArtifactsBlobURL = "https://acct.blob.core.windows.net/artifacts"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid blob config, got: %v", err)
	}

	cfg = baseValidConfig()
	cfg.ArtifactsSource = "local"
	cfg.ArtifactsDir = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error: local source without dir")
	}
}

func TestValidate_EntraRequiresCredentials(t *testing.T) {
	cfg := baseValidConfig()
	cfg.AuthMode = "entra"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when AUTH_MODE=entra without Entra credentials")
	}

	cfg.EntraTenantID = "tenant"
	cfg.EntraClientID = "client"
	cfg.EntraClientSecret = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid entra config, got: %v", err)
	}
}
