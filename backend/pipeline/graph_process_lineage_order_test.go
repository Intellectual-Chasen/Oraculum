// in-package test: 非公開の構築子で取り込んだ Windows イベントログの収集元から、番号と時刻で
// 推定した親子の候補を検査する。
package pipeline

import (
	"fmt"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// processTerminationCSV はプロセスの終了の論理レコード 1 件を返す。項目名は
// Windows が 4689 の説明に書く文字列である。
func processTerminationCSV(at, pid, name string) string {
	return "情報," + at + ",Microsoft-Windows-Security-Auditing,4689,プロセス終了,\"プロセスが終了しました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\tHOST01$\n\tアカウント ドメイン:\t\tCORP-TEST\n\n" +
		"プロセス情報:\n\tプロセス ID:\t" + pid + "\n\tプロセス名:\t" + name + "\n\t終了状態:\t0x0\"\n"
}

// viewerCSVGraph は CSV の収集元 1 件を取り込み、グラフを組む。
func viewerCSVGraph(t *testing.T, records ...string) Graph {
	t.Helper()
	document := "\ufeffレベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"
	for _, record := range records {
		document += record
	}
	scanned := scanIndexSource(t, NewTestParser(WindowsEventCSVFormatKey, nil),
		"exported-events.csv", WindowsEventCSVFormatKey, document)
	statuses := []core.ImportStatus{settleStatus(t, scanned, "winevent-csv")}
	result, err := newImportResult([]scannedSource{scanned}, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return NewGraph(result, AllMatchConditions())
}

// hasParentOf は、その子のノードへ向かう親子のエッジをグラフが持つかを返す。
func hasParentOf(graph Graph, childAt int) bool {
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindProcessParentChild && edge.target == childAt {
			return true
		}
	}
	return false
}

// 新しい順に並ぶ CSV で、同じ秒に作られた親と子は、子より前に作られた親を指す。
// 同じ番号を先に使った別のプロセスを親にしない。
func TestProcessLineageOrdersTheSameSecondOfANewestFirstSource(t *testing.T) {
	graph := viewerCSVGraph(t,
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x14", `C:\Example\child.exe`, "0x10"),
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x10", `C:\Example\parent.exe`, "0x4"),
		processCreationCSV("2001/02/03 04:05:05", "HOST01$", "0x10", `C:\Example\old.exe`, "0x4"),
	)
	oldAt := processNodeLabelled(t, graph, `C:\Example\old.exe`)
	parentAt := processNodeLabelled(t, graph, `C:\Example\parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	if !hasEdge(graph, core.EdgeKindProcessParentChild, parentAt, childAt) {
		t.Error("the child has no parent-child edge from the process created just before it")
	}
	if hasEdge(graph, core.EdgeKindProcessParentChild, oldAt, childAt) {
		t.Error("the child names the earlier process of the same pid as its parent")
	}
}

// 終了を記録したプロセスは、その後に作られたプロセスの親にならない。終了の後に同じ番号で
// 作られたプロセスが、その後の子の親になる。
func TestProcessLineageClosesTheIntervalAtTheTermination(t *testing.T) {
	graph := viewerCSVGraph(t,
		processCreationCSV("2001/02/03 04:05:05", "HOST01$", "0x10", `C:\Example\old.exe`, "0x4"),
		processTerminationCSV("2001/02/03 04:05:06", "0x10", `C:\Example\old.exe`),
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x14", `C:\Example\orphan.exe`, "0x10"),
		processCreationCSV("2001/02/03 04:05:08", "HOST01$", "0x10", `C:\Example\reused.exe`, "0x4"),
		processCreationCSV("2001/02/03 04:05:09", "HOST01$", "0x18", `C:\Example\child.exe`, "0x10"),
	)
	oldAt := processNodeLabelled(t, graph, `C:\Example\old.exe`)
	orphanAt := processNodeLabelled(t, graph, `C:\Example\orphan.exe`)
	reusedAt := processNodeLabelled(t, graph, `C:\Example\reused.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	if hasParentOf(graph, orphanAt) {
		t.Error("the process created after the termination of its parent pid has a parent")
	}
	if !hasEdge(graph, core.EdgeKindProcessParentChild, reusedAt, childAt) {
		t.Error("the child has no parent-child edge from the process that reused the pid")
	}
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindProcessParentChild && (edge.source == oldAt || edge.target == oldAt) &&
			(edge.source == reusedAt || edge.target == reusedAt) {
			t.Error("the terminated process and the process that reused its pid are parent and child")
		}
	}
	if len(graph.nodes[oldAt].evidence) != 2 {
		t.Errorf("the terminated process carries %d records, want the creation and the termination",
			len(graph.nodes[oldAt].evidence))
	}
}

// xmlProcessEvent は 4688 か 4689 の Event 1 件を返す。parentPid が空の Event は 4689 である。
func xmlProcessEvent(at, recordId, pid, name, parentPid string) string {
	system := func(eventId string) string {
		return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>` + eventId +
			`</EventID><TimeCreated SystemTime="2001-02-03T04:05:` + at + `Z"/><EventRecordID>` + recordId +
			`</EventRecordID><Channel>Security</Channel><Computer>host04.example.test</Computer></System>`
	}
	if parentPid == "" {
		return system("4689") + `<EventData><Data Name="ProcessId">` + pid +
			`</Data><Data Name="ProcessName">` + name + "</Data></EventData></Event>\n"
	}
	return system("4688") + `<EventData><Data Name="NewProcessId">` + pid +
		`</Data><Data Name="NewProcessName">` + name + `</Data><Data Name="ProcessId">` + parentPid +
		"</Data></EventData></Event>\n"
}

// xmlSourcesGraph は、XML の収集元を並べた順に取り込み、グラフを組む。
func xmlSourcesGraph(t *testing.T, sources ...[]string) Graph {
	t.Helper()
	scanned := make([]scannedSource, 0, len(sources))
	statuses := make([]core.ImportStatus, 0, len(sources))
	for index, events := range sources {
		document := "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<Events>\n"
		for _, event := range events {
			document += event
		}
		document += "</Events>\n"
		source := scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil),
			fmt.Sprintf("events-%d.xml", index), WindowsEventXMLFormatKey, document)
		scanned = append(scanned, source)
		statuses = append(statuses, settleStatus(t, source, fmt.Sprintf("xml-%d", index)))
	}
	result, err := newImportResult(scanned, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	assignments := make([]core.TerminalAssignment, 0, len(result.publications))
	for _, publication := range result.publications {
		assignments = append(assignments, oneTerminalAssignmentFor(t, publication))
	}
	return NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
}

// oneTerminalAssignmentFor は、収集元の全レコードが同じ 1 つの端末のものであるとする割当を返す。
// 適用期間は、収集元の最も早いレコードから最も遅いレコードの時刻までである。
func oneTerminalAssignmentFor(t *testing.T, publication SourcePublication) core.TerminalAssignment {
	t.Helper()
	records := publication.records
	first, last := records[0].ObservedAt, records[0].ObservedAt
	for _, record := range records {
		at, _ := record.ObservedAt.Instant()
		if from, _ := first.Instant(); at.Before(from) {
			first = record.ObservedAt
		}
		if to, _ := last.Instant(); at.After(to) {
			last = record.ObservedAt
		}
	}
	assignment := core.TerminalAssignment{
		ClientIp:             lineageClientIp,
		TerminalId:           lineageTerminalId,
		TerminalHostname:     lineageTerminalHostname,
		SourceId:             publication.status.SourceId,
		SourceContentSha256:  publication.status.Scope.SourceContentSha256,
		AssignmentValidRange: core.TimeRange{From: *first, To: *last},
		Origin:               core.TerminalAssignmentOriginAnalystSupplied,
		Derivation:           "同じ端末の 2 つの書き出し",
		Author:               "analyst",
		AppliesToSourceId:    publication.status.SourceId,
		BasisRecordRefs:      []core.AssertionRecordRef{core.NewAssertionRecordRef(records[0].Locator)},
	}
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the analyst assignment: %v", err)
	}
	return assignment
}

// すべて同じ時刻の観測だけを持つ収集元は並びの向きが決まらないため、同じ時刻の親子を作らない。
// 新しい順の収集元を昇順と読むと、子が同じ番号を先に使った別の収集元のプロセスを親にする。
func TestProcessLineageSkipsTheSameSecondOfASourceWithoutADirection(t *testing.T) {
	graph := xmlSourcesGraph(t,
		[]string{xmlProcessEvent("01", "101", "0x10", `C:\Example\old.exe`, "0x4")},
		[]string{
			xmlProcessEvent("07", "202", "0x14", `C:\Example\child.exe`, "0x10"),
			xmlProcessEvent("07", "201", "0x10", `C:\Example\new.exe`, "0x4"),
		},
	)
	oldAt := processNodeLabelled(t, graph, `C:\Example\old.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	if hasEdge(graph, core.EdgeKindProcessParentChild, oldAt, childAt) {
		t.Error("the child names the process of the other source as its parent")
	}
	if hasParentOf(graph, childAt) {
		t.Error("the child has a parent although the order within the second is unknown")
	}
}

// 範囲の重なる 2 つの収集元で、同じ時刻の同じ番号の起動と終了は前後が決まらないため、
// 終了のレコードを開いている区間へ入れない。
func TestProcessLineageKeepsAnUnorderedTerminationOutOfTheOpenInterval(t *testing.T) {
	overlapping := func(extra ...string) []string {
		events := []string{
			xmlProcessEvent("07", "303", "0x10", `C:\Example\new.exe`, "0x4"),
			xmlProcessEvent("07", "302", "0x10", `C:\Example\old.exe`, ""),
		}
		events = append(events, extra...)
		return append(events, xmlProcessEvent("05", "301", "0x10", `C:\Example\old.exe`, "0x4"))
	}
	graph := xmlSourcesGraph(t,
		overlapping(),
		overlapping(xmlProcessEvent("06", "304", "0x30", `C:\Example\other.exe`, "0x4")),
	)
	newAt := processNodeLabelled(t, graph, `C:\Example\new.exe`)
	if evidence := graph.nodes[newAt].evidence; len(evidence) != 2 {
		t.Errorf("the new process carries %d records, want its 2 creations only", len(evidence))
	}
}

// 時刻の順に並ばない収集元では、同じ秒の親と子の前後が決まらないため、親子を作らない。
func TestProcessLineageSkipsTheSameSecondOfAnUnorderedSource(t *testing.T) {
	graph := viewerCSVGraph(t,
		processCreationCSV("2001/02/03 04:05:05", "HOST01$", "0x10", `C:\Example\old.exe`, "0x4"),
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x14", `C:\Example\child.exe`, "0x10"),
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x10", `C:\Example\parent.exe`, "0x4"),
		processCreationCSV("2001/02/03 04:05:06", "HOST01$", "0x20", `C:\Example\other.exe`, "0x4"),
	)
	if childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`); hasParentOf(graph, childAt) {
		t.Error("the child has a parent although the order within the second is unknown")
	}
}
