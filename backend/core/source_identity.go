package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// LineEnding は行末の byte 列を持つ。
type LineEnding string

// LineEnding の値。
const (
	// LineEndingCrlf は行末が CR と LF の 2 byte である状態である。
	LineEndingCrlf LineEnding = "crlf"
	// LineEndingLf は行末が LF の 1 byte である状態である。
	LineEndingLf LineEnding = "lf"
	// LineEndingMixed は 1 つの収集元に 2 つの行末が混ざっている状態である。
	LineEndingMixed LineEnding = "mixed"
	// LineEndingUndetermined は行末の byte 列を確定できない状態である。
	LineEndingUndetermined LineEnding = "undetermined"
)

// IsKnown は LineEnding が定義の中の値であるかを返す。
func (l LineEnding) IsKnown() bool {
	switch l {
	case LineEndingCrlf, LineEndingLf, LineEndingMixed, LineEndingUndetermined:
		return true
	default:
		return false
	}
}

// FormatKey は入力形式を指す非空の識別子を持つ。
//
// **値を宣言するのは入力形式を読む adapter である** (InputFormat)。本 package は
// 識別子の型だけを持ち、どの形式が読めるかを知らない。
//
// **収集元がどの形式であるかを内容から推測しない。** 取り込みを求める側が明示する。
type FormatKey string

