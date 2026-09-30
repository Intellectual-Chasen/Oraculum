package sigma

import (
	"slices"
	"strings"
)

// service は Sigma の logsource の service 1 つが指す Windows イベントログのチャネルである。
//
// providers は、Channel の欄を持たないレコード (イベントビューアーの CSV) を service へ
// 対応付けるプロバイダの名前である。チャネルに書くプロバイダを 1 つに決められない service
// (system、application など) は providers を持たず、Channel を持つレコードだけに当てる。
//
// providerEventIDs は、複数のチャネルに書くプロバイダを、このチャネルに書くイベント ID に
// 限る表である。表に無いプロバイダはすべてのイベント ID を対応付ける。
type service struct {
	channels         []string
	providers        []string
	providerEventIDs map[string][]string
}

// services は logsource の service の名前から、そのチャネルを探す表である。
//
// 既知の制限: Sigma の Windows の service のうち、チャネルの名前が Windows の定義で決まるものだけを載せる,
// 表に無い service は評価しなかったルールとして件数と理由を画面に出すため、測る対象は評価しなかった件数である,
// 評価しなかったルールの理由に多い service が見つかったとき、そのチャネルを表に足す
var services = map[string]service{
	"security": {
		channels:  []string{"Security"},
		providers: []string{"Microsoft-Windows-Security-Auditing", "Microsoft-Windows-Eventlog"},
		// Microsoft-Windows-Eventlog は System にも書く (104 など)。Security に書くのは監査の
		// 停止、ログの消去、ログの満杯などのイベントである。
		providerEventIDs: map[string][]string{
			"Microsoft-Windows-Eventlog": {"1100", "1101", "1102", "1104", "1105", "1108"},
		},
	},
	"system":      {channels: []string{"System"}},
	"application": {channels: []string{"Application"}},
	"sysmon": {
		channels:  []string{"Microsoft-Windows-Sysmon/Operational"},
		providers: []string{"Microsoft-Windows-Sysmon"},
	},
	"powershell": {
		channels:  []string{"Microsoft-Windows-PowerShell/Operational", "PowerShellCore/Operational"},
		providers: []string{"Microsoft-Windows-PowerShell"},
	},
	"powershell-classic": {channels: []string{"Windows PowerShell"}, providers: []string{"PowerShell"}},
	"taskscheduler": {
		channels:  []string{"Microsoft-Windows-TaskScheduler/Operational"},
		providers: []string{"Microsoft-Windows-TaskScheduler"},
	},
	"windefend": {
		channels:  []string{"Microsoft-Windows-Windows Defender/Operational"},
		providers: []string{"Microsoft-Windows-Windows Defender"},
	},
	"wmi":                       {channels: []string{"Microsoft-Windows-WMI-Activity/Operational"}},
	"ntlm":                      {channels: []string{"Microsoft-Windows-NTLM/Operational"}},
	"codeintegrity-operational": {channels: []string{"Microsoft-Windows-CodeIntegrity/Operational"}},
	"bits-client":               {channels: []string{"Microsoft-Windows-Bits-Client/Operational"}},
	"dns-client":                {channels: []string{"Microsoft-Windows-DNS Client Events/Operational"}},
	"firewall-as": {
		channels: []string{"Microsoft-Windows-Windows Firewall With Advanced Security/Firewall"},
	},
	"terminalservices-localsessionmanager": {
		channels: []string{"Microsoft-Windows-TerminalServices-LocalSessionManager/Operational"},
	},
	"smbclient-security":     {channels: []string{"Microsoft-Windows-SmbClient/Security"}},
	"smbclient-connectivity": {channels: []string{"Microsoft-Windows-SmbClient/Connectivity"}},
	"smbserver-connectivity": {channels: []string{"Microsoft-Windows-SMBServer/Connectivity"}},
	"openssh":                {channels: []string{"OpenSSH/Operational"}},
	"appxdeployment-server":  {channels: []string{"Microsoft-Windows-AppXDeploymentServer/Operational"}},
	"appxpackaging-om":       {channels: []string{"Microsoft-Windows-AppxPackaging/Operational"}},
	"appmodel-runtime":       {channels: []string{"Microsoft-Windows-AppModel-Runtime/Admin"}},
	"capi2":                  {channels: []string{"Microsoft-Windows-CAPI2/Operational"}},
	"certificateservicesclient-lifecycle-system": {
		channels: []string{"Microsoft-Windows-CertificateServicesClient-Lifecycle-System/Operational"},
	},
	"diagnosis-scripted":    {channels: []string{"Microsoft-Windows-Diagnosis-Scripted/Operational"}},
	"dns-server":            {channels: []string{"DNS Server"}},
	"driver-framework":      {channels: []string{"Microsoft-Windows-DriverFrameworks-UserMode/Operational"}},
	"iis-configuration":     {channels: []string{"Microsoft-IIS-Configuration/Operational"}},
	"ldap":                  {channels: []string{"Microsoft-Windows-LDAP-Client/Debug"}},
	"lsa-server":            {channels: []string{"Microsoft-Windows-LSA/Operational"}},
	"msexchange-management": {channels: []string{"MSExchange Management"}},
	"printservice-admin":    {channels: []string{"Microsoft-Windows-PrintService/Admin"}},
	"printservice-operational": {
		channels: []string{"Microsoft-Windows-PrintService/Operational"},
	},
	"security-mitigations": {channels: []string{
		"Microsoft-Windows-Security-Mitigations/Kernel Mode", "Microsoft-Windows-Security-Mitigations/User Mode",
	}},
	"servicebus-client": {channels: []string{
		"Microsoft-ServiceBus-Client/Admin", "Microsoft-ServiceBus-Client/Operational",
	}},
	"shell-core": {channels: []string{"Microsoft-Windows-Shell-Core/Operational"}},
	"applocker": {channels: []string{
		"Microsoft-Windows-AppLocker/MSI and Script", "Microsoft-Windows-AppLocker/EXE and DLL",
		"Microsoft-Windows-AppLocker/Packaged app-Deployment", "Microsoft-Windows-AppLocker/Packaged app-Execution",
	}},
}

