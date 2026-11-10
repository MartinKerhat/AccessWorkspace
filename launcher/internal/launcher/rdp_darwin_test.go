//go:build darwin

package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Outside a .app bundle (go test, go run) the bundled client cannot exist;
// the ACCESS_WORKSPACE_MACFREERDP override is the development path.
func TestFindFreeRDPClientDarwin(t *testing.T) {
	t.Setenv("ACCESS_WORKSPACE_MACFREERDP", "")
	if _, err := findFreeRDPClient(); err == nil {
		t.Fatal("expected no bundled client when not running from a .app bundle")
	}

	app := filepath.Join(t.TempDir(), "MacFreeRDP.app")
	binary := filepath.Join(app, "Contents", "MacOS", "MacFreeRDP")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ACCESS_WORKSPACE_MACFREERDP", app)
	got, err := findFreeRDPClient()
	if err != nil || got != binary {
		t.Fatalf("override by .app path: got %q, %v; want %q", got, err, binary)
	}
	t.Setenv("ACCESS_WORKSPACE_MACFREERDP", binary)
	got, err = findFreeRDPClient()
	if err != nil || got != binary {
		t.Fatalf("override by executable path: got %q, %v; want %q", got, err, binary)
	}
	t.Setenv("ACCESS_WORKSPACE_MACFREERDP", filepath.Join(t.TempDir(), "missing"))
	if _, err := findFreeRDPClient(); err == nil {
		t.Fatal("expected an error for an override that does not exist")
	}

	if caps := Capabilities(); caps["rdp"] != false {
		t.Fatalf("capabilities should report rdp=false without a client, got %v", caps)
	}
}

// The bundled Cocoa client differs from the command-line clients in two
// spellings, and getting either wrong fails without a useful message: plain
// /from-stdin never delivers the stored password, and /g: is not compiled
// into the bundled build at all.
func TestFreeRDPDarwinArgumentSpellings(t *testing.T) {
	if freeRDPStdinPasswordArg != "/from-stdin:force" {
		t.Fatalf("stdin password argument = %q, want /from-stdin:force", freeRDPStdinPasswordArg)
	}
	if got := freeRDPGatewayArg("gateway.example"); got != "/gateway:g:gateway.example" {
		t.Fatalf("gateway argument = %q, want /gateway:g:gateway.example", got)
	}
	// Without it, hosts that sign the TLS handshake with SHA-1 drop the
	// connection before the password prompt.
	if freeRDPTLSSecLevelArg != "/tls:seclevel:0" {
		t.Fatalf("TLS security level argument = %q, want /tls:seclevel:0", freeRDPTLSSecLevelArg)
	}
}

// currentDisplaySize answers (0, 0) without a WindowServer session (go test
// over SSH, a headless CI job) and preferredRDPDesktopSize then substitutes a
// fixed size; the session window must be usable either way.
func TestPreferredRDPDesktopSizeDarwinIsSaneForASessionWindow(t *testing.T) {
	if w, h := currentDisplaySize(); w == 0 && h == 0 {
		t.Log("no display in this session; checking the fallback size")
	}
	width, height := preferredRDPDesktopSize()
	if width < 1024 || height < 768 {
		t.Fatalf("desktop size %dx%d below the minimum session size", width, height)
	}
	if width%2 != 0 || height%2 != 0 {
		t.Fatalf("desktop size %dx%d must be even", width, height)
	}
}

