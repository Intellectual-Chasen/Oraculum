package pipeline

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// addReverseLookupNameEdges は、接続先のアドレスを収集元が逆引きした名前のノードへ、候補の
// 関係を張る。
//
// **起点は、そのレコードが記録したプロセスである。** レコードのノードが指すプロセスの
// ノードを持たないときは、レコードのノードを起点にする。名前はレコードのノードの属性から読む。
//
// **名前のノードは、要求したホスト名と同じ識別鍵で組む。** 同じ名前を要求したレコードが
// あれば、そのノードへ候補を張る。無いノードは参照だけのノードとして足し、そのレコードを
// 根拠にする。既にあるノードの状態と根拠と表示名を変えない。
func (g *Graph) addReverseLookupNameEdges() {
	for recordAt, record := range g.records {
		if !record.hasRecordNode {
			continue
		}
		for _, attribute := range g.nodes[record.recordNode].attributes {
			if attribute.field.Semantic != core.SemanticKeyConnectionDestinationReverseLookupName {
				continue
			}
			// 識別鍵を組む経路は語彙の役割で項目を選ぶため、要求したホスト名の項目として渡す。
			name := *attribute.field
			name.Semantic = core.SemanticKeyConnectionDestinationHostname
			names := []core.RecordField{name}
			for _, key := range core.DestinationNodeKeys(names, core.RecordScope{}) {
				g.addReverseLookupNameEdge(recordAt, record.recordNode, key, names)
			}
		}
	}
}

// addReverseLookupNameEdge は、レコード 1 件が指す逆引きの名前 1 つへ候補の関係を張る。
func (g *Graph) addReverseLookupNameEdge(recordAt, recordNode int, key core.NodeKey, names []core.RecordField) {
	target := g.ensureNode(key, core.NodeObservationReferenced)
	if label, labelled := labelOfSemantic(names, core.SemanticKeyConnectionDestinationHostname); labelled {
		g.nodes[target].applyLabel(label)
	}
	at := g.ensureEdge(core.EdgeKindReverseLookupName, core.RelationStateCandidate,
		g.processNamedBy(recordNode), target)
	g.addEdgeEvidence(at, recordAt)
	if node := &g.nodes[target]; node.observation == core.NodeObservationReferenced &&
		!slices.Contains(node.evidence, recordAt) {
		node.evidence = append(node.evidence, recordAt)
	}
}

// processNamedBy は、レコードのノードが指すプロセスのノードの位置を返す。指すプロセスが
// 無いときは、レコードのノードの位置を返す。
func (g *Graph) processNamedBy(recordNode int) int {
	for _, edgeAt := range g.adjacency[recordNode].outgoing {
		edge := g.edges[edgeAt]
		if edge.kind == core.EdgeKindRecordNamesObject && g.nodes[edge.target].key.Kind == core.NodeKindProcess {
			return edge.target
		}
	}
	return recordNode
}
