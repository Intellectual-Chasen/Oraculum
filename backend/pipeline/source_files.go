package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sourceFileListingLimit は、基準の directory の一覧 1 回が持つ項目の上限である。
const sourceFileListingLimit = 5000

// ListRequestedFiles は、基準の directory の下の directory dir の項目を、file ごとの入力形式の
// 候補とともに返す。dir は基準の directory からの相対 path であり、`.` は基準の directory
// そのものである。recursive が真のときは、下の directory の file と、辿らない項目を返す。
//
// **項目は読み込みの要求と同じ基準で辿る。** 基準の外へ出る path と symbolic link を退け、
// 返す path は `/` で区切り、読み込みの要求の originPath にそのまま渡せる。
//
// ctx が取り消されたら走査を止め、ctx の失敗を返す。
func (s *Stages) ListRequestedFiles(ctx context.Context, dir string, recursive bool) (core.SourceFileListing, error) {
	reject := func(rejection core.LoadingRejection, err error, cause error) (core.SourceFileListing, error) {
		return core.SourceFileListing{}, &LoadingRequestError{Rejection: rejection, OriginPath: dir, err: err, cause: cause}
	}
	if s.config.Requested == nil || s.config.Requested.FS == nil {
		return core.SourceFileListing{}, &LoadingRequestError{
			Rejection: core.LoadingRejectionNoBaseDirectory,
			err:       errors.New("the server has no base directory for requested files"),
		}
	}
	cleaned := filepath.Clean(dir)
	switch {
	case hasControlCharacter(dir):
		return reject(core.LoadingRejectionPathControlCharacter, errors.New("the path has a control character"), nil)
	case cleaned != "." && !isRequestedLocalPath(cleaned):
		return reject(core.LoadingRejectionPathOutsideBase, errors.New("the path must be relative to the base directory"), nil)
	}
	fsys := s.config.Requested.FS
	root := filepath.ToSlash(cleaned)
	info, err := fs.Stat(fsys, root)
	if err != nil {
		rejection := listingRejection(err)
		return reject(rejection, fmt.Errorf("reading the directory failed as %s", rejection), err)
	}
	if !info.IsDir() {
		return reject(core.LoadingRejectionNotDirectory, errors.New("the path is not a directory"), nil)
	}
	lister := sourceFileLister{
		ctx: ctx, fsys: fsys, open: s.config.Requested.Open, detector: newFormatDetector(s.config.Import.Parsers),
	}
	listing := core.SourceFileListing{Path: root, Recursive: recursive, Entries: []core.SourceFileEntry{}}
	if recursive {
		err = lister.walk(root, &listing)
	} else {
		err = lister.list(root, &listing)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return core.SourceFileListing{}, ctxErr
	}
	if err != nil {
		rejection := listingRejection(err)
		return reject(rejection, fmt.Errorf("reading the directory failed as %s", rejection), err)
	}
	markCompanions(lister.detector, listing.Entries)
	return listing, nil
}

// listingRejection は、一覧にする directory を読めなかった失敗を理由の種別にする。
//
// **基準の外へ出る symbolic link の失敗は errno を持たない** (os.Root)。「無い」と「権限が
// 無い」以外は、読めない理由にまとめる。
func listingRejection(err error) core.LoadingRejection {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return core.LoadingRejectionFileAbsent
	default:
		return core.LoadingRejectionFileUnreadable
	}
}

// sourceFileLister は基準の directory の項目を読み、file の候補を決める。
type sourceFileLister struct {
	ctx      context.Context
	fsys     fs.FS
	open     func(originPath string) (io.ReadCloser, error)
	detector formatDetector
}

// list は directory 1 つの項目を、directory、それ以外の順に、それぞれ名前の順で足す。
func (l sourceFileLister) list(root string, listing *core.SourceFileListing) error {
	entries, err := fs.ReadDir(l.fsys, root)
	if err != nil {
		return err
	}
	var directories, others []core.SourceFileEntry
	for _, entry := range entries {
		if err := l.ctx.Err(); err != nil {
			return err
		}
		if len(directories)+len(others) == sourceFileListingLimit {
			listing.Truncated = true
			break
		}
		item := l.entry(path.Join(root, entry.Name()), entry)
		if item.Kind == core.SourceFileKindDirectory {
			directories = append(directories, item)
		} else {
			others = append(others, item)
		}
	}
	listing.Entries = append(append(listing.Entries, directories...), others...)
	return nil
}

// errListingFull は、再帰の一覧が上限に達したときに走査を止める印である。
var errListingFull = errors.New("the listing reached its limit")

// walk は directory の下の directory 以外の項目を、path の辞書順で足す。
//
// **辿らない directory を、選べない項目として一覧に残す。** symbolic link の directory、名前を
// 読み込みの要求に渡せない directory、中を読めない下の directory は辿らない。まとめて選んだ分析者が、
// 除いた項目を数えられる。一覧にする directory そのものを読めないときは失敗を返す。
func (l sourceFileLister) walk(root string, listing *core.SourceFileListing) error {
	err := fs.WalkDir(l.fsys, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if err := l.ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if name == root || entry == nil {
				return walkErr
			}
			// 中を読めない下の directory は、その directory を選べない項目にして次へ進む。
			item := l.entry(name, entry)
			item.Kind = core.SourceFileKindOther
			item.SizeBytes, item.FormatCandidates, item.Undetected = nil, nil, nil
			return l.appendWalked(listing, item, fs.SkipDir)
		}
		item := l.entry(name, entry)
		if entry.IsDir() && item.Kind == core.SourceFileKindDirectory {
			return nil
		}
		if item.Kind == core.SourceFileKindDirectory {
			item.Kind = core.SourceFileKindOther
		}
		next := error(nil)
		if entry.IsDir() && name != root {
			next = fs.SkipDir
		}
		return l.appendWalked(listing, item, next)
	})
	if errors.Is(err, errListingFull) {
		return nil
	}
	return err
}

