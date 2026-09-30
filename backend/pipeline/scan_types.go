package pipeline

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// RecordEntry は取り込んだレコード 1 件である。
//
// Locator の SourceId は走査の時点で空である。**埋めるのは識別子を発行した後の段階で
// ある** (sourceId は取り込み 1 件を指し、走査より後に発行される)。
// SourceContentSha256 と SourceFileName は走査の時点で埋まる。
// Semantics の ProcessRef も同じ順で埋まる。
type RecordEntry struct {
	// Locator はレコードの位置である。SourceId は後の段階が埋める。
	Locator core.RecordLocator
	// RawText はレコードの原文である。
	RawText string
	// ObservedAt はレコードが持つ時刻である。読めなかったときは値を持たない。
	ObservedAt *core.Timestamp
	// Semantics はレコードの意味付けの結果である。意味付けに至らなかったレコードでは
	// 値を持たない。
	Semantics *RecordSemantics
	// Terminal はレコードが記録した端末の項目である。要素は語彙の項目を semantic に
	// 持ち、IP から端末への割当の材料になる。端末の欄を持たない入力形式のレコードでは
	// 要素数 0 である。
	Terminal []core.RecordField
}

// sourceMeasurement は収集元の byte 列を端まで 1 回読んで出す測定値である。
//
// 全体を読み切れなかったときは作らない。部分的な読み取りから作った値を、収集元全体の
// 識別として渡さないためである。作るのは measureWholeSource だけである。
type sourceMeasurement struct {
	// ContentSha256 は収集元の byte 列全体の sha256 である。小文字 16 進 64 文字。
	ContentSha256 string
	// SizeBytes は収集元の byte 数である。
	SizeBytes int64
	// NewlineCount は収集元に含まれる改行の個数である。
	NewlineCount int64
	// EndsWithNewline は収集元の末尾が改行で終わるかである。
	EndsWithNewline bool
	// LineEnding は収集元の行末の byte 列である。
	LineEnding core.LineEnding
}

// failedRecord は読み取りに失敗したレコード 1 件である。
type failedRecord struct {
	// Failure は adapter が埋めた失敗である。
	Failure core.ImportFailure
	// RawText は失敗したレコードの原文である。位置を確定できない失敗では値を持たない。
	RawText *string
	// ObservedAt はレコードが持つ時刻である。読み取れなかったときは値を持たない。
	ObservedAt *core.Timestamp
	// Kept は、同じ位置のレコードを読めた欄を持つレコードとしても取り込んだかである
	// (core.ImportFailure.RecordTruncated)。
	Kept bool
}

// scannedSource は収集元 1 件を走査し終えた材料である。
//
// **公開してよいかを決めていない。** 公開の可否を決めるのは decidePublicationState で、
// 決めた結果を持つのは ImportResult と SourcePublication である。
//
// ImportResult を作れるのは newImportResult だけである。
//
// **Locator の SourceId は走査の時点で空である。** 識別子を発行した後の段階が埋める。
type scannedSource struct {
	// Plan は走査の対象を指した計画である。
	Plan SourcePlan
	// Measurement は収集元の測定値である。
	Measurement sourceMeasurement
	// Parser は走査に使ったパーサーの識別である。
	Parser ParserIdentity
	// Records は読み取れたレコードである。
	Records []RecordEntry
	// Failures は読み取りの失敗と原文である。
	Failures []failedRecord
	// Counts は区分ごとの件数である。
	Counts core.ImportCountSet
	// DiagnosisCounts は失敗原因の分類ごとの件数である。
	DiagnosisCounts []core.DiagnosisCount
	// FailureCount は打ち切りの前に数えた失敗の総数である。
	FailureCount int64
	// Scope は走査した範囲である。SourceId は後の段階が埋める。
	Scope core.RecordRange
	// ReadStopped は走査が収集元の末尾に達する前に止まったかである。
	ReadStopped bool
	// MessageUnrenderedCount は、説明を組めなかった読み取れたレコードの件数である。
	MessageUnrenderedCount int64
	// MessageUnrenderedRecords は、説明を組めなかったレコードの Records の中の位置である。
	MessageUnrenderedRecords []int
	// TerminalCandidates は、端末の候補の名前ごとに、その名前が現れた読み取れたレコードの
	// 件数を持つ。
	TerminalCandidates map[string]int64
	// FileHeader は収集元の file の見出しが記録した値である (SourceHeaderReader)。
	FileHeader []core.RecordField
	// Members は収集元を構成する file である (CompanionFileParser)。付属の file を読まない
	// 形式では nil である。
	Members []core.SourceMember
	// TerminalNamings は、読み取れたレコードが記録した端末自身の名前である
	// (ParsedRecord.TerminalNamings)。名前を記録したレコードが無い収集元では nil である。
	TerminalNamings []recordTerminalNaming
}

