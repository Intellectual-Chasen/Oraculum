// in-package test: 2 台の端末の記録から、ログオンの連鎖の候補の有無と根拠の種類を確かめる。
// 端末 A (host-a) が接続元、端末 B (host-b) がログオンを受けた端末である。
package pipeline

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// chainLogonXML は、host-b が記録した ip からのネットワークのログオンを返す。Logon ID は記録番号から作る。
func chainLogonXML(recordID, at, account, ip string) string {
	return securityEventXML(recordID, "4624", logonHostB, at,
		"TargetUserName", account, "TargetDomainName", "EXAMPLE", "TargetLogonId", "0xd"+recordID,
		"LogonType", "3", "IpAddress", ip, "IpPort", "50001")
}

// chainGraph は host-a と host-b の file を取り込み、192.0.2.10 を host-a (ws-a) へ、192.0.2.20 を
// host-b (ws-b) へ割り当てた観測の層を、ログオンのセッションの最長の時間 limit で組む。
// 割当の期間は host-b の file の期間であり、host-b の file の先頭に置く割当の無い接続元のログオン
// (記録番号 20) が、期間を host-a の記録の前から始める。
func chainGraph(t *testing.T, limit time.Duration, hostA, hostB string) Graph {
	t.Helper()
	lead := chainLogonXML("20", "2001-02-03T03:00:00.000Z", "user-lead", "198.51.100.9")
	result := windowsEventSessionResult(t, "<Events>\n"+hostA+"</Events>\n", "<Events>\n"+lead+hostB+"</Events>\n")
	return ObservedGraphWithLogonSessionLimit(limit)(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
		sessionAssignment(t, result, 1, "192.0.2.20", "ws-b", logonHostB, false),
	}))
}

// chainEdge は、ログオンの連鎖の 1 本を、両端の記録番号と状態と条件の種別で比べる形である。
// from は、セッションの始まりの記録番号と、それと違うときは `/` に続く終わりを決めた記録番号である。
type chainEdge struct {
	from, to   string
	state      core.RelationState
	conditions string
}

// chainEdgesOf は、グラフのログオンの連鎖を並びのまま返す。
func chainEdgesOf(t *testing.T, graph Graph) []chainEdge {
	t.Helper()
	var edges []chainEdge
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindLogonChain {
			continue
		}
		detail := edgeDetailOfKind(t, graph, core.EdgeKindLogonChain, edge.source, edge.target)
		from, evidence, sides := sessionPairsOf(t, graph, edge.source, edge.target, detail.RecordPairs)
		if !slices.Equal(edge.evidence, append(evidence, sides[1])) {
			t.Errorf("the evidence %v is not the session records %v and the logon", edge.evidence, evidence)
		}
		var conditions string
		for _, key := range conditionKeysOf(detail.RecordPairs[0]) {
			conditions += string(key) + " "
		}
		edges = append(edges, chainEdge{from, recordNameOf(t, graph, sides[1]), edge.state, conditions})
	}
	return edges
}

// sessionPairsOf は、セッションを起点に持つ関係の組を確かめ、始まりと終わりの記録番号 (違うときは
// `/` で並べる) と、組の起点の側の記録の位置と、最初の組の両側を返す。組は始まりの記録と終点、
// 終わりの記録と終点の順であり (終わりが始まりと同じときは 1 つ)、終わりの組の条件の種別は
// 始まりの組の条件の種別の先頭と同じである。
func sessionPairsOf(
	t *testing.T, graph Graph, source, target int, pairs []core.EdgeRecordPair,
) (string, []int, [2]int) {
	t.Helper()
	if len(pairs) != 1 && len(pairs) != 2 {
		t.Fatalf("the relation carries %d pairs, want 1 or 2", len(pairs))
	}
	first := pairSides(t, graph, pairs[0])
	if graph.records[first[0]].recordNode != source || graph.records[first[1]].recordNode != target {
		t.Errorf("the first pair %v does not name the session start and the logon", first)
	}
	name := recordNameOf(t, graph, first[0])
	lefts := []int{first[0]}
	if len(pairs) == 2 {
		second := pairSides(t, graph, pairs[1])
		startKeys, endKeys := conditionKeysOf(pairs[0]), conditionKeysOf(pairs[1])
		if second[1] != first[1] || len(endKeys) > len(startKeys) || !slices.Equal(endKeys, startKeys[:len(endKeys)]) {
			t.Errorf("the second pair %v does not join the session end with the logon", second)
		}
		name += "/" + recordNameOf(t, graph, second[0])
		lefts = append(lefts, second[0])
	}
	return name, lefts, first
}

