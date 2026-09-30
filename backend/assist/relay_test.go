package assist_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Intellectual-Chasen/Oraculum/backend/assist"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const testConversation = "0123456789abcdef0123456789abcdef"

// fakeServer は Oraculum server の会話の endpoint の代わりに答え、受けた要求を記録する。
type fakeServer struct {
	mu       sync.Mutex
	requests []string
	bodies   map[string]map[string]any
	// origins は、要求ごとに受けた Origin の header である。
	origins map[string]string
	opened  int
	// sessions が nil でなければ、登録された有効な cookie だけを受け付ける。
	sessions map[string]bool
	// overviewDelay は、調査の概要の要求に答えるまで待つ時間である。
	overviewDelay time.Duration
}

// otherConversation は、偽の server が 2 つ目に発行する会話の識別子である。
const otherConversation = "fedcba9876543210fedcba9876543210"

func (s *fakeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v0/session/me", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"authentication":"none"}`)
	})
	mux.HandleFunc("POST /api/v0/conversations", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		s.mu.Lock()
		id := testConversation
		if s.opened > 0 {
			id = otherConversation
		}
		s.opened++
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"conversation":{"id":"`+id+
			`","provider":"claude","permissionRevision":1,"createdAt":"2030-01-02T03:04:05.000Z"}}`)
	})
	mux.HandleFunc("POST /api/v0/conversations/{id}/{operation}", func(w http.ResponseWriter, r *http.Request) {
		body := s.record(r)
		switch r.PathValue("operation") {
		case "turns":
			_, _ = io.WriteString(w, `{"turnId":"`+body["turnId"].(string)+`","records":[{"ref":"r1"}]}`)
		case "overview":
			time.Sleep(s.overviewDelay)
			_, _ = io.WriteString(w, `{"sources":[{"sourceId":"src:1","fileName":"synthetic.log"}]}`)
		case "graph-search":
			_, _ = io.WriteString(w, `{"matchedNodeCount":0,"nodes":[]}`)
		case "search-query-checks":
			query, _ := body["searchQuery"].(map[string]any)
			if depth, _ := query["depth"].(float64); depth > 8 {
				_, _ = io.WriteString(w, `{"accepted":false,"reason":"depth must be between 0 and 8"}`)
				return
			}
			// n1 は発行した参照、n7 は種別の壊れた起点を返す参照、n5 は起点を返さない参照、n3 は検証が
			// 失敗する参照、それ以外は発行していない参照である。
			switch origins, _ := query["nodeIds"].([]any); {
			case len(origins) == 0:
				_, _ = io.WriteString(w, `{"accepted":true}`)
			case origins[0] == "n1":
				_, _ = io.WriteString(w, `{"accepted":true,"origins":[{"id":"node:1","kind":"process","label":"a.exe"}]}`)
			case origins[0] == "n7":
				_, _ = io.WriteString(w, `{"accepted":true,"origins":[{"id":"node:7","kind":"planet","label":""}]}`)
			case origins[0] == "n5":
				_, _ = io.WriteString(w, `{"accepted":true}`)
			case origins[0] == "n3":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"code":"internal_error","message":"the check failed"}`)
			default:
				_, _ = io.WriteString(w, `{"accepted":false,"reason":"the reference is not issued"}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/v0/sources", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		_, _ = io.WriteString(w, `{"sources":[]}`)
	})
	mux.HandleFunc("POST /api/v0/assertions", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		_, _ = io.WriteString(w, `{}`)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		cookie, err := r.Cookie("oraculum_session")
		allowed := s.sessions == nil || (err == nil && s.sessions[cookie.Value])
		s.mu.Unlock()
		if !allowed {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"authentication_required","message":"the session is not valid"}`)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *fakeServer) record(r *http.Request) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	s.requests = append(s.requests, key)
	s.bodies[key] = body
	s.origins[key] = r.Header.Get("Origin")
	return body
}

// fakeProvider は発言ごとに MCP の tool を呼び、結果を応答の文にする偽の提供者である。
type fakeProvider struct {
	mu      sync.Mutex
	prompts []string
	tools   []*mcp.Tool
	// beforeTools は、発言の登録後から tool を読む前までに起きる変更を test で起こす。
	beforeTools func()
}

func (p *fakeProvider) Key() core.AssistProvider { return core.AssistProviderClaude }

func (p *fakeProvider) Open(_ context.Context, launch assist.ProviderLaunch) (assist.ProviderSession, error) {
	return &fakeSession{provider: p, launch: launch}, nil
}

type fakeSession struct {
	provider *fakeProvider
	launch   assist.ProviderLaunch
}

// bearer は要求へ bearer secret を付ける。
type bearer struct {
	secret string
}

func (b bearer) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(request)
}

func (s *fakeSession) Send(ctx context.Context, text string, emit func(core.AssistEvent) error) error {
	if s.provider.beforeTools != nil {
		s.provider.beforeTools()
	}
	s.provider.mu.Lock()
	s.provider.prompts = append(s.provider.prompts, text)
	s.provider.mu.Unlock()
	client := mcp.NewClient(&mcp.Implementation{Name: "fake-provider", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: s.launch.MCPURL, HTTPClient: &http.Client{Transport: bearer{secret: s.launch.MCPSecret}},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	s.provider.mu.Lock()
	s.provider.tools = listed.Tools
	s.provider.mu.Unlock()
	overview, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "overview", Arguments: map[string]any{}})
	if err != nil {
		return err
	}
	rejected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "show_search_query", Arguments: map[string]any{
		"searchQuery": map[string]any{"depth": 99}, "explanation": "深すぎる条件",
	}})
	if err != nil {
		return err
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "show_search_query", Arguments: map[string]any{
		"searchQuery": map[string]any{"depth": 1, "nodeKinds": []string{"process"}}, "explanation": "親のプロセスを辿る",
	}}); err != nil {
		return err
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "graph_search", Arguments: map[string]any{
		"searchQuery": map[string]any{"depth": 1, "nodeKinds": []string{"process"}},
	}}); err != nil {
		return err
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "graph_search", Arguments: map[string]any{
		"searchQuery": map[string]any{"depth": 1, "nodeKinds": []string{"process"}, "granularity": "record"},
	}}); err != nil {
		return err
	}
	answer := textOf(overview)
	if rejected.IsError {
		answer += " rejected: " + textOf(rejected)
	}
	return emit(core.AssistEvent{Kind: core.AssistEventKindText, Text: answer})
}

func (s *fakeSession) Close() error { return nil }

func textOf(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "")
}

// silentProvider は、止められるまで応答を返さない提供者である。出力を返さない CLI の代わりに使う。
// **Send は ctx を見ない。** 実際の CLI の adapter も、出力を待つ間は ctx を見られない。
type silentProvider struct {
	mu       sync.Mutex
	launches []assist.ProviderLaunch
}

func (p *silentProvider) Key() core.AssistProvider { return core.AssistProviderClaude }

func (p *silentProvider) Open(_ context.Context, launch assist.ProviderLaunch) (assist.ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.launches = append(p.launches, launch)
	return &silentSession{stopped: make(chan struct{})}, nil
}

func (p *silentProvider) launch(index int) assist.ProviderLaunch {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.launches[index]
}

type silentSession struct {
	once    sync.Once
	stopped chan struct{}
}

func (s *silentSession) Send(context.Context, string, func(core.AssistEvent) error) error {
	<-s.stopped
	return errors.New("the provider was stopped")
}

func (s *silentSession) Close() error {
	s.once.Do(func() { close(s.stopped) })
	return nil
}

// relayUnderTest は、偽の server と偽の提供者で組んだ中継と、動いている MCP の待ち受けである。
type relayUnderTest struct {
	relay    *assist.Relay
	server   *fakeServer
	provider *fakeProvider
	browser  http.Handler
	host     string
	mcpURL   string
	// serverOrigin は偽の server の origin である。
	serverOrigin string
}

func startRelay(t *testing.T) relayUnderTest {
	t.Helper()
	provider := &fakeProvider{}
	relay := startRelayWith(t, provider, 0)
	relay.provider = provider
	return relay
}

// startRelayWith は、provider を登録し、発言の上限を turnLimit にした中継を動かす。
func startRelayWith(t *testing.T, provider assist.Provider, turnLimit time.Duration) relayUnderTest {
	t.Helper()
	server := &fakeServer{bodies: map[string]map[string]any{}, origins: map[string]string{}}
	return startRelayForServer(t, provider, turnLimit, server)
}

func startRelayForServer(t *testing.T, provider assist.Provider, turnLimit time.Duration, server *fakeServer) relayUnderTest {
	t.Helper()
	upstream := httptest.NewServer(server.handler())
	t.Cleanup(upstream.Close)
	origin, _ := url.Parse(upstream.URL)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay, err := assist.New(assist.Config{
		Server: origin, Providers: map[core.AssistProvider]assist.Provider{core.AssistProviderClaude: provider},
		BrowserPort: testBrowserPort, MCPPort: listener.Addr().(*net.TCPAddr).Port, TempDir: t.TempDir(),
		TurnLimit: turnLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	mcpServer := &http.Server{Handler: relay.MCPHandler(), ReadHeaderTimeout: time.Second}
	go func() { _ = mcpServer.Serve(listener) }()
	t.Cleanup(func() {
		_ = mcpServer.Close()
		_ = relay.Close()
	})
	return relayUnderTest{relay: relay, server: server, browser: relay.BrowserHandler(), host: ownHost,
		mcpURL: relay.MCPURL(), serverOrigin: upstream.URL}
}

func (r relayUnderTest) send(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	return r.sendWithSession(t, method, target, body, "")
}

func (r relayUnderTest) sendWithSession(t *testing.T, method, target, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.AddCookie(&http.Cookie{Name: "oraculum_session", Value: token})
	}
	request.Host = r.host
	if method != http.MethodGet {
		request.Header.Set("Origin", "http://"+r.host)
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	r.browser.ServeHTTP(recorder, request)
	return recorder
}

// ログインの cookie は会話ごとに保持し、他のセッションへ会話を渡さず、提供者にも渡さない。
func TestRelayKeepsTheBrowserSessionForEachConversation(t *testing.T) {
	server := &fakeServer{
		bodies: map[string]map[string]any{}, origins: map[string]string{},
		sessions: map[string]bool{"session-a": true, "session-b": true},
	}
	provider := &fakeProvider{}
	relay := startRelayForServer(t, provider, 0, server)
	opened := relay.sendWithSession(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`, "session-a")
	if opened.Code != http.StatusCreated {
		t.Fatalf("opening with a valid session = %d %s", opened.Code, opened.Body)
	}
	turnPath := "/assist/conversations/" + testConversation + "/turns"
	turnBody := `{"turnId":"signed-in-turn","text":"概要を読む","context":{"matchConditions":[]}}`
	turn := relay.sendWithSession(t, http.MethodPost, turnPath, turnBody, "session-a")
	if turn.Code != http.StatusOK {
		t.Fatalf("turn = %d %s", turn.Code, turn.Body)
	}
	events := readEvents(t, turn.Body)
	if len(events) == 0 || events[len(events)-1].Kind != core.AssistEventKindTurnEnd {
		t.Fatalf("events = %+v, want a completed turn", events)
	}
	if !slices.ContainsFunc(events, func(event core.AssistEvent) bool {
		return event.Kind == core.AssistEventKindText && strings.Contains(event.Text, "synthetic.log")
	}) {
		t.Fatalf("events = %+v, want the authenticated MCP result", events)
	}
	provider.mu.Lock()
	prompts := slices.Clone(provider.prompts)
	provider.mu.Unlock()
	if strings.Contains(strings.Join(prompts, ""), "session-a") {
		t.Fatal("the provider received the server session")
	}
	other := relay.sendWithSession(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`, "session-b")
	if other.Code != http.StatusCreated {
		t.Fatalf("opening another session = %d %s", other.Code, other.Body)
	}
	for token, id := range map[string]string{"session-a": testConversation, "session-b": otherConversation} {
		listed := relay.sendWithSession(t, http.MethodGet, "/assist/conversations", "", token)
		var body struct {
			Conversations []struct{ Id string }
		}
		if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if listed.Code != http.StatusOK || len(body.Conversations) != 1 || body.Conversations[0].Id != id {
			t.Fatalf("conversation listing = %d %s, want only %s", listed.Code, listed.Body, id)
		}
	}
	eventsPath := "/assist/conversations/" + testConversation + "/events"
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, eventsPath, ""}, {http.MethodPost, turnPath, turnBody},
	} {
		response := relay.sendWithSession(t, request.method, request.path, request.body, "session-b")
		if response.Code != http.StatusNotFound {
			t.Fatalf("another session's conversation = %d %s", response.Code, response.Body)
		}
	}
	server.mu.Lock()
	server.sessions["session-a"] = false
	server.mu.Unlock()
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/assist/conversations", ""}, {http.MethodGet, eventsPath, ""},
		{http.MethodPost, turnPath, turnBody},
	} {
		response := relay.sendWithSession(t, request.method, request.path, request.body, "session-a")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("an invalidated session = %d %s", response.Code, response.Body)
		}
	}
}

// 応答の途中で server のセッションが失効すると、以後の MCP の読み取りと条件表示を退ける。
func TestRelayToolsRejectARevokedServerSession(t *testing.T) {
	server := &fakeServer{
		bodies: map[string]map[string]any{}, origins: map[string]string{},
		sessions: map[string]bool{"session-a": true},
	}
	provider := &fakeProvider{beforeTools: func() {
		server.mu.Lock()
		server.sessions["session-a"] = false
		server.mu.Unlock()
	}}
	relay := startRelayForServer(t, provider, 0, server)
	opened := relay.sendWithSession(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`, "session-a")
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body)
	}
	turn := relay.sendWithSession(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns",
		turnBody("turn-1"), "session-a")
	if turn.Code != http.StatusOK {
		t.Fatalf("turn = %d %s", turn.Code, turn.Body)
	}
	events := readEvents(t, turn.Body)
	if !slices.ContainsFunc(events, func(event core.AssistEvent) bool {
		return event.Kind == core.AssistEventKindText && strings.Contains(event.Text, "authentication_required")
	}) {
		t.Fatalf("events = %+v, want the session failure from MCP", events)
	}
	if slices.ContainsFunc(events, func(event core.AssistEvent) bool {
		return event.Kind == core.AssistEventKindSearchQueryCard
	}) {
		t.Fatal("the invalidated session displayed a search card")
	}
}

