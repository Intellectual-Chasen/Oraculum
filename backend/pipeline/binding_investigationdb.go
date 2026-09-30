package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Investigation は調査 1 件の保存先を、取り込みの前後で扱う。
//
// 使い方は 3 段階である。PrepareInvestigation で開き (または作成を準備し)、RecordedPlans と
// Ordinals を取り込みに渡し、取り込みの後に Settle で新しい収集元を記録して store を得る。
//
// **原資料とパース結果を保存先に置かない。** 開くたびに、記録した取り込みの指定から原資料を
// 読み直して組み直す。記録した通番を同じ順で発行するため、sourceId と、それを指す割当が
// 開き直した後も同じ値になる。
type Investigation struct {
	dir string
	// sourceRoot は調査が記録する基準である。
	sourceRoot string
	// openRoot は、この起動の間だけ sourceRoot の代わりに使う基準である。空文字列は sourceRoot を使う。
	openRoot string
	// db は開いた保存先である。作成を準備した調査では Settle まで nil である。
	db       *investigationdb.DB
	recorded []investigationdb.Source
	// skipped は、記録した収集の directory の取り込まなかった file である。
	skipped     []core.SkippedFile
	assertions  []recordedAssertion
	proposals   []recordedAssistProposal
	assignments []core.TerminalAssignment
	ordinals    *recordedOrdinals
	// access は利用者の役割である。作成を準備した調査では、Settle まで変更をメモリに溜める。
	access *AccessStore
	// workspaces は保存したワークスペースである。作成を準備した調査では、Settle までメモリに置く。
	workspaces *WorkspaceStore
	// assist は記録した AI 支援の記録である。
	assist assistRecords
}

// PrepareInvestigation は dir の調査を開く。dir に調査が無ければ、作成を準備する。
//
// 基準は取り込みの指定の OriginPath を解決する絶対 path である。newRoot は作成する調査の基準で
// あり、Settle が記録する。overrideRoot は、空文字列でなければ、開いた調査ではその起動の間だけ
// 記録した基準の代わりに使い、記録しない。収集物を別の場所へ移した調査を開くためである。
// 作成する調査では overrideRoot を newRoot の代わりに記録する。取り込みに使った基準を記録する。
func PrepareInvestigation(ctx context.Context, dir, newRoot, overrideRoot string) (*Investigation, error) {
	for _, root := range []string{newRoot, overrideRoot} {
		if root != "" && !filepath.IsAbs(root) {
			return nil, fmt.Errorf("preparing the investigation: the source root %q is not absolute", root)
		}
	}
	exists, err := investigationdb.Exists(dir)
	if err != nil {
		return nil, fmt.Errorf("preparing the investigation: %w", err)
	}
	if !exists {
		if overrideRoot != "" {
			newRoot = overrideRoot
		}
		if newRoot == "" {
			return nil, errors.New("preparing the investigation: a new investigation requires a source root")
		}
		return &Investigation{dir: dir, sourceRoot: newRoot, ordinals: newRecordedOrdinals(nil),
			access: NewAccessStore(nil), workspaces: NewWorkspaceStore(nil)}, nil
	}
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("preparing the investigation: %w", err)
	}
	investigation := &Investigation{
		dir: dir, sourceRoot: contents.Meta.SourceRoot, openRoot: overrideRoot, db: db, recorded: contents.Sources,
		assignments: contents.Assignments, skipped: contents.SkippedFiles,
		ordinals: newRecordedOrdinals(ordinalsOfSources(contents.Sources)),
		assist: assistRecords{
			permissions:   contents.AssistPermissions,
			conversations: contents.AssistConversations.Conversations,
			turns:         contents.AssistConversations.Turns,
			refs:          contents.AssistConversations.Refs,
			disclosures:   contents.AssistConversations.Disclosures,
		},
	}
	for _, stored := range contents.Assertions {
		investigation.assertions = append(investigation.assertions,
			recordedAssertion{assertion: stored.Assertion, ordinal: stored.Ordinal})
	}
	for _, stored := range contents.AssistProposals {
		investigation.proposals = append(investigation.proposals,
			recordedAssistProposal{proposal: stored.Proposal, ordinal: stored.Ordinal})
	}
	members, err := membersOfStored(contents.Members)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("preparing the investigation: %w", err), db.Close())
	}
	investigation.access = NewAccessStore(members)
	investigation.access.journal = accessJournalOf(ctx, db)
	workspaces, changes, err := workspacesOfStored(contents.Workspaces)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("preparing the investigation: %w", err), db.Close())
	}
	investigation.workspaces = NewWorkspaceStore(workspaces)
	investigation.workspaces.changes = changes
	investigation.workspaces.journal = workspaceJournalOf(ctx, db)
	return investigation, nil
}

