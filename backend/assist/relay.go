package assist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// Config は中継の組み立ての値である。
type Config struct {
	// Server は Oraculum server の origin である。画面と `/api/v0/` の転送先であり、会話の endpoint の
	// 呼び出し先である。
	Server *url.URL
	// Providers は提供者の識別から、その提供者を探す表である。
	Providers map[core.AssistProvider]Provider
	// BrowserPort と MCPPort は、中継が待ち受ける loopback の port である。
	BrowserPort int
	MCPPort     int
	// HTTPClient は server を呼ぶ client である。
	HTTPClient *http.Client
	// TempDir は会話ごとの一時 directory を作る親の directory である。空は OS の一時 directory。
	TempDir string
	// TurnLimit は発言 1 つへの応答を待つ上限である。0 は defaultTurnLimit を使う。
	TurnLimit time.Duration
}

// defaultTurnLimit は発言 1 つへの応答を待つ上限の既定値である。上限を超えた会話は、提供者を
// 止めて応答の途中の状態を外す。止めないと、出力を返さない提供者がその会話の以後の発言を塞ぎ続ける。
//
// 既知の制限: 発言 1 つの応答を 30 分までにする, 実機の応答時間は測っていない (偽の CLI だけで
// 確かめた)。tool を 10 回ほど呼ぶ調査の応答は数分で終わる見込みである, 実機の会話で上限に
// 達する応答が出たときに見直す
const defaultTurnLimit = 30 * time.Minute

// Relay は分析者の端末で動く AI 支援の中継である。
type Relay struct {
	config        Config
	client        ServerClient
	proxy         *httputil.ReverseProxy
	conversations *conversations
	// ctx は会話の提供者の process の寿命である。Close で取り消す。
	ctx    context.Context
	cancel context.CancelFunc
	// mu は closed と wait への Add を守る。Close が Wait を始めた後に応答を足さない。
	mu     sync.Mutex
	closed bool
	wait   sync.WaitGroup
}

// New は中継を組む。
func New(config Config) (*Relay, error) {
	if config.Server == nil || config.Server.Host == "" ||
		(config.Server.Scheme != "http" && config.Server.Scheme != "https") {
		return nil, errors.New("the Oraculum server must be an http or https origin")
	}
	if len(config.Providers) == 0 {
		return nil, errors.New("the relay needs one provider or more")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	if config.TurnLimit <= 0 {
		config.TurnLimit = defaultTurnLimit
	}
	target := config.Server
	serverOrigin := target.Scheme + "://" + target.Host
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			// **状態を変える画面の要求の Origin を server の origin に置き換える。** guardBrowser が、
			// その要求の Origin が中継であることを確かめた。server は、自分の origin の外から届いた
			// 状態を変える要求を退ける。
			if request.In.Header.Get("Origin") != "" {
				request.Out.Header.Set("Origin", serverOrigin)
			}
		},
		// **応答を溜めずに流す。** グラフの組み立て中の取得と、まとめ方の最適化は長く続く。
		FlushInterval: -1,
		Transport:     config.HTTPClient.Transport,
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Relay{
		config: config, client: NewServerClient(target, config.HTTPClient), proxy: proxy,
		conversations: newConversations(), ctx: ctx, cancel: cancel,
	}, nil
}

// MCPURL は、提供者へ渡す MCP の待ち受けの URL である。
func (r *Relay) MCPURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(r.config.MCPPort) + mcpPath
}

// BrowserHandler は browser の要求に答える handler を返す。
func (r *Relay) BrowserHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assist/status", r.status)
	mux.HandleFunc("GET /assist/conversations", r.listConversations)
	mux.HandleFunc("POST /assist/conversations", r.openConversation)
	mux.HandleFunc("GET /assist/conversations/{conversationId}/events", r.events)
	mux.HandleFunc("POST /assist/conversations/{conversationId}/turns", r.turn)
	mux.Handle("/assist/", http.NotFoundHandler())
	mux.Handle("/", r.proxy)
	return guardBrowser(r.config.BrowserPort, mux)
}

// Close は会話の提供者をすべて止め、会話ごとの一時 directory を消す。
//
// **提供者を止めてから応答の終わりを待つ。** 応答の途中の Send は提供者の出力を待っており、ctx の
// 取り消しを見られない。提供者を止めると出力が閉じ、Send が返る。
func (r *Relay) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.cancel()
	items := r.conversations.all()
	// 会話ごとの停止は標準入力を閉じた後の猶予を待つため、並べて止める。
	stopped := make([]error, len(items))
	var stopping sync.WaitGroup
	for index, item := range items {
		stopping.Go(func() { stopped[index] = item.stop() })
	}
	stopping.Wait()
	problems := slices.DeleteFunc(stopped, func(err error) bool { return err == nil })
	r.wait.Wait()
	for _, item := range items {
		if err := os.RemoveAll(item.workDir); err != nil {
			problems = append(problems, fmt.Errorf("removing the conversation directory: %w", err))
		}
	}
	return errors.Join(problems...)
}

