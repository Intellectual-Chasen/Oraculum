// in-package test: 非公開の構築子で取り込み結果を組み、候補のエッジの詳細が持つレコードの組を検査する。
package pipeline

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// pairSides は、組の両側のレコードの g.records での位置を返す。レコードを持たない側は
// noPairRecord である。
func pairSides(t *testing.T, graph Graph, pair core.EdgeRecordPair) [2]int {
	t.Helper()
	sides := [2]int{noPairRecord, noPairRecord}
	for index, side := range []*core.GraphEvidence{pair.Left, pair.Right} {
		if side == nil {
			continue
		}
		at, found := graph.recordAtLocator(side.RecordRef)
		if !found {
			t.Fatalf("the pair names a record the graph does not hold: %+v", side.RecordRef)
		}
		sides[index] = at
	}
	return sides
}

// conditionKeysOf は組の条件の種別を並びのまま返す。
func conditionKeysOf(pair core.EdgeRecordPair) []core.EdgePairConditionKey {
	keys := make([]core.EdgePairConditionKey, 0, len(pair.Conditions))
	for _, condition := range pair.Conditions {
		keys = append(keys, condition.ConditionKey)
	}
	return keys
}

// comparableValuesOf は欄の比べられる値を並びのまま返す。
func comparableValuesOf(fields []core.RecordField) []string {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value, readable := comparableTextOf(field); readable {
			values = append(values, value)
		}
	}
	return values
}

// requireRecordPairs は、kind のエッジのすべてが、want の条件の種別を並びのまま持つレコードの
// 組を 1 つ以上持ち、組の両側がエッジの根拠であり、関係の起点と終点に対応することを確かめる。
//
// 両端がレコードのノードのエッジでは、両側のレコードがそのノードである。両端がプロセスの
// エッジでは、両側のレコードがそのプロセスを記録したレコードである。
func requireRecordPairs(t *testing.T, graph Graph, kind core.EdgeKind, want ...core.EdgePairConditionKey) {
	t.Helper()
	edges := edgePairsOfKind(graph, kind)
	if len(edges) == 0 {
		t.Fatalf("the graph holds no %s edge", kind)
	}
	for _, ends := range edges {
		detail := edgeDetailOfKind(t, graph, kind, ends[0], ends[1])
		if len(detail.RecordPairs) == 0 {
			t.Errorf("the %s edge carries no record pair", kind)
		}
		edge := graph.edges[graph.edgeAt[detail.Edge.Id]]
		for _, pair := range detail.RecordPairs {
			if keys := conditionKeysOf(pair); !slices.Equal(keys, want) {
				t.Errorf("a %s pair carries the conditions %v, want %v", kind, keys, want)
			}
			for index, at := range pairSides(t, graph, pair) {
				if at == noPairRecord {
					continue
				}
				if !slices.Contains(edge.evidence, at) {
					t.Errorf("a %s pair names the record %d outside the evidence", kind, at)
				}
				end := ends[index]
				switch graph.nodes[end].key.Kind {
				case core.NodeKindRecord:
					if graph.records[at].recordNode != end {
						t.Errorf("the side %d of a %s pair is not the record of the end", index, kind)
					}
				case core.NodeKindProcess:
					if process, recorded := graph.recordedProcessAt(at); !recorded || process != end {
						t.Errorf("the side %d of a %s pair does not record the process of the end", index, kind)
					}
				}
			}
		}
	}
}

// edgeDetailOfKind は、kind のエッジのうち source から target へのエッジの詳細を返す。
func edgeDetailOfKind(t *testing.T, graph Graph, kind core.EdgeKind, source, target int) EdgeDetail {
	t.Helper()
	detail, found := graph.EdgeDetail(edgeIdOf(kind, graph.nodes[source].id, graph.nodes[target].id),
		EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("no %s edge from %s to %s", kind, graph.nodes[source].id, graph.nodes[target].id)
	}
	for _, pair := range detail.RecordPairs {
		if err := pair.Validate(); err != nil {
			t.Errorf("a record pair = %v, want a valid pair", err)
		}
	}
	return detail
}

