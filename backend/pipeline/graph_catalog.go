package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// GraphLayers は、グラフを 2 つの層に分けて組む関数の組である。本番の経路は DefaultGraphLayers を渡す。
type GraphLayers struct {
	// Observed は、取り込み結果から関連付けの条件の選択に依らない観測の層を組む。
	Observed func(ImportResult) Graph
	// WithCandidates は、観測の層の複製に、選択で関連付けが挙げた候補のエッジを足す。
	// 観測の層を書き換えない。
	WithCandidates func(observed Graph, result ImportResult, selection MatchConditionSelection) Graph
}

// DefaultGraphLayers は本番の組み方 (NewObservedGraph と Graph.WithCandidateEdges) を返す。
func DefaultGraphLayers() GraphLayers {
	return GraphLayers{Observed: NewObservedGraph, WithCandidates: Graph.WithCandidateEdges}
}

// GraphCatalog は、分析者の入力の状態と関連付けの条件の選択ごとに、組んだグラフを保つ。
//
// **分析者の入力の更新回数は、端末の割当の更新回数と収集元の時刻の解釈の更新回数の和である** (currentState)。
// どちらかを記録すると入力の更新回数が増える。
//
// **組み立てを要求から切り離して走らせる。** 組み立ての間も、グラフを読まない要求は待たない。
// 同じ入力の状態と選択を同時に求めた要求は、組み立て中の 1 件の完了を待つ。
//
// **観測の層は入力の状態ごとに 1 回だけ組む。** 選択を変えた要求は、関連付けの候補のエッジだけを足す。
//
// **入力の状態が変わったら、その後の要求は新しい入力の状態の取り込み結果で組む。** 古い入力の状態で始めた組み立ては
// 終わっても新しい入力の状態の表へ入らない。
type GraphCatalog struct {
	store  InvestigationStore
	layers GraphLayers
	// rebuild は、割当または時刻の解釈の記録の後に組み直しを始める合図である。容量は 1 であり、
	// 続けて届いた合図は 1 回の組み直しにまとまる。
	rebuild chan struct{}
	// candidateSlots は、関連付けの候補のエッジを同時に足す組み立ての数を抑える。
	candidateSlots chan struct{}

	mu    sync.Mutex
	state *catalogState
}

// catalogState は分析者の入力の状態 1 つに対応する取り込み結果と、その入力の状態で組んだグラフの表である。
type catalogState struct {
	// revision は入力の更新回数であり、端末の割当の更新回数と収集元の時刻の解釈の更新回数の和である。
	revision int64
	// result は、この入力の状態の割当と時刻の解釈を当てた取り込み結果である。
	result ImportResult
	// observed は、この入力の状態の観測の層である。最初に求めた要求が組み立てを始める。
	observed *layerEntry
	// entries は選択の鍵から、組み立て中または組み終えたグラフを探す。
	entries map[string]*catalogEntry
	// completed は組み終えた鍵を、組み終えた順に並べる。保持数を超えたら先頭を捨てる。
	completed []string
	// previous は、1 つ前の入力の状態で組み終えていた選択である。新しく組み終えた順に並ぶ。
	// 割当の記録の後の組み直しが、分析者の見ていた選択を先に組むために使う。
	previous []MatchConditionSelection
}

// layerEntry は観測の層 1 つである。done が閉じた後に graph と err を読む。
type layerEntry struct {
	done  chan struct{}
	graph Graph
	err   error
}

// catalogEntry は選択 1 つのグラフである。done が閉じた後に graph と err を読む。
type catalogEntry struct {
	selection MatchConditionSelection
	done      chan struct{}
	graph     Graph
	err       error
}

// retainedMatchSelections は、1 つの入力の状態で同時に保つ選択の数である。
//
// 既知の制限: 保持数を固定の値に置き、グラフの大きさで数えない,
// 条件の選択は要求が持つ外部入力であり、上限を置かないと幅を 1 ずつ変える要求だけで
// グラフが際限なく積もる。保持数を減らすと、保持から外れた選択へ戻る操作が組み直しを待つ,
// 1 人の分析者が上限を超える数の選択を見比べる操作を記録したとき、または server の RSS が
// 分析者の端末の記憶域を超えたときに、グラフの大きさで数える形にする。
const retainedMatchSelections = 4

// concurrentCandidateBuilds は、関連付けの候補のエッジを同時に足す組み立ての数である。
//
// 既知の制限: 同時に組む数を固定の値に置く,
// 1 つの組み立てが CPU の数までの goroutine で候補集合を組み (buildCandidateSets)、選択ごとに
// グラフの複製を保つ。同時に組む数を抑えないと、条件を続けて変える操作だけで CPU と memory を
// 使い切る。2 本を超えたときの所要は repo の fixture の大きさでは測れない,
// 候補のエッジを足す所要が 1 秒を下回ったとき、または分析者が 2 つより多い選択を同時に
// 見比べる操作を記録したときに見直す。
const concurrentCandidateBuilds = 2

