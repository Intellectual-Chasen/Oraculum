package auditd_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 生の値と表示用の値を別の項目で持つ。
func TestRecordKeepsTheRawAndTheInterpretedValueApart(t *testing.T) {
	record := readOneRecord(t, executedEventSource)
	line, found := record.LineOfType(auditd.LineTypeSyscall)
	if !found {
		t.Fatal("the record carries no SYSCALL line")
	}

	raw, found := line.Item("uid", false)
	if !found {
		t.Fatal("the SYSCALL line carries no raw uid")
	}
	if raw.Value() != "0" {
		t.Errorf("the raw uid is %q, want %q", raw.Value(), "0")
	}
	interpreted, found := line.Item("UID", true)
	if !found {
		t.Fatal("the SYSCALL line carries no interpreted UID")
	}
	if interpreted.Value() != "root" {
		t.Errorf("the interpreted UID is %q, want %q", interpreted.Value(), "root")
	}
	if !interpreted.Interpreted() || raw.Interpreted() {
		t.Errorf("the raw uid reads as interpreted=%t and UID as interpreted=%t, want false then true",
			raw.Interpreted(), interpreted.Interpreted())
	}

	// 応答の項目としても別々に出る。
	fields := requireFields(t, record)
	rawField := requireField(t, fields, "SYSCALL.uid")
	interpretedField := requireField(t, fields, "SYSCALL.UID")
	if rawField.Name == interpretedField.Name {
		t.Fatal("the raw value and the interpreted value share one field name")
	}
	if rawField.Semantic != "" {
		t.Errorf("SYSCALL.uid carries the semantic %q, want none", rawField.Semantic)
	}
}

// auid と uid を 1 つの実行主体へまとめない。
func TestRecordKeepsTheLoginUserApartFromTheRunningUser(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, executedEventSource))
	loginUser := requireField(t, fields, "SYSCALL.auid")
	runningUser := requireField(t, fields, "SYSCALL.uid")
	if rawTextOf(t, loginUser) == rawTextOf(t, runningUser) {
		t.Fatalf("the fixture no longer separates auid from uid: both are %q",
			rawTextOf(t, loginUser))
	}
	// 他の収集元の利用者名と比べられるのは、表示用の値だけである。
	accountName := requireField(t, fields, "SYSCALL.AUID")
	if accountName.Semantic != core.SemanticKeyAccountName {
		t.Errorf("SYSCALL.AUID carries the semantic %q, want %q",
			accountName.Semantic, core.SemanticKeyAccountName)
	}
	if loginUser.Semantic != "" {
		t.Errorf("SYSCALL.auid carries the semantic %q, want none", loginUser.Semantic)
	}
}

// 16 進表記の引数を復号した正規化値を、原資料の文字列と分けて持つ。
func TestCommandLineDecodesTheHexArguments(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, executedEventSource))

	argument := requireField(t, fields, "EXECVE.a1")
	if rawTextOf(t, argument) != "2D7820" {
		t.Errorf("the raw argument is %q, want %q", rawTextOf(t, argument), "2D7820")
	}

	commandLine := requireField(t, fields, "commandLine")
	if normalizedOf(t, commandLine) != "tool -x  --out" {
		t.Errorf("the command line is %q, want %q", normalizedOf(t, commandLine), "tool -x  --out")
	}
	if commandLine.Semantic != core.SemanticKeyProcessCommandLine {
		t.Errorf("the command line carries the semantic %q, want %q",
			commandLine.Semantic, core.SemanticKeyProcessCommandLine)
	}
}

// 分割された 1 つの引数を、断片を連結した 1 つの値として読む。
//
// auditd は長い引数を `a1_len` と `a1[0]` `a1[1]` の組へ分ける。断片を別の引数として
// 並べると、コマンド行の引数の数が原資料と食い違う。
func TestCommandLineJoinsASplitArgument(t *testing.T) {
	const splitSource = "type=SYSCALL msg=audit(1000000000.300:20003): arch=c000003e syscall=59 " +
		"success=yes exit=0 ppid=1001 pid=1004 auid=4001 uid=0 comm=\"tool\" exe=\"/usr/bin/tool\"\n" +
		"type=EXECVE msg=audit(1000000000.300:20003): argc=3 a0=\"tool\" a1_len=6 " +
		"a1[0]=2D64 a1[1]=3230 a2=\"--out\"\n"
	fields := requireFields(t, readOneRecord(t, splitSource))

	// 2D64 は "-d"、3230 は "20" である。断片を連結した 1 つの引数は "-d20" になる。
	commandLine := requireField(t, fields, "commandLine")
	if normalizedOf(t, commandLine) != "tool -d20 --out" {
		t.Errorf("the command line is %q, want %q",
			normalizedOf(t, commandLine), "tool -d20 --out")
	}
}

