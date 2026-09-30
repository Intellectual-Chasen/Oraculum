package pipeline

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// edgeBasis は関連付けと割当から作ったエッジの成立の根拠である。
type edgeBasis struct {
	// matches は候補のエッジを作った関連付けである。
	matches []candidateMatch
	// exactMatches は、位置と段階から組み直せない関連付けを matches の位置で探す表である。
	// 表に位置がある関連付けでは、matches の要素を読まない。
	exactMatches map[int]core.EdgeMatch
	// assignmentBases は、接続元のアドレスから端末を導いて作った候補のエッジの成立の
	// 根拠である。用いた割当ごとに 1 件を持つ。
	assignmentBases []core.EdgeAssignmentBasis
	// terminalAssignments は、利用者が収集元に付けた割当のうち、その IP からエッジを作った
	// 割当である (addAssignedAddressEdges)。
	terminalAssignments []core.TerminalAssignment
	// observedInRecords は、terminalAssignments を持つエッジを、レコードも直接作ったかである。
	observedInRecords bool
	// lease は、収集の registry の DHCP のリースから作った terminal_address のエッジの、
	// リースを得た時刻から収集の最後のレコードの時刻までの期間である (addProfileAddressEdges)。
	lease *core.TimeRange
	// pairs は、観測の層が候補のエッジを作ったレコードの組である (addRecordPair)。
	pairs []recordPair
	// sessions は、ログオンのセッションから作ったエッジの、起点のセッションの期間と終点の
	// レコードである (addSessionBasis)。影響のエッジの D がセッションの期間を持つ。
	sessions []sessionSpan
	// indistinguishable は、関連付けの候補を含む互いに区別できない候補の組が指すノードの数の最大
	// である。別のノードを指す候補と同じ組に入らないエッジでは 0 である (addCandidateEdge)。
	indistinguishable int32
}

// indistinguishableCount は edgeBasis.indistinguishable を返す。成立の根拠を持たないエッジでは 0 である。
func (e graphEdge) indistinguishableCount() int32 {
	if e.basis == nil {
		return 0
	}
	return e.basis.indistinguishable
}

// ensureBasis はエッジの成立の根拠を返す。まだ無ければ足す。
func (e *graphEdge) ensureBasis() *edgeBasis {
	if e.basis == nil {
		e.basis = &edgeBasis{}
	}
	return e.basis
}

// matchList はエッジの関連付けを返す。成立の根拠を持たないエッジでは nil を返す。
func (e graphEdge) matchList() []candidateMatch {
	if e.basis == nil {
		return nil
	}
	return e.basis.matches
}

// assignmentBasisList はエッジの割当の根拠を返す。成立の根拠を持たないエッジでは nil を返す。
func (e graphEdge) assignmentBasisList() []core.EdgeAssignmentBasis {
	if e.basis == nil {
		return nil
	}
	return e.basis.assignmentBases
}

// terminalAssignmentList はエッジを作った端末の割当を返す。成立の根拠を持たないエッジでは
// nil を返す。
func (e graphEdge) terminalAssignmentList() []core.TerminalAssignment {
	if e.basis == nil {
		return nil
	}
	return e.basis.terminalAssignments
}

// candidateMatch は候補のエッジを作った関連付け 1 件である。
//
// **関連付け 1 件ごとに位置の構造体を持たない。** 候補のレコードは g.records の位置で、段階に共通する
// 値は g.matchStages の位置で指す。応答の core.EdgeMatchTable は edgeMatchTableOf が組む。
//
// 既知の制限: 位置と段階を int32 で指し、int32 で表せない位置の関連付けは core.EdgeMatch のまま持つ,
// レコードの位置が int32 を超える入力は repo の fixture で作れず、2 つの形が混ざる経路を
// 測れない, レコードの数が int32 の上限に近づく入力を扱うときに見直す
type candidateMatch struct {
	// candidate は候補のレコードの g.records での位置である。
	candidate int32
	// stage は関連付けを出した段階の g.matchStages での位置である。
	stage int32
	// reasons は、段階の reasonSets のうちこの関連付けの確定しない理由の位置である。
	reasons int32
}

// matchStage は起点 1 件に対する候補集合の最後の段階と、その段階が挙げた関連付けが共有する値である。
type matchStage struct {
	// origin は起点のレコードの g.records での位置である。
	origin              int32
	stageKey            core.StageKey
	conditions          []core.MatchCondition
	assumptions         []core.MatchAssumption
	timeWindow          core.TimeWindow
	clockDependencyNote string
	stageTallies        []core.MatchStageTally
	// groups は互いに区別できない候補の組を、レコードの g.records での位置で持つ。
	groups [][]int32
	// reasonSets は段階の関連付けが持つ確定しない理由の異なりである。
	reasonSets [][]string
}

