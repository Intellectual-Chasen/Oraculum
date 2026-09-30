package squid_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// exampleSemantics は exampleLine の 10 欄の語彙の項目である。ident と requestLine と
// squidStatus は combined に固有の意味を持ち、空の値になる。
var exampleSemantics = map[string]core.SemanticKey{
	"clientIp":    core.SemanticKeyConnectionSourceAddress,
	"ident":       "",
	"user":        core.SemanticKeyAccountName,
	"requestTime": core.SemanticKeyEventTime,
	"requestLine": "",
	"statusCode":  core.SemanticKeyHttpStatusCode,
	"replyBytes":  core.SemanticKeyHttpResponseBytes,
	"referer":     core.SemanticKeyHttpReferrer,
	"userAgent":   core.SemanticKeyHttpUserAgent,
	"squidStatus": "",
}

// 10 欄が対応表の語彙の項目を持つ。
func TestRecordFieldsCarryTheSemanticOfEveryItem(t *testing.T) {
	record := readOK(t, exampleLine)
	requestTime, failure := squid.ParseRequestTime(record)
	if failure != nil {
		t.Fatal(failure)
	}
	fields := squid.RecordFields(record, requestTime)
	if len(fields) != len(exampleSemantics) {
		t.Fatalf("field count = %d, want %d", len(fields), len(exampleSemantics))
	}
	for _, field := range fields {
		want, known := exampleSemantics[field.Name]
		if !known {
			t.Fatalf("the record carries an unexpected field named %q", field.Name)
		}
		if field.Semantic != want {
			t.Errorf("the semantic of %q = %q, want %q", field.Name, field.Semantic, want)
		}
		if want != "" && !want.IsKnown() {
			t.Errorf("the semantic of %q = %q, which is outside the vocabulary",
				field.Name, want)
		}
	}
}

// 要求先の host の文字列の形が、接続先 IP アドレスと接続先ホスト名を分ける。
func TestRequestTargetHostSemanticReadsTheShapeOfTheHost(t *testing.T) {
	cases := map[string]struct {
		line string
		want core.SemanticKey
	}{
		"an IPv4 host": {
			line: `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] ` +
				`"GET http://198.51.100.9:8080/a HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`,
			want: core.SemanticKeyConnectionDestinationAddress,
		},
		"an IPv6 host": {
			line: `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] ` +
				`"CONNECT [2001:db8::1]:443 HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`,
			want: core.SemanticKeyConnectionDestinationAddress,
		},
		"a host name": {
			line: `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] ` +
				`"GET http://example.test/a HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`,
			want: core.SemanticKeyConnectionDestinationHostname,
		},
		"a request target without an authority": {
			line: `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] ` +
				`"GET /a HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`,
			want: "",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			requestLine, failure := squid.ParseRequestLine(readOK(t, testCase.line))
			if failure != nil {
				t.Fatal(failure)
			}
			got := squid.RequestTargetHostSemantic(requestLine)
			if got != testCase.want {
				t.Errorf("semantic = %q, want %q", got, testCase.want)
			}
			if testCase.want != "" && !got.IsKnown() {
				t.Errorf("semantic = %q, which is outside the vocabulary", got)
			}
		})
	}
}
