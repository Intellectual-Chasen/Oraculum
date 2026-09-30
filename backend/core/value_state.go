package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ValueState は値の状態を持つ。「値が無い」と「値が 0」を別の状態で表す。
type ValueState string

// ValueState の値。
const (
	// ValueStatePresent は値がある状態である。0 という数値もこの状態である。
	ValueStatePresent ValueState = "present"
	// ValueStateAbsent は入力形式が値の不在を表す文字列を持ち、その文字列が入っている状態である。
	ValueStateAbsent ValueState = "absent"
	// ValueStateItemAbsent は入力形式に欄が無い、または当該レコードに key が出ない状態である。
	ValueStateItemAbsent ValueState = "item_absent"
	// ValueStateNoBody は応答の本体が無いことを、入力形式が値の不在と同じ文字列で表す状態である。
	ValueStateNoBody ValueState = "no_body"
	// ValueStateOutOfDefinition は値が入力形式の定義の範囲の外にある状態である。
	ValueStateOutOfDefinition ValueState = "out_of_definition"
	// ValueStateDerived は原資料に文字列が無く、別の収集元との突き合わせから値を導いた状態である。
	ValueStateDerived ValueState = "derived"
	// ValueStateDerivationUndetermined は導出の対象だが導出できなかった状態である。
	ValueStateDerivationUndetermined ValueState = "derivation_undetermined"
	// ValueStateTruncated は、原資料の行が途中で切れ、文字列が値の後ろを持たない状態である。
	// 原資料の文字列は切れた位置までの byte 列である。
	ValueStateTruncated ValueState = "truncated"
)

// IsKnown は ValueState が上の const のいずれかであるかを返す。
func (v ValueState) IsKnown() bool {
	switch v {
	case ValueStatePresent, ValueStateAbsent, ValueStateItemAbsent,
		ValueStateNoBody, ValueStateOutOfDefinition, ValueStateDerived, ValueStateDerivationUndetermined,
		ValueStateTruncated:
		return true
	default:
		return false
	}
}

// RawAndNormalized は原資料の文字列と、比較に用いる正規化値を別の項目で持つ。
//
// 3 つの文字列を pointer で持つのは、原資料に実在する空文字列と `-` を欄の不在の代用に
// しないためである。項目が出ていない状態を nil が表し、値が空文字列である状態を空文字列を
// 指す pointer が表す。
type RawAndNormalized struct {
	RawText    *string    `json:"rawText,omitempty"`
	Normalized *string    `json:"normalized,omitempty"`
	Derivation *string    `json:"derivation,omitempty"`
	ValueState ValueState `json:"valueState"`
}

// NewRawValue は原資料の文字列を持ち、正規化値を持たない 1 項目を組む。
// item_absent、derived、derivation_undetermined は error を返す。
func NewRawValue(valueState ValueState, rawText string) (RawAndNormalized, error) {
	value := RawAndNormalized{RawText: &rawText, ValueState: valueState}
	if err := value.Validate(); err != nil {
		return RawAndNormalized{}, fmt.Errorf("building RawAndNormalized: %w", err)
	}
	return value, nil
}

// NewNormalizedValue は正規化値と、その導き方を持つ 1 項目を組む。
// 原資料の文字列が必須なので、item_absent、derived、derivation_undetermined は error を返す。
func NewNormalizedValue(
	valueState ValueState, rawText, normalized, derivation string,
) (RawAndNormalized, error) {
	value := RawAndNormalized{
		RawText:    &rawText,
		Normalized: &normalized,
		Derivation: &derivation,
		ValueState: valueState,
	}
	if err := value.Validate(); err != nil {
		return RawAndNormalized{}, fmt.Errorf("building RawAndNormalized: %w", err)
	}
	return value, nil
}

// NewAbsentItemValue は入力形式に欄が無い 1 項目を返す。持つのは valueState だけである。
func NewAbsentItemValue() RawAndNormalized {
	return RawAndNormalized{ValueState: ValueStateItemAbsent}
}

