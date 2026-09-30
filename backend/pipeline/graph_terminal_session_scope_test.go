// in-package test: 自身の terminal.id を持たないレコードのうち、どのレコードが遠隔のセッションの
// 候補を作るかと、同じ収集元に割当が複数あるときの端末を確かめる。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sysmonConnectionXML は Sysmon の通信のイベント (3) 1 件を返す。
func sysmonConnectionXML(computer, at, recordID, sourceIp, initiated string) string {
	return `<Event><System><Provider Name="Microsoft-Windows-Sysmon"/><EventID>3</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Microsoft-Windows-Sysmon/Operational</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData><Data Name="ProcessGuid">{00000000-0000-0000-0000-000000000001}</Data>` +
		`<Data Name="ProcessId">4</Data><Data Name="Image">C:\Example\net.exe</Data>` +
		`<Data Name="Initiated">` + initiated + `</Data><Data Name="SourceIp">` + sourceIp + `</Data>` +
		`<Data Name="SourcePort">49152</Data><Data Name="DestinationIp">198.51.100.20</Data>` +
		`<Data Name="DestinationPort">445</Data></EventData></Event>` + "\n"
}

// sessionLogonCSV は、イベントビューアーが書き出したログオンの成功 (4624) の論理
// レコード 1 件を、時刻と接続元とログオンの種別を与えて返す。項目名は Windows が 4624 の
// 説明に書く文字列である。
func sessionLogonCSV(at, ip, logonType string) string {
	return "情報," + at + ",Microsoft-Windows-Security-Auditing,4624,ログオン,\"アカウントが正常にログオンしました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\t-\n\tアカウント ドメイン:\t\t-\n\n" +
		"ログオン タイプ:\t\t\t" + logonType + "\n\n" +
		"新しいログオン:\n\tアカウント名:\t\tuser-a\n\tアカウント ドメイン:\t\tEXAMPLE\n\n" +
		"ネットワーク情報:\n\tワークステーション名:\tWS\n\tソース ネットワーク アドレス:\t" + ip +
		"\n\tソース ポート:\t\t49152\"\n"
}

// sessionEdgeCount は、グラフが持つ遠隔のセッションの候補の本数を返す。
func sessionEdgeCount(t *testing.T, result ImportResult, assignments ...core.TerminalAssignment) int {
	t.Helper()
	graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
	return len(terminalSessionEdges(t, graph))
}

// 自分の端末から始めた通信を記録した Sysmon のレコードは、接続元のアドレスに割当があっても
// 候補を作らない。ログオンを記録していないレコードである。
func TestSysmonConnectionIsNoRemoteSession(t *testing.T) {
	document := "<Events>\n" +
		processCreationXML("ws.example.test", sessionTime("04:00:00", true), "701", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		sysmonConnectionXML("ws.example.test", sessionTime("04:10:00", true), "702", "192.0.2.10", "true") +
		"</Events>\n"
	result := windowsEventSessionResult(t, document)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "ws-9", "", false)
	if count := sessionEdgeCount(t, result, assignment); count != 0 {
		t.Errorf("the Sysmon connection makes %d remote session relations, want none", count)
	}
}

// 分析者が端末を与えた収集元の HTTP の要求の記録は、候補を作らない。
func TestHttpRequestsAreNoRemoteSession(t *testing.T) {
	for name, source := range map[string]sessionSource{
		"apache": {name: "web.log", format: ApacheAccessCombinedFormatKey,
			document: `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET / HTTP/1.1" 200 10 "-" "agent"` + "\n" +
				`192.0.2.10 - - [03/Feb/2001:04:11:00 +0000] "GET /a HTTP/1.1" 200 10 "-" "agent"` + "\n"},
		"squid": {name: "access.log", format: SquidFormatKey,
			document: `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://a.example.test/ HTTP/1.1" ` +
				`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n" +
				`192.0.2.10 - - [03/Feb/2001:04:11:00 +0000] "GET http://b.example.test/ HTTP/1.1" ` +
				`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			result := sessionSourcesResult(t, source)
			server := sessionAssignment(t, result, 0, "192.0.2.80", "", "", true)
			client := sessionAssignment(t, result, 0, "192.0.2.10", "ws-9", "", false)
			if count := sessionEdgeCount(t, result, server, client); count != 0 {
				t.Errorf("the HTTP requests make %d remote session relations, want none", count)
			}
		})
	}
}

// 記録した収集元に付けた割当の端末は、その収集元のレコードの接続元にしない。同じ収集元に
// 別の端末の割当が 2 件あり、収集元が Computer ごとの端末に置かれるときも同じである。
func TestLogonFromTheRecordingSourceOwnAssignmentIsNoSession(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument)
	first := sessionAssignment(t, result, 0, "192.0.2.10", "dc-a", "", true)
	second := sessionAssignment(t, result, 0, "192.0.2.11", "dc-b", "", true)
	if count := sessionEdgeCount(t, result, first, second); count != 0 {
		t.Errorf("the recording source makes %d remote session relations from itself, want none", count)
	}
}

