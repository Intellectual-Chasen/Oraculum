package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// 種別に依らない観測が必要とする key。**欠けたレコードから観測を作らない。**
//
// sn は RecordLocator の sequenceNumber の材料、evt と subEvt は ObservationKind の raw の
// 2 要素である。値の材料になる key だけを並べる。tmid を必須にするのは、tmid を読んで端末へ
// 対応付ける側の判断である。evt=os subEvt=evtLog と evt=session のレコードは psGUID と psPath を
// 持たないため、psGUID / com / csid / psPath を必須にしない。
//
// プロセス開始と通信の一覧と重なるが、どの key を必須とするかはレコードの種別ごとの判断で
// あるため、一覧を共有しない。
var recordObservationRequiredKeys = []string{
	keySequenceNumber, keyEvent, keySubEvent,
}

// documentedSubEventsV30 は V3.0 の Recorder が出力する evt と subEvt の組である。key が evt、
// 値がその evt の subEvt の集合である。
var documentedSubEventsV30 = map[string]map[string]struct{}{
	"sys": {
		"start": {}, "stop": {}, "run": {}, "chgConf": {}, "outDsk": {},
		"startRed": {}, "stopRed": {},
	},
	"os": {
		"startSafemode": {}, "stopSafemode": {}, "suspend": {}, "resume": {},
		"chgDate": {}, "evtLog": {},
	},
	"dev": {"mnt": {}, "unMnt": {}, "mntd": {}},
	"session": {
		"login": {}, "loginFail": {}, "loginR": {}, "loginRFail": {}, "logout": {},
		"lock": {}, "unlock": {}, "conR": {}, "dconR": {}, "startSCR": {}, "stopSCR": {},
	},
	"ps": {"start": {}, "stop": {}, "run": {}, "inject": {}, "guardInject": {}},
	"file": {
		"close": {}, "del": {}, "rename": {}, "copy": {}, "chgAttr": {}, "delDir": {},
		"renameDir": {}, "impWPD": {}, "expWPD": {}, "renameWPD": {}, "delWPD": {},
		"download": {}, "unblock": {}, "enableMacro": {},
	},
	"reg":  {"setVal": {}, "delKey": {}, "delVal": {}, "renameKey": {}},
	"prt":  {"create": {}},
	"win":  {"active": {}},
	"clip": {"paste": {}},
	"net": {
		"con": {}, "acpt": {}, "dcon": {}, "lsn": {}, "webURL": {}, "chgSSID": {},
		"dnsQuery": {}, "mailSend": {}, "mailRecv": {},
	},
	"windowsDefender": {"dtctMalState": {}},
}

// documentedSubEventsV32 は V3.2 の Recorder が出力する evt と subEvt の組である。並べ方は
// documentedSubEventsV30 と同じである。
var documentedSubEventsV32 = map[string]map[string]struct{}{
	"sys": {
		"start": {}, "stop": {}, "run": {}, "chgConf": {}, "outDsk": {},
		"startRed": {}, "stopRed": {}, "suspendLog": {}, "resumeLog": {}, "restart": {},
		"suspendPSMon": {}, "resumePSMon": {},
	},
	"os": {
		"startSafemode": {}, "stopSafemode": {}, "suspend": {}, "resume": {},
		"chgDate": {}, "evtLog": {}, "start": {}, "stop": {},
	},
	"dev": {"mnt": {}, "unMnt": {}, "mntd": {}, "con": {}, "dcon": {}},
	"session": {
		"login": {}, "loginFail": {}, "loginR": {}, "loginRFail": {}, "logout": {},
		"lock": {}, "unlock": {}, "conR": {}, "dconR": {}, "startSCR": {}, "stopSCR": {},
	},
	"ps": {"start": {}, "stop": {}, "run": {}, "inject": {}, "guardInject": {}},
	"file": {
		"close": {}, "del": {}, "rename": {}, "copy": {}, "chgAttr": {}, "delDir": {},
		"renameDir": {}, "impWPD": {}, "expWPD": {}, "renameWPD": {}, "delWPD": {},
		"download": {}, "unblock": {}, "enableMacro": {}, "upload": {},
	},
	"reg":  {"setVal": {}, "delKey": {}, "delVal": {}, "renameKey": {}},
	"prt":  {"create": {}},
	"win":  {"active": {}, "inactive": {}, "psActive": {}, "psInactive": {}},
	"clip": {"copy": {}, "paste": {}},
	"net": {
		"con": {}, "acpt": {}, "dcon": {}, "lsn": {}, "webURL": {}, "chgSSID": {},
		"dnsQuery": {}, "mailSend": {}, "mailRecv": {}, "login": {},
	},
	"wmi":        {"psCreate": {}},
	"powerShell": {"exec": {}},
	"windowsDefender": {
		"scnComplete": {}, "scnCancel": {}, "scnFail": {}, "qrtnRestore": {},
		"qrtnRestoreFail": {}, "qrtnDel": {}, "qrtnDelFail": {}, "dtctMalState": {},
		"malStateActTaken": {}, "malStateActFail": {}, "malStateActCritFail": {},
		"ASRBlock": {}, "ASRAudit": {}, "sigUpdate": {}, "sigUpdateFail": {},
		"engUpdate": {}, "engUpdateFail": {}, "chgConf": {}, "smartScrnApp": {},
		"smartScrnUri": {}, "smartScrnUsr": {},
	},
}

