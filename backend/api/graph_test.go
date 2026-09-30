package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// graphPath は`/api/v0/graph` の path である。
const graphPath = "/api/v0/graph"

// wholeGraphQuery は fixture のグラフ全体が 1 ページに入る要求である。
// 上限は、レコードのノードと、レコードが対象を指す関係を含めた全体を覆う。
const wholeGraphQuery = "depth=1"

// graphResponse は`/api/v0/graph` の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type graphResponse struct {
	Nodes                   []subgraphNode       `json:"nodes"`
	NodeCount               int64                `json:"nodeCount"`
	Edges                   []graphEdge          `json:"edges"`
	EdgeCount               int64                `json:"edgeCount"`
	SubgraphNodeCount       int64                `json:"subgraphNodeCount"`
	NodeLimit               int                  `json:"nodeLimit,omitempty"`
	NodeLimitExceeded       bool                 `json:"nodeLimitExceeded,omitempty"`
	EdgeKindCounts          []core.EdgeKindCount `json:"edgeKindCounts,omitempty"`
	MatchedKinds            []core.MatchedKind   `json:"matchedKinds"`
	MatchedAccountPairs     int64                `json:"matchedAccountIdentityPairCount,omitempty"`
	NodeKinds               []string             `json:"nodeKinds,omitempty"`
	Granularity             string               `json:"granularity,omitempty"`
	NodeIds                 []string             `json:"nodeIds,omitempty"`
	Depth                   int                  `json:"depth"`
	EdgeKinds               []string             `json:"edgeKinds,omitempty"`
	EventCategory           string               `json:"eventCategory,omitempty"`
	EventAction             string               `json:"eventAction,omitempty"`
	EventActionFrom         *uint64              `json:"eventActionFrom,omitempty"`
	EventActionTo           *uint64              `json:"eventActionTo,omitempty"`
	ValueField              string               `json:"valueField,omitempty"`
	FieldContains           []string             `json:"fieldContains,omitempty"`
	FieldEquals             []string             `json:"fieldEquals,omitempty"`
	Case                    string               `json:"case,omitempty"`
	Terminal                string               `json:"terminal,omitempty"`
	AddressInCidr           string               `json:"addressInCidr,omitempty"`
	AddressNotInCidr        string               `json:"addressNotInCidr,omitempty"`
	ValueContains           []string             `json:"valueContains,omitempty"`
	ValueExcludes           []string             `json:"valueExcludes,omitempty"`
	SearchExpression        string               `json:"searchExpression,omitempty"`
	ConditionsOnOriginsOnly bool                 `json:"conditionsOnOriginsOnly,omitempty"`
	EndpointRecordsInPeriod bool                 `json:"endpointRecordsInPeriod,omitempty"`
	CountBy                 string               `json:"countBy,omitempty"`
	ValueCounts             []core.ValueCount    `json:"valueCounts,omitempty"`
	DistinctValueCount      *int64               `json:"distinctValueCount,omitempty"`
	ValueCountsEmptyReason  core.EmptyReason     `json:"valueCountsEmptyReason,omitempty"`
	ValueCountsNodeKinds    []core.NodeKind      `json:"valueCountsNodeKinds,omitempty"`
	Sources                 []string             `json:"source,omitempty"`
	TimeFrom                *core.RequestedTime  `json:"timeFrom,omitempty"`
	TimeTo                  *core.RequestedTime  `json:"timeTo,omitempty"`
	FilterUnit              string               `json:"filterUnit,omitempty"`
	EmptyReason             core.EmptyReason     `json:"emptyReason,omitempty"`
	UnknownFields           []string             `json:"unknownFields,omitempty"`
	UnknownFieldsHint       string               `json:"unknownFieldsHint,omitempty"`
}

type subgraphNode struct {
	Id             string                   `json:"id"`
	Kind           string                   `json:"kind"`
	KeyForm        string                   `json:"keyForm"`
	Identity       []core.NodeIdentityValue `json:"identity"`
	Label          core.RawAndNormalized    `json:"label"`
	Observation    string                   `json:"observation"`
	CreationRecord string                   `json:"creationRecord"`
	Selection      string                   `json:"selection"`
	// AccountName と AccountNameWithheld は、アカウントのノードをまとめる鍵と、持たない理由である。
	AccountName         *core.AccountNameKey           `json:"accountName,omitempty"`
	AccountNameWithheld core.AccountNameWithheldReason `json:"accountNameWithheld,omitempty"`
	// ValueMatches は検索が一致した欄である。検索の文字列を与えない要求では出ない。
	ValueMatches []core.NodeValueMatch `json:"valueMatches,omitempty"`
	// Terminals は根拠のレコードが名乗った端末である。1 つも名乗らないノードでは出ない。
	Terminals []core.GraphNode `json:"terminals,omitempty"`
	// Record はレコードの要約である。recordSummary を与えた要求の、合致したレコードのノードだけが持つ。
	Record *core.RecordSummary `json:"record,omitempty"`
}

type graphEdge struct {
	Id              string          `json:"id"`
	Kind            string          `json:"kind"`
	State           string          `json:"state"`
	SourceNodeId    string          `json:"sourceNodeId"`
	TargetNodeId    string          `json:"targetNodeId"`
	ApplicableRange *core.TimeRange `json:"applicableRange,omitempty"`
	// AssignmentOrigins は、エッジを作った端末の割当の由来である。
	AssignmentOrigins []core.TerminalAssignmentOrigin `json:"assignmentOrigins,omitempty"`
	EvidenceCount     int64                           `json:"evidenceCount"`
	// EvidenceByCase は案件ごとの根拠の件数である。案件を区別しない取り込みでは出ない。
	EvidenceByCase []core.CaseEvidenceCount `json:"evidenceByCase,omitempty"`
}

// graphManifest は fixture と一緒に置いた期待値である。
type graphManifest struct {
	Sources []struct {
		File   string         `json:"file"`
		Format core.FormatKey `json:"format"`
		State  string         `json:"state"`
	} `json:"sources"`
	RecordNodes []struct {
		File         string  `json:"file"`
		PositionKind string  `json:"positionKind"`
		Positions    []int64 `json:"positions"`
	} `json:"recordNodes"`
	Nodes []struct {
		Kind           string   `json:"kind"`
		KeyForm        string   `json:"keyForm"`
		Identity       []string `json:"identity"`
		Label          string   `json:"label"`
		LabelState     string   `json:"labelState"`
		Observation    string   `json:"observation"`
		CreationRecord string   `json:"creationRecord"`
	} `json:"nodes"`
	Edges []struct {
		Kind          string `json:"kind"`
		State         string `json:"state"`
		Source        int    `json:"source"`
		Target        int    `json:"target"`
		EvidenceCount int64  `json:"evidenceCount"`
	} `json:"edges"`
	ProcessDetail struct {
		NodeKind      string   `json:"nodeKind"`
		NodeIdentity  []string `json:"nodeIdentity"`
		EvidenceCount int64    `json:"evidenceCount"`
		Attributes    []struct {
			Semantic          core.SemanticKey `json:"semantic"`
			ValueCount        int64            `json:"valueCount"`
			ObservationCounts []int64          `json:"observationCounts"`
		} `json:"attributes"`
		EdgeCounts []core.NodeEdgeCount `json:"edgeCounts"`
	} `json:"processDetail"`
}

func graphFixtures(t *testing.T) graphManifest {
	t.Helper()
	data, err := os.ReadFile(runFixtureDir + "graph-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest graphManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Sources) == 0 || len(manifest.Nodes) == 0 || len(manifest.Edges) == 0 {
		t.Fatalf("graph manifest sources=%d nodes=%d edges=%d, want every collection populated",
			len(manifest.Sources), len(manifest.Nodes), len(manifest.Edges))
	}
	return manifest
}

