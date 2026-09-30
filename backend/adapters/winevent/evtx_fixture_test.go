package winevent_test

import (
	"bytes"
	"os"
	"testing"
)

// processCreationFixture は、他の package の test が EVTX の取り込みに使う file である。
// 中身は synthesizedProcessCreationEVTX が組む byte 列と同じである。
const processCreationFixture = "../../internal/testdata/winevent/process-creation.evtx"

// synthesizedProcessCreationEVTX は、Security の 4688 を 2 件持つ EVTX を組む。
func synthesizedProcessCreationEVTX() []byte {
	chunk := newSynthChunk()
	for at, name := range []string{`C:\Example\a-parent.exe`, `C:\Example\b-child.exe`} {
		event := synthEventAt(uint64(at+1), uint64(at+71), "host-e.example.test")
		event.data = []synthData{{name: "NewProcessId", value: "0x2b"}, {name: "NewProcessName", value: name},
			{name: "ParentProcessName", value: `C:\Example\shell.exe`}}
		chunk.record(event)
	}
	return append(synthFileHeader(3, 1, 0), chunk.bytes()...)
}

// ORACULUM_WRITE_FIXTURES=1 で実行すると、fixture を組み直して書き出す。
func TestProcessCreationFixtureMatchesTheSynthesizer(t *testing.T) {
	want := synthesizedProcessCreationEVTX()
	if os.Getenv("ORACULUM_WRITE_FIXTURES") == "1" {
		if err := os.WriteFile(processCreationFixture, want, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(processCreationFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the fixture differs from the synthesizer; rerun with ORACULUM_WRITE_FIXTURES=1")
	}
	items, _ := readEVTX(t, got)
	if len(items) != 2 {
		t.Fatalf("the fixture holds %d records, want 2", len(items))
	}
}
