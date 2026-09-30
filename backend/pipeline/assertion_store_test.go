// in-package test: fixture のグラフを組む非公開の helper を使う。
package pipeline

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// storeEpoch は所見を記録した時刻の起点である。ホストの時計から切り離した値である。
var storeEpoch = time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

// storeStep は 1 回の記録ごとに進む幅である。
const storeStep = time.Second

// steppingClock は記録のたびに固定の幅で進む時計である。
type steppingClock struct{ ticks int64 }

func (c *steppingClock) Now() core.AssertionTime {
	c.ticks++
	return core.NewAssertionTime(storeEpoch.Add(time.Duration(c.ticks) * storeStep))
}

func annotationDraft(nodeId, note string) AssertionDraft {
	return AssertionDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: nodeId},
		Author: "analyst-a",
		Basis:  core.AssertionBasis{Note: note},
	}
}

// 記録した所見は、著者・時刻・対象・根拠を持ち、履歴の無い最初の改訂になる。
func TestMemoryAssertionStoreRecordsTheFirstRevision(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	created, err := store.Create(annotationDraft("n:process:a", "同じ端末の起動のレコードを確認した"))
	if err != nil {
		t.Fatalf("Create() = %v, want the draft to be stored", err)
	}
	if created.Author != "analyst-a" || created.Basis.Note == "" {
		t.Errorf("created = %+v, want the author and the basis of the draft", created)
	}
	if created.State != core.AssertionStateActive {
		t.Errorf("state = %q, want %q", created.State, core.AssertionStateActive)
	}
	if created.RevisionNumber != core.FirstAssertionRevisionNumber {
		t.Errorf("revisionNumber = %d, want %d",
			created.RevisionNumber, core.FirstAssertionRevisionNumber)
	}
	if len(created.History) != 0 {
		t.Errorf("history = %+v, want the first revision to supersede nothing", created.History)
	}
	if want := core.NewAssertionTime(storeEpoch.Add(storeStep)); created.RecordedAt != want {
		t.Errorf("recordedAt = %q, want %q", created.RecordedAt, want)
	}
	found, ok := store.Find(created.Id)
	if !ok || found.Id != created.Id {
		t.Errorf("Find(%q) = %+v %v, want the stored assertion", created.Id, found, ok)
	}
}

// 改訂を足すと、置き換えられた改訂が履歴に残り、著者と時刻と根拠を保つ。
func TestMemoryAssertionStoreKeepsTheSupersededRevision(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	created, err := store.Create(annotationDraft("n:process:a", "最初の根拠"))
	if err != nil {
		t.Fatal(err)
	}
	revised, err := store.Revise(created.Id, AssertionRevisionDraft{
		State:        core.AssertionStateWithdrawn,
		Author:       "analyst-b",
		Basis:        core.AssertionBasis{Note: "別の端末のレコードで否定された"},
		BaseRevision: created.RevisionNumber,
	})
	if err != nil {
		t.Fatalf("Revise() = %v, want the revision to be stored", err)
	}
	if revised.State != core.AssertionStateWithdrawn || revised.Author != "analyst-b" {
		t.Errorf("revised = %+v, want the state and the author of the draft", revised)
	}
	if revised.RevisionNumber != created.RevisionNumber+1 {
		t.Errorf("revisionNumber = %d, want %d", revised.RevisionNumber, created.RevisionNumber+1)
	}
	if len(revised.History) != 1 {
		t.Fatalf("history = %+v, want the one superseded revision", revised.History)
	}
	superseded := revised.History[0]
	if superseded.Author != created.Author || superseded.RecordedAt != created.RecordedAt ||
		superseded.Basis.Note != created.Basis.Note {
		t.Errorf("the superseded revision %+v differs from the created assertion %+v",
			superseded, created)
	}
	if revised.RecordedAt == created.RecordedAt {
		t.Errorf("recordedAt stayed %q, want the new revision to carry a later time",
			revised.RecordedAt)
	}
	if revised.Target.NodeId != created.Target.NodeId {
		t.Errorf("revised %+v changed the target of %+v", revised, created)
	}
}

