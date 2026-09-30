package assist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// mcpPath は MCP の待ち受けの path である。
const mcpPath = "/mcp"

// MCPHandler は、会話ごとの bearer secret で守る MCP の待ち受けの handler を返す。
//
// **tool の引数は会話の識別子を持たない。** 会話は bearer secret から探す。証拠の文に従った LLM が
// 別の会話を指せない。
func (r *Relay) MCPHandler() http.Handler {
	servers := &mcpServers{relay: r, byConversation: map[string]*mcp.Server{}}
	streamable := mcp.NewStreamableHTTPHandler(servers.serverFor, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
	})
	verify := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		item, found := r.conversations.bySecret(token)
		if !found {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: item.id, Expiration: time.Now().Add(time.Hour)}, nil
	}
	mux := http.NewServeMux()
	mux.Handle(mcpPath, auth.RequireBearerToken(verify, nil)(streamable))
	return guardMCP(r.config.MCPPort, mux)
}

// mcpServers は会話ごとの MCP の server を 1 回だけ組んで保つ。
type mcpServers struct {
	relay          *Relay
	mu             sync.Mutex
	byConversation map[string]*mcp.Server
}

// serverFor は bearer secret で確かめた会話の MCP の server を返す。
func (s *mcpServers) serverFor(request *http.Request) *mcp.Server {
	info := auth.TokenInfoFromContext(request.Context())
	if info == nil {
		return nil
	}
	item, found := s.relay.conversations.find(info.UserID)
	if !found {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if server, built := s.byConversation[item.id]; built {
		return server
	}
	server := newToolServer(s.relay, item)
	server.AddReceivingMiddleware(recordToolCalls(item))
	s.byConversation[item.id] = server
	return server
}

// turnInput は、応答の途中の発言の中で読む tool の共通の入力である。
type overviewInput struct{}

type graphSearchInput struct {
	SearchQuery core.SearchQuery `json:"searchQuery" jsonschema:"the search conditions; nodeKinds and granularity choose what to show; nodeIds takes node references issued in this conversation, such as n3"`
}

type targetInput struct {
	Ref string `json:"ref" jsonschema:"a node or edge reference issued in this conversation, such as n3 or e7"`
}

type recordsInput struct {
	Refs []string `json:"refs" jsonschema:"record references issued in this conversation, such as r12; at most 10"`
}

type timelineInput struct {
	Conditions core.RecordConditions `json:"conditions" jsonschema:"conditions narrowing the evidence records"`
}

type showSearchQueryInput struct {
	SearchQuery core.SearchQuery `json:"searchQuery" jsonschema:"the search conditions the analyst can apply on the screen; nodeIds takes node references issued in this conversation, such as n3; the screen shows the record granularity only with the record nodeKind; without nodeKinds and granularity the screen chooses what to show from the conditions"`
	Explanation string           `json:"explanation" jsonschema:"why these conditions help the analyst, in Japanese"`
}

// newToolServer は会話 1 件の MCP の server を、Oraculum の tool を持たせて組む。
func newToolServer(r *Relay, item *conversation) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "oraculum", Version: "1"}, nil)
	read := func(operation string) func(ctx context.Context, body map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, body map[string]any) (*mcp.CallToolResult, any, error) {
			turn, active := item.activeTurn()
			if !active {
				return errorResult("no message of the analyst is being answered"), nil, nil
			}
			body["turnId"] = turn.id
			response, err := item.client.Call(ctx, item.id, operation, body)
			if err != nil {
				return nil, nil, err
			}
			if !response.OK() {
				return errorResult(response.ErrorMessage()), nil, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(response.Body)}}}, nil, nil
		}
	}
	mcp.AddTool(server, &mcp.Tool{Name: "overview", Description: "調査の概要 (収集元、ノードの種類ごとの件数、事象の種別、" +
		"関連付けの条件) を読む。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ overviewInput) (*mcp.CallToolResult, any, error) {
			return read("overview")(ctx, map[string]any{})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "graph_search", Description: "検索の条件でグラフを検索し、合ったノードと関係を" +
		"短い参照付きで読む。レコードのノードは record に記録の参照を持つ。" +
		"分析者の画面が条件を適用するボタンを表示するのは、granularity が record のときは nodeKinds に" +
		"record を含む条件の検索である。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, input graphSearchInput) (*mcp.CallToolResult, any, error) {
			return read("graph-search")(ctx, map[string]any{"searchQuery": input.SearchQuery})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "target_detail", Description: "この会話で受け取ったノードまたは関係の参照の詳細" +
		"(属性と根拠の記録の参照) を読む。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, input targetInput) (*mcp.CallToolResult, any, error) {
			return read("targets")(ctx, map[string]any{"ref": input.Ref})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "record_text", Description: "この会話で受け取った記録の参照の原文と読めた欄を" +
		"読む。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, input recordsInput) (*mcp.CallToolResult, any, error) {
			return read("records")(ctx, map[string]any{"refs": input.Refs})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "timeline", Description: "根拠の記録を絞る条件で時系列を読む。" +
		"期間の書き方は graph_search と同じである。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, input timelineInput) (*mcp.CallToolResult, any, error) {
			return read("timeline")(ctx, map[string]any{"conditions": input.Conditions})
		})
	mcp.AddTool(server, &mcp.Tool{Name: "show_search_query", Description: "検索の条件を分析者の画面に card として出す。" +
		"server が条件を検証し、通ったときだけ出す。分析者が適用するかを決める。"},
		func(ctx context.Context, _ *mcp.CallToolRequest, input showSearchQueryInput) (*mcp.CallToolResult, any, error) {
			return r.showSearchQuery(ctx, item, input)
		})
	return server
}

