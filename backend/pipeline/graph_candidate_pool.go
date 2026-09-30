package pipeline

import (
	"cmp"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// candidateLookup は、1 つの案件の関連付けで起点が探す候補の母集合と端末の割当を、
// 探す鍵ごとに 1 度だけ組んで保つ。
//
// **同じ母集合を探す起点どうしで、母集合から導いた値を共有する。** 起点ごとに母集合を
// 走査し直すと、所要が起点の件数と母集合の件数の積で増える。
//
// addCandidateEdges の呼び出し 1 回が 1 つを組み、呼び出しの外へ持ち出さない。
// 複数の goroutine から同時に使わない。
type candidateLookup struct {
	sides     candidateSides
	selection MatchConditionSelection
	pools     map[candidatePoolKey]*candidatePool
	// assignments は接続元 IP ごとの割当である。並びは terminalAssignments.entries の順である。
	assignments clientAssignments
	// proxyAddresses は収集元の sourceId から、その収集元の全体に付けた端末の IP アドレスを
	// 探す表である (sourceTerminalAddressesOf)。組んだ後に addCandidateEdges が入れる。
	proxyAddresses map[string][]string
}

// newCandidateLookup は候補の側と割当から、母集合を探す表の枠を組む。母集合は探されたときに組む。
func newCandidateLookup(
	sides candidateSides, assignments terminalAssignments, selection MatchConditionSelection,
) *candidateLookup {
	return &candidateLookup{
		sides: sides, selection: selection,
		pools:       make(map[candidatePoolKey]*candidatePool),
		assignments: clientAssignmentsOf(assignments),
	}
}

// sourceTerminalAddressesOf は、利用者が収集元の全体に付けた割当 (AppliesToSourceId を持つ
// 割当) の IP アドレスを、収集元の sourceId ごとに重ねず文字列の順で返す。アドレスは
// netip.Addr の文字列へ直す。
//
// 既知の制限: 割当の適用期間を起点の時刻と比べず、収集元の全体に付けた割当の IP をすべて使う,
// 1 つの Proxy のログが Proxy の IP を 1 つだけ持つことを前提にする,
// 1 つの Proxy のログの途中で Proxy の IP が変わる入力を扱うときに、起点の時刻で割当を選ぶ
func sourceTerminalAddressesOf(result ImportResult) map[string][]string {
	addresses := make(map[string][]string)
	for _, assignment := range result.userAssignments() {
		address, err := netip.ParseAddr(assignment.ClientIp)
		if assignment.AppliesToSourceId == "" || err != nil {
			continue
		}
		held := addresses[assignment.AppliesToSourceId]
		if text := address.String(); !slices.Contains(held, text) {
			addresses[assignment.AppliesToSourceId] = append(held, text)
		}
	}
	for _, held := range addresses {
		slices.Sort(held)
	}
	return addresses
}

// proxyAddressesOf は、起点の候補を Proxy への接続に限るかと、限るときの Proxy のアドレスを
// 返す。
//
// **候補の組が接続先 IP で絞れないとき、候補が記録した接続先は Proxy のアドレスである**
// (windowsConnectionMatchConditions)。その組では、起点の要求先と比べる代わりに、候補の
// 接続先を起点の収集元 (Proxy のログ) の端末の IP と比べる。分析者が接続先 IP の条件を
// 外した選択では限らない。
func (l *candidateLookup) proxyAddressesOf(origin matchSide) (addresses []string, restricts bool) {
	_, byIp := l.selection.narrows(core.ConditionKeyDestinationIp)
	if !byIp || len(l.sides.candidates) == 0 ||
		l.sides.candidateDeclarations.narrows(core.ConditionKeyDestinationIp) {
		return nil, false
	}
	return l.proxyAddresses[origin.record.Locator.SourceId], true
}

// candidatePoolKey は起点 1 件に対する候補の母集合を指す鍵である。
//
// 母集合を探す鍵は分析者の選択に従う。接続先 IP と接続先 port のうち、選んだ条件の値だけを
// 鍵に入れる。選ばなかった条件の値は空の文字列である。proxy は候補の接続先を限る Proxy の
// アドレスを "," でつないだ文字列であり、限らないときは空の文字列である。
type candidatePoolKey struct {
	byIp   bool
	byPort bool
	ip     string
	port   string
	proxy  string
}

// poolFor は、起点 1 件に対する候補の母集合を返す。
//
// **母集合を探す鍵も分析者の選択に従う。** 接続先の条件を外した要求で、接続先の文字列で
// 探した母集合を返すと、応答は「その条件で絞っていない」と書きながら実際には絞った結果を
// 出すことになる。
//
// ホスト名だけを持つ起点は、接続先 IP の条件を選んだ要求でも接続先 IP を鍵に入れない
// (namesHostnameOnly)。
//
// proxyAddresses を与えたときは、接続先がそのどれかである候補に限る (proxyAddressesOf)。
//
// 母集合の並びは候補の側のレコードを取り込んだ順である。
func (l *candidateLookup) poolFor(origin matchSide, proxyAddresses []string) *candidatePool {
	// 候補の組が絞れない条件の値も鍵に入れない (sideDeclarations.narrows)。
	declarations := l.sides.candidateDeclarations
	_, byIp := l.selection.narrows(core.ConditionKeyDestinationIp)
	byIp = byIp && !namesHostnameOnly(origin) && declarations.narrows(core.ConditionKeyDestinationIp)
	_, byPort := l.selection.narrows(core.ConditionKeyDestinationPort)
	byPort = byPort && declarations.narrows(core.ConditionKeyDestinationPort)
	key := candidatePoolKey{byIp: byIp, byPort: byPort, proxy: strings.Join(proxyAddresses, ",")}
	if byIp {
		key.ip, _ = comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationAddress)
	}
	if byPort {
		key.port, _ = comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationPort)
	}
	if pool, built := l.pools[key]; built {
		return pool
	}
	pool := newCandidatePool(l.sides.candidatesOf(key))
	pool.proxyAddresses = proxyAddresses
	l.pools[key] = pool
	return pool
}

