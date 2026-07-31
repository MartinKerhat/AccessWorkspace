package artifacts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GitHubSource lists artifacts from a GitHub repository's Releases, so any
// deployment of the app can serve the project's published launcher and
// browser-extension builds without operating its own artifact store. Release
// assets live in a flat namespace, so categories match by the artifact naming
// convention (Category.MatchesAsset) instead of folder prefixes.
//
// The release listing is cached briefly: the GitHub API rate-limits
// unauthenticated clients to 60 requests/hour/IP, while the backend lists
// artifacts on every launcher-runtime call and ticket resolve. An optional
// token raises the limit and allows private repositories.
type GitHubSource struct {
	repo    string // "owner/name"
	token   string // optional
	apiBase string // overridable for tests; default https://api.github.com
	client  *http.Client

	mu      sync.Mutex
	cached  []githubAsset
	expires time.Time
}

const githubCacheTTL = 5 * time.Minute

func NewGitHubSource(repo, token string) *GitHubSource {
	return &GitHubSource{
		repo:    strings.Trim(strings.TrimSpace(repo), "/"),
		token:   strings.TrimSpace(token),
		apiBase: "https://api.github.com",
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	UpdatedAt          string `json:"updated_at"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	Draft  bool          `json:"draft"`
	Assets []githubAsset `json:"assets"`
}

// assets returns the flattened asset list across recent releases, cached for
// githubCacheTTL. On refresh failure the last known listing keeps serving, so
// a GitHub hiccup never empties the app's download offers.
func (s *GitHubSource) assets(ctx context.Context) ([]githubAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.expires) {
		return s.cached, nil
	}

	listURL := fmt.Sprintf("%s/repos/%s/releases?per_page=50", s.apiBase, s.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if s.cached != nil {
			return s.cached, nil
		}
		return nil, fmt.Errorf("list github releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if s.cached != nil {
			return s.cached, nil
		}
		return nil, fmt.Errorf("list github releases: unexpected status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		if s.cached != nil {
			return s.cached, nil
		}
		return nil, fmt.Errorf("decode github releases: %w", err)
	}

	var all []githubAsset
	for _, release := range releases {
		if release.Draft {
			continue
		}
		all = append(all, release.Assets...)
	}
	s.cached = all
	s.expires = time.Now().Add(githubCacheTTL)
	return all, nil
}

func (s *GitHubSource) List(ctx context.Context, category Category) ([]Artifact, error) {
	assets, err := s.assets(ctx)
	if err != nil {
		return nil, err
	}
	var items []Artifact
	for _, asset := range assets {
		if !category.MatchesAsset(asset.Name) {
			continue
		}
		items = append(items, Artifact{
			Name:        asset.Name,
			Category:    category.Key,
			Version:     ParseVersion(asset.Name),
			SizeBytes:   asset.Size,
			ModifiedAt:  asset.UpdatedAt,
			DownloadURL: asset.BrowserDownloadURL,
		})
	}
	sortNewestFirst(items)
	return items, nil
}

func (s *GitHubSource) Open(ctx context.Context, category Category, name string) (io.ReadCloser, *ObjectInfo, error) {
	if !safeArtifactName(name) || !category.MatchesAsset(name) {
		return nil, nil, ErrNotFound
	}
	assets, err := s.assets(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, asset := range assets {
		if asset.Name != name {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
		if err != nil {
			return nil, nil, err
		}
		if s.token != "" {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("download github release asset: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				return nil, nil, ErrNotFound
			}
			return nil, nil, fmt.Errorf("download github release asset: unexpected status %d", resp.StatusCode)
		}
		return resp.Body, &ObjectInfo{
			Name:        name,
			ContentType: contentTypeFor(name),
			Size:        asset.Size,
		}, nil
	}
	return nil, nil, ErrNotFound
}
