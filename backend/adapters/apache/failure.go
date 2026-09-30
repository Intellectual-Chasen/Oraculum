package apache

import (
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// failureAt はこの層で確定できる項目を埋める。不明な位置は欠測のままにする。収集元、パーサーのバージョン、
// 無害化した文面、参照は呼び出し側が足すため、返す値は単独では ImportFailure.Validate を
// 通らない。
func failureAt(stage core.FailureStage, formatKey core.FormatKey, line, offset int64, expected, observed string) *core.ImportFailure {
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
		failure.Interpretation = interpretationOf(formatKey)
		failure.UnresolvedReason = "the lexical structure alone does not distinguish unsupported format, inconsistent input, and tokenizer defects"
	case core.FailureStageNormalize:
		failure.Interpretation = "Apache HTTP Server field values interpreted at the precision and offset state documented for " + string(formatKey)
		failure.UnresolvedReason = "the value interpretation alone does not distinguish unsupported format, inconsistent input, and normalization defects"
	}
	return failure
}

func interpretationOf(formatKey core.FormatKey) string {
	if formatKey == FormatKeyError {
		return "Apache HTTP Server error log items separated by brackets, with a trailing free-text message"
	}
	return "Apache HTTP Server combined access log items separated by spaces outside brackets and quotes"
}

func observedAt(raw string, position int, base int64) string {
	offset := base + int64(position)
	if position >= len(raw) {
		return fmt.Sprintf("the end of the record at byte offset %d", offset)
	}
	return fmt.Sprintf("the byte 0x%02x at byte offset %d", raw[position], offset)
}
