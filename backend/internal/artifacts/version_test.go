package artifacts

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.5.7", "0.5.8", -1},
		{"0.5.8", "0.5.7", 1},
		{"0.5.8", "0.5.8", 0},
		{"0.5.10", "0.5.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.5", "0.5.0", 0},
		{"v0.5.8", "0.5.8", 0},
		{"", "0.0.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNewestVersion(t *testing.T) {
	items := []Artifact{
		{Name: "launcher-v0.5.7.exe"},
		{Name: "launcher-v0.5.10.exe"},
		{Name: "launcher-v0.5.9.exe"},
		{Name: "no-version.exe"},
	}
	if got := NewestVersion(items); got != "0.5.10" {
		t.Errorf("NewestVersion = %q, want 0.5.10", got)
	}
	if got := NewestVersion([]Artifact{{Name: "no-version.exe"}}); got != "" {
		t.Errorf("NewestVersion without versions = %q, want empty", got)
	}
}
