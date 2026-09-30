package winevent_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ログオンの成功の説明。見出しと項目名は
// Windows が 4624 の説明に書く文字列である。
const logonSuccessDescription = "アカウントが正常にログオンしました。\n\n" +
	"サブジェクト:\n" +
	"\tセキュリティ ID:\t\tSYSTEM\n" +
	"\tアカウント名:\t\tHOST01$\n" +
	"\tアカウント ドメイン:\t\tCORP-TEST\n" +
	"\tログオン ID:\t\t0x3e7\n\n" +
	"ログオン タイプ:\t\t\t10\n\n" +
	"新しいログオン:\n" +
	"\tセキュリティ ID:\t\tCORP-TEST\\user03\n" +
	"\tアカウント名:\t\tuser03\n" +
	"\tアカウント ドメイン:\t\tCORP-TEST\n" +
	"\tログオン ID:\t\t0xA1B2C\n\n" +
	"ネットワーク情報:\n" +
	"\tワークステーション名:\tCLIENT03\n" +
	"\tソース ネットワーク アドレス:\t192.0.2.30\n" +
	"\tソース ポート:\t\t50001\n\n" +
	"このイベントは、合成した説明の文です。"

// ログオンの失敗の説明。見出しと項目名は Windows が 4625 の説明に書く文字列である。
const logonFailureDescription = "アカウントがログオンに失敗しました。\n\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tHOST01$\n" +
	"\tログオン ID:\t\t0x3e7\n\n" +
	"ログオン タイプ:\t\t\t3\n\n" +
	"ログオンを失敗したアカウント:\n" +
	"\tアカウント名:\t\tuser04\n" +
	"\tアカウント ドメイン:\t\tCORP-TEST\n\n" +
	"エラー情報:\n" +
	"\t状態:\t\t\t0xc0000234\n" +
	"\tサブ ステータス:\t\t0xc0000064\n\n" +
	"ネットワーク情報:\n" +
	"\tワークステーション名:\tCLIENT04\n" +
	"\tソース ネットワーク アドレス:\t192.0.2.40\n" +
	"\tソース ポート:\t\t50002\n"

// ログオフの説明。ログオフの説明は終わったセッションの見出しに「サブジェクト」を書く。
const logoffDescription = "アカウントがログオフしました。\n\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tuser03\n" +
	"\tアカウント ドメイン:\t\tCORP-TEST\n" +
	"\tログオン ID:\t\t0xA1B2C\n\n" +
	"ログオン タイプ:\t\t\t10\n"

// 利用者が始めたログオフの説明。終えるセッションを「サブジェクト」の見出しに書く。
const userInitiatedLogoffDescription = "ユーザー開始のログオフ:\n\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tuser03\n" +
	"\tログオン ID:\t\t0xA1B2C\n"

// 特権の割り当ての説明。
const specialPrivilegesDescription = "新しいログオンに特権が割り当てられました。\n\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tuser03\n" +
	"\tログオン ID:\t\t0xa1b2c\n\n" +
	"特権:\t\tSeDebugPrivilege\n"

func observeViewer(t *testing.T, eventID, description string) winevent.Observation {
	t.Helper()
	events, failures := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-Security-Auditing", eventID, "", description))
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	observation, failure := winevent.Observe(events[0])
	if failure != nil {
		t.Fatalf("Observe() failure = %+v", failure)
	}
	return observation
}

