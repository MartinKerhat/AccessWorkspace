package resources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

const azureKEKProviderID = "akv"

// azureKeyVaultKEK wraps DEKs with an RSA key that lives in Azure Key Vault.
// Wrap/unwrap run inside the vault (the KEK never leaves it); the client
// authenticates with whatever identity the deployment provides (workload
// identity in AKS). Guarantee: a DB dump plus every stored key/env copy
// still unwraps nothing — an attacker additionally needs a live identity
// with crypto rights on the key, which is auditable in Azure.
type azureKeyVaultKEK struct {
	client   *azkeys.Client
	vaultURL string
	keyName  string
}

// azureWrappedDEK is the stored wrap payload. The kid records exactly which
// key version wrapped this DEK, so unwrap keeps working across key rotation:
// new wraps use the latest version, old rows unwrap by their recorded kid.
type azureWrappedDEK struct {
	KID        string `json:"kid"`
	Ciphertext []byte `json:"ct"`
}

func NewAzureKeyVaultKEK(vaultURL, keyName string, credential azcore.TokenCredential) (KEKProvider, error) {
	vaultURL = strings.TrimRight(strings.TrimSpace(vaultURL), "/")
	keyName = strings.TrimSpace(keyName)
	if vaultURL == "" || keyName == "" {
		return nil, errors.New("azure key vault KEK requires a vault URL and key name")
	}
	client, err := azkeys.NewClient(vaultURL, credential, nil)
	if err != nil {
		return nil, err
	}
	return &azureKeyVaultKEK{client: client, vaultURL: vaultURL, keyName: keyName}, nil
}

func (p *azureKeyVaultKEK) ID() string {
	return azureKEKProviderID
}

func (p *azureKeyVaultKEK) WrapDEK(ctx context.Context, dek []byte) ([]byte, error) {
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP256
	result, err := p.client.WrapKey(ctx, p.keyName, "", azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     dek,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("key vault wrap: %w", err)
	}
	if result.KID == nil || len(result.Result) == 0 {
		return nil, errors.New("key vault wrap returned an empty result")
	}
	return json.Marshal(azureWrappedDEK{KID: string(*result.KID), Ciphertext: result.Result})
}

func (p *azureKeyVaultKEK) UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error) {
	var payload azureWrappedDEK
	if err := json.Unmarshal(wrapped, &payload); err != nil {
		return nil, fmt.Errorf("key vault wrapped payload is invalid: %w", err)
	}
	kid := azkeys.ID(payload.KID)
	// The client is bound to one vault; a kid from another vault means the
	// deployment config moved without rewrapping the data first.
	if !strings.HasPrefix(strings.ToLower(payload.KID), strings.ToLower(p.vaultURL)+"/") {
		return nil, fmt.Errorf("secret was wrapped by a different vault (%s); rewrap before switching vaults", payload.KID)
	}
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP256
	result, err := p.client.UnwrapKey(ctx, kid.Name(), kid.Version(), azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     payload.Ciphertext,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("key vault unwrap: %w", err)
	}
	return result.Result, nil
}

// cachingKEKProvider memoizes UnwrapDEK results for a short TTL so hot paths
// (reveals, launches, sync) don't pay a network round-trip per secret and
// stay far away from Key Vault rate limits. DEKs live in memory only and
// expire; WrapDEK is never cached (each secret gets a fresh DEK anyway).
type cachingKEKProvider struct {
	inner KEKProvider
	ttl   time.Duration

	mu      sync.Mutex
	entries map[[32]byte]dekCacheEntry
}

type dekCacheEntry struct {
	dek       []byte
	expiresAt time.Time
}

const dekCacheMaxEntries = 4096

func NewCachingKEKProvider(inner KEKProvider, ttl time.Duration) KEKProvider {
	if ttl <= 0 {
		return inner
	}
	return &cachingKEKProvider{
		inner:   inner,
		ttl:     ttl,
		entries: map[[32]byte]dekCacheEntry{},
	}
}

func (c *cachingKEKProvider) ID() string {
	return c.inner.ID()
}

func (c *cachingKEKProvider) WrapDEK(ctx context.Context, dek []byte) ([]byte, error) {
	return c.inner.WrapDEK(ctx, dek)
}

func (c *cachingKEKProvider) UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error) {
	key := sha256.Sum256(wrapped)
	now := time.Now()

	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && entry.expiresAt.After(now) {
		dek := append([]byte(nil), entry.dek...)
		c.mu.Unlock()
		return dek, nil
	}
	c.mu.Unlock()

	dek, err := c.inner.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if len(c.entries) >= dekCacheMaxEntries {
		for k, entry := range c.entries {
			if !entry.expiresAt.After(now) {
				delete(c.entries, k)
			}
		}
		// Still full after dropping expired entries: skip caching this one
		// rather than evicting live entries at random.
		if len(c.entries) >= dekCacheMaxEntries {
			c.mu.Unlock()
			return dek, nil
		}
	}
	c.entries[key] = dekCacheEntry{dek: append([]byte(nil), dek...), expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
	return dek, nil
}
