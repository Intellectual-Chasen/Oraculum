package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// syntheticProcessRecord は端末 terminal のプロセス process のレコードを組む。
func syntheticProcessRecord(
	t *testing.T, line int64, start bool, terminal, process string, extra ...core.RecordField,
) RecordEntry {
	t.Helper()
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			ProcessStart:    start,
			Fields: append([]core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, terminal),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, process),
			}, extra...),
		},
	}
}

// 書き込んだファイルを後で同じ端末で起動すると、書き込みのファイルのノードから起動した
// プロセスへ実行の関係が出る。別の端末の同じ path の起動は、別のファイルのノードから出る。
func TestProcessExecutableLinksTheWrittenFileOnTheSameTerminal(t *testing.T) {
	const path = `C:\synthetic\tool.exe`
	graph := graphOfRecords(t, []RecordEntry{
		syntheticProcessRecord(t, 1, false, "T1", "{W1}",
			syntheticField(t, "target", core.SemanticKeyFilePath, path)),
		syntheticProcessRecord(t, 2, true, "T1", "{R1}",
			syntheticField(t, "image", core.SemanticKeyProcessBinaryPath, path)),
		syntheticProcessRecord(t, 3, true, "T2", "{R2}",
			syntheticField(t, "image", core.SemanticKeyProcessBinaryPath, path)),
	})
	writes := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindFileOperation}, Depth: 1}).Edges
	if len(writes) != 1 {
		t.Fatalf("got %d file operations, want 1", len(writes))
	}
	written := writes[0].TargetNodeId
	subgraph := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindProcessExecutable}, Depth: 1})
	if len(subgraph.Edges) != 2 {
		t.Fatalf("got %d executable relations, want 2", len(subgraph.Edges))
	}
	processOf := make(map[string]string)
	for _, node := range subgraph.Nodes {
		if node.Kind == core.NodeKindProcess {
			processOf[node.Id] = node.Identity[1].Value
		}
	}
	for _, edge := range subgraph.Edges {
		switch processOf[edge.TargetNodeId] {
		case "{R1}":
			if edge.SourceNodeId != written {
				t.Errorf("the start on T1 runs %q, want the written file %q", edge.SourceNodeId, written)
			}
		case "{R2}":
			if edge.SourceNodeId == written {
				t.Errorf("the start on T2 runs the file written on T1")
			}
		default:
			t.Errorf("the executable relation ends at %q, want a started process", edge.TargetNodeId)
		}
	}
}

// 起動のレコードでないレコードの実行ファイルの path は、実行の関係を作らない。
func TestProcessExecutableNeedsTheStartRecord(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		syntheticProcessRecord(t, 1, false, "T1", "{P1}",
			syntheticField(t, "image", core.SemanticKeyProcessBinaryPath, `C:\synthetic\tool.exe`)),
	})
	edges := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindProcessExecutable}, Depth: 1}).Edges
	if len(edges) != 0 {
		t.Errorf("got %d executable relations, want 0", len(edges))
	}
}
