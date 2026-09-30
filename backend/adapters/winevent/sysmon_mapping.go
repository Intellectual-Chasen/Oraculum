package winevent

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// providerSysmon は Sysmon の Operational チャネルのイベントを書くプロバイダの名前である。
const providerSysmon = "Microsoft-Windows-Sysmon"

// Sysmon のイベント ID。値は Sysmon のイベントの定義が決める。
const (
	sysmonProcessCreate              = "1"
	sysmonFileCreateTime             = "2"
	sysmonNetworkConnect             = "3"
	sysmonFileCreate                 = "11"
	sysmonRegistryObjectCreateDelete = "12"
	sysmonRegistryValueSet           = "13"
	sysmonRegistryObjectRename       = "14"
)

// sysmonOperationData は、プロセスが行った操作を記録するイベントに共通の `<Data>` の対応で
// ある。ProcessGuid と ProcessId と Image は操作を行ったプロセスを指す。
//
// **UtcTime は Sysmon が操作を捉えた時刻であり、TimeCreated@SystemTime はイベントログが
// イベントを記録した時刻である。** 2 つを別の項目に置き、差を補わない。
var sysmonOperationData = map[string]core.SemanticKey{
	"UtcTime":     core.SemanticKeyEventOperationStartTime,
	"ProcessGuid": core.SemanticKeyProcessId,
	"ProcessId":   core.SemanticKeyProcessPid,
	"Image":       core.SemanticKeyProcessBinaryPath,
}

