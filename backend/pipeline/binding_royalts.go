package pipeline

import (
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/royalts"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RoyalTSFormats は Royal TS 文書の adapter が読めると宣言した入力形式を、走査器の作り方と
// 組にする。**読める形式の定義元は royalts.Formats である。**
func RoyalTSFormats() []FormatRegistration {
	return registrationsOf(royalts.Formats(), newRoyalTSParser)
}

func royalTSItemSemantics() []core.SemanticKey {
	return []core.SemanticKey{
		core.SemanticKeyConnectionDestinationAddress, core.SemanticKeyConnectionDestinationHostname,
		core.SemanticKeyAccountName,
	}
}

func newRoyalTSParser(format core.InputFormat, _ string) (SourceParser, error) {
	return &royalTSParser{format: format}, nil
}

type royalTSParser struct {
	reader royalts.Reader
	format core.InputFormat
}

func (p *royalTSParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: royalts.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: royalTSItemSemantics(),
		// Royal TS 文書は接続を観測した記録ではなく、保存された接続項目である。
		// 時刻の項目を持たないため、この収集元の時刻精度は未使用のままにする
		// (TimePrecision を宣言しても、時刻を持たないレコードの関連付けには使われない)。
		TimePrecision:             core.PrecisionSecond,
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
	}
}

func (p *royalTSParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *royalTSParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	connection, failure, err := p.reader.Next()
	parsed := ParsedRecord{
		RawText: connection.RawText(), LineNumber: connection.LineNumber(),
		ByteOffset: connection.ByteOffset(),
	}
	sequence := connection.Sequence()
	if sequence > 0 {
		parsed.SequenceNumber = &sequence
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	parsed.Semantics = &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          royalts.ConnectionFields(connection),
	}
	return parsed, nil, nil
}
