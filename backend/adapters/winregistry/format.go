package winregistry

import (
	"bytes"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserID は本 package の走査器を指す識別子である。parserVersion の材料になる。
const ParserID = "windows-registry-hive"

// FormatKeyHive は Windows の registry の hive の主 file である。同じ directory の
// `<主 file の名前>.LOG1` と `.LOG2` を transaction log として一緒に読む。
const FormatKeyHive core.FormatKey = "windows_registry_hive"

// LogSuffixes は主 file の名前に足して transaction log の file を指す接尾辞である。
//
// **並びが収集元の byte 列の中の並びである。** 主 file の後に、この順で連結する。
func LogSuffixes() []string {
	return []string{".LOG1", ".LOG2"}
}

// HasHiveSignature は、file の先頭の byte 列 head が hive の見出しの署名 `regf` を持つかを返す。
// transaction log も先頭に同じ署名の見出しを持つ。主 file と見分けるのは file 名の接尾辞
// (LogSuffixes) である。
func HasHiveSignature(head []byte) bool {
	return bytes.HasPrefix(head, []byte("regf"))
}

// Formats は本 package が読める入力形式の宣言である。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{
		{
			Key: FormatKeyHive, ParserID: ParserID,
			PositionKind: core.PositionKindByteRange,
			SpecInput:    core.FormatSpecInputRejected,
		},
	}
}
