// in-package test: 関連付けを位置と段階で持つ非公開の形と、組み直した core.EdgeMatch を突き合わせる。
package pipeline

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// matchTestSourceId はレコードの収集元の識別子である。
const matchTestSourceId = "src-match-test"

// matchTestLocator はレコードの位置を行番号で組む。
func matchTestLocator(line int64) core.RecordLocator {
	return core.RecordLocator{
		SourceId: matchTestSourceId, SourceContentSha256: "sha-match-test",
		SourceFileName: "match-test.log", PositionKind: core.PositionKindLineNumber,
		LineNumber: &line, RecordRawTextRef: fmt.Sprintf("raw-%d", line),
	}
}

// matchTestTime は事象の時刻を秒の値で組む。
func matchTestTime(t *testing.T, second int) *core.Timestamp {
	t.Helper()
	text := fmt.Sprintf("2024-03-14T10:20:%02d+09:00", second)
	rawText, normalized, offsetText := text, text, "+09:00"
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond, OffsetState: core.OffsetStateInValue,
		OffsetText: &offsetText,
		Clock:      core.ClockTerminalLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &timestamp
}

// matchTestGraph は行番号 1 から 3 のレコードとエッジ 1 本を持つグラフを組む。
// レコードの位置は行番号から 1 を引いた値である。
func matchTestGraph(t *testing.T) Graph {
	t.Helper()
	graph := Graph{recordAt: make(map[string]int), edgeAt: map[string]int{"e:match-test": 0}}
	for line := int64(1); line <= 3; line++ {
		locator := matchTestLocator(line)
		graph.records = append(graph.records, graphRecord{
			locator: locator, eventTime: matchTestTime(t, int(line)),
		})
		graph.recordAt[recordKeyOf(locator)] = len(graph.records) - 1
	}
	graph.edges = []graphEdge{{id: "e:match-test"}}
	return graph
}

// matchTestStage は時刻を比べる段階を、行番号 2 と 3 の候補を互いに区別できない組にして組む。
func matchTestStage() core.CandidateStage {
	return core.CandidateStage{
		StageKey:    core.StageKeySecondTimeMatched,
		Conditions:  []core.MatchCondition{{ConditionKey: core.ConditionKeyTerminalIpAssignment}},
		Assumptions: []core.MatchAssumption{{Statement: "synthetic assumption"}},
		IndistinguishableGroups: [][]core.RecordLocator{
			{matchTestLocator(2), matchTestLocator(3)},
		},
	}
}

// matchTestEnds は行番号 1 のレコードを起点にした、関連付けに共通する値である。
func matchTestEnds() candidateEdgeEnds {
	return candidateEdgeEnds{
		originAt: 0, originRef: matchTestLocator(1), clockDependencyNote: "synthetic note",
		stageTallies: []core.MatchStageTally{
			{StageKey: core.StageKeySecondTimeMatched, MemberCount: 2},
		},
	}
}

// matchTestCandidate は行番号 line の候補を、段階の規則どおりの時刻の比較と理由で組む。
func matchTestCandidate(
	t *testing.T, stage core.CandidateStage, line int64, reasons []string,
) core.Candidate {
	t.Helper()
	return core.Candidate{
		RecordRef: matchTestLocator(line),
		TimeComparison: core.TimeComparison{
			ComparisonUnit: core.ComparisonUnitSecond,
			LeftTime:       matchTestTime(t, 1), RightTime: matchTestTime(t, int(line)),
			Assumptions: stage.Assumptions,
		},
		UnresolvedReasons: reasons,
	}
}

// expectedEdgeMatch は関連付け 1 件を core.EdgeMatch で組む。組み直した値と突き合わせる相手である。
func expectedEdgeMatch(
	ends candidateEdgeEnds, stage core.CandidateStage, member core.Candidate,
) core.EdgeMatch {
	return core.EdgeMatch{
		OriginRef: ends.originRef, CandidateRef: member.RecordRef, StageKey: stage.StageKey,
		Conditions: stage.Conditions, Assumptions: stage.Assumptions, TimeWindow: stage.TimeWindow,
		TimeComparison: member.TimeComparison, ClockDependencyNote: ends.clockDependencyNote,
		StageTallies:            ends.stageTallies,
		IndistinguishableGroups: stage.IndistinguishableGroups,
		UnresolvedReasons:       member.UnresolvedReasons,
	}
}

