// in-package test: 影響の経路の集合 G(a, b) を、定義の経路の条件から数え上げた集合と比べる。
package pipeline

import (
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
)

// point は 1 件のレコードの影響のエッジの制約 (区間が点) を返す。
func point(clock int, at int64) []clockSpan { return []clockSpan{{clock, timeSpan{at, at}}} }

// pointStep は D と A が同じ点の影響のエッジを返す。
func pointStep(from, to, clock int, at int64) influenceStep {
	return influenceStep{from: from, to: to, depart: point(clock, at), arrive: point(clock, at)}
}

// feasibleWalk は、影響のエッジの列 walk が、定義の経路の条件 2 と 3 を満たすかを返す。
//
// 変数は各エッジの T^d と T^a と、端末ごとのずれ o_C である。条件はどれも 2 つの変数の差の
// 上限であり、差の上限の和が負になる巡回が無いとき、条件を満たす値を選べる (Bellman-Ford)。
func feasibleWalk(steps []influenceStep, walk []int, a, b pathEnd, clockCount int) bool {
	type bound struct {
		from, to int
		weight   int64
	}
	// 変数 to - 変数 from ≤ weight を from から to への重みにする。
	var bounds []bound
	departVar := func(i int) int { return clockCount + 2*i }
	arriveVar := func(i int) int { return clockCount + 2*i + 1 }
	limit := func(x, y int, c int64) { bounds = append(bounds, bound{y, x, c}) } // x - y ≤ c
	within := func(variable int, constraints []clockSpan) {
		for _, constraint := range constraints {
			if constraint.span.hi != noUpper {
				limit(variable, constraint.clock, constraint.span.hi)
			}
			if constraint.span.lo != noLower {
				limit(constraint.clock, variable, -constraint.span.lo)
			}
		}
	}
	for i, at := range walk {
		within(departVar(i), steps[at].depart)
		within(arriveVar(i), steps[at].arrive)
		limit(departVar(i), arriveVar(i), 0)
		if i+1 < len(walk) {
			limit(arriveVar(i), departVar(i+1), 0)
		}
	}
	limit(a.clock, departVar(0), -a.at)
	limit(arriveVar(len(walk)-1), b.clock, b.at)
	distance := make([]int64, clockCount+2*len(walk))
	for range len(distance) {
		for _, edge := range bounds {
			if distance[edge.from]+edge.weight < distance[edge.to] {
				distance[edge.to] = distance[edge.from] + edge.weight
			}
		}
	}
	for _, edge := range bounds {
		if distance[edge.from]+edge.weight < distance[edge.to] {
			return false
		}
	}
	return true
}

// enumeratedPathEdges は、長さ maxLength までの a から b への影響のエッジの列のうち、経路の
// 条件を満たす列に乗るエッジの番号の集合を返す。
func enumeratedPathEdges(steps []influenceStep, a, b pathEnd, clockCount, maxLength int) map[int]bool {
	on := map[int]bool{}
	var walk []int
	var extend func(vertex int)
	extend = func(vertex int) {
		if len(walk) > 0 && vertex == b.vertex && feasibleWalk(steps, walk, a, b, clockCount) {
			for _, at := range walk {
				on[at] = true
			}
		}
		if len(walk) == maxLength {
			return
		}
		for at, step := range steps {
			if step.from == vertex {
				walk = append(walk, at)
				extend(step.to)
				walk = walk[:len(walk)-1]
			}
		}
	}
	extend(a.vertex)
	return on
}

func onPathSet(outcome reachOutcome) map[int]bool {
	on := map[int]bool{}
	for at := range outcome.onPath {
		on[at] = true
	}
	return on
}

func sortedKeys(set map[int]bool) []int { return slices.Sorted(maps.Keys(set)) }

