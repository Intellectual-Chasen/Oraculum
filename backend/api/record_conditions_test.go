package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 根拠のレコードを絞る条件のうち、与えたが空の項目を退ける。検索の条件は空の文字列を
// 「条件を与えない」と読むため、query の空の項目が通知なしに作用しない条件にならないことを確かめる。
func TestRecordConditionsRejectAnItemGivenEmpty(t *testing.T) {
	handler := graphHandler(t)
	// 期間の端の文字列は、精度と比較の単位を伴わないと欠けた項目として先に退けられる。
	companions := map[string]string{
		"timeFrom": "&timeFromPrecision=second&filterUnit=second",
		"timeTo":   "&timeToPrecision=second&filterUnit=second",
	}
	for _, item := range []string{
		"terminal", "case", "timeFrom", "timeFromPrecision", "timeTo", "timeToPrecision", "filterUnit",
	} {
		t.Run(item, func(t *testing.T) {
			query := item + "=" + companions[item]
			for name, request := range map[string]func() core.ApiError{
				"graph": func() core.ApiError {
					return requestGraphError(t, handler, graphLimits+"&"+query, http.StatusBadRequest)
				},
				"timeline": func() core.ApiError {
					return requestTimelineError(t, handler, query, http.StatusBadRequest)
				},
			} {
				apiError := request()
				if apiError.Code != core.ApiErrorCodeInvalidRequest ||
					!strings.Contains(apiError.Message, item+" must not be empty") {
					t.Errorf("%s: error=%+v, want %s must not be empty", name, apiError, item)
				}
			}
		})
	}
}

// 反対側。空の項目を持たない要求は、期間を与えない条件として受け付ける。
func TestRecordConditionsAcceptARequestWithoutTheItems(t *testing.T) {
	handler := graphHandler(t)
	if response := requestTimeline(t, handler, ""); response.Code != http.StatusOK {
		t.Errorf("timeline status=%d body=%s", response.Code, response.Body)
	}
}