// documentedSubEvents はバージョンごとの表の和集合である。
//
// **レコードは Recorder のバージョンを持つ欄を持たないため、和集合で意味を確定する。** 片方の
// 表だけを探すと、もう片方のバージョンの Recorder が出した組を undetermined にする。
// 表にある組は形式が意味を定めており、observationKindOfRecord が determined を返す。
var documentedSubEvents = unionOfSubEventTables(documentedSubEventsV30, documentedSubEventsV32)

// inferredSubEvent は 1 組について推定した意味と、その根拠である。根拠を書かない推定は、
// 分析者が推定の当否を確かめられないため、2 つの欄をどちらも必須にする。
type inferredSubEvent struct {
	meaning  string
	evidence string
}

// inferredSubEvents は、どのバージョンの表にも載らず、レコードが持つ欄と前後のレコードから意味を
// 推定した組である。observationKindOfRecord は本表の組に inferred を返す。
var inferredSubEvents = map[string]map[string]inferredSubEvent{
	"file": {
		"create": {
			meaning: "プロセスがファイルを新しく作った記録",
			evidence: "レコードは path と drvType を持ち、close が持つ read と write と size と new と " +
				"sTime を持たない。同じ path の close が後に続き、その close が new=1 を持つことがある",
		},
		"createDir": {
			meaning: "プロセスがフォルダーを新しく作った記録",
			evidence: "レコードの欄は delDir と同じ path と drvType である。その path を親に持つ path の " +
				"レコードが後に続くことがある",
		},
	},
	"reg": {
		"create": {
			meaning: "プロセスがレジストリのキーを新しく作った記録",
			evidence: "レコードはキーの path を持ち、値を表す entry と valType と valStr と valNum を " +
				"持たない。欄の組は delKey と同じで、setVal と delVal とは異なる。同じキーの path に " +
				"対する setVal が後に続くことがある",
		},
	},
	"net": {
		"est": {
			meaning: "TCP の接続が確立した記録",
			evidence: "レコードは con と同じ srcIP と srcPort と dstIP と dstPort を持つ。同じ 4 つ組の " +
				"dcon が後に続くことがある。同じ 4 つ組の est が続かない con もあり、con は接続の試みを記録する",
		},
		"openUDP": {
			meaning: "プロセスが UDP のポートを開いて待ち受けを始めた記録",
			evidence: "レコードは port だけを持ち、接続の相手を表す srcIP と srcPort と dstIP と " +
				"dstPort を持たない。欄の組は TCP の待ち受けを始めた記録 (lsn) に対応する",
		},
	},
}

// unionOfSubEventTables はバージョンごとの表を 1 つの表へまとめる。引数の表を変更しない。
func unionOfSubEventTables(tables ...map[string]map[string]struct{}) map[string]map[string]struct{} {
	union := map[string]map[string]struct{}{}
	for _, table := range tables {
		for event, subEvents := range table {
			merged, found := union[event]
			if !found {
				merged = map[string]struct{}{}
				union[event] = merged
			}
			for subEvent := range subEvents {
				merged[subEvent] = struct{}{}
			}
		}
	}
	return union
}

// observationKindOfRecord は evt と subEvt の 2 件から core.ObservationKind を組む。
//
// 呼ぶのは ParseRecordObservation が 2 つの key の値を確かめた後だけである。
func observationKindOfRecord(record Record) core.ObservationKind {
	status := documentedKindStatus(record)
	kind := core.ObservationKind{Raw: observationKindRaw(record), Status: status}
	if status == core.ObservationKindStatusInferred {
		kind.Meaning = inferredSubEvents[eventOf(record)][subEventOf(record)].meaning
	}
	return kind
}

// subEventOf はレコードの subEvt の値を返す。subEvt を持たないレコードでは空の文字列を返す。
func subEventOf(record Record) string {
	field, found := record.Field(keySubEvent)
	if !found {
		return ""
	}
	return field.Value()
}

// documentedKindStatus は evt と subEvt の組を意味の状態へ直す。
//
// documentedSubEvents にある組は determined、inferredSubEvents にある組は inferred、
// どちらにも無い組は undetermined である。同じ組が両方に載ったときに形式の定めを推定で
// 上書きしないため、documentedSubEvents を先に探す。
func documentedKindStatus(record Record) core.ObservationKindStatus {
	event, hasEvent := record.Field(keyEvent)
	subEvent, hasSubEvent := record.Field(keySubEvent)
	if !hasEvent || !hasSubEvent {
		return core.ObservationKindStatusUndetermined
	}
	if _, documented := documentedSubEvents[event.Value()][subEvent.Value()]; documented {
		return core.ObservationKindStatusDetermined
	}
	if _, inferred := inferredSubEvents[event.Value()][subEvent.Value()]; inferred {
		return core.ObservationKindStatusInferred
	}
	return core.ObservationKindStatusUndetermined
}
