package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// recordsPath は`/api/v0/records` の path である。
const recordsPath = "/api/v0/records"

// absentSha256 は取り込んだどの収集元とも異なる内容の識別である。
const absentSha256 = "0000000000000000000000000000000000000000000000000000000000000000"

// squidResponseFieldNames は`/api/v0/records` の応答が返す fields の name を、応答の順で写す。
//
// 実装の定数を参照せずに書く。参照すると、実装が集合を変えたときに期待値が追随して
// 検査が通る。
var squidResponseFieldNames = []string{
	"clientIp", "ident", "user", "requestTime", "requestLine", "statusCode", "replyBytes",
	"referer", "userAgent", "squidStatus", "requestMethod", "requestTargetHost",
	"requestTargetPort", "requestTarget", "requestVersion", "clientTerminal", "clientTerminalName",
	"clientPort", "process",
}

// markiiResponseFieldNames は`/api/v0/records` の応答が返す fields の name を、応答の順で写す。
// communications-markii.log の通信レコードが書いた key の並びである。
var markiiResponseFieldNames = []string{
	"headerTime", "sn", "evt", "subEvt", "com", "tmid", "csid", "psGUID", "psPath",
	"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send",
}

// fileEventFieldNames は records-markii.log の file / close レコードを
// 指す要求への応答の fields の name である。
//
// **接続と通信量の項目が出ない。** srcIP・srcPort・dstIP・dstPort・recv・send は通信の
// レコードが定める key であり、evt が file のレコードはこの key を定めない。
var fileEventFieldNames = []string{
	"headerTime", "sn", "evt", "subEvt", "com", "tmid", "psGUID", "ip", "path", "sha256",
}

// siblingCommunicationFieldNames は records-markii.log の通信レコードの name である。
// 同じ収集元の evt が net のレコードで、ip の key を持つ。
var siblingCommunicationFieldNames = []string{
	"headerTime", "sn", "evt", "subEvt", "com", "tmid", "csid", "psGUID", "psPath", "ip",
	"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send",
}

// fileEventValueStates は上記の file / close レコードを指す要求への応答の fields の、name ごとの
// valueState である。すべての要素が present である。
//
// 実装の出力を転記せず、fixture のレコードから読める内訳を書く。
var fileEventValueStates = map[string]core.ValueState{
	"headerTime": core.ValueStatePresent,
	"sn":         core.ValueStatePresent,
	"evt":        core.ValueStatePresent,
	"subEvt":     core.ValueStatePresent,
	"com":        core.ValueStatePresent,
	"tmid":       core.ValueStatePresent,
	"psGUID":     core.ValueStatePresent,
	"ip":         core.ValueStatePresent,
	"path":       core.ValueStatePresent,
	"sha256":     core.ValueStatePresent,
}

// recordResponse は`/api/v0/records` の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type recordResponse struct {
	RecordRef       core.RecordLocator    `json:"recordRef"`
	RawText         string                `json:"rawText"`
	SourceIdentity  core.SourceIdentity   `json:"sourceIdentity"`
	Fields          []core.RecordField    `json:"fields"`
	ObservationKind core.ObservationKind  `json:"observationKind"`
	EventKind       *core.EventKindPair   `json:"eventKind,omitempty"`
	DerivationTrail *core.DerivationTrail `json:"derivationTrail,omitempty"`
}

// recordsManifest は fixture と一緒に置いた期待値である。
type recordsManifest struct {
	SquidRecord         manifestRecord `json:"squidRecord"`
	MarkiiRecord        manifestRecord `json:"markiiRecord"`
	FileEventRecord     manifestRecord `json:"fileEventRecord"`
	NeighbouringRecord  manifestRecord `json:"neighbouringRecord"`
	RecordWithSemantics manifestRecord `json:"recordWithSemanticsOfTheSameSource"`
}

type manifestRecord struct {
	File                string            `json:"file"`
	Format              core.FormatKey    `json:"format"`
	PositionKind        core.PositionKind `json:"positionKind"`
	SequenceNumber      int64             `json:"sequenceNumber"`
	LineNumber          int64             `json:"lineNumber"`
	RawText             string            `json:"rawText"`
	ObservationKindRaw  int               `json:"observationKindRawCount"`
	ObservationKindStat string            `json:"observationKindStatus"`
	// SemanticByFieldName は応答の fields の name ごとの語彙の項目である。
	// 載らない name の項目は semantic を持たない。
	SemanticByFieldName map[string]core.SemanticKey `json:"semanticByFieldName"`
	// FileSha256 は evt が file のレコードが書いた対象ファイルの内容の hash である。
	FileSha256 string `json:"fileSha256"`
	Scope      struct {
		RangeKind    core.RangeKind    `json:"rangeKind"`
		PositionKind core.PositionKind `json:"positionKind"`
		FromPosition int64             `json:"fromPosition"`
		ToPosition   int64             `json:"toPosition"`
	} `json:"scope"`
}

// `/api/v0/records` の応答の fields の name は、Squid のレコードを指した要求で
// squidResponseFieldNames と一致する。
func TestRecordsEndpointReturnsTheFieldSetOfTheSquidRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.SquidRecord.File)
	want := manifest.SquidRecord

	decoded := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&lineNumber="+strconv.FormatInt(want.LineNumber, 10)),
		http.StatusOK)

	assertFieldNames(t, decoded.Fields, squidResponseFieldNames)
	if decoded.RawText != want.RawText {
		t.Fatalf("rawText=%q want %q", decoded.RawText, want.RawText)
	}
	if decoded.RecordRef.SourceFileName != want.File ||
		decoded.RecordRef.PositionKind != want.PositionKind {
		t.Fatalf("recordRef=%+v want %q and %q",
			decoded.RecordRef, want.File, want.PositionKind)
	}
	if decoded.RecordRef.LineNumber == nil || *decoded.RecordRef.LineNumber != want.LineNumber {
		t.Fatalf("recordRef.lineNumber=%v want %d", decoded.RecordRef.LineNumber, want.LineNumber)
	}
	// Squid combined はレコードを一意に指す通番を持たない。
	if decoded.RecordRef.SequenceNumber != nil {
		t.Fatalf("recordRef.sequenceNumber=%d want none", *decoded.RecordRef.SequenceNumber)
	}
	if decoded.SourceIdentity.SourceId != identity.SourceId {
		t.Fatalf("sourceIdentity.sourceId=%q want %q",
			decoded.SourceIdentity.SourceId, identity.SourceId)
	}
	if len(decoded.ObservationKind.Raw) != want.ObservationKindRaw ||
		string(decoded.ObservationKind.Status) != want.ObservationKindStat {
		t.Fatalf("observationKind=%+v want %d raw items and status %q",
			decoded.ObservationKind, want.ObservationKindRaw, want.ObservationKindStat)
	}
}

