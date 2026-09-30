package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestParseErrorTime(t *testing.T) {
	record := errorRecordOf(t, errorFullShapeLine)
	timestamp, failure := apache.ParseErrorTime(record)
	if failure != nil {
		t.Fatalf("ParseErrorTime() returned failure %+v", failure)
	}
	if timestamp.Precision != core.PrecisionMicrosecond {
		t.Errorf("Precision = %q, want %q", timestamp.Precision, core.PrecisionMicrosecond)
	}
	if timestamp.OffsetState != core.OffsetStateItemAbsent {
		t.Errorf("OffsetState = %q, want %q", timestamp.OffsetState, core.OffsetStateItemAbsent)
	}
	if timestamp.Clock != core.ClockTerminalLocal {
		t.Errorf("Clock = %q, want %q", timestamp.Clock, core.ClockTerminalLocal)
	}
	if timestamp.Meaning != core.MeaningRecordOutput {
		t.Errorf("Meaning = %q, want %q", timestamp.Meaning, core.MeaningRecordOutput)
	}
	got, ok := timestamp.NormalizedValue()
	if !ok {
		t.Fatal("NormalizedValue() is absent")
	}
	if want := "2000-02-01T11:22:33.123456"; got != want {
		t.Errorf("NormalizedValue() = %q, want %q", got, want)
	}
	if err := timestamp.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestParseErrorTimeRejectsMalformed(t *testing.T) {
	line := `[not a time] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] AH01215: message`
	record := errorRecordOf(t, line)
	_, failure := apache.ParseErrorTime(record)
	if failure == nil {
		t.Fatal("ParseErrorTime() did not return a failure")
	}
	if failure.Stage != "normalize" {
		t.Errorf("failure.Stage = %q, want normalize", failure.Stage)
	}
}
