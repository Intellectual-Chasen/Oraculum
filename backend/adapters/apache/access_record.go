package apache

import "slices"

// AccessItemName は combined のアクセスログの欄を識別する。
type AccessItemName string

// combined の 9 項目。字面は LogFormat 指定
// (%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-Agent}i") と対応する。
const (
	// AccessItemClientIP は要求元 host (%h) の欄である。
	AccessItemClientIP AccessItemName = "clientIp"
	// AccessItemIdent は RFC 1413 の identd による識別 (%l) の欄である。
	AccessItemIdent AccessItemName = "ident"
	// AccessItemUser は HTTP 認証の利用者名 (%u) の欄である。
	AccessItemUser AccessItemName = "user"
	// AccessItemRequestTime は要求を受け取った時刻 (%t) の欄である。
	AccessItemRequestTime AccessItemName = "requestTime"
	// AccessItemRequestLine は要求行 (%r) の欄である。
	AccessItemRequestLine AccessItemName = "requestLine"
	// AccessItemStatusCode は最後の応答の状態符号 (%>s) の欄である。
	AccessItemStatusCode AccessItemName = "statusCode"
	// AccessItemReplyBytes は client へ返した本体の大きさ (%b) の欄である。
	AccessItemReplyBytes AccessItemName = "replyBytes"
	// AccessItemReferer は Referer ヘッダ (%{Referer}i) の欄である。
	AccessItemReferer AccessItemName = "referer"
	// AccessItemUserAgent は User-Agent ヘッダ (%{User-Agent}i) の欄である。
	AccessItemUserAgent AccessItemName = "userAgent"
)

// accessItemOrder は combined の欄の並び順である。
var accessItemOrder = []AccessItemName{
	AccessItemClientIP, AccessItemIdent, AccessItemUser, AccessItemRequestTime,
	AccessItemRequestLine, AccessItemStatusCode, AccessItemReplyBytes,
	AccessItemReferer, AccessItemUserAgent,
}

// AccessItem は欄の原資料の文字列と位置、引用符と角括弧の外側を外した値を持つ。
type AccessItem struct {
	name       AccessItemName
	rawText    string
	value      string
	byteOffset int64
}

// Name は欄の名前を返す。
func (i AccessItem) Name() AccessItemName { return i.name }

// RawValue は外側の引用符または角括弧を含む欄の原資料の文字列を返す。
func (i AccessItem) RawValue() string { return i.rawText }

// Value は外側の引用符または角括弧を外し、引用符内の escape を復号した値を返す。
func (i AccessItem) Value() string { return i.value }

// ByteOffset はレコード内の RawValue の開始 byte offset を 0 起点で返す。
func (i AccessItem) ByteOffset() int64 { return i.byteOffset }

// AccessRecord はアクセスログの 1 レコードの原文と位置と、読めた欄を持つ。
type AccessRecord struct {
	rawText    string
	lineEnding string
	lineNumber int64
	byteOffset int64
	items      []AccessItem
}

// RawText は改行を除いた原文を返す。
func (r AccessRecord) RawText() string { return r.rawText }

// LineEnding は LF、CR LF、または末尾の改行無しを表す空文字列を返す。
func (r AccessRecord) LineEnding() string { return r.lineEnding }

// LineNumber は収集元内の行番号を 1 起点で返す。
func (r AccessRecord) LineNumber() int64 { return r.lineNumber }

// ByteOffset は収集元内の行頭の byte offset を 0 起点で返す。
func (r AccessRecord) ByteOffset() int64 { return r.byteOffset }

// Items は原文の順序で項目を返す。slice の変更は AccessRecord に及ばない。
func (r AccessRecord) Items() []AccessItem { return slices.Clone(r.items) }

// Item は指定した欄を返す。文字列の分割で読めなかった欄では ok が偽になる。
func (r AccessRecord) Item(name AccessItemName) (AccessItem, bool) {
	for _, item := range r.items {
		if item.name == name {
			return item, true
		}
	}
	return AccessItem{}, false
}
