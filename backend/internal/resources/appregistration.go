package resources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func (s *Service) UpdateAppRegistrationNotificationPolicies(ctx context.Context, user auth.User, id string, input AppRegistrationNotificationPolicyUpdateInput) (Resource, error) {
	if !user.IsAdmin {
		return Resource{}, ErrForbidden
	}
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if resource.Type != TypeAppRegistration {
		return Resource{}, fmt.Errorf("%w: only app registrations support notification policies", ErrInvalidInput)
	}
	resourcePolicy := normalizeOptionalNotificationPolicy(input.ResourcePolicy)
	credentialPolicies := make([]AppRegistrationCredentialPolicyInput, 0, len(input.CredentialPolicies))
	for _, item := range input.CredentialPolicies {
		keyID := strings.TrimSpace(item.KeyID)
		if keyID == "" {
			continue
		}
		credentialPolicies = append(credentialPolicies, AppRegistrationCredentialPolicyInput{
			KeyID:  keyID,
			Policy: normalizeOptionalNotificationPolicy(item.Policy),
		})
	}
	if err := s.repo.ReplaceAppRegistrationNotificationPolicies(ctx, id, resourcePolicy, credentialPolicies); err != nil {
		return Resource{}, err
	}
	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if s.notifications != nil {
		_ = s.notifications.EvaluateResource(ctx, id)
	}
	return updated, nil
}

