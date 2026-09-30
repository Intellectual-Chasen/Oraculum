package pipeline_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// catalogWait は、別の goroutine の進行を待つ上限である。上限に達したら test を失敗させ、
// 永久に止まらないようにする。
const catalogWait = 5 * time.Second

// buildCall は、差し替えた組み立て関数が受け取った割当の更新回数と選択である。
type buildCall struct {
	revision  int
	selection string
}

// recordingBuilder は、受け取った引数を記録し、gate が返す channel が閉じるまで組み立てを止める。
type recordingBuilder struct {
	mu      sync.Mutex
	calls   []buildCall
	entered chan buildCall
	// gate は呼び出しごとに待つ channel を返す。nil を返した呼び出しは待たない。
	gate func(buildCall) <-chan struct{}
	// panics は、何回目の呼び出しで panic するかを返す。
	panics func(index int) bool
}

func newRecordingBuilder() *recordingBuilder {
	return &recordingBuilder{entered: make(chan buildCall, 16)}
}

// layers は、観測の層を空のグラフとし、候補の層で呼び出しを記録する組み方を返す。
func (b *recordingBuilder) layers() pipeline.GraphLayers {
	return pipeline.GraphLayers{
		Observed:       func(pipeline.ImportResult) pipeline.Graph { return pipeline.Graph{} },
		WithCandidates: b.build,
	}
}

func (b *recordingBuilder) build(
	_ pipeline.Graph, result pipeline.ImportResult, selection pipeline.MatchConditionSelection,
) pipeline.Graph {
	call := buildCall{revision: len(result.AnalystTerminalAssignments()), selection: selectionName(selection)}
	b.mu.Lock()
	index := len(b.calls)
	b.calls = append(b.calls, call)
	b.mu.Unlock()
	b.entered <- call
	if b.gate != nil {
		if gate := b.gate(call); gate != nil {
			<-gate
		}
	}
	if b.panics != nil && b.panics(index) {
		panic("the test builder panics")
	}
	return pipeline.Graph{}
}

func (b *recordingBuilder) recorded() []buildCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.calls)
}

// selectionName は選択の条件の種別を並びのまま連ねる。test の中で選択を指すためだけに使う。
func selectionName(selection pipeline.MatchConditionSelection) string {
	keys := make([]string, 0, len(selection.Conditions))
	for _, condition := range selection.Conditions {
		keys = append(keys, string(condition.ConditionKey))
	}
	return strings.Join(keys, ",")
}

// revisionedAssignments は、割当の更新回数だけを増やせる保存先である。更新回数と同じ数の空の割当を返す。
type revisionedAssignments struct {
	mu       sync.Mutex
	revision int64
}

func (s *revisionedAssignments) List() ([]core.TerminalAssignment, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return make([]core.TerminalAssignment, s.revision), s.revision
}

func (s *revisionedAssignments) Create(pipeline.TerminalAssignmentDraft) (core.TerminalAssignment, error) {
	return core.TerminalAssignment{}, errors.New("the test store records no assignment")
}

func (s *revisionedAssignments) Revision() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision
}

func (s *revisionedAssignments) advance() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision++
}

// catalogStore は、取り込み結果が空で、割当の更新回数を増やせる保存先である。
type catalogStore struct {
	assignments *revisionedAssignments
	assertions  pipeline.AssertionStore
}

func newCatalogStore() catalogStore {
	return catalogStore{
		assignments: &revisionedAssignments{},
		assertions:  pipeline.NewMemoryAssertionStore(pipeline.SystemClock{}),
	}
}

func (s catalogStore) ImportResult() pipeline.ImportResult { return pipeline.ImportResult{} }

func (s catalogStore) Assertions() pipeline.AssertionStore { return s.assertions }

func (s catalogStore) TerminalAssignments() pipeline.TerminalAssignmentStore { return s.assignments }

func (s catalogStore) Assist() pipeline.AssistStore { return pipeline.UnavailableAssistStore() }

func (s catalogStore) AssistProposals() pipeline.AssistProposalStore {
	return pipeline.NewUnavailableAssistProposalStore()
}