// 推定した親子の詳細が、親の起動のレコードと子の起動のレコードの組を、プロセス番号と端末と
// 時刻の順の条件で持つ。子の根拠だけでは、親をどのレコードから選んだかを読めない。
func TestProcessParentChildDetailPairsTheParentStartWithTheChild(t *testing.T) {
	graph := viewerCSVGraph(t,
		processCreationCSV("2001/02/03 04:05:07", "HOST01$", "0x14", `C:\Example\child.exe`, "0x10"),
		processCreationCSV("2001/02/03 04:05:05", "HOST01$", "0x10", `C:\Example\parent.exe`, "0x4"),
	)
	parentAt := processNodeLabelled(t, graph, `C:\Example\parent.exe`)
	childAt := processNodeLabelled(t, graph, `C:\Example\child.exe`)
	detail := edgeDetailOfKind(t, graph, core.EdgeKindProcessParentChild, parentAt, childAt)
	if detail.RecordPairCount != 1 || len(detail.RecordPairs) != 1 {
		t.Fatalf("the edge carries %d pairs, want 1", detail.RecordPairCount)
	}
	pair := detail.RecordPairs[0]
	// CSV は新しい順に並び、子の作成が先頭のレコードである。
	if sides := pairSides(t, graph, pair); sides != [2]int{1, 0} {
		t.Errorf("the pair joins the records %v, want the parent creation 1 and the child creation 0", sides)
	}
	want := []core.EdgePairConditionKey{
		core.EdgePairConditionProcessPid, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder,
	}
	if keys := conditionKeysOf(pair); !slices.Equal(keys, want) {
		t.Fatalf("the pair carries the conditions %v, want %v", keys, want)
	}
	pid := pair.Conditions[0]
	left, right := comparableValuesOf(pid.LeftValue), comparableValuesOf(pid.RightValue)
	if len(left) != 1 || !slices.Equal(left, right) {
		t.Errorf("the pid condition compares %v with %v, want the parent pid on both sides", left, right)
	}
	if pid.LeftValue[0].Semantic != core.SemanticKeyProcessPid ||
		pid.RightValue[0].Semantic != core.SemanticKeyParentProcessPid {
		t.Errorf("the pid condition reads %s and %s, want the pid of the parent and the parent pid of the child",
			pid.LeftValue[0].Semantic, pid.RightValue[0].Semantic)
	}
	// 端末の欄を持たない CSV のレコードは収集元の端末に置く。条件は置いた端末を両側に持つ。
	terminal := pair.Conditions[1]
	leftTerminal, rightTerminal := comparableValuesOf(terminal.LeftValue), comparableValuesOf(terminal.RightValue)
	if len(leftTerminal) != 1 || !slices.Equal(leftTerminal, rightTerminal) {
		t.Errorf("the terminal condition compares %v with %v, want the placed terminal on both sides",
			leftTerminal, rightTerminal)
	}
	if pair.Left.EventTime == nil || pair.Right.EventTime == nil {
		t.Error("a side of the pair carries no event time for the time order")
	}
}

// ログオンと操作の候補の詳細が、ログオンと操作のレコードの組と、Logon ID の両側の値を持つ。
func TestLogonSessionDetailPairsTheLogonWithTheOperation(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	detail := edgeDetailOfKind(t, graph, core.EdgeKindLogonSessionOperation, nodes["31"], nodes["33"])
	if len(detail.RecordPairs) != 1 {
		t.Fatalf("the edge carries %d pairs, want 1", len(detail.RecordPairs))
	}
	pair := detail.RecordPairs[0]
	want := [2]int{recordOfEvent(t, graph, "31"), recordOfEvent(t, graph, "33")}
	if sides := pairSides(t, graph, pair); sides != want {
		t.Fatalf("the pair joins the records %v, want the logon and the operation %v", sides, want)
	}
	logonId := pair.Conditions[0]
	if logonId.ConditionKey != core.EdgePairConditionLogonId {
		t.Fatalf("the first condition is %s, want logon_id", logonId.ConditionKey)
	}
	left, right := comparableValuesOf(logonId.LeftValue), comparableValuesOf(logonId.RightValue)
	if len(left) != 1 || !slices.Equal(left, right) {
		t.Errorf("the logon id condition compares %v with %v, want the same Logon ID", left, right)
	}
}

// 接続元が未同定の遠隔のセッションの組は、起点の側にレコードを持たず、終点の側のログオンの
// 接続元のアドレスを持つ。
func TestUnidentifiedSourceSessionPairsTheLogonAlone(t *testing.T) {
	result := windowsEventSessionResult(t, sessionClientDocument, sessionServerDocument)
	graph := NewGraph(result, AllMatchConditions())
	edges := edgePairsOfKind(graph, core.EdgeKindUnidentifiedSourceRemoteSession)
	if len(edges) == 0 {
		t.Fatal("the graph holds no unidentified source session")
	}
	for _, ends := range edges {
		detail := edgeDetailOfKind(t, graph, core.EdgeKindUnidentifiedSourceRemoteSession, ends[0], ends[1])
		if detail.RecordPairCount == 0 || detail.RecordPairCount != len(detail.Evidence) {
			t.Errorf("the edge carries %d pairs for %d evidence records, want one pair per record",
				detail.RecordPairCount, detail.Edge.EvidenceCount)
		}
		address, _ := graph.nodes[ends[0]].key.LabelValue()
		for _, pair := range detail.RecordPairs {
			if pair.Left != nil || pair.Right == nil {
				t.Fatalf("the pair joins %+v and %+v, want the logon on the target side alone", pair.Left, pair.Right)
			}
			condition := pair.Conditions[0]
			if condition.ConditionKey != core.EdgePairConditionSourceUnassigned ||
				!slices.Equal(comparableValuesOf(condition.RightValue), []string{address}) {
				t.Errorf("the pair carries %s with %v, want source_unassigned with %s",
					condition.ConditionKey, comparableValuesOf(condition.RightValue), address)
			}
		}
	}
}

