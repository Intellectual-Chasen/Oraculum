package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestParseAccessTime(t *testing.T) {
	record := accessRecordOf(t, fullShapeLine)
	timestamp, failure := apache.ParseAccessTime(record)
	if failure != nil {
		t.Fatalf("ParseAccessTime() returned failure %+v", failure)
	}
	if timestamp.Precision != core.PrecisionSecond {
		t.Errorf("Precision = %q, want %q", timestamp.Precision, core.PrecisionSecond)
	}
	if timestamp.OffsetState != core.OffsetStateInValue {
		t.Errorf("OffsetState = %q, want %q", timestamp.OffsetState, core.OffsetStateInValue)
	}
	if timestamp.Clock != core.ClockTerminalLocal {
		t.Errorf("Clock = %q, want %q", timestamp.Clock, core.ClockTerminalLocal)
	}
	if timestamp.Meaning != core.MeaningEvent {
		t.Errorf("Meaning = %q, want %q", timestamp.Meaning, core.MeaningEvent)
	}
	got, ok := timestamp.NormalizedValue()
	if !ok {
		t.Fatal("NormalizedValue() is absent")
	}
	if want := "2000-02-01T11:20:10+09:00"; got != want {
		t.Errorf("NormalizedValue() = %q, want %q", got, want)
	}
	if err := timestamp.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestParseAccessTimeRejectsMalformed(t *testing.T) {
	line := `203.0.113.10 - - [not-a-time] "GET / HTTP/1.1" 200 12 "-" "-"`
	record := accessRecordOf(t, line)
	_, failure := apache.ParseAccessTime(record)
	if failure == nil {
		t.Fatal("ParseAccessTime() did not return a failure")
	}
	if failure.Stage != "normalize" {
		t.Errorf("failure.Stage = %q, want normalize", failure.Stage)
	}
}
