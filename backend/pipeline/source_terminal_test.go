// in-package test: 収集元の端末をグラフへ置く条件を、組んだグラフのノードとエッジで確かめる。
package pipeline

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// auditd の収集元。host は example.test 系である。
//
// 1 事象が、プロセス番号、実行ファイルの path (PATH の item=0)、ログイン名 (AUID) を持つ。
const accountedAuditdSource = "type=SYSCALL msg=audit(1000000000.100:1): arch=c000003e syscall=59 success=yes ppid=2000 pid=2001 auid=4001 comm=\"tool\" exe=\"/usr/bin/tool\" key=\"exec_log\"" +
	"\x1dARCH=x86_64 SYSCALL=execve AUID=\"operator\" UID=\"root\"\n" +
	"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"tool\"\n" +
	"type=PATH msg=audit(1000000000.100:1): item=0 name=\"/usr/bin/tool\" nametype=NORMAL\n"

// accountedPath と accountedLogin は accountedAuditdSource の事象が指す値である。
const (
	accountedPath  = "/usr/bin/tool"
	accountedLogin = "operator"
)

// 端末を指定していない収集元では、名前不明の端末がプロセスとファイルとアカウントをつなぐ。
func TestUnknownTerminalConnectsTheObjectsOfASourceWithoutATerminal(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	graph := NewGraph(result, AllMatchConditions())
	sha := result.publications[0].status.Scope.SourceContentSha256

	terminalKey, _ := core.RecordingTerminalNodeKey(sha)
	terminalAt := requireNodeAt(t, graph, terminalKey)
	label := graph.nodes[terminalAt].label
	if label.ValueState != core.ValueStateDerived || label.Normalized == nil ||
		*label.Normalized != "linux-host.log を記録した端末 (名前不明)" {
		t.Errorf("the terminal label is %+v, want the derived name of the unknown terminal", label)
	}

	assertTerminalScopedObjects(t, graph, terminalKey)
}

// 端末の外部識別子を指定すると、名前不明の端末が指定した端末に置き換わる。
func TestSpecifiedTerminalReplacesTheUnknownTerminal(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{analystAssignmentFor(t, result)}), AllMatchConditions())

	unknownKey, _ := core.RecordingTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256)
	if _, present := graph.nodeAt[nodeIdOf(unknownKey)]; present {
		t.Error("the graph keeps the unknown terminal after the terminal was specified")
	}
	specifiedKey, _ := core.TerminalNodeKey(lineageTerminalId)
	requireNodeAt(t, graph, specifiedKey)
	assertTerminalScopedObjects(t, graph, specifiedKey)
}

// 端末の外部識別子を省いた指定は、収集元を記録した端末のノードに表示名と IP を与える。
func TestSpecifiedTerminalWithoutIdentifierNamesTheRecordingTerminal(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	assignment := analystAssignmentFor(t, result)
	assignment.TerminalId = ""
	assignment.Origin = core.TerminalAssignmentOriginImportSpecified
	assignment.Derivation, assignment.Author, assignment.BasisRecordRefs = "", "", nil
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the import specification: %v", err)
	}
	result.importAssignments = []core.TerminalAssignment{assignment}
	graph := NewGraph(result, AllMatchConditions())

	terminalKey, _ := core.RecordingTerminalNodeKey(assignment.SourceContentSha256)
	terminalAt := requireNodeAt(t, graph, terminalKey)
	label := graph.nodes[terminalAt].label
	if label.Normalized == nil || *label.Normalized != lineageTerminalHostname {
		t.Errorf("the terminal label is %+v, want the specified name %q", label, lineageTerminalHostname)
	}
	ipKey := core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: lineageClientIp}}}
	ipAt := requireNodeAt(t, graph, ipKey)
	if !hasEdge(graph, core.EdgeKindTerminalAddress, terminalAt, ipAt) {
		t.Error("the specified terminal has no terminal_address edge to the specified IP")
	}
	assertTerminalScopedObjects(t, graph, terminalKey)
}

// unscopedAuditdEvent は、プロセスもファイルもログイン名も指さない事象である。
// 端末の範囲の対象を指さないため、名前不明の端末のノードを指さない。
const unscopedAuditdEvent = "type=CONFIG_CHANGE msg=audit(1000000100.000:2): auid=4001 ses=1 op=add_rule key=\"exec_log\" list=4 res=1\n"

