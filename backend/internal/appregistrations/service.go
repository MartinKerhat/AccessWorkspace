package appregistrations

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

const (
	graphScope   = "https://graph.microsoft.com/.default"
	graphBaseURL = "https://graph.microsoft.com/v1.0"
)

type RuntimeConfig struct {
	Authority    string
	TenantID     string
	ClientID     string
	ClientSecret string
	Configured   bool
}

type OwnerItem struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	DisplayName       string `json:"displayName"`
	Email             string `json:"email"`
	UserPrincipalName string `json:"userPrincipalName"`
}

type CredentialItem struct {
	KeyID         string     `json:"keyId"`
	DisplayName   string     `json:"displayName"`
	Type          string     `json:"type"`
	StartDateTime *time.Time `json:"startDateTime,omitempty"`
	EndDateTime   *time.Time `json:"endDateTime,omitempty"`
	Hint          string     `json:"hint"`
	Usage         string     `json:"usage"`
}

type ApplicationItem struct {
	ID              string           `json:"id"`
	AppID           string           `json:"appId"`
	DisplayName     string           `json:"displayName"`
	SignInAudience  string           `json:"signInAudience"`
	PublisherDomain string           `json:"publisherDomain"`
	Owners          []OwnerItem      `json:"owners"`
	OwnerError      string           `json:"ownerError,omitempty"`
	Credentials     []CredentialItem `json:"credentials"`
}

type DiscoverResult struct {
	Items []ApplicationItem `json:"items"`
}

// TokenSource returns a bearer token for one scope. Used when no client
// secret is configured in admin settings: the deployment's Azure identity
// (workload identity, managed identity, or env credentials) takes over.
type TokenSource func(ctx context.Context, scope string) (string, error)

type SettingsProvider struct {
	Runtime func(context.Context) (RuntimeConfig, error)
	// ChainToken is the fallback identity. An explicitly configured client
	// secret always wins; removing it hands Graph access to the ambient
	// identity.
	ChainToken TokenSource
}

type Service struct {
	provider     SettingsProvider
	client       *http.Client
	graphBaseURL string
}

type RequestError struct {
	StatusCode int
	Body       string
}

func (e RequestError) Error() string {
	return e.Message()
}

func (e RequestError) Message() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("Microsoft Graph request failed with status %d", e.StatusCode)
	}

	var payload graphErrorResponse
	if err := json.Unmarshal([]byte(body), &payload); err == nil {
		if payload.Error.Message != "" {
			if payload.Error.Code != "" {
				return fmt.Sprintf("Microsoft Graph %d %s: %s", e.StatusCode, payload.Error.Code, payload.Error.Message)
			}
			return fmt.Sprintf("Microsoft Graph %d: %s", e.StatusCode, payload.Error.Message)
		}
	}

	return fmt.Sprintf("Microsoft Graph request failed with status %d: %s", e.StatusCode, body)
}

func IsNotFound(err error) bool {
	var requestErr RequestError
	return errors.As(err, &requestErr) && requestErr.StatusCode == http.StatusNotFound
}

func NewService(provider SettingsProvider) *Service {
	return &Service{
		provider:     provider,
		client:       http.DefaultClient,
		graphBaseURL: graphBaseURL,
	}
}

func (s *Service) Discover(ctx context.Context) (DiscoverResult, error) {
	runtime, err := s.loadRuntime(ctx)
	if err != nil {
		return DiscoverResult{}, err
	}
	if !runtime.Configured && s.provider.ChainToken == nil {
		return DiscoverResult{Items: []ApplicationItem{}}, nil
	}

	accessToken, err := s.accessToken(ctx, runtime, graphScope)
	if err != nil {
		return DiscoverResult{}, err
	}

	items, err := s.listApplications(ctx, accessToken)
	if err != nil {
		return DiscoverResult{}, err
	}
	return DiscoverResult{Items: items}, nil
}

