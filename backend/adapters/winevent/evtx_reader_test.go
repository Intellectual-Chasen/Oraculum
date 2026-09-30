package winevent_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// synthFileTime は 2001-02-03T04:05:06.1234567Z の FILETIME である。値である。
const synthFileTime uint64 = 126256467061234567

// synthEventAt はプロセスの作成のイベントを返す。
func synthEventAt(headerID, recordID uint64, computer string) synthEvent {
	return synthEvent{
		headerID: headerID, recordID: recordID,
		writtenAt: synthFileTime + headerID, created: synthFileTime + headerID,
		provider: "Microsoft-Windows-Security-Auditing", eventID: 4688, keywords: 0x8000000000000301,
		channel: "Security", computer: computer,
		data: []synthData{{name: "NewProcessId", value: "0x2b"}, {name: "NewProcessName", value: `C:\Example\child.exe`}},
	}
}

// evtxItem は Next が返した 1 件である。
type evtxItem struct {
	event   winevent.Event
	failure *core.ImportFailure
}

func readEVTX(t *testing.T, content []byte) ([]evtxItem, *winevent.EVTXReader) {
	t.Helper()
	var reader winevent.EVTXReader
	reader.Reset(bytes.NewReader(content))
	var items []evtxItem
	for range 1000 {
		event, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return items, &reader
		}
		if err != nil {
			t.Fatalf("Next() returned error %v", err)
		}
		items = append(items, evtxItem{event, failure})
	}
	t.Fatal("Next() did not reach the end")
	return nil, nil
}

// sectionText は Sections の名前の値を返す。
func sectionText(event winevent.Event, name string) (string, bool) {
	for _, value := range event.Sections {
		if value.Name == name {
			return value.Text, true
		}
	}
	return "", false
}

func headerText(t *testing.T, fields []core.RecordField, name string) string {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			raw, _ := field.Text.RawTextValue()
			return raw
		}
	}
	t.Fatalf("the file header carries no %q in %+v", name, fields)
	return ""
}

func TestEVTXReaderReadsRecordsAcrossChunks(t *testing.T) {
	first := newSynthChunk()
	firstStart, firstSize := first.record(synthEventAt(7, 1007, "host-a.example.test"))
	secondStart, _ := first.record(synthEventAt(8, 1008, "host-b.example.test"))
	next := newSynthChunk()
	nextStart, nextSize := next.record(synthEventAt(9, 1009, "host-a.example.test"))
	file := append(synthFileHeader(10, 2, 0), first.bytes()...)
	file = append(file, next.bytes()...)

	items, reader := readEVTX(t, file)
	if len(items) != 3 {
		t.Fatalf("read %d items, want the 3 records", len(items))
	}
	for _, item := range items {
		if item.failure != nil {
			t.Fatalf("unexpected failure %+v", item.failure)
		}
	}
	event := items[0].event
	wantOffset := int64(synthFileHeaderSize + firstStart)
	if event.Source.ByteOffset != wantOffset || event.Source.ByteLength != int64(firstSize) {
		t.Errorf("source = offset %d length %d, want %d %d", event.Source.ByteOffset, event.Source.ByteLength,
			wantOffset, firstSize)
	}
	if got := items[1].event.Source.ByteOffset; got != int64(synthFileHeaderSize+secondStart) {
		t.Errorf("second record offset = %d, want %d", got, synthFileHeaderSize+secondStart)
	}
	third := items[2].event.Source
	if want := int64(synthFileHeaderSize + synthChunkSize + nextStart); third.ByteOffset != want ||
		third.ByteLength != int64(nextSize) {
		t.Errorf("third record = offset %d length %d, want %d %d", third.ByteOffset, third.ByteLength, want, nextSize)
	}
	if event.Source.LineNumber != 0 {
		t.Errorf("an EVTX record carries line %d, want none", event.Source.LineNumber)
	}
	wantXML := `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"></Provider>` +
		`<EventID>4688</EventID><Keywords>0x8000000000000301</Keywords>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:06.1234574Z"></TimeCreated>` +
		`<EventRecordID>1007</EventRecordID><Channel>Security</Channel>` +
		`<Computer>host-a.example.test</Computer></System>` +
		`<EventData><Data Name="NewProcessId">0x2b</Data>` +
		`<Data Name="NewProcessName">C:\Example\child.exe</Data></EventData></Event>`
	if event.Source.RawText != wantXML {
		t.Errorf("raw text =\n%s\nwant\n%s", event.Source.RawText, wantXML)
	}
	if got := *event.System.EventRecordID; got != "1007" {
		t.Errorf("EventRecordID = %q, want 1007", got)
	}
	if got, _ := sectionText(event, "RecordHeader.RecordID"); got != "7" {
		t.Errorf("the record header number = %q, want 7 apart from EventRecordID", got)
	}
	if got := event.EventData[1]; got.Name != "NewProcessName" || got.Text != `C:\Example\child.exe` {
		t.Errorf("EventData[1] = %+v", got)
	}
	header := reader.SourceHeader()
	if got := headerText(t, header, "NextRecordID"); got != "10" {
		t.Errorf("NextRecordID = %q, want 10", got)
	}
	if got := headerText(t, header, "Dirty"); got != "false" {
		t.Errorf("Dirty = %q, want false", got)
	}
}