// 同じ path の応答の fields の name の集合は、返すレコードが持つ項目が決める。markii 形式の
// 通信のレコードを指した要求は、段階 1 がそのレコードに意味付けを与えた key を返す。
func TestRecordsEndpointReturnsTheFieldSetOfTheMarkIIRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	want := manifest.MarkiiRecord

	decoded := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber="+
		strconv.FormatInt(want.SequenceNumber, 10)), http.StatusOK)

	assertFieldNames(t, decoded.Fields, markiiResponseFieldNames)
	if decoded.RecordRef.SourceFileName != want.File ||
		decoded.RecordRef.PositionKind != want.PositionKind {
		t.Fatalf("recordRef=%+v want %q and %q",
			decoded.RecordRef, want.File, want.PositionKind)
	}
	if decoded.RecordRef.SequenceNumber == nil ||
		*decoded.RecordRef.SequenceNumber != want.SequenceNumber {
		t.Fatalf("recordRef.sequenceNumber=%v want %d",
			decoded.RecordRef.SequenceNumber, want.SequenceNumber)
	}
	if len(decoded.ObservationKind.Raw) != want.ObservationKindRaw ||
		string(decoded.ObservationKind.Status) != want.ObservationKindStat {
		t.Fatalf("observationKind=%+v want %d raw items and status %q",
			decoded.ObservationKind, want.ObservationKindRaw, want.ObservationKindStat)
	}
	// 反対側。同じ path が 2 つの集合を返し、要素数と name がレコードごとに分かれる。
	squid := recordsSourceIdentity(t, handler, manifest.SquidRecord.File)
	squidRecord := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(squid)+"&lineNumber="+
		strconv.FormatInt(manifest.SquidRecord.LineNumber, 10)), http.StatusOK)
	if len(squidRecord.Fields) == len(decoded.Fields) {
		t.Fatalf("both formats returned %d fields", len(decoded.Fields))
	}
}

// 通番で指した markii 形式のレコードを行番号でも探せる。
func TestRecordsEndpointFindsTheMarkIIRecordByLineNumber(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	want := manifest.MarkiiRecord

	bySequence := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber="+
		strconv.FormatInt(want.SequenceNumber, 10)), http.StatusOK)
	byLine := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&lineNumber="+
		strconv.FormatInt(want.LineNumber, 10)), http.StatusOK)

	if byLine.RecordRef.RecordRawTextRef != bySequence.RecordRef.RecordRawTextRef {
		t.Fatalf("recordRawTextRef by line=%q by sequence=%q",
			byLine.RecordRef.RecordRawTextRef, bySequence.RecordRef.RecordRawTextRef)
	}
	if byLine.RecordRef.LineNumber == nil || *byLine.RecordRef.LineNumber != want.LineNumber {
		t.Fatalf("recordRef.lineNumber=%v want %d", byLine.RecordRef.LineNumber, want.LineNumber)
	}
	// 2 つの位置を同時に与えた要求も同じレコードを返す。
	both := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber="+
		strconv.FormatInt(want.SequenceNumber, 10)+"&lineNumber="+
		strconv.FormatInt(want.LineNumber, 10)), http.StatusOK)
	if both.RecordRef.RecordRawTextRef != bySequence.RecordRef.RecordRawTextRef {
		t.Fatalf("recordRawTextRef with both positions=%q want %q",
			both.RecordRef.RecordRawTextRef, bySequence.RecordRef.RecordRawTextRef)
	}
}

