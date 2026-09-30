package core

// SourceUploadResult は保存した資料と、その資料に関係する付属資料の形式判定を返す。
// directory 全体の一覧ではない。
type SourceUploadResult struct {
	Entries []SourceFileEntry `json:"entries"`
}

// Validate は保存した資料の項目を検査する。
func (r SourceUploadResult) Validate() error {
	for _, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SourceFileKind は、基準の directory の下の項目の種類である。
type SourceFileKind string

// SourceFileKind の値。
const (
	// SourceFileKindDirectory は directory である。
	SourceFileKindDirectory SourceFileKind = "directory"
	// SourceFileKindFile は通常の file である。
	SourceFileKindFile SourceFileKind = "file"
	// SourceFileKindOther は、選べない項目である。directory でも通常の file でもない項目 (device file、
	// 基準の外を指す symbolic link など)、読み込みの要求に渡せない名前の項目、再帰の一覧で辿らない
	// directory (symbolic link の directory、中を読めない directory) を含む。
	SourceFileKindOther SourceFileKind = "other"
)

// IsKnown は SourceFileKind が定義の中の値であるかを返す。
func (k SourceFileKind) IsKnown() bool {
	switch k {
	case SourceFileKindDirectory, SourceFileKindFile, SourceFileKindOther:
		return true
	default:
		return false
	}
}

// SourceFileUndetectedReason は、file の入力形式の候補が無い理由である。
type SourceFileUndetectedReason string

// SourceFileUndetectedReason の値。
const (
	// SourceFileUndetectedReasonUnsupportedFormat は、file の先頭の byte 列を、読める入力形式の
	// どれも読めない理由である。
	SourceFileUndetectedReasonUnsupportedFormat SourceFileUndetectedReason = "unsupported_format"
	// SourceFileUndetectedReasonEmptyFile は、file が 0 byte である理由である。
	SourceFileUndetectedReasonEmptyFile SourceFileUndetectedReason = "empty_file"
	// SourceFileUndetectedReasonUnreadable は、file を開けないか読めない理由である。
	SourceFileUndetectedReasonUnreadable SourceFileUndetectedReason = "unreadable"
	// SourceFileUndetectedReasonCompanionFile は、file が同じ directory の主 file と一緒に読む
	// 付属の file (registry の transaction log) である理由である。主 file を選ぶと読み、単独では
	// 読まない。
	SourceFileUndetectedReasonCompanionFile SourceFileUndetectedReason = "companion_file"
)

// IsKnown は SourceFileUndetectedReason が定義の中の値であるかを返す。
func (r SourceFileUndetectedReason) IsKnown() bool {
	switch r {
	case SourceFileUndetectedReasonUnsupportedFormat, SourceFileUndetectedReasonEmptyFile,
		SourceFileUndetectedReasonUnreadable, SourceFileUndetectedReasonCompanionFile:
		return true
	default:
		return false
	}
}

// SourceFileListing は、基準の directory の下の directory 1 つの項目の一覧である。
// **本型が項目の定義元である。**
type SourceFileListing struct {
	// Path は一覧にした directory の、基準の directory からの相対 path である。基準の
	// directory そのものは `.` である。
	Path string `json:"path"`
	// Recursive は、下の directory の file も一覧に含めたかである。真の一覧は file だけを持つ。
	Recursive bool `json:"recursive"`
	// Entries は項目を path の辞書順に並べたものである。偽の Recursive の一覧は directory を先に
	// 並べる。
	Entries []SourceFileEntry `json:"entries"`
	// Truncated は、項目の数が上限に達し、残りの項目を含めなかったかである。
	Truncated bool `json:"truncated"`
}

// SourceFileEntry は、基準の directory の下の項目 1 つである。
type SourceFileEntry struct {
	// OriginPath は基準の directory からの相対 path である。読み込みの要求の originPath に
	// そのまま渡せる。
	OriginPath string `json:"originPath"`
	// Name は項目の名前である。
	Name string `json:"name"`
	// Kind は項目の種類である。
	Kind SourceFileKind `json:"kind"`
	// SizeBytes は file の byte 数である。file だけが持つ。
	SizeBytes *int64 `json:"sizeBytes,omitempty"`
	// FormatCandidates は、file を読める入力形式の候補を識別子の順に並べたものである。file
	// だけが持ち、候補が無い file では要素数 0 である。
	FormatCandidates []FormatKey `json:"formatCandidates,omitempty"`
	// Undetected は、file の候補が無い理由である。候補を持つ file と、file でない項目は持たない。
	Undetected *SourceFileUndetected `json:"undetected,omitempty"`
}

// SourceFileUndetected は、file の入力形式の候補が無い理由である。
type SourceFileUndetected struct {
	// Reason は理由の種別である。
	Reason SourceFileUndetectedReason `json:"reason"`
	// DetectedKind は、先頭の byte 列から分かった file の種類である (SkippedFile.DetectedKind)。
	// 分からない file では出ない。
	DetectedKind string `json:"detectedKind,omitempty"`
}

// Validate は項目の整合を確かめる。
func (l SourceFileListing) Validate() error {
	if err := firstProblem(
		requirePresent("SourceFileListing.path", l.Path),
		requireRelativePath("SourceFileListing.path", l.Path),
		requireSanitized("SourceFileListing.path", l.Path),
	); err != nil {
		return err
	}
	for index, entry := range l.Entries {
		if err := entry.Validate(); err != nil {
			return itemError("SourceFileListing.entries at "+formatIndex(index), err)
		}
		if l.Recursive && entry.Kind == SourceFileKindDirectory {
			return itemError("SourceFileListing.entries at "+formatIndex(index)+" is a directory of a recursive listing",
				ErrInconsistentValue)
		}
	}
	return nil
}

// Validate は項目の整合を確かめる。
func (e SourceFileEntry) Validate() error {
	if err := firstProblem(
		requirePresent("SourceFileEntry.originPath", e.OriginPath),
		requireRelativePath("SourceFileEntry.originPath", e.OriginPath),
		requireSanitized("SourceFileEntry.originPath", e.OriginPath),
		requirePresent("SourceFileEntry.name", e.Name),
		requireKnownEnum("SourceFileEntry.kind", e.Kind),
	); err != nil {
		return err
	}
	if e.Kind != SourceFileKindFile {
		if e.SizeBytes != nil || len(e.FormatCandidates) != 0 || e.Undetected != nil {
			return itemError("SourceFileEntry of a non-file carries file items", ErrUnexpectedItem)
		}
		return nil
	}
	if e.SizeBytes == nil {
		return itemError("SourceFileEntry.sizeBytes", ErrMissingRequiredItem)
	}
	if err := requireNonNegative("SourceFileEntry.sizeBytes", *e.SizeBytes); err != nil {
		return err
	}
	if (len(e.FormatCandidates) == 0) == (e.Undetected == nil) {
		return itemError("SourceFileEntry carries exactly one of formatCandidates and undetected",
			ErrInconsistentValue)
	}
	if e.Undetected != nil {
		return requireKnownEnum("SourceFileEntry.undetected.reason", e.Undetected.Reason)
	}
	for _, key := range e.FormatCandidates {
		if err := requirePresent("SourceFileEntry.formatCandidates[]", string(key)); err != nil {
			return err
		}
	}
	return nil
}
