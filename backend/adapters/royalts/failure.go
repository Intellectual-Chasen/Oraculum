package royalts

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// failureAt はこの層で確定できる項目を埋める。収集元、パーサーのバージョン、無害化した文面、参照は呼び出し
// 側が足すため、返す値は単独では ImportFailure.Validate を通らない。
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
		failure.Interpretation = "the whole Royal TS document read into memory before any element was interpreted"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	case core.FailureStageTokenize:
		failure.Interpretation = "the Royal TS document parsed as well-formed XML, scanning for RoyalRDSConnection elements"
		failure.UnresolvedReason = "the lexical structure alone does not distinguish unsupported format, inconsistent input, and parser defects"
	}
	return failure
}
