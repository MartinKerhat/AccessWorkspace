package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"access-workspace/backend/internal/notifications"
	"access-workspace/backend/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminConfigView struct {
	AuthMode                           string                                      `json:"authMode"`
	EntraTenantID                      string                                      `json:"entraTenantId"`
	EntraClientID                      string                                      `json:"entraClientId"`
	EntraAuthority                     string                                      `json:"entraAuthority"`
	EntraRedirectURI                   string                                      `json:"entraRedirectUri"`
	EntraGroupSource                   string                                      `json:"entraGroupSource"`
	EntraClientSecretSet               bool                                        `json:"entraClientSecretSet"`
	EntraConfigured                    bool                                        `json:"entraConfigured"`
	EntraEnabled                       bool                                        `json:"entraEnabled"`
	KeyVaultSources                    []KeyVaultSource                            `json:"keyVaultSources"`
	KeyVaultSourceCount                int                                         `json:"keyVaultSourceCount"`
	LocalGroupCount                    int                                         `json:"localGroupCount"`
	DirectRightsRuleCount              int                                         `json:"directRightsRuleCount"`
	AppRegistrationNotificationPolicy  resources.AppRegistrationNotificationPolicy `json:"appRegistrationNotificationPolicy"`
	NotificationEmailEnabled           bool                                        `json:"notificationEmailEnabled"`
	NotificationEmailHost              string                                      `json:"notificationEmailHost"`
	NotificationEmailPort              int                                         `json:"notificationEmailPort"`
	NotificationEmailUsername          string                                      `json:"notificationEmailUsername"`
	NotificationEmailPasswordSet       bool                                        `json:"notificationEmailPasswordSet"`
	NotificationEmailFrom              string                                      `json:"notificationEmailFrom"`
	NotificationEmailConfigured        bool                                        `json:"notificationEmailConfigured"`
	AppRegistrationAutoSyncEnabled     bool                                        `json:"appRegistrationAutoSyncEnabled"`
	AppRegistrationSyncIntervalMinutes int                                         `json:"appRegistrationSyncIntervalMinutes"`
	AppRegistrationLastSyncedAt        *time.Time                                  `json:"appRegistrationLastSyncedAt,omitempty"`
	AppRegistrationLastSyncStatus      string                                      `json:"appRegistrationLastSyncStatus"`
	AppRegistrationLastSyncError       string                                      `json:"appRegistrationLastSyncError"`
	AppRegistrationLastSyncSummary     string                                      `json:"appRegistrationLastSyncSummary"`
	RDPSigning                         RDPSigningConfigView                        `json:"rdpSigning"`
}

