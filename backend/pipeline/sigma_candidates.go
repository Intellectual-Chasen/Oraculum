package pipeline

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// SigmaRuleSpec は起動が渡した Sigma のルールの集合の指定である。
type SigmaRuleSpec struct {
	// Directory はルールの file を置いた directory である。
	Directory string
	// Revision は起動が渡した commit の文字列である。空は渡していないことを表す。
	Revision string
	// ExpectedContentSha256 は記録した取り込みの指定が持つルールの集合の内容の識別である。
	// 値があれば、読んだ file の識別と一致しない集合で起動を止める。
	ExpectedContentSha256 *string
}

// SigmaRuleSetInfo は評価に使ったルールの集合を特定する情報である。
type SigmaRuleSetInfo struct {
	// Directory は SigmaRuleSpec.Directory と同じである。
	Directory string `json:"directory"`
	// Revision は commit の文字列である。commit を確かめられなかった集合では出ない。
	Revision *string `json:"revision,omitempty"`
	// RevisionSource は commit をどこから決めたかである。git_head は directory を追跡する git の
	// 作業ツリーの HEAD、argument は起動が渡した文字列 (HEAD と突き合わせていない)、unverified は
	// commit を確かめられなかったことを表す。
	RevisionSource string `json:"revisionSource"`
	// RevisionDetail は git の HEAD をルールの集合の commit にできなかった理由である。commit にしたときは出ない。
	RevisionDetail string `json:"revisionDetail,omitempty"`
	// GitWorkTree は directory を含む git の作業ツリーの root の絶対 path である。作業ツリーが
	// 見つからないときは出ない。
	GitWorkTree string `json:"gitWorkTree,omitempty"`
	// ContentSha256 はルールの file の path と内容から求めた識別である。小文字 16 進 64 文字。
	ContentSha256 string `json:"contentSha256"`
	// RuleFileCount は読んだルールの file の数である。
	RuleFileCount int64 `json:"ruleFileCount"`
}

// SigmaEvaluation は Windows イベントログのレコードに Sigma のルールを当てた結果である。
//
// **一致は候補である。** ルールの条件に一致したレコードを示し、観測の関係や分析者の判断を
// 作らない。
type SigmaEvaluation struct {
	// RuleSet は評価に使ったルールの集合である。ルールの集合を渡していない起動では nil である。
	RuleSet *SigmaRuleSetInfo
	// EvaluatedRuleCount は評価したルールの数である。
	EvaluatedRuleCount int64
	// EvaluatedRecordCount は、Channel (または Provider) がルールの logsource の service に
	// 一致し、ルールを当てた Windows イベントログのレコードの数である。
	EvaluatedRecordCount int64
	// UnevaluatedRecordGroups は、Channel (または Provider) がどの service にも一致せず、
	// どのルールも一致しえないレコードを、チャネル (Channel を持たないレコードはプロバイダ) ごとに
	// 数えたものである。件数の多い順に並ぶ。
	UnevaluatedRecordGroups []SigmaUnevaluatedRecordGroup
	// SkippedPairCount は、ルールが参照する項目をレコードが名前付きの欄として持たないため
	// 当てなかったルールとレコードの組の数である。SkippedPairRecordCount はその組を 1 つ以上
	// 持つレコードの数である。
	SkippedPairCount       int64
	SkippedPairRecordCount int64
	// RecordsWithoutSemantics は、意味付けに至らず項目を持たないため、ルールを当てなかった
	// Windows イベントログのレコードの数である。
	RecordsWithoutSemantics int64
	// Rules は 1 件以上のレコードに一致したルールを path の文字列の順で持つ。
	Rules []SigmaMatchedRule
	// Matches は一致したルールとレコードの組を、ルールの path の文字列、収集元の入力順、収集元の
	// 中のレコードの順で持つ。
	Matches []SigmaRuleMatch
	// UnevaluatedRules は評価しなかったルールの file を path の文字列の順で持つ。
	UnevaluatedRules []SigmaUnevaluatedRule
}

// SigmaUnevaluatedRecordGroup は、どのルールも一致しえないレコードの分け方 1 つと件数である。
//
// Channel の欄を持つレコードは Channel に、持たずに Provider の欄を持つレコードは Provider に、
// 欄の文字列 (空の文字列を含む) を置く。どちらの欄も持たないレコードは ChannelAndProviderAbsent が
// 真である。3 つのうち 1 つだけが出る。
type SigmaUnevaluatedRecordGroup struct {
	Channel                  *string `json:"channel,omitempty"`
	Provider                 *string `json:"provider,omitempty"`
	ChannelAndProviderAbsent bool    `json:"channelAndProviderAbsent,omitempty"`
	RecordCount              int64   `json:"recordCount"`
}

// SigmaMatchedRule は 1 件以上のレコードに一致したルールである。
type SigmaMatchedRule struct {
	Path       string
	ID         string
	Title      string
	Author     string
	Level      string
	Status     string
	Condition  string
	Selections []SigmaSelection
	MatchCount int64
}

// SigmaSelection はルールの検索 1 つの名前と、file に書かれた定義である。
type SigmaSelection struct {
	Name       string
	Definition string
}

// SigmaRuleMatch はルール 1 つに一致したレコード 1 件である。
type SigmaRuleMatch struct {
	RulePath string
	Record   core.RecordLocator
	// MatchedSelections はレコードに一致した検索の名前を、ルールの file の順で持つ。
	MatchedSelections []string
	// Terminal はレコードが記録した端末の名前である。レコードが端末を記録しないときは、
	// 評価の時点で収集元に割り当てた端末の表示名か識別子を持ち、TerminalAssigned が真である。
	// 評価は起動の時点で 1 回だけ行うため、後から画面で記録した割当は反映しない。
	// どちらも無いときは空の文字列である。
	Terminal         string
	TerminalAssigned bool
	// EventTime はレコードの時刻である。読めなかったときは値を持たない。
	EventTime *core.Timestamp
}

// SigmaUnevaluatedRule は評価しなかったルールの file 1 つである。
type SigmaUnevaluatedRule struct {
	Path   string
	ID     string
	Title  string
	Reason string
	Detail string
}
