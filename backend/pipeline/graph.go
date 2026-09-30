package pipeline

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// nodeIdPrefix と edgeIdPrefix は識別子の種類を文字列で分ける。
const (
	nodeIdPrefix = "n:"
	edgeIdPrefix = "e:"
)

// derivationFileNameFromPath はファイルの path の末尾から表示名を導いたことを表す文字列である。
// 分析者が画面で読む値であるため日本語で書く。
const derivationFileNameFromPath = "file.path の末尾の要素"

// derivationRecordPositionFromLocator は、レコードのノードの表示名を収集元の file 名と
// レコードの位置から導いたことを表す文字列である。位置は、行、ID、byte 位置のうち 1 つである。
const derivationRecordPositionFromLocator = "収集元の file 名とレコードの位置"

// derivationRecordPositionWithEventId は、Windows イベントログのレコードのノードの表示名を、
// Event ID と収集元の file 名とレコードの位置から導いたことを表す文字列である。
const derivationRecordPositionWithEventId = "Event ID と収集元の file 名とレコードの位置"

// derivationRecordNumberWithEventId は、Windows イベントログのレコードのノードの表示名を、
// Event ID と収集元の file 名と EventRecordID から導いたことを表す文字列である。
const derivationRecordNumberWithEventId = "Event ID と収集元の file 名と EventRecordID"

// pathSeparators はファイルの path の区切りである。原資料の文字列を書き換えずに末尾の要素を
// 取るため、Windows と POSIX の両方の区切りを見る。
const pathSeparators = `\/`

// Graph は取り込み結果から組んだ観測層のグラフである。
//
// **原資料のレコードから直接作った関係だけを持つ。** 意味論で結んだ関係は別の層である。
//
// 索引は NewGraph が取り込み結果を 1 回走査して組む。走査の順がノードとエッジの並び順に
// なる。並び順は収集元の取り込みの入力順、同じ収集元の中はレコードの走査順である。
type Graph struct {
	records  []graphRecord
	recordAt map[string]int
	// nodes は観測の層を組み終えた後に書き換えない。選択ごとのグラフは観測の層と同じ配列を
	// 共有する。
	nodes  []graphNode
	nodeAt map[string]int
	// placedTerminals は、レコードを置いた端末の識別子 (graphRecord.placedTerminalNodeId) から
	// その端末の識別鍵を探す表である。**端末のノードがグラフに無い端末も要素にする。**
	// 名前不明の端末に置いたレコードの接続先の鍵を、グラフを組んだときと同じ範囲で組み直す。
	placedTerminals map[string]core.NodeKey
	// adjacency はノードごとの隣接であり、nodes と同じ位置で探す。選択ごとに候補のエッジが
	// 変わるため、選択ごとのグラフが自分の配列を持つ。
	adjacency []nodeAdjacency
	edges     []graphEdge
	// edgeAt は観測の層のエッジを識別子から探す表である。選択ごとのグラフは観測の層と同じ
	// 表を共有し、書き換えない。
	edgeAt map[string]int
	// candidateEdgeAt は候補のエッジを識別子から探す表である。観測の層では nil である。
	candidateEdgeAt map[string]int
	// observedEdgeCount は、選択に依らない観測の層のエッジの数である。edges の先頭から
	// この数までが観測の層であり、残りは関連付けの候補のエッジである。
	observedEdgeCount int
	// originOutcomes は、関係を導く起点になったレコードのノードの識別子から、導いた
	// 結果の分類を探す表である。
	//
	// **起点にならなかったレコードを要素にしない。** 起点はレコードの一部である。空の値を
	// 全件に持たせると、起点でないことと結果が無いことが同じ表現になる。
	originOutcomes map[string]core.RelationDerivationOutcome
	// originsWithoutRecordNode は、レコードのノードを組めなかった起点の件数である。
	originsWithoutRecordNode int
	// logonSessionRejections は、操作のレコードのノードの g.nodes での位置から、Logon ID が
	// 一致しながら関係にしなかったログオンを探す表である。観測の層を組む間だけ書き、選択ごとの
	// グラフは同じ表を共有する。
	logonSessionRejections map[int][]logonSessionRejection
	// attributeAt は、観測の層を組む間だけ、ノードと値から属性の位置を探す表である
	// (addAttribute)。組み終えたグラフでは nil である。
	attributeAt map[attributeValueKey]int
	// unreadableCandidates は関連付けの探す語彙の項目を比べられる形で持たず、候補にできなかった
	// レコードの件数である。
	unreadableCandidates int
	// droppedCandidates は段階 2 に残りながらエッジへ写せなかった候補の件数である。
	droppedCandidates int
	// recordings は収集元ごとの収録範囲である。並びは取り込みの入力順である。
	//
	// **NewGraph の中で採る。** Timeline に ImportResult を引数で渡す形にすると、
	// NewGraph に渡したのと別の取り込み結果を渡せてしまう。
	recordings []sourceRecording
	// terminalNames は、収集の端末のノードの識別子から、その端末が名乗った名前ごとの記録を
	// 探す表である (collectionTerminal.history)。観測の層を組む間だけ書く。
	terminalNames map[string][]TerminalName
	// terminalProfiles は、収集の端末のノードの識別子から、収集の registry が記録した端末の情報を
	// 探す表である (addTerminalProfiles)。観測の層を組む間だけ書く。
	terminalProfiles map[string]TerminalProfile
	// terminalFacts は、レコードを置いた端末のノードの識別子から、その端末のレコードの分類の集計を
	// 探す表である (collectTerminalFact)。観測の層を組む間だけ書く。
	terminalFacts map[string]*terminalFacts
	// additionalEventTimes は、ObservedAt のほかに事象の時刻を持つレコードの g.records での位置から、
	// その時刻を探す表である (RecordSemantics.AdditionalEventTimes)。持たないレコードを要素にしない。
	additionalEventTimes map[int][]additionalEventTime
	// eventTimeFieldNames は、additionalEventTimes に入るレコードの位置から、ObservedAt を記録した
	// event.time の欄の名前を探す表である。
	eventTimeFieldNames map[int]string
	// caseOfSource は収集元の sourceId から、収集元に付けた案件を探す表である。
	// 案件を区別しない取り込みでは要素数 0 である。
	caseOfSource map[string]string
	// sourceInterpretations は収集元の sourceId から、その収集元の時刻の解釈を探す表である
	// (ImportResult.sourceInterpretations)。応答の割当の期間に、期間を読んだずれを与える。
	sourceInterpretations map[string]core.TimestampInterpretation
	// fieldNames は、レコードの欄の語彙の項目と原資料の key の組の集合である。値を読めない
	// 欄も入る。欄の名前がどこにも無いことと、欄はあるが値を読めないことを分ける
	// (CountedFieldObservationOf)。
	fieldNames map[fieldName]struct{}
	// recordHeaderIdNames は、取り込んだ入力形式がレコードの見出しの番号に宣言した欄の名前の
	// 集合である (RecordNumberingNames.RecordHeaderID)。レコードの要約が番号を読む。
	recordHeaderIdNames map[string]struct{}
	// matchSelection は、候補のエッジを作る関連付けが候補を絞るのに用いた条件の選択である。
	//
	// **グラフ 1 つと選択 1 つが対応する。** 関連付けの結果はこの選択で変わるため、別の選択の
	// 答えは別のグラフを組んで返す。
	matchSelection MatchConditionSelection
	// matchStages は、候補のエッジの関連付けが共有する段階である。関連付けは位置でここを指す。
	matchStages []matchStage
	// candidateSetProblem は、候補集合の構築が退けた理由である。candidate_set_failed に
	// 数えた起点があるときだけ値を持つ。最初の 1 件だけを保つ。
	candidateSetProblem error
	// declarationProblem は、収集元の宣言の組から段階の条件を組めなかった理由である。
	//
	// **分類の件数だけでは原因の宣言に到達できない。** グラフの経路は応答を 500 にせず、
	// 候補のエッジが消えるだけであるため、理由を保って呼び出し元が log に載せる。
	declarationProblem error
	// accountNames は、アカウントのノードの g.nodes での位置から、同じアカウントとしてまとめる
	// 鍵か、鍵を持たない理由を探す表である (indexAccountNames)。観測の層を組むときに 1 回だけ
	// 書き、選択ごとのグラフは同じ表を共有する。
	accountNames map[int]accountNameOfNode
}

