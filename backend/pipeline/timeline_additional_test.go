package pipeline

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// runHistoryGraph は、ObservedAt のほかに 2 つの事象の時刻を持つレコードと、時刻を 1 つだけ持つ
// レコードのグラフを組む。
func runHistoryGraph(t *testing.T) Graph {
	t.Helper()
	timeField := func(name string, second int) core.RecordField {
		field, err := core.NewTimestampField(name, "", *matchTestTime(t, second))
		if err != nil {
			t.Fatal(err)
		}
		return field
	}
	earlier, later := timeField("Run#2", 5), timeField("Run#3", 30)
	main, err := core.NewTimestampField("Run", core.SemanticKeyEventTime, *matchTestTime(t, 10))
	if err != nil {
		t.Fatal(err)
	}
	lineOne, lineTwo := int64(1), int64(2)
	return graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &lineOne,
		},
		ObservedAt: matchTestTime(t, 10),
		Semantics: &RecordSemantics{
			ObservationKind:      core.ObservationKind{Raw: []core.RecordField{}},
			Fields:               []core.RecordField{main, earlier, later},
			AdditionalEventTimes: []core.RecordField{earlier, later},
		},
	}, {
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &lineTwo,
		},
		ObservedAt: matchTestTime(t, 20),
		Semantics:  &RecordSemantics{ObservationKind: core.ObservationKind{Raw: []core.RecordField{}}},
	}})
}

// timelineRowsOf は行を「行番号 時刻の欄の名前 秒」の並びへ直す。
func timelineRowsOf(t *testing.T, timeline Timeline) []string {
	t.Helper()
	rows := make([]string, 0, len(timeline.Entries))
	for _, entry := range timeline.Entries {
		if err := entry.Validate(); err != nil {
			t.Fatalf("the row %+v is invalid: %v", entry, err)
		}
		at, _ := entry.EventTime.Instant()
		rows = append(rows, strconv.FormatInt(*entry.RecordRef.LineNumber, 10)+" "+entry.TimeFieldName+" "+at.Format("05"))
	}
	return rows
}

// ほかの事象の時刻は、その時刻の位置に同じレコードの行を足し、期間は行ごとに判定する。
func TestTimelineListsEveryEventTimeOfARecord(t *testing.T) {
	graph := runHistoryGraph(t)
	zone := time.FixedZone("", 9*60*60)
	at := func(second int) *time.Time {
		instant := time.Date(2024, 3, 14, 10, 20, second, 0, zone)
		return &instant
	}
	for _, tc := range []struct {
		name     string
		filter   RecordFilter
		want     []string
		recorded int64
	}{
		// ほかの事象の時刻を持つレコードは、ObservedAt の行にも時刻の欄の名前を持つ。
		{"whole", RecordFilter{}, []string{"1 Run#2 05", "1 Run 10", "2  20", "1 Run#3 30"}, 2},
		{"the ObservedAt and an earlier run", RecordFilter{TimeFrom: at(4), TimeTo: at(12)},
			[]string{"1 Run#2 05", "1 Run 10"}, 1},
		{"a later run alone", RecordFilter{TimeFrom: at(25), TimeTo: at(35)}, []string{"1 Run#3 30"}, 1},
	} {
		query := TimelineQuery{RecordFilter: tc.filter}
		query.Validate()
		timeline := graph.Timeline(query)
		if got := timelineRowsOf(t, timeline); !slices.Equal(got, tc.want) {
			t.Errorf("%s: rows = %v, want %v", tc.name, got, tc.want)
		}
		coverage := timeline.SourceCoverages[0]
		if got := coverage.MatchedRecordCount; got != tc.recorded {
			t.Errorf("%s: matched records = %d, want %d", tc.name, got, tc.recorded)
		}
		if got := coverage.MatchedRowCount; got != int64(len(tc.want)) {
			t.Errorf("%s: matched rows = %d, want %d", tc.name, got, len(tc.want))
		}
		// 収録範囲の両端はほかの事象の時刻を含む。
		first, _ := coverage.ObservedRangeFirst.Instant()
		last, _ := coverage.ObservedRangeLast.Instant()
		if first.Second() != 5 || last.Second() != 30 {
			t.Errorf("%s: the coverage is %v to %v, want the seconds 05 to 30", tc.name, first, last)
		}
		histogram := graph.TimeHistogram(tc.filter, nil, 1)
		// 区切りの幅に精度の範囲が収まらない時刻は SpanningRecordCount に数える。
		total := histogram.SpanningRecordCount
		for _, row := range histogram.Rows {
			total += row.Counts[0]
		}
		if total != len(tc.want) {
			t.Errorf("%s: the histogram counts %d, want the %d rows of the timeline", tc.name, total, len(tc.want))
		}
	}
}
