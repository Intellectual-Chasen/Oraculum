package core

// RecordNumberExamination は、収集元のレコードの番号の抜けを調べた結果の区分である。
type RecordNumberExamination string

// RecordNumberExamination の値。
const (
	// RecordNumberExaminationGapsFound は、番号を振った単位のどれかに抜けがある区分である。
	RecordNumberExaminationGapsFound RecordNumberExamination = "gaps_found"
	// RecordNumberExaminationNoGaps は、読めた番号がどの単位でも最小から最大まで続く区分である。
	RecordNumberExaminationNoGaps RecordNumberExamination = "no_gaps"
	// RecordNumberExaminationNotExamined は、抜けを調べなかった区分である。
	RecordNumberExaminationNotExamined RecordNumberExamination = "not_examined"
)

// RecordNumberNotExaminedReason は、番号の抜けを調べなかった理由である。
type RecordNumberNotExaminedReason string

// RecordNumberNotExaminedReason の値。
const (
	// RecordNumberNotExaminedNotDeclared は、入力形式が番号を調べる対象として番号の欄を宣言して
	// いない理由である。形式の原資料が番号を持つかは名乗らない。
	RecordNumberNotExaminedNotDeclared RecordNumberNotExaminedReason = "record_numbers_not_declared"
	// RecordNumberNotExaminedNumbersUnreadable は、番号の欄を宣言した形式でも、番号を読めた
	// レコードが 0 件である理由である。
	RecordNumberNotExaminedNumbersUnreadable RecordNumberNotExaminedReason = "record_numbers_unreadable"
)

// 読める番号は、10 進の符号の無い整数で 2^53 - 1 以下の文字列である。応答の番号の項目はどれも
// この範囲に収まり、JavaScript の数として値を丸めずに読める。2^53 以上の番号は読めない番号の
// 件数に入る。

// RecordNumberListLimit は、応答の位置と抜けの配列が持つ要素の上限である。件数の項目は上限で
// 切る前の全件を数える。
const RecordNumberListLimit = 200

// RecordNumbers は 1 つの収集元のレコードの番号の抜けである。**番号が抜けた理由を名乗らない。**
// 取り込みの失敗の記録が抜けの位置にあるときだけ、その失敗を示す。
type RecordNumbers struct {
	SourceId    string                  `json:"sourceId"`
	Examination RecordNumberExamination `json:"examination"`
	// NotExaminedReason は Examination が not_examined のときだけ出る。
	NotExaminedReason RecordNumberNotExaminedReason `json:"notExaminedReason,omitempty"`
	// UnreadableRecordCount は、番号の欄を宣言した形式のうち、番号を読めなかったレコードの件数で
	// ある。番号の欄を宣言していない形式では出ない。
	UnreadableRecordCount *int64 `json:"unreadableRecordCount,omitempty"`
	// Streams は番号を振った単位 (チャネル) ごとの結果である。チャネルの文字列の順に並ぶ。
	Streams []RecordNumberStream `json:"streams"`
	// FileHeader は file の見出しの次のレコード番号を、読めたレコードの番号と比べた結果である。
	// 見出しの項目を宣言していない入力形式では出ない。
	FileHeader *RecordNumberFileHeader `json:"fileHeader,omitempty"`
	// ListLimit は Gaps と FailureRecordRefs が持つ要素の上限である。GapCount と
	// FailureRecordCount が要素数を超える配列は、先頭の ListLimit 件に切ってある。
	ListLimit int64 `json:"listLimit"`
}

// RecordNumberStream は、番号を振った 1 つの単位 (同じ収集元の同じチャネル) の番号の範囲と抜けで
// ある。**単位に端末を含めない。** 1 つの file の中で Computer の名前が変わっても番号は続き、
// 変わり目の抜けを見つけるためである。
type RecordNumberStream struct {
	// Channel は単位の文字列である。チャネルの欄の無いレコードの単位では出ない。
	Channel *string `json:"channel,omitempty"`
	// Computers は単位のレコードが名乗った Computer ごとの件数である。Computer の文字列の順に並び、
	// 欄の無いレコードの要素が先頭に来る。2 種類以上の Computer を持つ単位の重複した番号は、
	// 複数の端末の記録が混ざっていることがありうる。理由は名乗らない。
	Computers []RecordNumberComputer `json:"computers"`
	// RecordCount は番号を読めたレコードの件数である。
	RecordCount  int64  `json:"recordCount"`
	LowestNumber uint64 `json:"lowestNumber"`
	// HighestNumber は読めた最大の番号である。
	HighestNumber uint64 `json:"highestNumber"`
	// MissingNumberCount は最小と最大の間で、どのレコードも持たない番号の個数である。
	MissingNumberCount uint64 `json:"missingNumberCount"`
	// DuplicatedRecordCount は、同じ単位で先に現れた番号をもう一度持つレコードの件数である。
	DuplicatedRecordCount int64 `json:"duplicatedRecordCount"`
	// GapCount は抜けた番号の範囲の個数である。
	GapCount int64 `json:"gapCount"`
	// Gaps は抜けた番号の範囲である。番号の昇順に並び、先頭の ListLimit 件までを持つ。
	Gaps []RecordNumberGap `json:"gaps"`
}

