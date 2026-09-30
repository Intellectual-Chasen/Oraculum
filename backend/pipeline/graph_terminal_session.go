package pipeline

import (
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// transcriptKey は 1 つの事象の転記を指す鍵である。
// 同じ事象を写した 2 件のレコードを、独立した根拠として数えないために使う。
type transcriptKey struct {
	terminalId string
	// items は、収集元が宣言した欄の値を宣言の並びで連ねた文字列である
	// (ParserIdentity.TranscriptIdentityItems)。
	items string
}

// clientTerminalSourceFieldName は、外部識別子も表示名も持たない割当が導いた端末を、割当を
// 付けた収集元の file 名で渡す根拠の条件の欄の名前である。語彙の項目を名乗らない。
const clientTerminalSourceFieldName = "clientTerminalSource"

// terminalSessionScan は、案件 1 つの遠隔のセッションの候補を足す走査の状態である。
type terminalSessionScan struct {
	// result は案件 1 つの取り込み結果である。割当を付けた収集元の file 名を探す。
	result ImportResult
	// assignments は、期間の両端を時点として読める割当である
	// (terminalAssignments.comparableEntries)。
	assignments []core.TerminalAssignment
	terminals   sourceTerminals
	// seen はエッジごとに数えた転記である (countsAsNewEvidence)。
	seen map[int]map[transcriptKey]struct{}
	// assignedTerminals は、割当だけが指す端末のノードの位置である (sessionSourceNode)。
	// **案件をまたいで 1 つを持つ。** 同じ端末を別の案件のレコードも参照する。
	assignedTerminals map[int]struct{}
}

// recordsLogon は、レコードがログオンを記録しているかを返す。ログオンの種別か、遠隔の
// セッションの接続元が名乗った名前の項目を持つレコードと、binding が遠隔のセッションの
// 記録と決めたレコード (RecordSemantics.RemoteSession) である。共有の名前を持つレコードも
// 含める。共有へのアクセスは、接続元が端末へ入ったセッションの中の操作である。
//
// **自身の terminal.id を持たないレコードは、ログオンを記録したものだけが候補を作る。**
// 自分の端末から始めた通信の記録と、HTTP の要求の記録も接続元のアドレスを持つが、端末へ
// 入ったセッションを記録していない。
func recordsLogon(record RecordEntry, fields []core.RecordField) bool {
	return (record.Semantics != nil && record.Semantics.RemoteSession) ||
		len(fieldsWithSemantic(fields, core.SemanticKeyEventLogonType)) > 0 ||
		len(fieldsWithSemantic(fields, core.SemanticKeyRemoteSessionClientHostname)) > 0 ||
		len(fieldsWithSemantic(fields, core.SemanticKeyShareName)) > 0
}

// addTerminalSessionEdges は、接続元のアドレスを記録したレコードから、そのアドレスを
// 保持していた端末と、レコードを記録した端末を結ぶ候補のエッジを足す。
//
// **アドレスの一致だけで端末を決めない。** 端末を導くのは IP から端末への割当であり、
// レコードの時刻が割当を適用してよい期間の中にあることを確かめる。期間の中に 2 つ以上の
// 端末があるレコードは、端末ごとに候補のエッジを持つ。
//
// **レコードを記録した端末は、レコードを置く端末である** (core.RecordTerminalNodeKey)。
// terminal.id を持たないレコードは収集元の端末に置かれ、名前不明の端末も終点になる。
// そのレコードはログオンを記録したものに限る (recordsLogon)。
//
// 接続元の端末とレコードを記録した端末が同じ組は関係にしない。プロセスが自分の端末の
// アドレスから外へ出した接続のレコードが、その形になる。
//
// assignedTerminals は、割当だけが指す端末のノードの位置である。呼び出し元がグラフ
// 1 つにつき 1 つを渡す。
func (g *Graph) addTerminalSessionEdges(
	result ImportResult, terminals sourceTerminals, assignedTerminals map[int]struct{},
) {
	scan := terminalSessionScan{
		result:            result,
		assignments:       terminalAssignmentsOf(result).comparableEntries(),
		terminals:         terminals,
		seen:              make(map[int]map[transcriptKey]struct{}),
		assignedTerminals: assignedTerminals,
	}
	type ticketRequest struct {
		record          RecordEntry
		transcriptItems []string
	}
	var ticketRequests []ticketRequest
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		transcriptItems := publication.TranscriptIdentityItems()
		for _, record := range publication.records {
			if recordsTicketRequest(record) {
				ticketRequests = append(ticketRequests, ticketRequest{record, transcriptItems})
				continue
			}
			g.addTerminalSessionEdgesOfRecord(scan, record, transcriptItems, false)
		}
	}
	// **チケットの要求は、ログオンのレコードが作った候補にだけ根拠を足す。** 全ログオンの
	// 後に走査し、取り込みの並びに依らない。
	for _, request := range ticketRequests {
		g.addTerminalSessionEdgesOfRecord(scan, request.record, request.transcriptItems, true)
	}
}

