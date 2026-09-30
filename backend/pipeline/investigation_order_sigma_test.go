// in-package test: ルールの一致から、Sigma の一致の集計で対象を並べる。
package pipeline

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sigmaTestRule はルール 1 つのレベルと、一致したレコードの g.records での位置である。
type sigmaTestRule struct {
	level   string
	records []int
}

// sigmaTestEvaluation はルールの一致から、ルールの集合を渡した起動の評価の結果を組む。
func sigmaTestEvaluation(graph Graph, rules ...sigmaTestRule) SigmaEvaluation {
	evaluation := SigmaEvaluation{RuleSet: &SigmaRuleSetInfo{
		Directory: "rules", RevisionSource: "unverified", ContentSha256: strings.Repeat("a", 64),
	}}
	for i, rule := range rules {
		path := fmt.Sprintf("rules/synthetic_%d.yml", i)
		evaluation.Rules = append(evaluation.Rules, SigmaMatchedRule{Path: path, Level: rule.level})
		for _, at := range rule.records {
			evaluation.Matches = append(evaluation.Matches,
				SigmaRuleMatch{RulePath: path, Record: graph.records[at].locator})
		}
	}
	return evaluation
}

func sigmaOrderOf(t *testing.T, graph Graph, sigma SigmaEvaluation) InvestigationOrder {
	t.Helper()
	return sigmaOrderAtLeast(t, graph, sigma, "")
}

// sigmaOrderAtLeast は、minLevel 以上のレベルのルールの一致だけで並べる。
func sigmaOrderAtLeast(t *testing.T, graph Graph, sigma SigmaEvaluation, minLevel string) InvestigationOrder {
	t.Helper()
	order, known := graph.InvestigationOrder(GraphQuery{}, InvestigationOrderSigmaMatches, sigma, minLevel)
	if !known {
		t.Fatal("InvestigationOrder reports the sigma method as unknown")
	}
	return order
}

// processValuesOf はプロセスの識別の値から、手法の値を求める。
func processValuesOf(t *testing.T, order InvestigationOrder) map[string]*float64 {
	t.Helper()
	at := slices.IndexFunc(order.Kinds, func(k InvestigationOrderKind) bool { return k.Kind == core.NodeKindProcess })
	if at < 0 {
		t.Fatalf("the order has no process entries: %+v", order.Kinds)
	}
	values := make(map[string]*float64)
	for _, entry := range order.Kinds[at].Entries {
		values[entry.Node.Identity[len(entry.Node.Identity)-1].Value] = entry.Value
	}
	return values
}

func TestInvestigationOrderSigmaPutsHigherLevelAboveMoreRecords(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `C:\b.txt`),
		orderTestRecord(t, 3, "{P2}", `C:\c.txt`),
		orderTestRecord(t, 4, "{P2}", `C:\d.txt`))
	// レベルの文字列は大文字と小文字を区別しない。
	order := sigmaOrderOf(t, graph, sigmaTestEvaluation(graph,
		sigmaTestRule{level: "HIGH", records: []int{0}},
		sigmaTestRule{level: "low", records: []int{1, 2, 3}}))

	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 1}, {"{P2}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want the high match above the 3 low matches", got)
	}
	values := processValuesOf(t, order)
	if got := values["{P1}"]; got == nil || *got != 5*sigmaLevelRankMultiplier+1 {
		t.Errorf("value of {P1} = %v, want high (5) and 1 record", got)
	}
	if got := values["{P2}"]; got == nil || *got != 3*sigmaLevelRankMultiplier+3 {
		t.Errorf("value of {P2} = %v, want low (3) and 3 records", got)
	}
}

func TestInvestigationOrderSigmaPutsMoreRecordsAboveInTheSameLevel(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	// {P1} は位置 0 と 2、{P2} は位置 1 のレコードに指される。位置 1 は 2 つのルールに一致する。
	order := sigmaOrderOf(t, graph, sigmaTestEvaluation(graph,
		sigmaTestRule{level: "medium", records: []int{0, 1, 2}},
		sigmaTestRule{level: "medium", records: []int{1}}))

	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 1}, {"{P2}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want 2 medium records above 1 record matched by 2 rules", got)
	}
}

func TestInvestigationOrderSigmaPutsUnmatchedObjectsBelow(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `C:\b.txt`))
	sigma := sigmaTestEvaluation(graph, sigmaTestRule{level: "low", records: []int{0}})
	// グラフに無いレコードの一致は数えない。
	line := int64(1)
	sigma.Matches = append(sigma.Matches, SigmaRuleMatch{
		RulePath: sigma.Rules[0].Path,
		Record: core.RecordLocator{
			SourceId: "absent", SourceFileName: "absent.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
	})
	order := sigmaOrderOf(t, graph, sigma)

	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 1}, {"{P2}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want the unmatched process below", got)
	}
	values := processValuesOf(t, order)
	if got := values["{P1}"]; got == nil || *got != 3*sigmaLevelRankMultiplier+1 {
		t.Errorf("value of {P1} = %v, want low (3) and 1 record", got)
	}
	if got := values["{P2}"]; got == nil || *got != 0 {
		t.Errorf("value of {P2} = %v, want 0 for no match", got)
	}
}