// sysmonEventData は Sysmon のイベント ID ごとの `<Data>` の対応である。1 のほかのイベントは
// sysmonOperationData の対応も持つ。
//
// 1 はプロセスの生成を記録する。ProcessGuid が生成されたプロセス、ParentProcessGuid が
// 生成したプロセスを指し、UtcTime は生成の時刻である。**親と子のプロセスを結ぶ鍵は GUID である。**
// ProcessId と ParentProcessId は OS が再利用する番号であり、プロセスの属性として保つ。
// GUID は Sysmon が 1 つの端末の中で書く文字列のまま比べ、中括弧と大文字小文字をそろえない。
// 同じ端末の ProcessGuid と ParentProcessGuid は同じ書き方をする。
//
// 3 は通信を記録する。Initiated が true のイベントでは、Source が自分の端末、Destination が
// 相手である。**それ以外のイベントでは Source と Destination に語彙の項目を与えない**
// (endpointSemanticsOf)。着信では Destination が自分の端末であり、接続先への関係を
// 張ると、プロセスが自分の端末のアドレスと通信したことになる。
//
// 既知の制限: 着信の通信で、プロセスと相手 (Source) の間に関係を張らない,
// 語彙に接続元から着信したプロセスへ向ける関係の種別が無く、足すと関係の意味が変わるため、
// 測る対象が無い,
// 着信を受けたプロセスと接続元を結ぶ関係の種別を語彙に置いたとき、Source をその関係へ写す
//
// **DestinationHostname は connection.destination_reverse_lookup_name へ写す。** Sysmon は
// 接続先の IP アドレスを逆引きして DestinationHostname を書き、プロセスが要求で指定した
// ホスト名とは出どころが違う。
//
// 12〜14 はレジストリの操作を記録する。EventType が操作の種別、Details が 13 の書いた値である。
//
// 既知の制限: TargetObject をキーと値の名前に分けず、registry_value.key_path へ写す,
// 13 の TargetObject は値の名前までを含む path であり、値の名前は `\` を含みうるため、末尾の
// 区切りで分けると別の値を指す。どこで分けるかを文字列から決められず、測る対象が無い,
// Sysmon がキーと値の名前を別の項目で書くようになったとき、名前を registry_value.name へ写す
//
// 既知の制限: User、Hashes、ParentCommandLine に語彙の項目を与えない,
// User は `ドメイン\名前` を 1 つの文字列に、Hashes は複数の算法の hash を 1 つの文字列に書き、
// 語彙の項目は 1 つの値を 1 つの文字列で比べる。分けた値の比べられる形を決めておらず、
// 測る対象が無い,
// 1 つの文字列を複数の項目へ分けて持つ形を決めたとき、表に足す
var sysmonEventData = map[string]map[string]core.SemanticKey{
	sysmonProcessCreate: {
		"UtcTime":           core.SemanticKeyProcessStartTime,
		"ProcessGuid":       core.SemanticKeyProcessId,
		"ProcessId":         core.SemanticKeyProcessPid,
		"Image":             core.SemanticKeyProcessBinaryPath,
		"CommandLine":       core.SemanticKeyProcessCommandLine,
		"FileVersion":       core.SemanticKeyProcessBinaryFileVersion,
		"Description":       core.SemanticKeyProcessBinaryDescription,
		"Product":           core.SemanticKeyProcessBinaryProduct,
		"Company":           core.SemanticKeyProcessBinaryCompany,
		"TerminalSessionId": core.SemanticKeyProcessSessionId,
		"ParentProcessGuid": core.SemanticKeyParentProcessId,
		"ParentProcessId":   core.SemanticKeyParentProcessPid,
		"ParentImage":       core.SemanticKeyParentProcessBinaryPath,
	},
	sysmonFileCreateTime: {
		"TargetFilename":  core.SemanticKeyFilePath,
		"CreationUtcTime": core.SemanticKeyFileCreatedTime,
	},
	sysmonNetworkConnect: {
		"Protocol":            core.SemanticKeyConnectionProtocol,
		"SourceIp":            core.SemanticKeyConnectionSourceAddress,
		"SourcePort":          core.SemanticKeyConnectionSourcePort,
		"DestinationIp":       core.SemanticKeyConnectionDestinationAddress,
		"DestinationPort":     core.SemanticKeyConnectionDestinationPort,
		"DestinationHostname": core.SemanticKeyConnectionDestinationReverseLookupName,
	},
	sysmonFileCreate: {
		"TargetFilename":  core.SemanticKeyFilePath,
		"CreationUtcTime": core.SemanticKeyFileCreatedTime,
	},
	sysmonRegistryObjectCreateDelete: {
		"EventType":    core.SemanticKeyEventAction,
		"TargetObject": core.SemanticKeyRegistryValueKeyPath,
	},
	sysmonRegistryValueSet: {
		"EventType":    core.SemanticKeyEventAction,
		"TargetObject": core.SemanticKeyRegistryValueKeyPath,
		"Details":      core.SemanticKeyRegistryValueData,
	},
	sysmonRegistryObjectRename: {
		"EventType":    core.SemanticKeyEventAction,
		"TargetObject": core.SemanticKeyRegistryValueKeyPath,
	},
}

// ContentReplacementKinds は、ファイルまたはレジストリの値の内容を置き換える記録の観測の種別を
// 返す。
//
// Sysmon のイベントの定義が次のように定める。
//   - 11 (FileCreate) はファイルの作成と上書きを記録する。
//   - 13 (RegistryEvent Value Set) は値の設定を記録する。値の設定は値の data と種類の全体を
//     置き換える (Windows の RegSetValueEx)。
//
// 12 は 1 つのイベント ID でキーと値の作成と削除を持ち、観測の種別で作成と削除を分けられない
// ため含めない。
func ContentReplacementKinds() []core.ContentReplacementSelector {
	selectors := make([]core.ContentReplacementSelector, 0, 2)
	for _, eventID := range []string{sysmonFileCreate, sysmonRegistryValueSet} {
		selectors = append(selectors, core.ContentReplacementSelector{Kind: core.ObservationKindSelector{
			Items: []core.ObservationKindSelectorItem{
				{Name: nameProviderName, Value: providerSysmon},
				{Name: nameEventID, Value: eventID},
			},
		}})
	}
	return selectors
}

