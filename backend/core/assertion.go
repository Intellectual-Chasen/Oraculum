package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AssertionTargetKind は所見が指す対象の種類である。
type AssertionTargetKind string

// AssertionTargetKind の値。
const (
	// AssertionTargetKindNode はグラフのノード 1 つを指す。
	AssertionTargetKindNode AssertionTargetKind = "node"
	// AssertionTargetKindEdge は関係 1 本を指す。
	AssertionTargetKindEdge AssertionTargetKind = "edge"
	// AssertionTargetKindRecord は原資料のレコード 1 件を指す。
	AssertionTargetKindRecord AssertionTargetKind = "record"
	// AssertionTargetKindSource は収集元 1 件を、収集元の内容の識別で指す。
	// この対象の所見は、収集元の時刻を読む UTC からのずれ (Assertion.TimeOffset) を持つ。
	AssertionTargetKindSource AssertionTargetKind = "source"
)

// IsKnown は AssertionTargetKind が定義の中の値であるかを返す。
func (k AssertionTargetKind) IsKnown() bool {
	switch k {
	case AssertionTargetKindNode, AssertionTargetKindEdge, AssertionTargetKindRecord,
		AssertionTargetKindSource:
		return true
	default:
		return false
	}
}

// AssertionState は所見が現在も主張されているかである。
type AssertionState string

// AssertionState の値。
const (
	// AssertionStateActive は分析者が現在も主張している所見である。
	AssertionStateActive AssertionState = "active"
	// AssertionStateWithdrawn は分析者が取り下げた所見である。
	// **取り下げた所見も履歴とともに残る。**
	AssertionStateWithdrawn AssertionState = "withdrawn"
)

// IsKnown は AssertionState が定義の中の値であるかを返す。
func (s AssertionState) IsKnown() bool {
	return s == AssertionStateActive || s == AssertionStateWithdrawn
}

// AssertionTargetOrigin は所見の対象が何から出たかである。
//
// **観測から直に出した関係、関連付けが挙げた候補、分析者が付けた所見を分ける。**
// 現在の取り込み結果が対象を持たない状態も別の値で表す。
type AssertionTargetOrigin string

// AssertionTargetOrigin の値。
const (
	// AssertionTargetOriginObservation は、原資料のレコードから直接作った対象である。
	AssertionTargetOriginObservation AssertionTargetOrigin = "observation"
	// AssertionTargetOriginMatchingCandidate は、関連付けが候補として挙げた関係である。
	AssertionTargetOriginMatchingCandidate AssertionTargetOrigin = "matching_candidate"
	// AssertionTargetOriginAnalystAssertion は、分析者が所見で足した対象である。
	// 観測層のグラフはこの対象を持たない。
	AssertionTargetOriginAnalystAssertion AssertionTargetOrigin = "analyst_assertion"
	// AssertionTargetOriginAbsent は、現在の取り込み結果に対象が無い状態である。
	// 対象を持たない取り込みへ所見を持ち込んだときと、記録時にグラフにあった関係が
	// 再解析で消えたときに出る。
	AssertionTargetOriginAbsent AssertionTargetOrigin = "absent"
)

// IsKnown は AssertionTargetOrigin が定義の中の値であるかを返す。
func (o AssertionTargetOrigin) IsKnown() bool {
	switch o {
	case AssertionTargetOriginObservation, AssertionTargetOriginMatchingCandidate,
		AssertionTargetOriginAnalystAssertion, AssertionTargetOriginAbsent:
		return true
	default:
		return false
	}
}

// assertionTimeLayout は所見を記録した時刻の文字列の書式である。
// 秒より細かい桁はミリ秒までを持ち、末尾の Z が UTC を表す。
const assertionTimeLayout = "2006-01-02T15:04:05.000Z"

// AssertionTime は所見を記録した時刻である。
//
// 値は UTC の RFC 3339 の文字列であり、Oraculum を動かすホストの時計が刻む。
// **原資料の時刻と別の型である。** Timestamp は原資料の文字列と精度と時計の帰属を持つが、
// 本型が持つのは Oraculum が所見を受け取った時点だけである。
type AssertionTime string

// NewAssertionTime は時刻を UTC のミリ秒までの文字列へ直す。
func NewAssertionTime(instant time.Time) AssertionTime {
	return AssertionTime(instant.UTC().Format(assertionTimeLayout))
}

