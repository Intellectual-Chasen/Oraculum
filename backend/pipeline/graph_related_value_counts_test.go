// in-package test: ノードに繋がる関係の相手側の欄の値ごとの件数を確かめる。
package pipeline

import (
	"slices"
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// relatedCountTexts は値ごとの件数を「値 件数」の文字列にして並べる。
func relatedCountTexts(t *testing.T, counts []core.ValueCount) []string {
	t.Helper()
	texts := make([]string, 0, len(counts))
	for _, count := range counts {
		if err := count.Validate(); err != nil {
			t.Errorf("%s: %v", count.Value, err)
		}
		texts = append(texts, count.Value+" "+strconv.FormatInt(count.RecordCount, 10))
	}
	slices.Sort(texts)
	return texts
}

// 相手が対象のノードのとき、その属性を観測した根拠のうち、関係の根拠のレコードだけを数える。
// 同じプロセスの実行ファイルを 2 件のレコードが記録しても、1 件目のレコードのノードから辿った
// 相手の件数は 1 件である。
func TestRelatedValueCountsKeepTheEvidenceOfTheRelation(t *testing.T) {
	record := func(line int64) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
					syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
					syntheticField(t, "psPath", core.SemanticKeyProcessBinaryPath, `C:\Example\tool.exe`),
				},
			},
		}
	}
	graph := graphOfRecords(t, []RecordEntry{record(1), record(2)})
	first := graph.nodes[graph.records[0].recordNode].id
	counts, _ := graph.RelatedValueCounts(first, core.EdgeKindRecordNamesObject, core.EdgeDirectionOutgoing,
		GraphQuery{CountBySemantic: core.SemanticKeyProcessBinaryPath})
	if got, want := relatedCountTexts(t, counts), []string{`C:\Example\tool.exe 1`}; !slices.Equal(got, want) {
		t.Errorf("counts = %v, want %v (the second record observes the process outside the relation)", got, want)
	}
}

// HTTP の要求の相手のホスト名のノードは、ホスト名を識別の欄に持ち、属性に持たない。数える欄が
// 相手の識別の欄であるときは、関係の根拠のレコードがその欄に記録した相手の値を数える。
func TestRelatedValueCountsCountTheIdentityOfTheOtherEnd(t *testing.T) {
	graph := graphOf(t)
	client := -1
	for index, node := range graph.nodes {
		if node.key.Kind == core.NodeKindIp && node.key.Values[len(node.key.Values)-1].Value == "192.0.2.1" {
			client = index
		}
	}
	if client < 0 {
		t.Fatal("the graph holds no IP node for the client 192.0.2.1")
	}
	counts, _ := graph.RelatedValueCounts(graph.nodes[client].id, core.EdgeKindHttpRequest,
		core.EdgeDirectionOutgoing, GraphQuery{CountBySemantic: core.SemanticKeyConnectionDestinationHostname})
	if got, want := relatedCountTexts(t, counts), []string{"example.test 1"}; !slices.Equal(got, want) {
		t.Errorf("counts = %v, want %v", got, want)
	}

	// 根拠のレコードの欄の値が相手の識別鍵の値と違うときは、その値を数えない。
	var host core.NodeKey
	var evidence []int
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindHttpRequest && edge.source == client &&
			graph.nodes[edge.target].key.Kind == core.NodeKindDomain {
			host, evidence = graph.nodes[edge.target].key, edge.evidence
		}
	}
	if len(evidence) == 0 {
		t.Fatal("the client has no HTTP request edge to a hostname")
	}
	other := core.NodeKey{Kind: host.Kind, Form: host.Form, Values: []core.NodeIdentityValue{{Value: "other.example.test"}}}
	query := GraphQuery{CountBySemantic: core.SemanticKeyConnectionDestinationHostname}
	graph.countIdentityOfOtherEnd(other, evidence, query, func(value string, _ int) {
		t.Errorf("the value %q that differs from the other end is counted", value)
	})

	// 相手がレコードのノードのときは、レコードのノードの属性として 1 件だけ数える。
	domain := graph.nodeAt[nodeIdOf(host)]
	fromRecords, _ := graph.RelatedValueCounts(graph.nodes[domain].id, core.EdgeKindRecordNamesObject,
		core.EdgeDirectionIncoming, query)
	if got, want := relatedCountTexts(t, fromRecords), []string{"example.test 1"}; !slices.Equal(got, want) {
		t.Errorf("record counts = %v, want %v", got, want)
	}
}

// 相手の識別鍵が持つ端末の範囲の値は、相手の識別の値として数えない。端末の欄は端末のノードの
// 属性として数える。
func TestRelatedValueCountsSkipTheTerminalScopeOfTheOtherEnd(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
			},
		},
	}})
	scoped := core.NodeKey{Kind: core.NodeKindProcess, Values: []core.NodeIdentityValue{
		{Semantic: core.SemanticKeyTerminalId, Value: "T1"},
		{Semantic: core.SemanticKeyProcessId, Value: "{P1}"},
	}}
	query := GraphQuery{CountBySemantic: core.SemanticKeyTerminalId}
	graph.countIdentityOfOtherEnd(scoped, []int{0}, query, func(value string, _ int) {
		t.Errorf("the terminal scope %q of the other end is counted", value)
	})
}

// ログオン 31 の外向きの同じログオンセッションの操作の候補は、操作のレコードの欄だけを数え、
// 起点のログオン自身の欄を数えない。操作のレコード 39 の内向きは、2 件のログオンの欄を数える。
func TestRelatedValueCountsCountTheOtherEndOfTheRelation(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	id := func(recordID string) string { return graph.nodes[nodes[recordID]].id }
	query := GraphQuery{CountBySemantic: core.SemanticKeyWindowsEventId}

	outgoing, found := graph.RelatedValueCounts(id("31"), core.EdgeKindLogonSessionOperation,
		core.EdgeDirectionOutgoing, query)
	if !found {
		t.Fatal("the logon node is not found")
	}
	// 32 (4672)、33 (4688)、34 (4624)、39 (4720) が操作である。31 自身の 4624 は数えない。
	if got, want := relatedCountTexts(t, outgoing), []string{"4624 1", "4672 1", "4688 1", "4720 1"}; !slices.Equal(got, want) {
		t.Errorf("outgoing counts = %v, want %v", got, want)
	}

	incoming, _ := graph.RelatedValueCounts(id("39"), core.EdgeKindLogonSessionOperation,
		core.EdgeDirectionIncoming, query)
	if got, want := relatedCountTexts(t, incoming), []string{"4624 2"}; !slices.Equal(got, want) {
		t.Errorf("incoming counts = %v, want the two logons %v", got, want)
	}

	// 別の種別の関係の相手 (レコードが指す対象のノード) は、イベント ID の欄を持たない。
	other, _ := graph.RelatedValueCounts(id("31"), core.EdgeKindRecordNamesObject,
		core.EdgeDirectionOutgoing, query)
	if len(other) != 0 {
		t.Errorf("record_names_object counts = %v, want none from the object nodes", relatedCountTexts(t, other))
	}

	if _, found := graph.RelatedValueCounts("n:absent", core.EdgeKindLogonSessionOperation,
		core.EdgeDirectionOutgoing, query); found {
		t.Error("an absent node is found")
	}
}