func readEvents(t *testing.T, body io.Reader) []core.AssistEvent {
	t.Helper()
	var events []core.AssistEvent
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var event core.AssistEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("reading an event %q: %v", scanner.Text(), err)
		}
		events = append(events, event)
	}
	return events
}

// 発言は server へ文脈を登録してから提供者へ送り、提供者の tool の呼び出しは server の会話の
// endpoint へ届く。応答の event は NDJSON で流れ、検証を通った検索の条件だけが card になる。
func TestRelayAnswersAMessageThroughTheTools(t *testing.T) {
	relay := startRelay(t)
	status := relay.send(t, http.MethodGet, "/assist/status", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"claude"`) {
		t.Fatalf("status = %d %s", status.Code, status.Body)
	}
	opened := relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	if opened.Code != http.StatusCreated || !strings.Contains(opened.Body.String(), testConversation) {
		t.Fatalf("open = %d %s", opened.Code, opened.Body)
	}
	turn := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns", `{
		"turnId":"turn-1","text":"親のプロセスを調べたい",
		"context":{"matchConditions":[{"conditionKey":"destination_ip","tolerance":0}],"searchQuery":{"depth":1},
		"nodeKindsFromConditions":true}}`)
	if turn.Code != http.StatusOK || turn.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("turn = %d %s", turn.Code, turn.Body)
	}
	events := readEvents(t, turn.Body)
	var kinds []core.AssistEventKind
	for _, event := range events {
		kinds = append(kinds, event.Kind)
		if event.TurnId != "turn-1" {
			t.Errorf("event %+v belongs to another turn", event)
		}
	}
	use, result := core.AssistEventKindToolUse, core.AssistEventKindToolResult
	wantKinds := []core.AssistEventKind{
		core.AssistEventKindUserMessage, use, result, use, result, use, core.AssistEventKindSearchQueryCard, result,
		use, result, use, result, core.AssistEventKindText, core.AssistEventKindTurnEnd,
	}
	if !slices.Equal(kinds, wantKinds) {
		t.Fatalf("event kinds = %v, want %v", kinds, wantKinds)
	}
	// 呼び出しは入力を持ち、結果は呼び出しの通番と LLM に返した本文を持つ。
	if events[1].ToolName != "overview" || events[2].ToolUseSequence != events[1].Sequence ||
		!strings.Contains(events[2].ToolResult, "synthetic.log") || events[2].ToolFailed {
		t.Errorf("overview call = %+v, result = %+v", events[1], events[2])
	}
	if !strings.Contains(string(events[3].ToolInput), `"depth":99`) || !events[4].ToolFailed ||
		!strings.Contains(events[4].ToolResult, "depth must be between") || events[4].SearchQuery != nil {
		t.Errorf("rejected call = %+v, result = %+v", events[3], events[4])
	}
	card := events[6]
	if card.SearchQuery == nil || card.SearchQuery.Depth != 1 || card.Explanation != "親のプロセスを辿る" ||
		len(card.MatchConditions) != 1 || card.MatchConditions[0].ConditionKey != core.ConditionKeyDestinationIp {
		t.Errorf("card = %+v", card)
	}
	// 画面の検索欄で表せるグラフの検索は、結果に適用できる条件を持つ。
	if events[8].ToolName != "graph_search" || events[9].SearchQuery == nil || events[9].SearchQuery.Depth != 1 {
		t.Errorf("graph search call = %+v, result = %+v", events[8], events[9])
	}
	// 種別の組がレコードを含まないレコードの粒度は、画面の検索欄で表せない。
	if events[11].ToolFailed || events[11].SearchQuery != nil {
		t.Errorf("record granularity result = %+v, want no search query for the screen", events[11])
	}
	if !strings.Contains(events[12].Text, "synthetic.log") || !strings.Contains(events[12].Text, "depth must be between") {
		t.Errorf("answer = %q, want the overview body and the rejection reason", events[12].Text)
	}

	relay.server.mu.Lock()
	requests := slices.Clone(relay.server.requests)
	overview := relay.server.bodies["POST /api/v0/conversations/"+testConversation+"/overview"]
	registered := relay.server.bodies["POST /api/v0/conversations/"+testConversation+"/turns"]
	relay.server.mu.Unlock()
	wantRequests := []string{
		"POST /api/v0/conversations",
		"POST /api/v0/conversations/" + testConversation + "/turns",
		"POST /api/v0/conversations/" + testConversation + "/overview",
		"POST /api/v0/conversations/" + testConversation + "/search-query-checks",
		"POST /api/v0/conversations/" + testConversation + "/search-query-checks",
		"POST /api/v0/conversations/" + testConversation + "/graph-search",
		"POST /api/v0/conversations/" + testConversation + "/graph-search",
	}
	if !slices.Equal(requests, wantRequests) {
		t.Errorf("server requests = %v, want %v", requests, wantRequests)
	}
	if overview["turnId"] != "turn-1" {
		t.Errorf("overview body = %v, want the turn of the message", overview)
	}
	if registered["searchQuery"] == nil || registered["nodeKindsFromConditions"] != true {
		t.Errorf("turn registration = %v, want the search query and how the screen chose the node kinds", registered)
	}

	relay.provider.mu.Lock()
	prompt := relay.provider.prompts[0]
	var toolNames []string
	for _, tool := range relay.provider.tools {
		toolNames = append(toolNames, tool.Name)
	}
	relay.provider.mu.Unlock()
	if !strings.Contains(prompt, "親のプロセスを調べたい") || !strings.Contains(prompt, `"ref":"r1"`) {
		t.Errorf("prompt = %q, want the message and the registered context", prompt)
	}
	slices.Sort(toolNames)
	wantTools := []string{"graph_search", "overview", "record_text", "show_search_query", "target_detail", "timeline"}
	if !slices.Equal(toolNames, wantTools) {
		t.Errorf("tools = %v, want %v", toolNames, wantTools)
	}
	// 検索の条件の schema は、根拠のレコードを絞る条件の項目も同じ階層に持つ。
	for _, tool := range relay.provider.tools {
		if tool.Name != "graph_search" {
			continue
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		for _, property := range []string{`"depth"`, `"nodeKinds"`, `"eventCategory"`, `"timeFrom"`, `"filterUnit"`} {
			if !strings.Contains(string(schema), property) {
				t.Errorf("graph_search schema lacks %s: %s", property, schema)
			}
		}
	}
	// LLM は項目の書き方と取れる値を schema の description から読む。
	for _, tool := range relay.provider.tools {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Properties map[string]struct {
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema, &parsed); err != nil {
			t.Fatal(err)
		}
		for input, inner := range parsed.Properties {
			for name, property := range inner.Properties {
				if property.Description == "" {
					t.Errorf("%s: %s.%s has no description", tool.Name, input, name)
				}
			}
		}
	}

	// 同じ発言の 2 回目を退け、再読み込みした画面は会話の event を読み直せる。
	repeated := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns",
		`{"turnId":"turn-1","text":"again","context":{"matchConditions":[{"conditionKey":"destination_ip","tolerance":0}]}}`)
	if repeated.Code != http.StatusConflict {
		t.Errorf("repeated turn = %d, want 409", repeated.Code)
	}
	reloaded := relay.send(t, http.MethodGet, "/assist/conversations/"+testConversation+"/events?after=0", "")
	var listed struct {
		Events    []core.AssistEvent `json:"events"`
		Answering bool               `json:"answering"`
	}
	if err := json.Unmarshal(reloaded.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Events) != len(events) || listed.Answering {
		t.Errorf("reloaded = %+v, want the %d events and no answer in progress", listed, len(events))
	}
}

// 画面と `/api/v0/` の要求を server へそのまま転送する。
func TestRelayForwardsTheScreenRequests(t *testing.T) {
	relay := startRelay(t)
	response := relay.send(t, http.MethodGet, "/api/v0/sources", "")
	if response.Code != http.StatusOK || response.Body.String() != `{"sources":[]}` {
		t.Errorf("forwarded = %d %s", response.Code, response.Body)
	}
}

// 状態を変える画面の要求は、Origin を server の origin に置き換えて転送する。server は、自分の
// origin の外から届いた状態を変える要求を退ける。
func TestRelayForwardsTheScreenChangesFromTheServerOrigin(t *testing.T) {
	relay := startRelay(t)
	response := relay.send(t, http.MethodPost, "/api/v0/assertions", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("forwarded = %d %s", response.Code, response.Body)
	}
	relay.server.mu.Lock()
	defer relay.server.mu.Unlock()
	if got := relay.server.origins["POST /api/v0/assertions"]; got != relay.serverOrigin {
		t.Errorf("Origin = %q, want %q", got, relay.serverOrigin)
	}
}

// 登録していない会話と、提供者の登録の無い会話の発行を退ける。
func TestRelayRejectsUnknownConversations(t *testing.T) {
	relay := startRelay(t)
	if response := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns",
		`{"turnId":"turn-1","text":"x","context":{"matchConditions":[]}}`); response.Code != http.StatusNotFound {
		t.Errorf("unknown conversation = %d, want 404", response.Code)
	}
	if response := relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"other"}`); response.Code !=
		http.StatusBadRequest {
		t.Errorf("unknown provider = %d, want 400", response.Code)
	}
}

