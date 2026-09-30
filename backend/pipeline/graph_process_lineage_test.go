// in-package test: 非公開の構築子で取り込み結果を組み、分析者の割当を足したグラフを検査する。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// auditd の収集元。
const (
	// lineageSource は shell が 2 つのコマンドを実行し、その後で同じ番号が別の
	// プロセスへ再利用される流れである。
	//
	// **各事象が EXECVE の行を持つ。** 区間の始まりの根拠は起動のレコードであり、
	// auditd は実行の事象に EXECVE の行を伴う。
	lineageSource = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"bash\"\n" +
		"type=SYSCALL msg=audit(1000000000.200:2): ppid=1000 pid=1001 comm=\"first\" exe=\"/usr/bin/first\"\n" +
		"type=EXECVE msg=audit(1000000000.200:2): argc=1 a0=\"first\"\n" +
		"type=SYSCALL msg=audit(1000000000.300:3): ppid=1000 pid=1002 comm=\"second\" exe=\"/usr/bin/second\"\n" +
		"type=EXECVE msg=audit(1000000000.300:3): argc=1 a0=\"second\"\n" +
		"type=SYSCALL msg=audit(1000000900.400:4): ppid=1000 pid=1001 comm=\"reused\" exe=\"/usr/bin/reused\"\n" +
		"type=EXECVE msg=audit(1000000900.400:4): argc=1 a0=\"reused\"\n"
)

// 分析者が与える端末の値。
const (
	lineageTerminalId       = "terminal-linux-host"
	lineageTerminalHostname = "linux-host.example.test"
	lineageClientIp         = "198.51.100.114"
)

// 同じプロセス番号が時刻を隔てて 2 回現れる入力で、プロセスのノードが 2 つになる。
func TestProcessLineageSeparatesAReusedPid(t *testing.T) {
	graph := lineageGraph(t)
	pid1001 := processNodesOfPid(t, graph, "1001")
	if len(pid1001) != 2 {
		t.Fatalf("the pid 1001 reaches %d nodes, want 2", len(pid1001))
	}
	if pid1001[0] == pid1001[1] {
		t.Error("both instances of the pid 1001 share one node identifier")
	}
}

// 同じプロセスが複数の事象に現れても、ノードは 1 つである。
func TestProcessLineageKeepsOneNodePerInstance(t *testing.T) {
	// 同じ番号と同じ区間の始まりを持つ 2 つの事象は、1 つのプロセスである。
	// 連番が別で時刻が同じ 2 事象を置く。実行の失敗が並ぶ区間に現れる形である。
	const repeated = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000000.100:1): item=0 name=\"/usr/bin/bash\"\n" +
		"type=SYSCALL msg=audit(1000000000.100:2): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000000.100:2): item=0 name=\"/usr/local/bin/bash\"\n"
	graph := auditdGraph(t, repeated)
	if records := lineageRecordCount(t, repeated); records != 2 {
		t.Fatalf("the fixture holds %d records, want 2", records)
	}
	if nodes := processNodesOfPid(t, graph, "1000"); len(nodes) != 1 {
		t.Errorf("the pid 1000 reaches %d nodes, want 1", len(nodes))
	}
}

// 起動の後に続く事象が、別の時刻に現れても 1 つのノードに入る。
//
// 区間の始まりを事象ごとの時刻にすると、1 つのプロセスの連続した操作が事象の数だけ
// 別のノードに割れる。
func TestProcessLineageKeepsOneNodeAcrossLaterEvents(t *testing.T) {
	const continued = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"bash\"\n" +
		// 起動の後に続く事象。コマンド行を持たず、時刻が別である。
		"type=SYSCALL msg=audit(1000000000.500:2): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000000.500:2): item=0 name=\"/etc/passwd\"\n" +
		"type=SYSCALL msg=audit(1000000001.900:3): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000001.900:3): item=0 name=\"/etc/group\"\n"
	graph := auditdGraph(t, continued)

	if records := lineageRecordCount(t, continued); records != 3 {
		t.Fatalf("the fixture holds %d records, want 3", records)
	}
	nodes := processNodesOfPid(t, graph, "1000")
	if len(nodes) != 1 {
		t.Errorf("the pid 1000 reaches %d nodes, want 1", len(nodes))
	}
}

// 起動を記録していない番号は、最初に観測した時刻で 1 つの区間になる。
func TestProcessLineageOpensOneIntervalWithoutAStart(t *testing.T) {
	const withoutStart = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000000.100:1): item=0 name=\"/etc/passwd\"\n" +
		"type=SYSCALL msg=audit(1000000000.700:2): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=PATH msg=audit(1000000000.700:2): item=0 name=\"/etc/group\"\n"
	graph := auditdGraph(t, withoutStart)

	if nodes := processNodesOfPid(t, graph, "1000"); len(nodes) != 1 {
		t.Errorf("the pid 1000 reaches %d nodes, want 1", len(nodes))
	}
}

