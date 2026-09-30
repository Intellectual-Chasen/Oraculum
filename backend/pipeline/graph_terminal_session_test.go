// in-package test: 非公開の構築子で作った取り込み結果からグラフを組んで検査する。
package pipeline

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// graph-markii-session.log の 9 行は、端末 T1 (192.0.2.1) と端末 T2 (192.0.2.9) の
// レコードである。
//
// 3 行目と 4 行目は T2 が記録した T1 からの遠隔ログインとその失敗である。
// 5 行目は T1 が自分のアドレスを接続元に置いた外向きの接続である。
// 6 行目は接続元を持たない遠隔ログイン (srcIP が値の不在の文字列)、
// 7 行目は割当の無いアドレス 203.0.113.5 からの遠隔ログインである。
// 8 行目と 9 行目は、同じ channel と同じレコード ID を持つ 1 つの Windows イベントを
// 写した 2 件である。
func sessionGraph(t *testing.T) Graph {
	t.Helper()
	runner := graphRunner(t)
	result, err := runner.Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-session.log",
		FileName:   "graph-markii-session.log",
		FormatKey:  MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return NewGraph(result, AllMatchConditions())
}

func terminalSessionEdges(t *testing.T, graph Graph) []core.GraphEdge {
	t.Helper()
	return graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindTerminalRemoteSession}, Depth: 1}).Edges
}

// 接続元のアドレスを記録したレコードは、そのアドレスを保持していた端末から、
// レコードを記録した端末への候補のエッジを 1 本だけ作る。
func TestTerminalSessionEdgeRunsFromTheAddressHolderToTheRecordingTerminal(t *testing.T) {
	graph := sessionGraph(t)
	edges := terminalSessionEdges(t, graph)
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d remote session relations, want 1", len(edges))
	}
	edge := edges[0]
	if edge.State != core.RelationStateCandidate {
		t.Errorf("the relation carries the state %q, want candidate", edge.State)
	}
	detail, found := graph.EdgeDetail(edge.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the relation %q has no detail", edge.Id)
	}
	if detail.SourceNode.Identity[0].Value != "T1" ||
		detail.TargetNode.Identity[0].Value != "T2" {
		t.Fatalf("the relation runs from %q to %q, want T1 to T2",
			detail.SourceNode.Identity[0].Value, detail.TargetNode.Identity[0].Value)
	}
	// 遠隔ログインと遠隔ログインの失敗と、1 つに数えた Windows イベントの 3 件である。
	// 接続元を持たない行、割当の無いアドレスの行、自分のアドレスの行は根拠にならない。
	if edge.EvidenceCount != 3 {
		t.Errorf("the relation carries %d evidence records, want 3", edge.EvidenceCount)
	}
	actions := make([]string, 0, len(detail.Evidence))
	for _, evidence := range detail.Evidence {
		for _, field := range evidence.ObservationKind.Raw {
			if field.Semantic != core.SemanticKeyEventAction || field.Text == nil ||
				field.Text.RawText == nil {
				continue
			}
			actions = append(actions, *field.Text.RawText)
		}
	}
	requireSameStrings(t, "evidence actions", actions, "loginR", "loginRFail", "evtLog")
}

