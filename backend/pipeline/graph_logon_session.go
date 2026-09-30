package pipeline

import (
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sharedLogonIds は、端末の上で OS が固定の値として持つログオンセッションの Logon ID の
// 比べる値である。0x3e7 は SYSTEM、0x3e6 は匿名のログオン、0x3e5 は LOCAL SERVICE、
// 0x3e4 は NETWORK SERVICE、0x3e3 は IUSR のセッションであり、0x0 はセッションを持たない
// 事象が書く値である。
//
// 既知の制限: 固定の値の Logon ID を関係の鍵にしない, 固定の値のセッションは端末を起動して
// から止めるまで続き、その間のサービスの操作のすべてが同じ値を書く。結ぶと起動のときの
// 1 件のログオンに無関係な操作が集まり、分析者は利用者のログオンの関係を読めない,
// 固定の値のセッションの操作を起動の時刻と結ぶ要求が出たとき
var sharedLogonIds = []string{"0x0", "0x3e3", "0x3e4", "0x3e5", "0x3e6", "0x3e7"}

// logonSessionSide は、Logon ID を持つレコード 1 件である。
type logonSessionSide struct {
	// recordAt はグラフの根拠のレコードの位置である。
	recordAt int
	// terminal はレコードを置く端末のノードの識別子である。
	terminal string
	// source はレコードの収集元の識別子である。Logon ID は同じ収集元の中でだけ比べる。
	source string
	// logonId は Logon ID の比べる値である。
	logonId string
	// linkedLogonId は、ログオンのレコードが書いた、分けたログオンのもう一方の Logon ID の
	// 比べる値である。書いていないレコードと、固定の値 (sharedLogonIds) では空の文字列である。
	linkedLogonId string
	// at は事象の時刻である。timed が偽のときは値を持たない。
	at time.Time
	// localClock は、at が UTC からのずれを持たない壁時計の日時であるかである。
	localClock bool
	timed      bool
}

// logonSessionRejection は、関係にしなかったログオンのレコード 1 件と理由である。
type logonSessionRejection struct {
	reason core.LogonSessionRejectionReason
	// logonAt はログオンのレコードの g.records での位置である。
	logonAt int
}

// addLogonSessionEdges は、ログオンのレコードから、そのログオンが作ったセッションで行った
// 操作のレコードへの候補のエッジを足す。
//
// **同じ端末の同じ収集元のレコードの Logon ID の一致と、時刻の前後だけで結ぶ。** ログオンの
// event.target_logon_id と操作の event.subject_logon_id が一致し、操作の時刻がログオンの
// 時刻より前でない組が候補である。同じ時刻の組を結ぶのは、ログオンと同じ時刻に特権の
// 割り当てを記録するためである。
//
// **Logon ID が一致しながら結ばなかった組を、理由とともに操作のレコードのノードに残す**
// (logonSessionRejectionsOf)。端末が違う組、収集元が違う組、ログオンが操作より後の組、時刻の
// 前後を比べられない組である。
//
// 既知の制限: 1 台の端末の記録を 2 つ以上の file に分けて書き出した入力でも、file をまたぐ
// ログオンと操作を結ばない, 同じ記録を写した 2 つの file (CSV と XML) を結ぶと、同じ操作が
// 2 本の候補を持ち、file の違いを見分ける材料が Logon ID に無い, 分けた file を 1 つの収集元に
// まとめて取り込む要求が出たとき
//
// **同じ Logon ID のログオンが同じ端末に 2 件以上あるときは、条件を満たすすべてと結ぶ。**
// 端末を起動し直した後の Logon ID は、前の起動の値と同じになりうる。1 件に決める根拠を
// 持たない。
//
// 既知の制限: 不成立の組をグラフを組むときにすべて記録する。同じ Logon ID を H 台の端末が
// 書くと、端末が違う組は操作の件数 M に対して M×(H-1) 件になる,
// 所要と記憶域は不成立の件数に比例し、repo の fixture の大きさでは差が出ず測れない,
// 不成立の件数が所要か記憶域に出たとき、不成立の記録を Logon ID からログオンを探す索引に
// 替え、ノードの詳細の要求の時に組む
func (g *Graph) addLogonSessionEdges(result ImportResult, terminals sourceTerminals) {
	logons, operations := g.logonSessionSidesOf(result, terminals)
	// 分けたログオンの 2 件は、どちらも操作を持たないことがある。操作の有無で戻る前に結ぶ。
	g.addLinkedLogonEdges(logons)
	if len(logons) == 0 || len(operations) == 0 {
		return
	}
	for _, operation := range operations {
		for _, logon := range logons[operation.logonId] {
			if logon.recordAt == operation.recordAt {
				continue
			}
			if reason, rejected := logonSessionRejectionOf(logon, operation); rejected {
				g.rejectLogonSession(operation.recordAt, logonSessionRejection{reason: reason, logonAt: logon.recordAt})
				continue
			}
			edge := g.ensureEdge(core.EdgeKindLogonSessionOperation, core.RelationStateCandidate,
				g.records[logon.recordAt].recordNode, g.records[operation.recordAt].recordNode)
			g.addEdgeEvidence(edge, logon.recordAt)
			g.addEdgeEvidence(edge, operation.recordAt)
			g.addRecordPair(edge, pairRuleLogonSession, logon.recordAt, operation.recordAt)
		}
	}
}

// addLinkedLogonEdges は、1 つのログオンが分けて作った 2 件のログオンのレコードを結ぶ候補の
// エッジを足す。
//
// **同じ端末の同じ収集元の 2 件のログオンの Logon ID と、分けたログオンの Logon ID が互いを指す組だけを
// 結ぶ。** 起点は先に取り込んだ側であり、1 つの組から 1 本だけを張る。エッジは起点を
// 取り込んだ順に足す。
//
// 既知の制限: 一方向だけ指す組を結ばず、理由も残さない。時刻も比べない, Windows は分けた 2 件の
// どちらにも相手を書く。一方向だけの一致は端末を起動し直した後の Logon ID の再利用でありうる。
// 一方向だけの組の件数は測っていない, 片方のログオンが欠けた記録を扱う要求が出たとき
func (g *Graph) addLinkedLogonEdges(logons map[string][]logonSessionSide) {
	var linking []logonSessionSide
	for _, sides := range logons {
		for _, side := range sides {
			if side.linkedLogonId != "" {
				linking = append(linking, side)
			}
		}
	}
	slices.SortFunc(linking, func(a, b logonSessionSide) int { return a.recordAt - b.recordAt })
	for _, side := range linking {
		for _, partner := range logons[side.linkedLogonId] {
			if partner.linkedLogonId != side.logonId || partner.terminal != side.terminal ||
				partner.source != side.source || partner.recordAt <= side.recordAt {
				continue
			}
			edge := g.ensureEdge(core.EdgeKindLinkedLogon, core.RelationStateCandidate,
				g.records[side.recordAt].recordNode, g.records[partner.recordAt].recordNode)
			g.addEdgeEvidence(edge, side.recordAt)
			g.addEdgeEvidence(edge, partner.recordAt)
			g.addRecordPair(edge, pairRuleLinkedLogon, side.recordAt, partner.recordAt)
		}
	}
}

// logonSessionRejectionOf は、Logon ID が一致した組を関係にしない理由を返す。
// rejected が偽になるのは、関係にする組である。
func logonSessionRejectionOf(
	logon, operation logonSessionSide,
) (reason core.LogonSessionRejectionReason, rejected bool) {
	if logon.terminal != operation.terminal {
		return core.LogonSessionRejectionOtherTerminal, true
	}
	if logon.source != operation.source {
		return core.LogonSessionRejectionOtherSource, true
	}
	return sameTerminalOrderRejectionOf(logon, operation)
}

// sameTerminalOrderRejectionOf は、前の記録 earlier と後の記録 later を、端末と時刻の前後で
// 結ばない理由を返す。収集元は比べない。
func sameTerminalOrderRejectionOf(
	earlier, later logonSessionSide,
) (reason core.LogonSessionRejectionReason, rejected bool) {
	if earlier.terminal != later.terminal {
		return core.LogonSessionRejectionOtherTerminal, true
	}
	// 既知の制限: 解釈を持たない同じ端末の壁時計の日時は、書いた時計の前後の順に並ぶとみなす,
	// 壁時計の日時は夏時間の切り替えと時計の変更の時点を持たず、前後の逆転を検出できない,
	// 解釈を持たない時刻の並べ方を変えるとき
	if !earlier.timed || !later.timed || earlier.localClock != later.localClock {
		return core.LogonSessionRejectionTimeNotComparable, true
	}
	if later.at.Before(earlier.at) {
		return core.LogonSessionRejectionLogonAfterOperation, true
	}
	return "", false
}

// rejectLogonSession は、操作のレコードのノードに、関係にしなかったログオンを 1 件足す。
func (g *Graph) rejectLogonSession(operationAt int, rejection logonSessionRejection) {
	if g.logonSessionRejections == nil {
		g.logonSessionRejections = make(map[int][]logonSessionRejection)
	}
	node := g.records[operationAt].recordNode
	g.logonSessionRejections[node] = append(g.logonSessionRejections[node], rejection)
}

// logonSessionSidesOf は、公開中の収集元のレコードから、ログオンを Logon ID で探す表と、
// 操作の並びを組む。1 件のレコードが両方に入ることがある。ログオンを要求したセッションを
// 記録したログオンのレコードは、そのセッションの操作でもある。
//
// **レコードのノードを持たないレコードと、端末を決められないレコードを入れない。** 関係の
// 両端はレコードのノードであり、端末の範囲の外では Logon ID が一意でない。
func (g Graph) logonSessionSidesOf(
	result ImportResult, terminals sourceTerminals,
) (map[string][]logonSessionSide, []logonSessionSide) {
	logons := make(map[string][]logonSessionSide)
	var operations []logonSessionSide
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			// 端末は、割当で置き場所を差し替える前の、レコードを記録した端末である (recordingScope)。
			supplied, _ := terminals.forRecord(record)
			scope := terminals.recordingScope(record)
			if len(supplied) > 0 {
				record.Terminal = append(slices.Clone(record.Terminal), supplied...)
			}
			fields := graphFieldsOf(record)
			target, isLogon := sessionLogonIdOf(fields, core.SemanticKeyEventTargetLogonId)
			subject, isOperation := sessionLogonIdOf(fields, core.SemanticKeyEventSubjectLogonId)
			if !isLogon && !isOperation {
				continue
			}
			side, built := g.logonSessionSideOf(record, fields, scope)
			if !built {
				continue
			}
			if isLogon {
				side.logonId = target
				side.linkedLogonId, _ = sessionLogonIdOf(fields, core.SemanticKeyEventTargetLinkedLogonId)
				logons[target] = append(logons[target], side)
				side.linkedLogonId = ""
			}
			if isOperation {
				side.logonId = subject
				operations = append(operations, side)
			}
		}
	}
	return logons, operations
}

