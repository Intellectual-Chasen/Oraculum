package api

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// `GET /api/v0/graph` の要求の項目の名前。
//
// eventCategoryParam と eventActionParam と evidenceLimitParam は `/api/v0/nodes/{id}` と `/api/v0/edges/{id}` も
// 読む。名前を 1 か所で決めるためここに置く。
const (
	nodeKindParam         = "nodeKind"
	nodeIdParam           = "nodeId"
	depthParam            = "depth"
	edgeKindParam         = "edgeKind"
	eventCategoryParam    = "eventCategory"
	eventActionParam      = "eventAction"
	addressInCidrParam    = "addressInCidr"
	addressNotInCidrParam = "addressNotInCidr"
	valueContainsParam    = "valueContains"
	valueExcludesParam    = "valueExcludes"
	granularityParam      = "granularity"
	nodeLimitParam        = "nodeLimit"
	countByParam          = "countBy"
	evidenceLimitParam    = "evidenceLimit"
	// valueFieldParam は、valueContains と valueExcludes の文字列を当てる欄を 1 つに限る項目である。
	valueFieldParam = "valueField"
	// fieldContainsParam は、欄と文字列の組 1 つを `欄=文字列` で書く項目である。組ごとに繰り返す。
	fieldContainsParam = "fieldContains"
	// fieldEqualsParam は、値の全体が文字列と等しい欄だけを一致とする欄と文字列の組 1 つを
	// `欄=文字列` で書く項目である。組ごとに繰り返す (pipeline.FieldTerm.WholeValue)。
	fieldEqualsParam = "fieldEquals"
	// conditionsOnOriginsOnlyParam は、文字列と事象の種別の条件を起点の判定だけに当てる項目である。
	conditionsOnOriginsOnlyParam = "conditionsOnOriginsOnly"
	// recordSummaryParam は、合致したレコードのノードに位置と時刻と事象の種別を添える項目である。
	recordSummaryParam = "recordSummary"
	// endpointRecordsInPeriodParam は、端点のレコードが期間の外にあるエッジを辿らない項目である。
	endpointRecordsInPeriodParam = "endpointRecordsInPeriod"
	// flagValue は、真か偽の項目が取る唯一の文字列である。
	flagValue = "true"
)

// graphRequest は`/api/v0/graph` の要求の項目である。
type graphRequest struct {
	// search は検索の条件である。検証は core.SearchQuery の Validate が行う。
	search core.SearchQuery
	// nodeLimit は部分グラフのノードの数の上限である。0 は上限を与えなかったことを表す。
	nodeLimit int
	// records は根拠のレコードを絞る条件と、条件から読んだ期間の両端である。
	records recordConditionsRequest
	// addressIn と addressNotIn は検索の条件から読んだアドレスの範囲である。
	addressIn    *netip.Prefix
	addressNotIn *netip.Prefix
	// textSearch は値・欄・検索式に対する文字列条件である。
	textSearch pipeline.GraphQuery
	// recordSummary は、合致したレコードのノードに位置と時刻と事象の種別を添えるかである。
	recordSummary bool
}

// query は要求を pipeline の部分グラフの要求へ直す。
func (r graphRequest) query() pipeline.GraphQuery {
	built := pipeline.GraphQuery{
		NodeLimit: r.nodeLimit,
		NodeKinds: r.search.NodeKinds, Granularity: r.search.Granularity,
		NodeIds: r.search.NodeIds, Depth: r.search.Depth, EdgeKinds: r.search.EdgeKinds,
		AddressInPrefix: r.addressIn, AddressNotInPrefix: r.addressNotIn,
		RecordFilter:            r.records.recordFilter(),
		ConditionsOnOriginsOnly: r.search.ConditionsOnOriginsOnly,
		EndpointRecordsInPeriod: r.search.EndpointRecordsInPeriod,
		RecordSummary:           r.recordSummary,
	}
	built.ValueContains = r.textSearch.ValueContains
	built.ValueExcludes = r.textSearch.ValueExcludes
	built.ValueFieldSemantic = r.textSearch.ValueFieldSemantic
	built.ValueFieldName = r.textSearch.ValueFieldName
	built.FieldContains = r.textSearch.FieldContains
	built.Expression = r.textSearch.Expression
	if r.search.CountBy != "" {
		built.CountBySemantic, built.CountByName = pipeline.DesignatedField(r.search.CountBy)
	}
	return built
}

