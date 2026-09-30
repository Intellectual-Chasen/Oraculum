package winevent

import (
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	systemTimeLayout      = "2006-01-02T15:04:05"
	offsetLayout          = "Z07:00"
	millisecondDigits     = 3
	microsecondDigits     = 6
	dateLength            = len("2006-01-02")
	numericOffsetLength   = len("+09:00")
	numericOffsetColonsAt = len("+09")
)

// eventTimeOf は TimeCreated@SystemTime の文字列を時刻へ直す。ok が偽になるのは文字列を
// 日時として読めないときである。
//
// 日付と時刻の区切りは `T` と空白を受け付ける。UTC からのずれ (`Z` または `+09:00` の形) を
// 持つ文字列は in_value、持たない文字列は undetermined であり、**ずれを補わない。** ずれを
// 持たない時刻は関連付けに使う時点を持たない。
//
// 精度を秒未満の桁数より細かく名乗らない。7 桁の文字列はマイクロ秒へ切り捨てる。原資料の文字列は
// rawText が保つ。
//
// 日付を `/` で区切る文字列は、イベントビューアーが表示の形で書いた地方時である (viewerTimeOf)。
func eventTimeOf(raw string) (core.Timestamp, bool) {
	if strings.Contains(raw, "/") {
		return viewerTimeOf(raw)
	}
	dateTime, offset := splitOffset(raw)
	if len(dateTime) <= dateLength || (dateTime[dateLength] != 'T' && dateTime[dateLength] != ' ') {
		return core.Timestamp{}, false
	}
	input := dateTime[:dateLength] + "T" + dateTime[dateLength+1:]
	precision, layout := precisionOf(dateTime)
	timestamp := core.Timestamp{
		RawText: &raw, Precision: precision,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	}
	var instant time.Time
	var err error
	if offset == "" {
		instant, err = time.Parse(systemTimeLayout, input)
		timestamp.NormalizedForm, timestamp.OffsetState = core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined
	} else {
		instant, err = time.Parse(systemTimeLayout+offsetLayout, input+offset)
		layout += offsetLayout
		timestamp.NormalizedForm, timestamp.OffsetState = core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue
		timestamp.OffsetText = &offset
	}
	if err != nil {
		return core.Timestamp{}, false
	}
	normalized := instant.Format(layout)
	timestamp.Normalized = &normalized
	built, err := core.NewTimestamp(timestamp)
	return built, err == nil
}

// viewerTimeLayout はイベントビューアーが「日付と時刻」の欄に書く文字列の形である。月、日、時は
// 1 桁の文字列も受け付ける。
const viewerTimeLayout = "2006/1/2 15:04:05"

// viewerTimeOf はイベントビューアーが書いた地方時の文字列を時刻へ直す。文字列は UTC からの
// ずれを持たず、書き出した端末の表示の地方時である。**ずれを補わない。**
func viewerTimeOf(raw string) (core.Timestamp, bool) {
	instant, err := time.Parse(viewerTimeLayout, raw)
	if err != nil {
		return core.Timestamp{}, false
	}
	normalized := instant.Format(systemTimeLayout)
	built, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized, Precision: core.PrecisionSecond,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
		NormalizedForm: core.NormalizedFormLocalWithoutOffset, OffsetState: core.OffsetStateUndetermined,
	})
	return built, err == nil
}

// splitOffset は文字列を日時と UTC からのずれに分ける。ずれを持たない文字列は offset が空になる。
func splitOffset(raw string) (dateTime, offset string) {
	if strings.HasSuffix(raw, "Z") {
		return raw[:len(raw)-1], "Z"
	}
	at := len(raw) - numericOffsetLength
	if at > dateLength && (raw[at] == '+' || raw[at] == '-') && raw[at+numericOffsetColonsAt] == ':' {
		return raw[:at], raw[at:]
	}
	return raw, ""
}

// precisionOf は秒の小数部の桁数から精度と正規化値の layout を返す。
func precisionOf(dateTime string) (core.Precision, string) {
	digits := 0
	if dot := strings.LastIndexByte(dateTime, '.'); dot >= 0 {
		digits = len(dateTime) - dot - 1
	}
	switch {
	case digits >= microsecondDigits:
		return core.PrecisionMicrosecond, systemTimeLayout + ".000000"
	case digits >= millisecondDigits:
		return core.PrecisionMillisecond, systemTimeLayout + ".000"
	default:
		return core.PrecisionSecond, systemTimeLayout
	}
}
