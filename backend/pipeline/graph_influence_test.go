// in-package test: 根拠のレコードから作る影響のエッジを確かめる。
package pipeline

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// influenceBase はグラフの時刻の基準である。
var influenceBase = time.Date(2001, 2, 3, 4, 0, 0, 0, time.UTC)

// fakeGraph は、影響のエッジを確かめるために Graph の索引を直に組む。
type fakeGraph struct {
	t testing.TB
	g Graph
}

func newFakeGraph(t testing.TB) *fakeGraph {
	return &fakeGraph{t: t, g: Graph{nodeAt: map[string]int{}, edgeAt: map[string]int{}, recordAt: map[string]int{}}}
}

// node は種別 kind、名前 name のノードを足す。識別子は "n:" と名前である。
func (f *fakeGraph) node(kind core.NodeKind, name string) int {
	f.g.nodes = append(f.g.nodes, graphNode{id: "n:" + name, key: core.NodeKey{Kind: kind}, label: core.NewAbsentItemValue()})
	f.g.adjacency = append(f.g.adjacency, nodeAdjacency{})
	f.g.nodeAt["n:"+name] = len(f.g.nodes) - 1
	return len(f.g.nodes) - 1
}

// recordOption はレコードの項目を変える。
type recordOption func(*fakeGraph, int)

// byProcess はレコードが記録したプロセスを process にする。
func byProcess(process int) recordOption {
	return func(f *fakeGraph, at int) { f.g.setRecordedProcess(at, process) }
}

// withFlow はレコードの操作の分類を operation にする。
func withFlow(operation core.FlowOperation) recordOption {
	return func(f *fakeGraph, at int) { f.g.records[at].flowOperation = operation }
}

// inferredKind はレコードの観測の種別の意味を推定した状態にする。
func inferredKind() recordOption {
	return func(f *fakeGraph, at int) {
		f.g.records[at].observationKind.Status = core.ObservationKindStatusInferred
	}
}

// replacing はレコードを内容を置き換える記録にする。
func replacing() recordOption {
	return func(f *fakeGraph, at int) { f.g.records[at].replacesContent = true }
}

// untimed はレコードのタイムスタンプを無くす。
func untimed() recordOption {
	return func(f *fakeGraph, at int) {
		f.g.records[at].hasInstant = false
		f.g.records[at].eventTime = nil
	}
}

// localClock はレコードのタイムスタンプを、UTC からのずれの分からない地方時の文字列にする。
// 壁時計の日時は、足したときの時刻と同じである。
func localClock() recordOption {
	return func(f *fakeGraph, at int) {
		text := f.g.records[at].instant.Format("2006-01-02T15:04:05")
		local, err := core.NewTimestamp(core.Timestamp{
			RawText: &text, Normalized: &text, NormalizedForm: core.NormalizedFormLocalWithoutOffset,
			Precision: core.PrecisionSecond, OffsetState: core.OffsetStateItemAbsent,
			Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
		})
		if err != nil {
			f.t.Fatal(err)
		}
		f.g.records[at].eventTime, f.g.records[at].hasInstant = &local, false
	}
}

// plusMillis はレコードの時刻を ms だけ後へずらす。
func plusMillis(ms int) recordOption {
	return func(f *fakeGraph, at int) {
		f.g.records[at].instant = f.g.records[at].instant.Add(time.Duration(ms) * time.Millisecond)
	}
}

// secondPrecise はレコードのタイムスタンプの精度を秒にする。
func secondPrecise() recordOption {
	return func(f *fakeGraph, at int) { f.g.records[at].eventTime.Precision = core.PrecisionSecond }
}

// onTerminal はレコードのタイムスタンプを、端末 terminal の時計の値にする。
func onTerminal(terminal string) recordOption {
	return func(f *fakeGraph, at int) {
		f.g.records[at].eventTime.Clock = core.ClockTerminalLocal
		f.g.records[at].placedTerminalNodeId = terminal
	}
}

// withoutRecordNode はレコードのノードを無くす。
func withoutRecordNode() recordOption {
	return func(f *fakeGraph, at int) { f.g.records[at].hasRecordNode = false }
}

// withField はレコードのノードへ、語彙の項目 semantic の値 text の欄を足す。
func withField(semantic core.SemanticKey, text string) recordOption {
	return func(f *fakeGraph, at int) {
		field := textRecordField(f.t, string(semantic), text)
		field.Semantic = semantic
		node := &f.g.nodes[f.g.records[at].recordNode]
		node.attributes = append(node.attributes, graphAttribute{field: &field, evidence: []int{at}})
	}
}