// Access は利用者の役割の保存先を返す。
func (i *Investigation) Access() *AccessStore { return i.access }

// Workspaces はワークスペースの保存先を返す。
func (i *Investigation) Workspaces() *WorkspaceStore { return i.workspaces }

// workspaceJournalOf はワークスペースの変更を db へ書く journal を返す。
func workspaceJournalOf(ctx context.Context, db *investigationdb.DB) workspaceJournal {
	return func(workspaces []Workspace, removed []string, changes []WorkspaceChange, events []AuditEvent) error {
		stored := make([]investigationdb.Workspace, len(workspaces))
		for index, workspace := range workspaces {
			stored[index] = investigationdb.Workspace{
				Id: workspace.Id, Owner: workspace.Owner, Name: workspace.Name, State: string(workspace.State),
				Revision: workspace.Revision, UpdatedAt: workspace.UpdatedAt.UTC().Format(time.RFC3339Nano),
				UpdatedBy: workspace.UpdatedBy, Shares: make([]investigationdb.WorkspaceShare, len(workspace.Shares)),
			}
			for shareIndex, share := range workspace.Shares {
				stored[index].Shares[shareIndex] = investigationdb.WorkspaceShare{Login: share.Login,
					Access: string(share.Access), GrantedBy: share.GrantedBy,
					GrantedAt: share.GrantedAt.UTC().Format(time.RFC3339Nano)}
			}
		}
		storedChanges := make([]investigationdb.WorkspaceChange, len(changes))
		for index, change := range changes {
			storedChanges[index] = investigationdb.WorkspaceChange{WorkspaceId: change.WorkspaceId,
				Revision: change.Revision, ClientChangeId: change.ClientChangeId, Actor: change.Actor, Fields: change.Fields,
				RecordedAt: change.RecordedAt.UTC().Format(time.RFC3339Nano)}
		}
		return db.ReplaceWorkspaces(context.WithoutCancel(ctx), stored, removed, storedChanges,
			storedAuditEvents(events))
	}
}

// storedAuditEvents は監査の記録を保存先の形へ移す。
func storedAuditEvents(events []AuditEvent) []investigationdb.AuditEvent {
	stored := make([]investigationdb.AuditEvent, len(events))
	for index, event := range events {
		stored[index] = investigationdb.AuditEvent{RecordedAt: event.RecordedAt.UTC().Format(time.RFC3339Nano),
			Actor: event.Actor, Action: event.Action, Target: event.Target, Detail: event.Detail}
	}
	return stored
}

// workspacesOfStored は保存先から読んだワークスペースと、ワークスペースごとの変更の記録を、時刻の値で
// あることを確かめて返す。
func workspacesOfStored(stored []investigationdb.Workspace) ([]Workspace, map[string][]WorkspaceChange, error) {
	workspaces := make([]Workspace, len(stored))
	changes := map[string][]WorkspaceChange{}
	for index, workspace := range stored {
		updatedAt, err := time.Parse(time.RFC3339Nano, workspace.UpdatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("the workspace %q has an unreadable time", workspace.Id)
		}
		for _, change := range workspace.Changes {
			recordedAt, err := time.Parse(time.RFC3339Nano, change.RecordedAt)
			if err != nil {
				return nil, nil, fmt.Errorf("the workspace %q has a change with an unreadable time", workspace.Id)
			}
			changes[workspace.Id] = append(changes[workspace.Id], WorkspaceChange{WorkspaceId: change.WorkspaceId,
				Revision: change.Revision, ClientChangeId: change.ClientChangeId, Actor: change.Actor,
				Fields: change.Fields, RecordedAt: recordedAt})
		}
		workspaces[index] = Workspace{
			Id: workspace.Id, Owner: workspace.Owner, Name: workspace.Name, State: []byte(workspace.State),
			Revision: workspace.Revision, UpdatedAt: updatedAt, UpdatedBy: workspace.UpdatedBy,
		}
		for _, share := range workspace.Shares {
			grantedAt, err := time.Parse(time.RFC3339Nano, share.GrantedAt)
			access := WorkspaceAccess(share.Access)
			if err != nil || (access != WorkspaceView && access != WorkspaceEdit) {
				return nil, nil, fmt.Errorf("the workspace %q has a share with an unknown access or time", workspace.Id)
			}
			workspaces[index].Shares = append(workspaces[index].Shares, WorkspaceShare{Login: share.Login,
				Access: access, GrantedBy: share.GrantedBy, GrantedAt: grantedAt})
		}
	}
	return workspaces, changes, nil
}

