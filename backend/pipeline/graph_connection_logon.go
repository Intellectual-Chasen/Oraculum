package pipeline

import (
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// connectionLogonToleranceSeconds は、接続のレコードとログオンのレコードの時刻の差として
// 認める秒数の上限である。
//
// 既知の制限: 幅を固定の値に置く, 接続先の端末のログオンの時刻は、2 台の端末の時計のずれの分だけ
// 接続の時刻より前になりうる。ずれを測れる値は入力に無い,
// 時計のずれが 60 秒を超える組、または同じ接続元 port を 60 秒以内に使い直した組の実例が
// 出たとき、分析者が幅を選べるようにする
const connectionLogonToleranceSeconds = 60

// connectionLogonKey は、ログオンのレコードが記録した接続元のアドレスと port の組である。
type connectionLogonKey struct {
	address string
	port    string
}

// addConnectionLogonEdges は、端末の外向きの接続のレコードから、接続先の端末が記録した
// ログオンのレコードへの候補のエッジを足す。
//
// **接続元のアドレス・port と、ログオンが記録した接続元のアドレス・port の一致で結ぶ。**
// 接続先の端末は、接続先のアドレスの割当 (IP から端末への割当) で決め、接続の時刻が割当の
// 期間の中にあることを確かめる。ログオンのレコードを置いた端末が、その端末であるときだけ
// 結ぶ。2 つの時刻の差は connectionLogonToleranceSeconds 秒以内である。
//
// ログオンのレコードは、ログオンの種別を記録したレコードである。接続のレコードは、接続元の
// アドレス・port と接続先のアドレスを記録したレコードである。接続先だけを記録したレコード
// (資格情報を指定したログオンの要求) は接続にならない。接続とログオンを同じ端末が記録した組は結ばない。
func (g *Graph) addConnectionLogonEdges(result ImportResult) {
	logons := g.logonsByClientEndpoint(result)
	if len(logons) == 0 {
		return
	}
	assignments := terminalAssignmentsOf(result).comparableEntries()
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			g.addConnectionLogonEdgesOf(record, logons, assignments)
		}
	}
}

// logonsByClientEndpoint は、ログオンのレコードの位置を、記録した接続元のアドレスと port で
// 探す表を組む。端末に置かれず、時点を持たないレコードを入れない。
func (g Graph) logonsByClientEndpoint(result ImportResult) map[connectionLogonKey][]int {
	logons := make(map[connectionLogonKey][]int)
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			if record.Semantics == nil {
				continue
			}
			fields := record.Semantics.Fields
			if !carriesSemanticValue(fields, core.SemanticKeyEventLogonType) {
				continue
			}
			key, keyed := clientEndpointOf(fields)
			at, indexed := g.recordAtLocator(record.Locator)
			if !keyed || !indexed || !g.records[at].hasInstant || !g.records[at].hasRecordNode ||
				g.records[at].placedTerminalNodeId == "" {
				continue
			}
			logons[key] = append(logons[key], at)
		}
	}
	return logons
}

// clientEndpointOf は、レコードが記録した接続元のアドレスと port の組を返す。
func clientEndpointOf(fields []core.RecordField) (connectionLogonKey, bool) {
	address, hasAddress := comparableOfSemantic(fields, core.SemanticKeyConnectionSourceAddress)
	port, hasPort := comparableOfSemantic(fields, core.SemanticKeyConnectionSourcePort)
	return connectionLogonKey{address: address, port: port}, hasAddress && hasPort
}

// addConnectionLogonEdgesOf は、接続のレコード 1 件が作る候補のエッジを足す。
func (g *Graph) addConnectionLogonEdgesOf(
	record RecordEntry, logons map[connectionLogonKey][]int, assignments []core.TerminalAssignment,
) {
	if record.Semantics == nil || record.ObservedAt == nil {
		return
	}
	fields := record.Semantics.Fields
	destination, hasDestination := comparableOfSemantic(fields, core.SemanticKeyConnectionDestinationAddress)
	key, keyed := clientEndpointOf(fields)
	if !hasDestination || !keyed || len(logons[key]) == 0 {
		return
	}
	at, indexed := g.recordAtLocator(record.Locator)
	if !indexed || !g.records[at].hasInstant || !g.records[at].hasRecordNode ||
		g.records[at].placedTerminalNodeId == "" {
		return
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: destination, EventTime: *record.ObservedAt, Assignments: assignments,
	})
	// 期間の中の割当が 1 件に決まらない接続先は、端末を決めない。
	if err != nil || !resolution.Determined() {
		return
	}
	destinationTerminal, keyed := resolution.Members[0].TerminalNodeKey()
	if !keyed {
		return
	}
	connectedAt := g.records[at].instant.Unix()
	for _, logonAt := range logons[key] {
		logon := g.records[logonAt]
		if logon.placedTerminalNodeId != nodeIdOf(destinationTerminal) ||
			logon.placedTerminalNodeId == g.records[at].placedTerminalNodeId {
			continue
		}
		if difference := logon.instant.Unix() - connectedAt; difference > connectionLogonToleranceSeconds ||
			difference < -connectionLogonToleranceSeconds {
			continue
		}
		edge := g.ensureEdge(core.EdgeKindConnectionLogonMatch, core.RelationStateCandidate,
			g.records[at].recordNode, logon.recordNode)
		g.addEdgeEvidence(edge, at)
		g.addEdgeEvidence(edge, logonAt)
		g.addRecordPair(edge, pairRuleConnectionLogon, at, logonAt)
	}
}