// showSearchQuery は、server が検証を通した検索の条件を、応答の途中の発言の card として会話に足す。
func (r *Relay) showSearchQuery(
	ctx context.Context, item *conversation, input showSearchQueryInput,
) (*mcp.CallToolResult, any, error) {
	turn, active := item.activeTurn()
	if !active {
		return errorResult("no message of the analyst is being answered"), nil, nil
	}
	checked, rejection, err := checkSearchQuery(ctx, item, turn.id, input.SearchQuery)
	if err != nil {
		return nil, nil, err
	}
	if rejection != "" {
		return errorResult(rejection), nil, nil
	}
	if _, err := item.append(turn.id, core.AssistEvent{
		Kind: core.AssistEventKindSearchQueryCard, SearchQuery: &checked.query, Origins: checked.origins,
		Explanation: input.Explanation, MatchConditions: turn.conditions,
	}); err != nil {
		return errorResult("the search conditions are not showable"), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{
		Text: `{"shown":true}`,
	}}}, nil, nil
}

// recordToolCalls は、応答の途中の発言への tool の呼び出しと結果を、会話の event として記録する。
//
// **受信の 1 か所で記録する。** 入力の schema で退けた呼び出しと知らない tool の呼び出しも画面に
// 出す。結果は呼び出しを記録した発言へ足し、上限の時間で応答が終わった後に返った結果も残す。
func recordToolCalls(item *conversation) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			call, isCall := request.(*mcp.CallToolRequest)
			if !isCall || call.Params == nil {
				return next(ctx, method, request)
			}
			turn, active := item.activeTurn()
			if !active {
				return next(ctx, method, request)
			}
			name, input := call.Params.Name, call.Params.Arguments
			if len(input) > 0 && !json.Valid(input) {
				input, _ = json.Marshal(string(input))
			}
			used, err := item.append(turn.id, core.AssistEvent{
				Kind: core.AssistEventKindToolUse, ToolName: name, ToolInput: input,
			})
			if err != nil {
				return next(ctx, method, request)
			}
			result, callErr := next(ctx, method, request)
			event := core.AssistEvent{Kind: core.AssistEventKindToolResult, ToolName: name, ToolUseSequence: used.Sequence}
			if tool, isTool := result.(*mcp.CallToolResult); callErr == nil && isTool {
				event.ToolResult, event.ToolFailed = resultText(tool), tool.IsError
			} else {
				event.ToolFailed = true
				if callErr != nil {
					event.ToolResult = callErr.Error()
				}
			}
			if name == "graph_search" && !event.ToolFailed {
				if checked, found := showableQuery(ctx, item, turn.id, call.Params.Arguments); found {
					event.SearchQuery, event.Origins, event.MatchConditions = &checked.query, checked.origins,
						turn.conditions
				}
			}
			// 画面に適用する条件を付けた event を記録できないときも、tool の結果は記録する。
			if _, err := item.append(turn.id, event); err != nil && event.SearchQuery != nil {
				event.SearchQuery, event.Origins, event.MatchConditions = nil, nil, nil
				_, _ = item.append(turn.id, event)
			}
			return result, callErr
		}
	}
}

