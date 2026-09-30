// in-package test: 候補の側の欄の集合と時刻の精度をまとめる非公開の関数を直接叩く。
package pipeline

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 相手の時刻の精度は、候補の収集元のうち最も粗いものを採る。
//
// **空の文字列を未初期化の目印にしない。** 精度そのものが空を取りうるため、先頭の収集元が
// 空の精度を持つ組で、後続の既知の精度に上書きされる。
func TestCoarsestTimePrecisionKeepsTheCoarsestRegardlessOfOrder(t *testing.T) {
	cases := map[string]struct {
		sides []core.Precision
		want  core.Precision
	}{
		"候補が 0 件": {nil, core.Precision("")},
		"秒とミリ秒": {
			[]core.Precision{core.PrecisionSecond, core.PrecisionMillisecond},
			core.PrecisionSecond,
		},
		"ミリ秒と秒": {
			[]core.Precision{core.PrecisionMillisecond, core.PrecisionSecond},
			core.PrecisionSecond,
		},
		"未確定が先頭": {
			[]core.Precision{core.Precision(""), core.PrecisionMillisecond},
			core.Precision(""),
		},
		"未確定が後続": {
			[]core.Precision{core.PrecisionMillisecond, core.Precision("")},
			core.Precision(""),
		},
		"ミリ秒だけ": {
			[]core.Precision{core.PrecisionMillisecond}, core.PrecisionMillisecond,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			sides := make([]matchSide, 0, len(want.sides))
			for _, precision := range want.sides {
				sides = append(sides, matchSide{timePrecision: precision})
			}
			if got := coarsestTimePrecision(sides); got != want.want {
				t.Errorf("coarsestTimePrecision = %q, want %q", got, want.want)
			}
		})
	}
}

// 相手の欄の集合は、候補の収集元の和集合である。
//
// **欄を持つ収集元が 1 つでもあれば「相手の入力形式に欄が無い」とは言えない。**
func TestUnionItemSemanticsCollectsEverySource(t *testing.T) {
	sides := []matchSide{
		{itemSemantics: []core.SemanticKey{
			core.SemanticKeyConnectionDestinationPort, core.SemanticKeyEventTime,
		}},
		{itemSemantics: []core.SemanticKey{
			core.SemanticKeyEventTime, core.SemanticKeyConnectionSourcePort,
		}},
	}
	got := unionItemSemantics(sides)
	want := []core.SemanticKey{
		core.SemanticKeyConnectionDestinationPort,
		core.SemanticKeyEventTime,
		core.SemanticKeyConnectionSourcePort,
	}
	for _, semantic := range want {
		if !slices.Contains(got, semantic) {
			t.Errorf("the union %v omits %q", got, semantic)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the union is %v, want the %d distinct semantics without a repeat",
			got, len(want))
	}
}

// squidOriginSide は Squid の収集元のレコードを起点に置いた側を返す。
//
// 段階の条件の一覧と並びを決めるのは起点の側の収集元の宣言である
// (ParserIdentity.ConnectionMatchConditions)。
func squidOriginSide() matchSide {
	return matchSide{matchConditions: squidConnectionMatchConditions()}
}

// markIICounterpart は markii 形式の収集元を候補の側に置いた宣言を返す。
func markIICounterpart() sideDeclarations {
	declarations := newSideDeclarations()
	declarations.add(SourcePublication{
		status: core.ImportStatus{SourceId: "src-counterpart"},
		parser: ParserIdentity{
			ConnectionMatchConditions: markiiConnectionMatchConditions(),
			ConnectionRequestKinds:    markiiConnectionRequestKinds(),
		},
	})
	return declarations
}

// 混ざる組の test が使う値。
const (
	mixedTerminalId    = "11111111-2222-3333-4444-555555555555"
	mixedDestinationIp = "198.51.100.7"
	mixedDestinationPt = "8080"
	mixedEventTimeText = "2024-03-14T10:20:30+09:00"
)

// mixedObservationKindTime は事象の時刻を返す。
func mixedObservationKindTime(t *testing.T) core.Timestamp {
	t.Helper()
	rawText, normalized, offsetText := mixedEventTimeText, mixedEventTimeText, "+09:00"
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond, OffsetState: core.OffsetStateInValue,
		OffsetText: &offsetText,
		Clock:      core.ClockTerminalLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return timestamp
}

