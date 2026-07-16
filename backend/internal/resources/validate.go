package resources

import (
	"fmt"
	"strings"

	"access-workspace/backend/internal/auth"
)

func normalizeInput(input CreateResourceInput) CreateResourceInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Owner = strings.TrimSpace(input.Owner)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.OwnerTeam = strings.TrimSpace(input.OwnerTeam)
	input.Environment = strings.TrimSpace(input.Environment)
	input.Status = strings.TrimSpace(input.Status)
	input.FolderPath = normalizeFolderPath(input.FolderPath)
	input.LaunchMode = strings.TrimSpace(input.LaunchMode)
	input.SourceObjectID = strings.TrimSpace(input.SourceObjectID)
	input.Notes = strings.TrimSpace(input.Notes)
	input.TargetHost = strings.TrimSpace(input.TargetHost)
	input.TargetURL = strings.TrimSpace(input.TargetURL)
	input.TargetSystem = strings.TrimSpace(input.TargetSystem)
	input.Username = strings.TrimSpace(input.Username)
	input.ConnectionDomain = strings.TrimSpace(input.ConnectionDomain)
	input.ConnectionWindowMode = strings.TrimSpace(input.ConnectionWindowMode)
	input.ConnectionScreenMode = strings.TrimSpace(input.ConnectionScreenMode)
	input.ConnectionMacAddress = strings.TrimSpace(input.ConnectionMacAddress)
	input.VaultName = strings.TrimSpace(input.VaultName)
	input.ObjectName = strings.TrimSpace(input.ObjectName)
	input.ObjectType = strings.TrimSpace(input.ObjectType)
	input.ObjectVersion = strings.TrimSpace(input.ObjectVersion)
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.Provider = strings.TrimSpace(input.Provider)
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.CredentialType = strings.TrimSpace(input.CredentialType)
	input.DisplayNameExternal = strings.TrimSpace(input.DisplayNameExternal)
	input.LinkedSecretRef = strings.TrimSpace(input.LinkedSecretRef)
	input.SecretValue = strings.TrimSpace(input.SecretValue)
	input.SecretReference = strings.TrimSpace(input.SecretReference)

	if input.Status == "" {
		input.Status = "active"
	}

	if input.SourceKind == "" {
		switch input.Type {
		case TypeKeyVaultSecret:
			input.SourceKind = SourceKindAzureKeyVault
		case TypeAppRegistration:
			input.SourceKind = SourceKindEntraAppRegistration
		default:
			input.SourceKind = SourceKindManual
		}
	}
	if input.LaunchMode == "" && (input.Type == TypeSSH || input.Type == TypeRDP) {
		input.LaunchMode = "native_launcher"
	}
	if input.Type == TypeWebPortal {
		input.TargetURL = normalizePortalURLString(input.TargetURL)
	}
	if input.Type == TypeRDP {
		input.ConnectionWindowMode = "launcher_default"
		input.ConnectionScreenMode = "launcher_default"
		input.ConnectionUseMultipleMonitors = false
		input.ConnectionShowConnectionBar = true
	}

	input.AllowedGroups = normalizeValues(input.AllowedGroups)
	if input.Personal {
		input.AllowedGroups = []string{}
	}
	// Saved passwords have a single usage flag (copyAllowed); reveal and
	// launch are meaningless for them and must not linger in storage where a
	// future check could pick them up.
	if input.Type == TypeSharedSecret {
		input.RevealAllowed = false
		input.LaunchAllowed = false
	}
	return input
}

func normalizeFolderPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	parts := strings.Split(value, "/")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, "/")
}

func folderDepth(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	return len(strings.Split(value, "/"))
}

func normalizeValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func validateInput(input CreateResourceInput) error {
	if input.Name == "" || input.Owner == "" {
		return fmt.Errorf("%w: name and owner are required", ErrInvalidInput)
	}
	if folderDepth(input.FolderPath) > 2 {
		return fmt.Errorf("%w: folder path supports only root/subfolder", ErrInvalidInput)
	}
	if input.Personal {
		if CategoryForType(input.Type) != "passwords" {
			return fmt.Errorf("%w: only password objects can be personal", ErrInvalidInput)
		}
		if input.SourceKind != "" && input.SourceKind != SourceKindManual {
			return fmt.Errorf("%w: personal password objects must be manual", ErrInvalidInput)
		}
		if input.Username == "" {
			return fmt.Errorf("%w: personal password objects require a username", ErrInvalidInput)
		}
	}

	switch input.Type {
	case TypeSSH, TypeRDP:
		if input.TargetHost == "" {
			return fmt.Errorf("%w: connections require a target host", ErrInvalidInput)
		}
	case TypeWebPortal:
		if _, ok := normalizePortalURL(input.TargetURL); !ok {
			return fmt.Errorf("%w: password entries with portal access require a target URL", ErrInvalidInput)
		}
	case TypeSharedSecret:
		// Saved passwords can exist without a specific target system so they can be reused
		// as personal or shared connection overrides.
	case TypeKeyVaultSecret:
		if input.VaultName == "" || input.ObjectName == "" {
			return fmt.Errorf("%w: key vault entries require vault and object names", ErrInvalidInput)
		}
		if input.ObjectType == "" {
			input.ObjectType = "secret"
		}
	case TypeAppRegistration:
		if input.Provider == "" || input.ApplicationID == "" {
			return fmt.Errorf("%w: app registrations require provider and application id", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported resource type", ErrInvalidInput)
	}

	if input.SecretMode == "" {
		return fmt.Errorf("%w: secret mode is required", ErrInvalidInput)
	}
	if input.SecretMode == SecretModePrompt && input.Type != TypeSSH && input.Type != TypeRDP {
		return fmt.Errorf("%w: prompt-on-launch secret mode is only valid for connections", ErrInvalidInput)
	}
	if input.SecretMode == SecretModeInline &&
		(input.Type == TypeSharedSecret || input.Type == TypeWebPortal) &&
		input.SecretValue == "" {
		return fmt.Errorf("%w: inline passwords require a secret value", ErrInvalidInput)
	}
	if input.SecretMode == SecretModeExternal && input.SecretReference == "" && input.LinkedSecretRef == "" && input.Type != TypeAppRegistration {
		return fmt.Errorf("%w: external-reference objects require a secret reference", ErrInvalidInput)
	}
	if input.SecretMode == SecretModePrompt && input.SecretValue != "" {
		return fmt.Errorf("%w: prompt-on-launch resources cannot store a secret value", ErrInvalidInput)
	}

	return nil
}

func preserveManagedFields(existing Resource, input UpdateResourceInput, user auth.User) UpdateResourceInput {
	if strings.TrimSpace(input.OwnerUserID) == "" {
		input.OwnerUserID = existing.OwnerUserID
	}
	if existing.Type == TypeKeyVaultSecret && existing.SourceKind == SourceKindAzureKeyVault {
		input.Type = existing.Type
		input.SourceKind = existing.SourceKind
		input.SourceObjectID = existing.SourceObjectID
		input.LastSyncedAt = existing.LastSyncedAt
		input.VaultName = existing.VaultName
		input.ObjectName = existing.ObjectName
		input.ObjectType = existing.ObjectType
		input.ObjectVersion = existing.ObjectVersion
		input.ContentType = existing.ContentType
		input.ExpiresAt = existing.ExpiresAt
		input.LinkedSecretRef = existing.LinkedSecretRef
		input.SecretMode = existing.Secret.Mode
		input.SecretReference = existing.Secret.Reference
		input.SecretValue = ""
	}
	if existing.Type == TypeAppRegistration && existing.SourceKind == SourceKindEntraAppRegistration {
		input.Name = existing.Name
		input.Type = existing.Type
		input.Status = existing.Status
		input.SourceKind = existing.SourceKind
		input.SourceObjectID = existing.SourceObjectID
		input.LastSyncedAt = existing.LastSyncedAt
		input.Provider = existing.Provider
		input.ApplicationID = existing.ApplicationID
		input.TenantID = existing.TenantID
		input.ClientID = existing.ClientID
		input.CredentialType = existing.CredentialType
		input.CredentialExpiresAt = existing.CredentialExpiresAt
		input.DisplayNameExternal = existing.DisplayNameExternal
		input.LaunchAllowed = existing.LaunchAllowed
		input.RevealAllowed = existing.RevealAllowed
		input.CopyAllowed = existing.CopyAllowed
		input.SecretMode = existing.Secret.Mode
		input.SecretReference = existing.Secret.Reference
		input.SecretValue = ""
	}
	if existing.Personal && input.Personal {
		input.Owner = existing.Owner
		input.OwnerUserID = existing.OwnerUserID
		input.AllowedGroups = []string{}
	}
	return input
}

func preserveExistingSecret(existing Resource, input UpdateResourceInput) UpdateResourceInput {
	if input.SecretMode == SecretModeInline && strings.TrimSpace(input.SecretValue) == "" {
		input.SecretValue = existing.Secret.Value
	}
	if input.SecretMode == SecretModeExternal && strings.TrimSpace(input.SecretReference) == "" {
		input.SecretReference = existing.Secret.Reference
	}
	return input
}

func resourceToUpdateInput(resource Resource) UpdateResourceInput {
	return UpdateResourceInput{
		Name:                          resource.Name,
		Type:                          resource.Type,
		Personal:                      resource.Personal,
		Description:                   resource.Description,
		Owner:                         resource.Owner,
		OwnerUserID:                   resource.OwnerUserID,
		OwnerTeam:                     resource.OwnerTeam,
		Environment:                   resource.Environment,
		Status:                        resource.Status,
		FolderPath:                    resource.FolderPath,
		LaunchMode:                    resource.LaunchMode,
		SourceKind:                    resource.SourceKind,
		SourceObjectID:                resource.SourceObjectID,
		LastSyncedAt:                  resource.LastSyncedAt,
		Notes:                         resource.Notes,
		TargetHost:                    resource.TargetHost,
		TargetPort:                    resource.TargetPort,
		TargetURL:                     resource.TargetURL,
		TargetSystem:                  resource.TargetSystem,
		Username:                      resource.Username,
		ConnectionDomain:              resource.ConnectionDomain,
		ConnectionAdminSession:        resource.ConnectionAdminSession,
		ConnectionAutomaticLogon:      resource.ConnectionAutomaticLogon,
		ConnectionWindowMode:          resource.ConnectionWindowMode,
		ConnectionUseMultipleMonitors: resource.ConnectionUseMultipleMonitors,
		ConnectionShowConnectionBar:   resource.ConnectionShowConnectionBar,
		ConnectionScreenMode:          resource.ConnectionScreenMode,
		ConnectionMacAddress:          resource.ConnectionMacAddress,
		VaultName:                     resource.VaultName,
		ObjectName:                    resource.ObjectName,
		ObjectType:                    resource.ObjectType,
		ObjectVersion:                 resource.ObjectVersion,
		ContentType:                   resource.ContentType,
		ExpiresAt:                     resource.ExpiresAt,
		Provider:                      resource.Provider,
		ApplicationID:                 resource.ApplicationID,
		TenantID:                      resource.TenantID,
		ClientID:                      resource.ClientID,
		CredentialType:                resource.CredentialType,
		CredentialExpiresAt:           resource.CredentialExpiresAt,
		DisplayNameExternal:           resource.DisplayNameExternal,
		LinkedSecretRef:               resource.LinkedSecretRef,
		LaunchAllowed:                 resource.LaunchAllowed,
		RevealAllowed:                 resource.RevealAllowed,
		CopyAllowed:                   resource.CopyAllowed,
		AllowedGroups:                 append([]string{}, resource.AllowedGroups...),
		SecretMode:                    resource.Secret.Mode,
		SecretReference:               resource.Secret.Reference,
	}
}
