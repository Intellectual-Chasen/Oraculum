// in-package test: 端末が始めた接続の記録と、端末が自分の IP を記録したイベントから組む
// 端末と IP アドレスの関係を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// dnsClientProvider は、端末の DNS の登録を記録するプロバイダの名前である。
const dnsClientProvider = "Microsoft-Windows-DNS-Client"

// dnsRegistrationXML は、端末 computer が自分の IP address と DNS サーバ dnsServer を記録した
// 8020 を返す。
func dnsRegistrationXML(recordID, computer, systemTime, address, dnsServer string) string {
	return taskEventXML(dnsClientProvider, recordID, "8020", computer, systemTime,
		"HostName", "host", "DnsServerList", dnsServer, "Sent UpdateServer", dnsServer+":53",
		"Ipaddress", address)
}

// outboundConnectionXML は、端末 computer が protocol で destination へ始めた接続を記録した
// 5156 を返す。
func outboundConnectionXML(recordID, computer, systemTime, destination, protocol string) string {
	return securityEventXML(recordID, "5156", computer, systemTime,
		"ProcessID", "1234", "Direction", "%%14593", "SourceAddress", "192.0.2.10", "SourcePort", "50100",
		"DestAddress", destination, "DestPort", "445", "Protocol", protocol)
}

// ipTargetsOfKind は、種別が kind で起点が source のエッジの終点の IP アドレスを並べて返す。
func ipTargetsOfKind(graph Graph, kind core.EdgeKind, source int) []string {
	var targets []string
	for _, edge := range graph.edges {
		if edge.kind != kind || edge.source != source || graph.nodes[edge.target].key.Kind != core.NodeKindIp {
			continue
		}
		targets = append(targets, graph.nodes[edge.target].key.Values[0].Value)
	}
	slices.Sort(targets)
	return targets
}

// 端末が始めた接続の記録は、記録した端末から接続先の IP へのエッジを作る。受けた接続、
// Sysmon の始めていない接続、マルチキャスト・ブロードキャスト・ループバック・未指定の
// 接続先からは作らない。
func TestOutboundConnectionRunsFromTheRecordingTerminalToTheDestination(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("11", "4648", logonHostA, "2001-02-03T04:05:00.000Z",
			"TargetUserName", "user-a", "TargetServerName", "host-b", "IpAddress", "192.0.2.20", "IpPort", "445") +
		outboundConnectionXML("12", logonHostA, "2001-02-03T04:05:01.000Z", "198.51.100.20", "6") +
		securityEventXML("13", "5156", logonHostA, "2001-02-03T04:05:02.000Z",
			"ProcessID", "1234", "Direction", "%%14592", "SourceAddress", "192.0.2.10", "SourcePort", "445",
			"DestAddress", "198.51.100.30", "DestPort", "50200", "Protocol", "6") +
		outboundConnectionXML("14", logonHostA, "2001-02-03T04:05:03.000Z", "233.252.0.1", "17") +
		outboundConnectionXML("15", logonHostA, "2001-02-03T04:05:04.000Z", "192.0.2.255", "17") +
		outboundConnectionXML("16", logonHostA, "2001-02-03T04:05:05.000Z", "255.255.255.255", "17") +
		outboundConnectionXML("17", logonHostA, "2001-02-03T04:05:06.000Z", "127.0.0.1", "6") +
		outboundConnectionXML("18", logonHostA, "2001-02-03T04:05:07.000Z", "0.0.0.0", "6") +
		taskEventXML("Microsoft-Windows-Sysmon", "19", "3", logonHostA, "2001-02-03T04:05:08.000Z",
			"ProcessId", "1234", "Initiated", "true", "SourceIp", "192.0.2.10", "SourcePort", "50101",
			"DestinationIp", "203.0.113.5", "DestinationPort", "443") +
		taskEventXML("Microsoft-Windows-Sysmon", "20", "3", logonHostA, "2001-02-03T04:05:09.000Z",
			"ProcessId", "1234", "Initiated", "false", "SourceIp", "203.0.113.6", "SourcePort", "50102",
			"DestinationIp", "192.0.2.10", "DestinationPort", "445") +
		"</Events>\n"
	result := windowsEventImportResult(t, document)
	graph := NewGraph(result, AllMatchConditions())
	host, _ := core.RecordingHostTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256, logonHostA)
	terminal, present := graph.nodeAt[nodeIdOf(host)]
	if !present {
		t.Fatal("the graph holds no terminal node for the recording host")
	}
	want := []string{"192.0.2.20", "198.51.100.20", "203.0.113.5"}
	if got := ipTargetsOfKind(graph, core.EdgeKindTerminalOutboundConnection, terminal); !slices.Equal(got, want) {
		t.Errorf("the outbound connections run to %v, want %v", got, want)
	}
	for _, edge := range edgesOfKind(graph, core.EdgeKindTerminalOutboundConnection) {
		if edge.source != terminal || edge.state != core.RelationStateObserved || len(edge.evidence) != 1 {
			t.Errorf("the outbound connection %s is %q from %d with %d evidence records, "+
				"want an observed relation from the terminal with 1", edge.id, edge.state, edge.source, len(edge.evidence))
		}
	}

	// 初期の画面の要求 (端末と IP アドレス、深さ 1) がこのエッジを辿る。
	subgraph := graph.Query(GraphQuery{Granularity: core.GraphGranularityObject, Depth: 1,
		NodeKinds: []core.NodeKind{core.NodeKindTerminal, core.NodeKindIp}})
	outbound := 0
	for _, edge := range subgraph.Edges {
		if edge.Kind == core.EdgeKindTerminalOutboundConnection {
			outbound++
		}
	}
	if outbound != len(want) {
		t.Errorf("the terminal and ip query returns %d outbound connections, want %d", outbound, len(want))
	}
}

