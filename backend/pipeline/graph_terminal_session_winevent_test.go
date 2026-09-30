// in-package test: Windows イベントログの XML と分析者の割当からグラフを組み、
// ログオンのレコードが作る遠隔のセッションの候補を確かめる。
package pipeline

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// logonXML はログオンのイベント 1 件を返す。ip が空のイベントは IpAddress の
// Data を持たない。
func logonXML(computer, at, recordID, eventID, ip, logonType string) string {
	data := `<Data Name="TargetUserName">user-a</Data><Data Name="TargetDomainName">EXAMPLE</Data>` +
		`<Data Name="LogonType">` + logonType + `</Data>`
	if ip != "" {
		data += `<Data Name="IpAddress">` + ip + `</Data><Data Name="IpPort">49152</Data>`
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>` + eventID +
		`</EventID><TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Security</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData>` + data + `</EventData></Event>` + "\n"
}

// sessionTime は日の時刻 clock を返す。utc が偽の時刻は UTC からのずれを持たない。
func sessionTime(clock string, utc bool) string {
	if utc {
		return "2001-02-03T" + clock + "Z"
	}
	return "2001-02-03 " + clock
}

// sessionClientXML は接続元の端末 ws.example.test の file を返す。観測期間は 04:00:00 から
// 04:30:00 である。
func sessionClientXML(utc bool) string {
	return "<Events>\n" +
		processCreationXML("ws.example.test", sessionTime("04:00:00", utc), "701", "0x10", "0x1",
			`C:\Example\ws-shell.exe`, "ws-shell.exe") +
		processCreationXML("ws.example.test", sessionTime("04:30:00", utc), "702", "0x11", "0x10",
			`C:\Example\ws-tool.exe`, "ws-tool.exe") +
		"</Events>\n"
}

// sessionServerXML はログオンを記録した端末 dc.example.test の file を返す。
//
// 802 と 803 は 192.0.2.10 からのログオンの成功と失敗、804 は割当の無い 198.51.100.7 からの
// ログオン、805 は接続元の端末の観測期間の後の 192.0.2.10 からのログオン、806 は接続元を
// 記録しないログオンである。
func sessionServerXML(utc bool) string {
	return "<Events>\n" +
		processCreationXML("dc.example.test", sessionTime("04:00:30", utc), "801", "0x20", "0x1",
			`C:\Example\dc-service.exe`, "dc-service.exe") +
		logonXML("dc.example.test", sessionTime("04:10:00", utc), "802", "4624", "192.0.2.10", "3") +
		logonXML("dc.example.test", sessionTime("04:11:00", utc), "803", "4625", "192.0.2.10", "3") +
		logonXML("dc.example.test", sessionTime("04:12:00", utc), "804", "4624", "198.51.100.7", "3") +
		logonXML("dc.example.test", sessionTime("04:13:00", utc), "806", "4624", "", "5") +
		logonXML("dc.example.test", sessionTime("04:40:00", utc), "805", "4624", "192.0.2.10", "3") +
		"</Events>\n"
}

var (
	sessionClientDocument = sessionClientXML(true)
	sessionServerDocument = sessionServerXML(true)
)

// windowsEventSessionResult は、与えた XML を 1 つずつ別の収集元として取り込んだ結果を返す。
// file 名は並びの位置から `events-<位置>.xml` にする。
func windowsEventSessionResult(t *testing.T, documents ...string) ImportResult {
	t.Helper()
	sources := make([]sessionSource, 0, len(documents))
	for index, document := range documents {
		sources = append(sources, sessionSource{
			name: "events-" + strconv.Itoa(index) + ".xml", format: WindowsEventXMLFormatKey,
			document: document,
		})
	}
	return sessionSourcesResult(t, sources...)
}

// sessionSource は取り込む収集元 1 件である。caseId が空の収集元は案件を持たない。
type sessionSource struct {
	name, document, caseId string
	format                 core.FormatKey
}

// sessionSourcesResult は、与えた収集元を並びの順に取り込んだ結果を返す。
func sessionSourcesResult(t *testing.T, sources ...sessionSource) ImportResult {
	t.Helper()
	scanned := make([]scannedSource, 0, len(sources))
	statuses := make([]core.ImportStatus, 0, len(sources))
	for _, source := range sources {
		read := scanIndexSource(t, NewTestParser(source.format, nil), source.name, source.format,
			source.document)
		scanned = append(scanned, read)
		statuses = append(statuses, settleStatus(t, read, "source-"+source.name))
	}
	result, err := newImportResult(scanned, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	// 収集元の識別は取り込みの実行 (Runner.Run) が足す。時刻の解釈は識別の内容の識別で探し、
	// 案件は識別の案件で分ける。
	result.identities = make(map[string]core.SourceIdentity, len(scanned))
	for index, read := range scanned {
		identity := sourceIdentity(read, statuses[index].SourceId)
		if caseId := sources[index].caseId; caseId != "" {
			identity.CaseId = &caseId
		}
		result.identities[statuses[index].SourceId] = identity
	}
	return result
}

// sessionAssignment は、periodSource 番目の収集元の観測期間を期間に持つ分析者の割当を返す。
// applies が真の割当は、その収集元のレコードの全体に適用する。
func sessionAssignment(
	t *testing.T, result ImportResult, periodSource int,
	clientIp, terminalId, hostname string, applies bool,
) core.TerminalAssignment {
	t.Helper()
	publication := result.publications[periodSource]
	records := publication.records
	assignment := core.TerminalAssignment{
		ClientIp: clientIp, TerminalId: terminalId, TerminalHostname: hostname,
		SourceId:            publication.status.SourceId,
		SourceContentSha256: publication.status.Scope.SourceContentSha256,
		AssignmentValidRange: core.TimeRange{
			From: *records[0].ObservedAt, To: *records[len(records)-1].ObservedAt,
		},
		Origin:          core.TerminalAssignmentOriginAnalystSupplied,
		Derivation:      "接続元の端末の設定から導いた",
		Author:          "analyst",
		BasisRecordRefs: []core.AssertionRecordRef{core.NewAssertionRecordRef(records[0].Locator)},
	}
	if applies {
		assignment.AppliesToSourceId = publication.status.SourceId
	}
	if err := assignment.Validate(); err != nil {
		t.Fatalf("building the analyst assignment: %v", err)
	}
	return assignment
}

// sessionDetail は、遠隔のセッションの候補がちょうど 1 本であることを確かめ、その詳細を返す。
func sessionDetail(t *testing.T, graph Graph) EdgeDetail {
	t.Helper()
	edges := terminalSessionEdges(t, graph)
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d remote session relations, want 1", len(edges))
	}
	detail, found := graph.EdgeDetail(edges[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the relation %q has no detail", edges[0].Id)
	}
	return detail
}

// requireLogonTypeGroups は、根拠の区分がすべてログオンの種別 wantType を運び、区分の件数の
// 和が根拠の件数と一致することを確かめる。
func requireLogonTypeGroups(t *testing.T, detail EdgeDetail, wantType string) {
	t.Helper()
	var grouped int64
	for _, group := range detail.EvidenceGroups {
		if group.LogonType == nil {
			t.Fatalf("a group carries no logon type: %q", group.LogonTypeAbsence)
		}
		if value, readable := comparableTextOf(*group.LogonType); !readable || value != wantType {
			t.Errorf("a group carries the logon type %q, want %q", value, wantType)
		}
		grouped += group.EvidenceCount
	}
	if grouped != detail.Edge.EvidenceCount {
		t.Errorf("the groups hold %d records, want the %d evidence records",
			grouped, detail.Edge.EvidenceCount)
	}
}

// 分析者が接続元の端末の収集元に IP を割り当てると、その名前不明の端末から、ログオンを
// 記録した Computer の端末への候補ができる。
//
// 根拠は期間の中の 192.0.2.10 からのログオンの成功と失敗である。割当の無いアドレス、
// 期間の外、接続元を記録しないログオンは根拠にならない。
func TestWindowsLogonSessionRunsFromTheAssignedUnknownTerminal(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())

	client, _ := core.RecordingTerminalNodeKey(assignment.SourceContentSha256)
	server, _ := core.RecordingHostTerminalNodeKey(
		result.publications[1].status.Scope.SourceContentSha256, "dc.example.test")
	detail := sessionDetail(t, graph)
	if detail.Edge.SourceNodeId != nodeIdOf(client) || detail.Edge.TargetNodeId != nodeIdOf(server) {
		t.Fatalf("the relation runs from %q to %q, want the assigned terminal to the dc terminal",
			detail.Edge.SourceNodeId, detail.Edge.TargetNodeId)
	}
	if detail.Edge.State != core.RelationStateCandidate || detail.Edge.EvidenceCount != 2 {
		t.Errorf("the relation is %q with %d evidence records, want a candidate with 2",
			detail.Edge.State, detail.Edge.EvidenceCount)
	}
	requireLogonTypeGroups(t, detail, "3")

	if len(detail.AssignmentBases) != 1 || len(detail.AssignmentBases[0].Conditions) != 1 {
		t.Fatalf("the relation carries the bases %+v, want one basis with one condition",
			detail.AssignmentBases)
	}
	condition := detail.AssignmentBases[0].Conditions[0]
	if len(condition.LeftValue) != 1 || condition.LeftValue[0].Name != "EventData.IpAddress" {
		t.Errorf("the left value is %+v, want the IpAddress item", condition.LeftValue)
	}
	// 外部識別子も表示名も持たない割当の端末は、割当を付けた収集元の file 名で表す。
	// terminal.hostname を名乗らない。ノードの表示名を材料にしない。
	right := condition.RightValue
	if len(right) != 1 || right[0].Name != clientTerminalSourceFieldName || right[0].Semantic != "" ||
		right[0].Text == nil || right[0].Text.ValueState != core.ValueStateDerived {
		t.Fatalf("the right value is %+v, want the derived source of the terminal", right)
	}
	if value, _ := right[0].Text.ComparableValue(); value != "events-0.xml" {
		t.Errorf("the right value carries %q, want the file name of the assigned source", value)
	}

	resultMatches, err := graph.AttackRuleMatches(GraphQuery{Depth: 1}, foundationAttackRules(t))
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, match := range resultMatches.Matches {
		if match.RuleID != "attack.t1021.remote-services" {
			continue
		}
		for _, edge := range match.Edges {
			if edge.Role == "session" && edge.Edge.Id == detail.Edge.Id {
				matched = true
			}
		}
	}
	if !matched {
		t.Error("the Windows logon session has no T1021 candidate")
	}
}

// 分析者が端末の外部識別子で IP を割り当てた端末は、その端末を記録した収集元が無くても
// 候補の起点になる。起点のノードは、ログオンのレコードが割当を通して参照したノードである。
func TestWindowsLogonSessionRunsFromAnAssignedTerminalWithoutItsOwnSource(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "ws-9", "ws9.example.test", false)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())

	client, _ := core.TerminalNodeKey("ws-9")
	detail := sessionDetail(t, graph)
	if detail.Edge.SourceNodeId != nodeIdOf(client) {
		t.Fatalf("the relation runs from %q, want the assigned terminal", detail.Edge.SourceNodeId)
	}
	// 期間は dc の収集元の観測期間であり、805 も期間の中にある。
	if detail.Edge.EvidenceCount != 3 {
		t.Errorf("the relation carries %d evidence records, want 3", detail.Edge.EvidenceCount)
	}
	requireLogonTypeGroups(t, detail, "3")
	node := graph.nodes[graph.nodeAt[nodeIdOf(client)]]
	if node.observation != core.NodeObservationReferenced || len(node.evidence) != 3 {
		t.Errorf("the assigned terminal is %q with %d evidence records, want referenced with 3",
			node.observation, len(node.evidence))
	}
	if label, _ := node.label.ComparableValue(); label != "ws9.example.test" ||
		node.label.ValueState != core.ValueStateDerived {
		t.Errorf("the assigned terminal is labelled %+v, want the derived hostname", node.label)
	}
	right := detail.AssignmentBases[0].Conditions[0].RightValue
	if value, _ := right[0].Text.ComparableValue(); right[0].Semantic != core.SemanticKeyTerminalId ||
		value != "ws-9" {
		t.Errorf("the right value is %+v, want the terminal id", right)
	}
}

// 端末で絞る一覧は、参照だけの端末のノードを除いた端末を出す。一覧に出る端末は、どれを
// 選んでも 1 件以上のレコードに絞れる。割当だけが導いた参照の端末には、置いたレコードが無い。
func TestTerminalFilterKeepsRecordsOnEveryObservedTerminal(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "ws-9", "ws9.example.test", false)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())

	referenced, observed := 0, 0
	for _, node := range graph.nodes {
		if node.key.Kind != core.NodeKindTerminal {
			continue
		}
		kept := graph.Timeline(TimelineQuery{RecordFilter: RecordFilter{Terminal: node.id}}).Entries
		if node.observation == core.NodeObservationReferenced {
			referenced++
			if len(kept) != 0 {
				t.Errorf("the referenced terminal %q keeps %d records, want none", node.id, len(kept))
			}
			continue
		}
		observed++
		if len(kept) == 0 {
			t.Errorf("the observed terminal %q keeps no record", node.id)
		}
	}
	if referenced == 0 || observed == 0 {
		t.Fatalf("the graph holds %d referenced and %d observed terminals, want both", referenced, observed)
	}
}

// 記録した端末自身に割り当てた IP からのログオンは、関係にしない。
func TestWindowsLogonSessionSkipsTheRecordingTerminalItself(t *testing.T) {
	result := windowsEventSessionResult(t, sessionServerDocument)
	assignment := sessionAssignment(t, result, 0, "192.0.2.10", "", "", true)
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{assignment}), AllMatchConditions())
	if edges := terminalSessionEdges(t, graph); len(edges) != 0 {
		t.Errorf("the graph carries the remote session relations %+v, want none", edges)
	}
}

// interpretedSessionResult は、すべての収集元の時刻を offset で読む取り込み結果を返す。
func interpretedSessionResult(result ImportResult, offset core.UtcOffset) ImportResult {
	applied := make(map[string]core.TimestampInterpretation, len(result.publications))
	for _, publication := range result.publications {
		applied[publication.status.Scope.SourceContentSha256] = core.TimestampInterpretation{
			Offset: offset, AssertionId: "as:" + publication.status.SourceId,
		}
	}
	return result.withTimeInterpretations(applied)
}

// 時刻の解釈を記録した収集元は、解釈で読んだ観測期間を、地方時の文字列と分析者のずれの組で
// 返す。収集元の識別の観測期間は原資料から定まる値のまま、持たない。
//
// 地方時の期間を持つ分析者の割当は、グラフを組むときの解釈で期間を読む。解釈を取り消すと
// 期間を比べられず、割当を適用しない。解釈を変えると、期間もレコードも新しいずれで読む。
func TestWindowsLogonSessionUsesTheInterpretedObservedRange(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientXML(false), sessionServerXML(false))
	client := result.publications[0].status
	if _, found := result.InterpretedObservedRange(client.SourceId); found {
		t.Fatal("a source without an interpretation returned an interpreted range")
	}
	interpreted := interpretedSessionResult(result, "+00:00")
	interpretedRange, found := interpreted.InterpretedObservedRange(client.SourceId)
	if !found {
		t.Fatal("the interpreted source returned no interpreted range")
	}
	if *interpretedRange.From.Normalized != "2001-02-03T04:00:00" ||
		*interpretedRange.To.Normalized != "2001-02-03T04:30:00" ||
		interpretedRange.From.Interpretation == nil || interpretedRange.From.Interpretation.Offset != "+00:00" {
		t.Fatalf("the interpreted range is %+v, want the local range read at +00:00", interpretedRange)
	}
	if identity, _ := interpreted.Identity(client.SourceId); identity.ObservedRangeFirst != nil {
		t.Fatalf("the source identity carries the range %+v", identity.ObservedRangeFirst)
	}
	local := core.TimeRange{From: withoutInterpretation(t, interpretedRange.From),
		To: withoutInterpretation(t, interpretedRange.To)}
	assignmentOver := func(validRange core.TimeRange) core.TerminalAssignment {
		t.Helper()
		assignment := core.TerminalAssignment{
			ClientIp: "192.0.2.10", SourceId: client.SourceId,
			SourceContentSha256: client.Scope.SourceContentSha256, AssignmentValidRange: validRange,
			Origin: core.TerminalAssignmentOriginAnalystSupplied, Derivation: "接続元の端末の設定から導いた",
			Author: "analyst", AppliesToSourceId: client.SourceId,
			BasisRecordRefs: []core.AssertionRecordRef{
				core.NewAssertionRecordRef(result.publications[0].records[0].Locator),
			},
		}
		if err := assignment.Validate(); err != nil {
			t.Fatalf("the assignment over %+v was rejected: %v", validRange, err)
		}
		return assignment
	}
	evidenceOf := func(read ImportResult, assignment core.TerminalAssignment) []int64 {
		t.Helper()
		graph := NewGraph(read.WithAnalystTerminalAssignments(
			[]core.TerminalAssignment{assignment}), AllMatchConditions())
		numbers := []int64{}
		for _, edge := range terminalSessionEdges(t, graph) {
			detail, _ := graph.EdgeDetail(edge.Id, EdgeEvidenceFilter{})
			for _, evidence := range detail.Evidence {
				numbers = append(numbers, *evidence.RecordRef.LineNumber)
			}
		}
		return numbers
	}
	localAssignment := assignmentOver(local)
	if got := evidenceOf(interpreted, localAssignment); len(got) != 2 {
		t.Errorf("the interpreted sources join %d records, want the 2 logons inside the range", len(got))
	}
	if got := evidenceOf(result, localAssignment); len(got) != 0 {
		t.Errorf("the withdrawn interpretation still joins %d records, want none", len(got))
	}
	// -00:20 で読むと、期間は 04:20:00Z から 04:50:00Z、ログオンは 04:30:00Z と 04:31:00Z になる。
	changed := interpretedSessionResult(result, "-00:20")
	if got := evidenceOf(changed, localAssignment); len(got) != 2 {
		t.Errorf("the changed interpretation joins %d records, want the 2 logons read with it", len(got))
	}
	// UTC の時点で期間を持つ割当は、解釈に依らず同じ期間を持つ。04:31:00Z のログオンは外に出る。
	utc := core.TimeRange{From: absoluteOf(t, interpretedRange.From), To: absoluteOf(t, interpretedRange.To)}
	if got := evidenceOf(changed, assignmentOver(utc)); len(got) != 1 {
		t.Errorf("the UTC assignment joins %d records, want the 1 logon inside its fixed range", len(got))
	}
}

// withoutInterpretation は、分析者のずれを外した地方時の時刻を返す。
func withoutInterpretation(t *testing.T, timestamp core.Timestamp) core.Timestamp {
	t.Helper()
	timestamp.Interpretation = nil
	local, err := core.NewTimestamp(timestamp)
	if err != nil {
		t.Fatal(err)
	}
	return local
}

// absoluteOf は、分析者のずれで読んだ時点を UTC で書いた時刻を返す。
func absoluteOf(t *testing.T, timestamp core.Timestamp) core.Timestamp {
	t.Helper()
	absolute, converted := timestamp.AbsoluteByInterpretation()
	if !converted {
		t.Fatalf("the time %+v was not converted", timestamp)
	}
	return absolute
}

// 割当が無いとき、ログオンのレコードは候補を作らない。アドレスの一致だけで端末を決めない。
func TestWindowsLogonSessionNeedsAnAssignment(t *testing.T) {
	graph := NewGraph(windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument),
		AllMatchConditions())
	if edges := terminalSessionEdges(t, graph); len(edges) != 0 {
		t.Errorf("the graph carries the remote session relations %+v, want none", edges)
	}
}

// ずれを指定せずに起動で IP 付きの端末を指定した地方時の収集元は、解釈を持たない間、接続元
// IP で端末を決めない。画面で時刻の解釈を記録すると、割当の期間を読めて端末が決まる。
func TestImportSpecifiedTerminalOfALocalSourceWaitsForAnInterpretation(t *testing.T) {
	result, err := memoryRunner(t, map[string]string{
		"ws.xml": sessionClientXML(false), "dc.xml": sessionServerXML(false),
	}).Run([]SourcePlan{
		{OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey,
			Terminal: &SourceTerminal{TerminalId: "ws-01", Ip: "192.0.2.10"}},
		{OriginPath: "dc.xml", FileName: "dc.xml", FormatKey: WindowsEventXMLFormatKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	if edges := terminalSessionEdges(t, NewGraph(result, AllMatchConditions())); len(edges) != 0 {
		t.Fatalf("the uninterpreted range decides the terminal by IP: %+v", edges)
	}
	client, _ := core.TerminalNodeKey("ws-01")
	detail := sessionDetail(t, NewGraph(interpretedSessionResult(result, "+00:00"), AllMatchConditions()))
	if detail.Edge.SourceNodeId != nodeIdOf(client) || detail.Edge.EvidenceCount != 2 {
		t.Errorf("the relation runs from %q with %d evidence records, want ws-01 with 2",
			detail.Edge.SourceNodeId, detail.Edge.EvidenceCount)
	}
}