func TestCredentialFailureDetector(t *testing.T) {
	cases := []struct {
		name   string
		writes []string
		want   bool
	}{
		// What an older Windows host (CredSSP 5) produces for a wrong password.
		{"tls access denied", []string{"[ERROR][com.freerdp.core] - [transport_ssl_cb]: ERRCONNECT_AUTHENTICATION_FAILED [0x00020009]\n"}, true},
		{"logon failure", []string{"[ERROR][com.freerdp.core] - [nla_recv_pdu]: ERRCONNECT_LOGON_FAILURE [0x00020014]\n"}, true},
		{"wrong password", []string{"ERRCONNECT_WRONG_PASSWORD [0x00020016]\n"}, true},
		{"marker split across writes", []string{"noise noise ERRCONNECT_AUTHENTI", "CATION_FAILED [0x00020009]\n"}, true},
		{"marker after a long write", []string{strings.Repeat("x", 10000), "ERRCONNECT_LOGON_", "FAILURE\n"}, true},
		{"cancelled prompt", []string{"ERRCONNECT_CONNECT_CANCELLED [0x0002000B]\n"}, false},
		{"locked account", []string{"ERRCONNECT_ACCOUNT_LOCKED_OUT [0x00020017]\n"}, false},
		{"tls failure", []string{"ERRCONNECT_TLS_CONNECT_FAILED [0x00020008]\n"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detector := &credentialFailureDetector{}
			for _, w := range tc.writes {
				if n, err := detector.Write([]byte(w)); n != len(w) || err != nil {
					t.Fatalf("Write = %d, %v", n, err)
				}
			}
			if got := detector.Seen(); got != tc.want {
				t.Fatalf("Seen() = %v, want %v", got, tc.want)
			}
		})
	}
}

// fakeMacClient writes a client stand-in that records each run and prints
// the given FreeRDP output; HOME points at a temp dir so the client log lands
// there instead of the real state directory.
func fakeMacClient(t *testing.T, output string) (client string, runs func() []string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	record := filepath.Join(dir, "runs")
	client = filepath.Join(dir, "MacFreeRDP")
	script := "#!/bin/sh\necho \"$*\" >> '" + record + "'\necho '" + output + "' >&2\n"
	if err := os.WriteFile(client, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return client, func() []string {
		data, _ := os.ReadFile(record)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

// A rejected password brings the prompt back — without the stored password,
// which was just rejected — until the attempts run out.
func TestSuperviseMacRDPAsksAgainAfterRejectedCredentials(t *testing.T) {
	client, runs := fakeMacClient(t, "[ERROR] ERRCONNECT_AUTHENTICATION_FAILED [0x00020009]")
	run, err := startMacRDPClient(client, []string{"/v:host:3389", "/u:someone"}, "stored-secret")
	if err != nil {
		t.Fatal(err)
	}
	superviseMacRDP(client, []string{"/v:host:3389", "/u:someone"}, run)

	got := runs()
	if len(got) != maxRDPCredentialAttempts {
		t.Fatalf("client ran %d times, want %d: %q", len(got), maxRDPCredentialAttempts, got)
	}
	if !strings.Contains(got[0], freeRDPStdinPasswordArg) {
		t.Fatalf("first run should use the stored password: %q", got[0])
	}
	for _, retry := range got[1:] {
		if strings.Contains(retry, freeRDPStdinPasswordArg) || !strings.Contains(retry, "/u:someone") {
			t.Fatalf("a retry must prompt (no stored password) for the same user: %q", retry)
		}
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "access-workspace-launcher", "logs", "rdp-client.log"))
	if err != nil || !strings.Contains(string(data), "ERRCONNECT_AUTHENTICATION_FAILED") {
		t.Fatalf("client output should still reach rdp-client.log: %q, %v", data, err)
	}
}

func TestSuperviseMacRDPStopsWhenNotACredentialFailure(t *testing.T) {
	client, runs := fakeMacClient(t, "[ERROR] ERRCONNECT_CONNECT_CANCELLED [0x0002000B]")
	run, err := startMacRDPClient(client, []string{"/v:host:3389"}, "")
	if err != nil {
		t.Fatal(err)
	}
	superviseMacRDP(client, []string{"/v:host:3389"}, run)
	if got := runs(); len(got) != 1 {
		t.Fatalf("client ran %d times after a cancelled prompt, want 1: %q", len(got), got)
	}
}
