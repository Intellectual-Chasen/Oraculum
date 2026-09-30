package core

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UtcOffset は UTC からのずれを `+09:00` の形で持つ。
//
// 分析者が「この収集元の時刻は UTC+hh:mm として読む」と記録した値である。
// 原資料の文字列ではない。
type UtcOffset string

// utcOffsetPattern は UtcOffset の文字列の形である。符号、時の 2 桁、分の 2 桁を持つ。
var utcOffsetPattern = regexp.MustCompile(`^[+-][0-9]{2}:[0-5][0-9]$`)

// maxUtcOffsetSeconds は受け入れるずれの絶対値の上限である。ずれの大きさを 18 時間までとする。
const maxUtcOffsetSeconds = 18 * 60 * 60

// Seconds は UTC からのずれを秒で返す。ok が偽になるのは文字列が形に合わないときである。
func (o UtcOffset) Seconds() (int, bool) {
	text := string(o)
	if !utcOffsetPattern.MatchString(text) {
		return 0, false
	}
	hours, _ := strconv.Atoi(text[1:3])
	minutes, _ := strconv.Atoi(text[4:6])
	seconds := hours*60*60 + minutes*60
	if seconds > maxUtcOffsetSeconds {
		return 0, false
	}
	if text[0] == '-' {
		// RFC 3339 の -00:00 は「ずれが分からない」を表すため、ずれ 0 の記録として受け取らない。
		if seconds == 0 {
			return 0, false
		}
		seconds = -seconds
	}
	return seconds, true
}

// Validate は文字列が `+hh:mm` の形で、ずれが 18 時間以内であることを確かめる。
func (o UtcOffset) Validate() error {
	if problem := requirePresent("UtcOffset", string(o)); problem != nil {
		return problem
	}
	if _, ok := o.Seconds(); !ok {
		return itemError("UtcOffset "+strconv.Quote(string(o)), ErrInvalid)
	}
	return nil
}

// TimestampInterpretation は、UTC からのずれを持たない時刻に分析者が与えたずれである。
//
// **原資料の文字列と正規化値を書き換えない。** Timestamp の RawText・Normalized・OffsetState は
// 原資料から読んだ値のまま保ち、本型が関連付けと並びに使う時点を別に与える。
type TimestampInterpretation struct {
	// Offset は時刻を読むときの UTC からのずれである。
	Offset UtcOffset `json:"offset"`
	// AssertionId はずれを記録した分析者の所見の識別子である。**利用者が取り込みの起動で
	// 収集元に指定したずれは所見を持たず、本項目を出さない。**
	AssertionId string `json:"assertionId,omitempty"`
}

// Validate はずれを確かめる。
func (i TimestampInterpretation) Validate() error {
	if err := i.Offset.Validate(); err != nil {
		return itemError("TimestampInterpretation.offset", err)
	}
	return nil
}

// AcceptsInterpretation は、この時刻に分析者のずれを与えられるかを返す。
//
// 与えられるのは、原資料と入力形式の定義から時点が定まらず、正規化値が秒以下の桁まで持つ
// 地方時の文字列である時刻だけである。
func (t Timestamp) AcceptsInterpretation() bool {
	return !t.OffsetState.carriesInstant() &&
		t.NormalizedForm == NormalizedFormLocalWithoutOffset &&
		t.ValueState == ValueStatePresent && t.Normalized != nil
}

// WithInterpretation は、分析者のずれを与えた時刻を返す。
// 原資料の文字列、正規化値、精度、UTC からのずれの状態はそのまま保つ。
func (t Timestamp) WithInterpretation(interpretation TimestampInterpretation) (Timestamp, error) {
	t.Interpretation = &interpretation
	return NewTimestamp(t)
}

// AbsoluteByInterpretation は、分析者のずれで読んだ時点を UTC で書いた時刻を返す。精度、
// 時計、何の時刻かは保ち、分析者のずれを持たない。ok が偽になるのは、分析者のずれを
// 持たない時刻である。
//
// **利用者が地方時の文字列で入力した期間を、グラフを組むときの解釈で読むための値である。**
// 利用者の割当は分析者のずれを持てない (TerminalAssignment.Validate) ため、読んだ時点を
// ずれを持たない時刻で持つ。原資料の文字列も UTC で書いた文字列にする。原資料の文字列はずれを持たない
// ため、原資料の文字列に in_value の状態を付けると、原資料がずれを書いていたと読める。
func (t Timestamp) AbsoluteByInterpretation() (Timestamp, bool) {
	instant, interpreted := interpretedInstant(t)
	digits, hasFraction := requiredFractionDigits(t.Precision)
	if !interpreted || !hasFraction {
		return Timestamp{}, false
	}
	layout := "2006-01-02T15:04:05"
	if digits > 0 {
		layout += "." + strings.Repeat("0", digits)
	}
	text := instant.UTC().Format(layout + "Z07:00")
	offset := "Z"
	absolute, err := NewTimestamp(Timestamp{
		RawText: &text, Normalized: &text, NormalizedForm: NormalizedFormRFC3339Absolute,
		Precision: t.Precision, OffsetState: OffsetStateInValue, OffsetText: &offset,
		Clock: t.Clock, Meaning: t.Meaning, ValueState: t.ValueState,
	})
	return absolute, err == nil
}

// validateInterpretation は、分析者のずれを与えてよい時刻にだけずれがあることを確かめる。
func (t Timestamp) validateInterpretation() error {
	if t.Interpretation == nil {
		return nil
	}
	if !t.AcceptsInterpretation() {
		return itemError("Timestamp.interpretation on a timestamp whose instant is not open to "+
			"an analyst offset", ErrUnexpectedItem)
	}
	if err := t.Interpretation.Validate(); err != nil {
		return itemError("Timestamp.interpretation", err)
	}
	return nil
}

// interpretedInstant は正規化値の地方時を、分析者のずれで読んだ時点を返す。
func interpretedInstant(t Timestamp) (time.Time, bool) {
	if t.Interpretation == nil || !t.AcceptsInterpretation() {
		return time.Time{}, false
	}
	seconds, ok := t.Interpretation.Offset.Seconds()
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation(localWithoutOffsetLayout, *t.Normalized,
		time.FixedZone(string(t.Interpretation.Offset), seconds))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
