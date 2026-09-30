package core_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assertionContentSha256 は fixture の収集元の内容の識別である。
const assertionContentSha256 = "2b1f3c4d5e6f708192a3b4c5d6e7f80912a3b4c5d6e7f80912a3b4c5d6e7f809"

// assertionRecordedAt は所見を記録した時刻である。
const assertionRecordedAt core.AssertionTime = "2026-01-02T03:04:05.000Z"

func assertionLineRef(line int64) core.AssertionRecordRef {
	return core.AssertionRecordRef{
		SourceContentSha256: assertionContentSha256,
		PositionKind:        core.PositionKindLineNumber,
		LineNumber:          &line,
	}
}

func assertionNodeTarget() core.AssertionTarget {
	return core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:process:abc"}
}

func assertionEdgeTarget() core.AssertionTarget {
	return core.AssertionTarget{
		Kind: core.AssertionTargetKindEdge,
		Edge: &core.AssertionEdgeRef{
			Kind: core.EdgeKindProcessCommunication, SourceNodeId: "n:process:abc",
			TargetNodeId: "n:ip:def",
		},
	}
}

func validAssertion() core.Assertion {
	return core.Assertion{
		Id:             "as:0123",
		Target:         assertionNodeTarget(),
		State:          core.AssertionStateActive,
		Author:         "analyst-a",
		RecordedAt:     assertionRecordedAt,
		Basis:          core.AssertionBasis{Note: "起動のレコードと突き合わせた"},
		RevisionNumber: core.FirstAssertionRevisionNumber,
	}
}

// 所見は著者・時刻・対象・根拠・変更履歴を持ち、揃った組が通る。
func TestAssertionCarriesTheAuthorTimeTargetBasisAndHistory(t *testing.T) {
	assertion := validAssertion()
	assertion.RevisionNumber = core.FirstAssertionRevisionNumber + 1
	assertion.History = []core.AssertionRevision{{
		RevisionNumber: core.FirstAssertionRevisionNumber,
		State:          core.AssertionStateActive,
		Author:         "analyst-b",
		RecordedAt:     assertionRecordedAt,
		Basis:          core.AssertionBasis{Note: "最初の根拠", RecordRefs: []core.AssertionRecordRef{assertionLineRef(7)}},
	}}
	if err := assertion.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the complete assertion to pass", err)
	}
	if assertion.Author == "" || assertion.RecordedAt == "" || assertion.Basis.Note == "" {
		t.Errorf("assertion = %+v, want the author, the time and the basis populated", assertion)
	}
	if assertion.History[0].Author == assertion.Author {
		t.Errorf("the superseded revision carries the author %q of the current revision",
			assertion.Author)
	}
}

// 所見は自由記述のメモだけを主張の中身として持ち、種別と攻撃手法の欄を持たない。
func TestAssertionCarriesTheNoteWithoutAFixedFormat(t *testing.T) {
	assertion := validAssertion()
	assertion.Basis.Note = "接続先 port 5985 は WinRM と読んだ"
	if err := assertion.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the assertion carrying a free note to pass", err)
	}
	encoded, err := json.Marshal(assertion)
	if err != nil {
		t.Fatal(err)
	}
	var items map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &items); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kind", "technique"} {
		if _, carried := items[name]; carried {
			t.Errorf("the assertion %s carries the item %q, want the fixed format to stay out",
				encoded, name)
		}
	}
	if !strings.Contains(string(items["basis"]), assertion.Basis.Note) {
		t.Errorf("the basis %s drops the note %q, want the note of the analyst to be carried",
			items["basis"], assertion.Basis.Note)
	}
}

// 対象はノード・関係・レコードの 3 つを取り、種別に該当しない参照を持つ組を退ける。
func TestAssertionTargetCarriesOneReferenceForItsKind(t *testing.T) {
	edge := core.AssertionTarget{
		Kind: core.AssertionTargetKindEdge,
		Edge: &core.AssertionEdgeRef{
			Kind: core.EdgeKindProcessCommunication, SourceNodeId: "n:process:a",
			TargetNodeId: "n:ip:b",
		},
	}
	record := core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord, Record: pointerTo(assertionLineRef(3)),
	}
	for _, target := range []core.AssertionTarget{assertionNodeTarget(), edge, record} {
		if err := target.Validate(); err != nil {
			t.Errorf("Validate() of the %q target = %v, want it to pass", target.Kind, err)
		}
	}
	mixed := assertionNodeTarget()
	mixed.Record = pointerTo(assertionLineRef(3))
	if err := mixed.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("Validate() of a node target carrying a record = %v, want ErrInconsistentValue", err)
	}
	missing := core.AssertionTarget{Kind: core.AssertionTargetKindEdge}
	if err := missing.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("Validate() of an edge target without its reference = %v, want ErrMissingRequiredItem", err)
	}
}

