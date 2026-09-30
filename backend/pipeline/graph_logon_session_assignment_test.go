// in-package test: 割当の期間の端をまたぐログオンのセッションを、記録した端末の 1 つのセッションとして
// 組むことを確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// hostASessionXML は、host-a が記録した Logon ID 0xc1 のログオン (04:00) とログオフ (04:30) の file を返す。
func hostASessionXML(extra ...string) string {
	events := securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc1", "LogonType", "2")
	for _, event := range extra {
		events += event
	}
	return "<Events>\n" + events +
		securityEventXML("12", "4634", logonHostA, "2001-02-03T04:30:00.000Z", "TargetLogonId", "0xc1") + "</Events>\n"
}

// 割当の期間が host-a のセッションの片方の端だけを含んでも、ログオンからログオフまでが 1 つのセッションで
// あり、期間の中に来た host-a からのログオンは、そのセッションからの連鎖の候補になる。
func TestLogonChainKeepsTheSessionAcrossTheAssignmentBoundary(t *testing.T) {
	for name, clocks := range map[string][3]string{
		"ログオンが期間の中": {"03:50:00", "04:10:00", "04:10:00"},
		"ログオフが期間の中": {"04:20:00", "04:25:00", "04:40:00"},
	} {
		t.Run(name, func(t *testing.T) {
			at := func(clock string) string { return "2001-02-03T" + clock + ".000Z" }
			result := windowsEventSessionResult(t, hostASessionXML(), "<Events>\n"+
				chainLogonXML("21", at(clocks[0]), "user-lead", "198.51.100.9")+
				chainLogonXML("22", at(clocks[1]), "user-b", "192.0.2.10")+
				chainLogonXML("23", at(clocks[2]), "user-tail", "198.51.100.9")+"</Events>\n")
			graph := NewObservedGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
				sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
			}))
			want := []chainEdge{{"11/12", "22", core.RelationStateCandidate, chainLogonLogoff}}
			if got := chainEdgesOf(t, graph); !slices.Equal(got, want) {
				t.Errorf("logon chains = %+v, want %+v", got, want)
			}
		})
	}
}

// 期間の中で host-a を 2 台の端末に割り当てたとき、host-a のセッションはどちらの端末にも置かず、連鎖の
// 候補にならない。
func TestLogonChainSkipsTheSessionWhenTwoTerminalsNameTheHost(t *testing.T) {
	result := windowsEventSessionResult(t, hostASessionXML(), "<Events>\n"+
		chainLogonXML("21", "2001-02-03T03:50:00.000Z", "user-lead", "198.51.100.9")+
		chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10")+"</Events>\n")
	graph := NewObservedGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
		sessionAssignment(t, result, 1, "192.0.2.30", "ws-c", logonHostA, false),
	}))
	if got := chainEdgesOf(t, graph); len(got) != 0 {
		t.Errorf("logon chains = %+v, want none", got)
	}
}

// 要求が割当の期間の中にあり、セッションのログオンとログオフが期間の外にあっても、要求はそのセッションの
// 要求である。
func TestRequestedSessionKeepsTheSessionAcrossTheAssignmentBoundary(t *testing.T) {
	result := windowsEventSessionResult(t,
		hostASessionXML(explicitRequestXML("13", "2001-02-03T04:10:00.000Z", "user-b1", "0xc1")),
		"<Events>\n"+
			hostBLogonXML("21", "2001-02-03T04:05:00.000Z", "other", "192.0.2.10")+
			hostBLogonXML("22", "2001-02-03T04:10:00.500Z", "user-b1", "192.0.2.10")+"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		hostBAssignment(t, result, "192.0.2.20"),
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
	}), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	pairs := edgePairsOfKind(graph, core.EdgeKindRequestedSessionLogon)
	if want := [][2]int{{node("11"), node("22")}}; !slices.Equal(pairs, want) {
		t.Errorf("requested_session_logon = %v, want %v", pairs, want)
	}
}

// ログオンが割当の期間の中、操作が期間の外にあっても、同じ端末の同じ Logon ID のログオンと操作である。
func TestLogonSessionOperationKeepsTheTerminalAcrossTheAssignmentBoundary(t *testing.T) {
	result := windowsEventSessionResult(t,
		hostASessionXML(securityEventXML("13", "4672", logonHostA, "2001-02-03T04:20:00.000Z", "SubjectLogonId", "0xc1")),
		"<Events>\n"+
			chainLogonXML("21", "2001-02-03T03:50:00.000Z", "user-lead", "198.51.100.9")+
			chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-tail", "198.51.100.9")+"</Events>\n")
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
	}), AllMatchConditions())
	if got, want := logonSessionEdgeEnds(t, graph), []string{"4624->4672"}; !slices.Equal(got, want) {
		t.Errorf("the logon session edges are %v, want %v", got, want)
	}
	if got := rejectionReasonsOf(graph); len(got) != 0 {
		t.Errorf("the rejections are %v, want none", got)
	}
}
