// in-package test: 起点と終点のノードから影響の経路の集合を求める要求を確かめる。
package pipeline

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// pathSummary は影響の経路の結果を、影響のエッジの「元>先 時刻の根拠」と要素の「鍵 数」で返す。
func pathSummary(path InfluencePath) (edges, vertices []string) {
	for _, edge := range path.Edges {
		line := edge.SourceKey + ">" + edge.TargetKey
		for _, basis := range edge.TimeBases {
			line += " " + string(basis)
		}
		edges = append(edges, line)
	}
	for _, vertex := range path.Vertices {
		vertices = append(vertices, vertex.Key+" "+string(rune('0'+vertex.InfluenceEdgeCount)))
	}
	return edges, vertices
}

// writeReadGraph は、writer がファイルへ書き、reader がそのファイルを読むグラフを組む。
// 書き込みと読み込みのレコードの時刻と項目は引数が決める。
func writeReadGraph(t *testing.T, write, read int, writeOptions, readOptions []recordOption) *fakeGraph {
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	file := f.node(core.NodeKindFile, "file")
	written := f.record(write, append([]recordOption{byProcess(writer), withFlow(core.FlowOperationWrite)}, writeOptions...)...)
	readAt := f.record(read, append([]recordOption{byProcess(reader), withFlow(core.FlowOperationRead)}, readOptions...)...)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, written)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file, readAt)
	return f
}

func requirePath(t *testing.T, f *fakeGraph, query InfluencePathQuery) InfluencePath {
	t.Helper()
	path, found := f.g.InfluencePath(query)
	if !found {
		t.Fatalf("InfluencePath(%+v) found no node", query)
	}
	return path
}

// 書いたプロセスから読んだプロセスへの経路は、書き込みと読み込みの影響のエッジを通る。起点は
// ノードの最初のレコード、終点は最後のレコードのタイムスタンプである。各要素はその要素を端に持つ
// 影響のエッジの数を持つ。
func TestInfluencePathFollowsTheWriteAndTheRead(t *testing.T) {
	f := writeReadGraph(t, 10, 20, nil, nil)
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, vertices := pathSummary(path)
	if want := []string{"n:writer>n:file same_terminal", "n:file>n:reader same_terminal"}; !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
	if want := []string{"n:writer 1", "n:file 2", "n:reader 1"}; !slices.Equal(vertices, want) {
		t.Errorf("vertices = %v, want %v", vertices, want)
	}
	if path.Origin.Key != "n:writer" || path.Destination.Key != "n:reader" || len(path.Stops) != 0 {
		t.Errorf("origin=%q destination=%q stops=%v", path.Origin.Key, path.Destination.Key, path.Stops)
	}
	for _, edge := range path.Edges {
		if err := edge.Validate(); err != nil {
			t.Errorf("edge %s: %v", edge.Id, err)
		}
	}
}

// 読んだ後に書いたときは経路にならず、辿れるが時刻の条件を満たさないことを理由にする。逆向きの
// 終点へは辿れない。
func TestInfluencePathTellsWhyTheSetIsEmpty(t *testing.T) {
	f := writeReadGraph(t, 30, 20, nil, nil)
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	if len(path.Edges) != 0 || !slices.Equal(path.Stops, []core.InfluencePathStop{core.InfluencePathStopTimeOrderUnsatisfied}) {
		t.Errorf("edges=%v stops=%v, want the time order", path.Edges, path.Stops)
	}
	back := requirePath(t, f, InfluencePathQuery{From: "n:reader", To: "n:writer"})
	if !slices.Equal(back.Stops, []core.InfluencePathStop{core.InfluencePathStopNoInfluenceRoute}) {
		t.Errorf("stops=%v, want no route", back.Stops)
	}
	if _, found := f.g.InfluencePath(InfluencePathQuery{From: "n:writer", To: "n:absent"}); found {
		t.Error("an unknown node was found")
	}
}

