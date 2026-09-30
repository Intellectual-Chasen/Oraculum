package pipeline

import (
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// addProcessLineageEdges は、プロセスへ一意な識別子を振らない収集元のレコードから、
// プロセスのノードと親子の候補のエッジを足す。
//
// **プロセス番号だけで時間を跨いでまとめない。** 番号は OS が再利用するため、識別が
// 始まった時刻を鍵に入れて区間で区切る。一意な識別子を持たない対象の同一性は、識別が
// 有効な区間で区切る。
//
// **親子は候補である。** 親の実体を決める根拠は `(ppid, pid)` の一致と時刻の前後だけで
// あり、番号の再利用の区間が重なると別のプロセスを親に選ぶ。
func (g *Graph) addProcessLineageEdges(result ImportResult, terminals sourceTerminals) {
	instances, identified := g.processInstancesOf(result, terminals)
	for _, instance := range instances {
		g.addProcessInstance(instance)
		if instance.parent != noParent {
			g.addProcessParentCandidate(instances[instance.parent], instance)
		}
	}
	g.addProcessIdentityEdges(instances, identified)
}

// identifiedProcess は、一意な識別子で識別したプロセスのノード 1 つと、そのプロセスの
// プロセス番号を記録したレコードの位置である。
type identifiedProcess struct {
	node      int
	recordsAt []int
}

// identifiedProcesses は、端末のノードの識別子とプロセス番号の組 (identityPidKey) から、
// 一意な識別子で識別したプロセスを探す表である。
type identifiedProcesses map[string][]identifiedProcess

// identityPidKey は、端末のノードの識別子とプロセス番号から、同じプロセスの候補を探す鍵を組む。
func identityPidKey(terminalNodeId, pid string) string {
	return terminalNodeId + "\x00" + pid
}

// add は、一意な識別子のプロセスを記録したレコード 1 件を表へ入れる。レコードが記録した
// プロセスのノード (recordedProcessAt) とプロセス番号と、レコードを置いた端末で探す。
func (p identifiedProcesses) add(g Graph, record RecordEntry, at int) {
	if record.Semantics == nil || g.records[at].placedTerminalNodeId == "" {
		return
	}
	node, recorded := g.recordedProcessAt(at)
	pid, hasPid := comparableOfSemantic(record.Semantics.Fields, core.SemanticKeyProcessPid)
	if !recorded || !hasPid {
		return
	}
	key := identityPidKey(g.records[at].placedTerminalNodeId, pid)
	for index := range p[key] {
		if p[key][index].node == node {
			p[key][index].recordsAt = append(p[key][index].recordsAt, at)
			return
		}
	}
	p[key] = append(p[key], identifiedProcess{node: node, recordsAt: []int{at}})
}

// addProcessIdentityEdges は、プロセス番号と区間で識別したプロセスと、同じ端末・同じ番号の
// 一意な識別子のプロセスを、同じプロセスの候補で結ぶ。
//
// **2 つのプロセスの観測の時刻の範囲が重なる組だけを結ぶ。** 範囲は、そのノードを指した
// レコードの時刻の最小と最大である (秒の単位)。番号を再利用した別のプロセスは、範囲が
// 重ならない。
//
// 既知の制限: 範囲を観測したレコードの時刻で決め、起動の前と終了の後を範囲に入れない,
// 収録範囲の外の起動と終了を測れない, 観測の少ないプロセスの組が結ばれない実例が出たとき、
// 同じ番号の次の区間の始まりまでを範囲に入れる
func (g *Graph) addProcessIdentityEdges(instances []processInstance, identified identifiedProcesses) {
	if len(identified) == 0 {
		return
	}
	type span struct {
		from, to int64
		spanned  bool
	}
	spans := make(map[int]span)
	spanOf := func(node int) span {
		if held, found := spans[node]; found {
			return held
		}
		var computed span
		computed.from, computed.to, computed.spanned = g.secondSpanOf(g.nodes[node].evidence)
		spans[node] = computed
		return computed
	}
	for _, instance := range instances {
		processes := identified[identityPidKey(nodeIdOf(instance.terminal), instance.pid)]
		if len(processes) == 0 {
			continue
		}
		intervalAt, present := g.nodeAt[nodeIdOf(instance.key)]
		from, to, spanned := g.secondSpanOf(instance.recordsAt)
		if !present || !spanned {
			continue
		}
		for _, process := range processes {
			processSpan := spanOf(process.node)
			if !processSpan.spanned || processSpan.from > to || from > processSpan.to {
				continue
			}
			edge := g.ensureEdge(core.EdgeKindProcessIdentityMatch, core.RelationStateCandidate,
				process.node, intervalAt)
			// 識別子を持つレコードと持たないレコードは重ならず、組ごとにエッジは 1 本であるため、
			// 重なりを確かめずに足す。
			g.edges[edge].evidence = append(append(g.edges[edge].evidence, process.recordsAt...),
				instance.recordsAt...)
			g.addRecordPair(edge, pairRuleProcessIdentity,
				g.earliestRecord(process.recordsAt), g.earliestRecord(instance.recordsAt))
		}
	}
}

// earliestRecord は、時点を持つレコードのうち時刻の最も早いレコードの位置を返す。時点を持つ
// レコードが無いときは先頭の位置を返す。recordsAt は要素数 1 以上である。
func (g Graph) earliestRecord(recordsAt []int) int {
	earliest := recordsAt[0]
	for _, at := range recordsAt {
		record := g.records[at]
		if record.hasInstant && (!g.records[earliest].hasInstant || record.instant.Before(g.records[earliest].instant)) {
			earliest = at
		}
	}
	return earliest
}

// secondSpanOf は、レコードの時刻の最小と最大を UNIX 時刻の秒で返す。ok が偽になるのは、
// 時点を持つレコードが 1 件も無いときである。
func (g Graph) secondSpanOf(recordsAt []int) (int64, int64, bool) {
	var from, to int64
	spanned := false
	for _, at := range recordsAt {
		if !g.records[at].hasInstant {
			continue
		}
		second := g.records[at].instant.Unix()
		if !spanned || second < from {
			from = second
		}
		if !spanned || second > to {
			to = second
		}
		spanned = true
	}
	return from, to, spanned
}

// processObservation は、プロセス番号を持つレコード 1 件である。
type processObservation struct {
	// terminal は端末のノードの識別鍵である。
	terminal core.NodeKey
	// pid はプロセス番号の原資料の文字列である。
	pid string
	// parentPid は親のプロセス番号の原資料の文字列である。親の欄を持たないレコードでは空である。
	parentPid string
	// at は事象の時刻である。
	at time.Time
	// localClock は、at が UTC からのずれを持たない壁時計の日時であるかである
	// (core.Timestamp の LocalClockTime)。壁時計の日時は時点と比べない。
	localClock bool
	// atText は事象の時刻の正規化値である。
	atText string
	// startsProcess は、そのレコードがプロセスの起動を記録しているかである。
	startsProcess bool
	// endsProcess は、そのレコードがプロセスの終了を記録しているかである。
	endsProcess bool
	// sequence は、同じ時刻の観測どうしの前後を並びの順で決められる観測の組である。
	// sequence が異なる同じ時刻の 2 つの観測は、前後が決まらない。
	sequence int
	// recordAt はグラフの根拠のレコードの位置である。
	recordAt int
	// binaryPath はプロセスの実行ファイルの path である。読めないレコードでは出ない。
	binaryPath *core.RawAndNormalized
	// files は、そのレコードが端末の範囲で指すファイルのノードの識別鍵である。
	files []core.NodeKey
	// destinations は、そのレコードが指す通信の接続先のノードの識別鍵である。
	destinations []core.NodeKey
}

// namedObject は、区間に属するレコードが指すファイルか接続先 1 つと、プロセスから張る
// 関係の種別と、そのレコードの位置である。
type namedObject struct {
	kind     core.EdgeKind
	key      core.NodeKey
	recordAt int
}

// processInstance は、識別が有効な区間で区切ったプロセス 1 つと、その根拠のレコードである。
//
// **区間の始まりは、その番号の直近の起動の時刻である。** 事象ごとの時刻を鍵に入れると、
// 1 つのプロセスの連続した操作が事象の数だけ別のノードに割れる。
type processInstance struct {
	// key はプロセスのノードの識別鍵である。
	key core.NodeKey
	// pid はプロセス番号の原資料の文字列である。
	pid string
	// terminal は端末のノードの識別鍵である。
	terminal core.NodeKey
	// recordsAt はこの区間に属するレコードの位置である。並びは時刻の順である。
	recordsAt []int
	// creationRecordsAt は、この区間の生成を記録したレコードの位置である。区間を開いた
	// レコードが生成を記録していないとき (収録範囲の先頭より前に起動したプロセス) は
	// 要素数 0 である。
	creationRecordsAt []int
	// binaryPath はプロセスの実行ファイルの path である。読めないレコードでは出ない。
	binaryPath *core.RawAndNormalized
	// objects は、この区間に属するレコードが指すファイルと接続先である。
	objects []namedObject
	// parent は親のプロセスの区間の、processInstancesOf が返す並びの中の位置である。親を
	// 1 つに決められないときは noParent である。
	parent int
}

// noParent は、親を 1 つに決められなかった区間の processInstance.parent である。
const noParent = -1

// processInstancesOf は、プロセス番号を運びプロセスの外部識別子の項目を持たないレコードから
// プロセスの実体を組む。返す並びは区間の始まりの時刻の順である。
//
// **外部識別子を持つ収集元のレコードを対象にしない。** 識別子の一致で同一性が決まる
// レコードは、観測層のグラフが既にノードを作っている。
//
// **同じ番号の続きの事象を、開いている区間へ入れる。** 新しい区間を開くのは、その番号の
// 起動を記録したレコードと、区間を開いていない番号の最初のレコードだけである。終了を
// 記録したレコードが区間を閉じる。
//
// **親は、子の区間を開いた観測の時点で、親の番号の区間が開いていたプロセスである。**
// 番号は動いているプロセスの間で重ならない。親の番号の区間の開閉が観測と同じ時刻で、
// 前後を決められないときは、親を決めない。そのため、並びの向きが決まらない収集元と、
// 範囲の重なる複数の収集元では、同じ秒に起動した本当の親子も候補に出ない。同じ時刻の
// 同じ番号の起動と前後を決められない終了のレコードは、区間を閉じるだけで、どのプロセスの
// 根拠にも入らない。
//
// 一意な識別子のプロセスを記録したレコードは、同じプロセスの候補を探す表 (identifiedProcesses)
// に入る。
func (g Graph) processInstancesOf(
	result ImportResult, terminals sourceTerminals,
) ([]processInstance, identifiedProcesses) {
	observations, identified := g.processObservationsOf(result, terminals)
	// 区間を開く判定が時刻の順を前提にする。連番の順で並べない。同じ時刻の観測は
	// processObservationsOf が並べた順を保つ。
	sort.SliceStable(observations, func(left, right int) bool {
		return observations[left].at.Before(observations[right].at)
	})
	boundaries := processBoundariesOf(observations)
	instances := make([]processInstance, 0, len(observations))
	// open は番号ごとに開いている区間の、instances の中の位置である。
	open := make(map[string]int, len(observations))
	for _, observation := range observations {
		pidKey := processPidKey(observation, observation.pid)
		at, opened := open[pidKey]
		// 前後の決まらない終了は、どの区間の終了かが決まらない。区間を閉じ、レコードを
		// どの区間にも入れない。
		if observation.endsProcess && unorderedAtBoundary(observation, pidKey, boundaries) {
			delete(open, pidKey)
			continue
		}
		if !opened || observation.startsProcess {
			instance, built := processInstanceOf(observation)
			if !built {
				continue
			}
			instance.parent = liveParentOf(observation, open, boundaries)
			instances = append(instances, instance)
			at = len(instances) - 1
			open[pidKey] = at
		} else {
			instances[at].recordsAt = append(instances[at].recordsAt, observation.recordAt)
			instances[at].objects = append(instances[at].objects, observation.namedObjects()...)
			if instances[at].binaryPath == nil {
				instances[at].binaryPath = observation.binaryPath
			}
		}
		if observation.endsProcess {
			delete(open, pidKey)
		}
	}
	return instances, identified
}

// processPidKey は、観測の端末と時計の種類で、番号の区間を探す鍵を組む。
func processPidKey(observation processObservation, pid string) string {
	return nodeIdOf(observation.terminal) + "\x00" + pid + "\x00" + strconv.FormatBool(observation.localClock)
}

// processBoundary は、番号の区間を探す鍵と時刻の組である。
type processBoundary struct {
	pidKey string
	at     int64
}

// processBoundariesOf は、番号と時刻ごとに、起動か終了を記録した観測の sequence を返す。
func processBoundariesOf(observations []processObservation) map[processBoundary][]int {
	boundaries := make(map[processBoundary][]int)
	for _, observation := range observations {
		if observation.startsProcess || observation.endsProcess {
			key := processBoundary{processPidKey(observation, observation.pid), observation.at.UnixNano()}
			boundaries[key] = append(boundaries[key], observation.sequence)
		}
	}
	return boundaries
}

// liveParentOf は、観測の時点で親の番号の区間が開いていたプロセスの、instances の中の位置を
// 返す。親の欄を持たない観測、親の番号の区間が開いていない観測、親の番号の起動か終了が
// 観測と同じ時刻にあり前後を決められない観測では noParent を返す。
func liveParentOf(
	observation processObservation, open map[string]int, boundaries map[processBoundary][]int,
) int {
	if observation.parentPid == "" || observation.parentPid == observation.pid {
		return noParent
	}
	parentKey := processPidKey(observation, observation.parentPid)
	if unorderedAtBoundary(observation, parentKey, boundaries) {
		return noParent
	}
	if at, opened := open[parentKey]; opened {
		return at
	}
	return noParent
}

// unorderedAtBoundary は、観測と同じ時刻に、その番号の起動か終了が別の sequence にあるかを返す。
func unorderedAtBoundary(
	observation processObservation, pidKey string, boundaries map[processBoundary][]int,
) bool {
	for _, sequence := range boundaries[processBoundary{pidKey, observation.at.UnixNano()}] {
		if sequence != observation.sequence {
			return true
		}
	}
	return false
}

// processObservationsOf は、対象になるレコードを観測の並びへ直す。
//
// **同じ時刻の観測の前後は、収集元のレコードの並びで決める。** 時刻の降順に並ぶ収集元は
// 逆にして昇順にする。時刻の昇順にも降順にも並ばない収集元の観測と、別の収集元の観測は、
// 同じ時刻の前後を決めない (sequence)。
func (g Graph) processObservationsOf(
	result ImportResult, terminals sourceTerminals,
) ([]processObservation, identifiedProcesses) {
	observations := make([]processObservation, 0, 64)
	identified := make(identifiedProcesses)
	sequence := 0
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		published := g.publicationObservationsOf(publication, terminals, identified)
		ordered := orderByTime(published)
		for index := range published {
			published[index].sequence = sequence
			if !ordered {
				sequence++
			}
		}
		sequence++
		observations = append(observations, published...)
	}
	return observations, identified
}

