package core

// SemanticKey は入力形式をまたいで同じ意味を表す語彙の 1 項目である。値は
// `<対象>.<属性>` の形を持つ。
//
// 本 file の定数は項目の文字列を持つ。項目が意味を与える対象 (SemanticObject) と役割
// (SemanticRole) は semanticVocabulary が持つ。
type SemanticKey string

// SemanticObject は SemanticKey が意味を与える対象である。
type SemanticObject string

// SemanticObject の値。terminal から domain まではグラフのノードになる対象である。
// connection から http まではノードの間の関係と、関係を記録したレコードが持つ対象である。
const (
	// SemanticObjectTerminal は UCO Computer に対応する端末である。
	SemanticObjectTerminal SemanticObject = "terminal"
	// SemanticObjectProcess は UCO Process に対応するプロセスである。
	SemanticObjectProcess SemanticObject = "process"
	// SemanticObjectFile は UCO File に対応するファイルである。
	SemanticObjectFile SemanticObject = "file"
	// SemanticObjectRegistryValue は Windows レジストリの値である。
	SemanticObjectRegistryValue SemanticObject = "registry_value"
	// SemanticObjectAccount は UCO UserAccount に対応するアカウントである。
	SemanticObjectAccount SemanticObject = "account"
	// SemanticObjectIp は UCO IPAddress に対応する IP アドレスである。
	SemanticObjectIp SemanticObject = "ip"
	// SemanticObjectDomain は UCO DomainName に対応するホスト名である。
	SemanticObjectDomain SemanticObject = "domain"
	// SemanticObjectConnection は UCO NetworkConnection に対応する通信である。
	SemanticObjectConnection SemanticObject = "connection"
	// SemanticObjectEvent は UCO EventRecord に対応する事象である。
	SemanticObjectEvent SemanticObject = "event"
	// SemanticObjectRecord は収集元の中の 1 レコードである。
	SemanticObjectRecord SemanticObject = "record"
	// SemanticObjectHttp は UCO HTTPConnection に対応する HTTP の要求と応答である。
	SemanticObjectHttp SemanticObject = "http"
)

// SemanticRole は SemanticKey が対象に対して持つ役割である。
type SemanticRole string

// SemanticRole の値。
const (
	// SemanticRoleIdentity は対象のノードの識別鍵に入る項目である。同じ対象の
	// identity を集めた組がそのまま鍵になるとは限らない。鍵の組み方は NewRecordGraph が
	// 決める。
	SemanticRoleIdentity SemanticRole = "identity"
	// SemanticRoleLabel は分析者が読む表示名になる項目である。
	SemanticRoleLabel SemanticRole = "label"
	// SemanticRoleAttribute は識別鍵にも表示名にも入らない項目である。
	SemanticRoleAttribute SemanticRole = "attribute"
)

// 端末 (UCO Computer) の項目。
const (
	SemanticKeyTerminalId         SemanticKey = "terminal.id"
	SemanticKeyTerminalHostname   SemanticKey = "terminal.hostname"
	SemanticKeyTerminalIpAddress  SemanticKey = "terminal.ip_address"
	SemanticKeyTerminalSecurityId SemanticKey = "terminal.security_id"
)

// 端末の registry が記録した OS の項目。対象は端末である。
const (
	SemanticKeyTerminalOsProductName    SemanticKey = "terminal.os_product_name"
	SemanticKeyTerminalOsDisplayVersion SemanticKey = "terminal.os_display_version"
	SemanticKeyTerminalOsBuild          SemanticKey = "terminal.os_build"
	SemanticKeyTerminalOsUbr            SemanticKey = "terminal.os_ubr"
	SemanticKeyTerminalOsEdition        SemanticKey = "terminal.os_edition"
)

