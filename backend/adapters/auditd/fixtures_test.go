package auditd_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
)

// テスト用の収集元。IP は RFC 5737 の文書用範囲、host は example.test 系である。
//
// auditd の出力の形を持つ。0x1D の前後、16 進表記の引数、複数の PATH の行、
// msg='...' の内側のもう 1 階層、欠測の文字列を含む。
const (
	// separator は生の値と表示用の値を分ける 0x1D である。
	separator = "\x1d"

	// executedEventSource は execve が成功した 1 事象である。6 行を持つ。
	executedEventSource = "type=SYSCALL msg=audit(1000000000.100:20001): arch=c000003e syscall=59 success=yes exit=0 items=2 ppid=1001 pid=1002 auid=4001 uid=0 tty=pts9 ses=42 comm=\"tool\" exe=\"/usr/bin/tool\" key=\"exec_log\"" +
		separator + "ARCH=x86_64 SYSCALL=execve AUID=\"analyst@example.test\" UID=\"root\"\n" +
		"type=EXECVE msg=audit(1000000000.100:20001): argc=3 a0=\"tool\" a1=2D7820 a2=\"--out\"\n" +
		"type=CWD msg=audit(1000000000.100:20001): cwd=\"/tmp\"\n" +
		"type=PATH msg=audit(1000000000.100:20001): item=0 name=\"/usr/bin/tool\" inode=101 mode=0100755 nametype=NORMAL" +
		separator + "OUID=\"root\"\n" +
		"type=PATH msg=audit(1000000000.100:20001): item=1 name=\"/lib/loader.so\" inode=102 mode=0100755 nametype=NORMAL" +
		separator + "OUID=\"root\"\n" +
		"type=PROCTITLE msg=audit(1000000000.100:20001): proctitle=746F6F6C002D7820002D2D6F7574\n"

	// failedEventSource は execve が失敗した 1 事象である。EXECVE の行を持たない。
	failedEventSource = "type=SYSCALL msg=audit(1000000000.200:20002): arch=c000003e syscall=59 success=no exit=-2 items=1 ppid=1002 pid=1003 auid=4001 uid=0 comm=\"tool\" exe=\"/usr/bin/tool\" key=\"exec_log\"" +
		separator + "ARCH=x86_64 SYSCALL=execve AUID=\"analyst@example.test\" UID=\"root\"\n" +
		"type=PROCTITLE msg=audit(1000000000.200:20002): proctitle=746F6F6C002D2D6D697373696E67\n"

	// sessionEventSource は msg='...' の内側にもう 1 階層の key=value を持つ 1 事象である。
	sessionEventSource = "type=USER_START msg=audit(1000000000.300:20003): pid=1004 uid=0 auid=0 ses=43 msg='op=PAM:session_open acct=\"root\" exe=\"/usr/sbin/cron\" hostname=? addr=? terminal=cron res=success'" +
		separator + "UID=\"root\" AUID=\"root\"\n"

	// daemonEventSource は欠測を -1 で表す 1 事象である。
	daemonEventSource = "type=DAEMON_END msg=audit(1000000900.400:12): op=terminate auid=-1 uid=-1 ses=-1 pid=-1 res=success" +
		separator + "AUID=\"unset\" UID=\"unset\"\n"

	// truncatedHeadSource は先頭が事象の途中で切れている収集元である。
	// 最初の事象が SYSCALL の行を持たないまま CWD の行から始まる。
	truncatedHeadSource = "type=CWD msg=audit(1000000000.050:20000): cwd=\"/home/analyst\"\n" +
		"type=PROCTITLE msg=audit(1000000000.050:20000): proctitle=6C73002D6C\n" +
		executedEventSource
)

// readRecords は収集元を端まで走査し、レコードと診断を返す。
func readRecords(t *testing.T, source string) ([]auditd.Record, []string) {
	t.Helper()
	var reader auditd.Reader
	reader.Reset(strings.NewReader(source))
	records := make([]auditd.Record, 0, 8)
	problems := make([]string, 0, 4)
	for {
		record, failure, err := reader.Next()
		if failure != nil {
			problems = append(problems, failure.ObservedResult)
		}
		if err != nil {
			break
		}
		records = append(records, record)
	}
	return records, problems
}

// readOneRecord は 1 事象だけを持つ収集元から、その事象を返す。
func readOneRecord(t *testing.T, source string) auditd.Record {
	t.Helper()
	records, problems := readRecords(t, source)
	if len(problems) != 0 {
		t.Fatalf("reading the source reported %v, want no problem", problems)
	}
	if len(records) != 1 {
		t.Fatalf("the source holds %d records, want 1", len(records))
	}
	return records[0]
}
