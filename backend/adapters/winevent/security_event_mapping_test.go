package winevent_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	adminSid  = "S-1-5-21-1000-2000-3000-1201"
	memberSid = "S-1-5-21-1000-2000-3000-1202"
	groupSid  = "S-1-5-21-1000-2000-3000-512"
)

// semanticItem は、項目の名前と、期待する語彙の項目と比べる値である。空の semantic は語彙を
// 持たないことを期待する。
type semanticItem struct {
	name       string
	semantic   core.SemanticKey
	comparable string
}

// requireSemantics は、各項目が期待の語彙と比べる値を持つことを確かめる。
func requireSemantics(t *testing.T, observation winevent.Observation, items ...semanticItem) {
	t.Helper()
	for _, item := range items {
		field := fieldNamed(t, observation.Fields, item.name)
		if field.Semantic != item.semantic {
			t.Errorf("%s semantic = %q, want %q", item.name, field.Semantic, item.semantic)
		}
		if item.comparable == "" {
			continue
		}
		if value, ok := field.Text.ComparableValue(); !ok || value != item.comparable {
			t.Errorf("%s comparable = %q (%t), want %q", item.name, value, ok, item.comparable)
		}
	}
}

// 外向きの 5156 は、自分の端末を接続元、相手を接続先へ写す。10 進のプロセス番号はそのまま比べる。
func TestObserveMapsAnOutboundConnection(t *testing.T) {
	observation := observe(t, logonEvent("5156",
		"ProcessID", "1234", "Application", `\device\harddiskvolume2\example\tool.exe`,
		"Direction", "%%14593", "SourceAddress", "192.0.2.10", "SourcePort", "50100",
		"DestAddress", "198.51.100.20", "DestPort", "443", "Protocol", "6"))
	requireSemantics(t, observation,
		semanticItem{"EventData.ProcessID", core.SemanticKeyProcessPid, "1234"},
		semanticItem{"EventData.Application", core.SemanticKeyProcessBinaryPath, ""},
		semanticItem{"EventData.SourceAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.10"},
		semanticItem{"EventData.SourcePort", core.SemanticKeyConnectionSourcePort, "50100"},
		semanticItem{"EventData.DestAddress", core.SemanticKeyConnectionDestinationAddress, "198.51.100.20"},
		semanticItem{"EventData.DestPort", core.SemanticKeyConnectionDestinationPort, "443"},
		semanticItem{"EventData.Protocol", "", ""},
	)
	if observation.InboundConnection {
		t.Error("the outbound 5156 is an inbound connection record")
	}
}

// 内向きの 5156 は、相手の Dest を接続元へ、自分の端末の port を接続先の port へ写し、自分の
// 端末のアドレスに語彙を与えない。接続先のアドレスを持たない。
func TestObserveMapsAnInboundConnectionFromThePeer(t *testing.T) {
	observation := observe(t, logonEvent("5156",
		"ProcessID", "4", "Direction", "%%14592", "SourceAddress", "192.0.2.10", "SourcePort", "445",
		"DestAddress", "198.51.100.30", "DestPort", "50200"))
	requireSemantics(t, observation,
		semanticItem{"EventData.SourceAddress", "", ""},
		semanticItem{"EventData.SourcePort", core.SemanticKeyConnectionDestinationPort, "445"},
		semanticItem{"EventData.DestAddress", core.SemanticKeyConnectionSourceAddress, "198.51.100.30"},
		semanticItem{"EventData.DestPort", core.SemanticKeyConnectionSourcePort, "50200"},
	)
	if !observation.InboundConnection {
		t.Error("the inbound 5156 is not an inbound connection record")
	}
}

