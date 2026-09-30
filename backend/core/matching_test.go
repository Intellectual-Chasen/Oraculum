package core_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// otherPortCandidateSn は接続先 port が起点と異なる候補のレコードの通番である。
// このレコードを段階 1 の候補に数えない。
const otherPortCandidateSn = int64(900340)

// 2 台目のプロセス。段階 1 の候補のうち遅い時刻の 2 件が指す。
const secondMarkIIProcessId = "{55555555-6666-7777-8888-999999999999}"

// matchingManifest は fixture と、その期待値を持つ。
// 期待値を test で計算し直さず、manifest に書き写した値を使う。
type matchingManifest struct {
	Case struct {
		Records []matchingRecord `json:"records"`
		Stages  []matchingStage  `json:"stages"`
		Windows []matchingWindow `json:"windows"`
	} `json:"case"`
}

// matchingRecord は候補のレコード 1 件である。
type matchingRecord struct {
	SequenceNumber                  int64  `json:"sequenceNumber"`
	SubEvent                        string `json:"subEvt"`
	DestinationPort                 string `json:"dstPort"`
	ProcessId                       string `json:"psGUID"`
	HeaderTimeText                  string `json:"headerTimeText"`
	EventTime                       string `json:"eventTime"`
	CountedInClockIndependentStage  bool   `json:"countedInClockIndependentStage"`
	CountedInSecondTimeMatchedStage bool   `json:"countedInSecondTimeMatchedStage"`
}

// matchingStage は段階 1 つ分の期待値である。
type matchingStage struct {
	StageKey                         string   `json:"stageKey"`
	ComparisonUnit                   string   `json:"comparisonUnit"`
	WindowKind                       string   `json:"windowKind"`
	UsedConditionKeys                []string `json:"usedConditionKeys"`
	MemberCount                      int64    `json:"memberCount"`
	DistinctProcessCount             int64    `json:"distinctProcessCount"`
	IndistinguishableGroupCount      int64    `json:"indistinguishableGroupCount"`
	LargestGroupSize                 int      `json:"largestGroupSize"`
	UndeterminedObservationKindCount int      `json:"undeterminedObservationKindCount"`
	DeterminedObservationKindCount   int      `json:"determinedObservationKindCount"`
}

// matchingWindow は時刻の範囲 1 つ分の期待値である。段階 2 が取る候補は時刻の範囲ごとに変わる。
type matchingWindow struct {
	Name                  string              `json:"name"`
	WindowKind            string              `json:"windowKind"`
	LowerBound            *matchingWindowTime `json:"lowerBound"`
	UpperBound            *matchingWindowTime `json:"upperBound"`
	CenterTime            *matchingWindowTime `json:"centerTime"`
	RadiusSeconds         *int64              `json:"radiusSeconds"`
	MemberSequenceNumbers []int64             `json:"memberSequenceNumbers"`
}

// matchingWindowTime は時刻の範囲の端に要求が与えた文字列と、その正規化値である。
type matchingWindowTime struct {
	RequestText string `json:"requestText"`
	Normalized  string `json:"normalized"`
}

// loadMatchingManifest は fixture と期待値を読む。
func loadMatchingManifest(t *testing.T) matchingManifest {
	t.Helper()
	data, err := os.ReadFile("testdata/matching/manifest.json")
	if err != nil {
		t.Fatalf("reading the matching manifest: %v", err)
	}
	var manifest matchingManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decoding the matching manifest: %v", err)
	}
	if len(manifest.Case.Records) == 0 || len(manifest.Case.Stages) != 2 ||
		len(manifest.Case.Windows) == 0 {
		t.Fatalf("the matching manifest carries %d records, %d stages and %d windows",
			len(manifest.Case.Records), len(manifest.Case.Stages), len(manifest.Case.Windows))
	}
	return manifest
}

// expectedStage は manifest から段階の期待値を 1 件取り出す。
func expectedStage(t *testing.T, manifest matchingManifest, stageKey core.StageKey) matchingStage {
	t.Helper()
	for _, stage := range manifest.Case.Stages {
		if stage.StageKey == string(stageKey) {
			return stage
		}
	}
	t.Fatalf("the matching manifest carries no %s stage", stageKey)
	return matchingStage{}
}

// expectedSequenceNumbers は manifest から段階に入る候補の通番を並びで返す。
func expectedSequenceNumbers(manifest matchingManifest, stageKey core.StageKey) []int64 {
	numbers := make([]int64, 0, len(manifest.Case.Records))
	for _, record := range manifest.Case.Records {
		counted := record.CountedInClockIndependentStage
		if stageKey == core.StageKeySecondTimeMatched {
			counted = record.CountedInSecondTimeMatchedStage
		}
		if counted {
			numbers = append(numbers, record.SequenceNumber)
		}
	}
	return numbers
}

// matchOrigin は Squid のレコードを起点にした関連付けの起点を返す。
func matchOrigin(t *testing.T) core.MatchOrigin {
	t.Helper()
	return core.MatchOrigin{
		Observation:          matchOriginObservation(t, originDestinationIpField(t)),
		DestinationIpUse:     core.ConditionUseUsed,
		AssignmentValidRange: markIIAssignment(t).AssignmentValidRange,
	}
}

// originDestinationIpField は `%ru` の authority の host から導いた接続先 IP の項目を返す。
// host が IP アドレスであるため、語彙の項目は connection.destination_address である。
func originDestinationIpField(t *testing.T) core.RecordField {
	t.Helper()
	return semanticDerivedText(t, "requestTargetHost",
		core.SemanticKeyConnectionDestinationAddress, proxyTargetIp, proxyTargetIp,
		"%ru の authority から userinfo と port と IPv6 の角括弧を外した host")
}

// matchOriginObservation は起点のレコード 1 件の観測を返す。
//
// Squid の combined に client 側の port の欄が無いため、clientPort を item_absent で持つ。
// 段階 1 が比べるのは clientTerminal の terminal.id で、値は markii 形式の tmid と同じ
// 外部識別子である。
func matchOriginObservation(t *testing.T, destinationIp core.RecordField) core.MatchObservation {
	t.Helper()
	return core.MatchObservation{
		Ref:       squidLocator(squidLineNumber),
		EventTime: squidRequestTime(t),
		// Squid の 12 項目に観測の種別の欄が無いため、raw は要素数 0 である。
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields: []core.RecordField{
			semanticTimestampField(t, "requestTime", core.SemanticKeyEventTime,
				squidRequestTime(t)),
			semanticDerivedText(t, "clientTerminal", core.SemanticKeyTerminalId,
				markIIClientIp, markIITerminalId, "接続元 IP に IP から端末への割当を適用した"),
			semanticDerivedText(t, "clientTerminalName", core.SemanticKeyTerminalHostname,
				markIITerminalId, markIITerminalName, "端末の外部識別子から表示名を求めた"),
			destinationIp,
			semanticDerivedText(t, "requestTargetPort",
				core.SemanticKeyConnectionDestinationPort, "http://"+proxyTargetIp+"/", "80",
				"%ru の scheme http から port を導いた"),
			absentItemText(t, "clientPort"),
		},
	}
}

// matchCandidate は候補のレコード 1 件を返す。時刻は通番ごとに決まる。
func matchCandidate(t *testing.T, sequenceNumber int64, subEvent, destinationPort, processId string) core.MatchCandidateRecord {
	t.Helper()
	return matchCandidateAt(t, sequenceNumber, subEvent, destinationPort, processId,
		markIICandidateTime(t, sequenceNumber))
}

