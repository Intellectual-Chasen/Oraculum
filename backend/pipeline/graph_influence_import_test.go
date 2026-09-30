// in-package test: 取り込んだレコードから影響の経路を求める。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ログオンの連鎖を組む経路が残したセッションの期間から、影響のエッジの D はセッションの期間
// (ログオンからログオフまで)、A は接続元の端末からのログオンになる。根拠のレコードはセッションの
// 始まりと終わりの記録と終点のログオンであり、起点のセッションから終点のログオンへの経路に乗る。
// 割当の期間は、host-a のログオンからログオフまでの記録を含む。
func TestInfluencePathLeavesTheImportedSessionDuringItsPeriod(t *testing.T) {
	result := windowsEventSessionResult(t,
		"<Events>\n"+
			securityEventXML("11", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xc1", "LogonType", "2")+
			securityEventXML("12", "4634", logonHostA, "2001-02-03T04:30:00.000Z", "TargetLogonId", "0xc1")+"</Events>\n",
		"<Events>\n"+chainLogonXML("22", "2001-02-03T04:10:00.000Z", "user-b", "192.0.2.10")+"</Events>\n")
	assignments := []core.TerminalAssignment{
		sessionAssignment(t, result, 0, "192.0.2.10", "ws-a", logonHostA, false),
		sessionAssignment(t, result, 0, "192.0.2.20", "ws-b", logonHostB, false),
	}
	graph := NewObservedGraph(result.WithAnalystTerminalAssignments(assignments))
	named := map[string]int{}
	for at := range graph.records {
		if graph.records[at].hasRecordNode {
			named[recordNameOf(t, graph, at)] = at
		}
	}
	start, end, logon := named["11"], named["12"], named["22"]
	built := graph.newInfluenceGraph(graph.records[start].instant)
	var chains []builtInfluenceEdge
	for _, edge := range built.edges {
		if graph.edges[edge.graphEdge].kind == core.EdgeKindLogonChain {
			chains = append(chains, edge)
		}
	}
	if len(chains) != 1 {
		t.Fatalf("logon chain influence edges = %d, want 1", len(chains))
	}
	chain := chains[0]
	minutes := func(m int64) int64 { return m * 60_000_000_000 }
	if got := chain.exact.depart; len(got) != 1 || got[0].span != (timeSpan{0, minutes(30)}) {
		t.Errorf("D = %+v, want the session from the logon to the logoff", got)
	}
	if got := chain.exact.arrive; len(got) != 1 || got[0].span != (timeSpan{minutes(10), minutes(10)}) {
		t.Errorf("A = %+v, want the chained logon", got)
	}
	if !slices.Equal(chain.records, []int{start, end, logon}) {
		t.Errorf("records = %v, want %v", chain.records, []int{start, end, logon})
	}
	path, found := graph.InfluencePath(InfluencePathQuery{
		From: graph.nodes[graph.records[start].recordNode].id, To: graph.nodes[graph.records[logon].recordNode].id,
	})
	if !found || !slices.ContainsFunc(path.Edges, func(edge core.InfluenceEdge) bool {
		return edge.GraphEdgeKind == core.EdgeKindLogonChain && len(edge.Evidence) == 3
	}) {
		t.Errorf("found=%v edges=%+v stops=%v, want the logon chain on the path", found, path.Edges, path.Stops)
	}
}

// processNodeNamed は、識別鍵の値に name を持つプロセスのノードの識別子を返す。
func processNodeNamed(t *testing.T, graph Graph, name string) string {
	t.Helper()
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindProcess && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool { return value.Value == name }) {
			return node.id
		}
	}
	t.Fatalf("no process %s", name)
	return ""
}