// record は基準から seconds 秒のレコードを、レコードのノードと共に足す。
func (f *fakeGraph) record(seconds int, options ...recordOption) int {
	at := len(f.g.records)
	position := int64(at + 1)
	instant := influenceBase.Add(time.Duration(seconds) * time.Second)
	text := instant.Format("2006-01-02T15:04:05.000Z")
	eventTime, err := core.NewTimestamp(core.Timestamp{
		RawText: &text, Normalized: &text, NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision: core.PrecisionMillisecond, OffsetState: core.OffsetStateInValue, OffsetText: ptrTo("Z"),
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.g.records = append(f.g.records, graphRecord{
		locator: core.RecordLocator{
			SourceId: "s1", SourceContentSha256: strings.Repeat("c", 64), PositionKind: core.PositionKindSequenceNumber,
			SequenceNumber: &position, SourceFileName: "t.log", RecordRawTextRef: "raw:" + strconv.Itoa(at),
		},
		eventTime: &eventTime, instant: instant, hasInstant: true,
	})
	f.g.records[at].recordNode = f.node(core.NodeKindRecord, "r"+strconv.Itoa(at))
	f.g.records[at].hasRecordNode = true
	f.g.nodes[f.g.records[at].recordNode].evidence = []int{at}
	for _, option := range options {
		option(f, at)
	}
	return at
}

// edge は種別 kind のエッジを足す。
func (f *fakeGraph) edge(kind core.EdgeKind, state core.RelationState, source, target int, evidence ...int) int {
	f.g.edges = append(f.g.edges, graphEdge{
		id: "e:" + string(kind) + ":" + strconv.Itoa(len(f.g.edges)), kind: kind, state: state,
		source: source, target: target, evidence: evidence,
	})
	at := len(f.g.edges) - 1
	f.g.adjacency[source].outgoing = append(f.g.adjacency[source].outgoing, at)
	f.g.adjacency[target].incoming = append(f.g.adjacency[target].incoming, at)
	for _, record := range evidence {
		f.g.nodes[source].evidence = append(f.g.nodes[source].evidence, record)
		f.g.nodes[target].evidence = append(f.g.nodes[target].evidence, record)
	}
	return at
}

// pair はエッジ edge へレコードの組を足す。
func (f *fakeGraph) pair(edge int, rule pairRule, left, right int) {
	f.g.addRecordPair(edge, rule, left, right)
}

// described は影響のエッジを「元>先 向きの根拠/関係の状態/同一性 レコード D A」の文字列で返す。
// 時刻は基準からの秒である。
func described(g Graph, built *influenceGraph) []string {
	seconds := func(span timeSpan) string {
		text := func(value int64) string {
			switch value {
			case noLower:
				return "-"
			case noUpper:
				return "+"
			}
			return strconv.FormatFloat(float64(value)/float64(time.Second), 'f', -1, 64)
		}
		return "[" + text(span.lo) + "," + text(span.hi) + "]"
	}
	spans := func(constraints []clockSpan) string {
		var parts []string
		for _, constraint := range constraints {
			parts = append(parts, seconds(constraint.span))
		}
		return fmt.Sprint(parts)
	}
	var lines []string
	for _, edge := range built.edges {
		lines = append(lines, fmt.Sprintf("%s>%s %s/%s/%s r%v D%s A%s",
			g.vertexKeyOf(built, edge.exact.from), g.vertexKeyOf(built, edge.exact.to),
			edge.bases[0], edge.bases[1], edge.bases[2], edge.records, spans(edge.exact.depart), spans(edge.exact.arrive)))
	}
	slices.Sort(lines)
	return lines
}

func requireInfluence(t *testing.T, f *fakeGraph, want ...string) *influenceGraph {
	t.Helper()
	built := f.g.newInfluenceGraph(influenceBase)
	got := described(f.g, built)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("influence edges:\n%s\nwant:\n%s", joinLines(got), joinLines(want))
	}
	return built
}

func joinLines(lines []string) string {
	var text string
	for _, line := range lines {
		text += "  " + line + "\n"
	}
	return text
}

// 書き込みはプロセスから内容のバージョンへ、読み込みは内容のバージョンからプロセスへ渡る。
// 置き換えの記録の前の書き込みは、置き換えの後のバージョンにつながらない。
func TestInfluenceFollowsTheFileOperationsIntoContentVersions(t *testing.T) {
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	file := f.node(core.NodeKindFile, "file")
	early := f.record(10, byProcess(writer), withFlow(core.FlowOperationWrite))
	replaced := f.record(15, byProcess(writer), withFlow(core.FlowOperationWrite), replacing())
	read := f.record(20, byProcess(reader), withFlow(core.FlowOperationRead))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, early, replaced)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file, read)
	version := "n:file@2001-02-03T04:00:15Z"
	requireInfluence(t, f,
		"n:writer>n:file specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:writer>"+version+" specified_operation/observed/single_node r[1] D[[15,15]] A[[15,15]]",
		version+">n:reader specified_operation/observed/single_node r[2] D[[20,20]] A[[20,20]]",
	)
}

