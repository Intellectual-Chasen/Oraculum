package squid_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func requestRecord(t *testing.T, request string) squid.Record {
	t.Helper()
	return readOK(t, strings.Replace(exampleLine, "GET http://198.51.100.9:8080/a?q=1 HTTP/1.1", request, 1))
}

func TestRequestLineAuthority(t *testing.T) {
	for _, tt := range []struct{ method, target, authority string }{
		{"GET", "http://198.51.100.9:8080/a?q=1", "198.51.100.9:8080"},
		{"GET", "https://example.invalid/path", "example.invalid"},
		{"GET", "http://example.invalid:81?x=1", "example.invalid:81"},
		{"GET", "http://example.invalid:82#part", "example.invalid:82"},
		{"CONNECT", "198.51.100.9:443", "198.51.100.9:443"},
		{"CONNECT", "example.invalid:8443", "example.invalid:8443"},
		{"CONNECT", "[2001:db8::9]:443", "[2001:db8::9]:443"},
		{"GET", "http://[2001:db8::9]:8080/a", "[2001:db8::9]:8080"},
		{"GET", "https://[2001:db8::9]/", "[2001:db8::9]"},
		{"GET", "http://reader@example.invalid:8080/a", "reader@example.invalid:8080"},
		{"GET", "http://reader@[2001:db8::9]:8080/a", "reader@[2001:db8::9]:8080"},
		{"GET", "http://reader%40name@example.invalid/a", "reader%40name@example.invalid"},
		{"GET", "//example.invalid/a", "example.invalid"},
		{"GET", "custom+v1://example.invalid/a", "example.invalid"},
	} {
		got, failure := squid.ParseRequestLine(requestRecord(t, tt.method+" "+tt.target+" HTTP/1.1"))
		if failure != nil {
			t.Fatalf("target %q failed", tt.target)
		}
		if got.Method != tt.method || got.RawTarget != tt.target || got.Protocol != "HTTP/1.1" || got.Authority == nil || *got.Authority != tt.authority {
			t.Fatalf("request target %q not preserved: %+v", tt.target, got)
		}
	}
}

// TestRequestLineDropsHostBrackets は IPv6 の host から角括弧を外し、port を別に持つことを
// 確かめる。Authority は角括弧を持つ原資料の文字列のまま残る。
func TestRequestLineDropsHostBrackets(t *testing.T) {
	for _, tt := range []struct {
		method, target  string
		authority, host string
		port            string
		hasPort         bool
	}{
		{method: "CONNECT", target: "[2001:db8::9]:8443", authority: "[2001:db8::9]:8443", host: "2001:db8::9", port: "8443", hasPort: true},
		{method: "CONNECT", target: "[2001:db8::9]", authority: "[2001:db8::9]", host: "2001:db8::9"},
		{method: "GET", target: "http://[2001:db8::9]/x", authority: "[2001:db8::9]", host: "2001:db8::9"},
		{method: "GET", target: "http://[2001:db8::9]:8443/x", authority: "[2001:db8::9]:8443", host: "2001:db8::9", port: "8443", hasPort: true},
		// 角括弧を持たない host は文字列をそのまま持つ。
		{method: "GET", target: "http://198.51.100.9:8443/x", authority: "198.51.100.9:8443", host: "198.51.100.9", port: "8443", hasPort: true},
	} {
		got, failure := squid.ParseRequestLine(requestRecord(t, tt.method+" "+tt.target+" HTTP/1.1"))
		if failure != nil {
			t.Fatalf("target %q failed: %+v", tt.target, failure)
		}
		if got.Authority == nil || *got.Authority != tt.authority {
			t.Errorf("target %q authority = %v, want %q", tt.target, got.Authority, tt.authority)
		}
		checkOptionalText(t, tt.target+" host", got.AuthorityHost, tt.host, true)
		checkOptionalText(t, tt.target+" port", got.AuthorityPort, tt.port, tt.hasPort)
		if got.RawTarget != tt.target {
			t.Errorf("target %q rawTarget = %q", tt.target, got.RawTarget)
		}
	}
}

