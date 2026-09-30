package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// maxCandidateStages は 1 つの CandidateSet が持つ段階の上限である。
const maxCandidateStages = 2

// EmptyReason は要素数 0 の集合になった理由を持つ。
type EmptyReason string

// EmptyReason の値。置き場ごとに取りうる値は、下の isCandidateSetReason と isStageReason、
// および置き場を持つ型の Validate が決める。api の層だけが置く値は
// no_source_ingested と no_record_in_filter である。
const (
	// EmptyReasonNoRecordInFilter は絞り込みの条件に合うレコードが 0 件である理由である。
	EmptyReasonNoRecordInFilter EmptyReason = "no_record_in_filter"
	// EmptyReasonNoSourceIngested は収集元が 1 件も取り込まれていない理由である。
	// 置き場は `/api/v0/sources` の応答であり、本 package の型は置き場を持たない。
	EmptyReasonNoSourceIngested EmptyReason = "no_source_ingested"
	// EmptyReasonNoCandidateInWindow は時刻の範囲の中に候補が 0 件である理由である。
	EmptyReasonNoCandidateInWindow EmptyReason = "no_candidate_in_window"
	// EmptyReasonNoCandidateMatchingConditions は条件に一致する候補が 0 件である理由である。
	EmptyReasonNoCandidateMatchingConditions EmptyReason = "no_candidate_matching_conditions"
	// EmptyReasonOriginOutsideAssignmentRange は起点のレコードが IP 割当を適用してよい
	// 期間の外にある理由である。
	EmptyReasonOriginOutsideAssignmentRange EmptyReason = "origin_outside_assignment_range"
	// EmptyReasonPublicationWithheld は要求の範囲の結果公開を停止している理由である。
	EmptyReasonPublicationWithheld EmptyReason = "publication_withheld"
	// EmptyReasonCounterpartItemAbsent は関連付けに使える条件が入力形式に無い理由である。
	EmptyReasonCounterpartItemAbsent EmptyReason = "counterpart_item_absent"
	// EmptyReasonNoValueMatch は、検索の文字列を持つ欄がグラフに 1 つも無い理由である。
	//
	// **判定は他の絞り込みを外した走査で行う。** 種別も期間もアドレスの範囲も外して
	// なお一致しないことを確かめた応答だけがこの理由を名乗る。確かめずにこの理由を返すと、
	// 「その値はどこにも無い」という調査の結論を応答が偽って主張する。
	//
	// 走査の範囲はグラフに載った欄である。取り込みに失敗したレコードと、値を読めなかった欄は
	// この判定に入らない。
	EmptyReasonNoValueMatch EmptyReason = "no_value_match"
	// EmptyReasonValueMatchOutsideFilter は、検索の文字列を持つ欄はあるが、他の絞り込みで
	// 残らなかった理由である。
	//
	// **文字列が 1 つも無い状態と分ける。** 分析者が次に採る手が異なる。文字列が無い応答は
	// 文字列を見直す手掛かりであり、本理由は絞り込みを緩める手掛かりである。
	EmptyReasonValueMatchOutsideFilter EmptyReason = "value_match_outside_filter"
	// EmptyReasonNoFieldObserved は、数える欄の名前に値を持つ観測がグラフに 1 つも無い
	// 理由である。
	//
	// **欄の名前の打ち間違いと、絞り込みで残らなかった状態を分ける。** 判定は他の
	// 絞り込みを外した走査で行う。走査の範囲は no_value_match と同じくグラフに載った欄で
	// ある。
	EmptyReasonNoFieldObserved EmptyReason = "no_field_observed"
	// EmptyReasonNoValueInFilter は、数える欄に読めた値を持つ観測はあるが、絞り込みで
	// 残らなかった理由である。
	EmptyReasonNoValueInFilter EmptyReason = "no_value_in_filter"
	// EmptyReasonNoReadableValue は、数える欄の名前を持つレコードはあるが、読めた値を持つ
	// 観測がグラフに 1 つも無い理由である。値の不在を文字列で書いた欄 (Squid の `-`) がこれに該当する。
	EmptyReasonNoReadableValue EmptyReason = "no_readable_value"
	// EmptyReasonFieldOnOtherNodeKind は、数える欄の値を持つノードはあるが、その種別が
	// 要求の種別の条件をどれも通らない理由である。値を持つ種別を応答に添える。
	EmptyReasonFieldOnOtherNodeKind EmptyReason = "field_on_other_node_kind"
	// EmptyReasonRecordMatchWithoutObject は、対象の粒度の要求で、絞り込みに合うレコードは
	// あるが、そのレコードが端末のほかに対象のノードを指さない理由である。レコードの粒度で同じ
	// 絞り込みを走査して確かめる。
	EmptyReasonRecordMatchWithoutObject EmptyReason = "record_match_without_object"
)