// 読み書きの量を持つ記録は、量が正の向きを観測とし、量が 0 の向きと量の欄が無い向きを、向きを
// 決められない向きとする。期間の始まりの欄が無く、プロセスの起動が記録にあるときは、起動から
// レコードのタイムスタンプまでを区間にする。起動も無いときは下限を持たない。
func TestInfluenceReadsTheDirectionFromTheAmounts(t *testing.T) {
	f := newFakeGraph(t)
	started, bare := f.node(core.NodeKindProcess, "started"), f.node(core.NodeKindProcess, "bare")
	start := f.record(1, byProcess(started))
	f.g.nodes[started].addCreationRecord(start)
	file := f.node(core.NodeKindFile, "file")
	written := f.record(10, byProcess(started),
		withField(core.SemanticKeyEventWrittenBytes, "5"), withField(core.SemanticKeyEventReadBytes, "0"))
	read := f.record(20, byProcess(bare), withField(core.SemanticKeyEventReadBytes, "3"))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, started, file, written)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, bare, file, read)
	requireInfluence(t, f,
		"n:started>n:file specified_operation/observed/single_node r[1] D[[1,10]] A[[1,10]]",
		"n:file>n:started undetermined_direction/observed/single_node r[1] D[[1,10]] A[[1,10]]",
		"n:file>n:bare specified_operation/observed/single_node r[2] D[[-,20]] A[[-,20]]",
		"n:bare>n:file undetermined_direction/observed/single_node r[2] D[[-,20]] A[[-,20]]",
	)
}

// 操作を始めた時刻の欄を持つ期間の記録は、その時刻から区間を始める。
func TestInfluenceStartsThePeriodAtTheOperationStartTime(t *testing.T) {
	f := newFakeGraph(t)
	process, file := f.node(core.NodeKindProcess, "p"), f.node(core.NodeKindFile, "file")
	record := f.record(10, byProcess(process), withField(core.SemanticKeyEventWrittenBytes, "1"),
		withField(core.SemanticKeyEventReadBytes, "1"))
	startTime, err := core.NewTimestamp(core.Timestamp{
		RawText: ptrTo("2001-02-03T04:00:04Z"), Normalized: ptrTo("2001-02-03T04:00:04Z"),
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionSecond,
		OffsetState: core.OffsetStateInValue, OffsetText: ptrTo("Z"), Clock: core.ClockTerminalLocal,
		Meaning: core.MeaningOperationStart, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	field := core.RecordField{Name: "start", Semantic: core.SemanticKeyEventOperationStartTime, Timestamp: &startTime}
	node := &f.g.nodes[f.g.records[record].recordNode]
	node.attributes = append(node.attributes, graphAttribute{field: &field, evidence: []int{record}})
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, process, file, record)
	requireInfluence(t, f,
		"n:p>n:file specified_operation/observed/single_node r[0] D[[4,10]] A[[4,10]]",
		"n:file>n:p specified_operation/observed/single_node r[0] D[[4,10]] A[[4,10]]",
	)
}

func ptrTo(text string) *string { return &text }

// 分類の無い記録は両方の向きを、向きを決められない記録として作る。観測の種別の意味を推定した
// 記録の操作は、仕様にない記録の推定である。通信は両方の向きを作る。
func TestInfluenceMarksTheDirectionBasis(t *testing.T) {
	f := newFakeGraph(t)
	process, value := f.node(core.NodeKindProcess, "p"), f.node(core.NodeKindRegistryValue, "value")
	unclassified := f.record(10, byProcess(process))
	inferred := f.record(20, byProcess(process), withFlow(core.FlowOperationWrite), inferredKind())
	f.edge(core.EdgeKindRegistryOperation, core.RelationStateObserved, process, value, unclassified, inferred)
	address := f.node(core.NodeKindIp, "address")
	communication := f.record(30, byProcess(process), withFlow(core.FlowOperationCommunication))
	accepted := f.record(40, byProcess(process), withFlow(core.FlowOperationRead))
	sent := f.record(50, byProcess(process), withField(core.SemanticKeyConnectionSentBytes, "7"))
	noNode := f.record(60, byProcess(process), withoutRecordNode())
	f.edge(core.EdgeKindProcessCommunication, core.RelationStateObserved, process, address,
		communication, accepted, sent, noNode)
	requireInfluence(t, f,
		"n:p>n:value undetermined_direction/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:value>n:p undetermined_direction/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:p>n:value inferred_record/observed/single_node r[1] D[[20,20]] A[[20,20]]",
		"n:p>n:r2 specified_operation/observed/single_node r[2] D[[30,30]] A[[30,30]]",
		"n:r2>n:p specified_operation/observed/single_node r[2] D[[30,30]] A[[30,30]]",
		"n:r3>n:p specified_operation/observed/single_node r[3] D[[40,40]] A[[40,40]]",
		// 送受信の量は接続の全期間の量であり、期間の記録である。始まりも起動も記録に無い。
		"n:p>n:r4 specified_operation/observed/single_node r[4] D[[-,50]] A[[-,50]]",
		"n:r4>n:p undetermined_direction/observed/single_node r[4] D[[-,50]] A[[-,50]]",
	)
}

// 名前の変更は、プロセスから新しい名前のファイルへ、元の名前のファイルから新しい名前のファイルへ
// 渡る。元の名前のファイルへの操作の関係からは作らない。コピーはコピー元からコピー先へだけ渡る。
func TestInfluenceFollowsRenameAndCopy(t *testing.T) {
	f := newFakeGraph(t)
	process := f.node(core.NodeKindProcess, "p")
	old, renamed, copied := f.node(core.NodeKindFile, "old"), f.node(core.NodeKindFile, "new"), f.node(core.NodeKindFile, "copy")
	rename := f.record(10, byProcess(process), withFlow(core.FlowOperationRename))
	duplicate := f.record(20, byProcess(process))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, process, old, rename)
	f.edge(core.EdgeKindFileCopy, core.RelationStateObserved, old, renamed, rename)
	f.edge(core.EdgeKindFileCopy, core.RelationStateObserved, old, copied, duplicate)
	requireInfluence(t, f,
		"n:old>n:new specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:p>n:new specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:old>n:copy specified_operation/observed/single_node r[1] D[[20,20]] A[[20,20]]",
	)
}