func graphImportResult(t *testing.T) pipeline.ImportResult {
	t.Helper()
	manifest := graphFixtures(t)
	plans := make([]pipeline.SourcePlan, len(manifest.Sources))
	for i, source := range manifest.Sources {
		plans[i] = pipeline.SourcePlan{
			FormatKey: source.Format, FileName: source.File, OriginPath: "testdata/" + source.File,
		}
	}
	return importResultOfFiles(t, plans...)
}

func graphHandler(t *testing.T) http.Handler {
	t.Helper()
	return testHandler(graphImportResult(t))
}

func requestGraph(t *testing.T, handler http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(graphPath+"?"+query), nil))
	return response
}

func decodeGraph(t *testing.T, handler http.Handler, query string) graphResponse {
	t.Helper()
	response := requestGraph(t, handler, query)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded graphResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

func requestGraphError(t *testing.T, handler http.Handler, query string, wantStatus int) core.ApiError {
	t.Helper()
	response := requestGraph(t, handler, query)
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	return apiError
}

// wantRecordNode は fixture から導いたレコードのノード 1 つの期待値である。
type wantRecordNode struct {
	identity []string
	label    string
}

// manifestRecordNodes は manifest から、期待するレコードのノードを組む。
// 識別鍵の先頭は収集元の内容の識別であり、fixture の byte 列の sha256 を 16 進にした値である。
func manifestRecordNodes(t *testing.T, manifest graphManifest) []wantRecordNode {
	t.Helper()
	var wanted []wantRecordNode
	for _, source := range manifest.RecordNodes {
		content, err := os.ReadFile(runFixtureDir + source.File)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		digest := hex.EncodeToString(sum[:])
		// 表示名の位置の名前は、通番が ID、行番号が行である。
		positionName := map[string]string{"sequence_number": "ID", "line_number": "行"}[source.PositionKind]
		if positionName == "" {
			t.Fatalf("the manifest declares the position kind %q without a label name", source.PositionKind)
		}
		for _, position := range source.Positions {
			text := strconv.FormatInt(position, 10)
			wanted = append(wanted, wantRecordNode{
				identity: []string{digest, source.PositionKind, text},
				label:    source.File + " " + positionName + " " + text,
			})
		}
	}
	if len(wanted) == 0 {
		t.Fatal("the manifest declares no record node")
	}
	return wanted
}

func objectNodesOf(page graphResponse) []subgraphNode {
	nodes := make([]subgraphNode, 0, len(page.Nodes))
	for _, node := range page.Nodes {
		if node.Kind == string(core.NodeKindRecord) {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// objectEdgesOf は応答のエッジから、レコードが対象を指す関係を外した並びを返す。
func objectEdgesOf(page graphResponse) []graphEdge {
	edges := make([]graphEdge, 0, len(page.Edges))
	for _, edge := range page.Edges {
		if edge.Kind == string(core.EdgeKindRecordNamesObject) {
			continue
		}
		edges = append(edges, edge)
	}
	return edges
}

func TestGraphEndpointReturnsTheObservedSubgraph(t *testing.T) {
	manifest := graphFixtures(t)
	page := decodeGraph(t, graphHandler(t), wholeGraphQuery)
	// 全体が 1 ページに入る要求であるため、数えた総数と返した要素数は一致する。
	if page.NodeCount != int64(len(page.Nodes)) || page.EdgeCount != int64(len(page.Edges)) {
		t.Fatalf("nodeCount=%d edgeCount=%d carry %d nodes and %d edges",
			page.NodeCount, page.EdgeCount, len(page.Nodes), len(page.Edges))
	}
	objectNodes := objectNodesOf(page)
	objectEdges := objectEdgesOf(page)
	if len(objectNodes) != len(manifest.Nodes) {
		t.Fatalf("the response carries %d object nodes, want %d",
			len(objectNodes), len(manifest.Nodes))
	}
	for index, want := range manifest.Nodes {
		node := objectNodes[index]
		if node.Kind != want.Kind || node.KeyForm != want.KeyForm {
			t.Errorf("node %d is %s/%s, want %s/%s", index, node.Kind, node.KeyForm,
				want.Kind, want.KeyForm)
		}
		if node.Selection != string(core.NodeSelectionMatched) {
			t.Errorf("node %d carries the selection %q, want matched", index, node.Selection)
		}
		if node.Observation != want.Observation ||
			node.CreationRecord != want.CreationRecord {
			t.Errorf("node %d carries %q/%q, want %q/%q", index,
				node.Observation, node.CreationRecord, want.Observation, want.CreationRecord)
		}
		assertIdentityValues(t, index, node.Identity, want.Identity)
		assertLabel(t, index, node.Label, want.Label, want.LabelState)
	}
	if len(objectEdges) != len(manifest.Edges) {
		t.Fatalf("the response carries %d edges between objects, want %d",
			len(objectEdges), len(manifest.Edges))
	}
	for index, want := range manifest.Edges {
		edge := objectEdges[index]
		if edge.Kind != want.Kind || edge.State != want.State {
			t.Errorf("edge %d is %s/%s, want %s/%s", index, edge.Kind, edge.State,
				want.Kind, want.State)
		}
		if edge.SourceNodeId != objectNodes[want.Source].Id {
			t.Errorf("edge %d starts at %q, want the node %d", index, edge.SourceNodeId, want.Source)
		}
		if edge.TargetNodeId != objectNodes[want.Target].Id {
			t.Errorf("edge %d ends at %q, want the node %d", index, edge.TargetNodeId, want.Target)
		}
		if edge.EvidenceCount != want.EvidenceCount {
			t.Errorf("edge %d carries %d evidence records, want %d",
				index, edge.EvidenceCount, want.EvidenceCount)
		}
	}
}

func assertIdentityValues(t *testing.T, index int, got []core.NodeIdentityValue, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("node %d carries %d identity values, want %d", index, len(got), len(want))
	}
	for at, value := range want {
		if got[at].Value != value {
			t.Errorf("node %d identity %d is %q, want %q", index, at, got[at].Value, value)
		}
	}
}

func assertLabel(t *testing.T, index int, got core.RawAndNormalized, want, wantState string) {
	t.Helper()
	if string(got.ValueState) != wantState {
		t.Errorf("node %d label state is %q, want %q", index, got.ValueState, wantState)
	}
	value, readable := got.ComparableValue()
	// 表示名を持たないノードは item_absent を持ち、比べられる値を持たない。
	if wantState == string(core.ValueStateItemAbsent) {
		if readable {
			t.Errorf("node %d label is %q, want no comparable value", index, value)
		}
		return
	}
	if !readable || value != want {
		t.Errorf("node %d label is %q (readable %v), want %q", index, value, readable, want)
	}
}

// レコードのノードは原文の 1 行につき 1 つあり、識別鍵と表示名を収集元の内容の識別と
// 位置から組む。
func TestGraphEndpointCarriesEveryRecordAsANode(t *testing.T) {
	manifest := graphFixtures(t)
	page := decodeGraph(t, graphHandler(t), wholeGraphQuery)
	carried := make(map[string]subgraphNode)
	for _, node := range page.Nodes {
		if node.Kind != string(core.NodeKindRecord) {
			continue
		}
		if node.KeyForm != string(core.NodeKeyFormSourceContentPosition) {
			t.Errorf("the record node %q carries the key form %q, want %q",
				node.Id, node.KeyForm, core.NodeKeyFormSourceContentPosition)
		}
		if node.CreationRecord != string(core.NodeCreationRecordItemAbsent) {
			t.Errorf("the record node %q carries the creation record %q, want item_absent",
				node.Id, node.CreationRecord)
		}
		if node.Observation != string(core.NodeObservationObserved) {
			t.Errorf("the record node %q carries the observation %q, want observed",
				node.Id, node.Observation)
		}
		values := make([]string, 0, len(node.Identity))
		for _, item := range node.Identity {
			values = append(values, item.Value)
		}
		carried[strings.Join(values, "\x00")] = node
	}
	wanted := manifestRecordNodes(t, manifest)
	if len(carried) != len(wanted) {
		t.Fatalf("the response carries %d record nodes, want %d", len(carried), len(wanted))
	}
	for _, want := range wanted {
		node, present := carried[strings.Join(want.identity, "\x00")]
		if !present {
			t.Errorf("the response carries no record node with the identity %v", want.identity)
			continue
		}
		label, readable := node.Label.ComparableValue()
		if !readable || label != want.label {
			t.Errorf("the record node %v carries the label %q (readable %v), want %q",
				want.identity, label, readable, want.label)
		}
		if node.Label.ValueState != core.ValueStateDerived {
			t.Errorf("the record node %v carries the value state %q, want derived",
				want.identity, node.Label.ValueState)
		}
	}
}

// **応答の一部を切る上限と続きを取る位置を受けない。** 絞り込んだ結果を全件返す。nodeLimit は
// 図の本体を返すかだけを決める。綴りを通知せずに既定値で処理しないよう、未知の項目として退ける。
func TestGraphEndpointRejectsTheLimitAndTheCursor(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name  string
		query string
	}{
		{"edge limit", "depth=0&edgeLimit=200"},
		{"evidence limit", "depth=0&evidenceLimit=1"},
		{"value limit", "depth=0&countBy=process.binary_path&valueLimit=50"},
		{"limit", "depth=0&limit=100"},
		{"cursor", "depth=0&cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			apiError := requestGraphError(t, handler, testCase.query, http.StatusBadRequest)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", apiError.Code)
			}
		})
	}
}