// Mark II の接続の記録は外向きの接続になり、受け付けの記録はならない。
func TestMarkIIConnectBecomesAnOutboundConnection(t *testing.T) {
	document := "02/01/2000 10:00:01.100 +0900 sn=101 evt=net subEvt=con psGUID=p1 tmid=t com=TESTHOST csid=s psPath=app srcIP=192.0.2.10 srcPort=50002 dstIP=198.51.100.42 dstPort=8080\n" +
		"02/01/2000 10:00:02.100 +0900 sn=102 evt=net subEvt=acpt psGUID=p1 tmid=t com=TESTHOST csid=s psPath=app srcIP=198.51.100.43 srcPort=50003 dstIP=192.0.2.10 dstPort=445\n"
	graph := NewGraph(sessionSourcesResult(t, sessionSource{
		name: "ws.log", format: MarkIIFormatKey, document: document,
	}), AllMatchConditions())
	edges := edgesOfKind(graph, core.EdgeKindTerminalOutboundConnection)
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d outbound connections, want 1", len(edges))
	}
	if kind := graph.nodes[edges[0].source].key.Kind; kind != core.NodeKindTerminal {
		t.Errorf("the outbound connection runs from a %q node, want the terminal", kind)
	}
	if target := graph.nodes[edges[0].target].key.Values[0].Value; target != "198.51.100.42" {
		t.Errorf("the outbound connection runs to %q, want the connected destination", target)
	}
}

// 8020 が記録した自分の IP は、記録した端末からその IP への terminal_address になる。DNS
// サーバの IP はならない。自分の IP への接続は外向きの接続にならない。割当は作らない。
func TestDnsRegistrationRecordsTheAddressOfTheRecordingTerminal(t *testing.T) {
	document := "<Events>\n" +
		dnsRegistrationXML("21", logonHostA, "2001-02-03T04:05:00.000Z", "192.0.2.10", "192.0.2.53") +
		outboundConnectionXML("22", logonHostA, "2001-02-03T04:05:01.000Z", "192.0.2.10", "6") +
		outboundConnectionXML("23", logonHostA, "2001-02-03T04:05:02.000Z", "192.0.2.53", "17") +
		"</Events>\n"
	result := windowsEventImportResult(t, document)
	graph := NewGraph(result, AllMatchConditions())
	host, _ := core.RecordingHostTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256, logonHostA)
	terminal := graph.nodeAt[nodeIdOf(host)]
	if got := ipTargetsOfKind(graph, core.EdgeKindTerminalAddress, terminal); !slices.Equal(got, []string{"192.0.2.10"}) {
		t.Fatalf("the terminal holds the addresses %v, want the registered address", got)
	}
	addresses := edgesOfKind(graph, core.EdgeKindTerminalAddress)
	if len(addresses) != 1 || addresses[0].state != core.RelationStateObserved ||
		!slices.Equal(addresses[0].evidence, []int{recordOfEvent(t, graph, "21")}) {
		t.Errorf("the terminal address is %+v, want an observed relation from the 8020", addresses)
	}
	if got := ipTargetsOfKind(graph, core.EdgeKindTerminalOutboundConnection, terminal); !slices.Equal(got, []string{"192.0.2.53"}) {
		t.Errorf("the outbound connections run to %v, want only the DNS server", got)
	}
	if entries := terminalAssignmentsOf(result).entries; len(entries) != 0 {
		t.Errorf("the 8020 makes the assignments %+v, want none", entries)
	}
}

// 8020 の IP から別の file の端末へのログオンは、接続元が未同定の候補のまま残る。同じ file の
// 端末へのログオンは記録した端末自身からの接続であり、候補にならない。DNS サーバの IP からの
// ログオンは候補になる。
func TestDnsRegistrationKeepsTheUnidentifiedSessionsOfOtherTerminals(t *testing.T) {
	own := "<Events>\n" +
		dnsRegistrationXML("31", logonHostA, "2001-02-03T04:05:00.000Z", "192.0.2.10", "192.0.2.53") +
		logonXML(logonHostA, "2001-02-03T04:06:00.000Z", "32", "4624", "192.0.2.10", "3") +
		logonXML(logonHostA, "2001-02-03T04:07:00.000Z", "33", "4624", "192.0.2.53", "3") +
		"</Events>\n"
	other := "<Events>\n" +
		logonXML(logonHostB, "2001-02-03T04:06:00.000Z", "41", "4624", "192.0.2.10", "3") +
		"</Events>\n"
	result := windowsEventSessionResult(t, own, other)
	graph := NewGraph(result, AllMatchConditions())
	hostA, _ := core.RecordingHostTerminalNodeKey(result.publications[0].status.Scope.SourceContentSha256, logonHostA)
	hostB, _ := core.RecordingHostTerminalNodeKey(result.publications[1].status.Scope.SourceContentSha256, logonHostB)
	sessions := map[[2]string]bool{}
	for _, edge := range edgesOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession) {
		sessions[[2]string{graph.nodes[edge.source].key.Values[0].Value, graph.nodes[edge.target].id}] = true
	}
	want := map[[2]string]bool{
		{"192.0.2.10", nodeIdOf(hostB)}: true,
		{"192.0.2.53", nodeIdOf(hostA)}: true,
	}
	if len(sessions) != len(want) || !sessions[[2]string{"192.0.2.10", nodeIdOf(hostB)}] ||
		!sessions[[2]string{"192.0.2.53", nodeIdOf(hostA)}] {
		t.Errorf("the unidentified sessions are %v, want %v", sessions, want)
	}
	if edges := terminalSessionEdges(t, graph); len(edges) != 0 {
		t.Errorf("the graph carries the remote session relations %+v, want none", edges)
	}
}
