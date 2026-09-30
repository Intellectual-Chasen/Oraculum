package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 宣言 1 つ分の材料。test が決めた値で、原資料から転記していない。
func declaredFormat() core.InputFormat {
	return core.InputFormat{
		Key: "vendor_format_v1", ParserID: "vendor-reader",
		PositionKind: core.PositionKindLineNumber,
		SpecInput:    core.FormatSpecInputRejected, FormatSpec: "%a %b",
	}
}

// 必須の項目を欠いた宣言と、定義に無い列挙の値を持つ宣言を退ける。
func TestInputFormatValidateRejectsAnIncompleteDeclaration(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		change func(*core.InputFormat)
	}{
		{"no key", func(f *core.InputFormat) { f.Key = "" }},
		{"no parser", func(f *core.InputFormat) { f.ParserID = "" }},
		{"an unknown position kind", func(f *core.InputFormat) { f.PositionKind = "page" }},
		{"an unknown spec input", func(f *core.InputFormat) { f.SpecInput = "optional" }},
		{"an item order on a format taking it from the request", func(f *core.InputFormat) {
			f.SpecInput = core.FormatSpecInputRequired
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			format := declaredFormat()
			testCase.change(&format)
			if err := format.Validate(); err == nil {
				t.Fatalf("the declaration %+v passed the check", format)
			}
		})
	}
}

// 必須の項目が揃った宣言を通す。並びを指定から受け取る形式は、自身の並びを持たない。
func TestInputFormatValidateAcceptsADeclaration(t *testing.T) {
	for _, format := range []core.InputFormat{
		declaredFormat(),
		{
			Key: "vendor_format_spec", ParserID: "vendor-reader",
			PositionKind: core.PositionKindSequenceNumber,
			SpecInput:    core.FormatSpecInputRequired,
		},
	} {
		if err := format.Validate(); err != nil {
			t.Errorf("the declaration %+v: %v", format, err)
		}
	}
}

// 並びが決まっている形式は宣言の並びを返し、取り込みの指定が渡した並びを受け付けない。
func TestFormatSpecForRejectsASpecOnAFixedLayout(t *testing.T) {
	format := declaredFormat()
	spec, err := format.FormatSpecFor(nil)
	if err != nil {
		t.Fatalf("reading a source of a fixed layout: %v", err)
	}
	if spec != format.FormatSpec {
		t.Errorf("the item order = %q, want the declared one %q", spec, format.FormatSpec)
	}

	requested := "%a %b %c"
	if _, err := format.FormatSpecFor(&requested); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("passing an item order to a fixed layout returned %v, want ErrUnexpectedItem", err)
	}
}

// 並びを指定から受け取る形式は、指定の無い取り込みと空の指定を受け付けない。
func TestFormatSpecForRequiresASpec(t *testing.T) {
	format := declaredFormat()
	format.SpecInput, format.FormatSpec = core.FormatSpecInputRequired, ""

	requested := "%a %b %c"
	spec, err := format.FormatSpecFor(&requested)
	if err != nil {
		t.Fatalf("reading a source with the requested item order: %v", err)
	}
	if spec != requested {
		t.Errorf("the item order = %q, want the requested one %q", spec, requested)
	}

	empty := ""
	for _, absent := range []*string{nil, &empty} {
		if _, err := format.FormatSpecFor(absent); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("reading a source without an item order returned %v, want ErrMissingRequiredItem", err)
		}
	}
}

// 定義に無い列挙の値を持つ宣言は、並びを決められない。
func TestFormatSpecForRejectsAnUnknownSpecInput(t *testing.T) {
	format := declaredFormat()
	format.SpecInput = "optional"
	if _, err := format.FormatSpecFor(nil); !errors.Is(err, core.ErrUnknownEnumValue) {
		t.Errorf("an unknown spec input returned %v, want ErrUnknownEnumValue", err)
	}
}

// 2 値のいずれかであるかを、両側の値で確かめる。
func TestFormatSpecInputIsKnown(t *testing.T) {
	for _, known := range []core.FormatSpecInput{
		core.FormatSpecInputRejected, core.FormatSpecInputRequired,
	} {
		if !known.IsKnown() {
			t.Errorf("%q is outside the definition", known)
		}
	}
	for _, unknown := range []core.FormatSpecInput{"", "optional"} {
		if unknown.IsKnown() {
			t.Errorf("%q is inside the definition", unknown)
		}
	}
}
