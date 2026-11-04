package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/keyvault"
)

// KeyVaultSecretWriter is the write side of the Key Vault client. It is kept
// apart from KeyVaultSecretResolver so read-only fakes keep compiling; the
// real *keyvault.Service implements both.
type KeyVaultSecretWriter interface {
	WritableSources(ctx context.Context) ([]keyvault.Source, error)
	CreateSecret(ctx context.Context, vaultURL string, name string, input keyvault.SecretWrite) (keyvault.SecretItem, error)
	SetSecretValue(ctx context.Context, reference string, input keyvault.SecretWrite) (keyvault.SecretItem, error)
	UpdateSecretAttributes(ctx context.Context, reference string, version string, attributes keyvault.SecretAttributes) (keyvault.SecretVersion, keyvault.SecretItem, error)
}

// KeyVaultSecretValueInput is a value on its way to Azure. Value never
// reaches the database, a log line or an audit event; InputMode and FileName
// exist for the audit trail only.
type KeyVaultSecretValueInput struct {
	Value        string     `json:"value"`
	ContentType  string     `json:"contentType"`
	FileEncoding string     `json:"fileEncoding"`
	FileName     string     `json:"fileName"`
	InputMode    string     `json:"inputMode"`
	Enabled      bool       `json:"enabled"`
	NotBefore    *time.Time `json:"notBefore"`
	ExpiresAt    *time.Time `json:"expiresAt"`
}

// KeyVaultCreateSecretInput creates the secret in Azure and its catalog row.
// The catalog fields mirror the import modal's.
type KeyVaultCreateSecretInput struct {
	KeyVaultSecretValueInput
	VaultURL      string   `json:"vaultUrl"`
	SecretName    string   `json:"secretName"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Owner         string   `json:"owner"`
	OwnerUserID   string   `json:"ownerUserId"`
	OwnerTeam     string   `json:"ownerTeam"`
	Environment   string   `json:"environment"`
	Notes         string   `json:"notes"`
	AllowedGroups []string `json:"allowedGroups"`
	AllowedUsers  []string `json:"allowedUsers"`
}

// KeyVaultSecretStateInput is the enabled flag and active window of one
// version.
type KeyVaultSecretStateInput struct {
	Enabled   bool       `json:"enabled"`
	NotBefore *time.Time `json:"notBefore"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *Service) keyVaultWriter() (KeyVaultSecretWriter, error) {
	writer, ok := s.keyVault.(KeyVaultSecretWriter)
	if !ok || writer == nil {
		return nil, fmt.Errorf("%w: key vault writes are not available", ErrInvalidInput)
	}
	return writer, nil
}

// ListWritableKeyVaultSources tells the UI where a create may go and which
// objects can take a new value or a state change. Names and URLs only.
func (s *Service) ListWritableKeyVaultSources(ctx context.Context, user auth.User) ([]keyvault.Source, error) {
	capabilities := auth.CapabilitiesForUser(user).Categories["keyvault"]
	if !user.IsAdmin && !capabilities.Write && !capabilities.Edit {
		return nil, ErrForbidden
	}
	writer, err := s.keyVaultWriter()
	if err != nil {
		return nil, err
	}
	return writer.WritableSources(ctx)
}

// CreateKeyVaultSecret creates a secret in Azure and registers it in the
// catalog in one step (keyvault.write). Non-admins own what they create.
func (s *Service) CreateKeyVaultSecret(ctx context.Context, user auth.User, input KeyVaultCreateSecretInput) (Resource, error) {
	if !user.IsAdmin && !auth.CapabilitiesForUser(user).Categories["keyvault"].Create {
		return Resource{}, ErrForbidden
	}
	writer, err := s.keyVaultWriter()
	if err != nil {
		return Resource{}, err
	}

	// Held across the Azure write and the row insert so a concurrent sync
	// with auto-import cannot register the brand-new secret a second time.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	item, err := writer.CreateSecret(ctx, input.VaultURL, input.SecretName, secretWriteFromInput(input.KeyVaultSecretValueInput))
	if err != nil {
		return Resource{}, err
	}

	now := time.Now().UTC()
	reference := normalizeKeyVaultReference(item.ID)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = item.Name
	}
	create := CreateResourceInput{
		Name:            name,
		Type:            TypeKeyVaultSecret,
		Description:     input.Description,
		Owner:           input.Owner,
		OwnerUserID:     input.OwnerUserID,
		OwnerTeam:       input.OwnerTeam,
		Environment:     input.Environment,
		Status:          keyVaultStatus(item.Enabled, item.NotBefore, item.ExpiresAt, now),
		SourceKind:      SourceKindAzureKeyVault,
		SourceObjectID:  reference,
		LastSyncedAt:    &now,
		Notes:           input.Notes,
		VaultName:       item.VaultName,
		ObjectName:      item.Name,
		ObjectType:      "secret",
		ObjectVersion:   item.Version,
		ContentType:     item.ContentType,
		ExpiresAt:       item.ExpiresAt,
		NotBefore:       item.NotBefore,
		LinkedSecretRef: reference,
		RevealAllowed:   true,
		CopyAllowed:     true,
		AllowedGroups:   append([]string{}, input.AllowedGroups...),
		AllowedUsers:    append([]string{}, input.AllowedUsers...),
		SecretMode:      SecretModeExternal,
		SecretReference: reference,
	}
	create = enforceCreatorOwnership(user, create)
	create = normalizeInput(create)
	if err := validateInput(create); err != nil {
		return Resource{}, err
	}
	resource, err := s.repo.Create(ctx, create)
	if err != nil {
		return Resource{}, fmt.Errorf("the secret was created in Azure, but adding it to the catalog failed — import it from the vault: %w", err)
	}

	s.logKeyVaultWrite(ctx, user, audit.EventKeyVaultSecretCreated, resource, item, input.KeyVaultSecretValueInput)
	s.evaluateExpiry(ctx, resource.ID)
	return resource, nil
}

