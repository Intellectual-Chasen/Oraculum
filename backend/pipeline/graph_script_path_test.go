// in-package test: PowerShell の 4104 が記録したスクリプトのパスと、プロセスの引数が指した同じ
// パスが、同じ端末の 1 つのファイルのノードになることと、名前の分からない端末に置いたファイルの
// ノードがその印を持つことを確かめる。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// hostlessEventXML は、端末を名乗らない (Computer を持たない) イベント 1 件を返す。data は
// `<Data>` の名前と値を交互に並べる。
func hostlessEventXML(provider, channel, eventID, recordID, at string, data ...string) string {
	elements := ""
	for index := 0; index+1 < len(data); index += 2 {
		elements += `<Data Name="` + data[index] + `">` + data[index+1] + `</Data>`
	}
	return `<Event><System><Provider Name="` + provider + `"/><EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>` + channel + `</Channel></System><EventData>` + elements + `</EventData></Event>` + "\n"
}

// fileNodesOf は、path を持つファイルのノードの位置を返す。
func fileNodesOf(graph Graph, path string) []int {
	var files []int
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindFile && node.key.Values[len(node.key.Values)-1].Value == core.FilePathKeyValue(path) {
			files = append(files, at)
		}
	}
	return files
}

// 端末を名乗らない 2 つの収集元 (4104 と 4688) を分析者が同じ端末に置くと、4104 のパスと 4688 の
// 引数が指したパスは 1 つのファイルのノードになり、4104 のレコードとプロセスの両方から結ばれる。
// ノードは名前の分からない端末に置いた印を持たない。
func TestScriptBlockPathJoinsTheArgumentFileOnTheSameTerminal(t *testing.T) {
	const path = `C:\Example\run.ps1`
	scripts := "<Events>\n" +
		hostlessEventXML("Microsoft-Windows-PowerShell", "Microsoft-Windows-PowerShell/Operational", "4104", "71",
			"2001-02-03T04:05:01.000Z", "MessageNumber", "1", "MessageTotal", "1",
			"ScriptBlockText", "Write-Output example", "ScriptBlockId", "00000000-0000-0000-0000-000000000001",
			"Path", path) +
		hostlessEventXML("Microsoft-Windows-PowerShell", "Microsoft-Windows-PowerShell/Operational", "4104", "72",
			"2001-02-03T04:05:30.000Z", "MessageNumber", "1", "MessageTotal", "1",
			"ScriptBlockText", "Write-Output other", "ScriptBlockId", "00000000-0000-0000-0000-000000000002") +
		"</Events>\n"
	processes := "<Events>\n" +
		hostlessEventXML("Microsoft-Windows-Security-Auditing", "Security", "4688", "61", "2001-02-03T04:05:00.000Z",
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Example\shell.exe`,
			"CommandLine", `shell.exe -File `+path, "ProcessId", "0x10") +
		hostlessEventXML("Microsoft-Windows-Security-Auditing", "Security", "4688", "62", "2001-02-03T04:06:00.000Z",
			"NewProcessId", "0x4d3", "NewProcessName", `C:\Example\other.exe`, "CommandLine", "other.exe",
			"ProcessId", "0x10") +
		"</Events>\n"
	result := windowsEventSessionResult(t, scripts, processes)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "", "ws-a", "", true),
		sessionAssignment(t, result, 1, "", "ws-a", "", true),
	}), AllMatchConditions())
	files := fileNodesOf(graph, path)
	if len(files) != 1 {
		t.Fatalf("the path on the assigned terminal makes %d file nodes, want 1", len(files))
	}
	if graph.graphNode(files[0]).OnUnknownTerminal {
		t.Error("the file node on the assigned terminal is marked as on an unknown terminal")
	}
	kinds := map[core.EdgeKind]bool{}
	for _, edge := range graph.edges {
		if edge.target == files[0] {
			kinds[edge.kind] = true
		}
	}
	if !kinds[core.EdgeKindRecordNamesObject] || !kinds[core.EdgeKindArgumentNamesObject] {
		t.Errorf("the file node is reached by %v, want the script record and the process argument", kinds)
	}
}

// イベントビューアーの CSV の 4104 は端末を名乗らず、収集元の名前の分からない端末に置かれる。その
// パスのファイルのノードは、名前の分からない端末に置いた印を持つ。分析者が収集元を端末に置くと、
// 印を持たない。
func TestScriptBlockPathOnAnUnknownTerminalIsMarked(t *testing.T) {
	const path = `C:\Example\run.ps1`
	document := "\ufeffレベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n" +
		"詳細,2001/02/03 04:05:06,Microsoft-Windows-PowerShell,4104,リモート コマンドを実行します,\"" +
		"Scriptblock テキストを作成しています (1 個中 1 個目):\nWrite-Output example\n\n" +
		"ScriptBlock ID: 00000000-0000-4000-8000-000000000001\nパス: " + path + "\"\n" +
		"詳細,2001/02/03 04:05:07,Microsoft-Windows-PowerShell,4104,リモート コマンドを実行します,\"" +
		"Scriptblock テキストを作成しています (1 個中 1 個目):\nWrite-Output other\n\n" +
		"ScriptBlock ID: 00000000-0000-4000-8000-000000000002\nパス: \"\n"
	result := sessionSourcesResult(t, sessionSource{name: "scripts.csv", format: WindowsEventCSVFormatKey,
		document: document})
	graph := NewGraph(result, AllMatchConditions())
	files := fileNodesOf(graph, path)
	if len(files) != 1 || !graph.graphNode(files[0]).OnUnknownTerminal {
		t.Fatalf("the path on the unknown terminal makes the file nodes %v, want 1 marked node", files)
	}
	for _, at := range []int{graph.nodeAt[graph.records[0].placedTerminalNodeId]} {
		if graph.graphNode(at).OnUnknownTerminal {
			t.Error("the terminal node carries the mark of a file node")
		}
	}
	assigned := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "", "ws-a", "", true),
	}), AllMatchConditions())
	if files := fileNodesOf(assigned, path); len(files) != 1 || assigned.graphNode(files[0]).OnUnknownTerminal {
		t.Errorf("the path on the assigned terminal makes the file nodes %v, want 1 unmarked node", files)
	}
}
