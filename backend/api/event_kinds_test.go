package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const eventKindsPath = "/api/v0/event-kinds"

// eventKindsResponse は事象の種別の一覧の応答の項目名を test 側で固定する。
type eventKindsResponse struct {
	Kinds                    []eventKindItem `json:"kinds"`
	UncategorizedRecordCount int64           `json:"uncategorizedRecordCount"`
	Case                     string          `json:"case,omitempty"`
	Terminal                 string          `json:"terminal,omitempty"`
	SourceId                 string          `json:"sourceId,omitempty"`
	SemanticFields           []string        `json:"semanticFields"`
	FieldNames               []string        `json:"fieldNames"`
}

// 欄の名前の一覧は、取り込んだレコードの語彙の項目と原資料の key を昇順で重複なく返す。
func TestEventKindsListTheObservedFieldNames(t *testing.T) {
	handler := windowsEventHandler(t)
	whole := decodeEventKinds(t, handler, "")
	for _, want := range []struct {
		list []string
		name string
	}{{whole.SemanticFields, "windows_event.id"}, {whole.FieldNames, "psPath"}} {
		if !slices.Contains(want.list, want.name) {
			t.Errorf("list %v lacks %q", want.list, want.name)
		}
	}
	if !slices.IsSorted(whole.SemanticFields) || !slices.IsSorted(whole.FieldNames) ||
		len(slices.Compact(slices.Clone(whole.FieldNames))) != len(whole.FieldNames) {
		t.Errorf("lists are not sorted and unique: %v %v", whole.SemanticFields, whole.FieldNames)
	}
}

type eventKindItem struct {
	Category     string `json:"category"`
	Action       string `json:"action,omitempty"`
	RecordCount  int64  `json:"recordCount"`
	WindowsEvent *bool  `json:"windowsEvent,omitempty"`
}

// windowsEventHandler は、Windows イベントログの XML と markii 形式の行を取り込んだ handler である。
// XML はイベント ID を持つ 2 件と、イベント ID の要素を持たない 1 件と、空の文字列の 1 件を持つ。
func windowsEventHandler(t *testing.T) http.Handler {
	t.Helper()
	event := func(recordID, eventID string) string {
		return `<Event><System><Provider Name="Example-Provider"/>` + eventID +
			`<TimeCreated SystemTime="2001-02-03T04:05:0` + recordID + `Z"/>` +
			`<EventRecordID>` + recordID + `</EventRecordID>` +
			`<Computer>host01.example.test</Computer></System></Event>`
	}
	contents := map[string]string{
		"events.xml": strings.Join([]string{"<Events>",
			event("1", "<EventID>42</EventID>"), event("2", "<EventID>42</EventID>"),
			event("3", ""), event("4", "<EventID></EventID>"), "</Events>"}, "\n"),
		"markii.log": "02/01/2000 03:04:01.000 +0900 sn=1 evt=ps subEvt=start psGUID=p1 " +
			"tmid=t com=TESTHOST csid=s psPath=app\n",
	}
	registry, err := pipeline.NewFormatRegistry(pipeline.MarkIIFormats(), pipeline.WindowsEventFormats())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(contents[path])), nil
		},
		Parsers: registry, Minter: pipeline.DigestMinter{}, Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize, Revision: "api-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{
		{FormatKey: "windows_event_xml", FileName: "events.xml", OriginPath: "events.xml"},
		{FormatKey: markIIFormatKey, FileName: "markii.log", OriginPath: "markii.log"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return testHandler(result)
}

// Windows イベントログの組は windowsEvent を真で持ち、ほかの組は項目を出さない。イベント ID を
// 持たないイベントは組を作らず、組ごとの件数はその組で絞った時系列の件数と一致する。
func TestEventKindsMarkTheWindowsEventPairs(t *testing.T) {
	handler := windowsEventHandler(t)
	kinds := decodeEventKinds(t, handler, "")

	yes := true
	want := []eventKindItem{
		{Category: "Example-Provider", Action: "42", RecordCount: 2, WindowsEvent: &yes},
		{Category: "ps", Action: "start", RecordCount: 1},
	}
	if !reflect.DeepEqual(kinds.Kinds, want) || kinds.UncategorizedRecordCount != 2 {
		t.Fatalf("kinds = %+v (%d), want %+v and the two events without an event id",
			kinds.Kinds, kinds.UncategorizedRecordCount, want)
	}
	assertKindsMatchTheTimeline(t, handler, "", kinds)
}

func requestEventKinds(t *testing.T, handler http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := eventKindsPath
	if query != "" {
		path += "?" + query
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, withAllMatchConditions(path), nil))
	return response
}

