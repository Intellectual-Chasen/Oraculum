package markii_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// semanticFileRecord は evt が file、subEvt が close のレコードである。
const semanticFileRecord = "02/01/2000 03:04:05.678 +0900 loc=ja-JP type=ITM2 sn=1004 " +
	"lv=5 evt=file subEvt=close os=Win com=\"HOST01\" domain=\"AD\" " +
	"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
	"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 sessionID=0 " +
	"psGUID={66666666-7777-8888-9999-aaaaaaaaaaaa} " +
	"psPath=\"C:\\Tools\\agent.exe\" psID=1234 " +
	"path=\"C:\\Windows\\Temp\\report.txt\" size=1024 " +
	"sha256=" + semanticSha256 + " sha1=" + semanticSha1 + " md5=" + semanticMd5 + " " +
	"sTime=\"02/01/2000 03:04:00.000\" crTime=\"02/01/2000 02:00:00.000\" " +
	"acTime=\"02/01/2000 02:01:00.000\" moTime=\"02/01/2000 02:02:00.000\" " +
	"read=0 write=0 new=0"

// hash の値である。小文字 16 進で、長さは md5 が 32、sha1 が 40、sha256 が 64 である。
const (
	semanticSha256 = "1e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091"
	semanticSha1   = "1e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d"
	semanticMd5    = "1e5ac0d1c4f0dbb7b3c1a2e5d6f70819"
)

// semanticRegistryRecord は evt が reg、subEvt が setVal のレコードである。
const semanticRegistryRecord = "02/01/2000 03:04:06.789 +0900 loc=ja-JP type=ITM2 sn=1005 " +
	"lv=5 evt=reg subEvt=setVal os=Win com=\"HOST01\" domain=\"AD\" " +
	"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
	"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 " +
	"psGUID={66666666-7777-8888-9999-aaaaaaaaaaaa} " +
	"psPath=\"C:\\Tools\\agent.exe\" " +
	"path=\"HKEY_LOCAL_MACHINE\\SOFTWARE\\Example\" entry=\"Sample\" " +
	"valType=REG_SZ valStr=\"example\" valSize=7 sha256=" + semanticSha256

// semanticInjectionRecord は evt が ps、subEvt が inject のレコードである。
// 注入先の値は、中括弧付きの GUID と引用符付きの path の形を持つ値である。
const semanticInjectionRecord = "02/01/2000 03:04:07.890 +0900 loc=ja-JP type=ITM2 sn=1006 " +
	"lv=5 evt=ps subEvt=inject os=Win com=\"HOST01\" domain=\"AD\" " +
	"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
	"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 sessionID=2 " +
	"psGUID={66666666-7777-8888-9999-aaaaaaaaaaaa} " +
	"psPath=\"C:\\Tools\\agent.exe\" api=OpenProcess " +
	"tpsGUID={11111111-2222-3333-4444-bbbbbbbbbbbb} " +
	"tpsPath=\"C:\\Program Files\\Example\\target.exe\""

// 注入のレコードの key が、注入先のプロセスを指す語彙の項目を持ち、値を比べられる形で
// 保つ。
func TestRecordFieldsCarryTheInjectionTarget(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticInjectionRecord).Fields
	assertSemanticOfField(t, fields, "psGUID", core.SemanticKeyProcessId)
	assertSemanticOfField(t, fields, "tpsGUID", core.SemanticKeyInjectionTargetProcessId)
	assertSemanticOfField(t, fields, "tpsPath",
		core.SemanticKeyInjectionTargetProcessBinaryPath)
	// 中括弧を持つ GUID は中括弧を保つ。
	if got := textValueOfField(t, fields, "tpsGUID"); got !=
		"{11111111-2222-3333-4444-bbbbbbbbbbbb}" {
		t.Errorf("tpsGUID = %q, want the braced GUID", got)
	}
	// 引用符を外した文字列が比べられる値になる。注入先の path は空白を含む。
	if got := textValueOfField(t, fields, "tpsPath"); got !=
		`C:\Program Files\Example\target.exe` {
		t.Errorf(`tpsPath = %q, want C:\Program Files\Example\target.exe`, got)
	}
	// api は markii 形式に固有の意味を持つ key であり、語彙の項目を持たない。
	assertSemanticOfField(t, fields, "api", "")
}

