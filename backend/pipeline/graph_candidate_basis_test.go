// in-package test: Windows イベントログの XML と分析者の割当から、候補の関係の組が
// 持つ根拠 (時刻の範囲の幅と差、接続先の名前、他の端末への接続に使うアカウント、未同定の理由) を
// 確かめる。
package pipeline

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 接続とログオンの組は秒未満を切り捨てた時刻で比べる。60.9 秒離れた組は、切り捨てた差が 60 秒で
// 範囲の中にあり、組は照合と同じ切り捨てた差 60 秒と、秒の単位で比べたことを持つ。61 秒離れた組は
// 結ばない。
func TestConnectionLogonPairCarriesTheWholeSecondDifference(t *testing.T) {
	client := "<Events>\n" +
		processCreationXML("ws.example.test", "2001-02-03T04:00:00Z", "600", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		connectionXML("ws.example.test", "2001-02-03T04:10:00.000Z", "601", "4",
			"192.0.2.10", "49152", "192.0.2.1", "445", true) +
		processCreationXML("ws.example.test", "2001-02-03T04:30:00Z", "609", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
	// 700 と 709 は、割当の期間 (各 file の期間) を接続とログオンの前後へ延ばす。
	server := "<Events>\n" +
		processCreationXML("dc.example.test", "2001-02-03T04:00:00Z", "700", "0x20", "0x1",
			`C:\Example\dc-service.exe`, "dc-service.exe") +
		logonXML("dc.example.test", "2001-02-03T04:11:00.900Z", "701", "4624", "192.0.2.10", "3") +
		logonXML("dc.example.test", "2001-02-03T04:11:01.000Z", "702", "4624", "192.0.2.10", "3") +
		processCreationXML("dc.example.test", "2001-02-03T04:30:00Z", "709", "0x21", "0x20",
			`C:\Example\dc-tool.exe`, "dc-tool.exe") +
		"</Events>\n"
	result := windowsEventSessionResult(t, client, server)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "", true),
		sessionAssignment(t, result, 1, "192.0.2.1", "dc-1", "", true),
	}), AllMatchConditions())
	edges := connectionLogonEdges(graph)
	if len(edges) != 1 || edges[0].TargetNodeId != recordNodeIdOf(t, result, 1, "701") {
		t.Fatalf("the connection logon relations are %+v, want the logon 701 alone", edges)
	}
	detail, _ := graph.EdgeDetail(edges[0].Id, EdgeEvidenceFilter{})
	window := pairConditionOfKey(t, detail.RecordPairs[0], core.EdgePairConditionTimeProximity)
	if window.WindowSeconds == nil || *window.WindowSeconds != 60 || window.DifferenceSeconds == nil ||
		*window.DifferenceSeconds != 60 || !window.DifferenceInWholeSeconds {
		t.Errorf("the time condition carries %v, %v and %v, want 60, 60 and whole seconds",
			window.WindowSeconds, window.DifferenceSeconds, window.DifferenceInWholeSeconds)
	}
}