// 端末のノードはどれも、端末で絞ると、その端末に置いた収集元のレコードだけを残す。名前不明の
// 端末、分析者の割当の端末、端末の外部識別子を省いた取り込みの指定の端末を確かめる。
// 端末の範囲の対象を指さないレコードも残り、別の収集元の名前不明の端末のレコードは残らない。
// 絞ったレコードの時系列の行は、レコードが名乗った端末を持つ。
func TestEveryTerminalNodeFiltersTheRecordsPlacedOnIt(t *testing.T) {
	parser := NewTestParser(AuditdFormatKeyText, nil)
	first := scanIndexSource(t, parser, "linux-host.log", AuditdFormatKeyText,
		accountedAuditdSource+unscopedAuditdEvent)
	second := scanIndexSource(t, parser, "other-host.log", AuditdFormatKeyText,
		strings.ReplaceAll(accountedAuditdSource, "pid=2001", "pid=2002"))
	result, err := newImportResult([]scannedSource{first, second},
		[]core.ImportStatus{settleStatus(t, first, "auditd"), settleStatus(t, second, "auditd-other")},
		"run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	recordsOf := make(map[string]int, len(result.publications))
	for _, publication := range result.publications {
		recordsOf[publication.status.SourceId] = len(publication.records)
	}
	if got := len(result.publications[0].records); got != 2 {
		t.Fatalf("the first source holds %d records, want the process event and the unscoped event", got)
	}
	unscoped := result.publications[0].records[1].Locator
	specified := result
	assignment := analystAssignmentFor(t, result)
	assignment.TerminalId = ""
	assignment.Origin = core.TerminalAssignmentOriginImportSpecified
	assignment.Derivation, assignment.Author, assignment.BasisRecordRefs = "", "", nil
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the import specification: %v", err)
	}
	specified.importAssignments = []core.TerminalAssignment{assignment}
	// namesNoTerminal は、端末の範囲の対象を指さないレコードが端末のノードを指さないかである。
	// 利用者が指定した端末は収集元の全レコードが指す。
	for _, testCase := range []struct {
		name            string
		result          ImportResult
		namesNoTerminal bool
	}{
		{"the unknown terminal", result, true},
		{"an analyst assignment", result.WithAnalystTerminalAssignments(
			[]core.TerminalAssignment{analystAssignmentFor(t, result)}), false},
		{"an import specification without an identifier", specified, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graph := NewGraph(testCase.result, AllMatchConditions())
			unscopedAt, indexed := graph.recordAtLocator(unscoped)
			if !indexed {
				t.Fatal("the graph holds no unscoped record")
			}
			keptUnscoped := false
			terminals := 0
			for _, node := range graph.nodes {
				if node.key.Kind != core.NodeKindTerminal {
					continue
				}
				terminals++
				if testCase.namesNoTerminal && slices.Contains(node.evidence, unscopedAt) {
					t.Errorf("the unscoped record names the terminal %q", node.id)
				}
				timeline := graph.Timeline(TimelineQuery{RecordFilter: RecordFilter{Terminal: node.id}})
				if len(timeline.Entries) == 0 {
					t.Fatalf("filtering by the terminal %q keeps no record", node.id)
				}
				sourceId := timeline.Entries[0].RecordRef.SourceId
				if len(timeline.Entries) != recordsOf[sourceId] {
					t.Errorf("filtering by the terminal %q keeps %d records, want the %d records of %s",
						node.id, len(timeline.Entries), recordsOf[sourceId], sourceId)
				}
				for _, entry := range timeline.Entries {
					if entry.RecordRef.SourceId != sourceId {
						t.Errorf("filtering by the terminal %q keeps records of %s and %s",
							node.id, sourceId, entry.RecordRef.SourceId)
					}
					if recordKeyOf(entry.RecordRef) == recordKeyOf(unscoped) {
						keptUnscoped = true
					}
				}
				// 絞り込みはレコードを置いた端末で比べ、時系列の行はレコードが名乗った端末を
				// 持つ。テスト用の収集元のレコードは端末を名乗らないため、行の端末は空のまま
				// である。割当や名前不明の端末を、名乗った端末として行に出さない。
				for _, entry := range timeline.Entries {
					if entry.Terminal != nil {
						t.Errorf("the timeline entry carries the terminal %q, want none because the record names no terminal",
							entry.Terminal.Id)
					}
				}
			}
			if terminals != len(result.publications) {
				t.Errorf("the graph holds %d terminals, want one for each of the %d sources",
					terminals, len(result.publications))
			}
			if !keptUnscoped {
				t.Error("no terminal filter keeps the record that names no terminal-scoped object")
			}
		})
	}
}

