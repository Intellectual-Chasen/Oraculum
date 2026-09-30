// in-package test: G(a, b) の探索の状態の数が入力の大きさに比例することを確かめる。
package pipeline

import "testing"

// hubSteps は、起点から n 本の枝を経て 1 つの要素へ集まり、そこから n 本の枝を経て終点へ出る
// 影響のエッジを返す。要素 0 が起点、1 が集まる要素、2n+2 が終点である。extra は足すエッジである。
func hubSteps(n int, extra ...influenceStep) (int, []influenceStep) {
	var steps []influenceStep
	for i := range n {
		steps = append(steps, pointStep(0, 2+i, 0, int64(i)), pointStep(2+i, 1, 0, int64(10+i)))
	}
	for i := range n {
		steps = append(steps, pointStep(1, n+2+i, 0, int64(3*n+i)), pointStep(n+2+i, 2*n+2, 0, int64(4*n+i)))
	}
	return 2*n + 4, append(steps, extra...)
}

// 端末が 2 つ以上の探索は、同じ要素に遅く着いた集合を早く着いた集合で間引き、状態の数が枝の数に
// 比例する。集まる要素に着く時刻が n 通りあっても、そこから出る枝を n 回ずつ展開しない。
func TestReachManyClocksKeepsTheStatesLinear(t *testing.T) {
	const n = 200
	vertexCount, steps := hubSteps(n, pointStep(0, 2*n+3, 1, 5))
	a, b := pathEnd{vertex: 0, at: 0}, pathEnd{vertex: 2*n + 2, at: int64(10 * n)}
	outcome := reachManyClocks(vertexCount, steps, a, b)
	if len(outcome.onPath) != 4*n {
		t.Fatalf("G(a, b) has %d edges, want %d", len(outcome.onPath), 4*n)
	}
	if outcome.expansions > 10*n {
		t.Errorf("expansions = %d, want at most %d", outcome.expansions, 10*n)
	}
}

// 起点から前へ届かないエッジの端末は、探索の端末に数えない。経路の範囲の端末が 1 つなら、EA と
// LD で求める。
func TestReachCountsOnlyTheClocksBetweenTheEnds(t *testing.T) {
	const n = 10
	vertexCount, steps := hubSteps(n, pointStep(2*n+3, 1, 1, 5))
	a, b := pathEnd{vertex: 0, at: 0}, pathEnd{vertex: 2*n + 2, at: int64(10 * n)}
	outcome := reach(vertexCount, steps, a, b)
	if outcome.states != 0 || len(outcome.onPath) != 4*n {
		t.Errorf("states=%d edges=%d, want the one-clock computation and %d edges", outcome.states, len(outcome.onPath), 4*n)
	}
}