// SetKeyVaultSecretValue writes a new version of the secret behind a catalog
// object (keyvault.write plus access to that object).
func (s *Service) SetKeyVaultSecretValue(ctx context.Context, user auth.User, id string, input KeyVaultSecretValueInput) (Resource, error) {
	existing, err := s.azureKeyVaultResource(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if !canWriteKeyVaultValue(user, existing) {
		return Resource{}, accessDenied(user, existing.Summary())
	}
	writer, err := s.keyVaultWriter()
	if err != nil {
		return Resource{}, err
	}
	item, err := writer.SetSecretValue(ctx, keyVaultReference(existing), secretWriteFromInput(input))
	if err != nil {
		return Resource{}, err
	}
	resource, err := s.refreshKeyVaultRow(ctx, existing, item)
	if err != nil {
		return Resource{}, err
	}
	s.logKeyVaultWrite(ctx, user, audit.EventKeyVaultSecretValueSet, resource, item, input)
	return resource, nil
}

func (s *Service) azureKeyVaultResource(ctx context.Context, id string) (Resource, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if existing.Type != TypeKeyVaultSecret || existing.SourceKind != SourceKindAzureKeyVault || keyVaultReference(existing) == "" {
		return Resource{}, fmt.Errorf("%w: this object is not backed by an Azure Key Vault secret", ErrInvalidInput)
	}
	return existing, nil
}

func keyVaultReference(resource Resource) string {
	if reference := normalizeKeyVaultReference(resource.Secret.Reference); reference != "" {
		return reference
	}
	return normalizeKeyVaultReference(resource.SourceObjectID)
}

// refreshKeyVaultRow stores what Azure answered, exactly as a sync would, so
// the catalog never shows a state Azure does not have.
func (s *Service) refreshKeyVaultRow(ctx context.Context, existing Resource, item keyvault.SecretItem) (Resource, error) {
	now := time.Now().UTC()
	update := resourceToUpdateInput(existing)
	update.LastSyncedAt = &now
	if item.VaultName != "" {
		update.VaultName = item.VaultName
	}
	if item.Name != "" {
		update.ObjectName = item.Name
	}
	update.ObjectVersion = item.Version
	update.ContentType = item.ContentType
	update.ExpiresAt = item.ExpiresAt
	update.NotBefore = item.NotBefore
	update.Status = keyVaultStatus(item.Enabled, item.NotBefore, item.ExpiresAt, now)
	resource, err := s.repo.Update(ctx, existing.ID, update)
	if err != nil {
		return Resource{}, fmt.Errorf("the change was saved in Azure, but refreshing the catalog failed — the next sync will catch up: %w", err)
	}
	s.evaluateExpiry(ctx, resource.ID)
	return resource, nil
}

func (s *Service) evaluateExpiry(ctx context.Context, resourceID string) {
	if s.notifications == nil {
		return
	}
	_ = s.notifications.EvaluateResource(ctx, resourceID)
	_ = s.notifications.FlushPendingEmails(ctx)
}

func (s *Service) logKeyVaultWrite(ctx context.Context, user auth.User, eventType audit.EventType, resource Resource, item keyvault.SecretItem, input KeyVaultSecretValueInput) {
	metadata := map[string]any{
		"vault":     item.VaultName,
		"secret":    item.Name,
		"version":   item.Version,
		"inputMode": keyVaultInputMode(input),
		"enabled":   item.Enabled,
	}
	if input.FileEncoding != "" {
		metadata["fileName"] = truncate(strings.TrimSpace(input.FileName), 255)
		metadata["fileEncoding"] = input.FileEncoding
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    eventType,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     metadata,
	})
}

func keyVaultInputMode(input KeyVaultSecretValueInput) string {
	if input.FileEncoding != "" {
		return "file"
	}
	if input.InputMode == "generated" {
		return "generated"
	}
	return "typed"
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func secretWriteFromInput(input KeyVaultSecretValueInput) keyvault.SecretWrite {
	return keyvault.SecretWrite{
		Value:        input.Value,
		ContentType:  strings.TrimSpace(input.ContentType),
		FileEncoding: strings.TrimSpace(input.FileEncoding),
		Attributes: keyvault.SecretAttributes{
			Enabled:   input.Enabled,
			NotBefore: input.NotBefore,
			ExpiresAt: input.ExpiresAt,
		},
	}
}

// canWriteKeyVaultValue: admins, or keyvault.write holders who can see the
// object. Secrets outside the catalog stay out of reach for non-admins.
func canWriteKeyVaultValue(user auth.User, resource Resource) bool {
	if user.IsAdmin {
		return true
	}
	return auth.CapabilitiesForUser(user).Categories["keyvault"].Write && CanAccess(devViewer(user), resource.Summary())
}

// canChangeKeyVaultState: admins, or keyvault.edit holders who can see the
// object. Ownership alone is not enough here, unlike catalog metadata: the
// change lands in Azure and can stop every consumer of the secret.
func canChangeKeyVaultState(user auth.User, resource Resource) bool {
	if user.IsAdmin {
		return true
	}
	return auth.CapabilitiesForUser(user).Categories["keyvault"].Edit && CanAccess(devViewer(user), resource.Summary())
}
