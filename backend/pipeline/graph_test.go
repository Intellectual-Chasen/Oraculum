// in-package test: 非公開の構築子で作った取り込み結果からグラフを組んで検査する。
package pipeline

import (
	"encoding/json"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 期待値は下記 2 つの収集元の原文から読んで決める。原文と期待値の manifest は
// internal/testdata/run/ にあり、api の検査が同じ byte 列を読む。
//
// graph-markii.log の 15 行は、端末 T1 のプロセス {P1} が 1 つのファイルを閉じて複製し、
// レジストリの値を書き、接続先 198.51.100.7 へ繋ぎ、アカウント mallory の遠隔ログインが
// 失敗した並びである。7 行目は**同じホスト名 PC01 を名乗る別の端末 T2** で、8 行目は
// 形式が意味を定めない evt と subEvt の組 (file/create) を持ち、同じプロセスに別の psPath を
// 与える。
//
// **9 行目と 10 行目と 11 行目は 13:00:09 の 198.51.100.7:8080 への接続である。** subEvt は
// それぞれ con と est と dcon で、候補として数えるのは con と est の 2 件である。
//
// **2 つの軸を分ける材料が入っている。** 1 行目と 7 行目は親のプロセス {P0} を参照し、
// {P0} の起動のレコードはどちらの端末にも無い。12 行目は親 {P5} を参照し、13 行目が
// その {P5} の起動を記録する (参照が先、起動が後)。14 行目は既に起動を記録済みの {P1} を
// 親として参照する (起動が先、参照が後)。15 行目の {P8} は通信のレコードだけが記録した
// プロセスで、observation が observed かつ起動のレコードを持たない。
//
// graph-squid.log の 4 行は、接続元 192.0.2.1 と 192.0.2.9 から example.test と
// 198.51.100.7:8080 への要求である。接続元は markii 形式の端末 T1 と T2 が持つ IP と同じ文字列で
// ある。2 行目は 9 行目と 10 行目の markii 形式のレコードと端末・接続先・秒が揃い、関連付けが
// 候補のエッジを 1 本挙げて関連付けを 2 件添える。
//
// **候補が 0 件になる 2 つの理由が入っている。** 3 行目は同じ接続先で秒が揃わず、段階 1 では
// 候補があり段階 2 で 0 件になる。4 行目は接続元が端末 T2 で、同じ接続先の候補 (端末 T1) と
// 段階 1 の端末の条件が一致せず、段階 1 の時点で 0 件になる。
const graphFixtureDir = "../internal/testdata/run/"

func graphSourceText(t *testing.T, fileName string) string {
	t.Helper()
	data, err := os.ReadFile(graphFixtureDir + fileName)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// wholeGraphQuery はグラフ全体を 1 応答で受け取る要求である。
func wholeGraphQuery() GraphQuery {
	return GraphQuery{
		Depth: 1,
	}
}

// edgeEvidenceOf はエッジの根拠のレコードを `/api/v0/edges/{id}` と同じ経路で読む。
//
// **部分グラフの応答は根拠の中身を含まない。** 図は根拠を描かないためである。根拠を
// 確かめる test はこの helper を通す。
func edgeEvidenceOf(t *testing.T, graph Graph, edgeId string) []core.GraphEvidence {
	t.Helper()
	detail, found := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the graph has no edge %q", edgeId)
	}
	if int64(len(detail.Evidence)) != detail.Edge.EvidenceCount {
		t.Fatalf("the edge %q carries %d of %d evidence records, want every record",
			edgeId, len(detail.Evidence), detail.Edge.EvidenceCount)
	}
	return detail.Evidence
}

// wholeGraphOf はグラフ全体を受け取り、条件に一致したノードが全件入っていることを
// 確かめる。
//
// 一部だけの応答を全体として読むと、以降の期待値が部分集合に対する検査になる。
func wholeGraphOf(t *testing.T, graph Graph) Subgraph {
	t.Helper()
	subgraph := graph.Query(wholeGraphQuery())
	if subgraph.MatchedNodeCount != len(subgraph.Nodes) {
		t.Fatalf("the subgraph carries %d of %d matched nodes, want every node",
			len(subgraph.Nodes), subgraph.MatchedNodeCount)
	}
	if len(subgraph.Nodes) == 0 || len(subgraph.Edges) == 0 {
		t.Fatalf("the graph carries %d nodes and %d edges, want both populated",
			len(subgraph.Nodes), len(subgraph.Edges))
	}
	return subgraph
}

// graphRunner は 2 つの fixture を読む取り込みの実行器を返す。
//
// **Run の組み立て順を迂回しない。** 収集元の識別 (観測期間) を組むのは Run であり、
// 関連付けはその期間を IP から端末への割当の適用期間に使う。識別を後から差し込むと、Run が
// 識別を組まなくなった regression を検査が見逃す。
//
// **通番の発行器を器が持つ。** 同じ器で 2 回取り込むと、2 回目は別の通番を受け取り、
// 別の sourceId になる。
func graphRunner(t *testing.T) *Runner {
	t.Helper()
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				graphSourceText(t, strings.TrimPrefix(originPath, "testdata/")))), nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
		Revision: "graph-revision", SettingsDigest: "graph-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// graphResultFrom は器に 2 つの fixture を取り込ませる。
func graphResultFrom(t *testing.T, runner *Runner) ImportResult {
	t.Helper()
	result, err := runner.Run([]SourcePlan{
		{
			OriginPath: "testdata/graph-markii.log", FileName: "graph-markii.log",
			FormatKey: MarkIIFormatKey,
		},
		{
			OriginPath: "testdata/graph-squid.log", FileName: "graph-squid.log",
			FormatKey: SquidFormatKey,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func graphResult(t *testing.T) ImportResult {
	t.Helper()
	return graphResultFrom(t, graphRunner(t))
}

func graphOf(t *testing.T) Graph {
	t.Helper()
	return NewGraph(graphResult(t), AllMatchConditions())
}

func fileGraphRecord(t *testing.T, line int64, path string, destination string) RecordEntry {
	t.Helper()
	fields := []core.RecordField{
		syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		syntheticField(t, "path", core.SemanticKeyFilePath, path),
		syntheticField(t, "sha256", core.SemanticKeyFileSha256,
			"1e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091"),
	}
	if destination != "" {
		fields = append(fields, syntheticField(t, "dstPath",
			core.SemanticKeyFileDestinationPath, destination))
		fields = append(fields, syntheticField(t, "size", core.SemanticKeyFileSizeBytes, "51100"))
	}
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{Fields: fields,
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}}},
	}
}

func TestNewGraphBuildsFileCopyAndContentMatchEdges(t *testing.T) {
	source := settleSource(t, 0)
	source.Records = []RecordEntry{
		fileGraphRecord(t, 1, `C:\source.zip`, `E:\source.zip`),
		fileGraphRecord(t, 2, `E:\source.zip`, ""),
	}
	result, err := newImportResult([]scannedSource{source},
		[]core.ImportStatus{settleStatus(t, source, "source")}, "run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(result, AllMatchConditions())
	copyEdges := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindFileCopy}, Depth: 1}).Edges
	if len(copyEdges) != 1 || copyEdges[0].State != core.RelationStateObserved {
		t.Fatalf("copy edges = %+v, want one observed edge", copyEdges)
	}
	matchEdges := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindFileContentMatch}, Depth: 1}).Edges
	if len(matchEdges) != 1 || matchEdges[0].State != core.RelationStateCandidate {
		t.Fatalf("content match edges = %+v, want one candidate edge", matchEdges)
	}
	if matchEdges[0].EvidenceCount != 2 {
		t.Errorf("content match evidence count = %d, want the two hash observations",
			matchEdges[0].EvidenceCount)
	}
	// 2 つの種別を与えた段数 1 の問い合わせは、1 種ずつの問い合わせのエッジの和を返す。
	bothEdges := graph.Query(GraphQuery{
		EdgeKinds: []core.EdgeKind{core.EdgeKindFileCopy, core.EdgeKindFileContentMatch}, Depth: 1,
	}).Edges
	var gotIds []string
	for _, edge := range bothEdges {
		gotIds = append(gotIds, edge.Id)
	}
	wantIds := []string{copyEdges[0].Id, matchEdges[0].Id}
	slices.Sort(gotIds)
	slices.Sort(wantIds)
	if !slices.Equal(gotIds, wantIds) {
		t.Errorf("edges of both kinds = %v, want the union %v", gotIds, wantIds)
	}
	if matchEdges[0].SourceNodeId == matchEdges[0].TargetNodeId {
		t.Error("content match merged the source and destination file nodes")
	}
	copySource, found := graph.NodeDetail(copyEdges[0].SourceNodeId)
	if !found {
		t.Fatal("the copy source has no detail")
	}
	copyDestination, found := graph.NodeDetail(copyEdges[0].TargetNodeId)
	if !found {
		t.Fatal("the copy destination has no detail")
	}
	if !hasNodeAttribute(copySource, core.SemanticKeyFileSha256) ||
		!hasNodeAttribute(copyDestination, core.SemanticKeyFileSha256) {
		t.Fatal("the copy edge did not preserve the hash on both file nodes")
	}
	if hasNodeAttribute(copySource, core.SemanticKeyFileSizeBytes) ||
		hasNodeAttribute(copyDestination, core.SemanticKeyFileSizeBytes) {
		t.Fatal("the ambiguous copy size was attached to a file node")
	}
}

