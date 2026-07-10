package resources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/keyvault"
)

var ErrForbidden = errors.New("forbidden")
var ErrNotFound = errors.New("not found")
var ErrInvalidInput = errors.New("invalid input")

type ResourceStore interface {
	List(ctx context.Context, filter Filter) ([]ResourceSummary, error)
	Get(ctx context.Context, id string) (Resource, error)
	GetAny(ctx context.Context, id string) (Resource, error)
	ListArchived(ctx context.Context) ([]ArchivedResourceSummary, error)
	ListManagedKeyVault(ctx context.Context) ([]Resource, error)
	ListManagedAppRegistrations(ctx context.Context) ([]Resource, error)
	Create(ctx context.Context, input CreateResourceInput) (Resource, error)
	Update(ctx context.Context, id string, input UpdateResourceInput) (Resource, error)
	Archive(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	GetConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string) (ConnectionCredentialOverride, error)
	UpsertConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string, passwordResourceID string) error
	DeleteConnectionUserPasswordOverride(ctx context.Context, connectionID string, userID string) error
	ReplaceAppRegistrationSnapshot(ctx context.Context, resourceID string, credentials []AppRegistrationCredential, owners []AppRegistrationOwner) error
	ReplaceAppRegistrationNotificationPolicies(ctx context.Context, resourceID string, resourcePolicy *AppRegistrationNotificationPolicy, credentialPolicies []AppRegistrationCredentialPolicyInput) error
}

type AuditLogger interface {
	Log(ctx context.Context, params audit.LogParams) error
}

type KeyVaultSecretResolver interface {
	Discover(ctx context.Context) (keyvault.DiscoverResult, error)
	RevealSecret(ctx context.Context, reference string) (string, error)
	CurrentSecretMetadata(ctx context.Context, reference string) (keyvault.SecretItem, error)
}

type AppRegistrationResolver interface {
	Discover(ctx context.Context) (appregistrations.DiscoverResult, error)
	CurrentApplication(ctx context.Context, identifier string) (appregistrations.ApplicationItem, error)
}

type AppRegistrationNotificationEvaluator interface {
	EvaluateResource(ctx context.Context, resourceID string) error
}

type RDPSigningConfigProvider interface {
	GetRDPSigningRuntime(ctx context.Context) (RDPSigningRuntimeConfig, error)
}

type RDPSigningRuntimeConfig struct {
	Enabled               bool
	CertificateConfigured bool
	Subject               string
	ThumbprintSHA256      string
	PFXBase64             string
	PFXPassword           string
	LeafCertBase64        string
	RootCertBase64        string
}

// VaultPublicKeyResolver looks up a user's personal-vault public key.
// nil result = the user has no vault (personal secrets fall back to the
// shared class until one exists). Implemented by auth.Repository.
type VaultPublicKeyResolver interface {
	VaultPublicKey(ctx context.Context, userID string) ([]byte, error)
}

type Service struct {
	repo             ResourceStore
	audit            AuditLogger
	keyVault         KeyVaultSecretResolver
	appRegistrations AppRegistrationResolver
	notifications    AppRegistrationNotificationEvaluator
	cipher           *SecretCipher
	vaults           VaultPublicKeyResolver
	launchTickets    *launchTicketStore
	rdpSigning       RDPSigningConfigProvider
	syncMu           sync.Mutex
}

func NewService(repo ResourceStore, audit AuditLogger, keyVault KeyVaultSecretResolver, appRegistrations AppRegistrationResolver, notifications AppRegistrationNotificationEvaluator, cipherOrProviders ...any) *Service {
	var selectedCipher *SecretCipher
	var rdpSigning RDPSigningConfigProvider
	var vaults VaultPublicKeyResolver
	for _, item := range cipherOrProviders {
		switch value := item.(type) {
		case *SecretCipher:
			if value != nil {
				selectedCipher = value
			}
		case RDPSigningConfigProvider:
			if value != nil {
				rdpSigning = value
			}
		case VaultPublicKeyResolver:
			if value != nil {
				vaults = value
			}
		}
	}
	return &Service{
		repo:             repo,
		audit:            audit,
		keyVault:         keyVault,
		appRegistrations: appRegistrations,
		notifications:    notifications,
		cipher:           selectedCipher,
		vaults:           vaults,
		launchTickets:    newLaunchTicketStore(),
		rdpSigning:       rdpSigning,
	}
}

func (s *Service) List(ctx context.Context, user auth.User, filter Filter) ([]ResourceSummary, error) {
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	visible := explainVisibleResourcesForUser(user, items)
	out := make([]ResourceSummary, 0, len(visible))
	for _, item := range visible {
		out = append(out, item.ResourceSummary)
	}
	return out, nil
}

func (s *Service) ExplainVisibleResources(ctx context.Context, user auth.User, filter Filter) ([]VisibleResourceSummary, error) {
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return explainVisibleResourcesForUser(user, items), nil
}

func (s *Service) Get(ctx context.Context, user auth.User, id string) (Resource, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return Resource{}, ErrForbidden
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceViewed,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	return resource, nil
}

func (s *Service) ListArchived(ctx context.Context, user auth.User) ([]ArchivedResourceSummary, error) {
	if !user.IsAdmin {
		return nil, ErrForbidden
	}
	return s.repo.ListArchived(ctx)
}

func (s *Service) Create(ctx context.Context, user auth.User, input CreateResourceInput) (Resource, error) {
	if !user.IsAdmin && !canCreatePersonalPassword(user, input) {
		return Resource{}, ErrForbidden
	}
	input = enforcePersonalPasswordOwnership(user, input)
	input = normalizeInput(input)
	if err := s.prepareSecretForStorage(ctx, &input); err != nil {
		return Resource{}, err
	}
	if err := validateInput(input); err != nil {
		return Resource{}, err
	}
	resource, err := s.repo.Create(ctx, input)
	if err != nil {
		return Resource{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceCreated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	if resource.Type == TypeAppRegistration && s.notifications != nil {
		_ = s.notifications.EvaluateResource(ctx, resource.ID)
	}
	return resource, nil
}

func (s *Service) Update(ctx context.Context, user auth.User, id string, input UpdateResourceInput) (Resource, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if !canUpdateResource(user, existing) {
		return Resource{}, ErrForbidden
	}
	input = enforcePersonalPasswordOwnership(user, input)
	input = normalizeInput(input)
	input = preserveManagedFields(existing, input, user)
	input = preserveExistingSecret(existing, input)
	if err := s.prepareSecretForStorage(ctx, &input); err != nil {
		return Resource{}, err
	}
	if err := validateInput(input); err != nil {
		return Resource{}, err
	}
	resource, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return Resource{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	if resource.Type == TypeAppRegistration && s.notifications != nil {
		_ = s.notifications.EvaluateResource(ctx, resource.ID)
	}
	return resource, nil
}

func (s *Service) UpdateAppRegistrationNotificationPolicies(ctx context.Context, user auth.User, id string, input AppRegistrationNotificationPolicyUpdateInput) (Resource, error) {
	if !user.IsAdmin {
		return Resource{}, ErrForbidden
	}
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if resource.Type != TypeAppRegistration {
		return Resource{}, fmt.Errorf("%w: only app registrations support notification policies", ErrInvalidInput)
	}
	resourcePolicy := normalizeOptionalNotificationPolicy(input.ResourcePolicy)
	credentialPolicies := make([]AppRegistrationCredentialPolicyInput, 0, len(input.CredentialPolicies))
	for _, item := range input.CredentialPolicies {
		keyID := strings.TrimSpace(item.KeyID)
		if keyID == "" {
			continue
		}
		credentialPolicies = append(credentialPolicies, AppRegistrationCredentialPolicyInput{
			KeyID:  keyID,
			Policy: normalizeOptionalNotificationPolicy(item.Policy),
		})
	}
	if err := s.repo.ReplaceAppRegistrationNotificationPolicies(ctx, id, resourcePolicy, credentialPolicies); err != nil {
		return Resource{}, err
	}
	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if s.notifications != nil {
		_ = s.notifications.EvaluateResource(ctx, id)
	}
	return updated, nil
}

func (s *Service) Archive(ctx context.Context, user auth.User, id string) error {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !canArchiveResource(user, resource) {
		return ErrForbidden
	}
	if err := s.repo.Archive(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceArchived,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":   resource.Type,
			"reason": "removed_from_app",
		},
	})
	return nil
}

func (s *Service) Restore(ctx context.Context, user auth.User, id string) error {
	if !user.IsAdmin {
		return ErrForbidden
	}
	resource, err := s.repo.GetAny(ctx, id)
	if err != nil {
		return err
	}
	if resource.ArchivedAt == nil {
		return ErrNotFound
	}
	if err := s.repo.Restore(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceRestored,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	return nil
}

func (s *Service) Reveal(ctx context.Context, user auth.User, id string) (RevealResult, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return RevealResult{}, err
	}
	if !(canRevealResource(user, resource) && resource.RevealAllowed) && !canRevealStoredPassword(user, resource) {
		return RevealResult{}, ErrForbidden
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceRevealed,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	secretValue, err := s.resolveRevealValue(ctx, user, resource)
	if err != nil {
		return RevealResult{}, err
	}
	return RevealResult{
		ResourceID:      resource.ID,
		SecretMode:      resource.Secret.Mode,
		SecretValue:     secretValue,
		SecretReference: resource.Secret.Reference,
	}, nil
}

func (s *Service) Launch(ctx context.Context, user auth.User, id string) (LaunchPayload, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return LaunchPayload{}, err
	}
	if !canLaunchResource(user, resource) || !resource.LaunchAllowed {
		return LaunchPayload{}, ErrForbidden
	}
	resource = s.applyConnectionCredentialOverride(ctx, user, resource)
	payload := buildLaunchPayload(resource)
	payload, err = s.buildLauncherPayload(ctx, user, resource, payload)
	if err != nil {
		return LaunchPayload{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceLaunched,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":   resource.Type,
			"method": payload.Method,
		},
	})
	return payload, nil
}

