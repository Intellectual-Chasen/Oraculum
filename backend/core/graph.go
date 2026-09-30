package core

import (
	"net/netip"
	"strconv"
)

// NodeKind はグラフのノードの種別である。値は語彙の対象 (SemanticObject) のうちノードを
// 作る対象に 1 対 1 で対応する。
//
// 通信と事象と HTTP はノードにならない。通信は操作のイベントであり、ノードの間の関係が
// 持つ。3 つの対象が持つ欄は、その欄を持つレコードのノードの属性になる。
type NodeKind string

// NodeKind の値。
const (
	// NodeKindTerminal は端末である。
	NodeKindTerminal NodeKind = "terminal"
	// NodeKindProcess はプロセスである。
	NodeKindProcess NodeKind = "process"
	// NodeKindFile はファイルである。
	NodeKindFile NodeKind = "file"
	// NodeKindRegistryValue は Windows レジストリの値である。
	NodeKindRegistryValue NodeKind = "registry_value"
	// NodeKindAccount はアカウントである。
	NodeKindAccount NodeKind = "account"
	// NodeKindIp は IP アドレスである。
	NodeKindIp NodeKind = "ip"
	// NodeKindDomain はホスト名である。
	NodeKindDomain NodeKind = "domain"
	// NodeKindRecord は収集元の中のレコード 1 件である。
	//
	// **読めた欄をすべてグラフへ載せる置き場である。** 語彙の項目を持たない欄、対象が
	// 通信と事象と HTTP の欄、役割が identity の欄、同じ種別の対象を 2 つ以上記録した
	// レコードの欄、対象を 1 つも指さないレコードの欄は、対象のノードへ帰属先が決まら
	// ない。どれもその欄を持つレコードのノードの属性になる。
	NodeKindRecord NodeKind = "record"
)

// IsKnown は NodeKind が定義の中の値であるかを返す。
func (k NodeKind) IsKnown() bool {
	switch k {
	case NodeKindTerminal, NodeKindProcess, NodeKindFile, NodeKindRegistryValue,
		NodeKindAccount, NodeKindIp, NodeKindDomain, NodeKindRecord:
		return true
	default:
		return false
	}
}

// CarriesCreationRecord は、その種別のノードが生成を記録した根拠を持てるかを返す。
// 持てる種別のノードだけが NodeCreationRecord の present と absent を取る。
//
// **持てるのはプロセスとアカウントである。** 生成の事象はプロセスの起動とアカウントの作成で
// あり、ファイル・レジストリの値の生成の事象を持つ入力形式をまだ取り込んでいない。ファイルの
// file.created_time はファイルの属性として観測した時刻であり、生成の事象を記録したレコードの
// 有無とは別である。
func (k NodeKind) CarriesCreationRecord() bool {
	return k == NodeKindProcess || k == NodeKindAccount
}

// EdgeKind は関係の種別である。
//
// **イベントの種別を関係の種別に読み替えない。** 1 レコードの観測の種別は根拠の属性と
// して ObservationKind が持つ。原資料の文字列を既知の関係へ読み替える種別を置かない。
type EdgeKind string