// Instant は記録した時刻を返す。ok が偽になるのは文字列を時刻として読めないときである。
func (t AssertionTime) Instant() (time.Time, bool) {
	parsed, err := time.Parse(assertionTimeLayout, string(t))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// Validate は文字列が UTC のミリ秒までの書式であることを確かめる。
func (t AssertionTime) Validate() error {
	if problem := requirePresent("AssertionTime", string(t)); problem != nil {
		return problem
	}
	if _, readable := t.Instant(); !readable {
		return itemError("AssertionTime "+strconv.Quote(string(t)), ErrInvalid)
	}
	return nil
}

// AssertionRecordRef は所見が指す原資料のレコード 1 件である。
//
// **sourceId を持たない。** 同じ原資料を取り込み直すと sourceId は変わる。収集元の内容の
// 識別と収集元の中の位置は取り込みをまたいで同じ値になるため、この 2 つで指す。
//
// **参照は収集元の内容で指す。** 同じ byte 列の収集元を別の取得元から 2 件取り込んだ
// 結果では、1 つの参照が両方の収集元の同じ位置のレコードに一致する。2 件は内容の識別が
// 同じであり、参照はその内容の中の位置を指す。
//
// **所見の対象をグラフの根拠のレコードへ引き当てる鍵は、同じ材料で組む。** 取り込みを
// やり直しても同じレコードを指すことを求める鍵は、いずれも収集元の内容の識別と収集元の
// 中の位置で組む。片方の材料を変えるときは、もう片方も同じ材料に合わせる。
//
// **参照は、収集元がそのレコードについて持つ位置の値をすべて持つ。** 通番と行番号の
// 両方を持つ収集元では、参照も両方を持つ。PositionKind が定めるのは必須の 1 つであり、
// もう 1 つを省いてよいことを表さない。**片方だけを持つ参照は、両方を持つレコードの
// どれにも一致しない。** 引き当ては両方の値を材料にした鍵で行うためである
// (pipeline の AssertionResolver)。
//
// 片方だけの参照を持つ所見は、対象の出所が absent になる。**応答は、位置の値が足りない
// ことと、その位置のレコードが取り込み結果に無いことを分けない。** 分けるには、収集元
// ごとにどの位置の指し方を持つかを core が知る必要がある。NewAssertionRecordRef を通せば
// レコードの位置から材料がそろう。
type AssertionRecordRef struct {
	// SourceContentSha256 は収集元の内容の識別である。
	SourceContentSha256 string `json:"sourceContentSha256"`
	// PositionKind は位置の指し方である。
	PositionKind PositionKind `json:"positionKind"`
	// SequenceNumber は収集元の中の通番 (sn) の値である。
	// PositionKind が sequence_number のとき必須。収集元が通番を持つなら、
	// PositionKind が line_number のときも持つ。
	SequenceNumber *int64 `json:"sequenceNumber,omitempty"`
	// LineNumber は収集元の中の行番号である。1 起点。
	// PositionKind が line_number のとき必須。収集元が行番号を持つなら、
	// PositionKind が sequence_number のときも持つ。
	LineNumber *int64 `json:"lineNumber,omitempty"`
	// ByteOffset は収集元の先頭からのレコードの byte 位置である。
	// PositionKind が byte_range のとき必須。
	ByteOffset *int64 `json:"byteOffset,omitempty"`
}

// NewAssertionRecordRef はレコードの位置から、取り込みをまたいで同じ値になる参照を作る。
// sourceId と表示用の file 名は材料に入らない。
func NewAssertionRecordRef(locator RecordLocator) AssertionRecordRef {
	ref := AssertionRecordRef{
		SourceContentSha256: locator.SourceContentSha256,
		PositionKind:        locator.PositionKind,
	}
	if locator.SequenceNumber != nil {
		value := *locator.SequenceNumber
		ref.SequenceNumber = &value
	}
	if locator.LineNumber != nil {
		value := *locator.LineNumber
		ref.LineNumber = &value
	}
	if locator.ByteOffset != nil {
		value := *locator.ByteOffset
		ref.ByteOffset = &value
	}
	return ref
}

// Validate は項目の整合と、PositionKind に対応する位置の値があることを確かめる。
func (r AssertionRecordRef) Validate() error {
	problem := firstProblem(
		requireLowerHex64("AssertionRecordRef.sourceContentSha256", r.SourceContentSha256),
		requireKnownEnum("AssertionRecordRef.positionKind", r.PositionKind),
	)
	if problem != nil {
		return problem
	}
	switch r.PositionKind {
	case PositionKindSequenceNumber:
		if r.SequenceNumber == nil {
			return itemError("AssertionRecordRef.sequenceNumber", ErrMissingRequiredItem)
		}
	case PositionKindLineNumber:
		if r.LineNumber == nil {
			return itemError("AssertionRecordRef.lineNumber", ErrMissingRequiredItem)
		}
	case PositionKindByteRange:
		if r.ByteOffset == nil {
			return itemError("AssertionRecordRef.byteOffset", ErrMissingRequiredItem)
		}
	}
	if r.ByteOffset != nil {
		if problem := requireNonNegative("AssertionRecordRef.byteOffset",
			*r.ByteOffset); problem != nil {
			return problem
		}
	}
	if r.SequenceNumber != nil {
		if problem := requireNonNegative("AssertionRecordRef.sequenceNumber",
			*r.SequenceNumber); problem != nil {
			return problem
		}
	}
	if r.LineNumber != nil && *r.LineNumber < firstLineNumber {
		return itemError("AssertionRecordRef.lineNumber", ErrInvalid)
	}
	return nil
}

// digestParts は参照を、識別子のハッシュへ渡す文字列の並びへ直す。
func (r AssertionRecordRef) digestParts() []string {
	return []string{
		r.SourceContentSha256, string(r.PositionKind),
		optionalNumberText(r.SequenceNumber), optionalNumberText(r.LineNumber),
		optionalNumberText(r.ByteOffset),
	}
}

// AssertionEdgeRef は所見が指す関係 1 本である。
//
// **関係の種別と両端のノードの識別子で指す。** ノードの識別子は識別鍵だけから導くため、
// 取り込みをやり直しても同じ値になる (GraphNode.Id)。観測層のグラフがまだ持たない関係も
// 指せるため、分析者が足した候補も同じ形で表せる。
type AssertionEdgeRef struct {
	// Kind は関係の種別である。
	Kind EdgeKind `json:"kind"`
	// SourceNodeId は起点のノードの識別子である。
	SourceNodeId string `json:"sourceNodeId"`
	// TargetNodeId は終点のノードの識別子である。
	TargetNodeId string `json:"targetNodeId"`
}

// Validate は種別と両端の識別子を確かめる。
func (r AssertionEdgeRef) Validate() error {
	return firstProblem(
		requireKnownEnum("AssertionEdgeRef.kind", r.Kind),
		requirePresent("AssertionEdgeRef.sourceNodeId", r.SourceNodeId),
		requirePresent("AssertionEdgeRef.targetNodeId", r.TargetNodeId),
	)
}

// AssertionTarget は所見が指す対象である。Kind に対応する 1 つの参照だけを持つ。
//
// **レコードを指すときは Kind を record にする。** 収集元のレコードはグラフのノードにも
// なるため、Kind を node にしてレコードのノードの識別子を渡す形も書ける。理由は 2 つある。
//
// 1 つ目は、レコードの参照の材料が収集元の内容の識別と位置であり、取り込みをやり直しても
// 同じ値になることである。ノードの識別子は識別鍵から導いた digest であり、鍵の形を変えると
// 値が変わる。
//
// 2 つ目は、グラフに載らないレコードを指せることである。取り込みが部分的に失敗した範囲の
// レコードはノードにならない。
//
// **Kind が node の参照がレコードのノードを指すことを退ける検査は無い。** 所見はノードの
// 識別子だけを持ち、その識別子がどの種別のノードのものかを core は判定できない。
type AssertionTarget struct {
	// Kind は対象の種類である。
	Kind AssertionTargetKind `json:"kind"`
	// NodeId はノードの識別子である。Kind が node のとき必須。
	NodeId string `json:"nodeId,omitempty"`
	// Edge は関係の参照である。Kind が edge のとき必須。
	Edge *AssertionEdgeRef `json:"edge,omitempty"`
	// Record はレコードの参照である。Kind が record のとき必須。
	Record *AssertionRecordRef `json:"record,omitempty"`
	// SourceContentSha256 は収集元の内容の識別である。Kind が source のとき必須。
	// sourceId で指さないのは、同じ原資料を取り込み直すと sourceId が変わるためである。
	SourceContentSha256 string `json:"sourceContentSha256,omitempty"`
}

// Validate は Kind と、Kind に対応する 1 つの参照だけが入っていることを確かめる。
func (t AssertionTarget) Validate() error {
	if problem := requireKnownEnum("AssertionTarget.kind", t.Kind); problem != nil {
		return problem
	}
	switch t.Kind {
	case AssertionTargetKindNode:
		return t.validateNode()
	case AssertionTargetKindEdge:
		return t.validateEdge()
	case AssertionTargetKindRecord:
		return t.validateRecord()
	case AssertionTargetKindSource:
		return t.validateSource()
	}
	return nil
}

func (t AssertionTarget) validateNode() error {
	if problem := requirePresent("AssertionTarget.nodeId", t.NodeId); problem != nil {
		return problem
	}
	return t.requireOnly(t.Edge == nil && t.Record == nil && t.SourceContentSha256 == "")
}

func (t AssertionTarget) validateSource() error {
	if problem := requireLowerHex64("AssertionTarget.sourceContentSha256",
		t.SourceContentSha256); problem != nil {
		return problem
	}
	return t.requireOnly(t.NodeId == "" && t.Edge == nil && t.Record == nil)
}

func (t AssertionTarget) validateEdge() error {
	if t.Edge == nil {
		return itemError("AssertionTarget.edge", ErrMissingRequiredItem)
	}
	if err := t.Edge.Validate(); err != nil {
		return itemError("AssertionTarget.edge", err)
	}
	return t.requireOnly(t.NodeId == "" && t.Record == nil && t.SourceContentSha256 == "")
}

func (t AssertionTarget) validateRecord() error {
	if t.Record == nil {
		return itemError("AssertionTarget.record", ErrMissingRequiredItem)
	}
	if err := t.Record.Validate(); err != nil {
		return itemError("AssertionTarget.record", err)
	}
	return t.requireOnly(t.NodeId == "" && t.Edge == nil && t.SourceContentSha256 == "")
}

// requireOnly は Kind に該当しない参照が出ていないことを確かめる。
func (t AssertionTarget) requireOnly(onlyTheMatchingOne bool) error {
	if onlyTheMatchingOne {
		return nil
	}
	return itemError("AssertionTarget carries a reference beside its kind "+string(t.Kind),
		ErrInconsistentValue)
}

// DigestParts は対象を、識別子のハッシュへ渡す文字列の並びへ直す。
// 先頭に対象の種類を置くため、値が同じで種類が異なる対象は別の文字列の並びになる。
func (t AssertionTarget) DigestParts() []string {
	parts := []string{string(t.Kind), t.NodeId}
	if t.Edge != nil {
		parts = append(parts, string(t.Edge.Kind), t.Edge.SourceNodeId, t.Edge.TargetNodeId)
	}
	if t.Record != nil {
		parts = append(parts, t.Record.digestParts()...)
	}
	// 収集元の対象だけが文字列を足す。他の種類の対象の並びを変えると、記録済みの所見の識別子と
	// 対象の比べ方が変わる。
	if t.SourceContentSha256 != "" {
		parts = append(parts, t.SourceContentSha256)
	}
	return parts
}

// UnmarshalJSON は AssertionTarget を復元する。未知の項目と、Kind に該当しない参照を
// 持つ object は error を返す。
func (t *AssertionTarget) UnmarshalJSON(data []byte) error {
	type items AssertionTarget
	var decoded items
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding AssertionTarget: %w", err)
	}
	value := AssertionTarget(decoded)
	if err := value.Validate(); err != nil {
		return fmt.Errorf("decoding AssertionTarget: %w", err)
	}
	*t = value
	return nil
}

