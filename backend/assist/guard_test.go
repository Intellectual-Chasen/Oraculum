package assist_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/assist"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	testBrowserPort = 49152
	testMCPPort     = 49153
)

// newGuardedRelay は、server の代わりに受けた要求の path を記録する転送先を持つ中継を返す。
func newGuardedRelay(t *testing.T) (*assist.Relay, *[]string) {
	t.Helper()
	var forwarded []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = append(forwarded, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	server, _ := url.Parse(upstream.URL)
	relay, err := assist.New(assist.Config{
		Server: server, Providers: map[core.AssistProvider]assist.Provider{core.AssistProviderClaude: nil},
		BrowserPort: testBrowserPort, MCPPort: testMCPPort, TempDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	return relay, &forwarded
}

type browserRequest struct {
	name, method, target, host string
	headers                    map[string]string
	body                       string
	wantStatus                 int
}

func (c browserRequest) send(handler http.Handler) *httptest.ResponseRecorder {
	var body *strings.Reader
	if c.body != "" {
		body = strings.NewReader(c.body)
	} else {
		body = strings.NewReader("")
	}
	request := httptest.NewRequest(c.method, c.target, body)
	if c.body == "" {
		request.ContentLength = 0
	}
	request.Host = c.host
	for key, value := range c.headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

const ownHost = "127.0.0.1:49152"

// browser からの要求の検査の各条件を、通る要求と退ける要求の両側で確かめる。
func TestBrowserGuard(t *testing.T) {
	relay, forwarded := newGuardedRelay(t)
	handler := relay.BrowserHandler()
	own := map[string]string{"Origin": "http://127.0.0.1:49152"}
	ownJSON := map[string]string{"Origin": "http://127.0.0.1:49152", "Content-Type": "application/json"}
	for _, c := range []browserRequest{
		{"page from the relay host", http.MethodGet, "/", ownHost, nil, "", http.StatusOK},
		{"page from localhost", http.MethodGet, "/", "localhost:49152", nil, "", http.StatusOK},
		{"rebinding host", http.MethodGet, "/", "evil.example.test:49152", nil, "", http.StatusForbidden},
		{"another port", http.MethodGet, "/", "127.0.0.1:1", nil, "", http.StatusForbidden},
		{"same-origin fetch", http.MethodGet, "/api/v0/sources", ownHost,
			map[string]string{"Sec-Fetch-Site": "same-origin"}, "", http.StatusOK},
		{"typed navigation", http.MethodGet, "/", ownHost, map[string]string{"Sec-Fetch-Site": "none"}, "",
			http.StatusOK},
		{"cross-site fetch", http.MethodGet, "/api/v0/sources", ownHost,
			map[string]string{"Sec-Fetch-Site": "cross-site"}, "", http.StatusForbidden},
		{"write from the relay", http.MethodPost, "/api/v0/assertions", ownHost, ownJSON, "{}", http.StatusOK},
		{"write without a body", http.MethodPost, "/api/v0/grouping-optimizations", ownHost, own, "",
			http.StatusOK},
		{"write from another origin", http.MethodPost, "/api/v0/assertions", ownHost,
			map[string]string{"Origin": "http://evil.example.test", "Content-Type": "application/json"}, "{}",
			http.StatusForbidden},
		{"write without an origin", http.MethodPost, "/api/v0/assertions", ownHost,
			map[string]string{"Content-Type": "application/json"}, "{}", http.StatusForbidden},
		{"form write", http.MethodPost, "/api/v0/assertions", ownHost,
			map[string]string{"Origin": "http://127.0.0.1:49152", "Content-Type": "text/plain"}, "{}",
			http.StatusUnsupportedMediaType},
		{"conversation endpoint", http.MethodPost, "/api/v0/conversations", ownHost, ownJSON, "{}",
			http.StatusForbidden},
		{"conversation read", http.MethodPost, "/api/v0/conversations/" + strings.Repeat("a", 32) + "/records",
			ownHost, ownJSON, "{}", http.StatusForbidden},
		{"encoded separator", http.MethodPost, "/api/v0%2Fconversations", ownHost, ownJSON, "{}",
			http.StatusBadRequest},
		{"lowercase encoded separator", http.MethodGet, "/api/v0%2fconversations", ownHost, nil, "",
			http.StatusBadRequest},
		{"encoded backslash", http.MethodGet, "/api/v0%5Cconversations", ownHost, nil, "",
			http.StatusBadRequest},
		{"encoded conversation name", http.MethodGet, "/api/v0/%63onversations", ownHost, nil, "",
			http.StatusForbidden},
		{"encoded dot segment", http.MethodGet, "/api/v0/x/%2e%2e/conversations", ownHost, nil, "",
			http.StatusBadRequest},
		{"dot segment", http.MethodPost, "/api/v0/x/../conversations", ownHost, ownJSON, "{}",
			http.StatusBadRequest},
		{"double separator", http.MethodPost, "/api/v0//conversations", ownHost, ownJSON, "{}",
			http.StatusBadRequest},
		{"absolute form", http.MethodGet, "http://127.0.0.1:49152/", ownHost, nil, "", http.StatusBadRequest},
	} {
		response := c.send(handler)
		if response.Code != c.wantStatus {
			t.Errorf("%s: status = %d, want %d (%s)", c.name, response.Code, c.wantStatus, response.Body)
		}
		if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
			t.Errorf("%s: Content-Security-Policy = %q", c.name, got)
		}
	}
	for _, path := range *forwarded {
		if strings.Contains(path, "conversations") {
			t.Errorf("the relay forwarded %s", path)
		}
	}
	if len(*forwarded) == 0 {
		t.Error("the relay forwarded nothing")
	}
}

// 画面が符号化した識別子を、転送先で同じ識別子として読む。
func TestBrowserGuardForwardsEncodedIdentifiers(t *testing.T) {
	relay, forwarded := newGuardedRelay(t)
	for _, decoded := range []string{
		"/api/v0/nodes/n:process:example",
		"/api/v0/nodes/n:process:example/value-counts",
		"/api/v0/edges/e:connection:example",
		"/api/v0/edges/e:connection:example/url-fragments",
		"/api/v0/terminals/端末 example",
		"/api/v0/members/analyst@example.test",
	} {
		t.Run(decoded, func(t *testing.T) {
			segments := strings.Split(decoded, "/")
			for index, segment := range segments {
				segments[index] = url.QueryEscape(segment)
			}
			// encodeURIComponent と同じく、空白を %20 として送る。
			target := strings.ReplaceAll(strings.Join(segments, "/"), "+", "%20")
			before := len(*forwarded)
			response := (browserRequest{method: http.MethodGet, target: target, host: ownHost}).send(relay.BrowserHandler())
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body)
			}
			if len(*forwarded) != before+1 || (*forwarded)[before] != "GET "+decoded {
				t.Fatalf("forwarded = %v, want GET %s", (*forwarded)[before:], decoded)
			}
		})
	}
}

// CONNECT を退ける。
func TestBrowserGuardRefusesConnect(t *testing.T) {
	relay, _ := newGuardedRelay(t)
	request := httptest.NewRequest(http.MethodConnect, "/", nil)
	request.Host = ownHost
	request.RequestURI = ownHost
	recorder := httptest.NewRecorder()
	relay.BrowserHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("CONNECT status = %d, want 400", recorder.Code)
	}
}

// MCP の待ち受けは、Host の不一致、browser からの要求、secret の無い要求と不一致の要求を退ける。
func TestMCPGuard(t *testing.T) {
	relay, _ := newGuardedRelay(t)
	handler := relay.MCPHandler()
	for _, c := range []browserRequest{
		{"another host", http.MethodPost, "/mcp", "evil.example.test:49153", nil, "{}", http.StatusForbidden},
		{"browser origin", http.MethodPost, "/mcp", "127.0.0.1:49153",
			map[string]string{"Origin": "http://127.0.0.1:49152"}, "{}", http.StatusForbidden},
		{"no secret", http.MethodPost, "/mcp", "127.0.0.1:49153", nil, "{}", http.StatusUnauthorized},
		{"wrong secret", http.MethodPost, "/mcp", "127.0.0.1:49153",
			map[string]string{"Authorization": "Bearer " + strings.Repeat("0", 64)}, "{}", http.StatusUnauthorized},
	} {
		if response := c.send(handler); response.Code != c.wantStatus {
			t.Errorf("%s: status = %d, want %d", c.name, response.Code, c.wantStatus)
		}
	}
}
