package core_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// graphNodeId と graphEdgeId は検査用の不透明な識別子である。値の作り方は取り込みの実行が
// 持つため、文字列の形を検査が要求しない。
const (
	graphNodeId      = "n:terminal:0f"
	graphOtherNodeId = "n:ip:1a"
	graphEdgeId      = "e:terminal_address:2b"
)

// graphRecordLocator は検査用のレコード位置を返す。
func graphRecordLocator() core.RecordLocator {
	position := secondCandidateSn
	return core.RecordLocator{
		SourceId:            "source-1",
		SourceContentSha256: "0929a21531db43ebd02f35e9e0388541f472ff0cf7e1a09df13e798c3dd87c08",
		SourceFileName:      markIIFileName,
		PositionKind:        core.PositionKindSequenceNumber,
		SequenceNumber:      &position,
		RecordRawTextRef:    "raw:1",
	}
}

// graphTerminalNode は検査を通るノードを返す。
func graphTerminalNode(t *testing.T) core.GraphNode {
	t.Helper()
	label, err := core.NewRawValue(core.ValueStatePresent, "TESTHOST")
	if err != nil {
		t.Fatal(err)
	}
	return core.GraphNode{
		Id: graphNodeId, Kind: core.NodeKindTerminal, KeyForm: core.NodeKeyFormTerminalId,
		Identity:    []core.NodeIdentityValue{{Semantic: core.SemanticKeyTerminalId, Value: "T1"}},
		Label:       label,
		Observation: core.NodeObservationObserved,
		// 端末は生成を記録した根拠を持てない。
		CreationRecord: core.NodeCreationRecordItemAbsent,
	}
}

// graphProcessNode は生成のレコードを持つプロセスのノードを返す。
func graphProcessNode(t *testing.T) core.GraphNode {
	t.Helper()
	node := graphTerminalNode(t)
	node.Kind = core.NodeKindProcess
	node.KeyForm = core.NodeKeyFormTerminalProcess
	node.Identity = []core.NodeIdentityValue{
		{Semantic: core.SemanticKeyTerminalId, Value: "T1"},
		{Semantic: core.SemanticKeyProcessId, Value: "{P1}"},
	}
	node.CreationRecord = core.NodeCreationRecordPresent
	return node
}

func requireValid(t *testing.T, item string, value interface{ Validate() error }) {
	t.Helper()
	if err := value.Validate(); err != nil {
		t.Errorf("%s did not validate: %v", item, err)
	}
}

func requireInvalid(t *testing.T, item string, value interface{ Validate() error }) {
	t.Helper()
	if err := value.Validate(); err == nil {
		t.Errorf("%s validated, want an error", item)
	}
}

func TestGraphNodeValidateSeparatesTheCompleteNodeFromTheIncompleteOne(t *testing.T) {
	complete := graphTerminalNode(t)
	requireValid(t, "a complete node", complete)

	withoutId := complete
	withoutId.Id = ""
	requireInvalid(t, "a node without an id", withoutId)

	unknownKind := complete
	unknownKind.Kind = "session"
	requireInvalid(t, "a node of an unknown kind", unknownKind)

	unknownForm := complete
	unknownForm.KeyForm = "terminal_hostname"
	requireInvalid(t, "a node with an unknown key form", unknownForm)

	withoutIdentity := complete
	withoutIdentity.Identity = nil
	requireInvalid(t, "a node without an identity", withoutIdentity)

	emptyIdentityValue := complete
	emptyIdentityValue.Identity = []core.NodeIdentityValue{{Value: ""}}
	requireInvalid(t, "a node whose identity value is empty", emptyIdentityValue)

	unknownLabelState := complete
	unknownLabelState.Label = core.RawAndNormalized{ValueState: "observed"}
	requireInvalid(t, "a node whose label state is outside the vocabulary", unknownLabelState)

	unknownObservation := complete
	unknownObservation.Observation = "inferred"
	requireInvalid(t, "a node whose observation is outside the vocabulary", unknownObservation)

	unknownCreationRecord := complete
	unknownCreationRecord.CreationRecord = "undetermined"
	requireInvalid(t, "a node whose creation record is outside the vocabulary", unknownCreationRecord)
}

