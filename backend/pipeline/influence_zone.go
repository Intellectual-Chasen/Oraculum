package pipeline

import (
	"slices"
)

// zone は、経路の途中までの制約を満たす、時刻 T と端末ごとのずれ o_C の組の集合である。
// 変数 0 が T、変数 1 から先が端末ごとのずれである。bounds[i*size+j] は x_i - x_j の上限であり、
// noUpper は上限が無いことである。**集合を正規の形 (各上限が他の上限の和で狭まらない形) で持つ。**
//
// 制約はどれも 2 つの変数の差の上限である (T - o_C ∈ 区間、T の前後)。集合が空であることは、
// 差の上限の和が負になる変数の巡回があることと同じである。
type zone struct {
	size   int
	bounds []int64
}

// addBounds は上限の和を返す。どちらかが noUpper のときは noUpper である。
func addBounds(left, right int64) int64 {
	if left == noUpper || right == noUpper {
		return noUpper
	}
	return left + right
}

// newZone は制約の無い集合を返す。
func newZone(size int) zone {
	bounds := make([]int64, size*size)
	for i := range bounds {
		bounds[i] = noUpper
	}
	for i := range size {
		bounds[i*size+i] = 0
	}
	return zone{size: size, bounds: bounds}
}

func (z zone) at(i, j int) int64 { return z.bounds[i*z.size+j] }

// clone は集合を複製する。
func (z zone) clone() zone {
	return zone{size: z.size, bounds: slices.Clone(z.bounds)}
}

// constrain は x_i - x_j ≤ c を足し、集合が空でないかを返す。正規の形を保つ。
func (z zone) constrain(i, j int, c int64) bool {
	if c == noUpper || c >= z.at(i, j) {
		return true
	}
	if back := z.at(j, i); back != noUpper && c+back < 0 {
		return false
	}
	for p := range z.size {
		toI := z.at(p, i)
		if toI == noUpper {
			continue
		}
		for q := range z.size {
			through := addBounds(addBounds(toI, c), z.at(j, q))
			if through < z.at(p, q) {
				z.bounds[p*z.size+q] = through
			}
		}
	}
	return true
}

// within は、変数 T と端末 clock のずれ o について T - o ∈ span を足し、集合が空でないかを返す。
func (z zone) within(clock int, span timeSpan) bool {
	lower := noUpper
	if span.lo != noLower {
		lower = -span.lo
	}
	return z.constrain(0, clock, span.hi) && z.constrain(clock, 0, lower)
}

// later は T を後へ動かせるようにする。{(T, o) : ある T' ≤ T で (T', o) が集合にある}。
func (z zone) later() {
	for j := 1; j < z.size; j++ {
		z.bounds[j] = noUpper
	}
}

// earlier は T を前へ動かせるようにする。{(T, o) : ある T' ≥ T で (T', o) が集合にある}。
func (z zone) earlier() {
	for j := 1; j < z.size; j++ {
		z.bounds[j*z.size] = noUpper
	}
}

// includes は集合 other が z に含まれるかを返す。どちらも正規の形である。
func (z zone) includes(other zone) bool {
	for i, bound := range other.bounds {
		if bound > z.bounds[i] {
			return false
		}
	}
	return true
}

// meets は、2 つの集合の共通部分が空でないかを返す。
func (z zone) meets(other zone) bool {
	joined := z.clone()
	for i := range z.size {
		for j := range z.size {
			if !joined.constrain(i, j, other.at(i, j)) {
				return false
			}
		}
	}
	return true
}

// zoneState は探索の状態であり、要素と、制約の集合と、途中までの経路が起点または終点の端末と
// 別の端末のタイムスタンプを含むか (mixed) である。
type zoneState struct {
	vertex int
	mixed  bool
	zone   zone
}

// zoneStore は、要素と mixed の組ごとに、ほかの集合に含まれない集合を持つ。
type zoneStore struct {
	held  map[[2]int][]zone
	count int
	// expansions は、状態をエッジで進めて得た集合の数である。間引く前の数を数える。
	expansions int
}

func stateKey(vertex int, mixed bool) [2]int {
	if mixed {
		return [2]int{vertex, 1}
	}
	return [2]int{vertex, 0}
}

// add は集合を足し、ほかの集合に含まれない新しい集合であったかを返す。
func (s *zoneStore) add(state zoneState) bool {
	key := stateKey(state.vertex, state.mixed)
	held := s.held[key]
	for _, existing := range held {
		if existing.includes(state.zone) {
			return false
		}
	}
	held = slices.DeleteFunc(held, func(existing zone) bool { return state.zone.includes(existing) })
	s.held[key] = append(held, state.zone)
	s.count++
	return true
}

