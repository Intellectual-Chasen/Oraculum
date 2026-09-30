// in-package test: 同じアカウントとしてまとめる鍵を、アカウントのノードに付けることを確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// accountNameOfSid は、その SID のノードの応答の鍵と理由を返す。
func accountNameOfSid(t *testing.T, graph Graph, sid string) (*core.AccountNameKey, core.AccountNameWithheldReason) {
	t.Helper()
	node := accountNodeOfSid(graph, sid)
	if node < 0 {
		t.Fatalf("no account node carries the SID %s", sid)
	}
	response := graph.graphNode(node)
	if err := response.Validate(); err != nil {
		t.Fatalf("the SID node %s fails the validation: %v", sid, err)
	}
	return response.AccountName, response.AccountNameWithheld
}

// accountNameOfNamedRecord は、レコードが指す名前のノードの応答の鍵と理由を返す。
func accountNameOfNamedRecord(t *testing.T, graph Graph, recordID string) (*core.AccountNameKey, core.AccountNameWithheldReason) {
	t.Helper()
	node := nameNodeNamedBy(t, graph, recordOfEvent(t, graph, recordID))
	if node < 0 {
		t.Fatalf("the record %s names no name node", recordID)
	}
	response := graph.graphNode(node)
	if err := response.Validate(); err != nil {
		t.Fatalf("the name node of the record %s fails the validation: %v", recordID, err)
	}
	return response.AccountName, response.AccountNameWithheld
}

func sidRecordXML(recordID, at, sid, domain, name string) string {
	return securityEventXML(recordID, "4720", "dc.example.test", at,
		"TargetSid", sid, "TargetUserName", name, "TargetDomainName", domain)
}

func TestSidAccountNameWithholdsWhenNoNameWasRecorded(t *testing.T) {
	got := sidAccountName(nil)
	if got.key != nil || got.withheld != core.AccountNameWithheldNoNameRecorded {
		t.Errorf("sidAccountName(nil) = %+v, want no_name_recorded without a key", got)
	}
}

// 作り直しで 2 つになった SID と、名前だけを記録したレコードの名前のノードは、同じ鍵を持つ。
// 時刻を持たないレコードだけが名前と共に記録した SID も鍵を持つ。途中で名前を変えた SID は
// 鍵を持たず、理由を持つ。端末が違う Administrator は別の鍵を持つ。
func TestAccountNameKeysTheRecreatedSidsAndWithholdsTheRenamedSid(t *testing.T) {
	const timelessSid = "S-1-5-21-4000-5000-6000-1204"
	const renamedSid = "S-1-5-21-4000-5000-6000-1205"
	document := "<Events>\n" +
		sidRecordXML("81", "2001-02-03T04:00:00Z", aliasOldSid, aliasHost, aliasUser) +
		sidRecordXML("82", "2001-02-03T06:00:00Z", aliasNewSid, "host-q", "USER-Q") +
		remoteDesktopXML(rdpLsm, "21", "83", "2001-02-03T05:00:00Z",
			`<UserData><EventXML><User>`+aliasHost+`\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`) +
		// 時差を持たない時刻は、絶対時刻として読めない。
		sidRecordXML("84", "2001-02-03T04:00:00", timelessSid, aliasHost, aliasUser) +
		sidRecordXML("85", "2001-02-03T04:00:00Z", renamedSid, aliasHost, "user-r") +
		sidRecordXML("86", "2001-02-03T05:00:00Z", renamedSid, aliasHost, "user-s") +
		sidRecordXML("87", "2001-02-03T04:00:00Z", "S-1-5-21-1-2-3-500", "PC01", "Administrator") +
		sidRecordXML("88", "2001-02-03T04:00:00Z", "S-1-5-21-4-5-6-500", "PC02", "Administrator") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	if graph.records[recordOfEvent(t, graph, "84")].hasInstant {
		t.Fatal("the record 84 carries an instant, want a record without an instant")
	}
	// 3 つの SID と、大文字と小文字の違う 2 つの名前のノード (HOST-Q\user-q と host-q\USER-Q)。
	want := core.AccountNameKey{Name: `host-q\user-q`, NodeCount: 5}
	named := 0
	for index, node := range graph.nodes {
		if key, _ := graph.accountNameFields(index); node.key.Form == core.NodeKeyFormAccountDomainName &&
			key != nil && *key == want {
			named++
		}
	}
	if named != 2 {
		t.Errorf("%d name nodes carry the key, want 2", named)
	}
	for _, sid := range []string{aliasOldSid, aliasNewSid, timelessSid} {
		key, withheld := accountNameOfSid(t, graph, sid)
		if key == nil || *key != want {
			t.Errorf("the SID %s carries the key %+v (withheld %q), want %+v", sid, key, withheld, want)
		}
	}
	if key, _ := accountNameOfNamedRecord(t, graph, "83"); key == nil || *key != want {
		t.Errorf("the name node carries the key %+v, want %+v", key, want)
	}
	if key, withheld := accountNameOfSid(t, graph, renamedSid); key != nil ||
		withheld != core.AccountNameWithheldMultipleNames {
		t.Errorf("the renamed SID carries the key %+v and the reason %q, want the reason multiple_names", key, withheld)
	}
	first, _ := accountNameOfSid(t, graph, "S-1-5-21-1-2-3-500")
	second, _ := accountNameOfSid(t, graph, "S-1-5-21-4-5-6-500")
	if first == nil || second == nil || *first == *second {
		t.Errorf("the Administrators of 2 terminals carry the keys %+v and %+v, want 2 different keys", first, second)
	}
}

