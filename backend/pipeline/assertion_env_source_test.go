// in-package test: fixture のグラフを組む非公開の helper を使う。
package pipeline

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// envSourceDirVar は、入力の markii 形式の log を置いた directory を指す環境変数の名前である。
// 値が無ければ検査を飛ばす。
const envSourceDirVar = "ORACULUM_MARKII_SOURCE_DIR"

// envSourceRunner は envSourceDirVar が指す directory の file を読む取り込みの実行器を返す。
//
// 収集元の識別は取得元の path をリポジトリ相対で持つため、取り込みへ渡すのは file 名だけに
// し、置き場は本関数が足す。
//
// **通番の発行器を器が持つ。** 同じ器で 2 回取り込むと、2 回目は別の通番を受け取り、
// 別の sourceId になる。
func envSourceRunner(t *testing.T, logDir string) *Runner {
	t.Helper()
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			file, err := os.Open(filepath.Join(logDir, originPath))
			if err != nil {
				return nil, err
			}
			return file, nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize:       func(value string) string { return value },
		Revision:       "env-source-revision",
		SettingsDigest: "env-source-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// envSourcePlan は envSourceDirVar が指す directory から markii 形式の log を、file 名の
// 昇順で最初の 1 件を選ぶ。
func envSourcePlan(t *testing.T) (string, SourcePlan) {
	t.Helper()
	logDir := os.Getenv(envSourceDirVar)
	if logDir == "" {
		t.Skip(envSourceDirVar + " が設定されていないため飛ばす")
	}
	paths, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		t.Fatalf("listing the logs in %s: %v", logDir, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no log file found in %s", logDir)
	}
	sort.Strings(paths)
	fileName := filepath.Base(paths[0])
	return logDir, SourcePlan{
		OriginPath: fileName, FileName: fileName, FormatKey: MarkIIFormatKey,
	}
}

// envSourceDirVar が指す入力を 2 回取り込むと sourceId は変わり、所見が指すノード・関係・
// レコードは同じ対象を指したままである。
func TestAssertionTargetsSurviveTheReimportOfTheEnvSource(t *testing.T) {
	logDir, plan := envSourcePlan(t)
	runner := envSourceRunner(t, logDir)
	firstResult, err := runner.Run([]SourcePlan{plan})
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := runner.Run([]SourcePlan{plan})
	if err != nil {
		t.Fatal(err)
	}

	firstSources := sourceIdsOf(t, firstResult)
	secondSources := sourceIdsOf(t, secondResult)
	if len(firstSources) == 0 {
		t.Fatalf("the import of %s read no source", plan.FileName)
	}
	for index, sourceId := range firstSources {
		if sourceId == secondSources[index] {
			t.Fatalf("the reimport of %s kept the sourceId %q, want a new one",
				plan.FileName, sourceId)
		}
	}

	firstGraph := NewGraph(firstResult, AllMatchConditions())
	secondGraph := NewGraph(secondResult, AllMatchConditions())
	firstSubgraph := firstGraph.Query(GraphQuery{Depth: 1})
	if len(firstSubgraph.Nodes) == 0 || len(firstSubgraph.Edges) == 0 {
		t.Fatalf("the graph of %s carries %d nodes and %d edges, want both populated",
			plan.FileName, len(firstSubgraph.Nodes), len(firstSubgraph.Edges))
	}
	resolver := NewAssertionResolver(secondGraph)

	for _, node := range firstSubgraph.Nodes {
		assertion := assertionOf(core.AssertionTarget{
			Kind: core.AssertionTargetKindNode, NodeId: node.Id,
		})
		if origin := resolver.Origin(assertion, noAddedRelations()); origin !=
			core.AssertionTargetOriginObservation {
			t.Errorf("Origin() of the node %q after the reimport = %q, want %q",
				node.Id, origin, core.AssertionTargetOriginObservation)
		}
	}
	for _, edge := range firstSubgraph.Edges {
		want := core.AssertionTargetOriginObservation
		if edge.State == core.RelationStateCandidate {
			want = core.AssertionTargetOriginMatchingCandidate
		}
		assertion := assertionOf(edgeTarget(core.AssertionEdgeRef{
			Kind: edge.Kind, SourceNodeId: edge.SourceNodeId, TargetNodeId: edge.TargetNodeId,
		}))
		if origin := resolver.Origin(assertion, noAddedRelations()); origin != want {
			t.Errorf("Origin() of the edge %q after the reimport = %q, want %q",
				edge.Id, origin, want)
		}
	}
	for _, record := range firstGraph.records {
		assertion := assertionOf(core.AssertionTarget{
			Kind:   core.AssertionTargetKindRecord,
			Record: recordRefPointer(core.NewAssertionRecordRef(record.locator)),
		})
		if origin := resolver.Origin(assertion, noAddedRelations()); origin !=
			core.AssertionTargetOriginObservation {
			t.Errorf("Origin() of the record at %+v after the reimport = %q, want %q",
				record.locator.LineNumber, origin, core.AssertionTargetOriginObservation)
		}
	}
	t.Logf("file=%s nodes=%d edges=%d records=%d", plan.FileName,
		len(firstSubgraph.Nodes), len(firstSubgraph.Edges), len(firstGraph.records))
}
