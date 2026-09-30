package pipeline

import (
	"cmp"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sameConnectionToleranceSeconds は、同じ接続を記録した 2 件のレコードの時刻の差として認める
// 秒数の上限である。端末をまたぐ組と、開いた記録か閉じた記録かを読めない記録を含む同じ端末の
// 組に使う。
//
// 既知の制限: 幅を connectionLogonToleranceSeconds と同じ 60 秒の固定の値に置き、60 秒より長く
// 続いた接続の開閉を読めない記録を結ばない, 2 台の端末の時計のずれを測れる値は入力に無く、
// 開閉を読めない記録は port を使い直した別の接続と時刻の近さでしか分けられないため、幅を
// 入力から決められない, 時計のずれが 60 秒を超える組か、60 秒より長く続いた接続の開閉を読めない組の
// 実例が出たとき、分析者が幅を選べるようにする
const sameConnectionToleranceSeconds = connectionLogonToleranceSeconds

// connectionPhase は、接続を記録したレコードが接続を開いた記録か閉じた記録かである。
type connectionPhase uint8

const (
	// connectionPhaseUnknown は、入力形式の観測の種別から開閉を読めない記録である。
	connectionPhaseUnknown connectionPhase = iota
	connectionPhaseOpen
	connectionPhaseClose
)

// sameConnectionKey は、接続を記録したレコードの接続元のアドレスと port と、宛先の port の組である。
type sameConnectionKey struct {
	address, port, destinationPort string
}

// sameConnectionRecord は、接続を記録したレコード 1 件の位置と、記録した宛先のアドレスと時刻と
// 開閉である。受け付けた側の記録は、宛先のアドレスを持たないことがある (hasDestination が偽)。
type sameConnectionRecord struct {
	at             int
	destination    string
	hasDestination bool
	observedAt     core.Timestamp
	phase          connectionPhase
}

// addSameConnectionEdges は、同じ 1 本の接続を記録したレコードの組へ候補のエッジを足す。
// 接続元のアドレスと port と、宛先の port が一致する組だけを比べる。
//
//   - 同じ端末で、宛先のアドレスの記録の有無と値も一致する組のうち、開閉を読める記録は、
//     閉じた記録を、時刻の順でその前にある直近の開いた記録へ結ぶ (addPhasedConnectionEdges)。
//     時刻の許容幅を使わない。
//   - 同じ端末の組のうち片側でも開閉を読めない組は、時刻の差が sameConnectionToleranceSeconds
//     秒以内のときに、時刻の早い記録から遅い記録へ結ぶ。
//   - 端末をまたぐ組は、起点の側の宛先のアドレスの割当が 1 つに決める端末が終点の側を置いた
//     端末であり、終点の側が記録した宛先のアドレスが起点の側と一致し、時刻の差が
//     sameConnectionToleranceSeconds 秒以内のときに、接続した側から受け付けた側へ結ぶ。
//
// 端末に置かれず、時点を持たないレコードは結ばない。
func (g *Graph) addSameConnectionEdges(result ImportResult) {
	connections := g.connectionsByEndpoint(result)
	if len(connections) == 0 {
		return
	}
	assignments := terminalAssignmentsOf(result).comparableEntries()
	for _, records := range connections {
		g.addPhasedConnectionEdges(records)
		// 既知の制限: 同じ組の記録どうしを総当たりで比べる, 1 つの組は接続元の port が同じ記録であり、
		// port を使い直すまでの数件に留まる, 1 つの組の記録が数百件を超える入力で
		// グラフを組む時間が伸びたとき、時刻で並べて許容幅の時刻の範囲で絞る
		for i, left := range records {
			for _, right := range records[i+1:] {
				g.addSameConnectionEdgeOf(left, right, assignments)
			}
		}
	}
}

// connectionsByEndpoint は、接続を記録したレコードを、接続元のアドレスと port と宛先の port で
// 分けた組を返す。組の並びは最初の記録の位置の順であり、エッジを足す順を入力ごとに定める。
func (g Graph) connectionsByEndpoint(result ImportResult) [][]sameConnectionRecord {
	var connections [][]sameConnectionRecord
	positions := make(map[sameConnectionKey]int)
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			if record.Semantics == nil || record.ObservedAt == nil {
				continue
			}
			fields := record.Semantics.Fields
			source, hasSource := clientEndpointOf(fields)
			destinationPort, hasPort := comparableOfSemantic(fields, core.SemanticKeyConnectionDestinationPort)
			at, indexed := g.recordAtLocator(record.Locator)
			if !hasSource || !hasPort || !indexed || !g.records[at].hasInstant || !g.records[at].hasRecordNode ||
				g.records[at].placedTerminalNodeId == "" {
				continue
			}
			destination, hasDestination := comparableOfSemantic(fields, core.SemanticKeyConnectionDestinationAddress)
			key := sameConnectionKey{address: source.address, port: source.port, destinationPort: destinationPort}
			position, seen := positions[key]
			if !seen {
				position = len(connections)
				positions[key] = position
				connections = append(connections, nil)
			}
			connections[position] = append(connections[position], sameConnectionRecord{
				at: at, destination: destination, hasDestination: hasDestination, observedAt: *record.ObservedAt,
				phase: publication.connectionPhaseOf(record.Semantics.ObservationKind),
			})
		}
	}
	return connections
}

