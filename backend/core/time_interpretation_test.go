package core_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// UTC からのずれは `+hh:mm` の形で、18 時間以内の値だけを受け取る。
func TestUtcOffsetAcceptsTheSignedHourAndMinuteForm(t *testing.T) {
	accepted := map[core.UtcOffset]int{
		"+00:00": 0, "+09:00": 9 * 3600, "-05:30": -(5*3600 + 30*60), "+18:00": 18 * 3600,
	}
	for offset, want := range accepted {
		if err := offset.Validate(); err != nil {
			t.Errorf("%q: %v", offset, err)
		}
		if got, ok := offset.Seconds(); !ok || got != want {
			t.Errorf("%q: seconds=%d ok=%v, want %d", offset, got, ok, want)
		}
	}
	for _, offset := range []core.UtcOffset{"", "09:00", "+9:00", "+09:60", "+18:30", "-00:00", "Z", "+0900"} {
		if err := offset.Validate(); err == nil {
			t.Errorf("%q passed the validation", offset)
		}
	}
}

// localTimestamp は UTC からのずれを持たない地方時の時刻を返す。
func localTimestamp(t *testing.T, normalized string, precision core.Precision) core.Timestamp {
	t.Helper()
	raw := normalized
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized, NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision: precision, OffsetState: core.OffsetStateItemAbsent,
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return timestamp
}

// 分析者のずれを与えた地方時は、そのずれで読んだ時点を持ち、原資料の文字列と精度を保つ。
func TestTimestampWithInterpretationReadsTheLocalTimeWithTheOffset(t *testing.T) {
	local := localTimestamp(t, "2031-04-05T06:07:08.250", core.PrecisionMillisecond)
	if _, ok := local.Instant(); ok {
		t.Fatal("a local time without an offset carries an instant")
	}
	interpreted, err := local.WithInterpretation(core.TimestampInterpretation{
		Offset: "+09:00", AssertionId: "as:synthetic",
	})
	if err != nil {
		t.Fatal(err)
	}
	instant, ok := interpreted.Instant()
	want := time.Date(2031, time.April, 4, 21, 7, 8, 250_000_000, time.UTC)
	if !ok || !instant.Equal(want) {
		t.Fatalf("instant=%s ok=%v, want %s", instant, ok, want)
	}
	if *interpreted.RawText != *local.RawText || *interpreted.Normalized != *local.Normalized ||
		interpreted.Precision != local.Precision || interpreted.OffsetState != local.OffsetState ||
		interpreted.NormalizedForm != local.NormalizedForm {
		t.Fatalf("the interpretation rewrote the source items: %+v", interpreted)
	}
	if interpreted.Equal(local) {
		t.Fatal("the interpreted time equals the time without the interpretation")
	}
	encoded, err := json.Marshal(interpreted)
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.Timestamp
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(interpreted) {
		t.Fatalf("the JSON round trip lost the interpretation: %s", encoded)
	}
}

// 秒精度の時刻は、ずれを与えた後も秒精度のままである。同じ秒のミリ秒精度の時刻と同じ時点に
// 決めつけない。
func TestTimestampWithInterpretationKeepsTheSecondPrecision(t *testing.T) {
	interpretation := core.TimestampInterpretation{Offset: "+00:00", AssertionId: "as:synthetic"}
	second, err := localTimestamp(t, "2031-04-05T06:07:08", core.PrecisionSecond).WithInterpretation(interpretation)
	if err != nil {
		t.Fatal(err)
	}
	millisecond, err := localTimestamp(t, "2031-04-05T06:07:08.750", core.PrecisionMillisecond).
		WithInterpretation(interpretation)
	if err != nil {
		t.Fatal(err)
	}
	if second.Precision != core.PrecisionSecond || millisecond.Precision != core.PrecisionMillisecond {
		t.Fatalf("precisions %q and %q, want second and millisecond", second.Precision, millisecond.Precision)
	}
	secondAt, _ := second.Instant()
	millisecondAt, _ := millisecond.Instant()
	if secondAt.Equal(millisecondAt) {
		t.Fatal("the second-precision time was placed at the instant of the millisecond-precision time")
	}
	if !secondAt.Equal(millisecondAt.Truncate(time.Second)) {
		t.Fatalf("the two times %s and %s are not in the same second", secondAt, millisecondAt)
	}
}

// 時点が原資料から定まる時刻には、分析者のずれを与えない。
func TestTimestampRejectsAnInterpretationOnAnAbsoluteTime(t *testing.T) {
	raw, normalized, offset := "2031-04-05T06:07:08Z", "2031-04-05T06:07:08Z", "Z"
	absolute, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized, NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision: core.PrecisionSecond, OffsetState: core.OffsetStateInValue, OffsetText: &offset,
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if absolute.AcceptsInterpretation() {
		t.Fatal("an absolute time accepts an analyst offset")
	}
	_, err = absolute.WithInterpretation(core.TimestampInterpretation{Offset: "+09:00", AssertionId: "as:synthetic"})
	if !errors.Is(err, core.ErrUnexpectedItem) {
		t.Fatalf("err=%v, want ErrUnexpectedItem", err)
	}
}