// プロセス (UCO Process) の項目。
const (
	SemanticKeyProcessId            SemanticKey = "process.id"
	SemanticKeyProcessPid           SemanticKey = "process.pid"
	SemanticKeyProcessBinaryPath    SemanticKey = "process.binary_path"
	SemanticKeyProcessCommandLine   SemanticKey = "process.command_line"
	SemanticKeyProcessStartTime     SemanticKey = "process.start_time"
	SemanticKeyProcessUserName      SemanticKey = "process.user_name"
	SemanticKeyProcessUserDomain    SemanticKey = "process.user_domain"
	SemanticKeyProcessWindowTitle   SemanticKey = "process.window_title"
	SemanticKeyProcessClipboardData SemanticKey = "process.clipboard_data"
	// SemanticKeyProcessDecodedCommandLine は、コマンド行に埋め込まれた符号化された
	// 文字列を復号した行である。原資料の文字列は process.command_line が保つ。
	SemanticKeyProcessDecodedCommandLine SemanticKey = "process.decoded_command_line"
	// SemanticKeyProcessShellCommand は、プロセスが実行した shell のコマンドである。
	SemanticKeyProcessShellCommand SemanticKey = "process.shell_command"
	// SemanticKeyProcessSessionId は、プロセスが属する利用者のログインセッションの
	// 識別子である。
	SemanticKeyProcessSessionId SemanticKey = "process.session_id"
)

// 親プロセスを指す項目。対象はプロセスである。
const (
	SemanticKeyParentProcessId         SemanticKey = "parent_process.id"
	SemanticKeyParentProcessPid        SemanticKey = "parent_process.pid"
	SemanticKeyParentProcessBinaryPath SemanticKey = "parent_process.binary_path"
)

// コードインジェクションの注入先のプロセスを指す項目。対象はプロセスである。
const (
	SemanticKeyInjectionTargetProcessId         SemanticKey = "injection_target_process.id"
	SemanticKeyInjectionTargetProcessBinaryPath SemanticKey = "injection_target_process.binary_path"
)

// プロセスが実行したファイルの内容と property を指す項目。対象はプロセスである。
//
// レコードは実行ファイルの path を process.binary_path で持ち、ファイルそのものを指す
// file.path を持たない。ファイルのノードを作れないため、内容と property をプロセスの
// 属性として持つ。
const (
	SemanticKeyProcessBinaryMd5               SemanticKey = "process_binary.md5"
	SemanticKeyProcessBinarySha1              SemanticKey = "process_binary.sha1"
	SemanticKeyProcessBinarySha256            SemanticKey = "process_binary.sha256"
	SemanticKeyProcessBinaryCompany           SemanticKey = "process_binary.company"
	SemanticKeyProcessBinaryCopyright         SemanticKey = "process_binary.copyright"
	SemanticKeyProcessBinaryDescription       SemanticKey = "process_binary.description"
	SemanticKeyProcessBinaryFileVersion       SemanticKey = "process_binary.file_version"
	SemanticKeyProcessBinaryProduct           SemanticKey = "process_binary.product"
	SemanticKeyProcessBinaryProductVersion    SemanticKey = "process_binary.product_version"
	SemanticKeyProcessBinarySigner            SemanticKey = "process_binary.signer"
	SemanticKeyProcessBinaryCertificateIssuer SemanticKey = "process_binary.certificate_issuer"
	SemanticKeyProcessBinarySignatureValidity SemanticKey = "process_binary.signature_validity"
	SemanticKeyProcessBinaryCreatedTime       SemanticKey = "process_binary.created_time"
	SemanticKeyProcessBinaryAccessedTime      SemanticKey = "process_binary.accessed_time"
	SemanticKeyProcessBinaryModifiedTime      SemanticKey = "process_binary.modified_time"
)

// 遠隔から端末を操作した側を指す項目。対象は端末である。
//
// 値が指すのは、レコードを記録した端末を遠隔から操作した別の端末とアカウントである。
// **記録した端末の同一性の鍵に入らない。**
const (
	SemanticKeyRemoteSessionClientHostname    SemanticKey = "remote_session.client_hostname"
	SemanticKeyRemoteSessionClientAddress     SemanticKey = "remote_session.client_address"
	SemanticKeyRemoteSessionClientAccountName SemanticKey = "remote_session.client_account_name"
)

