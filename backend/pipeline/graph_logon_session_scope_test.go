// in-package test: イベントビューアーの CSV のログオンと、案件と端末の割当が決める Logon ID の
// 関連付けの範囲を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// viewerHeader は Windows のイベントビューアーが書く CSV の見出しである。
const viewerHeader = "\xef\xbb\xbf" + "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"

// viewerLogonCSV はログオンの成功の論理レコード 1 件を返す。見出しと項目名は
// Windows が 4624 の説明に書く文字列である。
func viewerLogonCSV(at, logonType, logonId, address string) string {
	return "情報," + at + ",Microsoft-Windows-Security-Auditing,4624,Logon,\"アカウントが正常にログオンしました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\tHOST01$\n\tログオン ID:\t\t0x3e7\n\n" +
		"ログオン タイプ:\t\t\t" + logonType + "\n\n" +
		"新しいログオン:\n\tアカウント名:\t\tuser05\n\tアカウント ドメイン:\t\tCORP-TEST\n\tログオン ID:\t\t" + logonId + "\n\n" +
		"ネットワーク情報:\n\tワークステーション名:\tCLIENT05\n\tソース ネットワーク アドレス:\t" + address +
		"\n\tソース ポート:\t\t50005\"\n"
}

// viewerPrivilegesCSV は特権の割り当ての論理レコード 1 件を返す。
func viewerPrivilegesCSV(at, logonId string) string {
	return "情報," + at + ",Microsoft-Windows-Security-Auditing,4672,Special Logon,\"新しいログオンに特権が割り当てられました。\n\n" +
		"サブジェクト:\n\tアカウント名:\t\tuser05\n\tログオン ID:\t\t" + logonId + "\n\n特権:\t\tSeDebugPrivilege\"\n"
}

// logonSessionEdgeEnds は、logon_session_operation のエッジを「起点の種別->終点の種別」の
// 観測の種別の EventID で並べる。
func logonSessionEdgeEnds(t *testing.T, graph Graph) []string {
	t.Helper()
	var ends []string
	for _, edge := range edgesOfKind(graph, core.EdgeKindLogonSessionOperation) {
		ends = append(ends, eventIdOfNode(t, graph, edge.source)+"->"+eventIdOfNode(t, graph, edge.target))
	}
	slices.Sort(ends)
	return ends
}

// eventIdOfNode はレコードのノードの根拠のレコードの観測の種別から EventID を返す。
func eventIdOfNode(t *testing.T, graph Graph, at int) string {
	t.Helper()
	node := graph.nodes[at]
	if node.key.Kind != core.NodeKindRecord || len(node.evidence) == 0 {
		t.Fatalf("the node %d is not a record node", at)
	}
	for _, field := range graph.records[node.evidence[0]].observationKind.Raw {
		if field.Name == "EventID" {
			value, _ := field.Text.ComparableValue()
			return value
		}
	}
	t.Fatalf("the record node %d carries no EventID", at)
	return ""
}

// rejectionReasonsOf は、グラフのすべてのレコードのノードが持つ不成立の理由を並べる。
func rejectionReasonsOf(graph Graph) []core.LogonSessionRejectionReason {
	var reasons []core.LogonSessionRejectionReason
	for _, rejections := range graph.logonSessionRejections {
		for _, rejection := range rejections {
			reasons = append(reasons, rejection.reason)
		}
	}
	return reasons
}

// イベントビューアーの CSV のログオンから、同じ file の特権の割り当てへ候補を結ぶ。CSV の地方時は
// 同じ端末の壁時計の日時として前後を比べる。4624 は種別のコードと接続元を持つ。
func TestLogonSessionEdgesLinkViewerCSVRecords(t *testing.T) {
	document := viewerHeader +
		viewerLogonCSV("2001/02/03 04:05:06", "10", "0x00000000000B0C0D", "192.0.2.50") +
		viewerPrivilegesCSV("2001/02/03 04:05:06", "0xb0c0d")
	graph := NewGraph(caseImport(t, caseSource{name: "exported-events.csv",
		format: string(WindowsEventCSVFormatKey), content: document}), AllMatchConditions())
	if got, want := logonSessionEdgeEnds(t, graph), []string{"4624->4672"}; !slices.Equal(got, want) {
		t.Errorf("the logon session edges are %v, want %v", got, want)
	}
	edges := edgesOfKind(graph, core.EdgeKindLogonSessionOperation)
	if len(edges) == 0 {
		t.Fatal("no logon session edge")
	}
	logon := graph.records[graph.nodes[edges[0].source].evidence[0]]
	if logonType, readable := comparableTextOf(*logon.logonType); !readable || logonType != "10" {
		t.Errorf("the logon type of the CSV logon is %q (%t), want 10", logonType, readable)
	}
	address := nodeWithIdentity(t, graph, core.NodeKindIp, "192.0.2.50")
	if !hasEdge(graph, core.EdgeKindRecordNamesObject, edges[0].source, address) {
		t.Error("the CSV logon does not name the source address")
	}
}

