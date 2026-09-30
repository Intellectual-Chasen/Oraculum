// in-package test: 公開前の走査材料の件数と byte 数の突き合わせを直接検査する。
package pipeline

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"reflect"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestFinishScanCounts(t *testing.T) {
	data, err := os.ReadFile("testdata/scan/counts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name         string
		Succeeded    int64
		Failed       int64
		ReadStopped  bool
		Failures     []core.DiagnosisClass
		Read         *int64
		FailureCount int64
		Diagnoses    []int64
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("scan count manifest is empty")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			source := scannedSource{ReadStopped: tc.ReadStopped}
			for _, class := range tc.Failures {
				source.Failures = append(source.Failures, failedRecord{Failure: core.ImportFailure{DiagnosisClass: class}})
			}
			if err := finishScanCounts(&source, tc.Succeeded, tc.Failed); err != nil {
				t.Fatal(err)
			}
			read, present := source.Counts.Count(core.ImportCategoryRead)
			if present != (tc.Read != nil) || (tc.Read != nil && read != *tc.Read) {
				t.Errorf("read = %d, present = %t, want %v", read, present, tc.Read)
			}
			for category, want := range map[core.ImportCategory]int64{core.ImportCategorySucceeded: tc.Succeeded, core.ImportCategoryFailed: tc.Failed} {
				if got, present := source.Counts.Count(category); !present || got != want {
					t.Errorf("%s = %d / %t, want %d", category, got, present, want)
				}
			}
			if source.FailureCount != tc.FailureCount || len(source.DiagnosisCounts) != len(tc.Diagnoses) {
				t.Fatalf("failure count = %d, diagnoses = %+v", source.FailureCount, source.DiagnosisCounts)
			}
			for index, count := range source.DiagnosisCounts {
				if count.Count != tc.Diagnoses[index] {
					t.Errorf("diagnosis %s = %d, want %d", count.DiagnosisClass, count.Count, tc.Diagnoses[index])
				}
			}
		})
	}
}

