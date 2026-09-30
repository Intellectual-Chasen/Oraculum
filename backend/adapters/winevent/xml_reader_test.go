package winevent_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// XML。ホスト名は example.test 系であり、原資料の値を写していない。
//
// 1 行目の宣言は version="1.1" を持つ。Go の encoding/xml は 1.1 の宣言を受け付けない。
const twoEventsDocument = `<?xml version="1.1" encoding="utf-8" standalone="yes" ?>

<Events>
<Event xmlns="urn:example:events"><System><Provider Name="Example-Provider" Guid="{00000000-0000-0000-0000-000000000001}"></Provider>
<EventID Qualifiers="">7</EventID>
<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"></TimeCreated>
<EventRecordID>101</EventRecordID>
<Execution ProcessID="11" ThreadID="12"></Execution>
<Channel>Application</Channel>
<Computer>host01.example.test</Computer>
</System>
<EventData><Data Name="Alpha">one</Data>
<Data>unnamed</Data>
</EventData>
</Event>

<Event xmlns="urn:example:events"><System><Provider Name="Example-Provider"></Provider>
<EventID>8</EventID>
<TimeCreated SystemTime="2001-02-03 04:05:07.654321"></TimeCreated>
<EventRecordID>102</EventRecordID>
<Channel>Application</Channel>
<Computer>host01.example.test</Computer>
</System>
</Event>
</Events>
`

func readAll(t *testing.T, document string) ([]winevent.Event, []*core.ImportFailure) {
	t.Helper()
	var reader winevent.XMLReader
	reader.Reset(strings.NewReader(document))
	var events []winevent.Event
	var failures []*core.ImportFailure
	for range 100 {
		event, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return events, failures
		}
		if err != nil {
			t.Fatalf("Next() returned error %v", err)
		}
		if failure != nil {
			failures = append(failures, failure)
			continue
		}
		events = append(events, event)
	}
	t.Fatal("Next() did not reach the end of the input")
	return nil, nil
}

func textOf(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("the item is absent")
	}
	return *value
}

func TestXMLReaderReadsEveryEventWithItsPosition(t *testing.T) {
	events, failures := readAll(t, twoEventsDocument)
	if len(failures) != 0 {
		t.Fatalf("failures = %+v, want none", failures)
	}
	firstStart := strings.Index(twoEventsDocument, "<Event xmlns")
	firstEnd := strings.Index(twoEventsDocument, "</Event>") + len("</Event>")
	secondStart := strings.LastIndex(twoEventsDocument, "<Event xmlns")
	want := []struct {
		recordID   string
		offset     int
		rawText    string
		lineNumber int64
		lineCount  int64
	}{
		{"101", firstStart, twoEventsDocument[firstStart:firstEnd], 4, 12},
		{"102", secondStart, "", 17, 8},
	}
	if len(events) != len(want) {
		t.Fatalf("read %d events, want %d", len(events), len(want))
	}
	for i, expected := range want {
		event := events[i]
		if got := textOf(t, event.System.EventRecordID); got != expected.recordID {
			t.Errorf("event %d EventRecordID = %q, want %q", i, got, expected.recordID)
		}
		if event.Source.ByteOffset != int64(expected.offset) {
			t.Errorf("event %d ByteOffset = %d, want %d", i, event.Source.ByteOffset, expected.offset)
		}
		if event.Source.LineNumber != expected.lineNumber || event.Source.LineCount != expected.lineCount {
			t.Errorf("event %d lines = %d+%d, want %d+%d", i, event.Source.LineNumber,
				event.Source.LineCount, expected.lineNumber, expected.lineCount)
		}
		if event.Source.ByteLength != int64(len(event.Source.RawText)) {
			t.Errorf("event %d ByteLength = %d, RawText has %d bytes", i, event.Source.ByteLength,
				len(event.Source.RawText))
		}
		if !strings.HasPrefix(event.Source.RawText, "<Event xmlns") || !strings.HasSuffix(event.Source.RawText, "</Event>") {
			t.Errorf("event %d RawText does not span <Event> to </Event>: %q", i, event.Source.RawText)
		}
		if expected.rawText != "" && event.Source.RawText != expected.rawText {
			t.Errorf("event %d RawText = %q, want %q", i, event.Source.RawText, expected.rawText)
		}
	}
	first := events[0]
	for _, item := range []struct {
		name string
		got  *string
		want string
	}{
		{"ProviderName", first.System.ProviderName, "Example-Provider"},
		{"ProviderGuid", first.System.ProviderGuid, "{00000000-0000-0000-0000-000000000001}"},
		{"EventID", first.System.EventID, "7"},
		{"EventIDQualifiers", first.System.EventIDQualifiers, ""},
		{"SystemTime", first.System.SystemTime, "2001-02-03T04:05:06.1234567Z"},
		{"ProcessID", first.System.ProcessID, "11"},
		{"ThreadID", first.System.ThreadID, "12"},
		{"Channel", first.System.Channel, "Application"},
		{"Computer", first.System.Computer, "host01.example.test"},
	} {
		if got := textOf(t, item.got); got != item.want {
			t.Errorf("%s = %q, want %q", item.name, got, item.want)
		}
	}
	wantData := []winevent.Value{{Name: "Alpha", Text: "one"}, {Name: "", Text: "unnamed"}}
	if len(first.EventData) != len(wantData) {
		t.Fatalf("EventData = %+v, want %+v", first.EventData, wantData)
	}
	for i := range wantData {
		if first.EventData[i] != wantData[i] {
			t.Errorf("EventData[%d] = %+v, want %+v", i, first.EventData[i], wantData[i])
		}
	}
	if events[1].System.EventIDQualifiers != nil || events[1].System.ProcessID != nil {
		t.Errorf("absent items read as present: %+v", events[1].System)
	}
}

