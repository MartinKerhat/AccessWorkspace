package payload

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type LaunchPayload struct {
	ResourceID   string                 `json:"resourceId"`
	ResourceType string                 `json:"resourceType"`
	Method       string                 `json:"method"`
	Target       string                 `json:"target"`
	Command      string                 `json:"command,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
}

func DecodeProtocolURI(raw string) (LaunchPayload, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return LaunchPayload{}, fmt.Errorf("parse launcher uri: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "access-workspace") {
		return LaunchPayload{}, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host != "launch" {
		return LaunchPayload{}, fmt.Errorf("unsupported launcher action %q", parsed.Host)
	}
	encoded := parsed.Query().Get("payload")
	if strings.TrimSpace(encoded) == "" {
		return LaunchPayload{}, fmt.Errorf("launcher uri does not include payload")
	}
	return DecodePayload(encoded)
}

func DecodePayload(encoded string) (LaunchPayload, error) {
	normalized := strings.TrimSpace(encoded)
	normalized = strings.ReplaceAll(normalized, "-", "+")
	normalized = strings.ReplaceAll(normalized, "_", "/")
	switch len(normalized) % 4 {
	case 2:
		normalized += "=="
	case 3:
		normalized += "="
	}

	bytes, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		return LaunchPayload{}, fmt.Errorf("decode payload bytes: %w", err)
	}

	var payload LaunchPayload
	if err := json.Unmarshal(bytes, &payload); err != nil {
		return LaunchPayload{}, fmt.Errorf("decode payload json: %w", err)
	}
	if payload.ResourceType == "" {
		return LaunchPayload{}, fmt.Errorf("payload is missing resourceType")
	}
	if payload.Target == "" && payload.URL == "" {
		return LaunchPayload{}, fmt.Errorf("payload is missing target")
	}
	if payload.Metadata == nil {
		payload.Metadata = map[string]interface{}{}
	}
	return payload, nil
}

func DecodePayloadFile(path string) (LaunchPayload, error) {
	bytes, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return LaunchPayload{}, fmt.Errorf("read payload file: %w", err)
	}

	var payload LaunchPayload
	if err := json.Unmarshal(bytes, &payload); err != nil {
		return LaunchPayload{}, fmt.Errorf("decode payload file json: %w", err)
	}
	if payload.ResourceType == "" {
		return LaunchPayload{}, fmt.Errorf("payload is missing resourceType")
	}
	if payload.Target == "" && payload.URL == "" {
		return LaunchPayload{}, fmt.Errorf("payload is missing target")
	}
	if payload.Metadata == nil {
		payload.Metadata = map[string]interface{}{}
	}
	return payload, nil
}

func MetadataString(metadata map[string]interface{}, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func MetadataBool(metadata map[string]interface{}, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	boolean, ok := value.(bool)
	return ok && boolean
}