func TestGraphEndpointFiltersByTheRequestedConditions(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name      string
		query     string
		wantNodes int64
		wantEdges int64
	}{
		// 対象のノード 25 件とレコードのノード 21 件、対象どうしの関係 31 本と
		// 対象を指す関係 73 本である。
		{"every node", wholeGraphQuery, 46, 104},
		{"terminals only", "depth=0&nodeKind=terminal", 2, 0},
		{"file operations", "depth=1&edgeKind=file_operation", 46, 2},
		// 2 つの種別の関係は、ファイルの操作の 2 本とコピーの 1 本の和である。
		{"file operations and copies", "depth=1&edgeKind=file_operation&edgeKind=file_copy", 46, 3},
		// copy のレコードは 1 件で、指す対象は端末・プロセス・元と先のファイル・IP の
		// 5 つである。エッジは対象どうしの 4 本と、レコードから対象を指す関係 5 本である。起点は
		// レコードと、端末を除く 4 つの対象である。
		{"a copy of a file", "depth=1&eventCategory=file&eventAction=copy", 5, 9},
		{"records only", "depth=0&nodeKind=record", 21, 0},
		{"an action the source does not carry", "depth=1&eventCategory=file&eventAction=del", 0, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			page := decodeGraph(t, handler, testCase.query)
			if page.NodeCount != testCase.wantNodes || page.EdgeCount != testCase.wantEdges {
				t.Fatalf("nodeCount=%d edgeCount=%d want %d and %d",
					page.NodeCount, page.EdgeCount, testCase.wantNodes, testCase.wantEdges)
			}
			if testCase.wantNodes == 0 && page.EmptyReason != core.EmptyReasonNoRecordInFilter {
				t.Errorf("emptyReason=%q want no_record_in_filter", page.EmptyReason)
			}
			if testCase.wantNodes != 0 && page.EmptyReason != "" {
				t.Errorf("emptyReason=%q want no value", page.EmptyReason)
			}
		})
	}
}

func TestGraphEndpointReturnsTheRequestedConditions(t *testing.T) {
	page := decodeGraph(t, graphHandler(t), "depth=1&nodeKind=process&edgeKind=ran_on"+
		"&edgeKind=file_operation"+
		"&eventCategory=ps&eventAction=start&timeFrom=2000-02-01T13:00:00%2B09:00"+
		"&timeFromPrecision=second&filterUnit=second"+
		"")
	// 関係の種別は要求に書いた順のまま返す。
	if !slices.Equal(page.NodeKinds, []string{"process"}) ||
		!slices.Equal(page.EdgeKinds, []string{"ran_on", "file_operation"}) || page.Depth != 1 {
		t.Errorf("the response returned nodeKinds=%q edgeKinds=%q depth=%d, want process/[ran_on file_operation]/1",
			page.NodeKinds, page.EdgeKinds, page.Depth)
	}
	if page.EventCategory != "ps" || page.EventAction != "start" || page.FilterUnit != "second" {
		t.Errorf("the response returned eventCategory=%q eventAction=%q filterUnit=%q",
			page.EventCategory, page.EventAction, page.FilterUnit)
	}
	if page.TimeFrom == nil || page.TimeFrom.RequestText != "2000-02-01T13:00:00+09:00" {
		t.Fatalf("the response returned timeFrom=%+v", page.TimeFrom)
	}
	if page.TimeTo != nil {
		t.Errorf("the response returned timeTo=%+v, want no value", page.TimeTo)
	}
	if page.EdgeCount != 5 {
		t.Errorf("the response counted %d ran_on edges of process start records, want 5",
			page.EdgeCount)
	}
}

// 条件を起点だけに当てた要求は、そのことを応答で返す。エッジの根拠の件数の意味が変わる。
func TestGraphEndpointReturnsConditionsOnOriginsOnly(t *testing.T) {
	for _, suffix := range []string{"", "&conditionsOnOriginsOnly=true"} {
		page := decodeGraph(t, graphHandler(t), "depth=1&eventCategory=ps"+suffix)
		if page.ConditionsOnOriginsOnly != (suffix != "") {
			t.Errorf("%q: the response returned conditionsOnOriginsOnly=%v", suffix,
				page.ConditionsOnOriginsOnly)
		}
	}
}

// 両端のレコードを期間で判定した要求は、そのことを応答で返す。
func TestGraphEndpointReturnsEndpointRecordsInPeriod(t *testing.T) {
	for _, suffix := range []string{"", "&endpointRecordsInPeriod=true"} {
		page := decodeGraph(t, graphHandler(t), "depth=1&eventCategory=ps"+suffix)
		if page.EndpointRecordsInPeriod != (suffix != "") {
			t.Errorf("%q: the response returned endpointRecordsInPeriod=%v", suffix,
				page.EndpointRecordsInPeriod)
		}
	}
}

func TestGraphEndpointExpandsTheNeighbourhoodOfANode(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	origin := whole.Nodes[1]
	if origin.Kind != "process" {
		t.Fatalf("the second node is a %q, want a process", origin.Kind)
	}
	page := decodeGraph(t, handler,
		"depth=1&nodeId="+origin.Id+"")
	if page.NodeCount != 1 || !slices.Equal(page.NodeIds, []string{origin.Id}) {
		t.Fatalf("nodeCount=%d nodeIds=%q want 1 and [%q]", page.NodeCount, page.NodeIds, origin.Id)
	}
	// プロセスに繋がるのは ran_on 1 本と file_operation 2 本と registry_operation 1 本と
	// process_communication 1 本と、起点になる process_parent_child 1 本と、終点になる
	// process_parent_child 1 本と process_executable 1 本と cross_source_connection_match 1 本と、
	// このプロセスを記録した 10 件のレコードからの record_names_object 10 本である。
	if page.EdgeCount != 19 {
		t.Fatalf("the neighbourhood carries %d edges, want 19", page.EdgeCount)
	}
	// 端点は端末・ファイル 2 件・実行ファイル・レジストリの値・接続先 IP・親のプロセス・
	// 子のプロセスの 8 つと、レコード 10 件である。候補のエッジの起点は接続先 IP と同じノードである。
	if len(page.Nodes) != 19 {
		t.Fatalf("the neighbourhood carries %d nodes, want 19", len(page.Nodes))
	}
}