// PID の区間のノードは、区間を開いたレコードの時刻を応答に持つ。識別の値の時刻は、その時刻の
// 正規化値である。
func TestProcessIntervalNodeCarriesTheTimeThatOpenedIt(t *testing.T) {
	graph := lineageGraph(t)
	checked := 0
	for index, node := range graph.nodes {
		response := graph.graphNode(index)
		if node.key.Form != core.NodeKeyFormTerminalProcessInterval {
			if response.IntervalStart != nil {
				t.Errorf("the %s node carries the interval start %+v", node.key.Form, response.IntervalStart)
			}
			continue
		}
		checked++
		identity := response.Identity[len(response.Identity)-1].Value
		if response.IntervalStart == nil || response.IntervalStart.Normalized == nil ||
			*response.IntervalStart.Normalized != identity {
			t.Errorf("the node %q carries the interval start %+v, want the time %q of its identity",
				node.id, response.IntervalStart, identity)
		}
		if err := response.Validate(); err != nil {
			t.Errorf("the node %q is invalid: %v", node.id, err)
		}
	}
	if checked == 0 {
		t.Fatal("the graph holds no process interval node")
	}
}

// 生成を記録したプロセスと、生成を見ていないプロセスを、ノードが読み分ける。
func TestProcessLineageMarksTheCreationRecord(t *testing.T) {
	const mixed = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"bash\"\n" +
		// 起動を記録していない番号。収録範囲の先頭より前に起動したプロセスの形である。
		"type=SYSCALL msg=audit(1000000000.300:2): ppid=900 pid=2000 comm=\"old\" exe=\"/usr/bin/old\"\n" +
		"type=PATH msg=audit(1000000000.300:2): item=0 name=\"/etc/passwd\"\n"
	graph := auditdGraph(t, mixed)

	if len(creationRecordsOfPid(t, graph, "1000")) == 0 {
		t.Error("the pid 1000 carries no creation record, want the EXECVE event as its creation")
	}
	if positions := creationRecordsOfPid(t, graph, "2000"); len(positions) != 0 {
		t.Errorf("the pid 2000 carries the creation records %v, want none", positions)
	}
}

// creationRecordsOfPid は、その番号のプロセスのノードが持つ生成のレコードの位置を返す。
func creationRecordsOfPid(t *testing.T, graph Graph, pid string) []int {
	t.Helper()
	for _, node := range graph.nodes {
		if node.key.Form != core.NodeKeyFormTerminalProcessInterval {
			continue
		}
		if len(node.key.Values) < 2 || node.key.Values[1].Value != pid {
			continue
		}
		return node.creationRecords
	}
	t.Fatalf("the graph holds no process node for the pid %s", pid)
	return nil
}

// 親子は時刻の前後で選び、候補として足す。
func TestProcessLineageAddsTheParentAsACandidate(t *testing.T) {
	graph := lineageGraph(t)
	edges := lineageParentChildEdges(graph)
	if len(edges) == 0 {
		t.Fatal("the graph holds no parent and child edge")
	}
	for _, edge := range edges {
		if edge.state != core.RelationStateCandidate {
			t.Errorf("the parent and child edge is %q, want %q",
				edge.state, core.RelationStateCandidate)
		}
	}
	// 3 つの子 (1001 の 2 つの実体と 1002) が shell を親に持つ。
	if len(edges) != 3 {
		t.Errorf("the graph holds %d parent and child edges, want 3", len(edges))
	}
}

// 再利用された番号の子は、その時刻に生きていた親を指す。
func TestProcessLineageChoosesTheParentLiveAtTheEvent(t *testing.T) {
	graph := lineageGraph(t)
	// 親は 1 つの実体である shell だけである。子の 3 件がすべて同じ親を指す。
	parents := make(map[int]struct{})
	for _, edge := range lineageParentChildEdges(graph) {
		parents[edge.source] = struct{}{}
	}
	if len(parents) != 1 {
		t.Errorf("the children reach %d parents, want the single shell instance", len(parents))
	}
}

// 区間の始まりを根拠付けられない親との関係は作らない。
func TestProcessLineageSkipsAParentWithoutItsStart(t *testing.T) {
	// ppid=900 は収録範囲に現れないため、shell の親のエッジは起きない。
	graph := lineageGraph(t)
	for _, edge := range lineageParentChildEdges(graph) {
		sourceValues := graph.nodes[edge.source].key.Values
		if len(sourceValues) > 1 && sourceValues[1].Value == "900" {
			t.Error("the graph holds an edge from a parent whose interval start is unknown")
		}
	}
}

// 利用者が端末を与えていない収集元では、名前不明の端末の範囲でプロセスを区切る。
func TestProcessLineageUsesTheUnknownTerminalOfTheSource(t *testing.T) {
	result := auditdImportResult(t, lineageSource)
	graph := NewGraph(result, AllMatchConditions())
	unknown, _ := core.RecordingTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256)
	nodes := processNodesOfPid(t, graph, "1001")
	if len(nodes) != 2 {
		t.Fatalf("the pid 1001 reaches %d nodes without a specified terminal, want 2", len(nodes))
	}
	for _, id := range nodes {
		if graph.nodes[graph.nodeAt[id]].key.Values[0] != unknown.Values[0] {
			t.Errorf("the process %q is not in the scope of the unknown terminal", id)
		}
	}
}