func TestInvestigationOrderSigmaLeavesValuesOutWithoutRuleSet(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	sigma := sigmaTestEvaluation(graph, sigmaTestRule{level: "critical", records: []int{0}})
	sigma.RuleSet = nil
	order := sigmaOrderOf(t, graph, sigma)

	for _, kind := range order.Kinds {
		for _, entry := range kind.Entries {
			if entry.Value != nil || entry.Rank != 1 {
				t.Errorf("%s entry = %+v, want no value and rank 1 without a rule set", kind.Kind, entry)
			}
		}
	}
	for _, parameter := range order.Parameters {
		if strings.HasPrefix(parameter.Name, "rule_set_") {
			t.Errorf("parameter %+v is reported without a rule set", parameter)
		}
	}
}

func TestInvestigationOrderSigmaPutsUnknownLevelBelowInformational(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `C:\b.txt`),
		orderTestRecord(t, 3, "{P3}", `C:\c.txt`),
		orderTestRecord(t, 4, "{P4}", `C:\d.txt`))
	order := sigmaOrderOf(t, graph, sigmaTestEvaluation(graph,
		sigmaTestRule{level: "severe", records: []int{0}},
		sigmaTestRule{level: "informational", records: []int{1}},
		sigmaTestRule{level: "", records: []int{2}}))

	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P2}", 1, 1}, {"{P1}", 2, 2}, {"{P3}", 2, 2}, {"{P4}", 4, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want informational, then unknown and empty levels, then no match", got)
	}
}

func TestInvestigationOrderSigmaCountsOnlyRulesAtOrAboveTheMinimumLevel(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P1}", `C:\b.txt`),
		orderTestRecord(t, 3, "{P2}", `C:\c.txt`))
	sigma := sigmaTestEvaluation(graph,
		sigmaTestRule{level: "high", records: []int{0}},
		sigmaTestRule{level: "low", records: []int{1, 2}})

	for _, tc := range []struct {
		minLevel, reported string
		p1, p2             float64
	}{
		{"", "all", 5*sigmaLevelRankMultiplier + 2, 3*sigmaLevelRankMultiplier + 1},
		{"low", "low", 5*sigmaLevelRankMultiplier + 2, 3*sigmaLevelRankMultiplier + 1},
		{"medium", "medium", 5*sigmaLevelRankMultiplier + 1, 0},
		{"critical", "critical", 0, 0},
	} {
		order := sigmaOrderAtLeast(t, graph, sigma, tc.minLevel)
		values := processValuesOf(t, order)
		if values["{P1}"] == nil || *values["{P1}"] != tc.p1 || values["{P2}"] == nil || *values["{P2}"] != tc.p2 {
			t.Errorf("min level %q: {P1} = %v, {P2} = %v, want %v and %v",
				tc.minLevel, values["{P1}"], values["{P2}"], tc.p1, tc.p2)
		}
		at := slices.IndexFunc(order.Parameters, func(p InvestigationOrderParameter) bool { return p.Name == "min_level" })
		if at < 0 || order.Parameters[at].Value != tc.reported {
			t.Errorf("min level %q: parameters = %+v, want min_level = %s", tc.minLevel, order.Parameters, tc.reported)
		}
	}
}

func TestInvestigationOrderSigmaReportsRuleSetRevision(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	sigma := sigmaTestEvaluation(graph)
	parameterOf := func(order InvestigationOrder, name string) string {
		at := slices.IndexFunc(order.Parameters, func(p InvestigationOrderParameter) bool { return p.Name == name })
		if at < 0 {
			t.Fatalf("parameters = %+v, want %s", order.Parameters, name)
		}
		return order.Parameters[at].Value
	}

	if got := parameterOf(sigmaOrderOf(t, graph, sigma), "rule_set_revision"); got != "unverified" {
		t.Errorf("revision = %q, want unverified for a rule set without a revision", got)
	}
	revision := "0123abcd"
	sigma.RuleSet.Revision = &revision
	order := sigmaOrderOf(t, graph, sigma)
	if got := parameterOf(order, "rule_set_revision"); got != revision {
		t.Errorf("revision = %q, want %q", got, revision)
	}
	if got := parameterOf(order, "rule_set_content_sha256"); got != sigma.RuleSet.ContentSha256 {
		t.Errorf("content sha256 = %q, want %q", got, sigma.RuleSet.ContentSha256)
	}
	if got := parameterOf(order, "evaluated_record_count"); got != "0" {
		t.Errorf("evaluated record count = %q, want 0 when no record was evaluated", got)
	}
	sigma.EvaluatedRecordCount = 7
	if got := parameterOf(sigmaOrderOf(t, graph, sigma), "evaluated_record_count"); got != "7" {
		t.Errorf("evaluated record count = %q, want 7", got)
	}
}

// 文字列の条件を与えた要求は、条件を満たすレコードの対象だけを、条件を満たすレコードの一致で並べる。
func TestInvestigationOrderSigmaUsesTheTextConditions(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `C:\b.txt`),
		orderTestRecord(t, 3, "{P2}", `C:\c.txt`))
	sigma := sigmaTestEvaluation(graph, sigmaTestRule{level: "high", records: []int{0, 1, 2}})
	order, known := graph.InvestigationOrder(
		GraphQuery{ValueContains: []string{"b.txt"}}, InvestigationOrderSigmaMatches, sigma, "")
	if !known {
		t.Fatal("InvestigationOrder reports the sigma method as unknown")
	}
	values := processValuesOf(t, order)
	if _, present := values["{P1}"]; present {
		t.Errorf("{P1} is ordered although no record of {P1} contains b.txt: %v", values)
	}
	if got := values["{P2}"]; got == nil || *got != 5*sigmaLevelRankMultiplier+1 {
		t.Errorf("value of {P2} = %v, want high (5) and the 1 record that contains b.txt", got)
	}
}
