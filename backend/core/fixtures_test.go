package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 収集元の内容の識別。小文字 16 進 64 桁。
const (
	markIISha256 = "aa11bb22cc33dd44ee55ff6600778899aa11bb22cc33dd44ee55ff6600778899"
	squidSha256  = "bb22cc33dd44ee55ff6600778899aa11bb22cc33dd44ee55ff6600778899aa11"
)

// 収集元の取り込みを指す内部識別子。値の作り方は取り込みの識別子を発行する実装が決める。
const (
	markIISourceId = "source-mark-ii"
	squidSourceId  = "source-squid"
)

// 収集元の file 名。markii 形式の側は端末ごとに 1 file を持つ。
const (
	markIIFileName = "endpoint-01.log"
	squidFileName  = "proxy-request.log"
)

// 収集元の取得元の path。
const (
	markIIOriginPath = "examples/logs/" + markIIFileName
	squidOriginPath  = "examples/logs/" + squidFileName
)

// 端末の名前と接続元 IP。IP は RFC 5737 の文書用の範囲から採る。
const (
	markIITerminalName = "ENDPOINT01"
	markIIClientIp     = "192.0.2.101"
	proxyTargetIp      = "198.51.100.21"
)

// 端末と、その上で動いたプロセスの識別子。
const (
	markIITerminalId = "11111111-2222-3333-4444-555555555555"
	markIIProcessId  = "{00000000-1111-2222-3333-444444444444}"
)

// markii 形式のレコードの通番。段階 1 の候補と、収集元の範囲の両端に使う。
// 大小関係は rangeFromSequenceNumber < 候補の通番 < rangeToSequenceNumber である。
const (
	rangeFromSequenceNumber = int64(900100)
	rangeToSequenceNumber   = int64(900900)
	firstCandidateSn        = int64(900310)
	secondCandidateSn       = int64(900311)
	thirdCandidateSn        = int64(900320)
	fourthCandidateSn       = int64(900321)
	ancestorCandidateSn     = int64(900300)
)

// Squid のレコードの行番号。
const squidLineNumber = int64(250)

// 1 レコードが複数行に分かれる収集元の位置。
const (
	byteRangeOffset    = int64(7321)
	byteRangeLength    = int64(300)
	byteRangeFirstLine = int64(137)
	byteRangeLineCount = int64(6)
)

// 観測期間の両端。端末への割当の適用期間にも同じ両端を使う。
const (
	markIIObservedRangeFirst = "2024-03-14T09:00:00.100+09:00"
	markIIObservedRangeLast  = "2024-03-14T11:00:00.900+09:00"
)

// stringPtr は省略可の文字列の項目へ値を与える。値が出ていない状態は nil が表す。
func stringPtr(value string) *string {
	return &value
}

// newTimestamp は 9 項目を与えた Timestamp を返す。
func newTimestamp(t *testing.T, items core.Timestamp) core.Timestamp {
	t.Helper()
	timestamp, err := core.NewTimestamp(items)
	if err != nil {
		t.Fatalf("building a Timestamp: %v", err)
	}
	return timestamp
}

// markIIEventTime は markii 形式のレコード 1 件のヘッダー時刻を返す。
func markIIEventTime(t *testing.T) core.Timestamp {
	t.Helper()
	return markIIHeaderTime(t, "03/14/2024 10:20:30.482 +0900", "2024-03-14T10:20:30.482+09:00")
}

