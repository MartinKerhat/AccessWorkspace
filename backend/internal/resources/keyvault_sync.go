package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/keyvault"
)

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
