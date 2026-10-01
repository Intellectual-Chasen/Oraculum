package pipeline

import (
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// timelineKeys は行を、収集元と原資料の位置で指した並びへ直す。
func timelineKeys(timeline Timeline) []string {
	keys := make([]string, 0, len(timeline.Entries))
	for _, entry := range timeline.Entries {
		keys = append(keys, entry.RecordRef.SourceId+" "+entry.RecordRef.RecordRawTextRef)
	}
	return keys
}

// 同じ取り込みへの同じ要求は、同じ並びを返す。
func TestTimelineReturnsTheSameOrderForTheSameRequest(t *testing.T) {
	graph := graphOf(t)
	first := timelineKeys(graph.Timeline(TimelineQuery{}))
	if len(first) == 0 {
		t.Fatal("the timeline carries no entry")
	}
	for attempt := range 4 {
		again := timelineKeys(graph.Timeline(TimelineQuery{}))
		if len(again) != len(first) {
			t.Fatalf("attempt %d returned %d entries, want %d",
				attempt, len(again), len(first))
		}
		for index := range first {
			if again[index] != first[index] {
				t.Fatalf("attempt %d put %q at %d, want %q",
					attempt, again[index], index, first[index])
			}
		}
	}
}

// 時刻の昇順に並び、同じ時刻の行は収集元と位置で先後が決まる。
func TestTimelineOrdersTheEntriesByTheInstant(t *testing.T) {
	timeline := graphOf(t).Timeline(TimelineQuery{})
	if len(timeline.Entries) < 2 {
		t.Fatalf("the timeline carries %d entries, want more than one",
			len(timeline.Entries))
	}
	for index := 1; index < len(timeline.Entries); index++ {
		earlier, earlierOk := timeline.Entries[index-1].EventTime.Instant()
		later, laterOk := timeline.Entries[index].EventTime.Instant()
		if !earlierOk || !laterOk {
			t.Fatalf("entries[%d] carries a time that cannot be compared", index)
		}
		if later.Before(earlier) {
			t.Fatalf("entries[%d] at %s comes after entries[%d] at %s",
				index, later, index-1, earlier)
		}
		if !later.Equal(earlier) {
			continue
		}
		left := timeline.Entries[index-1].RecordRef
		right := timeline.Entries[index].RecordRef
		if left.SourceId > right.SourceId {
			t.Fatalf("two entries at %s are ordered %q before %q",
				later, left.SourceId, right.SourceId)
		}
	}
}

// timelineRecord は時系列の検査に使うレコード 1 件である。
func timelineRecord(line int64, observedAt *core.Timestamp) RecordEntry {
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		ObservedAt: observedAt,
	}
}

// timelineTimestamp は正規化値の書式とずれの状態を与えた時刻を返す。
func timelineTimestamp(
	t *testing.T, normalized string, form core.NormalizedForm, offsetState core.OffsetState,
) *core.Timestamp {
	t.Helper()
	value := core.Timestamp{
		RawText: &normalized, Normalized: &normalized, NormalizedForm: form,
		Precision: core.PrecisionSecond, OffsetState: offsetState,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	}
	if offsetState == core.OffsetStateInValue {
		offset := "Z"
		value.OffsetText = &offset
	}
	timestamp, err := core.NewTimestamp(value)
	if err != nil {
		t.Fatal(err)
	}
	return &timestamp
}

// 時点を持つ行、ずれの決まらない地方時の行の順に並べ、時刻も地方時の文字列も持たないレコードを
// 列へ混ぜずに件数で数える。
func TestTimelineCountsTheUndatedRecordsOutsideTheList(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		timelineRecord(1, timelineTimestamp(t, "2031-04-05T06:07:01",
			core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
		timelineRecord(2, nil),
		timelineRecord(3, timelineTimestamp(t, "2031-04-05T06:07:09Z",
			core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)),
	})
	timeline := graph.Timeline(TimelineQuery{})
	if timeline.UndatedRecordCount != 1 {
		t.Fatalf("undatedRecordCount=%d, want the record without a time alone", timeline.UndatedRecordCount)
	}
	lines := make([]int64, 0, len(timeline.Entries))
	for _, entry := range timeline.Entries {
		lines = append(lines, *entry.RecordRef.LineNumber)
	}
	if !slices.Equal(lines, []int64{3, 1}) {
		t.Fatalf("the timeline lists the lines %v, want the dated line 3 before the local line 1", lines)
	}
}

