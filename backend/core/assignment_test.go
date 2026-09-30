package core_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// markIIAssignment は 1 台目の端末の観測期間を割当の適用期間として返す。
func markIIAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	return core.TerminalAssignment{
		ClientIp:            markIIClientIp,
		TerminalId:          markIITerminalId,
		TerminalHostname:    markIITerminalName,
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		AssignmentValidRange: core.TimeRange{
			From: clockTime(t, markIIObservedRangeFirst, core.ClockTerminalLocal),
			To:   clockTime(t, markIIObservedRangeLast, core.ClockTerminalLocal),
		},
		Origin: core.TerminalAssignmentOriginObservedInSource,
	}
}

// analystAssignment は分析者が別の収集元のレコードを根拠に与えた割当を返す。
func analystAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	assignment := markIIAssignment(t)
	assignment.Origin = core.TerminalAssignmentOriginAnalystSupplied
	assignment.Derivation = "別の端末の ssh の接続先から導いた"
	assignment.Author = "analyst"
	assignment.BasisRecordRefs = []core.AssertionRecordRef{
		core.NewAssertionRecordRef(squidLocator(squidLineNumber)),
	}
	return assignment
}

// secondMarkIIAssignment は 2 台目の端末の観測期間を、同じ接続元 IP の割当として返す。
// 2 つの期間は重なる。**表示名は 1 台目と同じである。** 表示名が同じ 2 台を、外部識別子で
// 別の割当として保つ。
func secondMarkIIAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	return core.TerminalAssignment{
		ClientIp:            markIIClientIp,
		TerminalId:          "66666666-7777-8888-9999-aaaaaaaaaaaa",
		TerminalHostname:    markIITerminalName,
		SourceId:            "source-mark-ii-second",
		SourceContentSha256: markIISha256,
		AssignmentValidRange: core.TimeRange{
			From: clockTime(t, "2024-03-14T09:00:30.200+09:00", core.ClockTerminalLocal),
			To:   clockTime(t, "2024-03-14T11:01:00.400+09:00", core.ClockTerminalLocal),
		},
		Origin: core.TerminalAssignmentOriginObservedInSource,
	}
}

// importAssignment は利用者が取り込みの起動で収集元に指定した割当を返す。
func importAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	assignment := markIIAssignment(t)
	assignment.Origin = core.TerminalAssignmentOriginImportSpecified
	assignment.AppliesToSourceId = assignment.SourceId
	return assignment
}

// どの由来の割当も、それ 1 件で端末を確定させる。
func TestResolveTerminalDeterminesTheTerminalWithOneAssignmentOfAnyOrigin(t *testing.T) {
	inRange := proxyTime(t, "2024-03-14T10:00:00.000+09:00")
	cases := map[string]core.TerminalAssignment{
		"分析者が画面から記録した割当":  analystAssignment(t),
		"取り込みの起動で指定した割当":  importAssignment(t),
		"収集元のレコードが記録した割当": markIIAssignment(t),
	}
	for name, assignment := range cases {
		t.Run(name, func(t *testing.T) {
			resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
				ClientIp: markIIClientIp, EventTime: inRange,
				Assignments: []core.TerminalAssignment{assignment},
			})
			if err != nil {
				t.Fatalf("resolving the terminal: %v", err)
			}
			if resolution.AssignmentState != core.TerminalAssignmentStateDetermined {
				t.Errorf("the state is %q, want %q", resolution.AssignmentState,
					core.TerminalAssignmentStateDetermined)
			}
			if len(resolution.Members) != 1 || resolution.Members[0].Origin != assignment.Origin {
				t.Errorf("the members are %+v, want the one assignment of the origin %q",
					resolution.Members, assignment.Origin)
			}
			if len(resolution.UnresolvedReasons) != 0 {
				t.Errorf("the resolution carries the reasons %v, want none",
					resolution.UnresolvedReasons)
			}
			if err := resolution.Validate(); err != nil {
				t.Errorf("the resolution does not validate: %v", err)
			}
		})
	}
}

