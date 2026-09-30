package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/accountsdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const usage = "usage: oraculum-server [" + addrFlag + " <host:port>] [" + allowedHostFlag + " <host:port>]... [" +
	accountsFlag + " <file>] [" + adminFlag + " <login>] [" + investigationFlag + " <dir>] [" +
	sourceRootFlag + " <dir>] [" + frontendDirFlag + " <dir>] [" + logonSessionLimitFlag + " <duration>] " +
	sourceargs.SigmaUsage + " " +
	sourceargs.AttackRulesUsage +
	" [" + sourceargs.Usage + "]"

// logonSessionLimitFlag は、ログオンのセッションが続く最長の時間を渡す flag である。値は Go の
// time.ParseDuration の文字列 (`24h`、`90m`) であり、0 より大きい。渡さない起動は
// pipeline.DefaultLogonSessionLimit を使う。
const logonSessionLimitFlag = "--logon-session-limit"

const defaultAddr = "127.0.0.1:8080"

// addrFlag は待ち受け先を渡す flag である。
const addrFlag = "--addr"

// allowedHostFlag は、別の端末の browser が server を開く `host:port` を渡す flag である。繰り返し
// 渡せる。
//
// server は、要求の Host と、状態を変える要求の Origin が、この値か loopback の名前に一致する
// 要求だけに答える (api.NewBoundaryHandler)。loopback 以外の --addr で待ち受ける起動は、この flag を
// 必要とする。
const allowedHostFlag = "--allowed-host"

// investigationFlag は調査の directory を渡す flag である。
//
// directory に調査が無ければ、渡した収集元を取り込んで調査を作る。あれば開き、記録した収集元を
// 取り込み直す。そのとき渡した収集元は調査に足す。
const investigationFlag = "--investigation"

// sourceRootFlag は、取り込みの指定の相対 path を解決する基準の directory を渡す flag である。
//
// **起動ごとに変わりうる値である。** 収集物を別の場所へ移した調査を開くとき、記録した基準の
// 代わりに使う。渡さない起動は、作成時に起動した directory (記録した基準) を使う。
//
// **要求による読み込みは、この基準の下の file だけを読む**。基準を持たない起動
// (調査も --source-root も渡さない起動) は、要求による読み込みを受け付けない。
const sourceRootFlag = "--source-root"

// frontendDirFlag は、画面の build (`vite build` の出力) の directory を渡す flag である。
//
// **配置する環境ごとに変わる値である。** build を置く場所は運用者が決める。渡した起動は、API と
// 同じ origin で画面を配信する。渡さない起動は API だけに答え、画面は Vite の開発 server が出す。
const frontendDirFlag = "--frontend-dir"

// readHeaderTimeout は要求 header の読み取りに与える上限である。
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout は停止の合図を受けた後、処理中の応答を待つ上限である。
const shutdownTimeout = 5 * time.Second

// options は起動引数から決まる設定である。
type options struct {
	addr string
	// allowedHosts は allowedHostFlag で渡した値である。loopback の名前は含まない。
	allowedHosts []string
	// accounts はアカウントの file の path である。空文字列の起動はアカウントを持たない。
	accounts string
	// admin は、起動のたびに調査の管理者にするログイン名である。空文字列の起動は役割を変えない。
	admin string
	plans []pipeline.SourcePlan
	// investigation は調査の directory である。空文字列の起動は調査を保存しない。
	investigation string
	// sourceRoot は記録した基準の代わりに使う基準の directory である。空文字列は記録した基準を使う。
	sourceRoot string
	// frontendDir は画面の build の directory である。空文字列の起動は画面を配信しない。
	frontendDir string
	// sigmaRules はレコードに当てる Sigma のルールの集合である。nil の起動は当てない。
	sigmaRules *pipeline.SigmaRuleSpec
	// attackRules は外部 ATT&CK rule directory である。空は install default を使う。
	attackRules string
	// logonSessionLimit はログオンのセッションが続く最長の時間である。
	logonSessionLimit time.Duration
}

