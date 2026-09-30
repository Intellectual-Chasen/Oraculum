package api_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const recordNumbersPath = "/api/v0/record-numbers"

// windowsEventStore は Windows イベントログの収集元を file 名の順に取り込み、file 名から
// 収集元の識別を探す表を返す。
func windowsEventStore(t *testing.T, files map[string]string, names ...string) (*pipeline.MemoryStore, map[string]core.SourceIdentity) {
	t.Helper()
	registry, err := pipeline.NewFormatRegistry(pipeline.WindowsEventFormats())
	if err != nil {
		t.Fatal(err)
	}
	plans := make([]pipeline.SourcePlan, 0, len(names))
	for _, name := range names {
		format := core.FormatKey("windows_event_xml")
		if strings.HasSuffix(name, ".csv") {
			format = "windows_event_viewer_csv"
		}
		plans = append(plans, pipeline.SourcePlan{OriginPath: name, FileName: name, FormatKey: format})
	}
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open:    func(path string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(files[path])), nil },
		Parsers: registry, Minter: pipeline.DigestMinter{},
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
	identities := map[string]core.SourceIdentity{}
	for _, entry := range entries {
		identities[entry.Identity.FileName] = entry.Identity
	}
	return pipeline.NewMemoryStore(result, &testAssertionClock{}), identities
}

// recordNumbersQuery は source の番号を問い、compared が非 nil なら突き合わせも問う要求の path である。
func recordNumbersQuery(source core.SourceIdentity, compared *core.SourceIdentity) string {
	query := url.Values{"sourceId": {source.SourceId}, "sourceContentSha256": {source.ContentSha256}}
	if compared != nil {
		query.Set("comparedSourceId", compared.SourceId)
		query.Set("comparedSourceContentSha256", compared.ContentSha256)
	}
	return recordNumbersPath + "?" + query.Encode()
}

type recordNumbersBody struct {
	RecordNumbers core.RecordNumbers     `json:"recordNumbers"`
	Comparison    *core.RecordComparison `json:"comparison"`
}

// 番号の抜けと、CSV との突き合わせを返す。CSV の時刻は分析者が記録した時刻の解釈で読む。
func TestRecordNumbersReportGapsAndCompareWithTheInterpretedCSV(t *testing.T) {
	event := func(recordID, at string) string {
		return `<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
			`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
			`<Channel>Application</Channel><Computer>host-a.example.test</Computer></System></Event>` + "\n"
	}
	files := map[string]string{
		"events.xml": "<Events>\n" + event("71", "2001-02-03T04:05:06Z") + event("74", "2001-02-03T04:05:07Z") + "</Events>\n",
		"events.csv": "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n" +
			"情報,2001/02/03 13:05:06,Example-Provider,7,,\"合成の説明\"\n",
	}
	store, identities := windowsEventStore(t, files, "events.xml", "events.csv")
	handler := handlerOf(store)
	xml, csv := identities["events.xml"], identities["events.csv"]

	var alone recordNumbersBody
	decodeInto(t, requestJSON(t, handler, http.MethodGet, recordNumbersQuery(xml, nil), nil, http.StatusOK), &alone)
	if alone.Comparison != nil || alone.RecordNumbers.Examination != core.RecordNumberExaminationGapsFound ||
		len(alone.RecordNumbers.Streams) != 1 || alone.RecordNumbers.Streams[0].MissingNumberCount != 2 {
		t.Fatalf("numbers = %+v", alone)
	}

	var before recordNumbersBody
	decodeInto(t, requestJSON(t, handler, http.MethodGet, recordNumbersQuery(csv, &xml), nil, http.StatusOK), &before)
	if before.RecordNumbers.Examination != core.RecordNumberExaminationNotExamined || before.Comparison == nil ||
		before.Comparison.NotComparedReason != core.RecordNotComparedTimeOffsetUndetermined {
		t.Fatalf("before the interpretation = %+v %+v", before.RecordNumbers, before.Comparison)
	}
	requestJSON(t, handler, http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, sourceInterpretationBody(csv.ContentSha256, "+09:00")), http.StatusCreated)
	var after recordNumbersBody
	decodeInto(t, requestJSON(t, handler, http.MethodGet, recordNumbersQuery(csv, &xml), nil, http.StatusOK), &after)
	if after.Comparison == nil || after.Comparison.State != core.RecordComparisonCompared ||
		after.Comparison.OneToOneKeyCount != 1 || len(after.Comparison.OnlyInSourceRecordRefs) != 0 ||
		len(after.Comparison.OnlyInComparedRecordRefs) != 1 ||
		after.Comparison.OnlyInComparedRecordRefs[0].SourceId != xml.SourceId {
		t.Fatalf("after the interpretation = %+v", after.Comparison)
	}
}

// 要求の項目の誤りを invalid_request で退け、収集元を探せない要求を source_not_found で返す。
func TestRecordNumbersRejectsMalformedRequests(t *testing.T) {
	files := map[string]string{"events.xml": "<Events>\n</Events>\n"}
	store, identities := windowsEventStore(t, files, "events.xml")
	handler := handlerOf(store)
	xml := identities["events.xml"]
	unknown := core.SourceIdentity{SourceId: "absent", ContentSha256: xml.ContentSha256}
	for _, path := range []string{
		recordNumbersPath,
		recordNumbersQuery(xml, &xml),
		recordNumbersQuery(xml, nil) + "&comparedSourceId=" + url.QueryEscape(xml.SourceId+"-other"),
		recordNumbersQuery(xml, nil) + "&unknown=1",
		recordNumbersQuery(xml, nil) + "&sourceId=" + url.QueryEscape(xml.SourceId),
	} {
		requestJSON(t, handler, http.MethodGet, path, nil, http.StatusBadRequest)
	}
	requestJSON(t, handler, http.MethodGet, recordNumbersQuery(unknown, nil), nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodGet, recordNumbersQuery(xml, &unknown), nil, http.StatusNotFound)
}

// 相手の内容の識別が食い違う要求と、公開を止めた収集元への要求を、records の操作と同じ code で退ける。
func TestRecordNumbersRejectsAMismatchedOrWithheldSource(t *testing.T) {
	files := map[string]string{"a.xml": "<Events>\n</Events>\n", "b.xml": "<Events>\n\n</Events>\n"}
	store, identities := windowsEventStore(t, files, "a.xml", "b.xml")
	mismatched := identities["b.xml"]
	mismatched.ContentSha256 = identities["a.xml"].ContentSha256
	var problem core.ApiError
	decodeInto(t, requestJSON(t, handlerOf(store), http.MethodGet,
		recordNumbersQuery(identities["a.xml"], &mismatched), nil, http.StatusConflict), &problem)
	if problem.Code != core.ApiErrorCodeSourceHashMismatch || problem.SourceId != mismatched.SourceId {
		t.Errorf("mismatched compared source = %+v", problem)
	}
	handler, withheld := withheldImportResult(t)
	decodeInto(t, requestJSON(t, handler, http.MethodGet, recordNumbersQuery(withheld, nil), nil, http.StatusConflict), &problem)
	if problem.Code != core.ApiErrorCodeImportWithheld {
		t.Errorf("withheld source = %+v", problem)
	}
}
