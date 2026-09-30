// 収集元で絞る要求の項目を確かめる。
package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 収集元で絞った要求は、その収集元のレコードだけを残し、用いた収集元を応答に返す。2 つを
// 与えた要求は、どちらかの収集元のレコードを残す。
func TestGraphEndpointNarrowsToTheRequestedSources(t *testing.T) {
	result := graphImportResult(t)
	handler := testHandler(result)
	fileOf := make(map[string]string)
	var ids []string
	for _, status := range result.Statuses() {
		identity, found := result.Identity(status.SourceId)
		if !found {
			t.Fatalf("the source %s carries no identity", status.SourceId)
		}
		fileOf[status.SourceId] = identity.FileName
		ids = append(ids, status.SourceId)
	}
	if len(ids) < 2 {
		t.Fatalf("the fixture carries %d sources, want 2 or more", len(ids))
	}
	const records = "depth=0&nodeKind=record"
	whole := decodeGraph(t, handler, records)

	narrowed := decodeGraph(t, handler, records+"&source="+url.QueryEscape(ids[0]))
	if narrowed.NodeCount == 0 || narrowed.NodeCount >= whole.NodeCount {
		t.Fatalf("the narrowed response matched %d of %d records", narrowed.NodeCount, whole.NodeCount)
	}
	for _, node := range narrowed.Nodes {
		label, _ := node.Label.ComparableValue()
		if !strings.HasPrefix(label, fileOf[ids[0]]) {
			t.Errorf("the narrowed response carries the record %q of another source", label)
		}
	}
	if !slices.Equal(narrowed.Sources, ids[:1]) {
		t.Errorf("the response returned source=%v, want %v", narrowed.Sources, ids[:1])
	}

	var both []string
	for _, id := range ids {
		both = append(both, "source="+url.QueryEscape(id))
	}
	every := decodeGraph(t, handler, records+"&"+strings.Join(both, "&"))
	if every.NodeCount != whole.NodeCount {
		t.Errorf("every source matched %d records, want %d", every.NodeCount, whole.NodeCount)
	}
}

// recordSummary を与えた要求は、合致したレコードのノードに位置と時刻と事象の種別を添える。
// 与えない要求と、レコードでないノードは持たない。
func TestGraphEndpointSummarizesTheMatchedRecords(t *testing.T) {
	handler := graphHandler(t)
	const records = "depth=0&nodeKind=record&nodeKind=process&eventCategory=ps"
	summarized := decodeGraph(t, handler, records+"&recordSummary=true")
	var carried int
	for _, node := range summarized.Nodes {
		if node.Kind != string(core.NodeKindRecord) {
			if node.Record != nil {
				t.Errorf("the %s node %s carries a record summary", node.Kind, node.Id)
			}
			continue
		}
		if node.Record == nil {
			t.Errorf("the record node %s carries no summary", node.Id)
			continue
		}
		carried++
		if node.Record.RecordRef.SourceFileName == "" || node.Record.EventTime == nil ||
			node.Record.EventCategory != "ps" || node.Record.EventAction == "" {
			t.Errorf("the record node %s carries %+v", node.Id, node.Record)
		}
	}
	if carried == 0 {
		t.Fatal("the response carries no record node")
	}
	for _, node := range decodeGraph(t, handler, records).Nodes {
		if node.Record != nil {
			t.Errorf("the node %s carries a summary without recordSummary", node.Id)
		}
	}
	apiError := requestGraphError(t, handler, records+"&recordSummary=false", http.StatusBadRequest)
	if apiError.Code != core.ApiErrorCodeInvalidRequest {
		t.Errorf("code=%q want invalid_request", apiError.Code)
	}
}

// 空の収集元と、取り込み結果に無い収集元を退ける。
func TestGraphEndpointRejectsAnUnknownSource(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{"source=", "source=no-such-source"} {
		apiError := requestGraphError(t, handler,
			"depth=0&"+query, http.StatusBadRequest)
		if apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s: code=%q want invalid_request", query, apiError.Code)
		}
	}
	// 同じ要求の項目を読む ATT&CK 候補と調べる順序の目安も、知らない収集元を退ける。
	for _, path := range []string{
		attackCandidatesPath + "?depth=1&source=no-such-source",
		investigationOrderPath + "record_count_descending?depth=1&source=no-such-source",
	} {
		response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(path))
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d want 400", path, response.Code)
		}
	}
}
