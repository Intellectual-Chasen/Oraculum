package pipeline

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RecordSemantics は 1 レコードの意味付けの結果のうち、取り込みの実行が渡す分である。
//
// 意味付けに至らなかったレコードでは ParsedRecord と RecordEntry が nil を持つ。
// 意味付けを行っていない状態と、意味付けの結果が空である状態を分けるためである。
type RecordSemantics struct {
	// ObservationKind はレコードの観測の種別と、その意味の状態である。
	// 入力形式に観測の種別の欄が無いとき Raw の要素数は 0 になる。
	ObservationKind core.ObservationKind
	// Fields は入力形式が定める key の集合であり、段階 1 の結果である。
	//
	// 応答の fields と同じものではない。**別の収集元から導く clientTerminal と、入力形式に
	// 欄そのものが無い要素を補うのは、取り込みの実行の段階 2 (FieldsBuilder) である**。
	Fields []core.RecordField
	// ProcessRef はレコードが記録したプロセスである。入力形式にプロセスの欄が無い
	// レコードでは nil である。
	//
	// SourceContentSha256 は走査の時点で埋まり、**SourceId は識別子を発行した後の段階が
	// 埋める** (Locator と同じ)。
	ProcessRef *core.ProcessRef
	// ParentProcessId は親のプロセスの識別子である。親の欄を持つレコードだけが値を持つ。
	// 親子の結び付けを行わず、文字列を渡すだけである。
	ParentProcessId *core.RawAndNormalized
	// ProcessStart は、レコードがプロセスの起動を記録したかである。
	//
	// **どのレコードが起動を記録したかを決めるのは、入力形式を読む binding である。**
	// 観測層のグラフは本項目を読み、入力形式ごとの evt と subEvt の文字列を読まない。
	// 起動を記録したレコードを持つプロセスだけが、コマンド行と起動時刻と親のプロセスの
	// 根拠を持つ。
	ProcessStart bool
	// ProcessEnd は、レコードがプロセスの終了を記録したかである。決めるのは ProcessStart と
	// 同じく binding である。
	ProcessEnd bool
	// RemoteSession は、レコードが端末へ入った遠隔のセッションを記録したかである。決めるのは
	// ProcessStart と同じく binding である。
	RemoteSession bool
	// InboundConnection は、レコードが端末への着信の接続の許可を記録したかである。決めるのは
	// ProcessStart と同じく binding である。
	InboundConnection bool
	// InboundGroupAddress は、着信の接続の許可のうち、自分の端末の側のアドレスがマルチキャストか
	// ブロードキャストのものかである。決めるのは ProcessStart と同じく binding である。
	InboundGroupAddress bool
	// OutboundConnection は、レコードを記録した端末が 1 つの相手へ始めた接続を、レコードが
	// connection.destination_address の相手として記録したかである。決めるのは ProcessStart と同じく
	// binding である。保存した接続の定義と、Proxy が中継した要求の記録は偽である。
	OutboundConnection bool
	// SystemStart は、レコードが端末の起動を記録したかである。決めるのは ProcessStart と同じく
	// binding である。
	SystemStart bool
	// AccountCreation は、レコードが target の役割のアカウントの作成を記録したかである。決めるのは
	// ProcessStart と同じく binding である。
	AccountCreation bool
	// Endpoint はレコードが記録した接続の両端である。接続を記録しないレコードでは
	// nil である。
	Endpoint *RecordEndpoint
	// AdditionalEventTimes は、ObservedAt のほかにレコードが記録した事象の時刻の項目である。
	// 要素は Fields の中の時刻の項目であり、時系列はレコード 1 件を時刻ごとの行で並べる。
	// 1 つのレコードが同じ種類の事象を複数回記録する入力形式 (実行の履歴) だけが持つ。
	AdditionalEventTimes []core.RecordField
}

// RecordEndpoint は 1 レコードが記録した接続の両端の項目を保持する。
//
// 項目を 1 件にまとめない。要素は語彙の項目を semantic に持ち、当該レコードに key が
// 出ない項目は valueState が item_absent の値を持つ。
//
// **関連付けの鍵に使う値を本型から取り出さない。** 鍵の文字列を持つのは RecordSemantics.Fields で
// ある。要求先から接続先 port を導く収集元は、鍵に使う値と応答へ出す値を同じ 1 つの項目で
// 持つ。
type RecordEndpoint struct {
	// ClientEndpoint は接続元の項目である。要素数は 1 以上である。
	ClientEndpoint []core.RecordField
	// Destination は接続先の項目である。要素数は 1 以上である。
	Destination []core.RecordField
}

// presentText は原資料の文字列だけを持つ項目を組む。
//
// error を返さない。状態は present に固定され、原資料の文字列は字句解析が読んだ文字列であるため、
// core の constructor の検査が失敗する組み合わせが無い。
func presentText(rawText string) core.RawAndNormalized {
	value, _ := core.NewRawValue(core.ValueStatePresent, rawText)
	return value
}

// normalizedText は原資料の文字列と正規化値と導き方を持つ項目を組む。
//
// error を返さない。状態は present に固定され、原資料の文字列と正規化値は字句解析が読んだ文字列で
// あり、導き方は非空の定数であるため、検査が失敗する組み合わせが無い。
func normalizedText(rawText, normalized, derivation string) core.RawAndNormalized {
	value, _ := core.NewNormalizedValue(core.ValueStatePresent, rawText, normalized, derivation)
	return value
}

// undeterminedText は導出の対象だが導出できなかった項目を組む。
//
// error を返さない。導き方は非空の定数であるため検査が失敗する組み合わせが無い。
func undeterminedText(derivation string) core.RawAndNormalized {
	value, _ := core.NewDerivationUndeterminedValue(derivation)
	return value
}

