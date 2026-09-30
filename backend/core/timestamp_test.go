package core_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestPrecisionKnownValues(t *testing.T) {
	known := []core.Precision{
		"year", "month", "day", "hour", "minute", "second", "millisecond", "microsecond",
	}
	for _, precision := range known {
		if !precision.IsKnown() {
			t.Errorf("precision %q must be a known value", precision)
		}
	}
	unknown := []core.Precision{"nanosecond", "Second", "", "seconds", "Microsecond"}
	for _, precision := range unknown {
		if precision.IsKnown() {
			t.Errorf("precision %q must not be accepted", precision)
		}
	}
}

// epoch の時刻は、UTC からのずれの文字列を持たないまま関連付けに使える。
func TestEpochTimestampCarriesAnInstant(t *testing.T) {
	// 原資料の文字列は auditd の msg=audit(949395600.100:4242) の秒とミリ秒である。
	epoch := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("949395600.100"),
		Normalized:     stringPtr("2000-02-01T09:00:00.100Z"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateEpoch,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	instant, ok := epoch.Instant()
	if !ok {
		t.Fatal("an epoch timestamp carries no instant, want the instant to be derived")
	}
	sameInstantWithOffset := markIIHeaderTime(t,
		"02/01/2000 18:00:00.100 +0900", "2000-02-01T18:00:00.100+09:00")
	want, ok := sameInstantWithOffset.Instant()
	if !ok {
		t.Fatal("the timestamp carrying its offset has no instant")
	}
	if !instant.Equal(want) {
		t.Errorf("instant = %s, want the same instant as %s",
			instant.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// UTC からのずれが未確定の時刻は関連付けに使えない。
func TestUndeterminedOffsetCarriesNoInstant(t *testing.T) {
	undetermined := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("03/14/2024 10:20:30.482"),
		Normalized:     stringPtr("2024-03-14T10:20:30.482"),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateUndetermined,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	if _, ok := undetermined.Instant(); ok {
		t.Error("an undetermined offset carries an instant, want none")
	}
}

// マイクロ秒の精度は小数 6 桁の正規化値を持ち、ミリ秒より細かい。
func TestMicrosecondPrecision(t *testing.T) {
	microsecond := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("[Tue Feb 01 11:22:33.123456 2000]"),
		Normalized:     stringPtr("2000-02-01T11:22:33.123456+09:00"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMicrosecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	if _, ok := microsecond.Instant(); !ok {
		t.Error("a microsecond timestamp carries no instant, want the instant to be derived")
	}
	if core.PrecisionMicrosecond.IsCoarserThan(core.PrecisionMillisecond) {
		t.Error("microsecond reads as coarser than millisecond, want it to be finer")
	}
	if !core.PrecisionMillisecond.IsCoarserThan(core.PrecisionMicrosecond) {
		t.Error("millisecond reads as no coarser than microsecond, want it to be coarser")
	}

	// 小数 3 桁の正規化値は、マイクロ秒の精度と食い違う。
	withThreeDigits := core.Timestamp{
		RawText:        stringPtr("[Tue Feb 01 11:22:33.123 2000]"),
		Normalized:     stringPtr("2000-02-01T11:22:33.123+09:00"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMicrosecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	}
	if _, err := core.NewTimestamp(withThreeDigits); err == nil {
		t.Error("NewTimestamp accepted three fraction digits for microsecond, want a rejection")
	}
}

// ずれを持たない時刻は壁時計の日時を返し、時点を持つ時刻は壁時計の日時を返さない。
func TestLocalClockTimeReadsOnlyTimesWithoutOffset(t *testing.T) {
	local := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("2001/2/3 4:05:06"),
		Normalized:     stringPtr("2001-02-03T04:05:06"),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateUndetermined,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	at, ok := local.LocalClockTime()
	if want := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC); !ok || !at.Equal(want) {
		t.Errorf("LocalClockTime() = %s %v, want %s", at, ok, want)
	}
	if _, instant := local.Instant(); instant {
		t.Error("a time without an offset carries an instant")
	}
	absolute := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("2001-02-03T04:05:06Z"),
		Normalized:     stringPtr("2001-02-03T04:05:06Z"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("Z"),
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	if _, ok := absolute.LocalClockTime(); ok {
		t.Error("an instant was read as a wall clock time")
	}
}

// 入力形式の定義がずれを定める時刻は、ずれの文字列を持たないまま関連付けに使える。
func TestFormatDefinedTimestampCarriesAnInstant(t *testing.T) {
	formatDefined := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("14/Mar/2024:01:20:30"),
		Normalized:     stringPtr("2024-03-14T01:20:30Z"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateFormatDefined,
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
	instant, ok := formatDefined.Instant()
	if !ok {
		t.Fatal("a format_defined timestamp carries no instant, want the instant to be derived")
	}
	if want := time.Date(2024, time.March, 14, 1, 20, 30, 0, time.UTC); !instant.Equal(want) {
		t.Errorf("instant = %s, want %s", instant.Format(time.RFC3339), want.Format(time.RFC3339))
	}

	// 時点が定まる状態の正規化値は、UTC からのずれを持たない形と食い違う。
	withoutOffset := core.Timestamp{
		RawText:        stringPtr("14/Mar/2024:01:20:30"),
		Normalized:     stringPtr("2024-03-14T01:20:30"),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateFormatDefined,
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	}
	if _, err := core.NewTimestamp(withoutOffset); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 利用者が与える時刻に、原資料の文字列だけに該当する状態は該当しない。
func TestRequestedTimeRejectsSourceOnlyOffsetStates(t *testing.T) {
	for _, state := range []core.OffsetState{core.OffsetStateEpoch, core.OffsetStateFormatDefined} {
		requested := core.RequestedTime{
			RequestText: "949395600",
			Precision:   core.PrecisionSecond,
			OffsetState: state,
		}
		if err := requested.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("%s: error = %v, want one wrapping %v", state, err, core.ErrInconsistentValue)
		}
	}
	accepted := core.RequestedTime{
		RequestText: "2024-03-14 10:20:30",
		Precision:   core.PrecisionSecond,
		OffsetState: core.OffsetStateItemAbsent,
	}
	if err := accepted.Validate(); err != nil {
		t.Errorf("a requested time without an offset was rejected: %v", err)
	}
}

func TestInstantReturnsTheNormalizedValue(t *testing.T) {
	instant, ok := markIIEventTime(t).Instant()
	if !ok {
		t.Fatal("the markii header time must have an instant")
	}
	if got := instant.Format(time.RFC3339Nano); got != "2024-03-14T10:20:30.482+09:00" {
		t.Errorf("instant = %q, want %q", got, "2024-03-14T10:20:30.482+09:00")
	}
	want := time.Date(2024, time.March, 14, 1, 20, 30, 482000000, time.UTC)
	if !instant.Equal(want) {
		t.Errorf("instant = %v, want %v", instant.UTC(), want)
	}
}

// Instant が ok を真にする 3 つの条件を 1 つずつ崩す。
func TestInstantIsAbsentWhenAConditionFails(t *testing.T) {
	t.Run("offsetState is not in_value", func(t *testing.T) {
		if _, ok := markIIStartTime(t).Instant(); ok {
			t.Error("a time whose offset is undetermined must not have an instant")
		}
	})

	t.Run("valueState is not present", func(t *testing.T) {
		timestamp := newTimestamp(t, core.Timestamp{
			RawText:        stringPtr("-"),
			Normalized:     stringPtr("2024-03-14T10:20:30+09:00"),
			NormalizedForm: core.NormalizedFormRFC3339Absolute,
			Precision:      core.PrecisionSecond,
			OffsetState:    core.OffsetStateInValue,
			OffsetText:     stringPtr("+0900"),
			Clock:          core.ClockObserverLocal,
			Meaning:        core.MeaningRecordOutput,
			ValueState:     core.ValueStateOutOfDefinition,
		})
		if _, ok := timestamp.Instant(); ok {
			t.Error("a time outside the definition of the format must not have an instant")
		}
	})

	t.Run("normalized is not a valid datetime", func(t *testing.T) {
		timestamp := newTimestamp(t, core.Timestamp{
			RawText:        stringPtr("14/Mar/2024"),
			Normalized:     stringPtr("2024-03-14"),
			NormalizedForm: core.NormalizedFormPartialDateTime,
			Precision:      core.PrecisionDay,
			OffsetState:    core.OffsetStateInValue,
			OffsetText:     stringPtr("+0900"),
			Clock:          core.ClockObserverLocal,
			Meaning:        core.MeaningRecordOutput,
			ValueState:     core.ValueStatePresent,
		})
		if _, ok := timestamp.Instant(); ok {
			t.Error("a normalized value without a time of day must not have an instant")
		}
	})
}

func TestTimestampJsonRoundTripKeepsEveryItem(t *testing.T) {
	cases := map[string]struct {
		timestamp core.Timestamp
		wantJson  string
	}{
		"offset in the value": {
			timestamp: markIIEventTime(t),
			wantJson: `{"rawText":"03/14/2024 10:20:30.482 +0900",` +
				`"normalized":"2024-03-14T10:20:30.482+09:00",` +
				`"normalizedForm":"rfc3339_absolute","precision":"millisecond",` +
				`"offsetState":"in_value","offsetText":"+0900","clock":"terminal_local",` +
				`"meaning":"event","valueState":"present"}`,
		},
		"offset undetermined": {
			timestamp: markIIStartTime(t),
			wantJson: `{"rawText":"03/14/2024 09:30:00.120",` +
				`"normalized":"2024-03-14T09:30:00.120",` +
				`"normalizedForm":"local_without_offset","precision":"millisecond",` +
				`"offsetState":"undetermined","clock":"terminal_local",` +
				`"meaning":"operation_start","valueState":"present"}`,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.timestamp)
			if err != nil {
				t.Fatalf("marshaling the timestamp: %v", err)
			}
			if string(encoded) != testCase.wantJson {
				t.Errorf("json = %s, want %s", encoded, testCase.wantJson)
			}

			var decoded core.Timestamp
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decoding the timestamp: %v", err)
			}
			assertSameTimestampItems(t, testCase.timestamp, decoded)

			wantInstant, wantOk := testCase.timestamp.Instant()
			gotInstant, gotOk := decoded.Instant()
			if gotOk != wantOk {
				t.Fatalf("Instant ok = %v, want %v", gotOk, wantOk)
			}
			if wantOk {
				if !gotInstant.Equal(wantInstant) {
					t.Errorf("instant = %v, want %v", gotInstant, wantInstant)
				}
				if gotInstant.Format(time.RFC3339Nano) != wantInstant.Format(time.RFC3339Nano) {
					t.Errorf("instant text = %q, want %q",
						gotInstant.Format(time.RFC3339Nano), wantInstant.Format(time.RFC3339Nano))
				}
			}
			if !decoded.Equal(testCase.timestamp) {
				t.Error("the decoded timestamp must equal the encoded one")
			}
		})
	}
}

// assertSameOptionalString は省略可の文字列の項目を、値と「出ているか」の両方で
// 突き合わせる。pointer 同士を比べると、同じ文字列を指す別の pointer が食い違いとして
// 報告される。
func assertSameOptionalString(t *testing.T, itemName string, want, got func() (string, bool)) {
	t.Helper()
	wantValue, wantPresent := want()
	gotValue, gotPresent := got()
	if gotPresent != wantPresent {
		t.Errorf("%s present = %t, want %t", itemName, gotPresent, wantPresent)
		return
	}
	if gotValue != wantValue {
		t.Errorf("%s = %q, want %q", itemName, gotValue, wantValue)
	}
}

// assertSameTimestampItems は 9 項目を 1 つずつ突き合わせる。
func assertSameTimestampItems(t *testing.T, want, got core.Timestamp) {
	t.Helper()
	assertSameOptionalString(t, "rawText", want.RawTextValue, got.RawTextValue)
	assertSameOptionalString(t, "normalized", want.NormalizedValue, got.NormalizedValue)
	if got.NormalizedForm != want.NormalizedForm {
		t.Errorf("normalizedForm = %q, want %q", got.NormalizedForm, want.NormalizedForm)
	}
	if got.Precision != want.Precision {
		t.Errorf("precision = %q, want %q", got.Precision, want.Precision)
	}
	if got.OffsetState != want.OffsetState {
		t.Errorf("offsetState = %q, want %q", got.OffsetState, want.OffsetState)
	}
	assertSameOptionalString(t, "offsetText", want.OffsetTextValue, got.OffsetTextValue)
	if got.Clock != want.Clock {
		t.Errorf("clock = %q, want %q", got.Clock, want.Clock)
	}
	if got.Meaning != want.Meaning {
		t.Errorf("meaning = %q, want %q", got.Meaning, want.Meaning)
	}
	if got.ValueState != want.ValueState {
		t.Errorf("valueState = %q, want %q", got.ValueState, want.ValueState)
	}
}

// offsetState が in_value 以外でも offsetText を持てる。offsetText は省略可で、
// offsetState が in_value のときだけ必須になる。
// UTC からのずれが未確定の時刻でも、原資料が持つ文字列は保持する。
func TestTimestampKeepsTheOffsetTextWhenTheOffsetIsUndetermined(t *testing.T) {
	timestamp := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("03/14/2024 09:30:00.120"),
		Normalized:     stringPtr("2024-03-14T09:30:00.120"),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateUndetermined,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningOperationStart,
		ValueState:     core.ValueStatePresent,
	})
	offsetText, ok := timestamp.OffsetTextValue()
	if !ok {
		t.Fatal("the offsetText of an undetermined offset must be kept")
	}
	if offsetText != "+0900" {
		t.Errorf("offsetText = %q, want %q", offsetText, "+0900")
	}
	// ずれが未確定である以上、文字列があっても時刻は導けない。
	if _, hasInstant := timestamp.Instant(); hasInstant {
		t.Error("a time whose offset is undetermined must not have an instant")
	}
}

func TestNewTimestampRejectsInconsistentItems(t *testing.T) {
	cases := map[string]struct {
		items core.Timestamp
		want  error
	}{
		"offsetText is absent while the offset is in the value": {
			items: core.Timestamp{
				RawText:     stringPtr("03/14/2024 10:20:30.482 +0900"),
				Normalized:  stringPtr("2024-03-14T10:20:30.482+09:00"),
				Precision:   core.PrecisionMillisecond,
				OffsetState: core.OffsetStateInValue,
				Clock:       core.ClockTerminalLocal,
				Meaning:     core.MeaningEvent,
				ValueState:  core.ValueStatePresent,
			},
			want: core.ErrMissingRequiredItem,
		},
		"normalized is absent while the value is present": {
			items: core.Timestamp{
				RawText:     stringPtr("03/14/2024 10:20:30.482 +0900"),
				Precision:   core.PrecisionMillisecond,
				OffsetState: core.OffsetStateInValue,
				OffsetText:  stringPtr("+0900"),
				Clock:       core.ClockTerminalLocal,
				Meaning:     core.MeaningEvent,
				ValueState:  core.ValueStatePresent,
			},
			want: core.ErrMissingRequiredItem,
		},
		"precision is an unknown value": {
			items: core.Timestamp{
				RawText:        stringPtr("2024-03-14T10:20:30.482789456+09:00"),
				Normalized:     stringPtr("2024-03-14T10:20:30.482789456+09:00"),
				NormalizedForm: core.NormalizedFormRFC3339Absolute,
				Precision:      "nanosecond",
				OffsetState:    core.OffsetStateInValue,
				OffsetText:     stringPtr("+0900"),
				Clock:          core.ClockTerminalLocal,
				Meaning:        core.MeaningEvent,
				ValueState:     core.ValueStatePresent,
			},
			want: core.ErrUnknownEnumValue,
		},
		"rawText is absent": {
			items: core.Timestamp{
				Normalized:     stringPtr("2024-03-14T10:20:30.482+09:00"),
				NormalizedForm: core.NormalizedFormRFC3339Absolute,
				Precision:      core.PrecisionMillisecond,
				OffsetState:    core.OffsetStateInValue,
				OffsetText:     stringPtr("+0900"),
				Clock:          core.ClockTerminalLocal,
				Meaning:        core.MeaningEvent,
				ValueState:     core.ValueStatePresent,
			},
			want: core.ErrMissingRequiredItem,
		},
		// valueState が item_absent の時刻は原資料の文字列と正規化値を出さない。
		"rawText is present while the item is absent": {
			items: core.Timestamp{
				RawText:     stringPtr("03/14/2024 10:20:30.482 +0900"),
				Precision:   core.PrecisionMillisecond,
				OffsetState: core.OffsetStateItemAbsent,
				Clock:       core.ClockTerminalLocal,
				Meaning:     core.MeaningEvent,
				ValueState:  core.ValueStateItemAbsent,
			},
			want: core.ErrUnexpectedItem,
		},
		// normalizedForm の値は offsetState と precision から一意に決まる。
		"normalizedForm contradicts the offsetState": {
			items: core.Timestamp{
				RawText:        stringPtr("03/14/2024 09:30:00.120"),
				Normalized:     stringPtr("2024-03-14T09:30:00.120"),
				NormalizedForm: core.NormalizedFormRFC3339Absolute,
				Precision:      core.PrecisionMillisecond,
				OffsetState:    core.OffsetStateUndetermined,
				Clock:          core.ClockTerminalLocal,
				Meaning:        core.MeaningOperationStart,
				ValueState:     core.ValueStatePresent,
			},
			want: core.ErrInconsistentValue,
		},
		"normalizedForm is absent while normalized is present": {
			items: core.Timestamp{
				RawText:     stringPtr("03/14/2024 09:30:00.120"),
				Normalized:  stringPtr("2024-03-14T09:30:00.120"),
				Precision:   core.PrecisionMillisecond,
				OffsetState: core.OffsetStateUndetermined,
				Clock:       core.ClockTerminalLocal,
				Meaning:     core.MeaningOperationStart,
				ValueState:  core.ValueStatePresent,
			},
			want: core.ErrUnknownEnumValue,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := core.NewTimestamp(testCase.items); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// struct literal で作った値は内部の時刻を持たないため、直列化の前に拒否する。
func TestMarshalRejectsATimestampBuiltWithoutTheConstructor(t *testing.T) {
	literal := core.Timestamp{
		RawText:        stringPtr("03/14/2024 10:20:30.482 +0900"),
		Normalized:     stringPtr("2024-03-14T10:20:30.482+09:00"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	}
	if _, ok := literal.Instant(); ok {
		t.Error("a timestamp built with a struct literal must not have an instant")
	}
	if err := literal.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("Validate error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
	if _, err := json.Marshal(literal); err == nil {
		t.Error("marshaling a timestamp without an internal instant must fail")
	}
}

func TestUnmarshalRejectsBrokenJson(t *testing.T) {
	cases := map[string]string{
		"an item outside the contract": `{"rawText":"a","normalized":"2024-03-14T10:20:30+09:00",` +
			`"precision":"second","offsetState":"in_value","offsetText":"+0900",` +
			`"clock":"observer_local","meaning":"event","valueState":"present","extra":1}`,
		"offsetText is absent while the offset is in the value": `{"rawText":"a",` +
			`"normalized":"2024-03-14T10:20:30+09:00","precision":"second",` +
			`"offsetState":"in_value","clock":"observer_local","meaning":"event",` +
			`"valueState":"present"}`,
		"clock is outside the contract": `{"rawText":"a",` +
			`"normalized":"2024-03-14T10:20:30+09:00","precision":"second",` +
			`"offsetState":"in_value","offsetText":"+0900","clock":"server_local",` +
			`"meaning":"event","valueState":"present"}`,
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			var decoded core.Timestamp
			if err := json.Unmarshal([]byte(encoded), &decoded); err == nil {
				t.Error("decoding must fail")
			}
		})
	}
}

// 秒精度の側とミリ秒精度の側を比べても、両側の精度が書き換わらない。
func TestTimeComparisonKeepsBothPrecisions(t *testing.T) {
	leftTime := squidRequestTime(t)
	rightTime := markIIEventTime(t)
	comparison := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		LeftTime:       &leftTime,
		RightTime:      &rightTime,
		Assumptions:    []core.MatchAssumption{clockOffsetAssumption()},
	}
	if err := comparison.Validate(); err != nil {
		t.Fatalf("validating the comparison: %v", err)
	}
	if comparison.LeftTime.Precision != core.PrecisionSecond {
		t.Errorf("leftTime precision = %q, want %q",
			comparison.LeftTime.Precision, core.PrecisionSecond)
	}
	if comparison.RightTime.Precision != core.PrecisionMillisecond {
		t.Errorf("rightTime precision = %q, want %q",
			comparison.RightTime.Precision, core.PrecisionMillisecond)
	}
}

func TestTimeComparisonRejectsMissingTimes(t *testing.T) {
	comparison := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		Assumptions:    []core.MatchAssumption{},
	}
	if err := comparison.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}

	leftTime := squidRequestTime(t)
	notCompared := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitNotCompared,
		LeftTime:       &leftTime,
		Assumptions:    []core.MatchAssumption{},
	}
	if err := notCompared.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}
}

