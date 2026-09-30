package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 複数の操作が共有する要求の項目の名前。sourceId と sourceContentSha256 は
// `/api/v0/records`、期間と比較の単位の項目は `/api/v0/graph` が読む。
const (
	sourceIdParam            = "sourceId"
	sourceContentSha256Param = "sourceContentSha256"
	timeFromParam            = "timeFrom"
	timeFromPrecisionParam   = "timeFromPrecision"
	timeToParam              = "timeTo"
	timeToPrecisionParam     = "timeToPrecision"
	filterUnitParam          = "filterUnit"
	// caseParam は根拠のレコードを収集元に付けた案件で絞る項目である。`/api/v0/graph`、
	// `/api/v0/edges/{id}`、時系列の操作が読む。
	caseParam = "case"
	// terminalParam は根拠のレコードを、レコードが名乗った端末のノードの識別子で絞る項目である。
	// `/api/v0/graph` と時系列の操作が読む。
	terminalParam = "terminal"
	// sourceParam は根拠のレコードを収集元の sourceId で絞る項目である。1 つの収集元につき
	// 1 回書く。
	sourceParam = "source"
	// eventActionFromParam と eventActionToParam は、事象の動作を 10 進の数として比べる範囲の
	// 両端である。`/api/v0/graph` と時系列の操作が読む。
	eventActionFromParam = "eventActionFrom"
	eventActionToParam   = "eventActionTo"
)

// recordConditionsRequest は、根拠のレコードを絞る条件と、条件から読んだ期間の両端である。
// 部分グラフの要求と時系列の要求が同じ条件を読む。
type recordConditionsRequest struct {
	conditions core.RecordConditions
	timeFrom   *core.RequestedBound
	timeTo     *core.RequestedBound
}

// newRecordConditionsRequest は検証を通った条件から期間の両端を読む。
func newRecordConditionsRequest(conditions core.RecordConditions) (recordConditionsRequest, error) {
	from, to, err := conditions.Period()
	if err != nil {
		return recordConditionsRequest{}, err
	}
	return recordConditionsRequest{conditions: conditions, timeFrom: from, timeTo: to}, nil
}

// readRecordConditions は根拠のレコードを絞る条件を query から読む。
//
// **query の文字列の形だけを確かめる。** 項目の組み合わせと値の範囲は core.RecordConditions の
// Validate が確かめる。与えたが空の項目は、項目を与えない要求と区別して退ける。
func readRecordConditions(query url.Values) (core.RecordConditions, error) {
	for _, name := range []string{
		terminalParam, caseParam, timeFromParam, timeFromPrecisionParam,
		timeToParam, timeToPrecisionParam, filterUnitParam,
	} {
		if _, given := query[name]; given && query.Get(name) == "" {
			return core.RecordConditions{}, errors.New(name + " must not be empty")
		}
	}
	conditions := core.RecordConditions{
		EventCategory:     query.Get(eventCategoryParam),
		EventAction:       query.Get(eventActionParam),
		TimeFrom:          query.Get(timeFromParam),
		TimeFromPrecision: core.Precision(query.Get(timeFromPrecisionParam)),
		TimeTo:            query.Get(timeToParam),
		TimeToPrecision:   core.Precision(query.Get(timeToPrecisionParam)),
		FilterUnit:        core.FilterUnit(query.Get(filterUnitParam)),
		Case:              query.Get(caseParam),
		Terminal:          query.Get(terminalParam),
		Sources:           query[sourceParam],
	}
	for _, bound := range []struct {
		name   string
		target **uint64
	}{{eventActionFromParam, &conditions.EventActionFrom}, {eventActionToParam, &conditions.EventActionTo}} {
		if _, given := query[bound.name]; !given {
			continue
		}
		// 値を message に載せない。
		number, err := strconv.ParseUint(query.Get(bound.name), 10, 64)
		if err != nil {
			return core.RecordConditions{}, errors.New(bound.name + " must be a decimal number up to 2^53-1")
		}
		*bound.target = &number
	}
	return conditions, nil
}

// readRecordConditionsRequest は根拠のレコードを絞る条件を query から読んで検証し、期間の両端を
// 読んだ条件を返す。
func readRecordConditionsRequest(query url.Values) (recordConditionsRequest, error) {
	conditions, err := readRecordConditions(query)
	if err != nil {
		return recordConditionsRequest{}, err
	}
	if err := conditions.Validate(); err != nil {
		return recordConditionsRequest{}, err
	}
	return newRecordConditionsRequest(conditions)
}

// readRequestedCase は案件の項目を読む。項目の無い要求では空の文字列を返す。
// 取り込み結果にその案件があるかは、取り込み結果を持つ handler が確かめる (checkKnownCase)。
func readRequestedCase(query url.Values) (string, error) {
	if _, given := query[caseParam]; !given {
		return "", nil
	}
	caseId := query.Get(caseParam)
	if err := core.ValidateCaseId(caseParam, caseId); err != nil {
		return "", fmt.Errorf("reading the requested case: %w", err)
	}
	return caseId, nil
}