// ファイル (UCO File) の項目。
const (
	SemanticKeyFilePath            SemanticKey = "file.path"
	SemanticKeyFileDestinationPath SemanticKey = "file.destination_path"
	SemanticKeyFileName            SemanticKey = "file.name"
	SemanticKeyFileSizeBytes       SemanticKey = "file.size_bytes"
	SemanticKeyFileCreatedTime     SemanticKey = "file.created_time"
	SemanticKeyFileModifiedTime    SemanticKey = "file.modified_time"
	SemanticKeyFileAccessedTime    SemanticKey = "file.accessed_time"
	SemanticKeyFileMd5             SemanticKey = "file.md5"
	SemanticKeyFileSha1            SemanticKey = "file.sha1"
	SemanticKeyFileSha256          SemanticKey = "file.sha256"
	// 実行ファイルの版の情報とインストーラが書く製品の名前と版の文字列、元の file 名、発行元と、
	// PE header が書くリンクの時刻。
	SemanticKeyFileProductName      SemanticKey = "file.product_name"
	SemanticKeyFileProductVersion   SemanticKey = "file.product_version"
	SemanticKeyFileOriginalFileName SemanticKey = "file.original_file_name"
	SemanticKeyFilePublisher        SemanticKey = "file.publisher"
	SemanticKeyFileLinkTime         SemanticKey = "file.link_time"
)

// Windows レジストリの値の項目。
const (
	SemanticKeyRegistryValueKeyPath  SemanticKey = "registry_value.key_path"
	SemanticKeyRegistryValueName     SemanticKey = "registry_value.name"
	SemanticKeyRegistryValueData     SemanticKey = "registry_value.data"
	SemanticKeyRegistryValueDataType SemanticKey = "registry_value.data_type"
)

// アカウント (UCO UserAccount) の項目。
const (
	SemanticKeyAccountSid    SemanticKey = "account.sid"
	SemanticKeyAccountName   SemanticKey = "account.name"
	SemanticKeyAccountDomain SemanticKey = "account.domain"
)

// 事象が役割を付けて記録したアカウントの項目。対象はアカウントである。
//
// **subject_account は事象が記録した操作を行ったアカウント、target_account は操作または
// ログオンの対象になったアカウントである。** 1 件のレコードが 2 つの役割を別のアカウントで
// 持つため、account.* にまとめない。役割は attribute であり、account の識別鍵に混ざらない。
// 2 つの役割のアカウントのノードと関係は NewRecordGraph が組む。
const (
	SemanticKeySubjectAccountSid    SemanticKey = "subject_account.sid"
	SemanticKeySubjectAccountName   SemanticKey = "subject_account.name"
	SemanticKeySubjectAccountDomain SemanticKey = "subject_account.domain"
	SemanticKeyTargetAccountSid     SemanticKey = "target_account.sid"
	SemanticKeyTargetAccountName    SemanticKey = "target_account.name"
	SemanticKeyTargetAccountDomain  SemanticKey = "target_account.domain"
)

// 通信 (UCO NetworkConnection) の項目。
const (
	SemanticKeyConnectionSourceAddress       SemanticKey = "connection.source_address"
	SemanticKeyConnectionSourcePort          SemanticKey = "connection.source_port"
	SemanticKeyConnectionDestinationAddress  SemanticKey = "connection.destination_address"
	SemanticKeyConnectionDestinationPort     SemanticKey = "connection.destination_port"
	SemanticKeyConnectionDestinationHostname SemanticKey = "connection.destination_hostname"
	// SemanticKeyConnectionDestinationReverseLookupName は、接続先のアドレスを収集元が逆引きした
	// 名前である。**要求したホスト名と別の項目にする。** 逆引きの名前は IP アドレスの持ち主が
	// 決め、ホスト名のノードを識別しない。名前のノードへは候補の関係だけを張る
	// (EdgeKindReverseLookupName)。
	SemanticKeyConnectionDestinationReverseLookupName SemanticKey = "connection.destination_reverse_lookup_name"
	// SemanticKeyConnectionDestinationServerName は、要求を記録した端末が接続先の端末を記録した
	// 名前である。ホスト名のノードを識別しない。接続先の端末は、この名前を記録した割当が導く。
	SemanticKeyConnectionDestinationServerName SemanticKey = "connection.destination_server_name"
	SemanticKeyConnectionProtocol              SemanticKey = "connection.protocol"
	SemanticKeyConnectionSentBytes             SemanticKey = "connection.sent_bytes"
	SemanticKeyConnectionReceivedBytes         SemanticKey = "connection.received_bytes"
)

