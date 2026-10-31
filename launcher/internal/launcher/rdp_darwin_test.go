//go:build darwin

package launcher

import (
	"os"
	"path/filepath"
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
