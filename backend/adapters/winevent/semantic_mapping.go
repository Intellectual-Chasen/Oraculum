package winevent

import (
	"maps"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 本 file が Windows イベントログの項目と語彙の項目の対応を 1 か所で持つ。読み取りの形式
// (XML、CSV、EVTX) は Event を組むだけで、対応を複製しない。
//
// 同じ 1 件の転記を見分ける欄は、Channel と EventRecordID が windows_event.* を持つ今も、
// TranscriptIdentityItems が欄の名前で宣言する。

// providerSecurityAuditing は Security チャネルの監査のイベントを書くプロバイダの名前である。
const providerSecurityAuditing = "Microsoft-Windows-Security-Auditing"

// eventIDProcessCreation は、Security の監査のうち、新しいプロセスの作成を記録するイベントの
// ID である。
const eventIDProcessCreation = "4688"

// eventIDProcessTermination は、Security の監査のうち、プロセスの終了を記録するイベントの ID である。
const eventIDProcessTermination = "4689"

// eventDataKey はプロバイダの名前、イベント ID、`<Data>` の Name の組である。`<Data>` の
// 名前が表す意味は、プロバイダとイベント ID の組ごとにイベントの定義が決める。
type eventDataKey struct {
	provider string
	eventID  string
	name     string
}

// securityEventDataSemantics は Security の監査のイベントの `<Data>` の対応である。
//
// 4688 は新しいプロセスの作成を記録する。NewProcessId と NewProcessName と CommandLine が
// 作成されたプロセス、ProcessId と ParentProcessName が作成を要求したプロセスである。
// Subject と Target のアカウントの対応は securityAccountEventDataSemantics が持つ。
//
// 4689 はプロセスの終了を記録する。ProcessId と ProcessName が終了したプロセスである。
//
// 5156 は通信の許可を記録する。ProcessID と Application が通信したプロセスであり、
// Application は `\device\harddiskvolume<N>\` で始まる path である。**Source は向きに関わらず
// 自分の端末、Dest は相手である。** 表は外向きの対応を持ち、内向きの対応は
// inboundConnectionSemantics が持つ。
//
// 5140 はネットワーク共有へのアクセス、4768 と 4769 は Kerberos のチケットの要求、4698 は
// タスクの登録を記録する。IpAddress と IpPort は要求の接続元である。
//
// 4648 は明示的な資格情報を使ったログオンの要求を記録する。IpAddress と IpPort は向きが逆で、
// 要求の接続先である。TargetServerName は接続先の端末の名前であり、ホスト名のノードを作らない
// 項目へ写す。`localhost` などの端末の中だけで意味を持つ名前と IP アドレスの文字列は写さない
// (namesRemoteServer)。
//
// 既知の制限: 5156 の Protocol と 4672 の PrivilegeList と 4698 の TaskContent と 5140 の
// ShareLocalPath を語彙へ写さない。Protocol は IANA の番号であり connection.protocol の比べる形と
// 異なる。PrivilegeList は 1 つの文字列に複数の特権を持つ。値は原資料の文字列の検索で見つかる,
// 件数やグラフに使う要求が無く測る対象が無い, 番号と名前の対応や特権の区切りを語彙に
// 置いたとき、表に行を足す
var securityEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerSecurityAuditing, eventIDProcessCreation, "NewProcessId"}:      core.SemanticKeyProcessPid,
	{providerSecurityAuditing, eventIDProcessCreation, "NewProcessName"}:    core.SemanticKeyProcessBinaryPath,
	{providerSecurityAuditing, eventIDProcessCreation, "CommandLine"}:       core.SemanticKeyProcessCommandLine,
	{providerSecurityAuditing, eventIDProcessCreation, "ProcessId"}:         core.SemanticKeyParentProcessPid,
	{providerSecurityAuditing, eventIDProcessCreation, "ParentProcessName"}: core.SemanticKeyParentProcessBinaryPath,

	{providerSecurityAuditing, eventIDProcessTermination, "ProcessId"}:   core.SemanticKeyProcessPid,
	{providerSecurityAuditing, eventIDProcessTermination, "ProcessName"}: core.SemanticKeyProcessBinaryPath,

	{providerSecurityAuditing, eventIDConnectionPermitted, "ProcessID"}:     core.SemanticKeyProcessPid,
	{providerSecurityAuditing, eventIDConnectionPermitted, "Application"}:   core.SemanticKeyProcessBinaryPath,
	{providerSecurityAuditing, eventIDConnectionPermitted, "SourceAddress"}: core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, eventIDConnectionPermitted, "SourcePort"}:    core.SemanticKeyConnectionSourcePort,
	{providerSecurityAuditing, eventIDConnectionPermitted, "DestAddress"}:   core.SemanticKeyConnectionDestinationAddress,
	{providerSecurityAuditing, eventIDConnectionPermitted, "DestPort"}:      core.SemanticKeyConnectionDestinationPort,

	{providerSecurityAuditing, "5140", "IpAddress"}: core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, "5140", "IpPort"}:    core.SemanticKeyConnectionSourcePort,
	{providerSecurityAuditing, "5140", "ShareName"}: core.SemanticKeyShareName,

	{providerSecurityAuditing, "4768", "IpAddress"}: core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, "4768", "IpPort"}:    core.SemanticKeyConnectionSourcePort,
	{providerSecurityAuditing, "4769", "IpAddress"}: core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, "4769", "IpPort"}:    core.SemanticKeyConnectionSourcePort,
	// 4768 は LogonGuid の `<Data>` を持たない。
	{providerSecurityAuditing, "4769", "LogonGuid"}: core.SemanticKeyEventTicketLogonGuid,
	// 4648 の LogonGuid は要求した側のセッションの値であり、作ったログオンの値ではないため写さない。
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "TargetLogonGuid"}: core.SemanticKeyEventTargetLogonGuid,
	// 4648 の IpAddress と IpPort は、記録した端末から見た接続先である。
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "IpAddress"}:        core.SemanticKeyConnectionDestinationAddress,
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "IpPort"}:           core.SemanticKeyConnectionDestinationPort,
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "TargetServerName"}: core.SemanticKeyConnectionDestinationServerName,

	{providerSecurityAuditing, "4698", "TaskName"}: core.SemanticKeyScheduledTaskName,
}

// Security の監査のうち、通信の許可と、明示的な資格情報を使ったログオンの要求と、Kerberos の
// サービスチケットの要求を記録するイベントの ID。
const (
	eventIDConnectionPermitted     = "5156"
	eventIDExplicitCredentialLogon = "4648"
	eventIDServiceTicketRequest    = "4769"
)

// namesRemoteServer は、4648 の TargetServerName が別の端末を指す名前であるかを返す。
// 空と `-` と `localhost` と、IP アドレスとして読める文字列は偽である。IP アドレスの接続先は
// IpAddress が持つ。
func namesRemoteServer(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == absentDataText || strings.EqualFold(name, "localhost") {
		return false
	}
	_, err := netip.ParseAddr(name)
	return err != nil
}

// outboundDirections は、5156 の Direction が外向きの通信を表す文字列である。XML は
// メッセージの ID、イベントビューアーの CSV は日本語の文字列で書く。
var outboundDirections = []string{"%%14593", "送信"}

// inboundConnectionSemantics は、内向きの 5156 の `<Data>` の対応である。相手の Dest を接続元、
// 自分の端末の SourcePort を接続先の port へ写す。自分の端末の SourceAddress は語彙を外す
// (空の項目)。接続先のアドレスを持たないため、プロセスの通信の関係を張らない。
var inboundConnectionSemantics = map[string]core.SemanticKey{
	"SourceAddress": "",
	"SourcePort":    core.SemanticKeyConnectionDestinationPort,
	"DestAddress":   core.SemanticKeyConnectionSourceAddress,
	"DestPort":      core.SemanticKeyConnectionSourcePort,
}