// connectionOnTerminal は、同じ端末の同じ 4 つ組の接続を分ける鍵である。
type connectionOnTerminal struct {
	terminal, destination string
	hasDestination        bool
}

// addPhasedConnectionEdges は、開閉を読める記録を時刻の順に並べ、閉じた記録を、同じ端末の
// 同じ 4 つ組でその前にある直近の開いた記録へ結ぶ。結んだ開いた記録は、次の閉じた記録に使わない。
// 時刻の同じ記録は位置の順に並べる。
func (g *Graph) addPhasedConnectionEdges(records []sameConnectionRecord) {
	var phased []sameConnectionRecord
	for _, record := range records {
		if record.phase != connectionPhaseUnknown {
			phased = append(phased, record)
		}
	}
	slices.SortStableFunc(phased, func(left, right sameConnectionRecord) int {
		return cmp.Or(g.records[left.at].instant.Compare(g.records[right.at].instant), cmp.Compare(left.at, right.at))
	})
	openAt := make(map[connectionOnTerminal]int)
	for _, record := range phased {
		key := connectionOnTerminal{
			terminal: g.records[record.at].placedTerminalNodeId, destination: record.destination,
			hasDestination: record.hasDestination,
		}
		if record.phase == connectionPhaseOpen {
			openAt[key] = record.at
			continue
		}
		if open, opened := openAt[key]; opened {
			g.addSameConnectionEdge(pairRuleSameConnectionOnTerminal, open, record.at)
			delete(openAt, key)
		}
	}
}

// addSameConnectionEdgeOf は、同じ組の 2 件の記録が、時刻の許容幅で同じ接続と読めるときに
// 候補のエッジを足す。同じ端末で両側とも開閉を読める組は addPhasedConnectionEdges が扱う。
func (g *Graph) addSameConnectionEdgeOf(
	left, right sameConnectionRecord, assignments []core.TerminalAssignment,
) {
	leftRecord, rightRecord := g.records[left.at], g.records[right.at]
	if difference := leftRecord.instant.Unix() - rightRecord.instant.Unix(); difference > sameConnectionToleranceSeconds ||
		difference < -sameConnectionToleranceSeconds {
		return
	}
	if leftRecord.placedTerminalNodeId == rightRecord.placedTerminalNodeId {
		if (left.phase != connectionPhaseUnknown && right.phase != connectionPhaseUnknown) ||
			left.hasDestination != right.hasDestination || left.destination != right.destination {
			return
		}
		if rightRecord.instant.Before(leftRecord.instant) {
			left, right = right, left
		}
		g.addSameConnectionEdge(pairRuleSameConnectionUnphased, left.at, right.at)
		return
	}
	if g.acceptedBy(left, right, assignments) {
		g.addSameConnectionEdge(pairRuleSameConnectionAcrossTerminals, left.at, right.at)
	} else if g.acceptedBy(right, left, assignments) {
		g.addSameConnectionEdge(pairRuleSameConnectionAcrossTerminals, right.at, left.at)
	}
}

// acceptedBy は、別の端末の記録 accepting が、記録 connecting の接続を受け付けた側の記録で
// あるかを返す。connecting の宛先のアドレスの割当が 1 つに決める端末が accepting を置いた
// 端末であり、accepting が宛先のアドレスを記録するときはその値が一致する。
func (g Graph) acceptedBy(
	connecting, accepting sameConnectionRecord, assignments []core.TerminalAssignment,
) bool {
	if !connecting.hasDestination ||
		(accepting.hasDestination && accepting.destination != connecting.destination) {
		return false
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: connecting.destination, EventTime: connecting.observedAt, Assignments: assignments,
	})
	if err != nil || !resolution.Determined() {
		return false
	}
	terminal, keyed := resolution.Members[0].TerminalNodeKey()
	return keyed && nodeIdOf(terminal) == g.records[accepting.at].placedTerminalNodeId
}

// addSameConnectionEdge は、起点 from の記録から終点 to の記録への候補のエッジを足す。
func (g *Graph) addSameConnectionEdge(rule pairRule, from, to int) {
	edge := g.ensureEdge(core.EdgeKindSameConnectionMatch, core.RelationStateCandidate,
		g.records[from].recordNode, g.records[to].recordNode)
	g.addEdgeEvidence(edge, from)
	g.addEdgeEvidence(edge, to)
	g.addRecordPair(edge, rule, from, to)
}
