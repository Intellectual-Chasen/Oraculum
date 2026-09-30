package squid

import (
	"errors"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errRecordTooLong = errors.New("squid: record exceeds the byte limit")
var errNotReset = errors.New("squid: call Reset with an input first")

type tokenizeProblem struct {
	offset   int
	expected string
}

// failureAt はこの層で確定できる項目を埋める。不明な位置は欠測のままにする。収集元、パーサーのバージョン、無害化した文面、参照は
// 呼び出し側が足すため、返す値は単独では ImportFailure.Validate を通らない。
func failureAt(stage core.FailureStage, line, offset int64, expected, observed string) *core.ImportFailure {
	failure := &core.ImportFailure{
		DiagnosisClass:  core.DiagnosisClassUndetermined,
		Stage:           stage,
		ExpectedMeaning: expected,
		ObservedResult:  observed,
	}
	if line >= 1 {
		failure.LineNumber = &line
		if offset >= 0 {
			failure.ByteOffset = &offset
		}
	}
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "source bytes read up to LF, CR LF, or EOF, without lexical interpretation"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	case core.FailureStageTokenize:
		failure.Interpretation = "Squid items separated by the literals of the logformat, with Squid quoted escapes"
		failure.UnresolvedReason = "the lexical structure alone does not distinguish unsupported format, inconsistent input, and tokenizer defects"
	case core.FailureStageNormalize:
		failure.Interpretation = "Squid time interpreted by the logformat time code, or request target authority derived from decoded request text"
		failure.UnresolvedReason = "the value interpretation alone does not distinguish unsupported format, inconsistent input, and normalization defects"
	}
	return failure
}

func (p tokenizeProblem) failure(record Record) *core.ImportFailure {
	return failureAt(core.FailureStageTokenize, record.lineNumber, record.byteOffset+int64(p.offset),
		p.expected, observedAt(record.rawText, p.offset, record.byteOffset))
}

func observedAt(raw string, position int, base int64) string {
	offset := base + int64(position)
	if position >= len(raw) {
		return fmt.Sprintf("the end of the record at byte offset %d", offset)
	}
	return fmt.Sprintf("the byte 0x%02x at byte offset %d", raw[position], offset)
}

func semanticFailure(record Record, name ItemName, expected string, cause error) *core.ImportFailure {
	offset := record.byteOffset
	observed := fmt.Sprintf("the %s item is absent", name)
	if item, ok := record.Item(name); ok {
		offset += item.byteOffset
		observed = observedAt(item.RawValue(), 0, offset)
	}
	if cause != nil {
		observed = cause.Error()
	}
	return failureAt(core.FailureStageNormalize, record.lineNumber, offset, expected, observed)
}
