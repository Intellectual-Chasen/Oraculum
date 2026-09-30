package apache

import "strings"

// accessTokenizeProblem は文字列の分割で確定した失敗の位置と、その場で期待していた形を持つ。
type accessTokenizeProblem struct {
	offset   int
	expected string
}

// tokenizeAccessLine は combined の 1 行を 9 項目へ分ける。
//
// 欄の区切りは引用符と角括弧の外の空白に限る。**未対応の並びを読み飛ばさない。** 9 項目に
// 満たない行と、9 項目の後に文字が残る行は、文字列の分割の失敗として返す。
func tokenizeAccessLine(raw string) ([]AccessItem, *accessTokenizeProblem) {
	items := make([]AccessItem, 0, len(accessItemOrder))
	pos := 0
	for index, name := range accessItemOrder {
		pos = skipSpaces(raw, pos)
		if pos >= len(raw) {
			return nil, &accessTokenizeProblem{offset: pos, expected: "the " + string(name) + " item"}
		}
		item, newPos, problem := scanAccessItem(raw, pos, name)
		if problem != nil {
			return nil, problem
		}
		items = append(items, item)
		pos = newPos
		if index < len(accessItemOrder)-1 {
			if pos >= len(raw) || raw[pos] != ' ' {
				return nil, &accessTokenizeProblem{offset: pos, expected: "a space before the next item"}
			}
		}
	}
	pos = skipSpaces(raw, pos)
	if pos < len(raw) {
		return nil, &accessTokenizeProblem{offset: pos, expected: "the end of the record after 9 items"}
	}
	return items, nil
}

func scanAccessItem(raw string, pos int, name AccessItemName) (AccessItem, int, *accessTokenizeProblem) {
	switch raw[pos] {
	case '[':
		rawText, value, newPos, ok := scanBracketed(raw, pos)
		if !ok {
			return AccessItem{}, pos, &accessTokenizeProblem{offset: pos, expected: "a closing ] for the " + string(name) + " item"}
		}
		return AccessItem{name: name, rawText: rawText, value: value, byteOffset: int64(pos)}, newPos, nil
	case '"':
		rawText, value, newPos, ok := scanQuoted(raw, pos)
		if !ok {
			return AccessItem{}, pos, &accessTokenizeProblem{offset: pos, expected: "a closing \" for the " + string(name) + " item"}
		}
		return AccessItem{name: name, rawText: rawText, value: value, byteOffset: int64(pos)}, newPos, nil
	default:
		rawText, newPos := scanUnquoted(raw, pos)
		return AccessItem{name: name, rawText: rawText, value: rawText, byteOffset: int64(pos)}, newPos, nil
	}
}

// skipSpaces は pos から続く ' ' の並びを進める。
func skipSpaces(raw string, pos int) int {
	for pos < len(raw) && raw[pos] == ' ' {
		pos++
	}
	return pos
}

// scanUnquoted は次の空白または行末までを 1 項目として返す。
func scanUnquoted(raw string, pos int) (string, int) {
	end := strings.IndexByte(raw[pos:], ' ')
	if end < 0 {
		return raw[pos:], len(raw)
	}
	return raw[pos : pos+end], pos + end
}

// scanBracketed は raw[pos] が '[' であることを前提に、対応する ']' までを返す。
// 角括弧の中に角括弧の入れ子と escape は無い。
func scanBracketed(raw string, pos int) (rawText, value string, newPos int, ok bool) {
	end := strings.IndexByte(raw[pos+1:], ']')
	if end < 0 {
		return "", "", pos, false
	}
	closing := pos + 1 + end
	return raw[pos : closing+1], raw[pos+1 : closing], closing + 1, true
}

// scanQuoted は raw[pos] が '"' であることを前提に、対応する閉じ引用符までを返す。
// \" は引用符 1 文字、\\ は逆斜線 1 文字に復号する。他の逆斜線 2 文字の並びは復号せず
// そのまま保持する (仕様に無い escape を値の破壊で読み替えない)。
func scanQuoted(raw string, pos int) (rawText, value string, newPos int, ok bool) {
	var decoded strings.Builder
	i := pos + 1
	for i < len(raw) {
		switch raw[i] {
		case '"':
			return raw[pos : i+1], decoded.String(), i + 1, true
		case '\\':
			if i+1 < len(raw) && (raw[i+1] == '"' || raw[i+1] == '\\') {
				decoded.WriteByte(raw[i+1])
				i += 2
				continue
			}
			decoded.WriteByte(raw[i])
			i++
		default:
			decoded.WriteByte(raw[i])
			i++
		}
	}
	return "", "", pos, false
}