// 事象 (UCO EventRecord) の項目。
const (
	SemanticKeyEventTime               SemanticKey = "event.time"
	SemanticKeyEventCategory           SemanticKey = "event.category"
	SemanticKeyEventAction             SemanticKey = "event.action"
	SemanticKeyEventMessage            SemanticKey = "event.message"
	SemanticKeyEventAccountName        SemanticKey = "event.account_name"
	SemanticKeyEventAccountDomain      SemanticKey = "event.account_domain"
	SemanticKeyEventOperationStartTime SemanticKey = "event.operation_start_time"
	// SemanticKeyEventReadBytes は、記録した操作が対象から読み出した byte 数である。
	SemanticKeyEventReadBytes SemanticKey = "event.read_bytes"
	// SemanticKeyEventWrittenBytes は、記録した操作が対象へ書き込んだ byte 数である。
	SemanticKeyEventWrittenBytes SemanticKey = "event.written_bytes"
)

// 事象が記録した、役割の項目では識別しない対象の項目。値はレコードの属性として検索に使う。
//
// share.name はアクセスされたネットワーク共有の名前、scheduled_task.name は登録された
// タスクの名前、started_task.name は起動されたタスクの名前、scheduled_task.command は登録
// されたタスクが実行するコマンド、service.name は登録されたサービスの名前であり、ノードを作らない。target_group はメンバーを追加された
// グループであり、target_account にまとめない。グループのノードはレコードのノードから対象を指す関係で結ぶ (record_graph.go の groupKeyOf)。
const (
	SemanticKeyShareName            SemanticKey = "share.name"
	SemanticKeyScheduledTaskName    SemanticKey = "scheduled_task.name"
	SemanticKeyStartedTaskName      SemanticKey = "started_task.name"
	SemanticKeyScheduledTaskCommand SemanticKey = "scheduled_task.command"
	SemanticKeyServiceName          SemanticKey = "service.name"
	SemanticKeyTargetGroupSid       SemanticKey = "target_group.sid"
	SemanticKeyTargetGroupName      SemanticKey = "target_group.name"
	SemanticKeyTargetGroupDomain    SemanticKey = "target_group.domain"
)

// 事象のうち、ログオンを記録した事象の項目。
//
// 値は Windows の定義が決めるコードである。原資料の文字列が別の形を持つ入力形式では、原資料の文字列の
// 項目を残し、コードは別の項目が持つ。
const (
	// SemanticKeyEventLogonType は、事象が記録したログオンの種別のコードである。
	SemanticKeyEventLogonType SemanticKey = "event.logon_type"
	// SemanticKeyEventLogonFailureStatus は、ログオンの失敗を記録した事象の、失敗の状態の
	// コードである。
	SemanticKeyEventLogonFailureStatus SemanticKey = "event.logon_failure_status"
	// SemanticKeyEventLogonFailureSubStatus は、失敗の状態を細かく分けたコードである。
	SemanticKeyEventLogonFailureSubStatus SemanticKey = "event.logon_failure_sub_status"
	// SemanticKeyEventOutboundAccountName は、ログオンが作ったセッションが他の端末への接続に
	// 使う資格情報のアカウントの名前である。**ログオンしたアカウントではない。** 対象を事象に
	// 置き、アカウントのノードを作らない。
	SemanticKeyEventOutboundAccountName SemanticKey = "event.outbound_account_name"
	// SemanticKeyEventOutboundAccountDomain は、同じ資格情報のアカウントのドメインである。
	SemanticKeyEventOutboundAccountDomain SemanticKey = "event.outbound_account_domain"
	// SemanticKeyEventAuthenticationPackage は、ログオンの認証に用いた方式の名前である
	// (Windows の AuthenticationPackageName。NTLM、Kerberos、Negotiate など)。
	SemanticKeyEventAuthenticationPackage SemanticKey = "event.authentication_package"
)