func TestRecordsEndpointRejectsInvalidRequests(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	source := recordsSourceQuery(identity)
	sequence := "sequenceNumber=" + strconv.FormatInt(manifest.MarkiiRecord.SequenceNumber, 10)
	position := "&" + sequence
	origin := "&originSourceId=" + identity.SourceId + "&originSourceContentSha256=" +
		identity.ContentSha256 + "&originLineNumber=1"
	cases := []struct {
		name              string
		query             string
		missingParameters []string
	}{
		{"every required item missing", "",
			[]string{"sourceId", "sourceContentSha256", "sequenceNumber", "lineNumber",
				"byteOffset"}},
		{"sourceId without its content", "sourceId=" + identity.SourceId + position,
			[]string{"sourceContentSha256"}},
		{"content without its sourceId",
			"sourceContentSha256=" + identity.ContentSha256 + position, []string{"sourceId"}},
		{"lineNumber without the content identity",
			"sourceId=" + identity.SourceId + "&lineNumber=1", []string{"sourceContentSha256"}},
		{"sequenceNumber alone", sequence,
			[]string{"sourceId", "sourceContentSha256"}},
		{"no position", source, []string{"sequenceNumber", "lineNumber", "byteOffset"}},
		{"sourceId empty", "sourceId=&sourceContentSha256=" + identity.ContentSha256 + position, nil},
		{"content empty", "sourceId=" + identity.SourceId + "&sourceContentSha256=" + position, nil},
		{"sequenceNumber not a number", source + "&sequenceNumber=x", nil},
		{"sequenceNumber below zero", source + "&sequenceNumber=-1", nil},
		{"lineNumber below its first value", source + "&lineNumber=0", nil},
		{"byteOffset below zero", source + "&byteOffset=-1", nil},
		{"byteOffset not a number", source + "&byteOffset=x", nil},
		{"lineNumber not a number", source + "&lineNumber=1.5", nil},
		{"cursor given", source + position + "&cursor=x", nil},
		// 起点の組 (起点の項目と関連付けの条件) の欠けた項目は、1 回の応答にまとめて返す。
		{"origin without its content", source + position + "&originSourceId=&originLineNumber=0",
			[]string{"originSourceContentSha256", "matchCondition"}},
		{"origin without its position",
			source + position + "&originSourceId=" + identity.SourceId +
				"&originSourceContentSha256=" + identity.ContentSha256,
			[]string{"originSequenceNumber", "originLineNumber", "originByteOffset", "matchCondition"}},
		{"origin position alone", source + position + "&originLineNumber=1",
			[]string{"originSourceId", "originSourceContentSha256", "matchCondition"}},
		{"origin without a match condition", source + position + origin, []string{"matchCondition"}},
		{"match condition without an origin", source + position + "&matchCondition=destination_port",
			[]string{"originSourceId", "originSourceContentSha256", "originSequenceNumber",
				"originLineNumber", "originByteOffset"}},
		{"record and origin both missing items", "matchCondition=destination_port",
			[]string{"sourceId", "sourceContentSha256", "sequenceNumber", "lineNumber", "byteOffset",
				"originSourceId", "originSourceContentSha256", "originSequenceNumber",
				"originLineNumber", "originByteOffset"}},
		{"origin sourceId empty", source + position + "&originSourceId=" +
			"&originSourceContentSha256=" + identity.ContentSha256 + "&originLineNumber=1" +
			"&matchCondition=destination_port", nil},
		{"origin lineNumber below its first value", source + position + "&originSourceId=" +
			identity.SourceId + "&originSourceContentSha256=" + identity.ContentSha256 +
			"&originLineNumber=0&matchCondition=destination_port", nil},
		{"origin with an unknown match condition",
			source + position + origin + "&matchCondition=unknown_condition", []string{"matchCondition"}},
		{"unknown parameter", source + position + "&sortKey=event_time", nil},
		{"unknown parameter carrying a record field",
			source + position + "&recordField=%7B%22name%22%3A%22clientIp%22%7D", nil},
		{"sequenceNumber twice", source + position + position, nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := requestRecordsError(t, handler, recordsPath+"?"+testCase.query,
				http.StatusBadRequest)
			if problem.Code != core.ApiErrorCodeInvalidRequest || problem.Message == "" {
				t.Fatalf("error=%+v", problem)
			}
			assertMissingParameters(t, problem.MissingParameters, testCase.missingParameters)
		})
	}
}

// 起点の項目を与えた要求は、起点から開いたレコードを挙げた関連付けの各段階を、到達した経路として返す。
func TestRecordsEndpointReturnsTheDerivationTrailOfTheMatch(t *testing.T) {
	handler := graphHandler(t)
	match := firstCandidateMatch(t, handler)

	decoded := decodeRecord(t, requestSources(t, handler,
		recordsPath+"?"+trailQuery(match.CandidateRef, match.OriginRef)), http.StatusOK)

	if decoded.RecordRef.RecordRawTextRef != match.CandidateRef.RecordRawTextRef {
		t.Fatalf("recordRef=%+v want the candidate %+v", decoded.RecordRef, match.CandidateRef)
	}
	trail := decoded.DerivationTrail
	if trail == nil {
		t.Fatal("the response carries no derivationTrail")
	}
	if err := trail.Validate(); err != nil {
		t.Fatalf("the trail does not validate: %v", err)
	}
	if trail.StoppedAt != nil {
		t.Fatalf("stoppedAt=%+v want the trail to reach the last step", trail.StoppedAt)
	}
	if trail.OriginRef.RecordRawTextRef != match.OriginRef.RecordRawTextRef {
		t.Fatalf("originRef=%+v want %+v", trail.OriginRef, match.OriginRef)
	}
	// **段階は関連付けが数えた段階と同じ並びである。** 段階ごとの出力はその段階の候補の件数を持つ。
	if len(trail.Steps) != len(match.StageTallies) {
		t.Fatalf("steps=%+v want one step for each of %+v", trail.Steps, match.StageTallies)
	}
	for index, tally := range match.StageTallies {
		step := trail.Steps[index]
		if step.StepKey != tally.StageKey {
			t.Errorf("step %d is %q, want %q", index, step.StepKey, tally.StageKey)
		}
		if !strings.Contains(step.Output, strconv.FormatInt(tally.MemberCount, 10)) {
			t.Errorf("step %d output %q does not carry the member count %d",
				index, step.Output, tally.MemberCount)
		}
	}
	first, last := trail.Steps[0], trail.Steps[len(trail.Steps)-1]
	if first.InputRefs[0].Record == nil ||
		first.InputRefs[0].Record.RecordRawTextRef != match.OriginRef.RecordRawTextRef {
		t.Errorf("the first step starts from %+v, want the origin record", first.InputRefs)
	}
	lastInput := last.InputRefs[len(last.InputRefs)-1]
	if lastInput.Record == nil || lastInput.Record.RecordRawTextRef != match.CandidateRef.RecordRawTextRef {
		t.Errorf("the last step ends at %+v, want the opened record", last.InputRefs)
	}
	// 時刻を比べた段階は、起点と候補の時刻の文字列を用いた識別子に載せる。
	if last.StepKey != string(core.StageKeySecondTimeMatched) || match.TimeComparison.LeftTime == nil ||
		match.TimeComparison.RightTime == nil {
		t.Fatalf("the fixture match ends at %q with %+v, want a time matched stage with both times",
			last.StepKey, match.TimeComparison)
	}
	leftTime, leftPresent := match.TimeComparison.LeftTime.RawTextValue()
	rightTime, rightPresent := match.TimeComparison.RightTime.RawTextValue()
	if !leftPresent || !rightPresent || leftTime == "" || rightTime == "" {
		t.Fatalf("the fixture times carry no raw text: %+v", match.TimeComparison)
	}
	used := strings.Join(last.UsedIdentifiers, "\n")
	if !strings.Contains(used, "起点 "+leftTime) || !strings.Contains(used, "候補 "+rightTime) {
		t.Errorf("the last step used %q, want the origin time %q and the candidate time %q",
			last.UsedIdentifiers, leftTime, rightTime)
	}
}

