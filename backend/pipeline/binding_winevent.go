package pipeline

import (
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// WindowsEventFormats は Windows イベントログの adapter が読めると宣言した入力形式を、
// 走査器の作り方と組にする。**読める形式の定義元は winevent.Formats である。**
func WindowsEventFormats() []FormatRegistration {
	return registrationsOf(winevent.Formats(), newWindowsEventParser)
}

// windowsEventReader は読み取りの形式ごとの走査器である。どれも形式に依らない Event を返す。
type windowsEventReader interface {
	Reset(input io.Reader)
	Next() (winevent.Event, *core.ImportFailure, error)
}

// windowsConnectionMatchConditions は、Windows イベントログのレコードが外向きの通信の関連付けの
// 候補になったときに引き受ける条件の宣言である。
//
// 端末は、レコードを置いた端末のノードの鍵から導いた terminal.id の項目で比べる
// (core.TerminalMatchValue)。**Windows のレコードは端末の外部識別子の欄を持たない。**
//
// **接続先の IP と port を、Proxy のログが記録した要求先とは比べない。** 端末が記録した接続先は、
// 端末が実際に接続した相手であり、Proxy を経由した要求では Proxy のアドレスである。接続先 IP は、
// Proxy のログの収集元に付けた端末の IP と比べる (candidateLookup.proxyAddressesOf)。
// 接続元 port は両側が欄を持つときに比べる。
func windowsConnectionMatchConditions() []ConnectionMatchCondition {
	return []ConnectionMatchCondition{
		{
			ConditionKey:      core.ConditionKeyTerminalIpAssignment,
			Semantics:         []core.SemanticKey{core.SemanticKeyTerminalId},
			NarrowsCandidates: true,
		},
		{
			ConditionKey: core.ConditionKeyDestinationIp,
			Semantics:    []core.SemanticKey{core.SemanticKeyConnectionDestinationAddress},
		},
		{
			ConditionKey: core.ConditionKeyDestinationPort,
			Semantics:    []core.SemanticKey{core.SemanticKeyConnectionDestinationPort},
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
			ConditionKey:      core.ConditionKeyClientPort,
			Semantics:         []core.SemanticKey{core.SemanticKeyConnectionSourcePort},
			NarrowsCandidates: true,
		},
		{
			ConditionKey: core.ConditionKeyProcess,
			Semantics:    []core.SemanticKey{core.SemanticKeyProcessId, core.SemanticKeyProcessPid},
		},
	}
}

func newWindowsEventParser(format core.InputFormat, _ string) (SourceParser, error) {
	switch format.Key {
	case winevent.FormatKeyViewerCSV:
		return &windowsEventParser{format: format, reader: &winevent.CSVReader{}}, nil
	case winevent.FormatKeyEVTX:
		return &windowsEventParser{format: format, reader: &winevent.EVTXReader{}, rawTextConverted: true}, nil
	}
	return &windowsEventParser{format: format, reader: &winevent.XMLReader{}}, nil
}

type windowsEventParser struct {
	reader windowsEventReader
	format core.InputFormat
	// rawTextConverted は、走査器が返す原文が収集元の byte 列から組み立てた文字列であるかである。
	rawTextConverted bool
}

func (p *windowsEventParser) Identity() ParserIdentity {
	if p.format.Key == winevent.FormatKeyViewerCSV {
		return ParserIdentity{
			ParserID: p.format.ParserID, SupportedFormatVersion: winevent.ParserIDViewerCSV,
			FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
			ItemSemantics: winevent.ItemSemantics(),
			// 「日付と時刻」の欄は秒までを書く (adapters/winevent の viewerTimeOf)。
			TimePrecision: core.PrecisionSecond,
			// CSV は端末の欄を持たない。イベントビューアーは 1 台の端末のログを 1 つの file に
			// 書き出すため、端末を指定していない収集元を名前不明の端末 1 台に置く。
			//
			// 既知の制限: 転送したイベントを集めたログの CSV も 1 台の端末に置く,
			// 端末の欄が無いため、1 つの file が複数台の記録を持つかを原文から測れない,
			// 複数台の記録を集めた CSV を取り込む必要が出たとき、端末を分ける欄の読み方を決める
			RecordedByOneTerminal:     true,
			ConnectionRequestKinds:    []core.ObservationKindSelector{},
			ConnectionMatchConditions: windowsConnectionMatchConditions(),
			// CSV もプロバイダとイベント ID を XML と同じ欄の名前で持つ。
			FlowOperationKinds: winevent.FlowOperationKinds(),
			// CSV はレコード番号とチャネルの欄を持たず、同じ事象の転記を見分けられない。
			TranscriptIdentityItems:  []string{},
			CountsUnrenderedMessages: true,
			RecordNumbering:          RecordNumberingNames(winevent.NumberingNamesOf(p.format.Key)),
		}
	}
	// XML と EVTX は同じ System の項目を持つ。原文を組み立てるのは EVTX だけである。
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: p.format.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: winevent.ItemSemantics(),
		// SystemTime は 100 ナノ秒の単位まで書き、精度はマイクロ秒へ切り捨てる
		// (adapters/winevent の eventTimeOf)。
		TimePrecision: core.PrecisionMicrosecond,
		// 転送したイベントを集めたチャネルの file は、複数台の Computer を持つ。
		RecordingTerminalPerHostname: true,
		ConnectionRequestKinds:       []core.ObservationKindSelector{},
		ConnectionMatchConditions:    windowsConnectionMatchConditions(),
		TranscriptIdentityItems:      winevent.TranscriptIdentityItems(),
		ContentReplacementKinds:      winevent.ContentReplacementKinds(),
		FlowOperationKinds:           winevent.FlowOperationKinds(),
		RawTextConverted:             p.rawTextConverted,
		RecordNumbering:              RecordNumberingNames(winevent.NumberingNamesOf(p.format.Key)),
	}
}

