// in-package test: Windows イベントログの Subject と Target のアカウントのノードと、役割の関係を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	roleTargetSid  = "S-1-5-21-1000-2000-3000-1101"
	roleSubjectSid = "S-1-5-21-1000-2000-3000-1102"
)

// roleAccountGraph は、同じ Target の 2 件のログオンと、Target の SID が NULL SID の
// ログオンの失敗を 1 つの file に並べてグラフを組む。
func roleAccountGraph(t *testing.T) Graph {
	t.Helper()
	logon := func(recordID string) string {
		return securityEventXML(recordID, "4624", logonHostA, "2001-02-03T04:05:10.000Z",
			"SubjectUserSid", "S-1-5-18", "SubjectUserName", "HOST-A$",
			"TargetUserSid", roleTargetSid, "TargetUserName", "user11",
			"TargetDomainName", "EXAMPLE", "LogonType", "3")
	}
	failure := securityEventXML("53", "4625", logonHostA, "2001-02-03T04:05:30.000Z",
		"SubjectUserSid", roleSubjectSid, "SubjectUserName", "admin12",
		"TargetUserSid", "S-1-0-0", "TargetUserName", "missing13", "TargetDomainName", "EXAMPLE", "LogonType", "3")
	document := "<Events>\n" + logon("51") + logon("52") + failure + "</Events>\n"
	return NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
}

// recordOfEvent は EventRecordID が recordID のレコードの位置を返す。
func recordOfEvent(t *testing.T, graph Graph, recordID string) int {
	t.Helper()
	for at, record := range graph.records {
		if !record.hasRecordNode {
			continue
		}
		for _, attribute := range graph.nodes[record.recordNode].attributes {
			if attribute.field.Name != "EventRecordID" {
				continue
			}
			if value, _ := attribute.field.Text.RawTextValue(); value == recordID {
				return at
			}
		}
	}
	t.Fatalf("no record carries the EventRecordID %s", recordID)
	return 0
}

// accountNodeOfSid は SID が sid のアカウントのノードの位置を返す。見つからなければ -1 である。
func accountNodeOfSid(graph Graph, sid string) int {
	for at, node := range graph.nodes {
		if node.key.Kind == core.NodeKindAccount && len(node.key.Values) == 1 &&
			node.key.Values[0].Value == sid {
			return at
		}
	}
	return -1
}

// edgeKindsBetween は source から target へのエッジの種別を返す。
func edgeKindsBetween(graph Graph, source, target int) []string {
	var kinds []string
	for _, edge := range graph.edges {
		if edge.source == source && edge.target == target {
			kinds = append(kinds, string(edge.kind))
		}
	}
	return kinds
}

// 2 件のログオンの Target は 1 つのアカウントのノードにまとまり、各レコードのノードから
// record_target_account で結ばれる。well-known な Subject はノードにならない。
func TestRoleAccountsBecomeOneSidNodeLinkedByTheRole(t *testing.T) {
	graph := roleAccountGraph(t)
	account := accountNodeOfSid(graph, roleTargetSid)
	if account < 0 {
		t.Fatalf("the graph holds no account node for %s", roleTargetSid)
	}
	if node := graph.nodes[account]; node.observation != core.NodeObservationReferenced ||
		node.key.Form != core.NodeKeyFormAccountSid {
		t.Errorf("the account node is %s/%s, want referenced/account_sid", node.observation, node.key.Form)
	}
	if label, _ := graph.nodes[account].label.RawTextValue(); label != "user11" {
		t.Errorf("the account label = %q, want user11", label)
	}
	if accountNodeOfSid(graph, "S-1-5-18") >= 0 {
		t.Error("the graph holds a node for the well-known subject")
	}
	for _, recordID := range []string{"51", "52"} {
		at := recordOfEvent(t, graph, recordID)
		kinds := strings.Join(edgeKindsBetween(graph, graph.records[at].recordNode, account), ",")
		if kinds != string(core.EdgeKindRecordTargetAccount) {
			t.Errorf("record %s links the account by %q, want only record_target_account", recordID, kinds)
		}
		// レコードのアカウントは、そのレコードが記録した名前のノードである。
		named := graph.nodes[graph.nodeAt[graph.records[at].accountNodeId]]
		if label, _ := named.label.RawTextValue(); named.key.Form != core.NodeKeyFormAccountDomainName || label != "user11" {
			t.Errorf("record %s names the account %v labelled %q, want the name node user11", recordID, named.key, label)
		}
	}
	terminalLinks := 0
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTerminalAccount && edge.target == account {
			terminalLinks++
		}
	}
	if terminalLinks != 1 {
		t.Errorf("the account has %d terminal_account edges, want one from the recording terminal", terminalLinks)
	}
}