// 起点と開いたレコードの間に関連付けが無い要求は、レコードを返し、選択の最後の段階で止まった
// 経路を持つ。時刻を比べる選択の最後の段階は時刻の段階である。
func TestRecordsEndpointStopsTheTrailWithoutAMatch(t *testing.T) {
	handler := graphHandler(t)
	match := firstCandidateMatch(t, handler)

	// 候補を起点に置き、起点だったレコードを開く。関連付けは起点から候補への向きだけを持つ。
	decoded := decodeRecord(t, requestSources(t, handler,
		recordsPath+"?"+trailQuery(match.OriginRef, match.CandidateRef)), http.StatusOK)

	trail := decoded.DerivationTrail
	if trail == nil || trail.StoppedAt == nil {
		t.Fatalf("derivationTrail=%+v want a trail that stops", trail)
	}
	if err := trail.Validate(); err != nil {
		t.Fatalf("the trail does not validate: %v", err)
	}
	wantOutput := "選んだ条件では、起点のレコードからこのレコードへの関連付けが無い。" +
		"この組がどの段階で外れたかを、グラフから特定できない"
	if trail.StoppedAt.StepKey != string(core.StageKeySecondTimeMatched) ||
		trail.StoppedAt.Output != wantOutput {
		t.Fatalf("stoppedAt=%+v want the time matched step with the output %q", trail.StoppedAt, wantOutput)
	}
	if trail.OriginRef.RecordRawTextRef != match.CandidateRef.RecordRawTextRef {
		t.Fatalf("originRef=%+v want the requested origin %+v", trail.OriginRef, match.CandidateRef)
	}
}

// 起点の収集元と位置も、開くレコードと同じ code で確かめる。
func TestRecordsEndpointReportsTheOriginFailures(t *testing.T) {
	handler := graphHandler(t)
	match := firstCandidateMatch(t, handler)
	absentSource := match.OriginRef
	absentSource.SourceId = "absent-source"
	absentLine := match.OriginRef
	absentLine.SequenceNumber, absentLine.ByteOffset = nil, nil
	farLine := int64(1) << 40
	absentLine.LineNumber = &farLine
	cases := []struct {
		name   string
		origin core.RecordLocator
		want   core.ApiErrorCode
	}{
		{"source_not_found", absentSource, core.ApiErrorCodeSourceNotFound},
		{"no record at the origin position", absentLine, core.ApiErrorCodePositionOutsideSource},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := requestRecordsError(t, handler,
				recordsPath+"?"+trailQuery(match.CandidateRef, testCase.origin), http.StatusNotFound)
			if problem.Code != testCase.want {
				t.Fatalf("code=%q want %q", problem.Code, testCase.want)
			}
			if problem.SourceId != testCase.origin.SourceId {
				t.Fatalf("sourceId=%q want the origin source %q", problem.SourceId, testCase.origin.SourceId)
			}
		})
	}
}

// firstCandidateMatch は、manifest が挙げる候補のエッジ (起点のノードの種別で選ぶ) が持つ最初の
// 関連付けを返す。関連付けの起点と候補が manifest の最初の組であることも確かめる。
func firstCandidateMatch(t *testing.T, handler http.Handler) edgeMatch {
	t.Helper()
	want := candidateEdgeFixture(t)
	edge := fixtureCandidateEdge(t, handler, want)
	matches := decodeEdge(t, handler, edge.Id, "").expandedMatches(t)
	if len(matches) == 0 || len(want.Matches) == 0 {
		t.Fatal("the candidate edge carries no match")
	}
	match, wantMatch := matches[0], want.Matches[0]
	if match.OriginRef.SourceFileName != wantMatch.OriginFileName || match.OriginRef.LineNumber == nil ||
		*match.OriginRef.LineNumber != wantMatch.OriginLineNumber ||
		match.CandidateRef.SourceFileName != wantMatch.CandidateFileName ||
		match.CandidateRef.SequenceNumber == nil ||
		*match.CandidateRef.SequenceNumber != wantMatch.CandidateSequenceNumber {
		t.Fatalf("the first match joins %+v and %+v, want the manifest pair %+v",
			match.OriginRef, match.CandidateRef, wantMatch)
	}
	return match
}

// trailQuery は、opened を開き origin を起点に置き、すべての条件を選んだ要求の文字列を返す。
func trailQuery(opened, origin core.RecordLocator) string {
	return locatorQuery("", opened) + "&" + locatorQuery("origin", origin) + "&" + allMatchConditionsQuery()
}

// locatorQuery はレコードの位置を、接頭辞 prefix を付けた項目の文字列にする。
func locatorQuery(prefix string, locator core.RecordLocator) string {
	name := func(item string) string {
		if prefix == "" {
			return item
		}
		return prefix + strings.ToUpper(item[:1]) + item[1:]
	}
	query := name("sourceId") + "=" + url.QueryEscape(locator.SourceId) + "&" +
		name("sourceContentSha256") + "=" + locator.SourceContentSha256
	for item, value := range map[string]*int64{
		"sequenceNumber": locator.SequenceNumber, "lineNumber": locator.LineNumber,
		"byteOffset": locator.ByteOffset,
	} {
		if value != nil {
			query += "&" + name(item) + "=" + strconv.FormatInt(*value, 10)
		}
	}
	return query
}

