package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// maxImportCountElements は counts の要素数の上限である。
const maxImportCountElements = 3

// maxDiagnosisCountElements は diagnosisCounts の要素数の上限である。
const maxDiagnosisCountElements = 4

// ImportCategory は取り込みの件数の区分を持つ。
type ImportCategory string

// ImportCategory の値。部分取り込み失敗の件数の区分である。
//
// 失敗原因の分類は DiagnosisClass が持つ。2 つを 1 つの列挙に混ぜない。
const (
	// ImportCategoryRead は読み込んだレコードの件数の区分である。
	ImportCategoryRead ImportCategory = "read"
	// ImportCategorySucceeded は取り込めたレコードの件数の区分である。
	ImportCategorySucceeded ImportCategory = "succeeded"
	// ImportCategoryFailed は scope の中で取り込めなかったレコードの件数の区分である。
	ImportCategoryFailed ImportCategory = "failed"
)

// IsKnown は ImportCategory が定義の中の値であるかを返す。
func (c ImportCategory) IsKnown() bool {
	switch c {
	case ImportCategoryRead, ImportCategorySucceeded, ImportCategoryFailed:
		return true
	default:
		return false
	}
}

// WithheldReason は公開を止めた理由を持つ。
type WithheldReason string

// WithheldReason の値。
const (
	// WithheldReasonIdentifierCollision は識別子が衝突した理由である。
	WithheldReasonIdentifierCollision WithheldReason = "identifier_collision"
	// WithheldReasonDanglingEvidenceReference は根拠の参照先が欠けた理由である。
	WithheldReasonDanglingEvidenceReference WithheldReason = "dangling_evidence_reference"
	// WithheldReasonMixedAnalysisRun は 2 つ以上の解析実行の結果が混ざった理由である。
	WithheldReasonMixedAnalysisRun WithheldReason = "mixed_analysis_run"
)

// IsKnown は WithheldReason が定義の中の値であるかを返す。
func (r WithheldReason) IsKnown() bool {
	switch r {
	case WithheldReasonIdentifierCollision, WithheldReasonDanglingEvidenceReference,
		WithheldReasonMixedAnalysisRun:
		return true
	default:
		return false
	}
}

// DiagnosisClass は解析失敗の診断の分類を持つ。
type DiagnosisClass string

// DiagnosisClass の値。
const (
	// DiagnosisClassUndetermined は失敗の原因を 4 分類のいずれにも確定できない状態である。
	DiagnosisClassUndetermined DiagnosisClass = "undetermined"
	// DiagnosisClassUnsupportedFormat は未対応の形式であることを確認した状態である。
	DiagnosisClassUnsupportedFormat DiagnosisClass = "unsupported_format"
	// DiagnosisClassInconsistentInputConfirmed は入力の不整合を確認した状態である。
	DiagnosisClassInconsistentInputConfirmed DiagnosisClass = "inconsistent_input_confirmed"
	// DiagnosisClassImplementationDefectConfirmed は実装不具合を確認した状態である。
	DiagnosisClassImplementationDefectConfirmed DiagnosisClass = "implementation_defect_confirmed"
)

// IsKnown は DiagnosisClass が定義の中の値であるかを返す。
func (c DiagnosisClass) IsKnown() bool {
	switch c {
	case DiagnosisClassUndetermined, DiagnosisClassUnsupportedFormat,
		DiagnosisClassInconsistentInputConfirmed, DiagnosisClassImplementationDefectConfirmed:
		return true
	default:
		return false
	}
}

// FailureStage は失敗した処理段階を持つ。
type FailureStage string

// FailureStage の値。
const (
	// FailureStageRead は収集元の byte 列を読む段階である。ImportCount の区分の read とは
	// 別の意味である。
	FailureStageRead FailureStage = "read"
	// FailureStageTokenize はレコードを文字列に分ける段階である。
	FailureStageTokenize FailureStage = "tokenize"
	// FailureStageFieldMap は文字列を項目へ対応付ける段階である。
	FailureStageFieldMap FailureStage = "field_map"
	// FailureStageNormalize は比較に用いる値を導く段階である。
	FailureStageNormalize FailureStage = "normalize"
	// FailureStageRelate はレコードどうしの関係を作る段階である。
	FailureStageRelate FailureStage = "relate"
)