// 内向きの 5156 は、自分の端末の側のアドレスがマルチキャストのとき (protocol を問わない) と、UDP
// (Protocol 17) のブロードキャスト (最後の octet が 255) のときに印を立てる。TCP の最後の octet が
// 255 のアドレス、宛先が 1 台のアドレス、外向きの 5156 には立てない。
func TestObserveMarksAnInboundConnectionToAGroupAddress(t *testing.T) {
	for _, tc := range []struct {
		local, protocol string
		want            bool
	}{
		{"192.0.2.255", "17", true}, {"255.255.255.255", "17", true}, {"::ffff:192.0.2.255", "17", true},
		{"233.252.0.1", "17", true}, {"233.252.0.1", "6", true}, {"ff0e::db8:0:1", "17", true},
		{"192.0.2.255", "6", false}, {"255.255.255.255", "6", false}, {"192.0.2.255", "", false},
		{"192.0.2.10", "17", false}, {"2001:db8::1", "17", false}, {"-", "17", false},
	} {
		for _, direction := range []string{"%%14592", "%%14593"} {
			observation := observe(t, logonEvent("5156", "ProcessID", "4", "Direction", direction,
				"SourceAddress", tc.local, "SourcePort", "137", "DestAddress", "198.51.100.30", "DestPort", "137",
				"Protocol", tc.protocol))
			if got := observation.InboundGroupAddress; got != (tc.want && direction == "%%14592") {
				t.Errorf("%s protocol %q %s marks the group address %v, want %v",
					tc.local, tc.protocol, direction, got, !got)
			}
		}
	}
}

