package pipeline

import (
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RecordGraphElements は 1 レコードを根拠に持つノードとエッジの識別子である。
type RecordGraphElements struct {
	// NodeIds は、そのレコードを根拠に持つノードと、EdgeIds のエッジの端点である。
	// 並びはグラフの並び順で、同じ識別子を 2 回持たない。
	NodeIds []string
	// EdgeIds は、そのレコードを根拠に持つエッジである。並びはグラフの並び順である。
	EdgeIds []string
}

// RecordGraphElements はレコードの位置から、そのレコードを根拠に持つノードとエッジを返す。
// レコードがグラフの根拠に入っていないときは、どちらも空である。
//
// ponytail: 呼ぶたびに全ノードと全エッジの根拠を走査する。1 回の選択で 1 回呼ぶ操作であり、
// 遅さが問題になったら、レコードからエッジを探す索引をグラフの組み立てで作る。
func (g Graph) RecordGraphElements(locator core.RecordLocator) RecordGraphElements {
	at, found := g.recordAtLocator(locator)
	if !found {
		return RecordGraphElements{NodeIds: []string{}, EdgeIds: []string{}}
	}
	return g.recordsGraphElements(map[int]bool{at: true})
}

// recordsGraphElements は、records のどれかのレコードを根拠に持つノードとエッジを返す。
// records の鍵はレコードの g.records での位置である。全ノードと全エッジの根拠を 1 回ずつ走査する。
func (g Graph) recordsGraphElements(records map[int]bool) RecordGraphElements {
	elements := RecordGraphElements{NodeIds: []string{}, EdgeIds: []string{}}
	cites := func(evidence []int) bool {
		for _, at := range evidence {
			if records[at] {
				return true
			}
		}
		return false
	}
	related := make([]bool, len(g.nodes))
	for index, node := range g.nodes {
		if cites(node.evidence) {
			related[index] = true
		}
	}
	for _, edge := range g.edges {
		if !cites(edge.evidence) {
			continue
		}
		elements.EdgeIds = append(elements.EdgeIds, edge.id)
		related[edge.source] = true
		related[edge.target] = true
	}
	for index, node := range g.nodes {
		if related[index] {
			elements.NodeIds = append(elements.NodeIds, node.id)
		}
	}
	return elements
}
