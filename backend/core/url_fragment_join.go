package core

// UrlFragmentEncoding は、つないだ断片の文字列を読んだ符号化の字母である。
type UrlFragmentEncoding string

// UrlFragmentEncoding の値。
const (
	// UrlFragmentEncodingBase64Url は `-` と `_` を使う Base64 の字母である (RFC 4648 の 5 節)。
	UrlFragmentEncodingBase64Url UrlFragmentEncoding = "base64url"
	// UrlFragmentEncodingBase64 は `+` と `/` を使う Base64 の字母である (RFC 4648 の 4 節)。
	UrlFragmentEncodingBase64 UrlFragmentEncoding = "base64"
)

// IsKnown は UrlFragmentEncoding が定義の中の値であるかを返す。
func (e UrlFragmentEncoding) IsKnown() bool {
	return e == UrlFragmentEncodingBase64Url || e == UrlFragmentEncodingBase64
}

// UrlFragmentDecodeFailure は、つないだ断片を復号しなかった、または復号できなかった理由である。
type UrlFragmentDecodeFailure string

// UrlFragmentDecodeFailure の値。
const (
	// UrlFragmentDecodeFailureMissingNumbers は、0 から最後の番号までに欠けた番号がある状態である。
	UrlFragmentDecodeFailureMissingNumbers UrlFragmentDecodeFailure = "missing_numbers"
	// UrlFragmentDecodeFailureConflictingDuplicates は、同じ番号に違う文字列の行がある状態である。
	UrlFragmentDecodeFailureConflictingDuplicates UrlFragmentDecodeFailure = "conflicting_duplicates"
	// UrlFragmentDecodeFailureAlphabetUndetermined は、文字列がどちらの字母にも該当しない状態である。
	UrlFragmentDecodeFailureAlphabetUndetermined UrlFragmentDecodeFailure = "alphabet_undetermined"
	// UrlFragmentDecodeFailureInvalidEncoding は、判定した字母で復号できなかった状態である。
	UrlFragmentDecodeFailureInvalidEncoding UrlFragmentDecodeFailure = "invalid_encoding"
)

// IsKnown は UrlFragmentDecodeFailure が定義の中の値であるかを返す。
func (f UrlFragmentDecodeFailure) IsKnown() bool {
	switch f {
	case UrlFragmentDecodeFailureMissingNumbers, UrlFragmentDecodeFailureConflictingDuplicates,
		UrlFragmentDecodeFailureAlphabetUndetermined, UrlFragmentDecodeFailureInvalidEncoding:
		return true
	default:
		return false
	}
}

// ZipIntegrity は、ZIP の entry を展開して CRC を確かめた結果である。
type ZipIntegrity string

// ZipIntegrity の値。
const (
	// ZipIntegrityPassed は、すべての entry を展開し、CRC が一致した状態である。
	ZipIntegrityPassed ZipIntegrity = "passed"
	// ZipIntegrityFailed は、展開または CRC の確かめに失敗した entry がある状態である。
	ZipIntegrityFailed ZipIntegrity = "failed"
	// ZipIntegrityNotChecked は、展開の上限に達して確かめを止めた状態である。
	ZipIntegrityNotChecked ZipIntegrity = "not_checked"
	// ZipIntegrityUnsupported は、暗号化した entry か、展開できない圧縮の方式の entry がある状態である。
	ZipIntegrityUnsupported ZipIntegrity = "unsupported"
)

// IsKnown は ZipIntegrity が定義の中の値であるかを返す。
func (z ZipIntegrity) IsKnown() bool {
	switch z {
	case ZipIntegrityPassed, ZipIntegrityFailed, ZipIntegrityNotChecked, ZipIntegrityUnsupported:
		return true
	default:
		return false
	}
}

