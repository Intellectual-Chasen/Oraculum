package pipeline_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// runFixtureDir は pipeline と api の 2 package が同じ byte 列を読むため、
// internal/testdata へ集約している。
const runFixtureDir = "../internal/testdata/run/"

func runnerConfig(input string) pipeline.Config {
	return pipeline.Config{
		Open:    func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(input)), nil },
		Parsers: pipeline.NewTestFormatRegistry(),
		Minter:  pipeline.DigestMinter{}, Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value }, Revision: "test-revision", SettingsDigest: "test-settings",
	}
}

type renamedParser struct {
	pipeline.SourceParser
	format core.FormatKey
}

func (p *renamedParser) Identity() pipeline.ParserIdentity {
	identity := p.SourceParser.Identity()
	identity.FormatKey = p.format
	return identity
}

type runFixture struct {
	File                                    string
	Format                                  core.FormatKey
	Digest                                  string
	Size, Newlines, Read, Succeeded, Failed int64
	State                                   core.PublicationState
}

func runFixtures(t *testing.T) []runFixture {
	t.Helper()
	data, err := os.ReadFile(runFixtureDir + "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []runFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 4 {
		t.Fatal("run manifest cases missing")
	}
	return fixtures
}

func TestRunnerPublicationsAndIdentities(t *testing.T) {
	fixtures := runFixtures(t)
	plans := make([]pipeline.SourcePlan, len(fixtures))
	inputs := make(map[string]string)
	for i, fixture := range fixtures {
		path := runFixtureDir + fixture.File
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inputs[path] = string(data)
		plans[i] = pipeline.SourcePlan{OriginPath: path, FileName: fixture.File, FormatKey: fixture.Format}
	}
	var opened []string
	config := runnerConfig("")
	config.Open = func(path string) (io.ReadCloser, error) {
		opened = append(opened, path)
		input, ok := inputs[path]
		if !ok {
			t.Fatalf("unexpected Open(%q)", path)
		}
		return io.NopCloser(strings.NewReader(input)), nil
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	statuses := result.Statuses()
	if len(statuses) != len(fixtures) || len(opened) != len(fixtures) {
		t.Fatalf("statuses=%d opened=%v", len(statuses), opened)
	}
	for i, fixture := range fixtures {
		status := statuses[i]
		if err := status.Validate(); err != nil {
			t.Fatal(err)
		}
		if opened[i] != plans[i].OriginPath || status.PublicationState != fixture.State {
			t.Fatalf("source %d: opened=%q status=%+v", i, opened[i], status)
		}
		identity, ok := result.Identity(status.SourceId)
		if !ok {
			t.Fatal("source identity missing")
		}
		if err := identity.Validate(); err != nil {
			t.Fatal(err)
		}
		if identity.SourceId != status.SourceId || identity.ContentSha256 != fixture.Digest || identity.OriginPath != plans[i].OriginPath || identity.FileName != fixture.File || identity.SizeBytes != fixture.Size || identity.NewlineCount != fixture.Newlines || !identity.EndsWithNewline || identity.LineEnding != core.LineEndingLf || identity.FormatKey != fixture.Format || identity.FormatVersion != nil || identity.RecordCount == nil || *identity.RecordCount != fixture.Read {
			t.Fatalf("identity=%+v", identity)
		}
		for category, want := range map[core.ImportCategory]int64{core.ImportCategoryRead: fixture.Read, core.ImportCategorySucceeded: fixture.Succeeded, core.ImportCategoryFailed: fixture.Failed} {
			if got, ok := status.Counts.Count(category); !ok || got != want {
				t.Errorf("%s=%d present=%t, want %d", category, got, ok, want)
			}
		}
		if status.FailureCount != fixture.Failed || len(status.Failures) != int(fixture.Failed) {
			t.Fatalf("diagnostics=%+v", status)
		}
		publication, ok := result.Publication(status.SourceId)
		if fixture.State == core.PublicationStateWithheld {
			if ok || status.WithheldReason != core.WithheldReasonIdentifierCollision {
				t.Fatalf("collision published: %+v", status)
			}
			continue
		}
		if !ok || len(publication.Records()) != int(fixture.Succeeded) {
			t.Fatalf("records=%+v present=%t", publication.Records(), ok)
		}
		lines := strings.Split(strings.TrimSuffix(inputs[plans[i].OriginPath], "\n"), "\n")
		for _, record := range publication.Records() {
			text, ok := result.RawText(record.Locator.RecordRawTextRef)
			if !ok || text != lines[*record.Locator.LineNumber-1] || text != record.RawText {
				t.Fatalf("record text=%q present=%t", text, ok)
			}
		}
		for _, failure := range status.Failures {
			text, ok := result.RawText(failure.RawTextRef)
			if !ok || text != lines[*failure.LineNumber-1] {
				t.Fatalf("failure text=%q present=%t", text, ok)
			}
			if failure.DiagnosisClass != core.DiagnosisClassUndetermined || failure.RecordRef == nil || failure.ParserVersion == "" || failure.SanitizedMessage == "" {
				t.Fatalf("incomplete diagnosis=%+v", failure)
			}
		}
		*identity.RecordCount = 999
		*identity.ObservedRangeFirst.RawText = "changed"
		again, ok := result.Identity(status.SourceId)
		if !ok || *again.RecordCount != fixture.Read || *again.ObservedRangeFirst.RawText == "changed" {
			t.Fatal("identity mutation escaped")
		}
	}
	for _, lookup := range []pipeline.ImportResult{result, {}} {
		if text, ok := lookup.RawText("absent"); ok || text != "" {
			t.Fatal("missing raw text present")
		}
		if identity, ok := lookup.Identity("absent"); ok || !reflect.DeepEqual(identity, core.SourceIdentity{}) {
			t.Fatal("missing identity present")
		}
	}
	second, err := runner.Run(plans[:1])
	if err != nil {
		t.Fatal(err)
	}
	if second.Statuses()[0].SourceId == statuses[0].SourceId {
		t.Fatal("reimport reused source ID")
	}
	again, ok := result.Identity(statuses[0].SourceId)
	if !ok || again.ContentSha256 != fixtures[0].Digest {
		t.Fatal("reimport mutated previous result")
	}
}

// 進行は収集元ごとに読んだ byte 数を累計で増やし、走査と識別を終えた収集元を入力順に 1 回報告する。
func TestRunnerReportsTheProgressOfEachSource(t *testing.T) {
	fixtures := runFixtures(t)
	plans := make([]pipeline.SourcePlan, len(fixtures))
	inputs := make(map[string]string)
	for i, fixture := range fixtures {
		path := runFixtureDir + fixture.File
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inputs[path] = string(data)
		plans[i] = pipeline.SourcePlan{OriginPath: path, FileName: fixture.File, FormatKey: fixture.Format}
	}
	config := runnerConfig("")
	config.Open = func(path string) (io.ReadCloser, error) {
		// 1 byte ずつ返し、読むたびに進行が届くことを確かめる。
		return io.NopCloser(iotest.OneByteReader(strings.NewReader(inputs[path]))), nil
	}
	lastRead := make([]int64, len(plans))
	var scanned []int
	config.Progress = func(progress pipeline.SourceProgress) {
		if progress.ReadBytes < lastRead[progress.Plan] {
			t.Errorf("source %d read %d bytes after %d", progress.Plan, progress.ReadBytes,
				lastRead[progress.Plan])
		}
		lastRead[progress.Plan] = progress.ReadBytes
		if progress.Scanned {
			scanned = append(scanned, progress.Plan)
		}
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(plans); err != nil {
		t.Fatal(err)
	}
	wantScanned := make([]int, len(plans))
	for i, fixture := range fixtures {
		wantScanned[i] = i
		if lastRead[i] != fixture.Size {
			t.Errorf("source %q reported %d bytes, want the size %d", fixture.File, lastRead[i], fixture.Size)
		}
	}
	if !reflect.DeepEqual(scanned, wantScanned) {
		t.Errorf("the scanned sources arrived as %v, want %v", scanned, wantScanned)
	}
}

func TestRunnerAcceptsARegisteredFormatKeyWithoutCoreRegistration(t *testing.T) {
	const format = core.FormatKey("vendor_format_v1")
	data, err := os.ReadFile(runFixtureDir + "squid.log")
	if err != nil {
		t.Fatal(err)
	}
	config := runnerConfig(string(data))
	config.Parsers = map[core.FormatKey]pipeline.ParserFactory{
		format: func(*string) (pipeline.SourceParser, error) {
			return &renamedParser{SourceParser: pipeline.NewTestSquidParser(), format: format}, nil
		},
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{{
		OriginPath: "run/squid.log", FileName: "squid.log", FormatKey: format,
	}})
	if err != nil {
		t.Fatal(err)
	}
	status := result.Statuses()[0]
	identity, ok := result.Identity(status.SourceId)
	if !ok || identity.FormatKey != format {
		t.Fatalf("identity=%+v present=%t, want format key %q", identity, ok, format)
	}
}

// markii 形式の末尾 DOS EOF marker は失敗として位置と原文を残すが、独立して読めた
// レコードは部分公開する。
func TestRunnerPublishesMarkIIRecordsWithTrailingDOSEOFAsPartial(t *testing.T) {
	const input = markiiScanLine + "\r\n\x1a"
	runner, err := pipeline.NewRunner(runnerConfig(input))
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run([]pipeline.SourcePlan{{
		OriginPath: "markii.log", FileName: "markii.log", FormatKey: pipeline.MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	status := result.Statuses()[0]
	if status.PublicationState != core.PublicationStatePublishedPartial {
		t.Fatalf("publicationState = %q, want %q", status.PublicationState, core.PublicationStatePublishedPartial)
	}
	if got, ok := status.Counts.Count(core.ImportCategoryRead); !ok || got != 2 {
		t.Errorf("read count = %d / %t, want 2 / true", got, ok)
	}
	if got, ok := status.Counts.Count(core.ImportCategorySucceeded); !ok || got != 1 {
		t.Errorf("succeeded count = %d / %t, want 1 / true", got, ok)
	}
	if got, ok := status.Counts.Count(core.ImportCategoryFailed); !ok || got != 1 {
		t.Errorf("failed count = %d / %t, want 1 / true", got, ok)
	}
	if len(status.Failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(status.Failures))
	}
	failure := status.Failures[0]
	if failure.DiagnosisClass != core.DiagnosisClassInconsistentInputConfirmed || failure.Stage != core.FailureStageTokenize {
		t.Fatalf("failure = %+v", failure)
	}
	if failure.RecordRef == nil || failure.RecordRef.PositionKind != core.PositionKindLineNumber || failure.RecordRef.LineNumber == nil || *failure.RecordRef.LineNumber != 2 {
		t.Fatalf("failure recordRef = %+v", failure.RecordRef)
	}
	if raw, ok := result.RawText(failure.RawTextRef); !ok || raw != "\x1a" {
		t.Errorf("failure raw text = %q / %t, want marker / true", raw, ok)
	}
	publication, ok := result.Publication(status.SourceId)
	if !ok || len(publication.Records()) != 1 {
		t.Fatalf("published records = %d / %t, want 1 / true", len(publication.Records()), ok)
	}
}

func TestNewRunnerDependencies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*pipeline.Config)
	}{
		{"open", func(c *pipeline.Config) { c.Open = nil }},
		{"parsers_nil", func(c *pipeline.Config) { c.Parsers = nil }},
		{"parsers_empty", func(c *pipeline.Config) { c.Parsers = map[core.FormatKey]pipeline.ParserFactory{} }},
		{"parser_factory", func(c *pipeline.Config) { c.Parsers[pipeline.SquidFormatKey] = nil }},
		{"parser_key", func(c *pipeline.Config) { c.Parsers[""] = pipeline.TestingFormatRegistry[pipeline.SquidFormatKey] }},
		{"minter", func(c *pipeline.Config) { c.Minter = nil }},
		{"typed_nil_minter", func(c *pipeline.Config) { c.Minter = (*pipeline.DigestMinter)(nil) }},
		{"ordinals", func(c *pipeline.Config) { c.Ordinals = nil }},
		{"sanitize", func(c *pipeline.Config) { c.Sanitize = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := runnerConfig("")
			tc.change(&config)
			if runner, err := pipeline.NewRunner(config); err == nil || runner != nil {
				t.Fatalf("runner=%v error=%v", runner, err)
			}
		})
	}
	config := runnerConfig("")
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	delete(config.Parsers, pipeline.SquidFormatKey)
	result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "empty.log", FileName: "empty.log", FormatKey: pipeline.SquidFormatKey}})
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := result.Identity(result.Statuses()[0].SourceId)
	if !ok || identity.RecordCount == nil || *identity.RecordCount != 0 {
		t.Fatalf("empty source=%+v", identity)
	}
}