// orderByTime は、1 つの収集元の観測が時刻の降順に並ぶとき逆にして昇順にする。ordered が
// 偽になるのは、観測が時刻の昇順にも降順にも並ばないときと、すべて同じ時刻で向きが
// 決まらないときである。
func orderByTime(observations []processObservation) (ordered bool) {
	ascending, descending := true, true
	for index := 1; index < len(observations); index++ {
		previous, current := observations[index-1].at, observations[index].at
		ascending = ascending && !current.Before(previous)
		descending = descending && !current.After(previous)
	}
	if descending && !ascending {
		slices.Reverse(observations)
	}
	return ascending != descending
}

// publicationObservationsOf は、1 つの収集元の対象になるレコードを、収集元の並びのまま
// 観測へ直す。
func (g Graph) publicationObservationsOf(
	publication SourcePublication, terminals sourceTerminals, identified identifiedProcesses,
) []processObservation {
	observations := make([]processObservation, 0, 64)
	for _, record := range publication.records {
		at, indexed := g.recordAtLocator(record.Locator)
		if !indexed {
			continue
		}
		if record.Semantics != nil && carriesSemanticValue(record.Semantics.Fields, core.SemanticKeyProcessId) {
			identified.add(g, record, at)
			continue
		}
		supplied, scope := terminals.forRecord(record)
		record.Terminal = append(slices.Clone(record.Terminal), supplied...)
		observation, built := processObservationOf(record, scope)
		if !built {
			continue
		}
		observation.recordAt = at
		observations = append(observations, observation)
	}
	return observations
}

