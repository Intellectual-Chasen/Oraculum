package api

import (
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// timelineRequest は時系列の操作の要求の項目である。
//
// **上限も続きを取る位置も持たない。** limit と cursor は未知の項目として退ける。
type timelineRequest struct {
	records recordConditionsRequest
	searchExpressionRequest
	// textSearch は値・欄の文字列条件である。近傍探索の経路判定には使用しない。
	textSearch pipeline.GraphQuery
	// nodeIds は起点のノードの識別子である。空のときはノードで絞らない。
	nodeIds []string
	// depth は起点から辿る段数である。nodeIds を与えた要求だけが持つ。
	depth         int
	accountNodeId string
	// find は原文から探す文字列である。空のときは探さない。
	find string
	// findCaseSensitive は、find を大文字と小文字を区別して探すかである。
	findCaseSensitive bool
}

const (
	accountNodeIdParam     = "accountNodeId"
	findParam              = "find"
	findCaseSensitiveParam = "findCaseSensitive"
)

// query は要求を pipeline の時系列の要求へ直す。
func (r timelineRequest) query() pipeline.TimelineQuery {
	return pipeline.TimelineQuery{
		RecordFilter: r.records.recordFilter(), TextSearch: r.textSearch,
		NodeIds: r.nodeIds, Depth: r.depth,
		AccountNodeId: r.accountNodeId,
	}
}

// parseTimelineRequest は時系列の操作の要求を読む。
func parseTimelineRequest(r *http.Request) (timelineRequest, *core.ApiError) {
	query := r.URL.Query()
	if err := checkTimelineParameterNames(query); err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	if missing := missingTimelineParameters(query); len(missing) > 0 {
		return timelineRequest{}, invalidRequestError(
			errors.New("required query parameters are missing"), missing)
	}
	if err := checkTimelineEmptyValues(query); err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	conditions, err := readRecordConditions(query)
	if err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	request, err := newTimelineRequest(conditions)
	if err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	if apiError := request.readSearchExpression(query); apiError != nil {
		return timelineRequest{}, apiError
	}
	textSearch := readTextSearchQuery(query)
	if err := textSearch.Validate(); err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	request.textSearch = textSearchOf(textSearch, request.searchExpression)
	if err := readTimelineOrigins(query, &request); err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	request.accountNodeId = query.Get(accountNodeIdParam)
	if _, given := query[accountNodeIdParam]; given && request.accountNodeId == "" {
		return timelineRequest{}, invalidRequestError(errors.New(accountNodeIdParam+" must not be empty"), nil)
	}
	if request.accountNodeId != "" && len(request.nodeIds) != 0 {
		return timelineRequest{}, invalidRequestError(errors.New(accountNodeIdParam+" and "+nodeIdParam+" cannot be combined"), nil)
	}
	if err := readTimelineFind(query, &request); err != nil {
		return timelineRequest{}, invalidRequestError(err, nil)
	}
	return request, nil
}

// newTimelineRequest は根拠のレコードを絞る条件を検証し、時系列の要求を組む。
func newTimelineRequest(conditions core.RecordConditions) (timelineRequest, error) {
	if err := conditions.Validate(); err != nil {
		return timelineRequest{}, err
	}
	records, err := newRecordConditionsRequest(conditions)
	if err != nil {
		return timelineRequest{}, err
	}
	return timelineRequest{records: records}, nil
}

// readTimelineFind は原文から探す文字列と、大文字と小文字を区別するかを読む。区別の項目は
// 文字列を与えた要求だけが持ち、値は true か false である。
func readTimelineFind(query url.Values, request *timelineRequest) error {
	_, givenFind := query[findParam]
	request.find = query.Get(findParam)
	if givenFind && request.find == "" {
		return errors.New(findParam + " must not be empty")
	}
	sensitive, givenCase := query[findCaseSensitiveParam]
	if !givenCase {
		return nil
	}
	if !givenFind {
		return errors.New(findCaseSensitiveParam + " requires " + findParam)
	}
	switch sensitive[0] {
	case "true":
		request.findCaseSensitive = true
	case "false":
	default:
		return errors.New(findCaseSensitiveParam + " must be true or false")
	}
	return nil
}

// readTimelineOrigins は起点のノードと段数を読む。段数は起点を与えた要求だけが持つ。
func readTimelineOrigins(query url.Values, request *timelineRequest) error {
	request.nodeIds = query[nodeIdParam]
	if slices.Contains(request.nodeIds, "") {
		return errors.New(nodeIdParam + " must not be empty")
	}
	if _, given := query[depthParam]; !given {
		return nil
	}
	if len(request.nodeIds) == 0 {
		return errors.New(depthParam + " requires " + nodeIdParam)
	}
	depth, err := parseDepth(query)
	request.depth = depth
	return err
}

// checkTimelineParameterNames は要求の項目の名前と多重度を確かめる。
//
// 綴り誤りを通知せずに既定値で処理する応答を避けるため、未知の項目を退ける。limit と
// cursor もここで退ける。
func checkTimelineParameterNames(query url.Values) error {
	known := map[string]struct{}{
		eventCategoryParam: {}, eventActionParam: {},
		eventActionFromParam: {}, eventActionToParam: {},
		timeFromParam: {}, timeFromPrecisionParam: {},
		timeToParam: {}, timeToPrecisionParam: {},
		filterUnitParam: {}, matchConditionParam: {}, caseParam: {}, terminalParam: {}, sourceParam: {},
		nodeIdParam: {}, depthParam: {}, accountNodeIdParam: {}, findParam: {}, findCaseSensitiveParam: {},
		searchExpressionParam: {},
		valueContainsParam:    {}, valueExcludesParam: {}, valueFieldParam: {},
		fieldContainsParam: {}, fieldEqualsParam: {},
	}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			return errors.New("unsupported query parameter")
		}
		if len(values) != 1 && !repeatedRequestItem(name) &&
			name != fieldContainsParam && name != fieldEqualsParam {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}

// missingTimelineParameters は欠けている必須の項目を、本関数が並べた項目の順で返す。
func missingTimelineParameters(query url.Values) []string {
	var missing []string
	_, hasTimeFrom := query[timeFromParam]
	_, hasTimeTo := query[timeToParam]
	for _, pair := range []struct{ text, precision string }{
		{timeFromParam, timeFromPrecisionParam}, {timeToParam, timeToPrecisionParam},
	} {
		if _, hasText := query[pair.text]; hasText {
			if _, hasPrecision := query[pair.precision]; !hasPrecision {
				missing = append(missing, pair.precision)
			}
		}
	}
	if _, hasUnit := query[filterUnitParam]; (hasTimeFrom || hasTimeTo) && !hasUnit {
		missing = append(missing, filterUnitParam)
	}
	_, hasNode := query[nodeIdParam]
	if _, hasDepth := query[depthParam]; hasNode && !hasDepth {
		missing = append(missing, depthParam)
	}
	return missing
}

// checkTimelineEmptyValues は、値を空にできない項目を確かめる。
func checkTimelineEmptyValues(query url.Values) error {
	for _, name := range []string{eventCategoryParam, eventActionParam} {
		if _, given := query[name]; given && query.Get(name) == "" {
			return errors.New(name + " must not be empty")
		}
	}
	return checkTextSearchEmptyValues(query)
}
