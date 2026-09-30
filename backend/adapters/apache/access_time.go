package apache

import (
	"errors"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// accessTimeLayout は %t の角括弧の中の文字列の layout である。
// 例: 01/Feb/2000:11:20:10 +0900。
const accessTimeLayout = "02/Jan/2006:15:04:05 -0700"

// ParseAccessTime は角括弧付きの要求時刻の欄を秒精度の Timestamp へ直す。
//
// 何の時刻かは要求を受け取った時刻であり (Apache 2.4 のログ仕様の
// "The time that the request was received.")、時計は Apache が動く端末自身のものである。
// 失敗の原因は未判定で、failure の stage は normalize である。
func ParseAccessTime(record AccessRecord) (core.Timestamp, *core.ImportFailure) {
	item, found := record.Item(AccessItemRequestTime)
	raw := item.RawValue()
	problem := func(cause error) (core.Timestamp, *core.ImportFailure) {
		return core.Timestamp{}, accessSemanticFailure(record, AccessItemRequestTime,
			"a bracketed local timestamp with second precision and a numeric offset", cause)
	}
	if !found {
		return problem(nil)
	}
	if len(raw) != len(accessTimeLayout)+len("[]") || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return problem(errors.New("time item does not match the bracketed second-precision layout"))
	}
	text := raw[1 : len(raw)-1]
	// time.Parse は layout が秒精度でも小数秒を受け付けるため、文字列の長さも検査する。
	instant, err := time.Parse(accessTimeLayout, text)
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
		OffsetText: &zone, Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return problem(err)
	}
	return timestamp, nil
}