// IsKnown は EmptyReason が定義の中の値であるかを返す。
func (e EmptyReason) IsKnown() bool {
	switch e {
	case EmptyReasonNoRecordInFilter, EmptyReasonNoSourceIngested,
		EmptyReasonNoCandidateInWindow, EmptyReasonNoCandidateMatchingConditions,
		EmptyReasonOriginOutsideAssignmentRange, EmptyReasonPublicationWithheld,
		EmptyReasonCounterpartItemAbsent, EmptyReasonNoValueMatch,
		EmptyReasonValueMatchOutsideFilter, EmptyReasonNoFieldObserved,
		EmptyReasonNoValueInFilter, EmptyReasonNoReadableValue, EmptyReasonFieldOnOtherNodeKind,
		EmptyReasonRecordMatchWithoutObject:
		return true
	default:
		return false
	}
}

// isCandidateSetReason は CandidateSet の stages が取りうる値であるかを返す。
//
// counterpart_item_absent は、起点のレコードが関連付けが取り出す語彙の項目を比べられる形で
// 持たない状態である。**割当の期間の外にある状態と同じ文字列にしない。** 2 つは分析者が
// 次に採る手が異なる。期間の外は収集元の観測期間を見直す手掛かりであり、項目を
// 比べられないことは入力形式の欄の不足である。
func (e EmptyReason) isCandidateSetReason() bool {
	switch e {
	case EmptyReasonOriginOutsideAssignmentRange, EmptyReasonPublicationWithheld,
		EmptyReasonCounterpartItemAbsent:
		return true
	default:
		return false
	}
}

// isStageReason は段階の種別ごとに置ける値であるかを返す。
//
// 段階 1 に no_candidate_in_window を置かない。段階 1 の windowKind は not_compared であり、
// 2 つの入力のレコードどうしの時刻を比べていないためである。
func (e EmptyReason) isStageReason(stageKey StageKey) bool {
	switch e {
	case EmptyReasonNoCandidateMatchingConditions, EmptyReasonCounterpartItemAbsent:
		return true
	case EmptyReasonNoCandidateInWindow:
		return stageKey == StageKeySecondTimeMatched
	default:
		return false
	}
}

// PublicationState は結果の公開の状態を持つ。
type PublicationState string

// PublicationState の値。
const (
	// PublicationStatePublishedFull は範囲の全体を公開している状態である。
	PublicationStatePublishedFull PublicationState = "published_full"
	// PublicationStatePublishedPartial は範囲の一部だけを公開している状態である。
	PublicationStatePublishedPartial PublicationState = "published_partial"
	// PublicationStateWithheld は範囲の結果公開を停止している状態である。
	PublicationStateWithheld PublicationState = "withheld"
)

// IsKnown は PublicationState が定義の中の値であるかを返す。
func (p PublicationState) IsKnown() bool {
	switch p {
	case PublicationStatePublishedFull, PublicationStatePublishedPartial, PublicationStateWithheld:
		return true
	default:
		return false
	}
}

// StageKey は候補集合の段階の種別を持つ。
type StageKey string

// StageKey の値。
const (
	// StageKeyClockIndependent は時計に依存しない条件だけの段階である。
	StageKeyClockIndependent StageKey = "clock_independent"
	// StageKeySecondTimeMatched は秒単位の時刻の一致を足した段階である。
	StageKeySecondTimeMatched StageKey = "second_time_matched"
)

// IsKnown は StageKey が定義の中の値であるかを返す。
func (s StageKey) IsKnown() bool {
	switch s {
	case StageKeyClockIndependent, StageKeySecondTimeMatched:
		return true
	default:
		return false
	}
}

// RelationState は関係の状態を持つ。候補を確定に変える値を持たない。
type RelationState string

// RelationState の値。
const (
	// RelationStateObserved は原資料のレコードから直接作った関係の状態である。
	RelationStateObserved RelationState = "observed"
	// RelationStateCandidate は関連付けが候補として挙げた関係の状態である。
	RelationStateCandidate RelationState = "candidate"
	// RelationStateUncertainChain は、1 つの終点に対する候補が 2 つ以上あり、どれが原因かを
	// 決められない候補の状態である。全候補がこの状態を持つ。
	RelationStateUncertainChain RelationState = "uncertain_chain"
)

// IsKnown は RelationState が定義の中の値であるかを返す。
func (r RelationState) IsKnown() bool {
	return r == RelationStateObserved || r == RelationStateCandidate || r == RelationStateUncertainChain
}