// argc が名乗った数の引数を読めない事象は、コマンド行を欠測として表す。
// 引数を欠いたコマンド行を、完全な値として画面へ出さない。
func TestCommandLineOfAnEventMissingAnArgument(t *testing.T) {
	const missingSource = "type=SYSCALL msg=audit(1000000000.400:20004): arch=c000003e syscall=59 " +
		"success=yes exit=0 ppid=1001 pid=1005 auid=4001 uid=0 comm=\"tool\" exe=\"/usr/bin/tool\"\n" +
		"type=EXECVE msg=audit(1000000000.400:20004): argc=3 a0=\"tool\" a1=\"--out\"\n"
	fields := requireFields(t, readOneRecord(t, missingSource))

	commandLine := requireField(t, fields, "commandLine")
	if commandLine.Text.ValueState != core.ValueStateItemAbsent {
		t.Errorf("the command line value state is %q, want %q",
			commandLine.Text.ValueState, core.ValueStateItemAbsent)
	}
}

// proctitle の 16 進を復号し、0x00 を空白に置き換えた正規化値を持つ。
func TestProctitleDecodesTheNulSeparatedArguments(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, executedEventSource))
	proctitle := requireField(t, fields, "proctitleCommandLine")
	// 区切りの 0x00 が空白になる。引数 "-x " 自体が末尾に空白を持つため、
	// 区切りの空白と並んで空白が 2 つになる。
	if normalizedOf(t, proctitle) != "tool -x  --out" {
		t.Errorf("the proctitle command line is %q, want %q",
			normalizedOf(t, proctitle), "tool -x  --out")
	}
	if strings.ContainsRune(normalizedOf(t, proctitle), 0x00) {
		t.Error("the proctitle command line carries a 0x00 byte, want it replaced with a space")
	}
	if rawTextOf(t, proctitle) != "746F6F6C002D7820002D2D6F7574" {
		t.Errorf("the raw proctitle is %q, want the hex text of the source",
			rawTextOf(t, proctitle))
	}
}

// type=EXECVE を持たない事象は、コマンド行を欠測として表す。
func TestCommandLineOfAnEventWithoutExecve(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, failedEventSource))
	commandLine := requireField(t, fields, "commandLine")
	if commandLine.Text.ValueState != core.ValueStateItemAbsent {
		t.Errorf("the command line value state is %q, want %q",
			commandLine.Text.ValueState, core.ValueStateItemAbsent)
	}
	// 引数は proctitle の復号値からだけ読める。
	proctitle := requireField(t, fields, "proctitleCommandLine")
	if normalizedOf(t, proctitle) != "tool --missing" {
		t.Errorf("the proctitle command line is %q, want %q",
			normalizedOf(t, proctitle), "tool --missing")
	}
}

// type=EXECVE を持つ事象と success=no の事象は重ならない。
func TestFailedEventsCarryNoExecveLine(t *testing.T) {
	source := executedEventSource + failedEventSource
	records, _ := readRecords(t, source)
	executed := make(map[string]struct{})
	failed := make(map[string]struct{})
	for _, record := range records {
		if _, found := record.LineOfType(auditd.LineTypeExecve); found {
			executed[record.Event().RawText] = struct{}{}
		}
		if syscall, found := record.LineOfType(auditd.LineTypeSyscall); found {
			if success, ok := syscall.Item("success", false); ok && success.Value() == "no" {
				failed[record.Event().RawText] = struct{}{}
			}
		}
	}
	if len(executed) == 0 || len(failed) == 0 {
		t.Fatalf("the fixture holds %d executed and %d failed events, want both",
			len(executed), len(failed))
	}
	for event := range failed {
		if _, both := executed[event]; both {
			t.Errorf("the event %q carries both an EXECVE line and success=no", event)
		}
	}
}