// mixedObservationKindObservation は関連付けに渡す観測 1 件を組む。
func mixedObservationKindObservation(
	t *testing.T, fileName string, kind core.ObservationKind,
) core.MatchObservation {
	t.Helper()
	eventTime := mixedObservationKindTime(t)
	timeField, err := core.NewTimestampField("eventTime", core.SemanticKeyEventTime, eventTime)
	if err != nil {
		t.Fatal(err)
	}
	lineNumber := int64(1)
	return core.MatchObservation{
		Ref: core.RecordLocator{
			SourceId: fileName, SourceContentSha256: strings.Repeat("a", 64),
			SourceFileName: fileName, PositionKind: core.PositionKindLineNumber,
			LineNumber: &lineNumber, RecordRawTextRef: "raw:" + fileName,
		},
		EventTime:       eventTime,
		ObservationKind: kind,
		Fields: []core.RecordField{
			textField("terminalId", core.SemanticKeyTerminalId, presentText(mixedTerminalId)),
			textField("destinationIp", core.SemanticKeyConnectionDestinationAddress,
				presentText(mixedDestinationIp)),
			textField("destinationPort", core.SemanticKeyConnectionDestinationPort,
				presentText(mixedDestinationPt)),
			timeField,
		},
	}
}

// mixedObservationKindRequest は、観測の種別の欄を持つ収集元と持たない収集元が候補の側に
// 混ざる関連付けの要求を組む。
//
// 候補は 3 件である。宣言した種別を持つ 1 件、宣言の外の種別を持つ 1 件、種別を持たない
// 1 件を置き、絞りの一致の仕方を 1 つの要求で分ける。
func mixedObservationKindRequest(
	t *testing.T, selectors []core.ObservationKindSelector,
) core.MatchRequest {
	t.Helper()
	selectedKind := core.ObservationKind{
		Raw: []core.RecordField{
			textField("evt", "", presentText("net")),
			textField("subEvt", "", presentText("con")),
		},
		Status: core.ObservationKindStatusDetermined,
	}
	rejectedKind := core.ObservationKind{
		Raw: []core.RecordField{
			textField("evt", "", presentText("net")),
			textField("subEvt", "", presentText("dcon")),
		},
		Status: core.ObservationKindStatusDetermined,
	}
	absentKind := core.ObservationKind{Raw: []core.RecordField{}}
	candidates := make([]core.MatchCandidateRecord, 0, 3)
	for _, member := range []struct {
		fileName string
		kind     core.ObservationKind
	}{
		{"with-kind-selected.log", selectedKind},
		{"with-kind-rejected.log", rejectedKind},
		{"without-kind.log", absentKind},
	} {
		candidates = append(candidates, core.MatchCandidateRecord{
			Observation: mixedObservationKindObservation(t, member.fileName, member.kind),
		})
	}
	eventTime := mixedObservationKindTime(t)
	return core.MatchRequest{
		Origin: core.MatchOrigin{
			Observation:      mixedObservationKindObservation(t, "origin.log", absentKind),
			DestinationIpUse: core.ConditionUseUsed,
			AssignmentValidRange: core.TimeRange{
				From: eventTime, To: eventTime,
			},
		},
		Candidates: candidates,
		Conditions: []core.MatchConditionSpec{
			{
				ConditionKey:         core.ConditionKeyTerminalIpAssignment,
				OriginSemantic:       core.SemanticKeyTerminalId,
				CounterpartSemantics: []core.SemanticKey{core.SemanticKeyTerminalId},
				Compared:             true,
			},
			{
				ConditionKey:   core.ConditionKeyDestinationIp,
				OriginSemantic: core.SemanticKeyConnectionDestinationAddress,
				CounterpartSemantics: []core.SemanticKey{
					core.SemanticKeyConnectionDestinationAddress,
				},
				Compared: true,
			},
			{
				ConditionKey:         core.ConditionKeySecondOfTime,
				OriginSemantic:       core.SemanticKeyEventTime,
				CounterpartSemantics: []core.SemanticKey{core.SemanticKeyEventTime},
			},
		},
		CounterpartItemSemantics: []core.SemanticKey{
			core.SemanticKeyTerminalId, core.SemanticKeyConnectionDestinationAddress,
			core.SemanticKeyConnectionDestinationPort, core.SemanticKeyEventTime,
		},
		CounterpartTimePrecision: core.PrecisionSecond,
		StageKeys:                []core.StageKey{core.StageKeyClockIndependent},
		IncludedObservationKinds: selectors,
		SecondStageWindow:        core.TimeWindow{WindowKind: core.WindowKindNotCompared},
		SecondStageAssumptions:   AllMatchConditions().assumptions(),
		ClockDependencyNote:      clockDependencyNoteText,
		PublicationState:         core.PublicationStatePublishedFull,
		UnprocessedRanges:        []core.RecordRange{},
		AnalysisRunRef:           "run-mixed",
	}
}

