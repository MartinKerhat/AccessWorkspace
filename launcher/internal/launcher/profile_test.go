package launcher

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func writeUTF16LE(t *testing.T, path string, content string) {
	t.Helper()
	units := utf16.Encode([]rune(content))
	raw := make([]byte, 0, 2+len(units)*2)
	raw = append(raw, 0xFF, 0xFE)
	for _, unit := range units {
		raw = binary.LittleEndian.AppendUint16(raw, unit)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write utf-16 fixture: %v", err)
	}
}

// rdpsign.exe rewrites the profile as UTF-16LE with a BOM. Reading it back as
// UTF-8 made every comparison fail, so the profile was rewritten and re-signed
// on every launch and its signature was never detected.
func TestReadRDPProfileTextDecodesSignedProfileEncoding(t *testing.T) {
	dir := t.TempDir()
	content := "full address:s:host:3389\r\nusername:s:someone\r\nsignature:s:AQABAAE=\r\n"

	utf16Path := filepath.Join(dir, "utf16.rdp")
	writeUTF16LE(t, utf16Path, content)
	decoded, err := readRDPProfileText(utf16Path)
	if err != nil {
		t.Fatalf("read utf-16 profile: %v", err)
	}
	if decoded != content {
		t.Fatalf("expected the utf-16 profile to decode to the original text, got %q", decoded)
	}
	if !rdpProfileHasSignature(decoded) {
		t.Fatal("expected the signature line to be found in the decoded profile")
	}

	plainPath := filepath.Join(dir, "utf8.rdp")
	if err := os.WriteFile(plainPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write utf-8 fixture: %v", err)
	}
	decoded, err = readRDPProfileText(plainPath)
	if err != nil {
		t.Fatalf("read utf-8 profile: %v", err)
	}
	if decoded != content {
		t.Fatalf("expected plain utf-8 to round-trip unchanged, got %q", decoded)
	}

	bomPath := filepath.Join(dir, "utf8bom.rdp")
	if err := os.WriteFile(bomPath, append([]byte{0xEF, 0xBB, 0xBF}, []byte(content)...), 0o600); err != nil {
		t.Fatalf("write utf-8 bom fixture: %v", err)
	}
	decoded, err = readRDPProfileText(bomPath)
	if err != nil {
		t.Fatalf("read utf-8 bom profile: %v", err)
	}
	if decoded != content {
		t.Fatalf("expected the utf-8 BOM to be stripped, got %q", decoded)
	}
}

// The generated content must stay a prefix-match against what rdpsign leaves on
// disk — it appends its own lines at the end — otherwise the cache never hits.
func TestRDPProfileMatchesSignedProfileWithAppendedLines(t *testing.T) {
	generated := "full address:s:host:3389\r\nusername:s:someone\r\n"
	signed := generated + "alternate full address:s:host:3389\r\nsignscope:s:Full Address\r\nsignature:s:AQABAAE=\r\n"

	if !rdpProfileMatches(generated, signed) {
		t.Fatal("expected a signed profile to still match the content it was generated from")
	}
	if rdpProfileMatches("full address:s:other:3389\r\n", signed) {
		t.Fatal("expected a changed profile not to match")
	}
}
