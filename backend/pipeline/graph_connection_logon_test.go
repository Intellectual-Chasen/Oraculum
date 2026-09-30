// in-package test: Windows イベントログの XML と分析者の割当からグラフを組み、
// 外向きの接続と接続先の端末のログオンの候補を確かめる。
package pipeline

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// connectionXML は通信の許可のイベント 1 件を返す。outbound が偽のイベントは内向きである。
func connectionXML(
	computer, at, recordID, pid, sourceAddress, sourcePort, destAddress, destPort string, outbound bool,
) string {
	direction := "%%14592"
	if outbound {
		direction = "%%14593"
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>5156</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Security</Channel><Computer>` + computer + `</Computer></System><EventData>` +
		`<Data Name="ProcessID">` + pid + `</Data>` +
		`<Data Name="Application">\device\harddiskvolume1\example\tool.exe</Data>` +
		`<Data Name="Direction">` + direction + `</Data>` +
		`<Data Name="SourceAddress">` + sourceAddress + `</Data><Data Name="SourcePort">` + sourcePort + `</Data>` +
		`<Data Name="DestAddress">` + destAddress + `</Data><Data Name="DestPort">` + destPort + `</Data>` +
		`</EventData></Event>` + "\n"
}

// connectionLogonDocuments は、接続元の端末 ws.example.test と接続先の端末 dc.example.test の
// file を返す。
//
// ws の 601 は 192.0.2.10:49152 から 192.0.2.1:445 への接続、602 は接続元 port が違う接続、
// 603 は 192.0.2.1:445 を接続先に記録した資格情報を指定したログオンの要求 (接続元を持たない) で
// ある。dc の 702 は 192.0.2.10:49152 からのログオンで接続の 1 秒前、706 は接続の 60 秒前、
// 705 は接続の 61 秒後、703 は 10 分後のログオンである。
func connectionLogonDocuments() (string, string) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "600", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		connectionXML("ws.example.test", "2001-02-03T04:10:02Z", "601", "4",
			"192.0.2.10", "49152", "192.0.2.1", "445", true) +
		connectionXML("ws.example.test", "2001-02-03T04:10:02Z", "602", "4",
			"192.0.2.10", "49153", "192.0.2.1", "445", true) +
		explicitCredentialXML("ws.example.test", "2001-02-03T04:10:02Z", "603", "0x10",
			"192.0.2.1", "445") +
		processCreationXML("ws.example.test", "2001-02-03T04:30:00Z", "604", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
	server := "<Events>\n" +
		processCreationXML("dc.example.test", "2001-02-03T04:00:00Z", "700", "0x20", "0x1",
			`C:\Example\dc-service.exe`, "dc-service.exe") +
		logonXML("dc.example.test", "2001-02-03T04:09:02Z", "706", "4624", "192.0.2.10", "3") +
		logonXML("dc.example.test", "2001-02-03T04:10:01Z", "702", "4624", "192.0.2.10", "3") +
		logonXML("dc.example.test", "2001-02-03T04:11:03Z", "705", "4624", "192.0.2.10", "3") +
		logonXML("dc.example.test", "2001-02-03T04:20:02Z", "703", "4624", "192.0.2.10", "3") +
		processCreationXML("dc.example.test", "2001-02-03T04:30:00Z", "704", "0x21", "0x20",
			`C:\Example\dc-tool.exe`, "dc-tool.exe") +
		"</Events>\n"
	return client, server
}

// connectionLogonEdges は接続とログオンの候補のエッジを返す。
func connectionLogonEdges(graph Graph) []core.GraphEdge {
	return graph.Query(GraphQuery{
		EdgeKinds: []core.EdgeKind{core.EdgeKindConnectionLogonMatch}, Depth: 1,
		Granularity: core.GraphGranularityRecord,
	}).Edges
}

// recordNodeIdOf は、収集元 source の EventRecordID が recordID のレコードのノードの識別子を返す。
func recordNodeIdOf(t *testing.T, result ImportResult, source int, recordID string) string {
	t.Helper()
	for _, record := range result.publications[source].records {
		if value, found := comparableOfName(record.Semantics.Fields, "EventRecordID"); found && value == recordID {
			key, _ := core.NewRecordNodeKey(record.Locator)
			return nodeIdOf(key)
		}
	}
	t.Fatalf("the source %d carries no record %s", source, recordID)
	return ""
}

// explicitCredentialXML は資格情報を指定したログオンの要求のイベント 1 件を返す。
func explicitCredentialXML(computer, at, recordID, pid, address, port string) string {
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4648</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Security</Channel><Computer>` + computer + `</Computer></System><EventData>` +
		`<Data Name="TargetUserName">user-a</Data><Data Name="TargetDomainName">EXAMPLE</Data>` +
		`<Data Name="ProcessId">` + pid + `</Data><Data Name="ProcessName">C:\Example\ws-shell.exe</Data>` +
		`<Data Name="IpAddress">` + address + `</Data><Data Name="IpPort">` + port + `</Data>` +
		`</EventData></Event>` + "\n"
}

// 接続元のアドレス・port が一致し、接続先のアドレスの割当が指す端末のログオンで、時刻の差が
// 60 秒以内 (両端を含む) の組だけが候補になる。port の違う接続、61 秒後と 10 分後のログオン、
// 接続先だけを記録した資格情報の要求は結ばない。
func TestConnectionLogonMatchJoinsThePortAndTheAssignedTerminal(t *testing.T) {
	client, server := connectionLogonDocuments()
	result := windowsEventSessionResult(t, client, server)
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
		sessionAssignment(t, result, 1, "192.0.2.1", "dc-1", "", true),
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	edges := connectionLogonEdges(graph)
	connection := recordNodeIdOf(t, result, 0, "601")
	want := map[string]bool{
		recordNodeIdOf(t, result, 1, "702"): true, recordNodeIdOf(t, result, 1, "706"): true,
	}
	if len(edges) != len(want) {
		t.Fatalf("the graph carries %d connection logon relations, want %d: %+v", len(edges), len(want), edges)
	}
	for _, edge := range edges {
		if edge.SourceNodeId != connection || !want[edge.TargetNodeId] {
			t.Errorf("the relation runs from %q to %q, want the connection 601 to the logon 702 or 706",
				edge.SourceNodeId, edge.TargetNodeId)
		}
		if edge.State != core.RelationStateCandidate || edge.EvidenceCount != 2 {
			t.Errorf("the relation is %q with %d evidence records, want a candidate with 2",
				edge.State, edge.EvidenceCount)
		}
	}
	requireRecordPairs(t, graph, core.EdgeKindConnectionLogonMatch, core.EdgePairConditionSourceEndpoint,
		core.EdgePairConditionDestinationTerminal, core.EdgePairConditionTimeProximity)
}

// 接続先のアドレスに割当が無い接続は、ログオンを記録した端末を決められず、候補にならない。
func TestConnectionLogonMatchNeedsTheAssignmentOfTheDestination(t *testing.T) {
	client, server := connectionLogonDocuments()
	result := windowsEventSessionResult(t, client, server)
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
		sessionAssignment(t, result, 1, "", "dc-1", "", true),
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	if edges := connectionLogonEdges(graph); len(edges) != 0 {
		t.Errorf("the graph carries %d connection logon relations, want none", len(edges))
	}
}

// 内向きの 5156 は、割当で決まる接続元の端末から、記録した端末への着信の接続の候補になる。
// port を問わない。遠隔のセッションの候補にならず、割当の無い接続元と外向きの 5156 は
// 候補を作らない。
func TestInboundConnectionMatchJoinsTheAssignedSourceTerminal(t *testing.T) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "800", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		processCreationXML("ws.example.test", "2001-02-03T04:30:00Z", "801", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
	server := "<Events>\n" +
		connectionXML("dc.example.test", "2001-02-03T04:10:00Z", "900", "4",
			"192.0.2.1", "8080", "192.0.2.10", "49152", false) +
		connectionXML("dc.example.test", "2001-02-03T04:11:00Z", "901", "4",
			"192.0.2.1", "445", "203.0.113.9", "49153", false) +
		connectionXML("dc.example.test", "2001-02-03T04:12:00Z", "902", "4",
			"192.0.2.1", "49154", "192.0.2.10", "445", true) +
		"</Events>\n"
	result := windowsEventSessionResult(t, client, server)
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
		sessionAssignment(t, result, 1, "", "dc-1", "", true),
	}
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	for _, kind := range []core.EdgeKind{
		core.EdgeKindTerminalRemoteSession, core.EdgeKindUnidentifiedSourceRemoteSession,
	} {
		if pairs := edgePairsOfKind(graph, kind); len(pairs) != 0 {
			t.Errorf("the inbound connections form %d %s relations, want none", len(pairs), kind)
		}
	}
	edges := graph.Query(GraphQuery{
		EdgeKinds: []core.EdgeKind{core.EdgeKindInboundConnectionMatch}, Depth: 1,
	}).Edges
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d inbound connection relations, want 1: %+v", len(edges), edges)
	}
	edge := edges[0]
	if edge.SourceNodeId != terminalNodeIdOf(t, graph, "ws-1") ||
		edge.TargetNodeId != terminalNodeIdOf(t, graph, "dc-1") {
		t.Errorf("the relation runs from %q to %q, want ws-1 to dc-1", edge.SourceNodeId, edge.TargetNodeId)
	}
	if edge.State != core.RelationStateCandidate || edge.EvidenceCount != 1 {
		t.Errorf("the relation is %q with %d evidence records, want a candidate with 1",
			edge.State, edge.EvidenceCount)
	}
	// 割当の照合に使った接続元の文字列を、比較に用いる値として持つ。
	detail, found := graph.EdgeDetail(edge.Id, EdgeEvidenceFilter{})
	if !found || len(detail.AssignmentBases) != 1 {
		t.Fatalf("the relation carries %d assignment bases, want 1", len(detail.AssignmentBases))
	}
	left := detail.AssignmentBases[0].Conditions[0].LeftValue[0].Text
	if normalized, ok := left.NormalizedValue(); !ok || normalized != "192.0.2.10" {
		t.Errorf("the compared client address is %q (%v), want 192.0.2.10", normalized, ok)
	}
}