// EdgeKind の値。
const (
	// EdgeKindRanOn はプロセスが端末で動いた関係である。
	EdgeKindRanOn EdgeKind = "ran_on"
	// EdgeKindProcessParentChild は親のプロセスが子のプロセスを起動した関係である。
	//
	// 根拠は子の起動のレコードである。親の起動のレコードを収集元が持たないときも関係は
	// 成り立ち、親のノードは observation が referenced になる。
	EdgeKindProcessParentChild EdgeKind = "process_parent_child"
	// EdgeKindProcessInjection は、レコードが注入元のプロセスから注入先のプロセスへの
	// コードインジェクションを記録した関係である。
	//
	// **注入の記録は悪性を意味しない。** レコードが持つのは、注入元と注入先と、注入に
	// 使った API の名前だけである。悪性の判断は分析者が別の根拠と合わせて行う。
	EdgeKindProcessInjection EdgeKind = "process_injection"
	// EdgeKindCrossSourceConnectionMatch は、ある収集元のレコードが記録した通信の相手として、
	// 別の収集元のレコードが記録したプロセスを関連付けが挙げた関係である。
	//
	// **確定した関係ではない。** 状態は candidate である。関連付けの候補が 1 件でも、
	// 証拠が足りなければ候補のままである。
	EdgeKindCrossSourceConnectionMatch EdgeKind = "cross_source_connection_match"
	// EdgeKindFileOperation はプロセスがファイルを対象にした操作の関係である。
	EdgeKindFileOperation EdgeKind = "file_operation"
	// EdgeKindFileCopy はレコードがファイルの内容をコピー元からコピー先へ移した関係である。
	EdgeKindFileCopy EdgeKind = "file_copy"
	// EdgeKindProcessExecutable は、プロセスの起動のレコードが記録した実行ファイルから、
	// 起動したプロセスへの関係である。実行ファイルのノードは、ほかのファイルのノードと同じく
	// 端末と path で識別する。
	EdgeKindProcessExecutable EdgeKind = "process_executable"
	// EdgeKindFileContentMatch は同じハッシュを観測したファイルの内容一致候補である。
	// **ファイル実体の同一性や来歴を確定しない。** 状態は candidate である。
	EdgeKindFileContentMatch EdgeKind = "file_content_match"
	// EdgeKindRegistryOperation はプロセスがレジストリの値を対象にした操作の関係である。
	EdgeKindRegistryOperation EdgeKind = "registry_operation"
	// EdgeKindProcessCommunication はプロセスが接続先を相手にした通信の関係である。
	EdgeKindProcessCommunication EdgeKind = "process_communication"
	// EdgeKindTerminalAddress は端末が IP アドレスを持った関係である。
	EdgeKindTerminalAddress EdgeKind = "terminal_address"
	// EdgeKindTerminalRemoteSession は、レコードが記録した接続元のアドレスを保持していた
	// 端末から、そのレコードを記録した端末への遠隔のセッションの候補である。
	//
	// **確定した関係ではない。** 状態は candidate である。接続元のアドレスから端末を
	// 導くのは IP から端末への割当であり、割当は収集元の観測期間の中でだけ適用できる。
	// 同じアドレスを別の期間に別の端末が持つ入力では、候補が 2 件以上になる。
	//
	// **接続に成功したことを意味しない。** 遠隔ログインの失敗を記録したレコードも同じ
	// 関係を作る。成功と失敗を分けるのは根拠のレコードの観測の種別である。
	//
	// **port の一致だけでプロトコルと横展開を確定しない。** 1 本の関係が、用いた手段の
	// 異なる根拠を同時に持つ。用いた手段を分けて読む材料は EdgeEvidenceGroup が持つ観測の
	// 種別と接続先 port であり、その port がどのプロトコルであるかは Assertion のメモとして
	// 分析者が書く。
	EdgeKindTerminalRemoteSession EdgeKind = "terminal_remote_session"
	// EdgeKindTerminalAccount は端末のレコードがアカウントを記録した関係である。
	//
	// **「ログインした」を意味しない。** レコードが指すアカウントには、その端末で操作を
	// 行ったアカウントと、その端末へのログインを試みたアカウントの両方が入る。
	// どちらであるかを分けるのは根拠のイベントの種別であり、ObservationKind と
	// event.action が持つ。**「ログインした」という関係は観測層に置かない。**
	EdgeKindTerminalAccount EdgeKind = "terminal_account"
	// EdgeKindHttpRequest は接続元のアドレスが要求先へ HTTP の要求を出した関係である。
	EdgeKindHttpRequest EdgeKind = "http_request"
	// EdgeKindRecordNamesObject は、レコードがその対象を指す関係である。起点が
	// レコードのノード、終点が指された対象のノードである。
	//
	// **対象そのものを記録した関係と、属性として参照しただけの関係を、種別で分けない。**
	// 区別は終点のノードの NodeObservation が持つ。分けると、同じ 1 本の関係を 2 つの
	// 種別へ振り分ける判定を、関係の側にも対象の側にも置くことになる。
	//
	// **役割を付けて記録したアカウントは、本種別で結ばない。** 結ぶのは
	// EdgeKindRecordSubjectAccount と EdgeKindRecordTargetAccount である。
	EdgeKindRecordNamesObject EdgeKind = "record_names_object"
	// EdgeKindRecordSubjectAccount は、レコードが、記録した操作を行ったアカウントとして
	// そのアカウントを記録した関係である。起点がレコードのノード、終点がアカウントのノードである。
	EdgeKindRecordSubjectAccount EdgeKind = "record_subject_account"
	// EdgeKindRecordTargetAccount は、レコードが、記録した操作またはログオンの対象として
	// そのアカウントを記録した関係である。起点がレコードのノード、終点がアカウントのノードである。
	//
	// **「ログインした」を意味しない。** ログオンの失敗を記録したレコードも、ログオンを試みた
	// アカウントの SID を記録していれば同じ関係を作る。成功と失敗を分けるのは根拠の
	// レコードの観測の種別である。
	EdgeKindRecordTargetAccount EdgeKind = "record_target_account"
	// EdgeKindLogonSessionOperation は、ログオンのレコードから、そのログオンが作ったセッションで
	// 行った操作のレコードへの候補である。起点と終点はどちらもレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、同じ端末の 2 件のレコードの
	// Logon ID の一致と時刻の前後だけである。Logon ID は端末を起動し直すと同じ値を再び使うため、
	// 同じ Logon ID のログオンが同じ端末に 2 件以上あるときは、操作のレコード 1 件が候補を
	// 2 本以上持つ。
	EdgeKindLogonSessionOperation EdgeKind = "logon_session_operation"
	// EdgeKindArgumentNamesObject は、コマンド行の引数が UNC の path または `http(s)://` の URL で
	// 記録したアドレスまたはホスト名と、ドライブ文字の path で記録したファイルへの候補である。起点は
	// そのコマンド行で生成されたプロセスのノードである。生成を記録したプロセスを持たないレコードでは、
	// レコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。**引数に現れたことを、接続・
	// ログオン・ファイルの作成や書き込みの成立と読み替えない。** コピー元と読むだけの
	// ファイルも指された対象になる。
	//
	// **引数が指すホスト名は、分析者の割当がそのホスト名を記録した端末のノードへも結ぶ。**
	// 割当は同じ案件にあり、レコードの時刻が割当の期間の中にあるときだけ結ぶ。関係は割当を
	// 持ち、根拠のレコードは引数のレコードである。割当を持たない名前の一致では端末へ結ばない。
	EdgeKindArgumentNamesObject EdgeKind = "argument_names_object"
	// EdgeKindTaskRegistrationRun は、タスクの登録のレコードから、同じ名前のタスクの起動の
	// レコードへの候補である。起点と終点はどちらもレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、同じ端末の 2 件のレコードの
	// タスクの名前の一致と時刻の前後だけである。起動は、登録を記録した観測の種別ごとに、
	// 起動より後でない登録のうち時刻が最も遅い登録 (起動の時刻で有効な登録の内容のバージョン)
	// とだけ結ばれる。
	// 起動したプロセスと、登録の定義の命令は比べない。
	EdgeKindTaskRegistrationRun EdgeKind = "task_registration_run"
	// EdgeKindLinkedLogon は、1 つのログオンが分けて作った 2 つのトークンのログオンのレコード
	// どうしの候補である。起点と終点はどちらもレコードのノードであり、起点は先に取り込んだ側である。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、同じ端末の 2 件のログオンの
	// レコードの Logon ID と、分けたログオンの Logon ID が互いを指すことだけである。昇格の
	// 有無は根拠のレコードの原文字列で読む。後続の操作へは張らない。操作は各ログオンの
	// logon_session_operation が持つ。
	EdgeKindLinkedLogon EdgeKind = "linked_logon"
	// EdgeKindTicketRequestLogon は、Kerberos のチケットの要求のレコードから、同じ LogonGuid の
	// ログオンのレコードへの候補である。起点と終点はどちらもレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は LogonGuid の一致だけであり、
	// 端末をまたいで結ぶ。時刻は比べない。全桁 0 の LogonGuid は一致の根拠にしない。
	// 要求の成功とログオンの成立を読み替えない。
	EdgeKindTicketRequestLogon EdgeKind = "ticket_request_logon"
	// EdgeKindReverseLookupName は、通信したプロセスから、接続先のアドレスを収集元が逆引きした
	// 名前のノードへの候補である。起点はレコードが記録したプロセスのノードであり、プロセスの
	// ノードが無いレコードではレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、収集元が接続先のアドレスを
	// 逆引きした名前だけである。逆引きの名前は IP アドレスの持ち主が決めるため、正規の
	// ドメインの名前を持つ別の相手でもありうる。**プロセスがその名前を要求したことと
	// 読み替えない。**
	EdgeKindReverseLookupName EdgeKind = "reverse_lookup_name"
	// EdgeKindConnectionLogonMatch は、端末の外向きの接続を記録したレコードから、接続先の端末が
	// 記録したログオンのレコードへの候補である。起点と終点はどちらもレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、接続の接続元のアドレス・port と、
	// ログオンが記録した接続元のアドレス・port の一致と、時刻の許容幅だけである。接続先の
	// アドレスをログオンを記録した端末へ導くのは分析者の割当であり、割当の期間の中でだけ結ぶ。
	// 接続の成功とログオンの成立を読み替えない。
	EdgeKindConnectionLogonMatch EdgeKind = "connection_logon_match"
	// EdgeKindProcessIdentityMatch は、一意な識別子で識別したプロセスと、プロセス番号と区間で
	// 識別したプロセスが、同じプロセスである候補である。起点は一意な識別子のプロセスのノード、
	// 終点は区間のプロセスのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、同じ端末の範囲で、プロセス番号が
	// 一致し、2 つのプロセスの観測の時刻の範囲が重なることだけである。プロセス番号を再利用して
	// 範囲が重ならない組は結ばない。
	EdgeKindProcessIdentityMatch EdgeKind = "process_identity_match"
	// EdgeKindExplicitCredentialLogon は、明示的な資格情報を使ったログオンの要求のレコードから、
	// 接続先の端末が記録したログオンの成功のレコードへの候補である。起点と終点はどちらも
	// レコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、要求の接続先のアドレスを分析者の
	// 割当が導いた端末がログオンを記録したこと、要求の資格情報のアカウントの名前とログオンした
	// アカウントの名前の一致、時刻の許容幅だけである。割当の期間の外と、要求を記録した端末
	// 自身へのログオンは結ばない。**要求の成功とログオンの成立を読み替えない。**
	EdgeKindExplicitCredentialLogon EdgeKind = "explicit_credential_logon" //nolint:gosec // 関係の種別の文字列であり、資格情報の値ではない。
	// EdgeKindUnidentifiedSourceRemoteSession は、ログオンの接続元の IP アドレスのノードから、
	// そのログオンを記録した端末への、接続元の端末が未同定の遠隔のセッションの候補である。
	//
	// **確定した関係ではない。** 状態は candidate である。起点は IP アドレスであり、端末として
	// 扱わない。レコードの時刻にそのアドレスを持つ端末の割当が無いとき (割当の期間の外を
	// 含む) だけ作る。割当が端末を導くレコードは EdgeKindTerminalRemoteSession を作る。
	// ループバックとリンクローカルのアドレスと、記録した端末が持つアドレスからは作らない。
	EdgeKindUnidentifiedSourceRemoteSession EdgeKind = "unidentified_source_remote_session"
	// EdgeKindTerminalOutboundConnection は、レコードを記録した端末から、その端末が始めた接続の
	// 接続先としてレコードが記録した IP アドレスへの関係である。状態は observed である。
	//
	// **接続の成立を意味しない。** 資格情報を指定した要求のように、試行だけを記録したレコードも
	// 同じ関係を作る。どのレコードが端末の始めた接続を記録したかを決めるのは入力形式を読む
	// binding である。マルチキャスト・ブロードキャスト・ループバック・リンクローカル・未指定の
	// 接続先と、記録した端末が持つアドレスへは作らない。影響の経路には含めない。
	EdgeKindTerminalOutboundConnection EdgeKind = "terminal_outbound_connection"
	// EdgeKindAccountIdentityMatch は、ドメインとログイン名で識別したアカウントのノードから、
	// セキュリティ識別子で識別したアカウントのノードへの、同じアカウントの候補である。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、同じ案件の中でセキュリティ
	// 識別子とドメインとログイン名を共に記録したレコードである。名前を記録したレコードの時刻の
	// 直前と直後の記録が同じセキュリティ識別子を指すときだけ結ぶ。削除と作り直しで識別子が
	// 変わった前後は、別の識別子へ結ぶ。
	EdgeKindAccountIdentityMatch EdgeKind = "account_identity_match"
	// EdgeKindInboundConnectionMatch は、着信の接続の許可を記録したレコードの接続元の端末から、
	// そのレコードを記録した端末への候補である。起点と終点はどちらも端末のノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、接続元のアドレスを分析者の
	// 割当が導いた端末と、レコードの時刻が割当の期間の中にあることだけである。port を問わない。
	// 接続の許可を遠隔のセッションの成立に読み替えない。割当が端末を導かない接続元からは作らない。
	EdgeKindInboundConnectionMatch EdgeKind = "inbound_connection_match"
	// EdgeKindSameConnectionMatch は、同じ 1 本の接続を記録した 2 件のレコードの候補である。
	// 起点と終点はどちらもレコードのノードであり、この関係で結ばれたレコードのノードの組が
	// 1 本の接続を指す。同じ端末の 2 件 (接続を開いた記録と閉じた記録) は時刻の早い記録から
	// 遅い記録へ、端末をまたぐ 2 件は接続した側の記録から受け付けた側の記録へ向く。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、接続元のアドレスと port、
	// 宛先の port の一致と、時刻の条件だけである。同じ端末で開閉を読める組は時刻の順序だけで
	// 結び、閉じた記録を、同じ 4 つ組でその前にある直近の開いた記録へ結ぶ。開閉を読めない組と
	// 端末をまたぐ組は時刻の許容幅で結ぶ。端末をまたぐ組の宛先のアドレスは、分析者の割当が
	// 受け付けた側の端末へ導くことで比べる。同じ端末で開閉を両方読める組は、port を使い直した
	// 別の接続を直近の開いた記録で分ける。それ以外の組は、時刻の許容幅の中で port を使い直した
	// 別の接続と分けられない。
	EdgeKindSameConnectionMatch EdgeKind = "same_connection_match"
	// EdgeKindRequestedSessionLogon は、端末 X のログオンのセッションから、そのセッションが資格情報を
	// 使って要求した、接続先の端末のログオンのレコードへの関係である。起点はセッションの始まりを
	// 決めたレコードのノード、終点はログオンのレコードのノードである。
	//
	// **確定した関係ではない。** 状態は candidate である。根拠は、要求の記録 (資格情報を使った
	// ログオンの要求) が操作を行ったセッションの Logon ID として起点のセッションの Logon ID を持ち、
	// 要求の時刻がセッションの期間の中にあることと、要求と終点のログオンが
	// EdgeKindExplicitCredentialLogon の条件 (アカウントの名前と時刻の範囲) を満たすことである。後者は
	// 候補の組であり、その組をレコードの組として根拠に出す。根拠のレコードは、セッションの始まりと
	// 終わりを決めた記録、要求の記録、終点のログオンである。
	EdgeKindRequestedSessionLogon EdgeKind = "requested_session_logon"
	// EdgeKindLogonChain は、端末 X のログオンのセッションから、X を接続元とするログオンのレコードへの
	// 候補である。起点はセッションの始まりを決めたレコードのノード、終点はログオンのレコードの
	// ノードである。
	//
	// **確定した関係ではない。** 根拠は、ログオンの接続元のアドレスを分析者の割当が X へ導くことと、
	// ログオンの時刻がセッションの期間の中にあることだけである。ログオフの記録が無いネットワークの
	// ログオン (種別 3) のセッションは、最後に記録された操作で終わる。匿名のログオン
	// (ANONYMOUS LOGON) と、画面の描画とフォントのドライバの仮想アカウント (SID S-1-5-90-*、
	// S-1-5-96-*) のセッションは候補にしない。1 件のログオンに候補のセッションが 1 つのとき
	// 状態は candidate、2 つ以上のとき全候補が uncertain_chain である。候補は、アカウントの一致と
	// ログオンの種別の区分で並べ、区分の条件と、並びでより上の区分に入る候補の数を組に出す
	// (EdgeCandidateTally)。EdgeKindRequestedSessionLogon が原因を決めたログオンへは作らない。
	// 根拠のレコードは、セッションの始まりと終わりを決めた記録と、終点のログオンである。
	EdgeKindLogonChain EdgeKind = "logon_chain"
)