// addMatchTestMembers は段階を足し、候補を行番号の位置のレコードとしてエッジへ足す。
func addMatchTestMembers(
	graph *Graph, ends candidateEdgeEnds, stage core.CandidateStage, members []core.Candidate,
	candidateAt []int,
) candidateEdgeEnds {
	ends.matchStage, ends.compactStage = graph.addMatchStage(ends, stage)
	for index, member := range members {
		graph.appendEdgeMatch(0, ends, stage, member, candidateAt[index])
	}
	return ends
}

// **位置と段階で持った関連付けから、元の core.EdgeMatch と同じ値を組み直す。** 位置の構造体を
// 関連付けごとに持たないことも確かめる。
func TestCompactMatchesRebuildTheEdgeMatches(t *testing.T) {
	graph := matchTestGraph(t)
	stage, ends := matchTestStage(), matchTestEnds()
	members := []core.Candidate{
		matchTestCandidate(t, stage, 2, []string{"synthetic reason"}),
		matchTestCandidate(t, stage, 3, []string{"synthetic reason"}),
	}
	ends = addMatchTestMembers(&graph, ends, stage, members, []int{1, 2})
	if !ends.compactStage {
		t.Fatal("the stage is not held by positions, want a compact stage")
	}
	if graph.edges[0].basis.exactMatches != nil {
		t.Errorf("the edge holds exact matches %+v, want none", graph.edges[0].basis.exactMatches)
	}
	want := []core.EdgeMatch{
		expectedEdgeMatch(ends, stage, members[0]), expectedEdgeMatch(ends, stage, members[1]),
	}
	if got := graph.edgeMatchesOf(graph.edges[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt matches are %+v, want %+v", got, want)
	}
	if got := graph.matchStages[ends.matchStage].groups; !reflect.DeepEqual(got, [][]int32{{1, 2}}) {
		t.Errorf("the stage groups are %v, want the record positions [[1 2]]", got)
	}
}

// **時刻を比べない段階の関連付けは、両端の時刻を持たない比較を組み直す。**
func TestCompactMatchOfAStageWithoutTimeComparison(t *testing.T) {
	graph := matchTestGraph(t)
	stage, ends := matchTestStage(), matchTestEnds()
	stage.StageKey = core.StageKeyClockIndependent
	stage.IndistinguishableGroups = [][]core.RecordLocator{}
	member := core.Candidate{
		RecordRef: matchTestLocator(2),
		TimeComparison: core.TimeComparison{
			ComparisonUnit: core.ComparisonUnitNotCompared, Assumptions: []core.MatchAssumption{},
		},
		UnresolvedReasons: []string{"synthetic reason"},
	}
	ends = addMatchTestMembers(&graph, ends, stage, []core.Candidate{member}, []int{1})
	if graph.edges[0].basis.exactMatches != nil {
		t.Errorf("the edge holds exact matches %+v, want none", graph.edges[0].basis.exactMatches)
	}
	want := []core.EdgeMatch{expectedEdgeMatch(ends, stage, member)}
	if got := graph.edgeMatchesOf(graph.edges[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt matches are %+v, want %+v", got, want)
	}
}

// **同じ段階の関連付けが別々の理由を持つとき、関連付けごとの理由を保つ。** nil と要素数 0 の集合は
// 応答で別の文字列になるため、別の理由として持つ。
func TestCompactMatchesKeepTheReasonsOfEachMatch(t *testing.T) {
	graph := matchTestGraph(t)
	stage, ends := matchTestStage(), matchTestEnds()
	members := []core.Candidate{
		matchTestCandidate(t, stage, 2, []string{"first reason"}),
		matchTestCandidate(t, stage, 3, []string{"second reason"}),
		matchTestCandidate(t, stage, 2, nil),
		matchTestCandidate(t, stage, 3, []string{}),
		matchTestCandidate(t, stage, 3, []string{"first reason"}),
	}
	ends = addMatchTestMembers(&graph, ends, stage, members, []int{1, 2, 1, 2, 2})
	got := graph.edgeMatchesOf(graph.edges[0])
	for index, member := range members {
		if !reflect.DeepEqual(got[index], expectedEdgeMatch(ends, stage, member)) {
			t.Errorf("the match at %d is %+v, want the reasons %#v", index, got[index],
				member.UnresolvedReasons)
		}
	}
	if got[2].UnresolvedReasons != nil || got[3].UnresolvedReasons == nil {
		t.Errorf("the reasons are %#v and %#v, want nil and an empty set",
			got[2].UnresolvedReasons, got[3].UnresolvedReasons)
	}
}

// **位置と段階から組み直せない関連付けを、core.EdgeMatch のまま持つ。** 候補の位置がグラフの
// レコードと食い違う関連付けと、時刻の比較が段階の規則と食い違う関連付けである。
func TestMatchesThatCannotBeRebuiltKeepTheExactForm(t *testing.T) {
	stage := matchTestStage()
	cases := map[string]func(t *testing.T) core.Candidate{
		"the candidate locator differs from the graph record": func(t *testing.T) core.Candidate {
			member := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
			member.RecordRef.RecordRawTextRef = "raw-other"
			return member
		},
		"the left time differs from the origin record": func(t *testing.T) core.Candidate {
			member := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
			member.TimeComparison.LeftTime = matchTestTime(t, 30)
			return member
		},
		"the comparison unit differs from the stage": func(t *testing.T) core.Candidate {
			member := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
			member.TimeComparison = core.TimeComparison{
				ComparisonUnit: core.ComparisonUnitNotCompared,
				Assumptions:    []core.MatchAssumption{},
			}
			return member
		},
	}
	for name, memberOf := range cases {
		t.Run(name, func(t *testing.T) {
			graph := matchTestGraph(t)
			ends := matchTestEnds()
			member := memberOf(t)
			ends = addMatchTestMembers(&graph, ends, stage, []core.Candidate{member}, []int{2})
			if _, exact := graph.edges[0].basis.exactMatches[0]; !exact {
				t.Fatal("the match is held by positions, want the exact form")
			}
			want := []core.EdgeMatch{expectedEdgeMatch(ends, stage, member)}
			if got := graph.edgeMatchesOf(graph.edges[0]); !reflect.DeepEqual(got, want) {
				t.Errorf("the matches are %+v, want %+v", got, want)
			}
		})
	}
}

// **起点と候補のレコードの位置から関連付けを探す。** 位置と段階で持つ関連付けも、core.EdgeMatch の
// まま持つ関連付けも探す。候補から起点への向きの組は関連付けを持たない。
func TestEdgeMatchBetweenFindsTheMatchOfTheTwoRecords(t *testing.T) {
	graph := matchTestGraph(t)
	graph.edges[0].kind = core.EdgeKindCrossSourceConnectionMatch
	stage, ends := matchTestStage(), matchTestEnds()
	compact := matchTestCandidate(t, stage, 2, []string{"synthetic reason"})
	exact := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
	exact.TimeComparison.LeftTime = matchTestTime(t, 30)
	ends = addMatchTestMembers(&graph, ends, stage, []core.Candidate{compact, exact}, []int{1, 2})
	if _, held := graph.edges[0].basis.exactMatches[1]; !held {
		t.Fatal("the second match is held by positions, want the exact form")
	}
	// 別の起点 (行番号 2) が同じ候補 (行番号 3) を挙げた関連付けを、位置と段階で持つ形で足す。
	// 候補の位置だけで比べると、起点が行番号 1 の組にこの関連付けを返してしまう。
	otherEnds := matchTestEnds()
	otherEnds.originAt, otherEnds.originRef = 1, matchTestLocator(2)
	other := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
	other.TimeComparison.LeftTime = matchTestTime(t, 2)
	otherEnds = addMatchTestMembers(&graph, otherEnds, stage, []core.Candidate{other}, []int{2})
	if _, held := graph.edges[0].basis.exactMatches[2]; held {
		t.Fatal("the match of the other origin is held in the exact form, want positions")
	}
	cases := []struct {
		origin int64
		member core.Candidate
		ends   candidateEdgeEnds
	}{
		{1, compact, ends}, {1, exact, ends}, {2, other, otherEnds},
	}
	for _, testCase := range cases {
		got, found, err := graph.EdgeMatchBetween(matchTestLocator(testCase.origin), testCase.member.RecordRef)
		if err != nil || !found {
			t.Fatalf("the match to %+v is not found (found %t, error %v)", testCase.member.RecordRef, found, err)
		}
		if want := expectedEdgeMatch(testCase.ends, stage, testCase.member); !reflect.DeepEqual(got, want) {
			t.Errorf("the match is %+v, want %+v", got, want)
		}
	}
	if _, found, err := graph.EdgeMatchBetween(matchTestLocator(2), matchTestLocator(1)); found || err != nil {
		t.Errorf("the reversed pair has a match (found %t, error %v), want none", found, err)
	}
	// exact の形で持つ関連付けの位置は、候補 0 と段階 0 の空の値を持つ。その値を関連付けと読まない。
	if _, found, err := graph.EdgeMatchBetween(matchTestLocator(1), matchTestLocator(1)); found || err != nil {
		t.Errorf("the empty position of the exact match reads as a match (found %t, error %v), want none", found, err)
	}
}

// **関連付け 1 件だけを組み直した値は、エッジの表全体から組み直した同じ位置の値と同じである。**
// 位置と段階で持つ関連付けも、core.EdgeMatch のまま持つ関連付けも同じである。
func TestEdgeMatchAtRebuildsTheSameMatchAsTheTable(t *testing.T) {
	graph := matchTestGraph(t)
	stage, ends := matchTestStage(), matchTestEnds()
	exact := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
	exact.TimeComparison.LeftTime = matchTestTime(t, 30)
	addMatchTestMembers(&graph, ends, stage, []core.Candidate{
		matchTestCandidate(t, stage, 2, []string{"first reason"}), exact,
		matchTestCandidate(t, stage, 3, []string{"second reason"}),
	}, []int{1, 2, 2})
	want := graph.edgeMatchesOf(graph.edges[0])
	if len(want) != 3 {
		t.Fatalf("the edge rebuilds %d matches, want the three added matches", len(want))
	}
	for index := range want {
		got, err := graph.edgeMatchAt(graph.edges[0], index)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want[index]) {
			t.Errorf("the match at %d is %+v, want %+v", index, got, want[index])
		}
	}
}

// **段階の並びと文字列の昇順が一致する。** 到達した経路は stepKey の文字列の昇順を段階の順として
// 確かめる (core.DerivationTrail.Validate)。段階を足してこの順が崩れると、2 段階以上の経路を組めない。
func TestStageKeysAreInLexicalOrder(t *testing.T) {
	keys := AllMatchConditions().stageKeys()
	if len(keys) < 2 || !slices.IsSorted(keys) {
		t.Errorf("the stages %v are not in lexical order, want every stage in ascending order", keys)
	}
}

// **関連付けの無い組の経路は、選択の最後の段階で止まる。** 時刻を比べる選択は時刻の段階、比べない
// 選択は時計に依存しない段階である。
func TestUnmatchedDerivationTrailStopsAtTheLastStageOfTheSelection(t *testing.T) {
	cases := map[string]struct {
		selection MatchConditionSelection
		want      core.StageKey
		used      []string
	}{
		"comparing time": {
			MatchConditionSelection{Conditions: []SelectedMatchCondition{
				{ConditionKey: core.ConditionKeyTerminalIpAssignment},
				{ConditionKey: core.ConditionKeySecondOfTime, Tolerance: 3},
			}},
			core.StageKeySecondTimeMatched,
			[]string{"選んだ条件: IP から端末への割当による端末", "選んだ条件: 秒単位の時刻 (前後 3 秒の範囲)"},
		},
		"without time": {
			MatchConditionSelection{Conditions: []SelectedMatchCondition{
				{ConditionKey: core.ConditionKeyDestinationPort},
			}},
			core.StageKeyClockIndependent,
			[]string{"選んだ条件: 接続先 port"},
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			// 経路の検査は内容の識別に 16 進 64 桁を求める。
			origin := matchTestLocator(1)
			origin.SourceContentSha256 = strings.Repeat("ab", 32)
			trail, err := testCase.selection.UnmatchedDerivationTrail(origin)
			if err != nil {
				t.Fatal(err)
			}
			if trail.StoppedAt == nil || trail.StoppedAt.StepKey != string(testCase.want) ||
				len(trail.Steps) != 1 || !reflect.DeepEqual(trail.Steps[0], *trail.StoppedAt) {
				t.Fatalf("trail=%+v want to stop at the only step %q", trail, testCase.want)
			}
			if !reflect.DeepEqual(trail.StoppedAt.UsedIdentifiers, testCase.used) {
				t.Errorf("usedIdentifiers=%q want %q", trail.StoppedAt.UsedIdentifiers, testCase.used)
			}
		})
	}
}