// 起点または終点のノードがタイムスタンプを持つレコードを持たないときは、経路を求めずに理由を返す。
func TestInfluencePathNeedsTheTimestampsOfTheEnds(t *testing.T) {
	f := writeReadGraph(t, 10, 20, []recordOption{untimed()}, []recordOption{untimed()})
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	want := []core.InfluencePathStop{
		core.InfluencePathStopOriginWithoutTimestamp, core.InfluencePathStopDestinationWithoutTimestamp,
	}
	if !slices.Equal(path.Stops, want) || path.Origin != nil || path.Destination != nil {
		t.Errorf("stops=%v origin=%v destination=%v, want %v", path.Stops, path.Origin, path.Destination, want)
	}
}

// frontierSummary は、先へ影響が進まないノードを「鍵 理由」で返す。
func frontierSummary(path InfluencePath) []string {
	var lines []string
	for _, item := range path.Frontier {
		lines = append(lines, item.Key+" "+string(item.Reason))
	}
	return lines
}

// 選んだ根拠の種類を持つ影響のエッジを除いて求め直す。除いた種類を戻すと経路があるときは、その
// 種類を理由に添える。
func TestInfluencePathExcludesTheSelectedBases(t *testing.T) {
	f := writeReadGraph(t, 10, 20, nil, []recordOption{inferredKind()})
	path := requirePath(t, f, InfluencePathQuery{
		From: "n:writer", To: "n:reader",
		Excluded: []core.InfluenceBasis{core.InfluenceBasisUncertainChain, core.InfluenceBasisInferredRecord},
	})
	wantStops := []core.InfluencePathStop{
		core.InfluencePathStopNoInfluenceRoute, core.InfluencePathStopRouteThroughExcludedBasis,
	}
	if len(path.Edges) != 0 || !slices.Equal(path.Stops, wantStops) {
		t.Errorf("edges=%v stops=%v, want %v", path.Edges, path.Stops, wantStops)
	}
	if !slices.Equal(path.RouteExcludedBases, []core.InfluenceBasis{core.InfluenceBasisInferredRecord}) {
		t.Errorf("route excluded bases = %v, want the inferred record", path.RouteExcludedBases)
	}
	if _, vertices := pathSummary(path); !slices.Equal(vertices, []string{"n:writer 1", "n:reader 0"}) {
		t.Errorf("vertices = %v, want the counts after the exclusion", vertices)
	}
	if got, want := frontierSummary(path), []string{"n:file outgoing_excluded"}; !slices.Equal(got, want) {
		t.Errorf("frontier = %v, want %v", got, want)
	}
	if path.Frontier[0].Node.Id != "n:file" || path.Frontier[0].InfluenceEdgeCount != 1 {
		t.Errorf("frontier vertex = %+v, want the file with 1 edge", path.Frontier[0].InfluenceVertex)
	}
	// 除いた種類を戻しても経路が無いときは、理由を添えない。
	back := requirePath(t, f, InfluencePathQuery{
		From: "n:reader", To: "n:writer", Excluded: []core.InfluenceBasis{core.InfluenceBasisInferredRecord},
	})
	if !slices.Equal(back.Stops, []core.InfluencePathStop{core.InfluencePathStopNoInfluenceRoute}) || back.RouteExcludedBases != nil {
		t.Errorf("stops=%v bases=%v, want no route only", back.Stops, back.RouteExcludedBases)
	}
}

// 除いた種類を戻して求め直す計算だけが状態の上限に達したときは、そのことを理由にする。上限に達しない
// ときの状態の数は、求め直した計算の状態の数を含む。
func TestInfluencePathTellsTheLimitOfTheExcludedBasisRoute(t *testing.T) {
	f := writeReadGraph(t, 10, 20, []recordOption{onTerminal("n:one")},
		[]recordOption{onTerminal("n:two"), inferredKind()})
	query := InfluencePathQuery{From: "n:writer", To: "n:reader", Excluded: []core.InfluenceBasis{core.InfluenceBasisInferredRecord}}
	saved := maxZoneStates
	t.Cleanup(func() { maxZoneStates = saved })
	limitedRetry := 0
	for limit := 1; limit < 200 && limitedRetry == 0; limit++ {
		maxZoneStates = limit
		path := requirePath(t, f, query)
		if slices.Contains(path.Stops, core.InfluencePathStopComputationLimit) {
			continue
		}
		if slices.Contains(path.Stops, core.InfluencePathStopExcludedBasisRouteLimit) {
			limitedRetry = limit
			if slices.Contains(path.Stops, core.InfluencePathStopRouteThroughExcludedBasis) || path.RouteExcludedBases != nil {
				t.Errorf("limit %d: stops=%v bases=%v, want no route claimed", limit, path.Stops, path.RouteExcludedBases)
			}
		}
	}
	if limitedRetry == 0 {
		t.Fatal("no limit stopped only the computation with the excluded bases restored")
	}
	maxZoneStates = saved
	path := requirePath(t, f, query)
	if !slices.Contains(path.Stops, core.InfluencePathStopRouteThroughExcludedBasis) || path.StateCount <= limitedRetry {
		t.Errorf("stops=%v states=%d, want the route and more than %d states of the restored computation",
			path.Stops, path.StateCount, limitedRetry)
	}
}

