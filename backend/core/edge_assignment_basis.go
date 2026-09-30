package core

// EdgeAssignmentBasis は、接続元のアドレスから端末を導いて作った候補のエッジ 1 本の
// 成立の根拠である。
//
// **用いた割当ごとに 1 件である。** 同じ 2 つの端末を結ぶエッジを複数のレコードが
// 作るとき、根拠にした接続元のアドレスと割当の期間がレコードによって異なりうる。
// **後から組んだ根拠で先の根拠を上書きしない。** 根拠のレコードごとに変わる観測の種別と
// 時刻は GraphEvidence が持つ。
//
// 関連付けの段階を経ないため EdgeMatch を持たない。EdgeMatch は起点のレコード 1 件と候補の
// レコード 1 件の組を持つが、IP から端末への割当は収集元の観測期間から導き、特定の
// 1 レコードに帰属しない。
type EdgeAssignmentBasis struct {
	// ClientIp は端末を導くのに用いたアドレスの文字列である。
	ClientIp string `json:"clientIp"`
	// SourceId は用いた割当の適用期間を読み取った収集元である (TerminalAssignment.SourceId)。
	// 同じアドレスと端末の割当を、収集元ごとに見分ける。
	SourceId string `json:"sourceId"`
	// Conditions は成立に用いた条件の全数である。terminal_ip_assignment を必ず含む。
	Conditions []MatchCondition `json:"conditions"`
	// Assumptions は条件が依拠する前提の集合である。要素数 0 の場合も集合である。
	Assumptions []MatchAssumption `json:"assumptions"`
	// ClockDependencyNote は判定が収集元の時計に依拠する箇所である。
	ClockDependencyNote string `json:"clockDependencyNote"`
}

// Validate は項目の整合を確かめる。
func (b EdgeAssignmentBasis) Validate() error {
	problem := firstProblem(
		requirePresent("EdgeAssignmentBasis.clientIp", b.ClientIp),
		requirePresent("EdgeAssignmentBasis.sourceId", b.SourceId),
		requirePresent("EdgeAssignmentBasis.clockDependencyNote", b.ClockDependencyNote),
	)
	if problem != nil {
		return problem
	}
	if len(b.Conditions) == 0 {
		return itemError("EdgeAssignmentBasis.conditions", ErrMissingRequiredItem)
	}
	if problem := validateElements("EdgeAssignmentBasis.conditions", b.Conditions); problem != nil {
		return problem
	}
	if !carriesAssignmentCondition(b.Conditions) {
		return itemError("EdgeAssignmentBasis.conditions", ErrMissingRequiredItem)
	}
	return validateElements("EdgeAssignmentBasis.assumptions", b.Assumptions)
}

// carriesAssignmentCondition は条件の集合が IP から端末への割当の条件を含むかを返す。
func carriesAssignmentCondition(conditions []MatchCondition) bool {
	for _, condition := range conditions {
		if condition.ConditionKey == ConditionKeyTerminalIpAssignment {
			return true
		}
	}
	return false
}
