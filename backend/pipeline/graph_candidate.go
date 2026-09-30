package pipeline

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 候補のエッジが依拠する前提と、時計に依拠する箇所の文。分析者が画面で読む値であるため
// 日本語で書く。
const (
	clockOffsetAssumptionStatement = "2 つの収集元の時計のずれが 1 秒未満である"
	clockOffsetUnresolvedReason    = "2 つの収集元の時計のずれを測れる値が入力に無い"
	clockDependencyNoteText        = "IP から端末への割当を適用してよい期間と起点のレコードの時刻の比較が、収集元の時計に依拠する"
)

// destinationKey は接続先 IP と接続先 port の文字列の組である。
type destinationKey struct {
	ip   string
	port string
}

// matchSide は関連付けに渡すレコード 1 件と、そのレコードの語彙の項目である。
type matchSide struct {
	record RecordEntry
	fields []core.RecordField
	state  core.PublicationState
	// itemSemantics は、このレコードの収集元が運びうる語彙の項目である。
	// 「相手の入力形式にその欄が無い」と言えるかの判定に用いる。
	itemSemantics []core.SemanticKey
	// timePrecision は、このレコードの収集元の時刻の精度である。
	timePrecision core.Precision
	// matchConditions は、このレコードの収集元が関連付けの条件へ差し出す宣言である。
	// 起点になったときに、段階が持つ条件の一覧と並びを決める。
	matchConditions []ConnectionMatchCondition
	// terminal は、端末の外部識別子の欄を持たない候補のレコードに、レコードを置いた端末の鍵から
	// 導いた terminal.id の項目である (placedTerminalFieldOf)。欄を持つ候補と起点では nil である。
	terminal *core.RecordField
	// processRef は候補のレコードが記録したプロセスである。起点では nil である。
	processRef *core.ProcessRef
}

// candidateTerminalFieldName は、候補のレコードを置いた端末から導いた terminal.id の項目の
// 名前である。原資料の key ではないため、導出であることが分かる名前にする。
const candidateTerminalFieldName = "placedTerminal"

// derivationPlacedTerminal は、候補の端末の項目を、レコードを置いた端末の鍵から導いたことを
// 表す文字列である。分析者が画面で読む値であるため日本語で書く。
const derivationPlacedTerminal = "レコードを置いた端末 (取り込みの指定と分析者の割当、収集元の範囲から決めた端末)"

