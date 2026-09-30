package pipeline

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// addLogonChainEdges は、端末 X を接続元とするログオンのレコードへ、そのログオンの時刻に X で
// ログオン中のセッションの始まりのレコードからの候補を足す。
//
// **ログオンは、接続元のアドレスと、作ったセッションの Logon ID を記録したレコードである。** 失敗した
// ログオンは作ったセッションの Logon ID を持たない。
// 接続元の端末 X は、ログオンの時刻にそのアドレスを持つ端末の割当が 1 台に決まるときの端末であり、
// ログオンを記録した端末と別である。候補は X のセッションのうち、期間がログオンの時刻を含むもので
// ある。匿名のログオンと仮想アカウントのセッションは候補にしない (chainCandidateOf)。候補が 1 つの
// ときは candidate、2 つ以上のときは全候補を uncertain_chain にする。候補は、アカウントの一致と
// ログオンの種別の区分で並べ、区分の条件と、並びでより上の区分に入る候補の数を組の根拠に出す
// (tallyOf)。
// セッションが X のセッションであるかは、ログオンの時刻にセッションを記録した端末を置く端末で
// 決める (sourceTerminals.placementAt)。割当の期間の端をまたぐセッションは、期間の中のログオンに
// だけ候補になる。
//
// 要求したセッションの関係が原因を決めたログオン (determined) と、接続元の割当が 1 台に決まらない
// ログオンと、候補の無いログオンからは作らない。
//
// 既知の制限: ログオン 1 件ごとに接続元の端末に置かれ得るセッションをすべて比べ、期間が含む候補を
// すべて結ぶ, 1 台のセッション 5,000 件と、その端末からのログオン 5,000 件 (同時にログオン中の
// セッションが約 50) で、25 万本を取り込みを含め 4.7 秒・318 MB で組んだ, 所要か記憶域に出たとき、
// セッションを始まりの時刻で並べて比べる範囲を絞る
func (g *Graph) addLogonChainEdges(
	result ImportResult, terminals sourceTerminals, sessions []logonSessionPeriod, determined map[int]struct{},
) {
	// セッションを、記録した端末と、そのホスト名を記録した割当の端末の両方で引けるようにする。
	byTerminal := make(map[string][]logonSessionPeriod)
	for _, session := range sessions {
		start := g.records[session.startAt]
		placements := terminals.assignedTerminalIds(start.locator.SourceId, start.assignableHostname)
		if !slices.Contains(placements, session.terminal) {
			placements = append(placements, session.terminal)
		}
		for _, terminal := range placements {
			byTerminal[terminal] = append(byTerminal[terminal], session)
		}
	}
	assignments := terminalAssignmentsOf(result).comparableEntries()
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			logon, source, chained := g.chainedLogonOf(record, assignments, determined)
			if !chained {
				continue
			}
			var candidates []chainCandidate
			instant := g.records[logon].instant
			for _, session := range byTerminal[source] {
				start := g.records[session.startAt]
				if session.contains(instant) && terminals.placementAt(start.locator.SourceId, start.assignableHostname,
					session.terminal, instant) == source {
					if candidate, kept := g.chainCandidateOf(session, logon); kept {
						candidates = append(candidates, candidate)
					}
				}
			}
			state := core.RelationStateCandidate
			if len(candidates) > 1 {
				state = core.RelationStateUncertainChain
			}
			for _, candidate := range candidates {
				session := candidate.session
				edge := g.ensureEdge(core.EdgeKindLogonChain, state,
					g.records[session.startAt].recordNode, g.records[logon].recordNode)
				g.addSessionBasis(edge, session,
					logonChainRule(session.startKind, session.endKind, candidate.accountMatch, candidate.class),
					logonChainEndRule(session.startKind, session.endKind), tallyOf(candidate, candidates), logon)
				g.addEdgeEvidence(edge, logon)
			}
		}
	}
}

// chainCandidate は、ログオン 1 件に挙がったセッション 1 つと、並べる区分である。
type chainCandidate struct {
	session      logonSessionPeriod
	accountMatch bool
	class        sessionLogonClass
}

// tier は候補の区分の順である。小さいほど並びの上に入る。
func (c chainCandidate) tier() uint8 {
	tier := uint8(c.class)
	if !c.accountMatch {
		tier += uint8(sessionLogonClassCount)
	}
	return tier
}

