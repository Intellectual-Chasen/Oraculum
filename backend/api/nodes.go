package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const nodesPattern = "GET /api/v0/nodes/{id}"

// `/api/v0/nodes/{id}` の要求の項目の名前。id は path が持つ。
const nodePathValue = "id"

// nodesHandler はノード 1 つの詳細を返す。
type nodesHandler struct {
	result pipeline.ImportResult
	graph  pipeline.Graph
}

// nodeResponse は`/api/v0/nodes/{id}` の応答である。
// **本型が項目の定義元である。**
//
// 属性と根拠を全件返す。上限も続きを取る位置も持たない。
type nodeResponse struct {
	Node           core.GraphNode       `json:"node"`
	Attributes     []core.NodeAttribute `json:"attributes"`
	AttributeCount int64                `json:"attributeCount"`
	Evidence       []core.GraphEvidence `json:"evidence"`
	EvidenceCount  int64                `json:"evidenceCount"`
	// EvidenceByCase は evidenceCount を根拠のレコードの案件ごとに分けた件数である。
	// 案件を区別しない取り込みでは出ない。
	EvidenceByCase []core.CaseEvidenceCount `json:"evidenceByCase,omitempty"`
	EdgeCounts     []core.NodeEdgeCount     `json:"edgeCounts"`
	// CreationRecords は対象の生成を記録した根拠のレコードである。
	CreationRecords     []core.GraphEvidence `json:"creationRecords"`
	CreationRecordCount int64                `json:"creationRecordCount"`
	// RelationDerivation は、このレコードを起点にして関係を導いた結果である。
	// レコードの種別のノードだけが持つ。
	RelationDerivation *core.RelationDerivation `json:"relationDerivation,omitempty"`
	// LogonSessionRejections は、このレコードの操作の Logon ID が一致しながら、関係にしな
	// かったログオンのレコードと理由である。全件を持つ。
	LogonSessionRejections []core.LogonSessionRejection `json:"logonSessionRejections"`
}

func (h nodesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseNodeRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	detail, found := h.graph.NodeDetail(id)
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code:    core.ApiErrorCodeRecordNotFound,
			Message: "no node matches the requested id",
		})
		return
	}
	writeJSON(w, http.StatusOK, nodeResponse{
		Node:                   detail.Node,
		Attributes:             emptyIfNil(detail.Attributes),
		AttributeCount:         int64(detail.AttributeCount),
		Evidence:               emptyIfNil(detail.Evidence),
		EvidenceCount:          int64(detail.EvidenceCount),
		EvidenceByCase:         detail.EvidenceByCase,
		EdgeCounts:             emptyIfNil(detail.EdgeCounts),
		CreationRecords:        emptyIfNil(detail.CreationRecords),
		CreationRecordCount:    int64(detail.CreationRecordCount),
		RelationDerivation:     detail.RelationDerivation,
		LogonSessionRejections: emptyIfNil(detail.LogonSessionRejections),
	})
}

// emptyIfNil は要素数 0 の集合を null ではなく空の配列として出す。
func emptyIfNil[Element any](values []Element) []Element {
	if values == nil {
		return []Element{}
	}
	return values
}

// parseNodeRequest は`/api/v0/nodes/{id}` の要求を読み、対象のノードの id を返す。
func parseNodeRequest(r *http.Request) (string, *core.ApiError) {
	if err := checkNodeParameterNames(r.URL.Query()); err != nil {
		return "", invalidRequestError(err, nil)
	}
	id := r.PathValue(nodePathValue)
	if id == "" {
		return "", invalidRequestError(errors.New("id must not be empty"), nil)
	}
	return id, nil
}

// checkNodeParameterNames は要求の項目の名前と多重度を確かめる。
func checkNodeParameterNames(query url.Values) error {
	known := map[string]struct{}{matchConditionParam: {}}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			return errors.New("unsupported query parameter")
		}
		if len(values) != 1 && !repeatedRequestItem(name) {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}