// Candidate は候補 1 件を持つ。RelationState は candidate の 1 値だけを取る。
type Candidate struct {
	// RecordRef は候補のレコード位置である。
	RecordRef RecordLocator `json:"recordRef"`
	// EventTime は候補のレコードの時刻である。
	EventTime Timestamp `json:"eventTime"`
	// TimeComparison は候補 1 件と起点の時刻の比較である。
	TimeComparison TimeComparison `json:"timeComparison"`
	// FailedRecordDependency は候補が取り込みで失敗したレコードに依拠しているかである。
	FailedRecordDependency bool `json:"failedRecordDependency"`
	// ProcessRef は候補が指すプロセスである。出ない場合は入力形式にプロセスの欄が無い。
	ProcessRef *ProcessRef `json:"processRef,omitempty"`
	// ObservationKind は候補のレコードの観測の種別と、その意味の状態である。
	ObservationKind ObservationKind `json:"observationKind"`
	// RelationState は関係の状態である。
	RelationState RelationState `json:"relationState"`
	// UnresolvedReasons は確定しない理由の集合である。要素数は 1 以上である。
	UnresolvedReasons []string `json:"unresolvedReasons"`
}

// Validate は項目の整合と、TimeComparison の RightTime が EventTime と等しいことを
// 確かめる。
func (c Candidate) Validate() error {
	if err := c.RecordRef.Validate(); err != nil {
		return itemError("Candidate.recordRef", err)
	}
	if err := c.EventTime.Validate(); err != nil {
		return itemError("Candidate.eventTime", err)
	}
	if err := c.TimeComparison.Validate(); err != nil {
		return itemError("Candidate.timeComparison", err)
	}
	if c.TimeComparison.RightTime != nil && !c.TimeComparison.RightTime.Equal(c.EventTime) {
		return itemError("Candidate.timeComparison.rightTime differs from Candidate.eventTime",
			ErrInconsistentValue)
	}
	if c.ProcessRef != nil {
		if err := c.ProcessRef.Validate(); err != nil {
			return itemError("Candidate.processRef", err)
		}
	}
	if err := c.ObservationKind.Validate(); err != nil {
		return itemError("Candidate.observationKind", err)
	}
	// 候補 1 件の relationState は candidate に限る。
	// 語彙の他の値を持つ候補を受け付けない。
	if c.RelationState != RelationStateCandidate {
		return itemError("Candidate.relationState is not candidate", ErrInvalid)
	}
	if len(c.UnresolvedReasons) == 0 {
		return itemError("Candidate.unresolvedReasons", ErrMissingRequiredItem)
	}
	return requireNoEmptyElement("Candidate.unresolvedReasons", c.UnresolvedReasons)
}

