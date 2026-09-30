package core

// terminalIdentityItemCount は端末の識別子の個数である。語彙の terminal.id の 1 件を比べる。
const terminalIdentityItemCount = 1

// maxSideValueCount は leftValue と rightValue の要素数の上限である。
const maxSideValueCount = 3

// maxTimeConditionLeftValueCount は時刻の 2 条件の leftValue の要素数の上限である。
// 起点のレコードの時刻 1 件で、段階の中で 1 つに定まる。
const maxTimeConditionLeftValueCount = 1

// ConditionKey は関連付けの条件の種別を持つ。
type ConditionKey string

// ConditionKey の値。
const (
	// ConditionKeyTerminalIpAssignment は、端末の外部識別子を持たないレコードの接続元 IP
	// から導いた端末が、相手のレコードが持つ端末の外部識別子と一致するかである。
	ConditionKeyTerminalIpAssignment ConditionKey = "terminal_ip_assignment"
	// ConditionKeyTerminalIdentityMatches は 2 つのレコードの端末の外部識別子
	// (語彙の terminal.id) の値が一致するかである。子のプロセスの関連付けが使う。
	ConditionKeyTerminalIdentityMatches ConditionKey = "terminal_identity_matches"
	// ConditionKeyParentProcessIdMatches は子のレコードが持つ親プロセスの識別子
	// (語彙の parent_process.id) の値が、親のレコードが持つプロセスの識別子
	// (語彙の process.id) の値と一致するかである。子のプロセスの関連付けが使う。
	ConditionKeyParentProcessIdMatches ConditionKey = "parent_process_id_matches"
	// ConditionKeyDestinationIp は 2 つのレコードの接続先 IP の文字列が一致するかである。
	ConditionKeyDestinationIp ConditionKey = "destination_ip"
	// ConditionKeyDestinationPort は 2 つのレコードの接続先 port の文字列が一致するかである。
	ConditionKeyDestinationPort ConditionKey = "destination_port"
	// ConditionKeyDestinationAuthority は 2 つのレコードの接続先 URL の authority が
	// 一致するかである。
	ConditionKeyDestinationAuthority ConditionKey = "destination_authority"
	// ConditionKeySecondOfTime は 2 つのレコードの時刻が秒単位で一致するかである。
	ConditionKeySecondOfTime ConditionKey = "second_of_time"
	// ConditionKeySubSecondOfTime は 2 つのレコードの時刻が秒未満まで一致するかである。
	ConditionKeySubSecondOfTime ConditionKey = "sub_second_of_time"
	// ConditionKeyClientPort は 2 つのレコードの接続元 port の文字列が一致するかである。
	ConditionKeyClientPort ConditionKey = "client_port"
	// ConditionKeyProcess は 2 つのレコードのプロセスが一致するかである。
	ConditionKeyProcess ConditionKey = "process"
	// ConditionKeyUser は 2 つのレコードの利用者が一致するかである。
	ConditionKeyUser ConditionKey = "user"
)

// KnownConditionKeys は契約が定める条件の種別を、本関数が並べた順で返す。
//
// **分析者が選べる条件の一覧はこの関数が定義元である。** 画面は取得した一覧を描く。
func KnownConditionKeys() []ConditionKey {
	return []ConditionKey{
		ConditionKeyTerminalIpAssignment,
		ConditionKeyTerminalIdentityMatches,
		ConditionKeyParentProcessIdMatches,
		ConditionKeyDestinationIp,
		ConditionKeyDestinationPort,
		ConditionKeyDestinationAuthority,
		ConditionKeySecondOfTime,
		ConditionKeySubSecondOfTime,
		ConditionKeyClientPort,
		ConditionKeyProcess,
		ConditionKeyUser,
	}
}

// IsKnown は ConditionKey が定義の中の値であるかを返す。
func (c ConditionKey) IsKnown() bool {
	for _, known := range KnownConditionKeys() {
		if c == known {
			return true
		}
	}
	return false
}

// requiredSideValueCount は conditionKey ごとの leftValue と rightValue の要素数を返す。
// 0 は要素数を固定しないことを表す。
//
// IP から端末への割当と、端末の識別子の一致は別の conditionKey で表すため、要素数も別である。
func (c ConditionKey) requiredSideValueCount() int {
	switch c {
	case ConditionKeyTerminalIdentityMatches:
		return terminalIdentityItemCount
	case ConditionKeyParentProcessIdMatches:
		return 1
	default:
		return 0
	}
}