type runReadCloser struct {
	io.Reader
	closed   bool
	closeErr error
}

func TestRunnerNamesSecondSourceOnOpenFailure(t *testing.T) {
	cause := errors.New("permission denied")
	config := runnerConfig("")
	var opened []string
	config.Open = func(path string) (io.ReadCloser, error) {
		opened = append(opened, path)
		if path == "second.log" {
			return nil, cause
		}
		return io.NopCloser(strings.NewReader("")), nil
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run([]pipeline.SourcePlan{
		{OriginPath: "first.log", FileName: "first.log", FormatKey: pipeline.SquidFormatKey},
		{OriginPath: "second.log", FileName: "second.log", FormatKey: pipeline.SquidFormatKey},
	})
	if !reflect.DeepEqual(opened, []string{"first.log", "second.log"}) {
		t.Fatalf("opened=%q", opened)
	}
	if !errors.Is(err, cause) || err.Error() != "importing source \"second.log\": permission denied" {
		t.Fatalf("error=%v", err)
	}
}

func (r *runReadCloser) Close() error { r.closed = true; return r.closeErr }

func TestRunnerReadAndCloseFailures(t *testing.T) {
	cause := errors.New("source unavailable")
	for _, mode := range []string{"open", "read", "close", "both", "nil_reader"} {
		t.Run(mode, func(t *testing.T) {
			input := &runReadCloser{Reader: strings.NewReader("source")}
			if mode == "read" || mode == "both" {
				input.Reader = iotest.ErrReader(cause)
			}
			if mode == "close" || mode == "both" {
				input.closeErr = cause
			}
			config := runnerConfig("")
			config.Open = func(path string) (io.ReadCloser, error) {
				if path != "input.log" {
					t.Fatalf("open path=%q", path)
				}
				if mode == "open" {
					return nil, cause
				}
				if mode == "nil_reader" {
					return nil, nil
				}
				return input, nil
			}
			runner, err := pipeline.NewRunner(config)
			if err != nil {
				t.Fatal(err)
			}
			result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "input.log", FileName: "input.log", FormatKey: pipeline.SquidFormatKey}})
			if err == nil || len(result.Statuses()) != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if mode != "nil_reader" && !errors.Is(err, cause) {
				t.Fatalf("cause lost: %v", err)
			}
			if input.closed != (mode != "open" && mode != "nil_reader") {
				t.Fatalf("closed=%t", input.closed)
			}
		})
	}
}

