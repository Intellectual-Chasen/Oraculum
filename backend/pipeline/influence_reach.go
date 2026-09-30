package pipeline

import (
	"container/heap"
	"slices"
)

// pathEnd は起点または終点であり、要素の番号と、タイムスタンプと、それを記録した端末である。
type pathEnd struct {
	vertex int
	clock  int
	at     int64
}

// reachOutcome は G(a, b) を求めた結果である。
type reachOutcome struct {
	// onPath は G(a, b) に乗る影響のエッジの番号と、そのエッジを通る経路が複数の端末の
	// タイムスタンプを含むか (mixed) である。
	onPath map[int]pathClocks
	// reached は、起点から時刻の条件を満たして届いた要素の番号である。
	reached []int
	// blocked は、reached の要素のうち、出る影響のエッジがどれも時刻の条件を満たさない要素である。
	blocked map[int]bool
	// limited は、探索の状態の数が上限に達したことである。
	limited bool
	// states は、端末が 2 つ以上の入力の探索が前と後ろから足した状態の数の和である。端末が 1 つの
	// 入力では 0 である。
	states int
	// expansions は、端末が 2 つ以上の探索が状態をエッジで進めた回数である。
	expansions int
	// oneClock は、タイムスタンプを記録した端末が 1 つであり、EA と LD で求めたことである。
	oneClock bool
}

// pathClocks は、あるエッジを通る経路が 1 つの端末のタイムスタンプだけのもの (single) と、
// 複数の端末のタイムスタンプを含むもの (mixed) のどちらを持つかである。
type pathClocks struct{ single, mixed bool }

// maxZoneStates は、端末が 2 つ以上の入力で、探索が持つ状態の数の上限である。
//
// 既知の制限: 状態の数が上限に達したら集合を空にし、理由を返す, 状態の数は端末の数と経路の
// 分かれ方で決まり、repo の fixture の大きさでは上限に届かない, 分析者の操作が上限に達したとき、
// そのときの状態の数から上限と計算の方法を見直す
var maxZoneStates = 200_000

// clocksOf は、影響のエッジと起点と終点が含む端末の番号を、重なり無く返す。
func clocksOf(steps []influenceStep, ends ...pathEnd) []int {
	var clocks []int
	add := func(clock int) {
		if !slices.Contains(clocks, clock) {
			clocks = append(clocks, clock)
		}
	}
	for _, end := range ends {
		add(end.clock)
	}
	for _, step := range steps {
		for _, constraint := range slices.Concat(step.depart, step.arrive) {
			add(constraint.clock)
		}
	}
	return clocks
}

// reach は起点 a から終点 b への G(a, b) を求める。vertexCount は要素の数である。
//
// タイムスタンプを記録した端末が 1 つのときは、EA と LD で求める (reachOneClock)。2 つ以上の
// ときは、端末ごとのずれを上限の無い未知数として、経路の条件の制約の集合を状態に持って求める
// (reachManyClocks)。
//
// **数える端末は、時刻を問わずに起点から前へ届き、かつ終点から後ろへ届くエッジの端末である。**
// ほかのエッジはどの経路にも乗らず、EA と LD で G(a, b) を求めるときに値を変えない。
func reach(vertexCount int, steps []influenceStep, a, b pathEnd) reachOutcome {
	if len(clocksOf(stepsBetween(vertexCount, steps, a.vertex, b.vertex), a, b)) == 1 {
		outcome := reachOneClock(vertexCount, steps, a, b)
		outcome.oneClock = true
		return outcome
	}
	return reachManyClocks(vertexCount, steps, a, b)
}

