package winevent

import (
	"regexp"
	"slices"
	"strings"
)

// viewerItemKey はプロバイダの名前、イベント ID、説明の見出し、説明の項目名の組である。
type viewerItemKey struct {
	provider string
	eventID  string
	section  string
	label    string
}

// viewerItemNames は、イベントビューアーが日本語で組んだ説明の項目名を、`<EventData>` の
// `<Data>` の Name へ写す表である。値は Windows がイベントの定義に書いた項目名である。
// 写した項目は XML の Event と同じ意味の対応 (eventDataSemanticTables) を受ける。
//
// 4688 の作成を要求したアカウントの見出しは、Windows のバージョンにより「サブジェクト」と
// 「作成元サブジェクト」がある。親のプロセス番号の項目名は「クリエーター プロセス ID」と
// 「クリエータ プロセス ID」と「作成元プロセス ID」がある。5140 の「共有名」は、Windows のバージョンにより
// 見出しを持たない項目と、「共有情報」の見出しの下の項目がある。
//
// ログオンのイベントの項目は viewerLogonItemNames が足す。Subject のアカウントの項目は
// withViewerLogonItemNames が足す。
//
// 4728・4732・4756 のメンバの「アカウント名」は識別名 (DN) であり、XML と同じ MemberName へ写す。
// メンバの「セキュリティ ID」と 4769 の「サービス ID」は accountIdValues が値にする。
//
// 既知の制限: 4726 の項目名を載せない, 4726 の見出しを持つ CSV が repo の中に無く測れない,
// 4726 を含む CSV を取り込む要求が出たとき、見出しを確かめて足す
//
// 既知の制限: 4624 の TargetLinkedLogonId と LogonGuid、4769 の LogonGuid、4648 の TargetLogonGuid の
// 項目名を載せない。CSV のレコードは分けたログオンの対応とチケットの要求の関係に入らない,
// これらの見出しを持つ CSV が repo の中に無く測れない, これらの欄を含む CSV を取り込む要求が出たとき、
// 見出しを確かめて足す
//
// 「セキュリティ ID」の項目を SubjectUserSid と TargetUserSid へ写さない。イベントビューアーは
// SID を「SYSTEM」や「<ドメイン>\<名前>」の文字列へ直して書き出すため、値が SID の形を持たない。
// SID の欄を持たないレコードのアカウントは、「アカウント ドメイン」と「アカウント名」の組の
// ノードになる (core の roleAccountsOf)。
//
// 既知の制限: 名前へ直せなかった SID の形の「セキュリティ ID」を写さない, SID の形のまま書き出す
// CSV が repo の中に無く測れない, SID の形のまま書き出す CSV を取り込む要求が出たとき、SID の形の値だけを写す
//
// 既知の制限: 語彙の対応を持たない項目名を写さない,
// 写しても意味の対応が変わらず、測る対象が無い,
// eventDataSemanticTables に表を足すとき、同じイベントの項目名を本表に足す
var viewerItemNames = withViewerLogonItemNames(map[viewerItemKey]string{
	{providerSecurityAuditing, eventIDProcessCreation, "サブジェクト", "アカウント名"}:         "SubjectUserName",
	{providerSecurityAuditing, eventIDProcessCreation, "サブジェクト", "アカウント ドメイン"}:     "SubjectDomainName",
	{providerSecurityAuditing, eventIDProcessCreation, "作成元サブジェクト", "アカウント名"}:      "SubjectUserName",
	{providerSecurityAuditing, eventIDProcessCreation, "作成元サブジェクト", "アカウント ドメイン"}:  "SubjectDomainName",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "新しいプロセス ID"}:     "NewProcessId",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "新しいプロセス名"}:       "NewProcessName",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "クリエーター プロセス ID"}: "ProcessId",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "クリエータ プロセス ID"}:  "ProcessId",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "作成元プロセス ID"}:     "ProcessId",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "作成元プロセス名"}:       "ParentProcessName",
	{providerSecurityAuditing, eventIDProcessCreation, "プロセス情報", "プロセスのコマンド ライン"}:  "CommandLine",
	{providerSecurityAuditing, eventIDProcessTermination, "プロセス情報", "プロセス ID"}:     "ProcessId",
	{providerSecurityAuditing, eventIDProcessTermination, "プロセス情報", "プロセス名"}:       "ProcessName",

	{providerSecurityAuditing, eventIDConnectionPermitted, "アプリケーション情報", "プロセス ID"}:   "ProcessID",
	{providerSecurityAuditing, eventIDConnectionPermitted, "アプリケーション情報", "アプリケーション名"}: "Application",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "方向"}:          "Direction",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "送信元アドレス"}:     "SourceAddress",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "ソース ポート"}:     "SourcePort",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "宛先アドレス"}:      "DestAddress",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "宛先ポート"}:       "DestPort",
	{providerSecurityAuditing, eventIDConnectionPermitted, "ネットワーク情報", "プロトコル"}:       "Protocol",

	{providerSecurityAuditing, "5140", "ネットワーク情報", "送信元アドレス"}: "IpAddress",
	{providerSecurityAuditing, "5140", "ネットワーク情報", "ソース ポート"}: "IpPort",
	{providerSecurityAuditing, "5140", "", "共有名"}:             "ShareName",
	{providerSecurityAuditing, "5140", "共有情報", "共有名"}:         "ShareName",

	{providerSecurityAuditing, "4768", "アカウント情報", "アカウント名"}:       "TargetUserName",
	{providerSecurityAuditing, "4768", "アカウント情報", "提供された領域名"}:     "TargetDomainName",
	{providerSecurityAuditing, "4768", "ネットワーク情報", "クライアント アドレス"}: "IpAddress",
	{providerSecurityAuditing, "4768", "ネットワーク情報", "クライアント ポート"}:  "IpPort",

	{providerSecurityAuditing, "4769", "アカウント情報", "アカウント名"}:       "TargetUserName",
	{providerSecurityAuditing, "4769", "アカウント情報", "アカウント ドメイン"}:   "TargetDomainName",
	{providerSecurityAuditing, "4769", "サービス情報", "サービス名"}:         "ServiceName",
	{providerSecurityAuditing, "4769", "ネットワーク情報", "クライアント アドレス"}: "IpAddress",
	{providerSecurityAuditing, "4769", "ネットワーク情報", "クライアント ポート"}:  "IpPort",

	{providerSecurityAuditing, "4698", "タスク情報", "タスク名"}: "TaskName",

	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "資格情報が使用されたアカウント", "アカウント名"}:     "TargetUserName",
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "資格情報が使用されたアカウント", "アカウント ドメイン"}: "TargetDomainName",
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "ターゲット サーバー", "ターゲット サーバー名"}:     "TargetServerName",
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "ネットワーク情報", "ネットワーク アドレス"}:       "IpAddress",
	{providerSecurityAuditing, eventIDExplicitCredentialLogon, "ネットワーク情報", "ポート"}:               "IpPort",

	{providerSecurityAuditing, "4720", "新しいアカウント", "アカウント名"}:     "TargetUserName",
	{providerSecurityAuditing, "4720", "新しいアカウント", "アカウント ドメイン"}: "TargetDomainName",

	{providerSecurityAuditing, "4728", "グループ", "グループ名"}:     "TargetUserName",
	{providerSecurityAuditing, "4728", "グループ", "グループ ドメイン"}: "TargetDomainName",
	{providerSecurityAuditing, "4728", "メンバ", "アカウント名"}:     "MemberName",
	{providerSecurityAuditing, "4732", "グループ", "グループ名"}:     "TargetUserName",
	{providerSecurityAuditing, "4732", "グループ", "グループ ドメイン"}: "TargetDomainName",
	{providerSecurityAuditing, "4732", "メンバ", "アカウント名"}:     "MemberName",
	{providerSecurityAuditing, "4756", "グループ", "グループ名"}:     "TargetUserName",
	{providerSecurityAuditing, "4756", "グループ", "グループ ドメイン"}: "TargetDomainName",
	{providerSecurityAuditing, "4756", "メンバ", "アカウント名"}:     "MemberName",
})

