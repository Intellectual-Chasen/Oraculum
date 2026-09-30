// in-package test: 非公開の候補の層を、索引を使わない関連付けの手順と突き合わせる。
package pipeline

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// oracleCandidateGraph は、起点ごとに候補の母集合を走査し直す手順で候補の層を組む。
//
// 母集合を探す鍵ごとに観測を組んで共有する手順 (candidateLookup) と同じ結果になることを、
// この手順との突き合わせで確かめる。この手順は、起点ごとに母集合の全件から候補の観測を組み、
// 母集合の全件を core.BuildCandidateSet へ渡す。
func oracleCandidateGraph(
	observed Graph, result ImportResult, selection MatchConditionSelection,
) Graph {
	graph := observed.withoutCandidateEdges(selection)
	for _, scope := range result.caseScopes() {
		assignments := terminalAssignmentsOf(scope)
		for groupIndex, sides := range graph.candidateSideGroups(scope) {
			for _, origin := range sides.origins {
				outcome := graph.oracleEdgesOfOrigin(scope, sides, assignments, origin)
				graph.recordOriginOutcome(origin, outcome, groupIndex == 0)
			}
		}
	}
	return graph
}

// collectCandidateSides は、観測の層を組んで最初の候補の組を返す。
func collectCandidateSides(result ImportResult) candidateSides {
	return NewObservedGraph(result).candidateSideGroups(result)[0]
}

func (s candidateSides) oracleCandidatesFor(
	origin matchSide, selection MatchConditionSelection,
) []matchSide {
	_, byIp := selection.narrows(core.ConditionKeyDestinationIp)
	_, byPort := selection.narrows(core.ConditionKeyDestinationPort)
	byIp = byIp && s.candidateDeclarations.narrows(core.ConditionKeyDestinationIp)
	byPort = byPort && s.candidateDeclarations.narrows(core.ConditionKeyDestinationPort)
	destinationIp, hasIp := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationAddress)
	// 要求先がホスト名である起点は、接続先 IP の条件を選んでも接続先 IP で絞らない。
	// 起点の手前の分類が、接続先 IP もホスト名も持たない起点を外している。
	byIp = byIp && hasIp
	destinationPort, _ := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationPort)
	if byIp && byPort {
		return s.byDestination[destinationKey{ip: destinationIp, port: destinationPort}]
	}
	if !byIp && !byPort {
		return s.candidates
	}
	selected := make([]matchSide, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		semantic := core.SemanticKeyConnectionDestinationAddress
		wanted := destinationIp
		if byPort {
			semantic = core.SemanticKeyConnectionDestinationPort
			wanted = destinationPort
		}
		if value, readable := comparableOfSemantic(candidate.fields, semantic); readable &&
			value == wanted {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func (g *Graph) oracleEdgesOfOrigin(
	result ImportResult, sides candidateSides, assignments terminalAssignments, origin matchSide,
) core.RelationDerivationOutcome {
	destination, hasIp := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationAddress)
	if !hasIp {
		hostname, hasHostname := comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationHostname)
		if !hasHostname {
			return core.RelationDerivationDestinationIpAbsent
		}
		destination = hostname
	}
	_, byPort := g.matchSelection.narrows(core.ConditionKeyDestinationPort)
	if byPort && sides.candidateDeclarations.narrows(core.ConditionKeyDestinationPort) {
		if _, hasPort := comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationPort); !hasPort {
			return core.RelationDerivationDestinationPortAbsent
		}
	}
	proxies, restricts := oracleProxyAddresses(result, sides, g.matchSelection, origin)
	if restricts && len(proxies) == 0 {
		return core.RelationDerivationProxyAddressUnknown
	}
	candidates := sides.oracleCandidatesFor(origin, g.matchSelection)
	if restricts {
		candidates = slices.DeleteFunc(slices.Clone(candidates), func(candidate matchSide) bool {
			destination, _ := comparableOfSemantic(
				candidate.fields, core.SemanticKeyConnectionDestinationAddress)
			return !slices.Contains(proxies, destination)
		})
	}
	if len(candidates) == 0 {
		return core.RelationDerivationNoCandidateRecord
	}
	request, prior, outcome := g.oracleMatchRequestOf(result, assignments, origin, candidates,
		sides.candidateDeclarations, proxies)
	if outcome != core.RelationDerivationMatched {
		return outcome
	}
	set, err := core.BuildCandidateSet(request)
	if err != nil {
		if g.candidateSetProblem == nil {
			g.candidateSetProblem = err
		}
		return core.RelationDerivationFailed
	}
	return g.oracleApplyCandidateSet(set, prior, origin, destination, candidates)
}