func (s *Service) ListPasswordOptions(ctx context.Context, user auth.User) ([]ResourceSummary, error) {
	items, err := s.repo.List(ctx, Filter{})
	if err != nil {
		return nil, err
	}
	visible := explainVisibleResourcesForUser(user, items)
	options := make([]ResourceSummary, 0, len(visible))
	for _, item := range visible {
		if !isPasswordOverrideCandidateSummary(item.ResourceSummary) {
			continue
		}
		options = append(options, item.ResourceSummary)
	}
	return options, nil
}

func (s *Service) ListPortalCredentialMatches(ctx context.Context, user auth.User, rawURL string) ([]PortalCredentialMatch, error) {
	currentURL, ok := normalizePortalURL(rawURL)
	if !ok {
		return nil, fmt.Errorf("%w: portal url is required", ErrInvalidInput)
	}
	if !auth.CapabilitiesForUser(user).Categories["passwords"].View {
		return nil, ErrForbidden
	}

	items, err := s.repo.List(ctx, Filter{})
	if err != nil {
		return nil, err
	}

	visible := explainVisibleResourcesForUser(user, items)
	matches := make([]PortalCredentialMatch, 0, len(visible))
	for _, item := range visible {
		if item.Type != TypeWebPortal || !item.CopyAllowed {
			continue
		}
		if !portalURLMatches(item.TargetURL, currentURL) {
			continue
		}
		matches = append(matches, PortalCredentialMatch{
			ResourceID:   item.ID,
			ResourceName: item.Name,
			Username:     item.Username,
			TargetURL:    item.TargetURL,
			Personal:     item.Personal,
			Owner:        item.Owner,
		})
	}

	slices.SortFunc(matches, func(a, b PortalCredentialMatch) int {
		aTarget, _ := normalizePortalURL(a.TargetURL)
		bTarget, _ := normalizePortalURL(b.TargetURL)
		if a.Personal != b.Personal {
			if a.Personal {
				return -1
			}
			return 1
		}
		if len(aTarget.Path) != len(bTarget.Path) {
			if len(aTarget.Path) > len(bTarget.Path) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ResourceName, b.ResourceName)
	})

	return matches, nil
}

func (s *Service) FillPortalCredential(ctx context.Context, user auth.User, resourceID string, rawURL string) (PortalCredentialFillResult, error) {
	resource, err := s.repo.Get(ctx, resourceID)
	if err != nil {
		return PortalCredentialFillResult{}, err
	}
	if resource.Type != TypeWebPortal {
		return PortalCredentialFillResult{}, fmt.Errorf("%w: only web portal logins support browser fill", ErrInvalidInput)
	}
	if !canFillPasswordResource(user, resource) {
		return PortalCredentialFillResult{}, ErrForbidden
	}

	currentURL, ok := normalizePortalURL(rawURL)
	if !ok {
		return PortalCredentialFillResult{}, fmt.Errorf("%w: portal url is required", ErrInvalidInput)
	}
	if !portalURLMatches(resource.TargetURL, currentURL) {
		return PortalCredentialFillResult{}, ErrForbidden
	}

	password, err := s.resolveRevealValue(ctx, user, resource)
	if err != nil {
		return PortalCredentialFillResult{}, err
	}

	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceFilled,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":    resource.Type,
			"channel": "browser_extension",
			"url":     currentURL.String(),
		},
	})

	return PortalCredentialFillResult{
		ResourceID:   resource.ID,
		ResourceName: resource.Name,
		Username:     resource.Username,
		Password:     password,
		TargetURL:    resource.TargetURL,
	}, nil
}

func (s *Service) GetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) (ConnectionCredentialOverride, error) {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	override, err := s.repo.GetConnectionUserPasswordOverride(ctx, connectionID, user.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ConnectionCredentialOverride{ConnectionID: connectionID}, nil
		}
		return ConnectionCredentialOverride{}, err
	}
	passwordResource, err := s.repo.Get(ctx, override.PasswordResourceID)
	if err != nil || !canViewResource(user, passwordResource.Summary()) || !isPasswordOverrideCandidate(passwordResource) {
		return ConnectionCredentialOverride{ConnectionID: connectionID}, nil
	}
	override.PasswordResourceName = passwordResource.Name
	override.Username = passwordResource.Username
	override.Personal = passwordResource.Personal
	return override, nil
}

func (s *Service) SetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string, input ConnectionCredentialOverrideInput) (ConnectionCredentialOverride, error) {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	passwordID := strings.TrimSpace(input.PasswordResourceID)
	if passwordID == "" {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: password resource is required", ErrInvalidInput)
	}
	passwordResource, err := s.repo.Get(ctx, passwordID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, passwordResource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if !isPasswordOverrideCandidate(passwordResource) {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: selected password object cannot be used as a connection override", ErrInvalidInput)
	}
	if err := s.repo.UpsertConnectionUserPasswordOverride(ctx, connectionID, user.ID, passwordID); err != nil {
		return ConnectionCredentialOverride{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":                   resource.Type,
			"connectionOverride":     "password_resource",
			"overridePasswordObject": passwordID,
		},
	})
	return s.GetConnectionCredentialOverride(ctx, user, connectionID)
}

func (s *Service) ClearConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) error {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	if !canViewResource(user, resource.Summary()) {
		return ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	if err := s.repo.DeleteConnectionUserPasswordOverride(ctx, connectionID, user.ID); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":               resource.Type,
			"connectionOverride": "cleared",
		},
	})
	return nil
}

func (s *Service) ResolveLaunchTicket(_ context.Context, ticket string) (LaunchPayload, error) {
	if s.launchTickets == nil {
		return LaunchPayload{}, ErrNotFound
	}
	return s.launchTickets.Redeem(ticket)
}