// accountIdValues は、アカウントの SID を書く項目 (グループの変更のメンバーの「セキュリティ ID」、
// 4769 の「サービス ID」) を値にする。SID の形の文字列は `<prefix>Sid`、`<ドメイン>\<名前>` の文字列は
// `<prefix>DomainName` と `<prefix>UserName` の 2 つの値にする。どちらでもない文字列は項目名の値の
// まま返す。
//
// イベントビューアーは、名前へ直せた SID を `<ドメイン>\<名前>` の文字列で書き出す。
func accountIdValues(prefix, name, value string) []Value {
	if strings.HasPrefix(value, "S-1-") {
		return []Value{{Name: prefix + "Sid", Text: value}}
	}
	if domain, user, found := strings.Cut(value, `\`); found && domain != "" && user != "" {
		return []Value{{Name: prefix + "DomainName", Text: domain}, {Name: prefix + "UserName", Text: user}}
	}
	return []Value{{Name: name, Text: value}}
}

// viewerSection は説明の見出しと項目名の組である。見出しを持たない項目の見出しは空の文字列である。
type viewerSection struct {
	section string
	label   string
}

// viewerLogonItemNames は、ログオンのイベントの説明の項目名と、`<Data>` の Name の対応である。
//
// ログオンの種別は見出しを持たない項目である。成功のログオンの種別は、Windows のバージョンにより「ログオン情報」の
// 見出しの下に置く。ログオンしたアカウントの見出しは、成功が
// 「新しいログオン」、失敗が Windows のバージョンにより「ログオンを失敗したアカウント」と「ログオンに失敗した
// アカウント」である。ログオフの説明は、終わったセッションを「サブジェクト」の見出しに書き、
// XML の Target の値に相当する。
var viewerLogonItemNames = map[string]map[viewerSection]string{
	eventIDLogonSuccess: {
		{"", "ログオン タイプ"}:                 "LogonType",
		{"ログオン情報", "ログオン タイプ"}:           "LogonType",
		{"新しいログオン", "アカウント名"}:            "TargetUserName",
		{"新しいログオン", "アカウント ドメイン"}:        "TargetDomainName",
		{"新しいログオン", "ログオン ID"}:           "TargetLogonId",
		{"新しいログオン", "ネットワーク アカウント名"}:     "TargetOutboundUserName",
		{"新しいログオン", "ネットワーク アカウント ドメイン"}: "TargetOutboundDomainName",
		{"ネットワーク情報", "ワークステーション名"}:       "WorkstationName",
		{"ネットワーク情報", "ソース ネットワーク アドレス"}:  "IpAddress",
		{"ネットワーク情報", "ソース ポート"}:          "IpPort",
		{"詳細な認証情報", "認証パッケージ"}:           "AuthenticationPackageName",
	},
	eventIDLogonFailure: {
		{"", "ログオン タイプ"}:                 "LogonType",
		{"ログオンを失敗したアカウント", "アカウント名"}:     "TargetUserName",
		{"ログオンを失敗したアカウント", "アカウント ドメイン"}: "TargetDomainName",
		{"ログオンに失敗したアカウント", "アカウント名"}:     "TargetUserName",
		{"ログオンに失敗したアカウント", "アカウント ドメイン"}: "TargetDomainName",
		{"エラー情報", "状態"}:                  "Status",
		{"エラー情報", "サブ ステータス"}:            "SubStatus",
		{"ネットワーク情報", "ワークステーション名"}:       "WorkstationName",
		{"ネットワーク情報", "ソース ネットワーク アドレス"}:  "IpAddress",
		{"ネットワーク情報", "ソース ポート"}:          "IpPort",
		{"詳細な認証情報", "認証パッケージ"}:           "AuthenticationPackageName",
	},
	eventIDLogoff: {
		{"", "ログオン タイプ"}:         "LogonType",
		{"サブジェクト", "アカウント名"}:     "TargetUserName",
		{"サブジェクト", "アカウント ドメイン"}: "TargetDomainName",
		{"サブジェクト", "ログオン ID"}:    "TargetLogonId",
	},
	eventIDUserInitiatedLogoff: {
		{"サブジェクト", "ログオン ID"}: "TargetLogonId",
	},
}

// withViewerLogonItemNames は、表にログオンのイベントの項目名と、操作を行ったセッションの
// Logon ID の項目名を足して返す。
//
// 操作を行ったセッションは「サブジェクト」の見出しの「ログオン ID」である。4688 は Windows のバージョンにより
// 見出しが「作成元サブジェクト」である。
func withViewerLogonItemNames(table map[viewerItemKey]string) map[viewerItemKey]string {
	for eventID, items := range viewerLogonItemNames {
		for item, name := range items {
			table[viewerItemKey{providerSecurityAuditing, eventID, item.section, item.label}] = name
		}
	}
	for _, eventID := range securitySubjectLogonEventIDs {
		table[viewerItemKey{providerSecurityAuditing, eventID, "サブジェクト", "ログオン ID"}] = "SubjectLogonId"
	}
	for _, eventID := range subjectAccountEventIDs {
		table[viewerItemKey{providerSecurityAuditing, eventID, "サブジェクト", "アカウント名"}] = "SubjectUserName"
		table[viewerItemKey{providerSecurityAuditing, eventID, "サブジェクト", "アカウント ドメイン"}] = "SubjectDomainName"
	}
	table[viewerItemKey{providerSecurityAuditing, eventIDProcessCreation, "作成元サブジェクト", "ログオン ID"}] =
		"SubjectLogonId"
	return table
}

// insertionStringsMarker は、説明を組めなかったイベントの説明で、イベントが持つ値の前に
// Windows が置く行の文字列である。
const insertionStringsMarker = "イベントには次の情報が含まれています:"

// unrenderedPrefix は、書き出した端末がイベントの定義を持たないときに Windows が説明の
// 先頭に置く文字列である。
func unrenderedPrefix(provider, eventID string) string {
	return `ソース "` + provider + `" からのイベント ID ` + eventID + " の説明が見つかりません。"
}

// descriptionValues は説明から `名前: 値` の形の行を取り出す。
//
// 行頭に空白を持たず値が空の行 (`サブジェクト:`) のうち、次の行が字下げを持つ行は見出しで
// あり、空行まで続く字下げした項目の名前を `見出し.項目名` にする。次の行が字下げを持たない
// 行 (`RuleName: `) は値の空の項目である。字下げしない項目は項目名だけを名前にする。
// viewerItemNames が持つ組は、`<Data>` の Name を名前にする。
//
// 値を持つ項目の直後に続く、字下げした `名前:` の形でない行は、同じ項目の続きである。
// listItemNames の項目 (4672 の特権) は、イベントビューアーが 1 行に 1 つずつ字下げして書く
// 一覧であり、続きの行を同じ名前の次の値にする。そのほかの項目は、続きの行を改行でつないで
// 1 つの値にする。
//
// **説明を組めなかったイベントは欄名を推測しない。** unrendered を真にし、イベントが持つ
// 値の並びを、名前の無い 1 つの値として原資料の文字列のまま返す。値の区切りは説明の文字列から
// 決まらない。値の中の改行と、空の値の空行が区切りの改行と同じ文字列である。
//
// PowerShell の 4104 の説明は scriptBlockValues が読む。
func descriptionValues(provider, eventID, description string) (values []Value, unrendered bool) {
	if strings.HasPrefix(description, unrenderedPrefix(provider, eventID)) {
		if _, after, found := strings.Cut(description, insertionStringsMarker); found {
			return []Value{{Text: strings.TrimLeft(after, " \r\n")}}, true
		}
		return nil, true
	}
	if provider == providerPowerShell && eventID == eventIDScriptBlock {
		return scriptBlockValues(description), false
	}
	if sentence, found := taskSchedulerSentences[eventID]; found && provider == providerTaskScheduler {
		return sentence.values(description), false
	}
	section := ""
	// continued は直前の行が値を足した項目の名前である。字下げした `名前:` を持たない行は、
	// continued の項目の続きである。
	continued := ""
	lines := strings.Split(description, "\n")
	for at, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			section, continued = "", ""
			continue
		}
		label, value, ok := descriptionItemOf(trimmed)
		indented := len(trimmed) < len(line)
		if !ok {
			if indented && continued != "" {
				text := strings.TrimRight(trimmed, " \t")
				if listItemNames[continued] {
					values = append(values, Value{Name: continued, Text: text})
				} else {
					values[len(values)-1].Text += "\n" + text
				}
			} else {
				continued = ""
			}
			continue
		}
		continued = ""
		if value == "" && !indented && at+1 < len(lines) && isIndentedItemLine(lines[at+1]) {
			section = label
			continue
		}
		name := label
		if indented && section != "" {
			name = section + "." + label
		}
		if mapped, found := viewerItemNames[viewerItemKey{provider, eventID, section, label}]; found {
			name = mapped
		}
		if provider == providerSecurityAuditing && section == "メンバ" && label == "セキュリティ ID" &&
			slices.Contains(groupMemberAddedEventIDs[:], eventID) {
			values = append(values, accountIdValues("Member", name, value)...)
			continue
		}
		if provider == providerSecurityAuditing && eventID == eventIDServiceTicketRequest &&
			section == "サービス情報" && label == "サービス ID" {
			values = append(values, accountIdValues("Service", name, value)...)
			continue
		}
		// グループの「セキュリティ ID」は SID の形の文字列だけを TargetSid にする。名前へ直した文字列は
		// 項目名の値のまま残す。
		if provider == providerSecurityAuditing && section == "グループ" && label == "セキュリティ ID" &&
			slices.Contains(groupMemberAddedEventIDs[:], eventID) && strings.HasPrefix(value, "S-1-") {
			values = append(values, Value{Name: "TargetSid", Text: value})
			continue
		}
		values = append(values, Value{Name: name, Text: value})
		if value != "" {
			continued = name
		}
	}
	return values, false
}

// listItemNames は、1 行に 1 つの値を持つ一覧の項目の名前である。
var listItemNames = map[string]bool{"特権": true}

// taskSchedulerSentence は、タスク スケジューラの説明の文 1 つと、文が持つ値の `<Data>` の
// Name である。値は文の中の二重引用符の中の文字列であり、二重引用符を含まない。129 のプロセス
// 番号は引用符を持たない数字の並びである。
type taskSchedulerSentence struct {
	pattern *regexp.Regexp
	names   []string
}

// taskSchedulerSentences は、イベントビューアーが日本語で組んだタスク スケジューラの説明の
// 文である。説明は `名前: 値` の行を持たず、値を文の中に書く。
//
// 既知の制限: 文が 1 字でも違う説明から値を取らない。Windows のバージョンで違う文字列の説明と英語の説明は、欄を
// 持たないレコードとして取り込まれる, バージョンごとの文字列を持つ CSV が repo の中に無く測れない, 別の
// 文字列の説明を取り込む要求が出たとき、その文を本表に足す
var taskSchedulerSentences = map[string]taskSchedulerSentence{
	eventIDTaskStarted: {
		regexp.MustCompile(`^タスク スケジューラは、ユーザー "([^"]*)" の "([^"]*)" タスクの "([^"]*)" インスタンスを開始しました。$`),
		[]string{"UserContext", "TaskName", "InstanceId"},
	},
	eventIDTaskRegistered: {
		regexp.MustCompile(`^ユーザー "([^"]*)" はタスク スケジューラのタスク "([^"]*)" を登録しました。$`),
		[]string{"UserContext", "TaskName"},
	},
	eventIDTaskProcessCreated: {
		regexp.MustCompile(`^タスク スケジューラは、プロセス ID (\d+) でタスク "([^"]*)"、インスタンス "([^"]*)" を起動しました。$`),
		[]string{"ProcessID", "TaskName", "Path"},
	},
}

