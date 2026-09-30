package winevent

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// failureAt はこの層で確定できる項目を埋める。収集元、パーサーのバージョン、無害化した文面、参照は呼び出し
// 側が足すため、返す値は単独では ImportFailure.Validate を通らない。
func failureAt(stage core.FailureStage, source Source, expected, observed string) *core.ImportFailure {
	failure := &core.ImportFailure{
		DiagnosisClass:  core.DiagnosisClassUndetermined,
		Stage:           stage,
		ExpectedMeaning: expected,
		ObservedResult:  observed,
	}
	if source.LineNumber >= 1 {
		line := source.LineNumber
		failure.LineNumber = &line
	}
	// 行を持たない EVTX の 1 件は、行番号を持たず byte 範囲だけを持つ。
	if source.LineNumber >= 1 || source.ByteLength >= 1 {
		offset := source.ByteOffset
		failure.ByteOffset = &offset
	}
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "the whole Windows event XML file read into memory before any element was interpreted"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	case core.FailureStageTokenize:
		failure.Interpretation = "UTF-8 bytes split at <Event and </Event> tags, each element parsed as XML on its own"
		failure.UnresolvedReason = "the lexical structure alone does not distinguish a truncated export, inconsistent input, and parser defects"
	case core.FailureStageNormalize:
		failure.Interpretation = "TimeCreated@SystemTime read as an ISO 8601 date and time with an optional UTC offset, " +
			"or as the Event Viewer's local date and time separated by /"
		failure.UnresolvedReason = "the value alone does not distinguish an unsupported writer, inconsistent input, and parser defects"
	}
	return failure
}