// isClosed は Close を呼んだ後であることを返す。
func (r *Relay) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// statusResponse は中継の状態である。画面は、この JSON を読めたときだけ中継が有ると読む。
type statusResponse struct {
	Relay     string                `json:"relay"`
	Providers []core.AssistProvider `json:"providers"`
}

func (r *Relay) status(w http.ResponseWriter, _ *http.Request) {
	providers := make([]core.AssistProvider, 0, len(r.config.Providers))
	for _, provider := range core.AssistProviders() {
		if _, registered := r.config.Providers[provider]; registered {
			providers = append(providers, provider)
		}
	}
	writeRelayJSON(w, http.StatusOK, statusResponse{Relay: "oraculum-assist", Providers: providers})
}

// browserClient は server にセッションを確かめ、要求を出した browser の client を返す。
func (r *Relay) browserClient(w http.ResponseWriter, request *http.Request) (ServerClient, bool) {
	client := r.client.forBrowser(request)
	response, err := client.checkSession(request.Context())
	if err != nil {
		slog.Error("checking the browser session failed", "error", err)
		writeRelayError(w, http.StatusBadGateway, "the Oraculum server did not confirm the session")
		return ServerClient{}, false
	}
	if !response.OK() {
		passServerError(w, response)
		return ServerClient{}, false
	}
	return client, true
}

func (r *Relay) listConversations(w http.ResponseWriter, request *http.Request) {
	client, ok := r.browserClient(w, request)
	if !ok {
		return
	}
	writeRelayJSON(w, http.StatusOK, map[string]any{"conversations": r.conversations.summaries(client)})
}

// openConversation は server に会話を発行させ、提供者の会話を起動する。
func (r *Relay) openConversation(w http.ResponseWriter, request *http.Request) {
	client, ok := r.browserClient(w, request)
	if !ok {
		return
	}
	var body struct {
		Provider core.AssistProvider `json:"provider"`
	}
	if !decodeRelayBody(w, request, &body) {
		return
	}
	provider, registered := r.config.Providers[body.Provider]
	if !registered {
		writeRelayError(w, http.StatusBadRequest, "the relay has no such provider")
		return
	}
	opened, response, err := client.OpenConversation(request.Context(), body.Provider)
	if err != nil {
		slog.Error("opening the conversation on the Oraculum server failed", "error", err)
		writeRelayError(w, http.StatusBadGateway, "the Oraculum server did not open the conversation")
		return
	}
	if !response.OK() {
		passServerError(w, response)
		return
	}
	item, err := r.launch(opened, provider, client)
	if err != nil {
		slog.Error("launching the provider failed", "error", err)
		writeRelayError(w, http.StatusBadGateway, "the relay could not launch the provider")
		return
	}
	if !r.adopt(item) {
		writeRelayError(w, http.StatusServiceUnavailable, errRelayClosed.Error())
		return
	}
	writeRelayJSON(w, http.StatusCreated, map[string]any{"conversation": conversationSummary{
		Id: item.id, Provider: item.provider,
	}})
}

// errRelayClosed は、中継が止まる途中であることを表す。
var errRelayClosed = errors.New("the relay is shutting down")

// adopt は起動した会話を一覧に足す。中継が止まる途中なら、会話を止めて偽を返す。
//
// **Close と同じ lock の中で足す。** Close が止める会話の一覧を取った後に、止まらない会話を足さない。
func (r *Relay) adopt(item *conversation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		// #nosec G703 -- workDir は launch の os.MkdirTemp が作る。要求の cookie は client にだけ保持する。
		if err := errors.Join(item.stop(), os.RemoveAll(item.workDir)); err != nil {
			slog.Error("stopping the conversation opened while shutting down failed", "error", err)
		}
		return false
	}
	r.conversations.add(item)
	return true
}

// launch は会話ごとの一時 directory と secret を作り、提供者の会話を起動する。
func (r *Relay) launch(opened core.AssistConversation, provider Provider, client ServerClient) (*conversation, error) {
	secret, err := newSecret()
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(r.config.TempDir, "oraculum-assist-")
	if err != nil {
		return nil, fmt.Errorf("creating the conversation directory: %w", err)
	}
	item := newConversation(opened.Id, opened.Provider, secret, workDir, client)
	session, err := provider.Open(r.ctx, ProviderLaunch{
		WorkDir: workDir, MCPURL: r.MCPURL(), MCPSecret: secret, Instructions: instructions,
	})
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(workDir))
	}
	item.session = session
	return item, nil
}