// AssertionBasis は分析者が所見に添えた根拠である。
type AssertionBasis struct {
	// Note は分析者が書いた記述である。入力された言語のまま保つ。
	Note string `json:"note"`
	// RecordRefs は根拠に挙げたレコードである。要素数 0 の場合も集合である。
	RecordRefs []AssertionRecordRef `json:"recordRefs"`
}

// Validate は根拠の記述と、挙げたレコードを確かめる。
//
// **空白文字だけの記述を退ける。** 対象を持たない取り込み結果で分析者が読めるのは
// Note の記述だけであり、空白だけの記述は何も残さない (Assertion の doc comment)。
func (b AssertionBasis) Validate() error {
	if problem := requirePresent("AssertionBasis.note", b.Note); problem != nil {
		return problem
	}
	if strings.TrimSpace(b.Note) == "" {
		return itemError("AssertionBasis.note carries blank characters alone",
			ErrMissingRequiredItem)
	}
	return validateElements("AssertionBasis.recordRefs", b.RecordRefs)
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (b AssertionBasis) MarshalJSON() ([]byte, error) {
	// items は AssertionBasis の method を持たないため、この Marshal は再帰しない。
	type items AssertionBasis
	copied := items(b)
	copied.RecordRefs = emptyIfNil(copied.RecordRefs)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling AssertionBasis: %w", err)
	}
	return encoded, nil
}

