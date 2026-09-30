package pipeline

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 復号した byte 列と ZIP の読み方の上限。
const (
	leadingBytesLength = 16
	// 既知の制限: ZIP の名前の一覧と CRC を確かめる entry を 1000 件、展開の合計を 64 MiB で打ち切る,
	// 上限は展開の爆弾を止める値であり、正しい入力の大きさから決めていない, 上限に達する入力を
	// 確認したときに見直す
	maxZipEntries       = 1000
	maxZipInflatedBytes = 64 << 20
	// 断片の番号を読む bit 幅。番号の数え方が int64 の範囲を越えないようにする。
	fragmentNumberBits = 32
	zipContentType     = "application/zip"
	// zipEncryptedFlag は ZIP の汎用 flag のうち、entry を暗号化したことを示す bit である。
	zipEncryptedFlag = 0x1
)

// numberedFragment は、URL の path が `/<番号>/<文字列>` の形をした根拠のレコード 1 件である。
type numberedFragment struct {
	number  int64
	token   string
	locator core.RecordLocator
	// status は HTTP の状態の文字列である。状態の欄を読めないレコードでは空である。
	status string
	// truncated は URL の文字列が途中で切れているかである。
	truncated bool
}

// joinable は、行の文字列をつなげるかを返す。HTTP の状態が成功 (2xx) か状態を読めない行で、
// URL が途中で切れていない行である。**成功でない応答の要求を、送った断片の根拠にしない。**
// 拒まれた要求の文字列は、届いた断片と同じとは限らない。
func (f numberedFragment) joinable() bool {
	return !f.truncated && (f.status == "" || len(f.status) == 3 && f.status[0] == '2')
}

// UrlFragmentJoin は、関係 1 本の根拠のレコードが URL の path に書いた番号付きの断片を、
// 送信の回ごとに番号の順でつなぎ、復号した結果を返す。found が偽になるのは関係が無いときである。
//
// 根拠は filter の条件を通したものだけを読む (evidenceInGroup)。案件で絞ると、別の案件の
// 収集元の行をつながない。
//
// 既知の制限: 送信の回を、根拠の並び (取り込みの順) で番号が直前の行より小さくなった点と、
// 収集元が替わった点だけで区切る, 同じ番号の再送が隣り合う行に出ることを前提にする, 同じ番号の
// 再送が離れた位置に出る入力を確認したときに見直す
//
// 既知の制限: 番号を持たない要求はつながず件数だけを返す, 時刻の順は送信の順を保証せず、
// つないだ byte 列が壊れる, 順序を示す欄を持つ形式を扱うときに見直す
func (g Graph) UrlFragmentJoin(edgeId string, filter EdgeEvidenceFilter) (core.UrlFragmentJoin, bool) {
	at, found := g.edgeIndexOf(edgeId)
	if !found {
		return core.UrlFragmentJoin{}, false
	}
	join := core.UrlFragmentJoin{EdgeId: edgeId, Segments: []core.UrlFragmentSegment{}}
	var run []numberedFragment
	for _, position := range g.evidenceInGroup(g.edges[at].evidence, filter) {
		record := g.records[position]
		fragment, numbered := numberedFragmentOf(record.requestUrl)
		if !numbered {
			join.UnnumberedRecordCount++
			continue
		}
		if len(run) > 0 {
			last := run[len(run)-1]
			if fragment.number < last.number || record.locator.SourceId != last.locator.SourceId {
				join.Segments = append(join.Segments, joinedSegment(run))
				run = nil
			}
		}
		fragment.locator = record.locator
		if record.httpStatus != nil && record.httpStatus.Text != nil {
			fragment.status = httpStatusOf(*record.httpStatus.Text)
		}
		run = append(run, fragment)
	}
	if len(run) > 0 {
		join.Segments = append(join.Segments, joinedSegment(run))
	}
	return join, true
}

// httpStatusOf は状態の欄の文字列を返す。欄を持ち値を読めない行 (値の不在の文字列など) は、
// 原資料の文字列か `-` を返し、つなげる行にしない (numberedFragment.joinable)。
func httpStatusOf(value core.RawAndNormalized) string {
	if status, readable := value.ComparableValue(); readable {
		return status
	}
	if raw, found := value.RawTextValue(); found && raw != "" {
		return raw
	}
	return "-"
}

