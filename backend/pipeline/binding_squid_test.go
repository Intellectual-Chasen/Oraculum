package pipeline_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const squidScanLine = `192.0.2.1 - - [02/Jan/2024:03:04:05 +0000] "GET http://example.test/ HTTP/1.1" 200 1 "-" "agent" TCP_MISS:HIER_DIRECT`

// 走査器は、読んだ欄の並びを、対応する入力形式のバージョンと並びの指定の両方に名乗る。
func TestSquidParserIdentity(t *testing.T) {
	const combinedSpec = `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st ` +
		`"%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`
	got := pipeline.NewTestSquidParser().Identity()
	if got.ParserID != "squid-combined" || got.SupportedFormatVersion != combinedSpec ||
		got.FormatSpec != combinedSpec ||
		got.FormatKey != pipeline.SquidFormatKey || got.PositionKind != core.PositionKindLineNumber {
		t.Errorf("identity = %+v", got)
	}
}

// 別の並びで読んだ走査器は、別の並びの指定を名乗る。parserVersion の材料が分かれる。
func TestSquidLogFormatParserCarriesTheRequestedSpec(t *testing.T) {
	const spec = `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %>st %<st ` +
		`"%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh %mt`
	got := pipeline.NewTestSquidLogFormatParser(spec).Identity()
	if got.FormatKey != pipeline.SquidLogFormatKey {
		t.Errorf("the format key = %q, want %q", got.FormatKey, pipeline.SquidLogFormatKey)
	}
	if got.FormatSpec != spec {
		t.Errorf("the format spec = %q, want the requested spec", got.FormatSpec)
	}
	if got.SupportedFormatVersion != spec {
		t.Errorf("the supported format version = %q, want the requested spec",
			got.SupportedFormatVersion)
	}
}

// 取り込みの指定が渡した並びの走査器は、既定の並びに無い欄を意味付けまで持つ。
//
// ヘッダーの欄は logformat が書いた名前を持ち、上位との間の値の欄は語彙の項目を
// 持たない。既定の並びの欄と導いた欄はそのまま揃う。
func TestSquidLogFormatParserCarriesTheItemsOfTheRequestedSpec(t *testing.T) {
	const spec = `%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %<Hs %<st ` +
		`"%{User-Agent}>h" "%{X-Forwarded-For}>h" %<a %mt`
	const line = `192.0.2.1 [02/Jan/2024:03:04:05 +0000] "GET http://example.test/ HTTP/1.1" ` +
		`200 504 1 "agent" "198.51.100.7, 203.0.113.9" 198.51.100.1 text/html`

	parser := pipeline.NewTestSquidLogFormatParser(spec)
	parser.Reset(strings.NewReader(line))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}

	want := map[string]struct {
		text     string
		semantic core.SemanticKey
	}{
		"clientIp":                      {"192.0.2.1", core.SemanticKeyConnectionSourceAddress},
		"statusCode":                    {"200", core.SemanticKeyHttpStatusCode},
		"upstreamStatusCode":            {"504", ""},
		"replyBytes":                    {"1", core.SemanticKeyHttpResponseBytes},
		"userAgent":                     {`"agent"`, core.SemanticKeyHttpUserAgent},
		"requestHeader.X-Forwarded-For": {`"198.51.100.7, 203.0.113.9"`, ""},
		"upstreamIp":                    {"198.51.100.1", ""},
		"mimeType":                      {"text/html", ""},
		"requestMethod":                 {"GET", core.SemanticKeyHttpRequestMethod},
	}
	for _, field := range record.Semantics.Fields {
		expected, named := want[field.Name]
		if !named {
			continue
		}
		delete(want, field.Name)
		if field.Text == nil || field.Text.RawText == nil || *field.Text.RawText != expected.text {
			t.Errorf("%s carries %+v, want the raw text %q", field.Name, field.Text, expected.text)
		}
		if field.Semantic != expected.semantic {
			t.Errorf("%s names the semantic %q, want %q", field.Name, field.Semantic, expected.semantic)
		}
	}
	for name := range want {
		t.Errorf("the record carries no field named %q", name)
	}
}

