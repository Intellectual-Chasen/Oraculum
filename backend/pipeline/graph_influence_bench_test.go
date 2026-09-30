// in-package test: 影響のエッジを作る所要を、よく書かれるファイルで測る。
package pipeline

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// busyFileGraph は、1 つのプロセスが 1 つのファイルへ writes 件書き込み、100 件に 1 件が内容の
// 置き換えであるグラフを組む。
func busyFileGraph(b testing.TB, writes int) *fakeGraph {
	f := newFakeGraph(b)
	process, file := f.node(core.NodeKindProcess, "p"), f.node(core.NodeKindFile, "file")
	records := make([]int, 0, writes)
	for i := range writes {
		options := []recordOption{byProcess(process), withFlow(core.FlowOperationWrite)}
		if i%100 == 0 {
			options = append(options, replacing())
		}
		records = append(records, f.record(i, options...))
	}
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, process, file, records...)
	return f
}

// BenchmarkNewInfluenceGraphOnABusyFile は、よく書かれるファイルの影響のエッジを作る所要を測る。
// 書き込みの件数を 4 倍にしたときの所要の比で、件数の 2 乗に比例するかを読む。
func BenchmarkNewInfluenceGraphOnABusyFile(b *testing.B) {
	for _, writes := range []int{2_500, 10_000} {
		f := busyFileGraph(b, writes)
		b.Run(strconv.Itoa(writes), func(b *testing.B) {
			for range b.N {
				f.g.newInfluenceGraph(influenceBase)
			}
		})
	}
}
