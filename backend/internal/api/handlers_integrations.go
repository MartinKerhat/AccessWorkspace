package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/resources"
)

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

	// Validate every reference before creating anything so a bad item late
	// in the selection cannot leave a half-imported batch behind.
	serviceItems := make([]resources.KeyVaultImportItem, 0, len(items))
	for _, item := range items {
		secretID := strings.TrimSpace(item.SecretID)
		if err := s.keyVault.ValidateReference(r.Context(), secretID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		serviceItems = append(serviceItems, resources.KeyVaultImportItem{
			VaultURL:    item.VaultURL,
			VaultName:   item.VaultName,
			ObjectName:  item.ObjectName,
			SecretID:    secretID,
			ContentType: item.ContentType,
			ExpiresAt:   item.ExpiresAt,
			Enabled:     item.Enabled,
		})
	}

	result, err := s.resources.ImportKeyVaultSecrets(r.Context(), user, resources.KeyVaultImportInput{
		Description:   input.Description,
		Owner:         input.Owner,
		OwnerTeam:     input.OwnerTeam,
		Environment:   input.Environment,
		Notes:         input.Notes,
		AllowedGroups: input.AllowedGroups,
		Items:         serviceItems,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": result.Items, "skipped": result.Skipped})
}

func (s *Server) handleKeyVaultSync(w http.ResponseWriter, r *http.Request, user auth.User) {
	// SyncKeyVault also enforces this, but keep the handler-level gate in step
	// with its sibling key-vault endpoints (discover/import) as defense in depth.
	if !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
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