// textField は名前と語彙の項目と値から時刻以外の項目を組む。
//
// semantic を決めるのは、値を導く元の文字列を読む adapter である。
//
// error を返さない。名前は非空の定数で、semantic は語彙の項目か空の値で、値は上記
// 3 つの constructor を通っているため、検査が失敗する組み合わせが無い。
func textField(
	name string, semantic core.SemanticKey, text core.RawAndNormalized,
) core.RecordField {
	field, _ := core.NewTextField(name, semantic, text)
	return field
}

// fieldsNamed は名前で指した項目を、names の並び順で返す。
//
// 集合に無い名前は要素にならない。**呼ぶのは、その名前の項目を必ず返す adapter の結果に
// 対してだけである。** 接続の欄を定める入力形式の adapter は、当該レコードに key が
// 出ないときも item_absent の項目として返す。
func fieldsNamed(fields []core.RecordField, names ...string) []core.RecordField {
	selected := make([]core.RecordField, 0, len(names))
	for _, name := range names {
		for _, field := range fields {
			if field.Name == name {
				selected = append(selected, field)
				break
			}
		}
	}
	return selected
}

// fieldsWithSemantic は語彙の項目で指した項目を、集合の並び順で返す。
//
// **入力形式ごとの key の文字列で探さない。** 探す側が形式の知識を持つと、形式が増える
// たびに key の一覧が増える。
func fieldsWithSemantic(
	fields []core.RecordField, semantic core.SemanticKey,
) []core.RecordField {
	// 呼び出しの大半は項目の有無だけを読み、該当する項目が無い。該当したときだけ確保する。
	var matched []core.RecordField
	for _, field := range fields {
		if field.Semantic == semantic {
			matched = append(matched, field)
		}
	}
	return matched
}

// hasSemantic は語彙の項目で指した項目があるかを、配列を確保せずに返す。
func hasSemantic(fields []core.RecordField, semantic core.SemanticKey) bool {
	return slices.ContainsFunc(fields, func(field core.RecordField) bool { return field.Semantic == semantic })
}

// comparableOfSemantic は語彙の項目で指した 1 件目の項目から、文字列の一致を比べる値を
// 返す。ok が偽になるのは、その意味を持つ項目が無いときと、値を比べられないときである。
func comparableOfSemantic(fields []core.RecordField, semantic core.SemanticKey) (string, bool) {
	for _, field := range fields {
		if field.Semantic != semantic || field.Text == nil {
			continue
		}
		return field.Text.ComparableValue()
	}
	return "", false
}

// comparableOfName は名前で指した 1 件目の項目から、文字列の一致を比べる値を返す。
// ok が偽になるのは、その名前の項目が無いときと、値を比べられないときである。
//
// **呼ぶのは、探す名前を収集元の宣言から受け取る処理だけである** (ParserIdentity の
// TranscriptIdentityItems)。入力形式ごとの key の文字列を本 package が持たないため、
// 語彙の項目で探せない値はこの経路で探す。
func comparableOfName(fields []core.RecordField, name string) (string, bool) {
	for _, field := range fields {
		if field.Name != name || field.Text == nil {
			continue
		}
		return field.Text.ComparableValue()
	}
	return "", false
}

func cloneRawAndNormalized(value core.RawAndNormalized) core.RawAndNormalized {
	value.RawText = clonePointer(value.RawText)
	value.Normalized = clonePointer(value.Normalized)
	value.Derivation = clonePointer(value.Derivation)
	return value
}

func cloneTimestamp(value core.Timestamp) core.Timestamp {
	value.RawText = clonePointer(value.RawText)
	value.Normalized = clonePointer(value.Normalized)
	value.OffsetText = clonePointer(value.OffsetText)
	value.Interpretation = clonePointer(value.Interpretation)
	return value
}

func cloneTimestampPointer(value *core.Timestamp) *core.Timestamp {
	if value == nil {
		return nil
	}
	copied := cloneTimestamp(*value)
	return &copied
}

func cloneRecordField(field core.RecordField) core.RecordField {
	if field.Text != nil {
		text := cloneRawAndNormalized(*field.Text)
		field.Text = &text
	}
	field.Timestamp = cloneTimestampPointer(field.Timestamp)
	return field
}

func cloneRecordFields(fields []core.RecordField) []core.RecordField {
	if fields == nil {
		return nil
	}
	cloned := slices.Clone(fields)
	for i := range cloned {
		cloned[i] = cloneRecordField(cloned[i])
	}
	return cloned
}

func cloneEndpoint(endpoint *RecordEndpoint) *RecordEndpoint {
	if endpoint == nil {
		return nil
	}
	return &RecordEndpoint{
		ClientEndpoint: cloneRecordFields(endpoint.ClientEndpoint),
		Destination:    cloneRecordFields(endpoint.Destination),
	}
}

// cloneSemantics は意味付けの結果を深く複製する。
//
// core.RawAndNormalized と core.Timestamp が文字列を pointer で持つため、struct の代入
// だけでは複製元と同じ文字列を指し続ける。
func cloneSemantics(semantics *RecordSemantics) *RecordSemantics {
	if semantics == nil {
		return nil
	}
	copied := *semantics
	copied.ObservationKind.Raw = cloneRecordFields(semantics.ObservationKind.Raw)
	copied.Fields = cloneRecordFields(semantics.Fields)
	copied.ProcessRef = clonePointer(semantics.ProcessRef)
	if semantics.ParentProcessId != nil {
		parentProcessId := cloneRawAndNormalized(*semantics.ParentProcessId)
		copied.ParentProcessId = &parentProcessId
	}
	copied.Endpoint = cloneEndpoint(semantics.Endpoint)
	copied.AdditionalEventTimes = cloneRecordFields(semantics.AdditionalEventTimes)
	return &copied
}