// events は会話の event のうち、通番が after より大きいものを返す。再読み込みした画面が会話を
// 出し直す。
func (r *Relay) events(w http.ResponseWriter, request *http.Request) {
	client, ok := r.browserClient(w, request)
	if !ok {
		return
	}
	item, found := r.conversations.find(request.PathValue("conversationId"))
	if !found || !item.client.sameSession(client) {
		writeRelayError(w, http.StatusNotFound, "the relay has no such conversation")
		return
	}
	after, err := strconv.ParseInt(request.URL.Query().Get("after"), 10, 64)
	if request.URL.Query().Has("after") && (err != nil || after < 0) {
		writeRelayError(w, http.StatusBadRequest, "after must be a non-negative integer")
		return
	}
	events, _ := item.eventsAfter(after)
	if events == nil {
		events = []core.AssistEvent{}
	}
	_, answering := item.activeTurn()
	writeRelayJSON(w, http.StatusOK, map[string]any{"events": events, "answering": answering})
}

// turnRequestBody は分析者の発言 1 つである。
type turnRequestBody struct {
	TurnId string `json:"turnId"`
	Text   string `json:"text"`
	// Context は発言に添える画面の文脈である。中継は server へ登録し、LLM には server の応答を渡す。
	Context turnContext `json:"context"`
}

// turnContext は発言に添える画面の文脈である。
type turnContext struct {
	MatchConditions         []core.AssistMatchCondition `json:"matchConditions"`
	Case                    string                      `json:"case,omitempty"`
	SearchQuery             *core.SearchQuery           `json:"searchQuery,omitempty"`
	NodeKindsFromConditions bool                        `json:"nodeKindsFromConditions,omitempty"`
	Records                 []core.RecordLocator        `json:"records,omitempty"`
	NodeIds                 []string                    `json:"nodeIds,omitempty"`
	EdgeIds                 []string                    `json:"edgeIds,omitempty"`
}

// turn は発言を受け付け、応答の event を NDJSON で流す。
//
// **browser との接続が切れても、応答が終わるまで提供者を止めない。** 応答は会話の event として
// 残り、再読み込みした画面が読み直す。
func (r *Relay) turn(w http.ResponseWriter, request *http.Request) {
	client, ok := r.browserClient(w, request)
	if !ok {
		return
	}
	item, found := r.conversations.find(request.PathValue("conversationId"))
	if !found || !item.client.sameSession(client) {
		writeRelayError(w, http.StatusNotFound, "the relay has no such conversation")
		return
	}
	var body turnRequestBody
	if !decodeRelayBody(w, request, &body) {
		return
	}
	if core.ValidateAssistTurnId("turnId", body.TurnId) != nil || body.Text == "" {
		writeRelayError(w, http.StatusBadRequest, "the message needs a turnId and a text")
		return
	}
	if r.isClosed() {
		writeRelayError(w, http.StatusServiceUnavailable, errRelayClosed.Error())
		return
	}
	if err := item.begin(body.TurnId, body.Context.MatchConditions); err != nil {
		writeRelayError(w, http.StatusConflict, err.Error())
		return
	}
	response, err := item.client.Call(request.Context(), item.id, "turns", turnRegistration(body))
	if err != nil || !response.OK() {
		item.abandon(body.TurnId)
		if err != nil {
			slog.Error("registering the message on the Oraculum server failed", "error", err)
			writeRelayError(w, http.StatusBadGateway, "the Oraculum server did not register the message")
			return
		}
		passServerError(w, response)
		return
	}
	message, err := item.append(body.TurnId, core.AssistEvent{Kind: core.AssistEventKindUserMessage,
		Text: body.Text})
	if err != nil {
		item.abandon(body.TurnId)
		writeRelayError(w, http.StatusBadRequest, "the message is not readable")
		return
	}
	if !r.answer(item, body.TurnId, promptOf(body.Text, response.Body)) {
		endTurn(item, body.TurnId, "中継が止まる途中のため、発言を提供者へ送りませんでした。")
	}
	streamTurn(w, request, item, body.TurnId, message.Sequence-1)
}

