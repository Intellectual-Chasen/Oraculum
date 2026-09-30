package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/tooling"
)

// mcpServerName は CLI の設定に書く Oraculum の MCP の server の名前である。tool の名前は
// `mcp__oraculum__<tool>` になる。
const mcpServerName = "oraculum"

// allowedEnvironment は子 process に渡す環境変数の名前である。
//
// **API key と認証の token を渡さない。** 渡すのは、CLI が分析者本人のログインを見つける
// ための home と設定の directory、実行 file の探索、locale、proxy と証明書の設定である。
var allowedEnvironment = []string{
	"HOME", "PATH", "LANG", "LANGUAGE", "TZ", "TMPDIR", "CLAUDE_CONFIG_DIR",
	"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "SSL_CERT_DIR",
	"USERPROFILE", "APPDATA", "LOCALAPPDATA", "SystemRoot", "TEMP", "TMP",
}

// FilterEnvironment は environ から許可の一覧の変数だけを残す。`LC_` で始まる locale の変数も
// 残す。
func FilterEnvironment(environ []string) []string {
	kept := make([]string, 0, len(allowedEnvironment))
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.HasPrefix(name, "LC_") || contains(allowedEnvironment, name) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// CLI は起動する `claude` の実行 file と、子 process に渡す環境変数である。
type CLI struct {
	// Path は `claude` の実行 file の path である。
	Path string
	// Environ は子 process に渡す環境変数である。FilterEnvironment で選んだ値を渡す。
	Environ []string
	// Stderr は子 process の標準 error の書き込み先である。nil は捨てる。
	Stderr io.Writer
}

// Launch は会話 1 件の起動の値である。
type Launch struct {
	// WorkDir は会話ごとの空の一時 directory である。MCP の設定の file を置き、子 process の作業
	// directory にする。
	WorkDir string
	// MCPURL と MCPSecret は、中継が会話のために開いた MCP の待ち受けと bearer secret である。
	MCPURL    string
	MCPSecret string
	// Instructions は system prompt に足す指示である。
	Instructions string
}

// Args は Launch の起動の argv (実行 file の名前を除く) を返す。mcpConfig は MCP の設定の file の
// path である。
func Args(launch Launch, mcpConfig string) []string {
	args := []string{
		"-p", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json",
		"--restricted", "--tools", "",
		"--mcp-config", mcpConfig, "--strict-mcp-config",
		"--allowedTools", "mcp__" + mcpServerName + "__*",
		"--permission-mode", "dontAsk",
	}
	if launch.Instructions != "" {
		args = append(args, "--append-system-prompt", launch.Instructions)
	}
	return args
}

// mcpConfigOf は MCP の設定の file の中身を返す。
func mcpConfigOf(launch Launch) ([]byte, error) {
	config := map[string]any{"mcpServers": map[string]any{mcpServerName: map[string]any{
		"type": "http", "url": launch.MCPURL,
		"headers": map[string]string{"Authorization": "Bearer " + launch.MCPSecret},
	}}}
	return json.Marshal(config)
}

// Open は `claude` を起動し、発言を続けて送れる会話を返す。
func (c CLI) Open(ctx context.Context, launch Launch) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := mcpConfigOf(launch)
	if err != nil {
		return nil, fmt.Errorf("writing the MCP configuration: %w", err)
	}
	path := filepath.Join(launch.WorkDir, "mcp-config.json")
	if err := os.WriteFile(path, config, 0o600); err != nil {
		// file の中身 (secret) を error に載せない。
		return nil, fmt.Errorf("writing the MCP configuration in %q: %w", launch.WorkDir, err)
	}
	process, err := tooling.StartProcess(tooling.ProcessSpec{
		Path: c.Path, Args: Args(launch, path), Env: c.Environ, Dir: launch.WorkDir, Stderr: c.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("launching the claude CLI: %w", err)
	}
	scanner := bufio.NewScanner(process.Stdout)
	scanner.Buffer(make([]byte, 0, initialLineBuffer), maxLineBytes)
	return &Session{process: process, lines: scanner}, nil
}

// 標準出力の 1 行の長さの上限。
//
// 既知の制限: 1 行を 16 MiB までにする, 1 行は応答の文か tool の結果を含む event 1 つであり、tool の
// 本文は server が件数の上限で切り詰める (1 回の本文は数十 KiB)。実機の行の長さは測っていない,
// 実機の会話でこの上限に達したときに見直す
const (
	initialLineBuffer = 64 * 1024
	maxLineBytes      = 16 * 1024 * 1024
)

// Session は起動した `claude` の会話 1 件である。
type Session struct {
	process *tooling.Process
	lines   *bufio.Scanner
	// mu は発言を 1 つずつ送る。
	mu sync.Mutex
}

// ErrProviderFailed は、CLI が発言への応答を失敗で終えたか、応答の途中で終わったことを表す。
var ErrProviderFailed = errors.New("claude cli: the response failed")

// Send は発言を 1 つ送り、応答の event を emit へ順に渡し、発言の終わりで返る。
//
// **ctx の取り消しで process を止めない。** 取り消したときは応答の終わりまで読み捨てずに返り、
// 次の Send は前の応答の残りを読み飛ばさない。呼び出し元は取り消した会話を閉じる。
func (s *Session) Send(ctx context.Context, text string, emit func(core.AssistEvent) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, err := json.Marshal(userMessage{Type: "user", Message: userContent{Role: "user", Content: text}})
	if err != nil {
		return fmt.Errorf("encoding the message: %w", err)
	}
	if _, err := s.process.Stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("sending the message: %w: %w", ErrProviderFailed, err)
	}
	for s.lines.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, err := readEvent(s.lines.Bytes(), emit)
		if err != nil || done {
			return err
		}
	}
	if err := s.lines.Err(); err != nil {
		return fmt.Errorf("reading the response: %w: %w", ErrProviderFailed, err)
	}
	// 終了の状態で、CLI が自分で終わったか止められたかを分ける。出力の終わりの時点で終了を
	// 待てていなければ、状態を持たない error を返す。
	select {
	case <-s.process.Done():
		if exit := s.process.Err(); exit != nil {
			return fmt.Errorf("the claude cli ended before the response finished: %w: %w", ErrProviderFailed, exit)
		}
	case <-time.After(time.Second):
	}
	return fmt.Errorf("the claude cli ended before the response finished: %w", ErrProviderFailed)
}

// Close は `claude` を止める。作業 directory は、それを作った呼び出し元が消す。
func (s *Session) Close() error {
	return s.process.Stop()
}

// userMessage は stream-json の入力の発言 1 つである。
type userMessage struct {
	Type    string      `json:"type"`
	Message userContent `json:"message"`
}

type userContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
