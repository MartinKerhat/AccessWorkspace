package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/artifacts"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/keyvault"
	"access-workspace/backend/internal/resources"
)

type ResourceService interface {
	List(ctx context.Context, user auth.User, filter resources.Filter) ([]resources.ResourceSummary, error)
	ExplainVisibleResources(ctx context.Context, user auth.User, filter resources.Filter) ([]resources.VisibleResourceSummary, error)
	Get(ctx context.Context, user auth.User, id string) (resources.Resource, error)
	ListArchived(ctx context.Context, user auth.User) ([]resources.ArchivedResourceSummary, error)
	Create(ctx context.Context, user auth.User, input resources.CreateResourceInput) (resources.Resource, error)
	Update(ctx context.Context, user auth.User, id string, input resources.UpdateResourceInput) (resources.Resource, error)
	Archive(ctx context.Context, user auth.User, id string) error
	Restore(ctx context.Context, user auth.User, id string) error
	ListPasswordOptions(ctx context.Context, user auth.User) ([]resources.ResourceSummary, error)
	ListPortalCredentialMatches(ctx context.Context, user auth.User, rawURL string) ([]resources.PortalCredentialMatch, error)
	FillPortalCredential(ctx context.Context, user auth.User, resourceID string, rawURL string) (resources.PortalCredentialFillResult, error)
	GetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) (resources.ConnectionCredentialOverride, error)
	SetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string, input resources.ConnectionCredentialOverrideInput) (resources.ConnectionCredentialOverride, error)
	ClearConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) error
	Reveal(ctx context.Context, user auth.User, id string) (resources.RevealResult, error)
	Launch(ctx context.Context, user auth.User, id string) (resources.LaunchPayload, error)
	ResolveLaunchTicket(ctx context.Context, ticket string) (resources.LaunchPayload, error)
	SyncKeyVault(ctx context.Context, user auth.User, sources []resources.KeyVaultSyncSourceConfig, automatic bool) (resources.KeyVaultSyncResult, error)
	ImportAppRegistrations(ctx context.Context, user auth.User, input resources.AppRegistrationImportInput) ([]resources.Resource, error)
	SyncAppRegistrations(ctx context.Context, user auth.User, automatic bool) (resources.AppRegistrationSyncResult, error)
	UpdateAppRegistrationNotificationPolicies(ctx context.Context, user auth.User, id string, input resources.AppRegistrationNotificationPolicyUpdateInput) (resources.Resource, error)
}

type AuditService interface {
	Log(ctx context.Context, params audit.LogParams) error
	List(ctx context.Context, filter audit.ListFilter) ([]audit.Event, int, error)
	ListEventTypes(ctx context.Context) ([]string, error)
	RecentForUser(ctx context.Context, userID string, limit int) ([]audit.Event, error)
}

type Dependencies struct {
	Authenticator    auth.Authenticator
	Resources        ResourceService
	KeyVault         KeyVaultService
	AppRegistrations AppRegistrationService
	Audit            AuditService
	FrontendURL      string
	AdminConfig      AdminConfigProvider
	LocalGroups      LocalGroupAdminService
	Notifications    NotificationService
	Artifacts        ArtifactService
}

type AdminConfigService interface {
	Get(ctx context.Context) (any, error)
	GetRuntime(ctx context.Context) (any, error)
	Update(ctx context.Context, input any) (any, error)
	UpdateKeyVaultSyncState(ctx context.Context, payload any) error
	UpdateAppRegistrationSyncState(ctx context.Context, state any) error
	GenerateTestRDPSigningCertificate(ctx context.Context) (any, error)
}

type RDPSigningPublicConfig struct {
	Enabled               bool   `json:"enabled"`
	CertificateConfigured bool   `json:"certificateConfigured"`
	Subject               string `json:"subject"`
	ThumbprintSHA256      string `json:"thumbprintSha256"`
	LeafCertBase64        string `json:"leafCertBase64"`
	RootCertBase64        string `json:"rootCertBase64"`
}

type RDPSigningPublicProvider interface {
	GetRDPSigningPublic(ctx context.Context) (RDPSigningPublicConfig, error)
}

type AdminConfigProvider interface {
	AdminConfigService
	RDPSigningPublicProvider
}

type NotificationService interface {
	ListForUser(ctx context.Context, userID string, limit int) ([]resources.UserNotification, error)
	ListRecentEmailDeliveries(ctx context.Context, limit int) ([]resources.NotificationDeliveryRecord, error)
	MarkRead(ctx context.Context, userID string, notificationID string) error
}

