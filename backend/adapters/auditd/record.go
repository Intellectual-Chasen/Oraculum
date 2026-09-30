package auditd

import "slices"

// 行の種別のうち、本 package が欄の意味を読む文字列。
const (
	// LineTypeSyscall は事象の本体である。実行主体とプロセス番号を持つ。
	LineTypeSyscall = "SYSCALL"
	// LineTypeExecve は実行した引数を持つ。
	LineTypeExecve = "EXECVE"
	// LineTypeCwd は作業 directory を持つ。
	LineTypeCwd = "CWD"
	// LineTypePath は触れた path とその属性を持つ。1 事象に複数現れる。
	LineTypePath = "PATH"
	// LineTypeProctitle は引数列を NUL 区切りで連結した byte 列の 16 進表記を持つ。
	LineTypeProctitle = "PROCTITLE"
)

// syscallCompanionTypes は、同じ事象に SYSCALL の行があって初めて現れる種別である。
//
// この種別の行だけを持つ事象が収集元の先頭にあるとき、収集元の先頭が事象の途中で
// 切れている。
var syscallCompanionTypes = []string{
	LineTypeExecve, LineTypeCwd, LineTypePath, LineTypeProctitle,
}

// Record は auditd の 1 事象を、原文と位置と行ごとの文字列へ分けた結果として持つ。
//
// 値の意味付けを持たない。時刻の正規化と 16 進の復号は行わない。原資料の文字列と位置までを返す。
type Record struct {
	event      EventKey
	rawText    string
	lineNumber int64
	lineCount  int64
	byteOffset int64
	byteLength int64
	lineEnding string
	lines      []Line
}

// EventKey は 1 事象を指す `msg=audit(<秒>.<ミリ秒>:<連番>)` の値である。
//
// **連番だけを鍵にしない。** 連番は auditd の起動ごとに振り直され、値が増え続けない。
type EventKey struct {
	// RawText は括弧の内側の原資料の文字列である。`949395600.100:4242` の形を取る。
	RawText string
	// Seconds は UNIX epoch 秒である。
	Seconds int64
	// Milliseconds は秒未満のミリ秒である。
	Milliseconds int64
	// Serial は auditd が事象へ振った連番である。
	Serial int64
}

// Event は事象を指す鍵を返す。
func (r Record) Event() EventKey {
	return r.event
}

// RawText は末尾の改行を除いたレコードの原文を返す。レコードが複数行に分かれるとき、
// 行と行の間の改行を含む。
//
// **この事象の行だけを連ねた文字列である。** 同じ事象の行が連続しない収集元では、
// ByteOffset から ByteLength の範囲に別の事象の行が入るため、その範囲の byte 列とは
// 一致しない。
func (r Record) RawText() string {
	return r.rawText
}

// LineNumber はレコードの先頭の行の行番号を返す。1 起点。
func (r Record) LineNumber() int64 {
	return r.lineNumber
}

// LineCount はレコードが占める行数を返す。
func (r Record) LineCount() int64 {
	return r.lineCount
}

// ByteOffset は収集元の先頭からレコードの先頭までの byte 数を返す。0 起点。
func (r Record) ByteOffset() int64 {
	return r.byteOffset
}

// ByteLength は、先頭の行の先頭から最後の行の末尾までの byte 数を返す。
// 最後の行の改行の byte 数を含まない。
//
// 同じ事象の行が連続しない収集元では、この範囲に別の事象の行が入る。
// レコードが持つ行の数は LineCount が返す。
func (r Record) ByteLength() int64 {
	return r.byteLength
}

// LineEnding はレコードの末尾の改行の文字列を返す。"\r\n" か "\n" か、末尾に改行が無いとき ""。
func (r Record) LineEnding() string {
	return r.lineEnding
}

// Lines は原文の並び順で行を返す。返した slice の変更は Record に及ばない。
func (r Record) Lines() []Line {
	return slices.Clone(r.lines)
}