// 1 つの file を複数台の端末が書く形式では、端末の欄を欠くレコードを名前不明の端末に置かない。
func TestUnknownTerminalNeedsAFormatRecordedByOneTerminal(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	if !result.publications[0].parser.RecordedByOneTerminal {
		t.Fatal("the auditd binding does not declare that one terminal records its file")
	}
	result.publications[0].parser.RecordedByOneTerminal = false
	graph := NewGraph(result, AllMatchConditions())

	unknownKey, _ := core.RecordingTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256)
	if _, present := graph.nodeAt[nodeIdOf(unknownKey)]; present {
		t.Error("a format recorded by several terminals was placed on the unknown terminal")
	}
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindProcess || node.key.Kind == core.NodeKindFile {
			t.Errorf("the graph holds the %s node %+v without the scope of a terminal",
				node.key.Kind, node.key)
		}
	}
}

// 端末の IP だけを指定した収集元の端末は、名前不明の端末と同じ表示名を持つ。
func TestSpecifiedTerminalWithTheIpAloneKeepsTheUnknownName(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	assignment := analystAssignmentFor(t, result)
	assignment.TerminalId, assignment.TerminalHostname = "", ""
	assignment.Origin = core.TerminalAssignmentOriginImportSpecified
	assignment.Derivation, assignment.Author, assignment.BasisRecordRefs = "", "", nil
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the import specification: %v", err)
	}
	result.importAssignments = []core.TerminalAssignment{assignment}
	graph := NewGraph(result, AllMatchConditions())

	terminalKey, _ := core.RecordingTerminalNodeKey(assignment.SourceContentSha256)
	label := graph.nodes[requireNodeAt(t, graph, terminalKey)].label
	if label.Normalized == nil || *label.Normalized != "linux-host.log を記録した端末 (名前不明)" {
		t.Errorf("the terminal label is %+v, want the name of the unknown terminal", label)
	}
}

// 同じ収集元に 2 件の割当があるとき、どちらの端末にも決めず、名前不明の端末に置く。
func TestAmbiguousSpecificationLeavesTheUnknownTerminal(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	one := analystAssignmentFor(t, result)
	another := one
	another.TerminalId = lineageTerminalId + "-another"
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{one, another}), AllMatchConditions())

	for _, id := range []string{one.TerminalId, another.TerminalId} {
		key, _ := core.TerminalNodeKey(id)
		if _, present := graph.nodeAt[nodeIdOf(key)]; present {
			t.Errorf("the graph places the ambiguous source on the terminal %q", id)
		}
	}
	unknownKey, _ := core.RecordingTerminalNodeKey(one.SourceContentSha256)
	requireNodeAt(t, graph, unknownKey)
	assertTerminalScopedObjects(t, graph, unknownKey)
}

// 取り込みの指定は収集元の観測期間を適用期間にする。観測期間を持たない収集元には付けない。
func TestImportSpecifiedAssignmentNeedsTheObservedRange(t *testing.T) {
	result := auditdImportResult(t, accountedAuditdSource)
	records := result.publications[0].records
	identity := core.SourceIdentity{
		SourceId:           result.publications[0].status.SourceId,
		ContentSha256:      result.publications[0].status.Scope.SourceContentSha256,
		ObservedRangeFirst: records[0].ObservedAt, ObservedRangeLast: records[len(records)-1].ObservedAt,
	}
	terminal := SourceTerminal{TerminalHostname: lineageTerminalHostname}

	assignment, err := importSpecifiedAssignment(terminal, identity, nil)
	if err != nil {
		t.Fatalf("the import specification was rejected: %v", err)
	}
	if assignment.Origin != core.TerminalAssignmentOriginImportSpecified ||
		assignment.AppliesToSourceId != identity.SourceId ||
		*assignment.AssignmentValidRange.From.Normalized != *identity.ObservedRangeFirst.Normalized {
		t.Errorf("the assignment is %+v, want the import specification over the observed range of %q",
			assignment, identity.SourceId)
	}

	identity.ObservedRangeFirst, identity.ObservedRangeLast = nil, nil
	if _, err := importSpecifiedAssignment(terminal, identity, nil); err == nil ||
		!strings.Contains(err.Error(), "no record time") {
		t.Errorf("error = %v, want the reason that the source has no observed range", err)
	}
}