func decodeEventKinds(t *testing.T, handler http.Handler, query string) eventKindsResponse {
	t.Helper()
	response := requestEventKinds(t, handler, query)
	if response.Code != http.StatusOK {
		t.Fatalf("query=%q status=%d body=%s", query, response.Code, response.Body.String())
	}
	var decoded eventKindsResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// timelineRecordCount は、時系列が同じ条件で返すレコードの件数 (時刻を持つ行と持たない件数の和) である。
func timelineRecordCount(t *testing.T, handler http.Handler, query string) int64 {
	t.Helper()
	timeline := decodeTimeline(t, handler, query)
	return timeline.EntryCount + timeline.UndatedRecordCount
}

// assertKindsMatchTheTimeline は、組ごとの件数が、その組で絞った時系列の件数と一致し、
// 組の件数と分類を持たない件数の和が、絞らない時系列の件数と一致することを確かめる。
func assertKindsMatchTheTimeline(
	t *testing.T, handler http.Handler, scope string, kinds eventKindsResponse,
) {
	t.Helper()
	join := func(query string) string {
		if scope == "" {
			return query
		}
		if query == "" {
			return scope
		}
		return scope + "&" + query
	}
	var total int64
	for index, kind := range kinds.Kinds {
		if kind.RecordCount <= 0 {
			t.Fatalf("kind %+v carries no record", kind)
		}
		if index > 0 && kinds.Kinds[index-1].RecordCount < kind.RecordCount {
			t.Fatalf("kinds are not in descending order of the count: %+v", kinds.Kinds)
		}
		narrowing := "eventCategory=" + url.QueryEscape(kind.Category)
		if kind.Action != "" {
			narrowing += "&eventAction=" + url.QueryEscape(kind.Action)
		}
		if got := timelineRecordCount(t, handler, join(narrowing)); kind.Action != "" && got != kind.RecordCount {
			t.Fatalf("kind %+v, the timeline narrowed to it has %d records", kind, got)
		}
		total += kind.RecordCount
	}
	if want := timelineRecordCount(t, handler, scope); total+kinds.UncategorizedRecordCount != want {
		t.Fatalf("kinds sum to %d and %d records carry no category, the timeline has %d",
			total, kinds.UncategorizedRecordCount, want)
	}
}

func TestEventKindsCountTheRecordsTheTimelineReturnsForEachKind(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeEventKinds(t, handler, "")
	if len(whole.Kinds) < 2 {
		t.Fatalf("the fixture carries fewer than two event kinds: %+v", whole.Kinds)
	}
	assertKindsMatchTheTimeline(t, handler, "", whole)
}

func TestEventKindsNarrowToTheRequestedTerminal(t *testing.T) {
	handler := graphHandler(t)
	terminals := decodeGraph(t, handler,
		"depth=0&granularity=object&nodeKind=terminal")
	if len(terminals.Nodes) == 0 {
		t.Fatal("the fixture carries no terminal")
	}
	terminal := terminals.Nodes[0].Id
	scope := "terminal=" + url.QueryEscape(terminal)
	narrowed := decodeEventKinds(t, handler, scope)
	if narrowed.Terminal != terminal {
		t.Fatalf("terminal=%q want %q", narrowed.Terminal, terminal)
	}
	assertKindsMatchTheTimeline(t, handler, scope, narrowed)
}

// 収集元で絞った要求は、その収集元のレコードの組だけを数え、sourceId をそのまま返す。
// 2 つの収集元で絞った件数の和は、絞らない件数と一致する。
func TestEventKindsNarrowToTheRequestedSource(t *testing.T) {
	handler := windowsEventHandler(t)
	page := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	if len(page.Sources) != 2 {
		t.Fatalf("sources = %d, want the XML and the markii log", len(page.Sources))
	}
	whole := decodeEventKinds(t, handler, "")
	wholeTotal := whole.UncategorizedRecordCount
	for _, kind := range whole.Kinds {
		wholeTotal += kind.RecordCount
	}
	var total int64
	for _, item := range page.Sources {
		query := "sourceId=" + url.QueryEscape(item.Source.SourceId) +
			"&sourceContentSha256=" + url.QueryEscape(item.Source.ContentSha256)
		narrowed := decodeEventKinds(t, handler, query)
		if narrowed.SourceId != item.Source.SourceId {
			t.Errorf("sourceId=%q, want %q", narrowed.SourceId, item.Source.SourceId)
		}
		for _, kind := range narrowed.Kinds {
			if (kind.Category == "ps") != (item.Source.FileName == "markii.log") {
				t.Errorf("%s counts the kind %+v of the other source", item.Source.FileName, kind)
			}
			total += kind.RecordCount
		}
		total += narrowed.UncategorizedRecordCount
	}
	if total != wholeTotal {
		t.Errorf("the sources sum to %d records, the whole import has %d", total, wholeTotal)
	}
	first := page.Sources[0].Source
	for query, status := range map[string]int{
		"sourceId=" + url.QueryEscape(first.SourceId):                                 http.StatusBadRequest,
		"sourceContentSha256=" + url.QueryEscape(first.ContentSha256):                 http.StatusBadRequest,
		"sourceId=absent&sourceContentSha256=" + url.QueryEscape(first.ContentSha256): http.StatusNotFound,
		"sourceId=" + url.QueryEscape(first.SourceId) + "&sourceContentSha256=00":     http.StatusConflict,
	} {
		if response := requestEventKinds(t, handler, query); response.Code != status {
			t.Errorf("query=%q status=%d want %d body=%s", query, response.Code, status, response.Body.String())
		}
	}
}

func TestEventKindsRejectTheEventKindConditionsAndUnknownItems(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"eventCategory=ps",
		"eventAction=start",
		"unexpected=value",
		"terminal=",
		"terminal=n:terminal:absent",
		"case=missing",
	} {
		if response := requestEventKinds(t, handler, query); response.Code != http.StatusBadRequest {
			t.Fatalf("query=%q status=%d body=%s", query, response.Code, response.Body.String())
		}
	}
}
