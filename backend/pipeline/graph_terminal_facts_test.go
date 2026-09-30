// in-package test: 端末のレコードの分類と、接続元 IP ごとの遠隔のログオンの集計を確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// terminalFactsDocument は端末 1 台の Security のイベントである。192.0.2.50 から 3 回失敗し
// 1 回成功し、192.0.2.60 から 1 回失敗する。種別 2 の 4624 は遠隔のログオンでない。4720 は
// アカウントの作成である。
func terminalFactsDocument() string {
	failure := func(recordID, at, ip, user string) string {
		return securityEventXML(recordID, "4625", logonHostA, at,
			"TargetUserName", user, "TargetDomainName", "EXAMPLE", "LogonType", "3", "IpAddress", ip)
	}
	return "<Events>\n" +
		failure("71", "2001-02-03T04:05:03Z", "192.0.2.50", "user71") +
		failure("72", "2001-02-03T04:05:01Z", "192.0.2.50", "user72") +
		failure("73", "2001-02-03T04:05:02Z", "192.0.2.50", "user71") +
		failure("74", "2001-02-03T04:05:04Z", "192.0.2.60", "user74") +
		securityEventXML("75", "4624", logonHostA, "2001-02-03T04:05:05Z",
			"TargetUserName", "user71", "TargetDomainName", "EXAMPLE", "LogonType", "10",
			"IpAddress", "192.0.2.50", "TargetLogonId", "0x75") +
		securityEventXML("76", "4624", logonHostA, "2001-02-03T04:05:06Z",
			"TargetUserName", "user76", "TargetDomainName", "EXAMPLE", "LogonType", "2",
			"IpAddress", "127.0.0.1", "TargetLogonId", "0x76") +
		securityEventXML("77", "4720", logonHostA, "2001-02-03T04:05:07Z",
			"TargetUserName", "user77", "TargetDomainName", "EXAMPLE", "TargetSid", "S-1-5-21-1000-2000-3000-1377") +
		"</Events>\n"
}

// onlyTerminalIdOf は、端末の一覧がただ 1 台の端末を持つことを確かめ、その識別子を返す。
func onlyTerminalIdOf(t *testing.T, graph Graph) string {
	t.Helper()
	summaries := graph.TerminalSummaries()
	if len(summaries) != 1 {
		t.Fatalf("the terminals are %+v, want one", summaries)
	}
	return summaries[0].Node.Id
}

