package artifacts

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// LocalSource lists artifacts from a directory tree rooted at Dir. The files are
// served as static content by the frontend under BaseURL + "/downloads/". In
// production Dir is a mounted volume; in development it is a local folder.
type LocalSource struct {
	Dir     string // filesystem root containing the category prefixes
	BaseURL string // origin that serves /downloads/ (e.g. the frontend URL)
}

func NewLocalSource(dir, baseURL string) *LocalSource {
	return &LocalSource{Dir: dir, BaseURL: strings.TrimRight(baseURL, "/")}
}

func (s *LocalSource) List(_ context.Context, category Category) ([]Artifact, error) {
	dir := filepath.Join(s.Dir, filepath.FromSlash(category.Prefix))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Artifact{}, nil
		}
		return nil, err
	}

	items := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !category.AllowsExt(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		items = append(items, Artifact{
			Name:        entry.Name(),
			Category:    category.Key,
			Version:     ParseVersion(entry.Name()),
			SizeBytes:   info.Size(),
			ModifiedAt:  info.ModTime().UTC().Format(time.RFC3339),
			DownloadURL: s.downloadURL(category.Prefix, entry.Name()),
		})
	}
	sortNewestFirst(items)
	return items, nil
}

func (s *LocalSource) downloadURL(prefix, name string) string {
	return s.BaseURL + "/downloads/" + path.Join(prefix, name)
}