// 欠測を表す文字列を 0 や空値へまとめない。
func TestAbsentValuesKeepTheirRawText(t *testing.T) {
	cases := map[string]struct {
		source string
		field  string
		want   string
	}{
		"DAEMON_END の -1": {daemonEventSource, "DAEMON_END.auid", "-1"},
		"表示用の unset":      {daemonEventSource, "DAEMON_END.AUID", `"unset"`},
		"記録の無い host":      {sessionEventSource, "USER_START.msg#2.hostname", "?"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			fields := requireFields(t, readOneRecord(t, want.source))
			field := requireField(t, fields, want.field)
			if field.Text.ValueState != core.ValueStateAbsent {
				t.Errorf("the value state is %q, want %q",
					field.Text.ValueState, core.ValueStateAbsent)
			}
			if rawTextOf(t, field) != want.want {
				t.Errorf("the raw text is %q, want %q", rawTextOf(t, field), want.want)
			}
		})
	}
}

// ses の 4294967295 は未設定であり、値 0 と別の状態になる。
func TestUnsetSessionIsAbsent(t *testing.T) {
	const source = "type=LOGIN msg=audit(1000000000.500:20004): pid=1005 uid=0 auid=4294967295 ses=4294967295 res=1\n"
	fields := requireFields(t, readOneRecord(t, source))
	for _, name := range []string{"LOGIN.auid", "LOGIN.ses"} {
		field := requireField(t, fields, name)
		if field.Text.ValueState != core.ValueStateAbsent {
			t.Errorf("%s is %q, want %q", name, field.Text.ValueState, core.ValueStateAbsent)
		}
		if rawTextOf(t, field) != "4294967295" {
			t.Errorf("%s keeps the raw text %q, want %q", name, rawTextOf(t, field), "4294967295")
		}
	}
	// 同じ行の pid は値である。
	pid := requireField(t, fields, "LOGIN.pid")
	if pid.Text.ValueState != core.ValueStatePresent {
		t.Errorf("LOGIN.pid is %q, want %q", pid.Text.ValueState, core.ValueStatePresent)
	}
}

// ses は SYSCALL では操作を行ったセッション、成功した USER_START では開いたセッション、成功した
// USER_END では閉じたセッションの番号である。失敗した行と内側に res を持たない行の ses は語彙の
// 項目を持たない。
func TestSessionNumberCarriesTheRoleOfTheLine(t *testing.T) {
	const end = "type=USER_END msg=audit(1000000000.600:20005): pid=1004 uid=0 auid=0 ses=43 " +
		"msg='op=PAM:session_close acct=\"root\" exe=\"/usr/sbin/cron\" hostname=? addr=? terminal=cron res=success'\n"
	const failed = "type=USER_START msg=audit(1000000000.700:20006): pid=1006 uid=0 auid=0 ses=44 " +
		"msg='op=PAM:session_open acct=\"root\" exe=\"/usr/sbin/cron\" hostname=? addr=? terminal=cron res=failed'\n"
	const bare = "type=USER_END msg=audit(1000000000.800:20007): pid=1006 uid=0 auid=0 ses=44\n"
	for _, tc := range []struct {
		source, name string
		want         core.SemanticKey
	}{
		{executedEventSource, "SYSCALL.ses", core.SemanticKeyEventSubjectLogonId},
		{sessionEventSource, "USER_START.ses", core.SemanticKeyEventTargetLogonId},
		{end, "USER_END.ses", core.SemanticKeyEventLogoffLogonId},
		{failed, "USER_START.ses", ""},
		{bare, "USER_END.ses", ""},
	} {
		field := requireField(t, requireFields(t, readOneRecord(t, tc.source)), tc.name)
		if field.Semantic != tc.want {
			t.Errorf("%s of %q carries %q, want %q", tc.name, tc.source, field.Semantic, tc.want)
		}
	}
	declared := auditd.ItemSemantics()
	for _, want := range []core.SemanticKey{core.SemanticKeyEventSubjectLogonId,
		core.SemanticKeyEventTargetLogonId, core.SemanticKeyEventLogoffLogonId} {
		if !slices.Contains(declared, want) {
			t.Errorf("ItemSemantics does not declare %q", want)
		}
	}
}

