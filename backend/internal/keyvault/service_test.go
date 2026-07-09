package keyvault

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessTokenPrecedence(t *testing.T) {
	ctx := context.Background()

	// A fake Entra token endpoint proves the client-secret path was taken.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"secret-path-token"}`)
	}))
	defer tokenServer.Close()

	chainCalled := false
	chain := func(ctx context.Context, scope string) (string, error) {
		chainCalled = true
		return "chain-token", nil
	}

	// Configured client secret wins even when a chain is available.
	service := NewService(SettingsProvider{ChainToken: chain})
	configured := RuntimeConfig{
		Authority:    tokenServer.URL,
		TenantID:     "tenant",
		ClientID:     "client",
		ClientSecret: "secret",
		Configured:   true,
	}
	token, err := service.accessToken(ctx, configured, vaultScope)
	if err != nil || token != "secret-path-token" {
		t.Fatalf("expected client-secret path, got token=%q err=%v", token, err)
	}
	if chainCalled {
		t.Fatalf("chain must not be used while a client secret is configured")
	}

	// Without a configured secret the chain takes over.
	token, err = service.accessToken(ctx, RuntimeConfig{}, vaultScope)
	if err != nil || token != "chain-token" {
		t.Fatalf("expected chain fallback, got token=%q err=%v", token, err)
	}
	if !chainCalled {
		t.Fatalf("expected chain to be used when no client secret is configured")
	}

	// Neither available: a descriptive error, not a silent failure.
	bare := NewService(SettingsProvider{})
	if _, err := bare.accessToken(ctx, RuntimeConfig{}, vaultScope); err == nil ||
		!strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not-configured error, got %v", err)
	}
}

func TestIsNotFoundDetectsAzure404(t *testing.T) {
	err := RequestError{StatusCode: http.StatusNotFound, Body: "not found"}
	if !IsNotFound(err) {
		t.Fatalf("expected 404 request error to be treated as not found")
	}
}

func TestIsNotFoundIgnoresOtherErrors(t *testing.T) {
	if IsNotFound(RequestError{StatusCode: http.StatusForbidden, Body: "forbidden"}) {
		t.Fatalf("expected non-404 request error not to be treated as not found")
	}
	if IsNotFound(fmt.Errorf("network failed")) {
		t.Fatalf("expected generic error not to be treated as not found")
	}
}