// composeSpecs は宣言の組から段階の条件を組む。宣言の食い違いは test の失敗にする。
func composeSpecs(
	t *testing.T, origin matchSide, counterpart sideDeclarations,
	selection MatchConditionSelection,
) []core.MatchConditionSpec {
	t.Helper()
	specs, problem := candidateConditionSpecs(origin, counterpart, selection)
	if problem != nil {
		t.Fatalf("composing the stage conditions: %v", problem)
	}
	return specs
}

// 絞りに用いる条件は、相手の語彙の項目を 1 件だけ挙げる。
//
// 2 件以上を挙げると、候補の観測に入れる項目と、比較が実際に比べる項目が別の語彙に
// なりうる。宣言の側でこの前提を保つ。
func TestCandidateConditionSpecsNameOneCounterpartSemanticWhenCompared(t *testing.T) {
	specs := composeSpecs(t, squidOriginSide(), markIICounterpart(), AllMatchConditions())
	compared := 0
	for _, spec := range specs {
		if !spec.Compared {
			continue
		}
		compared++
		if len(spec.CounterpartSemantics) != 1 {
			t.Errorf("the compared condition %s names %d counterpart semantics, want 1",
				spec.ConditionKey, len(spec.CounterpartSemantics))
		}
	}
	// **絞りに用いる条件を 1 件以上持つ。** 1 件も無い宣言が作る段階は、どの候補も
	// 除かないまま「条件に一致した候補」を名乗る集合になる。
	if compared == 0 {
		t.Error("the declaration compares no condition while every condition is selected")
	}
}

// 分析者が選ばなかった条件を絞りに用いない。
func TestCandidateConditionSpecsFollowTheSelection(t *testing.T) {
	comparedOf := func(selection MatchConditionSelection) bool {
		for _, spec := range composeSpecs(t, squidOriginSide(), markIICounterpart(), selection) {
			if spec.ConditionKey == core.ConditionKeyDestinationIp {
				return spec.Compared
			}
		}
		t.Fatal("the declaration carries no destination_ip condition")
		return false
	}
	if !comparedOf(AllMatchConditions()) {
		t.Error("the destination_ip condition is not compared while the analyst selects it")
	}
	withoutDestinationIp := MatchConditionSelection{
		Conditions: []SelectedMatchCondition{
			{ConditionKey: core.ConditionKeySecondOfTime},
		},
	}
	if comparedOf(withoutDestinationIp) {
		t.Error("the destination_ip condition is compared while the analyst leaves it out")
	}
}