type KeyVaultSource struct {
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

type UpdateAdminConfigInput struct {
	EntraTenantID                      string                                      `json:"entraTenantId"`
	EntraClientID                      string                                      `json:"entraClientId"`
	EntraAuthority                     string                                      `json:"entraAuthority"`
	EntraRedirectURI                   string                                      `json:"entraRedirectUri"`
	EntraGroupSource                   string                                      `json:"entraGroupSource"`
	EntraClientSecret                  string                                      `json:"entraClientSecret"`
	EntraEnabled                       bool                                        `json:"entraEnabled"`
	KeyVaultSources                    []KeyVaultSource                            `json:"keyVaultSources"`
	AppRegistrationNotificationPolicy  resources.AppRegistrationNotificationPolicy `json:"appRegistrationNotificationPolicy"`
	NotificationEmailEnabled           bool                                        `json:"notificationEmailEnabled"`
	NotificationEmailHost              string                                      `json:"notificationEmailHost"`
	NotificationEmailPort              int                                         `json:"notificationEmailPort"`
	NotificationEmailUsername          string                                      `json:"notificationEmailUsername"`
	NotificationEmailPassword          string                                      `json:"notificationEmailPassword"`
	NotificationEmailFrom              string                                      `json:"notificationEmailFrom"`
	AppRegistrationAutoSyncEnabled     bool                                        `json:"appRegistrationAutoSyncEnabled"`
	AppRegistrationSyncIntervalMinutes int                                         `json:"appRegistrationSyncIntervalMinutes"`
	RDPSigningEnabled                  bool                                        `json:"rdpSigningEnabled"`
}

type KeyVaultSyncState struct {
	LastSyncedAt    *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncStatus  string     `json:"lastSyncStatus,omitempty"`
	LastSyncError   string     `json:"lastSyncError,omitempty"`
	LastSyncSummary string     `json:"lastSyncSummary,omitempty"`
}

type AppRegistrationSyncState struct {
	LastSyncedAt    *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncStatus  string     `json:"lastSyncStatus,omitempty"`
	LastSyncError   string     `json:"lastSyncError,omitempty"`
	LastSyncSummary string     `json:"lastSyncSummary,omitempty"`
}

type AppRegistrationSyncSettings struct {
	Enabled         bool
	IntervalMinutes int
	State           AppRegistrationSyncState
}

type EntraRuntimeConfig struct {
	Enabled      bool
	TenantID     string
	ClientID     string
	Authority    string
	RedirectURI  string
	GroupSource  string
	ClientSecret string
	Configured   bool
}

type AdminConfigStore struct {
	db       *pgxpool.Pool
	defaults Config
	cipher   *resources.SecretCipher
}

// sensitiveSettingKeys lists admin_settings values that are encrypted at rest
// with the resource secret cipher. Every read goes through loadSettings, which
// transparently decrypts them; every write must encrypt via encryptSetting.
var sensitiveSettingKeys = map[string]bool{
	"entra_client_secret":         true,
	"notification_email_password": true,
	"rdp_signing_pfx_base64":      true,
	"rdp_signing_pfx_password":    true,
}

func NewAdminConfigStore(db *pgxpool.Pool, defaults Config, cipher *resources.SecretCipher) *AdminConfigStore {
	return &AdminConfigStore{db: db, defaults: defaults, cipher: cipher}
}

func (c Config) AdminView() AdminConfigView {
	return AdminConfigView{
		AuthMode:                           c.AuthMode,
		EntraTenantID:                      c.EntraTenantID,
		EntraClientID:                      c.EntraClientID,
		EntraAuthority:                     c.EntraAuthority,
		EntraRedirectURI:                   c.EntraRedirectURI,
		EntraGroupSource:                   c.EntraGroupSource,
		EntraClientSecretSet:               c.EntraClientSecret != "",
		EntraConfigured:                    c.EntraTenantID != "" && c.EntraClientID != "" && c.EntraRedirectURI != "" && c.EntraClientSecret != "",
		EntraEnabled:                       false,
		KeyVaultSources:                    []KeyVaultSource{},
		AppRegistrationNotificationPolicy:  defaultAppRegistrationNotificationPolicy(),
		NotificationEmailPort:              587,
		AppRegistrationAutoSyncEnabled:     true,
		AppRegistrationSyncIntervalMinutes: 60,
		RDPSigning: RDPSigningConfigView{
			Enabled:               false,
			CertificateConfigured: false,
		},
	}
}

func (c Config) EntraRuntimeConfig() EntraRuntimeConfig {
	return EntraRuntimeConfig{
		Enabled:      false,
		TenantID:     c.EntraTenantID,
		ClientID:     c.EntraClientID,
		Authority:    c.EntraAuthority,
		RedirectURI:  c.EntraRedirectURI,
		GroupSource:  c.EntraGroupSource,
		ClientSecret: c.EntraClientSecret,
		Configured:   c.EntraTenantID != "" && c.EntraClientID != "" && c.EntraRedirectURI != "" && c.EntraClientSecret != "",
	}
}

func (s *AdminConfigStore) loadSettings(ctx context.Context) (map[string]string, error) {
	settings := map[string]string{}
	rows, err := s.db.Query(ctx, `select key, value from admin_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for key := range sensitiveSettingKeys {
		value, ok := settings[key]
		if !ok {
			continue
		}
		plain, err := s.cipher.DecryptFromStorage(value)
		if err != nil {
			return nil, fmt.Errorf("decrypt admin setting %q: %w", key, err)
		}
		settings[key] = plain
	}

	return settings, nil
}

func (s *AdminConfigStore) encryptSetting(key, value string) (string, error) {
	if !sensitiveSettingKeys[key] {
		return value, nil
	}
	encrypted, err := s.cipher.EncryptForStorage(value)
	if err != nil {
		return "", fmt.Errorf("encrypt admin setting %q: %w", key, err)
	}
	return encrypted, nil
}

// EncryptPlaintextSecretSettings re-encrypts sensitive admin settings written
// before values were encrypted at rest. Runs at startup; rows that already
// carry the encryption envelope are left untouched.
func (s *AdminConfigStore) EncryptPlaintextSecretSettings(ctx context.Context) error {
	keys := make([]string, 0, len(sensitiveSettingKeys))
	for key := range sensitiveSettingKeys {
		keys = append(keys, key)
	}
	rows, err := s.db.Query(ctx, `select key, value from admin_settings where key = any($1)`, keys)
	if err != nil {
		return err
	}
	pending := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return err
		}
		if strings.TrimSpace(value) == "" || resources.IsEncryptedForStorage(value) {
			continue
		}
		pending[key] = value
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for key, value := range pending {
		encrypted, err := s.cipher.EncryptForStorage(value)
		if err != nil {
			return fmt.Errorf("encrypt admin setting %q: %w", key, err)
		}
		// Guard on the old value so a concurrent update is not clobbered.
		if _, err := s.db.Exec(ctx, `
			update admin_settings
			set value = $2, updated_at = now()
			where key = $1 and value = $3
		`, key, encrypted, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *AdminConfigStore) Get(ctx context.Context) (any, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}

	view := s.defaults.AdminView()
	if value := settings["entra_tenant_id"]; value != "" {
		view.EntraTenantID = value
	}
	if value := settings["entra_client_id"]; value != "" {
		view.EntraClientID = value
	}
	if value := settings["entra_authority"]; value != "" {
		view.EntraAuthority = value
	}
	if value := settings["entra_redirect_uri"]; value != "" {
		view.EntraRedirectURI = value
	}
	if value := settings["entra_group_source"]; value != "" {
		view.EntraGroupSource = value
	}
	if value := settings["entra_client_secret"]; value != "" {
		view.EntraClientSecretSet = true
	}
	if settings["entra_enabled"] == "true" {
		view.EntraEnabled = true
	}
	view.KeyVaultSources = mergeKeyVaultSyncState(
		parseKeyVaultSources(settings["keyvault_sources_json"]),
		parseKeyVaultSyncStates(settings["keyvault_sync_state_json"]),
	)
	view.AppRegistrationNotificationPolicy = parseNotificationPolicy(settings["app_registration_notification_policy_json"])
	if value := strings.TrimSpace(settings["notification_email_host"]); value != "" {
		view.NotificationEmailHost = value
	}
	if value := strings.TrimSpace(settings["notification_email_username"]); value != "" {
		view.NotificationEmailUsername = value
	}
	if value := strings.TrimSpace(settings["notification_email_from"]); value != "" {
		view.NotificationEmailFrom = value
	}
	if value := strings.TrimSpace(settings["notification_email_password"]); value != "" {
		view.NotificationEmailPasswordSet = true
	}
	view.NotificationEmailEnabled = settings["notification_email_enabled"] == "true"
	if value := strings.TrimSpace(settings["notification_email_port"]); value != "" {
		if port, err := parseIntSetting(value, 587); err == nil {
			view.NotificationEmailPort = port
		}
	}
	if view.NotificationEmailPort <= 0 {
		view.NotificationEmailPort = 587
	}
	view.NotificationEmailConfigured = view.NotificationEmailHost != "" && view.NotificationEmailFrom != ""
	view.AppRegistrationAutoSyncEnabled = settings["app_registration_sync_enabled"] != "false"
	if value := strings.TrimSpace(settings["app_registration_sync_interval_minutes"]); value != "" {
		if interval, err := parseIntSetting(value, 60); err == nil {
			view.AppRegistrationSyncIntervalMinutes = interval
		}
	}
	if view.AppRegistrationSyncIntervalMinutes <= 0 {
		view.AppRegistrationSyncIntervalMinutes = 60
	}
	appSyncState := parseAppRegistrationSyncState(settings["app_registration_sync_state_json"])
	view.AppRegistrationLastSyncedAt = appSyncState.LastSyncedAt
	view.AppRegistrationLastSyncStatus = appSyncState.LastSyncStatus
	view.AppRegistrationLastSyncError = appSyncState.LastSyncError
	view.AppRegistrationLastSyncSummary = appSyncState.LastSyncSummary
	view.RDPSigning = rdpSigningViewFromSettings(settings)
	view.KeyVaultSourceCount = len(view.KeyVaultSources)
	view.EntraConfigured = view.EntraTenantID != "" && view.EntraClientID != "" && view.EntraRedirectURI != "" && view.EntraClientSecretSet
	view.DirectRightsRuleCount = countDirectRightsRules(settings["entra_direct_rights_json"], s.defaults.EntraDirectRights)
	if err := s.db.QueryRow(ctx, `select count(*) from local_groups`).Scan(&view.LocalGroupCount); err != nil {
		return nil, err
	}
	return view, nil
}

func (s *AdminConfigStore) GetRuntime(ctx context.Context) (any, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}

	config := s.defaults.EntraRuntimeConfig()
	if value := settings["entra_tenant_id"]; value != "" {
		config.TenantID = value
	}
	if value := settings["entra_client_id"]; value != "" {
		config.ClientID = value
	}
	if value := settings["entra_authority"]; value != "" {
		config.Authority = value
	}
	if value := settings["entra_redirect_uri"]; value != "" {
		config.RedirectURI = value
	}
	if value := settings["entra_group_source"]; value != "" {
		config.GroupSource = value
	}
	if value := settings["entra_client_secret"]; value != "" {
		config.ClientSecret = value
	}
	if settings["entra_enabled"] == "true" {
		config.Enabled = true
	}
	config.Configured = config.TenantID != "" && config.ClientID != "" && config.RedirectURI != "" && config.ClientSecret != ""
	return config, nil
}

func (s *AdminConfigStore) GetNotificationEmailRuntime(ctx context.Context) (notifications.NotificationEmailRuntimeConfig, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return notifications.NotificationEmailRuntimeConfig{}, err
	}

	config := notifications.NotificationEmailRuntimeConfig{
		Enabled:  settings["notification_email_enabled"] == "true",
		Host:     strings.TrimSpace(settings["notification_email_host"]),
		Port:     587,
		Username: strings.TrimSpace(settings["notification_email_username"]),
		Password: strings.TrimSpace(settings["notification_email_password"]),
		From:     strings.TrimSpace(settings["notification_email_from"]),
	}
	if value := strings.TrimSpace(settings["notification_email_port"]); value != "" {
		if port, err := parseIntSetting(value, 587); err == nil {
			config.Port = port
		}
	}
	if config.Port <= 0 {
		config.Port = 587
	}
	config.Configured = config.Host != "" && config.From != ""
	return config, nil
}

func (s *AdminConfigStore) GetAppRegistrationNotificationPolicy(ctx context.Context) (resources.AppRegistrationNotificationPolicy, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return resources.AppRegistrationNotificationPolicy{}, err
	}
	return parseNotificationPolicy(settings["app_registration_notification_policy_json"]), nil
}

func (s *AdminConfigStore) GetEntraRuntime(ctx context.Context) (EntraRuntimeConfig, error) {
	runtime, err := s.GetRuntime(ctx)
	if err != nil {
		return EntraRuntimeConfig{}, err
	}
	config, ok := runtime.(EntraRuntimeConfig)
	if !ok {
		return EntraRuntimeConfig{}, nil
	}
	return config, nil
}

func (s *AdminConfigStore) ListKeyVaultSources(ctx context.Context) ([]KeyVaultSource, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	return mergeKeyVaultSyncState(
		parseKeyVaultSources(settings["keyvault_sources_json"]),
		parseKeyVaultSyncStates(settings["keyvault_sync_state_json"]),
	), nil
}

func (s *AdminConfigStore) UpdateKeyVaultSyncState(ctx context.Context, payload any) error {
	states := map[string]KeyVaultSyncState{}
	switch value := payload.(type) {
	case map[string]KeyVaultSyncState:
		states = value
	case map[string]any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &states); err != nil {
			return err
		}
	default:
		return nil
	}
	encoded, err := json.Marshal(normalizeKeyVaultSyncStates(states))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into admin_settings (key, value, updated_at)
		values ('keyvault_sync_state_json', $1, now())
		on conflict (key) do update
		set value = excluded.value, updated_at = now()
	`, string(encoded))
	return err
}

func (s *AdminConfigStore) GetAppRegistrationSyncSettings(ctx context.Context) (AppRegistrationSyncSettings, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return AppRegistrationSyncSettings{}, err
	}
	config := AppRegistrationSyncSettings{
		Enabled:         settings["app_registration_sync_enabled"] != "false",
		IntervalMinutes: 60,
		State:           parseAppRegistrationSyncState(settings["app_registration_sync_state_json"]),
	}
	if value := strings.TrimSpace(settings["app_registration_sync_interval_minutes"]); value != "" {
		if interval, err := parseIntSetting(value, 60); err == nil {
			config.IntervalMinutes = interval
		}
	}
	if config.IntervalMinutes <= 0 {
		config.IntervalMinutes = 60
	}
	return config, nil
}

