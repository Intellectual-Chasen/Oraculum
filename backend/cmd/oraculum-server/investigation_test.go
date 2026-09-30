// in-package test: 非公開の openStages で調査を作り、開き直した後の API の応答を比べる。
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// investigationSources は調査に取り込む fixture の起動引数である。
func investigationSources() []string {
	return []string{
		"infotrace_mark_ii:" + filepath.Join(runFixtureDir, "markii.log"),
		"squid_combined:" + filepath.Join(runFixtureDir, "squid.log"),
	}
}

// openedInvestigation は openStages で開いた調査と、その API である。
type openedInvestigation struct {
	investigation *pipeline.Investigation
	// root は要求による読み込みの基準である。基準を持たない起動では nil である。
	root    io.Closer
	stages  *pipeline.Stages
	handler http.Handler
}

func openInvestigation(t *testing.T, args ...string) openedInvestigation {
	t.Helper()
	opened, err := tryOpenInvestigation(pipeline.DefaultGraphLayers(), args...)
	if err != nil {
		t.Fatal(err)
	}
	return opened
}

// tryOpenInvestigation は調査を開き、layers で組むグラフの API を返す。layers を差し替えると、
// 取り込みの導出を変えた build で調査を開き直す状態を作れる。
func tryOpenInvestigation(layers pipeline.GraphLayers, args ...string) (openedInvestigation, error) {
	opts, err := parseArgs(args)
	if err != nil {
		return openedInvestigation{}, err
	}
	stages, investigation, root, err := openStages(context.Background(), opts, layers, nil, io.Discard)
	if err != nil {
		return openedInvestigation{}, err
	}
	// 起動の時点で読み込んだ調査は、待ち受けの後に背景で処理する。test は同期で処理を終える。
	if _, loaded := stages.Loaded(); loaded {
		if err := stages.Process(); err != nil {
			return openedInvestigation{}, err
		}
	}
	handler, err := api.NewStagedHandler(stages, nil, pipeline.AttackRuleSet{})
	if err != nil {
		return openedInvestigation{}, err
	}
	return openedInvestigation{
		investigation: investigation, root: root, stages: stages, handler: handler,
	}, nil
}

func (o openedInvestigation) close(t *testing.T) {
	t.Helper()
	o.stages.Wait()
	if o.root != nil {
		if err := o.root.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.investigation.Close(); err != nil {
		t.Fatal(err)
	}
}

func allConditionsQuery() string {
	values := url.Values{}
	for _, key := range core.KnownConditionKeys() {
		values.Add("matchCondition", string(key))
	}
	return values.Encode()
}

func (o openedInvestigation) request(t *testing.T, method, path string, body any, want int) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, reader)
	recorder := httptest.NewRecorder()
	o.handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s answered %d, want %d: %s", method, path, recorder.Code, want, recorder.Body.String())
	}
	return recorder.Body.Bytes()
}

// snapshot は、開き直した後に比べる応答を集める。
func (o openedInvestigation) snapshot(t *testing.T) map[string][]byte {
	t.Helper()
	conditions := allConditionsQuery()
	return map[string][]byte{
		"sources":     o.request(t, http.MethodGet, "/api/v0/sources", nil, http.StatusOK),
		"assertions":  o.request(t, http.MethodGet, "/api/v0/assertions?"+conditions, nil, http.StatusOK),
		"assignments": o.request(t, http.MethodGet, "/api/v0/terminal-assignments", nil, http.StatusOK),
		"graph": o.request(t, http.MethodGet, "/api/v0/graph?"+conditions+"&depth=1", nil,
			http.StatusOK),
	}
}

