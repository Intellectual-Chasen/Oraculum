package squid

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const requestTimeLayout = "02/Jan/2006:15:04:05 -0700"

// ParseRequestTime は requestTime の欄を Timestamp へ直す。
//
// 角括弧で囲んだ既定の %tl は、秒の精度と数値の UTC からのずれを持つ時刻として読む。
// ほかの時刻の code は logformat が書いた形で読む (parseSpecifiedTime)。
// 失敗の原因は未判定で、failure の stage は normalize である。
func ParseRequestTime(record Record) (core.Timestamp, *core.ImportFailure) {
	item, found := record.Item(ItemRequestTime)
	if found && item.time != nil {
		return parseSpecifiedTime(record, item)
	}
	raw := item.RawValue()
	problem := func(cause error) (core.Timestamp, *core.ImportFailure) {
		return core.Timestamp{}, semanticFailure(record, ItemRequestTime, "a bracketed local timestamp with second precision and a numeric offset", cause)
	}
	if !found {
		return problem(nil)
	}
	if len(raw) != len(requestTimeLayout)+len("[]") || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return problem(errors.New("time item does not match the bracketed second-precision layout"))
	}
	text := raw[1 : len(raw)-1]
	// time.Parse は layout が秒精度でも小数秒を受け付けるため、文字列の長さも検査する。
	instant, err := time.Parse(requestTimeLayout, text)
	if err != nil {
		return problem(err)
	}
	_, zone, found := strings.Cut(text, " ")
	if !found || len(zone) != len("+0000") || zone[1:3] >= "24" || zone[3:] >= "60" {
		return problem(errors.New("numeric time offset is outside the hour or minute range"))
	}
	normalized := instant.Format(time.RFC3339)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond, OffsetState: core.OffsetStateInValue,
		OffsetText: &zone, Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return problem(err)
	}
	return timestamp, nil
}

// TimePrecisionOf は並びの requestTime の欄が書く時刻の精度を返す。requestTime の欄を
// 持たない並びには秒を返す。
func TimePrecisionOf(layout Layout) core.Precision {
	for _, item := range layout.items {
		if item.name == ItemRequestTime && item.time != nil {
			precision, _ := subsecondPrecision(item.time.subsecondDigits)
			return precision
		}
	}
	return core.PrecisionSecond
}

// subsecondPrecision は %tu の桁数から、Timestamp の精度と正規化値の秒未満の桁数を返す。
//
// 精度を桁数より細かく名乗らない。4 桁と 5 桁はミリ秒へ、1 桁と 2 桁は秒へ切り捨てる。
// 原資料の文字列は欄の rawText が保つ。
func subsecondPrecision(digits int) (core.Precision, int) {
	const millisecondDigits, microsecondDigits = 3, 6
	switch {
	case digits >= microsecondDigits:
		return core.PrecisionMicrosecond, microsecondDigits
	case digits >= millisecondDigits:
		return core.PrecisionMillisecond, millisecondDigits
	default:
		return core.PrecisionSecond, 0
	}
}

// parseSpecifiedTime は %ts、%tl、%tg と、まとめた `.%tu` の時刻を Timestamp へ直す。
//
// %tu の文字列は単位の数であり、小数として読まない。`%ts.%tu` の `1.5` は 1 秒と 5 ミリ秒である。
func parseSpecifiedTime(record Record, item Item) (core.Timestamp, *core.ImportFailure) {
	spec, raw := item.time, item.RawValue()
	problem := func(cause error) (core.Timestamp, *core.ImportFailure) {
		return core.Timestamp{}, semanticFailure(record, ItemRequestTime,
			"a time item written by the logformat time specifier", cause)
	}
	secondsText, subsecondText := raw, ""
	if spec.subsecondDigits > 0 {
		dot := strings.LastIndexByte(raw, '.')
		secondsText, subsecondText = raw[:dot], strings.TrimLeft(raw[dot+1:], " ")
	}
	instant, offsetState, err := spec.instantOf(secondsText)
	if err != nil {
		return problem(err)
	}
	precision, digits := subsecondPrecision(spec.subsecondDigits)
	if subsecondText != "" {
		units, err := strconv.Atoi(subsecondText)
		limit := int(math.Pow10(spec.subsecondDigits))
		if err != nil || units >= limit {
			return problem(fmt.Errorf("subsecond time %q exceeds %d digits", subsecondText, spec.subsecondDigits))
		}
		instant = instant.Add(time.Duration(units) * time.Second / time.Duration(limit))
	}
	layout := "2006-01-02T15:04:05"
	if digits > 0 {
		layout += "." + strings.Repeat("0", digits)
	}
	form := core.NormalizedFormLocalWithoutOffset
	var offsetText *string
	if offsetState != core.OffsetStateItemAbsent {
		form, layout = core.NormalizedFormRFC3339Absolute, layout+"Z07:00"
	}
	if offsetState == core.OffsetStateInValue {
		zone := instant.Format("-0700")
		offsetText = &zone
	}
	normalized := instant.Format(layout)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized, NormalizedForm: form,
		Precision: precision, OffsetState: offsetState, OffsetText: offsetText,
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return problem(errors.Join(errors.New("building the request time"), err))
	}
	return timestamp, nil
}

// instantOf は秒までの文字列を時刻へ直し、UTC からのずれの状態を返す。UTC からのずれを
// 持たない時刻は、壁時計の字面を UTC の location に置いて返す。
//
//   - %ts と strftime の %s は epoch である。
//   - strftime の %z は in_value である。
//   - %z を持たない %tg は、Squid が gmtime で書く定義に従い format_defined の UTC である。
//   - %z を持たない %tl は item_absent であり、地方時のずれを仮定しない。
func (s *timeSpec) instantOf(text string) (time.Time, core.OffsetState, error) {
	if s.code == "ts" {
		seconds, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("seconds since the epoch %q: %w", text, err)
		}
		return time.Unix(seconds, 0).UTC(), core.OffsetStateEpoch, nil
	}
	end, clock, scanProblem := s.format.scan(text, 0)
	if scanProblem != nil || end != len(text) {
		return time.Time{}, "", fmt.Errorf("time %q does not follow the strftime format", text)
	}
	instant, err := clock.civil()
	switch {
	case err != nil:
		return time.Time{}, "", err
	case s.format.carriesEpoch():
		return instant.UTC(), core.OffsetStateEpoch, nil
	case s.format.carriesOffset():
		return instant, core.OffsetStateInValue, nil
	case s.code == "tg":
		return instant, core.OffsetStateFormatDefined, nil
	default:
		return instant, core.OffsetStateItemAbsent, nil
	}
}