// NewGraphCatalog は store の取り込み結果と割当から、layers でグラフを組む catalog を返す。
func NewGraphCatalog(store InvestigationStore, layers GraphLayers) *GraphCatalog {
	return &GraphCatalog{
		store: store, layers: layers,
		rebuild:        make(chan struct{}, 1),
		candidateSlots: make(chan struct{}, concurrentCandidateBuilds),
	}
}

// Current は今の入力の状態を当てた取り込み結果と、入力の更新回数を返す。グラフの組み立てを待たない。
func (c *GraphCatalog) Current() (ImportResult, int64) {
	state := c.currentState()
	return state.result, state.revision
}

// Graph は、今の入力の状態と selection に対応するグラフと、そのグラフを組んだ取り込み結果を返す。
//
// 組み立て中なら完了を待つ。ctx が取り消されたら待つのをやめて ctx の error を返す。
// **組み立ては止めない。** 後の要求がその結果を使う。
func (c *GraphCatalog) Graph(
	ctx context.Context, selection MatchConditionSelection,
) (Graph, ImportResult, error) {
	state := c.currentState()
	entry := c.entryFor(state, selection)
	if err := await(ctx, entry.done); err != nil {
		return Graph{}, ImportResult{}, err
	}
	if entry.err != nil {
		return Graph{}, ImportResult{}, entry.err
	}
	return entry.graph, state.result, nil
}

// Observed は、今の入力の状態の観測の層と、それを組んだ取り込み結果を返す。関連付けの候補の
// エッジを持たないため、選択ごとの組み立てを待たない。
func (c *GraphCatalog) Observed(ctx context.Context) (Graph, ImportResult, error) {
	state := c.currentState()
	entry := c.observedEntry(state)
	if err := await(ctx, entry.done); err != nil {
		return Graph{}, ImportResult{}, err
	}
	if entry.err != nil {
		return Graph{}, ImportResult{}, entry.err
	}
	return entry.graph, state.result, nil
}

// Prepare は selection のグラフを組み終えるまで待つ。server が待ち受けを始める前に呼ぶ。
func (c *GraphCatalog) Prepare(ctx context.Context, selection MatchConditionSelection) error {
	_, _, err := c.Graph(ctx, selection)
	return err
}

// Invalidate は、端末の割当または収集元の時刻の解釈を記録した後の組み直しを始める合図を送る。
// 待たない。
func (c *GraphCatalog) Invalidate() {
	select {
	case c.rebuild <- struct{}{}:
	default:
		// 合図が既に 1 件待っている。その合図の組み直しが最新の入力の状態を読むため、この合図は要らない。
	}
}

// Run は、合図を受けるたびに最新の入力の状態で、1 つ前の入力の状態が保っていた選択のグラフを組む。
// ctx が取り消されたら戻る。
func (c *GraphCatalog) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.rebuild:
		}
		for _, selection := range c.currentState().previous {
			if err := c.Prepare(ctx, selection); err != nil && ctx.Err() != nil {
				return
			}
		}
	}
}

// await は done が閉じるまで待つ。ctx が先に取り消されたら ctx の error を返す。
func await(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for the graph: %w", ctx.Err())
	}
}

// currentState は今の入力の状態を返す。入力の更新回数が増えていれば新しい状態へ差し替える。
//
// **入力の更新回数は、端末の割当の更新回数と、収集元の時刻の解釈の更新回数の和である。** どちらの更新回数も記録のたびに
// 増えるため、和も増える。時刻の解釈は、並びと関連付けの時刻の範囲に使う時点を変える。
//
// **新しい入力の状態の取り込み結果は lock の外で組む。** 時刻の解釈を当てる操作は解釈を持つ収集元の
// レコードを複製するため、lock の中で組むと、その間グラフを読む要求がすべて待つ。同じ入力の状態を
// 同時に組んだ 2 本のうち、先に lock を取った 1 本の結果が残る。
func (c *GraphCatalog) currentState() *catalogState {
	inputs := analystInputsOf(c.store)
	if current := c.stateAtLeast(inputs.revision); current != nil {
		return current
	}
	result := inputs.applyTo(c.store.ImportResult())
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != nil && c.state.revision >= inputs.revision {
		return c.state
	}
	if conflicted := inputs.interpretations.conflicted; len(conflicted) > 0 {
		// 保存先は同じ収集元の 2 件目を退けるため、ここに来るのは保存先の外から入った所見である。
		// 出すのは収集元の内容の digest だけで、分析者の記述を載せない。
		slog.Warn("time interpretations conflict on a source and are not applied",
			slog.Any("source_content_sha256", conflicted))
	}
	next := &catalogState{revision: inputs.revision, result: result, entries: map[string]*catalogEntry{}}
	if c.state != nil {
		for index := len(c.state.completed) - 1; index >= 0; index-- {
			// 表に無い鍵は組み直す選択に入れない。失敗した entry は表から外れている。
			if entry, found := c.state.entries[c.state.completed[index]]; found {
				next.previous = append(next.previous, entry.selection)
			}
		}
	}
	c.state = next
	return next
}

