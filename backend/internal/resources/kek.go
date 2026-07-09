package resources

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
)

// KEKProvider wraps and unwraps per-secret data-encryption keys (DEKs).
// Implementations decide where the key-encryption key lives; callers never
// see it. The ID is stored inside each envelope so ciphertext written under
// one provider stays readable while a rotation to another is in progress.
// IDs must not contain ':' (the envelope field separator).
type KEKProvider interface {
	ID() string
	WrapDEK(ctx context.Context, dek []byte) ([]byte, error)
	UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error)
}

const localKEKProviderID = "local"

// localKEKProvider wraps DEKs with AES-256-GCM under a key derived from
// RESOURCE_SECRET_KEY. Guarantee level: a DB dump alone is useless; a dump
// plus any copy of the env key reads shared/app-scope secrets. The
// azure_key_vault provider (later phase) upgrades that without touching data.
type localKEKProvider struct {
	aead cipher.AEAD
}

func newLocalKEKProvider(rawKey string) (*localKEKProvider, error) {
	sum := sha256.Sum256([]byte("kek:local:" + strings.TrimSpace(rawKey)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &localKEKProvider{aead: aead}, nil
}

func (p *localKEKProvider) ID() string {
	return localKEKProviderID
}

func (p *localKEKProvider) WrapDEK(_ context.Context, dek []byte) ([]byte, error) {
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return p.aead.Seal(nonce, nonce, dek, nil), nil
}

func (p *localKEKProvider) UnwrapDEK(_ context.Context, wrapped []byte) ([]byte, error) {
	if len(wrapped) < p.aead.NonceSize() {
		return nil, errors.New("wrapped key payload is invalid")
	}
	nonce := wrapped[:p.aead.NonceSize()]
	return p.aead.Open(nil, nonce, wrapped[p.aead.NonceSize():], nil)
}