// 観測の種別の意味を推定した名前の変更は、元の名前から新しい名前へのエッジも、プロセスから
// 新しい名前へのエッジも、仕様にない記録の推定である。
func TestInfluenceMarksTheInferredRenameOnBothEdges(t *testing.T) {
	f := newFakeGraph(t)
	process := f.node(core.NodeKindProcess, "p")
	old, renamed := f.node(core.NodeKindFile, "old"), f.node(core.NodeKindFile, "new")
	rename := f.record(10, byProcess(process), withFlow(core.FlowOperationRename), inferredKind())
	f.edge(core.EdgeKindFileCopy, core.RelationStateObserved, old, renamed, rename)
	requireInfluence(t, f,
		"n:old>n:new inferred_record/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:p>n:new inferred_record/observed/single_node r[0] D[[10,10]] A[[10,10]]",
	)
}

// 子の起動、コードの注入、実行は観測の各レコードの点で渡る。候補の親子は、親のレコードを D、子の
// レコードを A にする。タイムスタンプの無いレコードは影響のエッジにせず、数える。
func TestInfluenceFollowsProcessStarts(t *testing.T) {
	f := newFakeGraph(t)
	parent, child, target := f.node(core.NodeKindProcess, "parent"), f.node(core.NodeKindProcess, "child"), f.node(core.NodeKindProcess, "target")
	image := f.node(core.NodeKindFile, "image")
	start := f.record(10, byProcess(child))
	missing := f.record(0, byProcess(child), untimed())
	injection := f.record(20, byProcess(child))
	f.edge(core.EdgeKindProcessParentChild, core.RelationStateObserved, parent, child, start, missing)
	f.edge(core.EdgeKindProcessExecutable, core.RelationStateObserved, image, child, start)
	f.edge(core.EdgeKindProcessInjection, core.RelationStateObserved, child, target, injection)
	later, laterChild := f.record(5), f.record(30)
	candidate := f.edge(core.EdgeKindProcessParentChild, core.RelationStateCandidate, child, target, laterChild)
	f.pair(candidate, pairRuleProcessParent, later, laterChild)
	built := requireInfluence(t, f,
		"n:parent>n:child specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:image>n:child specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:child>n:target specified_operation/observed/single_node r[2] D[[20,20]] A[[20,20]]",
		"n:child>n:target specified_operation/candidate/single_node r[3 4] D[[5,5]] A[[30,30]]",
	)
	if len(built.untimed) != 1 {
		t.Errorf("untimed records = %d, want 1", len(built.untimed))
	}
}

