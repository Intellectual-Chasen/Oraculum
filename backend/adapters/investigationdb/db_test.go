package investigationdb_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func stringPointer(value string) *string { return &value }
func intPointer(value int64) *int64      { return &value }

func sampleSources() []investigationdb.Source {
	return []investigationdb.Source{
		{CaseId: stringPointer("baseline"), FormatKey: "infotrace_mark_ii", OriginPath: "logs/pc01.log",
			ContentSha256: shaA, ImportOrdinal: 1},
		{CaseId: stringPointer("challenge"), FormatKey: "squid_logformat", OriginPath: "logs/proxy/access.log",
			FormatSpec: stringPointer(`%>a %un [%tl] "%rm %ru"`), ContentSha256: shaB, ImportOrdinal: 1},
		// 同じ取得元と内容をもう一度取り込んだ収集元は、通番で区別する。空の欄の並びは空文字列のまま保つ。
		{FormatKey: "infotrace_mark_ii", OriginPath: "logs/pc01.log", FormatSpec: stringPointer(""),
			ContentSha256: shaA, ImportOrdinal: 2},
	}
}

func recordedAt(minute int) core.AssertionTime {
	return core.NewAssertionTime(time.Date(2030, 1, 2, 3, minute, 0, 0, time.UTC))
}

func sampleAssertions() []investigationdb.StoredAssertion {
	byLine := core.AssertionRecordRef{SourceContentSha256: shaA, PositionKind: core.PositionKindLineNumber,
		LineNumber: intPointer(12), ByteOffset: intPointer(340)}
	bySequence := core.AssertionRecordRef{SourceContentSha256: shaB, PositionKind: core.PositionKindSequenceNumber,
		SequenceNumber: intPointer(7)}
	return []investigationdb.StoredAssertion{
		{Ordinal: 1, Assertion: core.Assertion{
			Id: "as:node", Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:terminal:1"},
			State: core.AssertionStateWithdrawn, Author: "analyst-b", RecordedAt: recordedAt(9),
			Basis:          core.AssertionBasis{Note: "改訂後の所見", RecordRefs: []core.AssertionRecordRef{bySequence}},
			RevisionNumber: 2,
			History: []core.AssertionRevision{{RevisionNumber: 1, State: core.AssertionStateActive,
				Author: "analyst-a", RecordedAt: recordedAt(4),
				Basis: core.AssertionBasis{Note: "最初の所見", RecordRefs: []core.AssertionRecordRef{byLine, bySequence}}}},
		}},
		{Ordinal: 2, Assertion: core.Assertion{
			Id: "as:edge", Target: core.AssertionTarget{Kind: core.AssertionTargetKindEdge,
				Edge: &core.AssertionEdgeRef{Kind: core.EdgeKindProcessParentChild, SourceNodeId: "n:process:1",
					TargetNodeId: "n:process:2"}},
			State: core.AssertionStateActive, Author: "analyst-a", RecordedAt: recordedAt(5),
			Basis: core.AssertionBasis{Note: "根拠の無い所見", RecordRefs: []core.AssertionRecordRef{}}, RevisionNumber: 1,
			History: []core.AssertionRevision{}, AddsRelation: true,
		}},
		{Ordinal: 3, Assertion: core.Assertion{
			Id: "as:record", Target: core.AssertionTarget{Kind: core.AssertionTargetKindRecord, Record: &byLine},
			State: core.AssertionStateActive, Author: "analyst-a", RecordedAt: recordedAt(6),
			Basis:          core.AssertionBasis{Note: "レコードの所見", RecordRefs: []core.AssertionRecordRef{byLine}},
			RevisionNumber: 1, History: []core.AssertionRevision{},
		}},
		{Ordinal: 4, Assertion: core.Assertion{
			Id: "as:source", Target: core.AssertionTarget{Kind: core.AssertionTargetKindSource, SourceContentSha256: shaA},
			State: core.AssertionStateWithdrawn, Author: "analyst-b", RecordedAt: recordedAt(8),
			Basis:      core.AssertionBasis{Note: "解釈を取り消した", RecordRefs: []core.AssertionRecordRef{}},
			TimeOffset: offsetPointer("+09:00"), RevisionNumber: 2,
			History: []core.AssertionRevision{{RevisionNumber: 1, State: core.AssertionStateActive,
				Author: "analyst-a", RecordedAt: recordedAt(7), TimeOffset: offsetPointer("+00:00"),
				Basis: core.AssertionBasis{Note: "両側のレコードを比べた", RecordRefs: []core.AssertionRecordRef{byLine, bySequence}}}},
		}},
	}
}

func offsetPointer(value core.UtcOffset) *core.UtcOffset { return &value }