func TestNodeLabelOfDoesNotCopyAnAmbiguousFileName(t *testing.T) {
	fields := []core.RecordField{
		syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		syntheticField(t, "path", core.SemanticKeyFilePath, `C:\source.zip`),
		syntheticField(t, "dstPath", core.SemanticKeyFileDestinationPath, `E:\source.zip`),
		syntheticField(t, "name", core.SemanticKeyFileName, "shared-name"),
	}
	allKeys := core.NewRecordGraph(fields).Nodes
	keys := make([]core.NodeKey, 0, len(allKeys))
	for _, key := range allKeys {
		if key.Kind == core.NodeKindFile {
			keys = append(keys, key)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("file keys = %d, want source and destination", len(keys))
	}
	for _, key := range keys {
		label, readable := nodeLabelOf(fields, key)
		if !readable {
			t.Fatalf("file key %+v has no label", key)
		}
		value, comparable := label.ComparableValue()
		if !comparable || value == "shared-name" {
			t.Errorf("file key %+v has label %q, want the key-derived file name", key, value)
		}
	}
}

// 要求先の URI から host を切り出した項目は、原資料の文字列に URI 全体を持つ (requestTargetHost)。
// ホスト名のノードの表示名は識別鍵の値 (host) であり、URI 全体を表示名の原資料の文字列にしない。
func TestNodeLabelOfDerivesTheKeyValueCutFromALongerLexeme(t *testing.T) {
	const host = "www.example.test"
	fields := []core.RecordField{
		textField(requestTargetHostFieldName, core.SemanticKeyConnectionDestinationHostname,
			normalizedText("http://"+host+"/path/index.html?q=1", host,
				derivationRequestTargetHost)),
	}
	label, readable := nodeLabelOf(fields, domainKeyOf(t, fields))
	if !readable {
		t.Fatal("the hostname node has no label")
	}
	if label.RawText != nil {
		t.Errorf("label rawText = %q, want absent", *label.RawText)
	}
	if label.ValueState != core.ValueStateDerived || label.Normalized == nil ||
		*label.Normalized != host || label.Derivation == nil ||
		*label.Derivation != derivationRequestTargetHost {
		t.Errorf("label = %+v, want the host derived by %q", label, derivationRequestTargetHost)
	}
}

// 原資料の文字列が識別鍵の値そのものである項目は、原資料の文字列を表示名に保つ。
func TestNodeLabelOfKeepsTheRawTextEqualToTheKeyValue(t *testing.T) {
	fields := []core.RecordField{
		syntheticField(t, "host", core.SemanticKeyConnectionDestinationHostname, "www.example.test"),
	}
	label, readable := nodeLabelOf(fields, domainKeyOf(t, fields))
	if !readable {
		t.Fatal("the hostname node has no label")
	}
	if label.ValueState != core.ValueStatePresent || label.RawText == nil ||
		*label.RawText != "www.example.test" {
		t.Errorf("label = %+v, want the present raw text www.example.test", label)
	}
}

// 同じレコードが、値を切り出した項目と、原資料の文字列が値そのものである項目の両方を持つときは、
// 項目の並びに依らず原資料の文字列が値そのものである項目を表示名にする。
func TestNodeLabelOfPrefersTheRawTextEqualToTheKeyValue(t *testing.T) {
	const host = "www.example.test"
	fields := []core.RecordField{
		textField(requestTargetHostFieldName, core.SemanticKeyConnectionDestinationHostname,
			normalizedText("http://"+host+"/index.html", host, derivationRequestTargetHost)),
		syntheticField(t, "host", core.SemanticKeyConnectionDestinationHostname, host),
	}
	label, readable := nodeLabelOf(fields, domainKeyOf(t, fields))
	if !readable {
		t.Fatal("the hostname node has no label")
	}
	if label.ValueState != core.ValueStatePresent || label.RawText == nil ||
		*label.RawText != host {
		t.Errorf("label = %+v, want the present raw text %s", label, host)
	}
}

// 原資料の文字列を持たない導いた値の項目が先に並んでも、原資料の文字列が識別鍵の値そのものである項目を選ぶ。
func TestNodeLabelOfPrefersTheRawTextOverAnEarlierDerivedValue(t *testing.T) {
	const host = "www.example.test"
	derived, err := core.NewDerivedValue(host, derivationRequestTargetHost)
	if err != nil {
		t.Fatalf("building the derived value: %v", err)
	}
	fields := []core.RecordField{
		textField(requestTargetHostFieldName, core.SemanticKeyConnectionDestinationHostname, derived),
		syntheticField(t, "host", core.SemanticKeyConnectionDestinationHostname, host),
	}
	label, readable := nodeLabelOf(fields, domainKeyOf(t, fields))
	if !readable {
		t.Fatal("the hostname node has no label")
	}
	if label.ValueState != core.ValueStatePresent || label.RawText == nil ||
		*label.RawText != host {
		t.Errorf("label = %+v, want the present raw text %s", label, host)
	}
}

// 先のレコードが導いた表示名を持つノードは、後のレコードが同じ文字列を原資料に書いていれば、
// その原資料の文字列を表示名にする。別の文字列の表示名と、導いた表示名では置き換えない。
func TestApplyLabelPrefersALaterRawTextEqualToTheDerivedLabel(t *testing.T) {
	const address = "192.0.2.5"
	derived, err := core.NewDerivedValue(address, "IPv4-mapped IPv6 address written as dotted decimal IPv4")
	if err != nil {
		t.Fatalf("building the derived value: %v", err)
	}
	raw := func(text string) core.RawAndNormalized {
		value, err := core.NewRawValue(core.ValueStatePresent, text)
		if err != nil {
			t.Fatalf("building the raw value: %v", err)
		}
		return value
	}

	node := graphNode{label: core.NewAbsentItemValue()}
	node.applyLabel(derived)
	node.applyLabel(raw("192.0.2.6"))
	if node.label.ValueState != core.ValueStateDerived {
		t.Fatalf("label = %+v, want the derived label kept against another text", node.label)
	}
	node.applyLabel(raw(address))
	if node.label.ValueState != core.ValueStatePresent || node.label.RawText == nil ||
		*node.label.RawText != address {
		t.Fatalf("label = %+v, want the raw text %s", node.label, address)
	}
	node.applyLabel(derived)
	if node.label.ValueState != core.ValueStatePresent {
		t.Errorf("label = %+v, want the raw text kept against a derived label", node.label)
	}
}

// domainKeyOf は項目が指すホスト名のノードの識別鍵を 1 つ返す。
func domainKeyOf(t *testing.T, fields []core.RecordField) core.NodeKey {
	t.Helper()
	for _, key := range core.NewRecordGraph(fields).Nodes {
		if key.Kind == core.NodeKindDomain {
			return key
		}
	}
	t.Fatal("the fields name no hostname node")
	return core.NodeKey{}
}

func hasNodeAttribute(detail NodeDetail, semantic core.SemanticKey) bool {
	for _, attribute := range detail.Attributes {
		if attribute.Semantic == semantic {
			return true
		}
	}
	return false
}

// edgeSummary は 1 本のエッジを「種別 起点の表示名 終点の表示名 根拠件数」へ直す。
func edgeSummary(subgraph Subgraph, edge core.GraphEdge) string {
	return string(edge.Kind) + " " + labelIn(subgraph, edge.SourceNodeId) + " -> " +
		labelIn(subgraph, edge.TargetNodeId) + " " + strconv.FormatInt(edge.EvidenceCount, 10)
}

func labelIn(subgraph Subgraph, id string) string {
	for _, node := range subgraph.Nodes {
		if node.Id != id {
			continue
		}
		if value, readable := node.Label.ComparableValue(); readable {
			return string(node.Kind) + ":" + value
		}
		return string(node.Kind) + ":" + string(node.Label.ValueState)
	}
	return "missing:" + id
}

func TestNewGraphBuildsTheObservedLayer(t *testing.T) {
	graph := graphOf(t)
	subgraph := wholeGraphOf(t, graph)
	if graph.nodeCount() != len(subgraph.Nodes) || graph.edgeCount() != len(subgraph.Edges) {
		t.Fatalf("the graph carries %d nodes and %d edges, want the queried %d and %d",
			graph.nodeCount(), graph.edgeCount(), len(subgraph.Nodes), len(subgraph.Edges))
	}
	// 期待する種別のノードがすべて出る。エッジの端点が nodes にあることは、labelIn が
	// 探せない端点を missing: として返し、下の期待値と食い違うことで決まる。
	kinds := map[core.NodeKind]int{}
	for _, node := range subgraph.Nodes {
		kinds[node.Kind]++
	}
	for _, kind := range []core.NodeKind{
		core.NodeKindTerminal, core.NodeKindProcess, core.NodeKindFile,
		core.NodeKindRegistryValue, core.NodeKindAccount, core.NodeKindIp, core.NodeKindDomain,
		core.NodeKindRecord,
	} {
		if kinds[kind] == 0 {
			t.Errorf("the graph carries no %s node", kind)
		}
	}
	// **レコードが対象を指す関係を外して確かめる。** 本 test の対象は、レコードから
	// 作った対象どうしの関係である。レコードが対象を指す関係は
	// TestNewGraphNamesTheObjectsOfEveryRecord が確かめる。
	summaries := make([]string, 0, len(subgraph.Edges))
	for _, edge := range subgraph.Edges {
		if edge.Kind == core.EdgeKindRecordNamesObject {
			continue
		}
		summaries = append(summaries, edgeSummary(subgraph, edge))
	}
	want := []string{
		`terminal_address terminal:PC01 -> ip:192.0.2.1 15`,
		`terminal_account terminal:PC01 -> account:alice 1`,
		`ran_on process:C:\app.exe -> terminal:PC01 9`,
		`process_parent_child process:C:\parent.exe -> process:C:\app.exe 1`,
		`process_executable file:C:\app.exe -> process:C:\app.exe 1`,
		`file_operation process:C:\app.exe -> file:secret.txt 2`,
		`file_copy file:secret.txt -> file:secret.txt 1`,
		`registry_operation process:C:\app.exe -> registry_value:Startup 1`,
		`process_communication process:C:\app.exe -> ip:198.51.100.7 4`,
		`terminal_account terminal:PC01 -> account:mallory 1`,
		`terminal_address terminal:PC01 -> ip:192.0.2.9 1`,
		`ran_on process:C:\app.exe -> terminal:PC01 1`,
		`process_parent_child process:item_absent -> process:C:\app.exe 1`,
		`process_executable file:C:\app.exe -> process:C:\app.exe 1`,
		`file_operation process:C:\app.exe -> file:new.txt 1`,
		`ran_on process:C:\child6.exe -> terminal:PC01 1`,
		`process_parent_child process:C:\late.exe -> process:C:\child6.exe 1`,
		`process_executable file:C:\child6.exe -> process:C:\child6.exe 1`,
		`ran_on process:C:\late.exe -> terminal:PC01 1`,
		`process_executable file:C:\late.exe -> process:C:\late.exe 1`,
		`ran_on process:C:\child7.exe -> terminal:PC01 1`,
		`process_parent_child process:C:\app.exe -> process:C:\child7.exe 1`,
		`process_executable file:C:\child7.exe -> process:C:\child7.exe 1`,
		`ran_on process:C:\net8.exe -> terminal:PC01 2`,
		`process_communication process:C:\net8.exe -> ip:198.51.100.7 2`,
		`http_request ip:192.0.2.1 -> domain:example.test 1`,
		`http_request ip:192.0.2.1 -> ip:198.51.100.7 2`,
		`http_request ip:192.0.2.9 -> ip:198.51.100.7 1`,
		`terminal_outbound_connection terminal:PC01 -> ip:198.51.100.7 4`,
		`cross_source_connection_match domain:example.test -> process:C:\net8.exe 2`,
		`cross_source_connection_match ip:198.51.100.7 -> process:C:\app.exe 3`,
	}
	if len(summaries) != len(want) {
		t.Fatalf("the subgraph carries %v, want %v", summaries, want)
	}
	for index, summary := range want {
		if summaries[index] != summary {
			t.Errorf("edge %d is %q, want %q", index, summaries[index], summary)
		}
	}
}

// レコードのノードは収集元の file 名と位置を表示名に持ち、そのレコードが指す対象へ
// 関係を張る。期待値は fixture の原文から読んで決める。markii 形式の位置は原文の sn、
// Squid の位置は行番号である。
func TestNewGraphNamesTheObjectsOfEveryRecord(t *testing.T) {
	subgraph := wholeGraphOf(t, graphOf(t))
	named := make(map[string][]string)
	for _, edge := range subgraph.Edges {
		if edge.Kind != core.EdgeKindRecordNamesObject {
			continue
		}
		source := labelIn(subgraph, edge.SourceNodeId)
		named[source] = append(named[source], labelIn(subgraph, edge.TargetNodeId))
		// **対象を指す関係の起点はレコードのノードだけである。**
		if !strings.HasPrefix(source, string(core.NodeKindRecord)+":") {
			t.Errorf("a naming edge starts at %q, want a record node", source)
		}
	}
	// 原文の 15 行のうち、指す対象の組が互いに異なる行を文字列で指す。
	for _, testCase := range []struct {
		record string
		want   []string
	}{
		// 1 行目はプロセスの起動で、親のプロセスを参照する。
		{`record:graph-markii.log ID 1`, []string{
			`terminal:PC01`, `process:C:\app.exe`, `account:alice`, `ip:192.0.2.1`,
			`process:C:\parent.exe`,
		}},
		// 6 行目は遠隔ログインの失敗で、プロセスを指さない。
		{`record:graph-markii.log ID 6`, []string{
			`terminal:PC01`, `account:mallory`, `ip:192.0.2.1`,
		}},
		// 13 行目は親を参照しないプロセスの起動である。
		{`record:graph-markii.log ID 13`, []string{
			`terminal:PC01`, `process:C:\late.exe`, `ip:192.0.2.1`,
		}},
		// Squid の 1 行目は接続元のアドレスとホスト名を指す。
		{`record:graph-squid.log 行 1`, []string{
			`ip:192.0.2.1`, `domain:example.test`,
		}},
	} {
		t.Run(testCase.record, func(t *testing.T) {
			got := slices.Clone(named[testCase.record])
			want := slices.Clone(testCase.want)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("the record names %v, want %v", got, want)
			}
		})
	}
	// レコードのノードは原文の行ごとに 1 つある。
	for _, want := range []string{
		`record:graph-markii.log ID 1`, `record:graph-markii.log ID 15`,
		`record:graph-squid.log 行 1`, `record:graph-squid.log 行 4`,
	} {
		if len(named[want]) == 0 {
			t.Errorf("the graph carries no naming edge from %q", want)
		}
	}
}