// IsKnown は FailureStage が定義の中の値であるかを返す。
func (s FailureStage) IsKnown() bool {
	switch s {
	case FailureStageRead, FailureStageTokenize, FailureStageFieldMap,
		FailureStageNormalize, FailureStageRelate:
		return true
	default:
		return false
	}
}

// requiresRecordRef は失敗した処理段階がレコードの位置を必ず持つかを返す。
//
// field_map / normalize / relate の失敗は、入力形式と位置の項目を読めたかに応じて
// レコードの位置を確定する。入力形式が持つ通番を読めない場合は行番号で指す。
func (s FailureStage) requiresRecordRef() bool {
	switch s {
	case FailureStageFieldMap, FailureStageNormalize, FailureStageRelate:
		return true
	default:
		return false
	}
}

// ImportCount は 1 つの区分の件数を持つ。
type ImportCount struct {
	// Category は取り込み結果の区分である。
	Category ImportCategory `json:"category"`
	// Count はその区分の件数である。単位は件。
	Count int64 `json:"count"`
}

// Validate は区分が既知であり、件数が負でないことを確かめる。
func (c ImportCount) Validate() error {
	return firstProblem(
		requireKnownEnum("ImportCount.category", c.Category),
		requireNonNegative("ImportCount.count", c.Count),
	)
}

// UnmarshalJSON は全項目を復元する。count を欠いた object は error を返す。
// count に 0 を置いて「数えていない」を表さないため、欠けた count を 0 へ復元しない。
func (c *ImportCount) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Category ImportCategory `json:"category"`
		Count    *int64         `json:"count"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding ImportCount: %w", err)
	}
	if decoded.Count == nil {
		return fmt.Errorf("decoding ImportCount: %w",
			itemError("ImportCount.count", ErrMissingRequiredItem))
	}
	value := ImportCount{Category: decoded.Category, Count: *decoded.Count}
	if err := value.Validate(); err != nil {
		return fmt.Errorf("decoding ImportCount: %w", err)
	}
	*c = value
	return nil
}

// ImportCountSet は区分ごとの件数の集合を持つ。
// 区分の一意性と、既知の区分であること、件数が負でないこと、要素数が上限 3 に収まること、
// 3 区分を揃えた集合で read が succeeded と failed の和に一致することを、構築と復元の
// 境界で検査する。
//
// 数えていない区分は要素を持たない形で表す。件数が 0 の区分は要素を持ち Count が 0 になる。
// 2 つの状態は Count の返す ok で分かれる。
type ImportCountSet struct {
	counts []ImportCount
}

// NewImportCountSet は 5 つの制約を検査した件数の集合を返す。
func NewImportCountSet(counts ...ImportCount) (ImportCountSet, error) {
	if problem := validateImportCounts(counts); problem != nil {
		return ImportCountSet{}, problem
	}
	stored := make([]ImportCount, len(counts))
	copy(stored, counts)
	return ImportCountSet{counts: stored}, nil
}

// validateImportCounts は区分の一意性、既知の区分、非負の件数、要素数の上限、
// 3 区分を揃えた集合の和の一致を確かめる。
func validateImportCounts(counts []ImportCount) error {
	if len(counts) > maxImportCountElements {
		return itemError("ImportStatus.counts", ErrTooManyElements)
	}
	seen := make(map[ImportCategory]int64, len(counts))
	for index, count := range counts {
		if problem := count.Validate(); problem != nil {
			return itemError("ImportStatus.counts at "+formatIndex(index), problem)
		}
		if _, duplicate := seen[count.Category]; duplicate {
			return itemError("ImportStatus.counts has a repeated category "+
				string(count.Category), ErrDuplicateElement)
		}
		seen[count.Category] = count.Count
	}
	return validateReadIsTheSumOfBothOutcomes(seen)
}

// validateReadIsTheSumOfBothOutcomes は 3 区分を揃えて出す集合で、read の件数が
// succeeded と failed の件数の和に一致することを確かめる。
//
// 位置を確定できない失敗がある応答でも同じ一致が成立する。レコードの区切りを確定する前に
// 止まった失敗は read の件数にも数えられていないためである。
func validateReadIsTheSumOfBothOutcomes(counts map[ImportCategory]int64) error {
	read, hasRead := counts[ImportCategoryRead]
	succeeded, hasSucceeded := counts[ImportCategorySucceeded]
	failed, hasFailed := counts[ImportCategoryFailed]
	if !hasRead || !hasSucceeded || !hasFailed {
		return nil
	}
	total, problem := sumCounts("ImportStatus.counts", succeeded, failed)
	if problem != nil {
		return problem
	}
	if read != total {
		return itemError("ImportStatus.counts read differs from the sum of succeeded and failed",
			ErrInconsistentValue)
	}
	return nil
}

// Count は区分の件数を返す。ok が偽になるのは、その区分を数えていないときである。
func (s ImportCountSet) Count(category ImportCategory) (int64, bool) {
	for _, count := range s.counts {
		if count.Category == category {
			return count.Count, true
		}
	}
	return 0, false
}

// Len は集合の要素数を返す。
func (s ImportCountSet) Len() int {
	return len(s.counts)
}

// DiagnosisCount は 1 つの失敗原因の分類の件数を持つ。
type DiagnosisCount struct {
	// DiagnosisClass は失敗原因の分類である。
	DiagnosisClass DiagnosisClass `json:"diagnosisClass"`
	// Count はその分類の件数である。単位は件。
	Count int64 `json:"count"`
}

// Validate は分類が既知であり、件数が負でないことを確かめる。
func (c DiagnosisCount) Validate() error {
	return firstProblem(
		requireKnownEnum("DiagnosisCount.diagnosisClass", c.DiagnosisClass),
		requireNonNegative("DiagnosisCount.count", c.Count),
	)
}

// UnmarshalJSON は全項目を復元する。count を欠いた object は error を返す。
// count に 0 を置いて「数えていない」を表さないため、欠けた count を 0 へ復元しない。
func (c *DiagnosisCount) UnmarshalJSON(data []byte) error {
	var decoded struct {
		DiagnosisClass DiagnosisClass `json:"diagnosisClass"`
		Count          *int64         `json:"count"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding DiagnosisCount: %w", err)
	}
	if decoded.Count == nil {
		return fmt.Errorf("decoding DiagnosisCount: %w",
			itemError("DiagnosisCount.count", ErrMissingRequiredItem))
	}
	value := DiagnosisCount{DiagnosisClass: decoded.DiagnosisClass, Count: *decoded.Count}
	if err := value.Validate(); err != nil {
		return fmt.Errorf("decoding DiagnosisCount: %w", err)
	}
	*c = value
	return nil
}