// matchCandidateAt は時刻を与えて候補のレコード 1 件を返す。
func matchCandidateAt(t *testing.T, sequenceNumber int64, subEvent, destinationPort, processId string,
	eventTime core.Timestamp,
) core.MatchCandidateRecord {
	t.Helper()
	process := core.ProcessRef{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		ProcessId:           processId,
		TerminalId:          markIITerminalId,
	}
	return core.MatchCandidateRecord{
		Observation: core.MatchObservation{
			Ref:             markIILocator(sequenceNumber),
			EventTime:       eventTime,
			ObservationKind: markIIObservationKind(t, subEvent),
			// tmid は `/api/v0/records` の fields に無く、関連付けの観測には入る。
			Fields: []core.RecordField{
				semanticText(t, "tmid", core.SemanticKeyTerminalId, markIITerminalId),
				semanticText(t, "com", core.SemanticKeyTerminalHostname, markIITerminalName),
				semanticText(t, "dstIP", core.SemanticKeyConnectionDestinationAddress,
					proxyTargetIp),
				semanticText(t, "dstPort", core.SemanticKeyConnectionDestinationPort,
					destinationPort),
			},
		},
		ProcessRef:             &process,
		FailedRecordDependency: false,
		UnresolvedReasons:      nil,
	}
}

// positiveCandidates は manifest が持つレコードを候補として返す。
func positiveCandidates(t *testing.T) []core.MatchCandidateRecord {
	t.Helper()
	manifest := loadMatchingManifest(t)
	records := make([]core.MatchCandidateRecord, 0, len(manifest.Case.Records))
	for _, record := range manifest.Case.Records {
		records = append(records, matchCandidateAt(t, record.SequenceNumber, record.SubEvent,
			record.DestinationPort, record.ProcessId,
			markIIHeaderTime(t, record.HeaderTimeText, record.EventTime)))
	}
	return records
}

// matchRequest は 2 つの段階を求める要求を返す。
func matchRequest(t *testing.T, candidates []core.MatchCandidateRecord) core.MatchRequest {
	t.Helper()
	return core.MatchRequest{
		Origin:                   matchOrigin(t),
		Candidates:               candidates,
		Conditions:               matchConditionSpecs(core.ConditionUseUsed),
		CounterpartItemSemantics: counterpartItemSemantics(),
		CounterpartTimePrecision: core.PrecisionMillisecond,
		StageKeys:                []core.StageKey{core.StageKeyClockIndependent, core.StageKeySecondTimeMatched},
		IncludedObservationKinds: includedObservationKinds(),
		SecondStageWindow:        sameSecondWindow(),
		SecondStageAssumptions:   []core.MatchAssumption{clockOffsetAssumption()},
		ClockDependencyNote: "割当の適用期間の判定が Proxy と " + markIITerminalName +
			" の時計のずれに依拠する",
		PublicationState:      core.PublicationStatePublishedFull,
		UnprocessedRanges:     []core.RecordRange{},
		UnprocessedRangeCount: 0,
		AnalysisRunRef:        "analysis-run-1",
	}
}

// replaceOriginField は起点の観測の項目を name で差し替える。
func replaceOriginField(
	t *testing.T, request *core.MatchRequest, name string, replacement core.RecordField,
) {
	t.Helper()
	for index, field := range request.Origin.Observation.Fields {
		if field.Name == name {
			request.Origin.Observation.Fields[index] = replacement
			return
		}
	}
	t.Fatalf("the origin observation carries no field named %s", name)
}

// candidateWithFields は項目を差し替えた候補を 1 件返す。時刻と観測の種別は段階 2 まで
// 通る値を保つ。
func candidateWithFields(t *testing.T, fields []core.RecordField) core.MatchCandidateRecord {
	t.Helper()
	record := matchCandidate(t, firstCandidateSn, "con", "80", markIIProcessId)
	record.Observation.Fields = fields
	return record
}

// candidateDestinationFields は候補の接続先の 2 項目を返す。端末の項目だけを変える
// test が使う。
func candidateDestinationFields(t *testing.T) []core.RecordField {
	t.Helper()
	return []core.RecordField{
		semanticText(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, proxyTargetIp),
		semanticText(t, "dstPort", core.SemanticKeyConnectionDestinationPort, "80"),
	}
}

// stageByKey は候補集合から段階を 1 つ取り出す。
func stageByKey(t *testing.T, set core.CandidateSet, stageKey core.StageKey) core.CandidateStage {
	t.Helper()
	for _, stage := range set.Stages {
		if stage.StageKey == stageKey {
			return stage
		}
	}
	t.Fatalf("the candidate set carries no %s stage", stageKey)
	return core.CandidateStage{}
}

// usedConditionKeys は段階が用いた条件の種別を、段階の並びで返す。
func usedConditionKeys(stage core.CandidateStage) []string {
	keys := make([]string, 0, len(stage.Conditions))
	for _, condition := range stage.Conditions {
		if condition.Use == core.ConditionUseUsed {
			keys = append(keys, string(condition.ConditionKey))
		}
	}
	return keys
}

// conditionByKey は段階の条件を 1 つ取り出す。
func conditionByKey(t *testing.T, stage core.CandidateStage, key core.ConditionKey) core.MatchCondition {
	t.Helper()
	for _, condition := range stage.Conditions {
		if condition.ConditionKey == key {
			return condition
		}
	}
	t.Fatalf("the %s stage carries no %s condition", stage.StageKey, key)
	return core.MatchCondition{}
}

// sequenceNumbersOf は候補のレコードの通番を並びで返す。
func sequenceNumbersOf(t *testing.T, members []core.Candidate) []int64 {
	t.Helper()
	numbers := make([]int64, 0, len(members))
	for _, member := range members {
		if member.RecordRef.SequenceNumber == nil {
			t.Fatalf("a member carries no sequence number")
		}
		numbers = append(numbers, *member.RecordRef.SequenceNumber)
	}
	return numbers
}

// buildSet は要求から候補集合を組む。
func buildSet(t *testing.T, request core.MatchRequest) core.CandidateSet {
	t.Helper()
	set, err := core.BuildCandidateSet(request)
	if err != nil {
		t.Fatalf("building the candidate set: %v", err)
	}
	if problem := set.Validate(); problem != nil {
		t.Fatalf("the built candidate set does not satisfy the contract: %v", problem)
	}
	return set
}