// 精度が秒のタイムスタンプは区間を両端で 1 秒広げ、広げたときにだけ成り立つ経路の影響のエッジは
// 精度の幅の種類を持つ。
func TestInfluencePathWidensTheSecondPrecision(t *testing.T) {
	f := writeReadGraph(t, 11, 10, []recordOption{secondPrecise()}, []recordOption{secondPrecise()})
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, _ := pathSummary(path)
	want := []string{"n:writer>n:file same_terminal precision_width", "n:file>n:reader same_terminal precision_width"}
	if !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
}

// 2 つの端末のタイムスタンプを含む経路は、ずれの差の上限の無い種類を持つ。
func TestInfluencePathMarksPathsAcrossTerminals(t *testing.T) {
	f := writeReadGraph(t, 10, 5, []recordOption{onTerminal("n:one")}, []recordOption{onTerminal("n:two")})
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, _ := pathSummary(path)
	want := []string{"n:writer>n:file unbounded_offset", "n:file>n:reader unbounded_offset"}
	if !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
}

// 起点から届いたが先へ影響が進まないノードを、出るエッジが無いことと、時刻の条件を満たさない
// ことに分けて返す。
func TestInfluencePathReportsTheFrontier(t *testing.T) {
	f := writeReadGraph(t, 10, 20, nil, nil)
	writer := f.g.nodeAt["n:writer"]
	dead, late := f.node(core.NodeKindFile, "dead"), f.node(core.NodeKindFile, "late")
	after := f.node(core.NodeKindProcess, "after")
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, dead,
		f.record(12, byProcess(writer), withFlow(core.FlowOperationWrite)))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, late,
		f.record(13, byProcess(writer), withFlow(core.FlowOperationWrite)))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, after, late,
		f.record(11, byProcess(after), withFlow(core.FlowOperationRead)))
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	want := []string{"n:dead no_outgoing_influence", "n:late outgoing_time_unsatisfied"}
	if got := frontierSummary(path); !slices.Equal(got, want) || path.FrontierCount != 2 {
		t.Errorf("frontier = %v (%d), want %v", got, path.FrontierCount, want)
	}
	// 先へ影響が進まないノードは経路のエッジの端でなく、経路の要素に入れない。
	if _, vertices := pathSummary(path); !slices.Equal(vertices, []string{"n:writer 3", "n:file 2", "n:reader 1"}) {
		t.Errorf("vertices = %v, want only the ends of the edges", vertices)
	}
}

// 起点のファイルは、起点のタイムスタンプの内容のバージョンの要素である。
func TestInfluencePathStartsAtTheContentVersionOfTheOrigin(t *testing.T) {
	f := writeReadGraph(t, 10, 20, []recordOption{replacing()}, nil)
	path := requirePath(t, f, InfluencePathQuery{From: "n:file", To: "n:reader"})
	edges, _ := pathSummary(path)
	if want := []string{"n:file@2001-02-03T04:00:10Z>n:reader same_terminal"}; !slices.Equal(edges, want) {
		t.Errorf("edges = %v, want %v", edges, want)
	}
	if path.Origin.Key != "n:file@2001-02-03T04:00:10Z" || path.Vertices[0].VersionStart != "2001-02-03T04:00:10Z" {
		t.Errorf("origin = %q vertex = %+v", path.Origin.Key, path.Vertices[0])
	}
}

