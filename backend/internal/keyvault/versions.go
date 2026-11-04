package keyvault

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SecretVersion is one version of a secret as the versions list reports it:
// attributes only, never a value. Key Vault cannot delete a single version, so
// enabled and the active window are all there is to manage on one.
type SecretVersion struct {
	Version     string     `json:"version"`
	Current     bool       `json:"current"`
	Enabled     bool       `json:"enabled"`
	ContentType string     `json:"contentType"`
	NotBefore   *time.Time `json:"notBefore,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// Version ids are 32 hex characters; the looser pattern still keeps anything
// that could change the request path (a slash, "..", a query) out of the URL.
var secretVersionPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)

// VersionFromReference returns the version segment of a pinned reference, or
// "" for a versionless one.
func VersionFromReference(reference string) string {
	return secretVersionFromID(strings.TrimRight(strings.TrimSpace(reference), "/"))
}

// ListSecretVersions lists every version of the secret behind reference,
// newest first. The first one is the current version — the one a versionless
// `get` returns, whether or not it is enabled. Reading needs no write switch.
func (s *Service) ListSecretVersions(ctx context.Context, reference string) ([]SecretVersion, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	reference = strings.TrimRight(strings.TrimSpace(reference), "/")
	if reference == "" {
		return nil, fmt.Errorf("secret reference is required")
	}
	if !belongsToConfiguredSource(reference, sources) {
		return nil, fmt.Errorf("secret reference does not belong to a configured key vault source")
	}
	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return nil, err
	}
	bundles, err := s.allVersions(ctx, accessToken, versionlessReference(reference))
	if err != nil {
		return nil, err
	}
	out := make([]SecretVersion, 0, len(bundles))
	for index, bundle := range bundles {
		out = append(out, SecretVersion{
			Version:     secretVersionFromID(bundle.ID),
			Current:     index == 0,
			Enabled:     bundle.Attributes.Enabled,
			ContentType: bundle.ContentType,
			NotBefore:   unixTimePtr(bundle.Attributes.NotBeforeUnix),
			ExpiresAt:   unixTimePtr(bundle.Attributes.ExpiresUnix),
			CreatedAt:   unixTimePtr(bundle.Attributes.CreatedUnix),
			UpdatedAt:   unixTimePtr(bundle.Attributes.UpdatedUnix),
		})
	}
	return out, nil
}

// RevealSecretVersion reads the value of one version. Azure refuses a disabled
// version with 403, like a disabled secret.
func (s *Service) RevealSecretVersion(ctx context.Context, reference string, version string) (string, error) {
	if !secretVersionPattern.MatchString(version) {
		return "", fmt.Errorf("%w: invalid secret version", ErrInvalidSecret)
	}
	return s.RevealSecret(ctx, versionlessReference(strings.TrimRight(strings.TrimSpace(reference), "/"))+"/"+version)
}

// UpdateSecretAttributes changes enabled and the active window of one version
// of the secret behind reference. An empty version means the version the
// reference points at: the pinned one, or else the current one. It returns the
// attributes as they were before the change, for the audit trail.
func (s *Service) UpdateSecretAttributes(ctx context.Context, reference string, version string, attributes SecretAttributes) (SecretVersion, SecretItem, error) {
	runtime, source, reference, err := s.writableReference(ctx, reference)
	if err != nil {
		return SecretVersion{}, SecretItem{}, err
	}
	if version == "" {
		version = secretVersionFromID(reference)
	}
	if version != "" && !secretVersionPattern.MatchString(version) {
		return SecretVersion{}, SecretItem{}, fmt.Errorf("%w: invalid secret version", ErrInvalidSecret)
	}
	if err := validateAttributes(attributes); err != nil {
		return SecretVersion{}, SecretItem{}, err
	}
	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return SecretVersion{}, SecretItem{}, err
	}

	// Reading the version first both captures the previous state and resolves
	// "current" to a concrete version id, so the PATCH never lands on a
	// version created in between.
	base := versionlessReference(reference)
	target := base
	if version != "" {
		target = base + "/" + version
	}
	previous, err := s.bundleFromVersions(ctx, accessToken, target)
	if err != nil {
		return SecretVersion{}, SecretItem{}, err
	}
	bundle, err := sendJSON[secretBundle](ctx, s.client, http.MethodPatch, withAPIVersion(base+"/"+secretVersionFromID(previous.ID)), accessToken, vaultSecretPatch{
		Attributes: attributesPayload(attributes),
	})
	if err != nil {
		return SecretVersion{}, SecretItem{}, err
	}
	before := SecretVersion{
		Version:   secretVersionFromID(previous.ID),
		Enabled:   previous.Attributes.Enabled,
		NotBefore: unixTimePtr(previous.Attributes.NotBeforeUnix),
		ExpiresAt: unixTimePtr(previous.Attributes.ExpiresUnix),
	}
	return before, secretItemFromBundle(base, bundle, source), nil
}

// allVersions pages through the versions list (Azure returns it unordered)
// and sorts it newest first by creation time.
func (s *Service) allVersions(ctx context.Context, accessToken string, base string) ([]secretBundle, error) {
	out := []secretBundle{}
	endpoint := withAPIVersion(base + "/versions")
	for endpoint != "" {
		page, err := callJSON[vaultSecretsListResponse](ctx, s.client, http.MethodGet, endpoint, accessToken, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Value...)
		endpoint = page.NextLink
	}
	sort.SliceStable(out, func(i, j int) bool {
		return unixValue(out[i].Attributes.CreatedUnix) > unixValue(out[j].Attributes.CreatedUnix)
	})
	return out, nil
}

func versionlessReference(reference string) string {
	if version := secretVersionFromID(reference); version != "" {
		return strings.TrimSuffix(reference, "/"+version)
	}
	return reference
}

func unixValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