// 日時の書式を指定した %tl と、引用符の無い %rm %ru を持つ並びを意味付けまで持つ。
//
// method は %rm の欄が 1 件だけ持ち、要求行の HTTP のバージョンは持たない。時刻は UTC からのずれを
// 持たないミリ秒の時刻になる。
func TestSquidLogFormatParserCarriesTheSeparateRequestItems(t *testing.T) {
	const spec = `%{%Y/%m/%d %H:%M:%S}tl.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`
	const line = `2002/02/01 03:25:07.412     37 192.0.2.5 TCP_TUNNEL/200 3072 CONNECT host.example.test:443 - HIER_DIRECT/198.51.100.25 -`
	parser := pipeline.NewTestSquidLogFormatParser(spec)
	identity := parser.Identity()
	if identity.TimePrecision != core.PrecisionMillisecond {
		t.Errorf("the time precision = %q, want millisecond", identity.TimePrecision)
	}
	for _, semantic := range []core.SemanticKey{
		core.SemanticKeyHttpRequestMethod, core.SemanticKeyConnectionDestinationHostname,
		core.SemanticKeyConnectionDestinationPort, core.SemanticKeyEventTime,
	} {
		if !containsSemantic(identity.ItemSemantics, semantic) {
			t.Errorf("the item semantics %v lack %q", identity.ItemSemantics, semantic)
		}
	}
	if containsSemantic(identity.ItemSemantics, core.SemanticKeyHttpRequestVersion) {
		t.Error("the item semantics name the request version without a request line")
	}
	parser.Reset(strings.NewReader(line))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.ObservedAt == nil || *record.ObservedAt.Normalized != "2002-02-01T03:25:07.412" ||
		record.ObservedAt.OffsetState != core.OffsetStateItemAbsent {
		t.Errorf("the observed time = %+v, want the local time without an offset", record.ObservedAt)
	}
	counts := map[string]int{}
	values := map[string]string{}
	for _, field := range record.Semantics.Fields {
		counts[field.Name]++
		if field.Text != nil && field.Text.Normalized != nil {
			values[field.Name] = *field.Text.Normalized
		} else if field.Text != nil && field.Text.RawText != nil {
			values[field.Name] = *field.Text.RawText
		}
	}
	for name, want := range map[string]string{
		"requestMethod": "CONNECT", "requestTargetHost": "host.example.test", "requestTargetPort": "443",
		"squidRequestStatus": "TCP_TUNNEL", "hierarchyStatus": "HIER_DIRECT", "responseTime": "37",
	} {
		if counts[name] != 1 || values[name] != want {
			t.Errorf("the %s field appears %d times with %q, want once with %q", name, counts[name], values[name], want)
		}
	}
	if counts["requestVersion"] != 0 {
		t.Error("the record carries a request version without a request line")
	}
	if record.Semantics.Endpoint == nil {
		t.Error("the record carries no connection endpoint")
	}
}

// %ru だけを持ち、%rm も要求行の欄も持たない並びは、要求先から host と port を導き、
// method と要求行の HTTP のバージョンの項目を足さない。
func TestSquidLogFormatParserReadsATargetWithoutMethod(t *testing.T) {
	parser := pipeline.NewTestSquidLogFormatParser("referrer")
	parser.Reset(strings.NewReader(
		"1012533907.412 192.0.2.1 http://example.test/from http://target.example.test:8080/to"))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	names := map[string]string{}
	for _, field := range record.Semantics.Fields {
		if field.Text != nil && field.Text.Normalized != nil {
			names[field.Name] = *field.Text.Normalized
		} else {
			names[field.Name] = ""
		}
	}
	if names["requestTargetHost"] != "target.example.test" || names["requestTargetPort"] != "8080" {
		t.Errorf("the derived target = %q:%q, want target.example.test:8080",
			names["requestTargetHost"], names["requestTargetPort"])
	}
	for _, absent := range []string{"requestMethod", "requestVersion"} {
		if _, found := names[absent]; found {
			t.Errorf("the record carries the %s field without a method item", absent)
		}
	}
}