// RecordNumberComputer は、単位の中で 1 つの Computer を名乗ったレコードの件数である。
type RecordNumberComputer struct {
	// Computer は Computer の文字列である。欄の無いレコードの要素では出ない。
	Computer    *string `json:"computer,omitempty"`
	RecordCount int64   `json:"recordCount"`
}

// RecordNumberGap は続けて抜けた番号の範囲 1 つである。
type RecordNumberGap struct {
	// FirstMissingNumber から LastMissingNumber までの番号を、どのレコードも持たない。
	FirstMissingNumber uint64 `json:"firstMissingNumber"`
	LastMissingNumber  uint64 `json:"lastMissingNumber"`
	// PrecedingRecordRef と FollowingRecordRef は、抜けの直前と直後の番号を持つレコードである。
	PrecedingRecordRef RecordLocator `json:"precedingRecordRef"`
	FollowingRecordRef RecordLocator `json:"followingRecordRef"`
	// FailureRecordCount は、抜けに結んだ取り込みの失敗のレコードの件数である。結ぶのは、直前と
	// 直後のレコードが収集元のレコードの並びで隣り合い、失敗の位置が直前のレコードの終わりから
	// 直後のレコードの始まりまでの byte 範囲にあるときだけである。
	FailureRecordCount int64 `json:"failureRecordCount"`
	// FailureRecordRefs は結んだ失敗のレコードの位置である。先頭の ListLimit 件までを持つ。
	FailureRecordRefs []RecordLocator `json:"failureRecordRefs"`
}

// RecordNumberHeaderComparison は file の見出しの次のレコード番号と、読めた最大の番号の 1 つ
// 後ろとの比較の区分である。
type RecordNumberHeaderComparison string

// RecordNumberHeaderComparison の値。
const (
	// RecordNumberHeaderAgrees は 2 つの番号が等しい区分である。
	RecordNumberHeaderAgrees RecordNumberHeaderComparison = "agrees"
	// RecordNumberHeaderDiffers は 2 つの番号が異なる区分である。
	RecordNumberHeaderDiffers RecordNumberHeaderComparison = "differs"
	// RecordNumberHeaderNoRecordNumbers は、レコードの見出しの番号を読めたレコードが 0 件の区分である。
	RecordNumberHeaderNoRecordNumbers RecordNumberHeaderComparison = "no_record_numbers"
	// RecordNumberHeaderUnreadable は、file の見出しの次のレコード番号を読めなかった区分である。
	RecordNumberHeaderUnreadable RecordNumberHeaderComparison = "header_unreadable"
)

// RecordNumberFileHeader は file の見出しの値と、レコードの見出しの番号の最大の比較である。
type RecordNumberFileHeader struct {
	// NextRecordNumber は file の見出しが記録した次のレコード番号である。読めなかったときは出ない。
	NextRecordNumber *uint64 `json:"nextRecordNumber,omitempty"`
	// HighestRecordNumber はレコードの見出しの番号の最大である。読めたレコードが 0 件のときは出ない。
	HighestRecordNumber *uint64 `json:"highestRecordNumber,omitempty"`
	// UnreadableRecordHeaderCount は、レコードの見出しの番号を読めなかったレコードの件数である。
	UnreadableRecordHeaderCount int64                        `json:"unreadableRecordHeaderCount"`
	Comparison                  RecordNumberHeaderComparison `json:"comparison"`
	// Dirty は file の見出しの dirty の印の文字列である。見出しが印を持たないときは出ない。
	Dirty *string `json:"dirty,omitempty"`
}

// RecordComparisonKey は、2 つの収集元のレコードを突き合わせた鍵である。
type RecordComparisonKey string