// graphRecord は根拠のレコード 1 件である。ノードとエッジは本 slice の位置で根拠を指す。
type graphRecord struct {
	locator         core.RecordLocator
	eventTime       *core.Timestamp
	observationKind core.ObservationKind
	inputCapability string
	// eventCategory と eventAction は事象の種別の組である (eventKindOf)。windowsEventKind は、
	// 組を Windows イベントログのプロバイダとイベント ID から作ったかである。
	eventCategory    string
	eventAction      string
	windowsEventKind bool
	instant          time.Time
	hasInstant       bool
	// replacesContent は、レコードが記録したファイルまたはレジストリの値の内容を置き換える
	// 記録かである (SourcePublication.replacesContent)。
	replacesContent bool
	// flowOperation はレコードの操作の分類である (SourcePublication.flowOperation)。宣言が
	// 該当しないレコードでは空の文字列である。
	flowOperation core.FlowOperation
	// destinationPort はレコードが記録した接続先 port の項目である。
	// 欄を持たないレコードでは nil である。
	destinationPort *core.RecordField
	// logonType はレコードが記録したログオンの種別のコードの項目である。
	// 種別を持たないレコードでは nil である。
	logonType *core.RecordField
	// requestUrl はレコードが記録した HTTP の要求先の URL の項目である。
	// 欄を持たないレコードでは nil である。
	requestUrl *core.RecordField
	// httpStatus はレコードが記録した HTTP の状態の項目である。欄を持たないレコードでは nil である。
	httpStatus *core.RecordField
	// proxyStatus は Proxy のログのレコードが記録した要求処理の結果の項目である
	// (SourcePublication.requestStatusOf)。欄を持たないレコードでは nil である。
	proxyStatus *core.RecordField
	// accountNodeId はレコードが記録したアカウントのノードの識別子である。
	// アカウントの識別鍵に入る値を持たないレコードでは空の文字列である。
	accountNodeId string
	// terminalNodeId はレコードを出した端末のノードの識別子である。
	// 端末の識別鍵に入る値を持たないレコードでは空の文字列である。
	//
	// **分析者が与えた端末の割当から導いた端末を入れない。** 割当の鍵は
	// (収集元, 接続元 IP, 端末) で「その IP をその期間どの端末が保持していたか」を表し、
	// 「このレコードを出した端末」と意味が違う。IP からの推論を、推論と示さずに
	// レコードへ帰属させる形になる。
	terminalNodeId string
	// placedTerminalNodeId はレコードを置いた端末のノードの識別子である。端末で絞る条件
	// (RecordFilter.Terminal) だけが比べる。端末に置かないレコードでは空の文字列である。
	//
	// **端末のノードを組む鍵と同じ鍵を使う (core.RecordTerminalNodeKey)。** 端末の一覧に
	// 出る端末はどれも、その端末に置いたレコードへ絞れる。名前不明の端末と、利用者が
	// 収集元に付けた割当の端末も、その収集元のレコードを置いた端末である
	// (sourceTerminals.forRecord)。
	placedTerminalNodeId string
	// recordingTerminalNodeId は、分析者の割当で置き場所を差し替える前の、レコードを記録した
	// 端末のノードの識別子である (sourceTerminals.recordingScope)。ログオンのセッションを
	// 比べる端末である。端末に置かないレコードでは空の文字列である。
	recordingTerminalNodeId string
	// assignableHostname は、分析者の割当が置き場所を差し替え得るレコードのホスト名の比べる値で
	// ある (sourceTerminals.assignableHostname)。差し替えないレコードでは空の文字列である。
	assignableHostname string
	// recordNode はこのレコードのノードの g.nodes での位置である。hasRecordNode が偽の
	// レコードはノードを持たない (addRecordNode)。
	recordNode    int
	hasRecordNode bool
	// processNode は、このレコードが記録したプロセスのノードの g.nodes での位置に 1 を
	// 足した値である。プロセスを記録しないレコードでは 0 である (recordedProcessAt)。
	//
	// **一意な識別子のプロセスも、プロセス番号と区間で識別したプロセスも、観測の層が決める。**
	// 候補の層は、レコードからプロセスのノードを探すためにノードを組み直さない。
	processNode int32
}

// recordedProcessAt は、レコード at が記録したプロセスのノードの位置を返す。
// ok が偽になるのは、プロセスを記録しないレコードである。
func (g Graph) recordedProcessAt(at int) (int, bool) {
	node := g.records[at].processNode
	return int(node) - 1, node > 0
}

// setRecordedProcess は、レコード at が記録したプロセスのノードの位置を残す。先に残した
// 位置を書き換えない。int32 に収まらない位置は残さない。
func (g *Graph) setRecordedProcess(at, node int) {
	if g.records[at].processNode == 0 && node >= 0 && node < math.MaxInt32 {
		g.records[at].processNode = int32(node) + 1
	}
}

// eventKindPair はレコードの事象の分類と動作の組を返す。分類を持たないレコードでは nil を返す。
func (r graphRecord) eventKindPair() *core.EventKindPair {
	return newEventKindPair(r.eventCategory, r.eventAction)
}

func newEventKindPair(category, action string) *core.EventKindPair {
	if category == "" {
		return nil
	}
	return &core.EventKindPair{Category: category, Action: action}
}

// EventKindPairOf はフィールドから事象の分類と動作の組を返す。グラフのレコードに付ける組と
// 同じ規則 (eventKindOf) で決める。分類を持たないレコードでは nil を返す。
func EventKindPairOf(fields []core.RecordField) *core.EventKindPair {
	category, action, _ := eventKindOf(fields)
	return newEventKindPair(category, action)
}

// graphNode はグラフのノード 1 つと、その属性と根拠である。
type graphNode struct {
	id          string
	key         core.NodeKey
	label       core.RawAndNormalized
	observation core.NodeObservation
	// creationRecords は、対象の生成を記録したレコードの g.records での位置である。
	// observation と別の軸である。並びはグラフへ入った順で、同じ位置を 2 回持たない。
	creationRecords []int
	attributes      []graphAttribute
	evidence        []int
}

// nodeAdjacency はノード 1 つから出るエッジと入るエッジの g.edges での位置である。
// 並びはエッジを足した順であり、候補のエッジの位置は観測の層のエッジの位置より後ろに並ぶ。
type nodeAdjacency struct {
	outgoing []int
	incoming []int
}

// graphAttribute はノードの 1 つの意味に観測した値 1 件と、その観測の根拠である。
// 観測の件数は evidence の要素数である。
type graphAttribute struct {
	// field は最初に観測したレコードの項目である。**取り込み結果の項目そのものを指す。**
	// 取り込み結果は組んだ後に書き換えないため、複製しない。
	field *core.RecordField
	// evidence は観測したレコードの g.records での位置である。並びは観測した順であり、
	// 先頭が最初に観測したレコードである。要素数は 1 以上である。
	evidence []int
}

// firstRecord は属性を最初に観測したレコードの g.records での位置を返す。
func (a graphAttribute) firstRecord() int {
	return a.evidence[0]
}

// fieldName は欄 1 つの語彙の項目と原資料の key の組である。
type fieldName struct {
	semantic core.SemanticKey
	name     string
}

// attributeValueKey は同じ値の観測を 1 件にまとめる鍵である。原資料の key の文字列を含めるため、
// 同じ意味を別の key が持つレコードは別の要素になる。
type attributeValueKey struct {
	// node は属性を持つノードの g.nodes での位置である。
	node     int
	semantic core.SemanticKey
	name     string
	value    string
}