// reachManyClocks は、タイムスタンプを記録した端末が 2 つ以上のときの G(a, b) を求める。
//
// **端末ごとのずれを上限の無い未知数とする。** 経路の条件は、T^d、T^a、o_C の差の上限の集まり
// である。起点から前へ、終点から後ろへ、経路の途中までの制約を満たす (T, o) の集合を状態として
// 辿り、集合を含み合いで間引く。影響のエッジ s は、s.u までの前からの集合を s で進めた集合と、
// s.v からの後ろからの集合が交わるとき G(a, b) に乗る。同じ端末のタイムスタンプの順序は、ずれが
// 端末ごとに 1 つの未知数であることから保たれる。
//
// 集合の上限は区間の端点の差の和であり、和の項の数は端末の数で抑えられる。集合の種類は有限で
// あり、探索は終わる。
//
// 既知の制限: 端末ごとのずれを上限の無い未知数とし、分析者が端末の組ごとに与えるずれの差の上限 ε を
// 制約に入れない, ε を分析者が与える操作と保存先がまだ無く、上限の無い未知数は経路の条件で ε を
// 与えない組の扱いそのものである, ε を与える操作を足したとき、与えた組の |o_C - o_D| ≤ ε を集合の
// 制約に足し、時刻の根拠に上限のあるずれの差を加える
func reachManyClocks(vertexCount int, steps []influenceStep, a, b pathEnd) reachOutcome {
	clocks := clocksOf(steps, a, b)
	variable := make(map[int]int, len(clocks))
	for index, clock := range clocks {
		variable[clock] = index + 1
	}
	size := len(clocks) + 1
	outgoing := make([][]int, vertexCount)
	incoming := make([][]int, vertexCount)
	for at, step := range steps {
		outgoing[step.from] = append(outgoing[step.from], at)
		incoming[step.to] = append(incoming[step.to], at)
	}
	apply := func(z zone, constraints []clockSpan) bool {
		for _, constraint := range constraints {
			if !z.within(variable[constraint.clock], constraint.span) {
				return false
			}
		}
		return true
	}
	other := func(step influenceStep, clock int) bool {
		return slices.ContainsFunc(slices.Concat(step.depart, step.arrive),
			func(constraint clockSpan) bool { return constraint.clock != clock })
	}
	forwardStep := func(from zone, step influenceStep) (zone, bool) {
		z := from.clone()
		z.later()
		if !apply(z, step.depart) {
			return zone{}, false
		}
		z.later()
		if !apply(z, step.arrive) {
			return zone{}, false
		}
		// 着いた後は時刻を後へ動かせる。遅く着いた集合は早く着いた集合に含まれ、間引かれる。
		z.later()
		return z, true
	}
	backwardStep := func(from zone, step influenceStep) (zone, bool) {
		z := from.clone()
		z.earlier()
		if !apply(z, step.arrive) {
			return zone{}, false
		}
		z.earlier()
		if !apply(z, step.depart) {
			return zone{}, false
		}
		z.earlier()
		return z, true
	}
	outcome := reachOutcome{onPath: map[int]pathClocks{}, blocked: map[int]bool{}}
	start := newZone(size)
	start.within(variable[a.clock], timeSpan{a.at, a.at})
	forward, limited := explore(zoneState{vertex: a.vertex, zone: start}, func(state zoneState, visit func(zoneState)) {
		for _, at := range outgoing[state.vertex] {
			if next, feasible := forwardStep(state.zone, steps[at]); feasible {
				visit(zoneState{steps[at].to, state.mixed || other(steps[at], a.clock), next})
			}
		}
	})
	end := newZone(size)
	end.within(variable[b.clock], timeSpan{noLower, b.at})
	end.earlier()
	backward, backwardLimited := explore(zoneState{vertex: b.vertex, zone: end}, func(state zoneState, visit func(zoneState)) {
		for _, at := range incoming[state.vertex] {
			if next, feasible := backwardStep(state.zone, steps[at]); feasible {
				visit(zoneState{steps[at].from, state.mixed || other(steps[at], b.clock), next})
			}
		}
	})
	outcome.states = forward.count + backward.count
	outcome.expansions = forward.expansions + backward.expansions
	if limited || backwardLimited {
		outcome.limited = true
		return outcome
	}
	for at, step := range steps {
		clocksOnPath := pathClocks{}
		for _, mixedBefore := range []bool{false, true} {
			for _, before := range forward.held[stateKey(step.from, mixedBefore)] {
				advanced, feasible := forwardStep(before, step)
				if !feasible {
					continue
				}
				for _, mixedAfter := range []bool{false, true} {
					for _, after := range backward.held[stateKey(step.to, mixedAfter)] {
						if !advanced.meets(after) {
							continue
						}
						if mixedBefore || mixedAfter || a.clock != b.clock || other(step, a.clock) {
							clocksOnPath.mixed = true
						} else {
							clocksOnPath.single = true
						}
					}
				}
			}
		}
		if clocksOnPath.single || clocksOnPath.mixed {
			outcome.onPath[at] = clocksOnPath
		}
	}
	for vertex := range vertexCount {
		zones := slices.Concat(forward.held[stateKey(vertex, false)], forward.held[stateKey(vertex, true)])
		if len(zones) == 0 {
			continue
		}
		outcome.reached = append(outcome.reached, vertex)
		outcome.blocked[vertex] = len(outgoing[vertex]) > 0 && !slices.ContainsFunc(outgoing[vertex], func(at int) bool {
			return slices.ContainsFunc(zones, func(z zone) bool {
				_, feasible := forwardStep(z, steps[at])
				return feasible
			})
		})
	}
	return outcome
}

// explore は start から expand が渡す状態を辿り、含み合いで間引いた状態の集まりを返す。
// limited は、足した状態の数が maxZoneStates に達したことである。
func explore(start zoneState, expand func(zoneState, func(zoneState))) (zoneStore, bool) {
	store := zoneStore{held: map[[2]int][]zone{}}
	store.add(start)
	queue := []zoneState{start}
	for len(queue) > 0 {
		if store.count > maxZoneStates {
			return store, true
		}
		state := queue[0]
		queue = queue[1:]
		if !slices.ContainsFunc(store.held[stateKey(state.vertex, state.mixed)],
			func(held zone) bool { return held.includes(state.zone) && state.zone.includes(held) }) {
			continue
		}
		expand(state, func(next zoneState) {
			store.expansions++
			if store.add(next) {
				queue = append(queue, next)
			}
		})
	}
	return store, false
}
