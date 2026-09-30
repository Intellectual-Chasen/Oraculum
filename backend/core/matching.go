package core

import (
	"fmt"
	"time"
)

// minIndistinguishableGroupSize は indistinguishableGroups の 1 組が持つ要素数の下限である。
//
// 組が表すのは「用いた条件が同一で互いに区別できない要素の組」であり、要素が 1 件の組には
// 区別できない相手がいない。
const minIndistinguishableGroupSize = 2

// MatchObservation は関連付けに渡すレコード 1 件の観測である。
//
// 持つのはレコードの位置、レコードの時刻、観測の種別、レコードの項目の 4 つである。
// Fields の要素は adapter が入力形式の key から組んだ RecordField である。当該レコードに
// key が出ない項目は valueState が item_absent の要素になる。
//
// **Fields は `/api/v0/records` の応答の fields と同じ集合ではない。** 関連付けが取り出すのは
// 語彙の項目を持つ要素で、応答の fields に出ない要素も入る。関連付けが必要とする語彙の項目は
// MatchRequest の godoc が持つ。
//
// 入力形式ごとの name の文字列を本 package が決めない。比べる項目を取り出すのは
// RecordField.Semantic が持つ語彙の項目である。
type MatchObservation struct {
	// Ref はレコードの位置である。
	Ref RecordLocator
	// EventTime はレコードの時刻である。
	EventTime Timestamp
	// ObservationKind はレコードの観測の種別と、その意味の状態である。
	ObservationKind ObservationKind
	// Fields はレコードの項目の集合である。
	Fields []RecordField
}

// Validate は観測の項目の整合と、項目の name と語彙の項目がそれぞれ重ならないことを
// 確かめる。
//
// 語彙の項目の重なりを拒むのは、関連付けが語彙の項目で 1 つの項目を取り出すためである。同じ
// 語彙の項目を持つ要素が 2 つある観測では、どちらの値を比べたかが読めない。語彙の項目を
// 持つ要素は 1 つの観測に 1 つである。
// 語彙の項目を持たない要素は、その入力形式に固有の意味を持つため重なりを数えない。
func (o MatchObservation) Validate() error {
	if err := o.Ref.Validate(); err != nil {
		return itemError("MatchObservation.ref", err)
	}
	if err := o.EventTime.Validate(); err != nil {
		return itemError("MatchObservation.eventTime", err)
	}
	if err := o.ObservationKind.Validate(); err != nil {
		return itemError("MatchObservation.observationKind", err)
	}
	seenNames := make(map[string]struct{}, len(o.Fields))
	seenSemantics := make(map[SemanticKey]struct{}, len(o.Fields))
	for index, field := range o.Fields {
		if err := field.Validate(); err != nil {
			return itemError("MatchObservation.fields at "+formatIndex(index), err)
		}
		if _, duplicate := seenNames[field.Name]; duplicate {
			return itemError("MatchObservation.fields has a repeated name "+field.Name,
				ErrDuplicateElement)
		}
		seenNames[field.Name] = struct{}{}
		if field.Semantic == "" {
			continue
		}
		if _, duplicate := seenSemantics[field.Semantic]; duplicate {
			return itemError("MatchObservation.fields has a repeated semantic "+
				string(field.Semantic), ErrDuplicateElement)
		}
		seenSemantics[field.Semantic] = struct{}{}
	}
	return nil
}

// FieldBySemantic は語彙の項目で指した項目を返す。ok が偽になるのは、その語彙の項目を
// 持つ要素が集合に無いときと、空の語彙の項目を渡したときである。
//
// 空の語彙の項目を集合の中の要素と一致させない。空の値は「この入力形式に固有の意味を
// 持つ」という主張であり、意味が一致する相手を持たない。
func (o MatchObservation) FieldBySemantic(semantic SemanticKey) (RecordField, bool) {
	if semantic == "" {
		return RecordField{}, false
	}
	for _, field := range o.Fields {
		if field.Semantic == semantic {
			return field, true
		}
	}
	return RecordField{}, false
}

