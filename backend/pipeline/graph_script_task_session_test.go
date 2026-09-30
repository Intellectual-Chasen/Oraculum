// in-package test: スクリプトブロック、タスクのプロセスと操作、リモート デスクトップの記録が
// 作る関係を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const powerShellProvider = "Microsoft-Windows-PowerShell"

// fileNodeOf は、識別値のどれかが path のファイルのノードの位置を返す。見つからなければ -1 である。
func fileNodeOf(graph Graph, path string) int {
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindFile && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool { return value.Value == core.FilePathKeyValue(path) }) {
			return at
		}
	}
	return -1
}

// requireNamedFromEvent は、EventRecordID が recordID のレコードのノードから target への
// argument_names_object があることを確かめる。
func requireNamedFromEvent(t *testing.T, graph Graph, recordID string, target int) {
	const kind = core.EdgeKindArgumentNamesObject
	t.Helper()
	record := graph.records[recordOfEvent(t, graph, recordID)].recordNode
	if target < 0 || !slices.Contains(edgePairsOfKind(graph, kind), [2]int{record, target}) {
		t.Errorf("no %s from the record %s to the node %d", kind, recordID, target)
	}
}

// 4104 の本文の引数が名指したファイルと UNC の host へ、レコードのノードから候補が張られる。
// 分けて記録した本文は件ごとに読み、境界で切れた path は切れた文字列のファイルになる。
func TestScriptBlockTextNamesObjectsFromTheRecord(t *testing.T) {
	document := "<Events>\n" +
		taskEventXML(powerShellProvider, "201", "4104", logonHostA, "2001-02-03T04:05:00.000Z",
			"MessageNumber", "1", "MessageTotal", "1",
			"ScriptBlockText", `.\t.exe a 'X:\d\o.zip' "X:\d\i.txt"`+"\n"+`dir \\host201\s`) +
		taskEventXML(powerShellProvider, "202", "4104", logonHostA, "2001-02-03T04:05:01.000Z",
			"MessageNumber", "1", "MessageTotal", "2", "ScriptBlockText", `.\t.exe a 'X:\d\ou`) +
		taskEventXML(powerShellProvider, "203", "4104", logonHostA, "2001-02-03T04:05:01.000Z",
			"MessageNumber", "2", "MessageTotal", "2", "ScriptBlockText", `t.zip' X:\d\j.txt`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	for _, path := range []string{`X:\d\o.zip`, `X:\d\i.txt`} {
		requireNamedFromEvent(t, graph, "201", fileNodeOf(graph, path))
	}
	requireNamedFromEvent(t, graph, "201", nodeOf(graph, core.NodeKindDomain, "host201"))
	requireNamedFromEvent(t, graph, "202", fileNodeOf(graph, `X:\d\ou`))
	requireNamedFromEvent(t, graph, "203", fileNodeOf(graph, `X:\d\j.txt`))
	if fileNodeOf(graph, `X:\d\out.zip`) >= 0 {
		t.Error("the parts were joined into one script block")
	}
}

// シェルが実行したコマンドの項目は、入力形式に依らず引数が名指した対象の候補を起こす。
func TestShellCommandNamesObjects(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "shCmd", core.SemanticKeyProcessShellCommand, `copy X:\d\k.txt \\192.0.2.40\s`),
			},
		},
	}})
	for _, target := range []int{fileNodeOf(graph, `X:\d\k.txt`), nodeOf(graph, core.NodeKindIp, "192.0.2.40")} {
		if target < 0 {
			t.Fatal("the shell command names no file or address node")
		}
		if !slices.ContainsFunc(edgePairsOfKind(graph, core.EdgeKindArgumentNamesObject),
			func(pair [2]int) bool { return pair[1] == target }) {
			t.Errorf("no argument_names_object to %s", graph.nodes[target].id)
		}
	}
}

// 106 の登録は、同じ名前の 100・129・200 の起動へそれぞれ候補を張る。129 は `NT TASK` を前に
// 付けた名前を比べる。129 はプロセス番号の区間に入り、実行ファイルはプロセスの実行ファイルで
// あり、ファイルのノードにならない。
func TestTaskRegistrationLinksTheProcessAndActionRecords(t *testing.T) {
	document := "<Events>\n" +
		taskEventXML(taskSchedulerProvider, "301", "106", logonHostA, "2001-02-03T04:05:00.000Z",
			"TaskName", `\Grp\Job1`) +
		taskEventXML(taskSchedulerProvider, "302", "129", logonHostA, "2001-02-03T04:06:00.000Z",
			"TaskName", `NT TASK\grp\job1`, "Path", `X:\d\job1.exe`, "ProcessID", "4101") +
		taskEventXML(taskSchedulerProvider, "303", "100", logonHostA, "2001-02-03T04:06:00.001Z",
			"TaskName", `\Grp\Job1`) +
		taskEventXML(taskSchedulerProvider, "304", "200", logonHostA, "2001-02-03T04:06:00.002Z",
			"TaskName", `\Grp\Job1`, "ActionName", `X:\d\job1.exe`) +
		taskEventXML(taskSchedulerProvider, "305", "129", logonHostA, "2001-02-03T04:07:00.000Z",
			"TaskName", `\Job2`, "Path", "host305.exe", "ProcessID", "4102") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	pairs := edgePairsOfKind(graph, core.EdgeKindTaskRegistrationRun)
	want := [][2]int{{node("301"), node("302")}, {node("301"), node("303")}, {node("301"), node("304")}}
	slices.SortFunc(pairs, func(left, right [2]int) int { return left[1] - right[1] })
	slices.SortFunc(want, func(left, right [2]int) int { return left[1] - right[1] })
	if !slices.Equal(pairs, want) {
		t.Errorf("task_registration_run = %v, want %v", pairs, want)
	}
	for _, recordID := range []string{"302", "305"} {
		if intervalHolding(graph, recordOfEvent(t, graph, recordID)) < 0 {
			t.Errorf("the 129 record %s belongs to no process interval", recordID)
		}
	}
	if fileNodeOf(graph, `X:\d\job1.exe`) >= 0 {
		t.Error("the executable of the created process became a file node")
	}
}

