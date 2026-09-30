package pipeline

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ErrAssistUnavailable は、AI 支援の記録を置く調査の保存先を持たない起動であることを表す。
//
// **停止で消える記録を監査の記録として扱わない。** 調査の directory を渡さない起動は、送信の
// 許可も受け渡しの記録も受け付けない。
var ErrAssistUnavailable = errors.New("assist store: the launch keeps no investigation to record the assist")

// ErrAssistNotPermitted は、会話の提供者への送信を現在の改訂が許可していないことを表す。
var ErrAssistNotPermitted = errors.New("assist store: the investigation does not permit sending to the provider")

// ErrAssistConversationNotFound は、識別子に一致する会話が無いことを表す。
var ErrAssistConversationNotFound = errors.New("assist store: the identifier matches no conversation")

// ErrAssistTurnNotFound は、会話に該当する発言が登録されていないことを表す。
var ErrAssistTurnNotFound = errors.New("assist store: the conversation has no such turn")

// ErrAssistTurnExists は、同じ発言を 2 回登録しようとしたことを表す。
var ErrAssistTurnExists = errors.New("assist store: the conversation already registered the turn")

// AssistPermissionDraft は、送信の許可の新しい改訂 1 つの入力である。改訂の番号と時刻は保存先が
// 決める。
type AssistPermissionDraft struct {
	// Provider は証拠を送る提供者である。
	Provider core.AssistProvider
	// Action は許可と取り消しのどちらを記録するかである。
	Action core.AssistPermissionAction
	// Analyst は改訂を記録する分析者の名前である。
	Analyst string
}

// AssistRefIssuer は、受け渡しの本文に載せる記録・ノード・関係へ、会話の中の短い参照を発行する。
//
// 同じ会話で同じものに発行した参照は同じ文字列を返す。発行は受け渡しの記録と同じ transaction で
// 確定し、記録を書けなかった受け渡しの参照は捨てる。
type AssistRefIssuer interface {
	Record(target core.AssistRecordTarget) string
	Node(nodeId string) string
	Edge(edgeId string) string
}

// AssistComposition は、受け渡しの本文と、本文に載せた記録の短い参照である。
type AssistComposition struct {
	Body       []byte
	RecordRefs []string
	Truncated  bool
}

// AssistDisclosureRequest は受け渡し 1 回の入力である。位置、時刻、許可の改訂、関連付けの条件は
// 保存先が決める。
type AssistDisclosureRequest struct {
	ConversationId string
	TurnId         string
	Tool           core.AssistTool
	// Request は要求の本文の JSON の文字列である。
	Request string
	// Versions は本文を組み直すのに要る commit、改訂の番号、更新回数である。PermissionRevision と
	// MatchConditions は保存先が埋める。
	Versions core.AssistVersions
	// Turn は、Tool が発言の登録のときに登録する発言である。
	Turn *core.AssistTurn
}

// AssistStore は AI 支援の記録を保つ保存先の port である。
type AssistStore interface {
	// Available は、この起動が AI 支援の記録を置けるかを返す。偽の保存先は、読み書きのすべてに
	// ErrAssistUnavailable を返す。
	Available() bool
	// PermissionRevisions は、送信の許可の改訂を記録した順に返す。
	PermissionRevisions() ([]core.AssistPermissionRevision, error)
	// RecordPermission は送信の許可の新しい改訂を記録し、記録した改訂を返す。
	RecordPermission(draft AssistPermissionDraft) (core.AssistPermissionRevision, error)
	// OpenConversation は、提供者への送信を現在の改訂が許可しているときに会話を発行する。
	OpenConversation(provider core.AssistProvider, model string) (core.AssistConversation, error)
	// Turn は会話に登録した発言を返す。
	Turn(conversationId, turnId string) (core.AssistTurn, error)
	// Permitted は、会話の提供者への送信を現在の改訂が許可しているときに nil を返す。受け渡しを
	// 記録しない操作 (検索の条件の検証) が、取り消しの後の要求を退けるのに使う。
	Permitted(conversationId string) error
	// ResolveRef は会話の中で発行した短い参照を探す。ok が偽になるのは、この会話で発行して
	// いない参照である。
	ResolveRef(conversationId, ref string) (core.AssistShortRef, bool, error)
	// Disclose は、許可を確かめてから compose で本文を組み、受け渡しの記録を書いた後に本文を返す。
	//
	// **記録を書けなかった本文を返さない。** compose は保存先の lock の中で呼ばれるため、グラフの
	// 組み立てのような待ちを持たず、組み終えた値から本文を作るだけにする。
	Disclose(
		request AssistDisclosureRequest, compose func(AssistRefIssuer) (AssistComposition, error),
	) (core.AssistDisclosure, []byte, error)
	// Disclosures は会話の受け渡しの記録を記録した順に返す。
	Disclosures(conversationId string) ([]core.AssistDisclosure, error)
}