// まとめる鍵と理由はアカウントのノードだけが持ち、2 つは同時に出ない。
func TestGraphNodeValidateKeepsTheAccountNameOnAccounts(t *testing.T) {
	account := graphTerminalNode(t)
	account.Kind = core.NodeKindAccount
	account.KeyForm = core.NodeKeyFormAccountSid
	account.Identity = []core.NodeIdentityValue{{Semantic: core.SemanticKeyAccountSid, Value: "S-1-5-21-1-2-3-1001"}}
	account.CreationRecord = core.NodeCreationRecordAbsent
	key := &core.AccountNameKey{Name: `host-a\user-a`, NodeCount: 2}

	keyed := account
	keyed.AccountName = key
	requireValid(t, "an account with a key", keyed)

	withheld := account
	withheld.AccountNameWithheld = core.AccountNameWithheldMultipleNames
	requireValid(t, "an account with a reason", withheld)

	both := keyed
	both.AccountNameWithheld = core.AccountNameWithheldMultipleNames
	requireInvalid(t, "an account with a key and a reason", both)

	unknown := account
	unknown.AccountNameWithheld = "renamed"
	requireInvalid(t, "an account with an unknown reason", unknown)

	uncounted := account
	uncounted.AccountName = &core.AccountNameKey{Name: `host-a\user-a`}
	requireInvalid(t, "an account key counting no node", uncounted)

	terminal := graphTerminalNode(t)
	terminal.AccountName = key
	requireInvalid(t, "a terminal with an account key", terminal)
}

// **2 つの軸は独立している。** 生成を記録した根拠を持てるのはプロセスだけである。
func TestNodeCreationRecordSeparatesTheAxisFromTheObservation(t *testing.T) {
	for _, creationRecord := range []core.NodeCreationRecord{
		core.NodeCreationRecordPresent, core.NodeCreationRecordAbsent,
		core.NodeCreationRecordItemAbsent,
	} {
		if !creationRecord.IsKnown() {
			t.Errorf("the creation record %q is not known", creationRecord)
		}
	}
	for _, creationRecord := range []core.NodeCreationRecord{"undetermined", ""} {
		if creationRecord.IsKnown() {
			t.Errorf("the creation record %q is reported as known", creationRecord)
		}
	}
	if !core.NodeKindProcess.CarriesCreationRecord() || !core.NodeKindAccount.CarriesCreationRecord() {
		t.Error("a process or an account does not carry a creation record")
	}
	for _, kind := range []core.NodeKind{
		core.NodeKindTerminal, core.NodeKindFile, core.NodeKindRegistryValue,
		core.NodeKindIp, core.NodeKindDomain,
	} {
		if kind.CarriesCreationRecord() {
			t.Errorf("the kind %q carries a creation record", kind)
		}
	}
	// 記録したレコードがあるプロセスでも、生成のレコードが無い組が成り立つ。
	withoutCreation := graphProcessNode(t)
	withoutCreation.CreationRecord = core.NodeCreationRecordAbsent
	requireValid(t, "an observed process without a creation record", withoutCreation)

	// 生成を記録した根拠を持てない種別に present と absent を置かない。
	terminalWithCreation := graphTerminalNode(t)
	terminalWithCreation.CreationRecord = core.NodeCreationRecordPresent
	requireInvalid(t, "a terminal carrying a creation record", terminalWithCreation)

	processItemAbsent := graphProcessNode(t)
	processItemAbsent.CreationRecord = core.NodeCreationRecordItemAbsent
	requireInvalid(t, "a process whose creation record is item_absent", processItemAbsent)
}

func TestNodeObservationIsKnownSeparatesTheTwoValuesFromTheOthers(t *testing.T) {
	for _, observation := range []core.NodeObservation{
		core.NodeObservationObserved, core.NodeObservationReferenced,
	} {
		if !observation.IsKnown() {
			t.Errorf("the node observation %q is not known", observation)
		}
	}
	for _, observation := range []core.NodeObservation{"inferred", ""} {
		if observation.IsKnown() {
			t.Errorf("the node observation %q is reported as known", observation)
		}
	}
	referenced := graphTerminalNode(t)
	referenced.Observation = core.NodeObservationReferenced
	requireValid(t, "a node that only a reference names", referenced)
}

func TestNodeIdentityValueValidateSeparatesTheKnownSemanticFromTheUnknownOne(t *testing.T) {
	requireValid(t, "an identity value with a semantic",
		core.NodeIdentityValue{Semantic: core.SemanticKeyTerminalId, Value: "T1"})
	// address の形は語彙の項目を 1 つに定めないため、semantic を持たない要素が成立する。
	requireValid(t, "an identity value without a semantic",
		core.NodeIdentityValue{Value: "192.0.2.1"})
	requireInvalid(t, "an identity value of an unknown semantic",
		core.NodeIdentityValue{Semantic: "terminal.serial", Value: "T1"})
	requireInvalid(t, "an identity value without a value",
		core.NodeIdentityValue{Semantic: core.SemanticKeyTerminalId})
}