// udpProtocol は、5156 の Protocol が UDP を表す番号である。
const udpProtocol = "17"

// groupAddressText は、アドレスの文字列がマルチキャストのアドレスであるか、protocol が UDP で
// ブロードキャストのアドレスであるかを返す。読めない文字列は偽である。ブロードキャストは UDP だけが
// 使うため、TCP の接続のアドレスはブロードキャストと読まない。
//
// ponytail: IPv4 のブロードキャストは、255.255.255.255 と最後の octet が 255 のアドレスで判定する。
// 5156 はネットワークの長さを記録しないため、/24 より長いネットワークのブロードキャスト
// (例: /25 の .127) を見分けられず、/24 より短いネットワークの .255 で終わる端末のアドレスへの
// UDP をブロードキャストと読む。割当がネットワークの長さを持つようになったら、その長さで判定する。
func groupAddressText(text, protocol string) bool {
	address, err := netip.ParseAddr(strings.TrimSpace(text))
	if err != nil {
		return false
	}
	address = address.Unmap()
	if address.IsMulticast() {
		return true
	}
	return strings.TrimSpace(protocol) == udpProtocol && address.Is4() && address.As4()[3] == 255
}

// outboundConnectionOf は、Event が端末から 1 つの相手へ始めた接続を記録したかを返す
// (Observation.OutboundConnection)。Sysmon の Protocol は名前で書き、5156 は番号で書く。
func outboundConnectionOf(event Event, kind providerEvent) bool {
	switch kind {
	case providerEvent{providerSecurityAuditing, eventIDExplicitCredentialLogon}:
		return true
	case providerEvent{providerSecurityAuditing, eventIDConnectionPermitted}:
		return slices.Contains(outboundDirections, eventDataText(event, "Direction")) &&
			!groupAddressText(eventDataText(event, "DestAddress"), eventDataText(event, "Protocol"))
	case providerEvent{providerSysmon, sysmonNetworkConnect}:
		protocol := eventDataText(event, "Protocol")
		if strings.EqualFold(strings.TrimSpace(protocol), "udp") {
			protocol = udpProtocol
		}
		return sysmonInitiated(event) && !groupAddressText(eventDataText(event, "DestinationIp"), protocol)
	}
	return false
}

// endpointSemanticsOf は、通信の向きにより語彙の項目を替える `<Data>` の名前と、替えた項目を
// 返す。空の項目は語彙を外す。項目を替えないイベントでは nil である。
func endpointSemanticsOf(event Event) map[string]core.SemanticKey {
	switch (providerEvent{textOf(event.System.ProviderName), textOf(event.System.EventID)}) {
	case providerEvent{providerSysmon, sysmonNetworkConnect}:
		if !sysmonInitiated(event) {
			return sysmonUnlinkedEndpointSemantics
		}
	case providerEvent{providerSecurityAuditing, eventIDConnectionPermitted}:
		if !slices.Contains(outboundDirections, eventDataText(event, "Direction")) {
			return inboundConnectionSemantics
		}
	}
	return nil
}

// Security の監査のうち、ログオンの種別を記録するイベントの ID。
const (
	eventIDLogonSuccess = "4624"
	eventIDLogonFailure = "4625"
	eventIDLogoff       = "4634"
)

// securityLogonEventDataSemantics は、Security の監査のうち、ログオンの種別を記録する
// イベントの `<Data>` の対応である。
//
// 4624 はログオンの成功、4625 はログオンの失敗、4634 はログオフを記録する。
// IpAddress と IpPort と WorkstationName はログオンの接続元である。Subject と Target の
// アカウントの対応は securityAccountEventDataSemantics が持つ。**LogonType を持たない
// イベントに種別のコードを与えない。** 4768、4769、4776 などの認証のイベントは LogonType の
// `<Data>` を持たず、表に載せない。
//
// Status と SubStatus は 4625 の失敗の理由を 16 進のコードで持つ。FailureReason は同じ理由の
// メッセージの ID であり、コードではないため写さない。
//
// 4624 の TargetLogonId はログオンが作ったセッションの Logon ID である。4634 と 4647 の
// TargetLogonId は終えたセッションの Logon ID であり、event.logoff_logon_id へ写す。
// TargetLinkedLogonId は、管理者のアカウントのログオンが 2 つに分けて作ったセッションの
// もう片方であり、event.target_linked_logon_id へ写す。LogonGuid は Kerberos のチケットと
// ログオンを結ぶ値であり、event.target_logon_guid へ写す。
//
// 既知の制限: ElevatedToken を語彙へ写さない。分けた 2 つのログオンの関係の向きは取り込んだ順で
// 決め、昇格の有無は根拠のレコードの原資料の文字列で読む, 件数やグラフに使う要求が無く測る対象が無い,
// 昇格した側を起点にする要求が出たとき
var securityLogonEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerSecurityAuditing, eventIDLogonSuccess, "TargetLogonId"}:       core.SemanticKeyEventTargetLogonId,
	{providerSecurityAuditing, eventIDLogonSuccess, "TargetLinkedLogonId"}: core.SemanticKeyEventTargetLinkedLogonId,
	{providerSecurityAuditing, eventIDLogonSuccess, "LogonGuid"}:           core.SemanticKeyEventTargetLogonGuid,
	{providerSecurityAuditing, eventIDLogonSuccess, "LogonType"}:           core.SemanticKeyEventLogonType,
	{providerSecurityAuditing, eventIDLogonSuccess, "IpAddress"}:           core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, eventIDLogonSuccess, "IpPort"}:              core.SemanticKeyConnectionSourcePort,
	{providerSecurityAuditing, eventIDLogonSuccess, "WorkstationName"}:     core.SemanticKeyRemoteSessionClientHostname,
	// TargetOutboundUserName と TargetOutboundDomainName は、作ったセッションが他の端末への
	// 接続に使う資格情報であり、ログオンしたアカウントではない (LogonType 9)。
	{providerSecurityAuditing, eventIDLogonSuccess, "TargetOutboundUserName"}:   core.SemanticKeyEventOutboundAccountName,
	{providerSecurityAuditing, eventIDLogonSuccess, "TargetOutboundDomainName"}: core.SemanticKeyEventOutboundAccountDomain,

	{providerSecurityAuditing, eventIDLogonFailure, "LogonType"}:       core.SemanticKeyEventLogonType,
	{providerSecurityAuditing, eventIDLogonFailure, "IpAddress"}:       core.SemanticKeyConnectionSourceAddress,
	{providerSecurityAuditing, eventIDLogonFailure, "IpPort"}:          core.SemanticKeyConnectionSourcePort,
	{providerSecurityAuditing, eventIDLogonFailure, "WorkstationName"}: core.SemanticKeyRemoteSessionClientHostname,
	{providerSecurityAuditing, eventIDLogonFailure, "Status"}:          core.SemanticKeyEventLogonFailureStatus,
	{providerSecurityAuditing, eventIDLogonFailure, "SubStatus"}:       core.SemanticKeyEventLogonFailureSubStatus,

	// AuthenticationPackageName は、ログオンの認証に用いた方式の名前 (NTLM、Kerberos など) である。
	{providerSecurityAuditing, eventIDLogonSuccess, "AuthenticationPackageName"}: core.SemanticKeyEventAuthenticationPackage,
	{providerSecurityAuditing, eventIDLogonFailure, "AuthenticationPackageName"}: core.SemanticKeyEventAuthenticationPackage,

	{providerSecurityAuditing, eventIDLogoff, "LogonType"}:                  core.SemanticKeyEventLogonType,
	{providerSecurityAuditing, eventIDLogoff, "TargetLogonId"}:              core.SemanticKeyEventLogoffLogonId,
	{providerSecurityAuditing, eventIDUserInitiatedLogoff, "TargetLogonId"}: core.SemanticKeyEventLogoffLogonId,
}