func TestRecordsEndpointReportsTheRequestedSourceFailures(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	position := "&sequenceNumber=" + strconv.FormatInt(manifest.MarkiiRecord.SequenceNumber, 10)

	t.Run("source_not_found", func(t *testing.T) {
		problem := requestRecordsError(t, handler, recordsPath+"?sourceId=absent-source"+
			"&sourceContentSha256="+identity.ContentSha256+position, http.StatusNotFound)
		if problem.Code != core.ApiErrorCodeSourceNotFound {
			t.Fatalf("code=%q want source_not_found", problem.Code)
		}
		if problem.SourceId != "absent-source" ||
			problem.SourceContentSha256 != identity.ContentSha256 {
			t.Fatalf("sourceId=%q sourceContentSha256=%q",
				problem.SourceId, problem.SourceContentSha256)
		}
	})
	t.Run("source_hash_mismatch", func(t *testing.T) {
		problem := requestRecordsError(t, handler, recordsPath+"?sourceId="+identity.SourceId+
			"&sourceContentSha256="+absentSha256+position, http.StatusConflict)
		if problem.Code != core.ApiErrorCodeSourceHashMismatch {
			t.Fatalf("code=%q want source_hash_mismatch", problem.Code)
		}
		if problem.SourceContentSha256 != absentSha256 {
			t.Fatalf("sourceContentSha256=%q want the requested value", problem.SourceContentSha256)
		}
		if problem.OriginPath != identity.OriginPath {
			t.Fatalf("originPath=%q want %q", problem.OriginPath, identity.OriginPath)
		}
	})
	t.Run("import_withheld", func(t *testing.T) {
		withheldHandler, withheld := withheldImportResult(t)
		problem := requestRecordsError(t, withheldHandler, recordsPath+"?sourceId="+
			withheld.SourceId+"&sourceContentSha256="+withheld.ContentSha256+position,
			http.StatusConflict)
		if problem.Code != core.ApiErrorCodeImportWithheld {
			t.Fatalf("code=%q want import_withheld", problem.Code)
		}
		if problem.ImportStatusRef == "" {
			t.Fatal("importStatusRef is absent")
		}
	})
}

// 収集元の範囲の外の位置を position_outside_source、範囲の中で一致するレコードが無い
// 位置を record_not_found で読み分ける。
func TestRecordsEndpointSeparatesThePositionOutsideTheSourceFromTheMissingRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	squid := recordsSourceIdentity(t, handler, manifest.SquidRecord.File)
	markii := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	squidScope := manifest.SquidRecord.Scope
	markiiScope := manifest.MarkiiRecord.Scope

	cases := []struct {
		name   string
		query  string
		want   core.ApiErrorCode
		status int
	}{
		{"line number above the scanned range",
			recordsSourceQuery(squid) + "&lineNumber=" +
				strconv.FormatInt(squidScope.ToPosition+1, 10),
			core.ApiErrorCodePositionOutsideSource, http.StatusNotFound},
		{"sequence number below the scanned range",
			recordsSourceQuery(markii) + "&sequenceNumber=" +
				strconv.FormatInt(markiiScope.FromPosition-1, 10),
			core.ApiErrorCodePositionOutsideSource, http.StatusNotFound},
		// scope の positionKind と単位が異なる位置を範囲と比べない。行番号は
		// 通番の範囲の外にあるが、判定は record_not_found に回る。
		{"line number outside a sequence number scope",
			recordsSourceQuery(markii) + "&lineNumber=" +
				strconv.FormatInt(markiiScope.ToPosition+1, 10),
			core.ApiErrorCodeRecordNotFound, http.StatusNotFound},
		{"sequence number inside the scanned range with no record",
			recordsSourceQuery(markii) + "&sequenceNumber=" +
				strconv.FormatInt(markiiScope.FromPosition+1, 10),
			core.ApiErrorCodeRecordNotFound, http.StatusNotFound},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := requestRecordsError(t, handler, recordsPath+"?"+testCase.query,
				testCase.status)
			if problem.Code != testCase.want {
				t.Fatalf("code=%q want %q", problem.Code, testCase.want)
			}
			if problem.SourceId == "" || problem.SourceContentSha256 == "" {
				t.Fatalf("error=%+v carries no source", problem)
			}
		})
	}
	if squidScope.RangeKind != core.RangeKindPositioned ||
		markiiScope.PositionKind != core.PositionKindSequenceNumber {
		t.Fatalf("manifest scopes=%+v and %+v", squidScope, markiiScope)
	}
}

