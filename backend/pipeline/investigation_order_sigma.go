package pipeline

import (
	"fmt"
	"strconv"
	"strings"
)

// InvestigationOrderSigmaMatches は、対象を指すレコードに一致した Sigma のルールの最高レベルの
// 高い順、同じレベルでは一致したレコードの件数の多い順である。
const InvestigationOrderSigmaMatches = "sigma"

// sigmaLevelRankMultiplier は、値の中でレベルの順位に掛ける倍率である。
//
// 既知の制限: 対象 1 つの一致したレコードの件数は sigmaLevelRankMultiplier (10^9) 未満とする。
// 件数がこれに達すると、値が 1 つ上のレベルの値と重なる。critical の値の最大が 10 桁に収まり、
// 有効数字 10 桁に丸めて比べる compareOrderValues でも件数の差が残る倍率である。
// 対象 1 つの一致したレコードが 10^8 件を超える入力が現れたら、値をレベルと件数の 2 つの鍵に分ける。
const sigmaLevelRankMultiplier = 1_000_000_000

// sigmaOtherLevelRank は、sigmaLevelRanks に無いレベルの文字列と空の文字列の順位である。
// 一致しない対象の順位 0 より上、informational より下である。
const sigmaOtherLevelRank = 1

// sigmaLevelRanks は、小文字にしたレベルの文字列から順位を探す表である。
var sigmaLevelRanks = map[string]int{"informational": 2, "low": 3, "medium": 4, "high": 5, "critical": 6}

// sigmaLevelRankOf はレベルの文字列の順位を返す。文字列は大文字と小文字を区別せずに比べる。
func sigmaLevelRankOf(level string) int {
	if rank, known := sigmaLevelRanks[strings.ToLower(level)]; known {
		return rank
	}
	return sigmaOtherLevelRank
}

// KnownSigmaLevel は、level が並びに使うレベルの下限として受け付ける文字列 (小文字) かを返す。
func KnownSigmaLevel(level string) bool {
	_, known := sigmaLevelRanks[level]
	return known
}

// sigmaMatchValues は、対象を指すレコードのうちルールに一致したレコードについて、最高レベルの
// 順位 × sigmaLevelRankMultiplier + 一致したレコードの件数を値にする。一致したレコードが無い対象は
// 値 0 である。ルールの集合を渡していない起動では、全対象が値を持たない。
//
// 1 件のレコードが複数のルールに一致しても 1 件と数える。グラフに無いレコードの一致は数えない。
// 下限のレベル (input.sigmaMinLevel) を与えたときは、それより低いレベルのルールの一致を数えない。
//
// 既知の制限: ルールを当てていないレコード (Windows イベントログ以外など) だけに指された対象も、
// 一致しなかった対象と同じ値 0 にする。測定で決めた手法の定義である。当てたかどうかを並びで
// 分ける要求が出たら、当てたレコードを持たない対象を値なしにする。
func sigmaMatchValues(g Graph, input orderInput) []orderValue {
	values := make([]orderValue, len(input.objects))
	if input.sigma.RuleSet == nil {
		return values
	}
	levelOf := make(map[string]string, len(input.sigma.Rules))
	for _, rule := range input.sigma.Rules {
		levelOf[rule.Path] = rule.Level
	}
	minRank := sigmaLevelRanks[input.sigmaMinLevel]
	// highest はレコードの g.records での位置から、一致したルールの最高レベルの順位を求める。
	highest := make(map[int]int)
	for _, match := range input.sigma.Matches {
		rank := sigmaLevelRankOf(levelOf[match.RulePath])
		if rank < minRank {
			continue
		}
		at, inGraph := g.recordAtLocator(match.Record)
		if !inGraph {
			continue
		}
		highest[at] = max(highest[at], rank)
	}
	for i, records := range input.records {
		rank, count := 0, 0
		for _, at := range records {
			if level, matched := highest[at]; matched {
				rank, count = max(rank, level), count+1
			}
		}
		values[i] = orderValue{value: float64(rank*sigmaLevelRankMultiplier + count), present: true}
	}
	return values
}

// sigmaMatchParameters は、ルールの集合の commit と内容の識別、ルールを当てたレコードの数と、下限の
// レベル、値の組み方を返す。ルールの集合を渡していない起動では、下限のレベルと値の組み方だけを返す。
// 下限のレベルを与えていない要求の下限は all である。
func sigmaMatchParameters(input orderInput) []InvestigationOrderParameter {
	var parameters []InvestigationOrderParameter
	if ruleSet := input.sigma.RuleSet; ruleSet != nil {
		revision := "unverified"
		if ruleSet.Revision != nil {
			revision = *ruleSet.Revision
		}
		parameters = append(parameters,
			InvestigationOrderParameter{Name: "rule_set_revision", Value: revision},
			InvestigationOrderParameter{Name: "rule_set_content_sha256", Value: ruleSet.ContentSha256},
			InvestigationOrderParameter{
				Name: "evaluated_record_count", Value: strconv.FormatInt(input.sigma.EvaluatedRecordCount, 10),
			})
	}
	minLevel := input.sigmaMinLevel
	if minLevel == "" {
		minLevel = "all"
	}
	return append(parameters,
		InvestigationOrderParameter{Name: "min_level", Value: minLevel},
		InvestigationOrderParameter{
			Name:  "value_formula",
			Value: fmt.Sprintf("level_rank * %d + matched_record_count", sigmaLevelRankMultiplier),
		},
		InvestigationOrderParameter{
			Name:  "level_ranks",
			Value: "none=0 other=1 informational=2 low=3 medium=4 high=5 critical=6",
		})
}
