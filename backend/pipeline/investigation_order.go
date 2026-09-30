package pipeline

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 調べる順序の目安の手法の名前。要求の path に書く文字列と同じである。
const (
	// InvestigationOrderRecordCountDescending は、対象を指すレコードの件数の多い順である。
	InvestigationOrderRecordCountDescending = "record_count_descending"
	// InvestigationOrderRecordCountAscending は、対象を指すレコードの件数の少ない順である。
	InvestigationOrderRecordCountAscending = "record_count_ascending"
)

// InvestigationOrder は、部分グラフの対象を 1 つの手法の値で並べた、調べる順序の目安である。
//
// **関係の状態 (observed / candidate) と別の属性である。** 目安は対象を見る順を示すだけで、
// 関係の成立や対象の性質を判定しない。
type InvestigationOrder struct {
	// Method は並べた手法の名前である。
	Method string
	// Parameters は手法が使った係数である。係数を持たない手法では空である。
	Parameters []InvestigationOrderParameter
	// Inputs は計算に使った対象・組・レコードの件数と期間である。
	Inputs InvestigationOrderInputs
	// Kinds は対象の種別ごとの並びである。種別の文字列の昇順に並ぶ。
	Kinds []InvestigationOrderKind
}

// InvestigationOrderParameter は手法の係数 1 つの名前と値の文字列である。
type InvestigationOrderParameter struct {
	Name  string
	Value string
}

// InvestigationOrderInputs は、手法が値を計算した入力の大きさである。
type InvestigationOrderInputs struct {
	// ObjectCount は並べた対象の数である。
	ObjectCount int
	// PairCount は対象の組の数である (orderInput.pairs)。
	PairCount int
	// RecordCount は、対象を指すレコードの異なり数である。
	RecordCount int
	// TimedRecordCount は、そのうち時点を持つレコードの数である。
	TimedRecordCount int
	// TimeRange は、時点を持つレコードの時刻の両端である。時点を持つレコードが無いときは nil である。
	TimeRange *core.TimeRange
}

// InvestigationOrderKind は 1 つの種別の対象の並びである。
type InvestigationOrderKind struct {
	Kind    core.NodeKind
	Entries []InvestigationOrderEntry
}

// InvestigationOrderEntry は並びの 1 行である。
type InvestigationOrderEntry struct {
	Node core.GraphNode
	// Value は手法の値である。手法が値を決められない対象 (時点を持つレコードが無い対象など)
	// では nil である。
	Value *float64
	// Rank は、その対象より上に並ぶ値を持つ対象の数に 1 を足した順位である。同じ値の対象は
	// 同じ順位になる。値を持たない対象は、値を持つ対象のすべてより下の 1 つの順位になる。
	Rank int
	// TieCount は、その対象と同じ順位の対象の数である。
	TieCount int
}

// orderInput は手法が値を計算する材料である。
type orderInput struct {
	// objects は並べる対象の g.nodes での位置である。グラフの並び順に並ぶ。
	objects []int
	// records は objects と同じ並びで、対象を指すレコードの g.records での位置である。
	// 絞り込みを通ったものだけを、昇順に重複無く持つ。
	records [][]int
	// pairs は対象の組である。
	pairs []orderPair
	// sigma は起動時に Sigma のルールを当てた結果である。ルールを渡していない起動では空である。
	sigma SigmaEvaluation
	// sigmaMinLevel は、Sigma の手法が一致を数えるルールのレベルの下限である。空は全レベルを数える。
	sigmaMinLevel string
}

// orderPair は、直接の関係でつながるか、同じレコードに指された 2 つの対象である。
//
// **関係の向きを持たない。** 同じレコードに指された 2 つの対象の間には向きが無い。
type orderPair struct {
	// a と b は orderInput.objects での位置である。a < b である。
	a, b int
	// records は組の根拠の g.records での位置である。直接の関係の根拠と、2 つを指した
	// レコードを合わせ、昇順に重複無く持つ。根拠を持たない関係 (分析者の割当から作った関係)
	// だけの組では空である。
	records []int
}

