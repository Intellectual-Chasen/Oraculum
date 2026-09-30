package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// FormatKeyWindowsCollection は、Windows の端末 1 台から集めた file を置いた directory を指す
// 計画の入力形式である。ExpandPlans が directory の中の file ごとの計画に置き換える。
//
// **file の入力形式は、利用者が directory を収集として指定したときだけ、先頭の byte 列の署名で
// 決める。** 署名で決められない file は取り込まず、理由とともに一覧にする (core.SkippedFile)。
const FormatKeyWindowsCollection core.FormatKey = "windows_collection"

// SignedParser は、収集の directory の file を先頭の byte 列の署名で振り分けてよい入力形式の
// パーサーが持つ。署名を知っているのは入力形式を読む adapter である。
type SignedParser interface {
	// HasSignature は、file の先頭の byte 列 head がこの入力形式の署名を持つかを返す。head は
	// file の先頭の最大 collectionHeadBytes byte である。
	HasSignature(head []byte) bool
}

// errCollectionWithoutSource は、収集の directory が取り込む file を 1 つも持たない失敗である。
var errCollectionWithoutSource = errors.New("the collection has no file of a supported format")

// collectionHeadBytes は、入力形式の署名と、分かる範囲の種類の署名を比べるために読む file の
// 先頭の byte 数である。最も長い署名は SQLite の 16 byte である。
const collectionHeadBytes = 16

// signedFormat は、署名で振り分ける入力形式 1 つと、その形式が主 file と一緒に読む付属の file の
// 接尾辞である。
type signedFormat struct {
	key      core.FormatKey
	parser   SignedParser
	suffixes []string
}

// signedFormatsOf は、パーサーの表のうち署名で振り分ける入力形式を、識別子の順に返す。
//
// 欄の並びの指定を求める形式は、指定なしでは作れず、署名も持たないため数えない。
func signedFormatsOf(parsers map[core.FormatKey]ParserFactory) []signedFormat {
	var formats []signedFormat
	for _, key := range slices.Sorted(maps.Keys(parsers)) {
		parser, err := parsers[key](nil)
		if err != nil {
			continue
		}
		signed, ok := parser.(SignedParser)
		if !ok {
			continue
		}
		format := signedFormat{key: key, parser: signed}
		if companion, ok := parser.(CompanionFileParser); ok {
			format.suffixes = companion.CompanionSuffixes()
		}
		formats = append(formats, format)
	}
	return formats
}

// collectionFile は、収集の directory で見つけた file 1 つと、その判定である。
type collectionFile struct {
	originPath string
	format     *signedFormat
	skipped    *core.SkippedFile
}

// ExpandCollection は、fsys の中の directory dir を再帰的に辿り、入力形式の署名を持つ file ごとの
// 取り込みの計画と、取り込まない file の一覧を、path の辞書順で返す。署名は parsers のうち
// SignedParser を持つパーサーが判定する。
//
// dir は fsys の root からの相対 path である。返す計画と一覧の OriginPath も fsys の root からの
// 相対 path であり、計画は CollectionPath に dir を持つ。
//
// **付属の file を別の収集元にしない。** 主 file の path に付属の接尾辞を足した file は、主 file の
// 収集元の一部として取り込みの実行が読む (CompanionFileParser)。主 file の無い付属の file は
// 取り込まない。
func ExpandCollection(
	fsys fs.FS, dir string, parsers map[core.FormatKey]ParserFactory,
) ([]SourcePlan, []core.SkippedFile, error) {
	root := path.Clean(filepath.ToSlash(dir))
	if !fs.ValidPath(root) {
		return nil, nil, fmt.Errorf("the collection %q is not a relative path under the base directory", dir)
	}
	formats := signedFormatsOf(parsers)
	var files []collectionFile
	err := fs.WalkDir(fsys, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		file, err := classifyCollectionFile(fsys, name, entry, formats)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading the collection %q: %w", dir, err)
	}
	companions := map[string]bool{}
	for _, file := range files {
		if file.format != nil && !file.hasCompanionName() {
			for _, suffix := range file.format.suffixes {
				companions[file.originPath+suffix] = true
			}
		}
	}
	var plans []SourcePlan
	var skipped []core.SkippedFile
	for _, file := range files {
		switch {
		case companions[file.originPath]:
			continue
		case file.skipped != nil:
			skipped = append(skipped, *file.skipped)
		case file.hasCompanionName():
			skipped = append(skipped, core.SkippedFile{
				OriginPath: file.originPath, Reason: core.SkippedFileReasonCompanionWithoutMain,
				DetectedKind: string(file.format.key),
			})
		default:
			plans = append(plans, SourcePlan{
				OriginPath: file.originPath, FileName: filepath.Base(file.originPath),
				FormatKey: file.format.key, CollectionPath: filepath.FromSlash(root),
			})
		}
	}
	return plans, skipped, nil
}