// 同じ path の key が、evt によって別の語彙の項目になる。
func TestRecordFieldsSplitThePathSemanticByEvent(t *testing.T) {
	fileFields := parseRecordObservationOK(t, semanticFileRecord).Fields
	assertSemanticOfField(t, fileFields, "path", core.SemanticKeyFilePath)

	registryFields := parseRecordObservationOK(t, semanticRegistryRecord).Fields
	assertSemanticOfField(t, registryFields, "path", core.SemanticKeyRegistryValueKeyPath)
}

// file と reg のレコードの key が、対応表の語彙の項目を持つ。
func TestRecordFieldsCarryTheSemanticOfTheMappedKeys(t *testing.T) {
	fileWant := map[string]core.SemanticKey{
		"headerTime": core.SemanticKeyEventTime,
		"sn":         core.SemanticKeyRecordSequenceNumber,
		"evt":        core.SemanticKeyEventCategory,
		"subEvt":     core.SemanticKeyEventAction,
		"com":        core.SemanticKeyTerminalHostname,
		"tmid":       core.SemanticKeyTerminalId,
		"csid":       core.SemanticKeyTerminalSecurityId,
		"psGUID":     core.SemanticKeyProcessId,
		"psPath":     core.SemanticKeyProcessBinaryPath,
		"psID":       core.SemanticKeyProcessPid,
		"path":       core.SemanticKeyFilePath,
		"size":       core.SemanticKeyFileSizeBytes,
		"sha256":     core.SemanticKeyFileSha256,
		"sha1":       core.SemanticKeyFileSha1,
		"md5":        core.SemanticKeyFileMd5,
		"sTime":      core.SemanticKeyEventOperationStartTime,
		"sessionID":  core.SemanticKeyProcessSessionId,
		"read":       core.SemanticKeyEventReadBytes,
		"write":      core.SemanticKeyEventWrittenBytes,
	}
	for name, want := range fileWant {
		assertSemanticOfField(t, parseRecordObservationOK(t, semanticFileRecord).Fields, name, want)
	}

	registryWant := map[string]core.SemanticKey{
		"path":    core.SemanticKeyRegistryValueKeyPath,
		"entry":   core.SemanticKeyRegistryValueName,
		"valType": core.SemanticKeyRegistryValueDataType,
		"valStr":  core.SemanticKeyRegistryValueData,
	}
	for name, want := range registryWant {
		assertSemanticOfField(t,
			parseRecordObservationOK(t, semanticRegistryRecord).Fields, name, want)
	}
}

// semanticEventLogRecord は evt が os、subEvt が evtLog のレコードである。
// Windows イベントログが記録した接続元の欄を持つ。
const semanticEventLogRecord = "02/01/2000 03:04:08.901 +0900 loc=ja-JP type=ITM2 " +
	"sn=1007 lv=5 evt=os subEvt=evtLog os=Win com=\"HOST01\" domain=\"AD\" " +
	"tmid=" + recordObservationTerminal + " csid=S-1-5-21-1-2-3 " +
	"channel=\"Security\" evtID=4624 evtRecID=7002 evtSrc=\"Microsoft-Windows-Security\" " +
	"wsName=\"HOST02\" wsIp=\"198.51.100.24\" wsPort=50634"

// semanticPowerShellRecord は evt が powerShell、subEvt が exec のレコードである。
const semanticPowerShellRecord = "02/01/2000 03:04:09.012 +0900 loc=ja-JP type=ITM2 " +
	"sn=1008 lv=5 evt=powerShell subEvt=exec os=Win com=\"HOST01\" domain=\"AD\" " +
	"tmid=" + recordObservationTerminal + " csid=S-1-5-21-1-2-3 sessionID=2 " +
	"psGUID={66666666-7777-8888-9999-aaaaaaaaaaaa} psPath=\"C:\\Tools\\agent.exe\" " +
	"shCmd=\"Invoke-WebRequest\" cmdType=\"Cmdlet\""

