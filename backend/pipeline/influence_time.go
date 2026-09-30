package pipeline

import "math"

// 影響のエッジの時刻は、基準の時刻からの ns の差 (int64) で持つ。
const (
	// noLower と noUpper は、区間が下限または上限を持たないことを表す端点である。
	noLower int64 = math.MinInt64
	noUpper int64 = math.MaxInt64
)

// timeSpan は、タイムスタンプの区間 [lo, hi] (両端を含む) である。
type timeSpan struct{ lo, hi int64 }

// intersect は 2 つの区間の共通部分を返す。
func (s timeSpan) intersect(other timeSpan) timeSpan {
	return timeSpan{max(s.lo, other.lo), min(s.hi, other.hi)}
}

// clockSpan は時刻の制約 1 つであり、区間と、そのタイムスタンプを記録した端末の組である。
// clock は端末の番号である。
type clockSpan struct {
	clock int
	span  timeSpan
}

// influenceStep は、経路を求める計算が読む影響のエッジ 1 本である。from と to は要素の番号、
// depart と arrive は D と A の制約である。どちらも要素数 1 以上である。
type influenceStep struct {
	from, to       int
	depart, arrive []clockSpan
}
