package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// windowRadiusUpperBound は symmetric_seconds の半幅の上限である。
const windowRadiusUpperBound = int64(9223372036)

// sameSecondWindow は起点と同じ秒だけを取る時刻の範囲を返す。
func sameSecondWindow() core.TimeWindow {
	center := squidRequestedCenterTime()
	return core.TimeWindow{WindowKind: core.WindowKindSameSecond, CenterTime: &center}
}

// requestedWindowTime は manifest が持つ時刻の範囲の端を、要求が与えた時刻にする。
func requestedWindowTime(value matchingWindowTime) core.RequestedTime {
	return core.RequestedTime{
		RequestText:    value.RequestText,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateUndetermined,
		Normalized:     value.Normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Derivation:     "起点のレコードの日付 2024-03-14 と UTC からのずれ +09:00 を補った",
	}
}

// timeWindowOf は manifest の時刻の範囲 1 件を、要求が持つ時刻の範囲にする。
func timeWindowOf(window matchingWindow) core.TimeWindow {
	built := core.TimeWindow{
		WindowKind:    core.WindowKind(window.WindowKind),
		RadiusSeconds: window.RadiusSeconds,
	}
	if window.LowerBound != nil {
		lower := requestedWindowTime(*window.LowerBound)
		built.LowerBound = &lower
	}
	if window.UpperBound != nil {
		upper := requestedWindowTime(*window.UpperBound)
		built.UpperBound = &upper
	}
	if window.CenterTime != nil {
		center := requestedWindowTime(*window.CenterTime)
		built.CenterTime = &center
	}
	return built
}

// manifestWindow は manifest から名前で時刻の範囲の期待値を 1 件取り出す。
func manifestWindow(t *testing.T, manifest matchingManifest, name string) matchingWindow {
	t.Helper()
	for _, window := range manifest.Case.Windows {
		if window.Name == name {
			return window
		}
	}
	t.Fatalf("the matching manifest carries no window named %q", name)
	return matchingWindow{}
}

// equalInt64Ptr は 2 つの省略可の整数が、同じ有無と同じ値を持つかを返す。
func equalInt64Ptr(got, want *int64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

// zeroRadiusWindowName は manifest が持つ半幅 0 の時刻の範囲の名前である。
const zeroRadiusWindowName = "symmetric_seconds の半幅 0 は起点の秒だけを取る"

// 時刻の範囲の種別と両端ごとに、段階 2 が取る候補を固定する。期待値は manifest の windows が持つ。
//
// 時刻の範囲の中の候補と、中心の前後 1 秒にある候補を fixture が持つため、半幅 0 と半幅 1 が別の
// 候補集合になる。
func TestBuildCandidateSetSelectsMembersByWindow(t *testing.T) {
	manifest := loadMatchingManifest(t)
	for _, window := range manifest.Case.Windows {
		t.Run(window.Name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			request.SecondStageWindow = timeWindowOf(window)
			second := stageByKey(t, buildSet(t, request), core.StageKeySecondTimeMatched)

			if second.MemberCount != int64(len(window.MemberSequenceNumbers)) {
				t.Fatalf("the second_time_matched memberCount is %d, want %d",
					second.MemberCount, len(window.MemberSequenceNumbers))
			}
			if got := sequenceNumbersOf(t, second.Members); !equalInt64Slice(
				got, window.MemberSequenceNumbers) {
				t.Errorf("the second_time_matched members are %v, want %v",
					got, window.MemberSequenceNumbers)
			}
			if string(second.TimeWindow.WindowKind) != window.WindowKind {
				t.Errorf("the second_time_matched windowKind is %q, want %q",
					second.TimeWindow.WindowKind, window.WindowKind)
			}
			if !equalInt64Ptr(second.TimeWindow.RadiusSeconds, window.RadiusSeconds) {
				t.Errorf("the second_time_matched radiusSeconds is %v, want %v",
					second.TimeWindow.RadiusSeconds, window.RadiusSeconds)
			}
		})
	}
}

// 半幅 0 の symmetric_seconds は same_second と同じ候補集合を返し、組も同じ形になる。
func TestBuildCandidateSetAcceptsZeroRadiusWindow(t *testing.T) {
	manifest := loadMatchingManifest(t)
	want := expectedStage(t, manifest, core.StageKeySecondTimeMatched)
	request := matchRequest(t, positiveCandidates(t))
	request.SecondStageWindow = timeWindowOf(manifestWindow(t, manifest, zeroRadiusWindowName))
	second := stageByKey(t, buildSet(t, request), core.StageKeySecondTimeMatched)

	sameSecond := stageByKey(t, buildSet(t, matchRequest(t, positiveCandidates(t))),
		core.StageKeySecondTimeMatched)
	if !equalRecordRefs(t, second.Members, sameSecond.Members) {
		t.Errorf("the members of the zero radius window are %v, want the same_second members %v",
			sequenceNumbersOf(t, second.Members), sequenceNumbersOf(t, sameSecond.Members))
	}
	if second.IndistinguishableGroupCount != want.IndistinguishableGroupCount {
		t.Fatalf("the second_time_matched indistinguishableGroupCount is %d, want %d",
			second.IndistinguishableGroupCount, want.IndistinguishableGroupCount)
	}
	if len(second.IndistinguishableGroups[0]) != want.LargestGroupSize {
		t.Errorf("the second_time_matched group carries %d members, want %d",
			len(second.IndistinguishableGroups[0]), want.LargestGroupSize)
	}
}