// UnmarshalJSON は全項目を復元する。failedRecordDependency を欠いた object は error を
// 返す。同項目は必須であり、欠けた値を偽へ復元すると、取り込みで失敗したレコードに
// 依拠する候補を依拠しない候補として読める形になる。
func (c *Candidate) UnmarshalJSON(data []byte) error {
	type items Candidate
	var decoded struct {
		items
		FailedRecordDependency *bool `json:"failedRecordDependency"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding Candidate: %w", err)
	}
	if problem := requireDecodedFlag(
		"Candidate.failedRecordDependency", decoded.FailedRecordDependency); problem != nil {
		return fmt.Errorf("decoding Candidate: %w", problem)
	}
	value := Candidate(decoded.items)
	value.FailedRecordDependency = *decoded.FailedRecordDependency
	*c = value
	return nil
}

// CandidateStage は段階 1 つ分の候補集合を持つ。段階ごとに独立した Members を持つ。
type CandidateStage struct {
	// StageKey は段階の種別である。
	StageKey StageKey `json:"stageKey"`
	// Conditions は段階が関連付けに用いた条件と、用いなかった条件の全数の集合である。
	// **要素の一覧を本 package が決めない。** 入力形式を読む adapter の binding が宣言し、
	// 関連付けを実行する側が組む (MatchConditionSpec)。
	// 用いた条件は Use が used の要素で、件数は別の項目で持たない。
	Conditions []MatchCondition `json:"conditions"`
	// IncludedObservationKinds は候補として数えるレコードを選ぶ観測の種別の集合である。
	// 入力間の値の比較は Conditions が持つ。出るときの要素数は 1 以上である。出ない場合は
	// 候補の側の入力形式に観測の種別の欄が無い。
	IncludedObservationKinds []ObservationKindSelector `json:"includedObservationKinds,omitempty"`
	// Assumptions は段階が依拠する前提の集合である。要素数 0 の場合も集合である。
	Assumptions []MatchAssumption `json:"assumptions"`
	// TimeWindow は関連付けに用いた時刻の範囲である。
	TimeWindow TimeWindow `json:"timeWindow"`
	// ComparisonUnit は段階が時刻を比べた単位である。
	// 候補 1 件ごとの時刻の比較は Candidate の TimeComparison が持つ。
	ComparisonUnit ComparisonUnit `json:"comparisonUnit"`
	// Members は候補の集合である。要素数 0 の場合も集合である。
	Members []Candidate `json:"members"`
	// MemberCount は段階が数えた候補の総数である。
	MemberCount int64 `json:"memberCount"`
	// DistinctProcessCount は段階が数えたプロセスの個数である。
	// 出ない場合は候補がプロセスの項目を持たない。
	DistinctProcessCount *int64 `json:"distinctProcessCount,omitempty"`
	// IndistinguishableGroups は用いた条件が同一で互いに区別できない要素の組の集合である。
	// 要素数 0 の場合も集合である。総数が 0 のときも理由を持たない。
	IndistinguishableGroups [][]RecordLocator `json:"indistinguishableGroups"`
	// IndistinguishableGroupCount は段階が数えた組の総数である。
	IndistinguishableGroupCount int64 `json:"indistinguishableGroupCount"`
	// EmptyReason は MemberCount が 0 になった理由である。
	// MemberCount が 1 以上のとき出ない。
	EmptyReason EmptyReason `json:"emptyReason,omitempty"`
	// ClockDependencyNote は段階が時計に依拠する箇所である。
	// 出ない場合は段階が時計に依拠しない。
	ClockDependencyNote string `json:"clockDependencyNote,omitempty"`
}

// Validate は項目の整合と、段階の不変条件を確かめる。
func (s CandidateStage) Validate() error {
	problem := firstProblem(
		requireKnownEnum("CandidateStage.stageKey", s.StageKey),
		requireKnownEnum("CandidateStage.comparisonUnit", s.ComparisonUnit),
	)
	if problem != nil {
		return problem
	}
	return firstProblem(
		s.validateConditions(),
		s.validateSelectorsAndAssumptions(),
		s.validateStageKind(),
		s.validateMembers(),
		s.validateProcessCount(),
		s.validateGroups(),
	)
}

// validateConditions は条件が 1 件以上あり、種別が重ならないことを確かめる。
//
// conditions は関連付けを実行する側が挙げた条件の全数を持つ。**どの条件を持つかを本 package が
// 決めない。** 条件の一覧は入力形式の組ごとに変わる (MatchConditionSpec)。用いた条件の
// 件数は use が used の要素の個数であり、別の項目では持たない。
func (s CandidateStage) validateConditions() error {
	if len(s.Conditions) == 0 {
		return itemError("CandidateStage.conditions", ErrMissingRequiredItem)
	}
	if err := ValidateMatchConditions(s.Conditions, s.MemberCount); err != nil {
		return itemError("CandidateStage.conditions", err)
	}
	seenConditionKeys := make(map[ConditionKey]struct{}, len(s.Conditions))
	for _, condition := range s.Conditions {
		if _, seen := seenConditionKeys[condition.ConditionKey]; seen {
			return itemError("CandidateStage.conditions has a repeated conditionKey "+
				string(condition.ConditionKey), ErrDuplicateElement)
		}
		seenConditionKeys[condition.ConditionKey] = struct{}{}
		// 起点が期間の外にある応答は段階を 1 つも持たない。段階と並ぶ真の値は作れない。
		if condition.OutsideAssignmentRange != nil && *condition.OutsideAssignmentRange {
			return itemError("CandidateStage.conditions carries outsideAssignmentRange "+
				"while the stage exists", ErrInconsistentValue)
		}
		// 時刻を比べていない段階が時刻の条件を用いた形にしない。段階の comparisonUnit が
		// not_compared であることは、段階が時刻を比べていないことを表す。
		if s.ComparisonUnit == ComparisonUnitNotCompared &&
			condition.ConditionKey.isTimeCondition() && condition.Use == ConditionUseUsed {
			return itemError("CandidateStage.conditions uses the time condition "+
				string(condition.ConditionKey)+" while the stage compares no time",
				ErrInconsistentValue)
		}
	}
	return nil
}

// validateSelectorsAndAssumptions は観測の種別の選択と前提の要素を確かめる。
//
// Assumptions は要素数 0 の場合も集合である。
func (s CandidateStage) validateSelectorsAndAssumptions() error {
	if problem := validateAssumptions("CandidateStage.assumptions", s.Assumptions); problem != nil {
		return problem
	}
	// 出るときの要素数は 1 以上である。要素数 0 の集合を出す形にしない。
	if s.IncludedObservationKinds != nil && len(s.IncludedObservationKinds) == 0 {
		return itemError("CandidateStage.includedObservationKinds", ErrMissingRequiredItem)
	}
	return validateElements("CandidateStage.includedObservationKinds",
		s.IncludedObservationKinds)
}

// validateStageKind は段階の種別と、比較の単位と時刻の範囲の対応を確かめる。
//
// stageKey が second_time_matched の段階に windowKind の not_compared を置くことを拒否しない。
// stageKeys に second_time_matched を含む要求と windowKind が not_compared の組み合わせを、
// 要求を invalid_request で拒否するか段階 2 を要素数 0 で返すかは未決である。
// 関連付けの実装が決める。
func (s CandidateStage) validateStageKind() error {
	if err := s.TimeWindow.Validate(); err != nil {
		return itemError("CandidateStage.timeWindow", err)
	}
	switch s.StageKey {
	case StageKeyClockIndependent:
		if s.ComparisonUnit != ComparisonUnitNotCompared {
			return itemError("CandidateStage.comparisonUnit on a clock_independent stage",
				ErrInconsistentValue)
		}
		if s.TimeWindow.WindowKind != WindowKindNotCompared {
			return itemError("CandidateStage.timeWindow on a clock_independent stage",
				ErrInconsistentValue)
		}
	case StageKeySecondTimeMatched:
		if s.ComparisonUnit != ComparisonUnitSecond {
			return itemError("CandidateStage.comparisonUnit on a second_time_matched stage",
				ErrInconsistentValue)
		}
	}
	return nil
}

// validateMembers は候補の要素と、件数と EmptyReason の対応を確かめる。
//
// EmptyReason を出す条件は MemberCount で決める。
func (s CandidateStage) validateMembers() error {
	for index, member := range s.Members {
		if problem := member.Validate(); problem != nil {
			return itemError("CandidateStage.members at "+formatIndex(index), problem)
		}
		if member.TimeComparison.ComparisonUnit != s.ComparisonUnit {
			return itemError("CandidateStage.members at "+formatIndex(index)+
				" compares time in another unit than the stage", ErrInconsistentValue)
		}
		if problem := s.validateMemberObservationKind(index, member); problem != nil {
			return problem
		}
	}
	if problem := validateCountedSet(
		"CandidateStage.member", s.MemberCount, len(s.Members)); problem != nil {
		return problem
	}
	if s.MemberCount == 0 {
		if !s.EmptyReason.isStageReason(s.StageKey) {
			return itemError("CandidateStage.emptyReason "+string(s.EmptyReason)+" on a "+
				string(s.StageKey)+" stage", ErrInvalid)
		}
		return nil
	}
	if s.EmptyReason != "" {
		return itemError("CandidateStage.emptyReason", ErrUnexpectedItem)
	}
	return nil
}

// validateMemberObservationKind は候補の観測の種別が、候補として数えるレコードを選ぶ条件に
// 該当することを確かめる。
//
// **絞りと同じ判定を使う** (selectedByObservationKind)。MatchRequest が候補を絞った後に
// 段階の検査が別の判定で拒むと、絞りを検査されずに通った候補を検査だけが除く組ができる。
//
// 受け取るのは、選択子を持たない段階のすべての候補と、選択子を持つ段階のうち種別を持たない
// 候補と、選択子のいずれかに一致する候補である。段階が何で絞ったかを読む側は、選択子の集合と
// 候補ごとの observationKind の 2 つを読む。
func (s CandidateStage) validateMemberObservationKind(index int, member Candidate) error {
	if selectedByObservationKind(member.ObservationKind, s.IncludedObservationKinds) {
		return nil
	}
	return itemError("CandidateStage.members at "+formatIndex(index)+
		" carries an observation kind outside CandidateStage.includedObservationKinds",
		ErrInconsistentValue)
}

// validateProcessCount はプロセスの個数と、候補が持つプロセスの項目の対応を確かめる。
//
// 出ない場合は候補がプロセスの項目を持たない。候補 1 件が指すプロセスは高々 1 個であるため、
// 個数は候補の総数を超えない。
//
// 総数は打ち切りの前に段階が数えた個数であるため、応答に入れた候補が指す異なる
// プロセスの個数は下限になる。応答が候補の全数を持つとき、2 つの値は一致する。
func (s CandidateStage) validateProcessCount() error {
	returnedProcesses := make(map[ProcessRef]struct{}, len(s.Members))
	for _, member := range s.Members {
		if member.ProcessRef != nil {
			returnedProcesses[*member.ProcessRef] = struct{}{}
		}
	}
	returnedCount := int64(len(returnedProcesses))
	if s.DistinctProcessCount == nil {
		if returnedCount > 0 {
			return itemError("CandidateStage.distinctProcessCount", ErrMissingRequiredItem)
		}
		return nil
	}
	if problem := requireNonNegative(
		"CandidateStage.distinctProcessCount", *s.DistinctProcessCount); problem != nil {
		return problem
	}
	if *s.DistinctProcessCount > s.MemberCount {
		return itemError("CandidateStage.distinctProcessCount is larger than "+
			"CandidateStage.memberCount", ErrInconsistentValue)
	}
	if *s.DistinctProcessCount < returnedCount {
		return itemError("CandidateStage.distinctProcessCount is smaller than the number of "+
			"distinct processes the returned members point at", ErrInconsistentValue)
	}
	if int64(len(s.Members)) == s.MemberCount && *s.DistinctProcessCount != returnedCount {
		return itemError("CandidateStage.distinctProcessCount differs from the number of "+
			"distinct processes while the stage returns every member", ErrInconsistentValue)
	}
	return nil
}

// validateGroups は互いに区別できない要素の組と、その総数と打ち切りを確かめる。
//
// 組の要素数に下限を置かない。indistinguishableGroupCount を全候補を組に分けた総数とするかは
// 未決である。全候補を組に分ける数え方を選ぶと、
// 要素数 1 の組を持つ応答が要る。関連付けの実装が決める。
func (s CandidateStage) validateGroups() error {
	for groupIndex, group := range s.IndistinguishableGroups {
		for index, locator := range group {
			if err := locator.Validate(); err != nil {
				return itemError("CandidateStage.indistinguishableGroups at "+
					formatIndex(groupIndex)+" "+formatIndex(index), err)
			}
		}
	}
	return validateCountedSet("CandidateStage.indistinguishableGroup",
		s.IndistinguishableGroupCount, len(s.IndistinguishableGroups))
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (s CandidateStage) MarshalJSON() ([]byte, error) {
	// items は CandidateStage の method を持たないため、この Marshal は再帰しない。
	type items CandidateStage
	copied := items(s)
	copied.Assumptions = emptyIfNil(copied.Assumptions)
	copied.Members = emptyIfNil(copied.Members)
	copied.IndistinguishableGroups = emptyIfNil(copied.IndistinguishableGroups)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling CandidateStage: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON は全項目を復元する。memberCount と indistinguishableGroupCount を
// 欠いた object は error を返す。2 項目は必須であり、欠けた総数を 0 へ復元すると、
// 打ち切りの前に数えた総数が候補 0 件として読める形になる。
func (s *CandidateStage) UnmarshalJSON(data []byte) error {
	type items CandidateStage
	var decoded struct {
		items
		MemberCount                 *int64 `json:"memberCount"`
		IndistinguishableGroupCount *int64 `json:"indistinguishableGroupCount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding CandidateStage: %w", err)
	}
	problem := firstProblem(
		requireDecodedNumber("CandidateStage.memberCount", decoded.MemberCount),
		requireDecodedNumber("CandidateStage.indistinguishableGroupCount",
			decoded.IndistinguishableGroupCount),
	)
	if problem != nil {
		return fmt.Errorf("decoding CandidateStage: %w", problem)
	}
	value := CandidateStage(decoded.items)
	value.MemberCount = *decoded.MemberCount
	value.IndistinguishableGroupCount = *decoded.IndistinguishableGroupCount
	*s = value
	return nil
}

