package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// 知らない項目を退ける応答は、退けた名前と受け付ける名前を返す。
func TestGraphEndpointNamesTheRejectedAndAcceptedParameters(t *testing.T) {
	apiError := requestGraphError(t, graphHandler(t), "depth=1&eventId=1", http.StatusBadRequest)
	for _, want := range []string{`"eventId"`, "searchExpression", "nodeKind", "fieldEquals", "/api/v0/event-kinds"} {
		if !strings.Contains(apiError.Message, want) {
			t.Errorf("message=%q, want it to name %s", apiError.Message, want)
		}
	}
}

// 検索式の欄のうち、どのノードの属性にも無い欄を unknownFields が返す。
func TestGraphEndpointNamesTheUnknownSearchExpressionFields(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, recordsQuery+"&searchExpression="+
		url.QueryEscape("absentField == 1 or dstPort == 8080 or absentField == 2"))
	if !slices.Equal(page.UnknownFields, []string{"absentField"}) {
		t.Errorf("unknownFields=%v, want [absentField]", page.UnknownFields)
	}
	if !strings.Contains(page.UnknownFieldsHint, "/api/v0/event-kinds") {
		t.Errorf("unknownFieldsHint=%q, want it to name the event-kinds endpoint", page.UnknownFieldsHint)
	}
	known := decodeGraph(t, handler, recordsQuery+"&searchExpression="+url.QueryEscape("dstPort == 8080"))
	if known.UnknownFields != nil || known.UnknownFieldsHint != "" {
		t.Errorf("unknownFields=%v hint=%q, want none for an observed field", known.UnknownFields, known.UnknownFieldsHint)
	}
}