// UTC からのずれの分からないタイムスタンプは、収集元ごとに別の端末が記録したタイムスタンプとして
// 読む。起点と終点のレコードに使え、経路は上限の無いずれの差を持つ。
func TestInfluencePathReadsTheLocalClockAsTheSourceTerminal(t *testing.T) {
	f := writeReadGraph(t, 10, 20, []recordOption{localClock()}, []recordOption{localClock()})
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, _ := pathSummary(path)
	want := []string{"n:writer>n:file same_terminal", "n:file>n:reader same_terminal"}
	if !slices.Equal(edges, want) || len(path.Stops) != 0 || path.UntimedRecordCount != 0 {
		t.Errorf("edges=%v stops=%v untimed=%d, want %v", edges, path.Stops, path.UntimedRecordCount, want)
	}
	mixed := writeReadGraph(t, 10, 5, []recordOption{localClock()}, nil)
	mixedPath := requirePath(t, mixed, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	if edges, _ := pathSummary(mixedPath); !slices.Equal(edges,
		[]string{"n:writer>n:file unbounded_offset", "n:file>n:reader unbounded_offset"}) {
		t.Errorf("edges across the local and the UTC clocks = %v", edges)
	}
}

// 起点のノードの最初のレコードは、UTC の時点を持つレコードから選ぶ。UTC からのずれの分からない
// レコードの壁時計の日時は、UTC の時点と前後を比べない。
func TestInfluencePathChoosesTheEndsAmongComparableTimestamps(t *testing.T) {
	f := writeReadGraph(t, 10, 5, []recordOption{replacing()}, []recordOption{localClock()})
	path := requirePath(t, f, InfluencePathQuery{From: "n:file", To: "n:reader"})
	if path.Origin == nil || path.Origin.Key != "n:file@2001-02-03T04:00:10Z" ||
		path.Origin.Record.RecordRef.RecordRawTextRef != "raw:0" {
		t.Fatalf("origin = %+v, want the write with the UTC timestamp", path.Origin)
	}
	if edges, _ := pathSummary(path); !slices.Equal(edges, []string{"n:file@2001-02-03T04:00:10Z>n:reader unbounded_offset"}) {
		t.Errorf("edges = %v, want the local read of the replaced version", edges)
	}
}

// タイムスタンプを持たないレコードは、両方の向きの影響のエッジを作らなくても 1 件と数える。
func TestInfluencePathCountsEachUntimedRecordOnce(t *testing.T) {
	f := writeReadGraph(t, 10, 20, nil, nil)
	writer, file := f.g.nodeAt["n:writer"], f.g.nodeAt["n:file"]
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, f.record(0, byProcess(writer), untimed()))
	if path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"}); path.UntimedRecordCount != 1 {
		t.Errorf("untimed records = %d, want 1", path.UntimedRecordCount)
	}
}

// 同じ Id の影響のエッジを 1 本にしてから、各要素の影響のエッジの数を数える。
func TestInfluencePathCountsTheDegreeAfterMergingTheSameEdges(t *testing.T) {
	f := newFakeGraph(t)
	destination, process := f.node(core.NodeKindDomain, "destination"), f.node(core.NodeKindProcess, "p")
	origin, candidate := f.record(10), f.record(12, byProcess(process))
	match := f.edge(core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate, destination, process, origin, candidate)
	f.g.matchStages = []matchStage{{origin: int32(origin)}, {origin: int32(origin)}}
	f.g.edges[match].ensureBasis().matches = []candidateMatch{
		{candidate: int32(candidate), stage: 0}, {candidate: int32(candidate), stage: 1},
	}
	f.g.nodes[f.g.records[origin].recordNode].evidence = []int{origin}
	path := requirePath(t, f, InfluencePathQuery{
		From: "n:p", To: "n:r0", Excluded: []core.InfluenceBasis{core.InfluenceBasisRequestedDestination},
	})
	if _, vertices := pathSummary(path); !slices.Equal(vertices, []string{"n:p 2", "n:r0 2"}) {
		t.Errorf("vertices = %v, want each end of the two merged edges counted once", vertices)
	}
}

