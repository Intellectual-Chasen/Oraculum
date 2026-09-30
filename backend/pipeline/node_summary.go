package pipeline

import (
	"cmp"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// NodeSummary は、1 つのノードとそれに接するレコードの件数と時刻の範囲である。
type NodeSummary struct {
	Node core.GraphNode
	// Terminal は、端末の範囲に置いたアドレスのノード (core.NodeKeyFormTerminalAddress) の、
	// 範囲の端末のノードである。範囲を持たないノードと、範囲の端末のノードがグラフに無いときは
	// nil である。
	Terminal *core.GraphNode
	// RecordCount は、ノードの根拠と、ノードに接するエッジの根拠のうち、絞り込みを通った
	// レコードの件数である。同じレコードを 2 回数えない。
	RecordCount int
	// FirstTime と LastTime は、数えたレコードのうち時刻を比べられるものの最も早い時刻と
	// 最も遅い時刻である。比べられるレコードが無いときは nil である。
	FirstTime *core.Timestamp
	LastTime  *core.Timestamp
	// LocalTimeRecordCount は、数えたレコードのうち UTC からのずれの決まらない地方時の文字列を持ち、
	// 時点を持たないものの件数である。UndatedRecordCount は、時刻を持たないものの件数である。
	//
	// **2 つは FirstTime と LastTime の範囲の外にある。** 範囲をノードの全レコードの期間として
	// 読ませないよう、範囲から外れたレコードの件数を別に返す。分け方は時系列と同じである
	// (graphRecord.localWithoutInstant)。
	LocalTimeRecordCount int
	UndatedRecordCount   int
}

// NodeSummaries は kind のノードごとに、接するレコードの件数と時刻の範囲を返す。
// 絞り込みを通ったレコードが 1 件も無いノードは含めない。並びは件数の降順、同じ件数では
// 識別子の昇順である。
//
// expression は検索式である。nil は式で絞らない。式はレコード 1 件ごとに、そのレコードの
// ノードの属性で判定する (recordPasses)。時系列の要求の式と同じ判定である。
func (g Graph) NodeSummaries(kind core.NodeKind, filter RecordFilter, expression *SearchExpression) []NodeSummary {
	filter.Validate()
	passes := g.recordPasses(g.withResolvedFieldNames(GraphQuery{RecordFilter: filter, Expression: expression}))
	summaries := []NodeSummary{}
	// countedBy はレコードの位置ごとに、そのレコードを最後に数えたノードの位置に 1 を足した値である。
	// ノードごとに印を消さずに済み、1 つのノードの根拠が多くても次のノードの判定の費用が増えない。
	countedBy := make([]int, len(g.records))
	for index, node := range g.nodes {
		if node.key.Kind != kind {
			continue
		}
		summary := NodeSummary{}
		var first, last *graphRecord
		count := func(at int) {
			if countedBy[at] == index+1 || !passes(at) {
				return
			}
			countedBy[at] = index + 1
			summary.RecordCount++
			record := &g.records[at]
			if !record.hasInstant {
				if record.localWithoutInstant() {
					summary.LocalTimeRecordCount++
				} else {
					summary.UndatedRecordCount++
				}
				return
			}
			if first == nil || record.instant.Before(first.instant) {
				first = record
			}
			if last == nil || record.instant.After(last.instant) {
				last = record
			}
		}
		for _, at := range node.evidence {
			count(at)
		}
		adjacency := g.adjacency[index]
		for _, edges := range [][]int{adjacency.outgoing, adjacency.incoming} {
			for _, edge := range edges {
				for _, at := range g.edges[edge].evidence {
					count(at)
				}
			}
		}
		if summary.RecordCount == 0 {
			continue
		}
		summary.Node = g.graphNode(index)
		if terminalAt, found := g.scopeTerminalOf(node.key); found {
			terminal := g.graphNode(terminalAt)
			summary.Terminal = &terminal
		}
		if first != nil {
			summary.FirstTime = cloneTimestampPointer(first.eventTime)
			summary.LastTime = cloneTimestampPointer(last.eventTime)
		}
		summaries = append(summaries, summary)
	}
	slices.SortFunc(summaries, func(left, right NodeSummary) int {
		return cmp.Or(
			cmp.Compare(right.RecordCount, left.RecordCount),
			cmp.Compare(left.Node.Id, right.Node.Id),
		)
	})
	return summaries
}

// scopeTerminalOf は、端末の範囲に置いたアドレスの鍵 (core.NodeKeyFormTerminalAddress) から、
// 範囲の端末のノードの位置を返す。
//
// **端末の範囲に置いた対象の鍵は、端末の鍵の値だけを持ち、形を持たない** (terminalScopedValues)。
// 端末の鍵の形ごとに候補の鍵を組み、グラフにあるノードを探す。当たった候補が 1 つのときだけ
// 返す。形の違う 2 つの端末が同じ値の並びを持つときは、どちらの端末かを決められない。
//
// **core に端末の鍵の形を追加したら、ここに候補を追加する。** 候補に無い形の端末は、範囲の
// 端末として見つからない。
//
// found が偽になるのは、範囲を持たない鍵と、範囲の端末のノードがグラフに無いか 1 つに決まらない
// ときである。
func (g Graph) scopeTerminalOf(key core.NodeKey) (int, bool) {
	// hostTerminalKeyValueCount は、収集元の内容の識別とホスト名で指す端末の鍵の値の数である。
	const hostTerminalKeyValueCount = 2
	if key.Form != core.NodeKeyFormTerminalAddress || len(key.Values) < 2 {
		return 0, false
	}
	values := key.Values[:len(key.Values)-1]
	var candidates []core.NodeKey
	add := func(candidate core.NodeKey, built bool) {
		if built {
			candidates = append(candidates, candidate)
		}
	}
	switch len(values) {
	case 1:
		add(core.TerminalNodeKey(values[0].Value))
		add(core.RecordingTerminalNodeKey(values[0].Value))
		add(core.CollectionTerminalNodeKey(values[0].Value))
	case hostTerminalKeyValueCount:
		add(core.RecordingHostTerminalNodeKey(values[0].Value, values[1].Value))
	}
	found := -1
	for _, candidate := range candidates {
		at, present := g.nodeAt[nodeIdOf(candidate)]
		if !present {
			continue
		}
		if found >= 0 && found != at {
			return 0, false
		}
		found = at
	}
	return found, found >= 0
}
