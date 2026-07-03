package appregistrations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverListsApplicationsCredentialsAndOwners(t *testing.T) {
	var tokenRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tenant/oauth2/v2.0/token":
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("scope") != graphScope {
				t.Fatalf("expected graph scope, got %q", r.Form.Get("scope"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"graph-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1.0/applications":
			if got := r.Header.Get("Authorization"); got != "Bearer graph-token" {
				t.Fatalf("expected bearer token, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"value": [{
					"id": "app-object-1",
					"appId": "client-id-1",
					"displayName": "Billing Worker",
					"signInAudience": "AzureADMyOrg",
					"publisherDomain": "example.internal",
					"passwordCredentials": [{
						"keyId": "secret-key-1",
						"displayName": "prod secret",
						"startDateTime": "2026-01-01T00:00:00Z",
						"endDateTime": "2026-08-01T00:00:00Z",
						"hint": "abc"
					}],
					"keyCredentials": [{
						"keyId": "cert-key-1",
						"displayName": "prod cert",
						"startDateTime": "2026-01-01T00:00:00Z",
						"endDateTime": "2027-01-01T00:00:00Z",
						"usage": "Verify"
					}]
				}]
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1.0/applications/app-object-1/owners":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"value": [{
					"@odata.type": "#microsoft.graph.user",
					"id": "owner-1",
					"displayName": "Alice Admin",
					"mail": "alice@example.internal",
					"userPrincipalName": "alice@example.internal"
				}]
			}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	service := NewService(SettingsProvider{
		Runtime: func(context.Context) (RuntimeConfig, error) {
			return RuntimeConfig{
				Authority:    server.URL,
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
				Configured:   true,
			}, nil
		},
	})
	service.graphBaseURL = server.URL + "/v1.0"

	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected one token request, got %d", tokenRequests)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one application, got %#v", result.Items)
	}
	item := result.Items[0]
	if item.AppID != "client-id-1" || item.DisplayName != "Billing Worker" {
		t.Fatalf("expected application metadata, got %#v", item)
	}
	if len(item.Credentials) != 2 {
		t.Fatalf("expected secret and certificate credentials, got %#v", item.Credentials)
	}
	if item.Credentials[0].Type != "client_secret" || item.Credentials[1].Type != "certificate" {
		t.Fatalf("expected credential types to be normalized, got %#v", item.Credentials)
	}
	if len(item.Owners) != 1 || item.Owners[0].Email != "alice@example.internal" {
		t.Fatalf("expected owner metadata, got %#v", item.Owners)
	}
}

func TestCurrentApplicationFallsBackToAppID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tenant/oauth2/v2.0/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"graph-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1.0/applications/client-id-1":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1.0/applications(appId='client-id-1')"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"app-object-1","appId":"client-id-1","displayName":"Billing Worker"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1.0/applications/app-object-1/owners":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":[]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	service := NewService(SettingsProvider{
		Runtime: func(context.Context) (RuntimeConfig, error) {
			return RuntimeConfig{
				Authority:    server.URL,
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
				Configured:   true,
			}, nil
		},
	})
	service.graphBaseURL = server.URL + "/v1.0"

	item, err := service.CurrentApplication(context.Background(), "client-id-1")
	if err != nil {
		t.Fatalf("expected lookup to succeed, got %v", err)
	}
	if item.ID != "app-object-1" || item.AppID != "client-id-1" {
		t.Fatalf("expected app id fallback result, got %#v", item)
	}
}

func TestDiscoverReturnsEmptyWhenRuntimeIsNotConfigured(t *testing.T) {
	service := NewService(SettingsProvider{
		Runtime: func(context.Context) (RuntimeConfig, error) {
			return RuntimeConfig{}, nil
		},
	})

	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected no applications without runtime config, got %#v", result.Items)
	}
}

func TestRequestErrorMessageExtractsGraphPayload(t *testing.T) {
	err := RequestError{
		StatusCode: http.StatusForbidden,
		Body:       `{"error":{"code":"Authorization_RequestDenied","message":"Insufficient privileges to complete the operation."}}`,
	}

	got := err.Message()
	want := "Microsoft Graph 403 Authorization_RequestDenied: Insufficient privileges to complete the operation."
	if got != want {
		t.Fatalf("expected parsed Graph message %q, got %q", want, got)
	}
}