// LinesOfType は種別に一致する行を原文の並び順で返す。
func (r Record) LinesOfType(recordType string) []Line {
	matched := make([]Line, 0, len(r.lines))
	for _, line := range r.lines {
		if line.recordType == recordType {
			matched = append(matched, line)
		}
	}
	return matched
}

// LineOfType は種別に一致する最初の行を返す。
// ok が偽になるのは、その種別の行を事象が持たないときである。
func (r Record) LineOfType(recordType string) (Line, bool) {
	for _, line := range r.lines {
		if line.recordType == recordType {
			return line, true
		}
	}
	return Line{}, false
}

// HasSyscallCompanionOnly は、SYSCALL の行を持たないまま、SYSCALL がある事象にだけ
// 現れる種別の行を持つかを返す。
func (r Record) HasSyscallCompanionOnly() bool {
	if _, found := r.LineOfType(LineTypeSyscall); found {
		return false
	}
	for _, line := range r.lines {
		if slices.Contains(syscallCompanionTypes, line.recordType) {
			return true
		}
	}
	return false
}

// Line は事象を構成する 1 行を持つ。
type Line struct {
	recordType string
	rawText    string
	lineNumber int64
	byteOffset int64
	items      []Item
}

// Type は `type=` の値を返す。読めなかった行では空文字列を返す。
func (l Line) Type() string {
	return l.recordType
}

// RawText は改行を除いた行の原文を返す。
func (l Line) RawText() string {
	return l.rawText
}

// LineNumber は収集元の中の行番号を返す。1 起点。
func (l Line) LineNumber() int64 {
	return l.lineNumber
}

// ByteOffset は収集元の先頭から行頭までの byte 数を返す。0 起点。
func (l Line) ByteOffset() int64 {
	return l.byteOffset
}

// Items は原文の並び順で key=value を返す。返した slice の変更は Line に及ばない。
func (l Line) Items() []Item {
	return slices.Clone(l.items)
}

// Item は key に一致する最初の項目を返す。
//
// **生の値と表示用の値を分けて取り出す。** interpreted が真のとき 0x1D の後ろの項目を、
// 偽のとき前の項目を対象にする。`uid=0` と `UID="root"` は key の文字列が異なるが、
// 文字列の違いに頼らず立場で選べるようにする。
func (l Line) Item(key string, interpreted bool) (Item, bool) {
	for _, item := range l.items {
		if item.key == key && item.interpreted == interpreted {
			return item, true
		}
	}
	return Item{}, false
}

// Item は 1 つの key=value を持つ。
type Item struct {
	key         string
	rawValue    string
	value       string
	quote       byte
	interpreted bool
	byteOffset  int64
	inner       []Item
}

// Key は key の文字列を返す。
func (i Item) Key() string {
	return i.key
}

// RawValue は原文の value の文字列を返す。引用符で囲む形では外側の引用符を含む。
func (i Item) RawValue() string {
	return i.rawValue
}

// Value は外側の引用符を外した値を返す。
//
// 原資料に実在する値を欄の不在の代用にしない。`hostname=?` は "?" の 1 文字を返し、
// `key=(null)` は "(null)" を返す。key の不在は Line.Item の ok が偽になる。
func (i Item) Value() string {
	return i.value
}

// Quoted は value が引用符で囲まれていたかを返す。
func (i Item) Quoted() bool {
	return i.quote != 0
}

// Interpreted は、auditd が数値から求めた表示用の値であるかを返す。
// 0x1D より後ろにある項目が真になる。
func (i Item) Interpreted() bool {
	return i.interpreted
}

// ByteOffset は行の先頭から key の開始までの byte 数を返す。0 起点。
func (i Item) ByteOffset() int64 {
	return i.byteOffset
}

// Inner は value の内側にあるもう 1 階層の key=value を返す。
//
// `type=USER_START` の `msg='op=PAM:session_open ... res=success'` が持つ。
// 内側を持たない項目では要素数 0 である。
func (i Item) Inner() []Item {
	return slices.Clone(i.inner)
}