// orderValue は手法が 1 つの対象に付けた値である。present が偽の対象は値を持たない。
type orderValue struct {
	value   float64
	present bool
}

// orderMethod は手法 1 つである。
type orderMethod struct {
	parameters []InvestigationOrderParameter
	// inputParameters は入力から決まる係数を返す。返した係数は parameters の後ろに続く。
	// 係数が入力に依らない手法では nil である。
	inputParameters func(input orderInput) []InvestigationOrderParameter
	// descending は値の大きい対象を上に並べるかである。
	descending bool
	// values は input.objects と同じ並びで値を返す。
	values func(g Graph, input orderInput) []orderValue
}

// orderMethods は手法の名前から手法を探す表である。
var orderMethods = map[string]orderMethod{
	InvestigationOrderRecordCountDescending: {descending: true, values: recordCountValues},
	InvestigationOrderRecordCountAscending:  {descending: false, values: recordCountValues},
	InvestigationOrderDegree:                {descending: true, values: degreeValues},
	InvestigationOrderSigmaMatches: {
		descending: true, values: sigmaMatchValues, inputParameters: sigmaMatchParameters,
	},
}

// recordCountValues は対象を指すレコードの件数を値にする。
func recordCountValues(_ Graph, input orderInput) []orderValue {
	values := make([]orderValue, len(input.objects))
	for i, records := range input.records {
		values[i] = orderValue{value: float64(len(records)), present: true}
	}
	return values
}

// InvestigationOrder は要求の部分グラフの対象を、method の手法の値で種別ごとに並べる。
// ok が偽になるのは、method が知らない手法の名前であるときである。
//
// **計算するグラフは、要求の表示の条件に依らない。** 粒度はレコード、上限を置かない、ノードと
// 関係の種別で絞らない。要求から使うのは、レコードの絞り込みと文字列の条件と起点と段数だけである。
// 表示の条件で母集団が変わると、同じ対象の順位が画面の操作で変わる。
//
// 起点を与えない要求は段数 1 で広げる。絞り込みを通る全ノードとその間の関係が入る。
//
// 文字列の条件は、対象を指すレコードもフィルタする (recordPasses)。レコードのノードを持たない
// レコードと、分析者の割当だけから作った IP のノードは、文字列の条件を与えた要求の対象に入らない。
// 判定する欄を持たないためである。
//
// sigmaMinLevel は Sigma の手法が一致を数えるルールのレベルの下限で、空は全レベルを数える。
// 呼び出し元は KnownSigmaLevel で確かめた文字列を渡す。ほかの手法は sigmaMinLevel を使わない。
func (g Graph) InvestigationOrder(
	query GraphQuery, method string, sigma SigmaEvaluation, sigmaMinLevel string,
) (InvestigationOrder, bool) {
	chosen, known := orderMethods[method]
	if !known {
		return InvestigationOrder{}, false
	}
	scoped := GraphQuery{
		NodeIds: query.NodeIds, Depth: query.Depth, RecordFilter: query.RecordFilter,
		ValueContains: query.ValueContains, ValueExcludes: query.ValueExcludes,
		ValueFieldSemantic: query.ValueFieldSemantic, ValueFieldName: query.ValueFieldName,
		FieldContains: query.FieldContains, Expression: query.Expression,
		ConditionsOnOriginsOnly: query.ConditionsOnOriginsOnly,
	}
	if len(scoped.NodeIds) == 0 {
		scoped.Depth = 1
	}
	input := g.orderInputOf(scoped)
	input.sigma, input.sigmaMinLevel = sigma, sigmaMinLevel
	values := chosen.values(g, input)
	parameters := slices.Clone(chosen.parameters)
	if chosen.inputParameters != nil {
		parameters = append(parameters, chosen.inputParameters(input)...)
	}
	return InvestigationOrder{
		Method:     method,
		Parameters: parameters,
		Inputs:     g.orderInputsSummary(input),
		Kinds:      g.rankedKinds(input, values, chosen.descending),
	}, true
}

