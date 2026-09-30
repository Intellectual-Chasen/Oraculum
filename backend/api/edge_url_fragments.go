package api

import (
	"errors"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const edgeUrlFragmentsPattern = "GET /api/v0/edges/{id}/url-fragments"

// edgeUrlFragmentsHandler は、関係 1 本の根拠のレコードが URL の path に書いた番号付きの
// 断片を、送信の回ごとにつないで復号した結果を返す。応答の項目の定義元は core.UrlFragmentJoin である。
//
// 既知の制限: 断片を上限と続きの位置を持たずに全件返す, 応答は関係の根拠の部分集合であり、
// 断片の数は関係の根拠の件数で抑えられる, 応答が関係の詳細の応答の目安を超えたときに
// 回ごとの取得へ分ける
type edgeUrlFragmentsHandler struct {
	graph pipeline.Graph
}

func (h edgeUrlFragmentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	known := map[string]struct{}{matchConditionParam: {}, caseParam: {}}
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
	caseId, err := readRequestedCase(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if apiErr := checkKnownCase(h.graph, caseId); apiErr != nil {
		writeError(w, http.StatusBadRequest, *apiErr)
		return
	}
	join, found := h.graph.UrlFragmentJoin(r.PathValue(edgePathValue), pipeline.EdgeEvidenceFilter{Case: caseId})
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code:    core.ApiErrorCodeRecordNotFound,
			Message: "no edge matches the requested id",
		})
		return
	}
	writeJSON(w, http.StatusOK, join)
}