// 収集の端末の改名の前の名前をドメインに書いた名前のノードは、今の名前と共に記録した SID の
// ノードと同じ鍵を持つ。端末の名前でないドメインの同じログイン名は別の鍵を持つ。
func TestAccountNameKeysTheOldHostnameWithTheCurrentHostname(t *testing.T) {
	events := eventSource(t, "Security.xml", "triage-d",
		renameXML("host-new.example.test", "2001-02-03T05:00:00Z", "700", "HOST-OLD", "HOST-NEW"),
		securityEventXML("701", "4720", "host-new.example.test", "2001-02-03T06:00:00Z",
			"TargetSid", aliasSid, "TargetUserName", aliasUser, "TargetDomainName", "HOST-NEW"),
		remoteDesktopXML(rdpLsm, "21", "702", "2001-02-03T04:00:00Z",
			`<UserData><EventXML><User>host-old\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`),
		remoteDesktopXML(rdpLsm, "21", "703", "2001-02-03T04:00:00Z",
			`<UserData><EventXML><User>unrelated\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`))
	registry := collectionSource{
		scanned: scanIndexSource(t, &terminalNamingParser{t: t}, "SYSTEM", "terminal_naming_test",
			"HOST-NEW|2001-02-03T07:00:00Z\n"),
		collection: "triage-d",
	}
	graph := NewGraph(collectionImportResult(t, events, registry), AllMatchConditions())
	sidKey, withheld := accountNameOfSid(t, graph, aliasSid)
	if sidKey == nil {
		t.Fatalf("the SID carries no key, the reason %q", withheld)
	}
	if oldKey, _ := accountNameOfNamedRecord(t, graph, "702"); oldKey == nil || *oldKey != *sidKey {
		t.Errorf("the old hostname account carries the key %+v, want the key of the SID %+v", oldKey, sidKey)
	}
	if unrelated, _ := accountNameOfNamedRecord(t, graph, "703"); unrelated == nil || *unrelated == *sidKey {
		t.Errorf("the unrelated domain account carries the key %+v, want a key other than %+v", unrelated, sidKey)
	}
}