// graphNodeWithIdentity は識別鍵の値で応答のノードを 1 つ選ぶ。
func graphNodeWithIdentity(t *testing.T, page graphResponse, want ...string) subgraphNode {
	t.Helper()
	for _, node := range page.Nodes {
		values := make([]string, 0, len(node.Identity))
		for _, item := range node.Identity {
			values = append(values, item.Value)
		}
		if slices.Equal(values, want) {
			return node
		}
	}
	t.Fatalf("no node carries the identity %v", want)
	return subgraphNode{}
}

// assertGraphCarriesNodes は応答のノードの集合が、期待した識別子の集合と一致することを
// 確かめる。
func assertGraphCarriesNodes(t *testing.T, page graphResponse, want ...string) {
	t.Helper()
	carried := make([]string, 0, len(page.Nodes))
	for _, node := range page.Nodes {
		carried = append(carried, node.Id)
	}
	slices.Sort(carried)
	expected := slices.Clone(want)
	slices.Sort(expected)
	if !slices.Equal(carried, expected) {
		t.Errorf("the response carries the nodes %v, want %v", carried, expected)
	}
}

// assertGraphCarriesParentChildEdges は応答のエッジの集合が、期待した親子の組と一致する
// ことを確かめる。組は (起点のノード, 終点のノード) で指定する。
func assertGraphCarriesParentChildEdges(t *testing.T, page graphResponse, want ...[2]string) {
	t.Helper()
	carried := make([]string, 0, len(page.Edges))
	for _, edge := range page.Edges {
		carried = append(carried, edge.Kind+" "+edge.SourceNodeId+" "+edge.TargetNodeId)
	}
	expected := make([]string, 0, len(want))
	for _, pair := range want {
		expected = append(expected,
			string(core.EdgeKindProcessParentChild)+" "+pair[0]+" "+pair[1])
	}
	slices.Sort(carried)
	slices.Sort(expected)
	if !slices.Equal(carried, expected) {
		t.Errorf("the response carries the edges %v, want %v", carried, expected)
	}
}

// TestGraphEndpointFollowsTheAncestorChainBeyondOneStep は、1 ホップを超えて祖先を辿る要求が
// 連鎖の全体と、連鎖が途切れる位置を 1 回の応答で返すことを確かめる。
//
// fixture の連鎖は {P0} が {P1} を、{P1} が {P7} を起動した並びである。{P0} の起動を
// 記録したレコードは収集元に無く、その位置で祖先を辿れなくなる。
func TestGraphEndpointFollowsTheAncestorChainBeyondOneStep(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	origin := graphNodeWithIdentity(t, whole, "T1", "{P7}")
	parent := graphNodeWithIdentity(t, whole, "T1", "{P1}")
	ancestor := graphNodeWithIdentity(t, whole, "T1", "{P0}")
	chainQuery := "&edgeKind=process_parent_child&nodeId=" + origin.Id +
		""

	oneStep := decodeGraph(t, handler, "depth=1"+chainQuery)
	assertGraphCarriesNodes(t, oneStep, origin.Id, parent.Id)
	assertGraphCarriesParentChildEdges(t, oneStep, [2]string{parent.Id, origin.Id})

	twoSteps := decodeGraph(t, handler, "depth=2"+chainQuery)
	assertGraphCarriesNodes(t, twoSteps, origin.Id, parent.Id, ancestor.Id)
	assertGraphCarriesParentChildEdges(t, twoSteps,
		[2]string{parent.Id, origin.Id}, [2]string{ancestor.Id, parent.Id})
	if twoSteps.Depth != 2 {
		t.Errorf("the response returned depth=%d, want 2", twoSteps.Depth)
	}
	// **連鎖が途切れる位置は、生成を記録したレコードを持たない位置である。**
	broken := graphNodeWithIdentity(t, twoSteps, "T1", "{P0}")
	if broken.CreationRecord != string(core.NodeCreationRecordAbsent) ||
		broken.Observation != string(core.NodeObservationReferenced) {
		t.Errorf("the ancestor carries creationRecord=%q observation=%q, want absent and referenced",
			broken.CreationRecord, broken.Observation)
	}
	// 段数を上限まで上げても、連鎖は生成のレコードが無い位置より先へ伸びない。
	maxSteps := decodeGraph(t, handler, "depth=8"+chainQuery)
	assertGraphCarriesNodes(t, maxSteps, origin.Id, parent.Id, ancestor.Id)
	assertGraphCarriesParentChildEdges(t, maxSteps,
		[2]string{parent.Id, origin.Id}, [2]string{ancestor.Id, parent.Id})
}

// TestGraphEndpointFiltersAddressNodesByRange は、アドレスの範囲の内と外のそれぞれで
// 残る IP アドレスのノードを確かめる。
func TestGraphEndpointFiltersAddressNodesByRange(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	localFirst := graphNodeWithIdentity(t, whole, "192.0.2.1").Id
	localSecond := graphNodeWithIdentity(t, whole, "192.0.2.9").Id
	remote := graphNodeWithIdentity(t, whole, "198.51.100.7").Id
	limits := "&depth=0&nodeKind=ip"

	inside := decodeGraph(t, handler, "addressInCidr=192.0.2.0%2F24"+limits)
	assertGraphCarriesNodes(t, inside, localFirst, localSecond)
	if inside.AddressInCidr != "192.0.2.0/24" {
		t.Errorf("the response returned addressInCidr=%q, want the requested range",
			inside.AddressInCidr)
	}
	outside := decodeGraph(t, handler, "addressNotInCidr=192.0.2.0%2F24"+limits)
	assertGraphCarriesNodes(t, outside, remote)
	if outside.AddressNotInCidr != "192.0.2.0/24" {
		t.Errorf("the response returned addressNotInCidr=%q, want the requested range",
			outside.AddressNotInCidr)
	}
	// 範囲の中の値を持つ文字列は、その長さが表す範囲と同じ結果になる。
	withHost := decodeGraph(t, handler, "addressNotInCidr=192.0.2.9%2F24"+limits)
	assertGraphCarriesNodes(t, withHost, remote)
	// 2 つの範囲を重ねた要求は、両方を満たすノードだけを返す。
	both := decodeGraph(t, handler,
		"addressInCidr=192.0.2.0%2F24&addressNotInCidr=192.0.2.1%2F32"+limits)
	assertGraphCarriesNodes(t, both, localSecond)
	// どのアドレスも通らない範囲は、合致するノードを持たない理由を持つ。
	none := decodeGraph(t, handler, "addressInCidr=203.0.113.0%2F24"+limits)
	assertGraphCarriesNodes(t, none)
	if none.EmptyReason != core.EmptyReasonNoRecordInFilter {
		t.Errorf("emptyReason=%q want no_record_in_filter", none.EmptyReason)
	}
}

// TestGraphEndpointKeepsTheDestinationsOutsideTheRange は、プロセスを起点に接続先を
// 辿る要求が、アドレスの範囲で接続先を絞ることを確かめる。
//
// **起点のプロセスは範囲で除かれない。** 範囲で絞るのは IP アドレスのノードだけである。
func TestGraphEndpointKeepsTheDestinationsOutsideTheRange(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	origin := graphNodeWithIdentity(t, whole, "T1", "{P1}")
	destination := graphNodeWithIdentity(t, whole, "198.51.100.7").Id
	communication := "&depth=1&edgeKind=process_communication&nodeId=" + origin.Id +
		""

	outside := decodeGraph(t, handler, "addressNotInCidr=192.0.2.0%2F24"+communication)
	assertGraphCarriesNodes(t, outside, origin.Id, destination)
	inside := decodeGraph(t, handler, "addressNotInCidr=198.51.100.0%2F24"+communication)
	assertGraphCarriesNodes(t, inside, origin.Id)
	if inside.EdgeCount != 0 {
		t.Errorf("the response carries %d edges to the excluded destination, want none",
			inside.EdgeCount)
	}
}

