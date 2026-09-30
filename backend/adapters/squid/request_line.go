package squid

import (
	"fmt"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RequestLine は要求行の 3 token と authority の切り出し結果を持つ。
type RequestLine struct {
	// Method は要求 method の原資料の文字列である。
	Method string
	// RawTarget は %ru の token 全体の原資料の文字列である。
	RawTarget string
	// Protocol は HTTP/ とバージョンの原資料の文字列である。要求先だけを書いた欄から読んだときは空である。
	Protocol string
	// Scheme は Squid の escape を復号した URI の scheme 部分である。
	// scheme を書いていない要求先 (CONNECT の authority form、error:...、相対参照、*、
	// // で始まる要求先) では nil である。
	Scheme *string
	// Authority は Squid の escape を復号した URI の authority 部分である。userinfo と port を保持する。
	// authority を持たない要求先では nil である。
	Authority *string
	// AuthorityHost は Authority から userinfo と port を外した host である。
	// IPv6 の host は URI が書いた角括弧を外した 16 進の文字列である。
	// authority を持たない要求先では nil である。
	AuthorityHost *string
	// AuthorityPort は Authority が書いた port の原資料の文字列である。
	// port を書いていない authority と、authority を持たない要求先では nil である。
	// 区切りの : だけを書いて port の文字列が空である authority も nil である。
	AuthorityPort *string
}

const authorityShape = "a request target with a delimited authority or without authority"

// CarriesRequestTarget はレコードが要求先を書いた欄 (requestLine、%ru、%>ru) を持つかを返す。
func CarriesRequestTarget(record Record) bool {
	for _, name := range []ItemName{ItemRequestLine, ItemRequestURL, ItemClientRequestURL} {
		if _, found := record.Item(name); found {
			return true
		}
	}
	return false
}

// ParseRequestLine は要求行を分解し、要求先の authority を切り出す。
// scheme と既定の port は補わず、percent encoding を復号しない。
// authority を持たない error:...、相対参照、* は成功し、Authority が nil になる。
//
// 要求先は requestLine、%ru、%>ru の欄の順に、最初に見つかった欄から読む。%ru と %>ru から
// 読むときは、%rm の欄を method にし、Protocol を空にする。
func ParseRequestLine(record Record) (RequestLine, *core.ImportFailure) {
	item, found := record.Item(ItemRequestLine)
	const shape = "method, request target, and HTTP/version separated by spaces"
	if !found {
		for _, name := range []ItemName{ItemRequestURL, ItemClientRequestURL} {
			if target, hasTarget := record.Item(name); hasTarget {
				return parseRequestTarget(record, target)
			}
		}
		return RequestLine{}, semanticFailure(record, ItemRequestLine, shape, nil)
	}
	if !item.Quoted() {
		return RequestLine{}, semanticFailure(record, ItemRequestLine, "a quoted request line", nil)
	}
	raw := item.RawValue()
	// 要求行全体を囲む引用符を外し、token 内の escape は原資料の文字列のまま保つ。
	raw = raw[1 : len(raw)-1]
	problem := func(expected, reason string, position int) (RequestLine, *core.ImportFailure) {
		failure := semanticFailure(record, ItemRequestLine, expected, fmt.Errorf("%s in request %q", reason, raw))
		offset := record.byteOffset + item.byteOffset + 1 + int64(position)
		failure.ByteOffset = &offset
		return RequestLine{}, failure
	}
	method, rest, hasMethod := strings.Cut(raw, " ")
	target, protocol, hasTarget := strings.Cut(rest, " ")
	switch {
	case method == "":
		return problem(shape, "missing method", 0)
	case !hasMethod:
		return problem(shape, "missing request target", len(raw))
	case target == "":
		return problem(shape, "missing request target", len(method)+1)
	case !hasTarget || protocol == "":
		return problem(shape, "missing protocol", len(raw))
	}
	protocolStart := len(method) + 1 + len(target) + 1
	if extra := strings.IndexAny(protocol, " \t\r\n"); extra >= 0 {
		return problem(shape, "extra request token", protocolStart+extra)
	}
	if !strings.HasPrefix(protocol, "HTTP/") {
		return problem(shape, "invalid protocol", protocolStart)
	}
	if protocol == "HTTP/" {
		return problem(shape, "missing HTTP version", len(raw))
	}
	decodedMethod, decodedRest, _ := strings.Cut(item.Value(), " ")
	decodedTarget, _, _ := strings.Cut(decodedRest, " ")
	parts, issue := targetParts(decodedMethod, decodedTarget)
	if issue != nil {
		position := rawRequestOffset(raw, len(decodedMethod)+1+issue.offset)
		return problem(authorityShape, issue.reason, position)
	}
	return RequestLine{
		Method: method, RawTarget: target, Protocol: protocol,
		Scheme: parts.scheme, Authority: parts.authority,
		AuthorityHost: parts.host, AuthorityPort: parts.port,
	}, nil
}

// parseRequestTarget は要求先だけを書いた欄から authority を切り出す。
//
// 引用符で囲まない欄の値は escape を持たないため、失敗の位置は値の中の位置そのものである。
func parseRequestTarget(record Record, target Item) (RequestLine, *core.ImportFailure) {
	method, _ := record.Item(ItemRequestMethod)
	rawTarget, base := innerRawValue(target)
	parts, issue := targetParts(method.Value(), target.Value())
	if issue != nil {
		position := issue.offset
		if target.Quoted() {
			position = rawRequestOffset(rawTarget, issue.offset)
		}
		failure := semanticFailure(record, target.name, authorityShape,
			fmt.Errorf("%s in request target %q", issue.reason, rawTarget))
		offset := record.byteOffset + base + int64(position)
		failure.ByteOffset = &offset
		return RequestLine{}, failure
	}
	rawMethod, _ := innerRawValue(method)
	return RequestLine{
		Method: rawMethod, RawTarget: rawTarget,
		Scheme: parts.scheme, Authority: parts.authority,
		AuthorityHost: parts.host, AuthorityPort: parts.port,
	}, nil
}

// innerRawValue は欄の原資料の文字列から外側の引用符を外した文字列と、その文字列のレコード内の開始位置を返す。
func innerRawValue(item Item) (string, int64) {
	if item.Quoted() {
		return item.rawText[1 : len(item.rawText)-1], item.byteOffset + 1
	}
	return item.rawText, item.byteOffset
}

// rawRequestOffset は復号した値の byte 位置を、Squid escape を持つ原資料の文字列の位置へ戻す。
func rawRequestOffset(raw string, decodedOffset int) int {
	position := 0
	for decoded := 0; decoded < decodedOffset && position < len(raw); decoded++ {
		if raw[position] == '\\' {
			position++
		}
		position++
	}
	return position
}

type requestProblem struct {
	offset int
	reason string
}

// requestTarget は要求先の文字列を scheme と authority と host と port へ切り分けた結果で
// ある。4 つとも、その文字列を書いていない要求先では nil になる。
type requestTarget struct {
	scheme    *string
	authority *string
	host      *string
	port      *string
}

func targetParts(method, target string) (requestTarget, *requestProblem) {
	// Squid が組み立てた error URI は、元の method が CONNECT の場合も authority を持たない。
	if strings.HasPrefix(target, "error:") {
		return requestTarget{}, nil
	}
	var parts requestTarget
	var authority string
	start := 0
	if method == "CONNECT" && !strings.Contains(target, "://") {
		if at := strings.IndexAny(target, "/?#"); at >= 0 {
			return requestTarget{}, &requestProblem{at, "delimiter in CONNECT authority"}
		}
		authority = target
	} else {
		rest := target
		if !strings.HasPrefix(rest, "//") {
			scheme, after, found := strings.Cut(rest, ":")
			if !found || !validScheme(scheme) || !strings.HasPrefix(after, "//") {
				return requestTarget{}, nil
			}
			parts.scheme = &scheme
			rest = after
		}
		authority = rest[len("//"):]
		start = len(target) - len(authority)
		if end := strings.IndexAny(authority, "/?#"); end >= 0 {
			authority = authority[:end]
		}
	}
	if issue := validAuthority(authority); issue != nil {
		issue.offset += start
		return requestTarget{}, issue
	}
	parts.authority = &authority
	parts.host, parts.port = authorityHostAndPort(authority)
	return parts, nil
}

// authorityHostAndPort は validAuthority を通った authority を host と port へ分ける。
//
// validAuthority が角括弧の対応と port の 10 進数字を確かめた後に呼ぶ。host は userinfo と
// port を外した文字列で、URI の区切り記号である IPv6 の角括弧も外す。markii 形式の dstIP が
// IPv6 を角括弧無しで書くため、角括弧を保つと接続先 IP の条件が 2 つの文字列を等しいと
// 判定できない。
// %ru 全体の原資料の文字列は RawTarget が保つ。
//
// port の文字列が空である authority は port を nil で返す。空の文字列を port として持つと、
// port を書いた要求先と書いていない要求先を区別できなくなる。
func authorityHostAndPort(authority string) (*string, *string) {
	hostPort := authority
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		hostPort = authority[at+1:]
	}
	if strings.HasPrefix(hostPort, "[") {
		end := strings.IndexByte(hostPort, ']')
		host := hostPort[len("["):end]
		rest := hostPort[end+1:]
		if rest == "" {
			return &host, nil
		}
		return &host, presentPort(rest[len(":"):])
	}
	host, port, hasPort := strings.Cut(hostPort, ":")
	if !hasPort {
		return &hostPort, nil
	}
	return &host, presentPort(port)
}

