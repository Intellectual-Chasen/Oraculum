package auditd_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/auditd"
)

// sourceEnv は、検査が読む監査ログの path を渡す環境変数である。値は監査ログ 1 件の path か、
// 監査ログだけを置いた directory の path である。設定しないとき検査を飛ばす。
const sourceEnv = "ORACULUM_AUDITD_SOURCE"

// eventKeyPattern は原資料から事象を指す鍵を数えるための文字列である。
var eventKeyPattern = regexp.MustCompile(`msg=audit\(([0-9.]+:[0-9]+)\)`)

// 環境変数が指す監査ログの全事象を取り込み、事象の数が msg=audit(...) の異なりの数と一致する。
func TestReaderCountsEveryEventOfTheEnvSource(t *testing.T) {
	for _, path := range envSources(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			wantEvents := distinctEventKeysOfFile(t, path)

			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("opening %s: %v", path, err)
			}
			defer func() {
				_ = file.Close() // 読み取りだけであり、閉じる失敗は結果を変えない
			}()

			var reader auditd.Reader
			reader.Reset(file)
			seen := make(map[string]struct{}, len(wantEvents))
			records := 0
			longestLine := 0
			problems := 0
			for {
				record, failure, err := reader.Next()
				if failure != nil {
					problems++
					line := int64(0)
					if failure.LineNumber != nil {
						line = *failure.LineNumber
					}
					t.Logf("failure at line %d: %s", line, failure.ObservedResult)
				}
				if err != nil {
					break
				}
				// 読めなかった範囲の診断は、レコードを伴わずに返る。
				if record.LineCount() == 0 {
					continue
				}
				records++
				seen[record.Event().RawText] = struct{}{}
				for _, line := range record.Lines() {
					longestLine = max(longestLine, len(line.RawText()))
				}
				if _, err := auditd.RecordFields(record); err != nil {
					t.Fatalf("building the fields of the event %q: %v",
						record.Event().RawText, err)
				}
				if _, err := auditd.EventTime(record.Event()); err != nil {
					t.Fatalf("building the event time of %q: %v", record.Event().RawText, err)
				}
			}
			t.Logf("records=%d events=%d longestLine=%d problems=%d",
				records, len(wantEvents), longestLine, problems)

			if records != len(wantEvents) {
				t.Errorf("the source holds %d records for %d distinct events",
					records, len(wantEvents))
			}
			for event := range wantEvents {
				if _, found := seen[event]; !found {
					t.Errorf("the event %q reached no record", event)
				}
			}
		})
	}
}

// envSources は sourceEnv が指す監査ログの path を返す。
func envSources(t *testing.T) []string {
	t.Helper()
	directory := os.Getenv(sourceEnv)
	if directory == "" {
		t.Skipf("set %s to an auditd source, or to a directory holding auditd sources, "+
			"to run this test", sourceEnv)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("reading %s: %v", directory, err)
	}
	if !info.IsDir() {
		return []string{directory}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading %s: %v", directory, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}
	if len(paths) == 0 {
		t.Fatalf("%s holds no .log file", directory)
	}
	return paths
}

// distinctEventKeysOfFile は原資料の事象を指す鍵の異なりを返す。
//
// **期待値を実装の出力から作らない。** 走査器を通さず、原資料の文字列を直に数える。
func distinctEventKeysOfFile(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() {
		_ = file.Close() // 読み取りだけであり、閉じる失敗は結果を変えない
	}()
	events := make(map[string]struct{}, 4096)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		if matched := eventKeyPattern.FindSubmatch(scanner.Bytes()); matched != nil {
			events[string(matched[1])] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return events
}
