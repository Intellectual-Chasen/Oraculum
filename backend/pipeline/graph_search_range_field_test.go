// in-package test: イベント ID の範囲の絞り込みと、欄を指定した文字列の検索を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// eventRecord は、プロバイダ provider のイベント ID eventID と、欄 fields を持つレコード 1 件である。
func eventRecord(t *testing.T, line int64, eventID string, fields ...core.RecordField) RecordEntry {
	t.Helper()
	record := timelineRecord(line, utcAt(t, "2031-04-05T06:00:00Z"))
	record.Semantics = &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields: append([]core.RecordField{
			syntheticField(t, "Provider", core.SemanticKeyWindowsEventProvider, "Example-Provider"),
			syntheticField(t, "EventID", core.SemanticKeyWindowsEventId, eventID),
		}, fields...),
	}
	return record
}

// matchedRecordLabels は部分グラフのうち、合致したレコードのノードの表示名から、収集元の file 名と
// 位置の部分を並べる。表示名の先頭の Event ID は外す。
func matchedRecordLabels(t *testing.T, graph Graph, query GraphQuery) []string {
	t.Helper()
	query.Granularity = core.GraphGranularityRecord
	query.NodeKinds = []core.NodeKind{core.NodeKindRecord}
	var labels []string
	for _, node := range graph.Query(query).Nodes {
		if node.Selection == core.NodeSelectionMatched {
			label, _ := node.Label.ComparableValue()
			if eventID, rest, cut := strings.Cut(label, " "); cut && strings.Trim(eventID, "0123456789") == "" {
				label = rest
			}
			labels = append(labels, label)
		}
	}
	slices.Sort(labels)
	return labels
}

func uint64Of(value uint64) *uint64 { return &value }

// 範囲は両端を含む。範囲の外と、数として読めない動作のレコードは除かれる。片側だけの範囲も絞る。
// EventAction の完全一致と重ねると、両方を満たすレコードだけが残る。
func TestEventActionRangeKeepsTheRecordsInsideBothBounds(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		eventRecord(t, 1, "7999"), eventRecord(t, 2, "8000"), eventRecord(t, 3, "8500"),
		eventRecord(t, 4, "8999"), eventRecord(t, 5, "9000"), eventRecord(t, 6, "80a0"),
	})
	for name, testCase := range map[string]struct {
		filter RecordFilter
		want   []string
	}{
		"両端": {RecordFilter{EventActionFrom: uint64Of(8000), EventActionTo: uint64Of(8999)},
			[]string{"synthetic.log 行 2", "synthetic.log 行 3", "synthetic.log 行 4"}},
		"下端だけ": {RecordFilter{EventActionFrom: uint64Of(8999)},
			[]string{"synthetic.log 行 4", "synthetic.log 行 5"}},
		"完全一致と重ねる": {RecordFilter{EventAction: "8500", EventActionFrom: uint64Of(8000), EventActionTo: uint64Of(8999)},
			[]string{"synthetic.log 行 3"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := matchedRecordLabels(t, graph, GraphQuery{Depth: 0, RecordFilter: testCase.filter})
			if !slices.Equal(got, testCase.want) {
				t.Errorf("matched %v, want %v", got, testCase.want)
			}
			if entries := graph.Timeline(TimelineQuery{RecordFilter: testCase.filter}).Entries; len(entries) != len(testCase.want) {
				t.Errorf("the timeline holds %d rows, want %d", len(entries), len(testCase.want))
			}
		})
	}
}

// 欄を指定した文字列の条件は、その欄にだけ一致する。語彙の項目でも原資料の key でも指せ、key の
// 指定は語彙を持つ欄にも一致する。含まない側は、その欄に文字列を含まないレコードを残す。一致した
// 欄は指定の欄だけである。
func TestValueFieldLimitsTheSearchToOneField(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		eventRecord(t, 1, "4688",
			syntheticField(t, "CommandLine", core.SemanticKeyProcessCommandLine, "tool.exe -a example"),
			syntheticField(t, "Note", "", "plain")),
		eventRecord(t, 2, "4688",
			syntheticField(t, "CommandLine", core.SemanticKeyProcessCommandLine, "other.exe"),
			syntheticField(t, "Note", "", "example")),
	})
	for name, testCase := range map[string]struct {
		query GraphQuery
		want  []string
	}{
		"全欄": {GraphQuery{ValueContains: []string{"example"}},
			[]string{"synthetic.log 行 1", "synthetic.log 行 2"}},
		"語彙の項目": {GraphQuery{ValueContains: []string{"example"}, ValueFieldSemantic: core.SemanticKeyProcessCommandLine},
			[]string{"synthetic.log 行 1"}},
		"原資料の key (語彙を持つ欄)": {GraphQuery{ValueContains: []string{"example"}, ValueFieldName: "CommandLine"},
			[]string{"synthetic.log 行 1"}},
		"原資料の key (語彙を持たない欄)": {GraphQuery{ValueContains: []string{"example"}, ValueFieldName: "Note"},
			[]string{"synthetic.log 行 2"}},
		"含まない": {GraphQuery{ValueExcludes: []string{"example"}, ValueFieldName: "CommandLine"},
			[]string{"synthetic.log 行 2"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := matchedRecordLabels(t, graph, testCase.query); !slices.Equal(got, testCase.want) {
				t.Errorf("matched %v, want %v", got, testCase.want)
			}
		})
	}
	query := GraphQuery{ValueContains: []string{"example"}, ValueFieldName: "CommandLine",
		Granularity: core.GraphGranularityRecord, NodeKinds: []core.NodeKind{core.NodeKindRecord}}
	for _, node := range graph.Query(query).Nodes {
		for _, match := range node.ValueMatches {
			if match.Semantic != core.SemanticKeyProcessCommandLine {
				t.Errorf("%s matched %+v, want the command line alone", node.Id, match)
			}
		}
	}
}
