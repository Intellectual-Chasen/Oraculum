package pipeline_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 要求行から導く 3 件が、Squid の adapter が決めた語彙の項目を持つ。
//
// squidScanLine の要求先はホスト名を書くため、接続先の項目は接続先ホスト名になる。
func TestSquidParserCarriesTheSemanticOfTheDerivedItems(t *testing.T) {
	parser := pipeline.NewTestSquidParser()
	parser.Reset(strings.NewReader(squidScanLine))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}

	want := map[string]core.SemanticKey{
		"requestMethod":     core.SemanticKeyHttpRequestMethod,
		"requestTargetHost": core.SemanticKeyConnectionDestinationHostname,
		"requestTargetPort": core.SemanticKeyConnectionDestinationPort,
		"requestTarget":     core.SemanticKeyHttpRequestUrl,
	}
	found := 0
	for _, field := range record.Semantics.Fields {
		expected, known := want[field.Name]
		if !known {
			continue
		}
		found++
		if field.Semantic != expected {
			t.Errorf("the semantic of %q = %q, want %q", field.Name, field.Semantic, expected)
		}
	}
	if found != len(want) {
		t.Fatalf("the record carries %d of the derived items, want %d", found, len(want))
	}
}

// 要求の byte 数を持つ形式の行は、要求と応答の byte 数を別の項目で持つ。
func TestSquidRequestBytesParserCarriesBothByteCounts(t *testing.T) {
	const line = `192.0.2.1 - - [02/Jan/2024:03:04:05 +0000] ` +
		`"GET http://example.test/ HTTP/1.1" 200 65 42 "-" "agent" TCP_MISS:HIER_DIRECT`
	identity := pipeline.NewTestSquidRequestBytesParser().Identity()
	if identity.FormatKey != pipeline.SquidRequestBytesFormatKey ||
		identity.PositionKind != core.PositionKindLineNumber {
		t.Errorf("identity = %+v", identity)
	}
	parser := pipeline.NewTestSquidRequestBytesParser()
	parser.Reset(strings.NewReader(line))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}
	want := map[string]struct {
		semantic core.SemanticKey
		value    string
	}{
		"requestBytes": {core.SemanticKeyHttpRequestBytes, "65"},
		"replyBytes":   {core.SemanticKeyHttpResponseBytes, "42"},
	}
	found := 0
	for _, field := range record.Semantics.Fields {
		expected, known := want[field.Name]
		if !known {
			continue
		}
		found++
		if field.Semantic != expected.semantic {
			t.Errorf("the semantic of %q = %q, want %q",
				field.Name, field.Semantic, expected.semantic)
		}
		if field.Text == nil {
			t.Fatalf("the field %q carries no text value", field.Name)
		}
		if got, readable := field.Text.ComparableValue(); !readable || got != expected.value {
			t.Errorf("the value of %q = %q (readable %v), want %q",
				field.Name, got, readable, expected.value)
		}
	}
	if found != len(want) {
		t.Fatalf("the record carries %d of the two byte counts, want %d", found, len(want))
	}
	// combined の形式で同じ行を読むと失敗する。
	combined := pipeline.NewTestSquidParser()
	combined.Reset(strings.NewReader(line))
	if _, combinedFailure, _ := combined.Next(); combinedFailure == nil {
		t.Error("the combined parser accepted a line carrying the request bytes")
	}
}