// tallyOf は、candidates の数と、そのうち並びで candidate より上の区分に入る候補の数と、candidate の
// 区分の順を返す。
func tallyOf(candidate chainCandidate, candidates []chainCandidate) candidateTally {
	preceding := 0
	for _, other := range candidates {
		if other.tier() < candidate.tier() {
			preceding++
		}
	}
	// 候補の数が上限で止まったときも、上の区分の候補の数を候補の数より小さく保つ。
	count := saturatedUint16(len(candidates))
	return candidateTally{candidates: count, preceding: min(saturatedUint16(preceding), count-1),
		tier: candidate.tier()}
}

// candidateTierOf は、エッジ edge のセッションが候補を並べた区分の順の最小と、その区分を決めた
// 条件を返す。ok が偽になるのは、区分で並べた候補のセッションをエッジが持たないときである。
func candidateTierOf(edge graphEdge) (tier int, conditions []core.EdgePairConditionKey, ok bool) {
	tier, ok = candidateTierNumberOf(edge)
	if !ok {
		return 0, nil, false
	}
	account := core.EdgePairConditionSessionAccountMatch
	if tier >= int(sessionLogonClassCount) {
		account = core.EdgePairConditionSessionAccountDifferent
	}
	return tier, []core.EdgePairConditionKey{
		account, sessionLogonConditions[tier%int(sessionLogonClassCount)],
	}, true
}

// candidateTierNumberOf は、エッジ edge のセッションが候補を並べた区分の順の最小を返す。ok が偽に
// なるのは、区分で並べた候補のセッションをエッジが持たないときである。
func candidateTierNumberOf(edge graphEdge) (tier int, ok bool) {
	for _, session := range edge.sessionList() {
		if session.candidates > 0 && (!ok || int(session.tier) < tier) {
			tier, ok = int(session.tier), true
		}
	}
	return tier, ok
}

// sortByCandidateTier は、エッジの位置の並び edges を、候補を並べた区分の順で安定に並べ替える。
// 区分を持たないエッジを先に元の順で置き、区分を持つエッジを区分の順に続ける。接続元のセッションの
// 側から開いたときも、上の区分の候補が先に出る。区分はエッジごとに 1 回だけ求める。区分を持つ
// エッジが 1 本も無いときは並べ替えず、偽を返す。
func (g Graph) sortByCandidateTier(edges []int) bool {
	type keyed struct{ edge, tier int }
	items := make([]keyed, len(edges))
	tiered := false
	for index, at := range edges {
		items[index] = keyed{at, -1}
		if tier, found := candidateTierNumberOf(g.edges[at]); found {
			items[index].tier, tiered = tier, true
		}
	}
	if !tiered {
		return false
	}
	slices.SortStableFunc(items, func(left, right keyed) int { return cmp.Compare(left.tier, right.tier) })
	for index, item := range items {
		edges[index] = item.edge
	}
	return true
}

// saturatedUint16 は 0 以上の数を uint16 の上限で止めて返す。
func saturatedUint16(count int) uint16 {
	if count < 0 || count > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(count)
}

// anonymousAccountSid は、匿名のログオン (ANONYMOUS LOGON) のアカウントの SID である。
const anonymousAccountSid = "s-1-5-7"

// anonymousAccountName は、匿名のログオンのアカウントの名前である。
const anonymousAccountName = "anonymous logon"

// virtualAccountSidPrefixes は、画面の描画 (Window Manager の DWM-n) とフォントのドライバ
// (UMFD-n) の仮想アカウントの SID の接頭辞である。比べる値は小文字である。
var virtualAccountSidPrefixes = []string{"s-1-5-90-", "s-1-5-96-"}

// interactiveLogonTypes は、対話と画面の遠隔操作のログオンの種別のコードである。12 と 13 は
// キャッシュした資格情報の画面の遠隔操作とロックの解除である。
var interactiveLogonTypes = []string{"2", "7", "10", "11", "12", "13"}