// compactIndex は位置を int32 へ直す。ok が偽になるのは int32 で表せない位置である。
func compactIndex(at int) (int32, bool) {
	if at < 0 || at > math.MaxInt32 {
		return 0, false
	}
	return int32(at), true
}

// addMatchStage は段階 1 つを関連付けが共有する値として g.matchStages へ足し、その位置を返す。
//
// ok が偽になるのは、起点の位置または互いに区別できない候補の組を、グラフのレコードの位置から
// 同じ値で組み直せないときである。その段階の関連付けは core.EdgeMatch のまま持つ。
func (g *Graph) addMatchStage(ends candidateEdgeEnds, stage core.CandidateStage) (int32, bool) {
	origin, originFits := compactIndex(ends.originAt)
	if !originFits || !sameLocator(ends.originRef, g.records[ends.originAt].locator) {
		return 0, false
	}
	groups := make([][]int32, 0, len(stage.IndistinguishableGroups))
	for _, group := range stage.IndistinguishableGroups {
		members := make([]int32, 0, len(group))
		for _, locator := range group {
			at, found := g.recordAtLocator(locator)
			if !found || !sameLocator(locator, g.records[at].locator) {
				return 0, false
			}
			member, fits := compactIndex(at)
			if !fits {
				return 0, false
			}
			members = append(members, member)
		}
		groups = append(groups, members)
	}
	at, fits := compactIndex(len(g.matchStages))
	if !fits {
		return 0, false
	}
	g.matchStages = append(g.matchStages, matchStage{
		origin: origin, stageKey: stage.StageKey, conditions: stage.Conditions,
		assumptions: stage.Assumptions, timeWindow: stage.TimeWindow,
		clockDependencyNote: ends.clockDependencyNote, stageTallies: ends.stageTallies,
		groups: groups,
	})
	return at, true
}

// appendEdgeMatch は候補 1 件の関連付けをエッジへ足す。
//
// **位置と段階から同じ値を組み直せない関連付けを core.EdgeMatch のまま持つ。** 候補のレコードの位置が
// グラフのレコードと食い違う関連付けと、時刻の比較が段階と両端のレコードの時刻から導けない関連付けである。
func (g *Graph) appendEdgeMatch(
	edgeAt int, ends candidateEdgeEnds, stage core.CandidateStage, member core.Candidate,
	candidateAt int,
) {
	basis := g.edges[edgeAt].ensureBasis()
	if match, compact := g.compactMatchOf(ends, member, candidateAt); compact {
		basis.matches = append(basis.matches, match)
		return
	}
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
}

// compactMatchOf は候補 1 件の関連付けを位置と段階で指す形にする。ok が偽になるのは、その形から
// 同じ値を組み直せないときである。
func (g *Graph) compactMatchOf(
	ends candidateEdgeEnds, member core.Candidate, candidateAt int,
) (candidateMatch, bool) {
	if !ends.compactStage {
		return candidateMatch{}, false
	}
	candidate, fits := compactIndex(candidateAt)
	if !fits || !sameLocator(member.RecordRef, g.records[candidateAt].locator) {
		return candidateMatch{}, false
	}
	stage := &g.matchStages[ends.matchStage]
	if !sameTimeComparison(member.TimeComparison, g.timeComparisonOf(*stage, candidate)) {
		return candidateMatch{}, false
	}
	reasons := slices.IndexFunc(stage.reasonSets, func(reasons []string) bool {
		return sameStrings(reasons, member.UnresolvedReasons)
	})
	if reasons < 0 {
		reasons = len(stage.reasonSets)
		stage.reasonSets = append(stage.reasonSets, member.UnresolvedReasons)
	}
	reasonsAt, fits := compactIndex(reasons)
	if !fits {
		return candidateMatch{}, false
	}
	return candidateMatch{candidate: candidate, stage: ends.matchStage, reasons: reasonsAt}, true
}