// candidatesOf は、鍵が指す候補の母集合を取り込んだ順で返す。
func (s candidateSides) candidatesOf(key candidatePoolKey) []matchSide {
	candidates := s.destinationCandidatesOf(key)
	if key.proxy == "" {
		return candidates
	}
	proxies := strings.Split(key.proxy, ",")
	return slices.DeleteFunc(slices.Clone(candidates), func(candidate matchSide) bool {
		destination, readable := comparableOfSemantic(
			candidate.fields, core.SemanticKeyConnectionDestinationAddress)
		return !readable || !slices.Contains(proxies, destination)
	})
}

// destinationCandidatesOf は、鍵の接続先 IP と接続先 port が指す候補を取り込んだ順で返す。
func (s candidateSides) destinationCandidatesOf(key candidatePoolKey) []matchSide {
	if key.byIp && key.byPort {
		return s.byDestination[destinationKey{ip: key.ip, port: key.port}]
	}
	if !key.byIp && !key.byPort {
		return s.candidates
	}
	semantic, wanted := core.SemanticKeyConnectionDestinationAddress, key.ip
	if key.byPort {
		semantic, wanted = core.SemanticKeyConnectionDestinationPort, key.port
	}
	selected := make([]matchSide, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		if value, readable := comparableOfSemantic(candidate.fields, semantic); readable &&
			value == wanted {
			selected = append(selected, candidate)
		}
	}
	return selected
}

// clientAssignments は IP から端末への割当を、接続元 IP で探す表である。
type clientAssignments map[string][]core.TerminalAssignment

// clientAssignmentsOf は割当を接続元 IP ごとに分ける。同じ接続元 IP の割当は entries の順に並ぶ。
//
// **期間を比べられない割当も入れる。** 関連付けの起点の端末は、FieldsBuilder の clientTerminal と
// 同じ判定で決める。比べられない割当がある接続元 IP は、端末が 1 つに決まらない。
func clientAssignmentsOf(assignments terminalAssignments) clientAssignments {
	byClientIp := make(clientAssignments)
	for _, assignment := range assignments.entries {
		byClientIp[assignment.ClientIp] = append(byClientIp[assignment.ClientIp], assignment)
	}
	return byClientIp
}