// chainCandidateOf は、ログオン logon に挙がったセッション session を並べる区分を返す。kept が偽に
// なるのは、セッションのアカウントが匿名のログオンか、画面の描画とフォントのドライバの仮想
// アカウントのときである (serviceOnlyAccount)。どちらのセッションも利用者の操作を持たず、別の
// 端末へログオンする元にならない。
//
// **アカウントは SID か名前が一致すれば一致とする。** 名前はドメインを比べず、大文字と小文字を
// 区別しない。種別 9 のログオンが他の端末への接続に使うアカウントも比べる。
//
// 割り切り: 同じ名前の別のドメインのアカウントを一致とする, 要求と接続先のログオンはドメインを
// 別の文字列 (短い名前・FQDN) で書くことがある, 別のドメインの同名のアカウントを候補の先に並べた
// 入力を確認したとき、ドメインの文字列をそろえる規則を足す
func (g Graph) chainCandidateOf(session logonSessionPeriod, logon int) (chainCandidate, bool) {
	sessionAccounts := accountValuesOf(g.fieldsAt(session.startAt, sessionAccountItems(session.startKind)))
	if serviceOnlyAccount(sessionAccounts) {
		return chainCandidate{}, false
	}
	candidate := chainCandidate{session: session, class: sessionLogonOther}
	for value := range accountValuesOf(g.fieldsAt(logon, chainedLogonAccountItems)) {
		if _, shared := sessionAccounts[value]; shared {
			candidate.accountMatch = true
		}
	}
	// 最初の操作の記録から始まるセッションは、ログオンの種別を記録していない。
	if session.startKind == sessionStartFirstOperation {
		return candidate, true
	}
	logonType, _ := comparableOfSemantic(g.fieldsAt(session.startAt,
		[]core.SemanticKey{core.SemanticKeyEventLogonType}), core.SemanticKeyEventLogonType)
	switch {
	case slices.Contains(interactiveLogonTypes, logonType):
		candidate.class = sessionLogonInteractive
	case logonType == networkLogonType:
		candidate.class = sessionLogonNetwork
	}
	return candidate, true
}

// serviceOnlyAccount は、アカウントの SID と名前の集合 (accountValuesOf) が、匿名のログオンか、
// 画面の描画とフォントのドライバの仮想アカウントを含むかを返す。
func serviceOnlyAccount(accounts map[string]struct{}) bool {
	if _, anonymous := accounts[anonymousAccountSid]; anonymous {
		return true
	}
	if _, anonymous := accounts[anonymousAccountName]; anonymous {
		return true
	}
	for value := range accounts {
		for _, prefix := range virtualAccountSidPrefixes {
			if strings.HasPrefix(value, prefix) {
				return true
			}
		}
	}
	return false
}

// fieldsAt は、g.records の位置 at のレコードの欄のうち、語彙の項目が semantics に入るものを返す
// (recordFieldsWithSemantics)。int32 に収まらない位置では要素数 0 である。
func (g Graph) fieldsAt(at int, semantics []core.SemanticKey) []core.RecordField {
	compact, fits := compactIndex(at)
	if !fits {
		return nil
	}
	return g.recordFieldsWithSemantics(compact, semantics)
}

// accountValuesOf は、アカウントの SID と名前の欄の比べる値を小文字にした集合を返す。ドメインの
// 欄と、値の無い文字列 (`-`) は入れない。
func accountValuesOf(fields []core.RecordField) map[string]struct{} {
	values := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		switch field.Semantic {
		case core.SemanticKeyTargetAccountDomain, core.SemanticKeySubjectAccountDomain,
			core.SemanticKeyEventOutboundAccountDomain:
			continue
		}
		if value, readable := comparableOfSemantic([]core.RecordField{field}, field.Semantic); readable &&
			value != "" && value != "-" {
			values[strings.ToLower(value)] = struct{}{}
		}
	}
	return values
}

// chainedLogonOf は、record がログオンの連鎖の終点になるログオンであるとき、その g.records での
// 位置と、接続元の端末のノードの識別子を返す (addLogonChainEdges)。
func (g Graph) chainedLogonOf(
	record RecordEntry, assignments []core.TerminalAssignment, determined map[int]struct{},
) (int, string, bool) {
	fields := graphFieldsOf(record)
	address, hasAddress := comparableOfSemantic(fields, core.SemanticKeyConnectionSourceAddress)
	if _, logged := sessionLogonIdOf(fields, core.SemanticKeyEventTargetLogonId); !logged || !hasAddress {
		return 0, "", false
	}
	// hasInstant が真のレコードは ObservedAt を持つ (addGraphRecord)。
	at, indexed := g.recordAtLocator(record.Locator)
	if _, caused := determined[at]; !indexed || caused || !g.records[at].hasRecordNode || !g.records[at].hasInstant {
		return 0, "", false
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: address, EventTime: *record.ObservedAt, Assignments: assignments,
	})
	if err != nil || !resolution.Determined() {
		return 0, "", false
	}
	source, keyed := resolution.Members[0].TerminalNodeKey()
	if !keyed || nodeIdOf(source) == g.records[at].placedTerminalNodeId {
		return 0, "", false
	}
	return at, nodeIdOf(source), true
}
