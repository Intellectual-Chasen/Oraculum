package pipeline

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sourceInterpretation は収集元を指す所見である。
func sourceInterpretation(id, content string, offset core.UtcOffset, state core.AssertionState) core.Assertion {
	return core.Assertion{
		Id: id, Target: core.AssertionTarget{Kind: core.AssertionTargetKindSource, SourceContentSha256: content},
		State: state, Author: "analyst-a", RecordedAt: "2031-01-02T03:04:05.000Z",
		Basis: core.AssertionBasis{Note: "合成の根拠"}, TimeOffset: &offset,
		RevisionNumber: core.FirstAssertionRevisionNumber,
	}
}

// 主張されている解釈だけを適用し、同じ収集元に 2 件が主張されているときはどちらも適用しない。
// 改訂の番号は取り消した所見の改訂も数える。
func TestTimeInterpretationsApplyTheActiveInterpretationOfEachSource(t *testing.T) {
	applied, conflicted, withdrawn := "a1", "b2", "c3"
	read := timeInterpretationsOf([]core.Assertion{
		sourceInterpretation("as:1", applied, "+09:00", core.AssertionStateActive),
		sourceInterpretation("as:2", conflicted, "+00:00", core.AssertionStateActive),
		sourceInterpretation("as:3", conflicted, "+01:00", core.AssertionStateActive),
		sourceInterpretation("as:4", withdrawn, "+02:00", core.AssertionStateWithdrawn),
		{Id: "as:5", Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:x"},
			State: core.AssertionStateActive, RevisionNumber: core.FirstAssertionRevisionNumber},
	})
	if got := read.applied[applied]; got.Offset != "+09:00" || got.AssertionId != "as:1" {
		t.Errorf("the applied source reads %+v, want +09:00 from as:1", got)
	}
	for _, content := range []string{conflicted, withdrawn} {
		if got, found := read.applied[content]; found {
			t.Errorf("the source %q reads %+v, want no interpretation", content, got)
		}
	}
	if len(read.conflicted) != 1 || read.conflicted[0] != conflicted {
		t.Errorf("conflicted=%v, want [%s]", read.conflicted, conflicted)
	}
	if read.revision != 4*core.FirstAssertionRevisionNumber {
		t.Errorf("revision=%d, want the sum of the revision numbers of the source assertions", read.revision)
	}
}

// sourceDraft は収集元を指す所見の最初の改訂の入力である。
func sourceDraft(content string, offset core.UtcOffset) AssertionDraft {
	return AssertionDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindSource, SourceContentSha256: content},
		Author: "analyst-a", Basis: core.AssertionBasis{Note: "合成の根拠"}, TimeOffset: &offset,
	}
}