// FirstAssertionRevisionNumber は所見を最初に記録した改訂の番号である。
const FirstAssertionRevisionNumber = 1

// AssertionRevision は所見の置き換えられた改訂 1 つである。
//
// 現在の改訂は Assertion が直接持つ。本型が持つのは、新しい改訂で置き換えられた古い改訂である。
type AssertionRevision struct {
	// RevisionNumber は改訂の番号である。FirstAssertionRevisionNumber 起点の連番である。
	RevisionNumber int64 `json:"revisionNumber"`
	// State はその改訂の時点で所見が主張されていたかである。
	State AssertionState `json:"state"`
	// Author はその改訂を記録した分析者である。
	Author string `json:"author"`
	// RecordedAt はその改訂を記録した時刻である。
	RecordedAt AssertionTime `json:"recordedAt"`
	// Basis はその改訂の根拠である。
	Basis AssertionBasis `json:"basis"`
	// TimeOffset はその改訂で収集元の時刻を読む UTC からのずれである。対象が収集元の所見に
	// だけ出る。
	TimeOffset *UtcOffset `json:"timeOffset,omitempty"`
}

// Validate は項目の整合を確かめる。
func (r AssertionRevision) Validate() error {
	if r.RevisionNumber < FirstAssertionRevisionNumber {
		return itemError("AssertionRevision.revisionNumber", ErrInvalid)
	}
	problem := firstProblem(
		requireKnownEnum("AssertionRevision.state", r.State),
		requirePresent("AssertionRevision.author", r.Author),
		requireSanitized("AssertionRevision.author", r.Author),
	)
	if problem != nil {
		return problem
	}
	if err := r.RecordedAt.Validate(); err != nil {
		return itemError("AssertionRevision.recordedAt", err)
	}
	if err := r.Basis.Validate(); err != nil {
		return itemError("AssertionRevision.basis", err)
	}
	if r.TimeOffset != nil {
		if err := r.TimeOffset.Validate(); err != nil {
			return itemError("AssertionRevision.timeOffset", err)
		}
	}
	return nil
}

