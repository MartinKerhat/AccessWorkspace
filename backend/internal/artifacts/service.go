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
}

func NewService(source Source, chromeStoreURL, firefoxStoreURL string) *Service {
	return &Service{
		source:          source,
		chromeStoreURL:  strings.TrimSpace(chromeStoreURL),
		firefoxStoreURL: strings.TrimSpace(firefoxStoreURL),
	}
}

// PackageView is one browser-extension option presented to the UI.
type PackageView struct {
	ID          string     `json:"id"`
	Browser     string     `json:"browser"`
	Variant     string     `json:"variant,omitempty"`
	Label       string     `json:"label"`
	PackageType string     `json:"packageType"` // "store" | "zip" | "xpi" | "app"
	Status      string     `json:"status"`      // "available" | "unavailable" | "planned"
	InstallURL  string     `json:"installUrl,omitempty"`
	ActionLabel string     `json:"actionLabel"`
	Notes       string     `json:"notes"`
	DownloadURL string     `json:"downloadUrl,omitempty"` // newest file
	Files       []Artifact `json:"files"`
}

// LauncherDownloads returns every launcher build across launcher categories,
// newest first.
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
	return all, nil
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
	views := make([]PackageView, 0, len(ExtensionCategories)+1)

	for _, category := range ExtensionCategories {
		files, err := s.source.List(ctx, category)
		if err != nil {
			return nil, err
		}
		views = append(views, s.packageView(category, files))
	}

	// Safari has no downloadable package yet.
	views = append(views, PackageView{
		ID: "extension-safari", Browser: "safari", Label: "Safari",
		PackageType: "app", Status: "planned", Files: []Artifact{},
		Notes: "Planned. Safari needs a native Safari Web Extension wrapper plus Apple signing and notarization.",
	})

	return views, nil
}

func (s *Service) packageView(category Category, files []Artifact) PackageView {
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
	}

	return view
}

func availabilityFor(files []Artifact) string {
	if len(files) > 0 {
		return "available"
	}
	return "unavailable"
}