// checkedQuery は、server が検証を通し、起点の短い参照をノードの識別子へ直した検索の条件である。
type checkedQuery struct {
	query   core.SearchQuery
	origins []core.AssistOrigin
}

// showableQuery は、グラフの検索の入力の条件が画面の検索欄で表せるときにその条件を返す。
//
// 起点を持たない条件は、読み取りを通った条件と同じであるため、server へ検証を求めない。起点を
// 持つ条件は、起点の識別子と表示名を server に求める。検証の失敗は、条件を返さないだけである。
func showableQuery(
	ctx context.Context, item *conversation, turnId string, arguments json.RawMessage,
) (checkedQuery, bool) {
	var input graphSearchInput
	if json.Unmarshal(arguments, &input) != nil || input.SearchQuery.Validate() != nil ||
		input.SearchQuery.ValidateShowable() != nil {
		return checkedQuery{}, false
	}
	if len(input.SearchQuery.NodeIds) == 0 {
		return checkedQuery{query: input.SearchQuery}, true
	}
	checked, rejection, err := checkSearchQuery(ctx, item, turnId, input.SearchQuery)
	return checked, err == nil && rejection == ""
}

// checkSearchQuery は server に検索の条件を検証させる。退けたときは、LLM が次の手を判別できる
// 理由を rejection に返す。
func checkSearchQuery(
	ctx context.Context, item *conversation, turnId string, query core.SearchQuery,
) (checked checkedQuery, rejection string, err error) {
	response, err := item.client.Call(ctx, item.id, "search-query-checks", map[string]any{
		"turnId": turnId, "searchQuery": query,
	})
	if err != nil {
		return checkedQuery{}, "", err
	}
	if !response.OK() {
		return checkedQuery{}, response.ErrorMessage(), nil
	}
	var check struct {
		Accepted bool                `json:"accepted"`
		Reason   string              `json:"reason"`
		Origins  []core.AssistOrigin `json:"origins"`
	}
	// 受理した条件は起点ごとに 1 つの解決を持つ。数の合わない応答から条件を組み直すと、起点を
	// 落とした条件を画面へ渡す。
	if err := json.Unmarshal(response.Body, &check); err != nil ||
		(check.Accepted && len(check.Origins) != len(query.NodeIds)) {
		return checkedQuery{}, "", errors.New("the search query check is not readable")
	}
	if !check.Accepted {
		return checkedQuery{}, "the server rejected the search conditions: " + check.Reason, nil
	}
	checked = checkedQuery{query: query, origins: check.Origins}
	checked.query.NodeIds = nil
	for _, origin := range check.Origins {
		checked.query.NodeIds = append(checked.query.NodeIds, origin.Id)
	}
	return checked, "", nil
}

// resultText は tool の結果の文の部分をつないだ文字列である。
func resultText(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if part, isText := content.(*mcp.TextContent); isText {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

// errorResult は、LLM が次の手を判別できる理由を持つ tool の失敗の結果である。
func errorResult(reason string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: reason}}}
}
