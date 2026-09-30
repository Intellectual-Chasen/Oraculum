// in-package test: 非公開の構築子を通して公開結果の境界を検査する。
package pipeline

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestSettleActualSquidScan(t *testing.T) {
	const good = "192.0.2.1 - - [10/Oct/2000:13:55:36 +0000] \"GET http://example.test/ HTTP/1.1\" 200 12 \"-\" \"test\" TCP_MISS:DIRECT\n"
	const privateTime = "99/Oct/2000:13:55:36 +0000"
	bad := strings.Replace(good, "10/Oct/2000:13:55:36 +0000", privateTime, 1)
	for _, tc := range []struct {
		name, input       string
		records, failures int
		state             core.PublicationState
	}{
		{"success", good, 1, 0, core.PublicationStatePublishedFull},
		{"failure", bad, 0, 1, core.PublicationStatePublishedPartial},
		{"mixed", good + bad, 1, 1, core.PublicationStatePublishedPartial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := settleScannedSquid(t, tc.input)
			if len(source.Records) != tc.records || len(source.Failures) != tc.failures {
				t.Fatalf("unexpected scan: %+v", source)
			}
			for _, record := range source.Records {
				if record.Locator.SourceId != "" || record.Locator.RecordRawTextRef != "" {
					t.Fatal("scan prefilled identity or raw reference")
				}
			}
			for _, failed := range source.Failures {
				failure := failed.Failure
				if failure.RecordRef == nil || failure.RecordRef.SourceId != "" || failure.RecordRef.RecordRawTextRef != "" {
					t.Fatal("unexpected failure locator")
				}
				if !strings.Contains(failure.ObservedResult, privateTime) {
					t.Fatal("regression input lacks private source text")
				}
			}
			expectedTexts := strings.Split(strings.TrimSuffix(tc.input, "\n"), "\n")
			stored := make(map[string]string)
			calls := 0
			supplier := func(locator core.RecordLocator, rawText string) string {
				calls++
				if locator.SourceId != "source" || locator.SourceContentSha256 != source.Measurement.ContentSha256 || locator.SourceFileName != "synthetic.log" || locator.PositionKind != core.PositionKindLineNumber || locator.LineNumber == nil || *locator.LineNumber < 1 || *locator.LineNumber > int64(len(expectedTexts)) {
					t.Fatalf("supplier locator=%+v", locator)
				}
				if rawText != expectedTexts[*locator.LineNumber-1] {
					t.Fatalf("supplier raw text=%q, want %q", rawText, expectedTexts[*locator.LineNumber-1])
				}
				ref := settleRawTextRef(locator, rawText)
				stored[ref] = rawText
				return ref
			}
			status, err := buildImportStatus(source, "run", "parser-v1", "source", settleSanitize, supplier)
			if err != nil {
				t.Fatal(err)
			}
			result, err := newImportResult([]scannedSource{source}, []core.ImportStatus{status}, "run", supplier)
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.records+tc.failures || len(stored) != tc.records+tc.failures {
				t.Fatalf("supplier calls=%d stored texts=%d, want %d", calls, len(stored), tc.records+tc.failures)
			}
			publication, ok := result.Publication("source")
			if !ok || len(publication.Records()) != tc.records || publication.Status().PublicationState != tc.state {
				t.Fatal("scanned publication differs")
			}
			for _, record := range publication.Records() {
				if record.Locator.SourceId != "source" || record.Locator.RecordRawTextRef != "raw:synthetic:1" {
					t.Fatal("record locator was not completed")
				}
				if stored[record.Locator.RecordRawTextRef] != strings.TrimSuffix(good, "\n") {
					t.Fatal("published raw reference does not resolve to the source text")
				}
			}
			for _, failure := range publication.Status().Failures {
				if strings.Contains(failure.SanitizedMessage, privateTime) || failure.RecordRef.SourceId != "source" || failure.RawTextRef != failure.RecordRef.RecordRawTextRef {
					t.Fatalf("completed failure: %+v", failure)
				}
				if stored[failure.RawTextRef] != strings.TrimSuffix(bad, "\n") {
					t.Fatal("failure raw reference does not resolve to the source text")
				}
			}
		})
	}
}