// 地方時の収集元は、ずれの指定の有無によらず地方時の範囲を期間にする。期間は、画面で記録した
// 時刻の解釈か起動で指定したずれで読む。
func TestImportSpecifiedAssignmentOfALocalSourceTakesTheLocalRange(t *testing.T) {
	local := core.TimeRange{
		From: *localAt(t, "2001-02-03T04:05:06"), To: *localAt(t, "2001-02-03T05:06:07"),
	}
	identity := core.SourceIdentity{SourceId: "local-source", ContentSha256: strings.Repeat("a", 64)}
	offset := core.UtcOffset("+09:00")
	for _, terminal := range []SourceTerminal{
		{TerminalId: lineageTerminalId},
		{TerminalId: lineageTerminalId, TimeOffset: &offset},
	} {
		assignment, err := importSpecifiedAssignment(terminal, identity, &local)
		if err != nil {
			t.Fatalf("the import specification (offset %v) was rejected: %v", terminal.TimeOffset, err)
		}
		if *assignment.AssignmentValidRange.From.Normalized != *local.From.Normalized ||
			assignment.AssignmentValidRange.From.Interpretation != nil {
			t.Errorf("the valid range is %+v, want the local range without an interpretation",
				assignment.AssignmentValidRange)
		}
	}
}

// レコードが自身の端末の欄を持つとき、利用者の値を足さない。
func TestSourceTerminalKeepsTheRecordOwnTerminal(t *testing.T) {
	const sourceId = "source-with-its-own-terminal"
	supplied, built := specifiedTerminalOf(core.TerminalAssignment{
		TerminalId: lineageTerminalId, TerminalHostname: lineageTerminalHostname,
		Origin: core.TerminalAssignmentOriginAnalystSupplied, Derivation: "別の収集元の接続先から導いた",
	}, false)
	if !built {
		t.Fatal("the assignment builds no terminal")
	}
	terminals := sourceTerminals{sourceId: supplied}

	withoutTerminal := RecordEntry{Locator: core.RecordLocator{SourceId: sourceId}}
	if got, _ := terminals.forRecord(withoutTerminal); len(got) != len(supplied.fields) {
		t.Errorf("a record without a terminal takes %d fields, want %d",
			len(got), len(supplied.fields))
	}
	withTerminal := RecordEntry{
		Locator:  core.RecordLocator{SourceId: sourceId},
		Terminal: []core.RecordField{syntheticField(t, "terminalId", core.SemanticKeyTerminalId, "T1")},
	}
	if got, _ := terminals.forRecord(withTerminal); len(got) != 0 {
		t.Errorf("a record holding its own terminal takes %d fields, want none", len(got))
	}
}

// twoEventAuditdSource は accountedAuditdSource の事象の後ろに、同じ形の事象をもう 1 件置く。
const twoEventAuditdSource = accountedAuditdSource +
	"type=SYSCALL msg=audit(1000000000.200:2): arch=c000003e syscall=59 success=yes ppid=2000 pid=2002 auid=4001 comm=\"tool\" exe=\"/usr/bin/tool\" key=\"exec_log\"" +
	"\x1dARCH=x86_64 SYSCALL=execve AUID=\"operator\" UID=\"root\"\n" +
	"type=EXECVE msg=audit(1000000000.200:2): argc=1 a0=\"tool\"\n" +
	"type=PATH msg=audit(1000000000.200:2): item=0 name=\"/usr/bin/tool\" nametype=NORMAL\n"