func TestBuildCandidateSetCountsBothStages(t *testing.T) {
	manifest := loadMatchingManifest(t)
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))

	if len(set.Stages) != len(manifest.Case.Stages) {
		t.Fatalf("the candidate set carries %d stages, want %d",
			len(set.Stages), len(manifest.Case.Stages))
	}
	if set.EmptyReason != "" {
		t.Fatalf("the candidate set carries the emptyReason %q, want none", set.EmptyReason)
	}
	if set.AnalysisRunRef != "analysis-run-1" {
		t.Errorf("analysisRunRef is %q, want %q", set.AnalysisRunRef, "analysis-run-1")
	}

	for _, want := range manifest.Case.Stages {
		stage := stageByKey(t, set, core.StageKey(want.StageKey))
		if stage.MemberCount != want.MemberCount {
			t.Errorf("the %s memberCount is %d, want %d",
				want.StageKey, stage.MemberCount, want.MemberCount)
		}
		if stage.DistinctProcessCount == nil || *stage.DistinctProcessCount != want.DistinctProcessCount {
			t.Errorf("the %s distinctProcessCount is %v, want %d",
				want.StageKey, stage.DistinctProcessCount, want.DistinctProcessCount)
		}
		if got := usedConditionKeys(stage); !reflect.DeepEqual(got, want.UsedConditionKeys) {
			t.Errorf("the %s stage uses the conditions %v, want %v",
				want.StageKey, got, want.UsedConditionKeys)
		}
		if string(stage.ComparisonUnit) != want.ComparisonUnit {
			t.Errorf("the %s comparisonUnit is %q, want %q",
				want.StageKey, stage.ComparisonUnit, want.ComparisonUnit)
		}
		if string(stage.TimeWindow.WindowKind) != want.WindowKind {
			t.Errorf("the %s windowKind is %q, want %q",
				want.StageKey, stage.TimeWindow.WindowKind, want.WindowKind)
		}
		wantMembers := expectedSequenceNumbers(manifest, core.StageKey(want.StageKey))
		if got := sequenceNumbersOf(t, stage.Members); !equalInt64Slice(got, wantMembers) {
			t.Errorf("the %s members are %v, want %v", want.StageKey, got, wantMembers)
		}
	}
}

// equalInt64Slice は 2 つの並びが同じ長さと同じ値を持つかを返す。
func equalInt64Slice(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// equalRecordRefs は 2 つの候補の並びが同じ recordRef の集合を持つかを返す。
// RecordLocator は pointer の項目を持つため、直列化した文字列で数える。
func equalRecordRefs(t *testing.T, got, want []core.Candidate) bool {
	t.Helper()
	return maps.Equal(recordRefCounts(t, got), recordRefCounts(t, want))
}

// recordRefCounts は候補のレコードの位置を、直列化した文字列ごとに数える。
func recordRefCounts(t *testing.T, members []core.Candidate) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(members))
	for _, member := range members {
		encoded, err := json.Marshal(member.RecordRef)
		if err != nil {
			t.Fatalf("encoding the recordRef of a member: %v", err)
		}
		counts[string(encoded)]++
	}
	return counts
}

func TestBuildCandidateSetKeepsStagesIndependent(t *testing.T) {
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))
	first := stageByKey(t, set, core.StageKeyClockIndependent)
	second := stageByKey(t, set, core.StageKeySecondTimeMatched)

	// 段階 2 の要素は段階 1 の差分ではなく、それ自体で完結した集合である。
	for _, member := range second.Members {
		if member.TimeComparison.ComparisonUnit != core.ComparisonUnitSecond {
			t.Errorf("a second_time_matched member compares time in %q, want %q",
				member.TimeComparison.ComparisonUnit, core.ComparisonUnitSecond)
		}
		if member.TimeComparison.RightTime == nil {
			t.Fatalf("a second_time_matched member carries no rightTime")
		}
		if !member.TimeComparison.RightTime.Equal(member.EventTime) {
			t.Errorf("a second_time_matched member rightTime differs from its eventTime")
		}
		if len(member.TimeComparison.Assumptions) != 1 {
			t.Errorf("a second_time_matched member carries %d assumptions, want 1",
				len(member.TimeComparison.Assumptions))
		}
	}
	for _, member := range first.Members {
		if member.TimeComparison.ComparisonUnit != core.ComparisonUnitNotCompared {
			t.Errorf("a clock_independent member compares time in %q, want %q",
				member.TimeComparison.ComparisonUnit, core.ComparisonUnitNotCompared)
		}
		if member.TimeComparison.LeftTime != nil || member.TimeComparison.RightTime != nil {
			t.Errorf("a clock_independent member carries compared times")
		}
		if member.RelationState != core.RelationStateCandidate {
			t.Errorf("a member carries the relationState %q, want %q",
				member.RelationState, core.RelationStateCandidate)
		}
		if len(member.UnresolvedReasons) == 0 {
			t.Errorf("a member carries no unresolvedReasons")
		}
	}
}

func TestBuildCandidateSetGroupsIndistinguishableMembers(t *testing.T) {
	manifest := loadMatchingManifest(t)
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))

	for _, want := range manifest.Case.Stages {
		stage := stageByKey(t, set, core.StageKey(want.StageKey))
		if stage.IndistinguishableGroupCount != want.IndistinguishableGroupCount {
			t.Fatalf("the %s indistinguishableGroupCount is %d, want %d",
				want.StageKey, stage.IndistinguishableGroupCount, want.IndistinguishableGroupCount)
		}
		if int64(len(stage.IndistinguishableGroups)) != want.IndistinguishableGroupCount {
			t.Fatalf("the %s stage carries %d groups, want %d",
				want.StageKey, len(stage.IndistinguishableGroups), want.IndistinguishableGroupCount)
		}
		if len(stage.IndistinguishableGroups[0]) != want.LargestGroupSize {
			t.Errorf("the %s group carries %d members, want %d",
				want.StageKey, len(stage.IndistinguishableGroups[0]), want.LargestGroupSize)
		}
	}
}

func TestBuildCandidateSetOmitsSingleMemberGroup(t *testing.T) {
	candidates := []core.MatchCandidateRecord{
		matchCandidate(t, firstCandidateSn, "con", "80", markIIProcessId),
	}
	set := buildSet(t, matchRequest(t, candidates))
	first := stageByKey(t, set, core.StageKeyClockIndependent)

	if first.MemberCount != 1 {
		t.Fatalf("the clock_independent memberCount is %d, want 1", first.MemberCount)
	}
	if first.IndistinguishableGroupCount != 0 {
		t.Errorf("the clock_independent indistinguishableGroupCount is %d, want 0",
			first.IndistinguishableGroupCount)
	}
	if len(first.IndistinguishableGroups) != 0 {
		t.Errorf("the clock_independent stage carries %d groups, want 0",
			len(first.IndistinguishableGroups))
	}
	if first.Members[0].RelationState != core.RelationStateCandidate {
		t.Errorf("the single member carries the relationState %q, want %q",
			first.Members[0].RelationState, core.RelationStateCandidate)
	}
}

// 段階は要求が宣言した条件の全数を、宣言と同じ conditionKey で持つ。
func TestBuildCandidateSetCarriesEveryDeclaredCondition(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	want := make([]core.ConditionKey, 0, len(request.Conditions))
	for _, spec := range request.Conditions {
		want = append(want, spec.ConditionKey)
	}
	set := buildSet(t, request)

	for _, stage := range set.Stages {
		got := make([]core.ConditionKey, 0, len(stage.Conditions))
		for _, condition := range stage.Conditions {
			got = append(got, condition.ConditionKey)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the %s stage carries the conditionKeys %v, want %v",
				stage.StageKey, got, want)
		}
	}
	if !reflect.DeepEqual(set.Stages[0].IncludedObservationKinds, includedObservationKinds()) {
		t.Errorf("the clock_independent stage selects %#v, want %#v",
			set.Stages[0].IncludedObservationKinds, includedObservationKinds())
	}
}