func TestSubgraphNodeValidateSeparatesTheKnownSelectionFromTheUnknownOne(t *testing.T) {
	node := graphTerminalNode(t)
	for _, selection := range []core.NodeSelection{
		core.NodeSelectionMatched, core.NodeSelectionEdgeEndpoint,
	} {
		if !selection.IsKnown() {
			t.Errorf("the selection %q is not known", selection)
		}
		requireValid(t, "a node carrying the selection "+string(selection),
			core.SubgraphNode{GraphNode: node, Selection: selection})
	}
	if core.NodeSelection("neighbour").IsKnown() {
		t.Error("an unknown selection is reported as known")
	}
	requireInvalid(t, "a node of an unknown selection",
		core.SubgraphNode{GraphNode: node, Selection: "neighbour"})
	// 埋め込んだノードの検査も通る。
	broken := node
	broken.Id = ""
	requireInvalid(t, "a node without an id carrying a known selection",
		core.SubgraphNode{GraphNode: broken, Selection: core.NodeSelectionMatched})
}

func TestGraphEvidenceValidateSeparatesTheReadableRecordFromTheBrokenOne(t *testing.T) {
	withoutTime := core.GraphEvidence{RecordRef: graphRecordLocator()}
	requireValid(t, "evidence without an event time", withoutTime)

	withTime := withoutTime
	eventTime := markIIEventTime(t)
	withTime.EventTime = &eventTime
	requireValid(t, "evidence with an event time", withTime)

	withoutRef := withoutTime
	withoutRef.RecordRef = core.RecordLocator{}
	requireInvalid(t, "evidence without a record reference", withoutRef)

	brokenTime := withoutTime
	brokenTime.EventTime = &core.Timestamp{}
	requireInvalid(t, "evidence carrying a broken event time", brokenTime)

	brokenKind := withoutTime
	brokenKind.ObservationKind = core.ObservationKind{Status: core.ObservationKindStatusDetermined}
	requireInvalid(t, "evidence carrying a status without a raw item", brokenKind)

	// 時刻の欄の名前を持つ時系列の行は、その行の時刻を持つ。
	requireValid(t, "a timeline row of a named time", core.TimelineEntry{GraphEvidence: withTime, TimeFieldName: "Run#2"})
	requireInvalid(t, "a timeline row naming a time without the time",
		core.TimelineEntry{GraphEvidence: withoutTime, TimeFieldName: "Run#2"})
}

// graphEdge は検査を通るエッジを返す。
func graphEdge(t *testing.T) core.GraphEdge {
	t.Helper()
	return core.GraphEdge{
		Id: graphEdgeId, Kind: core.EdgeKindTerminalAddress, State: core.RelationStateObserved,
		SourceNodeId: graphNodeId, TargetNodeId: graphOtherNodeId,
		EvidenceCount: 1,
	}
}

func TestGraphEdgeValidateSeparatesTheConsistentEdgeFromTheInconsistentOne(t *testing.T) {
	complete := graphEdge(t)
	requireValid(t, "a complete edge", complete)

	withRange := complete
	from := markIIEventTime(t)
	withRange.ApplicableRange = &core.TimeRange{From: from, To: from}
	requireValid(t, "an edge carrying an applicable range", withRange)

	unknownKind := complete
	unknownKind.Kind = "copied_to"
	requireInvalid(t, "an edge of an unknown kind", unknownKind)

	unknownState := complete
	unknownState.State = "derived"
	requireInvalid(t, "an edge of an unknown state", unknownState)

	withoutTarget := complete
	withoutTarget.TargetNodeId = ""
	requireInvalid(t, "an edge without a target", withoutTarget)

	brokenRange := complete
	brokenRange.ApplicableRange = &core.TimeRange{}
	requireInvalid(t, "an edge carrying a broken applicable range", brokenRange)

	// 根拠を 1 件も持たない関係は無い。件数が負の組を受け付けない。
	negativeCount := complete
	negativeCount.EvidenceCount = -1
	requireInvalid(t, "an edge counting a negative number of evidence records", negativeCount)

	noEvidence := complete
	noEvidence.EvidenceCount = 0
	requireValid(t, "an edge counting no evidence record", noEvidence)

	// 互いに区別できない候補の数は、このエッジを含む 2 以上である。
	for count, valid := range map[int64]bool{2: true, 1: false, -1: false} {
		indistinguishable := complete
		indistinguishable.IndistinguishableCandidateCount = count
		if err := indistinguishable.Validate(); (err == nil) != valid {
			t.Errorf("an edge counting %d indistinguishable candidates: Validate() = %v, want valid %v",
				count, err, valid)
		}
	}
	// 候補の区分は 1 以上の番号と、1 つ以上の既知の条件を持つ。
	match := []core.EdgePairConditionKey{core.EdgePairConditionSessionAccountMatch}
	for name, testCase := range map[string]struct {
		tier  core.EdgeCandidateTier
		valid bool
	}{
		"the top tier":         {core.EdgeCandidateTier{Tier: 1, Conditions: match}, true},
		"the tier 0":           {core.EdgeCandidateTier{Tier: 0, Conditions: match}, false},
		"no condition":         {core.EdgeCandidateTier{Tier: 1}, false},
		"an unknown condition": {core.EdgeCandidateTier{Tier: 1, Conditions: []core.EdgePairConditionKey{"unknown"}}, false},
	} {
		tiered := complete
		tiered.CandidateTier = &testCase.tier
		if err := tiered.Validate(); (err == nil) != testCase.valid {
			t.Errorf("%s: Validate() = %v, want valid %v", name, err, testCase.valid)
		}
	}
}