// candidateOriginOutcomes は、起点が取りうる分類の全数を、数え上げの並び順で返す。
//
// **起点にならなかったレコードの分類を外す。** 起点の数え上げの母集団は、関係を導く
// 起点になったレコードである。
func candidateOriginOutcomes() []core.RelationDerivationOutcome {
	outcomes := make([]core.RelationDerivationOutcome, 0,
		len(core.KnownRelationDerivationOutcomes()))
	for _, outcome := range core.KnownRelationDerivationOutcomes() {
		if outcome == core.RelationDerivationNotUsedAsOrigin {
			continue
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// CandidateOriginCount は分類 1 件と、その分類になった起点の件数である。
type CandidateOriginCount struct {
	// Outcome は関連付けの結果の分類である。
	Outcome core.RelationDerivationOutcome
	// OriginCount はその分類になった起点のレコードの件数である。
	OriginCount int
}

// CandidateOriginCounts は関連付けの起点を分類ごとに数えた結果を、分類の並び順で返す。
//
// **件数 0 の分類も要素にする。** 分類そのものが結果から消えると、その経路を通る起点が
// 1 件も無かったのか、分類を数えていないのかを読み分けられない。
func (g Graph) CandidateOriginCounts() []CandidateOriginCount {
	tally := make(map[core.RelationDerivationOutcome]int, len(g.originOutcomes))
	for _, outcome := range g.originOutcomes {
		tally[outcome]++
	}
	counts := make([]CandidateOriginCount, 0, len(candidateOriginOutcomes()))
	for _, outcome := range candidateOriginOutcomes() {
		counts = append(counts, CandidateOriginCount{
			Outcome: outcome, OriginCount: tally[outcome],
		})
	}
	return counts
}

// OriginsWithoutRecordNode は、レコードのノードを組めなかった起点の件数である。
//
// **0 でない値は内部で処理できなかったことである。** その起点の結果はノードの詳細から読めない。
func (g Graph) OriginsWithoutRecordNode() int { return g.originsWithoutRecordNode }

// RelationDerivationOf は、レコードのノード 1 つを起点にして関係を導いた結果を返す。
// 起点にならなかったレコードのノードでは not_used_as_origin を返す。
func (g Graph) RelationDerivationOf(nodeId string) (core.RelationDerivation, bool) {
	index, found := g.nodeAt[nodeId]
	if !found || g.nodes[index].key.Kind != core.NodeKindRecord {
		return core.RelationDerivation{}, false
	}
	outcome, used := g.originOutcomes[nodeId]
	if !used {
		outcome = core.RelationDerivationNotUsedAsOrigin
	}
	derivation, err := core.NewRelationDerivation(outcome)
	if err != nil {
		return core.RelationDerivation{}, false
	}
	return derivation, true
}

// CandidateRecordCounts は、起点ごとに除いた候補の延べ件数である。
//
// **起点の分類では表せない、処理できなかった候補である。** 1 つの起点が候補を 5 件持ち 3 件だけを
// 除いたとき、その起点は matched になる。除いた側を数えるのが本型である。
//
// **レコードの異なり数ではない。** 候補の集合は接続先の文字列で共有するため、同じ 1 件の
// レコードを 2 つの起点が除くと 2 回数える。
type CandidateRecordCounts struct {
	// Unreadable は関連付けが比べる語彙の項目を比べられる形で持たず、候補にできなかった組の
	// 延べ件数である。段階 1 より前で除かれる。
	Unreadable int
	// Dropped は段階 2 に残りながら、ノードまたはレコードをグラフから探せずエッジへ写せなかった
	// 組の延べ件数である。
	Dropped int
}

// CandidateRecordCounts は、起点ごとに除いた候補の延べ件数を返す。
func (g Graph) CandidateRecordCounts() CandidateRecordCounts {
	return CandidateRecordCounts{
		Unreadable: g.unreadableCandidates, Dropped: g.droppedCandidates,
	}
}

// SourceDeclarationProblem は、収集元の宣言の組から段階の条件を組めなかった理由を返す。
//
// source_declaration_conflict に数えた起点があるときだけ値を持つ。error 文が含むのは
// 条件の種別と語彙の項目の文字列であり、原資料の byte を含まない。
func (g Graph) SourceDeclarationProblem() error {
	return g.declarationProblem
}

// CandidateSetProblem は、候補集合の構築が退けた理由を返す。
//
// candidate_set_failed に数えた起点があるときだけ値を持つ。error 文が含むのは項目の
// 名前と検査の種別であり、原資料の byte を含まない。
func (g Graph) CandidateSetProblem() error {
	return g.candidateSetProblem
}

// candidateSides は関連付けの起点になるレコードと、候補になるレコードを分けた組である。
//
// **起点と候補を入力形式の名前で分けない。** 端末が決まりプロセスを記録したレコードが候補の
// 側、プロセスを記録しない要求のレコードが起点の側である。起点の端末は、別の収集元の IP から
// 端末への割当を適用して導く。候補の端末は、レコードの端末の外部識別子か、レコードを置いた
// 端末である。
//
// **候補は、関連付けの条件の宣言が同じ収集元ごとに 1 つの組に集める** (candidateSideGroups)。
// 接続先で候補を絞れる収集元と絞れない収集元を 1 つの組に混ぜると、組の条件を組めない。
// 起点は全部の組で同じである。
type candidateSides struct {
	origins       []matchSide
	byDestination map[destinationKey][]matchSide
	// candidates は候補の側のレコードのうち、プロセスを記録したものを取り込んだ順に
	// 並べた全数である。接続先の条件を分析者が選ばなかった要求が母集合として走査する。
	//
	// **接続先の欄で絞らない。** 絞ると、その条件を選ばない要求でも接続先の欄を持たない
	// レコードが候補から外れる。プロセスの記録だけを求めるのは、候補のエッジの終点に
	// なるノードがそこで決まるためである。
	candidates []matchSide
	// candidateDeclarations は、この組の候補のレコードを出した収集元の宣言である。起点の
	// 関連付けが相手の宣言に用いる。
	candidateDeclarations sideDeclarations
}

// newCandidateSides は候補を 1 件も持たない組を組む。
func newCandidateSides() candidateSides {
	return candidateSides{
		byDestination:         make(map[destinationKey][]matchSide),
		candidateDeclarations: newSideDeclarations(),
	}
}

// addCandidateEdges は関連付けが挙げた候補を、候補のエッジとしてグラフへ足す。
//
// 起点 1 件につき候補集合を 1 つ組み、最後の段階の候補をエッジにする。最後の段階は分析者が
// 選んだ条件が決める。時刻の条件を選んだ要求では秒単位の時刻まで一致した段階、選ばない
// 要求では時刻を比べない段階である。
//
// エッジの起点は起点のレコードが指す接続先のノード、終点は候補のレコードが記録した
// プロセスのノードである。**同じ 2 つのノードを結ぶ関連付けが 2 件以上あるときは 1 本にまとめ、
// 関連付け 1 件ごとの条件を matches が保つ。**
//
// **手前の段階の件数を関連付けが持つ。** 段階ごとの候補の件数は EdgeMatch.StageTallies が持ち、
// 時刻の条件がどれだけ候補を減らしたかを 1 本のエッジから読める。
//
// **候補の母集合と、母集合から組む候補の観測を、探す鍵ごとに 1 度だけ組む** (candidateLookup)。
// 起点ごとの所要は、段階 1 の比べる値が起点と一致する候補の件数で決まる。
//
// **候補集合は、起点を candidateOriginChunk 件ずつ区切って同時に組む。** 要求を組む段階と
// エッジへ写す段階は起点の並びの順に 1 件ずつ行うため、エッジの並びと件数と退けた理由は
// 起点を 1 件ずつ処理したときと同じになる。
//
// **候補の組ごとに、起点の全件で関連付けを組む** (candidateSideGroups)。起点の分類は組ごとの
// 分類をまとめる (recordOriginOutcome)。
func (g *Graph) addCandidateEdges(result ImportResult) {
	assignments := terminalAssignmentsOf(result)
	proxyAddresses := sourceTerminalAddressesOf(result)
	for groupIndex, sides := range g.candidateSideGroups(result) {
		lookup := newCandidateLookup(sides, assignments, g.matchSelection)
		lookup.proxyAddresses = proxyAddresses
		pending := make([]originDerivation, 0, min(candidateOriginChunk, len(sides.origins)))
		for start := 0; start < len(sides.origins); start += candidateOriginChunk {
			chunk := sides.origins[start:min(start+candidateOriginChunk, len(sides.origins))]
			pending = pending[:0]
			for _, origin := range chunk {
				pending = append(pending, g.prepareOriginDerivation(result, lookup, origin))
			}
			buildCandidateSets(pending)
			for index := range pending {
				g.recordOriginOutcome(pending[index].origin, g.finishOriginDerivation(pending[index]),
					groupIndex == 0)
				pending[index] = originDerivation{}
			}
		}
	}
}

// candidateOriginChunk は、候補集合を同時に組む起点の件数の上限である。
//
// 既知の制限: 区切りの件数を固定の値に置く,
// 区切りの件数を変えた所要は repo の fixture の大きさでは測れない,
// 区切りの中の候補集合が保つ記憶域が問題になったときに見直す。
const candidateOriginChunk = 256

// buildCandidateSets は起点ごとの候補集合を、CPU の数までの goroutine で同時に組む。
func buildCandidateSets(pending []originDerivation) {
	workers := min(runtime.GOMAXPROCS(0), len(pending))
	if workers <= 1 {
		for index := range pending {
			pending[index].buildSet()
		}
		return
	}
	var next atomic.Int64
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			for {
				index := int(next.Add(1)) - 1
				if index >= len(pending) {
					return
				}
				pending[index].buildSet()
			}
		})
	}
	group.Wait()
}

// recordOriginOutcome は、起点 1 件の結果をレコードのノードの識別子で探せる形に残す。
//
// **レコードの位置でなくノードの識別子を鍵にする。** ノードの詳細はノードの位置しか
// 持たず、レコードを探す鍵 (収集元と位置) とノードの識別鍵 (内容の digest と位置) は
// 材料が違うため、鍵から鍵へ変換できない。
//
// **ノードを組めなかった起点を通知せずに除かない。** 件数を別に数え、log に載せる。件数は
// 最初の候補の組 (counted が真) でだけ数える。
//
// **候補の組が 2 つ以上あるときは、組ごとの分類を outcomePrecedence の順位で 1 つにまとめる。**
// 組の並び (取り込みの順) に依らない。候補を挙げた組があれば matched、内部で処理できなかったことは他の
// 組の分類で隠さず、関連付けが先の段階まで進んだ組の分類を採る。
func (g *Graph) recordOriginOutcome(
	origin matchSide, outcome core.RelationDerivationOutcome, counted bool,
) {
	key, buildable := core.NewRecordNodeKey(origin.record.Locator)
	if !buildable {
		if counted {
			g.originsWithoutRecordNode++
		}
		return
	}
	id := nodeIdOf(key)
	if previous, recorded := g.originOutcomes[id]; recorded &&
		outcomeRank(previous) <= outcomeRank(outcome) {
		return
	}
	g.originOutcomes[id] = outcome
}

