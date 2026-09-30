package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const eventKindsPattern = "GET /api/v0/event-kinds"

// eventKindsHandler は、取り込んだレコードが持つ事象の分類と動作の組を件数とともに返す。
// 画面は、事象の種別の条件に与える値の選択肢に使う。
type eventKindsHandler struct {
	graph  pipeline.Graph
	result pipeline.ImportResult
}

// eventKindItem は事象の分類と動作の組 1 つである。
type eventKindItem struct {
	// Category と Action は原資料の文字列である。動作の欄を持たないレコードの組は
	// Action を持たない。
	Category string `json:"category"`
	Action   string `json:"action,omitempty"`
	// RecordCount はこの組を持ち、絞り込みを通ったレコードの件数である。1 以上である。
	RecordCount int64 `json:"recordCount"`
	// WindowsEvent は、Category がプロバイダの名前、Action がイベント ID である組かである。
	// 事象の分類を持たない Windows イベントログのレコードの組が真になる。
	WindowsEvent bool `json:"windowsEvent,omitempty"`
}

// eventKindsResponse は事象の種別の一覧の応答である。
// **本型が項目の定義元である。**
type eventKindsResponse struct {
	// Kinds は件数の多い順に並び、同じ件数の組は分類、動作の文字列の順に並ぶ。
	Kinds []eventKindItem `json:"kinds"`
	// UncategorizedRecordCount は、絞り込みを通りながら事象の分類を持たないレコードの
	// 件数である。**この件数のレコードは、事象の種別の条件を与えた要求の結果に入らない。**
	UncategorizedRecordCount int64 `json:"uncategorizedRecordCount"`
	// Case と Terminal と SourceId は、要求が与えた絞り込みの条件をそのまま返す。
	// SourceId を与えた要求は、その収集元の取り込めたレコードだけを数える。取り込めなかった
	// レコードは組にも UncategorizedRecordCount にも入らない。
	Case     string `json:"case,omitempty"`
	Terminal string `json:"terminal,omitempty"`
	SourceId string `json:"sourceId,omitempty"`
	// SemanticFields と FieldNames は、取り込んだ全レコードが持つ欄の語彙の項目と原資料の
	// key である。どちらも昇順に並び、重複を持たない。**要求の絞り込みを使わない。**
	// グラフの searchExpression・fieldEquals・fieldContains・countBy に書ける欄の名前である。
	SemanticFields []core.SemanticKey `json:"semanticFields"`
	FieldNames     []string           `json:"fieldNames"`
}

func (h eventKindsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if err := checkEventKindsParameterNames(query); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	conditions, err := readRecordConditions(query)
	if err == nil {
		err = conditions.Validate()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	request := recordConditionsRequest{conditions: conditions}
	if apiError := checkKnownCase(h.graph, conditions.Case); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownTerminal(h.graph, conditions.Terminal); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	source, apiError := requestedEventKindsSource(query)
	if apiError != nil {
		writeError(w, http.StatusBadRequest, *apiError)
		return
	}
	if source.sourceId != "" {
		if _, apiError := checkRequestedSource(h.result, source); apiError != nil {
			writeError(w, httpStatusFor(apiError.Code), *apiError)
			return
		}
	}
	kinds, uncategorized := h.graph.EventKindsInSource(request.recordFilter(), source.sourceId)
	response := eventKindsResponse{
		Kinds:                    make([]eventKindItem, 0, len(kinds)),
		UncategorizedRecordCount: uncategorized,
		Case:                     conditions.Case,
		Terminal:                 conditions.Terminal,
		SourceId:                 source.sourceId,
	}
	response.SemanticFields, response.FieldNames = h.graph.ObservedFieldNames()
	for _, kind := range kinds {
		response.Kinds = append(response.Kinds, eventKindItem{
			Category: kind.Category, Action: kind.Action, RecordCount: kind.RecordCount,
			WindowsEvent: kind.WindowsEvent,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

// checkEventKindsParameterNames は要求の項目の名前と多重度を確かめる。
//
// **事象の分類と動作の項目を退ける。** 本操作はその 2 つの条件の選択肢を返すため、
// 与えられても使わない。通知せずに捨てずに退ける。
func checkEventKindsParameterNames(query url.Values) error {
	known := map[string]struct{}{
		matchConditionParam: {}, caseParam: {}, terminalParam: {},
		sourceIdParam: {}, sourceContentSha256Param: {},
	}
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

// requestedEventKindsSource は、要求が収集元で絞るときの収集元を返す。sourceId と
// sourceContentSha256 は組で与える。どちらも与えない要求は、全収集元を数える。
func requestedEventKindsSource(query url.Values) (requestedSource, *core.ApiError) {
	if query.Has(sourceIdParam) == query.Has(sourceContentSha256Param) {
		return requestedSource{sourceId: query.Get(sourceIdParam), contentSha256: query.Get(sourceContentSha256Param)}, nil
	}
	missing := sourceIdParam
	if query.Has(sourceIdParam) {
		missing = sourceContentSha256Param
	}
	return requestedSource{}, invalidRequestError(errors.New("required query parameters are missing"), []string{missing})
}