func settleScannedSquid(t *testing.T, input string) scannedSource {
	t.Helper()
	measurement, err := measureWholeSource(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	source, err := scanSource(NewTestSquidParser(), strings.NewReader(input), measurement, SourcePlan{FileName: "synthetic.log", FormatKey: SquidFormatKey})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestImportResultPublications(t *testing.T) {
	if _, ok := (ImportResult{}).Publication("one"); ok {
		t.Fatal("zero result published")
	}
	sources := []scannedSource{settleSource(t, 0), settleSource(t, 1)}
	statuses := []core.ImportStatus{settleStatus(t, sources[0], "two"), settleStatus(t, sources[1], "one")}
	result, err := newImportResult(sources, statuses, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"two", "one"} {
		publication, ok := result.Publication(id)
		if !ok || publication.Status().PublicationState != statuses[i].PublicationState {
			t.Fatalf("publication %s: %+v, %v", id, publication.Status(), ok)
		}
		if result.Statuses()[i].SourceId != id || result.Statuses()[i].AnalysisRunRef != "run" {
			t.Fatalf("status order or run: %+v", result.Statuses())
		}
	}
	if _, ok := result.Publication("absent"); ok {
		t.Fatal("missing source published")
	}
}

func TestImportResultWithholdsRun(t *testing.T) {
	for _, mixedRun := range []bool{false, true} {
		sources := []scannedSource{settleSource(t, 0), settleSource(t, 0)}
		statuses := []core.ImportStatus{settleStatus(t, sources[0], "one"), settleStatus(t, sources[1], "one")}
		want := core.WithheldReasonIdentifierCollision
		if mixedRun {
			statuses[1].SourceId = "two"
			statuses[1].Scope.SourceId = "two"
			statuses[1].AnalysisRunRef = "other-run"
			want = core.WithheldReasonMixedAnalysisRun
		}
		result, err := newImportResult(sources, statuses, "run", settleRawTextRef)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range result.Statuses() {
			if status.PublicationState != core.PublicationStateWithheld || status.WithheldReason != want || status.AnalysisRunRef != "run" {
				t.Fatalf("status=%+v", status)
			}
			publication, ok := result.Publication(status.SourceId)
			if ok || len(publication.Records()) != 0 {
				t.Fatal("withheld source carries records")
			}
		}
	}
}

func TestImportResultCopiesValues(t *testing.T) {
	s := settleSource(t, 1)
	s.Records = []RecordEntry{settleRecord()}
	raw := "time text"
	s.Records[0].ObservedAt = &core.Timestamp{RawText: &raw}
	line := int64(1)
	s.Failures[0].Failure.LineNumber = &line
	status := settleStatus(t, s, "one")
	result, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	publication, ok := result.Publication("one")
	if !ok {
		t.Fatal("publication missing")
	}
	records := publication.Records()
	records[0].RawText = "changed"
	*records[0].Locator.LineNumber = 8
	*records[0].ObservedAt.RawText = "changed"
	s.Records[0].RawText = "input changed"
	*s.Records[0].Locator.LineNumber = 9
	*s.Records[0].ObservedAt.RawText = "input changed"
	returned := result.Statuses()
	*returned[0].Failures[0].LineNumber = 4
	returned[0].DiagnosisCounts[0].Count = 5
	returned[0].SourceId = "changed"
	*status.Failures[0].LineNumber = 6
	got := publication.Records()[0]
	if got.RawText != "synthetic record" || *got.Locator.LineNumber != 1 || *got.ObservedAt.RawText != "time text" {
		t.Fatalf("records mutated: %+v", got)
	}
	if *publication.Status().Failures[0].LineNumber != 1 || publication.Status().DiagnosisCounts[0].Count != 1 || result.Statuses()[0].SourceId != "one" {
		t.Fatal("status mutated")
	}
}

func TestImportResultRejectsMismatchedMaterials(t *testing.T) {
	s := settleSource(t, 0)
	status := settleStatus(t, s, "one")
	if _, err := newImportResult([]scannedSource{s}, nil, "run", settleRawTextRef); err == nil {
		t.Fatal("length mismatch accepted")
	}
	if _, err := newImportResult(nil, nil, "", settleRawTextRef); err == nil {
		t.Fatal("missing run accepted")
	}
	status.Scope.SourceContentSha256 = "other"
	if _, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", settleRawTextRef); err == nil {
		t.Fatal("source mismatch accepted")
	}
}

func TestImportResultLimitsCollisionToSource(t *testing.T) {
	sources := []scannedSource{settleSource(t, 0), settleSource(t, 0)}
	sources[0].Records = []RecordEntry{settleRecord(), settleRecord()}
	statuses := []core.ImportStatus{settleStatus(t, sources[0], "one"), settleStatus(t, sources[1], "two")}
	result, err := newImportResult(sources, statuses, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Publication("one"); ok {
		t.Fatal("colliding records published")
	}
	if _, ok := result.Publication("two"); !ok {
		t.Fatal("independent source withheld")
	}
}

func TestImportResultRejectsInvalidRecords(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*RecordEntry)
	}{
		{"missing position", func(r *RecordEntry) { r.Locator.LineNumber = nil }},
		{"different source", func(r *RecordEntry) { r.Locator.SourceId = "other" }},
		{"different hash", func(r *RecordEntry) { r.Locator.SourceContentSha256 = "other" }},
		{"different file", func(r *RecordEntry) { r.Locator.SourceFileName = "other.log" }},
		{"different position kind", func(r *RecordEntry) {
			r.Locator.PositionKind = core.PositionKindSequenceNumber
			r.Locator.SequenceNumber = clonePointer(r.Locator.LineNumber)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settleSource(t, 0)
			s.Records = []RecordEntry{settleRecord()}
			tc.change(&s.Records[0])
			status := settleStatus(t, s, "one")
			if _, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", settleRawTextRef); err == nil {
				t.Fatal("invalid record published")
			}
		})
	}
}

