package squid_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 組み込みの combined の logformat を読んだ並びが、combined の欄を原文の順で挙げた
// 通りになる。
func TestParseLogFormatBuildsTheCombinedOrder(t *testing.T) {
	layout, err := squid.ParseLogFormat(squid.CombinedSpec)
	if err != nil {
		t.Fatalf("parsing the built-in combined spec: %v", err)
	}
	requireItemNames(t, "the parsed combined", squid.ItemOrderOf(layout),
		"clientIp", "ident", "user", "requestTime", "requestLine",
		"statusCode", "replyBytes", "referer", "userAgent", "squidStatus")
	if layout.Spec() != squid.CombinedSpec {
		t.Errorf("the layout carries the spec %q, want the built-in combined spec", layout.Spec())
	}
}

// CombinedRequestBytesSpec を読んだ並びが、要求の byte 数を状態符号と応答 byte 数の間に持つ。
func TestParseLogFormatBuildsTheRequestBytesOrder(t *testing.T) {
	layout, err := squid.ParseLogFormat(squid.CombinedRequestBytesSpec)
	if err != nil {
		t.Fatalf("parsing the request bytes spec: %v", err)
	}
	requireItemNames(t, "the parsed request bytes layout", squid.ItemOrderOf(layout),
		"clientIp", "ident", "user", "requestTime", "requestLine",
		"statusCode", "requestBytes", "replyBytes", "referer", "userAgent", "squidStatus")
}

// 宣言が持つ並びと、同じ文字列を読んで作った並びの欄が一致する。
// 宣言の並びを読む経路と、取り込みの指定を読む経路が同じ結果を出す。
func TestDeclaredFormatSpecsBuildTheSameOrderAsTheLayouts(t *testing.T) {
	for _, format := range squid.Formats() {
		if format.SpecInput != core.FormatSpecInputRejected {
			continue
		}
		t.Run(string(format.Key), func(t *testing.T) {
			layout, err := squid.LayoutForSpec(format.FormatSpec)
			if err != nil {
				t.Fatalf("building the order of %q: %v", format.Key, err)
			}
			record, failure := readWithLayout(t, requestBytesLineFor(format.Key), layout)
			if format.Key == squid.FormatKeyCombinedRequestBytes && failure != nil {
				t.Fatalf("reading a line of %q: %+v", format.Key, failure)
			}
			if format.Key == squid.FormatKeyCombined && failure == nil {
				t.Fatal("the combined order accepted a line carrying the request bytes item")
			}
			_ = record
		})
	}
}

func requestBytesLineFor(core.FormatKey) string { return requestBytesLine }

// 要求と応答のヘッダーは、logformat が書いたヘッダー名を欄の名前に載せる。
// Referer と User-Agent は既存の欄の名前を保つ。
func TestParseLogFormatNamesTheHeaderItems(t *testing.T) {
	const spec = `%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" ` +
		`"%{X-Forwarded-For}>h" "%{Content-Type}<h"`
	layout, err := squid.ParseLogFormat(spec)
	if err != nil {
		t.Fatalf("parsing a spec with arbitrary headers: %v", err)
	}
	requireItemNames(t, "the header layout", squid.ItemOrderOf(layout),
		"clientIp", "requestTime", "requestLine", "statusCode", "replyBytes",
		"referer", "userAgent",
		"requestHeader.X-Forwarded-For", "responseHeader.Content-Type")
}

// 対応表に無い指定子と、欄の位置を決められない書き方を、それぞれ別の理由で退ける。
// **読み飛ばさない。** 読み飛ばすと後続の欄がすべてずれ、診断を出さずに別の値を取り込む。
func TestParseLogFormatRejectsUnreadableSpecs(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		spec   string
		reason string
		offset int
	}{
		{"an Apache specifier", `%h [%tl]`, "outside the supported set", 0},
		{"an unknown code in a namespace", `%>a %ssl::unknown`, "outside the supported set", 4},
		{"an unterminated brace", `%>a %{Referer>h`, "no closing brace", 4},
		{"an incomplete specifier", `%>a %`, "incomplete specifier", 4},
		{"no item", ``, "declares no item", 0},
		{"only literals", `- - -`, "declares no item", 0},
		{"no literal between two specifiers", `%>a%>Hs [%tl]`, "no literal between them", 0},
		{"an unquoted closing quote", `%>a "%{Referer}>h`, "no closing quote", 4},
		{"a header name outside the token bytes",
			`%>a "%{Bad Name}>h"`, "outside letters, digits", 5},
		{"a line break byte", `%>a %byte{10} %>Hs`, "line break", 4},
		{"a zero byte", `%>a %byte{0} %>Hs`, "from 1 to 255", 4},
		{"a truncated client address", `%.8>a %>Hs`, "cannot be truncated", 0},
		{"a truncated request target", `%>a %.20ru`, "cannot be truncated", 4},
		// 書式の中の %Q の位置は、logformat の先頭から 9 byte 目である。
		{"an unknown strftime conversion", `%>a %{%Y %Q}tl`, "", 9},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := squid.ParseLogFormat(testCase.spec)
			if err == nil {
				t.Fatal("the parser accepted a spec it cannot read")
			}
			var problem *squid.LogFormatError
			if !errors.As(err, &problem) {
				t.Fatalf("the error %v does not carry the position in the spec", err)
			}
			if !strings.Contains(problem.Reason, testCase.reason) {
				t.Errorf("the reason = %q, want it to carry %q", problem.Reason, testCase.reason)
			}
			if problem.Offset != testCase.offset {
				t.Errorf("the offset = %d, want %d", problem.Offset, testCase.offset)
			}
		})
	}
}

