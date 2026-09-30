package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// 通信のレコードを選ぶ evt と subEvt の値。
const (
	communicationEvent             = "net"
	communicationSubEventConnect   = "con"
	communicationSubEventAccept    = "acpt"
	communicationSubEventClose     = "dcon"
	communicationSubEventEstablish = "est"
	communicationSubEventOpenUDP   = "openUDP"
)

// communicationSubEventStatus は、受け付ける subEvt と、その意味の状態の対応である。
//
// **受け付ける一覧と状態の判断を 1 つの表が持つ。** 表に無い値を持つレコードからは観測を
// 作らない。con と acpt と dcon は形式が意味を定め、est と openUDP は意味を推定した組であり、
// 推定した意味と根拠は inferredSubEvents が持つ。
var communicationSubEventStatus = map[string]core.ObservationKindStatus{
	communicationSubEventConnect:   core.ObservationKindStatusDetermined,
	communicationSubEventAccept:    core.ObservationKindStatusDetermined,
	communicationSubEventClose:     core.ObservationKindStatusDetermined,
	communicationSubEventEstablish: core.ObservationKindStatusInferred,
	communicationSubEventOpenUDP:   core.ObservationKindStatusInferred,
}

// 接続の相手と通信量を表す key の文字列。
//
// **共有へ上げない。** 6 つともプロセス開始のレコードに出ない。
const (
	keySourceIp      = "srcIP"
	keySourcePort    = "srcPort"
	keyDestIp        = "dstIP"
	keyDestPort      = "dstPort"
	keyReceivedBytes = "recv"
	keySentBytes     = "send"
)

// communicationOptionalKeys は通信のレコードが定める key のうち、出ないレコードが
// 実在する 6 つである。**当該レコードに出ない key を item_absent の項目にする**。
//
// 既知の制限: 通信のレコードが定める key をこの 6 つに限る,
// srcIP と srcPort と dstIP と dstPort を持たないのは openUDP、recv と send を持つのは dcon であり、
// ほかの key の有無が subEvt によって変わるかを repo の中の例から確かめられない,
// 通信のレコードが定める key の全数を確定できたとき、一覧をその全数に合わせる
var communicationOptionalKeys = []string{
	keySourceIp, keySourcePort, keyDestIp, keyDestPort, keyReceivedBytes, keySentBytes,
}

// 通信の観測が必要とする key。**欠けたレコードから観測を作らない。**
//
// sn は位置、psGUID と tmid はプロセスの識別、com と csid は端末の識別、psPath は
// 実行 file の path、evt と subEvt は観測の種別である。openUDP のレコードが接続の相手の
// 4 つを持たないため、4 つを必須にしない。プロセス開始の一覧と同じ値だが、必須の判断は
// レコードの種別ごとに行うため一覧を共有しない。
var communicationRequiredKeys = []string{
	keySequenceNumber, keyEvent, keySubEvent, keyProcessGuid,
	keyTerminalId, keyComputerName, keySecurityId, keyProcessPath,
}

// communicationFieldsOf は通信のレコードが定める key を core.RecordField の集合へ直す。
//
// 原文に出る key は recordFieldsOf が原文の並び順で並べる。当該レコードに出ない
// communicationOptionalKeys の key は、valueState が item_absent の項目として末尾に足す。
//
// key は定数で非空、足す値は item_absent であり、core の constructor の検査が失敗しないため
// error を返さない。
func communicationFieldsOf(record Record, eventTime core.Timestamp) []core.RecordField {
	fields := recordFieldsOf(record, eventTime)
	event := eventOf(record)
	for _, key := range communicationOptionalKeys {
		if record.FieldCount(key) > 0 {
			continue
		}
		field, _ := core.NewTextField(key, semanticOf(key, event), core.NewAbsentItemValue())
		fields = append(fields, field)
	}
	return fields
}

// observationKindOfCommunication は evt と subEvt の 2 件から core.ObservationKind を
// 組む。
//
// **推定した意味の文字列を、種別に依らない観測と同じ表から探す。** 2 つの経路が別の文字列を
// 持つと、同じ組の意味がレコードの種別によって変わる。
func observationKindOfCommunication(record Record) core.ObservationKind {
	status := subEventStatus(record)
	kind := core.ObservationKind{Raw: observationKindRaw(record), Status: status}
	if status == core.ObservationKindStatusInferred {
		kind.Meaning = inferredSubEvents[eventOf(record)][subEventOf(record)].meaning
	}
	return kind
}

// subEventStatus は subEvt の値から観測の種別の意味の状態を返す。
//
// 呼ぶのは ParseCommunication が subEvt を表の 5 つのいずれかだと確かめた後だけである。
// 表に無い値では、確かめていない意味を確定として返さないために undetermined を返す。
func subEventStatus(record Record) core.ObservationKindStatus {
	subEvent, found := record.Field(keySubEvent)
	if !found {
		return core.ObservationKindStatusUndetermined
	}
	status, known := communicationSubEventStatus[subEvent.Value()]
	if !known {
		return core.ObservationKindStatusUndetermined
	}
	return status
}
