package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// matchConditionParam は、候補を絞るのに用いる条件を持つ要求の項目である。
//
// **1 つの条件につき 1 回書く。** グラフを読む操作がすべて同じ項目を読む。
const matchConditionParam = "matchCondition"

// matchConditionToleranceSeparator は条件の種別と、認める幅を分ける字である。
const matchConditionToleranceSeparator = "~"

// readMatchConditions は、候補を絞るのに用いる条件の選択を読む。
//
// **既定の選択を持たない。** 項目を書かない要求を退ける。既定を当てると、条件を渡し
// 忘れた画面が「この条件で絞った」と読める応答を受け取る。
//
// **条件を 1 つ以上求める。** 条件を 1 つも比べない段階は、相手の側のレコードをそのまま
// 候補に並べ、根拠の無い組を候補として示すことになる。
func readMatchConditions(query url.Values) (pipeline.MatchConditionSelection, error) {
	given, present := query[matchConditionParam]
	if !present || len(given) == 0 {
		return pipeline.MatchConditionSelection{},
			errors.New(matchConditionParam + " is required")
	}
	conditions := make([]pipeline.SelectedMatchCondition, 0, len(given))
	for _, item := range given {
		condition, err := parseMatchCondition(item)
		if err != nil {
			return pipeline.MatchConditionSelection{}, err
		}
		conditions = append(conditions, condition)
	}
	selection := pipeline.MatchConditionSelection{Conditions: conditions}
	if err := selection.Validate(); err != nil {
		return pipeline.MatchConditionSelection{}, err
	}
	return selection, nil
}

// parseMatchCondition は要求の 1 件の文字列を、条件の種別と認める幅へ読む。
func parseMatchCondition(item string) (pipeline.SelectedMatchCondition, error) {
	key, toleranceText, hasTolerance := strings.Cut(item, matchConditionToleranceSeparator)
	condition := pipeline.SelectedMatchCondition{ConditionKey: core.ConditionKey(key)}
	if !condition.ConditionKey.IsKnown() {
		return pipeline.SelectedMatchCondition{}, errors.New(matchConditionParam +
			" must name a condition the contract defines")
	}
	if !hasTolerance {
		return condition, nil
	}
	tolerance, err := strconv.ParseInt(toleranceText, 10, 64)
	if err != nil || tolerance < 0 || tolerance > core.MaxSafeInteger {
		return pipeline.SelectedMatchCondition{}, errors.New(matchConditionParam +
			" carries a tolerance that must be a non-negative safe integer")
	}
	condition.Tolerance = tolerance
	return condition, nil
}

// matchSelectingHandler は、要求が与えた関連付けの条件に対応するグラフで応答する経路である。
//
// **条件を読むのはグラフを読む経路だけである。** 取り込み状況・端末の割当は関連付けの条件で
// 変わらない。原資料のレコードは、起点を与えて経路を求める要求だけが条件を読む。
type matchSelectingHandler struct {
	graphs graphSource
	build  func(pipeline.Graph, pipeline.ImportResult) http.Handler
}

func (h matchSelectingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	selection, err := readMatchConditions(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest,
			*invalidRequestError(err, []string{matchConditionParam}))
		return
	}
	graph, result, ok := graphOfSelection(w, r, h.graphs, selection)
	if !ok {
		return
	}
	h.build(graph, result).ServeHTTP(w, r)
}

// graphOfSelection は選択に対応するグラフを取る。ok が偽のときは、失敗の応答を書き終えたか、
// 要求の取り消しで応答を書かないことにした。
func graphOfSelection(
	w http.ResponseWriter, r *http.Request, graphs graphSource, selection pipeline.MatchConditionSelection,
) (pipeline.Graph, pipeline.ImportResult, bool) {
	graph, result, err := graphs(r.Context(), selection)
	if err == nil {
		return graph, result, true
	}
	if r.Context().Err() != nil {
		// 要求の取り消しで待つのをやめた。応答を受け取る相手がいないため、何も書かない。
		return pipeline.Graph{}, pipeline.ImportResult{}, false
	}
	// 組み立ての panic を catalog が error へ移したものである。err は原資料の byte 列を持たない。
	slog.Error("building the graph failed", "error", err)
	writeError(w, http.StatusInternalServerError, core.ApiError{
		Code: core.ApiErrorCodeInternalError, Message: "building the graph failed",
	})
	return pipeline.Graph{}, pipeline.ImportResult{}, false
}

// repeatedRequestItem は、1 つの要求に 2 回以上書いてよい項目かを返す。
//
// 関連付けの条件は 1 件につき 1 回、近傍を広げる起点のノードは 1 個につき 1 回、ノードの種別と
// 検索の文字列と収集元は 1 つにつき 1 回書くため、どれも項目が繰り返し現れる。
func repeatedRequestItem(name string) bool {
	switch name {
	case matchConditionParam, nodeKindParam,
		valueContainsParam, valueExcludesParam, nodeIdParam, sourceParam:
		return true
	default:
		return false
	}
}
