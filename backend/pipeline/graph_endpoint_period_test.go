// in-package test: 端点のレコードが期間の外にあるエッジを外す要求を確かめる。
package pipeline

import (
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 期間の中のログオンから、期間の外の操作へ結んだ候補は、端点のレコードを期間で判定する要求で外れ、
// 判定しない要求では残る。
func TestEndpointRecordsInPeriodDropsEdgesToRecordsOutsideThePeriod(t *testing.T) {
	graph, nodes := logonSessionGraph(t)
	from := time.Date(2001, 2, 3, 4, 5, 8, 0, time.UTC)
	to := time.Date(2001, 2, 3, 4, 5, 12, 0, time.UTC)
	query := GraphQuery{
		Granularity: core.GraphGranularityRecord,
		NodeKinds:   []core.NodeKind{core.NodeKindRecord},
		EdgeKinds:   []core.EdgeKind{core.EdgeKindLogonSessionOperation},
		Depth:       1,
		RecordFilter: RecordFilter{
			TimeFrom: &from, TimeTo: &to, TimeUnit: time.Second,
		},
	}
	idOf := make(map[string]string, len(nodes))
	for recordID, at := range nodes {
		idOf[graph.nodes[at].id] = recordID
	}
	edgesOf := func(query GraphQuery) []string {
		var got []string
		for _, edge := range graph.Query(query).Edges {
			got = append(got, idOf[edge.SourceNodeId]+"->"+idOf[edge.TargetNodeId])
		}
		slices.Sort(got)
		return got
	}

	if got, want := edgesOf(query), []string{"31->32", "31->33", "31->34", "31->39"}; !slices.Equal(got, want) {
		t.Errorf("without the endpoint check the edges are %v, want %v", got, want)
	}
	query.EndpointRecordsInPeriod = true
	if got, want := edgesOf(query), []string{"31->32"}; !slices.Equal(got, want) {
		t.Errorf("with the endpoint check the edges are %v, want %v", got, want)
	}

	// 期間を与えない要求では、判定を指定してもエッジを外さない。
	query.RecordFilter = RecordFilter{}
	checked := edgesOf(query)
	query.EndpointRecordsInPeriod = false
	if unchecked := edgesOf(query); len(checked) == 0 || !slices.Equal(checked, unchecked) {
		t.Errorf("without a period the endpoint check gave %v, want %v", checked, unchecked)
	}
}