// eventIDUserInitiatedLogoff は、利用者が始めたログオフを記録する Security の監査のイベントの ID である。
const eventIDUserInitiatedLogoff = "4647"

// systemStartEvents は、端末の起動を記録するイベントである。4608 は Windows の起動、6005 は
// イベントログのサービスの開始、Kernel-General の 12 は OS の起動を記録する。
var systemStartEvents = map[providerEvent]struct{}{
	{providerSecurityAuditing, "4608"}:         {},
	{"EventLog", "6005"}:                       {},
	{"Microsoft-Windows-Kernel-General", "12"}: {},
}

// accountEventData は、1 つの役割のアカウントを持つ `<Data>` の Name の組と、写す語彙の項目である。
type accountEventData struct {
	sidName, userName, domainName string
	sid, user, domain             core.SemanticKey
}

// Security の監査のイベントが Subject と Target のアカウントを書く `<Data>` の組。
var (
	subjectAccountData = accountEventData{
		"SubjectUserSid", "SubjectUserName", "SubjectDomainName",
		core.SemanticKeySubjectAccountSid, core.SemanticKeySubjectAccountName,
		core.SemanticKeySubjectAccountDomain,
	}
	targetUserAccountData = accountEventData{
		"TargetUserSid", "TargetUserName", "TargetDomainName",
		core.SemanticKeyTargetAccountSid, core.SemanticKeyTargetAccountName,
		core.SemanticKeyTargetAccountDomain,
	}
	targetSidAccountData = accountEventData{
		"TargetSid", "TargetUserName", "TargetDomainName",
		core.SemanticKeyTargetAccountSid, core.SemanticKeyTargetAccountName,
		core.SemanticKeyTargetAccountDomain,
	}
	targetGroupData = accountEventData{
		"TargetSid", "TargetUserName", "TargetDomainName",
		core.SemanticKeyTargetGroupSid, core.SemanticKeyTargetGroupName, core.SemanticKeyTargetGroupDomain,
	}
)

// subjectAccountEventIDs は、Subject のアカウントを写す Security の監査のイベントの ID である。
var subjectAccountEventIDs = []string{
	eventIDLogonSuccess, eventIDLogonFailure, eventIDExplicitCredentialLogon, eventIDProcessCreation,
	"4672", "4698", "4703",
	"4720", "4722", "4724", "4726", "4728", "4732", "4738", "4756", "5140", "5379", "5381", "5382",
}

// groupMemberAddedEventIDs は、グローバル・ローカル・ユニバーサルのセキュリティ グループへの
// メンバーの追加を記録する Security の監査のイベントの ID である。
var groupMemberAddedEventIDs = [3]string{"4728", "4732", "4756"}

// groupMemberEventDataSemantics は、グループへ追加されたメンバーの `<Data>` の対応である。
// MemberDomainName と MemberUserName は `<Data>` の名前ではない。イベントビューアーが名前へ
// 直したメンバーの「セキュリティ ID」を分けた値の名前である (descriptionValues)。
func groupMemberEventDataSemantics() map[eventDataKey]core.SemanticKey {
	table := make(map[eventDataKey]core.SemanticKey)
	for _, eventID := range groupMemberAddedEventIDs {
		table[eventDataKey{providerSecurityAuditing, eventID, "MemberSid"}] = core.SemanticKeyTargetAccountSid
		table[eventDataKey{providerSecurityAuditing, eventID, "MemberName"}] = core.SemanticKeyTargetAccountName
		table[eventDataKey{providerSecurityAuditing, eventID, "MemberUserName"}] = core.SemanticKeyTargetAccountName
		table[eventDataKey{providerSecurityAuditing, eventID, "MemberDomainName"}] = core.SemanticKeyTargetAccountDomain
	}
	return table
}

// securityAccountEventDataSemantics は、Security の監査のイベントが役割を付けて書いた
// アカウントの `<Data>` の対応である。
//
// Subject は記録した操作を行ったアカウントである。ログオンとログオンの失敗では、ログオンを
// 要求したアカウントである。Target は、ログオンとログオンの失敗とログオフではログオンの
// 対象のアカウント、4688 では作成されたプロセスのアカウントである。
//
// 4625 は存在しないアカウントの名前も記録する。そのときの TargetUserSid は NULL SID であり、
// 試行された名前はドメインと名前の組のノードになる (core.NewRecordGraph)。
//
// 4720 と 4726 の Target は作成または削除されたアカウント、4722・4724・4738 の Target は
// 有効にされた・パスワードを設定された・変更されたアカウント、4768 の Target はチケットを
// 要求したアカウントである。4728・4732・4756 の Member はグループへ追加されたアカウントであり、
// Target はグループである (target_group)。MemberName は識別名 (DN) であり、先頭の CN の値を
// 比べる (accountNameData)。
//
// 4769 は、Target の欄にチケットを要求したアカウントを、Service の欄に要求された
// サービスのアカウントを書く。**要求したアカウントを Subject、サービスを Target へ写す。**
// 要求したアカウントの名前は `<名前>@<領域>` の形であり、`@` の前を比べる
// (accountNameData)。4648 の Target は、ログオンの要求に使った資格情報のアカウントである。
// 4769 の Subject と 4648 の Target は SID の欄を持たず、ドメインと名前の組のノードになる。
var securityAccountEventDataSemantics = mergeEventDataSemantics(
	accountEventDataSemantics(subjectAccountData, subjectAccountEventIDs...),
	accountEventDataSemantics(targetUserAccountData,
		eventIDLogonSuccess, eventIDLogonFailure, eventIDLogoff, eventIDProcessCreation),
	accountEventDataSemantics(targetSidAccountData, "4720", "4722", "4724", "4726", "4738", "4768"),
	accountEventDataSemantics(targetGroupData, groupMemberAddedEventIDs[:]...),
	groupMemberEventDataSemantics(),
	map[eventDataKey]core.SemanticKey{
		{providerSecurityAuditing, eventIDServiceTicketRequest, "ServiceSid"}:  core.SemanticKeyTargetAccountSid,
		{providerSecurityAuditing, eventIDServiceTicketRequest, "ServiceName"}: core.SemanticKeyTargetAccountName,
		// ServiceDomainName と ServiceUserName は `<Data>` の名前ではない。イベントビューアーが
		// 名前へ直した「サービス ID」を分けた値の名前である (accountIdValues)。
		{providerSecurityAuditing, eventIDServiceTicketRequest, "ServiceDomainName"}:   core.SemanticKeyTargetAccountDomain,
		{providerSecurityAuditing, eventIDServiceTicketRequest, "ServiceUserName"}:     core.SemanticKeyTargetAccountName,
		{providerSecurityAuditing, eventIDServiceTicketRequest, "TargetUserName"}:      core.SemanticKeySubjectAccountName,
		{providerSecurityAuditing, eventIDServiceTicketRequest, "TargetDomainName"}:    core.SemanticKeySubjectAccountDomain,
		{providerSecurityAuditing, eventIDExplicitCredentialLogon, "TargetUserName"}:   core.SemanticKeyTargetAccountName,
		{providerSecurityAuditing, eventIDExplicitCredentialLogon, "TargetDomainName"}: core.SemanticKeyTargetAccountDomain,
	},
)

