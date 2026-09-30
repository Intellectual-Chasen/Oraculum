package core_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// emptyReason は語彙が名前で挙げた値だけを受け取る。
func TestEmptyReasonAcceptsTheContractValues(t *testing.T) {
	known := []core.EmptyReason{
		"no_record_in_filter", "no_source_ingested", "no_candidate_in_window",
		"no_candidate_matching_conditions", "origin_outside_assignment_range",
		"publication_withheld", "counterpart_item_absent",
	}
	for _, reason := range known {
		if !reason.IsKnown() {
			t.Errorf("emptyReason %q must be one of the contract values", reason)
		}
	}
	unknown := []core.EmptyReason{"no_source", "no_candidate", "", "No_Record_In_Filter"}
	for _, reason := range unknown {
		if reason.IsKnown() {
			t.Errorf("emptyReason %q must not be accepted", reason)
		}
	}
}

// 段階ごとに置ける emptyReason の値が違う。
//
// 段階 2 だけが no_candidate_in_window を置ける。段階 1 の timeWindow の windowKind は
// not_compared であり、時刻を比べていない段階が「時刻の範囲の中に候補が 0 件」を名乗らない。
func TestStageEmptyReasonDependsOnTheStageKey(t *testing.T) {
	stageOneAccepted := []core.EmptyReason{
		core.EmptyReasonNoCandidateMatchingConditions,
		core.EmptyReasonCounterpartItemAbsent,
	}
	stageTwoAccepted := []core.EmptyReason{
		core.EmptyReasonNoCandidateInWindow,
		core.EmptyReasonNoCandidateMatchingConditions,
		core.EmptyReasonCounterpartItemAbsent,
	}

	for _, reason := range stageOneAccepted {
		stage := clockIndependentStage(t, []core.Candidate{}, reason)
		if err := stage.Validate(); err != nil {
			t.Errorf("validating a clock_independent stage with %q: %v", reason, err)
		}
	}
	for _, reason := range stageTwoAccepted {
		stage := secondTimeMatchedStage(t, []core.Candidate{}, reason)
		if err := stage.Validate(); err != nil {
			t.Errorf("validating a second_time_matched stage with %q: %v", reason, err)
		}
	}

	// 段階 1 に no_candidate_in_window を置かない。
	rejected := clockIndependentStage(t, []core.Candidate{}, core.EmptyReasonNoCandidateInWindow)
	if err := rejected.Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
	}
}

// 段階の conditions は種別が重ならない 1 件以上の条件を持つ。条件を 1 件足した段階を受理し、
// 1 件も持たない段階と、同じ種別を 2 回持つ段階を拒否する。
func TestStageCarriesTheDeclaredMatchConditions(t *testing.T) {
	stage := clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, "")
	if err := stage.Validate(); err != nil {
		t.Fatalf("validating a stage carrying every condition: %v", err)
	}

	// **段階 1 が用いる条件を文字列で挙げる。** 用いない条件を足しても、この一覧は変わらない。
	var used []string
	for _, condition := range stage.Conditions {
		if condition.Use == core.ConditionUseUsed {
			used = append(used, string(condition.ConditionKey))
		}
	}
	want := []string{"terminal_ip_assignment", "destination_ip", "destination_port"}
	if strings.Join(used, ",") != strings.Join(want, ",") {
		t.Errorf("the first stage uses the conditions %v, want %v", used, want)
	}

	// **条件を 1 件足した段階を受理する。** 段階が持つ条件の一覧は入力形式の組で変わり、
	// core は閉じた表を持たない (MatchConditionSpec)。
	t.Run("an added condition", func(t *testing.T) {
		long := stage
		long.Conditions = append(append([]core.MatchCondition{}, stage.Conditions...),
			core.MatchCondition{
				ConditionKey: core.ConditionKeyDestinationAuthority,
				Use:          core.ConditionUseItemAbsentOnCounterpart,
			})
		if err := long.Validate(); err != nil {
			t.Errorf("validating a stage carrying an added condition: %v", err)
		}
	})

	// 条件を 1 件も持たない段階を受理しない。
	t.Run("no condition", func(t *testing.T) {
		empty := stage
		empty.Conditions = []core.MatchCondition{}
		if err := empty.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	// 同じ conditionKey を 2 回持つ段階を受理しない。
	t.Run("a repeated conditionKey", func(t *testing.T) {
		repeated := stage
		repeated.Conditions = append(append([]core.MatchCondition{}, stage.Conditions...),
			stage.Conditions[len(stage.Conditions)-1])
		if err := repeated.Validate(); !errors.Is(err, core.ErrDuplicateElement) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrDuplicateElement)
		}
	})
}