// parseGraphRequest は`/api/v0/graph` の要求を読む。
//
// **`code` の判定は本関数に書いた上からの順で行う。** query の文字列の形、検索の条件の検証
// (core.SearchQuery の Validate)、ノードの数の上限の順である。要求の値だけで決まる判定をすべて
// ここで行い、内部状態を要する判定 (nodeId に一致するノードの有無) を呼び出し元に残す。
func parseGraphRequest(r *http.Request) (graphRequest, *core.ApiError) {
	query := r.URL.Query()
	if err := checkGraphParameterNames(query); err != nil {
		return graphRequest{}, invalidRequestError(err, nil)
	}
	if missing := missingGraphParameters(query); len(missing) > 0 {
		return graphRequest{}, invalidRequestError(
			errors.New("required query parameters are missing"), missing)
	}
	if err := checkGraphEmptyValues(query); err != nil {
		return graphRequest{}, invalidRequestError(err, nil)
	}
	// 空の検索式も式として読み、誤りの範囲を付けて退ける。検索の条件は空の文字列を「条件を
	// 与えない」と読むため、query で与えた空の式を検索の条件へ渡す前に分ける。
	if query.Get(searchExpressionParam) == "" {
		var empty searchExpressionRequest
		if apiError := empty.readSearchExpression(query); apiError != nil {
			return graphRequest{}, apiError
		}
	}
	flags, err := readGraphFlags(query)
	if err != nil {
		return graphRequest{}, invalidRequestError(err, nil)
	}
	search, err := readSearchQuery(query)
	if err != nil {
		return graphRequest{}, invalidRequestError(err, nil)
	}
	search.ConditionsOnOriginsOnly = flags[conditionsOnOriginsOnlyParam]
	search.EndpointRecordsInPeriod = flags[endpointRecordsInPeriodParam]
	request, err := newGraphRequest(search)
	if err != nil {
		return graphRequest{}, searchQueryError(err)
	}
	request.recordSummary = flags[recordSummaryParam]
	if err := readGraphNodeLimit(query, &request); err != nil {
		return graphRequest{}, invalidRequestError(err, nil)
	}
	return request, nil
}

// newGraphRequest は検索の条件を検証し、検索の条件から読む値を持つ要求を組む。
func newGraphRequest(search core.SearchQuery) (graphRequest, error) {
	if err := search.Validate(); err != nil {
		return graphRequest{}, err
	}
	records, err := newRecordConditionsRequest(search.RecordConditions)
	if err != nil {
		return graphRequest{}, err
	}
	in, notIn, err := search.AddressPrefixes()
	if err != nil {
		return graphRequest{}, err
	}
	var expression *pipeline.SearchExpression
	if search.SearchExpression != "" {
		parsed, err := pipeline.ParseSearchExpression(search.SearchExpression)
		if err != nil {
			return graphRequest{}, err
		}
		expression = parsed
	}
	request := graphRequest{
		search: search, records: records, addressIn: in, addressNotIn: notIn,
		textSearch: textSearchOf(search, expression),
	}
	return request, nil
}