// candidatePool は候補の母集合 1 つと、母集合から起点に依らず導ける値である。
type candidatePool struct {
	// sides は母集合のレコードである。並びは取り込んだ順である。
	sides []matchSide
	// sideAt はレコードの位置の鍵 (recordKeyOf) から、その位置を持つ sides の最初の要素の
	// 番号を探す表である。
	sideAt map[string]int
	// itemSemantics は母集合の収集元が運びうる語彙の項目の和集合である (unionItemSemantics)。
	itemSemantics []core.SemanticKey
	// timePrecision は母集合の収集元の時刻の精度のうち、最も粗いものである (coarsestTimePrecision)。
	timePrecision core.Precision
	// proxyAddresses は母集合の接続先を限った Proxy のアドレスである。限っていないときは nil である。
	proxyAddresses []string
	// members は条件の宣言の組ごとに、母集合から組んだ候補の観測である。鍵は specsKeyOf である。
	members map[string]*candidateMembers
	// endpoints は sides の要素ごとに、候補のエッジの終点と根拠の位置をグラフから探した結果で
	// ある。並びは sides と同じである。まだ探していない要素は looked が偽である。
	//
	// **グラフから探した値を保ってよいのは、候補のエッジがノードとレコードを足さないため
	// である** (addCandidateEdge)。母集合は addCandidateEdges の呼び出し 1 回の中だけで使う。
	endpoints []candidateEndpoint
}

// candidateEndpoint は候補 1 件について、グラフから探したプロセスのノードと根拠の位置である。
type candidateEndpoint struct {
	looked      bool
	resolvable  bool
	target      int
	candidateAt int
}

// newCandidatePool は母集合から、起点に依らず導ける値を 1 度だけ組む。
func newCandidatePool(sides []matchSide) *candidatePool {
	pool := &candidatePool{
		sides:         sides,
		sideAt:        make(map[string]int, len(sides)),
		itemSemantics: unionItemSemantics(sides),
		timePrecision: coarsestTimePrecision(sides),
		members:       make(map[string]*candidateMembers),
		endpoints:     make([]candidateEndpoint, len(sides)),
	}
	for index, side := range sides {
		key := recordKeyOf(side.record.Locator)
		if _, taken := pool.sideAt[key]; !taken {
			pool.sideAt[key] = index
		}
	}
	return pool
}

// endpointOf は母集合からレコードの位置で候補 1 件を探し、プロセスのノードと根拠の位置を
// graph から探して返す。同じ候補を 2 回目に探すときは、1 回目に探した結果を返す。
//
// ok が偽になるのは、その位置のレコードが母集合に無いとき、母集合が無いとき、候補のレコード
// またはそのプロセスのノードを graph から探せないときである。
func (p *candidatePool) endpointOf(graph *Graph, ref core.RecordLocator) (candidateEndpoint, bool) {
	if p == nil {
		return candidateEndpoint{}, false
	}
	at, found := p.sideAt[recordKeyOf(ref)]
	if !found {
		return candidateEndpoint{}, false
	}
	endpoint := &p.endpoints[at]
	if !endpoint.looked {
		endpoint.looked = true
		endpoint.target, endpoint.candidateAt, endpoint.resolvable =
			graph.candidateEndpointsOf(p.sides[at])
	}
	return *endpoint, endpoint.resolvable
}

// size は母集合のレコードの件数である。母集合が無いときは 0 である。
func (p *candidatePool) size() int {
	if p == nil {
		return 0
	}
	return len(p.sides)
}

// membersFor は、条件の宣言の組 specs で母集合から組んだ候補の観測を返す。
// 同じ宣言の組では 2 回目から組んだ値を返す。
func (p *candidatePool) membersFor(specs []core.MatchConditionSpec) *candidateMembers {
	key := specsKeyOf(specs)
	if members, built := p.members[key]; built {
		return members
	}
	members := newCandidateMembers(p.sides, specs)
	p.members[key] = members
	return members
}