// isTimeCondition は候補の側の時刻が候補 1 件ごとに変わる 2 つの条件であるかを返す。
func (c ConditionKey) isTimeCondition() bool {
	return c == ConditionKeySecondOfTime || c == ConditionKeySubSecondOfTime
}

// ConditionUse は条件の使い方を持つ。
type ConditionUse string

// ConditionUse の値。
const (
	// ConditionUseUsed は段階が条件を用いた状態である。
	ConditionUseUsed ConditionUse = "used"
	// ConditionUseNotUsed は段階が条件を用いなかった状態である。
	ConditionUseNotUsed ConditionUse = "not_used"
	// ConditionUseNoComparableCounterpart は比べられる相手の値が無い状態である。
	ConditionUseNoComparableCounterpart ConditionUse = "no_comparable_counterpart"
	// ConditionUseItemAbsentOnCounterpart は相手の入力形式に欄が無い状態である。
	ConditionUseItemAbsentOnCounterpart ConditionUse = "item_absent_on_counterpart"
)

// IsKnown は ConditionUse が定義の中の値であるかを返す。
func (c ConditionUse) IsKnown() bool {
	switch c {
	case ConditionUseUsed, ConditionUseNotUsed, ConditionUseNoComparableCounterpart,
		ConditionUseItemAbsentOnCounterpart:
		return true
	default:
		return false
	}
}

// MatchConditionSpec は段階が持つ条件 1 件と、両側が取り出す語彙の項目の宣言である。
//
// **どの条件を段階が持つかを本 package が決めない。** 条件の一覧は入力形式の組ごとに
// 変わるため、入力形式を読む adapter の binding が宣言し、関連付けを実行する側が組んで
// MatchRequest が持つ。
type MatchConditionSpec struct {
	// ConditionKey は条件の種別である。
	ConditionKey ConditionKey
	// OriginSemantic は起点の側が取り出す語彙の項目である。
	OriginSemantic SemanticKey
	// CounterpartSemantics は候補の側が取り出す語彙の項目である。
	//
	// 起点と候補の入力形式が異なると、同じ意味を別の語彙の項目で持つ。相手が同じ意味を
	// 2 つ以上の語彙の項目で持つこともあるため、集合で受け取る。要素数 1 以上である。
	CounterpartSemantics []SemanticKey
	// Compared は段階がこの条件で候補を絞るかである。真の要素だけが use に used を取る。
	Compared bool
}

// Validate は宣言 1 件の項目の整合を確かめる。
func (s MatchConditionSpec) Validate() error {
	if problem := requireKnownEnum("MatchConditionSpec.conditionKey", s.ConditionKey); problem != nil {
		return problem
	}
	if s.OriginSemantic == "" {
		return itemError("MatchConditionSpec.originSemantic of the "+
			string(s.ConditionKey)+" condition", ErrMissingRequiredItem)
	}
	if len(s.CounterpartSemantics) == 0 {
		return itemError("MatchConditionSpec.counterpartSemantics of the "+
			string(s.ConditionKey)+" condition", ErrMissingRequiredItem)
	}
	seen := make(map[SemanticKey]struct{}, len(s.CounterpartSemantics))
	for _, semantic := range s.CounterpartSemantics {
		if semantic == "" {
			return itemError("MatchConditionSpec.counterpartSemantics of the "+
				string(s.ConditionKey)+" condition has an empty element", ErrMissingRequiredItem)
		}
		if _, duplicate := seen[semantic]; duplicate {
			return itemError("MatchConditionSpec.counterpartSemantics of the "+
				string(s.ConditionKey)+" condition repeats "+string(semantic), ErrDuplicateElement)
		}
		seen[semantic] = struct{}{}
	}
	return nil
}

