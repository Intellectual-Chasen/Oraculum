package core_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 2 つの段階は別々の要素数を持つ。段階 2 の要素は段階 1 の要素の部分集合である。
func TestCandidateSetHoldsBothStagesIndependently(t *testing.T) {
	stageOneMembers := []core.Candidate{
		notComparedCandidate(t, firstCandidateSn),
		notComparedCandidate(t, secondCandidateSn),
		notComparedCandidate(t, thirdCandidateSn),
		notComparedCandidate(t, fourthCandidateSn),
	}
	stageTwoMembers := []core.Candidate{
		secondMatchedCandidate(t, firstCandidateSn),
		secondMatchedCandidate(t, secondCandidateSn),
	}
	set := candidateSet(t, []core.CandidateStage{
		clockIndependentStage(t, stageOneMembers, ""),
		secondTimeMatchedStage(t, stageTwoMembers, ""),
	}, "")
	if err := set.Validate(); err != nil {
		t.Fatalf("validating the candidate set: %v", err)
	}
	if set.Stages[0].MemberCount != int64(len(stageOneMembers)) {
		t.Errorf("clock_independent memberCount = %d, want %d",
			set.Stages[0].MemberCount, len(stageOneMembers))
	}
	if set.Stages[1].MemberCount != int64(len(stageTwoMembers)) {
		t.Errorf("second_time_matched memberCount = %d, want %d",
			set.Stages[1].MemberCount, len(stageTwoMembers))
	}
	if set.Stages[0].Members[0].RelationState != core.RelationStateCandidate {
		t.Errorf("relationState = %q, want %q",
			set.Stages[0].Members[0].RelationState, core.RelationStateCandidate)
	}
}

// 候補集合の JSON の往復の後も、時刻の比較と件数が変わらない。
func TestCandidateSetJsonRoundTripKeepsTheComparison(t *testing.T) {
	set := candidateSet(t, []core.CandidateStage{
		clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, secondCandidateSn)}, ""),
		secondTimeMatchedStage(t, []core.Candidate{secondMatchedCandidate(t, secondCandidateSn)}, ""),
	}, "")
	encoded, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshaling the candidate set: %v", err)
	}

	var decoded core.CandidateSet
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding the candidate set: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("validating the decoded candidate set: %v", err)
	}
	if len(decoded.Stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(decoded.Stages))
	}
	if decoded.Stages[1].MemberCount != 1 {
		t.Errorf("second_time_matched memberCount = %d, want 1", decoded.Stages[1].MemberCount)
	}

	wantCandidate := set.Stages[1].Members[0]
	gotCandidate := decoded.Stages[1].Members[0]
	assertSameTimestampItems(t, wantCandidate.EventTime, gotCandidate.EventTime)
	wantInstant, wantOk := wantCandidate.TimeComparison.RightTime.Instant()
	gotInstant, gotOk := gotCandidate.TimeComparison.RightTime.Instant()
	if gotOk != wantOk {
		t.Fatalf("Instant ok = %v, want %v", gotOk, wantOk)
	}
	if !gotInstant.Equal(wantInstant) {
		t.Errorf("instant = %v, want %v", gotInstant, wantInstant)
	}
	if !gotCandidate.TimeComparison.RightTime.Equal(gotCandidate.EventTime) {
		t.Error("the decoded comparison must still point at the event time of its candidate")
	}
}