// 始まりの組の条件の種別。セッションを始めた対話のログオン (種別 2) はアカウントの名前を持たず、
// 終点のログオンのアカウントと一致しない。
const (
	chainLogonLogoff = "source_terminal session_start_logon session_end_logoff " + chainInteractiveOther
	chainLogonLimit  = "source_terminal session_start_logon session_end_time_limit " + chainInteractiveOther
	// chainInteractiveOther は、アカウントが一致しない対話のログオンの区分の条件である。
	chainInteractiveOther = "session_account_different session_logon_interactive "
)

// ログオフのあるセッションは、ログオフまでのログオンの候補になる。ログオフの後のログオン、割当の
// 無い接続元のログオン、接続元がログオンを記録した端末自身であるログオン、作ったセッションの
// Logon ID を持たないログオン、時点を持たない (UTC からのずれの無い) ログオンは候補を持たない。
func TestLogonChainRunsWithinTheSessionUntilTheLogoff(t *testing.T) {
	graph := chainGraph(t, DefaultLogonSessionLimit,
		securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc1", "LogonType", "2")+
			securityEventXML("12", "4634", logonHostA, "2001-02-03T04:30:00.000Z", "TargetLogonId", "0xc1"),
		chainLogonXML("21", "2001-02-03T04:00:00.000Z", "user-b", "198.51.100.9")+
			chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10")+
			chainLogonXML("23", "2001-02-03T04:12:00.000Z", "user-b", "192.0.2.20")+
			securityEventXML("24", "4624", logonHostB, "2001-02-03T04:14:00.000Z",
				"LogonType", "3", "IpAddress", "192.0.2.10", "IpPort", "50001")+
			chainLogonXML("26", "2001-02-03 04:16:00.000", "user-b", "192.0.2.10")+
			chainLogonXML("25", "2001-02-03T04:40:00.000Z", "user-b", "192.0.2.10"))
	want := []chainEdge{{"11/12", "22", core.RelationStateCandidate, chainLogonLogoff}}
	if got := chainEdgesOf(t, graph); !slices.Equal(got, want) {
		t.Errorf("the logon chains are %v, want %v", got, want)
	}
}

// ログオフも起動の記録も無いセッションは、最後の操作から T の後までのログオンの候補になる。
// 始まりは最初の操作である。T の外のログオンは候補を持たない。
func TestLogonChainRunsUntilTheLimitAfterTheLastOperation(t *testing.T) {
	hostA := securityEventXML("11", "4672", logonHostA, "2001-02-03T04:00:00.000Z", "SubjectLogonId", "0xc2") +
		securityEventXML("12", "4688", logonHostA, "2001-02-03T04:05:00.000Z", "SubjectLogonId", "0xc2")
	hostB := chainLogonXML("21", "2001-02-03T04:00:00.000Z", "user-b", "198.51.100.9") +
		chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10") +
		chainLogonXML("23", "2001-02-03T04:20:00.000Z", "user-b", "192.0.2.10")
	want := []chainEdge{{"11/12", "22", core.RelationStateCandidate,
		"source_terminal session_start_first_operation session_end_time_limit " +
			"session_account_different session_logon_other "}}
	if got := chainEdgesOf(t, chainGraph(t, 10*time.Minute, hostA, hostB)); !slices.Equal(got, want) {
		t.Errorf("the logon chains with T of 10 minutes are %v, want %v", got, want)
	}
	if got := chainEdgesOf(t, chainGraph(t, time.Minute, hostA, hostB)); len(got) != 0 {
		t.Errorf("the logon chains with T of 1 minute are %v, want none", got)
	}
}

