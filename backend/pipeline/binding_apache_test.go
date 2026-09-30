package pipeline_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const apacheAccessScanLine = `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET /files/archive.zip HTTP/1.1" 200 2048 "-" "example-agent/1.0"`

const apacheErrorScanLine = `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] AH01215: example message`

// 走査器は、読める形式と観測の種別を持たないことを名乗る。combined は自身の端末の識別子を
// 持たないため、接続の関連付けの宣言を持たない。
func TestApacheAccessParserIdentity(t *testing.T) {
	got := pipeline.NewTestApacheAccessParser().Identity()
	if got.FormatKey != pipeline.ApacheAccessCombinedFormatKey {
		t.Errorf("FormatKey = %q, want %q", got.FormatKey, pipeline.ApacheAccessCombinedFormatKey)
	}
	if got.PositionKind != core.PositionKindLineNumber {
		t.Errorf("PositionKind = %q, want %q", got.PositionKind, core.PositionKindLineNumber)
	}
	if got.TimePrecision != core.PrecisionSecond {
		t.Errorf("TimePrecision = %q, want %q", got.TimePrecision, core.PrecisionSecond)
	}
	if len(got.ConnectionRequestKinds) != 0 {
		t.Errorf("ConnectionRequestKinds = %v, want an empty set", got.ConnectionRequestKinds)
	}
	if len(got.ConnectionMatchConditions) != 0 {
		t.Errorf("ConnectionMatchConditions = %v, want an empty set", got.ConnectionMatchConditions)
	}
}

func TestApacheErrorParserIdentity(t *testing.T) {
	got := pipeline.NewTestApacheErrorParser().Identity()
	if got.FormatKey != pipeline.ApacheErrorFormatKey {
		t.Errorf("FormatKey = %q, want %q", got.FormatKey, pipeline.ApacheErrorFormatKey)
	}
	if got.TimePrecision != core.PrecisionMicrosecond {
		t.Errorf("TimePrecision = %q, want %q", got.TimePrecision, core.PrecisionMicrosecond)
	}
}

// ファイルを取得する GET の要求が、要求行から導いた 3 項目とともに応答の fields に載る。
func TestApacheAccessParserCarriesRequestLineFields(t *testing.T) {
	parser := pipeline.NewTestApacheAccessParser()
	parser.Reset(strings.NewReader(apacheAccessScanLine))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next() = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}
	if record.Semantics.Endpoint != nil {
		t.Error("the record must not carry an Endpoint; combined does not identify its own terminal")
	}
	want := map[string]struct {
		text     string
		semantic core.SemanticKey
	}{
		"clientIp":      {"203.0.113.10", core.SemanticKeyConnectionSourceAddress},
		"statusCode":    {"200", core.SemanticKeyHttpStatusCode},
		"replyBytes":    {"2048", core.SemanticKeyHttpResponseBytes},
		"requestMethod": {"GET", core.SemanticKeyHttpRequestMethod},
		"requestTarget": {"/files/archive.zip", core.SemanticKeyHttpRequestUrl},
	}
	for _, field := range record.Semantics.Fields {
		expected, named := want[field.Name]
		if !named {
			continue
		}
		if field.Semantic != expected.semantic {
			t.Errorf("field %s semantic = %q, want %q", field.Name, field.Semantic, expected.semantic)
		}
		if field.Text == nil {
			t.Errorf("field %s carries no text", field.Name)
			continue
		}
		got, ok := field.Text.RawTextValue()
		if !ok || got != expected.text {
			t.Errorf("field %s RawTextValue() = %q, %v, want %q, true", field.Name, got, ok, expected.text)
		}
		delete(want, field.Name)
	}
	for name := range want {
		t.Errorf("field %s is absent from the record's fields", name)
	}
	if record.ObservedAt == nil {
		t.Fatal("ObservedAt is absent")
	}
}

func TestApacheErrorParserCarriesFields(t *testing.T) {
	parser := pipeline.NewTestApacheErrorParser()
	parser.Reset(strings.NewReader(apacheErrorScanLine))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next() = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}
	if record.ObservedAt == nil {
		t.Fatal("ObservedAt is absent")
	}
	if precision := record.ObservedAt.Precision; precision != core.PrecisionMicrosecond {
		t.Errorf("ObservedAt.Precision = %q, want %q", precision, core.PrecisionMicrosecond)
	}
	if offsetState := record.ObservedAt.OffsetState; offsetState != core.OffsetStateItemAbsent {
		t.Errorf("ObservedAt.OffsetState = %q, want %q", offsetState, core.OffsetStateItemAbsent)
	}
}
