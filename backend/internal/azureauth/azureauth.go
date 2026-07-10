// Package azureauth provides app-level Azure access tokens through the
// standard credential chain, so the reader identity needs no stored client
// secret. Resolution order (DefaultAzureCredential): env-provided client
// secret/certificate -> workload identity (federated service-account token
// in Kubernetes) -> managed identity (Azure VM/App Service/ACA) -> Azure CLI
// (local dev). Which tier a deployment lands on is decided entirely by its
// environment; callers just ask for a token per scope.
package azureauth

import (
	"context"
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// TokenSource returns a bearer token for one scope (e.g.
// "https://vault.azure.net/.default"). Implementations cache internally.
type TokenSource func(ctx context.Context, scope string) (string, error)

type chain struct {
	mu   sync.Mutex
	cred azcore.TokenCredential
	err  error
	done bool
}

// Chain is a lazily-initialized DefaultAzureCredential: deployments that
// never need it (settings-based client secret, or no Azure at all) pay
// nothing and see no startup errors.
type Chain struct {
	inner chain
}

func NewChain() *Chain {
	return &Chain{}
}

// TokenSource returns tokens per scope through the chain.
func (c *Chain) TokenSource() TokenSource {
	return c.inner.token
}

// Credential exposes the underlying azcore credential for Azure SDK clients
// (e.g. Key Vault keys). Construction still happens lazily on first token.
func (c *Chain) Credential() azcore.TokenCredential {
	return lazyCredential{chain: &c.inner}
}

// lazyCredential defers DefaultAzureCredential construction to the first
// GetToken call so that merely wiring it up never fails at startup.
type lazyCredential struct {
	chain *chain
}

func (l lazyCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	cred, err := l.chain.credential()
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("azure credential chain unavailable: %w", err)
	}
	return cred.GetToken(ctx, options)
}

// NewChainTokenSource builds a TokenSource over DefaultAzureCredential.
func NewChainTokenSource() TokenSource {
	return NewChain().TokenSource()
}

func (c *chain) credential() (azcore.TokenCredential, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.done {
		c.cred, c.err = azidentity.NewDefaultAzureCredential(nil)
		c.done = true
	}
	return c.cred, c.err
}

func (c *chain) token(ctx context.Context, scope string) (string, error) {
	cred, err := c.credential()
	if err != nil {
		return "", fmt.Errorf("azure credential chain unavailable: %w", err)
	}
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", fmt.Errorf("azure credential chain token for %s: %w", scope, err)
	}
	return token.Token, nil
}