// MatchOrigin は関連付けの起点のレコード 1 件と、起点の側の関連付けの条件を持つ。
type MatchOrigin struct {
	// Observation は起点のレコードの観測である。段階 1 の 3 条件と second_of_time が取り出す
	// 語彙の項目は MatchRequest の godoc が持つ。
	Observation MatchObservation
	// DestinationIpUse は接続先 IP の条件の使い方である。used と
	// no_comparable_counterpart と not_used の 3 値を取る。
	//
	// **用いない理由を 2 つに分ける。** 要求先の authority がホスト名である起点は、
	// 相手の接続先 IP と比べられる値を持たないため no_comparable_counterpart になる。
	// 分析者がこの条件を関連付けに選ばなかった起点は not_used になる。前者は原資料について
	// 言えることであり、後者は選択を変えれば変わる。
	DestinationIpUse ConditionUse
	// AssignmentValidRange は IP から端末への割当を適用してよい期間である。
	AssignmentValidRange TimeRange
}

// Validate は起点の項目の整合と、関連付けに使う値が比べられる形であることを確かめる。
func (o MatchOrigin) Validate() error {
	if err := o.Observation.Validate(); err != nil {
		return itemError("MatchOrigin.observation", err)
	}
	if err := o.AssignmentValidRange.Validate(); err != nil {
		return itemError("MatchOrigin.assignmentValidRange", err)
	}
	if problem := requireKnownEnum("MatchOrigin.destinationIpUse", o.DestinationIpUse); problem != nil {
		return problem
	}
	// item_absent_on_counterpart を取らない。接続先 IP の欄を持たない入力形式の
	// レコードは、この経路の手前で候補の側から外れる。
	switch o.DestinationIpUse {
	case ConditionUseUsed, ConditionUseNoComparableCounterpart, ConditionUseNotUsed:
	default:
		return itemError("MatchOrigin.destinationIpUse "+string(o.DestinationIpUse), ErrInvalid)
	}
	return nil
}

// validateComparableFields は段階 1 の条件が用いる起点の値が、文字列を比べられる形で
// 入っていることと、second_of_time が名乗る項目が入っていることを確かめる。
//
// 比べられない値を持つ起点で候補を 0 件にすると、値を読めなかった状態が「条件に一致する
// 候補が 0 件」として読める形になる。呼び出し元が端末の判定と authority の切り出しを
// 済ませてから渡す。
func (o MatchOrigin) validateComparableFields(comparedSemantics []SemanticKey) error {
	for _, semantic := range comparedSemantics {
		field, found := o.Observation.FieldBySemantic(semantic)
		if !found {
			return itemError("MatchOrigin.observation carries no field whose semantic is "+
				string(semantic), ErrMissingRequiredItem)
		}
		if _, ok := comparableValue(field); !ok {
			return itemError("MatchOrigin.observation carries the semantic "+string(semantic)+
				" on a field with no value to compare", ErrMissingRequiredItem)
		}
	}
	// second_of_time の leftValue は起点のレコードの時刻の項目を名乗る。値は
	// Observation.EventTime から取り、name をこの項目から取る。
	if _, found := o.Observation.FieldBySemantic(SemanticKeyEventTime); !found {
		return itemError("MatchOrigin.observation carries no field whose semantic is "+
			string(SemanticKeyEventTime), ErrMissingRequiredItem)
	}
	return nil
}

// MatchCandidateRecord は候補になりうるレコード 1 件を持つ。
type MatchCandidateRecord struct {
	// Observation は候補のレコードの観測である。
	Observation MatchObservation
	// ProcessRef は候補が指すプロセスである。入力形式にプロセスの欄が無いとき nil である。
	ProcessRef *ProcessRef
	// FailedRecordDependency は候補が取り込みで失敗したレコードに依拠しているかである。
	FailedRecordDependency bool
	// UnresolvedReasons はこの候補に固有の、確定しない理由である。段階の条件から導く理由は
	// BuildCandidateSet が足す。
	UnresolvedReasons []string
}

// Validate は候補のレコード 1 件の項目の整合を確かめる。
func (c MatchCandidateRecord) Validate() error {
	if err := c.Observation.Validate(); err != nil {
		return itemError("MatchCandidateRecord.observation", err)
	}
	if c.ProcessRef != nil {
		if err := c.ProcessRef.Validate(); err != nil {
			return itemError("MatchCandidateRecord.processRef", err)
		}
	}
	return requireNoEmptyElement("MatchCandidateRecord.unresolvedReasons", c.UnresolvedReasons)
}