// markii 形式の close は読み書きの量で向きが決まる。書いたプロセスから、後で読んだプロセスへの経路は
// ファイルを通り、書く前に読んだプロセスへは届かない。
func TestInfluencePathFollowsMarkIIFileCloses(t *testing.T) {
	line := func(clock, sequence, process, amounts string) string {
		return "01/02/2024 " + clock + ".000 +0000 sn=" + sequence + " evt=file subEvt=close psGUID=" + process +
			` tmid=T1 com="PC01" path="C:\data\shared.txt" ` + amounts + "\n"
	}
	document := line("03:00:00", "1", "{P3}", "read=4 write=0") +
		line("03:01:00", "2", "{P1}", "read=0 write=5") +
		line("03:02:00", "3", "{P2}", "read=3 write=0")
	graph := NewGraph(sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document}),
		AllMatchConditions())
	writer, reader, early := processNodeNamed(t, graph, "{P1}"), processNodeNamed(t, graph, "{P2}"), processNodeNamed(t, graph, "{P3}")
	path, found := graph.InfluencePath(InfluencePathQuery{
		From: writer, To: reader, Excluded: []core.InfluenceBasis{core.InfluenceBasisUndeterminedDirection},
	})
	if !found || len(path.Edges) != 2 {
		t.Fatalf("found=%v edges=%+v, want the write and the read", found, path.Edges)
	}
	if path.Edges[0].SourceKey != writer || path.Edges[1].TargetKey != reader ||
		!slices.Equal(path.Edges[0].Bases, []core.InfluenceBasis{
			core.InfluenceBasisSpecifiedOperation, core.InfluenceBasisObserved, core.InfluenceBasisSingleNode,
		}) {
		t.Errorf("edges = %+v", path.Edges)
	}
	before, _ := graph.InfluencePath(InfluencePathQuery{
		From: writer, To: early, Excluded: []core.InfluenceBasis{core.InfluenceBasisUndeterminedDirection},
	})
	if len(before.Edges) != 0 {
		t.Errorf("the read before the write joined the path: %+v", before.Edges)
	}
}

// operationInfluence は、操作と通信の関係から作った影響のエッジを「元の種別>先の種別 向きの根拠」の
// 文字列で、並べて返す。
func operationInfluence(graph Graph) []string {
	built := graph.newInfluenceGraph(influenceBase)
	var lines []string
	for _, edge := range built.edges {
		switch graph.edges[edge.graphEdge].kind {
		case core.EdgeKindFileOperation, core.EdgeKindRegistryOperation, core.EdgeKindProcessCommunication:
			lines = append(lines, string(graph.nodes[built.vertices[edge.exact.from].node].key.Kind)+">"+
				string(graph.nodes[built.vertices[edge.exact.to].node].key.Kind)+" "+string(edge.bases[0]))
		}
	}
	slices.Sort(lines)
	return lines
}

// Sysmon の 2、11、12、13 はプロセスから対象への書き込みであり、読み込みの向きを作らない。3 は
// 送信と受信を分けない通信であり、プロセスと接続の間の両方の向きを作る。
func TestInfluenceReadsTheSysmonEventsDirections(t *testing.T) {
	process := []string{"UtcTime", "2001-02-03 04:05:00.000", "ProcessGuid", "{00000000-0000-0000-0000-000000000001}",
		"ProcessId", "4", "Image", `C:\tool.exe`}
	cases := []struct {
		eventID string
		data    []string
		want    []string
	}{
		{"2", []string{"TargetFilename", `C:\data\b.txt`}, []string{"process>file specified_operation"}},
		{"11", []string{"TargetFilename", `C:\data\b.txt`}, []string{"process>file specified_operation"}},
		{"12", []string{"EventType", "CreateKey", "TargetObject", `HKLM\SOFTWARE\Example`},
			[]string{"process>registry_value specified_operation"}},
		{"13", []string{"EventType", "SetValue", "TargetObject", `HKLM\SOFTWARE\Example\Value`},
			[]string{"process>registry_value specified_operation"}},
		{"3", []string{"Initiated", "true", "SourceIp", "192.0.2.1", "SourcePort", "50000",
			"DestinationIp", "198.51.100.2", "DestinationPort", "443"},
			[]string{"process>record specified_operation", "record>process specified_operation"}},
	}
	for _, test := range cases {
		event := taskEventXML("Microsoft-Windows-Sysmon", "1", test.eventID, logonHostA, "2001-02-03T04:05:00.000Z",
			append(slices.Clone(process), test.data...)...)
		graph := NewGraph(windowsEventImportResult(t, "<Events>\n"+event+"</Events>\n"), AllMatchConditions())
		if got := operationInfluence(graph); !slices.Equal(got, test.want) {
			t.Errorf("event %s: influence = %v, want %v", test.eventID, got, test.want)
		}
	}
}