// Windows イベントログが記録した接続元の欄が、同じ evt の srcIP と同じ語彙の項目を持つ。
func TestRecordFieldsMapTheEventLogConnectionSource(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticEventLogRecord).Fields

	assertSemanticOfField(t, fields, "wsIp", core.SemanticKeyConnectionSourceAddress)
	assertSemanticOfField(t, fields, "wsPort", core.SemanticKeyConnectionSourcePort)
	if got := textValueOfField(t, fields, "wsIp"); got != "198.51.100.24" {
		t.Errorf("wsIp = %q, want the address without the quotes", got)
	}
	if got := textValueOfField(t, fields, "wsPort"); got != "50634" {
		t.Errorf("wsPort = %q, want the port without the quotes", got)
	}
	// 接続元のホスト名は、遠隔から操作した端末の項目へ写る。
	assertSemanticOfField(t, fields, "wsName", core.SemanticKeyRemoteSessionClientHostname)
	if got := textValueOfField(t, fields, "wsName"); got != "HOST02" {
		t.Errorf("wsName = %q, want the hostname without the quotes", got)
	}
}

// PowerShell のレコードの shCmd と sessionID が語彙の項目を持つ。
func TestRecordFieldsMapThePowerShellCommand(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticPowerShellRecord).Fields

	assertSemanticOfField(t, fields, "shCmd", core.SemanticKeyProcessShellCommand)
	assertSemanticOfField(t, fields, "sessionID", core.SemanticKeyProcessSessionId)
	if got := textValueOfField(t, fields, "shCmd"); got != "Invoke-WebRequest" {
		t.Errorf("shCmd = %q, want the command without the quotes", got)
	}
	// コマンドの種類は markii 形式に固有の意味を持ち、語彙の項目を持たない。
	assertSemanticOfField(t, fields, "cmdType", "")
}

// タイムゾーンを持たない sTime は、原資料の文字列の精度と未確定のずれを Timestamp に保持する。
func TestRecordFieldsMapTimezoneLessStartTimeToTimestamp(t *testing.T) {
	for _, testCase := range []struct {
		line     string
		semantic core.SemanticKey
	}{
		{semanticFileRecord, core.SemanticKeyEventOperationStartTime},
		{strings.Replace(semanticFileRecord, "evt=file subEvt=close", "evt=ps subEvt=start", 1),
			core.SemanticKeyProcessStartTime},
	} {
		fields := parseRecordObservationOK(t, testCase.line).Fields
		found := false
		for _, field := range fields {
			if field.Name != "sTime" {
				continue
			}
			found = true
			if field.Semantic != testCase.semantic {
				t.Errorf("sTime semantic = %q, want %q", field.Semantic, testCase.semantic)
			}
			if field.Kind != core.RecordFieldKindTimestamp || field.Timestamp == nil {
				t.Errorf("sTime kind = %q, want timestamp", field.Kind)
				break
			}
			timestamp := field.Timestamp
			if raw, ok := timestamp.RawTextValue(); !ok || raw != `"02/01/2000 03:04:00.000"` {
				t.Errorf("sTime rawText = %q (%t), want quoted source lexeme", raw, ok)
			}
			if normalized, ok := timestamp.NormalizedValue(); !ok || normalized != "2000-02-01T03:04:00.000" {
				t.Errorf("sTime normalized = %q (%t), want local millisecond timestamp", normalized, ok)
			}
			if timestamp.NormalizedForm != core.NormalizedFormLocalWithoutOffset ||
				timestamp.Precision != core.PrecisionMillisecond ||
				timestamp.OffsetState != core.OffsetStateUndetermined ||
				timestamp.Clock != core.ClockTerminalLocal ||
				timestamp.Meaning != core.MeaningOperationStart {
				t.Errorf("sTime timestamp metadata = %+v, want local metadata", timestamp)
			}
			break
		}
		if !found {
			t.Fatal("the record carries no field named sTime")
		}
	}
}