// Assertion は分析者が付けた所見 1 件である。
//
// **所見は自由記述のメモ 1 つと、その根拠に挙げたレコードで成り立つ。** 種別の区分と、
// 特定の目録の識別子を持つ欄を持たない。分析者が対象について書いた記述を Basis.Note が
// そのまま持つ。
//
// **原資料と自動導出を上書きしない。** 取り込み結果と観測層のグラフは本型を持たず、
// 本型が対象を識別子で指す。
//
// 現在の改訂の著者・時刻・根拠を直接持ち、置き換えられた改訂を History が保つ。
//
// **対象が取り込みから消えたときに何を指していたかは Basis.Note が持つ。** Target は
// 識別子だけを持つため、その識別子に該当する対象を持たない取り込み結果で分析者が読めるのは、
// 対象の出所が absent であることと、分析者自身が書いた根拠の記述になる。Basis.Note を必須に
// してあることがその安全網である。対象の説明を Target に持たせる要求は、2026-09-20 に
// 利用者の承認を得て外した。
type Assertion struct {
	// Id は所見を指す不透明な識別子である。
	Id string `json:"id"`
	// Target は所見が指す対象である。
	Target AssertionTarget `json:"target"`
	// State は所見が現在も主張されているかである。
	State AssertionState `json:"state"`
	// Author は現在の改訂を記録した分析者である。
	Author string `json:"author"`
	// RecordedAt は現在の改訂を記録した時刻である。
	RecordedAt AssertionTime `json:"recordedAt"`
	// Basis は現在の改訂の根拠である。
	Basis AssertionBasis `json:"basis"`
	// TimeOffset は、現在の改訂で収集元の時刻を「UTC からこのずれの地方時」として読む値である。
	// Target の Kind が source のとき必須で、他の種類の対象では出ない。根拠の比べたレコードは
	// Basis.RecordRefs が持つ。取り消した改訂 (State が withdrawn) も、取り消したずれを持つ。
	TimeOffset *UtcOffset `json:"timeOffset,omitempty"`
	// AddsRelation は、所見を記録した時点のグラフが対象の関係を持たなかったこと、つまり分析者が
	// 所見で関係を足したことである。Target の Kind が edge の所見だけが真を取り、改訂で変わらない。
	// 偽の所見の関係が後のグラフに無いときは、再解析で消えた関係である。
	AddsRelation bool `json:"addsRelation,omitempty"`
	// ProposalId は、分析者が AI 提案を採用して作った所見が持つ、元の提案の識別子である。改訂で
	// 変わらない。分析者が直接記録した所見では出ない。
	ProposalId string `json:"proposalId,omitempty"`
	// RevisionNumber は現在の改訂の番号である。
	RevisionNumber int64 `json:"revisionNumber"`
	// History は置き換えられた改訂である。古い順に並ぶ。要素数 0 の場合も集合である。
	History []AssertionRevision `json:"history"`
}

