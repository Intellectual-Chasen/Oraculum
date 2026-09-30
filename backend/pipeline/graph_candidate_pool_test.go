// in-package test: 非公開の候補の母集合と、段階 1 の値の組で探す表を直接確かめる。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// poolOrigin は、全候補の母集合と、候補のエッジを挙げる起点 1 件の観測と条件の宣言を返す。
func poolOrigin(t *testing.T) (*candidatePool, core.MatchObservation, []core.MatchConditionSpec) {
	t.Helper()
	result := graphResult(t)
	origin, _, _ := secondStageOf(t, result)
	sides := collectCandidateSides(result)
	assignment, resolved := resolveOriginTerminal(
		clientAssignmentsOf(terminalAssignmentsOf(result)), origin)
	if !resolved {
		t.Fatal("the origin carries no determined terminal")
	}
	specs := composeSpecs(t, origin, sides.candidateDeclarations, AllMatchConditions())
	observation, built := originObservationOf(origin, assignment, specs)
	if !built {
		t.Fatal("the origin observation is not built")
	}
	return newCandidatePool(sides.candidates), observation, specs
}

// matchesComparedValues は、候補が段階 1 の比べる値をすべて起点と同じ文字列で持つかを返す。
func matchesComparedValues(
	origin core.MatchObservation, record core.MatchCandidateRecord, specs []core.MatchConditionSpec,
) bool {
	for _, spec := range comparedSpecsOf(specs) {
		left, leftOk := matchValueOf(origin, spec.OriginSemantic)
		right, rightOk := counterpartMatchValue(record.Observation, spec)
		if !leftOk || !rightOk || left != right {
			return false
		}
	}
	return true
}

// **段階 1 の値の組が起点と一致する候補だけを、母集合の並びで返す。** 一致しない候補がある
// 母集合で、一致する候補を 1 件も除かず、一致しない候補を 1 件も含めないことを確かめる。
func TestCandidateMembersNarrowToTheComparedValues(t *testing.T) {
	pool, origin, specs := poolOrigin(t)
	members := pool.membersFor(specs)
	var want []core.MatchCandidateRecord
	for _, record := range members.records {
		if matchesComparedValues(origin, record, specs) {
			want = append(want, record)
		}
	}
	if len(want) == 0 || len(want) == len(members.records) {
		t.Fatalf("%d of %d candidates match the origin, want both matching and other candidates",
			len(want), len(members.records))
	}
	narrowed := members.narrowedFor(origin, specs)
	if !narrowed.checked {
		t.Error("the narrowed candidates are not carried as validated candidates")
	}
	got := narrowed.records
	if len(got) != len(want) {
		t.Fatalf("narrowed to %d candidates, want %d", len(got), len(want))
	}
	for index := range want {
		if recordKeyOf(got[index].Observation.Ref) != recordKeyOf(want[index].Observation.Ref) {
			t.Errorf("the narrowed candidate %d is %q, want %q", index,
				recordKeyOf(got[index].Observation.Ref), recordKeyOf(want[index].Observation.Ref))
		}
	}
	if again := pool.membersFor(specs); again != members {
		t.Error("the second lookup with the same conditions built the members again")
	}
}

// **起点が段階 1 の比べる値を読めないときと、検査を通らない候補があるときは、全件を返す。**
// どちらも core が要求を退ける経路であり、退ける理由を core の検査に任せる。
func TestCandidateMembersReturnEveryRecordWhenTheyCannotNarrow(t *testing.T) {
	pool, origin, specs := poolOrigin(t)
	members := pool.membersFor(specs)
	unreadable := origin
	unreadable.Fields = nil
	for _, field := range origin.Fields {
		if field.Semantic != core.SemanticKeyConnectionDestinationPort {
			unreadable.Fields = append(unreadable.Fields, field)
		}
	}
	// **全件を返すときは検査済みとして渡さない。** core が候補を検査し、要求を退ける。
	if got := members.narrowedFor(unreadable, specs); len(got.records) != len(members.records) || got.checked {
		t.Errorf("an origin without the destination port narrowed to %d of %d candidates (checked=%v)",
			len(got.records), len(members.records), got.checked)
	}
	unindexed := &candidateMembers{records: members.records}
	if got := unindexed.narrowedFor(origin, specs); len(got.records) != len(members.records) || got.checked {
		t.Errorf("members without the index narrowed to %d of %d candidates (checked=%v)",
			len(got.records), len(members.records), got.checked)
	}
}

// **候補のエッジの終点を 1 候補につき 1 度だけ探し、探せない候補を分ける。**
func TestCandidatePoolLooksUpTheEndpointOnce(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	pool := newCandidatePool(collectCandidateSides(result).candidates)
	if len(pool.sides) == 0 {
		t.Fatal("the pool carries no candidate")
	}
	ref := pool.sides[0].record.Locator
	first, resolvable := pool.endpointOf(&graph, ref)
	if !resolvable {
		t.Fatal("the first candidate carries no endpoint")
	}
	target, candidateAt, _ := graph.candidateEndpointsOf(pool.sides[0])
	if first.target != target || first.candidateAt != candidateAt {
		t.Errorf("the endpoint is (%d, %d), want (%d, %d)",
			first.target, first.candidateAt, target, candidateAt)
	}
	if !pool.endpoints[0].looked {
		t.Error("the pool did not keep the looked-up endpoint")
	}
	if second, _ := pool.endpointOf(&graph, ref); second != first {
		t.Errorf("the second lookup returned %+v, want %+v", second, first)
	}
	absent := ref
	absent.SourceId = "absent-source"
	if _, found := pool.endpointOf(&graph, absent); found {
		t.Error("a locator outside the pool resolved to an endpoint")
	}
	var missing *candidatePool
	if _, found := missing.endpointOf(&graph, ref); found {
		t.Error("a missing pool resolved to an endpoint")
	}
}
