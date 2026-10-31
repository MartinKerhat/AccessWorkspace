//go:build linux || darwin

package launcher

import (
	"strings"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/launcher/internal/payload"
)

func TestBuildFreeRDPArgsMapsConnectionFields(t *testing.T) {
	item := payload.LaunchPayload{
		ResourceType: "rdp",
		Target:       "host.example",
		Metadata: map[string]interface{}{
			"username":               "someone",
			"connectionDomain":       "CORP",
			"connectionAdminSession": true,
			"connectionName":         "Jump host",
			"secretValue":            "s3cret",
		},
	}
	args := buildFreeRDPArgs(item, "host.example", "3390", "gateway.example")
	want := []string{"/v:host.example:3390", "/u:someone", "/d:CORP", "/g:gateway.example", "/admin", "/t:Jump host", "+clipboard", "/cert:tofu"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for _, arg := range args {
		if strings.Contains(arg, "s3cret") {
			t.Fatalf("the password must never be an argument, got %v", args)
		}
	}
}

func TestBuildFreeRDPArgsOmitsEmptyFields(t *testing.T) {
	item := payload.LaunchPayload{ResourceType: "rdp", Target: "host", Metadata: map[string]interface{}{}}
	args := buildFreeRDPArgs(item, "host", "3389", "")
	want := []string{"/v:host:3389", "+clipboard", "/cert:tofu"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestSanitizeRDPArgsForLogHidesPasswords(t *testing.T) {
	got := sanitizeRDPArgsForLog([]string{"/v:host", "/p:secret", "/gp:other", "/from-stdin"})
	want := "/v:host /p:*** /p:*** /from-stdin"
	if strings.Join(got, " ") != want {
		t.Fatalf("sanitized = %q, want %q", strings.Join(got, " "), want)
	}
}
