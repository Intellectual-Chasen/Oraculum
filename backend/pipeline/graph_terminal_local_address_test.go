// in-package test: ループバックとリンクローカルのアドレスを端末の範囲で識別し、グラフを組む各処理が
// 同じ範囲のノードを探すことを確かめる。
package pipeline

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// localAddressKey は、端末 terminal の範囲に置いたアドレスのノードの識別鍵を組む。
func localAddressKey(terminal core.NodeKey, address string) core.NodeKey {
	return core.NodeKey{
		Kind: core.NodeKindIp, Form: core.NodeKeyFormTerminalAddress,
		Values: append(slices.Clone(terminal.Values), core.NodeIdentityValue{Value: address}),
	}
}

// globalAddressKey は、端末をまたいで 1 つのノードになるアドレスの識別鍵を組む。
func globalAddressKey(address string) core.NodeKey {
	return core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: address}}}
}

// localAddressRecord は、行番号 line の秒に記録したレコードを組む。terminal はレコードが
// 名乗った端末の項目 (RecordEntry.Terminal) である。
func localAddressRecord(t *testing.T, line int64, terminal []core.RecordField, fields ...core.RecordField) RecordEntry {
	t.Helper()
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		ObservedAt: matchTestTime(t, int(line)),
		Terminal:   terminal,
		Semantics: &RecordSemantics{Fields: fields,
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}}},
	}
}

// localAddressResult は、渡したレコードだけを持つ収集元 1 件の取り込み結果を組む。declare は
// 収集元の入力形式の宣言 (ParserIdentity) を書き換える。
func localAddressResult(t *testing.T, declare func(*ParserIdentity), records ...RecordEntry) ImportResult {
	t.Helper()
	source := settleSource(t, 0)
	declare(&source.Parser)
	source.Records = records
	result, err := newImportResult([]scannedSource{source},
		[]core.ImportStatus{settleStatus(t, source, "source")}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// rewrittenFixtureResult は、run の fixture の文字列を replacer で書き換えてから取り込む。
func rewrittenFixtureResult(t *testing.T, replacer *strings.Replacer, plans ...SourcePlan) ImportResult {
	t.Helper()
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(replacer.Replace(
				graphSourceText(t, strings.TrimPrefix(originPath, "testdata/"))))), nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
		Revision: "graph-revision", SettingsDigest: "graph-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// requireNoGlobalLocalAddress は、ループバックとリンクローカルのアドレスを、端末の範囲を
// 持たない鍵で識別したノードがグラフに無いことを確かめる。
func requireNoGlobalLocalAddress(t *testing.T, graph Graph) {
	t.Helper()
	for _, node := range graph.nodes {
		address, readable := node.key.AddressValue()
		if node.key.Form == core.NodeKeyFormAddress && readable &&
			(address.IsLoopback() || address.IsLinkLocalUnicast()) {
			t.Errorf("the graph holds the address %s without the scope of a terminal", address)
		}
	}
}

// 2 つの端末が同じループバックのアドレスを持つとき、アドレスのノードを端末ごとに分ける。
// 各ノードの関係の根拠は、その端末のレコードだけである。ほかのアドレスは 1 つのノードである。
func TestLoopbackAddressOfTwoTerminalsIsTwoNodes(t *testing.T) {
	record := func(line int64, terminal, address string) RecordEntry {
		return localAddressRecord(t, line, nil,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, terminal),
			syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, address))
	}
	graph := NewGraph(localAddressResult(t, func(*ParserIdentity) {},
		record(1, "T1", "127.0.0.1"), record(2, "T2", "127.0.0.1"),
		record(3, "T1", "192.0.2.1"), record(4, "T2", "192.0.2.1"),
	), AllMatchConditions())

	for _, testCase := range []struct {
		terminal string
		line     int64
	}{{"T1", 1}, {"T2", 2}} {
		terminalKey, _ := core.TerminalNodeKey(testCase.terminal)
		terminalAt := requireNodeAt(t, graph, terminalKey)
		addressAt := requireNodeAt(t, graph, localAddressKey(terminalKey, "127.0.0.1"))
		kinds := map[core.EdgeKind]int{}
		for _, edge := range graph.edges {
			if edge.target != addressAt && edge.source != addressAt {
				continue
			}
			kinds[edge.kind]++
			if edge.kind == core.EdgeKindTerminalAddress && edge.source != terminalAt {
				t.Errorf("the address of %s has a terminal_address edge from %+v",
					testCase.terminal, graph.nodes[edge.source].key)
			}
			for _, evidence := range edgeEvidenceOf(t, graph, edge.id) {
				if line := evidence.RecordRef.LineNumber; line == nil || *line != testCase.line {
					t.Errorf("the %s edge of the address of %s holds the evidence %+v, want the line %d",
						edge.kind, testCase.terminal, evidence.RecordRef, testCase.line)
				}
			}
		}
		if kinds[core.EdgeKindTerminalAddress] != 1 || kinds[core.EdgeKindRecordNamesObject] != 1 {
			t.Errorf("the address of %s has the edges %v, want one terminal_address and one record_names_object",
				testCase.terminal, kinds)
		}
	}
	requireNoGlobalLocalAddress(t, graph)

	// 端末の中だけで意味を持つアドレスでなければ、2 つの端末の同じ文字列は 1 つのノードである。
	sharedAt := requireNodeAt(t, graph, globalAddressKey("192.0.2.1"))
	for _, terminal := range []string{"T1", "T2"} {
		terminalKey, _ := core.TerminalNodeKey(terminal)
		if !hasEdge(graph, core.EdgeKindTerminalAddress, requireNodeAt(t, graph, terminalKey), sharedAt) {
			t.Errorf("the terminal %s has no terminal_address edge to the shared address", terminal)
		}
	}
}

