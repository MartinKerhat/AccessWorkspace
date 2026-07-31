package artifacts

import "fmt"

// Config selects and configures an artifact Source.
type Config struct {
	Source      string // "local", "blob", or "github"
	Dir         string // local: filesystem root
	BaseURL     string // local: origin that serves /downloads/ (frontend URL)
	BlobURL     string // blob: container URL
	BlobSAS     string // blob: SAS token (optional)
	GitHubRepo  string // github: "owner/name" whose Releases carry the artifacts
	GitHubToken string // github: optional token (rate limits / private repos)
}

// NewSource builds the configured Source.
func NewSource(cfg Config) (Source, error) {
	switch cfg.Source {
	case "local":
		return NewLocalSource(cfg.Dir, cfg.BaseURL), nil
	case "blob":
		return NewBlobSource(cfg.BlobURL, cfg.BlobSAS), nil
	case "github":
		return NewGitHubSource(cfg.GitHubRepo, cfg.GitHubToken), nil
	default:
		return nil, fmt.Errorf("unknown artifacts source %q", cfg.Source)
	}
}