// processObservationOf はレコード 1 件を観測へ直す。
// built が偽になるのは、番号と端末と時刻のいずれかを欠くレコードである。
//
// **端末の欄を持たないレコードは、収集元の端末に置く** (scope)。
//
// **時点を持たない時刻は、壁時計の日時で並べる。** UTC からのずれを補わない。壁時計の
// 日時の観測は、時点を持つ観測と別の区間と親に分け、2 つの時刻を比べない。分析者が
// 収集元の時刻の解釈を与えた時刻は Instant が時点を返し、時点で並ぶ。
//
// 既知の制限: 解釈を持たない同じ端末の壁時計の日時は、書いた時計の前後の順に並ぶとみなす,
// 壁時計の日時は夏時間の切り替えと時計の変更の時点を持たず、前後の逆転を検出できない,
// 解釈を持たない時刻の並べ方を変えるとき
func processObservationOf(record RecordEntry, scope core.RecordScope) (processObservation, bool) {
	if record.ObservedAt == nil {
		return processObservation{}, false
	}
	at, comparable := record.ObservedAt.Instant()
	localClock := false
	if !comparable {
		at, localClock = record.ObservedAt.LocalClockTime()
		comparable = localClock
	}
	if !comparable {
		return processObservation{}, false
	}
	atText, hasText := record.ObservedAt.NormalizedValue()
	if !hasText {
		return processObservation{}, false
	}
	fields := graphFieldsOf(record)
	// 外部識別子の項目を持つレコードは観測層のグラフが扱う。**値の状態を問わない。** 識別子の
	// 欄を持つ収集元は番号の再利用を識別子で区切るため、識別子が値を持たないレコードを番号で
	// 区間のプロセスへ入れると、別のプロセスの操作と親子の候補が混ざる。
	if len(fieldsWithSemantic(fields, core.SemanticKeyProcessId)) > 0 {
		return processObservation{}, false
	}
	pid, hasPid := comparableOfSemantic(fields, core.SemanticKeyProcessPid)
	if !hasPid {
		return processObservation{}, false
	}
	terminal, built := core.RecordTerminalNodeKey(fields, scope)
	if !built {
		return processObservation{}, false
	}
	observation := processObservation{
		terminal: terminal, pid: pid, at: at, localClock: localClock, atText: atText,
		// **起動の根拠は、コマンド行と、binding が起動と宣言したレコードである。** 実行した
		// プログラムと引数を記録したレコードと、起動を記録する種別のイベントが、その番号で
		// 新しいプロセスが始まったことを示す。コマンド行の記録を止めた端末の起動のイベントは
		// コマンド行を持たない。
		startsProcess: carriesSemanticValue(fields, core.SemanticKeyProcessCommandLine) ||
			recordsProcessStart(record),
		endsProcess: record.Semantics != nil && record.Semantics.ProcessEnd,
	}
	observation.parentPid, _ = comparableOfSemantic(fields, core.SemanticKeyParentProcessPid)
	// ファイルの鍵は観測層のグラフと同じ経路で組む。同じレコードが指すファイルの
	// ノードを指す。複写先のファイルは file_copy が表すため含めない。
	observation.files = core.OperatedFileNodeKeys(fields, scope)
	observation.destinations = core.DestinationNodeKeys(fields, scope)
	if label, labelled := labelOfSemantic(fields, core.SemanticKeyProcessBinaryPath); labelled {
		observation.binaryPath = &label
	}
	return observation, true
}

