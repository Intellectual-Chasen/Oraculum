package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

type timeHistogramBody struct {
	Start  string `json:"start"`
	StepMs int64  `json:"stepMs"`
	Rows   []struct {
		Terminal *core.GraphNode `json:"terminal"`
		Counts   []int           `json:"counts"`
	} `json:"rows"`
	LocalTimeRecordCount int `json:"localTimeRecordCount"`
	UndatedRecordCount   int `json:"undatedRecordCount"`
	SpanningRecordCount  int `json:"spanningRecordCount"`
}

func requestTimeHistogram(handler http.Handler, query string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions("/api/v0/time-histogram?"+query), nil))
	return response
}

// 分布は時系列と同じレコードを数える。区切りに入れた件数と、精度で区切りに収まらない件数と、
// 地方時の件数と時刻の無い件数の和は、同じ絞り込みの時系列の行と時刻を読めない件数の和に一致する。
// 時刻の無い件数は、時系列の時刻を読めない件数と同じである。
func TestTimeHistogramCountsTheRecordsOfTheTimeline(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	chosen := whole.Entries[0].RecordRef.SourceId
	for _, filter := range []string{"", "source=" + url.QueryEscape(chosen)} {
		response := requestTimeHistogram(handler, "columns=60&"+filter)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var body timeHistogramBody
		decodeJSON(t, response.Body, &body)
		timeline := decodeTimeline(t, handler, filter)
		placed := 0
		for _, row := range body.Rows {
			if len(row.Counts) != 60 {
				t.Fatalf("a row carries %d columns, want 60", len(row.Counts))
			}
			for _, count := range row.Counts {
				placed += count
			}
		}
		outside := body.SpanningRecordCount + body.LocalTimeRecordCount + body.UndatedRecordCount
		if int64(placed+outside) != timeline.EntryCount+timeline.UndatedRecordCount ||
			int64(body.UndatedRecordCount) != timeline.UndatedRecordCount {
			t.Errorf("%q: the histogram counts %d+%d (undated %d), the timeline has %d+%d", filter,
				placed, outside, body.UndatedRecordCount, timeline.EntryCount, timeline.UndatedRecordCount)
		}
		if _, err := time.Parse(time.RFC3339Nano, body.Start); err != nil || body.StepMs < 1 {
			t.Errorf("%q: start=%q stepMs=%d", filter, body.Start, body.StepMs)
		}
	}
}

// 区切りの数の無い要求、範囲の外の数、2 回の指定、時系列だけの項目を invalid_request で退ける。
func TestTimeHistogramRejectsAnInvalidRequest(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"", "columns=0", "columns=1001", "columns=x", "columns=60&columns=60",
		"columns=60&find=x", "columns=60&nodeId=n", "columns=60&source=no-such-source",
	} {
		if response := requestTimeHistogram(handler, query); response.Code != http.StatusBadRequest {
			t.Errorf("%q returned status=%d", query, response.Code)
		}
	}
}
