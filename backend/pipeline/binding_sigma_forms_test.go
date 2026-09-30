// in-package test: 入力形式ごとのレコードへルールを当て、形式が持たない項目で候補を作らないことを確かめる。
package pipeline

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sigmaImportResult は収集元 1 件を走査した取り込み結果を返す。
func sigmaImportResult(t *testing.T, format core.FormatKey, fileName, document string) ImportResult {
	t.Helper()
	scanned := scanIndexSource(t, NewTestParser(format, nil), fileName, format, document)
	statuses := []core.ImportStatus{settleStatus(t, scanned, "sigma")}
	result, err := newImportResult([]scannedSource{scanned}, statuses, "run", byteRangeRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func evaluateSyntheticRules(t *testing.T, result ImportResult, files map[string]string) SigmaEvaluation {
	t.Helper()
	rules, err := LoadSigmaRules(SigmaRuleSpec{Directory: writeSigmaRules(t, files)})
	if err != nil {
		t.Fatal(err)
	}
	return EvaluateSigmaRules(result, &rules)
}

func TestEvaluateSigmaRulesReadsEVTX(t *testing.T) {
	content, err := os.ReadFile("../internal/testdata/winevent/process-creation.evtx")
	if err != nil {
		t.Fatal(err)
	}
	result := sigmaImportResult(t, WindowsEVTXFormatKey, "events.evtx", string(content))
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"match.yml": matchingRule})
	if len(evaluation.Matches) != 1 {
		t.Fatalf("Matches = %+v, want the child record of the EVTX", evaluation.Matches)
	}
	raw := ""
	for _, record := range result.publications[0].records {
		if record.Locator == evaluation.Matches[0].Record {
			raw = record.RawText
		}
	}
	if !strings.Contains(raw, `b-child.exe`) {
		t.Fatalf("the candidate points at %q", raw)
	}
}

// parentUserFilterRule は Sysmon の 1 を前提にした `not filter` を持つ。Security の 4688 は
// ParentUser を記録しない。
const parentUserFilterRule = `title: Synthetic filtered child
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    Image|endswith: '\b-child.exe'
  filter:
    ParentUser|contains: 'SYNTHETIC'
  condition: selection and not filter
`

func TestEvaluateSigmaRulesDoesNotTreatFieldsAbsentFromSecurityAuditingAsEmpty(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument)
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"filtered.yml": parentUserFilterRule})
	if len(evaluation.Matches) != 0 {
		t.Fatalf("Matches = %+v, want none on 4688, which does not record ParentUser", evaluation.Matches)
	}
	// ParentUser を参照しないルールは 4688 に当てる。
	evaluation = evaluateSyntheticRules(t, result, map[string]string{"match.yml": matchingRule})
	if len(evaluation.Matches) != 1 {
		t.Fatalf("Matches = %+v, want the child record", evaluation.Matches)
	}
}

// viewerCSVHeader はイベントビューアーの CSV の BOM と見出しの行である。
var viewerCSVHeader = string([]rune{0xfeff}) + "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"

// accountCreationCSV はアカウントの作成の 4720 の論理レコード 1 件である。CSV は「属性」の
// 「SAM アカウント名」を `<Data>` の Name へ写さず、見出しと項目名の組のままである。
var accountCreationCSV = viewerCSVHeader +
	"情報,2001/02/03 04:05:06,Microsoft-Windows-Security-Auditing,4720,User Account Management," +
	"\"ユーザー アカウントが作成されました。\n\n" +
	"サブジェクト:\n\tアカウント名:\t\tadmin01\n\tログオン ID:\t\t0x3e7\n\n" +
	"新しいアカウント:\n\tアカウント名:\t\tHOST09$\n\n" +
	"属性:\n\tSAM アカウント名:\t\tHOST09$\n\"\n"

// securityRule は service: security のルールを返す。
func securityRule(title, detection string) string {
	return "title: " + title + "\nlogsource:\n  product: windows\n  service: security\ndetection:\n" + detection
}

// accountCreationFilterRule は、CSV が名前へ写さない SamAccountName を `not filter` で参照する。
var accountCreationFilterRule = securityRule("Synthetic account without machine accounts",
	"  selection:\n    EventID: 4720\n  filter:\n    SamAccountName|endswith: '$'\n  condition: selection and not filter\n")

var accountCreationRule = securityRule("Synthetic account creation",
	"  selection:\n    EventID: 4720\n  condition: selection\n")

