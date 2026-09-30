package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
)

// synthesized error ログ 1 行。RFC 5737 の文書用範囲を使う。
const errorFullShapeLine = `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] AH01215: example message`

func errorRecordOf(t *testing.T, line string) apache.ErrorRecord {
	t.Helper()
	var reader apache.ErrorReader
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

func TestErrorTokenizeFullShape(t *testing.T) {
	record := errorRecordOf(t, errorFullShapeLine)
	want := map[apache.ErrorItemName]string{
		apache.ErrorItemTime:       "Tue Feb 01 11:22:33.123456 2000",
		apache.ErrorItemModule:     "cgi",
		apache.ErrorItemSeverity:   "error",
		apache.ErrorItemPid:        "4242",
		apache.ErrorItemTid:        "24",
		apache.ErrorItemClientIP:   "203.0.113.10",
		apache.ErrorItemClientPort: "50100",
		apache.ErrorItemMessage:    "AH01215: example message",
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

func TestErrorTokenizePreservesLiteralBackslashR(t *testing.T) {
	line := `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] line one\r: line two`
	record := errorRecordOf(t, line)
	item, ok := record.Item(apache.ErrorItemMessage)
	if !ok {
		t.Fatal("message item is absent")
	}
	want := `line one\r: line two`
	if item.Value() != want {
		t.Errorf("Value() = %q, want %q (the 2-char backslash-r must not be decoded)", item.Value(), want)
	}
}

func TestErrorTokenizeFailures(t *testing.T) {
	cases := map[string]string{
		"missing client bracket": `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] AH01215: message`,
		"malformed pid tid":      `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid four:tid 24] [client 203.0.113.10:50100] AH01215: message`,
		"empty message":          `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] `,
		"unterminated bracket":   `[Tue Feb 01 11:22:33.123456 2000] [cgi:error [pid 4242:tid 24] [client 203.0.113.10:50100] AH01215: message`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			var reader apache.ErrorReader
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