// accessJournalOf は役割の変更を db へ書く journal を返す。
func accessJournalOf(ctx context.Context, db *investigationdb.DB) accessJournal {
	return func(members []Member, removed []string, events []AuditEvent) error {
		stored := make([]investigationdb.Member, len(members))
		for index, member := range members {
			stored[index] = investigationdb.Member{Login: member.Login, Role: string(member.Role),
				GrantedBy: member.GrantedBy, GrantedAt: member.GrantedAt.UTC().Format(time.RFC3339Nano)}
		}
		return db.ReplaceMembers(context.WithoutCancel(ctx), stored, removed, storedAuditEvents(events))
	}
}

// membersOfStored は保存先から読んだ役割を、定義の中の役割と時刻の値であることを確かめて返す。
func membersOfStored(stored []investigationdb.Member) ([]Member, error) {
	members := make([]Member, len(stored))
	for index, member := range stored {
		grantedAt, err := time.Parse(time.RFC3339Nano, member.GrantedAt)
		if err != nil || !Role(member.Role).IsKnown() {
			return nil, fmt.Errorf("the member %q has an unknown role or time", member.Login)
		}
		members[index] = Member{Login: member.Login, Role: Role(member.Role), GrantedBy: member.GrantedBy,
			GrantedAt: grantedAt}
	}
	return members, nil
}

// IsNew は、調査を作成する準備の状態かを返す。
func (i *Investigation) IsNew() bool { return i.db == nil }

// Dir は調査の directory を返す。
func (i *Investigation) Dir() string { return i.dir }

// SourceRoot は、この起動で取り込みの指定の OriginPath を解決する基準の絶対 path を返す。
func (i *Investigation) SourceRoot() string {
	if i.openRoot != "" {
		return i.openRoot
	}
	return i.sourceRoot
}

// RecordedPlans は記録した取り込みの指定を、記録した順の計画にして返す。
//
// 計画は記録した sha256 を ExpectedContentSha256 に持つ。原資料が記録と異なれば、取り込み
// 全体が止まる (Runner.scan)。
func (i *Investigation) RecordedPlans() []SourcePlan {
	plans := make([]SourcePlan, len(i.recorded))
	for index, source := range i.recorded {
		sha := source.ContentSha256
		plans[index] = SourcePlan{
			OriginPath: source.OriginPath, FileName: filepath.Base(source.OriginPath), FormatKey: source.FormatKey,
			FormatSpec: clonePointer(source.FormatSpec), CaseId: clonePointer(source.CaseId),
			ExpectedContentSha256: &sha, Terminal: sourceTerminalOfRecorded(source.Terminal),
			CollectionPath: source.CollectionPath,
		}
	}
	return plans
}

// sourceTerminalOfRecorded は調査に記録した端末を、取り込みの計画の端末へ直す。
func sourceTerminalOfRecorded(terminal *investigationdb.Terminal) *SourceTerminal {
	if terminal == nil {
		return nil
	}
	return &SourceTerminal{
		TerminalId: terminal.Id, TerminalHostname: terminal.Hostname, Ip: terminal.Ip,
		TimeOffset: clonePointer(terminal.TimeOffset),
	}
}

// recordedTerminalOf は取り込みの計画の端末を、調査に記録する端末へ直す。
func recordedTerminalOf(terminal *SourceTerminal) *investigationdb.Terminal {
	if terminal == nil {
		return nil
	}
	return &investigationdb.Terminal{
		Id: terminal.TerminalId, Hostname: terminal.TerminalHostname, Ip: terminal.Ip,
		TimeOffset: clonePointer(terminal.TimeOffset),
	}
}

// CheckAddedPlans は、起動が新しく渡した計画に、既に記録した取得元と入力形式の組が無いことを
// 確かめる。
//
// **同じ起動引数で調査を開き直した起動を退ける。** 受け付けると、同じ収集元が新しい通番で
// もう 1 件取り込まれ、すべての根拠の件数が 2 倍になる。取得元は、この起動の基準で解決した
// file の path で比べる。`./a.log` と `a.log` のような書き方の違いで同じ file を通さない。
func (i *Investigation) CheckAddedPlans(plans []SourcePlan) error {
	for _, plan := range plans {
		for _, source := range i.recorded {
			if i.OriginFile(plan.OriginPath) == i.OriginFile(source.OriginPath) && plan.FormatKey == source.FormatKey {
				return fmt.Errorf("the investigation already records %s:%s; open it without that source argument",
					plan.FormatKey, plan.OriginPath)
			}
		}
	}
	return nil
}

