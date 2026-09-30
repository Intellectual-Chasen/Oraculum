package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// stagesOver は、fsys を基準の directory にした段階を返す。
func stagesOver(t *testing.T, fsys fs.FS) *pipeline.Stages {
	t.Helper()
	config := collectionConfig()
	open := func(name string) (io.ReadCloser, error) { return fsys.Open(filepath.ToSlash(name)) }
	config.Open = open
	stages, err := pipeline.NewStages(context.Background(), pipeline.StagesConfig{
		Import: config,
		Requested: &pipeline.RequestedFiles{
			Open: open, FS: fsys,
			Size: func(name string) (int64, error) {
				info, err := fs.Stat(fsys, filepath.ToSlash(name))
				if err != nil {
					return 0, err
				}
				return info.Size(), nil
			},
		},
		Recorder: pipeline.NewMemoryRecorder(), Layers: pipeline.DefaultGraphLayers(), Clock: fixedClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stages.Wait)
	return stages
}

// listedFile は一覧の項目のうち、test が比べる値である。
type listedFile struct {
	kind       core.SourceFileKind
	candidates []core.FormatKey
	reason     core.SourceFileUndetectedReason
}

func listedFiles(t *testing.T, listing core.SourceFileListing) map[string]listedFile {
	t.Helper()
	if err := listing.Validate(); err != nil {
		t.Fatalf("the listing does not validate: %v", err)
	}
	files := map[string]listedFile{}
	for _, entry := range listing.Entries {
		file := listedFile{kind: entry.Kind, candidates: entry.FormatCandidates}
		if entry.Undetected != nil {
			file.reason = entry.Undetected.Reason
		}
		files[entry.OriginPath] = file
	}
	return files
}

func requireListed(t *testing.T, got, want map[string]listedFile) {
	t.Helper()
	for path, file := range want {
		if got[path].kind != file.kind || !slices.Equal(got[path].candidates, file.candidates) ||
			got[path].reason != file.reason {
			t.Errorf("%s is %+v, want %+v", path, got[path], file)
		}
	}
}

// directory の一覧は、署名を持つ file に形式の候補を付け、付属の file を主 file の一部にする。
func TestStagesListARequestedDirectory(t *testing.T) {
	stages := stagesOver(t, collectionFiles)
	listing, err := stages.ListRequestedFiles(context.Background(), "triage/config", false)
	if err != nil {
		t.Fatal(err)
	}
	hive := []core.FormatKey{"windows_registry_hive"}
	want := map[string]listedFile{
		"triage/config/SYSTEM":      {kind: core.SourceFileKindFile, candidates: hive},
		"triage/config/SYSTEM.LOG1": {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonCompanionFile},
		"triage/config/SYSTEM.LOG2": {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonCompanionFile},
		// 主 file の無い付属の file も、単独の収集元にしない。
		"triage/config/SOFTWARE.LOG1": {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonCompanionFile},
	}
	got := listedFiles(t, listing)
	requireListed(t, got, want)
	if len(got) != len(want) {
		t.Errorf("the listing has %d entries, want %d", len(got), len(want))
	}

	top, err := stages.ListRequestedFiles(context.Background(), "triage", false)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []core.SourceFileKind
	for _, entry := range top.Entries {
		kinds = append(kinds, entry.Kind)
	}
	// directory を先に並べる。
	if first := slices.Index(kinds, core.SourceFileKindFile); first < 0 ||
		slices.Contains(kinds[first:], core.SourceFileKindDirectory) {
		t.Errorf("the kinds are in the order %v, want the directories first", kinds)
	}
}

// 再帰の一覧は下の directory の file を持ち、directory を持たない。候補の無い file は理由を持つ。
func TestStagesListARequestedDirectoryRecursively(t *testing.T) {
	stages := stagesOver(t, collectionFiles)
	listing, err := stages.ListRequestedFiles(context.Background(), "./triage", true)
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != "triage" || !listing.Recursive || listing.Truncated {
		t.Errorf("the listing is %q recursive=%v truncated=%v", listing.Path, listing.Recursive, listing.Truncated)
	}
	got := listedFiles(t, listing)
	requireListed(t, got, map[string]listedFile{
		"triage/logs/System.evtx":         {kind: core.SourceFileKindFile, candidates: []core.FormatKey{"windows_evtx"}},
		"triage/prefetch/APP-0000AAAA.pf": {kind: core.SourceFileKindFile, candidates: []core.FormatKey{"windows_prefetch"}},
		"triage/empty.dat":                {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonEmptyFile},
		"triage/archive.zip":              {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonUnsupportedFormat},
		"triage/link":                     {kind: core.SourceFileKindOther},
	})
	if _, found := got["outside/System.evtx"]; found {
		t.Error("the recursive listing of triage lists a file outside it")
	}
}

// 読み込みの要求に渡せない名前の項目は、一覧を止めずに選べない項目になる。名前の読めない byte は
// `[0xNN]` に、制御文字は `[U+NNNN]` に書き換え、違う名前を違う path で出す。その下の file は一覧に
// 出さない。
func TestStagesListNamesTheRequestCannotCarryAsOtherEntries(t *testing.T) {
	stages := stagesOver(t, fstest.MapFS{
		"logs/a\u0085b.log":     {Data: []byte(detectSample)},
		"logs/c\xffd.log":       {Data: []byte(detectSample)},
		"logs/\x95\xf1.log":     {Data: []byte(detectSample)},
		"logs/\x8d\x90.log":     {Data: []byte(detectSample)},
		"logs/e\x01f/inner.log": {Data: []byte(detectSample)},
		"logs/plain.log":        {Data: []byte{}},
		"\x95\xf1-top.log":      {Data: []byte(detectSample)},
		// Windows で作った名前は、基準の directory の直下で絶対 path として読まれる。
		`\x.log`:   {Data: []byte(detectSample)},
		`C:\x.log`: {Data: []byte(detectSample)},
	})
	for _, recursive := range []bool{false, true} {
		listing, err := stages.ListRequestedFiles(context.Background(), "logs", recursive)
		if err != nil {
			t.Fatalf("recursive=%v: %v", recursive, err)
		}
		requireListed(t, listedFiles(t, listing), map[string]listedFile{
			"logs/a[U+0085]b.log":   {kind: core.SourceFileKindOther},
			"logs/c[0xFF]d.log":     {kind: core.SourceFileKindOther},
			"logs/[0x95][0xF1].log": {kind: core.SourceFileKindOther},
			"logs/[0x8D][0x90].log": {kind: core.SourceFileKindOther},
			"logs/e[U+0001]f":       {kind: core.SourceFileKindOther},
			"logs/plain.log":        {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonEmptyFile},
		})
		if len(listing.Entries) != 6 {
			t.Errorf("recursive=%v: the listing has %d entries, want 6", recursive, len(listing.Entries))
		}
	}
	// 基準の directory の直下の項目も、相対 path の検査を通る path で出す。
	top, err := stages.ListRequestedFiles(context.Background(), ".", false)
	if err != nil {
		t.Fatal(err)
	}
	requireListed(t, listedFiles(t, top), map[string]listedFile{
		"[0x95][0xF1]-top.log": {kind: core.SourceFileKindOther},
		"[0x5C]x.log":          {kind: core.SourceFileKindOther},
		"C[0x3A][0x5C]x.log":   {kind: core.SourceFileKindOther},
		"logs":                 {kind: core.SourceFileKindDirectory},
	})
}

// readDirFailingFS は、directory failing の中身を読めない file system である。
type readDirFailingFS struct {
	fstest.MapFS
	failing string
}

func (f readDirFailingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.failing {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.ReadDir(name)
}

// 再帰の一覧は、中を読めない下の directory を選べない項目にして、ほかの file の一覧を続ける。
// 一覧にする directory そのものを読めないときは失敗を返す。
func TestStagesListAroundAnUnreadableSubdirectory(t *testing.T) {
	stages := stagesOver(t, readDirFailingFS{MapFS: fstest.MapFS{
		"logs/locked/inner.log": {Data: []byte(detectSample)},
		"logs/open/plain.log":   {Data: []byte{}},
	}, failing: "logs/locked"})
	listing, err := stages.ListRequestedFiles(context.Background(), "logs", true)
	if err != nil {
		t.Fatal(err)
	}
	got := listedFiles(t, listing)
	requireListed(t, got, map[string]listedFile{
		"logs/locked":         {kind: core.SourceFileKindOther},
		"logs/open/plain.log": {kind: core.SourceFileKindFile, reason: core.SourceFileUndetectedReasonEmptyFile},
	})
	if len(got) != 2 {
		t.Errorf("the listing has %d entries, want 2: %v", len(got), got)
	}

	var rejected *pipeline.LoadingRequestError
	if _, err := stages.ListRequestedFiles(context.Background(), "logs/locked", true); !errors.As(err, &rejected) ||
		rejected.Rejection != core.LoadingRejectionFileUnreadable {
		t.Errorf("listing the unreadable directory returned %v", err)
	}
}

// detectSample は、どの形式にも当たらない file の中身である。
const detectSample = "sample text\n"

// 項目の数が上限を超えると、上限までの項目と打ち切りの印を返す。
func TestStagesListUpToTheLimit(t *testing.T) {
	const limit = 5000
	files := fstest.MapFS{}
	for index := range limit + 1 {
		files[fmt.Sprintf("many/%05d.log", index)] = &fstest.MapFile{Data: []byte{}}
	}
	stages := stagesOver(t, files)
	for _, recursive := range []bool{false, true} {
		listing, err := stages.ListRequestedFiles(context.Background(), "many", recursive)
		if err != nil {
			t.Fatal(err)
		}
		if len(listing.Entries) != limit || !listing.Truncated {
			t.Errorf("recursive=%v: %d entries truncated=%v, want %d and truncated", recursive,
				len(listing.Entries), listing.Truncated, limit)
		}
	}
}

// 要求が取り消されたら、走査を止めて取り消しの失敗を返す。
func TestStagesStopTheListingWhenCanceled(t *testing.T) {
	stages := stagesOver(t, collectionFiles)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, recursive := range []bool{false, true} {
		if _, err := stages.ListRequestedFiles(ctx, "triage", recursive); !errors.Is(err, context.Canceled) {
			t.Errorf("recursive=%v: listing after the cancel returned %v", recursive, err)
		}
	}
}

// `\` かドライブ文字で始まる path の一覧は、その directory が在っても、基準の外の path として退ける。
// 一覧の応答の path は絶対 path に見える値を持てない (core.SourceFileListing.Validate)。
func TestStagesRefuseAListingPathThatReadsAsAbsolute(t *testing.T) {
	stages := stagesOver(t, fstest.MapFS{
		`\x/inner.log`:  {Data: []byte(detectSample)},
		"C:/sub/in.log": {Data: []byte(detectSample)},
	})
	for _, dir := range []string{`\x`, "C:/sub"} {
		_, err := stages.ListRequestedFiles(context.Background(), dir, false)
		var rejected *pipeline.LoadingRequestError
		if !errors.As(err, &rejected) || rejected.Rejection != core.LoadingRejectionPathOutsideBase {
			t.Errorf("listing %q returned %v, want %q", dir, err, core.LoadingRejectionPathOutsideBase)
		}
	}
}

// 一覧にできない path は、理由の種別を持つ失敗で退ける。
func TestStagesRefuseAnInvalidListing(t *testing.T) {
	stages := stagesOver(t, collectionFiles)
	for dir, want := range map[string]core.LoadingRejection{
		"..":                   core.LoadingRejectionPathOutsideBase,
		"triage/../..":         core.LoadingRejectionPathOutsideBase,
		"absent":               core.LoadingRejectionFileAbsent,
		"triage/notes.txt":     core.LoadingRejectionNotDirectory,
		"triage/\x00":          core.LoadingRejectionPathControlCharacter,
		"triage/logs/../../..": core.LoadingRejectionPathOutsideBase,
	} {
		_, err := stages.ListRequestedFiles(context.Background(), dir, false)
		var rejected *pipeline.LoadingRequestError
		if !errors.As(err, &rejected) || rejected.Rejection != want || rejected.OriginPath != dir {
			t.Errorf("listing %q returned %v, want %q", dir, err, want)
		}
	}

	withoutBase, err := pipeline.NewStages(context.Background(), pipeline.StagesConfig{
		Import: collectionConfig(), Recorder: pipeline.NewMemoryRecorder(), Layers: pipeline.DefaultGraphLayers(),
		Clock: fixedClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var rejected *pipeline.LoadingRequestError
	if _, err := withoutBase.ListRequestedFiles(context.Background(), ".", false); !errors.As(err, &rejected) ||
		rejected.Rejection != core.LoadingRejectionNoBaseDirectory {
		t.Errorf("listing without a base directory returned %v", err)
	}
}
