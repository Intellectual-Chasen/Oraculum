package auditd

import (
	"fmt"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// millisecondDigits は事象を指す鍵が持つ秒未満の桁数である。
const millisecondDigits = 3

// EventTime は事象を指す鍵の秒とミリ秒から、レコードの時刻を組む。
//
// 原資料の文字列は UNIX epoch 秒であり、UTC からのずれの文字列を持たない。時点は 1 つに定まる
// ため OffsetState は epoch である。刻んだ時計は Linux の system clock であり、
// 端末そのものの時計に該当する。
//
// **正規化値を UTC で書く。** 原資料は時間帯を持たないため、収集元の所在地の時間帯で
// 書くと、原資料に無い値を正規化値へ入れることになる。
func EventTime(event EventKey) (core.Timestamp, error) {
	rawText := event.RawText
	if stamp, _, found := cutSerial(event.RawText); found {
		rawText = stamp
	}
	at := time.Unix(event.Seconds, event.Milliseconds*int64(time.Millisecond)).UTC()
	normalized := at.Format(rfc3339Milliseconds)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText:        &rawText,
		Normalized:     &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateEpoch,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	if err != nil {
		return core.Timestamp{}, fmt.Errorf("building the event time of %q: %w", rawText, err)
	}
	return timestamp, nil
}

// rfc3339Milliseconds はミリ秒 3 桁の RFC 3339 の layout である。
const rfc3339Milliseconds = "2006-01-02T15:04:05.000Z07:00"

// cutSerial は `<秒>.<ミリ秒>:<連番>` を時刻の文字列と連番の文字列へ分ける。
func cutSerial(rawText string) (stamp, serial string, found bool) {
	for index := len(rawText) - 1; index >= 0; index-- {
		if rawText[index] == ':' {
			return rawText[:index], rawText[index+1:], true
		}
	}
	return rawText, "", false
}