// `<EventData`、`<EventID`、`<EventRecordID`、`<Events>` を 1 件の開始と読まない。
// 開始タグの直後が改行やタブでも 1 件の開始と読む。
func TestXMLReaderSplitsOnlyAtEventStartTags(t *testing.T) {
	document := "<Events><Event\n><System><EventID>1</EventID><EventRecordID>5</EventRecordID></System>" +
		"<EventData><Data Name=\"A\">x</Data></EventData></Event >" +
		"<Event\t><System><EventRecordID>6</EventRecordID></System></Event></Events>"
	events, failures := readAll(t, document)
	if len(failures) != 0 {
		t.Fatalf("failures = %+v, want none", failures)
	}
	if len(events) != 2 {
		t.Fatalf("read %d events, want 2", len(events))
	}
	if got := textOf(t, events[1].System.EventRecordID); got != "6" {
		t.Errorf("second EventRecordID = %q, want 6", got)
	}
	if !strings.HasSuffix(events[0].Source.RawText, "</Event >") {
		t.Errorf("first RawText = %q, want the end tag with a space", events[0].Source.RawText)
	}
}

// `</Event>` を持たない 1 件は次の `<Event` の手前で区切り、位置と理由を持つ失敗にする。
// 構文の壊れた 1 件も失敗にする。どちらの後ろの件も読む。
func TestXMLReaderContinuesAfterBrokenEvents(t *testing.T) {
	document := "<Events>\n" +
		"<Event><System><EventRecordID>1</EventRecordID></System>\n" +
		"<Event><System><EventRecordID>2</EventRecordID>\n</Data></System></Event>\n" +
		"<Event><System><EventRecordID>3</EventRecordID></System></Event>\n" +
		"</Events>\n"
	var reader winevent.XMLReader
	reader.Reset(strings.NewReader(document))

	truncated, failure, err := reader.Next()
	if err != nil || failure == nil {
		t.Fatalf("first Next() = %+v, %v; want a failure", failure, err)
	}
	if failure.Stage != core.FailureStageTokenize || failure.LineNumber == nil || *failure.LineNumber != 2 ||
		failure.ByteOffset == nil || *failure.ByteOffset != int64(len("<Events>\n")) {
		t.Errorf("truncated failure = %+v, want tokenize at line 2 byte %d", failure, len("<Events>\n"))
	}
	if !strings.Contains(failure.ObservedResult, "no </Event> end tag") {
		t.Errorf("truncated ObservedResult = %q", failure.ObservedResult)
	}
	if truncated.Source.RawText != "<Event><System><EventRecordID>1</EventRecordID></System>\n" {
		t.Errorf("truncated RawText = %q", truncated.Source.RawText)
	}

	malformed, failure, err := reader.Next()
	if err != nil || failure == nil {
		t.Fatalf("second Next() = %+v, %v; want a failure", failure, err)
	}
	if failure.LineNumber == nil || *failure.LineNumber != 3 || malformed.Source.LineCount != 2 {
		t.Errorf("malformed failure line = %v, LineCount = %d; want line 3 over 2 lines",
			failure.LineNumber, malformed.Source.LineCount)
	}
	// 閉じタグの食い違いは 1 件の 2 行目、収集元の 4 行目にある。
	if !strings.HasPrefix(failure.ObservedResult, "XML syntax error on line 4:") {
		t.Errorf("malformed ObservedResult = %q, want the line in the source", failure.ObservedResult)
	}

	event, failure, err := reader.Next()
	if err != nil || failure != nil {
		t.Fatalf("third Next() = %+v, %v; want an event", failure, err)
	}
	if got := textOf(t, event.System.EventRecordID); got != "3" {
		t.Errorf("EventRecordID = %q, want 3", got)
	}
	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("last Next() error = %v, want io.EOF", err)
	}
}

