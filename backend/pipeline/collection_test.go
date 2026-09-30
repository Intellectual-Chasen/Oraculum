package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 収集の directory の file。先頭の byte 列だけが入力形式の署名であり、残りは値である。
var collectionFiles = fstest.MapFS{
	"triage/logs/System.evtx":         {Data: []byte("ElfFile\x00synthetic-system")},
	"triage/logs/Security.evtx":       {Data: []byte("ElfFile\x00synthetic-security")},
	"triage/config/SYSTEM":            {Data: []byte("regf-synthetic-hive")},
	"triage/config/SYSTEM.LOG1":       {Data: []byte("regf-synthetic-log1")},
	"triage/config/SYSTEM.LOG2":       {Data: []byte("\x00\x00\x00\x00zeroed-log2")},
	"triage/config/SOFTWARE.LOG1":     {Data: []byte("regf-orphan-log")},
	"triage/prefetch/APP-0000AAAA.pf": {Data: []byte("MAM\x04synthetic-compressed")},
	"triage/prefetch/OLD-0000BBBB.pf": {Data: []byte("\x11\x00\x00\x00SCCAsynthetic")},
	"triage/notes.txt":                {Data: []byte("collected by the synthetic tool\n")},
	"triage/archive.zip":              {Data: []byte("PK\x03\x04synthetic")},
	"triage/browser/History":          {Data: []byte("SQLite format 3\x00synthetic")},
	"triage/browser/Cache.dat":        {Data: []byte("\x00\x00\x00\x00\xef\xcd\xab\x89synthetic")},
	"triage/empty.dat":                {Data: []byte{}},
	"triage/link":                     {Data: []byte("elsewhere"), Mode: fs.ModeSymlink},
	"outside/System.evtx":             {Data: []byte("ElfFile\x00outside")},
}

func TestExpandCollectionPlansSignedFilesAndListsTheRest(t *testing.T) {
	plans, skipped, err := pipeline.ExpandCollection(collectionFiles, "./triage", pipeline.NewTestFormatRegistry())
	if err != nil {
		t.Fatal(err)
	}
	type planned struct {
		origin, collection string
		format             core.FormatKey
	}
	var got []planned
	for _, plan := range plans {
		if plan.FileName != filepath.Base(plan.OriginPath) {
			t.Errorf("the plan %q has the file name %q", plan.OriginPath, plan.FileName)
		}
		got = append(got, planned{plan.OriginPath, plan.CollectionPath, plan.FormatKey})
	}
	want := []planned{
		{"triage/config/SYSTEM", "triage", "windows_registry_hive"},
		{"triage/logs/Security.evtx", "triage", "windows_evtx"},
		{"triage/logs/System.evtx", "triage", "windows_evtx"},
		{"triage/prefetch/APP-0000AAAA.pf", "triage", "windows_prefetch"},
		{"triage/prefetch/OLD-0000BBBB.pf", "triage", "windows_prefetch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the plans are %+v, want %+v", got, want)
	}
	wantSkipped := []core.SkippedFile{
		{OriginPath: "triage/archive.zip", Reason: core.SkippedFileReasonUnsupportedFormat, DetectedKind: "zip"},
		{OriginPath: "triage/browser/Cache.dat", Reason: core.SkippedFileReasonUnsupportedFormat, DetectedKind: "ese"},
		{OriginPath: "triage/browser/History", Reason: core.SkippedFileReasonUnsupportedFormat, DetectedKind: "sqlite"},
		{OriginPath: "triage/config/SOFTWARE.LOG1", Reason: core.SkippedFileReasonCompanionWithoutMain,
			DetectedKind: "windows_registry_hive"},
		{OriginPath: "triage/empty.dat", Reason: core.SkippedFileReasonEmptyFile},
		{OriginPath: "triage/link", Reason: core.SkippedFileReasonNotRegularFile},
		{OriginPath: "triage/notes.txt", Reason: core.SkippedFileReasonUnsupportedFormat, DetectedKind: "text"},
	}
	if !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("the skipped files are %+v, want %+v", skipped, wantSkipped)
	}
	for _, file := range skipped {
		if err := file.Validate(); err != nil {
			t.Errorf("the skipped file %q: %v", file.OriginPath, err)
		}
	}
}