// FlowOperationKinds は、影響のエッジの向きを決める操作の分類を、プロバイダとイベント ID ごとに返す。
//
// Sysmon のイベントの定義が次のように定める。
//   - 2 (ファイルの作成時刻の変更)、11 (FileCreate)、12 (キーと値の作成と削除)、13 (値の設定) は
//     プロセスが対象を変えた記録であり、書き込みである。
//   - 3 (NetworkConnect) は送信と受信を分けない。Initiated は接続を張った側を表し、情報の向きを
//     表さない。
//
// 14 (キーと値の名前の変更) は新しい名前の欄を語彙に写さないため含めない。
//
// Security の監査の 4720 (アカウントの作成)、4726 (削除)、4728・4732・4756 (グループへの追加) は、
// アカウントの管理操作である。対象のアカウントは Target または Member の欄が指す。4624 (ログオンの
// 成功) は、Target の欄が指すアカウントの資格情報を使ったログオンである。
func FlowOperationKinds() []core.FlowOperationSelector {
	operations := []struct {
		provider, eventID string
		operation         core.FlowOperation
	}{
		{providerSysmon, sysmonFileCreateTime, core.FlowOperationWrite},
		{providerSysmon, sysmonFileCreate, core.FlowOperationWrite},
		{providerSysmon, sysmonRegistryObjectCreateDelete, core.FlowOperationWrite},
		{providerSysmon, sysmonRegistryValueSet, core.FlowOperationWrite},
		{providerSysmon, sysmonNetworkConnect, core.FlowOperationCommunication},
		{providerSecurityAuditing, "4720", core.FlowOperationAccountManagement},
		{providerSecurityAuditing, "4726", core.FlowOperationAccountManagement},
		{providerSecurityAuditing, groupMemberAddedEventIDs[0], core.FlowOperationAccountManagement},
		{providerSecurityAuditing, groupMemberAddedEventIDs[1], core.FlowOperationAccountManagement},
		{providerSecurityAuditing, groupMemberAddedEventIDs[2], core.FlowOperationAccountManagement},
		{providerSecurityAuditing, eventIDLogonSuccess, core.FlowOperationLogon},
	}
	selectors := make([]core.FlowOperationSelector, 0, len(operations))
	for _, item := range operations {
		selectors = append(selectors, core.FlowOperationSelector{
			Kind: core.ObservationKindSelector{Items: []core.ObservationKindSelectorItem{
				{Name: nameProviderName, Value: item.provider},
				{Name: nameEventID, Value: item.eventID},
			}},
			Operation: item.operation,
		})
	}
	return selectors
}

// sysmonUnlinkedEndpointSemantics は、3 の通信のうち接続元と接続先を持つ `<Data>` の名前で
// あり、自分の端末から始めていない通信で語彙を外す (空の項目)。
var sysmonUnlinkedEndpointSemantics = map[string]core.SemanticKey{
	"SourceIp": "", "SourcePort": "", "DestinationIp": "", "DestinationPort": "", "DestinationHostname": "",
}

// sysmonInitiated は、通信のイベントが自分の端末から始めた通信を記録したかを返す。
// Initiated が無いイベントと、`true` のほかの文字列を持つイベントでは偽である。
func sysmonInitiated(event Event) bool {
	return eventDataText(event, "Initiated") == "true"
}

// eventDataText は、名前が name の最初の `<Data>` の文字列を返す。無ければ空の文字列である。
func eventDataText(event Event, name string) string {
	for _, data := range event.EventData {
		if data.Name == name {
			return data.Text
		}
	}
	return ""
}

// sysmonEventDataSemantics は Sysmon のイベントの `<Data>` の対応である。
var sysmonEventDataSemantics = sysmonEventDataTable()

