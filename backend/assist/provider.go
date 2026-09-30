package assist

import (
	"context"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ProviderLaunch は、提供者の会話 1 件を起動する値である。
type ProviderLaunch struct {
	// WorkDir は会話ごとの空の一時 directory である。中継が作り、会話を閉じると消す。
	WorkDir string
	// MCPURL と MCPSecret は、会話の MCP の待ち受けと bearer secret である。
	MCPURL    string
	MCPSecret string
	// Instructions は system prompt に足す指示である。
	Instructions string
}

// Provider は、LLM の提供者の会話を起動する port である。Claude、Codex、OpenCode などの提供者は
// adapter が本 port を満たし、cmd が登録する。
type Provider interface {
	// Key は提供者の識別である。
	Key() core.AssistProvider
	// Open は会話 1 件を起動する。
	Open(ctx context.Context, launch ProviderLaunch) (ProviderSession, error)
}

// ProviderSession は、起動した提供者の会話 1 件である。
type ProviderSession interface {
	// Send は発言を 1 つ送り、応答の event を emit へ順に渡し、発言の終わりで返る。emit へ渡す
	// event は種類と文と tool の名前だけを持ち、通番と発言の識別子は中継が付ける。
	Send(ctx context.Context, text string, emit func(core.AssistEvent) error) error
	// Close は提供者の会話を止める。
	Close() error
}
