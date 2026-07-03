package artifacts

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BlobSource lists artifacts from an Azure Blob Storage container using the
// List Blobs REST API (no Azure SDK dependency). Downloads point directly at the
// blob URL. A SAS token grants list + read access; if the container allows
// anonymous access the token may be empty.
type BlobSource struct {
	ContainerURL string // e.g. https://account.blob.core.windows.net/artifacts
	SAS          string // SAS query string (with or without a leading '?'); may be empty
	Client       *http.Client
}

func NewBlobSource(containerURL, sas string) *BlobSource {
	return &BlobSource{
		ContainerURL: strings.TrimRight(containerURL, "/"),
		SAS:          strings.TrimPrefix(strings.TrimSpace(sas), "?"),
		Client:       &http.Client{Timeout: 15 * time.Second},
	}
}

type blobList struct {
	Blobs struct {
		Blob []struct {
			Name       string `xml:"Name"`
			Properties struct {
				LastModified  string `xml:"Last-Modified"`
				ContentLength int64  `xml:"Content-Length"`
			} `xml:"Properties"`
		} `xml:"Blob"`
	} `xml:"Blobs"`
}

func (s *BlobSource) List(ctx context.Context, category Category) ([]Artifact, error) {
	listURL := s.ContainerURL + "?restype=container&comp=list&prefix=" + url.QueryEscape(category.Prefix+"/")
	if s.SAS != "" {
		listURL += "&" + s.SAS
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure blob list failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var parsed blobList
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	items := make([]Artifact, 0, len(parsed.Blobs.Blob))
	for _, blob := range parsed.Blobs.Blob {
		name := blob.Name[strings.LastIndex(blob.Name, "/")+1:]
		if name == "" || !category.AllowsExt(name) {
			continue
		}
		items = append(items, Artifact{
			Name:        name,
			Category:    category.Key,
			Version:     ParseVersion(name),
			SizeBytes:   blob.Properties.ContentLength,
			ModifiedAt:  normalizeBlobTime(blob.Properties.LastModified),
			DownloadURL: s.downloadURL(blob.Name),
		})
	}
	sortNewestFirst(items)
	return items, nil
}

func (s *BlobSource) downloadURL(blobName string) string {
	segments := strings.Split(blobName, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	full := s.ContainerURL + "/" + strings.Join(segments, "/")
	if s.SAS != "" {
		full += "?" + s.SAS
	}
	return full
}

func normalizeBlobTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC1123, raw); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}
