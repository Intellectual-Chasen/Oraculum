package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// localTimeSpec は UTC からのずれを持たない地方時を持つ Squid の logformat である。
const localTimeSpec = `%{%Y/%m/%d %H:%M:%S}tl.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`

// squidSource は取り込む Squid の収集元 1 件である。
type squidSource struct {
	name, spec, content string
	// terminal は取り込みの起動で収集元に指定する端末とずれである。指定しない収集元では nil である。
	terminal *pipeline.SourceTerminal
}

// localProxyLine は地方時の Squid の 1 行である。
func localProxyLine(at, host string) string {
	return at + "     10 192.0.2.20 TCP_MISS/200 100 GET http://" + host +
		"/ - HIER_DIRECT/198.51.100.9 text/html\n"
}

// squidStore は Squid の収集元を並べた順に取り込み、file 名から内容の識別を探す表を返す。
func squidStore(t *testing.T, sources ...squidSource) (*pipeline.MemoryStore, map[string]string) {
	t.Helper()
	contents := map[string]string{}
	plans := make([]pipeline.SourcePlan, 0, len(sources))
	for _, source := range sources {
		contents[source.name] = source.content
		spec := source.spec
		plans = append(plans, pipeline.SourcePlan{
			OriginPath: source.name, FileName: source.name, FormatKey: "squid_logformat", FormatSpec: &spec,
			Terminal: source.terminal,
		})
	}
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open:    func(path string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(contents[path])), nil },
		Parsers: testFormatRegistry(t), Minter: pipeline.DigestMinter{},
		Ordinals: pipeline.NewInMemoryOrdinals(), Sanitize: output.Sanitize,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	digests := map[string]string{}
	for _, entry := range entries {
		digests[entry.Identity.FileName] = entry.Identity.ContentSha256
	}
	return pipeline.NewMemoryStore(result, &testAssertionClock{}), digests
}

// timeInterpretationStore は、地方時の収集元 1 件と、epoch の時刻の収集元 1 件を取り込む。
// 地方時のレコードは、epoch のレコード 2 件の UTC の時刻の間の字面を持つ。
func timeInterpretationStore(t *testing.T) (*pipeline.MemoryStore, string) {
	t.Helper()
	store, digests := squidStore(t,
		squidSource{name: "local.log", spec: localTimeSpec,
			content: localProxyLine("2031/04/05 06:07:08.250", "a.example.test")},
		squidSource{name: "utc.log", spec: "squid",
			content: "1933135625.000     10 192.0.2.21 TCP_MISS/200 100 GET http://b.example.test/ - " +
				"HIER_DIRECT/198.51.100.9 text/html\n" +
				"1933135630.000     10 192.0.2.21 TCP_MISS/200 100 GET http://c.example.test/ - " +
				"HIER_DIRECT/198.51.100.9 text/html\n"})
	return store, digests["local.log"]
}

// sourceInterpretationBody は収集元の時刻の解釈を記録する要求の本文である。
func sourceInterpretationBody(content string, offset core.UtcOffset) map[string]any {
	return map[string]any{
		"target": map[string]any{"kind": "source", "sourceContentSha256": content},
		"author": "analyst-a", "basis": map[string]any{"note": "合成の根拠"}, "timeOffset": string(offset),
	}
}

// 取り込みの起動でずれを指定した収集元は、そのずれを一覧に出す。指定の無い収集元は出さない。
// 分析者が解釈を記録しても、起動で指定したずれは同じ値のまま出る。
func TestSourcesReportTheOffsetSpecifiedAtImport(t *testing.T) {
	offset := core.UtcOffset("+09:00")
	store, digests := squidStore(t,
		squidSource{name: "specified.log", spec: localTimeSpec,
			content:  localProxyLine("2031/04/05 06:07:08.250", "a.example.test"),
			terminal: &pipeline.SourceTerminal{TimeOffset: &offset}},
		squidSource{name: "plain.log", spec: localTimeSpec,
			content: localProxyLine("2031/04/05 06:07:09.250", "b.example.test")})
	handler := handlerOf(store)
	offsetsOf := func() map[string]core.UtcOffset {
		t.Helper()
		offsets := map[string]core.UtcOffset{}
		for _, item := range decodeSources(t, requestPath(t, handler, http.MethodGet, "/api/v0/sources"),
			http.StatusOK).Sources {
			offsets[item.Source.FileName] = item.ImportTimeOffset
		}
		return offsets
	}
	want := map[string]core.UtcOffset{"specified.log": "+09:00", "plain.log": ""}
	if got := offsetsOf(); !maps.Equal(got, want) {
		t.Fatalf("the import offsets are %v, want %v", got, want)
	}
	requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, sourceInterpretationBody(digests["specified.log"], "+00:00")), http.StatusCreated)
	if got := offsetsOf(); !maps.Equal(got, want) {
		t.Fatalf("the import offsets after the analyst interpretation are %v, want %v", got, want)
	}
}

