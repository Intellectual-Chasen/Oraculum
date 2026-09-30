package squid_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const exampleLine = `192.0.2.8 - - [14/Feb/2024:08:09:10 +0530] "GET http://198.51.100.9:8080/a?q=1 HTTP/1.1" 200 42 "-" "agent" TCP_MISS:HIER_DIRECT`

func readOne(t *testing.T, input string) (squid.Record, *core.ImportFailure) {
	t.Helper()
	var reader squid.Reader
	reader.Reset(strings.NewReader(input), squid.LayoutCombined)
	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading one record: %v", err)
	}
	return record, failure
}

func readOK(t *testing.T, input string) squid.Record {
	t.Helper()
	record, failure := readOne(t, input)
	if failure != nil {
		t.Fatalf("tokenizing: %+v", failure)
	}
	return record
}

func checkItems(t *testing.T, record squid.Record, complete bool) {
	t.Helper()
	var lexemes []string
	for _, item := range record.Items() {
		start, size := item.ByteOffset(), item.ByteLength()
		if start < 0 || size < 0 || start > int64(len(record.RawText())) || size > int64(len(record.RawText()))-start {
			t.Fatalf("item outside record: offset=%d length=%d", start, size)
		}
		if record.RawText()[start:start+size] != item.RawValue() {
			t.Fatal("item bytes differ from its source range")
		}
		lexemes = append(lexemes, item.RawValue())
	}
	if complete && (len(lexemes) != 10 || strings.Join(lexemes, " ") != record.RawText()) {
		t.Fatal("items do not reconstruct the record")
	}
}

func TestTokenizeItems(t *testing.T) {
	record := readOK(t, exampleLine)
	checkItems(t, record, true)
	want := []struct {
		name   squid.ItemName
		raw    string
		offset int64
	}{
		{squid.ItemClientIP, "192.0.2.8", 0},
		{squid.ItemIdent, "-", 10},
		{squid.ItemUser, "-", 12},
		{squid.ItemRequestTime, "[14/Feb/2024:08:09:10 +0530]", 14},
		{squid.ItemRequestLine, `"GET http://198.51.100.9:8080/a?q=1 HTTP/1.1"`, 43},
	}
	for _, expected := range want {
		item, ok := record.Item(expected.name)
		if !ok || item.RawValue() != expected.raw || item.ByteOffset() != expected.offset {
			t.Errorf("item %s: raw=%q offset=%d", expected.name, item.RawValue(), item.ByteOffset())
		}
	}
	names := []squid.ItemName{squid.ItemClientIP, squid.ItemIdent, squid.ItemUser, squid.ItemRequestTime,
		squid.ItemRequestLine, squid.ItemStatusCode, squid.ItemReplyBytes, squid.ItemReferer, squid.ItemUserAgent, squid.ItemSquidStatus}
	for index, item := range record.Items() {
		if item.Name() != names[index] {
			t.Fatal("item order differs")
		}
	}
	items := record.Items()
	items[0] = squid.Item{}
	if item, _ := record.Item(squid.ItemClientIP); item.RawValue() != "192.0.2.8" {
		t.Fatal("Items aliases Record")
	}
	if _, ok := record.Item("unknown"); ok {
		t.Fatal("unknown item is present")
	}
}

func TestTokenizeQuotesAndBrackets(t *testing.T) {
	line := strings.Replace(exampleLine, `"agent"`, `"日本語 \"quoted\" \\ end\r\n\t"`, 1)
	record := readOK(t, line)
	item, _ := record.Item(squid.ItemUserAgent)
	if !item.Quoted() || item.RawValue() != `"日本語 \"quoted\" \\ end\r\n\t"` || item.Value() != "日本語 \"quoted\" \\ end\r\n\t" {
		t.Fatalf("quoted item: raw=%q value=%q", item.RawValue(), item.Value())
	}
	timestamp, _ := record.Item(squid.ItemRequestTime)
	if timestamp.Quoted() || timestamp.RawValue() != "[14/Feb/2024:08:09:10 +0530]" || timestamp.Value() != timestamp.RawValue() {
		t.Fatal("time brackets not preserved")
	}
	checkItems(t, record, true)
	for _, value := range []string{"", "-"} {
		r := readOK(t, strings.Replace(exampleLine, `"agent"`, `"`+value+`"`, 1))
		i, ok := r.Item(squid.ItemUserAgent)
		if !ok || i.Value() != value || i.RawValue() != `"`+value+`"` {
			t.Fatal("empty and absent lexemes differ")
		}
	}
}

