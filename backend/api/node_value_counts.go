package api

import (
	"errors"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const nodeValueCountsPattern = "GET /api/v0/nodes/{id}/value-counts"

// 関係の相手側の欄を数える要求の項目の名前。
const (
	relatedEdgeKindParam  = "edgeKind"
	relatedDirectionParam = "direction"
)

// nodeValueCountsHandler は、ノード 1 つに繋がる、ある種別・ある向きの関係の相手側のノードが
// 数える欄に観測した値ごとの件数を返す。
type nodeValueCountsHandler struct {
	graph pipeline.Graph
}

// nodeValueCountsResponse は関係の相手側の欄の値ごとの件数の応答である。
// **本型が項目の定義元である。**
type nodeValueCountsResponse struct {
	// NodeId と EdgeKind と Direction と CountBy は、要求が与えた値をそのまま返す。
	NodeId    string             `json:"nodeId"`
	EdgeKind  core.EdgeKind      `json:"edgeKind"`
	Direction core.EdgeDirection `json:"direction"`
	CountBy   string             `json:"countBy"`
	// ValueCounts は、その関係の根拠のレコードに観測した値ごとの件数である。並びは最初に
	// 見た順である。
	ValueCounts []core.ValueCount `json:"valueCounts"`
	// EmptyReason は、数える欄の名前をグラフのどこにも観測していないとき (no_field_observed) と、
	// 欄の名前はあるが読めた値がどこにも無いとき (no_readable_value) だけ出る。値はあるが
	// 相手に値が無いときと、その種別と向きの関係が無いときは、空の配列で出ない。
	EmptyReason core.EmptyReason `json:"emptyReason,omitempty"`
}

func (h nodeValueCountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	known := map[string]struct{}{
		matchConditionParam: {}, relatedEdgeKindParam: {}, relatedDirectionParam: {}, countByParam: {},
	}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("unsupported query parameter"), nil))
			return
		}
		if len(values) != 1 && !repeatedRequestItem(name) {
			writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("query parameter must occur once"), nil))
			return
		}
	}
	var missing []string
	for _, name := range []string{relatedEdgeKindParam, relatedDirectionParam, countByParam} {
		if query.Get(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		writeError(w, http.StatusBadRequest,
			*invalidRequestError(errors.New("required query parameters are missing"), missing))
		return
	}
	kind := core.EdgeKind(query.Get(relatedEdgeKindParam))
	direction := core.EdgeDirection(query.Get(relatedDirectionParam))
	if !kind.IsKnown() || !direction.IsKnown() {
		writeError(w, http.StatusBadRequest,
			*invalidRequestError(errors.New("edgeKind and direction must be known values"), nil))
		return
	}
	id := r.PathValue(nodePathValue)
	countBy := query.Get(countByParam)
	semantic, name := pipeline.DesignatedField(countBy)
	graphQuery := pipeline.GraphQuery{CountBySemantic: semantic, CountByName: name}
	counts, found := h.graph.RelatedValueCounts(id, kind, direction, graphQuery)
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code:    core.ApiErrorCodeRecordNotFound,
			Message: "no node matches the requested id",
		})
		return
	}
	response := nodeValueCountsResponse{
		NodeId: id, EdgeKind: kind, Direction: direction, CountBy: countBy,
		ValueCounts: emptyIfNil(counts),
	}
	if len(counts) == 0 {
		// 関係の相手の種別は要求で選ばないため、種別と絞り込みの理由を出さない。
		if reason, _ := h.graph.CountedFieldObservationOf(graphQuery).EmptyReason(graphQuery); reason == core.EmptyReasonNoFieldObserved ||
			reason == core.EmptyReasonNoReadableValue {
			response.EmptyReason = reason
		}
	}
	writeJSON(w, http.StatusOK, response)
}