func TestNewGraphSeparatesTerminalsSharingAHostname(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}, Depth: 0})
	if subgraph.MatchedNodeCount != 2 {
		t.Fatalf("the graph carries %d terminals, want 2", subgraph.MatchedNodeCount)
	}
	first, second := subgraph.Nodes[0], subgraph.Nodes[1]
	if first.Id == second.Id {
		t.Fatalf("two terminals share the node id %q", first.Id)
	}
	for _, node := range subgraph.Nodes {
		label, readable := node.Label.ComparableValue()
		if !readable || label != "PC01" {
			t.Fatalf("the terminal %q carries the label %+v, want PC01", node.Id, node.Label)
		}
	}
	requireIdentity(t, first, "terminal.id", "T1")
	requireIdentity(t, second, "terminal.id", "T2")
}

func requireIdentity(t *testing.T, node core.SubgraphNode, semantic, value string) {
	t.Helper()
	if len(node.Identity) == 0 {
		t.Fatalf("the node %q carries no identity", node.Id)
	}
	if string(node.Identity[0].Semantic) != semantic || node.Identity[0].Value != value {
		t.Errorf("the node identity is %+v, want %s=%s", node.Identity[0], semantic, value)
	}
}

func TestNodeIdIsStableAcrossImportsOfTheSameContent(t *testing.T) {
	// 同じ内容を別の取り込みとして読み直す。再取り込みは新しい sourceId を発行するが、
	// 識別子を導く材料が識別鍵の組だけであるため、ノードとエッジの識別子は変わらない。
	runner := graphRunner(t)
	first := NewGraph(graphResultFrom(t, runner), AllMatchConditions())
	second := NewGraph(graphResultFrom(t, runner), AllMatchConditions())
	firstGraph := first.Query(wholeGraphQuery())
	secondGraph := second.Query(wholeGraphQuery())
	requireSourceIdsDiffer(t, first, second, firstGraph, secondGraph)
	if len(firstGraph.Nodes) != len(secondGraph.Nodes) {
		t.Fatalf("the two imports carry %d and %d nodes",
			len(firstGraph.Nodes), len(secondGraph.Nodes))
	}
	for index, node := range firstGraph.Nodes {
		if secondGraph.Nodes[index].Id != node.Id {
			t.Errorf("node %d is %q on the second import, want %q",
				index, secondGraph.Nodes[index].Id, node.Id)
		}
	}
	if len(firstGraph.Edges) != len(secondGraph.Edges) {
		t.Fatalf("the two imports carry %d and %d edges",
			len(firstGraph.Edges), len(secondGraph.Edges))
	}
	for index, edge := range firstGraph.Edges {
		if secondGraph.Edges[index].Id != edge.Id {
			t.Errorf("edge %d is %q on the second import, want %q",
				index, secondGraph.Edges[index].Id, edge.Id)
		}
	}
}

// requireSourceIdsDiffer は 2 つの取り込みが別の収集元の識別子を名乗ることを確かめる。
// 同じ識別子で比べると、識別子をまたいだ安定を確かめたことにならない。
func requireSourceIdsDiffer(t *testing.T, firstGraph, secondGraph Graph,
	first, second Subgraph,
) {
	t.Helper()
	firstSource := edgeEvidenceOf(t, firstGraph, first.Edges[0].Id)[0].RecordRef.SourceId
	secondSource := edgeEvidenceOf(t, secondGraph, second.Edges[0].Id)[0].RecordRef.SourceId
	if firstSource == "" || secondSource == "" {
		t.Fatalf("the evidence carries the source identifiers %q and %q",
			firstSource, secondSource)
	}
	if firstSource == secondSource {
		t.Fatalf("both imports name the source %q, want two different identifiers",
			firstSource)
	}
}

// **絞り込みに合わないノードも、エッジの端点として応答に入る。** 入らないと、応答の
// エッジが nodes に無いノードを指す。端点は selection が edge_endpoint になり、
// 絞り込みに合ったノードと区別できる。
func TestQueryCarriesTheEndpointsOutsideTheFilter(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{
		NodeIds: []string{terminalNodeIdOf(t, graph, "T1")},
		Depth:   1,
	})
	// 起点の端末 T1 だけが絞り込みに合う。
	if subgraph.MatchedNodeCount != 1 {
		t.Fatalf("the query matched %d nodes, want the one requested terminal",
			subgraph.MatchedNodeCount)
	}
	matched, endpoints := 0, 0
	carried := make(map[string]struct{}, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		carried[node.Id] = struct{}{}
		switch node.Selection {
		case core.NodeSelectionMatched:
			matched++
		case core.NodeSelectionEdgeEndpoint:
			endpoints++
		}
	}
	// 起点の端末 T1 に繋がるのは terminal_address 1 本と terminal_outbound_connection 1 本と
	// terminal_account 2 本と ran_on 5 本と、T1 を指す 15 件のレコードからの record_names_object
	// 15 本である。端点は IP 2 件・アカウント 2 件・プロセス 5 件・レコード 15 件の 24 である。
	if matched != 1 || endpoints != 24 {
		t.Fatalf("the subgraph carries %d matched nodes and %d endpoints, want 1 and 24",
			matched, endpoints)
	}
	if matched+endpoints != len(subgraph.Nodes) {
		t.Errorf("the subgraph carries %d nodes but %d are matched or endpoints",
			len(subgraph.Nodes), matched+endpoints)
	}
	if len(subgraph.Edges) != 24 {
		t.Fatalf("the subgraph carries %d edges, want 24", len(subgraph.Edges))
	}
	if subgraph.EdgeCount != len(subgraph.Edges) {
		t.Errorf("the subgraph counts %d edges and returns %d",
			subgraph.EdgeCount, len(subgraph.Edges))
	}
	for _, edge := range subgraph.Edges {
		for _, endpoint := range []string{edge.SourceNodeId, edge.TargetNodeId} {
			if _, present := carried[endpoint]; !present {
				t.Errorf("the edge %q points at the node %q, which the response omits",
					edge.Id, endpoint)
			}
		}
	}
}

// **起点を複数与えた要求は、起点ごとの近傍の和を返す。** 分析者がノードを 1 つずつ選んで
// 関係先を足していくと、起点の数だけ近傍が重なる。
func TestQueryWithSeveralOriginsReturnsTheUnionOfTheirNeighbourhoods(t *testing.T) {
	graph := graphOf(t)
	idsOf := func(subgraph Subgraph) map[string]struct{} {
		ids := make(map[string]struct{}, len(subgraph.Nodes))
		for _, node := range subgraph.Nodes {
			ids[node.Id] = struct{}{}
		}
		return ids
	}
	first := terminalNodeIdOf(t, graph, "T1")
	firstNodes := idsOf(graph.Query(GraphQuery{NodeIds: []string{first}, Depth: 1}))
	// 2 つ目の起点は、1 つ目の近傍の外にあるノードにする。和が 1 つ目の近傍より広がる。
	second := ""
	for _, node := range graph.Query(GraphQuery{Depth: 0}).Nodes {
		if _, inside := firstNodes[node.Id]; !inside {
			second = node.Id
			break
		}
	}
	if second == "" {
		t.Fatal("the fixture carries no node outside the neighbourhood of T1")
	}
	secondNodes := idsOf(graph.Query(GraphQuery{NodeIds: []string{second}, Depth: 1}))

	both := graph.Query(GraphQuery{NodeIds: []string{first, second}, Depth: 1})
	if both.MatchedNodeCount != 2 {
		t.Fatalf("the query matched %d nodes, want the two origins", both.MatchedNodeCount)
	}
	want := make(map[string]struct{}, len(firstNodes)+len(secondNodes))
	for id := range firstNodes {
		want[id] = struct{}{}
	}
	for id := range secondNodes {
		want[id] = struct{}{}
	}
	if got := idsOf(both); !maps.Equal(got, want) {
		t.Fatalf("the query carries %d nodes, want the %d nodes of the two neighbourhoods",
			len(got), len(want))
	}
}

// terminalNodeIdOf は識別鍵の値で指す端末のノードの識別子を返す。
func terminalNodeIdOf(t *testing.T, graph Graph, terminalId string) string {
	t.Helper()
	found := graph.Query(GraphQuery{
		NodeKinds: []core.NodeKind{core.NodeKindTerminal},
		Depth:     0,
	})
	for _, node := range found.Nodes {
		for _, value := range node.Identity {
			if value.Semantic == core.SemanticKeyTerminalId && value.Value == terminalId {
				return node.Id
			}
		}
	}
	t.Fatalf("the fixture carries no terminal named %q", terminalId)
	return ""
}

