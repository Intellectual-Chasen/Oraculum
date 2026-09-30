package royalts

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// ParserID は本 package の走査器を指す識別子である。parserVersion の材料になる。
const ParserID = "royalts-rdsconnection"

// FormatKeyRDSConnection は Royal TS/TSX 文書の RDP 接続項目である。
const FormatKeyRDSConnection core.FormatKey = "royalts_rds_connection"

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{
		{
			Key: FormatKeyRDSConnection, ParserID: ParserID,
			PositionKind: core.PositionKindSequenceNumber,
			SpecInput:    core.FormatSpecInputRejected,
		},
	}
}
