package pipeline

import (
	"fmt"
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// maxInfluenceFrontier は、応答に載せる、先へ影響が進まないノードの上限である。
//
// 既知の制限: 先へ影響が進まないノードを要素の並びの先頭から上限まで返し、総数を別に返す,
// 起点から届くノードの数は依存の爆発で入力の大きさに比例し、全件を画面で読めない,
// 分析者が上限より後ろのノードを読む操作が出たとき、続きを取る要求を足す
const maxInfluenceFrontier = 100

// maxInfluenceEdges は、応答に載せる G(a, b) の影響のエッジの上限である。
//
// 割り切り: 起点から終点への短い列に乗るエッジから順に上限まで返し、残りの本数を別に返す, 上限の値は
// 応答の大きさを抑えるために置いた, 分析者が上限より遠いエッジを読む操作が要るとき、続きを取る要求を足す
var maxInfluenceEdges = 3000

// InfluencePathQuery は影響の経路の要求である。
type InfluencePathQuery struct {
	// From と To は起点と終点のノードの識別子である。
	From, To string
	// Excluded は除く根拠の種類である。どれかを持つ影響のエッジを除いて経路を求める。
	Excluded []core.InfluenceBasis
}

// InfluencePath は、起点から終点へのどれかの経路に乗る影響のエッジの集合 G(a, b) である。
type InfluencePath struct {
	// Origin と Destination は起点と終点である。ノードがタイムスタンプを持つレコードを持たない
	// ときは nil である。
	Origin, Destination *core.InfluenceEndpoint
	// Vertices は、Edges の端と、起点と終点の要素である。並びは要素を作った順である。
	Vertices []core.InfluenceVertex
	// Edges は G(a, b) の影響のエッジである。並びは影響のエッジを作った順である。maxInfluenceEdges 本を
	// 超えるときは、起点から終点への短い列に乗るエッジから順に上限までを持ち (shortRouteEdges)、
	// 載せなかった本数は OmittedEdgeCount である。
	Edges            []core.InfluenceEdge
	OmittedEdgeCount int
	// Stops は、Edges が空である理由、または求めきれなかった理由である。
	Stops []core.InfluencePathStop
	// RouteExcludedBases は、Stops が core.InfluencePathStopRouteThroughExcludedBasis を持つときの、
	// 除いた根拠の種類を戻して求めた経路のエッジが持つ、除いた根拠の種類である。並びは要求の順を保つ。
	RouteExcludedBases []core.InfluenceBasis
	// OriginKind と DestinationKind は、Stops が core.InfluencePathStopOriginNotInfluenceEnd または
	// core.InfluencePathStopDestinationNotInfluenceEnd を持つときの、その端のノードの種別である。
	// ほかのときは空である。
	OriginKind, DestinationKind core.NodeKind
	// Frontier は、起点から届いたが先へ影響が進まず、Edges の端でもないノードである。先頭から
	// maxInfluenceFrontier 件までを持ち、総数は FrontierCount である。
	Frontier      []core.InfluenceFrontier
	FrontierCount int
	// UntimedRecordCount は、タイムスタンプを持たず影響のエッジにしなかった根拠のレコードの数である。
	UntimedRecordCount int
	// StateCount は、区間を広げた計算と広げない計算と、除いた根拠の種類を戻して求め直した計算のうち、
	// 探索の状態の数が最も多い計算の数である (reachOutcome.states)。応答に出さず、所要の記録に使う。
	StateCount int
}

// InfluencePath は起点 query.From から終点 query.To への G(a, b) を求める。ok が偽になるのは、
// どちらかのノードがグラフに無いときである。
//
// **起点はノードの最初のレコード、終点はノードの最後のレコードのタイムスタンプである。**
// 区間は精度が秒のタイムスタンプを両端で広げ、広げないときに経路の条件を満たさない影響のエッジに
// 精度の幅の種類を付ける。起点と終点のタイムスタンプも同じ幅だけ外へ広げる。
func (g Graph) InfluencePath(query InfluencePathQuery) (InfluencePath, bool) {
	from, fromFound := g.nodeAt[query.From]
	to, toFound := g.nodeAt[query.To]
	if !fromFound || !toFound {
		return InfluencePath{}, false
	}
	origin, originTimed := g.boundaryRecord(from, true)
	destination, destinationTimed := g.boundaryRecord(to, false)
	var result InfluencePath
	if !originTimed {
		result.Stops = append(result.Stops, core.InfluencePathStopOriginWithoutTimestamp)
	}
	if !destinationTimed {
		result.Stops = append(result.Stops, core.InfluencePathStopDestinationWithoutTimestamp)
	}
	if !originTimed || !destinationTimed {
		return result, true
	}
	originAt, _ := g.influenceInstant(origin)
	destinationAt, _ := g.influenceInstant(destination)
	built := g.newInfluenceGraph(originAt)
	built.separateCandidates(from, to)
	result.UntimedRecordCount = len(built.untimed)
	kept := built.without(query.Excluded)
	a, aEnds := g.influenceEndOf(built, from, origin, -1)
	b, bEnds := g.influenceEndOf(built, to, destination, 1)
	if !aEnds {
		result.Stops = append(result.Stops, core.InfluencePathStopOriginNotInfluenceEnd)
		result.OriginKind = g.nodes[from].key.Kind
	}
	if !bEnds {
		result.Stops = append(result.Stops, core.InfluencePathStopDestinationNotInfluenceEnd)
		result.DestinationKind = g.nodes[to].key.Kind
	}
	if !aEnds || !bEnds {
		return result, true
	}
	aExact, bExact := a, b
	aExact.at, bExact.at = built.offset(originAt), built.offset(destinationAt)
	widened := reach(len(built.vertices), stepsOf(built, kept, true), a, b)
	exact := reach(len(built.vertices), stepsOf(built, kept, false), aExact, bExact)
	result.StateCount = max(widened.states, exact.states)
	// 区間を広げない計算が上限に達したときは、どのエッジが精度の幅だけで成り立つかが分からない。
	if exact.limited && !widened.limited {
		result.Stops = append(result.Stops, core.InfluencePathStopComputationLimit)
	}
	result.Origin = &core.InfluenceEndpoint{Key: g.vertexKeyOf(built, a.vertex), Record: g.evidenceItems([]int{origin})[0]}
	result.Destination = &core.InfluenceEndpoint{Key: g.vertexKeyOf(built, b.vertex), Record: g.evidenceItems([]int{destination})[0]}
	degree := make([]int64, len(built.vertices))
	for _, at := range kept {
		degree[built.edges[at].exact.from]++
		degree[built.edges[at].exact.to]++
	}
	chains := uncertainChainsInto(built, kept)
	onPath := map[int]bool{a.vertex: true, b.vertex: true}
	shown, omitted, routed := shortRouteEdges(built, kept, stepsOf(built, kept, true), widened, a, b)
	result.OmittedEdgeCount = omitted
	if !routed {
		result.Stops = append(result.Stops, core.InfluencePathStopTruncatedRouteUnverified)
	}
	for _, index := range shown {
		at := kept[index]
		edge := built.edges[at]
		onPath[edge.exact.from], onPath[edge.exact.to] = true, true
		_, exactOn := exact.onPath[index]
		item := g.influenceEdgeItem(built, edge, widened.onPath[index], exactOn || exact.limited)
		if edge.bases[1] == core.InfluenceBasisUncertainChain {
			item.CandidateCount = int64(len(chains[edge.exact.to]))
			item.CandidateTally, item.CandidateOrder = g.chainCandidateOrder(edge)
		}
		result.Edges = append(result.Edges, item)
	}
	for vertex := range built.vertices {
		if onPath[vertex] {
			result.Vertices = append(result.Vertices, g.influenceVertexItem(built, vertex, degree[vertex]))
		}
	}
	result.Stops = append(result.Stops, pathStops(widened, len(built.vertices), stepsOf(built, kept, true), a, b)...)
	if len(widened.onPath) == 0 && !widened.limited && len(query.Excluded) > 0 {
		addExcludedBasisRoute(&result, built, query.Excluded, a, b)
	}
	g.addFrontier(&result, built, kept, widened, onPath, degree)
	return result, true
}

// addExcludedBasisRoute は、根拠の種類を除かずに区間を広げて a から b への経路を求め直し、経路が
// あればそのエッジが持つ除いた種類 excluded と理由を、計算が上限に達したときはその理由を result へ足す。
// 求め直した計算の状態の数を result.StateCount に含める。
func addExcludedBasisRoute(result *InfluencePath, built *influenceGraph, excluded []core.InfluenceBasis, a, b pathEnd) {
	all := built.without(nil)
	outcome := reach(len(built.vertices), stepsOf(built, all, true), a, b)
	result.StateCount = max(result.StateCount, outcome.states)
	if outcome.limited {
		result.Stops = append(result.Stops, core.InfluencePathStopExcludedBasisRouteLimit)
		return
	}
	result.RouteExcludedBases = routeExcludedBases(built, all, outcome, excluded)
	if result.RouteExcludedBases != nil {
		result.Stops = append(result.Stops, core.InfluencePathStopRouteThroughExcludedBasis)
	}
}

// routeExcludedBases は、影響のエッジ all から求めた経路 outcome のエッジが持つ、除いた根拠の種類
// excluded を、excluded の並びで返す。経路が無いときは nil である。
func routeExcludedBases(
	built *influenceGraph, all []int, outcome reachOutcome, excluded []core.InfluenceBasis,
) []core.InfluenceBasis {
	held := map[core.InfluenceBasis]bool{}
	for index := range outcome.onPath {
		for _, basis := range built.edges[all[index]].bases {
			held[basis] = true
		}
	}
	var bases []core.InfluenceBasis
	for _, basis := range excluded {
		if held[basis] {
			bases = append(bases, basis)
		}
	}
	return bases
}

// chainCandidateOrder は、ログオンの連鎖のセッションから作った影響のエッジ edge の、候補の並びの
// 件数と、並びの区分を決めた条件の値を返す。セッションの始まりの記録と終点のログオンの組を、関係の
// レコードの組から探す。組が無いときは nil である。
func (g Graph) chainCandidateOrder(edge builtInfluenceEdge) (*core.EdgeCandidateTally, []core.EdgePairConditionKey) {
	graphEdge := g.edges[edge.graphEdge]
	start, target := edge.records[0], edge.records[len(edge.records)-1]
	for _, pair := range graphEdge.pairList() {
		if int(pair.left) != start || int(pair.right) != target || pair.rule < pairRuleLogonChain+logonChainSessionRules {
			continue
		}
		// 始まりの記録と終点の組の規則の条件は、最後にアカウントの一致とログオンの種別の区分を持つ。
		conditions := pair.rule.conditions()
		var order []core.EdgePairConditionKey
		for _, condition := range conditions[len(conditions)-2:] {
			order = append(order, condition.key)
		}
		return candidateTallyOf(graphEdge.sessionList(), pair), order
	}
	return nil, nil
}

// uncertainChainsInto は、kept の影響のエッジのうち関係の状態が不確定の連鎖であるものを、先の要素
// ごとに、根拠の関係の集合で返す。
func uncertainChainsInto(built *influenceGraph, kept []int) map[int]map[int]struct{} {
	chains := map[int]map[int]struct{}{}
	for _, at := range kept {
		edge := built.edges[at]
		if edge.bases[1] != core.InfluenceBasisUncertainChain {
			continue
		}
		if chains[edge.exact.to] == nil {
			chains[edge.exact.to] = map[int]struct{}{}
		}
		chains[edge.exact.to][edge.graphEdge] = struct{}{}
	}
	return chains
}

// shortRouteEdges は、G(a, b) に乗る影響のエッジの kept の中の位置を、そのエッジを通る起点
// origin から終点 destination への最短の列のエッジの数が小さい順に maxInfluenceEdges 本まで返し、
// 残りの本数を omitted で返す。
//
// 列のエッジの数は d_a(元) + 1 + d_b(先) である。d_a は G(a, b) のエッジだけを辿って起点から、
// d_b は終点まで後ろ向きに辿って数えた最少のエッジの数である。同じ数のエッジは d_a(元) の小さい
// 順、さらに同じなら kept の並びを保つ。
//
// **時刻の条件を満たす列の 1 本を、ほかのエッジより先に、起点の側から置く。** 列は、区間を広げた
// 制約 steps で終点へ最も早く届く列である (earliestRoute)。d_a と d_b は時刻を問わない数であり、
// 並べる順にだけ使う。
//
// routed が偽になるのは、エッジを上限で切り、返したエッジが時刻の条件を満たす起点から終点への列を
// 含むと確かめられないときである。タイムスタンプを記録した端末が 2 つ以上の入力と、起点と終点が
// 同じ要素の入力では列を求めない。列が上限より長いときは、列の起点の側の一部だけを返す。
func shortRouteEdges(
	built *influenceGraph, kept []int, steps []influenceStep, outcome reachOutcome, a, b pathEnd,
) (shown []int, omitted int, routed bool) {
	outgoing, incoming := map[int][]int{}, map[int][]int{}
	for index := range kept {
		if _, on := outcome.onPath[index]; on {
			outgoing[steps[index].from] = append(outgoing[steps[index].from], index)
			incoming[steps[index].to] = append(incoming[steps[index].to], index)
			shown = append(shown, index)
		}
	}
	if len(shown) <= maxInfluenceEdges {
		return shown, 0, true
	}
	fromOrigin := hopsFrom(a.vertex, outgoing, func(index int) int { return steps[index].to })
	toDestination := hopsFrom(b.vertex, incoming, func(index int) int { return steps[index].from })
	var route []int
	if outcome.oneClock {
		route = earliestRoute(steps, shown, a, b)
	}
	order := map[int]int{}
	for position, index := range route {
		order[index] = position + 1
	}
	// 列のエッジは列の並びで先頭に、ほかのエッジは列の後ろに置く。
	rank := func(index int) (int, int, int) {
		position, onRoute := order[index]
		if !onRoute {
			position = len(route) + 1
		}
		from, to := steps[index].from, steps[index].to
		return position, fromOrigin[from] + 1 + toDestination[to], fromOrigin[from]
	}
	slices.SortStableFunc(shown, func(left, right int) int {
		leftPosition, leftLength, leftHops := rank(left)
		rightPosition, rightLength, rightHops := rank(right)
		switch {
		case leftPosition != rightPosition:
			return leftPosition - rightPosition
		case leftLength != rightLength:
			return leftLength - rightLength
		default:
			return leftHops - rightHops
		}
	})
	routed = len(route) > 0 && len(route) <= maxInfluenceEdges
	return shown[:maxInfluenceEdges], len(shown) - maxInfluenceEdges, routed
}

// hopsFrom は、要素 start から、要素ごとのエッジ next を辿って各要素へ届く最少のエッジの数を返す。
// end はエッジの位置から、辿った先の要素を返す。
func hopsFrom(start int, next map[int][]int, end func(int) int) map[int]int {
	hops := map[int]int{start: 0}
	queue := []int{start}
	for len(queue) > 0 {
		vertex := queue[0]
		queue = queue[1:]
		for _, index := range next[vertex] {
			other := end(index)
			if _, found := hops[other]; !found {
				hops[other] = hops[vertex] + 1
				queue = append(queue, other)
			}
		}
	}
	return hops
}

// boundaryRecord は、ノード node の根拠のレコードのうち、タイムスタンプが最も早い (first が真)
// または最も遅いレコードを返す。同じタイムスタンプのレコードは根拠の並びの先の方を選ぶ。
//
// **前後を比べるのは、比べられるタイムスタンプどうしだけである。** UTC の時点を持つレコードから
// 選び、無いときは、UTC からのずれの分からないレコードのうち、根拠の並びで最初のレコードと同じ
// 収集元 (同じ端末) のレコードから選ぶ。
//
// 既知の制限: UTC の時点を持つレコードがあるノードでは、地方時のレコードを起点と終点に選ばない, 地方時の
// 壁時計の日時は別の端末の時刻であり、どちらが先かは経路の条件のずれで決まり、1 件を選べない,
// 定義の「起点または終点を 2 つ以上選んだときの和集合」を求める操作ができたとき、比べられない
// レコードをすべて候補にして和集合を出す
func (g Graph) boundaryRecord(node int, first bool) (int, bool) {
	evidence := g.nodes[node].evidence
	candidates := slices.DeleteFunc(slices.Clone(evidence), func(at int) bool { return !g.records[at].hasInstant })
	if len(candidates) == 0 {
		local := slices.IndexFunc(evidence, func(at int) bool {
			_, timed := g.influenceInstant(at)
			return timed
		})
		if local < 0 {
			return -1, false
		}
		source := g.records[evidence[local]].locator.SourceId
		candidates = slices.DeleteFunc(slices.Clone(evidence), func(at int) bool {
			_, timed := g.influenceInstant(at)
			return !timed || g.records[at].locator.SourceId != source
		})
	}
	chosen := candidates[0]
	chosenAt, _ := g.influenceInstant(chosen)
	for _, at := range candidates[1:] {
		instant, _ := g.influenceInstant(at)
		if (first && instant.Before(chosenAt)) || (!first && instant.After(chosenAt)) {
			chosen, chosenAt = at, instant
		}
	}
	return chosen, true
}

// pathEndOf は、ノード node とレコード record のタイムスタンプの起点 (outward が負) または終点
// (outward が正) を返す。精度が秒のタイムスタンプは、精度の幅だけ外へ広げる。
func (g Graph) pathEndOf(built *influenceGraph, node, record, outward int) pathEnd {
	instant, _ := g.influenceInstant(record)
	// 地方時のレコードを選ぶのは UTC の時点を持つレコードの無いノードであり (boundaryRecord)、その
	// ノードは UTC の置き換えの記録を持たない。バージョンを分けない要素になる。
	vertex := built.plainVertex(node)
	if g.records[record].hasInstant {
		vertex = g.versionsOfInstant(built, node, instant)
	}
	at := built.offset(instant)
	if g.secondPrecision(record) {
		at += int64(outward) * secondPrecisionWidth
	}
	return pathEnd{vertex: vertex, clock: built.clockOf(g, record), at: at}
}

// influenceEndOf は、ノード node とレコード record の起点または終点を返す (pathEndOf)。
//
// **レコードのノードに影響のエッジが 1 本も無いときは、そのレコードが記録したプロセス (actingEnd)
// に読み替える。** 起動のレコードは起動したプロセスになる。ok が偽になるのは、読み替えた後も、端の
// ノードのどの要素にも影響のエッジが根拠の種類で除く前から 1 本も無いときである。
func (g Graph) influenceEndOf(built *influenceGraph, node, record, outward int) (pathEnd, bool) {
	end := g.pathEndOf(built, node, record, outward)
	if built.touches(node) {
		return end, true
	}
	if g.nodes[node].key.Kind != core.NodeKindRecord {
		return end, false
	}
	acting, present := g.actingEnd(record)
	if !present || acting == node {
		return end, false
	}
	end.vertex = built.plainVertex(acting)
	return end, built.touches(acting)
}

// touches は、ノード node のどれかの要素を端に持つ影響のエッジがあるかを返す。
func (b *influenceGraph) touches(node int) bool {
	return slices.ContainsFunc(b.edges, func(edge builtInfluenceEdge) bool {
		return b.vertices[edge.exact.from].node == node || b.vertices[edge.exact.to].node == node
	})
}

// versionsOfInstant は、ノード node の時刻 at の内容のバージョンの要素を返す。
func (g Graph) versionsOfInstant(built *influenceGraph, node int, at time.Time) int {
	kind := g.nodes[node].key.Kind
	if kind != core.NodeKindFile && kind != core.NodeKindRegistryValue {
		return built.plainVertex(node)
	}
	start, replaced := built.versionStartAt(g, node, at)
	return built.vertex(node, start, replaced)
}

// without は、除く根拠の種類をどれも持たない影響のエッジの番号を返す。
//
// **同じ関係の同じレコードから同じ両端に作った影響のエッジは 1 本にする。** 経路の集合と、各要素の
// 影響のエッジの数は、1 本にした後の集合から求める。
func (b *influenceGraph) without(excluded []core.InfluenceBasis) []int {
	kept := make([]int, 0, len(b.edges))
	seen := map[string]bool{}
	for at, edge := range b.edges {
		if slices.ContainsFunc(edge.bases[:], func(basis core.InfluenceBasis) bool { return slices.Contains(excluded, basis) }) {
			continue
		}
		key := fmt.Sprint(edge.graphEdge, edge.exact.from, edge.exact.to, edge.records)
		if !seen[key] {
			seen[key] = true
			kept = append(kept, at)
		}
	}
	return kept
}

// stepsOf は、kept の影響のエッジの制約を、区間を広げた値 (widened が真) または広げない値で返す。
func stepsOf(built *influenceGraph, kept []int, widened bool) []influenceStep {
	steps := make([]influenceStep, len(kept))
	for index, at := range kept {
		steps[index] = built.edges[at].exact
		if widened {
			steps[index] = built.edges[at].widened
		}
	}
	return steps
}

// pathStops は、G(a, b) が空である理由を返す。
func pathStops(outcome reachOutcome, vertexCount int, steps []influenceStep, a, b pathEnd) []core.InfluencePathStop {
	switch {
	case outcome.limited:
		return []core.InfluencePathStop{core.InfluencePathStopComputationLimit}
	case len(outcome.onPath) > 0:
		return nil
	case connects(vertexCount, steps, a.vertex, b.vertex):
		return []core.InfluencePathStop{core.InfluencePathStopTimeOrderUnsatisfied}
	default:
		return []core.InfluencePathStop{core.InfluencePathStopNoInfluenceRoute}
	}
}

// connects は、時刻を問わずに、要素 from から to へ影響のエッジを 1 本以上辿れるかを返す。
func connects(vertexCount int, steps []influenceStep, from, to int) bool {
	outgoing := make([][]int, vertexCount)
	for _, step := range steps {
		outgoing[step.from] = append(outgoing[step.from], step.to)
	}
	seen := make([]bool, vertexCount)
	queue := slices.Clone(outgoing[from])
	for len(queue) > 0 {
		vertex := queue[0]
		queue = queue[1:]
		if vertex == to {
			return true
		}
		if seen[vertex] {
			continue
		}
		seen[vertex] = true
		queue = append(queue, outgoing[vertex]...)
	}
	return false
}

// addFrontier は、起点から届いたが先へ影響が進まず、経路に乗らない要素を result へ足す。
// kept は根拠の種類で除いた後の影響のエッジの番号である。
func (g Graph) addFrontier(
	result *InfluencePath, built *influenceGraph, kept []int, outcome reachOutcome, onPath map[int]bool, degree []int64,
) {
	outgoingAll, outgoingKept := make([]int, len(built.vertices)), make([]int, len(built.vertices))
	for _, edge := range built.edges {
		outgoingAll[edge.exact.from]++
	}
	for _, at := range kept {
		outgoingKept[built.edges[at].exact.from]++
	}
	// 候補ごとに分けた接続のレコードの要素 (separateCandidates) は、同じノードを 1 つにまとめる。
	// 経路に乗った要素のノードは、ほかの要素も frontier に出さない。
	listed := map[int]bool{}
	for vertex, on := range onPath {
		if item := built.vertices[vertex]; on && !item.versioned {
			listed[item.node] = true
		}
	}
	for _, vertex := range outcome.reached {
		item := built.vertices[vertex]
		if onPath[vertex] || (!item.versioned && listed[item.node]) {
			continue
		}
		var reason core.InfluenceFrontierReason
		switch {
		case outcome.blocked[vertex]:
			reason = core.InfluenceFrontierReasonOutgoingTimeUnsatisfied
		case outgoingAll[vertex] == 0:
			reason = core.InfluenceFrontierReasonNoOutgoingInfluence
		case outgoingKept[vertex] == 0:
			reason = core.InfluenceFrontierReasonOutgoingExcluded
		default:
			continue
		}
		listed[item.node] = true
		result.FrontierCount++
		if len(result.Frontier) < maxInfluenceFrontier {
			result.Frontier = append(result.Frontier, core.InfluenceFrontier{
				InfluenceVertex: g.influenceVertexItem(built, vertex, degree[vertex]), Reason: reason,
			})
		}
	}
}

// influenceVertexItem は要素を応答の項目へ直す。
func (g Graph) influenceVertexItem(built *influenceGraph, vertex int, degree int64) core.InfluenceVertex {
	item := built.vertices[vertex]
	vertexItem := core.InfluenceVertex{Key: g.vertexKeyOf(built, vertex), Node: g.graphNode(item.node), InfluenceEdgeCount: degree}
	if item.versioned {
		vertexItem.VersionStart = item.start.UTC().Format(time.RFC3339Nano)
	}
	if item.entered {
		via := g.graphNode(item.via)
		vertexItem.EnteredFrom = &via
	}
	return vertexItem
}

// influenceEdgeItem は影響のエッジを応答の項目へ直す。clocks は区間を広げたときの経路の端末、
// exactHolds は、広げないときにこのエッジを通る経路があるか、広げない計算が上限に達して分からない
// ことである。偽のときだけ精度の幅の種類を付ける。
//
// 既知の制限: 精度の幅の種類を、区間を広げないときにそのエッジを通る経路が 1 つも無いエッジにだけ
// 付ける, 経路ごとの種類を持つには経路を 1 本ずつ数え上げる必要があり、G(a, b) は経路の数を
// 持たない, 経路を 1 本ずつ出す操作を足したとき、経路ごとに広げないときの条件を確かめる
func (g Graph) influenceEdgeItem(built *influenceGraph, edge builtInfluenceEdge, clocks pathClocks, exactHolds bool) core.InfluenceEdge {
	var timeBases []core.InfluenceTimeBasis
	if clocks.single {
		timeBases = append(timeBases, core.InfluenceTimeBasisSameTerminal)
	}
	if clocks.mixed {
		timeBases = append(timeBases, core.InfluenceTimeBasisUnboundedOffset)
	}
	if !exactHolds {
		timeBases = append(timeBases, core.InfluenceTimeBasisPrecisionWidth)
	}
	source, target := g.vertexKeyOf(built, edge.exact.from), g.vertexKeyOf(built, edge.exact.to)
	graphEdge := g.edges[edge.graphEdge]
	parts := []string{graphEdge.id, source, target}
	for _, record := range edge.records {
		locator := g.records[record].locator
		parts = append(parts, locator.SourceContentSha256, locatorPositionKey(locator))
	}
	return core.InfluenceEdge{
		Id: "i:" + identityDigest(parts), SourceKey: source, TargetKey: target,
		GraphEdgeId: graphEdge.id, GraphEdgeKind: graphEdge.kind,
		Bases:     slices.Clone(edge.bases[:]),
		TimeBases: timeBases,
		Evidence:  g.evidenceItems(edge.records),
	}
}
