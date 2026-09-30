package pipeline

import (
	"math"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// pairRule は、観測の層が候補の関係を作った規則である。規則ごとに、成立に用いた条件と、
// 条件が両側のレコードから読む語彙の項目が決まる (pairRules)。
type pairRule uint8

const (
	pairRuleProcessParent pairRule = iota
	pairRuleProcessIdentity
	pairRuleLogonSession
	pairRuleLinkedLogon
	pairRuleTaskRun
	pairRuleTicketLogon
	pairRuleConnectionLogon
	pairRuleExplicitCredential
	pairRuleAccountIdentity
	pairRuleUnidentifiedSource
	pairRuleSourceOutsideAssignmentRange
	pairRuleSameConnectionOnTerminal
	pairRuleSameConnectionUnphased
	pairRuleSameConnectionAcrossTerminals
	pairRuleRequestedSession
	// pairRuleLogonChain は、ログオンの連鎖の規則の先頭である。**最後に置く。**
	//
	// 先頭から logonChainSessionRules 個は、セッションの始まりと終わりを決めた記録の種類の組ごとの
	// 規則であり、終わりの記録と終点の組が使う (logonChainEndRule)。続く規則は、その組に
	// アカウントの一致とログオンの種別の区分を足した規則であり、始まりの記録と終点の組が使う
	// (logonChainRule)。アカウントとログオンの種別は始まりの記録から読む。
	pairRuleLogonChain
)

// logonChainSessionRules は、始まりと終わりを決めた記録の種類の組の数である。
const logonChainSessionRules = pairRule(sessionStartKindCount) * pairRule(sessionEndKindCount)

// 規則の番号の最後が uint8 に収まることを compile で確かめる。収まらないと配列の長さが負になる。
var _ [math.MaxUint8 - (int(pairRuleLogonChain) + int(logonChainSessionRules)*(1+2*int(sessionLogonClassCount)) - 1)]struct{}

// sessionLogonClass は、セッションを始めたログオンの種別の区分である。並びは候補を並べる順である。
type sessionLogonClass uint8

const (
	sessionLogonInteractive sessionLogonClass = iota
	sessionLogonOther
	sessionLogonNetwork
	sessionLogonClassCount
)

// logonChainEndRule は、始まりと終わりを決めた記録の種類の組の、終わりの記録と終点の組の規則を返す。
func logonChainEndRule(start sessionStartKind, end sessionEndKind) pairRule {
	return pairRuleLogonChain + pairRule(start)*pairRule(sessionEndKindCount) + pairRule(end)
}

// logonChainRule は、始まりと終わりを決めた記録の種類、アカウントの一致、ログオンの種別の区分の
// 組の、始まりの記録と終点の組の規則を返す。
func logonChainRule(start sessionStartKind, end sessionEndKind, accountMatch bool, class sessionLogonClass) pairRule {
	session := logonChainEndRule(start, end) - pairRuleLogonChain
	account := pairRule(0)
	if accountMatch {
		account = 1
	}
	return pairRuleLogonChain + logonChainSessionRules + (session*2+account)*pairRule(sessionLogonClassCount) +
		pairRule(class)
}

// sessionStartConditions と sessionEndConditions は、始まりと終わりを決めた記録の種類ごとの条件である。
var (
	sessionStartConditions = [sessionStartKindCount]pairCondition{
		sessionStartLogon:          {key: core.EdgePairConditionSessionStartLogon},
		sessionStartFirstOperation: {key: core.EdgePairConditionSessionStartFirstOperation},
		sessionStartLogoffRecord:   {key: core.EdgePairConditionSessionStartLogoffRecord},
	}
	sessionEndConditions = [sessionEndKindCount]pairCondition{
		sessionEndLogoff:        {key: core.EdgePairConditionSessionEndLogoff},
		sessionEndSystemStart:   {key: core.EdgePairConditionSessionEndSystemStart},
		sessionEndTimeLimit:     {key: core.EdgePairConditionSessionEndTimeLimit},
		sessionEndLastOperation: {key: core.EdgePairConditionSessionEndLastOperation},
	}
	pairSourceTerminal = pairCondition{core.EdgePairConditionSourceTerminal,
		[]core.SemanticKey{placedTerminal}, []core.SemanticKey{core.SemanticKeyConnectionSourceAddress}}
	// sessionLogonConditions は、セッションを始めたログオンの種別の区分ごとの条件である。
	sessionLogonConditions = [sessionLogonClassCount]core.EdgePairConditionKey{
		sessionLogonInteractive: core.EdgePairConditionSessionLogonInteractive,
		sessionLogonOther:       core.EdgePairConditionSessionLogonOther,
		sessionLogonNetwork:     core.EdgePairConditionSessionLogonNetwork,
	}
	// logonAccountItems は、ログオンが作ったセッションのアカウントを持つ語彙の項目である。種別 9 の
	// ログオンが他の端末への接続に使うアカウントを含む。
	logonAccountItems = []core.SemanticKey{
		core.SemanticKeyTargetAccountSid, core.SemanticKeyTargetAccountDomain, core.SemanticKeyTargetAccountName,
		core.SemanticKeyEventOutboundAccountDomain, core.SemanticKeyEventOutboundAccountName,
	}
	// operationAccountItems は、操作を行ったアカウントを持つ語彙の項目である。
	operationAccountItems = []core.SemanticKey{
		core.SemanticKeySubjectAccountSid, core.SemanticKeySubjectAccountDomain, core.SemanticKeySubjectAccountName,
	}
	// chainedLogonAccountItems は、ログオンの連鎖の終点のログオンのアカウントを持つ語彙の項目である。
	chainedLogonAccountItems = []core.SemanticKey{
		core.SemanticKeyTargetAccountSid, core.SemanticKeyTargetAccountDomain, core.SemanticKeyTargetAccountName,
	}
)

// sessionAccountItems は、始まりの種類が start のセッションのアカウントを持つ語彙の項目である。
func sessionAccountItems(start sessionStartKind) []core.SemanticKey {
	if start == sessionStartFirstOperation {
		return operationAccountItems
	}
	return logonAccountItems
}

// conditions は規則の成立の条件を返す。ログオンの連鎖の規則は、接続元の端末、始まり、終わりの順で
// あり、始まりの記録と終点の組の規則は、続けてアカウントとログオンの種別の区分を持つ。
func (r pairRule) conditions() []pairCondition {
	if r < pairRuleLogonChain {
		return pairRules[r]
	}
	offset := r - pairRuleLogonChain
	if offset < logonChainSessionRules {
		return []pairCondition{pairSourceTerminal, sessionStartConditions[offset/pairRule(sessionEndKindCount)],
			sessionEndConditions[offset%pairRule(sessionEndKindCount)]}
	}
	offset -= logonChainSessionRules
	class := sessionLogonClass(offset % pairRule(sessionLogonClassCount))
	offset /= pairRule(sessionLogonClassCount)
	session := offset / 2
	start := sessionStartKind(session / pairRule(sessionEndKindCount))
	account := pairCondition{core.EdgePairConditionSessionAccountDifferent, sessionAccountItems(start),
		chainedLogonAccountItems}
	if offset%2 == 1 {
		account.key = core.EdgePairConditionSessionAccountMatch
	}
	logonType := []core.SemanticKey{core.SemanticKeyEventLogonType}
	return append((pairRuleLogonChain + session).conditions(), account,
		pairCondition{sessionLogonConditions[class], logonType, logonType})
}

// pairCondition は、規則の条件 1 つと、条件が両側のレコードから読む語彙の項目である。
// 時刻の条件は項目を持たない。比べた時刻は組の両側の根拠が持つ。
//
// placedTerminal の語彙の項目は、欄の代わりに、レコードを置いた端末を読む印である。規則が
// 比べた端末は、欄を持たないレコードでは収集元の端末であり、割当がまとめた端末である
// (sourceTerminals.forRecord)。
type pairCondition struct {
	key         core.EdgePairConditionKey
	left, right []core.SemanticKey
}

// placedTerminal は、レコードを置いた端末を読む印である。語彙に無い文字列であり、欄の語彙の
// 項目と一致しない。
const placedTerminal core.SemanticKey = "\x00placed_terminal"

// placedTerminalFieldName は、レコードを置いた端末を持つ欄の名前である。
const placedTerminalFieldName = "placedTerminal"

var (
	pairTerminal = pairCondition{core.EdgePairConditionTerminal,
		[]core.SemanticKey{placedTerminal}, []core.SemanticKey{placedTerminal}}
	pairTimeOrder     = pairCondition{key: core.EdgePairConditionTimeOrder}
	pairTimeProximity = pairCondition{key: core.EdgePairConditionTimeProximity}
	pairAccountItems  = []core.SemanticKey{
		core.SemanticKeyTargetAccountDomain, core.SemanticKeyTargetAccountName, core.SemanticKeyTargetAccountSid,
		core.SemanticKeySubjectAccountDomain, core.SemanticKeySubjectAccountName, core.SemanticKeySubjectAccountSid,
	}
	pairSourceEndpoint = []core.SemanticKey{
		core.SemanticKeyConnectionSourceAddress, core.SemanticKeyConnectionSourcePort,
	}
	pairDestinationTerminal = pairCondition{core.EdgePairConditionDestinationTerminal,
		[]core.SemanticKey{core.SemanticKeyConnectionDestinationAddress}, pairTerminal.right}
	pairDestinationEndpointItems = []core.SemanticKey{
		core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyConnectionDestinationPort,
	}
	pairDestinationEndpoint = pairCondition{core.EdgePairConditionDestinationEndpoint,
		pairDestinationEndpointItems, pairDestinationEndpointItems}
)

// pairRules は、規則ごとの成立の条件である。並びは画面に出す順である。
var pairRules = [...][]pairCondition{
	pairRuleProcessParent: {
		{core.EdgePairConditionProcessPid,
			[]core.SemanticKey{core.SemanticKeyProcessPid}, []core.SemanticKey{core.SemanticKeyParentProcessPid}},
		pairTerminal, pairTimeOrder,
	},
	pairRuleProcessIdentity: {
		{core.EdgePairConditionProcessPid,
			[]core.SemanticKey{core.SemanticKeyProcessPid}, []core.SemanticKey{core.SemanticKeyProcessPid}},
		pairTerminal, {key: core.EdgePairConditionTimeOverlap},
	},
	pairRuleLogonSession: {
		{core.EdgePairConditionLogonId,
			[]core.SemanticKey{core.SemanticKeyEventTargetLogonId}, []core.SemanticKey{core.SemanticKeyEventSubjectLogonId}},
		pairTerminal, pairTimeOrder,
	},
	pairRuleLinkedLogon: {
		{core.EdgePairConditionLinkedLogonId,
			[]core.SemanticKey{core.SemanticKeyEventTargetLogonId, core.SemanticKeyEventTargetLinkedLogonId},
			[]core.SemanticKey{core.SemanticKeyEventTargetLinkedLogonId, core.SemanticKeyEventTargetLogonId}},
		pairTerminal,
	},
	pairRuleTaskRun: {
		{core.EdgePairConditionTaskName,
			[]core.SemanticKey{core.SemanticKeyScheduledTaskName}, []core.SemanticKey{core.SemanticKeyStartedTaskName}},
		pairTerminal, pairTimeOrder,
	},
	pairRuleTicketLogon: {
		{core.EdgePairConditionLogonGuid,
			[]core.SemanticKey{core.SemanticKeyEventTicketLogonGuid}, []core.SemanticKey{core.SemanticKeyEventTargetLogonGuid}},
	},
	pairRuleConnectionLogon: {
		{core.EdgePairConditionSourceEndpoint, pairSourceEndpoint, pairSourceEndpoint},
		pairDestinationTerminal, pairTimeProximity,
	},
	pairRuleExplicitCredential: {
		{core.EdgePairConditionAccount,
			[]core.SemanticKey{core.SemanticKeyTargetAccountName}, []core.SemanticKey{core.SemanticKeyTargetAccountName}},
		// 要求は接続先を、アドレスか端末の名前 (割当が記録した名前と比べる) で書く。
		{core.EdgePairConditionDestinationTerminal, []core.SemanticKey{
			core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyConnectionDestinationServerName,
		}, pairTerminal.right},
		pairTimeProximity,
	},
	pairRuleAccountIdentity: {
		// SID のレコードは名前のレコードの直前のことも直後のこともあり、時刻の順を条件にしない。
		{core.EdgePairConditionAccount, pairAccountItems, pairAccountItems},
		{key: core.EdgePairConditionNearestIdentityRecord},
	},
	pairRuleUnidentifiedSource: {
		{core.EdgePairConditionSourceUnassigned,
			nil, []core.SemanticKey{core.SemanticKeyConnectionSourceAddress}},
	},
	pairRuleSourceOutsideAssignmentRange: {
		{core.EdgePairConditionSourceOutsideAssignmentRange,
			nil, []core.SemanticKey{core.SemanticKeyConnectionSourceAddress}},
	},
	pairRuleSameConnectionOnTerminal: {
		{core.EdgePairConditionSourceEndpoint, pairSourceEndpoint, pairSourceEndpoint},
		pairDestinationEndpoint, pairTerminal, pairTimeOrder,
	},
	pairRuleSameConnectionUnphased: {
		{core.EdgePairConditionSourceEndpoint, pairSourceEndpoint, pairSourceEndpoint},
		pairDestinationEndpoint, pairTerminal, pairTimeProximity,
	},
	pairRuleSameConnectionAcrossTerminals: {
		{core.EdgePairConditionSourceEndpoint, pairSourceEndpoint, pairSourceEndpoint},
		pairDestinationEndpoint, pairDestinationTerminal, pairTimeProximity,
	},
	pairRuleRequestedSession: {
		// 種別 9 のログオンが始めたセッションは、他の端末への接続に使うアカウントを持つ。
		{core.EdgePairConditionRequestingSession, []core.SemanticKey{
			core.SemanticKeyEventTargetLogonId, core.SemanticKeyEventSubjectLogonId,
			core.SemanticKeyEventOutboundAccountDomain, core.SemanticKeyEventOutboundAccountName,
		}, nil},
	},
}

// noPairRecord は、組の片側がレコードを持たないことを表す位置である。
const noPairRecord = -1

// recordPair は、観測の層が候補の関係を作ったレコードの組 1 つである。**値を複製しない。**
// 両側を g.records の位置で指し、応答を組むときにレコードのノードの属性から値を読む。
type recordPair struct {
	left, right int32
	rule        pairRule
}

// addRecordPair は、候補のエッジへ成立に用いたレコードの組を 1 つ足す。直前に足した組と同じ
// 組は足さない。int32 に収まらない位置の組は足さない。
func (g *Graph) addRecordPair(edgeAt int, rule pairRule, left, right int) {
	leftAt, leftFits := compactPairIndex(left)
	rightAt, rightFits := compactPairIndex(right)
	if !leftFits || !rightFits {
		return
	}
	pair := recordPair{left: leftAt, right: rightAt, rule: rule}
	basis := g.edges[edgeAt].ensureBasis()
	if count := len(basis.pairs); count > 0 && basis.pairs[count-1] == pair {
		return
	}
	basis.pairs = append(basis.pairs, pair)
}

// compactPairIndex は組の片側の位置を int32 へ直す。noPairRecord はそのまま通す。
func compactPairIndex(at int) (int32, bool) {
	if at == noPairRecord {
		return noPairRecord, true
	}
	return compactIndex(at)
}

// pairList はエッジのレコードの組を返す。成立の根拠を持たないエッジでは nil を返す。
func (e graphEdge) pairList() []recordPair {
	if e.basis == nil {
		return nil
	}
	return e.basis.pairs
}

// maxResponsePairs は、`/api/v0/edges/{id}` の応答に載せるレコードの組の上限である。
//
// 既知の制限: 組の先頭から上限までを返し、総数を別に返す, 同じアカウントの候補は、そのアカウントを
// 指すレコードの数だけ組を持ち、よく使うアカウントでは数万になる。組の両側のレコードは
// 根拠のレコードの表が全件を持つ, 分析者が上限より後ろの組を読む操作が出たとき、続きを取る
// 要求を足す
const maxResponsePairs = 1000

// recordPairsOf は、エッジのレコードの組を応答の形にして、先頭から maxResponsePairs 組までを返す。
func (g Graph) recordPairsOf(edge graphEdge) []core.EdgeRecordPair {
	held := edge.pairList()
	pairs := make([]core.EdgeRecordPair, 0, min(len(held), maxResponsePairs))
	for _, pair := range held[:min(len(held), maxResponsePairs)] {
		item := core.EdgeRecordPair{Left: g.pairEvidence(pair.left), Right: g.pairEvidence(pair.right)}
		for _, condition := range pair.rule.conditions() {
			item.Conditions = append(item.Conditions, g.pairConditionOf(pair, condition))
		}
		if pair.rule >= pairRuleLogonChain {
			item.CandidateTally = candidateTallyOf(edge.sessionList(), pair)
		}
		pairs = append(pairs, item)
	}
	return pairs
}

// candidateTallyOf は、ログオンの連鎖の組 pair の起点のセッションが、同じ終点に挙がった候補の中で
// 持つ件数を返す。組に該当するセッションが無いときは nil である。
func candidateTallyOf(sessions []sessionSpan, pair recordPair) *core.EdgeCandidateTally {
	for _, session := range sessions {
		if session.target == int(pair.right) && session.candidates > 0 &&
			(session.startAt == int(pair.left) || session.endAt == int(pair.left)) {
			return &core.EdgeCandidateTally{
				CandidateCount: int(session.candidates), PrecedingCandidateCount: int(session.preceding),
			}
		}
	}
	return nil
}

// pairConditionOf は、組 1 つの条件 1 つを応答の形にする。許容幅を持つ条件には、幅と、両側の
// 時刻を時点として読めるときの差を添える。
func (g Graph) pairConditionOf(pair recordPair, condition pairCondition) core.EdgePairCondition {
	item := core.EdgePairCondition{
		ConditionKey: condition.key,
		LeftValue:    g.recordFieldsWithSemantics(pair.left, condition.left),
		RightValue:   g.recordFieldsWithSemantics(pair.right, condition.right),
	}
	tolerance := pair.rule.proximityWindow()
	if condition.key != core.EdgePairConditionTimeProximity || tolerance == 0 {
		return item
	}
	window := int64(tolerance / time.Second)
	item.WindowSeconds = &window
	if pair.left == noPairRecord || pair.right == noPairRecord ||
		!g.records[pair.left].hasInstant || !g.records[pair.right].hasInstant {
		return item
	}
	left, right := g.records[pair.left].instant, g.records[pair.right].instant
	// 照合と同じ比べ方の差を出す。接続の組は秒未満を切り捨てた時刻で比べる。
	difference := right.Sub(left).Seconds()
	if pair.rule.comparesWholeSeconds() {
		difference = float64(right.Unix() - left.Unix())
		item.DifferenceInWholeSeconds = true
	}
	item.DifferenceSeconds = &difference
	return item
}

// comparesWholeSeconds は、規則が両側の時刻の秒未満を切り捨てて比べるかを返す
// (graph_connection_logon.go と graph_same_connection.go の照合)。
func (r pairRule) comparesWholeSeconds() bool {
	return r == pairRuleConnectionLogon || r == pairRuleSameConnectionUnphased ||
		r == pairRuleSameConnectionAcrossTerminals
}

// proximityWindow は、規則の時刻の差の許容幅である。時刻の差を条件にしない規則では 0 である。
func (r pairRule) proximityWindow() time.Duration {
	switch r {
	case pairRuleExplicitCredential:
		return explicitCredentialTolerance
	case pairRuleConnectionLogon:
		return connectionLogonToleranceSeconds * time.Second
	case pairRuleSameConnectionUnphased, pairRuleSameConnectionAcrossTerminals:
		return sameConnectionToleranceSeconds * time.Second
	default:
		return 0
	}
}

// pairEvidence は組の片側のレコードを根拠の形で返す。レコードを持たない側では nil を返す。
func (g Graph) pairEvidence(at int32) *core.GraphEvidence {
	if at == noPairRecord {
		return nil
	}
	return &g.evidenceItems([]int{int(at)})[0]
}

// recordFieldsWithSemantics は、レコードの欄のうち語彙の項目が semantics に入るものを、
// semantics の並びで複製して返す。
//
// **レコードのノードの属性から読む。** 属性は読めた欄をすべて持ち、取り込み結果の項目を指す
// (addRecordNode)。レコードのノードを持たないレコードと、その側がレコードを持たない組では
// 要素数 0 を返す。
func (g Graph) recordFieldsWithSemantics(at int32, semantics []core.SemanticKey) []core.RecordField {
	fields := []core.RecordField{}
	if at == noPairRecord {
		return fields
	}
	for _, semantic := range semantics {
		if semantic == placedTerminal {
			if field, placed := g.placedTerminalField(int(at)); placed {
				fields = append(fields, field)
			}
			continue
		}
		if !g.records[at].hasRecordNode {
			continue
		}
		for _, attribute := range g.nodes[g.records[at].recordNode].attributes {
			if attribute.field.Semantic == semantic {
				fields = append(fields, cloneRecordField(*attribute.field))
			}
		}
	}
	return fields
}

// placedTerminalField は、レコード at を置いた端末の表示名を欄の形で返す。ok が偽になるのは、
// 端末に置かないレコードと、端末のノードが表示名を持たないときである。
func (g Graph) placedTerminalField(at int) (core.RecordField, bool) {
	index, present := g.nodeAt[g.records[at].placedTerminalNodeId]
	if !present || g.records[at].placedTerminalNodeId == "" {
		return core.RecordField{}, false
	}
	field, err := core.NewTextField(placedTerminalFieldName, "", cloneRawAndNormalized(g.nodes[index].label))
	return field, err == nil
}