type KeyVaultService interface {
	Discover(ctx context.Context) (keyvault.DiscoverResult, error)
	ValidateReference(ctx context.Context, reference string) error
}

type AppRegistrationService interface {
	Discover(ctx context.Context) (appregistrations.DiscoverResult, error)
}

type Server struct {
	authenticator    auth.Authenticator
	resources        ResourceService
	keyVault         KeyVaultService
	appRegistrations AppRegistrationService
	audit            AuditService
	frontendURL      string
	adminConfig      AdminConfigProvider
	localGroups      LocalGroupAdminService
	notifications    NotificationService
	artifacts        ArtifactService

	launcherVersionMu      sync.Mutex
	launcherVersionCached  string
	launcherVersionExpires time.Time

	authLimiter *ipRateLimiter
}

// ArtifactService lists downloadable launcher and browser-extension artifacts.
type ArtifactService interface {
	LauncherDownloads(ctx context.Context) ([]artifacts.Artifact, error)
	ExtensionPackages(ctx context.Context) ([]artifacts.PackageView, error)
	Open(ctx context.Context, category, name string) (io.ReadCloser, *artifacts.ObjectInfo, error)
}

func NewServer(deps Dependencies) *Server {
	return &Server{
		authenticator:    deps.Authenticator,
		resources:        deps.Resources,
		keyVault:         deps.KeyVault,
		appRegistrations: deps.AppRegistrations,
		audit:            deps.Audit,
		frontendURL:      deps.FrontendURL,
		adminConfig:      deps.AdminConfig,
		localGroups:      deps.LocalGroups,
		notifications:    deps.Notifications,
		artifacts:        deps.Artifacts,
		// 20 auth attempts per minute per source IP: generous for humans,
		// a hard brake on scripted guessing. Cross-replica protection is the
		// persistent account lockout in the auth layer.
		authLimiter: newIPRateLimiter(20, time.Minute),
	}
}

// sensitiveAuthPaths are throttled per source IP — the endpoints where an
// attacker submits guessable credentials/tokens.
var sensitiveAuthPaths = map[string]bool{
	"/api/auth/login":                true,
	"/api/auth/invite/accept":        true,
	"/api/auth/vault/unlock":         true,
	"/api/auth/vault/setup":          true,
	"/api/auth/vault/passkey/unlock": true,
}

