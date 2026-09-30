package markii_test

// 通信の検査が使う行。
//
// 端末名・GUID・SID・path・IP・port は本 test が決めた値である。IP は RFC 5737 の文書用の
// 範囲から採る。期待値は testdata/communication-manifest.json が持つ。

const (
	// 必須の 8 key と接続の相手の 4 key をすべて持つ正常な 1 行。subEvt は con である。
	communicationConnectLine = `02/01/2000 13:30:00.500 +0900 loc=ja-JP type=ITM2 sn=900300 lv=5 ` +
		`evt=net subEvt=con os=Win com="HOST01" domain="EXAMPLE" profile="example_profile" ` +
		`tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 sessionID=0 ` +
		`ip=192.0.2.10,fe80::1111:2222:3333:4444 mac=00:00:5e:00:53:01 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" ` +
		`srcIP=192.0.2.10 srcPort=50417 dstIP=198.51.100.20 dstPort=443 ` +
		`dstHost="example.invalid"`

	// 接続の受け入れ。subEvt は acpt である。
	communicationAcceptLine = `02/01/2000 13:30:01.000 +0900 sn=900301 evt=net subEvt=acpt ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" ` +
		`srcIP=198.51.100.20 srcPort=51000 dstIP=192.0.2.10 dstPort=445`

	// 切断。subEvt は dcon である。通信量の 2 つの key を持つ。
	communicationCloseLine = `02/01/2000 13:30:02.000 +0900 sn=900302 evt=net subEvt=dcon ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" ` +
		`srcIP=192.0.2.10 srcPort=50417 dstIP=198.51.100.20 dstPort=443 recv=0 send=1024`

	// 接続の成立。subEvt は est であり、意味を推定した組である。
	communicationEstablishLine = `02/01/2000 13:30:03.000 +0900 sn=900303 evt=net subEvt=est ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" ` +
		`srcIP=192.0.2.10 srcPort=50926 dstIP=198.51.100.21 dstPort=80`

	// UDP の口を開く。subEvt は openUDP である。接続の相手の 4 key を 1 つも持たず、
	// port を 1 つ持つ。
	communicationOpenUdpLine = `02/01/2000 13:30:04.000 +0900 sn=900304 evt=net subEvt=openUDP ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" port=53`

	// 必須の key を 1 つ欠く行。psPath が無い。
	communicationWithoutPathLine = `02/01/2000 13:30:05.000 +0900 sn=900305 evt=net subEvt=con ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`srcIP=192.0.2.10 srcPort=50417 dstIP=198.51.100.20 dstPort=443`

	// sn の value が 10 進整数でない行。
	communicationWithBadSequenceLine = `02/01/2000 13:30:06.000 +0900 sn=x900306 evt=net subEvt=con ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe"`

	// ヘッダーの日時が日時として解釈できない行。月が 13 である。
	communicationWithBadHeaderTimeLine = `13/45/2000 99:99:99.999 +0900 sn=900307 evt=net subEvt=con ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe"`

	// 必須の key が 1 レコードに 2 回出る行。psGUID が 2 つある。
	communicationWithRepeatedKeyLine = `02/01/2000 13:30:08.000 +0900 sn=900308 evt=net subEvt=con ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" ` +
		`psGUID={00000000-1111-2222-3333-777777777777}`

	// evt が net で、subEvt が受け付ける 5 つのいずれでもない行。
	communicationWithUnknownSubEventLine = `02/01/2000 13:30:09.000 +0900 sn=900309 evt=net ` +
		`subEvt=listen com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe"`

	// subEvt を欠く行。通信かどうかを判定できない。
	communicationWithoutSubEventLine = `02/01/2000 13:30:10.000 +0900 sn=900310 evt=net ` +
		`com="HOST01" psGUID={00000000-1111-2222-3333-555555555555}`

	// 引用符付きの srcIP を持つ行。net のレコードの srcIP は引用符を持たないが、文字列の分割が
	// 引用符付きの srcIP を扱えるため、正規化値を持つことを固定する。
	communicationWithQuotedSourceIpLine = `02/01/2000 13:30:11.000 +0900 sn=900311 evt=net ` +
		`subEvt=con com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Tools\agent.exe" srcIP="192.0.2.10"`
)

// communicationOptionalKeyNames は、当該レコードに出ないとき Fields が item_absent の
// 項目で持つ key である。一覧は testdata/communication-manifest.json の
// optionalKeysCompletedInFields が持つ。
var communicationOptionalKeyNames = []string{
	"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send",
}

// communicationHeaderText は必須の key の検査が組み立てる行のヘッダーである。
const communicationHeaderText = "02/01/2000 13:30:20.000 +0900"

// communicationRequiredKeyTokens は必須の 8 key を 1 つずつ持つ文字列である。
//
// 必須の key の検査が、この一覧から 1 つを外した行・値を空にした行・2 回置いた行を
// 組み立てる。key の名前を並べた一覧は
// testdata/communication-manifest.json の requiredKeys が持つ。
var communicationRequiredKeyTokens = []struct {
	key   string
	value string
}{
	{"sn", "900320"},
	{"evt", "net"},
	{"subEvt", "con"},
	{"psGUID", "{00000000-1111-2222-3333-555555555555}"},
	{"tmid", "00000000-1111-2222-3333-444444444444"},
	{"com", `"HOST01"`},
	{"csid", "S-1-5-21-1111111111-2222222222-3333333333"},
	{"psPath", `"C:\Tools\agent.exe"`},
}

// 正常な行から読める値。manifest と同じ値をここにも置き、test から名前で探す。
const (
	communicationSequenceNumber = int64(900300)
	communicationProcessGuid    = "{00000000-1111-2222-3333-555555555555}"
	communicationTerminalId     = "00000000-1111-2222-3333-444444444444"
	communicationRawHeaderTime  = "02/01/2000 13:30:00.500 +0900"
	communicationNormalizedTime = "2000-02-01T13:30:00.500+09:00"
	communicationOffsetText     = "+0900"
	communicationSourceIp       = "192.0.2.10"
	communicationSourcePort     = "50417"
	communicationDestIp         = "198.51.100.20"
	communicationDestPort       = "443"
	communicationRawPath        = `"C:\Tools\agent.exe"`
	communicationPath           = `C:\Tools\agent.exe`
)