func run(args []string, stdout, stderr io.Writer) (code int) {
	// 診断ログの出力境界はここだけである。外部由来の文字列は、この境界で無害化してから
	// log と画面に出す。
	slog.SetDefault(output.NewSanitizingLogger(stderr))
	if limit := applyDefaultMemoryLimit(); limit > 0 {
		slog.Info("set the Go memory limit from the cgroup memory.max", "bytes", limit)
	}
	opts, err := parseArgs(args)
	if err != nil {
		return reportUsageError(stderr, err, usage)
	}
	// 取り込みの前に読み、ルールの集合の誤りで取り込みを待たせない。
	attackRuleSet, err := sourceargs.LoadAttackRules(opts.attackRules)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := sourceargs.ReportAttackRules(stdout, attackRuleSet); err != nil {
		return reportError(stderr, err)
	}
	sigmaRules, err := sourceargs.LoadSigmaRules(opts.sigmaRules)
	if err != nil {
		return reportError(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// アカウントの file も取り込みの前に開く。
	var accounts api.Accounts
	if opts.accounts != "" {
		db, err := accountsdb.Open(ctx, opts.accounts)
		if err != nil {
			return reportError(stderr, err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				code = reportError(stderr, err)
			}
		}()
		accounts = accountsPort{db: db}
	}
	sigma := &sigmaResult{rules: sigmaRules}
	layers := pipeline.DefaultGraphLayers()
	layers.Observed = pipeline.ObservedGraphWithLogonSessionLimit(opts.logonSessionLimit)
	parts, err := openServer(ctx, opts, layers, sigma, stderr)
	if err != nil {
		return reportError(stderr, err)
	}
	// **段階の実行を待ってから保存先を閉じる。** 読み込みが調査へ記録している最中に閉じない。
	defer func() {
		stop()
		parts.stages.Wait()
		for _, closer := range parts.closers {
			if err := closer.Close(); err != nil {
				code = reportError(stderr, err)
			}
		}
	}()
	stages, investigation := parts.stages, parts.investigation
	staged, err := api.NewStagedHandler(stages, sigma.current, attackRuleSet)
	if err != nil {
		return reportError(stderr, "initializing the api:", err)
	}
	// 調査の directory を渡さない起動のワークスペースはメモリに置き、停止すると消える。
	workspaces := pipeline.NewWorkspaceStore(nil)
	if investigation != nil {
		workspaces = investigation.Workspaces()
	}
	// hub は server に 1 つであり、ワークスペースの接続中の利用者へ変更と presence を配信する。
	hub := api.NewWorkspaceHub()
	routed := api.NewWorkspaceHandler(staged, workspaces, nil, nil, hub)
	if accounts != nil {
		access, err := prepareAccess(ctx, opts.admin, accounts, investigation)
		if err != nil {
			return reportError(stderr, err)
		}
		routed = api.NewAccessHandler(api.NewWorkspaceHandler(staged, workspaces, access, accounts, hub), access, accounts)
	}
	listener, err := net.Listen("tcp", opts.addr)
	if err != nil {
		return reportError(stderr, "listening on "+strconv.Quote(opts.addr)+":", err)
	}
	// 画面の build も境界の handler の内側に置き、要求の Host を API と同じ条件で確かめる。
	// port 0 の起動でも実際の port で許可する。listen の後に組む。
	handler, err := api.NewBoundaryHandler(
		api.NewSessionHandler(api.NewServerHandler(routed, parts.frontend), accounts),
		allowedHosts(opts.allowedHosts, listener.Addr()))
	if err != nil {
		_ = listener.Close()
		return reportError(stderr, "initializing the server boundary:", err)
	}
	if err := startProcessing(stages, stderr); err != nil {
		return reportError(stderr, "building the graph:", err)
	}
	if err := serve(ctx, listener, handler, stdout); err != nil {
		return reportError(stderr, "serving the api:", err)
	}
	return 0
}

// sigmaResult は、読み込んだ取り込み結果に Sigma のルールを当てた結果を持つ。
//
// **読み込みの段階の終わりに当てる。** 取り込み結果は読み込みを終えるまで無い。当てた結果は
// 処理を終えた後の API が返す。rules が nil の起動はルールを当てない。
type sigmaResult struct {
	rules *pipeline.SigmaRules

	mu         sync.Mutex
	evaluation pipeline.SigmaEvaluation
}

// evaluate は取り込み結果にルールを当て、件数と所要を運用者へ伝える。
func (s *sigmaResult) evaluate(stderr io.Writer, result pipeline.ImportResult) error {
	evaluation, err := sourceargs.EvaluateSigmaRules(stderr, result, s.rules)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluation = evaluation
	return nil
}

// current は最後に当てた結果を返す。まだ当てていないときは、ルールの集合を持たない結果を返す。
func (s *sigmaResult) current() pipeline.SigmaEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evaluation
}

// startProcessing は、起動の時点で読み込みを終えた調査の処理を背景で始める。
//
// **待ち受けを始めた後に組む。** グラフの組み立ては秒の単位で待つため、起動の時点にも最初の
// 要求にも置かない。組み終えるまで、グラフを読む要求は stage_not_ready で答える。
// 読み込みを終えていない起動は、分析者が読み込みと処理を始めるのを待つ。
func startProcessing(stages *pipeline.Stages, stderr io.Writer) error {
	if _, loaded := stages.Loaded(); !loaded {
		_, err := output.Fprintln(stderr, "waiting for the sources to be loaded through the api")
		return err
	}
	if _, err := output.Fprintln(stderr, "building the graph for the default selection"); err != nil {
		return fmt.Errorf("writing the progress line: %w", err)
	}
	return stages.StartProcessing()
}

func parseArgs(args []string) (options, error) {
	sigmaRules, args, err := sourceargs.ParseSigmaRules(args, os.ReadFile)
	if err != nil {
		return options{}, err
	}
	plans, rest, err := sourceargs.Parse(args, os.ReadFile, addrFlag, allowedHostFlag, accountsFlag, adminFlag,
		investigationFlag, sourceRootFlag, frontendDirFlag, logonSessionLimitFlag, sourceargs.AttackRulesFlag)
	if err != nil {
		return options{}, err
	}
	opts := options{addr: defaultAddr, plans: plans, sigmaRules: sigmaRules,
		logonSessionLimit: pipeline.DefaultLogonSessionLimit}
	var limit string
	var attackRules string
	targets := map[string]*string{addrFlag: &opts.addr, accountsFlag: &opts.accounts, adminFlag: &opts.admin,
		investigationFlag: &opts.investigation, sourceRootFlag: &opts.sourceRoot,
		frontendDirFlag: &opts.frontendDir, logonSessionLimitFlag: &limit, sourceargs.AttackRulesFlag: &attackRules}
	seen := map[string]bool{}
	for index := 0; index < len(rest); index++ {
		flag := rest[index]
		target, known := targets[flag]
		if !known && flag != allowedHostFlag {
			return options{}, fmt.Errorf("parsing arguments: unexpected argument %q", flag)
		}
		if seen[flag] && flag != allowedHostFlag {
			return options{}, fmt.Errorf("parsing arguments: %s is given twice", flag)
		}
		index++
		if index == len(rest) || rest[index] == "" {
			return options{}, fmt.Errorf("parsing arguments: %s requires a value", flag)
		}
		seen[flag] = true
		if flag == allowedHostFlag {
			if err := api.CheckAllowedHost(rest[index]); err != nil {
				return options{}, fmt.Errorf("parsing arguments: %s: %w", allowedHostFlag, err)
			}
			opts.allowedHosts = append(opts.allowedHosts, rest[index])
			continue
		}
		*target = rest[index]
	}
	// 別の端末へ公開する起動は、利用者を識別し、分析者の判断を停止の後も保つ。
	if !isLoopbackAddr(opts.addr) &&
		(len(opts.allowedHosts) == 0 || opts.accounts == "" || opts.investigation == "") {
		return options{}, fmt.Errorf("parsing arguments: %s, %s and %s are required when %s is not a loopback address",
			allowedHostFlag, accountsFlag, investigationFlag, addrFlag)
	}
	if opts.admin != "" && opts.accounts == "" {
		return options{}, fmt.Errorf("parsing arguments: %s requires %s", adminFlag, accountsFlag)
	}
	if seen[logonSessionLimitFlag] {
		parsed, err := time.ParseDuration(limit)
		if err != nil || parsed <= 0 {
			return options{}, fmt.Errorf("parsing arguments: %s takes a positive duration such as 24h, got %q",
				logonSessionLimitFlag, limit)
		}
		opts.logonSessionLimit = parsed
	}
	opts.attackRules = attackRules
	// 収集元を渡さない起動は、要求による読み込みの基準の directory か、既存の調査を必要とする。
	// 調査が無いときの基準の有無は openStages が確かめる。
	if len(opts.plans) == 0 && opts.investigation == "" && opts.sourceRoot == "" {
		return options{}, errors.New("parsing arguments: a source, " + sourceRootFlag + " or " +
			investigationFlag + " is required")
	}
	return opts, nil
}

// serverParts は、起動引数から開いた段階と画面の build、停止のときに閉じる保存先である。
type serverParts struct {
	stages *pipeline.Stages
	// investigation は開いた調査である。調査の directory を渡さない起動では nil である。
	investigation *pipeline.Investigation
	// frontend は画面の build を配信する handler である。画面を配信しない起動では nil である。
	frontend http.Handler
	// closers は停止のときに、段階の実行を待ってから閉じる。
	closers []io.Closer
}

// openServer は画面の build と調査の段階を開く。
//
// **画面の build を取り込みの前に開く。** 置き場の誤りで取り込みを待たせない。段階を開けない
// ときは、開いた画面の build を閉じてから失敗を返す。
func openServer(
	ctx context.Context, opts options, layers pipeline.GraphLayers, sigma *sigmaResult, stderr io.Writer,
) (serverParts, error) {
	frontend, frontendRoot, err := openFrontend(opts.frontendDir)
	if err != nil {
		return serverParts{}, err
	}
	stages, investigation, root, err := openStages(ctx, opts, layers, sigma, stderr)
	if err != nil {
		if frontendRoot != nil {
			err = errors.Join(err, frontendRoot.Close())
		}
		return serverParts{}, err
	}
	parts := serverParts{stages: stages, investigation: investigation, frontend: frontend}
	if frontendRoot != nil {
		parts.closers = append(parts.closers, frontendRoot)
	}
	if root != nil {
		parts.closers = append(parts.closers, root)
	}
	if investigation != nil {
		parts.closers = append(parts.closers, investigation)
	}
	return parts, nil
}

// openFrontend は画面の build の directory を開き、画面を配信する handler を返す。dir が空の
// 起動は画面を配信せず、handler と root はどちらも nil である。返した root は呼び出し元が閉じる。
func openFrontend(dir string) (http.Handler, *os.Root, error) {
	if dir == "" {
		return nil, nil, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening the frontend build %q: %w", dir, err)
	}
	handler, err := api.NewFrontendHandler(root)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("serving the frontend build %q: %w", dir, err), root.Close())
	}
	return handler, root, nil
}