// CandidateSet は 1 つの起点に対する段階ごとの候補集合を持つ。
type CandidateSet struct {
	// OriginRef は関連付けの起点のレコード位置である。
	OriginRef RecordLocator `json:"originRef"`
	// Stages は段階の集合である。要素数 0 の場合も集合である。
	Stages []CandidateStage `json:"stages"`
	// EmptyReason は Stages の要素数が 0 になった理由である。
	EmptyReason EmptyReason `json:"emptyReason,omitempty"`
	// BothSideCounts は両側の件数である。出ない場合は両側の母数を数えていない。
	BothSideCounts *BothSideCounts `json:"bothSideCounts,omitempty"`
	// UnprocessedRanges は取り込みで未処理または失敗したレコードの範囲である。
	// 要素数 0 の場合も集合である。
	UnprocessedRanges []RecordRange `json:"unprocessedRanges"`
	// UnprocessedRangeCount は UnprocessedRanges の要素の総数である。
	UnprocessedRangeCount int64 `json:"unprocessedRangeCount"`
	// PublicationState は候補集合を作った範囲の公開の状態である。
	PublicationState PublicationState `json:"publicationState"`
	// AnalysisRunRef は候補を作った解析実行への参照である。値の作り方は本 package が
	// 決めない。
	AnalysisRunRef string `json:"analysisRunRef"`
}

