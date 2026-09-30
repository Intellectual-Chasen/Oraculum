package core

import "strings"

// terminalMatchValueSeparator は、外部識別子を持たない端末の鍵の形と値を連ねる文字列である。
const terminalMatchValueSeparator = "/"

// TerminalMatchValue は、関連付けの terminal_ip_assignment の条件が両側の端末を比べる値を、
// 端末のノードの識別鍵から返す。起点の側は割当が指す端末の鍵、候補の側はレコードを置いた
// 端末の鍵 (RecordTerminalNodeKey) を渡す。
//
// **比べるのは端末のノードである。** 同じ鍵の 2 つの端末は同じ値になり、別の鍵は別の値になる。
// 端末の外部識別子の鍵は識別子の文字列そのものを返す。レコードが名乗った外部識別子と同じ文字列に
// なり、分析者が条件の値を読める。外部識別子を持たない端末の鍵 (収集元を記録した端末と、
// 収集元とホスト名の組の端末) は、鍵の形と値を連ねた文字列を返す。
//
// ok が偽になるのは、端末の鍵でないときと、値を持たない鍵である。
func TerminalMatchValue(key NodeKey) (string, bool) {
	if key.Kind != NodeKindTerminal || len(key.Values) == 0 {
		return "", false
	}
	if key.Form == NodeKeyFormTerminalId {
		return key.Values[0].Value, key.Values[0].Value != ""
	}
	parts := make([]string, 0, len(key.Values)+1)
	parts = append(parts, string(key.Form))
	for _, value := range key.Values {
		parts = append(parts, value.Value)
	}
	return strings.Join(parts, terminalMatchValueSeparator), true
}

// comparableValue はレコードの 1 項目から、文字列の一致を比べる値を返す。
//
// 正規化値を持つ項目は正規化値を返し、原文字列だけを持つ項目は原文字列を返す。導出値は
// 正規化値だけを持つため、同じ経路で比べられる。ok が偽になるのは、値がある状態と導出できた
// 状態のどちらでもない項目と、時刻を持つ項目である。時刻を比べるのは Candidate の timeComparison である。
func comparableValue(field RecordField) (string, bool) {
	if field.Kind != RecordFieldKindText || field.Text == nil {
		return "", false
	}
	switch field.Text.ValueState {
	case ValueStatePresent, ValueStateDerived:
	default:
		return "", false
	}
	if normalized, ok := field.Text.NormalizedValue(); ok {
		return normalized, true
	}
	return field.Text.RawTextValue()
}

// conditionDisplayName は確定しない理由と到達した経路の文に載せる条件の呼び名を返す。
//
// 分析者が画面で読む文であるため日本語で書く。
func conditionDisplayName(key ConditionKey) string {
	switch key {
	case ConditionKeyTerminalIpAssignment:
		return "IP から端末への割当による端末"
	case ConditionKeyTerminalIdentityMatches:
		return "端末の識別子の一致"
	case ConditionKeyParentProcessIdMatches:
		return "親のプロセスの識別子の一致"
	case ConditionKeyDestinationAuthority:
		return "接続先 URL の authority"
	case ConditionKeyDestinationIp:
		return "接続先 IP"
	case ConditionKeyDestinationPort:
		return "接続先 port"
	case ConditionKeySecondOfTime:
		return "秒単位の時刻"
	case ConditionKeySubSecondOfTime:
		return "秒未満の時刻"
	case ConditionKeyClientPort:
		return "接続元 port"
	case ConditionKeyProcess:
		return "プロセス"
	case ConditionKeyUser:
		return "利用者"
	default:
		return string(key)
	}
}

// conditionUnresolvedReason は、用いなかった条件 1 件が確定を妨げている理由を返す。
// ok が偽になるのは、段階がその条件を用いたときである。
func conditionUnresolvedReason(condition MatchCondition) (string, bool) {
	name := "「" + conditionDisplayName(condition.ConditionKey) + "」"
	switch condition.Use {
	case ConditionUseNotUsed:
		return name + "をこの段階の条件に用いていない", true
	case ConditionUseNoComparableCounterpart:
		return name + "を両側で比べられる値の組が無い", true
	case ConditionUseItemAbsentOnCounterpart:
		return name + "の欄が相手の入力形式に無い", true
	default:
		return "", false
	}
}