// ノードの一覧の端末の範囲のアドレスの行は、範囲の端末のノードを持つ。範囲を持たないアドレスの
// 行は端末を持たない。同じ文字列の 2 行を、端末で見分けられる。
func TestNodeSummariesOfLoopbackNameTheTerminalOfTheScope(t *testing.T) {
	record := func(line int64, terminal, address string) RecordEntry {
		return localAddressRecord(t, line, nil,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, terminal),
			syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, address))
	}
	graph := NewGraph(localAddressResult(t, func(*ParserIdentity) {},
		record(1, "T1", "127.0.0.1"), record(2, "T2", "127.0.0.1"),
		record(3, "T1", "192.0.2.1"),
	), AllMatchConditions())

	terminals := map[string]string{}
	for _, summary := range graph.NodeSummaries(core.NodeKindIp, RecordFilter{}, nil) {
		terminal := ""
		if summary.Terminal != nil {
			terminal = summary.Terminal.Id
		}
		terminals[summary.Node.Id] = terminal
	}
	for _, name := range []string{"T1", "T2"} {
		terminalKey, _ := core.TerminalNodeKey(name)
		addressId := graph.nodes[requireNodeAt(t, graph, localAddressKey(terminalKey, "127.0.0.1"))].id
		if want := graph.nodes[requireNodeAt(t, graph, terminalKey)].id; terminals[addressId] != want {
			t.Errorf("the loopback of %s names the terminal %q, want %q", name, terminals[addressId], want)
		}
	}
	sharedId := graph.nodes[requireNodeAt(t, graph, globalAddressKey("192.0.2.1"))].id
	if terminal, listed := terminals[sharedId]; !listed || terminal != "" {
		t.Errorf("the shared address names the terminal %q (listed %t), want none", terminal, listed)
	}
}

