package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func (s *Service) Launch(ctx context.Context, user auth.User, id string) (LaunchPayload, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return LaunchPayload{}, err
	}
	if !canLaunchResource(user, resource) || !resource.LaunchAllowed {
		return LaunchPayload{}, accessDenied(user, resource.Summary())
	}
	resource = s.applyConnectionCredentialOverride(ctx, user, resource)
	payload := buildLaunchPayload(resource)
	payload, err = s.buildLauncherPayload(ctx, user, resource, payload)
	if err != nil {
		return LaunchPayload{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceLaunched,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":   resource.Type,
			"method": payload.Method,
		},
	})
	return payload, nil
}

func (s *Service) ListPasswordOptions(ctx context.Context, user auth.User) ([]ResourceSummary, error) {
	items, err := s.repo.List(ctx, Filter{})
	if err != nil {
		return nil, err
	}
	visible := explainVisibleResourcesForUser(user, items)
	options := make([]ResourceSummary, 0, len(visible))
	for _, item := range visible {
		if !isPersonalPasswordOverrideOption(item.ResourceSummary) {
			continue
		}
		options = append(options, item.ResourceSummary)
	}
	return options, nil
}

func (s *Service) GetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) (ConnectionCredentialOverride, error) {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	override, err := s.repo.GetConnectionUserPasswordOverride(ctx, connectionID, user.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ConnectionCredentialOverride{ConnectionID: connectionID}, nil
		}
		return ConnectionCredentialOverride{}, err
	}
	passwordResource, err := s.repo.Get(ctx, override.PasswordResourceID)
	if err != nil || !canViewResource(user, passwordResource.Summary()) || !isPasswordOverrideCandidate(passwordResource) {
		return ConnectionCredentialOverride{ConnectionID: connectionID}, nil
	}
	override.PasswordResourceName = passwordResource.Name
	override.Username = passwordResource.Username
	override.Personal = passwordResource.Personal
	return override, nil
}

func (s *Service) SetConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string, input ConnectionCredentialOverrideInput) (ConnectionCredentialOverride, error) {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, resource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	passwordID := strings.TrimSpace(input.PasswordResourceID)
	if passwordID == "" {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: password resource is required", ErrInvalidInput)
	}
	passwordResource, err := s.repo.Get(ctx, passwordID)
	if err != nil {
		return ConnectionCredentialOverride{}, err
	}
	if !canViewResource(user, passwordResource.Summary()) {
		return ConnectionCredentialOverride{}, ErrForbidden
	}
	if !isPersonalPasswordOverrideOption(passwordResource.Summary()) {
		return ConnectionCredentialOverride{}, fmt.Errorf("%w: only your personal saved passwords can be used as a connection override", ErrInvalidInput)
	}
	if err := s.repo.UpsertConnectionUserPasswordOverride(ctx, connectionID, user.ID, passwordID); err != nil {
		return ConnectionCredentialOverride{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":                   resource.Type,
			"connectionOverride":     "password_resource",
			"overridePasswordObject": passwordID,
		},
	})
	return s.GetConnectionCredentialOverride(ctx, user, connectionID)
}

func (s *Service) ClearConnectionCredentialOverride(ctx context.Context, user auth.User, connectionID string) error {
	resource, err := s.repo.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	if !canViewResource(user, resource.Summary()) {
		return ErrForbidden
	}
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return fmt.Errorf("%w: only ssh and rdp connections support personal overrides", ErrInvalidInput)
	}
	if err := s.repo.DeleteConnectionUserPasswordOverride(ctx, connectionID, user.ID); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceUpdated,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":               resource.Type,
			"connectionOverride": "cleared",
		},
	})
	return nil
}

func (s *Service) ResolveLaunchTicket(_ context.Context, ticket string) (LaunchPayload, error) {
	if s.launchTickets == nil {
		return LaunchPayload{}, ErrNotFound
	}
	return s.launchTickets.Redeem(ticket)
}