type stoppedRunParser struct {
	pipeline.SourceParser
	scannedBytes int64
}

func (p *stoppedRunParser) Reset(input io.Reader) {
	p.SourceParser.Reset(io.MultiReader(io.LimitReader(input, p.scannedBytes), iotest.ErrReader(errors.New("synthetic scan interruption"))))
}

func TestRunnerStoppedScanKeepsRecordsAndDiagnosis(t *testing.T) {
	manifest, err := os.ReadFile(runFixtureDir + "stopped.json")
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Size, ScannedBytes, Succeeded, Failed, FailureCount int64
		Digest, EmptyRaw                                    string
		Stages                                              []core.FailureStage
	}
	if err := json.Unmarshal(manifest, &want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(runFixtureDir + "partial.log")
	if err != nil {
		t.Fatal(err)
	}
	config := runnerConfig(string(data))
	config.Parsers[pipeline.SquidFormatKey] = func(*string) (pipeline.SourceParser, error) {
		return &stoppedRunParser{pipeline.NewTestSquidParser(), want.ScannedBytes}, nil
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "partial.log", FileName: "partial.log", FormatKey: pipeline.SquidFormatKey}})
	if err != nil {
		t.Fatal(err)
	}
	status := result.Statuses()[0]
	identity, ok := result.Identity(status.SourceId)
	if !ok || identity.RecordCount != nil || identity.SizeBytes != want.Size || identity.ContentSha256 != want.Digest {
		t.Fatalf("identity=%+v", identity)
	}
	if _, ok := status.Counts.Count(core.ImportCategoryRead); ok {
		t.Fatal("stopped read count present")
	}
	if status.PublicationState != core.PublicationStatePublishedPartial || status.Scope.RangeKind != core.RangeKindWholeSource || status.FailureCount != want.FailureCount || len(status.Failures) != len(want.Stages) {
		t.Fatalf("status=%+v", status)
	}
	publication, ok := result.Publication(status.SourceId)
	if !ok || len(publication.Records()) != int(want.Succeeded) {
		t.Fatalf("records=%+v published=%t", publication.Records(), ok)
	}
	for i, stage := range want.Stages {
		if status.Failures[i].Stage != stage {
			t.Fatalf("failure=%+v want stage=%s", status.Failures[i], stage)
		}
	}
	for category, count := range map[core.ImportCategory]int64{core.ImportCategorySucceeded: want.Succeeded, core.ImportCategoryFailed: want.Failed} {
		if got, ok := status.Counts.Count(category); !ok || got != count {
			t.Fatalf("%s=%d present=%t", category, got, ok)
		}
	}
	if text, ok := result.RawText(status.Failures[0].RawTextRef); !ok || text != want.EmptyRaw {
		t.Fatalf("empty raw=%q present=%t", text, ok)
	}
}

