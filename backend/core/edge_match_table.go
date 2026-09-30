package core

import (
	"encoding/json"
	"fmt"
)

// EdgeMatchTable はエッジ 1 本の関連付けのすべてを、関連付けが共有する値を 1 度だけ置いて持つ。
// 関連付けの結果 1 件の値 (EdgeMatch) は Match が組み直す。
//
// **関連付けごとに段階の値とレコードの位置を繰り返さない。** 同じ起点の関連付けは、同じ段階の条件と、
// 同じ互いに区別できない候補の組を持つ。関連付けごとに書くと、エッジ 1 本の応答の大きさが関連付けの
// 数と候補の数の積で増える。段階とレコードは表の位置で指す。
type EdgeMatchTable struct {
	// MatchRecords は、関連付けと段階が指すレコードである。同じレコードを 2 度置かない。
	MatchRecords []EdgeMatchRecord `json:"matchRecords"`
	// MatchStages は、起点 1 件に対して候補を挙げた段階である。
	MatchStages []EdgeMatchStage `json:"matchStages"`
	// Matches は関連付けの結果 1 件ずつの値である。エッジが関連付けを並べた順に並べる。
	Matches []EdgeMatchRef `json:"matches"`
}

// EdgeMatchRecord は関連付けと段階が指すレコード 1 件である。
type EdgeMatchRecord struct {
	// Ref はレコードの位置である。
	Ref RecordLocator `json:"ref"`
	// EventTime はレコードの事象の時刻である。時刻を持たないレコードでは出ない。
	//
	// 時刻を比べた段階の関連付けは、起点と候補のこの値を比べた時刻として持つ (Match)。
	EventTime *Timestamp `json:"eventTime,omitempty"`
	// ProxyStatus は Proxy のログのレコードが記録した要求処理の結果の欄である (Squid の
	// %Ss、または %Ss:%Sh)。cache からの応答かを原文を開かずに読む。欄を持たないレコードでは出ない。
	ProxyStatus *RecordField `json:"proxyStatus,omitempty"`
}

// EdgeMatchStage は、起点 1 件に対して候補を挙げた段階と、その段階の関連付けが共有する値である。
// 項目の意味は EdgeMatch の同じ名前の項目と同じである。
type EdgeMatchStage struct {
	// Origin は起点のレコードの、EdgeMatchTable.MatchRecords での位置である。
	Origin              int               `json:"origin"`
	StageKey            StageKey          `json:"stageKey"`
	Conditions          []MatchCondition  `json:"conditions"`
	Assumptions         []MatchAssumption `json:"assumptions"`
	TimeWindow          TimeWindow        `json:"timeWindow"`
	ClockDependencyNote string            `json:"clockDependencyNote"`
	StageTallies        []MatchStageTally `json:"stageTallies"`
	// ComparisonUnit は、段階が起点と候補の時刻を比べた単位である。
	ComparisonUnit ComparisonUnit `json:"comparisonUnit"`
	// IndistinguishableGroups は互いに区別できない候補の組を、EdgeMatchTable.MatchRecords での
	// 位置で持つ。要素数 0 の場合も集合である。
	IndistinguishableGroups [][]int `json:"indistinguishableGroups"`
	// UnresolvedReasonSets は、段階の関連付けが持つ確定しない理由の異なりである。関連付けは位置で指す。
	UnresolvedReasonSets [][]string `json:"unresolvedReasonSets"`
}

// EdgeMatchRef は関連付けの結果 1 件である。段階とレコードと理由を位置で指す。
type EdgeMatchRef struct {
	// Stage は関連付けを出した段階の、EdgeMatchTable.MatchStages での位置である。
	Stage int `json:"stage"`
	// Candidate は候補のレコードの、EdgeMatchTable.MatchRecords での位置である。
	Candidate int `json:"candidate"`
	// UnresolvedReasons は確定しない理由の、段階の UnresolvedReasonSets での位置である。
	UnresolvedReasons int `json:"unresolvedReasons"`
	// TimeComparison は、段階と両端のレコードの時刻から組み直せない時刻の比較である。
	// 組み直せる関連付けでは出ない。組み直し方は Match が定める。
	TimeComparison *TimeComparison `json:"timeComparison,omitempty"`
}