// RecordComparisonKey の値。
const (
	// RecordComparisonKeyRecordNumber はチャネル、端末、EventRecordID の組である。
	RecordComparisonKeyRecordNumber RecordComparisonKey = "channel_computer_record_number"
	// RecordComparisonKeySecondProviderEvent は、時点を秒に切り捨てた値、プロバイダ、イベント ID の
	// 組である。片方の形式が番号を持たないときに使う。
	RecordComparisonKeySecondProviderEvent RecordComparisonKey = "second_provider_event_id"
)

// RecordComparisonState は突き合わせの区分である。
type RecordComparisonState string

// RecordComparisonState の値。
const (
	RecordComparisonCompared    RecordComparisonState = "compared"
	RecordComparisonNotCompared RecordComparisonState = "not_compared"
)

// RecordNotComparedReason は突き合わせなかった理由である。
type RecordNotComparedReason string

// RecordNotComparedReason の値。
const (
	// RecordNotComparedNotDeclared は、どちらかの入力形式が突き合わせの鍵の欄を宣言していない理由である。
	RecordNotComparedNotDeclared RecordNotComparedReason = "comparison_key_not_declared"
	// RecordNotComparedTimeOffsetUndetermined は、どちらかの収集元で時点の定まる時刻を持つ
	// レコードが 0 件である理由である。UTC からのずれを推測で補わない。
	RecordNotComparedTimeOffsetUndetermined RecordNotComparedReason = "time_offset_undetermined"
)

// RecordComparison は 2 つの収集元のレコードを鍵で突き合わせた結果である。**鍵が同じ
// レコードどうしを同じ事象と名乗らない。** 鍵ごとの件数だけを比べる。
type RecordComparison struct {
	ComparedSourceId string                `json:"comparedSourceId"`
	State            RecordComparisonState `json:"state"`
	// NotComparedReason は State が not_compared のときだけ出る。
	NotComparedReason RecordNotComparedReason `json:"notComparedReason,omitempty"`
	// Key は突き合わせた鍵である。State が not_compared で鍵を決められないときは出ない。
	Key RecordComparisonKey `json:"key,omitempty"`
	// OneToOneKeyCount は、両方の収集元でちょうど 1 件ずつのレコードが持つ鍵の個数である。
	OneToOneKeyCount int64 `json:"oneToOneKeyCount"`
	// OnlyInSourceRecordCount と OnlyInComparedRecordCount は、相手の収集元のどのレコードも
	// 持たない鍵を持つレコードの件数である。
	OnlyInSourceRecordCount   int64 `json:"onlyInSourceRecordCount"`
	OnlyInComparedRecordCount int64 `json:"onlyInComparedRecordCount"`
	// OnlyInSourceRecordRefs と OnlyInComparedRecordRefs はそのレコードの位置である。収集元の中の
	// 順に並び、先頭の ListLimit 件までを持つ。
	OnlyInSourceRecordRefs   []RecordLocator `json:"onlyInSourceRecordRefs"`
	OnlyInComparedRecordRefs []RecordLocator `json:"onlyInComparedRecordRefs"`
	// 片方にしか無いレコードを、相手の収録範囲の外と内に分けた件数と、範囲の内のレコードの位置である。
	//
	// **範囲は、相手の収集元の鍵を持つレコードが占める範囲である。** 番号の鍵ではチャネルごとの
	// 番号の最小から最大、時刻の鍵ではプロバイダごとの時点の秒の最初から最後であり、両端を含む。
	// 相手が同じチャネル (プロバイダ) の鍵を 1 つも持たないレコードは範囲の外である。相手が鍵を
	// 持つレコードを 1 件も持たないときは範囲を決めず、外にも内にも数えない。
	//
	// **範囲の内の差を、欠けたレコードと名乗らない。** 種別で絞って書き出した形式も同じ差を持つ。
	// 位置は収集元の中の順に並び、先頭の ListLimit 件までを持つ。
	OnlyInSourceOutsideComparedRangeRecordCount int64           `json:"onlyInSourceOutsideComparedRangeRecordCount"`
	OnlyInSourceInsideComparedRangeRecordCount  int64           `json:"onlyInSourceInsideComparedRangeRecordCount"`
	OnlyInSourceInsideComparedRangeRecordRefs   []RecordLocator `json:"onlyInSourceInsideComparedRangeRecordRefs"`
	OnlyInComparedOutsideSourceRangeRecordCount int64           `json:"onlyInComparedOutsideSourceRangeRecordCount"`
	OnlyInComparedInsideSourceRangeRecordCount  int64           `json:"onlyInComparedInsideSourceRangeRecordCount"`
	OnlyInComparedInsideSourceRangeRecordRefs   []RecordLocator `json:"onlyInComparedInsideSourceRangeRecordRefs"`
	// ListLimit は位置の配列が持つ要素の上限である。
	ListLimit int64 `json:"listLimit"`
	// UndeterminedKeyCount は、両方の収集元のレコードが持ち、どちらかで 2 件以上のレコードが
	// 持つ鍵の個数である。鍵だけではレコードを 1 対 1 に決められない。
	UndeterminedKeyCount int64 `json:"undeterminedKeyCount"`
	// UndeterminedSourceRecordCount と UndeterminedComparedRecordCount は、その鍵を持つ
	// レコードの件数である。
	UndeterminedSourceRecordCount   int64 `json:"undeterminedSourceRecordCount"`
	UndeterminedComparedRecordCount int64 `json:"undeterminedComparedRecordCount"`
	// UnequalKeyCount は、決められない鍵のうち、2 つの収集元で件数の違う鍵の個数である。
	UnequalKeyCount int64 `json:"unequalKeyCount"`
	// SourceSurplusRecordCount は、決められない鍵ごとに、この収集元の件数が相手の件数を超えた分の
	// 和である。ComparedSurplusRecordCount は相手の側の同じ和である。
	SourceSurplusRecordCount   int64 `json:"sourceSurplusRecordCount"`
	ComparedSurplusRecordCount int64 `json:"comparedSurplusRecordCount"`
	// UnequalKeys は件数の違う鍵ごとに、鍵に入れていない欄の値で分けた結果である。並びは
	// この収集元の中の最初のレコードの順で、先頭の ListLimit 個までを持つ。
	UnequalKeys []RecordComparisonUnequalKey `json:"unequalKeys"`
	// SourceUnkeyedRecordCount と ComparedUnkeyedRecordCount は、鍵の欄を読めず、突き合わせに
	// 入らなかったレコードの件数である。
	SourceUnkeyedRecordCount   int64 `json:"sourceUnkeyedRecordCount"`
	ComparedUnkeyedRecordCount int64 `json:"comparedUnkeyedRecordCount"`
}

