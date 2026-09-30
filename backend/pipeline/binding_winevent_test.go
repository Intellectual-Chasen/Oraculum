// in-package test: 走査した結果の失敗と位置、組んだグラフの端末とプロセスのノードを確かめる。
package pipeline

import (
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Windows イベントログの XML。host は example.test 系である。2 件目は `</Event>` を持たない。
const windowsEventScanDocument = `<?xml version="1.1" encoding="utf-8"?>
<Events>
<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4688</EventID>
<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/><EventRecordID>301</EventRecordID>
<Channel>Security</Channel><Computer>host03.example.test</Computer></System>
<EventData><Data Name="NewProcessId">0x2b</Data><Data Name="NewProcessName">C:\Example\child.exe</Data>
<Data Name="ProcessId">0x1f</Data><Data Name="CommandLine">child.exe /q</Data></EventData></Event>
<Event><System><EventRecordID>302</EventRecordID>
<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>
<TimeCreated SystemTime="2001-02-03 04:05:07"/><EventRecordID>303</EventRecordID>
<Channel>Application</Channel><Computer>host03.example.test</Computer></System></Event>
</Events>
`

// processCreationXML はプロセスの作成のイベント 1 件を返す。commandLine が空の
// イベントは CommandLine の Data を持たない。
func processCreationXML(computer, at, recordID, pid, parentPid, name, commandLine string) string {
	data := `<Data Name="NewProcessId">` + pid + `</Data><Data Name="NewProcessName">` + name +
		`</Data><Data Name="ProcessId">` + parentPid + `</Data>`
	if commandLine != "" {
		data += `<Data Name="CommandLine">` + commandLine + `</Data>`
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4688</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>Security</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData>` + data + `</EventData></Event>` + "\n"
}

// 2 台の Computer を持つ file。host-a のプロセス 0x10 と、host-b でプロセス番号 0x10 を親に
// 持つプロセス 0x11 がある。2 つを 1 台の端末に置くと、端末をまたぐ親子の候補ができる。
var twoComputersDocument = "<Events>\n" +
	processCreationXML("host-a.example.test", "2001-02-03T04:05:06Z", "401", "0x10", "0x1",
		`C:\Example\a-parent.exe`, "a-parent.exe") +
	processCreationXML("host-b.example.test", "2001-02-03T04:05:07Z", "402", "0x11", "0x10",
		`C:\Example\b-child.exe`, "b-child.exe") +
	"</Events>\n"

// windowsEventImportResult は XML の文書を 1 つずつ別の収集元として取り込む。
func windowsEventImportResult(t *testing.T, documents ...string) ImportResult {
	t.Helper()
	var sources []scannedSource
	var statuses []core.ImportStatus
	for index, document := range documents {
		suffix := strconv.Itoa(index + 1)
		scanned := scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil),
			"events-"+suffix+".xml", WindowsEventXMLFormatKey, document)
		sources = append(sources, scanned)
		statuses = append(statuses, settleStatus(t, scanned, "winevent-"+suffix))
	}
	result, err := newImportResult(sources, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// processNodeLabelled は、その実行ファイルの path を表示名に持つプロセスのノードの位置を返す。
func processNodeLabelled(t *testing.T, graph Graph, path string) int {
	t.Helper()
	for at, node := range graph.nodes {
		if raw, _ := node.label.RawTextValue(); node.key.Kind == core.NodeKindProcess && raw == path {
			return at
		}
	}
	t.Fatalf("the graph holds no process labelled %q", path)
	return -1
}

func TestWindowsEventParserIdentity(t *testing.T) {
	identity := NewTestParser(WindowsEventXMLFormatKey, nil).Identity()
	if identity.FormatKey != WindowsEventXMLFormatKey || identity.PositionKind != core.PositionKindByteRange {
		t.Errorf("identity = %q %q, want %q byte_range", identity.FormatKey, identity.PositionKind,
			WindowsEventXMLFormatKey)
	}
	if identity.RecordedByOneTerminal || !identity.RecordingTerminalPerHostname {
		t.Errorf("RecordedByOneTerminal = %v, RecordingTerminalPerHostname = %v; want false and true",
			identity.RecordedByOneTerminal, identity.RecordingTerminalPerHostname)
	}
	if err := identity.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
	// 1 台の端末が書く形式と、ホスト名ごとに端末を分ける形式を同時に名乗る宣言を拒む。
	identity.RecordedByOneTerminal = true
	if err := identity.Validate(); err == nil {
		t.Error("Validate() accepted a format both recorded by one terminal and separated by hostname")
	}
}

// 壊れた 1 件は位置と原文を持つ失敗になり、前後の件はレコードになる。
func TestWindowsEventScanKeepsTheRecordsAroundABrokenEvent(t *testing.T) {
	scanned := scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil),
		"host03-security.xml", WindowsEventXMLFormatKey, windowsEventScanDocument)
	if len(scanned.Records) != 2 || len(scanned.Failures) != 1 {
		t.Fatalf("records = %d, failures = %d; want 2 and 1", len(scanned.Records), len(scanned.Failures))
	}
	brokenStart := strings.Index(windowsEventScanDocument, "<Event><System><EventRecordID>302")
	secondStart := strings.LastIndex(windowsEventScanDocument, "<Event>")
	failed := scanned.Failures[0]
	ref := failed.Failure.RecordRef
	if ref == nil || ref.PositionKind != core.PositionKindByteRange || ref.ByteOffset == nil ||
		*ref.ByteOffset != int64(brokenStart) || ref.ByteLength == nil ||
		*ref.ByteLength != int64(secondStart-brokenStart) || ref.LineNumber == nil || *ref.LineNumber != 8 {
		t.Errorf("RecordRef = %+v, want byte range %d+%d at line 8", ref, brokenStart, secondStart-brokenStart)
	}
	if failed.RawText == nil || *failed.RawText != windowsEventScanDocument[brokenStart:secondStart] {
		t.Errorf("failed RawText = %v, want the broken element", failed.RawText)
	}
	last := scanned.Records[1]
	if last.Locator.ByteOffset == nil || *last.Locator.ByteOffset != int64(secondStart) {
		t.Errorf("the record after the broken element starts at %v, want %d", last.Locator.ByteOffset, secondStart)
	}
	if last.ObservedAt == nil || last.ObservedAt.OffsetState != core.OffsetStateUndetermined {
		t.Errorf("ObservedAt = %+v, want a time without an offset", last.ObservedAt)
	}
}

// Computer が収集元の端末の表示名になり、プロセスの作成が端末の範囲のプロセスを組む。
func TestWindowsEventGraphNamesTheTerminalByComputer(t *testing.T) {
	result := windowsEventImportResult(t, windowsEventScanDocument)
	graph := NewGraph(result, AllMatchConditions())
	sha := result.publications[0].status.Scope.SourceContentSha256
	terminalKey, _ := core.RecordingHostTerminalNodeKey(sha, "host03.example.test")
	terminalAt := requireNodeAt(t, graph, terminalKey)
	if raw, _ := graph.nodes[terminalAt].label.RawTextValue(); raw != "host03.example.test" {
		t.Errorf("the terminal label is %+v, want the Computer", graph.nodes[terminalAt].label)
	}
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	if !hasEdge(graph, core.EdgeKindRanOn, childAt, terminalAt) {
		t.Error("the process has no ran_on edge to the terminal of its Computer")
	}
}

// Computer ごとに端末を分ける。別の端末のプロセスは、その Computer の端末で動き、端末を
// またぐ親子の候補を持たない。
func TestWindowsEventGraphSeparatesTheTerminalsOfAFile(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument)
	graph := NewGraph(result, AllMatchConditions())
	sha := result.publications[0].status.Scope.SourceContentSha256
	hostA, _ := core.RecordingHostTerminalNodeKey(sha, "host-a.example.test")
	hostB, _ := core.RecordingHostTerminalNodeKey(sha, "host-b.example.test")
	hostAAt, hostBAt := requireNodeAt(t, graph, hostA), requireNodeAt(t, graph, hostB)
	parentAt := processNodeLabelled(t, graph, `C:\Example\a-parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\b-child.exe`)
	if !hasEdge(graph, core.EdgeKindRanOn, parentAt, hostAAt) || !hasEdge(graph, core.EdgeKindRanOn, childAt, hostBAt) {
		t.Error("a process has no ran_on edge to the terminal of its Computer")
	}
	if hasEdge(graph, core.EdgeKindRanOn, parentAt, hostBAt) || hasEdge(graph, core.EdgeKindRanOn, childAt, hostAAt) {
		t.Error("a process ran on the terminal of another Computer")
	}
	if edges := lineageParentChildEdges(graph); len(edges) != 0 {
		t.Errorf("the graph holds parent-child edges %+v across two terminals", edges)
	}
	unknown, _ := core.RecordingTerminalNodeKey(sha)
	if _, present := graph.nodeAt[nodeIdOf(unknown)]; present {
		t.Error("the graph holds one terminal for the whole file")
	}
}

// 分析者が収集元に端末を 1 台指定すると、2 つの Computer のレコードが 1 台の端末に戻る。
func TestWindowsEventAssignmentJoinsTheTerminalsOfAFile(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument)
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
	parentAt := processNodeLabelled(t, graph, `C:\Example\a-parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\b-child.exe`)
	if !hasEdge(graph, core.EdgeKindRanOn, parentAt, terminalAt) || !hasEdge(graph, core.EdgeKindRanOn, childAt, terminalAt) {
		t.Error("a process has no ran_on edge to the specified terminal")
	}
	if !hasEdge(graph, core.EdgeKindProcessParentChild, parentAt, childAt) {
		t.Error("the processes on the specified terminal have no parent-child edge")
	}
}

// 1 台にまとめた端末のレコードが 2 つ以上の Computer を名乗り、割当が表示名を持つときだけ、
// 割当の表示名が端末の表示名になる。Computer の値は端末の属性として残る。
func TestWindowsEventAssignedTerminalNameLabelsAJoinedTerminal(t *testing.T) {
	for _, test := range []struct {
		name       string
		document   string
		terminalId string
		hostname   string
		// wantRaw は表示名の原資料の文字列である。空なら割当の表示名 (導いた値) を期待する。
		wantRaw string
	}{
		{"two Computers with a name", twoComputersDocument, "", lineageTerminalHostname, ""},
		{"two Computers with an id and a name", twoComputersDocument, lineageTerminalId, lineageTerminalHostname, ""},
		{"one Computer with a name", sameComputerSecondDocument, "", lineageTerminalHostname, "host-a.example.test"},
		{"two Computers without a name", twoComputersDocument, "", "", "host-a.example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := windowsEventImportResult(t, test.document)
			assignment := analystAssignmentFor(t, result)
			assignment.TerminalId, assignment.TerminalHostname = test.terminalId, test.hostname
			graph := NewGraph(result.WithAnalystTerminalAssignments(
				[]core.TerminalAssignment{assignment}), AllMatchConditions())
			terminalKey, _ := assignment.TerminalNodeKey()
			node := graph.nodes[requireNodeAt(t, graph, terminalKey)]
			label := node.label
			if test.wantRaw != "" {
				assertComputerLabel(t, label, test.wantRaw)
				return
			}
			assertAssignedTerminalLabel(t, label, test.hostname, assignment.Derivation)
			for _, computer := range []string{"host-a.example.test", "host-b.example.test"} {
				if !slices.ContainsFunc(node.attributes, func(attribute graphAttribute) bool {
					raw, _ := attribute.field.Text.RawTextValue()
					return attribute.field.Semantic == core.SemanticKeyTerminalHostname &&
						attribute.field.Text.ValueState == core.ValueStatePresent && raw == computer
				}) {
					t.Errorf("the terminal lost the Computer %q read from the source", computer)
				}
			}
		})
	}
}

// assertAssignedTerminalLabel は、端末の表示名が割当の表示名 (derived) であり、導き方が
// 割当の筋道と置き換えの理由を持つことを確かめる。
func assertAssignedTerminalLabel(t *testing.T, label core.RawAndNormalized, hostname, derivation string) {
	t.Helper()
	want := "分析者が与えた端末の割当: " + derivation +
		"。この端末のレコードが 2 つ以上のホスト名を名乗るため、割当の表示名を端末の表示名にする"
	if label.ValueState != core.ValueStateDerived || label.Normalized == nil ||
		*label.Normalized != hostname || label.Derivation == nil || *label.Derivation != want {
		t.Errorf("the terminal label is %+v, want the name %q derived as %q", label, hostname, want)
	}
}

// 同じ端末を指す割当が異なる表示名を持つときは、どれにも選ばず、原資料の Computer を
// 表示名に保つ。同じ収集元の 2 件の割当と、同じ端末の識別子を指す 2 つの収集元の割当の
// どちらでも、取り込みの順と割当の並びに依らない。
func TestWindowsEventConflictingAssignedNamesKeepTheComputer(t *testing.T) {
	const one, two = "terminal-one.example.test", "terminal-two.example.test"
	for _, reversed := range []bool{false, true} {
		t.Run("one source, reversed="+strconv.FormatBool(reversed), func(t *testing.T) {
			result := windowsEventImportResult(t, twoComputersDocument)
			assignments := []core.TerminalAssignment{
				sessionAssignment(t, result, 0, "192.0.2.10", "", one, true),
				sessionAssignment(t, result, 0, "192.0.2.10", "", two, true),
			}
			if reversed {
				slices.Reverse(assignments)
			}
			graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
			key, _ := assignments[0].TerminalNodeKey()
			assertComputerLabel(t, graph.nodes[requireNodeAt(t, graph, key)].label, "host-a.example.test")
		})
		for _, swapped := range []bool{false, true} {
			t.Run("two sources, reversed="+strconv.FormatBool(reversed)+
				", swapped="+strconv.FormatBool(swapped), func(t *testing.T) {
				result, assignments := twoSourcesOfOneTerminal(t, one, two, "", swapped)
				if reversed {
					slices.Reverse(assignments)
				}
				graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
				key, _ := core.TerminalNodeKey(lineageTerminalId)
				assertComputerLabel(t, graph.nodes[requireNodeAt(t, graph, key)].label, "host-a.example.test")
			})
		}
	}
}

// 2 つの収集元の割当が同じ端末を同じ表示名で指すときは、その表示名を端末の表示名にする。
// 筋道が違うときは、取り込みの順と割当の並びに依らず、文字列の順で先の筋道を導き方に持つ。
func TestWindowsEventAgreeingAssignedNamesOfTwoSourcesLabelTheTerminal(t *testing.T) {
	const name = "terminal-one.example.test"
	for _, reversed := range []bool{false, true} {
		for _, swapped := range []bool{false, true} {
			t.Run("reversed="+strconv.FormatBool(reversed)+", swapped="+strconv.FormatBool(swapped),
				func(t *testing.T) {
					result, assignments := twoSourcesOfOneTerminal(t, name, name, "別の筋道", swapped)
					if reversed {
						slices.Reverse(assignments)
					}
					graph := NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
					key, _ := core.TerminalNodeKey(lineageTerminalId)
					assertAssignedTerminalLabel(t, graph.nodes[requireNodeAt(t, graph, key)].label,
						name, "別の筋道")
				})
		}
	}
}

// twoSourcesOfOneTerminal は、2 つの Computer を持つ収集元と 1 つの Computer を持つ収集元を
// 取り込み、両方の収集元に同じ端末の識別子を指す割当を 1 件ずつ付ける。firstName は 2 つの
// Computer を持つ収集元の割当の表示名である。secondDerivation が空でなければ、もう一方の
// 割当の筋道にする。swapped が真なら、1 つの Computer を持つ収集元を先に取り込む。
func twoSourcesOfOneTerminal(
	t *testing.T, firstName, secondName, secondDerivation string, swapped bool,
) (ImportResult, []core.TerminalAssignment) {
	t.Helper()
	documents := []string{twoComputersDocument, sameComputerSecondDocument}
	names := []string{firstName, secondName}
	if swapped {
		slices.Reverse(documents)
		slices.Reverse(names)
	}
	result := windowsEventImportResult(t, documents...)
	assignments := make([]core.TerminalAssignment, len(documents))
	for index := range documents {
		assignments[index] = sessionAssignment(t, result, index, "192.0.2.10",
			lineageTerminalId, names[index], true)
	}
	if secondDerivation != "" {
		second := 1
		if swapped {
			second = 0
		}
		assignments[second].Derivation = secondDerivation
		if err := assignments[second].Validate(); err != nil {
			t.Fatalf("building the second assignment: %v", err)
		}
	}
	return result, assignments
}

// assertComputerLabel は、端末の表示名が原資料から読んだ Computer であることを確かめる。
func assertComputerLabel(t *testing.T, label core.RawAndNormalized, computer string) {
	t.Helper()
	if raw, _ := label.RawTextValue(); raw != computer || label.ValueState != core.ValueStatePresent {
		t.Errorf("the terminal label is %+v, want the Computer %q read from the source", label, computer)
	}
}

// UTC からのずれを持たない時刻のプロセスは壁時計の日時で並べ、同じ時計の親だけを選ぶ。
// ずれを持つ時刻の親と前後を比べない。
func TestWindowsEventLineageKeepsLocalAndAbsoluteTimesApart(t *testing.T) {
	document := "<Events>\n" +
		processCreationXML("host-a.example.test", "2001-02-03T04:05:06Z", "601", "0x10", "0x1",
			`C:\Example\absolute-parent.exe`, "") +
		processCreationXML("host-a.example.test", "2001-02-03 04:05:07", "602", "0x11", "0x10",
			`C:\Example\local-child-of-absolute.exe`, "") +
		processCreationXML("host-a.example.test", "2001-02-03 04:05:06", "603", "0x20", "0x1",
			`C:\Example\local-parent.exe`, "") +
		processCreationXML("host-a.example.test", "2001-02-03 04:05:07", "604", "0x21", "0x20",
			`C:\Example\local-child.exe`, "") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	localParent := processNodeLabelled(t, graph, `C:\Example\local-parent.exe`)
	localChild := processNodeLabelled(t, graph, `C:\Example\local-child.exe`)
	if !hasEdge(graph, core.EdgeKindProcessParentChild, localParent, localChild) {
		t.Error("the processes of local times have no parent-child edge")
	}
	absoluteParent := processNodeLabelled(t, graph, `C:\Example\absolute-parent.exe`)
	mixedChild := processNodeLabelled(t, graph, `C:\Example\local-child-of-absolute.exe`)
	if hasEdge(graph, core.EdgeKindProcessParentChild, absoluteParent, mixedChild) {
		t.Error("a local time was compared with an instant to choose the parent")
	}
}

// CommandLine を持たないプロセスの作成も起動である。番号が再利用されたプロセスを 1 つに
// まとめない。
func TestWindowsEventProcessCreationWithoutCommandLineStartsAProcess(t *testing.T) {
	document := "<Events>\n" +
		processCreationXML("host-a.example.test", "2001-02-03T04:05:06Z", "501", "0x20", "0x1",
			`C:\Example\first.exe`, "") +
		processCreationXML("host-a.example.test", "2001-02-03T04:05:09Z", "502", "0x20", "0x1",
			`C:\Example\second.exe`, "") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	if nodes := processNodesOfPid(t, graph, "32"); len(nodes) != 2 {
		t.Errorf("the reused process number has nodes %v, want one per creation", nodes)
	}
	firstAt := processNodeLabelled(t, graph, `C:\Example\first.exe`)
	if len(graph.nodes[firstAt].creationRecords) == 0 {
		t.Error("the process carries no creation record")
	}
}

// EVTX の原文は読み取りが組み立てた XML であり、XML と CSV の file の原文は収集元の byte 列
// である。XML と EVTX は Computer ごとの端末、CSV は収集元ごとの端末に置く。
func TestWindowsEventParsersDeclareWhetherTheRawTextIsConverted(t *testing.T) {
	evtx := NewTestParser(WindowsEVTXFormatKey, nil).Identity()
	if evtx.FormatKey != WindowsEVTXFormatKey || evtx.PositionKind != core.PositionKindByteRange ||
		!evtx.RawTextConverted || evtx.ParserID == "" || !evtx.RecordingTerminalPerHostname ||
		evtx.RecordedByOneTerminal {
		t.Errorf("EVTX identity = %+v, want byte_range with the converted raw text per Computer", evtx)
	}
	xml := NewTestParser(WindowsEventXMLFormatKey, nil).Identity()
	if xml.RawTextConverted || xml.ParserID == evtx.ParserID || !xml.RecordingTerminalPerHostname {
		t.Errorf("XML identity = %+v, want the source bytes as the raw text and its own parser", xml)
	}
	csv := NewTestParser(WindowsEventCSVFormatKey, nil).Identity()
	if csv.RawTextConverted || csv.ParserID == evtx.ParserID || !csv.RecordedByOneTerminal ||
		csv.RecordingTerminalPerHostname {
		t.Errorf("CSV identity = %+v, want the source bytes as the raw text on one terminal", csv)
	}
	for _, identity := range []ParserIdentity{evtx, xml, csv} {
		if err := identity.Validate(); err != nil {
			t.Errorf("Validate(%s) = %v", identity.FormatKey, err)
		}
	}
}

// byteRangeRawTextRef は byte 範囲で指すレコードの原文への参照を返す。
func byteRangeRawTextRef(locator core.RecordLocator, _ string) string {
	if locator.ByteOffset == nil {
		return ""
	}
	return "raw:bytes:" + strconv.FormatInt(*locator.ByteOffset, 10)
}

// EVTX の見出しを持たない収集元は、行番号を持たない byte 範囲の失敗 1 件になる。
func TestWindowsEventEVTXScanLocatesAFailureByByteRange(t *testing.T) {
	const input = "not an event log file"
	scanned := scanIndexSource(t, NewTestParser(WindowsEVTXFormatKey, nil), "events.evtx", WindowsEVTXFormatKey, input)
	if len(scanned.Records) != 0 || len(scanned.Failures) != 1 {
		t.Fatalf("records = %d, failures = %d; want 0 and 1", len(scanned.Records), len(scanned.Failures))
	}
	ref := scanned.Failures[0].Failure.RecordRef
	if ref == nil || ref.PositionKind != core.PositionKindByteRange || *ref.ByteOffset != 0 ||
		*ref.ByteLength != int64(len(input)) || ref.LineNumber != nil || ref.LineCount != nil {
		t.Errorf("RecordRef = %+v, want the byte range of the whole input without lines", ref)
	}
	if len(scanned.FileHeader) != 0 {
		t.Errorf("FileHeader = %+v, want none for a file without the header", scanned.FileHeader)
	}
	status, err := buildImportStatus(scanned, "run", "parser-v1", "evtx", settleSanitize, byteRangeRawTextRef)
	if err != nil {
		t.Fatalf("buildImportStatus() = %v", err)
	}
	if len(status.Failures) != 1 || status.Failures[0].Validate() != nil || status.Failures[0].LineNumber != nil {
		t.Errorf("failures = %+v, want 1 valid failure without a line number", status.Failures)
	}
}

// byteRangeParser は行番号を持たない byte 範囲のレコード 1 件と、file の見出しを返す。
type byteRangeParser struct {
	returned bool
}

func (p *byteRangeParser) Identity() ParserIdentity {
	return ParserIdentity{ParserID: "byte-range", FormatKey: "byte_range_test",
		PositionKind: core.PositionKindByteRange, RawTextConverted: true}
}

func (p *byteRangeParser) Reset(input io.Reader) {
	p.returned = false
	_, _ = io.Copy(io.Discard, input)
}

func (p *byteRangeParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	if p.returned {
		return ParsedRecord{}, nil, io.EOF
	}
	p.returned = true
	length := int64(24)
	return ParsedRecord{RawText: "<Event></Event>", ByteOffset: 8, ByteLength: &length}, nil, nil
}

func (p *byteRangeParser) SourceHeader() []core.RecordField {
	value, _ := core.NewRawValue(core.ValueStatePresent, "7")
	field, _ := core.NewTextField("NextRecordID", "", value)
	return []core.RecordField{field}
}

// 行を持たない形式のレコードは byte 範囲だけで指し、原文の変換と file の見出しは収集元の
// 識別に載る。
func TestScanLocatesARecordWithoutLinesAndCarriesTheFileHeader(t *testing.T) {
	scanned := scanIndexSource(t, &byteRangeParser{}, "records.bin", "byte_range_test", strings.Repeat("\x00", 32))
	if len(scanned.Records) != 1 {
		t.Fatalf("records = %d, failures = %+v; want 1 record", len(scanned.Records), scanned.Failures)
	}
	locator := scanned.Records[0].Locator
	if locator.PositionKind != core.PositionKindByteRange || *locator.ByteOffset != 8 || *locator.ByteLength != 24 ||
		locator.LineNumber != nil {
		t.Errorf("locator = %+v, want the byte range without a line number", locator)
	}
	identity := sourceIdentity(scanned, "source")
	if !identity.RawTextConverted || len(identity.FileHeader) != 1 || identity.FileHeader[0].Name != "NextRecordID" {
		t.Errorf("identity = %+v, want the converted raw text and the file header", identity)
	}
	// scanIndexSource の計画は取得元を持たない。
	identity.OriginPath = "records.bin"
	if err := identity.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}

// 2 つ目の収集元。1 つ目の収集元 (twoComputersDocument) と同じ Computer の名前 host-a を持つ。
// プロセスの作成と、対象を指さない 1 件がある。
var sameComputerSecondDocument = "<Events>\n" +
	processCreationXML("host-a.example.test", "2001-02-03T04:06:06Z", "601", "0x30", "0x1",
		`C:\Example\second-file.exe`, "second-file.exe") +
	`<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:06:07Z"/><EventRecordID>602</EventRecordID>` +
	`<Channel>Application</Channel><Computer>host-a.example.test</Computer></System></Event>` + "\n" +
	"</Events>\n"

// hostTerminalIdOf は収集元の中のホスト名の端末のノードの識別子を返す。
func hostTerminalIdOf(t *testing.T, result ImportResult, source int, hostname string) string {
	t.Helper()
	key, built := core.RecordingHostTerminalNodeKey(
		result.publications[source].status.Scope.SourceContentSha256, hostname)
	if !built {
		t.Fatalf("building the terminal key of %q", hostname)
	}
	return nodeIdOf(key)
}

// 同じ Computer の名前を持つ 2 つの収集元は、収集元ごとに別の端末になる。
func TestWindowsEventGraphKeepsTheSameComputerOfTwoSourcesApart(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument, sameComputerSecondDocument)
	graph := NewGraph(result, AllMatchConditions())
	first, second := hostTerminalIdOf(t, result, 0, "host-a.example.test"),
		hostTerminalIdOf(t, result, 1, "host-a.example.test")
	if first == second {
		t.Fatal("the two sources share one terminal key for the same Computer")
	}
	for _, id := range []string{first, second, hostTerminalIdOf(t, result, 0, "host-b.example.test")} {
		if !graph.IsTerminalNode(id) {
			t.Errorf("the graph holds no terminal %s", id)
		}
	}
	matched := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}})
	terminals := 0
	for _, node := range matched.Nodes {
		if node.Kind == core.NodeKindTerminal {
			terminals++
		}
	}
	if terminals != 3 {
		t.Errorf("the terminal query returns %d terminals, want the 2 of the first source and 1 of the second", terminals)
	}
}

