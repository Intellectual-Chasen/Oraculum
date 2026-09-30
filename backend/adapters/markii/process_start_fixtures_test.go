package markii_test

// プロセス開始の検査が使う行。
//
// 端末名・GUID・SID・path・利用者名は本 test が決めた値である。文字列の構造だけを、
// プロセス開始のレコードに合わせた。
//
// 期待値は testdata/process-start-manifest.json が持つ。実装の出力を転記していない。

const (
	// 必須の 8 key をすべて持つ正常な 1 行。引用符付きの value と引用符無しの value を
	// 両方持ち、値が 0 の欄 (sessionID) と、親への参照 (parentGUID) を持つ。
	processStartLine = `02/01/2000 13:20:00.500 +0900 loc=ja-JP type=ITM2 sn=900200 lv=5 evt=ps ` +
		`subEvt=start os=Win com="HOST01" domain="EXAMPLE" profile="example_profile" ` +
		`tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 sessionID=0 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} ` +
		`psPath="C:\Windows\system32\notepad.exe" psID=4321 ` +
		`parentGUID={00000000-1111-2222-3333-666666666666} ` +
		`parentPath="C:\Windows\explorer.exe" psUser="analyst" psDomain="EXAMPLE" ` +
		`cmd="notepad.exe ""C:\work\note.txt"""`

	// 必須の key を 1 つ欠く行。psPath が無い。
	processStartWithoutPathLine = `02/01/2000 13:20:01.000 +0900 sn=900201 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555}`

	// 省略可の parentGUID を欠く行。必須の 8 key はすべて持つ。
	processStartWithoutParentLine = `02/01/2000 13:20:02.000 +0900 sn=900202 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Windows\system32\notepad.exe"`

	// sn の value が 10 進整数でない行。
	processStartWithBadSequenceLine = `02/01/2000 13:20:03.000 +0900 sn=x900203 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Windows\system32\notepad.exe"`

	// ヘッダーの日時が日時として解釈できない行。月が 13 である。
	processStartWithBadHeaderTimeLine = `13/45/2000 99:99:99.999 +0900 sn=900204 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Windows\system32\notepad.exe"`

	// 必須の key が 1 レコードに 2 回出る行。psGUID が 2 つある。
	processStartWithRepeatedKeyLine = `02/01/2000 13:20:05.000 +0900 sn=900205 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Windows\system32\notepad.exe" ` +
		`psGUID={00000000-1111-2222-3333-777777777777}`

	// プロセスの終了のレコード。
	processStopLine = `02/01/2000 13:20:06.000 +0900 sn=900206 evt=ps subEvt=stop ` +
		`com="HOST01" psGUID={00000000-1111-2222-3333-555555555555}`

	// 通信のレコード。
	communicationLine = `02/01/2000 13:20:07.000 +0900 sn=900207 evt=net subEvt=con ` +
		`com="HOST01" srcIP=192.0.2.10 dstIP=198.51.100.20 dstPort=443`

	// 省略可の key が 1 レコードに 2 回出る行。必須の 8 key はすべて 1 回ずつ持つ。
	// 2 つの lv は別の値である。
	processStartWithRepeatedOptionalKeyLine = `02/01/2000 13:20:10.000 +0900 sn=900210 evt=ps ` +
		`subEvt=start com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Windows\system32\notepad.exe" ` +
		`lv=5 lv=7`

	// 必須の key が値を持たない行。key はあるが value が空である。
	processStartWithEmptyGuidLine = `02/01/2000 13:20:09.000 +0900 sn=900209 evt=ps subEvt=start ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 psGUID= ` +
		`psPath="C:\Windows\system32\notepad.exe"`

	// evt を欠く行。プロセス開始かどうかを判定できない。
	processStartWithoutEventLine = `02/01/2000 13:20:08.000 +0900 sn=900208 subEvt=start ` +
		`com="HOST01" psGUID={00000000-1111-2222-3333-555555555555}`
)

// 正常な行から読める値。manifest と同じ値をここにも置き、test から名前で探す。
const (
	processStartSequenceNumber = int64(900200)
	processStartProcessGuid    = "{00000000-1111-2222-3333-555555555555}"
	processStartTerminalId     = "00000000-1111-2222-3333-444444444444"
	processStartParentGuid     = "{00000000-1111-2222-3333-666666666666}"
	processStartRawHeaderTime  = "02/01/2000 13:20:00.500 +0900"
	processStartNormalizedTime = "2000-02-01T13:20:00.500+09:00"
	processStartOffsetText     = "+0900"
	processStartRawPath        = `"C:\Windows\system32\notepad.exe"`
	processStartPath           = `C:\Windows\system32\notepad.exe`
)
