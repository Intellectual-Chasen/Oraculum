package api_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// nodesPath は`/api/v0/nodes/{id}` の path の接頭辞である。
const nodesPath = "/api/v0/nodes/"

// nodeResponse は`/api/v0/nodes/{id}` の応答の項目名を test 側で固定する。
type nodeResponse struct {
	Node           subgraphNode         `json:"node"`
	Attributes     []core.NodeAttribute `json:"attributes"`
	AttributeCount int64                `json:"attributeCount"`
	Evidence       []core.GraphEvidence `json:"evidence"`
	EvidenceCount  int64                `json:"evidenceCount"`
	// EvidenceByCase は案件ごとの根拠の件数である。案件を区別しない取り込みでは出ない。
	EvidenceByCase []core.CaseEvidenceCount `json:"evidenceByCase,omitempty"`
	EdgeCounts     []core.NodeEdgeCount     `json:"edgeCounts"`
	// CreationRecords は対象の生成を記録した根拠のレコードである。
	CreationRecords     []core.GraphEvidence `json:"creationRecords"`
	CreationRecordCount int64                `json:"creationRecordCount"`
	// RelationDerivation はレコードのノードだけが持つ。
	RelationDerivation *core.RelationDerivation `json:"relationDerivation,omitempty"`
	// LogonSessionRejections は、関係にしなかったログオンである。要素数 0 でも空の配列で出る。
	LogonSessionRejections *[]core.LogonSessionRejection `json:"logonSessionRejections"`
}

func requestNode(t *testing.T, handler http.Handler, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(nodesPath+id+"?"+query), nil))
	return response
}

func decodeNode(t *testing.T, handler http.Handler, id, query string) nodeResponse {
	t.Helper()
	response := requestNode(t, handler, id, query)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded nodeResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// graphNodeIdOf は`/api/v0/graph` の応答から、種別と識別鍵の値で指定したノードの識別子を読む。
func graphNodeIdOf(t *testing.T, handler http.Handler, kind string, identity []string) string {
	t.Helper()
	for _, node := range decodeGraph(t, handler, wholeGraphQuery).Nodes {
		if node.Kind != kind || len(node.Identity) != len(identity) {
			continue
		}
		matched := true
		for at, value := range identity {
			if node.Identity[at].Value != value {
				matched = false
				break
			}
		}
		if matched {
			return node.Id
		}
	}
	t.Fatalf("the subgraph carries no %s node with the identity %v", kind, identity)
	return ""
}

// processDetailNodeId は、manifest が詳細の期待値を持つプロセスのノードの識別子を返す。
func processDetailNodeId(t *testing.T, handler http.Handler) string {
	t.Helper()
	detail := graphFixtures(t).ProcessDetail
	return graphNodeIdOf(t, handler, detail.NodeKind, detail.NodeIdentity)
}

// terminalNodeId は、2 つの収集元がどちらも記録する端末 T1 のノードの識別子を返す。
func terminalNodeId(t *testing.T, handler http.Handler) string {
	t.Helper()
	return graphNodeIdOf(t, handler, "terminal", []string{"T1"})
}

func TestNodeEndpointReturnsTheAttributesAndTheEdgeCounts(t *testing.T) {
	manifest := graphFixtures(t)
	handler := graphHandler(t)
	want := manifest.ProcessDetail
	id := graphNodeIdOf(t, handler, want.NodeKind, want.NodeIdentity)
	detail := decodeNode(t, handler, id, "")
	if detail.Node.Id != id || detail.Node.Kind != want.NodeKind {
		t.Fatalf("the response returned the node %q of kind %q, want %q and %s",
			detail.Node.Id, detail.Node.Kind, id, want.NodeKind)
	}
	// Logon ID を持たないレコードの対象でも、関係にしなかったログオンの欄は空の配列で出る。
	if detail.LogonSessionRejections == nil || len(*detail.LogonSessionRejections) != 0 {
		t.Errorf("logonSessionRejections = %v, want an empty array", detail.LogonSessionRejections)
	}
	if detail.EvidenceCount != want.EvidenceCount {
		t.Fatalf("evidenceCount=%d want %d", detail.EvidenceCount, want.EvidenceCount)
	}
	if detail.AttributeCount != int64(len(want.Attributes)) ||
		len(detail.Attributes) != len(want.Attributes) {
		t.Fatalf("attributeCount=%d with %d attributes, want %d of both",
			detail.AttributeCount, len(detail.Attributes), len(want.Attributes))
	}
	for index, attribute := range want.Attributes {
		got := detail.Attributes[index]
		if got.Semantic != attribute.Semantic || got.ValueCount != attribute.ValueCount {
			t.Errorf("attribute %d is %s with %d values, want %s with %d",
				index, got.Semantic, got.ValueCount, attribute.Semantic, attribute.ValueCount)
		}
		for at, count := range attribute.ObservationCounts {
			if got.Values[at].ObservationCount != count {
				t.Errorf("attribute %d value %d was observed %d times, want %d",
					index, at, got.Values[at].ObservationCount, count)
			}
		}
	}
	assertEdgeCounts(t, detail.EdgeCounts, want.EdgeCounts)
}

func assertEdgeCounts(t *testing.T, got, want []core.NodeEdgeCount) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("the response carries %d edge counts, want %d", len(got), len(want))
	}
	for index, count := range want {
		if !reflect.DeepEqual(got[index], count) {
			t.Errorf("edge count %d is %+v, want %+v", index, got[index], count)
		}
	}
}