// values は説明の文から値を取り出す。文と一致しない説明は値を返さない。
func (s taskSchedulerSentence) values(description string) []Value {
	match := s.pattern.FindStringSubmatch(strings.TrimRight(description, "\r\n"))
	if match == nil {
		return nil
	}
	values := make([]Value, 0, len(s.names))
	for at, name := range s.names {
		values = append(values, Value{Name: name, Text: match[at+1]})
	}
	return values
}

// providerPowerShell と eventIDScriptBlock は、PowerShell がスクリプトブロックの本文を記録する
// イベントのプロバイダの名前と ID である。
const (
	providerPowerShell = "Microsoft-Windows-PowerShell"
	eventIDScriptBlock = "4104"
)

// scriptBlockHeading は 4104 の説明の 1 行目である。1 つ目の数が分割の総数、2 つ目の数が
// 何番目の分割かである。
var scriptBlockHeading = regexp.MustCompile(`^Scriptblock テキストを作成しています \((\d+) 個中 (\d+) 個目\):\n`)

// scriptBlockValues は 4104 の説明を、XML の 4104 の `<Data>` と同じ名前と並びの値にする。
//
// 本文は 1 行目の後ろから、最後の `ScriptBlock ID:` の行の前の空行までである。本文は任意の
// 行を持つため、`名前: 値` の形の行を項目にしない。1 行目と ScriptBlock ID とパスの行が
// そろわない説明は、欄名を推測せずに値を返さない。
func scriptBlockValues(description string) []Value {
	heading := scriptBlockHeading.FindStringSubmatch(description)
	if heading == nil {
		return nil
	}
	rest := description[len(heading[0]):]
	const idLine = "\n\nScriptBlock ID: "
	at := strings.LastIndex(rest, idLine)
	if at < 0 {
		return nil
	}
	id, path, found := strings.Cut(rest[at+len(idLine):], "\nパス: ")
	if !found {
		return nil
	}
	return []Value{
		{Name: "MessageNumber", Text: heading[2]}, {Name: "MessageTotal", Text: heading[1]},
		{Name: "ScriptBlockText", Text: rest[:at]}, {Name: "ScriptBlockId", Text: id}, {Name: "Path", Text: path},
	}
}

// isIndentedItemLine は、行頭に空白を持ち、空白の後ろに文字列が続く行であるかを返す。
func isIndentedItemLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return len(trimmed) < len(line) && strings.TrimSpace(trimmed) != ""
}

// descriptionItemOf は `名前:` の後ろに空白か行末が続く行を、項目名と値に分ける。値は区切りの
// 空白を除いた文字列である。`C:\` のように `:` の後ろが空白でない行と、名前が文 (`。` を含む)
// である行は項目にしない。
func descriptionItemOf(line string) (label, value string, ok bool) {
	colon := strings.IndexByte(line, ':')
	if colon <= 0 {
		return "", "", false
	}
	rest := line[colon+1:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", "", false
	}
	label = strings.TrimRight(line[:colon], " \t")
	if label == "" || strings.Contains(label, "。") {
		return "", "", false
	}
	return label, strings.TrimLeft(rest, " \t"), true
}
