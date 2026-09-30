package pipeline

import (
	"maps"
	"slices"
)

// separateCandidates は、2 件以上の候補のプロセスを持つ接続のレコードを、候補のプロセスの間の
// 中継にしない。from と to は起点と終点のノードである。
//
// **候補のプロセス P_i からレコード R への影響のエッジの先を、R に P_i から入った要素 R#i にする。**
// R から出る影響のエッジのうち、候補のプロセスへのエッジ以外を R#i にも複製する。R から候補の
// プロセスへの影響のエッジは R からだけ出る。P_i → R → P_j の列は経路にならず、P_i から R の
// 接続とログオンの組を通る列は残る。候補が 1 件のレコードは分けない。
//
// **起点と終点のノードのレコードは分けない。** 終点のレコードからは候補のプロセスへ出ず、起点の
// レコードへは候補のプロセスから入らない。除くのは、起点または終点へ戻る列だけである。
func (b *influenceGraph) separateCandidates(from, to int) {
	receives := map[int][]int{}
	outgoing := map[int][]int{}
	for index, edge := range b.edges {
		if _, receive := b.candidateReceives[index]; receive {
			receives[edge.exact.from] = append(receives[edge.exact.from], index)
			continue
		}
		outgoing[edge.exact.from] = append(outgoing[edge.exact.from], index)
	}
	dropped := map[int]struct{}{}
	for _, vertex := range slices.Sorted(maps.Keys(b.candidateSends)) {
		sends := b.candidateSends[vertex]
		var candidates []int
		for _, send := range sends {
			if !slices.Contains(candidates, send.candidate) {
				candidates = append(candidates, send.candidate)
			}
		}
		if len(candidates) <= 1 {
			continue
		}
		node := b.vertices[vertex].node
		if node == from || node == to {
			if node == to {
				for _, index := range receives[vertex] {
					dropped[index] = struct{}{}
				}
			}
			if node == from {
				for _, send := range sends {
					dropped[send.edge] = struct{}{}
				}
			}
			continue
		}
		for _, candidate := range candidates {
			entered := b.enteredVertex(node, candidate)
			for _, send := range sends {
				if send.candidate == candidate {
					b.edges[send.edge].exact.to, b.edges[send.edge].widened.to = entered, entered
				}
			}
			for _, index := range outgoing[vertex] {
				copied := b.edges[index]
				copied.exact.from, copied.widened.from = entered, entered
				b.edges = append(b.edges, copied)
			}
		}
	}
	if len(dropped) > 0 {
		kept := b.edges[:0]
		for index, edge := range b.edges {
			if _, drop := dropped[index]; !drop {
				kept = append(kept, edge)
			}
		}
		b.edges = kept
	}
}