// IsKnown は EdgeKind が定義の中の値であるかを返す。
func (k EdgeKind) IsKnown() bool {
	switch k {
	case EdgeKindRanOn, EdgeKindProcessParentChild, EdgeKindProcessInjection, EdgeKindFileOperation,
		EdgeKindFileCopy, EdgeKindProcessExecutable, EdgeKindFileContentMatch,
		EdgeKindRegistryOperation, EdgeKindProcessCommunication, EdgeKindTerminalAddress,
		EdgeKindTerminalRemoteSession, EdgeKindTerminalAccount, EdgeKindHttpRequest,
		EdgeKindCrossSourceConnectionMatch, EdgeKindRecordNamesObject,
		EdgeKindRecordSubjectAccount, EdgeKindRecordTargetAccount,
		EdgeKindLogonSessionOperation, EdgeKindArgumentNamesObject, EdgeKindTaskRegistrationRun,
		EdgeKindLinkedLogon, EdgeKindTicketRequestLogon, EdgeKindReverseLookupName,
		EdgeKindConnectionLogonMatch, EdgeKindProcessIdentityMatch,
		EdgeKindExplicitCredentialLogon, EdgeKindUnidentifiedSourceRemoteSession,
		EdgeKindTerminalOutboundConnection, EdgeKindAccountIdentityMatch, EdgeKindInboundConnectionMatch, EdgeKindSameConnectionMatch,
		EdgeKindRequestedSessionLogon, EdgeKindLogonChain:
		return true
	default:
		return false
	}
}