// recordsTicketRequest は、レコードが Kerberos のチケットの要求を記録したかを返す。要求を
// 記録したレコードだけが event.ticket_logon_guid の欄を持つ。
func recordsTicketRequest(record RecordEntry) bool {
	return record.Semantics != nil &&
		len(fieldsWithSemantic(record.Semantics.Fields, core.SemanticKeyEventTicketLogonGuid)) > 0
}

// addTerminalSessionEdgesOfRecord はレコード 1 件が作る候補のエッジを足す。joinOnly が真の
// レコードは、既にある候補のエッジに根拠を足すだけで、エッジを作らない。
func (g *Graph) addTerminalSessionEdgesOfRecord(
	scan terminalSessionScan, record RecordEntry, transcriptItems []string, joinOnly bool,
) {
	// **マルチキャストとブロードキャストの着信は、どの関係にもしない。** 相手の端末は宛先を 1 台に
	// 決めずに送っており、自分の端末への接続を記録していない。
	if record.ObservedAt == nil || (record.Semantics != nil && record.Semantics.InboundGroupAddress) {
		return
	}
	ownTerminal := len(fieldsWithSemantic(graphFieldsOf(record), core.SemanticKeyTerminalId)) > 0
	supplied, scope := scan.terminals.forRecord(record)
	if len(supplied) > 0 {
		record.Terminal = append(slices.Clone(record.Terminal), supplied...)
	}
	fields := graphFieldsOf(record)
	clientIp, hasClientIp := comparableOfSemantic(
		fields, core.SemanticKeyConnectionSourceAddress)
	// **着信の接続の許可は、遠隔のセッションに合流させない。** 割当で決まる接続元の端末からの
	// 着信の接続の候補だけを作る。
	inbound := !joinOnly && record.Semantics != nil && record.Semantics.InboundConnection
	if !hasClientIp || (!ownTerminal && !joinOnly && !inbound && !recordsLogon(record, fields)) {
		return
	}
	sessionKind := core.EdgeKindTerminalRemoteSession
	if inbound {
		sessionKind = core.EdgeKindInboundConnectionMatch
	}
	target, built := core.RecordTerminalNodeKey(fields, scope)
	if !built {
		return
	}
	targetIndex, targetPresent := g.nodeAt[nodeIdOf(target)]
	if !targetPresent {
		return
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: clientIp, EventTime: *record.ObservedAt, Assignments: scan.assignments,
	})
	if err != nil {
		return
	}
	at, indexed := g.recordAtLocator(record.Locator)
	if !indexed {
		return
	}
	if len(resolution.Members) == 0 {
		if !inbound && sourceUnidentified(resolution) {
			rule := pairRuleUnidentifiedSource
			if resolution.AssignmentState == core.TerminalAssignmentStateOriginOutsideAssignmentRange {
				rule = pairRuleSourceOutsideAssignmentRange
			}
			g.addUnidentifiedSourceSession(scan, clientIp, targetIndex, at, fields, transcriptItems, joinOnly, rule)
		}
		return
	}
	for _, member := range resolution.Members {
		// **記録した収集元に付けた割当の端末は、その収集元を記録した端末そのものである。**
		// 同じ収集元に別の端末の割当が 2 件以上あり、収集元が名前不明の端末に置かれるときも、
		// 自分の端末からの関係にしない。
		if member.AppliesToSourceId != "" && member.AppliesToSourceId == record.Locator.SourceId {
			continue
		}
		// **根拠を足すだけのレコードは、割当の端末のノードを作らない。**
		if joinOnly {
			key, built := member.TerminalNodeKey()
			if !built {
				continue
			}
			source, present := g.nodeAt[nodeIdOf(key)]
			if !present {
				continue
			}
			if _, joined := g.edgeIndexOf(edgeIdOf(core.EdgeKindTerminalRemoteSession,
				g.nodes[source].id, g.nodes[targetIndex].id)); !joined {
				continue
			}
		}
		sourceIndex, found := g.sessionSourceNode(scan, member, at)
		if !found || sourceIndex == targetIndex {
			continue
		}
		basis, composed := terminalSessionBasis(fields, clientIp, member, scan.appliedFileName(member))
		if !composed {
			continue
		}
		edge, present := g.sessionEdge(sessionKind, sourceIndex, targetIndex, joinOnly)
		if !present {
			continue
		}
		g.addAssignmentBasis(edge, basis)
		if countsAsNewEvidence(scan.seen, edge, g.nodes[targetIndex].id, fields, transcriptItems) {
			g.addEdgeEvidence(edge, at)
		}
	}
}

