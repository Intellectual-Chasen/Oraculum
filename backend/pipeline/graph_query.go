package pipeline

import (
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// GraphQuery は部分グラフの要求である。定型の検索と近傍の展開を項目の組で表し、欄と演算子で
// 書いた条件を検索式 (Expression) で表す。
type GraphQuery struct {
	// NodeLimit は部分グラフのノードの数の上限である。0 は上限を置かない。
	//
	// **上限を超えた部分グラフは本体を組まない。** 合ったノードと、関係の種別ごとのエッジの本数だけを
	// 返す (Subgraph.NodeLimitExceeded)。
	NodeLimit int
	// NodeKinds はノードの種別で絞る。種別のどれかに一致するノードが残る。空の値は種別で
	// 絞らない。
	//
	// 粒度が対象のときは、近傍を辿って届くノードもこの種別に限る。
	NodeKinds []core.NodeKind
	// Granularity はレコードのノードを出すかである。ゼロ値はレコードの粒度として扱う。
	Granularity core.GraphGranularity
	// NodeIds は近傍を展開する起点のノードである。起点のどれかに一致するノードから広げる。
	// 空の値は起点を指定しない。
	NodeIds []string
	// Depth は起点から広げる段数である。0 はノードだけ、1 は隣り合うノードまで、
	// 2 以上はその段数だけ辿って到達したノードまでを返す。
	Depth int
	// EdgeKinds は関係の種別で絞る。どれかの種別の関係だけを辿り、応答に出す。空の値は
	// 種別で絞らない。段数 2 以上では、種別の混ざった経路を辿る。
	EdgeKinds []core.EdgeKind
	// AddressInPrefix はアドレスがこの範囲に入る IP アドレスのノードだけを残す。
	// AddressNotInPrefix はアドレスがこの範囲に入らない IP アドレスのノードだけを残す。
	// nil はその向きで絞らない。
	//
	// **どちらかを与えた要求は IP アドレスのノードだけを返す。** 他の種別のノードは
	// 比べるアドレスを持たない。
	AddressInPrefix    *netip.Prefix
	AddressNotInPrefix *netip.Prefix
	// ValueContains は、どの文字列も含む値を持つ属性のあるノードだけを残す。文字列ごとに
	// 一致する属性は別でよい。空の値は文字列で絞らない。
	// ValueExcludes は、どの文字列も含まないノードだけを残す。
	//
	// **判定の対象はノードの属性の値である。** レコードのノードがそのレコードの読めた欄を
	// すべて属性に持つため、レコードのノードでは 1 件のレコードの欄に対する判定になる。
	// 対象のノードでは識別鍵にしか現れない値も、そのレコードのノードが拾う。
	//
	// **粒度が対象のときは、レコードのノードで判定した結果を対象へ移す。** 絞り込みを通り、
	// 文字列の条件を満たしたレコードを 1 件以上根拠に持つ対象が合う。
	ValueContains []string
	ValueExcludes []string
	// ValueFieldSemantic と ValueFieldName は、ValueContains と ValueExcludes を当てる欄を 1 つに
	// 限る。語彙の項目と原資料の key のどちらか一方を与える。どちらも空の値は全欄に当てる
	// (searchesField)。ValueExcludes は、その欄に文字列を含まないノードを残す。
	ValueFieldSemantic core.SemanticKey
	ValueFieldName     string
	// FieldContains は欄と文字列の組である。どの組も、その欄に文字列を含む値を持つ属性のある
	// ノードだけを残す。組ごとに欄が違ってよく、ValueContains と同じく 1 件のレコードの欄に対する
	// 判定になる。ValueField の指定は組に当てない。
	FieldContains []FieldTerm
	// Expression は検索式である。式を満たす属性のあるノードだけを残す。nil は式で絞らない。
	// ValueContains と同じく、レコードのノードでは 1 件のレコードの欄に対する判定になる。
	Expression *SearchExpression
	// CountBySemantic と CountByName は、値ごとに数える欄を指す。語彙の項目と原資料の
	// key のどちらか一方を与える。どちらも空の値は数えない。
	CountBySemantic core.SemanticKey
	CountByName     string
	// RecordFilter は根拠のレコードを絞る条件である。
	RecordFilter
	// ConditionsOnOriginsOnly は、文字列の条件 (ValueContains・ValueExcludes・FieldContains・
	// Expression) と事象の種別の
	// 条件 (EventCategory・EventAction・動作の範囲) を起点のノードの判定だけに当てる。
	// 広げる段階のエッジは、関係の種別・ノードの種別・アドレスの範囲と、残りの絞り込み
	// (期間・案件・端末) で判定する。偽の値は広げる段階にも同じ条件を当てる。
	ConditionsOnOriginsOnly bool
	// EndpointRecordsInPeriod は、期間 (TimeFrom・TimeTo) を与えた要求で、端点のレコードのノードが
	// 期間の外にあるエッジを辿らない。偽の値は、根拠のレコードの 1 件が期間の中にあるエッジを辿る。
	// 端点がレコードのノードでないエッジには当てない。
	EndpointRecordsInPeriod bool
	// RecordSummary は、合致したレコードのノードに、指すレコードの位置と時刻と事象の種別を
	// 添えるかである (core.SubgraphNode.Record)。
	RecordSummary bool
	// valueFieldExact と countByExact は、ValueFieldName と CountByName を名前の一致だけで当てるか
	// である。公開の関数が withResolvedFieldNames で決める。
	valueFieldExact bool
	countByExact    bool
}

// FieldTerm は、文字列 Contains を当てる欄 1 つとの組である。欄は語彙の項目 Semantic と原資料の
// key Name のどちらか一方で指す。欄の名前の当て方は ValueFieldName と同じである (fieldDesignated)。
type FieldTerm struct {
	Semantic core.SemanticKey
	Name     string
	Contains string
	// WholeValue は、欄の値の全体が Contains と等しいときだけ一致とするかである。偽の組は値が
	// Contains を含むときに一致とする。どちらも大文字と小文字を区別しない。
	WholeValue bool
	// exact は Name を名前の一致だけで当てるかである (withResolvedFieldNames)。
	exact bool
}

// searches は欄が組の欄であるかを返す。WholeValue の組では、原資料の文字列か正規化値の全体が
// Contains と等しい欄だけを組の欄にする。文字列を含むかの判定は呼び出し元が行い、等しい値は
// その判定も満たす。
func (f FieldTerm) searches(field core.RecordField) bool {
	if !fieldDesignated(field, f.Semantic, f.Name, f.exact) {
		return false
	}
	if !f.WholeValue {
		return true
	}
	rawText, hasRaw := fieldRawText(field)
	normalized, hasNormalized := fieldNormalized(field)
	return hasRaw && strings.EqualFold(rawText, f.Contains) ||
		hasNormalized && strings.EqualFold(normalized, f.Contains)
}

// stepOf は、広げる段階のエッジの判定とその根拠の絞り込みに当てる要求と、根拠のレコード 1 件の
// 判定を返す。passes は query の判定 (recordPasses) である。
func (g Graph) stepOf(query GraphQuery, passes func(at int) bool) (GraphQuery, func(at int) bool) {
	if !query.ConditionsOnOriginsOnly {
		return query, passes
	}
	step := query.stepQuery()
	return step, g.recordPasses(step)
}

// stepQuery は広げる段階のエッジの判定に当てる要求を返す。
func (q GraphQuery) stepQuery() GraphQuery {
	if !q.ConditionsOnOriginsOnly {
		return q
	}
	step := q
	step.ValueContains, step.ValueExcludes, step.FieldContains, step.Expression = nil, nil, nil, nil
	step.EventCategory, step.EventAction = "", ""
	step.EventActionFrom, step.EventActionTo = nil, nil
	return step
}

// Subgraph は要求が選んだ部分グラフである。
//
// **Edges の両端は必ず Nodes にある。** 絞り込みに合わない端点も Nodes に入り、
// Selection が edge_endpoint になる。
type Subgraph struct {
	// Nodes は応答が含むノードである。
	Nodes []core.SubgraphNode
	// MatchedNodeCount は絞り込みに合うノードの件数である。
	//
	// 応答に入れた合致ノードの個数を別の項目で持たない。Nodes のうち Selection が
	// matched の要素の個数がその値である。
	MatchedNodeCount int
	// Edges は応答が含むエッジである。
	Edges []core.GraphEdge
	// EdgeCount はエッジの件数である。len(Edges) と等しい。
	EdgeCount int
	// ValueCounts は数える欄を与えた要求が返す、値ごとの件数である。
	ValueCounts []core.ValueCount
	// DistinctValueCount は値の異なりの個数である。len(ValueCounts) と等しい。
	//
	// **値の不在を値の 1 つとして数えない。** 収集元が値の不在を文字列で書く入力形式
	// (Squid の `-`) では、その文字列を値として数えると、記録されなかったことを記録された
	// 値と同じ扱いにする。
	//
	// **値ごとの件数の和と、合致したレコードの件数を比べない。** 1 レコードが同じ欄へ
	// 2 つの異なる値を持つと、そのレコードを 2 つの値が数える。和は合致したレコードの
	// 件数より小さくも大きくもなる (valueCounts の既知の制限)。
	DistinctValueCount int
	// SubgraphNodeCount は部分グラフのノードの数である。合ったノードと、辿ったエッジの端点の和である。
	// 上限を超えないときは len(Nodes) と等しい。
	SubgraphNodeCount int
	// NodeLimitExceeded は、SubgraphNodeCount が要求の上限 (GraphQuery.NodeLimit) を超えたかである。
	// 真のとき、Nodes は合ったノードだけを持ち、Edges は空であり、EdgeKindCounts が辿ったエッジの
	// 本数を関係の種別ごとに持つ。
	NodeLimitExceeded bool
	// EdgeKindCounts は、上限を超えたときに、辿ったエッジの本数を関係の種別ごとに持つ。種別の昇順に並ぶ。
	// 上限を超えないときは nil である。
	EdgeKindCounts []core.EdgeKindCount
	// MatchedKinds は、絞り込みに合うノードの種別ごとの件数である。種別の昇順に並ぶ。
	// 件数の和は MatchedNodeCount と等しい。
	MatchedKinds []core.MatchedKind
	// MatchedAccountIdentityPairCount は、絞り込みに合うアカウントのうち、同じアカウントの候補
	// (account_identity_match) で結ばれた SID と名前のノードの組の数である。MatchedKinds は
	// その組の 2 つのノードを別に数える。
	MatchedAccountIdentityPairCount int64
}

// objects は要求が対象の粒度であるかを返す。
func (q GraphQuery) objects() bool {
	return q.Granularity == core.GraphGranularityObject
}

// kindMatches はその種別のノードが種別の絞り込みを通るかを返す。
//
// **対象の粒度はレコードのノードを通さない。**
func (q GraphQuery) kindMatches(kind core.NodeKind) bool {
	if q.objects() && kind == core.NodeKindRecord {
		return false
	}
	return len(q.NodeKinds) == 0 || slices.Contains(q.NodeKinds, kind)
}

// SearchesText は要求が文字列の条件を持つかを返す。
func (q GraphQuery) SearchesText() bool {
	return len(q.ValueContains) > 0 || len(q.ValueExcludes) > 0 || len(q.FieldContains) > 0 ||
		q.Expression != nil
}

// filtersEventKind は要求が事象の種別 (分類・動作・動作の範囲) の条件を持つかを返す。
func (q GraphQuery) filtersEventKind() bool {
	return q.EventCategory != "" || q.EventAction != "" ||
		q.EventActionFrom != nil || q.EventActionTo != nil
}

// textMatches は、ノードの属性が文字列の条件を満たすかを返す。
func (q GraphQuery) textMatches(node graphNode) bool {
	for _, contains := range q.ValueContains {
		if !hasValueMatch(node, contains, q.searchesField) {
			return false
		}
	}
	for _, excludes := range q.ValueExcludes {
		if hasValueMatch(node, excludes, q.searchesField) {
			return false
		}
	}
	for _, term := range q.FieldContains {
		if !hasValueMatch(node, term.Contains, term.searches) {
			return false
		}
	}
	return q.Expression == nil || q.Expression.matches(node)
}

// searchesField は、文字列の条件を当てる欄であるかを返す。欄を指定しない要求は全欄に当てる。
//
// 欄の名前の当て方は fieldNamed である。
func (q GraphQuery) searchesField(field core.RecordField) bool {
	if q.ValueFieldSemantic == "" && q.ValueFieldName == "" {
		return true
	}
	return fieldDesignated(field, q.ValueFieldSemantic, q.ValueFieldName, q.valueFieldExact)
}

// fieldDesignated は、欄が語彙の項目 semantic か原資料の key name の指定に一致するかを返す。
// semantic と name のどちらか一方を与える。検索の欄 (searchesField) と数える欄 (countsField) が
// 同じ判定を使う。exact は、name と同じ名前の欄をグラフが持つかである (hasFieldNamed)。
//
// **原資料の key で指したときは、語彙の項目を持つ欄にも当てる。** 語彙へ写した欄を原資料の
// 名前で探す要求と数える要求が、0 件にならないようにする。
func fieldDesignated(field core.RecordField, semantic core.SemanticKey, name string, exact bool) bool {
	if semantic != "" {
		return field.Semantic == semantic
	}
	if exact {
		return field.Name == name
	}
	return name != "" && fieldNamed(field.Name, name)
}

// fieldNamed は、原資料の欄の名前 name が、要求の名前 requested に一致するかを返す。
//
// **要求の名前は欄の名前そのものか、欄の名前を `.` で区切った末尾の部分である。** 入れ子の
// 要素の名前を親の名前と `.` で繋ぐ入力形式 (Windows イベントログの `EventData.NewProcessName`)
// の欄を、原資料に書かれた要素の名前 (`NewProcessName`) で指せる。
//
// **同じ名前の欄をグラフが持つときは、末尾の部分で当てない (withResolvedFieldNames)。**
// `Level` と `RenderingInfo.Level` のように意味の違う欄を 1 つの集計にまとめない。同じ名前の欄が
// 無く、末尾の部分が 2 つ以上の欄に一致するときは、どれも当てる。
func fieldNamed(name, requested string) bool {
	return name == requested || strings.HasSuffix(name, "."+requested)
}

// hasFieldNamed は、名前がちょうど name の欄を持つレコードがあるかを返す。
func (g Graph) hasFieldNamed(name string) bool {
	if name == "" {
		return false
	}
	for observed := range g.fieldNames {
		if observed.name == name {
			return true
		}
	}
	return false
}

// withResolvedFieldNames は、文字列を当てる欄と数える欄の原資料の名前を、同じ名前の欄をグラフが
// 持つときは名前の一致だけで当てる要求に直す。要求を受け取る公開の関数が最初に呼ぶ。
func (g Graph) withResolvedFieldNames(query GraphQuery) GraphQuery {
	query.valueFieldExact = g.hasFieldNamed(query.ValueFieldName)
	query.countByExact = g.hasFieldNamed(query.CountByName)
	// 要求の組の並びを書き換えないよう、複製に決める。
	query.FieldContains = slices.Clone(query.FieldContains)
	for at := range query.FieldContains {
		query.FieldContains[at].exact = g.hasFieldNamed(query.FieldContains[at].Name)
	}
	if query.Expression != nil {
		query.Expression = query.Expression.withExactNames(g.hasFieldNamed).withNamedAccounts(g)
	}
	return query
}

// HasNode はその識別子のノードがグラフにあるかを返す。
func (g Graph) HasNode(id string) bool {
	_, found := g.nodeAt[id]
	return found
}

// IsTerminalNode はその識別子のノードがグラフにあり、端末のノードであるかを返す。
func (g Graph) IsTerminalNode(id string) bool {
	index, found := g.nodeAt[id]
	return found && g.nodes[index].key.Kind == core.NodeKindTerminal
}

// nodeCount と edgeCount はグラフ全体の個数である。
func (g Graph) nodeCount() int { return len(g.nodes) }

func (g Graph) edgeCount() int { return len(g.edges) }

// reach は、絞り込みに合うノードと、広げる段階の要求と、合うノードから辿れるエッジの位置を返す。
// query は withResolvedFieldNames を通した要求である。
func (g Graph) reach(query GraphQuery) (matched []int, step GraphQuery, reachable []int) {
	passes := g.recordPasses(query)
	matched = g.matchedNodes(query, passes)
	step, stepPasses := g.stepOf(query, passes)
	if query.Depth > 0 {
		reachable = g.reachableEdges(matched, step, stepPasses)
	}
	return matched, step, reachable
}

// Query は要求が選んだ部分グラフを返す。
//
// **絞り込みに合うノードの全件から近傍を広げ、全件を返す。** 続きの位置を持たない。
// ノードの数が要求の上限 (GraphQuery.NodeLimit) を超えるときは、合ったノードと、関係の種別ごとの
// エッジの本数を返し、エッジを空にする (Subgraph.NodeLimitExceeded)。
func (g Graph) Query(query GraphQuery) Subgraph {
	query = g.withResolvedFieldNames(query)
	matched, step, reachable := g.reach(query)
	subgraph := Subgraph{
		MatchedNodeCount: len(matched),
		MatchedKinds:     g.matchedKindsOf(matched),
	}
	subgraph.MatchedAccountIdentityPairCount = g.accountIdentityPairsOf(matched)
	subgraph.ValueCounts, subgraph.DistinctValueCount = g.valueCounts(matched, query)
	selection := make(map[int]core.NodeSelection, len(matched)+2*len(reachable))
	for _, index := range matched {
		selection[index] = core.NodeSelectionMatched
	}
	order := g.subgraphNodeOrder(matched, reachable, selection)
	subgraph.SubgraphNodeCount = len(order)
	// **上限を超える図は、合ったノードとエッジの本数だけを返す。** 画面は上限を超えた図を描かないため、
	// 端点とエッジの本体を組むとメモリだけを使う。起点が多いと本体は数百 MB になる。
	if query.NodeLimit > 0 && len(order) > query.NodeLimit {
		subgraph.NodeLimitExceeded = true
		order = order[:len(matched)]
		subgraph.EdgeKindCounts = g.edgeKindCountsOf(reachable)
		reachable = nil
	}
	// 上限を超えない応答だけが、エッジと端点のノードを候補の区分の順に並べる。上限を超えた応答の
	// reachable は空であり、並べ替えない。
	if g.sortByCandidateTier(reachable) {
		order = g.subgraphNodeOrder(matched, reachable, selection)
	}
	subgraph.Nodes = make([]core.SubgraphNode, 0, len(order))
	for _, index := range order {
		subgraph.Nodes = append(subgraph.Nodes, g.subgraphNode(index, selection[index], query))
	}
	subgraph.Edges = make([]core.GraphEdge, 0, len(reachable))
	for _, index := range reachable {
		subgraph.Edges = append(subgraph.Edges, g.subgraphEdge(g.edges[index], step))
	}
	subgraph.EdgeCount = len(subgraph.Edges)
	return subgraph
}

// subgraphNodeOrder は応答のノードの並びを返す。matched を先に置き、reachable のエッジの端点を
// エッジの順に続ける。selection に無い端点は、エッジの端点として selection へ足す。
func (g Graph) subgraphNodeOrder(
	matched, reachable []int, selection map[int]core.NodeSelection,
) []int {
	order := make([]int, 0, len(matched)+2*len(reachable))
	order = append(order, matched...)
	placed := make(map[int]struct{}, cap(order))
	for _, index := range matched {
		placed[index] = struct{}{}
	}
	for _, index := range reachable {
		edge := g.edges[index]
		for _, endpoint := range []int{edge.source, edge.target} {
			if _, present := placed[endpoint]; present {
				continue
			}
			placed[endpoint] = struct{}{}
			if _, selected := selection[endpoint]; !selected {
				selection[endpoint] = core.NodeSelectionEdgeEndpoint
			}
			order = append(order, endpoint)
		}
	}
	return order
}

// edgeKindCountsOf は、reachable のエッジの本数を関係の種別ごとに、種別の昇順で返す。
func (g Graph) edgeKindCountsOf(reachable []int) []core.EdgeKindCount {
	counts := map[core.EdgeKind]int64{}
	for _, index := range reachable {
		counts[g.edges[index].kind]++
	}
	kinds := make([]core.EdgeKindCount, 0, len(counts))
	for _, kind := range slices.Sorted(maps.Keys(counts)) {
		kinds = append(kinds, core.EdgeKindCount{Kind: kind, Count: counts[kind]})
	}
	return kinds
}

// matchedKindsOf はノードの種別ごとの件数を、種別の昇順で返す。
func (g Graph) matchedKindsOf(nodes []int) []core.MatchedKind {
	counts := map[core.NodeKind]int64{}
	for _, node := range nodes {
		counts[g.nodes[node].key.Kind]++
	}
	kinds := make([]core.MatchedKind, 0, len(counts))
	for _, kind := range slices.Sorted(maps.Keys(counts)) {
		kinds = append(kinds, core.MatchedKind{Kind: kind, Count: counts[kind]})
	}
	return kinds
}

// accountIdentityPairsOf は、nodes のアカウントのうち、同じアカウントの候補
// (account_identity_match) で結ばれたノードの組の数を返す。同じ組を結ぶエッジが 2 本以上
// あっても 1 組と数える。
func (g Graph) accountIdentityPairsOf(nodes []int) int64 {
	accounts := make(map[int]bool)
	for _, node := range nodes {
		if g.nodes[node].key.Kind == core.NodeKindAccount {
			accounts[node] = true
		}
	}
	pairs := make(map[[2]int]bool)
	for node := range accounts {
		for _, at := range g.adjacency[node].outgoing {
			edge := g.edges[at]
			if edge.kind == core.EdgeKindAccountIdentityMatch && accounts[edge.target] {
				pairs[[2]int{min(node, edge.target), max(node, edge.target)}] = true
			}
		}
	}
	return int64(len(pairs))
}

// countsField は 1 項目が、数える欄の指定に一致するかを返す。
//
// **語彙の項目と原資料の key のどちらか一方で指す。** 語彙の項目で指した要求は、同じ意味を
// 別の key が持つ収集元の値も 1 つにまとめる。key で指した要求はその key の値だけを数え、
// 語彙へ写した欄も数える (fieldDesignated)。
func (q GraphQuery) countsField(field core.RecordField) bool {
	return fieldDesignated(field, q.CountBySemantic, q.CountByName, q.countByExact)
}

// countsValues は要求が値ごとの件数を求めるかを返す。
func (q GraphQuery) countsValues() bool {
	return q.CountBySemantic != "" || q.CountByName != ""
}

// valueCounts は、絞り込みに合うノードが数える欄に観測した値ごとの件数を返す。
// 2 つ目の戻り値は打ち切りの前に数えた値の異なりの個数である。
//
// **比べられる値を持たない観測を数えない。** 値の不在は属性にならない (addAttribute)。
//
// 既知の制限: 値の不在で数から外したレコードの件数を応答に出さず、**応答からは求められない**,
// 属性は比べられる値を持つ観測だけを持ち (addAttribute)、グラフは欄を組んだ後にレコードの
// 欄を保持しない。件数を出すには、全レコードの欄を保持し続けるか、取り込み結果を二度目に
// 全件走査する必要がある。値ごとの件数の和と合致したレコードの件数の差から求めることも
// できない。1 レコードが同じ欄へ 2 つの異なる値を持つと、そのレコードを 2 つの値が数える
// ためである。markii 形式の 1 レコードは端末の IPv4 と IPv6 を 1 つずつ持つ,
// 値の不在の件数を求める受入条件が出たときに見直す
//
// **数えるのはレコードの異なりである。** 同じ値を対象のノードとレコードのノードの両方が
// 属性に持つため、属性の観測の件数を足し合わせると 1 つの観測を 2 回数える。
//
// **数えるのは絞り込みを通った根拠のレコードだけである。** ノードが絞り込みに合うのは、
// 根拠の 1 件以上が通ればよい (anyRecordMatches)。属性が持つ根拠をそのまま数えると、
// 分類や期間の外のレコードが件数と時刻の両端と根拠に混ざる。エッジの根拠を
// subgraphEdge が filteredEvidence で絞るのと同じ判定である。
//
// **絞り込みを通った根拠を 1 件も持たない値を返さない。** 件数 0 の値を返すと、その値が
// 問いの範囲にあると読める。
//
// 並びは、絞り込みに合うノードを走査して値を最初に見た順である。ノードの並び順が取り込みの
// 入力順とレコードの走査順であるため、同じ取り込みへの同じ要求は同じ並びを返す。
//
// 既知の制限: 集計は絞り込みに合うノードの全属性を走査し、欄から探す索引を持たない,
// 所要は属性の総数に比例し、repo の fixture の大きさでは測れない,
// 画面の既定の要求の応答が 1,000,000 byte または 100 ms を超えたときに見直す
func (g Graph) valueCounts(matched []int, query GraphQuery) ([]core.ValueCount, int) {
	if !query.countsValues() {
		return nil, 0
	}
	order := make([]string, 0)
	records := make(map[string]map[int]struct{})
	for _, index := range matched {
		for _, attribute := range g.nodes[index].attributes {
			if !query.countsField(*attribute.field) {
				continue
			}
			value, readable := comparableFieldValue(*attribute.field)
			if !readable {
				continue
			}
			passed := g.filteredEvidence(attribute.evidence, query)
			if len(passed) == 0 {
				continue
			}
			if _, seen := records[value]; !seen {
				order = append(order, value)
				records[value] = make(map[int]struct{})
			}
			for _, at := range passed {
				records[value][at] = struct{}{}
			}
		}
	}
	counts := make([]core.ValueCount, 0, len(order))
	for _, value := range order {
		counts = append(counts, g.valueCountOf(value, records[value]))
	}
	return counts, len(order)
}

// valueCountOf は 1 つの値の件数と時刻の両端を組む。
//
// **根拠の中身を含めない。** 図は根拠を描かない。根拠は値を選んだときに `/api/v0/records` が返す。
func (g Graph) valueCountOf(
	value string, positions map[int]struct{},
) core.ValueCount {
	evidence := make([]int, 0, len(positions))
	for at := range positions {
		evidence = append(evidence, at)
	}
	slices.Sort(evidence)
	count := core.ValueCount{
		Value:       value,
		RecordCount: int64(len(evidence)),
	}
	if span := g.applicableRange(evidence); span != nil {
		first, last := span.From, span.To
		count.FirstEventTime, count.LastEventTime = &first, &last
	}
	count.Intervals = g.eventIntervals(evidence)
	return count
}

// eventIntervals は、根拠のレコードのうち時点を持つレコードを時刻の順に並べ、隣り合う 2 件の
// 時刻の差の分布を返す。時点を持つレコードが 2 件未満のときは nil である。
//
// 既知の制限: 集計の全部の値について分布を組み、応答に載せる, 値ごとに根拠を並べ替えるため
// 計算は根拠の総数 N に対して O(N log N) であり、応答は値 1 つにつき分布 1 つ分増える,
// 画面の既定の要求の応答が 1,000,000 byte または 100 ms を
// 超えたとき、選んだ値の分布だけを返す要求の項目へ移す
func (g Graph) eventIntervals(evidence []int) *core.EventIntervals {
	instants := make([]int64, 0, len(evidence))
	coarse := false
	for _, at := range evidence {
		record := g.records[at]
		if !record.hasInstant {
			continue
		}
		instants = append(instants, record.instant.UnixMilli())
		// 秒か秒より粗い精度の時刻は、ミリ秒より粗い。
		if record.eventTime != nil && record.eventTime.Precision.IsCoarserThan(core.PrecisionMillisecond) {
			coarse = true
		}
	}
	// 差を 1 つ取るには 2 件のレコードが要る。
	if len(instants) < minTimedRecordsForIntervals {
		return nil
	}
	slices.Sort(instants)
	gaps := make([]int64, 0, len(instants)-1)
	intervals := &core.EventIntervals{
		TimedRecordCount: int64(len(instants)),
		BinCounts:        make([]int64, len(core.IntervalBoundsMilliseconds)+1),
		CoarsePrecision:  coarse,
	}
	for at := 1; at < len(instants); at++ {
		gap := instants[at] - instants[at-1]
		gaps = append(gaps, gap)
		// 階級は [下端, 上端) であり、境界と同じ差は上の階級に入る。
		bin, _ := slices.BinarySearch(core.IntervalBoundsMilliseconds, gap+1)
		intervals.BinCounts[bin]++
	}
	slices.Sort(gaps)
	quantile := func(quarters int) int64 { return gaps[quarters*(len(gaps)-1)/quartersInWhole] }
	intervals.MinMilliseconds, intervals.MaxMilliseconds = gaps[0], gaps[len(gaps)-1]
	intervals.LowerQuartileMilliseconds, intervals.MedianMilliseconds, intervals.UpperQuartileMilliseconds =
		quantile(lowerQuartileQuarters), quantile(medianQuarters), quantile(upperQuartileQuarters)
	return intervals
}

// minTimedRecordsForIntervals は、差の分布を組む時点を持つレコードの最少の件数である。
const minTimedRecordsForIntervals = 2

// 四分位と中央値の位置を、全体を 4 つに分けた数で表す。
const (
	quartersInWhole       = 4
	lowerQuartileQuarters = 1
	medianQuarters        = 2
	upperQuartileQuarters = 3
)

// HasValueMatchAnywhere は、他の絞り込みを外して文字列の条件だけで走査し、条件を満たす属性を
// 持つノードがあるかを返す。対象の粒度ではレコードのノードだけを見て、レコード単位で判定する。
//
// **走査するのはグラフのノードの属性である。** 属性になるのは、読めたレコードの読めた欄
// だけである (addAttribute)。取り込みに失敗したレコードと、値を読めなかった欄は走査に
// 入らない。**偽が返ったことを「収集元のどこにも無い」と読み替えない。**
//
// **0 件の応答の理由を分けるためだけに呼ぶ。** 「グラフに載った欄に文字列が 1 つも無い」と
// 「文字列はあるが他の絞り込みで残らなかった」を、確かめずに名乗らない
// (core.EmptyReasonNoValueMatch)。
//
// 既知の制限: 0 件の応答で、グラフの全ノードの全属性をもう 1 度走査する,
// 走るのは合致が 0 件の応答だけで、1 件以上を返す応答は走査しない。所要は repo の fixture の
// 大きさでは測れない, 0 件の応答が 100 ms を超えたときに見直す
func (g Graph) HasValueMatchAnywhere(query GraphQuery) bool {
	if !query.SearchesText() {
		return false
	}
	query = g.withResolvedFieldNames(query)
	for _, node := range g.nodes {
		// **対象の粒度では、合うかをレコード単位で決めるため、レコードのノードだけを見る。**
		// 対象のノードの属性は複数のレコードから集めたもので、文字列を別々のレコードに持つ
		// 対象を「文字列はある」と読み違える。
		if query.objects() && node.key.Kind != core.NodeKindRecord {
			continue
		}
		if query.textMatches(node) {
			return true
		}
	}
	return false
}

// HasRecordMatchWithoutObject は、対象の粒度の要求で合うノードが 0 件のとき、同じ絞り込みに
// レコードの粒度で合うレコードのノードがあるかを返す。真のとき、合うレコードはあるが、その
// レコードは端末のほかに対象のノードを指さない。
//
// **端末を指すだけのレコードも真に数える。** 事象の種別で絞る要求は、端末を合うノードに数えない
// (matchedNodes)。レコードを記録した端末は、ほぼすべてのレコードが指す。
//
// **0 件の応答の理由を分けるためだけに呼ぶ。** レコードの粒度の要求では偽を返す。
func (g Graph) HasRecordMatchWithoutObject(query GraphQuery) bool {
	if !query.objects() {
		return false
	}
	query = g.withResolvedFieldNames(query)
	// 種別とアドレスの範囲の絞り込みで残らなかった対象を持つ要求は、この理由に該当しない。
	query.NodeKinds = nil
	query.AddressInPrefix = nil
	query.AddressNotInPrefix = nil
	if len(g.matchedNodes(query, g.recordPasses(query))) > 0 {
		return false
	}
	query.Granularity = core.GraphGranularityRecord
	query.NodeKinds = []core.NodeKind{core.NodeKindRecord}
	return len(g.matchedNodes(query, g.recordPasses(query))) > 0
}

// CountedFieldObservation は、他の絞り込みを外して、数える欄をグラフのどこで観測したかである。
type CountedFieldObservation struct {
	// Named は、数える欄の名前を持つレコードが 1 件以上あるかである。値を読めない欄も数える。
	Named bool
	// NodeKinds は、数える欄の読めた値を属性に持つノードの種別である。種別の昇順に並ぶ。
	NodeKinds []core.NodeKind
}

// EmptyReason は、値ごとの件数が 0 件になった要求 query について、観測から理由を決める。
// 2 つ目の戻り値は、理由が core.EmptyReasonFieldOnOtherNodeKind のときに、欄の値を持つ
// ノードの種別である。
//
//   - 欄の名前がどこにも無い: core.EmptyReasonNoFieldObserved
//   - 欄の名前はあるが、読めた値がどこにも無い: core.EmptyReasonNoReadableValue
//   - 欄の値を持つノードの種別が、要求の種別の条件をどれも通らない: core.EmptyReasonFieldOnOtherNodeKind
//   - それ以外 (期間・案件・端末・事象の種別・収集元などの絞り込みで残らない): core.EmptyReasonNoValueInFilter
func (o CountedFieldObservation) EmptyReason(query GraphQuery) (core.EmptyReason, []core.NodeKind) {
	switch {
	case len(o.NodeKinds) == 0 && !o.Named:
		return core.EmptyReasonNoFieldObserved, nil
	case len(o.NodeKinds) == 0:
		return core.EmptyReasonNoReadableValue, nil
	case !slices.ContainsFunc(o.NodeKinds, query.kindMatches):
		return core.EmptyReasonFieldOnOtherNodeKind, o.NodeKinds
	default:
		return core.EmptyReasonNoValueInFilter, nil
	}
}

// CountedFieldObservationOf は、他の絞り込みを外して走査し、数える欄をグラフのどこで観測したかを
// 返す。
//
// **走査するのはグラフのノードの属性と、レコードの欄の名前の集合である。** 属性の走査の範囲は
// HasValueMatchAnywhere と同じで、読めたレコードの読めた欄に限る。
//
// **値ごとの件数が 0 件になった理由を分けるためだけに呼ぶ。** 欄の名前の打ち間違いと、
// 値の不在と、対象の種別の選び方と、絞り込みで残らなかった状態を、確かめずに同じ応答にしない。
//
// 既知の制限: 値の異なりが 0 件の応答で、グラフの全ノードの全属性をもう 1 度走査する,
// **走る条件は合致したノードの件数に依らない。** 検索の文字列も 0 件になる要求では 1 応答で
// 2 回走る。所要は repo の fixture の大きさでは測れない, 0 件の応答が 100 ms を超えたときに見直す
func (g Graph) CountedFieldObservationOf(query GraphQuery) CountedFieldObservation {
	if !query.countsValues() {
		return CountedFieldObservation{}
	}
	query = g.withResolvedFieldNames(query)
	observation := CountedFieldObservation{Named: g.ObservesField(query.CountBySemantic, query.CountByName)}
	for _, node := range g.nodes {
		if slices.Contains(observation.NodeKinds, node.key.Kind) {
			continue
		}
		for _, attribute := range node.attributes {
			if !query.countsField(*attribute.field) {
				continue
			}
			if _, readable := comparableFieldValue(*attribute.field); readable {
				observation.NodeKinds = append(observation.NodeKinds, node.key.Kind)
				break
			}
		}
	}
	slices.Sort(observation.NodeKinds)
	return observation
}

// ObservesField は、欄の指定 (語彙の項目 semantic か原資料の key name) に一致する欄を持つ
// レコードが 1 件以上あるかを返す。値を読めない欄も数える。
func (g Graph) ObservesField(semantic core.SemanticKey, name string) bool {
	exact := g.hasFieldNamed(name)
	for observed := range g.fieldNames {
		if fieldDesignated(core.RecordField{Semantic: observed.semantic, Name: observed.name}, semantic, name, exact) {
			return true
		}
	}
	return false
}

// ObservedFieldNames は、取り込んだレコードが持つ欄の語彙の項目と原資料の key を、それぞれ
// 重複を除いて昇順で返す。値を読めない欄も入る。検索式と欄の条件に書ける名前の一覧である。
func (g Graph) ObservedFieldNames() (semantics []core.SemanticKey, names []string) {
	semantics, names = []core.SemanticKey{}, []string{}
	for observed := range g.fieldNames {
		if observed.semantic != "" {
			semantics = append(semantics, observed.semantic)
		}
		if observed.name != "" {
			names = append(names, observed.name)
		}
	}
	slices.Sort(semantics)
	slices.Sort(names)
	return slices.Compact(semantics), slices.Compact(names)
}

// matchedNodes は絞り込みに合うノードを、グラフの並び順で返す。passes は根拠のレコード 1 件が
// 絞り込みと文字列の条件を通るかを返す (recordPasses)。
func (g Graph) matchedNodes(query GraphQuery, passes func(at int) bool) []int {
	matched := make([]int, 0, len(g.nodes))
	for index, node := range g.nodes {
		if len(query.NodeIds) > 0 && !slices.Contains(query.NodeIds, node.id) {
			continue
		}
		if !query.kindMatches(node.key.Kind) {
			continue
		}
		if !query.addressMatches(node.key) {
			continue
		}
		// 端末は全レコードに指される。根拠のレコードで事象の種別と文字列を判定すると、条件に
		// 合うレコードが 1 件でもある端末がすべて起点に入る。端末は自身の属性で文字列を判定し、
		// 事象の種別の条件を持つ要求では起点に数えない。要求が NodeIds で指した端末は除かない。
		terminal := node.key.Kind == core.NodeKindTerminal
		if terminal && !slices.Contains(query.NodeIds, node.id) &&
			(query.filtersEventKind() || (query.SearchesText() && !query.textMatches(node))) {
			continue
		}
		// 文字列の条件を持つ要求の端末は、どちらの粒度でも下のレコードの粒度の判定 (自身の属性の
		// 文字列と、根拠のレコードの絞り込み) に通す。
		if query.objects() && (!terminal || !query.SearchesText()) {
			// 割当の IP のノードは割当の側で判定する。割当は文字列の条件を満たす欄を持たない。
			if !slices.ContainsFunc(node.evidence, passes) &&
				(query.SearchesText() || !g.assignedAddressMatches(index, query.RecordFilter)) {
				continue
			}
			matched = append(matched, index)
			continue
		}
		if !query.textMatches(node) {
			continue
		}
		if !g.anyRecordMatches(node.evidence, query) &&
			!g.assignedAddressMatches(index, query.RecordFilter) {
			continue
		}
		matched = append(matched, index)
	}
	return matched
}

// recordPasses は、根拠のレコード 1 件が絞り込みと文字列の条件の両方を通るかを返す関数を作る。
//
// **文字列の条件はそのレコードのノードの属性で判定する。** レコードのノードを持たないレコードは、
// 文字列の条件を持つ要求を通らない。判定する欄が無いためである。
//
// 文字列の判定はレコードごとに 1 回だけ行い、結果を覚える。対象のノードの根拠は同じレコードを
// 多く共有する。
func (g Graph) recordPasses(query GraphQuery) func(at int) bool {
	const (
		unknown = iota
		passed
		failed
	)
	var verdicts []uint8
	if query.SearchesText() {
		verdicts = make([]uint8, len(g.records))
	}
	return func(at int) bool {
		if !g.recordMatches(at, query.RecordFilter) {
			return false
		}
		if verdicts == nil {
			return true
		}
		if verdicts[at] == unknown {
			record := g.records[at]
			verdicts[at] = failed
			if record.hasRecordNode && query.textMatches(g.nodes[record.recordNode]) {
				verdicts[at] = passed
			}
		}
		return verdicts[at] == passed
	}
}

// addressMatches はノードがアドレスの範囲の絞り込みを通るかを返す。
//
// **範囲で絞るのは IP アドレスのノードだけである。** 他の種別のノードは比べるアドレスを
// 持たず、範囲を与えた要求でも通る。プロセスを起点に接続先を辿る要求が、起点のプロセスを
// 範囲の外として除かないためである。
//
// **原資料の文字列をアドレスとして読めない IP アドレスのノードは通らない。** 範囲の内と外の
// どちらであるかを決められないためである。
//
// IPv4 の範囲と IPv6 のアドレスを比べた結果は、範囲に入らないである。
func (q GraphQuery) addressMatches(key core.NodeKey) bool {
	if q.AddressInPrefix == nil && q.AddressNotInPrefix == nil {
		return true
	}
	if key.Kind != core.NodeKindIp {
		return true
	}
	address, readable := key.AddressValue()
	if !readable {
		return false
	}
	if q.AddressInPrefix != nil && !q.AddressInPrefix.Contains(address) {
		return false
	}
	if q.AddressNotInPrefix != nil && q.AddressNotInPrefix.Contains(address) {
		return false
	}
	return true
}

// reachableEdges は合致したノードから要求の段数だけ辿って到達したエッジを、グラフの
// 並び順で重複無く返す。
//
// **1 つ前の段階で新しく到達したノードだけを次の段階の起点にする。** 既に起点として広げた
// ノードを再び広げると、同じノードの周りを段数の回数だけ走査する。
//
// 段数の途中で新しいノードへ到達しなくなった要求は、そこで走査を止める。祖先の連鎖を辿る
// 要求は、起動のレコードが無い段階で連鎖が尽き、残りの段数を使わずに終わる。
//
// **起点のノードだけが、レコードが対象を指す関係を端末から辿る。** 端末はその端末の全レコードに
// 指されるため、中継の段階の端末から辿ると全レコードが入る。
//
// query と passes は段階のエッジの判定に当てる要求とその判定であり、呼び出し元が stepOf で作る。
func (g Graph) reachableEdges(nodes []int, query GraphQuery, passes func(at int) bool) []int {
	selected := make(map[int]struct{})
	reached := make(map[int]struct{}, len(nodes))
	frontier := make([]int, 0, len(nodes))
	for _, index := range nodes {
		if _, seen := reached[index]; seen {
			continue
		}
		reached[index] = struct{}{}
		frontier = append(frontier, index)
	}
	for step := 0; step < query.Depth && len(frontier) > 0; step++ {
		next := make([]int, 0, len(frontier))
		for _, index := range frontier {
			next = g.expandNode(index, step == 0, query, passes, selected, reached, next)
		}
		frontier = next
	}
	incident := make([]int, 0, len(selected))
	for at := range selected {
		incident = append(incident, at)
	}
	slices.Sort(incident)
	return incident
}

// expandNode は 1 つのノードに繋がるエッジを選び、新しく到達した端点を next へ足して返す。
//
// **アドレスの範囲を通らない端点へ向かうエッジを選ばない。** 範囲で外した接続先が、
// エッジの端点として応答へ戻ることを避ける。
//
// **対象の粒度で文字列の条件を持つ要求は、文字列の条件を満たすレコードを根拠に持つエッジだけを
// 辿る。** 文字列を持たないレコードが作った関係まで辿ると、端末やアカウントのように関係の
// 多い対象から、問いと関わらない対象が数百件添えられる。親子の連鎖は文字列の条件を持たない
// 要求で辿る。
//
// origin は index が起点のノードであるかである。起点以外の端末は、レコードが対象を指す関係を
// 辿らない (reachableEdges)。
func (g Graph) expandNode(index int, origin bool, query GraphQuery, passes func(at int) bool,
	selected, reached map[int]struct{}, next []int,
) []int {
	followsText := query.objects() && query.SearchesText()
	relayTerminal := !origin && g.nodes[index].key.Kind == core.NodeKindTerminal
	adjacency := g.adjacency[index]
	for _, at := range slices.Concat(adjacency.outgoing, adjacency.incoming) {
		if _, taken := selected[at]; taken {
			continue
		}
		if relayTerminal && g.edges[at].kind == core.EdgeKindRecordNamesObject {
			continue
		}
		if len(query.EdgeKinds) > 0 && !slices.Contains(query.EdgeKinds, g.edges[at].kind) {
			continue
		}
		// **対象の粒度は、種別の絞り込みを通らない端点へ辿らない。** レコードのノードと、
		// レコードが対象を指す関係はここで外れる。
		if query.objects() && (!query.kindMatches(g.nodes[g.edges[at].source].key.Kind) ||
			!query.kindMatches(g.nodes[g.edges[at].target].key.Kind)) {
			continue
		}
		if !query.addressMatches(g.nodes[g.edges[at].source].key) ||
			!query.addressMatches(g.nodes[g.edges[at].target].key) {
			continue
		}
		if query.EndpointRecordsInPeriod && (!g.recordNodeInPeriod(g.edges[at].source, query.RecordFilter) ||
			!g.recordNodeInPeriod(g.edges[at].target, query.RecordFilter)) {
			continue
		}
		if followsText && !slices.ContainsFunc(g.edges[at].evidence, passes) {
			continue
		}
		if !followsText && !g.anyRecordMatches(g.edges[at].evidence, query) &&
			!g.assignmentsMatch(g.edges[at], query.RecordFilter) {
			continue
		}
		selected[at] = struct{}{}
		for _, endpoint := range []int{g.edges[at].source, g.edges[at].target} {
			if _, seen := reached[endpoint]; seen {
				continue
			}
			reached[endpoint] = struct{}{}
			next = append(next, endpoint)
		}
	}
	return next
}

// recordNodeInPeriod は、ノードがレコードのノードでないか、そのレコードが filter の期間の中に
// あるかを返す。期間の他の条件は当てない。レコードのノードの根拠はそのレコードである。
func (g Graph) recordNodeInPeriod(index int, filter RecordFilter) bool {
	if g.nodes[index].key.Kind != core.NodeKindRecord {
		return true
	}
	period := RecordFilter{TimeFrom: filter.TimeFrom, TimeTo: filter.TimeTo, TimeUnit: filter.TimeUnit}
	return slices.ContainsFunc(g.nodes[index].evidence, func(at int) bool {
		return g.recordMatches(at, period)
	})
}

// subgraphNode は 1 つのノードを応答の項目へ直す。
func (g Graph) subgraphNode(
	index int, selection core.NodeSelection, query GraphQuery,
) core.SubgraphNode {
	built := core.SubgraphNode{
		GraphNode: g.graphNode(index),
		Selection: selection,
	}
	if len(query.ValueContains) > 0 {
		built.ValueMatches = valueMatchesOf(g.nodes[index], query.ValueContains, query.searchesField)
	}
	for _, term := range query.FieldContains {
		for _, found := range valueMatchesOf(g.nodes[index], []string{term.Contains}, term.searches) {
			if !slices.Contains(built.ValueMatches, found) {
				built.ValueMatches = append(built.ValueMatches, found)
			}
		}
	}
	if query.Expression != nil {
		built.ValueMatches = query.Expression.collectValueMatches(g.nodes[index], built.ValueMatches)
	}
	built.Terminals = g.namedTerminals(index, selection, query)
	if query.RecordSummary && selection == core.NodeSelectionMatched {
		built.Record = g.recordSummaryOf(index)
	}
	return built
}

// recordSummaryOf は、レコードのノードが指すレコードの位置と時刻と事象の種別を返す。
// レコードのノードでないノードでは nil である。
//
// **レコードのノードの根拠は、そのノードが指すレコード 1 件である (addRecordNode)。**
func (g Graph) recordSummaryOf(index int) *core.RecordSummary {
	node := g.nodes[index]
	if node.key.Kind != core.NodeKindRecord || len(node.evidence) == 0 {
		return nil
	}
	record := g.records[node.evidence[0]]
	summary := &core.RecordSummary{
		RecordRef:     cloneLocator(record.locator),
		EventTime:     cloneTimestampPointer(record.eventTime),
		EventCategory: record.eventCategory,
		EventAction:   record.eventAction,
		WindowsEvent:  record.windowsEventKind,
	}
	for _, attribute := range node.attributes {
		field := attribute.field
		switch {
		case field.Semantic == core.SemanticKeyWindowsEventChannel && summary.Channel == "":
			if channel, readable := comparableFieldValue(*field); readable {
				summary.Channel = channel
			}
		// レコードの番号は原資料の文字列で渡す。
		case field.Semantic == core.SemanticKeyWindowsEventRecordId && summary.EventRecordId == "" &&
			field.Text != nil && field.Text.RawText != nil:
			summary.EventRecordId = *field.Text.RawText
		case g.namesRecordHeaderId(field.Name) && summary.RecordHeaderId == "" &&
			field.Text != nil && field.Text.RawText != nil:
			summary.RecordHeaderId = *field.Text.RawText
		}
	}
	return summary
}

// namesRecordHeaderId は、欄の名前がレコードの見出しの番号の欄の名前であるかを返す。
func (g Graph) namesRecordHeaderId(name string) bool {
	_, found := g.recordHeaderIdNames[name]
	return found
}

// namedTerminals は、ノードの根拠のレコードが名乗った端末を、根拠の並び順で返す。
//
// **母集団は Selection ごとに違う。** matched のノードは絞り込みを通った根拠だけを
// 見る。ノードが絞り込みに合うのは根拠の 1 件以上が通ればよく (anyRecordMatches)、
// 根拠をそのまま読むと分類や期間の外のレコードが名乗った端末が混ざる。
// edge_endpoint のノードは、そのノード自身の根拠を絞り込みで判定していない
// (expandNode はエッジの根拠で判定する) ため、根拠の全件を見る。
func (g Graph) namedTerminals(
	index int, selection core.NodeSelection, query GraphQuery,
) []core.GraphNode {
	evidence := g.nodes[index].evidence
	if selection == core.NodeSelectionMatched {
		evidence = g.filteredEvidence(evidence, query)
	}
	var terminals []core.GraphNode
	seen := make(map[string]struct{})
	for _, at := range evidence {
		id := g.records[at].terminalNodeId
		if id == "" {
			continue
		}
		if _, taken := seen[id]; taken {
			continue
		}
		terminal, found := g.nodeAt[id]
		if !found {
			continue
		}
		seen[id] = struct{}{}
		terminals = append(terminals, g.graphNode(terminal))
	}
	return terminals
}

// graphNode は 1 つのノードを、どの操作にも共通の項目へ直す。
func (g Graph) graphNode(index int) core.GraphNode {
	node := g.nodes[index]
	accountName, accountNameWithheld := g.accountNameFields(index)
	return core.GraphNode{
		AccountName:         accountName,
		AccountNameWithheld: accountNameWithheld,
		Id:                  node.id,
		Kind:                node.key.Kind,
		KeyForm:             node.key.Form,
		Identity:            node.key.Identity(),
		Label:               cloneRawAndNormalized(node.label),
		Observation:         node.observation,
		CreationRecord:      creationRecordOf(node.key.Kind, len(node.creationRecords)),
		OnUnknownTerminal:   g.onUnknownTerminal(node.key),
		IntervalStart:       g.intervalStartOf(node),
	}
}

// intervalStartOf は、PID の区間のノードの区間を開いたレコードの時刻を返す。区間を開いた
// レコードは、時刻の正規化値が鍵の最後の値と一致する根拠のレコードである
// (processInstanceOf)。ほかの形のノードと、そのレコードを持たないノードでは nil を返す。
func (g Graph) intervalStartOf(node graphNode) *core.Timestamp {
	if node.key.Form != core.NodeKeyFormTerminalProcessInterval || len(node.key.Values) == 0 {
		return nil
	}
	start := node.key.Values[len(node.key.Values)-1].Value
	for _, at := range node.evidence {
		eventTime := g.records[at].eventTime
		if eventTime == nil {
			continue
		}
		if normalized, ok := eventTime.NormalizedValue(); ok && normalized == start {
			return cloneTimestampPointer(eventTime)
		}
	}
	return nil
}

// onUnknownTerminal は、ファイルのノードの鍵 key が、名前の分からない端末 (収集元を記録した端末) の
// 鍵の値で始まるかを返す。ファイルの鍵は端末の鍵の値と path を並べる (core.NewRecordGraph)。
func (g Graph) onUnknownTerminal(key core.NodeKey) bool {
	if key.Kind != core.NodeKindFile || len(key.Values) < 2 {
		return false
	}
	// 端末の鍵の値は、収集元の内容の識別 1 つか、それとホスト名の組である。
	terminal := key.Values[:len(key.Values)-1]
	unknown, built := core.RecordingTerminalNodeKey(terminal[0].Value)
	if len(terminal) > 1 {
		unknown, built = core.RecordingHostTerminalNodeKey(terminal[0].Value, terminal[1].Value)
	}
	if !built {
		return false
	}
	_, present := g.nodeAt[nodeIdOf(unknown)]
	return present
}

// valueMatchesOf は、ノードの属性のうち検索の文字列を含む値を持つ欄を、属性の並び順で返す。
//
// **同じ欄の原資料の文字列と正規化値の両方に一致したときは、2 件を返す。** 分析者が原文の
// どこを読むかが 2 つで異なる。
//
// **同じ欄に一致した値が 2 件以上あっても、欄ごとに 1 件へまとめる。** 応答が含むのは一致した
// 欄であり、一致した値の個数ではない。値は `/api/v0/nodes/{id}` がノードの属性として返す。
//
// 既知の制限: 検索は属性の値だけを比べる,
// 識別鍵が持つのは比べられる値 1 つで、原資料の文字列と正規化値のどちらであるかを保っていないため、
// 一致した値の形を持つ契約 (core.NodeValueMatch.Form) を識別鍵では満たせない。同じ値は
// レコードのノードが属性として持つため、ファイル名を検索すると、その path を識別鍵に持つ
// ファイルのノードが、file.path で一致したレコードのノードから段数 1 で返る, 識別鍵が原資料の文字列と正規化値を分けて
// 持つようになったときに見直す
func valueMatchesOf(
	node graphNode, contains []string, searched func(core.RecordField) bool,
) []core.NodeValueMatch {
	matches := make([]core.NodeValueMatch, 0, len(node.attributes))
	for _, attribute := range node.attributes {
		if !searched(*attribute.field) {
			continue
		}
		for _, form := range matchedFormsOfAny(*attribute.field, contains) {
			matches = appendValueMatch(matches, *attribute.field, form)
		}
	}
	return matches
}

// appendValueMatch は、欄 field が形 form で一致したことを matches へ足す。同じ欄と形の一致を
// 既に持つときは足さず、先に一致した値を残す。
func appendValueMatch(
	matches []core.NodeValueMatch, field core.RecordField, form core.ValueMatchForm,
) []core.NodeValueMatch {
	found := core.NodeValueMatch{Form: form}
	if field.Semantic != "" {
		found.Semantic = field.Semantic
	} else {
		found.Name = field.Name
	}
	if slices.ContainsFunc(matches, func(match core.NodeValueMatch) bool {
		return match.Semantic == found.Semantic && match.Name == found.Name && match.Form == form
	}) {
		return matches
	}
	if form == core.ValueMatchFormRawText {
		found.Value, _ = fieldRawText(field)
	} else {
		found.Value, _ = fieldNormalized(field)
	}
	return append(matches, found)
}

// hasValueMatch は、ノードの属性のどれかが検索の文字列を含むかを返す。
//
// **絞り込みの判定に valueMatchesOf を使わない。** 判定はグラフの全ノードを対象にし、
// ノードごとに一致した欄の集合を組むと、応答に入れない大多数のノードの分まで
// 割り当てが起きる。一致した欄を組むのは応答へ入れるノードだけである。
//
// 既知の制限: 検索は全ノードの全属性を走査し、値から探す索引を持たない,
// 所要は属性の総数に比例し、repo の fixture の大きさでは測れない,
// 画面の既定の要求の応答が 1,000,000 byte または 100 ms を超えたときに見直す
func hasValueMatch(node graphNode, contains string, searched func(core.RecordField) bool) bool {
	for _, attribute := range node.attributes {
		if searched(*attribute.field) && len(matchedFormsOf(*attribute.field, contains)) > 0 {
			return true
		}
	}
	return false
}

// matchedFormsOfAny は 1 項目の原資料の文字列と正規化値のうち、検索の文字列のどれかを含むものの
// 形を返す。同じ形を 2 回返さない。
func matchedFormsOfAny(field core.RecordField, contains []string) []core.ValueMatchForm {
	var forms []core.ValueMatchForm
	for _, token := range contains {
		for _, form := range matchedFormsOf(field, token) {
			if !slices.Contains(forms, form) {
				forms = append(forms, form)
			}
		}
	}
	return forms
}

// matchedFormsOf は 1 項目の原資料の文字列と正規化値のうち、検索の文字列を含むものの形を返す。
//
// **大文字と小文字を区別せずに比べる。** 同じ path を収集元が `System32` と `system32` の
// 両方で記録するため、区別すると同じ対象の値の一部にしか一致しない。
func matchedFormsOf(field core.RecordField, contains string) []core.ValueMatchForm {
	var forms []core.ValueMatchForm
	if rawText, present := fieldRawText(field); present && containsFold(rawText, contains) {
		forms = append(forms, core.ValueMatchFormRawText)
	}
	normalized, present := fieldNormalized(field)
	if present && containsFold(normalized, contains) {
		forms = append(forms, core.ValueMatchFormNormalized)
	}
	return forms
}

// containsFold は、大文字と小文字を区別せずに text が token を含むかを返す。
//
// **ASCII だけの検索の文字列は、値を小文字にした複製を作らずに byte で比べ、ASCII の大文字と
// 小文字だけをそろえる。** 検索はグラフの全ノードの全属性を対象にし、値ごとに複製を作ると
// 属性の数だけ割り当てが起きる。
// 値の中の ASCII の外の文字は、ASCII の文字列のどの byte とも一致しない。ASCII の外の文字を
// 含む検索の文字列は、2 つを strings.ToLower で小文字にして比べる。
//
// **UTF-8 として読めない byte を持つ文字列と、U+FFFD を持つ文字列を読めない byte を持つ値に当てる
// ときは、大小をそろえずにそのまま比べる。** strings.ToLower は読めない byte を U+FFFD に
// 置き換え、別の byte どうしを一致させる。U+FFFD を持たない読める文字列は、値の中の置き換えた
// U+FFFD と一致しないため、値に読めない byte があっても大小をそろえて比べる。
func containsFold(text, token string) bool {
	if !isASCII(token) {
		if !utf8.ValidString(token) ||
			(strings.ContainsRune(token, utf8.RuneError) && !utf8.ValidString(text)) {
			return strings.Contains(text, token)
		}
		return strings.Contains(strings.ToLower(text), strings.ToLower(token))
	}
	if token == "" {
		return true
	}
	first := asciiLower(token[0])
	for start := 0; start+len(token) <= len(text); start++ {
		if asciiLower(text[start]) == first && asciiEqualFold(text[start+1:start+len(token)], token[1:]) {
			return true
		}
	}
	return false
}

// isASCII は文字列が ASCII の文字だけを持つかを返す。
func isASCII(text string) bool {
	for index := 0; index < len(text); index++ {
		if text[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// asciiEqualFold は、同じ長さの ASCII の 2 つの文字列が大文字と小文字を除いて等しいかを返す。
func asciiEqualFold(left, right string) bool {
	for index := 0; index < len(left); index++ {
		if asciiLower(left[index]) != asciiLower(right[index]) {
			return false
		}
	}
	return true
}

// asciiLower は ASCII の大文字を小文字にする。
func asciiLower(character byte) byte {
	if 'A' <= character && character <= 'Z' {
		return character + 'a' - 'A'
	}
	return character
}

// fieldRawText と fieldNormalized は、値が時刻の項目とそれ以外の項目の両方から値を取る。
func fieldRawText(field core.RecordField) (string, bool) {
	if field.Text != nil {
		return field.Text.RawTextValue()
	}
	if field.Timestamp != nil {
		return field.Timestamp.RawTextValue()
	}
	return "", false
}

func fieldNormalized(field core.RecordField) (string, bool) {
	if field.Text != nil {
		return field.Text.NormalizedValue()
	}
	if field.Timestamp != nil {
		return field.Timestamp.NormalizedValue()
	}
	return "", false
}

// subgraphEdge は 1 本のエッジを応答の項目へ直す。根拠は絞り込みを通った要素だけを数える。
//
// **根拠の中身を含めない。** 図は根拠を描かない。根拠はエッジを選んだときに
// `/api/v0/edges/{id}` が返す。件数と時刻の両端は、図が関係の多さと時間帯を表すのに使うため含める。
func (g Graph) subgraphEdge(edge graphEdge, query GraphQuery) core.GraphEdge {
	return g.responseEdge(edge, g.filteredEvidence(edge.evidence, query))
}

// responseEdge は 1 本のエッジを、根拠のレコード evidence から応答の項目へ直す。
//
// **根拠のレコードを 1 件も持たない割当のエッジは、割当の適用期間を期間にする。** 取り込みの
// 起動で指定した割当は根拠のレコードを持たず、適用期間はその収集元の観測期間である。
func (g Graph) responseEdge(edge graphEdge, evidence []int) core.GraphEdge {
	assignments := edge.terminalAssignmentList()
	applicable := g.applicableRange(evidence)
	if applicable == nil && len(edge.evidence) == 0 {
		applicable = assignmentsRange(g.interpretedAssignments(assignments))
	}
	// registry のリースから作ったエッジは、リースの期間を期間にする。
	if edge.basis != nil && edge.basis.lease != nil && len(evidence) > 0 {
		applicable = &core.TimeRange{From: cloneTimestamp(edge.basis.lease.From), To: cloneTimestamp(edge.basis.lease.To)}
	}
	var origins []core.TerminalAssignmentOrigin
	for _, assignment := range assignments {
		if !slices.Contains(origins, assignment.Origin) {
			origins = append(origins, assignment.Origin)
		}
	}
	response := core.GraphEdge{
		Id:                              edge.id,
		Kind:                            edge.kind,
		State:                           edge.state,
		SourceNodeId:                    g.nodes[edge.source].id,
		TargetNodeId:                    g.nodes[edge.target].id,
		ApplicableRange:                 applicable,
		AssignmentOrigins:               origins,
		EvidenceCount:                   int64(len(evidence)),
		EvidenceByCase:                  g.evidenceByCase(evidence),
		IndistinguishableCandidateCount: int64(edge.indistinguishableCount()),
	}
	if tier, conditions, tiered := candidateTierOf(edge); tiered {
		response.CandidateTier = &core.EdgeCandidateTier{Tier: tier + 1, Conditions: conditions}
	}
	return response
}

// interpretedAssignments は、地方時の期間を持つ割当の期間の両端へ、期間を読み取った収集元の
// 時刻の解釈を与えた複製を返す。応答の期間から、時刻を読んだずれとその出どころを読める。
// 解釈を持たない収集元の割当と、ずれを与えられない期間は変えない。
func (g Graph) interpretedAssignments(assignments []core.TerminalAssignment) []core.TerminalAssignment {
	interpreted := slices.Clone(assignments)
	for index, assignment := range interpreted {
		interpreted[index].AssignmentValidRange = g.interpretedValidRange(assignment)
	}
	return interpreted
}

// interpretedValidRange は、割当の適用期間の両端へ、期間を読み取った収集元の時刻の解釈を与えた
// 期間を返す。解釈を持たない収集元の割当と、ずれを与えられない期間は、適用期間をそのまま返す。
func (g Graph) interpretedValidRange(assignment core.TerminalAssignment) core.TimeRange {
	interpretation, found := g.sourceInterpretations[assignment.SourceId]
	if !found {
		return assignment.AssignmentValidRange
	}
	if read, ok := interpretedRangeOf(&assignment.AssignmentValidRange, interpretation); ok {
		return read
	}
	return assignment.AssignmentValidRange
}

// assignmentsRange は割当の適用期間を合わせた両端を返す。割当が無いときは nil を返す。
//
// **時点として読めない期間 (UTC からのずれを持たない収集元の観測期間) は、どの割当も同じ文字列の
// 期間を持つときだけ、その文字列のまま返す。** 読めない期間は他の期間と比べられないため、読める
// 期間と混ざるときと、読めない期間どうしが違うときは nil を返す。
func assignmentsRange(assignments []core.TerminalAssignment) *core.TimeRange {
	if len(assignments) == 0 {
		return nil
	}
	if joined, readable := joinedAssignmentsRange(assignments); readable {
		return joined
	}
	first := assignments[0].AssignmentValidRange
	for _, assignment := range assignments[1:] {
		if !reflect.DeepEqual(assignment.AssignmentValidRange, first) {
			return nil
		}
	}
	return &core.TimeRange{From: cloneTimestamp(first.From), To: cloneTimestamp(first.To)}
}

// joinedAssignmentsRange は、割当の適用期間を合わせた両端を返す。readable が偽になるのは、
// 両端を時点として読めない割当が 1 件以上あるときである。
func joinedAssignmentsRange(assignments []core.TerminalAssignment) (*core.TimeRange, bool) {
	var joined *core.TimeRange
	var firstAt, lastAt time.Time
	for _, assignment := range assignments {
		valid := assignment.AssignmentValidRange
		from, fromRead := valid.From.Instant()
		to, toRead := valid.To.Instant()
		if !fromRead || !toRead {
			return nil, false
		}
		if joined == nil {
			joined = &core.TimeRange{From: cloneTimestamp(valid.From), To: cloneTimestamp(valid.To)}
			firstAt, lastAt = from, to
			continue
		}
		if from.Before(firstAt) {
			joined.From, firstAt = cloneTimestamp(valid.From), from
		}
		if to.After(lastAt) {
			joined.To, lastAt = cloneTimestamp(valid.To), to
		}
	}
	return joined, true
}

// evidenceItems は根拠のレコードを応答の項目へ直す。
func (g Graph) evidenceItems(positions []int) []core.GraphEvidence {
	items := make([]core.GraphEvidence, 0, len(positions))
	for _, at := range positions {
		record := g.records[at]
		items = append(items, core.GraphEvidence{
			RecordRef: cloneLocator(record.locator),
			EventTime: cloneTimestampPointer(record.eventTime),
			ObservationKind: core.ObservationKind{
				Raw:     cloneRecordFields(record.observationKind.Raw),
				Status:  record.observationKind.Status,
				Meaning: record.observationKind.Meaning,
			},
			EventKind: record.eventKindPair(),
		})
	}
	return items
}

// applicableRange は根拠のレコードの時刻の両端を返す。
//
// **絶対時刻として読める根拠を 1 件も持たないエッジでは値を返さない。** 時刻を読めない
// 状態を既定の日付で埋めない。
func (g Graph) applicableRange(evidence []int) *core.TimeRange {
	var first, last *core.Timestamp
	var firstAt, lastAt time.Time
	for _, at := range evidence {
		record := g.records[at]
		if !record.hasInstant || record.eventTime == nil {
			continue
		}
		if first == nil || record.instant.Before(firstAt) {
			first, firstAt = record.eventTime, record.instant
		}
		if last == nil || record.instant.After(lastAt) {
			last, lastAt = record.eventTime, record.instant
		}
	}
	if first == nil || last == nil {
		return nil
	}
	return &core.TimeRange{From: cloneTimestamp(*first), To: cloneTimestamp(*last)}
}

// filteredEvidence は絞り込みを通った根拠のレコードを、走査の順で返す。
func (g Graph) filteredEvidence(evidence []int, query GraphQuery) []int {
	if !query.filtersRecords() {
		return evidence
	}
	passed := make([]int, 0, len(evidence))
	for _, at := range evidence {
		if g.recordMatches(at, query.RecordFilter) {
			passed = append(passed, at)
		}
	}
	return passed
}

// anyRecordMatches は根拠のレコードの 1 件以上が絞り込みを通るかを返す。
func (g Graph) anyRecordMatches(evidence []int, query GraphQuery) bool {
	if !query.filtersRecords() {
		return true
	}
	for _, at := range evidence {
		if g.recordMatches(at, query.RecordFilter) {
			return true
		}
	}
	return false
}

// recordMatches は根拠のレコード 1 件が絞り込みを通るかを返す。
//
// **比較に用いる時刻を持たないレコードは、期間を与えた要求の結果に入らない。** 時計の
// ずれを補った時刻を作らないため、期間の中にあるかを決められない。
func (g Graph) recordMatches(at int, query RecordFilter) bool {
	record := g.records[at]
	if query.EventCategory != "" && record.eventCategory != query.EventCategory {
		return false
	}
	if query.EventAction != "" && record.eventAction != query.EventAction {
		return false
	}
	if query.filtersEventActionRange() && !query.eventActionInRange(record.eventAction) {
		return false
	}
	if query.Case != "" && g.caseOfRecord(at) != query.Case {
		return false
	}
	if query.Terminal != "" && record.placedTerminalNodeId != query.Terminal {
		return false
	}
	if len(query.Sources) > 0 && !slices.Contains(query.Sources, record.locator.SourceId) {
		return false
	}
	if query.TimeFrom == nil && query.TimeTo == nil {
		return true
	}
	return record.hasInstant && query.instantInPeriod(record.instant)
}