func TestEVTXReaderObservesProcessCreationPerComputer(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 51, "host-a.example.test"))
	chunk.record(synthEventAt(2, 52, "host-b.example.test"))
	items, _ := readEVTX(t, append(synthFileHeader(3, 1, 0), chunk.bytes()...))
	computers := map[string]bool{}
	for _, item := range items {
		observation, failure := winevent.Observe(item.event)
		if failure != nil {
			t.Fatalf("Observe failed: %+v", failure)
		}
		if !observation.ProcessStart {
			t.Error("a 4688 record is not a process start")
		}
		if observation.EventTime == nil || observation.EventTime.Precision != core.PrecisionMicrosecond {
			t.Errorf("event time = %+v, want microsecond precision", observation.EventTime)
		}
		raw, _ := observation.Terminal[0].Text.RawTextValue()
		computers[raw] = true
	}
	if !computers["host-a.example.test"] || !computers["host-b.example.test"] {
		t.Errorf("terminals = %v, want one per Computer", computers)
	}
}

func TestEVTXReaderReadsRecordsBeyondTheHeaderOfADirtyFile(t *testing.T) {
	chunk := newSynthChunk()
	for id := uint64(1); id <= 4; id++ {
		chunk.record(synthEventAt(id, id, "host-a.example.test"))
	}
	// 見出しの次のレコード番号は 3 であり、番号 3 と 4 のレコードは見出しの値を超える。
	items, reader := readEVTX(t, append(synthFileHeader(3, 1, 1), chunk.bytes()...))
	var ids []string
	for _, item := range items {
		if item.failure != nil {
			t.Fatalf("unexpected failure %+v", item.failure)
		}
		ids = append(ids, *item.event.System.EventRecordID)
	}
	if strings.Join(ids, ",") != "1,2,3,4" {
		t.Errorf("EventRecordIDs = %v, want 1 to 4", ids)
	}
	if got := headerText(t, reader.SourceHeader(), "Dirty"); got != "true" {
		t.Errorf("Dirty = %q, want true", got)
	}
	if got := headerText(t, reader.SourceHeader(), "NextRecordID"); got != "3" {
		t.Errorf("NextRecordID = %q, want 3", got)
	}
}

// 見出しが記録した chunk の数と、走査で数えた署名を持つ chunk の数を別の値として返す。
// 書き込みの途中で閉じられた file は、見出しの数より多い chunk を持つ。
func TestEVTXReaderCountsTheSignedChunksApartFromTheHeader(t *testing.T) {
	first, second := newSynthChunk(), newSynthChunk()
	first.record(synthEventAt(1, 1, "host-a.example.test"))
	second.record(synthEventAt(2, 2, "host-a.example.test"))
	content := append(synthFileHeader(2, 1, 1), first.bytes()...)
	_, reader := readEVTX(t, append(content, second.bytes()...))
	if got := headerText(t, reader.SourceHeader(), "ChunkCount"); got != "1" {
		t.Errorf("ChunkCount = %q, want the 1 chunk of the header", got)
	}
	if got := headerText(t, reader.SourceHeader(), "SignedChunkCount"); got != "2" {
		t.Errorf("SignedChunkCount = %q, want the 2 chunks in the file", got)
	}
}