// candidateMembers は、母集合から条件の宣言の組 1 つで組んだ候補の観測である。
type candidateMembers struct {
	// records は観測を組めた候補である。並びは母集合の並びである。
	records []core.MatchCandidateRecord
	// unreadable は、関連付けが比べる語彙の項目を比べられる形で持たず、観測を組めなかった
	// 母集合のレコードの件数である。
	unreadable int
	// byComparedValues は、段階 1 が比べる条件の相手の側の値の組から、その組を持つ候補を探す表で
	// ある。候補は records の並びで並ぶ。
	//
	// **比べる値を 1 つでも読めない候補を入れない。** その候補は段階 1 のどの起点とも一致しない
	// (backend/core の matchesClockIndependentConditions)。
	//
	// **同じ組を探す起点どうしで、同じ slice を core へ渡す。** core.BuildCandidateSet は
	// 要求の候補を読むだけであり、起点ごとに複製すると所要と記憶域が起点の件数に比例して増える。
	//
	// core.MatchCandidateRecord.Validate を通らない候補が records に 1 件でもあるときは nil
	// である。そのとき narrowedFor は records の全件を返し、core が候補の検査で要求を退ける。
	//
	// **検査済みの並びとして持つ。** 候補は母集合を組むときに 1 回だけ検査し、起点ごとの要求は
	// 同じ候補を検査し直さない (core.ValidatedCandidates)。
	byComparedValues map[string]core.ValidatedCandidates
	// byTime は、段階 1 の値の組ごとに、組の候補を時刻の順に並べた索引である (timedOf)。
	// 端末だけで絞る組の要求が、時刻の範囲で候補を先に絞るときに組む。
	byTime map[string]timedCandidates
}

// narrowedCandidates は、起点 1 つの要求へ渡す候補の並びである。
type narrowedCandidates struct {
	records []core.MatchCandidateRecord
	// validated は records を検査済みの並びとして持つ。checked が偽のときは読まない。
	validated core.ValidatedCandidates
	checked   bool
}

// newCandidateMembers は母集合の各レコードから候補の観測を組み、段階 1 の値の組で探す表を作る。
func newCandidateMembers(sides []matchSide, specs []core.MatchConditionSpec) *candidateMembers {
	members := &candidateMembers{
		records: make([]core.MatchCandidateRecord, 0, len(sides)),
	}
	for _, side := range sides {
		member, ok := candidateObservationOf(side, specs)
		if !ok {
			members.unreadable++
			continue
		}
		members.records = append(members.records, member)
	}
	validated, err := core.ValidateCandidates(members.records)
	if err != nil {
		return members
	}
	compared := comparedSpecsOf(specs)
	groups := make(map[string][]int)
	for index, record := range members.records {
		key, readable := comparedValuesKey(compared, func(spec core.MatchConditionSpec) (string, bool) {
			return counterpartMatchValue(record.Observation, spec)
		})
		if !readable {
			continue
		}
		groups[key] = append(groups[key], index)
	}
	members.byComparedValues = make(map[string]core.ValidatedCandidates, len(groups))
	for key, indexes := range groups {
		members.byComparedValues[key] = validated.Subset(indexes)
	}
	return members
}

// narrowedFor は、起点の観測と段階 1 の比べる値が一致しうる候補を、records の並びで返す。
//
// **返す集合は、core が段階 1 に残す候補をすべて含む。** 段階 1 の値の組が起点と一致する候補
// だけを返し、core が同じ候補について観測の種別と段階 1 の条件を比べ直す。段階の候補と件数と
// 並びは、records の全件を渡したときと同じになる。
//
// 起点が段階 1 の比べる値を 1 つでも読めないときは records の全件を返す。その要求は core が
// 起点の検査で退ける。
func (m *candidateMembers) narrowedFor(
	origin core.MatchObservation, specs []core.MatchConditionSpec,
) narrowedCandidates {
	if m.byComparedValues == nil {
		return narrowedCandidates{records: m.records}
	}
	key, readable := comparedValuesKey(comparedSpecsOf(specs),
		func(spec core.MatchConditionSpec) (string, bool) {
			return matchValueOf(origin, spec.OriginSemantic)
		})
	if !readable {
		return narrowedCandidates{records: m.records}
	}
	validated := m.byComparedValues[key]
	return narrowedCandidates{records: validated.Records(), validated: validated, checked: true}
}