// accountNameData は、アカウントの名前を別の形の文字列で書く `<Data>` と、名前を正規化値に置く
// 関数である。原資料の文字列を保つ。
//
// 4769 の TargetUserName は `<名前>@<領域>` の形、グループの変更の MemberName は識別名 (DN) で
// ある。
var accountNameData = map[eventDataKey]func(core.RecordField) core.RecordField{
	{providerSecurityAuditing, eventIDServiceTicketRequest, "TargetUserName"}: principalNameField,
	{providerSecurityAuditing, "4728", "MemberName"}:                          distinguishedNameField,
	{providerSecurityAuditing, "4732", "MemberName"}:                          distinguishedNameField,
	{providerSecurityAuditing, "4756", "MemberName"}:                          distinguishedNameField,
}

// principalNameDerivation は `<名前>@<領域>` から名前を取り出した導き方である。
const principalNameDerivation = "user principal name without the realm"

// distinguishedNameDerivation は識別名の先頭の CN の値を名前にした導き方である。
const distinguishedNameDerivation = "common name of the first relative distinguished name"

// principalNameField は、`@` を含む値の `@` の前を正規化値に置いた項目を返す。`@` を含まない
// 値と値の不在は field のまま返す。
func principalNameField(field core.RecordField) core.RecordField {
	text, readable := field.Text.RawTextValue()
	name, _, found := strings.Cut(text, "@")
	if !readable || !found || name == "" || field.Text.ValueState != core.ValueStatePresent {
		return field
	}
	return normalizedNameField(field, text, name, principalNameDerivation)
}

// distinguishedNameField は、`CN=` で始まる識別名の先頭の CN の値を正規化値に置いた項目を
// 返す。`\` で escape した字は escape を外す。`CN=` で始まらない値と値の不在は field のまま返す。
func distinguishedNameField(field core.RecordField) core.RecordField {
	text, readable := field.Text.RawTextValue()
	if !readable || field.Text.ValueState != core.ValueStatePresent || len(text) < 3 ||
		!strings.EqualFold(text[:3], "CN=") {
		return field
	}
	var name strings.Builder
	for at := 3; at < len(text) && text[at] != ','; at++ {
		if text[at] == '\\' && at+1 < len(text) {
			at++
		}
		name.WriteByte(text[at])
	}
	if name.Len() == 0 {
		return field
	}
	return normalizedNameField(field, text, name.String(), distinguishedNameDerivation)
}

// normalizedNameField は、原資料の文字列 text の比べる形を name にした項目を返す。組めないときは field の
// まま返す。
func normalizedNameField(field core.RecordField, text, name, derivation string) core.RecordField {
	value, err := core.NewNormalizedValue(core.ValueStatePresent, text, name, derivation)
	if err != nil {
		return field
	}
	normalized, err := core.NewTextField(field.Name, field.Semantic, value)
	if err != nil {
		return field
	}
	return normalized
}

// accountEventDataSemantics は、イベントの ID ごとに、1 つの役割のアカウントの対応を組む。
func accountEventDataSemantics(
	data accountEventData, eventIDs ...string,
) map[eventDataKey]core.SemanticKey {
	table := make(map[eventDataKey]core.SemanticKey, 3*len(eventIDs))
	for _, eventID := range eventIDs {
		table[eventDataKey{providerSecurityAuditing, eventID, data.sidName}] = data.sid
		table[eventDataKey{providerSecurityAuditing, eventID, data.userName}] = data.user
		table[eventDataKey{providerSecurityAuditing, eventID, data.domainName}] = data.domain
	}
	return table
}

// mergeEventDataSemantics は、組が重ならない対応の表を 1 つにまとめる。
func mergeEventDataSemantics(
	tables ...map[eventDataKey]core.SemanticKey,
) map[eventDataKey]core.SemanticKey {
	merged := make(map[eventDataKey]core.SemanticKey)
	for _, table := range tables {
		maps.Copy(merged, table)
	}
	return merged
}

// securitySubjectLogonEventIDs は、SubjectLogonId の `<Data>` に、記録した操作を行った
// セッションの Logon ID を書く Security の監査のイベントの ID である。ログオンとログオンの
// 失敗では、ログオンを要求したセッションである。
//
// 既知の制限: SubjectLogonId を写すイベントを ID の一覧で挙げる, 一覧はイベントの定義が
// SubjectLogonId を置くイベントのうち、ログオン・特権・アカウントとグループの変更・
// プロセス・サービスとタスク・オブジェクトと共有への操作であり、定義の全体を数えていない
// ため件数を測れない, 一覧に無いイベントの SubjectLogonId を関係に使う要求が出たとき、
// 一覧に足す
var securitySubjectLogonEventIDs = []string{
	eventIDLogonSuccess, eventIDLogonFailure, "4648", "4672", "4673", "4674",
	eventIDProcessCreation, "4689", "4696", "4697", "4698", "4699", "4700", "4701", "4702",
	"4720", "4722", "4723", "4724", "4725", "4726", "4727", "4728", "4729", "4731", "4732",
	"4733", "4737", "4738", "4740", "4754", "4756", "4757", "4767", "4780", "4781", "4798",
	"4799", "4656", "4658", "4660", "4661", "4662", "4663", "4670", "4985", "5140", "5142",
	"5143", "5144", "5145", "4703", "5379", "5381", "5382",
}

// securitySubjectLogonEventDataSemantics は securitySubjectLogonEventIDs の SubjectLogonId の対応
// である。
var securitySubjectLogonEventDataSemantics = subjectLogonIdSemantics(securitySubjectLogonEventIDs)

// subjectLogonIdSemantics は、イベントの ID ごとに SubjectLogonId の対応を 1 行ずつ組む。
func subjectLogonIdSemantics(eventIDs []string) map[eventDataKey]core.SemanticKey {
	table := make(map[eventDataKey]core.SemanticKey, len(eventIDs))
	for _, eventID := range eventIDs {
		table[eventDataKey{providerSecurityAuditing, eventID, "SubjectLogonId"}] =
			core.SemanticKeyEventSubjectLogonId
	}
	return table
}

// providerTaskScheduler はタスク スケジューラのイベントを書くプロバイダの名前である。
const providerTaskScheduler = "Microsoft-Windows-TaskScheduler"

// タスク スケジューラのうち、タスクの登録と起動を記録するイベントの ID。
const (
	eventIDTaskRegistered     = "106"
	eventIDTaskStarted        = "100"
	eventIDTaskProcessCreated = "129"
	eventIDTaskActionStarted  = "200"
)

// taskSchedulerEventDataSemantics はタスク スケジューラのイベントの `<Data>` の対応である。
//
// 106 はタスクの登録、100 はタスクのインスタンスの開始、129 はタスクのプロセスの作成、
// 200 は操作の開始を記録する。TaskName はその対象のタスクの名前であり、129 は名前の前に
// taskNamePrefix を付けることがある。129 の ProcessID と Path は作成したプロセスの番号と
// 実行ファイルである。**129 を起動の記録にしない** (processStartEvents)。同じプロセスの 4688 が
// 開いた番号の区間へ入り、4688 の親子と続きの事象から切り離さない。
//
// 既知の制限: 終了の記録の無い同じ番号の前の区間が開いているとき、129 はその区間に入る,
// 129 と 4688 は時刻の差を比べる規則を持たず、同じプロセスかを決められない, 番号の再利用で
// 別のプロセスが 1 つの区間にまとまる入力が見つかったとき
//
// 既知の制限: 200 と 201 の ActionName、UserContext を語彙へ写さない。ActionName は COM の
// handler では path の形を持たず、pid の無いレコードの実行ファイルはグラフに何も足さない。
// UserContext は `ドメイン\名前` の文字列で SID を持たない。値は原資料の文字列の検索で見つかる,
// 件数やグラフに使う要求が無く測る対象が無い, 起動した操作をアカウントと結ぶ要求が出たとき
var taskSchedulerEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerTaskScheduler, eventIDTaskRegistered, "TaskName"}:      core.SemanticKeyScheduledTaskName,
	{providerTaskScheduler, eventIDTaskStarted, "TaskName"}:         core.SemanticKeyStartedTaskName,
	{providerTaskScheduler, eventIDTaskProcessCreated, "TaskName"}:  core.SemanticKeyStartedTaskName,
	{providerTaskScheduler, eventIDTaskProcessCreated, "Path"}:      core.SemanticKeyProcessBinaryPath,
	{providerTaskScheduler, eventIDTaskProcessCreated, "ProcessID"}: core.SemanticKeyProcessPid,
	{providerTaskScheduler, eventIDTaskActionStarted, "TaskName"}:   core.SemanticKeyStartedTaskName,
}

