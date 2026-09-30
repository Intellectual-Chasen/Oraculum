// in-package test: 上限は非公開の定数であり、外の test package から読めない。
package api

import (
	"net/http/httptest"
	"testing"
)

// 条件を起点だけに当てる項目は true だけを受け付け、要求へ写す。
func TestTheGraphRequestReadsConditionsOnOriginsOnly(t *testing.T) {
	for _, testCase := range []struct {
		suffix   string
		want     bool
		rejected bool
	}{
		{"", false, false},
		{"&conditionsOnOriginsOnly=true", true, false},
		{"&conditionsOnOriginsOnly=false", false, true},
		{"&conditionsOnOriginsOnly=", false, true},
	} {
		request, apiError := parseGraphRequest(httptest.NewRequest("GET",
			"/api/v0/graph?depth=2"+testCase.suffix, nil))
		if (apiError != nil) != testCase.rejected {
			t.Errorf("%q: rejected=%v, want %v", testCase.suffix, apiError != nil, testCase.rejected)
			continue
		}
		if apiError == nil && request.query().ConditionsOnOriginsOnly != testCase.want {
			t.Errorf("%q: conditionsOnOriginsOnly=%v, want %v", testCase.suffix,
				request.query().ConditionsOnOriginsOnly, testCase.want)
		}
	}
}

// 端点のレコードを期間で判定する項目は true だけを受け付け、要求へ写す。
func TestTheGraphRequestReadsEndpointRecordsInPeriod(t *testing.T) {
	for suffix, want := range map[string]bool{"": false, "&endpointRecordsInPeriod=true": true} {
		request, apiError := parseGraphRequest(httptest.NewRequest("GET",
			"/api/v0/graph?depth=1"+suffix, nil))
		if apiError != nil || request.query().EndpointRecordsInPeriod != want {
			t.Errorf("%q: error=%v endpointRecordsInPeriod=%v, want %v", suffix, apiError,
				request.query().EndpointRecordsInPeriod, want)
		}
	}
	if _, apiError := parseGraphRequest(httptest.NewRequest("GET",
		"/api/v0/graph?depth=1&endpointRecordsInPeriod=false", nil)); apiError == nil {
		t.Error("endpointRecordsInPeriod=false was accepted")
	}
}

// ノードの数の上限は、項目が無い要求で 0 (上限なし) になり、与えた値を要求へ写す。
func TestTheGraphRequestReadsTheNodeLimit(t *testing.T) {
	for suffix, want := range map[string]int{"": 0, "&nodeLimit=1": 1, "&nodeLimit=5000": 5000} {
		request, apiError := parseGraphRequest(httptest.NewRequest("GET", "/api/v0/graph?depth=1"+suffix, nil))
		if apiError != nil || request.query().NodeLimit != want {
			t.Errorf("%q: error=%v nodeLimit=%d, want %d", suffix, apiError, request.query().NodeLimit, want)
		}
	}
}
