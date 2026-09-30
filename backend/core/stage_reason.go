package core

// StageFailureReason は段階が失敗した理由の種別である。
type StageFailureReason string

// StageFailureReason の値。読み込みの段階と処理の段階は、それぞれ別の値を使う
// (StageFailureReason.IsLoadingReason、StageFailureReason.IsProcessingReason)。
const (
	// StageFailureReasonSourceImportFailed は、収集元 1 件の読み取りか取り込みが失敗した
	// 理由である。失敗した収集元を StageFailure.OriginPath が指す。
	StageFailureReasonSourceImportFailed StageFailureReason = "source_import_failed"
	// StageFailureReasonImportFailed は、収集元を指さない取り込みの失敗である。
	StageFailureReasonImportFailed StageFailureReason = "import_failed"
	// StageFailureReasonRecordingFailed は、読み込んだ収集元を調査へ記録する処理が失敗した
	// 理由である。
	StageFailureReasonRecordingFailed StageFailureReason = "recording_failed"
	// StageFailureReasonGraphBuildFailed は、グラフを組む処理が失敗した理由である。
	StageFailureReasonGraphBuildFailed StageFailureReason = "graph_build_failed"
	// StageFailureReasonInterrupted は、server の停止で段階を打ち切った理由である。
	// 読み込みと処理のどちらの段階も使う。
	StageFailureReasonInterrupted StageFailureReason = "interrupted"
)

// IsLoadingReason は、読み込みの段階が使う理由であるかを返す。
func (r StageFailureReason) IsLoadingReason() bool {
	switch r {
	case StageFailureReasonSourceImportFailed, StageFailureReasonImportFailed,
		StageFailureReasonRecordingFailed, StageFailureReasonInterrupted:
		return true
	default:
		return false
	}
}

// IsProcessingReason は、処理の段階が使う理由であるかを返す。
func (r StageFailureReason) IsProcessingReason() bool {
	switch r {
	case StageFailureReasonGraphBuildFailed, StageFailureReasonInterrupted:
		return true
	default:
		return false
	}
}

// IsKnown は StageFailureReason が定義の中の値であるかを返す。
func (r StageFailureReason) IsKnown() bool { return r.IsLoadingReason() || r.IsProcessingReason() }

// LoadingRejection は、読み込みの要求を退けた理由の種別である。
type LoadingRejection string

// LoadingRejection の値。
const (
	// LoadingRejectionNoBaseDirectory は、server が要求による読み込みの基準の directory を
	// 持たない理由である。
	LoadingRejectionNoBaseDirectory LoadingRejection = "no_base_directory"
	// LoadingRejectionNoSource は、要求が収集元を 1 件も持たない理由である。
	LoadingRejectionNoSource LoadingRejection = "no_source"
	// LoadingRejectionPartialCase は、一部の収集元だけが案件を持つ理由である。
	LoadingRejectionPartialCase LoadingRejection = "partial_case"
	// LoadingRejectionCaseInvalid は、収集元の案件の文字列が案件の識別子の条件に合わない理由である。
	LoadingRejectionCaseInvalid LoadingRejection = "case_invalid"
	// LoadingRejectionPathOutsideBase は、path、または path が辿る symbolic link が、基準の
	// directory から上へ出ずに辿れる相対 path の条件に合わない理由である。途中で基準の外へ出て
	// から戻る symbolic link と、絶対 path の symbolic link は、指す先が基準の中でもこの理由になる。
	LoadingRejectionPathOutsideBase LoadingRejection = "path_outside_base"
	// LoadingRejectionPathControlCharacter は、path が制御文字を持つ理由である。
	LoadingRejectionPathControlCharacter LoadingRejection = "path_control_character"
	// LoadingRejectionFormatUnknown は、入力形式を読む parser が無い理由である。
	LoadingRejectionFormatUnknown LoadingRejection = "format_unknown"
	// LoadingRejectionFormatSpecInvalid は、欄の並びの指定を入力形式が受け付けない理由である。
	LoadingRejectionFormatSpecInvalid LoadingRejection = "format_spec_invalid"
	// LoadingRejectionTerminalInvalid は、収集元を記録した端末の指定が値の条件に合わない理由である。
	LoadingRejectionTerminalInvalid LoadingRejection = "terminal_invalid"
	// LoadingRejectionSourceRepeated は、同じ path と入力形式の収集元を 2 回指した理由である。
	LoadingRejectionSourceRepeated LoadingRejection = "source_repeated"
	// LoadingRejectionFileAbsent は、path の file が基準の directory の下に無い理由である。
	LoadingRejectionFileAbsent LoadingRejection = "file_absent"
	// LoadingRejectionFileUnreadable は、path の file を読む権限が無いか、開けない理由である。
	LoadingRejectionFileUnreadable LoadingRejection = "file_unreadable"
	// LoadingRejectionFileNotRegular は、path が通常の file を指さない理由である。
	LoadingRejectionFileNotRegular LoadingRejection = "file_not_regular"
	// LoadingRejectionNotDirectory は、一覧にする path が directory を指さない理由である。
	LoadingRejectionNotDirectory LoadingRejection = "not_directory"
)

// LoadingRejections は LoadingRejection の値の一覧を返す。
func LoadingRejections() []LoadingRejection {
	return []LoadingRejection{
		LoadingRejectionNoBaseDirectory, LoadingRejectionNoSource, LoadingRejectionPartialCase,
		LoadingRejectionCaseInvalid, LoadingRejectionPathOutsideBase, LoadingRejectionPathControlCharacter,
		LoadingRejectionFormatUnknown, LoadingRejectionFormatSpecInvalid,
		LoadingRejectionTerminalInvalid, LoadingRejectionSourceRepeated,
		LoadingRejectionFileAbsent, LoadingRejectionFileUnreadable, LoadingRejectionFileNotRegular,
		LoadingRejectionNotDirectory,
	}
}

// IsKnown は LoadingRejection が定義の中の値であるかを返す。
func (r LoadingRejection) IsKnown() bool {
	for _, known := range LoadingRejections() {
		if r == known {
			return true
		}
	}
	return false
}
