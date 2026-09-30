package pipeline

import (
	"fmt"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// SelectedMatchCondition は、分析者が候補の絞り込みに用いると決めた条件 1 件である。
type SelectedMatchCondition struct {
	// ConditionKey は条件の種別である。
	ConditionKey core.ConditionKey
	// Tolerance はこの条件が値の一致に認める幅である。**意味は条件の種別が決める。**
	// 時刻の条件では秒であり、幅 0 は同じ秒だけを取る。幅を定めない条件は 0 だけを取る。
	Tolerance int64
}

// MatchConditionSelection は、候補の絞り込みにどの条件をどの幅で用いるかの選択である。
//
// **時刻に特化した項目を置かない。** 時刻は条件の種別の 1 つであり、他の条件と同じ 1 件の
// 要素として選ぶ。条件の種別を増やしたときに、選択の形を変えずに済む。
//
// **選ばなかった条件も結果から消えない。** 段階が持つ条件の一覧は全数であり、選ばなかった
// 条件は use が not_used の要素として応答に残る。分析者は「その条件で絞っていない」ことを
// 読める。
type MatchConditionSelection struct {
	// Conditions は用いる条件である。並びは要求が与えた順である。
	Conditions []SelectedMatchCondition
}

// AllMatchConditions は、契約が定めるすべての条件を幅 0 で選んだ選択を返す。
//
// 収集元が「候補を絞れる」と挙げていない条件は、選んでも絞りに使われない
// (candidateConditionSpecs)。
func AllMatchConditions() MatchConditionSelection {
	keys := core.KnownConditionKeys()
	conditions := make([]SelectedMatchCondition, 0, len(keys))
	for _, key := range keys {
		conditions = append(conditions, SelectedMatchCondition{ConditionKey: key})
	}
	return MatchConditionSelection{Conditions: conditions}
}

// toleratedConditions は、値の一致に幅を取れる条件の種別と、その幅の単位の名前である。
//
// **幅を読まない条件に幅を与えた要求を通さない。** 通すと、指定が作用しないまま候補が
// 変わらず、分析者は幅を広げたと読む。
var toleratedConditions = map[core.ConditionKey]string{
	core.ConditionKeySecondOfTime: "秒",
}

// Validate は選んだ条件の種別と幅を確かめる。
func (s MatchConditionSelection) Validate() error {
	seen := make(map[core.ConditionKey]struct{}, len(s.Conditions))
	for _, condition := range s.Conditions {
		if !condition.ConditionKey.IsKnown() {
			return fmt.Errorf("the match condition %q is outside the contract: %w",
				string(condition.ConditionKey), core.ErrInvalid)
		}
		if _, duplicated := seen[condition.ConditionKey]; duplicated {
			return fmt.Errorf("the match condition %q is selected more than once: %w",
				string(condition.ConditionKey), core.ErrInconsistentValue)
		}
		seen[condition.ConditionKey] = struct{}{}
		if condition.Tolerance < 0 {
			return fmt.Errorf("the tolerance of the match condition %q is negative: %w",
				string(condition.ConditionKey), core.ErrInvalid)
		}
		if _, tolerated := toleratedConditions[condition.ConditionKey]; !tolerated &&
			condition.Tolerance != 0 {
			return fmt.Errorf("the match condition %q takes no tolerance: %w",
				string(condition.ConditionKey), core.ErrInconsistentValue)
		}
	}
	return nil
}

// narrows は、その条件で候補を絞るかと、認める幅を返す。
func (s MatchConditionSelection) narrows(key core.ConditionKey) (int64, bool) {
	for _, condition := range s.Conditions {
		if condition.ConditionKey == key {
			return condition.Tolerance, true
		}
	}
	return 0, false
}

// destinationIpUse は、起点が接続先 IP の条件をどう扱うかを返す。hostnameOnly は、起点が
// 接続先を要求先のホスト名だけで指すかである (namesHostnameOnly)。
//
// **起点の名乗りと段階の条件を揃える。** 選択から外した条件を起点が used と名乗ると、
// core が要求を食い違いとして退け、その起点は候補を 1 件も挙げられなくなる。
//
// **用いない理由を 2 つに分ける。** 条件を選んだ要求でホスト名だけを持つ起点は
// no_comparable_counterpart、条件を選ばない要求の起点は not_used である。
func (s MatchConditionSelection) destinationIpUse(hostnameOnly bool) core.ConditionUse {
	if _, chosen := s.narrows(core.ConditionKeyDestinationIp); !chosen {
		return core.ConditionUseNotUsed
	}
	if hostnameOnly {
		return core.ConditionUseNoComparableCounterpart
	}
	return core.ConditionUseUsed
}

// comparesTime は、時刻の一致で候補を絞るかと、認める幅を秒で返す。
func (s MatchConditionSelection) comparesTime() (int64, bool) {
	return s.narrows(core.ConditionKeySecondOfTime)
}

// stageKeys は、この選択で関連付けに求める段階を返す。
//
// **時刻の条件を選ばない関連付けは段階 2 を求めない。** 段階 2 は時刻の一致で絞る段階であり、
// 時刻を比べない要求を段階 2 として組むと core が退ける。
func (s MatchConditionSelection) stageKeys() []core.StageKey {
	if _, compared := s.comparesTime(); !compared {
		return []core.StageKey{core.StageKeyClockIndependent}
	}
	return []core.StageKey{core.StageKeyClockIndependent, core.StageKeySecondTimeMatched}
}

// lastStageKey は、候補をエッジへ写す段階の種別を返す。
func (s MatchConditionSelection) lastStageKey() core.StageKey {
	keys := s.stageKeys()
	return keys[len(keys)-1]
}

// UnmatchedDerivationTrail は、この選択で起点のレコードから関連付けの無いレコードを開いたときの
// 経路を、選択の最後の段階で止まった形で組む。
func (s MatchConditionSelection) UnmatchedDerivationTrail(
	origin core.RecordLocator,
) (core.DerivationTrail, error) {
	keys := make([]core.ConditionKey, 0, len(s.Conditions))
	for _, condition := range s.Conditions {
		keys = append(keys, condition.ConditionKey)
	}
	var tolerance *int64
	if value, compared := s.comparesTime(); compared {
		tolerance = &value
	}
	return core.UnmatchedDerivationTrail(origin, s.lastStageKey(), keys, tolerance)
}

// assumptions は、この選択で挙げた候補が依拠する前提を返す。
//
// **幅を広げたら前提も変わる。** 幅 5 秒で挙げた候補に「ずれが 1 秒未満である」を添えると、
// 成り立っていない前提を根拠として示すことになる。
//
// **時刻を比べない関連付けは時計の前提を持たない。** 時計に依拠しない関連付けへ時計の前提を
// 添えると、根拠の読み方を誤らせる。
func (s MatchConditionSelection) assumptions() []core.MatchAssumption {
	tolerance, compared := s.comparesTime()
	if !compared {
		return []core.MatchAssumption{}
	}
	statement := clockOffsetAssumptionStatement
	if tolerance > 0 {
		statement = fmt.Sprintf("2 つの収集元の時計のずれが %d 秒以下である", tolerance)
	}
	return []core.MatchAssumption{{
		AssumptionKey:    core.AssumptionKeyClockOffsetBelowOneSecond,
		EvidenceClass:    core.EvidenceClassUnconfirmed,
		Statement:        statement,
		UnresolvedReason: clockOffsetUnresolvedReason,
	}}
}

// windowOf は起点のレコードの時刻から、この選択の時刻の範囲を組む。
// ok が偽になるのは、中心を要する時刻の範囲で、起点の時刻が絶対時刻の正規化値も分析者のずれ
// (Timestamp.Interpretation) で読める時点も持たないとき、または組んだ時刻の範囲が検査を通らない
// ときである。
//
// **時刻の範囲の中心は起点のレコードの時刻である。** 絶対時刻の両端で指す時刻の範囲は、起点ごとに中心の
// 変わる関連付けの時刻の範囲にならない。期間で根拠を絞り込むのは要求の時刻の条件である。
func (s MatchConditionSelection) windowOf(eventTime core.Timestamp) (core.TimeWindow, bool) {
	tolerance, compared := s.comparesTime()
	if !compared {
		return core.TimeWindow{WindowKind: core.WindowKindNotCompared}, true
	}
	normalized, present := eventTime.NormalizedValue()
	requestText, derivation := normalized, "起点のレコードの時刻の正規化値"
	// 分析者がずれを与えた地方時は、そのずれで読んだ時点を中心にする。正規化値の文字列は
	// 要求の文字列として残す。
	if eventTime.Interpretation != nil {
		instant, ok := eventTime.Instant()
		if !ok {
			return core.TimeWindow{}, false
		}
		normalized = instant.Format(time.RFC3339Nano)
		derivation = "起点のレコードの時刻の正規化値を、分析者が記録した UTC からのずれ " +
			string(eventTime.Interpretation.Offset) + " で読んだ値"
	} else if !present || eventTime.NormalizedForm != core.NormalizedFormRFC3339Absolute {
		return core.TimeWindow{}, false
	}
	center := core.RequestedTime{
		RequestText:    requestText,
		Precision:      eventTime.Precision,
		OffsetState:    eventTime.OffsetState,
		Normalized:     normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Derivation:     derivation,
	}
	window := core.TimeWindow{WindowKind: core.WindowKindSameSecond, CenterTime: &center}
	if tolerance > 0 {
		window.WindowKind = core.WindowKindSymmetricSeconds
		radius := tolerance
		window.RadiusSeconds = &radius
	}
	if window.Validate() != nil {
		return core.TimeWindow{}, false
	}
	return window, true
}
