package api_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const investigationOrderPath = "/api/v0/investigation-order/"

type investigationOrderResponse struct {
	Method     string `json:"method"`
	Parameters []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"parameters"`
	Inputs struct {
		ObjectCount      int             `json:"objectCount"`
		PairCount        int             `json:"pairCount"`
		RecordCount      int             `json:"recordCount"`
		TimedRecordCount int             `json:"timedRecordCount"`
		TimeRange        *core.TimeRange `json:"timeRange"`
	} `json:"inputs"`
	Kinds []struct {
		Kind    core.NodeKind `json:"kind"`
		Entries []struct {
			Node     core.GraphNode `json:"node"`
			Value    *float64       `json:"value"`
			Rank     int            `json:"rank"`
			TieCount int            `json:"tieCount"`
		} `json:"entries"`
	} `json:"kinds"`
}

func requestInvestigationOrder(
	t *testing.T, handler http.Handler, method, query string, status int,
) investigationOrderResponse {
	t.Helper()
	response := requestPath(t, handler, http.MethodGet,
		withAllMatchConditions(investigationOrderPath+method+"?"+query))
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	if status != http.StatusOK {
		var problem core.ApiError
		decodeJSON(t, response.Body, &problem)
		if problem.Code != core.ApiErrorCodeInvalidRequest {
			t.Fatalf("error=%+v", problem)
		}
		return investigationOrderResponse{}
	}
	var payload investigationOrderResponse
	decodeJSON(t, response.Body, &payload)
	return payload
}

func TestInvestigationOrderReturnsRankedObjectsByKind(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	got := requestInvestigationOrder(t, handler, "record_count_descending", wholeGraphQuery, http.StatusOK)

	if got.Method != "record_count_descending" || got.Parameters == nil || len(got.Parameters) != 0 {
		t.Errorf("method %q parameters %+v, want the method and an empty parameter list", got.Method, got.Parameters)
	}
	if got.Inputs.ObjectCount == 0 || got.Inputs.RecordCount == 0 || got.Inputs.TimeRange == nil {
		t.Errorf("inputs = %+v, want objects, records and their time range", got.Inputs)
	}
	for _, kind := range got.Kinds {
		if kind.Kind == core.NodeKindRecord {
			t.Errorf("the order carries record nodes: %+v", kind)
		}
		if len(kind.Entries) == 0 || kind.Entries[0].Rank != 1 || kind.Entries[0].Value == nil {
			t.Errorf("%s entries = %+v, want a first entry at rank 1 with a value", kind.Kind, kind.Entries)
		}
	}
}

func TestInvestigationOrderDoesNotDependOnDisplayConditions(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	var orders []investigationOrderResponse
	for _, display := range []string{
		wholeGraphQuery,
		"nodeLimit=1&depth=1",
		"depth=1&granularity=object&nodeKind=terminal",
	} {
		orders = append(orders,
			requestInvestigationOrder(t, handler, "record_count_ascending", display, http.StatusOK))
	}
	for _, order := range orders[1:] {
		if !reflect.DeepEqual(order, orders[0]) {
			t.Errorf("order = %+v, want the same order for every display condition %+v", order, orders[0])
		}
	}
}

func TestInvestigationOrderPassesTheSigmaMinimumLevel(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	for query, want := range map[string]string{"": "all", "&sigmaMinLevel=medium": "medium"} {
		got := requestInvestigationOrder(t, handler, "sigma", wholeGraphQuery+query, http.StatusOK)
		var minLevel string
		for _, parameter := range got.Parameters {
			if parameter.Name == "min_level" {
				minLevel = parameter.Value
			}
		}
		if minLevel != want {
			t.Errorf("query %q: parameters = %+v, want min_level = %s", query, got.Parameters, want)
		}
	}
}

func TestInvestigationOrderRejectsInvalidRequests(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	requestInvestigationOrder(t, handler, "unknown_method", wholeGraphQuery, http.StatusBadRequest)
	for _, query := range []string{
		"case=missing", "depth=bad", "unexpected=value",
		"sigmaMinLevel=severe", "sigmaMinLevel=High", "sigmaMinLevel=low&sigmaMinLevel=high",
	} {
		requestInvestigationOrder(t, handler, "record_count_descending",
			wholeGraphQuery+"&"+query, http.StatusBadRequest)
	}
	response := requestPath(t, handler, http.MethodPost,
		withAllMatchConditions(investigationOrderPath+"record_count_descending?"+wholeGraphQuery))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", response.Code)
	}
}