type failingRunIdentity struct {
	digest string
	t      *testing.T
	mode   string
	cause  error
	calls  []string
}

func (f *failingRunIdentity) Next(path, hash string) (int64, error) {
	f.calls = append(f.calls, "ordinal")
	f.checkSource(path, hash)
	if f.mode == "ordinal" {
		return 0, f.cause
	}
	return 3, nil
}

func (f *failingRunIdentity) SourceId(path, hash string, ordinal int64) (string, error) {
	f.calls = append(f.calls, "source")
	f.checkSource(path, hash)
	if ordinal != 3 {
		f.t.Fatalf("ordinal=%d", ordinal)
	}
	if f.mode == "source" {
		return "", f.cause
	}
	if f.mode == "empty_source" {
		return "", nil
	}
	return "issued-source", nil
}

func (f *failingRunIdentity) ParserVersion(parser pipeline.ParserIdentity, revision, settings string) (string, error) {
	f.calls = append(f.calls, "version")
	if !reflect.DeepEqual(parser, pipeline.NewTestSquidParser().Identity()) ||
		revision != "test-revision" || settings != "test-settings" {
		f.t.Fatalf("version args=%+v %q %q", parser, revision, settings)
	}
	if f.mode == "version" {
		return "", f.cause
	}
	if f.mode == "empty_version" {
		return "", nil
	}
	return "issued-version", nil
}

