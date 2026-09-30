package pipeline

import (
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 要求行から導く項目の名前。応答の fields に出る name である。
const (
	apacheRequestMethodFieldName  = "requestMethod"
	apacheRequestTargetFieldName  = "requestTarget"
	apacheRequestVersionFieldName = "requestVersion"
)

// 導出の元と導き方を表す文字列。分析者が画面で読む値であるため日本語で書く。
const (
	derivationApacheRequestLine = "要求行を空白で 3 token に分けたうちの 1 つ"
)

// ApacheFormats は Apache HTTP Server の adapter が読めると宣言した入力形式を、走査器の
// 作り方と組にする。**読める形式の定義元は apache.Formats である。**
func ApacheFormats() []FormatRegistration {
	return registrationsOf(apache.Formats(), newApacheParser)
}

func newApacheParser(format core.InputFormat, _ string) (SourceParser, error) {
	switch format.Key {
	case apache.FormatKeyAccessCombined:
		return &apacheAccessParser{format: format}, nil
	case apache.FormatKeyError:
		return &apacheErrorParser{format: format}, nil
	default:
		return nil, fmt.Errorf("apache: unknown format key %q", format.Key)
	}
}

// apacheAccessSemantics はこの入力形式のレコードが持ちうる語彙の項目である。
//
// requestLine 欄そのものは固有の意味を持つが、そこから導く 3 項目
// (requestMethod / requestTarget / requestVersion) は語彙の項目を持つ。
func apacheAccessSemantics() []core.SemanticKey {
	order := []apache.AccessItemName{
		apache.AccessItemClientIP, apache.AccessItemIdent, apache.AccessItemUser,
		apache.AccessItemRequestTime, apache.AccessItemRequestLine, apache.AccessItemStatusCode,
		apache.AccessItemReplyBytes, apache.AccessItemReferer, apache.AccessItemUserAgent,
	}
	semantics := make([]core.SemanticKey, 0, len(order)+3)
	for _, item := range order {
		if semantic := apache.AccessSemanticOfItem(item); semantic != "" {
			semantics = append(semantics, semantic)
		}
	}
	semantics = append(semantics,
		apache.SemanticRequestMethod, apache.SemanticRequestTarget, apache.SemanticRequestVersion)
	return semantics
}

type apacheAccessParser struct {
	reader apache.AccessReader
	format core.InputFormat
}

func (p *apacheAccessParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: apache.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: apacheAccessSemantics(),
		// %t は秒までを書く (adapters/apache の ParseAccessTime)。
		TimePrecision: core.PrecisionSecond,
		// アクセスログは観測の種別の欄を持たない。レコードはすべて要求の記録である。
		ConnectionRequestKinds: []core.ObservationKindSelector{},
		// **接続の関連付けには参加しない。** combined は自身の端末の識別子を持たず、
		// %h は要求元 (接続元) を指す。既存の関連付けの条件は起点と候補の両側が接続先を名乗る
		// 前提であり、本形式の役割 (自身が接続先) には当てられない。接続の向きは 1 件の
		// レコードから決めない。
		ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
	}
}

func (p *apacheAccessParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *apacheAccessParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	parsed := ParsedRecord{
		RawText: record.RawText(), LineEnding: record.LineEnding(),
		LineNumber: record.LineNumber(), ByteOffset: record.ByteOffset(),
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	observedAt, failure := apache.ParseAccessTime(record)
	if failure != nil {
		return parsed, failure, nil
	}
	parsed.ObservedAt = &observedAt
	requestLine, failure := apache.ParseAccessRequestLine(record)
	if failure != nil {
		return parsed, failure, nil
	}
	parsed.Semantics = apacheAccessRecordSemantics(record, observedAt, requestLine)
	return parsed, nil, nil
}

// apacheAccessRecordSemantics は Apache のアクセスログのレコードを取り込みの実行が持つ組へ
// 対応付ける。
//
// Fields は adapter が返す combined の欄に、同じレコードの要求行から導いた requestMethod と
// requestTarget と requestVersion を足した集合である。**Endpoint を組まない。** combined は
// 自身の端末の識別子を持たず、接続の両端を確定できない。
func apacheAccessRecordSemantics(
	record apache.AccessRecord, requestTime core.Timestamp, requestLine apache.AccessRequestLine,
) *RecordSemantics {
	fields := apache.AccessRecordFields(record, requestTime)
	fields = append(fields,
		textField(apacheRequestMethodFieldName, apache.SemanticRequestMethod,
			normalizedText(requestLine.Method, requestLine.Method, derivationApacheRequestLine)),
		textField(apacheRequestTargetFieldName, apache.SemanticRequestTarget,
			normalizedText(requestLine.Target, requestLine.Target, derivationApacheRequestLine)),
		textField(apacheRequestVersionFieldName, apache.SemanticRequestVersion,
			normalizedText(requestLine.Protocol, requestLine.Protocol, derivationApacheRequestLine)))
	return &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          fields,
	}
}

type apacheErrorParser struct {
	reader apache.ErrorReader
	format core.InputFormat
}

func (p *apacheErrorParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: apache.ParserID,
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		ItemSemantics: apacheErrorSemantics(),
		// 時刻はマイクロ秒までを書く (adapters/apache の ParseErrorTime)。
		TimePrecision:             core.PrecisionMicrosecond,
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
	}
}

func apacheErrorSemantics() []core.SemanticKey {
	order := []apache.ErrorItemName{
		apache.ErrorItemTime, apache.ErrorItemModule, apache.ErrorItemSeverity,
		apache.ErrorItemPid, apache.ErrorItemTid, apache.ErrorItemClientIP,
		apache.ErrorItemClientPort, apache.ErrorItemMessage,
	}
	semantics := make([]core.SemanticKey, 0, len(order))
	for _, item := range order {
		if semantic := apache.ErrorSemanticOfItem(item); semantic != "" {
			semantics = append(semantics, semantic)
		}
	}
	return semantics
}

func (p *apacheErrorParser) Reset(input io.Reader) {
	p.reader.Reset(input)
}

func (p *apacheErrorParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	parsed := ParsedRecord{
		RawText: record.RawText(), LineEnding: record.LineEnding(),
		LineNumber: record.LineNumber(), ByteOffset: record.ByteOffset(),
	}
	if failure != nil || err != nil {
		return parsed, failure, err
	}
	observedAt, failure := apache.ParseErrorTime(record)
	if failure != nil {
		return parsed, failure, nil
	}
	parsed.ObservedAt = &observedAt
	parsed.Semantics = &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          apache.ErrorRecordFields(record, observedAt),
	}
	return parsed, nil, nil
}
