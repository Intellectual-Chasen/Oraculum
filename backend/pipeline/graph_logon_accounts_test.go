// in-package test: Windows イベントログの XML と分析者の割当から、資格情報を使った
// ログオンの要求の候補、接続元が未同定の遠隔のセッションの候補、チケットの要求の根拠、
// 同じアカウントの候補を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// accountHostB はログオンを受けた端末のホスト名である。
const accountHostB = "host-b.example.test"

// requestSourceXML は、host-a が記録した 4648 を並べた file を返す。data は 4648 ごとの
// 記録番号、時刻、接続先のアドレス、資格情報のアカウントの名前である。
func requestSourceXML(requests ...[4]string) string {
	document := "<Events>\n"
	for _, request := range requests {
		document += securityEventXML(request[0], "4648", logonHostA, request[1],
			"TargetUserName", request[3], "TargetDomainName", "EXAMPLE",
			"IpAddress", request[2], "IpPort", "445")
	}
	return document + "</Events>\n"
}

// hostBLogonXML は host-b が記録した 4624 を返す。
func hostBLogonXML(recordID, at, account, ip string) string {
	return securityEventXML(recordID, "4624", accountHostB, at,
		"TargetUserSid", "S-1-5-21-7-8-9-1101", "TargetUserName", account, "TargetDomainName", "EXAMPLE",
		"LogonType", "3", "IpAddress", ip, "IpPort", "50001")
}

// hostBAssignment は、host-b の file の観測期間に clientIp を端末 ws-b (host-b) へ割り当てる。
func hostBAssignment(t *testing.T, result ImportResult, clientIp string) core.TerminalAssignment {
	t.Helper()
	return sessionAssignment(t, result, 1, clientIp, "ws-b", accountHostB, false)
}

