// in-package test: 分けたログオンの対応と、チケットの要求とログオンの LogonGuid の一致の候補を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

type syntheticEvent struct {
	recordID string
	xml      string
}

// eventsGraph はイベントを 1 つの file に並べてグラフを組み、記録番号ごとのレコードの
// ノードの位置を返す。
func eventsGraph(t *testing.T, events []syntheticEvent) (Graph, map[string]int) {
	t.Helper()
	var document strings.Builder
	document.WriteString("<Events>\n")
	offsets := make(map[int64]string, len(events))
	for _, event := range events {
		offsets[int64(document.Len())] = event.recordID
		document.WriteString(event.xml)
	}
	document.WriteString("</Events>\n")
	graph := NewGraph(windowsEventImportResult(t, document.String()), AllMatchConditions())
	nodes := make(map[string]int, len(offsets))
	for _, record := range graph.records {
		if record.locator.ByteOffset == nil || !record.hasRecordNode {
			continue
		}
		if recordID, found := offsets[*record.locator.ByteOffset]; found {
			nodes[recordID] = record.recordNode
		}
	}
	if len(nodes) != len(events) {
		t.Fatalf("the graph holds record nodes for %d of the events", len(nodes))
	}
	return graph, nodes
}

// edgePairs は種別のエッジを「起点->終点」の記録番号で返す。各エッジの根拠が両端のレコードで
// あり、状態が候補であることも確かめる。
func edgePairs(t *testing.T, graph Graph, nodes map[string]int, kind core.EdgeKind) []string {
	t.Helper()
	var pairs []string
	for _, edge := range edgesOfKind(graph, kind) {
		if edge.state != core.RelationStateCandidate {
			t.Errorf("%s edge is %s, want candidate", kind, edge.state)
		}
		if len(edge.evidence) != 2 {
			t.Errorf("%s edge carries %d evidence records, want 2", kind, len(edge.evidence))
		}
		pairs = append(pairs, recordIdOfNode(t, nodes, edge.source)+"->"+recordIdOfNode(t, nodes, edge.target))
	}
	slices.Sort(pairs)
	return pairs
}

func logonWithLink(recordID, computer, target, linked string) syntheticEvent {
	return syntheticEvent{recordID, securityEventXML(recordID, "4624", computer, "2001-02-03T04:05:10.000Z",
		"SubjectLogonId", "0x3e7", "TargetLogonId", target, "TargetLinkedLogonId", linked, "LogonType", "10")}
}

// 同じ端末で互いを指す 2 件のログオンだけを、先に取り込んだ側から 1 本で結ぶ。一方向だけの組、
// 別の端末の組、分けていないログオン (0x0) は結ばない。相手の Logon ID の操作は、相手の
// ログオンから同じセッションの操作の候補で結ぶ。
func TestLinkedLogonEdgesLinkTheMutualPairOnTheSameTerminal(t *testing.T) {
	graph, nodes := eventsGraph(t, []syntheticEvent{
		logonWithLink("1", logonHostA, "0x00000000000B0001", "0x00000000000B0002"),
		logonWithLink("2", logonHostA, "0x00000000000B0002", "0x00000000000B0001"),
		// 3 は 4 を指すが、4 は 3 を指さない。
		logonWithLink("3", logonHostA, "0x00000000000B0003", "0x00000000000B0004"),
		logonWithLink("4", logonHostA, "0x00000000000B0004", "0x0"),
		// 5 と 6 は互いを指すが、端末が違う。
		logonWithLink("5", logonHostA, "0x00000000000B0005", "0x00000000000B0006"),
		logonWithLink("6", logonHostB, "0x00000000000B0006", "0x00000000000B0005"),
		{"7", securityEventXML("7", "4688", logonHostA, "2001-02-03T04:05:20.000Z",
			"SubjectLogonId", "0x00000000000B0002", "NewProcessId", "0x2b", "NewProcessName", `C:\Example\tool.exe`)},
	})
	if got := edgePairs(t, graph, nodes, core.EdgeKindLinkedLogon); !slices.Equal(got, []string{"1->2"}) {
		t.Errorf("linked logon edges = %v, want [1->2]", got)
	}
	if got := edgePairs(t, graph, nodes, core.EdgeKindLogonSessionOperation); !slices.Equal(got, []string{"2->7"}) {
		t.Errorf("logon session edges = %v, want [2->7]", got)
	}
	requireRecordPairs(t, graph, core.EdgeKindLinkedLogon,
		core.EdgePairConditionLinkedLogonId, core.EdgePairConditionTerminal)
	requireRecordPairs(t, graph, core.EdgeKindLogonSessionOperation,
		core.EdgePairConditionLogonId, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder)
}

