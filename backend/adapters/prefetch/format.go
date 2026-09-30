package prefetch

import (
	"bytes"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserID は本 package の走査器を指す識別子である。parserVersion の材料になる。
const ParserID = "windows-prefetch"

// FormatKeyPrefetch は Windows の Prefetch の file (拡張子 .pf) である。
const FormatKeyPrefetch core.FormatKey = "windows_prefetch"

// HasSignature は、file の先頭の byte 列 head が Prefetch の file の署名を持つかを返す。
// 圧縮した file は先頭に `MAM` と圧縮の方式 4 を、圧縮していない file は位置 4 に `SCCA` を持つ。
func HasSignature(head []byte) bool {
	return bytes.HasPrefix(head, []byte(compressedSignature+"\x04")) ||
		len(head) >= offsetSignature+4 && string(head[offsetSignature:offsetSignature+4]) == uncompressedMagic
}

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{
		{
			Key: FormatKeyPrefetch, ParserID: ParserID,
			PositionKind: core.PositionKindByteRange,
			SpecInput:    core.FormatSpecInputRejected,
		},
	}
}