// oracleProxyAddresses は、起点の候補を Proxy への接続に限るかと、起点の収集元の全体に
// 付けた割当の IP を返す。割当を 1 件ずつ読む。
func oracleProxyAddresses(
	result ImportResult, sides candidateSides, selection MatchConditionSelection, origin matchSide,
) ([]string, bool) {
	_, byIp := selection.narrows(core.ConditionKeyDestinationIp)
	if !byIp || len(sides.candidates) == 0 ||
		sides.candidateDeclarations.narrows(core.ConditionKeyDestinationIp) {
		return nil, false
	}
	var addresses []string
	for _, assignment := range result.userAssignments() {
		if assignment.AppliesToSourceId == origin.record.Locator.SourceId && assignment.ClientIp != "" &&
			!slices.Contains(addresses, assignment.ClientIp) {
			addresses = append(addresses, assignment.ClientIp)
		}
	}
	slices.Sort(addresses)
	return addresses, true
}

func (g *Graph) oracleMatchRequestOf(
	result ImportResult, assignments terminalAssignments, origin matchSide,
	candidates []matchSide, counterpart sideDeclarations, proxyAddresses []string,
) (core.MatchRequest, []core.MatchStageTally, core.RelationDerivationOutcome) {
	terminalAssignments, resolved := oracleResolveOriginTerminal(assignments, origin)
	if !resolved {
		return core.MatchRequest{}, nil, core.RelationDerivationTerminalUndetermined
	}
	assignment := terminalAssignments[0]
	if _, keyed := assignment.TerminalNodeKey(); !keyed {
		return core.MatchRequest{}, nil, core.RelationDerivationTerminalIdAbsent
	}
	specs, declarationProblem := candidateConditionSpecs(origin, counterpart, g.matchSelection)
	if declarationProblem != nil {
		g.declarationProblem = declarationProblem
		return core.MatchRequest{}, nil, core.RelationDerivationSourceDeclarationConflict
	}
	if !anyComparedCondition(specs) {
		return core.MatchRequest{}, nil, core.RelationDerivationNoComparedCondition
	}
	timeBound := timeBoundSpecs(counterpart, specs)
	if _, comparesTime := g.matchSelection.comparesTime(); timeBound && !comparesTime {
		return core.MatchRequest{}, nil, core.RelationDerivationNoComparedCondition
	}
	observation, built := originObservationOf(origin, terminalAssignments, specs)
	if !built {
		return core.MatchRequest{}, nil, core.RelationDerivationOriginItemUnreadable
	}
	window, windowed := g.matchSelection.windowOf(*origin.record.ObservedAt)
	if !windowed {
		return core.MatchRequest{}, nil, core.RelationDerivationOriginItemUnreadable
	}
	members := make([]core.MatchCandidateRecord, 0, len(candidates))
	for _, candidate := range candidates {
		member, ok := candidateObservationOf(candidate, specs)
		if !ok {
			g.unreadableCandidates++
			continue
		}
		members = append(members, member)
	}
	if len(members) == 0 {
		return core.MatchRequest{}, nil, core.RelationDerivationCandidateItemUnreadable
	}
	parts := matchRequestParts{
		result: result, origin: origin, assignment: assignment, observation: observation,
		window: window, members: narrowedCandidates{records: members}, specs: specs,
		counterpartItemSemantics: unionItemSemantics(candidates),
		counterpartTimePrecision: coarsestTimePrecision(candidates),
		counterpart:              counterpart, selection: g.matchSelection,
		proxyAddresses: proxyAddresses,
	}
	var prior []core.MatchStageTally
	if timeBound {
		// 段階 1 の候補を 1 件ずつ比べて選び、時刻の範囲の中の候補を秒で選ぶ。索引を使わない。
		grouped, inWindow := oracleTimeBoundMembers(observation, specs, window, members)
		if len(grouped) == 0 {
			return core.MatchRequest{}, nil, core.RelationDerivationNoCandidateMatchingConditions
		}
		if len(inWindow) == 0 {
			return core.MatchRequest{}, nil, core.RelationDerivationNoCandidateInWindow
		}
		processes := make(map[core.ProcessRef]struct{})
		for _, member := range grouped {
			if member.ProcessRef != nil {
				processes[*member.ProcessRef] = struct{}{}
			}
		}
		tally := core.MatchStageTally{
			StageKey: core.StageKeyClockIndependent, MemberCount: int64(len(grouped)),
		}
		if len(processes) > 0 {
			count := int64(len(processes))
			tally.DistinctProcessCount = &count
		}
		parts.members = narrowedCandidates{records: inWindow}
		parts.stageKeys, prior = []core.StageKey{core.StageKeySecondTimeMatched},
			[]core.MatchStageTally{tally}
	}
	request, err := matchRequestOfMembers(parts)
	if err != nil {
		return core.MatchRequest{}, nil, core.RelationDerivationOriginItemUnreadable
	}
	return request, prior, core.RelationDerivationMatched
}