// 2 つの案件で同じドメインとログイン名を記録した SID は、案件ごとの別の鍵を持つ。2 つの案件の
// レコードが記録した名前のノードは鍵を持たず、理由を持つ。
func TestAccountNameKeysDoNotCrossCases(t *testing.T) {
	document := func(recordID, sid string) string {
		return "<Events>\n" + sidRecordXML(recordID, "2001-02-03T04:00:00Z", sid, aliasHost, aliasUser) +
			remoteDesktopXML(rdpLsm, "21", recordID+"1", "2001-02-03T05:00:00Z",
				`<UserData><EventXML><User>`+aliasHost+`\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`) +
			"</Events>\n"
	}
	graph := NewGraph(caseImport(t,
		caseSource{name: "left.xml", format: string(WindowsEventXMLFormatKey), content: document("91", aliasOldSid), caseId: caseOf("case-left")},
		caseSource{name: "right.xml", format: string(WindowsEventXMLFormatKey), content: document("92", aliasNewSid), caseId: caseOf("case-right")},
	), AllMatchConditions())
	left, _ := accountNameOfSid(t, graph, aliasOldSid)
	right, _ := accountNameOfSid(t, graph, aliasNewSid)
	if left == nil || left.CaseId != "case-left" || right == nil || right.CaseId != "case-right" {
		t.Fatalf("the SIDs carry the keys %+v and %+v, want the keys of each case", left, right)
	}
	if left.NodeCount != 1 || right.NodeCount != 1 {
		t.Errorf("the SID keys count %d and %d nodes, want 1 each", left.NodeCount, right.NodeCount)
	}
	if key, withheld := accountNameOfNamedRecord(t, graph, "911"); key != nil ||
		withheld != core.AccountNameWithheldMultipleCases {
		t.Errorf("the name node of 2 cases carries the key %+v and the reason %q, want the reason multiple_cases", key, withheld)
	}
}

// ドメインの短い名前と FQDN。
const (
	shortDomain = "CORP"
	fqdnDomain  = "CORP.LOCAL"
)

