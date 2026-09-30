package pipeline

import (
	"slices"
	"strconv"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// secondPrecisionWidth は、精度が秒のタイムスタンプの区間を両端で広げる幅である。
const secondPrecisionWidth = int64(time.Second)

// influenceVertexOf は影響の経路の要素 1 つであり、グラフのノードと、内容のバージョンである。
// versioned が偽の要素は、置き換えの記録の前から続くバージョン、またはバージョンに分けない
// 種別のノードである。
//
// entered が真の要素は、2 件以上の候補のプロセスを持つレコードのノードに、候補のプロセス via から
// 入った要素である (separateCandidates)。
type influenceVertexOf struct {
	node      int
	start     time.Time
	versioned bool
	entered   bool
	via       int
}

// vertexKey は要素を探す鍵である。
type vertexKey struct {
	node      int
	start     int64
	versioned bool
	entered   bool
	via       int
}

// candidateSend は、候補のプロセス candidate から接続のレコードへの影響のエッジ 1 本の番号 edge である。
type candidateSend struct{ candidate, edge int }

// builtInfluenceEdge は根拠のレコードから作った影響のエッジ 1 本である。exact は区間を広げない
// 制約、widened は精度が秒のタイムスタンプの区間を両端で広げた制約である。
type builtInfluenceEdge struct {
	exact, widened influenceStep
	records        []int
	graphEdge      int
	bases          [3]core.InfluenceBasis
}

// influenceGraph は、グラフの全エッジの根拠のレコードから作った影響のエッジの集まりである。
// 時刻は base からの ns の差である。
type influenceGraph struct {
	base     time.Time
	vertices []influenceVertexOf
	vertexAt map[vertexKey]int
	edges    []builtInfluenceEdge
	clocks   map[string]int
	// untimed は、タイムスタンプを持たず、影響のエッジにしなかった根拠のレコードの位置の集合である。
	untimed map[int]struct{}
	// replacements は、ノードごとの置き換えの記録のタイムスタンプの昇順の一覧である
	// (Graph.contentReplacements)。ノードを初めて探したときに作る。
	replacements map[int][]time.Time
	// replacementAt は、ノードごとの置き換えの記録の位置の一覧である (replacementRecords)。
	replacementAt map[int][]int
	// registrationWrites は、主体のプロセスから登録の内容のバージョンへの影響のエッジを作った
	// 登録のレコードである。1 件の登録から 1 本だけ作る。
	registrationWrites map[int]struct{}
	// candidateSends は、関連付けの候補のプロセスから接続のレコードの要素への影響のエッジを、
	// レコードの要素ごとに持つ。candidateReceives は、接続のレコードから候補のプロセスへの影響の
	// エッジの番号の集合である (separateCandidates)。
	candidateSends    map[int][]candidateSend
	candidateReceives map[int]struct{}
	// requestedDestinations は、接続先への影響のエッジを作った、接続のレコードと接続先のノードの組である。
	requestedDestinations map[[2]int]struct{}
}

// constraintOf は、1 件のレコードの時刻の制約を、区間を広げない値と広げた値で返す。
type constraintOf struct{ exact, widened clockSpan }

// newInfluenceGraph は、グラフの全エッジの根拠のレコードから影響のエッジを作る。base は時刻の
// 差の基準である。
//
// **影響のエッジは根拠のレコードとその欄の役割から作る。** グラフのエッジは同じ種別と両端の
// レコードを 1 本に束ね、読み込みと書き込みの記録が 1 本に混ざる。関係の種別ごとの作り方は
// addInfluenceOf が持つ。
func (g Graph) newInfluenceGraph(base time.Time) *influenceGraph {
	built := &influenceGraph{
		base: base, vertexAt: map[vertexKey]int{}, clocks: map[string]int{},
		registrationWrites: map[int]struct{}{}, untimed: map[int]struct{}{}, replacements: map[int][]time.Time{},
		replacementAt: map[int][]int{}, candidateSends: map[int][]candidateSend{}, candidateReceives: map[int]struct{}{},
		requestedDestinations: map[[2]int]struct{}{},
	}
	for at := range g.edges {
		g.addInfluenceOf(built, at)
	}
	return built
}

// addInfluenceOf は、エッジ at の根拠のレコードから影響のエッジを作る。
//
// レコードのノードを端に持つ関係は、そのレコードが記録したプロセス (操作の主体、起動した
// プロセス) を影響のエッジの端にし (actingEnd)、ログオンのレコードと、接続を表すレコードと、
// 登録のレコードはレコードのノードを端にする (recordEnd)。
func (g Graph) addInfluenceOf(built *influenceGraph, at int) {
	edge := g.edges[at]
	relation := core.InfluenceBasisOfState(edge.state)
	switch edge.kind {
	case core.EdgeKindProcessParentChild, core.EdgeKindProcessInjection, core.EdgeKindProcessExecutable:
		if edge.state == core.RelationStateObserved {
			for _, record := range edge.evidence {
				g.addSingleRecord(built, at, record, edge.source, edge.target, core.InfluenceBasisSpecifiedOperation, relation)
			}
			return
		}
		for _, pair := range edge.pairList() {
			left, right := int(pair.left), int(pair.right)
			g.addRecordPairInfluence(built, at, left, right, edge.source, edge.target, relation)
		}
	case core.EdgeKindFileOperation, core.EdgeKindRegistryOperation:
		for _, record := range edge.evidence {
			toObject, fromObject := g.recordFlow(record, core.SemanticKeyEventWrittenBytes, core.SemanticKeyEventReadBytes)
			if g.records[record].flowOperation == core.FlowOperationRename {
				continue
			}
			g.addSingleRecord(built, at, record, edge.source, edge.target, toObject, relation)
			g.addSingleRecord(built, at, record, edge.target, edge.source, fromObject, relation)
		}
	case core.EdgeKindFileCopy:
		for _, record := range edge.evidence {
			g.addSingleRecord(built, at, record, edge.source, edge.target, g.specifiedBasis(record), relation)
			process, recorded := g.recordedProcessAt(record)
			if recorded && g.records[record].flowOperation == core.FlowOperationRename {
				g.addSingleRecord(built, at, record, process, edge.target, g.specifiedBasis(record), relation)
			}
		}
	case core.EdgeKindProcessCommunication:
		for _, record := range edge.evidence {
			connection, present := g.recordEnd(record)
			if !present {
				continue
			}
			send, receive := g.recordFlow(record, core.SemanticKeyConnectionSentBytes, core.SemanticKeyConnectionReceivedBytes)
			g.addSingleRecord(built, at, record, edge.source, connection, send, relation)
			g.addSingleRecord(built, at, record, connection, edge.source, receive, relation)
		}
	case core.EdgeKindCrossSourceConnectionMatch:
		g.addConnectionMatchInfluence(built, at, relation)
	case core.EdgeKindSameConnectionMatch:
		// 端末をまたぐ組だけが、接続した側の接続から受け付けたプロセスへの影響のエッジになる。
		// 同じ端末の開いた記録と閉じた記録の組は、端末をまたぐ影響のエッジの対応に該当しない。
		for _, pair := range edge.pairList() {
			if pair.rule == pairRuleSameConnectionAcrossTerminals {
				g.addPairInfluence(built, at, pair, g.recordEnd, g.actingEnd, relation)
			}
		}
	case core.EdgeKindTaskRegistrationRun:
		for _, pair := range edge.pairList() {
			g.addRegistrationWrite(built, at, int(pair.left))
			g.addPairInfluence(built, at, pair, g.recordEnd, g.actingEnd, relation)
		}
	case core.EdgeKindConnectionLogonMatch:
		for _, pair := range edge.pairList() {
			g.addPairInfluence(built, at, pair, g.recordEnd, g.recordEnd, relation)
		}
	case core.EdgeKindExplicitCredentialLogon, core.EdgeKindTicketRequestLogon:
		for _, pair := range edge.pairList() {
			g.addPairInfluence(built, at, pair, g.actingEnd, g.recordEnd, relation)
		}
	case core.EdgeKindLogonSessionOperation:
		for _, pair := range edge.pairList() {
			g.addPairInfluence(built, at, pair, g.recordEnd, g.actingEnd, relation)
		}
	case core.EdgeKindLinkedLogon:
		for _, pair := range edge.pairList() {
			g.addLinkedLogonInfluence(built, at, int(pair.left), int(pair.right))
		}
	case core.EdgeKindRequestedSessionLogon, core.EdgeKindLogonChain:
		g.addSessionInfluence(built, at, relation)
	case core.EdgeKindProcessIdentityMatch:
		g.addProcessIdentityInfluence(built, at)
	case core.EdgeKindArgumentNamesObject:
		// 端末を端にすると、同じ端末のすべてのプロセスがつながる。
		if g.nodes[edge.target].key.Kind == core.NodeKindTerminal {
			return
		}
		for _, record := range edge.evidence {
			g.addSingleRecord(built, at, record, edge.source, edge.target, core.InfluenceBasisArgumentName, relation)
		}
	case core.EdgeKindRecordTargetAccount:
		for _, record := range edge.evidence {
			switch g.records[record].flowOperation {
			case core.FlowOperationAccountManagement:
				if subject, present := g.actingEnd(record); present {
					g.addSingleRecord(built, at, record, subject, edge.target, core.InfluenceBasisAccountManagement, relation)
				}
			case core.FlowOperationLogon:
				// ログオンしたアカウントのノード (識別子と名前) ごとに 1 本である。
				if logon, present := g.recordEnd(record); present {
					g.addSingleRecord(built, at, record, edge.target, logon, core.InfluenceBasisCredentialUse, relation)
				}
			}
		}
	}
	// ほかの種別 (http_request、file_content_match、account_identity_match、端末を端に持つ関係、
	// アカウントの管理操作とログオンを除く、レコードが対象を指す関係) からは影響のエッジを作らない。
}

// addPairInfluence は、2 件のレコードの組 pair から、元の側のレコードの端 fromEnd から先の側の
// レコードの端 toEnd への影響のエッジを作る。D は元の側、A は先の側のレコードの制約である。
// どちらかの端を探せないときは作らない。
func (g Graph) addPairInfluence(
	built *influenceGraph, at int, pair recordPair, fromEnd, toEnd func(int) (int, bool), relation core.InfluenceBasis,
) {
	left, right := int(pair.left), int(pair.right)
	from, fromPresent := fromEnd(left)
	to, toPresent := toEnd(right)
	if fromPresent && toPresent {
		g.addRecordPairInfluence(built, at, left, right, from, to, relation)
	}
}

// addRegistrationWrite は、登録のレコード registration が登録した主体のプロセスを記録するとき、
// そのプロセスから登録の内容のバージョンへの書き込みの影響のエッジを作る。登録 1 件から 1 本である。
func (g Graph) addRegistrationWrite(built *influenceGraph, at, registration int) {
	if _, done := built.registrationWrites[registration]; done {
		return
	}
	built.registrationWrites[registration] = struct{}{}
	process, recorded := g.recordedProcessAt(registration)
	version, present := g.recordEnd(registration)
	if !recorded || !present {
		return
	}
	g.addSingleRecord(built, at, registration, process, version, core.InfluenceBasisSpecifiedOperation,
		core.InfluenceBasisObserved)
}

// addConnectionMatchInfluence は、別の収集元のレコードが表す接続と、候補のプロセスの間の通信の
// 影響のエッジを両方の向きに作る。候補のプロセスは関係の終点、接続は起点のレコードのノードである。
// 接続から、そのレコードが指す接続先 (関係の起点) への影響のエッジも作る。
func (g Graph) addConnectionMatchInfluence(built *influenceGraph, at int, relation core.InfluenceBasis) {
	edge := g.edges[at]
	for index, match := range edge.matchList() {
		origin, candidate, resolved := g.matchRecordsOf(edge, index, match)
		if !resolved {
			continue
		}
		connection, present := g.recordEnd(origin)
		if !present {
			continue
		}
		sent := len(built.edges)
		g.addRecordPairInfluence(built, at, candidate, origin, edge.target, connection, relation)
		received := len(built.edges)
		g.addRecordPairInfluence(built, at, origin, candidate, connection, edge.target, relation)
		vertex := built.plainVertex(connection)
		for index := sent; index < received; index++ {
			built.candidateSends[vertex] = append(built.candidateSends[vertex], candidateSend{edge.target, index})
		}
		for index := received; index < len(built.edges); index++ {
			built.candidateReceives[index] = struct{}{}
		}
		// 同じレコードの候補ごとの関係から、接続先への影響のエッジを 1 本だけ作る。
		if _, done := built.requestedDestinations[[2]int{origin, edge.source}]; !done {
			built.requestedDestinations[[2]int{origin, edge.source}] = struct{}{}
			g.addSingleRecord(built, at, origin, connection, edge.source, core.InfluenceBasisRequestedDestination,
				core.InfluenceBasisObserved)
		}
	}
}

// matchRecordsOf は、エッジ edge の index 番目の関連付けの起点と候補のレコードの位置を返す。
// 位置と段階から組み直せない関連付けは、両端のレコードを位置で探し直す。ok が偽になるのは、
// 探し直した位置がグラフの根拠に無いときである。
func (g Graph) matchRecordsOf(edge graphEdge, index int, match candidateMatch) (origin, candidate int, ok bool) {
	exact, found := edge.basis.exactMatches[index]
	if !found {
		return int(g.matchStages[match.stage].origin), int(match.candidate), true
	}
	origin, originFound := g.recordAtLocator(exact.OriginRef)
	candidate, candidateFound := g.recordAtLocator(exact.CandidateRef)
	return origin, candidate, originFound && candidateFound
}

// addSessionInfluence は、ログオンのセッションから終点のログオンのセッションへの影響のエッジを作る。
// D は起点のセッションの期間、A は終点のログオンのレコードの制約である。要求したセッションの関係も
// D をセッションの期間にする。期間の端は、始まりと終わりを決めた記録の精度で広げる。
func (g Graph) addSessionInfluence(built *influenceGraph, at int, relation core.InfluenceBasis) {
	edge := g.edges[at]
	for _, session := range edge.sessionList() {
		// セッションの期間は時点を持つレコードだけから組む (logonSessionPeriodsOf)。
		arrive, timed := g.recordConstraint(built, session.target)
		if !timed {
			continue
		}
		clock := built.clockOf(g, session.startAt)
		exact := timeSpan{built.offset(session.start), built.offset(session.end)}
		widened := exact
		if g.secondPrecision(session.startAt) {
			widened.lo -= secondPrecisionWidth
		}
		if g.secondPrecision(session.endAt) {
			widened.hi += secondPrecisionWidth
		}
		built.add(builtInfluenceEdge{
			exact:     influenceStep{depart: []clockSpan{{clock, exact}}, arrive: []clockSpan{arrive.exact}},
			widened:   influenceStep{depart: []clockSpan{{clock, widened}}, arrive: []clockSpan{arrive.widened}},
			records:   uniqueRecords(session.startAt, session.endAt, session.target),
			graphEdge: at,
			bases:     [3]core.InfluenceBasis{core.InfluenceBasisSpecifiedOperation, relation, core.InfluenceBasisSingleNode},
		}, built.plainVertex(edge.source), built.plainVertex(edge.target))
	}
}

// addLinkedLogonInfluence は、1 つのログオンが分けた 2 つのトークンのログオンのレコードを、
// 等価の影響のエッジで結ぶ。D と A はどちらも、各ログオンのレコードのタイムスタンプから後の範囲である。
func (g Graph) addLinkedLogonInfluence(built *influenceGraph, at, left, right int) {
	first, firstPresent := g.recordEnd(left)
	second, secondPresent := g.recordEnd(right)
	leftFrom, leftTimed := g.recordConstraint(built, left)
	rightFrom, rightTimed := g.recordConstraint(built, right)
	if !firstPresent || !secondPresent {
		return
	}
	if !leftTimed || !rightTimed {
		return
	}
	for _, constraint := range []*constraintOf{&leftFrom, &rightFrom} {
		constraint.exact.span.hi, constraint.widened.span.hi = noUpper, noUpper
	}
	g.addEquivalence(built, at, first, second, []constraintOf{leftFrom, rightFrom}, []int{left, right})
}

// addProcessIdentityInfluence は、一意な識別子で識別したプロセスと、プロセス番号と区間で識別した
// プロセスを、等価の影響のエッジで結ぶ。D と A はどちらも、各ノードを観測したタイムスタンプの範囲と、
// そのタイムスタンプを記録した端末の組である。
func (g Graph) addProcessIdentityInfluence(built *influenceGraph, at int) {
	edge := g.edges[at]
	source, sourceTimed := g.observedRange(built, g.nodes[edge.source].evidence)
	target, targetTimed := g.observedRange(built, g.nodes[edge.target].evidence)
	if !sourceTimed || !targetTimed {
		return
	}
	var records []int
	for _, pair := range edge.pairList() {
		records = append(records, int(pair.left), int(pair.right))
	}
	g.addEquivalence(built, at, edge.source, edge.target, []constraintOf{source, target}, records)
}

// observedRange は、レコードの集まりのうちタイムスタンプを持つものの最も早い時刻から最も遅い時刻
// までの制約を返す。端末は最も早いレコードの端末である。ok が偽になるのは、タイムスタンプを持つ
// レコードが無いときであり、そのときはレコードをタイムスタンプの無いレコードに数える。
//
// 既知の制限: 範囲を UTC の時点を持つレコードだけから求め、地方時だけのタイムスタンプのレコードを
// 範囲に入れない, 地方時の値は収集元ごとの別の端末の時刻であり、UTC の時点と 1 つの範囲に並べられない,
// 地方時だけで観測したプロセスの一致を読む要求が出たとき、端末ごとの範囲を別の制約にする
func (g Graph) observedRange(built *influenceGraph, records []int) (constraintOf, bool) {
	first, last := -1, -1
	for _, at := range records {
		record := g.records[at]
		if !record.hasInstant {
			continue
		}
		if first < 0 || record.instant.Before(g.records[first].instant) {
			first = at
		}
		if last < 0 || record.instant.After(g.records[last].instant) {
			last = at
		}
	}
	if first < 0 {
		for _, at := range records {
			built.untimed[at] = struct{}{}
		}
		return constraintOf{}, false
	}
	clock := built.clockOf(g, first)
	exact := timeSpan{built.offset(g.records[first].instant), built.offset(g.records[last].instant)}
	widened := exact
	if g.secondPrecision(first) {
		widened.lo -= secondPrecisionWidth
	}
	if g.secondPrecision(last) {
		widened.hi += secondPrecisionWidth
	}
	return constraintOf{clockSpan{clock, exact}, clockSpan{clock, widened}}, true
}

// addEquivalence は、2 つの要素の間に、向きが逆の 2 本の等価の影響のエッジを作る。D と A は
// どちらも constraints である。
func (g Graph) addEquivalence(built *influenceGraph, at, first, second int, constraints []constraintOf, records []int) {
	var exact, widened []clockSpan
	for _, constraint := range constraints {
		exact = append(exact, constraint.exact)
		widened = append(widened, constraint.widened)
	}
	relation := core.InfluenceBasisOfState(g.edges[at].state)
	for _, pair := range [][2]int{{first, second}, {second, first}} {
		built.add(builtInfluenceEdge{
			exact:     influenceStep{depart: exact, arrive: exact},
			widened:   influenceStep{depart: widened, arrive: widened},
			records:   records,
			graphEdge: at,
			bases:     [3]core.InfluenceBasis{core.InfluenceBasisSpecifiedOperation, relation, core.InfluenceBasisEquivalence},
		}, built.plainVertex(pair[0]), built.plainVertex(pair[1]))
	}
}

// uniqueRecords はレコードの位置を、重なりを除いて並びのまま返す。
func uniqueRecords(records ...int) []int {
	unique := make([]int, 0, len(records))
	for _, record := range records {
		if !slices.Contains(unique, record) {
			unique = append(unique, record)
		}
	}
	return unique
}

// addRecordPairInfluence は、2 件のレコード left (元の側) と right (先の側) を根拠に、ノード from
// から to への影響のエッジを作る。D は left、A は right の制約である。元の側のノードの内容の
// バージョンは left の区間、先の側のノードの内容のバージョンは right の区間で探す。
func (g Graph) addRecordPairInfluence(
	built *influenceGraph, at, left, right, from, to int, relation core.InfluenceBasis,
) {
	depart, departTimed := g.recordConstraint(built, left)
	arrive, arriveTimed := g.recordConstraint(built, right)
	if !departTimed || !arriveTimed {
		return
	}
	for _, source := range g.versionsOf(built, from, left) {
		for _, target := range g.versionsOf(built, to, right) {
			built.add(builtInfluenceEdge{
				exact:     influenceStep{depart: source.constrain(depart.exact), arrive: target.constrain(arrive.exact)},
				widened:   influenceStep{depart: source.constrain(depart.widened), arrive: target.constrain(arrive.widened)},
				records:   uniqueRecords(left, right),
				graphEdge: at,
				bases:     [3]core.InfluenceBasis{core.InfluenceBasisSpecifiedOperation, relation, core.InfluenceBasisSingleNode},
			}, source.vertex, target.vertex)
		}
	}
}

// addSingleRecord は、1 件のレコード record が記録した操作の影響のエッジを、ノード from から to へ
// 作る。D と A は同じ 1 つの制約であり、T^d = T^a とする。direction が空のときは作らない。
func (g Graph) addSingleRecord(
	built *influenceGraph, at, record, from, to int, direction, relation core.InfluenceBasis,
) {
	if direction == "" {
		return
	}
	constraint, timed := g.recordConstraint(built, record)
	if !timed {
		return
	}
	for _, source := range g.versionsOf(built, from, record) {
		for _, target := range g.versionsOf(built, to, record) {
			exact := source.clip(target.clip(constraint.exact))
			widened := source.clip(target.clip(constraint.widened))
			if exact.span.lo > exact.span.hi {
				continue
			}
			bounds := slices.Concat(source.bounds, target.bounds)
			exactConstraints := append([]clockSpan{exact}, bounds...)
			widenedConstraints := append([]clockSpan{widened}, bounds...)
			built.add(builtInfluenceEdge{
				exact:     influenceStep{depart: exactConstraints, arrive: exactConstraints},
				widened:   influenceStep{depart: widenedConstraints, arrive: widenedConstraints},
				records:   []int{record},
				graphEdge: at,
				bases:     [3]core.InfluenceBasis{direction, relation, core.InfluenceBasisSingleNode},
			}, source.vertex, target.vertex)
		}
	}
}

// add は影響のエッジを、要素 from から to へ足す。
func (b *influenceGraph) add(edge builtInfluenceEdge, from, to int) {
	edge.exact.from, edge.exact.to = from, to
	edge.widened.from, edge.widened.to = from, to
	b.edges = append(b.edges, edge)
}

// recordFlow は、読み書きの量の欄と操作の分類から、レコード record の影響の向きごとの根拠の
// 種類を返す。toObject はプロセスから対象へ、fromObject は対象からプロセスへの向きである。空の
// 文字列はその向きの影響のエッジを作らないことである。
//
// **量を持つ記録は、量が正の向きを観測とする。** 量が 0 の向きと量の欄が無い向きは、向きを
// 決められない向きとし、両方の向きを作る。量の欄を持たない記録は操作の分類で決め、分類の無い
// 記録は両方の向きを、向きを決められない記録として作る。
func (g Graph) recordFlow(record int, toObjectItem, fromObjectItem core.SemanticKey) (toObject, fromObject core.InfluenceBasis) {
	toAmount, toCarried := g.recordAmount(record, toObjectItem)
	fromAmount, fromCarried := g.recordAmount(record, fromObjectItem)
	specified := g.specifiedBasis(record)
	if toCarried || fromCarried {
		toObject, fromObject = core.InfluenceBasisUndeterminedDirection, core.InfluenceBasisUndeterminedDirection
		if toAmount > 0 {
			toObject = specified
		}
		if fromAmount > 0 {
			fromObject = specified
		}
		return toObject, fromObject
	}
	switch g.records[record].flowOperation {
	case core.FlowOperationWrite:
		return specified, ""
	case core.FlowOperationRead:
		return "", specified
	case core.FlowOperationCommunication:
		return specified, specified
	default:
		return core.InfluenceBasisUndeterminedDirection, core.InfluenceBasisUndeterminedDirection
	}
}

// specifiedBasis は、レコードの操作から向きを決めたときの向きの根拠を返す。観測の種別の意味を
// 推定した記録は、仕様にない記録の推定である。
func (g Graph) specifiedBasis(record int) core.InfluenceBasis {
	if g.records[record].observationKind.Status == core.ObservationKindStatusInferred {
		return core.InfluenceBasisInferredRecord
	}
	return core.InfluenceBasisSpecifiedOperation
}

// recordAmount は、レコードの読み書きの量の欄 item の値を返す。ok が偽になるのは、欄が無いか、
// 値を整数として読めないときである。
func (g Graph) recordAmount(record int, item core.SemanticKey) (int64, bool) {
	field, present := g.recordAttribute(record, item)
	if !present || field.Text == nil {
		return 0, false
	}
	text, readable := field.Text.ComparableValue()
	if !readable {
		return 0, false
	}
	amount, err := strconv.ParseInt(text, 10, 64)
	return amount, err == nil
}

// recordAttribute は、レコードのノードの属性から、語彙の項目 item の最初の欄を返す。
func (g Graph) recordAttribute(record int, item core.SemanticKey) (*core.RecordField, bool) {
	if !g.records[record].hasRecordNode {
		return nil, false
	}
	for _, attribute := range g.nodes[g.records[record].recordNode].attributes {
		if attribute.field.Semantic == item {
			return attribute.field, true
		}
	}
	return nil, false
}

// recordEnd はレコードのノードを返す。ok が偽になるのは、レコードのノードが無いときである。
func (g Graph) recordEnd(record int) (int, bool) {
	return g.records[record].recordNode, g.records[record].hasRecordNode
}

// actingEnd は、レコードが記録したプロセス (操作の主体、起動したプロセス) を返す。プロセスを
// 記録しないレコードでは、レコードのノードを返す。
func (g Graph) actingEnd(record int) (int, bool) {
	if process, recorded := g.recordedProcessAt(record); recorded {
		return process, true
	}
	return g.recordEnd(record)
}

// secondPrecision は、レコードのタイムスタンプの精度が秒であるかを返す。
func (g Graph) secondPrecision(record int) bool {
	eventTime := g.records[record].eventTime
	return eventTime != nil && eventTime.Precision == core.PrecisionSecond
}

// influenceInstant は、レコード record のタイムスタンプを返す。UTC からのずれが分からない
// タイムスタンプは、壁時計の日時を返す。その値は収集元の端末の時刻である (clockOf)。ok が偽に
// なるのは、タイムスタンプを持たないレコードである。
func (g Graph) influenceInstant(record int) (time.Time, bool) {
	entry := g.records[record]
	if entry.hasInstant {
		return entry.instant, true
	}
	if entry.eventTime == nil {
		return time.Time{}, false
	}
	return entry.eventTime.LocalClockTime()
}

// recordConstraint は、レコード record が記録した操作の時刻の制約を返す。ok が偽になるのは、
// タイムスタンプを持たないレコードであり、そのレコードをタイムスタンプの無いレコードに数える。
//
// 区間は操作が起こりえた時刻の範囲である。期間の操作をまとめて 1 件に記録するレコード (読み書きの
// 量または送受信の量の欄を持つレコード) は、期間の始まり (操作を始めた時刻の欄) からレコードの
// タイムスタンプまで、始まりが記録に無いときは操作したプロセスの起動のタイムスタンプから、起動も
// 記録に無いときは下限を持たない。ほかのレコードはタイムスタンプの点である。
func (g Graph) recordConstraint(built *influenceGraph, record int) (constraintOf, bool) {
	instant, timed := g.influenceInstant(record)
	if !timed {
		built.untimed[record] = struct{}{}
		return constraintOf{}, false
	}
	at := built.offset(instant)
	exact := timeSpan{at, at}
	if lower, period := g.periodStart(record); period {
		exact.lo = noLower
		if !lower.IsZero() {
			exact.lo = min(built.offset(lower), at)
		}
	}
	widened := exact
	if g.secondPrecision(record) {
		if widened.lo != noLower {
			widened.lo -= secondPrecisionWidth
		}
		widened.hi += secondPrecisionWidth
	}
	clock := built.clockOf(g, record)
	return constraintOf{clockSpan{clock, exact}, clockSpan{clock, widened}}, true
}

// periodAmountItems は、期間の操作の量を持つ語彙の項目である。ファイルの読み書きの量と、接続の
// 送受信の量である。
var periodAmountItems = []core.SemanticKey{
	core.SemanticKeyEventReadBytes, core.SemanticKeyEventWrittenBytes,
	core.SemanticKeyConnectionSentBytes, core.SemanticKeyConnectionReceivedBytes,
}

// periodStart は、期間の操作をまとめて記録したレコードの期間の始まりを返す。period が偽のレコードは
// 1 回の操作の記録である。始まりが分からないときは zero の時刻を返す。
//
// 期間の記録は periodAmountItems のどれかを持つレコードである。
func (g Graph) periodStart(record int) (time.Time, bool) {
	if !slices.ContainsFunc(periodAmountItems, func(item core.SemanticKey) bool {
		_, carried := g.recordAttribute(record, item)
		return carried
	}) {
		return time.Time{}, false
	}
	if field, present := g.recordAttribute(record, core.SemanticKeyEventOperationStartTime); present && field.Timestamp != nil {
		if start, readable := field.Timestamp.Instant(); readable {
			return start, true
		}
	}
	process, recorded := g.recordedProcessAt(record)
	if !recorded {
		return time.Time{}, true
	}
	var start time.Time
	for _, creation := range g.nodes[process].creationRecords {
		if entry := g.records[creation]; entry.hasInstant && (start.IsZero() || entry.instant.Before(start)) {
			start = entry.instant
		}
	}
	return start, true
}

// offset は時刻を base からの ns の差で返す。
func (b *influenceGraph) offset(at time.Time) int64 { return int64(at.Sub(b.base)) }

// clockOf は、レコード record のタイムスタンプを記録した端末の番号を返す。端末の時計のタイム
// スタンプはレコードを置いた端末、ほかのタイムスタンプと、置いた端末の無いレコードは収集元である。
func (b *influenceGraph) clockOf(g Graph, record int) int {
	entry := g.records[record]
	key := "source:" + entry.locator.SourceId
	switch {
	case !entry.hasInstant:
		// UTC からのずれが分からないタイムスタンプは、収集元ごとの別の端末の時刻である。同じ収集元の
		// UTC の時点とも別の端末にする。
		key = "local:" + entry.locator.SourceId
	case entry.eventTime != nil && entry.eventTime.Clock == core.ClockTerminalLocal && entry.placedTerminalNodeId != "":
		key = "terminal:" + entry.placedTerminalNodeId
	}
	clock, known := b.clocks[key]
	if !known {
		clock = len(b.clocks)
		b.clocks[key] = clock
	}
	return clock
}

// versionEnd は、影響のエッジの端 1 つの要素と、その要素の内容のバージョンの期間である。
//
// period はレコードと同じ端末の時刻で比べた期間であり、制約の区間を切り詰める。bounds は、
// レコードと別の端末の時刻で記録した置き換えが決める期間であり、別の制約として足す。
type versionEnd struct {
	vertex int
	period timeSpan
	bounds []clockSpan
}

// clip は制約の区間を、端の内容のバージョンの期間に切り詰める。
func (v versionEnd) clip(constraint clockSpan) clockSpan {
	constraint.span = constraint.span.intersect(v.period)
	return constraint
}

// constrain は、制約 constraint を端の期間で切り詰め、別の端末の期間の制約を続けて返す。
func (v versionEnd) constrain(constraint clockSpan) []clockSpan {
	return append([]clockSpan{v.clip(constraint)}, v.bounds...)
}

// everyVersion は、ノード node の全部の内容のバージョンを、各バージョンの期間を置き換えを記録した
// 端末の時刻の制約にして返す (versionsOf)。
func (g Graph) everyVersion(built *influenceGraph, node int) []versionEnd {
	records := built.replacementRecords(g, node)
	var versions []versionEnd
	previous := -1
	for index := 0; index <= len(records); index++ {
		if index < len(records) && previous >= 0 && g.records[records[index]].instant.Equal(g.records[previous].instant) {
			continue
		}
		version := versionEnd{period: timeSpan{noLower, noUpper}}
		if previous < 0 {
			version.vertex = built.plainVertex(node)
		} else {
			version.vertex = built.vertex(node, g.records[previous].instant, true)
			version.bounds = append(version.bounds,
				clockSpan{built.clockOf(g, previous), timeSpan{built.offset(g.records[previous].instant), noUpper}})
		}
		if index < len(records) {
			next := records[index]
			version.bounds = append(version.bounds,
				clockSpan{built.clockOf(g, next), timeSpan{noLower, built.offset(g.records[next].instant) - 1}})
			previous = next
		}
		versions = append(versions, version)
	}
	return versions
}

// versionsOf は、ノード node の端を、レコード record の区間と重なる内容のバージョンごとに返す。
// ファイルとレジストリの値だけが内容のバージョンに分かれる (contentVersionStartAt)。ほかの種別の
// ノードは 1 つの要素である。
//
// 区間が 2 つ以上のバージョンと重なるときは、各バージョンの期間を境で切る。最も古いバージョンの
// 始まりと最も新しいバージョンの終わりは切らない。
//
// **UTC からのずれの分からないタイムスタンプのレコードは、UTC の置き換えの記録と前後を比べない。**
// 全部のバージョンにつなぎ、各バージョンの期間を置き換えを記録した端末の時刻の制約として足す
// (everyVersion)。経路の条件は、ずれを選んでその制約を満たすときだけ成り立つ。
//
// 既知の制限: 別の端末のレコードがどのバージョンに該当するかを、全部のバージョンへの影響のエッジと期間の
// 制約で表す, ずれは未知であり、定義の経路の条件は制約をずれとともに選べるかで決まる。候補を
// 1 つに決めると条件を満たす経路を除き、制約を付けずに全部へつなぐと条件を満たさない経路を足す,
// バージョンの数の多いファイルを地方時の記録が読み、影響のエッジの数が要求の所要に出たとき、
// バージョンの期間を 1 つの制約の区間の集まりとして持つ形を考える
func (g Graph) versionsOf(built *influenceGraph, node, record int) []versionEnd {
	kind := g.nodes[node].key.Kind
	if kind != core.NodeKindFile && kind != core.NodeKindRegistryValue {
		return []versionEnd{{vertex: built.plainVertex(node), period: timeSpan{noLower, noUpper}}}
	}
	if !g.records[record].hasInstant {
		return g.everyVersion(built, node)
	}
	lower, period := g.periodStart(record)
	var found []versionEnd
	upper := noUpper
	at, _ := g.influenceInstant(record)
	for {
		start, replaced := built.versionStartAt(g, node, at)
		version := versionEnd{vertex: built.vertex(node, start, replaced), period: timeSpan{noLower, upper}}
		if !replaced {
			found = append(found, version)
			break
		}
		version.period.lo = built.offset(start)
		found = append(found, version)
		if !period || (!lower.IsZero() && !start.After(lower)) {
			break
		}
		upper = built.offset(start) - 1
		at = start.Add(-time.Nanosecond)
	}
	found[len(found)-1].period.lo = noLower
	if len(found) == 1 {
		found[0].period = timeSpan{noLower, noUpper}
	}
	return found
}

// versionStartAt は、ノード node の時刻 at の内容のバージョンの始まりを返す (Graph.contentVersionStartAt)。
// ノードの置き換えの記録の一覧を 1 度だけ作り、二分探索で探す。
func (b *influenceGraph) versionStartAt(g Graph, node int, at time.Time) (time.Time, bool) {
	replacements, known := b.replacements[node]
	if !known {
		replacements = g.contentReplacements(node)
		b.replacements[node] = replacements
	}
	return versionStartIn(replacements, at)
}

// replacementRecords は、ノード node の置き換えの記録の位置を返す (Graph.contentReplacementRecords)。
// ノードごとに 1 度だけ作る。
func (b *influenceGraph) replacementRecords(g Graph, node int) []int {
	records, known := b.replacementAt[node]
	if !known {
		records = g.contentReplacementRecords(node)
		b.replacementAt[node] = records
	}
	return records
}

// vertexKeyOf は要素を指す文字列を返す。内容を置き換える記録の後のバージョンは、ノードの識別子に
// バージョンの始まりの時刻を続ける。
func (g Graph) vertexKeyOf(built *influenceGraph, vertex int) string {
	item := built.vertices[vertex]
	if item.entered {
		return g.nodes[item.node].id + "#via:" + g.nodes[item.via].id
	}
	if !item.versioned {
		return g.nodes[item.node].id
	}
	return g.nodes[item.node].id + "@" + item.start.UTC().Format(time.RFC3339Nano)
}

// plainVertex はバージョンに分けないノードの要素を返す。
func (b *influenceGraph) plainVertex(node int) int { return b.vertex(node, time.Time{}, false) }

// vertex はノードと内容のバージョンの要素を返す。まだ無ければ足す。
func (b *influenceGraph) vertex(node int, start time.Time, versioned bool) int {
	key := vertexKey{node: node, versioned: versioned}
	if versioned {
		key.start = start.UnixNano()
	}
	if at, found := b.vertexAt[key]; found {
		return at
	}
	b.vertices = append(b.vertices, influenceVertexOf{node: node, start: start, versioned: versioned})
	b.vertexAt[key] = len(b.vertices) - 1
	return len(b.vertices) - 1
}

// enteredVertex は、ノード node に候補のプロセス via から入った要素を返す。まだ無ければ足す。
func (b *influenceGraph) enteredVertex(node, via int) int {
	key := vertexKey{node: node, entered: true, via: via}
	if at, found := b.vertexAt[key]; found {
		return at
	}
	b.vertices = append(b.vertices, influenceVertexOf{node: node, entered: true, via: via})
	b.vertexAt[key] = len(b.vertices) - 1
	return len(b.vertices) - 1
}