// outcomePrecedence は、候補の組ごとの起点の分類をまとめるときの優先の順である。先の分類ほど
// 優先する。
var outcomePrecedence = []core.RelationDerivationOutcome{
	core.RelationDerivationMatched,
	core.RelationDerivationFailed,
	core.RelationDerivationNodeUnresolved,
	core.RelationDerivationSourceDeclarationConflict,
	core.RelationDerivationDestinationIpAbsent,
	core.RelationDerivationDestinationPortAbsent,
	core.RelationDerivationTerminalUndetermined,
	core.RelationDerivationTerminalIdAbsent,
	core.RelationDerivationOriginItemUnreadable,
	core.RelationDerivationNoCandidateInWindow,
	core.RelationDerivationNoCandidateMatchingConditions,
	core.RelationDerivationCandidateItemUnreadable,
	// Proxy のアドレスが無いことは、Windows の候補の組だけが言う。他の組が関連付けを進めた
	// 分類を隠さない。
	core.RelationDerivationProxyAddressUnknown,
	core.RelationDerivationNoComparedCondition,
	core.RelationDerivationNoCandidateRecord,
}

// outcomeRank は outcomePrecedence の中の位置を返す。並びに無い分類は最も後ろである。
func outcomeRank(outcome core.RelationDerivationOutcome) int {
	if at := slices.Index(outcomePrecedence, outcome); at >= 0 {
		return at
	}
	return len(outcomePrecedence)
}

// candidateSideGroups は公開中の収集元のレコードを、起点の側と候補の組へ分ける。組の並びは、
// その宣言の収集元が最初に現れた順である。候補が 1 件も無いときも、起点を持つ組を 1 つ返す。
//
// **起点は、接続を記録し、プロセスを記録しないレコードである。** 端末の割当を付けた Proxy の
// レコードも起点に残る。**候補は、プロセスを記録し、接続 (接続の組、または接続元 port と
// 接続先のアドレス) を記録し、端末が決まるレコードである。** 接続先だけを記録したレコード
// (資格情報を指定したログオンの要求、URL の要求先) は接続を記録していない。端末は、レコードの端末の外部識別子か、レコードを置いた端末である
// (graphRecord.placedTerminalNodeId)。どちらも持たないレコードと、時刻を持たないレコードは、
// どちらの側にも入らない。
func (g Graph) candidateSideGroups(result ImportResult) []candidateSides {
	var origins []matchSide
	var groups []candidateSides
	groupAt := make(map[string]int)
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		groupKey := declarationsKeyOf(publication.ConnectionMatchConditions())
		for _, record := range publication.records {
			if record.Semantics == nil || record.ObservedAt == nil {
				continue
			}
			at, indexed := g.recordAtLocator(record.Locator)
			if !indexed {
				continue
			}
			process, recordsProcess := g.recordedProcessAt(at)
			if !recordsProcess && record.Semantics.ProcessRef == nil {
				if record.Semantics.Endpoint != nil {
					origins = append(origins, matchSideOf(publication, record))
				}
				continue
			}
			if record.Semantics.Endpoint == nil && !recordsConnection(record.Semantics.Fields) {
				continue
			}
			side, placed := g.candidateSideOf(publication, record, at, process, recordsProcess)
			if !placed {
				continue
			}
			index, found := groupAt[groupKey]
			if !found {
				index = len(groups)
				groupAt[groupKey] = index
				groups = append(groups, newCandidateSides())
			}
			groups[index].candidateDeclarations.add(publication)
			groups[index].addCandidate(side)
		}
	}
	if len(groups) == 0 {
		groups = append(groups, newCandidateSides())
	}
	for index := range groups {
		groups[index].origins = origins
	}
	return groups
}

// recordsConnection は、レコードが接続元 port と接続先のアドレスを記録しているかを返す。
func recordsConnection(fields []core.RecordField) bool {
	return carriesSemanticValue(fields, core.SemanticKeyConnectionSourcePort) &&
		carriesSemanticValue(fields, core.SemanticKeyConnectionDestinationAddress)
}

// matchSideOf は収集元のレコード 1 件を、関連付けに渡す形にする。
func matchSideOf(publication SourcePublication, record RecordEntry) matchSide {
	return matchSide{
		record: record, fields: graphFieldsOf(record),
		state:           publication.status.PublicationState,
		itemSemantics:   publication.ItemSemantics(),
		matchConditions: publication.ConnectionMatchConditions(),
		timePrecision:   publication.TimePrecision(),
	}
}

// candidateSideOf は候補のレコード 1 件を、端末とプロセスを添えて関連付けに渡す形にする。
// ok が偽になるのは、端末が決まらないレコードである。
//
// **端末の外部識別子の欄を持つレコードは、その欄で比べる。** 持たないレコードは、置いた端末の
// 鍵から terminal.id の項目を導く (placedTerminalFieldOf)。
//
// **プロセスの参照を持たないレコードは、記録したプロセスのノードから参照を組む。** プロセス
// 番号だけを記録した収集元では、区間で識別したプロセスのノードを指す。
func (g Graph) candidateSideOf(
	publication SourcePublication, record RecordEntry, at, process int, recordsProcess bool,
) (matchSide, bool) {
	side := matchSideOf(publication, record)
	side.processRef = record.Semantics.ProcessRef
	var terminal core.NodeKey
	if ownId, owns := comparableOfSemantic(side.fields, core.SemanticKeyTerminalId); owns {
		key, keyed := core.TerminalNodeKey(ownId)
		if !keyed {
			return matchSide{}, false
		}
		terminal = key
	} else if len(fieldsWithSemantic(side.fields, core.SemanticKeyTerminalId)) > 0 {
		// 読めない外部識別子の欄を持つレコードは、置いた端末で置き換えない。
		return matchSide{}, false
	} else {
		placed, found := g.placedTerminals[g.records[at].placedTerminalNodeId]
		if !found {
			return matchSide{}, false
		}
		field, built := placedTerminalFieldOf(placed)
		if !built {
			return matchSide{}, false
		}
		terminal, side.terminal = placed, &field
	}
	if side.processRef == nil && recordsProcess {
		ref := core.ProcessRef{
			SourceId: record.Locator.SourceId, SourceContentSha256: record.Locator.SourceContentSha256,
			ProcessId: g.nodes[process].id, TerminalNodeId: nodeIdOf(terminal),
		}
		if terminal.Form == core.NodeKeyFormTerminalId {
			ref.TerminalId = terminal.Values[0].Value
		}
		side.processRef = &ref
	}
	return side, true
}