// Security の 4720 と 4732 は、アカウントの管理操作として、レコードから作成したアカウントと、
// グループへ追加したメンバーのアカウントへ渡る。グループのノードへは渡らない。
func TestInfluenceReadsTheAccountManagementEvents(t *testing.T) {
	const created, member, group = "S-1-5-21-1-2-3-1001", "S-1-5-21-1-2-3-1002", "S-1-5-21-1-2-3-1003"
	document := "<Events>\n" +
		securityEventXML("31", "4720", logonHostA, "2001-02-03T04:05:00.000Z", "TargetSid", created) +
		securityEventXML("32", "4732", logonHostA, "2001-02-03T04:06:00.000Z", "MemberSid", member, "TargetSid", group) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	built := graph.newInfluenceGraph(influenceBase)
	var lines []string
	for _, edge := range built.edges {
		if edge.bases[0] != core.InfluenceBasisAccountManagement {
			continue
		}
		from, to := graph.nodes[built.vertices[edge.exact.from].node], graph.nodes[built.vertices[edge.exact.to].node]
		var sid string
		for _, value := range to.key.Values {
			sid += value.Value
		}
		lines = append(lines, recordNameOf(t, graph, edge.records[0])+" "+string(from.key.Kind)+">"+sid)
	}
	slices.Sort(lines)
	if want := []string{"31 record>" + created, "32 record>" + member}; !slices.Equal(lines, want) {
		t.Errorf("account management influence = %v, want %v", lines, want)
	}
}

