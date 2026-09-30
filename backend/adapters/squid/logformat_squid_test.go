package squid_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// secondsLocalSpec は日時の書式を指定した %tl と、処理時間、スラッシュ区切りの欄を持つ logformat である。
const secondsLocalSpec = `%{%Y/%m/%d %H:%M:%S}tl.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`

// 本 test が決めた行。3 行目は 1 行の長さの上限で %ru の途中が切れた形である。
const secondsLocalLines = `2002/02/01 03:25:07.412     37 192.0.2.5 TCP_TUNNEL/200 3072 CONNECT host.example.test:443 - HIER_DIRECT/198.51.100.25 -
2002/02/01 03:25:19.736 1054127 192.0.2.4 TCP_MISS/503 4028 GET http://ipv6.example.test/a.txt - HIER_DIRECT/2001:db8::172d:e911 text/html
2002/02/01 06:40:11.254      1 192.0.2.5 NONE/400 18432 GET http://198.51.100.4/aaaa
2002/02/01 06:40:12.000      0 192.0.2.5 NONE/000 0 NONE error:transaction-end-before-headers - HIER_NONE/- -
`

type readResult struct {
	record  squid.Record
	failure *core.ImportFailure
}

func readAll(t *testing.T, input string, layout squid.Layout) []readResult {
	t.Helper()
	var reader squid.Reader
	reader.Reset(strings.NewReader(input), layout)
	var results []readResult
	for {
		record, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return results
		}
		if err != nil {
			t.Fatalf("reading the records: %v", err)
		}
		results = append(results, readResult{record, failure})
	}
}

// 日時の書式を指定した %tl と、%Ss/%03>Hs と %Sh/%<a を別の欄として読む。
func TestReaderReadsTheSpecifiedLocalTimeLayout(t *testing.T) {
	layout, err := squid.LayoutForSpec(secondsLocalSpec)
	if err != nil {
		t.Fatalf("building the layout: %v", err)
	}
	requireItemNames(t, "the specified local time layout", squid.ItemOrderOf(layout),
		"requestTime", "responseTime", "clientIp", "squidRequestStatus", "statusCode",
		"replyBytes", "requestMethod", "requestUrl", "user", "hierarchyStatus", "upstreamIp", "mimeType")
	results := readAll(t, secondsLocalLines, layout)
	if len(results) != strings.Count(secondsLocalLines, "\n") {
		t.Fatalf("the reader returned %d records for %d lines", len(results), strings.Count(secondsLocalLines, "\n"))
	}
	want := map[int64]map[squid.ItemName]string{
		1: {"requestTime": "2002/02/01 03:25:07.412", "responseTime": "37", "squidRequestStatus": "TCP_TUNNEL",
			"statusCode": "200", "requestUrl": "host.example.test:443", "hierarchyStatus": "HIER_DIRECT",
			"upstreamIp": "198.51.100.25", "mimeType": "-"},
		2: {"responseTime": "1054127", "upstreamIp": "2001:db8::172d:e911", "mimeType": "text/html"},
		4: {"statusCode": "000", "requestMethod": "NONE", "hierarchyStatus": "HIER_NONE", "upstreamIp": "-"},
	}
	for _, result := range results {
		line := result.record.LineNumber()
		if line == 3 {
			requireTruncatedRecordFailure(t, result)
			continue
		}
		if result.failure != nil {
			t.Fatalf("line %d failed: %+v", line, result.failure)
		}
		for name, value := range want[line] {
			if got := itemValue(t, result.record, name); got != value {
				t.Errorf("line %d: the %s item = %q, want %q", line, name, got, value)
			}
		}
	}
}

// 途中で切れた行は、行番号と byte offset を持つ文字列の分割の失敗になり、原文を保つ。
func requireTruncatedRecordFailure(t *testing.T, result readResult) {
	t.Helper()
	if result.failure == nil || result.failure.Stage != core.FailureStageTokenize {
		t.Fatalf("the truncated line carries the failure %+v, want a tokenize failure", result.failure)
	}
	if result.failure.LineNumber == nil || *result.failure.LineNumber != 3 || result.failure.ByteOffset == nil {
		t.Errorf("the truncated line lost its position: %+v", result.failure)
	}
	if !strings.HasSuffix(result.record.RawText(), "http://198.51.100.4/aaaa") {
		t.Errorf("the truncated line lost its raw text: %q", result.record.RawText())
	}
}

