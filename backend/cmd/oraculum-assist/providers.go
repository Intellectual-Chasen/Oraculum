package main

import (
	"context"
	"io"
	"os"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/claudecli"
	"github.com/Intellectual-Chasen/Oraculum/backend/assist"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/tooling"
)

// claudeProvider は Claude の CLI の adapter を、中継の提供者の port に合わせる。
type claudeProvider struct {
	cli claudecli.CLI
}

func (p claudeProvider) Key() core.AssistProvider { return core.AssistProviderClaude }

func (p claudeProvider) Open(ctx context.Context, launch assist.ProviderLaunch) (assist.ProviderSession, error) {
	return p.cli.Open(ctx, claudecli.Launch{
		WorkDir: launch.WorkDir, MCPURL: launch.MCPURL, MCPSecret: launch.MCPSecret,
		Instructions: launch.Instructions,
	})
}

// providersOf は、端末で見つかった提供者の CLI を登録する。
//
// 提供者を足すときは、core の AssistProvider に値を 1 つ、adapter を 1 つ、ここに登録を 1 つ足す。
func providersOf(stderr io.Writer) (map[core.AssistProvider]assist.Provider, error) {
	path, err := tooling.LookPath("claude")
	if err != nil {
		return nil, err
	}
	return map[core.AssistProvider]assist.Provider{
		core.AssistProviderClaude: claudeProvider{cli: claudecli.CLI{
			Path: path, Environ: claudecli.FilterEnvironment(os.Environ()), Stderr: stderr,
		}},
	}, nil
}