// placedTerminalFieldOf は、端末の鍵から、関連付けが端末を比べる terminal.id の項目を組む
// (core.TerminalMatchValue)。値の状態は derived である。
func placedTerminalFieldOf(terminal core.NodeKey) (core.RecordField, bool) {
	text, valued := core.TerminalMatchValue(terminal)
	if !valued {
		return core.RecordField{}, false
	}
	value, err := core.NewDerivedValue(text, derivationPlacedTerminal)
	if err != nil {
		return core.RecordField{}, false
	}
	field, err := core.NewTextField(candidateTerminalFieldName, core.SemanticKeyTerminalId, value)
	if err != nil {
		return core.RecordField{}, false
	}
	return field, true
}

// addCandidate は候補になるレコードを母集合へ入れ、接続先の文字列で探せる表にも入れる。
//
// **接続先を比べられないレコードも母集合に入れる。** 接続先の条件を分析者が選ばない要求は
// その欄を関連付けに用いないため、欄の有無で候補を除くと、応答は「その条件で絞っていない」と
// 書きながら実際にはその欄で絞った結果を出すことになる。接続先の文字列で探す表には、
// 両方の欄を比べられるレコードだけが入る。
func (s *candidateSides) addCandidate(side matchSide) {
	s.candidates = append(s.candidates, side)
	destinationIp, hasIp := comparableOfSemantic(
		side.fields, core.SemanticKeyConnectionDestinationAddress)
	destinationPort, hasPort := comparableOfSemantic(
		side.fields, core.SemanticKeyConnectionDestinationPort)
	if !hasIp || !hasPort {
		return
	}
	key := destinationKey{ip: destinationIp, port: destinationPort}
	s.byDestination[key] = append(s.byDestination[key], side)
}

// prepareOriginDerivation は起点 1 件について、候補集合を組む要求を組む。要求を組めない
// 起点では、その理由の分類を持つ値を返す。
//
// **要求先の authority がホスト名である起点も候補を組む。** その起点は相手の接続先 IP と
// 比べられる値を持たないため、接続先 IP の条件を絞りに用いず (namesHostnameOnly)、
// 残りの選んだ条件で候補を絞る。候補のエッジの起点はホスト名のノードである。
func (g *Graph) prepareOriginDerivation(
	result ImportResult, lookup *candidateLookup, origin matchSide,
) originDerivation {
	derivation := originDerivation{origin: origin}
	destination, named := originDestinationOf(origin)
	if !named {
		derivation.outcome = core.RelationDerivationDestinationIpAbsent
		return derivation
	}
	derivation.destination = destination
	// 接続先 port を選び、候補の組がその条件で絞れる要求だけが、その欄を持たない起点を欠測と
	// して分ける。それ以外の要求では、その欄の有無が候補の絞り込みに作用しない。
	_, byPort := g.matchSelection.narrows(core.ConditionKeyDestinationPort)
	if byPort && lookup.sides.candidateDeclarations.narrows(core.ConditionKeyDestinationPort) {
		if _, hasPort := comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationPort); !hasPort {
			derivation.outcome = core.RelationDerivationDestinationPortAbsent
			return derivation
		}
	}
	proxyAddresses, restricts := lookup.proxyAddressesOf(origin)
	if restricts && len(proxyAddresses) == 0 {
		derivation.outcome = core.RelationDerivationProxyAddressUnknown
		return derivation
	}
	derivation.pool = lookup.poolFor(origin, proxyAddresses)
	if derivation.pool.size() == 0 {
		derivation.outcome = core.RelationDerivationNoCandidateRecord
		return derivation
	}
	derivation.request, derivation.priorTallies, derivation.outcome = g.matchRequestOf(
		result, lookup.assignments, origin, derivation.pool, lookup.sides.candidateDeclarations)
	return derivation
}

// originDestinationOf は起点のレコードが指す接続先の文字列を返す。接続先 IP を比べられる
// 形で持つ起点は接続先 IP を、持たない起点は要求先のホスト名を返す。ok が偽になるのは、
// どちらも比べられる形で持たないときである。
func originDestinationOf(origin matchSide) (string, bool) {
	if destinationIp, hasIp := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationAddress); hasIp {
		return destinationIp, true
	}
	return comparableOfSemantic(origin.fields, core.SemanticKeyConnectionDestinationHostname)
}

// namesHostnameOnly は、起点が接続先を要求先のホスト名だけで指すかを返す。真になるのは、
// 接続先 IP を比べられる形で持たず、ホスト名を比べられる形で持つ起点である。
//
// **その起点は、接続先 IP の条件を選んだ要求でも、その条件で候補を絞らない。** 要求先の
// authority がホスト名である起点は名前解決の結果を持たず、相手の接続先 IP と比べる値が無い。
// その条件の use は no_comparable_counterpart になる (MatchConditionSelection.destinationIpUse)。
func namesHostnameOnly(origin matchSide) bool {
	if _, hasIp := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationAddress); hasIp {
		return false
	}
	_, hasHostname := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationHostname)
	return hasHostname
}

// originDerivation は起点 1 件について、候補集合を組む前の分類と要求と、組んだ候補集合である。
type originDerivation struct {
	origin matchSide
	// destination は起点のレコードが指す接続先の文字列である (originDestinationOf)。
	// 候補のエッジの起点になるノードを探す。
	destination string
	pool        *candidatePool
	// outcome が matched のときだけ、request から set と err を組む。
	outcome core.RelationDerivationOutcome
	request core.MatchRequest
	// priorTallies は、要求に求めなかった手前の段階を数えた結果である (timeBoundRequestOf)。
	// 候補集合の段階の数え上げの前に並ぶ。
	priorTallies []core.MatchStageTally
	set          core.CandidateSet
	err          error
}

// buildSet は要求から候補集合を組む。分類が matched でない起点では何もしない。
//
// **グラフと母集合を書き換えない。** 複数の起点の buildSet を同時に呼べる。
func (d *originDerivation) buildSet() {
	if d.outcome != core.RelationDerivationMatched {
		return
	}
	d.set, d.err = core.BuildCandidateSet(d.request)
	d.request = core.MatchRequest{}
}

