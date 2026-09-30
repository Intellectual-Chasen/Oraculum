package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const timeHistogramPattern = "GET /api/v0/time-histogram"

// columnsParam は区切りの数である。
const columnsParam = "columns"

// maxHistogramColumns は区切りの数の上限である。
const maxHistogramColumns = 1000

// millisecondLayout は、最初の区切りの始まりの時刻を書く書式である。
const millisecondLayout = "2006-01-02T15:04:05.000Z07:00"

// timeHistogramHandler は、絞り込みを通ったレコードを時刻の区切りと端末で数えて返す。
type timeHistogramHandler struct {
	graph pipeline.Graph
}

// timeHistogramRow は timeHistogramResponse の 1 行である。
type timeHistogramRow struct {
	// Terminal は行の端末である。端末を特定できないレコードの行では出ない。
	Terminal *core.GraphNode `json:"terminal,omitempty"`
	// Counts は区切りごとの件数であり、要素の数は columns と同じである。
	Counts []int `json:"counts"`
}

// timeHistogramResponse は件数の分布の応答である。**本型が項目の定義元である。**
type timeHistogramResponse struct {
	// Start は最初の区切りの始まりの UTC の時刻である。ミリ秒の精度の RFC 3339 で書く。
	// 時刻を比べられるレコードが無いときは出ない。
	Start string `json:"start,omitempty"`
	// StepMs は 1 つの区切りの幅のミリ秒である。1 秒以上の幅は秒の単位に切り上げる。
	// 時刻を比べられるレコードが無いときは 0 である。
	StepMs int64 `json:"stepMs"`
	// Rows は端末ごとの行であり、件数の合計の降順に並ぶ。
	Rows []timeHistogramRow `json:"rows"`
	// LocalTimeRecordCount は UTC からのずれの決まらない地方時のレコードの件数、UndatedRecordCount は
	// 時刻を持たないレコードの件数である。どちらも時点を持たず、区切りに入らない。
	LocalTimeRecordCount int `json:"localTimeRecordCount"`
	UndatedRecordCount   int `json:"undatedRecordCount"`
	// SpanningRecordCount は、時点を持つが、時刻の精度の範囲が 1 つの区切りに収まらず、区切りに
	// 入れなかったレコードの件数である。
	SpanningRecordCount int `json:"spanningRecordCount"`
}

func (h timeHistogramHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	request, apiError := parseSummaryFilter(query, columnsParam)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	columns, err := readColumns(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if apiError := checkKnownRecordConditions(h.graph, request.records.conditions); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	histogram := h.graph.TimeHistogram(request.records.recordFilter(), request.searchExpression, columns)
	response := timeHistogramResponse{
		StepMs:               histogram.Step.Milliseconds(),
		Rows:                 make([]timeHistogramRow, 0, len(histogram.Rows)),
		LocalTimeRecordCount: histogram.LocalTimeRecordCount,
		UndatedRecordCount:   histogram.UndatedRecordCount,
		SpanningRecordCount:  histogram.SpanningRecordCount,
	}
	// 区切りを決めたのは時点を持つレコードがあったときである。そのレコードがどれも区切りに
	// 収まらず行が無いときも、区切りの始まりと幅を返す。
	if histogram.Step > 0 {
		response.Start = histogram.Start.UTC().Format(millisecondLayout)
	}
	for _, row := range histogram.Rows {
		response.Rows = append(response.Rows, timeHistogramRow{Terminal: row.Terminal, Counts: row.Counts})
	}
	writeJSON(w, http.StatusOK, response)
}

// readColumns は区切りの数を読む。1 回だけ、1 以上 maxHistogramColumns 以下の 10 進で与える。
func readColumns(query url.Values) (int, error) {
	columns, err := strconv.Atoi(query.Get(columnsParam))
	if len(query[columnsParam]) != 1 || err != nil || columns < 1 || columns > maxHistogramColumns {
		return 0, errors.New(columnsParam + " must be one decimal number from 1 to 1000")
	}
	return columns, nil
}