// 2 件のレコードの組の関係は、関係の種別ごとの端を結ぶ。レコードが記録したプロセスを持たない
// ときはレコードのノードを端にする。同じ接続の組のうち、同じ端末の組からは作らない。
func TestInfluenceFollowsTheRecordPairs(t *testing.T) {
	f := newFakeGraph(t)
	subject, runner := f.node(core.NodeKindProcess, "subject"), f.node(core.NodeKindProcess, "runner")
	registration := f.record(10, byProcess(subject)) // r0
	run := f.record(20, byProcess(runner))           // r1
	unrecorded := f.record(25)                       // r2
	task := f.edge(core.EdgeKindTaskRegistrationRun, core.RelationStateCandidate,
		f.g.records[registration].recordNode, f.g.records[run].recordNode, registration, run)
	f.pair(task, pairRuleTaskRun, registration, run)
	f.pair(task, pairRuleTaskRun, registration, unrecorded)
	connection, logon := f.record(30), f.record(31) // r3 r4
	f.pair(f.edge(core.EdgeKindConnectionLogonMatch, core.RelationStateCandidate,
		f.g.records[connection].recordNode, f.g.records[logon].recordNode, connection, logon),
		pairRuleConnectionLogon, connection, logon)
	request, ticketLogon := f.record(40, byProcess(subject)), f.record(41) // r5 r6
	f.pair(f.edge(core.EdgeKindTicketRequestLogon, core.RelationStateCandidate,
		f.g.records[request].recordNode, f.g.records[ticketLogon].recordNode, request, ticketLogon),
		pairRuleTicketLogon, request, ticketLogon)
	operation := f.record(50, byProcess(runner)) // r7
	f.pair(f.edge(core.EdgeKindLogonSessionOperation, core.RelationStateCandidate,
		f.g.records[logon].recordNode, f.g.records[operation].recordNode, logon, operation),
		pairRuleLogonSession, logon, operation)
	opened, closed := f.record(60), f.record(61) // r8 r9
	onTerminal := f.edge(core.EdgeKindSameConnectionMatch, core.RelationStateCandidate,
		f.g.records[opened].recordNode, f.g.records[closed].recordNode, opened, closed)
	f.pair(onTerminal, pairRuleSameConnectionOnTerminal, opened, closed)
	accepting := f.record(62, byProcess(runner)) // r10
	across := f.edge(core.EdgeKindSameConnectionMatch, core.RelationStateCandidate,
		f.g.records[opened].recordNode, f.g.records[accepting].recordNode, opened, accepting)
	f.pair(across, pairRuleSameConnectionAcrossTerminals, opened, accepting)
	explicitRequest, explicitLogon := f.record(70, byProcess(runner)), f.record(71) // r11 r12
	f.pair(f.edge(core.EdgeKindExplicitCredentialLogon, core.RelationStateCandidate,
		f.g.records[explicitRequest].recordNode, f.g.records[explicitLogon].recordNode, explicitRequest, explicitLogon),
		pairRuleExplicitCredential, explicitRequest, explicitLogon)
	requireInfluence(t, f,
		"n:runner>n:r12 specified_operation/candidate/single_node r[11 12] D[[70,70]] A[[71,71]]",
		"n:subject>n:r0 specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:r0>n:runner specified_operation/candidate/single_node r[0 1] D[[10,10]] A[[20,20]]",
		"n:r0>n:r2 specified_operation/candidate/single_node r[0 2] D[[10,10]] A[[25,25]]",
		"n:r3>n:r4 specified_operation/candidate/single_node r[3 4] D[[30,30]] A[[31,31]]",
		"n:subject>n:r6 specified_operation/candidate/single_node r[5 6] D[[40,40]] A[[41,41]]",
		"n:r4>n:runner specified_operation/candidate/single_node r[4 7] D[[31,31]] A[[50,50]]",
		"n:r8>n:runner specified_operation/candidate/single_node r[8 10] D[[60,60]] A[[62,62]]",
	)
}

// 組の端のレコードのノードが無いときは作らない。登録のレコードがプロセスを記録しないときは、
// 登録の内容のバージョンへの書き込みを作らない。
func TestInfluenceSkipsPairsWithoutEnds(t *testing.T) {
	f := newFakeGraph(t)
	registration, run := f.record(10, withoutRecordNode()), f.record(20)
	task := f.edge(core.EdgeKindTaskRegistrationRun, core.RelationStateCandidate,
		f.g.records[run].recordNode, f.g.records[run].recordNode, registration, run)
	f.pair(task, pairRuleTaskRun, registration, run)
	untimedRequest, logon := f.record(0, untimed()), f.record(30)
	explicit := f.edge(core.EdgeKindExplicitCredentialLogon, core.RelationStateCandidate,
		f.g.records[untimedRequest].recordNode, f.g.records[logon].recordNode, untimedRequest, logon)
	f.pair(explicit, pairRuleExplicitCredential, untimedRequest, logon)
	built := requireInfluence(t, f)
	if len(built.untimed) != 1 {
		t.Errorf("untimed records = %d, want 1", len(built.untimed))
	}
}