// 事象のうち、ログオンセッションを指す項目。値は Windows の Logon ID (ログオンセッションの
// LUID) である。
//
// **2 つの項目は関係の向きの両端である。** ログオンの成功が作ったセッションを
// event.target_logon_id が持ち、事象を記録した操作を行ったセッションを
// event.subject_logon_id が持つ。1 件のログオンのレコードが両方を持つ。要求したセッションと
// 作ったセッションが別であるため、1 つの項目にまとめない。
//
// **process.session_id と別の意味である。** process.session_id は端末の利用者のセッションの
// 番号であり、ログオンセッションを指さない。
const (
	// SemanticKeyEventTargetLogonId は、ログオンの成功を記録した事象が作ったログオン
	// セッションの Logon ID である。
	SemanticKeyEventTargetLogonId SemanticKey = "event.target_logon_id"
	// SemanticKeyEventSubjectLogonId は、事象が記録した操作を行ったログオンセッションの
	// Logon ID である。
	SemanticKeyEventSubjectLogonId SemanticKey = "event.subject_logon_id"
	// SemanticKeyEventTargetLinkedLogonId は、1 つのログオンが 2 つのトークンに分けて作った
	// ログオンセッションのうち、もう一方の Logon ID である。
	SemanticKeyEventTargetLinkedLogonId SemanticKey = "event.target_linked_logon_id"
	// SemanticKeyEventLogoffLogonId は、ログオフを記録した事象が終えたログオンセッションの
	// Logon ID である。
	SemanticKeyEventLogoffLogonId SemanticKey = "event.logoff_logon_id"
	// SemanticKeyEventSessionStartTime は、ログオフを記録した事象が書いた、終えたセッションの
	// 始まりの時刻である。
	SemanticKeyEventSessionStartTime SemanticKey = "event.session_start_time"
)

// 事象の項目のうち、Kerberos のチケットとログオンを結ぶ LogonGuid。原資料の文字列を保ち、比べるときは
// 前後の中括弧を外して大文字と小文字を区別しない。
const (
	// SemanticKeyEventTicketLogonGuid は、チケットの要求を記録した事象の LogonGuid である。
	SemanticKeyEventTicketLogonGuid SemanticKey = "event.ticket_logon_guid"
	// SemanticKeyEventTargetLogonGuid は、ログオンと、明示的な資格情報を使ったログオンの
	// 要求を記録した事象の、作ったログオンの LogonGuid である。
	SemanticKeyEventTargetLogonGuid SemanticKey = "event.target_logon_guid"
)

// 事象のうち、Windows イベントログが記録した事象の項目。値を持つのは Windows イベントログを
// 持つ入力形式である。
const (
	// SemanticKeyWindowsEventId は Windows イベントログのイベント ID である。
	SemanticKeyWindowsEventId SemanticKey = "windows_event.id"
	// SemanticKeyWindowsEventChannel はイベントを書いたチャネルの名前である。
	SemanticKeyWindowsEventChannel SemanticKey = "windows_event.channel"
	// SemanticKeyWindowsEventRecordId はチャネルの中でレコードを指す番号である。
	SemanticKeyWindowsEventRecordId SemanticKey = "windows_event.record_id"
	// SemanticKeyWindowsEventProvider はイベントを書いたプロバイダの名前である。
	SemanticKeyWindowsEventProvider SemanticKey = "windows_event.provider"
)

// 収集元の中のレコードの項目。
const (
	SemanticKeyRecordSequenceNumber SemanticKey = "record.sequence_number"
)

// HTTP (UCO HTTPConnection) の項目。
const (
	SemanticKeyHttpRequestMethod     SemanticKey = "http.request_method"
	SemanticKeyHttpRequestUrl        SemanticKey = "http.request_url"
	SemanticKeyHttpRequestDecodedUrl SemanticKey = "http.request_decoded_url"
	SemanticKeyHttpRequestVersion    SemanticKey = "http.request_version"
	SemanticKeyHttpRequestBytes      SemanticKey = "http.request_bytes"
	SemanticKeyHttpResponseBytes     SemanticKey = "http.response_bytes"
	SemanticKeyHttpStatusCode        SemanticKey = "http.status_code"
	SemanticKeyHttpUserAgent         SemanticKey = "http.user_agent"
	SemanticKeyHttpReferrer          SemanticKey = "http.referrer"
)