// assistJournal は、AI 支援の記録をメモリに確定する前に調査の保存先へ書く関数の組である。
//
// **保存先へ書けた記録だけを確定する。** 書けなかった許可をメモリにだけ残すと、次の起動で
// 許可の状態が変わる。
type assistJournal struct {
	// permissionRecorded は送信の許可の新しい改訂を書く。
	permissionRecorded func(revision core.AssistPermissionRevision) error
	// conversationOpened は発行した会話を書く。
	conversationOpened func(conversation core.AssistConversation) error
	// disclosed は受け渡しの記録と、同じ受け渡しで登録した発言と発行した短い参照を 1 つの
	// transaction で書く。
	disclosed func(disclosure core.AssistDisclosure, turn *core.AssistTurn, refs []core.AssistShortRef) error
}

// assistRecords は、調査の保存先から読み戻した AI 支援の記録である。
type assistRecords struct {
	permissions   []core.AssistPermissionRevision
	conversations []core.AssistConversation
	turns         []core.AssistTurn
	// refs は会話の識別子から、その会話で発行した短い参照を発行した順に取り出す。
	refs        map[string][]core.AssistShortRef
	disclosures []core.AssistDisclosure
}

// assistConversationState は会話 1 件の、発言と短い参照の索引である。
type assistConversationState struct {
	conversation core.AssistConversation
	turns        map[string]core.AssistTurn
	// refs は短い参照の文字列から、それが指すものを探す。
	refs map[string]core.AssistShortRef
	// issued は指すものの鍵から、発行した短い参照を探す。
	issued map[string]string
	// ordinals は種類ごとの最後の通番である。
	ordinals map[core.AssistRefKind]int64
}

// MemoryAssistStore は AssistStore をメモリの上で満たす。journal を持つ保存先は、記録を
// メモリに確定する前に調査の保存先へ書く。
type MemoryAssistStore struct {
	clock   AssertionClock
	journal assistJournal
	// newId は会話の識別子を作る。test は固定の値を返す関数に差し替える。
	newId func() (string, error)
	mu    sync.Mutex
	// permissions は送信の許可の改訂を記録した順に持つ。
	permissions   []core.AssistPermissionRevision
	conversations map[string]*assistConversationState
	disclosures   []core.AssistDisclosure
}

// NewMemoryAssistStore は、記録を調査の保存先へ書かずにメモリにだけ保つ保存先を返す。
func NewMemoryAssistStore(clock AssertionClock) *MemoryAssistStore {
	return &MemoryAssistStore{
		clock: clock, newId: newAssistConversationId, conversations: map[string]*assistConversationState{},
	}
}

// newJournaledAssistStore は、調査の保存先から読み戻した記録を持ち、以後の記録を journal へ書く
// 保存先を返す。
func newJournaledAssistStore(
	clock AssertionClock, journal assistJournal, records assistRecords,
) (*MemoryAssistStore, error) {
	store := NewMemoryAssistStore(clock)
	store.journal = journal
	for _, revision := range records.permissions {
		if err := revision.Validate(); err != nil {
			return nil, fmt.Errorf("restoring the assist permissions: %w", err)
		}
		if revision.RevisionNumber != store.nextRevisionNumber(revision.Provider) {
			return nil, fmt.Errorf("restoring the assist permissions: the revision %d of %s is out of order: %w",
				revision.RevisionNumber, revision.Provider, ErrAssertionStoreFailure)
		}
		store.permissions = append(store.permissions, revision)
	}
	if err := store.restoreConversations(records); err != nil {
		return nil, fmt.Errorf("restoring the assist conversations: %w", err)
	}
	return store, nil
}