// **core の test が持つ宣言の複製と、本 package の宣言を突き合わせる。**
//
// core は pipeline を import できないため、core/fixtures_test.go の matchConditionSpecs が
// 同じ組を手で持つ。片方だけを直すと、core の test が古い宣言のまま通り続ける。
func TestCandidateConditionSpecsMatchTheCoreTestDeclaration(t *testing.T) {
	// core/fixtures_test.go の matchConditionSpecs と同じ組である。
	want := []core.MatchConditionSpec{
		{
			ConditionKey:         core.ConditionKeyTerminalIpAssignment,
			OriginSemantic:       core.SemanticKeyTerminalId,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyTerminalId},
			Compared:             true,
		},
		{
			ConditionKey:   core.ConditionKeyDestinationIp,
			OriginSemantic: core.SemanticKeyConnectionDestinationAddress,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionDestinationAddress,
			},
			Compared: true,
		},
		{
			ConditionKey:   core.ConditionKeyDestinationPort,
			OriginSemantic: core.SemanticKeyConnectionDestinationPort,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionDestinationPort,
			},
			Compared: true,
		},
		{
			ConditionKey:         core.ConditionKeySecondOfTime,
			OriginSemantic:       core.SemanticKeyEventTime,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey:         core.ConditionKeySubSecondOfTime,
			OriginSemantic:       core.SemanticKeyEventTime,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey:   core.ConditionKeyClientPort,
			OriginSemantic: core.SemanticKeyConnectionSourcePort,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionSourcePort,
			},
		},
		{
			ConditionKey:         core.ConditionKeyProcess,
			OriginSemantic:       core.SemanticKeyProcessId,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyProcessId},
		},
		{
			ConditionKey:   core.ConditionKeyUser,
			OriginSemantic: core.SemanticKeyAccountName,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyAccountName, core.SemanticKeyEventAccountName,
			},
		},
	}
	got := composeSpecs(t, squidOriginSide(), markIICounterpart(), AllMatchConditions())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidateConditionSpecs = %#v, want the declaration core/fixtures_test.go "+
			"copies (%#v)", got, want)
	}
}

// 相手の語彙の項目は候補の側の収集元の宣言から決まる。
//
// 同じ起点でも、候補の側に置く収集元が変われば比べる相手の項目が変わる。候補の側の
// 収集元が 1 つも無い組では、起点の側が挙げた項目を相手の項目に置く。
func TestCandidateConditionSpecsTakeTheCounterpartSemanticsFromTheCandidateSide(t *testing.T) {
	counterpartOf := func(counterpart sideDeclarations) []core.SemanticKey {
		for _, spec := range composeSpecs(
			t, squidOriginSide(), counterpart, AllMatchConditions()) {
			if spec.ConditionKey == core.ConditionKeyUser {
				return spec.CounterpartSemantics
			}
		}
		t.Fatal("the declaration carries no user condition")
		return nil
	}
	withCounterpart := counterpartOf(markIICounterpart())
	want := []core.SemanticKey{core.SemanticKeyAccountName, core.SemanticKeyEventAccountName}
	if !reflect.DeepEqual(withCounterpart, want) {
		t.Errorf("the user condition compares %v, want the counterpart declaration %v",
			withCounterpart, want)
	}
	withoutCounterpart := counterpartOf(newSideDeclarations())
	if !reflect.DeepEqual(withoutCounterpart, []core.SemanticKey{core.SemanticKeyAccountName}) {
		t.Errorf("the user condition compares %v, want the origin declaration %v",
			withoutCounterpart, []core.SemanticKey{core.SemanticKeyAccountName})
	}
}

// 起点が挙げていない条件は段階に入らない。
//
// 条件の一覧と並びを決めるのは起点の側の宣言である。相手だけが挙げた条件を足すと、
// 起点が値を持たない条件の leftValue が段階に出る。
func TestCandidateConditionSpecsFollowTheOriginDeclarationOrder(t *testing.T) {
	origin := matchSide{matchConditions: []ConnectionMatchCondition{
		{
			ConditionKey:      core.ConditionKeyDestinationPort,
			Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationPort},
			NarrowsCandidates: true,
		},
		{
			ConditionKey: core.ConditionKeySecondOfTime,
			Semantics:    []core.SemanticKey{core.SemanticKeyEventTime},
		},
	}}
	keys := make([]core.ConditionKey, 0)
	for _, spec := range composeSpecs(t, origin, markIICounterpart(), AllMatchConditions()) {
		keys = append(keys, spec.ConditionKey)
	}
	want := []core.ConditionKey{
		core.ConditionKeyDestinationPort, core.ConditionKeySecondOfTime,
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("the stage carries the conditions %v, want the origin declaration %v", keys, want)
	}
}