// 4648 の接続先のアドレスを割当が導いた端末の、同じアカウントのログオンへ候補を張る。
// 名前の大文字と小文字は区別しない。許容幅の外、別のアカウント、割当の無いアドレス、
// ループバックの接続先、割当の期間の外、要求を記録した端末自身、ログオンの失敗は結ばない。
func TestExplicitCredentialLogonRunsToTheLogonOfTheDestination(t *testing.T) {
	result := windowsEventSessionResult(t,
		requestSourceXML(
			[4]string{"11", "2001-02-03T04:10:00.000Z", "192.0.2.20", "user-b"},
			[4]string{"12", "2001-02-03T04:10:30.000Z", "198.51.100.9", "user-b"},
			[4]string{"13", "2001-02-03T04:10:40.000Z", "127.0.0.1", "user-b"},
			// 割当の期間 (host-b の file の 04:00:00 から) の外の要求である。
			[4]string{"14", "2001-02-03T03:59:59.900Z", "192.0.2.20", "user-b"}),
		"<Events>\n"+
			hostBLogonXML("21", "2001-02-03T04:00:00.000Z", "other", "192.0.2.10")+
			hostBLogonXML("26", "2001-02-03T04:00:00.300Z", "user-b", "192.0.2.10")+
			// host-b 自身が記録した要求は、割当が host-b 自身を導くため結ばない。
			securityEventXML("27", "4648", accountHostB, "2001-02-03T04:10:00.100Z",
				"TargetUserName", "user-b", "TargetDomainName", "EXAMPLE", "IpAddress", "192.0.2.20", "IpPort", "445")+
			// ログオンの失敗は結ばない。
			securityEventXML("28", "4625", accountHostB, "2001-02-03T04:10:00.200Z",
				"TargetUserSid", "S-1-0-0", "TargetUserName", "user-b", "TargetDomainName", "EXAMPLE",
				"LogonType", "3", "IpAddress", "192.0.2.10", "IpPort", "50001", "Status", "0x0000abcd")+
			hostBLogonXML("22", "2001-02-03T04:10:00.500Z", "USER-B", "192.0.2.10")+
			hostBLogonXML("23", "2001-02-03T04:10:00.600Z", "user-c", "192.0.2.10")+
			hostBLogonXML("24", "2001-02-03T04:10:05.000Z", "user-b", "192.0.2.10")+
			hostBLogonXML("25", "2001-02-03T04:20:00.000Z", "other", "192.0.2.10")+
			"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{hostBAssignment(t, result, "192.0.2.20")}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	pairs := edgePairsOfKind(graph, core.EdgeKindExplicitCredentialLogon)
	if want := [][2]int{{node("11"), node("22")}}; !slices.Equal(pairs, want) {
		t.Fatalf("explicit_credential_logon = %v, want %v", pairs, want)
	}
	edge := graph.edges[graph.edgeAt[edgeIdOf(core.EdgeKindExplicitCredentialLogon,
		graph.nodes[node("11")].id, graph.nodes[node("22")].id)]]
	if edge.state != core.RelationStateCandidate || len(edge.evidence) != 2 {
		t.Errorf("the relation is %q with %d evidence records, want a candidate with 2",
			edge.state, len(edge.evidence))
	}
	requireRecordPairs(t, graph, core.EdgeKindExplicitCredentialLogon, core.EdgePairConditionAccount,
		core.EdgePairConditionDestinationTerminal, core.EdgePairConditionTimeProximity)

	// 割当が無いときは結ばない。
	bare := NewGraph(result, AllMatchConditions())
	if pairs := edgePairsOfKind(bare, core.EdgeKindExplicitCredentialLogon); len(pairs) != 0 {
		t.Errorf("explicit_credential_logon without the assignment = %v, want none", pairs)
	}
}

// 接続先のアドレスを持たず TargetServerName だけを持つ 4648 は、その名前を記録した割当が
// 1 台の端末を導くとき、その端末のログオンへ候補を張る。割当の短い名前は FQDN の先頭と比べる。
// 名前が 2 台の端末に一致するときは結ばない。
func TestExplicitCredentialLogonRunsToTheServerNamedByTheAssignment(t *testing.T) {
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
	named := sessionAssignment(t, result, 1, "192.0.2.20", "ws-b", "host-b", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{named}),
		AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	if pairs, want := edgePairsOfKind(graph, core.EdgeKindExplicitCredentialLogon),
		[][2]int{{node("31"), node("42")}}; !slices.Equal(pairs, want) {
		t.Fatalf("explicit_credential_logon = %v, want %v", pairs, want)
	}

	// ws-c は host-a の file の期間に同じ短い名前を記録した別の端末である。
	other := sessionAssignment(t, result, 0, "192.0.2.21", "ws-c", "host-b", false)
	ambiguous := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{named, other}),
		AllMatchConditions())
	if pairs := edgePairsOfKind(ambiguous, core.EdgeKindExplicitCredentialLogon); len(pairs) != 0 {
		t.Errorf("explicit_credential_logon with two terminals of the name = %v, want none", pairs)
	}

	// IP が要求を記録した端末自身に一致する要求は、名前で別の端末を探し直さない。
	selfRequests := "<Events>\n" +
		securityEventXML("32", "4648", logonHostA, "2001-02-03T04:10:00.000Z",
			"TargetUserName", "user-b", "TargetDomainName", "EXAMPLE",
			"TargetServerName", "HOST-B.example.test", "IpAddress", "192.0.2.30", "IpPort", "445") +
		"</Events>\n"
	selfResult := windowsEventSessionResult(t, selfRequests, logons)
	self := sessionAssignment(t, selfResult, 0, "192.0.2.30", "ws-a", "host-a", true)
	selfNamed := sessionAssignment(t, selfResult, 1, "192.0.2.20", "ws-b", "host-b", true)
	selfGraph := NewGraph(selfResult.WithAnalystTerminalAssignments([]core.TerminalAssignment{self, selfNamed}),
		AllMatchConditions())
	if pairs := edgePairsOfKind(selfGraph, core.EdgeKindExplicitCredentialLogon); len(pairs) != 0 {
		t.Errorf("explicit_credential_logon of a request to the own address = %v, want none", pairs)
	}
}

