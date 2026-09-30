package markii

import (
	"strconv"
	"strings"
)

// 文字列の区切りと引用符。
const (
	fieldSeparator = ' '
	quote          = '"'
	keyValueSign   = '='
	// asciiLimit は ASCII の範囲の外を表す最小の byte である。UTF-8 の多 byte 文字は
	// すべて本値以上の byte で始まる。
	asciiLimit = 0x80
)

// tokenizeRecord は 1 レコードの原文を文字列へ分ける。
//
// 左から順に消費する。正規表現を使わない。value の内側を key の候補として見ない。
// 引用符付き value の中には key=value と同じ字面が入るため、素朴な正規表現は value の
// 内側の文字列を key として余計に拾う。除外を正規表現の後処理で行わない。
//
// 返す tokenizeProblem が nil でないのは、文字列の分割を続けられない形に該当したときである。
// そのときも読めた field までを返す。部分的に読めた範囲を捨てない。
func tokenizeRecord(rawText string) ([]Field, *tokenizeProblem) {
	if len(rawText) < fieldsStart {
		return nil, &tokenizeProblem{
			byteOffset: int64(len(rawText)),
			expected:   "a header of 29 bytes followed by one space",
			observed:   "a record of " + strconv.Itoa(len(rawText)) + " bytes",
		}
	}
	if rawText[fieldsStart-1] != fieldSeparator {
		return nil, &tokenizeProblem{
			byteOffset: fieldsStart - 1,
			expected:   "one space at byte offset 29",
			observed:   "the byte " + quoteByte(rawText[fieldsStart-1]),
		}
	}
	if problem := requireASCIIHeader(rawText); problem != nil {
		return nil, problem
	}
	// ヘッダーの内部の区切り。先頭 23 文字が日時、25 文字目から 5 文字が UTC からの
	// ずれであり、24 文字目は半角空白である。
	// 確かめないと、区切りを持たないヘッダーの 2 つの文字列が原文と食い違う。
	if rawText[dateTimeEnd] != fieldSeparator {
		return nil, &tokenizeProblem{
			byteOffset: dateTimeEnd,
			expected:   "one space at byte offset 23, between the date and time and the zone",
			observed:   "the byte " + quoteByte(rawText[dateTimeEnd]),
		}
	}

	var fields []Field
	position := fieldsStart
	for position < len(rawText) {
		field, next, problem := tokenizeField(rawText, position)
		if problem != nil {
			return fields, problem
		}
		fields = append(fields, field)
		position = next
		// 区切りを消費して末尾に達した。区切りの後には field が続くはずである。
		// 続かない形を成功として通すと、末尾の 1 byte が原文と食い違う。
		// **読めた field を捨てない。** 完成した field を足した後に失敗を返す。
		if position == len(rawText) && rawText[position-1] == fieldSeparator {
			return fields, &tokenizeProblem{
				byteOffset: int64(position - 1),
				expected:   "a key=value after the separator at byte offset " + strconv.Itoa(position-1),
				observed:   "the end of the record at byte offset " + strconv.Itoa(len(rawText)),
			}
		}
	}
	// レコードは 1 つ以上の key=value を持つ。
	// ヘッダーと半角空白だけの行を成功として通さない。
	if len(fields) == 0 {
		return nil, &tokenizeProblem{
			byteOffset: fieldsStart,
			expected:   "at least one key=value after the header",
			observed:   "the end of the record at byte offset " + strconv.Itoa(len(rawText)),
		}
	}
	return fields, nil
}

// requireASCIIHeader はヘッダーの 29 byte が ASCII だけであることを確かめる。
//
// 確かめずに位置で切ると、多 byte 文字が先頭にある入力で文字の途中を切る。
func requireASCIIHeader(rawText string) *tokenizeProblem {
	for index := 0; index < zoneEnd; index++ {
		if rawText[index] >= asciiLimit {
			return &tokenizeProblem{
				byteOffset: int64(index),
				expected:   "an ASCII byte inside the 29 byte header",
				observed:   "the byte " + quoteByte(rawText[index]),
			}
		}
	}
	return nil
}

