package winevent_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
)

// 名前の変更の Event。名前は example.test 系である。
const renameDocument = `<Events>
<Event xmlns="urn:example:events"><System><Provider Name="EventLog"></Provider>
<EventID Qualifiers="32768">6011</EventID>
<TimeCreated SystemTime="2001-02-03T04:05:06.0000000Z"></TimeCreated>
<EventRecordID>201</EventRecordID>
<Channel>System</Channel>
<Computer>new-host.example.test</Computer>
</System>
<EventData><Data>OLD-HOST</Data>
<Data>NEW-HOST</Data>
<Data></Data>
</EventData>
</Event>
<Event xmlns="urn:example:events"><System><Provider Name="EventLog"></Provider>
<EventID>6013</EventID>
<Channel>System</Channel>
<Computer>new-host.example.test</Computer>
</System>
<EventData><Data>OLD-HOST</Data>
<Data>NEW-HOST</Data>
</EventData>
</Event>
<Event xmlns="urn:example:events"><System><Provider Name="Example-Provider"></Provider>
<EventID>6011</EventID>
<Channel>System</Channel>
</System>
<EventData><Data>OLD-HOST</Data>
<Data>NEW-HOST</Data>
</EventData>
</Event>
<Event xmlns="urn:example:events"><System><Provider Name="EventLog"></Provider>
<EventID>6011</EventID>
<Channel>System</Channel>
</System>
<EventData><Data Name="Old">OLD-HOST</Data>
<Data>NEW-HOST</Data>
</EventData>
</Event>
</Events>
`

func TestComputerRenameOfReadsTheNamesBeforeAndAfter(t *testing.T) {
	events, failures := readAll(t, renameDocument)
	if len(failures) != 0 || len(events) != 4 {
		t.Fatalf("read %d events and %d failures", len(events), len(failures))
	}
	previous, current, ok := winevent.ComputerRenameOf(events[0])
	if !ok || previous != "OLD-HOST" || current != "NEW-HOST" {
		t.Errorf("the rename is (%q, %q, %v)", previous, current, ok)
	}
	for index, name := range map[int]string{1: "another event", 2: "another provider", 3: "one unnamed value"} {
		if _, _, ok := winevent.ComputerRenameOf(events[index]); ok {
			t.Errorf("%s read as a rename", name)
		}
	}
}

// EVTX の 6011 は、変更の前と後の名前を文字列の配列の型の値 1 つとして、Name の無い `<Data>`
// 1 つに差し込む。
func TestComputerRenameOfReadsTheStringArrayOfEVTX(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEvent{
		headerID: 1, recordID: 61, writtenAt: synthFileTime, created: synthFileTime,
		provider: "EventLog", eventID: 6011, keywords: 0x80000000000000,
		channel: "System", computer: "NEW-HOST",
		data: []synthData{{list: []string{"OLD HOST", "NEW-HOST"}}},
	})
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	if len(items) != 1 || items[0].failure != nil {
		t.Fatalf("read %+v", items)
	}
	previous, current, ok := winevent.ComputerRenameOf(items[0].event)
	if !ok || previous != "OLD HOST" || current != "NEW-HOST" {
		t.Errorf("the rename is (%q, %q, %v); raw text %q", previous, current, ok, items[0].event.Source.RawText)
	}
}

// 要素の無い文字列の配列を持つ名前付きの Data も、空の Data として残る。
func TestEVTXKeepsTheNamedDataOfAnEmptyStringArray(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEvent{
		headerID: 1, recordID: 62, writtenAt: synthFileTime, created: synthFileTime,
		provider: "EventLog", eventID: 6011, keywords: 0x80000000000000,
		channel: "System", computer: "NEW-HOST",
		data: []synthData{{name: "Names", list: []string{}}},
	})
	items, _ := readEVTX(t, append(synthFileHeader(2, 1, 0), chunk.bytes()...))
	if len(items) != 1 || items[0].failure != nil {
		t.Fatalf("read %+v", items)
	}
	raw := items[0].event.Source.RawText
	if !strings.Contains(raw, `<Data Name="Names">`) {
		t.Errorf("the raw text %q drops the empty Data", raw)
	}
}