// msg='...' の内側を、外側と同じ並びとして読まない。
func TestInnerMessageIsReadAsItsOwnPairs(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, sessionEventSource))
	account := requireField(t, fields, "USER_START.msg#2.acct")
	if normalizedOf(t, account) != "root" {
		t.Errorf("the inner acct is %q, want %q", normalizedOf(t, account), "root")
	}
	// 事象を指す msg と、もう 1 階層の並びを持つ msg を別の項目として持つ。
	eventKey := requireField(t, fields, "USER_START.msg")
	if !strings.HasPrefix(rawTextOf(t, eventKey), "audit(") {
		t.Errorf("the first msg is %q, want the event key", rawTextOf(t, eventKey))
	}
	outer := requireField(t, fields, "USER_START.msg#2")
	if !strings.Contains(rawTextOf(t, outer), "op=PAM:session_open") {
		t.Errorf("the outer msg raw text is %q, want the whole inner text", rawTextOf(t, outer))
	}
}

// 同じ key を持つ 2 つの PATH の行を、別の項目として持つ。
func TestPathLinesKeepTheirOwnFields(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, executedEventSource))
	first := requireField(t, fields, "PATH[0].name")
	second := requireField(t, fields, "PATH[1].name")
	if normalizedOf(t, first) != "/usr/bin/tool" {
		t.Errorf("the first path is %q, want %q", normalizedOf(t, first), "/usr/bin/tool")
	}
	if normalizedOf(t, second) != "/lib/loader.so" {
		t.Errorf("the second path is %q, want %q", normalizedOf(t, second), "/lib/loader.so")
	}
	// 語彙の項目を持つのは、事象が記録した file だけである。
	if first.Semantic != core.SemanticKeyFilePath {
		t.Errorf("the first path carries the semantic %q, want %q",
			first.Semantic, core.SemanticKeyFilePath)
	}
	if second.Semantic != "" {
		t.Errorf("the second path carries the semantic %q, want none", second.Semantic)
	}
}

// 語彙の項目は 1 事象に 1 度だけ出る。
func TestRecordFieldsCarryEachSemanticOnce(t *testing.T) {
	fields := requireFields(t, readOneRecord(t, executedEventSource))
	seen := make(map[core.SemanticKey]string, len(fields))
	for _, field := range fields {
		if field.Semantic == "" {
			continue
		}
		if previous, duplicate := seen[field.Semantic]; duplicate {
			t.Errorf("the semantic %q is carried by both %q and %q",
				field.Semantic, previous, field.Name)
		}
		seen[field.Semantic] = field.Name
	}
}

// 項目の名前は 1 事象の中で重複しない。
func TestRecordFieldsCarryDistinctNames(t *testing.T) {
	sources := map[string]string{
		"実行した事象":          executedEventSource,
		"実行に失敗した事象":       failedEventSource,
		"内側にもう 1 階層を持つ事象": sessionEventSource,
		"欠測を -1 で表す事象":    daemonEventSource,
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			fields := requireFields(t, readOneRecord(t, source))
			seen := make(map[string]struct{}, len(fields))
			for _, field := range fields {
				if _, duplicate := seen[field.Name]; duplicate {
					t.Errorf("the field name %q occurs twice", field.Name)
				}
				seen[field.Name] = struct{}{}
			}
		})
	}
}

func requireFields(t *testing.T, record auditd.Record) []core.RecordField {
	t.Helper()
	fields, err := auditd.RecordFields(record)
	if err != nil {
		t.Fatalf("building the fields: %v", err)
	}
	return fields
}

func requireField(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	t.Fatalf("the record carries no field %q. it carries %v", name, names)
	return core.RecordField{}
}

func rawTextOf(t *testing.T, field core.RecordField) string {
	t.Helper()
	if field.Text == nil || field.Text.RawText == nil {
		t.Fatalf("the field %q carries no raw text", field.Name)
	}
	return *field.Text.RawText
}

func normalizedOf(t *testing.T, field core.RecordField) string {
	t.Helper()
	if field.Text == nil || field.Text.Normalized == nil {
		t.Fatalf("the field %q carries no normalized value", field.Name)
	}
	return *field.Text.Normalized
}