func TestScanSourcePreservesFailureRawText(t *testing.T) {
	const raw = "broken"
	input := raw + "\n"
	measurement, err := measureWholeSource(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	plan := SourcePlan{FileName: "synthetic.log", FormatKey: SquidFormatKey}
	source, err := scanSource(NewTestSquidParser(), strings.NewReader(input), measurement, plan)
	if err != nil || len(source.Failures) != 1 {
		t.Fatalf("failures=%d error=%v", len(source.Failures), err)
	}
	if source.Failures[0].RawText == nil || *source.Failures[0].RawText != raw {
		t.Fatalf("scanned failure does not retain raw text %q", raw)
	}
}

func TestScanSourceSequenceScope(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sequences []string
		from, to  int64
	}{
		{"middle_maximum", []string{"7", "9", "8"}, 7, 9},
		{"descending", []string{"7", "6"}, 6, 7},
		{"ascending", []string{"7", "8", "9"}, 7, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input string
			for _, sequence := range tc.sequences {
				input += "01/02/2024 03:04:05.006 +0000 sn=" + sequence + " evt=other subEvt=other tmid=t\n"
			}
			measurement, err := measureWholeSource(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			plan := SourcePlan{FileName: "synthetic.log", FormatKey: MarkIIFormatKey}
			source, err := scanSource(NewTestMarkIIParser(), strings.NewReader(input), measurement, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(source.Records) != len(tc.sequences) || len(source.Failures) != 0 {
				t.Fatalf("records=%d failures=%d", len(source.Records), len(source.Failures))
			}
			if source.Scope.FromPosition == nil || source.Scope.ToPosition == nil {
				t.Fatalf("missing scope bounds: %+v", source.Scope)
			}
			if *source.Scope.FromPosition != tc.from || *source.Scope.ToPosition != tc.to {
				t.Errorf("scope=%d..%d, want %d..%d", *source.Scope.FromPosition, *source.Scope.ToPosition, tc.from, tc.to)
			}
			for _, record := range source.Records {
				sequence := *record.Locator.SequenceNumber
				if sequence < *source.Scope.FromPosition || sequence > *source.Scope.ToPosition {
					t.Errorf("scope excludes sn=%d", sequence)
				}
			}
			status, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize, settleRawTextRef)
			if err != nil {
				t.Fatalf("building status: %v", err)
			}
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFinishScanCountsRejectsNegativeCount(t *testing.T) {
	if err := finishScanCounts(&scannedSource{}, -1, 0); err == nil {
		t.Fatal("negative count was accepted")
	}
}

func TestScanByteReaderAndSizeFailure(t *testing.T) {
	for _, input := range []string{"abc", "ab", "abcd"} {
		t.Run(input, func(t *testing.T) {
			reader := &scanByteReader{input: strings.NewReader(input)}
			data, err := io.ReadAll(reader)
			if err != nil || string(data) != input || reader.bytes != int64(len(input)) {
				t.Fatalf("read = %q, bytes = %d, error = %v", data, reader.bytes, err)
			}
			failure := scanSizeFailure(3, reader.bytes)
			if input == "abc" {
				if failure != nil {
					t.Errorf("equal size produced failure: %+v", failure)
				}
				return
			}
			if failure == nil || failure.Stage != core.FailureStageRead || failure.DiagnosisClass != core.DiagnosisClassUndetermined || failure.ByteOffset == nil || *failure.ByteOffset != int64(len(input)) || failure.RecordRef != nil {
				t.Errorf("size failure = %+v", failure)
			}
		})
	}
	cause := errors.New("source interrupted")
	reader := &scanByteReader{input: io.MultiReader(strings.NewReader("abc"), iotest.ErrReader(cause))}
	data, err := io.ReadAll(reader)
	if string(data) != "abc" || reader.bytes != 3 || !errors.Is(err, cause) {
		t.Errorf("interrupted read = %q, bytes = %d, error = %v", data, reader.bytes, err)
	}
}

func TestScanBoundsRequireBothEndpoints(t *testing.T) {
	first, last := int64(2), int64(5)
	cases := []struct {
		name       string
		positions  []*int64
		stopped    bool
		positioned bool
	}{
		{"empty", nil, false, false},
		{"known", []*int64{&first, &last}, false, true},
		{"unknown_first", []*int64{nil, &last}, false, false},
		{"unknown_last", []*int64{&first, nil}, false, false},
		{"unknown_middle", []*int64{&first, nil, &last}, false, true},
		{"stopped", []*int64{&first, &last}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bounds scanBounds
			for _, position := range tc.positions {
				bounds.observe(position)
			}
			scope := core.RecordRange{SourceId: "import-1", SourceContentSha256: "content-1"}
			bounds.apply(&scope, core.PositionKindSequenceNumber, tc.stopped)
			if scope.SourceId != "import-1" || scope.SourceContentSha256 != "content-1" {
				t.Errorf("scope source identity changed: %+v", scope)
			}
			if tc.positioned {
				if scope.RangeKind != core.RangeKindPositioned || scope.PositionKind != core.PositionKindSequenceNumber || scope.FromPosition == nil || scope.ToPosition == nil || *scope.FromPosition != 2 || *scope.ToPosition != 5 {
					t.Errorf("positioned scope = %+v", scope)
				}
			} else if scope.RangeKind != core.RangeKindWholeSource || scope.PositionKind != "" || scope.FromPosition != nil || scope.ToPosition != nil {
				t.Errorf("whole-source scope = %+v", scope)
			}
		})
	}
}

func TestScanSource(t *testing.T) {
	data, err := os.ReadFile("testdata/scan/import.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name         string
		Parser       string
		Input        string
		ReadError    bool
		SizeDelta    int64
		Succeeded    int64
		Failed       int64
		Read         *int64
		FailureCount int64
		Records      int
		Stages       []core.FailureStage
		Scope        core.RangeKind
		PositionKind core.PositionKind
		FromPosition *int64
		ToPosition   *int64
		RawTexts     []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("source scan manifest is empty")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			parser := NewTestSquidParser()
			if tc.Parser == "markii" {
				parser = NewTestMarkIIParser()
			}
			if tc.Parser == "bare_error" {
				parser = &scanErrorParser{}
			}
			measurement, err := measureWholeSource(strings.NewReader(tc.Input))
			if err != nil {
				t.Fatal(err)
			}
			measurement.SizeBytes += tc.SizeDelta
			plan := SourcePlan{OriginPath: "fixture/source.log", FileName: "source.log", FormatKey: parser.Identity().FormatKey}
			var input io.Reader = strings.NewReader(tc.Input)
			cause := errors.New("source interrupted")
			if tc.ReadError {
				input = io.MultiReader(input, iotest.ErrReader(cause))
			}
			source, err := scanSource(parser, input, measurement, plan)
			if fake, ok := parser.(*scanErrorParser); ok && fake.read != tc.Input {
				t.Errorf("parser read %q, want %q", fake.read, tc.Input)
			}
			if (err != nil) != (tc.ReadError || tc.SizeDelta != 0) || (tc.ReadError && !errors.Is(err, cause)) {
				t.Fatalf("scan error = %v", err)
			}
			if source.Plan != plan || source.Measurement != measurement ||
				!reflect.DeepEqual(source.Parser, parser.Identity()) ||
				source.ReadStopped != tc.ReadError {
				t.Errorf("scan metadata = %+v", source)
			}
			for category, want := range map[core.ImportCategory]int64{core.ImportCategorySucceeded: tc.Succeeded, core.ImportCategoryFailed: tc.Failed} {
				if got, present := source.Counts.Count(category); !present || got != want {
					t.Errorf("%s = %d / %t, want %d", category, got, present, want)
				}
			}
			read, present := source.Counts.Count(core.ImportCategoryRead)
			if present != (tc.Read != nil) || (tc.Read != nil && read != *tc.Read) {
				t.Errorf("read = %d / %t, want %v", read, present, tc.Read)
			}
			if source.FailureCount != tc.FailureCount || len(source.Records) != tc.Records || len(source.Failures) != len(tc.Stages) {
				t.Fatalf("records = %d, failures = %d / %d", len(source.Records), len(source.Failures), source.FailureCount)
			}
			if source.Scope.RangeKind != tc.Scope || source.Scope.PositionKind != tc.PositionKind || source.Scope.SourceId != "" || source.Scope.SourceContentSha256 != measurement.ContentSha256 {
				t.Errorf("scope = %+v", source.Scope)
			}
			if !equalScanPosition(source.Scope.FromPosition, tc.FromPosition) || !equalScanPosition(source.Scope.ToPosition, tc.ToPosition) {
				t.Errorf("scope endpoints = %+v, want %v / %v", source.Scope, tc.FromPosition, tc.ToPosition)
			}
			for index, failed := range source.Failures {
				failure := failed.Failure
				if (failure.RecordRef != nil) != (failed.RawText != nil) {
					t.Errorf("failure raw text presence differs from record position: %+v", failed)
				}
				if failure.Stage != tc.Stages[index] || failure.SourceContentSha256 != measurement.ContentSha256 || failure.SourceId != "" || failure.RawTextRef != "" || (failure.RecordRef == nil) != (failure.Stage == core.FailureStageRead) {
					t.Errorf("failure = %+v", failure)
				}
			}
			if len(source.Records) != len(tc.RawTexts) {
				t.Fatalf("raw text count = %d, want %d", len(source.Records), len(tc.RawTexts))
			}
			for index, record := range source.Records {
				if record.Locator.SourceId != "" || record.Locator.RecordRawTextRef != "" || record.Locator.SourceContentSha256 != measurement.ContentSha256 || record.Locator.SourceFileName != plan.FileName || record.RawText != tc.RawTexts[index] {
					t.Errorf("record = %+v", record)
				}
			}
			var diagnosed int64
			for _, count := range source.DiagnosisCounts {
				diagnosed += count.Count
			}
			if diagnosed != tc.FailureCount {
				t.Errorf("diagnosis count = %d, want %d", diagnosed, tc.FailureCount)
			}
		})
	}
}

func equalScanPosition(actual, expected *int64) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return *actual == *expected
}

