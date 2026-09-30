package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// processedTarget は処理を終えた後だけ答える操作の要求先である。
var processedTarget = withAllMatchConditions("/api/v0/assertions")

// newStagedFixture は run の fixture の directory を基準に、要求による読み込みを受け付ける段階と、
// その handler を返す。
func newStagedFixture(t *testing.T, requested bool) (*pipeline.Stages, http.Handler) {
	t.Helper()
	openUnder := func(name string) (io.ReadCloser, error) {
		return os.Open(filepath.Join(runFixtureDir, name)) // #nosec G304 -- test の fixture を開く。
	}
	config := pipeline.StagesConfig{
		Import: pipeline.Config{
			Open: openUnder, Parsers: testFormatRegistry(t), Minter: pipeline.DigestMinter{},
			Sanitize: output.Sanitize, Revision: "api-test",
		},
		Recorder: pipeline.NewMemoryRecorder(), Layers: pipeline.DefaultGraphLayers(),
		Clock: &testAssertionClock{},
	}
	if requested {
		config.Requested = &pipeline.RequestedFiles{
			Open: openUnder, FS: os.DirFS(runFixtureDir),
			Size: func(name string) (int64, error) {
				info, err := os.Stat(filepath.Join(runFixtureDir, name))
				if err != nil {
					return 0, err
				}
				return info.Size(), nil
			},
		}
	}
	stages, err := pipeline.NewStages(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stages.Wait)
	handler, err := api.NewStagedHandler(stages, nil, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	return stages, handler
}

// loadingRequestOfFixtures は run の fixture をすべて読む要求の本文を返す。
func loadingRequestOfFixtures(t *testing.T) string {
	t.Helper()
	type source struct {
		OriginPath string         `json:"originPath"`
		FormatKey  core.FormatKey `json:"formatKey"`
	}
	var body struct {
		Sources []source `json:"sources"`
	}
	for _, fixture := range runFixtures(t) {
		body.Sources = append(body.Sources, source{OriginPath: fixture.File, FormatKey: fixture.Format})
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// serveStaged は要求を 1 件送り、status を確かめて本文を返す。
func serveStaged(t *testing.T, handler http.Handler, method, target, body string, want int) []byte {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, reader))
	if recorder.Code != want {
		t.Fatalf("%s %s: status=%d want %d body=%s", method, target, recorder.Code, want, recorder.Body.String())
	}
	return recorder.Body.Bytes()
}

// requireStageErrorCode は失敗の応答が code を持つことを確かめる。
func requireStageErrorCode(t *testing.T, body []byte, want core.ApiErrorCode) core.ApiError {
	t.Helper()
	var apiError core.ApiError
	if err := json.Unmarshal(body, &apiError); err != nil {
		t.Fatalf("decoding the failure %s: %v", body, err)
	}
	if apiError.Code != want {
		t.Fatalf("code=%q want %q (message %q)", apiError.Code, want, apiError.Message)
	}
	return apiError
}

// decodeStages は段階の状態を読み、不変条件を確かめる。
func decodeStages(t *testing.T, body []byte) core.InvestigationStages {
	t.Helper()
	var stages core.InvestigationStages
	if err := json.Unmarshal(body, &stages); err != nil {
		t.Fatalf("decoding the stages %s: %v", body, err)
	}
	if err := stages.Validate(); err != nil {
		t.Fatalf("the stages do not validate: %v (%s)", err, body)
	}
	return stages
}

// 段階を始める前は、段階の状態だけを答え、調査の API を stage_not_ready で退ける。
func TestStagedHandlerRefusesTheInvestigationBeforeTheStages(t *testing.T) {
	_, handler := newStagedFixture(t, true)

	snapshot := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
	if snapshot.Loading.State != core.StageStateNotStarted || snapshot.Processing.State != core.StageStateNotStarted {
		t.Errorf("the stages are %q and %q, want both not started", snapshot.Loading.State, snapshot.Processing.State)
	}
	if !snapshot.AcceptsRequestedLoading {
		t.Error("the server with a base directory does not accept the requested loading")
	}
	for _, target := range []string{"/api/v0/sources", processedTarget, "/api/v0/unknown"} {
		body := serveStaged(t, handler, http.MethodGet, target, "", http.StatusConflict)
		requireStageErrorCode(t, body, core.ApiErrorCodeStageNotReady)
	}
	body := serveStaged(t, handler, http.MethodPost, "/api/v0/stages/processing", "", http.StatusConflict)
	requireStageErrorCode(t, body, core.ApiErrorCodeStageNotReady)
	serveStaged(t, handler, http.MethodGet, "/index.html", "", http.StatusNotFound)
	body = serveStaged(t, handler, http.MethodGet, "/api/v0/stages?verbose=1", "", http.StatusBadRequest)
	requireStageErrorCode(t, body, core.ApiErrorCodeInvalidRequest)
}

// 読み込みを終えると収集元の一覧を答え、処理を終えるまでグラフを stage_not_ready で退ける。
// 処理を終えるとグラフを答える。終えた段階を始め直す要求は stage_already_started で退ける。
func TestStagedHandlerAnswersEachStageInTurn(t *testing.T) {
	stages, handler := newStagedFixture(t, true)
	request := loadingRequestOfFixtures(t)

	started := decodeStages(t, serveStaged(t, handler, http.MethodPost, "/api/v0/stages/loading", request, http.StatusAccepted))
	if started.Loading.State != core.StageStateRunning && started.Loading.State != core.StageStateCompleted {
		t.Errorf("the accepted loading is %q, want running or completed", started.Loading.State)
	}
	for _, source := range started.Loading.Sources {
		if source.SizeBytes == nil {
			t.Errorf("the requested source %q carries no size", source.OriginPath)
		}
	}
	stages.Wait()

	loaded := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
	if loaded.Loading.State != core.StageStateCompleted {
		t.Fatalf("the loading is %q (failure %+v), want completed", loaded.Loading.State, loaded.Loading.Failure)
	}
	for _, source := range loaded.Loading.Sources {
		if source.Status == nil || source.SizeBytes == nil || source.ReadBytes != *source.SizeBytes {
			t.Errorf("the loaded source %+v does not carry its status and every byte read", source)
		}
	}
	page := decodeSources(t, requestSources(t, handler, "/api/v0/sources"), http.StatusOK)
	if len(page.Sources) != len(loaded.Loading.Sources) {
		t.Errorf("the sources list %d sources, the loading read %d", len(page.Sources), len(loaded.Loading.Sources))
	}
	body := serveStaged(t, handler, http.MethodGet, processedTarget, "", http.StatusConflict)
	requireStageErrorCode(t, body, core.ApiErrorCodeStageNotReady)
	body = serveStaged(t, handler, http.MethodPost, "/api/v0/stages/loading", request, http.StatusConflict)
	requireStageErrorCode(t, body, core.ApiErrorCodeStageAlreadyStarted)
	body = serveStaged(t, handler, http.MethodPost, "/api/v0/stages/processing", `{"steps":[]}`, http.StatusBadRequest)
	requireStageErrorCode(t, body, core.ApiErrorCodeInvalidRequest)

	serveStaged(t, handler, http.MethodPost, "/api/v0/stages/processing", "", http.StatusAccepted)
	stages.Wait()
	processed := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
	if processed.Processing.State != core.StageStateCompleted {
		t.Fatalf("the processing is %q (failure %+v), want completed", processed.Processing.State, processed.Processing.Failure)
	}
	serveStaged(t, handler, http.MethodGet, processedTarget, "", http.StatusOK)
	body = serveStaged(t, handler, http.MethodPost, "/api/v0/stages/processing", "", http.StatusConflict)
	requireStageErrorCode(t, body, core.ApiErrorCodeStageAlreadyStarted)
	serveStaged(t, handler, http.MethodGet, "/index.html", "", http.StatusNotFound)
}

// 読み込めない要求は 400 で退け、段階の状態を変えない。
func TestStagedHandlerRefusesAnInvalidLoadingRequest(t *testing.T) {
	fixture := runFixtures(t)[0]
	source := func(originPath string, formatKey core.FormatKey) map[string]any {
		return map[string]any{"originPath": originPath, "formatKey": formatKey}
	}
	withTerminal := func(terminal map[string]any) map[string]any {
		item := source(fixture.File, fixture.Format)
		item["terminal"] = terminal
		return item
	}
	bodyOf := func(sources ...map[string]any) string {
		data, err := json.Marshal(map[string]any{"sources": sources})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	readable := source(fixture.File, fixture.Format)
	parentPath := "../" + filepath.Base(runFixtureDir) + "/" + fixture.File
	absolutePath := "/" + fixture.File
	// rejection と originPath は、失敗の応答が持つ退けた理由と退けた収集元である。要求の本文を
	// 読めない失敗と要求全体の失敗は、それぞれ空文字列である。
	for name, request := range map[string]struct {
		body       string
		rejection  core.LoadingRejection
		originPath string
	}{
		"not JSON":         {body: `sources`},
		"an unknown field": {body: `{"sources":[],"root":"/"}`},
		"a trailing value": {body: bodyOf(readable) + `{}`},
		"no source":        {body: bodyOf(), rejection: core.LoadingRejectionNoSource},
		"a parent path": {body: bodyOf(source(parentPath, fixture.Format)),
			rejection: core.LoadingRejectionPathOutsideBase, originPath: parentPath},
		"an absolute path": {body: bodyOf(source(absolutePath, fixture.Format)),
			rejection: core.LoadingRejectionPathOutsideBase, originPath: absolutePath},
		"an absent file": {body: bodyOf(source("./absent.log", fixture.Format)),
			rejection: core.LoadingRejectionFileAbsent, originPath: "./absent.log"},
		"an unknown format": {body: bodyOf(source(fixture.File, "no_such_format")),
			rejection: core.LoadingRejectionFormatUnknown, originPath: fixture.File},
		"the same source": {body: bodyOf(readable, readable),
			rejection: core.LoadingRejectionSourceRepeated, originPath: fixture.File},
		"an empty terminal": {body: bodyOf(withTerminal(map[string]any{})),
			rejection: core.LoadingRejectionTerminalInvalid, originPath: fixture.File},
		"an unknown terminal": {body: bodyOf(withTerminal(map[string]any{"mac": "x"}))},
	} {
		t.Run(name, func(t *testing.T) {
			_, handler := newStagedFixture(t, true)
			response := serveStaged(t, handler, http.MethodPost, "/api/v0/stages/loading", request.body, http.StatusBadRequest)
			apiError := requireStageErrorCode(t, response, core.ApiErrorCodeInvalidRequest)
			if apiError.LoadingRejection != request.rejection || apiError.OriginPath != request.originPath {
				t.Errorf("the failure is %q for %q, want %q for %q",
					apiError.LoadingRejection, apiError.OriginPath, request.rejection, request.originPath)
			}
			if err := apiError.Validate(); err != nil {
				t.Errorf("the failure does not validate: %v", err)
			}
			snapshot := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
			if snapshot.Loading.State != core.StageStateNotStarted {
				t.Errorf("the refused request left the loading %q", snapshot.Loading.State)
			}
		})
	}
}

// fixedStages は、読み込みを終えた保存先と、処理を終えた Session を test が決める段階である。
// session が nil の間は処理を終えていない。
type fixedStages struct {
	store   pipeline.InvestigationStore
	session *pipeline.Session
}

func (s *fixedStages) Snapshot() core.InvestigationStages { return core.InvestigationStages{} }

func (s *fixedStages) StartRequestedLoading([]pipeline.SourcePlan) error {
	return pipeline.ErrStageAlreadyStarted
}

func (s *fixedStages) StartRequestedLoadingThenProcessing([]pipeline.SourcePlan) error {
	return pipeline.ErrStageAlreadyStarted
}

func (s *fixedStages) StartProcessing() error { return pipeline.ErrStageAlreadyStarted }

func (s *fixedStages) ListRequestedFiles(context.Context, string, bool) (core.SourceFileListing, error) {
	return core.SourceFileListing{}, pipeline.ErrLoadingRequestInvalid
}

func (s *fixedStages) Loaded() (pipeline.InvestigationStore, bool) { return s.store, true }

func (s *fixedStages) Processed() (pipeline.Session, bool) {
	if s.session == nil {
		return pipeline.Session{}, false
	}
	return *s.session, true
}

// 時刻の解釈を記録した調査の収集元の一覧は、処理を終える前も、処理を終えた後と同じ項目を答える。
func TestStagedHandlerListsTheInterpretedRangeBeforeTheProcessing(t *testing.T) {
	store, localContent := timeInterpretationStore(t)
	requestJSON(t, handlerOf(store), http.MethodPost, withAllMatchConditions(assertionsPath),
		encodeBody(t, sourceInterpretationBody(localContent, "+09:00")), http.StatusCreated)
	stages := &fixedStages{store: store}
	handler, err := api.NewStagedHandler(stages, nil, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}

	loaded := requestJSON(t, handler, http.MethodGet, "/api/v0/sources", nil, http.StatusOK)
	var page sourcesResponse
	decodeInto(t, loaded, &page)
	index := slices.IndexFunc(page.Sources, func(item sourceResponseItem) bool {
		return item.Source.FileName == "local.log"
	})
	if index < 0 || page.Sources[index].InterpretedObservedRange == nil {
		t.Fatalf("the sources before the processing carry no interpreted range of local.log (index %d)", index)
	}

	stages.session = &pipeline.Session{
		Store: store, Catalog: pipeline.NewGraphCatalog(store, pipeline.DefaultGraphLayers()),
	}
	processed := requestJSON(t, handler, http.MethodGet, "/api/v0/sources", nil, http.StatusOK)
	if !bytes.Equal(loaded, processed) {
		t.Errorf("the sources differ across the processing:\nloaded    %s\nprocessed %s", loaded, processed)
	}
}

// 基準の directory を持たない server は、要求による読み込みを 400 で退ける。
func TestStagedHandlerWithoutABaseDirectoryRefusesTheRequestedLoading(t *testing.T) {
	_, handler := newStagedFixture(t, false)
	snapshot := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
	if snapshot.AcceptsRequestedLoading {
		t.Error("the server without a base directory accepts the requested loading")
	}
	body := serveStaged(t, handler, http.MethodPost, "/api/v0/stages/loading", loadingRequestOfFixtures(t), http.StatusBadRequest)
	apiError := requireStageErrorCode(t, body, core.ApiErrorCodeInvalidRequest)
	if apiError.LoadingRejection != core.LoadingRejectionNoBaseDirectory || apiError.OriginPath != "" {
		t.Errorf("the failure is %q for %q, want %q for no source",
			apiError.LoadingRejection, apiError.OriginPath, core.LoadingRejectionNoBaseDirectory)
	}
}