// SourceIdentity は収集元 1 件を指す。収集元は file 1 つか、主 file と付属の file の組 (Members) である。
// SourceId と SizeBytes の値の作り方は本 package が決めない。
type SourceIdentity struct {
	// SourceId は取り込み 1 件の実行を指す内部識別子である。原資料の値から作らない。
	SourceId string `json:"sourceId"`
	// ContentSha256 は収集元の内容の識別である。小文字 16 進 64 文字。
	ContentSha256 string `json:"contentSha256"`
	// OriginPath は収集元の取得元である。利用者が配置した位置を指すリポジトリ相対 path。
	OriginPath string `json:"originPath"`
	// FileName は表示に使う。単独で収集元を識別しない。同じ取り込みに同じ file 名の収集元が
	// 2 件以上あるときは、file 名に区別する文字列を括弧で足した値である。
	FileName string `json:"fileName"`
	// SizeBytes は収集元の byte 数である。
	SizeBytes int64 `json:"sizeBytes"`
	// RecordCount は収集元 1 件のレコード件数である。数える対象は収集元の全レコードで、
	// 要求の絞り込みの条件と limit を適用しない。出ない場合はレコード件数を確定できない。
	//
	// **複数の収集元の合計の件数を置かない。** SourceIdentity は収集元 1 件を指す組であり、
	// 和はどの 1 件のレコード件数でもない。合算するのは画面である。
	RecordCount *int64 `json:"recordCount,omitempty"`
	// NewlineCount は収集元の改行の個数である。単位は個であり、レコード件数と別の項目に
	// する。末尾に改行が無い収集元では 2 つの値が 1 だけ食い違う。
	NewlineCount int64 `json:"newlineCount"`
	// EndsWithNewline は末尾のレコードの後に改行があるかである。
	EndsWithNewline bool `json:"endsWithNewline"`
	// LineEnding は行末の byte 列である。
	LineEnding LineEnding `json:"lineEnding"`
	// FormatKey は入力形式である。
	FormatKey FormatKey `json:"formatKey"`
	// FormatVersion は入力形式のバージョンである。出ない場合はバージョンを読み取る項目が原資料に無い。
	// 原資料から読み取る値であるため pointer で持つ。バージョンの欄が空文字列である入力形式と、
	// バージョンの欄を持たない入力形式を別の状態で表す。
	FormatVersion *string `json:"formatVersion,omitempty"`
	// FormatSpec は収集元を読んだ欄の並びの指定である。出ない場合は、入力形式が欄の
	// 並びの指定を取らない。
	//
	// **収集元の値の意味はこの指定に依存する。** 同じ byte 列を別の指定で読むと、同じ
	// 位置の値が別の項目になる。分析者が結果を確かめる根拠であるため応答に出す。
	FormatSpec *string `json:"formatSpec,omitempty"`
	// CaseId は取り込みを求める側が収集元に付けた案件である。出ない場合は、取り込みが
	// 案件を区別しない。文字列は ValidateCaseId が定める。
	CaseId *string `json:"caseId,omitempty"`
	// CollectionPath は、収集元を取り出した収集の directory である。起動の directory からの
	// 相対 path。出ない場合は、収集元の file を 1 件ずつ指定した。
	CollectionPath string `json:"collectionPath,omitempty"`
	// ObservedRangeFirst は収集元の最初のレコードの時刻である。時点が 1 つに定まる時刻
	// (Timestamp.Instant) だけを比べる。ObservedRangeLast と揃って出る。2 つが出ない場合は、
	// 時点が定まる時刻を持つレコードが 0 件である。
	ObservedRangeFirst *Timestamp `json:"observedRangeFirst,omitempty"`
	// ObservedRangeLast は収集元の最後のレコードの時刻である。
	ObservedRangeLast *Timestamp `json:"observedRangeLast,omitempty"`
	// MessageUnrenderedCount は、書き出した端末がイベントの定義を持たず、説明を組めなかった
	// レコードの件数である。そのレコードの値は欄名を持たない。出ない場合は、入力形式が
	// 説明を組めたかを持たない。
	MessageUnrenderedCount *int64 `json:"messageUnrenderedCount,omitempty"`
	// TerminalCandidates は、レコードに現れた、収集元を記録した端末の候補である。件数の
	// 多い順、同数は名前の文字列の昇順に並ぶ。**端末に決めない。** 候補は別の端末の名前でも
	// ありうる。出ない場合は、この収集元が端末の候補を示していない。
	TerminalCandidates []TerminalCandidate `json:"terminalCandidates,omitempty"`
	// RawTextConverted が真の収集元では、レコードの原文は読み取りが収集元の byte 列から
	// 組み立てた文字列である。原資料の byte 列を指すのはレコードの位置 (byte 範囲) である。
	// 出ない場合、原文は収集元の byte 列そのものである。
	RawTextConverted bool `json:"rawTextConverted,omitempty"`
	// FileHeader は収集元の file の見出しが記録した値である。見出しを持たない入力形式と、
	// 見出しを読めなかった収集元では出ない。名前は 1 件の中で重複しない。
	FileHeader []RecordField `json:"fileHeader,omitempty"`
	// Members は、収集元を構成する file の並びである。入力形式が主 file と同じ directory の
	// 付属の file (registry の transaction log) を一緒に読む収集元だけが持つ。先頭が主 file で
	// ある。**収集元の byte 列は、並びの順に file を連結した byte 列である。** ContentSha256、
	// SizeBytes とレコードの位置は、連結した byte 列について言う。
	Members []SourceMember `json:"members,omitempty"`
}

// SourceMember は、収集元を構成する file 1 つである。
type SourceMember struct {
	// OriginPath は file の取得元である。repo root からの相対 path。
	OriginPath string `json:"originPath"`
	// ContentSha256 は file の内容の識別である。小文字 16 進 64 文字。
	ContentSha256 string `json:"contentSha256"`
	// ByteOffset は file の先頭の、連結した byte 列の中の位置である。
	ByteOffset int64 `json:"byteOffset"`
	// SizeBytes は file の byte 数である。
	SizeBytes int64 `json:"sizeBytes"`
}

// TerminalCandidate は、収集元を記録した端末の候補 1 つである。
type TerminalCandidate struct {
	// Name は候補の名前の原資料の文字列である。コンピューターのアカウント名 (`<ホスト名>$`) を含む。
	Name string `json:"name"`
	// RecordCount はこの名前が現れたレコードの件数である。1 以上。
	RecordCount int64 `json:"recordCount"`
}

