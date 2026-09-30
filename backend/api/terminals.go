package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const (
	terminalsPattern      = "GET /api/v0/terminals"
	terminalPattern       = "GET /api/v0/terminals/{id}"
	terminalEventsPattern = "GET /api/v0/terminals/{id}/events"
)

// 端末のレコードを返す要求の項目の名前。
const (
	terminalCategoryParam = "category"
	terminalSourceIpParam = "sourceIp"
)

// terminalsHandler は端末の一覧を返す。
type terminalsHandler struct {
	graph pipeline.Graph
}

// terminalsResponse は端末の一覧の応答である。**本型が項目の定義元である。**
type terminalsResponse struct {
	Terminals []core.TerminalSummary `json:"terminals"`
}

func (h terminalsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := checkTerminalParameterNames(r.URL.Query()); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	writeJSON(w, http.StatusOK, terminalsResponse{Terminals: h.graph.TerminalSummaries()})
}

// terminalHandler は端末 1 台の情報を返す。応答は core.TerminalDetail である。
type terminalHandler struct {
	graph pipeline.Graph
}

func (h terminalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := checkTerminalParameterNames(r.URL.Query()); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	detail, found := h.graph.TerminalDetail(r.PathValue(nodePathValue))
	if !found {
		writeTerminalNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// terminalEventsHandler は端末の分類に入るレコードを時刻の昇順に返す。
type terminalEventsHandler struct {
	graph pipeline.Graph
}

// terminalEventsResponse は端末のレコードの応答である。**本型が項目の定義元である。**
type terminalEventsResponse struct {
	TerminalId string `json:"terminalId"`
	// Category と SourceIp は要求が与えた条件をそのまま返す。省略した項目は出ない。
	Category core.TerminalCategory `json:"category,omitempty"`
	SourceIp string                `json:"sourceIp,omitempty"`
	Events   []core.TerminalEvent  `json:"events"`
}

func (h terminalEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if err := checkTerminalParameterNames(query, terminalCategoryParam, terminalSourceIpParam); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	category := core.TerminalCategory(query.Get(terminalCategoryParam))
	if category != "" && !category.IsKnown() {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("category must be a known value"), nil))
		return
	}
	id := r.PathValue(nodePathValue)
	sourceIp := query.Get(terminalSourceIpParam)
	events, found := h.graph.TerminalEvents(id, pipeline.TerminalEventQuery{Category: category, SourceIp: sourceIp})
	if !found {
		writeTerminalNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, terminalEventsResponse{
		TerminalId: id, Category: category, SourceIp: sourceIp, Events: events,
	})
}

// writeTerminalNotFound は、端末のノードでない id への 404 を書く。
func writeTerminalNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, core.ApiError{
		Code: core.ApiErrorCodeRecordNotFound, Message: "no terminal node matches the requested id",
	})
}

// checkTerminalParameterNames は、要求の項目が matchCondition と names だけであり、names の項目が
// 1 回ずつ現れることを確かめる。
func checkTerminalParameterNames(query url.Values, names ...string) error {
	known := map[string]struct{}{matchConditionParam: {}}
	for _, name := range names {
		known[name] = struct{}{}
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