func (s *Service) ImportAppRegistrations(ctx context.Context, user auth.User, input AppRegistrationImportInput) ([]Resource, error) {
	if !user.IsAdmin {
		return nil, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["appregistrations"].Import {
		return nil, ErrForbidden
	}
	if s.appRegistrations == nil {
		return nil, fmt.Errorf("%w: app registration integration is not available", ErrInvalidInput)
	}

	input = normalizeAppRegistrationImportInput(input)
	if input.Owner == "" {
		return nil, fmt.Errorf("%w: owner is required for app registration import", ErrInvalidInput)
	}
	if len(input.ApplicationIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one app registration must be selected", ErrInvalidInput)
	}

	existing, err := s.repo.ListManagedAppRegistrations(ctx)
	if err != nil {
		return nil, err
	}
	existingByExternalID := appRegistrationExistingKeys(existing)

	now := time.Now().UTC()
	imported := make([]Resource, 0, len(input.ApplicationIDs))
	for _, identifier := range input.ApplicationIDs {
		app, err := s.appRegistrations.CurrentApplication(ctx, identifier)
		if err != nil {
			return nil, err
		}
		if app.ID == "" && app.AppID == "" {
			continue
		}
		if appRegistrationAlreadyImported(existingByExternalID, app) {
			continue
		}

		createInput := appRegistrationCreateInput(input, app, now)
		resource, err := s.repo.Create(ctx, createInput)
		if err != nil {
			return nil, err
		}
		credentials, owners := appRegistrationSnapshot(app, resource.ID, now)
		if err := s.repo.ReplaceAppRegistrationSnapshot(ctx, resource.ID, credentials, owners); err != nil {
			return nil, err
		}
		resource.AppCredentials = credentials
		resource.AppOwners = owners
		imported = append(imported, resource)
		existingByExternalID[strings.TrimSpace(app.ID)] = struct{}{}
		existingByExternalID[strings.TrimSpace(app.AppID)] = struct{}{}

		_ = s.audit.Log(ctx, audit.LogParams{
			EventType:    audit.EventResourceCreated,
			UserID:       user.ID,
			UserName:     user.Name,
			ResourceID:   &resource.ID,
			ResourceName: &resource.Name,
			Metadata: map[string]any{
				"type":            resource.Type,
				"reason":          "app_registration_import",
				"credentialCount": len(credentials),
			},
		})
		if s.notifications != nil {
			_ = s.notifications.EvaluateResource(ctx, resource.ID)
		}
	}

	return imported, nil
}

func (s *Service) SyncAppRegistrations(ctx context.Context, user auth.User, automatic bool) (AppRegistrationSyncResult, error) {
	if !user.IsAdmin {
		return AppRegistrationSyncResult{}, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["appregistrations"].View {
		return AppRegistrationSyncResult{}, ErrForbidden
	}
	if s.appRegistrations == nil {
		return AppRegistrationSyncResult{}, fmt.Errorf("%w: app registration integration is not available", ErrInvalidInput)
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	items, err := s.repo.ListManagedAppRegistrations(ctx)
	if err != nil {
		return AppRegistrationSyncResult{}, err
	}

	now := time.Now().UTC()
	result := AppRegistrationSyncResult{Automatic: automatic}
	for _, item := range items {
		identifier := strings.TrimSpace(item.SourceObjectID)
		if identifier == "" {
			identifier = strings.TrimSpace(item.ApplicationID)
		}
		if identifier == "" {
			result.MissingResources++
			continue
		}

		result.AttemptedResources++
		app, err := s.appRegistrations.CurrentApplication(ctx, identifier)
		if err != nil {
			var graphErr appregistrations.RequestError
			if errors.As(err, &graphErr) && (graphErr.StatusCode == http.StatusUnauthorized || graphErr.StatusCode == http.StatusForbidden) {
				return result, err
			}
			if appregistrations.IsNotFound(err) {
				if err := s.repo.Archive(ctx, item.ID); err != nil {
					result.MissingResources++
					continue
				}
				result.RemovedResources++
				_ = s.audit.Log(ctx, audit.LogParams{
					EventType:    audit.EventResourceArchived,
					UserID:       user.ID,
					UserName:     user.Name,
					ResourceID:   &item.ID,
					ResourceName: &item.Name,
					Metadata: map[string]any{
						"type":      item.Type,
						"reason":    "app_registration_not_found",
						"automatic": automatic,
					},
				})
				continue
			}
			result.MissingResources++
			continue
		}

		update := resourceToUpdateInput(item)
		applyAppRegistrationMetadata(&update, app, now)
		if _, err := s.repo.Update(ctx, item.ID, update); err != nil {
			result.MissingResources++
			continue
		}
		credentials, owners := appRegistrationSnapshot(app, item.ID, now)
		if err := s.repo.ReplaceAppRegistrationSnapshot(ctx, item.ID, credentials, owners); err != nil {
			result.MissingResources++
			continue
		}
		if s.notifications != nil {
			_ = s.notifications.EvaluateResource(ctx, item.ID)
		}
		expiring, expired := countCredentialExpiry(credentials, now)
		result.ExpiringCredentials += expiring
		result.ExpiredCredentials += expired
		result.UpdatedResources++
	}

	return result, nil
}

func (s *Service) SyncKeyVault(ctx context.Context, user auth.User, sources []KeyVaultSyncSourceConfig, automatic bool) (KeyVaultSyncResult, error) {
	if !user.IsAdmin {
		return KeyVaultSyncResult{}, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["keyvault"].View {
		return KeyVaultSyncResult{}, ErrForbidden
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	now := time.Now().UTC()
	items, err := s.repo.ListManagedKeyVault(ctx)
	if err != nil {
		return KeyVaultSyncResult{}, err
	}
	existingByReference := map[string]Resource{}
	for _, item := range items {
		reference := strings.TrimRight(strings.TrimSpace(item.Secret.Reference), "/")
		if reference == "" {
			reference = strings.TrimRight(strings.TrimSpace(item.SourceObjectID), "/")
		}
		if reference == "" {
			continue
		}
		existingByReference[reference] = item
	}

	discoveredBySource := map[string]keyvault.DiscoverSourceResult{}
	needsDiscovery := false
	for _, source := range sources {
		if source.AutoImportEnabled {
			needsDiscovery = true
			break
		}
	}
	if needsDiscovery {
		discovered, err := s.keyVault.Discover(ctx)
		if err != nil {
			return KeyVaultSyncResult{}, err
		}
		for _, source := range discovered.Sources {
			discoveredBySource[strings.TrimRight(strings.TrimSpace(source.Source.VaultURL), "/")] = source
		}
	}

	result := KeyVaultSyncResult{
		Automatic: automatic,
		Sources:   make([]KeyVaultSyncSource, 0, len(sources)),
	}

	sourceMap := map[string]KeyVaultSyncSourceConfig{}
	for _, source := range sources {
		sourceMap[source.VaultURL] = source
	}

	for _, source := range sources {
		entry := KeyVaultSyncSource{
			VaultURL:        source.VaultURL,
			Name:            source.Name,
			SyncEnabled:     source.SyncEnabled,
			LastSyncedAt:    source.LastSyncedAt,
			LastSyncStatus:  source.LastSyncStatus,
			LastSyncError:   source.LastSyncError,
			LastSyncSummary: source.LastSyncSummary,
		}
		if automatic && source.SyncEnabled {
			entry.Due = source.LastSyncedAt == nil || source.SyncIntervalMinutes <= 0 || source.LastSyncedAt.Add(time.Duration(source.SyncIntervalMinutes)*time.Minute).Before(now)
		}
		result.Sources = append(result.Sources, entry)
	}

	for i := range result.Sources {
		source := sourceMap[result.Sources[i].VaultURL]
		if automatic && (!source.SyncEnabled || !result.Sources[i].Due) {
			continue
		}
		result.AttemptedSources++
		result.Sources[i].LastSyncedAt = &now
		result.Sources[i].LastSyncStatus = ""
		result.Sources[i].LastSyncError = ""
		result.Sources[i].LastSyncSummary = ""

		if source.AutoImportEnabled {
			if strings.TrimSpace(source.DefaultOwner) == "" {
				markKeyVaultSyncError(&result.Sources[i], "default owner is required for auto import")
			} else if discovered, ok := discoveredBySource[result.Sources[i].VaultURL]; ok {
				if discovered.Error != "" {
					markKeyVaultSyncError(&result.Sources[i], discovered.Error)
				} else {
					for _, secret := range discovered.Items {
						reference := strings.TrimRight(strings.TrimSpace(secret.ID), "/")
						if reference == "" {
							continue
						}
						if _, exists := existingByReference[reference]; exists {
							continue
						}
						created, err := s.repo.Create(ctx, autoImportCreateInput(source, secret, now))
						if err != nil {
							markKeyVaultSyncError(&result.Sources[i], err.Error())
							result.MissingResources++
							result.Sources[i].MissingCount++
							continue
						}
						existingByReference[reference] = created
						result.ImportedResources++
						result.Sources[i].ImportedCount++
						_ = s.audit.Log(ctx, audit.LogParams{
							EventType:    audit.EventResourceCreated,
							UserID:       user.ID,
							UserName:     user.Name,
							ResourceID:   &created.ID,
							ResourceName: &created.Name,
							Metadata: map[string]any{
								"type":      created.Type,
								"reason":    "key_vault_auto_import",
								"automatic": automatic,
							},
						})
					}
				}
			}
		}

		for _, item := range items {
			reference := strings.TrimRight(item.Secret.Reference, "/")
			if reference == "" {
				continue
			}
			if !strings.HasPrefix(reference, strings.TrimRight(source.VaultURL, "/")+"/") {
				continue
			}

			metadata, err := s.keyVault.CurrentSecretMetadata(ctx, reference)
			if err != nil {
				if keyvault.IsNotFound(err) {
					if err := s.repo.Archive(ctx, item.ID); err != nil {
						markKeyVaultSyncError(&result.Sources[i], err.Error())
						result.MissingResources++
						result.Sources[i].MissingCount++
						continue
					}
					result.RemovedResources++
					result.Sources[i].RemovedCount++
					_ = s.audit.Log(ctx, audit.LogParams{
						EventType:    audit.EventResourceArchived,
						UserID:       user.ID,
						UserName:     user.Name,
						ResourceID:   &item.ID,
						ResourceName: &item.Name,
						Metadata: map[string]any{
							"type":      item.Type,
							"reason":    "key_vault_secret_not_found",
							"automatic": automatic,
						},
					})
					continue
				}
				markKeyVaultSyncError(&result.Sources[i], err.Error())
				result.MissingResources++
				result.Sources[i].MissingCount++
				continue
			}

			update := resourceToUpdateInput(item)
			update.LastSyncedAt = &now
			update.VaultName = metadata.VaultName
			update.ObjectName = metadata.Name
			update.ObjectType = "secret"
			update.ObjectVersion = metadata.Version
			update.ContentType = metadata.ContentType
			update.ExpiresAt = metadata.ExpiresAt
			update.Status = keyVaultStatus(metadata.Enabled)
			update.SourceObjectID = strings.TrimRight(item.SourceObjectID, "/")
			update.LinkedSecretRef = strings.TrimRight(item.LinkedSecretRef, "/")
			update.SecretReference = reference

			if _, err := s.repo.Update(ctx, item.ID, update); err != nil {
				markKeyVaultSyncError(&result.Sources[i], err.Error())
				result.MissingResources++
				result.Sources[i].MissingCount++
				continue
			}

			result.UpdatedResources++
			result.Sources[i].UpdatedCount++
		}

		if result.Sources[i].LastSyncStatus == "" {
			result.Sources[i].LastSyncStatus = "ok"
			result.Sources[i].LastSyncError = ""
		}
		result.Sources[i].LastSyncSummary = summarizeKeyVaultSourceRun(result.Sources[i])
	}

	return result, nil
}

func markKeyVaultSyncError(source *KeyVaultSyncSource, message string) {
	source.LastSyncStatus = "error"
	if source.LastSyncError == "" {
		source.LastSyncError = strings.TrimSpace(message)
	}
}

func summarizeKeyVaultSourceRun(source KeyVaultSyncSource) string {
	parts := make([]string, 0, 4)
	if source.ImportedCount > 0 {
		parts = append(parts, fmt.Sprintf("Imported %d new resources", source.ImportedCount))
	}
	parts = append(parts, fmt.Sprintf("Updated %d imported resources", source.UpdatedCount))
	if source.RemovedCount > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d missing resources", source.RemovedCount))
	}
	if source.MissingCount > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", source.MissingCount))
	}
	return strings.Join(parts, ", ")
}

func keyVaultStatus(enabled bool) string {
	if enabled {
		return "active"
	}
	return "disabled"
}

func normalizeAppRegistrationImportInput(input AppRegistrationImportInput) AppRegistrationImportInput {
	input.Owner = strings.TrimSpace(input.Owner)
	input.OwnerTeam = strings.TrimSpace(input.OwnerTeam)
	input.Environment = strings.TrimSpace(input.Environment)
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.Description = strings.TrimSpace(input.Description)
	input.Notes = strings.TrimSpace(input.Notes)
	input.AllowedGroups = normalizeValues(input.AllowedGroups)
	input.ApplicationIDs = normalizeValues(input.ApplicationIDs)
	return input
}

func appRegistrationExistingKeys(items []Resource) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		if value := strings.TrimSpace(item.SourceObjectID); value != "" {
			out[value] = struct{}{}
		}
		if value := strings.TrimSpace(item.ApplicationID); value != "" {
			out[value] = struct{}{}
		}
		if value := strings.TrimSpace(item.ClientID); value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func appRegistrationAlreadyImported(existing map[string]struct{}, app appregistrations.ApplicationItem) bool {
	if _, ok := existing[strings.TrimSpace(app.ID)]; ok && strings.TrimSpace(app.ID) != "" {
		return true
	}
	if _, ok := existing[strings.TrimSpace(app.AppID)]; ok && strings.TrimSpace(app.AppID) != "" {
		return true
	}
	return false
}

func appRegistrationCreateInput(input AppRegistrationImportInput, app appregistrations.ApplicationItem, now time.Time) CreateResourceInput {
	create := CreateResourceInput{
		Name:                appRegistrationDisplayName(app),
		Type:                TypeAppRegistration,
		Description:         input.Description,
		Owner:               input.Owner,
		OwnerTeam:           input.OwnerTeam,
		Environment:         input.Environment,
		TenantID:            input.TenantID,
		SourceKind:          SourceKindEntraAppRegistration,
		SourceObjectID:      strings.TrimSpace(app.ID),
		LastSyncedAt:        &now,
		Notes:               input.Notes,
		Provider:            "entra",
		ApplicationID:       strings.TrimSpace(app.AppID),
		ClientID:            strings.TrimSpace(app.AppID),
		DisplayNameExternal: strings.TrimSpace(app.DisplayName),
		AllowedGroups:       append([]string{}, input.AllowedGroups...),
		SecretMode:          SecretModeExternal,
		SecretReference:     appRegistrationReference(app),
	}
	applyAppRegistrationCredentialSummary(&create.CredentialType, &create.CredentialExpiresAt, app.Credentials, now)
	create.Status = appRegistrationStatus(app.Credentials, now)
	if create.Description == "" {
		create.Description = "Imported from Microsoft Entra app registrations."
	}
	return create
}

func applyAppRegistrationMetadata(input *UpdateResourceInput, app appregistrations.ApplicationItem, now time.Time) {
	input.Name = appRegistrationDisplayName(app)
	input.SourceKind = SourceKindEntraAppRegistration
	input.SourceObjectID = strings.TrimSpace(app.ID)
	input.LastSyncedAt = &now
	input.Provider = "entra"
	input.ApplicationID = strings.TrimSpace(app.AppID)
	input.ClientID = strings.TrimSpace(app.AppID)
	input.DisplayNameExternal = strings.TrimSpace(app.DisplayName)
	input.SecretMode = SecretModeExternal
	input.SecretReference = appRegistrationReference(app)
	input.LaunchAllowed = false
	input.RevealAllowed = false
	input.CopyAllowed = false
	applyAppRegistrationCredentialSummary(&input.CredentialType, &input.CredentialExpiresAt, app.Credentials, now)
	input.Status = appRegistrationStatus(app.Credentials, now)
}

func appRegistrationSnapshot(app appregistrations.ApplicationItem, resourceID string, now time.Time) ([]AppRegistrationCredential, []AppRegistrationOwner) {
	credentials := make([]AppRegistrationCredential, 0, len(app.Credentials))
	for _, item := range app.Credentials {
		keyID := strings.TrimSpace(item.KeyID)
		credentialType := strings.TrimSpace(item.Type)
		if keyID == "" || credentialType == "" {
			continue
		}
		syncedAt := now
		credentials = append(credentials, AppRegistrationCredential{
			ResourceID:     resourceID,
			KeyID:          keyID,
			CredentialType: credentialType,
			DisplayName:    strings.TrimSpace(item.DisplayName),
			StartDateTime:  item.StartDateTime,
			EndDateTime:    item.EndDateTime,
			Hint:           strings.TrimSpace(item.Hint),
			Usage:          strings.TrimSpace(item.Usage),
			LastSyncedAt:   &syncedAt,
		})
	}

	owners := make([]AppRegistrationOwner, 0, len(app.Owners))
	for _, item := range app.Owners {
		ownerID := strings.TrimSpace(item.ID)
		if ownerID == "" {
			continue
		}
		owners = append(owners, AppRegistrationOwner{
			ResourceID:  resourceID,
			OwnerID:     ownerID,
			OwnerType:   strings.TrimSpace(item.Type),
			DisplayName: strings.TrimSpace(item.DisplayName),
			Email:       strings.TrimSpace(item.Email),
		})
	}
	return credentials, owners
}

func applyAppRegistrationCredentialSummary(credentialType *string, credentialExpiresAt **time.Time, credentials []appregistrations.CredentialItem, now time.Time) {
	next := nextAppRegistrationCredential(credentials, now)
	if next == nil {
		*credentialType = "none"
		*credentialExpiresAt = nil
		return
	}
	*credentialType = next.Type
	*credentialExpiresAt = next.EndDateTime
}

func appRegistrationStatus(credentials []appregistrations.CredentialItem, now time.Time) string {
	if len(credentials) == 0 {
		return "no_credentials"
	}
	next := nextFutureCredentialExpiry(credentials, now)
	if next == nil {
		for _, credential := range credentials {
			if credential.EndDateTime == nil {
				return "active"
			}
		}
		return "expired"
	}
	if !next.After(now.Add(30 * 24 * time.Hour)) {
		return "expiring"
	}
	return "active"
}

func nextAppRegistrationCredential(credentials []appregistrations.CredentialItem, now time.Time) *appregistrations.CredentialItem {
	if next := nextFutureCredential(credentials, now); next != nil {
		return next
	}
	var expired *appregistrations.CredentialItem
	for i := range credentials {
		if credentials[i].EndDateTime == nil {
			continue
		}
		if expired == nil || credentials[i].EndDateTime.After(*expired.EndDateTime) {
			expired = &credentials[i]
		}
	}
	return expired
}

func nextFutureCredential(credentials []appregistrations.CredentialItem, now time.Time) *appregistrations.CredentialItem {
	var next *appregistrations.CredentialItem
	for i := range credentials {
		expiresAt := credentials[i].EndDateTime
		if expiresAt == nil || !expiresAt.After(now) {
			continue
		}
		if next == nil || expiresAt.Before(*next.EndDateTime) {
			next = &credentials[i]
		}
	}
	return next
}

func nextFutureCredentialExpiry(credentials []appregistrations.CredentialItem, now time.Time) *time.Time {
	next := nextFutureCredential(credentials, now)
	if next == nil {
		return nil
	}
	return next.EndDateTime
}

func countCredentialExpiry(credentials []AppRegistrationCredential, now time.Time) (int, int) {
	expiring := 0
	expired := 0
	for _, credential := range credentials {
		if credential.EndDateTime == nil {
			continue
		}
		if !credential.EndDateTime.After(now) {
			expired++
			continue
		}
		if !credential.EndDateTime.After(now.Add(30 * 24 * time.Hour)) {
			expiring++
		}
	}
	return expiring, expired
}

func appRegistrationDisplayName(app appregistrations.ApplicationItem) string {
	if value := strings.TrimSpace(app.DisplayName); value != "" {
		return value
	}
	if value := strings.TrimSpace(app.AppID); value != "" {
		return value
	}
	return strings.TrimSpace(app.ID)
}

func appRegistrationReference(app appregistrations.ApplicationItem) string {
	if value := strings.TrimSpace(app.AppID); value != "" {
		return "app-registration://" + value
	}
	if value := strings.TrimSpace(app.ID); value != "" {
		return "app-registration://" + value
	}
	return "app-registration://unknown"
}

func autoImportCreateInput(source KeyVaultSyncSourceConfig, secret keyvault.SecretItem, now time.Time) CreateResourceInput {
	return CreateResourceInput{
		Name:            strings.TrimSpace(secret.Name),
		Type:            TypeKeyVaultSecret,
		Description:     strings.TrimSpace(source.DefaultDescription),
		Owner:           strings.TrimSpace(source.DefaultOwner),
		OwnerTeam:       strings.TrimSpace(source.DefaultOwnerTeam),
		Environment:     strings.TrimSpace(source.DefaultEnvironment),
		Status:          keyVaultStatus(secret.Enabled),
		SourceKind:      SourceKindAzureKeyVault,
		SourceObjectID:  strings.TrimRight(strings.TrimSpace(secret.ID), "/"),
		LastSyncedAt:    &now,
		Notes:           strings.TrimSpace(source.DefaultNotes),
		VaultName:       strings.TrimSpace(secret.VaultName),
		ObjectName:      strings.TrimSpace(secret.Name),
		ObjectType:      "secret",
		ObjectVersion:   strings.TrimSpace(secret.Version),
		ContentType:     strings.TrimSpace(secret.ContentType),
		ExpiresAt:       secret.ExpiresAt,
		LinkedSecretRef: strings.TrimRight(strings.TrimSpace(secret.ID), "/"),
		RevealAllowed:   true,
		CopyAllowed:     true,
		AllowedGroups:   append([]string{}, source.DefaultAllowedGroups...),
		SecretMode:      SecretModeExternal,
		SecretReference: strings.TrimRight(strings.TrimSpace(secret.ID), "/"),
	}
}

func buildLaunchPayload(resource Resource) LaunchPayload {
	payload := LaunchPayload{
		ResourceID:   resource.ID,
		ResourceType: resource.Type,
		Target:       resource.TargetHost,
		Metadata: map[string]any{
			"username":   resource.Username,
			"folderPath": resource.FolderPath,
			"launchMode": resource.LaunchMode,
			"secretMode": resource.Secret.Mode,
		},
	}

	switch resource.Type {
	case TypeRDP:
		port := 3389
		if resource.TargetPort != nil {
			port = *resource.TargetPort
		}
		payload.Method = "command_proposal"
		payload.Command = fmt.Sprintf("mstsc /v:%s:%d", resource.TargetHost, port)
		payload.Metadata["protocol"] = "rdp"
		payload.Metadata["port"] = fmt.Sprintf("%d", port)
		payload.Metadata["connectionDomain"] = resource.ConnectionDomain
		payload.Metadata["connectionAdminSession"] = resource.ConnectionAdminSession
		payload.Metadata["connectionAutomaticLogon"] = resource.ConnectionAutomaticLogon
		payload.Metadata["connectionMacAddress"] = resource.ConnectionMacAddress
	case TypeSSH:
		port := 22
		if resource.TargetPort != nil {
			port = *resource.TargetPort
		}
		payload.Method = "command_proposal"
		payload.Metadata["protocol"] = "ssh"
		payload.Metadata["port"] = fmt.Sprintf("%d", port)
		if resource.Username != "" {
			payload.Command = fmt.Sprintf("ssh %s@%s -p %d", resource.Username, resource.TargetHost, port)
		} else {
			payload.Command = fmt.Sprintf("ssh %s -p %d", resource.TargetHost, port)
		}
	case TypeWebPortal:
		payload.Method = "url"
		payload.URL = resource.TargetURL
		payload.Target = resource.TargetURL
	default:
		payload.Method = "metadata"
		payload.Metadata["message"] = "No native launcher for this resource type in the MVP."
	}

	return payload
}

func (s *Service) applyConnectionCredentialOverride(ctx context.Context, user auth.User, resource Resource) Resource {
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return resource
	}
	override, err := s.repo.GetConnectionUserPasswordOverride(ctx, resource.ID, user.ID)
	if err != nil {
		return resource
	}
	passwordResource, err := s.repo.Get(ctx, override.PasswordResourceID)
	if err != nil {
		return resource
	}
	if !canViewResource(user, passwordResource.Summary()) || !isPasswordOverrideCandidate(passwordResource) {
		return resource
	}
	resource.Username = passwordResource.Username
	resource.Secret = passwordResource.Secret
	resource.LinkedSecretRef = passwordResource.LinkedSecretRef
	return resource
}

func (s *Service) buildLauncherPayload(ctx context.Context, user auth.User, resource Resource, browserPayload LaunchPayload) (LaunchPayload, error) {
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return browserPayload, nil
	}

	launcherPayload := browserPayload
	launcherPayload.Method = "launcher_handoff"
	launcherPayload.Metadata = cloneLaunchMetadata(browserPayload.Metadata)
	launcherPayload.Metadata["resourceName"] = resource.Name
	launcherPayload.Metadata["connectionName"] = resource.Name
	launcherPayload.Metadata["connectionDomain"] = resource.ConnectionDomain
	launcherPayload.Metadata["connectionAutomaticLogon"] = resource.ConnectionAutomaticLogon
	if resource.Type == TypeRDP && s.rdpSigning != nil {
		config, err := s.rdpSigning.GetRDPSigningRuntime(ctx)
		if err != nil {
			return LaunchPayload{}, err
		}
		if config.Enabled && config.CertificateConfigured {
			launcherPayload.Metadata["rdpSigningEnabled"] = true
			launcherPayload.Metadata["rdpSigningSubject"] = config.Subject
			launcherPayload.Metadata["rdpSigningThumbprintSha256"] = config.ThumbprintSHA256
			launcherPayload.Metadata["rdpSigningPfxBase64"] = config.PFXBase64
			launcherPayload.Metadata["rdpSigningPfxPassword"] = config.PFXPassword
			launcherPayload.Metadata["rdpSigningLeafCertBase64"] = config.LeafCertBase64
			launcherPayload.Metadata["rdpSigningRootCertBase64"] = config.RootCertBase64
		}
	}

	secretValue, err := s.resolveLaunchSecret(ctx, user, resource)
	if err != nil {
		return LaunchPayload{}, err
	}
	if secretValue != "" {
		launcherPayload.Metadata["secretValue"] = secretValue
	}

	ticket := s.launchTickets.Issue(launcherPayload, 2*time.Minute)
	browserPayload.Method = "launcher_ticket"
	browserPayload.Metadata["launcherTicket"] = ticket
	return browserPayload, nil
}

func cloneLaunchMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

// decryptStoredSecret is the single read entrypoint for inline secret
// values: personal envelopes open with the requester's session vault key
// (ErrVaultLocked when absent), everything else through the org cipher.
func (s *Service) decryptStoredSecret(ctx context.Context, user auth.User, value string) (string, error) {
	if IsPersonalEnvelope(value) {
		return s.cipher.DecryptPersonalFromStorage(ctx, value, user.VaultPrivateKey)
	}
	return s.cipher.DecryptFromStorage(ctx, value)
}

func (s *Service) resolveRevealValue(ctx context.Context, user auth.User, resource Resource) (string, error) {
	if resource.Type == TypeKeyVaultSecret &&
		resource.SourceKind == SourceKindAzureKeyVault &&
		resource.Secret.Mode == SecretModeExternal &&
		resource.Secret.Reference != "" &&
		s.keyVault != nil {
		value, err := s.keyVault.RevealSecret(ctx, resource.Secret.Reference)
		if err != nil {
			return "", err
		}
		return value, nil
	}
	if resource.Secret.Mode == SecretModeInline && s.cipher != nil {
		return s.decryptStoredSecret(ctx, user, resource.Secret.Value)
	}
	return resource.Secret.Value, nil
}

func (s *Service) resolveLaunchSecret(ctx context.Context, user auth.User, resource Resource) (string, error) {
	switch resource.Secret.Mode {
	case SecretModePrompt:
		return "", nil
	case SecretModeInline:
		if s.cipher != nil {
			return s.decryptStoredSecret(ctx, user, resource.Secret.Value)
		}
		return resource.Secret.Value, nil
	case SecretModeExternal:
		reference := strings.TrimSpace(resource.Secret.Reference)
		if reference == "" {
			reference = strings.TrimSpace(resource.LinkedSecretRef)
		}
		if reference == "" || s.keyVault == nil {
			return "", nil
		}
		return s.keyVault.RevealSecret(ctx, reference)
	default:
		return "", nil
	}
}

type devViewer auth.User

func (u devViewer) GetID() string            { return u.ID }
func (u devViewer) GetLocalGroups() []string { return u.LocalGroups }
func (u devViewer) GetIsAdmin() bool         { return u.IsAdmin }

func canViewResource(user auth.User, resource ResourceSummary) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].View && CanAccess(devViewer(user), resource)
}

func canRevealResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal && CanAccess(devViewer(user), resource.Summary())
}

func canLaunchResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Launch && CanAccess(devViewer(user), resource.Summary())
}

func canFillPasswordResource(user auth.User, resource Resource) bool {
	return auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal &&
		resource.CopyAllowed &&
		CanAccess(devViewer(user), resource.Summary())
}

func canRevealStoredPassword(user auth.User, resource Resource) bool {
	return CategoryForType(resource.Type) == "passwords" &&
		auth.CapabilitiesForUser(user).Categories[resource.Category].Reveal &&
		CanAccess(devViewer(user), resource.Summary())
}

func explainVisibleResourcesForUser(user auth.User, items []ResourceSummary) []VisibleResourceSummary {
	capabilities := auth.CapabilitiesForUser(user)
	viewer := devViewer(user)
	out := make([]VisibleResourceSummary, 0, len(items))
	for _, item := range items {
		category := capabilities.Categories[item.Category]
		if !category.View {
			continue
		}
		scope, matchedGroups, ok := visibilityScopeForResource(viewer, item)
		if !ok {
			continue
		}
		out = append(out, VisibleResourceSummary{
			ResourceSummary:     item,
			CategoryAccessRight: categoryAccessRight(user, item.Category),
			VisibilityScope:     scope,
			MatchedLocalGroups:  matchedGroups,
		})
	}
	return out
}