// 影響のエッジを持たないレコードを端に選んだときは、そのレコードが記録したプロセスに読み替える。
func TestInfluencePathReadsARecordEndAsItsProcess(t *testing.T) {
	f := newFakeGraph(t)
	parent, child := f.node(core.NodeKindProcess, "parent"), f.node(core.NodeKindProcess, "child")
	f.g.nodes[parent].evidence = []int{f.record(5, byProcess(parent))}
	start := f.record(10, byProcess(child))
	f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, parent, child, start)
	path := requirePath(t, f, InfluencePathQuery{From: "n:parent", To: "n:r1"})
	edges, _ := pathSummary(path)
	if !slices.Equal(edges, []string{"n:parent>n:child same_terminal"}) || path.Destination == nil ||
		path.Destination.Key != "n:child" || path.Destination.Record.RecordRef.RecordRawTextRef != "raw:1" {
		t.Errorf("edges=%v destination=%+v stops=%v, want the started process as the destination", edges, path.Destination, path.Stops)
	}
}

// 影響のエッジの端にならないノードを選んだときは、たどれないことと分けて理由を返す。
func TestInfluencePathTellsTheEndsWithoutInfluence(t *testing.T) {
	f := writeReadGraph(t, 10, 20, nil, nil)
	address, terminal := f.node(core.NodeKindIp, "address"), f.node(core.NodeKindTerminal, "terminal")
	f.edge(core.EdgeKindTerminalAddress, core.RelationStateObserved, terminal, address, f.record(15))
	path := requirePath(t, f, InfluencePathQuery{From: "n:address", To: "n:terminal"})
	want := []core.InfluencePathStop{
		core.InfluencePathStopOriginNotInfluenceEnd, core.InfluencePathStopDestinationNotInfluenceEnd,
	}
	if !slices.Equal(path.Stops, want) || len(path.Edges) != 0 {
		t.Errorf("stops = %v, want %v", path.Stops, want)
	}
	// 端にならないノードの種別を添える。
	if path.OriginKind != core.NodeKindIp || path.DestinationKind != core.NodeKindTerminal {
		t.Errorf("origin kind=%q destination kind=%q, want ip and terminal", path.OriginKind, path.DestinationKind)
	}
	if reached := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:address"}); !slices.Equal(reached.Stops,
		[]core.InfluencePathStop{core.InfluencePathStopDestinationNotInfluenceEnd}) {
		t.Errorf("stops = %v, want only the destination", reached.Stops)
	}
}

// 不確定の連鎖の影響のエッジは、同じ先の要素に入る不確定の連鎖の関係の数と、候補の並びの件数と、
// 並びの区分を決めた条件を持つ。
func TestInfluencePathCountsTheUncertainChainsIntoTheSameLogon(t *testing.T) {
	f := newFakeGraph(t)
	logon := f.record(50)
	for index, start := range []int{f.record(10), f.record(20)} {
		chain := f.edge(core.EdgeKindLogonChain, core.RelationStateUncertainChain,
			f.g.records[start].recordNode, f.g.records[logon].recordNode, start, logon)
		f.g.edges[chain].ensureBasis().sessions = []sessionSpan{{
			start: f.g.records[start].instant, end: f.g.records[logon].instant, startAt: start, endAt: start, target: logon,
			candidateTally: candidateTally{candidates: 2, preceding: uint16(1 - index)},
		}}
		f.g.addRecordPair(chain, logonChainRule(sessionStartLogon, sessionEndLogoff, index == 1, sessionLogonNetwork), start, logon)
	}
	path := requirePath(t, f, InfluencePathQuery{From: "n:r1", To: "n:r0"})
	if len(path.Edges) != 1 || path.Edges[0].CandidateCount != 2 {
		t.Fatalf("edges = %+v, want one uncertain chain among 2", path.Edges)
	}
	edge := path.Edges[0]
	if edge.CandidateTally == nil || *edge.CandidateTally != (core.EdgeCandidateTally{CandidateCount: 2, PrecedingCandidateCount: 1}) {
		t.Errorf("tally = %+v, want 1 of 2 candidates above", edge.CandidateTally)
	}
	want := []core.EdgePairConditionKey{core.EdgePairConditionSessionAccountDifferent, core.EdgePairConditionSessionLogonNetwork}
	if !slices.Equal(edge.CandidateOrder, want) {
		t.Errorf("candidate order = %v, want %v", edge.CandidateOrder, want)
	}
	if err := edge.Validate(); err != nil {
		t.Error(err)
	}
}

