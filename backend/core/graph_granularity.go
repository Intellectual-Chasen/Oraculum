package core

// GraphGranularity は、部分グラフにレコードのノードを出すか、対象のノードだけを出すかである。
type GraphGranularity string

// GraphGranularity の値。
const (
	// GraphGranularityRecord は、レコードのノードと対象のノードの両方を出す粒度である。
	// 検索の文字列はノードの属性で判定する。
	GraphGranularityRecord GraphGranularity = "record"
	// GraphGranularityObject は、対象のノードとその間の関係だけを出す粒度である。
	//
	// **検索の条件はレコード 1 件ごとに判定する。** 条件を満たしたレコードが記録した対象が
	// 合う。レコードのノードと、レコードが対象を指す関係は出さない。
	GraphGranularityObject GraphGranularity = "object"
)

// IsKnown は GraphGranularity が定義の中の値であるかを返す。
func (g GraphGranularity) IsKnown() bool {
	return g == GraphGranularityRecord || g == GraphGranularityObject
}
