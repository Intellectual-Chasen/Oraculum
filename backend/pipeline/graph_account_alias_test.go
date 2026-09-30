// in-package test: SID と共に記録したドメインと名前の組を、SID のノードへ寄せることを確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// SID とアカウントの名前。
const (
	aliasSid      = "S-1-5-21-4000-5000-6000-1201"
	aliasOldSid   = "S-1-5-21-4000-5000-6000-1202"
	aliasNewSid   = "S-1-5-21-4000-5000-6000-1203"
	aliasHost     = "HOST-Q"
	aliasUser     = "user-q"
	rdpLsm        = "Microsoft-Windows-TerminalServices-LocalSessionManager"
	rdpConnection = "Microsoft-Windows-TerminalServices-RemoteConnectionManager"
)

// aliasAccountGraph は、1 つのアカウントを SID と名前の組で記録したレコードと、名前だけで
// 記録したレコードと、SID だけで記録したグループへの追加を並べてグラフを組む。
func aliasAccountGraph(t *testing.T) Graph {
	t.Helper()
	target := func(recordID, eventID, at string) string {
		return securityEventXML(recordID, eventID, "dc.example.test", at,
			"TargetSid", aliasSid, "TargetUserName", aliasUser, "TargetDomainName", aliasHost)
	}
	document := "<Events>\n" +
		remoteDesktopXML(rdpLsm, "21", "60", "2001-02-03T03:00:00Z",
			`<UserData><EventXML><User>host-q\USER-Q</User><Address>192.0.2.30</Address></EventXML></UserData>`) +
		target("61", "4720", "2001-02-03T04:00:00Z") +
		target("62", "4722", "2001-02-03T04:00:01Z") +
		target("63", "4724", "2001-02-03T04:00:02Z") +
		securityEventXML("64", "4732", "dc.example.test", "2001-02-03T04:00:03Z",
			"MemberSid", aliasSid, "TargetUserName", "group-q", "TargetDomainName", "Builtin",
			"TargetSid", "S-1-5-32-544") +
		target("65", "4738", "2001-02-03T04:00:04Z") +
		securityEventXML("66", "4624", "dc.example.test", "2001-02-03T04:10:00Z",
			"TargetUserSid", aliasSid, "TargetUserName", aliasUser, "TargetDomainName", aliasHost,
			"LogonType", "10") +
		remoteDesktopXML(rdpConnection, "1149", "67", "2001-02-03T04:09:59Z",
			`<UserData><EventXML><Param1>`+aliasUser+`</Param1><Param2>`+aliasHost+`</Param2><Param3>192.0.2.30</Param3></EventXML></UserData>`) +
		securityEventXML("69", "4625", "dc.example.test", "2001-02-03T04:09:00Z",
			"TargetUserSid", "S-1-0-0", "TargetUserName", aliasUser, "TargetDomainName", aliasHost,
			"LogonType", "3", "IpAddress", "192.0.2.30") +
		target("68", "4726", "2001-02-03T05:00:00Z") +
		"</Events>\n"
	return NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
}

// accountNodesNamedBy は、レコード at のノードから関係で指したアカウントのノードの位置を返す。
func accountNodesNamedBy(t *testing.T, graph Graph, at int) []int {
	t.Helper()
	record := graph.records[at]
	if !record.hasRecordNode {
		t.Fatalf("the record %d has no record node", at)
	}
	var accounts []int
	for _, edge := range graph.adjacency[record.recordNode].outgoing {
		if target := graph.edges[edge].target; graph.nodes[target].key.Kind == core.NodeKindAccount {
			accounts = append(accounts, target)
		}
	}
	return accounts
}

// nameNodeNamedBy は、レコード at が指すドメインと名前の組のアカウントのノードの位置を返す。
// 無いときは -1 を返す。
func nameNodeNamedBy(t *testing.T, graph Graph, at int) int {
	t.Helper()
	for _, account := range accountNodesNamedBy(t, graph, at) {
		if graph.nodes[account].key.Form == core.NodeKeyFormAccountDomainName {
			return account
		}
	}
	return -1
}

