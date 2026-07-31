package artifacts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newGitHubTestServer(t *testing.T, requestCount *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/repos/acme/workspace/releases", func(w http.ResponseWriter, r *http.Request) {
		if requestCount != nil {
			*requestCount++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"draft": false, "assets": [
				{"name": "access-workspace-browser-extension-chrome-v0.2.9.zip", "size": 100, "updated_at": "2026-07-28T10:00:00Z", "browser_download_url": "` + server.URL + `/dl/chrome.zip"},
				{"name": "access-workspace-browser-extension-firefox-signed-v0.2.9.xpi", "size": 200, "updated_at": "2026-07-28T10:00:00Z", "browser_download_url": "` + server.URL + `/dl/firefox.xpi"},
				{"name": "access-workspace-browser-extension-firefox-v0.2.9.xpi", "size": 150, "updated_at": "2026-07-28T10:00:00Z", "browser_download_url": "` + server.URL + `/dl/firefox-unsigned.xpi"},
				{"name": "access-workspace-launcher-windows-amd64-v0.6.1.exe", "size": 300, "updated_at": "2026-07-20T10:00:00Z", "browser_download_url": "` + server.URL + `/dl/launcher.exe"},
				{"name": "release-notes.txt", "size": 5, "updated_at": "2026-07-28T10:00:00Z", "browser_download_url": "` + server.URL + `/dl/notes.txt"}
			]},
			{"draft": true, "assets": [
				{"name": "access-workspace-launcher-windows-amd64-v9.9.9.exe", "size": 1, "updated_at": "2026-07-28T11:00:00Z", "browser_download_url": "` + server.URL + `/dl/draft.exe"}
			]}
		]`))
	})
	mux.HandleFunc("/dl/chrome.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("zip-bytes"))
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newTestGitHubSource(t *testing.T, requestCount *int) *GitHubSource {
	t.Helper()
	server := newGitHubTestServer(t, requestCount)
	source := NewGitHubSource("acme/workspace", "")
	source.apiBase = server.URL
	return source
}

func TestGitHubSource_ListMatchesCategories(t *testing.T) {
	source := newTestGitHubSource(t, nil)
	ctx := context.Background()

	cases := []struct {
		category Category
		want     string
	}{
		{CategoryExtensionChrome, "access-workspace-browser-extension-chrome-v0.2.9.zip"},
		{CategoryExtensionFirefoxSigned, "access-workspace-browser-extension-firefox-signed-v0.2.9.xpi"},
		{CategoryExtensionFirefoxUnsigned, "access-workspace-browser-extension-firefox-v0.2.9.xpi"},
		{CategoryLauncherWindows, "access-workspace-launcher-windows-amd64-v0.6.1.exe"},
	}
	for _, tc := range cases {
		items, err := source.List(ctx, tc.category)
		if err != nil {
			t.Fatalf("%s: %v", tc.category.Key, err)
		}
		if len(items) != 1 || items[0].Name != tc.want {
			t.Errorf("%s: got %+v, want single %q", tc.category.Key, items, tc.want)
		}
	}

	// The draft release's launcher asset must not leak in.
	items, _ := source.List(ctx, CategoryLauncherWindows)
	if items[0].Version != "0.6.1" {
		t.Errorf("expected non-draft version 0.6.1, got %q", items[0].Version)
	}
	// Linux launcher: no matching asset published.
	linux, err := source.List(ctx, CategoryLauncherLinux)
	if err != nil || len(linux) != 0 {
		t.Errorf("expected empty linux list, got %v (err %v)", linux, err)
	}
}

func TestGitHubSource_OpenStreamsAsset(t *testing.T) {
	source := newTestGitHubSource(t, nil)

	body, info, err := source.Open(context.Background(), CategoryExtensionChrome, "access-workspace-browser-extension-chrome-v0.2.9.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	bytes, _ := io.ReadAll(body)
	if string(bytes) != "zip-bytes" {
		t.Errorf("body = %q", string(bytes))
	}
	if info.ContentType != "application/zip" || info.Size != 100 {
		t.Errorf("info = %+v", info)
	}

	if _, _, err := source.Open(context.Background(), CategoryExtensionChrome, "no-such-file.zip"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	// A name from the wrong category must not resolve.
	if _, _, err := source.Open(context.Background(), CategoryExtensionChrome, "access-workspace-browser-extension-firefox-signed-v0.2.9.xpi"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-category open, got %v", err)
	}
}

func TestGitHubSource_CachesListing(t *testing.T) {
	requests := 0
	source := newTestGitHubSource(t, &requests)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := source.List(ctx, CategoryExtensionChrome); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 1 {
		t.Errorf("expected 1 upstream request thanks to caching, got %d", requests)
	}
}
