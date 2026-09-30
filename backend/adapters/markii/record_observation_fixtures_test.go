package markii_test

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// 種別に依らない観測の検査が使う行。
//
// 端末名・GUID・SID・path・利用者名は本 test が決めた値である。文字列の構造だけを、当該の
// evt と subEvt のレコードに合わせた。期待値は testdata/record-observation-manifest.json が持つ。

const (
	// evt が file のレコード。path と時刻の欄を持つ。通信の 4 項目と recv と send を持たない。
	fileCloseLine = `02/01/2000 03:04:05.678 +0900 loc=ja-JP type=ITM2 sn=800100 lv=5 ` +
		`evt=file subEvt=close os=Win com="HOST01" domain="EXAMPLE" profile="example_profile" ` +
		`tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 sessionID=0 ` +
		`psGUID={00000000-1111-2222-3333-555555555555} psPath="C:\Tools\writer.exe" ` +
		`path="C:\work\report.docx" drvType=HDD read=20480 write=0 size=65536 hide=0 new=0`

	// evt が session のレコード。利用者とその所属の欄を持つ。psGUID と psPath を持たない。
	sessionLoginRLine = `02/01/2000 10:08:09.700 +0900 loc=ja-JP type=ITM2 sn=800200 lv=5 ` +
		`evt=session subEvt=loginR os=Win com="HOST01" domain="EXAMPLE" ` +
		`profile="example_profile" tmid=00000000-1111-2222-3333-444444444444 ` +
		`csid=S-1-5-21-1111111111-2222222222-3333333333 usr="HOST01$" ` +
		`usrDomain="EXAMPLE.TEST" srcCom="-" srcIP="::1" srcPort=50712 evtRecID=4417`

	// 必須の key を 1 つずつ欠く 3 行。
	fileCloseWithoutSequenceLine = `02/01/2000 03:04:05.678 +0900 evt=file subEvt=close ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 path="C:\work\report.docx"`
	fileCloseWithoutEventLine = `02/01/2000 03:04:05.678 +0900 sn=800101 subEvt=close ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 path="C:\work\report.docx"`
	fileCloseWithoutSubEventLine = `02/01/2000 03:04:05.678 +0900 sn=800102 evt=file ` +
		`com="HOST01" tmid=00000000-1111-2222-3333-444444444444 path="C:\work\report.docx"`

	// 必須でない key を欠く行。tmid が無くても観測を作る。
	fileCloseWithoutTerminalIdLine = `02/01/2000 03:04:05.678 +0900 sn=800103 evt=file ` +
		`subEvt=close com="HOST01" path="C:\work\report.docx"`

	// 必須の key が値を持たない行。subEvt の key はあるが value が空である。
	fileCloseWithEmptySubEventLine = `02/01/2000 03:04:05.678 +0900 sn=800104 evt=file ` +
		`subEvt= com="HOST01" path="C:\work\report.docx"`

	// 必須の key が 1 レコードに 2 回出る行。subEvt が 2 つある。
	fileCloseWithRepeatedSubEventLine = `02/01/2000 03:04:05.678 +0900 sn=800105 evt=file ` +
		`subEvt=close com="HOST01" tmid=00000000-1111-2222-3333-444444444444 subEvt=del`

	// ヘッダーの日時が日時として解釈できない行。月が 13 である。
	fileCloseWithBadHeaderTimeLine = `13/45/2000 99:99:99.999 +0900 sn=800106 evt=file ` +
		`subEvt=close com="HOST01" tmid=00000000-1111-2222-3333-444444444444`
)

// 正常な 2 行から読める値。manifest と同じ値をここにも置き、test から名前で探す。
const (
	fileCloseSequenceNumber   = int64(800100)
	fileCloseNormalizedTime   = "2000-02-01T03:04:05.678+09:00"
	fileClosePath             = `C:\work\report.docx`
	sessionLoginRUser         = `HOST01$`
	sessionLoginRUserDomain   = "EXAMPLE.TEST"
	sessionLoginRSequenceNum  = int64(800200)
	recordObservationTerminal = "00000000-1111-2222-3333-444444444444"
)

// observedEventKind は evt と subEvt の 1 組と、その意味の状態である。
type observedEventKind struct {
	event    string
	subEvent string
	status   core.ObservationKindStatus
}

// observedEventKinds は Recorder が出力する組である。evt と subEvt の辞書順に並べる。
// testdata/record-observation-manifest.json の observedKinds.kinds と同じ組を持つ。
var observedEventKinds = []observedEventKind{
	{"clip", "copy", core.ObservationKindStatusDetermined},
	{"clip", "paste", core.ObservationKindStatusDetermined},
	{"file", "chgAttr", core.ObservationKindStatusDetermined},
	{"file", "close", core.ObservationKindStatusDetermined},
	{"file", "copy", core.ObservationKindStatusDetermined},
	{"file", "create", core.ObservationKindStatusInferred},
	{"file", "createDir", core.ObservationKindStatusInferred},
	{"file", "del", core.ObservationKindStatusDetermined},
	{"file", "delDir", core.ObservationKindStatusDetermined},
	{"file", "download", core.ObservationKindStatusDetermined},
	{"file", "rename", core.ObservationKindStatusDetermined},
	{"file", "renameDir", core.ObservationKindStatusDetermined},
	{"net", "acpt", core.ObservationKindStatusDetermined},
	{"net", "con", core.ObservationKindStatusDetermined},
	{"net", "dcon", core.ObservationKindStatusDetermined},
	{"net", "est", core.ObservationKindStatusInferred},
	{"net", "openUDP", core.ObservationKindStatusInferred},
	{"net", "webURL", core.ObservationKindStatusDetermined},
	{"os", "evtLog", core.ObservationKindStatusDetermined},
	{"powerShell", "exec", core.ObservationKindStatusDetermined},
	{"ps", "start", core.ObservationKindStatusDetermined},
	{"ps", "stop", core.ObservationKindStatusDetermined},
	{"reg", "create", core.ObservationKindStatusInferred},
	{"reg", "delKey", core.ObservationKindStatusDetermined},
	{"reg", "delVal", core.ObservationKindStatusDetermined},
	{"reg", "setVal", core.ObservationKindStatusDetermined},
	{"session", "conR", core.ObservationKindStatusDetermined},
	{"session", "dconR", core.ObservationKindStatusDetermined},
	{"session", "lock", core.ObservationKindStatusDetermined},
	{"session", "login", core.ObservationKindStatusDetermined},
	{"session", "loginR", core.ObservationKindStatusDetermined},
	{"session", "loginRFail", core.ObservationKindStatusDetermined},
	{"session", "logout", core.ObservationKindStatusDetermined},
	{"session", "unlock", core.ObservationKindStatusDetermined},
	{"sys", "startRed", core.ObservationKindStatusDetermined},
	{"sys", "stop", core.ObservationKindStatusDetermined},
	{"sys", "stopRed", core.ObservationKindStatusDetermined},
	{"win", "active", core.ObservationKindStatusDetermined},
	{"win", "psInactive", core.ObservationKindStatusDetermined},
	{"wmi", "psCreate", core.ObservationKindStatusDetermined},
}

// inferredObservedEventKinds は、observedEventKinds のうち意味を推定した組である。
var inferredObservedEventKinds = [][2]string{
	{"file", "create"},
	{"file", "createDir"},
	{"net", "est"},
	{"net", "openUDP"},
	{"reg", "create"},
}
