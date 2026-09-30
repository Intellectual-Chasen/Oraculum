package apache

import (
	"errors"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// errorTimeLayout はエラーログの時刻欄の layout である。
// 例: Tue Feb 01 11:22:33.123456 2000。
const errorTimeLayout = "Mon Jan 02 15:04:05.000000 2006"

// errorNormalizedLayout は正規化値の書式である。error.log は UTC からのずれを項目として
// 持たないため、UTC からのずれを含まない。
const errorNormalizedLayout = "2006-01-02T15:04:05.000000"

// ParseErrorTime はエラーログの時刻欄をマイクロ秒精度の Timestamp へ直す。
//
// 何の時刻かはログを出力した時刻であり、時計は Apache が動く端末自身のものである。
// UTC からのずれを項目として持たないため offsetState は item_absent になる
// (core.OffsetStateItemAbsent)。失敗の原因は未判定で、failure の stage は normalize である。
func ParseErrorTime(record ErrorRecord) (core.Timestamp, *core.ImportFailure) {
	item, found := record.Item(ErrorItemTime)
	raw := item.RawValue()
	problem := func(cause error) (core.Timestamp, *core.ImportFailure) {
		return core.Timestamp{}, errorSemanticFailure(record, ErrorItemTime,
			"a timestamp with microsecond precision and no UTC offset", cause)
	}
	if !found {
		return problem(nil)
	}
	instant, err := time.Parse(errorTimeLayout, raw)
	if err != nil {
		return problem(err)
	}
	normalized := instant.Format(errorNormalizedLayout)
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionMicrosecond, OffsetState: core.OffsetStateItemAbsent,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningRecordOutput,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		return problem(errors.Join(errors.New("building the error time"), err))
	}
	return timestamp, nil
}