// 同じ収集元を指す所見の 2 件目を、保存先が lock の中で退ける。読み戻した所見も数える。
func TestStoreRejectsASecondInterpretationOfTheSameSource(t *testing.T) {
	content, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	store := NewMemoryAssertionStore(&steppingClock{})
	first, err := store.Create(sourceDraft(content, "+09:00"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(sourceDraft(content, "+00:00")); !errors.Is(err, ErrSourceAlreadyInterpreted) {
		t.Fatalf("the second create returned %v, want ErrSourceAlreadyInterpreted", err)
	}
	if _, err := store.Create(sourceDraft(other, "+00:00")); err != nil {
		t.Fatalf("an interpretation of another source: %v", err)
	}
	if _, err := store.Create(annotationDraft("n:process:a", "収集元でない対象")); err != nil {
		t.Fatalf("an assertion on a node: %v", err)
	}
	var written int
	restored, err := newJournaledAssertionStore(&steppingClock{}, assertionJournal{
		created: func(core.Assertion, int64) error { written++; return nil },
	}, []recordedAssertion{{assertion: first, ordinal: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Create(sourceDraft(content, "+00:00")); !errors.Is(err, ErrSourceAlreadyInterpreted) {
		t.Fatalf("the restored store returned %v, want ErrSourceAlreadyInterpreted", err)
	}
	if written != 0 {
		t.Fatalf("the journal received %d rejected assertions", written)
	}
}

// 同時に届いた同じ収集元の記録のうち、1 件だけが残る。
func TestStoreKeepsOneInterpretationOfConcurrentCreates(t *testing.T) {
	content := strings.Repeat("c", 64)
	store := NewMemoryAssertionStore(&steppingClock{})
	const attempts = 16
	var wait sync.WaitGroup
	var stored atomic.Int64
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := store.Create(sourceDraft(content, "+09:00")); err == nil {
				stored.Add(1)
			}
		}()
	}
	wait.Wait()
	if stored.Load() != 1 || len(store.List()) != 1 {
		t.Fatalf("%d creates succeeded and the store holds %d, want 1", stored.Load(), len(store.List()))
	}
}

// 地方時の収録範囲は解析に失敗したレコードの時刻も数え、解釈を記録した収集元の識別にだけ
// 解釈で読んだ範囲が入る。原資料から範囲の定まる収集元の範囲は変えない。
func TestInterpretedRangeCountsTheFailedRecordsAndFillsOnlyTheMissingRange(t *testing.T) {
	early := timelineTimestamp(t, "2031-04-05T06:07:01", core.NormalizedFormLocalWithoutOffset,
		core.OffsetStateItemAbsent)
	late := timelineTimestamp(t, "2031-04-05T06:07:30", core.NormalizedFormLocalWithoutOffset,
		core.OffsetStateItemAbsent)
	middle := timelineTimestamp(t, "2031-04-05T06:07:10", core.NormalizedFormLocalWithoutOffset,
		core.OffsetStateItemAbsent)
	local := localRangeOf(scannedSource{
		Records:  []RecordEntry{timelineRecord(1, middle), timelineRecord(2, late)},
		Failures: []failedRecord{{ObservedAt: early}},
	})
	if local == nil || *local.From.Normalized != *early.Normalized || *local.To.Normalized != *late.Normalized {
		t.Fatalf("the local range is %+v, want from the failed record to the latest record", local)
	}
	result := ImportResult{
		publications: []SourcePublication{{status: core.ImportStatus{SourceId: "src:local"}, localRange: local}},
		identities: map[string]core.SourceIdentity{
			"src:local": {SourceId: "src:local", ContentSha256: "d1"},
		},
	}
	interpreted := result.withTimeInterpretations(map[string]core.TimestampInterpretation{
		"d1": {Offset: "+09:00", AssertionId: "as:1"},
	})
	identity := interpreted.withObservedRange(interpreted.identities["src:local"])
	from, fromOk := identity.ObservedRangeFirst.Instant()
	to, toOk := identity.ObservedRangeLast.Instant()
	if !fromOk || !toOk || !from.Equal(time.Date(2031, time.April, 4, 21, 7, 1, 0, time.UTC)) ||
		!to.Equal(time.Date(2031, time.April, 4, 21, 7, 30, 0, time.UTC)) {
		t.Fatalf("the interpreted range is %s to %s, want the local range read at +09:00", from, to)
	}
	if original := interpreted.identities["src:local"]; original.ObservedRangeFirst != nil {
		t.Fatalf("the source identity keeps the interpreted range %+v", original.ObservedRangeFirst)
	}
	absolute := core.SourceIdentity{SourceId: "src:local",
		ObservedRangeFirst: timelineTimestamp(t, "2031-01-01T00:00:00Z", core.NormalizedFormRFC3339Absolute,
			core.OffsetStateInValue),
		ObservedRangeLast: timelineTimestamp(t, "2031-01-02T00:00:00Z", core.NormalizedFormRFC3339Absolute,
			core.OffsetStateInValue)}
	if kept := interpreted.withObservedRange(absolute); *kept.ObservedRangeFirst.Normalized != "2031-01-01T00:00:00Z" {
		t.Fatalf("the absolute range was replaced by %+v", kept.ObservedRangeFirst)
	}
}

// 元レコードの欄のうち、解釈を与えるのは事象の時刻の欄だけである。同じレコードのほかの地方時の
// 欄は原資料から読んだ値のまま返す。
func TestRecordFieldsCarryTheInterpretationOnTheEventTimeAlone(t *testing.T) {
	local := *timelineTimestamp(t, "2031-04-05T06:07:08", core.NormalizedFormLocalWithoutOffset,
		core.OffsetStateItemAbsent)
	interpreted, err := local.WithInterpretation(core.TimestampInterpretation{Offset: "+09:00", AssertionId: "as:1"})
	if err != nil {
		t.Fatal(err)
	}
	fields := make([]core.RecordField, 0, 3)
	for _, semantic := range []core.SemanticKey{
		core.SemanticKeyEventTime, core.SemanticKeyFileCreatedTime, core.SemanticKeyProcessStartTime,
	} {
		field, err := core.NewTimestampField(string(semantic), semantic, local)
		if err != nil {
			t.Fatal(err)
		}
		fields = append(fields, field)
	}
	built, err := (&FieldsBuilder{}).Build(RecordEntry{
		ObservedAt: &interpreted, Semantics: &RecordSemantics{Fields: fields},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range built {
		carries := field.Timestamp.Interpretation != nil
		if carries != (field.Semantic == core.SemanticKeyEventTime) {
			t.Errorf("the field %s carries the interpretation: %v", field.Semantic, carries)
		}
	}
}

// 解釈を与えた秒精度の起点は、その秒を中心にした時刻の範囲を組む。範囲は秒で比べるため、同じ秒の
// ミリ秒精度の候補を 1 件に決めつけない。
func TestWindowOfAnInterpretedOriginIsCenteredOnItsInstant(t *testing.T) {
	raw, normalized := "2031/04/05 06:07:08", "2031-04-05T06:07:08"
	local, err := core.NewTimestamp(core.Timestamp{
		RawText: &raw, Normalized: &normalized, NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision: core.PrecisionSecond, OffsetState: core.OffsetStateItemAbsent,
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := MatchConditionSelection{Conditions: []SelectedMatchCondition{{ConditionKey: core.ConditionKeySecondOfTime}}}
	if _, windowed := selection.windowOf(local); windowed {
		t.Fatal("a local time without an interpretation built a window")
	}
	interpreted, err := local.WithInterpretation(core.TimestampInterpretation{Offset: "+09:00", AssertionId: "as:1"})
	if err != nil {
		t.Fatal(err)
	}
	window, windowed := selection.windowOf(interpreted)
	if !windowed {
		t.Fatal("an interpreted local time built no window")
	}
	if window.WindowKind != core.WindowKindSameSecond || window.CenterTime == nil ||
		window.CenterTime.Normalized != "2031-04-05T06:07:08+09:00" ||
		window.CenterTime.Precision != core.PrecisionSecond || window.CenterTime.RequestText != normalized {
		t.Fatalf("window=%+v center=%+v", window, window.CenterTime)
	}
}
