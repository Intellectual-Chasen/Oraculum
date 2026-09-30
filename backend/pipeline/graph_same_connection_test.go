// in-package test: Windows イベントログの XML と分析者の割当からグラフを組み、
// 同じ接続を記録したレコードの組の候補を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sameConnectionDocuments は、端末 ws.example.test (192.0.2.10) と dc.example.test (192.0.2.1) の
// file を返す。
//
// ws の 601 と 602 は 192.0.2.10:49152 から 192.0.2.1:445 への同じ接続を 28 秒あけて記録する。
// 603 は接続元 port の違う接続、604 は宛先のアドレスの違う接続、605 は dc の 8080 からの
// 接続を受け付けた記録である。dc の 701 は 601 を受け付けた記録、702 は宛先の port の違う
// 受け付け、703 は 10 分後に同じ port を使い直した受け付け、704 は 192.0.2.10:49152 からの
// ログオン (宛先の port を持たない)、705 は 192.0.2.10:49152 から別の宛先への外向きの記録、
// 706 は ws の 8080 へ向かう接続である。
func sameConnectionDocuments() (string, string) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "600", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		connectionXML("ws.example.test", "2001-02-03T04:10:02Z", "601", "4",
			"192.0.2.10", "49152", "192.0.2.1", "445", true) +
		connectionXML("ws.example.test", "2001-02-03T04:10:30Z", "602", "4",
			"192.0.2.10", "49152", "192.0.2.1", "445", true) +
		connectionXML("ws.example.test", "2001-02-03T04:10:02Z", "603", "4",
			"192.0.2.10", "49153", "192.0.2.1", "445", true) +
		connectionXML("ws.example.test", "2001-02-03T04:10:02Z", "604", "4",
			"192.0.2.10", "49152", "198.51.100.7", "445", true) +
		connectionXML("ws.example.test", "2001-02-03T04:12:00Z", "605", "4",
			"192.0.2.10", "8080", "192.0.2.1", "50000", false) +
		processCreationXML("ws.example.test", "2001-02-03T04:30:00Z", "606", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
	server := "<Events>\n" +
		processCreationXML("dc.example.test", "2001-02-03T04:00:00Z", "700", "0x20", "0x1",
			`C:\Example\dc-service.exe`, "dc-service.exe") +
		connectionXML("dc.example.test", "2001-02-03T04:10:01Z", "701", "4",
			"192.0.2.1", "445", "192.0.2.10", "49152", false) +
		connectionXML("dc.example.test", "2001-02-03T04:10:01Z", "702", "4",
			"192.0.2.1", "3389", "192.0.2.10", "49152", false) +
		connectionXML("dc.example.test", "2001-02-03T04:20:02Z", "703", "4",
			"192.0.2.1", "445", "192.0.2.10", "49152", false) +
		logonXML("dc.example.test", "2001-02-03T04:10:01Z", "704", "4624", "192.0.2.10", "3") +
		connectionXML("dc.example.test", "2001-02-03T04:10:01Z", "705", "4",
			"192.0.2.10", "49152", "203.0.113.5", "445", true) +
		connectionXML("dc.example.test", "2001-02-03T04:12:01Z", "706", "4",
			"192.0.2.1", "50000", "192.0.2.10", "8080", true) +
		processCreationXML("dc.example.test", "2001-02-03T04:30:00Z", "707", "0x21", "0x20",
			`C:\Example\dc-tool.exe`, "dc-tool.exe") +
		"</Events>\n"
	return client, server
}

// sameConnectionPairs は、同じ接続の候補のエッジの起点と終点の組を返す。
func sameConnectionPairs(t *testing.T, graph Graph) map[[2]string]bool {
	t.Helper()
	pairs := make(map[[2]string]bool)
	for _, edge := range graph.Query(GraphQuery{
		EdgeKinds: []core.EdgeKind{core.EdgeKindSameConnectionMatch}, Depth: 1,
		Granularity: core.GraphGranularityRecord,
	}).Edges {
		if edge.State != core.RelationStateCandidate || edge.EvidenceCount != 2 {
			t.Errorf("the relation is %q with %d evidence records, want a candidate with 2",
				edge.State, edge.EvidenceCount)
		}
		pairs[[2]string{edge.SourceNodeId, edge.TargetNodeId}] = true
	}
	return pairs
}

