package resources

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func (s *Service) ListPortalCredentialMatches(ctx context.Context, user auth.User, rawURL string) ([]PortalCredentialMatch, error) {
	currentURL, ok := normalizePortalURL(rawURL)
	if !ok {
		return nil, fmt.Errorf("%w: portal url is required", ErrInvalidInput)
	}
	if !auth.CapabilitiesForUser(user).Categories["passwords"].View {
		return nil, ErrForbidden
	}

	items, err := s.repo.List(ctx, Filter{})
	if err != nil {
		return nil, err
	}

	visible := explainVisibleResourcesForUser(user, items)
	matches := make([]PortalCredentialMatch, 0, len(visible))
	for _, item := range visible {
		if item.Type != TypeWebPortal || !item.CopyAllowed {
			continue
		}
		if !portalURLMatches(item.TargetURL, currentURL) {
			continue
		}
		matches = append(matches, PortalCredentialMatch{
			ResourceID:   item.ID,
			ResourceName: item.Name,
			Username:     item.Username,
			TargetURL:    item.TargetURL,
			Personal:     item.Personal,
			Owner:        item.Owner,
			OwnerUserID:  item.OwnerUserID,
		})
	}

	slices.SortFunc(matches, func(a, b PortalCredentialMatch) int {
		aTarget, _ := normalizePortalURL(a.TargetURL)
		bTarget, _ := normalizePortalURL(b.TargetURL)
		if a.Personal != b.Personal {
			if a.Personal {
				return -1
			}
			return 1
		}
		if len(aTarget.Path) != len(bTarget.Path) {
			if len(aTarget.Path) > len(bTarget.Path) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ResourceName, b.ResourceName)
	})

	return matches, nil
}

func (s *Service) FillPortalCredential(ctx context.Context, user auth.User, resourceID string, rawURL string) (PortalCredentialFillResult, error) {
	resource, err := s.repo.Get(ctx, resourceID)
	if err != nil {
		return PortalCredentialFillResult{}, err
	}
	if resource.Type != TypeWebPortal {
		return PortalCredentialFillResult{}, fmt.Errorf("%w: only web portal logins support browser fill", ErrInvalidInput)
	}
	if !canFillPasswordResource(user, resource) {
		return PortalCredentialFillResult{}, ErrForbidden
	}

	currentURL, ok := normalizePortalURL(rawURL)
	if !ok {
		return PortalCredentialFillResult{}, fmt.Errorf("%w: portal url is required", ErrInvalidInput)
	}
	if !portalURLMatches(resource.TargetURL, currentURL) {
		return PortalCredentialFillResult{}, ErrForbidden
	}

	password, err := s.resolveRevealValue(ctx, user, resource)
	if err != nil {
		return PortalCredentialFillResult{}, err
	}

	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceFilled,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata: map[string]any{
			"type":    resource.Type,
			"channel": "browser_extension",
			"url":     currentURL.String(),
		},
	})

	return PortalCredentialFillResult{
		ResourceID:   resource.ID,
		ResourceName: resource.Name,
		Username:     resource.Username,
		Password:     password,
		TargetURL:    resource.TargetURL,
	}, nil
}