type LocalGroupAdminService interface {
	ListLocalGroups(ctx context.Context) ([]auth.LocalGroup, error)
	ListUsers(ctx context.Context) ([]auth.UserSummary, error)
	CreateUser(ctx context.Context, input auth.CreateUserInput) (auth.UserAccessDetail, error)
	GetUserAccess(ctx context.Context, id string) (auth.UserAccessDetail, error)
	UpdateUserAccess(ctx context.Context, id string, input auth.UserAccessUpdateInput) (auth.UserAccessDetail, error)
	DeleteUser(ctx context.Context, actor auth.User, id string) (auth.DeleteUserResult, error)
	CreateLocalGroup(ctx context.Context, input auth.LocalGroupInput) error
	UpdateLocalGroup(ctx context.Context, name string, input auth.LocalGroupInput) error
	IssueUserInvite(ctx context.Context, actor auth.User, userID string, purpose string) (auth.UserInvite, error)
	ResetUserPassword(ctx context.Context, actor auth.User, userID string) (auth.UserInvite, error)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.writeCORS(w)

	if r.URL.Path == "/healthz" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if r.Method == http.MethodPost && sensitiveAuthPaths[r.URL.Path] {
		if !s.authLimiter.allow(clientIP(r), time.Now()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts; try again shortly"})
			return
		}
	}

	user, authErr := s.authenticator.CurrentUser(r.Context(), r)

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/launcher/tickets/"):
		s.handleLaunchTicketResolve(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/launcher/runtime":
		s.handleLauncherRuntime(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/browser-extension/runtime":
		s.handleBrowserExtensionRuntime(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/artifacts/download/"):
		s.handleArtifactDownload(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/launcher/rdp-signing/public":
		s.handleRDPSigningPublic(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/bootstrap":
		s.handleAuthBootstrap(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/login":
		s.handleLogin(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/logout":
		s.handleLogout(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/password":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleChangePassword(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/invite/accept":
		s.handleAcceptInvite(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/vault":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultStatus(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/setup":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultSetup(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/unlock":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultUnlock(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/passphrase":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultAddPassphrase(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/passkey/setup":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultPasskeySetup(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/passkey/unlock":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultPasskeyUnlock(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/passkey/add":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleVaultPasskeyAdd(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/vault/lock":
		if !requireAuth(w, user, authErr) {
			return
		}
		if err := s.authenticator.LockVault(r.Context(), requestBearerToken(r)); err != nil {
			writeError(w, err)
			return
		}
		s.auditVault(r, user, audit.EventVaultLocked, "")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/browser-extension-session":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleBrowserExtensionSession(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/browser-extension-connect-token":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleBrowserExtensionConnectToken(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/browser-extension-connect-exchange":
		s.handleBrowserExtensionConnectExchange(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/me":
		s.handleMe(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/microsoft/start":
		s.handleMicrosoftStart(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/microsoft/callback":
		s.handleMicrosoftCallback(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/resources":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleListResources(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/resources":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleCreateResource(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/passwords/options":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handlePasswordOptions(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/browser-extension/portal-match":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleBrowserExtensionPortalMatch(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/browser-extension/portal-fill":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleBrowserExtensionPortalFill(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/audit":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAuditList(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/config":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAdminConfig(w, r, user)
	case r.Method == http.MethodPut && r.URL.Path == "/api/admin/config":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleUpdateAdminConfig(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/admin/rdp-signing/test-certificate":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleGenerateRDPSigningTestCertificate(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/local-groups":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleListLocalGroups(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/archived-resources":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleListArchivedResources(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/users":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleListUsers(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/admin/users":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleCreateUser(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/notification-deliveries":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAdminNotificationDeliveries(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/admin/local-groups":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleCreateLocalGroup(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/keyvault/discover":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleKeyVaultDiscover(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/keyvault/import":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleKeyVaultImport(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/keyvault/sync":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleKeyVaultSync(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/appregistrations/discover":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAppRegistrationDiscover(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/appregistrations/import":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAppRegistrationImport(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/appregistrations/sync":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAppRegistrationSync(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/me/activity":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleRecentActivity(w, r, user)
	case r.Method == http.MethodGet && r.URL.Path == "/api/me/notifications":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleUserNotifications(w, r, user)
	case strings.HasPrefix(r.URL.Path, "/api/admin/local-groups/"):
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleLocalGroupRoutes(w, r, user)
	case strings.HasPrefix(r.URL.Path, "/api/admin/users/"):
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleAdminUserRoutes(w, r, user)
	case strings.HasPrefix(r.URL.Path, "/api/admin/archived-resources/"):
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleArchivedResourceRoutes(w, r, user)
	case strings.HasPrefix(r.URL.Path, "/api/resources/"):
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleResourceRoutes(w, r, user)
	case strings.HasPrefix(r.URL.Path, "/api/me/notifications/"):
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleUserNotificationRoutes(w, r, user)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

type keyVaultImportInput struct {
	Description   string               `json:"description"`
	Owner         string               `json:"owner"`
	OwnerTeam     string               `json:"ownerTeam"`
	Environment   string               `json:"environment"`
	Notes         string               `json:"notes"`
	AllowedGroups []string             `json:"allowedGroups"`
	Items         []keyVaultImportItem `json:"items"`

	// Backward-compatible single-item fields.
	VaultURL    string     `json:"vaultUrl"`
	VaultName   string     `json:"vaultName"`
	ObjectName  string     `json:"objectName"`
	SecretID    string     `json:"secretId"`
	ContentType string     `json:"contentType"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

type keyVaultImportItem struct {
	VaultURL    string     `json:"vaultUrl"`
	VaultName   string     `json:"vaultName"`
	ObjectName  string     `json:"objectName"`
	SecretID    string     `json:"secretId"`
	ContentType string     `json:"contentType"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Enabled     *bool      `json:"enabled"`
}

type keyVaultSyncInput struct {
	Automatic bool `json:"automatic"`
}

type appRegistrationSyncInput struct {
	Automatic bool `json:"automatic"`
}

type keyVaultSourceView struct {
	Name                 string     `json:"name"`
	VaultURL             string     `json:"vaultUrl"`
	SyncEnabled          bool       `json:"syncEnabled"`
	SyncIntervalMinutes  int        `json:"syncIntervalMinutes"`
	AutoImportEnabled    bool       `json:"autoImportEnabled"`
	DefaultOwner         string     `json:"defaultOwner"`
	DefaultOwnerTeam     string     `json:"defaultOwnerTeam"`
	DefaultEnvironment   string     `json:"defaultEnvironment"`
	DefaultDescription   string     `json:"defaultDescription"`
	DefaultNotes         string     `json:"defaultNotes"`
	DefaultAllowedGroups []string   `json:"defaultAllowedGroups"`
	LastSyncedAt         *time.Time `json:"lastSyncedAt"`
	LastSyncStatus       string     `json:"lastSyncStatus"`
	LastSyncError        string     `json:"lastSyncError"`
	LastSyncSummary      string     `json:"lastSyncSummary"`
}

type browserExtensionPortalURLInput struct {
	URL string `json:"url"`
}

type browserExtensionPortalFillInput struct {
	URL        string `json:"url"`
	ResourceID string `json:"resourceId"`
}

type browserExtensionConnectExchangeInput struct {
	Token          string `json:"token"`
	InstallationID string `json:"installationId"`
}

func requestBearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

type vaultPasskeyInput struct {
	CredentialID string `json:"credentialId"`
	PRFSalt      string `json:"prfSalt"`
	PRFSecret    string `json:"prfSecret"`
}

func (s *Server) writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", s.frontendURL)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	// Defensive headers on the API itself. Responses are JSON, never a
	// document, so the CSP is fully locked down; nosniff stops content-type
	// confusion; HSTS reinforces TLS. The SPA's own CSP lives in nginx.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
}

func queryLimit(r *http.Request, fallback int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

func queryOffset(r *http.Request) int {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	var graphErr appregistrations.RequestError
	if errors.As(err, &graphErr) {
		status = http.StatusBadGateway
		message = graphErr.Message()
		log.Printf("microsoft graph api error: %v", err)
	}
	var keyVaultErr keyvault.RequestError
	if errors.As(err, &keyVaultErr) {
		status = http.StatusBadGateway
		message = keyVaultErr.Error()
		log.Printf("key vault api error: %v", err)
	}
	if errors.Is(err, resources.ErrVaultLocked) || errors.Is(err, auth.ErrVaultLocked) {
		// 423 Locked: the personal vault needs an unlock in this session;
		// the frontend/extension react by starting the unlock flow.
		writeJSON(w, http.StatusLocked, map[string]string{
			"error": "personal vault is locked",
			"code":  "vault_locked",
		})
		return
	}
	if errors.Is(err, resources.ErrForbidden) {
		status = http.StatusForbidden
		message = "forbidden"
	}
	if errors.Is(err, resources.ErrNotFound) {
		status = http.StatusNotFound
		message = "not found"
	}
	if errors.Is(err, resources.ErrInvalidInput) {
		status = http.StatusBadRequest
		message = err.Error()
	}
	if errors.Is(err, auth.ErrInvalidInput) {
		status = http.StatusBadRequest
		message = "invalid input"
	}
	if errors.Is(err, auth.ErrNotFound) {
		status = http.StatusNotFound
		message = "not found"
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		status = http.StatusUnauthorized
		message = "unauthenticated"
	}
	if errors.Is(err, auth.ErrBlocked) {
		status = http.StatusForbidden
		message = "user is blocked"
	}
	if errors.Is(err, auth.ErrLockedOut) {
		status = http.StatusTooManyRequests
		message = "too many failed attempts; try again later"
	}
	if status == http.StatusInternalServerError {
		log.Printf("api internal error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func requireAuth(w http.ResponseWriter, user auth.User, err error) bool {
	if err != nil || user.ID == "" {
		if err != nil {
			writeError(w, err)
			return false
		}
		writeError(w, auth.ErrUnauthenticated)
		return false
	}
	return true
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func adminString(config any, structField string, jsonField string) string {
	if value, ok := config.(map[string]any); ok {
		if raw, ok := value[jsonField].(string); ok {
			return raw
		}
	}

	reflected := reflect.ValueOf(config)
	if reflected.IsValid() && reflected.Kind() == reflect.Struct {
		field := reflected.FieldByName(structField)
		if field.IsValid() && field.Kind() == reflect.String {
			return field.String()
		}
	}
	return ""
}

func adminBool(config any, structField string, jsonField string) bool {
	if value, ok := config.(map[string]any); ok {
		if raw, ok := value[jsonField].(bool); ok {
			return raw
		}
	}

	reflected := reflect.ValueOf(config)
	if reflected.IsValid() && reflected.Kind() == reflect.Struct {
		field := reflected.FieldByName(structField)
		if field.IsValid() && field.Kind() == reflect.Bool {
			return field.Bool()
		}
	}
	return false
}