// 絞りに用いない条件の use は、相手の収集元が欄を持つかで決まる。
func TestBuildCandidateSetDecidesTheUseFromTheCounterpartColumns(t *testing.T) {
	cases := map[string]struct {
		conditionKey core.ConditionKey
		carried      bool
		want         core.ConditionUse
	}{
		"相手が接続元 port の欄を持つ": {
			core.ConditionKeyClientPort, true, core.ConditionUseNotUsed,
		},
		"相手が接続元 port の欄を持たない": {
			core.ConditionKeyClientPort, false, core.ConditionUseItemAbsentOnCounterpart,
		},
		"相手がプロセスの欄を持つ": {
			core.ConditionKeyProcess, true, core.ConditionUseNotUsed,
		},
		"相手がプロセスの欄を持たない": {
			core.ConditionKeyProcess, false, core.ConditionUseItemAbsentOnCounterpart,
		},
		"相手が利用者の欄を持つ": {
			core.ConditionKeyUser, true, core.ConditionUseNotUsed,
		},
		"相手が利用者の欄を持たない": {
			core.ConditionKeyUser, false, core.ConditionUseItemAbsentOnCounterpart,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			if !want.carried {
				request.CounterpartItemSemantics = withoutConditionSemantics(
					request, want.conditionKey)
			}
			set := buildSet(t, request)

			for _, stage := range set.Stages {
				if got := conditionByKey(t, stage, want.conditionKey).Use; got != want.want {
					t.Errorf("the %s use of the %s stage is %q, want %q",
						want.conditionKey, stage.StageKey, got, want.want)
				}
			}
		})
	}
}

// 秒未満の時刻の use は両側の精度で決まる。片側が秒までなら比べる相手が無い。
func TestBuildCandidateSetDecidesTheSubSecondUseFromBothPrecisions(t *testing.T) {
	cases := map[string]struct {
		origin      core.Precision
		counterpart core.Precision
		want        core.ConditionUse
	}{
		"両側がミリ秒まで持つ": {
			core.PrecisionMillisecond, core.PrecisionMillisecond, core.ConditionUseNotUsed,
		},
		"起点が秒までしか持たない": {
			core.PrecisionSecond, core.PrecisionMillisecond,
			core.ConditionUseNoComparableCounterpart,
		},
		"相手が秒までしか持たない": {
			core.PrecisionMillisecond, core.PrecisionSecond,
			core.ConditionUseNoComparableCounterpart,
		},
		"両側が秒までしか持たない": {
			core.PrecisionSecond, core.PrecisionSecond,
			core.ConditionUseNoComparableCounterpart,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			if want.origin == core.PrecisionMillisecond {
				withMillisecondOriginTime(t, &request)
			}
			request.CounterpartTimePrecision = want.counterpart
			set := buildSet(t, request)

			for _, stage := range set.Stages {
				got := conditionByKey(t, stage, core.ConditionKeySubSecondOfTime).Use
				if got != want.want {
					t.Errorf("the sub_second_of_time use of the %s stage is %q, want %q",
						stage.StageKey, got, want.want)
				}
			}
		})
	}
}

// withMillisecondOriginTime は起点のレコードの時刻を、秒未満まで書く精度へ差し替える。
func withMillisecondOriginTime(t *testing.T, request *core.MatchRequest) {
	t.Helper()
	eventTime := newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("[14/Mar/2024:10:20:30.500 +0900]"),
		Normalized:     stringPtr("2024-03-14T10:20:30.500+09:00"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningRecordOutput,
		ValueState:     core.ValueStatePresent,
	})
	request.Origin.Observation.EventTime = eventTime
	replaceOriginField(t, request, "requestTime",
		semanticTimestampField(t, "requestTime", core.SemanticKeyEventTime, eventTime))
}

// 用いていない条件の leftValue に、値を持たない要素を出さない。
//
// Squid の combined に接続元 port の欄が無く、起点は clientPort を item_absent の要素で
// 持つ。その要素を leftValue に出すと、起点がこの条件の値を持つという主張になる。
func TestBuildCandidateSetOmitsTheLeftValueOfAnAbsentItem(t *testing.T) {
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))

	for _, stage := range set.Stages {
		condition := conditionByKey(t, stage, core.ConditionKeyClientPort)
		if condition.Use == core.ConditionUseUsed {
			t.Fatalf("the client_port use of the %s stage is used", stage.StageKey)
		}
		if len(condition.LeftValue) != 0 {
			t.Errorf("the client_port condition of the %s stage carries the leftValue %v",
				stage.StageKey, condition.LeftValue)
		}
	}
}

// 値を持つ項目は、用いていない条件でも leftValue に出す。分析者が起点の値を読める。
func TestBuildCandidateSetCarriesTheLeftValueOfAPresentItem(t *testing.T) {
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))

	for _, stage := range set.Stages {
		condition := conditionByKey(t, stage, core.ConditionKeySecondOfTime)
		if len(condition.LeftValue) != 1 {
			t.Errorf("the second_of_time condition of the %s stage carries %d leftValue elements, want 1",
				stage.StageKey, len(condition.LeftValue))
		}
	}
}

// 条件を 1 件足す作業で core の定数を書き換えずに済む。
func TestBuildCandidateSetCarriesAnAddedCondition(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	request.Conditions = append(request.Conditions, core.MatchConditionSpec{
		ConditionKey:         core.ConditionKeyDestinationAuthority,
		OriginSemantic:       core.SemanticKeyConnectionDestinationHostname,
		CounterpartSemantics: []core.SemanticKey{core.SemanticKeyConnectionDestinationHostname},
	})
	set := buildSet(t, request)

	for _, stage := range set.Stages {
		got := conditionByKey(t, stage, core.ConditionKeyDestinationAuthority).Use
		if got != core.ConditionUseItemAbsentOnCounterpart {
			t.Errorf("the destination_authority use of the %s stage is %q, want %q",
				stage.StageKey, got, core.ConditionUseItemAbsentOnCounterpart)
		}
	}
}

// withoutConditionSemantics は、宣言が挙げた語彙の項目を相手の欄の一覧から外して返す。
func withoutConditionSemantics(
	request core.MatchRequest, conditionKey core.ConditionKey,
) []core.SemanticKey {
	dropped := make(map[core.SemanticKey]struct{})
	for _, spec := range request.Conditions {
		if spec.ConditionKey != conditionKey {
			continue
		}
		for _, semantic := range spec.CounterpartSemantics {
			dropped[semantic] = struct{}{}
		}
	}
	kept := make([]core.SemanticKey, 0, len(request.CounterpartItemSemantics))
	for _, semantic := range request.CounterpartItemSemantics {
		if _, drop := dropped[semantic]; drop {
			continue
		}
		kept = append(kept, semantic)
	}
	return kept
}

func TestBuildCandidateSetGroupsThreeIndistinguishableCandidates(t *testing.T) {
	candidates := []core.MatchCandidateRecord{
		matchCandidate(t, firstCandidateSn, "con", "80", markIIProcessId),
		matchCandidate(t, secondCandidateSn, "con", "80", markIIProcessId),
		matchCandidate(t, ancestorCandidateSn, "con", "80", markIIProcessId),
	}
	set := buildSet(t, matchRequest(t, candidates))
	second := stageByKey(t, set, core.StageKeySecondTimeMatched)

	if second.MemberCount != int64(len(candidates)) {
		t.Fatalf("the second_time_matched memberCount is %d, want %d",
			second.MemberCount, len(candidates))
	}
	if second.IndistinguishableGroupCount != 1 {
		t.Fatalf("the second_time_matched indistinguishableGroupCount is %d, want 1",
			second.IndistinguishableGroupCount)
	}
	if len(second.IndistinguishableGroups[0]) != len(candidates) {
		t.Errorf("the second_time_matched group carries %d members, want %d",
			len(second.IndistinguishableGroups[0]), len(candidates))
	}
	for _, member := range second.Members {
		if member.RelationState != core.RelationStateCandidate {
			t.Errorf("a member of an indistinguishable group carries the relationState %q, want %q",
				member.RelationState, core.RelationStateCandidate)
		}
	}
}