// 同じ秒に同じ接続先へ接続した 2 つのプロセスは、Proxy の 1 行の互いに区別できない候補であり、
// 2 本のエッジはどちらも区別できない候補の数 2 を持つ。区別できる候補のエッジと、同じプロセスを
// 指す 2 件の候補だけの組のエッジは数を持たない。
func TestCandidateEdgesCarryTheIndistinguishableCount(t *testing.T) {
	connection := func(at, sn, subEvent, process, path, port string) string {
		return `02/01/2000 ` + at + ` +0900 sn=` + sn + ` evt=net subEvt=` + subEvent + ` psGUID=` + process +
			` tmid=T1 com="PC01" csid=S-1-5-21-1 ip=192.0.2.1 psPath="` + path + `" srcIP=192.0.2.1 srcPort=` + port +
			` dstIP=198.51.100.7 dstPort=8080` + "\n"
	}
	client := connection("13:00:09.000", "1", "con", "{P1}", `C:\one.exe`, "1001") +
		connection("13:00:09.000", "2", "con", "{P2}", `C:\two.exe`, "1002") +
		connection("13:00:12.000", "3", "con", "{P3}", `C:\three.exe`, "1003") +
		connection("13:00:12.000", "4", "est", "{P3}", `C:\three.exe`, "1004")
	const (
		proxy = `192.0.2.1 - - [01/Feb/2000:13:00:09 +0900] "GET http://198.51.100.7:8080/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` +
			"\n" +
			`192.0.2.1 - - [01/Feb/2000:13:00:12 +0900] "GET http://198.51.100.7:8080/b HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` +
			"\n"
	)
	texts := map[string]string{"client.log": client, "proxy.log": proxy}
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(texts[originPath])), nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
		Revision: "graph-revision", SettingsDigest: "graph-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]SourcePlan{
		{OriginPath: "client.log", FileName: "client.log", FormatKey: MarkIIFormatKey},
		{OriginPath: "proxy.log", FileName: "proxy.log", FormatKey: SquidFormatKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	got := map[string]int64{}
	for _, edge := range graph.Query(GraphQuery{
		EdgeKinds: []core.EdgeKind{core.EdgeKindCrossSourceConnectionMatch}, Depth: 1,
	}).Edges {
		target := graph.nodes[graph.nodeAt[edge.TargetNodeId]]
		label, _ := target.label.ComparableValue()
		got[label] = edge.IndistinguishableCandidateCount
		if err := edge.Validate(); err != nil {
			t.Errorf("the edge to %s does not validate: %v", label, err)
		}
	}
	want := map[string]int64{`C:\one.exe`: 2, `C:\two.exe`: 2, `C:\three.exe`: 0}
	if len(got) != len(want) || got[`C:\one.exe`] != 2 || got[`C:\two.exe`] != 2 || got[`C:\three.exe`] != 0 {
		t.Errorf("the candidate edges carry %v, want %v", got, want)
	}
}

// pairConditionOfKey は組の条件のうち key のものを返す。
func pairConditionOfKey(t *testing.T, pair core.EdgeRecordPair, key core.EdgePairConditionKey) core.EdgePairCondition {
	t.Helper()
	for _, condition := range pair.Conditions {
		if condition.ConditionKey == key {
			return condition
		}
	}
	t.Fatalf("the pair carries no %s condition: %v", key, conditionKeysOf(pair))
	return core.EdgePairCondition{}
}

// 資格情報の組は、接続先を導いた TargetServerName と、時刻の範囲の幅 (2 秒) と、要求からログオンまでの
// 時刻の差を持つ。照合は秒未満まで比べ、差は秒未満を切り捨てない。
func TestExplicitCredentialPairCarriesTheServerNameAndTheWindow(t *testing.T) {
	requests := "<Events>\n" +
		securityEventXML("31", "4648", logonHostA, "2001-02-03T04:10:00.000Z",
			"TargetUserName", "user-b", "TargetDomainName", "EXAMPLE",
			"TargetServerName", "HOST-B.example.test", "IpAddress", "-", "IpPort", "-") +
		"</Events>\n"
	logons := "<Events>\n" +
		hostBLogonXML("41", "2001-02-03T04:00:00.000Z", "other", "192.0.2.10") +
		hostBLogonXML("42", "2001-02-03T04:10:00.500Z", "user-b", "192.0.2.10") +
		"</Events>\n"
	result := windowsEventSessionResult(t, requests, logons)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.20", "ws-b", "host-b", true),
	}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	pair := edgeDetailOfKind(t, graph, core.EdgeKindExplicitCredentialLogon, node("31"), node("42")).RecordPairs[0]
	destination := pairConditionOfKey(t, pair, core.EdgePairConditionDestinationTerminal)
	if got := comparableValuesOf(destination.LeftValue); !slices.Contains(got, "HOST-B.example.test") {
		t.Errorf("the destination condition carries %v, want the TargetServerName", got)
	}
	window := pairConditionOfKey(t, pair, core.EdgePairConditionTimeProximity)
	if window.WindowSeconds == nil || *window.WindowSeconds != 2 ||
		window.DifferenceSeconds == nil || *window.DifferenceSeconds != 0.5 || window.DifferenceInWholeSeconds {
		t.Errorf("the time window is %v with the difference %v, want 2 seconds and 0.5",
			window.WindowSeconds, window.DifferenceSeconds)
	}
	if err := pair.Validate(); err != nil {
		t.Errorf("the pair does not validate: %v", err)
	}
}