// 5140、4698、4720、4726、4768 は、Subject と Target と接続元と対象の名前を語彙へ写す。
func TestObserveMapsTheSecurityEventsOfAccountsSharesAndTasks(t *testing.T) {
	subject := []string{"SubjectUserSid", adminSid, "SubjectUserName", "admin21",
		"SubjectDomainName", "EXAMPLE", "SubjectLogonId", "0x1a2b"}
	subjectItems := []semanticItem{
		{"EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, adminSid},
		{"EventData.SubjectUserName", core.SemanticKeySubjectAccountName, "admin21"},
		{"EventData.SubjectLogonId", core.SemanticKeyEventSubjectLogonId, "0x1a2b"},
	}
	target := []string{"TargetSid", memberSid, "TargetUserName", "user22", "TargetDomainName", "EXAMPLE"}
	targetItems := []semanticItem{
		{"EventData.TargetSid", core.SemanticKeyTargetAccountSid, memberSid},
		{"EventData.TargetUserName", core.SemanticKeyTargetAccountName, "user22"},
		{"EventData.TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
	}
	for name, tc := range map[string]struct {
		eventID string
		data    []string
		want    []semanticItem
	}{
		"共有": {"5140", append(subject, "IpAddress", "192.0.2.40", "IpPort", "50300",
			"ShareName", `\\*\EXAMPLE$`, "ShareLocalPath", `C:\Example`), append(subjectItems,
			semanticItem{"EventData.IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.40"},
			semanticItem{"EventData.IpPort", core.SemanticKeyConnectionSourcePort, "50300"},
			semanticItem{"EventData.ShareName", core.SemanticKeyShareName, `\\*\EXAMPLE$`},
			semanticItem{"EventData.ShareLocalPath", "", ""})},
		"タスク": {"4698", append(subject, "TaskName", `\Example Task`, "TaskContent", "<Task/>"),
			append(subjectItems,
				semanticItem{"EventData.TaskName", core.SemanticKeyScheduledTaskName, `\Example Task`},
				semanticItem{"EventData.TaskContent", "", ""})},
		"作成": {"4720", append(subject, target...), append(subjectItems, targetItems...)},
		"削除": {"4726", append(subject, target...), append(subjectItems, targetItems...)},
		"TGT": {"4768", append(target, "IpAddress", "192.0.2.50"), append(targetItems,
			semanticItem{"EventData.IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.50"})},
		"5379": {"5379", subject, subjectItems},
	} {
		t.Run(name, func(t *testing.T) {
			requireSemantics(t, observe(t, logonEvent(tc.eventID, tc.data...)), tc.want...)
		})
	}
}

// 4769 は、Target の欄の要求したアカウントを Subject へ、要求されたサービスを Target へ写す。
// 要求したアカウントの `<名前>@<領域>` は名前で比べる。IPv6 の形で書いた IPv4 の接続元は、
// ドット 10 進で比べる。どちらも原資料の文字列を保つ。
func TestObserveMapsTheServiceTicketRequest(t *testing.T) {
	observation := observe(t, logonEvent("4769",
		"TargetUserName", "user23@EXAMPLE.TEST", "TargetDomainName", "EXAMPLE.TEST",
		"ServiceName", "svc24", "ServiceSid", memberSid, "IpAddress", "::ffff:192.0.2.60", "IpPort", "50400"))
	if raw, _ := fieldNamed(t, observation.Fields, "EventData.TargetUserName").Text.RawTextValue(); raw != "user23@EXAMPLE.TEST" {
		t.Errorf("TargetUserName raw = %q, want the original text", raw)
	}
	requireSemantics(t, observation,
		semanticItem{"EventData.TargetUserName", core.SemanticKeySubjectAccountName, "user23"},
		semanticItem{"EventData.TargetDomainName", core.SemanticKeySubjectAccountDomain, "EXAMPLE.TEST"},
		semanticItem{"EventData.ServiceName", core.SemanticKeyTargetAccountName, "svc24"},
		semanticItem{"EventData.ServiceSid", core.SemanticKeyTargetAccountSid, memberSid},
		semanticItem{"EventData.IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.60"},
	)
	if raw, _ := fieldNamed(t, observation.Fields, "EventData.IpAddress").Text.RawTextValue(); raw != "::ffff:192.0.2.60" {
		t.Errorf("IpAddress raw = %q, want the original text", raw)
	}
}

// 4648 の IpAddress と IpPort は接続先、Target は要求に使った資格情報のアカウントである。
// `-` のアドレスと port は語彙を持たないか値の不在である。
func TestObserveMapsTheExplicitCredentialLogon(t *testing.T) {
	observation := observe(t, logonEvent("4648",
		"SubjectUserSid", memberSid, "SubjectUserName", "user31", "SubjectDomainName", "EXAMPLE",
		"TargetUserName", "user32", "TargetDomainName", "EXAMPLE", "TargetServerName", "host33.example.test",
		"IpAddress", "192.0.2.70", "IpPort", "445"))
	requireSemantics(t, observation,
		semanticItem{"EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, memberSid},
		semanticItem{"EventData.TargetUserName", core.SemanticKeyTargetAccountName, "user32"},
		semanticItem{"EventData.TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
		semanticItem{"EventData.TargetServerName", core.SemanticKeyConnectionDestinationServerName,
			"host33.example.test"},
		semanticItem{"EventData.IpAddress", core.SemanticKeyConnectionDestinationAddress, "192.0.2.70"},
		semanticItem{"EventData.IpPort", core.SemanticKeyConnectionDestinationPort, "445"})
	dashed := observe(t, logonEvent("4648", "IpAddress", "-", "IpPort", "-"))
	requireSemantics(t, dashed, semanticItem{"EventData.IpAddress", "", ""})
	if port := fieldNamed(t, dashed.Fields, "EventData.IpPort"); port.Text.ValueState != core.ValueStateAbsent {
		t.Errorf("IpPort - state = %q, want absent", port.Text.ValueState)
	}
	// 端末の中だけで意味を持つ名前と、IP アドレスとして読める名前は、接続先の名前にしない。
	for _, name := range []string{"localhost", "LOCALHOST", "-", "127.0.0.1", "::1", "192.0.2.70"} {
		requireSemantics(t, observe(t, logonEvent("4648", "TargetServerName", name)),
			semanticItem{"EventData.TargetServerName", "", ""})
	}
}

// 4624 の TargetOutbound* は、他の端末への接続に使う資格情報の項目であり、ログオンした
// アカウントの項目ではない。
func TestObserveMapsTheOutboundCredentialsOfTheLogon(t *testing.T) {
	requireSemantics(t, observe(t, logonEvent("4624",
		"TargetUserName", "user34", "TargetOutboundUserName", "user35", "TargetOutboundDomainName", ".")),
		semanticItem{"EventData.TargetUserName", core.SemanticKeyTargetAccountName, "user34"},
		semanticItem{"EventData.TargetOutboundUserName", core.SemanticKeyEventOutboundAccountName, "user35"},
		semanticItem{"EventData.TargetOutboundDomainName", core.SemanticKeyEventOutboundAccountDomain, "."})
	description := "アカウントが正常にログオンしました。\n\n" +
		"新しいログオン:\n\tアカウント名:\t\tuser34\n\tネットワーク アカウント名:\tuser35\n" +
		"\tネットワーク アカウント ドメイン:\tEXAMPLE\n"
	requireSemantics(t, observeViewer(t, "4624", description),
		semanticItem{"EventData.TargetOutboundUserName", core.SemanticKeyEventOutboundAccountName, "user35"},
		semanticItem{"EventData.TargetOutboundDomainName", core.SemanticKeyEventOutboundAccountDomain, "EXAMPLE"})
}

// 4624 と 4625 の AuthenticationPackageName を認証の方式へ写す。CSV は説明の「詳細な認証情報」
// の「認証パッケージ」を同じ名前へ写す。"-" は値の不在である。
func TestObserveMapsTheAuthenticationPackageOfTheLogon(t *testing.T) {
	for _, eventID := range []string{"4624", "4625"} {
		requireSemantics(t, observe(t, logonEvent(eventID, "AuthenticationPackageName", "NTLM")),
			semanticItem{"EventData.AuthenticationPackageName", core.SemanticKeyEventAuthenticationPackage, "NTLM"})
	}
	dashed := observe(t, logonEvent("4624", "AuthenticationPackageName", "-"))
	if field := fieldNamed(t, dashed.Fields, "EventData.AuthenticationPackageName"); field.Text.ValueState != core.ValueStateAbsent {
		t.Errorf("AuthenticationPackageName - state = %q, want absent", field.Text.ValueState)
	}
	description := "アカウントが正常にログオンしました。\n\n" +
		"詳細な認証情報:\n\tログオン プロセス:\t\tNtLmSsp\n\t認証パッケージ:\tKerberos\n"
	requireSemantics(t, observeViewer(t, "4624", description),
		semanticItem{"EventData.AuthenticationPackageName", core.SemanticKeyEventAuthenticationPackage, "Kerberos"})
}

// 4728・4732・4756 は、追加されたメンバーをアカウントの Target へ、グループを target_group へ
// 写す。メンバーの識別名は、先頭の CN の値を名前として比べ、原資料の文字列を保つ。
func TestObserveMapsTheMemberAndTheGroupOfAGroupChange(t *testing.T) {
	for _, eventID := range []string{"4728", "4732", "4756"} {
		observation := observe(t, logonEvent(eventID,
			"MemberName", `CN=Doe\, user25,CN=Users,DC=example,DC=test`, "MemberSid", memberSid,
			"TargetUserName", "Example Admins", "TargetDomainName", "EXAMPLE", "TargetSid", groupSid,
			"SubjectUserSid", adminSid, "SubjectUserName", "admin21"))
		requireSemantics(t, observation,
			semanticItem{"EventData.MemberName", core.SemanticKeyTargetAccountName, "Doe, user25"},
			semanticItem{"EventData.MemberSid", core.SemanticKeyTargetAccountSid, memberSid},
			semanticItem{"EventData.TargetSid", core.SemanticKeyTargetGroupSid, groupSid},
			semanticItem{"EventData.TargetUserName", core.SemanticKeyTargetGroupName, "Example Admins"},
			semanticItem{"EventData.TargetDomainName", core.SemanticKeyTargetGroupDomain, "EXAMPLE"},
			semanticItem{"EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, adminSid},
		)
		name := fieldNamed(t, observation.Fields, "EventData.MemberName")
		if raw, _ := name.Text.RawTextValue(); raw != `CN=Doe\, user25,CN=Users,DC=example,DC=test` {
			t.Errorf("%s MemberName raw = %q", eventID, raw)
		}
	}
}

// CSV の 4728・4732 は、メンバーの「セキュリティ ID」が名前へ直した `<ドメイン>\<名前>` の文字列で
// あるとき、ドメインと名前をメンバーのアカウントへ写す。SID の形の文字列は SID へ写す。
func TestObserveMapsTheViewerMemberAndGroupOfAGroupChange(t *testing.T) {
	description := func(heading, member, group string) string {
		return heading + "\n\nサブジェクト:\n\tセキュリティ ID:\t\tEXAMPLE\\admin21\n" +
			"\tアカウント名:\t\tadmin21\n\tアカウント ドメイン:\t\tEXAMPLE.TEST\n\tログオン ID:\t\t0x1\n\n" +
			"メンバ:\n\tセキュリティ ID:\t\t" + member + "\n" +
			"\tアカウント名:\t\tCN=user25,CN=Users,DC=example,DC=test\n\n" +
			"グループ:\n\tセキュリティ ID:\t\t" + group + "\n\tグループ名:\t\tExample Admins\n" +
			"\tグループ ドメイン:\t\tEXAMPLE\n"
	}
	for eventID, heading := range map[string]string{
		"4728": "セキュリティが有効なグローバル グループにメンバが追加されました。",
		"4732": "セキュリティが有効なローカル グループにメンバが追加されました。",
		"4756": "セキュリティが有効なユニバーサル グループにメンバが追加されました。",
	} {
		named := observeViewer(t, eventID, description(heading, `EXAMPLE\user25`, `EXAMPLE\Example Admins`))
		for _, field := range named.Fields {
			if field.Semantic == core.SemanticKeyTargetGroupSid {
				t.Errorf("%s maps the group name to the group sid", eventID)
			}
		}
		requireSemantics(t, named,
			semanticItem{"EventData.MemberDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
			semanticItem{"EventData.MemberUserName", core.SemanticKeyTargetAccountName, "user25"},
			semanticItem{"EventData.MemberName", core.SemanticKeyTargetAccountName, "user25"},
			semanticItem{"EventData.TargetUserName", core.SemanticKeyTargetGroupName, "Example Admins"},
			semanticItem{"EventData.TargetDomainName", core.SemanticKeyTargetGroupDomain, "EXAMPLE"},
			semanticItem{"EventData.SubjectUserName", core.SemanticKeySubjectAccountName, "admin21"},
		)
		unresolved := observeViewer(t, eventID, description(heading, memberSid, groupSid))
		requireSemantics(t, unresolved,
			semanticItem{"EventData.MemberSid", core.SemanticKeyTargetAccountSid, memberSid},
			semanticItem{"EventData.TargetSid", core.SemanticKeyTargetGroupSid, groupSid})
	}
}

// IP アドレスの項目は、IPv6 の形で書いた IPv4 だけをドット 10 進で比べる。IPv6 と読めない
// 文字列は原資料の文字列のまま比べ、"-" は値の不在である。
func TestObserveComparesIPv4MappedAddressesAsIPv4(t *testing.T) {
	for raw, want := range map[string]struct {
		comparable string
		present    bool
	}{
		"::ffff:192.0.2.70": {"192.0.2.70", true},
		"::FFFF:c000:246":   {"192.0.2.70", true},
		"2001:db8::1":       {"2001:db8::1", true},
		"192.0.2.300":       {"192.0.2.300", true},
		"-":                 {"", false},
	} {
		field := fieldNamed(t, observe(t, logonEvent("4624", "IpAddress", raw)).Fields, "EventData.IpAddress")
		value, ok := field.Text.ComparableValue()
		if ok != want.present || value != want.comparable {
			t.Errorf("%q comparable = %q (%t), want %q (%t)", raw, value, ok, want.comparable, want.present)
		}
	}
}

// 外向きの通信の説明。見出しと項目名は Windows が 5156 の説明に書く文字列である。
const connectionDescription = "Windows フィルタリング プラットフォームで、接続が許可されました。\n\n" +
	"アプリケーション情報:\n" +
	"\tプロセス ID:\t\t1234\n" +
	"\tアプリケーション名:\t\\device\\harddiskvolume2\\example\\tool.exe\n\n" +
	"ネットワーク情報:\n" +
	"\t方向:\t\t送信\n" +
	"\t送信元アドレス:\t\t192.0.2.10\n" +
	"\tソース ポート:\t\t50100\n" +
	"\t宛先アドレス:\t\t198.51.100.20\n" +
	"\t宛先ポート:\t\t443\n" +
	"\tプロトコル:\t\t6\n"

// 共有へのアクセスの説明。見出しと項目名は Windows が 5140 の説明に書く文字列である。
const shareAccessDescription = "ネットワーク共有オブジェクトにアクセスしました。\n\t\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tadmin21\n" +
	"\tアカウント ドメイン:\t\tEXAMPLE\n" +
	"\tログオン ID:\t\t0x1a2b\n\n" +
	"ネットワーク情報:\t\n" +
	"\t送信元アドレス:\t\t192.0.2.40\n" +
	"\tソース ポート:\t\t50300\n\t\n" +
	"共有名:\t\t\\\\*\\EXAMPLE$\n"

// チケットの要求の説明。見出しと項目名は Windows が 4769 の説明に書く文字列である。
const serviceTicketDescription = "Kerberos サービス チケットが要求されました。\n\n" +
	"アカウント情報:\n" +
	"\tアカウント名:\t\tuser23@EXAMPLE.TEST\n" +
	"\tアカウント ドメイン:\t\tEXAMPLE.TEST\n\n" +
	"サービス情報:\n" +
	"\tサービス名:\t\tsvc24\n\n" +
	"ネットワーク情報:\n" +
	"\tクライアント アドレス:\t::ffff:192.0.2.60\n" +
	"\tクライアント ポート:\t50400\n"

// グループの変更の説明。見出しと項目名は Windows が 4728 の説明に書く文字列である。
const groupChangeDescription = "セキュリティが有効なグローバル グループにメンバーが追加されました。\n\n" +
	"サブジェクト:\n" +
	"\tアカウント名:\t\tadmin21\n\n" +
	"メンバ:\n" +
	"\tアカウント名:\t\tCN=user25,CN=Users,DC=example,DC=test\n\n" +
	"グループ:\n" +
	"\tグループ名:\t\tExample Admins\n" +
	"\tグループ ドメイン:\t\tEXAMPLE\n"

// イベントビューアーの説明の項目が、XML の `<Data>` と同じ語彙の項目を持つ。CSV の 5156 の
// 向きは日本語の文字列で書かれる。
func TestObserveMapsTheViewerItemsOfTheSecurityEvents(t *testing.T) {
	for name, tc := range map[string]struct {
		eventID     string
		description string
		want        []semanticItem
	}{
		"外向きの通信": {"5156", connectionDescription, []semanticItem{
			{"ProcessID", core.SemanticKeyProcessPid, "1234"},
			{"Application", core.SemanticKeyProcessBinaryPath, ""},
			{"SourceAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.10"},
			{"DestAddress", core.SemanticKeyConnectionDestinationAddress, "198.51.100.20"},
			{"DestPort", core.SemanticKeyConnectionDestinationPort, "443"},
		}},
		"共有": {"5140", shareAccessDescription, []semanticItem{
			{"SubjectUserName", core.SemanticKeySubjectAccountName, "admin21"},
			{"SubjectLogonId", core.SemanticKeyEventSubjectLogonId, "0x1a2b"},
			{"IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.40"},
			{"IpPort", core.SemanticKeyConnectionSourcePort, "50300"},
			{"ShareName", core.SemanticKeyShareName, `\\*\EXAMPLE$`},
		}},
		"チケット": {"4769", serviceTicketDescription, []semanticItem{
			{"TargetUserName", core.SemanticKeySubjectAccountName, "user23"},
			{"ServiceName", core.SemanticKeyTargetAccountName, "svc24"},
			{"IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.60"},
		}},
		"タスク": {"4698", "スケジュールされたタスクが作成されました。\n\n" +
			"サブジェクト:\n\tアカウント名:\t\tadmin21\n\n" +
			"タスク情報:\n\tタスク名:\t\t\\Example Task\n", []semanticItem{
			{"SubjectUserName", core.SemanticKeySubjectAccountName, "admin21"},
			{"TaskName", core.SemanticKeyScheduledTaskName, `\Example Task`},
		}},
		"作成": {"4720", "ユーザー アカウントが作成されました。\n\n" +
			"サブジェクト:\n\tアカウント名:\t\tadmin21\n\n" +
			"新しいアカウント:\n\tアカウント名:\t\tuser22\n\tアカウント ドメイン:\t\tEXAMPLE\n", []semanticItem{
			{"TargetUserName", core.SemanticKeyTargetAccountName, "user22"},
			{"TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
		}},
		"TGT": {"4768", "Kerberos 認証チケット (TGT) が要求されました。\n\n" +
			"アカウント情報:\n\tアカウント名:\t\tuser22\n\t提供された領域名:\t\tEXAMPLE.TEST\n\n" +
			"ネットワーク情報:\n\tクライアント アドレス:\t::ffff:192.0.2.50\n\tクライアント ポート:\t50500\n", []semanticItem{
			{"TargetUserName", core.SemanticKeyTargetAccountName, "user22"},
			{"TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE.TEST"},
			{"IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.50"},
			{"IpPort", core.SemanticKeyConnectionSourcePort, "50500"},
		}},
		"グループ": {"4728", groupChangeDescription, []semanticItem{
			{"SubjectUserName", core.SemanticKeySubjectAccountName, "admin21"},
			{"TargetUserName", core.SemanticKeyTargetGroupName, "Example Admins"},
			{"TargetDomainName", core.SemanticKeyTargetGroupDomain, "EXAMPLE"},
		}},
		"資格情報": {"4648", "明示的な資格情報を使用してログオンが試行されました。\n\n" +
			"サブジェクト:\n\tアカウント名:\t\tadmin21\n\n" +
			"資格情報が使用されたアカウント:\n\tアカウント名:\t\tuser22\n\tアカウント ドメイン:\t\tEXAMPLE\n\n" +
			"ターゲット サーバー:\n\tターゲット サーバー名:\thost23.example.test\n\t追加情報:\thost23.example.test\n\n" +
			"ネットワーク情報:\n\tネットワーク アドレス:\t192.0.2.70\n\tポート:\t\t\t445\n", []semanticItem{
			{"TargetUserName", core.SemanticKeyTargetAccountName, "user22"},
			{"TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
			{"TargetServerName", core.SemanticKeyConnectionDestinationServerName, "host23.example.test"},
			{"IpAddress", core.SemanticKeyConnectionDestinationAddress, "192.0.2.70"},
			{"IpPort", core.SemanticKeyConnectionDestinationPort, "445"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			observation := observeViewer(t, tc.eventID, tc.description)
			for at := range tc.want {
				tc.want[at].name = "EventData." + tc.want[at].name
			}
			requireSemantics(t, observation, tc.want...)
		})
	}
}

// イベントビューアーの CSV の内向きの 5156 も、「プロトコル」が 17 (UDP) で送信元アドレスが
// ブロードキャストのときに印を立てる。6 (TCP) には立てない。
func TestObserveMarksAViewerInboundBroadcastByTheProtocol(t *testing.T) {
	for protocol, want := range map[string]bool{"17": true, "6": false} {
		description := strings.NewReplacer("送信\n", "受信\n", "192.0.2.10", "192.0.2.255",
			"プロトコル:\t\t6", "プロトコル:\t\t"+protocol).Replace(connectionDescription)
		if got := observeViewer(t, "5156", description).InboundGroupAddress; got != want {
			t.Errorf("the viewer 5156 with the protocol %s marks the group address %v, want %v", protocol, got, want)
		}
	}
}

// 5140 の「共有名」は、Windows のバージョンにより「共有情報」の見出しの下に置かれる。
func TestObserveMapsTheShareNameUnderTheShareInformationSection(t *testing.T) {
	description := "ネットワーク共有オブジェクトにアクセスしました。\n\t\n" +
		"ネットワーク情報:\t\n\t送信元アドレス:\t\t192.0.2.40\n\t\n" +
		"共有情報:\n\t共有名:\t\t\t\\\\*\\EXAMPLE$\n\t共有パス:\t\tC:\\Example\n"
	requireSemantics(t, observeViewer(t, "5140", description),
		semanticItem{"EventData.ShareName", core.SemanticKeyShareName, `\\*\EXAMPLE$`},
		semanticItem{"EventData.IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.40"},
	)
}

// CSV の 4769 の「サービス ID」は、名前へ直した文字列ならドメインと名前、SID の形なら SID に
// なり、要求されたサービスのアカウントを Target のノードにする。
func TestObserveMapsTheViewerServiceIdToTheTargetAccount(t *testing.T) {
	for serviceId, want := range map[string]core.NodeKeyForm{
		`EXAMPLE\svc24`:                core.NodeKeyFormAccountDomainName,
		"S-1-5-21-1000-2000-3000-1024": core.NodeKeyFormAccountSid,
	} {
		description := strings.Replace(serviceTicketDescription, "\tサービス名:\t\tsvc24\n",
			"\tサービス名:\t\tsvc24\n\tサービス ID:\t\t"+serviceId+"\n", 1)
		observation := observeViewer(t, "4769", description)
		var targets []core.NodeKey
		for _, naming := range core.NewRecordGraph(observation.Fields).Namings {
			if naming.Kind == core.EdgeKindRecordTargetAccount {
				targets = append(targets, naming.Target)
			}
		}
		if len(targets) != 1 || targets[0].Form != want {
			t.Errorf("%s: the target accounts = %+v, want one %s", serviceId, targets, want)
		}
	}
}

// 内向きの CSV の 5156 は、相手を接続元へ写す。
func TestObserveMapsAnInboundViewerConnectionFromThePeer(t *testing.T) {
	description := "Windows フィルタリング プラットフォームで、接続が許可されました。\n\n" +
		"ネットワーク情報:\n\t方向:\t\t着信\n\t送信元アドレス:\t\t192.0.2.10\n" +
		"\t宛先アドレス:\t\t198.51.100.30\n"
	requireSemantics(t, observeViewer(t, "5156", description),
		semanticItem{"EventData.SourceAddress", "", ""},
		semanticItem{"EventData.DestAddress", core.SemanticKeyConnectionSourceAddress, "198.51.100.30"},
	)
}