// timeComparisonOf は段階と候補のレコードから、起点と候補の時刻の比較を組む。
//
// 時刻を比べる段階 (second_time_matched) だけが両端の時刻を持つ。core の候補集合が候補 1 件の
// 比較を組む規則と同じである (core.MatchRequest の buildCandidate)。
func (g Graph) timeComparisonOf(stage matchStage, candidate int32) core.TimeComparison {
	if comparisonUnitOf(stage.stageKey) != core.ComparisonUnitSecond {
		return core.TimeComparison{
			ComparisonUnit: core.ComparisonUnitNotCompared, Assumptions: []core.MatchAssumption{},
		}
	}
	return core.TimeComparison{
		ComparisonUnit: core.ComparisonUnitSecond,
		LeftTime:       g.records[stage.origin].eventTime,
		RightTime:      g.records[candidate].eventTime,
		Assumptions:    core.TimeComparisonAssumptions(stage.assumptions),
	}
}

// comparisonUnitOf は段階が起点と候補の時刻を比べる単位を返す。timeComparisonOf と同じ規則である。
func comparisonUnitOf(stageKey core.StageKey) core.ComparisonUnit {
	if stageKey == core.StageKeySecondTimeMatched {
		return core.ComparisonUnitSecond
	}
	return core.ComparisonUnitNotCompared
}

// EdgeMatchBetween は、起点のレコードから候補のレコードを挙げた関連付け 1 件を返す。
// found が偽になるのは、2 つのレコードを結ぶ関連付けがグラフに無いときである。
//
// **起点の段階を先に絞り、関連付けは候補の位置だけで比べる。** 関連付けごとに段階を探すと、段階の配列を
// 関連付けの数だけ飛び飛びに読む。起点の段階は起点 1 件につき 1 つであり、段階の数は関連付けの数より
// ずっと少ない。見つけた関連付けは、その 1 件だけを表の形から組み直す。
//
// 既知の制限: 収集元をまたいだ通信の関連付けを全件走査し、起点と候補の位置で探す索引を持たない,
// 所要は関連付けの数に比例し、repo の fixture の大きさでは測れない,
// 関連付けが増えて 1 要求が 200 ms を超えたとき、起点と候補の位置で探す索引を持つ
func (g Graph) EdgeMatchBetween(origin, candidate core.RecordLocator) (core.EdgeMatch, bool, error) {
	originAt, originFound := g.recordAtLocator(origin)
	candidateAt, candidateFound := g.recordAtLocator(candidate)
	if !originFound || !candidateFound {
		return core.EdgeMatch{}, false, nil
	}
	originStages := make(map[int32]struct{})
	for at, stage := range g.matchStages {
		// 段階の位置は addMatchStage が int32 に収めたため、compactIndex は常に収まる。
		if key, fits := compactIndex(at); fits && int(stage.origin) == originAt {
			originStages[key] = struct{}{}
		}
	}
	for at := range g.edges {
		edge := &g.edges[at]
		if edge.kind != core.EdgeKindCrossSourceConnectionMatch {
			continue
		}
		if index, found := g.matchIndexBetween(edge, originStages, originAt, candidateAt); found {
			match, err := g.edgeMatchAt(*edge, index)
			return match, err == nil, err
		}
	}
	return core.EdgeMatch{}, false, nil
}

// matchIndexBetween は、エッジの関連付けのうち起点 originAt と候補 candidateAt を結ぶ最初の 1 件の
// 位置を返す。originStages は起点 originAt の段階の位置である。
//
// 位置と段階から組み直せない関連付けは、持つ位置に対応するグラフのレコードの位置を探して比べる。
func (g Graph) matchIndexBetween(
	edge *graphEdge, originStages map[int32]struct{}, originAt, candidateAt int,
) (int, bool) {
	matches := edge.matchList()
	first := len(matches)
	for index, match := range matches {
		if int(match.candidate) != candidateAt {
			continue
		}
		if _, exact := edge.basis.exactMatches[index]; exact {
			continue
		}
		if _, ofOrigin := originStages[match.stage]; ofOrigin {
			first = index
			break
		}
	}
	if edge.basis != nil {
		for index, exact := range edge.basis.exactMatches {
			if index >= first {
				continue
			}
			exactOrigin, originFound := g.recordAtLocator(exact.OriginRef)
			exactCandidate, candidateFound := g.recordAtLocator(exact.CandidateRef)
			if originFound && candidateFound && exactOrigin == originAt && exactCandidate == candidateAt {
				first = index
			}
		}
	}
	return first, first < len(matches)
}

