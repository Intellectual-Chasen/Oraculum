package squid_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 状態符号 200 の後ろに要求の byte 数 65 と応答の byte 数 42 を並べる。
const requestBytesLine = `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] ` +
	`"GET http://198.51.100.9:8080/a?q=1 HTTP/1.1" 200 65 42 "-" "agent" TCP_MISS:HIER_DIRECT`

func readWithLayout(
	t *testing.T, input string, layout squid.Layout,
) (squid.Record, *core.ImportFailure) {
	t.Helper()
	var reader squid.Reader
	reader.Reset(strings.NewReader(input), layout)
	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading one record: %v", err)
	}
	return record, failure
}

func itemValue(t *testing.T, record squid.Record, name squid.ItemName) string {
	t.Helper()
	item, found := record.Item(name)
	if !found {
		t.Fatalf("the record carries no item named %q", name)
	}
	return item.Value()
}

// 要求の byte 数を持つ並びは、状態符号の次の欄を要求の byte 数、その次を応答の byte 数と
// して読む。
func TestReaderReadsTheRequestBytesLayout(t *testing.T) {
	record, failure := readWithLayout(t, requestBytesLine, squid.LayoutCombinedRequestBytes)
	if failure != nil {
		t.Fatalf("tokenizing the request bytes layout: %+v", failure)
	}
	if got := itemValue(t, record, squid.ItemStatusCode); got != "200" {
		t.Errorf("the status code = %q, want 200", got)
	}
	if got := itemValue(t, record, squid.ItemRequestBytes); got != "65" {
		t.Errorf("the request bytes = %q, want 65", got)
	}
	if got := itemValue(t, record, squid.ItemReplyBytes); got != "42" {
		t.Errorf("the reply bytes = %q, want 42", got)
	}
	// 後続の欄が 1 つずれていないことを確かめる。
	if got := itemValue(t, record, squid.ItemUserAgent); got != "agent" {
		t.Errorf("the user agent = %q, want agent", got)
	}
	if got := itemValue(t, record, squid.ItemSquidStatus); got != "TCP_MISS:HIER_DIRECT" {
		t.Errorf("the Squid status = %q, want TCP_MISS:HIER_DIRECT", got)
	}
}

// 欄の数が並びと異なる行を成功にしない。**片方の並びで読める行を、もう片方の並びが
// 通知せずに受け取らない。**
func TestReaderRejectsTheLineOfTheOtherLayout(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		line   string
		layout squid.Layout
	}{
		{"the combined layout reading a request bytes line",
			requestBytesLine, squid.LayoutCombined},
		{"the request bytes layout reading a combined line",
			exampleLine, squid.LayoutCombinedRequestBytes},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, failure := readWithLayout(t, testCase.line, testCase.layout)
			if failure == nil {
				t.Fatal("the reader accepted a line whose item count differs from the layout")
			}
			if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
				t.Errorf("the failure carries the diagnosis %q, want undetermined",
					failure.DiagnosisClass)
			}
			if failure.Stage != core.FailureStageTokenize {
				t.Errorf("the failure carries the stage %q, want tokenize", failure.Stage)
			}
		})
	}
}

// 要求の byte 数の欄は語彙の項目を持ち、combined の並びには欄そのものが無い。
func TestRequestBytesCarriesTheSemanticAndIsAbsentFromCombined(t *testing.T) {
	if got := squid.SemanticOfItem(squid.ItemRequestBytes); got !=
		core.SemanticKeyHttpRequestBytes {
		t.Errorf("the semantic of the request bytes = %q, want http.request_bytes", got)
	}
	record, failure := readWithLayout(t, exampleLine, squid.LayoutCombined)
	if failure != nil {
		t.Fatalf("tokenizing the combined layout: %+v", failure)
	}
	if _, found := record.Item(squid.ItemRequestBytes); found {
		t.Error("the combined layout carries a request bytes item")
	}
}

// 2 つの並びの欄の一覧が、文字列で挙げた通りになる。
func TestItemOrderOfNamesTheItemsOfEachLayout(t *testing.T) {
	combined := squid.ItemOrderOf(squid.LayoutCombined)
	withRequestBytes := squid.ItemOrderOf(squid.LayoutCombinedRequestBytes)
	requireItemNames(t, "combined", combined,
		"clientIp", "ident", "user", "requestTime", "requestLine",
		"statusCode", "replyBytes", "referer", "userAgent", "squidStatus")
	requireItemNames(t, "combined with request bytes", withRequestBytes,
		"clientIp", "ident", "user", "requestTime", "requestLine",
		"statusCode", "requestBytes", "replyBytes", "referer", "userAgent", "squidStatus")
	if order := squid.ItemOrderOf(squid.Layout{}); len(order) != 0 {
		t.Errorf("a layout without items names %v, want no item", order)
	}
	// 返した slice の変更が表に及ばない。
	combined[0] = "changed"
	if again := squid.ItemOrderOf(squid.LayoutCombined); again[0] != "clientIp" {
		t.Errorf("the table kept %q after the caller changed the returned slice", again[0])
	}
}

func requireItemNames(t *testing.T, label string, got []squid.ItemName, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s names %d items, want %d (%v)", label, len(got), len(want), got)
	}
	for index, name := range want {
		if string(got[index]) != name {
			t.Errorf("%s item %d = %q, want %q", label, index, got[index], name)
		}
	}
}