// `<UserData>` の子要素の値を、要素の path を名前にして除かずに持つ。
func TestXMLReaderKeepsUserDataValues(t *testing.T) {
	document := `<Event><System><EventID>9</EventID></System>` +
		`<UserData><LogCleared xmlns="urn:example:user"><SubjectUserName>analyst01</SubjectUserName>` +
		`<SubjectDomainName>EXAMPLE</SubjectDomainName><Empty></Empty></LogCleared></UserData></Event>`
	events, failures := readAll(t, document)
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	want := []winevent.Value{
		{Name: "UserData.LogCleared.SubjectUserName", Text: "analyst01"},
		{Name: "UserData.LogCleared.SubjectDomainName", Text: "EXAMPLE"},
		{Name: "UserData.LogCleared.Empty", Text: ""},
	}
	got := events[0].Sections
	if len(got) != len(want) {
		t.Fatalf("Sections = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Sections[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// 日本語を含む値を、原資料の byte 列のまま持つ。
func TestXMLReaderKeepsJapaneseBytes(t *testing.T) {
	const message = "ログの消去 &amp; 再開"
	document := `<Event><System><EventID>9</EventID></System>` +
		`<EventData><Data Name="Note">端末の説明</Data></EventData>` +
		`<RenderingInfo Culture="ja-JP"><Message>` + message + `</Message></RenderingInfo></Event>`
	events, failures := readAll(t, document)
	if len(events) != 1 || len(failures) != 0 {
		t.Fatalf("read %d events and failures %+v, want 1 event", len(events), failures)
	}
	if got := events[0].EventData[0].Text; got != "端末の説明" {
		t.Errorf("Data = %q (% x), want the source bytes", got, got)
	}
	var found bool
	for _, section := range events[0].Sections {
		if section.Name == "RenderingInfo.Message" {
			found = true
			if section.Text != "ログの消去 & 再開" {
				t.Errorf("Message = %q (% x), want the decoded source bytes", section.Text, section.Text)
			}
		}
	}
	if !found {
		t.Errorf("Sections = %+v, want RenderingInfo.Message", events[0].Sections)
	}
	if !strings.Contains(events[0].Source.RawText, message) {
		t.Errorf("RawText does not keep the source bytes: %q", events[0].Source.RawText)
	}
}

// `<Event>` 要素を 1 つも持たない file は、1 件の失敗になる。空の file は失敗にならない。
func TestXMLReaderReportsAFileWithoutEvents(t *testing.T) {
	_, failures := readAll(t, "<?xml version=\"1.0\"?>\n<Events></Events>\n")
	if len(failures) != 1 || failures[0].Stage != core.FailureStageTokenize {
		t.Fatalf("failures = %+v, want one tokenize failure", failures)
	}
	events, failures := readAll(t, "\n")
	if len(events) != 0 || len(failures) != 0 {
		t.Errorf("an empty file read as %d events and %+v", len(events), failures)
	}
}

const goodEvent = "<Event><System><EventRecordID>7</EventRecordID></System></Event>"

// 件の外の byte で、置ける文字列でないものは、そこから次の `<Event` の手前までを原文に持つ
// 失敗になる。後ろの件は読む。
func TestXMLReaderReportsBytesOutsideEvents(t *testing.T) {
	for _, item := range []struct {
		name       string
		document   string
		unexpected string
		line       int64
	}{
		{"misspelled start tag", "<Events>\n<Evxnt><System/></Evxnt></Event>\n" + goodEvent + "</Events>",
			"<Evxnt><System/></Evxnt></Event>\n", 2},
		{"bytes before a start tag", "<Events>\nstray text\n" + goodEvent + "</Events>", "stray text\n", 2},
		{"missing first half", "<Data Name=\"A\">x</Data></EventData></Event>\n" + goodEvent,
			"<Data Name=\"A\">x</Data></EventData></Event>\n", 1},
		{"prefixed start tag", goodEvent + "\n<e:Event><System/></e:Event>\n", "<e:Event><System/></e:Event>\n", 2},
		{"empty element", goodEvent + "\n<Event/>\n", "<Event/>\n", 2},
	} {
		var reader winevent.XMLReader
		reader.Reset(strings.NewReader(item.document))
		var events int
		var failures []winevent.Event
		for range 10 {
			event, failure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("%s: Next() error %v", item.name, err)
			}
			if failure == nil {
				events++
				continue
			}
			if failure.Stage != core.FailureStageTokenize || failure.LineNumber == nil ||
				*failure.LineNumber != item.line || failure.ByteOffset == nil ||
				*failure.ByteOffset != int64(strings.Index(item.document, item.unexpected)) {
				t.Errorf("%s: failure = %+v, want tokenize at line %d", item.name, failure, item.line)
			}
			failures = append(failures, event)
		}
		if events != 1 || len(failures) != 1 || failures[0].Source.RawText != item.unexpected {
			t.Errorf("%s: %d events and failures %+v, want 1 event and the unexpected bytes %q",
				item.name, events, failures, item.unexpected)
		}
	}
}

// 宣言、DOCTYPE、コメント、`<Events>` の開始と終了タグは件の外に置ける。コメントと CDATA の
// 中の `<Event` と `</Event>` を境界と読まない。
func TestXMLReaderSkipsMarkupOutsideAndInsideEvents(t *testing.T) {
	document := "<?xml version=\"1.1\"?>\n<!DOCTYPE Events [<!ENTITY x \"y\">]>\n" +
		"<!-- <Event><System/></Event> -->\n<Events xmlns=\"urn:example:events\">\n" +
		"<Event><System><EventRecordID>8</EventRecordID></System><!-- </Event> -->" +
		"<EventData><Data Name=\"A\"><![CDATA[</Event><Event >]]></Data></EventData></Event>\n" +
		"</Events >\n"
	events, failures := readAll(t, document)
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	if got := events[0].EventData[0].Text; got != "</Event><Event >" {
		t.Errorf("CDATA = %q, want the tags as text", got)
	}
}

// 先頭の UTF-8 の BOM は件の外に置ける。位置は BOM を含めた file の中の byte 位置である。
func TestXMLReaderAcceptsAByteOrderMark(t *testing.T) {
	document := "\xEF\xBB\xBF<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<Events>" + goodEvent + "</Events>\n"
	events, failures := readAll(t, document)
	if len(events) != 1 || len(failures) != 0 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	if want := int64(strings.Index(document, goodEvent)); events[0].Source.ByteOffset != want {
		t.Errorf("ByteOffset = %d, want %d", events[0].Source.ByteOffset, want)
	}
	// BOM は file の先頭だけに置ける。
	_, failures = readAll(t, "<Events>\xEF\xBB\xBF"+goodEvent+"</Events>")
	if len(failures) != 1 {
		t.Errorf("failures = %+v, want one for a byte order mark after the start", failures)
	}
}

// UTF-16 で書いた file は、file 全体を原文に持つ件の外の byte の失敗 1 件になる。
func TestXMLReaderReportsUTF16AsBytesOutsideEvents(t *testing.T) {
	text := "<?xml version=\"1.0\" encoding=\"utf-16\"?>\n<Events>" + goodEvent + "</Events>\n"
	littleEndian := append([]byte{0xFF, 0xFE}, utf16le(text)...)
	bigEndian := utf16le(text)
	for i := 0; i < len(bigEndian); i += 2 {
		bigEndian[i], bigEndian[i+1] = bigEndian[i+1], bigEndian[i]
	}
	for _, item := range []struct {
		name     string
		document string
	}{
		{"little endian with a byte order mark", string(littleEndian)},
		{"big endian without a byte order mark", string(bigEndian)},
	} {
		var reader winevent.XMLReader
		reader.Reset(strings.NewReader(item.document))
		var failures []winevent.Event
		for range 10 {
			event, failure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("%s: Next() error %v", item.name, err)
			}
			if failure == nil {
				t.Fatalf("%s: read an event %+v", item.name, event)
			}
			if failure.Stage != core.FailureStageTokenize ||
				failure.ObservedResult != "bytes outside any <Event> element that are none of them" ||
				failure.LineNumber == nil || *failure.LineNumber != 1 ||
				failure.ByteOffset == nil || *failure.ByteOffset != 0 {
				t.Errorf("%s: failure = %+v, want bytes outside events at line 1, byte 0", item.name, failure)
			}
			failures = append(failures, event)
		}
		if len(failures) != 1 || failures[0].Source.RawText != item.document {
			t.Errorf("%s: failures %+v, want one holding the whole file", item.name, failures)
		}
	}
}

// 終わりの無い CDATA とコメントは、その 1 件または件の外の文字列だけを失敗にし、後ろの件を
// 読む。
func TestXMLReaderReadsPastUnterminatedSections(t *testing.T) {
	for _, item := range []struct {
		name       string
		document   string
		unexpected string
	}{
		{"CDATA inside an event",
			"<Events>\n<Event><System/><EventData><Data><![CDATA[a</Data></EventData></Event>\n" + goodEvent + "</Events>\n",
			"<Event><System/><EventData><Data><![CDATA[a</Data></EventData></Event>"},
		{"comment outside events", "<Events>\n<!-- x\n" + goodEvent + "</Events>\n", "<!-- x\n"},
	} {
		var reader winevent.XMLReader
		reader.Reset(strings.NewReader(item.document))
		var records []string
		var failed []string
		for range 10 {
			event, failure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("%s: Next() error %v", item.name, err)
			}
			if failure != nil {
				failed = append(failed, event.Source.RawText)
				continue
			}
			records = append(records, textOf(t, event.System.EventRecordID))
		}
		if len(failed) != 1 || failed[0] != item.unexpected || len(records) != 1 || records[0] != "7" {
			t.Errorf("%s: failed %q and records %v, want %q and the following event", item.name, failed,
				records, item.unexpected)
		}
	}
}

// 読み込みに失敗したときは、読めた byte の数を位置にした read の失敗と error を返す。
func TestXMLReaderReportsAReadFailureAtTheBytesRead(t *testing.T) {
	cause := errors.New("device detached")
	var reader winevent.XMLReader
	reader.Reset(io.MultiReader(strings.NewReader("<Events>\n<Eve"), iotest.ErrReader(cause)))
	_, failure, err := reader.Next()
	if !errors.Is(err, cause) {
		t.Fatalf("Next() error = %v, want the read error", err)
	}
	if failure == nil || failure.Stage != core.FailureStageRead || failure.ByteOffset == nil ||
		*failure.ByteOffset != int64(len("<Events>\n<Eve")) || failure.LineNumber == nil || *failure.LineNumber != 2 {
		t.Errorf("failure = %+v, want a read failure at byte %d on line 2", failure, len("<Events>\n<Eve"))
	}
	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next() after the read failure = %v, want io.EOF", err)
	}
}

func TestXMLReaderRequiresReset(t *testing.T) {
	var reader winevent.XMLReader
	if _, _, err := reader.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("Next() before Reset returned %v, want an error", err)
	}
}