// graphEdge はまとめたエッジ 1 本と、その根拠である。
type graphEdge struct {
	id       string
	kind     core.EdgeKind
	state    core.RelationState
	source   int
	target   int
	evidence []int
	// basis は関連付けと割当から作ったエッジの成立の根拠である。どちらも持たないエッジでは
	// nil である。
	//
	// **エッジの大半は観測から直接作ったエッジであり、成立の根拠を持たない。** エッジの
	// 配列は選択ごとのグラフが複製するため、持たない欄を pointer の先へ置く。
	basis *edgeBasis
}

// NewGraph は取り込み結果を 1 回走査して観測層のグラフを組み、関連付けが挙げた候補のエッジを
// 足す。
//
// 公開を止めた収集元のレコードをグラフに入れない。
//
// selection は候補のエッジを作る関連付けが候補を絞るのに用いる条件である。**呼び出し元が
// 必ず与える。** 既定の選択を本関数が埋めると、条件を渡し忘れた経路が通知せずに別の関連付けの
// 答えを返す。
func NewGraph(result ImportResult, selection MatchConditionSelection) Graph {
	return NewObservedGraph(result).WithCandidateEdges(result, selection)
}

// NewObservedGraph は取り込み結果を 1 回走査して、関連付けの条件の選択に依らない観測の層を組む。
// 関連付けの候補のエッジを持たない。
//
// 公開を止めた収集元のレコードをグラフに入れない。
//
// **エッジの slice の容量を長さに揃えて返す。** WithCandidateEdges が複製へエッジを足すとき、
// 観測の層と同じ配列へ書き込まない。
//
// ログオンのセッションが続く最長の時間は DefaultLogonSessionLimit である。
func NewObservedGraph(result ImportResult) Graph {
	return newObservedGraph(result, DefaultLogonSessionLimit)
}

// ObservedGraphWithLogonSessionLimit は、ログオンのセッションが続く最長の時間を limit にして
// 観測の層を組む関数を返す。ほかは NewObservedGraph と同じである。
func ObservedGraphWithLogonSessionLimit(limit time.Duration) func(ImportResult) Graph {
	return func(result ImportResult) Graph { return newObservedGraph(result, limit) }
}

// newObservedGraph は NewObservedGraph の本体である。limit はログオンのセッションが続く最長の時間である。
func newObservedGraph(result ImportResult, limit time.Duration) Graph {
	graph := Graph{
		nodeAt:          make(map[string]int),
		placedTerminals: make(map[string]core.NodeKey),
		edgeAt:          make(map[string]int),
		originOutcomes:  make(map[string]core.RelationDerivationOutcome),
		caseOfSource:    make(map[string]string),
		attributeAt:     make(map[attributeValueKey]int),

		recordHeaderIdNames: make(map[string]struct{}),
	}
	recordCount := 0
	for _, publication := range result.publications {
		if name := publication.parser.RecordNumbering.RecordHeaderID; name != "" {
			graph.recordHeaderIdNames[name] = struct{}{}
		}
		recordCount += len(publication.records)
	}
	// レコードの配列と位置の表を先に確保し、append と map の伸長のたびの複製を避ける。
	graph.records = make([]graphRecord, 0, recordCount)
	graph.recordAt = make(map[string]int, recordCount)
	scopes := result.caseScopes()
	// **案件ごとに絞った割当から収集元の端末を組む。** 期間を読み取った収集元と端末を与える
	// 収集元が別の案件にある割当は、どの案件の割当にも残らない (assignmentsInCase)。
	terminals := make(sourceTerminals)
	for _, scope := range scopes {
		maps.Copy(terminals, sourceTerminalsOf(scope))
	}
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			inputCapability := observeInputCapability(graphFieldsOf(record))
			// 自機の識別子を持たない収集元のレコードは、利用者が与えた端末で範囲が決まる。
			// 足す項目があるレコードだけを複製する。属性は端末の項目を指すため、足さない
			// レコードは取り込み結果の項目を指したまま複製を持たない。
			supplied, recordScope := terminals.forRecord(record)
			if len(supplied) > 0 {
				record.Terminal = append(slices.Clone(record.Terminal), supplied...)
			}
			graph.addRecord(record, recordScope)
			added := &graph.records[len(graph.records)-1]
			added.inputCapability = inputCapability
			added.replacesContent = publication.replacesContent(added.observationKind, graphFieldsOf(record))
			added.flowOperation = publication.flowOperation(added.observationKind)
			added.proxyStatus = publication.requestStatusOf(record)
			if recording, placed := core.RecordTerminalNodeKey(graphFieldsOf(record), terminals.recordingScope(record)); placed {
				added.recordingTerminalNodeId = nodeIdOf(recording)
			}
			added.assignableHostname = terminals.assignableHostname(record)
			graph.collectTerminalFact(publication.parser.FormatKey, record, len(graph.records)-1)
		}
	}
	// 端末の情報は端末のノードを足すため、端末の表示名を付けるより前に組む。
	graph.addTerminalProfiles(result, terminals)
	graph.applySourceTerminalLabels(result, terminals)
	graph.addAssignedAddressEdges(result, terminals)
	graph.attributeAt = nil
	graph.recordings = sourceRecordingsOf(result)
	graph.widenRecordingsByEventTimes()
	for sourceId := range result.identities {
		if caseId := result.caseOfSource(sourceId); caseId != "" {
			graph.caseOfSource[sourceId] = caseId
		}
	}
	graph.sourceInterpretations = result.sourceInterpretations
	for _, scope := range scopes {
		graph.addProcessLineageEdges(scope, terminals)
	}
	graph.addFileContentMatchEdges()
	graph.addArgumentNamedObjectEdges(namedAssignmentsByCase(result, scopes))
	graph.addReverseLookupNameEdges()
	graph.addTerminalOutboundEdges(result, terminals)
	assignedTerminals := make(map[int]struct{})
	for _, scope := range scopes {
		graph.addTerminalSessionEdges(scope, terminals, assignedTerminals)
	}
	for _, scope := range scopes {
		graph.addLogonSessionEdges(scope, terminals)
		graph.addTaskRegistrationRunEdges(scope, terminals)
		graph.addTicketRequestLogonEdges(scope)
		graph.addConnectionLogonEdges(scope)
		graph.addSameConnectionEdges(scope)
		explicitFrom := len(graph.edges)
		graph.addExplicitCredentialLogonEdges(scope)
		sessions := graph.logonSessionPeriodsOf(scope, limit)
		graph.addLogonChainEdges(scope, terminals, sessions, graph.addRequestedSessionEdges(sessions, explicitFrom))
		graph.addAccountIdentityEdges(scope, terminals.sameDomainNames)
	}
	for _, scope := range scopes {
		graph.addClientAddressEdges(scope)
	}
	graph.indexAccountNames(result, terminals.sameDomainNames)
	graph.observedEdgeCount = len(graph.edges)
	graph.indexTerminalFactNodes()
	graph.releaseObservedSpare()
	return graph
}

// WithCandidateEdges は観測の層 g の複製に、selection で関連付けが挙げた候補のエッジを足す。
// **g を書き換えない。** 同じ観測の層から、選択ごとのグラフを同時に組める。まとめる段階は
// 引き継がない。
//
// **関連付けの候補のエッジは、既にあるノードどうしを結ぶだけである** (addCandidateEdge)。
// 複製のノードの並びは g と同じであり、全選択のグラフで 1 つのまとめる段階を共有できる。
//
// selection は候補のエッジを作る関連付けが候補を絞るのに用いる条件である。**呼び出し元が
// 必ず与える。** 既定の選択を本関数が埋めると、条件を渡し忘れた経路が通知せずに別の答えを返す。
//
// 候補のエッジは、観測の層のエッジの後ろに並ぶ。案件ごとに遠隔のセッションのエッジと交互に
// 足していた NewGraph の変更前の並びとは異なる。
func (g Graph) WithCandidateEdges(result ImportResult, selection MatchConditionSelection) Graph {
	graph := g.withoutCandidateEdges(selection)
	for _, scope := range result.caseScopes() {
		graph.addCandidateEdges(scope)
	}
	graph.releaseCandidateSpare()
	return graph
}

