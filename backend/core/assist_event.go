package core

import "encoding/json"

// AssistEventKind は、会話の event の種類である。
type AssistEventKind string

// AssistEventKind の値。
const (
	// AssistEventKindUserMessage は分析者の発言である。
	AssistEventKindUserMessage AssistEventKind = "user_message"
	// AssistEventKindText は LLM の応答の文である。
	AssistEventKindText AssistEventKind = "text"
	// AssistEventKindToolUse は LLM が tool を呼んだことである。tool の名前と入力を持つ。
	AssistEventKindToolUse AssistEventKind = "tool_use"
	// AssistEventKindToolResult は tool の呼び出しの結果である。
	AssistEventKindToolResult AssistEventKind = "tool_result"
	// AssistEventKindSearchQueryCard は、LLM が画面に出した検索の条件の card である。
	AssistEventKindSearchQueryCard AssistEventKind = "search_query_card"
	// AssistEventKindTurnEnd は発言 1 つへの応答の終わりである。
	AssistEventKindTurnEnd AssistEventKind = "turn_end"
	// AssistEventKindProviderError は、提供者の失敗で応答が途中で終わったことである。
	AssistEventKindProviderError AssistEventKind = "provider_error"
)

// IsKnown は値が定義の中にあることを返す。
func (k AssistEventKind) IsKnown() bool {
	switch k {
	case AssistEventKindUserMessage, AssistEventKindText, AssistEventKindToolUse, AssistEventKindToolResult,
		AssistEventKindSearchQueryCard, AssistEventKindTurnEnd, AssistEventKindProviderError:
		return true
	default:
		return false
	}
}

// AssistEvent は会話の event 1 つである。中継が会話ごとに順に保ち、画面へ流す。
//
// **LLM の出力は文字列として渡す。** 画面は応答の文を Markdown として描くが、HTML を描かず、
// 画像を読み込まず、URL をリンクにしない。
type AssistEvent struct {
	// Sequence は会話の中の event の通番である。1 から始まる。
	Sequence int64           `json:"sequence"`
	TurnId   string          `json:"turnId"`
	Kind     AssistEventKind `json:"kind"`
	// Text は、分析者の発言、LLM の応答の文、提供者の失敗の説明である。
	Text string `json:"text,omitempty"`
	// ToolName は LLM が呼んだ tool の名前である。tool の呼び出しと結果が持つ。
	ToolName string `json:"toolName,omitempty"`
	// ToolInput は tool の呼び出しの入力の JSON である。
	ToolInput json.RawMessage `json:"toolInput,omitempty"`
	// ToolUseSequence は、tool の結果が答える呼び出しの event の通番である。
	ToolUseSequence int64 `json:"toolUseSequence,omitempty"`
	// ToolResult は LLM に返した tool の結果の本文である。ToolFailed は結果が失敗であることである。
	ToolResult string `json:"toolResult,omitempty"`
	ToolFailed bool   `json:"toolFailed,omitempty"`
	// SearchQuery は、server が検証を通した、画面の検索欄が表せる条件である。検索の条件の card が
	// 必ず持ち、tool の結果は画面に適用できるときだけ持つ。NodeIds は LLM の短い参照を解決した
	// ノードの識別子である。
	SearchQuery *SearchQuery `json:"searchQuery,omitempty"`
	// Origins は SearchQuery の NodeIds の各ノードの種別と表示名である。NodeIds と同じ順に並ぶ。
	Origins []AssistOrigin `json:"origins,omitempty"`
	// Explanation は、検索の条件の card に LLM が添えた説明である。画面は「AI の説明」と印を付けて
	// 出す。
	Explanation string `json:"explanation,omitempty"`
	// MatchConditions は、card を作った発言の関連付けの条件の選択である。
	MatchConditions []AssistMatchCondition `json:"matchConditions,omitempty"`
}

// Validate は項目の整合を確かめる。
func (e AssistEvent) Validate() error {
	problem := firstProblem(
		ValidateAssistTurnId("AssistEvent.turnId", e.TurnId),
		requireKnownEnum("AssistEvent.kind", e.Kind),
	)
	if problem != nil {
		return problem
	}
	if e.Sequence < 1 {
		return itemError("AssistEvent.sequence", ErrInvalid)
	}
	isResult := e.Kind == AssistEventKindToolResult
	isTool := isResult || e.Kind == AssistEventKindToolUse
	switch {
	case isTool != (e.ToolName != ""):
		return itemError("AssistEvent.toolName", ErrInconsistentValue)
	case e.Kind != AssistEventKindToolUse && len(e.ToolInput) > 0:
		return itemError("AssistEvent.toolInput", ErrInconsistentValue)
	case isResult != (e.ToolUseSequence > 0), e.ToolUseSequence >= e.Sequence:
		return itemError("AssistEvent.toolUseSequence", ErrInconsistentValue)
	case !isResult && (e.ToolResult != "" || e.ToolFailed):
		return itemError("AssistEvent.toolResult", ErrInconsistentValue)
	case (e.Kind == AssistEventKindSearchQueryCard) != (e.SearchQuery != nil) && !isResult:
		return itemError("AssistEvent.searchQuery", ErrInconsistentValue)
	}
	if e.SearchQuery != nil {
		if err := e.SearchQuery.Validate(); err != nil {
			return itemError("AssistEvent.searchQuery", err)
		}
	}
	if err := e.validateOrigins(); err != nil {
		return err
	}
	return validateAssistMatchConditions("AssistEvent.matchConditions", e.MatchConditions)
}

// validateOrigins は、起点の種別と表示名が検索の条件の起点のノードと同じ順に 1 つずつあることを
// 確かめる。表示名は空の文字列を許す。
func (e AssistEvent) validateOrigins() error {
	var nodeIds []string
	if e.SearchQuery != nil {
		nodeIds = e.SearchQuery.NodeIds
	}
	if len(e.Origins) != len(nodeIds) {
		return itemError("AssistEvent.origins", ErrInconsistentValue)
	}
	for index, origin := range e.Origins {
		if origin.Id != nodeIds[index] || !origin.Kind.IsKnown() {
			return itemError("AssistEvent.origins", ErrInconsistentValue)
		}
	}
	return nil
}

// AssistOrigin は、LLM が検索の起点に指したノードの識別子と種別と表示名である。画面は表示名で
// 起点を出す。
type AssistOrigin struct {
	Id    string   `json:"id"`
	Kind  NodeKind `json:"kind"`
	Label string   `json:"label"`
}