func TestBuildCandidateSetKeepsUndeterminedObservationKind(t *testing.T) {
	want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))
	first := stageByKey(t, set, core.StageKeyClockIndependent)

	statuses := make(map[core.ObservationKindStatus]int)
	for _, member := range first.Members {
		statuses[member.ObservationKind.Status]++
	}
	if statuses[core.ObservationKindStatusUndetermined] != want.UndeterminedObservationKindCount {
		t.Errorf("the clock_independent stage carries %d members whose observation kind is undetermined, want %d",
			statuses[core.ObservationKindStatusUndetermined], want.UndeterminedObservationKindCount)
	}
	if statuses[core.ObservationKindStatusDetermined] != want.DeterminedObservationKindCount {
		t.Errorf("the clock_independent stage carries %d members whose observation kind is determined, want %d",
			statuses[core.ObservationKindStatusDetermined], want.DeterminedObservationKindCount)
	}
	if set.Stages[0].Members[0].ObservationKind.Status != core.ObservationKindStatusUndetermined {
		t.Errorf("the est member carries the observation kind status %q, want %q",
			set.Stages[0].Members[0].ObservationKind.Status, core.ObservationKindStatusUndetermined)
	}
}

func TestBuildCandidateSetAcceptsRecordsWithoutObservationKind(t *testing.T) {
	candidate := matchCandidate(t, firstCandidateSn, "con", "80", markIIProcessId)
	// 候補の側の入力形式に観測の種別の欄が無い状態を、raw の要素数 0 で表す。
	candidate.Observation.ObservationKind = core.ObservationKind{Raw: []core.RecordField{}}
	request := matchRequest(t, []core.MatchCandidateRecord{candidate})
	request.IncludedObservationKinds = nil
	set := buildSet(t, request)

	first := stageByKey(t, set, core.StageKeyClockIndependent)
	if first.MemberCount != 1 {
		t.Fatalf("the clock_independent memberCount is %d, want 1", first.MemberCount)
	}
	if first.IncludedObservationKinds != nil {
		t.Errorf("the clock_independent stage carries includedObservationKinds while the input format has no observation kind column")
	}
}

func TestBuildCandidateSetEmptiesSecondStageOnly(t *testing.T) {
	candidates := []core.MatchCandidateRecord{
		matchCandidate(t, thirdCandidateSn, "est", "80", secondMarkIIProcessId),
		matchCandidate(t, fourthCandidateSn, "con", "80", secondMarkIIProcessId),
	}
	set := buildSet(t, matchRequest(t, candidates))

	first := stageByKey(t, set, core.StageKeyClockIndependent)
	if first.MemberCount != 2 {
		t.Fatalf("the clock_independent memberCount is %d, want 2", first.MemberCount)
	}
	if first.EmptyReason != "" {
		t.Errorf("the clock_independent stage carries the emptyReason %q, want none",
			first.EmptyReason)
	}
	second := stageByKey(t, set, core.StageKeySecondTimeMatched)
	if second.MemberCount != 0 {
		t.Fatalf("the second_time_matched memberCount is %d, want 0", second.MemberCount)
	}
	if second.EmptyReason != core.EmptyReasonNoCandidateInWindow {
		t.Errorf("the second_time_matched emptyReason is %q, want %q",
			second.EmptyReason, core.EmptyReasonNoCandidateInWindow)
	}
	for _, condition := range second.Conditions {
		if len(condition.RightValue) > 0 {
			t.Errorf("the %s condition carries rightValue while memberCount is 0",
				condition.ConditionKey)
		}
	}
}

func TestBuildCandidateSetEmptiesBothStages(t *testing.T) {
	candidates := []core.MatchCandidateRecord{
		matchCandidate(t, otherPortCandidateSn, "con", "8080", markIIProcessId),
	}
	set := buildSet(t, matchRequest(t, candidates))

	for _, stage := range set.Stages {
		if stage.MemberCount != 0 {
			t.Errorf("the %s memberCount is %d, want 0", stage.StageKey, stage.MemberCount)
		}
		if stage.EmptyReason != core.EmptyReasonNoCandidateMatchingConditions {
			t.Errorf("the %s emptyReason is %q, want %q", stage.StageKey, stage.EmptyReason,
				core.EmptyReasonNoCandidateMatchingConditions)
		}
		if stage.DistinctProcessCount != nil {
			t.Errorf("the %s stage carries a distinctProcessCount while it has no member",
				stage.StageKey)
		}
		for _, condition := range stage.Conditions {
			if len(condition.RightValue) > 0 {
				t.Errorf("the %s condition of the %s stage carries rightValue",
					condition.ConditionKey, stage.StageKey)
			}
			if condition.ConditionKey == core.ConditionKeyDestinationIp && len(condition.LeftValue) == 0 {
				t.Errorf("the destination_ip condition of the %s stage carries no leftValue",
					stage.StageKey)
			}
		}
	}
}

func TestBuildCandidateSetReturnsNoStageOutsideAssignmentRange(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	request.Origin.AssignmentValidRange = core.TimeRange{
		From: clockTime(t, "2024-03-14T11:10:00.000+09:00", core.ClockTerminalLocal),
		To:   clockTime(t, "2024-03-14T11:20:00.000+09:00", core.ClockTerminalLocal),
	}
	set := buildSet(t, request)

	if len(set.Stages) != 0 {
		t.Fatalf("the candidate set carries %d stages, want 0", len(set.Stages))
	}
	if set.EmptyReason != core.EmptyReasonOriginOutsideAssignmentRange {
		t.Errorf("the candidate set emptyReason is %q, want %q",
			set.EmptyReason, core.EmptyReasonOriginOutsideAssignmentRange)
	}
}

func TestBuildCandidateSetRejectsOriginTimeOutsideComparison(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *core.MatchRequest)
	}{
		{"起点の時刻に UTC からのずれが無い", func(t *testing.T, request *core.MatchRequest) {
			request.Origin.Observation.EventTime = localTimeWithoutOffset(t,
				"[14/Mar/2024:10:20:30]", "2024-03-14T10:20:30",
				core.ClockObserverLocal, core.MeaningRecordOutput)
		}},
		{"割当の期間の下端に UTC からのずれが無い", func(t *testing.T, request *core.MatchRequest) {
			request.Origin.AssignmentValidRange.From = localTimeWithoutOffset(t,
				"03/14/2024 09:00:00", "2024-03-14T09:00:00",
				core.ClockTerminalLocal, core.MeaningEvent)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			testCase.change(t, &request)

			set, err := core.BuildCandidateSet(request)
			if err == nil {
				t.Fatalf("building the candidate set returned no error")
			}
			if !errors.Is(err, core.ErrInconsistentValue) {
				t.Errorf("the error is %v, want one that wraps ErrInconsistentValue", err)
			}
			if want := "the origin time and the assignment range cannot be compared"; !strings.Contains(err.Error(), want) {
				t.Errorf("the error is %v, want one that carries %q", err, want)
			}
			// 比べられない時刻を origin_outside_assignment_range として返さない。
			if len(set.Stages) != 0 || set.EmptyReason != "" {
				t.Errorf("the failed call returned %d stages and the emptyReason %q, want 0 and none",
					len(set.Stages), set.EmptyReason)
			}
		})
	}
}

