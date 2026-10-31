package artifacts

import (
	"context"
	"io"
	"net/url"
	"strings"
)

// allCategories is every launcher + extension category, used to resolve a
// category by key for proxy downloads.
func allCategories() []Category {
	return append(append([]Category{}, LauncherCategories...), ExtensionCategories...)
}

func categoryByKey(key string) (Category, bool) {
	for _, category := range allCategories() {
		if category.Key == key {
			return category, true
		}
	}
	return Category{}, false
}

// proxyDownloadURL is the backend endpoint that streams an artifact. Downloads
// flow through the API so the underlying store (Blob/volume) stays private.
func proxyDownloadURL(categoryKey, name string) string {
	return "/api/artifacts/download/" + categoryKey + "/" + url.PathEscape(name)
}

// Service turns raw source listings into the views the API exposes. It applies
// the "store-primary" rule: when a browser store URL is configured, that store
// becomes the primary install action and direct download is the fallback.
type Service struct {
	source          Source
	chromeStoreURL  string
	firefoxStoreURL string
	safariStoreURL  string
}

// NewService wires the artifact source and the optional store listings
// (Chrome Web Store, Firefox Add-ons, Mac App Store); an empty URL means
// direct download for that browser.
func NewService(source Source, chromeStoreURL, firefoxStoreURL, safariStoreURL string) *Service {
	return &Service{
		source:          source,
		chromeStoreURL:  strings.TrimSpace(chromeStoreURL),
		firefoxStoreURL: strings.TrimSpace(firefoxStoreURL),
		safariStoreURL:  strings.TrimSpace(safariStoreURL),
	}
}

// PackageView is one browser-extension option presented to the UI.
type PackageView struct {
	ID          string     `json:"id"`
	Browser     string     `json:"browser"`
	Variant     string     `json:"variant,omitempty"`
	Label       string     `json:"label"`
	PackageType string     `json:"packageType"` // "store" | "zip" | "xpi" | "dmg"
	Status      string     `json:"status"`      // "available" | "unavailable"
	InstallURL  string     `json:"installUrl,omitempty"`
	ActionLabel string     `json:"actionLabel"`
	Notes       string     `json:"notes"`
	DownloadURL string     `json:"downloadUrl,omitempty"` // newest file
	Files       []Artifact `json:"files"`
}

// LauncherDownloads returns the newest launcher build of each platform, newest
// version first. Older builds are not listed (see newestPerCategory); the
// archive, when the source has one, is linked via LauncherReleasesURL.
func (s *Service) LauncherDownloads(ctx context.Context) ([]Artifact, error) {
	var all []Artifact
	for _, category := range LauncherCategories {
		items, err := s.source.List(ctx, category)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	for i := range all {
		all[i].DownloadURL = proxyDownloadURL(all[i].Category, all[i].Name)
	}
	sortNewestFirst(all)
	return newestPerCategory(all), nil
}

// LauncherReleasesURL is the browsable archive of all published launcher
// builds, or "" when the configured source has none (local dir, blob).
func (s *Service) LauncherReleasesURL() string {
	if archiver, ok := s.source.(ReleaseArchiver); ok {
		return archiver.ReleasesURL("launcher-v")
	}
	return ""
}

// Open streams a single artifact's bytes for the backend download proxy.
func (s *Service) Open(ctx context.Context, categoryKey, name string) (io.ReadCloser, *ObjectInfo, error) {
	category, ok := categoryByKey(categoryKey)
	if !ok || !category.AllowsExt(name) {
		return nil, nil, ErrNotFound
	}
	return s.source.Open(ctx, category, name)
}

// ExtensionPackages returns the browser-extension options, store-primary where a
// store URL is configured.
func (s *Service) ExtensionPackages(ctx context.Context) ([]PackageView, error) {
	views := make([]PackageView, 0, len(ExtensionCategories))

	for _, category := range ExtensionCategories {
		files, err := s.source.List(ctx, category)
		if err != nil {
			return nil, err
		}
		views = append(views, s.packageView(category, files))
	}

	return views, nil
}

func (s *Service) packageView(category Category, files []Artifact) PackageView {
	// Only the newest package per category is offered, same as the launcher.
	sortNewestFirst(files)
	files = newestPerCategory(files)
	for i := range files {
		files[i].DownloadURL = proxyDownloadURL(files[i].Category, files[i].Name)
	}
	view := PackageView{
		ID:      category.Key,
		Browser: category.Browser,
		Variant: category.Variant,
		Files:   files,
	}
	if len(files) > 0 {
		view.DownloadURL = files[0].DownloadURL
	}

	storeURL := ""
	switch category.Browser {
	case "chrome":
		storeURL = s.chromeStoreURL
	case "firefox":
		if category.Variant == "signed" {
			storeURL = s.firefoxStoreURL
		}
	case "safari":
		storeURL = s.safariStoreURL
	}

	switch category.Key {
	case CategoryExtensionChrome.Key:
		view.Label = "Chrome / Edge"
		if storeURL != "" {
			view.PackageType, view.Status = "store", "available"
			view.InstallURL, view.ActionLabel = storeURL, "Install from Chrome Web Store"
			view.Notes = "Install from the store listing, then connect the browser from the app."
		} else {
			view.PackageType = "zip"
			view.Status = availabilityFor(files)
			view.ActionLabel = "Download ZIP"
			view.Notes = "Chrome and Edge normally install from the Chrome Web Store or enterprise policy. Direct ZIP download is developer-only."
		}
	case CategoryExtensionFirefoxSigned.Key:
		view.Label = "Firefox (signed)"
		if storeURL != "" {
			view.PackageType, view.Status = "store", "available"
			view.InstallURL, view.ActionLabel = storeURL, "Install Firefox add-on"
			view.Notes = "Install the Mozilla-listed add-on, then connect the browser from the app."
		} else {
			view.PackageType = "xpi"
			view.Status = availabilityFor(files)
			view.ActionLabel = "Download signed XPI"
			view.Notes = "Mozilla-signed Firefox add-on. Recommended for normal Firefox users."
		}
	case CategoryExtensionFirefoxUnsigned.Key:
		view.Label = "Firefox (unsigned)"
		view.PackageType = "xpi"
		view.Status = availabilityFor(files)
		view.ActionLabel = "Download unsigned XPI"
		view.Notes = "Developer build. Sign it through Mozilla before distributing to normal Firefox users."
	case CategoryExtensionSafari.Key:
		view.Label = "Safari"
		if storeURL != "" {
			view.PackageType, view.Status = "store", "available"
			view.InstallURL, view.ActionLabel = storeURL, "Install from the Mac App Store"
			view.Notes = "Install the Mac app from the App Store, open it once and turn the extension on in Safari Settings › Extensions, then connect the browser from the app."
		} else {
			view.PackageType = "dmg"
			view.Status = availabilityFor(files)
			view.ActionLabel = "Download for macOS"
			view.Notes = "Safari extensions ship inside a small Mac app. Open the disk image, drag the app to Applications, open it once, then turn the extension on in Safari Settings › Extensions and allow it on this site when Safari asks."
		}
	}

	return view
}

func availabilityFor(files []Artifact) string {
	if len(files) > 0 {
		return "available"
	}
	return "unavailable"
}
