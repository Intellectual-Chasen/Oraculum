package pipeline

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// SigmaQueryResult は、Sigma の評価の一致を検索の条件でフィルタした結果である。
type SigmaQueryResult struct {
	// Rules は、フィルタ後に 1 件以上の一致を持つルールを評価の並びで持つ。
	Rules []SigmaQueryRule
	// Matches は、フィルタを通った一致を評価の並びで持つ。
	Matches []SigmaQueryMatch
	// OutsideGraphMatchCount は、条件を与えた要求で、グラフに無いレコードのため除いた一致の数である。
	// 条件を与えない要求では 0 である。
	OutsideGraphMatchCount int64
}

// SigmaQueryRule は一致したルール 1 つと、一致したレコードを根拠に持つノードとエッジである。
type SigmaQueryRule struct {
	// SigmaMatchedRule の MatchCount は、フィルタを通った一致の数である。
	SigmaMatchedRule
	// Graph は、フィルタを通った一致のレコードのどれかを根拠に持つノードとエッジである。
	// ノードはエッジの端点を含む (RecordGraphElements と同じ定義)。
	Graph RecordGraphElements
}

// SigmaQueryMatch は一致 1 件と、一致したレコードのノードである。
type SigmaQueryMatch struct {
	SigmaRuleMatch
	// RecordNode は一致したレコードのノードである。レコードがグラフに無いか、レコードのノードを
	// 持たないときは nil である。
	RecordNode *core.GraphNode
}

// SigmaMatchesIn は、評価の一致のうち、レコードが要求のレコードのフィルタと文字列の条件を通るものを返す。
//
// **使う条件は、レコードのフィルタと文字列の条件だけである。** 起点・ホップ数・種別・上限は、
// レコード単位の一致の範囲を変えない。文字列の条件は、レコードのノードの属性で判定する
// (recordPasses)。条件を与えた要求では、グラフに無いレコードの一致を除き、その数を返す。
func (g Graph) SigmaMatchesIn(query GraphQuery, evaluation SigmaEvaluation) SigmaQueryResult {
	query = g.withResolvedFieldNames(query)
	filtered := query.filtersRecords() || query.SearchesText()
	passes := g.recordPasses(query)
	result := SigmaQueryResult{Rules: []SigmaQueryRule{}, Matches: []SigmaQueryMatch{}}
	// recordsOf はルールの path から、フィルタを通った一致のレコードの g.records での位置を求める。
	recordsOf := make(map[string]map[int]bool, len(evaluation.Rules))
	counts := make(map[string]int64, len(evaluation.Rules))
	for _, match := range evaluation.Matches {
		at, inGraph := g.recordAtLocator(match.Record)
		if filtered && (!inGraph || !passes(at)) {
			if !inGraph {
				result.OutsideGraphMatchCount++
			}
			continue
		}
		item := SigmaQueryMatch{SigmaRuleMatch: match}
		if inGraph {
			if recordsOf[match.RulePath] == nil {
				recordsOf[match.RulePath] = map[int]bool{}
			}
			recordsOf[match.RulePath][at] = true
			if record := g.records[at]; record.hasRecordNode {
				node := g.graphNode(record.recordNode)
				item.RecordNode = &node
			}
		}
		counts[match.RulePath]++
		result.Matches = append(result.Matches, item)
	}
	for _, rule := range evaluation.Rules {
		if counts[rule.Path] == 0 {
			continue
		}
		rule.MatchCount = counts[rule.Path]
		// ponytail: ルールごとに全ノードと全エッジの根拠を走査する。一致したルールの数に比例する。
		// 遅さが問題になったら、ルールの集合を 1 回の走査で求める。
		result.Rules = append(result.Rules, SigmaQueryRule{
			SigmaMatchedRule: rule, Graph: g.recordsGraphElements(recordsOf[rule.Path]),
		})
	}
	return result
}