// 範囲の端末を、端末の鍵の形ごとに組んだ候補の鍵で探す。値が 2 つの端末の鍵
// (収集元の内容の識別とホスト名) の範囲では、その端末を返す。形の違う 2 つの端末が同じ値の
// 並びを持つときは、どちらの端末かを決めずに返さない。
func TestScopeTerminalOfFindsTheTerminalByTheKeyForms(t *testing.T) {
	record := func(line int64, terminal, address string) RecordEntry {
		return localAddressRecord(t, line, nil,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, terminal),
			syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, address))
	}
	graph := NewGraph(localAddressResult(t, func(*ParserIdentity) {},
		record(1, "T1", "127.0.0.1"), record(2, "T2", "127.0.0.1"),
	), AllMatchConditions())

	hostKey, _ := core.RecordingHostTerminalNodeKey("a"+strings.Repeat("0", 63), "host-a.example.test")
	hostAt := graph.ensureNode(hostKey, core.NodeObservationObserved)
	if at, found := graph.scopeTerminalOf(localAddressKey(hostKey, "::1")); !found || at != hostAt {
		t.Errorf("the scope of the two-value terminal key is %d (found %t), want %d", at, found, hostAt)
	}

	t1Key, _ := core.TerminalNodeKey("T1")
	t1At := requireNodeAt(t, graph, t1Key)
	if at, found := graph.scopeTerminalOf(localAddressKey(t1Key, "127.0.0.1")); !found || at != t1At {
		t.Errorf("the scope of T1 is %d (found %t), want %d", at, found, t1At)
	}

	// 形の違う端末が T2 と同じ値の並びを持つと、範囲の端末を 1 つに決められない。
	sameValues, _ := core.RecordingTerminalNodeKey("T2")
	graph.ensureNode(sameValues, core.NodeObservationObserved)
	t2Key, _ := core.TerminalNodeKey("T2")
	if at, found := graph.scopeTerminalOf(localAddressKey(t2Key, "127.0.0.1")); found {
		t.Errorf("the scope of T2 is the node %d, want none for two terminals of the same values", at)
	}
	for _, summary := range graph.NodeSummaries(core.NodeKindIp, RecordFilter{}, nil) {
		if summary.Node.Id == graph.nodes[requireNodeAt(t, graph, localAddressKey(t2Key, "127.0.0.1"))].id &&
			summary.Terminal != nil {
			t.Errorf("the loopback of T2 names the terminal %s, want none", summary.Terminal.Id)
		}
	}

	// 範囲の端末のノードがグラフに無いアドレスは、端末を返さない。
	absentKey, _ := core.TerminalNodeKey("T9")
	if at, found := graph.scopeTerminalOf(localAddressKey(absentKey, "127.0.0.1")); found {
		t.Errorf("the scope of an absent terminal is the node %d, want none", at)
	}
	// 範囲を持たないアドレスは、端末を返さない。
	if _, found := graph.scopeTerminalOf(globalAddressKey("192.0.2.1")); found {
		t.Error("an address without the scope of a terminal names a terminal")
	}
}

// 接続先がループバックのアドレスである Proxy 型の起点は、起点を置いた端末 (名前不明の端末) の
// 範囲のアドレスのノードから、関連付けの候補のエッジを持つ。Proxy 型のレコードは端末の
// ノードを指さず、端末は利用者が収集元に IP だけを指定して決める。
func TestLoopbackDestinationOriginDerivesTheCandidateEdgeFromItsTerminal(t *testing.T) {
	result := rewrittenFixtureResult(t, strings.NewReplacer(
		"dstIP=198.51.100.7 dstPort=8080", "dstIP=127.0.0.1 dstPort=8080",
		"http://198.51.100.7:8080/a", "http://127.0.0.1:8080/a",
	),
		SourcePlan{OriginPath: "testdata/graph-markii.log", FileName: "graph-markii.log", FormatKey: MarkIIFormatKey},
		SourcePlan{OriginPath: "testdata/graph-squid.log", FileName: "graph-squid.log", FormatKey: SquidFormatKey,
			Terminal: &SourceTerminal{Ip: "198.51.100.20"}},
	)
	graph := NewGraph(result, AllMatchConditions())

	squid := result.publications[1]
	if squid.status.SourceId == "" || squid.records[0].Locator.SourceFileName != "graph-squid.log" {
		t.Fatalf("the second publication is %+v, want the Squid source", squid.records[0].Locator)
	}
	squidTerminal, _ := core.RecordingTerminalNodeKey(squid.status.Scope.SourceContentSha256)
	wantSource := nodeIdOf(localAddressKey(squidTerminal, "127.0.0.1"))

	found := false
	for _, edge := range graph.edges[graph.observedEdgeCount:] {
		if edge.kind != core.EdgeKindCrossSourceConnectionMatch ||
			graph.nodes[edge.source].key.Kind != core.NodeKindIp {
			continue
		}
		if graph.nodes[edge.source].id != wantSource {
			t.Errorf("the candidate edge starts at %+v, want the loopback address of the Squid terminal",
				graph.nodes[edge.source].key)
			continue
		}
		found = true
	}
	if !found {
		t.Fatal("the graph carries no candidate edge from the loopback address of the origin")
	}
	requireNoGlobalLocalAddress(t, graph)
}