func TestScanSourceRejectsMissingInputsAndWrongFormat(t *testing.T) {
	plan := SourcePlan{FormatKey: MarkIIFormatKey}
	for _, tc := range []struct {
		parser SourceParser
		input  io.Reader
	}{
		{nil, strings.NewReader("")},
		{NewTestMarkIIParser(), nil},
		{NewTestSquidParser(), strings.NewReader("")},
	} {
		if _, err := scanSource(tc.parser, tc.input, sourceMeasurement{}, plan); err == nil {
			t.Error("invalid scan input was accepted")
		}
	}
}

type scanErrorParser struct {
	input io.Reader
	read  string
}

func (*scanErrorParser) Identity() ParserIdentity {
	return ParserIdentity{ParserID: "test-read-error", FormatKey: SquidFormatKey, PositionKind: core.PositionKindLineNumber}
}

func (p *scanErrorParser) Reset(input io.Reader) {
	p.input = input
	p.read = ""
}

func (p *scanErrorParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	data, err := io.ReadAll(p.input)
	p.read = string(data)
	return ParsedRecord{}, nil, err
}

func TestScanSourceRejectsSuccessfulRecordWithoutPosition(t *testing.T) {
	parser := &scanErrorParser{}
	plan := SourcePlan{FormatKey: parser.Identity().FormatKey}
	_, err := scanSource(parser, strings.NewReader("abc"), sourceMeasurement{SizeBytes: 3}, plan)
	if err == nil || parser.read != "abc" {
		t.Errorf("positionless success: read %q, error %v", parser.read, err)
	}
}