func buildLaunchPayload(resource Resource) LaunchPayload {
	payload := LaunchPayload{
		ResourceID:   resource.ID,
		ResourceType: resource.Type,
		Target:       resource.TargetHost,
		Metadata: map[string]any{
			"username":   resource.Username,
			"folderPath": resource.FolderPath,
			"launchMode": resource.LaunchMode,
			"secretMode": resource.Secret.Mode,
		},
	}

	switch resource.Type {
	case TypeRDP:
		port := 3389
		if resource.TargetPort != nil {
			port = *resource.TargetPort
		}
		payload.Method = "command_proposal"
		payload.Command = fmt.Sprintf("mstsc /v:%s:%d", resource.TargetHost, port)
		payload.Metadata["protocol"] = "rdp"
		payload.Metadata["port"] = fmt.Sprintf("%d", port)
		payload.Metadata["connectionDomain"] = resource.ConnectionDomain
		payload.Metadata["connectionAdminSession"] = resource.ConnectionAdminSession
		payload.Metadata["connectionAutomaticLogon"] = resource.ConnectionAutomaticLogon
		payload.Metadata["connectionMacAddress"] = resource.ConnectionMacAddress
		payload.Metadata["connectionGatewayHost"] = resource.ConnectionGatewayHost
	case TypeSSH:
		port := 22
		if resource.TargetPort != nil {
			port = *resource.TargetPort
		}
		payload.Method = "command_proposal"
		payload.Metadata["protocol"] = "ssh"
		payload.Metadata["port"] = fmt.Sprintf("%d", port)
		if resource.Username != "" {
			payload.Command = fmt.Sprintf("ssh %s@%s -p %d", resource.Username, resource.TargetHost, port)
		} else {
			payload.Command = fmt.Sprintf("ssh %s -p %d", resource.TargetHost, port)
		}
	case TypeWebPortal:
		payload.Method = "url"
		payload.URL = resource.TargetURL
		payload.Target = resource.TargetURL
	default:
		payload.Method = "metadata"
		payload.Metadata["message"] = "No native launcher for this resource type in the MVP."
	}

	return payload
}

func (s *Service) applyConnectionCredentialOverride(ctx context.Context, user auth.User, resource Resource) Resource {
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return resource
	}
	override, err := s.repo.GetConnectionUserPasswordOverride(ctx, resource.ID, user.ID)
	if err != nil {
		return resource
	}
	passwordResource, err := s.repo.Get(ctx, override.PasswordResourceID)
	if err != nil {
		return resource
	}
	if !canViewResource(user, passwordResource.Summary()) || !isPasswordOverrideCandidate(passwordResource) {
		return resource
	}
	resource.Username = passwordResource.Username
	resource.Secret = passwordResource.Secret
	resource.LinkedSecretRef = passwordResource.LinkedSecretRef
	return resource
}

func (s *Service) buildLauncherPayload(ctx context.Context, user auth.User, resource Resource, browserPayload LaunchPayload) (LaunchPayload, error) {
	if resource.Type != TypeSSH && resource.Type != TypeRDP {
		return browserPayload, nil
	}

	launcherPayload := browserPayload
	launcherPayload.Method = "launcher_handoff"
	launcherPayload.Metadata = cloneLaunchMetadata(browserPayload.Metadata)
	launcherPayload.Metadata["resourceName"] = resource.Name
	launcherPayload.Metadata["connectionName"] = resource.Name
	launcherPayload.Metadata["connectionDomain"] = resource.ConnectionDomain
	launcherPayload.Metadata["connectionAutomaticLogon"] = resource.ConnectionAutomaticLogon
	if resource.Type == TypeRDP && s.rdpSigning != nil {
		config, err := s.rdpSigning.GetRDPSigningRuntime(ctx)
		if err != nil {
			return LaunchPayload{}, err
		}
		if config.Enabled && config.CertificateConfigured {
			launcherPayload.Metadata["rdpSigningEnabled"] = true
			launcherPayload.Metadata["rdpSigningSubject"] = config.Subject
			launcherPayload.Metadata["rdpSigningThumbprintSha256"] = config.ThumbprintSHA256
			launcherPayload.Metadata["rdpSigningPfxBase64"] = config.PFXBase64
			launcherPayload.Metadata["rdpSigningPfxPassword"] = config.PFXPassword
			launcherPayload.Metadata["rdpSigningLeafCertBase64"] = config.LeafCertBase64
			launcherPayload.Metadata["rdpSigningRootCertBase64"] = config.RootCertBase64
		}
	}

	secretValue, err := s.resolveLaunchSecret(ctx, user, resource)
	if err != nil {
		return LaunchPayload{}, err
	}
	if secretValue != "" {
		launcherPayload.Metadata["secretValue"] = secretValue
	}

	ticket := s.launchTickets.Issue(launcherPayload, 2*time.Minute)
	browserPayload.Method = "launcher_ticket"
	browserPayload.Metadata["launcherTicket"] = ticket
	return browserPayload, nil
}

func cloneLaunchMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (s *Service) resolveLaunchSecret(ctx context.Context, user auth.User, resource Resource) (string, error) {
	switch resource.Secret.Mode {
	case SecretModePrompt:
		return "", nil
	case SecretModeInline:
		if s.cipher != nil {
			return s.decryptStoredSecret(ctx, user, resource.Secret.Value)
		}
		return resource.Secret.Value, nil
	case SecretModeExternal:
		reference := strings.TrimSpace(resource.Secret.Reference)
		if reference == "" {
			reference = strings.TrimSpace(resource.LinkedSecretRef)
		}
		if reference == "" || s.keyVault == nil {
			return "", nil
		}
		return s.keyVault.RevealSecret(ctx, reference)
	default:
		return "", nil
	}
}
