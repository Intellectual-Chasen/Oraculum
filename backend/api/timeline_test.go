package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// timelinePath は時系列の操作の path である。
const timelinePath = "/api/v0/timeline"

// periodUnjudged は期間の判定から外れたレコードの件数の組の項目名を test 側で固定する。
type periodUnjudged struct {
	LocalRecordCount   int64 `json:"localRecordCount"`
	UndatedRecordCount int64 `json:"undatedRecordCount"`
}

// timelineResponse は時系列の操作の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type timelineResponse struct {
	Entries            []timelineEntry `json:"entries"`
	EntryCount         int64           `json:"entryCount"`
	UndatedRecordCount int64           `json:"undatedRecordCount"`
	// PeriodUnjudged は期間の判定から外れたレコードの件数である。
	PeriodUnjudged   *periodUnjudged       `json:"periodUnjudged,omitempty"`
	SourceCoverages  []core.SourceCoverage `json:"sourceCoverages"`
	EventCategory    string                `json:"eventCategory,omitempty"`
	EventAction      string                `json:"eventAction,omitempty"`
	EventActionFrom  *uint64               `json:"eventActionFrom,omitempty"`
	EventActionTo    *uint64               `json:"eventActionTo,omitempty"`
	Case             string                `json:"case,omitempty"`
	Terminal         string                `json:"terminal,omitempty"`
	TimeFrom         *core.RequestedTime   `json:"timeFrom,omitempty"`
	TimeTo           *core.RequestedTime   `json:"timeTo,omitempty"`
	FilterUnit       string                `json:"filterUnit,omitempty"`
	SearchExpression string                `json:"searchExpression,omitempty"`
	Sources          []string              `json:"source,omitempty"`
	NodeIds          []string              `json:"nodeIds,omitempty"`
	Depth            *int                  `json:"depth,omitempty"`
	AccountNodeId    string                `json:"accountNodeId,omitempty"`
	FindMatches      *[]int                `json:"findMatches,omitempty"`
	EmptyReason      core.EmptyReason      `json:"emptyReason,omitempty"`
}

type timelineEntry struct {
	RecordRef       core.RecordLocator     `json:"recordRef"`
	EventTime       *core.Timestamp        `json:"eventTime,omitempty"`
	ObservationKind core.ObservationKind   `json:"observationKind"`
	EventKind       *core.EventKindPair    `json:"eventKind,omitempty"`
	Terminal        *timelineNode          `json:"terminal,omitempty"`
	Account         *timelineNode          `json:"account,omitempty"`
	AccountRoles    []core.EdgeKind        `json:"accountRoles,omitempty"`
	OtherAccounts   []core.TimelineAccount `json:"otherAccounts,omitempty"`
	SourceAddress   string                 `json:"sourceAddress,omitempty"`
}

func TestTimelineAccountHistoryUsesTheAccountIdAndReportsRoles(t *testing.T) {
	for _, handler := range []http.Handler{terminalsHandler(t), graphHandler(t)} {
		found := false
		whole := decodeTimeline(t, handler, "")
		for _, entry := range whole.Entries {
			if entry.Account == nil {
				continue
			}
			found = true
			id := entry.Account.Id
			history := decodeTimeline(t, handler, "accountNodeId="+url.QueryEscape(id))
			if history.EntryCount == 0 {
				t.Fatalf("account %q has no role-linked records", id)
			}
			if history.AccountNodeId != id {
				t.Errorf("accountNodeId=%q, want %q", history.AccountNodeId, id)
			}
			for _, record := range history.Entries {
				if len(record.AccountRoles) == 0 {
					t.Errorf("record %+v has no account role", record.RecordRef)
				}
			}
			requestTimelineError(t, handler, "accountNodeId=", http.StatusBadRequest)
			requestTimelineError(t, handler, "accountNodeId="+url.QueryEscape(id)+"&nodeId="+url.QueryEscape(id)+"&depth=1", http.StatusBadRequest)
			unknown := requestTimelineError(t, handler,
				"accountNodeId="+url.QueryEscape("n:unknown"), http.StatusNotFound)
			if unknown.Message != "no node matches the requested accountNodeId" {
				t.Errorf("unknown account error message = %q", unknown.Message)
			}
			if entry.Terminal != nil {
				requestTimelineError(t, handler, "accountNodeId="+url.QueryEscape(entry.Terminal.Id), http.StatusBadRequest)
			}
			break
		}
		if !found {
			t.Fatal("fixture has no account")
		}
	}
}

