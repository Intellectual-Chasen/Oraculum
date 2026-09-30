// in-package test: イベントビューアーの CSV を走査した収集元の識別と、組んだグラフの端末と
// プロセスを確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// processCreationCSV はプロセスの作成の論理レコード 1 件を返す。項目名は
// Windows が 4688 の説明に書く文字列である。
func processCreationCSV(at, account, pid, name, parentPid string) string {
	return "情報," + at + ",Microsoft-Windows-Security-Auditing,4688,プロセス作成,\"新しいプロセスが作成されました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\t" + account + "\n\tアカウント ドメイン:\t\tCORP-TEST\n\n" +
		"プロセス情報:\n\t新しいプロセス ID:\t\t" + pid + "\n\t新しいプロセス名:\t\t" + name +
		"\n\tクリエーター プロセス ID:\t" + parentPid + "\"\n"
}

// 同じ秒の 4688 が 2 件、説明を組めなかったレコードが 1 件、閉じない引用符のレコードが 1 件ある。
var viewerCSVDocument = "\ufeffレベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n" +
	processCreationCSV("2001/02/03 04:05:06", "HOST01$", "0x10", `C:\Example\parent.exe`, "0x1") +
	processCreationCSV("2001/02/03 04:05:06", "HOST01$", "0x11", `C:\Example\child.exe`, "0x10") +
	"情報,2001/02/03 04:05:07,Example-Provider,9,,\"ソース \"\"Example-Provider\"\" からのイベント ID 9 の" +
	"説明が見つかりません。\n\nイベントには次の情報が含まれています: \n\nvalue01\n\"\n" +
	"情報,2001/02/03 04:05:08,Example-Provider,10,,\"never closed\n" +
	processCreationCSV("2001/02/03 04:05:09", "analyst01", "0x12", `C:\Example\other.exe`, "0x1")

func TestWindowsEventCSVSourceCountsUnrenderedRecordsAndCandidates(t *testing.T) {
	scanned := scanIndexSource(t, NewTestParser(WindowsEventCSVFormatKey, nil),
		"exported-events.csv", WindowsEventCSVFormatKey, viewerCSVDocument)
	if len(scanned.Records) != 4 || len(scanned.Failures) != 1 {
		t.Fatalf("records = %d, failures = %+v; want 4 and 1", len(scanned.Records), scanned.Failures)
	}
	if line := scanned.Failures[0].Failure.LineNumber; line == nil ||
		*line != int64(strings.Count(viewerCSVDocument[:strings.Index(viewerCSVDocument, "情報,2001/02/03 04:05:08")], "\n"))+1 {
		t.Errorf("the broken record is reported on line %v", line)
	}
	identity := sourceIdentity(scanned, "source")
	identity.OriginPath = "exported-events.csv"
	if err := identity.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if identity.MessageUnrenderedCount == nil || *identity.MessageUnrenderedCount != 1 {
		t.Errorf("MessageUnrenderedCount = %v, want 1", identity.MessageUnrenderedCount)
	}
	want := []core.TerminalCandidate{{Name: "HOST01$", RecordCount: 2}}
	if !slices.Equal(identity.TerminalCandidates, want) {
		t.Errorf("TerminalCandidates = %+v, want %+v", identity.TerminalCandidates, want)
	}
	// 地方時の文字列は時点を持たないため、収録範囲にならない。
	if identity.ObservedRangeFirst != nil {
		t.Errorf("ObservedRangeFirst = %+v, want none for local times", identity.ObservedRangeFirst)
	}
	// XML の形式は説明を組めたかを持たない。
	xml := sourceIdentity(scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil),
		"events.xml", WindowsEventXMLFormatKey, windowsEventScanDocument), "xml")
	if xml.MessageUnrenderedCount != nil || xml.TerminalCandidates != nil {
		t.Errorf("the XML source carries %v %+v", xml.MessageUnrenderedCount, xml.TerminalCandidates)
	}
}

// 説明を組めなかったレコードを、件数だけでなく位置で特定できる。
func TestWindowsEventCSVSourceLocatesTheUnrenderedRecords(t *testing.T) {
	scanned := scanIndexSource(t, NewTestParser(WindowsEventCSVFormatKey, nil),
		"exported-events.csv", WindowsEventCSVFormatKey, viewerCSVDocument)
	result, err := newImportResult([]scannedSource{scanned},
		[]core.ImportStatus{settleStatus(t, scanned, "winevent-csv")}, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	refs, found := result.MessageUnrenderedRecordRefs("winevent-csv", 10)
	wantLine := int64(strings.Count(viewerCSVDocument[:strings.Index(viewerCSVDocument, "情報,2001/02/03 04:05:07")], "\n")) + 1
	if !found || len(refs) != 1 || refs[0].LineNumber == nil || *refs[0].LineNumber != wantLine {
		t.Fatalf("refs = %+v, want the record on line %d", refs, wantLine)
	}
	if limited, _ := result.MessageUnrenderedRecordRefs("winevent-csv", 0); len(limited) != 0 {
		t.Errorf("the limit 0 returns %d refs", len(limited))
	}
}

// 端末を指定していない CSV の収集元は、名前不明の端末 1 台に置く。説明のアカウント名で
// 端末を決めない。
// 4688 のプロセスはその端末で動き、同じ秒の地方時の親子が親子の候補になる。
func TestWindowsEventCSVGraphPlacesTheSourceOnOneUnknownTerminal(t *testing.T) {
	scanned := scanIndexSource(t, NewTestParser(WindowsEventCSVFormatKey, nil),
		"exported-events.csv", WindowsEventCSVFormatKey, viewerCSVDocument)
	statuses := []core.ImportStatus{settleStatus(t, scanned, "winevent-csv")}
	result, err := newImportResult([]scannedSource{scanned}, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	terminalKey, _ := core.RecordingTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256)
	terminalAt := requireNodeAt(t, graph, terminalKey)
	if raw, _ := graph.nodes[terminalAt].label.NormalizedValue(); !strings.Contains(raw, "exported-events.csv") {
		t.Errorf("the terminal label is %+v, want the unknown terminal of the file", graph.nodes[terminalAt].label)
	}
	parentAt := processNodeLabelled(t, graph, `C:\Example\parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	if !hasEdge(graph, core.EdgeKindRanOn, parentAt, terminalAt) || !hasEdge(graph, core.EdgeKindRanOn, childAt, terminalAt) {
		t.Error("a process has no ran_on edge to the unknown terminal of the file")
	}
	if !hasEdge(graph, core.EdgeKindProcessParentChild, parentAt, childAt) {
		t.Error("the processes of the same second have no parent-child edge")
	}
	if len(graph.nodes[childAt].creationRecords) == 0 {
		t.Error("the child process carries no creation record")
	}
	for _, node := range graph.nodes {
		if raw, _ := node.label.RawTextValue(); node.key.Kind == core.NodeKindTerminal && strings.Contains(raw, "HOST01") {
			t.Errorf("the machine account named the terminal %+v", node.key)
		}
	}
}