// withoutCandidateEdges は観測の層 g の複製を、selection の候補のエッジを足す前の状態で返す。
// **g を書き換えない。**
func (g Graph) withoutCandidateEdges(selection MatchConditionSelection) Graph {
	graph := g
	// g が候補のエッジを持っていても、観測の層まで切り詰めてから足す。ノードの隣接の並びは
	// エッジを足した順であり、候補のエッジの番号は末尾に並ぶ。ノードは候補のエッジを足しても
	// 変わらないため、g と同じ配列を共有する。
	graph.adjacency = make([]nodeAdjacency, len(g.adjacency))
	for index, adjacency := range g.adjacency {
		graph.adjacency[index] = nodeAdjacency{
			outgoing: observedEdgeIndexes(adjacency.outgoing, g.observedEdgeCount),
			incoming: observedEdgeIndexes(adjacency.incoming, g.observedEdgeCount),
		}
	}
	graph.edges = g.edges[:g.observedEdgeCount:g.observedEdgeCount]
	graph.candidateEdgeAt = make(map[string]int)
	graph.originOutcomes = make(map[string]core.RelationDerivationOutcome)
	graph.originsWithoutRecordNode = 0
	graph.unreadableCandidates = 0
	graph.droppedCandidates = 0
	graph.candidateSetProblem = nil
	graph.declarationProblem = nil
	graph.matchSelection = selection
	graph.matchStages = nil
	return graph
}

// observedEdgeIndexes は、エッジの番号の並びから観測の層の番号だけを残す。**容量を長さに揃える。**
// 返した slice へ足すと新しい配列を取るため、元の並びを書き換えない。
func observedEdgeIndexes(edges []int, observedEdgeCount int) []int {
	cut := len(edges)
	for cut > 0 && edges[cut-1] >= observedEdgeCount {
		cut--
	}
	return edges[:cut:cut]
}

// addRecord は 1 レコードのノードと、そのレコードが指すノードと作る関係をグラフへ足す。
//
// **対象を 1 つも指さないレコードもグラフへ入る。** 読めた欄はレコードのノードの属性に
// なるため、記録した対象の有無が索引に載るかを決めない。
func (g *Graph) addRecord(record RecordEntry, scope core.RecordScope) {
	fields := graphFieldsOf(record)
	// refs は fields と同じ並びで、取り込み結果の項目そのものを指す。属性は項目を複製せずに指す。
	refs := graphFieldRefsOf(record)
	recordGraph := core.NewRecordGraphInScope(fields, scope)
	at := g.addGraphRecord(record, fields, refs, scope)
	if g.fieldNames == nil {
		g.fieldNames = make(map[fieldName]struct{})
	}
	for _, field := range fields {
		g.fieldNames[fieldName{semantic: field.Semantic, name: field.Name}] = struct{}{}
	}
	nodeAt := make(map[string]int, len(recordGraph.Nodes)+len(recordGraph.ReferencedNodes))
	creationRecord := recordsProcessStart(record)
	for _, key := range recordGraph.Nodes {
		index := g.ensureNode(key, core.NodeObservationObserved)
		g.nodes[index].evidence = append(g.nodes[index].evidence, at)
		nodeAt[g.nodes[index].id] = index
		if label, found := nodeLabelOf(fields, key); found {
			g.nodes[index].applyLabel(label)
		}
		// **生成を記録したレコードが記録したプロセスにだけ根拠を足す。** 同じレコードが
		// 参照した親のプロセスには付かない。親の生成のレコードは別に要る。
		if creationRecord && key.Kind == core.NodeKindProcess {
			g.nodes[index].addCreationRecord(at)
		}
		// 記録した対象の中のプロセスは、レコードが記録したプロセスである。親と注入先は
		// 参照の側 (ReferencedNodes) に入る。
		if key.Kind == core.NodeKindProcess {
			g.setRecordedProcess(at, index)
		}
	}
	for _, referenced := range recordGraph.ReferencedNodes {
		index := g.ensureNode(referenced.Key, core.NodeObservationReferenced)
		g.nodes[index].evidence = append(g.nodes[index].evidence, at)
		nodeAt[g.nodes[index].id] = index
		if label, found := labelOfSemantic(fields, referenced.LabelSemantic); found {
			g.nodes[index].applyLabel(label)
		}
	}
	if record.Semantics != nil && record.Semantics.AccountCreation {
		for _, naming := range recordGraph.Namings {
			if naming.Kind != core.EdgeKindRecordTargetAccount {
				continue
			}
			if index, present := nodeAt[nodeIdOf(naming.Target)]; present {
				g.nodes[index].addCreationRecord(at)
			}
		}
	}
	g.addAttributes(fields, refs, recordGraph.Nodes, nodeAt, at)
	for _, link := range recordGraph.Links {
		// **両端がレコードが記録したノードにあるときだけ関係を足す。** 位置の表に無い鍵を
		// map の既定値 0 で探すと、グラフの先頭のノードへ通知せずに繋がる。
		// core.NewRecordGraph は関係の両端を必ず Nodes または ReferencedNodes に入れるため、
		// この分岐は到達しない。
		source, hasSource := nodeAt[nodeIdOf(link.Source)]
		target, hasTarget := nodeAt[nodeIdOf(link.Target)]
		if !hasSource || !hasTarget {
			continue
		}
		index := g.ensureEdge(link.Kind, core.RelationStateObserved, source, target)
		// SID の鍵と名前の鍵が 1 つのアカウントへ寄ったレコードは、同じエッジへ 2 本の関係を
		// 持つ。根拠は 1 回だけ足す。根拠はレコードの順に足すため、重なるのは末尾だけである。
		if evidence := g.edges[index].evidence; len(evidence) == 0 || evidence[len(evidence)-1] != at {
			g.edges[index].evidence = append(evidence, at)
		}
	}
	if creationRecord {
		g.addExecutableEdge(fields, scope, at)
	}
	// **レコードのノードと対象を指す関係は、そのレコードが作った対象どうしの関係の後に
	// 足す。** ノードとエッジの並び順は走査の順であり、上限の小さいページの先頭を
	// レコードが占めると、分析者が最初に見るものが調査の対象から外れる。
	if recordNode, built := g.addRecordNode(record, refs, at); built {
		g.records[at].recordNode, g.records[at].hasRecordNode = recordNode, true
		g.addNamingEdges(recordNode, recordGraph, nodeAt, at)
	}
}

// addExecutableEdge は、起動のレコードが記録した実行ファイルのノードから、起動した
// プロセスへの関係を足す。実行ファイルのノードは参照だけのノードである。起動のレコードは
// ファイルそのものを記録していない。
func (g *Graph) addExecutableEdge(fields []core.RecordField, scope core.RecordScope, at int) {
	link, found := core.ExecutableLink(fields, scope)
	if !found {
		return
	}
	process, found := g.nodeAt[nodeIdOf(link.Target)]
	if !found {
		return
	}
	file := g.ensureNode(link.Source, core.NodeObservationReferenced)
	g.nodes[file].evidence = append(g.nodes[file].evidence, at)
	if label, found := labelOfSemantic(fields, core.SemanticKeyProcessBinaryPath); found {
		g.nodes[file].applyLabel(label)
	}
	edge := g.ensureEdge(link.Kind, core.RelationStateObserved, file, process)
	g.edges[edge].evidence = append(g.edges[edge].evidence, at)
}