// edgeMatchAt はエッジの index 番目の関連付け 1 件を組み直す。edgeMatchTableOf の表の同じ位置の
// 関連付けと同じ値である。
func (g Graph) edgeMatchAt(edge graphEdge, index int) (core.EdgeMatch, error) {
	builder := newMatchTableBuilder(g)
	builder.table.Matches = []core.EdgeMatchRef{builder.ref(edge, index)}
	return builder.table.Match(0)
}

// edgeMatchTableOf はエッジ 1 本の関連付けを、段階とレコードを 1 度ずつ置いた表にする。関連付けは並び順の
// ままである。
//
// 表の時刻の比較の組み直し方 (core.EdgeMatchTable.Match) は timeComparisonOf と同じ規則であり、
// 位置と段階で指す関連付けは比較を持たない。
func (g Graph) edgeMatchTableOf(edge graphEdge) core.EdgeMatchTable {
	matches := edge.matchList()
	if len(matches) == 0 {
		return core.EdgeMatchTable{}
	}
	builder := newMatchTableBuilder(g)
	builder.table.Matches = make([]core.EdgeMatchRef, len(matches))
	for index := range matches {
		builder.table.Matches[index] = builder.ref(edge, index)
	}
	return builder.table
}

func newMatchTableBuilder(g Graph) *matchTableBuilder {
	return &matchTableBuilder{
		graph: g, recordAt: make(map[int32]int), stageAt: make(map[int32]int), locatorAt: make(map[string]int),
	}
}

// ref はエッジの index 番目の関連付けを、表の段階とレコードを指す形で置く。
func (b *matchTableBuilder) ref(edge graphEdge, index int) core.EdgeMatchRef {
	if exact, found := edge.basis.exactMatches[index]; found {
		return b.exactRef(exact)
	}
	match := edge.basis.matches[index]
	return core.EdgeMatchRef{
		Stage: b.stage(match.stage), Candidate: b.record(match.candidate),
		UnresolvedReasons: int(match.reasons),
	}
}

// matchTableBuilder は関連付けの表を組む。レコードとグラフの段階は、表に 1 度だけ置く。
//
// **位置と段階から組み直せない関連付けは、関連付けごとに段階を 1 つ置く。** その関連付けはグラフの段階を
// 持たず、共有する段階を指せない。
type matchTableBuilder struct {
	graph    Graph
	table    core.EdgeMatchTable
	recordAt map[int32]int
	stageAt  map[int32]int
	// locatorAt は、グラフのレコードと一致しないレコードの位置を、位置の全項目の文字列で探す。
	locatorAt map[string]int
}

// record はグラフのレコード at の、表での位置を返す。まだ置いていなければ置く。
func (b *matchTableBuilder) record(at int32) int {
	if position, placed := b.recordAt[at]; placed {
		return position
	}
	position := len(b.table.MatchRecords)
	b.table.MatchRecords = append(b.table.MatchRecords, core.EdgeMatchRecord{
		Ref: b.graph.records[at].locator, EventTime: b.graph.records[at].eventTime,
		ProxyStatus: b.graph.records[at].proxyStatus,
	})
	b.recordAt[at] = position
	return position
}

// stage はグラフの段階 at の、表での位置を返す。まだ置いていなければ置く。
func (b *matchTableBuilder) stage(at int32) int {
	if position, placed := b.stageAt[at]; placed {
		return position
	}
	stage := b.graph.matchStages[at]
	groups := make([][]int, 0, len(stage.groups))
	for _, group := range stage.groups {
		members := make([]int, 0, len(group))
		for _, member := range group {
			members = append(members, b.record(member))
		}
		groups = append(groups, members)
	}
	position := len(b.table.MatchStages)
	b.table.MatchStages = append(b.table.MatchStages, core.EdgeMatchStage{
		Origin: b.record(stage.origin), StageKey: stage.stageKey, Conditions: stage.conditions,
		Assumptions: stage.assumptions, TimeWindow: stage.timeWindow,
		ClockDependencyNote: stage.clockDependencyNote, StageTallies: stage.stageTallies,
		ComparisonUnit: comparisonUnitOf(stage.stageKey), IndistinguishableGroups: groups,
		UnresolvedReasonSets: stage.reasonSets,
	})
	b.stageAt[at] = position
	return position
}