type timelineNode struct {
	Id             string                   `json:"id"`
	Kind           string                   `json:"kind"`
	KeyForm        string                   `json:"keyForm"`
	Identity       []core.NodeIdentityValue `json:"identity"`
	Label          core.RawAndNormalized    `json:"label"`
	Observation    string                   `json:"observation"`
	CreationRecord string                   `json:"creationRecord"`
	// AccountName と AccountNameWithheld は、アカウントのノードをまとめる鍵と、持たない理由である。
	AccountName         *core.AccountNameKey           `json:"accountName,omitempty"`
	AccountNameWithheld core.AccountNameWithheldReason `json:"accountNameWithheld,omitempty"`
}

func requestTimeline(
	t *testing.T, handler http.Handler, query string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := timelinePath
	if query != "" {
		path += "?" + query
	}
	path = withAllMatchConditions(path)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func decodeTimeline(t *testing.T, handler http.Handler, query string) timelineResponse {
	t.Helper()
	response := requestTimeline(t, handler, query)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded timelineResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

func requestTimelineError(
	t *testing.T, handler http.Handler, query string, wantStatus int,
) core.ApiError {
	t.Helper()
	response := requestTimeline(t, handler, query)
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s",
			response.Code, wantStatus, response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	return apiError
}

// 収集元をまたいで 1 本の列になり、時刻の昇順に並ぶ。
func TestTimelineOrdersEveryRecordOfEverySourceByTheEventTime(t *testing.T) {
	page := decodeTimeline(t, graphHandler(t), "")
	if len(page.Entries) == 0 {
		t.Fatal("the timeline carries no entry")
	}
	if page.EntryCount != int64(len(page.Entries)) {
		t.Fatalf("entryCount=%d, len(entries)=%d", page.EntryCount, len(page.Entries))
	}
	var previous *core.Timestamp
	for index, entry := range page.Entries {
		if entry.EventTime == nil {
			t.Fatalf("entries[%d] carries no event time and is in the ordered list", index)
		}
		if previous == nil {
			previous = entry.EventTime
			continue
		}
		earlier, earlierOk := previous.Instant()
		later, laterOk := entry.EventTime.Instant()
		if !earlierOk || !laterOk {
			t.Fatalf("entries[%d] cannot be compared as an absolute time", index)
		}
		if later.Before(earlier) {
			t.Fatalf("entries[%d] at %s comes after %s", index, later, earlier)
		}
		previous = entry.EventTime
	}
	sources := make(map[string]struct{})
	for _, entry := range page.Entries {
		sources[entry.RecordRef.SourceId] = struct{}{}
	}
	if len(sources) < 2 {
		t.Fatalf("the timeline carries %d source, want the records of every source",
			len(sources))
	}
}

// 時刻を読めなかったレコードを列へ混ぜず、別の件数で示す。
func TestTimelineKeepsTheUndatedRecordsOutOfTheOrderedList(t *testing.T) {
	page := decodeTimeline(t, graphHandler(t), "")
	if page.UndatedRecordCount < 0 {
		t.Fatalf("undatedRecordCount=%d", page.UndatedRecordCount)
	}
	for index, entry := range page.Entries {
		if entry.EventTime == nil {
			t.Fatalf("entries[%d] carries no event time and is in the ordered list", index)
		}
		if _, ok := entry.EventTime.Instant(); !ok {
			t.Fatalf("entries[%d] carries a time that cannot be compared", index)
		}
	}
}

// 収録範囲を持つ収集元のそれぞれに、要求の期間との重なり方が付く。
func TestTimelineCarriesTheCoverageOfEverySource(t *testing.T) {
	page := decodeTimeline(t, graphHandler(t), "")
	if len(page.SourceCoverages) == 0 {
		t.Fatal("the timeline carries no source coverage")
	}
	counted := make(map[string]int64, len(page.SourceCoverages))
	for _, coverage := range page.SourceCoverages {
		if err := coverage.Validate(); err != nil {
			t.Fatalf("the coverage of %q does not validate: %v", coverage.SourceId, err)
		}
		if coverage.State != core.CoverageStateRangeNotRequested {
			t.Fatalf("a request without a period gave %q the state %q, want %q",
				coverage.SourceId, coverage.State, core.CoverageStateRangeNotRequested)
		}
		counted[coverage.SourceId] = coverage.MatchedRecordCount
	}
	// 収集元ごとの件数の和は、列の行数と時刻不明の件数の和に等しい。
	var sum int64
	for _, count := range counted {
		sum += count
	}
	if want := page.EntryCount + page.UndatedRecordCount; sum != want {
		t.Fatalf("the coverages counted %d records, want %d", sum, want)
	}
}

// 記録が無い区間と、事象が無い区間を別の表現で示す。
func TestTimelineSeparatesTheAbsentRecordingFromTheAbsentEvent(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	if len(whole.Entries) == 0 {
		t.Fatal("the timeline carries no entry")
	}
	// どの収集元も記録していない、収録範囲より十分に後の期間。
	outside := decodeTimeline(t, handler,
		"timeFrom=2099-01-01T00:00:00%2B09:00&timeFromPrecision=second"+
			"&timeTo=2099-01-02T00:00:00%2B09:00&timeToPrecision=second"+
			"&filterUnit=second")
	if outside.EntryCount != 0 {
		t.Fatalf("a period after every recording returned %d entries", outside.EntryCount)
	}
	for _, coverage := range outside.SourceCoverages {
		if coverage.ObservedRangeFirst == nil {
			continue
		}
		if coverage.State != core.CoverageStateOutsideRecording {
			t.Fatalf("%q has a recording range and the state %q, want %q",
				coverage.SourceId, coverage.State, core.CoverageStateOutsideRecording)
		}
	}
	// 収録範囲の中にありながら、その種別の事象を 1 件も持たない絞り込み。
	absentEvent := decodeTimeline(t, handler, "eventCategory=no_such_category")
	if absentEvent.EntryCount != 0 {
		t.Fatalf("an unknown event category returned %d entries", absentEvent.EntryCount)
	}
	states := make([]core.CoverageState, 0, len(absentEvent.SourceCoverages))
	for _, coverage := range absentEvent.SourceCoverages {
		if coverage.MatchedRecordCount != 0 {
			t.Fatalf("%q matched %d records of an unknown category",
				coverage.SourceId, coverage.MatchedRecordCount)
		}
		states = append(states, coverage.State)
	}
	if slices.Contains(states, core.CoverageStateOutsideRecording) {
		t.Fatal("a filter that matched no event marked a source as outside its recording")
	}
}

// 事象の分類で絞ると、その分類のレコードだけが列に並ぶ。
// 行の eventKind をそのまま条件に渡すと、その行を含み、同じ組の行だけに絞る。
func TestTimelineEntryEventKindNarrowsToTheEntriesOfTheSamePair(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	var picked *timelineEntry
	for i := range whole.Entries {
		if whole.Entries[i].EventKind != nil {
			picked = &whole.Entries[i]
			break
		}
	}
	if picked == nil {
		t.Fatal("the fixture carries no entry with an event kind")
	}
	kind := *picked.EventKind
	narrowed := decodeTimeline(t, handler, "eventCategory="+url.QueryEscape(kind.Category)+
		"&eventAction="+url.QueryEscape(kind.Action))
	if narrowed.EntryCount == 0 || narrowed.EntryCount >= whole.EntryCount {
		t.Fatalf("narrowing by %+v returned %d entries of %d", kind, narrowed.EntryCount, whole.EntryCount)
	}
	found := false
	for _, entry := range narrowed.Entries {
		if entry.EventKind == nil || *entry.EventKind != kind {
			t.Fatalf("an entry narrowed by %+v carries the event kind %+v", kind, entry.EventKind)
		}
		found = found || entry.RecordRef.RecordRawTextRef == picked.RecordRef.RecordRawTextRef
	}
	if !found {
		t.Fatalf("narrowing by %+v dropped the entry the pair came from", kind)
	}
}

func TestTimelineNarrowsTheListToTheRequestedEventCategory(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	var category string
	for _, entry := range whole.Entries {
		for _, raw := range entry.ObservationKind.Raw {
			if raw.Text == nil {
				continue
			}
			value, readable := raw.Text.ComparableValue()
			if readable && value != "" {
				category = value
				break
			}
		}
		if category != "" {
			break
		}
	}
	if category == "" {
		t.Fatal("the fixture carries no readable observation kind to narrow by")
	}
	narrowed := decodeTimeline(t, handler, "eventCategory="+category)
	if narrowed.EventCategory != category {
		t.Fatalf("the response returned the category %q, want %q",
			narrowed.EventCategory, category)
	}
	if narrowed.EntryCount == 0 {
		t.Fatalf("narrowing by the category %q of an entry of the whole list "+
			"returned no entry", category)
	}
	if narrowed.EntryCount >= whole.EntryCount {
		t.Fatalf("narrowing returned %d entries of the %d of the whole list, "+
			"and the fixture carries more than one category",
			narrowed.EntryCount, whole.EntryCount)
	}
	// 絞り込みの外にあるレコードが列に残らない。
	kept := make(map[string]struct{}, len(narrowed.Entries))
	for _, entry := range narrowed.Entries {
		kept[entry.RecordRef.RecordRawTextRef] = struct{}{}
	}
	if len(kept) != len(narrowed.Entries) {
		t.Fatalf("the narrowed list names %d distinct records in %d entries",
			len(kept), len(narrowed.Entries))
	}
}

// 上限と続きを取る位置の項目を受けない。
func TestTimelineRejectsALimitAndACursor(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"limit=10", "cursor=abc", "offset=1", "nodeLimit=100", "expansion=unfolded",
	} {
		t.Run(query, func(t *testing.T) {
			apiError := requestTimelineError(t, handler, query, http.StatusBadRequest)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want %q", apiError.Code, core.ApiErrorCodeInvalidRequest)
			}
		})
	}
}