func TestQueryFiltersEvidenceByEventAction(t *testing.T) {
	graph := graphOf(t)
	for _, testCase := range []struct {
		name          string
		action        string
		wantEdges     int
		wantEvidence  int64
		wantNodeCount int
	}{
		// copy のレコードは 1 件で、指す対象は端末・プロセス・元と先のファイル・IP の
		// 5 つである。エッジは対象どうしの 4 本と、レコードが対象を指す関係 5 本である。起点は
		// レコードと、端末を除く 4 つの対象である。
		{"an action the source carries", "copy", 9, 1, 5},
		{"an action the source does not carry", "del", 0, 0, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{Depth: 1, RecordFilter: RecordFilter{EventCategory: "file", EventAction: testCase.action}})
			if subgraph.MatchedNodeCount != testCase.wantNodeCount {
				t.Fatalf("the query matched %d nodes, want %d",
					subgraph.MatchedNodeCount, testCase.wantNodeCount)
			}
			if subgraph.EdgeCount != testCase.wantEdges {
				t.Fatalf("the query selected %d edges, want %d",
					subgraph.EdgeCount, testCase.wantEdges)
			}
			for _, edge := range subgraph.Edges {
				if edge.EvidenceCount != testCase.wantEvidence {
					t.Errorf("the edge %q carries %d evidence records, want %d",
						edge.Kind, edge.EvidenceCount, testCase.wantEvidence)
				}
			}
		})
	}
}

func TestQueryNarrowsTheApplicableRangeToTheRequestedPeriod(t *testing.T) {
	graph := graphOf(t)
	from := time.Date(2000, 2, 1, 13, 0, 2, 0, time.FixedZone("+0900", 9*60*60))
	subgraph := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindRanOn}, Depth: 1, RecordFilter: RecordFilter{TimeFrom: &from, TimeUnit: time.Second}})
	if len(subgraph.Edges) != 6 {
		t.Fatalf("the query selected %d ran_on edges, want 6", len(subgraph.Edges))
	}
	edge := subgraph.Edges[0]
	if edge.EvidenceCount != 7 {
		t.Fatalf("the ran_on edge carries %d evidence records, want 7", edge.EvidenceCount)
	}
	requireNormalizedTime(t, "applicableRange.from", edge.ApplicableRange.From,
		"2000-02-01T13:00:02.000+09:00")
	requireNormalizedTime(t, "applicableRange.to", edge.ApplicableRange.To,
		"2000-02-01T13:00:09.000+09:00")
}

func requireNormalizedTime(t *testing.T, item string, value core.Timestamp, want string) {
	t.Helper()
	normalized, readable := value.NormalizedValue()
	if !readable || normalized != want {
		t.Errorf("%s is %q (readable %v), want %q", item, normalized, readable, want)
	}
}

func TestApplicableRangeIsAbsentWithoutAComparableTime(t *testing.T) {
	graph := graphOf(t)
	if applicable := graph.applicableRange(nil); applicable != nil {
		t.Errorf("an edge without evidence carries the applicable range %+v, want none", applicable)
	}
}

// 根拠のレコードは、意味を推定した観測の種別を、推定した意味ごとそのまま持つ。
//
// file/create はどのバージョンの形式も意味を定めず、収集元のレコードから意味を推定した組である
// (backend/adapters/markii/record_observation_fields.go の inferredSubEvents)。
//
// **意味の文字列を assert する。** グラフへ取り込む addRecord と、根拠を組む
// evidenceItems の 2 か所が種別を複製する。状態だけを見ると、複製が意味を除いても
// 気付けない。
func TestEvidenceCarriesTheInferredObservationKind(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindFileOperation}, Depth: 1, RecordFilter: RecordFilter{EventCategory: "file", EventAction: "create"}})
	if len(subgraph.Edges) != 1 {
		t.Fatalf("the query selected %d edges, want 1", len(subgraph.Edges))
	}
	records := edgeEvidenceOf(t, graph, subgraph.Edges[0].Id)
	if len(records) != 1 {
		t.Fatalf("the edge carries %d evidence records, want 1", len(records))
	}
	evidence := records[0]
	if evidence.ObservationKind.Status != core.ObservationKindStatusInferred {
		t.Errorf("the evidence carries the observation kind status %q, want %q",
			evidence.ObservationKind.Status, core.ObservationKindStatusInferred)
	}
	if evidence.ObservationKind.Meaning == "" {
		t.Error("the evidence carries no meaning for the inferred observation kind")
	}
	if err := evidence.ObservationKind.Validate(); err != nil {
		t.Error(err)
	}
	if evidence.RecordRef.SequenceNumber == nil || *evidence.RecordRef.SequenceNumber != 8 {
		t.Errorf("the evidence points at %+v, want the record with the sequence number 8",
			evidence.RecordRef)
	}
}

// 候補が持つ観測の種別の複製が、推定した意味を除かない。
//
// cloneObservationKind は候補の起点と候補の側の 4 か所から呼ばれる
// (graph_candidate.go と candidate_set_terminal_origin.go)。**複製した値が Validate を
// 通ることも確かめる。** 意味を除いた inferred は検査を通らない
// (backend/core/observation_kind.go の validateMeaning)。
func TestCloneObservationKindKeepsTheInferredMeaning(t *testing.T) {
	const meaning = "接続を開いた記録と推定した"
	source := core.ObservationKind{
		Raw: []core.RecordField{
			syntheticField(t, "evt", core.SemanticKeyEventCategory, "net"),
		},
		Status:  core.ObservationKindStatusInferred,
		Meaning: meaning,
	}
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}

	cloned := cloneObservationKind(source)

	if cloned.Meaning != meaning {
		t.Errorf("the clone carries the meaning %q, want %q", cloned.Meaning, meaning)
	}
	if cloned.Status != core.ObservationKindStatusInferred {
		t.Errorf("the clone carries the status %q, want %q",
			cloned.Status, core.ObservationKindStatusInferred)
	}
	if err := cloned.Validate(); err != nil {
		t.Error(err)
	}
}

func TestNodeDetailKeepsConflictingAttributeValues(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindProcess}, Depth: 0})
	detail, found := graph.NodeDetail(subgraph.Nodes[0].Id)
	if !found {
		t.Fatalf("the node %q has no detail", subgraph.Nodes[0].Id)
	}
	binaryPaths := core.NodeAttribute{}
	for _, attribute := range detail.Attributes {
		if attribute.Semantic == core.SemanticKeyProcessBinaryPath {
			binaryPaths = attribute
		}
	}
	// **食い違う値を 1 つにまとめない。** 同じプロセスを別の binary_path で記録した
	// レコードが両方とも残り、値ごとに観測の回数が付く。
	if binaryPaths.ValueCount != int64(len(binaryPaths.Values)) || len(binaryPaths.Values) < 2 {
		t.Fatalf("the process carries the value count %d over %d binary paths, want two or more of each",
			binaryPaths.ValueCount, len(binaryPaths.Values))
	}
	seen := map[string]bool{}
	for at, value := range binaryPaths.Values {
		path, readable := value.Field.Text.ComparableValue()
		if !readable {
			t.Fatalf("binary path %d is %+v, want a readable value", at, value.Field)
		}
		if seen[path] {
			t.Errorf("binary path %d repeats the value %q", at, path)
		}
		seen[path] = true
		if value.ObservationCount < 1 {
			t.Errorf("binary path %q was observed %d times, want one or more",
				path, value.ObservationCount)
		}
	}
	// 詳細は根拠を全件返すため、根拠の総数と返した件数が一致する。
	if detail.EvidenceCount != len(detail.Evidence) {
		t.Errorf("the process carries %d evidence records over the total %d, want them equal",
			len(detail.Evidence), detail.EvidenceCount)
	}
}

// processNodes はプロセスのノードを走査の順で返す。
func processNodes(t *testing.T, graph Graph) []core.SubgraphNode {
	t.Helper()
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindProcess}, Depth: 0})
	return subgraph.Nodes
}

// **2 つの軸を別に読む。** observation はそのプロセスを記録したレコードの有無、
// creationRecord は生成のレコードの有無である。
func TestNodeObservationAndCreationRecordAreTwoAxes(t *testing.T) {
	nodes := processNodes(t, graphOf(t))
	// 並びは走査順である。{P1}、参照だけの {P0}@T1、{P2}、参照だけの {P0}@T2、
	// {P6}、{P5}、{P7}、通信だけが記録した {P8} になる。
	want := []struct {
		identity       string
		observation    core.NodeObservation
		creationRecord core.NodeCreationRecord
	}{
		{"{P1}", core.NodeObservationObserved, core.NodeCreationRecordPresent},
		{"{P0}", core.NodeObservationReferenced, core.NodeCreationRecordAbsent},
		{"{P2}", core.NodeObservationObserved, core.NodeCreationRecordPresent},
		{"{P0}", core.NodeObservationReferenced, core.NodeCreationRecordAbsent},
		{"{P6}", core.NodeObservationObserved, core.NodeCreationRecordPresent},
		{"{P5}", core.NodeObservationObserved, core.NodeCreationRecordPresent},
		{"{P7}", core.NodeObservationObserved, core.NodeCreationRecordPresent},
		{"{P8}", core.NodeObservationObserved, core.NodeCreationRecordAbsent},
	}
	if len(nodes) != len(want) {
		t.Fatalf("the graph carries %d processes, want %d", len(nodes), len(want))
	}
	for index, expected := range want {
		node := nodes[index]
		if node.Identity[1].Value != expected.identity {
			t.Fatalf("process %d is %q, want %q", index, node.Identity[1].Value, expected.identity)
		}
		if node.Observation != expected.observation ||
			node.CreationRecord != expected.creationRecord {
			t.Errorf("process %s carries %q/%q, want %q/%q", expected.identity,
				node.Observation, node.CreationRecord,
				expected.observation, expected.creationRecord)
		}
	}
}

// 生成を記録した根拠を持てない種別は item_absent を持つ。
func TestCreationRecordIsItemAbsentOutsideProcesses(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(wholeGraphQuery())
	checked := 0
	for _, node := range subgraph.Nodes {
		if node.Kind.CarriesCreationRecord() {
			continue
		}
		checked++
		if node.CreationRecord != core.NodeCreationRecordItemAbsent {
			t.Errorf("the %s node %q carries the creation record %q, want item_absent",
				node.Kind, node.Id, node.CreationRecord)
		}
	}
	if checked == 0 {
		t.Fatal("the subgraph carries no node outside processes")
	}
}

func TestNodeDetailSeparatesTheReferencedProcessFromTheObservedOne(t *testing.T) {
	graph := graphOf(t)
	nodes := processNodes(t, graph)
	// 参照した側のレコードの属性を、参照先のノードへ足さない。
	detail, found := graph.NodeDetail(nodes[1].Id)
	if !found {
		t.Fatalf("the node %q has no detail", nodes[1].Id)
	}
	if detail.AttributeCount != 0 {
		t.Errorf("the referenced process carries %d attributes, want 0", detail.AttributeCount)
	}
	if detail.EvidenceCount != 1 {
		t.Errorf("the referenced process carries %d evidence records, want 1", detail.EvidenceCount)
	}
	label, readable := detail.Node.Label.ComparableValue()
	if !readable || label != `C:\parent.exe` {
		t.Errorf(`the referenced process carries the label %q (readable %v), want C:\parent.exe`,
			label, readable)
	}
	// parentPath を持たないレコードが参照したプロセスは表示名を持たない。
	absent, found := graph.NodeDetail(nodes[3].Id)
	if !found {
		t.Fatalf("the node %q has no detail", nodes[3].Id)
	}
	if absent.Node.Label.ValueState != core.ValueStateItemAbsent {
		t.Errorf("the referenced process carries the label state %q, want item_absent",
			absent.Node.Label.ValueState)
	}
}