func (f *failingRunIdentity) AnalysisRunRef(manifest []pipeline.RunManifestEntry) (string, error) {
	f.calls = append(f.calls, "run")
	if !reflect.DeepEqual(manifest, []pipeline.RunManifestEntry{{SourceId: "issued-source", ParserVersion: "issued-version"}}) {
		f.t.Fatalf("manifest=%+v", manifest)
	}
	if f.mode == "run" {
		return "", f.cause
	}
	if f.mode == "empty_run" {
		return "", nil
	}
	return "issued-run", nil
}

func (f *failingRunIdentity) checkSource(path, hash string) {
	f.t.Helper()
	if path != "source.log" || hash != f.digest {
		f.t.Fatalf("source args=%q %q", path, hash)
	}
}

func TestRunnerIdentityDependencies(t *testing.T) {
	fixture := runFixtures(t)[1]
	data, err := os.ReadFile(runFixtureDir + "squid.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode  string
		calls []string
	}{
		{"ordinal", []string{"ordinal"}},
		{"source", []string{"ordinal", "source"}},
		{"version", []string{"ordinal", "source", "version"}},
		{"run", []string{"ordinal", "source", "version", "run"}},
		{"empty_source", []string{"ordinal", "source", "version"}},
		{"empty_version", []string{"ordinal", "source", "version"}},
		{"empty_run", []string{"ordinal", "source", "version", "run"}},
		{"success", []string{"ordinal", "source", "version", "run"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			fake := &failingRunIdentity{t: t, mode: tc.mode, digest: fixture.Digest, cause: errors.New("identity dependency failed")}
			config := runnerConfig(string(data))
			config.Minter, config.Ordinals = fake, fake
			runner, err := pipeline.NewRunner(config)
			if err != nil {
				t.Fatal(err)
			}
			result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "source.log", FileName: "source.log", FormatKey: pipeline.SquidFormatKey}})
			if !reflect.DeepEqual(fake.calls, tc.calls) {
				t.Fatalf("calls=%v want=%v", fake.calls, tc.calls)
			}
			if tc.mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if status := result.Statuses()[0]; status.SourceId != "issued-source" || status.AnalysisRunRef != "issued-run" {
					t.Fatalf("status=%+v", status)
				}
				return
			}
			if err == nil || len(result.Statuses()) != 0 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if !strings.HasPrefix(tc.mode, "empty_") && !errors.Is(err, fake.cause) {
				t.Fatalf("cause lost: %v", err)
			}
		})
	}
}

