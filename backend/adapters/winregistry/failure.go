package winregistry

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// failureAt はこの層で確定できる項目を埋める。収集元、パーサーのバージョン、無害化した文面、参照は呼び出し
// 側が足すため、返す値は単独では ImportFailure.Validate を通らない。
//
// offset は主 file と log を連結した byte 列の中の位置である。
func failureAt(stage core.FailureStage, offset int64, expected, observed string) *core.ImportFailure {
	failure := &core.ImportFailure{
		DiagnosisClass:  core.DiagnosisClassUndetermined,
		Stage:           stage,
		ExpectedMeaning: expected,
		ObservedResult:  observed,
		ByteOffset:      &offset,
	}
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "the primary file and its transaction logs read into memory before any cell was interpreted"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	default:
		failure.Interpretation = "a regf hive walked from the root key, after applying HvLE transaction log entries when the primary file is dirty"
		failure.UnresolvedReason = "the bytes alone do not distinguish a damaged copy, an unsupported variant, and parser defects"
	}
	return failure
}