// Validate は項目の整合と、候補集合の不変条件を確かめる。
func (c CandidateSet) Validate() error {
	if err := c.OriginRef.Validate(); err != nil {
		return itemError("CandidateSet.originRef", err)
	}
	problem := firstProblem(
		requireKnownEnum("CandidateSet.publicationState", c.PublicationState),
		requirePresent("CandidateSet.analysisRunRef", c.AnalysisRunRef),
	)
	if problem != nil {
		return problem
	}
	if c.BothSideCounts != nil {
		if countsProblem := c.BothSideCounts.Validate(); countsProblem != nil {
			return itemError("CandidateSet.bothSideCounts", countsProblem)
		}
	}
	return firstProblem(
		c.validateStages(),
		c.validateUnprocessedRanges(),
	)
}

// validateStages は段階の要素と、段階どうしの不変条件を確かめる。
func (c CandidateSet) validateStages() error {
	if len(c.Stages) > maxCandidateStages {
		return itemError("CandidateSet.stages", ErrTooManyElements)
	}
	stagesByKey := make(map[StageKey]CandidateStage, len(c.Stages))
	for index, stage := range c.Stages {
		if err := stage.Validate(); err != nil {
			return itemError("CandidateSet.stages at "+formatIndex(index), err)
		}
		if _, seen := stagesByKey[stage.StageKey]; seen {
			return itemError("CandidateSet.stages has a repeated stageKey "+string(stage.StageKey),
				ErrDuplicateElement)
		}
		stagesByKey[stage.StageKey] = stage
	}
	if problem := validateStagePair(stagesByKey); problem != nil {
		return problem
	}
	// CandidateSet の stages に置ける EmptyReason は isCandidateSetReason が定める。
	if c.EmptyReason != "" && !c.EmptyReason.isCandidateSetReason() {
		return itemError("CandidateSet.emptyReason "+string(c.EmptyReason), ErrInvalid)
	}
	if len(c.Stages) > 0 {
		// 公開を止めた応答は段階を 0 件で返す。
		if c.PublicationState == PublicationStateWithheld {
			return itemError("CandidateSet.stages on a withheld candidate set", ErrUnexpectedItem)
		}
		return requireAbsent("CandidateSet.emptyReason", string(c.EmptyReason))
	}
	// 公開を止めた応答は publication_withheld を置く。起点が期間の外にある状態を表す
	// origin_outside_assignment_range を置かない。2 値は別の状態を表す。
	if c.PublicationState == PublicationStateWithheld {
		if c.EmptyReason != EmptyReasonPublicationWithheld {
			return itemError("CandidateSet.emptyReason on a withheld candidate set is "+
				string(c.EmptyReason), ErrInconsistentValue)
		}
		return nil
	}
	// 公開している応答が段階を 1 つも持たない理由は 2 つである。起点が割当の期間の外に
	// ある状態と、関連付けが取り出す語彙の項目を起点が比べられる形で持たない状態である。
	// **候補が 0 件である理由をここに置かない。** 段階を組めた応答の理由は段階が持つ。
	switch c.EmptyReason {
	case EmptyReasonOriginOutsideAssignmentRange, EmptyReasonCounterpartItemAbsent:
		return nil
	default:
		return itemError("CandidateSet.emptyReason on a published candidate set with no stages is "+
			string(c.EmptyReason), ErrInconsistentValue)
	}
}