// 割当の IP は、割当の端末から IP への terminal_address の関係にだけなる。収集元のレコードは
// その IP を記録していないため、IP を指す関係を持たない。関係の詳細は割当を持ち、根拠は
// 割当が挙げたレコードだけである。
func TestAssignedAddressIsAnEdgeOfTheAssignmentAlone(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		origin       core.TerminalAssignmentOrigin
		wantEvidence int
	}{
		{"analyst supplied", core.TerminalAssignmentOriginAnalystSupplied, 1},
		// 取り込みの指定は根拠のレコードを持たない。
		{"import specified", core.TerminalAssignmentOriginImportSpecified, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, _ := assignedAuditdResult(t, testCase.origin)
			graph := NewGraph(result, AllMatchConditions())

			terminalKey, _ := core.TerminalNodeKey(lineageTerminalId)
			terminalAt := requireNodeAt(t, graph, terminalKey)
			ipAt := requireNodeAt(t, graph, core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
				Values: []core.NodeIdentityValue{{Value: lineageClientIp}}})
			edgeId := ""
			for _, edge := range graph.edges {
				if edge.target != ipAt && edge.source != ipAt {
					continue
				}
				if edge.kind != core.EdgeKindTerminalAddress || edge.source != terminalAt {
					t.Errorf("the %s edge from %+v names the assigned IP", edge.kind, graph.nodes[edge.source].key)
					continue
				}
				edgeId = edge.id
			}
			if edgeId == "" {
				t.Fatal("the assigned terminal has no terminal_address edge to the assigned IP")
			}
			// 端末の割当は収集元のレコードを割当の端末に置く。
			assertTerminalScopedObjects(t, graph, terminalKey)

			detail, found := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{})
			if !found {
				t.Fatal("the terminal_address edge has no detail")
			}
			if len(detail.TerminalAssignments) != 1 || detail.TerminalAssignments[0].Origin != testCase.origin ||
				detail.TerminalAssignments[0].ClientIp != lineageClientIp {
				t.Errorf("the edge carries the assignments %+v, want the one assignment of the IP",
					detail.TerminalAssignments)
			}
			if detail.ObservedInRecords {
				t.Error("the edge claims a record that names the assigned IP")
			}
			// Candidate API projects this same assignment from the matched edge detail.
			if len(detail.Evidence) != testCase.wantEvidence {
				t.Fatalf("the edge holds %d evidence records, want %d", len(detail.Evidence), testCase.wantEvidence)
			}
			// 一覧のエッジも詳細のエッジも、割当の由来を持つ。根拠のレコードを持たない
			// エッジの期間は、割当の適用期間である。
			listed := graph.Query(GraphQuery{Depth: 1, EdgeKinds: []core.EdgeKind{core.EdgeKindTerminalAddress}})
			listedAt := slices.IndexFunc(listed.Edges, func(edge core.GraphEdge) bool { return edge.Id == edgeId })
			if listedAt < 0 {
				t.Fatal("the subgraph drops the edge")
			}
			validRange := detail.TerminalAssignments[0].AssignmentValidRange
			for _, edge := range []core.GraphEdge{detail.Edge, listed.Edges[listedAt]} {
				if !slices.Equal(edge.AssignmentOrigins, []core.TerminalAssignmentOrigin{testCase.origin}) {
					t.Errorf("the edge carries the origins %v, want %s", edge.AssignmentOrigins, testCase.origin)
				}
				if testCase.wantEvidence == 0 &&
					(edge.ApplicableRange == nil || !reflect.DeepEqual(*edge.ApplicableRange, validRange)) {
					t.Errorf("the edge without evidence has the range %+v, want the valid range %+v of the assignment",
						edge.ApplicableRange, validRange)
				}
			}
			basisRecord := result.publications[0].records[0].Locator
			for _, evidence := range detail.Evidence {
				if recordKeyOf(evidence.RecordRef) != recordKeyOf(basisRecord) {
					t.Errorf("the evidence %+v is not the basis record of the assignment", evidence.RecordRef)
				}
			}
		})
	}
}