// 自分の端末の側のアドレスがマルチキャストか UDP のブロードキャストの内向きの 5156 は、割当で
// 決まる接続元の端末からも、割当の無い接続元からも、どの関係にもならない。同じ相手からの宛先が
// 1 台の着信は候補になる。
func TestInboundGroupAddressConnectionsFormNoRelation(t *testing.T) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "800", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		processCreationXML("ws.example.test", "2001-02-03T04:30:00Z", "801", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
	groups := []string{"192.0.2.255", "255.255.255.255", "233.252.0.1", "ff0e::db8:0:1"}
	var received string
	for index, local := range groups {
		for offset, peer := range []string{"192.0.2.10", "203.0.113.9"} {
			received += strings.Replace(connectionXML("dc.example.test", "2001-02-03T04:1"+strconv.Itoa(index)+":00Z",
				strconv.Itoa(900+index*2+offset), "4", local, "137", peer, "137", false),
				"</EventData>", `<Data Name="Protocol">17</Data></EventData>`, 1)
		}
	}
	unicast := connectionXML("dc.example.test", "2001-02-03T04:20:00Z", "990", "4",
		"192.0.2.1", "445", "192.0.2.10", "49152", false)
	for _, test := range []struct {
		name  string
		extra string
		want  int
	}{{"group addresses only", "", 0}, {"with a unicast connection", unicast, 1}} {
		result := windowsEventSessionResult(t, client, "<Events>\n"+received+test.extra+"</Events>\n")
		graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
			sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
			sessionAssignment(t, result, 1, "", "dc-1", "", true),
		}), AllMatchConditions())
		for _, kind := range []core.EdgeKind{
			core.EdgeKindTerminalRemoteSession, core.EdgeKindUnidentifiedSourceRemoteSession,
		} {
			if pairs := edgePairsOfKind(graph, kind); len(pairs) != 0 {
				t.Errorf("%s: the inbound connections form %d %s relations, want none", test.name, len(pairs), kind)
			}
		}
		if got := len(edgePairsOfKind(graph, core.EdgeKindInboundConnectionMatch)); got != test.want {
			t.Errorf("%s: the graph carries %d inbound connection relations, want %d", test.name, got, test.want)
		}
	}
}

