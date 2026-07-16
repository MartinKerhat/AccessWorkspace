package resources

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"

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
	Delete(ctx context.Context, id string) error
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

func (s *Service) Get(ctx context.Context, user auth.User, id string) (Resource, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return Resource{}, accessDenied(user, resource.Summary())
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
	if !user.IsAdmin && !canCreatePassword(user, input) && !canCreateConnection(user, input) {
		return Resource{}, ErrForbidden
	}
	input = enforceCreatorOwnership(user, input)
	input = enforcePersonalPasswordOwnership(user, input)
	input = normalizeInput(input)
	if err := s.prepareSecretForStorage(ctx, user, &input); err != nil {
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
		return Resource{}, accessDenied(user, existing.Summary())
	}
	// Only admins may reassign ownership; for everyone else the stored owner
	// always wins over whatever the request body claims.
	if !user.IsAdmin {
		input.OwnerUserID = existing.OwnerUserID
	}
	// Converting shared → personal seals the secret to the editor's vault, so
	// only the current owner may do it. A non-owner (e.g. an admin editing
	// someone else's shared resource) must first reassign ownership to
	// themselves via ownerUserId, then personalize as the new owner in a
	// second edit — never in one step, which would let them pull an org
	// secret straight into their own vault unnoticed.
	if input.Personal && !existing.Personal && existing.OwnerUserID != user.ID {
		return Resource{}, ErrForbidden
	}
	if isSharedMetadataEditor(user, existing) {
		input = restrictToSharedMetadata(existing, input)
	}
	input = enforcePersonalPasswordOwnership(user, input)
	input = normalizeInput(input)
	input = preserveManagedFields(existing, input, user)
	input = preserveExistingSecret(existing, input)
	if err := s.prepareSecretForStorage(ctx, user, &input); err != nil {
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

func (s *Service) Archive(ctx context.Context, user auth.User, id string) error {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !canArchiveResource(user, resource) {
		return accessDenied(user, resource.Summary())
	}
	// Personal objects are never archived: they must stay invisible to everyone
	// but their owner, and the archive view is an admin surface. "Remove" on a
	// personal object therefore deletes it for real (FKs cascade).
	if resource.Personal {
		if err := s.repo.Delete(ctx, id); err != nil {
			return err
		}
		_ = s.audit.Log(ctx, audit.LogParams{
			EventType:    audit.EventResourceDeleted,
			UserID:       user.ID,
			UserName:     user.Name,
			ResourceID:   &resource.ID,
			ResourceName: &resource.Name,
			Metadata: map[string]any{
				"type":   resource.Type,
				"reason": "personal_removed_permanently",
			},
		})
		return nil
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
	// Personal objects are hard-deleted rather than archived; any legacy
	// archived personal row stays invisible — even its id must not confirm
	// that it exists.
	if resource.Personal {
		return ErrNotFound
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
