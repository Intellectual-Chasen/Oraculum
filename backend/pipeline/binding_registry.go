package pipeline

import (
	"io"
	"path/filepath"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winregistry"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RegistryFormats は registry の hive の adapter が読めると宣言した入力形式を、走査器の作り方と
// 組にする。**読める形式の定義元は winregistry.Formats である。**
func RegistryFormats() []FormatRegistration {
	return registrationsOf(winregistry.Formats(), newRegistryParser)
}

func newRegistryParser(format core.InputFormat, _ string) (SourceParser, error) {
	return &registryParser{format: format}, nil
}

type registryParser struct {
	reader winregistry.Reader
	format core.InputFormat
}

func (p *registryParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: winregistry.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		// 語彙へ写すのは、実行ファイルとタスクの登録を記録した key の値だけである。
		ItemSemantics: winregistry.ItemSemantics(),
		// FILETIME は 100 ナノ秒の単位を持ち、精度はマイクロ秒へ切り捨てる。
		TimePrecision: core.PrecisionMicrosecond,
		// hive の file は、その file を置いた端末が書く。
		RecordedByOneTerminal:     true,
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
		// 原文は cell の byte 列を 16 進で書いた文字列である。
		RawTextConverted: true,
	}
}

func (p *registryParser) HasSignature(head []byte) bool { return winregistry.HasHiveSignature(head) }

func (p *registryParser) CompanionSuffixes() []string {
	return winregistry.LogSuffixes()
}

func (p *registryParser) SetMembers(members []core.SourceMember) {
	converted := make([]winregistry.Member, len(members))
	for i, member := range members {
		converted[i] = winregistry.Member{
			Name: filepath.Base(member.OriginPath), Offset: member.ByteOffset, Size: member.SizeBytes,
		}
	}
	p.reader.SetMembers(converted)
}

func (p *registryParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *registryParser) SourceHeader() []core.RecordField {
	return p.reader.SourceHeader()
}

func (p *registryParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	parsed := ParsedRecord{RawText: record.RawText, ByteOffset: record.ByteOffset}
	if record.ByteLength > 0 {
		parsed.ByteLength = &record.ByteLength
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	parsed.ObservedAt = record.ObservedAt
	parsed.TerminalCandidates = record.TerminalCandidates
	for _, name := range record.TerminalNames {
		parsed.TerminalNamings = append(parsed.TerminalNamings, TerminalNaming{Name: name})
	}
	parsed.Semantics = &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          record.Fields,
	}
	return parsed, nil, nil
}