// 日時の書式を指定した %tl は、UTC からのずれを仮定しないミリ秒の時刻になる。
func TestParseRequestTimeKeepsTheLocalTimeWithoutOffset(t *testing.T) {
	layout, err := squid.LayoutForSpec(secondsLocalSpec)
	if err != nil {
		t.Fatalf("building the layout: %v", err)
	}
	results := readAll(t, secondsLocalLines, layout)
	timestamp, failure := squid.ParseRequestTime(results[0].record)
	if failure != nil {
		t.Fatalf("parsing the request time: %+v", failure)
	}
	requireTimestamp(t, timestamp, "2002/02/01 03:25:07.412", "2002-02-01T03:25:07.412",
		core.PrecisionMillisecond, core.OffsetStateItemAbsent)
	if timestamp.OffsetText != nil {
		t.Errorf("the local time carries the offset text %q, want none", *timestamp.OffsetText)
	}
	if _, ok := timestamp.Instant(); ok {
		t.Error("the local time without an offset carries an instant")
	}
	if got := squid.TimePrecisionOf(layout); got != core.PrecisionMillisecond {
		t.Errorf("the layout precision = %q, want millisecond", got)
	}
}

// 時刻の code ごとに、UTC からのずれの状態と精度を値と code の定義から決める。
func TestParseRequestTimeReadsEachTimeCode(t *testing.T) {
	for _, testCase := range []struct {
		name, spec, line, raw, normalized string
		precision                         core.Precision
		offset                            core.OffsetState
	}{
		{"epoch with milliseconds", `%ts.%03tu %>a`, `1012533907.412 192.0.2.1`,
			"1012533907.412", "2002-02-01T03:25:07.412Z", core.PrecisionMillisecond, core.OffsetStateEpoch},
		// %tu の文字列は単位の数である。0 埋めしない 5 は 5 ミリ秒である。
		{"epoch with unpadded milliseconds", `%ts.%tu %>a`, `1012533907.5 192.0.2.1`,
			"1012533907.5", "2002-02-01T03:25:07.005Z", core.PrecisionMillisecond, core.OffsetStateEpoch},
		{"epoch with microseconds", `%ts.%.6tu %>a`, `1012533907.000123 192.0.2.1`,
			"1012533907.000123", "2002-02-01T03:25:07.000123Z", core.PrecisionMicrosecond, core.OffsetStateEpoch},
		{"epoch seconds", `%ts %>a`, `1012533907 192.0.2.1`,
			"1012533907", "2002-02-01T03:25:07Z", core.PrecisionSecond, core.OffsetStateEpoch},
		{"the default GMT time", `%tg %>a`, `01/Feb/2002:03:25:07 192.0.2.1`,
			"01/Feb/2002:03:25:07", "2002-02-01T03:25:07Z", core.PrecisionSecond, core.OffsetStateFormatDefined},
		{"the unbracketed default local time", `%tl %>a`, `01/Feb/2002:12:25:07 +0900 192.0.2.1`,
			"01/Feb/2002:12:25:07 +0900", "2002-02-01T12:25:07+09:00", core.PrecisionSecond, core.OffsetStateInValue},
		{"a local time with names", `%{%a %b %e %H:%M:%S %Y}tl|%>a`, `Tue Feb  5 12:25:07 2002|192.0.2.1`,
			"Tue Feb  5 12:25:07 2002", "2002-02-05T12:25:07", core.PrecisionSecond, core.OffsetStateItemAbsent},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			layout, err := squid.ParseLogFormat(testCase.spec)
			if err != nil {
				t.Fatalf("parsing %q: %v", testCase.spec, err)
			}
			record, failure := readWithLayout(t, testCase.line, layout)
			if failure != nil {
				t.Fatalf("reading %q: %+v", testCase.line, failure)
			}
			timestamp, failure := squid.ParseRequestTime(record)
			if failure != nil {
				t.Fatalf("parsing the request time: %+v", failure)
			}
			requireTimestamp(t, timestamp, testCase.raw, testCase.normalized, testCase.precision, testCase.offset)
			if got := itemValue(t, record, squid.ItemClientIP); got != "192.0.2.1" {
				t.Errorf("the client address = %q, want 192.0.2.1", got)
			}
			if got := squid.TimePrecisionOf(layout); got != testCase.precision {
				t.Errorf("the layout precision = %q, want %q", got, testCase.precision)
			}
		})
	}
}

