package keyvault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const vaultScope = "https://vault.azure.net/.default"

// apiVersion is the Key Vault data-plane version for every call. 7.4 is GA;
// keeping it in one place makes a move to a newer GA version a one-line change.
const apiVersion = "7.4"

// MaxSecretValueBytes is Key Vault's limit for one secret value (25 KB).
const MaxSecretValueBytes = 25 * 1024

// FileEncodingTag is the tag the az CLI writes on `secret set --file
// --encoding`, so `az keyvault secret download` can turn the stored string
// back into the original bytes. Uploads from the workspace write the same tag.
const FileEncodingTag = "file-encoding"

var (
	// ErrSourceNotWritable: the vault is configured, but an admin has not
	// allowed the workspace to write to it.
	ErrSourceNotWritable = errors.New("this key vault source does not allow writes from the workspace")
	// ErrSecretExists: create refused because the name is taken. A PUT on an
	// existing name would silently add a new version instead.
	ErrSecretExists = errors.New("a secret with this name already exists in the vault")
	// ErrInvalidSecret wraps every validation failure of a write request.
	ErrInvalidSecret = errors.New("invalid secret")
)

var secretNamePattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,127}$`)

type RuntimeConfig struct {
	Authority    string
	TenantID     string
	ClientID     string
	ClientSecret string
	Configured   bool
}

type Source struct {
	Name     string `json:"name"`
	VaultURL string `json:"vaultUrl"`
	// WriteEnabled is the admin's per-vault switch for every write the
	// workspace can make (create, new value, enabled/active window).
	WriteEnabled bool `json:"writeEnabled"`
}

type SecretItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	VaultName   string            `json:"vaultName"`
	VaultURL    string            `json:"vaultUrl"`
	ContentType string            `json:"contentType"`
	NotBefore   *time.Time        `json:"notBefore,omitempty"`
	ExpiresAt   *time.Time        `json:"expiresAt,omitempty"`
	Enabled     bool              `json:"enabled"`
	Version     string            `json:"version"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// SecretAttributes are the Azure attributes the workspace manages. NotBefore
// and ExpiresAt are informational for secrets (Key Vault still returns the
// value outside the window); only Enabled=false blocks reads.
type SecretAttributes struct {
	Enabled   bool
	NotBefore *time.Time
	ExpiresAt *time.Time
}

// SecretWrite is one value written to Azure. The value only ever passes
// through memory on its way to the vault.
type SecretWrite struct {
	Value       string
	ContentType string
	// FileEncoding is "utf-8" or "base64" for a value that came from a file
	// upload, empty for a typed or generated value.
	FileEncoding string
	Attributes   SecretAttributes
}

type DiscoverSourceResult struct {
	Source Source       `json:"source"`
	Items  []SecretItem `json:"items"`
	Error  string       `json:"error,omitempty"`
}

type DiscoverResult struct {
	Sources []DiscoverSourceResult `json:"sources"`
}

// TokenSource returns a bearer token for one scope. Used when no client
// secret is configured in admin settings: the deployment's Azure identity
// (workload identity, managed identity, or env credentials) takes over.
type TokenSource func(ctx context.Context, scope string) (string, error)

type SettingsProvider struct {
	Runtime func(context.Context) (RuntimeConfig, error)
	Sources func(context.Context) ([]Source, error)
	// ChainToken is the fallback identity. Precedence is deliberate: an
	// explicitly configured client secret always wins (no surprise behavior
	// change for existing deployments); removing it from admin settings hands
	// vault access to the ambient identity.
	ChainToken TokenSource
}

type Service struct {
	provider SettingsProvider
	client   *http.Client
}

type RequestError struct {
	StatusCode int
	Body       string
}

func (e RequestError) Error() string {
	return fmt.Sprintf("azure request failed: %s", strings.TrimSpace(e.Body))
}

