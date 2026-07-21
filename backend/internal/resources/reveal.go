package resources

import (
	"context"
	"fmt"
	"strings"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func (s *Service) Reveal(ctx context.Context, user auth.User, id string) (RevealResult, error) {
	resource, err := s.repo.Get(ctx, id)
	if err != nil {
		return RevealResult{}, err
	}
	if resource.Secret.Mode == SecretModeNone {
		return RevealResult{}, fmt.Errorf("%w: passwordless entries have no secret to reveal", ErrInvalidInput)
	}
	// Password objects are governed exclusively by canRevealStoredPassword
	// (owner/admin override plus the per-type flag); the generic
	// RevealAllowed clause applies to the other categories only. Without the
	// category split, a stray revealAllowed=true on a saved password would
	// grant readers a reveal the UI can neither show nor manage.
	generalRevealAllowed := CategoryForType(resource.Type) != "passwords" &&
		canRevealResource(user, resource) && resource.RevealAllowed
	if !generalRevealAllowed && !canRevealStoredPassword(user, resource) {
		return RevealResult{}, accessDenied(user, resource.Summary())
	}
	_ = s.audit.Log(ctx, audit.LogParams{
		EventType:    audit.EventResourceRevealed,
		UserID:       user.ID,
		UserName:     user.Name,
		ResourceID:   &resource.ID,
		ResourceName: &resource.Name,
		Metadata:     map[string]any{"type": resource.Type},
	})
	secretValue, err := s.resolveRevealValue(ctx, user, resource)
	if err != nil {
		return RevealResult{}, err
	}
	return RevealResult{
		ResourceID:      resource.ID,
		SecretMode:      resource.Secret.Mode,
		SecretValue:     secretValue,
		SecretReference: resource.Secret.Reference,
	}, nil
}

// decryptStoredSecret is the single read entrypoint for inline secret
// values: personal envelopes open with the requester's session vault key
// (ErrVaultLocked when absent), everything else through the org cipher.
func (s *Service) decryptStoredSecret(ctx context.Context, user auth.User, value string) (string, error) {
	if IsPersonalEnvelope(value) {
		return s.cipher.DecryptPersonalFromStorage(ctx, value, user.VaultPrivateKey)
	}
	return s.cipher.DecryptFromStorage(ctx, value)
}

func (s *Service) resolveRevealValue(ctx context.Context, user auth.User, resource Resource) (string, error) {
	if resource.Type == TypeKeyVaultSecret &&
		resource.SourceKind == SourceKindAzureKeyVault &&
		resource.Secret.Mode == SecretModeExternal &&
		resource.Secret.Reference != "" &&
		s.keyVault != nil {
		value, err := s.keyVault.RevealSecret(ctx, resource.Secret.Reference)
		if err != nil {
			return "", err
		}
		return value, nil
	}
	if resource.Secret.Mode == SecretModeInline && s.cipher != nil {
		return s.decryptStoredSecret(ctx, user, resource.Secret.Value)
	}
	return resource.Secret.Value, nil
}

func (s *Service) prepareSecretForStorage(ctx context.Context, user auth.User, input *CreateResourceInput) error {
	if input == nil || s.cipher == nil {
		return nil
	}
	if input.SecretMode != SecretModeInline {
		input.SecretValue = ""
		return nil
	}
	value := strings.TrimSpace(input.SecretValue)
	if value == "" {
		return nil
	}

	// An already-encrypted value means the secret was preserved (edit without
	// re-entering it). If the personal↔shared scope now differs from how it is
	// stored, switch classes — a re-wrap of the same secret, not a re-type.
	// personal→shared needs the owner's unlocked session to read the personal
	// envelope; shared→personal the server can do (it holds the org key and
	// the owner's public key).
	if IsEncryptedForStorage(value) {
		if IsPersonalEnvelope(value) == input.Personal {
			input.SecretValue = value
			return nil
		}
		var plain string
		var err error
		if IsPersonalEnvelope(value) {
			plain, err = s.cipher.DecryptPersonalFromStorage(ctx, value, user.VaultPrivateKey)
		} else {
			plain, err = s.cipher.DecryptFromStorage(ctx, value)
		}
		if err != nil {
			return err
		}
		return s.encryptForScope(ctx, input, plain)
	}

	return s.encryptForScope(ctx, input, value)
}

// encryptForScope seals plaintext into the class matching input.Personal:
// personal → sealed to the owner's vault public key (ErrVaultLocked if the
// owner has no vault yet, so the client runs setup); shared → org key.
func (s *Service) encryptForScope(ctx context.Context, input *CreateResourceInput, plain string) error {
	if input.Personal && s.vaults != nil {
		publicKey, err := s.vaults.VaultPublicKey(ctx, strings.TrimSpace(input.OwnerUserID))
		if err != nil {
			return err
		}
		if len(publicKey) == 0 {
			return ErrVaultLocked
		}
		encrypted, err := s.cipher.EncryptPersonalForStorage(ctx, plain, publicKey)
		if err != nil {
			return err
		}
		input.SecretValue = encrypted
		return nil
	}
	encrypted, err := s.cipher.EncryptForStorage(ctx, plain, SecretClassShared)
	if err != nil {
		return err
	}
	input.SecretValue = encrypted
	return nil
}