// 所見はノード・関係・レコードのいずれにも付けられる。
func TestAssertionTakesEveryTargetKind(t *testing.T) {
	for _, target := range []core.AssertionTarget{
		assertionNodeTarget(),
		assertionEdgeTarget(),
		{Kind: core.AssertionTargetKindRecord, Record: pointerTo(assertionLineRef(3))},
	} {
		assertion := validAssertion()
		assertion.Target = target
		if err := assertion.Validate(); err != nil {
			t.Errorf("Validate() of an assertion on the %q target = %v, want it to pass",
				target.Kind, err)
		}
	}
}

// 関係を足した印は関係の所見だけが持つ。
func TestAssertionAddsARelationOnlyOnAnEdge(t *testing.T) {
	onEdge := validAssertion()
	onEdge.Target, onEdge.AddsRelation = assertionEdgeTarget(), true
	if err := onEdge.Validate(); err != nil {
		t.Errorf("Validate() of an assertion adding a relation = %v, want it to pass", err)
	}
	onNode := validAssertion()
	onNode.AddsRelation = true
	if err := onNode.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("Validate() of a node assertion adding a relation = %v, want ErrUnexpectedItem", err)
	}
}

// レコードの参照は収集元の内容の識別と位置だけを持ち、sourceId を材料に入れない。
func TestAssertionRecordRefDropsTheSourceId(t *testing.T) {
	line := int64(12)
	locator := core.RecordLocator{
		SourceId:            "source-of-the-first-import",
		SourceContentSha256: assertionContentSha256,
		SourceFileName:      "pc01.log",
		PositionKind:        core.PositionKindLineNumber,
		LineNumber:          &line,
	}
	first := core.NewAssertionRecordRef(locator)
	locator.SourceId = "source-of-the-second-import"
	second := core.NewAssertionRecordRef(locator)

	if err := first.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the reference to pass", err)
	}
	encodedFirst, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	encodedSecond, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedFirst) != string(encodedSecond) {
		t.Errorf("the reference of the second import is %s, want the same reference as %s",
			encodedSecond, encodedFirst)
	}
	if strings.Contains(string(encodedFirst), "sourceId") {
		t.Errorf("the reference %s carries a sourceId, want the identifier of the import to stay out",
			encodedFirst)
	}
}

// 同じ行から始まる 2 つの事象を指す所見は、別の対象として区別される。
func TestAssertionRecordRefSeparatesByteRangesSharingTheFirstLine(t *testing.T) {
	line := int64(137)
	locator := core.RecordLocator{
		SourceContentSha256: assertionContentSha256,
		SourceFileName:      "linux-host.log",
		PositionKind:        core.PositionKindByteRange,
		LineNumber:          &line,
	}
	first, second := int64(7321), int64(7621)
	locator.ByteOffset = &first
	firstTarget := core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord,
		Record: func() *core.AssertionRecordRef {
			ref := core.NewAssertionRecordRef(locator)
			return &ref
		}(),
	}
	locator.ByteOffset = &second
	secondTarget := core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord,
		Record: func() *core.AssertionRecordRef {
			ref := core.NewAssertionRecordRef(locator)
			return &ref
		}(),
	}

	if err := firstTarget.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the target to pass", err)
	}
	firstParts := strings.Join(firstTarget.DigestParts(), "\x00")
	secondParts := strings.Join(secondTarget.DigestParts(), "\x00")
	if firstParts == secondParts {
		t.Errorf("both records digest as %q, want the byte offset to separate them", firstParts)
	}
	if !strings.Contains(firstParts, "7321") {
		t.Errorf("the digest parts %q carry no byte offset", firstParts)
	}
}