// target は logsource が指すレコードの集合 1 つである。service のチャネルのレコードのうち、
// eventIDs のいずれかを EventID に持ち、equals の欄がどれかの値と等しいものを指す。
// eventIDs が空の target はチャネルのすべてのレコードを指す。
type target struct {
	service  string
	eventIDs []string
	equals   map[string][]string
	// fieldNames は、ルールの項目名を、このチャネルのイベントの項目名へ直す表である。
	fieldNames map[string]string
	// fields は、イベントが記録する `<Data>` の Name の集合である。nil は集合を検査しない
	// ことを表す。ルールの項目名を fieldNames で直した後、System の項目 (systemFields) と
	// fields のどちらにも無い項目を参照するルールは、この target に当てない。
	fields map[string]struct{}
	// used は、ルールが参照する項目名を fieldNames で直したものである。ルールごとに
	// compileTargets が埋める。
	used []string
}

// sysmonCategories は Sysmon のイベント ID で表す category である。
var sysmonCategories = map[string][]string{
	"process_creation":         {"1"},
	"file_change":              {"2"},
	"network_connection":       {"3"},
	"sysmon_status":            {"4", "16"},
	"process_termination":      {"5"},
	"driver_load":              {"6"},
	"image_load":               {"7"},
	"create_remote_thread":     {"8"},
	"raw_access_thread":        {"9"},
	"process_access":           {"10"},
	"file_event":               {"11"},
	"registry_event":           {"12", "13", "14"},
	"registry_set":             {"13"},
	"registry_rename":          {"14"},
	"create_stream_hash":       {"15"},
	"pipe_created":             {"17", "18"},
	"wmi_event":                {"19", "20", "21"},
	"dns_query":                {"22"},
	"file_delete":              {"23"},
	"clipboard_change":         {"24"},
	"process_tampering":        {"25"},
	"file_delete_detected":     {"26"},
	"file_block_executable":    {"27"},
	"file_block_shredding":     {"28"},
	"file_executable_detected": {"29"},
	"sysmon_error":             {"255"},
}

// otherCategories は Sysmon のほかのチャネルのイベントで表す category である。
//
// process_creation は Sysmon の 1 と Security の 4688 の 2 つを指す。4688 は作成された
// プロセスを NewProcessName と NewProcessId、作成を要求したプロセスを ParentProcessName と
// ProcessId に書く。4688 は Sysmon の 1 が持つ ParentCommandLine、OriginalFileName、Hashes、
// IntegrityLevel、User などを記録しないため、それらを参照するルールを 4688 に当てない。
// registry_add と registry_delete は Sysmon の 12 を EventType で分ける。
var otherCategories = map[string][]target{
	"process_creation": {{
		service: "security", eventIDs: []string{"4688"},
		fieldNames: map[string]string{
			"Image": "NewProcessName", "ParentImage": "ParentProcessName",
			"ProcessId": "NewProcessId", "ParentProcessId": "ProcessId",
		},
		fields: fieldsOf("SubjectUserSid", "SubjectUserName", "SubjectDomainName", "SubjectLogonId",
			"NewProcessId", "NewProcessName", "TokenElevationType", "ProcessId", "CommandLine",
			"TargetUserSid", "TargetUserName", "TargetDomainName", "TargetLogonId", "ParentProcessName",
			"MandatoryLabel"),
	}},
	"registry_add": {{
		service: "sysmon", eventIDs: []string{"12"}, equals: map[string][]string{"EventType": {"CreateKey"}},
	}},
	"registry_delete": {{
		service: "sysmon", eventIDs: []string{"12"},
		equals: map[string][]string{"EventType": {"DeleteKey", "DeleteValue"}},
	}},
	"ps_module":                 {{service: "powershell", eventIDs: []string{"4103"}}},
	"ps_script":                 {{service: "powershell", eventIDs: []string{"4104"}}},
	"ps_classic_start":          {{service: "powershell-classic", eventIDs: []string{"400"}}},
	"ps_classic_provider_start": {{service: "powershell-classic", eventIDs: []string{"600"}}},
	"ps_classic_script":         {{service: "powershell-classic", eventIDs: []string{"800"}}},
}

