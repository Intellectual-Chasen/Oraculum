package core

import (
	"testing"
	"time"
)

// absoluteTimestamp は、絶対時刻として読める時刻を作る。
func absoluteTimestamp(t *testing.T, text string) Timestamp {
	t.Helper()
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parsing %q: %v", text, err)
	}
	normalized := at.Format(time.RFC3339)
	offset := text[len(text)-6:]
	built, err := NewTimestamp(Timestamp{
		RawText:        &text,
		Normalized:     &normalized,
		NormalizedForm: NormalizedFormRFC3339Absolute,
		Precision:      PrecisionSecond,
		OffsetState:    OffsetStateInValue,
		OffsetText:     &offset,
		Clock:          ClockTerminalLocal,
		Meaning:        MeaningEvent,
		ValueState:     ValueStatePresent,
	})
	if err != nil {
		t.Fatalf("building the timestamp of %q: %v", text, err)
	}
	return built
}

func instantAt(t *testing.T, text string) *time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parsing %q: %v", text, err)
	}
	return &at
}

func TestEvaluateCoverageReadsTheOverlapOfTheRequestedPeriod(t *testing.T) {
	recorded := TimeRange{
		From: absoluteTimestamp(t, "2000-02-01T13:00:00+09:00"),
		To:   absoluteTimestamp(t, "2000-02-01T14:00:00+09:00"),
	}
	for _, testCase := range []struct {
		name     string
		from, to *time.Time
		want     CoverageState
	}{
		{
			name: "期間を与えない要求",
			want: CoverageStateRangeNotRequested,
		},
		{
			name: "両端が収録範囲の内側",
			from: instantAt(t, "2000-02-01T13:10:00+09:00"),
			to:   instantAt(t, "2000-02-01T13:20:00+09:00"),
			want: CoverageStateCovered,
		},
		{
			name: "両端が収録範囲の端に並ぶ",
			from: instantAt(t, "2000-02-01T13:00:00+09:00"),
			to:   instantAt(t, "2000-02-01T14:00:00+09:00"),
			want: CoverageStateCovered,
		},
		{
			name: "下端が収録範囲より前",
			from: instantAt(t, "2000-02-01T12:00:00+09:00"),
			to:   instantAt(t, "2000-02-01T13:20:00+09:00"),
			want: CoverageStatePartiallyCovered,
		},
		{
			name: "上端が収録範囲より後",
			from: instantAt(t, "2000-02-01T13:20:00+09:00"),
			to:   instantAt(t, "2000-02-01T15:00:00+09:00"),
			want: CoverageStatePartiallyCovered,
		},
		{
			name: "要求の期間が収録範囲より前",
			from: instantAt(t, "2000-02-01T10:00:00+09:00"),
			to:   instantAt(t, "2000-02-01T11:00:00+09:00"),
			want: CoverageStateOutsideRecording,
		},
		{
			name: "要求の期間が収録範囲より後",
			from: instantAt(t, "2000-02-01T15:00:00+09:00"),
			to:   instantAt(t, "2000-02-01T16:00:00+09:00"),
			want: CoverageStateOutsideRecording,
		},
		{
			name: "下端だけを与え、収録範囲の内側",
			from: instantAt(t, "2000-02-01T13:30:00+09:00"),
			want: CoverageStateCovered,
		},
		{
			name: "下端だけを与え、収録範囲より後",
			from: instantAt(t, "2000-02-01T15:00:00+09:00"),
			want: CoverageStateOutsideRecording,
		},
		{
			name: "上端だけを与え、収録範囲より後へはみ出す",
			to:   instantAt(t, "2000-02-01T15:00:00+09:00"),
			want: CoverageStatePartiallyCovered,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := EvaluateCoverage(recorded, true, testCase.from, testCase.to, time.Second)
			if got != testCase.want {
				t.Fatalf("EvaluateCoverage returned %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestEvaluateCoverageSeparatesTheUnknownRangeFromTheAbsentOverlap(t *testing.T) {
	from := instantAt(t, "2000-02-01T13:10:00+09:00")
	to := instantAt(t, "2000-02-01T13:20:00+09:00")
	if got := EvaluateCoverage(TimeRange{}, false, from, to, time.Second); got !=
		CoverageStateRecordingRangeUnknown {
		t.Fatalf("a source without an absolute time returned %q, want %q",
			got, CoverageStateRecordingRangeUnknown)
	}
	// 収録範囲を持つと名乗りながら、両端を絶対時刻として読めない収集元。
	rawText := "10/05 13:00:00"
	normalized := "2000-02-01T13:00:00"
	relative, err := NewTimestamp(Timestamp{
		RawText:        &rawText,
		Normalized:     &normalized,
		NormalizedForm: NormalizedFormLocalWithoutOffset,
		Precision:      PrecisionSecond,
		OffsetState:    OffsetStateItemAbsent,
		Clock:          ClockTerminalLocal,
		Meaning:        MeaningEvent,
		ValueState:     ValueStatePresent,
	})
	if err != nil {
		t.Fatalf("building a timestamp without an offset: %v", err)
	}
	got := EvaluateCoverage(
		TimeRange{From: relative, To: relative}, true, from, to, time.Second)
	if got != CoverageStateUndetermined {
		t.Fatalf("a range that cannot be compared returned %q, want %q",
			got, CoverageStateUndetermined)
	}
}

// 比較の単位が割れると、行として期間に入るレコードを持つ収集元が範囲の外になる。
func TestEvaluateCoverageComparesWithTheGivenUnit(t *testing.T) {
	recorded := TimeRange{
		From: absoluteTimestamp(t, "2000-02-01T13:00:00+09:00"),
		To:   absoluteTimestamp(t, "2000-02-01T13:00:01+09:00"),
	}
	to, err := time.Parse(time.RFC3339Nano, "2000-02-01T13:00:01.900+09:00")
	if err != nil {
		t.Fatalf("parsing the upper bound: %v", err)
	}
	from := instantAt(t, "2000-02-01T13:00:00+09:00")
	if got := EvaluateCoverage(recorded, true, from, &to, time.Second); got !=
		CoverageStateCovered {
		t.Fatalf("the second unit returned %q, want %q", got, CoverageStateCovered)
	}
	if got := EvaluateCoverage(recorded, true, from, &to, time.Millisecond); got !=
		CoverageStatePartiallyCovered {
		t.Fatalf("the millisecond unit returned %q, want %q",
			got, CoverageStatePartiallyCovered)
	}
}

func TestEveryKnownCoverageStateIsAccepted(t *testing.T) {
	for _, state := range KnownCoverageStates() {
		if !state.IsKnown() {
			t.Fatalf("%q is listed and rejected by IsKnown", state)
		}
		if err := state.Validate(); err != nil {
			t.Fatalf("%q is listed and rejected by Validate: %v", state, err)
		}
	}
	for _, outside := range []CoverageState{"", "no_record", "COVERED"} {
		if outside.IsKnown() {
			t.Fatalf("%q is outside the contract and accepted", outside)
		}
	}
}

// 記録の不在と事象の不在を別の表現で示す。
func TestSourceCoverageSeparatesTheAbsentRecordingFromTheAbsentEvent(t *testing.T) {
	first := absoluteTimestamp(t, "2000-02-01T13:00:00+09:00")
	last := absoluteTimestamp(t, "2000-02-01T14:00:00+09:00")
	absentEvent := SourceCoverage{
		SourceId:           "source-a",
		SourceFileName:     "source-a.log",
		ObservedRangeFirst: &first,
		ObservedRangeLast:  &last,
		State:              CoverageStateCovered,
		MatchedRecordCount: 0,
	}
	absentRecording := SourceCoverage{
		SourceId:           "source-b",
		SourceFileName:     "source-b.log",
		State:              CoverageStateRecordingRangeUnknown,
		MatchedRecordCount: 0,
	}
	for _, coverage := range []SourceCoverage{absentEvent, absentRecording} {
		if err := coverage.Validate(); err != nil {
			t.Fatalf("%s did not validate: %v", coverage.SourceId, err)
		}
	}
	if absentEvent.State == absentRecording.State {
		t.Fatal("the absent event and the absent recording carry the same state")
	}
}

func TestSourceCoverageRejectsAHalfGivenRange(t *testing.T) {
	first := absoluteTimestamp(t, "2000-02-01T13:00:00+09:00")
	coverage := SourceCoverage{
		SourceId:           "source-a",
		SourceFileName:     "source-a.log",
		ObservedRangeFirst: &first,
		State:              CoverageStateCovered,
	}
	if err := coverage.Validate(); err == nil {
		t.Fatal("a coverage with only the first bound validated")
	}
}

// 収録範囲を持たない収集元に、重なりを読めたことを示す状態を付けない。
func TestSourceCoverageRejectsAnOverlapWithoutARange(t *testing.T) {
	coverage := SourceCoverage{
		SourceId:       "source-a",
		SourceFileName: "source-a.log",
		State:          CoverageStateCovered,
	}
	if err := coverage.Validate(); err == nil {
		t.Fatal("a coverage without a range claimed the requested period is covered")
	}
}

func TestSourceCoverageRejectsANegativeCount(t *testing.T) {
	coverage := SourceCoverage{
		SourceId:           "source-a",
		SourceFileName:     "source-a.log",
		State:              CoverageStateRangeNotRequested,
		MatchedRecordCount: -1,
	}
	if err := coverage.Validate(); err == nil {
		t.Fatal("a coverage with a negative count validated")
	}
}