// Match は位置 index の関連付けの結果 1 件の値を組み直す。
//
// 時刻の比較は、TimeComparison を持つ関連付けではその値である。持たない関連付けでは、段階の
// ComparisonUnit が second なら起点と候補の EventTime を段階の前提 (TimeComparisonAssumptions) で
// 比べた値であり、それ以外なら比べていない値 (前提の要素数 0) である。
//
// 位置が範囲の外であれば error を返す。
func (t EdgeMatchTable) Match(index int) (EdgeMatch, error) {
	if index < 0 || index >= len(t.Matches) {
		return EdgeMatch{}, itemError("EdgeMatchTable.matches at "+formatIndex(index), ErrInvalid)
	}
	ref := t.Matches[index]
	if err := t.checkRef(ref); err != nil {
		return EdgeMatch{}, itemError("EdgeMatchTable.matches at "+formatIndex(index), err)
	}
	stage := t.MatchStages[ref.Stage]
	groups := make([][]RecordLocator, 0, len(stage.IndistinguishableGroups))
	for _, group := range stage.IndistinguishableGroups {
		members := make([]RecordLocator, 0, len(group))
		for _, at := range group {
			members = append(members, t.MatchRecords[at].Ref)
		}
		groups = append(groups, members)
	}
	return EdgeMatch{
		OriginRef:               t.MatchRecords[stage.Origin].Ref,
		CandidateRef:            t.MatchRecords[ref.Candidate].Ref,
		StageKey:                stage.StageKey,
		Conditions:              stage.Conditions,
		Assumptions:             stage.Assumptions,
		TimeWindow:              stage.TimeWindow,
		TimeComparison:          t.timeComparisonOf(ref),
		ClockDependencyNote:     stage.ClockDependencyNote,
		StageTallies:            stage.StageTallies,
		IndistinguishableGroups: groups,
		UnresolvedReasons:       stage.UnresolvedReasonSets[ref.UnresolvedReasons],
	}, nil
}

// timeComparisonOf は関連付けの結果 1 件の時刻の比較を返す。ref の位置は checkRef が確かめ済みである。
func (t EdgeMatchTable) timeComparisonOf(ref EdgeMatchRef) TimeComparison {
	if ref.TimeComparison != nil {
		return *ref.TimeComparison
	}
	stage := t.MatchStages[ref.Stage]
	if stage.ComparisonUnit != ComparisonUnitSecond {
		return TimeComparison{ComparisonUnit: stage.ComparisonUnit, Assumptions: []MatchAssumption{}}
	}
	return TimeComparison{
		ComparisonUnit: ComparisonUnitSecond,
		LeftTime:       t.MatchRecords[stage.Origin].EventTime,
		RightTime:      t.MatchRecords[ref.Candidate].EventTime,
		Assumptions:    TimeComparisonAssumptions(stage.Assumptions),
	}
}

// checkRef は関連付けの結果 1 件の位置が表の範囲にあることを確かめる。
func (t EdgeMatchTable) checkRef(ref EdgeMatchRef) error {
	if ref.Stage < 0 || ref.Stage >= len(t.MatchStages) {
		return itemError("EdgeMatchRef.stage", ErrInvalid)
	}
	if ref.Candidate < 0 || ref.Candidate >= len(t.MatchRecords) {
		return itemError("EdgeMatchRef.candidate", ErrInvalid)
	}
	stage := t.MatchStages[ref.Stage]
	if ref.UnresolvedReasons < 0 || ref.UnresolvedReasons >= len(stage.UnresolvedReasonSets) {
		return itemError("EdgeMatchRef.unresolvedReasons", ErrInvalid)
	}
	if stage.Origin < 0 || stage.Origin >= len(t.MatchRecords) {
		return itemError("EdgeMatchStage.origin", ErrInvalid)
	}
	for groupIndex, group := range stage.IndistinguishableGroups {
		for _, at := range group {
			if at < 0 || at >= len(t.MatchRecords) {
				return itemError("EdgeMatchStage.indistinguishableGroups at "+formatIndex(groupIndex), ErrInvalid)
			}
		}
	}
	return nil
}

