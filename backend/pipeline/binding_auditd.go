package pipeline

import (
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AuditdFormatKey は Linux auditd の監査ログの形式の key である。
// 取り込みの実行と test が入力形式を指定するときに使う。
const AuditdFormatKey = auditd.FormatKeyAuditLog

// AuditdFormats は Linux auditd の監査ログの宣言と、走査器の作り方を組にして返す。
//
// **入力形式の知識は adapters/auditd が持つ。** 本 file だけが同 package を import する。
func AuditdFormats() []FormatRegistration {
	return registrationsOf(auditd.Formats(), func(
		format core.InputFormat, spec string,
	) (SourceParser, error) {
		return newAuditdParser(format, spec)
	})
}

func newAuditdParser(format core.InputFormat, _ string) (SourceParser, error) {
	return &auditdParser{format: format}, nil
}

type auditdParser struct {
	reader auditd.Reader
	format core.InputFormat
}

func (p *auditdParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID:      p.format.ParserID,
		FormatKey:     p.format.Key,
		PositionKind:  p.format.PositionKind,
		ItemSemantics: auditd.ItemSemantics(),
		// 事象を指す鍵の秒未満はミリ秒 3 桁である (adapters/auditd の EventTime)。
		TimePrecision: core.PrecisionMillisecond,
		// ログイン名は、監査ログを書いた端末にログインした利用者である
		// (adapters/auditd の AUID)。
		AccountNamesLocalToTerminal: true,
		// 監査ログは、その端末の kernel が書いた記録である。
		RecordedByOneTerminal: true,
	}
}

func (p *auditdParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *auditdParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	byteLength := record.ByteLength()
	lineCount := record.LineCount()
	parsed := ParsedRecord{
		RawText: record.RawText(), LineEnding: record.LineEnding(),
		LineNumber: record.LineNumber(), ByteOffset: record.ByteOffset(),
		ByteLength: &byteLength, LineCount: &lineCount,
	}
	if err != nil {
		return parsed, failure, err
	}
	// 先頭が切れている事象も読めた範囲を持つ。診断を返しながら意味付けも行う。
	semantics, observedAt, problem := auditdSemantics(record)
	if problem != nil {
		return parsed, firstFailure(failure, problem), nil
	}
	parsed.ObservedAt = observedAt
	parsed.Semantics = semantics
	return parsed, failure, nil
}

// firstFailure は 2 つの診断のうち先に起きたものを返す。
// 走査の診断は意味付けより前の段階で起きるため、走査の診断を優先する。
func firstFailure(scan, semantics *core.ImportFailure) *core.ImportFailure {
	if scan != nil {
		return scan
	}
	return semantics
}

// auditdSemantics は 1 事象を取り込みの実行が持つ組へ対応付ける。
//
// **ProcessRef と ParentProcessId と ProcessStart を持たない。** auditd はプロセスへ
// 一意な識別子を振らず、`pid` を時間を跨いだ同一性の鍵にできない。一意な識別子を
// 持たない対象の同一性は、識別が有効な区間で区切る。プロセスの同一性と親子の候補は、
// 識別が有効な区間で区切る段階が組む。
//
// 接続の欄を持たないため Endpoint を持たない。
func auditdSemantics(record auditd.Record) (*RecordSemantics, *core.Timestamp, *core.ImportFailure) {
	fields, err := auditd.RecordFields(record)
	if err != nil {
		return nil, nil, auditdSemanticsFailure(record,
			"the fields of the event read as text values",
			"building the fields stopped: "+err.Error())
	}
	observedAt, err := auditd.EventTime(record.Event())
	if err != nil {
		return nil, nil, auditdSemanticsFailure(record,
			"the event time read from the seconds and milliseconds of the event key",
			"building the event time stopped: "+err.Error())
	}
	observationKind, err := auditdObservationKind(record)
	if err != nil {
		return nil, nil, auditdSemanticsFailure(record,
			"the observation kind read from the type of the event",
			"building the observation kind stopped: "+err.Error())
	}
	return &RecordSemantics{
		ObservationKind: observationKind,
		Fields:          fields,
	}, &observedAt, nil
}

// auditdObservationKind は事象の観測の種別を組む。
//
// 状態は determined である。`type` の値が表す事象の意味は auditd の書式が定めており、
// レコードの出方からの推定ではない。
// 種別を読めた行が 1 つも無い事象では、要素数 0 の集合を持ち、状態は出ない。
func auditdObservationKind(record auditd.Record) (core.ObservationKind, error) {
	field, found, err := auditd.ObservationKindRaw(record)
	if err != nil {
		return core.ObservationKind{}, err
	}
	kind := core.ObservationKind{}
	if found {
		kind = core.ObservationKind{
			Raw:    []core.RecordField{field},
			Status: core.ObservationKindStatusDetermined,
		}
	}
	if err := kind.Validate(); err != nil {
		return core.ObservationKind{}, err
	}
	return kind, nil
}

// auditdSemanticsFailure は意味付けが止まった診断を返す。
func auditdSemanticsFailure(
	record auditd.Record, interpretation, observed string,
) *core.ImportFailure {
	lineNumber := record.LineNumber()
	byteOffset := record.ByteOffset()
	return &core.ImportFailure{
		DiagnosisClass:  core.DiagnosisClassUndetermined,
		Stage:           core.FailureStageFieldMap,
		LineNumber:      &lineNumber,
		ByteOffset:      &byteOffset,
		Interpretation:  interpretation,
		ExpectedMeaning: "an auditd event carried as fields, an event time, and an observation kind",
		ObservedResult:  observed,
		UnresolvedReason: "the value cannot be read as the meaning the format defines. " +
			"a malformed input, an unsupported output of another version, and a defect of " +
			"this parser are not told apart by the value alone",
	}
}