// 正規化値の書式は offsetState と precision から一意に決まる。3 値それぞれを通す。
func TestNormalizedFormFollowsTheOffsetStateAndPrecision(t *testing.T) {
	cases := map[string]struct {
		timestamp core.Timestamp
		want      core.NormalizedForm
	}{
		"an absolute time": {
			timestamp: markIIEventTime(t),
			want:      core.NormalizedFormRFC3339Absolute,
		},
		"a time without an offset": {
			timestamp: markIIStartTime(t),
			want:      core.NormalizedFormLocalWithoutOffset,
		},
		"a time with a coarser precision": {
			timestamp: newTimestamp(t, core.Timestamp{
				RawText:        stringPtr("14/Mar/2024"),
				Normalized:     stringPtr("2024-03-14"),
				NormalizedForm: core.NormalizedFormPartialDateTime,
				Precision:      core.PrecisionDay,
				OffsetState:    core.OffsetStateItemAbsent,
				Clock:          core.ClockFileProperty,
				Meaning:        core.MeaningProperty,
				ValueState:     core.ValueStatePresent,
			}),
			want: core.NormalizedFormPartialDateTime,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if got := testCase.timestamp.NormalizedForm; got != testCase.want {
				t.Errorf("normalizedForm = %q, want %q", got, testCase.want)
			}
			if err := testCase.timestamp.Validate(); err != nil {
				t.Fatalf("validating the timestamp: %v", err)
			}
		})
	}
}

