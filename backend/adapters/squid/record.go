package squid

import "slices"

// ItemName は 1 レコードの欄を識別する。どの欄を持つかは Layout が定める。
type ItemName string

const (
	// ItemClientIP は接続元 IP の欄である。
	ItemClientIP ItemName = "clientIp"
	// ItemIdent は ident の欄である。
	ItemIdent ItemName = "ident"
	// ItemUser は利用者の欄である。
	ItemUser ItemName = "user"
	// ItemRequestTime は要求時刻の欄である。
	ItemRequestTime ItemName = "requestTime"
	// ItemRequestLine は要求行の欄である。
	ItemRequestLine ItemName = "requestLine"
	// ItemStatusCode は client へ返した状態符号の欄である。
	ItemStatusCode ItemName = "statusCode"
	// ItemUpstreamStatusCode は上位の server から受け取った状態符号の欄である。
	// combined は本欄を持たない。
	//
	// **状態符号の語彙の項目を持たない。** 語彙の状態符号は client へ返した応答を指し、
	// 上位から受け取った応答と別の意味である。
	ItemUpstreamStatusCode ItemName = "upstreamStatusCode"
	// ItemRequestBytes は要求の byte 数の欄である。combined は本欄を持たない。
	ItemRequestBytes ItemName = "requestBytes"
	// ItemReplyBytes は応答 byte 数の欄である。
	ItemReplyBytes ItemName = "replyBytes"
	// ItemReferer は Referer の欄である。
	ItemReferer ItemName = "referer"
	// ItemUserAgent は User-Agent の欄である。
	ItemUserAgent ItemName = "userAgent"
	// ItemSquidStatus は Squid の状態の欄である。
	ItemSquidStatus ItemName = "squidStatus"
	// ItemMimeType は応答の MIME 型の欄である。combined は本欄を持たない。
	ItemMimeType ItemName = "mimeType"
	// ItemUpstreamIP は上位へ転送した先の IP の欄である。combined は本欄を持たない。
	//
	// **接続先 IP の語彙の項目を持たない。** 語彙の接続先は client から見た接続先を指し、
	// proxy から見た上位への転送先と別の意味である。
	ItemUpstreamIP ItemName = "upstreamIp"
)

// ヘッダーの欄の名前の接頭辞。Referer と User-Agent 以外のヘッダーの欄は、接頭辞に
// logformat が書いたヘッダー名を続けた名前になる。
const (
	// RequestHeaderItemPrefix は要求ヘッダーの欄の接頭辞である。
	RequestHeaderItemPrefix = "requestHeader."
	// ResponseHeaderItemPrefix は応答ヘッダーの欄の接頭辞である。
	ResponseHeaderItemPrefix = "responseHeader."
)

// Record はレコードの原文と位置と、読めた項目を持つ。
type Record struct {
	rawText    string
	lineEnding string
	lineNumber int64
	byteOffset int64
	items      []Item
}

// RawText は改行を除いた原文を返す。
func (r Record) RawText() string { return r.rawText }

// LineEnding は LF、CR LF、または末尾の改行無しを表す空文字列を返す。
func (r Record) LineEnding() string { return r.lineEnding }

// LineNumber は収集元内の行番号を 1 起点で返す。
func (r Record) LineNumber() int64 { return r.lineNumber }

// ByteOffset は収集元内の行頭の byte offset を 0 起点で返す。
func (r Record) ByteOffset() int64 { return r.byteOffset }

// Items は原文の順序で項目を返す。slice の変更は Record に及ばない。
func (r Record) Items() []Item { return slices.Clone(r.items) }

// Item は指定した欄を返す。文字列の分割で読めなかった欄では ok が偽になる。
func (r Record) Item(name ItemName) (Item, bool) {
	for _, item := range r.items {
		if item.name == name {
			return item, true
		}
	}
	return Item{}, false
}

// Item は欄の原資料の文字列と位置、引用符の復号後の値を持つ。
type Item struct {
	name       ItemName
	rawText    string
	value      string
	byteOffset int64
	quoted     bool
	// time は時刻の欄の読み方である。角括弧で囲んだ既定の %tl と、時刻以外の欄では nil である。
	time *timeSpec
	// truncated は、行が途中で切れ、欄の値が後ろを持たないかである (Reader.Next)。
	truncated bool
}

// Truncated は、行が途中で切れ、欄の値が切れた位置で終わっているかを返す。
func (i Item) Truncated() bool { return i.truncated }

// Name は欄の名前を返す。
func (i Item) Name() ItemName { return i.name }

// RawValue は外側の引用符、角括弧、escape を含む欄の原資料の文字列を返す。
// core.RawAndNormalized の rawText に対応する。
func (i Item) RawValue() string { return i.rawText }

// Value は引用符を外し、Squid の引用符内の escape を復号した値を返す。
func (i Item) Value() string { return i.value }

// Quoted は欄が引用符で囲まれているかを返す。
func (i Item) Quoted() bool { return i.quoted }

// ByteOffset はレコード内の RawValue の開始 byte offset を 0 起点で返す。
func (i Item) ByteOffset() int64 { return i.byteOffset }

// ByteLength は外側の引用符を含む RawValue の byte 長を返す。
func (i Item) ByteLength() int64 { return int64(len(i.rawText)) }
