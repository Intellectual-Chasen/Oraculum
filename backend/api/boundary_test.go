package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// boundaryRequest は境界の wrapper に要求を 1 つ渡し、次の handler に届いたかと応答を返す。
func boundaryRequest(t *testing.T, allowed []string, request *http.Request) (bool, *httptest.ResponseRecorder) {
	t.Helper()
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler, err := api.NewBoundaryHandler(next, allowed)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return reached, recorder
}

func newBoundaryRequest(method, host, path string, headers map[string]string) *http.Request {
	request := httptest.NewRequest(method, "http://placeholder"+path, nil)
	request.Host = host
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request
}

var boundaryHosts = []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "192.0.2.10:8080", "oraculum.example"}

func TestBoundaryPassesRequestsFromAllowedHosts(t *testing.T) {
	for name, request := range map[string]*http.Request{
		"GET by the loopback address": newBoundaryRequest(http.MethodGet, "127.0.0.1:8080", "/api/v0/graph", nil),
		"GET by the loopback name in upper case": newBoundaryRequest(http.MethodGet, "LOCALHOST:8080",
			"/api/v0/graph", nil),
		"GET by the IPv6 loopback": newBoundaryRequest(http.MethodGet, "[::1]:8080", "/api/v0/graph", nil),
		"GET by the allowed name on port 80": newBoundaryRequest(http.MethodGet, "oraculum.example",
			"/api/v0/graph", nil),
		"GET by the allowed name with port 80": newBoundaryRequest(http.MethodGet, "oraculum.example:80",
			"/api/v0/graph", nil),
		"POST from the same origin": newBoundaryRequest(http.MethodPost, "192.0.2.10:8080",
			"/api/v0/assertions", map[string]string{"Origin": "http://192.0.2.10:8080", "Sec-Fetch-Site": "same-origin"}),
		// 別の許可した名前で開いた画面も同じ server である。
		"POST from another allowed origin": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080",
			"/api/v0/assertions", map[string]string{"Origin": "http://localhost:8080"}),
		// body を持たない POST は Content-Type を持たない。
		"POST without a body": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080",
			"/api/v0/stages/processing", map[string]string{"Origin": "http://127.0.0.1:8080"}),
		// Origin を持たない要求は browser 以外の client が送る。
		"POST without Origin": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080",
			"/api/v0/stages/processing", nil),
		"HEAD by the loopback address": newBoundaryRequest(http.MethodHead, "127.0.0.1:8080", "/api/v0/graph", nil),
		"same-site GET of the api": newBoundaryRequest(http.MethodGet, "127.0.0.1:8080", "/api/v0/graph",
			map[string]string{"Sec-Fetch-Site": "same-site"}),
		"POST from the origin with the implied port 80": newBoundaryRequest(http.MethodPost, "oraculum.example",
			"/api/v0/assertions", map[string]string{"Origin": "http://oraculum.example"}),
		// 別のサイトのリンクから画面を開ける。
		"cross-site navigation to the screen": newBoundaryRequest(http.MethodGet, "127.0.0.1:8080", "/",
			map[string]string{"Sec-Fetch-Site": "cross-site"}),
	} {
		t.Run(name, func(t *testing.T) {
			reached, recorder := boundaryRequest(t, boundaryHosts, request)
			if !reached || recorder.Code != http.StatusNoContent {
				t.Fatalf("reached=%v status=%d body=%s", reached, recorder.Code, recorder.Body)
			}
		})
	}
}

func TestBoundaryRejectsRequestsFromOtherHostsAndOrigins(t *testing.T) {
	for name, request := range map[string]*http.Request{
		// DNS rebinding で攻撃者の名前から届く要求である。
		"GET by another host name": newBoundaryRequest(http.MethodGet, "attacker.example:8080",
			"/api/v0/graph", nil),
		"GET by another port": newBoundaryRequest(http.MethodGet, "127.0.0.1:9090", "/api/v0/graph", nil),
		"GET by the allowed name on another port": newBoundaryRequest(http.MethodGet, "oraculum.example:8080",
			"/api/v0/graph", nil),
		"GET without Host":         newBoundaryRequest(http.MethodGet, "", "/api/v0/graph", nil),
		"GET by an unbracketed v6": newBoundaryRequest(http.MethodGet, "::1", "/api/v0/graph", nil),
		"POST from another origin": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080", "/api/v0/assertions",
			map[string]string{"Origin": "http://attacker.example"}),
		"PUT from another origin": newBoundaryRequest(http.MethodPut, "127.0.0.1:8080",
			"/api/v0/assertions/a", map[string]string{"Origin": "http://attacker.example:8080"}),
		"POST from an https origin": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080",
			"/api/v0/assertions", map[string]string{"Origin": "https://127.0.0.1:8080"}),
		"POST from a sandboxed frame": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080",
			"/api/v0/assertions", map[string]string{"Origin": "null"}),
		"cross-site GET of the api": newBoundaryRequest(http.MethodGet, "127.0.0.1:8080", "/api/v0/raw-texts",
			map[string]string{"Sec-Fetch-Site": "cross-site"}),
		"POST with an empty Origin": newBoundaryRequest(http.MethodPost, "127.0.0.1:8080", "/api/v0/assertions",
			map[string]string{"Origin": ""}),
		"POST with two Origin headers": func() *http.Request {
			request := newBoundaryRequest(http.MethodPost, "127.0.0.1:8080", "/api/v0/assertions", nil)
			request.Header.Add("Origin", "http://127.0.0.1:8080")
			request.Header.Add("Origin", "http://attacker.example")
			return request
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			reached, recorder := boundaryRequest(t, boundaryHosts, request)
			if reached || recorder.Code != http.StatusForbidden {
				t.Fatalf("reached=%v status=%d", reached, recorder.Code)
			}
			var body core.ApiError
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != core.ApiErrorCodeRequestOriginRejected {
				t.Fatalf("code=%q", body.Code)
			}
		})
	}
}

func TestBoundaryRejectsUnreadableAllowedHosts(t *testing.T) {
	for _, host := range []string{"", "::1", ":8080", "[::1", "a]", "h:", "http://h:8080", "h/path:8080",
		"user@h:8080"} {
		if _, err := api.NewBoundaryHandler(http.NotFoundHandler(), []string{host}); err == nil {
			t.Errorf("allowed host %q accepted", host)
		}
		if err := api.CheckAllowedHost(host); err == nil {
			t.Errorf("CheckAllowedHost(%q) accepted", host)
		}
	}
	for _, host := range []string{"h", "h:8080", "[::1]", "[::1]:8080", "192.0.2.10:8080"} {
		if err := api.CheckAllowedHost(host); err != nil {
			t.Errorf("CheckAllowedHost(%q): %v", host, err)
		}
	}
}