// 暦に無い日付は正規化の失敗になる。
func TestParseRequestTimeRejectsADateOutsideTheCalendar(t *testing.T) {
	layout, err := squid.LayoutForSpec(secondsLocalSpec)
	if err != nil {
		t.Fatalf("building the layout: %v", err)
	}
	line := strings.Replace(strings.SplitN(secondsLocalLines, "\n", 2)[0], "2002/02/01", "2002/13/01", 1)
	record, failure := readWithLayout(t, line, layout)
	if failure != nil {
		t.Fatalf("tokenizing the line: %+v", failure)
	}
	if _, failure := squid.ParseRequestTime(record); failure == nil || failure.Stage != core.FailureStageNormalize {
		t.Fatalf("the thirteenth month carries the failure %+v, want a normalize failure", failure)
	}
}

func requireTimestamp(
	t *testing.T, timestamp core.Timestamp, raw, normalized string,
	precision core.Precision, offset core.OffsetState,
) {
	t.Helper()
	if got, _ := timestamp.RawTextValue(); got != raw {
		t.Errorf("the raw text = %q, want %q", got, raw)
	}
	if got, _ := timestamp.NormalizedValue(); got != normalized {
		t.Errorf("the normalized value = %q, want %q", got, normalized)
	}
	if timestamp.Precision != precision || timestamp.OffsetState != offset {
		t.Errorf("the precision and offset = %q %q, want %q %q",
			timestamp.Precision, timestamp.OffsetState, precision, offset)
	}
}

// %ru と %rm から要求先の authority を切り出す。要求行の HTTP のバージョンは持たない。
func TestParseRequestLineReadsTheSeparateTarget(t *testing.T) {
	layout, err := squid.LayoutForSpec(secondsLocalSpec)
	if err != nil {
		t.Fatalf("building the layout: %v", err)
	}
	results := readAll(t, secondsLocalLines, layout)
	for _, testCase := range []struct {
		index            int
		method, host     string
		port, scheme     string
		withoutAuthority bool
	}{
		{index: 0, method: "CONNECT", host: "host.example.test", port: "443"},
		{index: 1, method: "GET", host: "ipv6.example.test", scheme: "http"},
		{index: 3, method: "NONE", withoutAuthority: true},
	} {
		record := results[testCase.index].record
		if !squid.CarriesRequestTarget(record) {
			t.Fatalf("line %d carries no request target", record.LineNumber())
		}
		requestLine, failure := squid.ParseRequestLine(record)
		if failure != nil {
			t.Fatalf("line %d: %+v", record.LineNumber(), failure)
		}
		if requestLine.Method != testCase.method || requestLine.Protocol != "" {
			t.Errorf("line %d: method %q protocol %q, want %q and none",
				record.LineNumber(), requestLine.Method, requestLine.Protocol, testCase.method)
		}
		if testCase.withoutAuthority {
			if requestLine.AuthorityHost != nil {
				t.Errorf("line %d carries the host %q, want none", record.LineNumber(), *requestLine.AuthorityHost)
			}
			continue
		}
		if requestLine.AuthorityHost == nil || *requestLine.AuthorityHost != testCase.host {
			t.Errorf("line %d: the host = %v, want %q", record.LineNumber(), requestLine.AuthorityHost, testCase.host)
		}
		if got := optional(requestLine.AuthorityPort); got != testCase.port {
			t.Errorf("line %d: the port = %q, want %q", record.LineNumber(), got, testCase.port)
		}
		if got := optional(requestLine.Scheme); got != testCase.scheme {
			t.Errorf("line %d: the scheme = %q, want %q", record.LineNumber(), got, testCase.scheme)
		}
	}
}

