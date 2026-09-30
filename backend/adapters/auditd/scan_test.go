package auditd_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 同じ事象の連続する行を 1 レコードにまとめる。
func TestReaderGroupsTheLinesOfOneEvent(t *testing.T) {
	record := readOneRecord(t, executedEventSource)

	wantLineCount := int64(strings.Count(executedEventSource, "\n"))
	if record.LineCount() != wantLineCount {
		t.Errorf("the record spans %d lines, want %d", record.LineCount(), wantLineCount)
	}
	if record.LineNumber() != 1 {
		t.Errorf("the record starts at line %d, want 1", record.LineNumber())
	}
	if record.ByteOffset() != 0 {
		t.Errorf("the record starts at byte %d, want 0", record.ByteOffset())
	}
	// 原文は、収集元の先頭から末尾の改行を除いた範囲と一致する。
	wantRawText := strings.TrimSuffix(executedEventSource, "\n")
	if record.RawText() != wantRawText {
		t.Errorf("the raw text is %q, want %q", record.RawText(), wantRawText)
	}
	if record.ByteLength() != int64(len(wantRawText)) {
		t.Errorf("the record spans %d bytes, want %d", record.ByteLength(), len(wantRawText))
	}
	requireLineTypes(t, record, "SYSCALL", "EXECVE", "CWD", "PATH", "PATH", "PROCTITLE")
}

// 事象の数は msg=audit(...) の異なりの数と一致する。
func TestReaderCountsOneRecordPerEvent(t *testing.T) {
	source := executedEventSource + failedEventSource + sessionEventSource + daemonEventSource
	records, problems := readRecords(t, source)
	if len(problems) != 0 {
		t.Fatalf("reading the source reported %v, want no problem", problems)
	}

	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		seen[record.Event().RawText] = struct{}{}
	}
	wantEvents := distinctEventTexts(source)
	if len(records) != len(wantEvents) {
		t.Fatalf("the source holds %d records for %d distinct events",
			len(records), len(wantEvents))
	}
	for _, event := range wantEvents {
		if _, found := seen[event]; !found {
			t.Errorf("the event %q reached no record", event)
		}
	}
}

// 同じ事象の行が連続しない収集元でも、1 事象を 1 レコードにまとめる。
func TestReaderGroupsTheInterleavedLinesOfOneEvent(t *testing.T) {
	// 2 つの事象の行が交互に並ぶ。
	const interleaved = "type=SYSCALL msg=audit(1000000000.100:20001): pid=1002 comm=\"first\"\n" +
		"type=SYSCALL msg=audit(1000000000.110:20002): pid=1003 comm=\"second\"\n" +
		"type=CWD msg=audit(1000000000.100:20001): cwd=\"/tmp\"\n" +
		"type=CWD msg=audit(1000000000.110:20002): cwd=\"/var\"\n" +
		"type=PROCTITLE msg=audit(1000000000.100:20001): proctitle=6669727374\n"

	records, problems := readRecords(t, interleaved)
	if len(problems) != 0 {
		t.Fatalf("reading the source reported %v, want no problem", problems)
	}
	wantEvents := distinctEventTexts(interleaved)
	if len(records) != len(wantEvents) {
		t.Fatalf("the source holds %d records for %d distinct events",
			len(records), len(wantEvents))
	}
	// 返す順は、事象の先頭の行が現れた順である。
	if records[0].Event().RawText != wantEvents[0] {
		t.Errorf("the first record is %q, want %q",
			records[0].Event().RawText, wantEvents[0])
	}
	if records[0].LineCount() != 3 {
		t.Errorf("the first event holds %d lines, want 3", records[0].LineCount())
	}
	if records[1].LineCount() != 2 {
		t.Errorf("the second event holds %d lines, want 2", records[1].LineCount())
	}
	requireLineTypes(t, records[0], "SYSCALL", "CWD", "PROCTITLE")

	// 原文はその事象の行だけを連ねた文字列である。間に入った別の事象の行を含まない。
	if strings.Contains(records[0].RawText(), "second") {
		t.Errorf("the first record raw text is %q, want only its own lines",
			records[0].RawText())
	}
	// byte の範囲は先頭の行から最後の行までを指す。
	firstLine := records[0].Lines()[0]
	lastLine := records[0].Lines()[records[0].LineCount()-1]
	wantLength := lastLine.ByteOffset() + int64(len(lastLine.RawText())) - firstLine.ByteOffset()
	if records[0].ByteLength() != wantLength {
		t.Errorf("the first record spans %d bytes, want %d",
			records[0].ByteLength(), wantLength)
	}
}

