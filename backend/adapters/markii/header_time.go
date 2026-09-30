package markii

import (
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// headerTimeLayout はヘッダーの日時と UTC からのずれを合わせた文字列の書式である。
//
// 月 2 桁 / 日 2 桁 / 年 4 桁 / 時分秒 / ミリ秒 3 桁 / 半角空白 / ずれ 4 桁である。
const headerTimeLayout = "01/02/2006 15:04:05.000 -0700"

// HeaderTimestamp は evt の種別に依らずヘッダーの時刻を返す。
// ok が偽になるのはヘッダーの時刻を読み取れないときである。
//
// 精度はミリ秒、UTC からのずれは値の中にあり、刻んだのは端末の時計、意味は事象の時刻で
// ある。日時として解釈できない文字列では組まず、呼ぶ側が normalize の失敗を返す。
func HeaderTimestamp(record Record) (core.Timestamp, bool) {
	dateTime, hasDateTime := record.DateTimeText()
	zone, hasZone := record.ZoneText()
	if !hasDateTime || !hasZone {
		return core.Timestamp{}, false
	}
	rawText := dateTime + " " + zone

	instant, err := time.Parse(headerTimeLayout, rawText)
	if err != nil {
		return core.Timestamp{}, false
	}
	normalized := instant.Format(normalizedTimeLayout)

	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText:        &rawText,
		Normalized:     &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     &zone,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	if err != nil {
		return core.Timestamp{}, false
	}
	return timestamp, true
}

// normalizedTimeLayout は比較に用いる値の書式である。原資料の精度をそのまま保つため
// ミリ秒 3 桁を除かない。
const normalizedTimeLayout = "2006-01-02T15:04:05.000Z07:00"