// 観測の種別の選択子は、候補の側の収集元が挙げた種別の和集合である。
//
// **欄を持たない収集元が混ざる組でも、欄を持つ収集元の種別を除かない。** 要素数 0 へ
// まとめると、欄を持つ収集元の候補が種別で絞られなくなる。種別を持たない候補がこの集合で
// 除かれないことは、backend/core の selectedByObservationKind が決める。
func TestObservationKindsUnionEveryDeclaringSource(t *testing.T) {
	wantKinds := markiiConnectionRequestKinds()
	if got := markIICounterpart().observationKinds(); !reflect.DeepEqual(got, wantKinds) {
		t.Errorf("the counterpart selects %v, want the declared kinds %v", got, wantKinds)
	}
	mixed := markIICounterpart()
	mixed.add(SourcePublication{
		status: core.ImportStatus{SourceId: "src-without-kind-field"},
		parser: ParserIdentity{ConnectionRequestKinds: []core.ObservationKindSelector{}},
	})
	if got := mixed.observationKinds(); !reflect.DeepEqual(got, wantKinds) {
		t.Errorf("the mixed counterpart selects %v, want the declared kinds %v", got, wantKinds)
	}
	// 種別を 1 つも挙げない側は、種別で絞らない。
	if got := newSideDeclarations().observationKinds(); len(got) != 0 {
		t.Errorf("a side that declares no kind selects %v, want no selector", got)
	}
}

// 観測の種別の欄を持つ収集元と持たない収集元が候補の側に混ざる組で、関連付けが成立する。
//
// **両側を 1 つの test で通す。** 種別を持つ候補は宣言した種別に一致するものだけが残り、
// 種別を持たない候補は種別を理由に除かれない。どちらか片側だけを通すと、
// backend/core の validateMemberObservationKind との食い違いを見つけられない。
func TestBuildCandidateSetAcceptsMixedObservationKindSources(t *testing.T) {
	selectors := markiiConnectionRequestKinds()
	request := mixedObservationKindRequest(t, selectors)
	set, err := core.BuildCandidateSet(request)
	if err != nil {
		t.Fatalf("building the candidate set: %v", err)
	}
	stage, found := stageOf(set, core.StageKeyClockIndependent)
	if !found {
		t.Fatal("the candidate set carries no clock independent stage")
	}
	names := make([]string, 0, len(stage.Members))
	for _, member := range stage.Members {
		names = append(names, member.RecordRef.SourceFileName)
	}
	// 宣言した種別に一致する候補と、種別を持たない候補が残る。宣言の外の種別を持つ候補は
	// 除かれる。
	requireSameStrings(t, "stage members", names, "with-kind-selected.log", "without-kind.log")
	if !reflect.DeepEqual(stage.IncludedObservationKinds, selectors) {
		t.Errorf("the stage selects %v, want the declared kinds %v",
			stage.IncludedObservationKinds, selectors)
	}
}

// markIIOriginSide は markii 形式の収集元のレコードを起点に置いた側を返す。
func markIIOriginSide() matchSide {
	return matchSide{matchConditions: markiiConnectionMatchConditions()}
}

// squidCounterpart は Squid の収集元を候補の側に置いた宣言を返す。
func squidCounterpart() sideDeclarations {
	declarations := newSideDeclarations()
	declarations.add(SourcePublication{
		status: core.ImportStatus{SourceId: "src-address-only"},
		parser: ParserIdentity{
			ConnectionMatchConditions: squidConnectionMatchConditions(),
			ConnectionRequestKinds:    []core.ObservationKindSelector{},
		},
	})
	return declarations
}

