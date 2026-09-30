// in-package test: 端末の割当で端末を決めた Windows イベントログの通信のレコードを、Proxy の
// 要求の関連付けの候補にする経路を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// proxyCandidateResult は、端末 ws.example.test の通信の許可と Proxy の要求を取り込み、割当を
// 足した結果を返す。
//
// 収集元 0 は ws.example.test の file である。04:10:02 と 05:10:02 に、プロセス番号 16 が
// 192.0.2.10 から Proxy (192.0.2.80:8080) へ接続した。04:10:02 には、プロセス番号 20 が
// Proxy を通らずに 203.0.113.50:443 へ接続した。収集元 1 と 2 は、割当の期間を読み取る
// file である。割当 N は収集元 1 の期間 (04:00 から 04:30) に 192.0.2.10 とホスト名
// ws.example.test を端末 ws-1 に割り当て、割当 L は収集元 2 の期間 (05:00 から 05:30) に
// 192.0.2.10 を ws-1 に割り当てる。L はホスト名を記録しないため、05:10:02 の接続のレコードは
// ws-1 に置かれない。収集元 3 は Proxy のログであり、利用者が端末 proxy-1 と IP 192.0.2.80 を
// 付けた。
func proxyCandidateResult(t *testing.T) ImportResult {
	return proxyCandidateResultWithProxyIp(t, "192.0.2.80")
}

