package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// casedHandler は同じ収集元を 2 つの案件で取り込んだ handler を返す。
func casedHandler(t *testing.T) http.Handler {
	t.Helper()
	baseline, challenge := "baseline", "challenge"
	return testHandler(importResultOfFiles(t,
		pipeline.SourcePlan{FormatKey: markIIFormatKey, FileName: "graph-markii.log",
			OriginPath: "testdata/graph-markii.log", CaseId: &baseline},
		pipeline.SourcePlan{FormatKey: markIIFormatKey, FileName: "graph-markii.log",
			OriginPath: "testdata/graph-markii.log", CaseId: &challenge},
	))
}

// breakdownOf は案件ごとの件数を案件の識別子で探せる表にする。
func breakdownOf(counts []core.CaseEvidenceCount) map[string]int64 {
	table := make(map[string]int64, len(counts))
	for _, count := range counts {
		table[count.CaseId] = count.EvidenceCount
	}
	return table
}

// 部分グラフのエッジは案件ごとの件数を持ち、case を与えた要求はその案件の件数を返す。
func TestGraphEndpointCountsAndNarrowsByCase(t *testing.T) {
	handler := casedHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	if len(whole.Edges) == 0 {
		t.Fatal("the graph carries no edge")
	}
	breakdowns := make(map[string]map[string]int64, len(whole.Edges))
	for _, edge := range whole.Edges {
		breakdown := breakdownOf(edge.EvidenceByCase)
		if breakdown["baseline"]+breakdown["challenge"] != edge.EvidenceCount {
			t.Errorf("edge %s breaks down as %v with %d records", edge.Id, edge.EvidenceByCase, edge.EvidenceCount)
		}
		breakdowns[edge.Id] = breakdown
	}
	narrowed := decodeGraph(t, handler, wholeGraphQuery+"&case=baseline")
	if narrowed.Case != "baseline" || len(narrowed.Edges) == 0 {
		t.Fatalf("the narrowed response echoes %q with %d edges", narrowed.Case, len(narrowed.Edges))
	}
	for _, edge := range narrowed.Edges {
		if edge.EvidenceCount != breakdowns[edge.Id]["baseline"] {
			t.Errorf("edge %s carries %d baseline records, the breakdown says %d",
				edge.Id, edge.EvidenceCount, breakdowns[edge.Id]["baseline"])
		}
	}
}

// ノードの詳細とエッジの詳細も案件ごとの件数を返す。
func TestDetailEndpointsCountEvidenceByCase(t *testing.T) {
	handler := casedHandler(t)
	edge := graphEdgeOfKind(t, handler, "ran_on")
	detail := decodeEdge(t, handler, url.PathEscape(edge.Id), "")
	if got := breakdownOf(detail.Edge.EvidenceByCase); got["baseline"] < 1 || got["challenge"] < 1 {
		t.Errorf("the edge detail breaks down as %v", detail.Edge.EvidenceByCase)
	}
	narrowed := decodeEdge(t, handler, url.PathEscape(edge.Id), "case=challenge")
	if int64(len(narrowed.Edge.Evidence)) != breakdownOf(detail.Edge.EvidenceByCase)["challenge"] {
		t.Errorf("the challenge detail returns %d records, the breakdown says %v",
			len(narrowed.Edge.Evidence), detail.Edge.EvidenceByCase)
	}
	node := decodeNode(t, handler, edge.TargetNodeId, "")
	if got := breakdownOf(node.EvidenceByCase); got["baseline"]+got["challenge"] != node.EvidenceCount || len(got) != 2 {
		t.Errorf("the node detail breaks down as %v with %d records", node.EvidenceByCase, node.EvidenceCount)
	}
}

// case だけを与えた時系列の要求は、その案件の収集元だけを返す。
func TestTimelineEndpointNarrowsByCase(t *testing.T) {
	handler := casedHandler(t)
	whole := decodeTimeline(t, handler, "")
	narrowed := decodeTimeline(t, handler, "case=challenge")
	if narrowed.Case != "challenge" || narrowed.EntryCount == 0 || 2*narrowed.EntryCount != whole.EntryCount {
		t.Errorf("the challenge timeline echoes %q with %d entries of %d", narrowed.Case,
			narrowed.EntryCount, whole.EntryCount)
	}
	if len(narrowed.SourceCoverages) != 1 || len(whole.SourceCoverages) != 2 {
		t.Errorf("the challenge timeline covers %d sources of %d",
			len(narrowed.SourceCoverages), len(whole.SourceCoverages))
	}
}

// 取り込み結果に無い案件と、文字列を外れる案件を invalid_request で退ける。
func TestEndpointsRejectAnUnknownOrUnreadableCase(t *testing.T) {
	handler := casedHandler(t)
	edge := graphEdgeOfKind(t, handler, "ran_on")
	for _, value := range []string{"unknown", "has%20space", ""} {
		for name, path := range map[string]string{
			"graph":    graphPath + "?" + wholeGraphQuery + "&case=" + value,
			"timeline": timelinePath + "?case=" + value,
			"edge":     edgesPath + url.PathEscape(edge.Id) + "?case=" + value,
		} {
			response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(path))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "case") {
				t.Errorf("%s with case %q: status=%d body=%s", name, value, response.Code, response.Body.String())
			}
		}
	}
}
