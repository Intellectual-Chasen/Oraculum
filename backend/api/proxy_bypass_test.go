package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const proxyBypassPath = "/api/v0/proxy-bypass"

func requestProxyBypass(t *testing.T, handler http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(proxyBypassPath+"?"+query), nil))
	return response
}

// Proxy のログの収集元は接続元のアドレスごとの要求の件数を返す。Proxy のログでない収集元と、
// 収集元を指さない要求を退ける。
func TestProxyBypassCountsTheRequestsOfTheProxySource(t *testing.T) {
	handler := graphHandler(t)
	page := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	queries := map[string]string{}
	sourceIds := map[string]string{}
	for _, item := range page.Sources {
		sourceIds[item.Source.FileName] = item.Source.SourceId
		queries[item.Source.FileName] = "sourceId=" + url.QueryEscape(item.Source.SourceId) +
			"&sourceContentSha256=" + url.QueryEscape(item.Source.ContentSha256)
	}

	response := requestProxyBypass(t, handler, queries["graph-squid.log"])
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	// 収集元の 5 行のうち 4 行が 192.0.2.1、1 行が 192.0.2.9 からの要求である。割当が無いため
	// Proxy のアドレスは空であり、接続を数えない。
	want := `{"sourceId":"` + sourceIds["graph-squid.log"] + `","proxyAddresses":[],"unreadableProxyRecordCount":0,"clients":[` +
		`{"clientIp":"192.0.2.1","terminals":[],"proxyRequestCount":4,"proxyConnectionCount":0,"directConnectionCount":0,"directConnectionCountable":false,"directDestinations":[]},` +
		`{"clientIp":"192.0.2.9","terminals":[],"proxyRequestCount":1,"proxyConnectionCount":0,"directConnectionCount":0,"directConnectionCountable":false,"directDestinations":[]}]}`
	if body := strings.TrimSpace(response.Body.String()); body != want {
		t.Errorf("the comparison is\n%s\nwant\n%s", body, want)
	}

	notProxy := requestProxyBypass(t, handler, queries["graph-markii.log"])
	if notProxy.Code != http.StatusBadRequest || !strings.Contains(notProxy.Body.String(), `"sourceId"`) {
		t.Errorf("a non-proxy source: status=%d body=%s, want 400 naming sourceId", notProxy.Code, notProxy.Body.String())
	}
	for query, status := range map[string]int{
		"":                                       http.StatusBadRequest,
		"sourceId=absent&sourceContentSha256=00": http.StatusNotFound,
	} {
		if response := requestProxyBypass(t, handler, query); response.Code != status {
			t.Errorf("query=%q status=%d want %d body=%s", query, response.Code, status, response.Body.String())
		}
	}
}
