// in-package test: 件数の分布の区切りの決め方と、区切りに入れないレコードの数え方を確かめる。
package pipeline

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func sumOf(counts []int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

// columnOf は、時刻 at が入る区切りの位置を返す。
func columnOf(histogram TimeHistogram, at time.Time) int {
	return int(at.Sub(histogram.Start) / histogram.Step)
}

// 端末に置いた 5 行と、端末を特定できない squid の 2 行を、合計の多い順の 2 行で数える。
// 最も早い行と最も遅い行は、それぞれの時刻を含む区切りに入る。
func TestTimeHistogramCountsByColumnAndTerminal(t *testing.T) {
	histogram := eventKindsGraph(t).TimeHistogram(RecordFilter{}, nil, 60)

	if len(histogram.Rows) != 2 || histogram.LocalTimeRecordCount != 0 || histogram.UndatedRecordCount != 0 ||
		histogram.SpanningRecordCount != 0 {
		t.Fatalf("histogram = %+v; want 2 rows and every record in a column", histogram)
	}
	earliest := time.Date(2000, 1, 31, 18, 4, 1, 0, time.UTC)
	latest := time.Date(2000, 10, 10, 13, 55, 37, 0, time.UTC)
	terminal, squid := histogram.Rows[0], histogram.Rows[1]
	if terminal.Terminal == nil || sumOf(terminal.Counts) != 5 || terminal.Counts[columnOf(histogram, earliest)] != 5 {
		t.Errorf("the terminal row = %+v, want 5 records in the column of the earliest record", terminal)
	}
	if squid.Terminal != nil || sumOf(squid.Counts) != 2 || squid.Counts[columnOf(histogram, latest)] != 2 {
		t.Errorf("the squid row counts %v, want 2 records in the column of the latest record", squid.Counts)
	}
	// 始まりは最も早い行の時刻を幅の単位で切り捨てた時刻である。
	if !histogram.Start.Equal(earliest.Truncate(histogram.Step)) {
		t.Errorf("start = %s, want %s truncated to the step %s", histogram.Start, earliest, histogram.Step)
	}
	if histogram.Step%time.Second != 0 || !latest.Before(histogram.Start.Add(histogram.Step*60)) {
		t.Errorf("step = %s does not cover the span in 60 whole-second columns", histogram.Step)
	}
}

// 1 秒に満たない範囲はミリ秒の幅で割る。絞り込みを通ったレコードが無いときは行が無い。
func TestTimeHistogramNarrowsAndKeepsMillisecondSteps(t *testing.T) {
	graph := eventKindsGraph(t)
	netOnly := graph.TimeHistogram(RecordFilter{EventCategory: "net"}, nil, 60)
	if len(netOnly.Rows) != 1 || netOnly.Step != time.Millisecond || sumOf(netOnly.Rows[0].Counts) != 1 {
		t.Errorf("net only = step %s, rows %+v", netOnly.Step, netOnly.Rows)
	}
	// 事象の分類の欄で書いた検索式は、事象の分類の絞り込みと同じ件数の分布を返す。
	expression, err := ParseSearchExpression("event.category == net")
	if err != nil {
		t.Fatal(err)
	}
	expressed := graph.TimeHistogram(RecordFilter{}, expression, 60)
	if len(expressed.Rows) != len(netOnly.Rows) || expressed.Step != netOnly.Step ||
		!expressed.Start.Equal(netOnly.Start) || !slices.Equal(expressed.Rows[0].Counts, netOnly.Rows[0].Counts) {
		t.Errorf("narrowed by the expression = %+v, want the histogram narrowed by the category %+v",
			expressed, netOnly)
	}
	none := graph.TimeHistogram(RecordFilter{EventCategory: "no_such_category"}, nil, 60)
	if len(none.Rows) != 0 || none.LocalTimeRecordCount != 0 || none.UndatedRecordCount != 0 || none.Step != 0 {
		t.Errorf("no record = %+v, want no rows and no step", none)
	}
}

// histogramTimestamp は精度を与えた、UTC からのずれを持つ時刻を返す。
func histogramTimestamp(t *testing.T, normalized string, precision core.Precision) *core.Timestamp {
	t.Helper()
	offset := "Z"
	value, err := core.NewTimestamp(core.Timestamp{
		RawText: &normalized, Normalized: &normalized, NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision: precision, OffsetState: core.OffsetStateInValue, OffsetText: &offset,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &value
}

// 時点を持たないレコードは、時系列と同じく地方時と時刻の無いレコードに分けて数え、区切りに入れない。
// 時刻の精度の範囲が 1 つの区切りに収まらないレコードも、区切りに入れずに別に数える。
func TestTimeHistogramCountsTheRecordsOutsideTheColumnsApart(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		timelineRecord(1, histogramTimestamp(t, "2031-04-05T06:07:00.000Z", core.PrecisionMillisecond)),
		timelineRecord(2, histogramTimestamp(t, "2031-04-05T06:07:00.900Z", core.PrecisionMillisecond)),
		// 秒の精度の 1 件は 06:07:00 からの 1 秒のどこでもありうる。区切りは 1 秒より狭い。
		timelineRecord(3, histogramTimestamp(t, "2031-04-05T06:07:00Z", core.PrecisionSecond)),
		timelineRecord(4, timelineTimestamp(t, "2031-04-05T15:07:00",
			core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
		timelineRecord(5, nil),
	})
	histogram := graph.TimeHistogram(RecordFilter{}, nil, 10)
	if histogram.LocalTimeRecordCount != 1 || histogram.UndatedRecordCount != 1 {
		t.Errorf("local = %d, undated = %d; want 1 and 1",
			histogram.LocalTimeRecordCount, histogram.UndatedRecordCount)
	}
	if histogram.Step >= time.Second || histogram.SpanningRecordCount != 1 {
		t.Errorf("step = %s, spanning = %d; want a step under a second and the second-precision record apart",
			histogram.Step, histogram.SpanningRecordCount)
	}
	placed := 0
	for _, row := range histogram.Rows {
		placed += sumOf(row.Counts)
	}
	if placed != 2 {
		t.Errorf("placed %d records in the columns, want the 2 millisecond-precision records", placed)
	}
	// 区切りに入れた件数と、入れなかった 3 つの件数の和は、絞り込みを通ったレコードの件数である。
	if total := placed + histogram.SpanningRecordCount + histogram.LocalTimeRecordCount +
		histogram.UndatedRecordCount; total != len(graph.records) {
		t.Errorf("the counts add up to %d, want the %d records", total, len(graph.records))
	}
}

// 時点を持つレコードがどれも区切りに収まらないときは、行を返さずに、区切りの始まりと幅を返す。
// 同じ秒の秒の精度の 2 件は範囲の幅が 0 のため区切りの幅が 1 秒より狭くなり、どの区切りにも収まらない。
func TestTimeHistogramKeepsTheColumnsWhenNoRecordFitsInOne(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		timelineRecord(1, histogramTimestamp(t, "2031-04-05T06:07:00Z", core.PrecisionSecond)),
		timelineRecord(2, histogramTimestamp(t, "2031-04-05T06:07:00Z", core.PrecisionSecond)),
	})
	histogram := graph.TimeHistogram(RecordFilter{}, nil, 10)
	if len(histogram.Rows) != 0 || histogram.SpanningRecordCount != 2 {
		t.Errorf("rows = %+v, spanning = %d; want no rows and the 2 records apart",
			histogram.Rows, histogram.SpanningRecordCount)
	}
	earliest := time.Date(2031, 4, 5, 6, 7, 0, 0, time.UTC)
	if histogram.Step <= 0 || histogram.Step >= time.Second || !histogram.Start.Equal(earliest) {
		t.Errorf("start = %s, step = %s; want columns under a second from %s", histogram.Start, histogram.Step, earliest)
	}
}

// 区切りの始まりを幅の単位に揃え、最も早い時刻から最も遅い時刻までを columns 個の区切りで覆う。
// 区切りが 1 つのときは、揃えずに最も早い時刻から覆う。
func TestHistogramGridAlignsTheStartAndCoversTheRecords(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	spans := []int64{1_000, 60_000, 86_400_000, 30 * 86_400_000}
	for attempt := range 20_000 {
		start := time.UnixMilli(random.Int64N(4_000_000_000_000)).UTC()
		end := start.Add(time.Duration(random.Int64N(spans[attempt%len(spans)])) * time.Millisecond)
		columns := 1 + random.IntN(1000)
		if attempt%100 == 0 {
			columns = 1
		}
		gridStart, step := histogramGrid(start, end, columns)
		if gridStart.After(start) || !end.Before(gridStart.Add(step*time.Duration(columns))) {
			t.Fatalf("grid %s + %d x %s does not cover %s .. %s", gridStart, columns, step, start, end)
		}
		aligned := gridStart.Equal(gridStart.Truncate(step))
		if columns > 1 && !aligned {
			t.Fatalf("grid %s with the step %s over %d columns is not aligned", gridStart, step, columns)
		}
		if columns == 1 && !gridStart.Equal(start) {
			t.Fatalf("a single column starts at %s, want the earliest time %s", gridStart, start)
		}
	}
}

// 時刻の精度の範囲が 1 つの区切りに収まるレコードだけを区切りに入れる。幅が暦で変わる精度 (月と年) と
// 精度を持たない時刻は、どの幅の区切りにも入れない。
func TestHistogramColumnPlacesOnlyTheRecordsWithinOneColumn(t *testing.T) {
	start := time.Date(2031, 4, 5, 0, 0, 0, 0, time.UTC)
	record := func(at time.Time, precision core.Precision) graphRecord {
		return graphRecord{instant: at, hasInstant: true, eventTime: &core.Timestamp{Precision: precision}}
	}
	for _, testCase := range []struct {
		name    string
		record  graphRecord
		step    time.Duration
		columns int
		column  int
		fits    bool
	}{
		{"a minute in a minute column", record(start.Add(3*time.Minute), core.PrecisionMinute),
			time.Minute, 10, 3, true},
		{"a minute over two 30-second columns", record(start.Add(3*time.Minute), core.PrecisionMinute),
			30 * time.Second, 10, 0, false},
		{"an hour in an hour column", record(start.Add(2*time.Hour), core.PrecisionHour),
			time.Hour, 10, 2, true},
		{"an hour over two 30-minute columns", record(start.Add(2*time.Hour), core.PrecisionHour),
			30 * time.Minute, 10, 0, false},
		{"a day in a day column", record(start.Add(24*time.Hour), core.PrecisionDay),
			24 * time.Hour, 10, 1, true},
		{"a day over 24 hour columns", record(start.Add(24*time.Hour), core.PrecisionDay),
			time.Hour, 100, 0, false},
		{"a month in any column", record(start, core.PrecisionMonth), 366 * 24 * time.Hour, 10, 0, false},
		{"a year in any column", record(start, core.PrecisionYear), 366 * 24 * time.Hour, 10, 0, false},
		{"a time without a precision", graphRecord{instant: start, hasInstant: true}, time.Hour, 10, 0, false},
		{"a second in the last column", record(start.Add(9*time.Second), core.PrecisionSecond),
			time.Second, 10, 9, true},
		{"a second after the last column", record(start.Add(10*time.Second), core.PrecisionSecond),
			time.Second, 10, 0, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			column, fits := histogramColumn(testCase.record.eventTime, testCase.record.instant, start, testCase.step, testCase.columns)
			if fits != testCase.fits || (fits && column != testCase.column) {
				t.Errorf("histogramColumn = %d, %v; want %d, %v", column, fits, testCase.column, testCase.fits)
			}
		})
	}
}