// **起点の位置と互いに区別できない候補の組をグラフのレコードから組み直せない段階は、段階を
// 足さず、その段階の関連付けをすべて core.EdgeMatch のまま持つ。**
func TestStagesThatCannotBeRebuiltKeepTheExactMatches(t *testing.T) {
	cases := map[string]func(ends *candidateEdgeEnds, stage *core.CandidateStage){
		"the origin locator differs from the graph record": func(
			ends *candidateEdgeEnds, _ *core.CandidateStage,
		) {
			ends.originRef = matchTestLocator(2)
		},
		"a group member is absent from the graph": func(
			_ *candidateEdgeEnds, stage *core.CandidateStage,
		) {
			stage.IndistinguishableGroups = [][]core.RecordLocator{
				{matchTestLocator(2), matchTestLocator(9)},
			}
		},
		"a group member differs from the graph record": func(
			_ *candidateEdgeEnds, stage *core.CandidateStage,
		) {
			other := matchTestLocator(3)
			other.SourceFileName = "other.log"
			stage.IndistinguishableGroups = [][]core.RecordLocator{{matchTestLocator(2), other}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			graph := matchTestGraph(t)
			stage, ends := matchTestStage(), matchTestEnds()
			change(&ends, &stage)
			member := matchTestCandidate(t, stage, 2, []string{"synthetic reason"})
			ends = addMatchTestMembers(&graph, ends, stage, []core.Candidate{member}, []int{1})
			if ends.compactStage || len(graph.matchStages) != 0 {
				t.Fatalf("the stage is held by positions (%d stages), want no stage",
					len(graph.matchStages))
			}
			want := []core.EdgeMatch{expectedEdgeMatch(ends, stage, member)}
			if got := graph.edgeMatchesOf(graph.edges[0]); !reflect.DeepEqual(got, want) {
				t.Errorf("the matches are %+v, want %+v", got, want)
			}
		})
	}
}