// NodeObservation はノードを記録したレコードの種類である。
//
// **束ねられない対象を別のノードのまま保持する。** 識別情報が足りない対象は、レコードに
// 結び付いた未同定の対象として保持する。参照だけのノードを、対象そのものを記録したレコードがあるノードと
// 同じ状態で出さない。
//
// **本型が表すのは「その対象を記録したレコードがあるか」の軸だけである。** 生成を記録した
// レコードがあるかは NodeCreationRecord が表す。2 つの軸は独立しており、生成のレコードを
// 持たないプロセスでも、通信やファイルのレコードがそのプロセスを記録していれば observed に
// なる。
type NodeObservation string

// NodeObservation の値。
const (
	// NodeObservationObserved は、その対象そのものを記録したレコードが記録したノードで
	// ある。
	NodeObservationObserved NodeObservation = "observed"
	// NodeObservationReferenced は、別の対象を記録したレコードが属性として参照しただけの
	// ノードである。値を記録したレコードは、その対象を記録していない。
	NodeObservationReferenced NodeObservation = "referenced"
)

// IsKnown は NodeObservation が定義の中の値であるかを返す。
func (o NodeObservation) IsKnown() bool {
	return o == NodeObservationObserved || o == NodeObservationReferenced
}

// NodeCreationRecord は対象の生成を記録したレコードが、そのノードの根拠にあるかである。
//
// **`observation` と別の軸である。** 生成のレコードを持つプロセスだけが、コマンド行と
// 起動時刻と親のプロセスを求められる。
type NodeCreationRecord string