// exactRef は位置と段階から組み直せない関連付けを、その関連付けだけの段階と、関連付けが持つ時刻の比較で置く。
func (b *matchTableBuilder) exactRef(exact core.EdgeMatch) core.EdgeMatchRef {
	groups := make([][]int, 0, len(exact.IndistinguishableGroups))
	for _, group := range exact.IndistinguishableGroups {
		members := make([]int, 0, len(group))
		for _, locator := range group {
			members = append(members, b.locator(locator))
		}
		groups = append(groups, members)
	}
	position := len(b.table.MatchStages)
	b.table.MatchStages = append(b.table.MatchStages, core.EdgeMatchStage{
		Origin: b.locator(exact.OriginRef), StageKey: exact.StageKey, Conditions: exact.Conditions,
		Assumptions: exact.Assumptions, TimeWindow: exact.TimeWindow,
		ClockDependencyNote: exact.ClockDependencyNote, StageTallies: exact.StageTallies,
		ComparisonUnit: exact.TimeComparison.ComparisonUnit, IndistinguishableGroups: groups,
		UnresolvedReasonSets: [][]string{exact.UnresolvedReasons},
	})
	comparison := exact.TimeComparison
	return core.EdgeMatchRef{Stage: position, Candidate: b.locator(exact.CandidateRef), TimeComparison: &comparison}
}

// locator はレコードの位置 locator の、表での位置を返す。グラフのレコードと同じ値であれば
// そのレコードを指し、そうでなければ同じ値の位置を 1 度だけ置く。
func (b *matchTableBuilder) locator(locator core.RecordLocator) int {
	if at, found := b.graph.recordAtLocator(locator); found && sameLocator(locator, b.graph.records[at].locator) {
		if compact, fits := compactIndex(at); fits {
			return b.record(compact)
		}
	}
	key := locatorKey(locator)
	if position, placed := b.locatorAt[key]; placed {
		return position
	}
	position := len(b.table.MatchRecords)
	b.table.MatchRecords = append(b.table.MatchRecords, core.EdgeMatchRecord{Ref: locator})
	b.locatorAt[key] = position
	return position
}

// locatorKey はレコードの位置の全項目を並べた文字列である。sameLocator が同じとする 2 つの
// 位置は、同じ文字列になる。
func locatorKey(locator core.RecordLocator) string {
	optional := func(value *int64) string {
		if value == nil {
			return "-"
		}
		return strconv.FormatInt(*value, 10)
	}
	return strings.Join([]string{
		locator.SourceId, locator.SourceContentSha256, locator.SourceFileName, string(locator.PositionKind),
		locator.RecordRawTextRef, optional(locator.SequenceNumber), optional(locator.LineNumber),
		optional(locator.ByteOffset), optional(locator.ByteLength), optional(locator.LineCount),
	}, "\x00")
}

// sameLocator は 2 つのレコードの位置が同じ値を持つかを返す。
func sameLocator(left, right core.RecordLocator) bool {
	return left.SourceId == right.SourceId &&
		left.SourceContentSha256 == right.SourceContentSha256 &&
		left.SourceFileName == right.SourceFileName &&
		left.PositionKind == right.PositionKind &&
		left.RecordRawTextRef == right.RecordRawTextRef &&
		sameOptional(left.SequenceNumber, right.SequenceNumber) &&
		sameOptional(left.LineNumber, right.LineNumber) &&
		sameOptional(left.ByteOffset, right.ByteOffset) &&
		sameOptional(left.ByteLength, right.ByteLength) &&
		sameOptional(left.LineCount, right.LineCount)
}

// sameTimeComparison は 2 つの時刻の比較が同じ値を持つかを返す。
//
// 前提の集合は、要素数 0 の集合と nil を同じ値とする。応答はどちらも要素数 0 の集合として出す
// (core.TimeComparison の MarshalJSON)。
func sameTimeComparison(left, right core.TimeComparison) bool {
	return left.ComparisonUnit == right.ComparisonUnit &&
		sameTimestampPointer(left.LeftTime, right.LeftTime) &&
		sameTimestampPointer(left.RightTime, right.RightTime) &&
		slices.Equal(left.Assumptions, right.Assumptions)
}

// sameTimestampPointer は 2 つの時刻が、どちらも無いか同じ値を持つかを返す。
func sameTimestampPointer(left, right *core.Timestamp) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// sameOptional は 2 つの値が、どちらも無いか同じ値であるかを返す。
func sameOptional[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// sameStrings は 2 つの文字列の集合が同じ要素を同じ順に持ち、nil であるかも一致するかを返す。
// 応答は nil を null、要素数 0 の集合を [] として出す。
func sameStrings(left, right []string) bool {
	return (left == nil) == (right == nil) && slices.Equal(left, right)
}