// isLoopbackAddr は、--addr の値が loopback の address だけで待ち受けるかを返す。`:8080`、`0.0.0.0`、
// `[::]` は全ての interface で待ち受けるので偽である。
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// allowedHosts は、渡した値に、実際の待ち受け port の loopback の名前と、待ち受けた address を足す。
// 同じ端末の browser はどの --addr の起動でも loopback の名前で開け、`127.0.0.2` のような
// address を指定した起動はその address でも開ける。
func allowedHosts(given []string, listening net.Addr) []string {
	address, port, err := net.SplitHostPort(listening.String())
	if err != nil {
		return given
	}
	hosts := append([]string{}, given...)
	names := []string{"127.0.0.1", "localhost", "::1"}
	if ip := net.ParseIP(address); ip != nil && !ip.IsUnspecified() {
		names = append(names, address)
	}
	for _, name := range names {
		hosts = append(hosts, net.JoinHostPort(name, port))
	}
	return hosts
}

// serve は取り込み結果を公開し、停止の合図まで応答を続ける。
func serve(ctx context.Context, listener net.Listener, handler http.Handler, stdout io.Writer) error {
	addr := listener.Addr().String()
	// port に 0 を指定した起動でも実際の待ち受け先を運用者へ伝える。
	if _, err := output.Fprintf(stdout, "listening on %s\n", addr); err != nil {
		return fmt.Errorf("writing the listening address: %w", err)
	}
	// **要求の context を停止の合図で取り消す。** グラフの組み立てを待っている要求が、
	// 停止の上限 shutdownTimeout を超えて残らない。
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
		return fmt.Errorf("serving http on %q: %w", addr, err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down the server on %q: %w", addr, err)
	}
	return nil
}