// analystInputs は、保存先が持つ分析者の入力 (端末の割当と収集元の時刻の解釈) と、その更新回数である。
type analystInputs struct {
	assignments     []core.TerminalAssignment
	interpretations sourceTimeInterpretations
	// revision は、端末の割当の更新回数と収集元の時刻の解釈の更新回数の和である。
	revision int64
}

// analystInputsOf は store の分析者の入力を読む。
func analystInputsOf(store InvestigationStore) analystInputs {
	assignments, assignmentRevision := store.TerminalAssignments().List()
	interpretations := timeInterpretationsOf(store.Assertions().List())
	return analystInputs{
		assignments: assignments, interpretations: interpretations,
		revision: assignmentRevision + interpretations.revision,
	}
}

// applyTo は result に端末の割当と収集元の時刻の解釈を当てた取り込み結果を返す。
func (i analystInputs) applyTo(result ImportResult) ImportResult {
	return result.WithAnalystTerminalAssignments(i.assignments).withTimeInterpretations(i.interpretations.applied)
}

// CurrentImportResult は、store の取り込み結果に、今の端末の割当と収集元の時刻の解釈を当てた結果を
// 返す。GraphCatalog.Current と同じ組み立てであり、グラフを組まない。
//
// **処理を終える前の収集元の一覧が使う。** 処理を終えた後の一覧 (GraphCatalog.Current) と同じ
// 項目を答える。
func CurrentImportResult(store InvestigationStore) ImportResult {
	return analystInputsOf(store).applyTo(store.ImportResult())
}

// stateAtLeast は、入力の更新回数が revision 以上の状態を持っていればそれを返す。無ければ nil を返す。
func (c *GraphCatalog) stateAtLeast(revision int64) *catalogState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != nil && c.state.revision >= revision {
		return c.state
	}
	return nil
}

// observedEntry は state の観測の層を返す。まだ組んでいなければ組み立てを始める。
func (c *GraphCatalog) observedEntry(state *catalogState) *layerEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state.observed != nil {
		return state.observed
	}
	entry := &layerEntry{done: make(chan struct{})}
	state.observed = entry
	go func() {
		defer close(entry.done)
		entry.err = recovering("building the observed layer", func() {
			entry.graph = c.layers.Observed(state.result)
		})
	}()
	return entry
}

// entryFor は state の表から selection の entry を探す。無ければ組み立てを始めた entry を足す。
func (c *GraphCatalog) entryFor(state *catalogState, selection MatchConditionSelection) *catalogEntry {
	key := selection.key()
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, found := state.entries[key]; found {
		return entry
	}
	entry := &catalogEntry{selection: selection, done: make(chan struct{})}
	state.entries[key] = entry
	go c.fill(state, key, entry)
	return entry
}

// fill は entry のグラフを組み、完了を知らせる。
func (c *GraphCatalog) fill(state *catalogState, key string, entry *catalogEntry) {
	defer func() {
		c.finish(state, key, entry)
		close(entry.done)
	}()
	observed := c.observedEntry(state)
	<-observed.done
	if observed.err != nil {
		entry.err = observed.err
		return
	}
	c.candidateSlots <- struct{}{}
	entry.err = recovering("adding the candidate edges", func() {
		entry.graph = c.layers.WithCandidates(observed.graph, state.result, entry.selection)
	})
	<-c.candidateSlots
}

// recovering は work を走らせ、panic を error として返す。
//
// **panic を error へ移す。** 要求から切り離した goroutine の panic は net/http が回復しないため、
// server の process 全体が止まる。
func recovering(doing string, work func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s panicked: %v", doing, recovered)
			slog.Error("building the graph panicked", "error", err)
		}
	}()
	work()
	return nil
}

// finish は組み終えた entry を state の表へ記録し、保持数を超えた分を組み終えた順に捨てる。
//
// **失敗した entry を表に残さない。** 次の要求が組み直す。組み立て中の entry は捨てない。
func (c *GraphCatalog) finish(state *catalogState, key string, entry *catalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry.err != nil {
		if state.entries[key] == entry {
			delete(state.entries, key)
		}
		return
	}
	state.completed = append(state.completed, key)
	for len(state.completed) > retainedMatchSelections {
		delete(state.entries, state.completed[0])
		state.completed = state.completed[1:]
	}
}

// key は選択 1 つを、組んだグラフを探す鍵の文字列へ直す。
//
// **並びで別の鍵にしない。** 同じ条件の組を別の順で書いた 2 つの要求は同じ関連付けを求める。
func (s MatchConditionSelection) key() string {
	items := make([]string, 0, len(s.Conditions))
	for _, condition := range s.Conditions {
		items = append(items, string(condition.ConditionKey)+"~"+
			strconv.FormatInt(condition.Tolerance, 10))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}