// 同じ端末で互いを指す 2 件のログオンでも、別の収集元の file にあれば結ばない。
func TestLinkedLogonEdgesStayWithinASource(t *testing.T) {
	wrap := func(event syntheticEvent) string { return "<Events>\n" + event.xml + "</Events>\n" }
	result := windowsEventImportResult(t,
		wrap(logonWithLink("1", logonHostA, "0x00000000000B0001", "0x00000000000B0002")),
		wrap(logonWithLink("2", logonHostA, "0x00000000000B0002", "0x00000000000B0001")))
	// 2 つの file に同じ端末の外部識別子を与え、端末を同じにする。
	assignments := make([]core.TerminalAssignment, 0, len(result.publications))
	for at := range result.publications {
		single := result
		single.publications = result.publications[at : at+1]
		assignment := analystAssignmentFor(t, single)
		assignment.TerminalId = lineageTerminalId
		assignments = append(assignments, assignment)
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	if edges := edgesOfKind(graph, core.EdgeKindLinkedLogon); len(edges) != 0 {
		t.Errorf("the logons of two sources are linked by %d edges", len(edges))
	}
}

func ticketRequest(recordID, guid string) syntheticEvent {
	return syntheticEvent{recordID, securityEventXML(recordID, "4769", "dc.example.test",
		"2001-02-03T04:05:10.000Z", "ServiceName", "host01$", "LogonGuid", guid)}
}

// チケットの要求から、同じ LogonGuid のログオン (4624) と、明示的な資格情報を使ったログオン
// (4648) へ、端末をまたいで結ぶ。中括弧と大文字と小文字の違いは同じ値として比べ、要求より前の
// 時刻のログオンも結ぶ。全桁 0 の LogonGuid と、違う LogonGuid は結ばない。
func TestTicketRequestLogonEdgesLinkTheSameLogonGuidAcrossTerminals(t *testing.T) {
	const guid = "{0A1B2C3D-0000-1111-2222-333344445555}"
	const null = "{00000000-0000-0000-0000-000000000000}"
	graph, nodes := eventsGraph(t, []syntheticEvent{
		ticketRequest("1", guid),
		ticketRequest("2", null),
		{"3", securityEventXML("3", "4624", logonHostA, "2001-02-03T04:05:09.000Z",
			"TargetLogonId", "0x00000000000C0001", "LogonType", "3",
			"LogonGuid", "0a1b2c3d-0000-1111-2222-333344445555")},
		{"4", securityEventXML("4", "4648", logonHostB, "2001-02-03T04:05:11.000Z",
			"SubjectLogonId", "0x00000000000C0002", "LogonGuid", guid, "TargetLogonGuid", guid)},
		{"5", securityEventXML("5", "4624", "dc.example.test", "2001-02-03T04:05:12.000Z",
			"TargetLogonId", "0x00000000000C0003", "LogonType", "3", "LogonGuid", guid)},
		{"6", securityEventXML("6", "4624", logonHostA, "2001-02-03T04:05:13.000Z",
			"TargetLogonId", "0x00000000000C0004", "LogonType", "3", "LogonGuid", null)},
		{"7", securityEventXML("7", "4624", logonHostA, "2001-02-03T04:05:14.000Z",
			"TargetLogonId", "0x00000000000C0005", "LogonType", "3",
			"LogonGuid", "{0A1B2C3D-0000-1111-2222-333344445556}")},
	})
	want := []string{"1->3", "1->4", "1->5"}
	if got := edgePairs(t, graph, nodes, core.EdgeKindTicketRequestLogon); !slices.Equal(got, want) {
		t.Errorf("ticket request logon edges = %v, want %v", got, want)
	}
	requireRecordPairs(t, graph, core.EdgeKindTicketRequestLogon, core.EdgePairConditionLogonGuid)
}