// 段階 1 が 0 件なら段階 2 も 0 件である。
func TestCandidateSetRejectsCandidatesOnlyInTheSecondStage(t *testing.T) {
	stageOne := clockIndependentStage(t, []core.Candidate{}, core.EmptyReasonCounterpartItemAbsent)
	stageTwo := secondTimeMatchedStage(t, []core.Candidate{
		secondMatchedCandidate(t, secondCandidateSn),
	}, "")
	set := candidateSet(t, []core.CandidateStage{stageOne, stageTwo}, "")
	if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 段階 2 だけが 0 件になる応答は成り立つ。段階 1 の候補は消えない。
func TestCandidateSetKeepsTheFirstStageWhenTheSecondIsEmpty(t *testing.T) {
	stageOne := clockIndependentStage(t, []core.Candidate{
		notComparedCandidate(t, firstCandidateSn),
	}, "")
	stageTwo := secondTimeMatchedStage(t, []core.Candidate{},
		core.EmptyReasonNoCandidateInWindow)
	set := candidateSet(t, []core.CandidateStage{stageOne, stageTwo}, "")
	if err := set.Validate(); err != nil {
		t.Fatalf("validating the candidate set: %v", err)
	}
	if set.Stages[0].MemberCount != 1 {
		t.Errorf("clock_independent memberCount = %d, want 1", set.Stages[0].MemberCount)
	}
	if set.Stages[1].EmptyReason != core.EmptyReasonNoCandidateInWindow {
		t.Errorf("emptyReason = %q, want %q",
			set.Stages[1].EmptyReason, core.EmptyReasonNoCandidateInWindow)
	}
}

// emptyReason は要素数 0 のときだけ出る。
func TestStageEmptyReasonAppearsOnlyWithoutMembers(t *testing.T) {
	t.Run("a reason with members", func(t *testing.T) {
		stage := clockIndependentStage(t, []core.Candidate{
			notComparedCandidate(t, firstCandidateSn),
		}, core.EmptyReasonNoCandidateInWindow)
		if err := stage.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})

	t.Run("no reason without members", func(t *testing.T) {
		stage := clockIndependentStage(t, []core.Candidate{}, "")
		if err := stage.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})

	t.Run("a reason belonging to another place", func(t *testing.T) {
		stage := clockIndependentStage(t, []core.Candidate{}, core.EmptyReasonNoRecordInFilter)
		if err := stage.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})
}

// 段階を 0 件で返す候補集合は、起点が割当の期間の外にある理由を持つ。
func TestCandidateSetWithoutStagesCarriesTheReason(t *testing.T) {
	t.Run("with the reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{},
			core.EmptyReasonOriginOutsideAssignmentRange)
		if err := set.Validate(); err != nil {
			t.Fatalf("validating the candidate set: %v", err)
		}
	})

	t.Run("without the reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{}, "")
		if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 関連付けが取り出す語彙の項目を起点が比べられる形で持たない集合は
	// counterpart_item_absent を置く。**割当の期間の外と同じ文字列にしない。**
	t.Run("with the counterpart item reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{},
			core.EmptyReasonCounterpartItemAbsent)
		if err := set.Validate(); err != nil {
			t.Fatalf("validating a candidate set carrying the counterpart reason: %v", err)
		}
	})

	t.Run("with a reason belonging to another place", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{}, core.EmptyReasonNoCandidateInWindow)
		if err := set.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
		}
	})

	// 公開を止めた応答は publication_withheld を置く。2 値は別の状態を表すため、
	// origin_outside_assignment_range を名乗らせない。
	t.Run("withheld with the publication reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{}, core.EmptyReasonPublicationWithheld)
		set.PublicationState = core.PublicationStateWithheld
		if err := set.Validate(); err != nil {
			t.Fatalf("validating a withheld candidate set: %v", err)
		}
	})

	t.Run("withheld carrying the outside range reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{},
			core.EmptyReasonOriginOutsideAssignmentRange)
		set.PublicationState = core.PublicationStateWithheld
		if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("published carrying the publication reason", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{}, core.EmptyReasonPublicationWithheld)
		if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("withheld while carrying stages", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{
			clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, ""),
		}, "")
		set.PublicationState = core.PublicationStateWithheld
		if err := set.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})

	t.Run("with a reason next to stages", func(t *testing.T) {
		set := candidateSet(t, []core.CandidateStage{
			clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, ""),
		}, core.EmptyReasonOriginOutsideAssignmentRange)
		if err := set.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})
}

// 未処理の範囲が 1 件以上ある候補集合は published_full を返さない。
func TestCandidateSetWithUnprocessedRangesIsNotPublishedFull(t *testing.T) {
	set := candidateSet(t, []core.CandidateStage{
		clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, ""),
	}, "")
	set.UnprocessedRanges = []core.RecordRange{
		markIIRange(rangeFromSequenceNumber, rangeFromSequenceNumber+10),
	}
	set.UnprocessedRangeCount = 1

	if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	set.PublicationState = core.PublicationStatePublishedPartial
	if err := set.Validate(); err != nil {
		t.Fatalf("validating a partially published candidate set: %v", err)
	}
}

