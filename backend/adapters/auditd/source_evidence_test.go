package auditd_test

import (
	"os"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
)

// EXECVE を持つ事象の集合と success=no の事象の集合は交わらない。
func TestEnvSourceSeparatesTheFailedExecveEvents(t *testing.T) {
	records := recordsByEvent(t)
	executed := make(map[string]struct{}, len(records))
	failed := make(map[string]struct{}, len(records))
	for event, record := range records {
		if _, found := record.LineOfType(auditd.LineTypeExecve); found {
			executed[event] = struct{}{}
		}
		syscall, found := record.LineOfType(auditd.LineTypeSyscall)
		if !found {
			continue
		}
		if success, ok := syscall.Item("success", false); ok && success.Value() == "no" {
			failed[event] = struct{}{}
		}
	}
	t.Logf("executed=%d failed=%d", len(executed), len(failed))
	if len(executed) == 0 || len(failed) == 0 {
		t.Fatalf("the source holds %d executed and %d failed events, want both",
			len(executed), len(failed))
	}
	for event := range failed {
		if _, both := executed[event]; both {
			t.Errorf("the event %q carries both an EXECVE line and success=no", event)
		}
	}
}

// recordsByEvent は sourceEnv が指す監査ログの全事象を、事象を指す鍵で探せる形で返す。
func recordsByEvent(t *testing.T) map[string]auditd.Record {
	t.Helper()
	paths := envSources(t)
	records := make(map[string]auditd.Record, 4096)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening %s: %v", path, err)
		}
		var reader auditd.Reader
		reader.Reset(file)
		for {
			record, _, err := reader.Next()
			if err != nil {
				break
			}
			if record.LineCount() == 0 {
				continue
			}
			records[record.Event().RawText] = record
		}
		_ = file.Close() // 読み取りだけであり、閉じる失敗は結果を変えない
	}
	return records
}