// ホスト名ごとに端末を分ける収集元で、ホスト名を名乗らないレコードはどの端末にも置かれず、
// そのレコードのループバックのアドレスはノードにも関係にもならない。ホスト名を名乗った
// レコードのアドレスは、そのホスト名の端末の範囲のノードである。
func TestLoopbackAddressWithoutTheTerminalScopeBuildsNoNode(t *testing.T) {
	named := []core.RecordField{
		syntheticField(t, "hostname", core.SemanticKeyTerminalHostname, "host-a.example.test"),
	}
	addresses := func() []core.RecordField {
		return []core.RecordField{
			syntheticField(t, "srcIP", core.SemanticKeyConnectionSourceAddress, "127.0.0.1"),
			syntheticField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "::1"),
			syntheticField(t, "dstIP2", core.SemanticKeyConnectionDestinationAddress, "192.0.2.5"),
		}
	}
	result := localAddressResult(t,
		func(parser *ParserIdentity) { parser.RecordingTerminalPerHostname = true },
		localAddressRecord(t, 1, nil, addresses()...),
		localAddressRecord(t, 2, named, addresses()...),
	)
	graph := NewGraph(result, AllMatchConditions())

	unnamedAt, indexed := graph.recordAtLocator(result.publications[0].records[0].Locator)
	if !indexed {
		t.Fatal("the graph holds no record without the host name")
	}
	hostTerminal, _ := core.RecordingHostTerminalNodeKey(
		result.publications[0].status.Scope.SourceContentSha256, "host-a.example.test")
	scoped := []int{
		requireNodeAt(t, graph, localAddressKey(hostTerminal, "127.0.0.1")),
		requireNodeAt(t, graph, localAddressKey(hostTerminal, "::1")),
	}
	for at, node := range graph.nodes {
		address, readable := node.key.AddressValue()
		if !readable || !address.IsLoopback() && !address.IsLinkLocalUnicast() {
			continue
		}
		if !slices.Contains(scoped, at) {
			t.Errorf("the graph holds the address node %+v, want only the addresses of the named terminal", node.key)
		}
		if slices.Contains(node.evidence, unnamedAt) {
			t.Errorf("the address node %+v holds the record without the host name", node.key)
		}
	}
	for _, edge := range graph.edges {
		if !slices.Contains(scoped, edge.source) && !slices.Contains(scoped, edge.target) {
			continue
		}
		if slices.Contains(edge.evidence, unnamedAt) {
			t.Errorf("the %s edge of a loopback address holds the record without the host name", edge.kind)
		}
	}
	// 端末の中だけで意味を持つアドレスでなければ、端末の範囲が無くてもノードになる。
	shared := graph.nodes[requireNodeAt(t, graph, globalAddressKey("192.0.2.5"))]
	if !slices.Contains(shared.evidence, unnamedAt) {
		t.Error("the shared address does not hold the record without the host name")
	}
}

// 端末の外部識別子を持たずプロセス番号を持つレコードは、区間で区切ったプロセスを収集元の
// 端末に置く。そのプロセスから、同じ端末の範囲のループバックのアドレスへ通信の関係を張る。
func TestIntervalProcessCommunicatesWithTheLoopbackOfItsTerminal(t *testing.T) {
	result := localAddressResult(t,
		func(parser *ParserIdentity) { parser.RecordedByOneTerminal = true },
		localAddressRecord(t, 1, nil,
			syntheticField(t, "pid", core.SemanticKeyProcessPid, "4242"),
			syntheticField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "127.0.0.1")),
	)
	graph := NewGraph(result, AllMatchConditions())

	terminal, _ := core.RecordingTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256)
	addressAt := requireNodeAt(t, graph, localAddressKey(terminal, "127.0.0.1"))
	processes := processNodesOfPid(t, graph, "4242")
	if len(processes) != 1 {
		t.Fatalf("the pid 4242 has %d process nodes, want 1", len(processes))
	}
	processAt := graph.nodeAt[processes[0]]
	if graph.nodes[processAt].key.Form != core.NodeKeyFormTerminalProcessInterval {
		t.Fatalf("the process key is %+v, want a process interval", graph.nodes[processAt].key)
	}
	if !hasEdge(graph, core.EdgeKindProcessCommunication, processAt, addressAt) {
		t.Error("the interval process has no process_communication edge to the loopback of its terminal")
	}
	requireNoGlobalLocalAddress(t, graph)
}

