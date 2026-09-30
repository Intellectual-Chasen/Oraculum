package apache

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// 対応表が写す欄の意味の出典は Apache HTTP Server のログ仕様である。
// 表に載る欄は語彙の項目を semantic に持ち、表に載らない欄はこの入力形式に固有の意味を持つ。

// accessSemanticByItem は combined の欄と語彙の項目の対応である。
//
// ident は組み込みの combined がリテラルの - を置く位置であり、requestLine は要求行全体の
// 原資料の文字列であり、どちらもこの入力形式に固有の意味を持つ。要求行から導く 3 件の意味は
// 下記が持つ。
var accessSemanticByItem = map[AccessItemName]core.SemanticKey{
	AccessItemClientIP:    core.SemanticKeyConnectionSourceAddress,
	AccessItemUser:        core.SemanticKeyAccountName,
	AccessItemRequestTime: core.SemanticKeyEventTime,
	AccessItemStatusCode:  core.SemanticKeyHttpStatusCode,
	AccessItemReplyBytes:  core.SemanticKeyHttpResponseBytes,
	AccessItemReferer:     core.SemanticKeyHttpReferrer,
	AccessItemUserAgent:   core.SemanticKeyHttpUserAgent,
}

// 要求行から導く項目の語彙の項目。値を導くのは取り込みの実行であり、意味を決めるのは
// 要求行の文字列を読む本 adapter である。
const (
	// SemanticRequestMethod は要求行の method の意味である。
	SemanticRequestMethod = core.SemanticKeyHttpRequestMethod
	// SemanticRequestTarget は要求行の要求対象の意味である。path と query string を
	// 分けないため、URL 全体の意味を持つ。
	SemanticRequestTarget = core.SemanticKeyHttpRequestUrl
	// SemanticRequestVersion は要求行が書いた HTTP のバージョンの意味である。
	SemanticRequestVersion = core.SemanticKeyHttpRequestVersion
)

// AccessSemanticOfItem は 1 つの欄の語彙の項目を返す。対応表に無い欄には空の値を返す。
func AccessSemanticOfItem(name AccessItemName) core.SemanticKey {
	return accessSemanticByItem[name]
}