// **期間で絞ると、ずれの決まらない地方時のレコードは期間の判定から外れ、その件数を返す。**
// 期間で絞らない要求は件数の組を返さない。
func TestTimelineReportsTheLocalRecordsThePeriodCannotJudge(t *testing.T) {
	store, _ := timeInterpretationStore(t)
	handler := handlerOf(store)
	period := decodeTimeline(t, handler, "timeFrom=2031-04-05T00:00:00Z&timeFromPrecision=second&filterUnit=second")
	if period.PeriodUnjudged == nil || *period.PeriodUnjudged != (periodUnjudged{LocalRecordCount: 1}) {
		t.Fatalf("periodUnjudged=%+v, want the local record alone", period.PeriodUnjudged)
	}
	if whole := decodeTimeline(t, handler, ""); whole.PeriodUnjudged != nil {
		t.Fatalf("a request without a period returned periodUnjudged=%+v", whole.PeriodUnjudged)
	}
}

// 解釈を記録した地方時の収集元は、解釈で読んだ収録範囲で要求の期間との重なりを判定する。
// 収集元の識別の収録範囲は原資料の文字列から定まる値のまま、持たない。
func TestTimeInterpretationGivesTheLocalSourceItsRecordingRange(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	handler := handlerOf(store)
	// 地方時の収集元の唯一のレコードの秒だけを指す期間。
	period := "timeFrom=2031-04-05T06:07:08Z&timeFromPrecision=second" +
		"&timeTo=2031-04-05T06:07:08Z&timeToPrecision=second&filterUnit=second"
	coverageOf := func(timeline timelineResponse) core.SourceCoverage {
		t.Helper()
		for _, coverage := range timeline.SourceCoverages {
			if coverage.SourceFileName == "local.log" {
				return coverage
			}
		}
		t.Fatal("the timeline carries no coverage of the local source")
		return core.SourceCoverage{}
	}
	if state := coverageOf(decodeTimeline(t, handler, period)).State; state != core.CoverageStateRecordingRangeUnknown {
		t.Fatalf("the local source is %q before the interpretation, want recording_range_unknown", state)
	}
	requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, sourceInterpretationBody(localContent, "+00:00")), http.StatusCreated)
	coverage := coverageOf(decodeTimeline(t, handler, period))
	if coverage.State != core.CoverageStateCovered || coverage.ObservedRangeFirst == nil ||
		coverage.ObservedRangeFirst.Interpretation == nil ||
		*coverage.ObservedRangeFirst.RawText != "2031/04/05 06:07:08.250" {
		t.Fatalf("the local source coverage is %+v, want covered by the interpreted range", coverage)
	}
	// 収集元の識別は解釈を持たない。解釈で読んだ期間は組の別の項目が持つ。
	for _, item := range decodeSources(t, requestPath(t, handler, http.MethodGet, "/api/v0/sources"),
		http.StatusOK).Sources {
		encoded, err := json.Marshal(item.Source)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), `"interpretation"`) {
			t.Fatalf("the source identity carries the analyst interpretation: %s", encoded)
		}
	}
}

