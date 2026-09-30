package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PositionKind はレコードの位置の指し方を持つ。
type PositionKind string

// PositionKind の値。
const (
	// PositionKindSequenceNumber は収集元の中の通番で位置を指す。
	PositionKindSequenceNumber PositionKind = "sequence_number"
	// PositionKindLineNumber は収集元の中の行番号で位置を指す。
	PositionKindLineNumber PositionKind = "line_number"
	// PositionKindByteRange は収集元の先頭からの byte 位置で位置を指す。
	// 1 レコードが複数行に分かれる形式と、行を持たない形式が用いる。
	PositionKindByteRange PositionKind = "byte_range"
)

// IsKnown は PositionKind が定義の中の値であるかを返す。
func (p PositionKind) IsKnown() bool {
	switch p {
	case PositionKindSequenceNumber, PositionKindLineNumber, PositionKindByteRange:
		return true
	default:
		return false
	}
}

// firstLineNumber は行番号の起点である。
const firstLineNumber = 1

// LowestPosition は位置の指し方が取りうる最小の値を返す。
// 行番号は 1 起点、通番と byte 位置は 0 起点である。
func (p PositionKind) LowestPosition() int64 {
	if p == PositionKindLineNumber {
		return firstLineNumber
	}
	return 0
}

// RecordLocator は原資料のレコードへ到達する位置を持つ。
// SourceId は取り込み 1 件を指し、SourceContentSha256 は同じ取り込みの内容を指す。
// 2 つを揃えて持つ。
type RecordLocator struct {
	// SourceId は収集元の取り込み 1 件を指す内部識別子である。
	SourceId string `json:"sourceId"`
	// SourceContentSha256 は収集元の内容の識別である。
	SourceContentSha256 string `json:"sourceContentSha256"`
	// SourceFileName は表示に使う。
	SourceFileName string `json:"sourceFileName"`
	// PositionKind は位置の指し方である。
	PositionKind PositionKind `json:"positionKind"`
	// SequenceNumber は収集元の中の通番 (sn) の値である。
	// 入力形式が通番を持たない場合や、通番を読めなかった場合は出ない。
	SequenceNumber *int64 `json:"sequenceNumber,omitempty"`
	// LineNumber は収集元の中の行番号である。1 起点。
	// PositionKind が byte_range のときはレコードの先頭の行を指す。
	// 出ない場合は行番号を数えていない。
	LineNumber *int64 `json:"lineNumber,omitempty"`
	// ByteOffset は収集元の先頭からのレコードの byte 位置である。0 起点。
	ByteOffset *int64 `json:"byteOffset,omitempty"`
	// ByteLength はレコードが占める byte 数である。
	ByteLength *int64 `json:"byteLength,omitempty"`
	// LineCount はレコードが占める行数である。1 レコードが複数行に分かれる形式で出る。
	// 出ない場合は行数を数えていない。
	LineCount *int64 `json:"lineCount,omitempty"`
	// RecordRawTextRef はレコードの原文を返す操作への参照である。
	RecordRawTextRef string `json:"recordRawTextRef"`
}

// Validate は項目の整合と、PositionKind に対応する位置の値があることを確かめる。
func (l RecordLocator) Validate() error {
	problem := firstProblem(
		requirePresent("RecordLocator.sourceId", l.SourceId),
		requireLowerHex64("RecordLocator.sourceContentSha256", l.SourceContentSha256),
		requirePresent("RecordLocator.sourceFileName", l.SourceFileName),
		requireKnownEnum("RecordLocator.positionKind", l.PositionKind),
		requirePresent("RecordLocator.recordRawTextRef", l.RecordRawTextRef),
	)
	if problem != nil {
		return problem
	}
	switch l.PositionKind {
	case PositionKindSequenceNumber:
		if l.SequenceNumber == nil {
			return itemError("RecordLocator.sequenceNumber", ErrMissingRequiredItem)
		}
	case PositionKindLineNumber:
		if l.LineNumber == nil {
			return itemError("RecordLocator.lineNumber", ErrMissingRequiredItem)
		}
	case PositionKindByteRange:
		// 長さを欠いた byte 位置は、レコードの終わりを指せない。
		if l.ByteOffset == nil || l.ByteLength == nil {
			return itemError("RecordLocator.byteOffset and RecordLocator.byteLength",
				ErrMissingRequiredItem)
		}
	}
	if l.SequenceNumber != nil {
		if problem := requireNonNegative("RecordLocator.sequenceNumber", *l.SequenceNumber); problem != nil {
			return problem
		}
	}
	if l.LineNumber != nil && *l.LineNumber < firstLineNumber {
		return itemError("RecordLocator.lineNumber", ErrInvalid)
	}
	if l.ByteOffset != nil {
		if problem := requireNonNegative("RecordLocator.byteOffset", *l.ByteOffset); problem != nil {
			return problem
		}
	}
	// 長さ 0 のレコードは原文を持たない。
	if l.ByteLength != nil && *l.ByteLength < 1 {
		return itemError("RecordLocator.byteLength", ErrInvalid)
	}
	if l.LineCount != nil && *l.LineCount < 1 {
		return itemError("RecordLocator.lineCount", ErrInvalid)
	}
	return nil
}