// addRecordNode はレコードのノードを 1 つ足し、読めた欄をすべてその属性にして位置を返す。
//
// **語彙の項目の有無も、役割も、対象の種別も見ない。** レコードのノードは、そのレコードが
// 持つ欄そのものの置き場である。対象のノードへ帰属先が決まるかは addAttributes が別に
// 判定する。
//
// ok が偽になるのは、レコードの位置から識別鍵を組めないときである
// (core.NewRecordNodeKey)。その入力ではレコードのノードを作らず、レコードは根拠としてだけ
// グラフに残る。
//
// 既知の制限: 読めたレコードを 1 件ずつノードにし、レコードのノードの件数に上限を置かない,
// 返す量は要求のノードの数の上限 (nodeLimit) が抑えるため、レコードの件数は応答の大きさを
// 直に決めない。グラフを組む process の RSS はレコードの件数に比例し、repo の fixture の
// 大きさでは測れない,
// 画面の既定の要求の応答が 2,000,000 byte または 500 ms を超えたとき、またはグラフを組む
// process の最大 RSS が 8 GB を超えたときに見直す
func (g *Graph) addRecordNode(
	record RecordEntry, refs []*core.RecordField, at int,
) (int, bool) {
	key, buildable := core.NewRecordNodeKey(record.Locator)
	if !buildable {
		return 0, false
	}
	index := g.ensureNode(key, core.NodeObservationObserved)
	g.nodes[index].evidence = append(g.nodes[index].evidence, at)
	eventID, recordNumber := "", ""
	if g.records[at].windowsEventKind {
		eventID = g.records[at].eventAction
		for _, field := range refs {
			if field.Semantic == core.SemanticKeyWindowsEventRecordId && field.Text != nil && field.Text.RawText != nil {
				recordNumber = *field.Text.RawText
				break
			}
		}
	}
	if label, derived := recordLabelOf(record.Locator, eventID, recordNumber); derived {
		g.nodes[index].applyLabel(label)
	}
	g.nodes[index].attributes = slices.Grow(g.nodes[index].attributes, len(refs))
	for _, field := range refs {
		g.addAttribute(index, field, at)
	}
	return index, true
}

// recordLabelOf はレコードのノードの表示名を、収集元の file 名と、位置の名前と値から導く。
//
// 表示名は「<file> 行 <N>」「<file> ID <N>」「<file> 位置 <byte 位置>」のいずれかである。
// eventID が空でない Windows イベントログのレコードは、Event ID を先頭に置く。EventRecordID の
// 原文字列である recordNumber が空でなければ「<Event ID> <file> EventRecordID <N>」にする。
// 導出の結果は valueState の derived と derivation が持つ。
//
// ok が偽になるのは、file 名か位置の値が無いときと、導いた文字列を値として組めないときである。
func recordLabelOf(locator core.RecordLocator, eventID, recordNumber string) (core.RawAndNormalized, bool) {
	position, present := recordPositionText(locator)
	if !present || locator.SourceFileName == "" {
		return core.RawAndNormalized{}, false
	}
	derivation := derivationRecordPositionFromLocator
	// Windows イベントログのレコードは Event ID を先頭に置く。一覧と経路の表で、同じ収集元の
	// レコードを事象の種別で読み分けられる。
	if eventID != "" {
		derivation = derivationRecordPositionWithEventId
		if recordNumber != "" {
			position, derivation = "EventRecordID "+recordNumber, derivationRecordNumberWithEventId
		}
	}
	text := locator.SourceFileName + " " + position
	if eventID != "" {
		text = eventID + " " + text
	}
	value, err := core.NewDerivedValue(text, derivation)
	if err != nil {
		return core.RawAndNormalized{}, false
	}
	return value, true
}

// recordPositionText はレコードの位置を「<名前> <値>」で返す。通番で指すレコードは ID、
// 行番号を持つレコードは行、それ以外は byte 位置の始まりを使う。名前は画面のレコード位置の
// 名前と同じである。present が偽になるのは、どの値も無いときである。
func recordPositionText(locator core.RecordLocator) (string, bool) {
	switch {
	case locator.PositionKind == core.PositionKindSequenceNumber && locator.SequenceNumber != nil:
		return "ID " + strconv.FormatInt(*locator.SequenceNumber, 10), true
	case locator.LineNumber != nil:
		return "行 " + strconv.FormatInt(*locator.LineNumber, 10), true
	case locator.ByteOffset != nil:
		return "位置 " + strconv.FormatInt(*locator.ByteOffset, 10), true
	}
	return "", false
}

// addNamingEdges は、レコードのノードから、そのレコードが記録した対象のノードへ関係を足す。
//
// 対象そのものを記録した関係と、属性として参照しただけの関係を同じ種別で結ぶ
// (core.EdgeKindRecordNamesObject)。レコードが種別を付けて記録した対象 (役割を付けて
// 記録したアカウント) は、その種別だけで結ぶ。
func (g *Graph) addNamingEdges(
	recordNode int, recordGraph core.RecordGraph, nodeAt map[string]int, at int,
) {
	named := make([]core.NodeKey, 0, len(recordGraph.Nodes)+len(recordGraph.ReferencedNodes))
	named = append(named, recordGraph.Nodes...)
	for _, referenced := range recordGraph.ReferencedNodes {
		named = append(named, referenced.Key)
	}
	for _, key := range named {
		if !recordGraph.NamesWithKind(key) {
			g.addNamingEdge(core.EdgeKindRecordNamesObject, recordNode, key, nodeAt, at)
		}
	}
	for _, naming := range recordGraph.Namings {
		g.addNamingEdge(naming.Kind, recordNode, naming.Target, nodeAt, at)
	}
}

// addNamingEdge は、レコードのノードから key のノードへ kind の関係を 1 本足す。
func (g *Graph) addNamingEdge(
	kind core.EdgeKind, recordNode int, key core.NodeKey, nodeAt map[string]int, at int,
) {
	index, present := nodeAt[nodeIdOf(key)]
	if !present {
		return
	}
	edge := g.ensureEdge(kind, core.RelationStateObserved, recordNode, index)
	g.addEdgeEvidence(edge, at)
}

// addGraphRecord は根拠のレコードを 1 件足し、その位置を返す。
//
// **位置と時刻と観測の種別と接続先 port とログオンの種別の項目を、取り込み結果と共有する。** 取り込み結果は
// 組んだ後に書き換えず、応答へ出すときに複製する (evidenceItems、timelineEntry)。
// refs は fields と同じ並びで取り込み結果の項目を指す (graphFieldRefsOf)。
func (g *Graph) addGraphRecord(
	record RecordEntry, fields []core.RecordField, refs []*core.RecordField,
	scope core.RecordScope,
) int {
	entry := graphRecord{locator: record.Locator, eventTime: record.ObservedAt}
	if record.Semantics != nil {
		entry.observationKind = record.Semantics.ObservationKind
	}
	entry.eventCategory, entry.eventAction, entry.windowsEventKind = eventKindOf(fields)
	// 接続先 port の欄を 2 つ以上持つレコードでは先頭だけを保つ。1 レコードが記録する通信は
	// 1 つであり、2 つ目の欄は 1 つの語彙の項目が同じレコードで 2 回出た状態である。
	// 根拠の区分が原資料の欄の並び順で決まる形にするため、先頭を採る。
	entry.destinationPort = firstRefWithSemantic(refs, core.SemanticKeyConnectionDestinationPort)
	// ログオンの種別も同じ理由で先頭を採る。1 レコードが記録するログオンは 1 つである。
	entry.logonType = firstRefWithSemantic(refs, core.SemanticKeyEventLogonType)
	// 要求先の URL も同じ理由で先頭を採る。1 レコードが記録する要求は 1 つである。
	entry.requestUrl = firstRefWithSemantic(refs, core.SemanticKeyHttpRequestUrl)
	entry.httpStatus = firstRefWithSemantic(refs, core.SemanticKeyHttpStatusCode)
	if account, named := core.AccountNodeKey(fields, scope); named {
		entry.accountNodeId = nodeIdOf(account)
	}
	entry.terminalNodeId = recordedTerminalNodeId(fields, scope)
	if terminal, placed := core.RecordTerminalNodeKey(fields, scope); placed {
		entry.placedTerminalNodeId = nodeIdOf(terminal)
		if _, known := g.placedTerminals[entry.placedTerminalNodeId]; !known {
			g.placedTerminals[entry.placedTerminalNodeId] = terminal
		}
	}
	if record.ObservedAt != nil {
		entry.instant, entry.hasInstant = record.ObservedAt.Instant()
	}
	g.records = append(g.records, entry)
	at := len(g.records) - 1
	g.addAdditionalEventTimes(record, at)
	// 同じ位置を 2 件のレコードが名乗る取り込み結果は公開されない
	// (withhold.go の checkIdentifierCollision)。先に入れた要素を保つ。
	key := recordKeyOf(entry.locator)
	if _, taken := g.recordAt[key]; !taken {
		g.recordAt[key] = at
	}
	return at
}

