package apache

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// ErrorRecordFields は 1 レコードが持つ欄を core.RecordField の集合へ直す。
//
// 名前は ErrorItemName の文字列で、原文の並び順を保つ。共通の意味を持つ欄は語彙の項目を
// semantic に持つ (error_semantic_mapping.go)。時刻の欄は引数の Timestamp を kind が
// timestamp の項目として持ち、残りの欄は kind が text の項目として持つ。
//
// **time には ParseErrorTime が返した値を渡す。** 検査を通っていない Timestamp を渡した
// 呼び出しは、検査を通らない項目を含む集合を返す。
//
// error を返さない。名前は非空の定数、値は必ず文字列を持つ present の状態であるため、core の
// constructor の検査が失敗する組み合わせが無い。エラーログは値の不在を表す文字列を持たない。
func ErrorRecordFields(record ErrorRecord, observedAt core.Timestamp) []core.RecordField {
	items := record.Items()
	fields := make([]core.RecordField, 0, len(items))
	for _, item := range items {
		if item.Name() == ErrorItemTime {
			// observedAt は検査を通った値であるため、項目の検査が失敗しない。
			timeField, _ := core.NewTimestampField(
				string(ErrorItemTime), ErrorSemanticOfItem(ErrorItemTime), observedAt)
			fields = append(fields, timeField)
			continue
		}
		value, _ := core.NewRawValue(core.ValueStatePresent, item.RawValue())
		textField, _ := core.NewTextField(string(item.Name()), ErrorSemanticOfItem(item.Name()), value)
		fields = append(fields, textField)
	}
	return fields
}