// turnRegistration は発言の登録の要求の本文を組む。
func turnRegistration(body turnRequestBody) map[string]any {
	registration := map[string]any{"turnId": body.TurnId, "matchConditions": body.Context.MatchConditions}
	if body.Context.Case != "" {
		registration["case"] = body.Context.Case
	}
	if body.Context.SearchQuery != nil {
		registration["searchQuery"] = body.Context.SearchQuery
	}
	if body.Context.NodeKindsFromConditions {
		registration["nodeKindsFromConditions"] = true
	}
	if len(body.Context.Records) > 0 {
		registration["records"] = body.Context.Records
	}
	if len(body.Context.NodeIds) > 0 {
		registration["nodeIds"] = body.Context.NodeIds
	}
	if len(body.Context.EdgeIds) > 0 {
		registration["edgeIds"] = body.Context.EdgeIds
	}
	return registration
}

// promptOf は、分析者の発言と、server が登録した画面の文脈から、提供者へ送る発言を組む。
func promptOf(text string, turnBody []byte) string {
	// **server の本文を byte 列のまま渡す。** 受け渡しの監査記録の sha256 は、この本文の byte 列から求める。
	return "分析者の発言:\n" + text + "\n\n画面の文脈 (Oraculum server が受け渡しとして記録した値。短い参照の" +
		"本文は tool で読む):\n" + string(turnBody)
}

// answer は提供者へ発言を送り、応答の event を会話に足す。応答は中継の寿命の ctx で行い、
// browser の要求の取り消しで止めない。中継が止まる途中なら送らずに偽を返す。
//
// **発言 1 つの応答に上限の時間を置く。** 上限を超えたら提供者を止め、会話を止めた会話にする。
// 提供者の出力を待つ Send は ctx を見られないため、提供者を止めて出力を閉じる。
func (r *Relay) answer(item *conversation, turnId, prompt string) bool {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return false
	}
	r.wait.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wait.Done()
		var timedOut atomic.Bool
		timer := time.AfterFunc(r.config.TurnLimit, func() {
			timedOut.Store(true)
			if err := item.stop(); err != nil {
				slog.Error("stopping the provider after the time limit failed", "error", err)
			}
		})
		err := item.session.Send(r.ctx, prompt, func(event core.AssistEvent) error {
			_, err := item.append(turnId, event)
			return err
		})
		timer.Stop()
		switch {
		case err == nil:
			if _, err := item.append(turnId, core.AssistEvent{Kind: core.AssistEventKindTurnEnd}); err != nil {
				slog.Error("recording the end of the response failed", "error", err)
			}
		case timedOut.Load():
			endTurn(item, turnId, "応答が上限の時間内に終わらなかったため、提供者を止めました。新しい会話を始めてください。")
		default:
			slog.Error("the provider did not finish the response", "error", err)
			endTurn(item, turnId, "提供者の応答が途中で終わりました。")
		}
	}()
	return true
}

// endTurn は、提供者の失敗の event を足して発言を終える。
func endTurn(item *conversation, turnId, text string) {
	if _, err := item.append(turnId, core.AssistEvent{Kind: core.AssistEventKindProviderError,
		Text: text}); err != nil {
		slog.Error("recording the provider failure failed", "error", err)
	}
}

// streamTurn は、通番が start より大きい発言の event を NDJSON で書き、発言の終わりで返る。
func streamTurn(w http.ResponseWriter, request *http.Request, item *conversation, turnId string, start int64) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	after := start
	for {
		events, changed := item.eventsAfter(after)
		for _, event := range events {
			after = event.Sequence
			if event.TurnId != turnId {
				continue
			}
			if err := output.WriteJSON(w, event); err != nil {
				return
			}
			if event.Kind == core.AssistEventKindTurnEnd || event.Kind == core.AssistEventKindProviderError {
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-changed:
		case <-request.Context().Done():
			return
		}
	}
}

// passServerError は server の失敗の応答を、status と本文のまま画面へ返す。
func passServerError(w http.ResponseWriter, response ServerResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.Status)
	if _, err := w.Write(response.Body); err != nil {
		slog.Error("writing the server failure failed", "error", err)
	}
}

// relayError は中継の失敗の応答の本文である。
type relayError struct {
	Message string `json:"message"`
}

func writeRelayError(w http.ResponseWriter, status int, message string) {
	writeRelayJSON(w, status, relayError{Message: message})
}

func writeRelayJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := output.WriteJSON(w, value); err != nil {
		slog.Error("writing the relay response failed", "error", err)
	}
}

// maxRelayBodyBytes は中継が読む要求の本文の上限である。
const maxRelayBodyBytes = 1 << 20

// decodeRelayBody は要求の本文を、未知の項目と後続の値を退けて読む。失敗したときは応答を書き、
// 偽を返す。
func decodeRelayBody(w http.ResponseWriter, request *http.Request, into any) bool {
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRelayBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		writeRelayError(w, http.StatusBadRequest, "the request body is not a readable JSON object")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeRelayError(w, http.StatusBadRequest, "the request body carries a value after its JSON object")
		return false
	}
	return true
}