// namedObjects は、観測のレコードが指すファイルと接続先を、関係の種別とレコードの位置と
// 組にして返す。
func (o processObservation) namedObjects() []namedObject {
	objects := make([]namedObject, 0, len(o.files)+len(o.destinations))
	for _, key := range o.files {
		objects = append(objects, namedObject{core.EdgeKindFileOperation, key, o.recordAt})
	}
	for _, key := range o.destinations {
		objects = append(objects, namedObject{core.EdgeKindProcessCommunication, key, o.recordAt})
	}
	return objects
}

// carriesSemanticValue は、その意味の項目が読めた値を持つかを返す。
func carriesSemanticValue(fields []core.RecordField, semantic core.SemanticKey) bool {
	_, present := comparableOfSemantic(fields, semantic)
	return present
}

// processInstanceOf は観測 1 件から、新しい区間を開く。
// built が偽になるのは、区間で区切る鍵を組めない観測である。
func processInstanceOf(observation processObservation) (processInstance, bool) {
	key, keyed := core.NewProcessIntervalNodeKey(
		observation.terminal, observation.pid, observation.atText)
	if !keyed {
		return processInstance{}, false
	}
	instance := processInstance{
		key:        key,
		pid:        observation.pid,
		terminal:   observation.terminal,
		recordsAt:  []int{observation.recordAt},
		objects:    observation.namedObjects(),
		binaryPath: observation.binaryPath,
	}
	if observation.startsProcess {
		instance.creationRecordsAt = []int{observation.recordAt}
	}
	return instance, true
}