// orderInputOf は部分グラフの対象と、対象の組と、対象を指すレコードを集める。
func (g Graph) orderInputOf(query GraphQuery) orderInput {
	query = g.withResolvedFieldNames(query)
	passes := g.recordPasses(query)
	matched := g.matchedNodes(query, passes)
	step, stepPasses := g.stepOf(query, passes)
	var edges []int
	if query.Depth > 0 {
		edges = g.reachableEdges(matched, step, stepPasses)
	}
	// 広げる段階で届いた対象の根拠は、その段階の判定と同じ要求で絞る。
	isMatched := make(map[int]bool, len(matched))
	for _, index := range matched {
		isMatched[index] = true
	}
	nodes := slices.Clone(matched)
	for _, at := range edges {
		nodes = append(nodes, g.edges[at].source, g.edges[at].target)
	}
	slices.Sort(nodes)
	nodes = slices.Compact(nodes)

	var input orderInput
	position := make(map[int]int, len(nodes))
	for _, index := range nodes {
		if g.nodes[index].key.Kind == core.NodeKindRecord {
			continue
		}
		position[index] = len(input.objects)
		input.objects = append(input.objects, index)
		keep := stepPasses
		if isMatched[index] {
			keep = passes
		}
		records := slices.DeleteFunc(slices.Clone(g.nodes[index].evidence), func(at int) bool { return !keep(at) })
		slices.Sort(records)
		input.records = append(input.records, slices.Compact(records))
	}
	input.pairs = g.orderPairsOf(step, edges, position, input.records)
	return input
}

// orderPairsOf は、edges の関係でつながる対象と、同じレコードに指された対象から組を作る。
// position は g.nodes での位置から objects での位置を求め、records は対象を指すレコードである。
func (g Graph) orderPairsOf(query GraphQuery, edges []int, position map[int]int, records [][]int) []orderPair {
	var pairs []orderPair
	pairAt := make(map[[2]int]int)
	addPair := func(a, b int, evidence []int) {
		if a > b {
			a, b = b, a
		}
		key := [2]int{a, b}
		at, found := pairAt[key]
		if !found {
			at = len(pairs)
			pairAt[key] = at
			pairs = append(pairs, orderPair{a: a, b: b})
		}
		pairs[at].records = append(pairs[at].records, evidence...)
	}
	for _, at := range edges {
		source, sourceIsObject := position[g.edges[at].source]
		target, targetIsObject := position[g.edges[at].target]
		if !sourceIsObject || !targetIsObject || source == target {
			continue
		}
		addPair(source, target, g.filteredEvidence(g.edges[at].evidence, query))
	}
	// namedBy はレコードから、そのレコードが指す対象の objects での位置を求める。位置は昇順に並ぶ。
	namedBy := make(map[int][]int)
	for i, named := range records {
		for _, at := range named {
			namedBy[at] = append(namedBy[at], i)
		}
	}
	for at, named := range namedBy {
		for i := range named {
			for j := i + 1; j < len(named); j++ {
				addPair(named[i], named[j], []int{at})
			}
		}
	}
	slices.SortFunc(pairs, func(left, right orderPair) int {
		return cmp.Or(cmp.Compare(left.a, right.a), cmp.Compare(left.b, right.b))
	})
	for i := range pairs {
		slices.Sort(pairs[i].records)
		pairs[i].records = slices.Compact(pairs[i].records)
	}
	return pairs
}

