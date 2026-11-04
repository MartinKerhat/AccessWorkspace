package keyvault

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// WritableSources lists the configured vaults an admin has opened for writes.
// It carries names and URLs only, so it is safe to show non-admins who hold a
// write right.
func (s *Service) WritableSources(ctx context.Context) ([]Source, error) {
	_, sources, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.WriteEnabled {
			out = append(out, source)
		}
	}
	return out, nil
}

// CreateSecret creates a new secret. It refuses a name that already exists:
// Key Vault itself would accept the PUT and silently add a new version, which
// is "set a new value", a different operation behind a different right.
// The returned item's ID is the versionless reference the catalog stores.
func (s *Service) CreateSecret(ctx context.Context, vaultURL string, name string, input SecretWrite) (SecretItem, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return SecretItem{}, err
	}
	vaultURL = strings.TrimRight(strings.TrimSpace(vaultURL), "/")
	source, ok := findSource(sources, vaultURL)
	if !ok {
		return SecretItem{}, fmt.Errorf("%w: the vault is not a configured key vault source", ErrInvalidSecret)
	}
	if !source.WriteEnabled {
		return SecretItem{}, ErrSourceNotWritable
	}
	name = strings.TrimSpace(name)
	if !secretNamePattern.MatchString(name) {
		return SecretItem{}, fmt.Errorf("%w: the name must be 1-127 characters of letters, digits and dashes", ErrInvalidSecret)
	}
	if err := validateSecretWrite(input); err != nil {
		return SecretItem{}, err
	}

	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return SecretItem{}, err
	}
	reference := source.VaultURL + "/secrets/" + name

	_, err = callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, withAPIVersion(reference), accessToken, nil)
	switch {
	case err == nil:
		return SecretItem{}, ErrSecretExists
	case isForbidden(err):
		// A disabled secret answers `get` with 403 — it exists all the same.
		// (So does a missing `get` permission; then the PUT below would not
		// be allowed either, and refusing here is the clearer message.)
		return SecretItem{}, ErrSecretExists
	case !IsNotFound(err):
		return SecretItem{}, err
	}

	return s.putSecret(ctx, accessToken, reference, source, input, nil)
}

// SetSecretValue writes a new version of an existing secret. Tags of the
// current version are carried over (a new version starts without any), except
// the file-encoding tag, which always describes the value being written now.
func (s *Service) SetSecretValue(ctx context.Context, reference string, input SecretWrite) (SecretItem, error) {
	runtime, source, reference, err := s.writableReference(ctx, reference)
	if err != nil {
		return SecretItem{}, err
	}
	if secretVersionFromID(reference) != "" {
		return SecretItem{}, fmt.Errorf("%w: this object is pinned to one secret version; a new value would not reach it", ErrInvalidSecret)
	}
	if err := validateSecretWrite(input); err != nil {
		return SecretItem{}, err
	}

	accessToken, err := s.accessToken(ctx, runtime, vaultScope)
	if err != nil {
		return SecretItem{}, err
	}
	current, err := s.currentBundle(ctx, accessToken, reference)
	if err != nil {
		return SecretItem{}, err
	}
	return s.putSecret(ctx, accessToken, reference, source, input, current.Tags)
}

func (s *Service) writableReference(ctx context.Context, reference string) (RuntimeConfig, Source, string, error) {
	runtime, sources, err := s.loadConfig(ctx)
	if err != nil {
		return RuntimeConfig{}, Source{}, "", err
	}
	reference = strings.TrimRight(strings.TrimSpace(reference), "/")
	if reference == "" {
		return RuntimeConfig{}, Source{}, "", fmt.Errorf("%w: secret reference is required", ErrInvalidSecret)
	}
	if !belongsToConfiguredSource(reference, sources) {
		return RuntimeConfig{}, Source{}, "", fmt.Errorf("%w: secret reference does not belong to a configured key vault source", ErrInvalidSecret)
	}
	source := sourceForReference(reference, sources)
	if !source.WriteEnabled {
		return RuntimeConfig{}, Source{}, "", ErrSourceNotWritable
	}
	return runtime, source, reference, nil
}