// 解釈を記録した地方時の収集元は、解釈で読んだ観測期間を、地方時の文字列と分析者のずれの組で
// 一覧に出す。解釈を持たない収集元はその期間を出さない。
//
// 分析者は、ずれを外した地方時の期間で端末の割当を記録する。保存した割当は地方時の期間を
// そのまま持つ。ずれを持つ期間の割当は受け付けない。
func TestTimeInterpretationGivesTheLocalSourceAnAssignmentRange(t *testing.T) {
	store, digests := squidStore(t, squidSource{name: "local.log", spec: localTimeSpec,
		content: localProxyLine("2031/04/05 06:07:08.250", "a.example.test") +
			localProxyLine("2031/04/05 06:07:30.500", "b.example.test")})
	localContent := digests["local.log"]
	handler := handlerOf(store)
	localItem := func() sourceResponseItem {
		t.Helper()
		for _, item := range decodeSources(t, requestPath(t, handler, http.MethodGet, "/api/v0/sources"),
			http.StatusOK).Sources {
			if item.Source.FileName == "local.log" {
				return item
			}
		}
		t.Fatal("the sources carry no local source")
		return sourceResponseItem{}
	}
	if item := localItem(); item.InterpretedObservedRange != nil {
		t.Fatalf("the local source carries %+v before the interpretation", item.InterpretedObservedRange)
	}
	requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, sourceInterpretationBody(localContent, "+09:00")), http.StatusCreated)
	item := localItem()
	interpreted := item.InterpretedObservedRange
	if item.Source.ObservedRangeFirst != nil || interpreted == nil {
		t.Fatalf("the local source carries the identity range %+v and the interpreted range %+v, "+
			"want only the interpreted range", item.Source.ObservedRangeFirst, interpreted)
	}
	for _, end := range []struct {
		timestamp core.Timestamp
		want      string
	}{{interpreted.From, "2031-04-05T06:07:08.250"}, {interpreted.To, "2031-04-05T06:07:30.500"}} {
		if *end.timestamp.Normalized != end.want || end.timestamp.Interpretation == nil ||
			end.timestamp.Interpretation.Offset != "+09:00" {
			t.Errorf("an end of the interpreted range is %+v, want %s read at +09:00", end.timestamp, end.want)
		}
	}
	local := map[string]any{"from": withoutInterpretation(t, interpreted.From),
		"to": withoutInterpretation(t, interpreted.To)}
	body := func(validRange any) []byte {
		return encodeBody(t, map[string]any{
			"clientIp": "192.0.2.20", "terminalHostname": "local-host.example.test",
			"sourceId": item.Source.SourceId, "sourceContentSha256": item.Source.ContentSha256,
			"assignmentValidRange": validRange, "derivation": "合成の根拠",
			"basisRecordRefs": []map[string]any{{
				"sourceContentSha256": item.Source.ContentSha256,
				"positionKind":        string(core.PositionKindLineNumber), "lineNumber": 1,
			}},
			"author": "analyst", "appliesToSourceId": item.Source.SourceId,
		})
	}
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath, body(interpreted), http.StatusBadRequest)
	var created core.TerminalAssignment
	decodeInto(t, requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath, body(local),
		http.StatusCreated), &created)
	if *created.AssignmentValidRange.From.Normalized != "2031-04-05T06:07:08.250" ||
		*created.AssignmentValidRange.To.Normalized != "2031-04-05T06:07:30.500" ||
		created.AssignmentValidRange.From.Interpretation != nil {
		t.Errorf("the recorded assignment carries the range %+v, want the local range without the offset",
			created.AssignmentValidRange)
	}
}

// withoutInterpretation は、分析者のずれの項目を外した時刻の JSON の項目を返す。
func withoutInterpretation(t *testing.T, timestamp core.Timestamp) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(timestamp)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	decodeInto(t, encoded, &fields)
	delete(fields, "interpretation")
	return fields
}

// ずれの決まらない地方時の行は、収集元ごとにまとまって並ぶ。2 つの収集元の文字列の先後を
// 事実として混ぜない。
func TestTimelineKeepsTheUndeterminedRowsOfEachSourceTogether(t *testing.T) {
	store, _ := squidStore(t,
		squidSource{name: "first.log", spec: localTimeSpec,
			content: localProxyLine("2031/04/05 06:07:08.000", "a.example.test") +
				localProxyLine("2031/04/05 06:07:10.000", "b.example.test")},
		squidSource{name: "second.log", spec: localTimeSpec,
			content: localProxyLine("2031/04/05 06:07:09.000", "c.example.test")})
	timeline := decodeTimeline(t, handlerOf(store), "")
	files := timelineFiles(timeline)
	grouped := strings.Join(files, ",")
	if grouped != "first.log,first.log,second.log" && grouped != "second.log,first.log,first.log" {
		t.Fatalf("the undetermined rows are ordered %v, want the rows of each source together", files)
	}
}

// timelineFiles は時系列の行を、収集元の file 名の並びへ直す。
func timelineFiles(timeline timelineResponse) []string {
	files := make([]string, 0, len(timeline.Entries))
	for _, entry := range timeline.Entries {
		files = append(files, entry.RecordRef.SourceFileName)
	}
	return files
}

func requireTimelineFiles(t *testing.T, timeline timelineResponse, want ...string) {
	t.Helper()
	got := timelineFiles(timeline)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the timeline lists %v, want %v", got, want)
	}
}

// localEntry は時系列の中の地方時の収集元の行を返す。
func localEntry(t *testing.T, timeline timelineResponse) timelineEntry {
	t.Helper()
	for _, entry := range timeline.Entries {
		if entry.RecordRef.SourceFileName == "local.log" {
			return entry
		}
	}
	t.Fatal("the timeline lists no record of the local source")
	return timelineEntry{}
}

