package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/assist"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

const usage = "usage: oraculum-assist --server <Oraculum server URL>"

// serverFlag は Oraculum server の URL を渡す flag である。
//
// **配置する環境ごとに変わる値である。** チームで共有する server の場所は運用者が決める。
const serverFlag = "--server"

// readHeaderTimeout は要求 header の読み取りに与える上限である。
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout は停止の合図の後、応答の途中の要求を待つ上限である。
const shutdownTimeout = 5 * time.Second

// run は中継を起動し、停止の合図まで待ち受ける。
func run(args []string, stdout, stderr io.Writer) int {
	slog.SetDefault(output.NewSanitizingLogger(stderr))
	server, err := parseArgs(args)
	if err != nil {
		return reportError(stderr, err.Error()+"\n"+usage)
	}
	providers, err := providersOf(stderr)
	if err != nil {
		return reportError(stderr, "finding the claude CLI:", err)
	}
	return serveRelay(server, providers, stdout, stderr)
}

// parseArgs は起動引数を読む。
func parseArgs(args []string) (*url.URL, error) {
	if len(args) != 2 || args[0] != serverFlag {
		return nil, errors.New("the relay takes " + serverFlag + " once")
	}
	server, err := url.Parse(args[1])
	if err != nil || (server.Scheme != "http" && server.Scheme != "https") || server.Host == "" ||
		(server.Path != "" && server.Path != "/") || server.RawQuery != "" || server.User != nil {
		return nil, errors.New(serverFlag + " must be the http or https origin of the Oraculum server")
	}
	server.Path = ""
	return server, nil
}

// serveRelay は画面と MCP の 2 つの loopback の待ち受けを開き、停止の合図まで答える。
func serveRelay(
	server *url.URL, providers map[core.AssistProvider]assist.Provider, stdout, stderr io.Writer,
) (code int) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	browser, err := listenLoopback()
	if err != nil {
		return reportError(stderr, err)
	}
	mcp, err := listenLoopback()
	if err != nil {
		return reportError(stderr, errors.Join(err, browser.Close()))
	}
	relay, err := assist.New(assist.Config{
		Server: server, Providers: providers, BrowserPort: portOf(browser), MCPPort: portOf(mcp),
		HTTPClient: &http.Client{},
	})
	if err != nil {
		return reportError(stderr, errors.Join(err, browser.Close(), mcp.Close()))
	}
	defer func() {
		if err := relay.Close(); err != nil {
			code = reportError(stderr, err)
		}
	}()
	if _, err := output.Fprintf(stdout, "open http://127.0.0.1:%d/ in the browser\n", portOf(browser)); err != nil {
		return reportError(stderr, err)
	}
	failures := make(chan error, 2)
	for _, pair := range []struct {
		listener net.Listener
		handler  http.Handler
	}{{browser, relay.BrowserHandler()}, {mcp, relay.MCPHandler()}} {
		go func() { failures <- serve(ctx, pair.listener, pair.handler) }()
	}
	var problems []error
	for range 2 {
		if err := <-failures; err != nil {
			problems = append(problems, err)
			stop()
		}
	}
	if len(problems) > 0 {
		return reportError(stderr, errors.Join(problems...))
	}
	return 0
}

// listenLoopback は loopback の address の無作為な port で待ち受ける。
func listenLoopback() (net.Listener, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listening on the loopback address: %w", err)
	}
	return listener, nil
}

func portOf(listener net.Listener) int {
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}
	return address.Port
}

// serve は listener で handler に答え、ctx の取り消しで止まる。
func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving http: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down the relay: %w", err)
	}
	return nil
}

func reportError(stderr io.Writer, args ...any) int {
	if _, err := output.Fprintln(stderr, args...); err != nil {
		return 1
	}
	return 1
}