// 事象を指す鍵は秒とミリ秒と連番を別に持つ。
func TestReaderReadsTheEventKey(t *testing.T) {
	record := readOneRecord(t, executedEventSource)
	event := record.Event()
	if event.RawText != "1000000000.100:20001" {
		t.Errorf("the event raw text is %q, want %q", event.RawText, "1000000000.100:20001")
	}
	if event.Seconds != 1000000000 || event.Milliseconds != 100 || event.Serial != 20001 {
		t.Errorf("the event key is %+v, want seconds 1000000000, milliseconds 100, serial 20001",
			event)
	}
}

// 連番の大小を時刻の順として読まない。
func TestReaderKeepsTheEventOrderOfTheSource(t *testing.T) {
	// daemonEventSource の連番 12 は、先行する事象の連番 20001 より小さい。
	records, _ := readRecords(t, executedEventSource+daemonEventSource)
	if len(records) != 2 {
		t.Fatalf("the source holds %d records, want 2", len(records))
	}
	first, second := records[0].Event(), records[1].Event()
	if second.Serial >= first.Serial {
		t.Fatalf("the fixture no longer holds a decreasing serial: %d then %d",
			first.Serial, second.Serial)
	}
	if second.Seconds <= first.Seconds {
		t.Errorf("the later event is at %d seconds, want a value above %d",
			second.Seconds, first.Seconds)
	}
}

// 収集元の先頭が事象の途中で切れている状態を、読めなかった範囲として示す。
func TestReaderReportsATruncatedHead(t *testing.T) {
	var reader auditd.Reader
	reader.Reset(strings.NewReader(truncatedHeadSource))
	empty, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the truncated head: %v", err)
	}
	if failure == nil {
		t.Fatal("the truncated head reported no failure, want one")
	}
	if empty.LineCount() != 0 {
		t.Errorf("the failure carries %d lines, want the diagnosis alone", empty.LineCount())
	}
	if failure.Stage != core.FailureStageRead {
		t.Errorf("the failure stage is %q, want %q", failure.Stage, core.FailureStageRead)
	}
	if failure.DiagnosisClass != core.DiagnosisClassInconsistentInputConfirmed {
		t.Errorf("the diagnosis class is %q, want %q",
			failure.DiagnosisClass, core.DiagnosisClassInconsistentInputConfirmed)
	}
	if failure.LineNumber == nil || *failure.LineNumber != 1 {
		t.Errorf("the failure points at %v, want line 1", failure.LineNumber)
	}
	// 読めた範囲は除かない。切れている事象も次の Next がレコードとして返す。
	record, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the first record: %v", err)
	}
	if failure != nil {
		t.Errorf("the truncated event reported %q, want the diagnosis to come once",
			failure.ObservedResult)
	}
	requireLineTypes(t, record, "CWD", "PROCTITLE")

	// 続く完全な事象は診断を持たない。
	following, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("reading the second record: %v", err)
	}
	if failure != nil {
		t.Errorf("the complete event reported %q, want no failure", failure.ObservedResult)
	}
	if _, found := following.LineOfType(auditd.LineTypeSyscall); !found {
		t.Error("the second record carries no SYSCALL line")
	}
}

// SYSCALL の行を持たない種別の事象を、切れている先頭と混同しない。
func TestReaderAcceptsAnEventWithoutASyscallLine(t *testing.T) {
	_, problems := readRecords(t, sessionEventSource+daemonEventSource)
	if len(problems) != 0 {
		t.Errorf("reading the source reported %v, want no problem", problems)
	}
}

// Reset を呼ぶ前の Next は読み込みを始めない。
func TestReaderRequiresReset(t *testing.T) {
	var reader auditd.Reader
	if _, _, err := reader.Next(); err == nil {
		t.Error("Next without Reset returned no error, want one")
	}
}

func requireLineTypes(t *testing.T, record auditd.Record, want ...string) {
	t.Helper()
	got := make([]string, 0, len(record.Lines()))
	for _, line := range record.Lines() {
		got = append(got, line.Type())
	}
	if len(got) != len(want) {
		t.Fatalf("the record holds the line types %v, want %v", got, want)
	}
	for index, wanted := range want {
		if got[index] != wanted {
			t.Errorf("the line at %d is %q, want %q", index, got[index], wanted)
		}
	}
}

// distinctEventTexts は収集元から msg=audit(...) の異なりを、原文の並び順で返す。
func distinctEventTexts(source string) []string {
	const prefix = "msg=audit("
	seen := make(map[string]struct{})
	events := make([]string, 0, 8)
	for _, line := range strings.Split(source, "\n") {
		start := strings.Index(line, prefix)
		if start < 0 {
			continue
		}
		rest := line[start+len(prefix):]
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			continue
		}
		event := rest[:end]
		if _, found := seen[event]; found {
			continue
		}
		seen[event] = struct{}{}
		events = append(events, event)
	}
	return events
}