// 位置の指し方に該当する位置の値が無い参照を退け、揃った参照を通す。
func TestAssertionRecordRefNeedsThePositionOfItsPositionKind(t *testing.T) {
	sequence := int64(5)
	withSequence := core.AssertionRecordRef{
		SourceContentSha256: assertionContentSha256,
		PositionKind:        core.PositionKindSequenceNumber,
		SequenceNumber:      &sequence,
	}
	if err := withSequence.Validate(); err != nil {
		t.Errorf("Validate() with a sequence number = %v, want it to pass", err)
	}
	withoutSequence := withSequence
	withoutSequence.SequenceNumber = nil
	if err := withoutSequence.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("Validate() without the sequence number = %v, want ErrMissingRequiredItem", err)
	}
}

// 現在の改訂の番号は、置き換えられた改訂の個数の次である。
func TestAssertionRevisionNumberFollowsTheSupersededRevisions(t *testing.T) {
	assertion := validAssertion()
	assertion.History = []core.AssertionRevision{{
		RevisionNumber: core.FirstAssertionRevisionNumber,
		State:          core.AssertionStateActive,
		Author:         "analyst-a",
		RecordedAt:     assertionRecordedAt,
		Basis:          core.AssertionBasis{Note: "最初の根拠"},
	}}
	if err := assertion.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("Validate() with a stale revisionNumber = %v, want ErrInconsistentValue", err)
	}
	assertion.RevisionNumber = int64(len(assertion.History)) + core.FirstAssertionRevisionNumber
	if err := assertion.Validate(); err != nil {
		t.Errorf("Validate() with the following revisionNumber = %v, want it to pass", err)
	}
}

// 履歴の改訂の番号は起点からの連番である。
func TestAssertionHistoryCarriesConsecutiveRevisionNumbers(t *testing.T) {
	revision := func(number int64) core.AssertionRevision {
		return core.AssertionRevision{
			RevisionNumber: number, State: core.AssertionStateActive, Author: "analyst-a",
			RecordedAt: assertionRecordedAt, Basis: core.AssertionBasis{Note: "根拠"},
		}
	}
	assertion := validAssertion()
	assertion.History = []core.AssertionRevision{
		revision(core.FirstAssertionRevisionNumber),
		revision(core.FirstAssertionRevisionNumber + 2),
	}
	assertion.RevisionNumber = int64(len(assertion.History)) + core.FirstAssertionRevisionNumber
	if err := assertion.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("Validate() with a gap in the history = %v, want ErrInconsistentValue", err)
	}
	assertion.History[1] = revision(core.FirstAssertionRevisionNumber + 1)
	if err := assertion.Validate(); err != nil {
		t.Errorf("Validate() with consecutive revision numbers = %v, want it to pass", err)
	}
}

// 著者の文字列に制御文字を持つ所見を退け、制御文字を持たない文字列を通す。
func TestAssertionRejectsAControlCharacterInTheAuthor(t *testing.T) {
	assertion := validAssertion()
	assertion.Author = "analyst-a\ninjected"
	if err := assertion.Validate(); !errors.Is(err, core.ErrControlCharacter) {
		t.Errorf("Validate() with a newline in the author = %v, want ErrControlCharacter", err)
	}
	assertion.Author = "analyst-a injected"
	if err := assertion.Validate(); err != nil {
		t.Errorf("Validate() with a plain author = %v, want it to pass", err)
	}
}

// 根拠の記述が無い所見と、空白文字だけの記述を持つ所見を退け、記述のある所見を通す。
func TestAssertionNeedsTheBasisNote(t *testing.T) {
	assertion := validAssertion()
	assertion.Basis = core.AssertionBasis{}
	if err := assertion.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("Validate() without a basis note = %v, want ErrMissingRequiredItem", err)
	}
	for _, blank := range []string{" ", "\t", "\n", "  \t\n "} {
		assertion.Basis = core.AssertionBasis{Note: blank}
		if err := assertion.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("Validate() with the basis note %q = %v, want ErrMissingRequiredItem",
				blank, err)
		}
	}
	assertion.Basis = core.AssertionBasis{Note: "同じ端末の起動のレコードが無い"}
	if err := assertion.Validate(); err != nil {
		t.Errorf("Validate() with a basis note = %v, want it to pass", err)
	}
	// 記述の前後の空白は記述の一部として通る。
	assertion.Basis = core.AssertionBasis{Note: " 同じ端末の起動のレコードが無い\n"}
	if err := assertion.Validate(); err != nil {
		t.Errorf("Validate() with a basis note carrying surrounding blanks = %v, want it to pass",
			err)
	}
}