// 割当の接続元 IP がループバックのアドレスでも、遠隔セッションとその端末のアドレスの組が
// T1021 の規則に合う。端末の範囲の鍵では、アドレスは最後の値である。
func TestRemoteServicesRuleMatchesTheLoopbackClientIp(t *testing.T) {
	result := rewrittenFixtureResult(t, strings.NewReplacer("192.0.2.1", "127.0.0.1"),
		SourcePlan{OriginPath: "testdata/graph-markii-session.log", FileName: "graph-markii-session.log",
			FormatKey: MarkIIFormatKey},
	)
	graph := NewGraph(result, AllMatchConditions())
	resultMatches, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, foundationAttackRules(t))
	if err != nil {
		t.Fatal(err)
	}
	terminal, _ := core.TerminalNodeKey("T1")
	wantAddress := nodeIdOf(localAddressKey(terminal, "127.0.0.1"))
	requireNodeAt(t, graph, localAddressKey(terminal, "127.0.0.1"))

	var match *AttackRuleMatch
	for _, candidate := range resultMatches.Matches {
		if candidate.RuleID == "attack.t1021.remote-services" {
			match = &candidate
			break
		}
	}
	if match == nil || len(match.Edges) != 2 {
		t.Fatalf("the T1021 match is %+v, want a two-edge match", match)
	}
	var primary, contextual AttackRuleMatchedEdge
	for _, edge := range match.Edges {
		switch edge.Role {
		case "session":
			primary = edge
		case "address":
			contextual = edge
		}
	}
	if len(primary.AssignmentBases) == 0 || primary.AssignmentBases[0].ClientIp != "127.0.0.1" {
		t.Errorf("the primary assignment bases are %+v, want the loopback client IP", primary.AssignmentBases)
	}
	if contextual.Edge.Kind != core.EdgeKindTerminalAddress || contextual.Sink.Id != wantAddress {
		t.Errorf("the contextual edge is %+v to %q, want terminal_address to the loopback of T1",
			contextual.Edge, contextual.Sink.Id)
	}
}

// コマンド行の引数が UNC の host に書いたループバックのアドレスは、そのレコードを置いた端末の
// 範囲のアドレスのノードを指す。
func TestArgumentNamedLoopbackIsTheAddressOfTheTerminal(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("101", "4688", logonHostA, "2001-02-03T04:05:00.000Z",
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Windows\System32\cmd.exe`,
			"CommandLine", `cmd /c copy C:\Example\a.exe \\127.0.0.1\example-share\a.exe`, "ProcessId", "0x10") +
		"</Events>\n"
	result := windowsEventImportResult(t, document)
	graph := NewGraph(result, AllMatchConditions())

	terminal, _ := core.RecordingHostTerminalNodeKey(
		result.publications[0].status.Scope.SourceContentSha256, logonHostA)
	addressAt := requireNodeAt(t, graph, localAddressKey(terminal, "127.0.0.1"))
	record := recordOfEvent(t, graph, "101")
	found := false
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindArgumentNamesObject || edge.target != addressAt {
			continue
		}
		found = true
		if !slices.Equal(edge.evidence, []int{record}) {
			t.Errorf("the argument edge holds the evidence %v, want the 4688", edge.evidence)
		}
	}
	if !found {
		t.Error("no argument_names_object edge names the loopback of the terminal")
	}
	requireNoGlobalLocalAddress(t, graph)
}
