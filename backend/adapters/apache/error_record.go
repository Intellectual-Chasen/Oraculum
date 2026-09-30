package apache

import "slices"

// ErrorItemName はエラーログの欄を識別する。
type ErrorItemName string

// エラーログの欄。原資料の文字列の並びは
// [%{u}t] [%-m:%l] [pid %P:tid %T] [client %a] %M である。pid と tid と client の
// IP アドレスと port を別項目に分け、本文を最後の 1 項目として保つ。
const (
	// ErrorItemTime はログを出力した時刻の欄である。
	ErrorItemTime ErrorItemName = "time"
	// ErrorItemModule は出力した module の欄である。
	ErrorItemModule ErrorItemName = "module"
	// ErrorItemSeverity は重大度の欄である。
	ErrorItemSeverity ErrorItemName = "severity"
	// ErrorItemPid は出力した process の識別子の欄である。
	ErrorItemPid ErrorItemName = "pid"
	// ErrorItemTid は出力した thread の識別子の欄である。
	ErrorItemTid ErrorItemName = "tid"
	// ErrorItemClientIP は要求元の IP アドレスの欄である。
	ErrorItemClientIP ErrorItemName = "clientIp"
	// ErrorItemClientPort は要求元の port の欄である。
	ErrorItemClientPort ErrorItemName = "clientPort"
	// ErrorItemMessage は本文の欄である。
	ErrorItemMessage ErrorItemName = "message"
)

// ErrorItem は欄の原資料の文字列と位置と値を持つ。
type ErrorItem struct {
	name       ErrorItemName
	rawText    string
	value      string
	byteOffset int64
}

// Name は欄の名前を返す。
func (i ErrorItem) Name() ErrorItemName { return i.name }

// RawValue は欄の原資料の文字列を返す。角括弧の中の複数の項目に分けた欄では、分けた後の文字列を
// 持つ (角括弧そのものは含まない)。
func (i ErrorItem) RawValue() string { return i.rawText }

// Value は復号後の値を返す。本 adapter は逆斜線 escape を復号しないため、RawValue と
// 同じ文字列を返す。
func (i ErrorItem) Value() string { return i.value }

// ByteOffset はレコード内の RawValue の開始 byte offset を 0 起点で返す。
func (i ErrorItem) ByteOffset() int64 { return i.byteOffset }

// ErrorRecord はエラーログの 1 レコードの原文と位置と、読めた欄を持つ。
type ErrorRecord struct {
	rawText    string
	lineEnding string
	lineNumber int64
	byteOffset int64
	items      []ErrorItem
}

// RawText は改行を除いた原文を返す。
func (r ErrorRecord) RawText() string { return r.rawText }

// LineEnding は LF、CR LF、または末尾の改行無しを表す空文字列を返す。
func (r ErrorRecord) LineEnding() string { return r.lineEnding }

// LineNumber は収集元内の行番号を 1 起点で返す。
func (r ErrorRecord) LineNumber() int64 { return r.lineNumber }

// ByteOffset は収集元内の行頭の byte offset を 0 起点で返す。
func (r ErrorRecord) ByteOffset() int64 { return r.byteOffset }

// Items は原文の順序で項目を返す。slice の変更は ErrorRecord に及ばない。
func (r ErrorRecord) Items() []ErrorItem { return slices.Clone(r.items) }

// Item は指定した欄を返す。文字列の分割で読めなかった欄では ok が偽になる。
func (r ErrorRecord) Item(name ErrorItemName) (ErrorItem, bool) {
	for _, item := range r.items {
		if item.name == name {
			return item, true
		}
	}
	return ErrorItem{}, false
}