// 種別 9 のログオンが始めたセッションからの要求の関係は、他の端末への接続に使うアカウントを
// 要求したセッションの条件に持つ。
func TestRequestedSessionCarriesTheOutboundAccount(t *testing.T) {
	result := windowsEventSessionResult(t,
		"<Events>\n"+
			securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetUserName", "user-a",
				"TargetLogonId", "0xb9", "LogonType", "9", "TargetOutboundUserName", "user-b",
				"TargetOutboundDomainName", "EXAMPLE")+
			explicitRequestXML("12", "2001-02-03T04:10:00.000Z", "user-b", "0xb9")+
			"</Events>\n",
		"<Events>\n"+
			hostBLogonXML("21", "2001-02-03T04:00:00.000Z", "other", "192.0.2.10")+
			hostBLogonXML("22", "2001-02-03T04:10:00.500Z", "user-b", "192.0.2.10")+
			"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{hostBAssignment(t, result, "192.0.2.20")}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	pair := edgeDetailOfKind(t, graph, core.EdgeKindRequestedSessionLogon, node("11"), node("22")).RecordPairs[0]
	session := pairConditionOfKey(t, pair, core.EdgePairConditionRequestingSession)
	if got := comparableValuesOf(session.LeftValue); !slices.Contains(got, "user-b") {
		t.Errorf("the requesting session condition carries %v, want the outbound account user-b", got)
	}
}

// ログオフの記録が無いネットワークのログオン (種別 3) のセッションは最後の操作で終わる。その最後の
// 操作が資格情報を使ったログオンの要求であるとき、要求はセッションの期間の終わりにあり、要求した
// セッションの関係を作る。
func TestRequestedSessionRunsFromANetworkLogonEndingAtTheRequest(t *testing.T) {
	result := windowsEventSessionResult(t,
		"<Events>\n"+
			securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetUserName", "user-a",
				"TargetLogonId", "0xb3", "LogonType", "3")+
			explicitRequestXML("12", "2001-02-03T04:10:00.000Z", "user-b", "0xb3")+
			"</Events>\n",
		"<Events>\n"+
			hostBLogonXML("21", "2001-02-03T04:00:00.000Z", "other", "192.0.2.10")+
			hostBLogonXML("22", "2001-02-03T04:10:00.500Z", "user-b", "192.0.2.10")+
			"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{hostBAssignment(t, result, "192.0.2.20")}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	detail := edgeDetailOfKind(t, graph, core.EdgeKindRequestedSessionLogon, node("11"), node("22"))
	if name, _, _ := sessionPairsOf(t, graph, node("11"), node("22"), detail.RecordPairs[:2]); name != "11/12" {
		t.Errorf("the session runs over %s, want from the logon 11 to the request 12", name)
	}
}

// 未同定の遠隔のセッションの組は、接続元のアドレスに割当が無いことと、割当の期間の外であることを
// 別の条件で持つ。
func TestUnidentifiedSourcePairsSeparateTheOutsideRangeFromNoAssignment(t *testing.T) {
	// 804 は割当の無い 198.51.100.7 から、805 は期間の外の 192.0.2.10 からのログオンである。
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "", "", true),
	}), AllMatchConditions())
	got := map[string]core.EdgePairConditionKey{}
	for _, ends := range edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession) {
		address, _ := graph.nodes[ends[0]].key.LabelValue()
		for _, pair := range edgeDetailOfKind(t, graph, core.EdgeKindUnidentifiedSourceRemoteSession,
			ends[0], ends[1]).RecordPairs {
			got[address] = pair.Conditions[0].ConditionKey
		}
	}
	want := map[string]core.EdgePairConditionKey{
		"198.51.100.7": core.EdgePairConditionSourceUnassigned,
		"192.0.2.10":   core.EdgePairConditionSourceOutsideAssignmentRange,
	}
	if len(got) != len(want) || got["198.51.100.7"] != want["198.51.100.7"] || got["192.0.2.10"] != want["192.0.2.10"] {
		t.Errorf("the unidentified sources carry %v, want %v", got, want)
	}
}
