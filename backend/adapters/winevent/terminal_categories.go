package winevent

import (
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 分類の対象になる、Windows イベントログの外の成果物の種類。値は収集元の種類の名前にも使う。
const (
	ArtifactPrefetch  = "prefetch"
	ArtifactAmcache   = "amcache"
	ArtifactTaskCache = "task_cache"
)

// Windows イベントログのチャネルの名前。
const (
	channelSecurity    = "Security"
	channelSystem      = "System"
	channelApplication = "Application"
)

// 分類に使う、上の定数の外のプロバイダの名前。
const (
	providerServiceControlManager = "Service Control Manager"
	providerEventlog              = "Microsoft-Windows-Eventlog"
	providerDefender              = "Microsoft-Windows-Windows Defender"
	providerMsiInstaller          = "MsiInstaller"
)

// TerminalRecord は、分類を決めるのに読むレコードの値である。Windows イベントログのレコードは
// Provider と EventID と LogonType を、ほかの成果物のレコードは Artifact を持つ。
type TerminalRecord struct {
	Provider  string
	EventID   string
	LogonType string
	Artifact  string
}

// TerminalClassification はレコードの分類と、遠隔のログオンの結果である。
type TerminalClassification struct {
	Categories []core.TerminalCategory
	// LogonOutcome は remote_logon に入るレコードだけが持つ。
	LogonOutcome core.RemoteLogonOutcome
}

// terminalEventCategories は、プロバイダとイベント ID の組から、分類と遠隔のログオンの結果への対応である。
//
// 4624 は LogonType が remoteLogonTypes のときだけ remote_logon に入る (ClassifyTerminalRecord)。
// 1102 は Security のログの消去、104 はほかのログの消去、Defender の 5001 はリアルタイム保護の無効化である。
var terminalEventCategories = map[providerEvent]struct {
	category core.TerminalCategory
	outcome  core.RemoteLogonOutcome
}{
	{providerSecurityAuditing, eventIDLogonFailure}:          {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeFailure},
	{providerSecurityAuditing, eventIDLogonSuccess}:          {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
	{providerRdpCoreTS, "131"}:                               {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeConnection},
	{providerRemoteConnectionManager, "1149"}:                {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
	{providerLocalSessionManager, "21"}:                      {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
	{providerLocalSessionManager, "25"}:                      {core.TerminalCategoryRemoteLogon, core.RemoteLogonOutcomeSuccess},
	{providerSecurityAuditing, "4720"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4722"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4724"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4726"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4728"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4732"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4738"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerSecurityAuditing, "4756"}:                       {category: core.TerminalCategoryAccountManagement},
	{providerTaskScheduler, eventIDTaskRegistered}:           {category: core.TerminalCategoryTaskServiceRegistration},
	{providerSecurityAuditing, "4698"}:                       {category: core.TerminalCategoryTaskServiceRegistration},
	{providerServiceControlManager, eventIDServiceInstalled}: {category: core.TerminalCategoryTaskServiceRegistration},
	{providerPowerShell, eventIDScriptBlock}:                 {category: core.TerminalCategoryPowerShell},
	{providerMsiInstaller, "1033"}:                           {category: core.TerminalCategoryInstallation},
	{providerMsiInstaller, "1040"}:                           {category: core.TerminalCategoryInstallation},
	{providerMsiInstaller, "11707"}:                          {category: core.TerminalCategoryInstallation},
	{providerDefender, "5001"}:                               {category: core.TerminalCategoryDefenseEvasion},
	{providerEventlog, "1102"}:                               {category: core.TerminalCategoryDefenseEvasion},
	{providerEventlog, "104"}:                                {category: core.TerminalCategoryDefenseEvasion},
}

// remoteLogonTypes は、4624 のうち別の端末からのログオンを表す LogonType である。3 はネットワーク、
// 10 はリモート デスクトップである。
var remoteLogonTypes = []string{"3", "10"}

// artifactCategories は成果物の種類から分類への対応である。
var artifactCategories = map[string]core.TerminalCategory{
	ArtifactPrefetch:  core.TerminalCategoryProgramExecution,
	ArtifactAmcache:   core.TerminalCategoryProgramExecution,
	ArtifactTaskCache: core.TerminalCategoryTaskServiceRegistration,
}

// ClassifyTerminalRecord はレコードの分類を返す。どの分類にも入らないレコードでは Categories が
// 要素数 0 である。
func ClassifyTerminalRecord(record TerminalRecord) TerminalClassification {
	if category, found := artifactCategories[record.Artifact]; found {
		return TerminalClassification{Categories: []core.TerminalCategory{category}}
	}
	mapped, found := terminalEventCategories[providerEvent{record.Provider, record.EventID}]
	if !found {
		return TerminalClassification{}
	}
	if record.Provider == providerSecurityAuditing && record.EventID == eventIDLogonSuccess &&
		!slices.Contains(remoteLogonTypes, strings.TrimSpace(record.LogonType)) {
		return TerminalClassification{}
	}
	return TerminalClassification{Categories: []core.TerminalCategory{mapped.category}, LogonOutcome: mapped.outcome}
}

// TerminalCategorySources は分類のレコードを記録する収集元の種類を返す。Windows イベントログは
// チャネルの名前、ほかは成果物の種類である。
func TerminalCategorySources(category core.TerminalCategory) []string {
	switch category {
	case core.TerminalCategoryRemoteLogon:
		return []string{channelSecurity, providerRemoteConnectionManager + "/Operational",
			providerLocalSessionManager + "/Operational", providerRdpCoreTS + "/Operational"}
	case core.TerminalCategoryAccountManagement:
		return []string{channelSecurity}
	case core.TerminalCategoryProgramExecution:
		return []string{ArtifactPrefetch, ArtifactAmcache}
	case core.TerminalCategoryTaskServiceRegistration:
		return []string{providerTaskScheduler + "/Operational", channelSecurity, channelSystem, ArtifactTaskCache}
	case core.TerminalCategoryPowerShell:
		return []string{providerPowerShell + "/Operational"}
	case core.TerminalCategoryInstallation:
		return []string{channelApplication}
	case core.TerminalCategoryDefenseEvasion:
		return []string{providerDefender + "/Operational", channelSecurity, channelSystem}
	default:
		return []string{}
	}
}
