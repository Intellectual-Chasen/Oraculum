package pipeline

import (
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/prefetch"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// PrefetchFormats は Prefetch の adapter が読めると宣言した入力形式を、走査器の作り方と
// 組にする。**読める形式の定義元は prefetch.Formats である。**
func PrefetchFormats() []FormatRegistration {
	return registrationsOf(prefetch.Formats(), newPrefetchParser)
}

func newPrefetchParser(format core.InputFormat, _ string) (SourceParser, error) {
	return &prefetchParser{format: format}, nil
}

type prefetchParser struct {
	reader prefetch.Reader
	format core.InputFormat
}

func (p *prefetchParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: prefetch.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: prefetch.ItemSemantics(),
		// FILETIME は 100 ナノ秒の単位を持ち、精度はマイクロ秒へ切り捨てる。
		TimePrecision: core.PrecisionMicrosecond,
		// Prefetch の file は、その file を置いた端末が書く。file は端末を名乗らない。
		// 同じ端末の file を 1 台に置くには、収集元ごとに同じ端末の外部識別子を指定する。
		RecordedByOneTerminal:     true,
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
		// 原文は file の byte 列を 16 進で書いた文字列である。
		RawTextConverted: true,
	}
}

func (p *prefetchParser) HasSignature(head []byte) bool { return prefetch.HasSignature(head) }

func (p *prefetchParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *prefetchParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	// レコードの位置は file の先頭から全体である。
	parsed := ParsedRecord{RawText: record.RawText, ByteLength: &record.ByteLength}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	parsed.ObservedAt = prefetch.LastRunTime(record.File)
	fields := prefetch.Fields(record.File)
	parsed.Semantics = &RecordSemantics{
		ObservationKind:      core.ObservationKind{Raw: []core.RecordField{}},
		Fields:               fields,
		AdditionalEventTimes: prefetch.AdditionalRunTimes(fields),
	}
	return parsed, nil, nil
}
