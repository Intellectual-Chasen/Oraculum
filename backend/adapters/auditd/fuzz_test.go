package auditd_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
)

// 走査は、どの byte 列でも panic せずに終わる。
func FuzzReaderNext(f *testing.F) {
	for _, seed := range []string{
		executedEventSource, failedEventSource, sessionEventSource, daemonEventSource,
		truncatedHeadSource,
		"",
		"type=SYSCALL\n",
		"type=SYSCALL msg=audit(1.2:3)\n",
		"type=SYSCALL msg=audit(1.200:3): a=\"unclosed\n",
		"msg=audit(1.200:3): key=value" + separator + "KEY=\"value\"\n",
		"type=USER_START msg=audit(1.200:3): msg='op=x acct=\"y\"'\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		var reader auditd.Reader
		reader.Reset(strings.NewReader(source))
		for {
			record, _, err := reader.Next()
			if err != nil {
				return
			}
			// 読めなかった範囲の診断は、レコードを伴わずに返る。
			if record.LineCount() == 0 {
				continue
			}
			// 位置は原資料の範囲の中にある。
			if record.ByteOffset() < 0 || record.ByteOffset() > int64(len(source)) {
				t.Fatalf("the record starts at byte %d of a source of %d bytes",
					record.ByteOffset(), len(source))
			}
			if record.ByteLength() < 0 ||
				record.ByteOffset()+record.ByteLength() > int64(len(source)) {
				t.Fatalf("the record spans %d bytes from byte %d of a source of %d bytes",
					record.ByteLength(), record.ByteOffset(), len(source))
			}
			// 各行の原文は、収集元のその位置の byte 列と一致する。
			for _, line := range record.Lines() {
				end := line.ByteOffset() + int64(len(line.RawText()))
				if line.ByteOffset() < 0 || end > int64(len(source)) {
					t.Fatalf("the line spans bytes %d to %d of a source of %d bytes",
						line.ByteOffset(), end, len(source))
				}
				if at := source[line.ByteOffset():end]; at != line.RawText() {
					t.Fatalf("the line raw text is %q, want the source bytes %q",
						line.RawText(), at)
				}
			}
		}
	})
}

// 16 進の復号は、どの byte 列でも panic せずに終わる。
func FuzzRecordFields(f *testing.F) {
	for _, seed := range []string{
		executedEventSource, failedEventSource, daemonEventSource,
		"type=EXECVE msg=audit(1.200:3): argc=1 a0=2D\n",
		"type=EXECVE msg=audit(1.200:3): argc=1 a0=zz\n",
		"type=PROCTITLE msg=audit(1.200:3): proctitle=00\n",
		"type=PROCTITLE msg=audit(1.200:3): proctitle=\"not hex\"\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		var reader auditd.Reader
		reader.Reset(strings.NewReader(source))
		for {
			record, _, err := reader.Next()
			if err != nil {
				return
			}
			fields, err := auditd.RecordFields(record)
			if err != nil {
				continue
			}
			seen := make(map[string]struct{}, len(fields))
			for _, field := range fields {
				if _, duplicate := seen[field.Name]; duplicate {
					t.Fatalf("the field name %q occurs twice", field.Name)
				}
				seen[field.Name] = struct{}{}
			}
		}
	})
}