// tokenizeField は position から 1 つの key=value を読み、次の key の開始位置を返す。
func tokenizeField(rawText string, position int) (Field, int, *tokenizeProblem) {
	keyStart := position
	// key は英字または下線で始まり、2 文字目以降は英数字と下線である。
	// 数字で始まる文字列を key にしない。
	if !isKeyFirstByte(rawText[keyStart]) {
		return Field{}, 0, &tokenizeProblem{
			byteOffset: int64(keyStart),
			expected:   "a key starting with a letter or an underscore",
			observed:   "the byte " + quoteByte(rawText[keyStart]),
		}
	}
	keyEnd := keyStart
	for keyEnd < len(rawText) && isKeyByte(rawText[keyEnd]) {
		keyEnd++
	}
	if keyEnd == len(rawText) || rawText[keyEnd] != keyValueSign {
		return Field{}, 0, &tokenizeProblem{
			byteOffset: int64(keyEnd),
			expected:   "the sign = after the key " + rawText[keyStart:keyEnd],
			observed:   observedAt(rawText, keyEnd),
		}
	}

	valueStart := keyEnd + 1
	rawValue, value, quoted, valueEnd, problem := tokenizeValue(rawText, valueStart)
	if problem != nil {
		return Field{}, 0, problem
	}
	// value を消費し終えた次の 1 文字が半角空白か行末であることを確かめてから、
	// 次の key の一致判定に進む。確かめないと、素朴な正規表現と同じ 9 個の誤りを再現する。
	if valueEnd < len(rawText) && rawText[valueEnd] != fieldSeparator {
		return Field{}, 0, &tokenizeProblem{
			byteOffset: int64(valueEnd),
			expected: "one space or the end of the record after the value of " +
				rawText[keyStart:keyEnd],
			observed: "the byte " + quoteByte(rawText[valueEnd]),
		}
	}

	field := Field{
		key:             rawText[keyStart:keyEnd],
		rawValue:        rawValue,
		value:           value,
		quoted:          quoted,
		keyByteOffset:   int64(keyStart),
		valueByteOffset: int64(valueStart),
	}
	next := valueEnd
	if next < len(rawText) {
		// 区切りの半角空白 1 個を消費する。
		next++
	}
	return field, next, nil
}

// tokenizeValue は valueStart から value を読み、原資料の文字列と復号後の値と引用符の有無と
// 終端の位置を返す。
//
// 引用符で囲む形では、引用符を 1 個読んだ時点で次の 1 文字を先読みする。引用符なら
// 引用符 2 個として 2 byte 消費し、引用符でなければ閉じ引用符とする。
// key="" の 4 文字が空の value になり、cmd="""powershell の形が値の先頭に引用符 1 個を
// 持つ value になる。**先読みを省くと 2 つを同じ形に読む。**
func tokenizeValue(rawText string, valueStart int) (rawValue, value string, quoted bool, valueEnd int, problem *tokenizeProblem) {
	if valueStart >= len(rawText) || rawText[valueStart] != quote {
		end := valueStart
		for end < len(rawText) && rawText[end] != fieldSeparator {
			end++
		}
		unquoted := rawText[valueStart:end]
		return unquoted, unquoted, false, end, nil
	}

	var decoded strings.Builder
	position := valueStart + 1
	for position < len(rawText) {
		if rawText[position] != quote {
			decoded.WriteByte(rawText[position])
			position++
			continue
		}
		// 引用符を読んだ。次の 1 文字が引用符なら引用符 2 個で、1 個へ復号する。
		if position+1 < len(rawText) && rawText[position+1] == quote {
			decoded.WriteByte(quote)
			position += 2
			continue
		}
		position++
		return rawText[valueStart:position], decoded.String(), true, position, nil
	}
	return "", "", false, 0, &tokenizeProblem{
		byteOffset: int64(valueStart),
		expected:   "a closing quote for the value starting at byte offset " + strconv.Itoa(valueStart),
		observed:   "the end of the record at byte offset " + strconv.Itoa(len(rawText)),
	}
}

// isKeyFirstByte は key の先頭に使える byte であるかを返す。英字と下線である。
func isKeyFirstByte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b == '_':
		return true
	default:
		return false
	}
}

// isKeyByte は key の 2 文字目以降に使える byte であるかを返す。
//
// 先頭に使える byte に数字を足したものである。文字種の定義を 1 か所に置く。
func isKeyByte(b byte) bool {
	return isKeyFirstByte(b) || (b >= '0' && b <= '9')
}
