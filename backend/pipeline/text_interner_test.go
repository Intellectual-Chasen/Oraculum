// in-package test: 非公開の複製の手順が文字列の値を共有するかを pointer で確かめる。
package pipeline

import (
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// internerTestText は文字列の値を組む。normalized が空の文字列のときは正規化値を持たない。
func internerTestText(rawText, normalized string) core.RawAndNormalized {
	value := core.RawAndNormalized{RawText: &rawText, ValueState: core.ValueStatePresent}
	if normalized != "" {
		value.Normalized = &normalized
	}
	return value
}

// **同じ文字列の値は 1 つの複製を共有し、元の値とは共有しない。** 値の有無と文字列のどちらかが
// 違う値は別の複製になる。
func TestTextInternerSharesEqualValuesOnly(t *testing.T) {
	texts := make(textInterner)
	first := internerTestText("pc01.example.test", "")
	second := internerTestText("pc01.example.test", "")
	shared := texts.text(first)
	if texts.text(second) != shared {
		t.Error("two equal values yield different copies, want one shared copy")
	}
	if shared.RawText == first.RawText {
		t.Error("the shared copy points at the original text, want a copy")
	}
	if !reflect.DeepEqual(*shared, first) {
		t.Errorf("the shared copy is %+v, want the value %+v", *shared, first)
	}
	distinct := map[string]core.RawAndNormalized{
		"another raw text":     internerTestText("pc02.example.test", ""),
		"with a normalized":    internerTestText("pc01.example.test", "pc01.example.test"),
		"another value state":  {RawText: first.RawText, ValueState: core.ValueStateDerived},
		"without the raw text": {ValueState: core.ValueStatePresent},
	}
	for name, value := range distinct {
		if texts.text(value) == shared {
			t.Errorf("%s: the value shares the copy of %+v", name, first)
		}
	}
	empty := ""
	if texts.text(core.RawAndNormalized{RawText: &empty, ValueState: core.ValueStatePresent}) ==
		texts.text(core.RawAndNormalized{ValueState: core.ValueStatePresent}) {
		t.Error("an empty text and an absent text share a copy, want different copies")
	}
}

// **レコードを cloneRecords と同じ値で複製し、文字列の値だけをレコードどうしで共有する。**
func TestTextInternerCopiesRecordsLikeCloneRecords(t *testing.T) {
	host := func() core.RecordField {
		value := internerTestText("pc01.example.test", "")
		return core.RecordField{Name: "host", Semantic: core.SemanticKeyTerminalHostname,
			Kind: core.RecordFieldKindText, Text: &value}
	}
	parent := internerTestText("4242", "")
	line := int64(3)
	records := []RecordEntry{
		{
			Locator: core.RecordLocator{SourceId: "src", LineNumber: &line},
			Semantics: &RecordSemantics{
				Fields:          []core.RecordField{host()},
				ParentProcessId: &parent,
				Endpoint:        &RecordEndpoint{Destination: []core.RecordField{host()}},
			},
			Terminal: []core.RecordField{host()},
		},
		{Semantics: &RecordSemantics{Fields: []core.RecordField{host()}}},
		{},
	}
	got := make(textInterner).records(records)
	if want := cloneRecords(records); !reflect.DeepEqual(got, want) {
		t.Fatalf("the interned copy is %+v, cloneRecords yields %+v", got, want)
	}
	shared := got[0].Semantics.Fields[0].Text
	for name, text := range map[string]*core.RawAndNormalized{
		"the destination":  got[0].Semantics.Endpoint.Destination[0].Text,
		"the terminal":     got[0].Terminal[0].Text,
		"the other record": got[1].Semantics.Fields[0].Text,
	} {
		if text != shared {
			t.Errorf("%s does not share the text of the first field", name)
		}
	}
	if shared == records[0].Semantics.Fields[0].Text || got[0].Locator.LineNumber == &line {
		t.Error("the copy shares a value with the input records, want copies")
	}
	if got[2].Semantics != nil || got[2].Terminal != nil {
		t.Errorf("a record without semantics yields %+v, want nil semantics and terminal", got[2])
	}
}
