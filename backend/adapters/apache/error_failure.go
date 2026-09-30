package apache

import (
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func (p errorTokenizeProblem) failure(record ErrorRecord) *core.ImportFailure {
	return failureAt(core.FailureStageTokenize, FormatKeyError, record.lineNumber,
		record.byteOffset+int64(p.offset), p.expected, observedAt(record.rawText, p.offset, record.byteOffset))
}

func errorSemanticFailure(record ErrorRecord, name ErrorItemName, expected string, cause error) *core.ImportFailure {
	offset := record.byteOffset
	observed := fmt.Sprintf("the %s item is absent", name)
	if item, ok := record.Item(name); ok {
		offset += item.byteOffset
		observed = observedAt(item.RawValue(), 0, offset)
	}
	if cause != nil {
		observed = cause.Error()
	}
	return failureAt(core.FailureStageNormalize, FormatKeyError, record.lineNumber, offset, expected, observed)
}
