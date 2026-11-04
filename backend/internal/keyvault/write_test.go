package keyvault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeVault is a minimal Key Vault data plane: just enough of get, set,
// update and list-versions to exercise the write paths.
type fakeVault struct {
	server   *httptest.Server
	existing map[string]int // name -> HTTP status a `get` answers with
	tags     map[string]map[string]string
	versions string // raw JSON for GET /secrets/{name}/versions
	requests []fakeRequest
}

type fakeRequest struct {
	Method      string
	Path        string
	ContentType string
	Body        map[string]any
}

func newFakeVault(t *testing.T) *fakeVault {
	t.Helper()
	vault := &fakeVault{existing: map[string]int{}, tags: map[string]map[string]string{}}
	vault.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fakeRequest{Method: r.Method, Path: r.URL.Path, ContentType: r.Header.Get("Content-Type")}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &request.Body)
		}
		vault.requests = append(vault.requests, request)
		w.Header().Set("Content-Type", "application/json")

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		name := parts[1]
		id := vault.server.URL + "/secrets/" + name + "/v-new"
		switch {
		case r.Method == http.MethodGet && len(parts) == 3 && parts[2] == "versions":
			fmt.Fprint(w, vault.versions)
		case r.Method == http.MethodGet:
			status, ok := vault.existing[name]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":{"code":"SecretNotFound"}}`)
				return
			}
			if status != http.StatusOK {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"code":"Forbidden","message":"Operation get is not allowed on a disabled secret."}}`)
				return
			}
			tags, _ := json.Marshal(vault.tags[name])
			fmt.Fprintf(w, `{"id":%q,"value":"old","contentType":"text/plain","attributes":{"enabled":true},"tags":%s}`, vault.server.URL+"/secrets/"+name+"/v-old", tags)
		case r.Method == http.MethodPut, r.Method == http.MethodPatch:
			body, _ := json.Marshal(map[string]any{
				"id":          id,
				"contentType": request.Body["contentType"],
				"attributes":  request.Body["attributes"],
				"tags":        request.Body["tags"],
			})
			w.Write(body)
		}
	}))
	t.Cleanup(vault.server.Close)
	return vault
}

func (v *fakeVault) service(writeEnabled bool) *Service {
	return NewService(SettingsProvider{
		Runtime: func(context.Context) (RuntimeConfig, error) { return RuntimeConfig{}, nil },
		Sources: func(context.Context) ([]Source, error) {
			return []Source{{Name: "ops", VaultURL: v.server.URL, WriteEnabled: writeEnabled}}, nil
		},
		ChainToken: func(context.Context, string) (string, error) { return "token", nil },
	})
}

func (v *fakeVault) writes() []fakeRequest {
	out := []fakeRequest{}
	for _, request := range v.requests {
		if request.Method == http.MethodPut || request.Method == http.MethodPatch {
			out = append(out, request)
		}
	}
	return out
}

func TestCreateSecretRefusesVaultWithoutWriteSwitch(t *testing.T) {
	vault := newFakeVault(t)
	_, err := vault.service(false).CreateSecret(context.Background(), vault.server.URL, "api-key", SecretWrite{Value: "v", Attributes: SecretAttributes{Enabled: true}})
	if !errors.Is(err, ErrSourceNotWritable) {
		t.Fatalf("expected ErrSourceNotWritable, got %v", err)
	}
	if len(vault.requests) != 0 {
		t.Fatalf("no request may reach a read-only vault, got %d", len(vault.requests))
	}
}

func TestCreateSecretRefusesExistingNameIncludingDisabled(t *testing.T) {
	vault := newFakeVault(t)
	vault.existing["taken"] = http.StatusOK
	vault.existing["disabled"] = http.StatusForbidden
	service := vault.service(true)

	for _, name := range []string{"taken", "disabled"} {
		_, err := service.CreateSecret(context.Background(), vault.server.URL, name, SecretWrite{Value: "v", Attributes: SecretAttributes{Enabled: true}})
		if !errors.Is(err, ErrSecretExists) {
			t.Fatalf("%s: expected ErrSecretExists, got %v", name, err)
		}
	}
	if len(vault.writes()) != 0 {
		t.Fatalf("create must never PUT over an existing name")
	}
}

func TestCreateSecretValidatesBeforeCallingAzure(t *testing.T) {
	vault := newFakeVault(t)
	service := vault.service(true)
	value := strings.Repeat("x", MaxSecretValueBytes+1)

	cases := map[string]struct {
		name  string
		input SecretWrite
	}{
		"bad name":   {name: "has space", input: SecretWrite{Value: "v"}},
		"empty":      {name: "ok", input: SecretWrite{}},
		"too large":  {name: "ok", input: SecretWrite{Value: value}},
		"encoding":   {name: "ok", input: SecretWrite{Value: "v", FileEncoding: "hex"}},
		"bad window": {name: "ok", input: SecretWrite{Value: "v", Attributes: SecretAttributes{NotBefore: timePtr(2027, 2), ExpiresAt: timePtr(2027, 1)}}},
	}
	for label, item := range cases {
		_, err := service.CreateSecret(context.Background(), vault.server.URL, item.name, item.input)
		if !errors.Is(err, ErrInvalidSecret) {
			t.Fatalf("%s: expected ErrInvalidSecret, got %v", label, err)
		}
		if strings.Contains(err.Error(), "xxxx") {
			t.Fatalf("%s: an error must never echo the value", label)
		}
	}
	if len(vault.requests) != 0 {
		t.Fatalf("validation failures must not reach Azure, got %d requests", len(vault.requests))
	}
}

func TestCreateSecretWritesJSONWithAttributesAndFileTag(t *testing.T) {
	vault := newFakeVault(t)
	notBefore := timePtr(2027, 1)
	expires := timePtr(2027, 6)
	item, err := vault.service(true).CreateSecret(context.Background(), vault.server.URL+"/", "erp-signing", SecretWrite{
		Value:        "TUlJ",
		ContentType:  "application/x-pkcs12",
		FileEncoding: "base64",
		Attributes:   SecretAttributes{Enabled: true, NotBefore: notBefore, ExpiresAt: expires},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	writes := vault.writes()
	if len(writes) != 1 || writes[0].Method != http.MethodPut || writes[0].Path != "/secrets/erp-signing" {
		t.Fatalf("expected one PUT /secrets/erp-signing, got %+v", writes)
	}
	put := writes[0]
	if put.ContentType != "application/json" {
		t.Fatalf("vault writes must be JSON, got %q", put.ContentType)
	}
	if put.Body["value"] != "TUlJ" || put.Body["contentType"] != "application/x-pkcs12" {
		t.Fatalf("unexpected body %+v", put.Body)
	}
	attributes := put.Body["attributes"].(map[string]any)
	if attributes["nbf"] != float64(notBefore.Unix()) || attributes["exp"] != float64(expires.Unix()) || attributes["enabled"] != true {
		t.Fatalf("unexpected attributes %+v", attributes)
	}
	if put.Body["tags"].(map[string]any)[FileEncodingTag] != "base64" {
		t.Fatalf("expected the az CLI file-encoding tag, got %+v", put.Body["tags"])
	}

	if item.ID != vault.server.URL+"/secrets/erp-signing" {
		t.Fatalf("the catalog reference must be versionless, got %q", item.ID)
	}
	if item.Version != "v-new" || item.Name != "erp-signing" || !item.Enabled {
		t.Fatalf("unexpected item %+v", item)
	}
	if item.NotBefore == nil || !item.NotBefore.Equal(*notBefore) {
		t.Fatalf("expected notBefore from the response, got %v", item.NotBefore)
	}
}

func TestSetSecretValueCarriesTagsButNotFileEncoding(t *testing.T) {
	vault := newFakeVault(t)
	vault.existing["api-key"] = http.StatusOK
	vault.tags["api-key"] = map[string]string{"team": "ops", FileEncodingTag: "base64"}

	_, err := vault.service(true).SetSecretValue(context.Background(), vault.server.URL+"/secrets/api-key", SecretWrite{
		Value:      "typed",
		Attributes: SecretAttributes{Enabled: true},
	})
	if err != nil {
		t.Fatalf("set value: %v", err)
	}
	put := vault.writes()[0]
	tags := put.Body["tags"].(map[string]any)
	if tags["team"] != "ops" {
		t.Fatalf("tags of the current version must carry over, got %+v", tags)
	}
	if _, ok := tags[FileEncodingTag]; ok {
		t.Fatalf("a typed value must not inherit the previous upload's file-encoding tag")
	}
}

func TestSetSecretValueRefusesPinnedReference(t *testing.T) {
	vault := newFakeVault(t)
	_, err := vault.service(true).SetSecretValue(context.Background(), vault.server.URL+"/secrets/api-key/abc123", SecretWrite{Value: "v"})
	if !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("expected a pinned reference to be refused, got %v", err)
	}
}

func (v *fakeVault) twoVersions(name string) string {
	base := v.server.URL + "/secrets/" + name
	v.versions = fmt.Sprintf(`{"value":[
		{"id":"%[1]s/old","attributes":{"enabled":false,"created":100}},
		{"id":"%[1]s/new","attributes":{"enabled":true,"created":200,"exp":1900000000}}
	]}`, base)
	return base
}

func TestUpdateSecretAttributesPatchesCurrentVersionAndClearsDates(t *testing.T) {
	vault := newFakeVault(t)
	base := vault.twoVersions("api-key")
	before, item, err := vault.service(true).UpdateSecretAttributes(context.Background(), base, "", SecretAttributes{Enabled: false})
	if err != nil {
		t.Fatalf("update attributes: %v", err)
	}
	patch := vault.writes()[0]
	if patch.Method != http.MethodPatch || patch.Path != "/secrets/api-key/new" {
		t.Fatalf("expected PATCH on the current version by id, got %s %s", patch.Method, patch.Path)
	}
	attributes := patch.Body["attributes"].(map[string]any)
	for _, key := range []string{"nbf", "exp"} {
		value, present := attributes[key]
		if !present || value != nil {
			t.Fatalf("a cleared %s must be sent as explicit null, got %+v", key, attributes)
		}
	}
	if item.Enabled {
		t.Fatalf("expected the response's enabled=false in the item")
	}
	if before.Version != "new" || !before.Enabled || before.ExpiresAt == nil || before.ExpiresAt.Unix() != 1900000000 {
		t.Fatalf("expected the previous state of the current version for the audit trail, got %+v", before)
	}
}

func TestUpdateSecretAttributesTargetsAnOlderVersion(t *testing.T) {
	vault := newFakeVault(t)
	base := vault.twoVersions("api-key")
	before, _, err := vault.service(true).UpdateSecretAttributes(context.Background(), base, "old", SecretAttributes{Enabled: true})
	if err != nil {
		t.Fatalf("update attributes: %v", err)
	}
	if patch := vault.writes()[0]; patch.Path != "/secrets/api-key/old" {
		t.Fatalf("expected PATCH on the older version, got %s", patch.Path)
	}
	if before.Version != "old" || before.Enabled {
		t.Fatalf("expected the older version's previous state, got %+v", before)
	}
}

func TestVersionIdsThatCouldChangeThePathAreRefused(t *testing.T) {
	vault := newFakeVault(t)
	service := vault.service(true)
	base := vault.server.URL + "/secrets/api-key"
	for _, version := range []string{"../other", "a/b", "x?y=1"} {
		if _, _, err := service.UpdateSecretAttributes(context.Background(), base, version, SecretAttributes{Enabled: true}); !errors.Is(err, ErrInvalidSecret) {
			t.Fatalf("update %q: expected ErrInvalidSecret, got %v", version, err)
		}
		if _, err := service.RevealSecretVersion(context.Background(), base, version); !errors.Is(err, ErrInvalidSecret) {
			t.Fatalf("reveal %q: expected ErrInvalidSecret, got %v", version, err)
		}
	}
	if len(vault.requests) != 0 {
		t.Fatalf("a refused version id must not reach Azure, got %d requests", len(vault.requests))
	}
}

func TestListSecretVersionsNewestFirstWithoutWriteSwitch(t *testing.T) {
	vault := newFakeVault(t)
	base := vault.twoVersions("api-key")
	versions, err := vault.service(false).ListSecretVersions(context.Background(), base+"/old")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Version != "new" || !versions[0].Current || versions[1].Version != "old" || versions[1].Current {
		t.Fatalf("expected newest first with the current flag on it, got %+v", versions)
	}
	if versions[1].Enabled || versions[0].ExpiresAt == nil || versions[0].CreatedAt == nil {
		t.Fatalf("expected attributes per version, got %+v", versions)
	}
	if vault.requests[0].Path != "/secrets/api-key/versions" {
		t.Fatalf("a pinned reference must still list the whole secret, got %s", vault.requests[0].Path)
	}
}

func TestRevealSecretVersionReadsThatVersion(t *testing.T) {
	vault := newFakeVault(t)
	vault.existing["api-key"] = http.StatusOK
	value, err := vault.service(false).RevealSecretVersion(context.Background(), vault.server.URL+"/secrets/api-key", "old")
	if err != nil || value != "old" {
		t.Fatalf("reveal version: %q %v", value, err)
	}
	if path := vault.requests[0].Path; path != "/secrets/api-key/old" {
		t.Fatalf("expected GET on the version, got %s", path)
	}
}

func TestCurrentSecretMetadataFallsBackToVersionsForDisabledSecret(t *testing.T) {
	vault := newFakeVault(t)
	vault.existing["api-key"] = http.StatusForbidden
	base := vault.server.URL + "/secrets/api-key"
	vault.versions = fmt.Sprintf(`{"value":[
		{"id":"%[1]s/old","attributes":{"enabled":true,"created":100}},
		{"id":"%[1]s/new","attributes":{"enabled":false,"created":200,"nbf":1800000000,"exp":1900000000}}
	]}`, base)

	item, err := vault.service(false).CurrentSecretMetadata(context.Background(), base)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if item.Version != "new" || item.Enabled {
		t.Fatalf("expected the newest version, disabled, got %+v", item)
	}
	if item.NotBefore == nil || item.NotBefore.Unix() != 1800000000 || item.ExpiresAt == nil || item.ExpiresAt.Unix() != 1900000000 {
		t.Fatalf("expected the active window from the versions list, got %+v", item)
	}
}

func timePtr(year int, month time.Month) *time.Time {
	value := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	return &value
}
