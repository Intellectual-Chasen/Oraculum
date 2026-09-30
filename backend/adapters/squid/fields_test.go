package squid_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// fieldExpectation は exampleLine (tokenize_test.go) の欄 1 つの期待値である。
type fieldExpectation struct {
	name       string
	kind       core.RecordFieldKind
	rawText    string
	normalized string
	hasNormal  bool
	valueState core.ValueState
}

var exampleFields = []fieldExpectation{
	{name: "clientIp", kind: core.RecordFieldKindText, rawText: "192.0.2.8", valueState: core.ValueStatePresent},
	{name: "ident", kind: core.RecordFieldKindText, rawText: "-", valueState: core.ValueStateAbsent},
	{name: "user", kind: core.RecordFieldKindText, rawText: "-", valueState: core.ValueStateAbsent},
	{name: "requestTime", kind: core.RecordFieldKindTimestamp, rawText: "[14/Feb/2024:08:09:10 +0530]", valueState: core.ValueStatePresent},
	{
		name: "requestLine", kind: core.RecordFieldKindText,
		rawText:    `"GET http://198.51.100.9:8080/a?q=1 HTTP/1.1"`,
		normalized: "GET http://198.51.100.9:8080/a?q=1 HTTP/1.1", hasNormal: true,
		valueState: core.ValueStatePresent,
	},
	{name: "statusCode", kind: core.RecordFieldKindText, rawText: "200", valueState: core.ValueStatePresent},
	{name: "replyBytes", kind: core.RecordFieldKindText, rawText: "42", valueState: core.ValueStatePresent},
	{name: "referer", kind: core.RecordFieldKindText, rawText: `"-"`, valueState: core.ValueStateAbsent},
	{
		name: "userAgent", kind: core.RecordFieldKindText, rawText: `"agent"`,
		normalized: "agent", hasNormal: true, valueState: core.ValueStatePresent,
	},
	{name: "squidStatus", kind: core.RecordFieldKindText, rawText: "TCP_MISS:HIER_DIRECT", valueState: core.ValueStatePresent},
}

func TestRecordFieldsCarriesEveryItem(t *testing.T) {
	record := readOK(t, exampleLine)
	requestTime, failure := squid.ParseRequestTime(record)
	if failure != nil {
		t.Fatal(failure)
	}
	fields := squid.RecordFields(record, requestTime)
	if len(fields) != len(exampleFields) {
		t.Fatalf("field count = %d, want %d", len(fields), len(exampleFields))
	}
	for index, want := range exampleFields {
		got := fields[index]
		if err := got.Validate(); err != nil {
			t.Fatalf("field %q did not validate: %v", want.name, err)
		}
		if got.Name != want.name || got.Kind != want.kind {
			t.Fatalf("field %d = %q %q, want %q %q", index, got.Name, got.Kind, want.name, want.kind)
		}
		if want.kind == core.RecordFieldKindTimestamp {
			if got.Timestamp == nil || got.Timestamp.RawText == nil || *got.Timestamp.RawText != want.rawText {
				t.Fatalf("field %q timestamp = %+v", want.name, got.Timestamp)
			}
			continue
		}
		checkTextField(t, got, want)
	}
}

func checkTextField(t *testing.T, got core.RecordField, want fieldExpectation) {
	t.Helper()
	if got.Text == nil {
		t.Fatalf("field %q lost its value", want.name)
	}
	if got.Text.ValueState != want.valueState {
		t.Fatalf("field %q valueState = %q, want %q", want.name, got.Text.ValueState, want.valueState)
	}
	raw, hasRaw := got.Text.RawTextValue()
	if !hasRaw || raw != want.rawText {
		t.Fatalf("field %q rawText = %q %v, want %q", want.name, raw, hasRaw, want.rawText)
	}
	normalized, hasNormalized := got.Text.NormalizedValue()
	if hasNormalized != want.hasNormal || normalized != want.normalized {
		t.Fatalf("field %q normalized = %q %v, want %q %v",
			want.name, normalized, hasNormalized, want.normalized, want.hasNormal)
	}
	derivation, hasDerivation := got.Text.DerivationValue()
	if hasDerivation != want.hasNormal {
		t.Fatalf("field %q derivation = %q %v", want.name, derivation, hasDerivation)
	}
	if hasDerivation && derivation == "" {
		t.Fatalf("field %q derivation is empty", want.name)
	}
}

func TestRecordFieldsKeepsQuotedAbsentText(t *testing.T) {
	record := readOK(t, strings.Replace(exampleLine, `"agent"`, `"-"`, 1))
	requestTime, failure := squid.ParseRequestTime(record)
	if failure != nil {
		t.Fatal(failure)
	}
	fields := squid.RecordFields(record, requestTime)
	agent := fields[len(fields)-2]
	if agent.Name != "userAgent" || agent.Text == nil {
		t.Fatalf("user agent field = %+v", agent)
	}
	if agent.Text.ValueState != core.ValueStateAbsent {
		t.Fatalf("quoted %q must be absent, got %q", "-", agent.Text.ValueState)
	}
	raw, hasRaw := agent.Text.RawTextValue()
	if !hasRaw || raw != `"-"` {
		t.Fatalf("absent user agent rawText = %q %v", raw, hasRaw)
	}
	if _, hasNormalized := agent.Text.NormalizedValue(); hasNormalized {
		t.Fatalf("absent user agent carries a normalized value")
	}
}

