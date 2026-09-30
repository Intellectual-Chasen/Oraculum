package markii

import (
	"strconv"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// レコードの種別に依らず出る key の文字列。原資料の key をそのまま使う。
const (
	keySequenceNumber = "sn"
	keyEvent          = "evt"
	keySubEvent       = "subEvt"
	keyProcessGuid    = "psGUID"
	keyTerminalId     = "tmid"
	keyComputerName   = "com"
	keySecurityId     = "csid"
	// keyProcessPath は実行 file の path の key である。起動のレコードが収集元に無いプロセスでは、
	// 通信のレコードの psPath だけがプロセスの path の材料になる。
	keyProcessPath = "psPath"
	// keyTerminalAddresses は端末が持つ IP の key である。1 つの値が IPv4 と IPv6 を
	// カンマで連結した文字列を持つ。
	keyTerminalAddresses = "ip"
)

// terminalAddressSeparator は ip の値が IP を並べる区切りである。
const terminalAddressSeparator = ","

// headerTimeFieldName はヘッダーの時刻を持つ項目の名前である。
const headerTimeFieldName = "headerTime"

// markiiTimestampLayout はタイムゾーンを持たない markii 形式の時刻の書式である。
const markiiTimestampLayout = "01/02/2006 15:04:05.000"

// derivationRemovedQuotes は引用符を外して正規化値を得たことを表す文字列である。
const derivationRemovedQuotes = "引用符を外した文字列"

// derivationTerminalAddressElement は ip の値を区切りで分けて 1 つのアドレスを得たことを
// 表す文字列である。
const derivationTerminalAddressElement = "ip の値を区切りで分けた 1 つのアドレス"

// requireSingleFields は keys の各 key がちょうど 1 回ずつ出て、値が空でないことを
// 確かめる。
//
// 2 回出た key を先勝ちで採ると、どちらの文字列を根拠にしたかが応答から消える。値が空の
// key を受け付けないのは、core.ProcessRef.Validate が processGuid と terminalId に値を求める
// ためである。どのパーサーが受け付けなかったかは core.ImportFailure の Interpretation が持つ。
func requireSingleFields(record Record, keys []string) *fieldProblem {
	for _, key := range keys {
		expected := "the key " + key + " once, with a value, in a markii record"
		switch count := record.FieldCount(key); {
		case count == 0:
			return &fieldProblem{expected: expected, observed: "no field named " + key}
		case count > 1:
			return &fieldProblem{
				expected: expected,
				observed: strconv.Itoa(count) + " fields named " + key,
			}
		}
		field, _ := record.Field(key)
		if field.Value() == "" {
			return &fieldProblem{expected: expected, observed: "an empty value for " + key}
		}
	}
	return nil
}

// SequenceNumber は一意な sn の非負の 10 進整数値を返す。
// 欄の不在、重複、解釈できない値では ok が偽になる。
func SequenceNumber(record Record) (int64, bool) {
	if record.FieldCount(keySequenceNumber) != 1 {
		return 0, false
	}
	value, problem := sequenceNumberOf(record)
	return value, problem == nil
}

// sequenceNumberOf は sn を解釈し、失敗をレコード種別に依存しない診断で返す。
func sequenceNumberOf(record Record) (int64, *fieldProblem) {
	field, found := record.Field(keySequenceNumber)
	if !found {
		return 0, &fieldProblem{
			expected: "the key " + keySequenceNumber + " in a markii record",
			observed: "no field named " + keySequenceNumber,
		}
	}
	value, err := strconv.ParseInt(field.Value(), 10, 64)
	if err != nil || value < 0 {
		return 0, &fieldProblem{
			expected: "a non-negative decimal integer as the value of " + keySequenceNumber,
			observed: "a value that cannot be interpreted as a non-negative decimal integer",
		}
	}
	return value, nil
}

// textValueOf は 1 つの key の値を core.RawAndNormalized へ直す。
//
// **key が出ないレコードでは欄の不在を返す。** 原資料に実在する空文字列を欄の不在の
// 代用にしない。
//
// 同じ key が 2 回以上出るレコードでは 1 つ目を返す。原文の並び順で 1 件ずつ扱うのは
// valueOfField である。
func textValueOf(record Record, key string) core.RawAndNormalized {
	field, found := record.Field(key)
	if !found {
		return core.NewAbsentItemValue()
	}
	return valueOfField(field)
}

// valueOfField は 1 件の field の値を core.RawAndNormalized へ直す。
//
// 引用符付きの value は、引用符を外した文字列を正規化値として持つ。引用符無しの value は
// 正規化値を持たない。
//
// **名前で探し直さない。** 探し直すと、同じ key が 2 回出るレコードで 1 つ目の値を
// 2 回持ち、2 つ目の値が応答から消える。
//
// 状態は present か absent、文字列は非空、導き方は非空の定数であり、core の constructor の
// 検査が失敗しないため error を返さない。core の検査の規則が変わったときは、成功した観測の
// すべての field に Validate() を掛けているレコードの種別ごとの fuzz が拾う。
func valueOfField(field Field) core.RawAndNormalized {
	if !field.Quoted() {
		value, _ := core.NewRawValue(core.ValueStatePresent, field.RawValue())
		return value
	}
	// 既知の制限: 引用符付きの - を、欄の名前に依らず値の不在として扱う,
	// 引用符付きの - は srcCom と wsName と srcIP のような接続元を持つ欄が値の不在を表す文字列であり、
	// 引用符の外に - を置く形は無い,
	// 引用符付きの - を値として持つ欄を収集元で確認したとき、欄の名前で扱いを分ける
	if field.Value() == absentFieldText {
		value, _ := core.NewRawValue(core.ValueStateAbsent, field.RawValue())
		return value
	}
	value, _ := core.NewNormalizedValue(
		core.ValueStatePresent, field.RawValue(), field.Value(), derivationRemovedQuotes)
	return value
}

// absentFieldText は markii 形式が引用符の中に置く、値の不在を表す文字列である。
const absentFieldText = "-"

// timestampSemanticMeaning は時刻の項目が持つ意味と、その値を刻んだ時計である。
type timestampSemanticMeaning struct {
	meaning core.Meaning
	clock   core.Clock
}

// timestampSemanticMeanings は Timestamp として持つ語彙の項目の対応である。
//
// **操作の時刻とファイルの property 時刻を同じ時計にしない。** sTime は Recorder が操作を
// 観測した時点の値で、収集した端末の system clock が刻む。crTime と acTime と moTime は
// ファイル自身が持つ property であり、そのファイルを書いた別の計算機の時計が刻んだ値で
// ありうる。
var timestampSemanticMeanings = map[core.SemanticKey]timestampSemanticMeaning{
	core.SemanticKeyProcessStartTime:          {core.MeaningOperationStart, core.ClockTerminalLocal},
	core.SemanticKeyEventOperationStartTime:   {core.MeaningOperationStart, core.ClockTerminalLocal},
	core.SemanticKeyEventSessionStartTime:     {core.MeaningOperationStart, core.ClockTerminalLocal},
	core.SemanticKeyFileCreatedTime:           {core.MeaningProperty, core.ClockFileProperty},
	core.SemanticKeyFileAccessedTime:          {core.MeaningProperty, core.ClockFileProperty},
	core.SemanticKeyFileModifiedTime:          {core.MeaningProperty, core.ClockFileProperty},
	core.SemanticKeyProcessBinaryCreatedTime:  {core.MeaningProperty, core.ClockFileProperty},
	core.SemanticKeyProcessBinaryAccessedTime: {core.MeaningProperty, core.ClockFileProperty},
	core.SemanticKeyProcessBinaryModifiedTime: {core.MeaningProperty, core.ClockFileProperty},
}

// timestampFieldOf はタイムゾーンを持たない markii 形式の時刻を Timestamp の項目へ直す。
// ok が偽になるのは semantic が時刻項目でないか、値が定められた書式でないときである。
func timestampFieldOf(field Field, semantic core.SemanticKey) (core.RecordField, bool) {
	timestampMeaning, isTimestamp := timestampSemanticMeanings[semantic]
	if !isTimestamp {
		return core.RecordField{}, false
	}
	value := field.Value()
	instant, err := time.Parse(markiiTimestampLayout, value)
	if err != nil || instant.Format(markiiTimestampLayout) != value {
		return core.RecordField{}, false
	}
	normalized := instant.Format("2006-01-02T15:04:05.000")
	rawText := field.RawValue()
	timestamp, err := core.NewTimestamp(core.Timestamp{
		RawText:        &rawText,
		Normalized:     &normalized,
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionMillisecond,
		OffsetState:    core.OffsetStateUndetermined,
		Clock:          timestampMeaning.clock,
		Meaning:        timestampMeaning.meaning,
		ValueState:     core.ValueStatePresent,
	})
	if err != nil {
		return core.RecordField{}, false
	}
	result, err := core.NewTimestampField(field.Key(), semantic, timestamp)
	return result, err == nil
}

// observationKindRaw は evt と subEvt の 2 件を、この順で返す。原資料の並び順は使わない。
//
// 意味の状態はレコードの種別ごとの根拠に依るため、core.ObservationKind の Status は呼ぶ側が
// 渡す。呼ぶのは 2 つの key が値を持つレコードだけであり、検査が失敗しないため error を返さない。
func observationKindRaw(record Record) []core.RecordField {
	raw := make([]core.RecordField, 0, 2)
	event := eventOf(record)
	for _, key := range []string{keyEvent, keySubEvent} {
		field, _ := core.NewTextField(key, semanticOf(key, event), textValueOf(record, key))
		raw = append(raw, field)
	}
	return raw
}

// TerminalFields は 1 レコードが記録した端末の項目を、語彙の意味を付けて返す。
//
// 並びは terminal.id の 1 件、terminal.hostname の 1 件、terminal.ip_address の 1 件以上
// である。**tmid が端末の外部識別子、com が分析者の読む表示名である。**
//
// 応答の fields に入らない。IP から端末への割当の材料であり、割当をどの収集元のどの期間に
// 結び付けるかを決めるのは取り込みの実行である。ip を分けた要素は、原資料の文字列に ip の値
// 全体を持ち、正規化値に 1 つのアドレスを持つ。
//
// ok が偽になるのは、tmid と com と ip の 1 つ以上が出ないレコードと、tmid または com の
// 値が空であるレコードと、ip が空でない要素を 1 つも持たないレコードである。
func TerminalFields(record Record) ([]core.RecordField, bool) {
	terminalId, hasTerminalId := record.Field(keyTerminalId)
	hostname, hasHostname := record.Field(keyComputerName)
	addresses, hasAddresses := record.Field(keyTerminalAddresses)
	if !hasTerminalId || !hasHostname || !hasAddresses {
		return nil, false
	}
	if terminalId.Value() == "" || hostname.Value() == "" {
		return nil, false
	}
	elements := splitTerminalAddresses(addresses)
	if len(elements) == 0 {
		return nil, false
	}
	idField, _ := core.NewTextField(
		keyTerminalId, core.SemanticKeyTerminalId, valueOfField(terminalId))
	hostnameField, _ := core.NewTextField(
		keyComputerName, core.SemanticKeyTerminalHostname, valueOfField(hostname))
	return append([]core.RecordField{idField, hostnameField}, elements...), true
}

// splitTerminalAddresses は ip の値を区切りで分け、要素 1 つにつき 1 項目を返す。
// 空白だけの要素を除く。
//
// 既知の制限: 区切りでの分割と前後の空白の除去だけを行い、角括弧の除去と大小文字を
// そろえる処理を行わない,
// terminal.ip_address の項目は IPv6 を角括弧無しの小文字 16 進の文字列で比べ、ip の値は
// この形で IPv6 を書く。角括弧または大文字の IPv6 を持つ入力では、Squid の接続元 IP の
// 小文字表記と文字列の一致で外れ、clientTerminal が reason=該当が 0 件 になり、成立するはずの
// 関係が通知なしに失われる,
// 角括弧または大文字の IPv6 を持つ入力形式を取り込むとき。**片側だけを正規化しない。**
// 突き合わせる相手 (Squid の clientIp) がどの形を持つかを確かめてから両側を揃える
func splitTerminalAddresses(addresses Field) []core.RecordField {
	elements := strings.Split(addresses.Value(), terminalAddressSeparator)
	fields := make([]core.RecordField, 0, len(elements))
	for _, element := range elements {
		trimmed := strings.TrimSpace(element)
		if trimmed == "" {
			continue
		}
		value, _ := core.NewNormalizedValue(core.ValueStatePresent,
			addresses.RawValue(), trimmed, derivationTerminalAddressElement)
		field, _ := core.NewTextField(
			keyTerminalAddresses, core.SemanticKeyTerminalIpAddress, value)
		fields = append(fields, field)
	}
	return fields
}

// recordFieldsOf はレコードのすべての key を core.RecordField の集合へ直す。
//
// ヘッダーの時刻は headerTimeFieldName の名前で 1 件目に置く。残りは原資料の key の
// 文字列を名前にし、原文の並び順を保つ。
//
// 共通の意味を持つ key は語彙の項目を semantic に持つ (semantic_mapping.go)。
// headerTime は HeaderTimestamp が core.NewTimestamp を通した値、key は非空の文字列であり、
// 検査が失敗しないため error を返さない。
func recordFieldsOf(record Record, headerTime core.Timestamp) []core.RecordField {
	headerField, _ := core.NewTimestampField(
		headerTimeFieldName, core.SemanticKeyEventTime, headerTime)
	recordFields := record.Fields()
	event := eventOf(record)
	fields := make([]core.RecordField, 0, len(recordFields)+1)
	fields = append(fields, headerField)
	for _, field := range recordFields {
		semantic := recordSemanticOf(record, field.Key(), event)
		if timestampField, ok := timestampFieldOf(field, semantic); ok {
			fields = append(fields, timestampField)
			continue
		}
		// 時刻の値が書式に合わないときは semantic を付けず、原資料の文字列を保持する。
		if _, isTimestamp := timestampSemanticMeanings[semantic]; isTimestamp {
			semantic = ""
		}
		value := valueOfField(field)
		if semantic == core.SemanticKeyEventTargetLogonId || semantic == core.SemanticKeyEventSubjectLogonId ||
			semantic == core.SemanticKeyEventLogoffLogonId {
			value = logonIdValueOf(field)
		}
		textField, _ := core.NewTextField(field.Key(), semantic, value)
		fields = append(fields, textField)
	}
	return fields
}