// edgeMatch は検査を通る関連付けの結果 1 件を返す。
func edgeMatch(t *testing.T) core.EdgeMatch {
	t.Helper()
	candidate := secondMatchedCandidate(t, secondCandidateSn)
	centerTime := squidRequestedCenterTime()
	return core.EdgeMatch{
		OriginRef:    squidLocator(squidLineNumber),
		CandidateRef: candidate.RecordRef,
		StageKey:     core.StageKeySecondTimeMatched,
		Conditions:   stageConditions(t, core.StageKeySecondTimeMatched, true),
		Assumptions:  []core.MatchAssumption{clockOffsetAssumption()},
		TimeWindow: core.TimeWindow{
			WindowKind: core.WindowKindSameSecond, CenterTime: &centerTime,
		},
		TimeComparison: candidate.TimeComparison,
		ClockDependencyNote: "割当の適用期間の判定が Proxy と " + markIITerminalName +
			" の時計のずれに依拠する",
		StageTallies: []core.MatchStageTally{
			{
				StageKey:             core.StageKeyClockIndependent,
				MemberCount:          2,
				DistinctProcessCount: int64Ptr(1),
			},
			{
				StageKey:             core.StageKeySecondTimeMatched,
				MemberCount:          2,
				DistinctProcessCount: int64Ptr(1),
			},
		},
		IndistinguishableGroups: [][]core.RecordLocator{
			{markIILocator(firstCandidateSn), markIILocator(secondCandidateSn)},
		},
		UnresolvedReasons: candidate.UnresolvedReasons,
	}
}

func int64Ptr(value int64) *int64 { return &value }

// withStageTallies は段階の数え方だけを書き換えた関連付けを返す。
// 元の関連付けと要素を共有しないよう、段階の並びを複製してから渡す。
func withStageTallies(
	match core.EdgeMatch, change func([]core.MatchStageTally),
) core.EdgeMatch {
	tallies := slices.Clone(match.StageTallies)
	change(tallies)
	match.StageTallies = tallies
	return match
}

