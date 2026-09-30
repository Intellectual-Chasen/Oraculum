package api_test

import (
	"net/http"
	"net/url"
	"testing"
)

// nodeValueCountsResponse は関係の相手側の欄の値ごとの件数の応答の項目名を test 側で固定する。
type nodeValueCountsResponse struct {
	NodeId      string          `json:"nodeId"`
	EdgeKind    string          `json:"edgeKind"`
	Direction   string          `json:"direction"`
	CountBy     string          `json:"countBy"`
	ValueCounts []valueCountRow `json:"valueCounts"`
	EmptyReason string          `json:"emptyReason,omitempty"`
}

type valueCountRow struct {
	Value       string          `json:"value"`
	RecordCount int64           `json:"recordCount"`
	First       *map[string]any `json:"firstEventTime,omitempty"`
	Last        *map[string]any `json:"lastEventTime,omitempty"`
	Intervals   *map[string]any `json:"intervals,omitempty"`
}

func nodeValueCountsPath(nodeId, query string) string {
	return withAllMatchConditions(nodesPath + url.PathEscape(nodeId) + "/value-counts?" + query)
}

// 端末に内向きの ran_on の相手 (プロセス) の欄を数え、要求の値をそのまま返す。どこにも無い欄は
// 理由を付けて空で返す。
func TestNodeValueCountsCountTheProcessesThatRanOnTheTerminal(t *testing.T) {
	handler := graphHandler(t)
	terminal := terminalNodeId(t, handler)
	response := requestPath(t, handler, http.MethodGet, nodeValueCountsPath(terminal,
		"edgeKind=ran_on&direction=incoming&countBy=process.binary_path"))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded nodeValueCountsResponse
	decodeJSON(t, response.Body, &decoded)
	if decoded.NodeId != terminal || decoded.EdgeKind != "ran_on" || decoded.Direction != "incoming" ||
		decoded.CountBy != "process.binary_path" || len(decoded.ValueCounts) == 0 || decoded.EmptyReason != "" {
		t.Fatalf("response = %+v, want the process paths of the terminal", decoded)
	}

	missing := requestPath(t, handler, http.MethodGet, nodeValueCountsPath(terminal,
		"edgeKind=ran_on&direction=incoming&countBy=absentField"))
	var empty nodeValueCountsResponse
	decodeJSON(t, missing.Body, &empty)
	if missing.Code != http.StatusOK || len(empty.ValueCounts) != 0 || empty.EmptyReason != "no_field_observed" {
		t.Errorf("an absent field gives %d %+v, want an empty list with no_field_observed", missing.Code, empty)
	}
}

func TestNodeValueCountsRejectIncompleteOrUnknownItems(t *testing.T) {
	handler := graphHandler(t)
	terminal := terminalNodeId(t, handler)
	for query, status := range map[string]int{
		"direction=incoming&countBy=process.binary_path":                    http.StatusBadRequest,
		"edgeKind=ran_on&countBy=process.binary_path":                       http.StatusBadRequest,
		"edgeKind=ran_on&direction=incoming":                                http.StatusBadRequest,
		"edgeKind=logged_on&direction=incoming&countBy=process.binary_path": http.StatusBadRequest,
		"edgeKind=ran_on&direction=sideways&countBy=process.binary_path":    http.StatusBadRequest,
		"edgeKind=ran_on&direction=incoming&countBy=x&unexpected=value":     http.StatusBadRequest,
		// グラフの要求だけが関係の種別の繰り返しを受け付ける。
		"edgeKind=ran_on&edgeKind=ran_on&direction=incoming&countBy=x": http.StatusBadRequest,
	} {
		if response := requestPath(t, handler, http.MethodGet, nodeValueCountsPath(terminal, query)); response.Code != status {
			t.Errorf("query=%q status=%d want %d", query, response.Code, status)
		}
	}
	if response := requestPath(t, handler, http.MethodGet, nodeValueCountsPath("n:absent",
		"edgeKind=ran_on&direction=incoming&countBy=process.binary_path")); response.Code != http.StatusNotFound {
		t.Errorf("an absent node status=%d, want 404", response.Code)
	}
}
