package pipeline

import (
	"cmp"
	"io"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 要求行から導く項目の名前。応答の fields に出る name である。
const (
	requestMethodFieldName     = "requestMethod"
	requestTargetFieldName     = "requestTarget"
	requestTargetHostFieldName = "requestTargetHost"
	requestTargetPortFieldName = "requestTargetPort"
	requestVersionFieldName    = "requestVersion"
)

// 導出の元と導き方を表す文字列。分析者が画面で読む値であるため日本語で書く。
const (
	derivationRequestTargetHost = "要求先の URI の authority から userinfo と port と IPv6 の角括弧を外した host"
	derivationAuthorityPort     = "要求先の authority が書いた port"
	derivationSchemeDefaultPort = "要求先の scheme の既定の port"
	derivationRequestVersion    = "要求行の 3 番目の token を小文字へ直した文字列"
)

// derivationAuthorityAbsent と derivationPortAbsent は、導けなかった理由と導出の元にした
// %ru の文字列を 1 文で持つ。NewDerivationUndeterminedValue が rawText を拒むため、文字列の
// 置き場は derivation である。
//
// 文字列の分割が拒む制御 byte は %ru の literal な CR / LF / TAB / NUL の 4 つ、authority の
// 検査が読む範囲は authority 部分だけである。authority の外に置かれた他の制御 byte は
// %ru の文字列のまま derivation に残り、無害化は出力境界だけが担う。
func derivationAuthorityAbsent(rawTarget string) string {
	return "要求先 %ru の文字列 " + rawTarget + " が authority を持たない"
}

func derivationPortAbsent(rawTarget string) string {
	return "要求先 %ru の文字列 " + rawTarget + " が port と scheme のどちらも書いていない"
}

func derivationAuthorityTruncated(rawTarget string) string {
	return "要求先 %ru の文字列 " + rawTarget + " が authority の終わりの前で切れている"
}

// requestTargetTruncated は、要求先を読んだ欄が途中で切れているかを返す。欄は
// squid.ParseRequestLine と同じ順に探す。
func requestTargetTruncated(record squid.Record) bool {
	for _, name := range []squid.ItemName{squid.ItemRequestLine, squid.ItemRequestURL, squid.ItemClientRequestURL} {
		if item, found := record.Item(name); found {
			return item.Truncated()
		}
	}
	return false
}

// schemeDefaultPort は scheme から接続先 port を導く表である。
// scheme の文字列は大文字と小文字を区別しないため、小文字へ直してから探す。
//
// 既知の制限: 既定の port を導ける scheme を http と https の 2 つに限る,
// 測る対象が無い。対応する scheme の範囲であり、性能でも正答率でもない。
// 表に無い scheme の要求先は requestTargetPort が derivation_undetermined になり、
// 導けなかった状態として応答に出る,
// 表に無い scheme の要求先を収集元で確認したときに、その scheme の既定の port を足す
var schemeDefaultPort = map[string]string{"http": "80", "https": "443"}

// SquidFormats は Squid の adapter が読めると宣言した入力形式を、走査器の作り方と
// 組にする。**読める形式の定義元は squid.Formats である。**
func SquidFormats() []FormatRegistration {
	return registrationsOf(squid.Formats(), newSquidParser)
}

// squidConnectionMatchConditions は、Squid のレコードが外向きの通信の関連付けで引き受ける
// 条件の宣言である。
//
// 端末の外部識別子は別の収集元の割当から導いた項目が持つ (FieldsBuilder)。接続先の IP と
// port は %ru の要求先から導く 2 項目が持つ (requestTargetHost と requestTargetPort)。
// 利用者は %[un の欄が account.name を持つ。
//
// 接続元 port とプロセスの 2 条件は、combined が欄を持たない状態を分析者へ出すために挙げる。
// 段階の use は、相手の収集元が欄を持つかを見て core が決める (core の conditionUse)。
func squidConnectionMatchConditions() []ConnectionMatchCondition {
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
			Semantics:    []core.SemanticKey{core.SemanticKeyAccountName},
		},
	}
}

func newSquidParser(format core.InputFormat, formatSpec string) (SourceParser, error) {
	layout, err := squid.LayoutForSpec(formatSpec)
	if err != nil {
		return nil, err
	}
	return &squidParser{layout: layout, format: format}, nil
}

type squidParser struct {
	reader squid.Reader
	layout squid.Layout
	format core.InputFormat
}