// textSearchOf は、検証済みの検索条件から値・欄の文字列条件を pipeline の要求へ移す。
func textSearchOf(search core.SearchQuery, expression *pipeline.SearchExpression) pipeline.GraphQuery {
	textSearch := pipeline.GraphQuery{
		ValueContains: search.ValueContains, ValueExcludes: search.ValueExcludes,
		Expression: expression,
	}
	if search.ValueField != "" {
		textSearch.ValueFieldSemantic, textSearch.ValueFieldName = pipeline.DesignatedField(search.ValueField)
	}
	for _, pairs := range []struct {
		values     []string
		wholeValue bool
	}{
		{search.FieldContains, false},
		{search.FieldEquals, true},
	} {
		for _, pair := range pairs.values {
			// 組の形は Validate が確かめた。最初の `=` で欄と文字列を分ける。
			field, contains, _ := strings.Cut(pair, "=")
			semantic, name := pipeline.DesignatedField(field)
			textSearch.FieldContains = append(textSearch.FieldContains, pipeline.FieldTerm{
				Semantic: semantic, Name: name, Contains: contains, WholeValue: pairs.wholeValue,
			})
		}
	}
	return textSearch
}

// readTextSearchQuery は、値・欄に対する文字列条件を query から読む。
func readTextSearchQuery(query url.Values) core.SearchQuery {
	return core.SearchQuery{
		ValueContains: query[valueContainsParam], ValueExcludes: query[valueExcludesParam],
		ValueField:    query.Get(valueFieldParam),
		FieldContains: query[fieldContainsParam], FieldEquals: query[fieldEqualsParam],
	}
}

// readGraphFlags は、唯一の文字列 true だけを取る項目を読む。与えた項目の名前に真を持つ。
func readGraphFlags(query url.Values) (map[string]bool, error) {
	flags := map[string]bool{}
	for _, name := range []string{conditionsOnOriginsOnlyParam, recordSummaryParam, endpointRecordsInPeriodParam} {
		if _, given := query[name]; !given {
			continue
		}
		if query.Get(name) != flagValue {
			return nil, errors.New(name + " takes " + flagValue)
		}
		flags[name] = true
	}
	return flags, nil
}

// readSearchQuery は検索の条件を query から読む。文字列の形だけを確かめ、項目の組み合わせと値の
// 範囲は core.SearchQuery の Validate に残す。
func readSearchQuery(query url.Values) (core.SearchQuery, error) {
	records, err := readRecordConditions(query)
	if err != nil {
		return core.SearchQuery{}, err
	}
	search := readTextSearchQuery(query)
	search.Granularity = core.GraphGranularity(query.Get(granularityParam))
	search.NodeIds = query[nodeIdParam]
	search.SearchExpression = query.Get(searchExpressionParam)
	search.CountBy = query.Get(countByParam)
	search.AddressInCidr = query.Get(addressInCidrParam)
	search.AddressNotInCidr = query.Get(addressNotInCidrParam)
	search.RecordConditions = records
	for _, kind := range query[nodeKindParam] {
		search.NodeKinds = append(search.NodeKinds, core.NodeKind(kind))
	}
	for _, kind := range query[edgeKindParam] {
		search.EdgeKinds = append(search.EdgeKinds, core.EdgeKind(kind))
	}
	depth, err := strconv.Atoi(query.Get(depthParam))
	if err != nil {
		return core.SearchQuery{}, fmt.Errorf("depth must be between %d and %d",
			core.MinSearchDepth, core.MaxSearchDepth)
	}
	search.Depth = depth
	return search, nil
}

