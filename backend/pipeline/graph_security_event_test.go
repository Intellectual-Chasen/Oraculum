// in-package test: Security の通信・共有・アカウントの管理・チケットのイベントから組んだグラフを確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	securityAdminSid  = "S-1-5-21-1000-2000-3000-1301"
	securityMemberSid = "S-1-5-21-1000-2000-3000-1302"
	securityGroupSid  = "S-1-5-21-1000-2000-3000-512"
)

// nodeOf は種別が kind で最初の識別値が value のノードの位置を返す。見つからなければ -1 である。
func nodeOf(graph Graph, kind core.NodeKind, value string) int {
	for at, node := range graph.nodes {
		if node.key.Kind == kind && len(node.key.Values) > 0 && node.key.Values[0].Value == value {
			return at
		}
	}
	return -1
}

// edgePairsOfKind は種別が kind のエッジの (source, target) を返す。
func edgePairsOfKind(graph Graph, kind core.EdgeKind) [][2]int {
	var pairs [][2]int
	for _, edge := range graph.edges {
		if edge.kind == kind {
			pairs = append(pairs, [2]int{edge.source, edge.target})
		}
	}
	return pairs
}

// 16 進の番号の 4688 と 10 進の番号の外向きの 5156 は同じ区間のプロセスになり、そのプロセスから
// 接続先へ process_communication が張られる。内向きの 5156 は、相手への関係を張らない。
func TestSecurityConnectionLinksTheIntervalProcessToTheDestination(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("61", "4688", logonHostA, "2001-02-03T04:05:00.000Z",
			"NewProcessId", "0x4d2", "NewProcessName", `C:\Example\tool.exe`, "CommandLine", "tool.exe",
			"ProcessId", "0x10") +
		securityEventXML("62", "5156", logonHostA, "2001-02-03T04:05:10.000Z",
			"ProcessID", "1234", "Application", `\device\harddiskvolume2\example\tool.exe`,
			"Direction", "%%14593", "SourceAddress", "192.0.2.10", "SourcePort", "50100",
			"DestAddress", "198.51.100.20", "DestPort", "443") +
		securityEventXML("63", "5156", logonHostA, "2001-02-03T04:05:20.000Z",
			"ProcessID", "1234", "Direction", "%%14592", "SourceAddress", "192.0.2.10", "SourcePort", "445",
			"DestAddress", "198.51.100.30", "DestPort", "50200") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	destination := nodeOf(graph, core.NodeKindIp, "198.51.100.20")
	peer := nodeOf(graph, core.NodeKindIp, "198.51.100.30")
	if destination < 0 || peer < 0 {
		t.Fatalf("the graph holds no ip node for the destination (%d) or the peer (%d)", destination, peer)
	}
	var processes []int
	for _, pair := range edgePairsOfKind(graph, core.EdgeKindProcessCommunication) {
		if pair[1] == peer {
			t.Error("the inbound connection links a process to the peer")
		}
		if pair[1] == destination {
			processes = append(processes, pair[0])
		}
	}
	if len(processes) != 1 || graph.nodes[processes[0]].key.Kind != core.NodeKindProcess {
		t.Fatalf("process_communication sources to the destination = %v, want one process", processes)
	}
	if label, _ := graph.nodes[processes[0]].label.RawTextValue(); label != `C:\Example\tool.exe` {
		t.Errorf("the communicating process = %q, want the process the 4688 created", label)
	}
}

// 5379 の SubjectLogonId は、同じセッションを作ったログオンの操作になる。
func TestCredentialReadBecomesAnOperationOfTheLogonSession(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("71", "4624", logonHostA, "2001-02-03T04:05:00.000Z",
			"TargetLogonId", firstSessionLong, "LogonType", "2") +
		securityEventXML("72", "5379", logonHostA, "2001-02-03T04:05:10.000Z",
			"SubjectLogonId", firstSessionShort) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	logon := graph.records[recordOfEvent(t, graph, "71")].recordNode
	operation := graph.records[recordOfEvent(t, graph, "72")].recordNode
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindLogonSessionOperation), [2]int{logon, operation}) {
		t.Errorf("logon_session_operation = %v, want the logon to the 5379", edgePairsOfKind(graph, core.EdgeKindLogonSessionOperation))
	}
}