// 時刻の欄と要求先の欄を持たない並びは、時刻と接続の組を持たずにレコードを持つ。
func TestSquidLogFormatParserReadsALayoutWithoutTimeOrTarget(t *testing.T) {
	parser := pipeline.NewTestSquidLogFormatParser(`%>a %Ss`)
	parser.Reset(strings.NewReader("192.0.2.1 TCP_MISS"))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.ObservedAt != nil || record.Semantics == nil || record.Semantics.Endpoint != nil {
		t.Errorf("the record = %+v, want fields without a time or an endpoint", record)
	}
}

func containsSemantic(semantics []core.SemanticKey, want core.SemanticKey) bool {
	for _, semantic := range semantics {
		if semantic == want {
			return true
		}
	}
	return false
}

func TestSquidParserResetAndPositions(t *testing.T) {
	parser := pipeline.NewTestSquidParser()
	for range 2 {
		parser.Reset(strings.NewReader(squidScanLine + "\r\n" + squidScanLine))
		for index := range 2 {
			record, failure, err := parser.Next()
			if err != nil || failure != nil {
				t.Fatalf("Next = %+v, %v", failure, err)
			}
			wantEnding, wantOffset := "\r\n", int64(0)
			if index == 1 {
				wantEnding, wantOffset = "", int64(len(squidScanLine)+2)
			}
			if record.RawText != squidScanLine || record.LineEnding != wantEnding || record.ByteOffset != wantOffset || record.LineNumber != int64(index+1) || record.SequenceNumber != nil {
				t.Errorf("record position or original text = %+v", record)
			}
			if record.ObservedAt == nil || record.ObservedAt.Normalized == nil || *record.ObservedAt.Normalized != "2024-01-02T03:04:05Z" {
				t.Errorf("observed time = %+v", record.ObservedAt)
			}
		}
		record, failure, err := parser.Next()
		empty := record.RawText == "" && record.LineNumber == 0 && record.SequenceNumber == nil &&
			record.ObservedAt == nil && record.Semantics == nil && len(record.Terminal) == 0
		if !errors.Is(err, io.EOF) || failure != nil || !empty {
			t.Errorf("end = %+v, %+v, %v", record, failure, err)
		}
	}
}

func TestSquidParserFailuresKeepRawText(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		stage   core.FailureStage
		hasTime bool
	}{
		{"tokenize", "broken", core.FailureStageTokenize, false},
		{"time", strings.Replace(squidScanLine, "02/Jan/2024", "99/Jan/2024", 1), core.FailureStageNormalize, false},
		{"request", strings.Replace(squidScanLine, "HTTP/1.1", "INVALID", 1), core.FailureStageNormalize, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := pipeline.NewTestSquidParser()
			parser.Reset(strings.NewReader(tc.input + "\n" + squidScanLine))
			record, failure, err := parser.Next()
			if err != nil || failure == nil {
				t.Fatalf("Next = %+v, %v", failure, err)
			}
			if failure.Stage != tc.stage || record.RawText != tc.input || record.LineNumber != 1 || record.SequenceNumber != nil || (record.ObservedAt != nil) != tc.hasTime {
				t.Errorf("failure result = %+v, %+v", record, failure)
			}
			record, failure, err = parser.Next()
			if err != nil || failure != nil || record.LineNumber != 2 || record.RawText != squidScanLine {
				t.Errorf("following record = %+v, %+v, %v", record, failure, err)
			}
		})
	}
}

func TestSquidParserReadFailureKeepsDiagnosis(t *testing.T) {
	cause := errors.New("source interrupted")
	parser := pipeline.NewTestSquidParser()
	parser.Reset(io.MultiReader(strings.NewReader("fragment"), iotest.ErrReader(cause)))
	record, failure, err := parser.Next()
	if !errors.Is(err, cause) || failure == nil {
		t.Fatalf("Next = %+v, %v", failure, err)
	}
	if record.RawText != "fragment" || record.LineNumber != 1 || failure.Stage != core.FailureStageRead {
		t.Errorf("interrupted record = %+v, %+v", record, failure)
	}
}
