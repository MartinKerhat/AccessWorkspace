package artifacts

import (
	"strconv"
	"strings"
)

// CompareVersions compares dotted numeric versions ("0.5.8" style, optional
// leading "v"). Missing or non-numeric segments count as 0. Returns -1 when
// a < b, 0 when equal, 1 when a > b.
func CompareVersions(a, b string) int {
	aParts := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	bParts := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	length := len(aParts)
	if len(bParts) > length {
		length = len(bParts)
	}
	for i := 0; i < length; i++ {
		aValue, bValue := 0, 0
		if i < len(aParts) {
			aValue, _ = strconv.Atoi(strings.TrimSpace(aParts[i]))
		}
		if i < len(bParts) {
			bValue, _ = strconv.Atoi(strings.TrimSpace(bParts[i]))
		}
		if aValue != bValue {
			if aValue < bValue {
				return -1
			}
			return 1
		}
	}
	return 0
}

// NewestVersion returns the highest version parsed from the artifacts, or ""
// when none of them carries a parseable version.
func NewestVersion(items []Artifact) string {
	best := ""
	for _, item := range items {
		version := item.Version
		if version == "" {
			version = ParseVersion(item.Name)
		}
		if version == "" {
			continue
		}
		if best == "" || CompareVersions(version, best) > 0 {
			best = version
		}
	}
	return best
}
