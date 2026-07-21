package resources

import "time"

type ResourceType string

const (
	TypeRDP             ResourceType = "rdp"
	TypeSSH             ResourceType = "ssh"
	TypeWebPortal       ResourceType = "web_portal"
	TypeSharedSecret    ResourceType = "shared_secret"
	TypeKeyVaultSecret  ResourceType = "key_vault_secret"
	TypeAppRegistration ResourceType = "app_registration"
)

type SourceKind string

const (
	SourceKindManual               SourceKind = "manual"
	SourceKindAzureKeyVault        SourceKind = "azure_key_vault"
	SourceKindEntraAppRegistration SourceKind = "entra_app_registration"
)

type SecretMode string

const (
	SecretModeInline   SecretMode = "inline"
	SecretModeExternal SecretMode = "external_reference"
	SecretModePrompt   SecretMode = "prompt_on_launch"
)

type NotificationChannel string

const (
	NotificationChannelInApp NotificationChannel = "in_app"
	NotificationChannelEmail NotificationChannel = "email"
)

type AppRegistrationNotificationPolicy struct {
	Enabled      bool                  `json:"enabled"`
	ReminderDays []int                 `json:"reminderDays"`
	Channels     []NotificationChannel `json:"channels"`
}

type Secret struct {
	Mode      SecretMode `json:"mode"`
	Value     string     `json:"-"`
	Reference string     `json:"reference"`
}

type Resource struct {
	ID                            string                             `json:"id"`
	Name                          string                             `json:"name"`
	Type                          ResourceType                       `json:"type"`
	Category                      string                             `json:"category"`
	Personal                      bool                               `json:"personal"`
	Description                   string                             `json:"description"`
	Owner                         string                             `json:"owner"`
	OwnerUserID                   string                             `json:"ownerUserId"`
	OwnerTeam                     string                             `json:"ownerTeam"`
	Environment                   string                             `json:"environment"`
	Status                        string                             `json:"status"`
	FolderPath                    string                             `json:"folderPath"`
	LaunchMode                    string                             `json:"launchMode"`
	SourceKind                    SourceKind                         `json:"sourceKind"`
	SourceObjectID                string                             `json:"sourceObjectId"`
	LastSyncedAt                  *time.Time                         `json:"lastSyncedAt,omitempty"`
	Notes                         string                             `json:"notes"`
	TargetHost                    string                             `json:"targetHost"`
	TargetPort                    *int                               `json:"targetPort,omitempty"`
	TargetURL                     string                             `json:"targetUrl"`
	TargetSystem                  string                             `json:"targetSystem"`
	Username                      string                             `json:"username"`
	ConnectionDomain              string                             `json:"connectionDomain"`
	ConnectionAdminSession        bool                               `json:"connectionAdminSession"`
	ConnectionAutomaticLogon      bool                               `json:"connectionAutomaticLogon"`
	ConnectionWindowMode          string                             `json:"connectionWindowMode"`
	ConnectionUseMultipleMonitors bool                               `json:"connectionUseMultipleMonitors"`
	ConnectionShowConnectionBar   bool                               `json:"connectionShowConnectionBar"`
	ConnectionScreenMode          string                             `json:"connectionScreenMode"`
	ConnectionMacAddress          string                             `json:"connectionMacAddress"`
	ConnectionGatewayHost         string                             `json:"connectionGatewayHost"`
	VaultName                     string                             `json:"vaultName"`
	ObjectName                    string                             `json:"objectName"`
	ObjectType                    string                             `json:"objectType"`
	ObjectVersion                 string                             `json:"objectVersion"`
	ContentType                   string                             `json:"contentType"`
	ExpiresAt                     *time.Time                         `json:"expiresAt,omitempty"`
	Provider                      string                             `json:"provider"`
	ApplicationID                 string                             `json:"applicationId"`
	TenantID                      string                             `json:"tenantId"`
	ClientID                      string                             `json:"clientId"`
	CredentialType                string                             `json:"credentialType"`
	CredentialExpiresAt           *time.Time                         `json:"credentialExpiresAt,omitempty"`
	DisplayNameExternal           string                             `json:"displayNameExternal"`
	LinkedSecretRef               string                             `json:"linkedSecretRef"`
	LaunchAllowed                 bool                               `json:"launchAllowed"`
	RevealAllowed                 bool                               `json:"revealAllowed"`
	CopyAllowed                   bool                               `json:"copyAllowed"`
	AllowedGroups                 []string                           `json:"allowedGroups"`
	Secret                        Secret                             `json:"secret"`
	AppNotificationPolicyOverride *AppRegistrationNotificationPolicy `json:"appNotificationPolicyOverride,omitempty"`
	AppCredentials                []AppRegistrationCredential        `json:"appCredentials,omitempty"`
	AppOwners                     []AppRegistrationOwner             `json:"appOwners,omitempty"`
	CreatedAt                     time.Time                          `json:"createdAt"`
	UpdatedAt                     time.Time                          `json:"updatedAt"`
	ArchivedAt                    *time.Time                         `json:"archivedAt,omitempty"`
}