// recordAssertion は、グラフの最初のノードへ所見を記録し、別の分析者の改訂で取り下げる。
func (o openedInvestigation) recordAssertion(t *testing.T) {
	t.Helper()
	conditions := allConditionsQuery()
	var graph struct {
		Nodes []struct {
			Id string `json:"id"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(o.request(t, http.MethodGet, "/api/v0/graph?"+conditions+"&depth=1",
		nil, http.StatusOK), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) == 0 {
		t.Fatal("the fixture graph carries no node")
	}
	target := map[string]any{"kind": "node", "nodeId": graph.Nodes[0].Id}
	var created struct {
		Assertion core.Assertion `json:"assertion"`
	}
	if err := json.Unmarshal(o.request(t, http.MethodPost, "/api/v0/assertions?"+conditions, map[string]any{
		"target": target, "author": "analyst-a", "basis": map[string]any{"note": "最初の所見"},
	}, http.StatusCreated), &created); err != nil {
		t.Fatal(err)
	}
	o.request(t, http.MethodPut, "/api/v0/assertions/"+url.PathEscape(created.Assertion.Id)+"?"+conditions,
		map[string]any{"target": target, "author": "analyst-b", "state": "withdrawn",
			"basis": map[string]any{"note": "取り下げた"}, "baseRevision": created.Assertion.RevisionNumber},
		http.StatusOK)
}

// listedSource は /api/v0/sources の収集元 1 件のうち、test が読む項目である。観測範囲は
// JSON の文字列のまま持つ。
type listedSource struct {
	SourceId, OriginPath, ContentSha256   string
	ObservedRangeFirst, ObservedRangeLast string
}

func (o openedInvestigation) sources(t *testing.T) []listedSource {
	t.Helper()
	var response struct {
		Sources []struct {
			Source struct {
				SourceId           string          `json:"sourceId"`
				OriginPath         string          `json:"originPath"`
				ContentSha256      string          `json:"contentSha256"`
				ObservedRangeFirst json.RawMessage `json:"observedRangeFirst"`
				ObservedRangeLast  json.RawMessage `json:"observedRangeLast"`
			} `json:"source"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(o.request(t, http.MethodGet, "/api/v0/sources", nil, http.StatusOK), &response); err != nil {
		t.Fatal(err)
	}
	listed := make([]listedSource, len(response.Sources))
	for index, entry := range response.Sources {
		source := entry.Source
		listed[index] = listedSource{SourceId: source.SourceId, OriginPath: source.OriginPath,
			ContentSha256: source.ContentSha256, ObservedRangeFirst: string(source.ObservedRangeFirst),
			ObservedRangeLast: string(source.ObservedRangeLast)}
	}
	return listed
}

// recordAssignment は、最初の収集元の観測範囲に分析者の割当を 1 件記録する。
func (o openedInvestigation) recordAssignment(t *testing.T) {
	t.Helper()
	sources := o.sources(t)
	if len(sources) == 0 {
		t.Fatal("the investigation holds no source")
	}
	source := sources[0]
	o.request(t, http.MethodPost, "/api/v0/terminal-assignments", map[string]any{
		"clientIp": "198.51.100.77", "terminalId": "terminal-supplied-by-the-analyst",
		"terminalHostname": "linux-host.example.test", "sourceId": source.SourceId,
		"sourceContentSha256": source.ContentSha256,
		"assignmentValidRange": map[string]any{"from": json.RawMessage(source.ObservedRangeFirst),
			"to": json.RawMessage(source.ObservedRangeLast)},
		"derivation": "別の端末の ssh の接続先から導いた",
		"basisRecordRefs": []map[string]any{{"sourceContentSha256": source.ContentSha256,
			"positionKind": string(core.PositionKindLineNumber), "lineNumber": 1}},
		"author": "analyst-a",
	}, http.StatusCreated)
}

// 作成した調査へ記録した所見と割当は、閉じて開き直した後も同じ応答で返る。起動引数の収集元を
// 渡さずに開ける。
func TestReopenedInvestigationAnswersTheSame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	created := openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...)
	created.recordAssertion(t)
	created.recordAssignment(t)
	before := created.snapshot(t)
	created.close(t)

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	after := reopened.snapshot(t)
	for name, body := range before {
		if !bytes.Equal(body, after[name]) {
			t.Errorf("the %s response differs after reopening:\nbefore %s\nafter  %s", name, body, after[name])
		}
	}
	var counts struct {
		AssertionCount  int64 `json:"assertionCount"`
		AssignmentCount int64 `json:"assignmentCount"`
	}
	for _, name := range []string{"assertions", "assignments"} {
		if err := json.Unmarshal(after[name], &counts); err != nil {
			t.Fatal(err)
		}
	}
	if counts.AssertionCount == 0 || counts.AssignmentCount == 0 {
		t.Errorf("the reopened investigation holds %d assertions and %d assignments, want the recorded ones",
			counts.AssertionCount, counts.AssignmentCount)
	}
}