type timeInterpretationItem struct {
	Assertion    core.Assertion             `json:"assertion"`
	TargetOrigin core.AssertionTargetOrigin `json:"targetOrigin"`
}

// 収集元の時刻の解釈を記録すると、その収集元の地方時が UTC の時刻と同じ軸に並ぶ。
// 解釈を変えると並びが変わり、取り消すと未定の行へ戻る。変更と取り消しは改訂の履歴に残る。
func TestTimeInterpretationOfASourcePlacesItsLocalTimesOnTheUtcAxis(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	handler := handlerOf(store)

	before := decodeTimeline(t, handler, "")
	requireTimelineFiles(t, before, "utc.log", "utc.log", "local.log")
	if before.UndatedRecordCount != 0 {
		t.Fatalf("undatedRecordCount=%d, want the local time listed after the dated rows", before.UndatedRecordCount)
	}
	if local := localEntry(t, before); local.EventTime.Interpretation != nil {
		t.Fatalf("the local time carries an interpretation before one is recorded: %+v", local.EventTime)
	}
	basis := []core.AssertionRecordRef{
		core.NewAssertionRecordRef(before.Entries[2].RecordRef),
		core.NewAssertionRecordRef(before.Entries[0].RecordRef),
	}
	body := map[string]any{
		"target":     map[string]any{"kind": "source", "sourceContentSha256": localContent},
		"author":     "analyst-a",
		"basis":      map[string]any{"note": "同じ要求を記録した UTC の行と比べた", "recordRefs": basis},
		"timeOffset": "+00:00",
	}
	var created timeInterpretationItem
	decodeInto(t, requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, body), http.StatusCreated), &created)
	if created.TargetOrigin != core.AssertionTargetOriginObservation {
		t.Errorf("targetOrigin=%q, want observation", created.TargetOrigin)
	}
	if len(created.Assertion.Basis.RecordRefs) != len(basis) {
		t.Errorf("the basis carries %d records, want the %d compared records",
			len(created.Assertion.Basis.RecordRefs), len(basis))
	}

	interpreted := decodeTimeline(t, handler, "")
	requireTimelineFiles(t, interpreted, "utc.log", "local.log", "utc.log")
	local := localEntry(t, interpreted)
	if local.EventTime.Interpretation == nil || local.EventTime.Interpretation.Offset != "+00:00" ||
		local.EventTime.Interpretation.AssertionId != created.Assertion.Id {
		t.Fatalf("the local time carries %+v, want the recorded interpretation", local.EventTime.Interpretation)
	}
	if *local.EventTime.RawText != *localEntry(t, before).EventTime.RawText ||
		local.EventTime.OffsetState != core.OffsetStateItemAbsent {
		t.Fatalf("the interpretation rewrote the source items: %+v", local.EventTime)
	}

	var duplicate struct {
		Code     core.ApiErrorCode      `json:"code"`
		Conflict timeInterpretationItem `json:"conflict"`
	}
	decodeInto(t, requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, body), http.StatusConflict), &duplicate)
	if duplicate.Code != core.ApiErrorCodeAssertionChanged || duplicate.Conflict.Assertion.Id != created.Assertion.Id {
		t.Fatalf("the second interpretation answered %+v, want assertion_changed with the recorded one", duplicate)
	}

	revisionPath := withAllMatchConditions(assertionsPath + "/" + created.Assertion.Id)
	body["state"], body["timeOffset"] = "active", "+09:00"
	body["baseRevision"] = created.Assertion.RevisionNumber
	requestJSON(t, handler, http.MethodPut, revisionPath, encodeBody(t, body), http.StatusOK)
	requireTimelineFiles(t, decodeTimeline(t, handler, ""), "local.log", "utc.log", "utc.log")

	body["state"] = "withdrawn"
	body["baseRevision"] = created.Assertion.RevisionNumber + 1
	var withdrawn timeInterpretationItem
	decodeInto(t, requestJSON(t, handler, http.MethodPut, revisionPath, encodeBody(t, body), http.StatusOK),
		&withdrawn)
	after := decodeTimeline(t, handler, "")
	requireTimelineFiles(t, after, "utc.log", "utc.log", "local.log")
	if localEntry(t, after).EventTime.Interpretation != nil {
		t.Fatal("the local time keeps an interpretation after the withdrawal")
	}
	offsets := []core.UtcOffset{}
	for _, revision := range withdrawn.Assertion.History {
		offsets = append(offsets, *revision.TimeOffset)
	}
	if withdrawn.Assertion.State != core.AssertionStateWithdrawn ||
		strings.Join([]string{string(offsets[0]), string(offsets[1])}, ",") != "+00:00,+09:00" {
		t.Fatalf("state=%q history offsets=%v, want withdrawn after +00:00 and +09:00",
			withdrawn.Assertion.State, offsets)
	}
}

