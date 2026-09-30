package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// decodedCommandFieldName は cmd から導いたコマンド行の項目の名前である。
const decodedCommandFieldName = "decodedCmd"

// 導き方を表す 2 つの文字列。分析者が画面で読む値であるため日本語で書く。
const (
	derivationDecodedCommand   = "cmd の値の符号化された文字列を Base64 と UTF-16LE で復号した行"
	derivationUndecodedCommand = "cmd の値が符号化された文字列を宣言しているが、" +
		"Base64 と UTF-16LE で読める文字列へ復号できない"
)

// decodedCommandField は cmd の値に埋め込まれた符号化された文字列を復号した項目を導く。
//
// **原資料の文字列を置き換えない。** cmd の項目は原資料の文字列をそのまま持ち続け、復号した行は
// 別の項目が持つ。
//
// 復号できたときは導出済み、符号化された文字列を宣言していながら復号できないときは
// 導出未確定の項目を返す。
//
// ok が偽になるのは次の 4 つである。
//
//  1. cmd の key が無いレコード
//  2. 符号化された文字列を持たないコマンド行
//  3. decodedCommandValue が値の検査を通せなかったとき
//  4. core.NewTextField が項目の検査を通せなかったとき
//
// **検査を通らない項目を応答へ入れない。** 3 と 4 で項目を除いても、原資料の文字列は cmd の
// 項目に残る。
//
// **復号そのものは入力形式を知らない** (backend/core/encoded_command.go)。本関数が持つのは
// どの key の値を渡すかだけである。
func decodedCommandField(record Record) (core.RecordField, bool) {
	field, found := record.Field(keyCommandLine)
	if !found {
		return core.RecordField{}, false
	}
	decoding := core.DecodeEncodedCommandLine(field.Value())
	if !decoding.Encoded {
		return core.RecordField{}, false
	}
	value, err := decodedCommandValue(decoding)
	if err != nil {
		return core.RecordField{}, false
	}
	derived, err := core.NewTextField(
		decodedCommandFieldName, core.SemanticKeyProcessDecodedCommandLine, value)
	if err != nil {
		return core.RecordField{}, false
	}
	return derived, true
}

// decodedCommandValue は復号の結果を値の状態へ直す。
func decodedCommandValue(decoding core.EncodedCommandDecoding) (core.RawAndNormalized, error) {
	if !decoding.Decoded {
		return core.NewDerivationUndeterminedValue(derivationUndecodedCommand)
	}
	return core.NewDerivedValue(decoding.CommandLine, derivationDecodedCommand)
}
