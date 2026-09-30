package squid

import (
	"net"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 対応表が写す欄の意味の出典は Squid の logformat のページである。表に載る欄は語彙の項目を
// semantic に持ち、表に載らない欄は combined に固有の意味を持つ。

// semanticByItem は欄と語彙の項目の対応である。表に載らない欄は Squid に固有の意味を持つ。
//
// ident は組み込みの combined がリテラルの - を置く位置であり、requestLine は要求行
// 全体の原資料の文字列であり、squidStatus は Squid の要求処理の結果と上位への転送の経路である。
// 要求行から導く 3 件の意味は下記が持つ。%rm の requestMethod は要求行から導く method と
// 同じ意味である。
var semanticByItem = map[ItemName]core.SemanticKey{
	ItemClientIP:      core.SemanticKeyConnectionSourceAddress,
	ItemUser:          core.SemanticKeyAccountName,
	ItemRequestTime:   core.SemanticKeyEventTime,
	ItemStatusCode:    core.SemanticKeyHttpStatusCode,
	ItemRequestBytes:  core.SemanticKeyHttpRequestBytes,
	ItemReplyBytes:    core.SemanticKeyHttpResponseBytes,
	ItemReferer:       core.SemanticKeyHttpReferrer,
	ItemUserAgent:     core.SemanticKeyHttpUserAgent,
	ItemRequestMethod: core.SemanticKeyHttpRequestMethod,
}

// 要求行から導く項目の語彙の項目。値を導くのは取り込みの実行であり、意味を決めるのは
// 要求行の文字列を読む本 adapter である。
const (
	// SemanticRequestMethod は要求行の method の意味である。
	SemanticRequestMethod = core.SemanticKeyHttpRequestMethod
	// SemanticRequestTargetPort は要求先の port の意味である。
	SemanticRequestTargetPort = core.SemanticKeyConnectionDestinationPort
	// SemanticRequestVersion は要求行の 3 番目の token が書いた HTTP のバージョンの意味である。
	// ParseRequestLine が `HTTP/` の接頭辞とバージョンの文字列を確かめたうえで Protocol に置く。
	SemanticRequestVersion = core.SemanticKeyHttpRequestVersion
)

// SemanticOfItem は 1 つの欄の語彙の項目を返す。対応表に無い欄には空の値を返す。
func SemanticOfItem(name ItemName) core.SemanticKey {
	return semanticByItem[name]
}

// RequestTargetHostSemantic は要求先の host の文字列の形から語彙の項目を選ぶ。
//
// 文字列が IP アドレスのときは接続先 IP アドレス、文字列がホスト名のときは接続先ホスト名に
// なる。authority を書いていない要求先では
// host の文字列が無く、空の値を返す。
func RequestTargetHostSemantic(requestLine RequestLine) core.SemanticKey {
	if requestLine.AuthorityHost == nil {
		return ""
	}
	if net.ParseIP(*requestLine.AuthorityHost) != nil {
		return core.SemanticKeyConnectionDestinationAddress
	}
	return core.SemanticKeyConnectionDestinationHostname
}
