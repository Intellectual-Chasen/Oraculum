package core

// AssumptionKey は関連付けが依拠する前提の種別を持つ。
type AssumptionKey string

// AssumptionKey の値。
const (
	// AssumptionKeyClockOffsetBelowOneSecond は 2 つの時計のずれが 1 秒未満である前提である。
	AssumptionKeyClockOffsetBelowOneSecond AssumptionKey = "clock_offset_below_one_second"
	// AssumptionKeyIpAssignmentHoldsInObservationGap は観測の切れ目でも IP 割当が続くという
	// 前提である。
	AssumptionKeyIpAssignmentHoldsInObservationGap AssumptionKey = "ip_assignment_holds_in_observation_gap"
	// AssumptionKeyCounterpartConnectedToProxy は、候補が記録した接続先が Proxy の収集元の
	// 端末の IP アドレスであるとき、その接続が Proxy への接続であるという前提である。
	AssumptionKeyCounterpartConnectedToProxy AssumptionKey = "counterpart_connected_to_proxy"
)

// IsKnown は AssumptionKey が定義の中の値であるかを返す。
func (a AssumptionKey) IsKnown() bool {
	switch a {
	case AssumptionKeyClockOffsetBelowOneSecond, AssumptionKeyIpAssignmentHoldsInObservationGap,
		AssumptionKeyCounterpartConnectedToProxy:
		return true
	default:
		return false
	}
}

// EvidenceClass は前提の根拠区分を持つ。値は 公式説明 / 実機確認 / 推測 / 未確認 に
// 1 対 1 で対応する。
type EvidenceClass string

// EvidenceClass の値。
const (
	// EvidenceClassOfficial は公式説明を根拠とする区分である。
	EvidenceClassOfficial EvidenceClass = "official"
	// EvidenceClassMeasured は実機確認を根拠とする区分である。
	EvidenceClassMeasured EvidenceClass = "measured"
	// EvidenceClassInferred は推測を根拠とする区分である。
	EvidenceClassInferred EvidenceClass = "inferred"
	// EvidenceClassUnconfirmed は未確認の区分である。
	EvidenceClassUnconfirmed EvidenceClass = "unconfirmed"
)

// IsKnown は EvidenceClass が定義の中の値であるかを返す。
func (e EvidenceClass) IsKnown() bool {
	switch e {
	case EvidenceClassOfficial, EvidenceClassMeasured, EvidenceClassInferred, EvidenceClassUnconfirmed:
		return true
	default:
		return false
	}
}

// MatchAssumption は関連付けが依拠する前提を持つ。
type MatchAssumption struct {
	// AssumptionKey は前提の種別である。
	AssumptionKey AssumptionKey `json:"assumptionKey"`
	// EvidenceClass は前提の根拠区分である。
	EvidenceClass EvidenceClass `json:"evidenceClass"`
	// Statement は前提の内容である。
	Statement string `json:"statement"`
	// UnresolvedReason は確定できなかった理由である。EvidenceClass が unconfirmed のとき必須。
	UnresolvedReason string `json:"unresolvedReason,omitempty"`
}

// Validate は項目の整合を確かめる。
func (a MatchAssumption) Validate() error {
	problem := firstProblem(
		requireKnownEnum("MatchAssumption.assumptionKey", a.AssumptionKey),
		requireKnownEnum("MatchAssumption.evidenceClass", a.EvidenceClass),
		requirePresent("MatchAssumption.statement", a.Statement),
	)
	if problem != nil {
		return problem
	}
	if a.EvidenceClass == EvidenceClassUnconfirmed {
		return requirePresent("MatchAssumption.unresolvedReason", a.UnresolvedReason)
	}
	return nil
}

// TimeComparisonAssumptions は段階の前提のうち、候補 1 件の時刻の比較が依拠する前提を返す。
// 候補の母集合を組むときの前提 (MatchRequest.CandidateAssumptions の種別) を外す。
func TimeComparisonAssumptions(stageAssumptions []MatchAssumption) []MatchAssumption {
	kept := make([]MatchAssumption, 0, len(stageAssumptions))
	for _, assumption := range stageAssumptions {
		if assumption.AssumptionKey != AssumptionKeyCounterpartConnectedToProxy {
			kept = append(kept, assumption)
		}
	}
	return kept
}

// validateAssumptions は前提の集合の要素を確かめる。
func validateAssumptions(item string, assumptions []MatchAssumption) error {
	return validateElements(item, assumptions)
}
