// in-package test: 非公開の不変条件検査と公開判定を検査する。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func settleRecord() RecordEntry {
	line := int64(1)
	return RecordEntry{RawText: "synthetic record", Locator: core.RecordLocator{
		SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
		LineNumber: &line,
	}}
}

func TestDecidePublicationState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failures int64
		stopped  bool
		reason   core.WithheldReason
		want     core.PublicationState
	}{
		{"full", 0, false, "", core.PublicationStatePublishedFull},
		{"unpositioned failure", 1, false, "", core.PublicationStatePublishedPartial},
		{"stopped", 0, true, "", core.PublicationStatePublishedPartial},
		{"collision", 0, false, core.WithheldReasonIdentifierCollision, core.PublicationStateWithheld},
		{"reference", 0, false, core.WithheldReasonDanglingEvidenceReference, core.PublicationStateWithheld},
		{"run", 0, false, core.WithheldReasonMixedAnalysisRun, core.PublicationStateWithheld},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settleSource(t, tc.failures)
			s.ReadStopped = tc.stopped
			var broken []invariantBreak
			if tc.reason != "" {
				broken = []invariantBreak{{reason: tc.reason}}
			}
			state, reason := decidePublicationState(s, broken)
			if state != tc.want || reason != tc.reason {
				t.Fatalf("state=%s reason=%s", state, reason)
			}
		})
	}
}

func TestIdentifierCollisions(t *testing.T) {
	s := settleSource(t, 0)
	s.Records = []RecordEntry{settleRecord(), settleRecord()}
	broken, err := checkIdentifierCollision([]scannedSource{s}, []string{"one"})
	if err != nil || len(broken) != 1 || broken[0].runWide || broken[0].reason != core.WithheldReasonIdentifierCollision {
		t.Fatalf("broken=%+v error=%v", broken, err)
	}
	broken, err = checkIdentifierCollision([]scannedSource{{}, {}}, []string{"one", "one"})
	if err != nil || len(broken) != 1 || !broken[0].runWide {
		t.Fatalf("broken=%+v error=%v", broken, err)
	}
	broken, err = checkIdentifierCollision([]scannedSource{{}, {}}, []string{"one", "two"})
	if err != nil || len(broken) != 0 {
		t.Fatalf("broken=%+v error=%v", broken, err)
	}
	if _, err := checkIdentifierCollision([]scannedSource{s}, nil); err == nil {
		t.Fatal("length mismatch accepted")
	}
}

func TestIdentifierCollisionsIncludeFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		successes []int64
		failures  []int64
		want      int
	}{
		{"success_and_failure", []int64{7}, []int64{7}, 1},
		{"two_failures", nil, []int64{7, 7}, 1},
		{"distinct_positions", []int64{7}, []int64{8}, 0},
		{"unpositioned_failures", []int64{7}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := scannedSource{Failures: []failedRecord{{}, {}}}
			for _, sn := range tc.successes {
				source.Records = append(source.Records, RecordEntry{Locator: core.RecordLocator{
					PositionKind: core.PositionKindSequenceNumber, SequenceNumber: &sn,
				}})
			}
			for _, sn := range tc.failures {
				source.Failures = append(source.Failures, failedRecord{Failure: core.ImportFailure{RecordRef: &core.RecordLocator{
					PositionKind: core.PositionKindSequenceNumber, SequenceNumber: &sn,
				}}})
			}
			broken, err := checkIdentifierCollision([]scannedSource{{}, source}, []string{"one", "two"})
			if err != nil || len(broken) != tc.want {
				t.Fatalf("collisions=%d, want %d; error=%v", len(broken), tc.want, err)
			}
			for _, violation := range broken {
				if violation.sourceIndex != 1 || violation.runWide || violation.reason != core.WithheldReasonIdentifierCollision {
					t.Errorf("collision=%+v", violation)
				}
			}
		})
	}
}

func TestEvidenceReferences(t *testing.T) {
	s := settleSource(t, 1)
	status := settleStatus(t, s, "one")
	ref := settleRecord().Locator
	ref.SourceId = "one"
	ref.SourceContentSha256 = s.Measurement.ContentSha256
	ref.RecordRawTextRef = "raw:synthetic:1"
	status.Failures[0].RecordRef = &ref
	status.Failures[0].RawTextRef = ref.RecordRawTextRef
	if broken := checkEvidenceReferences([]scannedSource{s}, []core.ImportStatus{status}); len(broken) != 0 {
		t.Fatalf("valid reference: %+v", broken)
	}
	for _, tc := range []struct {
		name   string
		change func(*core.ImportFailure)
	}{
		{"missing raw text", func(f *core.ImportFailure) { f.RawTextRef = "missing" }},
		{"other source", func(f *core.ImportFailure) { f.RecordRef.SourceId = "other" }},
		{"other hash", func(f *core.ImportFailure) { f.RecordRef.SourceContentSha256 = "other" }},
		{"raw text without position", func(f *core.ImportFailure) { f.RecordRef = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := cloneStatus(status)
			tc.change(&changed.Failures[0])
			broken := checkEvidenceReferences([]scannedSource{s}, []core.ImportStatus{changed})
			if len(broken) != 1 || broken[0].reason != core.WithheldReasonDanglingEvidenceReference || broken[0].sourceIndex != 0 {
				t.Fatalf("broken=%+v", broken)
			}
		})
	}
}