func TestExpandCollectionRejectsAPathOutsideTheRoot(t *testing.T) {
	if _, _, err := pipeline.ExpandCollection(collectionFiles, "../triage", pipeline.NewTestFormatRegistry()); err == nil {
		t.Error("expanding a collection above the root succeeded")
	}
	if _, _, err := pipeline.ExpandCollection(collectionFiles, "absent", pipeline.NewTestFormatRegistry()); err == nil {
		t.Error("expanding an absent collection succeeded")
	}
}

func TestExpandPlansCarriesCaseAndTerminalToEveryMember(t *testing.T) {
	caseId := "case-a"
	terminal := &pipeline.SourceTerminal{TerminalHostname: "host-a.example.test"}
	single := pipeline.SourcePlan{OriginPath: "outside/System.evtx", FileName: "System.evtx", FormatKey: "windows_evtx",
		CaseId: &caseId}
	parsers := pipeline.NewTestFormatRegistry()
	plans, skipped, err := pipeline.ExpandPlans(collectionFiles, []pipeline.SourcePlan{
		single,
		{OriginPath: "triage", FormatKey: pipeline.FormatKeyWindowsCollection, CaseId: &caseId, Terminal: terminal},
	}, parsers)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plans[0], single) {
		t.Errorf("the plan of a single file became %+v", plans[0])
	}
	for _, plan := range plans[1:] {
		if plan.CaseId == nil || *plan.CaseId != caseId || plan.Terminal == nil || *plan.Terminal != *terminal {
			t.Errorf("the member %q has the case %v and the terminal %+v", plan.OriginPath, plan.CaseId, plan.Terminal)
		}
	}
	if len(plans) < 2 || len(skipped) == 0 {
		t.Errorf("expanding gave the plans %+v and the skipped files %+v", plans, skipped)
	}

	if _, _, err := pipeline.ExpandPlans(fstest.MapFS{"only/notes.txt": {Data: []byte("text")}},
		[]pipeline.SourcePlan{{OriginPath: "only", FormatKey: pipeline.FormatKeyWindowsCollection}},
		parsers); err == nil {
		t.Error("expanding a collection without a supported file succeeded")
	}
	spec := "%h"
	if _, _, err := pipeline.ExpandPlans(collectionFiles, []pipeline.SourcePlan{
		{OriginPath: "triage", FormatKey: pipeline.FormatKeyWindowsCollection, FormatSpec: &spec},
	}, parsers); err == nil {
		t.Error("expanding a collection with a format specification succeeded")
	}
}

// wholeFileParser は、収集の directory から取り込む入力形式の代わりである。連結した byte 列全体を
// 1 件のレコードとして返す。
type wholeFileParser struct {
	format    core.FormatKey
	signature func(head []byte) bool
	suffixes  []string
	content   []byte
	done      bool
}

func (p *wholeFileParser) HasSignature(head []byte) bool { return p.signature(head) }

// prefixAt は、先頭から offset byte の位置に prefix を持つ byte 列を見分ける。
func prefixAt(offset int, prefix string) func([]byte) bool {
	return func(head []byte) bool {
		return len(head) >= offset && bytes.HasPrefix(head[offset:], []byte(prefix))
	}
}

func (p *wholeFileParser) Identity() pipeline.ParserIdentity {
	return pipeline.ParserIdentity{
		ParserID: "whole-file-" + string(p.format), FormatKey: p.format, PositionKind: core.PositionKindByteRange,
		TimePrecision: core.PrecisionSecond, RecordedByOneTerminal: true, ItemSemantics: []core.SemanticKey{},
		ConnectionRequestKinds: []core.ObservationKindSelector{}, ConnectionMatchConditions: []pipeline.ConnectionMatchCondition{},
		TranscriptIdentityItems: []string{},
	}
}

func (p *wholeFileParser) Reset(input io.Reader) {
	p.content, _ = io.ReadAll(input)
	p.done = false
}

