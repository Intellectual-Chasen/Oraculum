// in-package test: 非公開の状態確定処理を直接検査する。
package pipeline

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func settleSource(t *testing.T, failures int64) scannedSource {
	t.Helper()
	counts, err := core.NewImportCountSet(core.ImportCount{Category: core.ImportCategoryFailed, Count: 0})
	if err != nil {
		t.Fatal(err)
	}
	s := scannedSource{
		Plan:        SourcePlan{FileName: "synthetic.log"},
		Parser:      ParserIdentity{PositionKind: core.PositionKindLineNumber},
		Measurement: sourceMeasurement{ContentSha256: strings.Repeat("a", 64)},
		Scope:       core.RecordRange{RangeKind: core.RangeKindWholeSource}, Counts: counts,
		FailureCount: failures,
	}
	if failures > 0 {
		s.DiagnosisCounts = []core.DiagnosisCount{{DiagnosisClass: core.DiagnosisClassUndetermined, Count: failures}}
	}
	for range failures {
		s.Failures = append(s.Failures, failedRecord{Failure: core.ImportFailure{
			DiagnosisClass: core.DiagnosisClassUndetermined, Stage: core.FailureStageRead,
			Interpretation: "UTF-8", ExpectedMeaning: "complete source", ObservedResult: "read\nstopped\x1b",
			UnresolvedReason: "cause not established",
		}})
	}
	return s
}

func TestBuildImportStatusSuppliesRawTextReference(t *testing.T) {
	s := settleSource(t, 1)
	ref := settleRecord().Locator
	ref.SourceContentSha256 = s.Measurement.ContentSha256
	s.Failures[0].Failure.Stage = core.FailureStageNormalize
	s.Failures[0].Failure.RecordRef = &ref
	s.Failures[0].Failure.LineNumber = clonePointer(ref.LineNumber)
	raw := "failed synthetic record"
	s.Failures[0].RawText = &raw
	counts, err := core.NewImportCountSet(core.ImportCount{Category: core.ImportCategoryFailed, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.Counts = counts
	expected := cloneLocator(ref)
	expected.SourceId = "source"
	calls := 0
	status, err := buildImportStatus(s, "run", "parser-v1", "source", settleSanitize,
		func(locator core.RecordLocator, rawText string) string {
			calls++
			if rawText != "failed synthetic record" {
				t.Fatalf("supplier raw text = %q", rawText)
			}
			if !reflect.DeepEqual(locator, expected) {
				t.Fatalf("supplier argument = %+v, want %+v", locator, expected)
			}
			*locator.LineNumber = 9
			return "raw:synthetic:1"
		})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || status.Failures[0].RawTextRef != "raw:synthetic:1" {
		t.Fatalf("calls=%d failure=%+v", calls, status.Failures[0])
	}
	if s.Failures[0].Failure.RawTextRef != "" || s.Failures[0].Failure.RecordRef.SourceId != "" || s.Failures[0].Failure.RecordRef.RecordRawTextRef != "" || *s.Failures[0].Failure.RecordRef.LineNumber != 1 || *status.Failures[0].RecordRef.LineNumber != 1 {
		t.Fatal("supplier mutated evidence")
	}
	if _, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", settleRawTextRef); err != nil {
		t.Fatalf("settling supplied reference: %v", err)
	}
	for _, supplier := range []func(core.RecordLocator, string) string{nil, func(core.RecordLocator, string) string { return "" }} {
		if _, err := buildImportStatus(s, "run", "parser-v1", "source", settleSanitize, supplier); err == nil {
			t.Fatal("missing raw reference accepted")
		}
	}
}

func TestBuildImportStatusPreservesEmptyLine(t *testing.T) {
	const good = "192.0.2.1 - - [02/Jan/2024:03:04:05 +0000] \"GET http://example.test/ HTTP/1.1\" 200 1 \"-\" \"agent\" TCP_MISS:HIER_DIRECT\n"
	source := settleScannedSquid(t, good+"\nx\n")
	if len(source.Records) != 1 || len(source.Failures) != 2 {
		t.Fatalf("records=%d failures=%d", len(source.Records), len(source.Failures))
	}
	stored := make(map[string]string)
	calls := 0
	status, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize,
		func(locator core.RecordLocator, rawText string) string {
			calls++
			if locator.SourceId != "source" || locator.SourceContentSha256 != source.Measurement.ContentSha256 || locator.LineNumber == nil {
				t.Fatalf("supplier locator=%+v", locator)
			}
			line := *locator.LineNumber
			if (line != 2 && line != 3) || (line == 2 && rawText != "") || (line == 3 && rawText != "x") {
				t.Fatalf("supplier line=%d raw text=%q", line, rawText)
			}
			ref := settleRawTextRef(locator, rawText)
			stored[ref] = rawText
			return ref
		})
	if err != nil {
		t.Fatalf("building status for empty source line: %v", err)
	}
	if calls != 2 || status.PublicationState != core.PublicationStatePublishedPartial || status.FailureCount != 2 {
		t.Fatalf("calls=%d state=%s failures=%d", calls, status.PublicationState, status.FailureCount)
	}
	for i, want := range []string{"", "x"} {
		if source.Failures[i].RawText == nil || *source.Failures[i].RawText != want {
			t.Fatalf("scanned failure %d does not retain raw text %q", i, want)
		}
		failure := status.Failures[i]
		ref := "raw:synthetic:" + strconv.Itoa(i+2)
		if failure.Stage != core.FailureStageTokenize || failure.RawTextRef != ref || failure.RecordRef == nil || failure.RecordRef.RecordRawTextRef != ref {
			t.Fatalf("failure=%+v", failure)
		}
		if text, exists := stored[ref]; !exists || text != want {
			t.Errorf("stored text=%q exists=%t, want %q", text, exists, want)
		}
	}
}