// checkKnownSearch は、検索の条件が指した案件、端末、収集元、起点のノードがグラフにあることを
// 確かめる。
//
// 要求が指したノードの有無は、取り込み結果を読まないと決まらない。判定の順序は
// record_not_found を invalid_request より後に置く。
func checkKnownSearch(graph pipeline.Graph, search core.SearchQuery) *core.ApiError {
	if apiError := checkKnownRecordConditions(graph, search.RecordConditions); apiError != nil {
		return apiError
	}
	return checkKnownNodes(graph, search.NodeIds)
}

// checkKnownRecordConditions は、根拠のレコードを絞る条件が指した案件、端末、収集元がグラフに
// あることを確かめる。
func checkKnownRecordConditions(graph pipeline.Graph, conditions core.RecordConditions) *core.ApiError {
	if apiError := checkKnownCase(graph, conditions.Case); apiError != nil {
		return apiError
	}
	if apiError := checkKnownTerminal(graph, conditions.Terminal); apiError != nil {
		return apiError
	}
	return checkKnownSources(graph, conditions.Sources)
}

// checkKnownCase は、要求が指した案件を取り込み結果が持つことを確かめる。
// 案件の項目の無い要求では nil を返す。
//
// **知らない案件を空の結果で返さない。** 空の結果は「その案件に該当するレコードが無い」と
// 読め、案件の文字列の誤りと区別できない。
func checkKnownCase(graph pipeline.Graph, caseId string) *core.ApiError {
	if caseId == "" || graph.HasCase(caseId) {
		return nil
	}
	return invalidRequestError(errors.New("no imported source carries the requested case"), nil)
}

// checkKnownTerminal は、要求が指した端末がグラフの端末のノードであることを確かめる。
// 端末の項目の無い要求では nil を返す。
//
// **知らない端末を空の結果で返さない。** checkKnownCase と同じ理由である。
func checkKnownTerminal(graph pipeline.Graph, terminal string) *core.ApiError {
	if terminal == "" || graph.IsTerminalNode(terminal) {
		return nil
	}
	return invalidRequestError(errors.New("no terminal node matches the requested terminal"), nil)
}

// checkKnownSources は、要求が指した収集元をすべて取り込み結果が持つことを確かめる。
// 収集元の項目の無い要求では nil を返す。
//
// **知らない収集元を空の結果で返さない。** checkKnownCase と同じ理由である。
func checkKnownSources(graph pipeline.Graph, sources []string) *core.ApiError {
	for _, sourceId := range sources {
		if !graph.HasSource(sourceId) {
			return invalidRequestError(errors.New("no imported source matches the requested source"), nil)
		}
	}
	return nil
}

// checkKnownNodes は、要求が起点に指したノードがすべてグラフにあることを確かめる。
// 1 つでも無いときは record_not_found を返す。起点の項目の無い要求では nil を返す。
//
// **識別子を message に載せない。** 載せるのは項目の名前だけである。
func checkKnownNodes(graph pipeline.Graph, nodeIds []string) *core.ApiError {
	return checkKnownNodesForParameter(graph, nodeIds, "nodeId")
}

// checkKnownNodesForParameter は、要求の指定した項目が指すノードがすべてグラフにあることを確かめる。
// 識別子は応答に載せず、見つからないときは項目名だけを返す。
func checkKnownNodesForParameter(
	graph pipeline.Graph, nodeIds []string, parameter string,
) *core.ApiError {
	for _, id := range nodeIds {
		if !graph.HasNode(id) {
			return &core.ApiError{
				Code:    core.ApiErrorCodeRecordNotFound,
				Message: fmt.Sprintf("no node matches the requested %s", parameter),
			}
		}
	}
	return nil
}

// recordFilter は条件を pipeline の絞り込みの条件へ直す。
func (r recordConditionsRequest) recordFilter() pipeline.RecordFilter {
	filter := pipeline.RecordFilter{
		EventCategory:   r.conditions.EventCategory,
		EventAction:     r.conditions.EventAction,
		EventActionFrom: r.conditions.EventActionFrom,
		EventActionTo:   r.conditions.EventActionTo,
		TimeUnit:        r.conditions.FilterUnit.Duration(),
		Case:            r.conditions.Case,
		Terminal:        r.conditions.Terminal,
		Sources:         r.conditions.Sources,
	}
	if r.timeFrom != nil {
		filter.TimeFrom = &r.timeFrom.Instant
	}
	if r.timeTo != nil {
		filter.TimeTo = &r.timeTo.Instant
	}
	filter.Validate()
	return filter
}

// requestedTimes は期間の両端のうち、応答へそのまま返す組を返す。与えていない端は nil である。
func (r recordConditionsRequest) requestedTimes() (from, to *core.RequestedTime) {
	if r.timeFrom != nil {
		from = &r.timeFrom.Requested
	}
	if r.timeTo != nil {
		to = &r.timeTo.Requested
	}
	return from, to
}

// requestedSource は要求が指した収集元 1 件である。
type requestedSource struct {
	sourceId      string
	contentSha256 string
}

func invalidRequestError(err error, missing []string) *core.ApiError {
	return &core.ApiError{
		Code:              core.ApiErrorCodeInvalidRequest,
		Message:           err.Error(),
		MissingParameters: missing,
	}
}
