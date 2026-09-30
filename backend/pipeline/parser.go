package pipeline

import (
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserIdentity はパーサー 1 つが名乗る項目である。
// FormatKey は SourceIdentity.formatKey になり、PositionKind は RecordLocator.positionKind に
// なり、FormatSpec は SourceIdentity.formatSpec になる。
type ParserIdentity struct {
	// ParserID はパーサーを指す識別子である。parserVersion の材料になる。
	ParserID string
	// SupportedFormatVersion はパーサーが読む入力形式のバージョンである。
	SupportedFormatVersion string
	// FormatKey は入力形式である。
	FormatKey core.FormatKey
	// PositionKind はこのパーサーが作る位置の指し方である。
	PositionKind core.PositionKind
	// FormatSpec は欄の並びを定めた指定である。並びの指定を取らない入力形式では空である。
	FormatSpec string
	// ItemSemantics はこの収集元のレコードが持ちうる語彙の項目である。
	//
	// **入力形式ごとの定数にしない。** 欄の並びを指定で受け取る形式では、持つ欄が収集元
	// ごとに変わる。**レコードの項目の集合からも導かない。** 欄を持たない形式のレコードに
	// item_absent の要素を補う経路があり、要素の有無は欄の有無を表さない
	// (FieldsBuilder の supplementedFields)。取り込みの実行が補う導出値を入れない。
	ItemSemantics []core.SemanticKey
	// TimePrecision はこの収集元のレコードの時刻の精度である。関連付けが秒未満を比べられるかの
	// 判定に用いる。
	TimePrecision core.Precision
	// StartTimeNoteText は、このパーサーが記録した起動時刻の欄に添える注記である。
	// 起動を記録しない入力形式では空である。分析者が画面で読む値であるため日本語で書く。
	StartTimeNoteText string
	// AccountNamesLocalToTerminal は、この入力形式の account.name が、収集元を記録した端末の
	// アカウントを指すかである。真の形式では、ログイン名を端末の範囲で識別する
	// (core.RecordScope の AccountNamesLocal)。
	//
	// **宣言するのは入力形式を読む adapter の binding である。** 別の端末のアカウントを
	// 指す形式 (接続先の資格情報の利用者名) では偽にする。
	AccountNamesLocalToTerminal bool
	// RecordedByOneTerminal は、この入力形式の 1 つの file を 1 台の端末が書くかである。
	// 真の形式では、端末を指定していない収集元の端末の範囲の対象を、名前不明の端末に置く。
	//
	// **複数台の端末の記録を 1 つの file に集める形式 (EDR の集約ログ) では偽にする。**
	// 端末の欄を欠くレコードを 1 台の端末に置くと、別の端末の対象が 1 つのノードになる。
	RecordedByOneTerminal bool
	// RecordingTerminalPerHostname は、この入力形式のレコードが記録した端末のホスト名を
	// 運び、1 つの file が複数台の端末の記録を持ちうるかである。真の形式では、端末を
	// 指定していない収集元のレコードを、収集元の中のホスト名ごとに別の名前不明の端末に置く。
	// ホスト名を持たないレコードはどの端末にも置かない。
	//
	// **収集元をまたいで同じホスト名の端末を 1 つにまとめない。** ホスト名は端末の外部識別子
	// ではない。1 台の端末の名前が変わっただけの file は、分析者が収集元に付ける端末の割当で
	// 1 台に戻せる。RecordedByOneTerminal と両方を真にした宣言は Validate が拒む。
	RecordingTerminalPerHostname bool
	// ConnectionRequestKinds は、外向きの通信の要求を記録した観測の種別である。
	//
	// **宣言するのは入力形式を読む adapter であり、binding が渡す。** 関連付けはこの宣言で
	// 候補として数えるレコードの母集合を決める。観測の種別の欄を持たない入力形式は
	// 要素数 0 の集合を宣言し、その収集元のレコードは種別で絞られない。
	ConnectionRequestKinds []core.ObservationKindSelector
	// ConnectionOpenKinds と ConnectionCloseKinds は、接続を開いた記録と閉じた記録の観測の種別で
	// ある。同じ端末の同じ接続の記録を、開いた記録と閉じた記録の順で結ぶのに用いる。開閉を
	// 観測の種別で分けない入力形式は宣言しない (nil)。
	ConnectionOpenKinds, ConnectionCloseKinds []core.ObservationKindSelector
	// ConnectionMatchConditions は、外向きの通信を記録したレコードどうしの関連付けで、この
	// 収集元が引き受ける条件の宣言である。
	//
	// **どの条件を段階が持つかを core と pipeline が決めない。** 条件の一覧と、両側が比べる
	// 語彙の項目は入力形式の組ごとに変わるため、入力形式を読む adapter の binding が宣言し、
	// 関連付けを実行する側が起点と候補の宣言を突き合わせて段階の条件にする。
	//
	// 並びが段階の conditions の並びになる。
	ConnectionMatchConditions []ConnectionMatchCondition
	// TranscriptIdentityItems は、同じ 1 つの事象を 2 回転記したレコードを見分ける欄の
	// 名前である。
	//
	// **宣言するのは入力形式を読む adapter であり、binding が渡す。** 端末と本項目が挙げた
	// 欄の値がすべて同じレコードは、収集元が同じ 1 つの事象から作った転記であり、独立した
	// 観測ではない。グラフは転記のうち 1 件だけを関係の根拠に数える。
	//
	// 要素数 0 を宣言した収集元のレコードは、毎回別の観測として数える。挙げた欄のいずれかを
	// 持たないレコードも同じ扱いになる。
	//
	// **欄の名前の定義元は adapter である。** 本 package は宣言を読んで値を取り出すだけで、
	// どの文字列がどの形式のものかを知らない。
	TranscriptIdentityItems []string
	// ProxyRequestLog は、この入力形式が Proxy を経由した要求を記録するログであるかである。
	// 真の収集元は、他の収集元が記録した Proxy を経由しない接続と並べる (ImportResult.ProxyBypassOf)。
	ProxyRequestLog bool
	// RequestStatusItems は、Proxy が記録した要求処理の結果の欄の名前である。関連付けの段階が
	// 起点の行の結果を出す (graphRecord.proxyStatus)。欄を持たない形式は宣言しない (nil)。
	RequestStatusItems []string
	// ContentReplacementKinds は、ファイルまたはレジストリの値の内容を置き換える記録 (作成し直し、
	// 切り詰め、削除、値の設定) を選ぶ観測の種別と欄の条件である。グラフは、この記録の
	// タイムスタンプで対象の内容のバージョンを区切る (Graph.contentIncludesWrite)。
	//
	// **宣言するのは入力形式を読む adapter であり、binding が渡す。** 置き換えを記録で
	// 示さない入力形式は宣言しない (nil)。その収集元のレコードは内容のバージョンを区切らない。
	ContentReplacementKinds []core.ContentReplacementSelector
	// FlowOperationKinds は、観測の種別ごとの操作の分類であり、影響のエッジの向きを決める
	// (graphRecord.flowOperation)。**宣言するのは入力形式を読む adapter であり、binding が渡す。**
	// 宣言の無い観測の種別のレコードは、読み書きの量の欄を持たなければ、向きを決められない記録になる。
	FlowOperationKinds []core.FlowOperationSelector
	// CountsUnrenderedMessages は、この入力形式のレコードが、書き出した端末が説明を組めた
	// かを持つかである。真の形式の収集元は、組めなかったレコードの件数を
	// SourceIdentity.MessageUnrenderedCount に出す (ParsedRecord.MessageUnrendered)。
	CountsUnrenderedMessages bool
	// RawTextConverted は、このパーサーが返す原文が、収集元の byte 列から組み立てた文字列で
	// あるかである。SourceIdentity.rawTextConverted になる。
	RawTextConverted bool
	// RecordNumbering は、レコードの番号の抜けと形式の違う取り込みの突き合わせに使う欄の名前で
	// ある。zero value の形式は、番号を調べる対象と突き合わせの対象として欄を宣言していない。
	// 欄の名前の定義元は adapter である。
	RecordNumbering RecordNumberingNames
}

// SourceHeaderReader は、収集元の file の見出しを読むパーサーが持つ。
//
// **走査が末尾に達した後に呼ぶ。** 返した値は SourceIdentity.fileHeader になる。
type SourceHeaderReader interface {
	// SourceHeader は見出しが記録した値を返す。見出しを読めなかった収集元では要素数 0 である。
	SourceHeader() []core.RecordField
}

// CompanionFileParser は、主 file と同じ directory の付属の file を一緒に読むパーサーが持つ。
//
// 取り込みの実行は、主 file の path に CompanionSuffixes の接尾辞を足した file を並びの順に
// 開き、在る file を主 file の後ろに連結した byte 列を収集元にする。無い file は飛ばす。
// **SetMembers は Reset より前に呼ぶ。** 先頭が主 file である。
type CompanionFileParser interface {
	// CompanionSuffixes は付属の file を指す接尾辞である。
	CompanionSuffixes() []string
	// SetMembers は連結した byte 列を構成する file を与える。
	SetMembers(members []core.SourceMember)
}

// ConnectionMatchCondition は、収集元 1 つが関連付けの条件 1 件へ差し出す宣言である。
type ConnectionMatchCondition struct {
	// ConditionKey は条件の種別である。
	ConditionKey core.ConditionKey
	// Semantics は、この収集元がこの条件の値を持つ語彙の項目である。要素数 1 以上で、
	// 先頭が起点の側になったときに比べる項目である。
	//
	// 同じ意味の値を 2 つ以上の語彙の項目で持つ収集元は、比べる順に並べて挙げる。
	Semantics []core.SemanticKey
	// NarrowsCandidates は、この収集元を起点にした関連付けがこの条件で候補を絞るかである。
	NarrowsCandidates bool
}

// Validate は宣言 1 件の項目の整合を確かめる。
//
// **宣言の不備を関連付けの分類に化けさせない。** 検査を通らない宣言から組んだ段階の条件は
// core の検査が受け取らず、起点のレコードについての事実を表す分類
// (origin_item_unreadable) で返る。分類は原資料について言えることだけを表す値である
// (core.RelationDerivationOutcome の godoc)。
//
// **絞りに用いる条件は語彙の項目を 1 つだけ挙げる。** 段階の条件が持つ相手の値は 1 件で
// あり、2 つ以上を挙げた宣言は core の検査が受け取らない
// (backend/core の MatchRequest.validateConditions)。
func (c ConnectionMatchCondition) Validate() error {
	if !c.ConditionKey.IsKnown() {
		return fmt.Errorf("the connection match condition names the unknown condition key %q",
			string(c.ConditionKey))
	}
	if len(c.Semantics) == 0 {
		return fmt.Errorf("the connection match condition %q names no semantic",
			string(c.ConditionKey))
	}
	seen := make(map[core.SemanticKey]struct{}, len(c.Semantics))
	for _, semantic := range c.Semantics {
		if semantic == "" {
			return fmt.Errorf("the connection match condition %q names an empty semantic",
				string(c.ConditionKey))
		}
		if _, duplicate := seen[semantic]; duplicate {
			return fmt.Errorf("the connection match condition %q repeats the semantic %q",
				string(c.ConditionKey), string(semantic))
		}
		seen[semantic] = struct{}{}
	}
	if c.NarrowsCandidates && len(c.Semantics) > 1 {
		return fmt.Errorf("the connection match condition %q narrows candidates while naming "+
			"more than one semantic", string(c.ConditionKey))
	}
	return nil
}

// Validate はパーサーが名乗る宣言の整合を確かめる。
//
// **レコードを 1 件も解析する前に呼ぶ** (scanSource)。宣言の不備は adapter の実装の欠陥で
// あり、取り込みを進めずに止める。
//
// 確かめるのは、本 package が組み立てに使う宣言である。FormatKey と PositionKind の整合は
// 呼び出し元が収集元の計画と突き合わせる。
func (i ParserIdentity) Validate() error {
	if i.RecordedByOneTerminal && i.RecordingTerminalPerHostname {
		return fmt.Errorf("validating the parser %q: a format recorded by one terminal cannot "+
			"separate the terminals of a file by hostname", i.ParserID)
	}
	// **観測の種別の宣言を、重複除去より前に確かめる。** 欄の名前を 2 回挙げた宣言が
	// 和集合へ入ると、異なる欄の名前の個数が食い違い、正しい宣言と別の組として扱われる
	// (sameSelector)。
	for _, selector := range i.ConnectionRequestKinds {
		if err := selector.Validate(); err != nil {
			return fmt.Errorf("validating the parser %q: %w", i.ParserID, err)
		}
	}
	seen := make(map[core.ConditionKey]struct{}, len(i.ConnectionMatchConditions))
	for _, declaration := range i.ConnectionMatchConditions {
		if err := declaration.Validate(); err != nil {
			return fmt.Errorf("validating the parser %q: %w", i.ParserID, err)
		}
		if _, duplicate := seen[declaration.ConditionKey]; duplicate {
			return fmt.Errorf("validating the parser %q: the connection match conditions repeat "+
				"the condition key %q", i.ParserID, string(declaration.ConditionKey))
		}
		seen[declaration.ConditionKey] = struct{}{}
	}
	return nil
}

// ParserFactory は入力形式 1 つ分のパーサーを作る。
//
// formatSpec は SourcePlan.FormatSpec の値である。並びの指定を取らない入力形式は、値を
// 受け取ったときに error を返す。指定を要する入力形式は、値が無いときと指定を読めない
// ときに error を返す。
//
// **収集元を開く前に呼ぶ。** 指定を読めない起動で収集元の byte を 1 つも読まない。
type ParserFactory func(formatSpec *string) (SourceParser, error)

// ParsedRecord は 1 レコードを読んだ結果のうち、取り込みの実行が使う分である。
type ParsedRecord struct {
	// RawText はレコードの原文である。原資料の byte 列をそのまま持つ。
	RawText string
	// LineEnding はこのレコードの行末の byte 列である。
	LineEnding string
	// LineNumber は収集元の中の行番号である。1 起点。
	LineNumber int64
	// ByteOffset は収集元の先頭からのレコードの開始位置である。0 起点。
	ByteOffset int64
	// ByteLength はレコードが占める byte 数である。
	// 1 レコードが複数行に分かれる入力形式が値を持つ。
	ByteLength *int64
	// LineCount はレコードが占める行数である。
	// 1 レコードが複数行に分かれる入力形式が値を持つ。
	LineCount *int64
	// SequenceNumber は収集元の中の通番である。
	// 入力形式が通番を持ち、かつ値を読めたときだけ値を持つ。
	SequenceNumber *int64
	// ObservedAt はレコードが持つ時刻である。読めなかったときは値を持たない。
	ObservedAt *core.Timestamp
	// Semantics はレコードの意味付けの結果である。意味付けに至らなかったレコードでは
	// 値を持たない。ProcessRef の SourceContentSha256 と SourceId は走査より後の段階が
	// 埋める。
	Semantics *RecordSemantics
	// Terminal はレコードが記録した端末の項目である。要素は語彙の項目を semantic に
	// 持つ。端末の欄を持たない入力形式のレコードでは要素数 0 である。
	//
	// **Semantics と別の項目である。** 入力形式が種別ごとの意味付けを持つとき、意味付けに
	// 至らないレコードも端末の欄を持つ。
	Terminal []core.RecordField
	// MessageUnrendered は、書き出した端末がイベントの定義を持たず、レコードの説明を組めな
	// かったかである。ParserIdentity.CountsUnrenderedMessages が真の形式だけが値を与える。
	MessageUnrendered bool
	// TerminalCandidates は、レコードに現れた、記録した端末の候補の名前である。端末に
	// 決めない。収集元の SourceIdentity.TerminalCandidates に件数とともに集める。
	TerminalCandidates []string
	// TerminalNamings は、レコードが記録した、そのレコードを書いた端末自身の名前である
	// (registry のコンピューター名、改名のイベント)。収集の directory の端末の名前の集合と
	// 名前の履歴の材料になる (collectionTerminalsOf)。
	TerminalNamings []TerminalNaming
}

// TerminalNaming は、レコードが記録した端末自身の名前 1 つである。
type TerminalNaming struct {
	// Name は端末の名前である。改名を記録したレコードでは改名の後の名前である。
	Name string
	// Previous は改名の前の名前である。改名を記録していないレコードでは空である。
	//
	// **空の Previous を持つ名前だけが、収集の端末の名前の起点になる。** 改名は起点の名前に
	// つながるときだけ、名前の集合へ足す。
	Previous string
}

// SourceParser は入力形式 1 つ分の読み取りを表す port である。
//
// 実装は binding_ で始まる file が持ち、その file だけが backend/adapters/ を import する。
// port を core に置かないのは、io.Reader を引数に取り、core が I/O を持てないためである。
type SourceParser interface {
	// Identity はこのパーサーが名乗る項目を返す。
	Identity() ParserIdentity
	// Reset は読み取りの対象を差し替え、走査を先頭に戻す。
	Reset(input io.Reader)
	// Next は次の 1 レコードを返す。
	//
	// 返り値の組み合わせは 4 通りである。
	//   - (レコード, nil, nil)        正常に読めた
	//   - (レコード, 失敗, nil)        字句解析か意味付けに失敗した。走査は続く。
	//                                 レコードは RawText と位置を持つ
	//   - (零値, nil, io.EOF)          収集元の末尾に達した
	//   - (零値か断片, 失敗, io.EOF 以外) 読み込みそのものに失敗した。走査は続かない。
	//                                 byte を 1 つも読めなかった場合はレコードが零値になり、
	//                                 位置は失敗だけに載る
	//
	// **字句解析に失敗したレコードも RawText を保持する。** 原資料の原文は診断の根拠で
	// あり、失敗したレコードほど原文が要る。原資料をパーサーに合わせて書き換えない。
	//
	// **失敗が返っても error が nil である組み合わせがあるため、呼び出し側は error だけを
	// 見て取り込みの完全さを判定しない。**
	// **読み込みの失敗では、失敗と error が同時に返る。** error が nil でないことを理由に
	// 失敗を捨てない。
	Next() (ParsedRecord, *core.ImportFailure, error)
}