// Validate は項目の整合と、改訂の番号と履歴の対応を確かめる。
func (a Assertion) Validate() error {
	problem := firstProblem(
		requirePresent("Assertion.id", a.Id),
		requireKnownEnum("Assertion.state", a.State),
		requirePresent("Assertion.author", a.Author),
		requireSanitized("Assertion.author", a.Author),
		requireSanitized("Assertion.proposalId", a.ProposalId),
	)
	if problem != nil {
		return problem
	}
	if err := a.Target.Validate(); err != nil {
		return itemError("Assertion.target", err)
	}
	if a.AddsRelation && a.Target.Kind != AssertionTargetKindEdge {
		return itemError("Assertion.addsRelation on a target other than an edge", ErrUnexpectedItem)
	}
	if err := a.RecordedAt.Validate(); err != nil {
		return itemError("Assertion.recordedAt", err)
	}
	if err := a.Basis.Validate(); err != nil {
		return itemError("Assertion.basis", err)
	}
	if err := a.validateTimeOffsets(); err != nil {
		return err
	}
	return a.validateHistory()
}

// validateTimeOffsets は、対象が収集元の所見の現在の改訂と全ての履歴の改訂がずれを持ち、
// 他の種類の対象の所見の改訂がずれを持たないことを確かめる。
func (a Assertion) validateTimeOffsets() error {
	wanted := a.Target.Kind == AssertionTargetKindSource
	check := func(item string, offset *UtcOffset) error {
		if offset == nil {
			if wanted {
				return itemError(item, ErrMissingRequiredItem)
			}
			return nil
		}
		if !wanted {
			return itemError(item+" on a target other than a source", ErrUnexpectedItem)
		}
		return offset.Validate()
	}
	if err := check("Assertion.timeOffset", a.TimeOffset); err != nil {
		return err
	}
	for index, revision := range a.History {
		if err := check("Assertion.history at "+formatIndex(index)+" timeOffset",
			revision.TimeOffset); err != nil {
			return err
		}
	}
	return nil
}

// validateHistory は履歴の改訂の番号が連番であり、現在の改訂の番号が履歴の次であることを
// 確かめる。
//
// **改訂の総数を別の項目に持たない。** 現在の改訂の番号と履歴の要素数の関係が総数を決める。
func (a Assertion) validateHistory() error {
	if problem := validateElements("Assertion.history", a.History); problem != nil {
		return problem
	}
	for index, revision := range a.History {
		expected := int64(index) + FirstAssertionRevisionNumber
		if revision.RevisionNumber != expected {
			return itemError("Assertion.history at "+formatIndex(index)+
				" carries a revisionNumber out of order", ErrInconsistentValue)
		}
	}
	if a.RevisionNumber != int64(len(a.History))+FirstAssertionRevisionNumber {
		return itemError("Assertion.revisionNumber differs from the number of superseded "+
			"revisions", ErrInconsistentValue)
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (a Assertion) MarshalJSON() ([]byte, error) {
	// items は Assertion の method を持たないため、この Marshal は再帰しない。
	type items Assertion
	copied := items(a)
	copied.History = emptyIfNil(copied.History)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling Assertion: %w", err)
	}
	return encoded, nil
}

// optionalNumberText は省略可の数値を、識別子のハッシュへ渡す文字列へ直す。
// 値が出ていない状態を 0 と同じ文字列にしないため、出ていない状態は空の文字列になる。
func optionalNumberText(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