// 同じ端末の 2 件は時刻の早い記録から遅い記録へ、端末をまたぐ組は接続した側から受け付けた側へ
// 結ばれ、601・602・701 が 1 本の接続を指す。port の違う組、宛先のアドレスの違う組、
// 時刻の離れた組、宛先の port を持たないログオンは結ばない。
func TestSameConnectionMatchJoinsTheRecordsOfOneConnection(t *testing.T) {
	client, server := sameConnectionDocuments()
	result := windowsEventSessionResult(t, client, server)
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
		sessionAssignment(t, result, 1, "192.0.2.1", "dc-1", "", true),
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	ws := func(id string) string { return recordNodeIdOf(t, result, 0, id) }
	dc := func(id string) string { return recordNodeIdOf(t, result, 1, id) }
	want := map[[2]string]bool{
		{ws("601"), ws("602")}: true,
		{ws("601"), dc("701")}: true,
		{ws("602"), dc("701")}: true,
		{dc("706"), ws("605")}: true,
	}
	got := sameConnectionPairs(t, graph)
	if len(got) != len(want) {
		t.Fatalf("the graph carries %d same connection relations, want %d: %v", len(got), len(want), got)
	}
	for pair := range want {
		if !got[pair] {
			t.Errorf("the graph carries no relation from %q to %q", pair[0], pair[1])
		}
	}
	onTerminal := []core.EdgePairConditionKey{core.EdgePairConditionSourceEndpoint,
		core.EdgePairConditionDestinationEndpoint, core.EdgePairConditionTerminal, core.EdgePairConditionTimeProximity}
	acrossTerminals := []core.EdgePairConditionKey{core.EdgePairConditionSourceEndpoint,
		core.EdgePairConditionDestinationEndpoint, core.EdgePairConditionDestinationTerminal,
		core.EdgePairConditionTimeProximity}
	for _, ends := range edgePairsOfKind(graph, core.EdgeKindSameConnectionMatch) {
		want := acrossTerminals
		if ends == [2]int{recordNodeAtOf(t, graph, ws("601")), recordNodeAtOf(t, graph, ws("602"))} {
			want = onTerminal
		}
		detail := edgeDetailOfKind(t, graph, core.EdgeKindSameConnectionMatch, ends[0], ends[1])
		for _, pair := range detail.RecordPairs {
			if keys := conditionKeysOf(pair); !slices.Equal(keys, want) {
				t.Errorf("a pair carries the conditions %v, want %v", keys, want)
			}
		}
	}
}

// recordNodeAtOf は、識別子 id のノードの位置を返す。
func recordNodeAtOf(t *testing.T, graph Graph, id string) int {
	t.Helper()
	for at, node := range graph.nodes {
		if nodeIdOf(node.key) == id {
			return at
		}
	}
	t.Fatalf("the graph carries no node %s", id)
	return 0
}

// 宛先のアドレスに割当が無いと、端末をまたぐ組は受け付けた側の端末を決められず結ばない。
// 同じ端末の組は割当に依らず結ぶ。
func TestSameConnectionMatchNeedsTheAssignmentOfTheDestination(t *testing.T) {
	client, server := sameConnectionDocuments()
	result := windowsEventSessionResult(t, client, server)
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "", "ws-1", "", true),
		sessionAssignment(t, result, 1, "", "dc-1", "", true),
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	got := sameConnectionPairs(t, graph)
	want := [2]string{recordNodeIdOf(t, result, 0, "601"), recordNodeIdOf(t, result, 0, "602")}
	if len(got) != 1 || !got[want] {
		t.Errorf("the graph carries the same connection relations %v, want only 601 to 602", got)
	}
}

// markiiConnectionLine は、 markii 形式の通信のレコード 1 行を返す。
func markiiConnectionLine(clock, sequence, subEvent, sourcePort string) string {
	return "01/02/2024 " + clock + ".000 +0000 sn=" + sequence + " evt=net subEvt=" + subEvent +
		" psGUID=p psID=12 tmid=t com=c csid=s psPath=app srcIP=192.0.2.10 srcPort=" + sourcePort +
		" dstIP=192.0.2.1 dstPort=445\n"
}

// markiiEndpointLine は、接続元 source と宛先 destination (どちらも "アドレス:port") を与えた
// markii 形式の通信のレコード 1 行を返す。
func markiiEndpointLine(clock, sequence, subEvent, source, destination string) string {
	sourceAddress, sourcePort, _ := strings.Cut(source, ":")
	destinationAddress, destinationPort, _ := strings.Cut(destination, ":")
	return "01/02/2024 " + clock + ".000 +0000 sn=" + sequence + " evt=net subEvt=" + subEvent +
		" psGUID=p psID=4 tmid=t com=c csid=s psPath=app srcIP=" + sourceAddress + " srcPort=" + sourcePort +
		" dstIP=" + destinationAddress + " dstPort=" + destinationPort + "\n"
}