// taskNamePrefix は、129 がタスクの名前の前に付ける文字列である。後ろに `\` から始まる名前が続く。
const taskNamePrefix = `NT TASK`

// taskNamePrefixDerivation は taskNamePrefix を外した導き方である。
const taskNamePrefixDerivation = "task name without the NT TASK prefix"

// powerShellEventDataSemantics は PowerShell のイベントの `<Data>` の対応である。
//
// 4104 の ScriptBlockText は PowerShell が実行したスクリプトブロックの本文である。本文が
// 長いときは 1 つのブロックを MessageTotal 件に分けて記録し、各件は本文の一部だけを持つ。
// Path は本文を読んだスクリプトのファイルであり、対話の入力の本文では空である。
//
// 既知の制限: 分けた件を 1 つの本文へつながない。件の境界で切れた path は、切れた文字列の
// まま読まれるか読まれない, 分けた件は ScriptBlockId と MessageNumber の順で原資料の文字列を並べて
// 読める, 境界で切れた path を記録した対象として辿る要求が出たとき
var powerShellEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerPowerShell, eventIDScriptBlock, "ScriptBlockText"}: core.SemanticKeyProcessShellCommand,
	{providerPowerShell, eventIDScriptBlock, "Path"}:            core.SemanticKeyFilePath,
}

// リモート デスクトップの接続とセッションを記録するプロバイダの名前。
const (
	providerRemoteConnectionManager = "Microsoft-Windows-TerminalServices-RemoteConnectionManager"
	providerLocalSessionManager     = "Microsoft-Windows-TerminalServices-LocalSessionManager"
	providerRdpCoreTS               = "Microsoft-Windows-RemoteDesktopServices-RdpCoreTS"
)

// remoteDesktopEventDataSemantics はリモート デスクトップのイベントの項目の対応である。
// 1149 と LocalSessionManager のイベントは値を `<UserData>` に書き、項目の名前は要素の
// path である。
//
// 1149 は認証に成功した接続を記録し、Param1 と Param2 がアカウントの名前と領域、Param3 が
// 接続元のアドレスである。LocalSessionManager の 21 はログオン、23 はログオフ、24 は切断、
// 25 は再接続を記録し、User は `領域\名前` の文字列のアカウント、Address は接続元である。
// Address は端末の前で操作したセッションでは IP アドレスでない文字列を持つ (textField)。
// RdpCoreTS の 131 は認証の前の接続の受け付けを記録し、ClientIP は port を付けたアドレスである。
//
// 既知の制限: SessionID を語彙へ写さない。対応する語彙の項目が無い。値は原資料の文字列の検索で見つかる,
// 件数やグラフに使う要求が無く測る対象が無い, セッションの番号で記録を結ぶ要求が出たとき
var remoteDesktopEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerRemoteConnectionManager, "1149", "UserData.EventXML.Param1"}: core.SemanticKeyTargetAccountName,
	{providerRemoteConnectionManager, "1149", "UserData.EventXML.Param2"}: core.SemanticKeyTargetAccountDomain,
	{providerRemoteConnectionManager, "1149", "UserData.EventXML.Param3"}: core.SemanticKeyConnectionSourceAddress,

	{providerLocalSessionManager, "21", "UserData.EventXML.User"}:    core.SemanticKeyTargetAccountName,
	{providerLocalSessionManager, "21", "UserData.EventXML.Address"}: core.SemanticKeyConnectionSourceAddress,
	{providerLocalSessionManager, "23", "UserData.EventXML.User"}:    core.SemanticKeyTargetAccountName,
	{providerLocalSessionManager, "24", "UserData.EventXML.User"}:    core.SemanticKeyTargetAccountName,
	{providerLocalSessionManager, "24", "UserData.EventXML.Address"}: core.SemanticKeyConnectionSourceAddress,
	{providerLocalSessionManager, "25", "UserData.EventXML.User"}:    core.SemanticKeyTargetAccountName,
	{providerLocalSessionManager, "25", "UserData.EventXML.Address"}: core.SemanticKeyConnectionSourceAddress,

	{providerRdpCoreTS, "131", "ClientIP"}: core.SemanticKeyConnectionSourceAddress,
}

// remoteSessionEvents は、端末へ入った遠隔のセッションを記録するイベントである。131 は
// 認証の前の接続の受け付けであり、載せない。
var remoteSessionEvents = map[providerEvent]struct{}{
	{providerRemoteConnectionManager, "1149"}: {},
	{providerLocalSessionManager, "21"}:       {},
	{providerLocalSessionManager, "24"}:       {},
	{providerLocalSessionManager, "25"}:       {},
}

// eventIDServiceInstalled は、サービス コントロール マネージャーのうち、サービスの登録を記録する
// イベントの ID である。
const eventIDServiceInstalled = "7045"

// serviceEventDataSemantics はサービスの登録のイベントの `<Data>` の対応である。ServiceName は
// 登録されたサービスの名前、ImagePath はサービスが起動する実行ファイルの path である。
//
// 既知の制限: ImagePath は引数と引用符を含む文字列をそのまま file.path にする, 引数を分ける規則が
// 実行ファイルの path の書き方ごとに異なり、1 つの規則を決める入力が無く測れない, 引数を含む値で同じファイルの
// ノードが分かれる実例を確認したとき
var serviceEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerServiceControlManager, eventIDServiceInstalled, "ServiceName"}: core.SemanticKeyServiceName,
	{providerServiceControlManager, eventIDServiceInstalled, "ImagePath"}:   core.SemanticKeyFilePath,
}

// providerDnsClient は、端末の DNS の名前の解決と登録を記録するプロバイダの名前である。
const providerDnsClient = "Microsoft-Windows-DNS-Client"

// dnsClientEventDataSemantics は DNS-Client のイベントの `<Data>` の対応である。8020 は、端末が
// 自分の名前と IP アドレスを DNS サーバへ登録できなかったことを記録し、Ipaddress にその端末の
// adapter の IP アドレスを書く。DnsServerList と Sent UpdateServer は DNS サーバの値であり、写さない。
//
// 既知の制限: 8020 が記録した IP アドレスからの接続元が未同定の遠隔のセッションの候補は、
// 同じ端末のノードへは、レコードの時刻に関わらず作られない, 8020 は 1 時点の記録で、その IP
// アドレスを持った期間を持たない, 同じ IP アドレスを後から別の端末が持つ入力を確認したとき
var dnsClientEventDataSemantics = map[eventDataKey]core.SemanticKey{
	{providerDnsClient, "8020", "Ipaddress"}: core.SemanticKeyTerminalIpAddress,
}

// eventDataSemanticTables は `<Data>` の対応の表の一覧である。プロバイダまたはイベントの
// 群ごとに表を分け、ここへ並べる。2 つの表が同じ組を持つときは、先に並べた表が決める。
var eventDataSemanticTables = []map[eventDataKey]core.SemanticKey{
	dnsClientEventDataSemantics,
	securityEventDataSemantics,
	securityLogonEventDataSemantics,
	securityAccountEventDataSemantics,
	securitySubjectLogonEventDataSemantics,
	taskSchedulerEventDataSemantics,
	powerShellEventDataSemantics,
	remoteDesktopEventDataSemantics,
	serviceEventDataSemantics,
	sysmonEventDataSemantics,
}

