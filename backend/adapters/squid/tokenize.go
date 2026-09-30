package squid

import (
	"strconv"
	"strings"
)

func tokenizeRecord(raw string, layout Layout) ([]Item, *tokenizeProblem) {
	items := make([]Item, 0, len(layout.items))
	position := 0
	for index, spec := range layout.items {
		if spec.separator != "" {
			if !strings.HasPrefix(raw[position:], spec.separator) {
				return items, &tokenizeProblem{position,
					"the literal " + strconv.Quote(spec.separator) + " before the " + string(spec.name) + " item"}
			}
			position += len(spec.separator)
		}
		item, next, problem := scanItem(raw, position, spec, layout.separatorAfter(index))
		if problem != nil {
			return items, problem
		}
		items = append(items, item)
		position = next
	}
	if layout.trailer != "" {
		if !strings.HasPrefix(raw[position:], layout.trailer) {
			return items, &tokenizeProblem{position,
				"the literal " + strconv.Quote(layout.trailer) + " after the last item"}
		}
		position += len(layout.trailer)
	}
	if position != len(raw) {
		return items, &tokenizeProblem{position, "the end of the record after the last item"}
	}
	return items, nil
}

// scanItem は 1 つの欄を読む。
//
// 囲みを持たない欄は、直後の literal が最初に現れる位置までを値にする。直後の literal を
// 持たない末尾の欄は、直前の literal で区切る。
// 幅を持つ欄は、Squid が値の前 (右寄せ) または後ろ (左寄せ) に足した空白を値に含めない。
func scanItem(
	raw string, start int, spec layoutItem, nextSeparator string,
) (Item, int, *tokenizeProblem) {
	if spec.widthMin > 0 && !spec.leftAligned {
		for start < len(raw) && raw[start] == ' ' {
			start++
		}
	}
	if start >= len(raw) {
		return Item{}, start, &tokenizeProblem{start, "a value for the " + string(spec.name) + " item"}
	}
	var end int
	value := ""
	quoted := spec.enclosure == enclosureQuotes || spec.shellQuoted && raw[start] == '"'
	switch {
	case quoted:
		var problem *tokenizeProblem
		value, end, problem = scanQuoted(raw, start)
		if problem != nil {
			return Item{}, end, problem
		}
	case spec.enclosure == enclosureBrackets:
		if raw[start] != '[' {
			return Item{}, start,
				&tokenizeProblem{start, "an opening bracket of the " + string(spec.name) + " item"}
		}
		closeAt := strings.IndexByte(raw[start+1:], ']')
		if closeAt < 0 {
			return Item{}, len(raw),
				&tokenizeProblem{len(raw), "a closing bracket of the " + string(spec.name) + " item"}
		}
		end = start + 1 + closeAt + 1
		value = raw[start:end]
	case spec.time != nil:
		var problem *tokenizeProblem
		if end, problem = spec.time.scan(raw, start); problem != nil {
			return Item{}, end, problem
		}
		value = raw[start:end]
	default:
		// 区切りが残る行は、この欄の後ろに欄の並びが書いていない byte があることを表し、
		// 末尾の検査が退ける。
		delimiter := nextSeparator
		if delimiter == "" {
			delimiter = spec.separator
		}
		end = delimiterAt(raw, start, delimiter, spec.escaped)
		value = raw[start:end]
	}
	valueEnd := end
	if spec.leftAligned && !quoted {
		value = strings.TrimRight(value, " ")
		valueEnd = start + len(value)
	}
	if valueEnd == start {
		return Item{}, start, &tokenizeProblem{start, "a value for the " + string(spec.name) + " item"}
	}
	if strings.ContainsAny(raw[start:valueEnd], "\r\n\t\x00") {
		return Item{}, end, &tokenizeProblem{start, "an item without literal control separators"}
	}
	if spec.leftAligned {
		for end-start < spec.widthMin && end < len(raw) && raw[end] == ' ' {
			end++
		}
	}
	return Item{
		name: spec.name, rawText: raw[start:valueEnd], value: value,
		byteOffset: int64(start), quoted: quoted, time: spec.time,
	}, end, nil
}

// delimiterAt は start から delimiter が最初に現れる位置を返す。現れないときは raw の終端を
// 返す。escaped の値では、`\` に続く byte を区切りとして読まない。
func delimiterAt(raw string, start int, delimiter string, escaped bool) int {
	if delimiter == "" {
		return len(raw)
	}
	if !escaped {
		if at := strings.Index(raw[start:], delimiter); at >= 0 {
			return start + at
		}
		return len(raw)
	}
	for position := start; position < len(raw); position++ {
		if raw[position] == '\\' {
			position++
			continue
		}
		if strings.HasPrefix(raw[position:], delimiter) {
			return position
		}
	}
	return len(raw)
}

func scanQuoted(raw string, start int) (string, int, *tokenizeProblem) {
	if raw[start] != '"' {
		return "", start, &tokenizeProblem{start, "an opening quote"}
	}
	var value strings.Builder
	for position := start + 1; position < len(raw); position++ {
		b := raw[position]
		switch b {
		case '"':
			return value.String(), position + 1, nil
		case '\\':
			position++
			if position == len(raw) {
				return "", position, &tokenizeProblem{position, "a quoted escape"}
			}
			switch raw[position] {
			case '"', '\\':
				b = raw[position]
			case 'r':
				b = '\r'
			case 'n':
				b = '\n'
			case 't':
				b = '\t'
			default:
				return "", position, &tokenizeProblem{position, "a Squid quoted escape"}
			}
		}
		value.WriteByte(b)
	}
	return "", len(raw), &tokenizeProblem{len(raw), "a closing quote"}
}

// scan は時刻の欄を読み、値の終わりを返す。
//
// ts は符号と数字、tl と tg は strftime の書式が定める文字列を読む。`.%tu` をまとめた欄は、
// 続けて `.` と、Squid が幅の指定で前に足した空白と、秒未満の数字を読む。
func (s *timeSpec) scan(raw string, start int) (int, *tokenizeProblem) {
	end := start
	if s.code == "ts" {
		if end < len(raw) && raw[end] == '-' {
			end++
		}
		if digits := digitsEnd(raw, end, len(raw)); digits > end {
			end = digits
		} else {
			return start, &tokenizeProblem{start, "the seconds since the epoch"}
		}
	} else {
		var problem *tokenizeProblem
		if end, _, problem = s.format.scan(raw, start); problem != nil {
			return end, problem
		}
	}
	if s.subsecondDigits == 0 {
		return end, nil
	}
	if end >= len(raw) || raw[end] != '.' {
		return end, &tokenizeProblem{end, "the literal \".\" before the subsecond time"}
	}
	digitsStart := end + 1
	for digitsStart < len(raw) && raw[digitsStart] == ' ' {
		digitsStart++
	}
	if next := digitsEnd(raw, digitsStart, len(raw)); next > digitsStart {
		return next, nil
	}
	return digitsStart, &tokenizeProblem{digitsStart, "the digits of the subsecond time"}
}
