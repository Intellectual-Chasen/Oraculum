package auditd

import (
	"encoding/hex"
	"strconv"
	"strings"
)

// nulByte は proctitle が引数の区切りに使う byte である。
const nulByte = 0x00

// decodeHexText は 16 進表記の値を byte 列へ戻す。
//
// auditd は、空白や引用符を含む引数を 16 進表記で書き、含まない引数を引用符で囲んで
// 書く。**引用符で囲まれた
// 値を復号の対象にしない。** `a0="grep"` の `grep` は 16 進として読める文字列ではない。
//
// ok が偽になるのは、引用符で囲まれた値と、16 進として読めない値である。
func decodeHexText(item Item) (string, bool) {
	if item.Quoted() {
		return "", false
	}
	decoded, err := hex.DecodeString(item.Value())
	if err != nil {
		return "", false
	}
	return string(decoded), true
}

// decodeProctitle は proctitle の 16 進表記を、引数を空白で区切った 1 行へ直す。
//
// 復号した byte 列は引数を 0x00 で区切る。**区切りの 0x00 を空白に置き換える。**
// 制御文字のまま画面と log へ出さない。末尾の 0x00 は区切りではないため除く。
//
// ok が偽になるのは、16 進として読めない値である。
func decodeProctitle(item Item) (string, bool) {
	decoded, ok := decodeHexText(item)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimRight(decoded, string(rune(nulByte)))
	return strings.ReplaceAll(trimmed, string(rune(nulByte)), " "), true
}

// commandLineOf は EXECVE の引数から、実行したコマンド行を組む。
//
// 引数は `argc` の個数だけ `a0` から並ぶ。16 進表記の引数は復号した値を、引用符で
// 囲まれた引数は引用符を外した値を使う。**原資料の文字列は項目ごとに別に保つ。** 本関数が
// 返すのは正規化値である。
//
// **`argc` が名乗った個数を読めない行はコマンド行を組まない。** 引数を 1 つ欠いた
// 行を、完全なコマンド行として画面へ出さない。
//
// ok が偽になるのは、EXECVE の行が `a0` を 1 つも持たないときと、読めた引数の数が
// `argc` と異なるときである。
func commandLineOf(line Line) (string, bool) {
	arguments := argumentsOf(line, func(item Item) string {
		if decoded, ok := decodeHexText(item); ok {
			return decoded
		}
		return item.Value()
	})
	if len(arguments) == 0 {
		return "", false
	}
	if declared, named := argumentCountOf(line); named && declared != len(arguments) {
		return "", false
	}
	return strings.Join(arguments, " "), true
}

// argumentsOf は EXECVE の引数を並びの順で返す。値の取り出し方を呼び出し元が渡す。
//
// **分割された引数を 1 つに連結する。** 長い引数を kernel が `a1_len` と `a1[0]`、
// `a1[1]` の組へ分けることがある。分けられた断片を別の引数として並べない。
func argumentsOf(line Line, valueOf func(Item) string) []string {
	arguments := make([]string, 0, len(line.items))
	for index := 0; ; index++ {
		if item, found := line.Item(argumentKey(index), false); found {
			arguments = append(arguments, valueOf(item))
			continue
		}
		joined, split := splitArgumentOf(line, index, valueOf)
		if !split {
			break
		}
		arguments = append(arguments, joined)
	}
	return arguments
}

// splitArgumentOf は、分割された 1 つの引数の断片を連結して返す。
// split が偽になるのは、その位置の引数が分割の形でも並んでいないときである。
func splitArgumentOf(line Line, index int, valueOf func(Item) string) (string, bool) {
	parts := make([]string, 0, 4)
	for part := 0; ; part++ {
		item, found := line.Item(splitArgumentKey(index, part), false)
		if !found {
			break
		}
		parts = append(parts, valueOf(item))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, ""), true
}

// argumentCountOf は EXECVE が名乗った引数の個数を返す。
// named が偽になるのは、行が `argc` を持たないときと、10 進整数として読めないときである。
func argumentCountOf(line Line) (int, bool) {
	item, found := line.Item(argumentCountKey, false)
	if !found {
		return 0, false
	}
	count, err := strconv.Atoi(item.Value())
	if err != nil {
		return 0, false
	}
	return count, true
}

// argumentCountKey は EXECVE が引数の個数を名乗る key である。
const argumentCountKey = "argc"

// argumentKey は EXECVE の引数 1 つ分の key を返す。
func argumentKey(index int) string {
	return "a" + strconv.Itoa(index)
}

// splitArgumentKey は、分割された引数の断片 1 つ分の key を返す。
func splitArgumentKey(index, part int) string {
	return argumentKey(index) + "[" + strconv.Itoa(part) + "]"
}