func TestBuildCandidateSetReturnsNoStageOnWithheldPublication(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	request.PublicationState = core.PublicationStateWithheld
	set := buildSet(t, request)

	if len(set.Stages) != 0 {
		t.Fatalf("the candidate set carries %d stages, want 0", len(set.Stages))
	}
	if set.EmptyReason != core.EmptyReasonPublicationWithheld {
		t.Errorf("the candidate set emptyReason is %q, want %q",
			set.EmptyReason, core.EmptyReasonPublicationWithheld)
	}
}

func TestBuildCandidateSetKeepsFailedRecordDependency(t *testing.T) {
	candidates := positiveCandidates(t)
	candidates[0].FailedRecordDependency = true
	candidates[0].UnresolvedReasons = []string{"取り込みで失敗したレコードに依拠する"}
	request := matchRequest(t, candidates)
	request.UnprocessedRanges = []core.RecordRange{markIIWholeSourceRange()}
	request.UnprocessedRangeCount = 1
	request.PublicationState = core.PublicationStatePublishedPartial
	set := buildSet(t, request)

	want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
	first := stageByKey(t, set, core.StageKeyClockIndependent)
	if first.MemberCount != want.MemberCount {
		t.Fatalf("the clock_independent memberCount is %d, want %d",
			first.MemberCount, want.MemberCount)
	}
	if !first.Members[0].FailedRecordDependency {
		t.Errorf("the first member lost its failedRecordDependency")
	}
	if first.Members[0].RelationState != core.RelationStateCandidate {
		t.Errorf("the first member carries the relationState %q, want %q",
			first.Members[0].RelationState, core.RelationStateCandidate)
	}
	found := false
	for _, reason := range first.Members[0].UnresolvedReasons {
		if reason == "取り込みで失敗したレコードに依拠する" {
			found = true
		}
	}
	if !found {
		t.Errorf("the first member lost the unresolvedReason given by the caller")
	}
}

func TestBuildCandidateSetCarriesConditionValues(t *testing.T) {
	set := buildSet(t, matchRequest(t, positiveCandidates(t)))
	first := stageByKey(t, set, core.StageKeyClockIndependent)
	second := stageByKey(t, set, core.StageKeySecondTimeMatched)

	terminal := conditionByKey(t, first, core.ConditionKeyTerminalIpAssignment)
	if terminal.OutsideAssignmentRange == nil || *terminal.OutsideAssignmentRange {
		t.Errorf("the terminal_ip_assignment outsideAssignmentRange is %v, want false",
			terminal.OutsideAssignmentRange)
	}
	if terminal.AssignmentValidRange == nil {
		t.Errorf("the terminal_ip_assignment condition carries no assignmentValidRange")
	}
	// 両側とも terminal.id を持つ項目を出す。候補の側の name は tmid であり、表示名の
	// com を出さない。
	if len(terminal.LeftValue) != 1 || terminal.LeftValue[0].Name != "clientTerminal" ||
		terminal.LeftValue[0].Semantic != core.SemanticKeyTerminalId {
		t.Errorf("the terminal_ip_assignment leftValue is %v, want the clientTerminal field carrying %q",
			terminal.LeftValue, core.SemanticKeyTerminalId)
	}
	if len(terminal.RightValue) != 1 || terminal.RightValue[0].Name != "tmid" ||
		terminal.RightValue[0].Semantic != core.SemanticKeyTerminalId {
		t.Errorf("the terminal_ip_assignment rightValue is %v, want the tmid field carrying %q",
			terminal.RightValue, core.SemanticKeyTerminalId)
	}

	destinationIp := conditionByKey(t, first, core.ConditionKeyDestinationIp)
	if len(destinationIp.LeftValue) != 1 || destinationIp.LeftValue[0].Name != "requestTargetHost" {
		t.Errorf("the destination_ip leftValue is %v, want the requestTargetHost field",
			destinationIp.LeftValue)
	}
	if len(destinationIp.RightValue) != 1 || destinationIp.RightValue[0].Name != "dstIP" {
		t.Errorf("the destination_ip rightValue is %v, want the dstIP field",
			destinationIp.RightValue)
	}
	destinationPort := conditionByKey(t, first, core.ConditionKeyDestinationPort)
	if len(destinationPort.LeftValue) != 1 || destinationPort.LeftValue[0].Name != "requestTargetPort" {
		t.Errorf("the destination_port leftValue is %v, want the requestTargetPort field",
			destinationPort.LeftValue)
	}
	if len(destinationPort.RightValue) != 1 || destinationPort.RightValue[0].Name != "dstPort" {
		t.Errorf("the destination_port rightValue is %v, want the dstPort field",
			destinationPort.RightValue)
	}
	if secondOfTime := conditionByKey(t, first, core.ConditionKeySecondOfTime); len(
		secondOfTime.LeftValue) != 1 || secondOfTime.LeftValue[0].Name != "requestTime" {
		t.Errorf("the second_of_time leftValue is %v, want the requestTime field",
			secondOfTime.LeftValue)
	}

	for _, stage := range []core.CandidateStage{first, second} {
		secondOfTime := conditionByKey(t, stage, core.ConditionKeySecondOfTime)
		if len(secondOfTime.LeftValue) != 1 {
			t.Fatalf("the second_of_time condition of the %s stage carries %d leftValue elements, want 1",
				stage.StageKey, len(secondOfTime.LeftValue))
		}
		if secondOfTime.LeftValue[0].Kind != core.RecordFieldKindTimestamp {
			t.Errorf("the second_of_time leftValue of the %s stage carries the kind %q, want %q",
				stage.StageKey, secondOfTime.LeftValue[0].Kind, core.RecordFieldKindTimestamp)
		}
		if secondOfTime.LeftValue[0].Timestamp.Precision != core.PrecisionSecond {
			t.Errorf("the second_of_time leftValue of the %s stage carries the precision %q, want %q",
				stage.StageKey, secondOfTime.LeftValue[0].Timestamp.Precision, core.PrecisionSecond)
		}
		if secondOfTime.LeftValue[0].Semantic != core.SemanticKeyEventTime {
			t.Errorf("the second_of_time leftValue of the %s stage carries the semantic %q, want %q",
				stage.StageKey, secondOfTime.LeftValue[0].Semantic, core.SemanticKeyEventTime)
		}
		if len(secondOfTime.RightValue) != 0 {
			t.Errorf("the second_of_time condition of the %s stage carries rightValue",
				stage.StageKey)
		}
	}
	if conditionByKey(t, first, core.ConditionKeySecondOfTime).Use != core.ConditionUseNotUsed {
		t.Errorf("the second_of_time use of the clock_independent stage is not not_used")
	}
	if conditionByKey(t, second, core.ConditionKeySecondOfTime).Use != core.ConditionUseUsed {
		t.Errorf("the second_of_time use of the second_time_matched stage is not used")
	}
	for _, member := range second.Members {
		if member.TimeComparison.RightTime.Precision != core.PrecisionMillisecond {
			t.Errorf("a second_time_matched member rightTime carries the precision %q, want %q",
				member.TimeComparison.RightTime.Precision, core.PrecisionMillisecond)
		}
	}
}

