package markii_test

import (
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ヘッダーの 29 文字から Timestamp の 9 項目を組む。
//
// プロセスの起動時刻はヘッダーの時刻である。ps / start のレコードは sTime を持たない。
func TestParseProcessStartBuildsTheHeaderTimestamp(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	if raw, ok := got.StartTime.RawTextValue(); !ok || raw != processStartRawHeaderTime {
		t.Errorf("rawText = %q (present %t), want %q", raw, ok, processStartRawHeaderTime)
	}
	if normalized, ok := got.StartTime.NormalizedValue(); !ok || normalized != processStartNormalizedTime {
		t.Errorf("normalized = %q (present %t), want %q", normalized, ok, processStartNormalizedTime)
	}
	if got.StartTime.NormalizedForm != core.NormalizedFormRFC3339Absolute {
		t.Errorf("normalizedForm = %q, want %q",
			got.StartTime.NormalizedForm, core.NormalizedFormRFC3339Absolute)
	}
	if got.StartTime.Precision != core.PrecisionMillisecond {
		t.Errorf("precision = %q, want %q", got.StartTime.Precision, core.PrecisionMillisecond)
	}
	if got.StartTime.OffsetState != core.OffsetStateInValue {
		t.Errorf("offsetState = %q, want %q", got.StartTime.OffsetState, core.OffsetStateInValue)
	}
	if offset, ok := got.StartTime.OffsetTextValue(); !ok || offset != processStartOffsetText {
		t.Errorf("offsetText = %q (present %t), want %q", offset, ok, processStartOffsetText)
	}
	if got.StartTime.Clock != core.ClockTerminalLocal {
		t.Errorf("clock = %q, want %q", got.StartTime.Clock, core.ClockTerminalLocal)
	}
	if got.StartTime.Meaning != core.MeaningEvent {
		t.Errorf("meaning = %q, want %q", got.StartTime.Meaning, core.MeaningEvent)
	}
	if got.StartTime.ValueState != core.ValueStatePresent {
		t.Errorf("valueState = %q, want %q", got.StartTime.ValueState, core.ValueStatePresent)
	}
}

// 関連付けに使う時刻を導ける。UTC からのずれが文字列にあるためである。
func TestParseProcessStartDerivesTheInstant(t *testing.T) {
	got := parseProcessStartOK(t, processStartLine)

	instant, ok := got.StartTime.Instant()
	if !ok {
		t.Fatal("the header timestamp must derive an instant")
	}
	want := time.Date(2000, time.February, 1, 4, 20, 0, 500_000_000, time.UTC)
	if !instant.Equal(want) {
		t.Errorf("instant = %s, want %s", instant.UTC().Format(time.RFC3339Nano),
			want.Format(time.RFC3339Nano))
	}
}

// 日時として解釈できないヘッダーは normalize の失敗になる。既定の値で埋めない。
func TestParseProcessStartReportsAHeaderTimeItCannotRead(t *testing.T) {
	_, failure := markii.ParseProcessStart(readOneOK(t, processStartWithBadHeaderTimeLine))
	if failure == nil {
		t.Fatal("a header time that is not a date and time must be reported as a failure")
	}
	if failure.Stage != core.FailureStageNormalize {
		t.Errorf("stage = %q, want %q", failure.Stage, core.FailureStageNormalize)
	}
	if failure.DiagnosisClass != core.DiagnosisClassUndetermined {
		t.Errorf("diagnosisClass = %q, want %q",
			failure.DiagnosisClass, core.DiagnosisClassUndetermined)
	}
	if failure.LineNumber == nil {
		t.Error("the failure must carry the line number")
	}
}

// parseProcessStartOK は 1 行を文字列に分割して解析し、成功した観測を返す。
func parseProcessStartOK(t *testing.T, line string) markii.ProcessStart {
	t.Helper()
	got, failure := markii.ParseProcessStart(readOneOK(t, line))
	if failure != nil {
		t.Fatalf("parsing the process start record: %+v", *failure)
	}
	return got
}