func sysmonEventDataTable() map[eventDataKey]core.SemanticKey {
	table := make(map[eventDataKey]core.SemanticKey)
	for eventID, data := range sysmonEventData {
		if eventID != sysmonProcessCreate {
			for name, semantic := range sysmonOperationData {
				table[eventDataKey{providerSysmon, eventID, name}] = semantic
			}
		}
		for name, semantic := range data {
			table[eventDataKey{providerSysmon, eventID, name}] = semantic
		}
	}
	return table
}

// timeMeaning は時刻の項目が何の時刻で、どの時計が刻んだかである。
type timeMeaning struct {
	meaning core.Meaning
	clock   core.Clock
}

// sysmonTimeMeanings は Sysmon が時刻を書く語彙の項目である。UtcTime はプロセスが動いた端末の
// 時計、CreationUtcTime はファイルの属性が持つ時刻である。
var sysmonTimeMeanings = map[core.SemanticKey]timeMeaning{
	core.SemanticKeyProcessStartTime:        {core.MeaningOperationStart, core.ClockTerminalLocal},
	core.SemanticKeyEventOperationStartTime: {core.MeaningOperationStart, core.ClockTerminalLocal},
	core.SemanticKeyFileCreatedTime:         {core.MeaningProperty, core.ClockFileProperty},
}

// sysmonNullGuid は、Sysmon が識別子を持たないプロセスの GUID の欄に書く文字列である。
const sysmonNullGuid = "{00000000-0000-0000-0000-000000000000}"

// addSysmonData は Sysmon の `<Data>` 1 つを項目にする。
//
// 時刻の項目は、文字列が UTC からのずれを書かないまま UTC を表す (Sysmon のイベントの定義)。
// 書式に合わない時刻には語彙の項目を与えず、原資料の文字列を保つ。
//
// **値を持たない文字列から対象のノードを作らない。** Sysmon は値の無い欄に空の文字列または `-` を、
// 親を持たないプロセスの ParentProcessGuid に全桁 0 の GUID を書く。ノードの識別に使う項目と
// 親の識別子と逆引きの名前では、3 つの文字列を値の不在として保つ。値が同じ文字列どうしで、別の
// プロセスが 1 つの親や 1 つの名前を持つことになるためである。
func (l *fieldList) addSysmonData(name string, semantic core.SemanticKey, text string) {
	if meaning, isTime := sysmonTimeMeanings[semantic]; isTime {
		if timestamp, ok := sysmonUtcTimeOf(text, meaning); ok {
			l.addTimestamp(name, semantic, timestamp)
			return
		}
		semantic = ""
	}
	identifying := semantic.Role() == core.SemanticRoleIdentity || semantic == core.SemanticKeyParentProcessId ||
		semantic == core.SemanticKeyConnectionDestinationReverseLookupName
	if identifying && (text == "" || text == "-" || text == sysmonNullGuid) {
		// 名前は非空、値は原資料の文字列を持つ absent、semantic は語彙の項目であるため検査が失敗しない。
		value, _ := core.NewRawValue(core.ValueStateAbsent, text)
		field, _ := core.NewTextField(l.uniqueName(name), semantic, value)
		l.fields = append(l.fields, field)
		return
	}
	l.addText(name, semantic, text)
}

// sysmonUtcTimeOf は UTC からのずれを書かない UTC の文字列を時刻へ直す。ok が偽になるのは文字列を
// 日時として読めないときと、文字列がずれを書くときである。
func sysmonUtcTimeOf(raw string, meaning timeMeaning) (core.Timestamp, bool) {
	if _, offset := splitOffset(raw); offset != "" {
		return core.Timestamp{}, false
	}
	timestamp, ok := eventTimeOf(raw + "Z")
	if !ok {
		return core.Timestamp{}, false
	}
	timestamp.RawText, timestamp.OffsetText = &raw, nil
	timestamp.OffsetState = core.OffsetStateFormatDefined
	timestamp.Meaning, timestamp.Clock = meaning.meaning, meaning.clock
	built, err := core.NewTimestamp(timestamp)
	return built, err == nil
}
