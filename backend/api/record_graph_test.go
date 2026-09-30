package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const recordGraphPath = "/api/v0/record-graph"

// recordGraphResponse は応答の項目名を test 側で固定する。
type recordGraphResponse struct {
	RecordRef core.RecordLocator `json:"recordRef"`
	NodeIds   []string           `json:"nodeIds"`
	EdgeIds   []string           `json:"edgeIds"`
}

func requestRecordGraph(
	t *testing.T, handler http.Handler, query string,
) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(recordGraphPath+"?"+query), nil))
	return response
}

// recordGraphQuery はレコードの位置を要求の項目へ直す。
func recordGraphQuery(locator core.RecordLocator) string {
	query := url.Values{
		"sourceId":            {locator.SourceId},
		"sourceContentSha256": {locator.SourceContentSha256},
	}
	if locator.SequenceNumber != nil {
		query.Set("sequenceNumber", strconv.FormatInt(*locator.SequenceNumber, 10))
	}
	if locator.LineNumber != nil {
		query.Set("lineNumber", strconv.FormatInt(*locator.LineNumber, 10))
	}
	return query.Encode()
}

// 端末を名乗るレコードは、その端末のノードと、端末を指すエッジを返す。返すエッジの端点は
// すべてノードに入る。
func TestRecordGraphReturnsTheNodesAndEdgesTheRecordIsEvidenceOf(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	var entry *timelineEntry
	for index := range whole.Entries {
		if whole.Entries[index].Terminal != nil {
			entry = &whole.Entries[index]
			break
		}
	}
	if entry == nil {
		t.Fatal("no entry names a terminal, and the fixture records terminals")
	}
	response := requestRecordGraph(t, handler, recordGraphQuery(entry.RecordRef))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded recordGraphResponse
	decodeJSON(t, response.Body, &decoded)
	if decoded.RecordRef.RecordRawTextRef != entry.RecordRef.RecordRawTextRef {
		t.Errorf("recordRef=%q want %q",
			decoded.RecordRef.RecordRawTextRef, entry.RecordRef.RecordRawTextRef)
	}
	if !slices.Contains(decoded.NodeIds, entry.Terminal.Id) {
		t.Errorf("nodeIds=%v lack the terminal %q the record named", decoded.NodeIds, entry.Terminal.Id)
	}
	if len(decoded.EdgeIds) == 0 {
		t.Error("the record named a terminal, and the response carries no edge")
	}
	graph := decodeGraph(t, handler,
		"depth=1&nodeId="+url.QueryEscape(entry.Terminal.Id))
	compared := 0
	for _, edge := range graph.Edges {
		if !slices.Contains(decoded.EdgeIds, edge.Id) {
			continue
		}
		compared++
		if !slices.Contains(decoded.NodeIds, edge.SourceNodeId) ||
			!slices.Contains(decoded.NodeIds, edge.TargetNodeId) {
			t.Errorf("the edge %q has an endpoint outside nodeIds", edge.Id)
		}
	}
	if compared == 0 {
		t.Error("no edge of the response is in the graph around the terminal")
	}
}

// 未知の項目と位置の無い要求を invalid_request で、知らない収集元を source_not_found で退ける。
func TestRecordGraphRejectsInvalidRequests(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	if len(whole.Entries) == 0 {
		t.Fatal("the timeline carries no entry")
	}
	locator := whole.Entries[0].RecordRef
	for _, query := range []string{
		recordGraphQuery(locator) + "&limit=1",
		url.Values{"sourceId": {locator.SourceId}, "sourceContentSha256": {locator.SourceContentSha256}}.Encode(),
	} {
		response := requestRecordGraph(t, handler, query)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d want 400", query, response.Code)
		}
	}
	absent := locator
	absent.SourceId = "source-absent"
	response := requestRecordGraph(t, handler, recordGraphQuery(absent))
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeSourceNotFound {
		t.Errorf("code=%q want source_not_found", apiError.Code)
	}
}
