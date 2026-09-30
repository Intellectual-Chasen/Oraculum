package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// ProcessStartParserID は本解析の識別子である。parserVersion の材料の 1 つである。
//
// parserVersion はパーサー、対応形式、コード revision、設定から組む。
// 本 package が知るのはパーサーと対応形式までで、コード revision と設定を足して値を組むのは
// 取り込みの実行である。
const ProcessStartParserID = "markii-process-start"

// プロセス開始のレコードを選ぶ条件。
const (
	processStartEvent    = "ps"
	processStartSubEvent = "start"
)

// ProcessStart は 1 件のプロセス開始のレコードから読んだ観測を持つ。
//
// **core の型を 1 つも完成させない。** core.RecordLocator と core.ProcessRef は
// sourceId と sourceContentSha256 を必須とし、2 項目を知るのは収集元を開く
// 取り込みの実行だけである。本型が持つのはその材料までである。
//
// 埋められない項目の欄を持たない。持つと、埋まっていない状態が zero value になり、
// 値が無い状態と値が空文字列である状態を分けられない。
type ProcessStart struct {
	// SequenceNumber は sn の 10 進整数の値である。読めなかったとき nil である。
	// core.RecordLocator の sequenceNumber の材料である。
	SequenceNumber *int64
	// LineNumber は収集元の中の行番号である。1 起点である。
	LineNumber int64
	// ProcessGuid は psGUID の値である。core.ProcessRef の processGuid の材料である。
	ProcessGuid string
	// TerminalId は tmid の値である。core.ProcessRef の terminalId の材料である。
	TerminalId string
	// ParentGuid は parentGUID である。key が出ないレコードでは valueState が
	// item_absent になる。**親子の結び付けを行わない。** 文字列を持つだけである。
	ParentGuid core.RawAndNormalized
	// StartTime はヘッダーの時刻である。プロセスの起動時刻を持つ。
	StartTime core.Timestamp
	// ObservationKind は evt と subEvt の 2 件である。
	ObservationKind core.ObservationKind
	// Fields は項目ごとの値の集合である。1 件目がヘッダーの時刻で、残りは原資料の
	// key の文字列を名前に持ち、原文の並び順を保つ。
	Fields []core.RecordField
}

// IsProcessStart はレコードがプロセス開始のものであるかを返す。
//
// evt が ps かつ subEvt が start のレコードだけが真になる。どちらかの key が出ない
// レコードは偽である。**判定に使うのは復号後の値である。**
func IsProcessStart(record Record) bool {
	event, hasEvent := record.Field(keyEvent)
	subEvent, hasSubEvent := record.Field(keySubEvent)
	return hasEvent && hasSubEvent &&
		event.Value() == processStartEvent && subEvent.Value() == processStartSubEvent
}

// ParseProcessStart は 1 件のプロセス開始のレコードを観測へ直す。
//
// 返り値の 2 つは次の組み合わせを取る。
//
//	正常   : ProcessStart が完成した値、failure が nil
//	失敗   : ProcessStart が読めた位置だけを持つ値、failure が診断
//
// **失敗のときも読めた位置を返す。** 取り込みの実行が診断に位置を足せるようにするため
// である。sn を読む前に止まった失敗では SequenceNumber が nil のままになる。
//
// 返した failure は本解析が埋められる項目だけを埋めた値である。
// **そのままでは Validate を通らない。**
//
// 他の種別のレコードを渡した呼び出しは field_map の失敗を返す。呼ぶ側が
// IsProcessStart で選ぶ。
func ParseProcessStart(record Record) (ProcessStart, *core.ImportFailure) {
	observation := ProcessStart{LineNumber: record.LineNumber()}

	if !IsProcessStart(record) {
		problem := fieldProblem{
			expected: "a record whose " + keyEvent + " is " + processStartEvent +
				" and whose " + keySubEvent + " is " + processStartSubEvent,
			observed: "a record that does not carry the two values",
		}
		failure := problem.importFailure(core.FailureStageFieldMap, record, processStartSemantics)
		return observation, &failure
	}
	if problem := requireSingleFields(record, processStartRequiredKeys); problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, processStartSemantics)
		return observation, &failure
	}

	sequenceNumber, problem := sequenceNumberOf(record)
	if problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, processStartSemantics)
		return observation, &failure
	}
	observation.SequenceNumber = &sequenceNumber

	startTime, readable := HeaderTimestamp(record)
	if !readable {
		problem := fieldProblem{
			expected: "a header that is a local date and time with the offset in the value",
			observed: "a header that cannot be read as a date and time",
		}
		failure := problem.importFailure(core.FailureStageNormalize, record, processStartSemantics)
		return observation, &failure
	}
	observation.StartTime = startTime

	// 必須の key は requireSingleFields が 1 回ずつ出ることを確かめている。
	processGuid, _ := record.Field(keyProcessGuid)
	terminalId, _ := record.Field(keyTerminalId)
	observation.ProcessGuid = processGuid.Value()
	observation.TerminalId = terminalId.Value()

	observation.ParentGuid = textValueOf(record, keyParentGuid)
	observation.ObservationKind = observationKindOf(record)
	observation.Fields = recordFieldsOf(record, startTime)

	return observation, nil
}
