package macdialog

import "testing"

func TestAppleScriptString(t *testing.T) {
	cases := map[string]string{
		`plain`:                 `"plain"`,
		`say "hi"`:              `"say \"hi\""`,
		`C:\path`:               `"C:\\path"`,
		"two\nlines":            `"two" & linefeed & "lines"`,
		`" & do shell script "`: `"\" & do shell script \""`,
	}
	for in, want := range cases {
		if got := AppleScriptString(in); got != want {
			t.Errorf("AppleScriptString(%q) = %s, want %s", in, got, want)
		}
	}
}