func IsNotFound(err error) bool {
	var requestErr RequestError
	return errors.As(err, &requestErr) && requestErr.StatusCode == http.StatusNotFound
}

func NewService(provider SettingsProvider) *Service {
	return &Service{
		provider: provider,
		client:   http.DefaultClient,
	}
}

func (s *Service) Discover(ctx context.Context) (DiscoverResult, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return DiscoverResult{}, err
	}

	if len(sources) == 0 || (!runtime.Configured && s.provider.ChainToken == nil) {
		return DiscoverResult{Sources: []DiscoverSourceResult{}}, nil
	}

	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return DiscoverResult{}, err
	}

	result := DiscoverResult{Sources: make([]DiscoverSourceResult, 0, len(sources))}
	for _, source := range sources {
		items, err := s.listSecrets(ctx, accessToken, source)
		entry := DiscoverSourceResult{
			Source: source,
			Items:  []SecretItem{},
		}
		if items != nil {
			entry.Items = items
		}
		if err != nil {
			entry.Error = err.Error()
		}
		result.Sources = append(result.Sources, entry)
	}
	return result, nil
}

func (s *Service) RevealSecret(ctx context.Context, reference string) (string, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return "", err
	}
	reference = strings.TrimRight(strings.TrimSpace(reference), "/")
	if reference == "" {
		return "", fmt.Errorf("secret reference is required")
	}
	if !belongsToConfiguredSource(reference, sources) {
		return "", fmt.Errorf("secret reference does not belong to a configured key vault source")
	}

	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return "", err
	}

	payload, err := callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, withAPIVersion(reference), accessToken, nil)
	if err != nil {
		return "", err
	}
	return payload.Value, nil
}

func (s *Service) CurrentSecretMetadata(ctx context.Context, reference string) (SecretItem, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return SecretItem{}, err
	}
	reference = strings.TrimRight(strings.TrimSpace(reference), "/")
	if reference == "" {
		return SecretItem{}, fmt.Errorf("secret reference is required")
	}
	if !belongsToConfiguredSource(reference, sources) {
		return SecretItem{}, fmt.Errorf("secret reference does not belong to a configured key vault source")
	}

	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return SecretItem{}, err
	}

	source := sourceForReference(reference, sources)
	payload, err := callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, withAPIVersion(reference), accessToken, nil)
	if err != nil {
		// Key Vault refuses `get` on a disabled secret (403), so its attributes
		// have to come from the versions list, which carries no value. Without
		// this a secret disabled in the portal never shows as disabled here.
		if isForbidden(err) {
			return s.metadataFromVersions(ctx, accessToken, reference, source)
		}
		return SecretItem{}, err
	}
	return secretItemFromBundle(reference, payload.secretBundle, source), nil
}

func (s *Service) ValidateReference(ctx context.Context, reference string) error {
	_, sources, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	reference = strings.TrimRight(strings.TrimSpace(reference), "/")
	if reference == "" {
		return fmt.Errorf("secret reference is required")
	}
	if !belongsToConfiguredSource(reference, sources) {
		return fmt.Errorf("secret reference does not belong to a configured key vault source")
	}
	return nil
}

func (s *Service) loadConfig(ctx context.Context) (RuntimeConfig, []Source, error) {
	runtime, err := s.provider.Runtime(ctx)
	if err != nil {
		return RuntimeConfig{}, nil, err
	}
	sources, err := s.provider.Sources(ctx)
	if err != nil {
		return RuntimeConfig{}, nil, err
	}
	return runtime, normalizeSources(sources), nil
}