// ipSelection と portSelection は、鍵が別になる 2 つの選択である。
var (
	ipSelection = pipeline.MatchConditionSelection{Conditions: []pipeline.SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyDestinationIp},
	}}
	portSelection = pipeline.MatchConditionSelection{Conditions: []pipeline.SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyDestinationPort},
	}}
)

func receive[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(catalogWait):
		t.Fatalf("%s did not happen within %s", what, catalogWait)
		var zero T
		return zero
	}
}

// graphResult は Graph の呼び出し 1 回の結果である。
type graphResult struct {
	result pipeline.ImportResult
	err    error
}

func requestGraph(
	ctx context.Context, catalog *pipeline.GraphCatalog, selection pipeline.MatchConditionSelection,
) <-chan graphResult {
	answered := make(chan graphResult, 1)
	go func() {
		_, result, err := catalog.Graph(ctx, selection)
		answered <- graphResult{result: result, err: err}
	}()
	return answered
}

// 同じ選択を同時に求めた要求は、組み立て 1 回の結果を共有する。
func TestTheCatalogBuildsOneGraphForConcurrentRequests(t *testing.T) {
	gate := make(chan struct{})
	builder := newRecordingBuilder()
	builder.gate = func(buildCall) <-chan struct{} { return gate }
	catalog := pipeline.NewGraphCatalog(newCatalogStore(), builder.layers())

	first := requestGraph(context.Background(), catalog, ipSelection)
	receive(t, builder.entered, "the first build")
	second := requestGraph(context.Background(), catalog, ipSelection)
	close(gate)
	for _, answered := range []<-chan graphResult{first, second} {
		if got := receive(t, answered, "the answer"); got.err != nil {
			t.Fatalf("a request failed: %v", got.err)
		}
	}
	want := []buildCall{{revision: 0, selection: selectionName(ipSelection)}}
	if got := builder.recorded(); !slices.Equal(got, want) {
		t.Errorf("the builder received %v, want %v", got, want)
	}
}

// 割当の更新回数が増えた後の要求は新しい入力の状態で組む。古い入力の状態の組み立ては、終わっても新しい入力の状態の表へ入らない。
func TestTheCatalogKeepsAStaleBuildOutOfTheNewRevision(t *testing.T) {
	staleGate := make(chan struct{})
	builder := newRecordingBuilder()
	builder.gate = func(call buildCall) <-chan struct{} {
		if call.revision == 0 {
			return staleGate
		}
		return nil
	}
	store := newCatalogStore()
	catalog := pipeline.NewGraphCatalog(store, builder.layers())

	stale := requestGraph(context.Background(), catalog, ipSelection)
	receive(t, builder.entered, "the build for the first revision")
	store.assignments.advance()
	current := receive(t, requestGraph(context.Background(), catalog, ipSelection), "the current answer")
	if current.err != nil || len(current.result.AnalystTerminalAssignments()) != 1 {
		t.Fatalf("the current answer carries %d assignments (err=%v), want the one of the new revision",
			len(current.result.AnalystTerminalAssignments()), current.err)
	}
	close(staleGate)
	late := receive(t, stale, "the stale answer")
	if late.err != nil || len(late.result.AnalystTerminalAssignments()) != 0 {
		t.Fatalf("the stale answer carries %d assignments (err=%v), want the result it started from",
			len(late.result.AnalystTerminalAssignments()), late.err)
	}
	// 新しい入力の状態の表は、新しい入力の状態で組んだグラフを保ち続ける。組み直しは起きない。
	receive(t, requestGraph(context.Background(), catalog, ipSelection), "the repeated answer")
	want := []buildCall{
		{revision: 0, selection: selectionName(ipSelection)},
		{revision: 1, selection: selectionName(ipSelection)},
	}
	if got := builder.recorded(); !slices.Equal(got, want) {
		t.Errorf("the builder received %v, want %v", got, want)
	}
}

