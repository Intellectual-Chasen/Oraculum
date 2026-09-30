package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const nodeSummariesPattern = "GET /api/v0/node-summaries"

// nodeSummariesHandler は 1 つの種別のノードごとに、接するレコードの件数と時刻の範囲を返す。
type nodeSummariesHandler struct {
	graph pipeline.Graph
}

// nodeSummaryItem は nodeSummariesResponse の 1 件である。
type nodeSummaryItem struct {
	Node core.GraphNode `json:"node"`
	// Terminal は、端末の範囲に置いたアドレス (keyForm が terminal_id_address) の、範囲の端末の
	// ノードである。範囲を持たないノードと、範囲の端末のノードがグラフに無いときは出ない。
	Terminal *core.GraphNode `json:"terminal,omitempty"`
	// RecordCount は、ノードの根拠と、ノードに接するエッジの根拠のうち、絞り込みを通った
	// レコードの件数である。
	RecordCount int `json:"recordCount"`
	// FirstTime と LastTime は、数えたレコードの最も早い時刻と最も遅い時刻である。
	// 時刻を比べられるレコードが無いときは出ない。
	FirstTime *core.Timestamp `json:"firstTime,omitempty"`
	LastTime  *core.Timestamp `json:"lastTime,omitempty"`
	// LocalTimeRecordCount は、数えたレコードのうち UTC からのずれの決まらない地方時のもので、
	// UndatedRecordCount は時刻を持たないものである。どちらも FirstTime と LastTime の範囲に入らない。
	LocalTimeRecordCount int `json:"localTimeRecordCount"`
	UndatedRecordCount   int `json:"undatedRecordCount"`
}

// nodeSummariesResponse はノードの一覧の応答である。**本型が項目の定義元である。**
type nodeSummariesResponse struct {
	// Nodes は、絞り込みを通ったレコードが 1 件以上接するノードである。件数の降順、同じ件数では
	// 識別子の昇順に並ぶ。
	Nodes     []nodeSummaryItem `json:"nodes"`
	NodeCount int               `json:"nodeCount"`
	NodeKind  core.NodeKind     `json:"nodeKind"`
}

// summaryFilterRequest は、時系列と同じ絞り込みの項目 (検索式を含む) である。ノードの一覧と
// 件数の分布の要求が読む。
type summaryFilterRequest struct {
	records recordConditionsRequest
	searchExpressionRequest
}

// nodeSummariesRequest はノードの一覧の要求の項目である。絞り込みの項目は時系列と同じである。
type nodeSummariesRequest struct {
	summaryFilterRequest
	kind core.NodeKind
}

func (h nodeSummariesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseNodeSummariesRequest(r.URL.Query())
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownRecordConditions(h.graph, request.records.conditions); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	summaries := h.graph.NodeSummaries(request.kind, request.records.recordFilter(), request.searchExpression)
	response := nodeSummariesResponse{
		Nodes: make([]nodeSummaryItem, 0, len(summaries)), NodeCount: len(summaries), NodeKind: request.kind,
	}
	for _, summary := range summaries {
		response.Nodes = append(response.Nodes, nodeSummaryItem{
			Node: summary.Node, Terminal: summary.Terminal, RecordCount: summary.RecordCount,
			FirstTime: summary.FirstTime, LastTime: summary.LastTime,
			LocalTimeRecordCount: summary.LocalTimeRecordCount, UndatedRecordCount: summary.UndatedRecordCount,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

// parseNodeSummariesRequest は種別と、根拠のレコードの絞り込みを読む。種別は 1 つだけを
// 必ず与え、レコードのノードは選べない。絞り込みの項目は時系列と同じである。
func parseNodeSummariesRequest(query url.Values) (nodeSummariesRequest, *core.ApiError) {
	filter, apiError := parseSummaryFilter(query, nodeKindParam)
	request := nodeSummariesRequest{summaryFilterRequest: filter}
	if apiError != nil {
		return request, apiError
	}
	request.kind = core.NodeKind(query.Get(nodeKindParam))
	if len(query[nodeKindParam]) != 1 || !request.kind.IsKnown() || request.kind == core.NodeKindRecord {
		return request, invalidRequestError(
			errors.New(nodeKindParam+" must be one node kind other than record"), nil)
	}
	return request, nil
}

// parseSummaryFilter は、時系列と同じ絞り込みの項目と、必ず与える own の項目を持つ要求を
// 読む。起点のノードと原文の検索の項目は退ける。own の値は呼び出し側が読む。
func parseSummaryFilter(query url.Values, own string) (summaryFilterRequest, *core.ApiError) {
	var request summaryFilterRequest
	withoutOwn := url.Values{}
	for name, values := range query {
		if name != own {
			withoutOwn[name] = values
		}
	}
	if err := checkTimelineParameterNames(withoutOwn); err != nil {
		return request, invalidRequestError(err, nil)
	}
	if query.Has(nodeIdParam) || query.Has(depthParam) || query.Has(findParam) ||
		query.Has(findCaseSensitiveParam) {
		return request, invalidRequestError(errors.New("unsupported query parameter"), nil)
	}
	if !query.Has(own) {
		return request, invalidRequestError(
			errors.New("required query parameters are missing"), []string{own})
	}
	if missing := missingTimelineParameters(query); len(missing) > 0 {
		return request, invalidRequestError(
			errors.New("required query parameters are missing"), missing)
	}
	if err := checkTimelineEmptyValues(query); err != nil {
		return request, invalidRequestError(err, nil)
	}
	records, err := readRecordConditionsRequest(query)
	if err != nil {
		return request, invalidRequestError(err, nil)
	}
	request.records = records
	if apiError := request.readSearchExpression(query); apiError != nil {
		return request, apiError
	}
	return request, nil
}