// 対象を指さないレコードだけを持つ Computer も、収集元の中の端末になり、時系列の行と
// 絞り込みがその端末を指せる。
func TestWindowsEventGraphHoldsTheTerminalOfAComputerWithoutProcesses(t *testing.T) {
	document := "<Events>\n" +
		`<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:07:07Z"/><EventRecordID>701</EventRecordID>` +
		`<Channel>System</Channel><Computer>host-c.example.test</Computer></System></Event>` + "\n" +
		"</Events>\n"
	result := windowsEventImportResult(t, document)
	graph := NewGraph(result, AllMatchConditions())
	terminal := hostTerminalIdOf(t, result, 0, "host-c.example.test")
	if !graph.IsTerminalNode(terminal) {
		t.Fatal("the graph holds no terminal for the Computer of the record")
	}
	filtered := graph.Timeline(TimelineQuery{RecordFilter: RecordFilter{Terminal: terminal}})
	if len(filtered.Entries) != 1 || filtered.Entries[0].Terminal == nil || filtered.Entries[0].Terminal.Id != terminal {
		t.Errorf("the timeline filtered by the terminal = %+v, want the one record on the terminal", filtered.Entries)
	}
}

// ホスト名の端末で絞ると、その収集元でそのホスト名を名乗ったレコードだけが残る。
func TestWindowsEventRecordFilterSelectsTheRecordsOfAHostTerminal(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument, sameComputerSecondDocument)
	graph := NewGraph(result, AllMatchConditions())
	second := hostTerminalIdOf(t, result, 1, "host-a.example.test")
	timeline := graph.Timeline(TimelineQuery{RecordFilter: RecordFilter{Terminal: second}})
	secondSource := result.publications[1].status.SourceId
	if len(timeline.Entries) != len(result.publications[1].records) {
		t.Fatalf("the filter keeps %d records, want the %d records of the second source",
			len(timeline.Entries), len(result.publications[1].records))
	}
	for _, entry := range timeline.Entries {
		if entry.RecordRef.SourceId != secondSource {
			t.Errorf("the filter keeps a record of %s", entry.RecordRef.SourceId)
		}
	}
}

