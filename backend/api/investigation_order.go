package api

import (
	"errors"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const investigationOrderPattern = "GET /api/v0/investigation-order/{method}"

// investigationOrderMethodPathValue は手法の名前を置く path の値の名前である。
const investigationOrderMethodPathValue = "method"

// investigationOrderHandler は、部分グラフの対象を 1 つの手法の値で並べた、調べる順序の目安を返す。
//
// **要求の項目はグラフの要求と同じである。** 使うのはレコードの絞り込みと起点と段数だけで、
// 表示の条件 (描画の上限・粒度・種別) は並びを変えない (pipeline.Graph.InvestigationOrder)。
type investigationOrderHandler struct {
	graph pipeline.Graph
	sigma pipeline.SigmaEvaluation
}

// investigationOrderResponse は調べる順序の目安の応答である。**本型が項目の定義元である。**
//
// 目安は関係の状態と別の属性であり、関係の成立や対象の性質を判定しない。
type investigationOrderResponse struct {
	// Method は並べた手法の名前である。
	Method string `json:"method"`
	// Parameters は手法が使った係数である。係数を持たない手法では空の配列である。
	Parameters []investigationOrderParameterItem `json:"parameters"`
	// Inputs は計算に使った対象・組・レコードの件数と期間である。
	Inputs investigationOrderInputsItem `json:"inputs"`
	// Kinds は対象の種別ごとの並びである。種別の文字列の昇順に並び、全件を返す。
	Kinds []investigationOrderKindItem `json:"kinds"`
}

// investigationOrderParameterItem は手法の係数 1 つである。
type investigationOrderParameterItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// investigationOrderInputsItem は計算に使った入力の大きさである (pipeline.InvestigationOrderInputs)。
type investigationOrderInputsItem struct {
	ObjectCount      int `json:"objectCount"`
	PairCount        int `json:"pairCount"`
	RecordCount      int `json:"recordCount"`
	TimedRecordCount int `json:"timedRecordCount"`
	// TimeRange は時点を持つレコードの時刻の両端である。時点を持つレコードが無いときは出ない。
	TimeRange *core.TimeRange `json:"timeRange,omitempty"`
}

// investigationOrderKindItem は 1 つの種別の対象の並びである。
type investigationOrderKindItem struct {
	Kind    core.NodeKind                 `json:"kind"`
	Entries []investigationOrderEntryItem `json:"entries"`
}

// investigationOrderEntryItem は並びの 1 行である (pipeline.InvestigationOrderEntry)。
type investigationOrderEntryItem struct {
	Node core.GraphNode `json:"node"`
	// Value は手法の値である。手法が値を決められない対象では null である。
	Value    *float64 `json:"value"`
	Rank     int      `json:"rank"`
	TieCount int      `json:"tieCount"`
}

// sigmaMinLevelParam は、Sigma の手法が一致を数えるルールのレベルの下限である。省略は全レベルを数える。
// 値は informational / low / medium / high / critical のどれかで、ほかの手法は使わない。
const sigmaMinLevelParam = "sigmaMinLevel"

// sigmaMinLevelOf は要求から下限のレベルを取り出し、残りの項目をグラフの要求として読む要求を返す。
func sigmaMinLevelOf(r *http.Request) (string, *http.Request, *core.ApiError) {
	query := r.URL.Query()
	levels, given := query[sigmaMinLevelParam]
	if !given {
		return "", r, nil
	}
	if len(levels) != 1 || !pipeline.KnownSigmaLevel(levels[0]) {
		return "", nil, invalidRequestError(
			errors.New("sigmaMinLevel must be one known Sigma rule level"), []string{sigmaMinLevelParam})
	}
	query.Del(sigmaMinLevelParam)
	rest := r.Clone(r.Context())
	rest.URL.RawQuery = query.Encode()
	return levels[0], rest, nil
}

func (h investigationOrderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sigmaMinLevel, r, apiError := sigmaMinLevelOf(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	request, apiError := parseGraphRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownSearch(h.graph, request.search); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	order, known := h.graph.InvestigationOrder(
		request.query(), r.PathValue(investigationOrderMethodPathValue), h.sigma, sigmaMinLevel)
	if !known {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("the method is not a known investigation order method"),
			[]string{investigationOrderMethodPathValue}))
		return
	}
	response := investigationOrderResponse{
		Method:     order.Method,
		Parameters: make([]investigationOrderParameterItem, 0, len(order.Parameters)),
		Inputs: investigationOrderInputsItem{
			ObjectCount: order.Inputs.ObjectCount, PairCount: order.Inputs.PairCount,
			RecordCount: order.Inputs.RecordCount, TimedRecordCount: order.Inputs.TimedRecordCount,
			TimeRange: order.Inputs.TimeRange,
		},
		Kinds: make([]investigationOrderKindItem, 0, len(order.Kinds)),
	}
	for _, parameter := range order.Parameters {
		response.Parameters = append(response.Parameters, investigationOrderParameterItem(parameter))
	}
	for _, kind := range order.Kinds {
		entries := make([]investigationOrderEntryItem, 0, len(kind.Entries))
		for _, entry := range kind.Entries {
			entries = append(entries, investigationOrderEntryItem(entry))
		}
		response.Kinds = append(response.Kinds, investigationOrderKindItem{Kind: kind.Kind, Entries: entries})
	}
	writeJSON(w, http.StatusOK, response)
}