// 値の検索は、一致した欄と値の形を返し、その値を持つ対象を部分グラフとして返す。
func TestGraphEndpointReturnsTheSubgraphOfASearchedValue(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	// fixture の 2 行目と 3 行目が C:\data\secret.txt を記録する。
	// Windows の path のファイルは、小文字にそろえた path を識別鍵に持つ。
	file := graphNodeWithIdentity(t, whole, "T1", `c:\data\secret.txt`).Id
	page := decodeGraph(t, handler,
		"valueContains=secret.txt&depth=1")
	if !slices.Equal(page.ValueContains, []string{"secret.txt"}) {
		t.Errorf("the response returned valueContains=%q, want the requested token",
			page.ValueContains)
	}
	carried := make(map[string]struct{}, len(page.Nodes))
	matched := 0
	for _, node := range page.Nodes {
		carried[node.Id] = struct{}{}
		if node.Selection != string(core.NodeSelectionMatched) {
			continue
		}
		matched++
		if len(node.ValueMatches) == 0 {
			t.Errorf("the matched node %q carries no value match", node.Id)
		}
		for _, hit := range node.ValueMatches {
			if (hit.Semantic == "") == (hit.Name == "") {
				t.Errorf("the value match %+v carries both names or neither", hit)
			}
			if !hit.Form.IsKnown() {
				t.Errorf("the value match carries the form %q", hit.Form)
			}
		}
	}
	if matched == 0 {
		t.Fatal("the search matched no node, want the records naming the file")
	}
	// **一致した値を持つファイルのノードへ、記録したレコードから 1 ホップで届く。**
	if _, present := carried[file]; !present {
		t.Error("the response carries no file node for the searched value")
	}
}

// assertValueCounts は応答の値ごとの件数が、期待した値と件数の組と一致することを確かめる。
func assertValueCounts(t *testing.T, page graphResponse, want map[string]int64) {
	t.Helper()
	got := make(map[string]int64, len(page.ValueCounts))
	for _, counted := range page.ValueCounts {
		got[counted.Value] = counted.RecordCount
	}
	if len(got) != len(want) {
		t.Fatalf("the response counts %v, want %v", got, want)
	}
	for value, count := range want {
		if got[value] != count {
			t.Errorf("the value %q was observed by %d records, want %d",
				value, got[value], count)
		}
	}
	if page.DistinctValueCount == nil || *page.DistinctValueCount != int64(len(want)) {
		t.Errorf("the response counted %v distinct values, want %d",
			page.DistinctValueCount, len(want))
	}
	// **数えた個数と応答へ入れた要素数が一致する。** 値ごとの件数は全件を返す。
	if page.DistinctValueCount != nil &&
		*page.DistinctValueCount != int64(len(page.ValueCounts)) {
		t.Errorf("the response counted %d distinct values and returned %d",
			*page.DistinctValueCount, len(page.ValueCounts))
	}
}

// 値ごとの件数は、語彙の項目と原資料の key のどちらでも数える欄を指せる。
// 数える範囲は絞り込みに合うノードの全件であり、応答へ入れたページではない。
func TestGraphEndpointCountsTheValuesOfAField(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name  string
		query string
		want  map[string]int64
	}{
		{
			// 期待値は fixture の原文の psPath から読んで決める。
			name:  "a field the vocabulary carries",
			query: "countBy=process.binary_path",
			want: map[string]int64{
				`C:\app.exe`: 9, `C:\app2.exe`: 1, `C:\child6.exe`: 1,
				`C:\late.exe`: 1, `C:\child7.exe`: 1, `C:\net8.exe`: 2,
			},
		},
		{
			// tmid は語彙の項目 terminal.id を持つ。原資料の key を指した要求は、
			// 語彙へ写した欄も数える。
			name:  "a key the vocabulary carries as a semantic",
			query: "countBy=tmid",
			want:  map[string]int64{"T1": 15, "T2": 1},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			whole := decodeGraph(t, handler,
				testCase.query+"&depth=0")
			assertValueCounts(t, whole, testCase.want)
			// **上限 1 のページでも同じ件数を返す。** 数える範囲はページではない。
			page := decodeGraph(t, handler,
				testCase.query+"&depth=0")
			assertValueCounts(t, page, testCase.want)
		})
	}
}

func TestGraphEndpointSeparatesTheEmptyReasonsOfTheSearch(t *testing.T) {
	handler := graphHandler(t)
	limits := "&depth=0"
	for _, testCase := range []struct {
		name  string
		query string
		want  core.EmptyReason
	}{
		{
			"a token no field carries",
			"valueContains=no-such-token-in-the-fixture" + limits,
			core.EmptyReasonNoValueMatch,
		},
		{
			// 文字列を照合する欄の名前がどのレコードにも無い。打ち間違いがこれに該当する。
			"a field name no record carries",
			"valueContains=secret.txt&valueField=pathh" + limits,
			core.EmptyReasonNoFieldObserved,
		},
		{
			// 欄はあるが、その欄の値が文字列を含まない。
			"a field without the token",
			"valueContains=secret.txt&valueField=psPath" + limits,
			core.EmptyReasonNoValueMatch,
		},
		{
			// 文字列は fixture にあるが、種別の絞り込みで残らない。
			// **どこにも無いと名乗らない。**
			"a token the filter removes",
			"valueContains=secret.txt&nodeKind=account" + limits,
			core.EmptyReasonValueMatchOutsideFilter,
		},
		{
			"an action the source does not carry",
			"eventCategory=file&eventAction=del" + limits,
			core.EmptyReasonNoRecordInFilter,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			page := decodeGraph(t, handler, testCase.query)
			if page.NodeCount != 0 {
				t.Fatalf("nodeCount=%d, want none", page.NodeCount)
			}
			if page.EmptyReason != testCase.want {
				t.Errorf("emptyReason=%q want %q", page.EmptyReason, testCase.want)
			}
		})
	}
}

// 合うレコードが端末のほかに対象を指さない 0 件は、検索の文字列の理由より先に判定する。
func TestGraphEndpointReportsARecordMatchWithoutObjectBeforeTheSearchReasons(t *testing.T) {
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "graph-record-only.log", OriginPath: "testdata/graph-record-only.log",
	}))
	page := decodeGraph(t, handler, "valueContains=scan-finished&eventCategory=session&granularity=object&depth=0")
	if page.NodeCount != 0 {
		t.Fatalf("nodeCount=%d, want none", page.NodeCount)
	}
	if page.EmptyReason != core.EmptyReasonRecordMatchWithoutObject {
		t.Errorf("emptyReason=%q want %q", page.EmptyReason, core.EmptyReasonRecordMatchWithoutObject)
	}
}

// 種別とアドレスの範囲で対象が残らない 0 件は、合うレコードが対象を指すため、ほかの理由を返す。
func TestGraphEndpointKeepsTheOtherReasonsWhenTheRecordPointsToAnObject(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name  string
		query string
		want  core.EmptyReason
	}{
		{
			"an address outside the range",
			"eventCategory=net&nodeKind=ip&addressInCidr=203.0.113.0/24&granularity=object&depth=0",
			core.EmptyReasonNoRecordInFilter,
		},
		{
			"a token the kind filter removes",
			"valueContains=secret.txt&nodeKind=account&granularity=object&depth=0",
			core.EmptyReasonValueMatchOutsideFilter,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			page := decodeGraph(t, handler, testCase.query)
			if page.NodeCount != 0 {
				t.Fatalf("nodeCount=%d, want none", page.NodeCount)
			}
			if page.EmptyReason != testCase.want {
				t.Errorf("emptyReason=%q want %q", page.EmptyReason, testCase.want)
			}
		})
	}
}

