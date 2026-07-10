package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// The passkey path's server side is just wrap/unwrap under the PRF secret;
// the WebAuthn ceremony that produces that secret is browser-side and must be
// verified in a real browser. This covers the crypto: the same PRF secret
// round-trips the private key, a wrong secret does not, and the derived wrap
// key is domain-separated from the passphrase derivation.
func TestPasskeyWrapUnwrapRoundTrip(t *testing.T) {
	_, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	prfSecret := bytes.Repeat([]byte{0x5A}, 32)

	wrapKey := derivePasskeyWrapKey(prfSecret)
	wrapped, err := vaultSeal(wrapKey, priv[:])
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	opened, err := vaultOpen(derivePasskeyWrapKey(prfSecret), wrapped)
	if err != nil || !bytes.Equal(opened, priv[:]) {
		t.Fatalf("expected round-trip with the same PRF secret, err=%v", err)
	}

	wrongSecret := bytes.Repeat([]byte{0x5B}, 32)
	if _, err := vaultOpen(derivePasskeyWrapKey(wrongSecret), wrapped); err == nil {
		t.Fatalf("expected a different PRF secret to fail unwrap")
	}

	// Passkey and passphrase derivations must not collide for equal inputs.
	if bytes.Equal(derivePasskeyWrapKey(prfSecret), derivePasswordWrapKey(string(prfSecret), prfSecret[:16])) {
		t.Fatalf("passkey and passphrase wrap keys must be domain-separated")
	}
}

func TestDecodePRFSecret(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if _, err := decodePRFSecret(valid); err != nil {
		t.Fatalf("expected valid 32-byte secret to decode, got %v", err)
	}
	if _, err := decodePRFSecret("not base64!!!"); err == nil {
		t.Fatalf("expected invalid base64 to be rejected")
	}
	short := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	if _, err := decodePRFSecret(short); err == nil {
		t.Fatalf("expected too-short secret to be rejected")
	}
}