// OriginFile は、取得元 originPath (基準からの相対 path) を、この起動の基準で解決した
// 文字列の上で正規の path を返す。
func (i *Investigation) OriginFile(originPath string) string {
	return filepath.Join(i.SourceRoot(), originPath)
}

// Ordinals は取り込みに渡す通番の発行器を返す。記録した収集元には記録した通番を返し、
// 新しい収集元には、同じ取得元と内容の組で記録した通番の最大の次を返す。
//
// **呼ぶたびに発行を始め直す。** 失敗した取り込みをやり直すとき、前の試行が発行した通番を
// 数えない。Settle は最後に始めた発行器が発行した通番を記録する。
func (i *Investigation) Ordinals() ImportOrdinalSource {
	i.ordinals = newRecordedOrdinals(ordinalsOfSources(i.recorded))
	return i.ordinals
}

// Settle は取り込みの後に、記録に無い収集元を保存先へ足し、保存先へ書く store を返す。
//
// plans は取り込みに渡した計画のすべてであり、先頭が RecordedPlans と同じ並びでなければ
// ならない。作成を準備した調査では、ここで保存先を作る。
func (i *Investigation) Settle(
	ctx context.Context, plans []SourcePlan, result ImportResult, clock AssertionClock,
) (InvestigationStore, error) {
	added, err := i.addedSources(plans, result)
	if err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	if i.db == nil {
		db, err := investigationdb.Create(ctx, i.dir, i.sourceRoot, added)
		if err != nil {
			return nil, fmt.Errorf("settling the investigation: %w", err)
		}
		i.db = db
	} else if len(added) > 0 {
		if err := i.db.AddSources(ctx, added); err != nil {
			return nil, fmt.Errorf("settling the investigation: %w", err)
		}
	}
	i.recorded = append(i.recorded, added...)
	// 取り込まなかった file は、記録したものに今回の file を足して返す。開き直した起動と、収集元を
	// 足さない起動も、前の起動の file を持つ。
	if found := result.SkippedFiles(); len(found) > 0 {
		if err := i.db.AddSkippedFiles(ctx, found); err != nil {
			return nil, fmt.Errorf("settling the investigation: %w", err)
		}
		for _, file := range found {
			at := slices.IndexFunc(i.skipped, func(held core.SkippedFile) bool { return held.OriginPath == file.OriginPath })
			if at < 0 {
				i.skipped = append(i.skipped, file)
			} else {
				i.skipped[at] = file
			}
		}
	}
	result = result.WithSkippedFiles(i.skipped)
	// 作成の前に与えた役割・監査の記録・ワークスペースを、作った調査へ書く。書けなければ、次の Settle で
	// やり直す。
	if err := i.access.attach(accessJournalOf(ctx, i.db)); err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	if err := i.workspaces.attach(workspaceJournalOf(ctx, i.db)); err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	db := i.db
	assertions, err := newJournaledAssertionStore(clock, assertionJournal{
		created: func(assertion core.Assertion, ordinal int64) error {
			return db.InsertAssertion(context.WithoutCancel(ctx), assertion, ordinal)
		},
		revised: func(assertion core.Assertion) error {
			return db.ReviseAssertion(context.WithoutCancel(ctx), assertion)
		},
	}, i.assertions)
	if err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	assignments, err := newJournaledTerminalAssignmentStore(func(assignment core.TerminalAssignment) error {
		return db.InsertAssignment(context.WithoutCancel(ctx), assignment)
	}, i.assignments)
	if err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	assist, err := newJournaledAssistStore(clock, assistJournal{
		permissionRecorded: func(revision core.AssistPermissionRevision) error {
			return db.InsertAssistPermission(context.WithoutCancel(ctx), revision)
		},
		conversationOpened: func(conversation core.AssistConversation) error {
			return db.InsertAssistConversation(context.WithoutCancel(ctx), conversation)
		},
		disclosed: func(disclosure core.AssistDisclosure, turn *core.AssistTurn, refs []core.AssistShortRef) error {
			return db.InsertAssistDisclosure(context.WithoutCancel(ctx), disclosure, turn, refs)
		},
	}, i.assist)
	if err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	proposals, err := newJournaledAssistProposalStore(clock, assertions, assistProposalJournal{
		created: func(proposal core.AssistProposal, ordinal int64) error {
			return db.InsertAssistProposal(context.WithoutCancel(ctx), proposal, ordinal)
		},
		adopted: func(proposal core.AssistProposal, assertion core.Assertion, assertionOrdinal int64) error {
			return db.AdoptAssistProposal(context.WithoutCancel(ctx), proposal, assertion, assertionOrdinal)
		},
		rejected: func(proposal core.AssistProposal) error {
			return db.RejectAssistProposal(context.WithoutCancel(ctx), proposal)
		},
	}, i.proposals)
	if err != nil {
		return nil, fmt.Errorf("settling the investigation: %w", err)
	}
	return newStoreOf(result, assertions, assignments, assist, proposals), nil
}