// finishOriginDerivation は組んだ候補集合をエッジへ写し、起点の分類を返す。
func (g *Graph) finishOriginDerivation(derivation originDerivation) core.RelationDerivationOutcome {
	if derivation.outcome != core.RelationDerivationMatched {
		return derivation.outcome
	}
	origin, destination, pool := derivation.origin, derivation.destination, derivation.pool
	set, err := derivation.set, derivation.err
	if err != nil {
		// 起点と候補の値が比べられることと、要求の項目が揃っていることを matchRequestOf が
		// 確かめている。残る error は取り込み結果が core の検査を通らない状態であり、
		// その起点の候補を挙げずに次の起点へ進む。グラフ全体の構築を止めない。
		//
		// **件数だけで終わらせない。** 分類の件数からは、どの検査が退けたかへ到達できない。
		// 最初の 1 件を保って呼び出し元が log に載せる。
		if g.candidateSetProblem == nil {
			g.candidateSetProblem = err
		}
		return core.RelationDerivationFailed
	}
	return g.applyCandidateSet(set, derivation.priorTallies, origin, destination, pool)
}

// matchRequestOf は起点 1 件と候補の母集合から関連付けの要求を組む。
// 分類が matched 以外になるのは、関連付けが用いる値を両側が比べられる形で持たないときである。
//
// **候補にできなかったレコードを数える。** 5 件のうち 3 件だけが読めない状態は、起点の
// 分類に出ない (その起点は残る 2 件で関連付けを実行する)。数えるのは母集合の全件についてである。
//
// **要求に入れる候補は、段階 1 の比べる値が起点と一致しうる候補だけである**
// (candidateMembers.narrowedFor)。相手の収集元が持つ語彙の項目と時刻の精度は、母集合の
// 全件から導く。
func (g *Graph) matchRequestOf(
	result ImportResult, assignments clientAssignments, origin matchSide,
	pool *candidatePool, counterpart sideDeclarations,
) (core.MatchRequest, []core.MatchStageTally, core.RelationDerivationOutcome) {
	terminalAssignments, resolved := resolveOriginTerminal(assignments, origin)
	if !resolved {
		return core.MatchRequest{}, nil, core.RelationDerivationTerminalUndetermined
	}
	// 確定した割当はすべて同じ端末を指す (core.ResolveTerminal)。端末と適用期間は先頭の割当で読む。
	assignment := terminalAssignments[0]
	// 端末のノードを指さない割当は、候補の端末と比べる値を持たない。
	if _, keyed := assignment.TerminalNodeKey(); !keyed {
		return core.MatchRequest{}, nil, core.RelationDerivationTerminalIdAbsent
	}
	specs, declarationProblem := candidateConditionSpecs(origin, counterpart, g.matchSelection)
	if declarationProblem != nil {
		g.declarationProblem = declarationProblem
		return core.MatchRequest{}, nil, core.RelationDerivationSourceDeclarationConflict
	}
	// **絞る条件を 1 つも持たない関連付けを組み立てない。** 分析者が選んだ条件を、この起点の
	// 収集元が候補の絞りに 1 つも使えない状態である。core は絞る条件を持たない要求を退ける
	// ため、内部で処理できなかったことと混ぜずにここで分ける。
	if !anyComparedCondition(specs) {
		return core.MatchRequest{}, nil, core.RelationDerivationNoComparedCondition
	}
	timeBound := timeBoundSpecs(counterpart, specs)
	// 端末だけで絞る組は、時刻を比べない選択では、端末の全部の接続を候補に挙げる。
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
	members := pool.membersFor(specs)
	g.unreadableCandidates += members.unreadable
	// 接続先の文字列が一致するレコードはあるが、そのすべてが関連付けが比べる語彙の項目を
	// 比べられる形で持たない状態である。レコードが 0 件である状態と分けて数える。
	if len(members.records) == 0 {
		return core.MatchRequest{}, nil, core.RelationDerivationCandidateItemUnreadable
	}
	parts := matchRequestParts{
		result: result, origin: origin, assignment: assignment, observation: observation,
		window: window, specs: specs,
		counterpartItemSemantics: pool.itemSemantics,
		counterpartTimePrecision: pool.timePrecision,
		counterpart:              counterpart, selection: g.matchSelection,
		proxyAddresses: pool.proxyAddresses,
	}
	var prior []core.MatchStageTally
	if timeBound {
		narrowed, tally, outcome := members.timeNarrowedFor(observation, specs, window)
		if outcome != core.RelationDerivationMatched {
			return core.MatchRequest{}, nil, outcome
		}
		parts.members, parts.stageKeys, prior = narrowed, []core.StageKey{core.StageKeySecondTimeMatched},
			[]core.MatchStageTally{tally}
	} else {
		parts.members = members.narrowedFor(observation, specs)
	}
	request, err := matchRequestOfMembers(parts)
	if err != nil {
		return core.MatchRequest{}, nil, core.RelationDerivationOriginItemUnreadable
	}
	return request, prior, core.RelationDerivationMatched
}

// timeBoundSpecs は、段階 1 の条件で候補を端末だけに絞る組かを返す。真になるのは、候補の組が
// 接続先の条件で候補を絞れず、端末の他に比べる条件を持たない要求である。
//
// **その組の段階 1 は、端末の全部の接続である。** 起点ごとに段階 1 の候補を全部組むと、所要と
// 記憶域が起点の件数と端末の接続の件数の積で増える。その組は時刻の範囲で候補を先に絞り、
// 段階 2 だけを core に求める (candidateMembers.timeNarrowedFor)。
func timeBoundSpecs(counterpart sideDeclarations, specs []core.MatchConditionSpec) bool {
	if counterpart.narrows(core.ConditionKeyDestinationIp) ||
		counterpart.narrows(core.ConditionKeyDestinationPort) {
		return false
	}
	for _, spec := range specs {
		if spec.Compared && spec.ConditionKey != core.ConditionKeyTerminalIpAssignment {
			return false
		}
	}
	return true
}

// matchRequestParts は関連付けの要求 1 つを組む材料である。
type matchRequestParts struct {
	result      ImportResult
	origin      matchSide
	assignment  core.TerminalAssignment
	observation core.MatchObservation
	window      core.TimeWindow
	members     narrowedCandidates
	specs       []core.MatchConditionSpec
	// counterpartItemSemantics と counterpartTimePrecision は、候補の母集合の全件から導いた
	// 相手の収集元の語彙の項目と時刻の精度である。
	counterpartItemSemantics []core.SemanticKey
	counterpartTimePrecision core.Precision
	counterpart              sideDeclarations
	selection                MatchConditionSelection
	// stageKeys は求める段階である。nil のときは選択が決める段階 (MatchConditionSelection.stageKeys) である。
	stageKeys []core.StageKey
	// proxyAddresses は候補の接続先を限った Proxy のアドレスである (candidatePool.proxyAddresses)。
	proxyAddresses []string
}