// HasSignature は EVTX の形式だけが署名で判定する。XML と CSV の file は署名を持たない。
func (p *windowsEventParser) HasSignature(head []byte) bool {
	return p.format.Key == winevent.FormatKeyEVTX && winevent.HasEVTXSignature(head)
}

func (p *windowsEventParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

// SourceHeader は EVTX の file の見出しが記録した値を返す。XML と CSV の file は見出しを持たない。
func (p *windowsEventParser) SourceHeader() []core.RecordField {
	if evtx, ok := p.reader.(*winevent.EVTXReader); ok {
		return evtx.SourceHeader()
	}
	return nil
}

func (p *windowsEventParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	event, failure, err := p.reader.Next()
	source := event.Source
	parsed := ParsedRecord{
		RawText: source.RawText, LineNumber: source.LineNumber, ByteOffset: source.ByteOffset,
		ByteLength: &source.ByteLength,
	}
	// 行を持たない EVTX の 1 件は行数を持たない。
	if source.LineCount >= 1 {
		parsed.LineCount = &source.LineCount
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	observation, problem := winevent.Observe(event)
	if problem != nil {
		return parsed, problem, nil
	}
	parsed.ObservedAt = observation.EventTime
	parsed.Terminal = observation.Terminal
	parsed.MessageUnrendered = observation.MessageUnrendered
	parsed.TerminalCandidates = observation.TerminalCandidates
	if previous, current, renamed := winevent.ComputerRenameOf(event); renamed {
		parsed.TerminalNamings = []TerminalNaming{{Name: current, Previous: previous}}
	}
	parsed.Semantics = &RecordSemantics{
		ObservationKind: observation.ObservationKind,
		Fields:          observation.Fields,
		// 起動を記録したイベントかを決めるのは adapter のイベントの定義である。ProcessRef は
		// 端末の外部識別子を要し、イベントは端末を Computer のホスト名でしか名乗らないため
		// 組まない。プロセスの外部識別子を持つイベントでは、Fields の process.id が持つ。
		ProcessStart:        observation.ProcessStart,
		ProcessEnd:          observation.ProcessEnd,
		RemoteSession:       observation.RemoteSession,
		InboundConnection:   observation.InboundConnection,
		InboundGroupAddress: observation.InboundGroupAddress,
		OutboundConnection:  observation.OutboundConnection,
		SystemStart:         observation.SystemStart,
		AccountCreation:     observation.AccountCreation,
	}
	return parsed, nil, nil
}
