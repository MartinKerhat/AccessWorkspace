package payload

import "testing"

// A password may legitimately start or end with whitespace, so the secret
// accessor must hand back exactly what the backend sent. MetadataString keeps
// trimming, because host/port/username/domain rely on it.
func TestMetadataSecretPreservesSurroundingWhitespace(t *testing.T) {
	metadata := map[string]interface{}{
		"secretValue": "  pa ss word\t",
		"username":    "  ops-admin  ",
	}

	if got := MetadataSecret(metadata, "secretValue"); got != "  pa ss word\t" {
		t.Fatalf("expected the secret verbatim, got %q", got)
	}
	if got := MetadataString(metadata, "username"); got != "ops-admin" {
		t.Fatalf("expected structural fields to stay trimmed, got %q", got)
	}
	if got := MetadataSecret(metadata, "missing"); got != "" {
		t.Fatalf("expected an empty string for a missing key, got %q", got)
	}
}

func TestDecodeProtocolURI(t *testing.T) {
	raw := "access-workspace://launch?payload=eyJyZXNvdXJjZUlkIjoiMSIsInJlc291cmNlVHlwZSI6InNzaCIsIm1ldGhvZCI6ImNvbW1hbmRfcHJvcG9zYWwiLCJ0YXJnZXQiOiJiYXN0aW9uLmludGVybmFsIiwibWV0YWRhdGEiOnsidXNlcm5hbWUiOiJvcHMtYWRtaW4ifX0"

	decoded, err := DecodeProtocolURI(raw)
	if err != nil {
		t.Fatalf("DecodeProtocolURI returned error: %v", err)
	}
	if decoded.ResourceType != "ssh" {
		t.Fatalf("expected ssh resource type, got %q", decoded.ResourceType)
	}
	if decoded.Target != "bastion.internal" {
		t.Fatalf("expected target bastion.internal, got %q", decoded.Target)
	}
	if MetadataString(decoded.Metadata, "username") != "ops-admin" {
		t.Fatalf("expected username ops-admin, got %q", MetadataString(decoded.Metadata, "username"))
	}
}
