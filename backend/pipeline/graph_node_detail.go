package pipeline

import (
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// NodeDetail はノード 1 つの詳細である。属性と根拠を全件含む。
type NodeDetail struct {
	// Node はノードの識別と表示名である。
	Node core.GraphNode
	// Attributes は役割が identity の項目を外した残りに観測した値である。役割が label の
	// 項目も入る (addAttributes)。
	Attributes []core.NodeAttribute
	// AttributeCount は属性の意味の個数である。
	AttributeCount int
	// Evidence はノードを指すレコードである。
	Evidence []core.GraphEvidence
	// EvidenceCount は根拠のレコードの総数である。
	EvidenceCount int
	// EvidenceByCase は EvidenceCount を根拠のレコードの案件ごとに分けた件数である。
	// 案件を区別しない取り込みでは nil である。
	EvidenceByCase []core.CaseEvidenceCount
	// CreationRecords は対象の生成を記録した根拠のレコードである。並びはグラフへ入った
	// 順である。生成を記録した根拠を持てない種別のノードでは要素数 0 である。
	CreationRecords []core.GraphEvidence
	// CreationRecordCount は生成の根拠のレコードの総数である。
	CreationRecordCount int
	// EdgeCounts はノードに繋がるエッジの、種別と向きごとの件数である。
	EdgeCounts []core.NodeEdgeCount
	// RelationDerivation は、このレコードを起点にして関係を導いた結果である。
	// レコードの種別のノードだけが持つ。他の種別のノードでは nil である。
	RelationDerivation *core.RelationDerivation
	// LogonSessionRejections は、このレコードの操作の Logon ID が一致しながら、関係にしな
	// かったログオンのレコードと理由である。そのような組を持たないノードでは要素数 0 である。
	LogonSessionRejections []core.LogonSessionRejection
}

// NodeDetail は識別子で指したノードの詳細を返す。
// ok が偽になるのは、その識別子のノードがグラフに無いときである。
func (g Graph) NodeDetail(id string) (NodeDetail, bool) {
	index, found := g.nodeAt[id]
	if !found {
		return NodeDetail{}, false
	}
	node := g.nodes[index]
	attributes := g.groupedAttributes(node)
	detail := NodeDetail{
		Node:                g.subgraphNode(index, core.NodeSelectionMatched, GraphQuery{}).GraphNode,
		Attributes:          attributes,
		AttributeCount:      len(attributes),
		Evidence:            g.evidenceItems(node.evidence),
		EvidenceCount:       len(node.evidence),
		EvidenceByCase:      g.evidenceByCase(node.evidence),
		EdgeCounts:          g.nodeEdgeCounts(g.adjacency[index]),
		CreationRecords:     g.evidenceItems(node.creationRecords),
		CreationRecordCount: len(node.creationRecords),
	}
	detail.LogonSessionRejections = g.logonSessionRejectionsOf(id)
	if derivation, carried := g.RelationDerivationOf(id); carried {
		detail.RelationDerivation = &derivation
	}
	return detail, true
}

// groupedAttributes は観測した属性を意味ごとにまとめる。並びは最初に観測した順である。
//
// **同じ意味に観測した異なる値を 1 つに寄せない。** 値の個数が 2 以上である要素は、
// 出典の異なる値が食い違っている状態を表す。
//
// 既知の制限: 1 つの意味に観測した値の集合 (NodeAttribute.Values) に上限と続きを与える
// 要求の項目を持たせない,
// 1 つの意味の値の個数は、同じ対象について出典が食い違った数であり、ノードの根拠の件数より
// 小さい,
// 1 つの意味の値の個数が要求の上限を要する大きさになる入力形式を取り込んだとき。
// **矛盾する値を 1 つに寄せて要素数を固定しない**
func (g Graph) groupedAttributes(node graphNode) []core.NodeAttribute {
	order := make([]attributeGroupKey, 0, len(node.attributes))
	grouped := make(map[attributeGroupKey][]core.NodeAttributeValue, len(node.attributes))
	for _, attribute := range node.attributes {
		key := attributeGroupKeyOf(*attribute.field)
		if _, seen := grouped[key]; !seen {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], core.NodeAttributeValue{
			Field:            cloneRecordField(*attribute.field),
			ObservationCount: int64(len(attribute.evidence)),
			FirstRecordRef:   cloneLocator(g.records[attribute.firstRecord()].locator),
		})
	}
	attributes := make([]core.NodeAttribute, 0, len(order))
	for _, key := range order {
		values := grouped[key]
		attributes = append(attributes, core.NodeAttribute{
			Semantic: key.semantic, Name: key.name,
			ValueCount: int64(len(values)), Values: values,
		})
	}
	return attributes
}

// attributeGroupKey は属性を 1 つにまとめる鍵である。
type attributeGroupKey struct {
	semantic core.SemanticKey
	name     string
}

// attributeGroupKeyOf は 1 項目のまとめ鍵を返す。
//
// **語彙の項目を持つ欄は、語彙の項目だけでまとめる。** 原資料の key を鍵に混ぜると、
// 同じ意味を別の key が持つ収集元で 1 つの意味が 2 つの属性に割れる。markii 形式の wsIp と ip は
// どちらも terminal.ip_address へ写るため、同じ 1 つの属性にまとまる。
//
// **語彙に写していない欄は原資料の key の文字列でまとめる。** その欄を指す名前が他に無い。
func attributeGroupKeyOf(field core.RecordField) attributeGroupKey {
	if field.Semantic != "" {
		return attributeGroupKey{semantic: field.Semantic}
	}
	return attributeGroupKey{name: field.Name}
}