// firstRefWithSemantic は、語彙の項目を持つ最初の項目を指して返す。持つ項目が無いときは
// nil を返す。
func firstRefWithSemantic(refs []*core.RecordField, semantic core.SemanticKey) *core.RecordField {
	if at := slices.IndexFunc(refs, func(field *core.RecordField) bool {
		return field.Semantic == semantic
	}); at >= 0 {
		return refs[at]
	}
	return nil
}

// recordedTerminalNodeId は、レコードが自ら名乗った端末のノードの識別子を返す。
// 名乗る値を持たないレコードでは空の文字列を返す。
//
// **分析者が画面から与えた端末の項目を読まない。** 割当は分析者の推論であり、レコードが
// 名乗った端末と意味が違う。時系列の行に推論を混ぜると、分析者はどの行が原資料の値で、
// どの行が自分の入力から導かれたかを読み分けられない。**取り込みの起動で指定した端末は
// 読む。** 利用者が収集元を記録した端末として起動の前に与えた値である。
//
// **ホスト名を名乗ったレコードは、収集元の中のそのホスト名の端末を名乗る。** 端末の外部
// 識別子を持たない形式でも、ホスト名ごとに分けた名前不明の端末は、レコード自身の欄から
// 決まる (sourceTerminals.forRecord)。**収集の directory から取り込んだレコードは、収集の端末を
// 名乗る。**
func recordedTerminalNodeId(fields []core.RecordField, scope core.RecordScope) string {
	for _, field := range fields {
		if field.Semantic != core.SemanticKeyTerminalId || field.Text == nil {
			continue
		}
		if field.Name == analystTerminalIdFieldName {
			continue
		}
		terminalId, readable := field.Text.ComparableValue()
		if !readable {
			continue
		}
		if terminal, named := core.TerminalNodeKey(terminalId); named {
			return nodeIdOf(terminal)
		}
	}
	if scope.Terminal == nil {
		return ""
	}
	// 収集の端末は、収集の registry が名前を記録した 1 台の端末であり、収集の file のレコードは
	// ホスト名を持たなくてもその端末が記録したレコードである。
	if scope.Terminal.Form == core.NodeKeyFormRecordingSourceHostname ||
		scope.Terminal.Form == core.NodeKeyFormCollection {
		return nodeIdOf(*scope.Terminal)
	}
	return ""
}

// recordKeyOf は根拠のレコードを収集元と位置で探す鍵を返す。
func recordKeyOf(locator core.RecordLocator) string {
	return locator.SourceId + "\x00" + locatorPositionKey(locator)
}

// recordAtLocator はレコードの位置から根拠の位置を返す。
// ok が偽になるのは、そのレコードがグラフの根拠に入っていないときである。
func (g Graph) recordAtLocator(locator core.RecordLocator) (int, bool) {
	at, found := g.recordAt[recordKeyOf(locator)]
	return at, found
}

// recordsProcessStart は、レコードがプロセスの起動を記録したかを返す。
// 意味付けに至らなかったレコードは偽である。
func recordsProcessStart(record RecordEntry) bool {
	return record.Semantics != nil && record.Semantics.ProcessStart
}

// creationRecordOf はノードの種別と、生成を記録した根拠のレコードの件数から、生成の
// レコードの有無を返す。
func creationRecordOf(kind core.NodeKind, creationRecordCount int) core.NodeCreationRecord {
	if !kind.CarriesCreationRecord() {
		return core.NodeCreationRecordItemAbsent
	}
	if creationRecordCount > 0 {
		return core.NodeCreationRecordPresent
	}
	return core.NodeCreationRecordAbsent
}

// graphFieldsOf は 1 レコードの意味付けの結果と端末の項目を 1 つの集合にする。
//
// 端末の項目のうち、意味付けの結果に同じ語彙の項目が無いものを足す。端末の持つ IP は
// 端末の項目だけが 1 つのアドレスごとに分けて持つ (ParsedRecord.Terminal を組む adapter)。
func graphFieldsOf(record RecordEntry) []core.RecordField {
	var fields []core.RecordField
	if record.Semantics != nil {
		fields = record.Semantics.Fields
	}
	// **足す項目が無いレコードは取り込み結果の slice を共有する。** 呼び出し側は項目を
	// 書き換えない。容量を長さに揃え、append が取り込み結果の配列へ書き込まない。
	merged := slices.Clip(fields)
	for _, field := range record.Terminal {
		if mergesTerminalField(fields, field) {
			merged = append(merged, field)
		}
	}
	return merged
}

// graphFieldRefsOf は graphFieldsOf と同じ項目を同じ並びで、取り込み結果の項目そのものを
// 指す pointer で返す。
func graphFieldRefsOf(record RecordEntry) []*core.RecordField {
	var fields []core.RecordField
	if record.Semantics != nil {
		fields = record.Semantics.Fields
	}
	refs := make([]*core.RecordField, 0, len(fields)+len(record.Terminal))
	for index := range fields {
		refs = append(refs, &fields[index])
	}
	for index := range record.Terminal {
		if mergesTerminalField(fields, record.Terminal[index]) {
			refs = append(refs, &record.Terminal[index])
		}
	}
	return refs
}

// mergesTerminalField は、端末の項目を意味付けの結果へ足すかを返す。意味付けの結果に同じ
// 語彙の項目が無い端末の項目を足す。
func mergesTerminalField(fields []core.RecordField, terminal core.RecordField) bool {
	return terminal.Semantic != "" && len(fieldsWithSemantic(fields, terminal.Semantic)) == 0
}

// ensureNode は識別鍵に対応するノードを返す。まだ無ければ足す。
//
// **対象そのものを記録したレコードが 1 件でもあるノードは observed になる。** 参照だけの
// ノードとして先に足した後で、そのプロセスの起動のレコードを走査したときも同じである。
// 逆に、observed のノードを後から referenced へ変えない。
func (g *Graph) ensureNode(key core.NodeKey, observation core.NodeObservation) int {
	id := nodeIdOf(key)
	if at, found := g.nodeAt[id]; found {
		if observation == core.NodeObservationObserved {
			g.nodes[at].observation = observation
		}
		return at
	}
	g.nodes = append(g.nodes, graphNode{
		id: id, key: key, label: core.NewAbsentItemValue(), observation: observation,
	})
	g.adjacency = append(g.adjacency, nodeAdjacency{})
	g.nodeAt[id] = len(g.nodes) - 1
	return len(g.nodes) - 1
}

// ensureEdge は種別と両端に対応するエッジを返す。まだ無ければ足す。
//
// **関係の状態を識別子の材料に入れない。** 同じ 2 つのノードを結ぶ観測の関係と候補の関係は
// 種別が異なるため、種別と両端で 1 本に定まる。
func (g *Graph) ensureEdge(
	kind core.EdgeKind, state core.RelationState, source, target int,
) int {
	id := edgeIdOf(kind, g.nodes[source].id, g.nodes[target].id)
	if at, found := g.edgeIndexOf(id); found {
		return at
	}
	g.edges = append(g.edges, graphEdge{
		id: id, kind: kind, state: state, source: source, target: target,
	})
	at := len(g.edges) - 1
	// 選択ごとのグラフは観測の層と共有する表を書き換えず、候補のエッジの表へ足す。
	if g.candidateEdgeAt != nil {
		g.candidateEdgeAt[id] = at
	} else {
		g.edgeAt[id] = at
	}
	g.adjacency[source].outgoing = append(g.adjacency[source].outgoing, at)
	g.adjacency[target].incoming = append(g.adjacency[target].incoming, at)
	return at
}

// edgeIndexOf は識別子で指したエッジの g.edges での位置を返す。
// ok が偽になるのは、その識別子のエッジがグラフに無いときである。
func (g Graph) edgeIndexOf(id string) (int, bool) {
	if at, found := g.candidateEdgeAt[id]; found {
		return at, true
	}
	at, found := g.edgeAt[id]
	return at, found
}