func TestRunnerRejectsInvalidPlansAndParserResults(t *testing.T) {
	plan := pipeline.SourcePlan{OriginPath: "source.log", FileName: "source.log", FormatKey: pipeline.SquidFormatKey}
	for _, mode := range []string{"unsupported", "nil_parser", "wrong_parser", "invalid_path"} {
		t.Run(mode, func(t *testing.T) {
			config := runnerConfig("broken\n\n")
			inputPlan := plan
			opened := 0
			open := config.Open
			config.Open = func(path string) (io.ReadCloser, error) { opened++; return open(path) }
			switch mode {
			case "unsupported":
				inputPlan.FormatKey = core.FormatKey("unregistered_format")
			case "nil_parser":
				config.Parsers[plan.FormatKey] = func(*string) (pipeline.SourceParser, error) { return nil, nil }
			case "wrong_parser":
				config.Parsers[plan.FormatKey] = pipeline.TestingFormatRegistry[pipeline.MarkIIFormatKey]
			case "invalid_path":
				inputPlan.OriginPath = "/absolute"
			}
			runner, err := pipeline.NewRunner(config)
			if err != nil {
				t.Fatal(err)
			}
			result, err := runner.Run([]pipeline.SourcePlan{inputPlan})
			if err == nil || len(result.Statuses()) != 0 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if (mode == "unsupported" || mode == "nil_parser") && opened != 0 {
				t.Fatalf("opened=%d before parser validation", opened)
			}
		})
	}
	for _, runner := range []*pipeline.Runner{nil, {}} {
		if result, err := runner.Run([]pipeline.SourcePlan{plan}); err == nil || len(result.Statuses()) != 0 {
			t.Fatalf("zero runner result=%+v error=%v", result, err)
		}
	}
}

