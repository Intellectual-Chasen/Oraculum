// in-package test: journal を持つ保存先の組み立ては非公開であり、外の test package から作れない。
package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// fixedAssistClock は呼ぶたびに 1 秒進む時刻を返す。
type fixedAssistClock struct{ ticks int }

func (c *fixedAssistClock) Now() core.AssertionTime {
	c.ticks++
	return core.NewAssertionTime(time.Date(2030, 1, 2, 3, 4, c.ticks, 0, time.UTC))
}

const testConversationId = "0123456789abcdef0123456789abcdef"

var testSourceSha = "c" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"

// recordingAssistJournal は書いた記録を集め、failing が真の間は書き込みを失敗させる。
type recordingAssistJournal struct {
	failing     bool
	disclosures []core.AssistDisclosure
	turns       []core.AssistTurn
	refs        []core.AssistShortRef
}

func (j *recordingAssistJournal) journal() assistJournal {
	fail := func() error {
		if j.failing {
			return errors.New("synthetic write failure")
		}
		return nil
	}
	return assistJournal{
		permissionRecorded: func(core.AssistPermissionRevision) error { return fail() },
		conversationOpened: func(core.AssistConversation) error { return fail() },
		disclosed: func(disclosure core.AssistDisclosure, turn *core.AssistTurn, refs []core.AssistShortRef) error {
			if err := fail(); err != nil {
				return err
			}
			j.disclosures = append(j.disclosures, disclosure)
			if turn != nil {
				j.turns = append(j.turns, *turn)
			}
			j.refs = append(j.refs, refs...)
			return nil
		},
	}
}

func newTestAssistStore(t *testing.T, journal *recordingAssistJournal) *MemoryAssistStore {
	t.Helper()
	store, err := newJournaledAssistStore(&fixedAssistClock{}, journal.journal(), assistRecords{})
	if err != nil {
		t.Fatal(err)
	}
	store.newId = func() (string, error) { return testConversationId, nil }
	return store
}

func recordTestPermission(t *testing.T, store *MemoryAssistStore, action core.AssistPermissionAction) {
	t.Helper()
	if _, err := store.RecordPermission(AssistPermissionDraft{
		Provider: core.AssistProviderClaude, Action: action, Analyst: "analyst-a",
	}); err != nil {
		t.Fatal(err)
	}
}

func testVersions() core.AssistVersions {
	return core.AssistVersions{SourceSetSha256: testSourceSha}
}