// 割当を記録した合図の後は、1 つ前の入力の状態で組み終えていた選択を、新しく組み終えた順に組み直す。
func TestTheCatalogRebuildsTheViewedSelectionsAfterTheSignal(t *testing.T) {
	builder := newRecordingBuilder()
	store := newCatalogStore()
	catalog := pipeline.NewGraphCatalog(store, builder.layers())
	for _, selection := range []pipeline.MatchConditionSelection{ipSelection, portSelection} {
		if err := catalog.Prepare(context.Background(), selection); err != nil {
			t.Fatal(err)
		}
		receive(t, builder.entered, "the first build of "+selectionName(selection))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go catalog.Run(ctx)

	store.assignments.advance()
	catalog.Invalidate()
	rebuilt := []buildCall{
		receive(t, builder.entered, "the first rebuild"),
		receive(t, builder.entered, "the second rebuild"),
	}
	want := []buildCall{
		{revision: 1, selection: selectionName(portSelection)},
		{revision: 1, selection: selectionName(ipSelection)},
	}
	if !slices.Equal(rebuilt, want) {
		t.Errorf("the signal rebuilt %v, want %v", rebuilt, want)
	}
}

// 同じ入力の状態の 2 つの選択は、観測の層を 1 回だけ組んで共有する。
func TestTheCatalogBuildsTheObservedLayerOncePerRevision(t *testing.T) {
	var mu sync.Mutex
	var observedResults []int
	builder := newRecordingBuilder()
	layers := builder.layers()
	layers.Observed = func(result pipeline.ImportResult) pipeline.Graph {
		mu.Lock()
		defer mu.Unlock()
		observedResults = append(observedResults, len(result.AnalystTerminalAssignments()))
		return pipeline.Graph{}
	}
	store := newCatalogStore()
	catalog := pipeline.NewGraphCatalog(store, layers)
	for _, selection := range []pipeline.MatchConditionSelection{ipSelection, portSelection} {
		if err := catalog.Prepare(context.Background(), selection); err != nil {
			t.Fatal(err)
		}
	}
	store.assignments.advance()
	if err := catalog.Prepare(context.Background(), ipSelection); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []int{0, 1}; !slices.Equal(observedResults, want) {
		t.Errorf("the observed layer was built for the revisions %v, want %v", observedResults, want)
	}
	if got := len(builder.recorded()); got != 3 {
		t.Errorf("the candidate layer was built %d times, want one per selection and revision", got)
	}
}

// 組み立ての panic は要求の error になり、server を止めない。次の要求は組み直す。
func TestTheCatalogTurnsAPanicIntoAnError(t *testing.T) {
	builder := newRecordingBuilder()
	builder.panics = func(index int) bool { return index == 0 }
	catalog := pipeline.NewGraphCatalog(newCatalogStore(), builder.layers())

	failed := receive(t, requestGraph(context.Background(), catalog, ipSelection), "the failed answer")
	if failed.err == nil || errors.Is(failed.err, context.Canceled) {
		t.Fatalf("the panicking build answered err=%v, want an error of the build", failed.err)
	}
	retried := receive(t, requestGraph(context.Background(), catalog, ipSelection), "the retried answer")
	if retried.err != nil {
		t.Fatalf("the retried build failed: %v", retried.err)
	}
	if got := len(builder.recorded()); got != 2 {
		t.Errorf("the builder ran %d times, want the failed build and one retry", got)
	}
}

// 要求を取り消すと待つのをやめる。組み立ては続き、後の要求がその結果を使う。
func TestTheCatalogStopsWaitingWhenTheRequestEnds(t *testing.T) {
	gate := make(chan struct{})
	builder := newRecordingBuilder()
	builder.gate = func(buildCall) <-chan struct{} { return gate }
	catalog := pipeline.NewGraphCatalog(newCatalogStore(), builder.layers())

	ctx, cancel := context.WithCancel(context.Background())
	abandoned := requestGraph(ctx, catalog, ipSelection)
	receive(t, builder.entered, "the build")
	cancel()
	if got := receive(t, abandoned, "the abandoned answer"); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("the abandoned request answered err=%v, want the cancellation", got.err)
	}
	close(gate)
	if got := receive(t, requestGraph(context.Background(), catalog, ipSelection), "the later answer"); got.err != nil {
		t.Fatalf("the later request failed: %v", got.err)
	}
	if got := len(builder.recorded()); got != 1 {
		t.Errorf("the builder ran %d times, want the one build the abandoned request started", got)
	}
}