func TestRequestLineWithoutAuthority(t *testing.T) {
	for _, target := range []string{"error:invalid-request", "/path?x=1", "*", "urn:example:record"} {
		got, failure := squid.ParseRequestLine(requestRecord(t, "GET "+target+" HTTP/1.0"))
		if failure != nil || got.Authority != nil || got.RawTarget != target || got.Protocol != "HTTP/1.0" {
			t.Fatalf("authority-free target %q differs", target)
		}
	}
}

func TestRequestLineDerivesAuthorityFromDecodedValue(t *testing.T) {
	const raw = `GET http://reader\"name\\suffix@example.invalid:8080/a HTTP/1.1`
	got, failure := squid.ParseRequestLine(requestRecord(t, raw))
	if failure != nil {
		t.Fatal(failure)
	}
	if got.Method != "GET" || got.RawTarget != `http://reader\"name\\suffix@example.invalid:8080/a` || got.Protocol != "HTTP/1.1" {
		t.Fatal("request tokens lost their original spelling")
	}
	if got.Authority == nil || *got.Authority != `reader"name\suffix@example.invalid:8080` {
		t.Fatal("authority still carries Squid escapes")
	}
}

func TestRequestLineKeepsHyphens(t *testing.T) {
	got, failure := squid.ParseRequestLine(requestRecord(t, "- - HTTP/1.1"))
	if failure != nil || got.Method != "-" || got.RawTarget != "-" || got.Protocol != "HTTP/1.1" || got.Authority != nil {
		t.Fatal("hyphens must remain literal request tokens")
	}
}

func TestConnectComputedErrorWithoutAuthority(t *testing.T) {
	got, failure := squid.ParseRequestLine(requestRecord(t, "CONNECT error:invalid-request HTTP/1.1"))
	if failure != nil || got.Authority != nil || got.RawTarget != "error:invalid-request" {
		t.Fatal("computed CONNECT error must carry no authority")
	}
}

func TestRequestLineRejectsMalformed(t *testing.T) {
	for _, tt := range []struct {
		request, reason string
		offset          int64
		authority       bool
	}{
		{"", "missing method", 0, false},
		{"GET", "missing request target", 3, false},
		{"GET /", "missing protocol", 5, false},
		{"GET / HTTP/", "missing HTTP version", 11, false},
		{"GET / HTTPS/1.1", "invalid protocol", 6, false},
		{"GET  / HTTP/1.1", "missing request target", 4, false},
		{"GET / HTTP/1.1 extra", "extra request token", 14, false},
		{"GET http:///path HTTP/1.1", "missing authority host", 11, true},
		{"GET http://[2001:db8::1/path HTTP/1.1", "unclosed host bracket", 23, true},
		{"GET http://[2001:db8::1]x/ HTTP/1.1", "unexpected byte after host bracket", 24, true},
		{"GET http://host:abc/ HTTP/1.1", "non-decimal port", 16, true},
		{"GET http://user@/ HTTP/1.1", "missing authority host", 16, true},
		{"CONNECT host:443/path HTTP/1.1", "delimiter in CONNECT authority", 16, true},
	} {
		_, failure := squid.ParseRequestLine(requestRecord(t, tt.request))
		expected := "method, request target, and HTTP/version separated by spaces"
		if tt.authority {
			expected = "a request target with a delimited authority or without authority"
		}
		checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{
			expected, fmt.Sprintf("%s in request %q", tt.reason, tt.request), 44 + tt.offset,
		})
	}
	_, failure := squid.ParseRequestLine(squid.Record{})
	checkUnknownPosition(t, failure)
}

func TestRequestLineRejectsAuthorityControls(t *testing.T) {
	for _, control := range []string{`\r`, `\n`, `\t`, "\x01", "\x1f", "\x7f"} {
		raw := "GET http://ho" + control + "st/a HTTP/1.1"
		_, failure := squid.ParseRequestLine(requestRecord(t, raw))
		checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{
			"a request target with a delimited authority or without authority",
			fmt.Sprintf("control byte in authority in request %q", raw), 57,
		})
	}
	// 前方の escape と多 byte 文字を越えた位置も、復号前の原資料へ戻す。
	raw := `GET http://読\"者@ho\nst/a HTTP/1.1`
	_, failure := squid.ParseRequestLine(requestRecord(t, raw))
	checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{
		"a request target with a delimited authority or without authority",
		fmt.Sprintf("control byte in authority in request %q", raw), 66,
	})
}