// proxyAssumptionOf は、候補の接続先を Proxy のアドレスに限った関連付けが依拠する前提を返す。
// 限っていないときは ok が偽である。
func proxyAssumptionOf(proxyAddresses []string) (core.MatchAssumption, bool) {
	if len(proxyAddresses) == 0 {
		return core.MatchAssumption{}, false
	}
	return core.MatchAssumption{
		AssumptionKey: core.AssumptionKeyCounterpartConnectedToProxy,
		EvidenceClass: core.EvidenceClassInferred,
		Statement: "候補の接続先が Proxy のログの収集元に付けた端末の IP アドレス (" +
			strings.Join(proxyAddresses, ", ") + ") である接続を、Proxy への接続とみなす",
	}, true
}

// matchRequestOfMembers は両側の観測と時刻の範囲から要求の組を作る。
func matchRequestOfMembers(parts matchRequestParts) (core.MatchRequest, error) {
	result, origin, assignment := parts.result, parts.origin, parts.assignment
	observation, window, members := parts.observation, parts.window, parts.members
	specs, counterpart := parts.specs, parts.counterpart
	stageKeys := parts.stageKeys
	if stageKeys == nil {
		stageKeys = parts.selection.stageKeys()
	}
	// 候補の組が接続先 IP で絞れないときは、起点の接続先 IP を比べる相手が無い。
	hostnameOnly := namesHostnameOnly(origin) ||
		!counterpart.narrows(core.ConditionKeyDestinationIp)
	// 未処理の範囲を持たない集合として組む。**候補のエッジが持つのは関連付け 1 件ごとの条件と
	// 前提であり、取り込みの未処理の範囲は `/api/v0/sources` の応答が持つ** (import_status.go)。
	request := core.MatchRequest{
		Origin: core.MatchOrigin{
			Observation:          observation,
			DestinationIpUse:     parts.selection.destinationIpUse(hostnameOnly),
			AssignmentValidRange: assignment.AssignmentValidRange,
		},
		Candidates:               members.records,
		Conditions:               specs,
		CounterpartItemSemantics: slices.Clone(parts.counterpartItemSemantics),
		CounterpartTimePrecision: parts.counterpartTimePrecision,
		StageKeys:                stageKeys,
		IncludedObservationKinds: counterpart.observationKinds(),
		SecondStageWindow:        window,
		SecondStageAssumptions:   parts.selection.assumptions(),
		ClockDependencyNote:      clockDependencyNoteText,
		PublicationState:         origin.state,
		UnprocessedRanges:        []core.RecordRange{},
		AnalysisRunRef:           result.AnalysisRunRef(),
	}
	if assumption, restricted := proxyAssumptionOf(parts.proxyAddresses); restricted {
		request.CandidateAssumptions = []core.MatchAssumption{assumption}
	}
	if members.checked {
		request = request.WithValidatedCandidates(members.validated)
	}
	return request, nil
}

// resolveOriginTerminal は起点の接続元 IP から端末を 1 つに確定させ、確定に使った割当を
// すべて返す。割当はどれも同じ端末を指す。ok が偽になるのは、端末が 1 つに確定しないときである。
func resolveOriginTerminal(
	assignments clientAssignments, origin matchSide,
) ([]core.TerminalAssignment, bool) {
	clientIp, readable := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionSourceAddress)
	if !readable {
		return nil, false
	}
	forClientIp := assignments[clientIp]
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

// originObservationOf は起点の観測を組む。
//
// **語彙の項目を持つ要素を入れる。** `/api/v0/records` の応答の fields の集合をそのまま渡すと、
// 接続先 port を持つ要素を欠いた観測になる。端末の外部識別子は別の収集元の割当から導いた値で
// あり、起点のレコードの項目に無い。
func originObservationOf(
	origin matchSide, members []core.TerminalAssignment, specs []core.MatchConditionSpec,
) (core.MatchObservation, bool) {
	terminal, composed := derivedTerminalFieldOf(origin, members)
	if !composed {
		return core.MatchObservation{}, false
	}
	fields, complete := comparedFieldsOf(origin.fields, requiredOriginSemantics(specs))
	if !complete {
		return core.MatchObservation{}, false
	}
	fields = append(fields, optionalFieldsOf(origin.fields, optionalOriginSemantics(specs))...)
	observation := core.MatchObservation{
		Ref:             cloneLocator(origin.record.Locator),
		EventTime:       cloneTimestamp(*origin.record.ObservedAt),
		ObservationKind: cloneObservationKind(origin.record.Semantics.ObservationKind),
		Fields:          append([]core.RecordField{terminal}, fields...),
	}
	if observation.Validate() != nil {
		return core.MatchObservation{}, false
	}
	return observation, true
}

// derivedTerminalFieldOf は接続元のアドレスの割当から導いた端末の外部識別子の項目を組む。
//
// 割当を探す鍵の name は、そのレコードが実際に持つ項目の name である
// (fields_builder.go の clientAddress)。`/api/v0/records` の clientTerminal と同じ derivation に
// なるよう、同じ経路で組む。members は同じ端末を指す割当であり (resolveOriginTerminal)、
// derivation にすべてを載せる。
func derivedTerminalFieldOf(
	side matchSide, members []core.TerminalAssignment,
) (core.RecordField, bool) {
	if len(members) == 0 {
		return core.RecordField{}, false
	}
	assignment := members[0]
	// 端末のノードを指さない割当は呼び出し元 (matchRequestOf) が terminal_id_absent として
	// 先に分ける。比べる値は候補の側と同じく端末のノードの鍵から導く (core.TerminalMatchValue)。
	terminal, keyed := assignment.TerminalNodeKey()
	if !keyed {
		return core.RecordField{}, false
	}
	text, valued := core.TerminalMatchValue(terminal)
	if !valued {
		return core.RecordField{}, false
	}
	matchKeyName, _ := clientAddress(side.fields)
	derivation := derivedTerminalDerivation(derivationItems{
		matchKey:    matchKey{name: matchKeyName, comparable: assignment.ClientIp},
		assignments: members,
	})
	value, err := core.NewDerivedValue(text, derivation)
	if err != nil {
		return core.RecordField{}, false
	}
	field, err := core.NewTextField(
		clientTerminalFieldName, core.SemanticKeyTerminalId, value)
	if err != nil {
		return core.RecordField{}, false
	}
	return field, true
}