// addProcessInstance はプロセスのノードと、端末で動いた関係を足す。
func (g *Graph) addProcessInstance(instance processInstance) {
	at := g.ensureNode(instance.key, core.NodeObservationObserved)
	g.nodes[at].evidence = append(g.nodes[at].evidence, instance.recordsAt...)
	for _, recordAt := range instance.creationRecordsAt {
		g.nodes[at].addCreationRecord(recordAt)
	}
	if instance.binaryPath != nil {
		g.nodes[at].applyLabel(*instance.binaryPath)
	}
	// **レコードのノードは、そのレコードが記録したプロセスを指す。** 観測層のグラフは
	// プロセス番号からプロセスを作らないため、レコードがプロセスを指す関係をここで足す。
	for _, recordAt := range instance.recordsAt {
		g.setRecordedProcess(recordAt, at)
		if record := g.records[recordAt]; record.hasRecordNode {
			edge := g.ensureEdge(core.EdgeKindRecordNamesObject, core.RelationStateObserved,
				record.recordNode, at)
			g.addEdgeEvidence(edge, recordAt)
		}
	}
	// **同じレコードが指すファイルを、そのプロセスの操作の対象にする。接続先を、その
	// プロセスの通信の相手にする。**
	for _, object := range instance.objects {
		if objectAt, named := g.nodeAt[nodeIdOf(object.key)]; named {
			edge := g.ensureEdge(object.kind, core.RelationStateObserved, at, objectAt)
			g.addEdgeEvidence(edge, object.recordAt)
		}
	}
	terminalAt, present := g.nodeAt[nodeIdOf(instance.terminal)]
	if !present {
		return
	}
	edge := g.ensureEdge(core.EdgeKindRanOn, core.RelationStateObserved, at, terminalAt)
	for _, recordAt := range instance.recordsAt {
		g.addEdgeEvidence(edge, recordAt)
	}
}

// addProcessParentCandidate は、processInstancesOf が選んだ親との候補のエッジを足す。
func (g *Graph) addProcessParentCandidate(parent, child processInstance) {
	parentAt, parentPresent := g.nodeAt[nodeIdOf(parent.key)]
	childAt, childPresent := g.nodeAt[nodeIdOf(child.key)]
	if !parentPresent || !childPresent {
		return
	}
	edge := g.ensureEdge(core.EdgeKindProcessParentChild, core.RelationStateCandidate,
		parentAt, childAt)
	// 親を選んだのは子の区間を開いた観測であり、その時点で親の区間を開いていたレコードと組む。
	// 親の区間を開いたレコードは、起動を記録していれば親の起動のレコードである。
	g.addRecordPair(edge, pairRuleProcessParent, parent.recordsAt[0], child.recordsAt[0])
	for _, recordAt := range child.recordsAt {
		g.addEdgeEvidence(edge, recordAt)
	}
}