// hostNameAuthorityField は `%ru` の authority の host がホスト名である項目を返す。
// 語彙の項目は connection.destination_hostname である。
func hostNameAuthorityField(t *testing.T) core.RecordField {
	t.Helper()
	return semanticDerivedText(t, "requestTargetHost",
		core.SemanticKeyConnectionDestinationHostname,
		"http://proxy.example.test/start", "proxy.example.test",
		"%ru の authority から userinfo と port と IPv6 の角括弧を外した host")
}

// 要求先の host がホスト名である起点は connection.destination_hostname を持ち、
// connection.destination_address を持たない。
func TestBuildCandidateSetMarksHostNameAuthorityAsNotComparable(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	request.Origin.Observation = matchOriginObservation(t, hostNameAuthorityField(t))
	request.Origin.DestinationIpUse = core.ConditionUseNoComparableCounterpart
	request.Conditions = matchConditionSpecs(core.ConditionUseNoComparableCounterpart)
	set := buildSet(t, request)

	first := stageByKey(t, set, core.StageKeyClockIndependent)
	condition := conditionByKey(t, first, core.ConditionKeyDestinationIp)
	if condition.Use != core.ConditionUseNoComparableCounterpart {
		t.Errorf("the destination_ip use is %q, want %q",
			condition.Use, core.ConditionUseNoComparableCounterpart)
	}
	if len(condition.RightValue) != 0 {
		t.Errorf("the destination_ip condition carries rightValue while it is not used")
	}
	// 接続先ホスト名を持つ項目を接続先 IP の条件の leftValue に入れない。
	if len(condition.LeftValue) != 0 {
		t.Errorf("the destination_ip condition carries the leftValue %v while the origin carries no %q",
			condition.LeftValue, core.SemanticKeyConnectionDestinationAddress)
	}
	// 接続先 IP を比べられない起点では、段階が用いる条件からその 1 件が外れる。
	want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
	wantUsed := make([]string, 0, len(want.UsedConditionKeys))
	for _, key := range want.UsedConditionKeys {
		if key != string(core.ConditionKeyDestinationIp) {
			wantUsed = append(wantUsed, key)
		}
	}
	if got := usedConditionKeys(first); !reflect.DeepEqual(got, wantUsed) {
		t.Errorf("the clock_independent stage uses the conditions %v, want %v", got, wantUsed)
	}
	found := false
	for _, reason := range first.Members[0].UnresolvedReasons {
		if reason == "「接続先 IP」を両側で比べられる値の組が無い" {
			found = true
		}
	}
	if !found {
		t.Errorf("the member carries no unresolvedReason for the destination_ip condition: %v",
			first.Members[0].UnresolvedReasons)
	}
}

// 段階は候補を全件持つ。memberCount は members の要素数と一致する。
func TestBuildCandidateSetCarriesEveryMember(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	set := buildSet(t, request)

	want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
	first := stageByKey(t, set, core.StageKeyClockIndependent)
	if first.MemberCount != want.MemberCount {
		t.Errorf("the clock_independent memberCount is %d, want %d",
			first.MemberCount, want.MemberCount)
	}
	if first.MemberCount != int64(len(first.Members)) {
		t.Errorf("the clock_independent stage returns %d members, want memberCount %d",
			len(first.Members), first.MemberCount)
	}
	if first.DistinctProcessCount == nil || *first.DistinctProcessCount != want.DistinctProcessCount {
		t.Errorf("the clock_independent distinctProcessCount is %v, want %d",
			first.DistinctProcessCount, want.DistinctProcessCount)
	}
	if first.IndistinguishableGroupCount != want.IndistinguishableGroupCount {
		t.Errorf("the clock_independent indistinguishableGroupCount is %d, want %d",
			first.IndistinguishableGroupCount, want.IndistinguishableGroupCount)
	}
}

// 段階 1 だけを求める要求は secondStageWindow を読まない。windowKind が not_compared の
// 要求も成り立つ。
func TestBuildCandidateSetBuildsSingleRequestedStage(t *testing.T) {
	cases := []struct {
		name   string
		window core.TimeWindow
	}{
		{"時刻の範囲が same_second", sameSecondWindow()},
		{"時刻の範囲が not_compared", core.TimeWindow{WindowKind: core.WindowKindNotCompared}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			request.StageKeys = []core.StageKey{core.StageKeyClockIndependent}
			request.SecondStageWindow = testCase.window
			set := buildSet(t, request)

			if len(set.Stages) != 1 {
				t.Fatalf("the candidate set carries %d stages, want 1", len(set.Stages))
			}
			if set.Stages[0].StageKey != core.StageKeyClockIndependent {
				t.Errorf("the candidate set carries the %s stage, want clock_independent",
					set.Stages[0].StageKey)
			}
		})
	}
}

func TestBuildCandidateSetRejectsIncompleteRequests(t *testing.T) {
	cases := []struct {
		name   string
		change func(*core.MatchRequest)
	}{
		{"起点の端末に比べられる値が無い", func(request *core.MatchRequest) {
			replaceOriginField(t, request, "clientTerminal",
				absentItemSemanticText(t, "clientTerminal", core.SemanticKeyTerminalId))
		}},
		{"起点の端末の項目が語彙の項目を持たない", func(request *core.MatchRequest) {
			replaceOriginField(t, request, "clientTerminal",
				derivedText(t, "clientTerminal", markIIClientIp, markIITerminalId,
					"接続元 IP に IP から端末への割当を適用した"))
		}},
		{"起点が時刻の語彙の項目を持たない", func(request *core.MatchRequest) {
			replaceOriginField(t, request, "requestTime",
				presentText(t, "requestTime", "[14/Mar/2024:10:20:30 +0900]"))
		}},
		{"起点が同じ語彙の項目を 2 つ持つ", func(request *core.MatchRequest) {
			request.Origin.Observation.Fields = append(request.Origin.Observation.Fields,
				semanticText(t, "clientTerminalId", core.SemanticKeyTerminalId, markIITerminalId))
		}},
		{"接続先 IP を用いる起点が接続先ホスト名しか持たない", func(request *core.MatchRequest) {
			replaceOriginField(t, request, "requestTargetHost", hostNameAuthorityField(t))
		}},
		{"段階の種別が 1 つも無い", func(request *core.MatchRequest) {
			request.StageKeys = nil
		}},
		{"段階の種別が重なる", func(request *core.MatchRequest) {
			request.StageKeys = []core.StageKey{
				core.StageKeyClockIndependent, core.StageKeyClockIndependent}
		}},
		{"解析実行への参照が無い", func(request *core.MatchRequest) {
			request.AnalysisRunRef = ""
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			testCase.change(&request)
			if _, err := core.BuildCandidateSet(request); err == nil {
				t.Fatalf("the request returned no error")
			} else if !errors.Is(err, core.ErrInvalid) {
				t.Errorf("the error is %v, want one that wraps ErrInvalid", err)
			}
		})
	}
}