func registerTestTurn(t *testing.T, store *MemoryAssistStore, turnId string) {
	t.Helper()
	turn := core.AssistTurn{
		ConversationId: testConversationId, TurnId: turnId,
		MatchConditions: []core.AssistMatchCondition{{ConditionKey: core.ConditionKeyDestinationIp}},
	}
	if _, _, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: testConversationId, TurnId: turnId, Tool: core.AssistToolTurn, Request: "{}",
		Versions: testVersions(), Turn: &turn,
	}, func(AssistRefIssuer) (AssistComposition, error) {
		return AssistComposition{Body: []byte(`{"refs":[]}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testRecordTarget(line int64) core.AssistRecordTarget {
	return core.AssistRecordTarget{SourceId: "src:1", Record: core.AssertionRecordRef{
		SourceContentSha256: testSourceSha, PositionKind: core.PositionKindLineNumber, LineNumber: &line,
	}}
}

// 調査の保存先へ書けた改訂だけを確定し、書けなかった改訂は番号を使わない。
func TestJournaledAssistStoreKeepsOnlyTheWrittenRevisions(t *testing.T) {
	journal := &recordingAssistJournal{failing: true}
	store := newTestAssistStore(t, journal)
	draft := AssistPermissionDraft{
		Provider: core.AssistProviderClaude, Action: core.AssistPermissionActionGrant, Analyst: "analyst-a",
	}
	if _, err := store.RecordPermission(draft); !errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("RecordPermission with a failing journal = %v, want the store failure", err)
	}
	if revisions, _ := store.PermissionRevisions(); len(revisions) != 0 {
		t.Fatalf("the failed write left %+v", revisions)
	}
	journal.failing = false
	recorded, err := store.RecordPermission(draft)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.RevisionNumber != core.FirstAssistPermissionRevisionNumber {
		t.Errorf("revision number = %d, want the first number after the failed write", recorded.RevisionNumber)
	}
}

// 読み戻した改訂は提供者ごとに 1 から順に続く。飛んだ番号を持つ記録は受け付けない。
func TestJournaledAssistStoreRestoresTheRevisionsInOrder(t *testing.T) {
	grant := core.AssistPermissionRevision{
		Provider: core.AssistProviderClaude, RevisionNumber: 1, Action: core.AssistPermissionActionGrant,
		Analyst: "analyst-a", RecordedAt: core.NewAssertionTime(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	revoke := grant
	revoke.RevisionNumber, revoke.Action = 2, core.AssistPermissionActionRevoke
	store, err := newJournaledAssistStore(&fixedAssistClock{}, assistJournal{},
		assistRecords{permissions: []core.AssistPermissionRevision{grant, revoke}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.RecordPermission(AssistPermissionDraft{
		Provider: core.AssistProviderClaude, Action: core.AssistPermissionActionGrant, Analyst: "analyst-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.RevisionNumber != 3 {
		t.Errorf("next revision number = %d, want 3", next.RevisionNumber)
	}
	skipped := revoke
	skipped.RevisionNumber = 3
	if _, err := newJournaledAssistStore(&fixedAssistClock{}, assistJournal{},
		assistRecords{permissions: []core.AssistPermissionRevision{grant, skipped}}); err == nil {
		t.Fatal("restoring a skipped revision number succeeded")
	}
}

// 許可の無い調査は会話を発行しない。許可した後は発行し、発行した時点の許可の改訂を持つ。
func TestOpenConversationRequiresThePermission(t *testing.T) {
	store := newTestAssistStore(t, &recordingAssistJournal{})
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); !errors.Is(err, ErrAssistNotPermitted) {
		t.Fatalf("OpenConversation without a permission = %v, want ErrAssistNotPermitted", err)
	}
	recordTestPermission(t, store, core.AssistPermissionActionGrant)
	conversation, err := store.OpenConversation(core.AssistProviderClaude, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.Id != testConversationId || conversation.PermissionRevision != 1 ||
		conversation.Model != "model-a" {
		t.Errorf("conversation = %+v", conversation)
	}
	recordTestPermission(t, store, core.AssistPermissionActionRevoke)
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); !errors.Is(err, ErrAssistNotPermitted) {
		t.Errorf("OpenConversation after the revocation = %v, want ErrAssistNotPermitted", err)
	}
}

// 受け渡しは本文の sha256 と長さを記録し、会話の途中で取り消すと以後の受け渡しを退ける。
func TestDiscloseRecordsTheBodyAndStopsAfterTheRevocation(t *testing.T) {
	journal := &recordingAssistJournal{}
	store := newTestAssistStore(t, journal)
	recordTestPermission(t, store, core.AssistPermissionActionGrant)
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); err != nil {
		t.Fatal(err)
	}
	registerTestTurn(t, store, "turn-1")
	body := []byte(`{"records":[{"ref":"r1"}]}`)
	disclosure, sent, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: testConversationId, TurnId: "turn-1", Tool: core.AssistToolRecords,
		Request: `{"refs":["r1"]}`, Versions: testVersions(),
	}, func(issuer AssistRefIssuer) (AssistComposition, error) {
		ref := issuer.Record(testRecordTarget(3))
		return AssistComposition{Body: body, RecordRefs: []string{ref}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(body)
	if string(sent) != string(body) || disclosure.BodySha256 != hex.EncodeToString(want[:]) ||
		disclosure.BodyBytes != int64(len(body)) {
		t.Errorf("disclosure = %+v, want the sha256 and the length of the sent body", disclosure)
	}
	if !slices.Equal(disclosure.RecordRefs, []string{"r1"}) || disclosure.Versions.PermissionRevision != 1 ||
		len(disclosure.Versions.MatchConditions) != 1 {
		t.Errorf("disclosure = %+v, want the record reference, the permission and the turn conditions",
			disclosure)
	}
	if len(journal.disclosures) != 2 || journal.disclosures[1].BodySha256 != disclosure.BodySha256 {
		t.Errorf("journal = %+v, want the turn and the records disclosures", journal.disclosures)
	}
	recordTestPermission(t, store, core.AssistPermissionActionRevoke)
	composed := false
	if _, _, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: testConversationId, TurnId: "turn-1", Tool: core.AssistToolOverview, Request: "{}",
		Versions: testVersions(),
	}, func(AssistRefIssuer) (AssistComposition, error) {
		composed = true
		return AssistComposition{Body: []byte("{}")}, nil
	}); !errors.Is(err, ErrAssistNotPermitted) || composed {
		t.Errorf("Disclose after the revocation = %v (composed %v), want ErrAssistNotPermitted", err, composed)
	}
}

// 記録を書けなかった受け渡しは本文を返さず、仮に発行した短い参照を捨てる。
func TestDiscloseDropsTheReferencesOfAFailedWrite(t *testing.T) {
	journal := &recordingAssistJournal{}
	store := newTestAssistStore(t, journal)
	recordTestPermission(t, store, core.AssistPermissionActionGrant)
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); err != nil {
		t.Fatal(err)
	}
	registerTestTurn(t, store, "turn-1")
	disclose := func() (core.AssistDisclosure, []byte, error) {
		return store.Disclose(AssistDisclosureRequest{
			ConversationId: testConversationId, TurnId: "turn-1", Tool: core.AssistToolGraphSearch,
			Request: "{}", Versions: testVersions(),
		}, func(issuer AssistRefIssuer) (AssistComposition, error) {
			return AssistComposition{Body: []byte(issuer.Node("n:process:1") + issuer.Record(testRecordTarget(7)))}, nil
		})
	}
	journal.failing = true
	if _, body, err := disclose(); !errors.Is(err, ErrAssertionStoreFailure) || body != nil {
		t.Fatalf("Disclose with a failing journal = %q, %v, want no body and the store failure", body, err)
	}
	if _, found, _ := store.ResolveRef(testConversationId, "n1"); found {
		t.Fatal("the failed write kept its short reference")
	}
	journal.failing = false
	_, body, err := disclose()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "n1r1" {
		t.Errorf("body = %q, want the first references after the failed write", body)
	}
	// 同じ会話で同じものは同じ参照になる。
	_, again, err := disclose()
	if err != nil || string(again) != "n1r1" {
		t.Errorf("second body = %q, %v, want the same references", again, err)
	}
	resolved, found, err := store.ResolveRef(testConversationId, "r1")
	if err != nil || !found || resolved.Record == nil || *resolved.Record.Record.LineNumber != 7 {
		t.Errorf("ResolveRef(r1) = %+v, %v, %v", resolved, found, err)
	}
}

// 登録していない発言の受け渡しと、同じ発言の 2 回目の登録を退ける。
func TestDiscloseRequiresARegisteredTurn(t *testing.T) {
	store := newTestAssistStore(t, &recordingAssistJournal{})
	recordTestPermission(t, store, core.AssistPermissionActionGrant)
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); err != nil {
		t.Fatal(err)
	}
	compose := func(AssistRefIssuer) (AssistComposition, error) { return AssistComposition{Body: []byte("{}")}, nil }
	if _, _, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: testConversationId, TurnId: "turn-9", Tool: core.AssistToolOverview, Request: "{}",
		Versions: testVersions(),
	}, compose); !errors.Is(err, ErrAssistTurnNotFound) {
		t.Errorf("Disclose on an unregistered turn = %v, want ErrAssistTurnNotFound", err)
	}
	registerTestTurn(t, store, "turn-1")
	turn := core.AssistTurn{ConversationId: testConversationId, TurnId: "turn-1"}
	if _, _, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: testConversationId, TurnId: "turn-1", Tool: core.AssistToolTurn, Request: "{}",
		Versions: testVersions(), Turn: &turn,
	}, compose); !errors.Is(err, ErrAssistTurnExists) {
		t.Errorf("registering the turn twice = %v, want ErrAssistTurnExists", err)
	}
	if _, _, err := store.Disclose(AssistDisclosureRequest{
		ConversationId: "ffffffffffffffffffffffffffffffff", TurnId: "turn-1", Tool: core.AssistToolOverview,
		Request: "{}", Versions: testVersions(),
	}, compose); !errors.Is(err, ErrAssistConversationNotFound) {
		t.Errorf("Disclose on an unknown conversation = %v, want ErrAssistConversationNotFound", err)
	}
}

// 読み戻した会話では、発行した参照を探せ、次の参照は読み戻した通番の次から発行する。
func TestJournaledAssistStoreRestoresTheConversation(t *testing.T) {
	journal := &recordingAssistJournal{}
	store := newTestAssistStore(t, journal)
	recordTestPermission(t, store, core.AssistPermissionActionGrant)
	conversation, err := store.OpenConversation(core.AssistProviderClaude, "")
	if err != nil {
		t.Fatal(err)
	}
	registerTestTurn(t, store, "turn-1")
	compose := func(line int64) func(AssistRefIssuer) (AssistComposition, error) {
		return func(issuer AssistRefIssuer) (AssistComposition, error) {
			return AssistComposition{Body: []byte(issuer.Record(testRecordTarget(line)))}, nil
		}
	}
	request := AssistDisclosureRequest{ConversationId: testConversationId, TurnId: "turn-1",
		Tool: core.AssistToolRecords, Request: "{}", Versions: testVersions()}
	if _, _, err := store.Disclose(request, compose(1)); err != nil {
		t.Fatal(err)
	}
	permissions, _ := store.PermissionRevisions()
	restored, err := newJournaledAssistStore(&fixedAssistClock{}, assistJournal{}, assistRecords{
		permissions: permissions, conversations: []core.AssistConversation{conversation}, turns: journal.turns,
		refs: map[string][]core.AssistShortRef{testConversationId: journal.refs}, disclosures: journal.disclosures,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref, found, err := restored.ResolveRef(testConversationId, "r1"); err != nil || !found ||
		*ref.Record.Record.LineNumber != 1 {
		t.Fatalf("ResolveRef(r1) after the restoration = %+v, %v, %v", ref, found, err)
	}
	_, body, err := restored.Disclose(request, compose(2))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "r2" {
		t.Errorf("next reference = %q, want r2", body)
	}
	disclosures, _ := restored.Disclosures(testConversationId)
	if len(disclosures) != 3 || disclosures[2].Ordinal != 3 {
		t.Errorf("disclosures = %+v, want the restored two and the new third", disclosures)
	}
}

// 調査の保存先を持たない起動の保存先は、読み書きのすべてを退ける。
func TestUnavailableAssistStoreRefusesEverything(t *testing.T) {
	store := UnavailableAssistStore()
	if store.Available() {
		t.Fatal("the unavailable store reports that it is available")
	}
	if _, err := store.PermissionRevisions(); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("PermissionRevisions = %v", err)
	}
	if _, err := store.RecordPermission(AssistPermissionDraft{}); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("RecordPermission = %v", err)
	}
	if _, err := store.OpenConversation(core.AssistProviderClaude, ""); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("OpenConversation = %v", err)
	}
	if !NewMemoryAssistStore(&fixedAssistClock{}).Available() {
		t.Error("the memory store reports that it is unavailable")
	}
}
