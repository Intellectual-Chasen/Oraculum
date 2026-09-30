package apache

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// ParserID は本 package の走査器を指す識別子である。parserVersion の材料になる。
const ParserID = "apache-http-server"

// 本 package が読める入力形式の識別子。
const (
	// FormatKeyAccessCombined は Apache HTTP Server の combined のアクセスログである。
	FormatKeyAccessCombined core.FormatKey = "apache_access_combined"
	// FormatKeyError は Apache HTTP Server のエラーログである。
	FormatKeyError core.FormatKey = "apache_error"
)

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{
		{
			Key: FormatKeyAccessCombined, ParserID: ParserID,
			PositionKind: core.PositionKindLineNumber,
			SpecInput:    core.FormatSpecInputRejected,
		},
		{
			Key: FormatKeyError, ParserID: ParserID,
			PositionKind: core.PositionKindLineNumber,
			SpecInput:    core.FormatSpecInputRejected,
		},
	}
}