// 検索の結果のレコードのノードから、`/api/v0/records` が求める位置の指定へ渡れる。
func TestGraphEndpointLeadsFromASearchedRecordToItsText(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler,
		"valueContains=secret.txt&nodeKind=record&depth=0")
	if page.NodeCount == 0 {
		t.Fatal("the search matched no record node")
	}
	detail := decodeNode(t, handler, url.PathEscape(page.Nodes[0].Id), "")
	if len(detail.Evidence) == 0 {
		t.Fatal("the record node carries no evidence, want its own record")
	}
	// `/api/v0/records` の要求は収集元の識別と位置の指定を求める
	// (backend/api/records_request.go の missingRecordsParameters)。
	locator := detail.Evidence[0].RecordRef
	if locator.SourceId == "" || locator.SourceContentSha256 == "" {
		t.Fatalf("the evidence carries the locator %+v, want the source identity", locator)
	}
	if locator.SequenceNumber == nil && locator.LineNumber == nil {
		t.Fatalf("the evidence carries the locator %+v, want a position", locator)
	}
}

func TestGraphEndpointRejectsAnUnknownNodeId(t *testing.T) {
	apiError := requestGraphError(t, graphHandler(t),
		"depth=1&nodeId=n:terminal:absent",
		http.StatusNotFound)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", apiError.Code)
	}
}

// 起点を繰り返して与えると、どの起点も合ったノードになり、応答は起点を書いた順に返す。
// 起点の 1 つでもグラフに無ければ退け、起点の数が上限を超えれば退ける。
func TestGraphEndpointAcceptsSeveralOriginsWithinTheLimit(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	if len(whole.Nodes) < 2 {
		t.Fatal("the fixture carries fewer than two nodes")
	}
	first, second := whole.Nodes[0].Id, whole.Nodes[1].Id
	query := "depth=1&nodeId=" + url.QueryEscape(second) +
		"&nodeId=" + url.QueryEscape(first)

	page := decodeGraph(t, handler, query)
	if page.NodeCount != 2 || !slices.Equal(page.NodeIds, []string{second, first}) {
		t.Fatalf("nodeCount=%d nodeIds=%q want 2 and [%q %q]",
			page.NodeCount, page.NodeIds, second, first)
	}

	apiError := requestGraphError(t, handler, query+"&nodeId=n:terminal:absent",
		http.StatusNotFound)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", apiError.Code)
	}

	// 上限ちょうどの数は受け取り、1 つ超えると退ける。上限は要求の型が定める 32 である。
	const originLimit = 32
	atLimit := "depth=1" +
		strings.Repeat("&nodeId="+url.QueryEscape(first), originLimit)
	if page := decodeGraph(t, handler, atLimit); len(page.NodeIds) != originLimit {
		t.Errorf("nodeIds at the limit = %d, want %d", len(page.NodeIds), originLimit)
	}
	tooMany := atLimit + "&nodeId=" + url.QueryEscape(first)
	apiError = requestGraphError(t, handler, tooMany, http.StatusBadRequest)
	if apiError.Code != core.ApiErrorCodeInvalidRequest {
		t.Errorf("code=%q want invalid_request", apiError.Code)
	}
}

// 部分グラフのノードの数が nodeLimit に収まる要求は、図の本体を返し、件数を数える。
func TestGraphEndpointDrawsTheSubgraphWithinTheNodeLimit(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	if whole.SubgraphNodeCount != int64(len(whole.Nodes)) || whole.NodeLimitExceeded ||
		whole.EdgeKindCounts != nil {
		t.Fatalf("the request without a limit counts %d nodes for %d, exceeded=%v kinds=%v",
			whole.SubgraphNodeCount, len(whole.Nodes), whole.NodeLimitExceeded, whole.EdgeKindCounts)
	}
	page := decodeGraph(t, handler, wholeGraphQuery+"&nodeLimit="+strconv.Itoa(len(whole.Nodes)))
	if page.NodeLimitExceeded || len(page.Nodes) != len(whole.Nodes) ||
		len(page.Edges) != len(whole.Edges) || page.NodeLimit != len(whole.Nodes) {
		t.Errorf("the limit equal to the count returns %d nodes and %d edges (exceeded=%v limit=%d), want %d and %d",
			len(page.Nodes), len(page.Edges), page.NodeLimitExceeded, page.NodeLimit,
			len(whole.Nodes), len(whole.Edges))
	}
}

// 辿るエッジが 0 本で nodeLimit を超える応答も、edgeKindCounts を空の配列で出す。
func TestGraphEndpointWritesEmptyEdgeKindCountsBeyondTheNodeLimit(t *testing.T) {
	response := httptest.NewRecorder()
	graphHandler(t).ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(graphPath+"?depth=0&nodeLimit=1"), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["nodeLimitExceeded"]) != "true" || string(body["edgeKindCounts"]) != "[]" {
		t.Errorf("nodeLimitExceeded=%s edgeKindCounts=%s, want true and []",
			body["nodeLimitExceeded"], body["edgeKindCounts"])
	}
}

// 部分グラフのノードの数が nodeLimit を超える要求は、合ったノードと、関係の種別ごとのエッジの
// 本数を返し、エッジを返さない。
func TestGraphEndpointReturnsTheMatchedNodesBeyondTheNodeLimit(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, "depth=1&nodeKind=terminal")
	if whole.SubgraphNodeCount <= whole.NodeCount {
		t.Fatalf("the fixture yields %d nodes for %d matched; the check needs an endpoint",
			whole.SubgraphNodeCount, whole.NodeCount)
	}
	page := decodeGraph(t, handler, "depth=1&nodeKind=terminal&nodeLimit="+
		strconv.FormatInt(whole.NodeCount, 10))
	if !page.NodeLimitExceeded || page.SubgraphNodeCount != whole.SubgraphNodeCount {
		t.Fatalf("exceeded=%v subgraphNodeCount=%d, want exceeded with %d",
			page.NodeLimitExceeded, page.SubgraphNodeCount, whole.SubgraphNodeCount)
	}
	var matched []string
	for _, node := range whole.Nodes {
		if node.Selection == string(core.NodeSelectionMatched) {
			matched = append(matched, node.Id)
		}
	}
	var returned []string
	for _, node := range page.Nodes {
		if node.Selection != string(core.NodeSelectionMatched) {
			t.Errorf("the response carries the %s node %q beyond the limit", node.Selection, node.Id)
		}
		returned = append(returned, node.Id)
	}
	if !slices.Equal(slices.Sorted(slices.Values(returned)), slices.Sorted(slices.Values(matched))) {
		t.Errorf("the response carries %v, want the matched nodes %v", returned, matched)
	}
	if len(page.Edges) != 0 || page.EdgeCount != 0 {
		t.Errorf("the response carries %d edges (edgeCount %d) beyond the limit", len(page.Edges), page.EdgeCount)
	}
	wantKinds := map[core.EdgeKind]int64{}
	for _, edge := range whole.Edges {
		wantKinds[core.EdgeKind(edge.Kind)]++
	}
	gotKinds := map[core.EdgeKind]int64{}
	for index, count := range page.EdgeKindCounts {
		if err := count.Validate(); err != nil {
			t.Errorf("edgeKindCounts[%d]: %v", index, err)
		}
		if index > 0 && page.EdgeKindCounts[index-1].Kind >= count.Kind {
			t.Errorf("edgeKindCounts are not in the ascending order of the kind: %v", page.EdgeKindCounts)
		}
		gotKinds[count.Kind] = count.Count
	}
	if !maps.Equal(gotKinds, wantKinds) {
		t.Errorf("edgeKindCounts=%v, want %v", gotKinds, wantKinds)
	}
}

