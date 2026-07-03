// Package artifacts enumerates downloadable build outputs (the desktop launcher
// and browser-extension packages) from a pluggable source. Files are no longer
// hardcoded by name: a source lists whatever is present under a category's
// folder/prefix, filtered to the extensions that category allows.
//
// Two sources are provided:
//   - LocalSource   — a directory on disk (a mounted volume in production, a
//     local folder in development), served as static files by the frontend.
//   - BlobSource    — an Azure Blob Storage container, listed over the REST API
//     (no SDK dependency) and downloaded directly via blob URLs.
package artifacts

import (
	"context"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Category describes one kind of downloadable artifact and where it lives.
type Category struct {
	Key      string   // stable identifier used in the API and URL paths
	Prefix   string   // folder / blob prefix, e.g. "extensions/firefox/signed"
	Kind     string   // "launcher" or "extension"
	Browser  string   // "", "chrome", or "firefox"
	Variant  string   // "", "signed", or "unsigned"
	Platform string   // "", "windows", ...
	Exts     []string // allowed file extensions (lowercase, with dot)
}

// AllowsExt reports whether name has an extension this category accepts.
func (c Category) AllowsExt(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range c.Exts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// Artifact is a single downloadable file discovered by a Source.
type Artifact struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Version     string `json:"version,omitempty"`
	SizeBytes   int64  `json:"sizeBytes"`
	ModifiedAt  string `json:"modifiedAt,omitempty"` // RFC3339
	DownloadURL string `json:"downloadUrl"`
}

// Source lists artifacts for a category and streams their bytes. Each returned
// Artifact carries a DownloadURL; the API rewrites it to the backend proxy path.
type Source interface {
	List(ctx context.Context, category Category) ([]Artifact, error)
	// Open streams a single artifact's bytes so the backend can proxy the
	// download from a private store. Returns ErrNotFound if it does not exist.
	Open(ctx context.Context, category Category, name string) (io.ReadCloser, *ObjectInfo, error)
}

// ObjectInfo describes a single artifact being streamed.
type ObjectInfo struct {
	Name        string
	ContentType string
	Size        int64
}

// ErrNotFound is returned by Open when the requested artifact does not exist.
var ErrNotFound = errors.New("artifact not found")

// contentTypeFor maps a filename to a download-friendly content type.
func contentTypeFor(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "application/zip"
	case strings.HasSuffix(lower, ".xpi"):
		return "application/x-xpinstall"
	default:
		return "application/octet-stream"
	}
}

// safeArtifactName rejects names that could escape a category folder.
func safeArtifactName(name string) bool {
	return name != "" && !strings.Contains(name, "/") && !strings.Contains(name, `\`) && !strings.Contains(name, "..")
}

// Standard categories. The Key values are stable API identifiers.
var (
	CategoryLauncherWindows = Category{
		Key: "launcher-windows", Prefix: "launcher/windows",
		Kind: "launcher", Platform: "windows", Exts: []string{".exe"},
	}
	CategoryExtensionChrome = Category{
		Key: "extension-chrome", Prefix: "extensions/chrome",
		Kind: "extension", Browser: "chrome", Exts: []string{".zip"},
	}
	CategoryExtensionFirefoxSigned = Category{
		Key: "extension-firefox-signed", Prefix: "extensions/firefox/signed",
		Kind: "extension", Browser: "firefox", Variant: "signed", Exts: []string{".xpi"},
	}
	CategoryExtensionFirefoxUnsigned = Category{
		Key: "extension-firefox-unsigned", Prefix: "extensions/firefox/unsigned",
		Kind: "extension", Browser: "firefox", Variant: "unsigned", Exts: []string{".xpi"},
	}
)

// LauncherCategories and ExtensionCategories group the standard categories.
var (
	LauncherCategories  = []Category{CategoryLauncherWindows}
	ExtensionCategories = []Category{
		CategoryExtensionChrome,
		CategoryExtensionFirefoxSigned,
		CategoryExtensionFirefoxUnsigned,
	}
)

var versionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// ParseVersion extracts a semantic version like "0.2.5" from a filename, or "".
func ParseVersion(name string) string {
	if m := versionPattern.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// sortNewestFirst orders artifacts by modified time descending, then name.
func sortNewestFirst(items []Artifact) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ModifiedAt != items[j].ModifiedAt {
			return items[i].ModifiedAt > items[j].ModifiedAt
		}
		return items[i].Name > items[j].Name
	})
}
