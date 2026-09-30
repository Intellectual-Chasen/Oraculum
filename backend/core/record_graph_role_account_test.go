package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	subjectSid = "S-1-5-21-1000-2000-3000-1001"
	targetSid  = "S-1-5-21-1000-2000-3000-1002"
)

// roleAccountFields は、Subject と Target のアカウントを持つ 1 レコードの項目を組む。
func roleAccountFields(t *testing.T, subject, target string) []core.RecordField {
	t.Helper()
	return []core.RecordField{
		textField(t, "EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, subject),
		textField(t, "EventData.SubjectUserName", core.SemanticKeySubjectAccountName, "admin01"),
		textField(t, "EventData.SubjectDomainName", core.SemanticKeySubjectAccountDomain, "EXAMPLE"),
		textField(t, "EventData.TargetUserSid", core.SemanticKeyTargetAccountSid, target),
		textField(t, "EventData.TargetUserName", core.SemanticKeyTargetAccountName, "user02"),
		textField(t, "EventData.TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"),
	}
}

// referencedAccount は、参照のノードのうち SID が sid のアカウントを返す。
func referencedAccount(t *testing.T, graph core.RecordGraph, sid string) core.ReferencedNode {
	t.Helper()
	for _, referenced := range graph.ReferencedNodes {
		if referenced.Key.Kind == core.NodeKindAccount && len(referenced.Key.Values) == 1 &&
			referenced.Key.Values[0].Value == sid {
			return referenced
		}
	}
	t.Fatalf("no referenced account %q in %+v", sid, graph.ReferencedNodes)
	return core.ReferencedNode{}
}

// namingsOf は、レコードが SID が sid のアカウントへ付けた関係の種別を返す。
func namingsOf(graph core.RecordGraph, sid string) []core.EdgeKind {
	var kinds []core.EdgeKind
	for _, naming := range graph.Namings {
		if naming.Target.Values[0].Value == sid {
			kinds = append(kinds, naming.Kind)
		}
	}
	return kinds
}

// 役割を付けて記録したアカウントは、SID の鍵を持つ参照のノードになり、役割の種別を持つ。
func TestNewRecordGraphReferencesTheRoleAccountsBySid(t *testing.T) {
	graph := core.NewRecordGraph(roleAccountFields(t, subjectSid, targetSid))
	if len(graph.Nodes) != 0 {
		t.Errorf("the record graph names %+v, want the accounts only as references", graph.Nodes)
	}
	for sid, want := range map[string]struct {
		naming core.EdgeKind
		label  core.SemanticKey
	}{
		subjectSid: {core.EdgeKindRecordSubjectAccount, core.SemanticKeySubjectAccountName},
		targetSid:  {core.EdgeKindRecordTargetAccount, core.SemanticKeyTargetAccountName},
	} {
		account := referencedAccount(t, graph, sid)
		if account.Key.Form != core.NodeKeyFormAccountSid ||
			account.Key.Values[0].Semantic != core.SemanticKeyAccountSid {
			t.Errorf("%s key = %+v, want account_sid keyed by account.sid", sid, account.Key)
		}
		if err := account.Key.Validate(); err != nil {
			t.Errorf("%s key = %v, want a valid key", sid, err)
		}
		if got := namingsOf(graph, sid); !slices.Equal(got, []core.EdgeKind{want.naming}) {
			t.Errorf("%s namings = %v, want %v", sid, got, want.naming)
		}
		if account.LabelSemantic != want.label {
			t.Errorf("%s label = %q, want %q", sid, account.LabelSemantic, want.label)
		}
	}
}

// 同じ SID の 2 つの役割は 1 つのノードになり、2 つの種別を持つ。account.sid で記録した
// レコードと同じ識別鍵になる。
func TestNewRecordGraphMergesTheTwoRolesOfOneSid(t *testing.T) {
	graph := core.NewRecordGraph(roleAccountFields(t, subjectSid, subjectSid))
	var sids []core.ReferencedNode
	for _, referenced := range graph.ReferencedNodes {
		if referenced.Key.Form == core.NodeKeyFormAccountSid {
			sids = append(sids, referenced)
		}
	}
	if len(sids) != 1 {
		t.Fatalf("the record graph references %+v, want one SID account", graph.ReferencedNodes)
	}
	account := sids[0]
	for _, naming := range []core.EdgeKind{core.EdgeKindRecordSubjectAccount, core.EdgeKindRecordTargetAccount} {
		if got := namingsOf(graph, subjectSid); !slices.Contains(got, naming) {
			t.Errorf("namings = %v, want %q", got, naming)
		}
	}
	named := core.NewRecordGraph([]core.RecordField{
		textField(t, "sid", core.SemanticKeyAccountSid, subjectSid),
	})
	if len(named.Nodes) != 1 || !slices.Equal(named.Nodes[0].Values, account.Key.Values) ||
		named.Nodes[0].Form != account.Key.Form {
		t.Errorf("account.sid names %+v, want the same key as %+v", named.Nodes, account.Key)
	}
}

// account.sid と役割の項目が同じアカウントを指すと、ノードは account.sid の側の 1 つであり、
// 役割の種別も残る。
func TestNewRecordGraphKeepsTheRoleOfTheAccountNamedByAccountSid(t *testing.T) {
	graph := core.NewRecordGraph(append([]core.RecordField{
		textField(t, "sid", core.SemanticKeyAccountSid, subjectSid),
	}, roleAccountFields(t, subjectSid, targetSid)...))
	if len(graph.Nodes) != 1 || graph.Nodes[0].Values[0].Value != subjectSid {
		t.Fatalf("the record graph names %+v, want the account.sid account", graph.Nodes)
	}
	var namings []string
	for _, naming := range graph.Namings {
		if naming.Target.Form == core.NodeKeyFormAccountSid {
			namings = append(namings, string(naming.Kind)+" "+naming.Target.Values[0].Value)
		}
	}
	slices.Sort(namings)
	want := []string{
		string(core.EdgeKindRecordSubjectAccount) + " " + subjectSid,
		string(core.EdgeKindRecordTargetAccount) + " " + targetSid,
	}
	if !slices.Equal(namings, want) {
		t.Errorf("namings = %v, want %v", namings, want)
	}
}

// well-known な SID と SID の形でない文字列からは、アカウントのノードを作らない。NULL SID は
// 名前のノードを作る (TestRoleAccountWithoutSidItemIsKeyedByDomainAndName)。
func TestNewRecordGraphSkipsTheSidsOutsideTheAccountRange(t *testing.T) {
	for _, sid := range []string{"S-1-5-18", "S-1-5-32-544", "EXAMPLE\\user02", "S-1-5-21-"} {
		graph := core.NewRecordGraph(roleAccountFields(t, sid, sid))
		if len(graph.ReferencedNodes) != 0 {
			t.Errorf("%q references %+v, want no account", sid, graph.ReferencedNodes)
		}
	}
	absent := core.NewRecordGraph([]core.RecordField{
		absentField(t, "EventData.TargetUserSid", core.SemanticKeyTargetAccountSid),
	})
	if len(absent.ReferencedNodes) != 0 {
		t.Errorf("the absent sid references %+v, want no account", absent.ReferencedNodes)
	}
}

// 端末に置いたレコードは、役割を付けて記録したアカウントへ terminal_account を張る。
// 端末の範囲の対象を指さないレコードでも、アカウントは残る。
func TestNewRecordGraphLinksTheTerminalToTheRoleAccounts(t *testing.T) {
	recording, _ := core.RecordingTerminalNodeKey(
		"0000000000000000000000000000000000000000000000000000000000000002")
	fields := roleAccountFields(t, subjectSid, targetSid)

	placed := core.NewRecordGraphInScope(fields, core.RecordScope{
		Terminal: &recording, NamesTerminal: true,
	})
	// 2 つの役割の SID のノードと、並べた名前のノードである。
	requireStrings(t, "links", linkNames(placed.Links),
		"terminal_account terminal account", "terminal_account terminal account",
		"terminal_account terminal account", "terminal_account terminal account")

	unplaced := core.NewRecordGraphInScope(fields, core.RecordScope{Terminal: &recording})
	requireStrings(t, "node kinds", nodeKinds(unplaced.Nodes))
	if len(unplaced.ReferencedNodes) != 4 {
		t.Errorf("the unplaced record references %+v, want both accounts by SID and by name", unplaced.ReferencedNodes)
	}
}

// レコードのアカウントは account.*、target、subject の順で選ぶ。役割が SID と名前を共に記録した
// ときは名前のノードを選ぶ。Target の SID が NULL SID のログオンの失敗は、subject へ戻らず Target の
// 名前のノードを選ぶ。
func TestAccountNodeKeyPrefersTheTargetAndKeepsTheFailedLogonWithoutSubject(t *testing.T) {
	accountOf := func(fields []core.RecordField) string {
		key, named := core.AccountNodeKey(fields, core.RecordScope{})
		if !named {
			return ""
		}
		return key.Values[len(key.Values)-1].Value
	}
	if got := accountOf(roleAccountFields(t, subjectSid, targetSid)); got != "user02" {
		t.Errorf("the account of a logon = %q, want the target name user02", got)
	}
	failed, _ := core.AccountNodeKey(roleAccountFields(t, subjectSid, "S-1-0-0"), core.RecordScope{})
	if failed.Form != core.NodeKeyFormAccountDomainName || failed.Values[1].Value != "user02" {
		t.Errorf("the account of a failed logon = %+v, want the target name user02", failed)
	}
	subjectOnly := roleAccountFields(t, subjectSid, targetSid)[:3]
	if got := accountOf(subjectOnly); got != "admin01" {
		t.Errorf("the account of a subject-only record = %q, want the subject name admin01", got)
	}
	named := append([]core.RecordField{
		textField(t, "sid", core.SemanticKeyAccountSid, "S-1-5-21-1000-2000-3000-1009"),
	}, roleAccountFields(t, subjectSid, targetSid)...)
	if got := accountOf(named); got != "S-1-5-21-1000-2000-3000-1009" {
		t.Errorf("the account of a record naming account.sid = %q, want the account.sid", got)
	}
}

// SID のノードを作る役割は、同じレコードが書いたドメインとログイン名の組のノードも並べて
// 指す。SID と名前が別のアカウントを指す記録でも、名前のノードから辿れる。
func TestRoleAccountWithSidAlsoNamesTheDomainAndName(t *testing.T) {
	fields := roleAccountFields(t, subjectSid, targetSid)
	var keys []string
	for _, key := range core.RoleAccountNodeKeys(fields) {
		var values []string
		for _, value := range key.Values {
			values = append(values, value.Value)
		}
		keys = append(keys, string(key.Form)+" "+strings.Join(values, `\`))
	}
	want := []string{
		string(core.NodeKeyFormAccountSid) + " " + targetSid,
		string(core.NodeKeyFormAccountDomainName) + ` EXAMPLE\user02`,
		string(core.NodeKeyFormAccountSid) + " " + subjectSid,
		string(core.NodeKeyFormAccountDomainName) + ` EXAMPLE\admin01`,
	}
	if !slices.Equal(keys, want) {
		t.Errorf("RoleAccountNodeKeys = %q, want %q", keys, want)
	}
	if key, _ := core.AccountNodeKey(fields, core.RecordScope{}); key.Form != core.NodeKeyFormAccountDomainName ||
		key.Values[1].Value != "user02" {
		t.Errorf("AccountNodeKey = %+v, want the name of the target", key)
	}
}

// 役割の SID の欄を持たないレコードのアカウントは、ドメインとログイン名の組を鍵にする。
// SID の欄の値が NULL SID だけのときも名前を鍵にする。well-known な SID では名前へ戻らない。
func TestRoleAccountWithoutSidItemIsKeyedByDomainAndName(t *testing.T) {
	nameOnly := []core.RecordField{
		textField(t, "EventData.SubjectUserName", core.SemanticKeySubjectAccountName, "admin01"),
		textField(t, "EventData.SubjectDomainName", core.SemanticKeySubjectAccountDomain, "EXAMPLE"),
	}
	want := core.NodeKey{Kind: core.NodeKindAccount, Form: core.NodeKeyFormAccountDomainName,
		Values: []core.NodeIdentityValue{
			{Semantic: core.SemanticKeyAccountDomain, Value: "EXAMPLE"},
			{Semantic: core.SemanticKeyAccountName, Value: "admin01"},
		}}
	keys := core.RoleAccountNodeKeys(nameOnly)
	if len(keys) != 1 || keys[0].Form != want.Form || !slices.Equal(keys[0].Values, want.Values) {
		t.Fatalf("RoleAccountNodeKeys = %+v, want %+v", keys, want)
	}
	graph := core.NewRecordGraph(nameOnly)
	if len(graph.Namings) != 1 || graph.Namings[0].Kind != core.EdgeKindRecordSubjectAccount {
		t.Errorf("Namings = %+v, want one subject naming", graph.Namings)
	}
	withNullSid := append(slices.Clone(nameOnly),
		textField(t, "EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, "S-1-0-0"))
	if keys := core.RoleAccountNodeKeys(withNullSid); len(keys) != 1 || !slices.Equal(keys[0].Values, want.Values) {
		t.Errorf("RoleAccountNodeKeys with a NULL SID = %+v, want %+v", keys, want)
	}
	withSystemSid := append(slices.Clone(nameOnly),
		textField(t, "EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, "S-1-5-18"))
	if keys := core.RoleAccountNodeKeys(withSystemSid); len(keys) != 0 {
		t.Errorf("RoleAccountNodeKeys with a well-known SID = %+v, want none", keys)
	}
	if keys := core.RoleAccountNodeKeys(nameOnly[:1]); len(keys) != 0 {
		t.Errorf("RoleAccountNodeKeys without the domain = %+v, want none", keys)
	}
}