// reportImport は収集元ごとの公開状態と件数を運用者へ伝える。
func reportImport(stderr io.Writer, plans []pipeline.SourcePlan, result pipeline.ImportResult) error {
	statuses := result.Statuses()
	if len(statuses) != len(plans) {
		return fmt.Errorf("%d statuses for %d plans", len(statuses), len(plans))
	}
	// 画面と同じ表示名で書く。同じ file 名の収集元を行で区別する。
	plans = pipeline.DistinguishFileNames(plans)
	for i, status := range statuses {
		var summary strings.Builder
		for _, category := range []core.ImportCategory{core.ImportCategoryRead, core.ImportCategorySucceeded, core.ImportCategoryFailed} {
			if count, ok := status.Counts.Count(category); ok {
				summary.WriteString(" " + string(category) + "=" + strconv.FormatInt(count, 10))
			}
		}
		if _, err := output.Fprintf(stderr, "%s %s%s\n", plans[i].FileName, status.PublicationState, summary.String()); err != nil {
			return fmt.Errorf("writing the summary line of %q: %w", plans[i].FileName, err)
		}
	}
	return nil
}

func reportError(stderr io.Writer, args ...any) int {
	if _, err := output.Fprintln(stderr, args...); err != nil {
		return 1
	}
	return 1
}

// reportUsageError は、無害化した理由の次の行に usage を出す。usage はこの package の定数で、
// 改行を含めてそのまま出す。
func reportUsageError(stderr io.Writer, err error, usageText string) int {
	reportError(stderr, err.Error())
	_, _ = io.WriteString(stderr, usageText+"\n")
	return 1
}
