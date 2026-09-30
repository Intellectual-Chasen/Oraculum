// in-package test: 1 件のレコードが 2 件以上の候補のプロセスを持つとき、そのレコードを候補の間の
// 中継にしないことを確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// relayGraph は、別の収集元の 1 件の接続のレコード r0 (10 秒) に、候補のプロセス first と second
// (どちらも 10 秒の記録) が関連付いたグラフを組む。r0 は接続先 destination を指す。
func relayGraph(t *testing.T) (f *fakeGraph, connection int) {
	f = newFakeGraph(t)
	destination := f.node(core.NodeKindDomain, "destination")
	first, second := f.node(core.NodeKindProcess, "first"), f.node(core.NodeKindProcess, "second")
	connection = f.record(10)
	firstRecord, secondRecord := f.record(10, byProcess(first)), f.record(10, byProcess(second))
	f.g.matchStages = []matchStage{{origin: int32(connection)}}
	for _, candidate := range []struct{ process, record int }{{first, firstRecord}, {second, secondRecord}} {
		match := f.edge(core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate,
			destination, candidate.process, connection, candidate.record)
		f.g.edges[match].ensureBasis().matches = []candidateMatch{{candidate: int32(candidate.record), stage: 0}}
	}
	f.g.nodes[f.g.records[connection].recordNode].evidence = []int{connection}
	return f, connection
}

// 1 件の接続のレコードが持つ 2 つの候補のプロセスの間に、そのレコードを通る経路を作らない。
func TestInfluencePathDoesNotRelayBetweenCandidatesOfOneRecord(t *testing.T) {
	f, _ := relayGraph(t)
	path := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:second"})
	if len(path.Edges) != 0 || !slices.Equal(path.Stops, []core.InfluencePathStop{core.InfluencePathStopNoInfluenceRoute}) {
		edges, _ := pathSummary(path)
		t.Errorf("edges=%v stops=%v, want no route through the shared record", edges, path.Stops)
	}
}

// 候補から入ったレコードの、関連付け以外から出るエッジは残る。候補のプロセスから、そのレコードの
// 接続とログオンの組を通ってログオンのセッションへ届く。要素は、どの候補から入ったかを持つ。
func TestInfluencePathKeepsTheOtherEdgesOfTheSharedRecord(t *testing.T) {
	f, connection := relayGraph(t)
	logon := f.record(12)
	f.pair(f.edge(core.EdgeKindConnectionLogonMatch, core.RelationStateCandidate,
		f.g.records[connection].recordNode, f.g.records[logon].recordNode, connection, logon),
		pairRuleConnectionLogon, connection, logon)
	path := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:r3"})
	edges, _ := pathSummary(path)
	want := []string{"n:first>n:r0#via:n:first same_terminal", "n:r0#via:n:first>n:r3 same_terminal"}
	if !slices.Equal(edges, want) {
		t.Fatalf("edges = %v, want %v", edges, want)
	}
	at := slices.IndexFunc(path.Vertices, func(vertex core.InfluenceVertex) bool { return vertex.Key == "n:r0#via:n:first" })
	if at < 0 || path.Vertices[at].Node.Id != "n:r0" || path.Vertices[at].EnteredFrom == nil ||
		path.Vertices[at].EnteredFrom.Id != "n:first" {
		t.Errorf("vertices = %+v, want the record entered from the first candidate", path.Vertices)
	}
}

// 候補のプロセスから、共有するレコードが指す接続先へ、候補ごとに 1 本ずつ届く。
func TestInfluencePathReachesTheDestinationOnceThroughEachCandidate(t *testing.T) {
	f, _ := relayGraph(t)
	path := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:destination"})
	edges, _ := pathSummary(path)
	want := []string{"n:first>n:r0#via:n:first same_terminal", "n:r0#via:n:first>n:destination same_terminal"}
	if !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
}

// 2 つ以上の端末のタイムスタンプを含む入力でも、分けた要素を通る経路が残り、候補の間の中継は
// 作らない。
func TestInfluencePathSeparatesTheCandidatesAcrossTerminals(t *testing.T) {
	f, connection := relayGraph(t)
	for _, name := range []string{"n:first", "n:second"} {
		for _, record := range f.g.nodes[f.g.nodeAt[name]].evidence {
			if record != connection {
				onTerminal("n:one")(f, record)
			}
		}
	}
	logon := f.record(12, onTerminal("n:two"))
	f.pair(f.edge(core.EdgeKindConnectionLogonMatch, core.RelationStateCandidate,
		f.g.records[connection].recordNode, f.g.records[logon].recordNode, connection, logon),
		pairRuleConnectionLogon, connection, logon)
	path := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:r3"})
	edges, _ := pathSummary(path)
	want := []string{"n:first>n:r0#via:n:first unbounded_offset", "n:r0#via:n:first>n:r3 unbounded_offset"}
	if !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
	relay := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:second"})
	if len(relay.Edges) != 0 {
		relayEdges, _ := pathSummary(relay)
		t.Errorf("edges = %v, want no relay across the terminals", relayEdges)
	}
}

