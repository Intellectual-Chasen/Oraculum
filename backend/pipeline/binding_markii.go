package pipeline

import (
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 接続の 4 項目の名前。原資料の key の文字列である。
const (
	markiiSourceIpFieldName   = "srcIP"
	markiiSourcePortFieldName = "srcPort"
	markiiDestIpFieldName     = "dstIP"
	markiiDestPortFieldName   = "dstPort"
)

// markiiStartTimeNoteText は起動時刻の欄に添える注記である。分析者が画面で読む値であるため
// 日本語で書く。
//
// 既知の制限: 注記の文字列を markii の全レコードで固定する,
// evt=ps subEvt=start のレコードは sTime を持たず、起動時刻をヘッダーの時刻が持つ,
// sTime を持つ markii のレコードを確認したとき、レコードごとに分ける注記に見直す
const markiiStartTimeNoteText = "起動時刻はレコードのヘッダーの時刻が持つ"

// MarkIIFormats は markii の adapter が読めると宣言した入力形式を、走査器の作り方と
// 組にする。**読める形式の定義元は markii.Formats である。**
//
// プロセス開始と通信と種別に依らない意味付けの 3 つとも sn を必須の key に置くため、
// 取り込みに成功したレコードは sn を持ち、sn を読めないレコードは stage が field_map の
// 失敗になる。行番号で指すのは、その失敗の recordRef である。
func MarkIIFormats() []FormatRegistration {
	return registrationsOf(markii.Formats(), newMarkIIParser)
}

// markiiConnectionRequestKinds は、外向きの通信の要求を記録した markii の観測の種別である。
// 欄の名前と value の文字列の定義元は markii である。
//
// `dcon` は切断、`acpt` は着信の受理、`openUDP` は UDP の口を開いた記録であり、別の収集元が
// 記録した要求の相手になり得ない。関連付けの相手に数えるのは `con` と `est` である。
func markiiConnectionRequestKinds() []core.ObservationKindSelector {
	return []core.ObservationKindSelector{
		markii.ObservationKindSelectorOf(markii.EventNetwork, markii.SubEventConnect),
		markii.ObservationKindSelectorOf(markii.EventNetwork, markii.SubEventEstablish),
	}
}

// markiiConnectionOpenKinds は、接続を開いた記録の markii の観測の種別 (`con`) である。
// `est` は端点を入れ替えて記録することがあり、開いた記録に数えない。
//
// 既知の制限: `acpt` を開閉を読めない記録として扱う。`acpt` の srcIP は相手の端末、dstIP は
// 自端末であり、`con` と `dcon` の srcIP は自端末である, 受け付けた側の `dcon` がどちらの向きで
// 端点を書くかを形式が定めず、repo の中に `acpt` と組になる `dcon` の例が無い,
// 受け付けた側の `dcon` の向きを示す定義か実例が出たとき、向きをそろえて `acpt` を開いた記録に足す
func markiiConnectionOpenKinds() []core.ObservationKindSelector {
	return []core.ObservationKindSelector{
		markii.ObservationKindSelectorOf(markii.EventNetwork, markii.SubEventConnect),
	}
}

// markiiConnectionCloseKinds は、接続を閉じた記録の markii の観測の種別 (`dcon`) である。
func markiiConnectionCloseKinds() []core.ObservationKindSelector {
	return []core.ObservationKindSelector{
		markii.ObservationKindSelectorOf(markii.EventNetwork, markii.SubEventClose),
	}
}

// markiiConnectionMatchConditions は、markii のレコードが外向きの通信の関連付けで引き受ける
// 条件の宣言である。
//
// 端末の外部識別子は tmid の欄が持つ。接続元と接続先は通信のレコードの srcIP と srcPort と
// dstIP と dstPort が持つ (markiiCommunicationSemantics)。利用者は usr の欄が
// account.name、evtUsr の欄が event.account_name を持つため、探す順に 2 つを挙げる。
func markiiConnectionMatchConditions() []ConnectionMatchCondition {
	return []ConnectionMatchCondition{
		{
			ConditionKey:      core.ConditionKeyTerminalIpAssignment,
			Semantics:         []core.SemanticKey{core.SemanticKeyTerminalId},
			NarrowsCandidates: true,
		},
		{
			ConditionKey:      core.ConditionKeyDestinationIp,
			Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationAddress},
			NarrowsCandidates: true,
		},
		{
			ConditionKey:      core.ConditionKeyDestinationPort,
			Semantics:         []core.SemanticKey{core.SemanticKeyConnectionDestinationPort},
			NarrowsCandidates: true,
		},
		{
			ConditionKey: core.ConditionKeySecondOfTime,
			Semantics:    []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey: core.ConditionKeySubSecondOfTime,
			Semantics:    []core.SemanticKey{core.SemanticKeyEventTime},
		},
		{
			ConditionKey: core.ConditionKeyClientPort,
			Semantics:    []core.SemanticKey{core.SemanticKeyConnectionSourcePort},
		},
		{
			ConditionKey: core.ConditionKeyProcess,
			Semantics:    []core.SemanticKey{core.SemanticKeyProcessId},
		},
		{
			ConditionKey: core.ConditionKeyUser,
			Semantics: []core.SemanticKey{
				core.SemanticKeyAccountName, core.SemanticKeyEventAccountName,
			},
		},
	}
}

