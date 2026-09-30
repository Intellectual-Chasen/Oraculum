package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const recordNumbersPattern = "GET /api/v0/record-numbers"

// 収集元のレコードの番号の要求の項目の名前。比べる相手の収集元は省略できる。
const (
	comparedSourceIdParam            = "comparedSourceId"
	comparedSourceContentSha256Param = "comparedSourceContentSha256"
)

// recordNumbersHandler は収集元 1 件のレコードの番号の抜けと、要求が与えたときは別の収集元との
// 突き合わせを返す。時刻の鍵は、分析者が記録した時刻の解釈を当てた取り込み結果から読む。
type recordNumbersHandler struct {
	result pipeline.ImportResult
}

// recordNumbersResponse はレコードの番号の要求の応答である。**本型が項目の定義元である。**
type recordNumbersResponse struct {
	RecordNumbers core.RecordNumbers `json:"recordNumbers"`
	// Comparison は要求が comparedSourceId を与えたときだけ出る。
	Comparison *core.RecordComparison `json:"comparison,omitempty"`
}

func (h recordNumbersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	source, compared, apiError := parseRecordNumbersRequest(r.URL.Query())
	if apiError != nil {
		writeError(w, http.StatusBadRequest, *apiError)
		return
	}
	for _, requested := range []*requestedSource{&source, compared} {
		if requested == nil {
			continue
		}
		if _, apiError := checkRequestedSource(h.result, *requested); apiError != nil {
			writeError(w, httpStatusFor(apiError.Code), *apiError)
			return
		}
	}
	numbers, found := h.result.RecordNumbers(source.sourceId)
	response := recordNumbersResponse{RecordNumbers: numbers}
	if found && compared != nil {
		var comparison core.RecordComparison
		comparison, found = h.result.CompareRecords(source.sourceId, compared.sourceId)
		response.Comparison = &comparison
	}
	if !found {
		// checkRequestedSource が公開された収集元だけを通すため、内部不変条件の破れである。
		slog.Error("reading record numbers of a published source failed")
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "reading record numbers failed",
		})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// parseRecordNumbersRequest は要求の項目を読む。compared は相手の収集元を与えない要求で nil である。
func parseRecordNumbersRequest(query url.Values) (requestedSource, *requestedSource, *core.ApiError) {
	known := []string{
		sourceIdParam, sourceContentSha256Param, comparedSourceIdParam, comparedSourceContentSha256Param,
	}
	for name, values := range query {
		if !slices.Contains(known, name) {
			return requestedSource{}, nil, invalidRequestError(
				errors.New(name+" is not a query parameter of a record numbers request"), nil)
		}
		if len(values) != 1 || values[0] == "" {
			return requestedSource{}, nil, invalidRequestError(errors.New(name+" must be given once with a value"), nil)
		}
	}
	var missing []string
	for _, name := range known[:2] {
		if !query.Has(name) {
			missing = append(missing, name)
		}
	}
	if query.Has(comparedSourceIdParam) != query.Has(comparedSourceContentSha256Param) {
		for _, name := range known[2:] {
			if !query.Has(name) {
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		return requestedSource{}, nil, invalidRequestError(errors.New("required query parameters are missing"), missing)
	}
	source := requestedSource{sourceId: query.Get(sourceIdParam), contentSha256: query.Get(sourceContentSha256Param)}
	if !query.Has(comparedSourceIdParam) {
		return source, nil, nil
	}
	compared := &requestedSource{
		sourceId: query.Get(comparedSourceIdParam), contentSha256: query.Get(comparedSourceContentSha256Param),
	}
	if compared.sourceId == source.sourceId {
		return requestedSource{}, nil, invalidRequestError(
			errors.New("comparedSourceId must name a source other than sourceId"), nil)
	}
	return source, compared, nil
}
