package core

import (
	"strings"
	"testing"
)

// 端末の外部識別子の鍵は識別子の文字列そのもの、外部識別子を持たない端末の鍵は形と値を連ねた
// 文字列になる。別の鍵は別の値になる。
func TestTerminalMatchValueComparesTheTerminalNode(t *testing.T) {
	byId, _ := TerminalNodeKey("host-1")
	sha := strings.Repeat("a", 64)
	recording, _ := RecordingTerminalNodeKey(sha)
	hosted, _ := RecordingHostTerminalNodeKey(sha, "host-1.example.test")
	values := map[string]bool{}
	for _, testCase := range []struct {
		key  NodeKey
		want string
	}{
		{byId, "host-1"},
		{recording, string(NodeKeyFormRecordingSource) + "/" + sha},
		{hosted, string(NodeKeyFormRecordingSourceHostname) + "/" + sha + "/host-1.example.test"},
	} {
		got, ok := TerminalMatchValue(testCase.key)
		if !ok || got != testCase.want {
			t.Errorf("TerminalMatchValue(%+v) = %q, %v, want %q", testCase.key, got, ok, testCase.want)
		}
		values[got] = true
	}
	if len(values) != 3 {
		t.Errorf("the three terminals share a value: %v", values)
	}
	if _, ok := TerminalMatchValue(NodeKey{Kind: NodeKindProcess, Values: byId.Values}); ok {
		t.Error("a process key yields a terminal value")
	}
}

// 端末を外部識別子かノードの識別子のどちらかで指すプロセスの参照だけが検査を通る。
func TestProcessRefNeedsTheTerminalIdOrTheTerminalNode(t *testing.T) {
	ref := ProcessRef{SourceId: "source-1", SourceContentSha256: strings.Repeat("b", 64), ProcessId: "p-1"}
	if ref.Validate() == nil {
		t.Error("a reference without a terminal passes")
	}
	withNode := ref
	withNode.TerminalNodeId = "n:terminal:1"
	withId := ref
	withId.TerminalId = "host-1"
	for _, valid := range []ProcessRef{withNode, withId} {
		if err := valid.Validate(); err != nil {
			t.Errorf("the reference %+v fails: %v", valid, err)
		}
	}
}
