package squid

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// clockField は時刻の文字列から読む成分である。
type clockField uint8

const (
	fieldNone clockField = iota
	fieldYear
	fieldCentury
	fieldYearInCentury
	fieldMonth
	fieldDay
	fieldYearDay
	fieldHour
	fieldHour12
	fieldPM
	fieldMinute
	fieldSecond
	fieldWeekday    // 0 が日曜 (%a %A %w)
	fieldISOWeekday // 7 が日曜 (%u)
	fieldEpoch
	fieldOffset // 符号付きの hhmm。-0930 は -930
	clockFieldCount
)

// clockFields は成分の集合である。
type clockFields uint32

func (s clockFields) has(f clockField) bool { return s&(1<<f) != 0 }

// fullDateTime は、集合が秒までの時点を 1 つに定めるかを返す。
func (s clockFields) fullDateTime() bool {
	if s.has(fieldEpoch) {
		return true
	}
	year := s.has(fieldYear) || s.has(fieldYearInCentury)
	date := (s.has(fieldMonth) && s.has(fieldDay)) || s.has(fieldYearDay)
	hour := s.has(fieldHour) || (s.has(fieldHour12) && s.has(fieldPM))
	return year && date && hour && s.has(fieldMinute) && s.has(fieldSecond)
}

// numericConversion は数字の変換の、glibc の既定の幅と埋めの文字である。
type numericConversion struct {
	width int
	pad   byte
	field clockField
}

var numericConversions = map[byte]numericConversion{
	'd': {2, '0', fieldDay}, 'e': {2, ' ', fieldDay}, 'm': {2, '0', fieldMonth},
	'H': {2, '0', fieldHour}, 'k': {2, ' ', fieldHour},
	'I': {2, '0', fieldHour12}, 'l': {2, ' ', fieldHour12},
	'M': {2, '0', fieldMinute}, 'S': {2, '0', fieldSecond},
	'y': {2, '0', fieldYearInCentury}, 'C': {2, '0', fieldCentury}, 'Y': {4, '0', fieldYear},
	'j': {3, '0', fieldYearDay}, 'u': {1, '0', fieldISOWeekday}, 'w': {1, '0', fieldWeekday},
	'g': {2, '0', fieldNone}, 'G': {4, '0', fieldNone},
	'U': {2, '0', fieldNone}, 'V': {2, '0', fieldNone}, 'W': {2, '0', fieldNone},
}

// compositeConversions は C locale で複合の変換が出す書式である。
var compositeConversions = map[byte]string{
	'c': "%a %b %e %H:%M:%S %Y", 'D': "%m/%d/%y", 'x': "%m/%d/%y", 'F': "%Y-%m-%d",
	'r': "%I:%M:%S %p", 'R': "%H:%M", 'T': "%H:%M:%S", 'X': "%H:%M:%S",
}

// 既知の制限: E と O は glibc が受け付ける組のうち、C locale で意味の変わらない代表の組だけを読む,
// 測れない理由: 対応する構文の範囲の選択である, 他の組を使う logformat が見つかったら足す。
const (
	eModifiable = "cCxXyY"
	oModifiable = "bBdeHIklmMSuUVwWyCgj"
)

const (
	daysPerWeek   = 7
	monthsPerYear = 12
	offsetDigits  = len("hhmm")
	// posixPivotYear は、%y だけの年を 20xx と読む上限 (排他) である。69-99 は 19xx と読む。
	posixPivotYear = 69
)

var (
	weekdayNames = namesOf(daysPerWeek, func(i int) string { return time.Weekday(i).String() })
	monthNames   = namesOf(monthsPerYear, func(i int) string { return time.Month(i + 1).String() })
	meridiemName = []string{"AM", "PM"}
)

func namesOf(n int, name func(int) string) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = name(i)
	}
	return names
}

// strftimeStep は書式の literal 1 つ、または変換 1 つを読む手順である。
type strftimeStep struct {
	literal   string
	conv      byte   // 0 は literal
	directive string // 書式の中の変換の文字列
	width     int    // 数字の文字列の幅。pad が 0 なら最大の桁数
	pad       byte   // '0'、' '、または 0 (埋め無し)
	field     clockField
	stop      byte // %Z の直後の literal の先頭 byte。無ければ 0
}

// strftimeFormat は strftime の書式を、文字列を読む手順へ直したものである。
type strftimeFormat struct {
	steps  []strftimeStep
	fields clockFields
}