// NodeCreationRecord の値。
const (
	// NodeCreationRecordPresent は生成を記録したレコードがあるノードである。
	NodeCreationRecordPresent NodeCreationRecord = "present"
	// NodeCreationRecordAbsent は生成を記録したレコードが 1 件も無いノードである。
	NodeCreationRecordAbsent NodeCreationRecord = "absent"
	// NodeCreationRecordItemAbsent は、生成を記録した根拠を持てない種別のノードである。
	// 軸そのものが該当しない状態を表す。
	NodeCreationRecordItemAbsent NodeCreationRecord = "item_absent"
)

// IsKnown は NodeCreationRecord が定義の中の値であるかを返す。
func (s NodeCreationRecord) IsKnown() bool {
	switch s {
	case NodeCreationRecordPresent, NodeCreationRecordAbsent, NodeCreationRecordItemAbsent:
		return true
	default:
		return false
	}
}

// NodeKeyForm は識別鍵の形である。
//
// 形は識別鍵の材料に入る。同じ値を持つ 2 つのノードでも、鍵の形が異なれば別のノードで
// ある。アカウントは 2 つの形を持つ。
type NodeKeyForm string

// NodeKeyForm の値。
const (
	// NodeKeyFormTerminalId は端末の外部識別子 1 つを鍵にする形である。
	NodeKeyFormTerminalId NodeKeyForm = "terminal_id"
	// NodeKeyFormTerminalProcess は端末の外部識別子とプロセスの外部識別子の組を鍵にする形である。
	NodeKeyFormTerminalProcess NodeKeyForm = "terminal_id_process_id"
	// NodeKeyFormTerminalFilePath は端末の外部識別子とファイルの path の組を鍵にする形である。
	NodeKeyFormTerminalFilePath NodeKeyForm = "terminal_id_file_path"
	// NodeKeyFormTerminalRegistryKeyPath は端末の外部識別子とレジストリキーの path の組を
	// 鍵にする形である。
	NodeKeyFormTerminalRegistryKeyPath NodeKeyForm = "terminal_id_registry_value_key_path"
	// NodeKeyFormAccountSid はアカウントのセキュリティ識別子 1 つを鍵にする形である。
	NodeKeyFormAccountSid NodeKeyForm = "account_sid"
	// NodeKeyFormAccountDomainName はドメインとログイン名の組を鍵にする形である。役割を付けて
	// 記録したアカウントでは、セキュリティ識別子の欄を持たないレコードが用いる。
	// **大文字と小文字をそろえず、ドメインの短い名前と FQDN を寄せない。**
	NodeKeyFormAccountDomainName NodeKeyForm = "account_domain_name"
	// NodeKeyFormAddress は IP アドレスの値 1 つを鍵にする形である。
	NodeKeyFormAddress NodeKeyForm = "address"
	// NodeKeyFormHostname は接続先のホスト名 1 つを鍵にする形である。
	NodeKeyFormHostname NodeKeyForm = "hostname"
	// NodeKeyFormSourceContentPosition は収集元の内容の識別と、その中の位置の組を鍵にする
	// 形である。レコードのノードが用いる。
	NodeKeyFormSourceContentPosition NodeKeyForm = "source_content_sha256_position"
	// NodeKeyFormTerminalProcessInterval は、端末の外部識別子とプロセス番号と、その番号の
	// 識別が始まった時刻の組を鍵にする形である。
	//
	// **プロセスへ一意な識別子を振らない収集元が用いる。** プロセス番号は OS が再利用する
	// ため、番号だけを鍵にすると別のプロセスが 1 つにまとめられる。一意な識別子を持たない対象の
	// 同一性は、識別が有効な区間で区切る。
	NodeKeyFormTerminalProcessInterval NodeKeyForm = "terminal_id_process_pid_interval"
	// NodeKeyFormRecordingSource は、収集元を記録した端末を、その収集元の内容の識別 1 つで
	// 鍵にする形である。
	//
	// **端末の外部識別子が分からない収集元の端末を指す。** 1 つのログのファイルは 1 台の
	// 端末が書いたものとして扱い、その端末の範囲でプロセスとファイルとアカウントを識別する。
	NodeKeyFormRecordingSource NodeKeyForm = "recording_source_content_sha256"
	// NodeKeyFormRecordingSourceHostname は、収集元の内容の識別と、その収集元のレコードが
	// 名乗った端末のホスト名の組で、端末を鍵にする形である。
	//
	// **1 つの file が複数台の端末の記録を持ちうる収集元の端末を指す。** ホスト名は端末の
	// 外部識別子ではないため、収集元をまたいで同じホスト名の端末を 1 つにまとめない。
	NodeKeyFormRecordingSourceHostname NodeKeyForm = "recording_source_content_sha256_hostname"
	// NodeKeyFormTerminalAccountName は、端末の鍵の値とログイン名の組を鍵にする形である。
	//
	// **ログイン名が収集元を記録した端末のアカウントを指す入力形式が用いる。** Linux の
	// 監査ログのログイン名は、その端末のアカウントである。
	NodeKeyFormTerminalAccountName NodeKeyForm = "terminal_id_account_name"
	// NodeKeyFormTerminalAddress は、端末の鍵の値と IP アドレスの組を鍵にする形である。
	//
	// **ループバックとリンクローカルのアドレスが用いる。** 2 つのアドレスは端末の中だけで
	// 意味を持ち、別の端末の同じ文字列は別のアドレスを指す。
	NodeKeyFormTerminalAddress NodeKeyForm = "terminal_id_address"
	// NodeKeyFormCollection は、端末 1 台から集めた収集の directory の端末を、収集の file の内容の
	// 識別から求めた値 1 つで鍵にする形である。
	//
	// **収集の file は 1 台の端末のものとして扱う。** registry、Prefetch と、その端末の名前を名乗る
	// イベントログのレコードを 1 つの端末に置く。値は内容だけから求め、取り込み 1 件を指す
	// sourceId を入れない。
	NodeKeyFormCollection NodeKeyForm = "collection_content_sha256"
)

