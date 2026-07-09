package keyvault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const vaultScope = "https://vault.azure.net/.default"

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
}

type SecretItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	VaultName   string     `json:"vaultName"`
	VaultURL    string     `json:"vaultUrl"`
	ContentType string     `json:"contentType"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Enabled     bool       `json:"enabled"`
	Version     string     `json:"version"`
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

	endpoint := reference + "?api-version=7.4"
	payload, err := callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
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

	endpoint := reference + "?api-version=7.4"
	payload, err := callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
	if err != nil {
		return SecretItem{}, err
	}

	source := sourceForReference(reference, sources)
	return SecretItem{
		ID:          reference,
		Name:        payload.Name(),
		VaultName:   sourceLabel(source),
		VaultURL:    source.VaultURL,
		ContentType: payload.ContentType,
		ExpiresAt:   unixTimePtr(payload.Attributes.ExpiresUnix),
		Enabled:     payload.Attributes.Enabled,
		Version:     secretVersionFromID(payload.ID),
	}, nil
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
	endpoint := strings.TrimRight(source.VaultURL, "/") + "/secrets?api-version=7.4"
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
				ExpiresAt:   unixTimePtr(value.Attributes.ExpiresUnix),
				Enabled:     value.Attributes.Enabled,
				Version:     secretVersionFromID(value.ID),
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

func callJSON[T any](ctx context.Context, client *http.Client, method string, endpoint string, accessToken string, body io.Reader) (T, error) {
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
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

type vaultSecretsListResponse struct {
	Value []struct {
		ID          string `json:"id"`
		ContentType string `json:"contentType"`
		Attributes  struct {
			Enabled     bool   `json:"enabled"`
			ExpiresUnix *int64 `json:"exp"`
		} `json:"attributes"`
	} `json:"value"`
	NextLink string `json:"nextLink"`
}

type vaultSecretResponse struct {
	ID          string `json:"id"`
	Value       string `json:"value"`
	ContentType string `json:"contentType"`
	Attributes  struct {
		Enabled     bool   `json:"enabled"`
		ExpiresUnix *int64 `json:"exp"`
	} `json:"attributes"`
}

func (r vaultSecretResponse) Name() string {
	if r.ID == "" {
		return ""
	}
	return secretNameFromID(r.ID)
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

func unixTimePtr(value *int64) *time.Time {
	if value == nil || *value <= 0 {
		return nil
	}
	parsed := time.Unix(*value, 0).UTC()
	return &parsed
}