func TestTokenizeFailuresPreserveSource(t *testing.T) {
	for _, tt := range []struct {
		line string
		want failureDetails
	}{
		{"", failureDetails{"a value for the clientIp item", "the end of the record at byte offset 0", 0}},
		{"short", failureDetails{"the literal \" \" before the ident item", "the end of the record at byte offset 5", 5}},
		{exampleLine + " ", failureDetails{"the end of the record after the last item", "the byte 0x20 at byte offset 128", 128}},
		{exampleLine + " extra", failureDetails{"the end of the record after the last item", "the byte 0x20 at byte offset 128", 128}},
		{" " + exampleLine, failureDetails{"a value for the clientIp item", "the byte 0x20 at byte offset 0", 0}},
		{strings.Replace(exampleLine, " - - ", "  - - ", 1), failureDetails{"a value for the ident item", "the byte 0x20 at byte offset 10", 10}},
		{strings.Replace(exampleLine, "[14/", "14/", 1), failureDetails{"an opening bracket of the requestTime item", "the byte 0x31 at byte offset 14", 14}},
		{strings.Replace(exampleLine, "+0530]", "+0530", 1), failureDetails{"a closing bracket of the requestTime item", "the end of the record at byte offset 127", 127}},
		{strings.Replace(exampleLine, `"agent"`, `"agent`, 1), failureDetails{"a closing quote", "the end of the record at byte offset 127", 127}},
		{strings.Replace(exampleLine, `"agent"`, `"agent"x`, 1), failureDetails{"the literal \" \" before the squidStatus item", "the byte 0x78 at byte offset 107", 107}},
		{strings.Replace(exampleLine, `"agent"`, `"\q"`, 1), failureDetails{"a Squid quoted escape", "the byte 0x71 at byte offset 102", 102}},
		{strings.Replace(exampleLine, `"agent"`, "\"a\tb\"", 1), failureDetails{"an item without literal control separators", "the byte 0x22 at byte offset 100", 100}},
		{strings.Replace(exampleLine, "200 42", "200", 1), failureDetails{"an opening quote", "the byte 0x54 at byte offset 105", 105}},
		{strings.Replace(exampleLine, `"agent"`, "agent", 1), failureDetails{"an opening quote", "the byte 0x61 at byte offset 100", 100}},
		{`192.0.2.8 - - [time] "x\`, failureDetails{"a quoted escape", "the end of the record at byte offset 24", 24}},
	} {
		line := tt.line
		record, failure := readOne(t, line+"\n")
		if failure == nil || failure.Stage != core.FailureStageTokenize {
			t.Fatalf("missing tokenize failure for %q", line)
		}
		if record.RawText() != line {
			t.Fatal("failed record lost raw text")
		}
		checkItems(t, record, false)
		checkFailure(t, failure, core.FailureStageTokenize, 1, tt.want)
	}
}

type failureDetails struct {
	expected string
	observed string
	offset   int64
}

func checkFailure(t *testing.T, failure *core.ImportFailure, stage core.FailureStage, line int64, want failureDetails) {
	t.Helper()
	if failure == nil {
		t.Fatal("missing failure")
	}
	if failure.Stage != stage || failure.DiagnosisClass != core.DiagnosisClassUndetermined ||
		failure.LineNumber == nil || *failure.LineNumber != line || failure.ByteOffset == nil {
		t.Fatal("failure classification or position differs")
	}
	if failure.Interpretation == "" || failure.ExpectedMeaning == "" || failure.ObservedResult == "" || failure.UnresolvedReason == "" {
		t.Fatal("failure lacks diagnostic items")
	}
	wantStage := map[core.FailureStage][2]string{
		core.FailureStageRead:      {"source bytes read up to LF, CR LF, or EOF, without lexical interpretation", "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"},
		core.FailureStageTokenize:  {"Squid items separated by the literals of the logformat, with Squid quoted escapes", "the lexical structure alone does not distinguish unsupported format, inconsistent input, and tokenizer defects"},
		core.FailureStageNormalize: {"Squid time interpreted by the logformat time code, or request target authority derived from decoded request text", "the value interpretation alone does not distinguish unsupported format, inconsistent input, and normalization defects"},
	}[stage]
	if failure.Interpretation != wantStage[0] || failure.UnresolvedReason != wantStage[1] {
		t.Errorf("stage diagnostics differ: interpretation=%q unresolved=%q", failure.Interpretation, failure.UnresolvedReason)
	}
	if failure.ExpectedMeaning != want.expected || failure.ObservedResult != want.observed || *failure.ByteOffset != want.offset {
		t.Errorf("diagnosis: expected=%q observed=%q offset=%d; want %+v", failure.ExpectedMeaning, failure.ObservedResult, *failure.ByteOffset, want)
	}
	if failure.SourceId != "" || failure.SourceContentSha256 != "" || failure.ParserVersion != "" || failure.SanitizedMessage != "" || failure.RecordRef != nil || failure.RawTextRef != "" {
		t.Fatal("failure contains caller-owned items")
	}
	if !errors.Is(failure.Validate(), core.ErrMissingRequiredItem) {
		t.Fatal("incomplete failure should require source identity")
	}
}

func TestReaderLineEndingsAndFinalRecord(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		var reader squid.Reader
		reader.Reset(strings.NewReader(exampleLine+ending+"bad"+ending+exampleLine), squid.LayoutCombined)
		offset := int64(0)
		for index, line := range []string{exampleLine, "bad", exampleLine} {
			r, f, err := reader.Next()
			if err != nil {
				t.Fatal(err)
			}
			if (f != nil) != (index == 1) {
				t.Fatal("unexpected lexical status")
			}
			wantEnding := ending
			if index == 2 {
				wantEnding = ""
			}
			if r.RawText() != line || r.LineNumber() != int64(index+1) || r.ByteOffset() != offset || r.LineEnding() != wantEnding {
				t.Fatal("record text or location differs")
			}
			if f != nil && (f.ByteOffset == nil || *f.ByteOffset != offset+3) {
				t.Fatal("failure offset is not source-relative")
			}
			offset += int64(len(line) + len(ending))
		}
		if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("want EOF, got %v", err)
		}
		reader.Reset(strings.NewReader(exampleLine), squid.LayoutCombined)
		if r, _, err := reader.Next(); err != nil || r.LineNumber() != 1 || r.ByteOffset() != 0 {
			t.Fatal("Reset did not reset state")
		}
	}
}

func TestReaderEmptyAndUninitialized(t *testing.T) {
	var reader squid.Reader
	if _, _, err := reader.Next(); err == nil {
		t.Fatal("uninitialized Reader succeeded")
	}
	reader.Reset(nil, squid.LayoutCombined)
	if _, _, err := reader.Next(); err == nil {
		t.Fatal("nil input succeeded")
	}
	reader.Reset(strings.NewReader(""), squid.LayoutCombined)
	if _, f, err := reader.Next(); f != nil || !errors.Is(err, io.EOF) {
		t.Fatal("empty input not EOF")
	}
}

func TestReaderByteLimit(t *testing.T) {
	const limit = 1 << 20
	for _, size := range []int{70 << 10, limit, limit + 1} {
		line := strings.Replace(exampleLine, "agent", strings.Repeat("a", size-len(exampleLine)+len("agent")), 1)
		for _, ending := range []string{"", "\n", "\r\n"} {
			var reader squid.Reader
			reader.Reset(strings.NewReader(line+ending), squid.LayoutCombined)
			r, f, err := reader.Next()
			if size <= limit {
				if err != nil || f != nil || r.RawText() != line || r.LineEnding() != ending {
					t.Fatalf("allowed size=%d ending=%q failed: %v", size, ending, err)
				}
			} else {
				if err == nil || errors.Is(err, io.EOF) {
					t.Fatal("oversized record succeeded")
				}
				checkFailure(t, f, core.FailureStageRead, 1, failureDetails{"a complete record within the byte limit", "squid: record exceeds the byte limit", int64(limit + 1)})
				if len(r.RawText()) > limit+1 || *f.ByteOffset != int64(len(r.RawText())) {
					t.Fatal("oversized record not bounded")
				}
			}
		}
	}
	var reader squid.Reader
	reader.Reset(strings.NewReader(strings.Repeat("a", limit)+"\r"), squid.LayoutCombined)
	if _, f, err := reader.Next(); err == nil || f == nil {
		t.Fatal("bare CR at EOF bypassed limit")
	}
}

var errBroken = errors.New("test source failure")

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errBroken }

func TestReaderIOFailure(t *testing.T) {
	for _, fragment := range []string{"", "fragment"} {
		var reader squid.Reader
		reader.Reset(io.MultiReader(strings.NewReader(exampleLine+"\n"+fragment), brokenReader{}), squid.LayoutCombined)
		if _, f, err := reader.Next(); f != nil || err != nil {
			t.Fatal("first record failed")
		}
		r, f, err := reader.Next()
		if !errors.Is(err, errBroken) || r.RawText() != fragment {
			t.Fatal("lost I/O error or fragment")
		}
		if fragment == "" && (r.LineNumber() != 0 || r.ByteOffset() != 0 || r.LineEnding() != "" || len(r.Items()) != 0) {
			t.Fatal("I/O failure without bytes returned a record")
		}
		if fragment != "" && r.LineNumber() != 2 {
			t.Fatal("fragment lost its line number")
		}
		if strings.Contains(err.Error(), "reading a record byte") {
			t.Fatal("read error wrapped twice")
		}
		checkFailure(t, f, core.FailureStageRead, 2, failureDetails{"a complete record within the byte limit", "test source failure", int64(len(exampleLine) + 1 + len(fragment))})
		if *f.ByteOffset != int64(len(exampleLine)+1+len(fragment)) {
			t.Fatal("I/O failure offset differs")
		}
		if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
			t.Fatal("reader continued after I/O failure")
		}
	}
}

func TestReaderPreservesIOFailureAfterLimitAndCR(t *testing.T) {
	var reader squid.Reader
	reader.Reset(io.MultiReader(strings.NewReader(strings.Repeat("a", 1<<20)+"\r"), brokenReader{}), squid.LayoutCombined)
	_, failure, err := reader.Next()
	if !errors.Is(err, errBroken) {
		t.Fatalf("I/O error was replaced: %v", err)
	}
	checkFailure(t, failure, core.FailureStageRead, 1, failureDetails{"a complete record within the byte limit", "test source failure", (1 << 20) + 1})
}
