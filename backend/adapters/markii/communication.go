package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// CommunicationParserID は本解析の識別子である。parserVersion の材料の 1 つである。
//
// parserVersion はパーサー、対応形式、コード revision、設定から組む。
// 本 package が知るのはパーサーと対応形式までで、コード revision と設定を足して値を組むのは
// 取り込みの実行である。
const CommunicationParserID = "markii-communication"

// Communication は 1 件の通信のレコードから読んだ観測を持つ。
//
// **core の型を 1 つも完成させない。** core.RecordLocator と core.ProcessRef は
// sourceId と sourceContentSha256 を必須とし、2 項目を知るのは収集元を開く
// 取り込みの実行だけである。本型が持つのはその材料までである。
//
// 埋められない項目の欄を持たない。持つと、埋まっていない状態が zero value になり、
// 値が無い状態と値が空文字列である状態を分けられない。接続の開始と切断を結び付ける処理は
// pipeline/ の責務である。
type Communication struct {
	// SequenceNumber は sn の 10 進整数の値である。読めなかったとき nil である。
	// core.RecordLocator の sequenceNumber の材料である。
	SequenceNumber *int64
	// LineNumber は収集元の中の行番号である。1 起点である。
	LineNumber int64
	// ProcessGuid は psGUID の値である。core.ProcessRef の processGuid の材料である。
	ProcessGuid string
	// TerminalId は tmid の値である。core.ProcessRef の terminalId の材料である。
	TerminalId string
	// EventTime はヘッダーの時刻である。**そのレコードが記録した事象の時刻**を持つ。
	// 切断のレコードでは切断の時刻を持つ。
	EventTime core.Timestamp
	// SourceIp は srcIP である。key が出ないレコードでは valueState が item_absent に
	// なる。openUDP のレコードは 4 つの接続の相手の key を 1 つも持たない。
	SourceIp core.RawAndNormalized
	// SourcePort は srcPort である。文字列を持つだけで、整数へ直さない。
	SourcePort core.RawAndNormalized
	// DestIp は dstIP である。
	DestIp core.RawAndNormalized
	// DestPort は dstPort である。文字列を持つだけで、整数へ直さない。
	DestPort core.RawAndNormalized
	// ObservationKind は evt と subEvt の 2 件である。status は communicationSubEventStatus が決める。
	ObservationKind core.ObservationKind
	// Fields は項目ごとの値の集合である。1 件目がヘッダーの時刻で、続くのは原資料の
	// key の文字列を名前に持つ項目で、原文の並び順を保つ。末尾は当該レコードに出ない
	// communicationOptionalKeys の key を item_absent で持つ項目である。
	Fields []core.RecordField
	// Connect は、レコードが端末から dstIP へ始めた接続を記録したか (subEvt が con) である。
	// 形式が意味を定める subEvt のうち、con だけが接続を始めた側の記録である。
	Connect bool
}

// IsCommunication はレコードが通信のものであるかを返す。
//
// evt が net であり、かつ subEvt が communicationSubEventStatus の表にある値である
// レコードだけが真になる。判定に使うのは復号後の値である。別のバージョンが足した subEvt を、
// 意味を確かめないまま観測にしないため、evt が net であることだけで受けない。
func IsCommunication(record Record) bool {
	event, hasEvent := record.Field(keyEvent)
	subEvent, hasSubEvent := record.Field(keySubEvent)
	if !hasEvent || !hasSubEvent || event.Value() != communicationEvent {
		return false
	}
	_, known := communicationSubEventStatus[subEvent.Value()]
	return known
}

// ParseCommunication は 1 件の通信のレコードを観測へ直す。
//
// 返り値の 2 つは次の組み合わせを取る。
//
//	正常   : Communication が完成した値、failure が nil
//	失敗   : Communication が読めた位置だけを持つ値、failure が診断
//
// **失敗のときも読めた位置を返す。** 取り込みの実行が診断に位置を足せるようにするため
// である。sn を読む前に止まった失敗では SequenceNumber が nil のままになる。
//
// 返した failure は本解析が埋められる項目だけを埋めた値であり、そのままでは Validate を
// 通らない。
//
// 他の種別のレコードを渡した呼び出しは field_map の失敗を返す。呼ぶ側が
// IsCommunication で選ぶ。
func ParseCommunication(record Record) (Communication, *core.ImportFailure) {
	observation := Communication{LineNumber: record.LineNumber()}

	if !IsCommunication(record) {
		problem := fieldProblem{
			expected: "a record whose " + keyEvent + " is " + communicationEvent +
				" and whose " + keySubEvent + " is one of the values the communication parser reads",
			observed: "a record that does not carry the two values",
		}
		failure := problem.importFailure(core.FailureStageFieldMap, record, communicationSemantics)
		return observation, &failure
	}
	if problem := requireSingleFields(record, communicationRequiredKeys); problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, communicationSemantics)
		return observation, &failure
	}

	sequenceNumber, problem := sequenceNumberOf(record)
	if problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, communicationSemantics)
		return observation, &failure
	}
	observation.SequenceNumber = &sequenceNumber

	eventTime, readable := HeaderTimestamp(record)
	if !readable {
		problem := fieldProblem{
			expected: "a header that is a local date and time with the offset in the value",
			observed: "a header that cannot be read as a date and time",
		}
		failure := problem.importFailure(core.FailureStageNormalize, record, communicationSemantics)
		return observation, &failure
	}
	observation.EventTime = eventTime

	// 必須の key は requireSingleFields が 1 回ずつ出ることを確かめている。
	processGuid, _ := record.Field(keyProcessGuid)
	terminalId, _ := record.Field(keyTerminalId)
	observation.ProcessGuid = processGuid.Value()
	observation.TerminalId = terminalId.Value()

	observation.SourceIp = textValueOf(record, keySourceIp)
	observation.SourcePort = textValueOf(record, keySourcePort)
	observation.DestIp = textValueOf(record, keyDestIp)
	observation.DestPort = textValueOf(record, keyDestPort)
	observation.ObservationKind = observationKindOfCommunication(record)
	observation.Fields = communicationFieldsOf(record, eventTime)
	observation.Connect = subEventOf(record) == communicationSubEventConnect

	return observation, nil
}