func TestAccountTimelineKeepsRoleAndOtherAccountOfTicketRequest(t *testing.T) {
	document := "<Events>\n" + securityEventXML("71", "4769", logonHostA, "2001-02-03T04:05:10.000Z",
		"TargetUserSid", roleSubjectSid, "TargetUserName", "requester",
		"TargetDomainName", "EXAMPLE", "ServiceSid", roleTargetSid,
		"ServiceName", "service", "IpAddress", "192.0.2.42") +
		securityEventXML("72", "4769", logonHostA, "2001-02-03T04:05:20.000Z",
			"TargetUserSid", roleSubjectSid, "TargetUserName", "requester",
			"TargetDomainName", "EXAMPLE", "ServiceSid", "S-1-5-21-1000-2000-3000-1103",
			"ServiceName", "service", "IpAddress", "192.0.2.43") + "</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	account := accountNodeOfSid(graph, roleTargetSid)
	if account < 0 {
		t.Fatal("the service account has no SID node")
	}
	timeline := graph.Timeline(TimelineQuery{AccountNodeId: graph.nodes[account].id})
	if len(timeline.Entries) != 1 {
		t.Fatalf("service account timeline = %+v, want the ticket request", timeline.Entries)
	}
	entry := timeline.Entries[0]
	if entry.RecordRef.RecordRawTextRef != graph.records[recordOfEvent(t, graph, "71")].locator.RecordRawTextRef {
		t.Errorf("account history included the other SID's record: %+v", entry.RecordRef)
	}
	if !slices.Contains(entry.AccountRoles, core.EdgeKindRecordTargetAccount) {
		t.Errorf("account roles = %v, want target", entry.AccountRoles)
	}
	if entry.EventKind == nil || entry.EventKind.Category == "" || entry.EventKind.Action != "4769" {
		t.Errorf("event kind = %+v, want Windows event 4769", entry.EventKind)
	}
	if !slices.ContainsFunc(entry.OtherAccounts, func(account core.TimelineAccount) bool {
		return account.Role == core.EdgeKindRecordSubjectAccount && account.Node.KeyForm == core.NodeKeyFormAccountDomainName &&
			len(account.Node.Identity) > 0 && account.Node.Identity[len(account.Node.Identity)-1].Value == "requester"
	}) {
		t.Errorf("other accounts = %+v, want the requester with domain and name", entry.OtherAccounts)
	}
	if entry.SourceAddress != "192.0.2.42" {
		t.Errorf("source address = %q", entry.SourceAddress)
	}
}

// Windows イベントログのレコードのノードの表示名は、Event ID を先頭に置き、収集元の file 名と
// EventRecordID を続ける。
func TestWindowsEventRecordLabelStartsWithTheEventId(t *testing.T) {
	graph := roleAccountGraph(t)
	at := recordOfEvent(t, graph, "53")
	label, present := graph.nodes[graph.records[at].recordNode].label.NormalizedValue()
	if want := "4625 events-1.xml EventRecordID 53"; !present || label != want {
		t.Errorf("the record label = %q, want %q", label, want)
	}
}