// evt が file のレコードも 200 と fields を返す。取り込みの段階 1 が全レコードに
// 意味付けを与えるため、markii 形式のレコードは not_implemented で返らない。
//
// **応答の fields は、そのレコードが書いた key を持つ。** 対象ファイルの path と
// sha256 が要素に入り、段階 1 が付けた file.path と file.sha256 の意味を持つ。
func TestRecordsEndpointReturnsTheFileEventRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	want := manifest.FileEventRecord
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: want.Format, FileName: want.File, OriginPath: "testdata/" + want.File,
	}))
	identity := recordsSourceIdentity(t, handler, want.File)
	position := "&sequenceNumber=" + strconv.FormatInt(want.SequenceNumber, 10)

	decoded := decodeRecord(t, requestSources(t, handler,
		recordsPath+"?"+recordsSourceQuery(identity)+position), http.StatusOK)

	assertFieldNames(t, decoded.Fields, fileEventFieldNames)
	assertValueStates(t, decoded.Fields, fileEventValueStates)
	assertSemanticByFieldName(t, decoded.Fields, want.SemanticByFieldName)
	if raw, ok := namedFieldText(t, decoded.Fields, "sha256").RawTextValue(); !ok ||
		raw != want.FileSha256 {
		t.Fatalf("sha256 rawText=%q (present %t) want %q", raw, ok, want.FileSha256)
	}
	if len(decoded.ObservationKind.Raw) != want.ObservationKindRaw ||
		string(decoded.ObservationKind.Status) != want.ObservationKindStat {
		t.Fatalf("observationKind=%+v want %d raw items and status %q",
			decoded.ObservationKind, want.ObservationKindRaw, want.ObservationKindStat)
	}
	// イベントの種類の組は、レコードの事象の分類と動作の欄の値である。
	wantKind := core.EventKindPair{
		Category: semanticFieldComparable(t, decoded.Fields, core.SemanticKeyEventCategory),
		Action:   semanticFieldComparable(t, decoded.Fields, core.SemanticKeyEventAction),
	}
	if decoded.EventKind == nil || *decoded.EventKind != wantKind {
		t.Fatalf("eventKind=%+v want %+v", decoded.EventKind, wantKind)
	}
	if decoded.RecordRef.SourceFileName != want.File ||
		decoded.RecordRef.PositionKind != want.PositionKind {
		t.Fatalf("recordRef=%+v want %q and %q",
			decoded.RecordRef, want.File, want.PositionKind)
	}
	if decoded.RecordRef.SequenceNumber == nil ||
		*decoded.RecordRef.SequenceNumber != want.SequenceNumber {
		t.Fatalf("recordRef.sequenceNumber=%v want %d",
			decoded.RecordRef.SequenceNumber, want.SequenceNumber)
	}
	if decoded.RecordRef.LineNumber == nil || *decoded.RecordRef.LineNumber != want.LineNumber {
		t.Fatalf("recordRef.lineNumber=%v want %d",
			decoded.RecordRef.LineNumber, want.LineNumber)
	}
	// 内容の識別が食い違う要求は source_hash_mismatch の行に該当する。
	mismatch := requestRecordsError(t, handler, recordsPath+"?sourceId="+identity.SourceId+
		"&sourceContentSha256="+absentSha256+position, http.StatusConflict)
	if mismatch.Code != core.ApiErrorCodeSourceHashMismatch {
		t.Fatalf("code=%q want source_hash_mismatch", mismatch.Code)
	}
}

// 同じ収集元の evt が net のレコードを指す要求も 200 と fields を返す。
//
// **同じ収集元の 2 件が別の name の集合を返す。** 集合を決めるのは返すレコードである。
func TestRecordsEndpointReturnsTheSiblingCommunicationRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	source := manifest.FileEventRecord
	want := manifest.RecordWithSemantics
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: source.Format, FileName: source.File, OriginPath: "testdata/" + source.File,
	}))
	identity := recordsSourceIdentity(t, handler, source.File)

	decoded := decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber="+
		strconv.FormatInt(want.SequenceNumber, 10)), http.StatusOK)

	assertFieldNames(t, decoded.Fields, siblingCommunicationFieldNames)
	if len(decoded.ObservationKind.Raw) != want.ObservationKindRaw ||
		string(decoded.ObservationKind.Status) != want.ObservationKindStat {
		t.Fatalf("observationKind=%+v want %d raw items and status %q",
			decoded.ObservationKind, want.ObservationKindRaw, want.ObservationKindStat)
	}
}

// sequenceNumber と lineNumber を両方与えた要求では、2 つの位置が同じ 1 件のレコードを
// 指す。別のレコードを指す要求は record_not_found で失敗する。
//
// fixture の 3 行は、同じ収集元にある位置の異なる 3 件のレコードを表す。
func TestRecordsEndpointRequiresTheTwoPositionsToPointAtOneRecord(t *testing.T) {
	manifest := recordsFixtures(t)
	want := manifest.RecordWithSemantics
	neighbour := manifest.NeighbouringRecord
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: want.Format, FileName: want.File, OriginPath: "testdata/" + want.File,
	}))
	identity := recordsSourceIdentity(t, handler, want.File)
	sequence := "&sequenceNumber=" + strconv.FormatInt(want.SequenceNumber, 10)

	// 通番は 3 行目、行番号は 2 行目を指す。どちらの位置にもレコードがあり、2 つは別の
	// レコードである。200 でどちらかを返す応答を落第させる。
	problem := requestRecordsError(t, handler, recordsPath+"?"+recordsSourceQuery(identity)+
		sequence+"&lineNumber="+strconv.FormatInt(neighbour.LineNumber, 10),
		http.StatusNotFound)
	if problem.Code != core.ApiErrorCodeRecordNotFound {
		t.Fatalf("code=%q want record_not_found", problem.Code)
	}
	if problem.SourceId != identity.SourceId ||
		problem.SourceContentSha256 != identity.ContentSha256 {
		t.Fatalf("sourceId=%q sourceContentSha256=%q want the requested values",
			problem.SourceId, problem.SourceContentSha256)
	}
	// 2 つの位置が同じレコードを指す要求は成功し、recordRef が両方の位置を持つ。
	sameRecord := recordsPath + "?" + recordsSourceQuery(identity) + sequence +
		"&lineNumber=" + strconv.FormatInt(want.LineNumber, 10)
	decoded := decodeRecord(t, requestSources(t, handler, sameRecord), http.StatusOK)
	if decoded.RecordRef.SequenceNumber == nil ||
		*decoded.RecordRef.SequenceNumber != want.SequenceNumber {
		t.Fatalf("recordRef.sequenceNumber=%v want %d",
			decoded.RecordRef.SequenceNumber, want.SequenceNumber)
	}
	if decoded.RecordRef.LineNumber == nil || *decoded.RecordRef.LineNumber != want.LineNumber {
		t.Fatalf("recordRef.lineNumber=%v want %d",
			decoded.RecordRef.LineNumber, want.LineNumber)
	}
	// 上の 2 つの要求が別の行番号を指していたことを、manifest の側でも固定する。
	if neighbour.LineNumber == want.LineNumber {
		t.Fatalf("the manifest gives the two records the same lineNumber %d", want.LineNumber)
	}
}

