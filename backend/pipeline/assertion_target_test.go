// in-package test: fixture のグラフを組む非公開の helper を使う。
package pipeline

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assertionOf は対象だけを差し替えた所見を作る。
// 保存先を通さずに出所の判定だけを確かめるため、識別子と時刻は値にする。
func assertionOf(target core.AssertionTarget) core.Assertion {
	return core.Assertion{
		Id: "as:fixture", Target: target, State: core.AssertionStateActive,
		Author: "analyst-a", RecordedAt: "2026-01-02T03:04:05.000Z",
		Basis:          core.AssertionBasis{Note: "fixture"},
		RevisionNumber: core.FirstAssertionRevisionNumber,
	}
}

// edgeRefOfState は fixture のグラフから、求める状態のエッジ 1 本の参照を返す。
func edgeRefOfState(t *testing.T, subgraph Subgraph, state core.RelationState) core.AssertionEdgeRef {
	t.Helper()
	for _, edge := range subgraph.Edges {
		if edge.State == state {
			return core.AssertionEdgeRef{
				Kind: edge.Kind, SourceNodeId: edge.SourceNodeId, TargetNodeId: edge.TargetNodeId,
			}
		}
	}
	t.Fatalf("the fixture graph carries no edge in the state %q", state)
	return core.AssertionEdgeRef{}
}

// 観測から直に出した関係、関連付けが挙げた候補、分析者が足した関係、現在の取り込みに無い対象を
// 別々の出所として分ける。
func TestAssertionResolverSeparatesTheOriginOfTheTarget(t *testing.T) {
	graph := graphOf(t)
	subgraph := wholeGraphOf(t, graph)
	resolver := NewAssertionResolver(graph)

	observed := edgeRefOfState(t, subgraph, core.RelationStateObserved)
	candidate := edgeRefOfState(t, subgraph, core.RelationStateCandidate)
	// 分析者が足した関係は、両端が観測層にありながらグラフが持たない組である。
	added := core.AssertionEdgeRef{
		Kind: core.EdgeKindFileCopy, SourceNodeId: observed.SourceNodeId,
		TargetNodeId: candidate.TargetNodeId,
	}
	if _, found := graph.edgeIndexOf(assertionEdgeIdOf(added)); found {
		t.Fatalf("the fixture graph already carries the edge %+v, want a pair it does not carry", added)
	}
	addition := assertionOf(edgeTarget(added))
	addition.AddsRelation = true
	relations := NewAddedRelations([]core.Assertion{addition})

	cases := []struct {
		name  string
		given core.Assertion
		want  core.AssertionTargetOrigin
	}{
		{
			name:  "observed edge",
			given: assertionOf(edgeTarget(observed)),
			want:  core.AssertionTargetOriginObservation,
		},
		{
			name:  "matching candidate edge",
			given: assertionOf(edgeTarget(candidate)),
			want:  core.AssertionTargetOriginMatchingCandidate,
		},
		{
			name:  "edge the analyst added",
			given: addition,
			want:  core.AssertionTargetOriginAnalystAssertion,
		},
		{
			name: "node outside the current import",
			given: assertionOf(core.AssertionTarget{
				Kind: core.AssertionTargetKindNode, NodeId: "n:process:absent",
			}),
			want: core.AssertionTargetOriginAbsent,
		},
		{
			name: "node of the current import",
			given: assertionOf(core.AssertionTarget{
				Kind: core.AssertionTargetKindNode, NodeId: subgraph.Nodes[0].Id,
			}),
			want: core.AssertionTargetOriginObservation,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if origin := resolver.Origin(testCase.given, relations); origin != testCase.want {
				t.Errorf("Origin() = %q, want %q", origin, testCase.want)
			}
		})
	}
}

// 取り下げた所見が指す関係は、誰も主張していないため出所が absent になる。
func TestAddedRelationsCountsTheAssertedRelations(t *testing.T) {
	graph := graphOf(t)
	subgraph := wholeGraphOf(t, graph)
	resolver := NewAssertionResolver(graph)
	observed := edgeRefOfState(t, subgraph, core.RelationStateObserved)
	candidate := edgeRefOfState(t, subgraph, core.RelationStateCandidate)
	added := core.AssertionEdgeRef{
		Kind: core.EdgeKindFileCopy, SourceNodeId: observed.SourceNodeId,
		TargetNodeId: candidate.TargetNodeId,
	}
	// 分析者が足した関係を主張する所見と、同じ関係を読みに来た別の所見を分ける。
	asserting := assertionOf(edgeTarget(added))
	asserting.AddsRelation = true
	reading := assertionOf(edgeTarget(added))
	reading.Id = "as:fixture-reading"

	asserted := NewAddedRelations([]core.Assertion{asserting})
	if origin := resolver.Origin(reading, asserted); origin !=
		core.AssertionTargetOriginAnalystAssertion {
		t.Errorf("Origin() while the assertion on the relation is asserted = %q, want %q",
			origin, core.AssertionTargetOriginAnalystAssertion)
	}

	asserting.State = core.AssertionStateWithdrawn
	withdrawn := NewAddedRelations([]core.Assertion{asserting})
	if origin := resolver.Origin(reading, withdrawn); origin !=
		core.AssertionTargetOriginAbsent {
		t.Errorf("Origin() while the assertion on the relation is withdrawn = %q, want %q",
			origin, core.AssertionTargetOriginAbsent)
	}
}

