package pipeline

import "slices"

// exactSlice は s を長さと同じ容量の配列へ移して返す。容量が長さと同じ s はそのまま返す。
//
// **nil と要素数 0 の集合を区別したまま返す。** 容量も長さに揃えるため、返した slice へ
// 足すと新しい配列を取り、元の配列を共有する別のグラフを書き換えない。
func exactSlice[T any](s []T) []T {
	if cap(s) == len(s) {
		return s
	}
	return slices.Clip(slices.Clone(s))
}

// releaseObservedSpare は、観測の層を組み終えたグラフの slice を長さに合わせた配列へ移し、
// 足していくときに伸ばした分の容量を返す。
//
// 観測の層は組んだ後に要素を足さない。選択ごとのグラフは観測の層を複製してから足す
// (withoutCandidateEdges)。
func (g *Graph) releaseObservedSpare() {
	g.records = exactSlice(g.records)
	g.nodes = exactSlice(g.nodes)
	for index := range g.nodes {
		node := &g.nodes[index]
		node.creationRecords = exactSlice(node.creationRecords)
		node.attributes = exactSlice(node.attributes)
		for at := range node.attributes {
			node.attributes[at].evidence = exactSlice(node.attributes[at].evidence)
		}
		node.evidence = exactSlice(node.evidence)
	}
	g.releaseAdjacencySpare()
	g.edges = exactSlice(g.edges)
	for index := range g.edges {
		g.edges[index].evidence = exactSlice(g.edges[index].evidence)
		if basis := g.edges[index].basis; basis != nil {
			basis.pairs = exactSlice(basis.pairs)
		}
	}
}

// releaseAdjacencySpare はノードごとの隣接を長さに合わせた配列へ移す。
func (g *Graph) releaseAdjacencySpare() {
	g.adjacency = exactSlice(g.adjacency)
	for index := range g.adjacency {
		g.adjacency[index].outgoing = exactSlice(g.adjacency[index].outgoing)
		g.adjacency[index].incoming = exactSlice(g.adjacency[index].incoming)
	}
}

// releaseCandidateSpare は、候補のエッジを足し終えたグラフの slice を長さに合わせた配列へ
// 移す。観測の層と共有する配列は容量が長さと同じであり、そのまま残る。
func (g *Graph) releaseCandidateSpare() {
	g.edges = exactSlice(g.edges)
	for index := g.observedEdgeCount; index < len(g.edges); index++ {
		g.edges[index].evidence = exactSlice(g.edges[index].evidence)
		if basis := g.edges[index].basis; basis != nil {
			basis.matches = exactSlice(basis.matches)
		}
	}
	g.matchStages = exactSlice(g.matchStages)
	g.releaseAdjacencySpare()
}