// 時系列の行は、レコードを置いたホスト名の端末を出す。
func TestWindowsEventTimelineNamesTheHostTerminalOfARecord(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument, sameComputerSecondDocument)
	graph := NewGraph(result, AllMatchConditions())
	want := map[string]string{
		result.publications[0].status.SourceId + ":401": hostTerminalIdOf(t, result, 0, "host-a.example.test"),
		result.publications[0].status.SourceId + ":402": hostTerminalIdOf(t, result, 0, "host-b.example.test"),
		result.publications[1].status.SourceId + ":601": hostTerminalIdOf(t, result, 1, "host-a.example.test"),
		result.publications[1].status.SourceId + ":602": hostTerminalIdOf(t, result, 1, "host-a.example.test"),
	}
	timeline := graph.Timeline(TimelineQuery{})
	if len(timeline.Entries) != len(want) {
		t.Fatalf("the timeline holds %d entries, want %d", len(timeline.Entries), len(want))
	}
	for _, entry := range timeline.Entries {
		record, _ := graph.recordAtLocator(entry.RecordRef)
		key := entry.RecordRef.SourceId + ":" + recordIDOf(t, graph, record)
		if entry.Terminal == nil || entry.Terminal.Id != want[key] {
			t.Errorf("the entry %s names the terminal %+v, want %s", key, entry.Terminal, want[key])
		}
	}
}

// recordIDOf は根拠のレコードの EventRecordID を返す。
func recordIDOf(t *testing.T, graph Graph, at int) string {
	t.Helper()
	node := graph.nodes[graph.records[at].recordNode]
	for _, attribute := range node.attributes {
		if attribute.field.Name == "EventRecordID" {
			raw, _ := attribute.field.Text.RawTextValue()
			return raw
		}
	}
	t.Fatalf("the record %d carries no EventRecordID", at)
	return ""
}