// candidateObservationOf は候補 1 件の観測を組む。
// ok が偽になるのは、関連付けが比べる語彙の項目を持つ要素を欠くときである。
func candidateObservationOf(
	candidate matchSide, specs []core.MatchConditionSpec,
) (core.MatchCandidateRecord, bool) {
	terminal, found := firstFieldWithSemantic(candidate.fields, core.SemanticKeyTerminalId)
	if !found && candidate.terminal != nil {
		terminal, found = cloneRecordField(*candidate.terminal), true
	}
	if !found {
		return core.MatchCandidateRecord{}, false
	}
	fields, complete := comparedFieldsOf(candidate.fields, requiredCounterpartSemantics(specs))
	if !complete {
		return core.MatchCandidateRecord{}, false
	}
	fields = append(fields,
		optionalFieldsOf(candidate.fields, optionalCounterpartSemantics(specs))...)
	observation := core.MatchObservation{
		Ref:             cloneLocator(candidate.record.Locator),
		EventTime:       cloneTimestamp(*candidate.record.ObservedAt),
		ObservationKind: cloneObservationKind(candidate.record.Semantics.ObservationKind),
		Fields:          append([]core.RecordField{terminal}, fields...),
	}
	if observation.Validate() != nil {
		return core.MatchCandidateRecord{}, false
	}
	return core.MatchCandidateRecord{
		Observation: observation,
		ProcessRef:  clonePointer(candidate.processRef),
	}, true
}

// optionalFieldsOf は語彙の項目を持つ要素を、在るものだけ 1 件ずつ返す。
//
// 段階の条件が起点の側の値を出すために要る項目を集める。欠けても観測を組めなかったことに
// しない。持たない起点でも関連付けは進み、その条件の use が欄の有無を持つ。
func optionalFieldsOf(
	fields []core.RecordField, wanted []core.SemanticKey,
) []core.RecordField {
	selected := make([]core.RecordField, 0, len(wanted))
	for _, semantic := range wanted {
		if field, found := firstFieldWithSemantic(fields, semantic); found {
			selected = append(selected, field)
		}
	}
	return selected
}

// comparedFieldsOf は関連付けが比べる語彙の項目を持つ要素を、1 語彙の項目につき 1 件返す。
// ok が偽になるのは、1 つでも欠けるときである。
//
// **集合の全体を渡さない。** 同じ語彙の項目を持つ要素が 2 つある観測は、どちらの値を
// 比べたかが読めない (backend/core/matching.go の MatchObservation.Validate)。
func comparedFieldsOf(
	fields []core.RecordField, compared []core.SemanticKey,
) ([]core.RecordField, bool) {
	selected := make([]core.RecordField, 0, len(compared))
	for _, semantic := range compared {
		field, found := firstFieldWithSemantic(fields, semantic)
		if !found {
			return nil, false
		}
		selected = append(selected, field)
	}
	return selected, true
}

// firstFieldWithSemantic は語彙の項目を持つ最初の要素を複製して返す。
func firstFieldWithSemantic(
	fields []core.RecordField, semantic core.SemanticKey,
) (core.RecordField, bool) {
	for _, field := range fields {
		if field.Semantic == semantic {
			return cloneRecordField(field), true
		}
	}
	return core.RecordField{}, false
}

// applyCandidateSet は候補集合の段階 2 の候補を、候補のエッジへ写す。
// 返すのは、その起点に対する関連付けの結果の分類である。
//
// **同じ接続先への起点が 2 件以上あると、1 本のエッジに 2 つの候補集合の関連付けが並ぶ。**
// 関連付け 1 件ごとに起点のレコードの位置 (originRef) と、その起点に対する段階ごとの件数
// (stageTallies) と、互いに区別できない候補の組を添えるため、
// どの起点から出た関連付けかを応答から読み分けられる。
//
// **写せなかった候補を通知せずに捨てない。** エッジを 1 本でも足した起点は matched になるため、
// 処理できなかった個々の候補は droppedCandidates が数える。
//
// 互いに区別できない候補の組 (stage.IndistinguishableGroups) を、総数を添えずに写す。
// 組の数は候補の数以下である。
//
// **候補が 0 件になった理由は段階の emptyReason から読む。** 段階 1 で既に 0 件だった起点を、
// 時刻の範囲の中に候補が無かった起点と同じ文字列で数えない。
//
// **求めた 2 つの段階を持たない候補集合は内部で処理できなかったことである。** 要求は段階 1 と段階 2 の
// 両方を求め、段階を 1 つも持たない候補集合になる 2 つの状態 (公開の停止と、起点が割当の
// 期間の外にあること) はこの経路の手前で分かれる。公開を止めた収集元のレコードは
// collectCandidateSides が外し、期間の外にある起点は resolveOriginTerminal が外す。
//
// prior は、要求に求めなかった手前の段階を数えた結果である。段階 1 を求めなかった要求では、
// 段階 1 の代わりに prior が段階の数え上げの先頭に並び、時計に依拠する箇所は要求に渡した文である。
func (g *Graph) applyCandidateSet(
	set core.CandidateSet, prior []core.MatchStageTally, origin matchSide, destination string,
	pool *candidatePool,
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
	ends.matchStage, ends.compactStage = g.addMatchStage(ends, stage)
	groupSizes := g.indistinguishableTargetCounts(stage, pool)
	added := 0
	for _, member := range stage.Members {
		if g.addCandidateEdge(ends, stage, member, pool, groupSizes[recordKeyOf(member.RecordRef)]) {
			added++
		}
	}
	if added == 0 {
		return core.RelationDerivationNodeUnresolved
	}
	return core.RelationDerivationMatched
}

// indistinguishableTargetCounts は、段階の互いに区別できない候補の組ごとに、組の候補が指す
// ノードの異なりの数を求め、候補のレコードの鍵 (recordKeyOf) から探せる表にする。**同じノードを
// 指す候補だけの組は数に入れない。** その組はエッジを 1 本しか作らず、区別できない相手のエッジが
// 無い。
func (g *Graph) indistinguishableTargetCounts(stage core.CandidateStage, pool *candidatePool) map[string]int {
	counts := make(map[string]int)
	for _, group := range stage.IndistinguishableGroups {
		targets := make(map[int]struct{}, len(group))
		for _, locator := range group {
			if endpoint, resolvable := pool.endpointOf(g, locator); resolvable {
				targets[endpoint.target] = struct{}{}
			}
		}
		if len(targets) <= 1 {
			continue
		}
		for _, locator := range group {
			counts[recordKeyOf(locator)] = max(counts[recordKeyOf(locator)], len(targets))
		}
	}
	return counts
}