// 時刻の比較は、分析者のずれを与えた地方時を受け入れ、ずれの無い地方時を退ける。
func TestTimeComparisonAcceptsAnInterpretedLocalTimeAlone(t *testing.T) {
	local := localTimestamp(t, "2031-04-05T06:07:08", core.PrecisionSecond)
	interpreted, err := local.WithInterpretation(core.TimestampInterpretation{Offset: "+00:00", AssertionId: "as:1"})
	if err != nil {
		t.Fatal(err)
	}
	comparison := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond, LeftTime: &interpreted, RightTime: &interpreted,
		Assumptions: []core.MatchAssumption{},
	}
	if err := comparison.Validate(); err != nil {
		t.Errorf("an interpreted local time: %v", err)
	}
	comparison.LeftTime = &local
	if err := comparison.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("a local time without an interpretation: err=%v, want ErrInconsistentValue", err)
	}
}

func sourceAssertion() core.Assertion {
	assertion := validAssertion()
	offset := core.UtcOffset("+09:00")
	assertion.Target = core.AssertionTarget{
		Kind: core.AssertionTargetKindSource, SourceContentSha256: assertionContentSha256,
	}
	assertion.TimeOffset = &offset
	assertion.Basis.RecordRefs = []core.AssertionRecordRef{assertionLineRef(3), assertionLineRef(7)}
	return assertion
}

// 収集元を指す所見はずれを持ち、他の対象の所見はずれを持たない。改訂の履歴も同じである。
func TestAssertionOnASourceCarriesTheTimeOffsetInEveryRevision(t *testing.T) {
	if err := sourceAssertion().Validate(); err != nil {
		t.Fatalf("a source assertion with its offset: %v", err)
	}
	withoutOffset := sourceAssertion()
	withoutOffset.TimeOffset = nil
	if err := withoutOffset.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("a source assertion without an offset: err=%v, want ErrMissingRequiredItem", err)
	}
	onNode := validAssertion()
	offset := core.UtcOffset("+09:00")
	onNode.TimeOffset = &offset
	if err := onNode.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("a node assertion with an offset: err=%v, want ErrUnexpectedItem", err)
	}
	revised := sourceAssertion()
	revised.State = core.AssertionStateWithdrawn
	revised.RevisionNumber = core.FirstAssertionRevisionNumber + 1
	revised.History = []core.AssertionRevision{{
		RevisionNumber: core.FirstAssertionRevisionNumber, State: core.AssertionStateActive,
		Author: "analyst-a", RecordedAt: assertionRecordedAt, Basis: revised.Basis,
	}}
	if err := revised.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("a superseded revision without an offset: err=%v, want ErrMissingRequiredItem", err)
	}
	revised.History[0].TimeOffset = &offset
	if err := revised.Validate(); err != nil {
		t.Errorf("a withdrawn source assertion with its history: %v", err)
	}
}

// 分析者のずれで読んだ時刻は、同じ時点を UTC で書いた時刻になる。精度と時計は保ち、
// 分析者のずれを持たない。ずれを持たない時刻は変換しない。
func TestAbsoluteByInterpretationWritesTheInterpretedInstantInUtc(t *testing.T) {
	local := localTimestamp(t, "2031-04-05T06:07:08.250", core.PrecisionMillisecond)
	if _, converted := local.AbsoluteByInterpretation(); converted {
		t.Fatal("a local time without an interpretation was converted")
	}
	interpreted, err := local.WithInterpretation(
		core.TimestampInterpretation{Offset: "+09:00", AssertionId: "as:1"})
	if err != nil {
		t.Fatal(err)
	}
	absolute, converted := interpreted.AbsoluteByInterpretation()
	if !converted {
		t.Fatal("the interpreted time was not converted")
	}
	if *absolute.Normalized != "2031-04-04T21:07:08.250Z" || *absolute.RawText != *absolute.Normalized ||
		absolute.OffsetState != core.OffsetStateInValue || absolute.Interpretation != nil ||
		absolute.Precision != core.PrecisionMillisecond || absolute.Clock != core.ClockObserverLocal {
		t.Fatalf("the absolute time is %+v, want the interpreted instant in UTC", absolute)
	}
	want, _ := interpreted.Instant()
	if got, ok := absolute.Instant(); !ok || !got.Equal(want) {
		t.Errorf("the absolute instant is %s, want %s", got, want)
	}
}

// 収集元の対象は内容の識別だけを持つ。
func TestAssertionTargetOnASourceCarriesTheContentDigestAlone(t *testing.T) {
	target := core.AssertionTarget{Kind: core.AssertionTargetKindSource, SourceContentSha256: "not-a-digest"}
	if err := target.Validate(); err == nil {
		t.Error("a source target with a malformed digest passed the validation")
	}
	target = core.AssertionTarget{
		Kind: core.AssertionTargetKindSource, SourceContentSha256: assertionContentSha256, NodeId: "n:x",
	}
	if err := target.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("a source target with a node identifier: err=%v, want ErrInconsistentValue", err)
	}
	node := assertionNodeTarget()
	node.SourceContentSha256 = assertionContentSha256
	if err := node.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("a node target with a source digest: err=%v, want ErrInconsistentValue", err)
	}
}
