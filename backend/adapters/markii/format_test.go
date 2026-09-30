package markii_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 宣言した入力形式の識別子は、利用者が起動引数に書く文字列である。
func TestFormatsDeclareTheKeyUsersType(t *testing.T) {
	var format core.InputFormat
	for _, declared := range markii.Formats() {
		if declared.Key != markii.FormatKeyClientLog {
			t.Errorf("the package declares the unexpected key %q", declared.Key)
			continue
		}
		format = declared
	}
	if format.Key == "" {
		t.Fatal("the package declares no format for the client log")
	}
	if markii.FormatKeyClientLog != "infotrace_mark_ii" {
		t.Errorf("the key = %q, want infotrace_mark_ii", markii.FormatKeyClientLog)
	}
	if format.ParserID != markii.ParserID {
		t.Errorf("the parser = %q, want %q", format.ParserID, markii.ParserID)
	}
	if format.PositionKind != core.PositionKindSequenceNumber {
		t.Errorf("records are pointed by %q, want sequence_number", format.PositionKind)
	}
	// レコードが欄の key を持つため、欄の並びの指定を取らない。
	if format.SpecInput != core.FormatSpecInputRejected {
		t.Errorf("the spec input = %q, want rejected", format.SpecInput)
	}
	if format.FormatSpec != "" {
		t.Errorf("the declaration carries the item order %q", format.FormatSpec)
	}
	if err := format.Validate(); err != nil {
		t.Errorf("the declaration: %v", err)
	}
}

// 返した宣言の変更が、次に返す宣言に及ばない。
func TestFormatsReturnsASliceTheCallerCanChange(t *testing.T) {
	declared := markii.Formats()
	for index := range declared {
		declared[index].Key = "changed"
	}
	for _, again := range markii.Formats() {
		if again.Key == "changed" {
			t.Error("the declaration kept the change the caller made to the returned slice")
		}
	}
}