// restoreConversations は読み戻した会話、発言、短い参照、受け渡しの記録を索引に入れる。
func (s *MemoryAssistStore) restoreConversations(records assistRecords) error {
	for _, conversation := range records.conversations {
		if err := conversation.Validate(); err != nil {
			return err
		}
		s.conversations[conversation.Id] = newAssistConversationState(conversation)
	}
	for _, turn := range records.turns {
		if err := turn.Validate(); err != nil {
			return err
		}
		state, found := s.conversations[turn.ConversationId]
		if !found {
			return fmt.Errorf("a turn has no conversation: %w", ErrAssertionStoreFailure)
		}
		state.turns[turn.TurnId] = turn
	}
	for conversationId, refs := range records.refs {
		state, found := s.conversations[conversationId]
		if !found {
			return fmt.Errorf("a short reference has no conversation: %w", ErrAssertionStoreFailure)
		}
		for _, ref := range refs {
			if err := state.restoreRef(ref); err != nil {
				return err
			}
		}
	}
	for index, disclosure := range records.disclosures {
		if err := disclosure.Validate(); err != nil {
			return err
		}
		if disclosure.Ordinal != int64(index)+1 {
			return fmt.Errorf("the disclosure %d is out of order: %w", disclosure.Ordinal, ErrAssertionStoreFailure)
		}
		s.disclosures = append(s.disclosures, disclosure)
	}
	return nil
}

// Available は、記録を保てることを返す。
func (s *MemoryAssistStore) Available() bool { return true }

// PermissionRevisions は送信の許可の改訂を記録した順に返す。
func (s *MemoryAssistStore) PermissionRevisions() ([]core.AssistPermissionRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.permissions), nil
}

// RecordPermission は送信の許可の新しい改訂を記録する。
//
// **改訂の番号は記録と同じ lock の中で決める。** 同時に届いた 2 つの改訂が同じ番号を取らない。
func (s *MemoryAssistStore) RecordPermission(
	draft AssistPermissionDraft,
) (core.AssistPermissionRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	number := s.nextRevisionNumber(draft.Provider)
	if number == math.MaxInt64 {
		return core.AssistPermissionRevision{}, fmt.Errorf(
			"recording the assist permission: the revisions are exhausted: %w", ErrAssertionStoreFailure)
	}
	revision := core.AssistPermissionRevision{
		Provider: draft.Provider, RevisionNumber: number, Action: draft.Action,
		Analyst: draft.Analyst, RecordedAt: s.clock.Now(),
	}
	if err := revision.Validate(); err != nil {
		return core.AssistPermissionRevision{}, fmt.Errorf("recording the assist permission: %w", err)
	}
	if s.journal.permissionRecorded != nil {
		if err := s.journal.permissionRecorded(revision); err != nil {
			return core.AssistPermissionRevision{}, fmt.Errorf(
				"recording the assist permission: %w: %w", ErrAssertionStoreFailure, err)
		}
	}
	s.permissions = append(s.permissions, revision)
	return revision, nil
}

// nextRevisionNumber は provider の次の改訂の番号を返す。呼び出し元が s.mu を持つ。
func (s *MemoryAssistStore) nextRevisionNumber(provider core.AssistProvider) int64 {
	return s.currentPermissionRevision(provider) + 1
}

// currentPermissionRevision は provider の最後の改訂の番号を返す。改訂を持たない提供者では 0 を返す。
// 呼び出し元が s.mu を持つ。
func (s *MemoryAssistStore) currentPermissionRevision(provider core.AssistProvider) int64 {
	last := core.FirstAssistPermissionRevisionNumber - 1
	for _, revision := range s.permissions {
		if revision.Provider == provider {
			last = revision.RevisionNumber
		}
	}
	return last
}

// permittedRevision は、provider への送信を現在の改訂が許可していればその改訂の番号を返す。
// 呼び出し元が s.mu を持つ。
func (s *MemoryAssistStore) permittedRevision(provider core.AssistProvider) (int64, error) {
	if !core.AssistPermitted(s.permissions, provider) {
		return 0, ErrAssistNotPermitted
	}
	return s.currentPermissionRevision(provider), nil
}

