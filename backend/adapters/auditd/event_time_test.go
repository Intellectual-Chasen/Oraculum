package auditd_test

import (
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 事象の時刻は epoch の状態で、ずれの文字列を持たないまま関連付けに使える。
func TestEventTimeCarriesAnInstant(t *testing.T) {
	record := readOneRecord(t, executedEventSource)
	timestamp, err := auditd.EventTime(record.Event())
	if err != nil {
		t.Fatalf("building the event time: %v", err)
	}

	if timestamp.OffsetState != core.OffsetStateEpoch {
		t.Errorf("the offset state is %q, want %q", timestamp.OffsetState, core.OffsetStateEpoch)
	}
	if timestamp.Precision != core.PrecisionMillisecond {
		t.Errorf("the precision is %q, want %q", timestamp.Precision, core.PrecisionMillisecond)
	}
	if timestamp.Clock != core.ClockTerminalLocal {
		t.Errorf("the clock is %q, want %q", timestamp.Clock, core.ClockTerminalLocal)
	}
	if timestamp.OffsetText != nil {
		t.Errorf("the timestamp carries the offset text %q, want none", *timestamp.OffsetText)
	}

	instant, ok := timestamp.Instant()
	if !ok {
		t.Fatal("the event time carries no instant, want the instant to be derived")
	}
	want := time.Unix(1000000000, 100*int64(time.Millisecond)).UTC()
	if !instant.Equal(want) {
		t.Errorf("the instant is %s, want %s",
			instant.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// 原資料の文字列は秒とミリ秒までであり、連番を含まない。
func TestEventTimeKeepsTheStampWithoutTheSerial(t *testing.T) {
	record := readOneRecord(t, executedEventSource)
	timestamp, err := auditd.EventTime(record.Event())
	if err != nil {
		t.Fatalf("building the event time: %v", err)
	}
	if timestamp.RawText == nil {
		t.Fatal("the event time carries no raw text")
	}
	if *timestamp.RawText != "1000000000.100" {
		t.Errorf("the raw text is %q, want %q", *timestamp.RawText, "1000000000.100")
	}
	if timestamp.Normalized == nil || *timestamp.Normalized != "2001-09-09T01:46:40.100Z" {
		t.Errorf("the normalized value is %v, want %q",
			timestamp.Normalized, "2001-09-09T01:46:40.100Z")
	}
}

// 時刻の順は連番の順と一致しない。
func TestEventTimeOrdersTheEventsByTheirStamp(t *testing.T) {
	records, _ := readRecords(t, executedEventSource+daemonEventSource)
	if len(records) != 2 {
		t.Fatalf("the source holds %d records, want 2", len(records))
	}
	instants := make([]time.Time, 0, len(records))
	for _, record := range records {
		timestamp, err := auditd.EventTime(record.Event())
		if err != nil {
			t.Fatalf("building the event time: %v", err)
		}
		instant, ok := timestamp.Instant()
		if !ok {
			t.Fatal("an event time carries no instant")
		}
		instants = append(instants, instant)
	}
	if !instants[0].Before(instants[1]) {
		t.Errorf("the events order as %s then %s, want the stamps to decide the order",
			instants[0].Format(time.RFC3339Nano), instants[1].Format(time.RFC3339Nano))
	}
	if records[0].Event().Serial <= records[1].Event().Serial {
		t.Fatalf("the fixture no longer holds a decreasing serial: %d then %d",
			records[0].Event().Serial, records[1].Event().Serial)
	}
}