// systemSemantics は System の項目の対応である。
//
// Computer はイベントを記録した端末の名前である。端末に製品が与えた外部識別子ではないため、
// terminal.id へ写さない。
//
// Provider@Name と EventID と Channel と EventRecordID は、イベントを書いたプロバイダ、
// イベント ID、チャネル、チャネルの中のレコード番号である。
var systemSemantics = map[string]core.SemanticKey{
	nameComputer:      core.SemanticKeyTerminalHostname,
	nameSystemTime:    core.SemanticKeyEventTime,
	nameProviderName:  core.SemanticKeyWindowsEventProvider,
	nameEventID:       core.SemanticKeyWindowsEventId,
	nameChannel:       core.SemanticKeyWindowsEventChannel,
	nameEventRecordID: core.SemanticKeyWindowsEventRecordId,
}

// userIDNamesSubject は、Security@UserID を操作の主体のアカウントの識別子として扱うかを返す。
//
// Security@UserID はイベントを書いたスレッドのアカウントである。`<Data>` が主体の識別子または
// 名前を記録したイベントは `<Data>` の値を使う。4769 は要求したアカウントを名前だけで記録する。
// Sysmon は自身のサービスのアカウントを書くため除く。
func userIDNamesSubject(event Event) bool {
	if textOf(event.System.ProviderName) == providerSysmon {
		return false
	}
	for _, data := range event.EventData {
		switch eventDataSemanticOf(event.System, data.Name) {
		case core.SemanticKeySubjectAccountSid, core.SemanticKeySubjectAccountName:
			return false
		}
	}
	return true
}

// sectionSemantics は Event.Sections の値の対応である。RenderingInfo の Message は、
// 書き出した端末がイベントの定義から組んだ本文である。
var sectionSemantics = map[string]core.SemanticKey{
	nameMessage: core.SemanticKeyEventMessage,
}

// nameMessage は書き出した端末が組んだ本文の名前である。
const nameMessage = "RenderingInfo.Message"

// machineAccountPattern は `<ホスト名>$` の形のアカウント名である。コンピューターのアカウントは
// NetBIOS 名 (15 字まで) に `$` を付けた名前であり、`ドメイン\` の後ろにも現れる。
var machineAccountPattern = regexp.MustCompile(`(?:^|[\s\\"])([A-Za-z0-9][A-Za-z0-9._-]{0,14}\$)(?:[\s"]|$)`)

// administrativeShares は Windows が定める管理用の共有の名前である。`\\*\IPC$` のように
// 共有の path に現れ、コンピューターのアカウント名と同じ形を持つ。1 文字の名前はドライブの
// 共有 (`C$`) である。
var administrativeShares = []string{"ADMIN$", "IPC$", "PRINT$", "FAX$"}

// machineAccountsIn は text が指すコンピューターのアカウント名のうち、seen に無いものを
// seen の後ろへ足して返す。管理用の共有の名前を除く。
func machineAccountsIn(text string, seen []string) []string {
	if !strings.Contains(text, "$") {
		return seen
	}
	for _, match := range machineAccountPattern.FindAllStringSubmatch(text, -1) {
		name := strings.ToUpper(match[1])
		if len(name) == len("C$") || slices.Contains(administrativeShares, name) {
			continue
		}
		if !slices.Contains(seen, match[1]) {
			seen = append(seen, match[1])
		}
	}
	return seen
}

// eventDataSemanticOf は 1 つの `<Data>` の語彙の項目を返す。対応の無い値には空の値を返し、
// 空の値は Windows イベントログに固有の意味を持つ。
func eventDataSemanticOf(system System, name string) core.SemanticKey {
	key := eventDataKey{provider: textOf(system.ProviderName), eventID: textOf(system.EventID), name: name}
	for _, table := range eventDataSemanticTables {
		if semantic, found := table[key]; found {
			return semantic
		}
	}
	return ""
}

// unnamedDataSemantics は、Name の無い `<Data>` を書くイベントの、`<Data>` の出現の順の語彙の
// 項目である。空の値の位置と、表の長さより後ろの `<Data>` は語彙を持たない。
var unnamedDataSemantics = map[providerEvent][]core.SemanticKey{
	// 1033 と 1034 はインストールと削除の完了であり、製品の名前、版、言語、状態、製造元の順に書く。
	{providerMsiInstaller, "1033"}: {core.SemanticKeyFileProductName, core.SemanticKeyFileProductVersion},
	{providerMsiInstaller, "1034"}: {core.SemanticKeyFileProductName, core.SemanticKeyFileProductVersion},
	// 1040 はインストーラの処理の開始であり、パッケージの path を書く。
	{providerMsiInstaller, "1040"}: {core.SemanticKeyFilePath},
	// 11707 と 11724 は製品の名前を含む結果の文を書く。
	{providerMsiInstaller, "11707"}: {core.SemanticKeyEventMessage},
	{providerMsiInstaller, "11724"}: {core.SemanticKeyEventMessage},
}

// ItemSemantics は本 package のレコードが持ちうる語彙の項目を返す。
//
// **対応表から導く。** 表に 1 行を足す作業で本関数を手で直さずに済む。
//
// **返した slice の変更は表に及ばない。**
func ItemSemantics() []core.SemanticKey {
	var semantics []core.SemanticKey
	for _, semantic := range systemSemantics {
		semantics = append(semantics, semantic)
	}
	for _, semantic := range sectionSemantics {
		semantics = append(semantics, semantic)
	}
	for _, table := range eventDataSemanticTables {
		for _, semantic := range table {
			semantics = append(semantics, semantic)
		}
	}
	for _, positional := range unnamedDataSemantics {
		semantics = append(semantics, positional...)
	}
	// map の走査の順は決まらないため、返す並びを文字列で 1 つに定める。
	slices.Sort(semantics)
	return slices.Compact(semantics)
}

// Observation は 1 件の Event を取り込みの項目へ直した結果である。
type Observation struct {
	// Fields は Event の全項目である。原文に現れた要素と属性を、空の文字列のものも含めて並べ、
	// 原文に無い System の項目は並べない。
	Fields []core.RecordField
	// ObservationKind はプロバイダの名前とイベント ID の組である。
	ObservationKind core.ObservationKind
	// EventTime は TimeCreated@SystemTime の時刻である。要素が無い Event では nil である。
	EventTime *core.Timestamp
	// Terminal は Event を記録した端末の項目である。Computer を持たない Event では要素数 0 である。
	Terminal []core.RecordField
	// ProcessStart は、Event が新しいプロセスの起動を記録したかである (processStartEvents)。
	ProcessStart bool
	// ProcessEnd は、Event がプロセスの終了を記録したかである (processEndEvents)。
	ProcessEnd bool
	// RemoteSession は、Event が端末へ入った遠隔のセッションを記録したかである
	// (remoteSessionEvents)。
	RemoteSession bool
	// InboundConnection は、Event が端末への着信の接続の許可を記録したかである (内向きの 5156)。
	InboundConnection bool
	// InboundGroupAddress は、内向きの 5156 のうち、自分の端末の側のアドレスがマルチキャストか
	// ブロードキャストであるかである (groupAddressText)。相手の端末は宛先を 1 台に決めずに送って
	// おり、自分の端末へ入った接続を記録していない。
	InboundGroupAddress bool
	// OutboundConnection は、Event が端末から 1 つの相手へ始めた接続を記録したかである。外向きの
	// 5156、Initiated が true の Sysmon の 3、資格情報を指定したログオンの要求 (4648) であり、
	// 接続先がマルチキャストかブロードキャストの記録を除く (groupAddressText)。
	OutboundConnection bool
	// SystemStart は、Event が端末の起動を記録したかである (systemStartEvents)。
	SystemStart bool
	// AccountCreation は、Event が Target のアカウントの作成を記録したか (4720) である。
	AccountCreation bool
	// MessageUnrendered は Event.MessageUnrendered と同じである。
	MessageUnrendered bool
	// TerminalCandidates は、Computer を持たない Event の説明が記録した `<ホスト名>$` の形の
	// アカウント名を、重複を除いて現れた順に持つ。記録した端末の候補であり、端末に決めない。
	// 説明が指すのは別の端末のアカウントでもありうる。
	TerminalCandidates []string
}