// 成立の根拠の接続元の欄は、割当と比べた文字列と一致する原資料の文字列だけに正規化値を付け、
// 欄が既に持つ正規化値と導き方を保つ。
func TestTerminalSessionBasisCarriesTheComparedClientAddress(t *testing.T) {
	member := core.TerminalAssignment{
		ClientIp: "192.0.2.10", TerminalId: "ws-1", SourceId: "source-a",
		AssignmentValidRange: core.TimeRange{
			From: *utcAt(t, "2001-02-03T04:00:00Z"), To: *utcAt(t, "2001-02-03T05:00:00Z"),
		},
	}
	cases := []struct {
		name           string
		text           core.RawAndNormalized
		wantNormalized string
		wantDerivation string
		wantNone       bool
	}{
		{
			name:           "keeps the normalized value of the field",
			text:           normalizedText("192.0.2.010", "192.0.2.10", "synthetic rule"),
			wantNormalized: "192.0.2.10",
			wantDerivation: "synthetic rule",
		},
		{
			name:     "leaves a raw text different from the compared address",
			text:     presentText(" 192.0.2.10 "),
			wantNone: true,
		},
		{
			name:           "carries the raw text compared with the assignment",
			text:           presentText("192.0.2.10"),
			wantNormalized: "192.0.2.10",
			wantDerivation: comparedClientIpDerivation,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := []core.RecordField{
				textField("SourceAddress", core.SemanticKeyConnectionSourceAddress, tc.text),
			}
			basis, ok := terminalSessionBasis(fields, "192.0.2.10", member, "")
			if !ok {
				t.Fatal("the basis is not composed")
			}
			left := basis.Conditions[0].LeftValue[0].Text
			normalized, hasNormalized := left.NormalizedValue()
			derivation, _ := left.DerivationValue()
			if tc.wantNone {
				if hasNormalized {
					t.Errorf("the field carries the normalized value %q, want none", normalized)
				}
				return
			}
			if normalized != tc.wantNormalized || derivation != tc.wantDerivation {
				t.Errorf("the field carries %q derived by %q, want %q derived by %q",
					normalized, derivation, tc.wantNormalized, tc.wantDerivation)
			}
		})
	}
}