func visibilityScopeForResource(user devViewer, item ResourceSummary) (string, []string, bool) {
	if item.Personal {
		// Personal resources are visible ONLY to their owner — admins included.
		if item.OwnerUserID == user.ID {
			return "personal", nil, true
		}
		return "", nil, false
	}
	if item.OwnerUserID == user.ID {
		return "owner", nil, true
	}
	if user.GetIsAdmin() {
		return "administrator", nil, true
	}
	if len(item.AllowedGroups) == 0 {
		return "everyone", nil, true
	}
	matched := MatchedAllowedGroups(user, item)
	if len(matched) == 0 {
		return "", nil, false
	}
	return "matched_groups", matched, true
}

func categoryAccessRight(user auth.User, category string) string {
	if user.IsAdmin {
		return "admin.access"
	}

	switch category {
	case "connections":
		if slices.Contains(user.Rights, "connections.edit") {
			return "connections.edit"
		}
		if slices.Contains(user.Rights, "connections.read") {
			return "connections.read"
		}
	case "keyvault":
		if slices.Contains(user.Rights, "keyvault.edit") {
			return "keyvault.edit"
		}
		if slices.Contains(user.Rights, "keyvault.read") {
			return "keyvault.read"
		}
	case "appregistrations":
		if slices.Contains(user.Rights, "appregistrations.edit") {
			return "appregistrations.edit"
		}
		if slices.Contains(user.Rights, "appregistrations.read") {
			return "appregistrations.read"
		}
	case "passwords":
		if slices.Contains(user.Rights, "passwords.edit") {
			return "passwords.edit"
		}
		if slices.Contains(user.Rights, "passwords.read") {
			return "passwords.read"
		}
	}

	return ""
}