// RangeKind は RecordRange が位置指定を持つかを判別する。
type RangeKind string

// RangeKind の値。
const (
	// RangeKindPositioned は両端のレコード位置を持つ範囲である。
	RangeKindPositioned RangeKind = "positioned"
	// RangeKindWholeSource は収集元 1 件の全レコードを位置指定なしで表す範囲である。
	RangeKindWholeSource RangeKind = "whole_source"
)

// IsKnown は RangeKind が定義の中の値であるかを返す。
func (k RangeKind) IsKnown() bool {
	switch k {
	case RangeKindPositioned, RangeKindWholeSource:
		return true
	default:
		return false
	}
}

// RecordRange は収集元の中のレコードの範囲を持つ。
//
// RangeKind が whole_source の範囲は positionKind と fromPosition と toPosition を
// 持たない。用いるのは、レコードの位置を
// 1 つも確定できない収集元と、両端のいずれかを確定できない収集元である。位置を確定できない
// 状態で 0 を入れて positioned を名乗らない。
type RecordRange struct {
	// SourceId は範囲が属する収集元の取り込み 1 件を指す内部識別子である。
	SourceId string `json:"sourceId"`
	// SourceContentSha256 は範囲が属する収集元の内容の識別である。
	SourceContentSha256 string `json:"sourceContentSha256"`
	// RangeKind は範囲の表し方である。
	RangeKind RangeKind `json:"rangeKind"`
	// PositionKind は位置の指し方である。RangeKind が positioned のとき必須。
	PositionKind PositionKind `json:"positionKind,omitempty"`
	// FromPosition は範囲の始まりの位置である。RangeKind が positioned のとき必須。
	FromPosition *int64 `json:"fromPosition,omitempty"`
	// ToPosition は範囲の終わりの位置である。RangeKind が positioned のとき必須。
	ToPosition *int64 `json:"toPosition,omitempty"`
}

// Validate は項目の整合と、範囲の前後を確かめる。
func (r RecordRange) Validate() error {
	problem := firstProblem(
		requirePresent("RecordRange.sourceId", r.SourceId),
		requireLowerHex64("RecordRange.sourceContentSha256", r.SourceContentSha256),
		requireKnownEnum("RecordRange.rangeKind", r.RangeKind),
	)
	if problem != nil {
		return problem
	}
	if r.RangeKind == RangeKindWholeSource {
		return r.validateWholeSource()
	}
	return r.validatePositioned()
}

// validateWholeSource は positionKind と fromPosition と toPosition が出ていないことを
// 確かめる。
func (r RecordRange) validateWholeSource() error {
	if r.PositionKind != "" {
		return itemError("RecordRange.positionKind is present while rangeKind is whole_source",
			ErrInconsistentValue)
	}
	if r.FromPosition != nil || r.ToPosition != nil {
		return itemError("RecordRange.fromPosition and RecordRange.toPosition are present "+
			"while rangeKind is whole_source", ErrInconsistentValue)
	}
	return nil
}

// validatePositioned は positionKind と fromPosition と toPosition が揃い、範囲の前後が
// 成り立つことを確かめる。
func (r RecordRange) validatePositioned() error {
	if problem := requireKnownEnum("RecordRange.positionKind", r.PositionKind); problem != nil {
		return problem
	}
	// 片方だけを持つ範囲を作らない。
	if r.FromPosition == nil || r.ToPosition == nil {
		return itemError("RecordRange.fromPosition and RecordRange.toPosition",
			ErrMissingRequiredItem)
	}
	problem := firstProblem(
		requireNonNegative("RecordRange.fromPosition", *r.FromPosition),
		requireNonNegative("RecordRange.toPosition", *r.ToPosition),
	)
	if problem != nil {
		return problem
	}
	if *r.FromPosition < r.PositionKind.LowestPosition() {
		return itemError("RecordRange.fromPosition", ErrInvalid)
	}
	if *r.ToPosition < *r.FromPosition {
		return itemError("RecordRange.fromPosition and RecordRange.toPosition", ErrInconsistentValue)
	}
	return nil
}