func (p *wholeFileParser) Next() (pipeline.ParsedRecord, *core.ImportFailure, error) {
	if p.done {
		return pipeline.ParsedRecord{}, nil, io.EOF
	}
	p.done = true
	length := int64(len(p.content))
	return pipeline.ParsedRecord{RawText: string(p.content), ByteLength: &length}, nil, nil
}

// companionWholeFileParser は付属の file を一緒に読む wholeFileParser である。
type companionWholeFileParser struct{ wholeFileParser }

func (p *companionWholeFileParser) CompanionSuffixes() []string { return p.suffixes }

func (p *companionWholeFileParser) SetMembers([]core.SourceMember) {}

func collectionRunner(t *testing.T) *pipeline.Runner {
	t.Helper()
	runner, err := pipeline.NewRunner(collectionConfig())
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestStagesLoadARequestedCollection(t *testing.T) {
	config := collectionConfig()
	stages, err := pipeline.NewStages(context.Background(), pipeline.StagesConfig{
		Import: config,
		Requested: &pipeline.RequestedFiles{
			Open: config.Open, FS: collectionFiles,
			Size: func(name string) (int64, error) {
				info, err := fs.Stat(collectionFiles, filepath.ToSlash(name))
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
	err = stages.StartRequestedLoading([]pipeline.SourcePlan{
		{OriginPath: "absent", FormatKey: pipeline.FormatKeyWindowsCollection},
	})
	var rejected *pipeline.LoadingRequestError
	if !errors.As(err, &rejected) || rejected.Rejection != core.LoadingRejectionFileAbsent {
		t.Errorf("requesting an absent collection returned %v", err)
	}
	if err := stages.StartRequestedLoading([]pipeline.SourcePlan{
		{OriginPath: "triage", FormatKey: pipeline.FormatKeyWindowsCollection},
	}); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	store, loaded := stages.Loaded()
	if !loaded {
		t.Fatalf("the loading is %+v", requireValidSnapshot(t, stages).Loading)
	}
	result := store.ImportResult()
	want, _, err := pipeline.ExpandCollection(collectionFiles, "triage", config.Parsers)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Fatalf("%d sources loaded, %d planned", len(entries), len(want))
	}
	for index, entry := range entries {
		if entry.Identity.OriginPath != want[index].OriginPath || entry.Identity.CollectionPath != "triage" {
			t.Errorf("the source %d is %q in %q, want %q", index, entry.Identity.OriginPath,
				entry.Identity.CollectionPath, want[index].OriginPath)
		}
	}
	var skipped []string
	for _, file := range result.SkippedFiles() {
		skipped = append(skipped, file.OriginPath)
	}
	if strings.Join(skipped, ",") !=
		"triage/archive.zip,triage/browser/Cache.dat,triage/browser/History,triage/config/SOFTWARE.LOG1,triage/empty.dat,triage/link,triage/notes.txt" {
		t.Errorf("the loaded result lists the skipped files %v", skipped)
	}
}

func collectionConfig() pipeline.Config {
	config := runnerConfig("")
	config.Parsers = map[core.FormatKey]pipeline.ParserFactory{
		"windows_evtx": func(*string) (pipeline.SourceParser, error) {
			return &wholeFileParser{format: "windows_evtx", signature: prefixAt(0, "ElfFile\x00")}, nil
		},
		"windows_prefetch": func(*string) (pipeline.SourceParser, error) {
			return &wholeFileParser{format: "windows_prefetch", signature: func(head []byte) bool {
				return prefixAt(0, "MAM\x04")(head) || prefixAt(4, "SCCA")(head)
			}}, nil
		},
		"windows_registry_hive": func(*string) (pipeline.SourceParser, error) {
			return &companionWholeFileParser{wholeFileParser{
				format: "windows_registry_hive", signature: prefixAt(0, "regf"), suffixes: []string{".LOG1", ".LOG2"},
			}}, nil
		},
	}
	config.Open = func(path string) (io.ReadCloser, error) {
		return collectionFiles.Open(filepath.ToSlash(path))
	}
	return config
}

func TestExpandedCollectionImportsLikeTheFilesOneByOne(t *testing.T) {
	expanded, _, err := pipeline.ExpandPlans(collectionFiles, []pipeline.SourcePlan{
		{OriginPath: "triage", FormatKey: pipeline.FormatKeyWindowsCollection},
	}, collectionConfig().Parsers)
	if err != nil {
		t.Fatal(err)
	}
	var oneByOne []pipeline.SourcePlan
	for _, plan := range expanded {
		oneByOne = append(oneByOne, pipeline.SourcePlan{
			OriginPath: plan.OriginPath, FileName: filepath.Base(plan.OriginPath), FormatKey: plan.FormatKey,
		})
	}
	fromCollection, err := collectionRunner(t).Run(expanded)
	if err != nil {
		t.Fatal(err)
	}
	fromFiles, err := collectionRunner(t).Run(oneByOne)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromCollection.Statuses(), fromFiles.Statuses()) {
		t.Errorf("the statuses differ:\n%+v\n%+v", fromCollection.Statuses(), fromFiles.Statuses())
	}
	collectionEntries, err := fromCollection.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	fileEntries, err := fromFiles.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(collectionEntries) != len(fileEntries) || len(collectionEntries) != len(expanded) {
		t.Fatalf("%d sources from the collection, %d from the files, %d plans",
			len(collectionEntries), len(fileEntries), len(expanded))
	}
	for index, entry := range collectionEntries {
		if entry.Identity.CollectionPath != "triage" {
			t.Errorf("the source %q has the collection %q", entry.Identity.OriginPath, entry.Identity.CollectionPath)
		}
		identity := entry.Identity
		identity.CollectionPath = ""
		if !reflect.DeepEqual(identity, fileEntries[index].Identity) {
			t.Errorf("the source %d differs:\n%+v\n%+v", index, identity, fileEntries[index].Identity)
		}
	}
	hive := collectionEntries[0].Identity
	var members []string
	for _, member := range hive.Members {
		members = append(members, member.OriginPath)
	}
	if strings.Join(members, ",") != "triage/config/SYSTEM,triage/config/SYSTEM.LOG1,triage/config/SYSTEM.LOG2" {
		t.Errorf("the hive has the members %v", members)
	}
}

// 調査は収集の directory の取り込まなかった file を記録し、開き直した起動の取り込み結果も持つ。
func TestInvestigationKeepsTheSkippedFilesAfterReopening(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "case")
	plans, skipped, err := pipeline.ExpandPlans(collectionFiles, []pipeline.SourcePlan{
		{OriginPath: "triage", FormatKey: pipeline.FormatKeyWindowsCollection},
	}, collectionConfig().Parsers)
	if err != nil || len(skipped) == 0 {
		t.Fatalf("expanding the collection returned %d skipped files and %v", len(skipped), err)
	}
	load := func(plansOf func(*pipeline.Investigation) []pipeline.SourcePlan, given []core.SkippedFile) []core.SkippedFile {
		t.Helper()
		investigation, err := pipeline.PrepareInvestigation(ctx, dir, t.TempDir(), "")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := investigation.Close(); err != nil {
				t.Fatal(err)
			}
		}()
		stages, err := pipeline.NewStages(ctx, pipeline.StagesConfig{
			Import: collectionConfig(), Recorder: investigation, Layers: pipeline.DefaultGraphLayers(),
			Clock: fixedClock{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := stages.Load(plansOf(investigation), given); err != nil {
			t.Fatal(err)
		}
		store, loaded := stages.Loaded()
		if !loaded {
			t.Fatal("the investigation did not load")
		}
		return store.ImportResult().SkippedFiles()
	}
	if got := load(func(*pipeline.Investigation) []pipeline.SourcePlan { return plans }, skipped); !reflect.DeepEqual(got, skipped) {
		t.Errorf("the created investigation lists %+v, want %+v", got, skipped)
	}
	if got := load((*pipeline.Investigation).RecordedPlans, nil); !reflect.DeepEqual(got, skipped) {
		t.Errorf("the reopened investigation lists %+v, want %+v", got, skipped)
	}
}