func TestAttackRuleMatchedEdgeCarriesTerminalAssignment(t *testing.T) {
	result, assignment := assignedAuditdResult(t, core.TerminalAssignmentOriginAnalystSupplied)
	graph := NewGraph(result, AllMatchConditions())
	rule := attackrules.Rule{
		ID: "synthetic.terminal-address", Title: "Terminal address", Description: "Assigned address edge",
		References: []string{"https://example.test/rules/terminal-address"},
		Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisInferred}},
		Variants: []attackrules.Variant{{ID: "default", Pattern: attackrules.Pattern{
			Hosts: []string{"terminal"},
			Nodes: map[string]attackrules.PatternNode{"ip": {Kind: string(core.NodeKindIp)}},
			Edges: map[string]attackrules.PatternEdge{
				"address": {Kind: string(core.EdgeKindTerminalAddress), From: "terminal", To: "ip"},
			},
			Evidence: attackrules.Evidence{Required: []string{"address"}},
		}}},
	}

	evaluation, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, []attackrules.Rule{rule})
	if err != nil {
		t.Fatalf("AttackRuleMatches() error = %v", err)
	}
	if len(evaluation.NotEvaluated) != 0 || len(evaluation.Matches) != 1 || len(evaluation.Matches[0].Edges) != 1 {
		t.Fatalf("AttackRuleMatches() = %+v; want one evaluated match with one edge", evaluation)
	}
	matched := evaluation.Matches[0].Edges[0]
	if matched.Role != "address" {
		t.Fatalf("matched edge role = %q, want address", matched.Role)
	}
	if !reflect.DeepEqual(matched.TerminalAssignments, []core.TerminalAssignment{assignment}) {
		t.Fatalf("matched edge assignments = %+v, want %+v", matched.TerminalAssignments, assignment)
	}
}

