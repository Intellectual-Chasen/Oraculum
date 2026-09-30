package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

type nodeSummariesBody struct {
	Nodes []struct {
		Node        core.GraphNode  `json:"node"`
		RecordCount int             `json:"recordCount"`
		FirstTime   *core.Timestamp `json:"firstTime"`
		LastTime    *core.Timestamp `json:"lastTime"`
		// LocalTimeRecordCount と UndatedRecordCount は、時刻の範囲に入らないレコードの件数である。
		LocalTimeRecordCount int `json:"localTimeRecordCount"`
		UndatedRecordCount   int `json:"undatedRecordCount"`
	} `json:"nodes"`
	NodeCount int           `json:"nodeCount"`
	NodeKind  core.NodeKind `json:"nodeKind"`
}

func requestNodeSummaries(handler http.Handler, query string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions("/api/v0/node-summaries?"+query), nil))
	return response
}

// 端末ごとの件数は、その端末から 1 ホップの時系列の行数と一致し、件数の降順に並ぶ。
// 時刻の範囲はその時系列の先頭と末尾の時刻である。
func TestNodeSummariesCountTheRecordsOneStepFromEachNode(t *testing.T) {
	handler := graphHandler(t)
	response := requestNodeSummaries(handler, "nodeKind=terminal")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body nodeSummariesBody
	decodeJSON(t, response.Body, &body)
	if body.NodeCount == 0 || body.NodeCount != len(body.Nodes) || body.NodeKind != core.NodeKindTerminal {
		t.Fatalf("nodeCount=%d nodes=%d kind=%s", body.NodeCount, len(body.Nodes), body.NodeKind)
	}
	for index, item := range body.Nodes {
		if item.Node.Kind != core.NodeKindTerminal {
			t.Errorf("nodes[%d] is a %s", index, item.Node.Kind)
		}
		if index > 0 && item.RecordCount > body.Nodes[index-1].RecordCount {
			t.Errorf("nodes[%d] counts %d after %d", index, item.RecordCount, body.Nodes[index-1].RecordCount)
		}
		near := decodeTimeline(t, handler, "nodeId="+url.QueryEscape(item.Node.Id)+"&depth=1")
		if int64(item.RecordCount) != near.EntryCount+near.UndatedRecordCount {
			t.Errorf("the terminal %s counts %d, the timeline one step away has %d+%d",
				item.Node.Id, item.RecordCount, near.EntryCount, near.UndatedRecordCount)
		}
		if near.EntryCount > 0 && (item.FirstTime == nil || item.LastTime == nil) {
			t.Errorf("the terminal %s carries no time range", item.Node.Id)
		}
	}
}

// 種別の無い要求、レコードの種別、2 つの種別、時系列だけの項目を invalid_request で退ける。
func TestNodeSummariesRejectAnInvalidRequest(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"", "nodeKind=record", "nodeKind=no_such_kind", "nodeKind=ip&nodeKind=terminal",
		"nodeKind=ip&find=x", "nodeKind=ip&depth=1", "nodeKind=ip&source=no-such-source",
	} {
		response := requestNodeSummaries(handler, query)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%q returned status=%d", query, response.Code)
		}
	}
}
