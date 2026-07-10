package resources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeKEKProvider is a reversible toy wrapper (XOR) with a distinct ID, used
// to exercise provider switching and caching without Azure.
type fakeKEKProvider struct {
	id          string
	unwrapCalls int
}

func (f *fakeKEKProvider) ID() string { return f.id }

func (f *fakeKEKProvider) WrapDEK(_ context.Context, dek []byte) ([]byte, error) {
	out := make([]byte, len(dek))
	for i, b := range dek {
		out[i] = b ^ 0xAA
	}
	return out, nil
}

func (f *fakeKEKProvider) UnwrapDEK(_ context.Context, wrapped []byte) ([]byte, error) {
	f.unwrapCalls++
	out := make([]byte, len(wrapped))
	for i, b := range wrapped {
		out[i] = b ^ 0xAA
	}
	return out, nil
}

func TestSecretCipherRewrapSwitchesProviderWithoutTouchingData(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")

	encrypted, err := cipher.EncryptForStorage(ctx, "the-password", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if cipher.NeedsRewrap(encrypted) {
		t.Fatalf("value under the active provider must not need rewrap")
	}

	// Switch the active provider — as the deployment config flip does.
	fake := &fakeKEKProvider{id: "akv"}
	cipher.UseKEKProvider(fake)

	if !cipher.NeedsRewrap(encrypted) {
		t.Fatalf("local-wrapped value must need rewrap after provider switch")
	}
	// Old rows stay readable before any migration runs.
	plain, err := cipher.DecryptFromStorage(ctx, encrypted)
	if err != nil || plain != "the-password" {
		t.Fatalf("pre-rewrap read: got %q err=%v", plain, err)
	}

	rewrapped, err := cipher.RewrapForStorage(ctx, encrypted)
	if err != nil {
		t.Fatalf("rewrap: %v", err)
	}
	if !strings.HasPrefix(rewrapped, "enc:v2:shared:akv:") {
		t.Fatalf("expected new kek id in envelope, got %q", rewrapped)
	}
	// The sealed payload (last field) must be byte-identical — rewrap never
	// reconstructs or re-encrypts the plaintext.
	oldParts := strings.SplitN(encrypted, ":", 6)
	newParts := strings.SplitN(rewrapped, ":", 6)
	if oldParts[5] != newParts[5] {
		t.Fatalf("sealed data changed during rewrap")
	}
	if cipher.NeedsRewrap(rewrapped) {
		t.Fatalf("rewrapped value must not need another rewrap")
	}
	plain, err = cipher.DecryptFromStorage(ctx, rewrapped)
	if err != nil || plain != "the-password" {
		t.Fatalf("post-rewrap read: got %q err=%v", plain, err)
	}
	// Rewrap is idempotent.
	again, err := cipher.RewrapForStorage(ctx, rewrapped)
	if err != nil || again != rewrapped {
		t.Fatalf("expected rewrap to pass through current values, err=%v", err)
	}
	// New writes use the new provider directly.
	fresh, err := cipher.EncryptForStorage(ctx, "new-secret", SecretClassShared)
	if err != nil || !strings.HasPrefix(fresh, "enc:v2:shared:akv:") {
		t.Fatalf("expected new writes under active provider, got %q err=%v", fresh, err)
	}
}

func TestCachingKEKProviderMemoizesUnwrap(t *testing.T) {
	ctx := context.Background()
	inner := &fakeKEKProvider{id: "akv"}
	cached := NewCachingKEKProvider(inner, 5*time.Minute)

	dek := []byte("0123456789abcdef0123456789abcdef")
	wrapped, err := cached.WrapDEK(ctx, dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}

	first, err := cached.UnwrapDEK(ctx, wrapped)
	if err != nil || string(first) != string(dek) {
		t.Fatalf("first unwrap: %q err=%v", first, err)
	}
	second, err := cached.UnwrapDEK(ctx, wrapped)
	if err != nil || string(second) != string(dek) {
		t.Fatalf("second unwrap: %q err=%v", second, err)
	}
	if inner.unwrapCalls != 1 {
		t.Fatalf("expected exactly one inner unwrap (cache hit on second), got %d", inner.unwrapCalls)
	}
	// Returned slices are copies — mutating one must not poison the cache.
	first[0] = 'X'
	third, err := cached.UnwrapDEK(ctx, wrapped)
	if err != nil || string(third) != string(dek) {
		t.Fatalf("cache poisoned by caller mutation: %q err=%v", third, err)
	}
}