func (s *AdminConfigStore) UpdateAppRegistrationSyncState(ctx context.Context, payload any) error {
	state := AppRegistrationSyncState{}
	switch value := payload.(type) {
	case AppRegistrationSyncState:
		state = value
	case map[string]any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &state); err != nil {
			return err
		}
	default:
		return nil
	}
	encoded, err := json.Marshal(normalizeAppRegistrationSyncState(state))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into admin_settings (key, value, updated_at)
		values ('app_registration_sync_state_json', $1, now())
		on conflict (key) do update
		set value = excluded.value, updated_at = now()
	`, string(encoded))
	return err
}

func (s *AdminConfigStore) Update(ctx context.Context, payload any) (any, error) {
	input := UpdateAdminConfigInput{}
	items := map[string]string{}
	if values, ok := payload.(map[string]any); ok {
		if value, ok := values["entraTenantId"].(string); ok {
			input.EntraTenantID = value
			items["entra_tenant_id"] = value
		}
		if value, ok := values["entraClientId"].(string); ok {
			input.EntraClientID = value
			items["entra_client_id"] = value
		}
		if value, ok := values["entraAuthority"].(string); ok {
			input.EntraAuthority = value
			items["entra_authority"] = value
		}
		if value, ok := values["entraRedirectUri"].(string); ok {
			input.EntraRedirectURI = value
			items["entra_redirect_uri"] = value
		}
		if value, ok := values["entraGroupSource"].(string); ok {
			input.EntraGroupSource = value
			items["entra_group_source"] = value
		}
		if value, ok := values["entraClientSecret"].(string); ok {
			input.EntraClientSecret = value
		}
		if value, ok := values["entraEnabled"].(bool); ok {
			input.EntraEnabled = value
			items["entra_enabled"] = "false"
			if value {
				items["entra_enabled"] = "true"
			}
		}
		if value, ok := values["keyVaultSources"].([]any); ok {
			input.KeyVaultSources = parseKeyVaultSourcesFromAny(value)
			encoded, err := json.Marshal(stripKeyVaultSyncState(normalizeKeyVaultSources(input.KeyVaultSources)))
			if err != nil {
				return AdminConfigView{}, err
			}
			items["keyvault_sources_json"] = string(encoded)
		}
		if value, ok := values["appRegistrationNotificationPolicy"].(map[string]any); ok {
			encoded, err := json.Marshal(normalizeNotificationPolicy(notificationPolicyFromAny(value)))
			if err != nil {
				return AdminConfigView{}, err
			}
			items["app_registration_notification_policy_json"] = string(encoded)
		}
		if value, ok := values["notificationEmailEnabled"].(bool); ok {
			input.NotificationEmailEnabled = value
			items["notification_email_enabled"] = "false"
			if value {
				items["notification_email_enabled"] = "true"
			}
		}
		if value, ok := values["notificationEmailHost"].(string); ok {
			input.NotificationEmailHost = strings.TrimSpace(value)
			items["notification_email_host"] = input.NotificationEmailHost
		}
		if value, ok := values["notificationEmailUsername"].(string); ok {
			input.NotificationEmailUsername = strings.TrimSpace(value)
			items["notification_email_username"] = input.NotificationEmailUsername
		}
		if value, ok := values["notificationEmailFrom"].(string); ok {
			input.NotificationEmailFrom = strings.TrimSpace(value)
			items["notification_email_from"] = input.NotificationEmailFrom
		}
		if value, ok := values["notificationEmailPort"].(float64); ok {
			input.NotificationEmailPort = int(value)
			items["notification_email_port"] = strconv.Itoa(input.NotificationEmailPort)
		}
		if value, ok := values["notificationEmailPassword"].(string); ok {
			input.NotificationEmailPassword = strings.TrimSpace(value)
		}
		if value, ok := values["appRegistrationAutoSyncEnabled"].(bool); ok {
			input.AppRegistrationAutoSyncEnabled = value
			items["app_registration_sync_enabled"] = "false"
			if value {
				items["app_registration_sync_enabled"] = "true"
			}
		}
		if value, ok := values["appRegistrationSyncIntervalMinutes"].(float64); ok {
			input.AppRegistrationSyncIntervalMinutes = int(value)
			items["app_registration_sync_interval_minutes"] = strconv.Itoa(input.AppRegistrationSyncIntervalMinutes)
		}
		if value, ok := values["rdpSigningEnabled"].(bool); ok {
			input.RDPSigningEnabled = value
			items["rdp_signing_enabled"] = "false"
			if value {
				items["rdp_signing_enabled"] = "true"
			}
		}
	}

	for key, value := range items {
		_, err := s.db.Exec(ctx, `
			insert into admin_settings (key, value, updated_at)
			values ($1, $2, now())
			on conflict (key) do update
			set value = excluded.value, updated_at = now()
		`, key, value)
		if err != nil {
			return AdminConfigView{}, err
		}
	}

	if input.EntraClientSecret != "" {
		encrypted, err := s.encryptSetting("entra_client_secret", input.EntraClientSecret)
		if err != nil {
			return AdminConfigView{}, err
		}
		_, err = s.db.Exec(ctx, `
			insert into admin_settings (key, value, updated_at)
			values ('entra_client_secret', $1, now())
			on conflict (key) do update
			set value = excluded.value, updated_at = now()
		`, encrypted)
		if err != nil {
			return AdminConfigView{}, err
		}
	}
	if input.NotificationEmailPassword != "" {
		encrypted, err := s.encryptSetting("notification_email_password", input.NotificationEmailPassword)
		if err != nil {
			return AdminConfigView{}, err
		}
		_, err = s.db.Exec(ctx, `
			insert into admin_settings (key, value, updated_at)
			values ('notification_email_password', $1, now())
			on conflict (key) do update
			set value = excluded.value, updated_at = now()
		`, encrypted)
		if err != nil {
			return AdminConfigView{}, err
		}
	}

	return s.Get(ctx)
}

func defaultAppRegistrationNotificationPolicy() resources.AppRegistrationNotificationPolicy {
	return resources.AppRegistrationNotificationPolicy{
		Enabled:      true,
		ReminderDays: []int{30, 14, 7, 3, 1, 0},
		Channels:     []resources.NotificationChannel{resources.NotificationChannelInApp},
	}
}

func parseNotificationPolicy(raw string) resources.AppRegistrationNotificationPolicy {
	if strings.TrimSpace(raw) == "" {
		return defaultAppRegistrationNotificationPolicy()
	}
	var parsed resources.AppRegistrationNotificationPolicy
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return defaultAppRegistrationNotificationPolicy()
	}
	return normalizeNotificationPolicy(parsed)
}

func notificationPolicyFromAny(value map[string]any) resources.AppRegistrationNotificationPolicy {
	policy := defaultAppRegistrationNotificationPolicy()
	if enabled, ok := value["enabled"].(bool); ok {
		policy.Enabled = enabled
	}
	if reminderDays, ok := value["reminderDays"].([]any); ok {
		policy.ReminderDays = make([]int, 0, len(reminderDays))
		for _, item := range reminderDays {
			if number, ok := item.(float64); ok {
				policy.ReminderDays = append(policy.ReminderDays, int(number))
			}
		}
	}
	if channels, ok := value["channels"].([]any); ok {
		policy.Channels = make([]resources.NotificationChannel, 0, len(channels))
		for _, item := range channels {
			channel, ok := item.(string)
			if !ok {
				continue
			}
			policy.Channels = append(policy.Channels, resources.NotificationChannel(strings.TrimSpace(channel)))
		}
	}
	return normalizeNotificationPolicy(policy)
}

func normalizeNotificationPolicy(policy resources.AppRegistrationNotificationPolicy) resources.AppRegistrationNotificationPolicy {
	if len(policy.ReminderDays) == 0 {
		policy.ReminderDays = defaultAppRegistrationNotificationPolicy().ReminderDays
	}
	seenDays := map[int]struct{}{}
	normalizedDays := make([]int, 0, len(policy.ReminderDays))
	for _, day := range policy.ReminderDays {
		if day < 0 || day > 365 {
			continue
		}
		if _, ok := seenDays[day]; ok {
			continue
		}
		seenDays[day] = struct{}{}
		normalizedDays = append(normalizedDays, day)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(normalizedDays)))
	policy.ReminderDays = normalizedDays
	if len(policy.Channels) == 0 {
		policy.Channels = []resources.NotificationChannel{resources.NotificationChannelInApp}
	}
	seenChannels := map[resources.NotificationChannel]struct{}{}
	normalizedChannels := make([]resources.NotificationChannel, 0, len(policy.Channels))
	for _, channel := range policy.Channels {
		switch channel {
		case resources.NotificationChannelInApp, resources.NotificationChannelEmail:
		default:
			continue
		}
		if _, ok := seenChannels[channel]; ok {
			continue
		}
		seenChannels[channel] = struct{}{}
		normalizedChannels = append(normalizedChannels, channel)
	}
	if len(normalizedChannels) == 0 {
		normalizedChannels = []resources.NotificationChannel{resources.NotificationChannelInApp}
	}
	policy.Channels = normalizedChannels
	return policy
}

func parseIntSetting(value string, fallback int) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback, err
	}
	return parsed, nil
}

func parseAppRegistrationSyncState(raw string) AppRegistrationSyncState {
	if strings.TrimSpace(raw) == "" {
		return AppRegistrationSyncState{}
	}
	var parsed AppRegistrationSyncState
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return AppRegistrationSyncState{}
	}
	return normalizeAppRegistrationSyncState(parsed)
}

func normalizeAppRegistrationSyncState(value AppRegistrationSyncState) AppRegistrationSyncState {
	value.LastSyncStatus = strings.TrimSpace(value.LastSyncStatus)
	value.LastSyncError = strings.TrimSpace(value.LastSyncError)
	value.LastSyncSummary = strings.TrimSpace(value.LastSyncSummary)
	return value
}

func countDirectRightsRules(stored string, fallback string) int {
	value := stored
	if value == "" {
		value = fallback
	}
	if value == "" {
		return 0
	}

	var parsed map[string][]string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return 0
	}
	return len(parsed)
}

func parseKeyVaultSources(raw string) []KeyVaultSource {
	if strings.TrimSpace(raw) == "" {
		return []KeyVaultSource{}
	}

	var parsed []KeyVaultSource
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []KeyVaultSource{}
	}
	return normalizeKeyVaultSources(parsed)
}

func parseKeyVaultSourcesFromAny(values []any) []KeyVaultSource {
	sources := make([]KeyVaultSource, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		source := KeyVaultSource{SyncEnabled: true, SyncIntervalMinutes: 60}
		if raw, ok := item["name"].(string); ok {
			source.Name = raw
		}
		if raw, ok := item["vaultUrl"].(string); ok {
			source.VaultURL = raw
		}
		if raw, ok := item["syncEnabled"].(bool); ok {
			source.SyncEnabled = raw
		}
		if raw, ok := item["syncIntervalMinutes"].(float64); ok {
			source.SyncIntervalMinutes = int(raw)
		}
		if raw, ok := item["autoImportEnabled"].(bool); ok {
			source.AutoImportEnabled = raw
		}
		if raw, ok := item["defaultOwner"].(string); ok {
			source.DefaultOwner = raw
		}
		if raw, ok := item["defaultOwnerTeam"].(string); ok {
			source.DefaultOwnerTeam = raw
		}
		if raw, ok := item["defaultEnvironment"].(string); ok {
			source.DefaultEnvironment = raw
		}
		if raw, ok := item["defaultDescription"].(string); ok {
			source.DefaultDescription = raw
		}
		if raw, ok := item["defaultNotes"].(string); ok {
			source.DefaultNotes = raw
		}
		if raw, ok := item["defaultAllowedGroups"].([]any); ok {
			source.DefaultAllowedGroups = parseStringListFromAny(raw)
		}
		sources = append(sources, source)
	}
	return normalizeKeyVaultSources(sources)
}

func parseStringListFromAny(values []any) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		items = append(items, raw)
	}
	return normalizeStringSlice(items)
}

func normalizeKeyVaultSources(values []KeyVaultSource) []KeyVaultSource {
	seen := map[string]struct{}{}
	out := make([]KeyVaultSource, 0, len(values))
	for _, value := range values {
		originalInterval := value.SyncIntervalMinutes
		value.Name = strings.TrimSpace(value.Name)
		value.VaultURL = strings.TrimRight(strings.TrimSpace(value.VaultURL), "/")
		if value.SyncIntervalMinutes <= 0 {
			value.SyncIntervalMinutes = 60
		}
		if !value.SyncEnabled && originalInterval <= 0 {
			value.SyncEnabled = true
		}
		value.DefaultOwner = strings.TrimSpace(value.DefaultOwner)
		value.DefaultOwnerTeam = strings.TrimSpace(value.DefaultOwnerTeam)
		value.DefaultEnvironment = strings.TrimSpace(value.DefaultEnvironment)
		value.DefaultDescription = strings.TrimSpace(value.DefaultDescription)
		value.DefaultNotes = strings.TrimSpace(value.DefaultNotes)
		value.DefaultAllowedGroups = normalizeStringSlice(value.DefaultAllowedGroups)
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

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parseKeyVaultSyncStates(raw string) map[string]KeyVaultSyncState {
	if strings.TrimSpace(raw) == "" {
		return map[string]KeyVaultSyncState{}
	}

	var parsed map[string]KeyVaultSyncState
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]KeyVaultSyncState{}
	}
	return normalizeKeyVaultSyncStates(parsed)
}

func normalizeKeyVaultSyncStates(values map[string]KeyVaultSyncState) map[string]KeyVaultSyncState {
	out := map[string]KeyVaultSyncState{}
	for key, value := range values {
		trimmedKey := strings.TrimRight(strings.TrimSpace(key), "/")
		if trimmedKey == "" {
			continue
		}
		value.LastSyncStatus = strings.TrimSpace(value.LastSyncStatus)
		value.LastSyncError = strings.TrimSpace(value.LastSyncError)
		value.LastSyncSummary = strings.TrimSpace(value.LastSyncSummary)
		out[trimmedKey] = value
	}
	return out
}

func mergeKeyVaultSyncState(sources []KeyVaultSource, states map[string]KeyVaultSyncState) []KeyVaultSource {
	merged := make([]KeyVaultSource, 0, len(sources))
	for _, source := range sources {
		if state, ok := states[source.VaultURL]; ok {
			source.LastSyncedAt = state.LastSyncedAt
			source.LastSyncStatus = state.LastSyncStatus
			source.LastSyncError = state.LastSyncError
			source.LastSyncSummary = state.LastSyncSummary
		}
		merged = append(merged, source)
	}
	return merged
}

func stripKeyVaultSyncState(sources []KeyVaultSource) []KeyVaultSource {
	clean := make([]KeyVaultSource, 0, len(sources))
	for _, source := range sources {
		source.LastSyncedAt = nil
		source.LastSyncStatus = ""
		source.LastSyncError = ""
		source.LastSyncSummary = ""
		clean = append(clean, source)
	}
	return clean
}