// 名前のアカウントと SID の候補の組は、名前のレコードと、その時刻の前後で同じ名前と SID を
// 記録したレコードである。
func TestAccountIdentityPairsTheNamedRecordWithTheSidRecords(t *testing.T) {
	const (
		oldSid = "S-1-5-21-7-8-9-1301"
		newSid = "S-1-5-21-7-8-9-1302"
	)
	named := func(recordID, at string) string {
		return securityEventXML(recordID, "4648", "dc.example.test", at,
			"TargetUserName", "USER-M", "TargetDomainName", "example")
	}
	document := "<Events>\n" +
		named("31", "2001-02-03T03:50:00Z") +
		accountEventXML("32", "4726", "2001-02-03T04:00:00Z", oldSid) +
		named("33", "2001-02-03T04:02:00Z") +
		accountEventXML("34", "4720", "2001-02-03T04:05:00Z", newSid) +
		named("35", "2001-02-03T04:30:00Z") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	want := map[string][2]int{
		oldSid: {recordOfEvent(t, graph, "31"), recordOfEvent(t, graph, "32")},
		newSid: {recordOfEvent(t, graph, "35"), recordOfEvent(t, graph, "34")},
	}
	for _, ends := range edgePairsOfKind(graph, core.EdgeKindAccountIdentityMatch) {
		sid, _ := graph.nodes[ends[1]].key.LabelValue()
		detail := edgeDetailOfKind(t, graph, core.EdgeKindAccountIdentityMatch, ends[0], ends[1])
		if len(detail.RecordPairs) != 1 {
			t.Fatalf("the edge to %s carries %d pairs, want 1", sid, len(detail.RecordPairs))
		}
		if sides := pairSides(t, graph, detail.RecordPairs[0]); sides != want[sid] {
			t.Errorf("the edge to %s pairs %v, want %v", sid, sides, want[sid])
		}
		// SID のレコードは名前のレコードの直後 (31 と 32) のことも直前 (35 と 34) のこともある。
		wantKeys := []core.EdgePairConditionKey{
			core.EdgePairConditionAccount, core.EdgePairConditionNearestIdentityRecord,
		}
		if keys := conditionKeysOf(detail.RecordPairs[0]); !slices.Equal(keys, wantKeys) {
			t.Errorf("the edge to %s carries the conditions %v, want %v", sid, keys, wantKeys)
		}
		delete(want, sid)
	}
	if len(want) != 0 {
		t.Errorf("no account identity edge to %v", want)
	}
}

// 名前だけの役割と SID を持つ役割を 1 件に持つレコードは、別の名前のレコードの SID の記録として
// 先に根拠に入っても、自らの組を持つ。
func TestAccountIdentityPairsTheNamedRecordThatAlsoRecordsTheSid(t *testing.T) {
	const sid = "S-1-5-21-7-8-9-1401"
	// 先に同じ名前を別の SID と共に記録した 4726 があり、名前は 1 つの SID に寄らない。
	document := "<Events>\n" +
		securityEventXML("39", "4726", "dc.example.test", "2001-02-03T03:00:00Z",
			"TargetSid", "S-1-5-21-7-8-9-1402", "TargetUserName", "USER-N", "TargetDomainName", "EXAMPLE") +
		securityEventXML("40", "4738", "dc.example.test", "2001-02-03T03:30:00Z",
			"TargetSid", sid, "TargetUserName", "USER-N", "TargetDomainName", "EXAMPLE") +
		securityEventXML("41", "4648", "dc.example.test", "2001-02-03T04:00:00Z",
			"TargetUserName", "USER-N", "TargetDomainName", "example") +
		securityEventXML("42", "4648", "dc.example.test", "2001-02-03T04:01:00Z",
			"SubjectUserSid", sid, "SubjectUserName", "USER-N", "SubjectDomainName", "EXAMPLE",
			"TargetUserName", "USER-N", "TargetDomainName", "example") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	edges := edgePairsOfKind(graph, core.EdgeKindAccountIdentityMatch)
	if len(edges) != 1 {
		t.Fatalf("the graph holds %d account identity edges, want 1", len(edges))
	}
	detail := edgeDetailOfKind(t, graph, core.EdgeKindAccountIdentityMatch, edges[0][0], edges[0][1])
	lefts := map[int]bool{}
	for _, pair := range detail.RecordPairs {
		lefts[pairSides(t, graph, pair)[0]] = true
	}
	if !lefts[recordOfEvent(t, graph, "41")] || !lefts[recordOfEvent(t, graph, "42")] {
		t.Errorf("the pairs start at the records %v, want both named records 41 and 42", lefts)
	}
}

