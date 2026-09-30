// in-package test: ノードの一覧の件数と時刻の範囲を、非公開の取り込み結果の組み立てで確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 時点を持たないレコードは件数に入れ、時刻の範囲には入れず、時系列と同じく地方時と時刻の無い
// レコードに分けて数える。
func TestNodeSummariesCountTheRecordsOutsideTheTimeRangeApart(t *testing.T) {
	record := func(line int64, observedAt *core.Timestamp) RecordEntry {
		entry := timelineRecord(line, observedAt)
		entry.Semantics = &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields:          []core.RecordField{syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1")},
		}
		return entry
	}
	graph := graphOfRecords(t, []RecordEntry{
		record(1, timelineTimestamp(t, "2031-04-05T06:07:09Z",
			core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)),
		record(2, timelineTimestamp(t, "2031-04-09T06:07:01",
			core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
		record(3, timelineTimestamp(t, "2031-04-09T06:07:02",
			core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
		record(4, nil),
	})
	summaries := graph.NodeSummaries(core.NodeKindTerminal, RecordFilter{}, nil)
	if len(summaries) != 1 {
		t.Fatalf("got %d terminals, want the terminal T1", len(summaries))
	}
	got := summaries[0]
	if got.RecordCount != len(graph.records) || got.LocalTimeRecordCount != 2 || got.UndatedRecordCount != 1 {
		t.Errorf("records = %d, local = %d, undated = %d; want %d, 2 and 1",
			got.RecordCount, got.LocalTimeRecordCount, got.UndatedRecordCount, len(graph.records))
	}
	if first, last := rawTimeOf(got.FirstTime), rawTimeOf(got.LastTime); first != "2031-04-05T06:07:09Z" ||
		last != first {
		t.Errorf("time range = %q .. %q, want only the record with an instant", first, last)
	}
	// 時系列も同じ 2 件を地方時の行に、1 件を時刻の無いレコードに数える。
	timeline := graph.Timeline(TimelineQuery{})
	if timeline.UndatedRecordCount != int64(got.UndatedRecordCount) ||
		len(timeline.Entries) != got.RecordCount-got.UndatedRecordCount {
		t.Errorf("the timeline lists %d entries and %d undated records, want the split of the summary",
			len(timeline.Entries), timeline.UndatedRecordCount)
	}
}

// rawTimeOf は時刻の原資料の文字列を返す。時刻が無いときは空の文字列を返す。
func rawTimeOf(time *core.Timestamp) string {
	if time == nil || time.RawText == nil {
		return ""
	}
	return *time.RawText
}

// 端末 1 台に置いた markii 形式の 5 行は、端末の根拠と端末に接するエッジの根拠が重なっても
// 5 件と数える。時刻の範囲は最も早い行と最も遅い行である。
func TestNodeSummariesCountEachRecordOnce(t *testing.T) {
	summaries := eventKindsGraph(t).NodeSummaries(core.NodeKindTerminal, RecordFilter{}, nil)

	if len(summaries) != 1 {
		t.Fatalf("got %d terminals, want 1", len(summaries))
	}
	got := summaries[0]
	if got.RecordCount != 5 {
		t.Errorf("recordCount = %d, want 5", got.RecordCount)
	}
	if first, last := rawTimeOf(got.FirstTime), rawTimeOf(got.LastTime); first != "02/01/2000 03:04:01.000 +0900" ||
		last != "02/01/2000 03:04:05.000 +0900" {
		t.Errorf("time range = %q .. %q", first, last)
	}
}

// IP アドレスは件数の降順、同じ件数では識別子の昇順に並ぶ。絞り込みを通ったレコードが
// 無いノードは含めない。
func TestNodeSummariesOrderByCountAndNarrowByTheFilter(t *testing.T) {
	graph := eventKindsGraph(t)
	whole := graph.NodeSummaries(core.NodeKindIp, RecordFilter{}, nil)

	// squid の 2 行が指す接続元と、markii 形式の接続の 1 行が指す 2 つのアドレス。
	counts := make([]int, 0, len(whole))
	for _, summary := range whole {
		counts = append(counts, summary.RecordCount)
	}
	if !slices.Equal(counts, []int{2, 1, 1}) {
		t.Fatalf("counts = %v, want [2 1 1]", counts)
	}
	if whole[1].Node.Id >= whole[2].Node.Id {
		t.Errorf("ties are ordered %s, %s; want ascending ids", whole[1].Node.Id, whole[2].Node.Id)
	}
	if rawTimeOf(whole[0].FirstTime) != "[10/Oct/2000:13:55:36 +0000]" ||
		rawTimeOf(whole[0].LastTime) != "[10/Oct/2000:13:55:37 +0000]" {
		t.Errorf("the squid client ranges %q .. %q",
			rawTimeOf(whole[0].FirstTime), rawTimeOf(whole[0].LastTime))
	}

	// 事象の分類で絞ると、squid の行だけが接する接続元は外れる。
	byCategory := graph.NodeSummaries(core.NodeKindIp, RecordFilter{EventCategory: "net"}, nil)
	if len(byCategory) != 2 || byCategory[0].Node.Id != whole[1].Node.Id ||
		byCategory[1].Node.Id != whole[2].Node.Id {
		t.Errorf("narrowed by category = %d nodes, want the two addresses of the connection", len(byCategory))
	}

	// 検索式も、レコード 1 件ごとにそのレコードの欄で絞る。事象の分類の欄で書いた式は、
	// 事象の分類の絞り込みと同じアドレスを残す。
	expression, err := ParseSearchExpression("event.category == net")
	if err != nil {
		t.Fatal(err)
	}
	byExpression := graph.NodeSummaries(core.NodeKindIp, RecordFilter{}, expression)
	expressedIds := make([]string, 0, len(byExpression))
	for _, summary := range byExpression {
		expressedIds = append(expressedIds, summary.Node.Id)
	}
	categoryIds := make([]string, 0, len(byCategory))
	for _, summary := range byCategory {
		categoryIds = append(categoryIds, summary.Node.Id)
	}
	if !slices.Equal(expressedIds, categoryIds) {
		t.Errorf("narrowed by the expression = %v, want the addresses narrowed by the category %v",
			expressedIds, categoryIds)
	}

	// 収集元で絞ると、その収集元の行が接するアドレスだけが残る。
	var squidSource string
	for _, recording := range graph.recordings {
		if recording.fileName == "access.log" {
			squidSource = recording.sourceId
		}
	}
	bySource := graph.NodeSummaries(core.NodeKindIp, RecordFilter{Sources: []string{squidSource}}, nil)
	if len(bySource) != 1 || bySource[0].Node.Id != whole[0].Node.Id || bySource[0].RecordCount != 2 {
		t.Errorf("narrowed by source = %+v, want only the squid client with 2 records", bySource)
	}
}