// 半幅の下限 0 と上限 windowRadiusUpperBound の両側を通す。
//
// 上限を超える半幅を受理すると、秒から ns への換算が桁あふれして時刻の範囲の下端が上端を超え、
// 時刻の範囲を広げた要求が no_candidate_in_window を返す。
func TestBuildCandidateSetChecksWindowRadiusBounds(t *testing.T) {
	t.Run("上限の半幅は段階 1 の全件を取る", func(t *testing.T) {
		want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
		radius := windowRadiusUpperBound
		second := stageByKey(t, buildSet(t, radiusRequest(t, &radius)),
			core.StageKeySecondTimeMatched)

		if second.MemberCount != want.MemberCount {
			t.Errorf("the second_time_matched memberCount is %d, want the clock_independent %d",
				second.MemberCount, want.MemberCount)
		}
		if second.EmptyReason != "" {
			t.Errorf("the second_time_matched emptyReason is %q, want none", second.EmptyReason)
		}
	})
	rejected := []struct {
		name   string
		radius int64
		want   error
	}{
		{"半幅が上限を 1 超える", windowRadiusUpperBound + 1, core.ErrInvalid},
		{"半幅が負", -1, core.ErrNegativeCount},
	}
	for _, testCase := range rejected {
		t.Run(testCase.name, func(t *testing.T) {
			radius := testCase.radius
			set, err := core.BuildCandidateSet(radiusRequest(t, &radius))
			if err == nil {
				t.Fatalf("building a symmetric_seconds stage with the radius %d returned no error",
					testCase.radius)
			}
			if !errors.Is(err, testCase.want) {
				t.Errorf("the error is %v, want one that wraps %v", err, testCase.want)
			}
			if set.EmptyReason == core.EmptyReasonNoCandidateInWindow {
				t.Errorf("the failed call returned the emptyReason %q", set.EmptyReason)
			}
		})
	}
}

// radiusRequest は半幅を与えた symmetric_seconds の要求を返す。
func radiusRequest(t *testing.T, radiusSeconds *int64) core.MatchRequest {
	t.Helper()
	center := squidRequestedCenterTime()
	request := matchRequest(t, positiveCandidates(t))
	request.SecondStageWindow = core.TimeWindow{
		WindowKind:    core.WindowKindSymmetricSeconds,
		CenterTime:    &center,
		RadiusSeconds: radiusSeconds,
	}
	return request
}

func TestBuildCandidateSetRejectsWindowsThatCompareNoTime(t *testing.T) {
	// 段階 1 の候補が 0 件になる起点と、段階を 1 つも組まない 2 つの起点でも、同じ要求が同じ
	// error になることを固定する。
	cases := []struct {
		name   string
		change func(*core.MatchRequest)
	}{
		{"段階 1 に候補がある", func(*core.MatchRequest) {}},
		{"候補が 1 件も無い", func(request *core.MatchRequest) {
			request.Candidates = nil
		}},
		// 公開を止めた範囲と割当の期間の外にある起点の 2 case は、要求の形と識別子だけで
		// 決まる失敗の判定が 200 の応答の本体の組み立てに先行することを固定する。
		{"公開を止めた範囲である", func(request *core.MatchRequest) {
			request.PublicationState = core.PublicationStateWithheld
		}},
		{"起点が割当の期間の外にある", func(request *core.MatchRequest) {
			request.Origin.AssignmentValidRange = core.TimeRange{
				From: clockTime(t, "2024-03-14T11:10:00.000+09:00", core.ClockTerminalLocal),
				To:   clockTime(t, "2024-03-14T11:20:00.000+09:00", core.ClockTerminalLocal),
			}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			request.SecondStageWindow = core.TimeWindow{WindowKind: core.WindowKindNotCompared}
			testCase.change(&request)

			set, err := core.BuildCandidateSet(request)
			if err == nil {
				t.Fatalf("building a second_time_matched stage with a not_compared window returned no error")
			}
			if !errors.Is(err, core.ErrInconsistentValue) {
				t.Errorf("the error is %v, want one that wraps ErrInconsistentValue", err)
			}
			if len(set.Stages) != 0 || set.EmptyReason != "" {
				t.Errorf("the failed call returned %d stages and the emptyReason %q, want 0 and none",
					len(set.Stages), set.EmptyReason)
			}
		})
	}
}