// **取り込み結果から組んだ候補のエッジは、関連付けをすべて位置と段階で持つ。** 値が元の関連付けと
// 同じであることは TestCandidateLayerMatchesTheRescanningDerivation が確かめる。
func TestCandidateEdgesOfAnImportHoldMatchesByPosition(t *testing.T) {
	graph := NewGraph(graphResult(t), AllMatchConditions())
	matched := 0
	for _, edge := range graph.edges[graph.observedEdgeCount:] {
		if len(edge.matchList()) == 0 {
			continue
		}
		matched++
		if edge.basis.exactMatches != nil {
			t.Errorf("the edge %q holds exact matches %+v, want none", edge.id,
				edge.basis.exactMatches)
		}
	}
	if matched == 0 {
		t.Fatal("no candidate edge carries a match, want the fixture to derive one")
	}
}

// 期間を時点として読めない割当 (解釈を持たない地方時の期間) が同じ接続元 IP にあるとき、
// 関連付けの起点の端末は 1 つに決まらない。起点の結果は terminal_undetermined であり、関連付けの
// 候補を作らない。FieldsBuilder の clientTerminal と同じ保守的な判定である。
func TestUncomparableAssignmentLeavesTheMatchingOriginUndetermined(t *testing.T) {
	result := graphResult(t)
	matchedEdges := func(graph Graph) []string {
		ids := []string{}
		for _, edge := range graph.edges[graph.observedEdgeCount:] {
			if len(edge.matchList()) > 0 {
				ids = append(ids, edge.id)
			}
		}
		return ids
	}
	undetermined := func(graph Graph) int {
		count := 0
		for _, outcome := range graph.originOutcomes {
			if outcome == core.RelationDerivationTerminalUndetermined {
				count++
			}
		}
		return count
	}
	baselineGraph := NewGraph(result, AllMatchConditions())
	if len(matchedEdges(baselineGraph)) == 0 || undetermined(baselineGraph) != 0 {
		t.Fatalf("the fixture derives %d candidate edges and %d undetermined origins, "+
			"want candidates and no undetermined origin", len(matchedEdges(baselineGraph)),
			undetermined(baselineGraph))
	}
	local := func(text string) core.Timestamp {
		t.Helper()
		value, err := core.NewTimestamp(core.Timestamp{
			RawText: &text, Normalized: &text, NormalizedForm: core.NormalizedFormLocalWithoutOffset,
			Precision: core.PrecisionSecond, OffsetState: core.OffsetStateItemAbsent,
			Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	publication := result.publications[0]
	assignment := core.TerminalAssignment{
		ClientIp: "192.0.2.1", TerminalId: "T9", SourceId: publication.status.SourceId,
		SourceContentSha256: publication.status.Scope.SourceContentSha256,
		AssignmentValidRange: core.TimeRange{
			From: local("2000-01-02T00:00:00"), To: local("2000-01-03T00:00:00"),
		},
		Origin: core.TerminalAssignmentOriginAnalystSupplied, Derivation: "合成の根拠", Author: "analyst",
		BasisRecordRefs: []core.AssertionRecordRef{
			core.NewAssertionRecordRef(publication.records[0].Locator),
		},
	}
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the local assignment: %v", err)
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	if got := matchedEdges(graph); len(got) != 0 {
		t.Errorf("the uncomparable assignment leaves the candidate edges %v, want none", got)
	}
	if undetermined(graph) == 0 {
		t.Errorf("the origins are %v, want the origins of the address undetermined", graph.originOutcomes)
	}
}

// 接続元 IP に一致する割当が 1 件で、その割当が端末の外部識別子を持たない起点は、割当を付けた
// 収集元を記録した端末のノードの鍵で候補の端末と比べる。起点は terminal_id_absent にも
// origin_item_unreadable にもならず、起点の端末の値は、その端末の鍵から導いた値である。
func TestAssignmentWithoutTerminalIdComparesTheRecordingTerminal(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	sides := collectCandidateSides(result)
	lookup := newCandidateLookup(sides, terminalAssignmentsOf(result), graph.matchSelection)
	wantTerminals := map[string]bool{}
	for clientIp, assignments := range lookup.assignments {
		// 端末の外部識別子を省けるのは利用者が入力した割当だけである。
		withoutId := assignments[0]
		withoutId.TerminalId = ""
		withoutId.AppliesToSourceId = withoutId.SourceId
		recording, _ := core.RecordingTerminalNodeKey(withoutId.SourceContentSha256)
		wantTerminal, _ := core.TerminalMatchValue(recording)
		wantTerminals[wantTerminal] = true
		withoutId.Origin = core.TerminalAssignmentOriginAnalystSupplied
		withoutId.Derivation, withoutId.Author = "合成の根拠", "analyst"
		withoutId.BasisRecordRefs = []core.AssertionRecordRef{
			core.NewAssertionRecordRef(sides.origins[0].record.Locator),
		}
		if err := withoutId.Validate(); err != nil {
			t.Fatalf("building the assignment without a terminal id: %v", err)
		}
		lookup.assignments[clientIp] = []core.TerminalAssignment{withoutId}
	}
	outcomes := map[core.RelationDerivationOutcome]int{}
	terminals := map[string]int{}
	for _, origin := range sides.origins {
		derivation := graph.prepareOriginDerivation(result, lookup, origin)
		outcomes[derivation.outcome]++
		if field, found := derivation.request.Origin.Observation.FieldBySemantic(
			core.SemanticKeyTerminalId); found {
			value, _ := field.Text.ComparableValue()
			terminals[value]++
		}
	}
	if outcomes[core.RelationDerivationTerminalIdAbsent] != 0 ||
		outcomes[core.RelationDerivationOriginItemUnreadable] != 0 || outcomes[core.RelationDerivationMatched] == 0 {
		t.Errorf("the origins are %v, want matched and neither terminal_id_absent nor origin_item_unreadable",
			outcomes)
	}
	for terminal := range terminals {
		if !wantTerminals[terminal] {
			t.Errorf("an origin compares the terminal %q, want one of %v", terminal, wantTerminals)
		}
	}
	if len(terminals) == 0 {
		t.Error("no origin carries a terminal value")
	}
}

// **関連付けを持たないエッジは関連付けの集合を nil で返す。**
func TestEdgeWithoutMatchesRebuildsNoMatch(t *testing.T) {
	graph := matchTestGraph(t)
	if got := graph.edgeMatchesOf(graph.edges[0]); got != nil {
		t.Errorf("the matches are %+v, want nil", got)
	}
}

// **int32 で表せる位置だけを位置で持つ。**
func TestCompactIndexAcceptsOnlyInt32Positions(t *testing.T) {
	cases := []struct {
		at   int
		want bool
	}{
		{at: -1, want: false}, {at: 0, want: true},
		{at: math.MaxInt32, want: true}, {at: math.MaxInt32 + 1, want: false},
	}
	for _, testCase := range cases {
		got, fits := compactIndex(testCase.at)
		if fits != testCase.want {
			t.Errorf("compactIndex(%d) fits = %v, want %v", testCase.at, fits, testCase.want)
		}
		if fits && int(got) != testCase.at {
			t.Errorf("compactIndex(%d) = %d, want the same position", testCase.at, got)
		}
	}
}
