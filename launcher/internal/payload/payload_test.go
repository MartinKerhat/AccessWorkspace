package payload

import "testing"

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