// markIIHeaderTime は markii 形式のヘッダー時刻を文字列と正規化値から返す。
func markIIHeaderTime(t *testing.T, rawText, normalized string) core.Timestamp {
	t.Helper()
	return newTimestamp(t, core.Timestamp{
		RawText:        stringPtr(rawText),
		Normalized:     stringPtr(normalized),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
}

// markIICandidateTime は候補の通番ごとのヘッダー時刻を返す。
//
// thirdCandidateSn と fourthCandidateSn が遅い方の時刻を持ち、表に無い通番は
// markIIEventTime と同じ時刻を返す。
func markIICandidateTime(t *testing.T, sequenceNumber int64) core.Timestamp {
	t.Helper()
	switch sequenceNumber {
	case thirdCandidateSn, fourthCandidateSn:
		return markIIHeaderTime(t, "03/14/2024 10:40:15.736 +0900", "2024-03-14T10:40:15.736+09:00")
	default:
		return markIIEventTime(t)
	}
}

// squidRequestTime は Squid のレコード 1 件の時刻を返す。
func squidRequestTime(t *testing.T) core.Timestamp {
	t.Helper()
	return newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("[14/Mar/2024:10:20:30 +0900]"),
		Normalized:     stringPtr("2024-03-14T10:20:30+09:00"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          core.ClockObserverLocal,
		Meaning:        core.MeaningRecordOutput,
		ValueState:     core.ValueStatePresent,
	})
}

// markIIStartTime は markii 形式の sTime を返す。UTC からのずれは未確定である。
//
// ミリ秒の末尾を 0 にしてある。末尾の 0 を除く書き出しの不具合を、JSON の期待値が
// 捕まえられるようにするためである。
func markIIStartTime(t *testing.T) core.Timestamp {
	t.Helper()
	return newTimestamp(t, core.Timestamp{
		RawText:        stringPtr("03/14/2024 09:30:00.120"),
		Normalized:     stringPtr("2024-03-14T09:30:00.120"),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateUndetermined,
		Clock:          core.ClockTerminalLocal,
		Meaning:        core.MeaningOperationStart,
		ValueState:     core.ValueStatePresent,
	})
}

// clockTime は clock と normalized を与えた時刻を返す。割当の期間の試験で使う。
func clockTime(t *testing.T, normalized string, clock core.Clock) core.Timestamp {
	t.Helper()
	return newTimestamp(t, core.Timestamp{
		RawText:        stringPtr(normalized),
		Normalized:     stringPtr(normalized),
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateInValue,
		OffsetText:     stringPtr("+0900"),
		Clock:          clock,
		Meaning:        core.MeaningEvent,
		ValueState:     core.ValueStatePresent,
	})
}

// localTimeWithoutOffset は UTC からのずれが未確定の時刻を返す。Instant が値を返さないため、
// この時刻を絶対時刻として他の時刻と比べられない。
func localTimeWithoutOffset(t *testing.T, rawText, normalized string,
	clock core.Clock, meaning core.Meaning,
) core.Timestamp {
	t.Helper()
	return newTimestamp(t, core.Timestamp{
		RawText:        stringPtr(rawText),
		Normalized:     stringPtr(normalized),
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateUndetermined,
		Clock:          clock,
		Meaning:        meaning,
		ValueState:     core.ValueStatePresent,
	})
}

// squidRequestedCenterTime は候補の要求が与えた時刻の範囲の中心を返す。
func squidRequestedCenterTime() core.RequestedTime {
	return core.RequestedTime{
		RequestText:    "10:20:30",
		Precision:      core.PrecisionSecond,
		OffsetState:    core.OffsetStateUndetermined,
		Normalized:     "2024-03-14T10:20:30+09:00",
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Derivation:     "起点のレコードの日付 2024-03-14 と UTC からのずれ +09:00 を補った",
	}
}

// markIILocator は markii 形式の 1 レコードの位置を返す。
func markIILocator(sequenceNumber int64) core.RecordLocator {
	return core.RecordLocator{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		SourceFileName:      markIIFileName,
		PositionKind:        core.PositionKindSequenceNumber,
		SequenceNumber:      &sequenceNumber,
		RecordRawTextRef:    "/api/v0/records",
	}
}

// squidLocator は Squid の 1 レコードの位置を返す。
func squidLocator(lineNumber int64) core.RecordLocator {
	return core.RecordLocator{
		SourceId:            squidSourceId,
		SourceContentSha256: squidSha256,
		SourceFileName:      squidFileName,
		PositionKind:        core.PositionKindLineNumber,
		LineNumber:          &lineNumber,
		RecordRawTextRef:    "/api/v0/records",
	}
}

// byteRangeLocator は 1 レコードが複数行に分かれる収集元の位置を返す。
func byteRangeLocator(offset, length int64) core.RecordLocator {
	line := byteRangeFirstLine
	lines := byteRangeLineCount
	return core.RecordLocator{
		SourceId:            squidSourceId,
		SourceContentSha256: squidSha256,
		SourceFileName:      squidFileName,
		PositionKind:        core.PositionKindByteRange,
		ByteOffset:          &offset,
		ByteLength:          &length,
		LineNumber:          &line,
		LineCount:           &lines,
		RecordRawTextRef:    "/api/v0/records",
	}
}

// newTextField は値が時刻以外の 1 項目を返す。
func newTextField(t *testing.T, name string, text core.RawAndNormalized) core.RecordField {
	t.Helper()
	field, err := core.NewTextField(name, "", text)
	if err != nil {
		t.Fatalf("building the %s field: %v", name, err)
	}
	return field
}

// presentText は値がある 1 項目を返す。正規化値は持たない。
func presentText(t *testing.T, name, rawText string) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, rawText)
	if err != nil {
		t.Fatalf("building the %s value: %v", name, err)
	}
	return newTextField(t, name, value)
}