// applyLabel は最初に観測した表示名を保つ。
//
// **後から観測した別の表示名で上書きしない。** 相違は同じ意味の属性として残り、
// 観測した値の個数を NodeAttribute の valueCount が持つ。
//
// **導いた表示名は、同じ文字列を原資料に書いた表示名で置き換える。** 表示の文字列は変わらない。
// 置き換えないと、ある収集元が別の形で書いた文字列から導いた注記が、同じ文字列をそのまま書いた
// 収集元のレコードにも付いて見える。
func (n *graphNode) applyLabel(label core.RawAndNormalized) {
	switch n.label.ValueState {
	case core.ValueStateItemAbsent:
		n.label = label
	case core.ValueStateDerived:
		derived, derivedOk := n.label.NormalizedValue()
		raw, rawOk := label.RawTextValue()
		if label.ValueState == core.ValueStatePresent && derivedOk && rawOk && raw == derived {
			n.label = label
		}
	}
}

// addCreationRecord は、対象の生成を記録したレコードの位置を根拠へ足す。
// 同じレコードが 2 度記録した位置を重ねて持たない。
func (n *graphNode) addCreationRecord(at int) {
	if slices.Contains(n.creationRecords, at) {
		return
	}
	n.creationRecords = append(n.creationRecords, at)
}

// nodeIdOf は識別鍵からノードの識別子を作る。
//
// **識別子を導く材料は識別鍵の組だけである。** ノードの種別と、識別鍵の形と、鍵の値を
// 長さ前置でつないだ文字列のハッシュであり、収集元の識別子 (sourceId) も解析実行への参照
// (analysisRunRef) も材料に入らない。
//
// **同じ識別鍵を持つノードは、取り込み直しても解析実行が変わっても同じ識別子になる。**
// 再取り込みは新しい sourceId を発行するが、識別子は変わらない。分析者が後の段階で
// ノードへ付ける注釈が、取り込みのたびに宙に浮くことを避けるためである。関係の注釈・
// 否認・候補追加は、原資料や自動導出とは別に記録する。
func nodeIdOf(key core.NodeKey) string {
	return nodeIdPrefix + string(key.Kind) + ":" + identityDigest(key.DigestParts())
}

// edgeIdOf は種別と両端の識別子からエッジの識別子を作る。
//
// 材料はノードの識別子であるため、エッジの識別子も取り込みと解析実行をまたいで安定する。
func edgeIdOf(kind core.EdgeKind, sourceNodeId, targetNodeId string) string {
	return edgeIdPrefix + string(kind) + ":" +
		identityDigest([]string{sourceNodeId, targetNodeId})
}

// nodeLabelOf は 1 レコードの項目から 1 つのノードの表示名を導く。
//
// **表示名になる項目を語彙の役割で探す。** 役割が label の項目をレコードが持つとき、その
// 項目の値が表示名になる。
// **役割が label の項目を語彙が持たない対象では、識別鍵の最後の値が表示名になる。**
// 対象は ip と domain と account であり、語彙の表の表示名の欄と一致する。アカウントを
// セキュリティ識別子で識別したレコードは鍵にログイン名を持たないため、表示名も
// セキュリティ識別子になる。
//
// ok が偽になるのは、表示名になる項目をレコードが持たず、識別鍵からも導けないときである。
func nodeLabelOf(fields []core.RecordField, key core.NodeKey) (core.RawAndNormalized, bool) {
	semantic, hasLabel := core.LabelSemanticOf(key.Kind)
	if !hasLabel {
		value, present := key.LabelValue()
		if !present {
			return core.RawAndNormalized{}, false
		}
		return identityFieldValue(fields, value)
	}
	if key.Kind == core.NodeKindFile && isAmbiguousFileRecord(fields) {
		return fileNameFromKeyValue(fields, key)
	}
	if label, found := labelOfSemantic(fields, semantic); found {
		return label, true
	}
	if key.Kind == core.NodeKindFile {
		return fileNameFromKeyValue(fields, key)
	}
	return core.RawAndNormalized{}, false
}

// recordedFilePath は、ファイルのノードの鍵の path を記録した項目の、比べる値を返す。鍵の path は
// 大文字と小文字をそろえた値であり (core.FilePathKeyValue)、表示には記録した文字列を使う。
// 見つからないときは鍵の path を返す。
func recordedFilePath(fields []core.RecordField, keyed string) string {
	for _, field := range fields {
		if field.Semantic.Object() != core.SemanticObjectFile ||
			field.Semantic.Role() != core.SemanticRoleIdentity || field.Text == nil {
			continue
		}
		if value, readable := field.Text.ComparableValue(); readable && core.FilePathKeyValue(value) == keyed {
			return value
		}
	}
	return keyed
}

// isAmbiguousFileRecord は 1 レコードが元と先の file を指す形であるかを返す。
func isAmbiguousFileRecord(fields []core.RecordField) bool {
	return hasSemantic(fields, core.SemanticKeyFilePath) && hasSemantic(fields, core.SemanticKeyFileDestinationPath)
}

// labelOfSemantic は語彙の項目が記録した値を表示名として返す。
// ok が偽になるのは、その語彙の項目を持つ要素が無いときと、値を読めないときである。
func labelOfSemantic(
	fields []core.RecordField, semantic core.SemanticKey,
) (core.RawAndNormalized, bool) {
	for _, field := range fields {
		if field.Semantic != semantic || field.Text == nil {
			continue
		}
		if _, readable := field.Text.ComparableValue(); readable {
			return cloneRawAndNormalized(*field.Text), true
		}
	}
	return core.RawAndNormalized{}, false
}

// identityFieldValue は識別鍵の値を持つ項目の値を返す。
//
// IP アドレスとホスト名のノードは識別鍵の値が表示名である。原資料の文字列が識別鍵の値そのものである
// 項目は、そのまま持つ。
//
// **原資料の文字列が識別鍵の値と異なる項目は、識別鍵の値を導いた値として持つ。** その項目は長い文字列
// から値を切り出した結果であり (要求先の URI から取った host、並べて書いたアドレスの 1 つ)、
// 原資料の文字列をそのまま持つと、表示名が URI やアドレスの並び全体になる。導き方は項目のものを使う。
//
// **原資料の文字列が識別鍵の値そのものである項目を先に探す。** 同じレコードが両方の項目を持つとき、
// 項目の並びで表示名が導いた値に替わらないようにする。
func identityFieldValue(fields []core.RecordField, value string) (core.RawAndNormalized, bool) {
	var first *core.RawAndNormalized
	firstIsCut := false
	for _, field := range fields {
		if field.Text == nil {
			continue
		}
		observed, readable := field.Text.ComparableValue()
		if !readable || observed != value {
			continue
		}
		raw, hasRaw := field.Text.RawTextValue()
		if hasRaw && raw == value {
			return cloneRawAndNormalized(*field.Text), true
		}
		if first == nil {
			first = field.Text
			firstIsCut = hasRaw && field.Text.Derivation != nil
		}
	}
	if first == nil {
		return core.RawAndNormalized{}, false
	}
	if !firstIsCut {
		return cloneRawAndNormalized(*first), true
	}
	derived, err := core.NewDerivedValue(value, *first.Derivation)
	if err != nil {
		return cloneRawAndNormalized(*first), true
	}
	return derived, true
}

// fileNameFromKeyValue は識別鍵が持つファイルの path の末尾の要素を表示名として導く。
// 末尾の要素は、レコードがその path を記録した文字列から取る (recordedFilePath)。
//
// 導出の結果は valueState の derived と derivation が持つ。
func fileNameFromKeyValue(fields []core.RecordField, key core.NodeKey) (core.RawAndNormalized, bool) {
	keyed, present := key.LabelValue()
	if !present {
		return core.RawAndNormalized{}, false
	}
	path := recordedFilePath(fields, keyed)
	tail := path
	if at := strings.LastIndexAny(path, pathSeparators); at >= 0 {
		tail = path[at+1:]
	}
	if tail == "" {
		return core.RawAndNormalized{}, false
	}
	value, err := core.NewDerivedValue(tail, derivationFileNameFromPath)
	if err != nil {
		return core.RawAndNormalized{}, false
	}
	return value, true
}