// MatchRequest は 1 つの起点に対する段階ごとの候補集合を組む要求である。
//
// 段階 1 の 3 条件は、起点と候補の両側の RecordField.Semantic が同じ語彙の項目を持つ要素を
// 取り出し、その比べられる形を比べる。項目の name を
// 本 package が受け取らない。呼び出し元は Origin.Observation.Fields と各
// Candidates[i].Observation.Fields に次の語彙の項目を持つ要素を入れる。
//
//   - terminal.id — terminal_ip_assignment が比べる端末の外部識別子。両側で必須である
//   - connection.destination_port — destination_port が比べる接続先 port。両側で必須である
//   - connection.destination_address — destination_ip が比べる接続先 IP。
//     Origin.DestinationIpUse が used のとき両側で必須である。起点の接続先がホスト名で
//     ある要求は、この語彙の項目を持たず use に no_comparable_counterpart を持つ
//   - event.time — second_of_time の leftValue が名乗る項目。起点の側で必須である。
//     leftValue が持つ時刻の値は Origin.Observation.EventTime から取る
//
// 1 つの観測が同じ語彙の項目を 2 つ以上持たない (MatchObservation.Validate)。
type MatchRequest struct {
	// Origin は関連付けの起点である。
	Origin MatchOrigin
	// Candidates は候補になりうるレコードである。呼び出し元が索引で限定した集合を渡す。
	Candidates []MatchCandidateRecord
	// Conditions は段階が持つ条件の宣言である。要素数は 1 以上で、conditionKey が重ならず、
	// compared が真の要素を 1 件以上持つ。
	Conditions []MatchConditionSpec
	// CounterpartItemSemantics は候補の側の収集元が持つ語彙の項目である。
	//
	// **レコードの項目の集合から導かない。** 欄を持たない入力形式のレコードにも
	// item_absent の要素を補う経路があり、要素の有無は欄の有無を表さない。候補が複数の
	// 収集元にまたがる段階では、収集元ごとの集合の和集合を渡す。
	CounterpartItemSemantics []SemanticKey
	// CounterpartTimePrecision は候補の側の時刻の精度である。sub_second_of_time が
	// 秒未満を比べられるかの判定に用いる。
	CounterpartTimePrecision Precision
	// StageKeys は求める段階の種別である。要素数は 1 または 2 である。
	StageKeys []StageKey
	// IncludedObservationKinds は候補として数えるレコードを選ぶ観測の種別である。
	// 候補の側の入力形式に観測の種別の欄が無いとき要素数 0 で渡す。
	IncludedObservationKinds []ObservationKindSelector
	// SecondStageWindow は段階 2 が用いる時刻の範囲である。既定値を持たない。
	SecondStageWindow TimeWindow
	// SecondStageAssumptions は段階 2 と、段階 2 の候補の時刻の比較が依拠する前提である。
	SecondStageAssumptions []MatchAssumption
	// CandidateAssumptions は、候補の母集合を組むときに依拠した前提である。求めたすべての段階の
	// 前提に載り、候補の時刻の比較の前提には載らない。
	CandidateAssumptions []MatchAssumption
	// ClockDependencyNote は段階 1 が時計に依拠する箇所である。
	ClockDependencyNote string
	// PublicationState は候補集合を作った範囲の公開の状態である。
	PublicationState PublicationState
	// UnprocessedRanges は取り込みで未処理または失敗したレコードの範囲である。
	UnprocessedRanges []RecordRange
	// UnprocessedRangeCount は UnprocessedRanges の要素の総数である。
	UnprocessedRangeCount int64
	// BothSideCounts は両側の件数である。母数を数えていないとき nil である。
	BothSideCounts *BothSideCounts
	// AnalysisRunRef は候補を作った解析実行への参照である。
	AnalysisRunRef string

	// validated は WithValidatedCandidates が載せた、検査済みの候補の並びである。
	// Candidates がこの並びと同じ slice のときだけ、Validate は候補の検査を省く。
	validated []MatchCandidateRecord
}

// ValidatedCandidates は、要素がすべて MatchCandidateRecord.Validate を通った候補の並びである。
// ValidateCandidates だけが作る。
//
// **同じ候補の並びを多くの起点の要求で使うときに、候補の検査を 1 回にまとめる。** 起点ごとに
// 同じ候補を検査し直すと、所要が起点の件数と候補の件数の積に比例する。
type ValidatedCandidates struct {
	records []MatchCandidateRecord
}

// ValidateCandidates は候補の並びを検査し、すべて通れば検査済みの並びを返す。
// 呼び出し元は返した後に records の要素を書き換えない。
func ValidateCandidates(records []MatchCandidateRecord) (ValidatedCandidates, error) {
	if err := validateElements("MatchRequest.candidates", records); err != nil {
		return ValidatedCandidates{}, err
	}
	return ValidatedCandidates{records: records}, nil
}