func TestNodeEndpointPointsEvidenceAtTheOriginalRecords(t *testing.T) {
	handler := graphHandler(t)
	detail := decodeNode(t, handler, processDetailNodeId(t, handler), "")
	if len(detail.Evidence) != int(detail.EvidenceCount) {
		t.Fatalf("the response carries %d of %d evidence records",
			len(detail.Evidence), detail.EvidenceCount)
	}
	first := detail.Evidence[0]
	if first.RecordRef.SourceFileName != "graph-markii.log" {
		t.Errorf("the first evidence record is in %q, want graph-markii.log",
			first.RecordRef.SourceFileName)
	}
	if first.RecordRef.SequenceNumber == nil || *first.RecordRef.SequenceNumber != 1 {
		t.Fatalf("the first evidence record is at %+v, want the sequence number 1",
			first.RecordRef)
	}
	if first.EventTime == nil {
		t.Fatal("the first evidence record carries no event time")
	}
	normalized, readable := first.EventTime.NormalizedValue()
	if !readable || normalized != "2000-02-01T13:00:00.000+09:00" {
		t.Errorf("the first evidence time is %q, want 2000-02-01T13:00:00.000+09:00", normalized)
	}
}

// 属性と根拠を全件返す。どのノードでも、数えた件数と並べた要素の数が一致する。
func TestNodeEndpointReturnsEveryAttributeAndEveryEvidenceRecord(t *testing.T) {
	handler := graphHandler(t)
	for _, node := range decodeGraph(t, handler, wholeGraphQuery).Nodes {
		detail := decodeNode(t, handler, node.Id, "")
		if detail.AttributeCount != int64(len(detail.Attributes)) {
			t.Errorf("the %s node %q returned attributeCount=%d with %d attributes",
				node.Kind, node.Id, detail.AttributeCount, len(detail.Attributes))
		}
		if detail.EvidenceCount != int64(len(detail.Evidence)) {
			t.Errorf("the %s node %q returned evidenceCount=%d with %d evidence records",
				node.Kind, node.Id, detail.EvidenceCount, len(detail.Evidence))
		}
	}
}

// **上限と続きを取る位置を受けない。** 綴りを通知せずに既定値で処理しないよう、未知の
// 項目として退ける。
func TestNodeEndpointRejectsTheLimitAndTheCursor(t *testing.T) {
	handler := graphHandler(t)
	id := terminalNodeId(t, handler)
	for _, testCase := range []struct {
		name  string
		query string
	}{
		{"attribute limit", "attributeLimit=20"},
		{"evidence limit", "evidenceLimit=20"},
		{"limit", "limit=20"},
		{"cursor", "cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestNode(t, handler, id, testCase.query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", apiError.Code)
			}
		})
	}
}

func TestNodeEndpointSeparatesTerminalsSharingAHostname(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler,
		"depth=0&nodeKind=terminal")
	if page.NodeCount != 2 {
		t.Fatalf("the graph carries %d terminals, want 2", page.NodeCount)
	}
	first := decodeNode(t, handler, page.Nodes[0].Id, "")
	second := decodeNode(t, handler, page.Nodes[1].Id, "")
	if first.Node.Id == second.Node.Id {
		t.Fatal("the two terminals share a node id")
	}
	for _, detail := range []nodeResponse{first, second} {
		label, readable := detail.Node.Label.ComparableValue()
		if !readable || label != "PC01" {
			t.Errorf("the terminal %q carries the label %q", detail.Node.Id, label)
		}
	}
	if first.Node.Identity[0].Value == second.Node.Identity[0].Value {
		t.Error("the two terminals share the identity value")
	}
}