// **期間で絞ると、時点を持たないレコードは期間の判定から外れ、外れた件数を理由ごとに数える。**
// ずれの決まらない地方時のレコードと、時刻を持たないレコードを分けて数える。
func TestTimelineCountsTheRecordsThePeriodCannotJudge(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		timelineRecord(1, timelineTimestamp(t, "2031-04-05T06:07:01",
			core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
		timelineRecord(2, nil),
		timelineRecord(3, timelineTimestamp(t, "2031-04-05T06:07:09Z",
			core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)),
	})
	from := time.Date(2031, 4, 5, 6, 0, 0, 0, time.UTC)
	timeline := graph.Timeline(TimelineQuery{
		RecordFilter: RecordFilter{TimeFrom: &from, TimeUnit: time.Second},
	})
	if len(timeline.Entries) != 1 || *timeline.Entries[0].RecordRef.LineNumber != 3 {
		t.Fatalf("the period listed %d entries, want the dated line 3 alone", len(timeline.Entries))
	}
	if timeline.PeriodUnjudgedLocalRecordCount != 1 || timeline.PeriodUnjudgedUndatedRecordCount != 1 {
		t.Errorf("the period left %d local and %d undated records unjudged, want 1 and 1",
			timeline.PeriodUnjudgedLocalRecordCount, timeline.PeriodUnjudgedUndatedRecordCount)
	}
	whole := graph.Timeline(TimelineQuery{})
	if whole.PeriodUnjudgedLocalRecordCount != 0 || whole.PeriodUnjudgedUndatedRecordCount != 0 {
		t.Errorf("a request without a period counted unjudged records: %+v", whole)
	}
}

// 期間で絞ると、期間の外のレコードが列から外れる。
func TestTimelineNarrowsTheListToTheRequestedPeriod(t *testing.T) {
	graph := graphOf(t)
	whole := graph.Timeline(TimelineQuery{})
	if len(whole.Entries) < 2 {
		t.Fatalf("the whole list carries %d entries, want more than one",
			len(whole.Entries))
	}
	from, ok := whole.Entries[len(whole.Entries)-1].EventTime.Instant()
	if !ok {
		t.Fatal("the last entry carries a time that cannot be compared")
	}
	narrowed := graph.Timeline(TimelineQuery{
		RecordFilter: RecordFilter{TimeFrom: &from, TimeUnit: time.Second},
	})
	if len(narrowed.Entries) == 0 {
		t.Fatal("a period that starts at the last entry returned no entry")
	}
	if len(narrowed.Entries) >= len(whole.Entries) {
		t.Fatalf("narrowing returned %d of the %d entries of the whole list",
			len(narrowed.Entries), len(whole.Entries))
	}
	for index, entry := range narrowed.Entries {
		at, readable := entry.EventTime.Instant()
		if !readable {
			t.Fatalf("entries[%d] carries a time that cannot be compared", index)
		}
		if at.Truncate(time.Second).Before(from.Truncate(time.Second)) {
			t.Fatalf("entries[%d] at %s is before the requested %s", index, at, from)
		}
	}
}