// 段階の種別と、時刻を比べた単位と時刻の範囲が対応する。
func TestStageKindMatchesTheComparison(t *testing.T) {
	t.Run("a clock_independent stage comparing seconds", func(t *testing.T) {
		stage := clockIndependentStage(t, []core.Candidate{
			notComparedCandidate(t, firstCandidateSn),
		}, "")
		stage.ComparisonUnit = core.ComparisonUnitSecond
		if err := stage.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a member comparing time in another unit", func(t *testing.T) {
		stage := secondTimeMatchedStage(t, []core.Candidate{
			notComparedCandidate(t, firstCandidateSn),
		}, "")
		if err := stage.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 候補の rightTime は同じ候補の eventTime と等しい。
func TestCandidateRejectsAComparisonAgainstAnotherTime(t *testing.T) {
	candidate := secondMatchedCandidate(t, secondCandidateSn)
	otherTime := squidRequestTime(t)
	candidate.TimeComparison.RightTime = &otherTime
	if err := candidate.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// memberCount は members の要素数と一致する。段階は候補を全件持つ。
func TestStageMemberCountEqualsTheNumberOfMembers(t *testing.T) {
	members := []core.Candidate{
		notComparedCandidate(t, firstCandidateSn),
		notComparedCandidate(t, secondCandidateSn),
	}
	stage := clockIndependentStage(t, members, "")
	if err := stage.Validate(); err != nil {
		t.Fatalf("validating the stage: %v", err)
	}
	if stage.MemberCount != int64(len(stage.Members)) {
		t.Errorf("memberCount = %d, want len(members) = %d",
			stage.MemberCount, len(stage.Members))
	}

	t.Run("a total differing from the number of members", func(t *testing.T) {
		broken := stage
		broken.MemberCount = int64(len(members)) + 1
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// unprocessedRangeCount は unprocessedRanges の要素数と一致する。
func TestCandidateSetUnprocessedRangeCountEqualsTheNumberOfRanges(t *testing.T) {
	set := candidateSet(t, []core.CandidateStage{
		clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, ""),
	}, "")
	set.UnprocessedRanges = []core.RecordRange{
		markIIRange(rangeFromSequenceNumber, rangeFromSequenceNumber+10),
	}
	set.UnprocessedRangeCount = int64(len(set.UnprocessedRanges))
	set.PublicationState = core.PublicationStatePublishedPartial
	if err := set.Validate(); err != nil {
		t.Fatalf("validating the candidate set: %v", err)
	}

	set.UnprocessedRangeCount = int64(len(set.UnprocessedRanges)) + 1
	if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 互いに区別できない要素の組は、複数のレコードを 1 つの要素として持つ。
// 組の総数は members と別の項目が持つ。
//
// 組 1 つあたりの要素数の下限は関連付けの実装が決めるため、検査しない。
func TestIndistinguishableGroupsHoldSeveralRecords(t *testing.T) {
	stage := clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, "")
	stage.IndistinguishableGroups = [][]core.RecordLocator{
		{squidLocator(260), squidLocator(262), squidLocator(263)},
	}
	stage.IndistinguishableGroupCount = 1
	if err := stage.Validate(); err != nil {
		t.Fatalf("validating the stage: %v", err)
	}
	if len(stage.IndistinguishableGroups[0]) != 3 {
		t.Errorf("group = %d records, want 3", len(stage.IndistinguishableGroups[0]))
	}
	if stage.IndistinguishableGroupCount != 1 {
		t.Errorf("indistinguishableGroupCount = %d, want 1", stage.IndistinguishableGroupCount)
	}

	// 総数は応答に入れた組の要素数と一致する。
	t.Run("a total differing from the number of returned groups", func(t *testing.T) {
		broken := stage
		broken.IndistinguishableGroupCount = int64(len(stage.IndistinguishableGroups)) + 1
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// comparisonUnit が not_compared の段階は、時刻の条件を用いた形にしない。
func TestStageRejectsAUsedTimeConditionWhileItComparesNoTime(t *testing.T) {
	// 段階 1 は comparisonUnit が not_compared で、second_of_time の use が not_used である。
	stage := clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, "")
	if err := stage.Validate(); err != nil {
		t.Fatalf("validating the first stage: %v", err)
	}

	// 同じ段階の second_of_time を used に変えた形は検査で失敗する。
	broken := stage
	broken.Conditions = stageConditions(t, core.StageKeySecondTimeMatched, true)
	if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 時刻の範囲は種別ごとに必要な項目を持つ。既定値を持たない。
// 境界と中心は要求が与えた時刻であり、RequestedTime が持つ。
func TestTimeWindowRequiresTheItemsOfItsKind(t *testing.T) {
	centerTime := squidRequestedCenterTime()
	radius := int64(60)
	cases := map[string]struct {
		window core.TimeWindow
		want   error
	}{
		"symmetric_seconds without a radius": {
			window: core.TimeWindow{
				WindowKind: core.WindowKindSymmetricSeconds,
				CenterTime: &centerTime,
			},
			want: core.ErrMissingRequiredItem,
		},
		"same_second with a radius": {
			window: core.TimeWindow{
				WindowKind:    core.WindowKindSameSecond,
				CenterTime:    &centerTime,
				RadiusSeconds: &radius,
			},
			want: core.ErrUnexpectedItem,
		},
		"second_range without bounds": {
			window: core.TimeWindow{WindowKind: core.WindowKindSecondRange},
			want:   core.ErrMissingRequiredItem,
		},
		"second_range without an upper bound": {
			window: core.TimeWindow{
				WindowKind: core.WindowKindSecondRange,
				LowerBound: &centerTime,
			},
			want: core.ErrMissingRequiredItem,
		},
		"second_range without a lower bound": {
			window: core.TimeWindow{
				WindowKind: core.WindowKindSecondRange,
				UpperBound: &centerTime,
			},
			want: core.ErrMissingRequiredItem,
		},
		"not_compared with a center": {
			window: core.TimeWindow{
				WindowKind: core.WindowKindNotCompared,
				CenterTime: &centerTime,
			},
			want: core.ErrUnexpectedItem,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if err := testCase.window.Validate(); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}

	valid := core.TimeWindow{
		WindowKind:    core.WindowKindSymmetricSeconds,
		CenterTime:    &centerTime,
		RadiusSeconds: &radius,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("validating a symmetric window: %v", err)
	}

	// 両端を揃えた second_range は通る。片方だけを欠いた 2 つの case と対をなす。
	bounded := core.TimeWindow{
		WindowKind: core.WindowKindSecondRange,
		LowerBound: &centerTime,
		UpperBound: &centerTime,
	}
	if err := bounded.Validate(); err != nil {
		t.Fatalf("validating a second_range window with both bounds: %v", err)
	}
}

// 時刻の範囲の半幅の下限は 0、上限は windowRadiusUpperBound である。
//
// 半幅 0 の時刻の範囲は centerTime の秒だけを取る。
// 型が拒否するのは 10 進整数が符号を持たないことに反する負の値である。
func TestTimeWindowChecksRadiusBounds(t *testing.T) {
	cases := []struct {
		name   string
		radius int64
		want   error
	}{
		{"下限の 0", 0, nil},
		{"下限を 1 下回る", -1, core.ErrNegativeCount},
		{"上限", windowRadiusUpperBound, nil},
		{"上限を 1 上回る", windowRadiusUpperBound + 1, core.ErrInvalid},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			centerTime := squidRequestedCenterTime()
			radius := testCase.radius
			window := core.TimeWindow{
				WindowKind:    core.WindowKindSymmetricSeconds,
				CenterTime:    &centerTime,
				RadiusSeconds: &radius,
			}

			err := window.Validate()
			if testCase.want == nil {
				if err != nil {
					t.Errorf("validating a window whose radius is %d: %v", testCase.radius, err)
				}
				return
			}
			if !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// markii 形式のレコードは観測の種別を 2 欄で持つ。欄の名前と個数を受け入れる範囲は
// observation_kind_test.go が確かめる。
func TestMarkIIObservationKindCarriesBothColumns(t *testing.T) {
	withBoth := markIIObservationKind(t, "est")
	if err := withBoth.Validate(); err != nil {
		t.Fatalf("validating an observation kind with both fields: %v", err)
	}
	if len(withBoth.Raw) != 2 {
		t.Errorf("raw = %d fields, want 2", len(withBoth.Raw))
	}
	if withBoth.Raw[0].Name != "evt" || withBoth.Raw[1].Name != "subEvt" {
		t.Errorf("raw names = %q and %q, want evt and subEvt",
			withBoth.Raw[0].Name, withBoth.Raw[1].Name)
	}

	t.Run("a repeated name", func(t *testing.T) {
		repeated := core.ObservationKind{
			Raw: []core.RecordField{withBoth.Raw[0], withBoth.Raw[0]},
		}
		if err := repeated.Validate(); !errors.Is(err, core.ErrDuplicateElement) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrDuplicateElement)
		}
	})

	// Squid の combined は観測の種別の欄を持たない。欄が無い状態を determined と
	// undetermined のいずれかで表さない。
	t.Run("a format without the observation kind columns", func(t *testing.T) {
		none := core.ObservationKind{Raw: []core.RecordField{}}
		if err := none.Validate(); err != nil {
			t.Fatalf("validating an observation kind without fields: %v", err)
		}
		if none.Status != "" {
			t.Errorf("status = %q, want it absent", none.Status)
		}
	})

	t.Run("a status without any field", func(t *testing.T) {
		broken := core.ObservationKind{
			Raw:    []core.RecordField{},
			Status: core.ObservationKindStatusDetermined,
		}
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 片方の valueState が item_absent の状態では、2 つの文字列の組から意味を判定できない。
	t.Run("one field whose value is absent from the format", func(t *testing.T) {
		partial := core.ObservationKind{
			Raw: []core.RecordField{withBoth.Raw[0], absentItemText(t, "subEvt")},
		}
		if err := partial.Validate(); err != nil {
			t.Fatalf("validating a partially absent observation kind: %v", err)
		}

		partial.Status = core.ObservationKindStatusUndetermined
		if err := partial.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	// 観測の種別が確定しないレコードも取り込みには成功している。ImportFailure の
	// diagnosisClass の undetermined と、ObservationKind の status の undetermined は
	// 別のものを指す。
	t.Run("the two records of the contract", func(t *testing.T) {
		// 形式は evt が net のときの subEvt の est に意味を定めない。
		undetermined := markIIObservationKind(t, "est")
		if err := undetermined.Validate(); err != nil {
			t.Fatalf("validating the observation kind of an est record: %v", err)
		}
		if undetermined.Status != core.ObservationKindStatusUndetermined {
			t.Errorf("status of an est record = %q, want %q",
				undetermined.Status, core.ObservationKindStatusUndetermined)
		}

		// 形式は evt が net のときの subEvt の dcon に意味を定める。
		determined := markIIObservationKind(t, "dcon")
		if err := determined.Validate(); err != nil {
			t.Fatalf("validating the observation kind of a dcon record: %v", err)
		}
		if determined.Status != core.ObservationKindStatusDetermined {
			t.Errorf("status of a dcon record = %q, want %q",
				determined.Status, core.ObservationKindStatusDetermined)
		}
	})

	t.Run("a field carrying a time", func(t *testing.T) {
		timestampField, err := core.NewTimestampField(
			"evt", core.SemanticKeyEventCategory, markIIEventTime(t))
		if err != nil {
			t.Fatalf("building a timestamp field: %v", err)
		}
		broken := core.ObservationKind{
			Raw: []core.RecordField{timestampField, withBoth.Raw[1]},
		}
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 段階 1 は前提を 0 件で返す。要素数 0 の場合も集合である。
func TestTimeComparisonAcceptsNoAssumption(t *testing.T) {
	comparison := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitNotCompared,
		Assumptions:    []core.MatchAssumption{},
	}
	if err := comparison.Validate(); err != nil {
		t.Fatalf("validating a comparison without assumptions: %v", err)
	}
	if len(comparison.Assumptions) != 0 {
		t.Errorf("assumptions = %d, want 0", len(comparison.Assumptions))
	}
}

// 候補として数えるレコードを選ぶ観測の種別は、value の文字列を持つ。
func TestIncludedObservationKindsCarryTheValues(t *testing.T) {
	stage := clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, "")
	if err := stage.Validate(); err != nil {
		t.Fatalf("validating the stage: %v", err)
	}
	if !reflect.DeepEqual(stage.IncludedObservationKinds, includedObservationKinds()) {
		t.Fatalf("includedObservationKinds = %#v, want %#v",
			stage.IncludedObservationKinds, includedObservationKinds())
	}

	t.Run("an empty set", func(t *testing.T) {
		empty := stage
		empty.IncludedObservationKinds = []core.ObservationKindSelector{}
		if err := empty.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a selector without a value", func(t *testing.T) {
		broken := stage
		broken.IncludedObservationKinds = []core.ObservationKindSelector{
			{Items: []core.ObservationKindSelectorItem{{Name: "evt"}}}}
		if err := broken.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	// 候補の側の入力形式に観測の種別の欄が無い段階は出さない。同じ段階の候補も観測の種別を
	// 持たない。
	t.Run("a format without the observation kind columns", func(t *testing.T) {
		absent := stage
		absent.IncludedObservationKinds = nil
		member := notComparedCandidate(t, firstCandidateSn)
		member.ObservationKind = core.ObservationKind{Raw: []core.RecordField{}}
		absent.Members = []core.Candidate{member}
		if err := absent.Validate(); err != nil {
			t.Fatalf("validating a stage without the selectors: %v", err)
		}
	})

	// 選択の条件を持たない段階は、観測の種別を持つ候補を受け取る。絞り
	// (MatchRequest.selectByObservationKind) が通した候補を、段階の検査だけが除く形にしない。
	t.Run("a member carrying a kind while the stage selects none", func(t *testing.T) {
		accepted := stage
		accepted.IncludedObservationKinds = nil
		if err := accepted.Validate(); err != nil {
			t.Errorf("validating a stage without the selectors: %v", err)
		}
	})

	// 選択の条件に一致しない観測の種別の候補を段階に入れない。
	t.Run("a member outside the selectors", func(t *testing.T) {
		broken := stage
		broken.IncludedObservationKinds = []core.ObservationKindSelector{
			twoItemSelector("net", "dcon")}
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 絞りと段階の検査が同じ判定を使う。
//
// **選択子が 0 件の場合と 1 件以上の場合の両方を通す。** 片側だけを通すと、絞りを検査されずに
// 通った候補を段階の検査が除く組を見つけられない。
func TestCandidateStageAcceptsEveryMemberTheSelectionKeeps(t *testing.T) {
	// notComparedCandidate の観測の種別は evt=net subEvt=est である
	// (fixtures_test.go の markIICandidateObservationKind)。
	carriesKind := notComparedCandidate(t, firstCandidateSn)
	absent := notComparedCandidate(t, firstCandidateSn)
	absent.ObservationKind = core.ObservationKind{Raw: []core.RecordField{}}
	stage := clockIndependentStage(t, []core.Candidate{carriesKind}, "")

	cases := map[string]struct {
		selectors []core.ObservationKindSelector
		members   []core.Candidate
		accepted  bool
	}{
		"選択子が 0 件で、種別を持つ候補":   {nil, []core.Candidate{carriesKind}, true},
		"選択子が 0 件で、種別を持たない候補": {nil, []core.Candidate{absent}, true},
		"選択子が 1 件以上で、一致する候補": {
			[]core.ObservationKindSelector{twoItemSelector("net", "est")},
			[]core.Candidate{carriesKind}, true,
		},
		"選択子が 1 件以上で、種別を持たない候補": {
			[]core.ObservationKindSelector{twoItemSelector("net", "est")},
			[]core.Candidate{absent}, true,
		},
		"選択子が 1 件以上で、一致しない候補": {
			[]core.ObservationKindSelector{twoItemSelector("net", "dcon")},
			[]core.Candidate{carriesKind}, false,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			subject := stage
			subject.IncludedObservationKinds = want.selectors
			subject.Members = want.members
			err := subject.Validate()
			if want.accepted && err != nil {
				t.Errorf("Validate = %v, want no problem", err)
			}
			if !want.accepted && !errors.Is(err, core.ErrInconsistentValue) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
			}
		})
	}
}

// EmptyReason は定義の中の値だけを既知にする。
func TestEmptyReasonIsKnownSeparatesTheContractFromTheOthers(t *testing.T) {
	for _, reason := range []core.EmptyReason{
		core.EmptyReasonNoRecordInFilter, core.EmptyReasonNoSourceIngested,
		core.EmptyReasonNoCandidateInWindow, core.EmptyReasonNoCandidateMatchingConditions,
		core.EmptyReasonOriginOutsideAssignmentRange, core.EmptyReasonPublicationWithheld,
		core.EmptyReasonCounterpartItemAbsent, core.EmptyReasonNoValueMatch,
		core.EmptyReasonValueMatchOutsideFilter, core.EmptyReasonNoFieldObserved,
		core.EmptyReasonNoValueInFilter, core.EmptyReasonNoReadableValue,
		core.EmptyReasonFieldOnOtherNodeKind,
	} {
		if !reason.IsKnown() {
			t.Errorf("the empty reason %q is not known", reason)
		}
	}
	for _, reason := range []core.EmptyReason{"", "no_value", "value_match_filtered"} {
		if reason.IsKnown() {
			t.Errorf("the empty reason %q is known", reason)
		}
	}
}
