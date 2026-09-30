package winevent_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func hasField(fields []core.RecordField, name string) bool {
	for _, field := range fields {
		if field.Name == name && field.Text != nil && field.Text.RawText != nil {
			return true
		}
	}
	return false
}

// EVTX の番号の欄の名前は、読み取りが組む file の見出しとレコードの項目の名前と一致する。
func TestNumberingNamesOfEVTXNameTheReadItems(t *testing.T) {
	chunk := newSynthChunk()
	chunk.record(synthEventAt(7, 1007, "host-a.example.test"))
	items, reader := readEVTX(t, append(synthFileHeader(8, 1, 1), chunk.bytes()...))
	if len(items) != 1 || items[0].failure != nil {
		t.Fatalf("read %+v, want one record", items)
	}
	names := winevent.NumberingNamesOf(winevent.FormatKeyEVTX)
	header := reader.SourceHeader()
	for _, name := range []string{names.HeaderNextRecordID, names.HeaderDirty} {
		if name == "" || !hasField(header, name) {
			t.Errorf("the file header carries no %q: %+v", name, header)
		}
	}
	if headerText(t, header, names.HeaderNextRecordID) != "8" || headerText(t, header, names.HeaderDirty) != "true" {
		t.Errorf("the file header = %+v", header)
	}
	observation, failure := winevent.Observe(items[0].event)
	if failure != nil {
		t.Fatalf("Observe() = %+v", failure)
	}
	for _, name := range []string{
		names.Channel, names.Computer, names.EventRecordID, names.RecordHeaderID, names.ProviderName, names.EventID,
	} {
		if name == "" || !hasField(observation.Fields, name) {
			t.Errorf("the record carries no %q", name)
		}
	}
	if headerText(t, observation.Fields, names.RecordHeaderID) != "7" ||
		headerText(t, observation.Fields, names.EventRecordID) != "1007" {
		t.Errorf("the record numbers = %+v", observation.Fields)
	}
}