// recordEdgeAssertion は関係 edge へ所見を記録し、所見の識別子と記録時の対象の出所を返す。
func (o openedInvestigation) recordEdgeAssertion(
	t *testing.T, edge core.AssertionEdgeRef,
) (string, core.AssertionTargetOrigin) {
	t.Helper()
	var created struct {
		Assertion    core.Assertion             `json:"assertion"`
		TargetOrigin core.AssertionTargetOrigin `json:"targetOrigin"`
	}
	if err := json.Unmarshal(o.request(t, http.MethodPost, "/api/v0/assertions?"+allConditionsQuery(),
		map[string]any{
			"target": map[string]any{"kind": "edge", "edge": edge}, "author": "analyst-a",
			"basis": map[string]any{"note": "関係への所見"},
		}, http.StatusCreated), &created); err != nil {
		t.Fatal(err)
	}
	return created.Assertion.Id, created.TargetOrigin
}

// assertionOrigins は所見の一覧を、所見の識別子から対象の出所への対応にする。
func (o openedInvestigation) assertionOrigins(t *testing.T) map[string]core.AssertionTargetOrigin {
	t.Helper()
	var listed struct {
		Assertions []struct {
			Assertion    core.Assertion             `json:"assertion"`
			TargetOrigin core.AssertionTargetOrigin `json:"targetOrigin"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal(o.request(t, http.MethodGet, "/api/v0/assertions?"+allConditionsQuery(), nil,
		http.StatusOK), &listed); err != nil {
		t.Fatal(err)
	}
	origins := make(map[string]core.AssertionTargetOrigin, len(listed.Assertions))
	for _, item := range listed.Assertions {
		origins[item.Assertion.Id] = item.TargetOrigin
	}
	return origins
}

// 取り込みの導出が変わって観測の関係が消えた調査を開き直すと、その関係への所見は absent になり、
// 分析者が足した関係への所見は analyst_assertion のまま残る。
func TestReopenedInvestigationSeparatesAVanishedRelationFromAnAddedOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	created := openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...)
	type listedEdge struct {
		core.AssertionEdgeRef
		State core.RelationState `json:"state"`
	}
	var graph struct {
		Edges []listedEdge `json:"edges"`
	}
	if err := json.Unmarshal(created.request(t, http.MethodGet,
		"/api/v0/graph?"+allConditionsQuery()+"&depth=1", nil, http.StatusOK), &graph); err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(graph.Edges, func(edge listedEdge) bool {
		return edge.State == core.RelationStateObserved
	})
	if index < 0 {
		t.Fatal("the fixture graph carries no observed edge")
	}
	observed := graph.Edges[index].AssertionEdgeRef
	// 分析者が足す関係は、観測の関係と同じ両端を逆向きに結ぶ、グラフに無い関係である。
	added := core.AssertionEdgeRef{Kind: core.EdgeKindFileCopy,
		SourceNodeId: observed.TargetNodeId, TargetNodeId: observed.SourceNodeId}
	onObserved, origin := created.recordEdgeAssertion(t, observed)
	if origin != core.AssertionTargetOriginObservation {
		t.Fatalf("the assertion on the observed edge answered %q, want %q",
			origin, core.AssertionTargetOriginObservation)
	}
	onAdded, origin := created.recordEdgeAssertion(t, added)
	if origin != core.AssertionTargetOriginAnalystAssertion {
		t.Fatalf("the assertion on the added edge answered %q, want %q",
			origin, core.AssertionTargetOriginAnalystAssertion)
	}
	created.close(t)

	// 観測の関係を 1 本も組まない導出で開き直す。
	withoutObservations := pipeline.GraphLayers{
		Observed: func(pipeline.ImportResult) pipeline.Graph {
			return pipeline.NewObservedGraph(pipeline.ImportResult{})
		},
		WithCandidates: func(observed pipeline.Graph, _ pipeline.ImportResult,
			_ pipeline.MatchConditionSelection) pipeline.Graph {
			return observed
		},
	}
	reopened, err := tryOpenInvestigation(withoutObservations, investigationFlag, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close(t)
	origins := reopened.assertionOrigins(t)
	want := map[string]core.AssertionTargetOrigin{
		onObserved: core.AssertionTargetOriginAbsent,
		onAdded:    core.AssertionTargetOriginAnalystAssertion,
	}
	for id, wanted := range want {
		if origins[id] != wanted {
			t.Errorf("the assertion %q answered %q after reopening, want %q", id, origins[id], wanted)
		}
	}
}

// 取り込みの起動で収集元に指定した端末は、調査に残り、開き直した後も同じ割当として一覧に出る。
func TestReopenedInvestigationKeepsTheSpecifiedTerminal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	sources := investigationSources()
	created := openInvestigation(t, investigationFlag, dir,
		sources[0], "--terminal-name", "proxy.example.test", "--terminal-ip", "192.0.2.8", sources[1])
	before := created.request(t, http.MethodGet, "/api/v0/terminal-assignments", nil, http.StatusOK)
	created.close(t)

	var listed struct {
		Assignments []core.TerminalAssignment `json:"assignments"`
	}
	if err := json.Unmarshal(before, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Assignments) != 1 {
		t.Fatalf("the investigation lists %+v, want the one specified terminal", listed.Assignments)
	}
	specified := listed.Assignments[0]
	if specified.Origin != core.TerminalAssignmentOriginImportSpecified ||
		specified.TerminalHostname != "proxy.example.test" || specified.ClientIp != "192.0.2.8" ||
		specified.TerminalId != "" || specified.AppliesToSourceId != specified.SourceId {
		t.Errorf("the specified terminal is %+v, want the name and the IP applied to its source", specified)
	}

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	after := reopened.request(t, http.MethodGet, "/api/v0/terminal-assignments", nil, http.StatusOK)
	if !bytes.Equal(before, after) {
		t.Errorf("the assignments differ after reopening:\nbefore %s\nafter  %s", before, after)
	}
}

// 開き直した調査へ足した収集元を記録し、記録済みの収集元の sourceId を変えない。
func TestSourcesAddedToAReopenedInvestigationAreRecorded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	first, added := investigationSources()[0], investigationSources()[1]
	created := openInvestigation(t, investigationFlag, dir, first)
	initial := created.sources(t)
	created.close(t)

	extended := openInvestigation(t, investigationFlag, dir, added)
	withAdded := extended.sources(t)
	extended.close(t)
	if len(withAdded) != len(initial)+1 || withAdded[0] != initial[0] {
		t.Fatalf("the extended investigation lists %+v, want %+v and the added source", withAdded, initial)
	}
	if _, addedPath, _ := strings.Cut(added, ":"); withAdded[len(withAdded)-1].OriginPath != addedPath {
		t.Errorf("the added source is %q, want %q", withAdded[len(withAdded)-1].OriginPath, addedPath)
	}

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	if got := reopened.sources(t); !slices.Equal(got, withAdded) {
		t.Errorf("the reopened investigation lists %+v, want %+v", got, withAdded)
	}
}

// 記録済みの収集元をもう一度渡した起動を、path の書き方が違っても退ける。
func TestReopeningWithARecordedSourceIsRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...).close(t)
	for _, origin := range []string{
		filepath.Join(runFixtureDir, "markii.log"),
		"./" + filepath.Join(runFixtureDir, "markii.log"),
		runFixtureDir + "//markii.log",
		runFixtureDir + "/../run/markii.log",
	} {
		_, err := tryOpenInvestigation(pipeline.DefaultGraphLayers(), investigationFlag, dir, "infotrace_mark_ii:"+origin)
		if err == nil || !strings.Contains(err.Error(), "already records") {
			t.Errorf("reopening with the recorded source written as %q returned %v, want a refusal", origin, err)
		}
	}
}

// --source-root を付けて作った調査は、取り込みに使った基準を記録し、付けずに開き直せる。
func TestInvestigationCreatedUnderASourceRootRecordsIt(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join(runFixtureDir, "markii.log"))
	if err != nil {
		t.Fatal(err)
	}
	// 起動した directory に無い path に置き、記録した基準からだけ読めるようにする。
	if err := os.MkdirAll(filepath.Join(root, "collected"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "collected", "markii.log"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "case")
	created := openInvestigation(t, investigationFlag, dir, sourceRootFlag, root,
		"infotrace_mark_ii:collected/markii.log")
	before := created.sources(t)
	created.close(t)

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	if got := reopened.sources(t); !slices.Equal(got, before) {
		t.Errorf("the reopened investigation lists %+v, want %+v", got, before)
	}
}

// 基準の directory だけを渡して作る調査は、要求で読み込んだ収集元と端末の指定を記録し、
// 開き直すと同じ一覧を答える。処理を終えるまで、グラフを読む要求を stage_not_ready で退ける。
func TestInvestigationRecordsTheRequestedLoading(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"markii.log", "squid.log"} {
		data, err := os.ReadFile(filepath.Join(runFixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "collected"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "collected", name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "case")
	created := openInvestigation(t, investigationFlag, dir, sourceRootFlag, root)
	conditions := allConditionsQuery()
	created.request(t, http.MethodGet, "/api/v0/assertions?"+conditions, nil, http.StatusConflict)
	created.request(t, http.MethodPost, "/api/v0/stages/loading", map[string]any{"sources": []map[string]any{
		{"originPath": "collected/markii.log", "formatKey": "infotrace_mark_ii"},
		{"originPath": "collected/squid.log", "formatKey": "squid_combined",
			"terminal": map[string]any{"id": "proxy-01", "hostname": "proxy.example.test"}},
	}}, http.StatusAccepted)
	created.stages.Wait()
	before := created.sources(t)
	created.request(t, http.MethodGet, "/api/v0/assertions?"+conditions, nil, http.StatusConflict)
	created.request(t, http.MethodPost, "/api/v0/stages/processing", nil, http.StatusAccepted)
	created.stages.Wait()
	created.request(t, http.MethodGet, "/api/v0/assertions?"+conditions, nil, http.StatusOK)
	created.close(t)

	wantPaths := []string{"collected/markii.log", "collected/squid.log"}
	var gotPaths []string
	for _, source := range before {
		gotPaths = append(gotPaths, source.OriginPath)
	}
	slices.Sort(gotPaths)
	if !slices.Equal(gotPaths, wantPaths) {
		t.Errorf("the loading listed %v, want %v", gotPaths, wantPaths)
	}
	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	if got := reopened.sources(t); !slices.Equal(got, before) {
		t.Errorf("the reopened investigation lists %+v, want %+v", got, before)
	}
	recorded := reopened.investigation.RecordedPlans()
	index := slices.IndexFunc(recorded, func(plan pipeline.SourcePlan) bool {
		return plan.OriginPath == "collected/squid.log"
	})
	if index < 0 || recorded[index].Terminal == nil || recorded[index].Terminal.TerminalId != "proxy-01" ||
		recorded[index].Terminal.TerminalHostname != "proxy.example.test" {
		t.Errorf("the investigation recorded %+v, want the squid source with its terminal", recorded)
	}
}

// 調査へ記録する読み込みは、失敗した後にやり直せる。やり直して作った調査は、失敗を挟まずに
// 作った調査と同じ応答を返す。
func TestInvestigationLoadingRetriesAfterAFailure(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"markii.log", "squid.log"} {
		data, err := os.ReadFile(filepath.Join(runFixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plans := []pipeline.SourcePlan{
		{OriginPath: "markii.log", FileName: "markii.log", FormatKey: "infotrace_mark_ii"},
		{OriginPath: "squid.log", FileName: "squid.log", FormatKey: "squid_combined"},
	}
	loadInto := func(dir string, failFirst bool) map[string][]byte {
		t.Helper()
		opened := openInvestigation(t, investigationFlag, dir, sourceRootFlag, root)
		if failFirst {
			broken := append(slices.Clone(plans[:1]),
				pipeline.SourcePlan{OriginPath: "missing.log", FileName: "missing.log", FormatKey: "squid_combined"})
			if err := opened.stages.Load(broken, nil); err == nil {
				t.Fatal("loading a missing source succeeded")
			}
		}
		if err := opened.stages.Load(plans, nil); err != nil {
			t.Fatalf("loading the sources after the failure: %v", err)
		}
		opened.close(t)
		reopened := openInvestigation(t, investigationFlag, dir)
		defer reopened.close(t)
		return reopened.snapshot(t)
	}
	retried := loadInto(filepath.Join(t.TempDir(), "retried"), true)
	direct := loadInto(filepath.Join(t.TempDir(), "direct"), false)
	for name, body := range direct {
		if !bytes.Equal(retried[name], body) {
			t.Errorf("the retried investigation answers %s with %s, want %s", name, retried[name], body)
		}
	}
}

// 調査へ記録できない読み込みは、保存 path を載せない理由で失敗し、障害を取り除くとやり直せる。
func TestRequestedLoadingThatCannotRecordFailsWithoutAPath(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join(runFixtureDir, "squid.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "squid.log"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "case")
	opened := openInvestigation(t, investigationFlag, dir, sourceRootFlag, root)
	// 起動の後に調査の directory の位置へ file を置き、読み込みの後に調査を作れなくする。
	if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"sources": []map[string]any{{"originPath": "squid.log", "formatKey": "squid_combined"}}}
	opened.request(t, http.MethodPost, "/api/v0/stages/loading", request, http.StatusAccepted)
	opened.stages.Wait()

	body := opened.request(t, http.MethodGet, "/api/v0/stages", nil, http.StatusOK)
	var snapshot core.InvestigationStages
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	want := core.StageFailure{Reason: core.StageFailureReasonRecordingFailed}
	if snapshot.Loading.Failure == nil || *snapshot.Loading.Failure != want {
		t.Fatalf("the loading failed with %+v, want %+v", snapshot.Loading.Failure, want)
	}
	for _, path := range []string{root, dir} {
		if bytes.Contains(body, []byte(path)) {
			t.Errorf("the stages %s name the server path %s", body, path)
		}
	}

	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	opened.request(t, http.MethodPost, "/api/v0/stages/loading", request, http.StatusAccepted)
	opened.stages.Wait()
	if _, loaded := opened.stages.Loaded(); !loaded {
		t.Error("the loading retried after removing the obstacle did not complete")
	}
	opened.close(t)
}

// 調査の無い directory は、収集元も基準の directory も渡さない起動で作らない。基準の directory を
// 渡した起動は、要求による読み込みを待ち、読み込むまで調査を作らない。
func TestNewInvestigationRequiresASourceOrASourceRoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	if _, err := tryOpenInvestigation(pipeline.DefaultGraphLayers(), investigationFlag, dir); err == nil {
		t.Fatal("opening an absent investigation without sources succeeded")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the refused start left %s (stat error %v)", dir, err)
	}

	waiting := openInvestigation(t, investigationFlag, dir, sourceRootFlag, t.TempDir())
	defer waiting.close(t)
	if _, loaded := waiting.stages.Loaded(); loaded {
		t.Error("the start without sources reported the loading as completed")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the start waiting for the loading left %s (stat error %v)", dir, err)
	}
}

// 原資料が記録と異なれば開かず、調査の file を変えない。基準を置き換えた起動も同じ検査を通る。
func TestInvestigationWithChangedSourceDoesNotOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...).close(t)
	file := filepath.Join(dir, "investigation.sqlite")
	before := fileDigest(t, file)

	moved := t.TempDir()
	for _, name := range []string{"markii.log", "squid.log"} {
		data, err := os.ReadFile(filepath.Join(runFixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "squid.log" {
			data = append(data, '\n')
		}
		target := filepath.Join(moved, runFixtureDir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := tryOpenInvestigation(pipeline.DefaultGraphLayers(), investigationFlag, dir, sourceRootFlag, moved)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("opening with a changed source returned %v, want a sha256 mismatch", err)
	}
	if after := fileDigest(t, file); after != before {
		t.Error("the refused open changed the investigation file")
	}

	// 同じ byte 列を別の場所へ移した調査は、基準を置き換えて開ける。
	original, err := os.ReadFile(filepath.Join(runFixtureDir, "squid.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moved, runFixtureDir, "squid.log"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	openInvestigation(t, investigationFlag, dir, sourceRootFlag, moved).close(t)
}

// 収集元も基準の directory も調査も渡さない起動は、読み込む手段が無いので退ける。基準の directory
// だけを渡した起動は、要求による読み込みを待つ。
func TestStartRequiresASourceASourceRootOrAnInvestigation(t *testing.T) {
	if _, err := parseArgs(nil); err == nil {
		t.Error("the start without a source, a source root or an investigation was accepted")
	}
	opts, err := parseArgs([]string{sourceRootFlag, "/cases"})
	if err != nil {
		t.Fatalf("the start with only a source root was refused: %v", err)
	}
	if opts.sourceRoot != "/cases" || len(opts.plans) != 0 || opts.investigation != "" {
		t.Errorf("the start with only a source root parsed as %+v", opts)
	}
}

// writerEnv は、子 process として所見を書き続ける役を指す環境変数である。
const writerEnv = "ORACULUM_TEST_INVESTIGATION_WRITER"

// 書き込みの途中で process を強制終了しても、調査は開け、書き終えたと報告した所見を失わない。
func TestKilledWriterLeavesAnOpenableInvestigation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...).close(t)

	cmd := exec.Command(os.Args[0], "-test.run=^TestInvestigationWriterProcess$") // #nosec G204 -- 自身の test binary を起動する。
	cmd.Env = append(os.Environ(), writerEnv+"="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	const acknowledgedBeforeKill = 20
	acknowledged := 0
	scanner := bufio.NewScanner(stdout)
	deadline := time.After(time.Minute)
	lines := make(chan string)
	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	for acknowledged < acknowledgedBeforeKill {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the writer stopped after %d assertions", acknowledged)
			}
			if count, found := strings.CutPrefix(line, "wrote "); found {
				if acknowledged, err = strconv.Atoi(count); err != nil {
					t.Fatal(err)
				}
			}
		case <-deadline:
			t.Fatal("the writer did not acknowledge the assertions in time")
		}
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait() // 強制終了した子 process の終了状態は失敗であり、検査しない。
	for range lines {
		// 子 process が終了の前に書いた行を読み捨てる。
	}

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	var assertions struct {
		AssertionCount int64 `json:"assertionCount"`
	}
	body := reopened.request(t, http.MethodGet, "/api/v0/assertions?"+allConditionsQuery(), nil, http.StatusOK)
	if err := json.Unmarshal(body, &assertions); err != nil {
		t.Fatal(err)
	}
	if assertions.AssertionCount < int64(acknowledged) {
		t.Errorf("the reopened investigation holds %d assertions, the writer acknowledged %d",
			assertions.AssertionCount, acknowledged)
	}
}

// TestInvestigationWriterProcess は、TestKilledWriterLeavesAnOpenableInvestigation が子 process として
// 起動したときだけ、所見を書き続け、書き終えるたびに件数を出す。
func TestInvestigationWriterProcess(t *testing.T) {
	dir := os.Getenv(writerEnv)
	if dir == "" {
		t.Skip("TestKilledWriterLeavesAnOpenableInvestigation が子 process として起動したときだけ走る。この skip は恒久である")
	}
	opts, err := parseArgs([]string{investigationFlag, dir})
	if err != nil {
		t.Fatal(err)
	}
	stages, investigation, root, err := openStages(context.Background(), opts, pipeline.DefaultGraphLayers(), nil,
		io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer investigation.Close()
	defer root.Close()
	store, loaded := stages.Loaded()
	if !loaded {
		t.Fatal("the reopened investigation did not load its recorded sources")
	}
	for written := 1; ; written++ {
		if _, err := store.Assertions().Create(pipeline.AssertionDraft{
			Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:terminal:" + strconv.Itoa(written)},
			Author: "analyst-a", Basis: core.AssertionBasis{Note: "書き続ける所見"},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stdout.WriteString("wrote " + strconv.Itoa(written) + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