func TestEdgeMatchValidateSeparatesTheCompleteMatchFromTheBrokenOne(t *testing.T) {
	complete := edgeMatch(t)
	requireValid(t, "a complete match", complete)

	withoutOrigin := complete
	withoutOrigin.OriginRef = core.RecordLocator{}
	requireInvalid(t, "a match without an origin record", withoutOrigin)

	withoutCandidate := complete
	withoutCandidate.CandidateRef = core.RecordLocator{}
	requireInvalid(t, "a match without a candidate record", withoutCandidate)

	unknownStage := complete
	unknownStage.StageKey = "sub_second_time_matched"
	requireInvalid(t, "a match of an unknown stage", unknownStage)

	withoutConditions := complete
	withoutConditions.Conditions = nil
	requireInvalid(t, "a match without conditions", withoutConditions)

	brokenWindow := complete
	brokenWindow.TimeWindow = core.TimeWindow{WindowKind: core.WindowKindSameSecond}
	requireInvalid(t, "a match whose window has no centre", brokenWindow)

	brokenComparison := complete
	brokenComparison.TimeComparison = core.TimeComparison{ComparisonUnit: core.ComparisonUnitSecond}
	requireInvalid(t, "a match whose comparison has no times", brokenComparison)

	withoutReason := complete
	withoutReason.UnresolvedReasons = nil
	requireInvalid(t, "a match without an unresolved reason", withoutReason)

	emptyReason := complete
	emptyReason.UnresolvedReasons = []string{""}
	requireInvalid(t, "a match whose unresolved reason is empty", emptyReason)

	// 段階を 1 つも持たない関連付けは、どの段階が候補を挙げたかを読めない。
	withoutTallies := complete
	withoutTallies.StageTallies = nil
	requireInvalid(t, "a match carrying no stage tally", withoutTallies)

	// 候補を 1 件も持たない段階の関連付けは成り立たない。
	//
	// **要素 1 件の検査を通る値にしてから、関連付けの側の検査へ渡す。** 候補が 0 件の要素は
	// 理由を持つため、理由を空のままにすると要素の検査が先に退け、関連付けの側の
	// 「末尾の段階の候補は 1 件以上である」へ到達しない。
	withoutMember := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		tallies[len(tallies)-1].MemberCount = 0
		tallies[len(tallies)-1].DistinctProcessCount = nil
		tallies[len(tallies)-1].EmptyReason = core.EmptyReasonNoCandidateInWindow
	})
	requireInvalid(t, "a match whose stage counts no candidate", withoutMember)

	// 末尾の段階がこの関連付けを出した段階である。
	endingElsewhere := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		tallies[len(tallies)-1].StageKey = core.StageKeyClockIndependent
	})
	requireInvalid(t, "a match whose tallies end at another stage", endingElsewhere)

	// 時刻を比べない段階は、時刻の範囲の中に候補が無いという理由を持たない。実行していない比較の
	// 結果を示すことになる。
	windowReasonOnStageOne := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		tallies[0].MemberCount = 0
		tallies[0].DistinctProcessCount = nil
		tallies[0].EmptyReason = core.EmptyReasonNoCandidateInWindow
	})
	requireInvalid(t, "a match whose clock-independent stage blames the window",
		windowReasonOnStageOne)

	// 同じ段階が、その段階で起こりうる理由を持つ組は成り立つ。
	conditionReasonOnStageOne := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		tallies[0].MemberCount = 0
		tallies[0].DistinctProcessCount = nil
		tallies[0].EmptyReason = core.EmptyReasonNoCandidateMatchingConditions
	})
	if err := conditionReasonOnStageOne.Validate(); err != nil {
		t.Errorf("a match whose clock-independent stage blames the conditions did not "+
			"validate: %v", err)
	}

	// 候補が指すプロセスの個数は候補の総数を超えない。
	tooManyProcesses := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		tallies[len(tallies)-1].DistinctProcessCount = int64Ptr(3)
	})
	requireInvalid(t, "a match counting more processes than candidates", tooManyProcesses)

	// 組は要素を 2 件以上持つ。区別できない相手がいない 1 件の組を作らない。
	singleMemberGroup := complete
	singleMemberGroup.IndistinguishableGroups = [][]core.RecordLocator{{markIILocator(firstCandidateSn)}}
	requireInvalid(t, "a match carrying a group of one", singleMemberGroup)

	brokenGroup := complete
	brokenGroup.IndistinguishableGroups = [][]core.RecordLocator{{{}, {}}}
	requireInvalid(t, "a match carrying a broken locator in a group", brokenGroup)

	// 候補がプロセスの項目を持たない段階では、プロセスの個数が出ない。
	withoutProcesses := withStageTallies(complete, func(tallies []core.MatchStageTally) {
		for index := range tallies {
			tallies[index].DistinctProcessCount = nil
		}
	})
	withoutProcesses.IndistinguishableGroups = nil
	requireValid(t, "a match whose candidates carry no process", withoutProcesses)

	// 時計に依拠する箇所は必須である。段階 1 から引き継ぐ割当の条件が時計に依拠する。
	withoutNote := complete
	withoutNote.ClockDependencyNote = ""
	requireInvalid(t, "a match without a clock dependency note", withoutNote)
}

func TestNodeEdgeCountValidateSeparatesTheKnownDirectionFromTheUnknownOne(t *testing.T) {
	for _, direction := range []core.EdgeDirection{
		core.EdgeDirectionOutgoing, core.EdgeDirectionIncoming,
	} {
		if !direction.IsKnown() {
			t.Errorf("the direction %q is not known", direction)
		}
		requireValid(t, "a count carrying the direction "+string(direction),
			core.NodeEdgeCount{
				EdgeKind: core.EdgeKindRanOn, Direction: direction,
				EdgeCount: 1, EvidenceCount: 6,
			})
	}
	if core.EdgeDirection("both").IsKnown() {
		t.Error("an unknown direction is reported as known")
	}
	requireInvalid(t, "a count of an unknown direction", core.NodeEdgeCount{
		EdgeKind: core.EdgeKindRanOn, Direction: "both", EdgeCount: 1, EvidenceCount: 6,
	})
	requireInvalid(t, "a count of an unknown edge kind", core.NodeEdgeCount{
		EdgeKind: "logged_on", Direction: core.EdgeDirectionOutgoing,
		EdgeCount: 1, EvidenceCount: 6,
	})
	requireInvalid(t, "a count carrying a negative edge count", core.NodeEdgeCount{
		EdgeKind: core.EdgeKindRanOn, Direction: core.EdgeDirectionOutgoing,
		EdgeCount: -1, EvidenceCount: 6,
	})
}