// SID と名前を共に記録したレコードと、SID だけのグループへの追加は、SID のノードを指す。
// 名前だけのレコード (LocalSessionManager の `領域\名前` と 1149) と、Target の SID が NULL SID の
// ログオンの失敗は、記録した名前のノードを指し、その名前のノードは SID のノードと同じ
// アカウントの候補のエッジで結ばれる。
func TestAccountNameRecordsKeepTheNameNodePairedWithTheSid(t *testing.T) {
	graph := aliasAccountGraph(t)
	sidNode := accountNodeOfSid(graph, aliasSid)
	if sidNode < 0 {
		t.Fatal("no account node carries the SID")
	}
	for _, recordID := range []string{"61", "64", "66"} {
		// グループへの追加は、グループのノードも指す。
		accounts := accountNodesNamedBy(t, graph, recordOfEvent(t, graph, recordID))
		if !slices.Contains(accounts, sidNode) {
			t.Errorf("the record %s names the accounts %v, want the SID node %d", recordID, accounts, sidNode)
		}
	}
	identity := edgePairsOfKind(graph, core.EdgeKindAccountIdentityMatch)
	for _, recordID := range []string{"60", "67", "69"} {
		at := recordOfEvent(t, graph, recordID)
		if slices.Contains(accountNodesNamedBy(t, graph, at), sidNode) {
			t.Errorf("the name-only record %s names the SID node %d", recordID, sidNode)
		}
		named := nameNodeNamedBy(t, graph, at)
		if named < 0 {
			t.Errorf("the name-only record %s names no name node", recordID)
			continue
		}
		if !slices.Contains(identity, [2]int{named, sidNode}) {
			t.Errorf("the name node of the record %s has no identity candidate to the SID node, edges %v",
				recordID, identity)
		}
	}
	// 表示名は記録した文字列である。
	if named := nameNodeNamedBy(t, graph, recordOfEvent(t, graph, "60")); named >= 0 {
		if raw, _ := graph.nodes[named].label.RawTextValue(); raw != `host-q\USER-Q` {
			t.Errorf("the name node of the record 60 is labelled %q, want the recorded host-q\\USER-Q", raw)
		}
	}
	for _, edge := range graph.edges {
		if edge.source == edge.target {
			t.Errorf("the edge %s loops on the node %d", edge.kind, edge.source)
		}
	}
	creation := graph.nodes[sidNode].creationRecords
	if len(creation) != 1 || creation[0] != recordOfEvent(t, graph, "61") {
		t.Errorf("the account carries the creation records %v, want the 4720", creation)
	}
	if got := graph.graphNode(sidNode).CreationRecord; got != core.NodeCreationRecordPresent {
		t.Errorf("the account carries the creation record %q, want present", got)
	}
}

