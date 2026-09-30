package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const recordGraphPattern = "GET /api/v0/record-graph"

// recordGraphHandler は 1 レコードを根拠に持つノードとエッジを返す。レコードの探し方と
// 収集元の検査は `/api/v0/records` と同じである。
type recordGraphHandler struct {
	records recordsHandler
	graph   pipeline.Graph
}

// recordGraphResponse は、1 レコードを根拠に持つノードとエッジの応答である。
// **本型が項目の定義元である。**
type recordGraphResponse struct {
	RecordRef core.RecordLocator `json:"recordRef"`
	// NodeIds は、そのレコードを根拠に持つノードと、EdgeIds のエッジの端点である。
	// レコードがグラフの根拠に入っていないときは空である。
	NodeIds []string `json:"nodeIds"`
	// EdgeIds は、そのレコードを根拠に持つエッジである。
	EdgeIds []string `json:"edgeIds"`
}

func (h recordGraphHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	record, apiError := parseRecordGraphRequest(r.URL.Query())
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	_, entry, apiError := h.records.findCheckedRecord(record)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	elements := h.graph.RecordGraphElements(entry.Locator)
	writeJSON(w, http.StatusOK, recordGraphResponse{
		RecordRef: entry.Locator, NodeIds: elements.NodeIds, EdgeIds: elements.EdgeIds,
	})
}

// parseRecordGraphRequest は 1 レコードを指す項目を読む。関連付けの条件は
// matchSelectingHandler が読む。
func parseRecordGraphRequest(query url.Values) (requestedRecord, *core.ApiError) {
	known := map[string]struct{}{
		sourceIdParam: {}, sourceContentSha256Param: {},
		sequenceNumberParam: {}, lineNumberParam: {}, byteOffsetParam: {},
		matchConditionParam: {},
	}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			return requestedRecord{}, invalidRecordsRequest(errors.New("unsupported query parameter"), nil)
		}
		if len(values) != 1 && name != matchConditionParam {
			return requestedRecord{}, invalidRecordsRequest(errors.New("query parameter must occur once"), nil)
		}
	}
	if missing := missingRecordsParameters(query); len(missing) > 0 {
		return requestedRecord{}, invalidRecordsRequest(
			errors.New("required query parameters are missing"), missing)
	}
	record, err := readRequestedRecord(query, sourceIdParam, sourceContentSha256Param,
		sequenceNumberParam, lineNumberParam, byteOffsetParam)
	if err != nil {
		return requestedRecord{}, invalidRecordsRequest(err, nil)
	}
	return record, nil
}