// graphAttributeValue は検査を通る属性の値を返す。
func graphAttributeValue(t *testing.T) core.NodeAttributeValue {
	t.Helper()
	text, err := core.NewRawValue(core.ValueStatePresent, `C:\app.exe`)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField("psPath", core.SemanticKeyProcessBinaryPath, text)
	if err != nil {
		t.Fatal(err)
	}
	return core.NodeAttributeValue{
		Field: field, ObservationCount: 5, FirstRecordRef: graphRecordLocator(),
	}
}

func TestNodeAttributeValueValidateSeparatesTheCompleteValueFromTheBrokenOne(t *testing.T) {
	complete := graphAttributeValue(t)
	requireValid(t, "a complete attribute value", complete)

	brokenField := complete
	brokenField.Field = core.RecordField{Name: "psPath"}
	requireInvalid(t, "an attribute value carrying a broken field", brokenField)

	negativeCount := complete
	negativeCount.ObservationCount = -1
	requireInvalid(t, "an attribute value observed a negative number of times", negativeCount)

	brokenRef := complete
	brokenRef.FirstRecordRef = core.RecordLocator{}
	requireInvalid(t, "an attribute value without a record reference", brokenRef)
}

func TestNodeAttributeValidateSeparatesTheCountedValuesFromTheMiscountedOnes(t *testing.T) {
	value := graphAttributeValue(t)
	complete := core.NodeAttribute{
		Semantic: core.SemanticKeyProcessBinaryPath, ValueCount: 1,
		Values: []core.NodeAttributeValue{value},
	}
	requireValid(t, "an attribute carrying one value", complete)

	twoValues := core.NodeAttribute{
		Semantic: core.SemanticKeyProcessBinaryPath, ValueCount: 2,
		Values: []core.NodeAttributeValue{value, value},
	}
	requireValid(t, "an attribute carrying two values", twoValues)

	unknownSemantic := complete
	unknownSemantic.Semantic = "process.image"
	requireInvalid(t, "an attribute of an unknown semantic", unknownSemantic)

	withoutValues := complete
	withoutValues.Values = nil
	requireInvalid(t, "an attribute without a value", withoutValues)

	miscounted := complete
	miscounted.ValueCount = 2
	requireInvalid(t, "an attribute whose value count differs from its values", miscounted)

	brokenValue := complete
	broken := value
	broken.ObservationCount = -1
	brokenValue.Values = []core.NodeAttributeValue{broken}
	requireInvalid(t, "an attribute carrying a broken value", brokenValue)
}

// 属性は語彙の項目と原資料の key のちょうど一方で名前を持つ。
func TestNodeAttributeValidateRequiresOneOfTheSemanticAndTheName(t *testing.T) {
	value := graphAttributeValue(t)
	withSemantic := core.NodeAttribute{
		Semantic: core.SemanticKeyProcessBinaryPath, ValueCount: 1,
		Values: []core.NodeAttributeValue{value},
	}
	requireValid(t, "an attribute named by the vocabulary", withSemantic)

	withName := core.NodeAttribute{
		Name: "psProfile", ValueCount: 1, Values: []core.NodeAttributeValue{value},
	}
	requireValid(t, "an attribute named by the key of the raw material", withName)

	withBoth := withSemantic
	withBoth.Name = "psPath"
	requireInvalid(t, "an attribute carrying both names", withBoth)

	withNeither := withSemantic
	withNeither.Semantic = ""
	requireInvalid(t, "an attribute carrying neither name", withNeither)
}

func TestEdgeKindIsKnownSeparatesTheObservedKindsFromTheOthers(t *testing.T) {
	for _, kind := range []core.EdgeKind{
		core.EdgeKindRanOn, core.EdgeKindProcessParentChild, core.EdgeKindProcessInjection,
		core.EdgeKindFileOperation, core.EdgeKindFileCopy, core.EdgeKindFileContentMatch,
		core.EdgeKindRegistryOperation, core.EdgeKindProcessCommunication,
		core.EdgeKindTerminalAddress, core.EdgeKindTerminalRemoteSession,
		core.EdgeKindTerminalAccount, core.EdgeKindHttpRequest,
		core.EdgeKindCrossSourceConnectionMatch, core.EdgeKindRecordNamesObject,
		core.EdgeKindLogonSessionOperation, core.EdgeKindRecordSubjectAccount,
		core.EdgeKindRecordTargetAccount, core.EdgeKindArgumentNamesObject,
		core.EdgeKindTaskRegistrationRun, core.EdgeKindLinkedLogon, core.EdgeKindTicketRequestLogon,
		core.EdgeKindReverseLookupName, core.EdgeKindConnectionLogonMatch,
		core.EdgeKindProcessIdentityMatch, core.EdgeKindInboundConnectionMatch,
		core.EdgeKindSameConnectionMatch,
	} {
		if !kind.IsKnown() {
			t.Errorf("the edge kind %q is not known", kind)
		}
	}
	// 語彙に取り込んでいない関係は種別にならない。
	for _, kind := range []core.EdgeKind{"logged_on", "copied_to", ""} {
		if kind.IsKnown() {
			t.Errorf("the edge kind %q is reported as known", kind)
		}
	}
}