// 等価の影響のエッジは向きが逆の 2 本であり、D と A はどちらも両方のノードの同一性が成り立つ期間の
// 制約を持つ。分けたログオンは各ログオンのタイムスタンプから後、プロセス番号の一致は各ノードを
// 観測したタイムスタンプの範囲である。
func TestInfluenceLinksEquivalentNodes(t *testing.T) {
	f := newFakeGraph(t)
	first, second := f.record(10), f.record(12)
	f.pair(f.edge(core.EdgeKindLinkedLogon, core.RelationStateCandidate,
		f.g.records[first].recordNode, f.g.records[second].recordNode, first, second),
		pairRuleLinkedLogon, first, second)
	unique, interval := f.node(core.NodeKindProcess, "unique"), f.node(core.NodeKindProcess, "interval")
	f.g.nodes[unique].evidence = []int{f.record(20), f.record(40)}
	f.g.nodes[interval].evidence = []int{f.record(30), f.record(50), f.record(0, untimed())}
	identity := f.edge(core.EdgeKindProcessIdentityMatch, core.RelationStateCandidate, unique, interval)
	f.pair(identity, pairRuleProcessIdentity, 2, 4)
	requireInfluence(t, f,
		"n:r0>n:r1 specified_operation/candidate/equivalence r[0 1] D[[10,+] [12,+]] A[[10,+] [12,+]]",
		"n:r1>n:r0 specified_operation/candidate/equivalence r[0 1] D[[10,+] [12,+]] A[[10,+] [12,+]]",
		"n:unique>n:interval specified_operation/candidate/equivalence r[2 4] D[[20,40] [30,50]] A[[20,40] [30,50]]",
		"n:interval>n:unique specified_operation/candidate/equivalence r[2 4] D[[20,40] [30,50]] A[[20,40] [30,50]]",
	)
}

// 等価の影響のエッジは、片方のレコードまたはノードがタイムスタンプを持たないとき作らない。
func TestInfluenceSkipsEquivalenceWithoutTimestamps(t *testing.T) {
	f := newFakeGraph(t)
	timed, missing := f.record(10), f.record(0, untimed())
	f.pair(f.edge(core.EdgeKindLinkedLogon, core.RelationStateCandidate,
		f.g.records[timed].recordNode, f.g.records[missing].recordNode, timed, missing),
		pairRuleLinkedLogon, timed, missing)
	noNode := f.record(20, withoutRecordNode())
	f.pair(f.edge(core.EdgeKindLinkedLogon, core.RelationStateCandidate,
		f.g.records[timed].recordNode, f.g.records[timed].recordNode, timed, noNode),
		pairRuleLinkedLogon, timed, noNode)
	unique, interval := f.node(core.NodeKindProcess, "unique"), f.node(core.NodeKindProcess, "interval")
	f.g.nodes[unique].evidence = []int{f.record(0, untimed())}
	f.edge(core.EdgeKindProcessIdentityMatch, core.RelationStateCandidate, unique, interval)
	built := requireInfluence(t, f)
	if len(built.untimed) != 2 {
		t.Errorf("untimed records = %d, want 2", len(built.untimed))
	}
}

// ログオンのセッションからの関係は、D がセッションの期間、A が終点のログオンのレコードである。
// 関係の状態は関係のものを持つ。
func TestInfluenceLeavesTheSessionDuringItsPeriod(t *testing.T) {
	f := newFakeGraph(t)
	start, end, logon := f.record(10), f.record(100), f.record(50)
	chain := f.edge(core.EdgeKindLogonChain, core.RelationStateUncertainChain,
		f.g.records[start].recordNode, f.g.records[logon].recordNode, start, end, logon)
	f.g.edges[chain].ensureBasis().sessions = []sessionSpan{{
		start: f.g.records[start].instant, end: f.g.records[end].instant.Add(time.Hour),
		startAt: start, endAt: end, target: logon,
	}}
	requested := f.edge(core.EdgeKindRequestedSessionLogon, core.RelationStateObserved,
		f.g.records[start].recordNode, f.g.records[logon].recordNode, start, logon)
	missing := f.record(0, untimed())
	f.g.edges[requested].ensureBasis().sessions = []sessionSpan{
		{start: f.g.records[start].instant, end: f.g.records[start].instant, startAt: start, endAt: start, target: logon},
		{start: f.g.records[start].instant, end: f.g.records[start].instant, startAt: start, endAt: start, target: missing},
	}
	built := requireInfluence(t, f,
		"n:r0>n:r2 specified_operation/uncertain_chain/single_node r[0 1 2] D[[10,3700]] A[[50,50]]",
		"n:r0>n:r2 specified_operation/observed/single_node r[0 2] D[[10,10]] A[[50,50]]",
	)
	if len(built.untimed) != 1 {
		t.Errorf("untimed records = %d, want 1", len(built.untimed))
	}
}