// OpenConversation は会話を発行する。許可は発行と同じ lock の中で確かめる。
func (s *MemoryAssistStore) OpenConversation(
	provider core.AssistProvider, model string,
) (core.AssistConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	permission, err := s.permittedRevision(provider)
	if err != nil {
		return core.AssistConversation{}, err
	}
	id, err := s.newId()
	if err != nil {
		return core.AssistConversation{}, fmt.Errorf("opening the conversation: %w: %w", ErrAssertionStoreFailure, err)
	}
	if _, taken := s.conversations[id]; taken {
		return core.AssistConversation{}, fmt.Errorf(
			"opening the conversation: the identifier is already taken: %w", ErrAssertionStoreFailure)
	}
	conversation := core.AssistConversation{
		Id: id, Provider: provider, Model: model, PermissionRevision: permission, CreatedAt: s.clock.Now(),
	}
	if err := conversation.Validate(); err != nil {
		return core.AssistConversation{}, fmt.Errorf("opening the conversation: %w", err)
	}
	if s.journal.conversationOpened != nil {
		if err := s.journal.conversationOpened(conversation); err != nil {
			return core.AssistConversation{}, fmt.Errorf(
				"opening the conversation: %w: %w", ErrAssertionStoreFailure, err)
		}
	}
	s.conversations[id] = newAssistConversationState(conversation)
	return conversation, nil
}

// Turn は会話に登録した発言を返す。
func (s *MemoryAssistStore) Turn(conversationId, turnId string) (core.AssistTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, found := s.conversations[conversationId]
	if !found {
		return core.AssistTurn{}, ErrAssistConversationNotFound
	}
	turn, found := state.turns[turnId]
	if !found {
		return core.AssistTurn{}, ErrAssistTurnNotFound
	}
	return turn, nil
}

// Permitted は、会話の提供者への送信を現在の改訂が許可しているかを確かめる。
func (s *MemoryAssistStore) Permitted(conversationId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, found := s.conversations[conversationId]
	if !found {
		return ErrAssistConversationNotFound
	}
	_, err := s.permittedRevision(state.conversation.Provider)
	return err
}

// ResolveRef は会話の中で発行した短い参照を探す。
func (s *MemoryAssistStore) ResolveRef(conversationId, ref string) (core.AssistShortRef, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, found := s.conversations[conversationId]
	if !found {
		return core.AssistShortRef{}, false, ErrAssistConversationNotFound
	}
	resolved, found := state.refs[ref]
	return resolved, found, nil
}

// Disclose は許可を確かめ、本文を組み、受け渡しの記録を書いてから本文を返す。
//
// 既知の制限: 受け渡しの全体を保存先の 1 つの lock で並べる, compose は組み終えたグラフから本文を
// 作るだけで、調査の file への書き込みは 1 回の transaction である。同時に届く受け渡しは分析者
// 1 人の会話の tool の呼び出しであり、並べて待つ時間は測っていない, 複数の分析者が同じ server で
// 同時に会話して応答の遅れを測ったときに見直す
func (s *MemoryAssistStore) Disclose(
	request AssistDisclosureRequest, compose func(AssistRefIssuer) (AssistComposition, error),
) (core.AssistDisclosure, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, found := s.conversations[request.ConversationId]
	if !found {
		return core.AssistDisclosure{}, nil, ErrAssistConversationNotFound
	}
	permission, err := s.permittedRevision(state.conversation.Provider)
	if err != nil {
		return core.AssistDisclosure{}, nil, err
	}
	turn, err := state.turnOf(request)
	if err != nil {
		return core.AssistDisclosure{}, nil, err
	}
	issuer := &assistRefIssuer{state: state, issued: map[string]string{}, ordinals: map[core.AssistRefKind]int64{}}
	composition, err := compose(issuer)
	if err != nil {
		return core.AssistDisclosure{}, nil, err
	}
	digest := sha256.Sum256(composition.Body)
	versions := request.Versions
	versions.PermissionRevision = permission
	versions.MatchConditions = slices.Clone(turn.MatchConditions)
	disclosure := core.AssistDisclosure{
		Ordinal: int64(len(s.disclosures)) + 1, ConversationId: request.ConversationId, TurnId: request.TurnId,
		Tool: request.Tool, Request: request.Request, RecordRefs: slices.Clone(composition.RecordRefs),
		Truncated: composition.Truncated, BodySha256: hex.EncodeToString(digest[:]),
		BodyBytes: int64(len(composition.Body)), Versions: versions, RecordedAt: s.clock.Now(),
	}
	if disclosure.RecordRefs == nil {
		disclosure.RecordRefs = []string{}
	}
	if err := disclosure.Validate(); err != nil {
		return core.AssistDisclosure{}, nil, fmt.Errorf("recording the disclosure: %w", err)
	}
	var registered *core.AssistTurn
	if request.Tool == core.AssistToolTurn {
		registered = &turn
	}
	if s.journal.disclosed != nil {
		if err := s.journal.disclosed(disclosure, registered, issuer.refs); err != nil {
			return core.AssistDisclosure{}, nil, fmt.Errorf(
				"recording the disclosure: %w: %w", ErrAssertionStoreFailure, err)
		}
	}
	issuer.settle()
	if registered != nil {
		state.turns[registered.TurnId] = *registered
	}
	s.disclosures = append(s.disclosures, disclosure)
	return disclosure, composition.Body, nil
}

