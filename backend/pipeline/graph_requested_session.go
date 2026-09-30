package pipeline

import (
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// terminalLogonId は、端末 1 台のログオンのセッションの Logon ID を指す鍵である。
type terminalLogonId struct {
	terminal, logonId string
}

// subjectLogonIdItems は、要求を行ったセッションの Logon ID を持つ語彙の項目である。
var subjectLogonIdItems = []core.SemanticKey{core.SemanticKeyEventSubjectLogonId}

// addRequestedSessionEdges は、g.edges の firstEdge から末尾までの資格情報を使ったログオンの要求の
// 候補 (explicit_credential_logon だけが並ぶ) から、要求したセッションの始まりのレコードから接続先の
// ログオンのレコードへの関係を足す。要求したセッションで原因が決まったログオンのレコードの
// g.records での位置を返す。
//
// **要求の記録が操作を行ったセッションの Logon ID を持ち、要求の時刻にその Logon ID の
// セッションが要求を記録した端末にあるときだけ結ぶ。** 期間が要求の時刻を含むセッションが
// 2 つ以上あるときは、どれとも結ぶ。Logon ID を持たない要求、固定の値の Logon ID の要求、
// 期間が要求の時刻を含むセッションの無い要求は結ばない。
//
// **関係の状態は candidate である。** 要求と接続先のログオンの組は、アカウントの名前と時刻の範囲で
// 結んだ候補であり、その組を資格情報の組の規則で根拠に残す。
func (g *Graph) addRequestedSessionEdges(sessions []logonSessionPeriod, firstEdge int) map[int]struct{} {
	byLogonId := make(map[terminalLogonId][]logonSessionPeriod)
	for _, session := range sessions {
		key := terminalLogonId{session.terminal, session.logonId}
		byLogonId[key] = append(byLogonId[key], session)
	}
	determined := make(map[int]struct{})
	lastEdge := len(g.edges)
	// firstEdge からのエッジは、呼び出し元が直前に足した explicit_credential_logon だけである。
	for edgeAt := firstEdge; edgeAt < lastEdge; edgeAt++ {
		for _, pair := range g.edges[edgeAt].pairList() {
			request, logon := int(pair.left), int(pair.right)
			subject, carried := sessionLogonIdOf(g.recordFieldsWithSemantics(pair.left, subjectLogonIdItems),
				core.SemanticKeyEventSubjectLogonId)
			if !carried {
				continue
			}
			for _, session := range byLogonId[terminalLogonId{g.records[request].recordingTerminalNodeId, subject}] {
				if !session.contains(g.records[request].instant) {
					continue
				}
				edge := g.ensureEdge(core.EdgeKindRequestedSessionLogon, core.RelationStateCandidate,
					g.records[session.startAt].recordNode, g.records[logon].recordNode)
				g.addSessionBasis(edge, session, pairRuleRequestedSession, pairRuleRequestedSession, candidateTally{}, logon)
				g.addEdgeEvidence(edge, request)
				g.addEdgeEvidence(edge, logon)
				// 要求と接続先のログオンを結んだのは資格情報の組の候補であり、その組を根拠に残す。
				g.addRecordPair(edge, pairRuleExplicitCredential, request, logon)
				determined[logon] = struct{}{}
			}
		}
	}
	return determined
}