// lineageGraph は分析者の割当を足した収集元のグラフを返す。
func lineageGraph(t *testing.T) Graph {
	t.Helper()
	return auditdGraph(t, lineageSource)
}

func auditdGraph(t *testing.T, source string) Graph {
	t.Helper()
	result := auditdImportResult(t, source)
	return NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{analystAssignmentFor(t, result)}), AllMatchConditions())
}

// lineageRecordCount は、テスト用の収集元が何件のレコードに分かれたかを返す。
// 1 つのプロセスが複数の事象に現れることを、ノードの数と別に確かめる。
func lineageRecordCount(t *testing.T, source string) int {
	t.Helper()
	result := auditdImportResult(t, source)
	count := 0
	for _, publication := range result.publications {
		count += len(publication.records)
	}
	return count
}

// auditdImportResult は auditd の収集元 1 件を取り込む。
func auditdImportResult(t *testing.T, source string) ImportResult {
	t.Helper()
	scanned := scanIndexSource(t, NewTestParser(AuditdFormatKeyText, nil),
		"linux-host.log", AuditdFormatKeyText, source)
	statuses := []core.ImportStatus{settleStatus(t, scanned, "auditd")}
	result, err := newImportResult([]scannedSource{scanned}, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// analystAssignmentFor は、その収集元の全レコードが 1 つの端末のものであるとする割当を返す。
//
// 適用期間は、収集元の最初と最後のレコードの時刻である。
func analystAssignmentFor(t *testing.T, result ImportResult) core.TerminalAssignment {
	t.Helper()
	if len(result.publications) == 0 {
		t.Fatal("the import result holds no publication")
	}
	publication := result.publications[0]
	records := publication.records
	if len(records) == 0 {
		t.Fatal("the publication holds no record")
	}
	first, last := records[0].ObservedAt, records[len(records)-1].ObservedAt
	if first == nil || last == nil {
		t.Fatal("the records hold no observed time")
	}
	assignment := core.TerminalAssignment{
		ClientIp:             lineageClientIp,
		TerminalId:           lineageTerminalId,
		TerminalHostname:     lineageTerminalHostname,
		SourceId:             publication.status.SourceId,
		SourceContentSha256:  publication.status.Scope.SourceContentSha256,
		AssignmentValidRange: core.TimeRange{From: *first, To: *last},
		Origin:               core.TerminalAssignmentOriginAnalystSupplied,
		Derivation:           "別の端末の ssh の接続先から導いた",
		Author:               "analyst",
		AppliesToSourceId:    publication.status.SourceId,
		BasisRecordRefs: []core.AssertionRecordRef{
			core.NewAssertionRecordRef(records[0].Locator),
		},
	}
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the analyst assignment: %v", err)
	}
	return assignment
}

// processNodesOfPid は、そのプロセス番号を鍵に持つノードの識別子を返す。
func processNodesOfPid(t *testing.T, graph Graph, pid string) []string {
	t.Helper()
	identifiers := make([]string, 0, 2)
	for _, node := range graph.nodes {
		if node.key.Form != core.NodeKeyFormTerminalProcessInterval {
			continue
		}
		// 端末の鍵の値の数は端末の形で変わるため、プロセス番号を語彙の項目で探す。
		if !slices.Contains(node.key.Values,
			core.NodeIdentityValue{Semantic: core.SemanticKeyProcessPid, Value: pid}) {
			continue
		}
		identifiers = append(identifiers, nodeIdOf(node.key))
	}
	return identifiers
}

// lineageParentChildEdges はグラフの中の親子のエッジを返す。
func lineageParentChildEdges(graph Graph) []graphEdge {
	edges := make([]graphEdge, 0, 4)
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindProcessParentChild {
			edges = append(edges, edge)
		}
	}
	return edges
}

// テスト用の収集元が、実装の出力ではなく原文の形から事象の数を持つことを確かめる。
// 1 事象が複数行に分かれるため、事象の鍵の異なりを数える。
func TestLineageSourceHoldsOneRecordPerEvent(t *testing.T) {
	keys := make(map[string]struct{})
	for _, line := range strings.Split(lineageSource, "\n") {
		at := strings.Index(line, "msg=audit(")
		if at < 0 {
			continue
		}
		end := strings.Index(line[at:], ")")
		if end < 0 {
			t.Fatalf("the line carries no event key: %q", line)
		}
		keys[line[at:at+end]] = struct{}{}
	}
	events := len(keys)
	result := auditdImportResult(t, lineageSource)
	records := 0
	for _, publication := range result.publications {
		records += len(publication.records)
	}
	if records != events {
		t.Errorf("the source holds %d records for %d events", records, events)
	}
}
