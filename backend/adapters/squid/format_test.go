package squid_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 宣言した入力形式の識別子は、利用者が起動引数に書く文字列である。
// 上の層はこの文字列で adapter を探すため、文字列を exact value で確かめる。
func TestFormatsDeclareTheKeysUsersType(t *testing.T) {
	want := map[core.FormatKey]core.FormatSpecInput{
		"squid_combined":               core.FormatSpecInputRejected,
		"squid_combined_request_bytes": core.FormatSpecInputRejected,
		"squid_logformat":              core.FormatSpecInputRequired,
	}
	declared := squid.Formats()
	for _, format := range declared {
		specInput, known := want[format.Key]
		if !known {
			t.Errorf("the package declares the unexpected key %q", format.Key)
			continue
		}
		if format.SpecInput != specInput {
			t.Errorf("%q takes the spec input %q, want %q", format.Key, format.SpecInput, specInput)
		}
		if format.ParserID != squid.ParserID {
			t.Errorf("%q names the parser %q, want %q", format.Key, format.ParserID, squid.ParserID)
		}
		if format.PositionKind != core.PositionKindLineNumber {
			t.Errorf("%q points records by %q, want line_number", format.Key, format.PositionKind)
		}
		if err := format.Validate(); err != nil {
			t.Errorf("the declaration of %q: %v", format.Key, err)
		}
		delete(want, format.Key)
	}
	for key := range want {
		t.Errorf("the package declares no format for %q", key)
	}
}

// 並びを取り込みの指定から受け取る形式は、自身では並びを定めない。
// 並びが決まっている形式は、その並びを logformat の文字列で持つ。
func TestFormatsCarryTheSpecOfTheFixedLayoutsOnly(t *testing.T) {
	for _, format := range squid.Formats() {
		switch format.SpecInput {
		case core.FormatSpecInputRejected:
			if format.FormatSpec == "" {
				t.Errorf("%q declares no item order", format.Key)
			}
			if _, err := squid.LayoutForSpec(format.FormatSpec); err != nil {
				t.Errorf("the declared order of %q: %v", format.Key, err)
			}
		case core.FormatSpecInputRequired:
			if format.FormatSpec != "" {
				t.Errorf("%q declares the item order %q while taking it from the request",
					format.Key, format.FormatSpec)
			}
		}
	}
}

// 返した宣言の変更が、次に返す宣言に及ばない。
func TestFormatsReturnsASliceTheCallerCanChange(t *testing.T) {
	declared := squid.Formats()
	if len(declared) == 0 {
		t.Fatal("the package declares no format")
	}
	for index := range declared {
		declared[index].Key = "changed"
	}
	for _, again := range squid.Formats() {
		if again.Key == "changed" {
			t.Error("the declaration kept the change the caller made to the returned slice")
		}
	}
}