// 時刻の範囲の時刻を関連付けに使う値として読めない要求は、候補が 1 件も無い場合も失敗する。
func TestBuildCandidateSetRejectsUnreadableWindowTimes(t *testing.T) {
	cases := []struct {
		name   string
		center core.RequestedTime
		want   error
	}{
		{
			name: "正規化値の書式が rfc3339_absolute でない",
			center: core.RequestedTime{
				RequestText:    "10:20:30",
				Precision:      core.PrecisionSecond,
				OffsetState:    core.OffsetStateUndetermined,
				Normalized:     "2024-03-14T10:20:30",
				NormalizedForm: core.NormalizedFormLocalWithoutOffset,
				Derivation:     "起点のレコードの日付 2024-03-14 を補った",
			},
			want: core.ErrMissingRequiredItem,
		},
		{
			name: "正規化値を日時として読めない",
			center: core.RequestedTime{
				RequestText:    "10:20:30",
				Precision:      core.PrecisionSecond,
				OffsetState:    core.OffsetStateUndetermined,
				Normalized:     "2024-03-14 10:20:30 JST",
				NormalizedForm: core.NormalizedFormRFC3339Absolute,
				Derivation:     "起点のレコードの日付 2024-03-14 と UTC からのずれ +09:00 を補った",
			},
			want: core.ErrInconsistentValue,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			center := testCase.center
			request := matchRequest(t, nil)
			request.SecondStageWindow = core.TimeWindow{
				WindowKind: core.WindowKindSameSecond,
				CenterTime: &center,
			}

			if _, err := core.BuildCandidateSet(request); err == nil {
				t.Fatalf("building the candidate set with an unreadable window time returned no error")
			} else if !errors.Is(err, testCase.want) {
				t.Errorf("the error is %v, want one that wraps %v", err, testCase.want)
			}
		})
	}
}

// 時刻を比べられない候補は段階 1 に数え、段階 2 に入れない。
//
// 下端が西暦 1 年の second_range は、比べられない時刻を秒に読み替えた値を含む時刻の範囲である。
// 同じ時刻の範囲で段階 2 が 1 件を返すことが、時刻の範囲の判定と時刻を読めたかの判定を分ける。
func TestBuildCandidateSetSkipsCandidatesWhoseTimeCannotBeCompared(t *testing.T) {
	windows := []struct {
		name   string
		window core.TimeWindow
	}{
		{"same_second", sameSecondWindow()},
		{"下端が西暦 1 年の second_range", firstYearLowerBoundWindow()},
	}
	for _, testCase := range windows {
		t.Run(testCase.name, func(t *testing.T) {
			candidates := []core.MatchCandidateRecord{
				matchCandidate(t, firstCandidateSn, "con", "80", markIIProcessId),
				matchCandidateAt(t, secondCandidateSn, "con", "80", markIIProcessId,
					localTimeWithoutOffset(t, "03/14/2024 10:20:30", "2024-03-14T10:20:30",
						core.ClockTerminalLocal, core.MeaningEvent)),
			}
			request := matchRequest(t, candidates)
			request.SecondStageWindow = testCase.window
			set := buildSet(t, request)

			first := stageByKey(t, set, core.StageKeyClockIndependent)
			if first.MemberCount != int64(len(candidates)) {
				t.Fatalf("the clock_independent memberCount is %d, want %d",
					first.MemberCount, len(candidates))
			}
			second := stageByKey(t, set, core.StageKeySecondTimeMatched)
			if second.MemberCount != 1 {
				t.Fatalf("the second_time_matched memberCount is %d, want 1", second.MemberCount)
			}
			if !equalInt64Slice(sequenceNumbersOf(t, second.Members), []int64{firstCandidateSn}) {
				t.Errorf("the second_time_matched members are %v, want only %d",
					sequenceNumbersOf(t, second.Members), firstCandidateSn)
			}
		})
	}
}

// firstYearLowerBoundWindow は下端が西暦 1 年、上端が起点の秒である second_range を返す。
func firstYearLowerBoundWindow() core.TimeWindow {
	lower := requestedWindowTime(matchingWindowTime{
		RequestText: "0001-01-01T00:00:00Z", Normalized: "0001-01-01T00:00:00Z"})
	upper := requestedWindowTime(matchingWindowTime{
		RequestText: "10:20:30", Normalized: "2024-03-14T10:20:30+09:00"})
	return core.TimeWindow{
		WindowKind: core.WindowKindSecondRange,
		LowerBound: &lower,
		UpperBound: &upper,
	}
}