// 収集元ごとの収録範囲を、要求の期間との重なり方と一緒に返す。
func TestTimelineCarriesTheCoverageOfEverySourceOfTheImport(t *testing.T) {
	graph := graphOf(t)
	timeline := graph.Timeline(TimelineQuery{})
	if len(timeline.SourceCoverages) != len(graph.recordings) {
		t.Fatalf("the timeline carries %d coverages, the graph holds %d recordings",
			len(timeline.SourceCoverages), len(graph.recordings))
	}
	for index, coverage := range timeline.SourceCoverages {
		if coverage.SourceId != graph.recordings[index].sourceId {
			t.Fatalf("coverages[%d] names %q, the recordings name %q",
				index, coverage.SourceId, graph.recordings[index].sourceId)
		}
		if coverage.State != core.CoverageStateRangeNotRequested {
			t.Fatalf("a request without a period gave %q the state %q, want %q",
				coverage.SourceId, coverage.State, core.CoverageStateRangeNotRequested)
		}
	}
}

// 比較の単位を与えない要求が、秒の単位で比べる。
func TestTimelineFallsBackToTheSecondUnit(t *testing.T) {
	graph := graphOf(t)
	whole := graph.Timeline(TimelineQuery{})
	if len(whole.Entries) == 0 {
		t.Fatal("the timeline carries no entry")
	}
	at, ok := whole.Entries[0].EventTime.Instant()
	if !ok {
		t.Fatal("the first entry carries a time that cannot be compared")
	}
	// 秒の中の位置を持つ上端。秒で切り捨てると先頭の行がその秒に並ぶ。
	upper := at.Truncate(time.Second).Add(900 * time.Millisecond)
	withoutUnit := graph.Timeline(TimelineQuery{
		RecordFilter: RecordFilter{TimeTo: &upper},
	})
	withSecond := graph.Timeline(TimelineQuery{
		RecordFilter: RecordFilter{TimeTo: &upper, TimeUnit: time.Second},
	})
	if len(withoutUnit.Entries) != len(withSecond.Entries) {
		t.Fatalf("a request without a unit returned %d entries, "+
			"a request with the second unit returned %d",
			len(withoutUnit.Entries), len(withSecond.Entries))
	}
	if len(withoutUnit.Entries) == 0 {
		t.Fatal("an upper bound inside the second of the first entry returned no entry")
	}
}

// レコードが名乗った端末を持ち、分析者が与えた割当から導いた端末を持たない。
func TestTimelineCarriesTheTerminalTheRecordNamed(t *testing.T) {
	graph := graphOf(t)
	timeline := graph.Timeline(TimelineQuery{})
	var named int
	for index, entry := range timeline.Entries {
		if entry.Terminal == nil {
			continue
		}
		named++
		if entry.Terminal.Kind != core.NodeKindTerminal {
			t.Fatalf("entries[%d].terminal carries the kind %q, want %q",
				index, entry.Terminal.Kind, core.NodeKindTerminal)
		}
		if _, found := graph.nodeAt[entry.Terminal.Id]; !found {
			t.Fatalf("entries[%d].terminal points at %q, which is not a node of the graph",
				index, entry.Terminal.Id)
		}
	}
	if named == 0 {
		t.Fatal("no entry names a terminal, and the fixture records terminals")
	}
}

// 分析者が与えた端末の項目を、レコードが名乗った端末として読まない。
func TestRecordedTerminalNodeIdSkipsTheAnalystTerminal(t *testing.T) {
	specified, built := specifiedTerminalOf(core.TerminalAssignment{
		SourceId:         "source-a",
		ClientIp:         "198.51.100.10",
		TerminalId:       "T-analyst",
		TerminalHostname: "analyst.example.test",
		Derivation:       "test",
	}, false)
	if !built {
		t.Fatal("the assignment did not build the analyst terminal fields")
	}
	analyst := specified.fields
	if got := recordedTerminalNodeId(analyst, specified.scope); got != "" {
		t.Fatalf("the analyst terminal fields resolved to the node %q, want none", got)
	}
	// 収集元 1 件の名前不明の端末は、レコードが名乗った端末でない。
	unknown, built := unknownTerminalOf("aa11bb22", "host.log", false)
	if !built {
		t.Fatal("the unknown terminal did not build")
	}
	if got := recordedTerminalNodeId(nil, unknown.scope); got != "" {
		t.Fatalf("the unknown terminal of the source resolved to the node %q, want none", got)
	}
	recorded := append([]core.RecordField{
		syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
	}, analyst...)
	terminal, named := core.TerminalNodeKey("T1")
	if !named {
		t.Fatal("the terminal key of T1 did not build")
	}
	if got, want := recordedTerminalNodeId(recorded, specified.scope), nodeIdOf(terminal); got != want {
		t.Fatalf("the recorded terminal resolved to %q, want %q", got, want)
	}
}