// NewDerivedValue は別の収集元との突き合わせで導いた値と導き方を持つ項目を組む。
// normalized は常に出現する。最小長を求めないため、空文字列も導出結果の
// 値として保持する。空文字列で欠測を表さない。derivation は導出をたどる唯一の手段で
// あり、空文字列では導き方を持てないため拒否する。
// 作成する層は取り込みの実行 (backend/pipeline/) に限る。
func NewDerivedValue(normalized, derivation string) (RawAndNormalized, error) {
	value := RawAndNormalized{
		Normalized: &normalized, Derivation: &derivation, ValueState: ValueStateDerived,
	}
	if err := value.Validate(); err != nil {
		return RawAndNormalized{}, fmt.Errorf("building derived RawAndNormalized: %w", err)
	}
	return value, nil
}

// NewDerivationUndeterminedValue は導出できなかった理由を持つ項目を組む。
// 別の収集元との突き合わせで導けなかった項目と、同じレコードの中の文字列から導けなかった
// 項目の両方が対象である。同じレコードの中の導出の失敗では、
// 導けなかった理由に加えて導出の元にした文字列を derivation に書く。rawText を持てないため、
// 文字列の置き場は derivation である。
// 作成する層は取り込みの実行 (backend/pipeline/) に限る。
func NewDerivationUndeterminedValue(derivation string) (RawAndNormalized, error) {
	value := RawAndNormalized{Derivation: &derivation, ValueState: ValueStateDerivationUndetermined}
	if err := value.Validate(); err != nil {
		return RawAndNormalized{}, fmt.Errorf("building undetermined RawAndNormalized: %w", err)
	}
	return value, nil
}

// RawTextValue は原資料の文字列を返す。ok が偽になるのは文字列が出ていないときである。
func (r RawAndNormalized) RawTextValue() (string, bool) {
	if r.RawText == nil {
		return "", false
	}
	return *r.RawText, true
}

// NormalizedValue は比較に用いる値を返す。ok が偽になるのは正規化値が出ていないときである。
func (r RawAndNormalized) NormalizedValue() (string, bool) {
	if r.Normalized == nil {
		return "", false
	}
	return *r.Normalized, true
}

// DerivationValue は正規化値の導き方を返す。ok が偽になるのは導き方が出ていないときである。
func (r RawAndNormalized) DerivationValue() (string, bool) {
	if r.Derivation == nil {
		return "", false
	}
	return *r.Derivation, true
}

// Validate は項目の整合を確かめる。
//
// valueState が item_absent の RawAndNormalized が持つのは valueState の 1 項目である。
func (r RawAndNormalized) Validate() error {
	if problem := requireKnownEnum("RawAndNormalized.valueState", r.ValueState); problem != nil {
		return problem
	}
	if r.ValueState == ValueStateDerived || r.ValueState == ValueStateDerivationUndetermined {
		return r.validateDerivation()
	}
	// 入力形式に欄が無いレコードは文字列を持たない。
	if r.ValueState == ValueStateItemAbsent {
		if r.RawText != nil || r.Normalized != nil || r.Derivation != nil {
			return itemError("RawAndNormalized.rawText and RawAndNormalized.normalized and "+
				"RawAndNormalized.derivation on an item_absent value", ErrUnexpectedItem)
		}
		return nil
	}
	if r.RawText == nil {
		return itemError("RawAndNormalized.rawText", ErrMissingRequiredItem)
	}
	if r.Normalized == nil {
		if r.Derivation != nil {
			return itemError("RawAndNormalized.derivation", ErrUnexpectedItem)
		}
		return nil
	}
	return r.requireDerivationPresent()
}

// validateDerivation は原資料の文字列を持たない導出の状態の項目を検査する。
func (r RawAndNormalized) validateDerivation() error {
	if r.RawText != nil {
		return itemError("RawAndNormalized.rawText on a derived or derivation_undetermined value", ErrUnexpectedItem)
	}
	if r.ValueState == ValueStateDerived && r.Normalized == nil {
		return itemError("RawAndNormalized.normalized", ErrMissingRequiredItem)
	}
	if r.ValueState == ValueStateDerivationUndetermined && r.Normalized != nil {
		return itemError("RawAndNormalized.normalized on a derivation_undetermined value", ErrUnexpectedItem)
	}
	return r.requireDerivationPresent()
}

// requireDerivationPresent は導き方が存在し、空文字列でないことを確かめる。
func (r RawAndNormalized) requireDerivationPresent() error {
	if r.Derivation == nil {
		return itemError("RawAndNormalized.derivation", ErrMissingRequiredItem)
	}
	return requirePresent("RawAndNormalized.derivation", *r.Derivation)
}