// oracleTimeBoundMembers は、段階 1 の比べる値が起点と一致する候補と、そのうち時刻が範囲の中に
// ある候補を、候補の並びで返す。
func oracleTimeBoundMembers(
	origin core.MatchObservation, specs []core.MatchConditionSpec, window core.TimeWindow,
	members []core.MatchCandidateRecord,
) ([]core.MatchCandidateRecord, []core.MatchCandidateRecord) {
	lower, upper, _ := windowSecondsOf(window)
	var grouped, inWindow []core.MatchCandidateRecord
	for _, member := range members {
		matched := true
		for _, spec := range comparedSpecsOf(specs) {
			left, leftOk := matchValueOf(origin, spec.OriginSemantic)
			right, rightOk := counterpartMatchValue(member.Observation, spec)
			matched = matched && leftOk && rightOk && left == right
		}
		if !matched {
			continue
		}
		grouped = append(grouped, member)
		if instant, readable := member.Observation.EventTime.Instant(); readable &&
			instant.Unix() >= lower && instant.Unix() <= upper {
			inWindow = append(inWindow, member)
		}
	}
	return grouped, inWindow
}

func oracleResolveOriginTerminal(
	assignments terminalAssignments, origin matchSide,
) ([]core.TerminalAssignment, bool) {
	clientIp, readable := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionSourceAddress)
	if !readable {
		return nil, false
	}
	forClientIp := make([]core.TerminalAssignment, 0, len(assignments.entries))
	for _, assignment := range assignments.entries {
		if assignment.ClientIp == clientIp {
			forClientIp = append(forClientIp, assignment)
		}
	}
	if len(forClientIp) == 0 {
		return nil, false
	}
	resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
		ClientIp: clientIp, EventTime: *origin.record.ObservedAt, Assignments: forClientIp,
	})
	if err != nil || !resolution.Determined() {
		return nil, false
	}
	return resolution.Members, true
}

func (g *Graph) oracleApplyCandidateSet(
	set core.CandidateSet, prior []core.MatchStageTally, origin matchSide, destination string,
	candidates []matchSide,
) core.RelationDerivationOutcome {
	first, hasFirst := stageOf(set, core.StageKeyClockIndependent)
	stage, found := stageOf(set, g.matchSelection.lastStageKey())
	if (!hasFirst && len(prior) == 0) || !found {
		return core.RelationDerivationNodeUnresolved
	}
	clockDependencyNote := clockDependencyNoteText
	if hasFirst {
		clockDependencyNote = first.ClockDependencyNote
	}
	if len(stage.Members) == 0 {
		return emptyStageOutcome(stage.EmptyReason)
	}
	originAt, known := g.recordAtLocator(origin.record.Locator)
	if !known {
		g.droppedCandidates += len(stage.Members)
		return core.RelationDerivationNodeUnresolved
	}
	source, located := g.destinationNodeAt(originAt, origin.fields, destination)
	if !located {
		g.droppedCandidates += len(stage.Members)
		return core.RelationDerivationNodeUnresolved
	}
	ends := candidateEdgeEnds{
		source: source, originAt: originAt, originRef: set.OriginRef,
		clockDependencyNote: clockDependencyNote,
		stageTallies:        append(slices.Clone(prior), stageTalliesOf(set)...),
	}
	added := 0
	for _, member := range stage.Members {
		if g.oracleAddCandidateEdge(ends, stage, member, candidates) {
			added++
		}
	}
	if added == 0 {
		return core.RelationDerivationNodeUnresolved
	}
	return core.RelationDerivationMatched
}

