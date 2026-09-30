package winevent_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 分類の対応を 1 行ずつ確かめる。4624 は LogonType が 3 か 10 のときだけ remote_logon に入る。
func TestClassifyTerminalRecordMapsEachEvent(t *testing.T) {
	const (
		security = "Microsoft-Windows-Security-Auditing"
		rcm      = "Microsoft-Windows-TerminalServices-RemoteConnectionManager"
		lsm      = "Microsoft-Windows-TerminalServices-LocalSessionManager"
		rdp      = "Microsoft-Windows-RemoteDesktopServices-RdpCoreTS"
	)
	for _, row := range []struct {
		record   winevent.TerminalRecord
		category core.TerminalCategory
		outcome  core.RemoteLogonOutcome
	}{
		{winevent.TerminalRecord{Provider: security, EventID: "4625"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeFailure},
		{winevent.TerminalRecord{Provider: security, EventID: "4624", LogonType: "3"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
		{winevent.TerminalRecord{Provider: security, EventID: "4624", LogonType: "10"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
		{winevent.TerminalRecord{Provider: security, EventID: "4624", LogonType: "2"}, "", ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4624"}, "", ""},
		{winevent.TerminalRecord{Provider: rdp, EventID: "131"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeConnection},
		{winevent.TerminalRecord{Provider: rcm, EventID: "1149"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
		{winevent.TerminalRecord{Provider: lsm, EventID: "21"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
		{winevent.TerminalRecord{Provider: lsm, EventID: "25"}, core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
		{winevent.TerminalRecord{Provider: lsm, EventID: "23"}, "", ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4720"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4722"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4724"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4726"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4728"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4732"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4738"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4756"}, core.TerminalCategoryAccountManagement, ""},
		{winevent.TerminalRecord{Artifact: winevent.ArtifactPrefetch}, core.TerminalCategoryProgramExecution, ""},
		{winevent.TerminalRecord{Artifact: winevent.ArtifactAmcache}, core.TerminalCategoryProgramExecution, ""},
		{winevent.TerminalRecord{Artifact: winevent.ArtifactTaskCache}, core.TerminalCategoryTaskServiceRegistration, ""},
		{winevent.TerminalRecord{Provider: "Microsoft-Windows-TaskScheduler", EventID: "106"}, core.TerminalCategoryTaskServiceRegistration, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4698"}, core.TerminalCategoryTaskServiceRegistration, ""},
		{winevent.TerminalRecord{Provider: "Service Control Manager", EventID: "7045"}, core.TerminalCategoryTaskServiceRegistration, ""},
		{winevent.TerminalRecord{Provider: "Microsoft-Windows-PowerShell", EventID: "4104"}, core.TerminalCategoryPowerShell, ""},
		{winevent.TerminalRecord{Provider: "MsiInstaller", EventID: "1033"}, core.TerminalCategoryInstallation, ""},
		{winevent.TerminalRecord{Provider: "MsiInstaller", EventID: "1040"}, core.TerminalCategoryInstallation, ""},
		{winevent.TerminalRecord{Provider: "MsiInstaller", EventID: "11707"}, core.TerminalCategoryInstallation, ""},
		{winevent.TerminalRecord{Provider: "Microsoft-Windows-Windows Defender", EventID: "5001"}, core.TerminalCategoryDefenseEvasion, ""},
		{winevent.TerminalRecord{Provider: "Microsoft-Windows-Eventlog", EventID: "1102"}, core.TerminalCategoryDefenseEvasion, ""},
		{winevent.TerminalRecord{Provider: "Microsoft-Windows-Eventlog", EventID: "104"}, core.TerminalCategoryDefenseEvasion, ""},
		{winevent.TerminalRecord{Provider: security, EventID: "4688"}, "", ""},
	} {
		got := winevent.ClassifyTerminalRecord(row.record)
		var want []core.TerminalCategory
		if row.category != "" {
			want = []core.TerminalCategory{row.category}
		}
		if !slices.Equal(got.Categories, want) || got.LogonOutcome != row.outcome {
			t.Errorf("%+v is classified as %+v, want %v %q", row.record, got, want, row.outcome)
		}
	}
}

// 全ての分類が、記録する収集元の種類を 1 つ以上持つ。
func TestTerminalCategorySourcesCoverEveryCategory(t *testing.T) {
	for _, category := range core.TerminalCategories() {
		if len(winevent.TerminalCategorySources(category)) == 0 {
			t.Errorf("%s has no source kind", category)
		}
	}
}

// 7045 の ServiceName はサービスの名前、ImagePath は実行ファイルの path である。
func TestObserveMapsTheServiceInstallation(t *testing.T) {
	observation := observe(t, `<Event><System><Provider Name="Service Control Manager"/>`+
		`<EventID>7045</EventID><TimeCreated SystemTime="2001-02-03T04:05:06Z"/>`+
		`<EventRecordID>501</EventRecordID><Channel>System</Channel>`+
		`<Computer>host05.example.test</Computer></System><EventData>`+
		`<Data Name="ServiceName">ExampleSvc</Data><Data Name="ImagePath">X:\svc\example.exe</Data>`+
		`</EventData></Event>`)
	requireSemantics(t, observation,
		semanticItem{"EventData.ServiceName", core.SemanticKeyServiceName, "ExampleSvc"},
		semanticItem{"EventData.ImagePath", core.SemanticKeyFilePath, `X:\svc\example.exe`})
}