// validateStagePair は 2 つの段階を揃って持つ候補集合の、段階どうしの不変条件を確かめる。
//
// 確かめるのは 3 つである。段階 2 の used の集合が段階 1 の used の集合に second_of_time を
// 足した集合であること、段階 1 が 0 件なら段階 2 も 0 件であること、段階 2 だけが 0 件である
// 応答の段階 2 の emptyReason が no_candidate_in_window であることである。
func validateStagePair(stagesByKey map[StageKey]CandidateStage) error {
	clockIndependent, hasClockIndependent := stagesByKey[StageKeyClockIndependent]
	secondTimeMatched, hasSecondTimeMatched := stagesByKey[StageKeySecondTimeMatched]
	if !hasClockIndependent || !hasSecondTimeMatched {
		return nil
	}
	if problem := validateSecondStageConditions(clockIndependent, secondTimeMatched); problem != nil {
		return problem
	}
	if clockIndependent.MemberCount == 0 && secondTimeMatched.MemberCount != 0 {
		return itemError("CandidateSet.stages has candidates in second_time_matched "+
			"while clock_independent has none", ErrInconsistentValue)
	}
	if clockIndependent.MemberCount > 0 && secondTimeMatched.MemberCount == 0 &&
		secondTimeMatched.EmptyReason != EmptyReasonNoCandidateInWindow {
		return itemError("CandidateSet.stages carries the emptyReason "+
			string(secondTimeMatched.EmptyReason)+" on second_time_matched while "+
			"clock_independent has candidates", ErrInconsistentValue)
	}
	return nil
}