// stepsBetween は、時刻を問わずに要素 from から前へ届き、かつ要素 to へ後ろから届くエッジを返す。
func stepsBetween(vertexCount int, steps []influenceStep, from, to int) []influenceStep {
	outgoing := make([][]int, vertexCount)
	incoming := make([][]int, vertexCount)
	for at, step := range steps {
		outgoing[step.from] = append(outgoing[step.from], at)
		incoming[step.to] = append(incoming[step.to], at)
	}
	walk := func(start int, next [][]int, end func(influenceStep) int) []bool {
		seen := make([]bool, vertexCount)
		seen[start] = true
		queue := []int{start}
		for len(queue) > 0 {
			vertex := queue[0]
			queue = queue[1:]
			for _, at := range next[vertex] {
				if other := end(steps[at]); !seen[other] {
					seen[other] = true
					queue = append(queue, other)
				}
			}
		}
		return seen
	}
	forward := walk(from, outgoing, func(step influenceStep) int { return step.to })
	backward := walk(to, incoming, func(step influenceStep) int { return step.from })
	var between []influenceStep
	for _, step := range steps {
		if forward[step.from] && backward[step.to] {
			between = append(between, step)
		}
	}
	return between
}

// merged は、1 つの端末の制約の区間の共通部分を返す。
func merged(constraints []clockSpan) timeSpan {
	span := timeSpan{noLower, noUpper}
	for _, constraint := range constraints {
		span = span.intersect(constraint.span)
	}
	return span
}

// reachOneClock は、タイムスタンプを記録した端末が 1 つのときの G(a, b) を、EA と LD から求める。
//
// EA(v) は a から v へ届く最も早い時刻、LD(u) は u から b へ届くために u が影響を受けてよい最も
// 遅い時刻である。EA(a) = t_a と LD(b) = t_b に固定する。どちらも一方向にだけ動く値を、値の
// 順に確定して求める。
func reachOneClock(vertexCount int, steps []influenceStep, a, b pathEnd) reachOutcome {
	outgoing := make([][]int, vertexCount)
	incoming := make([][]int, vertexCount)
	departs := make([]timeSpan, len(steps))
	arrives := make([]timeSpan, len(steps))
	for at, step := range steps {
		outgoing[step.from] = append(outgoing[step.from], at)
		incoming[step.to] = append(incoming[step.to], at)
		departs[at], arrives[at] = merged(step.depart), merged(step.arrive)
	}
	earliest := settle(vertexCount, a, func(u int, value int64, relax func(int, int64)) {
		for _, at := range outgoing[u] {
			x := max(value, departs[at].lo)
			y := max(x, arrives[at].lo)
			if x <= departs[at].hi && y <= arrives[at].hi {
				relax(steps[at].to, y)
			}
		}
	}, func(candidate, held int64) bool { return candidate < held })
	latest := settle(vertexCount, b, func(v int, value int64, relax func(int, int64)) {
		for _, at := range incoming[v] {
			y := min(value, arrives[at].hi)
			z := min(y, departs[at].hi)
			if arrives[at].lo <= y && departs[at].lo <= z {
				relax(steps[at].from, z)
			}
		}
	}, func(candidate, held int64) bool { return candidate > held })
	outcome := reachOutcome{onPath: map[int]pathClocks{}, blocked: map[int]bool{}}
	for at, step := range steps {
		ea, reachedFrom := earliest[step.from]
		ld, reachesTo := latest[step.to]
		if !reachedFrom || !reachesTo {
			continue
		}
		x, y := max(ea, departs[at].lo), min(ld, arrives[at].hi)
		if x <= departs[at].hi && arrives[at].lo <= y && x <= y {
			outcome.onPath[at] = pathClocks{single: true}
		}
	}
	for vertex := range vertexCount {
		value, reached := earliest[vertex]
		if !reached {
			continue
		}
		outcome.reached = append(outcome.reached, vertex)
		outcome.blocked[vertex] = len(outgoing[vertex]) > 0 && !slices.ContainsFunc(outgoing[vertex], func(at int) bool {
			x := max(value, departs[at].lo)
			return x <= departs[at].hi && max(x, arrives[at].lo) <= arrives[at].hi
		})
	}
	return outcome
}

