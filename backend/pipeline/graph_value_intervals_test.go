// in-package test: 集計の値ごとの、レコードの時刻の隣り合う差の分布を確かめる。
package pipeline

import (
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// intervalRecord は、欄 dst に value を持ち、時刻 at のレコード 1 件である。
func intervalRecord(t *testing.T, line int64, value string, at *core.Timestamp) RecordEntry {
	t.Helper()
	record := timelineRecord(line, at)
	record.Semantics = &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          []core.RecordField{syntheticField(t, "dst", "", value)},
	}
	return record
}

func utcAt(t *testing.T, text string) *core.Timestamp {
	t.Helper()
	return timelineTimestamp(t, text, core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)
}

func localAt(t *testing.T, text string) *core.Timestamp {
	t.Helper()
	return timelineTimestamp(t, text, core.NormalizedFormLocalWithoutOffset, core.OffsetStateItemAbsent)
}

func TestValueCountsCarryTheDistributionOfTheTimeGaps(t *testing.T) {
	interpreted, err := localAt(t, "2031-04-05T15:00:30").WithInterpretation(
		core.TimestampInterpretation{Offset: "+09:00", AssertionId: "as:1"})
	if err != nil {
		t.Fatal(err)
	}
	graph := graphOfRecords(t, []RecordEntry{
		// a: 時刻の順と逆に並べた 4 件と、時点を持たない 1 件。
		intervalRecord(t, 1, "a.example.test", utcAt(t, "2031-04-05T06:03:05Z")),
		intervalRecord(t, 2, "a.example.test", utcAt(t, "2031-04-05T06:02:00Z")),
		intervalRecord(t, 3, "a.example.test", utcAt(t, "2031-04-05T06:01:00Z")),
		intervalRecord(t, 4, "a.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 5, "a.example.test", localAt(t, "2031-04-05T06:04:00")),
		// b: 同じ時点の 2 件と、ちょうど 1 秒後の 1 件。
		intervalRecord(t, 6, "b.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 7, "b.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 8, "b.example.test", utcAt(t, "2031-04-05T06:00:01Z")),
		// c: 1 件だけ。d: 解釈を与えた地方時の 1 件と、時点を持つ 1 件。
		intervalRecord(t, 9, "c.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 10, "d.example.test", &interpreted),
		intervalRecord(t, 11, "d.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
	})
	counts := graph.Query(GraphQuery{CountByName: "dst", Depth: 0}).ValueCounts
	byValue := make(map[string]core.ValueCount, len(counts))
	for _, count := range counts {
		if err := count.Validate(); err != nil {
			t.Errorf("%s: %v", count.Value, err)
		}
		byValue[count.Value] = count
	}
	bins := func(pairs ...int) []int64 {
		counts := make([]int64, len(core.IntervalBoundsMilliseconds)+1)
		for at := 0; at+1 < len(pairs); at += 2 {
			counts[pairs[at]] = int64(pairs[at+1])
		}
		return counts
	}
	for value, want := range map[string]*core.EventIntervals{
		// 差は 60,000 と 60,000 と 65,000 である。3 つとも [1m, 5m) に入る。
		"a.example.test": {TimedRecordCount: 4, MinMilliseconds: 60_000, LowerQuartileMilliseconds: 60_000,
			MedianMilliseconds: 60_000, UpperQuartileMilliseconds: 60_000, MaxMilliseconds: 65_000,
			BinCounts: bins(5, 3), CoarsePrecision: true},
		// 差は 0 と 1,000 である。1,000 は [1s, 5s) に入る。
		"b.example.test": {TimedRecordCount: 3, MinMilliseconds: 0, LowerQuartileMilliseconds: 0,
			MedianMilliseconds: 0, UpperQuartileMilliseconds: 0, MaxMilliseconds: 1_000,
			BinCounts: bins(0, 1, 1, 1), CoarsePrecision: true},
		"c.example.test": nil,
		// 解釈を与えた地方時は時点を持ち、差は 30 秒である。
		"d.example.test": {TimedRecordCount: 2, MinMilliseconds: 30_000, LowerQuartileMilliseconds: 30_000,
			MedianMilliseconds: 30_000, UpperQuartileMilliseconds: 30_000, MaxMilliseconds: 30_000,
			BinCounts: bins(4, 1), CoarsePrecision: true},
	} {
		got := byValue[value].Intervals
		if (got == nil) != (want == nil) {
			t.Errorf("%s intervals = %+v, want %+v", value, got, want)
			continue
		}
		if got == nil {
			continue
		}
		if got.TimedRecordCount != want.TimedRecordCount || got.MinMilliseconds != want.MinMilliseconds ||
			got.LowerQuartileMilliseconds != want.LowerQuartileMilliseconds ||
			got.MedianMilliseconds != want.MedianMilliseconds ||
			got.UpperQuartileMilliseconds != want.UpperQuartileMilliseconds ||
			got.MaxMilliseconds != want.MaxMilliseconds || !slices.Equal(got.BinCounts, want.BinCounts) ||
			got.CoarsePrecision != want.CoarsePrecision {
			t.Errorf("%s intervals = %+v, want %+v", value, *got, *want)
		}
	}
	if byValue["a.example.test"].RecordCount != 5 {
		t.Errorf("a records = %d, want the local clock record counted but not timed", byValue["a.example.test"].RecordCount)
	}
}

// ミリ秒までの精度の時刻だけを並べた分布は、秒までの時刻を含むと示さない。
func TestValueIntervalsOfMillisecondTimesAreNotCoarse(t *testing.T) {
	millisecondAt := func(text string) *core.Timestamp {
		t.Helper()
		offset := "Z"
		timestamp, err := core.NewTimestamp(core.Timestamp{
			RawText: &text, Normalized: &text, NormalizedForm: core.NormalizedFormRFC3339Absolute,
			Precision: core.PrecisionMillisecond, OffsetState: core.OffsetStateInValue, OffsetText: &offset,
			Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
		})
		if err != nil {
			t.Fatal(err)
		}
		return &timestamp
	}
	graph := graphOfRecords(t, []RecordEntry{
		intervalRecord(t, 1, "a.example.test", millisecondAt("2031-04-05T06:00:00.100Z")),
		intervalRecord(t, 2, "a.example.test", millisecondAt("2031-04-05T06:00:00.350Z")),
	})
	counts := graph.Query(GraphQuery{CountByName: "dst", Depth: 0}).ValueCounts
	if len(counts) != 1 || counts[0].Intervals == nil {
		t.Fatalf("counts = %+v, want one value with intervals", counts)
	}
	if got := counts[0].Intervals; got.CoarsePrecision || got.MinMilliseconds != 250 {
		t.Errorf("intervals = %+v, want a 250 ms gap without the coarse precision", *got)
	}
}

// 最後の境界以上の差は、上端を持たない最後の階級に入る。
func TestValueIntervalsPutTheLongGapsInTheLastBin(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		intervalRecord(t, 1, "a.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 2, "a.example.test", utcAt(t, "2031-04-06T06:00:00Z")),
		intervalRecord(t, 3, "a.example.test", utcAt(t, "2031-04-09T06:00:00Z")),
	})
	counts := graph.Query(GraphQuery{CountByName: "dst", Depth: 0}).ValueCounts
	if len(counts) != 1 || counts[0].Intervals == nil {
		t.Fatalf("counts = %+v, want one value with intervals", counts)
	}
	bins := counts[0].Intervals.BinCounts
	if last := bins[len(bins)-1]; last != 2 {
		t.Errorf("the last bin holds %d gaps, want the one day and the three days", last)
	}
}

// 期間の絞り込みの外の根拠は、件数にも分布にも入らない。
func TestValueIntervalsFollowThePeriodFilter(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		intervalRecord(t, 1, "a.example.test", utcAt(t, "2031-04-05T06:00:00Z")),
		intervalRecord(t, 2, "a.example.test", utcAt(t, "2031-04-05T06:00:10Z")),
		intervalRecord(t, 3, "a.example.test", utcAt(t, "2031-04-05T07:00:00Z")),
	})
	from := time.Date(2031, 4, 5, 5, 59, 0, 0, time.UTC)
	to := time.Date(2031, 4, 5, 6, 1, 0, 0, time.UTC)
	counts := graph.Query(GraphQuery{
		CountByName: "dst", Depth: 0,
		RecordFilter: RecordFilter{TimeFrom: &from, TimeTo: &to, TimeUnit: time.Second},
	}).ValueCounts
	if len(counts) != 1 || counts[0].RecordCount != 2 || counts[0].Intervals == nil ||
		counts[0].Intervals.MaxMilliseconds != 10_000 {
		t.Fatalf("counts = %+v, want the two records in the period", counts)
	}
}