// providerEvent はプロバイダの名前とイベント ID の組である。
type providerEvent struct {
	provider string
	eventID  string
}

// processStartEvents は、新しいプロセスの起動を記録するイベントである。起動の記録は
// コマンド行の Data を持たないことがある。コマンド行の記録を止めた端末の 4688 である。
var processStartEvents = map[providerEvent]struct{}{
	{providerSecurityAuditing, eventIDProcessCreation}: {},
	{providerSysmon, sysmonProcessCreate}:              {},
}

// processEndEvents は、プロセスの終了を記録するイベントである。
//
// 既知の制限: Sysmon のプロセスの終了を載せない, Sysmon のイベントはプロセスを GUID で指し、
// 番号の区間で区切る段階の対象にならない, GUID を持たない終了のイベントを区間に使うとき
var processEndEvents = map[providerEvent]struct{}{
	{providerSecurityAuditing, eventIDProcessTermination}: {},
}

// Observe は Event を取り込みの項目へ直す。TimeCreated@SystemTime を時刻として読めない
// Event は、normalize の段階の失敗を返す。
func Observe(event Event) (Observation, *core.ImportFailure) {
	var observation Observation
	if raw := event.System.SystemTime; raw != nil {
		timestamp, ok := eventTimeOf(*raw)
		if !ok {
			return Observation{}, failureAt(core.FailureStageNormalize, event.Source,
				"TimeCreated@SystemTime written as an ISO 8601 date and time or the Event Viewer's local date and time",
				"the value "+strconv.Quote(*raw)+" does not follow the form")
		}
		observation.EventTime = &timestamp
	}
	fields := newFieldList()
	for _, slot := range event.System.slots() {
		if *slot.value == nil {
			continue
		}
		if slot.name == nameSystemTime {
			fields.addTimestamp(slot.name, systemSemantics[slot.name], *observation.EventTime)
			continue
		}
		semantic := systemSemantics[slot.name]
		if slot.name == nameUserID && userIDNamesSubject(event) {
			semantic = core.SemanticKeySubjectAccountSid
		}
		fields.addText(slot.name, semantic, **slot.value)
	}
	sysmon := textOf(event.System.ProviderName) == providerSysmon
	endpoints := endpointSemanticsOf(event)
	unnamedSemantics := unnamedDataSemantics[providerEvent{
		provider: textOf(event.System.ProviderName), eventID: textOf(event.System.EventID),
	}]
	unnamed := 0
	for _, data := range event.EventData {
		name := unnamedDataFieldName
		if data.Name != "" {
			name = "EventData." + data.Name
		}
		semantic := eventDataSemanticOf(event.System, data.Name)
		if data.Name == "" {
			if unnamed < len(unnamedSemantics) {
				semantic = unnamedSemantics[unnamed]
			}
			unnamed++
		}
		if replaced, found := endpoints[data.Name]; found {
			semantic = replaced
		}
		if semantic == core.SemanticKeyConnectionDestinationServerName && !namesRemoteServer(data.Text) {
			semantic = ""
		}
		if sysmon {
			fields.addSysmonData(name, semantic, data.Text)
			continue
		}
		fields.addText(name, semantic, data.Text)
		if normalize, found := accountNameData[eventDataKey{
			textOf(event.System.ProviderName), textOf(event.System.EventID), data.Name,
		}]; found {
			last := len(fields.fields) - 1
			fields.fields[last] = normalize(fields.fields[last])
		}
	}
	for _, section := range event.Sections {
		semantic, found := sectionSemantics[section.Name]
		if !found {
			semantic = eventDataSemanticOf(event.System, section.Name)
		}
		fields.addText(section.Name, semantic, section.Text)
		if section.Name == nameMessage && event.System.Computer == nil {
			observation.TerminalCandidates = machineAccountsIn(section.Text, observation.TerminalCandidates)
		}
	}
	observation.MessageUnrendered = event.MessageUnrendered
	observation.Fields = fields.fields
	observation.ObservationKind = observationKindOf(event.System)
	kind := providerEvent{provider: textOf(event.System.ProviderName), eventID: textOf(event.System.EventID)}
	_, observation.ProcessStart = processStartEvents[kind]
	_, observation.ProcessEnd = processEndEvents[kind]
	_, observation.RemoteSession = remoteSessionEvents[kind]
	_, observation.SystemStart = systemStartEvents[kind]
	observation.AccountCreation = kind == providerEvent{providerSecurityAuditing, "4720"}
	observation.InboundConnection = kind == providerEvent{providerSecurityAuditing, eventIDConnectionPermitted} &&
		!slices.Contains(outboundDirections, eventDataText(event, "Direction"))
	// 自分の端末の側のアドレスは語彙を外すため (inboundConnectionSemantics)、ここで読む。
	observation.InboundGroupAddress = observation.InboundConnection &&
		groupAddressText(eventDataText(event, "SourceAddress"), eventDataText(event, "Protocol"))
	observation.OutboundConnection = outboundConnectionOf(event, kind)
	if computer := event.System.Computer; computer != nil {
		observation.Terminal = []core.RecordField{textField(nameComputer, core.SemanticKeyTerminalHostname, *computer)}
	}
	return observation, nil
}

// TranscriptIdentityItems は、同じ 1 つの事象を 2 回転記したレコードを見分ける欄の名前を
// 返す。
//
// Channel はイベントを書いたチャネルの名前、EventRecordID はそのチャネルの中でイベントを
// 指す番号であり、同じ端末で 2 つが同じレコードは同じ 1 件のイベントの転記である。
func TranscriptIdentityItems() []string {
	return []string{nameChannel, nameEventRecordID}
}

// observationKindOf はプロバイダの名前とイベント ID を、この順で観測の種別にする。
// イベント ID の意味はプロバイダのイベントの定義が決めるため、状態は determined である。
func observationKindOf(system System) core.ObservationKind {
	kind := core.ObservationKind{Raw: []core.RecordField{}}
	for _, item := range []struct {
		name  string
		value *string
	}{{nameProviderName, system.ProviderName}, {nameEventID, system.EventID}} {
		if item.value != nil {
			kind.Raw = append(kind.Raw, textField(item.name, "", *item.value))
		}
	}
	if len(kind.Raw) > 0 {
		kind.Status = core.ObservationKindStatusDetermined
	}
	return kind
}

// fieldList は 1 件の項目を、名前が重ならないように並べる。
type fieldList struct {
	fields []core.RecordField
	taken  map[string]int
}

func newFieldList() *fieldList {
	return &fieldList{taken: make(map[string]int)}
}

// uniqueName は 1 件の中で重複しない名前を返す。同じ名前の 2 回目からは出現の番号を付ける。
func (l *fieldList) uniqueName(name string) string {
	l.taken[name]++
	if occurrence := l.taken[name]; occurrence > 1 {
		return name + "#" + strconv.Itoa(occurrence)
	}
	return name
}

func (l *fieldList) addText(name string, semantic core.SemanticKey, text string) {
	l.fields = append(l.fields, textField(l.uniqueName(name), semantic, text))
}