// 別の案件にある同じ Logon ID のログオンと操作は、候補にも不成立にも出ない。同じ案件なら、
// 別の file の端末どうしの不成立として出る。
func TestLogonSessionStaysWithinACase(t *testing.T) {
	logon := viewerHeader + viewerLogonCSV("2001/02/03 04:05:06", "2", "0xb0c0d", "192.0.2.51")
	operation := viewerHeader + viewerPrivilegesCSV("2001/02/03 04:05:07", "0xb0c0d")
	for name, tc := range map[string]struct {
		logonCase, operationCase *string
		want                     []core.LogonSessionRejectionReason
	}{
		"同じ案件": {caseOf("case-a"), caseOf("case-a"), []core.LogonSessionRejectionReason{
			core.LogonSessionRejectionOtherTerminal}},
		"別の案件": {caseOf("case-a"), caseOf("case-b"), nil},
	} {
		t.Run(name, func(t *testing.T) {
			graph := NewGraph(caseImport(t,
				caseSource{name: "logon.csv", format: string(WindowsEventCSVFormatKey), content: logon, caseId: tc.logonCase},
				caseSource{name: "operation.csv", format: string(WindowsEventCSVFormatKey), content: operation,
					caseId: tc.operationCase}), AllMatchConditions())
			if edges := logonSessionEdgeEnds(t, graph); len(edges) != 0 {
				t.Errorf("the files of two terminals are linked by %v", edges)
			}
			if got := rejectionReasonsOf(graph); !slices.Equal(got, tc.want) {
				t.Errorf("the rejections are %v, want %v", got, tc.want)
			}
		})
	}
}

// 分析者が 2 つの file に同じ端末の外部識別子を与えても、Logon ID は同じ収集元の中でだけ比べ、
// 収集元が違う組として残る。外部識別子を持たない割当は file ごとの端末を保ち、端末が違う組の
// ままである。
func TestLogonSessionStaysWithinASource(t *testing.T) {
	result := caseImport(t,
		caseSource{name: "logon.csv", format: string(WindowsEventCSVFormatKey),
			content: viewerHeader + viewerLogonCSV("2001/02/03 04:05:06", "2", "0xb0c0d", "192.0.2.52")},
		caseSource{name: "operation.csv", format: string(WindowsEventCSVFormatKey),
			content: viewerHeader + viewerPrivilegesCSV("2001/02/03 04:05:07", "0xb0c0d")})
	for name, tc := range map[string]struct {
		terminalId string
		edges      []string
		rejections []core.LogonSessionRejectionReason
	}{
		"外部識別子を持つ割当":   {lineageTerminalId, nil, []core.LogonSessionRejectionReason{core.LogonSessionRejectionOtherSource}},
		"外部識別子を持たない割当": {"", nil, []core.LogonSessionRejectionReason{core.LogonSessionRejectionOtherTerminal}},
	} {
		t.Run(name, func(t *testing.T) {
			assignments := make([]core.TerminalAssignment, 0, len(result.publications))
			for at := range result.publications {
				single := result
				single.publications = result.publications[at : at+1]
				assignment := analystAssignmentFor(t, single)
				assignment.TerminalId = tc.terminalId
				if err := assignment.Validate(); err != nil {
					t.Fatalf("building the assignment: %v", err)
				}
				assignments = append(assignments, assignment)
			}
			graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
			if got := logonSessionEdgeEnds(t, graph); !slices.Equal(got, tc.edges) {
				t.Errorf("the logon session edges are %v, want %v", got, tc.edges)
			}
			if got := rejectionReasonsOf(graph); !slices.Equal(got, tc.rejections) {
				t.Errorf("the rejections are %v, want %v", got, tc.rejections)
			}
		})
	}
}