// MatchCondition は関連付けに用いた条件 1 件を持つ。
type MatchCondition struct {
	ConditionKey ConditionKey `json:"conditionKey"`
	Use          ConditionUse `json:"use"`
	// LeftValue は起点の側の値の集合である。Use が used のとき必須で、要素数は 1 以上である。
	// MemberCount が 0 のときも出す。起点の側の値は候補の有無に依らず定まる。
	LeftValue []RecordField `json:"leftValue,omitempty"`
	// RightValue は候補の側の値の集合である。Use が used かつ ConditionKey が時刻の 2 条件の
	// いずれでもなく、かつ同じ段階または同じ応答の MemberCount が 1 以上のとき必須である。
	// 要素数は LeftValue の要素数と等しい。
	RightValue []RecordField `json:"rightValue,omitempty"`
	// AssignmentValidRange は IP から端末への割当を適用してよい期間である。
	// ConditionKey が terminal_ip_assignment のとき必須。
	AssignmentValidRange *TimeRange `json:"assignmentValidRange,omitempty"`
	// OutsideAssignmentRange は起点のレコードが AssignmentValidRange の外にあるかである。
	// 段階に 1 件の判定であり、候補ごとに変わらない。
	// ConditionKey が terminal_ip_assignment のとき必須。
	OutsideAssignmentRange *bool `json:"outsideAssignmentRange,omitempty"`
}

// Validate は項目の整合を確かめる。
//
// RightValue の必須は同じ段階または同じ応答の memberCount に依るため、本 method では
// 確かめない。確かめるのは ValidateMatchConditions である。
func (c MatchCondition) Validate() error {
	problem := firstProblem(
		requireKnownEnum("MatchCondition.conditionKey", c.ConditionKey),
		requireKnownEnum("MatchCondition.use", c.Use),
	)
	if problem != nil {
		return problem
	}
	if c.Use == ConditionUseUsed && len(c.LeftValue) == 0 {
		return itemError("MatchCondition.leftValue", ErrMissingRequiredItem)
	}
	// rightValue を禁じる条件は 2 つである。1 つ目は時刻の条件で、
	// 候補の側の時刻を Candidate の timeComparison が持つ。2 つ目は memberCount が 0 の
	// 段階と応答で、ValidateMatchConditions が確かめる。
	// use が used でない条件の rightValue を一律に拒否しない。rightValue が必須になるのは
	// 時刻の条件以外で use が used かつ候補が 1 件以上ある場合である。
	if c.ConditionKey.isTimeCondition() && len(c.RightValue) > 0 {
		return itemError("MatchCondition.rightValue is present for the time condition "+
			string(c.ConditionKey), ErrInconsistentValue)
	}
	// 時刻の条件の leftValue は起点のレコードの時刻 1 件で、段階の中で 1 つに定まる。
	if c.ConditionKey.isTimeCondition() && len(c.LeftValue) > maxTimeConditionLeftValueCount {
		return itemError("MatchCondition.leftValue for the time condition "+
			string(c.ConditionKey), ErrTooManyElements)
	}
	if problem := c.validateSideValues(); problem != nil {
		return problem
	}
	return c.validateAssignmentRange()
}

// validateSideValues は左右の値の要素の整合と、要素数の一致を確かめる。
func (c MatchCondition) validateSideValues() error {
	sides := []struct {
		item   string
		values []RecordField
	}{
		{"MatchCondition.leftValue", c.LeftValue},
		{"MatchCondition.rightValue", c.RightValue},
	}
	for _, side := range sides {
		// 条件が比べる項目の個数の上限は 3 である。
		if len(side.values) > maxSideValueCount {
			return itemError(side.item, ErrTooManyElements)
		}
		seenNames := make(map[string]struct{}, len(side.values))
		for index, value := range side.values {
			if err := value.Validate(); err != nil {
				return itemError(side.item+" at "+formatIndex(index), err)
			}
			// 時刻の条件の値は precision を伴う形で持つ。
			if c.ConditionKey.isTimeCondition() && value.Kind != RecordFieldKindTimestamp {
				return itemError(side.item+" at "+formatIndex(index)+
					" carries a time condition without a timestamp", ErrInconsistentValue)
			}
			// 1 つの条件が同じ項目を 2 回比べる形にしない。
			if _, duplicate := seenNames[value.Name]; duplicate {
				return itemError(side.item+" has a repeated name "+value.Name, ErrDuplicateElement)
			}
			seenNames[value.Name] = struct{}{}
			if problem := c.validateTerminalIdentitySemantic(side.item, index, value); problem != nil {
				return problem
			}
		}
	}
	// 要素数を固定する conditionKey は、固定した個数だけを受ける。
	if fixed := c.ConditionKey.requiredSideValueCount(); fixed > 0 {
		if len(c.LeftValue) > 0 && len(c.LeftValue) != fixed {
			return itemError("MatchCondition.leftValue element count for "+
				string(c.ConditionKey), ErrInconsistentValue)
		}
	}
	if len(c.RightValue) > 0 && len(c.RightValue) != len(c.LeftValue) {
		return itemError("MatchCondition.rightValue element count differs from "+
			"MatchCondition.leftValue", ErrInconsistentValue)
	}
	return nil
}

