package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// buildingWait は、別の goroutine の進行を待つ上限である。上限に達したら test を失敗させ、
// 永久に止まらないようにする。
const buildingWait = 5 * time.Second

// gatedLayers は、gate が閉じるまで候補のエッジを足す組み立てを止め、閉じた後は本番と同じ
// 関数で組む。組み立てに入ったことを entered へ知らせる。
func gatedLayers(gate <-chan struct{}, entered chan<- struct{}) pipeline.GraphLayers {
	layers := pipeline.DefaultGraphLayers()
	withCandidates := layers.WithCandidates
	layers.WithCandidates = func(
		observed pipeline.Graph, result pipeline.ImportResult, selection pipeline.MatchConditionSelection,
	) pipeline.Graph {
		entered <- struct{}{}
		<-gate
		return withCandidates(observed, result, selection)
	}
	return layers
}

// serveAsync は要求を別の goroutine で処理し、応答を channel で返す。
func serveAsync(handler http.Handler, method, path string) <-chan *httptest.ResponseRecorder {
	answered := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		answered <- response
	}()
	return answered
}

func awaitResponse(
	t *testing.T, answered <-chan *httptest.ResponseRecorder, what string,
) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case response := <-answered:
		return response
	case <-time.After(buildingWait):
		t.Fatalf("%s did not answer within %s", what, buildingWait)
		return nil
	}
}

// グラフを組んでいる間も、グラフを読まない要求は待たずに返る。
func TestSourcesAnswerWhileTheGraphIsBuilding(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	store := pipeline.NewMemoryStore(graphImportResult(t), &testAssertionClock{})
	handler := handlerWithLayers(store, gatedLayers(gate, entered))

	graph := serveAsync(handler, http.MethodGet, withAllMatchConditions(graphPath+"?"+wholeGraphQuery))
	select {
	case <-entered:
	case <-time.After(buildingWait):
		t.Fatal("the graph request did not start the build")
	}
	sources := awaitResponse(t, serveAsync(handler, http.MethodGet, "/api/v0/sources"), "the sources request")
	if sources.Code != http.StatusOK {
		t.Fatalf("the sources request answered %d while the graph was building, want 200", sources.Code)
	}
	select {
	case response := <-graph:
		t.Fatalf("the graph request answered %d before its build finished", response.Code)
	default:
	}
	close(gate)
	if response := awaitResponse(t, graph, "the graph request"); response.Code != http.StatusOK {
		t.Fatalf("the graph request answered %d after the build, want 200: %s",
			response.Code, response.Body.String())
	}
}

// 時系列は観測の層で返すため、関連付けの候補のエッジを足している間も返る。
func TestTheTimelineAnswersWhileTheCandidateEdgesAreBuilding(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	store := pipeline.NewMemoryStore(graphImportResult(t), &testAssertionClock{})
	handler := handlerWithLayers(store, gatedLayers(gate, entered))

	graph := serveAsync(handler, http.MethodGet, withAllMatchConditions(graphPath+"?"+wholeGraphQuery))
	select {
	case <-entered:
	case <-time.After(buildingWait):
		t.Fatal("the graph request did not start adding the candidate edges")
	}
	timeline := awaitResponse(t,
		serveAsync(handler, http.MethodGet, withAllMatchConditions("/api/v0/timeline")), "the timeline request")
	if timeline.Code != http.StatusOK {
		t.Fatalf("the timeline answered %d while the candidate edges were building, want 200: %s",
			timeline.Code, timeline.Body.String())
	}
	close(gate)
	if response := awaitResponse(t, graph, "the graph request"); response.Code != http.StatusOK {
		t.Fatalf("the graph request answered %d after the build, want 200", response.Code)
	}
}

// 組み立ての panic は internal_error の 500 になり、server は応答を続ける。
func TestTheGraphRequestAnswersInternalErrorWhenTheBuildPanics(t *testing.T) {
	store := pipeline.NewMemoryStore(graphImportResult(t), &testAssertionClock{})
	panicking := pipeline.DefaultGraphLayers()
	panicking.WithCandidates = func(
		pipeline.Graph, pipeline.ImportResult, pipeline.MatchConditionSelection,
	) pipeline.Graph {
		panic("the test builder panics")
	}
	handler := handlerWithLayers(store, panicking)

	apiError := requestGraphError(t, handler, wholeGraphQuery, http.StatusInternalServerError)
	if apiError.Code != core.ApiErrorCodeInternalError {
		t.Errorf("the error code is %q, want %q", apiError.Code, core.ApiErrorCodeInternalError)
	}
	if sources := requestPath(t, handler, http.MethodGet, "/api/v0/sources"); sources.Code != http.StatusOK {
		t.Errorf("the sources request answered %d after the panic, want 200", sources.Code)
	}
}

// screenDefaultSelectionQuery は、画面が最初に送る選択の文字列である。画面は契約が定める
// すべての条件を選び、幅を取る条件には幅 0 を付ける (frontend の everyMatchCondition)。
func screenDefaultSelectionQuery() string {
	var query strings.Builder
	for index, key := range core.KnownConditionKeys() {
		if index > 0 {
			query.WriteString("&")
		}
		query.WriteString(matchConditionParam + "=" + string(key))
		if key == core.ConditionKeySecondOfTime {
			query.WriteString("~0")
		}
	}
	return query.String()
}

// 起動時に組んだ選択のグラフを、画面が最初に送る選択の要求がそのまま使い、組み直さない。
func TestTheScreenDefaultSelectionUsesTheGraphBuiltAtStartup(t *testing.T) {
	store := pipeline.NewMemoryStore(graphImportResult(t), &testAssertionClock{})
	layers := pipeline.DefaultGraphLayers()
	withCandidates := layers.WithCandidates
	var built []string
	var mu sync.Mutex
	layers.WithCandidates = func(
		observed pipeline.Graph, result pipeline.ImportResult, selection pipeline.MatchConditionSelection,
	) pipeline.Graph {
		mu.Lock()
		built = append(built, fmt.Sprint(selection.Conditions))
		mu.Unlock()
		return withCandidates(observed, result, selection)
	}
	catalog := pipeline.NewGraphCatalog(store, layers)
	if err := catalog.Prepare(context.Background(), pipeline.AllMatchConditions()); err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewHandler(store, catalog, pipeline.SigmaEvaluation{}, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}

	response := requestPath(t, handler, http.MethodGet,
		graphPath+"?depth=1&"+screenDefaultSelectionQuery())
	if response.Code != http.StatusOK {
		t.Fatalf("the default request answered %d: %s", response.Code, response.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(built) != 1 {
		t.Errorf("the candidate layer was built for %v, want only the selection prepared at startup", built)
	}
}