// 記録時にグラフにあった関係を指す所見は、再解析でその関係が消えると absent になる。
// 所見自身がその関係を指していても、分析者が足した関係に数えない。
func TestAssertionOnAVanishedRelationIsAbsent(t *testing.T) {
	graph := graphOf(t)
	subgraph := wholeGraphOf(t, graph)
	resolver := NewAssertionResolver(graph)
	observed := edgeRefOfState(t, subgraph, core.RelationStateObserved)
	candidate := edgeRefOfState(t, subgraph, core.RelationStateCandidate)
	vanished := core.AssertionEdgeRef{
		Kind: core.EdgeKindFileCopy, SourceNodeId: observed.SourceNodeId,
		TargetNodeId: candidate.TargetNodeId,
	}
	if _, found := graph.edgeIndexOf(assertionEdgeIdOf(vanished)); found {
		t.Fatalf("the fixture graph already carries the edge %+v, want a pair it does not carry", vanished)
	}
	onVanished := assertionOf(edgeTarget(vanished))
	relations := NewAddedRelations([]core.Assertion{onVanished})
	if origin := resolver.Origin(onVanished, relations); origin != core.AssertionTargetOriginAbsent {
		t.Errorf("Origin() of the assertion on a vanished relation = %q, want %q",
			origin, core.AssertionTargetOriginAbsent)
	}
}

// レコードを指す所見は、収集元の内容の識別と位置でグラフの根拠に一致する。
func TestAssertionResolverFindsTheRecordTargetByItsContentAndPosition(t *testing.T) {
	graph := graphOf(t)
	resolver := NewAssertionResolver(graph)
	locator := graph.records[0].locator

	present := assertionOf(core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord, Record: recordRefPointer(core.NewAssertionRecordRef(locator)),
	})
	if origin := resolver.Origin(present, noAddedRelations()); origin !=
		core.AssertionTargetOriginObservation {
		t.Errorf("Origin() of a record in the graph = %q, want %q",
			origin, core.AssertionTargetOriginObservation)
	}

	elsewhere := core.NewAssertionRecordRef(locator)
	elsewhere.SourceContentSha256 =
		"0000000000000000000000000000000000000000000000000000000000000000"
	absent := assertionOf(core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord, Record: &elsewhere,
	})
	if origin := resolver.Origin(absent, noAddedRelations()); origin !=
		core.AssertionTargetOriginAbsent {
		t.Errorf("Origin() of a record outside the graph = %q, want %q",
			origin, core.AssertionTargetOriginAbsent)
	}
}

// 位置の値を 1 つ除いた参照は、両方を持つレコードのどれにも一致しない。
//
// **応答は、位置の値が足りないことと、その位置のレコードが無いことを分けない。**
// どちらも出所が absent になる。本 test はその挙動を固定し、書き漏れと区別する
// (core.AssertionRecordRef の doc comment)。
func TestAssertionResolverRequiresEveryPositionTheSourceCarries(t *testing.T) {
	graph := graphOf(t)
	resolver := NewAssertionResolver(graph)

	locator := recordLocatorCarryingBothPositions(t, graph)
	whole := core.NewAssertionRecordRef(locator)

	// 両方を持つ参照は一致する。
	if origin := resolver.Origin(recordAssertionOf(whole), noAddedRelations()); origin !=
		core.AssertionTargetOriginObservation {
		t.Errorf("Origin() of a reference carrying both positions = %q, want %q",
			origin, core.AssertionTargetOriginObservation)
	}

	// 反対側。PositionKind が指さない位置を除いた参照は、検査を通りながら、
	// どのレコードにも一致しない。**書き漏れを通知せずに別のレコードへ一致させない。**
	partial := core.NewAssertionRecordRef(locator)
	partial.LineNumber = nil
	if partial.PositionKind != core.PositionKindSequenceNumber {
		t.Fatalf("the fixture record points by %q, want the test to drop the position "+
			"the kind does not name", partial.PositionKind)
	}
	if err := partial.Validate(); err != nil {
		t.Fatalf("Validate() of the reference without the line number = %v, "+
			"want the kind to require the sequence number alone", err)
	}
	if origin := resolver.Origin(recordAssertionOf(partial), noAddedRelations()); origin !=
		core.AssertionTargetOriginAbsent {
		t.Errorf("Origin() of a reference without the line number = %q, want %q",
			origin, core.AssertionTargetOriginAbsent)
	}

	// PositionKind が指す位置を除いた参照は、引き当ての前に検査が退ける。
	required := core.NewAssertionRecordRef(locator)
	required.SequenceNumber = nil
	if err := required.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("Validate() of the reference without the sequence number = %v, "+
			"want the required position to be rejected", err)
	}
}