// 書式に合わない sTime は semantic を付けず、原資料の文字列を Text のまま保持する。
func TestRecordFieldsKeepMalformedStartTimeAsText(t *testing.T) {
	line := strings.Replace(semanticFileRecord,
		`sTime="02/01/2000 03:04:00.000"`,
		`sTime="02/01/2000 03:04:00.5"`, 1)
	fields := parseRecordObservationOK(t, line).Fields
	for _, field := range fields {
		if field.Name != "sTime" {
			continue
		}
		if field.Semantic != "" {
			t.Errorf("malformed sTime semantic = %q, want empty", field.Semantic)
		}
		if field.Kind != core.RecordFieldKindText || field.Text == nil || field.Timestamp != nil {
			t.Errorf("malformed sTime field = %+v, want text without timestamp", field)
			return
		}
		if raw, ok := field.Text.RawTextValue(); !ok || raw != `"02/01/2000 03:04:00.5"` {
			t.Errorf("malformed sTime rawText = %q (%t), want quoted source lexeme", raw, ok)
		}
		return
	}
	t.Fatal("the record carries no field named sTime")
}

// markii 形式に固有の意味を持つ key は semantic を持たない。
//
// reg のレコードの sha256 は、レコードの key に path が出ないファイルの hash であり、
// 対応表に載らない。file のレコードの sha256 は対象ファイルの hash であり、対応表に載る
// (semantic_mapping.go の既知の制限)。
func TestRecordFieldsCarryNoSemanticForTheFormatSpecificKeys(t *testing.T) {
	fileFields := parseRecordObservationOK(t, semanticFileRecord).Fields
	for _, name := range []string{
		"loc", "type", "lv", "os", "domain", "profile", "ip", "mac", "new",
	} {
		assertSemanticOfField(t, fileFields, name, "")
	}

	registryFields := parseRecordObservationOK(t, semanticRegistryRecord).Fields
	for _, name := range []string{"valSize", "sha256"} {
		assertSemanticOfField(t, registryFields, name, "")
	}
}

// Windows イベントログ由来の 4 つの key は windows_event.* を持ち、引用符を外した文字列で
// 比べる。引用符の無い値は原資料の文字列で比べる。
func TestRecordFieldsMapTheEventLogKeysToTheWindowsEventItems(t *testing.T) {
	want := map[string]core.SemanticKey{
		"channel":  core.SemanticKeyWindowsEventChannel,
		"evtID":    core.SemanticKeyWindowsEventId,
		"evtRecID": core.SemanticKeyWindowsEventRecordId,
		"evtSrc":   core.SemanticKeyWindowsEventProvider,
	}
	quoted := parseRecordObservationOK(t,
		`02/01/2000 03:04:05.678 +0900 sn=1 evt=os subEvt=evtLog `+
			`channel="Security" evtID="4624" evtRecID="12345" evtSrc="Microsoft-Windows-Security-Auditing"`).Fields
	unquoted := parseRecordObservationOK(t,
		`02/01/2000 03:04:05.678 +0900 sn=1 evt=os subEvt=evtLog `+
			`channel=Security evtID=4624 evtRecID=12345 evtSrc=Microsoft-Windows-Security-Auditing`).Fields
	comparable := map[string]string{
		"channel": "Security", "evtID": "4624", "evtRecID": "12345",
		"evtSrc": "Microsoft-Windows-Security-Auditing",
	}
	declared := markii.ItemSemantics()
	for name, semantic := range want {
		for _, fields := range [][]core.RecordField{quoted, unquoted} {
			assertSemanticOfField(t, fields, name, semantic)
			if got := textValueOfField(t, fields, name); got != comparable[name] {
				t.Errorf("%s comparable = %q, want %q", name, got, comparable[name])
			}
		}
		if !slices.Contains(declared, semantic) {
			t.Errorf("ItemSemantics does not declare %q", semantic)
		}
	}
}