func (g *Graph) oracleAddCandidateEdge(
	ends candidateEdgeEnds, stage core.CandidateStage, member core.Candidate,
	candidates []matchSide,
) bool {
	var candidate matchSide
	matched := false
	for _, side := range candidates {
		if recordKeyOf(side.record.Locator) == recordKeyOf(member.RecordRef) {
			candidate, matched = side, true
			break
		}
	}
	if !matched {
		g.droppedCandidates++
		return false
	}
	candidateAt, present := g.recordAtLocator(candidate.record.Locator)
	if !present {
		g.droppedCandidates++
		return false
	}
	target, hasProcess := g.recordedProcessAt(candidateAt)
	if !hasProcess {
		g.droppedCandidates++
		return false
	}
	at := g.ensureEdge(
		core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate, ends.source, target)
	g.addEdgeEvidence(at, ends.originAt)
	g.addEdgeEvidence(at, candidateAt)
	// 関連付けをすべて core.EdgeMatch のまま持つ。位置と段階で指す形と突き合わせる相手である。
	basis := g.edges[at].ensureBasis()
	if basis.exactMatches == nil {
		basis.exactMatches = make(map[int]core.EdgeMatch)
	}
	basis.exactMatches[len(basis.matches)] = core.EdgeMatch{
		OriginRef:               ends.originRef,
		CandidateRef:            member.RecordRef,
		StageKey:                stage.StageKey,
		Conditions:              stage.Conditions,
		Assumptions:             stage.Assumptions,
		TimeWindow:              stage.TimeWindow,
		TimeComparison:          member.TimeComparison,
		ClockDependencyNote:     ends.clockDependencyNote,
		StageTallies:            ends.stageTallies,
		IndistinguishableGroups: stage.IndistinguishableGroups,
		UnresolvedReasons:       member.UnresolvedReasons,
	}
	basis.matches = append(basis.matches, candidateMatch{})
	return true
}

// withoutMatches は、関連付けの持ち方を除いたエッジを返す。関連付けは edgeMatchesOf で組み直して
// 突き合わせる。
func withoutMatches(edge graphEdge) graphEdge {
	if edge.basis != nil {
		edge.basis = &edgeBasis{assignmentBases: edge.basis.assignmentBases}
	}
	return edge
}

// candidateLayerDifference は 2 つのグラフの候補の層の最初の食い違いを文で返す。
// 食い違いが無いときは空の文字列を返す。
func candidateLayerDifference(got, want Graph) string {
	if got.observedEdgeCount != want.observedEdgeCount {
		return fmt.Sprintf("observed edge count %d, want %d",
			got.observedEdgeCount, want.observedEdgeCount)
	}
	gotEdges, wantEdges := got.edges[got.observedEdgeCount:], want.edges[want.observedEdgeCount:]
	if len(gotEdges) != len(wantEdges) {
		return fmt.Sprintf("%d candidate edges, want %d", len(gotEdges), len(wantEdges))
	}
	for index := range wantEdges {
		if !reflect.DeepEqual(withoutMatches(gotEdges[index]), withoutMatches(wantEdges[index])) {
			return fmt.Sprintf("the candidate edge %q differs from %q",
				gotEdges[index].id, wantEdges[index].id)
		}
		if !reflect.DeepEqual(got.edgeMatchesOf(gotEdges[index]),
			want.edgeMatchesOf(wantEdges[index])) {
			return fmt.Sprintf("the matches of the candidate edge %q differ", wantEdges[index].id)
		}
	}
	if !reflect.DeepEqual(got.edgeAt, want.edgeAt) ||
		!reflect.DeepEqual(got.candidateEdgeAt, want.candidateEdgeAt) {
		return "the edge index differs"
	}
	for index := range want.nodes {
		if !slices.Equal(got.adjacency[index].outgoing, want.adjacency[index].outgoing) ||
			!slices.Equal(got.adjacency[index].incoming, want.adjacency[index].incoming) {
			return fmt.Sprintf("the adjacency of the node %q differs", want.nodes[index].id)
		}
	}
	if !reflect.DeepEqual(got.originOutcomes, want.originOutcomes) {
		return "the origin outcomes differ"
	}
	if got.originsWithoutRecordNode != want.originsWithoutRecordNode ||
		got.unreadableCandidates != want.unreadableCandidates ||
		got.droppedCandidates != want.droppedCandidates {
		return fmt.Sprintf("the counts (%d, %d, %d) differ from (%d, %d, %d)",
			got.originsWithoutRecordNode, got.unreadableCandidates, got.droppedCandidates,
			want.originsWithoutRecordNode, want.unreadableCandidates, want.droppedCandidates)
	}
	if fmt.Sprint(got.candidateSetProblem) != fmt.Sprint(want.candidateSetProblem) ||
		fmt.Sprint(got.declarationProblem) != fmt.Sprint(want.declarationProblem) {
		return fmt.Sprintf("the problems (%v, %v) differ from (%v, %v)",
			got.candidateSetProblem, got.declarationProblem,
			want.candidateSetProblem, want.declarationProblem)
	}
	return ""
}