func TestEvaluateSigmaRulesAppliesOnlySystemFieldRulesToCSVRecordsWithoutDataNames(t *testing.T) {
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "security.csv", accountCreationCSV)
	evaluation := evaluateSyntheticRules(t, result,
		map[string]string{"a_filter.yml": accountCreationFilterRule, "b_created.yml": accountCreationRule})
	if len(evaluation.Matches) != 1 || evaluation.Matches[0].RulePath != "b_created.yml" {
		t.Fatalf("Matches = %+v, want b_created.yml alone", evaluation.Matches)
	}
	if evaluation.EvaluatedRecordCount != 1 || evaluation.SkippedPairCount != 1 || evaluation.SkippedPairRecordCount != 1 {
		t.Fatalf("evaluated %d records, skipped %d pairs in %d records; want 1, 1, 1",
			evaluation.EvaluatedRecordCount, evaluation.SkippedPairCount, evaluation.SkippedPairRecordCount)
	}
}

// logonCSV はログオンの 4624 の論理レコードを返す。CSV はログオンの種別、ログオンした
// アカウント、接続元の項目名を `<Data>` の Name へ写す。
func logonCSV(account, address string) string {
	return "情報,2001/02/03 04:05:06,Microsoft-Windows-Security-Auditing,4624,Logon,\"アカウントが正常にログオンしました。\n\n" +
		"ログオン タイプ:\t\t3\n\n" +
		"新しいログオン:\n\tアカウント名:\t\t" + account + "\n\tアカウント ドメイン:\t\tCORP-TEST\n\n" +
		"ネットワーク情報:\n\tワークステーション名:\tWS01\n\tソース ネットワーク アドレス:\t" + address +
		"\n\tソース ポート:\t\t50000\n\"\n"
}

// networkLogonRule は、CSV が名前へ写した LogonType と IpAddress と TargetUserName を参照する。
var networkLogonRule = securityRule("Synthetic network logon from the documentation range",
	"  selection:\n    EventID: 4624\n    LogonType: 3\n    IpAddress|cidr: '192.0.2.0/24'\n"+
		"  filter:\n    TargetUserName|endswith: '$'\n  condition: selection and not filter\n")

func TestEvaluateSigmaRulesAppliesDataFieldRulesToCSVLogonsWithMappedNames(t *testing.T) {
	document := viewerCSVHeader + logonCSV("user01", "192.0.2.10") + logonCSV("HOST09$", "192.0.2.11") +
		logonCSV("user02", "198.51.100.7")
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "security.csv", document)
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"network.yml": networkLogonRule})
	if len(evaluation.Matches) != 1 || evaluation.SkippedPairCount != 0 {
		t.Fatalf("Matches = %+v, skipped %d; want the logon of user01 alone", evaluation.Matches, evaluation.SkippedPairCount)
	}
	for _, record := range result.publications[0].records {
		if record.Locator == evaluation.Matches[0].Record && !strings.Contains(record.RawText, "user01") {
			t.Fatalf("the candidate points at %q", record.RawText)
		}
	}
}

// どの service にも一致しないチャネルのレコードは、当てたレコードに数えず、チャネルごとに数える。
func TestEvaluateSigmaRulesCountsRecordsNoRuleCanApplyTo(t *testing.T) {
	unmapped := func(recordID string) string {
		return `<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
			`<TimeCreated SystemTime="2001-02-03T04:05:06Z"/><EventRecordID>` + recordID + `</EventRecordID>` +
			`<Channel>Example/Operational</Channel><Computer>host-a.example.test</Computer></System></Event>` + "\n"
	}
	document := "<Events>\n" + unmapped("501") + unmapped("502") +
		processCreationXML("host-a.example.test", "2001-02-03T04:05:07Z", "503", "0x11", "0x10",
			`C:\Example\b-child.exe`, "b-child.exe") + "</Events>\n"
	evaluation := evaluateSyntheticRules(t, windowsEventImportResult(t, document), map[string]string{"match.yml": matchingRule})
	groups, err := json.Marshal(evaluation.UnevaluatedRecordGroups)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"channel":"Example/Operational","recordCount":2}]`
	if evaluation.EvaluatedRecordCount != 1 || string(groups) != want {
		t.Fatalf("evaluated %d, groups %s; want 1 and %s", evaluation.EvaluatedRecordCount, groups, want)
	}
}
