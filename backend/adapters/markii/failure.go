package markii

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// errNotReset は Reset を呼ぶ前に Next を呼んだことを表す。
var errNotReset = errors.New("markii: the Reader has no input. call Reset first")

// errRecordTooLong はレコードが byte 数の上限を超えたことを表す。
var errRecordTooLong = errors.New("markii: the record exceeds the byte limit of one record")

// tokenizeProblem は 1 レコードの文字列の分割が止まった位置と理由を持つ。
//
// 公開しない。呼び出し側が受け取るのは core.ImportFailure である。
//
// 失敗の形を表す識別子を持たない。文字列の分割の失敗を機械的に読む側は別 package にいて、
// 本型が非公開である限り識別子を参照できない。読む側が受け取るのは
// core.ImportFailure の expectedMeaning と observedResult である。
type tokenizeProblem struct {
	byteOffset int64
	expected   string
	observed   string
	// diagnosisClass は通常の文字列不整合では空で、独立した根拠で分類できた形だけが持つ。
	diagnosisClass core.DiagnosisClass
}

// unresolvedReasonTokenize は、文字列の分割の失敗の原因を 4 分類のいずれにも確定できない
// 理由である。
//
// 文字列が期待した構造に合わないことだけでは、入力の不整合と実装の不具合を分けられない。
// パーサーの例外や schema validator の不合格だけで入力不整合に分類しない。
const unresolvedReasonTokenize = "the record does not follow the expected lexical structure. " +
	"a malformed input and a defect of this tokenizer are not told apart by the structure alone"

// dosEOFMarkerProblem は、ファイル末尾の 1 byte の 0x1a を DOS EOF の marker として
// 扱い、レコードではない入力の不整合と診断する。
// 末尾の 1 byte だけを対象にし、レコードの途中に現れた同じ byte は通常の未判定失敗に残す。
func dosEOFMarkerProblem() *tokenizeProblem {
	return &tokenizeProblem{
		expected:       "a markii record with a header and key=value fields",
		observed:       "a single DOS EOF marker byte 0x1a, not a markii record",
		diagnosisClass: core.DiagnosisClassInconsistentInputConfirmed,
	}
}

// unresolvedReasonRead は、収集元を読めなくなった原因を 4 分類のいずれにも確定できない
// 理由である。
const unresolvedReasonRead = "reading the source stopped. " +
	"a truncated source and a defect of the caller are not told apart by the stop alone"

// importFailure は文字列の分割が埋められる項目だけを埋めた core.ImportFailure を返す。
// stage は tokenize である。
//
// **返した値はそのままでは Validate を通らない。** core.ImportFailure が必須とする
// sourceId と sourceContentSha256 と parserVersion と sanitizedMessage は、文字列の分割が知り得ない。
// 4 項目を埋めるのはプロセス開始記録と通信記録のパーサーと取り込みの実行である。
//
// sanitizedMessage を埋めない。無害化は出力境界の責務であり、adapters は
// backend/output を import できない。外部由来の文字列は、出力境界で無害化してから log と
// 画面に出す。原資料側の証拠は observedResult が持つ。
//
// recordRef と rawTextRef を埋めない。stage が read または tokenize の失敗は、
// レコードの区切りを確定する前に止まる場合があり、位置の無い失敗を zero value で
// 埋めない。位置は lineNumber と byteOffset で持つ。
func (p tokenizeProblem) importFailure(lineNumber, recordByteOffset int64) core.ImportFailure {
	byteOffset := recordByteOffset + p.byteOffset
	diagnosisClass := p.diagnosisClass
	if diagnosisClass == "" {
		diagnosisClass = core.DiagnosisClassUndetermined
	}
	unresolvedReason := unresolvedReasonTokenize
	if diagnosisClass != core.DiagnosisClassUndetermined {
		unresolvedReason = ""
	}
	return core.ImportFailure{
		DiagnosisClass:   diagnosisClass,
		Stage:            core.FailureStageTokenize,
		LineNumber:       &lineNumber,
		ByteOffset:       &byteOffset,
		Interpretation:   "byte sequence read as UTF-8, line ending CR LF or LF, the header kept as text without parsing it as a date and time",
		ExpectedMeaning:  p.expected,
		ObservedResult:   p.observed,
		UnresolvedReason: unresolvedReason,
	}
}