// 段階が存在する応答の outsideAssignmentRange は偽である。
//
// 起点が期間の外にある応答は段階を 1 つも持たないため、真の値と候補が同じ応答に並ばない。
func TestStageRejectsAnOriginOutsideTheAssignmentRange(t *testing.T) {
	stage := clockIndependentStage(t, []core.Candidate{notComparedCandidate(t, firstCandidateSn)}, "")
	conditions := append([]core.MatchCondition{}, stage.Conditions...)
	outside := true
	conditions[0].OutsideAssignmentRange = &outside
	stage.Conditions = conditions
	if err := stage.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// プロセスの個数は候補の総数を超えず、候補がプロセスを持つときに出る。
func TestDistinctProcessCountFollowsTheMembers(t *testing.T) {
	member := notComparedCandidate(t, firstCandidateSn)
	member.ProcessRef = &core.ProcessRef{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		ProcessId:           markIIProcessId,
		TerminalId:          markIITerminalId,
	}
	stage := clockIndependentStage(t, []core.Candidate{member}, "")

	t.Run("a member with a process and no count", func(t *testing.T) {
		if err := stage.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("a count of one", func(t *testing.T) {
		counted := stage
		count := int64(1)
		counted.DistinctProcessCount = &count
		if err := counted.Validate(); err != nil {
			t.Fatalf("validating a stage carrying a process count: %v", err)
		}
	})

	t.Run("a count above the member total", func(t *testing.T) {
		broken := stage
		count := int64(2)
		broken.DistinctProcessCount = &count
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a count of zero next to a member with a process", func(t *testing.T) {
		broken := stage
		count := int64(0)
		broken.DistinctProcessCount = &count
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 失敗したレコードに依拠する候補がある候補集合は、未処理の範囲を 1 件以上持つ。
func TestCandidateSetLinksAFailedDependencyToTheUnprocessedRanges(t *testing.T) {
	member := notComparedCandidate(t, firstCandidateSn)
	member.FailedRecordDependency = true
	set := candidateSet(t, []core.CandidateStage{
		clockIndependentStage(t, []core.Candidate{member}, ""),
	}, "")
	if err := set.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	set.UnprocessedRanges = []core.RecordRange{
		markIIRange(rangeFromSequenceNumber, rangeFromSequenceNumber+10),
	}
	set.UnprocessedRangeCount = 1
	set.PublicationState = core.PublicationStatePublishedPartial
	if err := set.Validate(); err != nil {
		t.Fatalf("validating a candidate set with an unprocessed range: %v", err)
	}
}

// not_compared の比較は前提を要素数 0 で返す。要素数 1 以上を拒否する。
func TestNotComparedComparisonCarriesNoAssumption(t *testing.T) {
	withoutAssumption := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitNotCompared,
		Assumptions:    []core.MatchAssumption{},
	}
	if err := withoutAssumption.Validate(); err != nil {
		t.Fatalf("validating a comparison without assumptions: %v", err)
	}

	withAssumption := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitNotCompared,
		Assumptions:    []core.MatchAssumption{clockOffsetAssumption()},
	}
	if err := withAssumption.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}

	// 秒単位で比べた比較は前提を 1 件以上持てる。
	leftTime := squidRequestTime(t)
	rightTime := markIIEventTime(t)
	compared := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		LeftTime:       &leftTime,
		RightTime:      &rightTime,
		Assumptions:    []core.MatchAssumption{clockOffsetAssumption()},
	}
	if err := compared.Validate(); err != nil {
		t.Fatalf("validating a comparison with an assumption: %v", err)
	}
}

// 比べる 2 つの時刻の normalizedForm はどちらも rfc3339_absolute である。
func TestComparedTimesAreAbsolute(t *testing.T) {
	leftTime := markIIStartTime(t)
	rightTime := markIIEventTime(t)
	comparison := core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		LeftTime:       &leftTime,
		RightTime:      &rightTime,
		Assumptions:    []core.MatchAssumption{clockOffsetAssumption()},
	}
	if err := comparison.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 要素数 0 の場合も集合である必須の集合は、nil でも [] として出る。
//
// 対象の型と項目名は下の cases が名前で挙げる。
func TestRequiredSetsSerializeAsAnEmptyArray(t *testing.T) {
	cases := []struct {
		name  string
		value any
		items []string
	}{
		{
			name:  "TimeComparison",
			value: core.TimeComparison{ComparisonUnit: core.ComparisonUnitNotCompared},
			items: []string{"assumptions"},
		},
		{
			name:  "ObservationKind",
			value: core.ObservationKind{},
			items: []string{"raw"},
		},
		{
			name:  "CandidateStage",
			value: core.CandidateStage{},
			items: []string{"assumptions", "members", "indistinguishableGroups"},
		},
		{
			name:  "CandidateSet",
			value: core.CandidateSet{},
			items: []string{"stages", "unprocessedRanges"},
		},
		{
			name:  "ImportStatus",
			value: core.ImportStatus{},
			items: []string{"counts", "diagnosisCounts", "failures"},
		},
		{
			name:  "EdgeMatch",
			value: core.EdgeMatch{},
			items: []string{"assumptions", "indistinguishableGroups"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.value)
			if err != nil {
				t.Fatalf("marshaling %s: %v", testCase.name, err)
			}
			// 項目を挙げていない case は何も確かめない。
			if len(testCase.items) == 0 {
				t.Fatalf("the case %s names no required set", testCase.name)
			}
			for _, item := range testCase.items {
				want := `"` + item + `":[]`
				if !strings.Contains(string(encoded), want) {
					t.Errorf("json = %s, want it to contain %s", encoded, want)
				}
			}
		})
	}

	// 同じ package の ImportCountSet も要素数 0 の集合を [] として出す。
	counts, err := core.NewImportCountSet()
	if err != nil {
		t.Fatalf("building an empty count set: %v", err)
	}
	encoded, err := json.Marshal(counts)
	if err != nil {
		t.Fatalf("marshaling the count set: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("json = %s, want []", encoded)
	}
}

// 左右の集合の要素数の上限は 3 である。
func TestMatchConditionSideValuesStopAtThree(t *testing.T) {
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyDestinationIp,
		Use:          core.ConditionUseUsed,
		LeftValue: []core.RecordField{
			presentText(t, "dstIP", proxyTargetIp),
			presentText(t, "dstHost", markIITerminalName),
			presentText(t, "dstPort", "80"),
		},
	}
	if err := condition.Validate(); err != nil {
		t.Fatalf("validating the destination condition: %v", err)
	}

	t.Run("a fourth element", func(t *testing.T) {
		broken := condition
		broken.LeftValue = append(append([]core.RecordField{}, condition.LeftValue...),
			presentText(t, "hostName", markIITerminalName))
		if err := broken.Validate(); !errors.Is(err, core.ErrTooManyElements) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrTooManyElements)
		}
	})

	t.Run("a repeated name", func(t *testing.T) {
		broken := condition
		broken.LeftValue = []core.RecordField{
			presentText(t, "dstIP", proxyTargetIp),
			presentText(t, "dstIP", proxyTargetIp),
			presentText(t, "dstPort", "80"),
		}
		if err := broken.Validate(); !errors.Is(err, core.ErrDuplicateElement) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrDuplicateElement)
		}
	})
}