func TestRunnerObservedRange(t *testing.T) {
	data, err := os.ReadFile(runFixtureDir + "times.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Input, First, Last string
		Format                   core.FormatKey
		Failures                 int64
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	// 収録範囲は行の並びでなく時刻で決まる。その形を通す case を名前で確かめる。
	wanted := map[string]bool{
		"time_order_against_line_order":  false,
		"failed_record_widens_the_range": false,
		"no_readable_time":               false,
	}
	for _, tc := range cases {
		if _, listed := wanted[tc.Name]; listed {
			wanted[tc.Name] = true
		}
	}
	for name, present := range wanted {
		if !present {
			t.Fatalf("the time cases carry no case named %q", name)
		}
	}
	if len(cases) == 0 {
		t.Fatal("time cases missing")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			runner, err := pipeline.NewRunner(runnerConfig(tc.Input))
			if err != nil {
				t.Fatal(err)
			}
			result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "run/source.log", FileName: "source.log", FormatKey: tc.Format}})
			if err != nil {
				t.Fatal(err)
			}
			status := result.Statuses()[0]
			identity, ok := result.Identity(status.SourceId)
			if !ok || status.FailureCount != tc.Failures {
				t.Fatalf("identity present=%t failures=%d", ok, status.FailureCount)
			}
			if err := identity.Validate(); err != nil {
				t.Fatal(err)
			}
			for name, pair := range map[string]struct {
				got  *core.Timestamp
				want string
			}{
				"first": {identity.ObservedRangeFirst, tc.First}, "last": {identity.ObservedRangeLast, tc.Last},
			} {
				if pair.want == "" {
					if pair.got != nil {
						t.Errorf("%s time present: %+v", name, pair.got)
					}
				} else if pair.got == nil {
					t.Errorf("%s time missing; want %s", name, pair.want)
				} else if got, ok := pair.got.NormalizedValue(); !ok || got != pair.want {
					t.Errorf("%s time=%q present=%t, want %q", name, got, ok, pair.want)
				}
			}
		})
	}
}

type duplicateRunMinter struct {
	pipeline.DigestMinter
	runCalls int
}

func (*duplicateRunMinter) SourceId(string, string, int64) (string, error) { return "duplicate", nil }
func (m *duplicateRunMinter) AnalysisRunRef(entries []pipeline.RunManifestEntry) (string, error) {
	m.runCalls++
	return "run", nil
}

func TestRunnerRejectsDuplicateSourceIdentity(t *testing.T) {
	minter := &duplicateRunMinter{}
	config := runnerConfig("")
	config.Minter = minter
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{
		{OriginPath: "one.log", FileName: "one.log", FormatKey: pipeline.SquidFormatKey},
		{OriginPath: "two.log", FileName: "two.log", FormatKey: pipeline.SquidFormatKey},
	})
	if err == nil {
		t.Errorf("duplicate source identity accepted: %+v", result.Statuses())
	}
	if minter.runCalls != 0 {
		t.Errorf("analysis run minted after duplicate identity: calls=%d", minter.runCalls)
	}
	if _, ok := result.Identity("duplicate"); ok {
		t.Error("ambiguous source identity returned")
	}
}

func TestRunnerCombinedMarkIIParserVersion(t *testing.T) {
	data, err := os.ReadFile(runFixtureDir + "parser-version.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ParserId, SupportedFormatVersion, Revision, Settings, ParserVersion, Input string
		FailureCount                                                               int
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	config := runnerConfig(fixture.Input)
	config.Revision, config.SettingsDigest = fixture.Revision, fixture.Settings
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{{OriginPath: "markii.log", FileName: "markii.log", FormatKey: pipeline.MarkIIFormatKey}})
	if err != nil {
		t.Fatal(err)
	}
	identity := pipeline.NewTestMarkIIParser().Identity()
	if identity.ParserID != fixture.ParserId || identity.SupportedFormatVersion != fixture.SupportedFormatVersion {
		t.Fatalf("parser identity=%+v", identity)
	}
	failures := result.Statuses()[0].Failures
	if len(failures) != fixture.FailureCount {
		t.Fatalf("failures=%+v", failures)
	}
	for _, failure := range failures {
		if failure.ParserVersion != fixture.ParserVersion {
			t.Errorf("parser version=%s want=%s", failure.ParserVersion, fixture.ParserVersion)
		}
	}
}
