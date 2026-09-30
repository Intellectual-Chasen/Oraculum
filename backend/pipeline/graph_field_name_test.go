// in-package test: 検索と集計の欄の名前の当て方と、集計が 0 件になった理由を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// nestedFieldGraph は、入れ子の要素の名前を親の名前と `.` で繋いだ欄を持つレコード 3 件のグラフである。
// 1 と 2 は語彙へ写した欄を、3 は語彙へ写さない欄と、値の不在を文字列で書いた欄を持つ。
func nestedFieldGraph(t *testing.T) Graph {
	t.Helper()
	absent, err := core.NewRawValue(core.ValueStateAbsent, "-")
	if err != nil {
		t.Fatal(err)
	}
	absentField, err := core.NewTextField("Data.Missing", "", absent)
	if err != nil {
		t.Fatal(err)
	}
	return graphOfRecords(t, []RecordEntry{
		eventRecord(t, 1, "4688",
			syntheticField(t, "Data.ImagePath", core.SemanticKeyProcessBinaryPath, `C:\example\tool.exe`),
			syntheticField(t, "Data.Kind", core.SemanticKeyEventLogonType, "3")),
		eventRecord(t, 2, "4688",
			syntheticField(t, "Data.ImagePath", core.SemanticKeyProcessBinaryPath, `C:\example\other.exe`),
			syntheticField(t, "Data.Kind", core.SemanticKeyEventLogonType, "3")),
		eventRecord(t, 3, "4624",
			syntheticField(t, "Data.Note", "", "plain"), absentField),
	})
}

// 欄の名前は、欄の名前そのものと、`.` で区切った末尾の部分で指せる。語彙へ写した欄も
// 原資料の名前で一致する。末尾の部分の途中から始まる名前は一致しない。
func TestFieldNameMatchesTheWholeNameAndItsLastParts(t *testing.T) {
	graph := nestedFieldGraph(t)
	for name, testCase := range map[string]struct {
		field string
		want  []string
	}{
		"欄の名前そのもの": {"Data.ImagePath", []string{"synthetic.log 行 1"}},
		"末尾の部分":    {"ImagePath", []string{"synthetic.log 行 1"}},
		"部分の途中":    {"agePath", nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := matchedRecordLabels(t, graph,
				GraphQuery{ValueContains: []string{"tool"}, ValueFieldName: testCase.field})
			if !slices.Equal(got, testCase.want) {
				t.Errorf("matched %v, want %v", got, testCase.want)
			}
		})
	}
}

// 欄と文字列の組を複数置くと、1 件のレコードがどの組も満たすときに一致する。組ごとに欄が違ってよい。
func TestFieldTermsMatchEveryPairInOneRecord(t *testing.T) {
	graph := nestedFieldGraph(t)
	for name, testCase := range map[string]struct {
		terms []FieldTerm
		want  []string
	}{
		"2 つの欄の組": {[]FieldTerm{
			{Name: "ImagePath", Contains: "tool"},
			{Semantic: core.SemanticKeyEventLogonType, Contains: "3"},
		}, []string{"synthetic.log 行 1"}},
		"片方の組が外れる": {[]FieldTerm{
			{Name: "ImagePath", Contains: "tool"},
			{Name: "Kind", Contains: "4"},
		}, nil},
		// 文字列は組の欄でだけ判定する。
		"別の欄にある文字列": {[]FieldTerm{{Name: "Kind", Contains: "tool"}}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := matchedRecordLabels(t, graph, GraphQuery{FieldContains: testCase.terms})
			if !slices.Equal(got, testCase.want) {
				t.Errorf("matched %v, want %v", got, testCase.want)
			}
		})
	}
}