// parentChildEdges は親子の関係だけを走査の順で返す。
func parentChildEdges(t *testing.T, graph Graph) []core.GraphEdge {
	t.Helper()
	return graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindProcessParentChild}, Depth: 1}).Edges
}

// 同じ親の外部識別子を別の端末のレコードが指すとき、親のノードと親子の関係を
// 端末ごとに分ける。識別鍵が端末の外部識別子を持つためである。
func TestParentChildSeparatesTheTerminalsNamingTheSameParentId(t *testing.T) {
	graph := graphOf(t)
	nodes := processNodes(t, graph)
	firstParent, secondParent := nodes[1], nodes[3]
	for _, parent := range []core.SubgraphNode{firstParent, secondParent} {
		if parent.Identity[1].Value != "{P0}" {
			t.Fatalf("the referenced parent is %q, want {P0}", parent.Identity[1].Value)
		}
	}
	if firstParent.Identity[0].Value == secondParent.Identity[0].Value {
		t.Fatalf("the two parents name the terminal %q twice, want two terminals",
			firstParent.Identity[0].Value)
	}
	if firstParent.Id == secondParent.Id {
		t.Errorf("the parent %q on two terminals became one node", firstParent.Id)
	}
	sources := make(map[string]int)
	for _, edge := range parentChildEdges(t, graph) {
		sources[edge.SourceNodeId]++
	}
	if sources[firstParent.Id] != 1 || sources[secondParent.Id] != 1 {
		t.Errorf("the parents start %d and %d relations, want one each",
			sources[firstParent.Id], sources[secondParent.Id])
	}
}

// parentCreationRecord は端末 terminal のプロセス child が親 parent から起動したことを
// 記録したレコードを組む。
func parentCreationRecord(t *testing.T, line int64, terminal, child, parent string) RecordEntry {
	t.Helper()
	return RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			ProcessStart:    true,
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, terminal),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, child),
				syntheticField(t, "parentGUID", core.SemanticKeyParentProcessId, parent),
			},
		},
	}
}

// graphOfRecords は渡したレコードだけを持つ取り込み結果からグラフを組む。
func graphOfRecords(t *testing.T, records []RecordEntry) Graph {
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

// recordNodeDetailOf は、渡したレコード 1 件から組んだグラフのレコードのノードの詳細を返す。
func recordNodeDetailOf(t *testing.T, record RecordEntry) NodeDetail {
	t.Helper()
	graph := graphOfRecords(t, []RecordEntry{record})
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord}, Depth: 0})
	if len(subgraph.Nodes) != 1 {
		t.Fatalf("the graph carries %d record nodes, want 1", len(subgraph.Nodes))
	}
	detail, found := graph.NodeDetail(subgraph.Nodes[0].Id)
	if !found {
		t.Fatalf("the record node %q has no detail", subgraph.Nodes[0].Id)
	}
	return detail
}

// 語彙の項目を持つ欄は語彙の項目でまとまり、同じ意味を持つ 2 つの key が 1 つの属性になる。
// 語彙に写していない欄は原資料の key の文字列で別々の属性になる。
func TestRecordNodeGroupsTheAttributesByTheVocabularyAndTheRawKey(t *testing.T) {
	line := int64(1)
	detail := recordNodeDetailOf(t, RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				// 同じ意味を 2 つの key が持つ。1 つの属性にまとまる。
				syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
				syntheticField(t, "wsIp", core.SemanticKeyTerminalIpAddress, "192.0.2.9"),
				// 語彙に写していない 2 つの欄。key の文字列ごとに別の属性になる。
				syntheticField(t, "profile", "", "lab_server"),
				syntheticField(t, "loc", "", "ja-JP"),
			},
		},
	})
	grouped := make(map[string]int64, len(detail.Attributes))
	for _, attribute := range detail.Attributes {
		if (attribute.Semantic == "") == (attribute.Name == "") {
			t.Errorf("the attribute %+v carries both names or neither", attribute)
		}
		key := string(attribute.Semantic) + attribute.Name
		grouped[key] = attribute.ValueCount
	}
	want := map[string]int64{
		string(core.SemanticKeyTerminalId): 1, string(core.SemanticKeyTerminalIpAddress): 2,
		"profile": 1, "loc": 1,
	}
	if len(grouped) != len(want) {
		t.Fatalf("the record node carries the attributes %v, want %v", grouped, want)
	}
	for key, count := range want {
		if grouped[key] != count {
			t.Errorf("the attribute %q carries %d values, want %d", key, grouped[key], count)
		}
	}
}

// 対象を 1 つも指さないレコードもグラフに入り、読めた欄を属性に持つ。
func TestRecordNodeCarriesTheFieldsOfARecordNamingNoObject(t *testing.T) {
	line := int64(1)
	detail := recordNodeDetailOf(t, RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "msg", core.SemanticKeyEventMessage, "service restarted"),
				syntheticField(t, "loc", "", "ja-JP"),
			},
		},
	})
	carried := make([]string, 0, len(detail.Attributes))
	for _, attribute := range detail.Attributes {
		carried = append(carried, string(attribute.Semantic)+attribute.Name)
	}
	slices.Sort(carried)
	want := []string{"loc", string(core.SemanticKeyEventMessage)}
	slices.Sort(want)
	if !slices.Equal(carried, want) {
		t.Errorf("the record node carries the attributes %v, want %v", carried, want)
	}
	if len(detail.EdgeCounts) != 0 {
		t.Errorf("the record node carries %d edge counts, want none", len(detail.EdgeCounts))
	}
}

// 同じ親子の組を 2 件のレコードが指すとき、関係を 1 本にまとめ、根拠に 2 件を添える。
func TestParentChildFoldsTheRecordsNamingTheSamePair(t *testing.T) {
	graph := graphOfRecords(t, []RecordEntry{
		parentCreationRecord(t, 1, "T1", "{P1}", "{P0}"),
		parentCreationRecord(t, 2, "T1", "{P1}", "{P0}"),
		parentCreationRecord(t, 3, "T1", "{P2}", "{P0}"),
	})
	edges := parentChildEdges(t, graph)
	if len(edges) != 2 {
		t.Fatalf("the graph carries %d parent and child relations, want 2", len(edges))
	}
	byEvidence := make(map[int64]int)
	for _, edge := range edges {
		byEvidence[edge.EvidenceCount]++
	}
	if byEvidence[2] != 1 || byEvidence[1] != 1 {
		t.Fatalf("the relations carry the evidence counts %v, want one relation with 2 and one with 1",
			byEvidence)
	}
	for _, edge := range edges {
		returned := edgeEvidenceOf(t, graph, edge.Id)
		if edge.EvidenceCount != int64(len(returned)) {
			t.Errorf("the relation %q counts %d evidence records and returns %d",
				edge.Id, edge.EvidenceCount, len(returned))
		}
	}
}

// 親の外部識別子を持つ要素を持たないレコードは、親子の関係を作らない。
func TestParentChildIsAbsentWithoutTheParentId(t *testing.T) {
	line := int64(1)
	orphan := RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			ProcessStart:    true,
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
			},
		},
	}
	graph := graphOfRecords(t, []RecordEntry{orphan})
	if edges := parentChildEdges(t, graph); len(edges) != 0 {
		t.Errorf("the record without a parent id started %d relations, want 0", len(edges))
	}
	for _, node := range processNodes(t, graph) {
		if node.Observation != core.NodeObservationObserved {
			t.Errorf("the process %q carries the observation %q, want observed",
				node.Identity[1].Value, node.Observation)
		}
	}
}

// 注入のレコードは、注入元から注入先への向きを持つ関係を観測の状態で作る。
// 注入先は起動のレコードを持たない参照だけのノードになる。
func TestInjectionEdgeRunsFromTheInjectorToTheTarget(t *testing.T) {
	line := int64(1)
	injection := RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
				syntheticField(t, "tpsGUID", core.SemanticKeyInjectionTargetProcessId, "{P9}"),
				syntheticField(t, "tpsPath",
					core.SemanticKeyInjectionTargetProcessBinaryPath, `C:\target.exe`),
			},
		},
	}
	graph := graphOfRecords(t, []RecordEntry{injection})
	edges := graph.Query(GraphQuery{EdgeKinds: []core.EdgeKind{core.EdgeKindProcessInjection}, Depth: 1}).Edges
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d injection relations, want 1", len(edges))
	}
	if edges[0].State != core.RelationStateObserved {
		t.Errorf("the injection relation carries the state %q, want observed", edges[0].State)
	}
	detail, found := graph.EdgeDetail(edges[0].Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the injection relation %q has no detail", edges[0].Id)
	}
	if detail.SourceNode.Identity[1].Value != "{P1}" ||
		detail.TargetNode.Identity[1].Value != "{P9}" {
		t.Fatalf("the injection runs from %q to %q, want {P1} to {P9}",
			detail.SourceNode.Identity[1].Value, detail.TargetNode.Identity[1].Value)
	}
	if detail.SourceNode.Observation != core.NodeObservationObserved ||
		detail.TargetNode.Observation != core.NodeObservationReferenced {
		t.Errorf("the injection endpoints carry %q and %q, want observed and referenced",
			detail.SourceNode.Observation, detail.TargetNode.Observation)
	}
	if detail.TargetNode.CreationRecord != core.NodeCreationRecordAbsent {
		t.Errorf("the injection target carries the creation record %q, want absent",
			detail.TargetNode.CreationRecord)
	}
	label, readable := detail.TargetNode.Label.ComparableValue()
	if !readable || label != `C:\target.exe` {
		t.Errorf(`the injection target carries the label %q (readable %v), want C:\target.exe`,
			label, readable)
	}
}