// Disclosures は会話の受け渡しの記録を記録した順に返す。
func (s *MemoryAssistStore) Disclosures(conversationId string) ([]core.AssistDisclosure, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.conversations[conversationId]; !found {
		return nil, ErrAssistConversationNotFound
	}
	var disclosures []core.AssistDisclosure
	for _, disclosure := range s.disclosures {
		if disclosure.ConversationId == conversationId {
			disclosures = append(disclosures, disclosure)
		}
	}
	return disclosures, nil
}

func newAssistConversationState(conversation core.AssistConversation) *assistConversationState {
	return &assistConversationState{
		conversation: conversation, turns: map[string]core.AssistTurn{},
		refs: map[string]core.AssistShortRef{}, issued: map[string]string{}, ordinals: map[core.AssistRefKind]int64{},
	}
}

// turnOf は、受け渡しが従う発言を返す。発言の登録では、同じ発言が未登録であることを確かめる。
func (c *assistConversationState) turnOf(request AssistDisclosureRequest) (core.AssistTurn, error) {
	existing, registered := c.turns[request.TurnId]
	if request.Tool != core.AssistToolTurn {
		if !registered {
			return core.AssistTurn{}, ErrAssistTurnNotFound
		}
		return existing, nil
	}
	if registered {
		return core.AssistTurn{}, ErrAssistTurnExists
	}
	if request.Turn == nil || request.Turn.TurnId != request.TurnId ||
		request.Turn.ConversationId != request.ConversationId {
		return core.AssistTurn{}, fmt.Errorf("registering the turn: the turn does not match the request: %w",
			core.ErrInconsistentValue)
	}
	if err := request.Turn.Validate(); err != nil {
		return core.AssistTurn{}, fmt.Errorf("registering the turn: %w", err)
	}
	return *request.Turn, nil
}

// restoreRef は読み戻した短い参照を索引に入れ、種類ごとの通番を進める。
func (c *assistConversationState) restoreRef(ref core.AssistShortRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if _, taken := c.refs[ref.Ref]; taken {
		return fmt.Errorf("the short reference is recorded twice: %w", ErrAssertionStoreFailure)
	}
	c.refs[ref.Ref] = ref
	c.issued[assistRefKeyOf(ref)] = ref.Ref
	c.ordinals[ref.Kind]++
	if ref.Ref != core.AssistShortRefOf(ref.Kind, c.ordinals[ref.Kind]) {
		return fmt.Errorf("the short reference is out of order: %w", ErrAssertionStoreFailure)
	}
	return nil
}

// assistRefKeyOf は、短い参照が指すものを文字列の鍵にする。同じものは同じ鍵になる。
func assistRefKeyOf(ref core.AssistShortRef) string {
	switch ref.Kind {
	case core.AssistRefKindRecord:
		// **位置の指し方の主の位置だけを鍵に入れる。** レコードのノードの識別鍵は主の位置だけを
		// 持ち、記録の位置は行番号と byte の位置も持つ。同じ記録を同じ参照にする。
		record := ref.Record.Record
		var primary *int64
		switch record.PositionKind {
		case core.PositionKindSequenceNumber:
			primary = record.SequenceNumber
		case core.PositionKindLineNumber:
			primary = record.LineNumber
		case core.PositionKindByteRange:
			primary = record.ByteOffset
		}
		return identityDigest([]string{"record", ref.Record.SourceId, record.SourceContentSha256,
			string(record.PositionKind), assertionPositionText(primary)})
	case core.AssistRefKindNode:
		return "node\x00" + ref.NodeId
	default:
		return "edge\x00" + ref.EdgeId
	}
}

