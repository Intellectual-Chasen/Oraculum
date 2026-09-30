package prefetch

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// failureAt はこの層で確定できる項目を埋める。収集元、パーサーのバージョン、無害化した文面、参照は呼び出し
// 側が足すため、返す値は単独では ImportFailure.Validate を通らない。
//
// 位置は file の先頭である。圧縮した file では、展開した後の位置を原資料の byte へ戻せない。
func failureAt(stage core.FailureStage, expected, observed string) *core.ImportFailure {
	offset := int64(0)
	failure := &core.ImportFailure{
		DiagnosisClass:  core.DiagnosisClassUndetermined,
		Stage:           stage,
		ExpectedMeaning: expected,
		ObservedResult:  observed,
		ByteOffset:      &offset,
	}
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "the whole Prefetch file read into memory before any field was interpreted"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	default:
		failure.Interpretation = "a Prefetch file of format 17, 23, 26, 30 or 31, expanded with LZXpress Huffman when it starts with MAM"
		failure.UnresolvedReason = "the bytes alone do not distinguish a damaged copy, an unsupported variant, and parser defects"
	}
	return failure
}