// equivalenceSelections は突き合わせる選択である。
//
// 母集合を探す鍵と段階 1 の比べる値に係る条件の部分集合をすべて挙げ、時刻の条件を含む組には
// 幅 0 と 1 秒と 1 日を与える。
func equivalenceSelections() []MatchConditionSelection {
	keys := []core.ConditionKey{
		core.ConditionKeyTerminalIpAssignment, core.ConditionKeyTerminalIdentityMatches,
		core.ConditionKeyDestinationIp, core.ConditionKeyDestinationPort,
		core.ConditionKeySecondOfTime, core.ConditionKeyProcess,
	}
	selections := []MatchConditionSelection{AllMatchConditions()}
	for mask := 1; mask < 1<<len(keys); mask++ {
		for _, tolerance := range []int64{0, 1, 86_400} {
			var conditions []SelectedMatchCondition
			timed := false
			for bit, key := range keys {
				if mask&(1<<bit) == 0 {
					continue
				}
				condition := SelectedMatchCondition{ConditionKey: key}
				if key == core.ConditionKeySecondOfTime {
					condition.Tolerance, timed = tolerance, true
				}
				conditions = append(conditions, condition)
			}
			if !timed && tolerance != 0 {
				continue
			}
			selections = append(selections, MatchConditionSelection{Conditions: conditions})
		}
	}
	return selections
}

// equivalenceImports は突き合わせに使う取り込み結果である。案件を分けない取り込みと、
// 1 つの案件に入れた取り込みと、2 つの案件に分けた取り込みを含む。
func equivalenceImports(t *testing.T) map[string]ImportResult {
	t.Helper()
	read := func(name string) string {
		data, err := os.ReadFile(graphFixtureDir + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	squid := func(name, content string, caseId *string) caseSource {
		return caseSource{name: name, format: string(SquidFormatKey), content: content, caseId: caseId}
	}
	return map[string]ImportResult{
		"graph": graphResult(t),
		"one case": caseImport(t,
			markIISource("endpoint.log", read("graph-markii.log"), caseOf("challenge")),
			squid("proxy.log", read("graph-squid.log"), caseOf("challenge"))),
		"two cases": caseImport(t,
			markIISource("graph-endpoint.log", read("graph-markii.log"), caseOf("baseline")),
			squid("graph-proxy.log", read("graph-squid.log"), caseOf("baseline")),
			markIISource("communications-endpoint.log", read("communications-markii.log"),
				caseOf("challenge")),
			squid("communications-proxy.log", read("communications-squid.log"), caseOf("challenge"))),
		// 端末だけで絞り、時刻の範囲で候補を先に絞る組である。
		"windows": proxyCandidateResult(t),
	}
}

// **母集合を探す鍵ごとに組んだ観測を共有しても、候補の層は起点ごとに母集合を走査し直した
// 層と同じである。** エッジの並びと中身、隣接、起点の分類、除いた候補の件数、退けた理由を
// 突き合わせる。
func TestCandidateLayerMatchesTheRescanningDerivation(t *testing.T) {
	selections := equivalenceSelections()
	for name, result := range equivalenceImports(t) {
		t.Run(name, func(t *testing.T) {
			observed := NewObservedGraph(result)
			matchedSelections := 0
			for _, selection := range selections {
				got := observed.WithCandidateEdges(result, selection)
				want := oracleCandidateGraph(observed, result, selection)
				if difference := candidateLayerDifference(got, want); difference != "" {
					t.Fatalf("the selection %+v: %s", selection.Conditions, difference)
				}
				if len(want.edges) > want.observedEdgeCount {
					matchedSelections++
				}
			}
			// **候補のエッジを 1 本も持たない層どうしの一致だけで終わらせない。**
			if matchedSelections == 0 || matchedSelections == len(selections) {
				t.Error("no selection derives a candidate edge, want the comparison to cover edges")
			}
		})
	}
}