func (r Resource) Summary() ResourceSummary {
	return ResourceSummary{
		ID:                  r.ID,
		Name:                r.Name,
		Type:                r.Type,
		Category:            r.Category,
		Personal:            r.Personal,
		Description:         r.Description,
		Owner:               r.Owner,
		OwnerUserID:         r.OwnerUserID,
		OwnerTeam:           r.OwnerTeam,
		Environment:         r.Environment,
		Status:              r.Status,
		FolderPath:          r.FolderPath,
		LaunchMode:          r.LaunchMode,
		SourceKind:          r.SourceKind,
		TargetHost:          r.TargetHost,
		TargetPort:          r.TargetPort,
		TargetURL:           r.TargetURL,
		TargetSystem:        r.TargetSystem,
		Username:            r.Username,
		ConnectionDomain:    r.ConnectionDomain,
		VaultName:           r.VaultName,
		ObjectName:          r.ObjectName,
		Provider:            r.Provider,
		ApplicationID:       r.ApplicationID,
		CredentialExpiresAt: r.CredentialExpiresAt,
		ExpiresAt:           r.ExpiresAt,
		LaunchAllowed:       r.LaunchAllowed,
		RevealAllowed:       r.RevealAllowed,
		CopyAllowed:         r.CopyAllowed,
		AllowedGroups:       r.AllowedGroups,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
		ArchivedAt:          r.ArchivedAt,
	}
}