// 上限より多い影響のエッジは、同じ長さの列のエッジのうち起点に近い順に上限まで返し、残りの本数を返す。
func TestInfluencePathKeepsTheEdgesNearTheOrigin(t *testing.T) {
	saved := maxInfluenceEdges
	t.Cleanup(func() { maxInfluenceEdges = saved })
	maxInfluenceEdges = 1
	f := writeReadGraph(t, 10, 20, nil, nil)
	f.g.edges[0], f.g.edges[1] = f.g.edges[1], f.g.edges[0]
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, vertices := pathSummary(path)
	if !slices.Equal(edges, []string{"n:writer>n:file same_terminal"}) || path.OmittedEdgeCount != 1 {
		t.Errorf("edges=%v omitted=%d, want the write near the origin", edges, path.OmittedEdgeCount)
	}
	if !slices.Equal(vertices, []string{"n:file 2", "n:reader 1", "n:writer 1"}) {
		t.Errorf("vertices = %v", vertices)
	}
}

// 上限で切ったときに先に置く列は、時刻の条件を満たす列である。時刻を問わない最短の列
// (w が 10 秒に書いた f2 を、r が 5 秒に読む) を先に置かない。
func TestInfluencePathKeepsARouteThatSatisfiesTheTimeOrder(t *testing.T) {
	saved := maxInfluenceEdges
	t.Cleanup(func() { maxInfluenceEdges = saved })
	f := newFakeGraph(t)
	processes := map[string]int{}
	for _, name := range []string{"w", "p", "p2", "r"} {
		processes[name] = f.node(core.NodeKindProcess, name)
	}
	files := map[string]int{}
	for _, name := range []string{"f1", "f2", "f3"} {
		files[name] = f.node(core.NodeKindFile, name)
	}
	f.g.nodes[processes["w"]].evidence = []int{f.record(1, byProcess(processes["w"]))}
	for _, operation := range []struct {
		process, file string
		seconds       int
		flow          core.FlowOperation
	}{
		{"w", "f1", 2, core.FlowOperationWrite}, {"p", "f1", 3, core.FlowOperationRead},
		{"p", "f2", 4, core.FlowOperationWrite}, {"r", "f2", 5, core.FlowOperationRead},
		{"w", "f2", 10, core.FlowOperationWrite}, {"p2", "f2", 11, core.FlowOperationRead},
		{"p2", "f3", 12, core.FlowOperationWrite}, {"r", "f3", 13, core.FlowOperationRead},
	} {
		process := processes[operation.process]
		f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, process, files[operation.file],
			f.record(operation.seconds, byProcess(process), withFlow(operation.flow)))
	}
	for limit := 2; limit <= 4; limit++ {
		maxInfluenceEdges = limit
		path := requirePath(t, f, InfluencePathQuery{From: "n:w", To: "n:r"})
		// 返したエッジだけで、起点から終点へ時刻の条件を満たして届くかを、根拠のレコードの時刻で確かめる。
		arrived := map[string]int64{"n:w": 1}
		for changed := true; changed; {
			changed = false
			for _, edge := range path.Edges {
				at, reached := arrived[edge.SourceKey]
				record, _ := strconv.Atoi(strings.TrimPrefix(edge.Evidence[0].RecordRef.RecordRawTextRef, "raw:"))
				instant, _ := f.g.influenceInstant(record)
				when := int64(instant.Sub(influenceBase).Seconds())
				if held, found := arrived[edge.TargetKey]; reached && at <= when && (!found || when < held) {
					arrived[edge.TargetKey], changed = when, true
				}
			}
		}
		edges, _ := pathSummary(path)
		// 時刻の条件を満たす最も早い列は 4 本である。上限が 4 本より少ないときは、列の起点の側の
		// 一部を返し、経路が成り立つと確かめられないことを理由に出す。
		_, reached := arrived["n:r"]
		unverified := slices.Contains(path.Stops, core.InfluencePathStopTruncatedRouteUnverified)
		if reached != (limit >= 4) || unverified == reached || len(arrived) != limit+1 || path.OmittedEdgeCount != 8-limit {
			t.Errorf("limit %d: edges=%v stops=%v omitted=%d, want the route in the time order from the origin",
				limit, edges, path.Stops, path.OmittedEdgeCount)
		}
	}
}