// semanticText は語彙の項目を持つ、値がある 1 項目を返す。
func semanticText(
	t *testing.T, name string, semantic core.SemanticKey, rawText string,
) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, rawText)
	if err != nil {
		t.Fatalf("building the %s value: %v", name, err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatalf("building the %s field: %v", name, err)
	}
	return field
}

// derivedText は導出値を持つ 1 項目を返す。原資料の文字列は導出の元になった文字列である。
func derivedText(t *testing.T, name, rawText, normalized, derivation string) core.RecordField {
	t.Helper()
	value, err := core.NewNormalizedValue(core.ValueStatePresent, rawText, normalized, derivation)
	if err != nil {
		t.Fatalf("building the %s value: %v", name, err)
	}
	return newTextField(t, name, value)
}

// semanticDerivedText は語彙の項目を持つ、導出値を持つ 1 項目を返す。
func semanticDerivedText(
	t *testing.T, name string, semantic core.SemanticKey, rawText, normalized, derivation string,
) core.RecordField {
	t.Helper()
	value, err := core.NewNormalizedValue(core.ValueStatePresent, rawText, normalized, derivation)
	if err != nil {
		t.Fatalf("building the %s value: %v", name, err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatalf("building the %s field: %v", name, err)
	}
	return field
}

// semanticTimestampField は語彙の項目を持つ、時刻を持つ 1 項目を返す。
func semanticTimestampField(
	t *testing.T, name string, semantic core.SemanticKey, timestamp core.Timestamp,
) core.RecordField {
	t.Helper()
	field, err := core.NewTimestampField(name, semantic, timestamp)
	if err != nil {
		t.Fatalf("building the %s field: %v", name, err)
	}
	return field
}

// absentItemText は入力形式に欄が無い 1 項目を返す。
func absentItemText(t *testing.T, name string) core.RecordField {
	t.Helper()
	return newTextField(t, name, core.NewAbsentItemValue())
}

// absentItemSemanticText は語彙の項目を持ち、入力形式に欄が無い 1 項目を返す。
func absentItemSemanticText(
	t *testing.T, name string, semantic core.SemanticKey,
) core.RecordField {
	t.Helper()
	field, err := core.NewTextField(name, semantic, core.NewAbsentItemValue())
	if err != nil {
		t.Fatalf("building the %s field: %v", name, err)
	}
	return field
}

// destinationIpCondition は接続先 IP の一致を用いた条件を返す。
//
// withRightValue が偽のときは候補の側の値を出さない。候補が 1 件も無い段階には、比較した
// 候補の側の値が存在しない。
func destinationIpCondition(t *testing.T, withRightValue bool) core.MatchCondition {
	t.Helper()
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyDestinationIp,
		Use:          core.ConditionUseUsed,
		LeftValue: []core.RecordField{
			derivedText(t, "requestTargetHost", proxyTargetIp, proxyTargetIp,
				"%ru の authority から userinfo と port と IPv6 の角括弧を外した host"),
		},
	}
	if !withRightValue {
		return condition
	}
	condition.RightValue = []core.RecordField{presentText(t, "dstIP", proxyTargetIp)}
	return condition
}