// 4728・4732 のメンバーは record_target_account で結ばれ、グループはアカウントの種類のノードに
// なってレコードから record_names_object で結ばれる。ドメインのグループは SID のノード、
// builtin のグループは記録した端末の範囲の名前のノードである。4769 は要求されたサービスの
// アカウントを Target にする。
func TestGroupChangeAndServiceTicketLinkTheRoleAccounts(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("81", "4728", logonHostA, "2001-02-03T04:05:00.000Z",
			"MemberSid", securityMemberSid, "TargetSid", securityGroupSid, "TargetUserName", "Example Admins",
			"SubjectUserSid", securityAdminSid, "SubjectUserName", "admin31") +
		securityEventXML("82", "4769", logonHostA, "2001-02-03T04:05:10.000Z",
			"TargetUserName", "user32@EXAMPLE.TEST", "ServiceName", "svc33", "ServiceSid", securityMemberSid,
			"IpAddress", "::ffff:192.0.2.60") +
		securityEventXML("83", "4732", logonHostA, "2001-02-03T04:05:20.000Z",
			"MemberSid", securityMemberSid, "TargetSid", "S-1-5-32-544", "TargetUserName", "Example Local",
			"TargetDomainName", "Builtin", "SubjectUserSid", securityAdminSid) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	group := accountNodeOfSid(graph, securityGroupSid)
	member := accountNodeOfSid(graph, securityMemberSid)
	admin := accountNodeOfSid(graph, securityAdminSid)
	if group < 0 || member < 0 || admin < 0 {
		t.Fatalf("account nodes: group %d, member %d, admin %d; want all", group, member, admin)
	}
	if accountNodeOfSid(graph, "S-1-5-32-544") >= 0 {
		t.Error("the builtin group became a node across the terminals")
	}
	builtin := -1
	for at, node := range graph.nodes {
		values := node.key.Values
		if node.key.Form == core.NodeKeyFormTerminalAccountName && values[len(values)-1].Value == "Example Local" {
			builtin = at
		}
	}
	if builtin < 0 {
		t.Fatal("the builtin group has no node in the range of the terminal")
	}
	change := graph.records[recordOfEvent(t, graph, "81")].recordNode
	ticket := graph.records[recordOfEvent(t, graph, "82")].recordNode
	local := graph.records[recordOfEvent(t, graph, "83")].recordNode
	for _, want := range []struct {
		source, target int
		kind           core.EdgeKind
	}{
		{change, member, core.EdgeKindRecordTargetAccount},
		{change, admin, core.EdgeKindRecordSubjectAccount},
		{change, group, core.EdgeKindRecordNamesObject},
		{ticket, member, core.EdgeKindRecordTargetAccount},
		{local, member, core.EdgeKindRecordTargetAccount},
		{local, builtin, core.EdgeKindRecordNamesObject},
	} {
		if !slices.Contains(edgePairsOfKind(graph, want.kind), [2]int{want.source, want.target}) {
			t.Errorf("no %s from record %d to account %d", want.kind, want.source, want.target)
		}
	}
	if nodeOf(graph, core.NodeKindIp, "192.0.2.60") < 0 {
		t.Error("the IPv4-mapped client address does not name the dotted decimal ip node")
	}
	timeline := graph.Timeline(TimelineQuery{AccountNodeId: graph.nodes[group].id})
	if len(timeline.Entries) != 1 {
		t.Fatalf("group account timeline has %d entries, want the group-change record", len(timeline.Entries))
	}
	if !slices.Contains(timeline.Entries[0].AccountRoles, core.EdgeKindRecordNamesObject) {
		t.Errorf("group account roles = %v, want record_names_object", timeline.Entries[0].AccountRoles)
	}
}

// 4104 の Security@UserID は操作の主体のアカウントになり、同じ SID の 4624 のアカウントと
// 同じノードへ record_subject_account で結ばれる。
func TestScriptBlockUserIDLinksTheSubjectAccount(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("91", "4624", logonHostA, "2001-02-03T04:05:00.000Z",
			"TargetUserSid", securityMemberSid, "TargetUserName", "user41", "TargetDomainName", "EXAMPLE",
			"TargetLogonId", firstSessionLong, "LogonType", "2") +
		`<Event><System><Provider Name="Microsoft-Windows-PowerShell"/><EventID>4104</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:10.000Z"/><EventRecordID>92</EventRecordID>` +
		`<Channel>Microsoft-Windows-PowerShell/Operational</Channel><Computer>` + logonHostA + `</Computer>` +
		`<Security UserID="` + securityMemberSid + `"/></System>` +
		`<EventData><Data Name="ScriptBlockText">Get-Item example</Data></EventData></Event>` + "\n" +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	account := accountNodeOfSid(graph, securityMemberSid)
	if account < 0 {
		t.Fatal("the graph holds no account node for the SID")
	}
	logon := graph.records[recordOfEvent(t, graph, "91")].recordNode
	script := graph.records[recordOfEvent(t, graph, "92")].recordNode
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindRecordTargetAccount), [2]int{logon, account}) {
		t.Error("the 4624 does not link the account node of the SID")
	}
	if !slices.Contains(edgePairsOfKind(graph, core.EdgeKindRecordSubjectAccount), [2]int{script, account}) {
		t.Errorf("record_subject_account = %v, want the 4104 to the account node of the SID",
			edgePairsOfKind(graph, core.EdgeKindRecordSubjectAccount))
	}
}

// IPv6 の形で書いた IPv4 の接続元と、ドット 10 進の接続元は、1 つの IP のノードになる。
func TestIPv4MappedAndDottedSourcesShareOneIpNode(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("91", "4624", logonHostA, "2001-02-03T04:05:00.000Z",
			"TargetLogonId", firstSessionLong, "LogonType", "3", "IpAddress", "192.0.2.80") +
		securityEventXML("92", "4768", logonHostA, "2001-02-03T04:05:10.000Z",
			"TargetUserName", "user34", "IpAddress", "::ffff:192.0.2.80") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	count := 0
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindIp {
			count++
		}
	}
	if count != 1 || nodeOf(graph, core.NodeKindIp, "192.0.2.80") < 0 {
		t.Errorf("the graph holds %d ip nodes, want the one dotted decimal node", count)
	}
}