// **完全一致の組は、欄の値の全体が文字列と大文字と小文字を区別せずに等しいレコードだけに一致する。**
// 文字列を値の一部に含むだけのレコードと、番号の一部が一致するレコードは一致しない。
func TestFieldTermsWithWholeValueMatchTheEqualValueAlone(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		eventRecord(t, 1, "4624", syntheticField(t, "Kind", "", "1"), syntheticField(t, "User", "", "Admin")),
		eventRecord(t, 2, "4624", syntheticField(t, "Kind", "", "10"), syntheticField(t, "User", "", "siteadmin")),
		eventRecord(t, 3, "4624", syntheticField(t, "Kind", "", "21"), syntheticField(t, "User", "", "admin2")),
	})
	for name, testCase := range map[string]struct {
		term FieldTerm
		want []string
	}{
		"番号":      {FieldTerm{Name: "Kind", Contains: "1", WholeValue: true}, []string{"synthetic.log 行 1"}},
		"大小をそろえる": {FieldTerm{Name: "User", Contains: "admin", WholeValue: true}, []string{"synthetic.log 行 1"}},
		"部分一致のまま": {FieldTerm{Name: "Kind", Contains: "1"}, []string{"synthetic.log 行 1", "synthetic.log 行 2", "synthetic.log 行 3"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := matchedRecordLabels(t, graph, GraphQuery{FieldContains: []FieldTerm{testCase.term}})
			if !slices.Equal(got, testCase.want) {
				t.Errorf("matched %v, want %v", got, testCase.want)
			}
		})
	}
}

// 同じ名前の欄があるときは、末尾の部分が同じ別の欄を当てない。
func TestFieldNamePrefersTheWholeName(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		eventRecord(t, 1, "4688",
			syntheticField(t, "Rank", "", "4"),
			syntheticField(t, "Display.Rank", "", "Information")),
	})
	counts := graph.Query(GraphQuery{Granularity: core.GraphGranularityRecord, CountByName: "Rank"}).ValueCounts
	if len(counts) != 1 || counts[0].Value != "4" {
		t.Errorf("counted %+v, want the value 4 alone", counts)
	}
	matched := matchedRecordLabels(t, graph,
		GraphQuery{ValueContains: []string{"Information"}, ValueFieldName: "Rank"})
	if len(matched) != 0 {
		t.Errorf("matched %v through the other field", matched)
	}
	if !graph.ObservesField("", "Rank") || !graph.ObservesField("", "Display.Rank") {
		t.Error("ObservesField misses a field the graph carries")
	}
}

// 原資料の名前で指した集計は、語彙へ写した欄の値も数える。
func TestCountByNameCountsTheFieldsCarryingASemantic(t *testing.T) {
	graph := nestedFieldGraph(t)
	for _, name := range []string{"Data.Kind", "Kind"} {
		t.Run(name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{
				Granularity: core.GraphGranularityRecord, CountByName: name,
			})
			if len(subgraph.ValueCounts) != 1 || subgraph.ValueCounts[0].Value != "3" ||
				subgraph.ValueCounts[0].RecordCount != 2 {
				t.Errorf("counted %+v, want the value 3 of 2 records", subgraph.ValueCounts)
			}
		})
	}
}

// 集計が 0 件になった理由は、欄の名前が無い・値が無い・対象の種別が欄を持たない・絞り込みで
// 残らない、を分ける。
func TestCountedFieldObservationSeparatesTheEmptyReasons(t *testing.T) {
	graph := nestedFieldGraph(t)
	for name, testCase := range map[string]struct {
		query     GraphQuery
		want      core.EmptyReason
		wantKinds []core.NodeKind
	}{
		"欄の名前が無い": {GraphQuery{CountByName: "Absent"}, core.EmptyReasonNoFieldObserved, nil},
		"値が無い":    {GraphQuery{CountByName: "Missing"}, core.EmptyReasonNoReadableValue, nil},
		"対象の種別が欄を持たない": {
			GraphQuery{CountByName: "Note", Granularity: core.GraphGranularityObject},
			core.EmptyReasonFieldOnOtherNodeKind, []core.NodeKind{core.NodeKindRecord},
		},
		"絞り込みで残らない": {
			GraphQuery{CountByName: "Note", RecordFilter: RecordFilter{EventAction: "4688"}},
			core.EmptyReasonNoValueInFilter, nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if counts := graph.Query(testCase.query).ValueCounts; len(counts) != 0 {
				t.Fatalf("counted %+v, want none", counts)
			}
			got, kinds := graph.CountedFieldObservationOf(testCase.query).EmptyReason(testCase.query)
			if got != testCase.want || !slices.Equal(kinds, testCase.wantKinds) {
				t.Errorf("reason %q %v, want %q %v", got, kinds, testCase.want, testCase.wantKinds)
			}
		})
	}
}