// recordTerminalNaming は、レコード 1 件が記録した端末自身の名前 1 つである。
type recordTerminalNaming struct {
	// record は名前を記録したレコードの、収集元の Records の中の位置である。
	record int
	naming TerminalNaming
}

// SourcePlan は取り込む収集元 1 件の指定である。
type SourcePlan struct {
	// OriginPath は収集元の取得元である。repo root からの相対 path。
	OriginPath string
	// FileName は収集元の表示名である。呼び出し側は file 名を置く。同じ file 名の収集元が
	// 2 件以上あるとき、Runner.Run が区別する文字列を括弧で足す (DistinguishFileNames)。
	FileName string
	// FormatKey は入力形式である。**内容から推測せず、呼び出し側が明示する。**
	FormatKey core.FormatKey
	// FormatSpec は欄の並びの指定である。並びを指定から受け取る入力形式だけが値を持つ。
	// **収集元の内容から並びを推測しない。**
	FormatSpec *string
	// CaseId は収集元に付けた案件である。1 回の取り込みの収集元は、すべてが値を持つか、
	// すべてが値を持たない。
	CaseId *string
	// ExpectedContentSha256 は記録した取り込みの指定が持つ内容の識別である。値があれば、
	// 読んだ byte 列の sha256 と一致しない収集元で取り込み全体を止める。
	ExpectedContentSha256 *string
	// Terminal は利用者が指定した、この収集元を記録した端末である。指定しない収集元では
	// 出ない。
	Terminal *SourceTerminal
	// CollectionPath は、収集元を取り出した収集の directory である (ExpandCollection)。
	// 収集元の file を 1 件ずつ指定した計画では空である。
	CollectionPath string
}

// SourceTerminal は利用者が収集元 1 件に指定した、その収集元を記録した端末と、時刻を読む
// UTC からのずれである。
//
// **4 項目はどれも省ける。** 1 つ以上を持つ。省いた項目は空の文字列か nil である。端末の
// 3 項目のどれも持たない指定は、ずれだけを与え、端末を与えない (HasTerminal)。検査は
// core.TerminalAssignment の Validate が行う。
type SourceTerminal struct {
	// TerminalId は端末の外部識別子である。
	TerminalId string
	// TerminalHostname は端末の表示名である。
	TerminalHostname string
	// Ip は端末が持つ IP アドレスである。
	Ip string
	// TimeOffset は、UTC からのずれを持たない収集元の時刻を読む UTC からのずれである。
	// nil は指定していないことを表す。
	//
	// **所見を記録しない。** 分析者が画面で同じ収集元の時刻の解釈を記録したときは、その
	// 所見のずれで読む。
	TimeOffset *core.UtcOffset
}

// HasTerminal は、端末の 3 項目の 1 つ以上を持つかを返す。
func (t SourceTerminal) HasTerminal() bool {
	return t.TerminalId != "" || t.TerminalHostname != "" || t.Ip != ""
}

// Validate は収集元に指定した端末の形を確かめる。
//
// **4 項目のうち 1 つ以上を持つ。** IP は IP アドレスとして読める文字列に限る。取り込みの
// 実行は同じ検査を core.TerminalAssignment の Validate でもう一度行う。起動の文字列と
// 読み込みの要求を、収集元を読む前に退けるために置く。
func (t SourceTerminal) Validate() error {
	if !t.HasTerminal() && t.TimeOffset == nil {
		return errors.New("the terminal carries none of the identifier, the name, the IP and the time offset")
	}
	if t.TimeOffset != nil {
		if err := t.TimeOffset.Validate(); err != nil {
			return fmt.Errorf("the time offset of the terminal: %w", err)
		}
	}
	for _, value := range []string{t.TerminalId, t.TerminalHostname, t.Ip} {
		if value != "" && strings.TrimSpace(value) == "" {
			return errors.New("the terminal carries an item of blank characters alone")
		}
		if strings.ContainsFunc(value, unicode.IsControl) {
			return errors.New("the terminal carries a control character")
		}
	}
	if t.Ip != "" {
		if _, err := netip.ParseAddr(t.Ip); err != nil {
			return fmt.Errorf("the terminal IP %q is not an IP address", t.Ip)
		}
	}
	return nil
}