// IsKnown は NodeKeyForm が定義の中の値であるかを返す。
func (f NodeKeyForm) IsKnown() bool {
	switch f {
	case NodeKeyFormTerminalId, NodeKeyFormTerminalProcess, NodeKeyFormTerminalFilePath,
		NodeKeyFormTerminalRegistryKeyPath, NodeKeyFormAccountSid,
		NodeKeyFormAccountDomainName, NodeKeyFormAddress, NodeKeyFormHostname,
		NodeKeyFormSourceContentPosition, NodeKeyFormTerminalProcessInterval,
		NodeKeyFormRecordingSource, NodeKeyFormRecordingSourceHostname,
		NodeKeyFormTerminalAccountName, NodeKeyFormTerminalAddress, NodeKeyFormCollection:
		return true
	default:
		return false
	}
}

// NodeKindOf は語彙の対象に対応するノードの種別を返す。
// ok が偽になるのは、ノードを作らない対象である。
//
// **ノードの種別の値は対象の値と同じ文字列である。** 2 つを対応させる表を持たない。
func NodeKindOf(object SemanticObject) (NodeKind, bool) {
	kind := NodeKind(object)
	if !kind.IsKnown() {
		return "", false
	}
	return kind, true
}

// LabelSemanticOf はノードの種別の表示名になる語彙の項目を返す。
//
// **語彙の役割から求める。** 役割が label の項目が表示名になる。種別ごとの項目の表を持たない。
//
// ok が偽になるのは、その対象に label の役割を持つ項目が語彙に無いときである。
// 対象は ip と domain と account で、3 つの表示名は識別鍵の最後の値になる。
func LabelSemanticOf(kind NodeKind) (SemanticKey, bool) {
	for key, meaning := range semanticVocabulary {
		if meaning.object == SemanticObject(kind) && meaning.role == SemanticRoleLabel {
			return key, true
		}
	}
	return "", false
}

