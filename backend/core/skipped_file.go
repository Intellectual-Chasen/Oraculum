package core

// SkippedFileReason は、収集の directory の file を取り込まなかった理由の種別である。
type SkippedFileReason string

// SkippedFileReason の値。
const (
	// SkippedFileReasonUnsupportedFormat は、file の先頭の byte 列が、収集の directory から
	// 取り込む入力形式のどれにも当たらない理由である。
	SkippedFileReasonUnsupportedFormat SkippedFileReason = "unsupported_format"
	// SkippedFileReasonEmptyFile は、file が 0 byte である理由である。
	SkippedFileReasonEmptyFile SkippedFileReason = "empty_file"
	// SkippedFileReasonNotRegularFile は、directory の項目が通常の file でない理由である
	// (symbolic link、device file など)。
	SkippedFileReasonNotRegularFile SkippedFileReason = "not_regular_file"
	// SkippedFileReasonCompanionWithoutMain は、file 名が付属の file (registry の transaction log)
	// の形で、一緒に読む主 file が同じ directory に無い理由である。
	SkippedFileReasonCompanionWithoutMain SkippedFileReason = "companion_without_main"
)

// IsKnown は SkippedFileReason が定義の中の値であるかを返す。
func (r SkippedFileReason) IsKnown() bool {
	switch r {
	case SkippedFileReasonUnsupportedFormat, SkippedFileReasonEmptyFile, SkippedFileReasonNotRegularFile,
		SkippedFileReasonCompanionWithoutMain:
		return true
	default:
		return false
	}
}

// SkippedFile は、収集の directory にあり、取り込まなかった file 1 つである。
type SkippedFile struct {
	// OriginPath は file の取得元である。起動の directory からの相対 path。
	OriginPath string `json:"originPath"`
	// Reason は取り込まなかった理由である。
	Reason SkippedFileReason `json:"reason"`
	// DetectedKind は、先頭の byte 列から分かった file の種類である。機械処理用の英語の key。
	// 分からない file では出ない。
	DetectedKind string `json:"detectedKind,omitempty"`
}

// Validate は項目の整合を確かめる。
func (f SkippedFile) Validate() error {
	return firstProblem(
		requirePresent("SkippedFile.originPath", f.OriginPath),
		requireKnownEnum("SkippedFile.reason", f.Reason),
	)
}