// intervalHolding は、根拠に at のレコードを持つ区間のプロセスのノードの位置を返す。無ければ -1 である。
func intervalHolding(graph Graph, at int) int {
	return slices.IndexFunc(graph.nodes, func(process graphNode) bool {
		return process.key.Form == core.NodeKeyFormTerminalProcessInterval && slices.Contains(process.evidence, at)
	})
}

// 4688 が開いた区間へ、同じ番号の 129 と後の 4689 が入る。129 が別の区間を開いて、4688 の
// プロセスの続きの事象を切り離さない。
func TestTaskProcessCreationJoinsTheIntervalOfTheProcessCreation(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("311", "4688", logonHostA, "2001-02-03T04:06:00.000Z",
			"NewProcessId", "0x1005", "NewProcessName", `X:\d\job1.exe`, "ProcessId", "0x4") +
		taskEventXML(taskSchedulerProvider, "312", "129", logonHostA, "2001-02-03T04:06:00.010Z",
			"TaskName", `\Job1`, "Path", `X:\d\job1.exe`, "ProcessID", "4101") +
		securityEventXML("313", "4689", logonHostA, "2001-02-03T04:07:00.000Z", "ProcessId", "0x1005") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	started := intervalHolding(graph, recordOfEvent(t, graph, "311"))
	for _, recordID := range []string{"312", "313"} {
		if held := intervalHolding(graph, recordOfEvent(t, graph, recordID)); held != started || held < 0 {
			t.Errorf("the record %s belongs to the interval %d, want the 4688 interval %d", recordID, held, started)
		}
	}
}

// remoteDesktopXML は、リモート デスクトップのイベント 1 件を返す。
func remoteDesktopXML(provider, eventID, recordID, at, body string) string {
	return `<Event><System><Provider Name="` + provider + `"/><EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Example</Channel><Computer>dc.example.test</Computer></System>` + body + `</Event>` + "\n"
}

// remoteDesktopServerXML は、dc.example.test が記録した 1 件の接続の記録を持つ file を返す。
func remoteDesktopServerXML(connection string) string {
	return "<Events>\n" +
		processCreationXML("dc.example.test", sessionTime("04:00:30", true), "801", "0x20", "0x1",
			`C:\Example\dc-service.exe`, "dc-service.exe") +
		connection + "</Events>\n"
}

// 割当のある接続元からの 1149 は遠隔のセッションの候補を作る。同じ接続元からの 131 は
// 認証の前の接続の受け付けであり、候補を作らない。どちらのレコードもアドレスのノードと結ばれる。
func TestRemoteDesktopConnectionRunsFromTheAssignedTerminal(t *testing.T) {
	for _, testCase := range []struct {
		name, connection string
		sessions         int
	}{
		{"1149", remoteDesktopXML("Microsoft-Windows-TerminalServices-RemoteConnectionManager", "1149", "812",
			sessionTime("04:10:01", true),
			`<UserData><EventXML><Param1>user-a</Param1><Param2>EXAMPLE</Param2><Param3>192.0.2.10</Param3></EventXML></UserData>`), 1},
		{"131", remoteDesktopXML("Microsoft-Windows-RemoteDesktopServices-RdpCoreTS", "131", "812",
			sessionTime("04:10:00", true),
			`<EventData><Data Name="ConnType">TCP</Data><Data Name="ClientIP">192.0.2.10:50001</Data></EventData>`), 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := windowsEventSessionResult(t, sessionClientDocument, remoteDesktopServerXML(testCase.connection))
			assignment := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
			graph := NewGraph(result.WithAnalystTerminalAssignments(
				[]core.TerminalAssignment{assignment}), AllMatchConditions())
			if edges := terminalSessionEdges(t, graph); len(edges) != testCase.sessions {
				t.Errorf("the graph carries %d remote session relations, want %d", len(edges), testCase.sessions)
			}
			address := nodeOf(graph, core.NodeKindIp, "192.0.2.10")
			record := graph.records[recordOfEvent(t, graph, "812")].recordNode
			if address < 0 || !slices.ContainsFunc(graph.edges, func(edge graphEdge) bool {
				return (edge.source == record && edge.target == address) ||
					(edge.source == address && edge.target == record)
			}) {
				t.Error("the record is not linked to the address node")
			}
		})
	}
}