// appendWalked は再帰の一覧に項目を足し、走査の続け方 next を返す。上限に達していれば足さずに
// 走査を止める。
func (l sourceFileLister) appendWalked(listing *core.SourceFileListing, item core.SourceFileEntry, next error) error {
	if len(listing.Entries) == sourceFileListingLimit {
		listing.Truncated = true
		return errListingFull
	}
	listing.Entries = append(listing.Entries, item)
	return next
}

// entry は項目 1 つの種類を決め、file であれば入力形式の候補を決める。
//
// **symbolic link は、基準の下で辿った先の種類で数える。** 読み込みも基準の下に留まる symbolic
// link を開く (RequestedFiles)。辿れない link は、選べない項目である。
//
// **path を読み込みの要求に渡せない項目は、選べない項目にする。** UTF-8 として読めない名前は JSON で
// 別の文字列に変わり、制御文字を持つ path は読み込みの要求が退ける。`\` かドライブ文字で始まる path
// (Windows で作った名前) は、絶対 path として読まれる (core.IsAbsolutePath)。表示には、これらの文字を
// 書き換えた path を返す (displayablePath)。
func (l sourceFileLister) entry(name string, entry fs.DirEntry) core.SourceFileEntry {
	if !utf8.ValidString(name) || hasControlCharacter(name) || core.IsAbsolutePath(name) {
		return core.SourceFileEntry{
			OriginPath: displayablePath(name), Name: displayablePath(entry.Name()), Kind: core.SourceFileKindOther,
		}
	}
	item := core.SourceFileEntry{OriginPath: name, Name: entry.Name(), Kind: core.SourceFileKindOther}
	var info fs.FileInfo
	var err error
	if entry.Type()&fs.ModeSymlink != 0 {
		info, err = fs.Stat(l.fsys, name)
	} else {
		info, err = entry.Info()
	}
	if err != nil {
		return item
	}
	switch {
	case info.IsDir():
		item.Kind = core.SourceFileKindDirectory
	case info.Mode().IsRegular():
		item.Kind = core.SourceFileKindFile
		size := info.Size()
		item.SizeBytes = &size
		item.FormatCandidates, item.Undetected = l.detect(filepath.FromSlash(name), size)
	}
	return item
}

// displayablePath は、UTF-8 として読めない byte と `\` と `:` を `[0xNN]` に、制御文字を `[U+NNNN]` に
// 書き換えた path を返す。
//
// **読めない byte を 1 byte ずつ書き換える。** 違う名前が同じ表示にならず、一覧の項目を path で
// 区別できる。書き換えた path は `\` とドライブ文字で始まらず、相対 path の検査を通る
// (core.SourceFileEntry.Validate)。
func displayablePath(name string) string {
	var built strings.Builder
	for index := 0; index < len(name); {
		r, size := utf8.DecodeRuneInString(name[index:])
		switch {
		case r == utf8.RuneError && size <= 1, r == '\\', r == ':':
			fmt.Fprintf(&built, "[0x%02X]", name[index])
		case unicode.IsControl(r):
			fmt.Fprintf(&built, "[U+%04X]", r)
		default:
			built.WriteString(name[index : index+size])
		}
		index += max(size, 1)
	}
	return built.String()
}

// detect は file の先頭の標本を読み、入力形式の候補か、候補が無い理由を返す。
func (l sourceFileLister) detect(originPath string, size int64) ([]core.FormatKey, *core.SourceFileUndetected) {
	if size == 0 {
		return nil, &core.SourceFileUndetected{Reason: core.SourceFileUndetectedReasonEmptyFile}
	}
	unreadable := &core.SourceFileUndetected{Reason: core.SourceFileUndetectedReasonUnreadable}
	file, err := l.open(originPath)
	if err != nil {
		return nil, unreadable
	}
	sample := make([]byte, min(size, detectionSampleBytes))
	count, readErr := io.ReadFull(file, sample)
	_ = file.Close() // 読み取り専用の file を閉じる失敗は、読めた標本の判定を変えない。
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return nil, unreadable
	}
	result := l.detector.detect(sample[:count], int64(count) < size)
	if len(result.candidates) == 0 {
		return nil, &core.SourceFileUndetected{
			Reason: core.SourceFileUndetectedReasonUnsupportedFormat, DetectedKind: result.detectedKind,
		}
	}
	return result.candidates, nil
}

// markCompanions は付属の file を、単独の収集元の候補から外す。付属の file は、候補を持つ主 file の
// path に付属の接尾辞を足した file と、候補の形式の付属の接尾辞で名前が終わる file である。主 file の
// 収集元の読み込みが一緒に読む (CompanionFileParser)。
func markCompanions(detector formatDetector, entries []core.SourceFileEntry) {
	companions := map[string]bool{}
	for _, entry := range entries {
		for _, key := range entry.FormatCandidates {
			for _, suffix := range detector.companionSuffixes(key) {
				if strings.HasSuffix(entry.OriginPath, suffix) {
					companions[entry.OriginPath] = true
				} else {
					companions[entry.OriginPath+suffix] = true
				}
			}
		}
	}
	for index, entry := range entries {
		if entry.Kind == core.SourceFileKindFile && companions[entry.OriginPath] {
			entries[index].FormatCandidates = nil
			entries[index].Undetected = &core.SourceFileUndetected{Reason: core.SourceFileUndetectedReasonCompanionFile}
		}
	}
}