// semanticMeaning は 1 つの SemanticKey の対象と役割である。
type semanticMeaning struct {
	object SemanticObject
	role   SemanticRole
}

// semanticVocabulary は語彙の全項目と、項目ごとの対象と役割である。
var semanticVocabulary = map[SemanticKey]semanticMeaning{
	SemanticKeyTerminalId:         {SemanticObjectTerminal, SemanticRoleIdentity},
	SemanticKeyTerminalHostname:   {SemanticObjectTerminal, SemanticRoleLabel},
	SemanticKeyTerminalIpAddress:  {SemanticObjectIp, SemanticRoleIdentity},
	SemanticKeyTerminalSecurityId: {SemanticObjectTerminal, SemanticRoleAttribute},

	SemanticKeyTerminalOsProductName:    {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyTerminalOsDisplayVersion: {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyTerminalOsBuild:          {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyTerminalOsUbr:            {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyTerminalOsEdition:        {SemanticObjectTerminal, SemanticRoleAttribute},

	SemanticKeyProcessId:                 {SemanticObjectProcess, SemanticRoleIdentity},
	SemanticKeyProcessPid:                {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryPath:         {SemanticObjectProcess, SemanticRoleLabel},
	SemanticKeyProcessCommandLine:        {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessStartTime:          {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessUserName:           {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessUserDomain:         {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessWindowTitle:        {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessClipboardData:      {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessDecodedCommandLine: {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessShellCommand:       {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessSessionId:          {SemanticObjectProcess, SemanticRoleAttribute},

	SemanticKeyParentProcessId:         {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyParentProcessPid:        {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyParentProcessBinaryPath: {SemanticObjectProcess, SemanticRoleAttribute},

	SemanticKeyInjectionTargetProcessId:         {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyInjectionTargetProcessBinaryPath: {SemanticObjectProcess, SemanticRoleAttribute},

	SemanticKeyProcessBinaryMd5:               {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinarySha1:              {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinarySha256:            {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryCompany:           {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryCopyright:         {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryDescription:       {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryFileVersion:       {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryProduct:           {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryProductVersion:    {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinarySigner:            {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryCertificateIssuer: {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinarySignatureValidity: {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryCreatedTime:       {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryAccessedTime:      {SemanticObjectProcess, SemanticRoleAttribute},
	SemanticKeyProcessBinaryModifiedTime:      {SemanticObjectProcess, SemanticRoleAttribute},

	SemanticKeyRemoteSessionClientHostname:    {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyRemoteSessionClientAddress:     {SemanticObjectTerminal, SemanticRoleAttribute},
	SemanticKeyRemoteSessionClientAccountName: {SemanticObjectTerminal, SemanticRoleAttribute},

	SemanticKeyFilePath:             {SemanticObjectFile, SemanticRoleIdentity},
	SemanticKeyFileDestinationPath:  {SemanticObjectFile, SemanticRoleIdentity},
	SemanticKeyFileName:             {SemanticObjectFile, SemanticRoleLabel},
	SemanticKeyFileSizeBytes:        {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileCreatedTime:      {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileModifiedTime:     {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileAccessedTime:     {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileMd5:              {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileSha1:             {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileSha256:           {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileProductName:      {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileProductVersion:   {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileOriginalFileName: {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFilePublisher:        {SemanticObjectFile, SemanticRoleAttribute},
	SemanticKeyFileLinkTime:         {SemanticObjectFile, SemanticRoleAttribute},

	SemanticKeyRegistryValueKeyPath:  {SemanticObjectRegistryValue, SemanticRoleIdentity},
	SemanticKeyRegistryValueName:     {SemanticObjectRegistryValue, SemanticRoleLabel},
	SemanticKeyRegistryValueData:     {SemanticObjectRegistryValue, SemanticRoleAttribute},
	SemanticKeyRegistryValueDataType: {SemanticObjectRegistryValue, SemanticRoleAttribute},

	SemanticKeyAccountSid:    {SemanticObjectAccount, SemanticRoleIdentity},
	SemanticKeyAccountName:   {SemanticObjectAccount, SemanticRoleIdentity},
	SemanticKeyAccountDomain: {SemanticObjectAccount, SemanticRoleIdentity},

	SemanticKeySubjectAccountSid:    {SemanticObjectAccount, SemanticRoleAttribute},
	SemanticKeySubjectAccountName:   {SemanticObjectAccount, SemanticRoleAttribute},
	SemanticKeySubjectAccountDomain: {SemanticObjectAccount, SemanticRoleAttribute},
	SemanticKeyTargetAccountSid:     {SemanticObjectAccount, SemanticRoleAttribute},
	SemanticKeyTargetAccountName:    {SemanticObjectAccount, SemanticRoleAttribute},
	SemanticKeyTargetAccountDomain:  {SemanticObjectAccount, SemanticRoleAttribute},

	SemanticKeyConnectionSourceAddress:                {SemanticObjectIp, SemanticRoleIdentity},
	SemanticKeyConnectionSourcePort:                   {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionDestinationAddress:           {SemanticObjectIp, SemanticRoleIdentity},
	SemanticKeyConnectionDestinationPort:              {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionDestinationHostname:          {SemanticObjectDomain, SemanticRoleIdentity},
	SemanticKeyConnectionDestinationReverseLookupName: {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionDestinationServerName:        {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionProtocol:                     {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionSentBytes:                    {SemanticObjectConnection, SemanticRoleAttribute},
	SemanticKeyConnectionReceivedBytes:                {SemanticObjectConnection, SemanticRoleAttribute},

	SemanticKeyEventTime:               {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventCategory:           {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventAction:             {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventMessage:            {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventAccountName:        {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventAccountDomain:      {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventOperationStartTime: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventReadBytes:          {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventWrittenBytes:       {SemanticObjectEvent, SemanticRoleAttribute},

	SemanticKeyShareName:            {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyScheduledTaskName:    {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyStartedTaskName:      {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyScheduledTaskCommand: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyServiceName:          {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyTargetGroupSid:       {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyTargetGroupName:      {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyTargetGroupDomain:    {SemanticObjectEvent, SemanticRoleAttribute},

	SemanticKeyEventLogonType:             {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventLogonFailureStatus:    {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventLogonFailureSubStatus: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventOutboundAccountName:   {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventOutboundAccountDomain: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventAuthenticationPackage: {SemanticObjectEvent, SemanticRoleAttribute},

	SemanticKeyEventTargetLogonId:       {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventSubjectLogonId:      {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventTargetLinkedLogonId: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventLogoffLogonId:       {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventSessionStartTime:    {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventTicketLogonGuid:     {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyEventTargetLogonGuid:     {SemanticObjectEvent, SemanticRoleAttribute},

	SemanticKeyWindowsEventId:       {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyWindowsEventChannel:  {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyWindowsEventRecordId: {SemanticObjectEvent, SemanticRoleAttribute},
	SemanticKeyWindowsEventProvider: {SemanticObjectEvent, SemanticRoleAttribute},

	SemanticKeyRecordSequenceNumber: {SemanticObjectRecord, SemanticRoleAttribute},

	SemanticKeyHttpRequestMethod:     {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpRequestUrl:        {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpRequestDecodedUrl: {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpRequestVersion:    {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpRequestBytes:      {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpResponseBytes:     {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpStatusCode:        {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpUserAgent:         {SemanticObjectHttp, SemanticRoleAttribute},
	SemanticKeyHttpReferrer:          {SemanticObjectHttp, SemanticRoleAttribute},
}

// IsKnown は SemanticKey が語彙の中にあるかを返す。
func (k SemanticKey) IsKnown() bool {
	_, found := semanticVocabulary[k]
	return found
}

// Object は SemanticKey が意味を与える対象を返す。語彙の外の値には空の
// SemanticObject を返す。
func (k SemanticKey) Object() SemanticObject {
	return semanticVocabulary[k].object
}

// Role は SemanticKey が対象に対して持つ役割を返す。語彙の外の値には空の
// SemanticRole を返す。
func (k SemanticKey) Role() SemanticRole {
	return semanticVocabulary[k].role
}