// Records は検査済みの候補の並びを返す。呼び出し元は要素を書き換えない。
func (v ValidatedCandidates) Records() []MatchCandidateRecord {
	return v.records
}

// Subset は indexes が指す要素を、indexes の並びで持つ検査済みの並びを返す。
// 要素は検査済みの要素の複製であり、検査し直さない。範囲外の番号では panic する。
func (v ValidatedCandidates) Subset(indexes []int) ValidatedCandidates {
	records := make([]MatchCandidateRecord, 0, len(indexes))
	for _, index := range indexes {
		records = append(records, v.records[index])
	}
	return ValidatedCandidates{records: records}
}

// WithValidatedCandidates は、Candidates を検査済みの候補の並びにした要求を返す。
func (r MatchRequest) WithValidatedCandidates(candidates ValidatedCandidates) MatchRequest {
	r.Candidates = candidates.records
	r.validated = candidates.records
	return r
}

// candidatesValidated は、Candidates が WithValidatedCandidates の載せた並びのままかを返す。
//
// **並びの先頭の要素の位置と長さで比べる。** 呼び出し元が Candidates を別の並びへ差し替えた
// 要求は、候補の検査を省かない。
func (r MatchRequest) candidatesValidated() bool {
	if r.validated == nil || len(r.Candidates) != len(r.validated) {
		return false
	}
	return len(r.Candidates) == 0 || &r.Candidates[0] == &r.validated[0]
}

// Validate は要求の項目の整合を確かめる。
//
// 時刻の範囲の検査と解釈は secondStageWindowBounds が行い、BuildCandidateSet が Validate の直後に
// 呼ぶ。候補の並びが WithValidatedCandidates で載せた検査済みの並びのままなら、候補の検査を
// 省く。
func (r MatchRequest) Validate() error {
	// **条件の宣言を先に確かめる。** 起点が比べられる形で持つべき項目は宣言が決めるため、
	// 宣言の整合を確かめる前に起点の項目を検査できない。
	if problem := r.validateConditions(); problem != nil {
		return problem
	}
	if err := r.Origin.Validate(); err != nil {
		return itemError("MatchRequest.origin", err)
	}
	if err := r.Origin.validateComparableFields(r.comparedSemantics()); err != nil {
		return itemError("MatchRequest.origin", err)
	}
	// 各検査が返す error は既に項目名を持つため、包み直さない。
	var candidatesProblem error
	if !r.candidatesValidated() {
		candidatesProblem = validateElements("MatchRequest.candidates", r.Candidates)
	}
	problem := firstProblem(
		candidatesProblem,
		r.validateCounterpartItems(),
		r.validateStageKeys(),
		validateElements("MatchRequest.includedObservationKinds", r.IncludedObservationKinds),
		validateAssumptions("MatchRequest.secondStageAssumptions", r.SecondStageAssumptions),
		validateAssumptions("MatchRequest.candidateAssumptions", r.CandidateAssumptions),
		requireKnownEnum("MatchRequest.publicationState", r.PublicationState),
		requirePresent("MatchRequest.analysisRunRef", r.AnalysisRunRef),
	)
	if problem != nil {
		return problem
	}
	return validateElements("MatchRequest.unprocessedRanges", r.UnprocessedRanges)
}

// validateConditions は条件の宣言が重なりなく 1 件以上あり、段階が絞りに用いる条件を
// 1 件以上持つことを確かめる。
//
// **絞りに用いる条件を 1 件も持たない要求を通さない。** その要求が作る段階は、どの
// 候補も除かないまま「条件に一致した候補」を名乗る集合になる。
func (r MatchRequest) validateConditions() error {
	if len(r.Conditions) == 0 {
		return itemError("MatchRequest.conditions", ErrMissingRequiredItem)
	}
	seen := make(map[ConditionKey]struct{}, len(r.Conditions))
	compared := 0
	for index, spec := range r.Conditions {
		if err := spec.Validate(); err != nil {
			return itemError("MatchRequest.conditions at "+formatIndex(index), err)
		}
		if _, duplicate := seen[spec.ConditionKey]; duplicate {
			return itemError("MatchRequest.conditions has a repeated conditionKey "+
				string(spec.ConditionKey), ErrDuplicateElement)
		}
		seen[spec.ConditionKey] = struct{}{}
		if !spec.Compared {
			continue
		}
		compared++
		// **絞りに用いる条件は相手の語彙の項目を 1 件だけ挙げる。** 2 件以上を挙げると、
		// 候補の観測に入れる項目と、比較が実際に取り出す項目が別の語彙になりうる。
		if len(spec.CounterpartSemantics) != 1 {
			return itemError("MatchRequest.conditions: the compared condition "+
				string(spec.ConditionKey)+" names more than one counterpart semantic",
				ErrInconsistentValue)
		}
	}
	if compared == 0 {
		return itemError("MatchRequest.conditions carries no condition the stage compares",
			ErrMissingRequiredItem)
	}
	return r.validateDestinationIpAgreement()
}