// 4624 だけが、ログオンしたアカウントのノード (識別子と名前の両方) からログオンのレコードへの、資格情報の
// 使用の影響のエッジを作る。4688・4625・4634 の対象、4624 の主体、4769 のサービスのアカウント、
// well-known な識別子からは作らない。
//
// アカウント → 4624 → 同じセッションの 4720 → 作ったアカウント → 4624 の経路をたどれる。作ったアカウントの
// 4720 より前のログオンと、別のアカウントのログオンへは届かない。別の端末のログオンへは上限の無いずれの差で届く。
func TestInfluencePathFollowsTheCredentialUse(t *testing.T) {
	const operator, subject, created = "S-1-5-21-1-2-3-1100", "S-1-5-21-1-2-3-1200", "S-1-5-21-1-2-3-1300"
	logon := func(recordID, computer, at, sid, name, logonID string) string {
		return securityEventXML(recordID, "4624", computer, at, "SubjectUserSid", subject, "SubjectUserName", "subject",
			"SubjectDomainName", "EXAMPLE", "TargetUserSid", sid, "TargetUserName", name, "TargetDomainName", "EXAMPLE",
			"TargetLogonId", logonID, "LogonType", "3")
	}
	document := "<Events>\n" +
		logon("41", logonHostA, "2001-02-03T04:00:00.000Z", operator, "operator", "0xa1") +
		securityEventXML("42", "4720", logonHostA, "2001-02-03T04:05:00.000Z", "SubjectUserSid", operator,
			"SubjectUserName", "operator", "SubjectDomainName", "EXAMPLE", "SubjectLogonId", "0xa1",
			"TargetSid", created, "TargetUserName", "created", "TargetDomainName", "EXAMPLE") +
		logon("43", logonHostA, "2001-02-03T04:10:00.000Z", created, "created", "0xb1") +
		logon("44", logonHostA, "2001-02-03T04:01:00.000Z", created, "created", "0xb2") +
		logon("45", logonHostB, "2001-02-03T04:12:00.000Z", created, "created", "0xb3") +
		logon("46", logonHostA, "2001-02-03T04:11:00.000Z", "S-1-5-21-1-2-3-1400", "other", "0xb4") +
		securityEventXML("51", "4688", logonHostA, "2001-02-03T04:02:00.000Z", "TargetUserSid", "S-1-5-21-1-2-3-1501",
			"TargetUserName", "t4688", "TargetDomainName", "EXAMPLE", "NewProcessId", "0x10", "NewProcessName", `C:\a.exe`) +
		securityEventXML("52", "4625", logonHostA, "2001-02-03T04:02:00.000Z", "TargetUserSid", "S-1-5-21-1-2-3-1502",
			"TargetUserName", "t4625", "TargetDomainName", "EXAMPLE") +
		securityEventXML("53", "4634", logonHostA, "2001-02-03T04:02:00.000Z", "TargetUserSid", "S-1-5-21-1-2-3-1503",
			"TargetUserName", "t4634", "TargetDomainName", "EXAMPLE", "TargetLogonId", "0xc1") +
		securityEventXML("54", "4769", logonHostA, "2001-02-03T04:02:00.000Z", "ServiceSid", "S-1-5-21-1-2-3-1504",
			"ServiceName", "service", "TargetUserName", "operator@EXAMPLE") +
		logon("55", logonHostA, "2001-02-03T04:02:00.000Z", "S-1-5-18", "SYSTEM", "0x3e7") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	built := graph.newInfluenceGraph(influenceBase)
	var lines []string
	for _, edge := range built.edges {
		if edge.bases[0] != core.InfluenceBasisCredentialUse {
			continue
		}
		from := graph.nodes[built.vertices[edge.exact.from].node]
		var key string
		for _, value := range from.key.Values {
			key += value.Value
		}
		lines = append(lines, recordNameOf(t, graph, edge.records[0])+" "+key)
	}
	slices.Sort(lines)
	// 識別子のノードと名前のノードから 1 本ずつ出る。
	var want []string
	for _, item := range []struct{ record, sid, name string }{
		{"41", operator, "operator"}, {"43", created, "created"}, {"44", created, "created"},
		{"45", created, "created"}, {"46", "S-1-5-21-1-2-3-1400", "other"},
	} {
		want = append(want, item.record+" EXAMPLE"+item.name, item.record+" "+item.sid)
	}
	slices.Sort(want)
	if !slices.Equal(lines, want) {
		t.Errorf("credential use = %v, want %v", lines, want)
	}

	origin := accountNodeWith(t, graph, operator)
	recordNode := func(name string) string {
		for at := range graph.records {
			if graph.records[at].hasRecordNode && recordNameOf(t, graph, at) == name {
				return graph.nodes[graph.records[at].recordNode].id
			}
		}
		t.Fatalf("no record %s", name)
		return ""
	}
	reached := func(name string) InfluencePath {
		path, found := graph.InfluencePath(InfluencePathQuery{From: origin, To: recordNode(name)})
		if !found {
			t.Fatalf("no node for %s", name)
		}
		return path
	}
	path := reached("43")
	var kinds []core.InfluenceBasis
	for _, edge := range path.Edges {
		kinds = append(kinds, edge.Bases[0])
	}
	if !slices.Contains(kinds, core.InfluenceBasisCredentialUse) || !slices.Contains(kinds, core.InfluenceBasisAccountManagement) {
		t.Errorf("path to the created account's logon: bases=%v stops=%v, want the credential use and the account management",
			kinds, path.Stops)
	}
	for _, name := range []string{"44", "46"} {
		if unreached := reached(name); len(unreached.Edges) != 0 {
			t.Errorf("path to %s has %d edges, want none", name, len(unreached.Edges))
		}
	}
	across := reached("45")
	if len(across.Edges) == 0 || !slices.ContainsFunc(across.Edges, func(edge core.InfluenceEdge) bool {
		return slices.Contains(edge.TimeBases, core.InfluenceTimeBasisUnboundedOffset)
	}) {
		t.Errorf("path to the logon on the other terminal: edges=%+v stops=%v, want the unbounded offset",
			across.Edges, across.Stops)
	}
}

// influenceLines は影響のエッジを「元の種別:元の識別鍵の値>先の種別 向きの根拠」の文字列で、並べて返す。
func influenceLines(graph Graph) []string {
	built := graph.newInfluenceGraph(influenceBase)
	var lines []string
	for _, edge := range built.edges {
		from, to := graph.nodes[built.vertices[edge.exact.from].node], graph.nodes[built.vertices[edge.exact.to].node]
		key := ""
		if from.key.Kind == core.NodeKindAccount {
			for _, value := range from.key.Values {
				key += ":" + value.Value
			}
		}
		lines = append(lines, string(from.key.Kind)+key+">"+string(to.key.Kind)+" "+string(edge.bases[0]))
	}
	slices.Sort(lines)
	return lines
}

