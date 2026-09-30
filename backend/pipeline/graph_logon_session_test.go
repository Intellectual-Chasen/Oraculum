// in-package test: Logon ID で結んだログオンと操作の候補のエッジと、結ばなかった理由を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// securityEventXML は Security の監査のイベント 1 件を返す。data は Name と値の組を
// 交互に並べる。
func securityEventXML(recordID, eventID, computer, systemTime string, data ...string) string {
	var elements strings.Builder
	for at := 0; at+1 < len(data); at += 2 {
		elements.WriteString(`<Data Name="` + data[at] + `">` + data[at+1] + `</Data>`)
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>` + eventID +
		`</EventID><TimeCreated SystemTime="` + systemTime + `"/><EventRecordID>` + recordID +
		`</EventRecordID><Channel>Security</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData>` + elements.String() + `</EventData></Event>` + "\n"
}

// 端末の名前と Logon ID。Logon ID は 0 を詰めた文字列と詰めない文字列で同じ値を書く。
const (
	logonHostA        = "host-a.example.test"
	logonHostB        = "host-b.example.test"
	firstSessionLong  = "0x00000000000A0001"
	firstSessionShort = "0xa0001"
	secondSession     = "0x00000000000A0002"
)

// logonSessionEvents は 1 つの file に並べるイベントと、各イベントの記録番号である。
//
// 31 は端末 A の対話のログオン (種別 2) で、SYSTEM のセッションが要求した。32 と 33 はその
// セッションの操作である。34 はそのセッションが要求した、別の資格情報のログオン (種別 9) で
// あり、35 はその新しいセッションの操作である。36 は同じ Logon ID を端末 B で書き、37 は
// 新しいセッションの Logon ID を、そのログオンより前の時刻に書く。38 は端末 A を起動し
// 直した後に、31 と同じ Logon ID のログオンを記録し、39 はその後の操作である。40 は SYSTEM の
// セッションのログオン、41 は SYSTEM のセッションの操作である。42 は UTC からのずれを持たない
// 時刻で 31 と同じ Logon ID を書く。
var logonSessionEvents = []struct {
	recordID string
	xml      string
}{
	{"31", securityEventXML("31", "4624", logonHostA, "2001-02-03T04:05:10.000Z",
		"SubjectLogonId", "0x3e7", "TargetLogonId", firstSessionLong, "LogonType", "2")},
	{"32", securityEventXML("32", "4672", logonHostA, "2001-02-03T04:05:10.000Z",
		"SubjectLogonId", firstSessionLong)},
	{"33", securityEventXML("33", "4688", logonHostA, "2001-02-03T04:05:20.000Z",
		"SubjectLogonId", firstSessionShort, "NewProcessId", "0x2b", "NewProcessName", `C:\Example\tool.exe`)},
	{"34", securityEventXML("34", "4624", logonHostA, "2001-02-03T04:05:30.000Z",
		"SubjectLogonId", firstSessionLong, "TargetLogonId", secondSession, "LogonType", "9")},
	{"35", securityEventXML("35", "4648", logonHostA, "2001-02-03T04:05:40.000Z",
		"SubjectLogonId", secondSession)},
	{"36", securityEventXML("36", "4672", logonHostB, "2001-02-03T04:05:50.000Z",
		"SubjectLogonId", firstSessionLong)},
	{"37", securityEventXML("37", "4672", logonHostA, "2001-02-03T04:05:05.000Z",
		"SubjectLogonId", secondSession)},
	{"38", securityEventXML("38", "4624", logonHostA, "2001-02-03T04:06:00.000Z",
		"SubjectLogonId", "0x3e7", "TargetLogonId", firstSessionLong, "LogonType", "2")},
	{"39", securityEventXML("39", "4720", logonHostA, "2001-02-03T04:06:10.000Z",
		"SubjectLogonId", firstSessionLong)},
	{"40", securityEventXML("40", "4624", logonHostA, "2001-02-03T04:04:00.000Z",
		"SubjectLogonId", "0x0", "TargetLogonId", "0x00000000000003E7", "LogonType", "0")},
	{"41", securityEventXML("41", "4672", logonHostA, "2001-02-03T04:06:20.000Z",
		"SubjectLogonId", "0x3e7")},
	{"42", securityEventXML("42", "4672", logonHostA, "2001-02-03 04:06:30.000",
		"SubjectLogonId", firstSessionLong)},
}

// logonSessionGraph は logonSessionEvents を 1 つの file に並べてグラフを組み、記録番号ごとの
// レコードのノードの位置を返す。
func logonSessionGraph(t *testing.T) (Graph, map[string]int) {
	t.Helper()
	var document strings.Builder
	document.WriteString("<Events>\n")
	offsets := make(map[int64]string, len(logonSessionEvents))
	for _, event := range logonSessionEvents {
		offsets[int64(document.Len())] = event.recordID
		document.WriteString(event.xml)
	}
	document.WriteString("</Events>\n")
	graph := NewGraph(windowsEventImportResult(t, document.String()), AllMatchConditions())
	nodes := make(map[string]int, len(offsets))
	for _, record := range graph.records {
		if record.locator.ByteOffset == nil || !record.hasRecordNode {
			continue
		}
		if recordID, found := offsets[*record.locator.ByteOffset]; found {
			nodes[recordID] = record.recordNode
		}
	}
	if len(nodes) != len(logonSessionEvents) {
		t.Fatalf("the graph holds record nodes for %d of the events", len(nodes))
	}
	return graph, nodes
}

// recordIdOfNode はレコードのノードの位置から記録番号を求める。
func recordIdOfNode(t *testing.T, nodes map[string]int, at int) string {
	t.Helper()
	for recordID, node := range nodes {
		if node == at {
			return recordID
		}
	}
	t.Fatalf("the node %d is none of the event records", at)
	return ""
}

// ログオンから、同じ端末でそのログオンより前でない時刻の、同じ Logon ID の操作へ候補を結ぶ。
// ログオンを要求したセッションのログオンは、そのセッションの操作として連鎖する。同じ端末に
// 同じ Logon ID のログオンが 2 件あると、後の操作は両方と結ぶ。SYSTEM のセッションは結ばない。
func TestLogonSessionEdgesLinkTheLogonToLaterOperationsOnTheSameTerminal(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	var got []string
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindLogonSessionOperation {
			continue
		}
		if edge.state != core.RelationStateCandidate {
			t.Errorf("a logon session edge is %s, want candidate", edge.state)
		}
		logon, operation := recordIdOfNode(t, nodes, edge.source), recordIdOfNode(t, nodes, edge.target)
		got = append(got, logon+"->"+operation)
		// 関係の根拠は両側の元レコードである。
		var evidence []string
		for _, at := range edge.evidence {
			evidence = append(evidence, recordIdOfNode(t, nodes, graph.records[at].recordNode))
		}
		slices.Sort(evidence)
		if want := []string{logon, operation}; !slices.Equal(evidence, want) {
			t.Errorf("the edge %s->%s rests on %v, want %v", logon, operation, evidence, want)
		}
	}
	slices.Sort(got)
	want := []string{"31->32", "31->33", "31->34", "31->39", "34->35", "38->39"}
	if !slices.Equal(got, want) {
		t.Errorf("the logon session edges are %v, want %v", got, want)
	}
}

// Logon ID が一致しながら結ばなかったログオンを、操作のレコードのノードが理由とともに持つ。
func TestLogonSessionRejectionsNameTheLogonAndTheReason(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	want := map[string][]string{
		"32": {"38 logon_after_operation"},
		"33": {"38 logon_after_operation"},
		"34": {"38 logon_after_operation"},
		"35": nil,
		"36": {"31 other_terminal", "38 other_terminal"},
		"37": {"34 logon_after_operation"},
		"39": nil,
		"41": nil,
		"42": {"31 time_not_comparable", "38 time_not_comparable"},
	}
	for recordID, wantRejections := range want {
		detail, found := graph.NodeDetail(graph.nodes[nodes[recordID]].id)
		if !found {
			t.Fatalf("the record %s has no node detail", recordID)
		}
		var got []string
		for _, rejection := range detail.LogonSessionRejections {
			if err := rejection.Validate(); err != nil {
				t.Errorf("the record %s carries an invalid rejection: %v", recordID, err)
			}
			logonAt, indexed := graph.recordAtLocator(rejection.Logon.RecordRef)
			if !indexed {
				t.Fatalf("the rejection of %s names a record outside the graph", recordID)
			}
			got = append(got, recordIdOfNode(t, nodes, graph.records[logonAt].recordNode)+" "+string(rejection.Reason))
		}
		slices.Sort(got)
		if !slices.Equal(got, wantRejections) {
			t.Errorf("the record %s rejects %v, want %v", recordID, got, wantRejections)
		}
	}
}

// graph-markii-logon-session.log は、端末 T1 のログオン (401) と、同じ Logon ID を大文字で書いた
// 同じ時刻の特権の割り当て (402) と、同じ Logon ID を端末 T2 で書いた特権の割り当て (403) と、
// SYSTEM のセッションの特権の割り当て (404) を持つ。
//
// markii 形式の evtLogonID も、Windows イベントログと同じ条件で結ぶ。
func TestLogonSessionEdgesLinkMarkIIEventLogRecords(t *testing.T) {
	result, err := graphRunner(t).Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-logon-session.log",
		FileName:   "graph-markii-logon-session.log",
		FormatKey:  MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	var got []string
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindLogonSessionOperation {
			got = append(got, recordLabel(t, graph, edge.source)+"->"+recordLabel(t, graph, edge.target))
		}
	}
	if want := []string{"401->402"}; !slices.Equal(got, want) {
		t.Errorf("the logon session edges are %v, want %v", got, want)
	}
	for sequence, want := range map[string][]core.LogonSessionRejectionReason{
		"403": {core.LogonSessionRejectionOtherTerminal},
		"404": nil,
	} {
		detail, _ := graph.NodeDetail(graph.nodes[recordNodeWithSequence(t, graph, sequence)].id)
		var reasons []core.LogonSessionRejectionReason
		for _, rejection := range detail.LogonSessionRejections {
			reasons = append(reasons, rejection.Reason)
		}
		if !slices.Equal(reasons, want) {
			t.Errorf("the record %s rejects %v, want %v", sequence, reasons, want)
		}
	}
}

// recordNodeWithSequence は、通番の値を識別鍵に持つレコードのノードの位置を返す。
func recordNodeWithSequence(t *testing.T, graph Graph, sequence string) int {
	t.Helper()
	for at, node := range graph.nodes {
		if node.key.Kind != core.NodeKindRecord {
			continue
		}
		if value, _ := node.key.LabelValue(); value == sequence {
			return at
		}
	}
	t.Fatalf("the graph holds no record node at the sequence number %s", sequence)
	return -1
}

// recordLabel はレコードのノードの通番の値を返す。
func recordLabel(t *testing.T, graph Graph, at int) string {
	t.Helper()
	value, present := graph.nodes[at].key.LabelValue()
	if graph.nodes[at].key.Kind != core.NodeKindRecord || !present {
		t.Fatalf("the node %d is not a record node", at)
	}
	return value
}

// 関係の詳細の根拠の区分が、ログオンの種別を持つ。対話のログオンと別の資格情報のログオンを
// 種別で見分けられる。
func TestLogonSessionEdgeDetailCarriesTheLogonType(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	for _, item := range []struct {
		logon, operation, logonType string
	}{
		{"31", "33", "2"},
		{"34", "35", "9"},
	} {
		id := edgeIdOf(core.EdgeKindLogonSessionOperation,
			graph.nodes[nodes[item.logon]].id, graph.nodes[nodes[item.operation]].id)
		detail, found := graph.EdgeDetail(id, EdgeEvidenceFilter{})
		if !found {
			t.Fatalf("no edge from %s to %s", item.logon, item.operation)
		}
		var types []string
		for _, group := range detail.EvidenceGroups {
			if group.LogonType != nil {
				value, _ := group.LogonType.Text.ComparableValue()
				types = append(types, value)
			}
		}
		if !slices.Equal(types, []string{item.logonType}) {
			t.Errorf("the edge %s->%s carries the logon types %v, want %s",
				item.logon, item.operation, types, item.logonType)
		}
		if detail.SourceNode.Kind != core.NodeKindRecord || detail.TargetNode.Kind != core.NodeKindRecord {
			t.Errorf("the edge ends are %s and %s, want two records", detail.SourceNode.Kind, detail.TargetNode.Kind)
		}
	}
}