// compileStrftime は書式を読み取りの手順へ直す。error は *LogFormatError で、Offset は
// 書式の先頭からの byte offset である。
func compileStrftime(format string) (strftimeFormat, error) {
	var f strftimeFormat
	for i := 0; i < len(format); {
		if c := format[i]; c != '%' {
			if c == '\n' || c == '\r' || c == '\t' {
				return strftimeFormat{}, &LogFormatError{Offset: i, Token: format[i : i+1],
					Reason: "the time format contains a line break or tab"}
			}
			f.appendLiteral(format[i : i+1])
			i++
			continue
		}
		end, err := f.appendDirective(format, i)
		if err != nil {
			return strftimeFormat{}, err
		}
		i = end
	}
	for i := range f.steps {
		if f.steps[i].conv == 'Z' && i+1 < len(f.steps) && f.steps[i+1].conv == 0 {
			f.steps[i].stop = f.steps[i+1].literal[0]
		}
	}
	return f, nil
}

func (f *strftimeFormat) appendLiteral(text string) {
	if last := len(f.steps) - 1; last >= 0 && f.steps[last].conv == 0 {
		f.steps[last].literal += text
		return
	}
	f.steps = append(f.steps, strftimeStep{literal: text})
}

func (f *strftimeFormat) appendStep(step strftimeStep) {
	f.steps = append(f.steps, step)
	if step.field != fieldNone {
		f.fields |= 1 << step.field
	}
}

// appendDirective は start の '%' から変換 1 つを読み、その次の位置を返す。
// 文字列の並びは glibc と同じく flag、幅、E / O、変換の文字の順である。
func (f *strftimeFormat) appendDirective(format string, start int) (int, error) {
	i := start + 1
	var pad byte
	for ; i < len(format) && strings.IndexByte("_-0^#", format[i]) >= 0; i++ {
		// ^ と # は大文字小文字だけを変え、名前は大文字小文字を区別せずに一致を調べるため読み捨てる。
		if format[i] != '^' && format[i] != '#' {
			pad = format[i]
		}
	}
	widthStart := i
	for i < len(format) && '0' <= format[i] && format[i] <= '9' {
		i++
	}
	widthText := format[widthStart:i]
	var modifier byte
	if i < len(format) && (format[i] == 'E' || format[i] == 'O') {
		modifier = format[i]
		i++
	}
	if i >= len(format) {
		return 0, &LogFormatError{Offset: start, Token: format[start:],
			Reason: "the time format ends inside a conversion"}
	}
	conv := format[i]
	i++
	directive := format[start:i]
	fail := func(reason string) (int, error) {
		return 0, &LogFormatError{Offset: start, Token: directive, Reason: reason}
	}
	width := 0
	if widthText != "" {
		w, err := strconv.Atoi(widthText)
		if err != nil {
			return fail("the time conversion width exceeds the integer range")
		}
		width = w
	}
	if (modifier == 'E' && strings.IndexByte(eModifiable, conv) < 0) ||
		(modifier == 'O' && strings.IndexByte(oModifiable, conv) < 0) {
		return fail("the time conversion does not accept the E or O modifier")
	}
	if conv == 'n' || conv == 't' {
		return fail("the time conversion emits a line break or tab")
	}
	if n, ok := numericConversions[conv]; ok {
		step := strftimeStep{conv: conv, directive: directive, width: max(width, n.width), pad: n.pad, field: n.field}
		switch pad {
		case '0':
			step.pad = '0'
		case '_':
			step.pad = ' '
		case '-':
			step.pad = 0
			if width > n.width {
				step.pad = ' '
			}
		}
		f.appendStep(step)
		return i, nil
	}
	if width > 0 {
		return fail("the time conversion accepts a width only for numbers")
	}
	step := strftimeStep{conv: conv, directive: directive}
	switch conv {
	case 'a', 'A':
		step.field = fieldWeekday
	case 'b', 'B', 'h':
		step.field = fieldMonth
	case 'p', 'P':
		step.field = fieldPM
	case 's':
		step.field = fieldEpoch
	case 'z':
		// glibc は %z の埋めの flag で符号と数字の間に埋めを入れ、+hhmm の形を崩す。
		if pad != 0 {
			return fail("the time conversion %z accepts no padding flag")
		}
		step.field = fieldOffset
	case 'Z':
	case '%':
		f.appendLiteral("%")
		return i, nil
	default:
		sub, ok := compositeConversions[conv]
		if !ok {
			return fail("an unknown time conversion")
		}
		inner, err := compileStrftime(sub)
		if err != nil {
			return 0, err
		}
		for _, s := range inner.steps {
			if s.conv == 0 {
				f.appendLiteral(s.literal)
			} else {
				f.appendStep(s)
			}
		}
		return i, nil
	}
	f.appendStep(step)
	return i, nil
}

// fullDateTime は、書式が秒までの時点を 1 つに定める成分を持つかを返す。
func (f strftimeFormat) fullDateTime() bool { return f.fields.fullDateTime() }