// failureOf は items の中の失敗を 1 件だけ返す。
func failureOf(t *testing.T, items []evtxItem) evtxItem {
	t.Helper()
	var found []evtxItem
	for _, item := range items {
		if item.failure != nil {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d failures, want 1: %+v", len(found), found)
	}
	return found[0]
}

func recordIDs(items []evtxItem) string {
	var ids []string
	for _, item := range items {
		if item.failure == nil {
			ids = append(ids, *item.event.System.EventRecordID)
		}
	}
	return strings.Join(ids, ",")
}

func TestEVTXReaderReportsABrokenChunkAndReadsTheNext(t *testing.T) {
	first := newSynthChunk()
	first.record(synthEventAt(1, 1, "host-a.example.test"))
	broken := newSynthChunk()
	broken.record(synthEventAt(2, 2, "host-a.example.test"))
	brokenBytes := broken.bytes()
	copy(brokenBytes, "XlfChnk\x00")
	last := newSynthChunk()
	last.record(synthEventAt(3, 3, "host-a.example.test"))
	file := append(synthFileHeader(4, 3, 0), first.bytes()...)
	file = append(file, brokenBytes...)
	file = append(file, last.bytes()...)

	items, _ := readEVTX(t, file)
	failed := failureOf(t, items)
	wantOffset := int64(synthFileHeaderSize + synthChunkSize)
	if *failed.failure.ByteOffset != wantOffset || failed.event.Source.ByteLength != synthChunkSize {
		t.Errorf("failure at %d length %d, want the chunk at %d", *failed.failure.ByteOffset,
			failed.event.Source.ByteLength, wantOffset)
	}
	if failed.failure.Stage != core.FailureStageTokenize || !strings.Contains(failed.failure.ObservedResult, "chunk") {
		t.Errorf("failure = %+v", failed.failure)
	}
	if !strings.HasPrefix(failed.event.Source.RawText, "586C6643686E6B00") {
		t.Errorf("the failure's raw text starts with %.16q, want the chunk bytes in hexadecimal", failed.event.Source.RawText)
	}
	if got := recordIDs(items); got != "1,3" {
		t.Errorf("records = %s, want the records of the first and last chunks", got)
	}
}

func TestEVTXReaderReportsABrokenRecordAndReadsTheNextChunk(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	brokenStart, brokenSize := chunk.record(synthEventAt(2, 2, "host-a.example.test"))
	chunk.record(synthEventAt(3, 3, "host-a.example.test"))
	chunkBytes := chunk.bytes()
	// 2 件目の末尾の大きさの複製を書き換える。
	binary.LittleEndian.PutUint32(chunkBytes[brokenStart+brokenSize-4:], uint32(brokenSize+1))
	next := newSynthChunk()
	next.record(synthEventAt(4, 4, "host-a.example.test"))
	file := append(synthFileHeader(5, 2, 0), chunkBytes...)
	file = append(file, next.bytes()...)

	items, _ := readEVTX(t, file)
	failed := failureOf(t, items)
	if want := int64(synthFileHeaderSize + brokenStart); *failed.failure.ByteOffset != want {
		t.Errorf("failure at %d, want the broken record at %d", *failed.failure.ByteOffset, want)
	}
	if !strings.Contains(failed.failure.ObservedResult, "ends with") {
		t.Errorf("failure = %+v, want the size mismatch", failed.failure)
	}
	if got := recordIDs(items); got != "1,4" {
		t.Errorf("records = %s, want the record before the broken one and the next chunk", got)
	}
}

func TestEVTXReaderReportsARecordBeyondTheChunkHeaderCount(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	extraStart, _ := chunk.record(synthEventAt(2, 2, "host-a.example.test"))
	chunkBytes := chunk.bytes()
	// chunk の見出しの最後のレコード番号を 1 に戻す。
	binary.LittleEndian.PutUint64(chunkBytes[16:], 1)
	items, _ := readEVTX(t, append(synthFileHeader(3, 1, 0), chunkBytes...))
	failed := failureOf(t, items)
	if want := int64(synthFileHeaderSize + extraStart); *failed.failure.ByteOffset != want {
		t.Errorf("failure at %d, want the uncounted record at %d", *failed.failure.ByteOffset, want)
	}
	if got := recordIDs(items); got != "1" {
		t.Errorf("records = %s, want the counted record", got)
	}
}

func TestEVTXReaderLeavesRecordsAfterTheFreeSpace(t *testing.T) {
	chunk := newSynthChunk()
	_, firstSize := chunk.record(synthEventAt(2, 2, "host-a.example.test"))
	// chunk を前に使ったときのレコードが、空き領域の始まりの後ろに残る。
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	chunkBytes := chunk.bytes()
	binary.LittleEndian.PutUint64(chunkBytes[16:], 2)
	binary.LittleEndian.PutUint32(chunkBytes[48:], uint32(synthChunkHeaderSize+firstSize))
	items, _ := readEVTX(t, append(synthFileHeader(3, 1, 0), chunkBytes...))
	if len(items) != 1 || items[0].failure != nil || recordIDs(items) != "2" {
		t.Errorf("items = %+v, want the one counted record", items)
	}
}

// readEVTXWithin は file を読み、5 秒の中で走査が終わらないときは test を止める。
func readEVTXWithin(t *testing.T, file []byte) []evtxItem {
	t.Helper()
	done := make(chan []evtxItem, 1)
	go func() {
		var reader winevent.EVTXReader
		reader.Reset(bytes.NewReader(file))
		var items []evtxItem
		for {
			event, failure, err := reader.Next()
			if err != nil {
				break
			}
			items = append(items, evtxItem{event, failure})
		}
		done <- items
	}()
	select {
	case items := <-done:
		return items
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not return within 5 seconds")
		return nil
	}
}

// chunk の見出しの番号が uint64 の最後の 2 つでも、ライブラリの走査は数えたレコードの数で
// 終わる。数えた件数の後ろに大きさが 0 のレコードが続く。
func TestEVTXReaderEndsOnAChunkHeaderAtTheEndOfTheRecordNumbers(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	chunk.record(synthEventAt(2, 2, "host-a.example.test"))
	extraStart, _ := chunk.record(synthEventAt(3, 3, "host-a.example.test"))
	chunkBytes := chunk.bytes()
	binary.LittleEndian.PutUint64(chunkBytes[8:], math.MaxUint64-1)
	binary.LittleEndian.PutUint64(chunkBytes[16:], math.MaxUint64)
	binary.LittleEndian.PutUint32(chunkBytes[extraStart+4:], 0)
	binary.LittleEndian.PutUint64(chunkBytes[extraStart+8:], 1<<63|3)
	items := readEVTXWithin(t, append(synthFileHeader(4, 1, 0), chunkBytes...))
	failed := failureOf(t, items)
	if want := int64(synthFileHeaderSize + extraStart); *failed.failure.ByteOffset != want {
		t.Errorf("failure at %d, want the uncounted record at %d", *failed.failure.ByteOffset, want)
	}
	if got := recordIDs(items); got != "1,2" {
		t.Errorf("records = %s, want the 2 counted records", got)
	}
}

// chunk に収まらない件数を名乗る chunk の見出しは、見出しの矛盾の失敗になる。最初と最後の
// 番号の差が uint64 の最大の値でも、件数は 0 に戻らない。
func TestEVTXReaderRejectsAChunkHeaderThatCountsMoreRecordsThanFit(t *testing.T) {
	for name, last := range map[string]uint64{"the whole range": math.MaxUint64, "one past the limit": 1 << 20} {
		t.Run(name, func(t *testing.T) {
			// レコードを書き込んだ chunk にする。空き領域が見出しの直後から始まる chunk は、
			// 最後の番号がすべての bit が 1 の値のときレコードの無い chunk である。
			chunk := newSynthChunk()
			chunk.record(synthEventAt(1, 1, "host-a.example.test"))
			chunkBytes := chunk.bytes()
			binary.LittleEndian.PutUint64(chunkBytes[8:], 0)
			binary.LittleEndian.PutUint64(chunkBytes[16:], last)
			items := readEVTXWithin(t, append(synthFileHeader(2, 1, 0), chunkBytes...))
			failed := failureOf(t, items)
			if *failed.failure.ByteOffset != synthFileHeaderSize ||
				!strings.Contains(failed.failure.ObservedResult, "the chunk header names the records") {
				t.Errorf("failure = %+v, want the chunk header's record numbers", failed.failure)
			}
		})
	}
}

// chunk の見出しが実際より多い件数を名乗り、先頭のレコードの大きさが 0 の chunk では、その
// レコードの位置に失敗が 1 件出て走査が終わる。見出しの件数で回り続けないことは、件数の端と
// 件数の上限を扱う 2 つの test が確かめる。
func TestEVTXReaderEndsOnAChunkHeaderThatOvercountsRecords(t *testing.T) {
	chunk := newSynthChunk()
	start, _ := chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	chunkBytes := chunk.bytes()
	binary.LittleEndian.PutUint64(chunkBytes[16:], 1000)
	binary.LittleEndian.PutUint32(chunkBytes[start+4:], 0)
	binary.LittleEndian.PutUint64(chunkBytes[start+8:], 1<<63|1)
	file := append(synthFileHeader(2, 1, 0), chunkBytes...)
	done := make(chan []*core.ImportFailure, 1)
	go func() {
		var reader winevent.EVTXReader
		reader.Reset(bytes.NewReader(file))
		var failures []*core.ImportFailure
		for {
			_, failure, err := reader.Next()
			if err != nil {
				break
			}
			failures = append(failures, failure)
		}
		done <- failures
	}()
	select {
	case failures := <-done:
		want := int64(synthFileHeaderSize + start)
		if len(failures) != 1 || failures[0] == nil || *failures[0].ByteOffset != want {
			t.Errorf("failures = %+v, want 1 at the record of size 0 at %d", failures, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not return within 5 seconds")
	}
}

// XML の名前として読めない属性の名前を持つレコードは、原文の構造を変えずに失敗になる。
func TestEVTXReaderRejectsANameThatIsNotAnXMLName(t *testing.T) {
	event := synthEventAt(1, 1, "host-a.example.test")
	event.providerAttribute = `Name="spoofed" Guid`
	chunk := newSynthChunk()
	start, size := chunk.record(event)
	chunk.record(synthEventAt(2, 2, "host-a.example.test"))
	items, _ := readEVTX(t, append(synthFileHeader(3, 1, 0), chunk.bytes()...))
	failed := failureOf(t, items)
	if want := int64(synthFileHeaderSize + start); *failed.failure.ByteOffset != want ||
		failed.event.Source.ByteLength != int64(size) || !strings.Contains(failed.failure.ObservedResult, "XML name") {
		t.Errorf("failure = %+v at length %d, want the record at %d with the name", failed.failure,
			failed.event.Source.ByteLength, want)
	}
	if got := recordIDs(items); got != "2" {
		t.Errorf("records = %s, want the record after the failure", got)
	}
}

// 名前空間の宣言は、原文の子要素にも Event の値にもならない。
func TestEVTXReaderDropsNamespaceDeclarations(t *testing.T) {
	event := synthEventAt(1, 1, "host-a.example.test")
	event.userDataItem = "item-value"
	chunk := newSynthChunk()
	chunk.record(event)
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	if len(items) != 1 || items[0].failure != nil {
		t.Fatalf("items = %+v, want one record", items)
	}
	got := items[0].event
	if strings.Contains(got.Source.RawText, "xmlns") ||
		!strings.Contains(got.Source.RawText, "<UserData><Payload><Item>item-value</Item></Payload></UserData>") {
		t.Errorf("raw text = %s, want the UserData without the namespace declaration", got.Source.RawText)
	}
	if text, found := sectionText(got, "UserData.Payload.Item"); !found || text != "item-value" {
		t.Errorf("UserData.Payload.Item = %q, %v; want the item", text, found)
	}
	for _, section := range got.Sections {
		if strings.Contains(section.Name, "xmlns") || strings.Contains(section.Name, "ns1") {
			t.Errorf("sections carry the namespace declaration %+v", section)
		}
	}
}

func TestEVTXReaderReportsATruncatedChunk(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	file := append(synthFileHeader(2, 1, 0), chunk.bytes()[:synthChunkSize/2]...)
	items, _ := readEVTX(t, file)
	failed := failureOf(t, items)
	if *failed.failure.ByteOffset != synthFileHeaderSize || failed.event.Source.ByteLength != synthChunkSize/2 {
		t.Errorf("failure at %d length %d, want the partial chunk", *failed.failure.ByteOffset, failed.event.Source.ByteLength)
	}
}

func TestEVTXReaderSkipsUnusedChunks(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(1, 1, "host-a.example.test"))
	file := append(synthFileHeader(2, 1, 0), chunk.bytes()...)
	file = append(file, make([]byte, synthChunkSize)...)
	items, _ := readEVTX(t, file)
	if len(items) != 1 || items[0].failure != nil {
		t.Errorf("items = %+v, want the one record", items)
	}
}

// レコードが書き込まれていない chunk は、失敗にもレコードにもならない。空き領域の後ろに
// 前に使ったときのレコードが残る chunk も同じである。
func TestEVTXReaderSkipsChunksWithoutWrittenRecords(t *testing.T) {
	withLeftover := newSynthChunk()
	withLeftover.record(synthEventAt(1, 1, "host-a.example.test"))
	for name, chunkBytes := range map[string][]byte{"empty": newSynthChunk().bytes(), "leftover record": withLeftover.bytes()} {
		t.Run(name, func(t *testing.T) {
			le := binary.LittleEndian
			le.PutUint64(chunkBytes[8:], 1)
			le.PutUint64(chunkBytes[16:], math.MaxUint64)
			le.PutUint32(chunkBytes[48:], synthChunkHeaderSize)
			items, reader := readEVTX(t, append(synthFileHeader(1, 1, 0), chunkBytes...))
			if len(items) != 0 {
				t.Errorf("items = %+v, want none", items)
			}
			if got := headerText(t, reader.SourceHeader(), "SignedChunkCount"); got != "1" {
				t.Errorf("SignedChunkCount = %s, want the empty chunk counted", got)
			}
		})
	}
}

func TestEVTXReaderRejectsAFileWithoutTheHeader(t *testing.T) {
	items, reader := readEVTX(t, []byte("<Events></Events>"))
	failed := failureOf(t, items)
	if *failed.failure.ByteOffset != 0 || failed.failure.Stage != core.FailureStageTokenize {
		t.Errorf("failure = %+v, want a tokenize failure at the start", failed.failure)
	}
	if header := reader.SourceHeader(); len(header) != 0 {
		t.Errorf("the file header = %+v, want none", header)
	}
	empty, _ := readEVTX(t, nil)
	if emptyFailure := failureOf(t, empty); emptyFailure.failure.ByteOffset != nil {
		t.Errorf("the empty file's failure = %+v, want no position", emptyFailure.failure)
	}
}

func TestEVTXReaderRejectsAnUnsupportedVersion(t *testing.T) {
	header := synthFileHeader(1, 0, 0)
	binary.LittleEndian.PutUint16(header[38:], 4)
	items, reader := readEVTX(t, header)
	if failed := failureOf(t, items); !strings.Contains(failed.failure.ObservedResult, "4.1") {
		t.Errorf("failure = %+v, want the version", failed.failure)
	}
	// chunk を走査しなかった file は、署名を持つ chunk の数を 0 と名乗らない。
	for _, field := range reader.SourceHeader() {
		if field.Name == "SignedChunkCount" {
			t.Errorf("the file header carries %+v without walking the chunks", field)
		}
	}
}

func TestEVTXReaderWritesSystemTimeApartFromTheWrittenTime(t *testing.T) {
	event := synthEventAt(1, 1, "host-a.example.test")
	// 書き込み時刻と別の時点の SystemTime は、マイクロ秒までを書く。
	event.created = synthFileTime - 10*fileTimeTicksPerSecond
	chunk := newSynthChunk()
	chunk.record(event)
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	got := *items[0].event.System.SystemTime
	// ライブラリの float64 の秒は 2 マイクロ秒までずれる。原資料の文字列は 04:04:56.1234568 である。
	micros, found := strings.CutPrefix(got, "2001-02-03T04:04:56.")
	micros, ended := strings.CutSuffix(micros, "Z")
	if !found || !ended || len(micros) != 6 || micros < "123455" || micros > "123459" {
		t.Errorf("SystemTime = %q, want the microseconds within 2 of 123456.8", got)
	}
}

const fileTimeTicksPerSecond = 10_000_000

// EVTX の Sysmon のプロセスの生成は、XML と同じ意味の対応を通り、GUID の型の ProcessGuid が
// プロセスの識別子になる。全桁 0 の GUID は識別子を持たない値である。GUID である。
func TestEVTXReaderMapsASysmonProcessCreation(t *testing.T) {
	processGUID := []byte{0x78, 0x56, 0x34, 0x12, 0xcd, 0xab, 0x01, 0xef, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01}
	event := synthEventAt(1, 1, "host-a.example.test")
	event.provider, event.eventID, event.channel = "Microsoft-Windows-Sysmon", 1, "Microsoft-Windows-Sysmon/Operational"
	event.data = []synthData{
		{name: "ProcessGuid", guid: processGUID},
		{name: "ProcessId", value: "4321"},
		{name: "Image", value: `C:\Example\child.exe`},
		{name: "ParentProcessGuid", guid: make([]byte, 16)},
	}
	chunk := newSynthChunk()
	chunk.record(event)
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	observation, failure := winevent.Observe(items[0].event)
	if failure != nil {
		t.Fatalf("Observe failed: %+v", failure)
	}
	if !observation.ProcessStart {
		t.Error("the Sysmon process creation is not a process start")
	}
	fields := map[string]core.RecordField{}
	for _, field := range observation.Fields {
		fields[field.Name] = field
	}
	guid := fields["EventData.ProcessGuid"]
	if got, ok := guid.Text.ComparableValue(); guid.Semantic != core.SemanticKeyProcessId || !ok ||
		got != "{12345678-ABCD-EF01-2345-6789ABCDEF01}" {
		t.Errorf("ProcessGuid = %+v, want process.id with the GUID in braces", guid)
	}
	if parent := fields["EventData.ParentProcessGuid"]; parent.Text == nil || parent.Text.ValueState != core.ValueStateAbsent {
		t.Errorf("ParentProcessGuid = %+v, want the all-zero GUID read as absent", parent)
	}
}

// GUID の型の値は、Windows が XML に書き出す文字列と同じく中括弧で囲んだ大文字の 16 進になる。
// GUID の byte 列である。
func TestEVTXReaderWritesGUIDsInBraces(t *testing.T) {
	processGUID := []byte{0x78, 0x56, 0x34, 0x12, 0xcd, 0xab, 0x01, 0xef, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01}
	event := synthEventAt(1, 1, "host-a.example.test")
	event.data = []synthData{
		{name: "ProcessGuid", guid: processGUID},
		{name: "ParentProcessGuid", guid: make([]byte, 16)},
		{name: "Image", value: `C:\Example\child.exe`},
	}
	event.activityID = processGUID
	chunk := newSynthChunk()
	chunk.record(event)
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	if items[0].failure != nil {
		t.Fatalf("unexpected failure %+v", items[0].failure)
	}
	got := items[0].event
	const want = "{12345678-ABCD-EF01-2345-6789ABCDEF01}"
	if got.EventData[0].Text != want || got.EventData[1].Text != "{00000000-0000-0000-0000-000000000000}" {
		t.Errorf("EventData = %+v, want the GUIDs in braces", got.EventData)
	}
	if got.System.ActivityID == nil || *got.System.ActivityID != want {
		t.Errorf("ActivityID = %v, want %s", got.System.ActivityID, want)
	}
	if !strings.Contains(got.Source.RawText, `<Correlation ActivityID="`+want+`">`) {
		t.Errorf("raw text = %s, want the ActivityID attribute in braces", got.Source.RawText)
	}
}

func TestEVTXReaderReturnsTheReadFailure(t *testing.T) {
	var reader winevent.EVTXReader
	reader.Reset(io.MultiReader(bytes.NewReader([]byte("ElfFile")), errorReader{}))
	_, failure, err := reader.Next()
	if err == nil || failure == nil || failure.Stage != core.FailureStageRead || *failure.ByteOffset != 7 {
		t.Errorf("Next() = %+v, %v, want a read failure after the 7 bytes read", failure, err)
	}
	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next() after the read failure = %v, want EOF", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read error") }