// 端末が 1 つの入力で、EA と LD の式で求めた集合が、手で求めた集合と一致する。
func TestReachOneClockMatchesTheHandComputedSet(t *testing.T) {
	a, b := pathEnd{vertex: 0, at: 0}, pathEnd{vertex: 3, at: 100}
	steps := []influenceStep{
		pointStep(0, 1, 0, 10),  // 0: a → x
		pointStep(1, 3, 0, 20),  // 1: x → b
		pointStep(0, 2, 0, 30),  // 2: a → y。y から b へは 25 の後に出られない
		pointStep(2, 3, 0, 25),  // 3: y → b
		pointStep(1, 2, 0, 15),  // 4: x → y。a → x → y → b が 10 ≤ 15 ≤ 25 でつながる
		pointStep(3, 1, 0, 50),  // 5: b → x。x から b へは 50 の後に出られない
		pointStep(0, 3, 0, 150), // 6: 終点のタイムスタンプより後
		// 7: 2 件のレコードの候補で、A が D より前。T^d ≤ T^a を選べない。
		{from: 0, to: 3, depart: point(0, 60), arrive: point(0, 55)},
		// 8: 等価の影響のエッジのように 2 つの制約を持つ。共通部分 [40, 50] で成り立つ。
		{from: 0, to: 3, depart: []clockSpan{{0, timeSpan{0, 50}}, {0, timeSpan{40, 90}}},
			arrive: []clockSpan{{0, timeSpan{0, 50}}, {0, timeSpan{40, 90}}}},
		// 9: 共通部分が空の 2 つの制約。
		{from: 0, to: 3, depart: []clockSpan{{0, timeSpan{0, 30}}, {0, timeSpan{40, 90}}},
			arrive: point(0, 60)},
		// 10: 下限を持たない区間。T^d を起点の後に選べる。
		{from: 0, to: 3, depart: []clockSpan{{0, timeSpan{noLower, 70}}}, arrive: point(0, 70)},
	}
	outcome := reach(4, steps, a, b)
	want := []int{0, 1, 3, 4, 8, 10}
	if got := sortedKeys(onPathSet(outcome)); !slices.Equal(got, want) {
		t.Fatalf("G(a, b) = %v, want %v", got, want)
	}
	for at, clocks := range outcome.onPath {
		if !clocks.single || clocks.mixed {
			t.Errorf("edge %d: %+v, want only paths of one terminal", at, clocks)
		}
	}
	if enumerated := enumeratedPathEdges(steps, a, b, 1, 7); !maps.Equal(enumerated, onPathSet(outcome)) {
		t.Errorf("enumerated %v, want the same set as %v", sortedKeys(enumerated), want)
	}
}

// 起点から届いた要素のうち、出るエッジがどれも時刻の条件を満たさない要素と、出るエッジを
// 持たない要素を分ける。届かない要素は reached に入らない。
func TestReachReportsTheVerticesWhereTheInfluenceStops(t *testing.T) {
	a, b := pathEnd{vertex: 0, at: 0}, pathEnd{vertex: 3, at: 100}
	steps := []influenceStep{
		pointStep(0, 1, 0, 10), pointStep(1, 3, 0, 5), // 1 から出るエッジは届く前
		pointStep(0, 2, 0, 20), // 2 から出るエッジは無い
		pointStep(4, 3, 0, 30), // 4 には届かない
	}
	for name, run := range map[string]func(int, []influenceStep, pathEnd, pathEnd) reachOutcome{
		"one clock": reachOneClock, "many clocks": reachManyClocks,
	} {
		outcome := run(5, steps, a, b)
		if !slices.Equal(outcome.reached, []int{0, 1, 2}) {
			t.Errorf("%s: reached = %v, want [0 1 2]", name, outcome.reached)
		}
		if !outcome.blocked[1] || outcome.blocked[2] || outcome.blocked[0] {
			t.Errorf("%s: blocked = %v, want only 1", name, outcome.blocked)
		}
		if len(outcome.onPath) != 0 {
			t.Errorf("%s: G(a, b) = %v, want empty", name, outcome.onPath)
		}
	}
}