// ノードの一覧の行から、根拠のレコードが名乗った端末を読む。
func TestSubgraphNodeCarriesTheTerminalsItsEvidenceNamed(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{
		Depth: 1,
	})
	var named int
	for _, node := range subgraph.Nodes {
		for _, terminal := range node.Terminals {
			named++
			if terminal.Kind != core.NodeKindTerminal {
				t.Fatalf("the node %q carries a terminal of kind %q, want %q",
					node.Id, terminal.Kind, core.NodeKindTerminal)
			}
			if _, found := graph.nodeAt[terminal.Id]; !found {
				t.Fatalf("the node %q points at the terminal %q, "+
					"which is not a node of the graph", node.Id, terminal.Id)
			}
		}
		// 同じ端末を 2 回持たない。
		seen := make(map[string]struct{}, len(node.Terminals))
		for _, terminal := range node.Terminals {
			if _, taken := seen[terminal.Id]; taken {
				t.Fatalf("the node %q carries the terminal %q twice",
					node.Id, terminal.Id)
			}
			seen[terminal.Id] = struct{}{}
		}
	}
	if named == 0 {
		t.Fatal("no node names a terminal, and the fixture records terminals")
	}
}

// 起点のノードを与えた要求は、段数 0 で起点の根拠だけを、段数 1 で起点に繋がるエッジの根拠も
// 並べる。時刻を持たないレコードは件数だけに入る。
func TestTimelineNarrowsTheListToTheRecordsNearTheNode(t *testing.T) {
	graph := graphOf(t)
	origin := -1
	for index, node := range graph.nodes {
		if node.key.Kind == core.NodeKindProcess && len(graph.adjacency[index].outgoing) > 0 {
			origin = index
			break
		}
	}
	if origin < 0 {
		t.Fatal("the fixture has no process with a relation")
	}
	adjacent := slices.Concat(graph.adjacency[origin].outgoing, graph.adjacency[origin].incoming)
	for _, testCase := range []struct {
		depth int
		edges []int
	}{
		{depth: 0},
		{depth: 1, edges: adjacent},
	} {
		want := make(map[int]struct{})
		for _, at := range graph.nodes[origin].evidence {
			want[at] = struct{}{}
		}
		for _, edge := range testCase.edges {
			for _, at := range graph.edges[edge].evidence {
				want[at] = struct{}{}
			}
		}
		wantKeys := make(map[string]struct{}, len(want))
		for at := range want {
			locator := graph.records[at].locator
			wantKeys[locator.SourceId+" "+locator.RecordRawTextRef] = struct{}{}
		}
		timeline := graph.Timeline(TimelineQuery{
			NodeIds: []string{graph.nodes[origin].id}, Depth: testCase.depth,
		})
		if got := len(timeline.Entries) + int(timeline.UndatedRecordCount); got != len(want) {
			t.Fatalf("depth %d listed %d records, want %d", testCase.depth, got, len(want))
		}
		for _, key := range timelineKeys(timeline) {
			if _, found := wantKeys[key]; !found {
				t.Fatalf("depth %d listed %q, which no relation within the depth has as evidence",
					testCase.depth, key)
			}
		}
	}
	if len(graph.Timeline(TimelineQuery{
		NodeIds: []string{graph.nodes[origin].id}, Depth: 1,
	}).Entries) <= len(graph.Timeline(TimelineQuery{
		NodeIds: []string{graph.nodes[origin].id},
	}).Entries) {
		t.Fatal("depth 1 listed no more records than depth 0")
	}
}

