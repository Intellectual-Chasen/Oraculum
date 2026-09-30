package squid

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// absentItemText は combined が値の不在を表す文字列である。
// 引用符付きの欄はこの文字列を引用符で囲んで書く。
//
// 既知の制限: 復号後の値が - の欄をすべて absent にする,
// %<st に - を書く Squid の条件を repo の中で再現できず、%<st の - の意味を測れない,
// %<st に - を持つ入力を得たとき absent と no_body のどちらかを確かめる
const absentItemText = "-"

// derivationRemovedQuotes は引用符を外して正規化値を得たことを表す文字列である。
const derivationRemovedQuotes = "引用符を外した文字列"

// RecordFields は 1 レコードが持つ欄を core.RecordField の集合へ直す。欄の並びは Layout が定める。
//
// 名前は ItemName の文字列で、原文の並び順を保つ。共通の意味を持つ欄は語彙の項目を
// semantic に持つ (semantic_mapping.go)。要求時刻の欄は引数の Timestamp を
// kind が timestamp の項目として持ち、残りの欄は kind が text の項目として持つ。
//
// **requestTime には ParseRequestTime が返した値を渡す。** 検査を通っていない Timestamp を
// 渡した呼び出しは、検査を通らない項目を含む集合を返す。
//
// **文字列の分割が読めなかったレコードには使わない。** 読めた欄だけを返すため、要素数が
// ItemOrderOf(layout) の長さに満たない集合になる。例外は Squid の 1 行の上限で切れた行で
// あり、読めた欄だけを返し、切れた欄を truncated にする。
//
// error を返さない。名前は非空の定数、値の状態は present と absent と truncated のどれかで文字列が
// 入っており、正規化時の導き方は非空の定数 derivationRemovedQuotes であるため、core の
// constructor の検査が失敗する組み合わせが無い。
func RecordFields(record Record, requestTime core.Timestamp) []core.RecordField {
	items := record.Items()
	fields := make([]core.RecordField, 0, len(items))
	for _, item := range items {
		if item.Name() == ItemRequestTime {
			// requestTime は検査を通った値であるため、項目の検査が失敗しない。
			timeField, _ := core.NewTimestampField(
				string(ItemRequestTime), SemanticOfItem(ItemRequestTime), requestTime)
			fields = append(fields, timeField)
			continue
		}
		// 名前は定数で非空、値は文字列を持ち、semantic は対応表の値であるため検査が失敗しない。
		textField, _ := core.NewTextField(
			string(item.Name()), SemanticOfItem(item.Name()), itemValue(item))
		fields = append(fields, textField)
	}
	return fields
}

// itemValue は 1 つの欄の値を core.RawAndNormalized へ直す。
//
// 値の不在を表す文字列が入っている欄は absent になる。引用符付きで値を持つ欄は、引用符を
// 外した文字列を正規化値として持つ。
func itemValue(item Item) core.RawAndNormalized {
	if item.Truncated() {
		value, _ := core.NewRawValue(core.ValueStateTruncated, item.RawValue())
		return value
	}
	state := core.ValueStatePresent
	if item.Value() == absentItemText {
		state = core.ValueStateAbsent
	}
	if !item.Quoted() || state == core.ValueStateAbsent {
		value, _ := core.NewRawValue(state, item.RawValue())
		return value
	}
	value, _ := core.NewNormalizedValue(
		state, item.RawValue(), item.Value(), derivationRemovedQuotes)
	return value
}