// 端末の外部識別子を直に持つ起点の向きでも、段階の条件を exact value で確かめる。
//
// **この向きは相手の語彙の項目が狭まる。** 候補の側が account.name だけを挙げるため、
// user の条件の相手の項目は 1 件になる。もう一方の向き
// (TestCandidateConditionSpecsMatchTheCoreTestDeclaration) と組で、
// 相手の側の宣言が段階の条件を決めることを両方向から確かめる。
func TestCandidateConditionSpecsOfTheTerminalOwnerOrigin(t *testing.T) {
	want := []core.MatchConditionSpec{
		{
			ConditionKey:         core.ConditionKeyTerminalIpAssignment,
			OriginSemantic:       core.SemanticKeyTerminalId,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyTerminalId},
			Compared:             true,
		},
		{
			ConditionKey:   core.ConditionKeyDestinationIp,
			OriginSemantic: core.SemanticKeyConnectionDestinationAddress,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionDestinationAddress,
			},
			Compared: true,
		},
		{
			ConditionKey:   core.ConditionKeyDestinationPort,
			OriginSemantic: core.SemanticKeyConnectionDestinationPort,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionDestinationPort,
			},
			Compared: true,
		},
		{
			ConditionKey:         core.ConditionKeySecondOfTime,
			OriginSemantic:       core.SemanticKeyEventTime,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey:         core.ConditionKeySubSecondOfTime,
			OriginSemantic:       core.SemanticKeyEventTime,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey:   core.ConditionKeyClientPort,
			OriginSemantic: core.SemanticKeyConnectionSourcePort,
			CounterpartSemantics: []core.SemanticKey{
				core.SemanticKeyConnectionSourcePort,
			},
		},
		{
			ConditionKey:         core.ConditionKeyProcess,
			OriginSemantic:       core.SemanticKeyProcessId,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyProcessId},
		},
		{
			// 候補の側が account.name だけを挙げるため、event.account_name は入らない。
			ConditionKey:         core.ConditionKeyUser,
			OriginSemantic:       core.SemanticKeyAccountName,
			CounterpartSemantics: []core.SemanticKey{core.SemanticKeyAccountName},
		},
	}
	got := composeSpecs(t, markIIOriginSide(), squidCounterpart(), AllMatchConditions())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidateConditionSpecs = %#v, want %#v", got, want)
	}
}

// 絞りに用いる条件の相手の項目が 2 件になる宣言の組を、起点の分類に化けさせない。
//
// **原資料についての分類と分ける。** 候補の側の 2 つの収集元が同じ条件へ別の語彙の項目を
// 挙げた組であり、起点のレコードは関係しない。
func TestCandidateConditionSpecsRejectTwoCounterpartSemanticsOnANarrowingCondition(t *testing.T) {
	counterpart := newSideDeclarations()
	for index, semantic := range []core.SemanticKey{
		core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyTerminalIpAddress,
	} {
		counterpart.add(SourcePublication{
			status: core.ImportStatus{SourceId: "src-" + strconv.Itoa(index)},
			parser: ParserIdentity{ConnectionMatchConditions: []ConnectionMatchCondition{{
				ConditionKey:      core.ConditionKeyDestinationIp,
				Semantics:         []core.SemanticKey{semantic},
				NarrowsCandidates: true,
			}}},
		})
	}
	origin := matchSide{matchConditions: []ConnectionMatchCondition{{
		ConditionKey:      core.ConditionKeyDestinationIp,
		Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationAddress},
		NarrowsCandidates: true,
	}}}
	if _, problem := candidateConditionSpecs(
		origin, counterpart, AllMatchConditions()); problem == nil {
		t.Fatal("the composition accepted two counterpart semantics on a narrowing condition")
	}
	// 絞りに用いない条件では、同じ組を受け取る。
	origin.matchConditions[0].NarrowsCandidates = false
	specs, problem := candidateConditionSpecs(origin, counterpart, AllMatchConditions())
	if problem != nil {
		t.Fatalf("composing a condition that does not narrow candidates: %v", problem)
	}
	want := []core.SemanticKey{
		core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyTerminalIpAddress,
	}
	if len(specs) != 1 || !reflect.DeepEqual(specs[0].CounterpartSemantics, want) {
		t.Errorf("the composition returned %#v, want the union %v", specs, want)
	}
}

