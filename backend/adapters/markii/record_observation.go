package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// RecordObservationParserID は本解析の識別子である。parserVersion の材料の 1 つである。
//
// parserVersion はパーサー、対応形式、コード revision、設定から組む。
// 本 package が知るのはパーサーと対応形式までで、コード revision と設定を足して値を組むのは
// 取り込みの実行である。
const RecordObservationParserID = "markii-record-observation"

// RecordObservation は 1 件のレコードから、種別に依らずに読んだ観測を持つ。
//
// **プロセスと接続と親の参照を持たない。** psGUID と srcIP と parentGUID を持つ
// レコードでも、文字列は Fields が持つところまでである。参照を組むのはプロセス開始
// (ProcessStart) と通信 (Communication) の意味付けである。
//
// core の型を完成させない理由と、埋められない項目の欄を持たない理由は Communication と同じである。
type RecordObservation struct {
	// SequenceNumber は sn の 10 進整数の値である。読めなかったとき nil である。
	// core.RecordLocator の sequenceNumber の材料である。
	SequenceNumber *int64
	// LineNumber は収集元の中の行番号である。1 起点である。
	LineNumber int64
	// EventTime はヘッダーの時刻である。**そのレコードが記録した事象の時刻**を持つ。
	EventTime core.Timestamp
	// ObservationKind は evt と subEvt の 2 件である。status は documentedKindStatus が決める。
	ObservationKind core.ObservationKind
	// Fields は項目ごとの値の集合である。1 件目がヘッダーの時刻で、残りは原資料の
	// key の文字列を名前に持ち、原文の並び順を保つ。
	Fields []core.RecordField
}

// ParseRecordObservation は 1 件のレコードを、種別に依らない観測へ直す。
//
// **レコードの種別で受け付ける範囲を絞らない。** evt と subEvt の値が何であっても、
// recordObservationRequiredKeys を満たすレコードから観測を作る。意味を確かめた組で
// あるかは ObservationKind の Status が持つ。
//
// 返り値の 2 つは次の組み合わせを取る。
//
//	正常   : RecordObservation が完成した値、failure が nil
//	失敗   : RecordObservation が読めた位置だけを持つ値、failure が診断
//
// 失敗のときも読めた位置を返す。取り込みの実行が診断に位置を足せるようにするため
// である。sn を読む前に止まった失敗では SequenceNumber が nil のままになる。
//
// 返した failure は本解析が埋められる項目だけを埋めた値であり、そのままでは Validate を
// 通らない。
//
// 組む Fields は、当該レコードに出ている key だけを項目にする。出ない key を item_absent の
// 項目で補うのは、通信のレコードの communicationOptionalKeys だけである。
//
// 既知の制限: 任意の evt と subEvt の組が定める key の全数を列挙しない,
// 測れない - 組ごとの key の全数を確定できておらず、補うべき key の集合が決まらないため、
// 不足している項目の数を数える対象が無い,
// 組ごとの key の全数を確定できたとき、出ない key を item_absent の項目で補う
func ParseRecordObservation(record Record) (RecordObservation, *core.ImportFailure) {
	observation := RecordObservation{LineNumber: record.LineNumber()}

	if problem := requireSingleFields(record, recordObservationRequiredKeys); problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, recordObservationSemantics)
		return observation, &failure
	}

	sequenceNumber, problem := sequenceNumberOf(record)
	if problem != nil {
		failure := problem.importFailure(core.FailureStageFieldMap, record, recordObservationSemantics)
		return observation, &failure
	}
	observation.SequenceNumber = &sequenceNumber

	eventTime, readable := HeaderTimestamp(record)
	if !readable {
		problem := fieldProblem{
			expected: "a header that is a local date and time with the offset in the value",
			observed: "a header that cannot be read as a date and time",
		}
		failure := problem.importFailure(core.FailureStageNormalize, record, recordObservationSemantics)
		return observation, &failure
	}
	observation.EventTime = eventTime

	observation.ObservationKind = observationKindOfRecord(record)
	observation.Fields = recordFieldsOf(record, eventTime)
	// Web アクセスのレコードは接続先の欄を持たない。url の文字列から host を導き、
	// 接続先のノードを作れるようにする。
	if host, derived := urlHostField(record); derived {
		observation.Fields = append(observation.Fields, host)
	}
	// 符号化されたコマンド行は、原資料の文字列のままでは何を実行したかを読めない。復号した行を
	// 別の項目で持ち、原資料の文字列は cmd の項目に残す。
	if command, derived := decodedCommandField(record); derived {
		observation.Fields = append(observation.Fields, command)
	}
	// logonType は種別の名前とコードを 1 つの文字列に書く。コードを別の項目で持ち、Windows
	// イベントログの LogonType と同じ値で比べられるようにする。
	if code, derived := logonTypeCodeField(record); derived {
		observation.Fields = append(observation.Fields, code)
	}

	return observation, nil
}