// UrlFragmentJoin は、関係 1 本の根拠のレコードが URL の path に書いた番号付きの断片を、
// 送信の回ごとに番号の順でつないだ結果である。
type UrlFragmentJoin struct {
	// EdgeId は要求が与えた関係の識別子をそのまま返す。
	EdgeId string `json:"edgeId"`
	// UnnumberedRecordCount は、URL を記録していないか、URL の path が `/<番号>/<文字列>` の形で
	// ない根拠のレコードの件数である。これらの行はつながない。
	UnnumberedRecordCount int64 `json:"unnumberedRecordCount"`
	// Segments は送信の回である。並びは根拠の並びで回が始まった順である。
	Segments []UrlFragmentSegment `json:"segments"`
}

// UrlFragmentSegment は送信の 1 回である。
type UrlFragmentSegment struct {
	// FragmentCount は回に入った行の数である。同じ番号の再送も 1 行に数える。
	FragmentCount int64 `json:"fragmentCount"`
	// LastNumber は回の中で最も大きい番号である。
	LastNumber int64 `json:"lastNumber"`
	// DuplicateCount は、同じ番号の 2 行目以降の行の数である。
	DuplicateCount int64 `json:"duplicateCount"`
	// ConflictingDuplicateCount は、DuplicateCount のうち、つなげる行で、採った行と文字列が違う
	// 行の数である。
	ConflictingDuplicateCount int64 `json:"conflictingDuplicateCount"`
	// MissingNumberCount は、0 から LastNumber までの番号のうち、つなげる行を持たない番号の数
	// である (UrlFragment.Adopted)。状態が成功でない行と途中で切れた行だけを持つ番号も数える。
	MissingNumberCount int64 `json:"missingNumberCount"`
	// Fragments は回に入った行である。並びは番号の順で、同じ番号の中では根拠の並びである。
	Fragments []UrlFragment `json:"fragments"`
	// Encoding は文字列を読んだ字母である。Decoded があるときと、DecodeFailure が
	// invalid_encoding のときだけ出る。
	Encoding UrlFragmentEncoding `json:"encoding,omitempty"`
	// Decoded と DecodeFailure は、ちょうど一方が出る。
	Decoded       *DecodedBytes            `json:"decoded,omitempty"`
	DecodeFailure UrlFragmentDecodeFailure `json:"decodeFailure,omitempty"`
}

// UrlFragment は断片 1 行である。
type UrlFragment struct {
	// Number は URL の path の最初の要素が書いた番号である。
	Number int64 `json:"number"`
	// Adopted は、この行の文字列をつないだかである。同じ番号の行のうち、つなげる行の最初の
	// 1 行だけが真である。つなげる行は、HTTP の状態が成功 (2xx) か状態の欄を持たない行で、
	// URL が途中で切れていない行である。
	Adopted bool `json:"adopted"`
	// HttpStatusCode は行が記録した HTTP の状態の文字列である。状態の欄を持たない行では出ない。
	HttpStatusCode string `json:"httpStatusCode,omitempty"`
	// Truncated は、原資料の行が途中で切れ、URL の文字列が後ろを持たないかである。
	Truncated bool `json:"truncated,omitempty"`
	// RecordRef は断片を記録した原資料のレコードの位置である。
	RecordRef RecordLocator `json:"recordRef"`
}

// DecodedBytes は、つないで復号した byte 列を表す値である。byte 列そのものは持たない。
type DecodedBytes struct {
	// ByteCount は復号した byte 列の長さである。
	ByteCount int64 `json:"byteCount"`
	// Sha256 は byte 列の SHA-256 の小文字の 16 進表現である。
	Sha256 string `json:"sha256"`
	// LeadingBytesHex は先頭の最大 16 byte の小文字の 16 進表現である。
	LeadingBytesHex string `json:"leadingBytesHex"`
	// ContentType は先頭の byte 列から判定した形式である (WHATWG MIME Sniffing)。
	ContentType string `json:"contentType"`
	// Zip は ContentType が application/zip のときだけ出る。
	Zip *ZipListing `json:"zip,omitempty"`
}

// ZipListing は ZIP として読んだ結果である。
type ZipListing struct {
	// Readable は ZIP の中央ディレクトリを読めたかである。偽のときは Integrity と
	// Entries を持たない。
	Readable bool `json:"readable"`
	// Integrity は Readable が真のときだけ出る。
	Integrity ZipIntegrity `json:"integrity,omitempty"`
	// EntryCount は中央ディレクトリが持つ entry の総数である。Entries の上限で打ち切らない。
	EntryCount int64 `json:"entryCount"`
	// Entries は entry の名前である。並びは中央ディレクトリの順で、上限で打ち切ることがある。
	Entries []ZipEntryName `json:"entries"`
}