// レコードのノードの表示名は、位置の指し方ごとに位置の名前と値を file 名に続ける。
func TestRecordLabelOfNamesThePosition(t *testing.T) {
	number := func(value int64) *int64 { return &value }
	for name, item := range map[string]struct {
		locator                  core.RecordLocator
		eventID, recordNumber    string
		wantText, wantDerivation string
	}{
		"行単位のログ": {core.RecordLocator{SourceFileName: "a.log",
			PositionKind: core.PositionKindLineNumber, LineNumber: number(7)},
			"", "", "a.log 行 7", derivationRecordPositionFromLocator},
		"行番号を持つ byte の範囲": {core.RecordLocator{SourceFileName: "audit.log",
			PositionKind: core.PositionKindByteRange, LineNumber: number(3), ByteOffset: number(120)},
			"", "", "audit.log 行 3", derivationRecordPositionFromLocator},
		"通番": {core.RecordLocator{SourceFileName: "b.csv",
			PositionKind: core.PositionKindSequenceNumber, SequenceNumber: number(5), LineNumber: number(6)},
			"", "", "b.csv ID 5", derivationRecordPositionFromLocator},
		"byte の範囲": {core.RecordLocator{SourceFileName: "c.dat",
			PositionKind: core.PositionKindByteRange, ByteOffset: number(4096)},
			"", "", "c.dat 位置 4096", derivationRecordPositionFromLocator},
		"EventRecordID": {core.RecordLocator{SourceFileName: "d.evtx",
			PositionKind: core.PositionKindByteRange, ByteOffset: number(4096)},
			"4624", "81", "4624 d.evtx EventRecordID 81", derivationRecordNumberWithEventId},
		"EventRecordID を読めない": {core.RecordLocator{SourceFileName: "d.evtx",
			PositionKind: core.PositionKindByteRange, ByteOffset: number(4096)},
			"4624", "", "4624 d.evtx 位置 4096", derivationRecordPositionWithEventId},
	} {
		t.Run(name, func(t *testing.T) {
			label, derived := recordLabelOf(item.locator, item.eventID, item.recordNumber)
			text, _ := label.NormalizedValue()
			if !derived || text != item.wantText || label.Derivation == nil || *label.Derivation != item.wantDerivation {
				t.Errorf("recordLabelOf = %+v (%v), want %q derived by %q", label, derived, item.wantText, item.wantDerivation)
			}
		})
	}
	if _, derived := recordLabelOf(core.RecordLocator{SourceFileName: "e.log",
		PositionKind: core.PositionKindByteRange}, "", ""); derived {
		t.Error("recordLabelOf derived a label for a locator without a position")
	}
}

// SID の鍵と名前の鍵が 1 つのアカウントのノードへ寄ったレコードは、端末からアカウントへの
// エッジの根拠に 1 回だけ入る。
func TestMergedAccountKeysAddTheRecordToTheEdgeEvidenceOnce(t *testing.T) {
	graph := roleAccountGraph(t)
	account := accountNodeOfSid(graph, roleTargetSid)
	if account < 0 {
		t.Fatalf("the graph holds no account node for %s", roleTargetSid)
	}
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindTerminalAccount || edge.target != account {
			continue
		}
		seen := make(map[int]bool, len(edge.evidence))
		for _, at := range edge.evidence {
			if seen[at] {
				t.Errorf("the terminal_account edge holds the record %d twice in %v", at, edge.evidence)
			}
			seen[at] = true
		}
		if len(seen) != 2 {
			t.Errorf("the terminal_account edge holds %d records, want the 2 logons", len(seen))
		}
	}
}

// ログオンの失敗の Target の NULL SID は、試行された名前のノードになる。名前はレコードの属性にも
// 残る。レコードのアカウントは Subject へ戻らない。
func TestRoleAccountsOfAFailedLogonNameTheMissingTarget(t *testing.T) {
	graph := roleAccountGraph(t)
	at := recordOfEvent(t, graph, "53")
	record := graph.records[at]
	if record.accountNodeId == "" {
		t.Fatal("the failed logon names no account, want the tried name")
	}
	tried := graph.nodeAt[record.accountNodeId]
	if key := graph.nodes[tried].key; key.Form != core.NodeKeyFormAccountDomainName ||
		key.Values[len(key.Values)-1].Value != "missing13" {
		t.Errorf("the failed logon names %+v, want the name missing13", key)
	}
	subject := accountNodeOfSid(graph, roleSubjectSid)
	if subject < 0 {
		t.Fatalf("the graph holds no account node for the subject %s", roleSubjectSid)
	}
	if kinds := edgeKindsBetween(graph, record.recordNode, subject); len(kinds) != 1 ||
		kinds[0] != string(core.EdgeKindRecordSubjectAccount) {
		t.Errorf("the failed logon links the subject by %v, want record_subject_account", kinds)
	}
	named := false
	for _, attribute := range graph.nodes[record.recordNode].attributes {
		if attribute.field.Semantic == core.SemanticKeyTargetAccountName {
			value, _ := attribute.field.Text.RawTextValue()
			named = value == "missing13"
		}
	}
	if !named {
		t.Error("the failed logon record does not keep the target name as an attribute")
	}
}