func namedLogonXML(recordID, domain, name string) string {
	return remoteDesktopXML(rdpLsm, "21", recordID, "2001-02-03T05:00:00Z",
		`<UserData><EventXML><User>`+domain+`\`+name+`</User><Address>192.0.2.30</Address></EventXML></UserData>`)
}

// kerberosServiceTicketXML は、SID の欄を持たず、名前を UPN の形で記録したチケットの要求である。
func kerberosServiceTicketXML(recordID, domain, name string) string {
	return securityEventXML(recordID, "4769", "dc.example.test", "2001-02-03T05:00:00Z",
		"TargetUserName", name, "TargetDomainName", domain, "ServiceName", "krbtgt")
}

// ドメインの短い名前と FQDN のそれぞれと共に記録した SID は、名前が 1 つと数え、両方の名前の
// ノードと同じ鍵を持つ。UPN の形で記録した名前のノードも同じ鍵を持つ。
func TestAccountNameKeysTheShortDomainWithTheFqdnAndTheUpn(t *testing.T) {
	document := "<Events>\n" +
		sidRecordXML("121", "2001-02-03T04:00:00Z", aliasSid, shortDomain, "user-k") +
		sidRecordXML("122", "2001-02-03T04:00:01Z", aliasSid, fqdnDomain, "user-k") +
		namedLogonXML("123", shortDomain, "user-k") +
		kerberosServiceTicketXML("125", fqdnDomain, "user-k@"+fqdnDomain) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	// SID と、CORP\user-k と、UPN を記録した CORP.LOCAL\user-k。
	want := core.AccountNameKey{Name: `corp\user-k`, NodeCount: 3}
	if key, withheld := accountNameOfSid(t, graph, aliasSid); key == nil || *key != want {
		t.Errorf("the SID carries the key %+v (withheld %q), want %+v", key, withheld, want)
	}
	for _, recordID := range []string{"123", "125"} {
		if key, _ := accountNameOfNamedRecord(t, graph, recordID); key == nil || *key != want {
			t.Errorf("the name node of the record %s carries the key %+v, want %+v", recordID, key, want)
		}
	}
	// 4769 の UPN は、取り込みが `@` の前を比べる値に置く。
	upn := graph.nodes[nameNodeNamedBy(t, graph, recordOfEvent(t, graph, "125"))]
	if upn.key.Values[1].Value != "user-k" {
		t.Errorf("the UPN name node records the name %q, want user-k", upn.key.Values[1].Value)
	}
}

// UPN の形の名前は、`@` の後がドメインと同じとみなせるときだけ `@` の前をログイン名に読む。
func TestMergedAccountNameReadsTheUpnOfTheSameDomain(t *testing.T) {
	for _, example := range []struct {
		domains []string
		name    string
		want    []string
		login   string
	}{
		{[]string{"CORP"}, "User-K@CORP.LOCAL", []string{"corp"}, "user-k"},
		{[]string{"corp.local"}, "user-k@CORP", []string{"corp"}, "user-k"},
		{[]string{"CORP"}, "user-k@OTHER.LOCAL", []string{"corp"}, "user-k@other.local"},
		{[]string{"HOST-NEW", "host-old"}, "user-k", []string{"host-new", "host-old"}, "user-k"},
	} {
		domains, login := mergedAccountName(example.domains, example.name)
		if !slices.Equal(domains, example.want) || login != example.login {
			t.Errorf("mergedAccountName(%v, %q) = %v, %q, want %v, %q",
				example.domains, example.name, domains, login, example.want, example.login)
		}
	}
}

// 最初のラベルが短い名前と違う FQDN (`CORP` と `OTHER.LOCAL`) は別のドメインであり、両方と共に
// 記録した SID は名前が 2 つ以上である。
func TestAccountNameKeepsTheDomainsOfAnotherFirstLabelApart(t *testing.T) {
	document := "<Events>\n" +
		sidRecordXML("131", "2001-02-03T04:00:00Z", aliasSid, shortDomain, "user-m") +
		sidRecordXML("132", "2001-02-03T04:00:01Z", aliasSid, "OTHER.LOCAL", "user-m") +
		namedLogonXML("133", shortDomain, "user-m") +
		namedLogonXML("134", "OTHER.LOCAL", "user-m") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	if key, withheld := accountNameOfSid(t, graph, aliasSid); key != nil ||
		withheld != core.AccountNameWithheldMultipleNames {
		t.Errorf("the SID carries the key %+v and the reason %q, want the reason multiple_names", key, withheld)
	}
	short, _ := accountNameOfNamedRecord(t, graph, "133")
	other, _ := accountNameOfNamedRecord(t, graph, "134")
	if short == nil || other == nil || *short == *other {
		t.Errorf("the name nodes carry the keys %+v and %+v, want 2 different keys", short, other)
	}
}

// 短い名前と FQDN の SID でも、別の案件の SID は別の鍵を持つ。
func TestAccountNameKeepsTheShortDomainAndTheFqdnOfAnotherCaseApart(t *testing.T) {
	document := func(recordID, sid, domain string) string {
		return "<Events>\n" + sidRecordXML(recordID, "2001-02-03T04:00:00Z", sid, domain, "user-k") + "</Events>\n"
	}
	graph := NewGraph(caseImport(t,
		caseSource{name: "left.xml", format: string(WindowsEventXMLFormatKey), content: document("141", aliasOldSid, shortDomain), caseId: caseOf("case-left")},
		caseSource{name: "right.xml", format: string(WindowsEventXMLFormatKey), content: document("142", aliasNewSid, fqdnDomain), caseId: caseOf("case-right")},
	), AllMatchConditions())
	left, _ := accountNameOfSid(t, graph, aliasOldSid)
	right, _ := accountNameOfSid(t, graph, aliasNewSid)
	if left == nil || right == nil || left.Name != right.Name || *left == *right {
		t.Errorf("the SIDs carry the keys %+v and %+v, want the same name in 2 different cases", left, right)
	}
}
