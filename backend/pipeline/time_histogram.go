package pipeline

import (
	"cmp"
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// TimeHistogram は、絞り込みを通ったレコードを時刻の区切りと端末で数えた結果である。
type TimeHistogram struct {
	// Start は最初の区切りの始まりである。時刻を比べられるレコードの最も早い時刻を、区切りの幅の
	// 単位で切り捨てた時刻である (histogramGrid)。
	Start time.Time
	// Step は 1 つの区切りの幅である。1 秒以上の幅は秒の単位に切り上げる。
	Step time.Duration
	// Rows は端末ごとの、区切りごとの件数である。件数の合計の降順、同じ合計では端末の
	// 識別子の昇順に並ぶ。端末を特定できないレコードの行は Terminal を持たない。
	Rows []TimeHistogramRow
	// LocalTimeRecordCount は UTC からのずれの決まらない地方時のレコードの件数、UndatedRecordCount は
	// 時刻を持たないレコードの件数である。どちらも時点を持たず、区切りに入らない。分け方は時系列と
	// 同じである (graphRecord.localWithoutInstant)。
	LocalTimeRecordCount int
	UndatedRecordCount   int
	// SpanningRecordCount は、時点を持つが、時刻の精度の範囲が 1 つの区切りに収まらず、区切りに
	// 入れなかったレコードの件数である。
	//
	// **精度の範囲の始まりの区切りに数えない。** 秒の精度のレコードの事象は、その秒の中のどの時点でも
	// ありうる。幅が 1 秒より狭い区切りの 1 つに数えると、事象の時点を区切りの幅まで狭めて見せる。
	// 時系列の区切りの規則 (精度の範囲が 1 つの区切りに収まるレコードだけを区切りへ入れる) と同じである。
	SpanningRecordCount int
}

// TimeHistogramRow は 1 つの端末の、区切りごとの件数である。
type TimeHistogramRow struct {
	Terminal *core.GraphNode
	Counts   []int
}

// TimeHistogram は、時系列と同じ絞り込みを通ったレコードを columns 個の区切りと端末で数える。
// 時刻を比べられるレコードが無いときは Rows が空である。
//
// expression は検索式である。nil は式で絞らない。式は時系列の要求の式と同じく、レコード 1 件の
// ノードの属性で判定する (recordPasses)。
func (g Graph) TimeHistogram(filter RecordFilter, expression *SearchExpression, columns int) TimeHistogram {
	filter.Validate()
	// 期間はレコードの時刻ごとに判定する。1 つのレコードが持つ複数の事象の時刻 (additionalEventTimes)
	// を、時系列の行と同じく 1 つずつ数える。
	passes := g.recordPasses(g.withResolvedFieldNames(
		GraphQuery{RecordFilter: filter.withoutPeriod(), Expression: expression}))
	judgesPeriod := filter.TimeFrom != nil || filter.TimeTo != nil
	histogram := TimeHistogram{Rows: []TimeHistogramRow{}}
	var placed []timelineRow
	var start, end time.Time
	place := func(row timelineRow) {
		if len(placed) == 0 || row.instant.Before(start) {
			start = row.instant
		}
		if len(placed) == 0 || row.instant.After(end) {
			end = row.instant
		}
		placed = append(placed, row)
	}
	for at, record := range g.records {
		if !passes(at) {
			continue
		}
		switch {
		case !record.hasInstant && !judgesPeriod:
			if record.localWithoutInstant() {
				histogram.LocalTimeRecordCount++
			} else {
				histogram.UndatedRecordCount++
			}
		case record.hasInstant && filter.instantInPeriod(record.instant):
			place(timelineRow{at: at, additional: -1, instant: record.instant})
		}
		for index, additional := range g.additionalEventTimes[at] {
			if filter.instantInPeriod(additional.instant) {
				place(timelineRow{at: at, additional: index, instant: additional.instant})
			}
		}
	}
	if len(placed) == 0 {
		return histogram
	}
	histogram.Start, histogram.Step = histogramGrid(start, end, columns)
	rowAt := make(map[string]int)
	totals := []int{}
	for _, row := range placed {
		record := g.records[row.at]
		timestamp := record.eventTime
		if row.additional >= 0 {
			timestamp = g.additionalEventTimes[row.at][row.additional].field.Timestamp
		}
		column, fits := histogramColumn(timestamp, row.instant, histogram.Start, histogram.Step, columns)
		if !fits {
			histogram.SpanningRecordCount++
			continue
		}
		index, found := rowAt[record.terminalNodeId]
		if !found {
			index = len(histogram.Rows)
			rowAt[record.terminalNodeId] = index
			histogram.Rows = append(histogram.Rows, TimeHistogramRow{
				Terminal: g.graphNodeOf(record.terminalNodeId), Counts: make([]int, columns),
			})
			totals = append(totals, 0)
		}
		histogram.Rows[index].Counts[column]++
		totals[index]++
	}
	order := make([]int, len(histogram.Rows))
	for index := range order {
		order[index] = index
	}
	idOf := func(index int) string {
		if terminal := histogram.Rows[index].Terminal; terminal != nil {
			return terminal.Id
		}
		return ""
	}
	slices.SortFunc(order, func(left, right int) int {
		return cmp.Or(cmp.Compare(totals[right], totals[left]), cmp.Compare(idOf(left), idOf(right)))
	})
	sorted := make([]TimeHistogramRow, 0, len(order))
	for _, index := range order {
		sorted = append(sorted, histogram.Rows[index])
	}
	histogram.Rows = sorted
	return histogram
}

// histogramGrid は、最初の区切りの始まりと幅を決める。
//
// **始まりを幅の単位で切り捨てる。** 区切りの境目を、幅の倍数の読みやすい時刻 (幅が 1 秒以上なら
// 秒の境) にする。秒の精度のレコードが、幅が 1 秒以上の区切りの 2 つにまたがらない。
//
// 切り捨てると、覆う範囲が前へ最大で幅 1 つ分広がる。そのため幅は、最も早い時刻から最も遅い時刻
// までを columns-1 個の区切りで覆う幅にする。切り捨てた始まりから最も遅い時刻までは、columns 個の
// 区切りに必ず収まる。区切りが 1 つのときは切り捨てない。
func histogramGrid(start, end time.Time, columns int) (time.Time, time.Duration) {
	if columns == 1 {
		return start, histogramStep(end.Sub(start), 1)
	}
	step := histogramStep(end.Sub(start), columns-1)
	return start.Truncate(step), step
}

// histogramStep は、長さ span を columns 個の区切りで覆う幅を返す。最後のレコードが最後の区切りに
// 入るように、幅を 1 ms 広げて割る。1 秒以上の幅は秒の単位に切り上げる。
func histogramStep(span time.Duration, columns int) time.Duration {
	span = span.Truncate(time.Millisecond) + time.Millisecond
	step := max(time.Millisecond, (span+time.Duration(columns)*time.Millisecond-time.Millisecond)/
		time.Duration(columns)).Truncate(time.Millisecond)
	if step >= time.Second {
		step = (step + time.Second - time.Millisecond).Truncate(time.Second)
	}
	return step
}

// histogramColumn は、時点 instant を持つ時刻 timestamp の精度の範囲が収まる区切りの位置を返す。
// fits が偽になるのは、範囲が 2 つ以上の区切りにまたがるか、最後の区切りの外に出るときと、精度の幅が
// 定まらないとき (月と年) である。
func histogramColumn(timestamp *core.Timestamp, instant, start time.Time, step time.Duration, columns int) (int, bool) {
	width, fixed := precisionWidth(timestamp)
	if !fixed {
		return 0, false
	}
	first := int(instant.Sub(start) / step)
	last := int(instant.Add(width-time.Nanosecond).Sub(start) / step)
	if first != last || last >= columns {
		return 0, false
	}
	return first, true
}

// dayWidth は日の精度の範囲の幅である。日の精度の時刻の時点は、その UTC からのずれで読んだ日の
// 始まりであり、範囲はそこから 24 時間である。
const dayWidth = 24 * time.Hour

// precisionWidth は時刻の精度の範囲の幅を返す。fixed が偽になるのは、精度を持たない時刻と、
// 幅が暦で変わる精度 (月と年) のときである。
func precisionWidth(timestamp *core.Timestamp) (time.Duration, bool) {
	if timestamp == nil {
		return 0, false
	}
	switch timestamp.Precision {
	case core.PrecisionMicrosecond:
		return time.Microsecond, true
	case core.PrecisionMillisecond:
		return time.Millisecond, true
	case core.PrecisionSecond:
		return time.Second, true
	case core.PrecisionMinute:
		return time.Minute, true
	case core.PrecisionHour:
		return time.Hour, true
	case core.PrecisionDay:
		return dayWidth, true
	default:
		return 0, false
	}
}