// 候補を共有するレコードを終点と起点に選べる。
func TestInfluencePathEndsAndStartsAtTheSharedRecord(t *testing.T) {
	f, _ := relayGraph(t)
	into := requirePath(t, f, InfluencePathQuery{From: "n:first", To: "n:r0"})
	if edges, _ := pathSummary(into); !slices.Equal(edges, []string{"n:first>n:r0 same_terminal"}) {
		t.Errorf("edges into the record = %v", edges)
	}
	out := requirePath(t, f, InfluencePathQuery{From: "n:r0", To: "n:second"})
	if edges, _ := pathSummary(out); !slices.Equal(edges, []string{"n:r0>n:second same_terminal"}) {
		t.Errorf("edges out of the record = %v", edges)
	}
}

// 1 つの候補から入った要素が経路に乗るとき、同じレコードの別の候補から入った要素を frontier に
// 出さない。接続のレコード r0 は送受信の量を持つ期間の記録であり、候補の記録の時刻で入る時刻が
// 分かれる。
func TestInfluencePathOmitsTheFrontierOfARecordOnThePath(t *testing.T) {
	f := newFakeGraph(t)
	destination := f.node(core.NodeKindDomain, "destination")
	root := f.node(core.NodeKindProcess, "root")
	first, second := f.node(core.NodeKindProcess, "first"), f.node(core.NodeKindProcess, "second")
	connection := f.record(10, withField(core.SemanticKeyConnectionSentBytes, "1"))
	f.g.matchStages = []matchStage{{origin: int32(connection)}}
	for _, candidate := range []struct{ process, seconds int }{{first, 5}, {second, 9}} {
		record := f.record(candidate.seconds, byProcess(candidate.process))
		f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, root, candidate.process,
			f.record(2, byProcess(candidate.process)))
		match := f.edge(core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate,
			destination, candidate.process, connection, record)
		f.g.edges[match].ensureBasis().matches = []candidateMatch{{candidate: int32(record), stage: 0}}
	}
	f.g.nodes[root].evidence = []int{f.record(1, byProcess(root))}
	logon := f.record(7)
	f.pair(f.edge(core.EdgeKindConnectionLogonMatch, core.RelationStateCandidate,
		f.g.records[connection].recordNode, f.g.records[logon].recordNode, connection, logon),
		pairRuleConnectionLogon, connection, logon)
	path := requirePath(t, f, InfluencePathQuery{
		From: "n:root", To: f.g.nodes[f.g.records[logon].recordNode].id,
		Excluded: []core.InfluenceBasis{core.InfluenceBasisRequestedDestination},
	})
	edges, _ := pathSummary(path)
	if !slices.Contains(edges, "n:r0#via:n:first>"+f.g.nodes[f.g.records[logon].recordNode].id+" same_terminal") {
		t.Fatalf("edges = %v, want the route through the first candidate", edges)
	}
	for _, item := range path.Frontier {
		if strings.HasPrefix(item.Key, "n:r0") {
			t.Errorf("frontier = %v, want no element of the record on the path", path.Frontier)
		}
	}
}

// 2 つの候補から入ったレコードがどちらも先へ進まないとき、同じノードの要素を frontier に 1 つに
// まとめる。
func TestInfluencePathMergesTheFrontierOfTheSharedRecord(t *testing.T) {
	f, _ := relayGraph(t)
	root, third := f.node(core.NodeKindProcess, "root"), f.node(core.NodeKindProcess, "third")
	f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, root, f.g.nodeAt["n:first"],
		f.record(5, byProcess(f.g.nodeAt["n:first"])))
	f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, root, f.g.nodeAt["n:second"],
		f.record(6, byProcess(f.g.nodeAt["n:second"])))
	f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, root, third, f.record(30, byProcess(third)))
	f.g.nodes[root].evidence = []int{f.record(1, byProcess(root))}
	path := requirePath(t, f, InfluencePathQuery{
		From: "n:root", To: "n:third", Excluded: []core.InfluenceBasis{core.InfluenceBasisRequestedDestination},
	})
	var keys []string
	for _, item := range path.Frontier {
		keys = append(keys, item.Key)
	}
	if want := []string{"n:r0#via:n:first"}; !slices.Equal(keys, want) || path.FrontierCount != 1 {
		t.Errorf("frontier = %v (%d), want %v", keys, path.FrontierCount, want)
	}
}