// 段階 1 の 3 条件が取り出すのは語彙の項目である。
func TestBuildCandidateSetComparesBySemanticNotByName(t *testing.T) {
	cases := []struct {
		name            string
		terminalFields  []core.RecordField
		wantMemberCount int64
	}{
		{
			"name が異なり語彙の項目が同じ端末を数える",
			[]core.RecordField{
				semanticText(t, "endpointGuid", core.SemanticKeyTerminalId, markIITerminalId),
			},
			1,
		},
		{
			"name が同じで語彙の項目が異なる端末を数えない",
			[]core.RecordField{
				semanticText(t, "tmid", core.SemanticKeyTerminalHostname, markIITerminalId),
			},
			0,
		},
		{
			"語彙の項目が空の端末を数えない",
			[]core.RecordField{presentText(t, "tmid", markIITerminalId)},
			0,
		},
		{
			"表示名だけが起点と一致する端末を数えない",
			[]core.RecordField{
				semanticText(t, "tmid", core.SemanticKeyTerminalId,
					"99999999-8888-7777-6666-555555555555"),
				semanticText(t, "com", core.SemanticKeyTerminalHostname, markIITerminalName),
			},
			0,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fields := append(testCase.terminalFields, candidateDestinationFields(t)...)
			request := matchRequest(t, []core.MatchCandidateRecord{
				candidateWithFields(t, fields),
			})
			set := buildSet(t, request)

			first := stageByKey(t, set, core.StageKeyClockIndependent)
			if first.MemberCount != testCase.wantMemberCount {
				t.Errorf("the clock_independent memberCount is %d, want %d",
					first.MemberCount, testCase.wantMemberCount)
			}
		})
	}
}

// 接続先 IP を used で用いる要求は、起点が connection.destination_address を持つことを
// 求める。接続先ホスト名を持つ要素で代用しない。
//
// 代用すると、ホスト名と IP の文字列を比べた候補 0 件が「条件に一致する候補が 0 件」として
// 読める形になる。
func TestBuildCandidateSetRequiresTheDestinationAddressWhenTheConditionIsUsed(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	replaceOriginField(t, &request, "requestTargetHost", hostNameAuthorityField(t))
	if request.Origin.DestinationIpUse != core.ConditionUseUsed {
		t.Fatalf("the origin destinationIpUse is %q, want %q",
			request.Origin.DestinationIpUse, core.ConditionUseUsed)
	}

	_, err := core.BuildCandidateSet(request)
	if err == nil {
		t.Fatalf("the request returned no error")
	}
	if !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("the error is %v, want one that wraps ErrMissingRequiredItem", err)
	}
	if want := string(core.SemanticKeyConnectionDestinationAddress); !strings.Contains(
		err.Error(), want) {
		t.Errorf("the error is %v, want one that names %q", err, want)
	}

	// use を no_comparable_counterpart にした同じ起点は受理する。接続先 IP を絞りに
	// 用いない宣言になるため、起点にその欄を求めない。
	request.Origin.DestinationIpUse = core.ConditionUseNoComparableCounterpart
	request.Conditions = matchConditionSpecs(core.ConditionUseNoComparableCounterpart)
	if _, err := core.BuildCandidateSet(request); err != nil {
		t.Errorf("the request carrying no_comparable_counterpart returned the error %v", err)
	}
}

// FieldBySemantic は空の語彙の項目を集合の中の要素と一致させない。
//
// 一致させると、語彙の項目を持たない要素が、値の埋まっていない語彙の項目への応答として
// 返る。ノードの識別鍵にこの経路で形式固有の値が入ると、別のノードが 1 つに
// まとめられる。
func TestFieldBySemanticRejectsTheEmptyVocabularyItem(t *testing.T) {
	// clientPort が語彙の項目を持たない要素である。
	observation := matchOriginObservation(t, originDestinationIpField(t))

	if field, found := observation.FieldBySemantic(""); found {
		t.Errorf("the empty semantic returned the field %q, want no field", field.Name)
	}
	field, found := observation.FieldBySemantic(core.SemanticKeyTerminalId)
	if !found {
		t.Fatalf("the semantic %q returned no field", core.SemanticKeyTerminalId)
	}
	if field.Name != "clientTerminal" {
		t.Errorf("the semantic %q returned the field %q, want %q",
			core.SemanticKeyTerminalId, field.Name, "clientTerminal")
	}
}

// 語彙の項目を持たない要素は、同じ観測に 2 つ以上あってよい。「この入力形式に固有の
// 意味を持つ」という主張であり、意味が一致する相手を持たない。
func TestBuildCandidateSetAcceptsRepeatedEmptySemantic(t *testing.T) {
	request := matchRequest(t, positiveCandidates(t))
	request.Origin.Observation.Fields = append(request.Origin.Observation.Fields,
		absentItemText(t, "squidStatus"), absentItemText(t, "userAgent"))
	set := buildSet(t, request)

	want := expectedStage(t, loadMatchingManifest(t), core.StageKeyClockIndependent)
	first := stageByKey(t, set, core.StageKeyClockIndependent)
	if first.MemberCount != want.MemberCount {
		t.Errorf("the clock_independent memberCount is %d, want %d",
			first.MemberCount, want.MemberCount)
	}
}

// 接続先 IP の条件の宣言と、起点の側の使い方が食い違う要求を受理しない。
//
// 2 つが食い違うと、絞り込みに用いた条件の use が used にならない応答になる。
func TestBuildCandidateSetRejectsTheDestinationIpDisagreement(t *testing.T) {
	cases := map[string]struct {
		use      core.ConditionUse
		compared bool
	}{
		"用いる宣言と比べられない起点": {core.ConditionUseNoComparableCounterpart, true},
		"用いない宣言と比べられる起点": {core.ConditionUseUsed, false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			request.Origin.DestinationIpUse = want.use
			specs := matchConditionSpecs(core.ConditionUseUsed)
			for index := range specs {
				if specs[index].ConditionKey == core.ConditionKeyDestinationIp {
					specs[index].Compared = want.compared
				}
			}
			request.Conditions = specs

			if _, err := core.BuildCandidateSet(request); !errors.Is(
				err, core.ErrInconsistentValue) {
				t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
			}
		})
	}
}

// 候補の側の欄の集合は、既知の語彙の項目を重なりなく持つ。
func TestBuildCandidateSetRejectsBrokenCounterpartItems(t *testing.T) {
	cases := map[string]struct {
		mutate func(*core.MatchRequest)
		want   error
	}{
		"空の語彙の項目": {
			func(r *core.MatchRequest) {
				r.CounterpartItemSemantics = append(r.CounterpartItemSemantics, "")
			},
			core.ErrMissingRequiredItem,
		},
		"重なる語彙の項目": {
			func(r *core.MatchRequest) {
				r.CounterpartItemSemantics = append(
					r.CounterpartItemSemantics, core.SemanticKeyProcessId)
			},
			core.ErrDuplicateElement,
		},
		"既知でない精度": {
			func(r *core.MatchRequest) {
				r.CounterpartTimePrecision = core.Precision("nanosecond")
			},
			core.ErrInvalid,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			request := matchRequest(t, positiveCandidates(t))
			want.mutate(&request)
			if _, err := core.BuildCandidateSet(request); !errors.Is(err, want.want) {
				t.Errorf("error = %v, want one wrapping %v", err, want.want)
			}
		})
	}
	// 候補が 1 件も無い段階では相手の精度が定まらない。空の精度を受理する。
	t.Run("候補が 0 件の精度", func(t *testing.T) {
		request := matchRequest(t, positiveCandidates(t))
		request.CounterpartTimePrecision = core.Precision("")
		if _, err := core.BuildCandidateSet(request); err != nil {
			t.Errorf("building the candidate set without a counterpart precision: %v", err)
		}
	})
}