// withoutPartialEscape は、途中で切れた文字列の末尾に残る不完全な百分率符号化 (`%` か `%X`) を
// 外す。url.Parse が不完全な符号化を退け、番号を読めなくなるのを防ぐ。
func withoutPartialEscape(raw string) string {
	if at := strings.LastIndexByte(raw, '%'); at >= 0 && len(raw)-at < 3 {
		return raw[:at]
	}
	return raw
}

// numberedFragmentOf は URL の項目から、path の最初の要素の番号と、残りの文字列を読む。
// 番号は 10 進の数字だけの文字列で、32 bit に収まる値に限る。文字列は `/` を含んでよい。
// 要求先の絶対形式 (Squid) と origin 形式 (Apache) のどちらも読む。途中で切れた URL は、
// 番号の後ろの `/` まで残っていれば番号を読み、文字列が空でもよい。
func numberedFragmentOf(field *core.RecordField) (numberedFragment, bool) {
	if field == nil || field.Text == nil {
		return numberedFragment{}, false
	}
	truncated := field.Text.ValueState == core.ValueStateTruncated
	raw, readable := field.Text.ComparableValue()
	if truncated {
		raw, readable = field.Text.RawTextValue()
	}
	if !readable {
		return numberedFragment{}, false
	}
	if truncated {
		raw = withoutPartialEscape(raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return numberedFragment{}, false
	}
	digits, token, cut := strings.Cut(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if !cut || token == "" && !truncated {
		return numberedFragment{}, false
	}
	number, err := strconv.ParseUint(digits, 10, fragmentNumberBits)
	if err != nil {
		return numberedFragment{}, false
	}
	return numberedFragment{number: int64(number), token: token, truncated: truncated}, true
}

// joinedSegment は送信の 1 回の断片のうち、同じ番号のつなげる行の最初の 1 行の文字列をつなぐ
// (numberedFragment.joinable)。回は番号が小さくなる点で区切るため、run は番号の順に並んでいる。
//
// 既知の制限: 同じ番号のつなげる行の最初の行を採り、文字列の違う重複が 1 件でもあれば復号しない,
// どちらが正しいかを決める根拠を記録が持たない,
// 文字列の違う重複を持つ入力で、採る規則が要るときに見直す
func joinedSegment(sorted []numberedFragment) core.UrlFragmentSegment {
	segment := core.UrlFragmentSegment{
		FragmentCount: int64(len(sorted)),
		LastNumber:    sorted[len(sorted)-1].number,
		Fragments:     make([]core.UrlFragment, 0, len(sorted)),
	}
	var joined strings.Builder
	var adoptedToken string
	adoptedNumbers := int64(0)
	for index, fragment := range sorted {
		firstOfNumber := index == 0 || fragment.number != sorted[index-1].number
		if firstOfNumber {
			adoptedToken = ""
		} else {
			segment.DuplicateCount++
		}
		adopted := false
		switch {
		case !fragment.joinable():
		case adoptedToken == "":
			adopted = true
			adoptedNumbers++
			adoptedToken = fragment.token
			joined.WriteString(fragment.token)
		case fragment.token != adoptedToken:
			segment.ConflictingDuplicateCount++
		}
		segment.Fragments = append(segment.Fragments, core.UrlFragment{
			Number: fragment.number, Adopted: adopted, HttpStatusCode: fragment.status,
			Truncated: fragment.truncated, RecordRef: cloneLocator(fragment.locator),
		})
	}
	segment.MissingNumberCount = segment.LastNumber + 1 - adoptedNumbers
	switch {
	case segment.MissingNumberCount > 0:
		segment.DecodeFailure = core.UrlFragmentDecodeFailureMissingNumbers
	case segment.ConflictingDuplicateCount > 0:
		segment.DecodeFailure = core.UrlFragmentDecodeFailureConflictingDuplicates
	default:
		segment.Encoding, segment.Decoded, segment.DecodeFailure = decodeJoinedFragments(joined.String())
	}
	return segment
}

// decodeJoinedFragments は、つないだ文字列を字母を判定して復号する。
func decodeJoinedFragments(joined string) (
	core.UrlFragmentEncoding, *core.DecodedBytes, core.UrlFragmentDecodeFailure,
) {
	unpadded := strings.TrimRight(joined, "=")
	encoding, determined := fragmentAlphabetOf(unpadded)
	if !determined {
		return "", nil, core.UrlFragmentDecodeFailureAlphabetUndetermined
	}
	codec := base64.RawURLEncoding
	if encoding == core.UrlFragmentEncodingBase64 {
		codec = base64.RawStdEncoding
	}
	data, err := codec.DecodeString(unpadded)
	if err != nil {
		return encoding, nil, core.UrlFragmentDecodeFailureInvalidEncoding
	}
	decoded := describeDecodedBytes(data)
	return encoding, &decoded, ""
}

// fragmentAlphabetOf は、末尾の `=` を外した文字列が使う Base64 の字母を返す。
// 英数字の他に `-` と `_` だけを使えば base64url、`+` と `/` だけを使えば base64 である。
// 両方の組を使う文字列と、他の文字 (途中の `=` を含む) を持つ文字列は判定しない。
//
// 既知の制限: 字母を区別する文字を持たない文字列は base64url として読む, 両方の字母で復号の結果が
// 同じになるため測る対象が無い, 字母の外の変種 (例: base32) を扱うときに見直す
func fragmentAlphabetOf(unpadded string) (core.UrlFragmentEncoding, bool) {
	urlSafe, standard := false, false
	for _, character := range unpadded {
		switch {
		case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z',
			character >= '0' && character <= '9':
		case character == '-' || character == '_':
			urlSafe = true
		case character == '+' || character == '/':
			standard = true
		default:
			return "", false
		}
	}
	if urlSafe && standard {
		return "", false
	}
	if standard {
		return core.UrlFragmentEncodingBase64, true
	}
	return core.UrlFragmentEncodingBase64Url, true
}

// describeDecodedBytes は復号した byte 列の大きさと SHA-256 と形式を返す。ZIP なら中を読む。
//
// 既知の制限: 復号した byte 列を返さず、SHA-256 と形式と ZIP の一覧だけを返す, 持ち出された
// 内容を画面の origin から配る経路を作らないため測る対象が無い, 中身の解析を Oraculum の中で
// 行う要求が出たときに見直す
func describeDecodedBytes(data []byte) core.DecodedBytes {
	sum := sha256.Sum256(data)
	decoded := core.DecodedBytes{
		ByteCount:       int64(len(data)),
		Sha256:          hex.EncodeToString(sum[:]),
		LeadingBytesHex: hex.EncodeToString(data[:min(len(data), leadingBytesLength)]),
		ContentType:     http.DetectContentType(data),
	}
	if decoded.ContentType == zipContentType {
		listing := zipListingOf(data)
		decoded.Zip = &listing
	}
	return decoded
}

// zipListingOf は ZIP の中央ディレクトリから entry の名前を読み、entry を展開して CRC を確かめる。
func zipListingOf(data []byte) core.ZipListing {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	// ErrInsecurePath は、読めた一覧に安全でない名前があることを示す。一覧は使える。
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return core.ZipListing{Entries: []core.ZipEntryName{}}
	}
	listing := core.ZipListing{
		Readable:   true,
		EntryCount: int64(len(reader.File)),
		Integrity:  zipIntegrityOf(reader.File),
		Entries:    make([]core.ZipEntryName, 0, min(len(reader.File), maxZipEntries)),
	}
	for _, file := range reader.File[:min(len(reader.File), maxZipEntries)] {
		entry := core.ZipEntryName{Name: file.Name}
		if !utf8.ValidString(file.Name) {
			entry.Name = strings.ToValidUTF8(file.Name, string(utf8.RuneError))
			entry.NameHex = hex.EncodeToString([]byte(file.Name))
		}
		listing.Entries = append(listing.Entries, entry)
	}
	return listing
}

// zipIntegrityOf は entry を順に展開し、archive/zip が EOF で確かめる CRC の結果をまとめる。
// 展開した byte を実際に数えて上限と比べ、entry の見出しが書いた大きさを信じない。
func zipIntegrityOf(files []*zip.File) core.ZipIntegrity {
	if len(files) > maxZipEntries {
		return core.ZipIntegrityNotChecked
	}
	remaining := int64(maxZipInflatedBytes)
	for _, file := range files {
		if file.Flags&zipEncryptedFlag != 0 {
			return core.ZipIntegrityUnsupported
		}
		content, err := file.Open()
		if errors.Is(err, zip.ErrAlgorithm) {
			return core.ZipIntegrityUnsupported
		}
		if err != nil {
			return core.ZipIntegrityFailed
		}
		copied, err := io.Copy(io.Discard, io.LimitReader(content, remaining+1))
		_ = content.Close()
		if copied > remaining {
			return core.ZipIntegrityNotChecked
		}
		if err != nil {
			return core.ZipIntegrityFailed
		}
		remaining -= copied
	}
	return core.ZipIntegrityPassed
}