// hasCompanionName は、署名で形式を決めた file の名前が、その形式の付属の file の接尾辞で終わるかを
// 返す。
func (f collectionFile) hasCompanionName() bool {
	return f.format != nil && slices.ContainsFunc(f.format.suffixes, func(suffix string) bool {
		return strings.HasSuffix(f.originPath, suffix)
	})
}

// classifyCollectionFile は file 1 つの先頭の byte 列を読み、入力形式か取り込まない理由を決める。
//
// **開けない file と読めない file は、収集の展開全体を止める。** 取り込まない一覧に回すと、利用者が
// 権限を直せば読める file を通知だけで欠いた収集になる。
func classifyCollectionFile(
	fsys fs.FS, name string, entry fs.DirEntry, formats []signedFormat,
) (collectionFile, error) {
	file := collectionFile{originPath: filepath.FromSlash(name)}
	skip := func(reason core.SkippedFileReason, kind string) (collectionFile, error) {
		file.skipped = &core.SkippedFile{OriginPath: file.originPath, Reason: reason, DetectedKind: kind}
		return file, nil
	}
	if !entry.Type().IsRegular() {
		return skip(core.SkippedFileReasonNotRegularFile, "")
	}
	opened, err := fsys.Open(name)
	if err != nil {
		return collectionFile{}, err
	}
	head := make([]byte, collectionHeadBytes)
	count, readErr := io.ReadFull(opened, head)
	closeErr := opened.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return collectionFile{}, fmt.Errorf("reading %q: %w", name, readErr)
	}
	if closeErr != nil {
		return collectionFile{}, fmt.Errorf("closing %q: %w", name, closeErr)
	}
	head = head[:count]
	if count == 0 {
		return skip(core.SkippedFileReasonEmptyFile, "")
	}
	at := slices.IndexFunc(formats, func(format signedFormat) bool { return format.parser.HasSignature(head) })
	if at < 0 {
		return skip(core.SkippedFileReasonUnsupportedFormat, detectedKindOf(head))
	}
	file.format = &formats[at]
	return file, nil
}

// detectedKindOf は、入力形式の署名を持たない file の先頭の byte 列から、分かる範囲の種類を返す。
// 分からないときは空の字句である。
func detectedKindOf(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return "zip"
	case bytes.HasPrefix(head, []byte("SQLite format 3\x00")):
		return "sqlite"
	// ESE の database は位置 4 から little endian の 0x89ABCDEF を持つ。
	case len(head) >= 8 && bytes.Equal(head[4:8], []byte{0xef, 0xcd, 0xab, 0x89}):
		return "ese"
	case utf8.Valid(head) && !strings.ContainsFunc(string(head), func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	}):
		return "text"
	default:
		return ""
	}
}

// ExpandPlans は、計画のうち入力形式が FormatKeyWindowsCollection の計画を、その directory の
// file ごとの計画に置き換え (ExpandCollection)、ほかの計画は並びを保って返す。
//
// 展開した計画は、directory の計画の案件と端末を受け継ぐ。**収集は 1 台の端末から集めた file
// であり、起動で指定した端末をすべての file に付ける。** 取り込む file を 1 つも持たない
// directory の計画は error である。
func ExpandPlans(
	fsys fs.FS, plans []SourcePlan, parsers map[core.FormatKey]ParserFactory,
) ([]SourcePlan, []core.SkippedFile, error) {
	var expanded []SourcePlan
	var skipped []core.SkippedFile
	for _, plan := range plans {
		if plan.FormatKey != FormatKeyWindowsCollection {
			expanded = append(expanded, plan)
			continue
		}
		if plan.FormatSpec != nil {
			return nil, nil, fmt.Errorf("the collection %q takes no format specification", plan.OriginPath)
		}
		if fsys == nil {
			return nil, nil, fmt.Errorf("the collection %q cannot be read without a directory reader", plan.OriginPath)
		}
		members, rest, err := ExpandCollection(fsys, plan.OriginPath, parsers)
		if err != nil {
			return nil, nil, err
		}
		if len(members) == 0 {
			return nil, nil, fmt.Errorf("the collection %q: %w", plan.OriginPath, errCollectionWithoutSource)
		}
		for _, member := range members {
			member.CaseId, member.Terminal = clonePointer(plan.CaseId), clonePointer(plan.Terminal)
			expanded = append(expanded, member)
		}
		skipped = append(skipped, rest...)
	}
	return expanded, skipped, nil
}