// terminalAssignmentCondition は IP から端末への割当を用いた条件を返す。
//
// 両側とも terminal.id を持つ項目を置く。端末の表示名を条件に入れない。
func terminalAssignmentCondition(t *testing.T, withRightValue bool) core.MatchCondition {
	t.Helper()
	validRange := markIIAssignment(t).AssignmentValidRange
	outside := false
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyTerminalIpAssignment,
		Use:          core.ConditionUseUsed,
		LeftValue: []core.RecordField{
			semanticDerivedText(t, "clientTerminal", core.SemanticKeyTerminalId,
				markIIClientIp, markIITerminalId, "接続元 IP に IP から端末への割当を適用した"),
		},
		AssignmentValidRange:   &validRange,
		OutsideAssignmentRange: &outside,
	}
	if !withRightValue {
		return condition
	}
	condition.RightValue = []core.RecordField{
		semanticText(t, "tmid", core.SemanticKeyTerminalId, markIITerminalId),
	}
	return condition
}

// destinationPortCondition は接続先 port の一致を用いた条件を返す。
func destinationPortCondition(t *testing.T, withRightValue bool) core.MatchCondition {
	t.Helper()
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyDestinationPort,
		Use:          core.ConditionUseUsed,
		LeftValue: []core.RecordField{
			derivedText(t, "requestTargetPort", "http://"+proxyTargetIp+"/", "80",
				"%ru の scheme http から port を導いた"),
		},
	}
	if !withRightValue {
		return condition
	}
	condition.RightValue = []core.RecordField{presentText(t, "dstPort", "80")}
	return condition
}

// secondOfTimeCondition は秒単位の時刻の条件を返す。段階 1 は not_used、段階 2 は used である。
//
// 時刻の 2 条件は候補の側の値を出さない。候補の側の時刻は Candidate の timeComparison の
// rightTime が持つ。
func secondOfTimeCondition(t *testing.T, stageKey core.StageKey) core.MatchCondition {
	t.Helper()
	if stageKey == core.StageKeyClockIndependent {
		return core.MatchCondition{
			ConditionKey: core.ConditionKeySecondOfTime,
			Use:          core.ConditionUseNotUsed,
		}
	}
	field, err := core.NewTimestampField(
		"requestTime", core.SemanticKeyEventTime, squidRequestTime(t))
	if err != nil {
		t.Fatalf("building the requestTime field: %v", err)
	}
	return core.MatchCondition{
		ConditionKey: core.ConditionKeySecondOfTime,
		Use:          core.ConditionUseUsed,
		LeftValue:    []core.RecordField{field},
	}
}

// stageConditions は段階の conditions を全数返す。
func stageConditions(t *testing.T, stageKey core.StageKey, withRightValue bool) []core.MatchCondition {
	t.Helper()
	conditions := []core.MatchCondition{
		terminalAssignmentCondition(t, withRightValue),
		destinationIpCondition(t, withRightValue),
		destinationPortCondition(t, withRightValue),
		secondOfTimeCondition(t, stageKey),
		{ConditionKey: core.ConditionKeySubSecondOfTime, Use: core.ConditionUseNoComparableCounterpart},
		{ConditionKey: core.ConditionKeyClientPort, Use: core.ConditionUseItemAbsentOnCounterpart},
		{ConditionKey: core.ConditionKeyProcess, Use: core.ConditionUseItemAbsentOnCounterpart},
		{ConditionKey: core.ConditionKeyUser, Use: core.ConditionUseNotUsed},
	}
	return conditions
}

// definedSubEvents は、evt が net のときに形式が意味を定める subEvt の値である。
var definedSubEvents = []string{"con", "acpt", "dcon"}