// unresolvedReasonsOfConditions は、段階が用いなかった条件から確定しない理由を組む。
//
// 段階が用いた条件だけでは候補を 1 件に決められないことを、条件ごとの理由で表す。候補 1 件に
// 固有の理由は呼び出し元が MatchCandidateRecord に載せる。
func unresolvedReasonsOfConditions(conditions []MatchCondition) []string {
	reasons := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if reason, ok := conditionUnresolvedReason(condition); ok {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

// stageConditions は段階の conditions を、要求が挙げた条件の宣言から組む。
//
// conditions は宣言の全数を持つ。用いた条件の件数は use が used の要素の個数であり、
// 別の項目で持たない。
//
// 候補の側の値は、文字列が等しいことを条件にした比較で段階の要素すべてが同じ値になるため、
// 先頭の要素から取る。候補が 1 件も無い段階は rightValue を出さない。
func (r MatchRequest) stageConditions(
	stageKey StageKey, members []MatchCandidateRecord,
) ([]MatchCondition, error) {
	var right *MatchObservation
	if len(members) > 0 {
		right = &members[0].Observation
	}
	stageItem := "CandidateStage.conditions of the " + string(stageKey) + " stage"
	conditions := make([]MatchCondition, 0, len(r.Conditions))
	for _, spec := range r.Conditions {
		condition, err := r.conditionOf(spec, stageKey, right)
		if err != nil {
			return nil, itemError(stageItem, err)
		}
		conditions = append(conditions, condition)
	}
	return conditions, nil
}

// conditionOf は宣言 1 件から段階の条件 1 件を組む。
//
// 秒単位の時刻だけは起点の時刻の項目から組み、両側の値を取り出す経路を通らない。
func (r MatchRequest) conditionOf(
	spec MatchConditionSpec, stageKey StageKey, right *MatchObservation,
) (MatchCondition, error) {
	switch spec.ConditionKey {
	case ConditionKeyTerminalIpAssignment:
		return r.terminalCondition(spec, right)
	case ConditionKeySecondOfTime:
		return r.secondOfTimeCondition(stageKey)
	case ConditionKeyDestinationIp:
		return r.sideValueCondition(spec, right, r.Origin.DestinationIpUse)
	default:
		return r.sideValueCondition(spec, right, r.conditionUse(spec))
	}
}

// conditionUse は宣言 1 件の use を決める。
//
// **段階が絞りに用いる条件だけが used を取る。** 用いていない条件に used を出すと、
// 「用いた条件が同一で互いに区別できない組」という indistinguishableGroups の主張と、
// 確定しない理由の一覧が実際の絞り込みと食い違う。
//
// 絞りに用いない条件は、相手の収集元が欄を持つかだけで 2 値に決める。**候補のレコードの
// 値の有無から決めない。** 段階の条件は段階の全候補に係る 1 件であり、段階の先頭の候補 1 件が
// 値を持つかは、残りの候補について何も言わない。
func (r MatchRequest) conditionUse(spec MatchConditionSpec) ConditionUse {
	if spec.Compared {
		return ConditionUseUsed
	}
	if spec.ConditionKey == ConditionKeySubSecondOfTime {
		return r.subSecondUse()
	}
	if r.counterpartCarriesItem(spec) {
		return ConditionUseNotUsed
	}
	return ConditionUseItemAbsentOnCounterpart
}

// subSecondUse は秒未満の時刻の条件の use を返す。
//
// **両側の精度で決める。** 片側が秒までしか持たない組では、もう一方が秒未満を持っても
// 比べる相手が無い。
func (r MatchRequest) subSecondUse() ConditionUse {
	if r.Origin.Observation.EventTime.Precision.carriesSubSecond() &&
		r.CounterpartTimePrecision.carriesSubSecond() {
		return ConditionUseNotUsed
	}
	return ConditionUseNoComparableCounterpart
}

// counterpartCarriesItem は、相手の収集元が宣言の挙げた語彙の項目のいずれかを持つかを返す。
func (r MatchRequest) counterpartCarriesItem(spec MatchConditionSpec) bool {
	for _, wanted := range spec.CounterpartSemantics {
		for _, carried := range r.CounterpartItemSemantics {
			if wanted == carried {
				return true
			}
		}
	}
	return false
}

// sideValueCondition は両側の値を入れた条件 1 件を組む。
func (r MatchRequest) sideValueCondition(
	spec MatchConditionSpec, right *MatchObservation, use ConditionUse,
) (MatchCondition, error) {
	condition := MatchCondition{ConditionKey: spec.ConditionKey, Use: use}
	if err := r.applySideValues(&condition, right, spec); err != nil {
		return MatchCondition{}, err
	}
	return condition, nil
}

// terminalCondition は IP から端末への割当の条件を組む。
//
// outsideAssignmentRange は段階に 1 件の判定である。段階が存在する応答の起点は割当の期間の
// 内側にあるため、値は常に偽になる。
func (r MatchRequest) terminalCondition(
	spec MatchConditionSpec, right *MatchObservation,
) (MatchCondition, error) {
	validRange := r.Origin.AssignmentValidRange
	outside := false
	condition := MatchCondition{
		ConditionKey:           ConditionKeyTerminalIpAssignment,
		Use:                    r.conditionUse(spec),
		AssignmentValidRange:   &validRange,
		OutsideAssignmentRange: &outside,
	}
	if err := r.applySideValues(&condition, right, spec); err != nil {
		return MatchCondition{}, err
	}
	if problem := condition.Validate(); problem != nil {
		return MatchCondition{}, itemError("building the terminal_ip_assignment condition", problem)
	}
	return condition, nil
}

// applySideValues は条件に両側の値を入れる。両側とも semantic が指す語彙の項目を持つ
// 要素から取る。
//
// 起点の側の値は候補の有無に依らず定まるため、候補が 1 件も無い段階にも出す。use が used で
// ない条件には候補の側の値を出さない。
//
// use が used でない条件で、起点がその語彙の項目を持つ要素を持たないときは leftValue を
// 出さない。leftValue が必須になるのは use が used の条件だけである。別の語彙の項目を
// 持つ要素を leftValue に入れると、その条件が比べる意味の値を起点が持つという主張になる。
//
// use が used の条件で語彙の項目を持つ要素が無い状態は、公開の関数を通した呼び出しでは起こらない。起点の
// 側は MatchOrigin.validateComparableFields が BuildCandidateSet の先頭で確かめ、候補の側は
// 段階 1 の絞り込みを通った要素だけが right になる。値の無い条件を組んで先へ進めないよう、
// 本 method でも error を返す。
func (r MatchRequest) applySideValues(
	condition *MatchCondition, right *MatchObservation, spec MatchConditionSpec,
) error {
	left, found := r.Origin.Observation.FieldBySemantic(spec.OriginSemantic)
	if !found {
		if condition.Use == ConditionUseUsed {
			return itemError("building the "+string(condition.ConditionKey)+
				" condition: the origin record carries no field whose semantic is "+
				string(spec.OriginSemantic), ErrMissingRequiredItem)
		}
		return nil
	}
	// **用いていない条件に、値を持たない要素を leftValue として出さない。** 欄を持つ
	// 入力形式のレコードは、当該レコードに値が出ない項目も item_absent の要素で持つ。
	// その要素を出すと、起点がこの条件の値を持つという主張になる。
	if _, comparable := comparableValue(left); !comparable &&
		condition.Use != ConditionUseUsed {
		return nil
	}
	condition.LeftValue = []RecordField{left}
	if right == nil || condition.Use != ConditionUseUsed {
		return nil
	}
	value, found := counterpartField(*right, spec)
	if !found {
		return itemError("building the "+string(condition.ConditionKey)+
			" condition: the candidate record carries no field whose semantic is one of "+
			formatSemantics(spec.CounterpartSemantics), ErrMissingRequiredItem)
	}
	condition.RightValue = []RecordField{value}
	return nil
}

// counterpartField は候補の側の項目を、宣言が挙げた語彙の項目の並びの先頭から探して返す。
func counterpartField(
	observation MatchObservation, spec MatchConditionSpec,
) (RecordField, bool) {
	for _, semantic := range spec.CounterpartSemantics {
		if field, found := observation.FieldBySemantic(semantic); found {
			return field, true
		}
	}
	return RecordField{}, false
}

// formatSemantics は語彙の項目の集合を error message に載せる文字列にする。
func formatSemantics(semantics []SemanticKey) string {
	parts := make([]string, 0, len(semantics))
	for _, semantic := range semantics {
		parts = append(parts, string(semantic))
	}
	return strings.Join(parts, ", ")
}

// secondOfTimeCondition は秒単位の時刻の条件を組む。use は段階 1 で not_used、段階 2 で
// used である。
//
// leftValue を 2 つの段階のどちらにも出す。起点のレコードの時刻は候補の有無と段階の種別に
// 依らず 1 つに定まり、段階 1 の second_of_time を読む検査が時刻の精度を確かめられる。
// 候補の側の時刻は Candidate の timeComparison の rightTime が持つ。
//
// semantic は event.time である。項目が持つのは起点のレコードが記録した事象の時刻で
// あり、入力形式に依らず 1 つに定まる。
// name は起点の観測が event.time を持つ要素に付けた name を取る。値は
// Origin.Observation.EventTime から取り、時刻の出どころを 1 つに保つ。
func (r MatchRequest) secondOfTimeCondition(stageKey StageKey) (MatchCondition, error) {
	eventTime, found := r.Origin.Observation.FieldBySemantic(SemanticKeyEventTime)
	if !found {
		return MatchCondition{}, itemError("building the second_of_time condition: "+
			"the origin record carries no field whose semantic is "+string(SemanticKeyEventTime),
			ErrMissingRequiredItem)
	}
	field, err := NewTimestampField(
		eventTime.Name, SemanticKeyEventTime, r.Origin.Observation.EventTime)
	if err != nil {
		return MatchCondition{}, itemError("building the second_of_time condition", err)
	}
	use := ConditionUseNotUsed
	if stageKey == StageKeySecondTimeMatched {
		use = ConditionUseUsed
	}
	return MatchCondition{
		ConditionKey: ConditionKeySecondOfTime,
		Use:          use,
		LeftValue:    []RecordField{field},
	}, nil
}