// 値の向きが違う指定子を別の欄にする。
//
// `%>` は client との間、`%<` は上位の server との間を指す。2 つを同じ欄にすると、
// 上位から受け取った値が client へ返した値の語彙を名乗る。
func TestParseLogFormatSeparatesTheTwoDirections(t *testing.T) {
	const spec = `%>a %<a [%tl] "%rm %ru HTTP/%rv" %>Hs %<Hs %>st %<st`
	layout, err := squid.ParseLogFormat(spec)
	if err != nil {
		t.Fatalf("parsing a spec carrying both directions: %v", err)
	}
	requireItemNames(t, "both directions", squid.ItemOrderOf(layout),
		"clientIp", "upstreamIp", "requestTime", "requestLine",
		"statusCode", "upstreamStatusCode", "requestBytes", "replyBytes")
}

// 上位から受け取った値の欄は、client との間の値の語彙を名乗らない。
func TestUpstreamItemsCarryNoClientSemantic(t *testing.T) {
	for _, name := range []squid.ItemName{
		squid.ItemUpstreamIP, squid.ItemUpstreamStatusCode, squid.ItemMimeType,
	} {
		if got := squid.SemanticOfItem(name); got != "" {
			t.Errorf("the %s item names the semantic %q, want none", name, got)
		}
	}
	if got := squid.SemanticOfItem(squid.ItemStatusCode); got != core.SemanticKeyHttpStatusCode {
		t.Errorf("the status code names the semantic %q, want http.status_code", got)
	}
}

// 由来が別の利用者名の指定子を別の欄にし、account.name の語彙を %un の欄だけに付ける。
func TestParseLogFormatSeparatesTheUserSpecifiers(t *testing.T) {
	layout, err := squid.ParseLogFormat(`%>a %un %ul %ue [%tl]`)
	if err != nil {
		t.Fatalf("parsing the user specifiers: %v", err)
	}
	requireItemNames(t, "the user specifiers", squid.ItemOrderOf(layout),
		"clientIp", "user", "userLogin", "userExternal", "requestTime")
	for _, name := range []squid.ItemName{"userLogin", "userExternal"} {
		if got := squid.SemanticOfItem(name); got != "" {
			t.Errorf("the %s item names the semantic %q, want none", name, got)
		}
	}
}

// 引用符で囲まないヘッダーの欄を読む。Squid はヘッダーの値に URL の escape を掛けて書く
// ため、値は空白を含まない。
func TestReaderReadsUnquotedHeaderItems(t *testing.T) {
	layout, err := squid.ParseLogFormat(`%>a [%tl] %{Referer}>h %>h{X-Forwarded-For} %<h{Content-Type}`)
	if err != nil {
		t.Fatalf("parsing unquoted header items: %v", err)
	}
	const line = `192.0.2.1 [02/Jan/2024:03:04:05 +0000] http://example.test/a%20b 198.51.100.7 text/html;%20charset=UTF-8`
	record, failure := readWithLayout(t, line, layout)
	if failure != nil {
		t.Fatalf("reading unquoted header items: %+v", failure)
	}
	for name, want := range map[squid.ItemName]string{
		squid.ItemReferer:               "http://example.test/a%20b",
		"requestHeader.X-Forwarded-For": "198.51.100.7",
		"responseHeader.Content-Type":   "text/html;%20charset=UTF-8",
		squid.ItemRequestTime:           "[02/Jan/2024:03:04:05 +0000]",
		squid.ItemClientIP:              "192.0.2.1",
	} {
		if got := itemValue(t, record, name); got != want {
			t.Errorf("the %s item = %q, want %q", name, got, want)
		}
	}
}