// assistRefIssuer は、受け渡し 1 回の中で発行した短い参照を仮に持つ。受け渡しの記録を書けた
// 後に settle で会話の索引へ移す。
type assistRefIssuer struct {
	state *assistConversationState
	// issued と ordinals は、この受け渡しで仮に発行した参照と、種類ごとの通番の増分である。
	issued   map[string]string
	ordinals map[core.AssistRefKind]int64
	refs     []core.AssistShortRef
}

func (i *assistRefIssuer) Record(target core.AssistRecordTarget) string {
	return i.issue(core.AssistShortRef{Kind: core.AssistRefKindRecord, Record: &target})
}

func (i *assistRefIssuer) Node(nodeId string) string {
	return i.issue(core.AssistShortRef{Kind: core.AssistRefKindNode, NodeId: nodeId})
}

func (i *assistRefIssuer) Edge(edgeId string) string {
	return i.issue(core.AssistShortRef{Kind: core.AssistRefKindEdge, EdgeId: edgeId})
}

// issue は、会話で既に発行した参照があればそれを返し、無ければ次の通番の参照を仮に発行する。
func (i *assistRefIssuer) issue(ref core.AssistShortRef) string {
	key := assistRefKeyOf(ref)
	if existing, found := i.state.issued[key]; found {
		return existing
	}
	if existing, found := i.issued[key]; found {
		return existing
	}
	i.ordinals[ref.Kind]++
	ref.Ref = core.AssistShortRefOf(ref.Kind, i.state.ordinals[ref.Kind]+i.ordinals[ref.Kind])
	i.issued[key] = ref.Ref
	i.refs = append(i.refs, ref)
	return ref.Ref
}

// settle は仮に発行した参照を会話の索引へ移す。
func (i *assistRefIssuer) settle() {
	for _, ref := range i.refs {
		i.state.refs[ref.Ref] = ref
		i.state.issued[assistRefKeyOf(ref)] = ref.Ref
	}
	for kind, added := range i.ordinals {
		i.state.ordinals[kind] += added
	}
}

// newAssistConversationId は乱数 128 bit から会話の識別子を作る。
func newAssistConversationId() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("creating the conversation id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

// unavailableAssistStore は、調査の保存先を持たない起動の AssistStore である。
type unavailableAssistStore struct{}

// Available は偽を返す。
func (unavailableAssistStore) Available() bool { return false }

// PermissionRevisions は ErrAssistUnavailable を返す。
func (unavailableAssistStore) PermissionRevisions() ([]core.AssistPermissionRevision, error) {
	return nil, ErrAssistUnavailable
}

// RecordPermission は ErrAssistUnavailable を返す。
func (unavailableAssistStore) RecordPermission(AssistPermissionDraft) (core.AssistPermissionRevision, error) {
	return core.AssistPermissionRevision{}, ErrAssistUnavailable
}

// OpenConversation は ErrAssistUnavailable を返す。
func (unavailableAssistStore) OpenConversation(core.AssistProvider, string) (core.AssistConversation, error) {
	return core.AssistConversation{}, ErrAssistUnavailable
}

// Turn は ErrAssistUnavailable を返す。
func (unavailableAssistStore) Turn(string, string) (core.AssistTurn, error) {
	return core.AssistTurn{}, ErrAssistUnavailable
}

// Permitted は ErrAssistUnavailable を返す。
func (unavailableAssistStore) Permitted(string) error { return ErrAssistUnavailable }

// ResolveRef は ErrAssistUnavailable を返す。
func (unavailableAssistStore) ResolveRef(string, string) (core.AssistShortRef, bool, error) {
	return core.AssistShortRef{}, false, ErrAssistUnavailable
}

// Disclose は ErrAssistUnavailable を返す。
func (unavailableAssistStore) Disclose(
	AssistDisclosureRequest, func(AssistRefIssuer) (AssistComposition, error),
) (core.AssistDisclosure, []byte, error) {
	return core.AssistDisclosure{}, nil, ErrAssistUnavailable
}

// Disclosures は ErrAssistUnavailable を返す。
func (unavailableAssistStore) Disclosures(string) ([]core.AssistDisclosure, error) {
	return nil, ErrAssistUnavailable
}

// UnavailableAssistStore は、調査の保存先を持たない起動の AssistStore を返す。
func UnavailableAssistStore() AssistStore { return unavailableAssistStore{} }