// 端末の同一性は語彙の terminal.id を持つ 1 項目で判定する。
//
// **入力形式ごとの key の文字列で判定しない。** 端末の表示名は一意である根拠を持たず、
// 条件に入れると同じ表示名を持つ別の端末が 1 つにまとめられる。
func TestTerminalIdentityConditionComparesTheTerminalIdSemantic(t *testing.T) {
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyTerminalIdentityMatches,
		Use:          core.ConditionUseUsed,
		LeftValue: []core.RecordField{
			semanticText(t, "tmid", core.SemanticKeyTerminalId, markIITerminalId),
		},
		RightValue: []core.RecordField{
			semanticText(t, "tmid", core.SemanticKeyTerminalId, markIITerminalId),
		},
	}
	if err := condition.Validate(); err != nil {
		t.Fatalf("validating the terminal identity condition: %v", err)
	}

	t.Run("a second element", func(t *testing.T) {
		broken := condition
		broken.LeftValue = []core.RecordField{
			semanticText(t, "tmid", core.SemanticKeyTerminalId, markIITerminalId),
			semanticText(t, "com", core.SemanticKeyTerminalHostname, markIITerminalName),
		}
		broken.RightValue = nil
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("the terminal hostname instead of the identifier", func(t *testing.T) {
		broken := condition
		broken.LeftValue = []core.RecordField{
			semanticText(t, "com", core.SemanticKeyTerminalHostname, markIITerminalName),
		}
		broken.RightValue = nil
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("a field outside the vocabulary", func(t *testing.T) {
		broken := condition
		broken.LeftValue = []core.RecordField{presentText(t, "tmid", markIITerminalId)}
		broken.RightValue = nil
		if err := broken.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 候補の側の値を禁じる条件は 2 つだけである。
//
// 1 つは時刻の条件 (second_of_time / sub_second_of_time)、もう 1 つは memberCount が
// 0 の段階である。
// use が used 以外であることを理由に禁じない。
func TestRightValueIsForbiddenOnlyWhereTheContractSaysSo(t *testing.T) {
	// use が item_absent_on_counterpart の条件が候補の側の値を持つ形は通る。
	// 左右の要素数を揃え、要素数の不一致で失敗しないようにする。
	absentOnCounterpart := core.MatchCondition{
		ConditionKey: core.ConditionKeyClientPort,
		Use:          core.ConditionUseItemAbsentOnCounterpart,
		LeftValue:    []core.RecordField{presentText(t, "sPort", "51234")},
		RightValue:   []core.RecordField{presentText(t, "srcPort", "51234")},
	}
	if err := absentOnCounterpart.Validate(); err != nil {
		t.Fatalf("validating a condition whose counterpart lacks the item: %v", err)
	}

	// 時刻の条件が候補の側の値を持つ形は検査で失敗する。
	timeCondition := core.MatchCondition{
		ConditionKey: core.ConditionKeySecondOfTime,
		Use:          core.ConditionUseUsed,
		LeftValue:    []core.RecordField{presentText(t, "requestTime", "10:20:30")},
		RightValue:   []core.RecordField{presentText(t, "eventTime", "10:20:30")},
	}
	if err := timeCondition.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}
}