// identitySemanticCount は対象の識別鍵に入る役割を持つ語彙の項目の個数を返す。
//
// 個数が 2 以上である対象では、1 つの値がどの項目から来たかを識別鍵が固定しない。
func identitySemanticCount(object SemanticObject) int {
	count := 0
	for _, meaning := range semanticVocabulary {
		if meaning.object == object && meaning.role == SemanticRoleIdentity {
			count++
		}
	}
	return count
}

// NodeIdentityValue は識別鍵の 1 項目の値である。
type NodeIdentityValue struct {
	// Semantic は鍵の項目の語彙の項目である。鍵の形が語彙の項目を 1 つに定めないとき出ない。
	Semantic SemanticKey `json:"semantic,omitempty"`
	// Value は鍵の項目の比べられる値である。
	Value string `json:"value"`
}

// Validate は全項目を確かめる。
func (v NodeIdentityValue) Validate() error {
	return firstProblem(
		requireKnownEnumWhenPresent("NodeIdentityValue.semantic", v.Semantic),
		requirePresent("NodeIdentityValue.value", v.Value),
	)
}

// NodeKey はノード 1 つの識別鍵である。
//
// **鍵の材料は原資料の文字列だけである。** 収集元の識別子 (sourceId) も解析実行への参照も
// 入らない。**同じ文字列を持つノードは、取り込み直しても解析実行が変わっても同じ鍵になる。**
type NodeKey struct {
	// Kind はノードの種別である。
	Kind NodeKind
	// Form は識別鍵の形である。
	Form NodeKeyForm
	// Values は鍵の値である。値ごとに、その値を記録した語彙の項目を持つ。
	Values []NodeIdentityValue
}

// DigestParts は識別鍵を、長さ前置のハッシュへ渡す文字列の並びへ直す。
// 先頭に種別と形を置くため、値が同じで種別または形が異なる鍵は別の文字列の並びになる。
//
// **並びに語彙の項目を入れない。** 同じ 1 つの IP アドレスを terminal.ip_address と
// connection.destination_address のどちらが指しても、同じ 1 つのノードを指すため
// である。
// **並びに収集元の識別子と解析実行への参照も入れない。**
func (k NodeKey) DigestParts() []string {
	parts := make([]string, 0, len(k.Values)+2)
	parts = append(parts, string(k.Kind), string(k.Form))
	for _, value := range k.Values {
		parts = append(parts, value.Value)
	}
	return parts
}

// Identity は識別鍵を応答の項目へ直す。
func (k NodeKey) Identity() []NodeIdentityValue {
	return append(make([]NodeIdentityValue, 0, len(k.Values)), k.Values...)
}

// LabelValue は識別鍵の最後の値を返す。表示名になる項目が語彙に無い対象の表示名である。
// ok が偽になるのは、値を 1 つも持たない鍵である。
func (k NodeKey) LabelValue() (string, bool) {
	if len(k.Values) == 0 {
		return "", false
	}
	return k.Values[len(k.Values)-1].Value, true
}