// 元にした revision が古い改訂は、journal へ書かずに競合と現在の所見を返す。
func TestMemoryAssertionStoreRejectsTheRevisionOnAStaleBase(t *testing.T) {
	journaled := 0
	store := &MemoryAssertionStore{
		clock: &steppingClock{}, byId: make(map[string]core.Assertion),
		journal: assertionJournal{revised: func(core.Assertion) error { journaled++; return nil }},
	}
	created, err := store.Create(annotationDraft("n:process:a", "最初の根拠"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Revise(created.Id, AssertionRevisionDraft{
		State: core.AssertionStateWithdrawn, Author: "analyst-b",
		Basis: core.AssertionBasis{Note: "先の改訂"}, BaseRevision: created.RevisionNumber,
	})
	if err != nil {
		t.Fatalf("Revise() on the current base = %v, want it to be stored", err)
	}
	if first.RevisionNumber != created.RevisionNumber+1 || journaled != 1 {
		t.Fatalf("revisionNumber = %d, journaled = %d, want %d and 1",
			first.RevisionNumber, journaled, created.RevisionNumber+1)
	}
	current, err := store.Revise(created.Id, AssertionRevisionDraft{
		State: core.AssertionStateActive, Author: "analyst-c",
		Basis: core.AssertionBasis{Note: "後の改訂"}, BaseRevision: created.RevisionNumber,
	})
	if !errors.Is(err, ErrAssertionRevisionConflict) {
		t.Fatalf("Revise() on a stale base = %v, want ErrAssertionRevisionConflict", err)
	}
	if current.RevisionNumber != first.RevisionNumber || current.Author != "analyst-b" {
		t.Errorf("the conflict carries %+v, want the current assertion %+v", current, first)
	}
	if journaled != 1 {
		t.Errorf("journaled = %d, want the stale revision to stay out of the journal", journaled)
	}
}

// 改訂を足しても、分析者が関係を足した印を保つ。
func TestMemoryAssertionStoreKeepsTheAddedRelationOnRevision(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	created, err := store.Create(AssertionDraft{
		Target: edgeTarget(core.AssertionEdgeRef{
			Kind: core.EdgeKindFileCopy, SourceNodeId: "n:file:a", TargetNodeId: "n:file:b",
		}),
		Author: "analyst-a", Basis: core.AssertionBasis{Note: "コピーの関係を足した"},
		AddsRelation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := store.Revise(created.Id, AssertionRevisionDraft{
		State: core.AssertionStateActive, Author: "analyst-b",
		Basis: core.AssertionBasis{Note: "別の根拠を足した"}, BaseRevision: created.RevisionNumber,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !revised.AddsRelation {
		t.Errorf("revised = %+v, want the revision to keep addsRelation", revised)
	}
}

// 識別子に該当する所見が無い改訂の追加は ErrAssertionNotFound を返す。
func TestMemoryAssertionStoreReportsTheMissingAssertion(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	_, err := store.Revise("as:absent", AssertionRevisionDraft{
		State: core.AssertionStateActive, Author: "analyst-a",
		Basis: core.AssertionBasis{Note: "根拠"},
	})
	if !errors.Is(err, ErrAssertionNotFound) {
		t.Errorf("Revise() of an absent assertion = %v, want ErrAssertionNotFound", err)
	}
	if _, found := store.Find("as:absent"); found {
		t.Error("Find() of an absent assertion reported it as found")
	}
}

// 保存先の内部の失敗と、要求の項目の不備を別の error にする。
func TestMemoryAssertionStoreSeparatesTheStoreFailureFromTheDraftFault(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	store.ordinal = math.MaxInt64
	_, err := store.Create(annotationDraft("n:process:a", "根拠"))
	if !errors.Is(err, ErrAssertionStoreFailure) {
		t.Errorf("Create() with the ordinals exhausted = %v, want ErrAssertionStoreFailure", err)
	}

	store.ordinal = 0
	_, err = store.Create(annotationDraft("n:process:a", ""))
	if err == nil {
		t.Fatal("Create() without a basis note passed, want it to be rejected")
	}
	if errors.Is(err, ErrAssertionStoreFailure) {
		t.Errorf("Create() with a draft fault = %v, want the fault to stay out of "+
			"ErrAssertionStoreFailure", err)
	}
	if !errors.Is(err, core.ErrInvalid) {
		t.Errorf("Create() with a draft fault = %v, want core.ErrInvalid", err)
	}

	created, err := store.Create(annotationDraft("n:process:a", "根拠"))
	if err != nil {
		t.Fatal(err)
	}
	exhausted := store.byId[created.Id]
	exhausted.RevisionNumber = math.MaxInt64
	store.byId[created.Id] = exhausted
	_, err = store.Revise(created.Id, AssertionRevisionDraft{
		State: core.AssertionStateActive, Author: "analyst-b",
		Basis: core.AssertionBasis{Note: "2 つ目の根拠"}, BaseRevision: math.MaxInt64,
	})
	if !errors.Is(err, ErrAssertionStoreFailure) {
		t.Errorf("Revise() with the revisions exhausted = %v, want ErrAssertionStoreFailure", err)
	}
}

// 不備のある所見は保存先に入らない。
func TestMemoryAssertionStoreRejectsTheDraftWithoutABasis(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	draft := annotationDraft("n:process:a", "")
	if _, err := store.Create(draft); err == nil {
		t.Error("Create() without a basis note passed, want it to be rejected")
	}
	if listed := store.List(); len(listed) != 0 {
		t.Errorf("List() = %+v, want the rejected draft to stay out", listed)
	}
	draft.Basis.Note = "根拠"
	if _, err := store.Create(draft); err != nil {
		t.Errorf("Create() with a basis note = %v, want it to pass", err)
	}
	if listed := store.List(); len(listed) != 1 {
		t.Errorf("List() = %+v, want the accepted draft to be stored", listed)
	}
}

// 同じ対象へ所見を 2 件記録すると、2 件が別の識別子で並ぶ。
func TestMemoryAssertionStoreListsTheAssertionsInTheOrderOfRecording(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	first, err := store.Create(annotationDraft("n:process:a", "1 件目の根拠"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(annotationDraft("n:process:a", "2 件目の根拠"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Id == second.Id {
		t.Fatalf("the two assertions share the identifier %q", first.Id)
	}
	listed := store.List()
	if len(listed) != 2 || listed[0].Id != first.Id || listed[1].Id != second.Id {
		t.Errorf("List() = %+v, want %q then %q", listed, first.Id, second.Id)
	}
}

// 返した組を書き換えても保存先の組は変わらない。
func TestMemoryAssertionStoreReturnsACopy(t *testing.T) {
	store := NewMemoryAssertionStore(&steppingClock{})
	created, err := store.Create(annotationDraft("n:process:a", "根拠"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revise(created.Id, AssertionRevisionDraft{
		State: core.AssertionStateActive, Author: "analyst-b",
		Basis: core.AssertionBasis{Note: "2 つ目の根拠"}, BaseRevision: created.RevisionNumber,
	}); err != nil {
		t.Fatal(err)
	}
	listed := store.List()
	listed[0].History[0].Author = "overwritten"
	stored, ok := store.Find(created.Id)
	if !ok {
		t.Fatalf("Find(%q) lost the assertion", created.Id)
	}
	if stored.History[0].Author != created.Author {
		t.Errorf("the stored history carries the author %q, want %q",
			stored.History[0].Author, created.Author)
	}
}

// 所見は取り込み結果と観測層のグラフを書き換えない。
func TestMemoryStoreKeepsTheImportResultBesideTheAssertions(t *testing.T) {
	result := graphResult(t)
	store := NewMemoryStore(result, &steppingClock{})
	before := wholeGraphOf(t, NewGraph(store.ImportResult(), AllMatchConditions()))

	if _, err := store.Assertions().Create(AssertionDraft{
		Target: core.AssertionTarget{
			Kind: core.AssertionTargetKindEdge,
			Edge: &core.AssertionEdgeRef{
				Kind: before.Edges[0].Kind, SourceNodeId: before.Edges[0].SourceNodeId,
				TargetNodeId: before.Edges[0].TargetNodeId,
			},
		},
		Author: "analyst-a",
		Basis:  core.AssertionBasis{Note: "同じ秒の別の候補を採る"},
	}); err != nil {
		t.Fatal(err)
	}

	after := wholeGraphOf(t, NewGraph(store.ImportResult(), AllMatchConditions()))
	if len(after.Edges) != len(before.Edges) || len(after.Nodes) != len(before.Nodes) {
		t.Fatalf("the graph carries %d nodes and %d edges after the assertion, want %d and %d",
			len(after.Nodes), len(after.Edges), len(before.Nodes), len(before.Edges))
	}
	for index, edge := range after.Edges {
		if edge.Id != before.Edges[index].Id || edge.State != before.Edges[index].State {
			t.Errorf("edge %d became %q %q, want %q %q", index, edge.Id, edge.State,
				before.Edges[index].Id, before.Edges[index].State)
		}
	}
}