func TestNodeKeyLabelValueReturnsTheLastIdentityValue(t *testing.T) {
	key := core.NodeKey{
		Kind: core.NodeKindAccount, Form: core.NodeKeyFormAccountDomainName,
		Values: []core.NodeIdentityValue{
			{Semantic: core.SemanticKeyAccountDomain, Value: "AD"},
			{Semantic: core.SemanticKeyAccountName, Value: "alice"},
		},
	}
	value, present := key.LabelValue()
	if !present || value != "alice" {
		t.Errorf("the label value is %q (present %v), want alice", value, present)
	}
	if _, present := (core.NodeKey{Kind: core.NodeKindIp}).LabelValue(); present {
		t.Error("a key without a value returned a label value")
	}
}

// 候補は関係の状態に candidate だけを取る。語彙が持つ他の値を拒否する。
func TestCandidateValidateRejectsTheObservedRelationState(t *testing.T) {
	if !core.RelationStateObserved.IsKnown() || !core.RelationStateCandidate.IsKnown() {
		t.Fatal("the vocabulary does not know both relation states")
	}
	candidate := notComparedCandidate(t, secondCandidateSn)
	requireValid(t, "a candidate carrying the candidate state", candidate)

	observed := candidate
	observed.RelationState = core.RelationStateObserved
	requireInvalid(t, "a candidate carrying the observed state", observed)
}

// 一致した欄は、語彙の項目と原資料の key のちょうど一方で名前を持ち、値の形が契約の中にある。
func TestNodeValueMatchValidateSeparatesTheNamedMatchFromTheBrokenOne(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		match core.NodeValueMatch
		valid bool
	}{
		{
			"a match named by the vocabulary",
			core.NodeValueMatch{
				Semantic: core.SemanticKeyProcessCommandLine,
				Form:     core.ValueMatchFormRawText,
			}, true,
		},
		{
			"a match named by the key of the raw material",
			core.NodeValueMatch{Name: "psProfile", Form: core.ValueMatchFormNormalized}, true,
		},
		{
			"a match carrying both names",
			core.NodeValueMatch{
				Semantic: core.SemanticKeyProcessCommandLine, Name: "cmd",
				Form: core.ValueMatchFormRawText,
			}, false,
		},
		{
			"a match carrying neither name",
			core.NodeValueMatch{Form: core.ValueMatchFormRawText}, false,
		},
		{
			"a match of an unknown semantic",
			core.NodeValueMatch{Semantic: "process.image", Form: core.ValueMatchFormRawText},
			false,
		},
		{
			"a match of a form outside the contract",
			core.NodeValueMatch{Name: "psProfile", Form: "comparable_value"}, false,
		},
	} {
		if testCase.valid {
			requireValid(t, testCase.name, testCase.match)
			continue
		}
		requireInvalid(t, testCase.name, testCase.match)
	}
}

// 値の形は定義の中の 2 値だけを既知にする。
func TestValueMatchFormIsKnownSeparatesTheContractFromTheOthers(t *testing.T) {
	for _, form := range []core.ValueMatchForm{
		core.ValueMatchFormRawText, core.ValueMatchFormNormalized,
	} {
		if !form.IsKnown() {
			t.Errorf("the value match form %q is not known", form)
		}
	}
	for _, form := range []core.ValueMatchForm{"", "comparable_value", "identity"} {
		if form.IsKnown() {
			t.Errorf("the value match form %q is known", form)
		}
	}
}

// graphValueCount は検査を通る、値ごとの件数を組む。
func graphValueCount(t *testing.T) core.ValueCount {
	t.Helper()
	return core.ValueCount{Value: "ExampleClient/1.0", RecordCount: 1}
}

