// in-package test: Windows イベントログの XML からグラフを組み、一意な識別子の
// プロセスとプロセス番号の区間のプロセスを結ぶ同じプロセスの候補を確かめる。
package pipeline

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// GUID。Sysmon の GUID の形を持つ値である。
const (
	identityFirstGuid  = "{0A1B2C3D-0003-4000-8000-0000000000A1}"
	identitySecondGuid = "{0A1B2C3D-0003-4000-8000-0000000000B1}"
	identityTaskGuid   = "{0A1B2C3D-0003-4000-8000-0000000000C1}"
)

// taskProcessXML はタスクのプロセスの作成 (タスク スケジューラの 129) のイベント 1 件を返す。
func taskProcessXML(computer, at, recordID, pid, path string) string {
	return `<Event><System><Provider Name="Microsoft-Windows-TaskScheduler"/><EventID>129</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Microsoft-Windows-TaskScheduler/Operational</Channel><Computer>` + computer +
		`</Computer></System><EventData><Data Name="TaskName">\Example Task</Data>` +
		`<Data Name="Path">` + path + `</Data><Data Name="ProcessID">` + pid + `</Data>` +
		`</EventData></Event>` + "\n"
}

// タスク スケジューラの 129 が記録したプロセス番号の区間も、同じ番号の GUID のプロセスと結ぶ。
func TestProcessIdentityMatchJoinsTheTaskProcess(t *testing.T) {
	const host = "host-s.example.test"
	image := `C:\Example\task.exe`
	document := "<Events>\n" +
		sysmonCreateXML("40", identityTaskGuid, "61", image, sysmonRootGuid, "4", `C:\Example\root.exe`) +
		taskProcessXML(host, "2001-02-03T04:05:40Z", "140", "61", image) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	task := nodeWithIdentity(t, graph, core.NodeKindProcess, identityTaskGuid)
	interval := intervalProcessAt(t, graph, "61", "2001-02-03T04:05:40")
	requireRecordPairs(t, graph, core.EdgeKindProcessIdentityMatch, core.EdgePairConditionProcessPid,
		core.EdgePairConditionTerminal, core.EdgePairConditionTimeOverlap)
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindProcessIdentityMatch && edge.source == task && edge.target == interval {
			return
		}
	}
	t.Error("the graph misses the same process relation of the task process")
}

// terminationXML はプロセスの終了のイベント 1 件を返す。
func terminationXML(computer, at, recordID, pid, name string) string {
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4689</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Security</Channel><Computer>` + computer + `</Computer></System><EventData>` +
		`<Data Name="ProcessId">` + pid + `</Data><Data Name="ProcessName">` + name + `</Data>` +
		`</EventData></Event>` + "\n"
}

// processIdentityDocument は、同じ端末の Sysmon と Security のイベントを 1 つの file に並べる。
//
// プロセス番号 31 を 2 つのプロセスが順に使う。1 つ目は GUID A (Sysmon の 11 と 12) と、番号の
// 区間 (通信の許可の 04:05:12 と終了の 04:05:13) で記録され、2 つ目は GUID B (Sysmon の 30) と、
// 番号の区間 (作成の 04:05:30 と通信の許可の 04:05:31) で記録される。
func processIdentityDocument() string {
	const host = "host-s.example.test"
	image := `C:\Example\tool.exe`
	return "<Events>\n" +
		sysmonCreateXML("11", identityFirstGuid, "31", image, sysmonRootGuid, "4", `C:\Example\root.exe`) +
		sysmonOperationXML("12", "3", identityFirstGuid, "31", image,
			"Protocol", "tcp", "Initiated", "true", "SourceIp", "192.0.2.17", "SourcePort", "50017",
			"DestinationIp", "198.51.100.17", "DestinationPort", "8017") +
		connectionXML(host, "2001-02-03T04:05:12Z", "112", "31", "192.0.2.17", "50017",
			"198.51.100.17", "8017", true) +
		terminationXML(host, "2001-02-03T04:05:13Z", "113", "0x1f", image) +
		sysmonCreateXML("30", identitySecondGuid, "31", image, sysmonRootGuid, "4", `C:\Example\root.exe`) +
		processCreationXML(host, "2001-02-03T04:05:30Z", "130", "0x1f", "0x4", image, "tool.exe /second") +
		connectionXML(host, "2001-02-03T04:05:31Z", "131", "31", "192.0.2.17", "50031",
			"198.51.100.31", "8031", true) +
		"</Events>\n"
}

// intervalProcessAt は、番号 pid の区間のプロセスのうち、区間の始まりが startPrefix で始まる
// ノードの位置を返す。
func intervalProcessAt(t *testing.T, graph Graph, pid, startPrefix string) int {
	t.Helper()
	for at, node := range graph.nodes {
		values := node.key.Values
		if node.key.Form == core.NodeKeyFormTerminalProcessInterval && len(values) >= 2 &&
			values[len(values)-2].Value == pid && strings.HasPrefix(values[len(values)-1].Value, startPrefix) {
			return at
		}
	}
	t.Fatalf("the graph holds no process interval of %s starting at %s", pid, startPrefix)
	return -1
}

// 同じ端末・同じ番号で、観測の時刻の範囲が重なる GUID のプロセスと区間のプロセスだけを結ぶ。
// 番号を再利用した別のプロセスとは結ばない。
func TestProcessIdentityMatchJoinsOnlyOverlappingProcesses(t *testing.T) {
	graph := NewGraph(windowsEventImportResult(t, processIdentityDocument()), AllMatchConditions())
	first := nodeWithIdentity(t, graph, core.NodeKindProcess, identityFirstGuid)
	second := nodeWithIdentity(t, graph, core.NodeKindProcess, identitySecondGuid)
	firstInterval := intervalProcessAt(t, graph, "31", "2001-02-03T04:05:12")
	secondInterval := intervalProcessAt(t, graph, "31", "2001-02-03T04:05:30")
	type pair struct{ source, target int }
	got := map[pair]bool{}
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindProcessIdentityMatch {
			continue
		}
		if edge.state != core.RelationStateCandidate {
			t.Errorf("the relation %s is %s, want a candidate", edge.id, edge.state)
		}
		got[pair{edge.source, edge.target}] = true
	}
	want := map[pair]bool{{first, firstInterval}: true, {second, secondInterval}: true}
	if len(got) != len(want) {
		t.Fatalf("the graph carries the same process relations %v, want %v", got, want)
	}
	for key := range want {
		if !got[key] {
			t.Errorf("the graph misses the same process relation %v", key)
		}
	}
}
