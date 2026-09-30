package claudecli

import (
	"encoding/json"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// streamEvent は stream-json の出力の event 1 つのうち、本 package が読む項目である。
type streamEvent struct {
	Type    string          `json:"type"`
	Message *streamMessage  `json:"message,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// readEvent は stream-json の 1 行を読み、応答の event を emit へ渡す。done は発言の終わりの
// event を読んだことである。
//
// **読めない行と知らない event を読み飛ばす。** CLI がバージョンの更新で足した event で会話を止めない。
func readEvent(line []byte, emit func(core.AssistEvent) error) (done bool, err error) {
	var event streamEvent
	if json.Unmarshal(line, &event) != nil {
		return false, nil
	}
	switch event.Type {
	case "assistant":
		if event.Message == nil {
			return false, nil
		}
		// tool の呼び出しは中継の MCP の待ち受けが入力と結果とともに記録するため、文だけを読む。
		for _, content := range event.Message.Content {
			if content.Type != "text" || content.Text == "" {
				continue
			}
			if err := emit(core.AssistEvent{Kind: core.AssistEventKindText, Text: content.Text}); err != nil {
				return false, err
			}
		}
		return false, nil
	case "result":
		if event.IsError {
			return true, fmt.Errorf("the claude cli reported an error for the message: %w", ErrProviderFailed)
		}
		return true, nil
	default:
		return false, nil
	}
}
