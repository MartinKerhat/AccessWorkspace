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

func TestSortNewestFirst_VersionBeatsModifiedTime(t *testing.T) {
	items := []Artifact{
		{Name: "launcher-v0.6.3.exe", Version: "0.6.3", ModifiedAt: "2026-10-05T10:00:00Z"}, // re-uploaded old build
		{Name: "launcher-v0.6.10.exe", Version: "0.6.10", ModifiedAt: "2026-09-01T10:00:00Z"},
		{Name: "launcher-v0.6.9.exe", Version: "0.6.9", ModifiedAt: "2026-08-01T10:00:00Z"},
	}
	sortNewestFirst(items)
	got := []string{items[0].Version, items[1].Version, items[2].Version}
	want := []string{"0.6.10", "0.6.9", "0.6.3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestNewestPerCategory(t *testing.T) {
	items := []Artifact{
		{Name: "w-0.6.5", Category: "launcher-windows", Version: "0.6.5"},
		{Name: "l-0.6.5", Category: "launcher-linux", Version: "0.6.5"},
		{Name: "w-0.6.4", Category: "launcher-windows", Version: "0.6.4"},
		{Name: "l-0.6.4", Category: "launcher-linux", Version: "0.6.4"},
	}
	got := newestPerCategory(items)
	if len(got) != 2 || got[0].Name != "w-0.6.5" || got[1].Name != "l-0.6.5" {
		t.Fatalf("newestPerCategory = %+v, want newest windows then newest linux", got)
	}
	if got := newestPerCategory(nil); got != nil {
		t.Fatalf("nil in should stay nil, got %+v", got)
	}
}

func TestService_LauncherDownloadsNewestPerPlatformOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "launcher/windows/access-workspace-launcher-windows-amd64-v0.6.4.exe")
	writeFile(t, root, "launcher/windows/access-workspace-launcher-windows-amd64-v0.6.5.exe")
	writeFile(t, root, "launcher/windows/access-workspace-launcher-windows-amd64-v0.6.3.exe")
	writeFile(t, root, "launcher/linux/access-workspace-launcher-linux-amd64-v0.6.4.tar.gz")
	writeFile(t, root, "launcher/linux/access-workspace-launcher-linux-amd64-v0.6.5.tar.gz")

	svc := NewService(NewLocalSource(root, "http://frontend"), "", "")
	downloads, err := svc.LauncherDownloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 {
		t.Fatalf("expected one build per platform, got %d: %+v", len(downloads), downloads)
	}
	for _, d := range downloads {
		if d.Version != "0.6.5" {
			t.Errorf("%s: version = %q, want 0.6.5", d.Category, d.Version)
		}
		if want := "/api/artifacts/download/" + d.Category + "/" + d.Name; d.DownloadURL != want {
			t.Errorf("downloadURL = %q, want %q", d.DownloadURL, want)
		}
	}
	if got := NewestVersion(downloads); got != "0.6.5" {
		t.Errorf("NewestVersion = %q, want 0.6.5", got)
	}
	// A local directory has no browsable archive.
	if url := svc.LauncherReleasesURL(); url != "" {
		t.Errorf("local source releases URL = %q, want empty", url)
	}
}

func TestService_ExtensionFilesNewestOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "extensions/chrome/access-workspace-browser-extension-chrome-v0.2.10.zip")
	writeFile(t, root, "extensions/chrome/access-workspace-browser-extension-chrome-v0.2.11.zip")
	svc := NewService(NewLocalSource(root, "http://frontend"), "", "")
	pkgs, err := svc.ExtensionPackages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if pkg.ID != CategoryExtensionChrome.Key {
			continue
		}
		if len(pkg.Files) != 1 || pkg.Files[0].Version != "0.2.11" {
			t.Fatalf("expected only the newest chrome package, got %+v", pkg.Files)
		}
		if pkg.DownloadURL != pkg.Files[0].DownloadURL {
			t.Errorf("package downloadUrl = %q, want newest file %q", pkg.DownloadURL, pkg.Files[0].DownloadURL)
		}
	}
}

func TestGitHubSource_ReleasesURL(t *testing.T) {
	src := NewGitHubSource("owner/repo", "")
	if got, want := src.ReleasesURL("launcher-v"), "https://github.com/owner/repo/releases?q=launcher-v&expanded=true"; got != want {
		t.Errorf("ReleasesURL = %q, want %q", got, want)
	}
	svc := NewService(src, "", "")
	if svc.LauncherReleasesURL() == "" {
		t.Error("github-backed service should expose a releases URL")
	}
}