func timestamp(t *testing.T, normalized string) core.Timestamp {
	t.Helper()
	value, err := core.NewTimestamp(core.Timestamp{
		RawText: stringPointer(normalized), Normalized: stringPointer(normalized),
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionSecond,
		OffsetState: core.OffsetStateInValue, OffsetText: stringPointer("Z"), Clock: core.ClockTerminalLocal,
		Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func sampleAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	return core.TerminalAssignment{
		ClientIp: "192.0.2.10", TerminalId: "terminal-1", TerminalHostname: "pc01.example.test",
		TerminalHostnames: []string{"pc01", "pc01.example.test"},
		SourceId:          "src:1", SourceContentSha256: shaA,
		AssignmentValidRange: core.TimeRange{From: timestamp(t, "2030-01-02T03:00:00Z"),
			To: timestamp(t, "2030-01-02T04:00:00Z")},
		Origin: core.TerminalAssignmentOriginAnalystSupplied, Derivation: "DHCP の記録から導いた",
		BasisRecordRefs: []core.AssertionRecordRef{{SourceContentSha256: shaB,
			PositionKind: core.PositionKindByteRange, ByteOffset: intPointer(0)}},
		Author: "analyst-a", AppliesToSourceId: "src:2",
	}
}

// jsonOf は値の比較に使う。Timestamp は非公開の時刻を持つため、JSON の文字列で比べる。
func jsonOf(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func create(t *testing.T, dir string) *investigationdb.DB {
	t.Helper()
	db, err := investigationdb.Create(context.Background(), dir, "/cases/root", sampleSources())
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// 作成した調査へ書いた値を、閉じて開き直した後に同じ値で読み戻す。
func TestReopenedInvestigationReadsBackWhatWasWritten(t *testing.T) {
	ctx := context.Background()
	// 空白・?・#・非 ASCII を含む directory 名を URI の query と取り違えない。
	dir := filepath.Join(t.TempDir(), "case #1 ? 調査")
	db := create(t, dir)
	stored := sampleAssertions()
	for _, item := range stored {
		if err := db.InsertAssertion(ctx, item.Assertion, item.Ordinal); err != nil {
			t.Fatal(err)
		}
	}
	revised := stored[1].Assertion
	revised.History = []core.AssertionRevision{{RevisionNumber: 1, State: revised.State, Author: revised.Author,
		RecordedAt: revised.RecordedAt, Basis: revised.Basis}}
	revised.RevisionNumber, revised.State, revised.RecordedAt = 2, core.AssertionStateWithdrawn, recordedAt(20)
	revised.Basis = core.AssertionBasis{Note: "取り下げた", RecordRefs: []core.AssertionRecordRef{}}
	if err := db.ReviseAssertion(ctx, revised); err != nil {
		t.Fatal(err)
	}
	stored[1].Assertion = revised
	assignment := sampleAssignment(t)
	if err := db.InsertAssignment(ctx, assignment); err != nil {
		t.Fatal(err)
	}
	added := investigationdb.Source{FormatKey: "linux_auditd", OriginPath: "logs/audit.log", ContentSha256: shaB,
		ImportOrdinal: 1, Terminal: &investigationdb.Terminal{TimeOffset: offsetPointer("+09:00")},
		CollectionPath: "logs"}
	if err := db.AddSources(ctx, []investigationdb.Source{added}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, want := jsonOf(t, contents.Sources), jsonOf(t, append(sampleSources(), added)); got != want {
		t.Errorf("the sources read back as\n%s\nwant\n%s", got, want)
	}
	if got, want := jsonOf(t, contents.Assertions), jsonOf(t, stored); got != want {
		t.Errorf("the assertions read back as\n%s\nwant\n%s", got, want)
	}
	if got, want := jsonOf(t, contents.Assignments), jsonOf(t, []core.TerminalAssignment{assignment}); got != want {
		t.Errorf("the assignments read back as\n%s\nwant\n%s", got, want)
	}
	for index, read := range contents.Assignments {
		if _, comparable := read.AssignmentValidRange.From.Instant(); !comparable {
			t.Errorf("the assignment %d has a valid range without a comparable instant", index)
		}
	}
	if contents.Meta.SourceRoot != "/cases/root" || !strings.HasPrefix(contents.Meta.InvestigationId, "inv:") ||
		contents.Meta.CreatedAt == "" {
		t.Errorf("the meta reads back as %+v", contents.Meta)
	}
}

// 地方時の期間を持つ割当は、UTC からのずれを持たない文字列のまま読み戻す。同じ調査の UTC の
// 期間の割当は、時点を持つ期間のまま読み戻す。
func TestAssignmentWithALocalRangeReadsBackWithoutAnOffset(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	local := func(normalized string) core.Timestamp {
		t.Helper()
		value, err := core.NewTimestamp(core.Timestamp{
			RawText: stringPointer(normalized), Normalized: stringPointer(normalized),
			NormalizedForm: core.NormalizedFormLocalWithoutOffset, Precision: core.PrecisionSecond,
			OffsetState: core.OffsetStateUndetermined, Clock: core.ClockTerminalLocal,
			Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	utc := sampleAssignment(t)
	localAssignment := sampleAssignment(t)
	localAssignment.ClientIp = "192.0.2.11"
	localAssignment.AssignmentValidRange = core.TimeRange{
		From: local("2030-01-02T03:00:00"), To: local("2030-01-02T04:00:00"),
	}
	for _, assignment := range []core.TerminalAssignment{utc, localAssignment} {
		if err := assignment.Validate(); err != nil {
			t.Fatalf("the assignment %+v: %v", assignment, err)
		}
		if err := db.InsertAssignment(ctx, assignment); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, want := jsonOf(t, contents.Assignments),
		jsonOf(t, []core.TerminalAssignment{utc, localAssignment}); got != want {
		t.Fatalf("the assignments read back as\n%s\nwant\n%s", got, want)
	}
	if _, comparable := contents.Assignments[0].AssignmentValidRange.From.Instant(); !comparable {
		t.Error("the UTC assignment lost its instant")
	}
	read := contents.Assignments[1].AssignmentValidRange.From
	if _, comparable := read.Instant(); comparable || !read.AcceptsInterpretation() {
		t.Errorf("the local assignment reads back as %+v, want a local time open to an interpretation", read)
	}
}

// 既に調査の file がある directory への作成を退け、既存の file を変えない。
func TestCreateRefusesADirectoryHoldingAnInvestigation(t *testing.T) {
	dir := t.TempDir()
	if err := create(t, dir).Close(); err != nil {
		t.Fatal(err)
	}
	before := digestOf(t, filepath.Join(dir, investigationdb.FileName))
	_, err := investigationdb.Create(context.Background(), dir, "/cases/root", sampleSources())
	if !errors.Is(err, investigationdb.ErrExists) {
		t.Fatalf("the second create returned %v, want ErrExists", err)
	}
	if after := digestOf(t, filepath.Join(dir, investigationdb.FileName)); after != before {
		t.Error("the refused create changed the existing file")
	}
}

// 作成の途中で記録が失敗したら、作りかけの file を残さない。
func TestFailedCreateLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	duplicated := append(sampleSources(), sampleSources()[0])
	if _, err := investigationdb.Create(context.Background(), dir, "/cases/root", duplicated); err == nil {
		t.Fatal("the create with a duplicated source succeeded")
	}
	exists, err := investigationdb.Exists(dir)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("the failed create left the investigation file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("the failed create left %s", entry.Name())
	}
}

// 作成は取り込みの指定を 1 件以上と、絶対 path の基準 directory を求める。
func TestCreateRequiresSourcesAndAnAbsoluteRoot(t *testing.T) {
	ctx := context.Background()
	if _, err := investigationdb.Create(ctx, t.TempDir(), "/cases/root", nil); err == nil {
		t.Error("the create without sources succeeded")
	}
	if _, err := investigationdb.Create(ctx, t.TempDir(), "cases/root", sampleSources()); err == nil {
		t.Error("the create with a relative source root succeeded")
	}
}

// 作成した調査も、開き直しただけで何も書いていない調査も、別の接続は開けない。閉じた後は開ける。
func TestOpenInvestigationIsExclusive(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	if _, _, err := investigationdb.Open(ctx, dir); !errors.Is(err, investigationdb.ErrLocked) {
		t.Fatalf("opening a created investigation returned %v, want ErrLocked", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatalf("opening the released investigation failed: %v", err)
	}
	if _, _, err := investigationdb.Open(ctx, dir); !errors.Is(err, investigationdb.ErrLocked) {
		t.Fatalf("opening a reopened investigation returned %v, want ErrLocked", err)
	}
	if err := reopened.AddSources(ctx, []investigationdb.Source{{FormatKey: "squid_access_log",
		OriginPath: "logs/other.log", ContentSha256: shaB, ImportOrdinal: 2}}); err != nil {
		t.Fatalf("the holder of the reopened investigation cannot write: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	again, _, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatalf("opening the released investigation failed: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
}

// 調査でない SQLite file を開かず、変更も削除もしない。
func TestOpenRefusesAFileThatIsNotAnInvestigation(t *testing.T) {
	t.Run("not an investigation", func(t *testing.T) {
		dir := t.TempDir()
		if err := create(t, dir).Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, investigationdb.FileName)
		execRaw(t, path, `PRAGMA application_id = 0`)
		expectRefusedUnchanged(t, dir, path)
	})
	// 調査でない、rollback journal の SQLite file を WAL に書き換えない。
	t.Run("a rollback journal file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, investigationdb.FileName)
		execRaw(t, path, `CREATE TABLE other (value TEXT)`)
		expectRefusedUnchanged(t, dir, path)
		if _, err := os.Stat(path + "-wal"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the refused open left a WAL file: %v", err)
		}
	})
	// 作成の途中で止まった起動が残した file は、作成の途中であることを伝えて退ける。止まる時点は
	// file を作った直後 (空の file) と、WAL へ切り替えた後で表を作る前 (表の無い file) である。
	stopped := map[string]func(t *testing.T, path string){
		"an empty file": func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"a file switched to WAL without tables": func(t *testing.T, path string) {
			execRaw(t, path, `PRAGMA journal_mode = WAL`)
		},
	}
	for name, leave := range stopped {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, investigationdb.FileName)
			leave(t, path)
			if err := expectRefusedUnchanged(t, dir, path); !strings.Contains(err.Error(), "creation stopped") {
				t.Errorf("the refusal %q does not tell the creation stopped", err)
			}
		})
	}
}

// expectRefusedUnchanged は dir の調査を開くと ErrIncompatible で退けられ、path が変わらないことを確かめる。
func expectRefusedUnchanged(t *testing.T, dir, path string) error {
	t.Helper()
	before := digestOf(t, path)
	_, _, err := investigationdb.Open(context.Background(), dir)
	if !errors.Is(err, investigationdb.ErrIncompatible) {
		t.Fatalf("opening the file returned %v, want ErrIncompatible", err)
	}
	if after := digestOf(t, path); after != before {
		t.Error("the refused open changed the file")
	}
	return err
}

// Validate を通らない行を持つ file は開かない。
func TestOpenRefusesAnInvalidAssertion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	if err := db.InsertAssertion(ctx, sampleAssertions()[2].Assertion, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	execRaw(t, filepath.Join(dir, investigationdb.FileName), `UPDATE assertion_revision SET author = ''`)
	if _, _, err := investigationdb.Open(ctx, dir); err == nil {
		t.Fatal("opening a file with an assertion without its author succeeded")
	}
}

// Exists は調査の file の有無を返し、file でないものを調査と読まない。
func TestExistsReportsTheInvestigationFile(t *testing.T) {
	dir := t.TempDir()
	if exists, err := investigationdb.Exists(dir); err != nil || exists {
		t.Errorf("an empty directory reads as exists=%v err=%v", exists, err)
	}
	if err := os.Mkdir(filepath.Join(dir, investigationdb.FileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := investigationdb.Exists(dir); err == nil {
		t.Error("a directory in place of the file reads as an investigation")
	}
	created := t.TempDir()
	if err := create(t, created).Close(); err != nil {
		t.Fatal(err)
	}
	if exists, err := investigationdb.Exists(created); err != nil || !exists {
		t.Errorf("a created investigation reads as exists=%v err=%v", exists, err)
	}
}

// 本 package が読まない表と行を持つ file を開ける。開いて閉じた後も、表と行は残る。
func TestOpenSkipsTablesThePackageNoLongerReads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := create(t, dir).Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, investigationdb.FileName)
	for _, statement := range []string{
		`CREATE TABLE grouping (id INTEGER PRIMARY KEY CHECK (id = 1), revision INTEGER NOT NULL,
			level_count INTEGER NOT NULL CHECK (level_count >= 0))`,
		`CREATE TABLE grouping_condition (element INTEGER PRIMARY KEY, condition_key TEXT NOT NULL,
			tolerance INTEGER NOT NULL)`,
		`CREATE TABLE grouping_member (position INTEGER PRIMARY KEY, node_id TEXT NOT NULL,
			groups BLOB NOT NULL)`,
		`INSERT INTO grouping VALUES (1, 3, 2)`,
		`INSERT INTO grouping_condition VALUES (0, 'destination_ip', 0)`,
		`INSERT INTO grouping_member VALUES (0, 'n:a', X'0080')`,
	} {
		execRaw(t, path, statement)
	}
	reopened, _, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatalf("opening a file with tables the package no longer reads failed: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	var members int
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT count(*) FROM grouping_member`).Scan(&members); err != nil || members != 1 {
		t.Errorf("the table reads %d rows (err %v) after opening, want the row kept", members, err)
	}
}

func execRaw(t *testing.T, path, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
