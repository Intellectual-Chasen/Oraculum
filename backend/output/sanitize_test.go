package output_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"empty", "", ""},
		{"plain", "text 100% /path", "text 100% /path"},
		{"unicode", "日本語 e\u0301 が", "日本語 e\u0301 が"},
		{"controls", "a\n\r\x00\x1b\t\x7fz", `a\x0a\x0d\x00\x1b\x09\x7fz`},
		{"newline", "\n", `\x0a`},
		{"c1", "\u0085", `\xc2\x85`},
		{"invalid", "\xff", `\xff`},
		{"truncated_utf8", "日\xe3\x81", `日\xe3\x81`},
		{"replacement_rune", "\ufffd", "\ufffd"},
		{"literal_escape", `\x0a`, `\x0a`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := output.Sanitize(tt.input)
			if got != tt.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if strings.ContainsFunc(got, unicode.IsControl) {
				t.Fatalf("control character in %q", got)
			}
		})
	}
}

func FuzzSanitize(f *testing.F) {
	f.Fuzz(func(t *testing.T, text string) {
		got := output.Sanitize(text)
		if strings.ContainsFunc(got, unicode.IsControl) {
			t.Fatalf("control character in %q", got)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 in %q", got)
		}
		if twice := output.Sanitize(got); twice != got {
			t.Fatalf("Sanitize(%q) = %q, want %q", got, twice, got)
		}
	})
}