// settle は、端 end の値を固定し、expand が relax で渡す値のうち better が選ぶ値を、値の順に
// 確定する。返す表は届いた要素だけを持つ。
func settle(
	vertexCount int, end pathEnd, expand func(int, int64, func(int, int64)), better func(int64, int64) bool,
) map[int]int64 {
	values := map[int]int64{end.vertex: end.at}
	done := make([]bool, vertexCount)
	queue := &valueQueue{better: better}
	heap.Push(queue, valueItem{end.vertex, end.at})
	for queue.Len() > 0 {
		item := heap.Pop(queue).(valueItem)
		if done[item.vertex] || item.value != values[item.vertex] {
			continue
		}
		done[item.vertex] = true
		expand(item.vertex, item.value, func(vertex int, value int64) {
			if vertex == end.vertex || done[vertex] {
				return
			}
			if held, found := values[vertex]; found && !better(value, held) {
				return
			}
			values[vertex] = value
			heap.Push(queue, valueItem{vertex, value})
		})
	}
	return values
}

// earliestRoute は、タイムスタンプを記録した端末が 1 つの入力で、候補のエッジ candidates (steps の
// 位置) だけを通って起点 a から終点 b へ最も早く届く列を、起点の側から並べて返す。緩和は
// reachOneClock の EA と同じである。列の各エッジは時刻の条件を満たし、終点に着く時刻は b の
// タイムスタンプ以下である。届かないときと、起点と終点が同じ要素のときは nil を返す。
func earliestRoute(steps []influenceStep, candidates []int, a, b pathEnd) []int {
	if a.vertex == b.vertex {
		return nil
	}
	outgoing := map[int][]int{}
	for _, index := range candidates {
		outgoing[steps[index].from] = append(outgoing[steps[index].from], index)
	}
	earliest := map[int]int64{a.vertex: a.at}
	via := map[int]int{}
	done := map[int]bool{}
	queue := &valueQueue{better: func(candidate, held int64) bool { return candidate < held }}
	heap.Push(queue, valueItem{a.vertex, a.at})
	for queue.Len() > 0 && !done[b.vertex] {
		item := heap.Pop(queue).(valueItem)
		if done[item.vertex] || item.value != earliest[item.vertex] {
			continue
		}
		done[item.vertex] = true
		for _, index := range outgoing[item.vertex] {
			step := steps[index]
			depart, arrive := merged(step.depart), merged(step.arrive)
			x := max(item.value, depart.lo)
			y := max(x, arrive.lo)
			if x > depart.hi || y > arrive.hi || done[step.to] {
				continue
			}
			if held, found := earliest[step.to]; found && held <= y {
				continue
			}
			earliest[step.to], via[step.to] = y, index
			heap.Push(queue, valueItem{step.to, y})
		}
	}
	if !done[b.vertex] || earliest[b.vertex] > b.at {
		return nil
	}
	// via は確定した要素からだけ張るため、終点から辿ると起点で止まる。
	var route []int
	for vertex := b.vertex; vertex != a.vertex; vertex = steps[via[vertex]].from {
		route = append(route, via[vertex])
	}
	slices.Reverse(route)
	return route
}

// valueItem と valueQueue は settle の優先度付きの待ち行列である。
type valueItem struct {
	vertex int
	value  int64
}

type valueQueue struct {
	items  []valueItem
	better func(int64, int64) bool
}

func (q *valueQueue) Len() int { return len(q.items) }
func (q *valueQueue) Less(i, j int) bool {
	if q.items[i].value == q.items[j].value {
		return q.items[i].vertex < q.items[j].vertex
	}
	return q.better(q.items[i].value, q.items[j].value)
}
func (q *valueQueue) Swap(i, j int) { q.items[i], q.items[j] = q.items[j], q.items[i] }
func (q *valueQueue) Push(item any) { q.items = append(q.items, item.(valueItem)) }
func (q *valueQueue) Pop() any {
	last := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return last
}