// orderInputsSummary は計算に使った入力の大きさと期間を数える。
func (g Graph) orderInputsSummary(input orderInput) InvestigationOrderInputs {
	summary := InvestigationOrderInputs{ObjectCount: len(input.objects), PairCount: len(input.pairs)}
	seen := make(map[int]struct{})
	var first, last *core.Timestamp
	var firstAt, lastAt time.Time
	for _, records := range input.records {
		for _, at := range records {
			if _, counted := seen[at]; counted {
				continue
			}
			seen[at] = struct{}{}
			record := g.records[at]
			if !record.hasInstant || record.eventTime == nil {
				continue
			}
			summary.TimedRecordCount++
			if first == nil || record.instant.Before(firstAt) {
				first, firstAt = record.eventTime, record.instant
			}
			if last == nil || record.instant.After(lastAt) {
				last, lastAt = record.eventTime, record.instant
			}
		}
	}
	summary.RecordCount = len(seen)
	if first != nil && last != nil {
		summary.TimeRange = &core.TimeRange{From: cloneTimestamp(*first), To: cloneTimestamp(*last)}
	}
	return summary
}

// rankedKinds は対象を種別ごとに値で並べ、順位と同じ順位の数を付ける。
//
// **同じ値の中はグラフの並び順 (取り込みの順) に並べる。** 順位と同じ順位の数が、並びの中の
// 位置に依らない量である。
func (g Graph) rankedKinds(
	input orderInput, values []orderValue, descending bool,
) []InvestigationOrderKind {
	byKind := make(map[core.NodeKind][]int)
	for i, index := range input.objects {
		kind := g.nodes[index].key.Kind
		byKind[kind] = append(byKind[kind], i)
	}
	kinds := make([]InvestigationOrderKind, 0, len(byKind))
	for kind, members := range byKind {
		slices.SortStableFunc(members, func(left, right int) int {
			return compareOrderValues(values[left], values[right], descending)
		})
		entries := make([]InvestigationOrderEntry, len(members))
		for start := 0; start < len(members); {
			end := start + 1
			for end < len(members) &&
				compareOrderValues(values[members[start]], values[members[end]], descending) == 0 {
				end++
			}
			for at := start; at < end; at++ {
				entry := InvestigationOrderEntry{
					Node: g.graphNode(input.objects[members[at]]), Rank: start + 1, TieCount: end - start,
				}
				if value := values[members[at]]; value.present {
					entry.Value = &value.value
				}
				entries[at] = entry
			}
			start = end
		}
		kinds = append(kinds, InvestigationOrderKind{Kind: kind, Entries: entries})
	}
	slices.SortFunc(kinds, func(left, right InvestigationOrderKind) int {
		return cmp.Compare(left.Kind, right.Kind)
	})
	return kinds
}

// compareOrderValues は 2 つの値を並びの順で比べる。負の値は left を上に並べる。
//
// **値は有効数字 10 桁に丸めてから比べる。** 浮動小数の計算の順で末尾の桁だけが違う値を、
// 別の順位にしない。値を持たない対象は、値を持つ対象のすべてより下に並べる。
func compareOrderValues(left, right orderValue, descending bool) int {
	if left.present != right.present {
		if left.present {
			return -1
		}
		return 1
	}
	if !left.present {
		return 0
	}
	order := cmp.Compare(roundedOrderValue(left.value), roundedOrderValue(right.value))
	if descending {
		return -order
	}
	return order
}

// orderValueSignificantDigits は、値を比べる前に丸める有効数字の桁数である。
const orderValueSignificantDigits = 10

// roundedOrderValue は値を有効数字 orderValueSignificantDigits 桁に丸める。桁を合わせる倍率が
// 浮動小数の範囲を超える値は、丸めずに返す。
func roundedOrderValue(value float64) float64 {
	if value == 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return value
	}
	exponent := orderValueSignificantDigits - 1 - int(math.Floor(math.Log10(math.Abs(value))))
	scale := math.Pow10(exponent)
	if math.IsInf(scale, 0) || scale == 0 {
		return value
	}
	return math.Round(value*scale) / scale
}