// validateTerminalIdentitySemantic は端末の識別子の一致が比べる項目の意味を確かめる。
//
// 比べるのは語彙の terminal.id を持つ項目である。
// **入力形式ごとの key の文字列で確かめない。** 端末の表示名とレジストリの識別子は
// 一意性の根拠を持たず、同じ表示名を持つ別の端末を 1 件にまとめる向きに働く。
func (c MatchCondition) validateTerminalIdentitySemantic(
	item string, index int, value RecordField,
) error {
	if c.ConditionKey != ConditionKeyTerminalIdentityMatches {
		return nil
	}
	if value.Semantic != SemanticKeyTerminalId {
		return itemError(item+" at "+formatIndex(index)+" carries the semantic "+
			string(value.Semantic)+" instead of "+string(SemanticKeyTerminalId),
			ErrInconsistentValue)
	}
	return nil
}

// validateAssignmentRange は assignmentValidRange と outsideAssignmentRange の出現を
// conditionKey と突き合わせる。
//
// terminal_identity_matches と parent_process_id_matches に 2 項目を持たせない。同じ収集元の
// 中の親子の関連付けでは IP から端末への割当を適用しない。
func (c MatchCondition) validateAssignmentRange() error {
	if c.ConditionKey == ConditionKeyTerminalIpAssignment {
		if c.AssignmentValidRange == nil {
			return itemError("MatchCondition.assignmentValidRange", ErrMissingRequiredItem)
		}
		if c.OutsideAssignmentRange == nil {
			return itemError("MatchCondition.outsideAssignmentRange", ErrMissingRequiredItem)
		}
	}
	if c.ConditionKey == ConditionKeyTerminalIdentityMatches ||
		c.ConditionKey == ConditionKeyParentProcessIdMatches {
		if c.AssignmentValidRange != nil || c.OutsideAssignmentRange != nil {
			return itemError("MatchCondition.assignmentValidRange and "+
				"MatchCondition.outsideAssignmentRange are present for "+
				string(c.ConditionKey), ErrInconsistentValue)
		}
	}
	if c.AssignmentValidRange != nil {
		if err := c.AssignmentValidRange.Validate(); err != nil {
			return itemError("MatchCondition.assignmentValidRange", err)
		}
	}
	return nil
}

// ValidateMatchConditions は条件の集合と、候補の総数に依る RightValue の必須を確かめる。
//
// 候補が 1 件も無い段階と応答には比較した候補の側の値が存在しないため、memberCount が 0 の
// とき RightValue を出さない。候補の側の値を zero value と空文字列で埋めて出す形と、use を
// used から別の値へ書き換えて必須を避ける形のどちらも作らない。
func ValidateMatchConditions(conditions []MatchCondition, memberCount int64) error {
	if len(conditions) == 0 {
		return itemError("MatchCondition set", ErrMissingRequiredItem)
	}
	for index, condition := range conditions {
		if err := condition.Validate(); err != nil {
			return itemError("MatchCondition set at "+formatIndex(index), err)
		}
		// memberCount が 0 の段階と応答は、use の値に依らず rightValue を出さない。
		if memberCount == 0 && len(condition.RightValue) > 0 {
			return itemError("MatchCondition set at "+formatIndex(index)+
				" carries rightValue while memberCount is 0", ErrInconsistentValue)
		}
		if condition.Use != ConditionUseUsed || condition.ConditionKey.isTimeCondition() {
			continue
		}
		if memberCount > 0 && len(condition.RightValue) == 0 {
			return itemError("MatchCondition set at "+formatIndex(index)+
				" omits rightValue while memberCount is 1 or more", ErrMissingRequiredItem)
		}
	}
	return nil
}