// 端末が 2 つの入力で、同じ端末のタイムスタンプの順序は別の端末のエッジを挟んでも保たれ、
// 別の端末のタイムスタンプとの順序はずれで選べる。
func TestReachManyClocksKeepsTheOrderOfOneTerminal(t *testing.T) {
	const c, d = 0, 1
	a, b := pathEnd{vertex: 0, clock: c, at: 0}, pathEnd{vertex: 5, clock: c, at: 100}
	steps := []influenceStep{
		pointStep(0, 1, c, 10),  // 0: a → x
		pointStep(1, 2, d, 500), // 1: x → y (端末 d)
		pointStep(2, 5, c, 5),   // 2: y → b。端末 c の 10 の後に 5 を選べない
		pointStep(1, 3, d, 400), // 3: x → z (端末 d)
		pointStep(3, 5, c, 50),  // 4: z → b。c の 10 ≤ 50 ≤ 100 で、d のずれを選べる
		pointStep(3, 4, d, 300), // 5: z → w。端末 d の 400 の後に 300 を選べない
		pointStep(4, 5, c, 60),  // 6: w → b
	}
	outcome := reach(6, steps, a, b)
	if got := sortedKeys(onPathSet(outcome)); !slices.Equal(got, []int{0, 3, 4}) {
		t.Fatalf("G(a, b) = %v, want [0 3 4]", got)
	}
	for at, clocks := range outcome.onPath {
		if clocks.single || !clocks.mixed {
			t.Errorf("edge %d: %+v, want only paths of two terminals", at, clocks)
		}
	}
	if enumerated := enumeratedPathEdges(steps, a, b, 2, 11); !maps.Equal(enumerated, onPathSet(outcome)) {
		t.Errorf("enumerated %v, want %v", sortedKeys(enumerated), sortedKeys(onPathSet(outcome)))
	}
}

// 2 つの端末のタイムスタンプが交互に並ぶ経路は、1 つのずれの差 δ = o_d - o_c がすべての前後を
// 満たすときだけ成り立つ。c の 0 → d の 10 → c の 20 → d の 35 → c の 30 は δ ∈ [-10, -5] で
// 成り立つ。d の 35 → d の 45 → c の 31 は δ ≤ -14 を要し、c の 0 → d の 10 の δ ≥ -10 と両立しない。
func TestReachManyClocksNeedsOneOffsetForTheWholePath(t *testing.T) {
	const c, d = 0, 1
	a, b := pathEnd{vertex: 0, clock: c, at: 0}, pathEnd{vertex: 4, clock: c, at: 1000}
	steps := []influenceStep{
		pointStep(0, 1, d, 10), // 0
		pointStep(1, 2, c, 20), // 1
		pointStep(2, 3, d, 35), // 2
		pointStep(3, 4, c, 30), // 3: d の 35 の後に c の 30。差 ≤ -5 と、1 の差 ≥ -10 の両方を満たす
		pointStep(3, 5, d, 45), // 4
		pointStep(5, 4, c, 31), // 5: d の 45 の後に c の 31 は差 ≤ -14。0 と 1 の差 ≥ -10 と矛盾
	}
	outcome := reach(6, steps, a, b)
	if got := sortedKeys(onPathSet(outcome)); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Fatalf("G(a, b) = %v, want [0 1 2 3]", got)
	}
	if enumerated := enumeratedPathEdges(steps, a, b, 2, 11); !maps.Equal(enumerated, onPathSet(outcome)) {
		t.Errorf("enumerated %v, want %v", sortedKeys(enumerated), sortedKeys(onPathSet(outcome)))
	}
}

// 同じエッジを 1 つの端末だけの経路と 2 つの端末の経路の両方が通るとき、両方を返す。
func TestReachManyClocksReportsBothKindsOfPathsThroughAnEdge(t *testing.T) {
	const c, d = 0, 1
	a, b := pathEnd{vertex: 0, clock: c, at: 0}, pathEnd{vertex: 2, clock: c, at: 100}
	steps := []influenceStep{
		pointStep(0, 1, c, 10), // 0
		pointStep(1, 2, c, 20), // 1: c だけの経路
		pointStep(1, 3, d, 7),  // 2
		pointStep(3, 2, c, 30), // 3: d を挟む経路
	}
	outcome := reach(4, steps, a, b)
	if clocks := outcome.onPath[0]; !clocks.single || !clocks.mixed {
		t.Errorf("edge 0: %+v, want both", clocks)
	}
	if clocks := outcome.onPath[1]; !clocks.single || clocks.mixed {
		t.Errorf("edge 1: %+v, want only one terminal", clocks)
	}
	if clocks := outcome.onPath[2]; clocks.single || !clocks.mixed {
		t.Errorf("edge 2: %+v, want only two terminals", clocks)
	}
}