// 転記の同一性の宣言は、チャネルの名前とチャネルの中のレコード番号の 2 つの欄を挙げる。
//
// **欄の名前を挙げる。** 語彙の項目で探せない値であり、宣言を読む pipeline はこの文字列を
// 知らない。
func TestTranscriptIdentityItemsNameTheChannelAndTheRecordNumber(t *testing.T) {
	got := markii.TranscriptIdentityItems()
	want := []string{"channel", "evtRecID"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TranscriptIdentityItems = %v, want %v", got, want)
	}
	// 挙げた欄が実際にレコードの項目として出る。
	fields := parseRecordObservationOK(t,
		`02/01/2000 03:04:05.678 +0900 sn=1 evt=os subEvt=evtLog `+
			`channel="Security" evtRecID="12345"`).Fields
	for _, name := range got {
		if !slices.ContainsFunc(fields, func(field core.RecordField) bool {
			return field.Name == name
		}) {
			t.Errorf("the record carries no field named %q", name)
		}
	}
}

// プロセスに紐付く付加情報と Windows イベントの本文・アカウントを、原資料の文字列のまま
// 共通語彙へ写す。
func TestRecordFieldsMapProcessAndWindowsEventDetails(t *testing.T) {
	fields := parseRecordObservationOK(t,
		`02/01/2000 03:04:05.678 +0900 sn=1 evt=clip subEvt=paste `+
			`clipData="secret" winTitle="Terminal" evtMsg="event body" `+
			`evtUsr="alice" evtDomain="AD"`).Fields
	for name, want := range map[string]core.SemanticKey{
		"clipData":  core.SemanticKeyProcessClipboardData,
		"winTitle":  core.SemanticKeyProcessWindowTitle,
		"evtMsg":    core.SemanticKeyEventMessage,
		"evtUsr":    core.SemanticKeyEventAccountName,
		"evtDomain": core.SemanticKeyEventAccountDomain,
	} {
		assertSemanticOfField(t, fields, name, want)
	}
}

// webURL の URL、復号済み URL、ホスト名が、それぞれ要求と接続先の意味を持つ。
func TestRecordFieldsMapWebURLDetails(t *testing.T) {
	fields := parseRecordObservationOK(t,
		`02/01/2000 03:04:05.678 +0900 sn=1 evt=net subEvt=webURL `+
			`url="http://example.test/a%20b" decode="http://example.test/a b" `+
			`url_hostname="example.test"`).Fields
	for name, want := range map[string]core.SemanticKey{
		"url":          core.SemanticKeyHttpRequestUrl,
		"decode":       core.SemanticKeyHttpRequestDecodedUrl,
		"url_hostname": core.SemanticKeyConnectionDestinationHostname,
	} {
		assertSemanticOfField(t, fields, name, want)
	}
}

// 同じ sha256 の key が、evt によって語彙の項目と空の値に分かれる。
func TestRecordFieldsSplitTheHashSemanticByEvent(t *testing.T) {
	fileFields := parseRecordObservationOK(t, semanticFileRecord).Fields
	assertSemanticOfField(t, fileFields, "sha256", core.SemanticKeyFileSha256)

	registryFields := parseRecordObservationOK(t, semanticRegistryRecord).Fields
	assertSemanticOfField(t, registryFields, "sha256", "")
}

// hash の値は比べられる形の小文字 16 進を原資料の文字列のまま持つ。
func TestRecordFieldsKeepTheHashInTheComparableShape(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticFileRecord).Fields
	for name, want := range map[string]string{
		"sha256": semanticSha256, "sha1": semanticSha1, "md5": semanticMd5,
	} {
		if got := textValueOfField(t, fields, name); got != want {
			t.Errorf("the comparable value of %q = %q, want %q", name, got, want)
		}
	}
}