// 起動の記録のある端末では、ログオフの無いセッションは次の起動で終わる。起動の後のログオンは候補を
// 持たない。起動の記録が無ければ、同じログオンが T の中で候補になる。
func TestLogonChainEndsAtTheNextSystemStart(t *testing.T) {
	logon := securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc3", "LogonType", "2")
	hostB := chainLogonXML("21", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10") +
		chainLogonXML("22", "2001-02-03T04:30:00.000Z", "user-b", "192.0.2.10")
	booted := chainGraph(t, DefaultLogonSessionLimit,
		logon+securityEventXML("12", "4608", logonHostA, "2001-02-03T04:20:00.000Z"), hostB)
	want := []chainEdge{{"11/12", "21", core.RelationStateCandidate,
		"source_terminal session_start_logon session_end_system_start " + chainInteractiveOther}}
	if got := chainEdgesOf(t, booted); !slices.Equal(got, want) {
		t.Errorf("the logon chains with the system start are %v, want %v", got, want)
	}
	want = []chainEdge{
		{"11", "21", core.RelationStateCandidate, chainLogonLimit},
		{"11", "22", core.RelationStateCandidate, chainLogonLimit},
	}
	if got := chainEdgesOf(t, chainGraph(t, DefaultLogonSessionLimit, logon, hostB)); !slices.Equal(got, want) {
		t.Errorf("the logon chains without the system start are %v, want %v", got, want)
	}
}

// 4648 が要求したログオンは、要求したセッションの関係が原因を決め、ログオンの連鎖を持たない。
// 同じセッションの期間の別のログオンは、連鎖の候補になる。4648 が無ければ、同じログオンも候補になる。
func TestLogonChainSkipsTheLogonCausedByTheRequestedSession(t *testing.T) {
	logon := securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc4", "LogonType", "2")
	request := explicitRequestXML("12", "2001-02-03T04:10:00.000Z", "user-b1", "0xc4")
	hostB := chainLogonXML("21", "2001-02-03T04:10:00.500Z", "user-b1", "192.0.2.10") +
		chainLogonXML("22", "2001-02-03T04:15:00.000Z", "user-b2", "192.0.2.10")
	requested := chainGraph(t, DefaultLogonSessionLimit, logon+request, hostB)
	if got := len(edgePairsOfKind(requested, core.EdgeKindRequestedSessionLogon)); got != 1 {
		t.Fatalf("requested_session_logon = %d edges, want 1", got)
	}
	// 4648 は同じセッションの最後の操作であり、T を数え始める記録である。
	want := []chainEdge{{"11/12", "22", core.RelationStateCandidate, chainLogonLimit}}
	if got := chainEdgesOf(t, requested); !slices.Equal(got, want) {
		t.Errorf("the logon chains with the request are %v, want %v", got, want)
	}
	want = []chainEdge{
		{"11", "21", core.RelationStateCandidate, chainLogonLimit},
		{"11", "22", core.RelationStateCandidate, chainLogonLimit},
	}
	if got := chainEdgesOf(t, chainGraph(t, DefaultLogonSessionLimit, logon, hostB)); !slices.Equal(got, want) {
		t.Errorf("the logon chains without the request are %v, want %v", got, want)
	}
}

// 公開を止めた収集元の記録は、セッションにもログオンの連鎖の終点にもならない。
func TestLogonChainSkipsWithheldSources(t *testing.T) {
	result := windowsEventSessionResult(t,
		"<Events>\n"+securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z",
			"TargetLogonId", "0xc7", "LogonType", "2")+"</Events>\n",
		"<Events>\n"+chainLogonXML("20", "2001-02-03T03:00:00.000Z", "user-lead", "198.51.100.9")+
			chainLogonXML("21", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10")+"</Events>\n")
	assigned := result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-a", logonHostA, false),
	})
	if got := chainEdgesOf(t, NewObservedGraph(assigned)); len(got) != 1 {
		t.Fatalf("the logon chains of the published sources are %v, want 1", got)
	}
	for withheld := range assigned.publications {
		hidden := assigned
		hidden.publications = slices.Clone(assigned.publications)
		hidden.publications[withheld].status.PublicationState = core.PublicationStateWithheld
		graph := NewObservedGraph(hidden)
		if got := chainEdgesOf(t, graph); len(got) != 0 {
			t.Errorf("withholding the source %d leaves the logon chains %v", withheld, got)
		}
		for _, period := range graph.logonSessionPeriodsOf(hidden, DefaultLogonSessionLimit) {
			if withheld == 0 && period.logonId == "0xc7" {
				t.Errorf("withholding the terminal A leaves its session %+v", period)
			}
		}
	}
}