// validateDiagnosisCounts は分類の一意性、既知の分類、非負の件数、要素数の上限を確かめる。
//
// 数えていない分類は要素を持たない形で表す。件数が 0 の分類は要素を持ち Count が 0 になる。
func validateDiagnosisCounts(counts []DiagnosisCount) error {
	if len(counts) > maxDiagnosisCountElements {
		return itemError("ImportStatus.diagnosisCounts", ErrTooManyElements)
	}
	seen := make(map[DiagnosisClass]struct{}, len(counts))
	for index, count := range counts {
		if problem := count.Validate(); problem != nil {
			return itemError("ImportStatus.diagnosisCounts at "+formatIndex(index), problem)
		}
		if _, duplicate := seen[count.DiagnosisClass]; duplicate {
			return itemError("ImportStatus.diagnosisCounts has a repeated diagnosisClass "+
				string(count.DiagnosisClass), ErrDuplicateElement)
		}
		seen[count.DiagnosisClass] = struct{}{}
	}
	return nil
}

// All は集合の要素を、呼び出し側が書き換えられない複製として返す。
func (s ImportCountSet) All() []ImportCount {
	if len(s.counts) == 0 {
		return nil
	}
	copied := make([]ImportCount, len(s.counts))
	copy(copied, s.counts)
	return copied
}

// Validate は 5 つの制約を確かめる。
func (s ImportCountSet) Validate() error {
	return validateImportCounts(s.counts)
}

