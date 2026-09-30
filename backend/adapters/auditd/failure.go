package auditd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// errNotReset は Reset を呼ぶ前に Next を呼んだことを表す。
var errNotReset = errors.New("auditd: the Reader has no input. call Reset first")

// errRecordTooLong は 1 行が byte 数の上限を超えたことを表す。
var errRecordTooLong = errors.New("auditd: the line exceeds the byte limit of one line")

// tokenizeProblem は 1 行の文字列の分割が止まった位置と理由を持つ。
//
// 公開しない。呼び出し側が受け取るのは core.ImportFailure である。
type tokenizeProblem struct {
	byteOffset int64
	expected   string
	observed   string
	// diagnosisClass は通常の文字列不整合では空で、独立した根拠で分類できた形だけが持つ。
	diagnosisClass core.DiagnosisClass
}

// unresolvedReasonTokenize は、文字列の分割の失敗の原因を 4 分類のいずれにも確定できない
// 理由である。
const unresolvedReasonTokenize = "the line does not follow the expected lexical structure. " +
	"a malformed input and a defect of this tokenizer are not told apart by the structure alone"

// unresolvedReasonRead は、収集元を読めなくなった原因を 4 分類のいずれにも確定できない
// 理由である。
const unresolvedReasonRead = "reading the source stopped. " +
	"a truncated source and a defect of the caller are not told apart by the stop alone"

// interpretationTokenize は、文字列の分割が何をどう読んだかである。
const interpretationTokenize = "byte sequence read as UTF-8, line ending CR LF or LF, " +
	"each line read as key=value pairs with the values after 0x1D kept as a separate set"

// importFailure は文字列の分割が埋められる項目だけを埋めた core.ImportFailure を返す。
//
// **返した値はそのままでは Validate を通らない。** core.ImportFailure が必須とする
// sourceId と sourceContentSha256 と parserVersion と sanitizedMessage は、文字列の分割が
// 知り得ない。4 項目を埋めるのはパーサーと取り込みの実行である。
//
// sanitizedMessage を埋めない。無害化は出力境界の責務であり、adapters は
// backend/output を import できない。原資料側の証拠は observedResult が持つ。
func (p tokenizeProblem) importFailure(lineNumber, lineByteOffset int64) core.ImportFailure {
	byteOffset := lineByteOffset + p.byteOffset
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
		Interpretation:   interpretationTokenize,
		ExpectedMeaning:  p.expected,
		ObservedResult:   p.observed,
		UnresolvedReason: unresolvedReason,
	}
}

// truncatedHeadFailure は、収集元の先頭が事象の途中で切れている状態を返す。
//
// 判定の根拠は、収集元の最初の事象が SYSCALL の行を持たないまま、SYSCALL がある事象に
// だけ現れる種別の行を持つことである。その種別の行が単独で始まるのは、同じ事象の
// 先行する行が収集元に入っていないときである。**入っていない範囲の byte 数は測れない。**
func truncatedHeadFailure(record Record) core.ImportFailure {
	lineNumber := record.LineNumber()
	byteOffset := record.ByteOffset()
	types := make([]string, 0, len(record.lines))
	for _, line := range record.lines {
		types = append(types, line.Type())
	}
	return core.ImportFailure{
		DiagnosisClass: core.DiagnosisClassInconsistentInputConfirmed,
		Stage:          core.FailureStageRead,
		LineNumber:     &lineNumber,
		ByteOffset:     &byteOffset,
		Interpretation: interpretationTokenize,
		ExpectedMeaning: "the first event of the source starting at its own first line, " +
			"with the SYSCALL line that the companion lines belong to",
		ObservedResult: "the first event " + record.Event().RawText + " carries " +
			strconv.Itoa(len(record.lines)) + " lines of " + strconv.Quote(fmt.Sprint(types)) +
			" and no SYSCALL line, so the source begins inside an event",
	}
}

// readFailure は収集元を読めなくなった失敗を返す。stage は read である。
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

// quoteByte は 1 byte を診断に載せる形へ直す。
//
// 原資料の byte をそのまま載せず、16 進 2 桁の値として載せる。制御文字が診断ログの 1 件を
// 複数行に割ることを防ぐ。auditd の行は 0x1D を持つため、この形が要る。
func quoteByte(b byte) string {
	return fmt.Sprintf("0x%02x", b)
}

// observedAt は position の位置に何があったかを診断に載せる形へ直す。
func observedAt(rawText string, position int) string {
	if position >= len(rawText) {
		return "the end of the line at byte offset " + strconv.Itoa(position)
	}
	return "the byte " + quoteByte(rawText[position]) + " at byte offset " + strconv.Itoa(position)
}