func (s *Service) ImportAppRegistrations(ctx context.Context, user auth.User, input AppRegistrationImportInput) ([]Resource, error) {
	if !user.IsAdmin {
		return nil, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["appregistrations"].Import {
		return nil, ErrForbidden
	}
	if s.appRegistrations == nil {
		return nil, fmt.Errorf("%w: app registration integration is not available", ErrInvalidInput)
	}

	input = normalizeAppRegistrationImportInput(input)
	if input.Owner == "" {
		return nil, fmt.Errorf("%w: owner is required for app registration import", ErrInvalidInput)
	}
	if len(input.ApplicationIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one app registration must be selected", ErrInvalidInput)
	}

	existing, err := s.repo.ListManagedAppRegistrations(ctx)
	if err != nil {
		return nil, err
	}
	existingByExternalID := appRegistrationExistingKeys(existing)

	now := time.Now().UTC()
	imported := make([]Resource, 0, len(input.ApplicationIDs))
	for _, identifier := range input.ApplicationIDs {
		app, err := s.appRegistrations.CurrentApplication(ctx, identifier)
		if err != nil {
			return nil, err
		}
		if app.ID == "" && app.AppID == "" {
			continue
		}
		if appRegistrationAlreadyImported(existingByExternalID, app) {
			continue
		}

		createInput := appRegistrationCreateInput(input, app, now)
		resource, err := s.repo.Create(ctx, createInput)
		if err != nil {
			return nil, err
		}
		credentials, owners := appRegistrationSnapshot(app, resource.ID, now)
		if err := s.repo.ReplaceAppRegistrationSnapshot(ctx, resource.ID, credentials, owners); err != nil {
			return nil, err
		}
		resource.AppCredentials = credentials
		resource.AppOwners = owners
		imported = append(imported, resource)
		existingByExternalID[strings.TrimSpace(app.ID)] = struct{}{}
		existingByExternalID[strings.TrimSpace(app.AppID)] = struct{}{}

		_ = s.audit.Log(ctx, audit.LogParams{
			EventType:    audit.EventResourceCreated,
			UserID:       user.ID,
			UserName:     user.Name,
			ResourceID:   &resource.ID,
			ResourceName: &resource.Name,
			Metadata: map[string]any{
				"type":            resource.Type,
				"reason":          "app_registration_import",
				"credentialCount": len(credentials),
			},
		})
		if s.notifications != nil {
			_ = s.notifications.EvaluateResource(ctx, resource.ID)
		}
	}

	return imported, nil
}

func (s *Service) SyncAppRegistrations(ctx context.Context, user auth.User, automatic bool) (AppRegistrationSyncResult, error) {
	if !user.IsAdmin {
		return AppRegistrationSyncResult{}, ErrForbidden
	}
	if !auth.CapabilitiesForUser(user).Categories["appregistrations"].View {
		return AppRegistrationSyncResult{}, ErrForbidden
	}
	if s.appRegistrations == nil {
		return AppRegistrationSyncResult{}, fmt.Errorf("%w: app registration integration is not available", ErrInvalidInput)
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	items, err := s.repo.ListManagedAppRegistrations(ctx)
	if err != nil {
		return AppRegistrationSyncResult{}, err
	}

	now := time.Now().UTC()
	result := AppRegistrationSyncResult{Automatic: automatic}
	for _, item := range items {
		identifier := strings.TrimSpace(item.SourceObjectID)
		if identifier == "" {
			identifier = strings.TrimSpace(item.ApplicationID)
		}
		if identifier == "" {
			result.MissingResources++
			continue
		}

		result.AttemptedResources++
		app, err := s.appRegistrations.CurrentApplication(ctx, identifier)
		if err != nil {
			var graphErr appregistrations.RequestError
			if errors.As(err, &graphErr) && (graphErr.StatusCode == http.StatusUnauthorized || graphErr.StatusCode == http.StatusForbidden) {
				return result, err
			}
			if appregistrations.IsNotFound(err) {
				if err := s.repo.Archive(ctx, item.ID); err != nil {
					result.MissingResources++
					continue
				}
				result.RemovedResources++
				_ = s.audit.Log(ctx, audit.LogParams{
					EventType:    audit.EventResourceArchived,
					UserID:       user.ID,
					UserName:     user.Name,
					ResourceID:   &item.ID,
					ResourceName: &item.Name,
					Metadata: map[string]any{
						"type":      item.Type,
						"reason":    "app_registration_not_found",
						"automatic": automatic,
					},
				})
				continue
			}
			result.MissingResources++
			continue
		}

		update := resourceToUpdateInput(item)
		applyAppRegistrationMetadata(&update, app, now)
		if _, err := s.repo.Update(ctx, item.ID, update); err != nil {
			result.MissingResources++
			continue
		}
		credentials, owners := appRegistrationSnapshot(app, item.ID, now)
		if err := s.repo.ReplaceAppRegistrationSnapshot(ctx, item.ID, credentials, owners); err != nil {
			result.MissingResources++
			continue
		}
		if s.notifications != nil {
			_ = s.notifications.EvaluateResource(ctx, item.ID)
		}
		expiring, expired := countCredentialExpiry(credentials, now)
		result.ExpiringCredentials += expiring
		result.ExpiredCredentials += expired
		result.UpdatedResources++
	}

	return result, nil
}

func normalizeAppRegistrationImportInput(input AppRegistrationImportInput) AppRegistrationImportInput {
	input.Owner = strings.TrimSpace(input.Owner)
	input.OwnerTeam = strings.TrimSpace(input.OwnerTeam)
	input.Environment = strings.TrimSpace(input.Environment)
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.Description = strings.TrimSpace(input.Description)
	input.Notes = strings.TrimSpace(input.Notes)
	input.AllowedGroups = normalizeValues(input.AllowedGroups)
	input.ApplicationIDs = normalizeValues(input.ApplicationIDs)
	return input
}

func appRegistrationExistingKeys(items []Resource) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		if value := strings.TrimSpace(item.SourceObjectID); value != "" {
			out[value] = struct{}{}
		}
		if value := strings.TrimSpace(item.ApplicationID); value != "" {
			out[value] = struct{}{}
		}
		if value := strings.TrimSpace(item.ClientID); value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func appRegistrationAlreadyImported(existing map[string]struct{}, app appregistrations.ApplicationItem) bool {
	if _, ok := existing[strings.TrimSpace(app.ID)]; ok && strings.TrimSpace(app.ID) != "" {
		return true
	}
	if _, ok := existing[strings.TrimSpace(app.AppID)]; ok && strings.TrimSpace(app.AppID) != "" {
		return true
	}
	return false
}

func appRegistrationCreateInput(input AppRegistrationImportInput, app appregistrations.ApplicationItem, now time.Time) CreateResourceInput {
	create := CreateResourceInput{
		Name:                appRegistrationDisplayName(app),
		Type:                TypeAppRegistration,
		Description:         input.Description,
		Owner:               input.Owner,
		OwnerTeam:           input.OwnerTeam,
		Environment:         input.Environment,
		TenantID:            input.TenantID,
		SourceKind:          SourceKindEntraAppRegistration,
		SourceObjectID:      strings.TrimSpace(app.ID),
		LastSyncedAt:        &now,
		Notes:               input.Notes,
		Provider:            "entra",
		ApplicationID:       strings.TrimSpace(app.AppID),
		ClientID:            strings.TrimSpace(app.AppID),
		DisplayNameExternal: strings.TrimSpace(app.DisplayName),
		AllowedGroups:       append([]string{}, input.AllowedGroups...),
		SecretMode:          SecretModeExternal,
		SecretReference:     appRegistrationReference(app),
	}
	applyAppRegistrationCredentialSummary(&create.CredentialType, &create.CredentialExpiresAt, app.Credentials, now)
	create.Status = appRegistrationStatus(app.Credentials, now)
	if create.Description == "" {
		create.Description = "Imported from Microsoft Entra app registrations."
	}
	return create
}

func applyAppRegistrationMetadata(input *UpdateResourceInput, app appregistrations.ApplicationItem, now time.Time) {
	input.Name = appRegistrationDisplayName(app)
	input.SourceKind = SourceKindEntraAppRegistration
	input.SourceObjectID = strings.TrimSpace(app.ID)
	input.LastSyncedAt = &now
	input.Provider = "entra"
	input.ApplicationID = strings.TrimSpace(app.AppID)
	input.ClientID = strings.TrimSpace(app.AppID)
	input.DisplayNameExternal = strings.TrimSpace(app.DisplayName)
	input.SecretMode = SecretModeExternal
	input.SecretReference = appRegistrationReference(app)
	input.LaunchAllowed = false
	input.RevealAllowed = false
	input.CopyAllowed = false
	applyAppRegistrationCredentialSummary(&input.CredentialType, &input.CredentialExpiresAt, app.Credentials, now)
	input.Status = appRegistrationStatus(app.Credentials, now)
}

func appRegistrationSnapshot(app appregistrations.ApplicationItem, resourceID string, now time.Time) ([]AppRegistrationCredential, []AppRegistrationOwner) {
	credentials := make([]AppRegistrationCredential, 0, len(app.Credentials))
	for _, item := range app.Credentials {
		keyID := strings.TrimSpace(item.KeyID)
		credentialType := strings.TrimSpace(item.Type)
		if keyID == "" || credentialType == "" {
			continue
		}
		syncedAt := now
		credentials = append(credentials, AppRegistrationCredential{
			ResourceID:     resourceID,
			KeyID:          keyID,
			CredentialType: credentialType,
			DisplayName:    strings.TrimSpace(item.DisplayName),
			StartDateTime:  item.StartDateTime,
			EndDateTime:    item.EndDateTime,
			Hint:           strings.TrimSpace(item.Hint),
			Usage:          strings.TrimSpace(item.Usage),
			LastSyncedAt:   &syncedAt,
		})
	}

	owners := make([]AppRegistrationOwner, 0, len(app.Owners))
	for _, item := range app.Owners {
		ownerID := strings.TrimSpace(item.ID)
		if ownerID == "" {
			continue
		}
		owners = append(owners, AppRegistrationOwner{
			ResourceID:  resourceID,
			OwnerID:     ownerID,
			OwnerType:   strings.TrimSpace(item.Type),
			DisplayName: strings.TrimSpace(item.DisplayName),
			Email:       strings.TrimSpace(item.Email),
		})
	}
	return credentials, owners
}

func applyAppRegistrationCredentialSummary(credentialType *string, credentialExpiresAt **time.Time, credentials []appregistrations.CredentialItem, now time.Time) {
	next := nextAppRegistrationCredential(credentials, now)
	if next == nil {
		*credentialType = "none"
		*credentialExpiresAt = nil
		return
	}
	*credentialType = next.Type
	*credentialExpiresAt = next.EndDateTime
}

func appRegistrationStatus(credentials []appregistrations.CredentialItem, now time.Time) string {
	if len(credentials) == 0 {
		return "no_credentials"
	}
	next := nextFutureCredentialExpiry(credentials, now)
	if next == nil {
		for _, credential := range credentials {
			if credential.EndDateTime == nil {
				return "active"
			}
		}
		return "expired"
	}
	if !next.After(now.Add(30 * 24 * time.Hour)) {
		return "expiring"
	}
	return "active"
}

func nextAppRegistrationCredential(credentials []appregistrations.CredentialItem, now time.Time) *appregistrations.CredentialItem {
	if next := nextFutureCredential(credentials, now); next != nil {
		return next
	}
	var expired *appregistrations.CredentialItem
	for i := range credentials {
		if credentials[i].EndDateTime == nil {
			continue
		}
		if expired == nil || credentials[i].EndDateTime.After(*expired.EndDateTime) {
			expired = &credentials[i]
		}
	}
	return expired
}

func nextFutureCredential(credentials []appregistrations.CredentialItem, now time.Time) *appregistrations.CredentialItem {
	var next *appregistrations.CredentialItem
	for i := range credentials {
		expiresAt := credentials[i].EndDateTime
		if expiresAt == nil || !expiresAt.After(now) {
			continue
		}
		if next == nil || expiresAt.Before(*next.EndDateTime) {
			next = &credentials[i]
		}
	}
	return next
}

func nextFutureCredentialExpiry(credentials []appregistrations.CredentialItem, now time.Time) *time.Time {
	next := nextFutureCredential(credentials, now)
	if next == nil {
		return nil
	}
	return next.EndDateTime
}

func countCredentialExpiry(credentials []AppRegistrationCredential, now time.Time) (int, int) {
	expiring := 0
	expired := 0
	for _, credential := range credentials {
		if credential.EndDateTime == nil {
			continue
		}
		if !credential.EndDateTime.After(now) {
			expired++
			continue
		}
		if !credential.EndDateTime.After(now.Add(30 * 24 * time.Hour)) {
			expiring++
		}
	}
	return expiring, expired
}

func appRegistrationDisplayName(app appregistrations.ApplicationItem) string {
	if value := strings.TrimSpace(app.DisplayName); value != "" {
		return value
	}
	if value := strings.TrimSpace(app.AppID); value != "" {
		return value
	}
	return strings.TrimSpace(app.ID)
}

func appRegistrationReference(app appregistrations.ApplicationItem) string {
	if value := strings.TrimSpace(app.AppID); value != "" {
		return "app-registration://" + value
	}
	if value := strings.TrimSpace(app.ID); value != "" {
		return "app-registration://" + value
	}
	return "app-registration://unknown"
}