type AppRegistrationCredential struct {
	ResourceID                 string                             `json:"-"`
	KeyID                      string                             `json:"keyId"`
	CredentialType             string                             `json:"credentialType"`
	DisplayName                string                             `json:"displayName"`
	StartDateTime              *time.Time                         `json:"startDateTime,omitempty"`
	EndDateTime                *time.Time                         `json:"endDateTime,omitempty"`
	Hint                       string                             `json:"hint"`
	Usage                      string                             `json:"usage"`
	LastSyncedAt               *time.Time                         `json:"lastSyncedAt,omitempty"`
	NotificationPolicyOverride *AppRegistrationNotificationPolicy `json:"notificationPolicyOverride,omitempty"`
}

type AppRegistrationOwner struct {
	ResourceID  string `json:"-"`
	OwnerID     string `json:"ownerId"`
	OwnerType   string `json:"ownerType"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type ResourceSummary struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Type                ResourceType `json:"type"`
	Category            string       `json:"category"`
	Personal            bool         `json:"personal"`
	Description         string       `json:"description"`
	Owner               string       `json:"owner"`
	OwnerUserID         string       `json:"ownerUserId"`
	OwnerTeam           string       `json:"ownerTeam"`
	Environment         string       `json:"environment"`
	Status              string       `json:"status"`
	FolderPath          string       `json:"folderPath"`
	LaunchMode          string       `json:"launchMode"`
	SourceKind          SourceKind   `json:"sourceKind"`
	TargetHost          string       `json:"targetHost"`
	TargetPort          *int         `json:"targetPort,omitempty"`
	TargetURL           string       `json:"targetUrl"`
	TargetSystem        string       `json:"targetSystem"`
	Username            string       `json:"username"`
	ConnectionDomain    string       `json:"connectionDomain"`
	VaultName           string       `json:"vaultName"`
	ObjectName          string       `json:"objectName"`
	Provider            string       `json:"provider"`
	ApplicationID       string       `json:"applicationId"`
	CredentialExpiresAt *time.Time   `json:"credentialExpiresAt,omitempty"`
	ExpiresAt           *time.Time   `json:"expiresAt,omitempty"`
	LaunchAllowed       bool         `json:"launchAllowed"`
	RevealAllowed       bool         `json:"revealAllowed"`
	CopyAllowed         bool         `json:"copyAllowed"`
	AllowedGroups       []string     `json:"allowedGroups"`
	CreatedAt           time.Time    `json:"createdAt"`
	UpdatedAt           time.Time    `json:"updatedAt"`
	ArchivedAt          *time.Time   `json:"archivedAt,omitempty"`
}

type ArchivedResourceSummary struct {
	ResourceSummary
	ArchivedBy      string     `json:"archivedBy"`
	ArchivedReason  string     `json:"archivedReason"`
	ArchivedEventAt *time.Time `json:"archivedEventAt,omitempty"`
}

type VisibleResourceSummary struct {
	ResourceSummary
	CategoryAccessRight string   `json:"categoryAccessRight"`
	VisibilityScope     string   `json:"visibilityScope"`
	MatchedLocalGroups  []string `json:"matchedLocalGroups"`
}

type CreateResourceInput struct {
	Name        string       `json:"name"`
	Type        ResourceType `json:"type"`
	Personal    bool         `json:"personal"`
	Description string       `json:"description"`
	Owner       string       `json:"owner"`
	// OwnerUserID is honored only for admins; the service forces it to the
	// caller (create) or the stored owner (update) for everyone else.
	OwnerUserID                   string     `json:"ownerUserId"`
	OwnerTeam                     string     `json:"ownerTeam"`
	Environment                   string     `json:"environment"`
	Status                        string     `json:"status"`
	FolderPath                    string     `json:"folderPath"`
	LaunchMode                    string     `json:"launchMode"`
	SourceKind                    SourceKind `json:"sourceKind"`
	SourceObjectID                string     `json:"sourceObjectId"`
	LastSyncedAt                  *time.Time `json:"lastSyncedAt"`
	Notes                         string     `json:"notes"`
	TargetHost                    string     `json:"targetHost"`
	TargetPort                    *int       `json:"targetPort"`
	TargetURL                     string     `json:"targetUrl"`
	TargetSystem                  string     `json:"targetSystem"`
	Username                      string     `json:"username"`
	ConnectionDomain              string     `json:"connectionDomain"`
	ConnectionAdminSession        bool       `json:"connectionAdminSession"`
	ConnectionAutomaticLogon      bool       `json:"connectionAutomaticLogon"`
	ConnectionWindowMode          string     `json:"connectionWindowMode"`
	ConnectionUseMultipleMonitors bool       `json:"connectionUseMultipleMonitors"`
	ConnectionShowConnectionBar   bool       `json:"connectionShowConnectionBar"`
	ConnectionScreenMode          string     `json:"connectionScreenMode"`
	ConnectionMacAddress          string     `json:"connectionMacAddress"`
	ConnectionGatewayHost         string     `json:"connectionGatewayHost"`
	VaultName                     string     `json:"vaultName"`
	ObjectName                    string     `json:"objectName"`
	ObjectType                    string     `json:"objectType"`
	ObjectVersion                 string     `json:"objectVersion"`
	ContentType                   string     `json:"contentType"`
	ExpiresAt                     *time.Time `json:"expiresAt"`
	Provider                      string     `json:"provider"`
	ApplicationID                 string     `json:"applicationId"`
	TenantID                      string     `json:"tenantId"`
	ClientID                      string     `json:"clientId"`
	CredentialType                string     `json:"credentialType"`
	CredentialExpiresAt           *time.Time `json:"credentialExpiresAt"`
	DisplayNameExternal           string     `json:"displayNameExternal"`
	LinkedSecretRef               string     `json:"linkedSecretRef"`
	LaunchAllowed                 bool       `json:"launchAllowed"`
	RevealAllowed                 bool       `json:"revealAllowed"`
	CopyAllowed                   bool       `json:"copyAllowed"`
	AllowedGroups                 []string   `json:"allowedGroups"`
	SecretMode                    SecretMode `json:"secretMode"`
	SecretValue                   string     `json:"secretValue"`
	SecretReference               string     `json:"secretReference"`
}

type UpdateResourceInput = CreateResourceInput

type ConnectionCredentialOverride struct {
	ConnectionID         string     `json:"connectionId"`
	PasswordResourceID   string     `json:"passwordResourceId"`
	PasswordResourceName string     `json:"passwordResourceName"`
	Username             string     `json:"username"`
	Personal             bool       `json:"personal"`
	UpdatedAt            *time.Time `json:"updatedAt,omitempty"`
}

type ConnectionCredentialOverrideInput struct {
	PasswordResourceID string `json:"passwordResourceId"`
}

type PortalCredentialMatch struct {
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	Username     string `json:"username"`
	TargetURL    string `json:"targetUrl"`
	Personal     bool   `json:"personal"`
	Owner        string `json:"owner"`
	OwnerUserID  string `json:"ownerUserId"`
}

type PortalCredentialFillResult struct {
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	TargetURL    string `json:"targetUrl"`
}

type AppRegistrationCredentialPolicyInput struct {
	KeyID  string                             `json:"keyId"`
	Policy *AppRegistrationNotificationPolicy `json:"policy,omitempty"`
}

type AppRegistrationNotificationPolicyUpdateInput struct {
	ResourcePolicy     *AppRegistrationNotificationPolicy     `json:"resourcePolicy,omitempty"`
	CredentialPolicies []AppRegistrationCredentialPolicyInput `json:"credentialPolicies"`
}

type UserNotification struct {
	ID                    string                `json:"id"`
	UserID                string                `json:"userId"`
	ResourceID            string                `json:"resourceId"`
	ResourceName          string                `json:"resourceName"`
	CredentialKeyID       string                `json:"credentialKeyId"`
	CredentialDisplayName string                `json:"credentialDisplayName"`
	CredentialType        string                `json:"credentialType"`
	CredentialEndDateTime *time.Time            `json:"credentialEndDateTime,omitempty"`
	ReminderDay           int                   `json:"reminderDay"`
	Title                 string                `json:"title"`
	Body                  string                `json:"body"`
	Channels              []NotificationChannel `json:"channels"`
	ReadAt                *time.Time            `json:"readAt,omitempty"`
	EmailStatus           string                `json:"emailStatus"`
	EmailSentAt           *time.Time            `json:"emailSentAt,omitempty"`
	EmailError            string                `json:"emailError"`
	CreatedAt             time.Time             `json:"createdAt"`
}

type NotificationDeliveryRecord struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"userId"`
	UserName              string     `json:"userName"`
	UserEmail             string     `json:"userEmail"`
	ResourceID            string     `json:"resourceId"`
	ResourceName          string     `json:"resourceName"`
	CredentialKeyID       string     `json:"credentialKeyId"`
	CredentialDisplayName string     `json:"credentialDisplayName"`
	CredentialType        string     `json:"credentialType"`
	ReminderDay           int        `json:"reminderDay"`
	Title                 string     `json:"title"`
	EmailStatus           string     `json:"emailStatus"`
	EmailSentAt           *time.Time `json:"emailSentAt,omitempty"`
	EmailError            string     `json:"emailError"`
	CreatedAt             time.Time  `json:"createdAt"`
}