// **同じ事象を写した 2 件目を根拠に数えない。** 8 行目と 9 行目は、収集元が転記の同一性
// として宣言した欄 (ParserIdentity.TranscriptIdentityItems) の値がどちらも同じであり、
// 根拠は 1 件になる。
func TestTerminalSessionEdgeCountsOneTranscribedEventOnce(t *testing.T) {
	graph := sessionGraph(t)
	detail, found := graph.EdgeDetail(terminalSessionEdges(t, graph)[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatal("the relation has no detail")
	}
	recordIds := make([]string, 0, len(detail.Evidence))
	for _, evidence := range detail.Evidence {
		if evidence.RecordRef.SequenceNumber != nil {
			recordIds = append(recordIds,
				strconv.FormatInt(*evidence.RecordRef.SequenceNumber, 10))
		}
	}
	// 108 を数え、同じ事象を写した 109 を数えない。
	requireSameStrings(t, "evidence sequence numbers", recordIds, "103", "104", "108")
}

// 転記の同一性の宣言を持たない収集元のレコードは、毎回別の観測として数える。
//
// **宣言を持つ収集元だけが転記をまとめる。** 宣言を持たない収集元のレコードをまとめると、
// どの欄で同じ事象と判定したかを応答から読めない形で根拠が除かれる。
func TestCountsAsNewEvidenceCountsEveryRecordWithoutADeclaration(t *testing.T) {
	fields := []core.RecordField{
		textField("channel", "", presentText("Security")),
		textField("evtRecID", "", presentText("12345")),
	}
	seen := make(map[int]map[transcriptKey]struct{})
	for _, want := range []bool{true, true} {
		if got := countsAsNewEvidence(seen, 0, "T1", fields, nil); got != want {
			t.Errorf("countsAsNewEvidence = %t, want %t", got, want)
		}
	}
}

// 宣言した欄の値を連ねる文字列は、区切りの位置が変わる組を同じ鍵にしない。
//
// `a` と `b:c` を持つレコードと、`a:b` と `c` を持つレコードは別の事象である。
func TestTranscriptIdentityKeepsTheBoundaryBetweenItems(t *testing.T) {
	items := []string{"left", "right"}
	first, firstBuilt := transcriptIdentityOf([]core.RecordField{
		textField("left", "", presentText("a")),
		textField("right", "", presentText("b:c")),
	}, items)
	second, secondBuilt := transcriptIdentityOf([]core.RecordField{
		textField("left", "", presentText("a:b")),
		textField("right", "", presentText("c")),
	}, items)
	if !firstBuilt || !secondBuilt {
		t.Fatalf("the identities are built %t and %t, want both built", firstBuilt, secondBuilt)
	}
	if first == second {
		t.Errorf("two records with different item values share the identity %q", first)
	}
}

// 宣言した欄のいずれかを持たないレコードは、転記としてまとめない。
func TestTranscriptIdentityNeedsEveryDeclaredItem(t *testing.T) {
	fields := []core.RecordField{textField("channel", "", presentText("Security"))}
	if _, built := transcriptIdentityOf(fields, []string{"channel", "evtRecID"}); built {
		t.Error("the identity is built while the record carries no evtRecID item")
	}
	if _, built := transcriptIdentityOf(fields, []string{"channel"}); !built {
		t.Error("the identity is not built while the record carries the declared item")
	}
}

// 候補のエッジは、成立に用いた条件と割当の期間と時計への依拠を持つ。
func TestTerminalSessionEdgeCarriesTheAssignmentBasis(t *testing.T) {
	graph := sessionGraph(t)
	detail, found := graph.EdgeDetail(terminalSessionEdges(t, graph)[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatal("the relation has no detail")
	}
	// 関連付けの段階を経ないエッジであり、matches は要素数 0 である。
	if detail.MatchCount != 0 || len(detail.MatchTable.Matches) != 0 {
		t.Errorf("the relation carries %d matches, want 0", detail.MatchCount)
	}
	if len(detail.AssignmentBases) != 1 {
		t.Fatalf("the relation carries %d assignment bases, want 1",
			len(detail.AssignmentBases))
	}
	basis := detail.AssignmentBases[0]
	if basis.ClientIp != "192.0.2.1" {
		t.Errorf("the basis used the address %q, want 192.0.2.1", basis.ClientIp)
	}
	// 根拠は割当の期間を読み取った収集元を指す。
	if len(graph.recordings) != 1 || basis.SourceId != graph.recordings[0].sourceId {
		t.Errorf("the basis names the source %q, want the source of the assignment", basis.SourceId)
	}
	if len(basis.Conditions) != 1 {
		t.Fatalf("the basis carries %d conditions, want 1", len(basis.Conditions))
	}
	condition := basis.Conditions[0]
	if condition.ConditionKey != core.ConditionKeyTerminalIpAssignment ||
		condition.Use != core.ConditionUseUsed {
		t.Errorf("the condition is %q/%q, want terminal_ip_assignment/used",
			condition.ConditionKey, condition.Use)
	}
	if condition.AssignmentValidRange == nil {
		t.Fatal("the condition carries no assignment valid range")
	}
	if condition.OutsideAssignmentRange == nil || *condition.OutsideAssignmentRange {
		t.Error("the condition reports the record outside the assignment range")
	}
	if len(condition.LeftValue) != 1 || condition.LeftValue[0].Name != "srcIP" {
		t.Errorf("the left value is %+v, want the srcIP item", condition.LeftValue)
	}
	if len(condition.RightValue) != 1 ||
		condition.RightValue[0].Semantic != core.SemanticKeyTerminalId {
		t.Errorf("the right value is %+v, want the terminal id", condition.RightValue)
	}
	if condition.RightValue[0].Text == nil ||
		condition.RightValue[0].Text.ValueState != core.ValueStateDerived {
		t.Error("the resolved terminal is not carried as a derived value")
	}
	if basis.ClockDependencyNote == "" {
		t.Error("the basis carries no clock dependency note")
	}
	// **前提を持たない。** 割当の適用は秒未満のずれを仮定しない。
	if len(basis.Assumptions) != 0 {
		t.Errorf("the basis carries %d assumptions, want 0", len(basis.Assumptions))
	}
}

// 観測から直接作ったエッジは成立の根拠を持たない。
func TestObservedEdgeCarriesNoAssignmentBasis(t *testing.T) {
	graph := sessionGraph(t)
	ranOn := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindRanOn}, Depth: 1}).Edges
	if len(ranOn) == 0 {
		t.Fatal("the graph carries no observed relation")
	}
	detail, found := graph.EdgeDetail(ranOn[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatal("the observed relation has no detail")
	}
	if len(detail.AssignmentBases) != 0 {
		t.Errorf("the observed relation carries %d assignment bases, want 0",
			len(detail.AssignmentBases))
	}
}