func TestBuildImportStatusOmitsUnpositionedRawTextReference(t *testing.T) {
	for _, stage := range []core.FailureStage{core.FailureStageRead, core.FailureStageTokenize} {
		t.Run(string(stage), func(t *testing.T) {
			source := settleSource(t, 1)
			source.Failures[0].Failure.Stage = stage
			if source.Failures[0].RawText != nil || source.Failures[0].Failure.RecordRef != nil {
				t.Fatal("unpositioned failure carries raw text or a record reference")
			}
			calls := 0
			status, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize,
				func(core.RecordLocator, string) string { calls++; return "unused" })
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 || status.Failures[0].RawTextRef != "" || status.Failures[0].RecordRef != nil {
				t.Fatalf("calls=%d failure=%+v", calls, status.Failures[0])
			}
		})
	}
}

func TestBuildImportStatusRejectsRawTextPositionMismatch(t *testing.T) {
	for _, positioned := range []bool{false, true} {
		source := settleSource(t, 1)
		source.Failures[0].Failure.Stage = core.FailureStageTokenize
		if positioned {
			ref := settleRecord().Locator
			source.Failures[0].Failure.RecordRef = &ref
			source.Failures[0].Failure.LineNumber = clonePointer(ref.LineNumber)
			counts, err := core.NewImportCountSet(core.ImportCount{Category: core.ImportCategoryFailed, Count: 1})
			if err != nil {
				t.Fatal(err)
			}
			source.Counts = counts
		} else {
			raw := "unpositioned text"
			source.Failures[0].RawText = &raw
		}
		calls := 0
		_, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize,
			func(core.RecordLocator, string) string { calls++; return "unexpected" })
		if err == nil || !strings.Contains(err.Error(), "raw text and record reference must be present together") || calls != 0 {
			t.Fatalf("positioned=%t supplier calls=%d error=%v", positioned, calls, err)
		}
	}
}