// validateDestinationIpAgreement は、接続先 IP の条件の宣言と起点の側の使い方が一致する
// ことを確かめる。
//
// **2 つの定義元を作らない。** 段階の絞り込みは宣言の compared から決まり、条件の use は
// 起点の destinationIpUse から決まる。食い違うと、絞り込みに用いた条件の use が used
// にならない応答になる。
func (r MatchRequest) validateDestinationIpAgreement() error {
	for _, spec := range r.Conditions {
		if spec.ConditionKey != ConditionKeyDestinationIp {
			continue
		}
		if spec.Compared != (r.Origin.DestinationIpUse == ConditionUseUsed) {
			return itemError("MatchRequest.conditions: the destination_ip condition declares "+
				"compared while MatchOrigin.destinationIpUse is "+
				string(r.Origin.DestinationIpUse), ErrInconsistentValue)
		}
	}
	return nil
}

// validateCounterpartItems は候補の側の欄の集合が、既知の語彙の項目を重なりなく持つことを
// 確かめる。
func (r MatchRequest) validateCounterpartItems() error {
	seen := make(map[SemanticKey]struct{}, len(r.CounterpartItemSemantics))
	for index, semantic := range r.CounterpartItemSemantics {
		if semantic == "" {
			return itemError("MatchRequest.counterpartItemSemantics at "+formatIndex(index),
				ErrMissingRequiredItem)
		}
		if _, duplicate := seen[semantic]; duplicate {
			return itemError("MatchRequest.counterpartItemSemantics repeats "+string(semantic),
				ErrDuplicateElement)
		}
		seen[semantic] = struct{}{}
	}
	// 候補が 1 件も無い段階では相手の精度が定まらない。値があるときだけ既知の値かを確かめる。
	if r.CounterpartTimePrecision != "" {
		return requireKnownEnum(
			"MatchRequest.counterpartTimePrecision", r.CounterpartTimePrecision)
	}
	return nil
}

// comparedSemantics は段階 1 が両側の文字列を比べる語彙の項目を、起点の側で返す。
//
// 宣言の compared が真の要素から導く。段階の絞り込みと、条件の use が used になる要素が
// 同じ 1 つの宣言を読む。
func (r MatchRequest) comparedSemantics() []SemanticKey {
	semantics := make([]SemanticKey, 0, len(r.Conditions))
	for _, spec := range r.Conditions {
		if spec.Compared {
			semantics = append(semantics, spec.OriginSemantic)
		}
	}
	return semantics
}

// comparedSpecs は段階 1 が両側の文字列を比べる条件の宣言を返す。
func (r MatchRequest) comparedSpecs() []MatchConditionSpec {
	specs := make([]MatchConditionSpec, 0, len(r.Conditions))
	for _, spec := range r.Conditions {
		if spec.Compared {
			specs = append(specs, spec)
		}
	}
	return specs
}

