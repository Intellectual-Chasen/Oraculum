// in-package test: 4648 と接続先のログオンから、要求したセッションの関係を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// explicitRequestXML は、host-a が記録した、192.0.2.20 への 4648 を返す。subject が空のときは
// SubjectLogonId を持たない。
func explicitRequestXML(recordID, at, account, subject string) string {
	data := []string{"TargetUserName", account, "TargetDomainName", "EXAMPLE", "IpAddress", "192.0.2.20", "IpPort", "445"}
	if subject != "" {
		data = append(data, "SubjectLogonId", subject)
	}
	return securityEventXML(recordID, "4648", logonHostA, at, data...)
}

// 4648 の SubjectLogonId のセッションの期間に要求があれば、そのセッションの始まりのレコードから
// 接続先のログオンへ候補の関係を張り、要求と接続先のログオンの組を根拠に残す。SubjectLogonId を
// 持たない要求、固定の値の要求、ログオフの後の要求は張らない。
func TestRequestedSessionRunsFromTheSessionThatAskedForTheLogon(t *testing.T) {
	result := windowsEventSessionResult(t,
		"<Events>\n"+
			securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xb1", "LogonType", "2")+
			explicitRequestXML("12", "2001-02-03T04:10:00.000Z", "user-b1", "0xb1")+
			explicitRequestXML("13", "2001-02-03T04:11:00.000Z", "user-b2", "")+
			explicitRequestXML("14", "2001-02-03T04:12:00.000Z", "user-b3", "0x3e7")+
			securityEventXML("15", "4634", logonHostA, "2001-02-03T04:13:00.000Z", "TargetLogonId", "0xb1")+
			explicitRequestXML("16", "2001-02-03T04:14:00.000Z", "user-b4", "0xb1")+
			"</Events>\n",
		"<Events>\n"+
			hostBLogonXML("21", "2001-02-03T04:00:00.000Z", "other", "192.0.2.10")+
			hostBLogonXML("22", "2001-02-03T04:10:00.500Z", "user-b1", "192.0.2.10")+
			hostBLogonXML("23", "2001-02-03T04:11:00.500Z", "user-b2", "192.0.2.10")+
			hostBLogonXML("24", "2001-02-03T04:12:00.500Z", "user-b3", "192.0.2.10")+
			hostBLogonXML("26", "2001-02-03T04:14:00.500Z", "user-b4", "192.0.2.10")+
			"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{hostBAssignment(t, result, "192.0.2.20")}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	if got := len(edgePairsOfKind(graph, core.EdgeKindExplicitCredentialLogon)); got != 4 {
		t.Fatalf("explicit_credential_logon = %d edges, want 4", got)
	}
	pairs := edgePairsOfKind(graph, core.EdgeKindRequestedSessionLogon)
	if want := [][2]int{{node("11"), node("22")}}; !slices.Equal(pairs, want) {
		t.Fatalf("requested_session_logon = %v, want %v", pairs, want)
	}
	detail := edgeDetailOfKind(t, graph, core.EdgeKindRequestedSessionLogon, node("11"), node("22"))
	edge := graph.edges[graph.edgeAt[detail.Edge.Id]]
	// 根拠は、セッションの始まり (11)、終わりを決めたログオフ (15)、要求 (12)、ログオン (22) である。
	evidence := []int{recordOfEvent(t, graph, "11"), recordOfEvent(t, graph, "15"),
		recordOfEvent(t, graph, "12"), recordOfEvent(t, graph, "22")}
	if edge.state != core.RelationStateCandidate || !slices.Equal(edge.evidence, evidence) {
		t.Errorf("the relation is %q with the evidence %v, want candidate with %v", edge.state, edge.evidence, evidence)
	}
	if len(detail.RecordPairs) != 3 {
		t.Fatalf("the relation carries %d pairs, want the 2 session pairs and the credential pair", len(detail.RecordPairs))
	}
	if name, lefts, _ := sessionPairsOf(t, graph, node("11"), node("22"), detail.RecordPairs[:2]); name != "11/15" ||
		!slices.Equal(lefts, evidence[:2]) {
		t.Errorf("the pairs start from %s %v, want the session start 11 and the logoff 15", name, lefts)
	}
	credential := detail.RecordPairs[2]
	if sides := pairSides(t, graph, credential); sides != [2]int{evidence[2], evidence[3]} ||
		!slices.Equal(conditionKeysOf(credential), []core.EdgePairConditionKey{core.EdgePairConditionAccount,
			core.EdgePairConditionDestinationTerminal, core.EdgePairConditionTimeProximity}) {
		t.Errorf("the credential pair joins %v with %v, want the request 12 and the logon 22",
			sides, conditionKeysOf(credential))
	}
	pair := detail.RecordPairs[0]
	if keys := conditionKeysOf(pair); !slices.Equal(keys, []core.EdgePairConditionKey{core.EdgePairConditionRequestingSession}) {
		t.Errorf("the pair carries the conditions %v", keys)
	}
	if left := comparableValuesOf(pair.Conditions[0].LeftValue); !slices.Equal(left, []string{"0xb1"}) {
		t.Errorf("the requesting session condition carries %v, want the Logon ID of the session", left)
	}
}