func (s *Service) CurrentApplication(ctx context.Context, identifier string) (ApplicationItem, error) {
	runtime, err := s.loadRuntime(ctx)
	if err != nil {
		return ApplicationItem{}, err
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ApplicationItem{}, fmt.Errorf("application identifier is required")
	}

	accessToken, err := s.accessToken(ctx, runtime, graphScope)
	if err != nil {
		return ApplicationItem{}, err
	}

	item, err := s.getApplication(ctx, accessToken, identifier, false)
	if err != nil {
		var requestErr RequestError
		if errors.As(err, &requestErr) && (requestErr.StatusCode == http.StatusNotFound || requestErr.StatusCode == http.StatusBadRequest) {
			item, err = s.getApplication(ctx, accessToken, identifier, true)
		}
	}
	return item, err
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
	return "", fmt.Errorf("app registration access is not configured: set app registration credentials in admin settings or provide an Azure identity (workload identity, managed identity, or env credentials)")
}

func (s *Service) loadRuntime(ctx context.Context) (RuntimeConfig, error) {
	if s.provider.Runtime == nil {
		return RuntimeConfig{}, nil
	}
	runtime, err := s.provider.Runtime(ctx)
	if err != nil {
		return RuntimeConfig{}, err
	}
	runtime.Authority = strings.TrimRight(strings.TrimSpace(runtime.Authority), "/")
	runtime.TenantID = strings.TrimSpace(runtime.TenantID)
	runtime.ClientID = strings.TrimSpace(runtime.ClientID)
	runtime.ClientSecret = strings.TrimSpace(runtime.ClientSecret)
	runtime.Configured = runtime.Configured && runtime.Authority != "" && runtime.TenantID != "" && runtime.ClientID != "" && runtime.ClientSecret != ""
	return runtime, nil
}