func settleSanitize(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func settleStatus(t *testing.T, s scannedSource, id string) core.ImportStatus {
	t.Helper()
	status, err := buildImportStatus(s, "run", "parser-v1", id, settleSanitize, settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func settleRawTextRef(locator core.RecordLocator, _ string) string {
	if locator.LineNumber == nil {
		return ""
	}
	return "raw:synthetic:" + strconv.FormatInt(*locator.LineNumber, 10)
}

// **取り込みの状態は失敗を全件含む。** 収集元が持つ失敗の件数と、状態が含む要素数と、
// 状態が述べる総数の 3 つが一致する。
func TestBuildImportStatusCarriesEveryFailure(t *testing.T) {
	for _, failures := range []int64{0, 1, 5} {
		t.Run(strconv.FormatInt(failures, 10), func(t *testing.T) {
			source := settleSource(t, failures)
			status, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(status.Failures) != len(source.Failures) {
				t.Errorf("the status carries %d failures, want the %d of the source",
					len(status.Failures), len(source.Failures))
			}
			if int64(len(status.Failures)) != status.FailureCount {
				t.Errorf("the status carries %d failures, want the count %d",
					len(status.Failures), status.FailureCount)
			}
		})
	}
}

func TestBuildImportStatusCompletesFailures(t *testing.T) {
	s := settleSource(t, 1)
	status := settleStatus(t, s, "source")
	failure := status.Failures[0]
	if failure.SourceId != "source" || failure.SourceContentSha256 != strings.Repeat("a", 64) || failure.ParserVersion != "parser-v1" || failure.SanitizedMessage != "Import failure at stage read; diagnosis undetermined." {
		t.Fatalf("completed failure: %+v", failure)
	}
	if strings.IndexFunc(failure.SanitizedMessage, unicode.IsControl) != -1 {
		t.Fatal("control character in message")
	}
	if failure.UnresolvedReason != "cause not established" || failure.RecordRef != nil || failure.RawTextRef != "" {
		t.Fatalf("failure evidence changed: %+v", failure)
	}
	if status.AnalysisRunRef != "run" || status.PublicationState != core.PublicationStatePublishedPartial {
		t.Fatalf("run or publication: %+v", status)
	}
	if s.Failures[0].Failure.SourceId != "" {
		t.Fatal("input failure mutated")
	}
}

func TestDiagnosticOmitsSourceText(t *testing.T) {
	s := settleSource(t, 1)
	s.Failures[0].Failure.ObservedResult = "parsing time \"private-invalid-time\": invalid syntax"
	status := settleStatus(t, s, "source")
	if strings.Contains(status.Failures[0].SanitizedMessage, "private-invalid-time") {
		t.Fatal("source text in sanitized diagnostic")
	}
	if status.Failures[0].ObservedResult != s.Failures[0].Failure.ObservedResult {
		t.Fatal("original diagnosis changed")
	}
}

func TestBuildImportStatusRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*scannedSource)
		sanitize func(string) string
	}{
		{"unsanitized", func(*scannedSource) {}, func(s string) string { return s + "\n" }},
		{"nil sanitizer", func(*scannedSource) {}, nil},
		{"read with raw reference", func(s *scannedSource) { s.Failures[0].Failure.RawTextRef = "raw:1" }, settleSanitize},
		{"read with record reference", func(s *scannedSource) { s.Failures[0].Failure.RecordRef = &core.RecordLocator{} }, settleSanitize},
		{"field map without position", func(s *scannedSource) { s.Failures[0].Failure.Stage = core.FailureStageFieldMap }, settleSanitize},
		{"missing unresolved reason", func(s *scannedSource) { s.Failures[0].Failure.UnresolvedReason = "" }, settleSanitize},
		{"missing failure", func(s *scannedSource) { s.Failures = nil }, settleSanitize},
		{"negative total", func(s *scannedSource) { s.FailureCount = -1 }, settleSanitize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settleSource(t, 1)
			tc.change(&s)
			if _, err := buildImportStatus(s, "run", "parser-v1", "source", tc.sanitize, nil); err == nil {
				t.Fatal("invalid status accepted")
			}
		})
	}
}