// 宣言 1 件の検査は、通す組と拒否する組の両方を通す。
func TestConnectionMatchConditionValidate(t *testing.T) {
	cases := map[string]struct {
		declaration ConnectionMatchCondition
		accepted    bool
	}{
		"絞りに用いる条件が項目を 1 つ挙げる": {ConnectionMatchCondition{
			ConditionKey:      core.ConditionKeyDestinationIp,
			Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationAddress},
			NarrowsCandidates: true,
		}, true},
		"絞りに用いない条件が項目を 2 つ挙げる": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKeyUser,
			Semantics: []core.SemanticKey{
				core.SemanticKeyAccountName, core.SemanticKeyEventAccountName,
			},
		}, true},
		"絞りに用いる条件が項目を 2 つ挙げる": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKeyUser,
			Semantics: []core.SemanticKey{
				core.SemanticKeyAccountName, core.SemanticKeyEventAccountName,
			},
			NarrowsCandidates: true,
		}, false},
		"語彙の外の条件の種別": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKey("destination_mac"),
			Semantics:    []core.SemanticKey{core.SemanticKeyTerminalId},
		}, false},
		"項目を 1 つも挙げない": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKeyUser,
		}, false},
		"空の項目を挙げる": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKeyUser,
			Semantics:    []core.SemanticKey{""},
		}, false},
		"同じ項目を 2 回挙げる": {ConnectionMatchCondition{
			ConditionKey: core.ConditionKeyUser,
			Semantics: []core.SemanticKey{
				core.SemanticKeyAccountName, core.SemanticKeyAccountName,
			},
		}, false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			problem := want.declaration.Validate()
			if want.accepted && problem != nil {
				t.Errorf("Validate = %v, want no problem", problem)
			}
			if !want.accepted && problem == nil {
				t.Error("Validate accepted the declaration")
			}
		})
	}
}

// パーサーの宣言の検査は、同じ条件の種別を 2 回挙げる組を拒否する。
//
// 現行の binding の宣言は検査を通る。両側を 1 つの test で通す。
func TestParserIdentityValidateRejectsARepeatedConditionKey(t *testing.T) {
	for _, declarations := range [][]ConnectionMatchCondition{
		markiiConnectionMatchConditions(), squidConnectionMatchConditions(),
	} {
		identity := ParserIdentity{ParserID: "p", ConnectionMatchConditions: declarations}
		if problem := identity.Validate(); problem != nil {
			t.Errorf("Validate = %v, want no problem for a binding declaration", problem)
		}
	}
	repeated := ParserIdentity{ParserID: "p", ConnectionMatchConditions: []ConnectionMatchCondition{
		{
			ConditionKey: core.ConditionKeyUser,
			Semantics:    []core.SemanticKey{core.SemanticKeyAccountName},
		},
		{
			ConditionKey: core.ConditionKeyUser,
			Semantics:    []core.SemanticKey{core.SemanticKeyEventAccountName},
		},
	}}
	if problem := repeated.Validate(); problem == nil {
		t.Error("Validate accepted a repeated condition key")
	}
}

// パーサーの宣言の検査は、観測の種別の宣言も確かめる。
//
// **重複除去より前に確かめる。** 欄の名前を 2 回挙げた宣言が和集合へ入ると、異なる欄の
// 名前の個数が食い違い、正しい宣言と別の組として扱われる (sameSelector)。
func TestParserIdentityValidateChecksTheObservationKindSelectors(t *testing.T) {
	accepted := ParserIdentity{
		ParserID:               "p",
		ConnectionRequestKinds: markiiConnectionRequestKinds(),
	}
	if problem := accepted.Validate(); problem != nil {
		t.Errorf("Validate = %v, want no problem for a binding declaration", problem)
	}
	repeated := ParserIdentity{
		ParserID: "p",
		ConnectionRequestKinds: []core.ObservationKindSelector{{
			Items: []core.ObservationKindSelectorItem{
				{Name: "evt", Value: "net"},
				{Name: "evt", Value: "net"},
			},
		}},
	}
	if problem := repeated.Validate(); problem == nil {
		t.Error("Validate accepted a selector that repeats a field name")
	}
	empty := ParserIdentity{
		ParserID:               "p",
		ConnectionRequestKinds: []core.ObservationKindSelector{{Items: nil}},
	}
	if problem := empty.Validate(); problem == nil {
		t.Error("Validate accepted a selector that names no item")
	}
}