// 応答は先頭の maxResponsePairs 組だけを含み、総数を別に持つ。
func TestEdgeDetailCutsTheRecordPairsAtTheLimit(t *testing.T) {
	var document strings.Builder
	document.WriteString("<Events>\n")
	// 前日に同じ名前を別の SID と共に記録した 4726 があり、名前は 1 つの SID に寄らない。
	document.WriteString(securityEventXML("0", "4726", "dc.example.test", "2001-02-02T00:00:00Z",
		"TargetSid", "S-1-5-21-7-8-9-1502", "TargetUserName", "USER-P", "TargetDomainName", "EXAMPLE"))
	document.WriteString(securityEventXML("1", "4726", "dc.example.test", "2001-02-03T00:00:00Z",
		"TargetSid", "S-1-5-21-7-8-9-1501", "TargetUserName", "USER-P", "TargetDomainName", "EXAMPLE"))
	for index := range maxResponsePairs + 5 {
		document.WriteString(securityEventXML(strconv.Itoa(index+2), "4648", "dc.example.test",
			"2001-02-03T01:00:00Z", "TargetUserName", "USER-P", "TargetDomainName", "example"))
	}
	document.WriteString("</Events>\n")
	graph := NewGraph(windowsEventImportResult(t, document.String()), AllMatchConditions())
	edges := edgePairsOfKind(graph, core.EdgeKindAccountIdentityMatch)
	if len(edges) != 1 {
		t.Fatalf("the graph holds %d account identity edges, want 1", len(edges))
	}
	detail := edgeDetailOfKind(t, graph, core.EdgeKindAccountIdentityMatch, edges[0][0], edges[0][1])
	if detail.RecordPairCount != maxResponsePairs+5 || len(detail.RecordPairs) != maxResponsePairs {
		t.Errorf("the detail carries %d of %d pairs, want %d of %d",
			len(detail.RecordPairs), detail.RecordPairCount, maxResponsePairs, maxResponsePairs+5)
	}
}

// logonWithPackageCSV は、認証の方式を記録した 4624 の論理レコードを返す。
func logonWithPackageCSV(account, address, authenticationPackage string) string {
	logon := logonCSV(account, address)
	return logon[:len(logon)-2] + "\n詳細な認証情報:\n\tログオン プロセス:\t\tNtLmSsp\n\t認証パッケージ:\t" +
		authenticationPackage + "\n\"\n"
}

// 根拠の区分が、区分のレコードが記録した認証の方式を件数とともに持つ。
func TestEvidenceGroupsCountTheAuthenticationPackages(t *testing.T) {
	graph := viewerCSVGraph(t,
		logonWithPackageCSV("user01", "192.0.2.10", "NTLM"),
		logonWithPackageCSV("user01", "192.0.2.10", "Kerberos"),
		logonWithPackageCSV("user01", "192.0.2.10", "NTLM"),
	)
	groups := graph.evidenceGroups([]int{0, 1, 2})
	if len(groups) != 1 {
		t.Fatalf("the records fall into %d groups, want 1", len(groups))
	}
	if err := groups[0].Validate(); err != nil {
		t.Fatalf("the group = %v, want a valid group", err)
	}
	got := map[string]int64{}
	for _, authentication := range groups[0].Authentications {
		value, _ := comparableTextOf(authentication.Value)
		got[value] = authentication.EvidenceCount
	}
	if len(got) != 2 || got["NTLM"] != 2 || got["Kerberos"] != 1 {
		t.Errorf("the group counts the packages %v, want NTLM 2 and Kerberos 1", got)
	}
	if accounts := groups[0].Accounts; len(accounts) != 1 || accounts[0].EvidenceCount != 3 {
		t.Errorf("the group names the accounts %+v, want the logon account on the 3 records", accounts)
	}
}