func turnBody(turnId string) string {
	return `{"turnId":"` + turnId + `","text":"調べて",` +
		`"context":{"matchConditions":[{"conditionKey":"destination_ip","tolerance":0}]}}`
}

// startTurn は発言を別の goroutine で送り、応答の途中になるまで待つ。返す channel は、発言の応答の
// stream が終わると応答を 1 つ渡す。
func startTurn(t *testing.T, relay relayUnderTest, conversation, turnId string) <-chan *httptest.ResponseRecorder {
	t.Helper()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- relay.send(t, http.MethodPost, "/assist/conversations/"+conversation+"/turns", turnBody(turnId))
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		listed := relay.send(t, http.MethodGet, "/assist/conversations/"+conversation+"/events", "")
		if strings.Contains(listed.Body.String(), `"answering":true`) {
			return done
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the message did not reach the provider")
	return nil
}

// callTool は bearer secret の会話の MCP の tool を、空の入力で 1 回呼ぶ。
func callTool(t *testing.T, relay relayUnderTest, secret, name string) (*mcp.CallToolResult, error) {
	t.Helper()
	return callToolWith(t, relay, secret, name, map[string]any{})
}

// callToolWith は bearer secret の会話の MCP の tool を、入力を与えて 1 回呼ぶ。
func callToolWith(
	t *testing.T, relay relayUnderTest, secret, name string, arguments map[string]any,
) (*mcp.CallToolResult, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: relay.mcpURL, HTTPClient: &http.Client{Transport: bearer{secret: secret}}, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	return session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
}

// 起点を持つ検索の条件は、server が解決したノードの識別子と起点を持つ event として画面へ渡す。
// 解決できない起点の条件は画面へ渡さず、tool の結果だけを記録する。
func TestRelayResolvesTheOriginsOfAppliableSearches(t *testing.T) {
	provider := &silentProvider{}
	server := &fakeServer{bodies: map[string]map[string]any{}, origins: map[string]string{}}
	relay := startRelayForServer(t, provider, 2*time.Second, server)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	turn := startTurn(t, relay, testConversation, "turn-1")
	secret := provider.launch(0).MCPSecret
	search := func(origin string) map[string]any {
		return map[string]any{"searchQuery": map[string]any{"depth": 2, "nodeIds": []string{origin}}}
	}
	for _, origin := range []string{"n1", "n9", "n7", "n5", "n3"} {
		if result, err := callToolWith(t, relay, secret, "graph_search", search(origin)); err != nil || result.IsError {
			t.Fatalf("graph_search from %s = %+v, %v", origin, result, err)
		}
	}
	card := search("n1")
	card["explanation"] = "起点から辿る"
	if result, err := callToolWith(t, relay, secret, "show_search_query", card); err != nil || result.IsError {
		t.Fatalf("show_search_query = %+v, %v", result, err)
	}
	rejected := search("n9")
	rejected["explanation"] = "発行していない起点"
	if result, err := callToolWith(t, relay, secret, "show_search_query", rejected); err != nil || !result.IsError {
		t.Errorf("show_search_query from an unissued reference = %+v, %v, want a refusal", result, err)
	}
	// 黙った提供者の応答は上限の時間で終わる。
	<-turn

	response := relay.send(t, http.MethodGet, "/assist/conversations/"+testConversation+"/events?after=0", "")
	var body struct {
		Events []core.AssistEvent `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var results, cards []core.AssistEvent
	for _, event := range body.Events {
		switch {
		case event.Kind == core.AssistEventKindToolResult && event.ToolName == "graph_search":
			results = append(results, event)
		case event.Kind == core.AssistEventKindSearchQueryCard:
			cards = append(cards, event)
		}
	}
	if len(results) != 5 || len(cards) != 1 {
		t.Fatalf("events = %+v, want five graph_search results and one card", body.Events)
	}
	resolved := func(name string, event core.AssistEvent) {
		t.Helper()
		if event.SearchQuery == nil || !slices.Equal(event.SearchQuery.NodeIds, []string{"node:1"}) ||
			len(event.Origins) != 1 || event.Origins[0].Label != "a.exe" || len(event.MatchConditions) == 0 {
			t.Errorf("%s = %+v, want the resolved origin with the match conditions", name, event)
		}
	}
	resolved("graph_search from n1", results[0])
	resolved("card from n1", cards[0])
	for index, name := range map[int]string{
		1: "unissued n9", 2: "broken n7", 3: "n5 without origins", 4: "n3 of a failed check",
	} {
		if results[index].SearchQuery != nil || results[index].Origins != nil || results[index].ToolFailed {
			t.Errorf("graph_search from %s = %+v, want the result without the conditions", name, results[index])
		}
	}
}

// tool の呼び出しは応答の途中の発言にだけ記録する。知らない tool の呼び出しと、上限の時間で応答が
// 終わった後に返った結果も、呼び出しを記録した発言に記録する。
func TestRelayRecordsToolCallsOfTheAnsweringMessage(t *testing.T) {
	provider := &silentProvider{}
	server := &fakeServer{bodies: map[string]map[string]any{}, origins: map[string]string{},
		overviewDelay: 2 * time.Second}
	relay := startRelayForServer(t, provider, time.Second, server)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	turn := startTurn(t, relay, otherConversation, "turn-1")

	if idle, err := callTool(t, relay, provider.launch(0).MCPSecret, "overview"); err != nil || !idle.IsError {
		t.Errorf("tool of the idle conversation = %+v, %v, want a refusal", idle, err)
	}
	unknown, err := callTool(t, relay, provider.launch(1).MCPSecret, "delete_evidence")
	if err == nil && !unknown.IsError {
		t.Errorf("unknown tool = %+v, want a failure", unknown)
	}
	if _, err := callTool(t, relay, provider.launch(1).MCPSecret, "overview"); err != nil {
		t.Fatal(err)
	}
	<-turn

	listed := func(conversation string) []core.AssistEvent {
		t.Helper()
		response := relay.send(t, http.MethodGet, "/assist/conversations/"+conversation+"/events?after=0", "")
		var body struct {
			Events []core.AssistEvent `json:"events"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Events
	}
	if idle := listed(testConversation); len(idle) != 0 {
		t.Errorf("idle conversation events = %+v, want none", idle)
	}
	events := listed(otherConversation)
	find := func(kind core.AssistEventKind, name string) core.AssistEvent {
		t.Helper()
		index := slices.IndexFunc(events, func(event core.AssistEvent) bool {
			return event.Kind == kind && event.ToolName == name
		})
		if index < 0 {
			t.Fatalf("events = %+v, want %s of %s", events, kind, name)
		}
		return events[index]
	}
	unknownUse, unknownResult := find(core.AssistEventKindToolUse, "delete_evidence"),
		find(core.AssistEventKindToolResult, "delete_evidence")
	if unknownResult.ToolUseSequence != unknownUse.Sequence || !unknownResult.ToolFailed {
		t.Errorf("unknown tool call = %+v, result = %+v, want a failed result", unknownUse, unknownResult)
	}
	overviewUse, overviewResult := find(core.AssistEventKindToolUse, "overview"),
		find(core.AssistEventKindToolResult, "overview")
	ended := slices.IndexFunc(events, func(event core.AssistEvent) bool {
		return event.Kind == core.AssistEventKindProviderError
	})
	if ended < 0 || overviewResult.Sequence < events[ended].Sequence {
		t.Fatalf("events = %+v, want the overview result after the end of the answer", events)
	}
	if overviewResult.ToolUseSequence != overviewUse.Sequence || overviewResult.TurnId != "turn-1" ||
		overviewResult.ToolFailed || !strings.Contains(overviewResult.ToolResult, "synthetic.log") {
		t.Errorf("overview result = %+v, want the body in the turn of the call", overviewResult)
	}
}

// 応答の途中で中継を止めると、出力を返さない提供者を止めて有限の時間で戻り、発言は提供者の失敗で終わる。
func TestRelayCloseStopsASilentProvider(t *testing.T) {
	relay := startRelayWith(t, &silentProvider{}, 0)
	if opened := relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`); opened.Code !=
		http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body)
	}
	turn := startTurn(t, relay, testConversation, "turn-1")
	closed := make(chan error, 1)
	go func() { closed <- relay.relay.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return while the provider was silent")
	}
	events := readEvents(t, (<-turn).Body)
	if last := events[len(events)-1]; last.Kind != core.AssistEventKindProviderError {
		t.Errorf("last event = %+v, want the provider failure", last)
	}
	if again := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns",
		turnBody("turn-2")); again.Code != http.StatusServiceUnavailable && again.Code != http.StatusConflict {
		t.Errorf("turn after Close = %d, want the relay to refuse it", again.Code)
	}
}

// 上限の時間を超えた応答は提供者を止めて終わり、止めた会話は以後の発言を退ける。
func TestRelayStopsAProviderPastTheTurnLimit(t *testing.T) {
	relay := startRelayWith(t, &silentProvider{}, 50*time.Millisecond)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	turn := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns", turnBody("turn-1"))
	events := readEvents(t, turn.Body)
	last := events[len(events)-1]
	if last.Kind != core.AssistEventKindProviderError || !strings.Contains(last.Text, "上限の時間") {
		t.Errorf("last event = %+v, want the provider failure at the time limit", last)
	}
	again := relay.send(t, http.MethodPost, "/assist/conversations/"+testConversation+"/turns", turnBody("turn-2"))
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), "open a new conversation") {
		t.Errorf("turn after the limit = %d %s, want 409 asking for a new conversation", again.Code, again.Body)
	}
}

// 会話の MCP の secret は、その会話の tool だけに届く。tool は会話の識別子で server を呼ぶ。
func TestRelayToolsBelongToTheConversationOfTheSecret(t *testing.T) {
	provider := &silentProvider{}
	relay := startRelayWith(t, provider, 0)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	relay.send(t, http.MethodPost, "/assist/conversations", `{"provider":"claude"}`)
	turn := startTurn(t, relay, otherConversation, "turn-1")
	call := func(secret string) *mcp.CallToolResult {
		t.Helper()
		result, err := callTool(t, relay, secret, "overview")
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	// 1 つ目の会話は応答の途中でないため、その secret の tool は server を呼ばない。
	if idle := call(provider.launch(0).MCPSecret); !idle.IsError {
		t.Errorf("tool of the idle conversation = %s, want a refusal", textOf(idle))
	}
	if answering := call(provider.launch(1).MCPSecret); answering.IsError {
		t.Errorf("tool of the answering conversation = %s", textOf(answering))
	}
	relay.server.mu.Lock()
	requests := slices.Clone(relay.server.requests)
	relay.server.mu.Unlock()
	for _, request := range requests {
		if strings.Contains(request, testConversation+"/") {
			t.Errorf("the idle conversation reached the server: %s", request)
		}
	}
	if !slices.Contains(requests, "POST /api/v0/conversations/"+otherConversation+"/overview") {
		t.Errorf("server requests = %v, want the overview of the answering conversation", requests)
	}
	if err := relay.relay.Close(); err != nil {
		t.Error(err)
	}
	<-turn
}
