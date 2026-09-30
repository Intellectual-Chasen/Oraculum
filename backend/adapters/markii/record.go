package markii

import "slices"

// ヘッダーの位置。レコードの形は固定長のヘッダー 29 文字と半角空白 1 個と key=value の
// 並びである。ヘッダーは ASCII だけで書かれ、文字数と byte 数が一致する。
// ASCII であることは tokenizeRecord が毎回確かめる。
const (
	// dateTimeEnd は日時の文字列の終端である。先頭 23 文字が日時である。
	dateTimeEnd = 23
	// zoneStart は UTC からのずれの文字列の開始である。25 文字目から 5 文字である。
	zoneStart = 24
	// zoneEnd は UTC からのずれの文字列の終端である。
	zoneEnd = 29
	// fieldsStart は key=value の並びの開始である。ヘッダー 29 文字の後に半角空白 1 個が入る。
	fieldsStart = 30
)

// Record は markii 形式の 1 レコードを文字列へ分けた結果を持つ。
//
// 値の意味付けを持たない。時刻の正規化、sn の 10 進整数への変換、observationKind の
// 判定は行わない。原資料の文字列と位置までを返す。
type Record struct {
	rawText    string
	lineNumber int64
	byteOffset int64
	lineEnding string
	fields     []Field
}

// RawText は改行を除いたレコードの原文を返す。CR LF の CR を含まない。
func (r Record) RawText() string {
	return r.rawText
}

// LineNumber は収集元の中の行番号を返す。1 起点。
func (r Record) LineNumber() int64 {
	return r.lineNumber
}

// ByteOffset は収集元の先頭から行頭までの byte 数を返す。0 起点。
// 改行の byte 数を算入した値である。
func (r Record) ByteOffset() int64 {
	return r.byteOffset
}

// LineEnding はレコードの改行の文字列を返す。"\r\n" か "\n" か、末尾に改行が無いとき ""。
func (r Record) LineEnding() string {
	return r.lineEnding
}

// DateTimeText はヘッダーの日時の原資料の文字列を返す。ok が偽になるのは、レコードが
// ヘッダーを読める長さを持たないときである。
//
// 文字列を返すところまでを行う。日時としての解釈は行わない。
func (r Record) DateTimeText() (string, bool) {
	if len(r.rawText) < zoneEnd {
		return "", false
	}
	return r.rawText[:dateTimeEnd], true
}

// ZoneText はヘッダーの UTC からのずれの原資料の文字列を返す。ok が偽になるのは、レコードが
// ヘッダーを読める長さを持たないときである。
func (r Record) ZoneText() (string, bool) {
	if len(r.rawText) < zoneEnd {
		return "", false
	}
	return r.rawText[zoneStart:zoneEnd], true
}

// Fields は原文の並び順で field を返す。返した slice の変更は Record に及ばない。
//
// 並び順を保つのは、key の並びが固定でないためである。同じ evt と subEvt の組でも key の
// 並びが複数ある。
func (r Record) Fields() []Field {
	return slices.Clone(r.fields)
}

// Field は key に一致する最初の field を返す。ok が偽になるのは、key がレコードに
// 出ないときである。
//
// 同じ key が 1 レコードに 2 回出た場合は原文の並びで最初のものを返す。個数は
// FieldCount が返す。**map の上書きで重複を消さない。**
func (r Record) Field(key string) (Field, bool) {
	for _, field := range r.fields {
		if field.key == key {
			return field, true
		}
	}
	return Field{}, false
}

// FieldCount は key に一致する field の個数を返す。key がレコードに出ないとき 0 を返す。
//
// key の不在と、値が空文字列である状態を分けるために置く。不在は 0、
// key="" は 1 で Field の Value が空文字列になる。
func (r Record) FieldCount(key string) int {
	count := 0
	for _, field := range r.fields {
		if field.key == key {
			count++
		}
	}
	return count
}

// Field はレコードの 1 つの key=value を持つ。
//
// 原資料の文字列と復号後の値の両方を持つ。引用符の有無も持つ。同じ key が引用符付きの形と
// 引用符無しの形の両方で現れることがあり、2 つが別の出所の値を持つ。引用符の有無を
// 捨てると、その違いを後の処理が読めなくなる。
type Field struct {
	key             string
	rawValue        string
	value           string
	quoted          bool
	keyByteOffset   int64
	valueByteOffset int64
}

// Key は key の文字列を返す。
func (f Field) Key() string {
	return f.key
}

// RawValue は原文の value の文字列を返す。引用符で囲む形では外側の引用符を含み、
// 内側の引用符 2 個をそのまま含む。
func (f Field) RawValue() string {
	return f.rawValue
}

// Value は外側の引用符を外し、引用符 2 個を引用符 1 個へ復号した値を返す。
//
// 原資料に実在する空文字列と "-" を欄の不在の代用にしない。key="" は空文字列を返し、
// key="-" は "-" の 1 文字を返す。key の不在は Record.Field の ok が偽になる。
func (f Field) Value() string {
	return f.value
}

// Quoted は value が引用符で囲まれていたかを返す。
func (f Field) Quoted() bool {
	return f.quoted
}

// KeyByteOffset はレコードの先頭から key の開始までの byte 数を返す。0 起点。
func (f Field) KeyByteOffset() int64 {
	return f.keyByteOffset
}

// ValueByteOffset はレコードの先頭から RawValue の開始までの byte 数を返す。0 起点。
// 引用符で囲む形では開く引用符の位置を指す。
//
// Value は復号後の文字列であり、原文の byte 範囲を持たない。範囲を持つのは RawValue
// だけである。収集元の中の位置は Record.ByteOffset に本値を足した値である。
func (f Field) ValueByteOffset() int64 {
	return f.valueByteOffset
}

// ValueByteLength は RawValue の byte 数を返す。
func (f Field) ValueByteLength() int64 {
	return int64(len(f.rawValue))
}