// readFailure は収集元を読めなくなった失敗を返す。stage は read である。
//
// 文字列の分割の失敗と同じ項目を埋める。読み込みの失敗の内容は error が持ち、
// 本値が持つのは止まった位置である。
func readFailure(lineNumber, byteOffset int64, cause string) core.ImportFailure {
	return core.ImportFailure{
		DiagnosisClass:   core.DiagnosisClassUndetermined,
		Stage:            core.FailureStageRead,
		LineNumber:       &lineNumber,
		ByteOffset:       &byteOffset,
		Interpretation:   "byte sequence read as UTF-8, line ending CR LF or LF",
		ExpectedMeaning:  "the byte sequence of the source up to its end",
		ObservedResult:   cause,
		UnresolvedReason: unresolvedReasonRead,
	}
}

// fieldProblem は意味付けが止まった理由を持つ。
//
// 公開しない。呼び出し側が受け取るのは core.ImportFailure である。
type fieldProblem struct {
	expected string
	observed string
}

// recordSemantics は、意味付けの失敗の文面のうちレコードの種別ごとに異なる 2 つを持つ。
//
// 1 つの構造体にまとめる。個別の文字列の引数を並べると、呼び出し側で順番を取り違えても
// compile が通る。
type recordSemantics struct {
	// interpretation は何をどう読んだかである。core.ImportFailure の Interpretation になる。
	interpretation string
	// fieldMapReason は stage が field_map のときの未判定の理由である。
	fieldMapReason string
}

// unresolvedReasonNormalize は、比較に用いる値を導けなかった原因を 4 分類のいずれにも
// 確定できない理由である。
//
// 文面がレコードの種別を含まないため、種別ごとに変えない。
const unresolvedReasonNormalize = "the value cannot be read as the meaning the format defines. " +
	"a malformed input, an unsupported output of another version, and a defect of this parser " +
	"are not told apart by the value alone"

// importFailure は意味付けが埋められる項目だけを埋めた core.ImportFailure を返す。
//
// **返した値はそのままでは Validate を通らない。** core.ImportFailure が必須とする
// sourceId と sourceContentSha256 と parserVersion と sanitizedMessage は、意味付けが知り得ない。
// 4 項目を埋めるのは取り込みの実行である。
//
// recordRef と rawTextRef を埋めない。stage が field_map の失敗は recordRef を必須と
// するが、sn を読めないレコードでは RecordLocator を組めないため、
// 位置は lineNumber と byteOffset で持つ。
func (p fieldProblem) importFailure(
	stage core.FailureStage, record Record, semantics recordSemantics,
) core.ImportFailure {
	lineNumber := record.LineNumber()
	byteOffset := record.ByteOffset()
	unresolved := semantics.fieldMapReason
	if stage == core.FailureStageNormalize {
		unresolved = unresolvedReasonNormalize
	}
	return core.ImportFailure{
		DiagnosisClass:   core.DiagnosisClassUndetermined,
		Stage:            stage,
		LineNumber:       &lineNumber,
		ByteOffset:       &byteOffset,
		Interpretation:   semantics.interpretation,
		ExpectedMeaning:  p.expected,
		ObservedResult:   p.observed,
		UnresolvedReason: unresolved,
	}
}

// quoteByte は 1 byte を診断に載せる形へ直す。
//
// 原資料の byte をそのまま載せず、16 進 2 桁の値として載せる。制御文字が診断ログの 1 件を
// 複数行に割ることを防ぐ。
func quoteByte(b byte) string {
	return fmt.Sprintf("0x%02x", b)
}

// observedAt は position の位置に何があったかを診断に載せる形へ直す。
func observedAt(rawText string, position int) string {
	if position >= len(rawText) {
		return "the end of the record at byte offset " + strconv.Itoa(position)
	}
	return "the byte " + quoteByte(rawText[position]) + " at byte offset " + strconv.Itoa(position)
}
