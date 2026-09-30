// in-package test: 関連付けの表を組む edgeMatchTableOf は非公開であり、表から関連付けを組み直して読む。
package pipeline

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// edgeMatchesOf は、edgeMatchTableOf の表からエッジ 1 本の関連付けを並び順のまま組み直す。関連付けを
// 持たないエッジでは nil を返す。
//
// 表が関連付けを組み直せないときは panic する。組み直せない表は edgeMatchTableOf の誤りであり、
// 呼び出した test を失敗させる。
func (g Graph) edgeMatchesOf(edge graphEdge) []core.EdgeMatch {
	return expandedMatches(g.edgeMatchTableOf(edge))
}

// detailMatches はエッジの詳細の関連付けを、並び順のまま組み直す。
func detailMatches(t *testing.T, detail EdgeDetail) []core.EdgeMatch {
	t.Helper()
	if err := detail.MatchTable.Validate(); err != nil {
		t.Fatalf("the match table does not validate: %v", err)
	}
	return expandedMatches(detail.MatchTable)
}

func expandedMatches(table core.EdgeMatchTable) []core.EdgeMatch {
	if len(table.Matches) == 0 {
		return nil
	}
	matches := make([]core.EdgeMatch, len(table.Matches))
	for index := range table.Matches {
		match, err := table.Match(index)
		if err != nil {
			panic(fmt.Sprintf("the match table cannot rebuild the match %d: %v", index, err))
		}
		matches[index] = match
	}
	return matches
}

// 組み直せない関連付けも、グラフのレコードと同じ位置は位置と段階で持つ関連付けと同じレコードを指し、
// グラフに無い位置は同じ値ごとに 1 度だけ置く。
func TestExactMatchesShareTheRecordsOfTheTable(t *testing.T) {
	graph := matchTestGraph(t)
	stage, ends := matchTestStage(), matchTestEnds()
	compact := matchTestCandidate(t, stage, 2, []string{"synthetic reason"})
	// 時刻の比較が段階の規則と食い違う関連付けは、位置と段階で持てない。
	shifted := matchTestCandidate(t, stage, 2, []string{"synthetic reason"})
	shifted.TimeComparison.LeftTime = matchTestTime(t, 30)
	// グラフのレコードと食い違う位置の候補は、同じ値の 2 件が同じレコードを指す。
	outside := matchTestCandidate(t, stage, 3, []string{"synthetic reason"})
	outside.RecordRef.RecordRawTextRef = "raw-other"
	ends = addMatchTestMembers(&graph, ends, stage, []core.Candidate{compact, shifted, outside, outside},
		[]int{1, 1, 2, 2})
	if len(graph.edges[0].basis.exactMatches) != 3 {
		t.Fatalf("the edge holds %d exact matches, want the shifted and the two outside matches",
			len(graph.edges[0].basis.exactMatches))
	}
	// レコードの位置は sha256 の形を持たないため、Validate を通さず位置の関係を確かめる。
	table := graph.edgeMatchTableOf(graph.edges[0])
	matches := table.Matches
	if matches[1].Candidate != matches[0].Candidate {
		t.Errorf("the exact match points at the record %d, the compact match at %d", matches[1].Candidate,
			matches[0].Candidate)
	}
	if matches[2].Candidate != matches[3].Candidate || matches[2].Candidate == matches[0].Candidate {
		t.Errorf("the outside matches point at the records %d and %d", matches[2].Candidate, matches[3].Candidate)
	}
	if want := []core.EdgeMatch{
		expectedEdgeMatch(ends, stage, compact), expectedEdgeMatch(ends, stage, shifted),
		expectedEdgeMatch(ends, stage, outside), expectedEdgeMatch(ends, stage, outside),
	}; !reflect.DeepEqual(expandedMatches(table), want) {
		t.Errorf("the table rebuilds %+v, want %+v", expandedMatches(table), want)
	}
}

// 表は同じ段階と同じレコードを 1 度ずつ置き、関連付けはその位置を指す。
func TestEdgeMatchTablePlacesSharedValuesOnce(t *testing.T) {
	graph := NewGraph(graphResult(t), AllMatchConditions())
	tabled := 0
	for _, edge := range graph.edges {
		table := graph.edgeMatchTableOf(edge)
		if len(table.Matches) == 0 {
			continue
		}
		tabled++
		if err := table.Validate(); err != nil {
			t.Fatalf("the table of %q does not validate: %v", edge.id, err)
		}
		seen := make(map[string]bool, len(table.MatchRecords))
		for _, record := range table.MatchRecords {
			key := recordKeyOf(record.Ref)
			if seen[key] {
				t.Errorf("the table of %q places the record %q twice", edge.id, key)
			}
			seen[key] = true
		}
		// 位置と段階で持つ関連付けはグラフの段階を共有し、組み直せない関連付けは関連付けごとに段階を置く。
		stages := make(map[int32]bool)
		for index, match := range edge.matchList() {
			if _, exact := edge.basis.exactMatches[index]; !exact {
				stages[match.stage] = true
			}
		}
		if want := len(stages) + len(edge.basis.exactMatches); len(table.MatchStages) != want {
			t.Errorf("the table of %q places %d stages, want %d", edge.id, len(table.MatchStages), want)
		}
	}
	if tabled == 0 {
		t.Fatal("no candidate edge carries a match, want the fixture to derive one")
	}
}