func TestEdgeDetailCarriesTheConditionsOfTheCandidateEdge(t *testing.T) {
	graph := graphOf(t)
	candidate := candidateEdgeFrom(t, graph, core.NodeKindIp)
	if candidate.State != core.RelationStateCandidate {
		t.Errorf("the candidate edge carries the state %q, want candidate", candidate.State)
	}
	detail, found := graph.EdgeDetail(candidate.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the edge %q has no detail", candidate.Id)
	}
	// **2 件以上の関連付けを 1 件にまとめない。** 同じ起点が con と est の 2 件を候補に挙げる。
	matches := detailMatches(t, detail)
	if detail.MatchCount != 2 || len(matches) != 2 {
		t.Fatalf("the candidate edge carries %d matches, want 2", detail.MatchCount)
	}
	for index, wantSequence := range []int64{9, 10} {
		element := matches[index]
		if element.OriginRef.LineNumber == nil || *element.OriginRef.LineNumber != 2 {
			t.Errorf("match %d starts at %+v, want the Squid record on the line 2",
				index, element.OriginRef)
		}
		if element.CandidateRef.SequenceNumber == nil ||
			*element.CandidateRef.SequenceNumber != wantSequence {
			t.Errorf("match %d points at %+v, want the sequence number %d",
				index, element.CandidateRef, wantSequence)
		}
	}
	match := matches[0]
	if match.StageKey != core.StageKeySecondTimeMatched {
		t.Errorf("the match comes from the stage %q, want second_time_matched", match.StageKey)
	}
	// 段階は並び順に全数を持ち、末尾の段階がこの関連付けを出した段階である。
	wantStages := []core.StageKey{core.StageKeyClockIndependent, core.StageKeySecondTimeMatched}
	gotStages := make([]core.StageKey, 0, len(match.StageTallies))
	for _, tally := range match.StageTallies {
		gotStages = append(gotStages, tally.StageKey)
	}
	if !slices.Equal(gotStages, wantStages) {
		t.Fatalf("the match carries the stages %+v, want %+v", gotStages, wantStages)
	}
	// 段階 1 は接続先 IP と接続先 port と端末の割当で sn=9 と sn=10 を挙げ、段階 2 は同じ秒で
	// 絞る。2 件はどちらも 13:00:09 であるため減らず、指すプロセスはどちらも 1 つである。
	for _, tally := range match.StageTallies {
		if tally.MemberCount != int64(detail.MatchCount) {
			t.Errorf("the stage %q reports %d candidates, want %d",
				tally.StageKey, tally.MemberCount, detail.MatchCount)
		}
		if tally.DistinctProcessCount == nil || *tally.DistinctProcessCount != 1 {
			t.Errorf("the stage %q reports %+v distinct processes, want 1",
				tally.StageKey, tally.DistinctProcessCount)
		}
	}
	// 用いた条件の値がすべて同じ 2 件は、互いに区別できない 1 組になる。
	if len(match.IndistinguishableGroups) != 1 || len(match.IndistinguishableGroups[0]) != 2 {
		t.Fatalf("the match carries the indistinguishable groups %+v, want one group of 2",
			match.IndistinguishableGroups)
	}
	used := make([]string, 0, len(match.Conditions))
	for _, condition := range match.Conditions {
		if condition.Use == core.ConditionUseUsed {
			used = append(used, string(condition.ConditionKey))
		}
	}
	wantUsed := []string{
		"terminal_ip_assignment", "destination_ip", "destination_port", "second_of_time",
	}
	if len(used) != len(wantUsed) {
		t.Fatalf("the match used the conditions %v, want %v", used, wantUsed)
	}
	for index, key := range wantUsed {
		if used[index] != key {
			t.Errorf("used condition %d is %q, want %q", index, used[index], key)
		}
	}
	if match.TimeComparison.ComparisonUnit != core.ComparisonUnitSecond {
		t.Errorf("the match compared time in %q, want second",
			match.TimeComparison.ComparisonUnit)
	}
	if len(match.Assumptions) != 1 ||
		match.Assumptions[0].AssumptionKey != core.AssumptionKeyClockOffsetBelowOneSecond {
		t.Errorf("the match rests on the assumptions %+v, want clock_offset_below_one_second",
			match.Assumptions)
	}
	// **前提と別の項目である。** 段階 1 から引き継ぐ割当の条件が時計に依拠する箇所を運ぶ。
	// **値は段階 1 から写す。** 同じ定数を 2 か所に書かない。
	requireClockDependencyNoteFromFirstStage(t, match.ClockDependencyNote)
	if len(match.UnresolvedReasons) == 0 {
		t.Error("the match carries no unresolved reason, want at least one")
	}
	// **根拠は起点 1 件と候補 2 件の 3 件である。** 起点のレコードを候補ごとに入れ直さない。
	if detail.Edge.EvidenceCount != 3 || len(detail.Evidence) != 3 {
		t.Fatalf("the candidate edge carries %d evidence records, want 3",
			detail.Edge.EvidenceCount)
	}
	if detail.SourceNode.Id != candidate.SourceNodeId ||
		detail.TargetNode.Id != candidate.TargetNodeId {
		t.Errorf("the detail carries the nodes %q and %q, want %q and %q",
			detail.SourceNode.Id, detail.TargetNode.Id,
			candidate.SourceNodeId, candidate.TargetNodeId)
	}
}

// 候補として数えるレコードを選ぶ条件が、同じ接続先と同じ秒の dcon を除く。
func TestCandidateEdgeExcludesTheObservationKindOutsideTheSelector(t *testing.T) {
	graph := graphOf(t)
	candidate := candidateEdgeFrom(t, graph, core.NodeKindIp)
	detail, found := graph.EdgeDetail(candidate.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the edge %q has no detail", candidate.Id)
	}
	matches := detailMatches(t, detail)
	for _, match := range matches {
		if match.CandidateRef.SequenceNumber != nil &&
			*match.CandidateRef.SequenceNumber == 11 {
			t.Errorf("the match points at the sequence number 11, want the dcon record excluded")
		}
	}
	if len(matches) == 0 {
		t.Fatal("the candidate edge carries no match")
	}
	conditions := matches[0].Conditions
	if len(conditions) == 0 {
		t.Fatal("the match carries no condition")
	}
	// 選ぶ条件そのものは要求の項目であり、応答の conditions に入らない。候補の側に
	// dcon のレコードが残っていないことで、絞り込みが作用したことを確かめる。
	for _, evidence := range detail.Evidence {
		if evidence.RecordRef.SequenceNumber != nil &&
			*evidence.RecordRef.SequenceNumber == 11 {
			t.Errorf("the evidence carries the sequence number 11, want the dcon record excluded")
		}
	}
}

// candidateOriginsManifest は graph-manifest.json の起点の数え上げの期待値である。
type candidateOriginsManifest struct {
	Outcomes                 []string `json:"outcomes"`
	UnreadableCandidateCount int      `json:"unreadableCandidateCount"`
	DroppedCandidateCount    int      `json:"droppedCandidateCount"`
	Counts                   []struct {
		Outcome     string `json:"outcome"`
		OriginCount int    `json:"originCount"`
	} `json:"counts"`
}

func candidateOriginsFixture(t *testing.T) candidateOriginsManifest {
	t.Helper()
	data, err := os.ReadFile(graphFixtureDir + "graph-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		CandidateOrigins candidateOriginsManifest `json:"candidateOrigins"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.CandidateOrigins.Outcomes) == 0 {
		t.Fatal("the manifest carries no candidate origin outcome")
	}
	if len(manifest.CandidateOrigins.Counts) == 0 {
		t.Fatal("the manifest carries no candidate origin count")
	}
	return manifest.CandidateOrigins
}

// requireClockDependencyNoteFromFirstStage は、関連付けが持つ時計への依拠が段階 1 から
// 写した値であることを確かめる。
func requireClockDependencyNoteFromFirstStage(t *testing.T, got string) {
	t.Helper()
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	_, _, set := matchedOrigin(t, &graph, result)
	first, found := stageOf(set, core.StageKeyClockIndependent)
	if !found {
		t.Fatal("the candidate set carries no first stage")
	}
	if first.ClockDependencyNote == "" {
		t.Fatal("the first stage carries no clock dependency note")
	}
	if got != first.ClockDependencyNote {
		t.Errorf("the match carries the clock dependency note %q, want the first stage value %q",
			got, first.ClockDependencyNote)
	}
}

// **候補のエッジの端末の導出は、`/api/v0/records` の clientTerminal と同じ文字列である。**
// 導出は `/api/v0/edges/{id}` の conditions の leftValue として分析者に見える。
func TestCandidateEdgeTerminalDerivationMatchesTheRecordsResponse(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	origin, _, _ := matchedOrigin(t, &graph, result)
	assignment, resolved := resolveOriginTerminal(
		clientAssignmentsOf(terminalAssignmentsOf(result)), origin)
	if !resolved {
		t.Fatal("the origin carries no determined terminal")
	}
	specs := composeSpecs(
		t, origin, collectCandidateSides(result).candidateDeclarations, AllMatchConditions())
	observation, built := originObservationOf(origin, assignment, specs)
	if !built {
		t.Fatal("the origin observation is not built")
	}
	matching, found := observation.FieldBySemantic(core.SemanticKeyTerminalId)
	if !found || matching.Text == nil || matching.Text.Derivation == nil {
		t.Fatalf("the matching terminal field is %+v, want a derivation", matching)
	}
	// 同じレコードについて `/api/v0/records` が組む fields を求める。
	response, err := NewFieldsBuilder(result).Build(origin.record)
	if err != nil {
		t.Fatal(err)
	}
	responseField, present := firstFieldWithSemantic(response, core.SemanticKeyTerminalId)
	if !present || responseField.Text == nil || responseField.Text.Derivation == nil {
		t.Fatalf("the response terminal field is %+v, want a derivation", responseField)
	}
	if *matching.Text.Derivation != *responseField.Text.Derivation {
		t.Errorf("the matching derivation is %q, want the response derivation %q",
			*matching.Text.Derivation, *responseField.Text.Derivation)
	}
}

// matchedOrigin は候補のエッジを挙げた起点 1 件と、その候補集合を返す。
func matchedOrigin(
	t *testing.T, graph *Graph, result ImportResult,
) (matchSide, string, core.CandidateSet) {
	t.Helper()
	sides := collectCandidateSides(result)
	assignments := clientAssignmentsOf(terminalAssignmentsOf(result))
	for _, origin := range sides.origins {
		ip, hasIp := comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationAddress)
		port, hasPort := comparableOfSemantic(
			origin.fields, core.SemanticKeyConnectionDestinationPort)
		if !hasIp || !hasPort {
			continue
		}
		request, _, outcome := graph.matchRequestOf(result, assignments, origin,
			newCandidatePool(sides.byDestination[destinationKey{ip: ip, port: port}]),
			sides.candidateDeclarations)
		if outcome != core.RelationDerivationMatched {
			continue
		}
		set, err := core.BuildCandidateSet(request)
		if err != nil {
			t.Fatal(err)
		}
		stage, found := stageOf(set, core.StageKeySecondTimeMatched)
		if found && len(stage.Members) > 0 {
			return origin, ip, set
		}
	}
	t.Fatal("no origin carries a second stage candidate")
	return matchSide{}, "", core.CandidateSet{}
}

// secondStageOf は候補のエッジを挙げた起点 1 件と、その段階 2 を返す。
func secondStageOf(t *testing.T, result ImportResult) (matchSide, string, core.CandidateStage) {
	t.Helper()
	graph := NewGraph(result, AllMatchConditions())
	origin, ip, set := matchedOrigin(t, &graph, result)
	stage, found := stageOf(set, core.StageKeySecondTimeMatched)
	if !found {
		t.Fatal("the candidate set carries no second stage")
	}
	return origin, ip, stage
}