func (s *Service) listSecrets(ctx context.Context, accessToken string, source Source) ([]SecretItem, error) {
	endpoint := withAPIVersion(strings.TrimRight(source.VaultURL, "/") + "/secrets")
	items := []SecretItem{}

	for endpoint != "" {
		payload, err := callJSON[vaultSecretsListResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
		if err != nil {
			return nil, err
		}
		for _, value := range payload.Value {
			items = append(items, SecretItem{
				ID:          value.ID,
				Name:        secretNameFromID(value.ID),
				VaultName:   sourceLabel(source),
				VaultURL:    source.VaultURL,
				ContentType: value.ContentType,
				NotBefore:   unixTimePtr(value.Attributes.NotBeforeUnix),
				ExpiresAt:   unixTimePtr(value.Attributes.ExpiresUnix),
				Enabled:     value.Attributes.Enabled,
				Version:     secretVersionFromID(value.ID),
				Tags:        value.Tags,
			})
		}
		endpoint = payload.NextLink
	}

	return items, nil
}

// accessToken picks the auth path: an admin-settings client secret wins when
// fully configured; otherwise the deployment's ambient Azure identity chain.
func (s *Service) accessToken(ctx context.Context, runtime RuntimeConfig, scope string) (string, error) {
	if runtime.Configured {
		return s.clientCredentialsToken(ctx, runtime, scope)
	}
	if s.provider.ChainToken != nil {
		return s.provider.ChainToken(ctx, scope)
	}
	return "", fmt.Errorf("key vault access is not configured: set app registration credentials in admin settings or provide an Azure identity (workload identity, managed identity, or env credentials)")
}

func (s *Service) clientCredentialsToken(ctx context.Context, config RuntimeConfig, scope string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(config.Authority, "/"), config.TenantID)
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", scope)

	payload, err := callJSON[tokenResponse](ctx, s.client, http.MethodPost, endpoint, "", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("azure token response did not contain an access token")
	}
	return payload.AccessToken, nil
}

// callJSON sends a form body (the token request) or none (vault reads).
func callJSON[T any](ctx context.Context, client *http.Client, method string, endpoint string, accessToken string, body io.Reader) (T, error) {
	return doJSON[T](ctx, client, method, endpoint, accessToken, body, "application/x-www-form-urlencoded")
}

// sendJSON sends a JSON body (vault writes).
func sendJSON[T any](ctx context.Context, client *http.Client, method string, endpoint string, accessToken string, payload any) (T, error) {
	var zero T
	encoded, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	return doJSON[T](ctx, client, method, endpoint, accessToken, bytes.NewReader(encoded), "application/json")
}

func doJSON[T any](ctx context.Context, client *http.Client, method string, endpoint string, accessToken string, body io.Reader, contentType string) (T, error) {
	var zero T

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	response, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return zero, RequestError{
			StatusCode: response.StatusCode,
			Body:       strings.TrimSpace(string(payload)),
		}
	}

	var payload T
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return zero, err
	}
	return payload, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type vaultAttributes struct {
	Enabled       bool   `json:"enabled"`
	NotBeforeUnix *int64 `json:"nbf"`
	ExpiresUnix   *int64 `json:"exp"`
	CreatedUnix   *int64 `json:"created"`
	UpdatedUnix   *int64 `json:"updated"`
}

// secretBundle is everything Key Vault returns about one secret version
// except the value. Lists, PATCH responses and the versions list are exactly
// this; vaultSecretResponse adds the value for `get` and `set`.
type secretBundle struct {
	ID          string            `json:"id"`
	ContentType string            `json:"contentType"`
	Attributes  vaultAttributes   `json:"attributes"`
	Tags        map[string]string `json:"tags"`
}

type vaultSecretsListResponse struct {
	Value    []secretBundle `json:"value"`
	NextLink string         `json:"nextLink"`
}

type vaultSecretResponse struct {
	secretBundle
	Value string `json:"value"`
}

// vaultSecretWrite is the PUT body (create, or a new version).
type vaultSecretWrite struct {
	Value       string                 `json:"value"`
	ContentType string                 `json:"contentType,omitempty"`
	Attributes  vaultAttributesPayload `json:"attributes"`
	Tags        map[string]string      `json:"tags,omitempty"`
}