// ProcessRef はプロセスを指す。ProcessId の一意性の保証は 1 プロセスの起動から終了
// までであるため、端末と組にする。
type ProcessRef struct {
	// SourceId はプロセスを観測した収集元の取り込み 1 件を指す内部識別子である。
	SourceId string `json:"sourceId"`
	// SourceContentSha256 はプロセスを観測した収集元の内容の識別である。
	SourceContentSha256 string `json:"sourceContentSha256"`
	// ProcessId はプロセスの識別子である。**プロセスへ一意な識別子を振らない収集元では、
	// プロセス番号と区間で識別したプロセスのノードの識別子である。** 番号だけでは、番号を
	// 再利用した別のプロセスと区別できない。
	ProcessId string `json:"processId"`
	// TerminalId はプロセスを観測した端末の外部識別子である。外部識別子を持たない端末では出ない。
	TerminalId string `json:"terminalId,omitempty"`
	// TerminalNodeId はプロセスを観測した端末のノードの識別子である。レコードが端末の外部
	// 識別子を持たず、端末を収集元の割当と範囲から決めたときに出る。
	TerminalNodeId string `json:"terminalNodeId,omitempty"`
}

// Validate は収集元とプロセスの項目に値があり、端末を外部識別子かノードの識別子の
// どちらかで指すことを確かめる。
func (p ProcessRef) Validate() error {
	problem := firstProblem(
		requirePresent("ProcessRef.sourceId", p.SourceId),
		requireLowerHex64("ProcessRef.sourceContentSha256", p.SourceContentSha256),
		requirePresent("ProcessRef.processId", p.ProcessId),
	)
	if problem != nil || p.TerminalNodeId != "" {
		return problem
	}
	return requirePresent("ProcessRef.terminalId", p.TerminalId)
}

// TrailInputKind は TrailInputRef の指し先の細かさを判別する。
type TrailInputKind string

// TrailInputKind の値。
const (
	// TrailInputKindRecord は 1 レコードの位置を指す。
	TrailInputKindRecord TrailInputKind = "record"
	// TrailInputKindSource は収集元 1 件を指す。
	TrailInputKindSource TrailInputKind = "source"
)

// IsKnown は TrailInputKind が定義の中の値であるかを返す。
func (k TrailInputKind) IsKnown() bool {
	switch k {
	case TrailInputKindRecord, TrailInputKindSource:
		return true
	default:
		return false
	}
}

// TrailInputRef は段階の入力を、1 レコードと収集元 1 件の 2 つの細かさで指す。
//
// RecordLocator は収集元の中の 1 レコードを指す。収集元全体を入力とする段階は
// Kind を source にして Source で指す。位置の値に 0 を入れて収集元全体を表さない。
type TrailInputRef struct {
	// Kind は指し先の細かさの判別である。
	Kind TrailInputKind `json:"kind"`
	// Record は 1 レコードの位置である。Kind が record のとき必須。
	Record *RecordLocator `json:"record,omitempty"`
	// Source は収集元 1 件の識別である。Kind が source のとき必須。
	Source *SourceIdentity `json:"source,omitempty"`
}

// Validate は Kind と、Kind に対応する 1 つの参照だけが入っていることを確かめる。
func (r TrailInputRef) Validate() error {
	if problem := requireKnownEnum("TrailInputRef.kind", r.Kind); problem != nil {
		return problem
	}
	if r.Record != nil && r.Source != nil {
		return itemError("TrailInputRef.record and TrailInputRef.source", ErrInconsistentValue)
	}
	switch r.Kind {
	case TrailInputKindRecord:
		if r.Record == nil {
			return itemError("TrailInputRef.record", ErrMissingRequiredItem)
		}
		if err := r.Record.Validate(); err != nil {
			return itemError("TrailInputRef.record", err)
		}
	case TrailInputKindSource:
		if r.Source == nil {
			return itemError("TrailInputRef.source", ErrMissingRequiredItem)
		}
		if err := r.Source.Validate(); err != nil {
			return itemError("TrailInputRef.source", err)
		}
	}
	return nil
}

// UnmarshalJSON は TrailInputRef を復元する。Kind を欠いた object と、
// Record と Source を同時に持つ object は error を返す。
func (r *TrailInputRef) UnmarshalJSON(data []byte) error {
	type items TrailInputRef
	var decoded items
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding TrailInputRef: %w", err)
	}
	value := TrailInputRef(decoded)
	if err := value.Validate(); err != nil {
		return fmt.Errorf("decoding TrailInputRef: %w", err)
	}
	*r = value
	return nil
}