// logsource は Sigma のルールの logsource である。
type logsource struct {
	Product  string `yaml:"product"`
	Service  string `yaml:"service"`
	Category string `yaml:"category"`
}

// targets は logsource が指すレコードの集合を返す。評価できない logsource には理由を返す。
func (l logsource) targets() ([]target, string) {
	if !strings.EqualFold(l.Product, "windows") {
		return nil, "logsource product " + quoteOrEmpty(l.Product) + " is not windows"
	}
	if l.Category == "" {
		if l.Service == "" {
			return nil, "logsource has neither service nor category"
		}
		if _, known := services[l.Service]; !known {
			return nil, "logsource service " + quoteOrEmpty(l.Service) + " is not mapped to a channel"
		}
		return []target{{service: l.Service}}, ""
	}
	var found []target
	if eventIDs, sysmon := sysmonCategories[l.Category]; sysmon {
		found = append(found, target{service: "sysmon", eventIDs: eventIDs})
	}
	found = append(found, otherCategories[l.Category]...)
	if len(found) == 0 {
		return nil, "logsource category " + quoteOrEmpty(l.Category) + " is not mapped to an event"
	}
	if l.Service == "" {
		return found, ""
	}
	var narrowed []target
	for _, candidate := range found {
		if candidate.service == l.Service {
			narrowed = append(narrowed, candidate)
		}
	}
	if len(narrowed) == 0 {
		return nil, "logsource category " + quoteOrEmpty(l.Category) + " has no event in service " + quoteOrEmpty(l.Service)
	}
	return narrowed, ""
}

// quoteOrEmpty は理由の文に置く値を引用符で囲む。
func quoteOrEmpty(value string) string {
	return `"` + value + `"`
}

// providerService は、プロバイダ 1 つを対応付ける service と、対応付けるイベント ID である。
// eventIDs が空なら、すべてのイベント ID を対応付ける。
type providerService struct {
	service  string
	eventIDs []string
}

// servicesByChannel と servicesByProvider は、小文字にしたチャネルとプロバイダの名前から
// service を探す。
var servicesByChannel, servicesByProvider = indexServices()

func indexServices() (byChannel map[string]string, byProvider map[string]providerService) {
	byChannel, byProvider = map[string]string{}, map[string]providerService{}
	for name, candidate := range services {
		for _, channel := range candidate.channels {
			byChannel[strings.ToLower(channel)] = name
		}
		for _, provider := range candidate.providers {
			byProvider[strings.ToLower(provider)] = providerService{service: name, eventIDs: candidate.providerEventIDs[provider]}
		}
	}
	return byChannel, byProvider
}

// serviceOf は、レコードの Channel と Provider とイベント ID から service の名前を返す。
// Channel を持たないレコードだけを Provider で対応付ける。
func serviceOf(channel string, hasChannel bool, provider, eventID string) (string, bool) {
	if hasChannel {
		name, found := servicesByChannel[strings.ToLower(channel)]
		return name, found
	}
	mapped, found := servicesByProvider[strings.ToLower(provider)]
	if !found || (len(mapped.eventIDs) > 0 && !slices.Contains(mapped.eventIDs, eventID)) {
		return "", false
	}
	return mapped.service, true
}

// systemFields は、Sigma のルールがイベントの System の項目に付ける名前である。どのイベントも
// これらの項目を持ちうるため、イベントが持つ項目の集合の検査から外す。
var systemFields = map[string]struct{}{
	FieldEventID: {}, FieldChannel: {}, FieldProvider: {}, "Computer": {}, "Level": {}, "Task": {},
	"Opcode": {}, "Keywords": {}, "Version": {}, "EventRecordID": {}, "Provider_Guid": {},
	"Execution_ProcessID": {}, "Execution_ThreadID": {}, "Security_UserID": {},
}

// fieldsOf は項目の名前の一覧から集合を作る。
func fieldsOf(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}
