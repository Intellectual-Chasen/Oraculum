// in-package test: Channel や Provider や項目を欠くレコードで、候補と件数が崩れないことを確かめる。
package pipeline

import (
	"encoding/json"
	"testing"
)

// emptyChannelDocument は、空の Channel を 2 通りに書いたレコードと、Channel を持たずに空の
// Provider を持つレコードと、どちらも持たないレコードを 1 件ずつ持つ。
const emptyChannelDocument = "<Events>\n" +
	`<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:05:06Z"/><EventRecordID>601</EventRecordID>` +
	`<Channel></Channel><Computer>host-a.example.test</Computer></System></Event>` + "\n" +
	`<Event><System><Provider Name="Example-Provider"/><EventID>7</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:05:07Z"/><EventRecordID>602</EventRecordID>` +
	`<Channel/><Computer>host-a.example.test</Computer></System></Event>` + "\n" +
	`<Event><System><Provider Name=""/><EventID>7</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:05:08Z"/><EventRecordID>603</EventRecordID>` +
	`<Computer>host-a.example.test</Computer></System></Event>` + "\n" +
	`<Event><System><EventID>7</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:05:09Z"/><EventRecordID>604</EventRecordID>` +
	`<Computer>host-a.example.test</Computer></System></Event>` + "\n" +
	"</Events>\n"

// emptyChannelGroupsJSON は、emptyChannelDocument のレコードを分けた組の応答の文字列である。
// frontend の decoder の test は同じ文字列を読む。
const emptyChannelGroupsJSON = `[{"channel":"","recordCount":2},{"provider":"","recordCount":1},` +
	`{"channelAndProviderAbsent":true,"recordCount":1}]`

func TestEvaluateSigmaRulesKeepsEmptyChannelsAndProvidersApart(t *testing.T) {
	evaluation := evaluateSyntheticRules(t, windowsEventImportResult(t, emptyChannelDocument),
		map[string]string{"match.yml": matchingRule})
	got, err := json.Marshal(evaluation.UnevaluatedRecordGroups)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != emptyChannelGroupsJSON || evaluation.EvaluatedRecordCount != 0 {
		t.Fatalf("groups = %s, evaluated %d; want %s and 0", got, evaluation.EvaluatedRecordCount, emptyChannelGroupsJSON)
	}
}

// 意味付けに至らなかったレコードは、当てた件数にも logsource の外の件数にも入れず、別に数える。
func TestEvaluateSigmaRulesCountsRecordsWithoutSemantics(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument)
	result.publications[0].records[0].Semantics = nil
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"match.yml": matchingRule})
	total := int64(len(result.publications[0].records))
	if evaluation.RecordsWithoutSemantics != 1 || evaluation.EvaluatedRecordCount != total-1 ||
		len(evaluation.UnevaluatedRecordGroups) != 0 {
		t.Fatalf("without semantics %d, evaluated %d of %d, groups %+v",
			evaluation.RecordsWithoutSemantics, evaluation.EvaluatedRecordCount, total, evaluation.UnevaluatedRecordGroups)
	}
}

// sysmonProcessCSV は、OriginalFileName を記録しないバージョンの Sysmon の 1 の論理レコードである。
// Sysmon の説明はイベントの項目をすべて書く。
var sysmonProcessCSV = viewerCSVHeader +
	"情報,2001/02/03 04:05:06,Microsoft-Windows-Sysmon,1,Process Create (rule: ProcessCreate),\"Process Create:\n" +
	"UtcTime: 2001-02-03 04:05:06.000\nImage: C:\\Example\\b-child.exe\nCommandLine: b-child.exe\"\n"

// imageOrOriginalNameRule は、Image か OriginalFileName のどちらかで一致する。
var imageOrOriginalNameRule = `title: Synthetic child by image or original name
logsource:
  product: windows
  category: process_creation
detection:
  selection_image:
    Image|endswith: '\b-child.exe'
  selection_original:
    OriginalFileName: 'b-child.exe'
  condition: 1 of selection_*
`

// Sysmon の CSV は XML と同じく、説明に無い項目をイベントが持たない項目として扱う。
func TestEvaluateSigmaRulesTreatsSysmonCSVLikeXML(t *testing.T) {
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "sysmon.csv", sysmonProcessCSV)
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"image.yml": imageOrOriginalNameRule})
	if len(evaluation.Matches) != 1 || evaluation.SkippedPairCount != 0 {
		t.Fatalf("Matches = %+v, skipped %d; want the record by its image", evaluation.Matches, evaluation.SkippedPairCount)
	}
}