// 期間の端を与えた要求は、精度と比較の単位を必ず与える。
func TestTimelineRequiresThePrecisionAndTheUnitOfTheRequestedPeriod(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name, query string
		wantMissing []string
	}{
		{
			name:        "精度と単位の両方が無い",
			query:       "timeFrom=2000-02-01T13:00:00%2B09:00",
			wantMissing: []string{"timeFromPrecision", "filterUnit"},
		},
		{
			name: "単位だけが無い",
			query: "timeFrom=2000-02-01T13:00:00%2B09:00" +
				"&timeFromPrecision=second",
			wantMissing: []string{"filterUnit"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			apiError := requestTimelineError(t, handler, testCase.query, http.StatusBadRequest)
			if !slices.Equal(apiError.MissingParameters, testCase.wantMissing) {
				t.Fatalf("missingParameters=%v want %v",
					apiError.MissingParameters, testCase.wantMissing)
			}
		})
	}
}

// 応答は要求が用いた条件をそのまま返す。0 件の応答でも返す。
func TestTimelineReturnsTheConditionsItUsed(t *testing.T) {
	page := decodeTimeline(t, graphHandler(t),
		"timeFrom=2099-01-01T00:00:00%2B09:00&timeFromPrecision=second"+
			"&timeTo=2099-01-02T00:00:00%2B09:00&timeToPrecision=second"+
			"&filterUnit=second&eventCategory=file")
	if page.EntryCount != 0 {
		t.Fatalf("a period after every recording returned %d entries", page.EntryCount)
	}
	if page.EmptyReason != core.EmptyReasonNoRecordInFilter {
		t.Fatalf("emptyReason=%q want %q",
			page.EmptyReason, core.EmptyReasonNoRecordInFilter)
	}
	if page.TimeFrom == nil || page.TimeTo == nil {
		t.Fatal("the empty response dropped the period it used")
	}
	if page.FilterUnit != "second" {
		t.Fatalf("filterUnit=%q want second", page.FilterUnit)
	}
	if page.EventCategory != "file" {
		t.Fatalf("eventCategory=%q want file", page.EventCategory)
	}
}