func TestPublishedRecordRequiresRawReference(t *testing.T) {
	s := settleSource(t, 0)
	s.Records = []RecordEntry{settleRecord()}
	status := settleStatus(t, s, "one")
	for _, supplier := range []func(core.RecordLocator, string) string{nil, func(core.RecordLocator, string) string { return "" }} {
		if _, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", supplier); err == nil {
			t.Fatal("empty published record reference accepted")
		}
	}
}

func TestPublicationAcceptsScannedPositionKinds(t *testing.T) {
	for _, tc := range []struct {
		name             string
		parser, position core.PositionKind
		sequence         *int64
		want             bool
	}{
		{"line", core.PositionKindLineNumber, core.PositionKindLineNumber, nil, true},
		{"sequence", core.PositionKindSequenceNumber, core.PositionKindSequenceNumber, nil, true},
		// sn を読めないレコードは失敗になって公開されないため、通番を名乗るパーサーが
		// 行番号のレコードを公開する組み合わせを受け付けない。
		{"missing sequence is not published", core.PositionKindSequenceNumber, core.PositionKindLineNumber, nil, false},
		{"line with sequence", core.PositionKindSequenceNumber, core.PositionKindLineNumber, new(int64), false},
		{"unexpected sequence", core.PositionKindLineNumber, core.PositionKindSequenceNumber, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesParserPosition(core.RecordLocator{PositionKind: tc.position, SequenceNumber: tc.sequence}, ParserIdentity{PositionKind: tc.parser})
			if got != tc.want {
				t.Fatalf("accepted=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestImportResultWithholdsDanglingReference(t *testing.T) {
	s := settleSource(t, 1)
	ref := settleRecord().Locator
	ref.SourceContentSha256 = s.Measurement.ContentSha256
	s.Failures[0].Failure.RecordRef = &ref
	s.Failures[0].Failure.Stage = core.FailureStageNormalize
	raw := "failed synthetic record"
	s.Failures[0].RawText = &raw
	s.Failures[0].Failure.LineNumber = clonePointer(ref.LineNumber)
	s.Failures[0].Failure.RawTextRef = ref.RecordRawTextRef
	counts, err := core.NewImportCountSet(core.ImportCount{Category: core.ImportCategoryFailed, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.Counts = counts
	status := settleStatus(t, s, "one")
	status.Failures[0].RawTextRef = "raw:missing"
	result, err := newImportResult([]scannedSource{s}, []core.ImportStatus{status}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Publication("one"); ok {
		t.Fatal("dangling evidence published")
	}
	if result.Statuses()[0].WithheldReason != core.WithheldReasonDanglingEvidenceReference {
		t.Fatalf("status=%+v", result.Statuses()[0])
	}
}

func TestRawTextIndexUsesPositionValues(t *testing.T) {
	index := rawTextIndex{references: make(map[rawTextKey]string), texts: make(map[string]indexedRawText)}
	line := int64(1)
	locator := core.RecordLocator{SourceId: "source", SourceContentSha256: "hash", PositionKind: core.PositionKindLineNumber, LineNumber: &line}
	ref := index.add(locator, "")
	copyLocator := cloneLocator(locator)
	if again := index.add(copyLocator, ""); again != ref || len(index.texts) != 1 {
		t.Fatalf("copied position reference=%q want=%q texts=%d", again, ref, len(index.texts))
	}
	if text, ok := index.texts[ref]; !ok || text.text != "" {
		t.Fatalf("empty text=%+v present=%t", text, ok)
	}
	for _, change := range []func(*core.RecordLocator){
		func(l *core.RecordLocator) { l.SourceId = "other" },
		func(l *core.RecordLocator) { l.SourceContentSha256 = "other" },
		func(l *core.RecordLocator) { *l.LineNumber = 2 },
		func(l *core.RecordLocator) {
			l.PositionKind = core.PositionKindSequenceNumber
			l.SequenceNumber = clonePointer(l.LineNumber)
		},
	} {
		changed := cloneLocator(locator)
		change(&changed)
		if other := index.add(changed, "other text"); other == ref {
			t.Fatal("distinct source or position reused reference")
		}
	}
	if len(index.texts) != 5 {
		t.Fatalf("index size=%d", len(index.texts))
	}
}

func TestRawTextWithheldSource(t *testing.T) {
	source := settleSource(t, 0)
	source.Records = []RecordEntry{settleRecord(), settleRecord()}
	status := settleStatus(t, source, "withheld-source")
	index := rawTextIndex{references: make(map[rawTextKey]string), texts: make(map[string]indexedRawText)}
	locator := settleRecord().Locator
	locator.SourceId = status.SourceId
	locator.SourceContentSha256 = source.Measurement.ContentSha256
	ref := index.add(locator, "private source text")
	result, err := newImportResult([]scannedSource{source}, []core.ImportStatus{status}, "run", index.add)
	if err != nil {
		t.Fatal(err)
	}
	result.rawTexts = index
	if result.Statuses()[0].PublicationState != core.PublicationStateWithheld {
		t.Fatal("source was not withheld")
	}
	if _, ok := result.Publication(status.SourceId); ok {
		t.Fatal("withheld publication returned")
	}
	if text, ok := result.RawText(ref); ok || text != "" {
		t.Fatalf("withheld raw text=%q present=%t", text, ok)
	}
}