// **写せなかった候補を通知せずに捨てない。** 段階が挙げた候補をグラフから探せない起点は
// node_unresolved になり、探せなかった候補の件数が残る。
func TestApplyCandidateSetCountsTheCandidatesItCannotResolve(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	origin, destinationIp, set := matchedOrigin(t, &graph, result)
	stage, _ := stageOf(set, core.StageKeySecondTimeMatched)
	before := graph.CandidateRecordCounts().Dropped
	// 候補の集合を空で渡す。段階の候補に対応するレコードをグラフから探せない状態である。
	outcome := graph.applyCandidateSet(set, nil, origin, destinationIp, nil)
	if outcome != core.RelationDerivationNodeUnresolved {
		t.Errorf("the origin is classified as %q, want node_unresolved", outcome)
	}
	if dropped := graph.CandidateRecordCounts().Dropped; dropped != before+len(stage.Members) {
		t.Errorf("the graph dropped %d candidates, want %d",
			dropped, before+len(stage.Members))
	}
}

// **求めた段階を持たない候補集合は内部で処理できなかったことである。** 原資料について言える事実として
// 数えない。
func TestApplyCandidateSetReportsTheMissingStageAsAnInternalLoss(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	origin, destinationIp, set := matchedOrigin(t, &graph, result)
	withoutFirst := set
	withoutFirst.Stages = nil
	for _, stage := range set.Stages {
		if stage.StageKey == core.StageKeySecondTimeMatched {
			withoutFirst.Stages = append(withoutFirst.Stages, stage)
		}
	}
	before := graph.CandidateRecordCounts().Dropped
	outcome := graph.applyCandidateSet(withoutFirst, nil, origin, destinationIp, nil)
	if outcome != core.RelationDerivationNodeUnresolved {
		t.Errorf("a candidate set without the first stage is classified as %q, want node_unresolved",
			outcome)
	}
	// **段階を確かめる分岐が候補を数える手前にある。** 分岐を外すと、段階 2 の候補を 1 件ずつ
	// 探しに行って全件を除き、除いた件数が増える。
	if dropped := graph.CandidateRecordCounts().Dropped; dropped != before {
		t.Errorf("the graph dropped %d candidates, want the count to stay at %d",
			dropped, before)
	}
}

// **候補が 0 件になった理由を core.EmptyReason の粒度で分ける。**
func TestEmptyStageOutcomeSeparatesTheReasons(t *testing.T) {
	for _, testCase := range []struct {
		reason core.EmptyReason
		want   core.RelationDerivationOutcome
	}{
		{core.EmptyReasonNoCandidateInWindow, core.RelationDerivationNoCandidateInWindow},
		{
			core.EmptyReasonNoCandidateMatchingConditions,
			core.RelationDerivationNoCandidateMatchingConditions,
		},
		// counterpart_item_absent は Squid と markii 形式の組で起こらない。
		{core.EmptyReasonCounterpartItemAbsent, core.RelationDerivationNodeUnresolved},
		{"", core.RelationDerivationNodeUnresolved},
	} {
		if got := emptyStageOutcome(testCase.reason); got != testCase.want {
			t.Errorf("the empty reason %q is classified as %q, want %q",
				testCase.reason, got, testCase.want)
		}
	}
}

// **候補のレコードが 0 件である状態と、候補の値を読めない状態を分ける。**
func TestMatchRequestSeparatesTheUnreadableCandidateFromTheAbsentRecord(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	sides := collectCandidateSides(result)
	assignments := clientAssignmentsOf(terminalAssignmentsOf(result))
	origin, destinationIp, _ := secondStageOf(t, result)
	destinationPort, readable := comparableOfSemantic(
		origin.fields, core.SemanticKeyConnectionDestinationPort)
	if !readable {
		t.Fatal("the origin carries no destination port")
	}
	candidates := sides.byDestination[destinationKey{ip: destinationIp, port: destinationPort}]
	if len(candidates) == 0 {
		t.Fatal("the destination carries no candidate record")
	}
	// 候補の側から関連付けが比べる語彙の項目を外す。レコードは実在するが値を比べられない。
	stripped := make([]matchSide, 0, len(candidates))
	for _, candidate := range candidates {
		side := candidate
		side.fields = make([]core.RecordField, 0, len(candidate.fields))
		for _, field := range candidate.fields {
			if field.Semantic == core.SemanticKeyEventTime {
				continue
			}
			side.fields = append(side.fields, field)
		}
		stripped = append(stripped, side)
	}
	before := graph.CandidateRecordCounts().Unreadable
	if _, _, outcome := graph.matchRequestOf(result, assignments, origin, newCandidatePool(stripped),
		sides.candidateDeclarations); outcome != core.RelationDerivationCandidateItemUnreadable {
		t.Errorf("the origin is classified as %q, want candidate_item_unreadable", outcome)
	}
	// **読めなかった候補を 1 件ずつ数える。** 全件が読めない起点でも件数が残る。
	if unreadable := graph.CandidateRecordCounts().Unreadable; unreadable != before+len(stripped) {
		t.Errorf("the graph counted %d unreadable candidates, want %d",
			unreadable, before+len(stripped))
	}
	// 値を読める候補では関連付けの要求が組める。
	if _, _, outcome := graph.matchRequestOf(result, assignments, origin, newCandidatePool(candidates),
		sides.candidateDeclarations); outcome != core.RelationDerivationMatched {
		t.Errorf("the origin is classified as %q, want matched", outcome)
	}
}

// **分類の文字列の列と、分類ごとの件数を確かめる。**
//
// 文字列の列は manifest が全数を並び順で持つ。**件数 0 の分類も要素である**
// (CandidateOriginCounts の doc コメント)。実装が分類を除いた状態、件数 0 の分類を
// 結果から省いた状態、文字列を書き誤った状態を、列の食い違いで見つける。
//
// 件数は fixture が通る分類だけを manifest が持つ。挙げていない分類の件数が 0 で
// あることは、合計の一致が決める。起点を別の分類へ移すと合計か件数が食い違って失敗する。
func TestCandidateOriginCountsSeparateTheOutcomes(t *testing.T) {
	want := candidateOriginsFixture(t)
	graph := graphOf(t)
	counts := graph.CandidateOriginCounts()
	byOutcome := make(map[string]int, len(counts))
	outcomes := make([]string, 0, len(counts))
	total := 0
	for _, count := range counts {
		byOutcome[string(count.Outcome)] = count.OriginCount
		outcomes = append(outcomes, string(count.Outcome))
		total += count.OriginCount
	}
	if !slices.Equal(outcomes, want.Outcomes) {
		t.Fatalf("the graph counts the outcomes %v, want %v", outcomes, want.Outcomes)
	}
	wantTotal := 0
	for _, expected := range want.Counts {
		got, named := byOutcome[expected.Outcome]
		if !named {
			t.Errorf("the graph names no outcome %q", expected.Outcome)
			continue
		}
		if got != expected.OriginCount {
			t.Errorf("outcome %q counted %d origins, want %d",
				expected.Outcome, got, expected.OriginCount)
		}
		wantTotal += expected.OriginCount
	}
	// 合計が一致することで、manifest が挙げていない分類の件数が 0 に決まる。
	if total != wantTotal {
		t.Errorf("the graph counted %d origins over every outcome, want %d", total, wantTotal)
	}
	records := graph.CandidateRecordCounts()
	if records.Unreadable != want.UnreadableCandidateCount ||
		records.Dropped != want.DroppedCandidateCount {
		t.Errorf("the graph counted %d unreadable and %d dropped candidates, want %d and %d",
			records.Unreadable, records.Dropped,
			want.UnreadableCandidateCount, want.DroppedCandidateCount)
	}
}

func TestEdgeDetailCarriesNoMatchOnAnObservedEdge(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(wholeGraphQuery())
	observed := core.GraphEdge{}
	for _, edge := range subgraph.Edges {
		if edge.Kind == core.EdgeKindProcessParentChild {
			observed = edge
			break
		}
	}
	if observed.Id == "" {
		t.Fatal("the graph carries no process_parent_child edge")
	}
	detail, found := graph.EdgeDetail(observed.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the edge %q has no detail", observed.Id)
	}
	if detail.Edge.State != core.RelationStateObserved {
		t.Errorf("the parent and child edge carries the state %q, want observed",
			detail.Edge.State)
	}
	if detail.MatchCount != 0 || len(detail.MatchTable.Matches) != 0 {
		t.Errorf("the observed edge carries %d matches, want 0", detail.MatchCount)
	}
	if detail.SourceNode.Observation != core.NodeObservationReferenced {
		t.Errorf("the parent node carries the observation %q, want referenced",
			detail.SourceNode.Observation)
	}
}

func TestEdgeDetailRejectsAnUnknownEdgeId(t *testing.T) {
	if _, found := graphOf(t).EdgeDetail("e:ran_on:absent", EdgeEvidenceFilter{}); found {
		t.Error("an unknown edge id returned a detail, want none")
	}
}

// **エッジの詳細は根拠と関連付けを全件返す。** 返した要素数が、同じ応答が述べる総数に等しい。
func TestEdgeDetailReturnsEveryEvidenceAndMatch(t *testing.T) {
	graph := graphOf(t)
	candidate := candidateEdgeFrom(t, graph, core.NodeKindIp)
	detail, found := graph.EdgeDetail(candidate.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the edge %q has no detail", candidate.Id)
	}
	matches := detailMatches(t, detail)
	if len(detail.Evidence) == 0 || len(matches) == 0 {
		t.Fatalf("the detail carries %d evidence records and %d matches, want both populated",
			len(detail.Evidence), len(matches))
	}
	if int64(len(detail.Evidence)) != detail.Edge.EvidenceCount {
		t.Errorf("the detail returned %d evidence records, want the edge count %d",
			len(detail.Evidence), detail.Edge.EvidenceCount)
	}
	if len(matches) != detail.MatchCount {
		t.Errorf("the detail returned %d matches, want the count %d",
			len(matches), detail.MatchCount)
	}
}

// **ノードの詳細は属性と根拠を全件返す。** 返した要素数が、同じ応答が述べる総数に等しい。
func TestNodeDetailReturnsEveryAttributeAndEvidence(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}, Depth: 0})
	detail, found := graph.NodeDetail(subgraph.Nodes[0].Id)
	if !found {
		t.Fatalf("the node %q has no detail", subgraph.Nodes[0].Id)
	}
	if len(detail.Attributes) == 0 || len(detail.Evidence) == 0 {
		t.Fatalf("the detail carries %d attributes and %d evidence records, want both populated",
			len(detail.Attributes), len(detail.Evidence))
	}
	if len(detail.Attributes) != detail.AttributeCount {
		t.Errorf("the detail returned %d attributes, want the count %d",
			len(detail.Attributes), detail.AttributeCount)
	}
	if len(detail.Evidence) != detail.EvidenceCount {
		t.Errorf("the detail returned %d evidence records, want the count %d",
			len(detail.Evidence), detail.EvidenceCount)
	}
}

