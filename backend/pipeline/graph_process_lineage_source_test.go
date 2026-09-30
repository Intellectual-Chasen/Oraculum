// in-package test: 環境変数が指す入力の取り込み結果から、非公開のグラフの中身を数える。
package pipeline

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// lineageSourceEnv は、入力の auditd の監査ログ 1 件の path を渡す環境変数である。
// 値が無ければ検査を飛ばす。
const lineageSourceEnv = "ORACULUM_AUDITD_SOURCE"

// lineageSourceEnv が指す入力で、プロセスのノードと親子の候補の数を数える。
func TestProcessLineageOverTheEnvSource(t *testing.T) {
	path := os.Getenv(lineageSourceEnv)
	if path == "" {
		t.Skipf("%s が設定されていないため飛ばす", lineageSourceEnv)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- 検査が env で受け取った path の入力を読む。
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	result := auditdImportResult(t, string(content))
	graph := NewGraph(result.WithAnalystTerminalAssignments(
		[]core.TerminalAssignment{analystAssignmentFor(t, result)}), AllMatchConditions())

	processes := 0
	for _, node := range graph.nodes {
		if node.key.Form == core.NodeKeyFormTerminalProcessInterval {
			processes++
		}
	}
	parents := len(lineageParentChildEdges(graph))
	// 原文の文字列から数えた、プロセス番号を持つ事象の数。
	wantInstances := strings.Count(string(content), "type=SYSCALL msg=audit(")
	t.Logf("records=%d processes=%d parentChildEdges=%d syscallLines=%d",
		countRecords(result), processes, parents, wantInstances)

	// **区間の数と一致する。** 区間を開くのは起動を記録した事象と、起動を見ていない
	// 番号の最初の事象だけである。同じ番号の続きの事象は開いている区間へ入る。
	if want := processIntervals(string(content)); processes != want {
		t.Errorf("the graph holds %d process nodes for %d intervals", processes, want)
	}
	if processes == 0 {
		t.Fatal("the graph holds no process node")
	}
	if parents == 0 {
		t.Error("the graph holds no parent and child edge")
	}
	for _, edge := range lineageParentChildEdges(graph) {
		if edge.state != core.RelationStateCandidate {
			t.Fatalf("a parent and child edge is %q, want %q",
				edge.state, core.RelationStateCandidate)
		}
	}
}

// processIntervals は、原文からプロセスの識別が有効な区間の数を数える。
//
// 区間を開くのは、起動を記録した事象 (EXECVE の行を持つ事象) と、起動をまだ見ていない
// 番号の最初の事象である。同じ番号の続きの事象は開いている区間に属する。
//
// **期待値を実装の出力から作らない。** 走査器を通さず、原資料の文字列を直に読む。
// **同じ番号と同じ時刻の起動は 1 つの区間である。** 実行の失敗が並ぶ区間では、1 つの
// プロセスが同じミリ秒の中で複数の path を試し、事象が複数になる。
func processIntervals(content string) int {
	pids, starts, order := eventsOfSource(content)
	intervals := 0
	// open は番号ごとの、開いている区間の始まりの時刻である。
	open := make(map[string]string, 4096)
	for _, key := range order {
		pid, named := pids[key]
		if !named {
			continue
		}
		instant, _, _ := strings.Cut(key, ":")
		start, opened := open[pid]
		if opened {
			if _, starting := starts[key]; !starting {
				continue
			}
			if start == instant {
				continue
			}
		}
		intervals++
		open[pid] = instant
	}
	return intervals
}

// eventsOfSource は、事象の鍵ごとのプロセス番号と、起動を記録した事象の鍵と、
// 時刻の順に並べた鍵の並びを返す。
func eventsOfSource(content string) (map[string]string, map[string]struct{}, []string) {
	pids := make(map[string]string, 4096)
	starts := make(map[string]struct{}, 4096)
	order := make([]string, 0, 4096)
	for _, line := range strings.Split(content, "\n") {
		key, found := eventKeyOfLine(line)
		if !found {
			continue
		}
		// **プロセス番号は SYSCALL の行からだけ読む。** SERVICE_START や DAEMON_END は
		// 番号の欄を持つが、その番号が指すプロセスの実行を記録していない。
		if _, seen := pids[key]; !seen && strings.HasPrefix(line, "type=SYSCALL ") {
			if pid, named := cutFieldValue(line, " pid="); named {
				pids[key] = pid
				order = append(order, key)
			}
		}
		if strings.HasPrefix(line, "type=EXECVE ") {
			starts[key] = struct{}{}
		}
	}
	sort.SliceStable(order, func(left, right int) bool {
		return order[left] < order[right]
	})
	return pids, starts, order
}

// eventKeyOfLine は行が名乗る事象の時刻を、順の比較に使える文字列で返す。
func eventKeyOfLine(line string) (string, bool) {
	_, after, found := strings.Cut(line, "msg=audit(")
	if !found {
		return "", false
	}
	seconds, rest, found := strings.Cut(after, ":")
	if !found {
		return "", false
	}
	serial, _, found := strings.Cut(rest, ")")
	if !found {
		return "", false
	}
	// 秒は同じ桁数で並ぶため、文字列の大小が時刻の順に一致する。連番を後ろに置き、
	// 同じ時刻の事象を 1 つの鍵にまとめない。
	return seconds + ":" + serial, true
}

// cutFieldValue は key の後ろの値を、次の空白までの文字列として返す。
func cutFieldValue(line, key string) (string, bool) {
	_, after, found := strings.Cut(line, key)
	if !found {
		return "", false
	}
	value, _, _ := strings.Cut(after, " ")
	return value, true
}

func countRecords(result ImportResult) int {
	records := 0
	for _, publication := range result.publications {
		records += len(publication.records)
	}
	return records
}