// sessionLogonIdOf は、語彙の項目が持つ Logon ID の比べる値を返す。ok が偽になるのは、値を
// 比べられないときと、端末に共通の固定の値のときである (sharedLogonIds)。
func sessionLogonIdOf(fields []core.RecordField, semantic core.SemanticKey) (string, bool) {
	logonId, readable := comparableOfSemantic(fields, semantic)
	if !readable || slices.Contains(sharedLogonIds, logonId) {
		return "", false
	}
	return logonId, true
}

// logonSessionSideOf は、レコード 1 件の端末と時刻を組む。built が偽になるのは、レコードの
// ノードをグラフから探せないときと、端末を決められないときである。
func (g Graph) logonSessionSideOf(
	record RecordEntry, fields []core.RecordField, scope core.RecordScope,
) (logonSessionSide, bool) {
	at, indexed := g.recordAtLocator(record.Locator)
	if !indexed || !g.records[at].hasRecordNode {
		return logonSessionSide{}, false
	}
	terminal, scoped := core.RecordTerminalNodeKey(fields, scope)
	if !scoped {
		return logonSessionSide{}, false
	}
	side := logonSessionSide{recordAt: at, terminal: nodeIdOf(terminal), source: record.Locator.SourceId}
	if record.ObservedAt != nil {
		side.at, side.timed = record.ObservedAt.Instant()
		if !side.timed {
			side.at, side.localClock = record.ObservedAt.LocalClockTime()
			side.timed = side.localClock
		}
	}
	return side, true
}

// logonSessionRejectionsOf は、操作のレコードのノード 1 つについて、Logon ID が一致しながら
// 関係にしなかったログオンのレコードと理由を返す。並びはログオンのレコードを取り込んだ順で
// ある。関係にしなかった組を持たないノードでは要素数 0 である。
func (g Graph) logonSessionRejectionsOf(nodeId string) []core.LogonSessionRejection {
	index, found := g.nodeAt[nodeId]
	if !found {
		return nil
	}
	held := g.logonSessionRejections[index]
	rejections := make([]core.LogonSessionRejection, 0, len(held))
	for _, rejection := range held {
		rejections = append(rejections, core.LogonSessionRejection{
			Reason: rejection.reason, Logon: g.evidenceItems([]int{rejection.logonAt})[0],
		})
	}
	return rejections
}