// 別の収集元のレコードが表す接続と候補のプロセスの間は、両方の向きの通信である。接続から、その
// レコードが指す接続先へも渡る。
func TestInfluenceLinksTheMatchedConnection(t *testing.T) {
	f := newFakeGraph(t)
	destination, process := f.node(core.NodeKindDomain, "destination"), f.node(core.NodeKindProcess, "p")
	origin, candidate := f.record(10), f.record(12, byProcess(process))
	withoutNode := f.record(14, withoutRecordNode())
	match := f.edge(core.EdgeKindCrossSourceConnectionMatch, core.RelationStateCandidate, destination, process, origin, candidate)
	f.g.matchStages = []matchStage{{origin: int32(origin)}, {origin: int32(withoutNode)}}
	f.g.edges[match].ensureBasis().matches = []candidateMatch{{candidate: int32(candidate), stage: 0}, {candidate: int32(candidate), stage: 1}}
	requireInfluence(t, f,
		"n:p>n:r0 specified_operation/candidate/single_node r[1 0] D[[12,12]] A[[10,10]]",
		"n:r0>n:p specified_operation/candidate/single_node r[0 1] D[[10,10]] A[[12,12]]",
		"n:r0>n:destination requested_destination/observed/single_node r[0] D[[10,10]] A[[10,10]]",
	)
}

// 引数に現れた名前は、プロセスから、引数が指すファイルとアドレスへ渡る。端末へは渡らない。
func TestInfluenceFollowsTheArgumentNames(t *testing.T) {
	f := newFakeGraph(t)
	process, file := f.node(core.NodeKindProcess, "p"), f.node(core.NodeKindFile, "archive")
	address, terminal := f.node(core.NodeKindIp, "address"), f.node(core.NodeKindTerminal, "terminal")
	start := f.record(10, byProcess(process))
	for _, target := range []int{file, address, terminal} {
		f.edge(core.EdgeKindArgumentNamesObject, core.RelationStateCandidate, process, target, start)
	}
	requireInfluence(t, f,
		"n:p>n:archive argument_name/candidate/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:p>n:address argument_name/candidate/single_node r[0] D[[10,10]] A[[10,10]]",
	)
}

// アカウントの管理操作の記録は、操作の主体から対象のアカウントへ渡る。ほかの記録の対象の
// アカウントへは渡らない。
func TestInfluenceFollowsTheAccountManagement(t *testing.T) {
	f := newFakeGraph(t)
	account := f.node(core.NodeKindAccount, "account")
	created := f.record(10, withFlow(core.FlowOperationAccountManagement))
	logon := f.record(20)
	f.edge(core.EdgeKindRecordTargetAccount, core.RelationStateObserved, f.g.records[created].recordNode, account, created)
	f.edge(core.EdgeKindRecordTargetAccount, core.RelationStateObserved, f.g.records[logon].recordNode, account, logon)
	requireInfluence(t, f,
		"n:r0>n:account account_management/observed/single_node r[0] D[[10,10]] A[[10,10]]",
	)
}

// ログオンの記録は、ログオンしたアカウントからログオンのレコードへ、資格情報の使用として渡る。
// D と A はログオンのレコード 1 件のタイムスタンプである。ほかの記録の対象のアカウントと、ログオンの
// 主体のアカウントからは渡らない。
func TestInfluenceFollowsTheCredentialUse(t *testing.T) {
	f := newFakeGraph(t)
	account := f.node(core.NodeKindAccount, "account")
	logon := f.record(20, withFlow(core.FlowOperationLogon))
	other := f.record(30)
	f.edge(core.EdgeKindRecordTargetAccount, core.RelationStateObserved, f.g.records[logon].recordNode, account, logon)
	f.edge(core.EdgeKindRecordTargetAccount, core.RelationStateObserved, f.g.records[other].recordNode, account, other)
	f.edge(core.EdgeKindRecordSubjectAccount, core.RelationStateObserved, f.g.records[logon].recordNode, account, logon)
	requireInfluence(t, f,
		"n:account>n:r0 credential_use/observed/single_node r[0] D[[20,20]] A[[20,20]]",
	)
}