// addedSources は、記録に無い計画を、取り込みで読んだ sha256 と発行した通番を付けた取り込みの
// 指定にする。
func (i *Investigation) addedSources(plans []SourcePlan, result ImportResult) ([]investigationdb.Source, error) {
	if len(plans) < len(i.recorded) {
		return nil, fmt.Errorf("%d plans for %d recorded sources", len(plans), len(i.recorded))
	}
	for index, source := range i.recorded {
		if plans[index].OriginPath != source.OriginPath || plans[index].FormatKey != source.FormatKey {
			return nil, fmt.Errorf("plan %d is %q, the investigation records %q", index, plans[index].OriginPath,
				source.OriginPath)
		}
	}
	entries, err := result.SourceEntries()
	if err != nil {
		return nil, err
	}
	issued := i.ordinals.issuedOrdinals()
	if len(entries) != len(plans) || len(issued) != len(plans) {
		return nil, fmt.Errorf("%d plans, %d imported sources and %d issued ordinals", len(plans), len(entries),
			len(issued))
	}
	added := make([]investigationdb.Source, 0, len(plans)-len(i.recorded))
	for index := len(i.recorded); index < len(plans); index++ {
		plan, identity := plans[index], entries[index].Identity
		if identity.OriginPath != plan.OriginPath {
			return nil, fmt.Errorf("source %d is %q, planned %q", index, identity.OriginPath, plan.OriginPath)
		}
		added = append(added, investigationdb.Source{
			CaseId: clonePointer(plan.CaseId), FormatKey: plan.FormatKey, OriginPath: plan.OriginPath,
			FormatSpec: clonePointer(plan.FormatSpec), ContentSha256: identity.ContentSha256,
			ImportOrdinal: issued[index], Terminal: recordedTerminalOf(plan.Terminal),
			CollectionPath: plan.CollectionPath,
		})
	}
	return added, nil
}

// Close は保存先を閉じる。作成を準備したまま Settle していない調査では何もしない。
func (i *Investigation) Close() error {
	if i.db == nil {
		return nil
	}
	if err := i.db.Close(); err != nil {
		return fmt.Errorf("closing the investigation: %w", err)
	}
	return nil
}

// recordedOrdinals は、記録した収集元には記録した通番を記録した順に返し、新しい収集元には
// 同じ組の最大の次を返す通番の発行器である。発行した通番を発行した順に保つ。
type recordedOrdinals struct {
	mu      sync.Mutex
	pending map[[2]string][]int64
	highest map[[2]string]int64
	issued  []int64
}

// recordedOrdinal は記録した収集元 1 件の取得元と内容と通番である。
type recordedOrdinal struct {
	originPath, contentSha256 string
	ordinal                   int64
}

func ordinalsOfSources(sources []investigationdb.Source) []recordedOrdinal {
	recorded := make([]recordedOrdinal, len(sources))
	for index, source := range sources {
		recorded[index] = recordedOrdinal{source.OriginPath, source.ContentSha256, source.ImportOrdinal}
	}
	return recorded
}

func newRecordedOrdinals(recorded []recordedOrdinal) *recordedOrdinals {
	ordinals := &recordedOrdinals{pending: map[[2]string][]int64{}, highest: map[[2]string]int64{}}
	for _, source := range recorded {
		key := [2]string{source.originPath, source.contentSha256}
		ordinals.pending[key] = append(ordinals.pending[key], source.ordinal)
		ordinals.highest[key] = max(ordinals.highest[key], source.ordinal)
	}
	return ordinals
}

// Next は originPath と contentSha256 の組に対する次の通番を返す。
func (o *recordedOrdinals) Next(originPath, contentSha256 string) (int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := [2]string{originPath, contentSha256}
	var ordinal int64
	if queue := o.pending[key]; len(queue) > 0 {
		ordinal, o.pending[key] = queue[0], queue[1:]
	} else {
		if o.highest[key] == math.MaxInt64 {
			return 0, errors.New("issue import ordinal: exhausted")
		}
		ordinal = o.highest[key] + 1
		o.highest[key] = ordinal
	}
	o.issued = append(o.issued, ordinal)
	return ordinal, nil
}

func (o *recordedOrdinals) issuedOrdinals() []int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]int64(nil), o.issued...)
}