// 解釈を記録した収集元の元レコードは、時刻の欄に時系列と同じ分析者のずれを持つ。原資料の文字列は
// 変えない。取り消すとずれを持たない。
func TestTimeInterpretationReachesTheRecordTimeField(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	handler := handlerOf(store)
	var local core.SourceIdentity
	for _, item := range decodeSources(t, requestPath(t, handler, http.MethodGet, "/api/v0/sources"),
		http.StatusOK).Sources {
		if item.Source.ContentSha256 == localContent {
			local = item.Source
		}
	}
	timeField := func() core.Timestamp {
		t.Helper()
		record := decodeRecord(t, requestSources(t, handler,
			recordsPath+"?"+recordsSourceQuery(local)+"&lineNumber=1"), http.StatusOK)
		for _, field := range record.Fields {
			if field.Semantic == core.SemanticKeyEventTime && field.Timestamp != nil {
				return *field.Timestamp
			}
		}
		t.Fatal("the record carries no event time field")
		return core.Timestamp{}
	}
	if before := timeField(); before.Interpretation != nil {
		t.Fatalf("the time field carries %+v before the interpretation", before.Interpretation)
	}
	body := sourceInterpretationBody(localContent, "+09:00")
	var created timeInterpretationItem
	decodeInto(t, requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, body), http.StatusCreated), &created)
	interpreted := timeField()
	if interpreted.Interpretation == nil || interpreted.Interpretation.Offset != "+09:00" ||
		interpreted.Interpretation.AssertionId != created.Assertion.Id ||
		*interpreted.RawText != "2031/04/05 06:07:08.250" {
		t.Fatalf("the time field is %+v, want the source text read at +09:00", interpreted)
	}
	body["state"] = "withdrawn"
	body["baseRevision"] = created.Assertion.RevisionNumber
	requestJSON(t, handler, http.MethodPut,
		withAllMatchConditions(assertionsPath+"/"+created.Assertion.Id), encodeBody(t, body), http.StatusOK)
	if after := timeField(); after.Interpretation != nil {
		t.Fatalf("the time field keeps %+v after the withdrawal", after.Interpretation)
	}
}

// 収集元を指す所見はずれを必須とし、他の対象の所見はずれを持てない。
func TestTimeInterpretationRequiresTheOffsetOnASourceAlone(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	handler := handlerOf(store)
	basis := map[string]any{"note": "合成の根拠", "recordRefs": []any{}}
	cases := map[string]map[string]any{
		"a source without an offset": {
			"target": map[string]any{"kind": "source", "sourceContentSha256": localContent},
			"author": "analyst-a", "basis": basis,
		},
		"a malformed offset": {
			"target": map[string]any{"kind": "source", "sourceContentSha256": localContent},
			"author": "analyst-a", "basis": basis, "timeOffset": "+9",
		},
		"a node with an offset": {
			"target": map[string]any{"kind": "node", "nodeId": "n:synthetic"},
			"author": "analyst-a", "basis": basis, "timeOffset": "+09:00",
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
				encodeBody(t, body), http.StatusBadRequest)
		})
	}
	if listed := store.Assertions().List(); len(listed) != 0 {
		t.Fatalf("the store holds %d assertions after the rejected requests", len(listed))
	}
}

// 同じ収集元への解釈の記録が同時に届いても、1 件だけが 201 になり、残りは 409 になる。
func TestConcurrentTimeInterpretationsOfOneSourceStoreOne(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	handler := handlerOf(store)
	body := encodeBody(t, map[string]any{
		"target": map[string]any{"kind": "source", "sourceContentSha256": localContent},
		"author": "analyst-a", "basis": map[string]any{"note": "合成の根拠"}, "timeOffset": "+09:00",
	})
	// グラフを先に組み、要求が組み立てを待たずに記録へ進むようにする。
	decodeTimeline(t, handler, "")
	const attempts = 16
	statuses := make(chan int, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodPost, withAllMatchConditions(assertionsPath),
				bytes.NewReader(body))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			statuses <- recorder.Code
		}()
	}
	wait.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != attempts-1 {
		t.Fatalf("statuses %v, want one 201 and %d 409", counts, attempts-1)
	}
	if listed := store.Assertions().List(); len(listed) != 1 {
		t.Fatalf("the store holds %d interpretations of one source", len(listed))
	}
}