// イベントビューアーの CSV も XML と同じ操作の分類を持つ。4624 はログオンしたアカウントからログオンの
// レコードへの資格情報の使用、4720 は操作のレコードから作成したアカウントへの管理操作、Sysmon の 11 は
// プロセスからファイルへの書き込みの影響のエッジを 1 本ずつ作る。
func TestInfluenceReadsTheOperationsOfAViewerCSV(t *testing.T) {
	created := "情報,2001/02/03 04:05:08,Microsoft-Windows-Security-Auditing,4720,User Account Management," +
		"\"ユーザー アカウントが作成されました。\n\nサブジェクト:\n\tアカウント名:\t\tadmin01\n" +
		"\tアカウント ドメイン:\t\tCORP-TEST\n\tログオン ID:\t\t0x10\n\n新しいアカウント:\n" +
		"\tアカウント名:\t\tnew01\n\tアカウント ドメイン:\t\tCORP-TEST\n\"\n"
	fileCreate := "情報,2001/02/03 04:05:09,Microsoft-Windows-Sysmon,11,File created (rule: FileCreate)," +
		"\"File created:\nRuleName: -\nUtcTime: 2001-02-03 04:05:09.000\n" +
		"ProcessGuid: {00000000-0000-0000-0000-000000000001}\nProcessId: 4\nImage: C:\\tool.exe\n" +
		"TargetFilename: C:\\data\\b.txt\nCreationUtcTime: 2001-02-03 04:05:09.000\"\n"
	cases := []struct {
		name, record string
		want         []string
	}{
		{"4624", logonCSV("user01", "192.0.2.10"), []string{"account:CORP-TEST:user01>record credential_use"}},
		{"4720", created, []string{"record>account account_management"}},
		{"Sysmon 11", fileCreate, []string{"process>file specified_operation"}},
	}
	for _, test := range cases {
		if got := influenceLines(viewerCSVGraph(t, test.record)); !slices.Equal(got, test.want) {
			t.Errorf("%s: influence = %v, want %v", test.name, got, test.want)
		}
	}
}

// accountNodeWith は、識別鍵の値に sid を持つアカウントのノードの識別子を返す。
func accountNodeWith(t *testing.T, graph Graph, sid string) string {
	t.Helper()
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindAccount && slices.ContainsFunc(node.key.Values,
			func(value core.NodeIdentityValue) bool { return value.Value == sid }) {
			return node.id
		}
	}
	t.Fatalf("no account %s", sid)
	return ""
}

// markii 形式の file の del と reg の setVal、delVal、delKey はプロセスから対象への書き込み、net の acpt は
// 接続からプロセスへの受け入れ、con と dcon は両方の向きの通信である。
func TestInfluenceReadsTheMarkIIRecordDirections(t *testing.T) {
	network := ` csid=s psPath=app srcIP=192.0.2.1 srcPort=50000 dstIP=198.51.100.2 dstPort=443`
	cases := []struct {
		event, subEvent, target string
		want                    []string
	}{
		{"file", "del", ` path="C:\data\a.txt"`, []string{"process>file specified_operation"}},
		{"reg", "setVal", ` path="HKLM\SOFTWARE\Example\Value"`, []string{"process>registry_value specified_operation"}},
		{"reg", "delVal", ` path="HKLM\SOFTWARE\Example\Value"`, []string{"process>registry_value specified_operation"}},
		{"reg", "delKey", ` path="HKLM\SOFTWARE\Example"`, []string{"process>registry_value specified_operation"}},
		{"net", "acpt", network, []string{"record>process specified_operation"}},
		{"net", "con", network, []string{"process>record specified_operation", "record>process specified_operation"}},
		{"net", "dcon", network, []string{"process>record specified_operation", "record>process specified_operation"}},
	}
	for _, test := range cases {
		document := "01/02/2024 03:00:00.000 +0000 sn=1 evt=" + test.event + " subEvt=" + test.subEvent +
			` psGUID={P1} tmid=T1 com="PC01"` + test.target + "\n"
		graph := NewGraph(sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document}),
			AllMatchConditions())
		if got := operationInfluence(graph); !slices.Equal(got, test.want) {
			t.Errorf("%s %s: influence = %v, want %v", test.event, test.subEvent, got, test.want)
		}
	}
}