// carriesOffset は書式が %z を持つかを返す。
func (f strftimeFormat) carriesOffset() bool { return f.fields.has(fieldOffset) }

// carriesEpoch は書式が %s を持つかを返す。
func (f strftimeFormat) carriesEpoch() bool { return f.fields.has(fieldEpoch) }

// scan は raw の start から書式どおりに読み、値の終わり (排他) と読んだ成分を返す。
func (f strftimeFormat) scan(raw string, start int) (int, wallClock, *tokenizeProblem) {
	var clock wallClock
	position := start
	for _, step := range f.steps {
		end, problem := step.scan(raw, position, &clock)
		if problem != nil {
			return problem.offset, wallClock{}, problem
		}
		position = end
	}
	return position, clock, nil
}

func (s strftimeStep) scan(raw string, start int, clock *wallClock) (int, *tokenizeProblem) {
	problem := func(offset int) (int, *tokenizeProblem) {
		if s.conv == 0 {
			return 0, &tokenizeProblem{offset, fmt.Sprintf("the literal text %q of the time format", s.literal)}
		}
		return 0, &tokenizeProblem{offset, "a value of the time conversion " + s.directive}
	}
	switch s.conv {
	case 0:
		for i := 0; i < len(s.literal); i++ {
			if start+i >= len(raw) || raw[start+i] != s.literal[i] {
				return problem(start + i)
			}
		}
		return start + len(s.literal), nil
	case 'a', 'A', 'b', 'B', 'h', 'p', 'P':
		names, value := weekdayNames, 0
		switch s.conv {
		case 'b', 'B', 'h':
			names, value = monthNames, 1
		case 'p', 'P':
			names = meridiemName
		}
		abbreviated := s.conv == 'a' || s.conv == 'b' || s.conv == 'h'
		for index, name := range names {
			if abbreviated {
				name = name[:3]
			}
			if end := start + len(name); end <= len(raw) && strings.EqualFold(raw[start:end], name) {
				clock.set(s.field, index+value)
				return end, nil
			}
		}
		return problem(start)
	case 'z':
		if start >= len(raw) || (raw[start] != '+' && raw[start] != '-') {
			return problem(start)
		}
		end := digitsEnd(raw, start+1, offsetDigits)
		if end != start+1+offsetDigits {
			return problem(end)
		}
		clock.setDigits(s.field, raw[start:end])
		return end, nil
	case 'Z':
		end := start
		for end < len(raw) && raw[end] != s.stop && isZoneNameByte(raw[end]) {
			end++
		}
		if end == start {
			return problem(start)
		}
		return end, nil
	case 's':
		digits := start
		if digits < len(raw) && raw[digits] == '-' {
			digits++
		}
		end := digitsEnd(raw, digits, len(raw))
		if end == digits {
			return problem(digits)
		}
		clock.setDigits(s.field, raw[start:end])
		return end, nil
	}
	digits, limit := start, start+s.width
	if s.pad == ' ' {
		for digits < limit-1 && digits < len(raw) && raw[digits] == ' ' {
			digits++
		}
	}
	end := digitsEnd(raw, digits, limit-digits)
	if (s.pad == 0 && end == digits) || (s.pad != 0 && end != limit) {
		return problem(end)
	}
	clock.setDigits(s.field, raw[digits:end])
	return end, nil
}

// digitsEnd は start から最大 limit 個続く 10 進の数字の終わりを返す。
func digitsEnd(raw string, start, limit int) int {
	end := start
	for end < len(raw) && end-start < limit && '0' <= raw[end] && raw[end] <= '9' {
		end++
	}
	return end
}

func isZoneNameByte(c byte) bool {
	return ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') || c == '+' || c == '-'
}

// wallClock は読んだ成分である。
type wallClock struct {
	values [clockFieldCount]int
	seen   clockFields
	err    error
}

func (w *wallClock) setDigits(f clockField, digits string) {
	if f == fieldNone {
		return
	}
	value, err := strconv.Atoi(digits)
	if err != nil {
		w.fail(fmt.Errorf("time component %q exceeds the integer range", digits))
		return
	}
	w.set(f, value)
}

func (w *wallClock) set(f clockField, value int) {
	if f == fieldNone {
		return
	}
	if w.seen.has(f) && w.values[f] != value {
		w.fail(errors.New("time text repeats a component with different values"))
	}
	w.seen |= 1 << f
	w.values[f] = value
}