func newMarkIIParser(format core.InputFormat, _ string) (SourceParser, error) {
	return &markiiParser{format: format}, nil
}

type markiiParser struct {
	reader markii.Reader
	format core.InputFormat
}

func (p *markiiParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: markii.SupportedFormatVersion,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: markii.ItemSemantics(),
		// クライアントログのヘッダーの時刻はミリ秒までを書く (adapters/markii の headerTime)。
		TimePrecision:             core.PrecisionMillisecond,
		ConnectionRequestKinds:    markiiConnectionRequestKinds(),
		ConnectionOpenKinds:       markiiConnectionOpenKinds(),
		ConnectionCloseKinds:      markiiConnectionCloseKinds(),
		ConnectionMatchConditions: markiiConnectionMatchConditions(),
		ContentReplacementKinds:   markii.ContentReplacementKinds(),
		FlowOperationKinds:        markii.FlowOperationKinds(),
		TranscriptIdentityItems:   markii.TranscriptIdentityItems(),
		StartTimeNoteText:         markiiStartTimeNoteText,
	}
}

func (p *markiiParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *markiiParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	parsed := ParsedRecord{
		RawText: record.RawText(), LineEnding: record.LineEnding(),
		LineNumber: record.LineNumber(), ByteOffset: record.ByteOffset(),
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	if sequence, ok := markii.SequenceNumber(record); ok {
		parsed.SequenceNumber = &sequence
	}
	// 端末の項目はレコードの種別ごとの分岐より前で埋める。プロセス開始でも通信でも
	// ないレコードも端末の欄を持ち、IP から端末への割当の材料になる。
	// TerminalFields が偽を返すのは要素数 0 の集合を返すときだけであり、2 つ目の返り値を捨てる。
	parsed.Terminal, _ = markii.TerminalFields(record)
	switch {
	case markii.IsProcessStart(record):
		observation, problem := markii.ParseProcessStart(record)
		if problem == nil {
			parsed.ObservedAt = &observation.StartTime
			parsed.Semantics = markiiProcessStartSemantics(observation)
		}
		return parsed, problem, nil
	case markii.IsCommunication(record):
		observation, problem := markii.ParseCommunication(record)
		if problem == nil {
			parsed.ObservedAt = &observation.EventTime
			parsed.Semantics = markiiCommunicationSemantics(observation)
		}
		return parsed, problem, nil
	default:
		observation, problem := markii.ParseRecordObservation(record)
		if problem == nil {
			parsed.ObservedAt = &observation.EventTime
			parsed.Semantics = markiiRecordSemantics(observation)
		}
		return parsed, problem, nil
	}
}

// markiiRecordSemantics は種別に依らない観測を取り込みの実行が持つ組へ対応付ける。
//
// **ObservationKind と Fields の 2 つだけを持つ。** psGUID と srcIP と parentGUID を持つ
// レコードでも、ProcessRef と Endpoint と ParentProcessId を組まない。
func markiiRecordSemantics(observation markii.RecordObservation) *RecordSemantics {
	return &RecordSemantics{
		ObservationKind: observation.ObservationKind,
		Fields:          observation.Fields,
	}
}

// markiiProcessStartSemantics はプロセス開始の観測を取り込みの実行が持つ組へ対応付ける。
//
// ProcessRef の SourceContentSha256 と SourceId は走査より後の段階が埋める。
//
// **ProcessStart を真にするのは本関数だけである。** markii.ParseProcessStart が受け付ける
// のは evt が ps で subEvt が start の組のレコードだけであり、その組が起動の記録である
// (adapters/markii/process_start_fields.go の observationKindOf)。
func markiiProcessStartSemantics(observation markii.ProcessStart) *RecordSemantics {
	parentProcessId := observation.ParentGuid
	return &RecordSemantics{
		ObservationKind: observation.ObservationKind,
		Fields:          observation.Fields,
		ProcessRef: &core.ProcessRef{
			ProcessId: observation.ProcessGuid, TerminalId: observation.TerminalId,
		},
		ParentProcessId: &parentProcessId,
		ProcessStart:    true,
	}
}

// markiiCommunicationSemantics は通信の観測を取り込みの実行が持つ組へ対応付ける。
//
// 接続の項目は adapter が返した集合から名前で選ぶ。**semantic を binding が付け直さない。**
// 意味を決めるのは原資料の key を読む adapter である。
func markiiCommunicationSemantics(observation markii.Communication) *RecordSemantics {
	return &RecordSemantics{
		ObservationKind: observation.ObservationKind,
		Fields:          observation.Fields,
		ProcessRef: &core.ProcessRef{
			ProcessId: observation.ProcessGuid, TerminalId: observation.TerminalId,
		},
		Endpoint: &RecordEndpoint{
			ClientEndpoint: fieldsNamed(observation.Fields,
				markiiSourceIpFieldName, markiiSourcePortFieldName),
			Destination: fieldsNamed(observation.Fields,
				markiiDestIpFieldName, markiiDestPortFieldName),
		},
		OutboundConnection: observation.Connect,
	}
}