// 記録の時刻は UTC のミリ秒までの文字列であり、読めない文字列を退ける。
func TestAssertionTimeCarriesTheUtcMillisecondText(t *testing.T) {
	instant := time.Date(2026, time.January, 2, 3, 4, 5, int(6*time.Millisecond), time.UTC)
	recorded := core.NewAssertionTime(instant)
	if string(recorded) != "2026-01-02T03:04:05.006Z" {
		t.Errorf("NewAssertionTime() = %q, want the UTC text with milliseconds", recorded)
	}
	readBack, readable := recorded.Instant()
	if !readable || !readBack.Equal(instant) {
		t.Errorf("Instant() = %v %v, want %v", readBack, readable, instant)
	}
	if err := core.AssertionTime("2026-01-02T03:04:05Z").Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Error("Validate() of a text without milliseconds passed, want it to be rejected")
	}
}

// 必須の集合は要素数 0 の場合も集合として直列化する。
func TestAssertionSerializesItsRequiredSetsAsArrays(t *testing.T) {
	encoded, err := json.Marshal(validAssertion())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{`"history":[]`, `"recordRefs":[]`} {
		if !strings.Contains(string(encoded), item) {
			t.Errorf("the encoded assertion %s lacks %s", encoded, item)
		}
	}
}

// 対象を JSON から復元すると、未知の項目と、種別に該当しない参照を退ける。
func TestAssertionTargetUnmarshalRejectsUnknownItemsAndMixedReferences(t *testing.T) {
	var target core.AssertionTarget
	if err := json.Unmarshal([]byte(`{"kind":"node","nodeId":"n:ip:a"}`), &target); err != nil {
		t.Fatalf("Unmarshal of a node target = %v, want it to pass", err)
	}
	if target.NodeId != "n:ip:a" {
		t.Errorf("nodeId = %q, want the decoded value", target.NodeId)
	}
	if err := json.Unmarshal([]byte(`{"kind":"node","nodeId":"n:ip:a","mode":"x"}`),
		&target); err == nil {
		t.Error("Unmarshal of a target with an unknown item passed, want it to be rejected")
	}
	if err := json.Unmarshal(
		[]byte(`{"kind":"node","nodeId":"n:ip:a","edge":{"kind":"ran_on","sourceNodeId":"n:process:a","targetNodeId":"n:terminal:b"}}`),
		&target); err == nil {
		t.Error("Unmarshal of a node target carrying an edge passed, want it to be rejected")
	}
}

// 種別と値が同じで対象の種類だけが違う 2 つの対象は、別の文字列の並びになる。
func TestAssertionTargetDigestPartsSeparateTheTargetKinds(t *testing.T) {
	node := assertionNodeTarget()
	record := core.AssertionTarget{
		Kind: core.AssertionTargetKindRecord, Record: pointerTo(assertionLineRef(3)),
	}
	if strings.Join(node.DigestParts(), "\x00") == strings.Join(record.DigestParts(), "\x00") {
		t.Error("the two targets share their digest parts, want the target kind to separate them")
	}
}

// メモは新しい改訂で変えられ、置き換えられた改訂のメモは履歴に残る。
func TestAssertionNoteChangesWithTheRevision(t *testing.T) {
	assertion := validAssertion()
	assertion.Basis = core.AssertionBasis{Note: "WinRM と読み直した"}
	assertion.RevisionNumber = core.FirstAssertionRevisionNumber + 1
	assertion.History = []core.AssertionRevision{{
		RevisionNumber: core.FirstAssertionRevisionNumber,
		State:          core.AssertionStateActive,
		Author:         "analyst-b",
		RecordedAt:     assertionRecordedAt,
		Basis:          core.AssertionBasis{Note: "最初のメモ"},
	}}
	if err := assertion.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the revised note to pass", err)
	}
	if assertion.History[0].Basis.Note == assertion.Basis.Note {
		t.Errorf("the superseded revision carries the note %q of the current revision",
			assertion.Basis.Note)
	}
}

func pointerTo[T any](value T) *T { return &value }
