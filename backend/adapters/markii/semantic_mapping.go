package markii

import (
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 対応表に載る key は語彙の項目を semantic に持ち、表に載らない key は markii 形式に固有の
// 意味を持つ。時刻の正規化と kind の選択は fields.go が担い、対応表は key が表す意味だけを選ぶ。

// 対応表と宣言が使う key の文字列。原資料の key をそのまま使う。
//
// `channel` と `evtRecID` は windows_event.* を持ち、TranscriptIdentityItems が転記の同一性を
// 決める欄として名前で挙げる (format.go)。
const (
	keyCommandLine       = "cmd"
	keyProcessId         = "psID"
	keyParentPath        = "parentPath"
	keyProcessUser       = "psUser"
	keyProcessDomain     = "psDomain"
	keyUser              = "usr"
	keyUserDomain        = "usrDomain"
	keyEventChannel      = "channel"
	keyEventRecordId     = "evtRecID"
	keyEventId           = "evtID"
	keyEventSource       = "evtSrc"
	keyDestHost          = "dstHost"
	keyPath              = "path"
	keyDestinationPath   = "dstPath"
	keyFileSize          = "size"
	keyFileSha256        = "sha256"
	keyFileSha1          = "sha1"
	keyFileMd5           = "md5"
	keyRegistryEntry     = "entry"
	keyRegistryValueType = "valType"
	keyRegistryValueStr  = "valStr"
	keyRegistryValueNum  = "valNum"
	keyUrl               = "url"
	keyDecodedUrl        = "decode"
	keyUrlHostname       = "url_hostname"
	keyClipboardData     = "clipData"
	keyWindowTitle       = "winTitle"
	keyEventMessage      = "evtMsg"
	keyEventUser         = "evtUsr"
	keyEventDomain       = "evtDomain"
	keyStartTime         = "sTime"
	keyTargetProcessGuid = "tpsGUID"
	keyTargetProcessPath = "tpsPath"
	keyWorkstationIp     = "wsIp"
	keyWorkstationPort   = "wsPort"
	keySessionId         = "sessionID"
	keyShellCommand      = "shCmd"
	keyReadBytes         = "read"
	keyWrittenBytes      = "write"
)

// 実行ファイルまたは操作対象ファイルの内容と property を持つ key の文字列。
// どちらのファイルを指すかは evt が決める。
const (
	keyCompany           = "company"
	keyCopyright         = "copyright"
	keyFileDescription   = "fileDesc"
	keyFileVersion       = "fileVer"
	keyProduct           = "product"
	keyProductVersion    = "productVer"
	keySigner            = "signer"
	keyCertificateIssuer = "issuer"
	keySignatureValidity = "sig"
	keyCreatedTime       = "crTime"
	keyAccessedTime      = "acTime"
	keyModifiedTime      = "moTime"
)

// 遠隔から端末を操作した側を持つ key の文字列。
const (
	keyRemoteConsoleHostname = "rcCom"
	keyRemoteConsoleIp       = "rcIP"
	keySourceHostname        = "srcCom"
	keyWorkstationName       = "wsName"
	keyClientHostname        = "cliCom"
	keyClientUser            = "cliUsr"
)

// 対応表が evt の文字列で意味を分けるレコードの種別。
const (
	fileEvent     = "file"
	registryEvent = "reg"
	sessionEvent  = "session"
	osEvent       = "os"
)

// eventScopedKey は evt と組にして意味を決める key の鍵である。
type eventScopedKey struct {
	key   string
	event string
}

// semanticByEventAndKey は evt と組で意味が決まる key の対応である。
//
// path は evt が file のレコードで操作したファイルを、evt が reg のレコードで操作した
// レジストリキーを指す。size は path が指すファイルの大きさであり、file のレコードだけが持つ。
//
// sha256 と sha1 と md5 は、evt が file のレコードで path が指すファイルの内容の hash を持ち、
// evt が ps のレコードで psPath の実行ファイルの hash を持つ。ps のレコードはファイルを指す
// path を持たないため、hash は process_binary.* へ写してプロセスの属性にする。値は引用符を
// 持たない小文字 16 進であり、hash の項目が比べる形をそのまま満たすため正規化値を作らない。
//
// company / copyright / fileDesc / fileVer / product / productVer / signer / issuer / sig /
// crTime / acTime / moTime も、evt が ps のレコードで psPath の実行ファイルを指し、evt が
// file のレコードで path が指すファイルを指す。
//
// 既知の制限: evt が reg のレコードで、ファイルの内容と property を持つ key に意味を与えない,
// 値は entry が ImagePath である valStr が指すファイルのものであり、valStr は
// `%SystemRoot%\example.exe` のような環境変数の参照や `example.exe /n` のような相対 path と
// 引数を含むため、収集した端末の環境を持たない本 package が file.path へ直せない,
// valStr が指すファイルの path を収集元の情報だけで決められるようになったとき、
// 対象の key を file.* へ写す
//
// wsIp と wsPort は、evt が os のレコードで Windows イベントログが記録した接続元の
// アドレスと port である。srcIP と srcPort は net と session と os のレコードで接続元を持つ。
// 4 つとも接続元の項目へ写す。1 レコードは wsIp と srcIP を同時に持たない。
// rcIP は srcIP と同じレコードに現れることがあるため、remote_session.client_address へ写す
// (semanticByKey)。session と os の値は引用符付きで、引用符を外した文字列が正規化値になる。
//
// read と write は、evt が file のレコードで、path が指すファイルをプロセスが開いてから
// 閉じるまでに読んだ byte 数と書いた byte 数である。記録した操作の属性であり、ファイルノードの
// 属性にしない。
//
// `url` は webURL のレコードの要求先 URL、`decode` は `url` をパーセント復号した文字列、
// `url_hostname` は要求先のホスト名である。原資料の文字列と復号済みの文字列を混ぜないため、
// `url` と `decode` を別の語彙項目へ写す。`url_hostname` は接続先の domain node を指す。
//
// 既知の制限: `url_hostname` の意味を key 名から対応付ける,
// repo の中に `url_hostname` を持つレコードの例が無く、出力と host の対応を確かめられない,
// `url_hostname` を持つレコードで出力と host の対応を確認できたとき意味を再検討する
//
// sTime は ps でプロセス開始時刻、file で操作開始時刻を持つ。crTime と acTime と moTime は
// file で file.*_time、ps で process_binary.*_time になる。3 つはファイル自身の property であり、
// レコードのヘッダーの時刻と別の時刻である (fields.go の timestampSemanticMeanings)。
//
// tpsGUID と tpsPath は、evt が ps のレコードでコードインジェクションの注入先のプロセスを持つ。
// tpsGUID は psGUID と同じ中括弧付きの GUID の形である。注入元は同じレコードの psGUID である。
//
// 既知の制限: api に共通の意味を与えず、markii 形式に固有の key として保持する,
// 注入に使った API の名前を持つ語彙の項目が無い,
// 注入に使った API の名前を持つ語彙の項目を置いたとき、表に足す
var semanticByEventAndKey = map[eventScopedKey]core.SemanticKey{
	{key: keyPath, event: fileEvent}:                      core.SemanticKeyFilePath,
	{key: keyDestinationPath, event: fileEvent}:           core.SemanticKeyFileDestinationPath,
	{key: keyPath, event: registryEvent}:                  core.SemanticKeyRegistryValueKeyPath,
	{key: keyFileSize, event: fileEvent}:                  core.SemanticKeyFileSizeBytes,
	{key: keyFileSha256, event: fileEvent}:                core.SemanticKeyFileSha256,
	{key: keyFileSha1, event: fileEvent}:                  core.SemanticKeyFileSha1,
	{key: keyFileMd5, event: fileEvent}:                   core.SemanticKeyFileMd5,
	{key: keySourceIp, event: communicationEvent}:         core.SemanticKeyConnectionSourceAddress,
	{key: keySourcePort, event: communicationEvent}:       core.SemanticKeyConnectionSourcePort,
	{key: keySourceIp, event: sessionEvent}:               core.SemanticKeyConnectionSourceAddress,
	{key: keySourcePort, event: sessionEvent}:             core.SemanticKeyConnectionSourcePort,
	{key: keySourceIp, event: osEvent}:                    core.SemanticKeyConnectionSourceAddress,
	{key: keySourcePort, event: osEvent}:                  core.SemanticKeyConnectionSourcePort,
	{key: keyWorkstationIp, event: osEvent}:               core.SemanticKeyConnectionSourceAddress,
	{key: keyWorkstationPort, event: osEvent}:             core.SemanticKeyConnectionSourcePort,
	{key: keyReadBytes, event: fileEvent}:                 core.SemanticKeyEventReadBytes,
	{key: keyWrittenBytes, event: fileEvent}:              core.SemanticKeyEventWrittenBytes,
	{key: keyCreatedTime, event: fileEvent}:               core.SemanticKeyFileCreatedTime,
	{key: keyAccessedTime, event: fileEvent}:              core.SemanticKeyFileAccessedTime,
	{key: keyModifiedTime, event: fileEvent}:              core.SemanticKeyFileModifiedTime,
	{key: keyFileSha256, event: processStartEvent}:        core.SemanticKeyProcessBinarySha256,
	{key: keyFileSha1, event: processStartEvent}:          core.SemanticKeyProcessBinarySha1,
	{key: keyFileMd5, event: processStartEvent}:           core.SemanticKeyProcessBinaryMd5,
	{key: keyCompany, event: processStartEvent}:           core.SemanticKeyProcessBinaryCompany,
	{key: keyCopyright, event: processStartEvent}:         core.SemanticKeyProcessBinaryCopyright,
	{key: keyFileDescription, event: processStartEvent}:   core.SemanticKeyProcessBinaryDescription,
	{key: keyFileVersion, event: processStartEvent}:       core.SemanticKeyProcessBinaryFileVersion,
	{key: keyProduct, event: processStartEvent}:           core.SemanticKeyProcessBinaryProduct,
	{key: keyProductVersion, event: processStartEvent}:    core.SemanticKeyProcessBinaryProductVersion,
	{key: keySigner, event: processStartEvent}:            core.SemanticKeyProcessBinarySigner,
	{key: keyCertificateIssuer, event: processStartEvent}: core.SemanticKeyProcessBinaryCertificateIssuer,
	{key: keySignatureValidity, event: processStartEvent}: core.SemanticKeyProcessBinarySignatureValidity,
	{key: keyCreatedTime, event: processStartEvent}:       core.SemanticKeyProcessBinaryCreatedTime,
	{key: keyAccessedTime, event: processStartEvent}:      core.SemanticKeyProcessBinaryAccessedTime,
	{key: keyModifiedTime, event: processStartEvent}:      core.SemanticKeyProcessBinaryModifiedTime,
	{key: keyUrl, event: communicationEvent}:              core.SemanticKeyHttpRequestUrl,
	{key: keyDecodedUrl, event: communicationEvent}:       core.SemanticKeyHttpRequestDecodedUrl,
	{key: keyUrlHostname, event: communicationEvent}:      core.SemanticKeyConnectionDestinationHostname,
	{key: keyStartTime, event: processStartEvent}:         core.SemanticKeyProcessStartTime,
	{key: keyTargetProcessGuid, event: processStartEvent}: core.SemanticKeyInjectionTargetProcessId,
	{key: keyTargetProcessPath, event: processStartEvent}: core.SemanticKeyInjectionTargetProcessBinaryPath,
	{key: keyStartTime, event: fileEvent}:                 core.SemanticKeyEventOperationStartTime,
}

// semanticByKey は evt に依らず意味が決まる key の対応である。
//
// valStr と valNum は同じ registry_value.data を持つ。valType が文字列の型なら valStr、
// 整数の型なら valNum を持ち、REG_BINARY はどちらも持たない。**排他を構造として防いでいない。**
// 2 つを同時に持つレコードは registry_value.data を 2 件持つ。
//
// `clipData` と `winTitle` は process node の属性として出す。`evtMsg` は Windows イベントの
// 本文、`evtUsr` / `evtDomain` はイベントが記録したアカウントであり、どちらも値の不在と空文字を
// valueOfField の状態に残す。
//
// `rcCom` / `rcIP` / `srcCom` / `wsName` / `cliCom` / `cliUsr` は、レコードを記録した端末を
// 遠隔から操作した側のホスト名、アドレス、アカウント名を持つ。ホスト名を持つ 4 つを 1 つの
// 項目へ写すのは、1 レコードが 4 つのうち 2 つ以上を同時に持たないためである。
// `rcIP` は `srcIP` と同じレコードに現れることがあり、両方を接続元のアドレスへ写すと
// 通信の接続元を 1 つに決められないため、connection.source_address へ写さない。
//
// `sessionID` はログインしている利用者のユーザーセッション ID、`shCmd` は実行した PowerShell の
// コマンドであり、プロセスの属性へ写す。**`shCmd` を `process.command_line` へ写さない。**
// コマンド行の全体と、実行した 1 つのコマンドは別の値である。
//
// `channel` / `evtID` / `evtRecID` / `evtSrc` は、Windows イベントログのレコードを写した行の
// チャネル、イベント ID、チャネルの中のレコード番号、イベントを書いたプロバイダである。
//
// 既知の制限: `evtID` と `evtRecID` の先頭の 0 を外す正規化値を作らない,
// 2 つの値は Windows の 10 進の数字を引用符なしで書いたものであり、先頭に 0 を置く値の例が無い,
// 先頭に 0 を置く値を収集元で確認したとき、正規化値を足す
//
// 既知の制限: `evtUsr` / `evtDomain` を account の識別項目にせず event の属性へ写す,
// evtUsr だけを持つレコードがあり、usr と同じアカウントかを原資料から決められないため、
// 通常アカウントの鍵へ混ぜると値の違う一方を失う,
// 両者の同一性を確認し、複数のアカウント識別値を応答が保持できる設計に
// なったとき、account の識別項目への変更を再検討する
var semanticByKey = map[string]core.SemanticKey{
	keySequenceNumber:    core.SemanticKeyRecordSequenceNumber,
	keyEvent:             core.SemanticKeyEventCategory,
	keySubEvent:          core.SemanticKeyEventAction,
	keyTerminalId:        core.SemanticKeyTerminalId,
	keyComputerName:      core.SemanticKeyTerminalHostname,
	keySecurityId:        core.SemanticKeyTerminalSecurityId,
	keyProcessGuid:       core.SemanticKeyProcessId,
	keyProcessId:         core.SemanticKeyProcessPid,
	keyProcessPath:       core.SemanticKeyProcessBinaryPath,
	keyCommandLine:       core.SemanticKeyProcessCommandLine,
	keyClipboardData:     core.SemanticKeyProcessClipboardData,
	keyWindowTitle:       core.SemanticKeyProcessWindowTitle,
	keyEventMessage:      core.SemanticKeyEventMessage,
	keyEventUser:         core.SemanticKeyEventAccountName,
	keyEventDomain:       core.SemanticKeyEventAccountDomain,
	keyProcessUser:       core.SemanticKeyProcessUserName,
	keyProcessDomain:     core.SemanticKeyProcessUserDomain,
	keyParentGuid:        core.SemanticKeyParentProcessId,
	keyParentPath:        core.SemanticKeyParentProcessBinaryPath,
	keyUser:              core.SemanticKeyAccountName,
	keyUserDomain:        core.SemanticKeyAccountDomain,
	keyDestIp:            core.SemanticKeyConnectionDestinationAddress,
	keyDestPort:          core.SemanticKeyConnectionDestinationPort,
	keyDestHost:          core.SemanticKeyConnectionDestinationHostname,
	keyReceivedBytes:     core.SemanticKeyConnectionReceivedBytes,
	keySentBytes:         core.SemanticKeyConnectionSentBytes,
	keyRegistryEntry:     core.SemanticKeyRegistryValueName,
	keyRegistryValueType: core.SemanticKeyRegistryValueDataType,
	keyRegistryValueStr:  core.SemanticKeyRegistryValueData,
	keyRegistryValueNum:  core.SemanticKeyRegistryValueData,
	keySessionId:         core.SemanticKeyProcessSessionId,
	keyShellCommand:      core.SemanticKeyProcessShellCommand,
	keyEventLogonId:      core.SemanticKeyEventSubjectLogonId,

	keyEventChannel:  core.SemanticKeyWindowsEventChannel,
	keyEventId:       core.SemanticKeyWindowsEventId,
	keyEventRecordId: core.SemanticKeyWindowsEventRecordId,
	keyEventSource:   core.SemanticKeyWindowsEventProvider,

	keyRemoteConsoleHostname: core.SemanticKeyRemoteSessionClientHostname,
	keySourceHostname:        core.SemanticKeyRemoteSessionClientHostname,
	keyWorkstationName:       core.SemanticKeyRemoteSessionClientHostname,
	keyClientHostname:        core.SemanticKeyRemoteSessionClientHostname,
	keyRemoteConsoleIp:       core.SemanticKeyRemoteSessionClientAddress,
	keyClientUser:            core.SemanticKeyRemoteSessionClientAccountName,
}

// semanticOf は 1 つの key の共通の意味を返す。対応表に無い key には空の値を返す。
//
// evt と組にした対応を先に探す。evt を持たないレコードでは event が空の文字列になり、
// evt に依らない対応だけを探す。空の値は「この入力形式に固有の意味を持つ」という主張になる。
//
// 既知の制限: 応答の fields に入る ip に terminal.ip_address を与えない,
// terminal.ip_address の項目は 1 つの値に 1 つのアドレスを置いて比べるが、ip は複数の
// アドレスをカンマで連結した 1 個の文字列である。1 つのアドレスごとに分けた項目は
// TerminalFields が別に返し、IP から端末への割当がそれを読む,
// 応答の fields が 1 つのアドレスごとに項目を分けて持つ形になったとき、表に足す
//
// dstPath はコピーと名前変更の行先であり、file.destination_path へ写す。path と同じ
// file.path にすると、1 レコードのコピー元とコピー先を区別できず、方向の関係と内容
// 一致候補を組めない。
func semanticOf(key, event string) core.SemanticKey {
	if semantic, found := semanticByEventAndKey[eventScopedKey{key: key, event: event}]; found {
		return semantic
	}
	return semanticByKey[key]
}

// Logon ID を持つ key と、Windows のイベント ID の文字列。
const (
	keyEventLogonId        = "evtLogonID"
	keyWindowsEventId      = "evtID"
	windowsEventLogonLogon = "4624"
)

// windowsEventScopedKey は、Windows のイベント ID と組にして意味を決める key の鍵である。
type windowsEventScopedKey struct {
	key     string
	eventID string
}

// semanticByWindowsEventAndKey は、evt が os のレコードで、Windows のイベント ID と組で意味が
// 決まる key の対応である。semanticOf の対応より先に探す。
//
// evtLogonID は Windows のログオン ID を 16 進で書いた値である。ログオンの成功 (4624) の値は
// ログオンが作ったセッション、特権の割り当て (4672) の値は特権を割り当てたセッションであり、
// 4624 の値は同じセッションの後続の 4672 の値と一致する。ログオフ (4634 と 4647) の値は
// 終えたセッションであり、Windows イベントログの 4634 の TargetLogonId と同じく
// event.logoff_logon_id へ写す。ほかのイベントの値は、操作を行ったセッションとして
// semanticByKey が持つ。
var semanticByWindowsEventAndKey = map[windowsEventScopedKey]core.SemanticKey{
	{key: keyEventLogonId, eventID: windowsEventLogonLogon}: core.SemanticKeyEventTargetLogonId,
	{key: keyEventLogonId, eventID: "4634"}:                 core.SemanticKeyEventLogoffLogonId,
	{key: keyEventLogonId, eventID: "4647"}:                 core.SemanticKeyEventLogoffLogonId,
}

// subEventScopedKey は、evt と subEvt の組と key で意味を決める key の鍵である。
type subEventScopedKey struct {
	key, event, subEvent string
}

// semanticBySubEventAndKey は、evt と subEvt の組で意味が決まる key の対応である。
// recordSemanticOf が最初に探す。
//
// ログアウト (session の logout) の sTime は、終えたセッションの始まりの時刻である。
// 操作中断の解除とスクリーンセーバーの終了の sTime は中断の始まりを持つため、表に載せない。
var semanticBySubEventAndKey = map[subEventScopedKey]core.SemanticKey{
	{key: keyStartTime, event: sessionEvent, subEvent: "logout"}: core.SemanticKeyEventSessionStartTime,
}

// recordSemanticOf は、レコード 1 件の中の 1 つの key の共通の意味を返す。Windows の
// evt と subEvt の組にした対応、Windows のイベント ID と組にした対応、evt と組にした対応の順に探す。
func recordSemanticOf(record Record, key, event string) core.SemanticKey {
	if semantic, mapped := semanticBySubEventAndKey[subEventScopedKey{key, event, subEventOf(record)}]; mapped {
		return semantic
	}
	if eventID, found := record.Field(keyWindowsEventId); found {
		scoped := windowsEventScopedKey{key: key, eventID: eventID.Value()}
		if semantic, mapped := semanticByWindowsEventAndKey[scoped]; mapped {
			return semantic
		}
	}
	return semanticOf(key, event)
}

// logonIdValueOf は Logon ID の欄の値を組む。比べる形は core.LogonIdValue が決める。引用符の中が
// 値の不在を表す文字列である欄は valueOfField と同じ absent の値にする。
func logonIdValueOf(field Field) core.RawAndNormalized {
	value := valueOfField(field)
	if value.ValueState != core.ValueStatePresent {
		return value
	}
	return core.LogonIdValue(field.RawValue(), field.Value())
}

// ログオンの種別を持つ key の文字列と、そこから導いた種別のコードの項目の名前。
//
// 既知の制限: 4625 の failReason に語彙の項目を与えず、markii 形式に固有の key として保持する,
// failReason は失敗の理由の説明文を収集した端末の表示言語で書いた文字列であり、失敗の状態の
// コード (Windows の Status と SubStatus) を持たない。説明文 1 つに対応するコードは SubStatus で
// 分かれるため文字列からコードを 1 つに決められず、対応の正しさを測る対象が無い,
// 説明文と Status のコードの対応を Windows の定義から確かめられたとき、
// event.logon_failure_status へ写す
const (
	keyLogonType           = "logonType"
	logonTypeCodeFieldName = "logonTypeCode"
)

// 種別のコードの導き方を表す 2 つの文字列。分析者が画面で読む値であるため日本語で書く。
const (
	derivationLogonTypeCode         = "logonType の値の括弧の中の 10 進の数字"
	derivationUndeterminedLogonType = "logonType の値が「名前(数字)」の形でないため、" +
		"種別のコードを取り出せない"
)

// logonTypeCodeField は logonType の値から、ログオンの種別のコードの項目を導く。
//
// logonType の値は `Network(3)` のように、種別の名前の後ろに括弧で Windows の LogonType の
// 数字を置く。**原資料の文字列を置き換えない。** logonType の項目は原資料の文字列をそのまま持ち、
// コードは別の項目が持つ。形に合わない値からは導出未確定の項目を返し、数字を推し量らない。
//
// ok が偽になるのは、logonType の key が無いレコードと、値の不在を表す文字列を持つレコードで
// ある。
func logonTypeCodeField(record Record) (core.RecordField, bool) {
	field, found := record.Field(keyLogonType)
	if !found || valueOfField(field).ValueState == core.ValueStateAbsent {
		return core.RecordField{}, false
	}
	value, err := logonTypeCodeValue(field.Value())
	if err != nil {
		return core.RecordField{}, false
	}
	derived, err := core.NewTextField(logonTypeCodeFieldName, core.SemanticKeyEventLogonType, value)
	if err != nil {
		return core.RecordField{}, false
	}
	return derived, true
}

// logonTypeCodeValue は `名前(数字)` の形の文字列から、数字を先頭に 0 を置かない 10 進で返す。
func logonTypeCodeValue(text string) (core.RawAndNormalized, error) {
	open := strings.LastIndexByte(text, '(')
	digits, closed := strings.CutSuffix(text[open+1:], ")")
	// 名前の無い文字列と、括弧を閉じない文字列を退ける。括弧の中が符号や空白を含む文字列は
	// ParseUint が退ける。
	code, err := strconv.ParseUint(digits, 10, 32)
	if open <= 0 || !closed || err != nil {
		return core.NewDerivationUndeterminedValue(derivationUndeterminedLogonType)
	}
	return core.NewDerivedValue(strconv.FormatUint(code, 10), derivationLogonTypeCode)
}

// eventOf はレコードの evt の値を返す。evt を持たないレコードでは空の文字列を返す。
func eventOf(record Record) string {
	field, found := record.Field(keyEvent)
	if !found {
		return ""
	}
	return field.Value()
}