// 端末を与えた要求は、その端末を名乗るレコードの行だけを返し、端末をそのまま返す。
// 知らない端末と空の端末を退ける。
func TestTimelineNarrowsTheListToTheRequestedTerminal(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	var terminal string
	for _, entry := range whole.Entries {
		if entry.Terminal != nil {
			terminal = entry.Terminal.Id
			break
		}
	}
	if terminal == "" {
		t.Fatal("no entry names a terminal, and the fixture records terminals")
	}
	wanted := 0
	for _, entry := range whole.Entries {
		if entry.Terminal != nil && entry.Terminal.Id == terminal {
			wanted++
		}
	}
	page := decodeTimeline(t, handler, "terminal="+url.QueryEscape(terminal))
	if page.Terminal != terminal {
		t.Errorf("the response returned terminal=%q, want %q", page.Terminal, terminal)
	}
	if page.EntryCount != int64(wanted) || wanted == len(whole.Entries) {
		t.Errorf("the terminal kept %d of %d entries, want %d and fewer than the whole",
			page.EntryCount, len(whole.Entries), wanted)
	}
	for _, entry := range page.Entries {
		if entry.Terminal == nil || entry.Terminal.Id != terminal {
			t.Fatalf("the entry %+v names another terminal", entry.RecordRef)
		}
	}
	for _, query := range []string{"terminal=", "terminal=n:terminal:absent"} {
		apiError := requestTimelineError(t, handler, query, http.StatusBadRequest)
		if apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s: code=%q want invalid_request", query, apiError.Code)
		}
	}
}