// 利用者が入力した割当は、接続元 IP、端末の外部識別子、端末の表示名のうち、分かるものだけを持つ。
func TestUserSuppliedTerminalAssignmentCarriesAnySubsetOfTheTerminalItems(t *testing.T) {
	keep := func(ip, id, hostname bool) func(core.TerminalAssignment) core.TerminalAssignment {
		return func(a core.TerminalAssignment) core.TerminalAssignment {
			if !ip {
				a.ClientIp = ""
				a.AppliesToSourceId = a.SourceId
			}
			if !id {
				a.TerminalId = ""
				a.AppliesToSourceId = a.SourceId
			}
			if !hostname {
				a.TerminalHostname = ""
			}
			return a
		}
	}
	accepted := map[string]func(core.TerminalAssignment) core.TerminalAssignment{
		"3 項目":       keep(true, true, true),
		"接続元 IP だけ":  keep(true, false, false),
		"端末の外部識別子だけ": keep(false, true, false),
		"端末の表示名だけ":   keep(false, false, true),
	}
	origins := map[string]func(*testing.T) core.TerminalAssignment{
		"取り込みの指定": importAssignment, "画面から記録": analystAssignment,
	}
	for originName, originOf := range origins {
		for name, shape := range accepted {
			t.Run(originName+"/"+name, func(t *testing.T) {
				if err := shape(originOf(t)).Validate(); err != nil {
					t.Errorf("Validate rejected the assignment: %v", err)
				}
			})
		}
		t.Run(originName+"/3 項目をすべて欠く", func(t *testing.T) {
			none := keep(false, false, false)(originOf(t))
			if err := none.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
			}
		})
		t.Run(originName+"/IP アドレスとして読めない接続元 IP", func(t *testing.T) {
			assignment := originOf(t)
			assignment.ClientIp = "198.51.100.999"
			if err := assignment.Validate(); !errors.Is(err, core.ErrInvalid) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
			}
		})
		t.Run(originName+"/空白だけの表示名", func(t *testing.T) {
			assignment := originOf(t)
			assignment.TerminalHostname = "  "
			if err := assignment.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
			}
		})
	}

	t.Run("収集元のレコードが記録した割当は 3 項目を欠かない", func(t *testing.T) {
		observed := markIIAssignment(t)
		observed.TerminalHostname = ""
		if err := observed.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})
	t.Run("接続元 IP も付ける収集元もホスト名も持たない割当", func(t *testing.T) {
		assignment := analystAssignment(t)
		assignment.ClientIp, assignment.TerminalHostname = "", ""
		if err := assignment.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})
	// 接続元 IP を持たない端末にも、ホスト名を名乗るレコードと引数を結ぶ名前を足せる。
	t.Run("端末の外部識別子とホスト名だけの割当", func(t *testing.T) {
		for _, hostnames := range [][]string{nil, {"ws-01.example.test"}} {
			assignment := analystAssignment(t)
			assignment.ClientIp, assignment.TerminalHostnames = "", hostnames
			if err := assignment.Validate(); err != nil {
				t.Errorf("hostnames %v: Validate rejected the assignment: %v", hostnames, err)
			}
		}
	})
	t.Run("端末の外部識別子を欠く割当は期間の収集元に付ける", func(t *testing.T) {
		assignment := keep(true, false, true)(analystAssignment(t))
		assignment.AppliesToSourceId = ""
		if err := assignment.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
}

// 取り込みの指定は収集元 1 件に付き、分析者の欄を持たない。
func TestImportSpecifiedTerminalAssignmentAppliesToItsSource(t *testing.T) {
	t.Run("期間の収集元と端末を付ける収集元が異なる", func(t *testing.T) {
		assignment := importAssignment(t)
		assignment.AppliesToSourceId = "another-source"
		if err := assignment.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})
	t.Run("分析者の根拠のレコードを持つ", func(t *testing.T) {
		assignment := importAssignment(t)
		assignment.BasisRecordRefs = analystAssignment(t).BasisRecordRefs
		if err := assignment.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})
	t.Run("導いた筋道を持つ", func(t *testing.T) {
		assignment := importAssignment(t)
		assignment.Derivation = "分析者の説明"
		if err := assignment.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})
}

// 割当が指す端末のノードは、外部識別子があればその鍵、無ければ期間の収集元を記録した端末の鍵である。
func TestTerminalAssignmentNodeKey(t *testing.T) {
	withId := importAssignment(t)
	key, built := withId.TerminalNodeKey()
	want, _ := core.TerminalNodeKey(markIITerminalId)
	if !built || !reflect.DeepEqual(key, want) {
		t.Errorf("the key of an assignment with the identifier is %+v (built %v), want %+v",
			key, built, want)
	}

	withoutId := importAssignment(t)
	withoutId.TerminalId = ""
	key, built = withoutId.TerminalNodeKey()
	want, _ = core.RecordingTerminalNodeKey(markIISha256)
	if !built || !reflect.DeepEqual(key, want) {
		t.Errorf("the key of an assignment without the identifier is %+v (built %v), want %+v",
			key, built, want)
	}
	if key.Form != core.NodeKeyFormRecordingSource {
		t.Errorf("the form is %q, want %q", key.Form, core.NodeKeyFormRecordingSource)
	}

	ipOnly := markIIAssignment(t)
	ipOnly.TerminalId = ""
	if _, built := ipOnly.TerminalNodeKey(); built {
		t.Error("an assignment without the identifier and the source to apply to built a key")
	}
}

// 分析者が与えた割当は、導いた筋道と根拠のレコードを必ず持つ。
func TestTerminalAssignmentRequiresTheBasisOfAnAnalystSuppliedOne(t *testing.T) {
	cases := map[string]func(core.TerminalAssignment) core.TerminalAssignment{
		"導いた筋道が無い": func(a core.TerminalAssignment) core.TerminalAssignment {
			a.Derivation = ""
			return a
		},
		"導いた筋道が空白だけ": func(a core.TerminalAssignment) core.TerminalAssignment {
			a.Derivation = "   "
			return a
		},
		"根拠のレコードが無い": func(a core.TerminalAssignment) core.TerminalAssignment {
			a.BasisRecordRefs = nil
			return a
		},
		"記録した分析者が無い": func(a core.TerminalAssignment) core.TerminalAssignment {
			a.Author = ""
			return a
		},
	}
	for name, without := range cases {
		t.Run(name, func(t *testing.T) {
			if err := without(analystAssignment(t)).Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
			}
		})
	}

	t.Run("由来を宣言しない割当", func(t *testing.T) {
		assignment := markIIAssignment(t)
		assignment.Origin = ""
		if err := assignment.Validate(); !errors.Is(err, core.ErrUnknownEnumValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnknownEnumValue)
		}
	})

	t.Run("収集元が記録した割当に分析者の欄を入れない", func(t *testing.T) {
		assignment := markIIAssignment(t)
		assignment.Derivation = "分析者の説明"
		if err := assignment.Validate(); err == nil {
			t.Error("Validate accepted a derivation on an observed assignment, want a rejection")
		}
	})

	t.Run("収集元が記録した割当に分析者の根拠のレコードを入れない", func(t *testing.T) {
		assignment := markIIAssignment(t)
		assignment.BasisRecordRefs = analystAssignment(t).BasisRecordRefs
		if err := assignment.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
		}
	})
}