// markIIObservationKind は evt が net で subEvt が引数の値である観測の種別を返す。
// status は subEvt が definedSubEvents にあれば determined、無ければ undetermined である。
func markIIObservationKind(t *testing.T, subEvt string) core.ObservationKind {
	t.Helper()
	status := core.ObservationKindStatusUndetermined
	for _, defined := range definedSubEvents {
		if subEvt == defined {
			status = core.ObservationKindStatusDetermined
		}
	}
	return core.ObservationKind{
		Raw: []core.RecordField{
			presentText(t, "evt", "net"),
			presentText(t, "subEvt", subEvt),
		},
		Status: status,
	}
}

// markIICandidateObservationKind は候補の通番ごとの観測の種別を返す。
// secondCandidateSn は subEvt が con である。他の通番は est を返す。
func markIICandidateObservationKind(t *testing.T, sequenceNumber int64) core.ObservationKind {
	t.Helper()
	if sequenceNumber == secondCandidateSn {
		return markIIObservationKind(t, "con")
	}
	return markIIObservationKind(t, "est")
}

// includedObservationKinds は候補として数えるレコードを選ぶ観測の種別 2 件を返す。
func includedObservationKinds() []core.ObservationKindSelector {
	return []core.ObservationKindSelector{
		twoItemSelector("net", "con"),
		twoItemSelector("net", "est"),
	}
}

// matchConditionSpecs は Squid と markii 形式の組が持つ条件の宣言を返す。
//
// **pipeline の candidateConditionSpecs の複製である。** core は pipeline を import
// できないため同じ組を手で持つ。2 つが食い違わないことは、pipeline 側の
// TestCandidateConditionSpecsMatchTheCoreTestDeclaration が確かめる。
func matchConditionSpecs(destinationIpUse core.ConditionUse) []core.MatchConditionSpec {
	comparesDestinationIp := destinationIpUse == core.ConditionUseUsed
	return []core.MatchConditionSpec{
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
			Compared: comparesDestinationIp,
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
}

// counterpartItemSemantics は候補の側 (markii 形式) の収集元が持つ語彙の項目を返す。
// 接続元 port とプロセスと利用者の欄を持つ。
func counterpartItemSemantics() []core.SemanticKey {
	return []core.SemanticKey{
		core.SemanticKeyTerminalId,
		core.SemanticKeyConnectionDestinationAddress,
		core.SemanticKeyConnectionDestinationPort,
		core.SemanticKeyEventTime,
		core.SemanticKeyConnectionSourcePort,
		core.SemanticKeyProcessId,
		core.SemanticKeyEventAccountName,
	}
}

// twoItemSelector は観測の種別を 2 欄で書く入力形式の選ぶ条件を組む。
func twoItemSelector(event, subEvent string) core.ObservationKindSelector {
	return core.ObservationKindSelector{Items: []core.ObservationKindSelectorItem{
		{Name: "evt", Value: event},
		{Name: "subEvt", Value: subEvent},
	}}
}

// clockOffsetAssumption は段階 2 が依拠する前提を返す。
func clockOffsetAssumption() core.MatchAssumption {
	return core.MatchAssumption{
		AssumptionKey:    core.AssumptionKeyClockOffsetBelowOneSecond,
		EvidenceClass:    core.EvidenceClassUnconfirmed,
		Statement:        "Proxy と " + markIITerminalName + " の時計のずれが 1 秒未満である",
		UnresolvedReason: "時刻同期の設定を示す値が入力に無い",
	}
}

// notComparedCandidate は時刻を比べていない候補 1 件を返す。
// 時刻と観測の種別は通番ごとに変わる。
func notComparedCandidate(t *testing.T, sequenceNumber int64) core.Candidate {
	t.Helper()
	return core.Candidate{
		RecordRef: markIILocator(sequenceNumber),
		EventTime: markIICandidateTime(t, sequenceNumber),
		TimeComparison: core.TimeComparison{
			ComparisonUnit: core.ComparisonUnitNotCompared,
			Assumptions:    []core.MatchAssumption{},
		},
		FailedRecordDependency: false,
		ObservationKind:        markIICandidateObservationKind(t, sequenceNumber),
		RelationState:          core.RelationStateCandidate,
		UnresolvedReasons:      []string{"接続を一意に指す項目が Proxy 側に無い"},
	}
}

// secondMatchedCandidate は秒単位で時刻を比べた候補 1 件を返す。
func secondMatchedCandidate(t *testing.T, sequenceNumber int64) core.Candidate {
	t.Helper()
	leftTime := squidRequestTime(t)
	candidate := notComparedCandidate(t, sequenceNumber)
	eventTime := candidate.EventTime
	candidate.TimeComparison = core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		LeftTime:       &leftTime,
		RightTime:      &eventTime,
		Assumptions:    []core.MatchAssumption{clockOffsetAssumption()},
	}
	return candidate
}