func (s *Service) putSecret(ctx context.Context, accessToken string, reference string, source Source, input SecretWrite, carriedTags map[string]string) (SecretItem, error) {
	tags := map[string]string{}
	for key, value := range carriedTags {
		if key != FileEncodingTag {
			tags[key] = value
		}
	}
	if input.FileEncoding != "" {
		tags[FileEncodingTag] = input.FileEncoding
	}
	response, err := sendJSON[vaultSecretResponse](ctx, s.client, http.MethodPut, withAPIVersion(reference), accessToken, vaultSecretWrite{
		Value:       input.Value,
		ContentType: strings.TrimSpace(input.ContentType),
		Attributes:  attributesPayload(input.Attributes),
		Tags:        tags,
	})
	if err != nil {
		return SecretItem{}, err
	}
	return secretItemFromBundle(reference, response.secretBundle, source), nil
}

// currentBundle reads the current version's metadata, falling back to the
// versions list when the secret is disabled and `get` answers 403.
func (s *Service) currentBundle(ctx context.Context, accessToken string, reference string) (secretBundle, error) {
	response, err := callJSON[vaultSecretResponse](ctx, s.client, http.MethodGet, withAPIVersion(reference), accessToken, nil)
	if err == nil {
		return response.secretBundle, nil
	}
	if !isForbidden(err) {
		return secretBundle{}, err
	}
	return s.bundleFromVersions(ctx, accessToken, reference)
}

func (s *Service) metadataFromVersions(ctx context.Context, accessToken string, reference string, source Source) (SecretItem, error) {
	bundle, err := s.bundleFromVersions(ctx, accessToken, reference)
	if err != nil {
		return SecretItem{}, err
	}
	return secretItemFromBundle(reference, bundle, source), nil
}

// bundleFromVersions picks the pinned version when the reference names one,
// otherwise the newest by creation time — the version a versionless `get`
// would return.
func (s *Service) bundleFromVersions(ctx context.Context, accessToken string, reference string) (secretBundle, error) {
	pinned := secretVersionFromID(reference)
	versions, err := s.allVersions(ctx, accessToken, versionlessReference(reference))
	if err != nil {
		return secretBundle{}, err
	}
	for _, version := range versions {
		if pinned == "" || secretVersionFromID(version.ID) == pinned {
			return version, nil
		}
	}
	return secretBundle{}, RequestError{StatusCode: http.StatusNotFound, Body: "secret version not found"}
}

func findSource(sources []Source, vaultURL string) (Source, bool) {
	for _, source := range sources {
		if strings.EqualFold(source.VaultURL, vaultURL) {
			return source, true
		}
	}
	return Source{}, false
}

func validateSecretWrite(input SecretWrite) error {
	if input.Value == "" {
		return fmt.Errorf("%w: the secret value is empty", ErrInvalidSecret)
	}
	if len(input.Value) > MaxSecretValueBytes {
		return fmt.Errorf("%w: the secret value is %d bytes; Key Vault allows at most %d", ErrInvalidSecret, len(input.Value), MaxSecretValueBytes)
	}
	if len(input.ContentType) > 255 {
		return fmt.Errorf("%w: the content type is longer than 255 characters", ErrInvalidSecret)
	}
	switch input.FileEncoding {
	case "", "utf-8":
	case "base64":
		// `az keyvault secret download --encoding base64` decodes this, so a
		// value that is not valid base64 would come back as garbage.
		if _, err := base64.StdEncoding.DecodeString(input.Value); err != nil {
			return fmt.Errorf("%w: the uploaded file is marked base64 but the value is not valid base64", ErrInvalidSecret)
		}
	default:
		return fmt.Errorf("%w: unsupported file encoding %q", ErrInvalidSecret, input.FileEncoding)
	}
	return validateAttributes(input.Attributes)
}

func validateAttributes(attributes SecretAttributes) error {
	if attributes.NotBefore != nil && attributes.ExpiresAt != nil && !attributes.ExpiresAt.After(*attributes.NotBefore) {
		return fmt.Errorf("%w: the active-until date must be after the active-from date", ErrInvalidSecret)
	}
	return nil
}
