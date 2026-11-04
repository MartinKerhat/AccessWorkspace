package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

// ImportKeyVaultSecrets creates one managed resource per selected secret,
// skipping any secret that already has a live managed resource. The check
// mirrors the automatic sync (see SyncKeyVault): both key on the normalized
// secret identifier, so a secret auto-imported by one path is recognised as
// imported by the other. Archived objects are handled the same way too: one a
// user archived is skipped (Restore brings it back), one archived because its
// secret had vanished is restored.
func (s *Service) ImportKeyVaultSecrets(ctx context.Context, user auth.User, input KeyVaultImportInput) (KeyVaultImportResult, error) {
	if !user.IsAdmin {
		return KeyVaultImportResult{}, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["keyvault"].Import {
		return KeyVaultImportResult{}, ErrForbidden
	}
	if len(input.Items) == 0 {
		return KeyVaultImportResult{}, fmt.Errorf("%w: at least one Key Vault secret must be selected", ErrInvalidInput)
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	existing, err := s.repo.ListManagedKeyVault(ctx)
	if err != nil {
		return KeyVaultImportResult{}, err
	}
	existingByReference := managedKeyVaultReferences(existing)
	archivedByReference, err := s.archivedKeyVaultByReference(ctx)
	if err != nil {
		return KeyVaultImportResult{}, err
	}

	now := time.Now().UTC()
	result := KeyVaultImportResult{Items: make([]Resource, 0, len(input.Items))}
	for _, item := range input.Items {
		reference := normalizeKeyVaultReference(item.SecretID)
		if reference == "" {
			return KeyVaultImportResult{}, fmt.Errorf("%w: key vault secret id is required", ErrInvalidInput)
		}
		if _, exists := existingByReference[reference]; exists {
			result.Skipped++
			continue
		}
		if archived, ok := archivedByReference[reference]; ok {
			if !archived.reappearsWithSecret() {
				result.Archived++
				continue
			}
			restored, err := s.restoreReappearedKeyVaultRow(ctx, user, archived, false)
			if err != nil {
				return KeyVaultImportResult{}, err
			}
			delete(archivedByReference, reference)
			existingByReference[reference] = restored
			result.Restored++
			continue
		}

		created, err := s.Create(ctx, user, manualImportCreateInput(input, item, reference, now))
		if err != nil {
			return KeyVaultImportResult{}, err
		}
		existingByReference[reference] = created
		result.Items = append(result.Items, created)
	}

	return result, nil
}

func manualImportCreateInput(input KeyVaultImportInput, item KeyVaultImportItem, reference string, now time.Time) CreateResourceInput {
	return CreateResourceInput{
		Name:            strings.TrimSpace(item.ObjectName),
		Type:            TypeKeyVaultSecret,
		Description:     strings.TrimSpace(input.Description),
		Owner:           strings.TrimSpace(input.Owner),
		OwnerTeam:       strings.TrimSpace(input.OwnerTeam),
		Environment:     strings.TrimSpace(input.Environment),
		Status:          keyVaultImportStatus(item, now),
		SourceKind:      SourceKindAzureKeyVault,
		SourceObjectID:  reference,
		LastSyncedAt:    &now,
		Notes:           strings.TrimSpace(input.Notes),
		VaultName:       strings.TrimSpace(item.VaultName),
		ObjectName:      strings.TrimSpace(item.ObjectName),
		ObjectType:      "secret",
		ContentType:     strings.TrimSpace(item.ContentType),
		ExpiresAt:       item.ExpiresAt,
		NotBefore:       item.NotBefore,
		RevealAllowed:   true,
		CopyAllowed:     true,
		AllowedGroups:   append([]string{}, input.AllowedGroups...),
		SecretMode:      SecretModeExternal,
		SecretReference: reference,
		LinkedSecretRef: reference,
	}
}

// keyVaultImportStatus uses the same vocabulary as sync, so an imported row
// shows "expiring"/"inactive" right away instead of after the first sync.
// An item without the enabled flag (older clients) counts as enabled.
func keyVaultImportStatus(item KeyVaultImportItem, now time.Time) string {
	enabled := item.Enabled == nil || *item.Enabled
	return keyVaultStatus(enabled, item.NotBefore, item.ExpiresAt, now)
}

// normalizeKeyVaultReference is the canonical form of a secret identifier
// used for duplicate detection: trimmed, without a trailing slash.
func normalizeKeyVaultReference(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

// managedKeyVaultReferences indexes live managed Key Vault resources by
// their normalized secret reference, falling back to the source object id
// for rows whose secret row carries no reference.
func managedKeyVaultReferences(items []Resource) map[string]Resource {
	out := map[string]Resource{}
	for _, item := range items {
		reference := normalizeKeyVaultReference(item.Secret.Reference)
		if reference == "" {
			reference = normalizeKeyVaultReference(item.SourceObjectID)
		}
		if reference == "" {
			continue
		}
		out[reference] = item
	}
	return out
}