// FormatVersionValue は入力形式のバージョンを返す。ok が偽になるのはバージョンが出ていないときである。
func (s SourceIdentity) FormatVersionValue() (string, bool) {
	return optionalStringValue(s.FormatVersion)
}

// FormatSpecValue は欄の並びの指定を返す。ok が偽になるのは指定が出ていないときである。
func (s SourceIdentity) FormatSpecValue() (string, bool) {
	return optionalStringValue(s.FormatSpec)
}

// UnmarshalJSON は収集元の項目を復元する。sizeBytes と newlineCount と endsWithNewline を
// 欠いた object は error を返す。3 項目は必須であり、欠けた値を 0 と偽へ復元すると、
// 「値が無い」と「値が 0」が同じ形になる。
func (s *SourceIdentity) UnmarshalJSON(data []byte) error {
	type items SourceIdentity
	var decoded struct {
		items
		SizeBytes       *int64 `json:"sizeBytes"`
		NewlineCount    *int64 `json:"newlineCount"`
		EndsWithNewline *bool  `json:"endsWithNewline"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding SourceIdentity: %w", err)
	}
	problem := firstProblem(
		requireDecodedNumber("SourceIdentity.sizeBytes", decoded.SizeBytes),
		requireDecodedNumber("SourceIdentity.newlineCount", decoded.NewlineCount),
		requireDecodedFlag("SourceIdentity.endsWithNewline", decoded.EndsWithNewline),
	)
	if problem != nil {
		return fmt.Errorf("decoding SourceIdentity: %w", problem)
	}
	value := SourceIdentity(decoded.items)
	value.SizeBytes = *decoded.SizeBytes
	value.NewlineCount = *decoded.NewlineCount
	value.EndsWithNewline = *decoded.EndsWithNewline
	*s = value
	return nil
}

// Validate は収集元の項目の整合を確かめる。
func (s SourceIdentity) Validate() error {
	problem := firstProblem(
		requirePresent("SourceIdentity.sourceId", s.SourceId),
		requireLowerHex64("SourceIdentity.contentSha256", s.ContentSha256),
		requirePresent("SourceIdentity.originPath", s.OriginPath),
		requireRelativePath("SourceIdentity.originPath", s.OriginPath),
		requirePresent("SourceIdentity.fileName", s.FileName),
		requireNonNegative("SourceIdentity.sizeBytes", s.SizeBytes),
		requireNonNegative("SourceIdentity.newlineCount", s.NewlineCount),
		requireKnownEnum("SourceIdentity.lineEnding", s.LineEnding),
		requirePresent("SourceIdentity.formatKey", string(s.FormatKey)),
	)
	if problem != nil {
		return problem
	}
	if s.RecordCount != nil {
		if problem := requireNonNegative("SourceIdentity.recordCount", *s.RecordCount); problem != nil {
			return problem
		}
	}
	// 空の指定は、並びを指定から受け取らない入力形式と同じ意味になる。
	if s.FormatSpec != nil {
		if problem := requirePresent("SourceIdentity.formatSpec", *s.FormatSpec); problem != nil {
			return problem
		}
	}
	if s.CaseId != nil {
		if problem := ValidateCaseId("SourceIdentity.caseId", *s.CaseId); problem != nil {
			return problem
		}
	}
	// 2 項目が出ない場合の意味はどちらも「時刻を持つレコードが 0 件である」であり、
	// 片方だけを持つ収集元は無い。
	if (s.ObservedRangeFirst == nil) != (s.ObservedRangeLast == nil) {
		return itemError("SourceIdentity.observedRangeFirst and "+
			"SourceIdentity.observedRangeLast", ErrInconsistentValue)
	}
	if s.ObservedRangeFirst != nil {
		if err := s.ObservedRangeFirst.Validate(); err != nil {
			return itemError("SourceIdentity.observedRangeFirst", err)
		}
	}
	if s.ObservedRangeLast != nil {
		if err := s.ObservedRangeLast.Validate(); err != nil {
			return itemError("SourceIdentity.observedRangeLast", err)
		}
	}
	if s.MessageUnrenderedCount != nil {
		if problem := requireNonNegative("SourceIdentity.messageUnrenderedCount", *s.MessageUnrenderedCount); problem != nil {
			return problem
		}
	}
	for _, candidate := range s.TerminalCandidates {
		if problem := requirePresent("SourceIdentity.terminalCandidates.name", candidate.Name); problem != nil {
			return problem
		}
		if candidate.RecordCount < 1 {
			return itemError("SourceIdentity.terminalCandidates.recordCount", ErrInconsistentValue)
		}
	}
	names := make(map[string]struct{}, len(s.FileHeader))
	for _, field := range s.FileHeader {
		if err := field.Validate(); err != nil {
			return itemError("SourceIdentity.fileHeader", err)
		}
		if _, repeated := names[field.Name]; repeated {
			return itemError("SourceIdentity.fileHeader repeats the name "+field.Name, ErrInconsistentValue)
		}
		names[field.Name] = struct{}{}
	}
	return s.validateMembers()
}

// validateMembers は、構成する file が先頭の主 file から隙間なく続き、合計が収集元の byte 数に
// 一致することを確かめる。
func (s SourceIdentity) validateMembers() error {
	if len(s.Members) == 0 {
		return nil
	}
	if s.Members[0].OriginPath != s.OriginPath {
		return itemError("SourceIdentity.members[0].originPath", ErrInconsistentValue)
	}
	next := int64(0)
	for _, member := range s.Members {
		problem := firstProblem(
			requirePresent("SourceIdentity.members.originPath", member.OriginPath),
			requireRelativePath("SourceIdentity.members.originPath", member.OriginPath),
			requireLowerHex64("SourceIdentity.members.contentSha256", member.ContentSha256),
			requireNonNegative("SourceIdentity.members.sizeBytes", member.SizeBytes),
		)
		if problem != nil {
			return problem
		}
		if member.ByteOffset != next {
			return itemError("SourceIdentity.members.byteOffset", ErrInconsistentValue)
		}
		next += member.SizeBytes
	}
	if next != s.SizeBytes {
		return itemError("SourceIdentity.members.sizeBytes", ErrInconsistentValue)
	}
	return nil
}

// TimeRange は時刻の範囲を持つ。
type TimeRange struct {
	From Timestamp `json:"from"`
	To   Timestamp `json:"to"`
}

// Validate は 2 つの時刻の整合と、両端を比べられるときの前後を確かめる。
func (r TimeRange) Validate() error {
	if err := r.From.Validate(); err != nil {
		return itemError("TimeRange.from", err)
	}
	if err := r.To.Validate(); err != nil {
		return itemError("TimeRange.to", err)
	}
	from, fromOk := r.From.Instant()
	to, toOk := r.To.Instant()
	if fromOk && toOk && to.Before(from) {
		return itemError("TimeRange.from and TimeRange.to", ErrInconsistentValue)
	}
	return nil
}

// Contains は時刻が範囲の中にあるかを返す。両端を含む閉じた範囲として判定する。
//
// **比較の単位は秒である。** 両端と引数の時刻を秒に切り捨てた値で比べ、境界の秒に並ぶ
// レコードを期間の中として扱う。切り上げると、秒までの時刻を書く収集元のレコードが、
// 境界の秒に並ぶときに範囲の外になる。
//
// ok が偽になるのは、範囲の両端または引数の時刻を time として比べられないときである。
func (r TimeRange) Contains(at Timestamp) (bool, bool) {
	from, fromOk := r.From.Instant()
	to, toOk := r.To.Instant()
	instant, instantOk := at.Instant()
	if !fromOk || !toOk || !instantOk {
		return false, false
	}
	fromSecond := truncateToSecond(from)
	toSecond := truncateToSecond(to)
	atSecond := truncateToSecond(instant)
	return !atSecond.Before(fromSecond) && !atSecond.After(toSecond), true
}