// 同じ収集元に IP だけの割当が 2 件あるとき、収集元は 1 台の端末であり、その端末から
// ログオンを記録した端末への候補ができる。
func TestTwoAddressesOfOneSourceMakeOneTerminal(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	first := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
	second := sessionAssignment(t, result, 0, "192.0.2.12", "", "", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{first, second}), AllMatchConditions())
	client, _ := core.RecordingTerminalNodeKey(first.SourceContentSha256)
	parentAt := processNodeLabelled(t, graph, `C:\Example\ws-shell.exe`)
	if !hasEdge(graph, core.EdgeKindRanOn, parentAt, requireNodeAt(t, graph, client)) {
		t.Fatal("the process of the source does not run on the one terminal of the source")
	}
	detail := sessionDetail(t, graph)
	if detail.Edge.SourceNodeId != nodeIdOf(client) || detail.Edge.EvidenceCount != 2 {
		t.Errorf("the relation runs from %q with %d records, want the source terminal with 2",
			detail.Edge.SourceNodeId, detail.Edge.EvidenceCount)
	}
}

// 表示名を持ち外部識別子を持たない割当の端末は、割当の表示名を terminal.hostname で持つ。
// レコードの Computer がノードの表示名になっても、根拠の条件に写さない。
func TestSessionBasisCarriesTheAssignedHostname(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "", "ws-host.example.test", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	right := sessionDetail(t, graph).AssignmentBases[0].Conditions[0].RightValue
	if len(right) != 1 || right[0].Name != clientTerminalNameFieldName ||
		right[0].Semantic != core.SemanticKeyTerminalHostname || right[0].Text == nil ||
		right[0].Text.ValueState != core.ValueStateDerived {
		t.Fatalf("the right value is %+v, want the derived hostname of the assignment", right)
	}
	if value, _ := right[0].Text.ComparableValue(); value != "ws-host.example.test" {
		t.Errorf("the right value carries %q, want the hostname of the assignment", value)
	}
}

// 2 つの案件のログオンが同じ端末の外部識別子を導くとき、割当だけが指す端末のノードは
// 両方の案件のレコードを根拠に持つ。
func TestAssignedTerminalKeepsTheRecordsOfEveryCase(t *testing.T) {
	serverOf := func(computer, recordID string) string {
		return "<Events>\n" +
			processCreationXML(computer, sessionTime("04:00:30", true), recordID+"1", "0x20", "0x1",
				`C:\Example\service.exe`, "service.exe") +
			logonXML(computer, sessionTime("04:10:00", true), recordID+"2", "4624", "192.0.2.10", "3") +
			"</Events>\n"
	}
	result := sessionSourcesResult(t,
		sessionSource{name: "a.xml", format: WindowsEventXMLFormatKey, caseId: "case-a",
			document: serverOf("dc-a.example.test", "90")},
		sessionSource{name: "b.xml", format: WindowsEventXMLFormatKey, caseId: "case-b",
			document: serverOf("dc-b.example.test", "91")})
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-9", "", false),
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-9", "", false),
	}), AllMatchConditions())
	client, _ := core.TerminalNodeKey("ws-9")
	node := graph.nodes[requireNodeAt(t, graph, client)]
	if len(node.evidence) != 2 {
		t.Errorf("the assigned terminal carries %d evidence records, want the logon of each case",
			len(node.evidence))
	}
	if edges := terminalSessionEdges(t, graph); len(edges) != 2 {
		t.Errorf("the graph carries %d remote session relations, want one per case", len(edges))
	}
}

// 期間を時点として読めない割当 (解釈を持たない収集元の地方時の期間) は、同じ IP のほかの
// 割当の候補を消さない。その割当だけが候補を作らない。
func TestUncomparableAssignmentKeepsTheOtherCandidates(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientXML(false), sessionServerDocument)
	utc := sessionAssignment(t, result, 1, "192.0.2.10", "ws-9", "", false)
	local := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
	if count := sessionEdgeCount(t, result, utc); count != 1 {
		t.Fatalf("the UTC assignment alone makes %d remote session relations, want 1", count)
	}
	if count := sessionEdgeCount(t, result, utc, local); count != 1 {
		t.Errorf("the UTC and the uncomparable local assignments make %d remote session relations, "+
			"want the 1 of the UTC assignment", count)
	}
}

// イベントビューアーの CSV は収集元 1 台の端末に置かれ、そのログオンは記録した端末への
// 候補を作る。CSV の時刻はずれを持たないため、解釈を記録した収集元どうしで結ぶ。
func TestWindowsCsvLogonSessionEndsAtTheSourceTerminal(t *testing.T) {
	csv := "\ufeffレベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n" +
		processCreationCSV("2001/02/03 04:00:30", "HOST01$", "0x10", `C:\Example\parent.exe`, "0x1") +
		sessionLogonCSV("2001/02/03 04:10:00", "192.0.2.10", "3") +
		sessionLogonCSV("2001/02/03 04:40:00", "192.0.2.10", "3")
	local := sessionSourcesResult(t,
		sessionSource{name: "ws.xml", format: WindowsEventXMLFormatKey, document: sessionClientXML(false)},
		sessionSource{name: "server.csv", format: WindowsEventCSVFormatKey, document: csv})
	// 割当の期間は接続元の端末の収集元の地方時の文字列である。
	assignment := sessionAssignment(t, local, 0, "192.0.2.10", "", "", true)
	result := interpretedSessionResult(local, "+00:00")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	server, _ := core.RecordingTerminalNodeKey(result.publications[1].status.Scope.SourceContentSha256)
	detail := sessionDetail(t, graph)
	if detail.Edge.TargetNodeId != nodeIdOf(server) || detail.Edge.EvidenceCount != 1 {
		t.Fatalf("the relation ends at %q with %d records, want the CSV terminal with the logon "+
			"inside the range", detail.Edge.TargetNodeId, detail.Edge.EvidenceCount)
	}
	requireLogonTypeGroups(t, detail, "3")
}