func normalizeInput(input CreateResourceInput) CreateResourceInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Owner = strings.TrimSpace(input.Owner)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.OwnerTeam = strings.TrimSpace(input.OwnerTeam)
	input.Environment = strings.TrimSpace(input.Environment)
	input.Status = strings.TrimSpace(input.Status)
	input.FolderPath = normalizeFolderPath(input.FolderPath)
	input.LaunchMode = strings.TrimSpace(input.LaunchMode)
	input.SourceObjectID = strings.TrimSpace(input.SourceObjectID)
	input.Notes = strings.TrimSpace(input.Notes)
	input.TargetHost = strings.TrimSpace(input.TargetHost)
	input.TargetURL = strings.TrimSpace(input.TargetURL)
	input.TargetSystem = strings.TrimSpace(input.TargetSystem)
	input.Username = strings.TrimSpace(input.Username)
	input.ConnectionDomain = strings.TrimSpace(input.ConnectionDomain)
	input.ConnectionWindowMode = strings.TrimSpace(input.ConnectionWindowMode)
	input.ConnectionScreenMode = strings.TrimSpace(input.ConnectionScreenMode)
	input.ConnectionMacAddress = strings.TrimSpace(input.ConnectionMacAddress)
	input.VaultName = strings.TrimSpace(input.VaultName)
	input.ObjectName = strings.TrimSpace(input.ObjectName)
	input.ObjectType = strings.TrimSpace(input.ObjectType)
	input.ObjectVersion = strings.TrimSpace(input.ObjectVersion)
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.Provider = strings.TrimSpace(input.Provider)
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.CredentialType = strings.TrimSpace(input.CredentialType)
	input.DisplayNameExternal = strings.TrimSpace(input.DisplayNameExternal)
	input.LinkedSecretRef = strings.TrimSpace(input.LinkedSecretRef)
	input.SecretValue = strings.TrimSpace(input.SecretValue)
	input.SecretReference = strings.TrimSpace(input.SecretReference)

	if input.Status == "" {
		input.Status = "active"
	}

	if input.SourceKind == "" {
		switch input.Type {
		case TypeKeyVaultSecret:
			input.SourceKind = SourceKindAzureKeyVault
		case TypeAppRegistration:
			input.SourceKind = SourceKindEntraAppRegistration
		default:
			input.SourceKind = SourceKindManual
		}
	}
	if input.LaunchMode == "" && (input.Type == TypeSSH || input.Type == TypeRDP) {
		input.LaunchMode = "native_launcher"
	}
	if input.Type == TypeWebPortal {
		input.TargetURL = normalizePortalURLString(input.TargetURL)
	}
	if input.Type == TypeRDP {
		input.ConnectionWindowMode = "launcher_default"
		input.ConnectionScreenMode = "launcher_default"
		input.ConnectionUseMultipleMonitors = false
		input.ConnectionShowConnectionBar = true
	}

	input.AllowedGroups = normalizeValues(input.AllowedGroups)
	if input.Personal {
		input.AllowedGroups = []string{}
	}
	return input
}

func normalizeFolderPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	parts := strings.Split(value, "/")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, "/")
}

func folderDepth(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	return len(strings.Split(value, "/"))
}

func normalizeValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func validateInput(input CreateResourceInput) error {
	if input.Name == "" || input.Owner == "" {
		return fmt.Errorf("%w: name and owner are required", ErrInvalidInput)
	}
	if folderDepth(input.FolderPath) > 2 {
		return fmt.Errorf("%w: folder path supports only root/subfolder", ErrInvalidInput)
	}
	if input.Personal {
		if CategoryForType(input.Type) != "passwords" {
			return fmt.Errorf("%w: only password objects can be personal", ErrInvalidInput)
		}
		if input.SourceKind != "" && input.SourceKind != SourceKindManual {
			return fmt.Errorf("%w: personal password objects must be manual", ErrInvalidInput)
		}
		if input.Username == "" {
			return fmt.Errorf("%w: personal password objects require a username", ErrInvalidInput)
		}
	}

	switch input.Type {
	case TypeSSH, TypeRDP:
		if input.TargetHost == "" {
			return fmt.Errorf("%w: connections require a target host", ErrInvalidInput)
		}
	case TypeWebPortal:
		if _, ok := normalizePortalURL(input.TargetURL); !ok {
			return fmt.Errorf("%w: password entries with portal access require a target URL", ErrInvalidInput)
		}
	case TypeSharedSecret:
		// Saved passwords can exist without a specific target system so they can be reused
		// as personal or shared connection overrides.
	case TypeKeyVaultSecret:
		if input.VaultName == "" || input.ObjectName == "" {
			return fmt.Errorf("%w: key vault entries require vault and object names", ErrInvalidInput)
		}
		if input.ObjectType == "" {
			input.ObjectType = "secret"
		}
	case TypeAppRegistration:
		if input.Provider == "" || input.ApplicationID == "" {
			return fmt.Errorf("%w: app registrations require provider and application id", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported resource type", ErrInvalidInput)
	}

	if input.SecretMode == "" {
		return fmt.Errorf("%w: secret mode is required", ErrInvalidInput)
	}
	if input.SecretMode == SecretModePrompt && input.Type != TypeSSH && input.Type != TypeRDP {
		return fmt.Errorf("%w: prompt-on-launch secret mode is only valid for connections", ErrInvalidInput)
	}
	if input.SecretMode == SecretModeInline &&
		(input.Type == TypeSharedSecret || input.Type == TypeWebPortal) &&
		input.SecretValue == "" {
		return fmt.Errorf("%w: inline passwords require a secret value", ErrInvalidInput)
	}
	if input.SecretMode == SecretModeExternal && input.SecretReference == "" && input.LinkedSecretRef == "" && input.Type != TypeAppRegistration {
		return fmt.Errorf("%w: external-reference objects require a secret reference", ErrInvalidInput)
	}
	if input.SecretMode == SecretModePrompt && input.SecretValue != "" {
		return fmt.Errorf("%w: prompt-on-launch resources cannot store a secret value", ErrInvalidInput)
	}

	return nil
}

func preserveManagedFields(existing Resource, input UpdateResourceInput, user auth.User) UpdateResourceInput {
	if strings.TrimSpace(input.OwnerUserID) == "" {
		input.OwnerUserID = existing.OwnerUserID
	}
	if existing.Type == TypeKeyVaultSecret && existing.SourceKind == SourceKindAzureKeyVault {
		input.Type = existing.Type
		input.SourceKind = existing.SourceKind
		input.SourceObjectID = existing.SourceObjectID
		input.LastSyncedAt = existing.LastSyncedAt
		input.VaultName = existing.VaultName
		input.ObjectName = existing.ObjectName
		input.ObjectType = existing.ObjectType
		input.ObjectVersion = existing.ObjectVersion
		input.ContentType = existing.ContentType
		input.ExpiresAt = existing.ExpiresAt
		input.LinkedSecretRef = existing.LinkedSecretRef
		input.SecretMode = existing.Secret.Mode
		input.SecretReference = existing.Secret.Reference
		input.SecretValue = ""
	}
	if existing.Type == TypeAppRegistration && existing.SourceKind == SourceKindEntraAppRegistration {
		input.Name = existing.Name
		input.Type = existing.Type
		input.Status = existing.Status
		input.SourceKind = existing.SourceKind
		input.SourceObjectID = existing.SourceObjectID
		input.LastSyncedAt = existing.LastSyncedAt
		input.Provider = existing.Provider
		input.ApplicationID = existing.ApplicationID
		input.TenantID = existing.TenantID
		input.ClientID = existing.ClientID
		input.CredentialType = existing.CredentialType
		input.CredentialExpiresAt = existing.CredentialExpiresAt
		input.DisplayNameExternal = existing.DisplayNameExternal
		input.LaunchAllowed = existing.LaunchAllowed
		input.RevealAllowed = existing.RevealAllowed
		input.CopyAllowed = existing.CopyAllowed
		input.SecretMode = existing.Secret.Mode
		input.SecretReference = existing.Secret.Reference
		input.SecretValue = ""
	}
	if existing.Personal && input.Personal {
		input.Owner = existing.Owner
		input.OwnerUserID = existing.OwnerUserID
		input.AllowedGroups = []string{}
	}
	return input
}

func (s *Service) prepareSecretForStorage(ctx context.Context, input *CreateResourceInput) error {
	if input == nil || s.cipher == nil {
		return nil
	}
	if input.SecretMode != SecretModeInline {
		input.SecretValue = ""
		return nil
	}
	if strings.TrimSpace(input.SecretValue) == "" {
		return nil
	}
	// Personal secrets are sealed to the owner's vault public key — writing
	// needs no unlock, only that the vault exists. If the owner has no vault
	// yet, refuse with ErrVaultLocked so the client runs vault setup rather
	// than silently storing a "personal" secret in the org-readable class.
	// (SSO users get their vault at login; local users at login/creation.)
	if input.Personal && s.vaults != nil {
		publicKey, err := s.vaults.VaultPublicKey(ctx, strings.TrimSpace(input.OwnerUserID))
		if err != nil {
			return err
		}
		if len(publicKey) == 0 {
			return ErrVaultLocked
		}
		encrypted, err := s.cipher.EncryptPersonalForStorage(ctx, input.SecretValue, publicKey)
		if err != nil {
			return err
		}
		input.SecretValue = encrypted
		return nil
	}
	encrypted, err := s.cipher.EncryptForStorage(ctx, input.SecretValue, SecretClassShared)
	if err != nil {
		return err
	}
	input.SecretValue = encrypted
	return nil
}

func preserveExistingSecret(existing Resource, input UpdateResourceInput) UpdateResourceInput {
	if input.SecretMode == SecretModeInline && strings.TrimSpace(input.SecretValue) == "" {
		input.SecretValue = existing.Secret.Value
	}
	if input.SecretMode == SecretModeExternal && strings.TrimSpace(input.SecretReference) == "" {
		input.SecretReference = existing.Secret.Reference
	}
	return input
}

func resourceToUpdateInput(resource Resource) UpdateResourceInput {
	return UpdateResourceInput{
		Name:                          resource.Name,
		Type:                          resource.Type,
		Personal:                      resource.Personal,
		Description:                   resource.Description,
		Owner:                         resource.Owner,
		OwnerUserID:                   resource.OwnerUserID,
		OwnerTeam:                     resource.OwnerTeam,
		Environment:                   resource.Environment,
		Status:                        resource.Status,
		FolderPath:                    resource.FolderPath,
		LaunchMode:                    resource.LaunchMode,
		SourceKind:                    resource.SourceKind,
		SourceObjectID:                resource.SourceObjectID,
		LastSyncedAt:                  resource.LastSyncedAt,
		Notes:                         resource.Notes,
		TargetHost:                    resource.TargetHost,
		TargetPort:                    resource.TargetPort,
		TargetURL:                     resource.TargetURL,
		TargetSystem:                  resource.TargetSystem,
		Username:                      resource.Username,
		ConnectionDomain:              resource.ConnectionDomain,
		ConnectionAdminSession:        resource.ConnectionAdminSession,
		ConnectionAutomaticLogon:      resource.ConnectionAutomaticLogon,
		ConnectionWindowMode:          resource.ConnectionWindowMode,
		ConnectionUseMultipleMonitors: resource.ConnectionUseMultipleMonitors,
		ConnectionShowConnectionBar:   resource.ConnectionShowConnectionBar,
		ConnectionScreenMode:          resource.ConnectionScreenMode,
		ConnectionMacAddress:          resource.ConnectionMacAddress,
		VaultName:                     resource.VaultName,
		ObjectName:                    resource.ObjectName,
		ObjectType:                    resource.ObjectType,
		ObjectVersion:                 resource.ObjectVersion,
		ContentType:                   resource.ContentType,
		ExpiresAt:                     resource.ExpiresAt,
		Provider:                      resource.Provider,
		ApplicationID:                 resource.ApplicationID,
		TenantID:                      resource.TenantID,
		ClientID:                      resource.ClientID,
		CredentialType:                resource.CredentialType,
		CredentialExpiresAt:           resource.CredentialExpiresAt,
		DisplayNameExternal:           resource.DisplayNameExternal,
		LinkedSecretRef:               resource.LinkedSecretRef,
		LaunchAllowed:                 resource.LaunchAllowed,
		RevealAllowed:                 resource.RevealAllowed,
		CopyAllowed:                   resource.CopyAllowed,
		AllowedGroups:                 append([]string{}, resource.AllowedGroups...),
		SecretMode:                    resource.Secret.Mode,
		SecretReference:               resource.Secret.Reference,
	}
}

func canCreatePersonalPassword(user auth.User, input CreateResourceInput) bool {
	if !input.Personal {
		return false
	}
	return auth.CapabilitiesForUser(user).Categories["passwords"].Create && CategoryForType(input.Type) == "passwords"
}

func canUpdateResource(user auth.User, existing Resource) bool {
	isOwner := existing.OwnerUserID == user.ID &&
		auth.CapabilitiesForUser(user).Categories[existing.Category].Edit
	// Personal resources can only be managed by their owner. Admins are excluded
	// so they cannot flip a personal secret to shared and then reveal it.
	if existing.Personal {
		return isOwner
	}
	if user.IsAdmin {
		return true
	}
	return isOwner
}

func canArchiveResource(user auth.User, resource Resource) bool {
	isOwner := resource.OwnerUserID == user.ID &&
		auth.CapabilitiesForUser(user).Categories[resource.Category].Edit
	// Personal resources can only be managed by their owner — admins included.
	if resource.Personal {
		return isOwner
	}
	if user.IsAdmin {
		return true
	}
	return isOwner
}

func enforcePersonalPasswordOwnership(user auth.User, input CreateResourceInput) CreateResourceInput {
	if !input.Personal || CategoryForType(input.Type) != "passwords" {
		return input
	}
	input.Owner = user.Name
	input.OwnerUserID = user.ID
	input.OwnerTeam = ""
	input.AllowedGroups = []string{}
	return input
}

func isPasswordOverrideCandidate(resource Resource) bool {
	if CategoryForType(resource.Type) != "passwords" {
		return false
	}
	if resource.SourceKind != SourceKindManual {
		return false
	}
	if strings.TrimSpace(resource.Username) == "" {
		return false
	}
	return resource.Type == TypeSharedSecret || resource.Type == TypeWebPortal
}

func isPasswordOverrideCandidateSummary(resource ResourceSummary) bool {
	if CategoryForType(resource.Type) != "passwords" {
		return false
	}
	if resource.SourceKind != SourceKindManual {
		return false
	}
	if strings.TrimSpace(resource.Username) == "" {
		return false
	}
	return resource.Type == TypeSharedSecret || resource.Type == TypeWebPortal
}

func normalizeOptionalNotificationPolicy(policy *AppRegistrationNotificationPolicy) *AppRegistrationNotificationPolicy {
	if policy == nil {
		return nil
	}
	copyPolicy := *policy
	seenDays := map[int]struct{}{}
	normalizedDays := make([]int, 0, len(copyPolicy.ReminderDays))
	for _, day := range copyPolicy.ReminderDays {
		if day < 0 || day > 365 {
			continue
		}
		if _, ok := seenDays[day]; ok {
			continue
		}
		seenDays[day] = struct{}{}
		normalizedDays = append(normalizedDays, day)
	}
	slices.Sort(normalizedDays)
	slices.Reverse(normalizedDays)
	copyPolicy.ReminderDays = normalizedDays

	seenChannels := map[NotificationChannel]struct{}{}
	normalizedChannels := make([]NotificationChannel, 0, len(copyPolicy.Channels))
	for _, channel := range copyPolicy.Channels {
		switch channel {
		case NotificationChannelInApp, NotificationChannelEmail:
		default:
			continue
		}
		if _, ok := seenChannels[channel]; ok {
			continue
		}
		seenChannels[channel] = struct{}{}
		normalizedChannels = append(normalizedChannels, channel)
	}
	copyPolicy.Channels = normalizedChannels
	return &copyPolicy
}

type normalizedPortalTarget struct {
	Scheme string
	Host   string
	Path   string
}

func normalizePortalURLString(raw string) string {
	normalized, ok := normalizePortalURL(raw)
	if !ok {
		return strings.TrimSpace(raw)
	}
	return normalized.String()
}

func normalizePortalURL(raw string) (normalizedPortalTarget, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return normalizedPortalTarget{}, false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return normalizedPortalTarget{}, false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = "/"
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "/"
		}
	}
	return normalizedPortalTarget{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}, true
}

func (n normalizedPortalTarget) String() string {
	if n.Host == "" {
		return ""
	}
	return n.Scheme + "://" + n.Host + n.Path
}

func portalURLMatches(target string, current normalizedPortalTarget) bool {
	targetURL, ok := normalizePortalURL(target)
	if !ok {
		return false
	}
	if targetURL.Host != current.Host {
		return false
	}
	if targetURL.Scheme != "" && current.Scheme != "" && targetURL.Scheme != current.Scheme {
		return false
	}
	if targetURL.Path == "/" {
		return true
	}
	if current.Path == targetURL.Path {
		return true
	}
	return strings.HasPrefix(current.Path, targetURL.Path+"/")
}