// ログオンの時刻にログオン中のセッションが 2 つ以上あるとき、全候補を不確定の連鎖にする。
func TestLogonChainMarksEveryCandidateUncertainWhenTwoSessionsAreOpen(t *testing.T) {
	graph := chainGraph(t, DefaultLogonSessionLimit,
		securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc5", "LogonType", "2")+
			securityEventXML("12", "4624", logonHostA, "2001-02-03T04:05:00.000Z", "TargetLogonId", "0xc6", "LogonType", "10"),
		chainLogonXML("21", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10"))
	want := []chainEdge{
		{"11", "21", core.RelationStateUncertainChain, chainLogonLimit},
		{"12", "21", core.RelationStateUncertainChain, chainLogonLimit},
	}
	if got := chainEdgesOf(t, graph); !slices.Equal(got, want) {
		t.Errorf("the logon chains are %v, want %v", got, want)
	}
}

// ログオフの記録が無いネットワークのログオン (種別 3) のセッションは、最後の操作で終わる。
// 最後の操作の後のログオンは、T の中でも、次の起動の前でも候補を持たない。操作の無いセッションは
// 始まりで終わる。
func TestLogonChainEndsANetworkLogonAtItsLastOperation(t *testing.T) {
	hostA := securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z",
		"TargetUserName", "user-n", "TargetLogonId", "0xe1", "LogonType", "3") +
		securityEventXML("12", "4672", logonHostA, "2001-02-03T04:05:00.000Z", "SubjectLogonId", "0xe1") +
		securityEventXML("13", "4624", logonHostA, "2001-02-03T04:06:00.000Z",
			"TargetUserName", "user-n", "TargetLogonId", "0xe2", "LogonType", "3")
	hostB := chainLogonXML("21", "2001-02-03T04:03:00.000Z", "user-b", "192.0.2.10") +
		chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10") +
		// 割当の期間 (host-b の file の期間) を、host-a の起動の記録の後まで延ばす。
		chainLogonXML("29", "2001-02-03T04:40:00.000Z", "user-tail", "198.51.100.9")
	want := []chainEdge{{"11/12", "21", core.RelationStateCandidate,
		"source_terminal session_start_logon session_end_last_operation " +
			"session_account_different session_logon_network "}}
	boot := securityEventXML("14", "4608", logonHostA, "2001-02-03T04:30:00.000Z")
	for name, records := range map[string]string{"without the system start": hostA, "with the system start": hostA + boot} {
		if got := chainEdgesOf(t, chainGraph(t, DefaultLogonSessionLimit, records, hostB)); !slices.Equal(got, want) {
			t.Errorf("%s: the logon chains are %v, want %v", name, got, want)
		}
	}
}

// 匿名のログオン (SID S-1-5-7、または名前 ANONYMOUS LOGON) と、画面の描画とフォントのドライバの
// 仮想アカウント (SID S-1-5-90-*、S-1-5-96-*) のセッションは、期間がログオンの時刻を含んでも候補に
// ならない。残る候補が 1 つのとき、状態は candidate である。
func TestLogonChainSkipsAnonymousSessions(t *testing.T) {
	hostA := securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z",
		"TargetUserSid", "S-1-5-7", "TargetUserName", "-", "TargetLogonId", "0xe3", "LogonType", "2") +
		securityEventXML("12", "4624", logonHostA, "2001-02-03T04:01:00.000Z",
			"TargetUserName", "ANONYMOUS LOGON", "TargetLogonId", "0xe4", "LogonType", "2") +
		securityEventXML("14", "4624", logonHostA, "2001-02-03T04:01:10.000Z",
			"TargetUserSid", "S-1-5-90-0-7", "TargetUserName", "DWM-7", "TargetLogonId", "0xe6", "LogonType", "2") +
		securityEventXML("15", "4624", logonHostA, "2001-02-03T04:01:20.000Z",
			"TargetUserSid", "S-1-5-96-0-7", "TargetUserName", "UMFD-7", "TargetLogonId", "0xe7", "LogonType", "2") +
		securityEventXML("13", "4624", logonHostA, "2001-02-03T04:02:00.000Z",
			"TargetUserName", "user-c", "TargetLogonId", "0xe5", "LogonType", "2")
	want := []chainEdge{{"13", "21", core.RelationStateCandidate, chainLogonLimit}}
	got := chainEdgesOf(t, chainGraph(t, DefaultLogonSessionLimit, hostA,
		chainLogonXML("21", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10")))
	if !slices.Equal(got, want) {
		t.Errorf("the logon chains are %v, want %v", got, want)
	}
}

// 候補は、アカウントが一致するものを上に、次にログオンの種別 (対話・画面の遠隔操作、その他、
// ネットワーク) の区分で並べる。組は区分の条件と、候補の数と、並びでより上の区分に入る候補の数を
// 持つ。
// 名前が一致しなくても SID が一致すれば一致であり、種別 9 のログオンは他の端末への接続に使う
// アカウントも比べる。
func TestLogonChainOrdersTheCandidatesByAccountAndLogonType(t *testing.T) {
	graph := tieredChainGraph(t, "")
	type ordered struct {
		from, conditions     string
		candidates, previous int
	}
	var got []ordered
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindLogonChain {
			continue
		}
		pair := edgeDetailOfKind(t, graph, core.EdgeKindLogonChain, edge.source, edge.target).RecordPairs[0]
		if pair.CandidateTally == nil {
			t.Fatalf("the pair from %s carries no candidate tally", recordNameOf(t, graph, pairSides(t, graph, pair)[0]))
		}
		keys := conditionKeysOf(pair)
		// ログオンの種別の条件は、終点の側に終点のログオン (種別 3) の種別を持つ。
		if right := comparableValuesOf(pair.Conditions[4].RightValue); !slices.Equal(right, []string{"3"}) {
			t.Errorf("the logon type condition carries %v on the target side, want the type 3", right)
		}
		got = append(got, ordered{recordNameOf(t, graph, pairSides(t, graph, pair)[0]),
			string(keys[3]) + " " + string(keys[4]), pair.CandidateTally.CandidateCount,
			pair.CandidateTally.PrecedingCandidateCount})
	}
	// 12 (種別 12) と 15 (種別 13) はキャッシュした資格情報の遠隔操作とロック解除であり、対話の区分に入る。
	want := []ordered{
		{"11", "session_account_match session_logon_network", 5, 2},
		{"12", "session_account_different session_logon_interactive", 5, 3},
		{"13", "session_account_match session_logon_interactive", 5, 0},
		{"14", "session_account_match session_logon_other", 5, 1},
		{"15", "session_account_different session_logon_interactive", 5, 3},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the candidates are %v, want %v", got, want)
	}
}

// chainTier は、応答のエッジ 1 本の、両端のレコードの記録番号と区分である。
type chainTier struct {
	from, to string
	tier     int
	match    core.EdgePairConditionKey
}

// chainTiersOf は、応答のログオンの連鎖のエッジを並びのまま返す。
func chainTiersOf(t *testing.T, graph Graph, subgraph Subgraph) []chainTier {
	t.Helper()
	var tiers []chainTier
	for _, edge := range subgraph.Edges {
		if edge.Kind != core.EdgeKindLogonChain {
			if edge.CandidateTier != nil {
				t.Errorf("the %s edge carries the candidate tier %+v", edge.Kind, edge.CandidateTier)
			}
			continue
		}
		if edge.CandidateTier == nil || len(edge.CandidateTier.Conditions) != 2 {
			t.Fatalf("the logon chain edge carries the candidate tier %+v, want the tier and 2 conditions", edge.CandidateTier)
		}
		tiers = append(tiers, chainTier{
			recordNodeNameOf(t, graph, edge.SourceNodeId), recordNodeNameOf(t, graph, edge.TargetNodeId),
			edge.CandidateTier.Tier, edge.CandidateTier.Conditions[0],
		})
	}
	return tiers
}

// recordNodeNameOf は、レコードのノードの識別子 id のレコードの記録番号を返す。
func recordNodeNameOf(t *testing.T, graph Graph, id string) string {
	t.Helper()
	for at, record := range graph.records {
		if record.hasRecordNode && graph.nodes[record.recordNode].id == id {
			return recordNameOf(t, graph, at)
		}
	}
	t.Fatalf("no record has the node %s", id)
	return ""
}

// ログオンの側から開いた応答は、候補を区分の番号の順に並べ、区分の番号と条件を持つ。同じ区分の候補は
// 観測の層の順を保つ。
func TestLogonChainResponseListsTheLogonCandidatesByTier(t *testing.T) {
	graph := tieredChainGraph(t, "")
	logon := graph.nodes[graph.records[recordOfEvent(t, graph, "21")].recordNode].id
	got := chainTiersOf(t, graph, graph.Query(GraphQuery{NodeIds: []string{logon}, Depth: 1}))
	match, different := core.EdgePairConditionSessionAccountMatch, core.EdgePairConditionSessionAccountDifferent
	want := []chainTier{
		{"13", "21", 1, match}, {"14", "21", 2, match}, {"11", "21", 3, match},
		{"12", "21", 4, different}, {"15", "21", 4, different},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the candidates are listed as %v, want %v", got, want)
	}
}

// 接続元のセッションの側から開いた応答も、ログオンを区分の番号の順に並べる。観測の層では、
// アカウントが一致しないログオン (22) が一致するログオン (21) より先にある。
func TestLogonChainResponseListsTheSessionCandidatesByTier(t *testing.T) {
	graph := tieredChainGraph(t, securityEventXML("22", "4624", logonHostB, "2001-02-03T04:05:00.000Z",
		"TargetUserName", "user-c", "TargetDomainName", "EXAMPLE", "TargetLogonId", "0xd22", "LogonType", "3",
		"IpAddress", "192.0.2.10", "IpPort", "50002"))
	session := graph.nodes[graph.records[recordOfEvent(t, graph, "11")].recordNode].id
	subgraph := graph.Query(GraphQuery{NodeIds: []string{session}, Depth: 1})
	got := chainTiersOf(t, graph, subgraph)
	want := []chainTier{
		{"11", "21", 3, core.EdgePairConditionSessionAccountMatch},
		{"11", "22", 6, core.EdgePairConditionSessionAccountDifferent},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the candidates are listed as %v, want %v", got, want)
	}
	// 区分を持たないエッジは、区分を持つエッジより先に並ぶ。
	if first := slices.IndexFunc(subgraph.Edges, func(edge core.GraphEdge) bool { return edge.CandidateTier != nil }); first >= 0 &&
		slices.ContainsFunc(subgraph.Edges[first:], func(edge core.GraphEdge) bool { return edge.CandidateTier == nil }) {
		t.Error("an edge without a tier follows a tiered edge")
	}
}

// tieredChainGraph は、host-a の 5 つのセッション (11〜15) と、host-b の host-a からのログオン (21) を
// 組む。extraHostB は host-b の file の先頭に足すイベントである。
func tieredChainGraph(t *testing.T, extraHostB string) Graph {
	t.Helper()
	logon := func(recordID, at, logonType string, account ...string) string {
		data := append([]string{"TargetLogonId", "0xf" + recordID, "LogonType", logonType}, account...)
		return securityEventXML(recordID, "4624", logonHostA, at, data...)
	}
	operation := securityEventXML("19", "4672", logonHostA, "2001-02-03T04:30:00.000Z", "SubjectLogonId", "0xf11")
	hostA := logon("11", "2001-02-03T04:00:00.000Z", "3", "TargetUserName", "USER-B") + operation +
		logon("12", "2001-02-03T04:01:00.000Z", "12", "TargetUserName", "user-x") +
		logon("13", "2001-02-03T04:02:00.000Z", "10", "TargetUserName", "user-y", "TargetUserSid", "S-1-5-21-1-2-3-1001") +
		logon("14", "2001-02-03T04:03:00.000Z", "9", "TargetUserName", "user-z", "TargetOutboundUserName", "user-b") +
		logon("15", "2001-02-03T04:04:00.000Z", "13", "TargetUserName", "user-w")
	hostB := extraHostB + securityEventXML("21", "4624", logonHostB, "2001-02-03T04:10:00.000Z",
		"TargetUserName", "user-b", "TargetUserSid", "S-1-5-21-1-2-3-1001", "TargetDomainName", "EXAMPLE",
		"TargetLogonId", "0xd21", "LogonType", "3", "IpAddress", "192.0.2.10", "IpPort", "50001") +
		// 割当の期間 (host-b の file の期間) を、host-a の最後の操作の後まで延ばす。
		chainLogonXML("29", "2001-02-03T04:40:00.000Z", "user-tail", "198.51.100.9")
	return chainGraph(t, DefaultLogonSessionLimit, hostA, hostB)
}

// 候補の数が uint16 の上限を超えるとき、候補の数は上限で止まり、上の区分に入る候補の数は候補の数
// より小さい値に止まる。
func TestLogonChainTallySaturatesBelowTheCandidateCount(t *testing.T) {
	candidates := make([]chainCandidate, 70000)
	for index := range candidates {
		candidates[index] = chainCandidate{accountMatch: true, class: sessionLogonInteractive}
	}
	last := chainCandidate{class: sessionLogonNetwork}
	candidates[len(candidates)-1] = last
	if got := tallyOf(last, candidates); got.candidates != math.MaxUint16 || got.preceding != math.MaxUint16-1 {
		t.Errorf("the tally is %+v, want %d candidates with %d preceding", got, math.MaxUint16, math.MaxUint16-1)
	}
}

// ログオフの記録が始まりの時刻を書いたセッションも、その記録のログオンの種別で区分する。
func TestLogonChainClassifiesTheSessionStartedByALogoffRecord(t *testing.T) {
	logout := `02/01/2000 12:00:00.000 +0900 loc=ja-JP type=ITM2 sn=501 lv=5 evt=session subEvt=logout os=Win ` +
		`com="HOST01" domain="AD" tmid=00000000-0000-4000-8000-000000000001 csid=S-1-5-21-1-2-3 ` +
		`usr="user01" sessionID=2 logonType="RemoteInteractive(10)" sTime="02/01/2000 09:00:00.000"` + "\n"
	result := sessionSourcesResult(t, sessionSource{name: "client.log", format: MarkIIFormatKey, document: logout})
	graph := NewGraph(result, AllMatchConditions())
	periods := graph.logonSessionPeriodsOf(result, time.Hour)
	if len(periods) != 1 || periods[0].startKind != sessionStartLogoffRecord {
		t.Fatalf("the periods are %+v, want one session started by the logoff record", periods)
	}
	if candidate, kept := graph.chainCandidateOf(periods[0], periods[0].startAt); !kept ||
		candidate.class != sessionLogonInteractive {
		t.Errorf("the session is classified %v (kept %v), want interactive", candidate.class, kept)
	}
}