type Filter struct {
	Query string
	Type  string
	Host  string
}

type LaunchPayload struct {
	ResourceID   string         `json:"resourceId"`
	ResourceType ResourceType   `json:"resourceType"`
	Method       string         `json:"method"`
	Target       string         `json:"target"`
	Command      string         `json:"command,omitempty"`
	URL          string         `json:"url,omitempty"`
	Metadata     map[string]any `json:"metadata"`
}

type RevealResult struct {
	ResourceID      string     `json:"resourceId"`
	SecretMode      SecretMode `json:"secretMode"`
	SecretValue     string     `json:"secretValue,omitempty"`
	SecretReference string     `json:"secretReference,omitempty"`
}

type KeyVaultSyncResult struct {
	AttemptedSources  int                  `json:"attemptedSources"`
	ImportedResources int                  `json:"importedResources"`
	UpdatedResources  int                  `json:"updatedResources"`
	RemovedResources  int                  `json:"removedResources"`
	MissingResources  int                  `json:"missingResources"`
	SkippedResources  int                  `json:"skippedResources"`
	Automatic         bool                 `json:"automatic"`
	Sources           []KeyVaultSyncSource `json:"sources"`
}

type AppRegistrationImportInput struct {
	Owner          string   `json:"owner"`
	OwnerTeam      string   `json:"ownerTeam"`
	Environment    string   `json:"environment"`
	TenantID       string   `json:"tenantId"`
	Description    string   `json:"description"`
	Notes          string   `json:"notes"`
	AllowedGroups  []string `json:"allowedGroups"`
	ApplicationIDs []string `json:"applicationIds"`
}