// 受け付けた側の acpt は、srcIP に相手、dstIP に自端末を書く。受け付けた側の dcon の向きを
// 仕様が定めないため、acpt を開閉を読めない記録として扱い、同じ向きの 4 つ組の dcon と時刻の
// 許容幅の中でだけ結ぶ。1 時間後の dcon と、向きを入れ替えた 4 つ組の dcon は結ばない。
func TestSameConnectionMatchTreatsTheAcceptAsUnphased(t *testing.T) {
	document := markiiEndpointLine("03:00:00", "1", "acpt", "192.0.2.20:51000", "192.0.2.10:445") +
		markiiEndpointLine("03:00:30", "2", "dcon", "192.0.2.20:51000", "192.0.2.10:445") +
		markiiEndpointLine("04:00:00", "3", "acpt", "192.0.2.20:51001", "192.0.2.10:445") +
		markiiEndpointLine("05:00:00", "4", "dcon", "192.0.2.20:51001", "192.0.2.10:445") +
		markiiEndpointLine("06:00:00", "5", "acpt", "192.0.2.20:51002", "192.0.2.10:445") +
		markiiEndpointLine("06:00:10", "6", "dcon", "192.0.2.10:445", "192.0.2.20:51002")
	got := markiiSameConnectionPairs(t, document)
	if want := []string{"1->2"}; !slices.Equal(got, want) {
		t.Errorf("the same connection relations are %v, want %v", got, want)
	}
}

// markiiSameConnectionPairs は、markii 形式の document を取り込み、同じ接続の候補のエッジの起点と
// 終点の通番の組を返す。
func markiiSameConnectionPairs(t *testing.T, document string) []string {
	t.Helper()
	result := sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document})
	graph := NewGraph(result, AllMatchConditions())
	var got []string
	for _, ends := range edgePairsOfKind(graph, core.EdgeKindSameConnectionMatch) {
		got = append(got, recordLabel(t, graph, ends[0])+"->"+recordLabel(t, graph, ends[1]))
	}
	slices.Sort(got)
	return got
}

// 開閉を読める記録は、閉じた記録を直近の開いた記録へ結ぶ。1 時間続いた接続も結び、同じ 4 つ組で
// 開き直した接続は別の組になる。開いた記録の無い閉じた記録と、閉じた記録の無い開いた記録は結ばない。
func TestSameConnectionMatchJoinsTheCloseToTheLatestOpen(t *testing.T) {
	document := markiiConnectionLine("03:00:00", "1", "con", "49152") +
		markiiConnectionLine("04:00:00", "2", "dcon", "49152") +
		markiiConnectionLine("04:10:00", "3", "con", "49152") +
		markiiConnectionLine("05:30:00", "4", "dcon", "49152") +
		markiiConnectionLine("06:00:00", "5", "dcon", "49152") +
		markiiConnectionLine("06:10:00", "6", "con", "49153")
	got := markiiSameConnectionPairs(t, document)
	if want := []string{"1->2", "3->4"}; !slices.Equal(got, want) {
		t.Errorf("the same connection relations are %v, want %v", got, want)
	}
}

// 開閉を読める con → dcon の組は、接続元・宛先・端末・時刻の順の条件を並びのまま運び、時刻の
// 許容幅の条件を持たない。
func TestSameConnectionMatchRecordsTheTimeOrderForPhasedRecords(t *testing.T) {
	document := markiiConnectionLine("03:00:00", "1", "con", "49152") +
		markiiConnectionLine("04:00:00", "2", "dcon", "49152")
	result := sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document})
	graph := NewGraph(result, AllMatchConditions())
	requireRecordPairs(t, graph, core.EdgeKindSameConnectionMatch, core.EdgePairConditionSourceEndpoint,
		core.EdgePairConditionDestinationEndpoint, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder)
	for _, ends := range edgePairsOfKind(graph, core.EdgeKindSameConnectionMatch) {
		detail := edgeDetailOfKind(t, graph, core.EdgeKindSameConnectionMatch, ends[0], ends[1])
		for _, pair := range detail.RecordPairs {
			if slices.Contains(conditionKeysOf(pair), core.EdgePairConditionTimeProximity) {
				t.Errorf("the con to dcon pair carries the time proximity condition")
			}
		}
	}
}

// 開閉を読めない記録 (est) は、同じ端末の組でも時刻の許容幅で結ぶ。
func TestSameConnectionMatchKeepsTheToleranceForUnphasedRecords(t *testing.T) {
	document := markiiConnectionLine("03:00:00", "1", "est", "49152") +
		markiiConnectionLine("03:00:30", "2", "dcon", "49152") +
		markiiConnectionLine("04:00:00", "3", "dcon", "49152")
	got := markiiSameConnectionPairs(t, document)
	if want := []string{"1->2"}; !slices.Equal(got, want) {
		t.Errorf("the same connection relations are %v, want %v", got, want)
	}
}

// 接続を記録したレコードが無い入力は、同じ接続の候補を作らない。
func TestSameConnectionMatchNeedsConnectionRecords(t *testing.T) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "600", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		"</Events>\n"
	result := windowsEventSessionResult(t, client)
	graph := NewGraph(result, AllMatchConditions())
	if got := sameConnectionPairs(t, graph); len(got) != 0 {
		t.Errorf("the graph carries %d same connection relations, want none", len(got))
	}
}
