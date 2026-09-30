// in-package test: 非公開の parseArgs と serve に引数と出力先を注入する。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// runFixtureDir は取り込みの fixture を集約した場所である。
const runFixtureDir = "../../internal/testdata/run"

// fixtureManifest は fixture と一緒に置いた期待値である。
type fixtureManifest struct {
	File      string                `json:"file"`
	Format    core.FormatKey        `json:"format"`
	Digest    string                `json:"digest"`
	Read      int64                 `json:"read"`
	Succeeded int64                 `json:"succeeded"`
	Failed    int64                 `json:"failed"`
	State     core.PublicationState `json:"state"`
}

func readFixtureManifest(t *testing.T) map[string]fixtureManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runFixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []fixtureManifest
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	byFile := make(map[string]fixtureManifest, len(entries))
	for _, entry := range entries {
		byFile[entry.File] = entry
	}
	return byFile
}

func TestParseArgsAcceptsSourcesAndAddress(t *testing.T) {
	opts, err := parseArgs([]string{"squid_combined:logs/access.log", "--addr", "127.0.0.1:0", "infotrace_mark_ii:logs/endpoint.log"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.addr != "127.0.0.1:0" {
		t.Fatalf("addr=%q", opts.addr)
	}
	want := []pipeline.SourcePlan{
		{FormatKey: squid.FormatKeyCombined, OriginPath: "logs/access.log", FileName: "access.log"},
		{FormatKey: markii.FormatKeyClientLog, OriginPath: "logs/endpoint.log", FileName: "endpoint.log"},
	}
	if len(opts.plans) != len(want) {
		t.Fatalf("plans=%+v", opts.plans)
	}
	for i, plan := range opts.plans {
		if plan != want[i] {
			t.Fatalf("plan %d=%+v want=%+v", i, plan, want[i])
		}
	}
}

func TestParseArgsUsesTheDefaultAddress(t *testing.T) {
	opts, err := parseArgs([]string{"squid_combined:logs/access.log"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.addr != defaultAddr {
		t.Fatalf("addr=%q want=%q", opts.addr, defaultAddr)
	}
	if opts.logonSessionLimit != pipeline.DefaultLogonSessionLimit {
		t.Fatalf("logonSessionLimit=%v want=%v", opts.logonSessionLimit, pipeline.DefaultLogonSessionLimit)
	}
}

// ログオンのセッションが続く最長の時間は、flag の Go の時間の文字列で変えられる。
func TestParseArgsReadsTheLogonSessionLimit(t *testing.T) {
	opts, err := parseArgs([]string{"squid_combined:logs/access.log", "--logon-session-limit", "90m"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.logonSessionLimit != 90*time.Minute {
		t.Fatalf("logonSessionLimit=%v want=90m", opts.logonSessionLimit)
	}
}

func TestParseArgsReadsTheAttackRuleDirectory(t *testing.T) {
	directory := filepath.Join("testdata", "attack-rules")
	opts, err := parseArgs([]string{"--attack-rules", directory, "squid_combined:logs/access.log"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.attackRules != directory {
		t.Fatalf("attackRules=%q want=%q", opts.attackRules, directory)
	}
}

// --allowed-host は繰り返し渡せ、loopback 以外で待ち受ける起動を受け付ける。
func TestParseArgsReadsRepeatedAllowedHosts(t *testing.T) {
	opts, err := parseArgs([]string{"squid_combined:logs/access.log", "--addr", "0.0.0.0:8080",
		"--allowed-host", "192.0.2.10:8080", "--allowed-host", "oraculum.example:8080",
		"--accounts", "accounts.sqlite", "--investigation", "inv"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(opts.allowedHosts, " ") != "192.0.2.10:8080 oraculum.example:8080" ||
		opts.accounts != "accounts.sqlite" {
		t.Fatalf("allowedHosts=%q accounts=%q", opts.allowedHosts, opts.accounts)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080": true, "localhost:8080": true, "[::1]:8080": true,
		":8080": false, "0.0.0.0:8080": false, "[::]:8080": false, "192.0.2.10:8080": false,
		"not-an-address": false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q)=%v want=%v", addr, got, want)
		}
	}
}

// 許可する host は、渡した値と、実際の待ち受け port の loopback の名前である。
func TestAllowedHostsAddsTheLoopbackNamesOfTheListeningPort(t *testing.T) {
	got := allowedHosts([]string{"192.0.2.10:8080"}, &net.TCPAddr{IP: net.IPv4zero, Port: 41234})
	want := "192.0.2.10:8080 127.0.0.1:41234 localhost:41234 [::1]:41234"
	if strings.Join(got, " ") != want {
		t.Fatalf("allowedHosts=%q want=%q", got, want)
	}
}

// 127.0.0.1 以外の loopback の address で待ち受けた起動は、その address でも開ける。
func TestAllowedHostsAddsTheListeningAddress(t *testing.T) {
	got := allowedHosts(nil, &net.TCPAddr{IP: net.ParseIP("127.0.0.2"), Port: 41234})
	want := "127.0.0.1:41234 localhost:41234 [::1]:41234 127.0.0.2:41234"
	if strings.Join(got, " ") != want {
		t.Fatalf("allowedHosts=%q want=%q", got, want)
	}
}

func TestParseArgsRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--addr", "127.0.0.1:0"},
		{"squid_combined:logs/access.log", "--addr"},
		{"missing-colon"},
		{":logs/access.log"},
		{"squid_combined:"},
		{"squid_combined:/absolute/access.log"},
		{"squid_combined:logs/access.log", "--logon-session-limit", "a day"},
		{"squid_combined:logs/access.log", "--logon-session-limit", "0s"},
		// 全ての interface で待ち受ける起動は、別の端末が開く名前を必要とする。
		{"squid_combined:logs/access.log", "--addr", ":8080"},
		{"squid_combined:logs/access.log", "--addr", "0.0.0.0:8080"},
		{"squid_combined:logs/access.log", "--addr", "[::]:8080"},
		{"squid_combined:logs/access.log", "--addr", "192.0.2.10:8080"},
		{"squid_combined:logs/access.log", "--allowed-host"},
		// 別の端末へ公開する起動は、アカウントと調査の保存先を必要とする。
		{"squid_combined:logs/access.log", "--addr", "0.0.0.0:8080", "--allowed-host", "192.0.2.10:8080",
			"--investigation", "inv"},
		{"squid_combined:logs/access.log", "--addr", "0.0.0.0:8080", "--allowed-host", "192.0.2.10:8080",
			"--accounts", "accounts.sqlite"},
		{"squid_combined:logs/access.log", "--accounts"},
		// 許可する host の文字列の誤りは、調査を開く前に退ける。
		{"squid_combined:logs/access.log", "--allowed-host", "::1"},
		{"squid_combined:logs/access.log", "--allowed-host", "http://192.0.2.10:8080"},
		{"squid_combined:logs/access.log", "--allowed-host", "192.0.2.10/path:8080"},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Fatalf("args=%q accepted", args)
		}
	}
}

// importFixtures は fixture を取り込み、公開できる結果を返す。
func importFixtures(t *testing.T, files ...string) pipeline.ImportResult {
	t.Helper()
	manifest := readFixtureManifest(t)
	plans := make([]pipeline.SourcePlan, len(files))
	for i, file := range files {
		entry, ok := manifest[file]
		if !ok {
			t.Fatalf("fixture %q is absent from the manifest", file)
		}
		plans[i] = pipeline.SourcePlan{
			FormatKey:  entry.Format,
			OriginPath: filepath.Join(runFixtureDir, file),
			FileName:   file,
		}
	}
	config, err := importConfig()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestServeAnswersTheSourcesRequestAndShutsDown(t *testing.T) {
	manifest := readFixtureManifest(t)
	result := importFixtures(t, "squid.log", "markii.log")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout bytes.Buffer
	served := make(chan error, 1)
	store := pipeline.NewMemoryStore(result, pipeline.SystemClock{})
	catalog := pipeline.NewGraphCatalog(store, pipeline.DefaultGraphLayers())
	handler, err := api.NewHandler(store, catalog, pipeline.SigmaEvaluation{}, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	go func() { served <- serve(ctx, listener, handler, &stdout) }()

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + address + "/api/v0/sources")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.StatusCode, body)
	}
	var payload struct {
		Sources []struct {
			Source       core.SourceIdentity `json:"source"`
			ImportStatus core.ImportStatus   `json:"importStatus"`
		} `json:"sources"`
		SourceCount int64 `json:"sourceCount"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SourceCount != 2 || len(payload.Sources) != 2 {
		t.Fatalf("payload=%+v", payload)
	}
	for i, file := range []string{"squid.log", "markii.log"} {
		entry := manifest[file]
		identity := payload.Sources[i].Source
		if identity.FileName != file || identity.ContentSha256 != entry.Digest || identity.FormatKey != entry.Format {
			t.Fatalf("identity %d=%+v want file=%q digest=%q", i, identity, file, entry.Digest)
		}
		status := payload.Sources[i].ImportStatus
		if status.PublicationState != entry.State || status.FailureCount != entry.Failed {
			t.Fatalf("status %d=%+v want state=%q failed=%d", i, status, entry.State, entry.Failed)
		}
		for category, want := range map[core.ImportCategory]int64{
			core.ImportCategoryRead:      entry.Read,
			core.ImportCategorySucceeded: entry.Succeeded,
			core.ImportCategoryFailed:    entry.Failed,
		} {
			if got, ok := status.Counts.Count(category); !ok || got != want {
				t.Fatalf("%s of %s=%d present=%t want=%d", category, file, got, ok, want)
			}
		}
	}

	var raw any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	assertSafeIntegers(t, "response", raw)

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve kept running after the context ended")
	}
	if want := "listening on " + address + "\n"; stdout.String() != want {
		t.Fatalf("stdout=%q want=%q", stdout.String(), want)
	}
	afterShutdown, err := client.Get("http://" + address + "/api/v0/sources")
	if err == nil {
		_ = afterShutdown.Body.Close() // 停止した後に応答が返った場合の後始末である。
		t.Fatal("the listener kept accepting connections after the shutdown")
	}
}

func TestServeReportsTheListeningAddressFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// 書き込みに失敗した serve は listener を閉じないため、test が後始末する。
		_ = listener.Close()
	}()
	failed := &rejectedOutput{}
	err = serve(context.Background(), listener, http.NotFoundHandler(), failed)
	if err == nil || !strings.Contains(err.Error(), "writing the listening address") {
		t.Fatalf("err=%v", err)
	}
	if failed.calls != 1 || !strings.HasPrefix(string(failed.received), "listening on ") {
		t.Fatalf("calls=%d received=%q", failed.calls, failed.received)
	}
}

// maxSafeIntegerInJSON は IEEE754 の倍精度が正確に表せる整数の上限である
// (JavaScript の Number.MAX_SAFE_INTEGER と同じ値)。
//
// 実装の定数を参照せずに書く。参照すると、実装が上限を広げたときに期待値が追随して
// 検査が通る。
const maxSafeIntegerInJSON = 9007199254740991

// assertSafeIntegers は応答の整数が IEEE754 の倍精度で表せる範囲に収まることを確かめる。
//
// 画面は JSON.parse で応答を読む。範囲を超える整数は別の値として届き、画面の decoder が
// 応答全体を退ける。
func assertSafeIntegers(t *testing.T, path string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, member := range typed {
			assertSafeIntegers(t, path+"."+key, member)
		}
	case []any:
		for i, member := range typed {
			assertSafeIntegers(t, path+"["+strconv.Itoa(i)+"]", member)
		}
	case json.Number:
		integer, err := typed.Int64()
		if err != nil {
			// 整数として読めない値は対象の外である。
			return
		}
		if integer > maxSafeIntegerInJSON || integer < -maxSafeIntegerInJSON {
			t.Fatalf("%s=%s leaves the safe integer range", path, typed.String())
		}
	}
}

type rejectedOutput struct {
	calls    int
	received []byte
}

func (w *rejectedOutput) Write(data []byte) (int, error) {
	w.calls++
	w.received = append(w.received, data...)
	return 0, errors.New("output unavailable")
}

// restoreDefaultLogger は run が書き換える process 全体の既定 logger を test の終了後に戻す。
// 戻さないと、終了した test の bytes.Buffer に束縛された logger が既定のまま残る。
func restoreDefaultLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func TestRunReportsInvalidArguments(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if stdout.Len() != 0 || !strings.HasSuffix(stderr.String(), "\n"+usage+"\n") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// 理由は無害化して出し、usage は理由の次の行に出す。
func TestRunReportsTheUsageOnItsOwnLine(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"unexpected\x1b"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	reason, rest, found := strings.Cut(stderr.String(), "\n")
	if !found || rest != usage+"\n" || strings.ContainsRune(reason, '\x1b') {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunAccountsReportsTheUsageOnItsOwnLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runAccounts(nil, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.HasSuffix(stderr.String(), "\n"+accountsUsage+"\n") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

// TestRunInstallsTheSanitizingLogger は、run の後の既定の logger が無害化を通し、
// 注入した stderr へ書くことを確かめる。
func TestRunInstallsTheSanitizingLogger(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	stderr.Reset()
	slog.Error("listing sources failed", "error", errors.New("markii.log\nERROR forged"))
	line := stderr.String()
	if strings.Count(line, "\n") != 1 || !strings.Contains(line, `error="markii.log\\x0aERROR forged"`) {
		t.Fatalf("log line = %q", line)
	}
}

func TestRunReportsTheImportFailure(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	args := append(testAttackRuleArgs(), "squid_combined:"+filepath.Join(runFixtureDir, "absent.log"))
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "attack rules: 2 files") || !strings.Contains(stderr.String(), "running import") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunReportsTheListeningFailure(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	args := append(testAttackRuleArgs(), "--addr", "127.0.0.1:70000", "squid_combined:"+filepath.Join(runFixtureDir, "squid.log"))
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "listening on") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "squid.log published_full") {
		t.Fatalf("the import summary is absent: %q", stderr.String())
	}
}

// TestRunReportsTheImportSummaryFailure は要約の書き出しが失敗したとき、run が付ける文脈を固定する。
func TestRunReportsTheImportSummaryFailure(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout bytes.Buffer
	stderr := &rejectedOutput{}
	args := append(testAttackRuleArgs(), "squid_combined:"+filepath.Join(runFixtureDir, "squid.log"))
	if code := run(args, &stdout, stderr); code != 1 {
		t.Fatalf("exit=%d received=%q", code, stderr.received)
	}
	if !strings.Contains(stdout.String(), "attack rules: 2 files") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	// 1 回目が要約の行、2 回目が run の付けた文脈である。
	if stderr.calls != 2 {
		t.Fatalf("calls=%d received=%q", stderr.calls, stderr.received)
	}
	for _, want := range []string{
		"reporting the import summary:",
		`writing the summary line of "squid.log":`,
	} {
		if !strings.Contains(string(stderr.received), want) {
			t.Errorf("received=%q does not carry %q", stderr.received, want)
		}
	}
}

func testAttackRuleArgs() []string {
	return []string{"--attack-rules", filepath.Join("..", "..", "rules", "attack")}
}

func TestReportImportRejectsMismatchedStatuses(t *testing.T) {
	result := importFixtures(t, "squid.log")
	var stderr bytes.Buffer
	err := reportImport(&stderr, nil, result)
	if err == nil || !strings.Contains(err.Error(), "1 statuses for 0 plans") {
		t.Fatalf("err=%v", err)
	}
}
