package pipeline

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// textKey は文字列の値 1 つを、pointer の先の文字列と有無で表す鍵である。
type textKey struct {
	rawText, normalized, derivation          string
	hasRawText, hasNormalized, hasDerivation bool
	valueState                               core.ValueState
}

// textKeyOf は文字列の値の鍵を返す。
func textKeyOf(value core.RawAndNormalized) textKey {
	key := textKey{valueState: value.ValueState}
	if value.RawText != nil {
		key.rawText, key.hasRawText = *value.RawText, true
	}
	if value.Normalized != nil {
		key.normalized, key.hasNormalized = *value.Normalized, true
	}
	if value.Derivation != nil {
		key.derivation, key.hasDerivation = *value.Derivation, true
	}
	return key
}

// textInterner は、同じ文字列の値を 1 つの複製で共有しながらレコードを複製する。
//
// **公開する取り込み結果を組むときだけに使う。** 取り込み結果は組んだ後に書き換えず、
// 外へ返すときに cloneRecords が深く複製するため、レコードどうしが値を共有してもよい。
//
// 既知の制限: 文字列の値の複製を同じ値どうしで共有し、時刻は共有しない,
// ログの文字列の値は同じ値を繰り返し持ち、時刻の項目はレコードごとに異なる値を持つ,
// 時刻の項目に同じ値が多い入力形式を取り込んだときに時刻も共有する
type textInterner map[textKey]*core.RawAndNormalized

// text は文字列の値の複製を返す。同じ値を 2 回渡すと同じ複製を返す。
func (t textInterner) text(value core.RawAndNormalized) *core.RawAndNormalized {
	key := textKeyOf(value)
	if shared, found := t[key]; found {
		return shared
	}
	copied := cloneRawAndNormalized(value)
	t[key] = &copied
	return &copied
}

// field は項目 1 つを、文字列の値を共有して複製する。
func (t textInterner) field(field core.RecordField) core.RecordField {
	if field.Text != nil {
		field.Text = t.text(*field.Text)
	}
	field.Timestamp = cloneTimestampPointer(field.Timestamp)
	return field
}

// fields は項目の並びを複製する。nil は nil のまま返す。
func (t textInterner) fields(fields []core.RecordField) []core.RecordField {
	if fields == nil {
		return nil
	}
	cloned := slices.Clone(fields)
	for index := range cloned {
		cloned[index] = t.field(cloned[index])
	}
	return cloned
}

// semantics は意味付けの結果を、cloneSemantics と同じ範囲で複製する。
func (t textInterner) semantics(semantics *RecordSemantics) *RecordSemantics {
	if semantics == nil {
		return nil
	}
	copied := *semantics
	copied.ObservationKind.Raw = t.fields(semantics.ObservationKind.Raw)
	copied.Fields = t.fields(semantics.Fields)
	copied.ProcessRef = clonePointer(semantics.ProcessRef)
	if semantics.ParentProcessId != nil {
		copied.ParentProcessId = t.text(*semantics.ParentProcessId)
	}
	if semantics.Endpoint != nil {
		copied.Endpoint = &RecordEndpoint{
			ClientEndpoint: t.fields(semantics.Endpoint.ClientEndpoint),
			Destination:    t.fields(semantics.Endpoint.Destination),
		}
	}
	return &copied
}

// records はレコードの並びを、cloneRecords と同じ範囲で複製する。
func (t textInterner) records(records []RecordEntry) []RecordEntry {
	cloned := slices.Clone(records)
	for index := range cloned {
		cloned[index].Locator = cloneLocator(cloned[index].Locator)
		cloned[index].ObservedAt = cloneTimestampPointer(cloned[index].ObservedAt)
		cloned[index].Semantics = t.semantics(cloned[index].Semantics)
		cloned[index].Terminal = t.fields(cloned[index].Terminal)
	}
	return cloned
}