// sessionEdge は、source から target への kind の候補のエッジの位置を返す。joinOnly が真の
// ときはエッジを作らず、無ければ ok が偽である。
func (g *Graph) sessionEdge(kind core.EdgeKind, source, target int, joinOnly bool) (int, bool) {
	if joinOnly {
		return g.edgeIndexOf(edgeIdOf(kind, g.nodes[source].id, g.nodes[target].id))
	}
	return g.ensureEdge(kind, core.RelationStateCandidate, source, target), true
}

// sourceUnidentified は、割当が接続元の端末を導かなかった理由が、そのアドレスの割当が無いか、
// 割当の期間の外であるかを返す。時刻を比べられなかったときは偽である。
func sourceUnidentified(resolution core.TerminalResolution) bool {
	return resolution.AssignmentState == core.TerminalAssignmentStateOriginOutsideAssignmentRange ||
		slices.Contains(resolution.UnresolvedReasons, core.TerminalUnresolvedReasonNoAssignmentForClientIp)
}

// addUnidentifiedSourceSession は、接続元のアドレスのノードから、レコードを記録した端末への
// 接続元が未同定の遠隔のセッションの候補に、レコードを根拠として足す。rule は未同定の理由
// (割当が無い、割当の期間の外) の規則であり、組の条件に出る。
//
// **ループバックとリンクローカルのアドレスと、記録した端末が持つアドレスからは作らない。**
// 前者は端末の中の接続であり、後者は記録した端末自身からの接続である。
func (g *Graph) addUnidentifiedSourceSession(
	scan terminalSessionScan, clientIp string, targetIndex, at int,
	fields []core.RecordField, transcriptItems []string, joinOnly bool, rule pairRule,
) {
	address, err := netip.ParseAddr(clientIp)
	if err != nil || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
		return
	}
	sourceIndex, present := g.nodeAt[nodeIdOf(core.NodeKey{
		Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: clientIp}},
	})]
	if !present {
		return
	}
	if _, owned := g.edgeIndexOf(edgeIdOf(core.EdgeKindTerminalAddress,
		g.nodes[targetIndex].id, g.nodes[sourceIndex].id)); owned {
		return
	}
	edge, formed := g.sessionEdge(core.EdgeKindUnidentifiedSourceRemoteSession, sourceIndex, targetIndex, joinOnly)
	if formed && countsAsNewEvidence(scan.seen, edge, g.nodes[targetIndex].id, fields, transcriptItems) {
		g.addEdgeEvidence(edge, at)
		g.addRecordPair(edge, rule, noPairRecord, at)
	}
}

// appliedFileName は、割当を付けた収集元の file 名を返す。収集元の識別を探せないときは
// sourceId を返し、割当が収集元に付いていないときは空の文字列を返す。
func (s terminalSessionScan) appliedFileName(member core.TerminalAssignment) string {
	if identity, found := s.result.Identity(member.AppliesToSourceId); found && identity.FileName != "" {
		return identity.FileName
	}
	return member.AppliesToSourceId
}

// sessionSourceNode は、割当 1 件が導いた端末のノードの位置を返す。at は割当を探した
// レコードの位置である。ok が偽になるのは、端末のノードが無く、足さないときである。
//
// **利用者が端末の外部識別子で与えた端末は、グラフに無ければ参照のノードとして足す。**
// 分析者は、ログを持たない端末にも IP を割り当てられる。足したノードの根拠は、割当を通して
// その端末を参照したレコードであり、表示名は割当が与えた表示名である。
//
// 収集元のレコードから読んだ割当の端末と、外部識別子を持たない割当の端末 (収集元を記録した
// 端末) は、収集元のレコードが作ったノードだけを使う。
func (g *Graph) sessionSourceNode(
	scan terminalSessionScan, member core.TerminalAssignment, at int,
) (int, bool) {
	key, built := member.TerminalNodeKey()
	if !built {
		return 0, false
	}
	index, present := g.nodeAt[nodeIdOf(key)]
	if !present {
		if member.Origin == core.TerminalAssignmentOriginObservedInSource || member.TerminalId == "" {
			return 0, false
		}
		index = g.ensureNode(key, core.NodeObservationReferenced)
		scan.assignedTerminals[index] = struct{}{}
		if member.TerminalHostname != "" {
			label, err := core.NewDerivedValue(member.TerminalHostname, assignmentFieldDerivation(member))
			if err == nil {
				g.nodes[index].applyLabel(label)
			}
		}
	}
	// レコードは走査の順に来るため、同じレコードは末尾にだけ重なる。
	if _, assigned := scan.assignedTerminals[index]; assigned {
		evidence := g.nodes[index].evidence
		if len(evidence) == 0 || evidence[len(evidence)-1] != at {
			g.nodes[index].evidence = append(evidence, at)
		}
	}
	return index, true
}