// validateSecondStageConditions は段階 2 が用いた条件の集合を段階 1 と突き合わせる。
//
// 段階 2 の conditions のうち use が used の要素は、段階 1 の use が used の要素をすべて含み、
// second_of_time を足した集合である。
func validateSecondStageConditions(clockIndependent, secondTimeMatched CandidateStage) error {
	wanted := usedConditionKeys(clockIndependent)
	wanted[ConditionKeySecondOfTime] = struct{}{}
	got := usedConditionKeys(secondTimeMatched)
	for key := range wanted {
		if _, found := got[key]; !found {
			return itemError("CandidateSet.stages omits the used condition "+string(key)+
				" from second_time_matched", ErrMissingRequiredItem)
		}
	}
	for key := range got {
		if _, found := wanted[key]; !found {
			return itemError("CandidateSet.stages uses the condition "+string(key)+
				" in second_time_matched while clock_independent does not use it",
				ErrInconsistentValue)
		}
	}
	return nil
}

// usedConditionKeys は段階が用いた条件の種別の集合を返す。
func usedConditionKeys(stage CandidateStage) map[ConditionKey]struct{} {
	keys := make(map[ConditionKey]struct{}, len(stage.Conditions))
	for _, condition := range stage.Conditions {
		if condition.Use == ConditionUseUsed {
			keys[condition.ConditionKey] = struct{}{}
		}
	}
	return keys
}

// validateUnprocessedRanges は未処理の範囲と、公開の状態と失敗依存の候補の対応を確かめる。
//
// 既知の制限: 失敗依存の候補が未処理の範囲に入ることまでは検査しない,
// 範囲に入るのは候補のレコードまたは候補が用いた導出値の元のレコードだが、
// 導出値の元のレコードの位置を持つ項目が無く、包含を測る対象が型に無い,
// 導出値の元のレコード位置を持つ項目を型が持ったとき見直す。
func (c CandidateSet) validateUnprocessedRanges() error {
	if problem := validateElements("CandidateSet.unprocessedRanges",
		c.UnprocessedRanges); problem != nil {
		return problem
	}
	if problem := validateCountedSet("CandidateSet.unprocessedRange", c.UnprocessedRangeCount,
		len(c.UnprocessedRanges)); problem != nil {
		return problem
	}
	if c.UnprocessedRangeCount > 0 && c.PublicationState == PublicationStatePublishedFull {
		return itemError("CandidateSet.publicationState is published_full while "+
			"unprocessed ranges remain", ErrInconsistentValue)
	}
	if c.hasFailedRecordDependency() && c.UnprocessedRangeCount == 0 {
		return itemError("CandidateSet.unprocessedRangeCount is 0 while a candidate depends "+
			"on a failed record", ErrInconsistentValue)
	}
	return nil
}

// hasFailedRecordDependency は取り込みで失敗したレコードに依拠する候補があるかを返す。
func (c CandidateSet) hasFailedRecordDependency() bool {
	for _, stage := range c.Stages {
		for _, member := range stage.Members {
			if member.FailedRecordDependency {
				return true
			}
		}
	}
	return false
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (c CandidateSet) MarshalJSON() ([]byte, error) {
	// items は CandidateSet の method を持たないため、この Marshal は再帰しない。
	type items CandidateSet
	copied := items(c)
	copied.Stages = emptyIfNil(copied.Stages)
	copied.UnprocessedRanges = emptyIfNil(copied.UnprocessedRanges)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling CandidateSet: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON は全項目を復元する。unprocessedRangeCount を欠いた object は error を
// 返す。同項目は必須であり、欠けた総数を 0 へ復元すると、未処理の範囲がある候補集合を
// 未処理の範囲が無い候補集合として読める形になる。
func (c *CandidateSet) UnmarshalJSON(data []byte) error {
	type items CandidateSet
	var decoded struct {
		items
		UnprocessedRangeCount *int64 `json:"unprocessedRangeCount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding CandidateSet: %w", err)
	}
	if problem := requireDecodedNumber(
		"CandidateSet.unprocessedRangeCount", decoded.UnprocessedRangeCount); problem != nil {
		return fmt.Errorf("decoding CandidateSet: %w", problem)
	}
	value := CandidateSet(decoded.items)
	value.UnprocessedRangeCount = *decoded.UnprocessedRangeCount
	*c = value
	return nil
}