// timedCandidates は、段階 1 の値の組 1 つの候補を、時刻の秒の順に並べた索引である。
type timedCandidates struct {
	// seconds は候補の時刻の UNIX 時刻の秒である。昇順に並ぶ。時点を持たない候補は入らない。
	seconds []int64
	// indexes は seconds と同じ並びの、組の中の候補の位置である。
	indexes []int
	// distinctProcessCount は、組の候補が指す異なるプロセスの個数である。プロセスを指す候補が
	// 無いときは nil である。
	distinctProcessCount *int64
}

// timeNarrowedFor は、段階 1 の比べる値が起点と一致する候補のうち、時刻が範囲の中にある候補を、
// records の並びで返す。段階 1 の候補を数えた結果と、起点の分類を添える。
//
// **返す集合は、core が段階 2 に残す候補をすべて含む。** 時刻の範囲の両端を 1 秒ずつ広げて選び、core が
// 時刻の範囲で絞り直す。段階 1 の候補の件数は、時刻の範囲で絞る前の組の件数である。
//
// 分類が matched でないのは、段階 1 の候補が 0 件のとき (no_candidate_matching_conditions) と、
// 時刻の範囲の中の候補が 0 件のとき (no_candidate_in_window) である。起点が段階 1 の比べる値を読めない
// とき、時刻の範囲の両端を読めないとき、検査を通らない候補を持つ母集合では records の全件を返し、
// core が要求を退ける。
func (m *candidateMembers) timeNarrowedFor(
	origin core.MatchObservation, specs []core.MatchConditionSpec, window core.TimeWindow,
) (narrowedCandidates, core.MatchStageTally, core.RelationDerivationOutcome) {
	all := narrowedCandidates{records: m.records}
	allTally := core.MatchStageTally{
		StageKey: core.StageKeyClockIndependent, MemberCount: int64(len(m.records)),
	}
	if m.byComparedValues == nil {
		return all, allTally, core.RelationDerivationMatched
	}
	key, readable := comparedValuesKey(comparedSpecsOf(specs),
		func(spec core.MatchConditionSpec) (string, bool) {
			return matchValueOf(origin, spec.OriginSemantic)
		})
	lower, upper, bounded := windowSecondsOf(window)
	if !readable || !bounded {
		return all, allTally, core.RelationDerivationMatched
	}
	group := m.byComparedValues[key]
	if len(group.Records()) == 0 {
		return narrowedCandidates{}, core.MatchStageTally{},
			core.RelationDerivationNoCandidateMatchingConditions
	}
	timed := m.timedOf(key, group)
	from, _ := slices.BinarySearch(timed.seconds, lower-1)
	to, found := slices.BinarySearch(timed.seconds, upper+1)
	for found && to < len(timed.seconds) && timed.seconds[to] == upper+1 {
		to++
	}
	if from == to {
		return narrowedCandidates{}, core.MatchStageTally{}, core.RelationDerivationNoCandidateInWindow
	}
	indexes := slices.Clone(timed.indexes[from:to])
	slices.Sort(indexes)
	narrowed := group.Subset(indexes)
	tally := core.MatchStageTally{
		StageKey: core.StageKeyClockIndependent, MemberCount: int64(len(group.Records())),
		DistinctProcessCount: timed.distinctProcessCount,
	}
	return narrowedCandidates{records: narrowed.Records(), validated: narrowed, checked: true},
		tally, core.RelationDerivationMatched
}