func presentPort(port string) *string {
	if port == "" {
		return nil
	}
	return &port
}

func validScheme(scheme string) bool {
	for index := range len(scheme) {
		b := scheme[index]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
			continue
		}
		if index > 0 && ((b >= '0' && b <= '9') || b == '+' || b == '-' || b == '.') {
			continue
		}
		return false
	}
	return scheme != ""
}

func validAuthority(authority string) *requestProblem {
	for index := range len(authority) {
		if authority[index] < 0x20 || authority[index] == 0x7f {
			return &requestProblem{index, "control byte in authority"}
		}
	}
	hostPort := authority
	start := 0
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		if invalid := strings.IndexAny(authority[:at], "@[]"); invalid >= 0 {
			return &requestProblem{invalid, "invalid userinfo delimiter"}
		}
		start = at + 1
		hostPort = authority[start:]
	}
	if hostPort == "" {
		return &requestProblem{start, "missing authority host"}
	}
	if strings.HasPrefix(hostPort, "[") {
		end := strings.IndexByte(hostPort, ']')
		if end < 0 {
			return &requestProblem{len(authority), "unclosed host bracket"}
		}
		if end == 1 {
			return &requestProblem{start + 1, "empty bracketed host"}
		}
		if invalid := strings.IndexAny(hostPort[1:end], "[]"); invalid >= 0 {
			return &requestProblem{start + 1 + invalid, "invalid host bracket"}
		}
		rest := hostPort[end+1:]
		if rest == "" {
			return nil
		}
		if rest[0] != ':' {
			return &requestProblem{start + end + 1, "unexpected byte after host bracket"}
		}
		return decimalPort(rest[1:], start+end+len("]:"))
	}
	if invalid := strings.IndexAny(hostPort, "[]"); invalid >= 0 {
		return &requestProblem{start + invalid, "invalid host bracket"}
	}
	host, port, hasPort := strings.Cut(hostPort, ":")
	if host == "" {
		return &requestProblem{start, "missing authority host"}
	}
	if hasPort {
		return decimalPort(port, start+len(host)+1)
	}
	return nil
}

func decimalPort(port string, start int) *requestProblem {
	for index := range len(port) {
		if port[index] < '0' || port[index] > '9' {
			return &requestProblem{start + index, "non-decimal port"}
		}
	}
	return nil
}