func (p *squidParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: p.format.ParserID, SupportedFormatVersion: p.layout.Spec(),
		FormatKey: p.format.Key, PositionKind: p.format.PositionKind,
		FormatSpec:    p.layout.Spec(),
		ItemSemantics: squidItemSemantics(p.layout),
		TimePrecision: squid.TimePrecisionOf(p.layout),
		// アクセスログは観測の種別の欄を持たない。レコードはすべて要求の記録である。
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: squidConnectionMatchConditions(),
		// アクセスログの 1 行が 1 件の要求であり、同じ事象を 2 回転記する欄を持たない。
		TranscriptIdentityItems: []string{},
		ProxyRequestLog:         true,
		// %Ss:%Sh は組み込みの並びの欄、%Ss は logformat の欄である。
		RequestStatusItems: []string{string(squid.ItemSquidStatus), string(squid.ItemSquidRequestStatus)},
	}
}

// squidItemSemantics は欄の並びから、この収集元のレコードが持ちうる語彙の項目を返す。
//
// **要求先の欄から導く項目を本 file が足す。** squid.SemanticOfItem は要求行と %ru の欄に
// 語彙の項目を持たず、RequestTargetHostSemantic は RequestLine を引数に取るため欄の名前
// からは取り出せない。足した項目を実際に作るのは同 file の squidSemantics である。
// 要求先の authority はホスト名と IP のどちらにもなるため、両方を入れる。要求行の HTTP のバージョンは
// 要求行の欄だけが書く。
func squidItemSemantics(layout squid.Layout) []core.SemanticKey {
	order := squid.ItemOrderOf(layout)
	semantics := make([]core.SemanticKey, 0, len(order))
	seen := make(map[core.SemanticKey]struct{}, len(order))
	add := func(semantic core.SemanticKey) {
		if semantic == "" {
			return
		}
		if _, duplicate := seen[semantic]; duplicate {
			return
		}
		seen[semantic] = struct{}{}
		semantics = append(semantics, semantic)
	}
	for _, item := range order {
		switch item {
		case squid.ItemRequestLine:
			add(squid.SemanticRequestMethod)
			add(core.SemanticKeyConnectionDestinationAddress)
			add(core.SemanticKeyConnectionDestinationHostname)
			add(squid.SemanticRequestTargetPort)
			add(core.SemanticKeyHttpRequestUrl)
			add(squid.SemanticRequestVersion)
		case squid.ItemRequestURL, squid.ItemClientRequestURL:
			add(core.SemanticKeyConnectionDestinationAddress)
			add(core.SemanticKeyConnectionDestinationHostname)
			add(squid.SemanticRequestTargetPort)
			add(core.SemanticKeyHttpRequestUrl)
		default:
			add(squid.SemanticOfItem(item))
		}
	}
	return semantics
}

func (p *squidParser) Reset(input io.Reader) {
	p.reader.Reset(input, p.layout)
}

func (p *squidParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	record, failure, err := p.reader.Next()
	parsed := ParsedRecord{
		RawText: record.RawText(), LineEnding: record.LineEnding(),
		LineNumber: record.LineNumber(), ByteOffset: record.ByteOffset(),
	}
	// **途中で切れた行も、読めた欄を持つレコードとして返す。** 失敗の記録は残し、切れる前の
	// 欄が記録した要求を根拠から外さない。
	truncated := failure
	if err != nil || failure != nil && !failure.RecordTruncated {
		return parsed, failure, err
	}
	// 時刻の欄と要求先の欄を持たない並びでは、時刻と要求先を導かない。
	var requestTime core.Timestamp
	if _, hasTime := record.Item(squid.ItemRequestTime); hasTime {
		if requestTime, failure = squid.ParseRequestTime(record); failure != nil {
			return parsed, cmp.Or(truncated, failure), nil
		}
		parsed.ObservedAt = &requestTime
	}
	var requestLine *squid.RequestLine
	if squid.CarriesRequestTarget(record) {
		parsedLine, failure := squid.ParseRequestLine(record)
		switch {
		case failure == nil:
			requestLine = &parsedLine
		case truncated == nil:
			return parsed, failure, nil
		}
	}
	parsed.Semantics = squidSemantics(record, requestTime, requestLine)
	return parsed, truncated, nil
}