// 1 つの SID を別の名前と共に記録した (改名した) ときは、名前ごとのノードが残り、表示名は
// 記録した文字列である。改名の後の名前だけを記録したレコードは、改名の後の名前のノードを指す。
func TestRenamedAccountKeepsEachRecordedName(t *testing.T) {
	logon := func(recordID, at, name string) string {
		return securityEventXML(recordID, "4624", "dc.example.test", at,
			"TargetUserSid", aliasSid, "TargetUserName", name, "TargetDomainName", aliasHost, "LogonType", "10")
	}
	document := "<Events>\n" +
		logon("81", "2001-02-03T01:00:00Z", "Admin-Q") +
		logon("82", "2001-02-03T02:00:00Z", "renamed-q") +
		remoteDesktopXML(rdpLsm, "21", "83", "2001-02-03T02:10:00Z",
			`<UserData><EventXML><User>HOST-Q\renamed-q</User><Address>192.0.2.30</Address></EventXML></UserData>`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	sidNode := accountNodeOfSid(graph, aliasSid)
	if sidNode < 0 {
		t.Fatal("no account node carries the SID")
	}
	for recordID, want := range map[string]string{"81": "Admin-Q", "82": "renamed-q", "83": "renamed-q"} {
		named := nameNodeNamedBy(t, graph, recordOfEvent(t, graph, recordID))
		if named < 0 {
			t.Errorf("the record %s names no name node", recordID)
			continue
		}
		if raw, _ := graph.nodes[named].label.RawTextValue(); raw != want {
			t.Errorf("the record %s names the account labelled %q, want %q", recordID, raw, want)
		}
	}
	if slices.Contains(accountNodesNamedBy(t, graph, recordOfEvent(t, graph, "83")), sidNode) {
		t.Error("the name-only record names the SID node")
	}
	// 時系列の行とエッジの詳細が出すレコードのアカウントは、そのレコードが記録した名前である。
	for recordID, want := range map[string]string{"81": "Admin-Q", "82": "renamed-q"} {
		account := graph.nodes[graph.nodeAt[graph.records[recordOfEvent(t, graph, recordID)].accountNodeId]]
		if raw, _ := account.label.RawTextValue(); raw != want {
			t.Errorf("the record %s carries the account labelled %q, want %q", recordID, raw, want)
		}
	}
}

// XML の 4769 の要求したアカウント (TargetUserName) は、Security@UserID が空の値でも、SID の値でも、
// 名前のノードになる。
func TestTicketRequestXMLNamesTheRequestingAccount(t *testing.T) {
	for name, userID := range map[string]string{
		"an empty UserID":        "",
		"a well-known UserID":    "S-1-5-18",
		"a domain account's SID": "S-1-5-21-4000-5000-6000-1302",
	} {
		document := "<Events>\n" +
			`<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4769</EventID>` +
			`<TimeCreated SystemTime="2001-02-03T04:00:00Z"/><EventRecordID>91</EventRecordID>` +
			`<Channel>Security</Channel><Computer>dc.example.test</Computer><Security UserID="` + userID + `"/></System>` +
			`<EventData><Data Name="TargetUserName">user-q@EXAMPLE.TEST</Data>` +
			`<Data Name="TargetDomainName">EXAMPLE.TEST</Data><Data Name="ServiceName">HOST-Q$</Data>` +
			`<Data Name="ServiceSid">S-1-5-21-4000-5000-6000-1301</Data>` +
			`<Data Name="IpAddress">192.0.2.30</Data></EventData></Event>` + "\n" +
			"</Events>\n"
		graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
		at := recordOfEvent(t, graph, "91")
		named := nameNodeNamedBy(t, graph, at)
		if named < 0 {
			t.Errorf("%s: the 4769 names no account by the requesting name", name)
			continue
		}
		if values := graph.nodes[named].key.Values; values[len(values)-1].Value != "user-q" {
			t.Errorf("%s: the 4769 names the account %v, want user-q", name, values)
		}
		// UserID の SID は、要求したアカウントとして指されない。
		if userID != "" && accountNodeOfSid(graph, userID) >= 0 &&
			slices.Contains(accountNodesNamedBy(t, graph, at), accountNodeOfSid(graph, userID)) {
			t.Errorf("%s: the 4769 names the UserID %s as an account", name, userID)
		}
	}
}

// 同じ名前を 2 つの SID と共に記録した (削除と作り直し) ときは、名前だけのレコードを SID の
// ノードへ寄せない。
func TestAccountNameWithTwoSidsStaysApart(t *testing.T) {
	account := func(recordID, eventID, at, sid string) string {
		return securityEventXML(recordID, eventID, "dc.example.test", at,
			"TargetSid", sid, "TargetUserName", aliasUser, "TargetDomainName", aliasHost)
	}
	document := "<Events>\n" +
		account("71", "4720", "2001-02-03T01:00:00Z", aliasOldSid) +
		account("72", "4726", "2001-02-03T02:00:00Z", aliasOldSid) +
		account("73", "4720", "2001-02-03T03:00:00Z", aliasNewSid) +
		remoteDesktopXML(rdpLsm, "21", "74", "2001-02-03T04:00:00Z",
			`<UserData><EventXML><User>HOST-Q\user-q</User><Address>192.0.2.30</Address></EventXML></UserData>`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	accounts := accountNodesNamedBy(t, graph, recordOfEvent(t, graph, "74"))
	if len(accounts) != 1 || graph.nodes[accounts[0]].key.Form != core.NodeKeyFormAccountDomainName {
		t.Fatalf("the name-only record names %v, want one name node", accounts)
	}
	if accountNodeOfSid(graph, aliasOldSid) < 0 || accountNodeOfSid(graph, aliasNewSid) < 0 {
		t.Error("the graph lacks a node of either SID")
	}
}

// 欄を持たない検索の文字列は、SID だけを記録したグループへの追加も、指したアカウントの名前で
// 見つける。
func TestBareSearchTermFindsTheRecordNamingTheAccountBySid(t *testing.T) {
	graph := aliasAccountGraph(t)
	expression, err := ParseSearchExpression(aliasUser)
	if err != nil {
		t.Fatal(err)
	}
	groupAdd := graph.nodes[graph.records[recordOfEvent(t, graph, "64")].recordNode]
	if expression.matches(groupAdd) {
		t.Fatal("the fields of the group addition already carry the name")
	}
	if !expression.withNamedAccounts(graph).matches(groupAdd) {
		t.Error("the search does not find the group addition by the member name")
	}
	other, _ := ParseSearchExpression("user-absent")
	if other.withNamedAccounts(graph).matches(groupAdd) {
		t.Error("the search finds the group addition by an unrelated name")
	}
}

// アカウントの管理の事象を、区分と時刻の順に返す。グループへの追加はグループの名前を持つ。
func TestAccountHistoryListsTheManagementRecordsInTimeOrder(t *testing.T) {
	graph := aliasAccountGraph(t)
	history := graph.AccountHistory(graph.nodes[accountNodeOfSid(graph, aliasSid)].id)
	want := []AccountHistoryCategory{
		AccountHistoryCreated, AccountHistoryEnabled, AccountHistoryPasswordSet,
		AccountHistoryGroupMemberAdded, AccountHistoryChanged, AccountHistoryDeleted,
	}
	if len(history) != len(want) {
		t.Fatalf("the history carries %+v, want %v", history, want)
	}
	for at, entry := range history {
		if entry.Category != want[at] {
			t.Errorf("entry %d is %q, want %q", at, entry.Category, want[at])
		}
		if entry.EventTime == nil || entry.Record.SourceId == "" {
			t.Errorf("entry %d lacks the record or the time: %+v", at, entry)
		}
		name := ""
		if entry.GroupName != nil {
			name, _ = entry.GroupName.RawTextValue()
		}
		if wantGroup := entry.Category == AccountHistoryGroupMemberAdded; wantGroup != (name == "group-q") {
			t.Errorf("entry %d (%q) carries the group name %q", at, entry.Category, name)
		}
	}
	if got := graph.AccountHistory("absent"); got != nil {
		t.Errorf("the history of an absent node = %+v, want nil", got)
	}
}
