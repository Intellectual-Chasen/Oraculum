package squid

import (
	"fmt"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserID は本 package の走査器を指す識別子である。parserVersion の材料になる。
const ParserID = "squid-combined"

// 本 package が読める入力形式の識別子。
const (
	// FormatKeyCombined は Squid に組み込みの combined の並びのログである。
	FormatKeyCombined core.FormatKey = "squid_combined"
	// FormatKeyCombinedRequestBytes は、combined の状態符号と応答 byte 数の間に要求の
	// byte 数の欄を 1 つ持つ並びのログである。
	FormatKeyCombinedRequestBytes core.FormatKey = "squid_combined_request_bytes"
	// FormatKeyLogFormat は、欄の並びを取り込みの指定が渡す logformat で定めるログである。
	//
	// **並びそのものを識別子に載せない。** 識別子が指すのは、並びを取り込みの指定から
	// 受け取る仕組みである。読んだ並びは SourceIdentity.FormatSpec が持つ。
	FormatKeyLogFormat core.FormatKey = "squid_logformat"
)

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。形式を 1 つ足す作業は本 slice への追加で完結する。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{
		{
			Key: FormatKeyCombined, ParserID: ParserID,
			PositionKind: core.PositionKindLineNumber,
			SpecInput:    core.FormatSpecInputRejected, FormatSpec: CombinedSpec,
		},
		{
			Key: FormatKeyCombinedRequestBytes, ParserID: ParserID,
			PositionKind: core.PositionKindLineNumber,
			SpecInput:    core.FormatSpecInputRejected, FormatSpec: CombinedRequestBytesSpec,
		},
		{
			Key: FormatKeyLogFormat, ParserID: ParserID,
			PositionKind: core.PositionKindLineNumber,
			SpecInput:    core.FormatSpecInputRequired,
		},
	}
}

// LayoutForSpec は欄の並びの指定を並びへ直す。
//
// 宣言が定める並びと、取り込みの指定が渡した並びの、どちらも同じ文字列として受け取る
// (core.InputFormat.FormatSpecFor が 2 つのどちらであるかを決める)。
// 指定は logformat の文字列、組み込みの logformat の名前 (builtInLogFormats)、squid.conf の
// `logformat <名前> <logformat>` の 1 行のいずれかである。並びの Spec は展開した logformat である。
func LayoutForSpec(formatSpec string) (Layout, error) {
	spec := strings.TrimSpace(formatSpec)
	if directive, rest, found := strings.Cut(spec, " "); found && directive == "logformat" {
		_, spec, _ = strings.Cut(strings.TrimLeft(rest, " \t"), " ")
		spec = strings.TrimLeft(spec, " \t")
	}
	if builtIn, found := builtInLogFormats[spec]; found {
		spec = builtIn
	}
	layout, err := ParseLogFormat(spec)
	if err != nil {
		return Layout{}, fmt.Errorf("building the Squid item order: %w", err)
	}
	return layout, nil
}

// builtInLogFormats は Squid に組み込みの logformat の名前と文字列である。
//
// **文字列の出典は Squid の logformat と icap_log の文書である。** combined は本 package の
// CombinedSpec と ident の位置が違う。文書の combined は ident の位置に literal の - を置く。
var builtInLogFormats = map[string]string{
	"squid":      `%ts.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`,
	"common":     `%>a - %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st %Ss:%Sh`,
	"combined":   `%>a - %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`,
	"referrer":   `%ts.%03tu %>a %{Referer}>h %ru`,
	"useragent":  `%>a [%tl] "%{User-Agent}>h"`,
	"icap_squid": `%ts.%03tu %6icap::tr %>A %icap::to/%03icap::Hs %icap::<st %icap::rm %icap::ru %un -/%icap::<A -`,
}