// addAttributes は 1 レコードの項目を、その意味が対象とするノードの属性へ足す。
//
// **参照だけで記録したノードに属性を足さない。** 呼び出し元が渡す keys は、レコードが対象
// そのものとして記録したノードだけである (core.RecordGraph.Nodes)。
//
// 既知の制限: 1 レコードが同じ種別のノードを 2 つ以上指すとき、copy レコードの file 属性は
// hash だけを元・先の両ノードへ足し、それ以外を除く, file_copy のレコードが持つ
// file.size_bytes は元・先のどちらを指すかを形式が定めない,
// サイズ・時刻などの file 属性を元・先のどちらへ帰属できる根拠を入力形式ごとに定めたときに見直す
func (g *Graph) addAttributes(
	fields []core.RecordField, refs []*core.RecordField, keys []core.NodeKey,
	nodeAt map[string]int, at int,
) {
	single := singleNodeByKind(keys, nodeAt)
	fileNodes := nodesByKind(keys, nodeAt, core.NodeKindFile)
	destinationPathPresent := len(fieldsWithSemantic(fields,
		core.SemanticKeyFileDestinationPath)) > 0
	for _, field := range refs {
		kind, isNode := core.NodeKindOf(field.Semantic.Object())
		if !isNode || field.Semantic.Role() == core.SemanticRoleIdentity {
			continue
		}
		if kind == core.NodeKindFile && destinationPathPresent && len(fileNodes) > 1 {
			if !isFileContentAttribute(field.Semantic) {
				continue
			}
			for _, index := range fileNodes {
				g.addAttribute(index, field, at)
			}
			continue
		}
		index, unique := single[kind]
		if unique {
			g.addAttribute(index, field, at)
		}
	}
}

// isFileContentAttribute はコピー元とコピー先の両方を説明するハッシュ項目である。
func isFileContentAttribute(semantic core.SemanticKey) bool {
	switch semantic {
	case core.SemanticKeyFileMd5, core.SemanticKeyFileSha1, core.SemanticKeyFileSha256:
		return true
	default:
		return false
	}
}

// nodesByKind はレコードが記録した種別のノード位置を返す。
func nodesByKind(keys []core.NodeKey, nodeAt map[string]int, kind core.NodeKind) []int {
	indices := make([]int, 0, len(keys))
	for _, key := range keys {
		if key.Kind != kind {
			continue
		}
		if index, present := nodeAt[nodeIdOf(key)]; present {
			indices = append(indices, index)
		}
	}
	return indices
}

// singleNodeByKind は、レコードがちょうど 1 つだけ記録した種別のノードの位置を返す。
//
// **位置の表にある鍵だけを数える。** 位置の表に無い鍵を map の既定値 0 で探すと、グラフの
// 先頭のノードへ属性が付き、無関係なノードが観測値と firstRecordRef を持つ。
// addRecord が位置の表をレコードが記録したノードで埋めるため、除く分岐は到達しない。
func singleNodeByKind(keys []core.NodeKey, nodeAt map[string]int) map[core.NodeKind]int {
	counts := make(map[core.NodeKind]int, len(keys))
	single := make(map[core.NodeKind]int, len(keys))
	for _, key := range keys {
		index, present := nodeAt[nodeIdOf(key)]
		if !present {
			continue
		}
		// 同じ鍵が 2 回出たときは 1 つと数える。
		if previous, seen := single[key.Kind]; seen && previous == index {
			continue
		}
		counts[key.Kind]++
		single[key.Kind] = index
	}
	for kind, count := range counts {
		if count != 1 {
			delete(single, kind)
		}
	}
	return single
}

// comparableFieldValue は 1 項目の、文字列の一致を比べる値を返す。
//
// 値が時刻である項目は正規化値を返す。時刻の項目を除くと、process.start_time のように
// 読めている欄が索引から消える。
//
// **RawTextValue を返す行は Timestamp が表現しない状態への防御である。** 値がある時刻は
// 原資料の文字列と正規化値の両方を必ず持ち (Timestamp.validateAbsence)、欄が無い時刻
// (item_absent) はどちらも持たない。正規化値だけを欠く時刻は組めないため、その行に
// 到達する入力は現在の契約に無い。契約が変わったときに値を失わないよう、行を残す。
//
// ok が偽になるのは、値がある状態と導出できた状態のどちらでもない項目である。
func comparableFieldValue(field core.RecordField) (string, bool) {
	if field.Text != nil {
		return field.Text.ComparableValue()
	}
	if field.Timestamp == nil {
		return "", false
	}
	if normalized, present := field.Timestamp.NormalizedValue(); present {
		return normalized, true
	}
	return field.Timestamp.RawTextValue()
}

// addAttribute は位置 nodeAt のノードへ、同じ値の観測を 1 件にまとめて足す。
//
// **同じ値を探す表をグラフ全体で 1 つ持ち、組み終えたら捨てる** (NewObservedGraph)。
// 属性を足すのは観測の層を組む間だけである。
func (g *Graph) addAttribute(nodeAt int, field *core.RecordField, at int) {
	value, readable := comparableFieldValue(*field)
	if !readable {
		return
	}
	node := &g.nodes[nodeAt]
	key := attributeValueKey{node: nodeAt, semantic: field.Semantic, name: field.Name, value: value}
	if index, found := g.attributeAt[key]; found {
		node.attributes[index].evidence = append(node.attributes[index].evidence, at)
		return
	}
	node.attributes = append(node.attributes, graphAttribute{
		field: field, evidence: []int{at},
	})
	g.attributeAt[key] = len(node.attributes) - 1
}

// addFileContentMatchEdges は同じ算法の hash を観測した別々のファイルノードを候補で結ぶ。
// **hash の一致をファイル実体の同一性やコピーの来歴へ読み替えない。** 候補の根拠には、
// hash を各ノードで観測した全レコードを残す。関連付けの段階を経ないので、応答の matches は
// 空になる。
//
// **hash を観測したレコードの案件ごとに組を作る。** 同じファイルのノードを 2 つの案件が
// 観測したとき、案件をまたいだ 2 件の観測を 1 つの一致の根拠にしない。
func (g *Graph) addFileContentMatchEdges() {
	type hashKey struct {
		semantic core.SemanticKey
		value    string
		caseId   string
	}
	type nodeHash struct {
		index    int
		evidence []int
	}
	groups := make(map[hashKey][]nodeHash)
	groupOrder := make([]hashKey, 0)
	for index, node := range g.nodes {
		if node.key.Kind != core.NodeKindFile {
			continue
		}
		seen := make(map[hashKey]struct{})
		for _, attribute := range node.attributes {
			if attribute.field.Semantic != core.SemanticKeyFileMd5 &&
				attribute.field.Semantic != core.SemanticKeyFileSha1 &&
				attribute.field.Semantic != core.SemanticKeyFileSha256 {
				continue
			}
			if attribute.field.Text == nil {
				continue
			}
			value, readable := attribute.field.Text.ComparableValue()
			if !readable {
				continue
			}
			for _, caseId := range g.casesOf(attribute.evidence) {
				key := hashKey{semantic: attribute.field.Semantic, value: value, caseId: caseId}
				if _, alreadySeen := seen[key]; alreadySeen {
					continue
				}
				seen[key] = struct{}{}
				if _, exists := groups[key]; !exists {
					groupOrder = append(groupOrder, key)
				}
				groups[key] = append(groups[key], nodeHash{
					index: index, evidence: g.evidenceInCase(attribute.evidence, caseId),
				})
			}
		}
	}
	for _, key := range groupOrder {
		members := groups[key]
		for left := 0; left < len(members); left++ {
			for right := left + 1; right < len(members); right++ {
				at := g.ensureEdge(core.EdgeKindFileContentMatch, core.RelationStateCandidate,
					members[left].index, members[right].index)
				for _, evidence := range append(members[left].evidence, members[right].evidence...) {
					g.addEdgeEvidence(at, evidence)
				}
			}
		}
	}
}
