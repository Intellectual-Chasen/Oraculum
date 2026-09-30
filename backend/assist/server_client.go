package assist

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// conversationOperations は、中継の tool が呼ぶ server の会話の操作である。
//
// **ここに無い操作を呼ばない。** client は許可した endpoint だけを列挙し、browser 用の転送の
// 経路を通さない。
var conversationOperations = []string{
	"turns", "overview", "graph-search", "targets", "records", "timeline", "search-query-checks",
}

// maxServerResponseBytes は server の応答を読む上限である。
//
// 既知の制限: 応答を 8 MiB までにする, server は LLM 用の本文を件数の上限で切り詰め、1 回の本文は
// 数十 KiB である。上限ちょうどの本文は測っていない, server の上限を上げたときに見直す
const maxServerResponseBytes = 8 << 20

// ServerClient は、Oraculum server の会話の endpoint だけを呼ぶ client である。
type ServerClient struct {
	base *url.URL
	http *http.Client
	// session は、この会話を開いた browser の server セッションである。提供者へ渡さない。
	session string
}

// NewServerClient は server の origin へ要求を送る client を返す。
func NewServerClient(base *url.URL, client *http.Client) ServerClient {
	isolated := *client
	// cookie は browser の要求から会話ごとに選ぶ。共用の jar と転送先への redirect を使わない。
	isolated.Jar = nil
	isolated.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return ServerClient{base: base, http: &isolated}
}

// serverSessionCookie は server のログインで発行する cookie の名前である。
const serverSessionCookie = "oraculum_session"

// forBrowser は要求のセッションだけを使う client のコピーを返す。
func (c ServerClient) forBrowser(request *http.Request) ServerClient {
	c.session = ""
	if cookie, err := request.Cookie(serverSessionCookie); err == nil {
		c.session = cookie.Value
	}
	return c
}

// sameSession は両方の client が同じ browser のセッションを使うかを返す。
func (c ServerClient) sameSession(other ServerClient) bool {
	return subtle.ConstantTimeCompare([]byte(c.session), []byte(other.session)) == 1
}

// checkSession はログアウト・失効を含めて server に現在の認証を確かめる。
func (c ServerClient) checkSession(ctx context.Context) (ServerResponse, error) {
	return c.request(ctx, http.MethodGet, "/api/v0/session/me", nil)
}

// ServerResponse は server の応答の status と本文である。
type ServerResponse struct {
	Status int
	Body   []byte
}

// OK は応答が成功の status であることを返す。
func (r ServerResponse) OK() bool { return r.Status >= 200 && r.Status < 300 }

// ErrorMessage は失敗の応答が持つ ApiError の message を返す。読めない本文では status だけを返す。
func (r ServerResponse) ErrorMessage() string {
	var apiError core.ApiError
	if json.Unmarshal(r.Body, &apiError) != nil || apiError.Message == "" {
		return fmt.Sprintf("the Oraculum server answered %d", r.Status)
	}
	return string(apiError.Code) + ": " + apiError.Message
}

// OpenConversation は server に会話を発行させる。
func (c ServerClient) OpenConversation(
	ctx context.Context, provider core.AssistProvider,
) (core.AssistConversation, ServerResponse, error) {
	response, err := c.post(ctx, "/api/v0/conversations", map[string]any{"provider": provider})
	if err != nil || !response.OK() {
		return core.AssistConversation{}, response, err
	}
	var opened struct {
		Conversation core.AssistConversation `json:"conversation"`
	}
	if err := json.Unmarshal(response.Body, &opened); err != nil {
		return core.AssistConversation{}, response, fmt.Errorf("reading the opened conversation: %w", err)
	}
	if err := opened.Conversation.Validate(); err != nil {
		return core.AssistConversation{}, response, fmt.Errorf("reading the opened conversation: %w", err)
	}
	return opened.Conversation, response, nil
}

// Call は会話の操作 operation を呼ぶ。本文は JSON にして送る。
func (c ServerClient) Call(ctx context.Context, conversationId, operation string, body any) (ServerResponse, error) {
	if !slices.Contains(conversationOperations, operation) {
		return ServerResponse{}, fmt.Errorf("the operation %q is not a conversation operation", operation)
	}
	if core.ValidateAssistConversationId("conversation", conversationId) != nil {
		return ServerResponse{}, errors.New("the conversation identifier is not readable")
	}
	return c.post(ctx, "/api/v0/conversations/"+conversationId+"/"+operation, body)
}

func (c ServerClient) post(ctx context.Context, path string, body any) (ServerResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return ServerResponse{}, fmt.Errorf("encoding the request: %w", err)
	}
	return c.request(ctx, http.MethodPost, path, bytes.NewReader(encoded))
}

func (c ServerClient) request(ctx context.Context, method, path string, body io.Reader) (ServerResponse, error) {
	target := c.base.JoinPath(path)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return ServerResponse{}, fmt.Errorf("building the request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if c.session != "" {
		request.AddCookie(&http.Cookie{Name: serverSessionCookie, Value: c.session})
	}
	response, err := c.http.Do(request)
	if err != nil {
		return ServerResponse{}, fmt.Errorf("calling the Oraculum server: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxServerResponseBytes+1))
	if err != nil {
		return ServerResponse{}, fmt.Errorf("reading the Oraculum server response: %w", err)
	}
	if len(data) > maxServerResponseBytes {
		return ServerResponse{}, errors.New("the Oraculum server response exceeds the limit")
	}
	return ServerResponse{Status: response.StatusCode, Body: data}, nil
}