// 起点のノードと段数を与えた要求は、全件より少ない行を返し、段数を増やすと行が減らない。
// 起点と段数をそのまま返す。段数を欠く要求、起点の無い段数、範囲の外の段数、空の起点を
// invalid_request で、知らないノードを record_not_found で退ける。
func TestTimelineNarrowsTheListToTheRecordsNearTheRequestedNode(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	var account string
	for _, entry := range whole.Entries {
		if entry.Account != nil {
			account = entry.Account.Id
			break
		}
	}
	if account == "" {
		t.Fatal("no entry names an account, and the fixture records accounts")
	}
	origin := "nodeId=" + url.QueryEscape(account)
	near := decodeTimeline(t, handler, origin+"&depth=0")
	wider := decodeTimeline(t, handler, origin+"&depth=2")
	if near.EntryCount == 0 || near.EntryCount >= whole.EntryCount {
		t.Errorf("depth 0 kept %d of %d entries, want some and fewer than the whole",
			near.EntryCount, whole.EntryCount)
	}
	if wider.EntryCount < near.EntryCount {
		t.Errorf("depth 2 kept %d entries, fewer than the %d of depth 0",
			wider.EntryCount, near.EntryCount)
	}
	if !slices.Equal(near.NodeIds, []string{account}) || near.Depth == nil || *near.Depth != 0 {
		t.Errorf("the response returned nodeIds=%v depth=%v", near.NodeIds, near.Depth)
	}
	if whole.NodeIds != nil || whole.Depth != nil {
		t.Errorf("a request without an origin returned nodeIds=%v depth=%v",
			whole.NodeIds, whole.Depth)
	}
	for _, query := range []string{
		"depth=1", origin + "&depth=9", origin + "&depth=-1", "nodeId=&depth=1",
	} {
		apiError := requestTimelineError(t, handler, query, http.StatusBadRequest)
		if apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s: code=%q want invalid_request", query, apiError.Code)
		}
	}
	missing := requestTimelineError(t, handler, origin, http.StatusBadRequest)
	if !slices.Equal(missing.MissingParameters, []string{"depth"}) {
		t.Errorf("missingParameters=%v want [depth]", missing.MissingParameters)
	}
	unknown := requestTimelineError(t, handler, "nodeId=n:absent&depth=1", http.StatusNotFound)
	if unknown.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", unknown.Code)
	}
}

// 行が指す端末とアカウントは、グラフのノードとして返る。
func TestTimelineCarriesTheTerminalAndTheAccountAsGraphNodes(t *testing.T) {
	page := decodeTimeline(t, graphHandler(t), "")
	graph := decodeGraph(t, graphHandler(t), wholeGraphQuery)
	known := make(map[string]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		known[node.Id] = node.Kind
	}
	var withTerminal, withAccount int
	for index, entry := range page.Entries {
		for _, item := range []struct {
			name     string
			node     *timelineNode
			wantKind string
		}{
			{"terminal", entry.Terminal, "terminal"},
			{"account", entry.Account, "account"},
		} {
			if item.node == nil {
				continue
			}
			kind, present := known[item.node.Id]
			if !present {
				t.Fatalf("entries[%d].%s points at %q, which is not a node of the graph",
					index, item.name, item.node.Id)
			}
			if kind != item.wantKind {
				t.Fatalf("entries[%d].%s points at a node of kind %q, want %q",
					index, item.name, kind, item.wantKind)
			}
			if item.name == "terminal" {
				withTerminal++
			} else {
				withAccount++
			}
		}
	}
	if withTerminal == 0 {
		t.Fatal("no entry names a terminal, and the fixture records terminals")
	}
	if withAccount == 0 {
		t.Fatal("no entry names an account, and the fixture records accounts")
	}
}
