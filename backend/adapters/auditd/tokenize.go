package auditd

import (
	"strconv"
	"strings"
)

// interpretedSeparator は、auditd が記録した生の値と、auditd が数値から求めた表示用の値を
// 分ける 1 byte である。
const interpretedSeparator = 0x1d

// keyType は行の種別を持つ key である。行の先頭に出る。
const keyType = "type"

// keyMessage は事象を指す鍵を持つ key である。`type=USER_*` では、もう 1 階層の
// key=value の並びを持つ key でもある。
const keyMessage = "msg"

// eventPrefix は事象を指す鍵の値が取る接頭辞である。
const eventPrefix = "audit("

// tokenizeLine は 1 行を key=value の並びへ分ける。
//
// **0x1D の後ろの項目を、前の項目と別の立場で返す。** 同じ意味の生の値と表示用の値が
// 1 行に並ぶため、2 つを 1 つの項目にまとめない。
func tokenizeLine(rawText string) ([]Item, *tokenizeProblem) {
	items := make([]Item, 0, 16)
	interpreted := false
	position := 0
	for position < len(rawText) {
		if rawText[position] == ' ' {
			position++
			continue
		}
		if rawText[position] == interpretedSeparator {
			interpreted = true
			position++
			continue
		}
		item, next, problem := readItem(rawText, position, interpreted)
		if problem != nil {
			return items, problem
		}
		items = append(items, item)
		position = next
	}
	if len(items) == 0 {
		return nil, &tokenizeProblem{
			byteOffset: 0,
			expected:   "an auditd line holding at least one key=value pair",
			observed:   "a line carrying no key=value pair",
		}
	}
	return items, nil
}

// readItem は position から 1 つの key=value を読み、次の読み出し位置を返す。
func readItem(rawText string, position int, interpreted bool) (Item, int, *tokenizeProblem) {
	keyStart := position
	for position < len(rawText) && rawText[position] != '=' &&
		rawText[position] != ' ' && rawText[position] != interpretedSeparator {
		position++
	}
	if position >= len(rawText) || rawText[position] != '=' {
		return Item{}, position, &tokenizeProblem{
			byteOffset: int64(keyStart),
			expected:   "a key followed by '='",
			observed:   observedAt(rawText, position),
		}
	}
	key := rawText[keyStart:position]
	if key == "" {
		return Item{}, position, &tokenizeProblem{
			byteOffset: int64(keyStart),
			expected:   "a key of one byte or more before '='",
			observed:   "an empty key at byte offset " + strconv.Itoa(keyStart),
		}
	}
	position++
	rawValue, value, quote, next := readValue(rawText, position)
	item := Item{
		key: key, rawValue: rawValue, value: value, quote: quote,
		interpreted: interpreted, byteOffset: int64(keyStart),
	}
	// 外側と同じ並びとして読まないため、内側は別の項目として持つ。
	if key == keyMessage && quote == '\'' {
		inner, problem := tokenizeLine(value)
		if problem == nil {
			item.inner = inner
		}
	}
	return item, next, nil
}

// readValue は position から value を読む。
//
// 引用符で囲まれた値は閉じる引用符までを値にする。囲まれていない値は、空白と 0x1D と
// 行末のいずれかまでを値にする。auditd は値の中の引用符を escape しないため、
// 閉じる引用符を探す以上の復号を行わない。
func readValue(rawText string, position int) (rawValue, value string, quote byte, next int) {
	if position < len(rawText) && (rawText[position] == '"' || rawText[position] == '\'') {
		quote = rawText[position]
		if end := strings.IndexByte(rawText[position+1:], quote); end >= 0 {
			closing := position + 1 + end
			return rawText[position : closing+1], rawText[position+1 : closing], quote, closing + 1
		}
		// 閉じる引用符が無い値は、行末までを値にする。開く引用符を原資料の文字列に残す。
		return rawText[position:], rawText[position+1:], quote, len(rawText)
	}
	start := position
	for position < len(rawText) && rawText[position] != ' ' &&
		rawText[position] != interpretedSeparator {
		position++
	}
	return rawText[start:position], rawText[start:position], 0, position
}

// lineTypeOf は行の種別を返す。ok が偽になるのは、`type=` を先頭に持たない行である。
func lineTypeOf(items []Item) (string, bool) {
	if len(items) == 0 || items[0].key != keyType {
		return "", false
	}
	return items[0].value, true
}

// eventKeyOf は `msg=audit(<秒>.<ミリ秒>:<連番>)` から事象を指す鍵を読む。
//
// 値は `audit(949395600.100:4242):` の形を取り、閉じ括弧の後ろに `:` が続く行がある。
// 閉じ括弧までを鍵の範囲とする。
func eventKeyOf(items []Item) (EventKey, *tokenizeProblem) {
	for _, item := range items {
		if item.key != keyMessage || item.interpreted {
			continue
		}
		return parseEventKey(item.value, item.byteOffset)
	}
	return EventKey{}, &tokenizeProblem{
		expected: "an auditd line holding msg=audit(<seconds>.<milliseconds>:<serial>)",
		observed: "a line carrying no msg key outside the interpreted part",
	}
}

func parseEventKey(value string, byteOffset int64) (EventKey, *tokenizeProblem) {
	problem := &tokenizeProblem{
		byteOffset: byteOffset,
		expected:   "msg=audit(<seconds>.<milliseconds>:<serial>)",
		observed:   "a msg value of " + strconv.Quote(value) + " that the event key cannot be read from",
	}
	if !strings.HasPrefix(value, eventPrefix) {
		return EventKey{}, problem
	}
	closing := strings.IndexByte(value, ')')
	if closing < 0 {
		return EventKey{}, problem
	}
	inside := value[len(eventPrefix):closing]
	stamp, serialText, found := strings.Cut(inside, ":")
	if !found {
		return EventKey{}, problem
	}
	secondsText, millisecondsText, found := strings.Cut(stamp, ".")
	if !found {
		return EventKey{}, problem
	}
	seconds, err := strconv.ParseInt(secondsText, 10, 64)
	if err != nil {
		return EventKey{}, problem
	}
	// ミリ秒は 3 桁で書かれる。桁数が違う値を 0 埋めで受け取らない。
	if len(millisecondsText) != millisecondDigits {
		return EventKey{}, problem
	}
	milliseconds, err := strconv.ParseInt(millisecondsText, 10, 64)
	if err != nil {
		return EventKey{}, problem
	}
	serial, err := strconv.ParseInt(serialText, 10, 64)
	if err != nil {
		return EventKey{}, problem
	}
	return EventKey{
		RawText: inside, Seconds: seconds, Milliseconds: milliseconds, Serial: serial,
	}, nil
}