// 接続元 IP ごとに失敗と成功の件数、試したアカウントと成功したアカウント、最初と最後の時刻を数える。
func TestTerminalDetailCountsRemoteLogonsPerSourceIp(t *testing.T) {
	graph := NewGraph(windowsEventImportResult(t, terminalFactsDocument()), AllMatchConditions())
	id := onlyTerminalIdOf(t, graph)
	detail, found := graph.TerminalDetail(id)
	if !found {
		t.Fatal("the terminal has no detail")
	}
	if err := detail.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(detail.RemoteLogons) != 2 {
		t.Fatalf("the remote logons are %+v", detail.RemoteLogons)
	}
	first := detail.RemoteLogons[0]
	if first.SourceIp != "192.0.2.50" || first.FailureCount != 3 || first.SuccessCount != 1 ||
		!slices.Equal(first.AttemptedAccounts, []string{`EXAMPLE\user71`, `EXAMPLE\user72`}) ||
		!slices.Equal(first.LoggedOnAccounts, []string{`EXAMPLE\user71`}) {
		t.Errorf("the logons from 192.0.2.50 are %+v", first)
	}
	if first.First == nil || *first.First.Normalized != "2001-02-03T04:05:01Z" ||
		first.Last == nil || *first.Last.Normalized != "2001-02-03T04:05:05Z" {
		t.Errorf("the period of 192.0.2.50 is %v to %v", first.First, first.Last)
	}
	if second := detail.RemoteLogons[1]; second.SourceIp != "192.0.2.60" || second.FailureCount != 1 || second.SuccessCount != 0 {
		t.Errorf("the logons from 192.0.2.60 are %+v", second)
	}

	events, _ := graph.TerminalEvents(id, TerminalEventQuery{
		Category: core.TerminalCategoryRemoteLogon, SourceIp: "192.0.2.50"})
	if len(events) != 4 {
		t.Fatalf("the events are %d, want 4", len(events))
	}
	for _, event := range events {
		if err := event.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if got := *events[1].EventTime.Normalized; got != "2001-02-03T04:05:02Z" {
		t.Errorf("the second event in time order is at %s", got)
	}
	if !slices.ContainsFunc(events[1].NamedNodes, func(node core.GraphNode) bool { return node.Kind == core.NodeKindAccount }) {
		t.Errorf("the event names no account: %+v", events[1].NamedNodes)
	}
	outcomes := make([]core.RemoteLogonOutcome, 0, len(events))
	for _, event := range events {
		outcomes = append(outcomes, event.LogonOutcome)
	}
	want := []core.RemoteLogonOutcome{
		core.RemoteLogonOutcomeFailure, core.RemoteLogonOutcomeFailure, core.RemoteLogonOutcomeFailure,
		core.RemoteLogonOutcomeSuccess,
	}
	if !slices.Equal(outcomes, want) {
		t.Errorf("the outcomes are %v, want %v", outcomes, want)
	}
}

// 元のファイル名との違いは、空白だけの値を値の無い欄として扱い、大文字と小文字を区別せずに比べる。
func TestOriginalFileNameDiffersIgnoresBlankAndCase(t *testing.T) {
	fieldsOf := func(name, original string) []core.RecordField {
		field := func(fieldName string, semantic core.SemanticKey, text string) core.RecordField {
			value, err := core.NewRawValue(core.ValueStatePresent, text)
			if err != nil {
				t.Fatal(err)
			}
			return textField(fieldName, semantic, value)
		}
		return []core.RecordField{
			field("Name", core.SemanticKeyFileName, name),
			field("Original", core.SemanticKeyFileOriginalFileName, original),
		}
	}
	for _, tc := range []struct {
		name, original string
		want           bool
	}{
		{"tool.exe", "other.exe", true},
		{"tool.exe", "TOOL.EXE", false},
		{"tool.exe", "   ", false},
		{"tool.exe", "", false},
		{"tool.exe", "tool", false},
		{"tool.exe", "TOOL.dll", false},
		{"tool.exe", "abc_def_str", true},
		{"tool.exe", "renamed_tool_x64", true},
		{"tool.exe", "other", true},
		{"tool.exe", "other_name.exe", true},
	} {
		if got := originalFileNameDiffers(fieldsOf(tc.name, tc.original)); got != tc.want {
			t.Errorf("%q and %q: differs = %v, want %v", tc.name, tc.original, got, tc.want)
		}
	}
}

// 記録の無い分類は件数 0 と、分類に要る収集元の種類の有無を返す。Security を持つ端末の
// account_management は 1 件、powershell は件数 0 で収集元が無い。
func TestTerminalDetailReportsEmptyCategoriesWithSourcePresence(t *testing.T) {
	graph := NewGraph(windowsEventImportResult(t, terminalFactsDocument()), AllMatchConditions())
	detail, _ := graph.TerminalDetail(onlyTerminalIdOf(t, graph))
	counts := make(map[core.TerminalCategory]core.TerminalCategoryCount)
	for _, count := range detail.Categories {
		counts[count.Category] = count
	}
	if account := counts[core.TerminalCategoryAccountManagement]; account.RecordCount != 1 ||
		!slices.Equal(account.PresentSources, []string{"Security"}) {
		t.Errorf("account_management is %+v", account)
	}
	if powershell := counts[core.TerminalCategoryPowerShell]; powershell.RecordCount != 0 ||
		len(powershell.PresentSources) != 0 || len(powershell.RequiredSources) == 0 {
		t.Errorf("powershell is %+v", powershell)
	}
	if remote := counts[core.TerminalCategoryRemoteLogon]; remote.RecordCount != 5 {
		t.Errorf("remote_logon counts %d records, want 5 without the interactive logon", remote.RecordCount)
	}
	if _, found := graph.TerminalDetail("absent"); found {
		t.Error("an absent node has a detail")
	}
}
