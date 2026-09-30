package api

import (
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const sigmaRuleCandidatesPattern = "GET /api/v0/sigma-rule-candidates"

// sigmaRuleCandidatesHandler は、起動時に Windows イベントログのレコードへ Sigma のルールを
// 当てた結果を、要求のレコードのフィルタと文字列の条件でフィルタして返す。レコードは取り込みの
// 後に変わらないため、要求ごとに当て直さない。
type sigmaRuleCandidatesHandler struct {
	graph      pipeline.Graph
	evaluation pipeline.SigmaEvaluation
}

// sigmaRuleCandidatesResponse は Sigma のルールの候補の応答である。
// **本型が項目の定義元である。** 一致は候補であり、関係や分析者の判断を作らない。
type sigmaRuleCandidatesResponse struct {
	// RuleSet は評価に使ったルールの集合を特定する情報である。ルールの集合を渡していない起動では出ない。
	RuleSet *pipeline.SigmaRuleSetInfo `json:"ruleSet,omitempty"`
	// EvaluatedRuleCount から RecordsWithoutSemantics までは、要求の条件に依らない評価全体の値である。
	// EvaluatedRuleCount は評価したルールの数、EvaluatedRecordCount はルールを当てた
	// Windows イベントログのレコードの数である (pipeline.SigmaEvaluation)。
	EvaluatedRuleCount   int64 `json:"evaluatedRuleCount"`
	EvaluatedRecordCount int64 `json:"evaluatedRecordCount"`
	// UnevaluatedRecordGroups は、どのルールも該当しえないレコードのチャネルかプロバイダごとの件数である。
	UnevaluatedRecordGroups []pipeline.SigmaUnevaluatedRecordGroup `json:"unevaluatedRecordGroups"`
	// SkippedPairCount は、ルールが参照する項目をレコードが名前付きの欄として持たないため
	// 当てなかったルールとレコードの組の数、SkippedPairRecordCount はその組を持つレコードの数である。
	SkippedPairCount       int64 `json:"skippedPairCount"`
	SkippedPairRecordCount int64 `json:"skippedPairRecordCount"`
	// RecordsWithoutSemantics は、意味付けに至らなかったためルールを当てなかったレコードの数である。
	RecordsWithoutSemantics int64 `json:"recordsWithoutSemantics"`
	// Rules は、条件を通った 1 件以上のレコードに一致したルールを path の文字列の順で並べる。
	Rules []sigmaMatchedRuleItem `json:"rules"`
	// Matches は、条件を通った一致したルールとレコードの組である。ルールごとの件数は Rules の
	// matchCount と等しい。
	Matches []sigmaRuleMatchItem `json:"matches"`
	// OutsideGraphMatchCount は、条件を与えた要求で、グラフに無いレコードのため除いた一致の数である。
	OutsideGraphMatchCount int64 `json:"outsideGraphMatchCount"`
	// UnevaluatedRules は評価しなかったルールの file を path の文字列の順で並べる。
	UnevaluatedRules []sigmaUnevaluatedRuleItem `json:"unevaluatedRules"`
}

// sigmaMatchedRuleItem は一致したルール 1 つである。path はルールの集合の中でルールを 1 つに
// 決める。id などの項目はルールの file の値であり、file に無い項目は空の文字列である。
type sigmaMatchedRuleItem struct {
	Path       string               `json:"path"`
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	Author     string               `json:"author"`
	Level      string               `json:"level"`
	Status     string               `json:"status"`
	Condition  string               `json:"condition"`
	Selections []sigmaSelectionItem `json:"selections"`
	MatchCount int64                `json:"matchCount"`
	// NodeIds と EdgeIds は、条件を通った一致のレコードのどれかを根拠に持つノードとエッジである。
	// ノードはエッジの端点を含む。並びはグラフの並び順である。
	NodeIds []string `json:"nodeIds"`
	EdgeIds []string `json:"edgeIds"`
}

// sigmaSelectionItem はルールの検索 1 つの名前と、file に書かれた定義の YAML である。
type sigmaSelectionItem struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

// sigmaRuleMatchItem はルール 1 つに一致したレコード 1 件である。
type sigmaRuleMatchItem struct {
	RulePath string             `json:"rulePath"`
	Record   core.RecordLocator `json:"record"`
	// MatchedSelections はレコードに一致した検索の名前を、ルールの file の順で並べる。
	MatchedSelections []string `json:"matchedSelections"`
	// Terminal はレコードが記録した端末の名前か、レコードが端末を記録しないときに評価の時点で
	// 収集元に割り当てた端末の名前である。後者のとき TerminalAssigned が真である。
	// どちらも無いときは出ない (pipeline.SigmaRuleMatch)。
	Terminal         string `json:"terminal,omitempty"`
	TerminalAssigned bool   `json:"terminalAssigned,omitempty"`
	// EventTime はレコードの時刻である。読めなかったときは出ない。
	EventTime *core.Timestamp `json:"eventTime,omitempty"`
	// RecordNode は一致したレコードのノードである。レコードがグラフに無いか、レコードのノードを
	// 持たないときは出ない。
	RecordNode *core.GraphNode `json:"recordNode,omitempty"`
}

// sigmaUnevaluatedRuleItem は評価しなかったルールの file 1 つである。reason は分類の
// 機械の値、detail は理由の文である。
type sigmaUnevaluatedRuleItem struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

func (h sigmaRuleCandidatesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseGraphRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownSearch(h.graph, request.search); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	evaluation := h.evaluation
	filtered := h.graph.SigmaMatchesIn(request.query(), evaluation)
	response := sigmaRuleCandidatesResponse{
		RuleSet:                 evaluation.RuleSet,
		EvaluatedRuleCount:      evaluation.EvaluatedRuleCount,
		EvaluatedRecordCount:    evaluation.EvaluatedRecordCount,
		UnevaluatedRecordGroups: emptyIfNil(evaluation.UnevaluatedRecordGroups),
		SkippedPairCount:        evaluation.SkippedPairCount,
		SkippedPairRecordCount:  evaluation.SkippedPairRecordCount,
		RecordsWithoutSemantics: evaluation.RecordsWithoutSemantics,
		Rules:                   make([]sigmaMatchedRuleItem, 0, len(filtered.Rules)),
		Matches:                 make([]sigmaRuleMatchItem, 0, len(filtered.Matches)),
		OutsideGraphMatchCount:  filtered.OutsideGraphMatchCount,
		UnevaluatedRules:        make([]sigmaUnevaluatedRuleItem, 0, len(evaluation.UnevaluatedRules)),
	}
	for _, rule := range filtered.Rules {
		selections := make([]sigmaSelectionItem, 0, len(rule.Selections))
		for _, sel := range rule.Selections {
			selections = append(selections, sigmaSelectionItem{Name: sel.Name, Definition: sel.Definition})
		}
		response.Rules = append(response.Rules, sigmaMatchedRuleItem{
			Path: rule.Path, ID: rule.ID, Title: rule.Title, Author: rule.Author, Level: rule.Level,
			Status: rule.Status, Condition: rule.Condition, Selections: selections, MatchCount: rule.MatchCount,
			NodeIds: rule.Graph.NodeIds, EdgeIds: rule.Graph.EdgeIds,
		})
	}
	for _, match := range filtered.Matches {
		response.Matches = append(response.Matches, sigmaRuleMatchItem{
			RulePath: match.RulePath, Record: match.Record, MatchedSelections: emptyIfNil(match.MatchedSelections),
			Terminal: match.Terminal, TerminalAssigned: match.TerminalAssigned, EventTime: match.EventTime,
			RecordNode: match.RecordNode,
		})
	}
	for _, skipped := range evaluation.UnevaluatedRules {
		response.UnevaluatedRules = append(response.UnevaluatedRules, sigmaUnevaluatedRuleItem(skipped))
	}
	writeJSON(w, http.StatusOK, response)
}