func (w *wallClock) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// civil は成分から時刻を組む。%s があれば time.Unix の値、%z があればその FixedZone、
// どちらも無ければ time.UTC を壁時計の字面の入れ物として使う。
func (w wallClock) civil() (time.Time, error) {
	if w.err != nil {
		return time.Time{}, w.err
	}
	has, value := w.seen.has, func(f clockField) int { return w.values[f] }
	if has(fieldHour12) && !has(fieldPM) {
		return time.Time{}, errors.New("12-hour clock time has no AM or PM")
	}
	if !w.seen.fullDateTime() {
		return time.Time{}, errors.New("time format does not determine a date and a time to the second")
	}
	location := time.UTC
	if has(fieldOffset) {
		offset := value(fieldOffset)
		magnitude := max(offset, -offset)
		if magnitude/100 > 23 || magnitude%100 > 59 {
			return time.Time{}, errors.New("numeric time offset is outside the hour or minute range")
		}
		seconds := (magnitude/100*60 + magnitude%100) * 60
		if offset < 0 {
			seconds = -seconds
		}
		location = time.FixedZone("", seconds)
	}
	if has(fieldEpoch) {
		// %s が時点を 1 つに定める。他の成分は時間帯の分からない壁時計でありうるため突き合わせない。
		return time.Unix(int64(value(fieldEpoch)), 0).In(location), nil
	}
	year, err := w.year()
	if err != nil {
		return time.Time{}, err
	}
	date, err := w.date(year)
	if err != nil {
		return time.Time{}, err
	}
	hour, err := w.hour()
	if err != nil {
		return time.Time{}, err
	}
	if value(fieldMinute) > 59 || value(fieldSecond) > 59 {
		return time.Time{}, errors.New("minute or second is outside 0-59")
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour,
		value(fieldMinute), value(fieldSecond), 0, location), nil
}

func (w wallClock) year() (int, error) {
	has := w.seen.has
	century, yy := w.values[fieldCentury], w.values[fieldYearInCentury]
	if has(fieldYearInCentury) && yy > 99 {
		return 0, errors.New("year within the century is outside 0-99")
	}
	if has(fieldYear) {
		year := w.values[fieldYear]
		if (has(fieldCentury) && century != year/100) || (has(fieldYearInCentury) && yy != year%100) {
			return 0, errors.New("century or year within the century disagrees with the year")
		}
		return year, nil
	}
	if !has(fieldCentury) {
		century = 19
		if yy < posixPivotYear {
			century = 20
		}
	}
	return century*100 + yy, nil
}

func (w wallClock) date(year int) (time.Time, error) {
	has, value := w.seen.has, func(f clockField) int { return w.values[f] }
	var date time.Time
	if has(fieldMonth) && has(fieldDay) {
		month, day := value(fieldMonth), value(fieldDay)
		if month < 1 || month > 12 {
			return time.Time{}, errors.New("month is outside 1-12")
		}
		if day < 1 || day > time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
			return time.Time{}, errors.New("day is outside the days of the month")
		}
		date = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	} else {
		yearDay := value(fieldYearDay)
		if yearDay < 1 || yearDay > time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).YearDay() {
			return time.Time{}, errors.New("day of the year is outside the days of the year")
		}
		date = time.Date(year, 1, yearDay, 0, 0, 0, 0, time.UTC)
	}
	weekday := int(date.Weekday())
	switch {
	case has(fieldMonth) && value(fieldMonth) != int(date.Month()),
		has(fieldDay) && value(fieldDay) != date.Day(),
		has(fieldYearDay) && value(fieldYearDay) != date.YearDay():
		return time.Time{}, errors.New("month, day, and day of the year disagree")
	case has(fieldWeekday) && value(fieldWeekday) != weekday,
		has(fieldISOWeekday) && (value(fieldISOWeekday) < 1 || value(fieldISOWeekday) > 7 ||
			value(fieldISOWeekday)%7 != weekday):
		return time.Time{}, errors.New("weekday disagrees with the date")
	}
	return date, nil
}

func (w wallClock) hour() (int, error) {
	has, value := w.seen.has, func(f clockField) int { return w.values[f] }
	hour := value(fieldHour)
	if has(fieldHour) && hour > 23 {
		return 0, errors.New("hour is outside 0-23")
	}
	if has(fieldHour12) {
		h12 := value(fieldHour12)
		if h12 < 1 || h12 > 12 {
			return 0, errors.New("12-hour clock hour is outside 1-12")
		}
		h24 := h12%12 + 12*value(fieldPM)
		if has(fieldHour) && hour != h24 {
			return 0, errors.New("24-hour and 12-hour clock hours disagree")
		}
		return h24, nil
	}
	if has(fieldHour) && has(fieldPM) && (hour >= 12) != (value(fieldPM) == 1) {
		return 0, errors.New("AM or PM disagrees with the hour")
	}
	return hour, nil
}
