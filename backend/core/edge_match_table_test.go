package core_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// edgeMatchTable は edgeMatch(t) の関連付けの結果 1 件を持つ表と、その関連付けを返す。
// レコードは起点、区別できない候補の組の 2 件の順に置き、候補は組の 2 件目である。
func edgeMatchTable(t *testing.T) (core.EdgeMatchTable, core.EdgeMatch) {
	t.Helper()
	match := edgeMatch(t)
	if !reflect.DeepEqual(match.CandidateRef, match.IndistinguishableGroups[0][1]) {
		t.Fatal("the fixture candidate is not the second member of the group")
	}
	table := core.EdgeMatchTable{
		MatchRecords: []core.EdgeMatchRecord{
			{Ref: match.OriginRef, EventTime: match.TimeComparison.LeftTime},
			{Ref: match.IndistinguishableGroups[0][0]},
			{Ref: match.CandidateRef, EventTime: match.TimeComparison.RightTime},
		},
		MatchStages: []core.EdgeMatchStage{{
			Origin: 0, StageKey: match.StageKey, Conditions: match.Conditions, Assumptions: match.Assumptions,
			TimeWindow: match.TimeWindow, ClockDependencyNote: match.ClockDependencyNote,
			StageTallies: match.StageTallies, ComparisonUnit: match.TimeComparison.ComparisonUnit,
			IndistinguishableGroups: [][]int{{1, 2}},
			UnresolvedReasonSets:    [][]string{{"another reason"}, match.UnresolvedReasons},
		}},
		Matches: []core.EdgeMatchRef{{Stage: 0, Candidate: 2, UnresolvedReasons: 1}},
	}
	return table, match
}

// 表から組み直した関連付けは、段階と両端のレコードの時刻から比べた時刻を含めて元の関連付けと同じである。
func TestEdgeMatchTableRebuildsTheMatch(t *testing.T) {
	table, want := edgeMatchTable(t)
	requireValid(t, "a complete table", table)
	got, err := table.Match(0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt match is\n%+v\nwant\n%+v", got, want)
	}
	if _, err := table.Match(1); err == nil {
		t.Error("a position outside the matches rebuilt a match")
	}

	// 時刻を比べない段階の関連付けは、前提を持たない比べていない比較になる。
	notCompared := table
	notCompared.MatchStages = []core.EdgeMatchStage{table.MatchStages[0]}
	notCompared.MatchStages[0].ComparisonUnit = core.ComparisonUnitNotCompared
	got, err = notCompared.Match(0)
	if err != nil {
		t.Fatal(err)
	}
	if got.TimeComparison.ComparisonUnit != core.ComparisonUnitNotCompared || got.TimeComparison.LeftTime != nil ||
		got.TimeComparison.Assumptions == nil || len(got.TimeComparison.Assumptions) != 0 {
		t.Errorf("the not compared stage gives %+v", got.TimeComparison)
	}

	// 関連付けが持つ比較は、段階とレコードから組んだ値より先に採る。
	carried := core.TimeComparison{ComparisonUnit: core.ComparisonUnitSecond, LeftTime: want.TimeComparison.RightTime,
		RightTime: want.TimeComparison.LeftTime, Assumptions: want.TimeComparison.Assumptions}
	overriding := table
	overriding.Matches = []core.EdgeMatchRef{{Stage: 0, Candidate: 2, UnresolvedReasons: 1, TimeComparison: &carried}}
	requireValid(t, "a table whose match carries a comparison of the stage unit", overriding)
	got, err = overriding.Match(0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.TimeComparison, carried) {
		t.Errorf("the carried comparison became %+v", got.TimeComparison)
	}
}

func TestEdgeMatchTableValidateSeparatesTheCompleteTableFromTheBrokenOne(t *testing.T) {
	complete, _ := edgeMatchTable(t)
	requireValid(t, "an empty table", core.EdgeMatchTable{})

	cases := map[string]func(*core.EdgeMatchTable){
		"a broken record":            func(table *core.EdgeMatchTable) { table.MatchRecords[1].Ref = core.RecordLocator{} },
		"an origin outside":          func(table *core.EdgeMatchTable) { table.MatchStages[0].Origin = 3 },
		"a group member outside":     func(table *core.EdgeMatchTable) { table.MatchStages[0].IndistinguishableGroups = [][]int{{1, 3}} },
		"a group of one":             func(table *core.EdgeMatchTable) { table.MatchStages[0].IndistinguishableGroups = [][]int{{1}} },
		"a stage without reasons":    func(table *core.EdgeMatchTable) { table.MatchStages[0].UnresolvedReasonSets = nil },
		"an empty reason":            func(table *core.EdgeMatchTable) { table.MatchStages[0].UnresolvedReasonSets = [][]string{{""}} },
		"an unknown comparison unit": func(table *core.EdgeMatchTable) { table.MatchStages[0].ComparisonUnit = "minute" },
		"a stage without tallies":    func(table *core.EdgeMatchTable) { table.MatchStages[0].StageTallies = nil },
		"a stage outside":            func(table *core.EdgeMatchTable) { table.Matches[0].Stage = 1 },
		"a candidate outside":        func(table *core.EdgeMatchTable) { table.Matches[0].Candidate = -1 },
		"a reason set outside":       func(table *core.EdgeMatchTable) { table.Matches[0].UnresolvedReasons = 2 },
		// 時刻を比べた段階は、時刻を持たない候補や起点との比較を組めない。
		"a compared candidate without a time": func(table *core.EdgeMatchTable) { table.Matches[0].Candidate = 1 },
		"a compared origin without a time":    func(table *core.EdgeMatchTable) { table.MatchRecords[0].EventTime = nil },
		"a broken event time": func(table *core.EdgeMatchTable) {
			table.MatchRecords[1].EventTime = &core.Timestamp{}
		},
		"a broken carried comparison": func(table *core.EdgeMatchTable) {
			table.Matches[0].TimeComparison = &core.TimeComparison{ComparisonUnit: core.ComparisonUnitSecond}
		},
		"a carried comparison of another unit": func(table *core.EdgeMatchTable) {
			table.Matches[0].TimeComparison = &core.TimeComparison{
				ComparisonUnit: core.ComparisonUnitNotCompared, Assumptions: []core.MatchAssumption{},
			}
		},
	}
	for name, breakTable := range cases {
		t.Run(name, func(t *testing.T) {
			broken := complete
			broken.MatchRecords = append([]core.EdgeMatchRecord(nil), complete.MatchRecords...)
			broken.MatchStages = append([]core.EdgeMatchStage(nil), complete.MatchStages...)
			broken.Matches = append([]core.EdgeMatchRef(nil), complete.Matches...)
			breakTable(&broken)
			requireInvalid(t, name, broken)
		})
	}
}

// 必須の集合は要素数 0 の場合も集合として出る。
func TestEdgeMatchTableWritesEmptySets(t *testing.T) {
	encoded, err := json.Marshal(core.EdgeMatchTable{MatchStages: []core.EdgeMatchStage{{}}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		MatchRecords []json.RawMessage `json:"matchRecords"`
		Matches      []json.RawMessage `json:"matches"`
		MatchStages  []struct {
			Assumptions             []json.RawMessage `json:"assumptions"`
			IndistinguishableGroups []json.RawMessage `json:"indistinguishableGroups"`
		} `json:"matchStages"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.MatchRecords == nil || decoded.Matches == nil || decoded.MatchStages[0].Assumptions == nil ||
		decoded.MatchStages[0].IndistinguishableGroups == nil {
		t.Errorf("an empty set was written as null: %s", encoded)
	}
}