// RelatedValueCounts は、識別子で指したノードに繋がる kind の関係のうち direction の向きの
// ものについて、相手側のノードが数える欄に観測した値ごとの件数を返す。ok が偽になるのは、
// その識別子のノードがグラフに無いときである。
//
// **数えるのは、その関係の根拠のレコードに観測した値だけである。** 相手の属性の根拠と、
// エッジの根拠の共通部分を数える。相手が対象のノード (IP・アカウントなど) でも、その関係に
// 関わらないレコードの観測を数えない。同じレコードに 2 本のエッジから届いても 1 件と数える。
//
// **相手が対象のノードのとき、その識別の欄 (役割が identity の語彙) は、関係の根拠のレコードが
// 記録した相手の値として数える。** 対象のノードは identity の欄を属性に持たない (addAttributes)。
// 根拠のレコードのその欄の値のうち、相手の識別鍵の値と同じ値だけを数える。
//
// **期間と案件では絞らない。** ノードの詳細の関係の件数 (EdgeCounts) と同じ範囲である。
func (g Graph) RelatedValueCounts(
	id string, kind core.EdgeKind, direction core.EdgeDirection, query GraphQuery,
) ([]core.ValueCount, bool) {
	index, found := g.nodeAt[id]
	if !found {
		return nil, false
	}
	query = g.withResolvedFieldNames(query)
	edges := g.adjacency[index].outgoing
	if direction == core.EdgeDirectionIncoming {
		edges = g.adjacency[index].incoming
	}
	order := make([]string, 0)
	records := make(map[string]map[int]struct{})
	add := func(value string, recordAt int) {
		if _, seen := records[value]; !seen {
			order = append(order, value)
			records[value] = make(map[int]struct{})
		}
		records[value][recordAt] = struct{}{}
	}
	for _, at := range edges {
		edge := g.edges[at]
		if edge.kind != kind {
			continue
		}
		other := edge.target
		if direction == core.EdgeDirectionIncoming {
			other = edge.source
		}
		related := make(map[int]struct{}, len(edge.evidence))
		for _, recordAt := range edge.evidence {
			related[recordAt] = struct{}{}
		}
		for _, attribute := range g.nodes[other].attributes {
			if !query.countsField(*attribute.field) {
				continue
			}
			value, readable := comparableFieldValue(*attribute.field)
			if !readable {
				continue
			}
			for _, recordAt := range attribute.evidence {
				if _, inEdge := related[recordAt]; inEdge {
					add(value, recordAt)
				}
			}
		}
		if g.nodes[other].key.Kind != core.NodeKindRecord {
			g.countIdentityOfOtherEnd(g.nodes[other].key, edge.evidence, query, add)
		}
	}
	counts := make([]core.ValueCount, 0, len(order))
	for _, value := range order {
		counts = append(counts, g.valueCountOf(value, records[value]))
	}
	return counts, true
}

// countIdentityOfOtherEnd は、関係の根拠のレコード evidence が数える欄に記録した値のうち、相手の
// 対象のノードの識別鍵 key の値と同じ値を add へ渡す。数えるのは役割が identity の欄だけであり、
// 対象のノードの属性として数える欄と重ならない。
func (g Graph) countIdentityOfOtherEnd(
	key core.NodeKey, evidence []int, query GraphQuery, add func(value string, recordAt int),
) {
	for _, recordAt := range evidence {
		record := g.records[recordAt]
		if !record.hasRecordNode {
			continue
		}
		for _, attribute := range g.nodes[record.recordNode].attributes {
			field := *attribute.field
			if field.Semantic.Role() != core.SemanticRoleIdentity || !query.countsField(field) {
				continue
			}
			// 端末ではない相手の識別鍵の端末の値は、相手の範囲であり、相手の識別の値ではない。
			if field.Semantic.Object() == core.SemanticObjectTerminal && key.Kind != core.NodeKindTerminal {
				continue
			}
			value, readable := comparableFieldValue(field)
			if !readable || !slices.ContainsFunc(key.Values, func(identity core.NodeIdentityValue) bool {
				return strings.EqualFold(identity.Value, value)
			}) {
				continue
			}
			add(value, recordAt)
		}
	}
}

// nodeEdgeCounts はノードに繋がるエッジの件数を、種別と向きごとに数える。
//
// **上限と続きを与える要求の項目を持たない。** 要素数は関係の種別の個数と向きの個数で
// 決まり、レコード件数に比例しない。
func (g Graph) nodeEdgeCounts(adjacency nodeAdjacency) []core.NodeEdgeCount {
	counts := make([]core.NodeEdgeCount, 0, len(adjacency.outgoing)+len(adjacency.incoming))
	// evidence は counts と同じ位置の要素が数えた根拠のレコードの位置である。
	var evidence [][]int
	for _, side := range []struct {
		direction core.EdgeDirection
		edges     []int
	}{
		{core.EdgeDirectionOutgoing, adjacency.outgoing},
		{core.EdgeDirectionIncoming, adjacency.incoming},
	} {
		at := make(map[core.EdgeKind]int, len(side.edges))
		for _, index := range side.edges {
			edge := g.edges[index]
			position, seen := at[edge.kind]
			if !seen {
				counts = append(counts, core.NodeEdgeCount{
					EdgeKind: edge.kind, Direction: side.direction,
				})
				evidence = append(evidence, nil)
				position = len(counts) - 1
				at[edge.kind] = position
			}
			counts[position].EdgeCount++
			counts[position].EvidenceCount += int64(len(edge.evidence))
			evidence[position] = append(evidence[position], edge.evidence...)
		}
	}
	for position := range counts {
		counts[position].EvidenceByCase = g.evidenceByCase(evidence[position])
	}
	return counts
}
