package apache

import (
	"fmt"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AccessRequestLine は要求行の 3 token の原資料の文字列を持つ。
//
// **Apache の要求先は origin-form が前提であり、authority を切り出さない。** Squid の
// %ru と異なり、Apache の %r は自身が origin server であるため absolute-URI を持たない。
type AccessRequestLine struct {
	// Method は要求 method の原資料の文字列である。
	Method string
	// Target は要求対象の原資料の文字列である。path と query string を分けない。
	Target string
	// Protocol は HTTP/ とバージョンの原資料の文字列である。
	Protocol string
}

// ParseAccessRequestLine は %r の欄を 3 token へ分ける。
//
// 区切りは空白であり、method と target と protocol の意味は RFC 7230 の request-line
// 文法に基づく。3 token に満たない要求行と、protocol が `HTTP/` で始まらない要求行は
// 失敗にする。
func ParseAccessRequestLine(record AccessRecord) (AccessRequestLine, *core.ImportFailure) {
	const shape = "method, request target, and HTTP/version separated by spaces"
	item, found := record.Item(AccessItemRequestLine)
	if !found {
		return AccessRequestLine{}, accessSemanticFailure(record, AccessItemRequestLine, shape, nil)
	}
	raw := item.Value()
	problem := func(reason string) (AccessRequestLine, *core.ImportFailure) {
		return AccessRequestLine{}, accessSemanticFailure(record, AccessItemRequestLine, shape,
			fmt.Errorf("%s in request %q", reason, raw))
	}
	method, rest, hasMethod := strings.Cut(raw, " ")
	target, protocol, hasTarget := strings.Cut(rest, " ")
	switch {
	case method == "":
		return problem("missing method")
	case !hasMethod:
		return problem("missing request target")
	case target == "":
		return problem("missing request target")
	case !hasTarget || protocol == "":
		return problem("missing protocol")
	}
	if extra := strings.IndexByte(protocol, ' '); extra >= 0 {
		return problem("extra request token")
	}
	if !strings.HasPrefix(protocol, "HTTP/") || protocol == "HTTP/" {
		return problem("invalid protocol")
	}
	return AccessRequestLine{Method: method, Target: target, Protocol: protocol}, nil
}