// 応答は原資料の byte 列をそのまま持ち、log と端末出力の側で無害化する。
func TestRecordsEndpointKeepsTheRawTextAndSanitizesTheDiagnosticText(t *testing.T) {
	const bellByte = "\x07"
	// 1 行の Squid combined の user agent に制御文字を 1 byte 入れた収集元である。
	// この判断の test のために組み立てた入力であり、原資料の書き換えではない。
	content := `192.0.2.10 - - [01/Feb/2000:13:55:00 +0900] ` +
		`"GET http://198.51.100.42/first HTTP/1.1" 200 4096 "-" "curl` + bellByte + `/8" ` +
		"TCP_MEM_HIT:HIER_NONE\n"
	plan := pipeline.SourcePlan{
		FormatKey: squidFormatKey,
		FileName:  "control.log", OriginPath: "testdata/control.log",
	}
	handler := testHandler(recordsImportResultOfText(t, plan, content))
	identity := recordsSourceIdentity(t, handler, plan.FileName)

	response := requestSources(t, handler,
		recordsPath+"?"+recordsSourceQuery(identity)+"&lineNumber=1")
	body := response.Body.String()
	decoded := decodeRecord(t, response, http.StatusOK)

	if decoded.RawText != strings.TrimSuffix(content, "\n") {
		t.Fatalf("rawText=%q want the original line", decoded.RawText)
	}
	// 応答の byte 列は制御文字を JSON escape で表す。末尾のレコード区切りの改行を除く。
	for i, character := range []byte(strings.TrimSuffix(body, "\n")) {
		if character < 0x20 {
			t.Fatalf("body byte %d is the control character %#x", i, character)
		}
	}
	// 端末出力と log の側は \xNN 表記へ直す。
	sanitized := output.Sanitize(decoded.RawText)
	if !strings.Contains(sanitized, `\x07`) || strings.Contains(sanitized, bellByte) {
		t.Fatalf("sanitized raw text=%q", sanitized)
	}
}

// **上限と続きを取る位置の項目を受けない。** `/api/v0/records` はレコード 1 件を返す。
func TestRecordsEndpointRejectsALimitAndACursor(t *testing.T) {
	manifest := recordsFixtures(t)
	handler := testHandler(twoFormatImportResult(t))
	identity := recordsSourceIdentity(t, handler, manifest.MarkiiRecord.File)
	position := "&sequenceNumber=" + strconv.FormatInt(manifest.MarkiiRecord.SequenceNumber, 10)
	located := recordsPath + "?" + recordsSourceQuery(identity) + position

	for _, testCase := range []struct {
		name  string
		query string
	}{
		{"limit", "&limit=10"},
		{"cursor", "&cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			problem := requestRecordsError(t, handler, located+testCase.query,
				http.StatusBadRequest)
			if problem.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", problem.Code)
			}
			if len(problem.MissingParameters) != 0 {
				t.Fatalf("missingParameters=%v want none", problem.MissingParameters)
			}
		})
	}
}

// assertFieldNames は応答の fields の要素数と name の並びを確かめる。
func assertFieldNames(t *testing.T, got []core.RecordField, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fields=%d want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("fields[%d].name=%q want %q", i, got[i].Name, name)
		}
	}
}

// namedFieldText は name で指した項目の値を返す。
func namedFieldText(
	t *testing.T, fields []core.RecordField, name string,
) core.RawAndNormalized {
	t.Helper()
	for _, field := range fields {
		if field.Name == name && field.Text != nil {
			return *field.Text
		}
	}
	t.Fatalf("the fields carry no text item named %q", name)
	return core.RawAndNormalized{}
}

// assertValueStates は応答の fields の name ごとの valueState を確かめる。
func assertValueStates(t *testing.T, got []core.RecordField, want map[string]core.ValueState) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fields=%d want %d", len(got), len(want))
	}
	for _, field := range got {
		wanted, listed := want[field.Name]
		if !listed {
			t.Fatalf("fields carries %q, which the expected value states do not list", field.Name)
		}
		if state := valueStateOf(field); state != wanted {
			t.Fatalf("fields[%q].valueState=%q want %q", field.Name, state, wanted)
		}
	}
}

// valueStateOf は RecordField が持つ値の状態を返す。Kind が対応する値を持たない
// RecordField は Validate を通らないため、どちらも無い状態は空の値で落第させる。
func valueStateOf(field core.RecordField) core.ValueState {
	switch {
	case field.Timestamp != nil:
		return field.Timestamp.ValueState
	case field.Text != nil:
		return field.Text.ValueState
	default:
		return ""
	}
}

func assertMissingParameters(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("missingParameters=%v want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("missingParameters[%d]=%q want %q", i, got[i], name)
		}
	}
}

// recordsSourceQuery は収集元を指す 2 項目の query を返す。
func recordsSourceQuery(identity core.SourceIdentity) string {
	return "sourceId=" + identity.SourceId + "&sourceContentSha256=" + identity.ContentSha256
}