func (l *fieldList) addTimestamp(name string, semantic core.SemanticKey, timestamp core.Timestamp) {
	// 名前は非空、semantic は語彙の項目、時刻は NewTimestamp が検査した値であるため失敗しない。
	field, _ := core.NewTimestampField(l.uniqueName(name), semantic, timestamp)
	l.fields = append(l.fields, field)
}

// absentDataText は、Security の監査のイベントが `<Data>` に値が無いことを書く文字列である。
const absentDataText = "-"

// dashMeansAbsent は、値が absentDataText のとき値の不在として扱う語彙の項目である。
//
// **アドレスと port とホスト名とアカウントと種別のコードの値として "-" を比べない。** 比べると、
// 接続元を持たないログオンがすべて "-" という 1 つの IP アドレスのノードへ集まり、種別の
// コードを持たないログオンが "-" という種別の区分を作る。
var dashMeansAbsent = map[core.SemanticKey]struct{}{
	core.SemanticKeyConnectionSourceAddress:     {},
	core.SemanticKeyConnectionSourcePort:        {},
	core.SemanticKeyConnectionDestinationPort:   {},
	core.SemanticKeyRemoteSessionClientHostname: {},
	core.SemanticKeyEventOutboundAccountName:    {},
	core.SemanticKeyEventOutboundAccountDomain:  {},
	core.SemanticKeyEventAuthenticationPackage:  {},
	core.SemanticKeySubjectAccountSid:           {},
	core.SemanticKeySubjectAccountName:          {},
	core.SemanticKeySubjectAccountDomain:        {},
	core.SemanticKeyTargetAccountSid:            {},
	core.SemanticKeyTargetAccountName:           {},
	core.SemanticKeyTargetAccountDomain:         {},
	core.SemanticKeyTargetGroupSid:              {},
	core.SemanticKeyTargetGroupName:             {},
	core.SemanticKeyTargetGroupDomain:           {},
	core.SemanticKeyEventLogonType:              {},
	core.SemanticKeyEventTargetLogonId:          {},
	core.SemanticKeyEventSubjectLogonId:         {},
	core.SemanticKeyEventTargetLinkedLogonId:    {},
	core.SemanticKeyEventLogoffLogonId:          {},
	core.SemanticKeyEventTicketLogonGuid:        {},
	core.SemanticKeyEventTargetLogonGuid:        {},
}

// logonIdSemantics は、値が Logon ID である語彙の項目である。
var logonIdSemantics = map[core.SemanticKey]struct{}{
	core.SemanticKeyEventTargetLogonId:       {},
	core.SemanticKeyEventSubjectLogonId:      {},
	core.SemanticKeyEventTargetLinkedLogonId: {},
	core.SemanticKeyEventLogoffLogonId:       {},
}

// hexProcessIdDerivation は 16 進のプロセス番号を 10 進へ直した導き方である。
const hexProcessIdDerivation = "hexadecimal process id with the 0x prefix written in decimal"

// leadingZerosDerivation は 10 進の文字列から先頭の 0 を外した導き方である。
const leadingZerosDerivation = "decimal digits written without leading zeros"

// ipv4MappedDerivation は、IPv6 の形で書いた IPv4 のアドレス (`::ffff:a.b.c.d`) をドット
// 10 進へ直した導き方である。
const ipv4MappedDerivation = "IPv4-mapped IPv6 address written as dotted decimal IPv4"

// addressWithPortDerivation は、port を付けたアドレス (`a.b.c.d:p`、`[a.b.c.d]:p`) から
// アドレスだけを取り出した導き方である。
const addressWithPortDerivation = "address without the port"

// textField は 1 つの値を項目にする。
//
// IP アドレスの項目の値が IPv6 の形で書いた IPv4 であるときは、ドット 10 進の文字列を正規化値に
// 置く。語彙の IPv4 はドット 10 進で比べる。port を付けたアドレスは、アドレスだけを正規化値に
// 置く。IP アドレスとして読めない値は、語彙を外した項目にする。起動したタスクの名前が
// taskNamePrefix で始まるときは、接頭辞を外した名前を正規化値に置く。
// プロセス番号の項目の値が `0x` で始まる 16 進であるときは、10 進の文字列を正規化値に置く。
// 語彙のプロセス番号は 10 進で比べる。イベント ID とレコード番号の値が先頭に 0 を置く 10 進で
// あるときは、0 を外した文字列を正規化値に置く。Logon ID の項目は core.LogonIdValue が比べる形を
// 置く。値の不在を表す文字列は absent の値にする (dashMeansAbsent)。
func textField(name string, semantic core.SemanticKey, text string) core.RecordField {
	state := core.ValueStatePresent
	if _, marksAbsence := dashMeansAbsent[semantic]; marksAbsence && text == absentDataText {
		state = core.ValueStateAbsent
	}
	value, _ := core.NewRawValue(state, text)
	if _, logonId := logonIdSemantics[semantic]; logonId && state == core.ValueStatePresent {
		value = core.LogonIdValue(text, text)
	}
	switch semantic {
	case core.SemanticKeyProcessPid, core.SemanticKeyParentProcessPid:
		if decimal, ok := decimalOfHex(text); ok {
			value, _ = core.NewNormalizedValue(core.ValueStatePresent, text, decimal, hexProcessIdDerivation)
		}
	case core.SemanticKeyWindowsEventId, core.SemanticKeyWindowsEventRecordId:
		if decimal, ok := decimalWithoutLeadingZeros(text); ok {
			value, _ = core.NewNormalizedValue(core.ValueStatePresent, text, decimal, leadingZerosDerivation)
		}
	case core.SemanticKeyStartedTaskName:
		if name, found := strings.CutPrefix(text, taskNamePrefix); found && strings.HasPrefix(name, `\`) {
			value, _ = core.NewNormalizedValue(core.ValueStatePresent, text, name, taskNamePrefixDerivation)
		}
	}
	if semantic.Object() == core.SemanticObjectIp && state == core.ValueStatePresent {
		switch address, err := netip.ParseAddr(text); {
		case err == nil && address.Is4In6():
			value, _ = core.NewNormalizedValue(core.ValueStatePresent, text, address.Unmap().String(), ipv4MappedDerivation)
		case err != nil:
			// netip.ParseAddrPort は `[a.b.c.d]:p` を読まないため、host と port を先に分ける。
			host, _, err := net.SplitHostPort(text)
			withoutPort, parseErr := netip.ParseAddr(host)
			if err == nil && parseErr == nil {
				value, _ = core.NewNormalizedValue(core.ValueStatePresent, text,
					withoutPort.Unmap().String(), addressWithPortDerivation)
			} else {
				// IP アドレスとして読めない文字列は、アドレスのノードを作らないよう語彙を外す。
				semantic = ""
			}
		}
	}
	// 名前は非空、値は原資料の文字列を持つ present か absent、semantic は語彙の項目か空であるため
	// 検査が失敗しない。
	field, _ := core.NewTextField(name, semantic, value)
	return field
}

// decimalOfHex は `0x` で始まる 16 進の文字列を 10 進の文字列へ直す。
func decimalOfHex(text string) (string, bool) {
	digits, found := strings.CutPrefix(text, "0x")
	if !found || digits == "" {
		return "", false
	}
	number, err := strconv.ParseUint(digits, 16, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(number, 10), true
}

// decimalWithoutLeadingZeros は、先頭に 0 を置く 10 進の文字列から 0 を外した文字列を返す。
// ok が偽になるのは、数字だけでない文字列と、既に先頭に 0 を置かない文字列である。
func decimalWithoutLeadingZeros(text string) (string, bool) {
	if len(text) < 2 || text[0] != '0' || strings.Trim(text, "0123456789") != "" {
		return "", false
	}
	if trimmed := strings.TrimLeft(text, "0"); trimmed != "" {
		return trimmed, true
	}
	return "0", true
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
