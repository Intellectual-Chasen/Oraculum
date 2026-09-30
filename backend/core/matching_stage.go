package core

import (
	"strconv"
	"strings"
)

// buildStage は 1 つの段階を組む。
//
// clockIndependentCount は時計に依存しない条件で絞った後の候補の総数である。段階 2 が
// 要素数 0 になった理由を、段階 1 の絞り込みの結果と時刻の範囲の絞り込みの結果で分けるために使う。
func (r MatchRequest) buildStage(
	stageKey StageKey, members []MatchCandidateRecord, clockIndependentCount int,
) (CandidateStage, error) {
	memberCount := int64(len(members))
	conditions, err := r.stageConditions(stageKey, members)
	if err != nil {
		return CandidateStage{}, err
	}
	returned, err := r.buildMembers(stageKey, members, conditions)
	if err != nil {
		return CandidateStage{}, err
	}
	groups := r.indistinguishableGroups(stageKey, members)
	// 要素数 0 の集合を出さない。項目が出ない状態は、候補の側の入力形式に観測の種別の欄が
	// 無い状態を表す。
	var selectors []ObservationKindSelector
	if len(r.IncludedObservationKinds) > 0 {
		selectors = r.IncludedObservationKinds
	}
	stage := CandidateStage{
		StageKey:                    stageKey,
		Conditions:                  conditions,
		IncludedObservationKinds:    selectors,
		Assumptions:                 append([]MatchAssumption{}, r.CandidateAssumptions...),
		TimeWindow:                  TimeWindow{WindowKind: WindowKindNotCompared},
		ComparisonUnit:              ComparisonUnitNotCompared,
		Members:                     returned,
		MemberCount:                 memberCount,
		DistinctProcessCount:        distinctProcessCount(members),
		IndistinguishableGroups:     groups,
		IndistinguishableGroupCount: int64(len(groups)),
		EmptyReason:                 r.stageEmptyReason(stageKey, memberCount, clockIndependentCount),
	}
	if stageKey == StageKeyClockIndependent {
		stage.ClockDependencyNote = r.ClockDependencyNote
		return stage, nil
	}
	stage.Assumptions = append(stage.Assumptions, r.SecondStageAssumptions...)
	stage.TimeWindow = r.SecondStageWindow
	stage.ComparisonUnit = ComparisonUnitSecond
	return stage, nil
}

// stageEmptyReason は段階が要素数 0 になった理由を返す。要素が 1 件以上ある段階では空を返す。
//
// 段階 1 に no_candidate_in_window を置かない。段階 1 は 2 つの入力のレコードどうしの時刻を
// 比べておらず、比べていない時刻の範囲を候補の不在の理由として持たない。段階 2 が 0 件である応答の
// うち、段階 1 の絞り込みで既に 0 件になったものは段階 1 と同じ理由を持つ。
//
// counterpart_item_absent を返さない。要求は段階が絞りに用いる条件を 1 件以上挙げるため
// (MatchRequest.validateConditions)、関連付けに使える条件が 1 つも無い段階が起こらない。
func (r MatchRequest) stageEmptyReason(
	stageKey StageKey, memberCount int64, clockIndependentCount int,
) EmptyReason {
	if memberCount > 0 {
		return ""
	}
	if stageKey == StageKeyClockIndependent || clockIndependentCount == 0 {
		return EmptyReasonNoCandidateMatchingConditions
	}
	return EmptyReasonNoCandidateInWindow
}

// buildMembers は段階が挙げた候補を全件組む。
func (r MatchRequest) buildMembers(
	stageKey StageKey, members []MatchCandidateRecord, conditions []MatchCondition,
) ([]Candidate, error) {
	reasons := unresolvedReasonsOfConditions(conditions)
	built := make([]Candidate, 0, len(members))
	for index, record := range members {
		candidate, err := r.buildCandidate(stageKey, record, reasons)
		if err != nil {
			return nil, itemError("CandidateStage.members at "+formatIndex(index), err)
		}
		built = append(built, candidate)
	}
	return built, nil
}

