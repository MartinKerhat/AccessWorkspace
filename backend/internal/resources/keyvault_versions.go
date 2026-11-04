package resources

import (
	"context"
	"fmt"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/keyvault"
)

// KeyVaultVersionReader reads a secret's versions. Like KeyVaultSecretWriter
// it is asserted from s.keyVault, so read-only fakes keep compiling.
type KeyVaultVersionReader interface {
	ListSecretVersions(ctx context.Context, reference string) ([]keyvault.SecretVersion, error)
	RevealSecretVersion(ctx context.Context, reference string, version string) (string, error)
}

// ResourceAuditReader looks up an object's past events; the versions list
// uses it to name who wrote each version through the workspace.
type ResourceAuditReader interface {
	ForResource(ctx context.Context, resourceID string, eventTypes []audit.EventType, limit int) ([]audit.Event, error)
}

// KeyVaultSecretVersion is one row of the versions list on a Key Vault
// object. CreatedBy is known only for versions written from the workspace;
// Azure itself does not record who created a version.
type KeyVaultSecretVersion struct {
	keyvault.SecretVersion
	Pinned    bool   `json:"pinned"`
	CreatedBy string `json:"createdBy,omitempty"`
}

// keyVaultVersionCreatorEvents bounds the audit lookup; a secret rotated
// through the workspace more often than this shows "—" on its oldest rows.
const keyVaultVersionCreatorEvents = 500

func (s *Service) keyVaultVersionReader() (KeyVaultVersionReader, error) {
	reader, ok := s.keyVault.(KeyVaultVersionReader)
	if !ok || reader == nil {
		return nil, fmt.Errorf("%w: key vault versions are not available", ErrInvalidInput)
	}
	return reader, nil
}

// ListKeyVaultSecretVersions lists the versions of the secret behind a
// catalog object, newest first. Metadata only, read live from Azure and never
// stored; anyone who can view the object may see it.
func (s *Service) ListKeyVaultSecretVersions(ctx context.Context, user auth.User, id string) ([]KeyVaultSecretVersion, error) {
	resource, err := s.azureKeyVaultResource(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canViewResource(user, resource.Summary()) {
		return nil, accessDenied(user, resource.Summary())
	}
	reader, err := s.keyVaultVersionReader()
	if err != nil {
		return nil, err
	}
	reference := keyVaultReference(resource)
	versions, err := reader.ListSecretVersions(ctx, reference)
	if err != nil {
		return nil, err
	}
	pinned := keyvault.VersionFromReference(reference)
	creators := s.keyVaultVersionCreators(ctx, resource.ID)
	out := make([]KeyVaultSecretVersion, 0, len(versions))
	for _, version := range versions {
		out = append(out, KeyVaultSecretVersion{
			SecretVersion: version,
			Pinned:        pinned != "" && version.Version == pinned,
			CreatedBy:     creators[version.Version],
		})
	}
	return out, nil
}

// RevealKeyVaultSecretVersion reveals one version under the same rule as the
// object's own reveal, and audits which version was read.
func (s *Service) RevealKeyVaultSecretVersion(ctx context.Context, user auth.User, id string, version string) (RevealResult, error) {
	resource, err := s.azureKeyVaultResource(ctx, id)
	if err != nil {
		return RevealResult{}, err
	}
	if !canRevealResource(user, resource) || !secretUsageAllowed(resource) {
		return RevealResult{}, accessDenied(user, resource.Summary())
	}
	reader, err := s.keyVaultVersionReader()
	if err != nil {
		return RevealResult{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceRevealed,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type, "version": version},
	})
	value, err := reader.RevealSecretVersion(ctx, keyVaultReference(resource), version)
	if err != nil {
		return RevealResult{}, err
	}
	return RevealResult{
		ResourceID:      resource.ID,
		SecretMode:      resource.Secret.Mode,
		SecretValue:     value,
		SecretReference: resource.Secret.Reference,
	}, nil
}

// UpdateKeyVaultSecretVersionState changes enabled and the active window of
// one version in Azure (keyvault.edit plus access to the object — maintainer
// decision 2026-11-04: state is metadata, so it follows the edit right). The
// audit event records the version and its state before and after.
func (s *Service) UpdateKeyVaultSecretVersionState(ctx context.Context, user auth.User, id string, version string, input KeyVaultSecretStateInput) (Resource, error) {
	existing, err := s.azureKeyVaultResource(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if !canChangeKeyVaultState(user, existing) {
		return Resource{}, accessDenied(user, existing.Summary())
	}
	writer, err := s.keyVaultWriter()
	if err != nil {
		return Resource{}, err
	}
	reference := keyVaultReference(existing)
	previous, item, err := writer.UpdateSecretAttributes(ctx, reference, version, keyvault.SecretAttributes{
		Enabled:   input.Enabled,
		NotBefore: input.NotBefore,
		ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return Resource{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventKeyVaultSecretStateUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &existing.ID,
		ResourceName: &existing.Name,
		Metadata: map[string]any{
			"vault":     item.VaultName,
			"secret":    item.Name,
			"version":   item.Version,
			"enabled":   item.Enabled,
			"notBefore": item.NotBefore,
			"expiresAt": item.ExpiresAt,
			"previous": map[string]any{
				"enabled":   previous.Enabled,
				"notBefore": previous.NotBefore,
				"expiresAt": previous.ExpiresAt,
			},
		},
	})

	// The row mirrors the version it points at, which need not be the one
	// just changed; re-reading it is right in both cases.
	current, err := s.keyVault.CurrentSecretMetadata(ctx, reference)
	if err != nil {
		return Resource{}, fmt.Errorf("the change was saved in Azure, but refreshing the catalog failed — the next sync will catch up: %w", err)
	}
	return s.refreshKeyVaultRow(ctx, existing, current)
}

// keyVaultVersionCreators maps version id → user name from the object's
// create and new-value audit events.
func (s *Service) keyVaultVersionCreators(ctx context.Context, resourceID string) map[string]string {
	creators := map[string]string{}
	reader, ok := s.audit.(ResourceAuditReader)
	if !ok || reader == nil {
		return creators
	}
	events, err := reader.ForResource(ctx, resourceID, []audit.EventType{audit.EventKeyVaultSecretCreated, audit.EventKeyVaultSecretValueSet}, keyVaultVersionCreatorEvents)
	if err != nil {
		return creators
	}
	for _, event := range events {
		version, _ := event.Metadata["version"].(string)
		if version != "" && creators[version] == "" {
			creators[version] = event.UserName
		}
	}
	return creators
}