// TestReplyBytesSeparatesAbsentFromZero は %<st の - と 0 を別の状態で持つことを確かめる。
// - を absent にする判断は fields.go の absentItemText の既知の制限が持つ。
func TestReplyBytesSeparatesAbsentFromZero(t *testing.T) {
	for _, tt := range []struct {
		replyBytes string
		valueState core.ValueState
	}{
		{replyBytes: "-", valueState: core.ValueStateAbsent},
		{replyBytes: "0", valueState: core.ValueStatePresent},
	} {
		record := readOK(t, strings.Replace(exampleLine, " 200 42 ", " 200 "+tt.replyBytes+" ", 1))
		requestTime, failure := squid.ParseRequestTime(record)
		if failure != nil {
			t.Fatal(failure)
		}
		fields := squid.RecordFields(record, requestTime)
		got := fields[6]
		if got.Name != "replyBytes" || got.Text == nil {
			t.Fatalf("reply bytes field = %+v", got)
		}
		if got.Text.ValueState != tt.valueState {
			t.Errorf("replyBytes %q valueState = %q, want %q",
				tt.replyBytes, got.Text.ValueState, tt.valueState)
		}
		raw, hasRaw := got.Text.RawTextValue()
		if !hasRaw || raw != tt.replyBytes {
			t.Errorf("replyBytes %q rawText = %q %v", tt.replyBytes, raw, hasRaw)
		}
		if _, hasNormalized := got.Text.NormalizedValue(); hasNormalized {
			t.Errorf("replyBytes %q carried a normalized value", tt.replyBytes)
		}
	}
}

func TestRequestLineCarriesSchemeAndPort(t *testing.T) {
	for _, tt := range []struct {
		method, target   string
		scheme, host     string
		port             string
		hasScheme        bool
		hasHost, hasPort bool
	}{
		{method: "GET", target: "http://198.51.100.9:8080/a?q=1", scheme: "http", hasScheme: true, host: "198.51.100.9", hasHost: true, port: "8080", hasPort: true},
		{method: "GET", target: "http://example.invalid/path", scheme: "http", hasScheme: true, host: "example.invalid", hasHost: true},
		{method: "GET", target: "https://example.invalid/path", scheme: "https", hasScheme: true, host: "example.invalid", hasHost: true},
		{method: "GET", target: "custom+v1://example.invalid/a", scheme: "custom+v1", hasScheme: true, host: "example.invalid", hasHost: true},
		{method: "GET", target: "//example.invalid/a", host: "example.invalid", hasHost: true},
		{method: "CONNECT", target: "198.51.100.9:443", host: "198.51.100.9", hasHost: true, port: "443", hasPort: true},
		{method: "CONNECT", target: "[2001:db8::9]:443", host: "2001:db8::9", hasHost: true, port: "443", hasPort: true},
		{method: "GET", target: "http://[2001:db8::9]/", scheme: "http", hasScheme: true, host: "2001:db8::9", hasHost: true},
		{method: "GET", target: "http://reader@example.invalid:8080/a", scheme: "http", hasScheme: true, host: "example.invalid", hasHost: true, port: "8080", hasPort: true},
		{method: "GET", target: "http://reader@[2001:db8::9]:8080/a", scheme: "http", hasScheme: true, host: "2001:db8::9", hasHost: true, port: "8080", hasPort: true},
		{method: "GET", target: "http://example.invalid:/a", scheme: "http", hasScheme: true, host: "example.invalid", hasHost: true},
		{method: "GET", target: "/path?x=1"},
		{method: "GET", target: "error:invalid-request"},
	} {
		got, failure := squid.ParseRequestLine(requestRecord(t, tt.method+" "+tt.target+" HTTP/1.1"))
		if failure != nil {
			t.Fatalf("target %q failed: %+v", tt.target, failure)
		}
		checkOptionalText(t, tt.target+" scheme", got.Scheme, tt.scheme, tt.hasScheme)
		checkOptionalText(t, tt.target+" host", got.AuthorityHost, tt.host, tt.hasHost)
		checkOptionalText(t, tt.target+" port", got.AuthorityPort, tt.port, tt.hasPort)
	}
}

func checkOptionalText(t *testing.T, item string, got *string, want string, present bool) {
	t.Helper()
	if !present {
		if got != nil {
			t.Fatalf("%s = %q, want no value", item, *got)
		}
		return
	}
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %q", item, got, want)
	}
}