// timedOf は、段階 1 の値の組 key の候補を時刻の順に並べた索引を返す。同じ組では 2 回目から
// 組んだ索引を返す。
func (m *candidateMembers) timedOf(key string, group core.ValidatedCandidates) timedCandidates {
	if timed, built := m.byTime[key]; built {
		return timed
	}
	records := group.Records()
	order := make([]int, 0, len(records))
	seconds := make(map[int]int64, len(records))
	processes := make(map[core.ProcessRef]struct{})
	for index, record := range records {
		if record.ProcessRef != nil {
			processes[*record.ProcessRef] = struct{}{}
		}
		if instant, readable := record.Observation.EventTime.Instant(); readable {
			order = append(order, index)
			seconds[index] = instant.Unix()
		}
	}
	slices.SortStableFunc(order, func(left, right int) int {
		return cmp.Compare(seconds[left], seconds[right])
	})
	timed := timedCandidates{seconds: make([]int64, len(order)), indexes: order}
	for position, index := range order {
		timed.seconds[position] = seconds[index]
	}
	if len(processes) > 0 {
		count := int64(len(processes))
		timed.distinctProcessCount = &count
	}
	if m.byTime == nil {
		m.byTime = make(map[string]timedCandidates)
	}
	m.byTime[key] = timed
	return timed
}

// windowSecondsOf は時刻の範囲の両端を UNIX 時刻の秒で返す。ok が偽になるのは、時刻の範囲の中心を時点として
// 読めないときと、時刻を比べない範囲である。
func windowSecondsOf(window core.TimeWindow) (int64, int64, bool) {
	if window.CenterTime == nil {
		return 0, 0, false
	}
	center, err := time.Parse(time.RFC3339Nano, window.CenterTime.Normalized)
	if err != nil {
		return 0, 0, false
	}
	var radius int64
	if window.RadiusSeconds != nil {
		radius = *window.RadiusSeconds
	}
	return center.Unix() - radius, center.Unix() + radius, true
}

// comparedSpecsOf は段階 1 が両側の文字列を比べる条件の宣言を、宣言の並びで返す。
func comparedSpecsOf(specs []core.MatchConditionSpec) []core.MatchConditionSpec {
	compared := make([]core.MatchConditionSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Compared {
			compared = append(compared, spec)
		}
	}
	return compared
}

// comparedValuesKey は条件の宣言ごとに valueOf が返す値を、宣言の並びで 1 つの文字列にまとめる。
// ok が偽になるのは、値を 1 つでも読めないときである。
//
// 値の前に値の byte 数を置く。値がどの byte を含んでも、異なる値の組が同じ文字列にならない。
func comparedValuesKey(
	specs []core.MatchConditionSpec, valueOf func(core.MatchConditionSpec) (string, bool),
) (string, bool) {
	var key strings.Builder
	for _, spec := range specs {
		value, readable := valueOf(spec)
		if !readable {
			return "", false
		}
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String(), true
}

// counterpartMatchValue は候補の側の値を、宣言が挙げた語彙の項目の並びの先頭から探して返す。
//
// backend/core の counterpartValue と同じ規則で値を選ぶ。読めない語彙の項目を飛ばし、次の
// 語彙の項目を探す。
func counterpartMatchValue(
	observation core.MatchObservation, spec core.MatchConditionSpec,
) (string, bool) {
	for _, semantic := range spec.CounterpartSemantics {
		if value, readable := matchValueOf(observation, semantic); readable {
			return value, true
		}
	}
	return "", false
}

// matchValueOf は観測から語彙の項目で指した最初の要素の、文字列の一致を比べる値を返す。
//
// backend/core の observationValue と同じ規則で値を選ぶ。比べるのは文字列の項目だけである。
func matchValueOf(observation core.MatchObservation, semantic core.SemanticKey) (string, bool) {
	field, found := observation.FieldBySemantic(semantic)
	if !found || field.Kind != core.RecordFieldKindText || field.Text == nil {
		return "", false
	}
	return field.Text.ComparableValue()
}

// specsKeyOf は条件の宣言の組を 1 つの文字列にまとめる。宣言の並びも文字列に入る。
//
// 各要素の前に byte 数を置き、要素がどの byte を含んでも異なる組が同じ文字列にならない。
func specsKeyOf(specs []core.MatchConditionSpec) string {
	var key strings.Builder
	write := func(value string) {
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	for _, spec := range specs {
		write(string(spec.ConditionKey))
		write(string(spec.OriginSemantic))
		write(strconv.Itoa(len(spec.CounterpartSemantics)))
		for _, semantic := range spec.CounterpartSemantics {
			write(string(semantic))
		}
		write(strconv.FormatBool(spec.Compared))
	}
	return key.String()
}