// RecordFieldKind は RecordField が持つ値の形を判別する。
type RecordFieldKind string

// RecordFieldKind の値。
const (
	// RecordFieldKindText は値が時刻以外の項目である。
	RecordFieldKindText RecordFieldKind = "text"
	// RecordFieldKindTimestamp は値が時刻である項目である。
	RecordFieldKindTimestamp RecordFieldKind = "timestamp"
)

// IsKnown は RecordFieldKind が定義の中の値であるかを返す。
func (k RecordFieldKind) IsKnown() bool {
	switch k {
	case RecordFieldKindText, RecordFieldKindTimestamp:
		return true
	default:
		return false
	}
}

// RecordField はレコードの 1 項目を持つ。値が時刻である項目は Timestamp を持ち、
// 値が時刻以外の項目は RawAndNormalized を持つ。どちらを持っているかは Kind が表す。
type RecordField struct {
	Name string `json:"name"`
	// Semantic は入力形式をまたいで同じ意味を表す語彙の項目である。出ない項目は、
	// この入力形式に固有である。
	Semantic  SemanticKey       `json:"semantic,omitempty"`
	Kind      RecordFieldKind   `json:"kind"`
	Text      *RawAndNormalized `json:"text,omitempty"`
	Timestamp *Timestamp        `json:"timestamp,omitempty"`
}

// NewTextField は値が時刻以外の項目の RecordField を返す。
//
// semantic に空の値を渡した項目は、その入力形式に固有の意味を持つ。共通の意味を持つ
// 項目は語彙の値を渡す。
func NewTextField(name string, semantic SemanticKey, text RawAndNormalized) (RecordField, error) {
	field := RecordField{
		Name: name, Semantic: semantic, Kind: RecordFieldKindText, Text: &text,
	}
	if err := field.Validate(); err != nil {
		return RecordField{}, fmt.Errorf("building RecordField: %w", err)
	}
	return field, nil
}

// NewTimestampField は値が時刻である項目の RecordField を返す。
//
// semantic の扱いは NewTextField と同じである。
func NewTimestampField(
	name string, semantic SemanticKey, timestamp Timestamp,
) (RecordField, error) {
	field := RecordField{
		Name: name, Semantic: semantic, Kind: RecordFieldKindTimestamp, Timestamp: &timestamp,
	}
	if err := field.Validate(); err != nil {
		return RecordField{}, fmt.Errorf("building RecordField: %w", err)
	}
	return field, nil
}

// Validate は Kind と、Kind に対応する 1 つの値だけが入っていることを確かめる。
// Semantic は出ているときだけ語彙の中にあることを確かめる。
func (f RecordField) Validate() error {
	problem := firstProblem(
		requirePresent("RecordField.name", f.Name),
		requireKnownEnum("RecordField.kind", f.Kind),
		requireKnownEnumWhenPresent("RecordField.semantic", f.Semantic),
	)
	if problem != nil {
		return problem
	}
	if f.Text != nil && f.Timestamp != nil {
		return itemError("RecordField.text and RecordField.timestamp", ErrInconsistentValue)
	}
	switch f.Kind {
	case RecordFieldKindText:
		if f.Text == nil {
			return itemError("RecordField.text", ErrMissingRequiredItem)
		}
		if err := f.Text.Validate(); err != nil {
			return itemError("RecordField.text", err)
		}
	case RecordFieldKindTimestamp:
		if f.Timestamp == nil {
			return itemError("RecordField.timestamp", ErrMissingRequiredItem)
		}
		if err := f.Timestamp.Validate(); err != nil {
			return itemError("RecordField.timestamp", err)
		}
	}
	return nil
}

// UnmarshalJSON は RecordField を復元する。Kind を欠いた object と、
// Text と Timestamp を同時に持つ object は error を返す。
func (f *RecordField) UnmarshalJSON(data []byte) error {
	type items RecordField
	var decoded items
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding RecordField: %w", err)
	}
	field := RecordField(decoded)
	if err := field.Validate(); err != nil {
		return fmt.Errorf("decoding RecordField: %w", err)
	}
	*f = field
	return nil
}