// **収集元の全体に付けない割当 (接続元 IP ごとの割当) も、端末が持つ IP アドレスの関係になる。**
// 関係は割当を運び、根拠は割当が挙げたレコードである。割当の期間を読み取った収集元の案件で
// 絞った要求にも残る。
func TestAddressAssignmentOfAClientIsAnEdge(t *testing.T) {
	proxy := `192.0.2.10 - - [03/Feb/2001:04:10:02 +0000] "GET http://example.test/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:04:20:00 +0000] "GET http://example.test/b HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n"
	result := sessionSourcesResult(t,
		sessionSource{name: "access.log", format: SquidFormatKey, document: proxy, caseId: "case-a"})
	result = result.WithAnalystTerminalAssignments([]core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "client-a", "", false),
	})
	graph := NewGraph(result, AllMatchConditions())
	terminalKey, _ := core.TerminalNodeKey("client-a")
	terminalAt := requireNodeAt(t, graph, terminalKey)
	ipAt := requireNodeAt(t, graph, core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: "192.0.2.10"}}})
	edgeId := ""
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTerminalAddress && edge.source == terminalAt && edge.target == ipAt {
			edgeId = edge.id
		}
	}
	if edgeId == "" {
		t.Fatal("the assigned terminal has no terminal_address edge to the client IP")
	}
	detail, found := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{})
	if !found || len(detail.TerminalAssignments) != 1 || len(detail.Evidence) != 1 {
		t.Fatalf("the edge carries %+v, want the assignment and its basis record", detail)
	}
	inCase := graph.Query(GraphQuery{Depth: 1, EdgeKinds: []core.EdgeKind{core.EdgeKindTerminalAddress},
		RecordFilter: RecordFilter{Case: "case-a"}})
	if !slices.ContainsFunc(inCase.Edges, func(edge core.GraphEdge) bool { return edge.Id == edgeId }) {
		t.Errorf("the case filter drops the edge, the edges are %+v", inCase.Edges)
	}
	// 収集元で絞った要求は、割当の期間を読み取った収集元で比べる。
	for source, want := range map[string]bool{"source-access.log": true, "source-other.log": false} {
		inSource := graph.Query(GraphQuery{Depth: 1, EdgeKinds: []core.EdgeKind{core.EdgeKindTerminalAddress},
			RecordFilter: RecordFilter{Sources: []string{source}}})
		if got := slices.ContainsFunc(inSource.Edges, func(edge core.GraphEdge) bool {
			return edge.Id == edgeId
		}); got != want {
			t.Errorf("the source filter %s keeps the edge: %v, want %v", source, got, want)
		}
	}
}

// assignedAuditdResult は twoEventAuditdSource を取り込み、その収集元に origin の割当を 1 件
// 付けた取り込み結果と割当を返す。分析者の割当は先頭のレコードを根拠に挙げる。
func assignedAuditdResult(
	t *testing.T, origin core.TerminalAssignmentOrigin,
) (ImportResult, core.TerminalAssignment) {
	t.Helper()
	result := auditdImportResult(t, twoEventAuditdSource)
	if records := len(result.publications[0].records); records != 2 {
		t.Fatalf("the fixture holds %d records, want 2", records)
	}
	assignment := analystAssignmentFor(t, result)
	if origin == core.TerminalAssignmentOriginAnalystSupplied {
		return result.WithAnalystTerminalAssignments([]core.TerminalAssignment{assignment}), assignment
	}
	assignment.Origin = origin
	assignment.Derivation, assignment.Author, assignment.BasisRecordRefs = "", "", nil
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the import specification: %v", err)
	}
	result.importAssignments = []core.TerminalAssignment{assignment}
	return result, assignment
}

// 割当から作った関係と IP のノードは、レコードの絞り込みを割当の側で判定する。割当の端末、
// 割当の適用期間と重なる期間、割当を付けた収集元の案件では残り、外れる条件では消える。
func TestAssignedAddressEdgePassesTheRecordFilterOfItsAssignment(t *testing.T) {
	terminalKey, _ := core.TerminalNodeKey(lineageTerminalId)
	terminalId := nodeIdOf(terminalKey)
	ipKey := core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: lineageClientIp}}}
	// 先頭のレコードは 1000000000.100、2 件目は 1000000000.200 である。割当の適用期間は 2 件の間。
	second := time.Unix(1000000000, 200_000_000).UTC()
	whole := time.Unix(1000000000, 0).UTC()
	future := time.Unix(2000000000, 0).UTC()
	for _, testCase := range []struct {
		name   string
		origin core.TerminalAssignmentOrigin
		filter RecordFilter
		want   bool
	}{
		{"the assigned terminal", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{Terminal: terminalId}, true},
		{"the whole period", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{TimeFrom: &whole, TimeTo: &second, TimeUnit: time.Second}, true},
		// 根拠のレコード (先頭) は期間の外にあり、割当の適用期間は期間と重なる。
		{"a period without the basis record", core.TerminalAssignmentOriginAnalystSupplied,
			RecordFilter{TimeFrom: &second, TimeTo: &second, TimeUnit: time.Millisecond}, true},
		{"another terminal", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{Terminal: "terminal:another"}, false},
		{"a period after the assignment", core.TerminalAssignmentOriginAnalystSupplied,
			RecordFilter{TimeFrom: &future, TimeUnit: time.Second}, false},
		{"another case", core.TerminalAssignmentOriginAnalystSupplied,
			RecordFilter{Case: "another-case"}, false},
		// 割当は事象ではないため、事象の種別を持つ要求では割当の側で通さない。
		{"an event category", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{EventCategory: "no-such-category"}, false},
		{"an event action with the assigned terminal", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{EventAction: "no-such-action", Terminal: terminalId}, false},
		{"an event action range", core.TerminalAssignmentOriginImportSpecified,
			RecordFilter{EventActionFrom: uint64Of(0), Terminal: terminalId}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, _ := assignedAuditdResult(t, testCase.origin)
			graph := NewGraph(result, AllMatchConditions())
			ipId := graph.nodes[requireNodeAt(t, graph, ipKey)].id

			records := graph.Query(GraphQuery{Depth: 1, EdgeKinds: []core.EdgeKind{core.EdgeKindTerminalAddress},
				RecordFilter: testCase.filter})
			if got := len(records.Edges) == 1 && records.Edges[0].TargetNodeId == ipId; got != testCase.want {
				t.Errorf("the record granularity carries the edges %+v, want the assigned address %v",
					records.Edges, testCase.want)
			}
			objects := graph.Query(GraphQuery{Granularity: core.GraphGranularityObject,
				NodeKinds: []core.NodeKind{core.NodeKindIp}, RecordFilter: testCase.filter})
			matched := slices.ContainsFunc(objects.Nodes, func(node core.SubgraphNode) bool {
				return node.Id == ipId && node.Selection == core.NodeSelectionMatched
			})
			if matched != testCase.want {
				t.Errorf("the object granularity matches the assigned IP: %v, want %v", matched, testCase.want)
			}
		})
	}
}

// レコード自身の IP と割当が同じ端末と IP の関係を与えると、関係は 1 本であり、レコードが
// 直接観測したことと割当の両方を持つ。
func TestAssignedAddressEdgeKeepsTheRecordObservation(t *testing.T) {
	result, err := graphRunner(t).Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-session.log", FileName: "graph-markii-session.log",
		FormatKey: MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	assignment := analystAssignmentFor(t, result)
	assignment.ClientIp, assignment.TerminalId = "192.0.2.1", "T1"
	graph := NewGraph(result.WithAnalystTerminalAssignments([]core.TerminalAssignment{assignment}),
		AllMatchConditions())

	terminalKey, _ := core.TerminalNodeKey("T1")
	terminalAt := requireNodeAt(t, graph, terminalKey)
	ipAt := requireNodeAt(t, graph, core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: "192.0.2.1"}}})
	edges := 0
	edgeId := ""
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTerminalAddress && edge.source == terminalAt && edge.target == ipAt {
			edges++
			edgeId = edge.id
		}
	}
	if edges != 1 {
		t.Fatalf("the graph holds %d terminal_address edges from T1 to its address, want 1", edges)
	}
	detail, _ := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{})
	if !detail.ObservedInRecords || len(detail.TerminalAssignments) != 1 {
		t.Errorf("observedInRecords=%v assignments=%d, want the record observation and the assignment",
			detail.ObservedInRecords, len(detail.TerminalAssignments))
	}
}

// assertTerminalScopedObjects は、事象のプロセスとファイルとアカウントが terminal の範囲で
// 識別され、terminal につながることを確かめる。
func assertTerminalScopedObjects(t *testing.T, graph Graph, terminal core.NodeKey) {
	t.Helper()
	terminalAt := requireNodeAt(t, graph, terminal)
	scope := terminal.Values[0]

	processAt := -1
	for at, node := range graph.nodes {
		if node.key.Form == core.NodeKeyFormTerminalProcessInterval &&
			node.key.Values[0] == scope && node.key.Values[1].Value == "2001" {
			processAt = at
		}
	}
	if processAt < 0 {
		t.Fatal("the pid 2001 has no process node in the scope of the terminal")
	}
	if !hasEdge(graph, core.EdgeKindRanOn, processAt, terminalAt) {
		t.Error("the process has no ran_on edge to the terminal")
	}

	fileAt := requireNodeAt(t, graph, core.NodeKey{
		Kind: core.NodeKindFile, Form: core.NodeKeyFormTerminalFilePath,
		Values: []core.NodeIdentityValue{scope, {Semantic: core.SemanticKeyFilePath, Value: accountedPath}},
	})
	accountAt := requireNodeAt(t, graph, core.NodeKey{
		Kind: core.NodeKindAccount, Form: core.NodeKeyFormTerminalAccountName,
		Values: []core.NodeIdentityValue{scope, {Semantic: core.SemanticKeyAccountName, Value: accountedLogin}},
	})
	if !hasEdge(graph, core.EdgeKindTerminalAccount, terminalAt, accountAt) {
		t.Error("the account has no terminal_account edge from the terminal")
	}
	if !hasEdge(graph, core.EdgeKindFileOperation, processAt, fileAt) {
		t.Error("the process has no file_operation edge to the file its record named")
	}
	// ファイルとプロセスは、事象のレコードのノードからも指される。
	namedByRecord := func(target int) bool {
		for _, edge := range graph.edges {
			if edge.kind == core.EdgeKindRecordNamesObject && edge.target == target &&
				hasEdge(graph, core.EdgeKindRecordNamesObject, edge.source, terminalAt) {
				return true
			}
		}
		return false
	}
	if !namedByRecord(fileAt) {
		t.Error("no record node names both the file and the terminal")
	}
	if !namedByRecord(processAt) {
		t.Error("no record node names both the process and the terminal")
	}
}

// requireNodeAt はグラフの中のノードの位置を返す。無ければ test を止める。
func requireNodeAt(t *testing.T, graph Graph, key core.NodeKey) int {
	t.Helper()
	at, present := graph.nodeAt[nodeIdOf(key)]
	if !present {
		t.Fatalf("the graph holds no node of the key %+v", key)
	}
	return at
}

// hasEdge は、種別と両端が一致するエッジをグラフが持つかを返す。
func hasEdge(graph Graph, kind core.EdgeKind, source, target int) bool {
	for _, edge := range graph.edges {
		if edge.kind == kind && edge.source == source && edge.target == target {
			return true
		}
	}
	return false
}
