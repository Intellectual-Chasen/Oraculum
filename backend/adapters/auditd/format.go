package auditd

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// ParserID は収集元 1 件を読む走査器を指す識別子である。
// 事象単位のまとめと、行ごとの文字列の分割を結合した走査器を指す。parserVersion の材料になる。
const ParserID = "linux-auditd-log"

// FormatKeyAuditLog は Linux auditd の監査ログである。
const FormatKeyAuditLog core.FormatKey = "linux_auditd"

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。
//
// レコードが欄の key を持つため、欄の並びの指定を取らない。
//
// 位置の指し方は byte 範囲である。1 事象が複数行に分かれるため、行番号だけでは
// レコードの終わりを指せない。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{{
		Key: FormatKeyAuditLog, ParserID: ParserID,
		PositionKind: core.PositionKindByteRange,
		SpecInput:    core.FormatSpecInputRejected,
	}}
}

// ObservationKindSelectorOf は `type` の値から、候補として数えるレコードを選ぶ観測の
// 種別 1 件を組む。
//
// **欄の名前を本 package が持つ。** 名前は auditd の監査ログの key の文字列であり、
// core と pipeline はこの文字列を知らない。
func ObservationKindSelectorOf(recordType string) core.ObservationKindSelector {
	return core.ObservationKindSelector{Items: []core.ObservationKindSelectorItem{
		{Name: keyType, Value: recordType},
	}}
}

// ObservationKindRaw は 1 事象の観測の種別を持つ欄を返す。
//
// 事象の種別を表すのは、事象の本体の行の `type` である。`type=SYSCALL` を持つ事象は
// その値を、持たない事象は最初の行の値を持つ。
//
// ok が偽になるのは、種別を読めた行が 1 つも無いときである。
func ObservationKindRaw(record Record) (core.RecordField, bool, error) {
	recordType, ok := observationTypeOf(record)
	if !ok {
		return core.RecordField{}, false, nil
	}
	value, err := core.NewRawValue(core.ValueStatePresent, recordType)
	if err != nil {
		return core.RecordField{}, false, err
	}
	field, err := core.NewTextField(keyType, "", value)
	if err != nil {
		return core.RecordField{}, false, err
	}
	return field, true, nil
}

// observationTypeOf は事象の種別に使う `type` の値を返す。
func observationTypeOf(record Record) (string, bool) {
	if line, found := record.LineOfType(LineTypeSyscall); found {
		return line.Type(), true
	}
	for _, line := range record.Lines() {
		if line.Type() != "" {
			return line.Type(), true
		}
	}
	return "", false
}
