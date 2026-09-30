package pipeline

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 端末のプロセスの起動と通信 (UTC からのずれを持つ) と、地方時の Proxy の要求である。
// Proxy の時刻を UTC+09:00 として読むと、要求は通信と同じ秒に並び、
// 接続元 IP から端末を探す割当の期間 (端末の収録範囲) に入る。
const (
	interpretedEndpointLog = `02/01/2000 09:00:01.000 +0900 sn=150 evt=ps subEvt=start com="SRV01" ` +
		`tmid=11111111-2222-3333-4444-666666666666 csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`ip=192.0.2.101 psGUID={00000000-0000-0000-0000-000000000002} ` +
		`psPath="C:\Windows\system32\svchost.exe"` + "\n" +
		`02/01/2000 09:00:02.300 +0900 sn=202 evt=net subEvt=con com="SRV01" ` +
		`tmid=11111111-2222-3333-4444-666666666666 csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`ip=192.0.2.101 psGUID={00000000-0000-0000-0000-000000000002} ` +
		`psPath="C:\Windows\system32\svchost.exe" ` +
		`srcIP=192.0.2.101 srcPort=50002 dstIP=203.0.113.21 dstPort=80` + "\n"
	interpretedProxyLog = `2000/02/01 09:00:02.000     10 192.0.2.101 TCP_MISS/200 4096 GET ` +
		`http://203.0.113.21/first - HIER_DIRECT/203.0.113.21 text/html` + "\n"
	interpretedProxySpec = `%{%Y/%m/%d %H:%M:%S}tl.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`
)

// interpretedImport は端末と地方時の Proxy の収集元を取り込み、Proxy の内容の識別を返す。
func interpretedImport(t *testing.T) (ImportResult, string) {
	t.Helper()
	contents := map[string]string{"endpoint.log": interpretedEndpointLog, "proxy.log": interpretedProxyLog}
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(contents[originPath])), nil
		},
		Parsers: NewTestFormatRegistry(), Minter: DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := interpretedProxySpec
	result, err := runner.Run([]SourcePlan{
		{OriginPath: "endpoint.log", FileName: "endpoint.log", FormatKey: MarkIIFormatKey},
		{OriginPath: "proxy.log", FileName: "proxy.log", FormatKey: "squid_logformat", FormatSpec: &spec},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range result.identities {
		if identity.FileName == "proxy.log" {
			return result, identity.ContentSha256
		}
	}
	t.Fatal("the proxy source is not in the import result")
	return ImportResult{}, ""
}

// timeComparedCandidatesOf は、関連付けが挙げた候補のエッジのうち、分析者のずれで読んだ時刻を
// 秒で比べた関連付けを持つものを数える。
func timeComparedCandidatesOf(t *testing.T, graph Graph) int {
	t.Helper()
	count := 0
	for _, edge := range graph.edges {
		if edge.state != core.RelationStateCandidate {
			continue
		}
		table := graph.edgeMatchTableOf(edge)
		for index := range table.Matches {
			match, err := table.Match(index)
			if err != nil {
				t.Fatal(err)
			}
			comparison := match.TimeComparison
			if comparison.ComparisonUnit != core.ComparisonUnitSecond || comparison.LeftTime == nil ||
				comparison.RightTime == nil {
				continue
			}
			if comparison.LeftTime.Interpretation == nil && comparison.RightTime.Interpretation == nil {
				t.Fatalf("the candidate %q compares no interpreted time", edge.id)
			}
			count++
			break
		}
	}
	return count
}

// 地方時の収集元に時刻の解釈を記録すると、時刻を比べた段階の候補のエッジができ、ずれを変えると
// 消え、取り消すと消えたままになる。グラフは catalog が入力の状態ごとに組み直す。
func TestTimeInterpretationAddsAndWithdrawsTimeComparedCandidates(t *testing.T) {
	result, proxyContent := interpretedImport(t)
	store := NewMemoryStore(result, &steppingClock{})
	catalog := NewGraphCatalog(store, DefaultGraphLayers())
	candidates := func() int {
		t.Helper()
		graph, _, err := catalog.Graph(context.Background(), AllMatchConditions())
		if err != nil {
			t.Fatal(err)
		}
		return timeComparedCandidatesOf(t, graph)
	}
	revise := func(id string, state core.AssertionState, offset core.UtcOffset) {
		t.Helper()
		current, _ := store.Assertions().Find(id)
		if _, err := store.Assertions().Revise(id, AssertionRevisionDraft{
			State: state, Author: "analyst-b", Basis: core.AssertionBasis{Note: "合成の根拠"}, TimeOffset: &offset,
			BaseRevision: current.RevisionNumber,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if count := candidates(); count != 0 {
		t.Fatalf("%d time-compared candidates before the interpretation, want 0", count)
	}
	created, err := store.Assertions().Create(sourceDraft(proxyContent, "+09:00"))
	if err != nil {
		t.Fatal(err)
	}
	if count := candidates(); count != 1 {
		t.Fatalf("%d time-compared candidates after +09:00, want 1", count)
	}
	revise(created.Id, core.AssertionStateActive, "+00:00")
	if count := candidates(); count != 0 {
		t.Fatalf("%d time-compared candidates after +00:00, want 0", count)
	}
	revise(created.Id, core.AssertionStateActive, "+09:00")
	if count := candidates(); count != 1 {
		t.Fatalf("%d time-compared candidates after returning to +09:00, want 1", count)
	}
	revise(created.Id, core.AssertionStateWithdrawn, "+09:00")
	if count := candidates(); count != 0 {
		t.Fatalf("%d time-compared candidates after the withdrawal, want 0", count)
	}
}