// 時系列の文字列条件はレコードの粒度のグラフと同じレコードを残す。
func TestTimelineAppliesTheSameTextSearchAsRecordGraph(t *testing.T) {
	graph := graphOf(t)
	for _, testCase := range []struct {
		name   string
		search GraphQuery
	}{
		{"含む", GraphQuery{ValueContains: []string{"app.exe"}}},
		{"含まない", GraphQuery{ValueExcludes: []string{"app.exe"}}},
		{"欄の指定", GraphQuery{ValueContains: []string{"app.exe"}, ValueFieldName: "psPath"}},
		{"欄の部分一致", GraphQuery{FieldContains: []FieldTerm{{Name: "psPath", Contains: "app.exe"}}}},
		{"欄の完全一致", GraphQuery{FieldContains: []FieldTerm{{Name: "dstPort", Contains: "8080", WholeValue: true}}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graphQuery := testCase.search
			graphQuery.Depth = 0
			graphQuery.Granularity = core.GraphGranularityRecord
			graphQuery.NodeKinds = []core.NodeKind{core.NodeKindRecord}
			graphQuery.RecordSummary = true
			want := make([]string, 0)
			for _, node := range graph.Query(graphQuery).Nodes {
				if node.Record == nil {
					t.Fatalf("record node %q has no summary", node.Id)
				}
				ref := node.Record.RecordRef
				want = append(want, ref.SourceId+" "+ref.RecordRawTextRef)
			}
			slices.Sort(want)

			timeline := graph.Timeline(TimelineQuery{TextSearch: testCase.search})
			got := timelineKeys(timeline)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("the timeline listed %v, want the graph records %v", got, want)
			}
		})
	}
}

// 時系列の文字列条件は、起点から辿るエッジの選択に作用しない。
func TestTimelineTextSearchDoesNotChangeNearRecordSelection(t *testing.T) {
	graph := graphOf(t)
	origin := -1
	for index, node := range graph.nodes {
		if node.key.Kind == core.NodeKindProcess && len(graph.adjacency[index].outgoing) > 0 {
			origin = index
			break
		}
	}
	if origin < 0 {
		t.Fatal("the fixture has no process with a relation")
	}
	plain := TimelineQuery{NodeIds: []string{graph.nodes[origin].id}, Depth: 1}
	filtered := plain
	filtered.TextSearch = GraphQuery{ValueContains: []string{"app.exe"}}
	if got, want := graph.recordsNearNodes(filtered), graph.recordsNearNodes(plain); !slices.Equal(got, want) {
		t.Fatal("the text condition changed the records selected by near-edge traversal")
	}
}

// 母集団は Selection ごとに違う。matched は絞り込みを通った根拠だけを見る。
func TestMatchedNodeCarriesOnlyTheTerminalsOfTheFilteredEvidence(t *testing.T) {
	graph := graphOf(t)
	whole := graph.Query(GraphQuery{Depth: 0})
	narrowed := graph.Query(GraphQuery{
		Depth:        0,
		RecordFilter: RecordFilter{EventCategory: "file", EventAction: "copy"},
	})
	if len(narrowed.Nodes) == 0 {
		t.Fatal("the narrowed query matched no node")
	}
	wholeTerminals := make(map[string]int, len(whole.Nodes))
	for _, node := range whole.Nodes {
		wholeTerminals[node.Id] = len(node.Terminals)
	}
	for _, node := range narrowed.Nodes {
		before, present := wholeTerminals[node.Id]
		if !present {
			t.Fatalf("the narrowed query returned the node %q, "+
				"which the whole query did not return", node.Id)
		}
		if len(node.Terminals) > before {
			t.Fatalf("narrowing gave the node %q %d terminals, more than the %d "+
				"of the whole query", node.Id, len(node.Terminals), before)
		}
	}
}
