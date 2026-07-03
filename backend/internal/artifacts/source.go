package artifacts

import "fmt"

// Config selects and configures an artifact Source.
type Config struct {
	Source  string // "local" or "blob"
	Dir     string // local: filesystem root
	BaseURL string // local: origin that serves /downloads/ (frontend URL)
	BlobURL string // blob: container URL
	BlobSAS string // blob: SAS token (optional)
}

// NewSource builds the configured Source.
func NewSource(cfg Config) (Source, error) {
	switch cfg.Source {
	case "local":
		return NewLocalSource(cfg.Dir, cfg.BaseURL), nil
	case "blob":
		return NewBlobSource(cfg.BlobURL, cfg.BlobSAS), nil
	default:
		return nil, fmt.Errorf("unknown artifacts source %q", cfg.Source)
	}
}