// clockIndependentStage は段階 1 を返す。
func clockIndependentStage(t *testing.T, members []core.Candidate, emptyReason core.EmptyReason) core.CandidateStage {
	t.Helper()
	return core.CandidateStage{
		StageKey:                    core.StageKeyClockIndependent,
		Conditions:                  stageConditions(t, core.StageKeyClockIndependent, len(members) > 0),
		IncludedObservationKinds:    includedObservationKinds(),
		Assumptions:                 []core.MatchAssumption{},
		TimeWindow:                  core.TimeWindow{WindowKind: core.WindowKindNotCompared},
		ComparisonUnit:              core.ComparisonUnitNotCompared,
		Members:                     members,
		MemberCount:                 int64(len(members)),
		IndistinguishableGroups:     [][]core.RecordLocator{},
		IndistinguishableGroupCount: 0,
		EmptyReason:                 emptyReason,
		ClockDependencyNote: "割当の適用期間の判定が Proxy と " + markIITerminalName +
			" の時計のずれに依拠する",
	}
}

// secondTimeMatchedStage は段階 2 を返す。
func secondTimeMatchedStage(t *testing.T, members []core.Candidate, emptyReason core.EmptyReason) core.CandidateStage {
	t.Helper()
	centerTime := squidRequestedCenterTime()
	return core.CandidateStage{
		StageKey:                 core.StageKeySecondTimeMatched,
		Conditions:               stageConditions(t, core.StageKeySecondTimeMatched, len(members) > 0),
		IncludedObservationKinds: includedObservationKinds(),
		Assumptions:              []core.MatchAssumption{clockOffsetAssumption()},
		TimeWindow: core.TimeWindow{
			WindowKind: core.WindowKindSameSecond,
			CenterTime: &centerTime,
		},
		ComparisonUnit:              core.ComparisonUnitSecond,
		Members:                     members,
		MemberCount:                 int64(len(members)),
		IndistinguishableGroups:     [][]core.RecordLocator{},
		IndistinguishableGroupCount: 0,
		EmptyReason:                 emptyReason,
	}
}

// candidateSet は段階の集合から候補集合を返す。
func candidateSet(t *testing.T, stages []core.CandidateStage, emptyReason core.EmptyReason) core.CandidateSet {
	t.Helper()
	return core.CandidateSet{
		OriginRef:             squidLocator(squidLineNumber),
		Stages:                stages,
		EmptyReason:           emptyReason,
		UnprocessedRanges:     []core.RecordRange{},
		UnprocessedRangeCount: 0,
		PublicationState:      core.PublicationStatePublishedFull,
		AnalysisRunRef:        "analysis-run-1",
	}
}

// markIIRange は markii 形式のレコードの範囲を返す。両端を確定できた範囲である。
func markIIRange(fromPosition, toPosition int64) core.RecordRange {
	return core.RecordRange{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		RangeKind:           core.RangeKindPositioned,
		PositionKind:        core.PositionKindSequenceNumber,
		FromPosition:        &fromPosition,
		ToPosition:          &toPosition,
	}
}

// markIIWholeSourceRange は位置を 1 つも確定できない収集元の範囲を返す。
func markIIWholeSourceRange() core.RecordRange {
	return core.RecordRange{
		SourceId:            markIISourceId,
		SourceContentSha256: markIISha256,
		RangeKind:           core.RangeKindWholeSource,
	}
}