// 割当の無い接続元からのログオンは、アドレスのノードから記録した端末への未同定の候補になる。
// 割当の期間の外も未同定である。ループバックからは作らない。分析者がアドレスを割り当てると、
// 端末から端末への候補に置き換わる。
func TestUnidentifiedSourceSessionRunsFromTheAddress(t *testing.T) {
	// 804 は割当の無い 198.51.100.7 から、805 は期間の外の 192.0.2.10 からのログオンである。
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	server, _ := core.RecordingHostTerminalNodeKey(
		result.publications[1].status.Scope.SourceContentSha256, "dc.example.test")
	target := graph.nodeAt[nodeIdOf(server)]
	sources := map[string]int{}
	for _, pair := range edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession) {
		if pair[1] != target {
			t.Errorf("an unidentified session runs to %s", graph.nodes[pair[1]].id)
		}
		address, _ := graph.nodes[pair[0]].key.LabelValue()
		sources[address]++
	}
	if len(sources) != 2 || sources["198.51.100.7"] != 1 || sources["192.0.2.10"] != 1 {
		t.Errorf("unidentified sessions from %v, want 198.51.100.7 and 192.0.2.10", sources)
	}

	loopback := windowsEventSessionResult(t, "<Events>\n"+
		logonXML("dc.example.test", sessionTime("04:10:00", true), "901", "4624", "127.0.0.1", "3")+
		logonXML("dc.example.test", sessionTime("04:11:00", true), "902", "4624", "fe80::1", "3")+
		"</Events>\n")
	if pairs := edgePairsOfKind(NewGraph(loopback, AllMatchConditions()),
		core.EdgeKindUnidentifiedSourceRemoteSession); len(pairs) != 0 {
		t.Errorf("unidentified sessions from local addresses = %v, want none", pairs)
	}

	whole := sessionAssignment(t, result, 1, "198.51.100.7", "ws-7", "", false)
	assigned := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment, whole}), AllMatchConditions())
	for _, pair := range edgePairsOfKind(assigned, core.EdgeKindUnidentifiedSourceRemoteSession) {
		if address, _ := assigned.nodes[pair[0]].key.LabelValue(); address == "198.51.100.7" {
			t.Error("the assigned address still runs an unidentified session")
		}
	}
	terminal, _ := core.TerminalNodeKey("ws-7")
	if !slices.ContainsFunc(edgePairsOfKind(assigned, core.EdgeKindTerminalRemoteSession), func(pair [2]int) bool {
		return assigned.nodes[pair[0]].id == nodeIdOf(terminal)
	}) {
		t.Error("the assigned address runs no terminal remote session")
	}
	requireNoT1021On(t, graph, core.EdgeKindUnidentifiedSourceRemoteSession)
}

// 共有へのアクセスは、接続元のアドレスから記録した端末へ入ったセッションの根拠である。
func TestShareAccessRunsAnUnidentifiedSessionFromTheAddress(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("61", "5140", "dc.example.test", "2001-02-03T04:10:00.000Z",
			"SubjectUserName", "user-s", "SubjectDomainName", "EXAMPLE", "SubjectLogonId", "0x61",
			"IpAddress", "198.51.100.61", "IpPort", "50061", "ShareName", `\\*\IPC$`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	var sources []string
	for _, pair := range edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession) {
		address, _ := graph.nodes[pair[0]].key.LabelValue()
		sources = append(sources, address)
	}
	if !slices.Equal(sources, []string{"198.51.100.61"}) {
		t.Errorf("unidentified sessions from %v, want 198.51.100.61", sources)
	}
}