// requireSameStrings は並び順を含めて文字列の集合を確かめる。
func requireSameStrings(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for index, expected := range want {
		if got[index] != expected {
			t.Errorf("%s[%d] = %q, want %q", label, index, got[index], expected)
		}
	}
}

// **同じ割当を指す根拠を 2 回持たない。** 3 件のレコードが同じ端末の組を結ぶが、
// 用いたアドレスと割当の期間と導いた端末は 1 通りである。
func TestTerminalSessionEdgeFoldsTheBasesOfTheSameAssignment(t *testing.T) {
	graph := sessionGraph(t)
	edges := terminalSessionEdges(t, graph)
	detail, found := graph.EdgeDetail(edges[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatal("the relation has no detail")
	}
	if len(detail.AssignmentBases) != 1 {
		t.Fatalf("the relation carries %d assignment bases, want 1",
			len(detail.AssignmentBases))
	}
	if detail.Edge.EvidenceCount <= int64(len(detail.AssignmentBases)) {
		t.Errorf("the relation carries %d evidence records and %d bases, want more evidence than bases",
			detail.Edge.EvidenceCount, len(detail.AssignmentBases))
	}
	// 呼び出し側の変更がグラフに及ばない。
	detail.AssignmentBases[0].ClientIp = "changed"
	again, _ := graph.EdgeDetail(edges[0].Id, EdgeEvidenceFilter{})
	if again.AssignmentBases[0].ClientIp != "192.0.2.1" {
		t.Errorf("the graph kept %q after the caller changed the returned basis",
			again.AssignmentBases[0].ClientIp)
	}
}

// **同じアドレスを 2 つの端末が保持する入力では、端末ごとに候補のエッジを作る。**
// 割当が 1 つに定まらないことを、候補を作らない理由にしない。
func TestTerminalSessionEdgeKeepsEveryTerminalHoldingTheAddress(t *testing.T) {
	runner := graphRunner(t)
	result, err := runner.Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-session-shared.log",
		FileName:   "graph-markii-session-shared.log",
		FormatKey:  MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	edges := terminalSessionEdges(t, graph)
	if len(edges) != 2 {
		t.Fatalf("the graph carries %d remote session relations, want 2", len(edges))
	}
	sources := make([]string, 0, len(edges))
	for _, edge := range edges {
		detail, found := graph.EdgeDetail(edge.Id, EdgeEvidenceFilter{})
		if !found {
			t.Fatalf("the relation %q has no detail", edge.Id)
		}
		if detail.TargetNode.Identity[0].Value != "T3" {
			t.Errorf("the relation ends at %q, want T3", detail.TargetNode.Identity[0].Value)
		}
		// **どの端末を導いたかを根拠が持つ。** 2 本の関係は同じアドレスから別の端末を導く。
		if len(detail.AssignmentBases) != 1 {
			t.Fatalf("the relation carries %d assignment bases, want 1",
				len(detail.AssignmentBases))
		}
		if detail.AssignmentBases[0].ClientIp != "192.0.2.1" {
			t.Errorf("the basis used the address %q, want 192.0.2.1",
				detail.AssignmentBases[0].ClientIp)
		}
		sources = append(sources, detail.SourceNode.Identity[0].Value)
	}
	requireSameStrings(t, "relation sources", sources, "T1", "T2")
}