// 時刻の差の分布は、差を取ったレコードの件数と、四分位の順と、階級の件数の和を揃えて持つ。
func TestValueCountValidateChecksTheIntervals(t *testing.T) {
	valid := func() core.ValueCount {
		bins := make([]int64, len(core.IntervalBoundsMilliseconds)+1)
		bins[5] = 2
		return core.ValueCount{Value: "a.example.test", RecordCount: 3, Intervals: &core.EventIntervals{
			TimedRecordCount: 3, MinMilliseconds: 60_000, LowerQuartileMilliseconds: 60_000,
			MedianMilliseconds: 60_000, UpperQuartileMilliseconds: 65_000, MaxMilliseconds: 65_000,
			BinCounts: bins,
		}}
	}
	requireValid(t, "consistent intervals", valid())
	for name, broken := range map[string]func(*core.EventIntervals){
		"more timed records than records": func(i *core.EventIntervals) { i.TimedRecordCount = 4 },
		"one timed record":                func(i *core.EventIntervals) { i.TimedRecordCount = 1 },
		"quartiles out of order":          func(i *core.EventIntervals) { i.MedianMilliseconds = 70_000 },
		"a negative minimum":              func(i *core.EventIntervals) { i.MinMilliseconds = -1 },
		"bins not summing to the gaps":    func(i *core.EventIntervals) { i.BinCounts[0] = 1 },
		"bins of another length":          func(i *core.EventIntervals) { i.BinCounts = i.BinCounts[:3] },
	} {
		count := valid()
		broken(count.Intervals)
		requireInvalid(t, name, count)
	}
}

// 値ごとの件数は、時刻の両端を揃って持つか、どちらも持たない。
func TestValueCountValidateSeparatesTheCompleteBoundsFromTheHalfOnes(t *testing.T) {
	complete := graphValueCount(t)
	requireValid(t, "a value without time bounds", complete)

	bound := markIIEventTime(t)
	withBounds := complete
	withBounds.FirstEventTime, withBounds.LastEventTime = &bound, &bound
	requireValid(t, "a value with both time bounds", withBounds)

	firstOnly := complete
	firstOnly.FirstEventTime = &bound
	requireInvalid(t, "a value with only the first bound", firstOnly)

	lastOnly := complete
	lastOnly.LastEventTime = &bound
	requireInvalid(t, "a value with only the last bound", lastOnly)

	withoutValue := complete
	withoutValue.Value = ""
	requireInvalid(t, "a value without the observed text", withoutValue)

	negative := complete
	negative.RecordCount = -1
	requireInvalid(t, "a value observed by a negative number of records", negative)

	// 1 件も観測していない値は、その値を返さないことで表す。件数 0 そのものは検査を通る。
	noRecord := complete
	noRecord.RecordCount = 0
	requireValid(t, "a value observed by no record", noRecord)
}

// 区分を指す値は、ログオンの種別の値と、種別を持たないことを表す印のどちらか一方を持つ。
func TestEdgeEvidenceSelectorTakesEitherTheLogonTypeOrItsAbsence(t *testing.T) {
	base := core.EdgeEvidenceSelector{
		EventCategory: "os", EventAction: "evtLog", DestinationPortAbsent: true,
	}
	withType := base
	withType.LogonType = "3"
	requireValid(t, "a selector with a logon type", withType)
	withAbsence := base
	withAbsence.LogonTypeAbsent = true
	requireValid(t, "a selector of the records without a logon type", withAbsence)

	requireInvalid(t, "a selector without the logon type and its absence", base)
	both := withType
	both.LogonTypeAbsent = true
	requireInvalid(t, "a selector with the logon type and its absence", both)
}

// 区分はログオンの種別の欄と、欄が出ない理由のどちらか一方を持つ。
func TestEdgeEvidenceGroupTakesEitherTheLogonTypeOrTheReason(t *testing.T) {
	logonType, err := core.NewTextField("LogonType", core.SemanticKeyEventLogonType,
		core.RawAndNormalized{RawText: new("3"), ValueState: core.ValueStatePresent})
	if err != nil {
		t.Fatal(err)
	}
	base := core.EdgeEvidenceGroup{
		ObservationKind:        core.ObservationKind{Raw: []core.RecordField{}},
		DestinationPortAbsence: "no destination port",
		SelectorAbsence:        "no selector",
		Accounts:               []core.EdgeEvidenceAccount{},
		EvidenceCount:          1,
	}
	withType := base
	withType.LogonType = &logonType
	requireValid(t, "a group with a logon type", withType)
	withReason := base
	withReason.LogonTypeAbsence = "no logon type"
	requireValid(t, "a group with the reason the logon type is absent", withReason)

	requireInvalid(t, "a group without the logon type and the reason", base)
	both := withType
	both.LogonTypeAbsence = "no logon type"
	requireInvalid(t, "a group with the logon type and the reason", both)
}