// 検索の条件と粒度と端末と展開の指定を、応答がそのまま返し、条件どおりに絞る。
func TestGraphEndpointReturnsTheSearchConditions(t *testing.T) {
	handler := graphHandler(t)
	terminals := decodeGraph(t, handler,
		"depth=0&granularity=object&nodeKind=terminal")
	if len(terminals.Nodes) == 0 {
		t.Fatal("the fixture carries no terminal")
	}
	whole := decodeGraph(t, handler, wholeGraphQuery)
	// fixture の sn=2 は C:\data\secret.txt を、sn=3 はそれを E:\secret.txt へ写したことを
	// 端末 T1 で記録する。T2 の行は secret.txt を持たない。
	source := graphNodeWithIdentity(t, whole, "T1", `c:\data\secret.txt`).Id
	copied := graphNodeWithIdentity(t, whole, "T1", `e:\secret.txt`).Id
	terminalT1 := graphNodeWithIdentity(t, terminals, "T1").Id
	terminalT2 := graphNodeWithIdentity(t, terminals, "T2").Id
	query := func(terminal string, excludes ...string) string {
		built := "nodeLimit=50&depth=0&granularity=object&nodeKind=file" +
			"&valueContains=secret.txt&terminal=" + url.QueryEscape(terminal)
		for _, token := range excludes {
			built += "&valueExcludes=" + url.QueryEscape(token)
		}
		return built
	}
	for _, testCase := range []struct {
		name  string
		query string
		want  []string
	}{
		{"the files the searched records name", query(terminalT1), []string{source, copied}},
		// 除く文字列 E:\ は sn=3 にだけある。sn=3 だけが記録する E:\secret.txt が外れる。
		{"a token that removes the copy record", query(terminalT1, `E:\`), []string{source}},
		{"another terminal", query(terminalT2), nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			page := decodeGraph(t, handler, testCase.query)
			var matched []string
			for _, node := range page.Nodes {
				if node.Selection == string(core.NodeSelectionMatched) {
					matched = append(matched, node.Id)
				}
			}
			slices.Sort(matched)
			want := slices.Sorted(slices.Values(testCase.want))
			if !slices.Equal(matched, want) {
				t.Errorf("the search matched %v, want %v", matched, want)
			}
			if page.NodeCount != int64(len(testCase.want)) {
				t.Errorf("nodeCount=%d, want %d", page.NodeCount, len(testCase.want))
			}
		})
	}

	page := decodeGraph(t, handler, query(terminalT1, `E:\`)+"&nodeKind=process")
	if !slices.Equal(page.NodeKinds, []string{"file", "process"}) ||
		page.Granularity != "object" || page.Terminal != terminalT1 ||
		!slices.Equal(page.ValueContains, []string{"secret.txt"}) ||
		!slices.Equal(page.ValueExcludes, []string{`E:\`}) {
		t.Errorf("the response returned nodeKinds=%v granularity=%q terminal=%q "+
			"valueContains=%v valueExcludes=%v", page.NodeKinds, page.Granularity,
			page.Terminal, page.ValueContains, page.ValueExcludes)
	}
	if page.NodeLimit != 50 {
		t.Errorf("the response returned nodeLimit=%d, want 50", page.NodeLimit)
	}
	// sn=2 は secret.txt を読んだプロセス P1 も記録する。
	wantKinds := []core.MatchedKind{
		{Kind: core.NodeKindFile, Count: 1}, {Kind: core.NodeKindProcess, Count: 1},
	}
	if !slices.Equal(page.MatchedKinds, wantKinds) {
		t.Errorf("matchedKinds=%v, want %v", page.MatchedKinds, wantKinds)
	}
	for _, node := range page.Nodes {
		if node.Kind == string(core.NodeKindRecord) {
			t.Errorf("the object granularity carries the record node %q", node.Id)
		}
	}
}

// 検索の文字列は、含む文字列と含まない文字列を合わせて上限の数まで、1 つにつき上限の長さまで受け取る。
func TestGraphEndpointAcceptsTheSearchTokensAtTheMaximum(t *testing.T) {
	page := decodeGraph(t, graphHandler(t), "depth=0"+
		strings.Repeat("&valueContains=a", 8)+strings.Repeat("&valueExcludes=b", 7)+
		"&valueExcludes="+strings.Repeat("x", 1024))
	if len(page.ValueContains)+len(page.ValueExcludes) != 16 {
		t.Errorf("the response returned %d tokens, want the 16 requested",
			len(page.ValueContains)+len(page.ValueExcludes))
	}
}

func TestGraphEndpointRejectsInvalidRequests(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name        string
		query       string
		wantMissing []string
	}{
		{"no depth", "", []string{"depth"}},
		{"no depth with a node limit", "nodeLimit=10", []string{"depth"}},
		{"the removed expansion", "expansion=0&depth=0", nil},
		{"the removed unfolded expansion", "depth=0&expansion=unfolded", nil},
		{"the removed unfold", "depth=0&unfold=grp:a", nil},
		{"the removed expand", "depth=0&expand=n:a", nil},
		{"the removed fit limit", "fitUnitLimit=10&depth=0", nil},
		{"the removed node budget", "nodeBudget=50&depth=0", nil},
		{"a node limit of zero", "nodeLimit=0&depth=0", nil},
		{"a negative node limit", "nodeLimit=-1&depth=0", nil},
		{"a node limit that is not a number", "nodeLimit=some&depth=0", nil},
		{"an empty node limit", "nodeLimit=&depth=0", nil},
		{"depth above the maximum", "depth=9", nil},
		{"depth below the minimum", "depth=-1", nil},
		{"an address range without a prefix length", "depth=0&addressInCidr=192.0.2.0", nil},
		{"an excluded address range without a prefix length", "depth=0&addressNotInCidr=192.0.2.0", nil},
		{"an empty address range", "depth=0&addressInCidr=", nil},
		{"an empty searched token", "depth=0&valueContains=", nil},
		{"an empty counted field", "depth=0&countBy=", nil},
		{"an unknown node kind", "depth=0&nodeKind=session", nil},
		{"an unknown edge kind", "depth=1&edgeKind=copied_to", nil},
		{"an unknown edge kind after a known one", "depth=1&edgeKind=ran_on&edgeKind=copied_to", nil},
		{"an empty edge kind after a known one", "depth=1&edgeKind=ran_on&edgeKind=", nil},
		{"an unsupported parameter", "depth=0&sortKey=event_time", nil},
		{"a repeated parameter", "depth=0&depth=1", nil},
		{"a comparison unit without a time", "depth=0&filterUnit=second", nil},
		{"a time without a precision", "depth=0&timeFrom=2000-02-01T13:00:00%2B09:00&filterUnit=second", []string{"timeFromPrecision"}},
		{"an empty node kind", "depth=0&nodeKind=", nil},
		{"an empty node kind among the repeated kinds", "depth=0&nodeKind=process&nodeKind=", nil},
		{"an empty excluded token", "depth=0&valueExcludes=", nil},
		{"an empty searched token among the repeated tokens", "depth=0&valueContains=a&valueContains=", nil},
		{"an unknown granularity", "depth=0&granularity=session", nil},
		{"a record kind at the object granularity", "depth=0&granularity=object&nodeKind=record", nil},
		{"an empty terminal", "depth=0&terminal=", nil},
		{"an unknown terminal", "depth=0&terminal=n:terminal:absent", nil},
		{"search tokens beyond the maximum", "depth=0" +
			strings.Repeat("&valueContains=a", 9) + strings.Repeat("&valueExcludes=b", 8), nil},
		{"a search token beyond the maximum length", "depth=0&valueExcludes=" +
			strings.Repeat("x", 1025), nil},
		{"an unsupported comparison unit", "depth=0&timeFrom=2000-02-01T13:00:04%2B09:00&timeFromPrecision=second&filterUnit=hour", nil},
		{"a time text without an offset", "depth=0&timeFrom=2000-02-01T13:00:04&timeFromPrecision=second&filterUnit=second", nil},
		{"a fraction the precision does not have", "depth=0&timeFrom=2000-02-01T13:00:04.500%2B09:00&timeFromPrecision=second&filterUnit=second", nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			apiError := requestGraphError(t, handler, testCase.query, http.StatusBadRequest)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", apiError.Code)
			}
			if strings.Join(apiError.MissingParameters, ",") !=
				strings.Join(testCase.wantMissing, ",") {
				t.Errorf("missingParameters=%v want %v",
					apiError.MissingParameters, testCase.wantMissing)
			}
		})
	}
}

func TestGraphEndpointRejectsOtherMethods(t *testing.T) {
	response := httptest.NewRecorder()
	graphHandler(t).ServeHTTP(response,
		httptest.NewRequest(http.MethodPost,
			withAllMatchConditions(graphPath+"?"+wholeGraphQuery), nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("status=%d want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

// graphLimits は段数と 3 つの上限だけを与える要求の項目である。
const graphLimits = "depth=1"

// 期間の端に置く文字列。graph-markii.log の 15 行は 13:00:00 から 13:00:13、
// graph-squid.log の 3 行は 13:00:08 と 13:00:09 と 13:00:12 である。
//
// graphUpperBound は 13:00:04 の 1 秒だけを取る期間の上端である。同じ接続先への接続の
// レコードが 13:00:09 と 13:00:13 にもあるため、上端を与えずに下端だけで絞ると、比較の
// 単位を変えても結果が変わらない。
const (
	graphSecondBound      = "2000-02-01T13:00:04+09:00"
	graphMillisecondBound = "2000-02-01T13:00:04.500+09:00"
	graphMicrosecondBound = "2000-02-01T13:00:04.500123+09:00"
	graphUpperBound       = "2000-02-01T13:00:04.999+09:00"
)

// `/api/v0/graph` の期間の端の精度は second、millisecond、microsecond に限る。
// year・month・day・hour・minute を与えた要求は invalid_request で失敗する。
func TestGraphEndpointRejectsYearThroughMinutePeriodPrecision(t *testing.T) {
	handler := graphHandler(t)
	for _, precision := range []core.Precision{
		core.PrecisionYear, core.PrecisionMonth, core.PrecisionDay,
		core.PrecisionHour, core.PrecisionMinute,
	} {
		for _, bound := range []struct{ text, precisionItem string }{
			{"timeFrom", "timeFromPrecision"},
			{"timeTo", "timeToPrecision"},
		} {
			t.Run(bound.precisionItem+"="+string(precision), func(t *testing.T) {
				apiError := requestGraphError(t, handler, graphLimits+
					"&"+bound.text+"="+url.QueryEscape(graphSecondBound)+
					"&"+bound.precisionItem+"="+string(precision)+
					"&filterUnit=second", http.StatusBadRequest)
				if apiError.Code != core.ApiErrorCodeInvalidRequest {
					t.Errorf("code=%q want invalid_request", apiError.Code)
				}
				// 項目は揃っており、欠けている項目の一覧を出す条件に該当しない。
				if len(apiError.MissingParameters) != 0 {
					t.Errorf("missingParameters=%v want none", apiError.MissingParameters)
				}
			})
		}
	}
}

// 反対側。受け取る precision の値は応答を返し、要求の文字列と精度をそのまま返す。
func TestGraphEndpointAcceptsSecondMillisecondAndMicrosecondPrecision(t *testing.T) {
	handler := graphHandler(t)
	for _, accepted := range []struct {
		precision core.Precision
		text      string
	}{
		{core.PrecisionSecond, graphSecondBound},
		{core.PrecisionMillisecond, graphMillisecondBound},
		{core.PrecisionMicrosecond, graphMicrosecondBound},
	} {
		t.Run(string(accepted.precision), func(t *testing.T) {
			page := decodeGraph(t, handler, graphLimits+
				"&timeFrom="+url.QueryEscape(accepted.text)+
				"&timeFromPrecision="+string(accepted.precision)+
				"&timeTo="+url.QueryEscape(accepted.text)+
				"&timeToPrecision="+string(accepted.precision)+
				"&filterUnit=second")
			if page.TimeFrom == nil || page.TimeFrom.RequestText != accepted.text {
				t.Fatalf("timeFrom=%+v want the requested text %q", page.TimeFrom, accepted.text)
			}
			if page.TimeTo == nil || page.TimeTo.Precision != accepted.precision {
				t.Fatalf("timeTo=%+v want the precision %q", page.TimeTo, accepted.precision)
			}
		})
	}
}

// 対になる文字列を持たない精度の項目を退ける。
func TestGraphEndpointRejectsAPrecisionWithoutItsTime(t *testing.T) {
	apiError := requestGraphError(t, graphHandler(t),
		graphLimits+"&timeFromPrecision=second", http.StatusBadRequest)
	if apiError.Code != core.ApiErrorCodeInvalidRequest {
		t.Errorf("code=%q want invalid_request", apiError.Code)
	}
	if len(apiError.MissingParameters) != 0 {
		t.Errorf("missingParameters=%v want none", apiError.MissingParameters)
	}
}

// 比較の単位が期間の判定の結果を変える。
//
// graph-markii.log の sn=5 は evt が net で subEvt が con の 1 行で、13:00:04 の 1 秒に
// 入る唯一の process_communication の根拠である。レコードの時刻は 13:00:04.000 で、期間は
// 13:00:04.500 から 13:00:04.999 である。
// second は両端とレコードを 13:00:04 に切り捨てて期間の中に並べ、millisecond は切り捨てずに
// レコードを下端より前に置く。
func TestGraphEndpointAppliesTheComparisonUnitToThePeriod(t *testing.T) {
	handler := graphHandler(t)
	period := "&edgeKind=process_communication" +
		"&timeFrom=" + url.QueryEscape(graphMillisecondBound) +
		"&timeFromPrecision=millisecond" +
		"&timeTo=" + url.QueryEscape(graphUpperBound) +
		"&timeToPrecision=millisecond&filterUnit="

	withSecond := decodeGraph(t, handler, graphLimits+period+"second")
	if withSecond.EdgeCount != 1 {
		t.Errorf("edgeCount=%d want 1 with the second unit", withSecond.EdgeCount)
	}

	withMillisecond := decodeGraph(t, handler, graphLimits+period+"millisecond")
	if withMillisecond.EdgeCount != 0 {
		t.Errorf("edgeCount=%d want 0 with the millisecond unit", withMillisecond.EdgeCount)
	}
}

// 値ごとの件数が 0 件になった理由は、欄の名前が無い状態と、絞り込みで残らなかった状態を
// 分けて返す。
func TestGraphEndpointSeparatesTheEmptyReasonsOfTheValueCounts(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name      string
		query     string
		want      core.EmptyReason
		wantKinds []core.NodeKind
	}{
		{
			// 語彙にも原資料の key にも無い文字列。打ち間違いがこれに該当する。
			"a field name no attribute carries",
			"countBy=process.binary_pathh&depth=0",
			core.EmptyReasonNoFieldObserved, nil,
		},
		{
			// 欄の値を持つのはプロセスとレコードのノードで、アカウントのノードは持たない。
			"a field the node kind does not carry",
			"countBy=process.binary_path&depth=0&nodeKind=account",
			core.EmptyReasonFieldOnOtherNodeKind,
			[]core.NodeKind{core.NodeKindProcess, core.NodeKindRecord},
		},
		{
			// 欄はあるが、事象の分類の絞り込みで残らない。session のレコードは psPath を持たない。
			"a field the filter removes",
			"countBy=process.binary_path&depth=0&eventCategory=session",
			core.EmptyReasonNoValueInFilter, nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			page := decodeGraph(t, handler,
				testCase.query)
			if len(page.ValueCounts) != 0 {
				t.Fatalf("the response counts %d values, want none", len(page.ValueCounts))
			}
			if page.DistinctValueCount == nil || *page.DistinctValueCount != 0 {
				t.Fatalf("the response counted %v distinct values, want 0",
					page.DistinctValueCount)
			}
			if page.ValueCountsEmptyReason != testCase.want {
				t.Errorf("valueCountsEmptyReason=%q want %q",
					page.ValueCountsEmptyReason, testCase.want)
			}
			if !slices.Equal(page.ValueCountsNodeKinds, testCase.wantKinds) {
				t.Errorf("valueCountsNodeKinds=%v want %v",
					page.ValueCountsNodeKinds, testCase.wantKinds)
			}
		})
	}
}