// MarshalJSON は集合を要素の並びとして直列化する。要素が無い集合も並びとして出す。
func (s ImportCountSet) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("marshaling ImportCountSet: %w", err)
	}
	counts := s.counts
	if counts == nil {
		counts = []ImportCount{}
	}
	encoded, err := json.Marshal(counts)
	if err != nil {
		return nil, fmt.Errorf("marshaling ImportCountSet: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON は集合を復元し、NewImportCountSet と同じ制約を検査する。
func (s *ImportCountSet) UnmarshalJSON(data []byte) error {
	var decoded []ImportCount
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding ImportCountSet: %w", err)
	}
	rebuilt, err := NewImportCountSet(decoded...)
	if err != nil {
		return fmt.Errorf("decoding ImportCountSet: %w", err)
	}
	*s = rebuilt
	return nil
}

// ImportFailure は取り込みの失敗 1 件を持つ。原文を持たず、原文への参照だけを持つ。
type ImportFailure struct {
	// SourceId は失敗した取り込みの収集元である。
	SourceId string `json:"sourceId"`
	// SourceContentSha256 は同じ収集元の内容の識別である。
	SourceContentSha256 string `json:"sourceContentSha256"`
	// RecordRef は失敗したレコードの位置である。
	// 出ない場合はレコードの位置を確定できていない。
	RecordRef *RecordLocator `json:"recordRef,omitempty"`
	// LineNumber は確定できた行番号である。1 起点。
	// 出ない場合は行番号を確定できていない。
	LineNumber *int64 `json:"lineNumber,omitempty"`
	// DiagnosisClass は取り込みが失敗した原因の分類である。
	DiagnosisClass DiagnosisClass `json:"diagnosisClass"`
	// Stage は失敗した処理段階である。
	Stage FailureStage `json:"stage"`
	// ByteOffset は収集元の中の byte offset である。
	// 出ない場合は byte offset を数えていない。
	ByteOffset *int64 `json:"byteOffset,omitempty"`
	// Interpretation は文字コード、改行、日時の解釈である。
	Interpretation  string `json:"interpretation"`
	ExpectedMeaning string `json:"expectedMeaning"`
	ObservedResult  string `json:"observedResult"`
	// UnresolvedReason は DiagnosisClass が undetermined のとき必須である。
	UnresolvedReason string `json:"unresolvedReason,omitempty"`
	// RecordTruncated は、レコードが書き出した側の 1 行の長さの上限で終わり、途中で切れて
	// いるかである。真のレコードは、切れる前に読めた欄を持つレコードとしても取り込む。
	RecordTruncated bool `json:"recordTruncated,omitempty"`
	// ParserVersion はパーサーと対応形式とコード revision と設定である。
	// 値の作り方は本 package が決めない。
	ParserVersion string `json:"parserVersion"`
	// SanitizedMessage は診断ログと端末出力に出す表現である。
	// 制御文字と改行を持たない。token と credential を載せない。
	SanitizedMessage string `json:"sanitizedMessage"`
	// RawTextRef はレコードの原文を返す操作への参照である。
	// RecordRef があるとき必須。出ない場合はレコードの位置を確定できていない。
	RawTextRef string `json:"rawTextRef,omitempty"`
}

// Validate は項目の整合を確かめる。
//
// 失敗の位置は 3 段階の細かさで持つ。レコードの区切りを確定する前に処理が止まった失敗で
// recordRef を zero value で埋めない。
func (f ImportFailure) Validate() error {
	problem := firstProblem(
		requirePresent("ImportFailure.sourceId", f.SourceId),
		requireLowerHex64("ImportFailure.sourceContentSha256", f.SourceContentSha256),
		requireKnownEnum("ImportFailure.diagnosisClass", f.DiagnosisClass),
		requireKnownEnum("ImportFailure.stage", f.Stage),
		requirePresent("ImportFailure.interpretation", f.Interpretation),
		requirePresent("ImportFailure.expectedMeaning", f.ExpectedMeaning),
		requirePresent("ImportFailure.observedResult", f.ObservedResult),
		requirePresent("ImportFailure.parserVersion", f.ParserVersion),
		requirePresent("ImportFailure.sanitizedMessage", f.SanitizedMessage),
		requireSanitized("ImportFailure.sanitizedMessage", f.SanitizedMessage),
	)
	if problem != nil {
		return problem
	}
	if problem := f.validatePosition(); problem != nil {
		return problem
	}
	if f.DiagnosisClass == DiagnosisClassUndetermined {
		return requirePresent("ImportFailure.unresolvedReason", f.UnresolvedReason)
	}
	return nil
}

// validatePosition は位置の項目の整合を確かめる。
//
// rawTextRef は recordRef があるとき必須で、無いとき出ない。recordRef があるとき、
// lineNumber は recordRef の同名の項目と同じ値になる。
func (f ImportFailure) validatePosition() error {
	if f.ByteOffset != nil {
		if problem := requireNonNegative(
			"ImportFailure.byteOffset", *f.ByteOffset); problem != nil {
			return problem
		}
	}
	if f.LineNumber != nil && *f.LineNumber < firstLineNumber {
		return itemError("ImportFailure.lineNumber", ErrInvalid)
	}
	if f.RecordRef == nil {
		// field_map / normalize / relate の失敗は recordRef を必須とする。
		if f.Stage.requiresRecordRef() {
			return itemError("ImportFailure.recordRef on a "+string(f.Stage)+" failure",
				ErrMissingRequiredItem)
		}
		return requireAbsent("ImportFailure.rawTextRef", f.RawTextRef)
	}
	if problem := f.RecordRef.Validate(); problem != nil {
		return itemError("ImportFailure.recordRef", problem)
	}
	if problem := requirePresent("ImportFailure.rawTextRef", f.RawTextRef); problem != nil {
		return problem
	}
	return f.validatePositionAgainstRecordRef()
}

// validatePositionAgainstRecordRef は失敗が指す収集元と行番号が、レコード位置の同名の項目と
// 同じ値であることを確かめる。
func (f ImportFailure) validatePositionAgainstRecordRef() error {
	if f.RecordRef.SourceId != f.SourceId {
		return itemError("ImportFailure.recordRef.sourceId differs from ImportFailure.sourceId",
			ErrInconsistentValue)
	}
	if f.RecordRef.SourceContentSha256 != f.SourceContentSha256 {
		return itemError("ImportFailure.recordRef.sourceContentSha256 differs from "+
			"ImportFailure.sourceContentSha256", ErrInconsistentValue)
	}
	// 行番号を数えている失敗は、LineNumber と RecordRef.LineNumber に同じ値を持つ。
	// 片方だけを持つ応答を作らない。
	if (f.LineNumber == nil) != (f.RecordRef.LineNumber == nil) {
		return itemError("ImportFailure.lineNumber and ImportFailure.recordRef.lineNumber",
			ErrInconsistentValue)
	}
	if f.LineNumber != nil && *f.LineNumber != *f.RecordRef.LineNumber {
		return itemError("ImportFailure.lineNumber differs from ImportFailure.recordRef.lineNumber",
			ErrInconsistentValue)
	}
	return nil
}

// ImportStatus は 1 つの収集元の取り込みの状態を持つ。成功と失敗の 2 値の項目を持たない。
type ImportStatus struct {
	SourceId string         `json:"sourceId"`
	Scope    RecordRange    `json:"scope"`
	Counts   ImportCountSet `json:"counts"`
	// DiagnosisCounts は失敗原因の分類ごとの件数の集合である。要素数 0 の場合も集合である。
	// 要素の Count の和が一致する相手は FailureCount である。
	DiagnosisCounts  []DiagnosisCount `json:"diagnosisCounts"`
	PublicationState PublicationState `json:"publicationState"`
	// WithheldReason は PublicationState が withheld のとき必須である。
	WithheldReason WithheldReason  `json:"withheldReason,omitempty"`
	Failures       []ImportFailure `json:"failures"`
	// FailureCount は失敗の総数である。
	FailureCount int64 `json:"failureCount"`
	// AnalysisRunRef は取り込みを行った解析実行への参照である。
	// 値の作り方は本 package が決めない。
	AnalysisRunRef string `json:"analysisRunRef"`
}

// Validate は項目の整合を確かめる。
func (s ImportStatus) Validate() error {
	problem := firstProblem(
		requirePresent("ImportStatus.sourceId", s.SourceId),
		requireKnownEnum("ImportStatus.publicationState", s.PublicationState),
		requirePresent("ImportStatus.analysisRunRef", s.AnalysisRunRef),
	)
	if problem != nil {
		return problem
	}
	if scopeProblem := s.Scope.Validate(); scopeProblem != nil {
		return itemError("ImportStatus.scope", scopeProblem)
	}
	if s.Scope.SourceId != s.SourceId {
		return itemError("ImportStatus.scope.sourceId differs from ImportStatus.sourceId",
			ErrInconsistentValue)
	}
	if countsProblem := s.Counts.Validate(); countsProblem != nil {
		return itemError("ImportStatus.counts", countsProblem)
	}
	if diagnosisProblem := validateDiagnosisCounts(s.DiagnosisCounts); diagnosisProblem != nil {
		return diagnosisProblem
	}
	// diagnosisCounts を出す応答は counts に failed の要素を持たせる。
	if len(s.DiagnosisCounts) > 0 {
		if _, counted := s.Counts.Count(ImportCategoryFailed); !counted {
			return itemError("ImportStatus.counts omits the failed category while "+
				"ImportStatus.diagnosisCounts has elements", ErrInconsistentValue)
		}
	}
	// 公開を止めた状態だけが理由を持つ。
	withheldReasonProblem := requireAbsent("ImportStatus.withheldReason", string(s.WithheldReason))
	if s.PublicationState == PublicationStateWithheld {
		withheldReasonProblem = requireKnownEnum("ImportStatus.withheldReason", s.WithheldReason)
	}
	if withheldReasonProblem != nil {
		return withheldReasonProblem
	}
	return s.validateFailures()
}

// validateFailures は失敗の要素と、件数と打ち切りの対応を確かめる。
func (s ImportStatus) validateFailures() error {
	for index, failure := range s.Failures {
		if problem := failure.Validate(); problem != nil {
			return itemError("ImportStatus.failures at "+formatIndex(index), problem)
		}
		if failure.SourceId != s.SourceId {
			return itemError("ImportStatus.failures at "+formatIndex(index)+
				" points at another source", ErrInconsistentValue)
		}
	}
	if problem := s.validateFailureScope(); problem != nil {
		return problem
	}
	if problem := validateCountedSet("ImportStatus.failure", s.FailureCount,
		len(s.Failures)); problem != nil {
		return problem
	}
	// 分類ごとの件数の和は failureCount と一致する。failures を打ち切った応答でも
	// 2 つの値は変わらない。どちらも打ち切りの前に数えた総数である。
	classCounts := make(map[DiagnosisClass]int64, len(s.DiagnosisCounts))
	classTotals := make([]int64, 0, len(s.DiagnosisCounts))
	for _, count := range s.DiagnosisCounts {
		classCounts[count.DiagnosisClass] = count.Count
		classTotals = append(classTotals, count.Count)
	}
	diagnosisTotal, problem := sumCounts("ImportStatus.diagnosisCounts", classTotals...)
	if problem != nil {
		return problem
	}
	if diagnosisTotal != s.FailureCount {
		return itemError("the sum of ImportStatus.diagnosisCounts differs from "+
			"ImportStatus.failureCount", ErrInconsistentValue)
	}
	if problem := s.validatePublicationAgainstFailures(); problem != nil {
		return problem
	}
	return s.validateFailuresAgainstCounts(classCounts)
}

// validateFailureScope は失敗が指す収集元が、件数の対象とする範囲の収集元と同じである
// ことを確かめる。
//
// sourceId は取り込み 1 件を指し、contentSha256 は同じ取り込みの内容を指す。
// ImportStatus は内容の識別を scope の sourceContentSha256 で持つ。
//
// 既知の制限: 位置の範囲の包含は確かめない,
// failures の recordRef が scope の範囲の中にあることを求める規則が無く、
// 包含を求める根拠が無い, recordRef と scope の包含を求める規則が決まったとき見直す。
func (s ImportStatus) validateFailureScope() error {
	for index, failure := range s.Failures {
		if failure.SourceContentSha256 != s.Scope.SourceContentSha256 {
			return itemError("ImportStatus.failures at "+formatIndex(index)+
				" carries another content identifier than ImportStatus.scope",
				ErrInconsistentValue)
		}
	}
	return nil
}

// validatePublicationAgainstFailures は公開の状態を、位置を確定できた失敗と突き合わせる。
//
// レコードの位置を確定できた失敗を持つ収集元は published_partial になる。位置を確定できた
// 失敗があることを応答が示すのは、返した失敗が recordRef を持つ場合と、counts の failed の
// count が 1 以上である場合である。
//
// 位置を確定できない失敗だけを持つ収集元の publicationState は未決であるため、
// failureCount が 1 以上であることだけでは published_full を拒否しない。
func (s ImportStatus) validatePublicationAgainstFailures() error {
	if s.PublicationState != PublicationStatePublishedFull {
		return nil
	}
	for index, failure := range s.Failures {
		if failure.RecordRef != nil {
			return itemError("ImportStatus.publicationState is published_full while the failure at "+
				formatIndex(index)+" carries a record position", ErrInconsistentValue)
		}
	}
	if failed, counted := s.Counts.Count(ImportCategoryFailed); counted && failed > 0 {
		return itemError("ImportStatus.publicationState is published_full while the failed "+
			"category counts a record", ErrInconsistentValue)
	}
	return nil
}

// validateFailuresAgainstCounts は返却した失敗を分類別と位置確定の有無で集計し、
// 対応する総数と突き合わせる。
//
// 失敗 1 件はその diagnosisClass に数えられ、レコードの位置を確定できた 1 件は counts の
// failed にも算入される。算入の条件はレコードの位置を確定できたかだけで決まる。
//
// 総数は打ち切りの前に数えた値であるため、集計は下限になる。応答が失敗の全数を持つとき、
// 分類ごとの件数と位置を確定できた件数は集計と一致する。
func (s ImportStatus) validateFailuresAgainstCounts(classCounts map[DiagnosisClass]int64) error {
	returnedByClass := make(map[DiagnosisClass]int64, len(s.Failures))
	positionedCount := int64(0)
	for index, failure := range s.Failures {
		returnedByClass[failure.DiagnosisClass]++
		if counted, found := classCounts[failure.DiagnosisClass]; !found ||
			counted < returnedByClass[failure.DiagnosisClass] {
			return itemError("ImportStatus.diagnosisCounts does not count the failure at "+
				formatIndex(index)+" of the class "+string(failure.DiagnosisClass),
				ErrInconsistentValue)
		}
		if failure.RecordRef != nil {
			positionedCount++
		}
	}
	failed, counted := s.Counts.Count(ImportCategoryFailed)
	// failed が数えるのは、位置を確定できた失敗である。位置を確定できない失敗は
	// failures の要素になり failed に数えられないため、failed は failureCount を超えない。
	// 応答が失敗の一部しか含まないページでも、2 つの総数は打ち切りの前に数えた値なので
	// 関係は変わらない。
	if counted && failed > s.FailureCount {
		return itemError("ImportStatus.counts failed is larger than ImportStatus.failureCount",
			ErrInconsistentValue)
	}
	if positionedCount > 0 && (!counted || failed < positionedCount) {
		return itemError("ImportStatus.counts counts fewer failed records than the returned "+
			"failures whose record position is known", ErrInconsistentValue)
	}
	if int64(len(s.Failures)) != s.FailureCount {
		return nil
	}
	for _, count := range s.DiagnosisCounts {
		if count.Count != returnedByClass[count.DiagnosisClass] {
			return itemError("ImportStatus.diagnosisCounts of the class "+
				string(count.DiagnosisClass)+" differs from the number of returned failures "+
				"while the response carries every failure", ErrInconsistentValue)
		}
	}
	if counted && failed != positionedCount {
		return itemError("ImportStatus.counts failed differs from the number of returned "+
			"failures whose record position is known while the response carries every failure",
			ErrInconsistentValue)
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (s ImportStatus) MarshalJSON() ([]byte, error) {
	// items は ImportStatus の method を持たないため、この Marshal は再帰しない。
	type items ImportStatus
	copied := items(s)
	copied.DiagnosisCounts = emptyIfNil(copied.DiagnosisCounts)
	copied.Failures = emptyIfNil(copied.Failures)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling ImportStatus: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON は全項目を復元する。failureCount を欠いた object は error を返す。
// 同項目は必須であり、欠けた総数を 0 へ復元すると、打ち切りの前に数えた失敗の総数が
// 0 件として読める形になる。
func (s *ImportStatus) UnmarshalJSON(data []byte) error {
	type items ImportStatus
	var decoded struct {
		items
		FailureCount *int64 `json:"failureCount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decoding ImportStatus: %w", err)
	}
	if problem := requireDecodedNumber(
		"ImportStatus.failureCount", decoded.FailureCount); problem != nil {
		return fmt.Errorf("decoding ImportStatus: %w", problem)
	}
	value := ImportStatus(decoded.items)
	value.FailureCount = *decoded.FailureCount
	*s = value
	return nil
}