// Validate は項目の整合を確かめる。
//
// 段階とレコードは 1 度ずつ確かめ、関連付けは位置と時刻の比較を確かめる。すべての関連付けを Match で
// 組み直して EdgeMatch.Validate を通すことに加えて、関連付けが指さない段階とレコードと、関連付けが持つ
// 時刻の比較の単位が段階の単位と同じであることも確かめる。
func (t EdgeMatchTable) Validate() error {
	for index, record := range t.MatchRecords {
		if err := record.Ref.Validate(); err != nil {
			return itemError("EdgeMatchTable.matchRecords at "+formatIndex(index)+" ref", err)
		}
		if record.EventTime != nil {
			if err := record.EventTime.Validate(); err != nil {
				return itemError("EdgeMatchTable.matchRecords at "+formatIndex(index)+" eventTime", err)
			}
		}
	}
	for index, stage := range t.MatchStages {
		if err := t.validateStage(stage); err != nil {
			return itemError("EdgeMatchTable.matchStages at "+formatIndex(index), err)
		}
	}
	for index, ref := range t.Matches {
		if err := t.checkRef(ref); err != nil {
			return itemError("EdgeMatchTable.matches at "+formatIndex(index), err)
		}
		if err := t.timeComparisonOf(ref).Validate(); err != nil {
			return itemError("EdgeMatchTable.matches at "+formatIndex(index)+" timeComparison", err)
		}
		if ref.TimeComparison != nil && ref.TimeComparison.ComparisonUnit != t.MatchStages[ref.Stage].ComparisonUnit {
			return itemError("EdgeMatchTable.matches at "+formatIndex(index)+
				" timeComparison.comparisonUnit differs from the stage", ErrInconsistentValue)
		}
	}
	return nil
}

// validateStage は段階 1 つを確かめる。
func (t EdgeMatchTable) validateStage(stage EdgeMatchStage) error {
	if stage.Origin < 0 || stage.Origin >= len(t.MatchRecords) {
		return itemError("EdgeMatchStage.origin", ErrInvalid)
	}
	if err := validateMatchStageItems("EdgeMatchStage", stage.StageKey, stage.Conditions, stage.Assumptions,
		stage.TimeWindow, stage.ClockDependencyNote, stage.StageTallies); err != nil {
		return err
	}
	if problem := requireKnownEnum("EdgeMatchStage.comparisonUnit", stage.ComparisonUnit); problem != nil {
		return problem
	}
	for groupIndex, group := range stage.IndistinguishableGroups {
		if len(group) < minIndistinguishableGroupSize {
			return itemError("EdgeMatchStage.indistinguishableGroups at "+formatIndex(groupIndex),
				ErrMissingRequiredItem)
		}
		for _, at := range group {
			if at < 0 || at >= len(t.MatchRecords) {
				return itemError("EdgeMatchStage.indistinguishableGroups at "+formatIndex(groupIndex), ErrInvalid)
			}
		}
	}
	if len(stage.UnresolvedReasonSets) == 0 {
		return itemError("EdgeMatchStage.unresolvedReasonSets", ErrMissingRequiredItem)
	}
	for index, reasons := range stage.UnresolvedReasonSets {
		if err := validateUnresolvedReasons(
			"EdgeMatchStage.unresolvedReasonSets at "+formatIndex(index), reasons); err != nil {
			return err
		}
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (t EdgeMatchTable) MarshalJSON() ([]byte, error) {
	// items は EdgeMatchTable の method を持たないため、この Marshal は再帰しない。
	type items EdgeMatchTable
	copied := items(t)
	copied.MatchRecords = emptyIfNil(copied.MatchRecords)
	copied.MatchStages = emptyIfNil(copied.MatchStages)
	copied.Matches = emptyIfNil(copied.Matches)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling EdgeMatchTable: %w", err)
	}
	return encoded, nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (s EdgeMatchStage) MarshalJSON() ([]byte, error) {
	// items は EdgeMatchStage の method を持たないため、この Marshal は再帰しない。
	type items EdgeMatchStage
	copied := items(s)
	copied.Assumptions = emptyIfNil(copied.Assumptions)
	copied.IndistinguishableGroups = emptyIfNil(copied.IndistinguishableGroups)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling EdgeMatchStage: %w", err)
	}
	return encoded, nil
}