// candidateEdgeEnds は 1 つの起点から出る候補のエッジに共通する値である。
type candidateEdgeEnds struct {
	// source は起点のレコードが指す接続先のノードの位置である。
	source int
	// originAt は起点のレコードの根拠の位置である。
	originAt int
	// originRef は起点のレコードの位置である。
	originRef core.RecordLocator
	// clockDependencyNote は段階 1 が持つ、時計に依拠する箇所である。
	clockDependencyNote string
	// stageTallies は、この起点に対して候補集合が実行した段階を並び順で数えた結果である。
	stageTallies []core.MatchStageTally
	// matchStage は、この起点の関連付けが共有する段階の g.matchStages での位置である。
	// compactStage が偽のときは段階を足しておらず、関連付けを core.EdgeMatch のまま持つ。
	matchStage   int32
	compactStage bool
}

// stageTalliesOf は候補集合の段階を、集合が並べた順で数えた結果へ写す。
//
// **候補を 0 件にした段階も要素にする。** 時刻を使わない段階が何件に絞り、時刻の一致を
// 足した段階が何件に減らしたかを、関連付け 1 件から読めるようにする。
func stageTalliesOf(set core.CandidateSet) []core.MatchStageTally {
	tallies := make([]core.MatchStageTally, 0, len(set.Stages))
	for _, stage := range set.Stages {
		tallies = append(tallies, core.MatchStageTally{
			StageKey:             stage.StageKey,
			MemberCount:          stage.MemberCount,
			DistinctProcessCount: clonePointer(stage.DistinctProcessCount),
			EmptyReason:          stage.EmptyReason,
		})
	}
	return tallies
}

// addCandidateEdge は候補 1 件を、候補のエッジと関連付けへ写す。groupSize は、候補を含む互いに
// 区別できない候補の組が指すノードの数であり、組に入らない候補では 0 である
// (indistinguishableTargetCounts)。
// 偽を返すのは、候補のノードまたはレコードをグラフから探せないときである。
func (g *Graph) addCandidateEdge(
	ends candidateEdgeEnds, stage core.CandidateStage, member core.Candidate,
	pool *candidatePool, groupSize int,
) bool {
	endpoint, resolvable := pool.endpointOf(g, member.RecordRef)
	if !resolvable {
		g.droppedCandidates++
		return false
	}
	at := g.ensureEdge(core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate,
		ends.source, endpoint.target)
	g.addEdgeEvidence(at, ends.originAt)
	g.addEdgeEvidence(at, endpoint.candidateAt)
	g.appendEdgeMatch(at, ends, stage, member, endpoint.candidateAt)
	if size, fits := compactIndex(groupSize); fits && size > g.edges[at].basis.indistinguishable {
		g.edges[at].basis.indistinguishable = size
	}
	return true
}

// candidateEndpointsOf は候補 1 件について、プロセスのノードと根拠のレコードの位置を返す。
// ok が偽になるのは、候補のレコードまたはそのプロセスのノードをグラフから探せないときである。
//
// **プロセスのノードは、観測の層がレコードに残した位置で探す** (recordedProcessAt)。
func (g *Graph) candidateEndpointsOf(candidate matchSide) (int, int, bool) {
	candidateAt, present := g.recordAtLocator(candidate.record.Locator)
	if !present {
		return 0, 0, false
	}
	target, hasProcess := g.recordedProcessAt(candidateAt)
	if !hasProcess {
		return 0, 0, false
	}
	return target, candidateAt, true
}

// emptyStageOutcome は候補が 0 件になった段階の理由を、起点の分類へ写す。
//
// **段階 2 が返しうる core.EmptyReason のうち、この経路で起こるのは
// no_candidate_in_window と no_candidate_matching_conditions である。**
// counterpart_item_absent は原資料について言える事実だが、Squid と markii 形式の組では
// 起こらない (backend/core/matching_stage.go の stageEmptyReason)。
// **この経路で起こらない値が届いた状態は内部の不整合である。** 起点の分類でも内部で
// 処理できなかったこととして数える。
func emptyStageOutcome(reason core.EmptyReason) core.RelationDerivationOutcome {
	switch reason {
	case core.EmptyReasonNoCandidateInWindow:
		return core.RelationDerivationNoCandidateInWindow
	case core.EmptyReasonNoCandidateMatchingConditions:
		return core.RelationDerivationNoCandidateMatchingConditions
	default:
		return core.RelationDerivationNodeUnresolved
	}
}

// stageOf は候補集合から段階の種別で 1 つの段階を返す。
func stageOf(set core.CandidateSet, stageKey core.StageKey) (core.CandidateStage, bool) {
	for _, stage := range set.Stages {
		if stage.StageKey == stageKey {
			return stage, true
		}
	}
	return core.CandidateStage{}, false
}

// destinationNodeAt は起点のレコードが指す接続先のうち、文字列 destination を持つ
// ノードの位置を返す。接続先 IP の文字列では IP アドレスの、ホスト名の文字列ではホスト名の
// ノードを返す。originAt は起点のレコードの位置である。
//
// **鍵は、起点のレコードを置いた端末の範囲で組む。** ループバックとリンクローカルの
// アドレスのノードは端末ごとに別であり、起点の項目 (fields) は利用者が収集元に付けた端末を
// 含まない。
func (g Graph) destinationNodeAt(originAt int, fields []core.RecordField, destination string) (int, bool) {
	var scope core.RecordScope
	if terminal, placed := g.placedTerminals[g.records[originAt].placedTerminalNodeId]; placed {
		scope.Terminal = &terminal
	}
	for _, key := range core.DestinationNodeKeys(fields, scope) {
		value, present := key.LabelValue()
		if !present || value != destination {
			continue
		}
		at, found := g.nodeAt[nodeIdOf(key)]
		if found {
			return at, true
		}
	}
	return 0, false
}

// addEdgeEvidence は同じレコードを 1 本のエッジの根拠に 2 回入れないように足す。
func (g *Graph) addEdgeEvidence(edgeAt, recordAt int) {
	for _, existing := range g.edges[edgeAt].evidence {
		if existing == recordAt {
			return
		}
	}
	g.edges[edgeAt].evidence = append(g.edges[edgeAt].evidence, recordAt)
}

// cloneObservationKind は観測の種別を深く複製する。
func cloneObservationKind(kind core.ObservationKind) core.ObservationKind {
	return core.ObservationKind{
		Raw: cloneRecordFields(kind.Raw), Status: kind.Status, Meaning: kind.Meaning,
	}
}