// 応答に出る項目の semantic は、出ているときだけ語彙の中にある。
func TestRecordFieldsKeepEverySemanticInsideTheVocabulary(t *testing.T) {
	for _, line := range []string{semanticFileRecord, semanticRegistryRecord} {
		observation := parseRecordObservationOK(t, line)
		fields := append(observation.Fields, observation.ObservationKind.Raw...)
		for _, field := range fields {
			if field.Semantic == "" {
				continue
			}
			if !field.Semantic.IsKnown() {
				t.Errorf("the semantic of %q = %q, which is outside the vocabulary",
					field.Name, field.Semantic)
			}
		}
	}
}

// assertSemanticOfField は名前で指した項目の semantic を確かめる。
func assertSemanticOfField(
	t *testing.T, fields []core.RecordField, name string, want core.SemanticKey,
) {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			if field.Semantic != want {
				t.Errorf("the semantic of %q = %q, want %q", name, field.Semantic, want)
			}
			return
		}
	}
	t.Fatalf("the record carries no field named %q", name)
}

// semanticSessionRecord は evt が session、subEvt が loginR のレコードである。
// 接続元の 3 項目は引用符付きである。
const semanticSessionRecord = "02/01/2000 03:04:08.901 +0900 loc=ja-JP type=ITM2 sn=1007 " +
	"lv=5 evt=session subEvt=loginR os=Win com=\"HOST01\" domain=\"AD\" " +
	"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
	"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 " +
	"usr=\"alice\" usrDomain=\"AD\" srcCom=\"HOST09\" srcIP=\"198.51.100.9\" srcPort=\"50001\""

// 接続元を持たない同じ種別のレコード。3 項目は値の不在を表す文字列を持つ。
const semanticSessionRecordWithoutSource = "02/01/2000 03:04:09.012 +0900 loc=ja-JP " +
	"type=ITM2 sn=1008 lv=5 evt=session subEvt=loginR os=Win com=\"HOST01\" domain=\"AD\" " +
	"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
	"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 " +
	"usr=\"alice\" usrDomain=\"AD\" srcCom=\"-\" srcIP=\"-\" srcPort=\"-\""

// 遠隔ログインのレコードの接続元は、net のレコードと同じ語彙の項目を持つ。
func TestRecordFieldsCarryTheSessionSource(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticSessionRecord).Fields
	assertSemanticOfField(t, fields, "srcIP", core.SemanticKeyConnectionSourceAddress)
	assertSemanticOfField(t, fields, "srcPort", core.SemanticKeyConnectionSourcePort)
	if got := textValueOfField(t, fields, "srcIP"); got != "198.51.100.9" {
		t.Errorf("srcIP = %q, want 198.51.100.9", got)
	}
}

// **値の不在を表す文字列を値として持たない。** 引用符の中が - の欄は absent になり、
// 比べられる値を持たない。
func TestRecordFieldsSeparateTheAbsentSourceFromAValue(t *testing.T) {
	fields := parseRecordObservationOK(t, semanticSessionRecordWithoutSource).Fields
	for _, name := range []string{"srcIP", "srcPort", "srcCom"} {
		field, found := recordFieldNamed(fields, name)
		if !found {
			t.Fatalf("the record carries no field named %q", name)
		}
		if field.Text == nil {
			t.Fatalf("the field %q carries no text value", name)
		}
		if field.Text.ValueState != core.ValueStateAbsent {
			t.Errorf("the field %q carries the state %q, want absent",
				name, field.Text.ValueState)
		}
		if _, readable := field.Text.ComparableValue(); readable {
			t.Errorf("the field %q carries a comparable value", name)
		}
	}
}

// recordFieldNamed は名前で指した項目を返す。
func recordFieldNamed(fields []core.RecordField, name string) (core.RecordField, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return core.RecordField{}, false
}