// proxyTime は Proxy の時計が刻んだ時刻を返す。
func proxyTime(t *testing.T, normalized string) core.Timestamp {
	t.Helper()
	return clockTime(t, normalized, core.ClockObserverLocal)
}

func TestResolveTerminalOverTheAssignmentRange(t *testing.T) {
	cases := map[string]struct {
		eventTime         string
		wantState         core.TerminalAssignmentState
		wantMemberCount   int64
		wantUnresolved    []core.TerminalUnresolvedReason
		wantTerminalIndex int
	}{
		"inside the range": {
			eventTime:       "2024-03-14T10:20:30.000+09:00",
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"outside the range": {
			eventTime:       "2024-03-14T12:00:00.000+09:00",
			wantState:       core.TerminalAssignmentStateOriginOutsideAssignmentRange,
			wantMemberCount: 0,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"on the lower endpoint": {
			eventTime:       markIIObservedRangeFirst,
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"on the upper endpoint": {
			eventTime:       markIIObservedRangeLast,
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		// 比較の単位は秒である。境界の秒に並ぶレコードは期間の中として扱う。
		"one millisecond after the upper endpoint": {
			eventTime:       "2024-03-14T11:00:00.901+09:00",
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"the last millisecond of the upper endpoint second": {
			eventTime:       "2024-03-14T11:00:00.999+09:00",
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"one second after the upper endpoint": {
			eventTime:       "2024-03-14T11:00:01.000+09:00",
			wantState:       core.TerminalAssignmentStateOriginOutsideAssignmentRange,
			wantMemberCount: 0,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"one millisecond before the lower endpoint": {
			eventTime:       "2024-03-14T09:00:00.099+09:00",
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"the first millisecond of the lower endpoint second": {
			eventTime:       "2024-03-14T09:00:00.000+09:00",
			wantState:       core.TerminalAssignmentStateDetermined,
			wantMemberCount: 1,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
		"one second before the lower endpoint": {
			eventTime:       "2024-03-14T08:59:59.999+09:00",
			wantState:       core.TerminalAssignmentStateOriginOutsideAssignmentRange,
			wantMemberCount: 0,
			wantUnresolved:  []core.TerminalUnresolvedReason{},
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
				ClientIp:    markIIClientIp,
				EventTime:   proxyTime(t, testCase.eventTime),
				Assignments: []core.TerminalAssignment{markIIAssignment(t)},
				Assumptions: []core.MatchAssumption{clockOffsetAssumption()},
			})
			if err != nil {
				t.Fatalf("resolving the terminal: %v", err)
			}
			if err := resolution.Validate(); err != nil {
				t.Fatalf("validating the resolution: %v", err)
			}
			if resolution.AssignmentState != testCase.wantState {
				t.Errorf("assignmentState = %q, want %q",
					resolution.AssignmentState, testCase.wantState)
			}
			if resolution.MemberCount != testCase.wantMemberCount {
				t.Errorf("memberCount = %d, want %d",
					resolution.MemberCount, testCase.wantMemberCount)
			}
			if len(resolution.UnresolvedReasons) != len(testCase.wantUnresolved) {
				t.Errorf("unresolvedReasons = %v, want %v",
					resolution.UnresolvedReasons, testCase.wantUnresolved)
			}
			if testCase.wantMemberCount == 1 {
				if resolution.Members[0].TerminalId != markIITerminalId {
					t.Errorf("terminalId = %q, want %q", resolution.Members[0].TerminalId,
						markIITerminalId)
				}
				if !resolution.Determined() {
					t.Error("a single member must be reported as determined")
				}
			}
		})
	}
}

// 期間の重なる割当が同じ端末を指すときは、端末を確定させ、割当をすべて Members に残す。
// 取り込みの起動で収集元ごとに同じ端末と IP を指定した状態である。
func TestResolveTerminalDeterminesOverlappingAssignmentsOfOneTerminal(t *testing.T) {
	second := importAssignment(t)
	second.SourceId, second.AppliesToSourceId = "source-mark-ii-second", "source-mark-ii-second"
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:    markIIClientIp,
		EventTime:   proxyTime(t, "2024-03-14T10:00:00.000+09:00"),
		Assignments: []core.TerminalAssignment{importAssignment(t), second},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if err := resolution.Validate(); err != nil {
		t.Fatalf("validating the resolution: %v", err)
	}
	if !resolution.Determined() {
		t.Fatalf("assignmentState = %q, reasons = %v, want determined",
			resolution.AssignmentState, resolution.UnresolvedReasons)
	}
	if resolution.MemberCount != 2 {
		t.Errorf("memberCount = %d, want 2", resolution.MemberCount)
	}
}

// 割当の期間が重なるときは 1 つに確定させず、重なった割当をすべて残す。
func TestResolveTerminalKeepsOverlappingAssignments(t *testing.T) {
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:  markIIClientIp,
		EventTime: proxyTime(t, "2024-03-14T10:20:30.000+09:00"),
		Assignments: []core.TerminalAssignment{
			markIIAssignment(t),
			secondMarkIIAssignment(t),
		},
		Assumptions: []core.MatchAssumption{clockOffsetAssumption()},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if err := resolution.Validate(); err != nil {
		t.Fatalf("validating the resolution: %v", err)
	}
	if resolution.AssignmentState != core.TerminalAssignmentStateUndetermined {
		t.Errorf("assignmentState = %q, want %q",
			resolution.AssignmentState, core.TerminalAssignmentStateUndetermined)
	}
	if resolution.Determined() {
		t.Error("overlapping assignments must not be reported as determined")
	}
	if resolution.MemberCount != 2 {
		t.Errorf("memberCount = %d, want 2", resolution.MemberCount)
	}
	if len(resolution.UnresolvedReasons) != 1 ||
		resolution.UnresolvedReasons[0] != core.TerminalUnresolvedReasonOverlappingAssignments {
		t.Errorf("unresolvedReasons = %v, want [%q]",
			resolution.UnresolvedReasons, core.TerminalUnresolvedReasonOverlappingAssignments)
	}
}

// 時刻を比べられないときは端末を確定させない。
func TestResolveTerminalReportsATimeItCannotCompare(t *testing.T) {
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:    markIIClientIp,
		EventTime:   markIIStartTime(t),
		Assignments: []core.TerminalAssignment{markIIAssignment(t)},
		Assumptions: []core.MatchAssumption{clockOffsetAssumption()},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if resolution.AssignmentState != core.TerminalAssignmentStateUndetermined {
		t.Errorf("assignmentState = %q, want %q",
			resolution.AssignmentState, core.TerminalAssignmentStateUndetermined)
	}
	if len(resolution.UnresolvedReasons) != 1 ||
		resolution.UnresolvedReasons[0] != core.TerminalUnresolvedReasonTimeNotComparable {
		t.Errorf("unresolvedReasons = %v, want [%q]",
			resolution.UnresolvedReasons, core.TerminalUnresolvedReasonTimeNotComparable)
	}
}

// 接続元 IP の割当が 1 件も無いときは端末を確定させない。
func TestResolveTerminalReportsAMissingAssignment(t *testing.T) {
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:    "192.0.2.109",
		EventTime:   proxyTime(t, "2024-03-14T10:20:30.000+09:00"),
		Assignments: []core.TerminalAssignment{markIIAssignment(t)},
		Assumptions: []core.MatchAssumption{clockOffsetAssumption()},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if resolution.AssignmentState != core.TerminalAssignmentStateUndetermined {
		t.Errorf("assignmentState = %q, want %q",
			resolution.AssignmentState, core.TerminalAssignmentStateUndetermined)
	}
	if len(resolution.UnresolvedReasons) != 1 ||
		resolution.UnresolvedReasons[0] != core.TerminalUnresolvedReasonNoAssignmentForClientIp {
		t.Errorf("unresolvedReasons = %v, want [%q]",
			resolution.UnresolvedReasons, core.TerminalUnresolvedReasonNoAssignmentForClientIp)
	}
}

// 判定は入力の前提を結果に残し、2 つの時計を比べたことを結果に含める。
func TestResolveTerminalCarriesTheAssumptionsAndTheClocks(t *testing.T) {
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:    markIIClientIp,
		EventTime:   proxyTime(t, "2024-03-14T10:20:30.000+09:00"),
		Assignments: []core.TerminalAssignment{markIIAssignment(t)},
		Assumptions: []core.MatchAssumption{clockOffsetAssumption()},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if len(resolution.Assumptions) != 1 {
		t.Fatalf("assumptions = %d, want 1", len(resolution.Assumptions))
	}
	if resolution.Assumptions[0].AssumptionKey != core.AssumptionKeyClockOffsetBelowOneSecond {
		t.Errorf("assumptionKey = %q, want %q",
			resolution.Assumptions[0].AssumptionKey, core.AssumptionKeyClockOffsetBelowOneSecond)
	}
	if resolution.Assumptions[0].EvidenceClass != core.EvidenceClassUnconfirmed {
		t.Errorf("evidenceClass = %q, want %q",
			resolution.Assumptions[0].EvidenceClass, core.EvidenceClassUnconfirmed)
	}
	if resolution.ClockDependency == nil {
		t.Fatal("comparing a proxy time against a client observation range depends on two clocks")
	}
	if resolution.ClockDependency.LeftClock != core.ClockObserverLocal {
		t.Errorf("leftClock = %q, want %q",
			resolution.ClockDependency.LeftClock, core.ClockObserverLocal)
	}
	if resolution.ClockDependency.RightClock != core.ClockTerminalLocal {
		t.Errorf("rightClock = %q, want %q",
			resolution.ClockDependency.RightClock, core.ClockTerminalLocal)
	}
}

// 1 つの時計の中で比べた判定は、時計の依拠を結果に持たない。
func TestResolveTerminalWithinOneClockHasNoClockDependency(t *testing.T) {
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp:    markIIClientIp,
		EventTime:   clockTime(t, "2024-03-14T10:20:30.000+09:00", core.ClockTerminalLocal),
		Assignments: []core.TerminalAssignment{markIIAssignment(t)},
		Assumptions: []core.MatchAssumption{},
	})
	if err != nil {
		t.Fatalf("resolving the terminal: %v", err)
	}
	if resolution.ClockDependency != nil {
		t.Errorf("clockDependency = %v, want none", resolution.ClockDependency)
	}
}

func TestResolveTerminalRejectsABrokenInput(t *testing.T) {
	if _, err := core.ResolveTerminal(core.TerminalResolutionInput{
		EventTime:   proxyTime(t, "2024-03-14T10:20:30.000+09:00"),
		Assignments: []core.TerminalAssignment{markIIAssignment(t)},
	}); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}
}

// 割当が FQDN を持つときは名前の全体で、短い名前だけのときはホスト名の最初の `.` の前で比べる。
func TestTerminalAssignmentNamesHostname(t *testing.T) {
	withFqdn := analystAssignment(t)
	withFqdn.TerminalHostname = "Display"
	withFqdn.TerminalHostnames = []string{"host-a", "host-a.example.test"}
	shortOnly := analystAssignment(t)
	shortOnly.TerminalHostname = ""
	shortOnly.TerminalHostnames = []string{"HOST-A"}
	for _, check := range []struct {
		assignment core.TerminalAssignment
		hostname   string
		want       bool
	}{
		{withFqdn, "HOST-A.EXAMPLE.TEST", true},
		{withFqdn, "host-a", true},
		{withFqdn, "display", true},
		{withFqdn, "host-a.other.example.test", false},
		{shortOnly, "host-a.example.test", true},
		{shortOnly, "host-a", true},
		{shortOnly, "host-b.example.test", false},
		{shortOnly, "", false},
	} {
		if got := check.assignment.NamesHostname(check.hostname); got != check.want {
			t.Errorf("NamesHostname(%q) of %q = %v, want %v",
				check.hostname, check.assignment.TerminalHostnames, got, check.want)
		}
	}
}

// ホスト名の並びは利用者が入力した割当だけが持ち、空白を含む名前を退ける。
func TestTerminalAssignmentValidatesItsHostnames(t *testing.T) {
	supplied := analystAssignment(t)
	supplied.TerminalHostnames = []string{"host-a", "host-a.example.test"}
	if err := supplied.Validate(); err != nil {
		t.Errorf("the hostnames were rejected: %v", err)
	}
	supplied.TerminalHostnames = []string{"host a"}
	if err := supplied.Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInvalid)
	}
	observed := markIIAssignment(t)
	observed.TerminalHostnames = []string{"host-a"}
	if err := observed.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}
}