// proxyCandidateResultWithProxyIp は、Proxy の収集元の割当の IP を proxyIp にした
// proxyCandidateResult を返す。空の文字列は IP を持たない割当である。
func proxyCandidateResultWithProxyIp(t *testing.T, proxyIp string) ImportResult {
	t.Helper()
	const host = "ws.example.test"
	client := "<Events>\n" +
		processCreationXML(host, "2001-02-03T04:00:00Z", "800", "0x10", "0x4",
			`C:\Example\tool.exe`, "tool.exe /fetch") +
		processCreationXML(host, "2001-02-03T04:00:00Z", "805", "0x14", "0x4",
			`C:\Example\direct.exe`, "direct.exe") +
		connectionXML(host, "2001-02-03T04:10:02Z", "801", "16", "192.0.2.10", "49152",
			"192.0.2.80", "8080", true) +
		connectionXML(host, "2001-02-03T04:10:02Z", "806", "20", "192.0.2.10", "49153",
			"203.0.113.50", "443", true) +
		// 接続先だけを記録した資格情報の要求は、同じ秒でも候補にならない。
		explicitCredentialXML(host, "2001-02-03T04:10:02Z", "804", "0x10", "192.0.2.80", "8080") +
		connectionXML(host, "2001-02-03T05:10:02Z", "802", "16", "192.0.2.10", "49160",
			"192.0.2.80", "8080", true) +
		processCreationXML(host, "2001-02-03T05:30:00Z", "803", "0x11", "0x4",
			`C:\Example\other.exe`, "other.exe") +
		"</Events>\n"
	period := func(from, to string) string {
		return "<Events>\n" +
			processCreationXML("period.example.test", from, "900", "0x20", "0x4", `C:\Example\p.exe`, "p.exe") +
			processCreationXML("period.example.test", to, "901", "0x21", "0x4", `C:\Example\p.exe`, "p.exe") +
			"</Events>\n"
	}
	proxy := `192.0.2.10 - - [03/Feb/2001:04:10:02 +0000] "GET http://example.test/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:04:20:00 +0000] "GET http://example.test/c HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:05:10:02 +0000] "GET http://example.test/b HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n"
	result := sessionSourcesResult(t,
		sessionSource{name: "ws.xml", format: WindowsEventXMLFormatKey, document: client},
		sessionSource{name: "early.xml", format: WindowsEventXMLFormatKey,
			document: period("2001-02-03T04:00:00Z", "2001-02-03T04:30:00Z")},
		sessionSource{name: "late.xml", format: WindowsEventXMLFormatKey,
			document: period("2001-02-03T05:00:00Z", "2001-02-03T05:30:00Z")},
		sessionSource{name: "access.log", format: SquidFormatKey, document: proxy})
	return result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 1, "192.0.2.10", "ws-1", host, false),
		sessionAssignment(t, result, 2, "192.0.2.10", "ws-1", "", false),
		sessionAssignment(t, result, 3, proxyIp, "proxy-1", "", true),
	})
}

// proxyOriginOutcome は、Proxy のログの行 line の起点の分類を返す。
func proxyOriginOutcome(t *testing.T, graph Graph, result ImportResult, line int) core.RelationDerivationOutcome {
	t.Helper()
	record := result.publications[3].records[line]
	key, _ := core.NewRecordNodeKey(record.Locator)
	derivation, found := graph.RelationDerivationOf(nodeIdOf(key))
	if !found {
		t.Fatalf("the proxy line %d has no record node", line)
	}
	return derivation.Outcome
}

// **割当で端末を決めた通信のレコードが候補になる。** 端末の外部識別子の欄を持たない
// Windows のレコードも、置いた端末の鍵で起点の端末と比べる。Proxy と端末が記録した接続先は
// 別の値であり、端末が記録した接続先を Proxy の収集元の端末の IP と比べ、時刻で絞る。
// Proxy を通らない接続は候補にならない。端末を付けた Proxy の行も起点に残る。
func TestProxyRequestMatchesTheConnectionOfTheAssignedTerminal(t *testing.T) {
	result := proxyCandidateResult(t)
	graph := NewGraph(result, AllMatchConditions())
	var edges []graphEdge
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindCrossSourceConnectionMatch {
			edges = append(edges, edge)
		}
	}
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d candidate edges, want 1", len(edges))
	}
	edge := edges[0]
	source := nodeWithIdentity(t, graph, core.NodeKindDomain, "example.test")
	// 番号 16 (0x10) の区間は、04:00:00 の作成から始まる。
	target := intervalProcessAt(t, graph, "16", "2001-02-03T04:00:00")
	if edge.source != source || edge.target != target {
		t.Errorf("the candidate runs from %+v to %+v, want the requested host to the process 16",
			graph.nodes[edge.source].key, graph.nodes[edge.target].key)
	}
	for line, want := range []core.RelationDerivationOutcome{
		core.RelationDerivationMatched,
		core.RelationDerivationNoCandidateInWindow,
		// 05:10:02 の接続は割当の期間の外で、ws-1 に置かれない。
		core.RelationDerivationNoCandidateInWindow,
	} {
		if got := proxyOriginOutcome(t, graph, result, line); got != want {
			t.Errorf("the proxy line %d is %q, want %q", line, got, want)
		}
	}

	detail, found := graph.EdgeDetail(edge.id, EdgeEvidenceFilter{})
	if !found || detail.MatchCount != 1 {
		t.Fatalf("the candidate carries %d matches, want 1", detail.MatchCount)
	}
	match, err := detail.MatchTable.Match(0)
	if err != nil {
		t.Fatal(err)
	}
	// 起点の Proxy の行は、Proxy が記録した要求処理の結果を持つ。
	origin := detail.MatchTable.MatchRecords[detail.MatchTable.MatchStages[0].Origin]
	if origin.ProxyStatus == nil || origin.ProxyStatus.Text == nil {
		t.Fatal("the proxy origin carries no request status")
	}
	if raw, _ := origin.ProxyStatus.Text.RawTextValue(); raw != "TCP_MISS:DIRECT" {
		t.Errorf("the proxy origin carries the status %q, want TCP_MISS:DIRECT", raw)
	}
	if candidate := detail.MatchTable.MatchRecords[detail.MatchTable.Matches[0].Candidate]; candidate.ProxyStatus != nil {
		t.Errorf("the Windows candidate carries the proxy status %+v", candidate.ProxyStatus)
	}
	if len(match.StageTallies) != 2 || match.StageTallies[0].StageKey != core.StageKeyClockIndependent ||
		match.StageTallies[0].MemberCount != 1 || match.StageTallies[1].MemberCount != 1 {
		t.Errorf("the stage tallies are %+v, want one candidate in both stages", match.StageTallies)
	}
	uses := map[core.ConditionKey]core.ConditionUse{}
	for _, condition := range match.Conditions {
		uses[condition.ConditionKey] = condition.Use
		if condition.ConditionKey == core.ConditionKeyTerminalIpAssignment {
			if len(condition.RightValue) != 1 || condition.RightValue[0].Name != candidateTerminalFieldName {
				t.Fatalf("the terminal condition compares %+v, want the placed terminal", condition.RightValue)
			}
			if value, _ := condition.RightValue[0].Text.ComparableValue(); value != "ws-1" {
				t.Errorf("the terminal condition compares %q, want ws-1", value)
			}
		}
	}
	for key, want := range map[core.ConditionKey]core.ConditionUse{
		core.ConditionKeyTerminalIpAssignment: core.ConditionUseUsed,
		core.ConditionKeySecondOfTime:         core.ConditionUseUsed,
		core.ConditionKeyDestinationIp:        core.ConditionUseNoComparableCounterpart,
		core.ConditionKeyDestinationPort:      core.ConditionUseNotUsed,
	} {
		if uses[key] != want {
			t.Errorf("the condition %q is %q, want %q", key, uses[key], want)
		}
	}
	// Proxy のアドレスで候補を絞ったことを、関連付けの前提が持つ。
	if !slices.ContainsFunc(match.Assumptions, func(assumption core.MatchAssumption) bool {
		return assumption.AssumptionKey == core.AssumptionKeyCounterpartConnectedToProxy &&
			strings.Contains(assumption.Statement, "192.0.2.80")
	}) {
		t.Errorf("the assumptions are %+v, want the proxy address 192.0.2.80", match.Assumptions)
	}
	// 時刻の比較は、候補の母集合を組むときの前提に依拠しない。
	if slices.ContainsFunc(match.TimeComparison.Assumptions, func(assumption core.MatchAssumption) bool {
		return assumption.AssumptionKey == core.AssumptionKeyCounterpartConnectedToProxy
	}) {
		t.Errorf("the time comparison carries the proxy assumption: %+v", match.TimeComparison.Assumptions)
	}
}

// **Proxy の収集元に端末の IP が無い起点は、候補を Proxy への接続に限れない。** その理由を
// 起点の分類に出す。接続先 IP の条件を外した選択では、Proxy のアドレスを求めない。
func TestProxyRequestNeedsTheAddressOfTheProxy(t *testing.T) {
	result := proxyCandidateResultWithProxyIp(t, "")
	graph := NewGraph(result, AllMatchConditions())
	if got := proxyOriginOutcome(t, graph, result, 0); got != core.RelationDerivationProxyAddressUnknown {
		t.Errorf("the proxy line 0 is %q, want %q", got, core.RelationDerivationProxyAddressUnknown)
	}
	withoutDestination := NewGraph(result, MatchConditionSelection{Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyTerminalIpAssignment}, {ConditionKey: core.ConditionKeySecondOfTime},
	}})
	if got := proxyOriginOutcome(t, withoutDestination, result, 0); got != core.RelationDerivationMatched {
		t.Errorf("the proxy line 0 without the destination condition is %q, want matched", got)
	}
}

// **同じ端末を指す同じ IP の割当が 2 件以上あっても、起点の端末を決める。** 収集元ごとに
// 同じ端末と IP を指定した起動は、期間の重なる同じ内容の割当を収集元の数だけ持つ。
func TestProxyRequestResolvesTheTerminalOfRepeatedAssignments(t *testing.T) {
	result := proxyCandidateResult(t)
	repeated := sessionAssignment(t, result, 0, "192.0.2.10", "ws-1", "ws.example.test", false)
	result = result.WithAnalystTerminalAssignments(
		append(slices.Clone(result.analystAssignments), repeated))
	graph := NewGraph(result, AllMatchConditions())
	if got := proxyOriginOutcome(t, graph, result, 0); got != core.RelationDerivationMatched {
		t.Fatalf("the proxy line 0 is %q, want matched", got)
	}
	// 起点の端末の由来は、確定に使った 2 件の割当を挙げる。
	checked := false
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindCrossSourceConnectionMatch {
			continue
		}
		detail, _ := graph.EdgeDetail(edge.id, EdgeEvidenceFilter{})
		match, err := detail.MatchTable.Match(0)
		if err != nil {
			t.Fatal(err)
		}
		for _, condition := range match.Conditions {
			if condition.ConditionKey != core.ConditionKeyTerminalIpAssignment {
				continue
			}
			derivation, _ := condition.LeftValue[0].Text.DerivationValue()
			requireDerivationItems(t, derivation, "sourceId=source-ws.xml", "sourceId=source-early.xml")
			checked = true
		}
	}
	if !checked {
		t.Fatal("no candidate edge carries the terminal condition")
	}
}

// 端末だけで絞る候補の組は、時刻を比べない選択では関連付けない。端末の全部の接続を候補に
// 挙げることになる。
func TestProxyRequestNeedsTheTimeConditionForTheTerminalOnlyCandidates(t *testing.T) {
	result := proxyCandidateResult(t)
	graph := NewGraph(result, MatchConditionSelection{Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyTerminalIpAssignment},
	}})
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindCrossSourceConnectionMatch {
			t.Fatalf("the selection without the time derives the candidate %s", edge.id)
		}
	}
	if got := proxyOriginOutcome(t, graph, result, 0); got != core.RelationDerivationNoComparedCondition {
		t.Errorf("the proxy line is %q, want no_compared_condition", got)
	}
}

// markii 形式と Windows イベントログの候補を同じ案件に取り込んでも、条件の宣言が食い違う組を
// 作らない。候補は宣言ごとの組に分かれ、markii 形式の関連付けは Windows の候補が無いときと同じである。
func TestWindowsCandidatesLeaveTheMarkIIMatchesUnchanged(t *testing.T) {
	read := func(name string) string { return graphSourceText(t, name) }
	withoutWindows := caseImport(t,
		markIISource("endpoint.log", read("graph-markii.log"), nil),
		caseSource{name: "proxy.log", format: string(SquidFormatKey), content: read("graph-squid.log")})
	client, _ := connectionLogonDocuments()
	withWindows := caseImport(t,
		markIISource("endpoint.log", read("graph-markii.log"), nil),
		caseSource{name: "proxy.log", format: string(SquidFormatKey), content: read("graph-squid.log")},
		caseSource{name: "ws.xml", format: string(WindowsEventXMLFormatKey), content: client})
	before := NewGraph(withoutWindows, AllMatchConditions())
	after := NewGraph(withWindows, AllMatchConditions())
	if after.SourceDeclarationProblem() != nil {
		t.Fatalf("the sources conflict: %v", after.SourceDeclarationProblem())
	}
	if groups := after.candidateSideGroups(withWindows); len(groups) != 2 {
		t.Errorf("the candidates fall into %d groups, want the markii and the Windows groups", len(groups))
	}
	matchIds := func(graph Graph) []string {
		var ids []string
		for _, edge := range graph.edges {
			if edge.kind == core.EdgeKindCrossSourceConnectionMatch {
				ids = append(ids, edge.id)
			}
		}
		return ids
	}
	if got, want := matchIds(after), matchIds(before); len(want) == 0 || len(got) != len(want) {
		t.Errorf("the graph with the Windows source carries %d candidate edges, want %d", len(got), len(want))
	}
	// **起点の分類は、Windows の収集元を足しても、取り込みの順を入れ替えても変わらない。**
	windowsFirst := caseImport(t,
		caseSource{name: "ws.xml", format: string(WindowsEventXMLFormatKey), content: client},
		markIISource("endpoint.log", read("graph-markii.log"), nil),
		caseSource{name: "proxy.log", format: string(SquidFormatKey), content: read("graph-squid.log")})
	for _, selection := range []MatchConditionSelection{AllMatchConditions(), {Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyTerminalIpAssignment}, {ConditionKey: core.ConditionKeyDestinationIp},
	}}} {
		want := NewGraph(withoutWindows, selection).CandidateOriginCounts()
		for name, result := range map[string]ImportResult{"windows last": withWindows, "windows first": windowsFirst} {
			if got := NewGraph(result, selection).CandidateOriginCounts(); !slices.Equal(got, want) {
				t.Errorf("%s with %+v: the origin counts are %v, want %v", name, selection.Conditions, got, want)
			}
		}
	}
}