// recordAssertionOf はレコードを対象にした所見を組む。
func recordAssertionOf(ref core.AssertionRecordRef) core.Assertion {
	return assertionOf(core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord, Record: &ref,
	})
}

// recordLocatorCarryingBothPositions は、通番と行番号の両方を持つ根拠のレコードを返す。
func recordLocatorCarryingBothPositions(t *testing.T, graph Graph) core.RecordLocator {
	t.Helper()
	for _, record := range graph.records {
		if record.locator.SequenceNumber != nil && record.locator.LineNumber != nil {
			return record.locator
		}
	}
	t.Fatal("the fixture graph carries no record with both a sequence number and a line number")
	return core.RecordLocator{}
}

// 取り込みをやり直して sourceId が変わっても、所見の対象は同じ対象を指す。
//
// 同じ器で 2 回取り込むと 2 回目は別の通番を受け取り、別の sourceId になる。ノードの
// 識別子と、収集元の内容の識別と位置で指すレコードの参照は、その変化に依らない。
func TestAssertionTargetsSurviveTheReimport(t *testing.T) {
	runner := graphRunner(t)
	firstResult := graphResultFrom(t, runner)
	secondResult := graphResultFrom(t, runner)

	firstSources := sourceIdsOf(t, firstResult)
	secondSources := sourceIdsOf(t, secondResult)
	if len(firstSources) != len(secondSources) {
		t.Fatalf("the two imports read %d and %d sources, want the same sources",
			len(firstSources), len(secondSources))
	}
	for index, sourceId := range firstSources {
		if sourceId == secondSources[index] {
			t.Fatalf("source %d kept the identifier %q, want the reimport to mint a new one",
				index, sourceId)
		}
	}

	firstGraph := NewGraph(firstResult, AllMatchConditions())
	secondGraph := NewGraph(secondResult, AllMatchConditions())
	firstSubgraph := wholeGraphOf(t, firstGraph)
	resolver := NewAssertionResolver(secondGraph)

	for _, node := range firstSubgraph.Nodes {
		assertion := assertionOf(core.AssertionTarget{
			Kind: core.AssertionTargetKindNode, NodeId: node.Id,
		})
		if origin := resolver.Origin(assertion, noAddedRelations()); origin !=
			core.AssertionTargetOriginObservation {
			t.Errorf("Origin() of the node %q after the reimport = %q, want %q",
				node.Id, origin, core.AssertionTargetOriginObservation)
		}
	}
	for _, edge := range firstSubgraph.Edges {
		want := core.AssertionTargetOriginObservation
		if edge.State == core.RelationStateCandidate {
			want = core.AssertionTargetOriginMatchingCandidate
		}
		assertion := assertionOf(edgeTarget(core.AssertionEdgeRef{
			Kind: edge.Kind, SourceNodeId: edge.SourceNodeId, TargetNodeId: edge.TargetNodeId,
		}))
		if origin := resolver.Origin(assertion, noAddedRelations()); origin != want {
			t.Errorf("Origin() of the edge %q after the reimport = %q, want %q",
				edge.Id, origin, want)
		}
	}
	locator := firstGraph.records[0].locator
	recordAssertion := assertionOf(core.AssertionTarget{
		Kind:   core.AssertionTargetKindRecord,
		Record: recordRefPointer(core.NewAssertionRecordRef(locator)),
	})
	if origin := resolver.Origin(recordAssertion, noAddedRelations()); origin !=
		core.AssertionTargetOriginObservation {
		t.Errorf("Origin() of the record after the reimport = %q, want %q",
			origin, core.AssertionTargetOriginObservation)
	}
}

// byte の範囲で位置を示す同じ収集元の 2 件のレコードは、別の鍵になる。
func TestAssertionRecordKeySeparatesTwoRecordsOfAByteRangeSource(t *testing.T) {
	first, second := int64(0), int64(350)
	ref := func(offset *int64) core.AssertionRecordRef {
		return core.AssertionRecordRef{
			SourceContentSha256: "0000000000000000000000000000000000000000000000000000000000000001",
			PositionKind:        core.PositionKindByteRange, ByteOffset: offset,
		}
	}
	if assertionRecordKeyOf(ref(&first)) == assertionRecordKeyOf(ref(&second)) {
		t.Error("two records at different byte offsets share one key")
	}
}

// sourceIdsOf は取り込み結果の収集元の識別子を、取り込みの入力順で返す。
func sourceIdsOf(t *testing.T, result ImportResult) []string {
	t.Helper()
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Identity.SourceId)
	}
	return ids
}

func edgeTarget(ref core.AssertionEdgeRef) core.AssertionTarget {
	return core.AssertionTarget{Kind: core.AssertionTargetKindEdge, Edge: &ref}
}

func recordRefPointer(ref core.AssertionRecordRef) *core.AssertionRecordRef { return &ref }

// noAddedRelations は、分析者が関係を 1 本も足していない状態である。
func noAddedRelations() AddedRelations { return NewAddedRelations(nil) }