// 引用符で囲んだヘッダーの値は、空白を含んでも 1 つの欄として読める。
func TestReaderReadsAHeaderValueCarryingSpaces(t *testing.T) {
	const spec = `%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Content-Type}<h" %mt`
	layout, err := squid.ParseLogFormat(spec)
	if err != nil {
		t.Fatalf("parsing a spec with a response header: %v", err)
	}
	const line = `192.0.2.1 [02/Jan/2024:03:04:05 +0000] "GET / HTTP/1.1" 200 100 ` +
		`"text/html; charset=UTF-8" real/mimetype`
	record, failure := readWithLayout(t, line, layout)
	if failure != nil {
		t.Fatalf("reading a record whose header value carries a space: %+v", failure)
	}
	if got := itemValue(t, record, "responseHeader.Content-Type"); got != "text/html; charset=UTF-8" {
		t.Errorf("the content type = %q, want text/html; charset=UTF-8", got)
	}
	if got := itemValue(t, record, squid.ItemMimeType); got != "real/mimetype" {
		t.Errorf("the mime type = %q, want real/mimetype", got)
	}
}

// 空白以外の literal で区切った指定子を別の欄として読む。既存の出力を保つため、
// `%Ss:%Sh` だけは squidStatus の 1 欄のままにする。
func TestParseLogFormatReadsNonSpaceSeparators(t *testing.T) {
	layout, err := squid.ParseLogFormat(`ip=%>a|[%tl]|%>Hs|%<st|%Ss:%Sh;`)
	if err != nil {
		t.Fatalf("parsing items separated by a vertical bar: %v", err)
	}
	requireItemNames(t, "the vertical bar layout", squid.ItemOrderOf(layout),
		"clientIp", "requestTime", "statusCode", "replyBytes", "squidStatus")
	record, failure := readWithLayout(t,
		`ip=192.0.2.1|[02/Jan/2024:03:04:05 +0000]|200|42|TCP_MISS:HIER_DIRECT;`, layout)
	if failure != nil {
		t.Fatalf("reading items separated by a vertical bar: %+v", failure)
	}
	for name, want := range map[squid.ItemName]string{
		squid.ItemClientIP: "192.0.2.1", squid.ItemStatusCode: "200",
		squid.ItemReplyBytes: "42", squid.ItemSquidStatus: "TCP_MISS:HIER_DIRECT",
	} {
		if got := itemValue(t, record, name); got != want {
			t.Errorf("the %s item = %q, want %q", name, got, want)
		}
	}
	// 先頭と末尾の literal が行と合わないときは文字列の分割の失敗にする。
	for _, line := range []string{
		`192.0.2.1|[02/Jan/2024:03:04:05 +0000]|200|42|TCP_MISS:HIER_DIRECT;`,
		`ip=192.0.2.1|[02/Jan/2024:03:04:05 +0000]|200|42|TCP_MISS:HIER_DIRECT`,
	} {
		if _, failure := readWithLayout(t, line, layout); failure == nil ||
			failure.Stage != core.FailureStageTokenize {
			t.Errorf("the reader accepted %q without the literal of the layout", line)
		}
	}
}

// 同じ code を 2 回書いた欄は、2 つ目の欄の名前に番号を付け、語彙を付けない。
func TestParseLogFormatNumbersTheRepeatedItems(t *testing.T) {
	layout, err := squid.ParseLogFormat(`%>a %>a [%tl] %Hs %>Hs`)
	if err != nil {
		t.Fatalf("parsing repeated items: %v", err)
	}
	requireItemNames(t, "the repeated items", squid.ItemOrderOf(layout),
		"clientIp", "clientIp.2", "requestTime", "statusCode", "statusCode.2")
	if got := squid.SemanticOfItem("clientIp.2"); got != "" {
		t.Errorf("the repeated client address names the semantic %q, want none", got)
	}
}

// 欄を 1 つ増やした並びで読むと、行番号と byte offset を持つ文字列の分割の失敗になる。
func TestReaderReportsTheTokenizeFailureOfAWiderLayout(t *testing.T) {
	layout, err := squid.ParseLogFormat(squid.CombinedRequestBytesSpec + " %mt")
	if err != nil {
		t.Fatalf("parsing a spec with one more item: %v", err)
	}
	record, failure := readWithLayout(t, requestBytesLine, layout)
	if failure == nil {
		t.Fatal("the reader accepted a record carrying fewer items than the layout")
	}
	if failure.Stage != core.FailureStageTokenize {
		t.Errorf("the failure carries the stage %q, want tokenize", failure.Stage)
	}
	if failure.LineNumber == nil || *failure.LineNumber != 1 {
		t.Errorf("the failure carries the line number %v, want 1", failure.LineNumber)
	}
	if failure.ByteOffset == nil {
		t.Error("the failure carries no byte offset")
	}
	if record.RawText() != requestBytesLine {
		t.Error("the failed record lost its raw text")
	}
}
