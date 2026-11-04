package resources

import (
	"context"
	"fmt"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

// Reasons recorded on resource_archived events.
const (
	archiveReasonRemovedFromApp   = "removed_from_app"
	archiveReasonKeyVaultNotFound = "key_vault_secret_not_found"
)

// archivedKeyVaultLister is implemented by the real repository; asserted like
// KeyVaultSecretWriter so the many single-purpose test stores keep compiling.
type archivedKeyVaultLister interface {
	ListArchivedKeyVault(ctx context.Context) ([]ArchivedKeyVaultRow, error)
}

// archivedKeyVaultByReference indexes archived Key Vault objects by the secret
// they tracked; when one secret has several, the most recently archived wins.
func (s *Service) archivedKeyVaultByReference(ctx context.Context) (map[string]ArchivedKeyVaultRow, error) {
	out := map[string]ArchivedKeyVaultRow{}
	lister, ok := s.repo.(archivedKeyVaultLister)
	if !ok {
		return out, nil
	}
	rows, err := lister.ListArchivedKeyVault(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		reference := normalizeKeyVaultReference(row.Reference)
		if reference == "" {
			continue
		}
		if _, seen := out[reference]; !seen {
			out[reference] = row
		}
	}
	return out, nil
}

// reappearsWithSecret: the object was archived only because its secret had
// vanished from Azure, so when the secret is back the object comes back with
// it. Anything else — a user's Archive, or a reason the trail no longer has —
// stays archived until someone restores it; importing again would create a
// second object for the same secret.
func (row ArchivedKeyVaultRow) reappearsWithSecret() bool {
	return row.Reason == archiveReasonKeyVaultNotFound
}

// restoreReappearedKeyVaultRow brings back an object whose secret is in Azure
// again, keeping its owner, sharing and notes.
func (s *Service) restoreReappearedKeyVaultRow(ctx context.Context, user auth.User, row ArchivedKeyVaultRow, automatic bool) (Resource, error) {
	if err := s.repo.Restore(ctx, row.ID); err != nil {
		return Resource{}, err
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceRestored,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &row.ID,
		ResourceName: &row.Name,
		Metadata: map[string]any{
			"type":      TypeKeyVaultSecret,
			"reason":    "key_vault_secret_reappeared",
			"automatic": automatic,
		},
	})
	return s.repo.Get(ctx, row.ID)
}

// ensureNoActiveKeyVaultTwin refuses to restore an object whose secret is
// already tracked by an active one; the database's unique index would refuse
// it too, with a less useful message.
func (s *Service) ensureNoActiveKeyVaultTwin(ctx context.Context, resource Resource) error {
	if resource.Type != TypeKeyVaultSecret || resource.SourceKind != SourceKindAzureKeyVault {
		return nil
	}
	reference := keyVaultReference(resource)
	if reference == "" {
		return nil
	}
	active, err := s.repo.ListManagedKeyVault(ctx)
	if err != nil {
		return err
	}
	if twin, exists := managedKeyVaultReferences(active)[reference]; exists && twin.ID != resource.ID {
		return fmt.Errorf("%w: the active object %q already tracks this secret; archive it first or keep using it", ErrInvalidInput, twin.Name)
	}
	return nil
}