// イベントビューアーの説明のログオンの項目が、XML の `<Data>` と同じ語彙の項目を持つ。
func TestObserveMapsTheViewerLogonItems(t *testing.T) {
	type mapped struct {
		semantic   core.SemanticKey
		comparable string
	}
	for name, tc := range map[string]struct {
		eventID     string
		description string
		want        map[string]mapped
	}{
		"ログオンの成功": {"4624", logonSuccessDescription, map[string]mapped{
			"EventData.LogonType":         {core.SemanticKeyEventLogonType, "10"},
			"EventData.TargetLogonId":     {core.SemanticKeyEventTargetLogonId, "0xa1b2c"},
			"EventData.SubjectLogonId":    {core.SemanticKeyEventSubjectLogonId, "0x3e7"},
			"EventData.SubjectUserName":   {core.SemanticKeySubjectAccountName, "HOST01$"},
			"EventData.SubjectDomainName": {core.SemanticKeySubjectAccountDomain, "CORP-TEST"},
			"EventData.TargetUserName":    {core.SemanticKeyTargetAccountName, "user03"},
			"EventData.TargetDomainName":  {core.SemanticKeyTargetAccountDomain, "CORP-TEST"},
			"EventData.IpAddress":         {core.SemanticKeyConnectionSourceAddress, "192.0.2.30"},
			"EventData.IpPort":            {core.SemanticKeyConnectionSourcePort, "50001"},
			"EventData.WorkstationName":   {core.SemanticKeyRemoteSessionClientHostname, "CLIENT03"},
		}},
		"ログオンの失敗": {"4625", logonFailureDescription, map[string]mapped{
			"EventData.LogonType":        {core.SemanticKeyEventLogonType, "3"},
			"EventData.SubjectLogonId":   {core.SemanticKeyEventSubjectLogonId, "0x3e7"},
			"EventData.TargetUserName":   {core.SemanticKeyTargetAccountName, "user04"},
			"EventData.TargetDomainName": {core.SemanticKeyTargetAccountDomain, "CORP-TEST"},
			"EventData.Status":           {core.SemanticKeyEventLogonFailureStatus, "0xc0000234"},
			"EventData.SubStatus":        {core.SemanticKeyEventLogonFailureSubStatus, "0xc0000064"},
			"EventData.IpAddress":        {core.SemanticKeyConnectionSourceAddress, "192.0.2.40"},
			"EventData.IpPort":           {core.SemanticKeyConnectionSourcePort, "50002"},
			"EventData.WorkstationName":  {core.SemanticKeyRemoteSessionClientHostname, "CLIENT04"},
		}},
		"ログオフ": {"4634", logoffDescription, map[string]mapped{
			"EventData.LogonType":        {core.SemanticKeyEventLogonType, "10"},
			"EventData.TargetUserName":   {core.SemanticKeyTargetAccountName, "user03"},
			"EventData.TargetDomainName": {core.SemanticKeyTargetAccountDomain, "CORP-TEST"},
			// 終えたセッションは XML の 4634 と同じ名前と語彙の項目で持つ。
			"EventData.TargetLogonId": {core.SemanticKeyEventLogoffLogonId, "0xa1b2c"},
		}},
		"利用者が始めたログオフ": {"4647", userInitiatedLogoffDescription, map[string]mapped{
			"EventData.TargetLogonId": {core.SemanticKeyEventLogoffLogonId, "0xa1b2c"},
		}},
		"特権の割り当て": {"4672", specialPrivilegesDescription, map[string]mapped{
			"EventData.SubjectLogonId":  {core.SemanticKeyEventSubjectLogonId, "0xa1b2c"},
			"EventData.SubjectUserName": {core.SemanticKeySubjectAccountName, "user03"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			observation := observeViewer(t, tc.eventID, tc.description)
			for field, want := range tc.want {
				got := fieldNamed(t, observation.Fields, field)
				if got.Semantic != want.semantic {
					t.Errorf("%s semantic = %q, want %q", field, got.Semantic, want.semantic)
				}
				if value, ok := got.Text.ComparableValue(); !ok || value != want.comparable {
					t.Errorf("%s comparable = %q (%t), want %q", field, value, ok, want.comparable)
				}
			}
		})
	}
}

// 4624 のログオンの種別は、Windows のバージョンにより「ログオン情報」の見出しの下に置かれる。
func TestObserveMapsTheLogonTypeUnderTheLogonInformationSection(t *testing.T) {
	description := "アカウントが正常にログオンしました。\n\n" +
		"ログオン情報:\n\tログオン タイプ:\t\t3\n\t仮想アカウント:\t\tいいえ\n\n" +
		"新しいログオン:\n\tログオン ID:\t\t0xa1b2c\n"
	observation := observeViewer(t, "4624", description)
	field := fieldNamed(t, observation.Fields, "EventData.LogonType")
	if value, ok := field.Text.ComparableValue(); field.Semantic != core.SemanticKeyEventLogonType || !ok || value != "3" {
		t.Errorf("LogonType = %q %q (%t), want event.logon_type 3", field.Semantic, value, ok)
	}
}

// 4625 の失敗したアカウントの見出しは、Windows のバージョンにより「ログオンを失敗した」と「ログオンに失敗した」がある。
func TestObserveMapsBothFailedAccountSections(t *testing.T) {
	for _, section := range []string{"ログオンを失敗したアカウント", "ログオンに失敗したアカウント"} {
		description := "アカウントがログオンに失敗しました。\n\n" + section + ":\n\tアカウント名:\t\tuser04\n"
		observation := observeViewer(t, "4625", description)
		if field := fieldNamed(t, observation.Fields, "EventData.TargetUserName"); field.Semantic != core.SemanticKeyTargetAccountName {
			t.Errorf("%s: TargetUserName semantic = %q", section, field.Semantic)
		}
	}
}