func (s *Service) listApplications(ctx context.Context, accessToken string) ([]ApplicationItem, error) {
	query := url.Values{}
	query.Set("$select", "id,appId,displayName,signInAudience,publisherDomain,passwordCredentials,keyCredentials")
	query.Set("$top", "999")
	endpoint := strings.TrimRight(s.graphBaseURL, "/") + "/applications?" + query.Encode()
	items := []ApplicationItem{}

	for endpoint != "" {
		payload, err := callJSON[applicationsListResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
		if err != nil {
			return nil, err
		}
		for _, value := range payload.Value {
			item := value.toApplicationItem()
			owners, err := s.listOwners(ctx, accessToken, item.ID)
			if err != nil {
				item.OwnerError = err.Error()
			} else {
				item.Owners = owners
			}
			items = append(items, item)
		}
		endpoint = payload.NextLink
	}
	return items, nil
}

func (s *Service) getApplication(ctx context.Context, accessToken string, identifier string, useAppID bool) (ApplicationItem, error) {
	query := url.Values{}
	query.Set("$select", "id,appId,displayName,signInAudience,publisherDomain,passwordCredentials,keyCredentials")
	base := strings.TrimRight(s.graphBaseURL, "/")

	endpoint := base + "/applications/" + url.PathEscape(identifier) + "?" + query.Encode()
	if useAppID {
		escaped := strings.ReplaceAll(identifier, "'", "''")
		endpoint = base + "/applications(appId='" + escaped + "')?" + query.Encode()
	}

	payload, err := callJSON[graphApplication](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
	if err != nil {
		return ApplicationItem{}, err
	}
	item := payload.toApplicationItem()
	owners, err := s.listOwners(ctx, accessToken, item.ID)
	if err != nil {
		item.OwnerError = err.Error()
	} else {
		item.Owners = owners
	}
	return item, nil
}

func (s *Service) listOwners(ctx context.Context, accessToken string, applicationObjectID string) ([]OwnerItem, error) {
	applicationObjectID = strings.TrimSpace(applicationObjectID)
	if applicationObjectID == "" {
		return []OwnerItem{}, nil
	}
	query := url.Values{}
	query.Set("$select", "id,displayName,userPrincipalName,mail,appId,servicePrincipalNames")
	endpoint := strings.TrimRight(s.graphBaseURL, "/") + "/applications/" + url.PathEscape(applicationObjectID) + "/owners?" + query.Encode()
	owners := []OwnerItem{}

	for endpoint != "" {
		payload, err := callJSON[ownersListResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
		if err != nil {
			return nil, err
		}
		for _, value := range payload.Value {
			owners = append(owners, value.toOwnerItem())
		}
		endpoint = payload.NextLink
	}
	return owners, nil
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

type graphErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type applicationsListResponse struct {
	Value    []graphApplication `json:"value"`
	NextLink string             `json:"@odata.nextLink"`
}

type graphApplication struct {
	ID                  string            `json:"id"`
	AppID               string            `json:"appId"`
	DisplayName         string            `json:"displayName"`
	SignInAudience      string            `json:"signInAudience"`
	PublisherDomain     string            `json:"publisherDomain"`
	PasswordCredentials []graphCredential `json:"passwordCredentials"`
	KeyCredentials      []graphCredential `json:"keyCredentials"`
}

type graphCredential struct {
	KeyID         string     `json:"keyId"`
	DisplayName   string     `json:"displayName"`
	StartDateTime *time.Time `json:"startDateTime"`
	EndDateTime   *time.Time `json:"endDateTime"`
	Hint          string     `json:"hint"`
	Usage         string     `json:"usage"`
}

type ownersListResponse struct {
	Value    []graphOwner `json:"value"`
	NextLink string       `json:"@odata.nextLink"`
}

type graphOwner struct {
	ODataType             string   `json:"@odata.type"`
	ID                    string   `json:"id"`
	DisplayName           string   `json:"displayName"`
	Mail                  string   `json:"mail"`
	UserPrincipalName     string   `json:"userPrincipalName"`
	AppID                 string   `json:"appId"`
	ServicePrincipalNames []string `json:"servicePrincipalNames"`
}

func (a graphApplication) toApplicationItem() ApplicationItem {
	credentials := make([]CredentialItem, 0, len(a.PasswordCredentials)+len(a.KeyCredentials))
	for _, credential := range a.PasswordCredentials {
		credentials = append(credentials, credential.toCredentialItem("client_secret"))
	}
	for _, credential := range a.KeyCredentials {
		credentials = append(credentials, credential.toCredentialItem("certificate"))
	}
	return ApplicationItem{
		ID:              strings.TrimSpace(a.ID),
		AppID:           strings.TrimSpace(a.AppID),
		DisplayName:     strings.TrimSpace(a.DisplayName),
		SignInAudience:  strings.TrimSpace(a.SignInAudience),
		PublisherDomain: strings.TrimSpace(a.PublisherDomain),
		Owners:          []OwnerItem{},
		Credentials:     credentials,
	}
}

func (c graphCredential) toCredentialItem(credentialType string) CredentialItem {
	return CredentialItem{
		KeyID:         strings.TrimSpace(c.KeyID),
		DisplayName:   strings.TrimSpace(c.DisplayName),
		Type:          credentialType,
		StartDateTime: c.StartDateTime,
		EndDateTime:   c.EndDateTime,
		Hint:          strings.TrimSpace(c.Hint),
		Usage:         strings.TrimSpace(c.Usage),
	}
}

func (o graphOwner) toOwnerItem() OwnerItem {
	ownerType := strings.TrimPrefix(strings.TrimSpace(o.ODataType), "#microsoft.graph.")
	email := strings.TrimSpace(o.Mail)
	if email == "" {
		email = strings.TrimSpace(o.UserPrincipalName)
	}
	if email == "" && len(o.ServicePrincipalNames) > 0 {
		email = strings.TrimSpace(o.ServicePrincipalNames[0])
	}
	return OwnerItem{
		ID:                strings.TrimSpace(o.ID),
		Type:              ownerType,
		DisplayName:       strings.TrimSpace(o.DisplayName),
		Email:             email,
		UserPrincipalName: strings.TrimSpace(o.UserPrincipalName),
	}
}