// TrailStep は導出の 1 段階を持つ。段階の順序は StepKey が表す。
type TrailStep struct {
	// StepKey は段階の名前である。段階の順序を値で表す。
	StepKey string `json:"stepKey"`
	// InputRefs は段階の入力の集合である。Kind が source の入力を収集元ごとに 1 要素で持ち、
	// Kind が record の入力をレコードごとに 1 要素で持つ。同じ収集元の 2 レコードを入力と
	// する段階は、収集元 1 件に対して 2 要素を持つ。
	// 段階の入力のすべてが原資料の外にあるとき出ない。出るときの要素数は 1 以上である。
	InputRefs []TrailInputRef `json:"inputRefs,omitempty"`
	// UsedIdentifiers は段階で用いた識別子と情報の集合である。要素数は 1 以上である。
	UsedIdentifiers []string `json:"usedIdentifiers"`
	// Output は段階の出力である。
	Output string `json:"output"`
}

// Validate は項目の整合と、UsedIdentifiers の要素数が 1 以上であることを確かめる。
func (s TrailStep) Validate() error {
	if problem := requirePresent("TrailStep.stepKey", s.StepKey); problem != nil {
		return problem
	}
	// 出るときの要素数は 1 以上である。要素数 0 の集合を出す形にしない。
	if s.InputRefs != nil && len(s.InputRefs) == 0 {
		return itemError("TrailStep.inputRefs", ErrMissingRequiredItem)
	}
	if problem := validateElements("TrailStep.inputRefs", s.InputRefs); problem != nil {
		return problem
	}
	if len(s.UsedIdentifiers) == 0 {
		return itemError("TrailStep.usedIdentifiers", ErrMissingRequiredItem)
	}
	if problem := requireNoEmptyElement("TrailStep.usedIdentifiers",
		s.UsedIdentifiers); problem != nil {
		return problem
	}
	return requirePresent("TrailStep.output", s.Output)
}

// DerivationTrail は起点のレコードから元レコードまでの各段階を持つ。
// Steps は要素の並び順を段階の順序として持ち、並び順は StepKey の順序と一致する。
type DerivationTrail struct {
	// OriginRef は起点のレコード位置である。
	OriginRef RecordLocator `json:"originRef"`
	// Steps は各段階の集合である。要素数は 1 以上である。
	Steps []TrailStep `json:"steps"`
	// StoppedAt は進めなかった段階である。出ない場合は最後の段階まで到達した。
	StoppedAt *TrailStep `json:"stoppedAt,omitempty"`
}

// Validate は項目の整合と、Steps の要素数が 1 以上であることを確かめる。
func (t DerivationTrail) Validate() error {
	if err := t.OriginRef.Validate(); err != nil {
		return itemError("DerivationTrail.originRef", err)
	}
	if len(t.Steps) == 0 {
		return itemError("DerivationTrail.steps", ErrMissingRequiredItem)
	}
	seenStepKeys := make(map[string]struct{}, len(t.Steps))
	previousStepKey := ""
	for index, step := range t.Steps {
		if err := step.Validate(); err != nil {
			return itemError("DerivationTrail.steps at "+formatIndex(index), err)
		}
		if _, seen := seenStepKeys[step.StepKey]; seen {
			return itemError("DerivationTrail.steps has a repeated stepKey "+step.StepKey,
				ErrDuplicateElement)
		}
		// 並び順と StepKey の順序が食い違う応答を作らない。
		// 既知の制限: stepKey の順序を文字列の昇順で判定する,
		// 現れる stepKey は A1 から A8 / B1 から B3 / C1 から C5 / D1 から D3 の 1 桁の連番と
		// StageKey の値 (clock_independent、second_time_matched) で、文字列の昇順と段階の順が一致する
		// (StageKey の順は TestStageKeysAreInLexicalOrder が確かめる), stepKey に 2 桁の連番が
		// 現れたとき、または StageKey に段階を足したとき見直す。
		if previousStepKey != "" && step.StepKey < previousStepKey {
			return itemError("DerivationTrail.steps at "+formatIndex(index)+
				" has a stepKey before the previous one", ErrInconsistentValue)
		}
		previousStepKey = step.StepKey
		seenStepKeys[step.StepKey] = struct{}{}
	}
	if t.StoppedAt != nil {
		if err := t.StoppedAt.Validate(); err != nil {
			return itemError("DerivationTrail.stoppedAt", err)
		}
		// stoppedAt は steps の要素である。
		if _, seen := seenStepKeys[t.StoppedAt.StepKey]; !seen {
			return itemError("DerivationTrail.stoppedAt is not one of DerivationTrail.steps",
				ErrInconsistentValue)
		}
	}
	return nil
}