// 引用符で囲まない要求先の失敗は、レコード内の要求先の文字列の位置を指す。
func TestParseRequestLineLocatesTheSeparateTargetFailure(t *testing.T) {
	layout, err := squid.ParseLogFormat(`%>a %rm %ru`)
	if err != nil {
		t.Fatalf("parsing the layout: %v", err)
	}
	record, failure := readWithLayout(t, `192.0.2.1 CONNECT host.example.test/x:443`, layout)
	if failure != nil {
		t.Fatalf("tokenizing the line: %+v", failure)
	}
	_, failure = squid.ParseRequestLine(record)
	if failure == nil || failure.ByteOffset == nil {
		t.Fatalf("the delimiter in the CONNECT authority carries the failure %+v", failure)
	}
	// 失敗の位置は `/` の byte である。
	if want := int64(strings.Index(record.RawText(), "/x")); *failure.ByteOffset != want {
		t.Errorf("the failure points at byte %d, want %d", *failure.ByteOffset, want)
	}
}

func optional(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// 組み込みの logformat を名前で指定して読む。
func TestLayoutForSpecReadsTheBuiltInLogFormats(t *testing.T) {
	for _, testCase := range []struct {
		name, line string
		want       map[squid.ItemName]string
	}{
		{"squid", `1012533907.412     37 192.0.2.5 TCP_TUNNEL/200 3072 CONNECT host.example.test:443 - HIER_DIRECT/198.51.100.25 -`,
			map[squid.ItemName]string{"requestTime": "1012533907.412", "responseTime": "37", "upstreamIp": "198.51.100.25"}},
		{"common", `192.0.2.1 - - [02/Jan/2024:03:04:05 +0000] "GET http://example.test/ HTTP/1.1" 200 42 TCP_MISS:HIER_DIRECT`,
			map[squid.ItemName]string{"user": "-", "requestLine": "GET http://example.test/ HTTP/1.1", "squidStatus": "TCP_MISS:HIER_DIRECT"}},
		{"combined", `192.0.2.1 - - [02/Jan/2024:03:04:05 +0000] "GET http://example.test/ HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`,
			map[squid.ItemName]string{"userAgent": "agent", "replyBytes": "42"}},
		{"referrer", `1012533907.412 192.0.2.1 http://example.test/from http://example.test/to`,
			map[squid.ItemName]string{"referer": "http://example.test/from", "requestUrl": "http://example.test/to"}},
		{"useragent", `192.0.2.1 [02/Jan/2024:03:04:05 +0000] "Mozilla/5.0 (X11)"`,
			map[squid.ItemName]string{"userAgent": "Mozilla/5.0 (X11)"}},
		{"icap_squid", `1012533907.412     12 client.example.test ICAP_MOD/200 512 RESPMOD icap://icap.example.test:1344/respmod - -/198.51.100.9 -`,
			map[squid.ItemName]string{"icapResponseTime": "12", "clientFqdn": "client.example.test",
				"icapOutcome": "ICAP_MOD", "icapStatusCode": "200", "icapServerIp": "198.51.100.9"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			layout, err := squid.LayoutForSpec(testCase.name)
			if err != nil {
				t.Fatalf("building the built-in %s: %v", testCase.name, err)
			}
			if !strings.Contains(layout.Spec(), "%") {
				t.Errorf("the layout keeps the name %q, want the expanded logformat", layout.Spec())
			}
			record, failure := readWithLayout(t, testCase.line, layout)
			if failure != nil {
				t.Fatalf("reading a %s line: %+v", testCase.name, failure)
			}
			for name, want := range testCase.want {
				if got := itemValue(t, record, name); got != want {
					t.Errorf("the %s item = %q, want %q", name, got, want)
				}
			}
			if _, failure := squid.ParseRequestTime(record); failure != nil {
				t.Errorf("parsing the request time: %+v", failure)
			}
		})
	}
}

// squid.conf の logformat の 1 行を、名前を外した logformat として読む。
func TestLayoutForSpecReadsTheLogFormatDirective(t *testing.T) {
	layout, err := squid.LayoutForSpec("logformat custom  %>a %Ss/%03>Hs\n")
	if err != nil {
		t.Fatalf("building the directive: %v", err)
	}
	if layout.Spec() != "%>a %Ss/%03>Hs" {
		t.Errorf("the layout spec = %q, want the logformat without the directive", layout.Spec())
	}
	requireItemNames(t, "the directive", squid.ItemOrderOf(layout), "clientIp", "squidRequestStatus", "statusCode")
}

// encoding の指定、幅、`{arg}` の位置、名前空間の書き方をそれぞれ読む。
func TestReaderReadsTheSpecifierModifiers(t *testing.T) {
	for _, testCase := range []struct {
		name, spec, line string
		want             map[squid.ItemName]string
	}{
		{"a quoted region with two items", `%>a "%rm %ru" %Ss`,
			`192.0.2.1 "GET http://example.test/\"x" TCP_MISS`,
			map[squid.ItemName]string{"requestMethod": "GET", "requestUrl": `http://example.test/\"x`, "squidRequestStatus": "TCP_MISS"}},
		{"a bracketed region", `%>a [%un] %Ss`, `192.0.2.1 [name%5Bx%5D] TCP_MISS`,
			map[squid.ItemName]string{"user": "name%5Bx%5D"}},
		{"a shell quoted value with a space", `%>a %/un %Ss`, `192.0.2.1 "first last" TCP_MISS`,
			map[squid.ItemName]string{"user": "first last", "squidRequestStatus": "TCP_MISS"}},
		{"a shell quoted value without a space", `%>a %/un %Ss`, `192.0.2.1 single TCP_MISS`,
			map[squid.ItemName]string{"user": "single"}},
		{"a left aligned width", `%-6tr|%>a`, `37    |192.0.2.1`,
			map[squid.ItemName]string{"responseTime": "37", "clientIp": "192.0.2.1"}},
		{"a left aligned width before a space", `%-6tr %>a`, `37     192.0.2.1`,
			map[squid.ItemName]string{"responseTime": "37", "clientIp": "192.0.2.1"}},
		{"a right aligned string width", `%>a %20Ss|`, `192.0.2.1             TCP_MISS|`,
			map[squid.ItemName]string{"squidRequestStatus": "TCP_MISS"}},
		{"an argument after the code and namespaces", `%http::>a %>h{Referer} %tls::>sni %ssl::<cert_errors{+}`,
			`192.0.2.1 http://example.test/ sni.example.test X509_V_ERR_1+X509_V_ERR_2`,
			map[squid.ItemName]string{"clientIp": "192.0.2.1", "referer": "http://example.test/",
				"sslClientSni": "sni.example.test", "sslServerCertErrors": "X509_V_ERR_1+X509_V_ERR_2"}},
		{"a literal percent and byte", `%>a %% %Ss%byte{9}%>Hs`, "192.0.2.1 % TCP_MISS\t200",
			map[squid.ItemName]string{"squidRequestStatus": "TCP_MISS", "statusCode": "200"}},
		{"a header element and a note", `%>a %{X-Forwarded-For:,1}>h %{tag}note`, `192.0.2.1 198.51.100.1 alpha`,
			map[squid.ItemName]string{"requestHeader.X-Forwarded-For:,1": "198.51.100.1", "note.tag": "alpha"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			layout, err := squid.ParseLogFormat(testCase.spec)
			if err != nil {
				t.Fatalf("parsing %q: %v", testCase.spec, err)
			}
			record, failure := readWithLayout(t, testCase.line, layout)
			if failure != nil {
				t.Fatalf("reading %q: %+v", testCase.line, failure)
			}
			for name, want := range testCase.want {
				if got := itemValue(t, record, name); got != want {
					t.Errorf("the %s item = %q, want %q", name, got, want)
				}
			}
		})
	}
}