// 起点と終点の端末が違うときは、どの経路も 2 つ以上の端末のタイムスタンプを含む。
func TestReachManyClocksTreatsEndsOnTwoTerminalsAsMixed(t *testing.T) {
	a, b := pathEnd{vertex: 0, clock: 0, at: 0}, pathEnd{vertex: 1, clock: 1, at: -50}
	outcome := reach(2, []influenceStep{pointStep(0, 1, 0, 10)}, a, b)
	if clocks, on := outcome.onPath[0]; !on || clocks.single || !clocks.mixed {
		t.Errorf("edge 0: %+v on=%v, want a mixed path", clocks, on)
	}
}

// 状態の数が上限に達したとき、集合を空にして上限に達したことを返す。
func TestReachManyClocksStopsAtTheStateLimit(t *testing.T) {
	saved := maxZoneStates
	maxZoneStates = 0
	t.Cleanup(func() { maxZoneStates = saved })
	a, b := pathEnd{vertex: 0, clock: 0, at: 0}, pathEnd{vertex: 2, clock: 0, at: 100}
	outcome := reach(3, []influenceStep{pointStep(0, 1, 0, 10), pointStep(1, 2, 1, 20)}, a, b)
	if !outcome.limited || len(outcome.onPath) != 0 {
		t.Errorf("limited=%v onPath=%v, want the limit and an empty set", outcome.limited, outcome.onPath)
	}
}

// 小さな入力を無作為に作り、求めた集合が、定義の経路の条件から数え上げた集合と一致する。
// 端末が 1 つの入力は EA と LD の式を、2 つと 3 つの入力は制約の集合の探索を確かめる。
func TestReachMatchesTheEnumeratedSetOnRandomInputs(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	const vertexCount = 4
	// nonEmpty は端末の数ごとの、集合が空でない試行の数である。空の集合ばかりの比較にしない。
	nonEmpty := map[int]int{}
	t.Cleanup(func() {
		for clockCount := 1; clockCount <= 3; clockCount++ {
			if nonEmpty[clockCount] < 10 {
				t.Errorf("%d terminals: %d trials with a path, want at least 10", clockCount, nonEmpty[clockCount])
			}
		}
	})
	for trial := range 400 {
		clockCount := 1 + trial%3
		stepCount := 2 + random.IntN(5)
		steps := make([]influenceStep, stepCount)
		span := func() []clockSpan {
			clock := random.IntN(clockCount)
			lo := int64(random.IntN(10))
			hi := lo + int64(random.IntN(3))
			if random.IntN(6) == 0 {
				lo = noLower
			}
			return []clockSpan{{clock, timeSpan{lo, hi}}}
		}
		for at := range steps {
			steps[at] = influenceStep{from: random.IntN(vertexCount), to: random.IntN(vertexCount), depart: span()}
			if random.IntN(2) == 0 {
				steps[at].arrive = steps[at].depart
			} else {
				steps[at].arrive = span()
			}
		}
		a := pathEnd{vertex: 0, clock: random.IntN(clockCount), at: int64(random.IntN(4))}
		b := pathEnd{vertex: 1 + random.IntN(vertexCount-1), clock: random.IntN(clockCount), at: int64(5 + random.IntN(8))}
		outcome := reach(vertexCount, steps, a, b)
		// 巡回を除いた列は、除く前の列の条件の一部であり、条件を満たし続ける。エッジを通る列は、
		// 起点から s.u、s.v から終点への単純な列でつくれるため、長さ 2(頂点の数 - 1) + 1 まで数える。
		enumerated := enumeratedPathEdges(steps, a, b, clockCount, 2*(vertexCount-1)+1)
		if !maps.Equal(enumerated, onPathSet(outcome)) {
			t.Fatalf("trial %d (%d terminals): G(a, b) = %v, want the enumerated %v\nsteps=%+v a=%+v b=%+v",
				trial, clockCount, sortedKeys(onPathSet(outcome)), sortedKeys(enumerated), steps, a, b)
		}
		if len(enumerated) > 0 {
			nonEmpty[clockCount]++
		}
	}
}
