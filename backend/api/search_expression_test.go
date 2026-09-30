package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// recordsQuery はレコードのノードだけを、位置の要約を添えて返す要求である。
const recordsQuery = "depth=0&granularity=record&nodeKind=record&recordSummary=true"

// matchedRecordKeys は、応答のレコードのノードが指すレコードを、収集元と位置の文字列で並べる。
func matchedRecordKeys(t *testing.T, page graphResponse) []string {
	t.Helper()
	keys := make([]string, 0, len(page.Nodes))
	for _, node := range page.Nodes {
		if node.Record == nil {
			t.Fatalf("the record node %q carries no summary", node.Id)
		}
		keys = append(keys, node.Record.RecordRef.SourceId+" "+node.Record.RecordRef.RecordRawTextRef)
	}
	slices.Sort(keys)
	return keys
}

// 検索式は、欄と文字列の組と同じレコードを残し、グラフと時系列の両方が同じレコードを返す。
// 応答は式の文字列をそのまま返す。
func TestGraphAndTimelineApplyTheSameSearchExpression(t *testing.T) {
	handler := graphHandler(t)
	expression := "psPath contains app.exe and dstPort == 8080"
	expressed := decodeGraph(t, handler, recordsQuery+"&searchExpression="+url.QueryEscape(expression))
	paired := decodeGraph(t, handler, recordsQuery+"&fieldContains=psPath%3Dapp.exe&fieldContains=dstPort%3D8080")
	if expressed.NodeCount == 0 {
		t.Fatal("the expression matched no record, want the records of the port 8080 from app.exe")
	}
	if got, want := matchedRecordKeys(t, expressed), matchedRecordKeys(t, paired); !slices.Equal(got, want) {
		t.Errorf("the expression matched %v, want the records the field terms matched %v", got, want)
	}
	if expressed.SearchExpression != expression {
		t.Errorf("searchExpression echoes %q, want %q", expressed.SearchExpression, expression)
	}
	timeline := decodeTimeline(t, handler, "searchExpression="+url.QueryEscape(expression))
	if timeline.SearchExpression != expression {
		t.Errorf("the timeline echoes %q, want %q", timeline.SearchExpression, expression)
	}
	listed := make([]string, 0, len(timeline.Entries))
	for _, entry := range timeline.Entries {
		listed = append(listed, entry.RecordRef.SourceId+" "+entry.RecordRef.RecordRawTextRef)
	}
	slices.Sort(listed)
	if want := matchedRecordKeys(t, expressed); !slices.Equal(listed, want) ||
		timeline.EntryCount != int64(len(timeline.Entries)) {
		t.Errorf("the timeline listed %v, want the records the graph matched %v", listed, want)
	}
	// or で結ぶと、どちらかを満たすレコードが増える。
	either := decodeGraph(t, handler, recordsQuery+"&searchExpression="+
		url.QueryEscape("psPath contains app.exe or dstPort == 8080"))
	if either.NodeCount < expressed.NodeCount {
		t.Errorf("the disjunction matched %d records, fewer than the %d of the conjunction",
			either.NodeCount, expressed.NodeCount)
	}
	// 一致するレコードの無い式は、文字列の条件と同じ 0 件の理由を返す。
	none := decodeGraph(t, handler, recordsQuery+"&searchExpression="+url.QueryEscape("dstPort == 1"))
	if none.NodeCount != 0 || none.EmptyReason != core.EmptyReasonNoValueMatch {
		t.Errorf("nodeCount=%d emptyReason=%q, want no record and no_value_match", none.NodeCount, none.EmptyReason)
	}
}

// 構文の誤りは、グラフと時系列と、時系列と同じ条件で絞るノードの一覧と件数の分布で 400 と
// 誤りの範囲を返す。
func TestGraphAndTimelineLocateTheSyntaxError(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		text string
		want core.SearchExpressionError
	}{
		{"", core.SearchExpressionError{Reason: core.SearchExpressionErrorReasonEmptyExpression}},
		{"dstPort = 8080", core.SearchExpressionError{
			Reason: core.SearchExpressionErrorReasonUnexpectedCharacter, Offset: 8, Length: 1,
		}},
		{"(dstPort == 8080", core.SearchExpressionError{
			Reason: core.SearchExpressionErrorReasonUnclosedParenthesis, Offset: 0, Length: 1,
		}},
	} {
		query := "searchExpression=" + url.QueryEscape(testCase.text)
		summaries := requestNodeSummaries(handler, "nodeKind=terminal&"+query)
		if summaries.Code != http.StatusBadRequest {
			t.Fatalf("node summaries for %q status=%d, want 400", testCase.text, summaries.Code)
		}
		var summariesError core.ApiError
		decodeJSON(t, summaries.Body, &summariesError)
		histogram := requestTimeHistogram(handler, "columns=60&"+query)
		if histogram.Code != http.StatusBadRequest {
			t.Fatalf("time histogram for %q status=%d, want 400", testCase.text, histogram.Code)
		}
		var histogramError core.ApiError
		decodeJSON(t, histogram.Body, &histogramError)
		for name, apiError := range map[string]core.ApiError{
			"graph":          requestGraphError(t, handler, recordsQuery+"&"+query, http.StatusBadRequest),
			"timeline":       requestTimelineError(t, handler, query, http.StatusBadRequest),
			"node summaries": summariesError,
			"time histogram": histogramError,
		} {
			if apiError.Code != core.ApiErrorCodeInvalidRequest || apiError.SearchExpressionError == nil ||
				*apiError.SearchExpressionError != testCase.want {
				t.Errorf("%s for %q returned %+v, want invalid_request with %+v",
					name, testCase.text, apiError, testCase.want)
				continue
			}
			if err := apiError.Validate(); err != nil {
				t.Errorf("%s for %q returned an error that does not validate: %v", name, testCase.text, err)
			}
		}
	}
	// 式の項目は 1 回だけ書く。構文の誤りではないため、誤りの範囲を持たない。
	repeated := requestGraphError(t, handler,
		recordsQuery+"&searchExpression=a&searchExpression=b", http.StatusBadRequest)
	if repeated.SearchExpressionError != nil {
		t.Errorf("the repeated item returned the syntax error %+v, want none", repeated.SearchExpressionError)
	}
	requestTimelineError(t, handler, "searchExpression=a&searchExpression=b", http.StatusBadRequest)
}