// ZipEntryName は ZIP の entry 1 つの名前である。
type ZipEntryName struct {
	// Name は entry の名前である。
	Name string `json:"name"`
	// NameHex は名前の原 byte の小文字の 16 進表現である。名前が UTF-8 として読めない
	// ときだけ出る。Name は読めない byte を U+FFFD に置き換えた文字列である。
	NameHex string `json:"nameHex,omitempty"`
}

// Validate は結果の項目の組が契約を満たすことを確かめる。
func (j UrlFragmentJoin) Validate() error {
	if problem := firstProblem(
		requirePresent("UrlFragmentJoin.edgeId", j.EdgeId),
		requireNonNegative("UrlFragmentJoin.unnumberedRecordCount", j.UnnumberedRecordCount),
	); problem != nil {
		return problem
	}
	for index, segment := range j.Segments {
		if err := segment.validate(); err != nil {
			return itemError("UrlFragmentJoin.segments "+formatIndex(index), err)
		}
	}
	return nil
}

func (s UrlFragmentSegment) validate() error {
	if problem := firstProblem(
		requireNonNegative("fragmentCount", s.FragmentCount),
		requireNonNegative("lastNumber", s.LastNumber),
		requireNonNegative("duplicateCount", s.DuplicateCount),
		requireNonNegative("conflictingDuplicateCount", s.ConflictingDuplicateCount),
		requireNonNegative("missingNumberCount", s.MissingNumberCount),
		requireKnownEnumWhenPresent("encoding", s.Encoding),
		requireKnownEnumWhenPresent("decodeFailure", s.DecodeFailure),
	); problem != nil {
		return problem
	}
	if s.FragmentCount == 0 || int64(len(s.Fragments)) != s.FragmentCount ||
		s.DuplicateCount >= s.FragmentCount || s.ConflictingDuplicateCount > s.DuplicateCount ||
		s.MissingNumberCount > s.LastNumber+1 {
		return itemError("fragment counts", ErrInconsistentValue)
	}
	if (s.Decoded == nil) == (s.DecodeFailure == "") {
		return itemError("decoded and decodeFailure", ErrInconsistentValue)
	}
	wantsEncoding := s.Decoded != nil || s.DecodeFailure == UrlFragmentDecodeFailureInvalidEncoding
	if wantsEncoding == (s.Encoding == "") {
		return itemError("encoding", ErrInconsistentValue)
	}
	for index, fragment := range s.Fragments {
		if fragment.Number < 0 || fragment.Number > s.LastNumber {
			return itemError("fragments "+formatIndex(index)+" number", ErrInconsistentValue)
		}
		if err := fragment.RecordRef.Validate(); err != nil {
			return itemError("fragments "+formatIndex(index)+" recordRef", err)
		}
	}
	if s.Decoded != nil {
		return s.Decoded.validate()
	}
	return nil
}

func (d DecodedBytes) validate() error {
	if problem := firstProblem(
		requireNonNegative("decoded.byteCount", d.ByteCount),
		requireLowerHex64("decoded.sha256", d.Sha256),
		requirePresent("decoded.contentType", d.ContentType),
	); problem != nil {
		return problem
	}
	if d.Zip == nil {
		return nil
	}
	zip := *d.Zip
	if problem := firstProblem(
		requireNonNegative("decoded.zip.entryCount", zip.EntryCount),
		requireKnownEnumWhenPresent("decoded.zip.integrity", zip.Integrity),
	); problem != nil {
		return problem
	}
	if zip.Readable == (zip.Integrity == "") || int64(len(zip.Entries)) > zip.EntryCount ||
		(!zip.Readable && (zip.EntryCount != 0 || len(zip.Entries) != 0)) {
		return itemError("decoded.zip", ErrInconsistentValue)
	}
	return nil
}