func decodeRecord(t *testing.T, response *httptest.ResponseRecorder, wantStatus int) recordResponse {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var decoded recordResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// semanticFieldComparable は語彙の項目 semantic を持つ 1 件目の欄の、比べる値を返す。
func semanticFieldComparable(t *testing.T, fields []core.RecordField, semantic core.SemanticKey) string {
	t.Helper()
	for _, field := range fields {
		if field.Semantic != semantic || field.Text == nil {
			continue
		}
		if value, ok := field.Text.ComparableValue(); ok {
			return value
		}
	}
	t.Fatalf("no field carries a comparable value of %q", semantic)
	return ""
}

func requestRecordsError(t *testing.T, handler http.Handler, path string, wantStatus int) core.ApiError {
	t.Helper()
	response := requestSources(t, handler, path)
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var problem core.ApiError
	decodeJSON(t, response.Body, &problem)
	if err := problem.Validate(); err != nil {
		t.Fatalf("invalid API error: %v", err)
	}
	return problem
}

func recordsFixtures(t *testing.T) recordsManifest {
	t.Helper()
	data, err := os.ReadFile(runFixtureDir + "records-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest recordsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SquidRecord.File == "" || manifest.MarkiiRecord.File == "" ||
		manifest.FileEventRecord.File == "" ||
		manifest.RecordWithSemantics.File != manifest.FileEventRecord.File ||
		manifest.NeighbouringRecord.File != manifest.FileEventRecord.File {
		t.Fatalf("records manifest carries no file names: %+v", manifest)
	}
	return manifest
}

// recordsSourceIdentity は`/api/v0/sources` の応答から、file 名に対応する収集元の識別を読む。
func recordsSourceIdentity(t *testing.T, handler http.Handler, fileName string) core.SourceIdentity {
	t.Helper()
	sources := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	for _, item := range sources.Sources {
		if item.Source.FileName == fileName {
			return item.Source
		}
	}
	t.Fatalf("no source is named %q", fileName)
	return core.SourceIdentity{}
}

// recordsImportResultOfText は file を作らずに 1 件の収集元を取り込む。
//
// 原文に制御文字を含むレコードの期待値を、fixture の byte 列ではなく test の中で固定する。
func recordsImportResultOfText(t *testing.T, plan pipeline.SourcePlan, content string,
) pipeline.ImportResult {
	t.Helper()
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open: func(string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
		Parsers:  testFormatRegistry(t),
		Minter:   pipeline.DigestMinter{},
		Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize,
		Revision: "api-records-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{plan})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// 読めなかったレコードの位置への要求は、レコードが無い位置と別の code で返る。
//
// 行番号で指す場合を確かめる。partial.log の 2 行目は空行、3 行目は Squid combined の
// 項目数を満たさず、どちらも文字列へ分ける段階で失敗する。
func TestRecordsEndpointSeparatesTheUnreadableRecordByLineNumber(t *testing.T) {
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: squidFormatKey,
		FileName:  "partial.log", OriginPath: "testdata/partial.log",
	})
	handler := testHandler(result)
	identity := recordsSourceIdentity(t, handler, "partial.log")

	lines := failedLineNumbers(t, result)
	if len(lines) == 0 {
		t.Fatal("the import carries no failure with a line number")
	}
	for _, line := range lines {
		problem := requestRecordsError(t, handler, recordsPath+"?"+
			recordsSourceQuery(identity)+"&lineNumber="+strconv.FormatInt(line, 10),
			http.StatusNotFound)
		if problem.Code != core.ApiErrorCodeRecordUnreadable {
			t.Errorf("line %d: code=%q want record_unreadable", line, problem.Code)
		}
	}

	// 読めた位置は 200 で返る。**読めなかった位置と同じ扱いにしない。**
	decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&lineNumber=1"), http.StatusOK)
}

// 原資料から通番を読めた後に失敗したレコードは、通番で指しても別の code で返る。
func TestRecordsEndpointSeparatesTheUnreadableRecordBySequenceNumber(t *testing.T) {
	// unreadable-markii.log の 1 行目はヘッダーの日時が壊れており、通番を読んだ後の
	// 段階で失敗する。2 行目は読める。
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "unreadable-markii.log", OriginPath: "testdata/unreadable-markii.log",
	})
	handler := testHandler(result)
	identity := recordsSourceIdentity(t, handler, "unreadable-markii.log")

	failed := failedSequenceNumbers(t, result)
	if len(failed) == 0 {
		t.Fatal("the import carries no failure with a sequence number")
	}
	for _, sequenceNumber := range failed {
		problem := requestRecordsError(t, handler, recordsPath+"?"+
			recordsSourceQuery(identity)+"&sequenceNumber="+
			strconv.FormatInt(sequenceNumber, 10), http.StatusNotFound)
		if problem.Code != core.ApiErrorCodeRecordUnreadable {
			t.Errorf("sn=%d: code=%q want record_unreadable", sequenceNumber, problem.Code)
		}
	}

	// 取り込みが読めた通番は 200 で返る。
	decodeRecord(t, requestSources(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber=22"), http.StatusOK)
}

// 2 つの位置が別のレコードを指す要求は、読めなかった位置として返らない。
func TestRecordsEndpointRejectsThePositionsOfDifferentRecords(t *testing.T) {
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "unreadable-markii.log", OriginPath: "testdata/unreadable-markii.log",
	})
	handler := testHandler(result)
	identity := recordsSourceIdentity(t, handler, "unreadable-markii.log")

	// sn=25 は読めるレコードを指し、1 行目は別の失敗したレコードの行である。
	problem := requestRecordsError(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber=25&lineNumber=1",
		http.StatusNotFound)
	if problem.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", problem.Code)
	}
}

// 取り込みが失敗したどのレコードとも別の位置は、レコードが無い位置として返る。
func TestRecordsEndpointSeparatesTheAbsentPositionFromTheUnreadableOne(t *testing.T) {
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "unreadable-markii.log", OriginPath: "testdata/unreadable-markii.log",
	})
	handler := testHandler(result)
	identity := recordsSourceIdentity(t, handler, "unreadable-markii.log")

	// 23 は収集元の走査した範囲 (21 から 25) の内側にあり、どのレコードも持たない。
	problem := requestRecordsError(t, handler, recordsPath+"?"+
		recordsSourceQuery(identity)+"&sequenceNumber=23", http.StatusNotFound)
	if problem.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", problem.Code)
	}
}

// failedLineNumbers は取り込みが失敗したレコードの行番号を返す。
func failedLineNumbers(t *testing.T, result pipeline.ImportResult) []int64 {
	t.Helper()
	lines := make([]int64, 0)
	for _, status := range result.Statuses() {
		for _, failure := range status.Failures {
			if failure.LineNumber != nil {
				lines = append(lines, *failure.LineNumber)
			}
		}
	}
	return lines
}

// failedSequenceNumbers は取り込みが失敗したレコードの通番を返す。
func failedSequenceNumbers(t *testing.T, result pipeline.ImportResult) []int64 {
	t.Helper()
	numbers := make([]int64, 0)
	for _, status := range result.Statuses() {
		for _, failure := range status.Failures {
			if failure.RecordRef != nil && failure.RecordRef.SequenceNumber != nil {
				numbers = append(numbers, *failure.RecordRef.SequenceNumber)
			}
		}
	}
	return numbers
}