// webUrlRecord は evt が net、subEvt が webURL のレコードを組む。
// 接続先の欄を持たず、url だけが接続先を持つ。
func webUrlRecord(target string) string {
	return "02/01/2000 03:04:10.123 +0900 loc=ja-JP type=ITM2 sn=1009 lv=5 " +
		"evt=net subEvt=webURL os=Win com=\"HOST01\" domain=\"AD\" " +
		"profile=\"example_profile\" tmid=" + recordObservationTerminal + " " +
		"csid=S-1-5-21-1-2-3 ip=192.0.2.10,fe80::1 mac=00:00:5e:00:53:01 " +
		"psGUID={66666666-7777-8888-9999-aaaaaaaaaaaa} " +
		"psPath=\"C:\\Browser\\browser.exe\" brType=IE url=\"" + target + "\""
}

// Web アクセスのレコードは、url の文字列から接続先の host を導く。
// **host の文字列の形で語彙の項目が変わる。**
func TestRecordFieldsDeriveTheWebAccessDestination(t *testing.T) {
	for _, testCase := range []struct {
		target   string
		host     string
		semantic core.SemanticKey
	}{
		{"https://example.test/a?q=1", "example.test",
			core.SemanticKeyConnectionDestinationHostname},
		{"http://example.test:8080/a", "example.test",
			core.SemanticKeyConnectionDestinationHostname},
		{"https://198.51.100.9/a", "198.51.100.9",
			core.SemanticKeyConnectionDestinationAddress},
		{"https://[2001:db8::1]:8443/a", "2001:db8::1",
			core.SemanticKeyConnectionDestinationAddress},
	} {
		t.Run(testCase.target, func(t *testing.T) {
			fields := parseRecordObservationOK(t, webUrlRecord(testCase.target)).Fields
			field, found := recordFieldNamed(fields, "urlHost")
			if !found {
				t.Fatal("the record derived no destination host")
			}
			if field.Semantic != testCase.semantic {
				t.Errorf("the semantic = %q, want %q", field.Semantic, testCase.semantic)
			}
			if field.Text == nil || field.Text.ValueState != core.ValueStateDerived {
				t.Fatalf("the derived host carries the value %+v", field.Text)
			}
			if got := textValueOfField(t, fields, "urlHost"); got != testCase.host {
				t.Errorf("the derived host = %q, want %q", got, testCase.host)
			}
		})
	}
}

// **レコードが接続先の host の欄を持つときは導かない。** 同じ意味の項目が 2 つに
// 分かれると、1 件のアクセスの接続先のノードが 2 つになる。
func TestRecordFieldsDeriveNoDestinationWhenTheRecordCarriesTheHost(t *testing.T) {
	line := webUrlRecord("https://example.test/a") + " url_hostname=\"recorded.test\""
	fields := parseRecordObservationOK(t, line).Fields
	if _, found := recordFieldNamed(fields, "urlHost"); found {
		t.Error("the record derived a destination host while carrying url_hostname")
	}
	assertSemanticOfField(t, fields, "url_hostname",
		core.SemanticKeyConnectionDestinationHostname)
	hosts := 0
	for _, field := range fields {
		if field.Semantic == core.SemanticKeyConnectionDestinationHostname {
			hosts++
		}
	}
	if hosts != 1 {
		t.Errorf("the record carries %d destination hostname items, want 1", hosts)
	}
}

// **ネットワークの接続先でない url から接続先を導かない。**
func TestRecordFieldsDeriveNoDestinationOutsideTheWebSchemes(t *testing.T) {
	for _, target := range []string{
		"res://example.dll/start.htm", "www.example.test/", "c:\\temp\\page.html",
		"https:///a",
	} {
		t.Run(target, func(t *testing.T) {
			fields := parseRecordObservationOK(t, webUrlRecord(target)).Fields
			if _, found := recordFieldNamed(fields, "urlHost"); found {
				t.Error("the record derived a destination host")
			}
			// url の原資料の文字列は残る。
			assertSemanticOfField(t, fields, "url", core.SemanticKeyHttpRequestUrl)
		})
	}
}