// 宣言の食い違いの理由を、分類と一緒に呼び出し元へ渡す。
//
// **分類だけでは、500 を受けた運用者が原因の宣言に到達できない。** 理由の文字列が条件の
// 種別を挙げることを確かめる。
func TestCandidateSetBuilderCarriesTheDeclarationProblem(t *testing.T) {
	counterpart := newSideDeclarations()
	for index, semantic := range []core.SemanticKey{
		core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyTerminalIpAddress,
	} {
		counterpart.add(SourcePublication{
			status: core.ImportStatus{SourceId: "src-" + strconv.Itoa(index)},
			parser: ParserIdentity{ConnectionMatchConditions: []ConnectionMatchCondition{{
				ConditionKey:      core.ConditionKeyDestinationIp,
				Semantics:         []core.SemanticKey{semantic},
				NarrowsCandidates: true,
			}}},
		})
	}
	origin := matchSide{matchConditions: []ConnectionMatchCondition{{
		ConditionKey:      core.ConditionKeyDestinationIp,
		Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationAddress},
		NarrowsCandidates: true,
	}}}
	_, problem := candidateConditionSpecs(origin, counterpart, AllMatchConditions())
	if problem == nil {
		t.Fatal("the composition accepted two counterpart semantics on a narrowing condition")
	}
	if !strings.Contains(problem.Error(), string(core.ConditionKeyDestinationIp)) {
		t.Errorf("the problem %q names no condition key", problem)
	}
}

// 宣言の重なりの判定は、同じ組と違う組の両方を通す。
//
// 並びの違いは同じ宣言であり、欄の名前の違いと value の違いは別の宣言である。
func TestSameSelectorSeparatesTheItemSetFromTheOrder(t *testing.T) {
	selectorOf := func(items ...core.ObservationKindSelectorItem) core.ObservationKindSelector {
		return core.ObservationKindSelector{Items: items}
	}
	evtNet := core.ObservationKindSelectorItem{Name: "evt", Value: "net"}
	subConnect := core.ObservationKindSelectorItem{Name: "subEvt", Value: "con"}
	subClose := core.ObservationKindSelectorItem{Name: "subEvt", Value: "dcon"}
	otherName := core.ObservationKindSelectorItem{Name: "kind", Value: "con"}
	base := selectorOf(evtNet, subConnect)
	cases := map[string]struct {
		other core.ObservationKindSelector
		same  bool
	}{
		"同じ欄と同じ value":  {selectorOf(evtNet, subConnect), true},
		"並びが違う":         {selectorOf(subConnect, evtNet), true},
		"value が違う":     {selectorOf(evtNet, subClose), false},
		"欄の名前が違う":       {selectorOf(evtNet, otherName), false},
		"要素数が違う":        {selectorOf(evtNet), false},
		"同じ欄を 2 回挙げている": {selectorOf(evtNet, evtNet), false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := sameSelector(base, want.other); got != want.same {
				t.Errorf("sameSelector = %t, want %t", got, want.same)
			}
			if got := containsSelector([]core.ObservationKindSelector{base}, want.other); got != want.same {
				t.Errorf("containsSelector = %t, want %t", got, want.same)
			}
		})
	}
	if containsSelector(nil, base) {
		t.Error("containsSelector found a selector in an empty set")
	}
}

// 同じ収集元の宣言を 2 回取り込まない。
//
// 2 回取り込むと、語彙の項目の和集合と観測の種別の集合が収集元の数で変わる。
func TestSideDeclarationsIgnoreARepeatedSource(t *testing.T) {
	once := markIICounterpart()
	twice := markIICounterpart()
	twice.add(SourcePublication{
		status: core.ImportStatus{SourceId: "src-counterpart"},
		parser: ParserIdentity{
			ConnectionMatchConditions: markiiConnectionMatchConditions(),
			ConnectionRequestKinds:    markiiConnectionRequestKinds(),
		},
	})
	if !reflect.DeepEqual(once.observationKinds(), twice.observationKinds()) {
		t.Errorf("the repeated source changed the kinds to %v", twice.observationKinds())
	}
	if !reflect.DeepEqual(once.conditionSemantics, twice.conditionSemantics) {
		t.Errorf("the repeated source changed the semantics to %v", twice.conditionSemantics)
	}
	// 別の収集元を足すと和集合が広がる。
	added := markIICounterpart()
	added.add(SourcePublication{
		status: core.ImportStatus{SourceId: "src-other"},
		parser: ParserIdentity{ConnectionMatchConditions: []ConnectionMatchCondition{{
			ConditionKey: core.ConditionKeyUser,
			Semantics:    []core.SemanticKey{core.SemanticKeyProcessUserName},
		}}},
	})
	if reflect.DeepEqual(once.conditionSemantics, added.conditionSemantics) {
		t.Error("a second source did not widen the union of the semantics")
	}
}