// squidSemantics は Squid のレコードを取り込みの実行が持つ組へ対応付ける。
//
// Fields は adapter が返す欄に、同じレコードの要求先から導いた項目を足した集合である。
// 足す項目は requestTargetHost と requestTargetPort と requestTarget、要求行の欄を持つときは requestVersion、
// 要求行の欄を持ち %rm の欄を持たないときは requestMethod である。要求先の欄 (%ru) だけを
// 持つ並びは method の文字列を持たないため、requestMethod を足さない。**別の収集元から導く
// clientTerminal と clientTerminalName、入力形式に欄そのものが無い clientPort と process を
// 足すのは、取り込みの実行の段階 2 (FieldsBuilder) である**。
//
// アクセスログは観測の種別の欄を持たないため、ObservationKind の Raw は要素数 0 になる。
// プロセスの欄も持たないため ProcessRef を持たない。requestLine が nil のレコードは要求先を
// 書いていない。
func squidSemantics(
	record squid.Record, requestTime core.Timestamp, requestLine *squid.RequestLine,
) *RecordSemantics {
	fields := squid.RecordFields(record, requestTime)
	if requestLine != nil {
		_, hasLine := record.Item(squid.ItemRequestLine)
		if _, hasMethod := record.Item(squid.ItemRequestMethod); hasLine && !hasMethod {
			fields = append(fields, textField(requestMethodFieldName, squid.SemanticRequestMethod,
				presentText(requestLine.Method)))
		}
		target, host, port := presentText(requestLine.RawTarget), requestTargetHost(*requestLine),
			requestTargetPort(*requestLine)
		if requestTargetTruncated(record) {
			target, _ = core.NewRawValue(core.ValueStateTruncated, requestLine.RawTarget)
			// authority の途中で切れた要求先から、切れた host と port を導かない。
			if _, rest, found := strings.Cut(requestLine.RawTarget, "://"); !found || !strings.ContainsAny(rest, "/?#") {
				host = undeterminedText(derivationAuthorityTruncated(requestLine.RawTarget))
				port = host
			}
		}
		fields = append(fields,
			textField(requestTargetHostFieldName, squid.RequestTargetHostSemantic(*requestLine), host),
			textField(requestTargetPortFieldName, squid.SemanticRequestTargetPort, port),
			// 要求先の原資料の文字列を百分率符号化のまま持つ。URL の断片をつなぐ処理が読む。
			textField(requestTargetFieldName, core.SemanticKeyHttpRequestUrl, target),
		)
		if hasLine {
			fields = append(fields, textField(requestVersionFieldName, squid.SemanticRequestVersion,
				requestVersion(*requestLine)))
		}
	}
	semantics := &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          fields,
	}
	// アクセスログは接続元 port の欄を持たないため、接続元の要素は接続元 IP の 1 件で
	// ある。接続先の要素も要求先の host の 1 件である。同じレコードの
	// %ru から導いた port は Fields が持ち、関連付けの鍵に使う値と応答へ出す値を同じ
	// 1 つの項目から読む。どちらかの欄を持たない並びは接続の組を持たない。
	client := fieldsNamed(fields, string(squid.ItemClientIP))
	destination := fieldsNamed(fields, requestTargetHostFieldName)
	if len(client) > 0 && len(destination) > 0 {
		semantics.Endpoint = &RecordEndpoint{ClientEndpoint: client, Destination: destination}
	}
	return semantics
}

// requestVersion は要求行の 3 番目の token から HTTP のバージョンを導く。
//
// http.request_version の項目は小文字の文字列で比べるため、原資料の文字列を小文字へ直した値を
// 正規化値に置く。ParseRequestLine が `HTTP/` の接頭辞とバージョンの文字列があることを確かめてから
// Protocol に置くため、値が空になる経路が無い。
func requestVersion(requestLine squid.RequestLine) core.RawAndNormalized {
	return normalizedText(requestLine.Protocol, strings.ToLower(requestLine.Protocol),
		derivationRequestVersion)
}

// requestTargetHost は %ru の authority の host から接続先を導く。
//
// 持つのは userinfo と port と IPv6 の角括弧を外した host である。この文字列を持つのは、
// 接続先 IP の条件が相手の収集元の接続先 IP の文字列と比べるためである。port は
// requestTargetPort が別に持つ。
//
// authority を書いていない要求先では導出できなかった状態を返す。
func requestTargetHost(requestLine squid.RequestLine) core.RawAndNormalized {
	if requestLine.AuthorityHost == nil {
		return undeterminedText(derivationAuthorityAbsent(requestLine.RawTarget))
	}
	return normalizedText(requestLine.RawTarget, *requestLine.AuthorityHost,
		derivationRequestTargetHost)
}

// requestTargetPort は接続先 port を導く。
//
// authority が port を書いているときはその文字列を採り、書いていないときは scheme の
// 既定の port を採る。どちらも無い要求先では導出できなかった状態を
// 返す。既定の port を持たない scheme の要求先も同じ状態になる。
func requestTargetPort(requestLine squid.RequestLine) core.RawAndNormalized {
	if requestLine.AuthorityPort != nil {
		return normalizedText(requestLine.RawTarget, *requestLine.AuthorityPort,
			derivationAuthorityPort)
	}
	if requestLine.Scheme != nil {
		if port, known := schemeDefaultPort[strings.ToLower(*requestLine.Scheme)]; known {
			return normalizedText(requestLine.RawTarget, port, derivationSchemeDefaultPort)
		}
	}
	return undeterminedText(derivationPortAbsent(requestLine.RawTarget))
}