// レコードの要約は、Windows イベントログのレコードのプロバイダとイベント ID とチャネルを持つ。
func TestRecordSummaryCarriesTheWindowsEventKind(t *testing.T) {
	source := settleSource(t, 0)
	// 入力形式がレコードの見出しの番号の欄を宣言する。
	source.Parser.RecordNumbering.RecordHeaderID = "Header.Number"
	source.Records = []RecordEntry{
		eventRecord(t, 1, "4688",
			syntheticField(t, "Channel", core.SemanticKeyWindowsEventChannel, "Example-Channel"),
			syntheticField(t, "EventRecordID", core.SemanticKeyWindowsEventRecordId, "321"),
			syntheticField(t, "Header.Number", "", "654"),
			// 同じ欄を 2 つ持つレコードは、最初の値を持つ。
			syntheticField(t, "EventRecordID", core.SemanticKeyWindowsEventRecordId, "999"),
			syntheticField(t, "Header.Number", "", "998")),
	}
	result, err := newImportResult([]scannedSource{source},
		[]core.ImportStatus{settleStatus(t, source, "source")}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	nodes := graph.Query(GraphQuery{
		Granularity: core.GraphGranularityRecord, NodeKinds: []core.NodeKind{core.NodeKindRecord},
		RecordSummary: true,
	}).Nodes
	if len(nodes) != 1 || nodes[0].Record == nil {
		t.Fatalf("the query returned %+v, want one summarized record", nodes)
	}
	summary := nodes[0].Record
	if !summary.WindowsEvent || summary.EventCategory != "Example-Provider" ||
		summary.EventAction != "4688" || summary.Channel != "Example-Channel" ||
		summary.RecordRef.SourceFileName != "synthetic.log" || summary.EventTime == nil {
		t.Errorf("the summary is %+v", summary)
	}
	// レコードの見出しの番号と、XML の EventRecordID を持つ。
	if summary.RecordHeaderId != "654" || summary.EventRecordId != "321" {
		t.Errorf("the summary carries the record header number %q and EventRecordID %q, want 654 and 321",
			summary.RecordHeaderId, summary.EventRecordId)
	}
}

// 収集元で絞る条件は、与えた収集元のどれかのレコードを残す。
func TestRecordFilterNarrowsToTheSources(t *testing.T) {
	result := caseImport(t,
		auditdSource("left.log", caseParentSource, nil),
		auditdSource("right.log", caseChildSource, nil))
	graph := NewGraph(result, AllMatchConditions())
	idOf := make(map[string]string)
	for _, status := range result.Statuses() {
		identity, _ := result.Identity(status.SourceId)
		idOf[identity.FileName] = status.SourceId
	}
	for name, testCase := range map[string]struct {
		sources []string
		want    []string
	}{
		"1 つ":  {[]string{idOf["left.log"]}, []string{"left.log"}},
		"2 つ":  {[]string{idOf["left.log"], idOf["right.log"]}, []string{"left.log", "right.log"}},
		"絞らない": {nil, []string{"left.log", "right.log"}},
	} {
		t.Run(name, func(t *testing.T) {
			query := TimelineQuery{RecordFilter: RecordFilter{Sources: testCase.sources}}
			query.Validate()
			var got []string
			for _, entry := range graph.Timeline(query).Entries {
				if !slices.Contains(got, entry.RecordRef.SourceFileName) {
					got = append(got, entry.RecordRef.SourceFileName)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, testCase.want) {
				t.Errorf("the records come from %v, want %v", got, testCase.want)
			}
		})
	}
	if !graph.HasSource(idOf["left.log"]) || graph.HasSource("no-such-source") {
		t.Error("HasSource does not tell the imported sources from the others")
	}
}

// 割当は、割当を付けた収集元が収集元の条件に入るときだけ通る。
func TestAssignmentMatchesTheSourceItAppliesTo(t *testing.T) {
	assignment := core.TerminalAssignment{AppliesToSourceId: "source-a"}
	for name, testCase := range map[string]struct {
		sources []string
		want    bool
	}{
		"付けた収集元":   {[]string{"source-b", "source-a"}, true},
		"ほかの収集元":   {[]string{"source-b"}, false},
		"収集元で絞らない": {nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			got := Graph{}.assignmentMatches(assignment, "", RecordFilter{Sources: testCase.sources})
			if got != testCase.want {
				t.Errorf("the assignment passes: %v, want %v", got, testCase.want)
			}
		})
	}
}