// checkGraphParameterNames は要求の項目の名前と多重度を確かめる。
//
// 綴り誤りを通知せずに既定値で処理する応答を避けるため、未知の項目を退ける。
func checkGraphParameterNames(query url.Values) error {
	known := map[string]struct{}{
		nodeKindParam: {}, nodeIdParam: {},
		depthParam:    {},
		edgeKindParam: {}, eventCategoryParam: {}, eventActionParam: {},
		eventActionFromParam: {}, eventActionToParam: {}, valueFieldParam: {},
		addressInCidrParam: {}, addressNotInCidrParam: {}, valueContainsParam: {},
		valueExcludesParam: {}, granularityParam: {}, nodeLimitParam: {},
		countByParam: {}, timeFromParam: {},
		timeFromPrecisionParam: {}, timeToParam: {}, timeToPrecisionParam: {},
		filterUnitParam: {}, matchConditionParam: {}, caseParam: {}, terminalParam: {},
		conditionsOnOriginsOnlyParam: {}, sourceParam: {}, recordSummaryParam: {},
		fieldContainsParam: {}, fieldEqualsParam: {}, searchExpressionParam: {},
		endpointRecordsInPeriodParam: {},
	}
	if err := unsupportedParameterError(query, known); err != nil {
		return err
	}
	for name, values := range query {
		// edgeKind は本操作だけが繰り返しを受け付ける。関係の相手側の欄の要求は同じ名前を
		// 1 件で読むため、共有の repeatedRequestItem に入れない。
		if len(values) != 1 && !repeatedRequestItem(name) && name != edgeKindParam &&
			name != fieldContainsParam && name != fieldEqualsParam {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}

// unsupportedParameterError は、known に無い要求の項目の名前と、受け付ける項目の名前を
// 並べた誤りを返す。すべての項目を受け付けるときは nil を返す。
//
// 名前は %q で書き、利用者が入れた制御文字を文に載せない。
func unsupportedParameterError(query url.Values, known map[string]struct{}) error {
	var rejected []string
	for name := range query {
		if _, ok := known[name]; !ok {
			rejected = append(rejected, fmt.Sprintf("%q", name))
		}
	}
	if rejected == nil {
		return nil
	}
	slices.Sort(rejected)
	accepted := slices.Sorted(maps.Keys(known))
	return fmt.Errorf("unsupported query parameter: %s; accepted parameters: %s; %s",
		strings.Join(rejected, ", "), strings.Join(accepted, ", "), observedFieldsHint)
}

// missingGraphParameters は欠けている必須の項目を、本関数が並べた項目の順で返す。
func missingGraphParameters(query url.Values) []string {
	var missing []string
	if _, given := query[depthParam]; !given {
		missing = append(missing, depthParam)
	}
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
	return missing
}

// checkGraphEmptyValues は、与えたが空の項目を退ける。検索の条件は空の文字列を「条件を与えない」と
// 読むため、query で与えた空の項目を検索の条件へ渡す前に分ける。
func checkGraphEmptyValues(query url.Values) error {
	for _, name := range []string{
		granularityParam,
		eventCategoryParam, eventActionParam,
		addressInCidrParam, addressNotInCidrParam, countByParam,
	} {
		if _, given := query[name]; given && query.Get(name) == "" {
			return errors.New(name + " must not be empty")
		}
	}
	return checkTextSearchEmptyValues(query)
}

// checkTextSearchEmptyValues は文字列を伴わない欄指定と空の欄名を退ける。
func checkTextSearchEmptyValues(query url.Values) error {
	if _, given := query[valueFieldParam]; given && query.Get(valueFieldParam) == "" {
		return errors.New(valueFieldParam + " must name a field and come with a search term")
	}
	return nil
}

// parseDepth は段数の項目を読み、core.MinSearchDepth から core.MaxSearchDepth の範囲にあることを
// 確かめる。時系列の起点からの段数が使う。
func parseDepth(query url.Values) (int, error) {
	depth, err := strconv.Atoi(query.Get(depthParam))
	if err != nil || depth < core.MinSearchDepth || depth > core.MaxSearchDepth {
		return 0, fmt.Errorf("depth must be between %d and %d", core.MinSearchDepth, core.MaxSearchDepth)
	}
	return depth, nil
}

// readGraphNodeLimit はノードの数の上限を読む。項目が無い要求は上限を置かない。
func readGraphNodeLimit(query url.Values, request *graphRequest) error {
	if _, given := query[nodeLimitParam]; !given {
		return nil
	}
	limit, err := strconv.Atoi(query.Get(nodeLimitParam))
	if err != nil || limit < 1 {
		return errors.New(nodeLimitParam + " must be a positive integer")
	}
	request.nodeLimit = limit
	return nil
}
