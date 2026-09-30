package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
)

// synthesized combined 行。RFC 5737 の文書用範囲と example.test 系の文字列を使う。
const fullShapeLine = `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET / HTTP/1.1" 302 - "-" "example-agent/1.0"`

func accessRecordOf(t *testing.T, line string) apache.AccessRecord {
	t.Helper()
	var reader apache.AccessReader
	reader.Reset(newSingleLineReader(line))
	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() returned error %v", err)
	}
	if failure != nil {
		t.Fatalf("Next() returned failure %+v", failure)
	}
	return record
}

func TestAccessTokenizeFullShape(t *testing.T) {
	record := accessRecordOf(t, fullShapeLine)
	items := record.Items()
	if len(items) != 9 {
		t.Fatalf("len(items) = %d, want 9", len(items))
	}
	want := map[apache.AccessItemName]string{
		apache.AccessItemClientIP:    "203.0.113.10",
		apache.AccessItemIdent:       "-",
		apache.AccessItemUser:        "-",
		apache.AccessItemRequestTime: "[01/Feb/2000:11:20:10 +0900]",
		apache.AccessItemRequestLine: `"GET / HTTP/1.1"`,
		apache.AccessItemStatusCode:  "302",
		apache.AccessItemReplyBytes:  "-",
		apache.AccessItemReferer:     `"-"`,
		apache.AccessItemUserAgent:   `"example-agent/1.0"`,
	}
	for name, rawText := range want {
		item, ok := record.Item(name)
		if !ok {
			t.Errorf("item %s is absent", name)
			continue
		}
		if item.RawValue() != rawText {
			t.Errorf("item %s RawValue() = %q, want %q", name, item.RawValue(), rawText)
		}
	}
}

func TestAccessTokenizeQuoteEscape(t *testing.T) {
	line := `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET /a?q=%22x%22 HTTP/1.1" 200 12 "-" "agent \"quoted\" name"`
	record := accessRecordOf(t, line)
	item, ok := record.Item(apache.AccessItemUserAgent)
	if !ok {
		t.Fatal("userAgent item is absent")
	}
	if got, want := item.Value(), `agent "quoted" name`; got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
}

func TestAccessTokenizeFailures(t *testing.T) {
	cases := map[string]string{
		"too few items":        `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET / HTTP/1.1" 200`,
		"unterminated bracket": `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900 "GET / HTTP/1.1" 200 12 "-" "-"`,
		"unterminated quote":   `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET / HTTP/1.1 200 12 "-" "-"`,
		"trailing garbage":     fullShapeLine + " extra",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			var reader apache.AccessReader
			reader.Reset(newSingleLineReader(line))
			record, failure, err := reader.Next()
			if err != nil {
				t.Fatalf("Next() returned error %v", err)
			}
			if failure == nil {
				t.Fatal("Next() did not return a failure")
			}
			if failure.Stage != "tokenize" {
				t.Errorf("failure.Stage = %q, want tokenize", failure.Stage)
			}
			if record.RawText() != line {
				t.Errorf("record.RawText() = %q, want %q", record.RawText(), line)
			}
		})
	}
}