// UnequalKeyOutcome は、件数の違う鍵 1 つを欄の値で分けた結果の区分である。
type UnequalKeyOutcome string

// UnequalKeyOutcome の値。
const (
	// UnequalKeyIdentified は、片方にだけあるレコードを欄の値で特定できた区分である。特定した
	// レコードは、同じ側の他のどのレコードとも比べた値の組が違う。
	UnequalKeyIdentified UnequalKeyOutcome = "identified"
	// UnequalKeyNoComparedValues は、両方の収集元のその鍵のレコードに、同じ文字列で比べられる
	// 項目が無い区分である。
	UnequalKeyNoComparedValues UnequalKeyOutcome = "no_compared_values"
	// UnequalKeyUndecided は、比べた値の組で片方にだけあるレコードを 1 つに決められない区分で
	// ある。同じ値の組のレコードが同じ側に 2 件以上あるときと、対にならないレコードが両側に
	// あるときである。
	UnequalKeyUndecided UnequalKeyOutcome = "undecided"
)

// RecordComparisonUnequalKey は件数の違う鍵 1 つについて、鍵に入れていない欄の値まで比べた
// 結果である。
//
// **比べる項目は、その鍵のレコードの全件が持ち、少ない側の値がすべて多い側の値にある語彙の
// 項目である。** 形式ごとに文字列の書き方が違う項目 (例: 方向の欄の番号と文言) は、値が一致
// しないため比べる項目から外れる。
type RecordComparisonUnequalKey struct {
	Outcome UnequalKeyOutcome `json:"outcome"`
	// ComparedSemantics は値を比べた語彙の項目である。並びは語彙の文字列の順である。
	ComparedSemantics []SemanticKey `json:"comparedSemantics"`
	// SourceRecordRefs と ComparedRecordRefs は、その鍵を持つレコード全件の位置である。
	// 片方にだけあるレコードを特定できないときの候補である。
	SourceRecordRefs   []RecordLocator `json:"sourceRecordRefs"`
	ComparedRecordRefs []RecordLocator `json:"comparedRecordRefs"`
	// OnlyInSourceRecordRefs と OnlyInComparedRecordRefs は、比べた値の組が相手の収集元の
	// どのレコードとも対にならないレコードの位置である。
	OnlyInSourceRecordRefs   []RecordLocator `json:"onlyInSourceRecordRefs"`
	OnlyInComparedRecordRefs []RecordLocator `json:"onlyInComparedRecordRefs"`
}