// addAssignmentBasis は成立の根拠を 1 件足す。**同じ根拠を 2 回持たない。**
// 用いたアドレスと割当の期間と導いた端末が同じ根拠は、同じ 1 件である。
func (g *Graph) addAssignmentBasis(edge int, basis core.EdgeAssignmentBasis) {
	for _, existing := range g.edges[edge].assignmentBasisList() {
		if sameAssignmentBasis(existing, basis) {
			return
		}
	}
	held := g.edges[edge].ensureBasis()
	held.assignmentBases = append(held.assignmentBases, basis)
}

// sameAssignmentBasis は 2 つの根拠が同じ割当を指すかを返す。
func sameAssignmentBasis(left, right core.EdgeAssignmentBasis) bool {
	if left.ClientIp != right.ClientIp || len(left.Conditions) != len(right.Conditions) {
		return false
	}
	for index, condition := range left.Conditions {
		counterpart := right.Conditions[index]
		if condition.ConditionKey != counterpart.ConditionKey {
			return false
		}
		if !sameAssignmentRange(condition.AssignmentValidRange,
			counterpart.AssignmentValidRange) {
			return false
		}
		if !sameTerminalValue(condition.RightValue, counterpart.RightValue) {
			return false
		}
	}
	return true
}

// sameAssignmentRange は 2 つの割当の期間が同じ文字列であるかを返す。
func sameAssignmentRange(left, right *core.TimeRange) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftFrom, leftFromReadable := left.From.NormalizedValue()
	rightFrom, rightFromReadable := right.From.NormalizedValue()
	leftTo, leftToReadable := left.To.NormalizedValue()
	rightTo, rightToReadable := right.To.NormalizedValue()
	if leftFromReadable != rightFromReadable || leftToReadable != rightToReadable {
		return false
	}
	return leftFrom == rightFrom && leftTo == rightTo
}

// sameTerminalValue は 2 つの条件が同じ端末を導いたかを返す。
func sameTerminalValue(left, right []core.RecordField) bool {
	if len(left) != len(right) {
		return false
	}
	for index, field := range left {
		if field.Text == nil || right[index].Text == nil {
			return false
		}
		leftValue, leftReadable := field.Text.ComparableValue()
		rightValue, rightReadable := right[index].Text.ComparableValue()
		if leftReadable != rightReadable || leftValue != rightValue {
			return false
		}
	}
	return true
}

// countsAsNewEvidence は、そのレコードをエッジの根拠として数えるかを返す。
//
// **同じ事象を写した 2 件目のレコードを数えない。** 端末と、収集元が転記の同一性として
// 宣言した欄の値がすべて同じ 2 件は、収集元が同じ 1 つの事象から作った転記であり、独立した
// 観測ではない (ParserIdentity.TranscriptIdentityItems)。
//
// **どの欄が転記を指すかを本 package が決めない。** 宣言を持たない収集元のレコードと、
// 宣言した欄のいずれかを持たないレコードは、毎回数える。
func countsAsNewEvidence(
	seen map[int]map[transcriptKey]struct{}, edge int, terminalId string,
	fields []core.RecordField, transcriptItems []string,
) bool {
	items, identified := transcriptIdentityOf(fields, transcriptItems)
	if !identified {
		return true
	}
	key := transcriptKey{terminalId: terminalId, items: items}
	if seen[edge] == nil {
		seen[edge] = make(map[transcriptKey]struct{})
	}
	if _, taken := seen[edge][key]; taken {
		return false
	}
	seen[edge][key] = struct{}{}
	return true
}

