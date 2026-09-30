package claudecli_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/claudecli"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// fakeCLI は argv と環境変数と標準入力を作業 directory へ書き、発言ごとにテスト用の stream を出す
// 偽の `claude` である。
const fakeCLI = `#!/bin/sh
for a in "$@"; do printf '%s\n' "$a"; done > argv.txt
env > env.txt
while IFS= read -r line; do
  printf '%s\n' "$line" >> input.txt
  case "$line" in
    *fail*) printf '%s\n' '{"type":"result","subtype":"error_during_execution","is_error":true}'; continue;;
  esac
  printf '%s\n' '{"type":"system","subtype":"init"}'
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"reply"},{"type":"tool_use","name":"mcp__oraculum__graph_search","input":{}}]}}'
  printf '%s\n' 'not a JSON line'
  printf '%s\n' '{"type":"future_event"}'
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false}'
done
`

func writeFakeCLI(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on PATH; the fake CLI is a POSIX shell script and runs on the CI runners")
	}
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte(fakeCLI), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// 起動の argv は組み込みの tool を持たず、Oraculum の MCP の tool だけを許す組である。子 process は
// API key を受け取らず、MCP の secret は argv に載らない。
func TestOpenLaunchesTheCLIWithOnlyTheOraculumTools(t *testing.T) {
	cli := claudecli.CLI{Path: writeFakeCLI(t), Environ: claudecli.FilterEnvironment([]string{
		"HOME=/home/analyst", "PATH=" + os.Getenv("PATH"), "ANTHROPIC_API_KEY=synthetic-key",
		"ANTHROPIC_AUTH_TOKEN=synthetic-token", "LC_ALL=C.UTF-8", "GITHUB_TOKEN=synthetic-github",
	})}
	workDir := t.TempDir()
	launch := claudecli.Launch{WorkDir: workDir, MCPURL: "http://127.0.0.1:1/mcp", MCPSecret: "synthetic-secret",
		Instructions: "use the Oraculum tools"}
	session, err := cli.Open(context.Background(), launch)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var events []core.AssistEvent
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := session.Send(ctx, "first question", func(event core.AssistEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	argv := readLines(t, filepath.Join(workDir, "argv.txt"))
	want := claudecli.Args(launch, filepath.Join(workDir, "mcp-config.json"))
	if !slices.Equal(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	for _, required := range [][2]string{
		{"--tools", ""}, {"--allowedTools", "mcp__oraculum__*"}, {"--permission-mode", "dontAsk"},
	} {
		index := slices.Index(argv, required[0])
		if index < 0 || index+1 >= len(argv) || argv[index+1] != required[1] {
			t.Errorf("argv lacks %s %q", required[0], required[1])
		}
	}
	for _, flag := range []string{"--restricted", "--strict-mcp-config", "-p"} {
		if !slices.Contains(argv, flag) {
			t.Errorf("argv lacks %s", flag)
		}
	}
	if strings.Contains(strings.Join(argv, " "), "synthetic-secret") {
		t.Error("the MCP secret is on the argv")
	}
	env := strings.Join(readLines(t, filepath.Join(workDir, "env.txt")), "\n")
	for _, leaked := range []string{"synthetic-key", "synthetic-token", "synthetic-github"} {
		if strings.Contains(env, leaked) {
			t.Errorf("the child environment carries %s", leaked)
		}
	}
	if !strings.Contains(env, "HOME=/home/analyst") || !strings.Contains(env, "LC_ALL=C.UTF-8") {
		t.Errorf("the child environment lacks HOME or the locale: %s", env)
	}

	info, err := os.Stat(filepath.Join(workDir, "mcp-config.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mcp config = %v, %v, want mode 0600", info, err)
	}
	var config struct {
		McpServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	data, _ := os.ReadFile(filepath.Join(workDir, "mcp-config.json"))
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	server := config.McpServers["oraculum"]
	if server.Type != "http" || server.URL != launch.MCPURL ||
		server.Headers["Authorization"] != "Bearer synthetic-secret" {
		t.Errorf("mcp config = %+v", config)
	}

	// tool の呼び出しは中継の MCP の待ち受けが記録するため、CLI の出力からは読まない。
	wantEvents := []core.AssistEvent{
		{Kind: core.AssistEventKindText, Text: "reply"},
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("events = %+v, want %+v", events, wantEvents)
	}
}

// 1 つの process に発言を続けて送り、失敗の結果は ErrProviderFailed で返す。
func TestSessionSendsMessagesInOrderAndReportsAFailure(t *testing.T) {
	cli := claudecli.CLI{Path: writeFakeCLI(t), Environ: claudecli.FilterEnvironment(os.Environ())}
	workDir := t.TempDir()
	session, err := cli.Open(context.Background(), claudecli.Launch{WorkDir: workDir, MCPURL: "http://127.0.0.1:1/mcp",
		MCPSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ignore := func(core.AssistEvent) error { return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, text := range []string{"one", "two"} {
		if err := session.Send(ctx, text, ignore); err != nil {
			t.Fatalf("Send(%q) = %v", text, err)
		}
	}
	if err := session.Send(ctx, "please fail", ignore); !errors.Is(err, claudecli.ErrProviderFailed) {
		t.Errorf("Send of a failing message = %v, want ErrProviderFailed", err)
	}
	var inputs []string
	for _, line := range readLines(t, filepath.Join(workDir, "input.txt")) {
		var message struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatal(err)
		}
		if message.Type != "user" || message.Message.Role != "user" {
			t.Errorf("input line = %s", line)
		}
		inputs = append(inputs, message.Message.Content)
	}
	if !slices.Equal(inputs, []string{"one", "two", "please fail"}) {
		t.Errorf("inputs = %q", inputs)
	}
	if err := session.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}