func TestNodeDetailCountsEdgesByKindAndDirection(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}, Depth: 0})
	detail, found := graph.NodeDetail(subgraph.Nodes[0].Id)
	if !found {
		t.Fatalf("the node %q has no detail", subgraph.Nodes[0].Id)
	}
	// 端末 T1 を指すレコードは原文の 15 行である。16 行のうち 7 行目だけが端末 T2 を
	// 指す。
	want := map[string]int64{
		"terminal_address outgoing": 1, "terminal_outbound_connection outgoing": 1,
		"terminal_account outgoing": 2, "ran_on incoming": 5, "record_names_object incoming": 15,
	}
	got := make(map[string]int64, len(detail.EdgeCounts))
	for _, count := range detail.EdgeCounts {
		got[string(count.EdgeKind)+" "+string(count.Direction)] = count.EdgeCount
	}
	if len(got) != len(want) {
		t.Fatalf("the terminal carries the edge counts %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("the edge count %q is %d, want %d", key, got[key], value)
		}
	}
}

func TestNodeDetailRejectsAnUnknownNodeId(t *testing.T) {
	graph := graphOf(t)
	if _, found := graph.NodeDetail("n:terminal:absent"); found {
		t.Error("an unknown node id returned a detail, want none")
	}
	if graph.HasNode("n:terminal:absent") {
		t.Error("an unknown node id is reported as present")
	}
}

func TestGraphSkipsWithheldSources(t *testing.T) {
	markii := scanIndexSource(t, NewTestMarkIIParser(), "graph-markii.log",
		MarkIIFormatKey, graphSourceText(t, "graph-markii.log"))
	status := settleStatus(t, markii, "markii")
	// 同じ sourceId を 2 件並べると識別子の衝突で両方の公開が止まる (withhold.go)。
	result, err := newImportResult([]scannedSource{markii, markii},
		[]core.ImportStatus{status, status}, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	if graph := NewGraph(result, AllMatchConditions()); graph.nodeCount() != 0 || graph.edgeCount() != 0 {
		t.Errorf("the graph carries %d nodes and %d edges from withheld sources, want 0 and 0",
			graph.nodeCount(), graph.edgeCount())
	}
}

func TestGraphFieldsMergeTheTerminalAddresses(t *testing.T) {
	result := graphResult(t)
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	publication, ok := result.Publication(entries[0].Identity.SourceId)
	if !ok {
		t.Fatal("the markii source is not published")
	}
	records := publication.Records()
	fields := graphFieldsOf(records[0])
	addresses := fieldsWithSemantic(fields, core.SemanticKeyTerminalIpAddress)
	if len(addresses) != 1 {
		t.Fatalf("the merged fields carry %d terminal addresses, want 1", len(addresses))
	}
	if terminals := fieldsWithSemantic(fields, core.SemanticKeyTerminalId); len(terminals) != 1 {
		t.Errorf("the merged fields carry %d terminal identifiers, want 1", len(terminals))
	}
	if !strings.Contains(records[0].RawText, "sn=1 ") {
		t.Errorf("the first record is %q, want the record with the sequence number 1",
			records[0].RawText)
	}
}

// 分類ごとの集計が、起点ごとの結果から導かれる。2 つの数え方がずれない。
func TestCandidateOriginCountsFollowThePerOriginOutcomes(t *testing.T) {
	graph := graphOf(t)
	counts := graph.CandidateOriginCounts()

	perOrigin := make(map[core.RelationDerivationOutcome]int)
	var records int
	for _, node := range graph.nodes {
		if node.key.Kind != core.NodeKindRecord {
			continue
		}
		records++
		derivation, carried := graph.RelationDerivationOf(node.id)
		if !carried {
			t.Fatalf("the record node %q carries no derivation", node.id)
		}
		if derivation.Outcome == core.RelationDerivationNotUsedAsOrigin {
			continue
		}
		perOrigin[derivation.Outcome]++
	}
	if records == 0 {
		t.Fatal("the fixture carries no record node")
	}

	var counted int
	for _, count := range counts {
		if count.OriginCount != perOrigin[count.Outcome] {
			t.Fatalf("the tally of %q is %d, the per-origin outcomes give %d",
				count.Outcome, count.OriginCount, perOrigin[count.Outcome])
		}
		counted += count.OriginCount
	}
	// 集計の総和は、起点になったレコードとノードを組めなかった起点の和である。
	if want := len(graph.originOutcomes) + graph.OriginsWithoutRecordNode(); counted != want {
		t.Fatalf("the tally covers %d origins, want %d", counted, want)
	}
	// 起点にならなかったレコードを集計の母集団に入れない。
	if counted >= records {
		t.Fatalf("the tally covers %d of the %d record nodes, "+
			"want fewer than every record", counted, records)
	}
}

// 起点にならなかったレコードと、起点になって結果を出したレコードを別の値で示す。
func TestRelationDerivationSeparatesTheRecordsThatWereNotOrigins(t *testing.T) {
	graph := graphOf(t)
	var origins, others int
	for _, node := range graph.nodes {
		if node.key.Kind != core.NodeKindRecord {
			continue
		}
		derivation, _ := graph.RelationDerivationOf(node.id)
		if derivation.Outcome == core.RelationDerivationNotUsedAsOrigin {
			others++
			continue
		}
		origins++
	}
	if origins == 0 {
		t.Fatal("no record node was used as an origin")
	}
	if others == 0 {
		t.Fatal("every record node was used as an origin, want some that were not")
	}
}

// レコード以外の種別のノードは、関係を導いた結果を持たない。
func TestRelationDerivationIsAbsentForOtherKinds(t *testing.T) {
	graph := graphOf(t)
	var checked int
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindRecord {
			continue
		}
		if _, carried := graph.RelationDerivationOf(node.id); carried {
			t.Fatalf("the %s node %q carries a derivation", node.key.Kind, node.id)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the fixture carries no node of a kind other than record")
	}
}

// **絞る条件を持たない関連付けを、内部で処理できなかったことと混ぜない。** 収集元が候補の絞りに使える
// 条件を分析者がすべて外すと、関連付けは候補を挙げる材料を持たない。この状態を
// candidate_set_failed に数えると、分析者は実装の不具合と読む。
func TestTheSelectionWithoutANarrowingConditionIsNotAnInternalGap(t *testing.T) {
	// 収集元が NarrowsCandidates に挙げない条件だけを選ぶ。
	selection := MatchConditionSelection{Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyUser},
		{ConditionKey: core.ConditionKeyProcess},
	}}
	graph := NewGraph(graphResult(t), selection)
	byOutcome := make(map[core.RelationDerivationOutcome]int)
	for _, count := range graph.CandidateOriginCounts() {
		byOutcome[count.Outcome] = count.OriginCount
	}
	if byOutcome[core.RelationDerivationNoComparedCondition] == 0 {
		t.Fatalf("no origin is counted as %q", core.RelationDerivationNoComparedCondition)
	}
	if failed := byOutcome[core.RelationDerivationFailed]; failed != 0 {
		t.Errorf("%d origins are counted as %q", failed, core.RelationDerivationFailed)
	}
	if problem := graph.CandidateSetProblem(); problem != nil {
		t.Errorf("the graph carries the candidate set problem %v", problem)
	}
	// 分類の区分は、分析者が選択を変えれば変わることを表す。
	derivation, err := core.NewRelationDerivation(core.RelationDerivationNoComparedCondition)
	if err != nil {
		t.Fatal(err)
	}
	if derivation.Basis != core.RelationDerivationBasisAnalystSelection {
		t.Errorf("the basis is %q, want %q",
			derivation.Basis, core.RelationDerivationBasisAnalystSelection)
	}
}

// candidateStageTalliesOf は、候補のエッジが持つ関連付けの段階 1 の候補の総数を、
// 起点のレコードの位置ごとに返す。
func candidateStageTalliesOf(t *testing.T, graph Graph) map[int64]int64 {
	t.Helper()
	counted := make(map[int64]int64)
	for _, edge := range graph.edges {
		for _, match := range graph.edgeMatchesOf(edge) {
			if match.OriginRef.LineNumber == nil {
				continue
			}
			counted[*match.OriginRef.LineNumber] = match.StageTallies[0].MemberCount
		}
	}
	return counted
}

// **母集合を探す鍵も分析者の選択に従う。** 接続先 port を選択から外すと、その欄だけが
// 違うレコードが候補に入る。母集合を接続先の文字列で探したままにすると、応答は「その条件で
// 絞っていない」と書きながら、実際には絞った結果を出すことになる。
//
// fixture の候補の側は、同じ接続先 IP に対して port が 8080 のレコード (sn=9, sn=10) と
// 443 のレコード (sn=5, sn=15) を持つ。
func TestDroppingTheDestinationPortWidensTheCandidatePopulation(t *testing.T) {
	withPort := NewGraph(graphResult(t), AllMatchConditions())
	selection := MatchConditionSelection{Conditions: []SelectedMatchCondition{}}
	for _, condition := range AllMatchConditions().Conditions {
		if condition.ConditionKey == core.ConditionKeyDestinationPort {
			continue
		}
		selection.Conditions = append(selection.Conditions, condition)
	}
	withoutPort := NewGraph(graphResult(t), selection)

	narrow := candidateStageTalliesOf(t, withPort)
	wide := candidateStageTalliesOf(t, withoutPort)
	if len(narrow) == 0 {
		t.Fatal("the graph with every condition carries no match")
	}
	widened := false
	for line, count := range narrow {
		got, present := wide[line]
		if !present {
			t.Errorf("the origin at the line %d lost its match without the port condition", line)
			continue
		}
		if got < count {
			t.Errorf("the origin at the line %d counts %d candidates without the port condition "+
				"and %d with it, want no fewer", line, got, count)
		}
		if got > count {
			widened = true
		}
	}
	if !widened {
		t.Error("dropping the destination port condition widened no candidate population")
	}
}

// **接続先の条件を両方外した要求は、接続先の欄を持たないレコードも候補にする。**
// 欄の有無で候補を除くと、応答は「その条件で絞っていない」と書きながら、実際には
// その欄で絞った結果を出すことになる。
func TestTheCandidatePopulationKeepsRecordsWithoutTheDestination(t *testing.T) {
	sides := collectCandidateSides(graphResult(t))
	if len(sides.candidates) == 0 {
		t.Fatal("the candidate side carries no record")
	}
	indexed := 0
	for _, group := range sides.byDestination {
		indexed += len(group)
	}
	if len(sides.candidates) < indexed {
		t.Fatalf("the population carries %d records and the destination index %d, "+
			"want the population to carry no fewer", len(sides.candidates), indexed)
	}
	// 母集合に入るのは、プロセスを記録したレコードである。
	for _, candidate := range sides.candidates {
		if candidate.record.Semantics.ProcessRef == nil {
			t.Fatalf("the record at %+v carries no process and is in the population",
				candidate.record.Locator)
		}
	}
	// 接続先の文字列で探す表に入るのは、両方の欄を比べられるレコードだけである。
	for _, group := range sides.byDestination {
		for _, candidate := range group {
			if _, hasIp := comparableOfSemantic(candidate.fields,
				core.SemanticKeyConnectionDestinationAddress); !hasIp {
				t.Errorf("the record at %+v is indexed without a comparable destination ip",
					candidate.record.Locator)
			}
			if _, hasPort := comparableOfSemantic(candidate.fields,
				core.SemanticKeyConnectionDestinationPort); !hasPort {
				t.Errorf("the record at %+v is indexed without a comparable destination port",
					candidate.record.Locator)
			}
		}
	}
}