// requireNoT1021On は、kind のエッジが ATT&CK の T1021 の候補に入らないことを確かめる。
func requireNoT1021On(t *testing.T, graph Graph, kind core.EdgeKind) {
	t.Helper()
	result, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, foundationAttackRules(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range result.Matches {
		for _, edge := range match.Edges {
			if edge.Edge.Kind == kind {
				t.Errorf("the rule %s matched the %s relation", match.RuleID, kind)
			}
		}
	}
}

// ticketRequestXML は dc が記録した 4769 を返す。
func ticketRequestXML(recordID, at, ip string) string {
	return securityEventXML(recordID, "4769", "dc.example.test", at,
		"TargetUserName", "user-a@EXAMPLE.TEST", "TargetDomainName", "EXAMPLE.TEST",
		"ServiceName", "svc-a", "ServiceSid", "S-1-5-21-7-8-9-1201",
		"LogonGuid", "{11111111-2222-3333-4444-555555555555}", "IpAddress", ip, "IpPort", "50002")
}

// 4769 は、同じ接続元と端末の間にログオンが作った遠隔のセッションの候補があるときだけ、その
// 根拠に入る。4769 だけの組は候補を作らない。要求したアカウントは名前と領域の組のノードになる。
func TestTicketRequestJoinsTheSessionOfTheLogon(t *testing.T) {
	server := "<Events>\n" +
		ticketRequestXML("811", sessionTime("04:09:00", true), "::ffff:192.0.2.10") +
		logonXML("dc.example.test", sessionTime("04:10:00", true), "812", "4624", "192.0.2.10", "3") +
		ticketRequestXML("813", sessionTime("04:11:00", true), "192.0.2.30") +
		ticketRequestXML("814", sessionTime("04:12:00", true), "198.51.100.8") +
		"</Events>\n"
	result := windowsEventSessionResult(t, sessionClientDocument, server)
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "", "", true),
		sessionAssignment(t, result, 1, "192.0.2.30", "ws-30", "", false),
	}), AllMatchConditions())
	detail := sessionDetail(t, graph)
	if detail.Edge.EvidenceCount != 2 {
		t.Errorf("the session carries %d evidence records, want the logon and the ticket request",
			detail.Edge.EvidenceCount)
	}
	if pairs := edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession); len(pairs) != 0 {
		t.Errorf("the ticket request alone ran unidentified sessions %v", pairs)
	}
	requester := core.NodeKey{Kind: core.NodeKindAccount, Form: core.NodeKeyFormAccountDomainName,
		Values: []core.NodeIdentityValue{
			{Semantic: core.SemanticKeyAccountDomain, Value: "EXAMPLE.TEST"},
			{Semantic: core.SemanticKeyAccountName, Value: "user-a"},
		}}
	at, present := graph.nodeAt[nodeIdOf(requester)]
	if !present {
		t.Fatal("the requesting account has no node")
	}
	record := graph.records[recordOfEvent(t, graph, "811")].recordNode
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindRecordSubjectAccount), [2]int{record, at}) {
		t.Error("the ticket request does not name the requesting account as the subject")
	}
	// 4769 だけが指した割当の端末は、割当の IP の関係だけを持つ。
	unused, _ := core.TerminalNodeKey("ws-30")
	terminalAt, present := graph.nodeAt[nodeIdOf(unused)]
	if !present {
		t.Fatal("the assignment of ws-30 has no terminal node")
	}
	for _, edge := range graph.edges {
		if (edge.source == terminalAt || edge.target == terminalAt) && edge.kind != core.EdgeKindTerminalAddress {
			t.Errorf("the ticket request alone added the %s edge to the assigned terminal ws-30", edge.kind)
		}
	}
}

// 記録した端末が持つアドレスからのログオンは、未同定の候補にしない。
func TestUnidentifiedSourceSessionSkipsTheAddressOfTheRecordingTerminal(t *testing.T) {
	record := func(line int64, source string) RecordEntry {
		entry := timelineRecord(line, utcAt(t, "2001-02-03T04:05:00Z"))
		entry.Semantics = &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "tmip", core.SemanticKeyTerminalIpAddress, "192.0.2.50"),
				syntheticField(t, "srcIP", core.SemanticKeyConnectionSourceAddress, source),
			},
		}
		return entry
	}
	graph := graphOfRecords(t, []RecordEntry{record(1, "192.0.2.50"), record(2, "192.0.2.51")})
	sources := []string{}
	for _, pair := range edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession) {
		address, _ := graph.nodes[pair[0]].key.LabelValue()
		sources = append(sources, address)
	}
	if !slices.Equal(sources, []string{"192.0.2.51"}) {
		t.Errorf("unidentified sessions from %v, want only 192.0.2.51", sources)
	}
}

// accountEventXML は、Target のアカウントの SID と名前を記録した dc のイベントを返す。
func accountEventXML(recordID, eventID, at, sid string) string {
	return securityEventXML(recordID, eventID, "dc.example.test", at,
		"TargetSid", sid, "TargetUserName", "user-m", "TargetDomainName", "EXAMPLE")
}

