package apache

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// absentItemText は combined が値の不在を表す文字列である。引用符付きの欄はこの文字列を
// 引用符で囲んで書く。
const absentItemText = "-"

// derivationRemovedQuotes は引用符を外し、引用符内の逆斜線 escape (`\"` と `\\`) を
// 復号して正規化値を得たことを表す文字列である (access_tokenize.go の scanQuoted)。
const derivationRemovedQuotes = "引用符を外し逆斜線escapeを復号した文字列"

// AccessRecordFields は 1 レコードが持つ欄を core.RecordField の集合へ直す。
//
// 名前は AccessItemName の文字列で、原文の並び順を保つ。共通の意味を持つ欄は語彙の項目を
// semantic に持つ (access_semantic_mapping.go)。要求時刻の欄は引数の Timestamp を kind が
// timestamp の項目として持ち、残りの欄は kind が text の項目として持つ。
//
// **requestTime には ParseAccessTime が返した値を渡す。** 検査を通っていない Timestamp を
// 渡した呼び出しは、検査を通らない項目を含む集合を返す。
//
// error を返さない。名前は非空の定数、値の状態は present と absent と no_body のいずれかで
// 文字列が入っており、正規化時の導き方は非空の定数 derivationRemovedQuotes であるため、core の
// constructor の検査が失敗する組み合わせが無い。
func AccessRecordFields(record AccessRecord, requestTime core.Timestamp) []core.RecordField {
	items := record.Items()
	fields := make([]core.RecordField, 0, len(items))
	for _, item := range items {
		if item.Name() == AccessItemRequestTime {
			// requestTime は検査を通った値であるため、項目の検査が失敗しない。
			timeField, _ := core.NewTimestampField(
				string(AccessItemRequestTime), AccessSemanticOfItem(AccessItemRequestTime), requestTime)
			fields = append(fields, timeField)
			continue
		}
		textField, _ := core.NewTextField(
			string(item.Name()), AccessSemanticOfItem(item.Name()), accessItemValue(item))
		fields = append(fields, textField)
	}
	return fields
}

// accessItemValue は 1 つの欄の値を core.RawAndNormalized へ直す。
//
// 値の不在を表す文字列が入った欄は absent になる。%b (replyBytes) だけは、同じ文字列が
// 「応答の本体が無い」ことを表すため no_body になる。引用符付きで値を持つ欄は、引用符を
// 外した文字列を正規化値として持つ。
func accessItemValue(item AccessItem) core.RawAndNormalized {
	state := core.ValueStatePresent
	if item.Value() == absentItemText {
		state = core.ValueStateAbsent
		if item.Name() == AccessItemReplyBytes {
			state = core.ValueStateNoBody
		}
	}
	quoted := item.RawValue() != item.Value()
	if !quoted || state != core.ValueStatePresent {
		value, _ := core.NewRawValue(state, item.RawValue())
		return value
	}
	value, _ := core.NewNormalizedValue(state, item.RawValue(), item.Value(), derivationRemovedQuotes)
	return value
}
