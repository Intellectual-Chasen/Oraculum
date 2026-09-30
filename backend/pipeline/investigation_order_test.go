// in-package test: レコードからグラフを組み、調べる順序の目安の並びと入力を確かめる。
package pipeline

import (
	"slices"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// orderTestRecord は端末 T1 のプロセスがファイルを指すレコードを組む。
func orderTestRecord(t *testing.T, line int64, process, path string) RecordEntry {
	t.Helper()
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, process),
				syntheticField(t, "path", core.SemanticKeyFilePath, path),
			},
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		},
	}
}

// orderTestGraph はレコードを 1 つの収集元として取り込んだグラフを組む。
func orderTestGraph(t *testing.T, records ...RecordEntry) Graph {
	t.Helper()
	source := settleSource(t, 0)
	source.Records = records
	result, err := newImportResult([]scannedSource{source},
		[]core.ImportStatus{settleStatus(t, source, "source")}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	return NewGraph(result, AllMatchConditions())
}

// orderTestRecords は、プロセス {P1} が 2 件、{P2} が 1 件のレコードに指される 3 件である。
func orderTestRecords(t *testing.T) []RecordEntry {
	t.Helper()
	return []RecordEntry{
		orderTestRecord(t, 1, "{P1}", `c:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `c:\a.txt`),
		orderTestRecord(t, 3, "{P1}", `c:\b.txt`),
	}
}

// orderedIdentity は種別の並びを、識別の最後の値と順位と同じ順位の数の組で返す。
type orderedIdentity struct {
	identity string
	rank     int
	tieCount int
}

func orderedIdentitiesOf(t *testing.T, order InvestigationOrder, kind core.NodeKind) []orderedIdentity {
	t.Helper()
	at := slices.IndexFunc(order.Kinds, func(k InvestigationOrderKind) bool { return k.Kind == kind })
	if at < 0 {
		t.Fatalf("the order has no %s entries: %+v", kind, order.Kinds)
	}
	var identities []orderedIdentity
	for _, entry := range order.Kinds[at].Entries {
		identity := entry.Node.Identity
		identities = append(identities, orderedIdentity{
			identity: identity[len(identity)-1].Value, rank: entry.Rank, tieCount: entry.TieCount,
		})
	}
	return identities
}

func investigationOrderOf(t *testing.T, graph Graph, query GraphQuery, method string) InvestigationOrder {
	t.Helper()
	order, known := graph.InvestigationOrder(query, method, SigmaEvaluation{}, "")
	if !known {
		t.Fatalf("InvestigationOrder(%q) reports an unknown method", method)
	}
	return order
}

func TestInvestigationOrderRanksObjectsByRecordCount(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)

	descending := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountDescending)
	if got, want := orderedIdentitiesOf(t, descending, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 1}, {"{P2}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("descending processes = %+v, want %+v", got, want)
	}
	if got, want := orderedIdentitiesOf(t, descending, core.NodeKindFile), []orderedIdentity{
		{`c:\a.txt`, 1, 1}, {`c:\b.txt`, 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("descending files = %+v, want %+v", got, want)
	}

	ascending := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountAscending)
	if got, want := orderedIdentitiesOf(t, ascending, core.NodeKindProcess), []orderedIdentity{
		{"{P2}", 1, 1}, {"{P1}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("ascending processes = %+v, want %+v", got, want)
	}
}

func TestInvestigationOrderLeavesOutRecordNodes(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	order := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountDescending)

	kinds := make([]core.NodeKind, 0, len(order.Kinds))
	for _, kind := range order.Kinds {
		kinds = append(kinds, kind.Kind)
	}
	if want := []core.NodeKind{core.NodeKindFile, core.NodeKindProcess, core.NodeKindTerminal}; !slices.Equal(kinds, want) {
		t.Errorf("kinds = %v, want %v without the record kind", kinds, want)
	}
	if order.Inputs.ObjectCount != 5 || order.Inputs.RecordCount != 3 {
		t.Errorf("inputs = %+v, want 5 objects named by 3 records", order.Inputs)
	}
}

func TestInvestigationOrderGivesTiedObjectsTheSameRank(t *testing.T) {
	records := append(orderTestRecords(t), orderTestRecord(t, 4, "{P2}", `c:\b.txt`))
	graph := orderTestGraph(t, records...)
	order := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountDescending)

	// 同じ値の中はグラフの並び順 (取り込みの順) に並ぶ。
	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 2}, {"{P2}", 1, 2},
	}; !slices.Equal(got, want) {
		t.Errorf("tied processes = %+v, want %+v", got, want)
	}
}

func TestInvestigationOrderPairsObjectsNamedBySameRecord(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	input := graph.orderInputOf(GraphQuery{Depth: 1})

	identityOf := func(position int) string {
		identity := graph.nodes[input.objects[position]].key.Identity()
		return identity[len(identity)-1].Value
	}
	pairs := make(map[[2]string][]int, len(input.pairs))
	for _, pair := range input.pairs {
		pairs[[2]string{identityOf(pair.a), identityOf(pair.b)}] = pair.records
	}
	if len(pairs) != 7 {
		t.Errorf("pairs = %v, want the 7 pairs of objects named together", pairs)
	}
	// 2 件目のレコード (位置 1) だけが {P2} と a.txt を指す。
	for key, records := range pairs {
		if (key == [2]string{`c:\a.txt`, "{P2}"} || key == [2]string{"{P2}", `c:\a.txt`}) &&
			!slices.Equal(records, []int{1}) {
			t.Errorf("records of the {P2} and a.txt pair = %v, want [1]", records)
		}
	}
	if _, found := pairs[[2]string{"{P2}", `c:\b.txt`}]; found {
		t.Error("{P2} and b.txt are paired, want no pair for objects no record names together")
	}
}

func TestInvestigationOrderScopesToTheOrigin(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	whole := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountDescending)
	processes := orderedIdentitiesOf(t, whole, core.NodeKindProcess)
	if len(processes) != 2 {
		t.Fatalf("processes = %+v, want 2", processes)
	}
	var originId string
	for _, kind := range whole.Kinds {
		for _, entry := range kind.Entries {
			if entry.Node.Kind == core.NodeKindProcess && entry.Node.Identity[1].Value == "{P2}" {
				originId = entry.Node.Id
			}
		}
	}

	for _, tc := range []struct {
		query GraphQuery
		want  []string
	}{
		{GraphQuery{NodeIds: []string{originId}}, []string{"{P2}"}},
		{GraphQuery{NodeIds: []string{originId}, Depth: 1}, []string{"T1", `c:\a.txt`, "{P2}"}},
		{GraphQuery{NodeIds: []string{"n:process:absent"}}, nil},
	} {
		if got := objectIdentitiesOf(graph, graph.orderInputOf(tc.query)); !slices.Equal(got, tc.want) {
			t.Errorf("objects of %+v = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// objectIdentitiesOf は入力の対象を、識別の最後の値の文字列順で返す。
func objectIdentitiesOf(graph Graph, input orderInput) []string {
	var identities []string
	for _, index := range input.objects {
		identity := graph.nodes[index].key.Identity()
		identities = append(identities, identity[len(identity)-1].Value)
	}
	slices.Sort(identities)
	return identities
}

func TestInvestigationOrderCountsOnlyRecordsPassingTheFilter(t *testing.T) {
	records := orderTestRecords(t)
	for i := range records {
		records[i].ObservedAt = matchTestTime(t, 5*(i+1))
	}
	graph := orderTestGraph(t, records...)
	// 3 件目 ({P1} が b.txt を指す) を期間の外に置く。
	zone := time.FixedZone("", 9*60*60)
	from, to := time.Date(2024, 3, 14, 10, 20, 5, 0, zone), time.Date(2024, 3, 14, 10, 20, 10, 0, zone)
	query := GraphQuery{RecordFilter: RecordFilter{TimeFrom: &from, TimeTo: &to, TimeUnit: time.Second}}

	order := investigationOrderOf(t, graph, query, InvestigationOrderRecordCountDescending)
	if got, want := objectIdentitiesOf(graph, graph.orderInputOf(GraphQuery{Depth: 1, RecordFilter: query.RecordFilter})),
		[]string{"T1", `c:\a.txt`, "{P1}", "{P2}"}; !slices.Equal(got, want) {
		t.Errorf("objects = %v, want %v without b.txt", got, want)
	}
	if order.Inputs.RecordCount != 2 || order.Inputs.TimedRecordCount != 2 {
		t.Errorf("inputs = %+v, want the 2 records in the period", order.Inputs)
	}
	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 2}, {"{P2}", 1, 2},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want %+v counting 1 record each", got, want)
	}
	sigma := sigmaTestEvaluation(graph, sigmaTestRule{level: "high", records: []int{0, 2}})
	sigmaOrder, known := graph.InvestigationOrder(query, InvestigationOrderSigmaMatches, sigma, "")
	if !known {
		t.Fatal("InvestigationOrder reports the sigma method as unknown")
	}
	if got := processValuesOf(t, sigmaOrder)["{P1}"]; got == nil || *got != 5*sigmaLevelRankMultiplier+1 {
		t.Errorf("sigma value of {P1} = %v, want high with the 1 matched record in the period", got)
	}
}

func TestInvestigationOrderSummarizesTimedRecords(t *testing.T) {
	records := orderTestRecords(t)
	records[0].ObservedAt = matchTestTime(t, 5)
	records[2].ObservedAt = matchTestTime(t, 9)
	graph := orderTestGraph(t, records...)
	order := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderRecordCountDescending)

	if order.Inputs.TimedRecordCount != 2 || order.Inputs.TimeRange == nil {
		t.Fatalf("inputs = %+v, want 2 timed records and their range", order.Inputs)
	}
	if from, to := *order.Inputs.TimeRange.From.RawText, *order.Inputs.TimeRange.To.RawText; from !=
		"2024-03-14T10:20:05+09:00" || to != "2024-03-14T10:20:09+09:00" {
		t.Errorf("time range = %s .. %s, want the earliest and latest timed records", from, to)
	}
}

func TestInvestigationOrderPlacesObjectsWithoutValueLast(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	input := graph.orderInputOf(GraphQuery{Depth: 1})
	values := make([]orderValue, len(input.objects))
	for i, index := range input.objects {
		identity := graph.nodes[index].key.Identity()
		switch identity[len(identity)-1].Value {
		case "{P1}":
			// 有効数字 10 桁に丸めると {P2} と同じ値になる。
			values[i] = orderValue{value: 1 + 1e-12, present: true}
		case "{P2}":
			values[i] = orderValue{value: 1, present: true}
		}
	}
	kinds := graph.rankedKinds(input, values, true)
	at := slices.IndexFunc(kinds, func(k InvestigationOrderKind) bool { return k.Kind == core.NodeKindProcess })
	for _, entry := range kinds[at].Entries {
		if entry.Rank != 1 || entry.TieCount != 2 || entry.Value == nil {
			t.Errorf("process entry = %+v, want both processes tied at rank 1", entry)
		}
	}
	at = slices.IndexFunc(kinds, func(k InvestigationOrderKind) bool { return k.Kind == core.NodeKindFile })
	for _, entry := range kinds[at].Entries {
		if entry.Rank != 1 || entry.TieCount != 2 || entry.Value != nil {
			t.Errorf("file entry = %+v, want files without value tied in one group", entry)
		}
	}
}

func TestInvestigationOrderRejectsUnknownMethod(t *testing.T) {
	graph := orderTestGraph(t, orderTestRecords(t)...)
	if _, known := graph.InvestigationOrder(GraphQuery{}, "unknown", SigmaEvaluation{}, ""); known {
		t.Error("InvestigationOrder accepted an unknown method")
	}
}