// validateStageKeys は求める段階の種別が重なりなく 1 件以上あることを確かめる。
func (r MatchRequest) validateStageKeys() error {
	if len(r.StageKeys) == 0 {
		return itemError("MatchRequest.stageKeys", ErrMissingRequiredItem)
	}
	if len(r.StageKeys) > maxCandidateStages {
		return itemError("MatchRequest.stageKeys", ErrTooManyElements)
	}
	seen := make(map[StageKey]struct{}, len(r.StageKeys))
	for index, key := range r.StageKeys {
		if problem := requireKnownEnum(
			"MatchRequest.stageKeys at "+formatIndex(index), key); problem != nil {
			return problem
		}
		if _, duplicate := seen[key]; duplicate {
			return itemError("MatchRequest.stageKeys has a repeated stageKey "+string(key),
				ErrDuplicateElement)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// wantsStage は要求が段階の種別を求めているかを返す。
func (r MatchRequest) wantsStage(stageKey StageKey) bool {
	for _, key := range r.StageKeys {
		if key == stageKey {
			return true
		}
	}
	return false
}

// BuildCandidateSet は起点 1 件と候補になりうるレコード群から、段階ごとの候補集合を組む。
// 引数から結果を決める純粋な判定である。
//
// 段階 1 は terminal_ip_assignment と destination_ip と destination_port で絞り、段階 2 は
// 同じ 3 条件に second_of_time を足して絞る。段階 2 の要素は段階 1 の要素の中から選ぶため、
// 段階 1 が 0 件のとき段階 2 も 0 件になる。
//
// 3 条件が取り出す項目は両側の RecordField.Semantic である。要求が持つ語彙の項目は
// MatchRequest の godoc が持つ。
//
// 順位と確度と点数を持たない。relationState は candidate の 1 値だけを取る。
//
// 時刻の範囲の解釈を emptySet より先に行う。要求の形と識別子だけで決まる失敗の判定は、200 の応答の
// 本体の組み立てに先行する。
// 公開を止めた範囲の空集合と、割当の期間の外にある起点の空集合は、失敗の条件に該当しない
// 要求への応答である。同じ不正な時刻の範囲を持つ要求が、起点と公開の状態に依らず同じ結果になる。
func BuildCandidateSet(request MatchRequest) (CandidateSet, error) {
	if problem := request.Validate(); problem != nil {
		return CandidateSet{}, itemError("building the candidate set", problem)
	}
	window, err := request.secondStageWindowBounds()
	if err != nil {
		return CandidateSet{}, itemError("building the candidate set", err)
	}
	set, err := request.emptySet()
	if err != nil {
		return CandidateSet{}, err
	}
	if set.EmptyReason != "" {
		if validation := set.Validate(); validation != nil {
			return CandidateSet{}, itemError("building the candidate set", validation)
		}
		return set, nil
	}
	stages, err := request.buildStages(window)
	if err != nil {
		return CandidateSet{}, err
	}
	set.Stages = stages
	if validation := set.Validate(); validation != nil {
		return CandidateSet{}, itemError("building the candidate set", validation)
	}
	return set, nil
}

// emptySet は段階を持たない候補集合の枠を返す。段階を 1 つも作らない 2 つの状態では
// EmptyReason が入った値を返す。
//
// 公開を止めた範囲は publication_withheld、起点が割当の期間の外にある応答は
// origin_outside_assignment_range を置く。
func (r MatchRequest) emptySet() (CandidateSet, error) {
	set := CandidateSet{
		OriginRef:             r.Origin.Observation.Ref,
		Stages:                []CandidateStage{},
		BothSideCounts:        r.BothSideCounts,
		UnprocessedRanges:     emptyIfNil(r.UnprocessedRanges),
		UnprocessedRangeCount: r.UnprocessedRangeCount,
		PublicationState:      r.PublicationState,
		AnalysisRunRef:        r.AnalysisRunRef,
	}
	if r.PublicationState == PublicationStateWithheld {
		set.EmptyReason = EmptyReasonPublicationWithheld
		return set, nil
	}
	inside, comparable := r.Origin.AssignmentValidRange.Contains(r.Origin.Observation.EventTime)
	if !comparable {
		return CandidateSet{}, fmt.Errorf(
			"building the candidate set for the record at %q: the origin time and the assignment range cannot be compared: %w",
			r.Origin.Observation.Ref.RecordRawTextRef, ErrInconsistentValue)
	}
	if !inside {
		set.EmptyReason = EmptyReasonOriginOutsideAssignmentRange
		return set, nil
	}
	// 段階を作る枠であることを、要素数 0 の Stages と空の EmptyReason で表す。
	return set, nil
}

// buildStages は要求が求めた段階を、段階 1 の絞り込みを土台にして組む。
//
// BuildCandidateSet は emptySet が起点の時刻と割当の期間を比べられることを確かめた後に
// だけ本 method を呼ぶ。起点の時刻の Instant を本 method の下流で確かめ直さない。
//
// 段階 2 を組むかは window の compared が表す。段階 2 を求めているかの判定は
// secondStageWindowBounds が 1 度だけ行う。
func (r MatchRequest) buildStages(window windowBounds) ([]CandidateStage, error) {
	selected := r.selectByObservationKind()
	clockIndependent := r.filterByClockIndependentConditions(selected)
	stages := make([]CandidateStage, 0, len(r.StageKeys))
	if r.wantsStage(StageKeyClockIndependent) {
		stage, err := r.buildStage(StageKeyClockIndependent, clockIndependent, len(clockIndependent))
		if err != nil {
			return nil, itemError("CandidateSet.stages", err)
		}
		stages = append(stages, stage)
	}
	if !window.compared {
		return stages, nil
	}
	secondMatched := r.filterByWindow(clockIndependent, window)
	stage, err := r.buildStage(StageKeySecondTimeMatched, secondMatched, len(clockIndependent))
	if err != nil {
		return nil, itemError("CandidateSet.stages", err)
	}
	return append(stages, stage), nil
}

// selectByObservationKind は候補として数えるレコードを観測の種別で選ぶ。
//
// 入力間の値の比較は conditions が持ち、候補として数えるレコードを選ぶ条件は
// includedObservationKinds が持つ。
//
// **絞りの判定を段階の検査と 1 つの関数に置く** (selectedByObservationKind)。2 か所で別に
// 持つと、絞りを検査されずに通った候補を段階の検査が拒む組ができる。
func (r MatchRequest) selectByObservationKind() []MatchCandidateRecord {
	selected := make([]MatchCandidateRecord, 0, len(r.Candidates))
	for _, record := range r.Candidates {
		if selectedByObservationKind(record.Observation.ObservationKind,
			r.IncludedObservationKinds) {
			selected = append(selected, record)
		}
	}
	return selected
}

// selectedByObservationKind は観測の種別 1 件が選択子の集合に一致するかを返す。
//
// **偽を返すのは、選択子を持つ段階で、種別を持ちながらどの選択子にも一致しない候補だけで
// ある。** 選択子を 1 つも持たない段階は種別で絞っていない。種別を持たないレコードを除くと、
// 種別の欄を持たない収集元の候補が通知なしに全件消える。
//
// 絞り (selectByObservationKind) と段階の検査 (CandidateStage.validateMemberObservationKind) は、
// どちらも本関数を呼ぶ。
func selectedByObservationKind(kind ObservationKind, selectors []ObservationKindSelector) bool {
	if len(selectors) == 0 || len(kind.Raw) == 0 {
		return true
	}
	for _, selector := range selectors {
		if kind.Matches(selector) {
			return true
		}
	}
	return false
}

// filterByClockIndependentConditions は時計に依存しない条件で候補を絞る。
func (r MatchRequest) filterByClockIndependentConditions(
	records []MatchCandidateRecord,
) []MatchCandidateRecord {
	matched := make([]MatchCandidateRecord, 0, len(records))
	for _, record := range records {
		if r.matchesClockIndependentConditions(record) {
			matched = append(matched, record)
		}
	}
	return matched
}

// matchesClockIndependentConditions は 1 件の候補が段階 1 の条件をすべて満たすかを返す。
//
// 比べるのは、両側が同じ語彙の項目を持つ要素の文字列である。どちらかの側がその語彙の項目を
// 持つ要素を持たない条件と、比べられる値を持たない条件は、一致として扱わない。
// name が同じでも語彙の項目が異なる要素どうしを比べない。
func (r MatchRequest) matchesClockIndependentConditions(record MatchCandidateRecord) bool {
	for _, spec := range r.comparedSpecs() {
		left, leftOk := observationValue(r.Origin.Observation, spec.OriginSemantic)
		right, rightOk := counterpartValue(record.Observation, spec)
		if !leftOk || !rightOk || left != right {
			return false
		}
	}
	return true
}

// counterpartValue は候補の側の値を、宣言が挙げた語彙の項目の並びの先頭から探して返す。
//
// 相手が同じ意味を 2 つ以上の語彙の項目で持つ入力形式があるため、集合を順に探す。
func counterpartValue(observation MatchObservation, spec MatchConditionSpec) (string, bool) {
	for _, semantic := range spec.CounterpartSemantics {
		if value, ok := observationValue(observation, semantic); ok {
			return value, true
		}
	}
	return "", false
}

// observationValue は語彙の項目で指した要素の、文字列の一致を比べる値を返す。
func observationValue(observation MatchObservation, semantic SemanticKey) (string, bool) {
	field, found := observation.FieldBySemantic(semantic)
	if !found {
		return "", false
	}
	return comparableValue(field)
}

// filterByWindow は段階 2 の時刻の範囲で候補を絞る。
//
// 時刻の範囲の中にあるかを決められない時刻の候補は段階 2 に入れない。比べられない時刻を時刻の範囲の中の
// 時刻として扱うと、時刻を比べていない候補が時刻の一致を根拠に並ぶ形になる。
func (r MatchRequest) filterByWindow(
	records []MatchCandidateRecord, window windowBounds,
) []MatchCandidateRecord {
	matched := make([]MatchCandidateRecord, 0, len(records))
	for _, record := range records {
		instant, ok := record.Observation.EventTime.Instant()
		if !ok {
			continue
		}
		if window.contains(instant) {
			matched = append(matched, record)
		}
	}
	return matched
}

// windowBounds は段階 2 の時刻の範囲を秒に切り捨てた両端で表す。
//
// compared が偽の値は両端を持たない。段階 2 を求めない要求の時刻の範囲を zero value の時刻で表すと、
// 「時刻の範囲が無い」と「西暦 1 年の 1 秒の時刻の範囲」が同じ値になる。
type windowBounds struct {
	compared     bool
	lower, upper time.Time
}

// contains は時刻が時刻の範囲の中にあるかを返す。比較の単位は秒である。
func (b windowBounds) contains(at time.Time) bool {
	atSecond := truncateToSecond(at)
	return !atSecond.Before(b.lower) && !atSecond.After(b.upper)
}

// secondStageWindowBounds は要求の時刻の範囲を検査し、秒に切り捨てた両端を返す。
//
// 段階 2 を求めない要求は SecondStageWindow を読まずに両端の無い値を返す。段階 1 だけを求める
// 要求の時刻の範囲の形は API の層が candidate_window_missing で確かめるため、core は重複させない。
//
// 段階 2 を求める要求の windowKind が not_compared のときは拒む。段階 2 の comparisonUnit は
// second であり、比べていない時刻の範囲を持つ段階 2 は、比べた単位と時刻の範囲が食い違う応答になる。
// 半幅 0 の symmetric_seconds は受理し、centerTime の秒だけを取る時刻の範囲にする。
func (r MatchRequest) secondStageWindowBounds() (windowBounds, error) {
	if !r.wantsStage(StageKeySecondTimeMatched) {
		return windowBounds{}, nil
	}
	window := r.SecondStageWindow
	if problem := window.Validate(); problem != nil {
		return windowBounds{}, itemError("MatchRequest.secondStageWindow", problem)
	}
	switch window.WindowKind {
	case WindowKindSameSecond:
		center, err := requestedInstant("TimeWindow.centerTime", window.CenterTime)
		if err != nil {
			return windowBounds{}, err
		}
		return windowBounds{compared: true, lower: center, upper: center}, nil
	case WindowKindSecondRange:
		lower, err := requestedInstant("TimeWindow.lowerBound", window.LowerBound)
		if err != nil {
			return windowBounds{}, err
		}
		upper, err := requestedInstant("TimeWindow.upperBound", window.UpperBound)
		if err != nil {
			return windowBounds{}, err
		}
		return windowBounds{compared: true, lower: lower, upper: upper}, nil
	case WindowKindSymmetricSeconds:
		return symmetricWindowBounds(window)
	default:
		return windowBounds{}, itemError(
			"MatchRequest.secondStageWindow.windowKind not_compared while stageKeys wants second_time_matched",
			ErrInconsistentValue)
	}
}

// symmetricWindowBounds は中心から前後に同じ秒数だけ広げた時刻の範囲の両端を返す。
//
// 半幅があることと、下限 0 と上限 maxWindowRadiusSeconds は TimeWindow.Validate が
// 確かめる。呼び出し元の secondStageWindowBounds が本関数より先に呼ぶ。
func symmetricWindowBounds(window TimeWindow) (windowBounds, error) {
	center, err := requestedInstant("TimeWindow.centerTime", window.CenterTime)
	if err != nil {
		return windowBounds{}, err
	}
	radius := time.Duration(*window.RadiusSeconds) * time.Second
	return windowBounds{compared: true, lower: center.Add(-radius), upper: center.Add(radius)}, nil
}

// requestedInstant は要求が与えた時刻の正規化値から、関連付けに使う秒を返す。
//
// 正規化値を持たない要求の時刻を既定の日付とずれで補わない。
func requestedInstant(item string, requested *RequestedTime) (time.Time, error) {
	if requested == nil {
		return time.Time{}, itemError(item, ErrMissingRequiredItem)
	}
	if requested.NormalizedForm != NormalizedFormRFC3339Absolute || requested.Normalized == "" {
		return time.Time{}, itemError(item+" carries no absolute normalized value",
			ErrMissingRequiredItem)
	}
	parsed, err := time.Parse(time.RFC3339, requested.Normalized)
	if err != nil {
		return time.Time{}, itemError(item+" cannot be read as a date and time",
			ErrInconsistentValue)
	}
	return truncateToSecond(parsed), nil
}
