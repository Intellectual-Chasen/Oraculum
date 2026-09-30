// in-package test: HTTP の要求のエッジの根拠を HTTP の状態で分ける。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// **HTTP の要求の根拠は、観測の種別が同じでも HTTP の状態ごとの区分に分かれる。** 区分を指す
// 値で絞ると、その状態の根拠だけが返る。
func TestEdgeEvidenceGroupsSplitTheHttpRequestsByTheStatus(t *testing.T) {
	proxy := `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://example.test/a HTTP/1.1" 404 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:04:10:01 +0000] "GET http://example.test/b HTTP/1.1" 404 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:04:10:02 +0000] "GET http://example.test/c HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n"
	graph := NewGraph(sessionSourcesResult(t,
		sessionSource{name: "access.log", format: SquidFormatKey, document: proxy}), AllMatchConditions())
	checked := 0
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindHttpRequest || len(edge.evidence) != 3 {
			continue
		}
		checked++
		detail, _ := graph.EdgeDetail(edge.id, EdgeEvidenceFilter{})
		counts := map[string]int64{}
		for _, group := range detail.EvidenceGroups {
			if err := group.Validate(); err != nil {
				t.Fatalf("the group %+v = %v, want a valid group", group, err)
			}
			if group.HttpStatus == nil || group.Selector == nil || group.Selector.HttpStatus == "" {
				t.Fatalf("the group %+v carries no HTTP status to name", group)
			}
			status, _ := comparableTextOf(*group.HttpStatus)
			counts[status] = group.EvidenceCount
			narrowed, _ := graph.EdgeDetail(edge.id, EdgeEvidenceFilterOf(*group.Selector))
			if narrowed.Edge.EvidenceCount != group.EvidenceCount {
				t.Errorf("the selector of the status %s returned %d records, want %d",
					status, narrowed.Edge.EvidenceCount, group.EvidenceCount)
			}
		}
		if len(counts) != 2 || counts["404"] != 2 || counts["200"] != 1 {
			t.Errorf("the groups count %v, want 2 records of 404 and 1 record of 200", counts)
		}
	}
	if checked == 0 {
		t.Fatal("the graph carries no HTTP request relation of the 3 requests")
	}
}

// HTTP の要求でないレコードの区分は HTTP の状態を持たず、区分を指す値も状態で絞らない。
func TestEdgeEvidenceGroupsOutsideHttpCarryNoStatus(t *testing.T) {
	detail := logonTypesDetail(t, EdgeEvidenceFilter{})
	for _, group := range detail.EvidenceGroups {
		if group.HttpStatus != nil || group.HttpStatusAbsence != "" ||
			group.Selector != nil && (group.Selector.HttpStatus != "" || group.Selector.HttpStatusAbsent) {
			t.Errorf("the group %+v carries an HTTP status", group)
		}
	}
	if !slices.ContainsFunc(detail.EvidenceGroups, func(group core.EdgeEvidenceGroup) bool {
		return group.Selector != nil
	}) {
		t.Error("the groups carry no selectable group")
	}
}

// **観測の種別を持たない HTTP の区分を指す値は、観測の種別を持つ HTTP の要求を通さない。**
// 同じ状態の値を持っていても、観測の種別を持つレコードは別の区分である。
func TestHttpStatusFilterWithoutKindSkipsTheRecordsWithAKind(t *testing.T) {
	text := func(name string, semantic core.SemanticKey, value string) *core.RecordField {
		t.Helper()
		raw, err := core.NewRawValue(core.ValueStatePresent, value)
		if err != nil {
			t.Fatal(err)
		}
		field, err := core.NewTextField(name, semantic, raw)
		if err != nil {
			t.Fatal(err)
		}
		return &field
	}
	url := text("url", core.SemanticKeyHttpRequestUrl, "http://example.test/a")
	status := text("status", core.SemanticKeyHttpStatusCode, "404")
	graph := Graph{records: []graphRecord{
		{requestUrl: url, httpStatus: status},
		{requestUrl: url, httpStatus: status, eventCategory: "web", eventAction: "request"},
	}}
	filter := EdgeEvidenceFilter{HttpStatus: "404", LogonTypeAbsent: true, DestinationPortAbsent: true}
	if !graph.recordInGroup(0, filter) {
		t.Error("the record without a kind did not pass the status 404")
	}
	if graph.recordInGroup(1, filter) {
		t.Error("the record with a kind passed the filter of the group without a kind")
	}
}