type AppRegistrationSyncResult struct {
	AttemptedResources  int  `json:"attemptedResources"`
	UpdatedResources    int  `json:"updatedResources"`
	RemovedResources    int  `json:"removedResources"`
	MissingResources    int  `json:"missingResources"`
	ExpiringCredentials int  `json:"expiringCredentials"`
	ExpiredCredentials  int  `json:"expiredCredentials"`
	Automatic           bool `json:"automatic"`
}

type KeyVaultSyncSourceConfig struct {
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
	LastSyncedAt         *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncStatus       string     `json:"lastSyncStatus"`
	LastSyncError        string     `json:"lastSyncError"`
	LastSyncSummary      string     `json:"lastSyncSummary"`
}

type KeyVaultSyncSource struct {
	VaultURL        string     `json:"vaultUrl"`
	Name            string     `json:"name"`
	SyncEnabled     bool       `json:"syncEnabled"`
	Due             bool       `json:"due"`
	LastSyncedAt    *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncStatus  string     `json:"lastSyncStatus"`
	LastSyncError   string     `json:"lastSyncError"`
	LastSyncSummary string     `json:"lastSyncSummary"`
	ImportedCount   int        `json:"importedCount"`
	UpdatedCount    int        `json:"updatedCount"`
	RemovedCount    int        `json:"removedCount"`
	MissingCount    int        `json:"missingCount"`
	SkippedCount    int        `json:"skippedCount"`
}

func CategoryForType(resourceType ResourceType) string {
	switch resourceType {
	case TypeSSH, TypeRDP:
		return "connections"
	case TypeKeyVaultSecret:
		return "keyvault"
	case TypeAppRegistration:
		return "appregistrations"
	default:
		return "passwords"
	}
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