// vaultAttributesPayload has no omitempty on the dates on purpose: on PATCH
// an explicit null clears a date instead of leaving it as it was.
type vaultAttributesPayload struct {
	Enabled       bool   `json:"enabled"`
	NotBeforeUnix *int64 `json:"nbf"`
	ExpiresUnix   *int64 `json:"exp"`
}

type vaultSecretPatch struct {
	Attributes vaultAttributesPayload `json:"attributes"`
}

func normalizeSources(values []Source) []Source {
	seen := map[string]struct{}{}
	out := make([]Source, 0, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		value.VaultURL = strings.TrimRight(strings.TrimSpace(value.VaultURL), "/")
		if value.VaultURL == "" {
			continue
		}
		if _, ok := seen[value.VaultURL]; ok {
			continue
		}
		seen[value.VaultURL] = struct{}{}
		out = append(out, value)
	}
	return out
}

func belongsToConfiguredSource(reference string, sources []Source) bool {
	for _, source := range normalizeSources(sources) {
		if strings.HasPrefix(reference, strings.TrimRight(source.VaultURL, "/")+"/") {
			return true
		}
	}
	return false
}

func sourceLabel(source Source) string {
	if strings.TrimSpace(source.Name) != "" {
		return strings.TrimSpace(source.Name)
	}
	parsed, err := url.Parse(source.VaultURL)
	if err != nil {
		return source.VaultURL
	}
	host := parsed.Hostname()
	if host == "" {
		return source.VaultURL
	}
	return strings.TrimSuffix(host, ".vault.azure.net")
}

func sourceForReference(reference string, sources []Source) Source {
	for _, source := range normalizeSources(sources) {
		base := strings.TrimRight(source.VaultURL, "/")
		if strings.HasPrefix(reference, base+"/") || reference == base {
			return source
		}
	}
	return Source{VaultURL: secretVaultURLFromReference(reference)}
}

func secretNameFromID(id string) string {
	parsed, err := url.Parse(id)
	if err != nil {
		return id
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "secrets" {
			return parts[i+1]
		}
	}
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return id
}

func secretVersionFromID(id string) string {
	parsed, err := url.Parse(id)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == "secrets" {
			return parts[i+2]
		}
	}
	return ""
}

func secretVaultURLFromReference(reference string) string {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

func withAPIVersion(endpoint string) string {
	return endpoint + "?api-version=" + apiVersion
}

func isForbidden(err error) bool {
	var requestErr RequestError
	return errors.As(err, &requestErr) && requestErr.StatusCode == http.StatusForbidden
}

func secretItemFromBundle(reference string, bundle secretBundle, source Source) SecretItem {
	name := secretNameFromID(reference)
	if bundle.ID != "" {
		name = secretNameFromID(bundle.ID)
	}
	return SecretItem{
		ID:          reference,
		Name:        name,
		VaultName:   sourceLabel(source),
		VaultURL:    source.VaultURL,
		ContentType: bundle.ContentType,
		NotBefore:   unixTimePtr(bundle.Attributes.NotBeforeUnix),
		ExpiresAt:   unixTimePtr(bundle.Attributes.ExpiresUnix),
		Enabled:     bundle.Attributes.Enabled,
		Version:     secretVersionFromID(bundle.ID),
		Tags:        bundle.Tags,
	}
}

func unixPtr(value *time.Time) *int64 {
	if value == nil || value.IsZero() {
		return nil
	}
	unix := value.UTC().Unix()
	return &unix
}

func attributesPayload(attributes SecretAttributes) vaultAttributesPayload {
	return vaultAttributesPayload{
		Enabled:       attributes.Enabled,
		NotBeforeUnix: unixPtr(attributes.NotBefore),
		ExpiresUnix:   unixPtr(attributes.ExpiresAt),
	}
}

func unixTimePtr(value *int64) *time.Time {
	if value == nil || *value <= 0 {
		return nil
	}
	parsed := time.Unix(*value, 0).UTC()
	return &parsed
}
