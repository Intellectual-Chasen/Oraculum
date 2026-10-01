package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// イベント ID の範囲と、文字列を当てる欄の指定を受け付け、応答に返す。
func TestGraphAndTimelineEchoTheRangeAndTheValueField(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, wholeGraphQuery+
		"&eventActionFrom=8000&eventActionTo=8999&valueContains=app&valueField=psPath")
	if page.EventActionFrom == nil || *page.EventActionFrom != 8000 ||
		page.EventActionTo == nil || *page.EventActionTo != 8999 || page.ValueField != "psPath" {
		t.Errorf("graph echoes %v %v %q, want the range and the field", page.EventActionFrom, page.EventActionTo, page.ValueField)
	}
	timeline := decodeTimeline(t, handler, "eventActionFrom=8000")
	if timeline.EventActionFrom == nil || *timeline.EventActionFrom != 8000 || timeline.EventActionTo != nil {
		t.Errorf("timeline echoes %v %v, want the lower bound alone", timeline.EventActionFrom, timeline.EventActionTo)
	}
}

// 欄と文字列の組を複数置くと、どの組も満たすレコードが残る。組は応答に返る。
func TestGraphMatchesEveryFieldTermPair(t *testing.T) {
	handler := graphHandler(t)
	records := "depth=0&granularity=record&nodeKind=record"
	both := decodeGraph(t, handler, records+"&fieldContains=psPath%3Dapp.exe&fieldContains=dstPort%3D8080")
	if both.NodeCount != 3 {
		t.Errorf("nodeCount=%d, want the 3 records of the port 8080 from app.exe", both.NodeCount)
	}
	if !slices.Equal(both.FieldContains, []string{"psPath=app.exe", "dstPort=8080"}) {
		t.Errorf("fieldContains echoes %v", both.FieldContains)
	}
	// 文字列はその組の欄でだけ判定する。
	crossed := decodeGraph(t, handler, records+"&fieldContains=psPath%3D8080&fieldContains=dstPort%3Dapp.exe")
	if crossed.NodeCount != 0 || crossed.EmptyReason != core.EmptyReasonNoValueMatch {
		t.Errorf("nodeCount=%d emptyReason=%q, want no record", crossed.NodeCount, crossed.EmptyReason)
	}
	unknown := decodeGraph(t, handler, records+"&fieldContains=psPath%3Dapp.exe&fieldContains=noSuchField%3Dx")
	if unknown.NodeCount != 0 || unknown.EmptyReason != core.EmptyReasonNoFieldObserved {
		t.Errorf("nodeCount=%d emptyReason=%q, want no_field_observed", unknown.NodeCount, unknown.EmptyReason)
	}
}

// 完全一致の組は、値の全体が文字列と等しい欄だけに一致する。組は応答に返る。
func TestGraphMatchesTheWholeValueOfAFieldEqualsPair(t *testing.T) {
	handler := graphHandler(t)
	records := "depth=0&granularity=record&nodeKind=record"
	part := decodeGraph(t, handler, records+"&fieldContains=dstPort%3D808")
	if part.NodeCount != 3 {
		t.Fatalf("nodeCount=%d, want the 3 records whose port contains 808", part.NodeCount)
	}
	whole := decodeGraph(t, handler, records+"&fieldEquals=dstPort%3D808")
	if whole.NodeCount != 0 || !slices.Equal(whole.FieldEquals, []string{"dstPort=808"}) {
		t.Errorf("nodeCount=%d fieldEquals=%v, want no record and the echoed pair", whole.NodeCount, whole.FieldEquals)
	}
	equal := decodeGraph(t, handler, records+"&fieldEquals=dstPort%3D8080&fieldContains=psPath%3Dapp.exe")
	if equal.NodeCount != 3 {
		t.Errorf("nodeCount=%d, want the 3 records of the port 8080 from app.exe", equal.NodeCount)
	}
}

// 同じ文字列条件を使ったグラフと時系列は、同じレコードを返す。
func TestGraphAndTimelineApplyTheSameTextFilters(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name  string
		query string
	}{
		{"含む", "valueContains=app.exe"},
		{"含まない", "valueExcludes=app.exe"},
		{"フィールド指定", "valueContains=app.exe&valueField=psPath"},
		{"フィールド内の部分一致", "fieldContains=psPath%3Dapp.exe"},
		{"フィールド内の完全一致", "fieldEquals=dstPort%3D8080"},
		{"条件の組み合わせ", "valueContains=app.exe&fieldEquals=dstPort%3D8080"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graph := decodeGraph(t, handler, recordsQuery+"&"+testCase.query)
			timeline := decodeTimeline(t, handler, testCase.query)
			listed := make([]string, 0, len(timeline.Entries))
			for _, entry := range timeline.Entries {
				listed = append(listed, entry.RecordRef.SourceId+" "+entry.RecordRef.RecordRawTextRef)
			}
			slices.Sort(listed)
			if want := matchedRecordKeys(t, graph); !slices.Equal(listed, want) {
				t.Errorf("the timeline listed %v, want the graph records %v", listed, want)
			}
		})
	}
}

// 時系列は文字列条件の値の形と組み合わせをグラフと同じ検証で退ける。
func TestTimelineValidatesTextFiltersLikeGraph(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"valueContains=",
		"valueField=psPath",
		"fieldContains=psPath",
		"fieldEquals=psPath%3D",
	} {
		graphError := requestGraphError(t, handler, recordsQuery+"&"+query, http.StatusBadRequest)
		timelineError := requestTimelineError(t, handler, query, http.StatusBadRequest)
		if graphError.Message != timelineError.Message {
			t.Errorf("query %q graph rejected it as %q, timeline rejected it as %q",
				query, graphError.Message, timelineError.Message)
		}
	}
}

func TestGraphRejectsABrokenRangeOrAFieldWithoutTerms(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"eventActionFrom=",
		"eventActionFrom=abc",
		"eventActionFrom=-1",
		"eventActionFrom=+5",
		"eventActionFrom=9007199254740992",
		"eventActionTo=99999999999999999999",
		"eventActionFrom=9000&eventActionTo=8000",
		"valueField=psPath",
		"valueField=&valueContains=app",
		// 欄と文字列の組は `欄=文字列` で書き、どちらも空にできない。
		"fieldContains=psPath",
		"fieldContains=%3Dapp",
		"fieldContains=psPath%3D",
		// 組も検索の文字列の数に入り、欄と文字列はそれぞれ 1,024 byte までである。
		strings.Repeat("valueContains=a&", 16) + "fieldContains=psPath%3Dapp",
		"fieldContains=psPath%3D" + strings.Repeat("a", 1025),
		"fieldContains=" + strings.Repeat("a", 1025) + "%3Dapp",
	} {
		response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(graphPath+"?"+wholeGraphQuery+"&"+query))
		if response.Code != http.StatusBadRequest {
			t.Errorf("query=%q status=%d, want 400", query, response.Code)
		}
	}
	requestTimelineError(t, handler, "eventActionTo=x", http.StatusBadRequest)
	// 事象の種別の一覧は範囲を受け付けない。範囲はその一覧が選択肢を返す条件である。
	if response := requestEventKinds(t, handler, "eventActionFrom=8000"); response.Code != http.StatusBadRequest {
		t.Errorf("event-kinds status=%d, want 400", response.Code)
	}
}