// 正規化値の文字列は normalizedForm の書式に従う。
//
// local_without_offset の値に Z と +00:00 を付けず、partial_date_time の値に存在しない
// 下位の桁を補わない。
func TestNormalizedTextFollowsTheFormat(t *testing.T) {
	localWithoutOffset := func(normalized string) core.Timestamp {
		return core.Timestamp{
			RawText:        stringPtr("03/14/2024 09:30:00.120"),
			Normalized:     stringPtr(normalized),
			NormalizedForm: core.NormalizedFormLocalWithoutOffset,
			Precision:      core.PrecisionMillisecond,
			OffsetState:    core.OffsetStateUndetermined,
			Clock:          core.ClockTerminalLocal,
			Meaning:        core.MeaningOperationStart,
			ValueState:     core.ValueStatePresent,
		}
	}
	partialDateTime := func(normalized string, precision core.Precision) core.Timestamp {
		return core.Timestamp{
			RawText:        stringPtr("14/Mar/2024"),
			Normalized:     stringPtr(normalized),
			NormalizedForm: core.NormalizedFormPartialDateTime,
			Precision:      precision,
			OffsetState:    core.OffsetStateItemAbsent,
			Clock:          core.ClockFileProperty,
			Meaning:        core.MeaningProperty,
			ValueState:     core.ValueStatePresent,
		}
	}

	accepted := map[string]core.Timestamp{
		"a local time without an offset": localWithoutOffset("2024-03-14T09:30:00.120"),
		"a date":                         partialDateTime("2024-03-14", core.PrecisionDay),
		"a month":                        partialDateTime("2024-03", core.PrecisionMonth),
		"a minute":                       partialDateTime("2024-03-14T09:30", core.PrecisionMinute),
	}
	for name, items := range accepted {
		t.Run(name, func(t *testing.T) {
			if _, err := core.NewTimestamp(items); err != nil {
				t.Errorf("building the timestamp: %v", err)
			}
		})
	}

	rejected := map[string]core.Timestamp{
		"a local time carrying Z":       localWithoutOffset("2024-03-14T09:30:00.120Z"),
		"a local time carrying +00:00":  localWithoutOffset("2024-03-14T09:30:00.120+00:00"),
		"a month padded to a date time": partialDateTime("2024-03-01T00:00:00", core.PrecisionMonth),
		"a date padded to a date time":  partialDateTime("2024-03-14T00:00:00", core.PrecisionDay),
	}
	for name, items := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := core.NewTimestamp(items); !errors.Is(err, core.ErrInconsistentValue) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
			}
		})
	}
}

