package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"access-workspace-launcher-windows-amd64-v0.5.6.exe": "0.5.6",
		"ext-chrome-1.2.3.zip":                               "1.2.3",
		"no-version-here.xpi":                                "",
	}
	for name, want := range cases {
		if got := ParseVersion(name); got != want {
			t.Errorf("ParseVersion(%q) = %q, want %q", name, got, want)
		}
	}
}

func writeFile(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalSource_ListsAndFilters(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "extensions/firefox/signed/ext-v0.2.5.xpi")
	writeFile(t, root, "extensions/firefox/signed/readme.txt") // wrong ext, filtered out
	writeFile(t, root, "extensions/chrome/ext-v0.2.5.zip")

	src := NewLocalSource(root, "http://frontend")

	items, err := src.List(context.Background(), CategoryExtensionFirefoxSigned)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 firefox-signed artifact, got %d", len(items))
	}
	got := items[0]
	if got.Version != "0.2.5" {
		t.Errorf("version = %q, want 0.2.5", got.Version)
	}
	if want := "http://frontend/downloads/extensions/firefox/signed/ext-v0.2.5.xpi"; got.DownloadURL != want {
		t.Errorf("downloadURL = %q, want %q", got.DownloadURL, want)
	}
}

func TestLocalSource_MissingDirIsEmpty(t *testing.T) {
	src := NewLocalSource(t.TempDir(), "http://frontend")
	items, err := src.List(context.Background(), CategoryLauncherWindows)
	if err != nil {
		t.Fatalf("missing dir should not error, got: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestService_ExtensionStorePrimary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "extensions/chrome/ext-v0.2.5.zip")
	src := NewLocalSource(root, "http://frontend")

	// With a store URL, chrome is store-primary.
	svc := NewService(src, "https://chromewebstore.example/abc", "")
	pkgs, err := svc.ExtensionPackages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var chrome *PackageView
	for i := range pkgs {
		if pkgs[i].ID == CategoryExtensionChrome.Key {
			chrome = &pkgs[i]
		}
	}
	if chrome == nil {
		t.Fatal("chrome package missing")
	}
	if chrome.PackageType != "store" || chrome.InstallURL == "" {
		t.Errorf("expected store-primary chrome, got type=%q installURL=%q", chrome.PackageType, chrome.InstallURL)
	}
	// The direct-download file is still listed as a fallback.
	if len(chrome.Files) != 1 {
		t.Errorf("expected 1 fallback file, got %d", len(chrome.Files))
	}
}

func TestService_ExtensionDirectDownloadWhenNoStore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "extensions/chrome/ext-v0.2.5.zip")
	svc := NewService(NewLocalSource(root, "http://frontend"), "", "")
	pkgs, err := svc.ExtensionPackages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if pkg.ID == CategoryExtensionChrome.Key {
			if pkg.PackageType != "zip" || pkg.Status != "available" {
				t.Errorf("expected direct zip download, got type=%q status=%q", pkg.PackageType, pkg.Status)
			}
		}
	}
}