// transcriptIdentityOf は、収集元が宣言した欄の値を 1 つの文字列へ連ねる。
// ok が偽になるのは、宣言が欄を 1 つも挙げないときと、挙げた欄の値を比べられないときである。
//
// **値の前に byte 数を置く。** 区切りの文字列だけで連ねると、`a` と `b|c` を持つ組と
// `a|b` と `c` を持つ組が同じ文字列になり、別の事象を 1 つの転記としてまとめる。
func transcriptIdentityOf(fields []core.RecordField, items []string) (string, bool) {
	if len(items) == 0 {
		return "", false
	}
	var joined strings.Builder
	for _, name := range items {
		value, comparable := comparableOfName(fields, name)
		if !comparable {
			return "", false
		}
		joined.WriteString(strconv.Itoa(len(value)))
		joined.WriteString(":")
		joined.WriteString(value)
	}
	return joined.String(), true
}

// comparedClientIpDerivation は、IP から端末への割当と比べた接続元のアドレスの導き方である。
const comparedClientIpDerivation = "原資料の文字列を IP から端末への割当のアドレスと文字列の一致で比べた値"

// terminalSessionBasis は候補のエッジ 1 本の成立の根拠を組む。appliedFileName は割当を
// 付けた収集元の file 名である。ok が偽になるのは、条件の値を組めないときである。
//
// **導いた端末は、割当が持つ値だけで表す。** 外部識別子を terminal.id で、外部識別子が無い
// ときは表示名を terminal.hostname で渡す (FieldsBuilder の terminalValueOf と同じ項目)。
// どちらも持たない割当の端末は、割当を付けた収集元を記録した端末であり、その収集元の file
// 名を語彙の項目を名乗らない欄で渡す。ノードの表示名は割当から導いた値ではないため使わない。
func terminalSessionBasis(
	fields []core.RecordField, clientIp string, member core.TerminalAssignment, appliedFileName string,
) (core.EdgeAssignmentBasis, bool) {
	left := fieldsWithSemantic(fields, core.SemanticKeyConnectionSourceAddress)
	if len(left) == 0 {
		return core.EdgeAssignmentBasis{}, false
	}
	name, semantic, terminal := clientTerminalFieldName, core.SemanticKeyTerminalId, member.TerminalId
	switch {
	case terminal != "":
	case member.TerminalHostname != "":
		name, semantic, terminal = clientTerminalNameFieldName, core.SemanticKeyTerminalHostname,
			member.TerminalHostname
	case appliedFileName != "":
		name, semantic, terminal = clientTerminalSourceFieldName, "", appliedFileName
	default:
		return core.EdgeAssignmentBasis{}, false
	}
	matchKeyName, _ := clientAddress(fields)
	derivation := derivedTerminalDerivation(derivationItems{
		matchKey:    matchKey{name: matchKeyName, comparable: clientIp},
		assignments: []core.TerminalAssignment{member},
	})
	value, err := core.NewDerivedValue(terminal, derivation)
	if err != nil {
		return core.EdgeAssignmentBasis{}, false
	}
	right, err := core.NewTextField(name, semantic, value)
	if err != nil {
		return core.EdgeAssignmentBasis{}, false
	}
	// **割当と比べた文字列を、比較に用いる値として渡す。** 原資料の欄が正規化値を持たないときは、
	// 原資料の文字列をそのまま割当のアドレスと比べている。
	compared := cloneRecordField(left[0])
	if _, normalized := compared.Text.NormalizedValue(); !normalized {
		if raw, present := compared.Text.RawTextValue(); present && raw == clientIp {
			value := normalizedText(raw, clientIp, comparedClientIpDerivation)
			compared.Text = &value
		}
	}
	outside := false
	basis := core.EdgeAssignmentBasis{
		ClientIp: clientIp,
		SourceId: member.SourceId,
		Conditions: []core.MatchCondition{{
			ConditionKey: core.ConditionKeyTerminalIpAssignment,
			Use:          core.ConditionUseUsed,
			LeftValue:    []core.RecordField{compared},
			RightValue:   []core.RecordField{right},
			AssignmentValidRange: &core.TimeRange{
				From: cloneTimestamp(member.AssignmentValidRange.From),
				To:   cloneTimestamp(member.AssignmentValidRange.To),
			},
			OutsideAssignmentRange: &outside,
		}},
		Assumptions:         []core.MatchAssumption{},
		ClockDependencyNote: clockDependencyNoteText,
	}
	if basis.Validate() != nil {
		return core.EdgeAssignmentBasis{}, false
	}
	return basis, true
}