// 2 つの端末のタイムスタンプを含む入力で上限で切ったときは、返したエッジで経路が成り立つと
// 確かめられないことを理由に出す。上限に収まるときは出さない。
func TestInfluencePathTellsTheUnverifiedTruncationAcrossTerminals(t *testing.T) {
	saved := maxInfluenceEdges
	t.Cleanup(func() { maxInfluenceEdges = saved })
	f := writeReadGraph(t, 10, 5, []recordOption{onTerminal("n:one")}, []recordOption{onTerminal("n:two")})
	for limit, want := range map[int]bool{1: true, 2: false} {
		maxInfluenceEdges = limit
		path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
		if got := slices.Contains(path.Stops, core.InfluencePathStopTruncatedRouteUnverified); got != want {
			t.Errorf("limit %d: stops = %v, want the unverified truncation %v", limit, path.Stops, want)
		}
	}
}

// 起点から扇状に分かれる経路が上限を超えても、起点から終点への短い経路のエッジを先に返し、
// 終点に入るエッジを落とさない。
func TestInfluencePathKeepsAShortRouteToTheDestination(t *testing.T) {
	saved := maxInfluenceEdges
	t.Cleanup(func() { maxInfluenceEdges = saved })
	maxInfluenceEdges = 3
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	f.g.nodes[writer].evidence = []int{f.record(1, byProcess(writer))}
	for index := range 4 {
		file := f.node(core.NodeKindFile, "file"+string(rune('a'+index)))
		f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file,
			f.record(10, byProcess(writer), withFlow(core.FlowOperationWrite)))
		f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file,
			f.record(20, byProcess(reader), withFlow(core.FlowOperationRead)))
	}
	path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
	edges, _ := pathSummary(path)
	if !slices.ContainsFunc(edges, func(edge string) bool { return strings.HasPrefix(edge, "n:filea>n:reader") }) ||
		!slices.Contains(edges, "n:writer>n:filea same_terminal") || path.OmittedEdgeCount != 5 {
		t.Errorf("edges=%v omitted=%d, want a whole route to the destination", edges, path.OmittedEdgeCount)
	}
}

// 区間を広げない計算だけが状態の上限に達したときは、精度の幅の種類を付けず、求めきれなかった
// ことを理由にする。
func TestInfluencePathReportsTheLimitOfTheExactComputation(t *testing.T) {
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	file := f.node(core.NodeKindFile, "file")
	writes := []int{f.record(10, byProcess(writer), withFlow(core.FlowOperationWrite), secondPrecise())}
	for ms := 1; ms <= 50; ms++ {
		writes = append(writes, f.record(10, byProcess(writer), withFlow(core.FlowOperationWrite), plusMillis(ms)))
	}
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, writes...)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file,
		f.record(20, byProcess(reader), withFlow(core.FlowOperationRead), onTerminal("n:two")))
	saved := maxZoneStates
	t.Cleanup(func() { maxZoneStates = saved })
	var limitedOnlyExact bool
	for limit := 1; limit < 200 && !limitedOnlyExact; limit++ {
		maxZoneStates = limit
		path := requirePath(t, f, InfluencePathQuery{From: "n:writer", To: "n:reader"})
		if len(path.Edges) == 0 {
			continue
		}
		limitedOnlyExact = true
		if !slices.Contains(path.Stops, core.InfluencePathStopComputationLimit) {
			t.Errorf("limit %d: stops = %v, want the computation limit", limit, path.Stops)
		}
		for _, edge := range path.Edges {
			if slices.Contains(edge.TimeBases, core.InfluenceTimeBasisPrecisionWidth) {
				t.Errorf("limit %d: edge %s carries the precision width", limit, edge.Id)
			}
		}
	}
	if !limitedOnlyExact {
		t.Fatal("no limit stopped only the exact computation")
	}
}