// NewRecordNodeKey はレコードの位置から、そのレコードのノードの識別鍵を組む。
//
// **鍵の材料は収集元の内容の識別と、その中の位置だけである。** 収集元の識別子 (sourceId) は
// 取り込み 1 件を指すため入らない。同じ file を取り込み直しても内容の識別と位置は変わらず、
// レコードのノードの識別子も変わらない (NodeKey の doc コメント)。
//
// **AssertionRecordRef が同じ材料でレコードを指す。** 分析者が付けた所見がレコードのノードを
// 指し続けられるのは、両方が「取り込みをやり直しても同じレコードを指す」という同じ要求から、
// 収集元の内容の識別と位置を選んでいるためである。片方の材料を変えるときは、もう片方も
// 同じ commit で変える。
//
// 位置の指し方を鍵に入れるのは、通番の 1 と行番号の 1 が別の位置であるためである。
//
// **位置の値が持つ語彙の項目は、通番のときだけ出る。** 行番号に対応する語彙の項目が
// 無いためである。鍵の識別子は語彙の項目を材料に
// 入れないため、出ない側でも同じ識別子になる。
//
// ok が偽になるのは、位置の指し方に対応する位置の値をレコードが持たないときである。
// **位置を確定できない状態を 0 で埋めない。**
func NewRecordNodeKey(locator RecordLocator) (NodeKey, bool) {
	var position *int64
	var semantic SemanticKey
	switch locator.PositionKind {
	case PositionKindSequenceNumber:
		position, semantic = locator.SequenceNumber, SemanticKeyRecordSequenceNumber
	case PositionKindLineNumber:
		position = locator.LineNumber
	case PositionKindByteRange:
		position = locator.ByteOffset
	default:
		return NodeKey{}, false
	}
	if position == nil || locator.SourceContentSha256 == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindRecord,
		Form: NodeKeyFormSourceContentPosition,
		Values: []NodeIdentityValue{
			{Value: locator.SourceContentSha256},
			{Value: string(locator.PositionKind)},
			{Semantic: semantic, Value: strconv.FormatInt(*position, 10)},
		},
	}, true
}

// NewProcessIntervalNodeKey は、識別が有効な区間で区切ったプロセスの識別鍵を組む。
//
// terminal は端末の識別鍵である。pid はプロセス番号の原文字列、intervalStart はその番号の
// 識別が始まった時刻の正規化値である。
//
// **プロセス番号だけで時間を跨いでまとめない。** 区間の始まりを
// 鍵に入れることで、番号が再利用された 2 つのプロセスが 2 つのノードになる。
//
// ok が偽になるのは、端末の識別鍵が端末を指さないときと、番号または区間の始まりが
// 空文字列のときである。**値を確定できない状態を空文字列で埋めない。**
func NewProcessIntervalNodeKey(terminal NodeKey, pid, intervalStart string) (NodeKey, bool) {
	if terminal.Kind != NodeKindTerminal || len(terminal.Values) == 0 {
		return NodeKey{}, false
	}
	if pid == "" || intervalStart == "" {
		return NodeKey{}, false
	}
	return NodeKey{
		Kind: NodeKindProcess,
		Form: NodeKeyFormTerminalProcessInterval,
		Values: terminalScopedValues(terminal,
			NodeIdentityValue{Semantic: SemanticKeyProcessPid, Value: pid},
			NodeIdentityValue{Value: intervalStart}),
	}, true
}

// terminalScopedValues は、端末の範囲に置く対象の鍵の値を、端末の鍵の値の全部に続けて並べる。
//
// **端末の鍵の値を 1 つに削らない。** 収集元の内容の識別とホスト名の組で指す端末では、
// 先頭の値だけを写すと、同じ収集元の別の端末の対象が 1 つのノードにまとめられる。値が 1 つの
// 端末の鍵では、先頭の値を写した鍵と同じ鍵になる。
func terminalScopedValues(terminal NodeKey, values ...NodeIdentityValue) []NodeIdentityValue {
	scoped := make([]NodeIdentityValue, 0, len(terminal.Values)+len(values))
	return append(append(scoped, terminal.Values...), values...)
}

// AddressValue は識別鍵が持つ IP アドレスを返す。
//
// ok が偽になるのは、鍵の形がアドレスでないときと、原資料の文字列をアドレスとして読めない
// ときである。**読めない文字列を既定のアドレスで埋めない。** 呼び出し元は、アドレスの範囲に
// 入るかを決められない対象として扱う。
//
// IPv4 を表す IPv6 の文字列は IPv4 のアドレスへ直す。同じ 1 つのアドレスを、文字列の形ごとに
// 別の範囲の判定へ渡さないためである。
//
// 端末の範囲に置いたアドレスの鍵 (NodeKeyFormTerminalAddress) も、最後の値のアドレスを返す。
func (k NodeKey) AddressValue() (netip.Addr, bool) {
	if k.Form != NodeKeyFormAddress && k.Form != NodeKeyFormTerminalAddress {
		return netip.Addr{}, false
	}
	text, present := k.LabelValue()
	if !present {
		return netip.Addr{}, false
	}
	address, err := netip.ParseAddr(text)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

// Validate は種別と形と値を確かめる。
func (k NodeKey) Validate() error {
	problem := firstProblem(
		requireKnownEnum("NodeKey.kind", k.Kind),
		requireKnownEnum("NodeKey.form", k.Form),
	)
	if problem != nil {
		return problem
	}
	if len(k.Values) == 0 {
		return itemError("NodeKey.values", ErrMissingRequiredItem)
	}
	return validateElements("NodeKey.values", k.Values)
}