// 表で作らないと定めた関係の種別からは、影響のエッジを作らない。
func TestInfluenceIgnoresTheRelationsWithoutInfluence(t *testing.T) {
	f := newFakeGraph(t)
	process, file := f.node(core.NodeKindProcess, "p"), f.node(core.NodeKindFile, "file")
	other, terminal := f.node(core.NodeKindFile, "other"), f.node(core.NodeKindTerminal, "terminal")
	record := f.record(10, byProcess(process))
	for _, kind := range []core.EdgeKind{
		core.EdgeKindFileContentMatch, core.EdgeKindAccountIdentityMatch,
		core.EdgeKindRanOn, core.EdgeKindTerminalAddress, core.EdgeKindTerminalAccount,
		core.EdgeKindTerminalRemoteSession, core.EdgeKindUnidentifiedSourceRemoteSession,
		core.EdgeKindRecordNamesObject, core.EdgeKindRecordSubjectAccount, core.EdgeKindRecordTargetAccount,
		core.EdgeKindReverseLookupName, core.EdgeKindHttpRequest, core.EdgeKindInboundConnectionMatch,
	} {
		edge := f.edge(kind, core.RelationStateCandidate, file, other, record)
		f.pair(edge, pairRuleAccountIdentity, record, record)
	}
	f.edge(core.EdgeKindRanOn, core.RelationStateObserved, process, terminal, record)
	requireInfluence(t, f)
}

// 置き換えの記録を挟む期間の読み込みは、区間が重なる内容のバージョンごとにつながり、区間を
// バージョンの境で切る。
func TestInfluenceSplitsAPeriodAcrossContentVersions(t *testing.T) {
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	file := f.node(core.NodeKindFile, "file")
	replaced := f.record(15, byProcess(writer), withFlow(core.FlowOperationWrite), replacing())
	read := f.record(20, byProcess(reader), withField(core.SemanticKeyEventReadBytes, "4"))
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, replaced)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file, read)
	version := "n:file@2001-02-03T04:00:15Z"
	requireInfluence(t, f,
		"n:writer>"+version+" specified_operation/observed/single_node r[0] D[[15,15]] A[[15,15]]",
		version+">n:reader specified_operation/observed/single_node r[1] D[[15,20]] A[[15,20]]",
		"n:file>n:reader specified_operation/observed/single_node r[1] D[[-,14.999999999]] A[[-,14.999999999]]",
		"n:reader>"+version+" undetermined_direction/observed/single_node r[1] D[[15,20]] A[[15,20]]",
		"n:reader>n:file undetermined_direction/observed/single_node r[1] D[[-,14.999999999]] A[[-,14.999999999]]",
	)
}

// UTC からのずれの分からないタイムスタンプの読み込みは、UTC の置き換えの記録と前後を比べられない。
// ファイルのすべての内容のバージョンにつなぎ、各バージョンの期間を、置き換えを記録した端末の時刻の
// 制約として足す。
func TestInfluenceLinksALocalReadToEveryVersionWithTheirPeriods(t *testing.T) {
	f := newFakeGraph(t)
	writer, reader := f.node(core.NodeKindProcess, "writer"), f.node(core.NodeKindProcess, "reader")
	file := f.node(core.NodeKindFile, "file")
	early := f.record(10, byProcess(writer), withFlow(core.FlowOperationWrite))
	replaced := f.record(15, byProcess(writer), withFlow(core.FlowOperationWrite), replacing())
	read := f.record(20, byProcess(reader), withFlow(core.FlowOperationRead), localClock())
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, writer, file, early, replaced)
	f.edge(core.EdgeKindFileOperation, core.RelationStateObserved, reader, file, read)
	version := "n:file@2001-02-03T04:00:15Z"
	built := requireInfluence(t, f,
		"n:writer>n:file specified_operation/observed/single_node r[0] D[[10,10]] A[[10,10]]",
		"n:writer>"+version+" specified_operation/observed/single_node r[1] D[[15,15]] A[[15,15]]",
		"n:file>n:reader specified_operation/observed/single_node r[2] D[[20,20] [-,14.999999999]] A[[20,20] [-,14.999999999]]",
		version+">n:reader specified_operation/observed/single_node r[2] D[[20,20] [15,+]] A[[20,20] [15,+]]",
	)
	for _, edge := range built.edges {
		if edge.records[0] != read {
			continue
		}
		if len(edge.exact.depart) != 2 || edge.exact.depart[0].clock == edge.exact.depart[1].clock {
			t.Errorf("the local read carries %+v, want the version period on the clock of the replacement", edge.exact.depart)
		}
	}
}