// 名前だけのアカウントは、その時刻の直前と直後に同じ名前と共に記録した SID が 1 つのとき、
// その SID のノードと同じアカウントの候補で結ぶ。削除と作り直しの間の記録は結ばない。
func TestAccountIdentityMatchFollowsTheSidOfTheTime(t *testing.T) {
	const (
		oldSid = "S-1-5-21-7-8-9-1301"
		newSid = "S-1-5-21-7-8-9-1302"
	)
	named := func(recordID, at string) string {
		return securityEventXML(recordID, "4648", "dc.example.test", at,
			"TargetUserName", "USER-M", "TargetDomainName", "example")
	}
	document := "<Events>\n" +
		named("31", "2001-02-03T03:50:00Z") +
		accountEventXML("32", "4726", "2001-02-03T04:00:00Z", oldSid) +
		named("33", "2001-02-03T04:02:00Z") +
		accountEventXML("34", "4720", "2001-02-03T04:05:00Z", newSid) +
		named("35", "2001-02-03T04:30:00Z") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	byEvidence := map[string]string{}
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindAccountIdentityMatch {
			continue
		}
		sid, _ := graph.nodes[edge.target].key.LabelValue()
		for _, recordID := range []string{"31", "33", "35"} {
			if slices.Contains(edge.evidence, recordOfEvent(t, graph, recordID)) {
				byEvidence[recordID] = sid
			}
		}
	}
	want := map[string]string{"31": oldSid, "35": newSid}
	if len(byEvidence) != len(want) || byEvidence["31"] != oldSid || byEvidence["35"] != newSid {
		t.Errorf("the named records match %v, want %v", byEvidence, want)
	}
}

// **合ったアカウントの件数と一緒に、同じアカウントの候補で結ばれた SID と名前の組の数を返す。**
// 種別ごとの件数は SID と名前のノードを別に数えたままにし、組の数で 2 重に数えたことを示す。
//
// 同じ組を 2 件以上のレコードが根拠として結んでも、1 組と数える。
func TestQueryCountsTheAccountIdentityPairsAmongTheMatchedAccounts(t *testing.T) {
	const sid = "S-1-5-21-7-8-9-1401"
	named := func(recordID, at string) string {
		return securityEventXML(recordID, "4648", "dc.example.test", at,
			"TargetUserName", "USER-M", "TargetDomainName", "example")
	}
	for name, testCase := range map[string]struct {
		named        []string
		wantEvidence int
	}{
		// 根拠は名前だけのレコードと、SID と名前を共に記録したレコードである。
		"名前だけのレコード 1 件": {[]string{named("41", "2001-02-03T03:50:00Z")}, 2},
		"名前だけのレコード 2 件": {[]string{named("41", "2001-02-03T03:50:00Z"), named("43", "2001-02-03T03:55:00Z")}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			// 後から同じ名前を別の SID と共に記録した 4726 があり、名前は 1 つの SID に寄らない。
			document := "<Events>\n" + strings.Join(testCase.named, "") +
				accountEventXML("42", "4720", "2001-02-03T04:00:00Z", sid) +
				accountEventXML("44", "4726", "2001-02-03T05:00:00Z", "S-1-5-21-7-8-9-1402") + "</Events>\n"
			graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
			evidence := 0
			for _, edge := range graph.edges {
				if edge.kind == core.EdgeKindAccountIdentityMatch {
					evidence += len(edge.evidence)
				}
			}
			if evidence != testCase.wantEvidence {
				t.Fatalf("the identity match carries %d evidence records, want %d", evidence, testCase.wantEvidence)
			}
			subgraph := graph.Query(GraphQuery{
				NodeKinds: []core.NodeKind{core.NodeKindAccount}, Granularity: core.GraphGranularityObject,
			})
			// 合うアカウントは、名前だけのレコードの名前、4720 の SID、4726 の SID、2 件が共に書いた
			// 名前の 4 件である。同じアカウントの候補で結ばれるのは、名前だけのレコードの名前と
			// 4720 の SID の 1 組である。
			if !slices.Equal(subgraph.MatchedKinds, []core.MatchedKind{{Kind: core.NodeKindAccount, Count: 4}}) ||
				subgraph.MatchedAccountIdentityPairCount != 1 {
				t.Errorf("matched %v with %d identity pairs, want 4 accounts and 1 pair",
					subgraph.MatchedKinds, subgraph.MatchedAccountIdentityPairCount)
			}
		})
	}
}