// buildCandidate は候補 1 件を組む。relationState は candidate の 1 値だけを取る。
func (r MatchRequest) buildCandidate(
	stageKey StageKey, record MatchCandidateRecord, stageReasons []string,
) (Candidate, error) {
	reasons := make([]string, 0, len(stageReasons)+len(record.UnresolvedReasons))
	reasons = append(reasons, stageReasons...)
	reasons = append(reasons, record.UnresolvedReasons...)
	candidate := Candidate{
		RecordRef:              record.Observation.Ref,
		EventTime:              record.Observation.EventTime,
		TimeComparison:         TimeComparison{ComparisonUnit: ComparisonUnitNotCompared, Assumptions: []MatchAssumption{}},
		FailedRecordDependency: record.FailedRecordDependency,
		ProcessRef:             record.ProcessRef,
		ObservationKind:        record.Observation.ObservationKind,
		RelationState:          RelationStateCandidate,
		UnresolvedReasons:      reasons,
	}
	if stageKey == StageKeySecondTimeMatched {
		leftTime := r.Origin.Observation.EventTime
		rightTime := record.Observation.EventTime
		candidate.TimeComparison = TimeComparison{
			ComparisonUnit: ComparisonUnitSecond,
			LeftTime:       &leftTime,
			RightTime:      &rightTime,
			Assumptions:    r.SecondStageAssumptions,
		}
	}
	if problem := candidate.Validate(); problem != nil {
		return Candidate{}, itemError("building a candidate of the "+string(stageKey)+" stage",
			problem)
	}
	return candidate, nil
}

// distinctProcessCount は候補が指すプロセスの個数を返す。
// 候補がプロセスの項目を 1 つも持たないとき nil を返す。
func distinctProcessCount(records []MatchCandidateRecord) *int64 {
	seen := make(map[ProcessRef]struct{}, len(records))
	for _, record := range records {
		if record.ProcessRef != nil {
			seen[*record.ProcessRef] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	count := int64(len(seen))
	return &count
}

// indistinguishableGroups は、用いた条件の値がすべて同じ候補の組を返す。
//
// 要素が 1 件の組を作らない。組が表すのは互いに区別できない要素の集まりであり、要素が
// 1 件の組には区別できない相手がいない。並び順を順位として読まない。
func (r MatchRequest) indistinguishableGroups(
	stageKey StageKey, records []MatchCandidateRecord,
) [][]RecordLocator {
	order := make([]string, 0, len(records))
	members := make(map[string][]RecordLocator, len(records))
	for _, record := range records {
		key := r.indistinguishableKey(stageKey, record)
		if _, seen := members[key]; !seen {
			order = append(order, key)
		}
		members[key] = append(members[key], record.Observation.Ref)
	}
	groups := make([][]RecordLocator, 0, len(order))
	for _, key := range order {
		if len(members[key]) >= minIndistinguishableGroupSize {
			groups = append(groups, members[key])
		}
	}
	return groups
}

// indistinguishableKeySeparator は組の鍵の中で値を区切る文字列である。
//
// 用いる値は原資料の文字列と正規化値であり、どちらも改行を含まない。
const indistinguishableKeySeparator = "\n"

// unreadableValueMarker は、比べられる値を持っていない項目を組の鍵に置く文字列である。
// 段階が用いた条件の値は候補の絞り込みで既に読めているため、組を作る経路ではこの文字列が
// 現れない。原資料の文字列が持てない byte を選び、読めた値と衝突しない形にしている。
const unreadableValueMarker = "\x00"

// indistinguishableKey は 1 件の候補について、段階が用いた条件の値を 1 つの文字列にまとめる。
func (r MatchRequest) indistinguishableKey(stageKey StageKey, record MatchCandidateRecord) string {
	specs := r.comparedSpecs()
	parts := make([]string, 0, len(specs)+1)
	for _, spec := range specs {
		value, ok := counterpartValue(record.Observation, spec)
		if !ok {
			value = unreadableValueMarker
		}
		parts = append(parts, value)
	}
	if stageKey != StageKeySecondTimeMatched {
		return strings.Join(parts, indistinguishableKeySeparator)
	}
	second := unreadableValueMarker
	if instant, ok := record.Observation.EventTime.Instant(); ok {
		second = strconv.FormatInt(truncateToSecond(instant).Unix(), 10)
	}
	return strings.Join(append(parts, second), indistinguishableKeySeparator)
}