// spoofedParentCSV は、CommandLine の値の中の改行の後ろに ParentImage の行を書いた Sysmon の 1
// である。説明の偽の行が先に ParentImage の名前を取り、本物の行は ParentImage#2 になる。
var spoofedParentCSV = viewerCSVHeader +
	"情報,2001/02/03 04:05:06,Microsoft-Windows-Sysmon,1,Process Create (rule: ProcessCreate),\"Process Create:\n" +
	"Image: C:\\Example\\b-child.exe\nCommandLine: b-child.exe\nParentImage: C:\\Trusted\\shell.exe\n" +
	"ParentImage: C:\\Example\\dropper.exe\"\n"

// trustedParentRule は、許した親から起動した子を `not filter` で除く。
var trustedParentRule = `title: Synthetic child from an untrusted parent
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    Image|endswith: '\b-child.exe'
  filter:
    ParentImage|startswith: 'C:\Trusted\'
  condition: selection and not filter
`

// 名前が重なった項目は、どちらがイベントの項目かを決められない。その名前を参照するルールは
// 当てず、当てなかった組に数える。
func TestEvaluateSigmaRulesSkipsAmbiguousCSVFieldNames(t *testing.T) {
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "sysmon.csv", spoofedParentCSV)
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"parent.yml": trustedParentRule})
	if len(evaluation.Matches) != 0 || evaluation.SkippedPairCount != 1 {
		t.Fatalf("Matches = %+v, skipped %d; want no candidate and 1 skipped pair",
			evaluation.Matches, evaluation.SkippedPairCount)
	}
}

// sysmonUnrenderedCSV は、説明を組めなかった印を持たずに `名前: 値` の行を 1 つも持たない
// Sysmon の 1 である。
var sysmonUnrenderedCSV = viewerCSVHeader +
	"情報,2001/02/03 04:05:06,Microsoft-Windows-Sysmon,1,Process Create (rule: ProcessCreate),\"synthetic text without items\"\n"

func TestEvaluateSigmaRulesAppliesOnlySystemFieldRulesToSysmonCSVWithoutItems(t *testing.T) {
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "sysmon.csv", sysmonUnrenderedCSV)
	systemOnly := `title: Synthetic process creation
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    EventID: 1
  condition: selection
`
	evaluation := evaluateSyntheticRules(t, result,
		map[string]string{"a_parent.yml": trustedParentRule, "b_system.yml": systemOnly})
	if len(evaluation.Matches) != 1 || evaluation.Matches[0].RulePath != "b_system.yml" || evaluation.SkippedPairCount != 1 {
		t.Fatalf("Matches = %+v, skipped %d; want b_system.yml alone and 1 skipped pair",
			evaluation.Matches, evaluation.SkippedPairCount)
	}
}

// remoteLogonCSV は、接続元の行を表に無い見出しの文字列で書いた 4624 の論理レコードである。
// 接続元のアドレスは IpAddress へ写らない。
var remoteLogonCSV = viewerCSVHeader +
	"情報,2001/02/03 04:05:06,Microsoft-Windows-Security-Auditing,4624,Logon,\"アカウントが正常にログオンしました。\n\n" +
	"ログオン タイプ:\t\t10\n\n" +
	"ネットワーク情報:\n\t送信元ネットワーク アドレス:\t127.0.0.1\n\"\n"

var remoteLogonRule = securityRule("Synthetic remote logon from elsewhere",
	"  selection:\n    EventID: 4624\n    LogonType: 10\n  filter:\n    IpAddress: '127.0.0.1'\n"+
		"  condition: selection and not filter\n")

func TestEvaluateSigmaRulesSkipsCSVFieldsTheRecordDoesNotCarry(t *testing.T) {
	result := sigmaImportResult(t, WindowsEventCSVFormatKey, "security.csv", remoteLogonCSV)
	evaluation := evaluateSyntheticRules(t, result, map[string]string{"remote.yml": remoteLogonRule})
	if len(evaluation.Matches) != 0 || evaluation.SkippedPairCount != 1 {
		t.Fatalf("Matches = %+v, skipped %d; want no candidate and 1 skipped pair",
			evaluation.Matches, evaluation.SkippedPairCount)
	}
}
