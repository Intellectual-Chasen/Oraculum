package core_test

import (
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// recordInputOf はレコード 1 件を指す段階の入力を返す。
func recordInputOf(locator core.RecordLocator) core.TrailInputRef {
	return core.TrailInputRef{Kind: core.TrailInputKindRecord, Record: &locator}
}

// 時刻を比べた段階を含む関連付けは、段階ごとに 1 つの段階を持つ。時計に依存しない段階は起点を入力に取り、
// 時刻以外の条件の両側の値を用いる。時刻の段階は候補を入力に取り、時刻の範囲と両端の時刻の文字列を用いる。
func TestDerivationTrailOfATimeMatchedMatch(t *testing.T) {
	leftTime, rightTime := squidRequestTime(t), markIICandidateTime(t, thirdCandidateSn)
	radius := int64(3)
	match := core.EdgeMatch{
		OriginRef: squidLocator(3), CandidateRef: markIILocator(thirdCandidateSn),
		StageKey:   core.StageKeySecondTimeMatched,
		Conditions: stageConditions(t, core.StageKeySecondTimeMatched, true),
		TimeWindow: core.TimeWindow{WindowKind: core.WindowKindSymmetricSeconds, RadiusSeconds: &radius},
		TimeComparison: core.TimeComparison{
			ComparisonUnit: core.ComparisonUnitSecond, LeftTime: &leftTime, RightTime: &rightTime,
		},
		StageTallies: []core.MatchStageTally{
			{StageKey: core.StageKeyClockIndependent, MemberCount: 5},
			{StageKey: core.StageKeySecondTimeMatched, MemberCount: 2},
		},
	}

	trail, err := core.DerivationTrailOfMatch(match)
	if err != nil {
		t.Fatal(err)
	}

	want := core.DerivationTrail{
		OriginRef: match.OriginRef,
		Steps: []core.TrailStep{
			{
				StepKey:   "clock_independent",
				InputRefs: []core.TrailInputRef{recordInputOf(match.OriginRef)},
				UsedIdentifiers: []string{
					"IP から端末への割当による端末: 起点 11111111-2222-3333-4444-555555555555、" +
						"候補 11111111-2222-3333-4444-555555555555",
					"接続先 IP: 起点 198.51.100.21、候補 198.51.100.21",
					"接続先 port: 起点 80、候補 80",
				},
				Output: "この段階が挙げた候補 5 件",
			},
			{
				StepKey:   "second_time_matched",
				InputRefs: []core.TrailInputRef{recordInputOf(match.CandidateRef)},
				UsedIdentifiers: []string{
					"秒単位の時刻 (前後 3 秒の範囲): 起点 [14/Mar/2024:10:20:30 +0900]、" +
						"候補 03/14/2024 10:40:15.736 +0900",
				},
				Output: "この段階が挙げた候補 2 件",
			},
		},
	}
	if !reflect.DeepEqual(trail, want) {
		t.Errorf("trail=%+v\nwant %+v", trail, want)
	}
}

// 時計に依存しない段階だけの関連付けは、起点と候補の両方を入力に取る 1 段階の経路になる。
func TestDerivationTrailOfAClockIndependentMatch(t *testing.T) {
	match := core.EdgeMatch{
		OriginRef: squidLocator(3), CandidateRef: markIILocator(7),
		StageKey: core.StageKeyClockIndependent,
		Conditions: []core.MatchCondition{
			destinationPortCondition(t, true),
			secondOfTimeCondition(t, core.StageKeyClockIndependent),
		},
		StageTallies: []core.MatchStageTally{
			{StageKey: core.StageKeyClockIndependent, MemberCount: 4},
		},
	}

	trail, err := core.DerivationTrailOfMatch(match)
	if err != nil {
		t.Fatal(err)
	}

	want := core.DerivationTrail{
		OriginRef: match.OriginRef,
		Steps: []core.TrailStep{{
			StepKey: "clock_independent",
			InputRefs: []core.TrailInputRef{
				recordInputOf(match.OriginRef), recordInputOf(match.CandidateRef),
			},
			UsedIdentifiers: []string{"接続先 port: 起点 80、候補 80"},
			Output:          "この段階が挙げた候補 4 件",
		}},
	}
	if !reflect.DeepEqual(trail, want) {
		t.Errorf("trail=%+v\nwant %+v", trail, want)
	}
}

// 関連付けが無い組は、渡した段階で止まる。止まった段階の出力は、組が外れた段階をグラフから特定できない
// ことを書く。
func TestUnmatchedDerivationTrailStopsAtTheGivenStage(t *testing.T) {
	tolerance := int64(0)
	origin := squidLocator(3)

	trail, err := core.UnmatchedDerivationTrail(origin, core.StageKeySecondTimeMatched,
		[]core.ConditionKey{core.ConditionKeyDestinationPort, core.ConditionKeySecondOfTime}, &tolerance)
	if err != nil {
		t.Fatal(err)
	}

	step := core.TrailStep{
		StepKey:   "second_time_matched",
		InputRefs: []core.TrailInputRef{recordInputOf(origin)},
		UsedIdentifiers: []string{
			"選んだ条件: 接続先 port", "選んだ条件: 秒単位の時刻 (同じ秒の範囲)",
		},
		Output: "選んだ条件では、起点のレコードからこのレコードへの関連付けが無い。" +
			"この組がどの段階で外れたかを、グラフから特定できない",
	}
	want := core.DerivationTrail{OriginRef: origin, Steps: []core.TrailStep{step}, StoppedAt: &step}
	if !reflect.DeepEqual(trail, want) {
		t.Errorf("trail=%+v\nwant %+v", trail, want)
	}
}