// 要求が与えた時刻は rawText と clock と meaning を持たない。
// 「正規化値の 3 つの書式」の成立条件の欄を RequestedTime に適用しない。
func TestRequestedTimeCarriesThePrecisionWithoutAClock(t *testing.T) {
	requested := squidRequestedCenterTime()
	if err := requested.Validate(); err != nil {
		t.Fatalf("validating the requested time: %v", err)
	}
	if requested.Precision != core.PrecisionSecond {
		t.Errorf("precision = %q, want %q", requested.Precision, core.PrecisionSecond)
	}

	// offsetState が undetermined でも normalizedForm は rfc3339_absolute を取れる。
	// 補った桁とずれの出どころは derivation が表す。
	if requested.OffsetState != core.OffsetStateUndetermined {
		t.Errorf("offsetState = %q, want %q", requested.OffsetState, core.OffsetStateUndetermined)
	}
	if requested.NormalizedForm != core.NormalizedFormRFC3339Absolute {
		t.Errorf("normalizedForm = %q, want %q",
			requested.NormalizedForm, core.NormalizedFormRFC3339Absolute)
	}

	t.Run("without a derivation", func(t *testing.T) {
		broken := requested
		broken.Derivation = ""
		if err := broken.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("without a normalized value", func(t *testing.T) {
		undetermined := core.RequestedTime{
			RequestText: "10:20:30",
			Precision:   core.PrecisionSecond,
			OffsetState: core.OffsetStateUndetermined,
		}
		if err := undetermined.Validate(); err != nil {
			t.Fatalf("validating a requested time without a normalized value: %v", err)
		}
	})

	t.Run("a derivation without a normalized value", func(t *testing.T) {
		broken := requested
		broken.Normalized = ""
		broken.NormalizedForm = ""
		if err := broken.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})

	t.Run("without the request text", func(t *testing.T) {
		broken := requested
		broken.RequestText = ""
		if err := broken.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("an item outside the contract", func(t *testing.T) {
		var decoded core.RequestedTime
		encoded := `{"requestText":"10:20:30","precision":"second",` +
			`"offsetState":"undetermined","clock":"observer_local"}`
		if err := json.Unmarshal([]byte(encoded), &decoded); err == nil {
			t.Error("decoding a requested time with a clock must fail")
		}
	})
}