// markiiOnlyHandler は markii 形式の fixture だけを取り込む。収集元の一覧が異なるため、
// 解析実行への参照が graphHandler と別の値になる。
func markiiOnlyHandler(t *testing.T) http.Handler {
	t.Helper()
	return testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "graph-markii.log", OriginPath: "testdata/graph-markii.log",
	}))
}

// analysisRunRefOf は収集元の取り込み状況から、解析実行への参照を読む。
func analysisRunRefOf(t *testing.T, handler http.Handler) string {
	t.Helper()
	page := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	if len(page.Sources) == 0 {
		t.Fatal("the import carries no source, want the analysis run reference to be readable")
	}
	runRef := page.Sources[0].ImportStatus.AnalysisRunRef
	if runRef == "" {
		t.Fatal("the import status carries no analysis run reference")
	}
	return runRef
}

// 識別子は内容から導くため、別の解析実行が返した値がそのまま通る。
func TestNodeEndpointAcceptsANodeIdFromAnotherAnalysisRun(t *testing.T) {
	whole := graphHandler(t)
	markiiOnly := markiiOnlyHandler(t)
	if analysisRunRefOf(t, whole) == analysisRunRefOf(t, markiiOnly) {
		t.Fatal("the two imports name the same analysis run, want two different runs")
	}
	terminalId := terminalNodeId(t, whole)
	detail := decodeNode(t, markiiOnly, terminalId, "")
	if detail.Node.Id != terminalId || detail.Node.Kind != "terminal" {
		t.Fatalf("the other analysis run returned the node %q of kind %q, want %q and terminal",
			detail.Node.Id, detail.Node.Kind, terminalId)
	}
}

// Squid の収集元だけが記録するホスト名のノードは、markii 形式だけの解析実行に無い。
func TestNodeEndpointReportsANodeMissingFromTheCurrentAnalysisRun(t *testing.T) {
	whole := graphHandler(t)
	page := decodeGraph(t, whole,
		"depth=0&nodeKind=domain")
	if page.NodeCount != 1 {
		t.Fatalf("the whole graph carries %d domain nodes, want 1", page.NodeCount)
	}
	response := requestNode(t, markiiOnlyHandler(t), page.Nodes[0].Id, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusNotFound,
			response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", apiError.Code)
	}
}

func TestNodeEndpointRejectsAnUnknownNodeId(t *testing.T) {
	response := requestNode(t, graphHandler(t), "n:terminal:absent", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusNotFound,
			response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", apiError.Code)
	}
}

