package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/artifacts"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/browserextinfo"
	"access-workspace/backend/internal/keyvault"
	"access-workspace/backend/internal/launcherinfo"
	"access-workspace/backend/internal/resources"
	"github.com/google/uuid"
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
	List(ctx context.Context, limit int) ([]audit.Event, error)
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
	}
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
		s.handleLogout(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/password":
		if !requireAuth(w, user, authErr) {
			return
		}
		s.handleChangePassword(w, r, user)
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/invite/accept":
		s.handleAcceptInvite(w, r)
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

func (s *Server) handleLaunchTicketResolve(w http.ResponseWriter, r *http.Request) {
	if version := strings.TrimSpace(r.Header.Get("X-Access-Workspace-Launcher-Version")); version != "" && version != launcherinfo.RequiredVersion {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error":           "launcher upgrade required",
			"requiredVersion": launcherinfo.RequiredVersion,
		})
		return
	}
	ticket := strings.TrimPrefix(r.URL.Path, "/api/launcher/tickets/")
	ticket, _ = url.PathUnescape(ticket)
	if strings.TrimSpace(ticket) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	result, err := s.resources.ResolveLaunchTicket(r.Context(), ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLauncherRuntime(w http.ResponseWriter, r *http.Request) {
	downloads, err := s.artifacts.LauncherDownloads(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	recommended := ""
	if len(downloads) > 0 {
		recommended = downloads[0].DownloadURL
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requiredVersion": launcherinfo.RequiredVersion,
		"statusUrl":       launcherinfo.StatusURL,
		"launchUrl":       launcherinfo.LaunchURL,
		"downloadUrl":     recommended,
		"downloads":       downloads,
	})
}

func (s *Server) handleBrowserExtensionRuntime(w http.ResponseWriter, r *http.Request) {
	packages, err := s.artifacts.ExtensionPackages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	defaultDownloadURL := ""
	for _, pkg := range packages {
		if pkg.Status == "available" && pkg.DownloadURL != "" {
			defaultDownloadURL = pkg.DownloadURL
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requiredVersion": browserextinfo.RequiredVersion,
		"browser":         "chromium",
		"downloadUrl":     defaultDownloadURL,
		"packages":        packages,
	})
}

// handleArtifactDownload streams an artifact (launcher / browser extension) from
// the private store through the backend, so the store never needs to be reachable
// by the browser. Path: /api/artifacts/download/<category>/<name>.
func (s *Server) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/artifacts/download/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	category := parts[0]
	name, err := url.PathUnescape(parts[1])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	body, info, err := s.artifacts.Open(r.Context(), category, name)
	if err != nil {
		if errors.Is(err, artifacts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "download failed"})
		return
	}
	defer body.Close()

	contentType := "application/octet-stream"
	if info != nil && info.ContentType != "" {
		contentType = info.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	if info != nil && info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (s *Server) handleRDPSigningPublic(w http.ResponseWriter, r *http.Request) {
	config, err := s.adminConfig.GetRDPSigningPublic(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
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

func (s *Server) handleAuthBootstrap(w http.ResponseWriter, r *http.Request) {
	bootstrap := s.authenticator.Bootstrap()
	payload := map[string]any{
		"authMode":           bootstrap.AuthMode,
		"localLoginEnabled":  bootstrap.LocalLoginEnabled,
		"microsoftLoginHint": false,
	}

	if s.adminConfig == nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}

	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	payload["microsoftLoginHint"] = adminBool(configAny, "Enabled", "entraEnabled") &&
		adminBool(configAny, "Configured", "entraConfigured")

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.List(r.Context(), user, resources.Filter{
		Query: r.URL.Query().Get("q"),
		Type:  r.URL.Query().Get("type"),
		Host:  r.URL.Query().Get("host"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input resources.CreateResourceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.resources.Create(r.Context(), user, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handlePasswordOptions(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.ListPasswordOptions(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBrowserExtensionPortalMatch(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input browserExtensionPortalURLInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	items, err := s.resources.ListPortalCredentialMatches(r.Context(), user, input.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBrowserExtensionPortalFill(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input browserExtensionPortalFillInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.resources.FillPortalCredential(r.Context(), user, input.ResourceID, input.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleResourceRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/resources/")
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := s.resources.Get(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodPut:
			var input resources.UpdateResourceInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			item, err := s.resources.Update(r.Context(), user, id, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPut && parts[1] == "app-registration-notifications" {
		var input resources.AppRegistrationNotificationPolicyUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		item, err := s.resources.UpdateAppRegistrationNotificationPolicies(r.Context(), user, id, input)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 2 && parts[1] == "connection-override" {
		switch r.Method {
		case http.MethodGet:
			item, err := s.resources.GetConnectionCredentialOverride(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodPut:
			var input resources.ConnectionCredentialOverrideInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			item, err := s.resources.SetConnectionCredentialOverride(r.Context(), user, id, input)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodDelete:
			if err := s.resources.ClearConnectionCredentialOverride(r.Context(), user, id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "archive":
			if err := s.resources.Archive(r.Context(), user, id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
			return
		case "reveal":
			result, err := s.resources.Reveal(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		case "launch":
			result, err := s.resources.Launch(r.Context(), user, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !auth.CapabilitiesForUser(user).CanViewAudit {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	limit := queryLimit(r, 100)
	items, err := s.audit.List(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRecentActivity(w http.ResponseWriter, r *http.Request, user auth.User) {
	limit := queryLimit(r, 20)
	items, err := s.audit.RecentForUser(r.Context(), user.ID, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUserNotifications(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.notifications == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []resources.UserNotification{}})
		return
	}
	items, err := s.notifications.ListForUser(r.Context(), user.ID, queryLimit(r, 25))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	config, err := s.adminConfig.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleUpdateAdminConfig(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	config, err := s.adminConfig.Update(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) handleGenerateRDPSigningTestCertificate(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	view, err := s.adminConfig.GenerateTestRDPSigningCertificate(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleListLocalGroups(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	items, err := s.localGroups.ListLocalGroups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleListArchivedResources(w http.ResponseWriter, r *http.Request, user auth.User) {
	items, err := s.resources.ListArchived(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	items, err := s.localGroups.ListUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input auth.CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	item, err := s.localGroups.CreateUser(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	if input.Invite {
		invite, err := s.localGroups.IssueUserInvite(r.Context(), user, item.ID, "invite")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user": item, "invite": invite})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.authenticator.ChangeOwnPassword(r.Context(), user, input.CurrentPassword, input.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	_ = s.audit.Log(r.Context(), audit.LogParams{
		EventType: audit.EventUserAccessUpdated,
		UserID:    user.ID,
		UserName:  user.Name,
		Metadata:  map[string]any{"action": "password_changed"},
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.AcceptInvite(r.Context(), input.Token, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminNotificationDeliveries(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if s.notifications == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []resources.NotificationDeliveryRecord{}})
		return
	}
	items, err := s.notifications.ListRecentEmailDeliveries(r.Context(), queryLimit(r, 20))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminUserRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/")
	parts := strings.Split(path, "/")
	id, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil || id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "reset-password" {
		// Destroys the user's vault and personal secrets (unrecoverable by
		// design), kills their sessions, and returns a one-time reset link.
		invite, err := s.localGroups.ResetUserPassword(r.Context(), user, id)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserAccessUpdated,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"action":                   "password_reset_forced",
				"targetUserId":             id,
				"personalResourcesDeleted": invite.PersonalResourcesDeleted,
			},
		})
		writeJSON(w, http.StatusOK, invite)
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "invite" {
		// Re-issues the one-time invite link (invalidates any previous one).
		invite, err := s.localGroups.IssueUserInvite(r.Context(), user, id, "invite")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, invite)
		return
	}

	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "visible-resources" {
		item, err := s.localGroups.GetUserAccess(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		items, err := s.resources.ExplainVisibleResources(r.Context(), previewUserFromAccess(item), resources.Filter{
			Query: r.URL.Query().Get("q"),
			Type:  r.URL.Query().Get("type"),
			Host:  r.URL.Query().Get("host"),
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := s.localGroups.GetUserAccess(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	case http.MethodPut:
		var input auth.UserAccessUpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		item, err := s.localGroups.UpdateUserAccess(r.Context(), id, input)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserAccessUpdated,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"targetUserId":      item.ID,
				"targetUserName":    item.Name,
				"blocked":           item.Blocked,
				"directLocalGroups": item.DirectAssignedLocalGroups,
				"directRights":      item.DirectRights,
			},
		})
		writeJSON(w, http.StatusOK, item)
		return
	case http.MethodDelete:
		if id == user.ID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "you cannot delete your own account"})
			return
		}
		result, err := s.localGroups.DeleteUser(r.Context(), user, id)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = s.audit.Log(r.Context(), audit.LogParams{
			EventType: audit.EventUserDeleted,
			UserID:    user.ID,
			UserName:  user.Name,
			Metadata: map[string]any{
				"targetUserId":              id,
				"personalResourcesDeleted":  result.PersonalResourcesDeleted,
				"sharedResourcesReassigned": result.SharedResourcesReassigned,
			},
		})
		writeJSON(w, http.StatusOK, result)
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleArchivedResourceRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/archived-resources/")
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "restore" {
		if err := s.resources.Restore(r.Context(), user, id); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleUserNotificationRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.notifications == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/notifications/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "read" || r.Method != http.MethodPost {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.notifications.MarkRead(r.Context(), user.ID, parts[0]); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateLocalGroup(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var input auth.LocalGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.localGroups.CreateLocalGroup(r.Context(), input); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleKeyVaultDiscover(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if s.keyVault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "key vault integration is not available"})
		return
	}
	result, err := s.keyVault.Discover(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleKeyVaultImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if s.keyVault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "key vault integration is not available"})
		return
	}

	var input keyVaultImportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	items := input.Items
	if len(items) == 0 && strings.TrimSpace(input.SecretID) != "" {
		items = []keyVaultImportItem{{
			VaultURL:    input.VaultURL,
			VaultName:   input.VaultName,
			ObjectName:  input.ObjectName,
			SecretID:    input.SecretID,
			ContentType: input.ContentType,
			ExpiresAt:   input.ExpiresAt,
		}}
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one Key Vault secret must be selected"})
		return
	}

	imported := make([]resources.Resource, 0, len(items))
	now := time.Now().UTC()
	sharedDescription := strings.TrimSpace(input.Description)
	sharedOwner := strings.TrimSpace(input.Owner)
	sharedOwnerTeam := strings.TrimSpace(input.OwnerTeam)
	sharedEnvironment := strings.TrimSpace(input.Environment)
	sharedNotes := strings.TrimSpace(input.Notes)

	for _, item := range items {
		secretID := strings.TrimSpace(item.SecretID)
		if err := s.keyVault.ValidateReference(r.Context(), secretID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		resource, err := s.resources.Create(r.Context(), user, resources.CreateResourceInput{
			Name:            strings.TrimSpace(item.ObjectName),
			Type:            resources.TypeKeyVaultSecret,
			Description:     sharedDescription,
			Owner:           sharedOwner,
			OwnerTeam:       sharedOwnerTeam,
			Environment:     sharedEnvironment,
			Status:          keyVaultImportStatus(item.Enabled),
			SourceKind:      resources.SourceKindAzureKeyVault,
			SourceObjectID:  secretID,
			LastSyncedAt:    &now,
			Notes:           sharedNotes,
			VaultName:       strings.TrimSpace(item.VaultName),
			ObjectName:      strings.TrimSpace(item.ObjectName),
			ObjectType:      "secret",
			ContentType:     strings.TrimSpace(item.ContentType),
			ExpiresAt:       item.ExpiresAt,
			RevealAllowed:   true,
			CopyAllowed:     true,
			AllowedGroups:   input.AllowedGroups,
			SecretMode:      resources.SecretModeExternal,
			SecretReference: secretID,
			LinkedSecretRef: secretID,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		imported = append(imported, resource)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"items": imported})
}

func keyVaultImportStatus(enabled *bool) string {
	if enabled != nil && !*enabled {
		return "disabled"
	}
	return "active"
}

func (s *Server) handleKeyVaultSync(w http.ResponseWriter, r *http.Request, user auth.User) {
	if s.keyVault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "key vault integration is not available"})
		return
	}

	input := keyVaultSyncInput{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}

	configAny, err := s.adminConfig.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	sources := keyVaultSyncSourcesFromConfig(configAny)
	result, err := s.resources.SyncKeyVault(r.Context(), user, sources, input.Automatic)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.adminConfig.UpdateKeyVaultSyncState(r.Context(), keyVaultSyncStateMap(result.Sources)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAppRegistrationDiscover(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if s.appRegistrations == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app registration integration is not available"})
		return
	}
	result, err := s.appRegistrations.Discover(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAppRegistrationImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input resources.AppRegistrationImportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	items, err := s.resources.ImportAppRegistrations(r.Context(), user, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": items})
}

func (s *Server) handleAppRegistrationSync(w http.ResponseWriter, r *http.Request, user auth.User) {
	input := appRegistrationSyncInput{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}

	result, err := s.resources.SyncAppRegistrations(r.Context(), user, input.Automatic)
	if err != nil {
		_ = s.adminConfig.UpdateAppRegistrationSyncState(r.Context(), map[string]any{
			"lastSyncedAt":   time.Now().UTC(),
			"lastSyncStatus": "error",
			"lastSyncError":  err.Error(),
		})
		writeError(w, err)
		return
	}
	_ = s.adminConfig.UpdateAppRegistrationSyncState(r.Context(), map[string]any{
		"lastSyncedAt":    time.Now().UTC(),
		"lastSyncStatus":  "ok",
		"lastSyncError":   "",
		"lastSyncSummary": summarizeAppRegistrationSyncResult(result),
	})
	writeJSON(w, http.StatusOK, result)
}

func summarizeAppRegistrationSyncResult(result resources.AppRegistrationSyncResult) string {
	if result.AttemptedResources == 0 {
		return "No imported app registrations needed syncing."
	}
	parts := []string{fmt.Sprintf("Updated %d app registrations", result.UpdatedResources)}
	if result.RemovedResources > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d missing apps", result.RemovedResources))
	}
	if result.ExpiringCredentials > 0 {
		parts = append(parts, fmt.Sprintf("%d credentials expiring soon", result.ExpiringCredentials))
	}
	if result.ExpiredCredentials > 0 {
		parts = append(parts, fmt.Sprintf("%d credentials expired", result.ExpiredCredentials))
	}
	if result.MissingResources > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", result.MissingResources))
	}
	return strings.Join(parts, ", ")
}

func keyVaultSyncSourcesFromConfig(config any) []resources.KeyVaultSyncSourceConfig {
	encoded, err := json.Marshal(config)
	if err != nil {
		return []resources.KeyVaultSyncSourceConfig{}
	}
	var payload struct {
		KeyVaultSources []keyVaultSourceView `json:"keyVaultSources"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return []resources.KeyVaultSyncSourceConfig{}
	}
	sources := make([]resources.KeyVaultSyncSourceConfig, 0, len(payload.KeyVaultSources))
	for _, source := range payload.KeyVaultSources {
		sources = append(sources, resources.KeyVaultSyncSourceConfig{
			Name:                 source.Name,
			VaultURL:             source.VaultURL,
			SyncEnabled:          source.SyncEnabled,
			SyncIntervalMinutes:  source.SyncIntervalMinutes,
			AutoImportEnabled:    source.AutoImportEnabled,
			DefaultOwner:         source.DefaultOwner,
			DefaultOwnerTeam:     source.DefaultOwnerTeam,
			DefaultEnvironment:   source.DefaultEnvironment,
			DefaultDescription:   source.DefaultDescription,
			DefaultNotes:         source.DefaultNotes,
			DefaultAllowedGroups: append([]string{}, source.DefaultAllowedGroups...),
			LastSyncedAt:         source.LastSyncedAt,
			LastSyncStatus:       source.LastSyncStatus,
			LastSyncError:        source.LastSyncError,
			LastSyncSummary:      source.LastSyncSummary,
		})
	}
	return sources
}

func keyVaultSyncStateMap(sources []resources.KeyVaultSyncSource) map[string]any {
	out := map[string]any{}
	for _, source := range sources {
		out[source.VaultURL] = map[string]any{
			"lastSyncedAt":    source.LastSyncedAt,
			"lastSyncStatus":  source.LastSyncStatus,
			"lastSyncError":   source.LastSyncError,
			"lastSyncSummary": source.LastSyncSummary,
		}
	}
	return out
}

func (s *Server) handleLocalGroupRoutes(w http.ResponseWriter, r *http.Request, user auth.User) {
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/admin/local-groups/")
	name, _ = url.PathUnescape(name)
	if name == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var input auth.LocalGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.localGroups.UpdateLocalGroup(r.Context(), name, input); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMicrosoftStart(w http.ResponseWriter, r *http.Request) {
	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		log.Printf("microsoft auth start: load config failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=config_load_failed", http.StatusFound)
		return
	}

	enabled := adminBool(configAny, "Enabled", "entraEnabled")
	configured := adminBool(configAny, "Configured", "entraConfigured")
	if !enabled || !configured {
		log.Printf("microsoft auth start: unavailable enabled=%t configured=%t", enabled, configured)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_login_not_available", http.StatusFound)
		return
	}

	authority := strings.TrimRight(adminString(configAny, "Authority", "entraAuthority"), "/")
	tenantID := adminString(configAny, "TenantID", "entraTenantId")
	clientID := adminString(configAny, "ClientID", "entraClientId")
	redirectURI := adminString(configAny, "RedirectURI", "entraRedirectUri")

	if authority == "" || tenantID == "" || clientID == "" || redirectURI == "" {
		log.Printf("microsoft auth start: incomplete config authority=%t tenant=%t client=%t redirect=%t",
			authority != "", tenantID != "", clientID != "", redirectURI != "")
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_login_not_configured", http.StatusFound)
		return
	}

	state := uuid.NewString()
	authURL, err := url.Parse(fmt.Sprintf("%s/%s/oauth2/v2.0/authorize", authority, tenantID))
	if err != nil {
		log.Printf("microsoft auth start: invalid authority %q: %v", authority, err)
		http.Redirect(w, r, s.frontendURL+"?authError=invalid_microsoft_authority", http.StatusFound)
		return
	}

	query := authURL.Query()
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("response_mode", "query")
	query.Set("scope", microsoftScopes(configAny))
	query.Set("state", state)
	authURL.RawQuery = query.Encode()

	http.SetCookie(w, &http.Cookie{
		Name:     "aw_ms_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	log.Printf("microsoft auth start: redirecting to authorize endpoint tenant=%s redirect_uri=%s group_source=%s",
		tenantID, redirectURI, adminString(configAny, "GroupSource", "entraGroupSource"))
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func (s *Server) handleMicrosoftCallback(w http.ResponseWriter, r *http.Request) {
	if errorCode := r.URL.Query().Get("error"); errorCode != "" {
		log.Printf("microsoft auth callback: provider returned error=%s description=%s",
			errorCode, r.URL.Query().Get("error_description"))
		http.Redirect(w, r, s.frontendURL+"?authError="+url.QueryEscape(errorCode), http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie("aw_ms_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != state {
		log.Printf("microsoft auth callback: invalid state cookie_present=%t state_match=%t err=%v",
			err == nil && stateCookie != nil,
			err == nil && stateCookie != nil && stateCookie.Value == state,
			err)
		http.Redirect(w, r, s.frontendURL+"?authError=invalid_microsoft_state", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "aw_ms_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	if code == "" {
		log.Printf("microsoft auth callback: missing authorization code")
		http.Redirect(w, r, s.frontendURL+"?authError=missing_microsoft_code", http.StatusFound)
		return
	}

	configAny, err := s.adminConfig.GetRuntime(r.Context())
	if err != nil {
		log.Printf("microsoft auth callback: load config failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=config_load_failed", http.StatusFound)
		return
	}

	tokens, err := exchangeMicrosoftCode(r.Context(), configAny, code)
	if err != nil {
		log.Printf("microsoft auth callback: token exchange failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_token_exchange_failed", http.StatusFound)
		return
	}

	user, err := resolveMicrosoftUser(r.Context(), configAny, tokens)
	if err != nil {
		log.Printf("microsoft auth callback: user resolution failed: %v", err)
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_user_resolution_failed", http.StatusFound)
		return
	}

	result, err := s.authenticator.IssueSession(r.Context(), user, auth.ModeEntra)
	if err != nil {
		log.Printf("microsoft auth callback: session creation failed for user=%s: %v", user.ID, err)
		if errors.Is(err, auth.ErrBlocked) {
			http.Redirect(w, r, s.frontendURL+"?authError=user_blocked", http.StatusFound)
			return
		}
		http.Redirect(w, r, s.frontendURL+"?authError=microsoft_session_failed", http.StatusFound)
		return
	}

	log.Printf("microsoft auth callback: signed in user=%s email=%s groups=%d", user.ID, user.Email, len(user.Groups))
	http.Redirect(w, r, s.frontendURL+"?authToken="+url.QueryEscape(result.Token), http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if err := s.authenticator.Logout(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

func (s *Server) handleBrowserExtensionSession(w http.ResponseWriter, r *http.Request, user auth.User) {
	result, err := s.authenticator.IssueSession(r.Context(), user, s.authenticator.Bootstrap().AuthMode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        result.Token,
		"user":         result.User,
		"authMode":     result.AuthMode,
		"capabilities": result.Capabilities,
	})
}

func (s *Server) handleBrowserExtensionConnectToken(w http.ResponseWriter, r *http.Request, user auth.User) {
	result, err := s.authenticator.IssueBrowserExtensionConnectToken(r.Context(), user, s.authenticator.Bootstrap().AuthMode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleBrowserExtensionConnectExchange(w http.ResponseWriter, r *http.Request) {
	var input browserExtensionConnectExchangeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.authenticator.ExchangeBrowserExtensionConnectToken(r.Context(), input.Token, input.InstallationID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        result.Token,
		"user":         result.User,
		"authMode":     result.AuthMode,
		"capabilities": result.Capabilities,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.authenticator.CurrentUser(r.Context(), r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         user,
		"authMode":     s.authenticator.Bootstrap().AuthMode,
		"capabilities": auth.CapabilitiesForUser(user),
	})
}

func (s *Server) writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", s.frontendURL)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
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
	if errors.Is(err, resources.ErrVaultLocked) {
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

func previewUserFromAccess(item auth.UserAccessDetail) auth.User {
	localGroups := make([]string, 0, len(item.ResolvedLocalGroups))
	for _, group := range item.ResolvedLocalGroups {
		localGroups = append(localGroups, group.Name)
	}
	return auth.User{
		ID:          item.ID,
		Name:        item.Name,
		Email:       item.Email,
		LocalGroups: localGroups,
		Rights:      append([]string{}, item.Rights...),
		IsAdmin:     item.IsAdmin,
		Blocked:     item.Blocked,
	}
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