func TestNodeEndpointRejectsInvalidRequests(t *testing.T) {
	handler := graphHandler(t)
	id := terminalNodeId(t, handler)
	for _, testCase := range []struct {
		name        string
		query       string
		wantMissing []string
	}{
		{"an unsupported parameter", "depth=1", nil},
		{"a known parameter of another operation", "nodeLimit=10", nil},
		{"an empty value", "depth=", nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestNode(t, handler, id, testCase.query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
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

func TestNodeEndpointRejectsOtherMethods(t *testing.T) {
	handler := graphHandler(t)
	id := terminalNodeId(t, handler)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		httptest.NewRequest(http.MethodDelete,
			withAllMatchConditions(nodesPath+id+"?"+""), nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("status=%d want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

// レコードのノードから、そのレコードを起点にして関係を導いた結果を読む。
//
// **関係を 1 本も導けなかった起点の理由を、グラフの操作から読める。** 理由が無いと、
// 分析者は「導いた結果、相手が 0 件であった」と「導く処理を実行できなかった」を
// 区別できない。
func TestNodeEndpointCarriesTheRelationDerivationOfARecord(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, "depth=0&nodeKind=record")
	if len(page.Nodes) == 0 {
		t.Fatal("the fixture carries no record node")
	}
	outcomes := make(map[string]int)
	for _, node := range page.Nodes {
		detail := decodeNode(t, handler, node.Id, "")
		if detail.RelationDerivation == nil {
			t.Fatalf("the record node %q carries no relation derivation", node.Id)
		}
		if err := detail.RelationDerivation.Validate(); err != nil {
			t.Fatalf("the derivation of %q did not validate: %v", node.Id, err)
		}
		outcomes[string(detail.RelationDerivation.Outcome)]++
	}
	// 起点にならなかったレコードと、起点になったレコードを別の値で示す。
	if outcomes[string(core.RelationDerivationNotUsedAsOrigin)] == 0 {
		t.Fatal("no record is marked as not used as an origin")
	}
	var used int
	for outcome, count := range outcomes {
		if outcome != string(core.RelationDerivationNotUsedAsOrigin) {
			used += count
		}
	}
	if used == 0 {
		t.Fatal("no record carries the outcome of a derivation that ran")
	}
}

// レコード以外の種別のノードは、関係を導いた結果を持たない。
func TestNodeEndpointOmitsTheRelationDerivationOfOtherKinds(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, "depth=0")
	var checked int
	for _, node := range page.Nodes {
		if node.Kind == string(core.NodeKindRecord) {
			continue
		}
		detail := decodeNode(t, handler, node.Id, "")
		if detail.RelationDerivation != nil {
			t.Fatalf("the %s node %q carries a relation derivation",
				node.Kind, node.Id)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the fixture carries no node of a kind other than record")
	}
}

// locatorKey はレコードの位置を比較できる 1 つの文字列にする。位置の項目が pointer を
// 持つため、値の並びを符号化した文字列で突き合わせる。
func locatorKey(t *testing.T, locator core.RecordLocator) string {
	t.Helper()
	encoded, err := json.Marshal(locator)
	if err != nil {
		t.Fatalf("encoding the record locator: %v", err)
	}
	return string(encoded)
}

// **creationRecord の欄と creationRecords の集合が食い違わない。** 欄が present のノードは
// 生成を記録した根拠を指せ、absent と item_absent のノードは 1 件も指さない。
func TestNodeEndpointTiesTheCreationRecordAxisToTheRecordsItPointsAt(t *testing.T) {
	handler := graphHandler(t)
	seen := make(map[string]bool)
	for _, node := range decodeGraph(t, handler, wholeGraphQuery).Nodes {
		detail := decodeNode(t, handler, node.Id, "")
		if detail.CreationRecordCount != int64(len(detail.CreationRecords)) {
			t.Errorf("the %s node %q returned creationRecordCount=%d with %d records",
				node.Kind, node.Id, detail.CreationRecordCount, len(detail.CreationRecords))
		}
		seen[detail.Node.CreationRecord] = true
		switch detail.Node.CreationRecord {
		case string(core.NodeCreationRecordPresent):
			if len(detail.CreationRecords) == 0 {
				t.Errorf("the %s node %q carries creationRecord=present with no record",
					node.Kind, node.Id)
			}
		case string(core.NodeCreationRecordAbsent),
			string(core.NodeCreationRecordItemAbsent):
			if len(detail.CreationRecords) != 0 {
				t.Errorf("the %s node %q carries creationRecord=%q with the records %+v",
					node.Kind, node.Id, detail.Node.CreationRecord, detail.CreationRecords)
			}
		default:
			t.Errorf("the %s node %q carries the creation record %q, outside the vocabulary",
				node.Kind, node.Id, detail.Node.CreationRecord)
		}
		// 根拠として返すレコードは、そのノードを記録した根拠の中にある位置を指す。
		for _, creation := range detail.CreationRecords {
			if err := creation.Validate(); err != nil {
				t.Errorf("the %s node %q returned an invalid creation record: %v",
					node.Kind, node.Id, err)
			}
		}
	}
	// 3 つの値がすべて該当する入力である。片方の枝だけを通す fixture で不変条件を固定しない。
	for _, value := range []core.NodeCreationRecord{
		core.NodeCreationRecordPresent, core.NodeCreationRecordAbsent,
		core.NodeCreationRecordItemAbsent,
	} {
		if !seen[string(value)] {
			t.Errorf("the fixture carries no node whose creation record is %q", value)
		}
	}
}

// 生成を記録した根拠を持てない種別のノードは、欄と集合の両方で「該当しない」を返す。
func TestNodeEndpointReturnsNoCreationRecordOutsideProcesses(t *testing.T) {
	handler := graphHandler(t)
	for _, named := range []struct {
		kind     string
		identity []string
	}{
		{"terminal", []string{"T1"}},
		{"file", []string{"T1", `c:\data\secret.txt`}},
		{"registry_value", []string{"T1", `HKLM\Software\Run`}},
		{"ip", []string{"192.0.2.1"}},
		{"domain", []string{"example.test"}},
	} {
		id := graphNodeIdOf(t, handler, named.kind, named.identity)
		detail := decodeNode(t, handler, id, "")
		if detail.Node.CreationRecord != string(core.NodeCreationRecordItemAbsent) {
			t.Errorf("the %s node %v carries the creation record %q, want item_absent",
				named.kind, named.identity, detail.Node.CreationRecord)
		}
		if len(detail.CreationRecords) != 0 || detail.CreationRecordCount != 0 {
			t.Errorf("the %s node %v returned %d creation records (count %d), want none",
				named.kind, named.identity, len(detail.CreationRecords),
				detail.CreationRecordCount)
		}
	}
}

// 生成のレコードを持つプロセスと、持たないプロセスを、識別鍵で指定して読み分ける。
func TestNodeEndpointPointsAtTheRecordThatCreatedTheProcess(t *testing.T) {
	handler := graphHandler(t)
	created := decodeNode(t,
		handler, graphNodeIdOf(t, handler, "process", []string{"T1", "{P1}"}), "")
	if created.Node.CreationRecord != string(core.NodeCreationRecordPresent) {
		t.Fatalf("the process {P1} carries the creation record %q, want present",
			created.Node.CreationRecord)
	}
	if len(created.CreationRecords) == 0 {
		t.Fatal("the process {P1} points at no record that created it")
	}
	// 生成の根拠は、そのノードを記録した根拠のレコードの中の 1 件である。
	evidenceAt := make(map[string]bool, len(created.Evidence))
	for _, evidence := range created.Evidence {
		evidenceAt[locatorKey(t, evidence.RecordRef)] = true
	}
	for _, creation := range created.CreationRecords {
		if !evidenceAt[locatorKey(t, creation.RecordRef)] {
			t.Errorf("the creation record %+v of the process {P1} is outside its evidence",
				creation.RecordRef)
		}
	}

	// 通信のレコードだけが記録したプロセスは、生成の根拠を 1 件も持たない。
	communicated := decodeNode(t,
		handler, graphNodeIdOf(t, handler, "process", []string{"T1", "{P8}"}), "")
	if communicated.Node.CreationRecord != string(core.NodeCreationRecordAbsent) {
		t.Errorf("the process {P8} carries the creation record %q, want absent",
			communicated.Node.CreationRecord)
	}
	if len(communicated.CreationRecords) != 0 {
		t.Errorf("the process {P8} points at the creation records %+v, want none",
			communicated.CreationRecords)
	}
	if len(communicated.Evidence) == 0 {
		t.Error("the process {P8} carries no evidence, want the communication record")
	}
}

// graph-markii-logon-session.log の 403 は、端末 T1 のログオン 401 と同じ Logon ID を端末 T2 で
// 書いた操作である。応答は、関係にしなかったログオンを理由とログオンの元レコードの位置で持つ。
func TestNodeEndpointCarriesTheRejectedLogons(t *testing.T) {
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey, FileName: "graph-markii-logon-session.log",
		OriginPath: "testdata/graph-markii-logon-session.log",
	}))
	operationId, logonId := "", ""
	for _, node := range decodeGraph(t, handler, wholeGraphQuery+"&granularity=record&nodeKind=record").Nodes {
		switch node.Identity[len(node.Identity)-1].Value {
		case "403":
			operationId = node.Id
		case "401":
			logonId = node.Id
		}
	}
	if operationId == "" || logonId == "" {
		t.Fatal("the subgraph carries no record node for the sequence numbers 401 and 403")
	}
	var raw struct {
		LogonSessionRejections []map[string]json.RawMessage `json:"logonSessionRejections"`
	}
	if err := json.Unmarshal(requestNode(t, handler, operationId, "").Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.LogonSessionRejections) != 1 {
		t.Fatalf("logonSessionRejections = %v, want one element", raw.LogonSessionRejections)
	}
	names := slices.Sorted(maps.Keys(raw.LogonSessionRejections[0]))
	if !slices.Equal(names, []string{"logon", "reason"}) {
		t.Errorf("the rejection carries the members %v, want logon and reason", names)
	}
	rejection := (*decodeNode(t, handler, operationId, "").LogonSessionRejections)[0]
	if err := rejection.Validate(); err != nil {
		t.Errorf("the rejection is invalid: %v", err)
	}
	if rejection.Reason != core.LogonSessionRejectionOtherTerminal {
		t.Errorf("reason = %q, want other_terminal", rejection.Reason)
	}
	if sequence := rejection.Logon.RecordRef.SequenceNumber; sequence == nil || *sequence != 401 {
		t.Errorf("the rejected logon is at %v, want the sequence number 401", sequence)
	}
	// ログオンの側のノードは不成立を持たない。
	if logon := decodeNode(t, handler, logonId, ""); len(*logon.LogonSessionRejections) != 0 {
		t.Errorf("the logon record carries the rejections %v", *logon.LogonSessionRejections)
	}
}
