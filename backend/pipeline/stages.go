package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 段階の操作の失敗。api が status を割り当てる。
var (
	// ErrStageNotReady は、操作が必要とする段階をまだ終えていない失敗である。
	ErrStageNotReady = errors.New("stages: the stage the operation needs has not completed")
	// ErrStageAlreadyStarted は、始めようとした段階が実行中か完了済みである失敗である。
	ErrStageAlreadyStarted = errors.New("stages: the stage is running or has completed")
	// ErrLoadingRequestInvalid は、読み込みの要求が読み込みを始められない値を持つ失敗である。
	ErrLoadingRequestInvalid = errors.New("stages: the loading request is invalid")
	// ErrStagesStopped は、server の停止の合図を受けた後に段階を始めようとした失敗である。
	ErrStagesStopped = errors.New("stages: the server is stopping")
	// ErrRequestedFileNotRegular は、要求の path が通常の file を指さない失敗である。
	// RequestedFiles の実装が返す。
	ErrRequestedFileNotRegular = errors.New("stages: the requested path is not a regular file")
	// ErrRequestedPathOutsideBase は、要求の path、または path が辿る symbolic link が、基準の
	// directory から上へ出ずに辿れる相対 path の条件に合わない失敗である。途中で基準の外へ出て
	// から戻る symbolic link と、絶対 path の symbolic link を含む。RequestedFiles の実装が返す。
	ErrRequestedPathOutsideBase = errors.New("stages: the requested path leaves the base directory")
)

// errRecording は、読み込んだ収集元を調査へ記録する処理の失敗に付ける印である。
var errRecording = errors.New("recording the investigation")

// LoadingRequestError は読み込みの要求を退けた失敗である。errors.Is で ErrLoadingRequestInvalid に
// 該当する。
type LoadingRequestError struct {
	// Rejection は退けた理由の種別である。
	Rejection core.LoadingRejection
	// OriginPath は退けた収集元の、要求が指した path の原文である。要求全体を退けたときは
	// 空文字列である。
	OriginPath string
	// err は退けた理由である。文字列は要求の値だけから組む。
	err error
	// cause は RequestedFiles の実装が返した失敗である。要求の file を確かめる前に退けたときは
	// nil である。
	cause error
}

// Message は要求の応答に載せる文字列を返す。RequestedFiles の実装が返した失敗の文字列 (基準の
// directory の path と OS の失敗の文字列を含みうる) を含まない。
func (e *LoadingRequestError) Message() string {
	if e.OriginPath == "" {
		return fmt.Sprintf("%v: %v", ErrLoadingRequestInvalid, e.err)
	}
	return fmt.Sprintf("%v: source %q: %v", ErrLoadingRequestInvalid, e.OriginPath, e.err)
}

// Error は Message に、RequestedFiles の実装が返した失敗の文字列を足す。運用者への log が使う。
func (e *LoadingRequestError) Error() string {
	if e.cause == nil {
		return e.Message()
	}
	return fmt.Sprintf("%s: %v", e.Message(), e.cause)
}

func (e *LoadingRequestError) Unwrap() []error {
	if e.cause == nil {
		return []error{ErrLoadingRequestInvalid, e.err}
	}
	return []error{ErrLoadingRequestInvalid, e.err, e.cause}
}

// ImportRecorder は、読み込んだ収集元を記録し、分析者の値の保存先を返す。
//
// 実装は、調査の SQLite file に記録する Investigation と、何も記録しない NewMemoryRecorder の
// 2 つである。
type ImportRecorder interface {
	// Ordinals は取り込みに渡す通番の発行器を、取り込み 1 回ごとに新しく返す。
	Ordinals() ImportOrdinalSource
	// Settle は取り込みの後に収集元を記録し、分析者の値の保存先を返す。
	Settle(ctx context.Context, plans []SourcePlan, result ImportResult, clock AssertionClock) (InvestigationStore, error)
}

// memoryRecorder は何も記録しない ImportRecorder である。調査の directory を渡さない起動が使う。
type memoryRecorder struct{}

// NewMemoryRecorder は、分析者の値をメモリに置き、停止すると消える記録先を返す。
func NewMemoryRecorder() ImportRecorder { return memoryRecorder{} }

func (memoryRecorder) Ordinals() ImportOrdinalSource { return NewInMemoryOrdinals() }

func (memoryRecorder) Settle(
	_ context.Context, _ []SourcePlan, result ImportResult, clock AssertionClock,
) (InvestigationStore, error) {
	return NewMemoryStore(result, clock), nil
}

// RequestedFiles は、読み込みの要求が指す file を、server が決めた基準の directory の下だけで
// 開く口である。
//
// **基準の外を指す path を退けるのは実装の責務である。** 失敗は、file が無いとき
// fs.ErrNotExist を、読む権限が無いとき fs.ErrPermission を、通常の file でないとき
// ErrRequestedFileNotRegular を、path か path が辿る symbolic link が基準の directory から
// 上へ出るか絶対 path の link であるとき ErrRequestedPathOutsideBase を包む。
// **失敗の文字列は基準の directory の path を含みうる。** 要求の応答には載せない
// (LoadingRequestError.Message)。
type RequestedFiles struct {
	// Upload は既存の資料を上書きせず、新しい相対 path に本文を保存する。nil はアップロード不可。
	Upload func(batch, name string, body io.Reader) (string, error)
	// Open は基準からの相対 path の file を開く。
	Open func(originPath string) (io.ReadCloser, error)
	// Size は基準からの相対 path の file を開いて確かめ、byte 数を返す。読めない file を、読み込みを
	// 始める前に退ける。
	Size func(originPath string) (int64, error)
	// FS は基準の directory を root にした file system である。収集の directory を辿る
	// (ExpandPlans)。基準の外を指す path と symbolic link を退けるのは実装の責務である。
	FS fs.FS
}

// StagesConfig は Stages に注入する依存である。
type StagesConfig struct {
	// Import は取り込みの依存である。Open は起動引数と記録済みの計画を開く。Ordinals は
	// 読まず、Recorder が返す発行器を使う。
	Import Config
	// Requested は要求による読み込みの file を開く口である。nil の server は要求による
	// 読み込みを受け付けない。
	Requested *RequestedFiles
	// Recorder は読み込んだ収集元の記録先である。
	Recorder ImportRecorder
	// Layers はグラフの層の組み方である。
	Layers GraphLayers
	// Clock は分析者の値の時刻の出どころである。
	Clock AssertionClock
	// Loaded は読み込みを終えた直後に呼ぶ。運用者への要約に使う。nil のときは呼ばない。
	Loaded func(plans []SourcePlan, result ImportResult) error
}

// Session は処理を終えた調査である。
type Session struct {
	Store   InvestigationStore
	Catalog *GraphCatalog
}

// Stages は、収集元の読み込みと処理の 2 つの段階を、分析者が始める操作として持つ。
//
// **段階の結果は「無い → ある」の 1 回だけ公開する。** 読み込みを終えた保存先と、処理を
// 終えた Session は、公開した後に差し替えない。公開の前後をまたぐ要求が、古い組と新しい組を
// 混ぜて読むことが無い。
type Stages struct {
	ctx    context.Context
	config StagesConfig

	mu         sync.Mutex
	loading    loadingProgress
	processing processingProgress
	store      InvestigationStore
	session    *Session
	// running は実行中の段階を数える。**Add を mu の下で、停止の合図を確かめた後に呼ぶ。**
	// 停止の合図の後に Wait と Add が競合しない。
	running sync.WaitGroup
}

// loadingProgress は読み込みの段階の状態である。mu が守る。
type loadingProgress struct {
	state   core.StageState
	failure *core.StageFailure
	sources []loadingSourceProgress
	// thenProcess は、読み込みが完了したら続けて処理を始めるかである。
	thenProcess bool
	// processingFollowed は、読み込みの完了と同じ lock の中で処理を実行中にしたかである。
	// 読み込みの goroutine が受け取って処理を行う (takeFollowingProcessing)。
	processingFollowed bool
}

// loadingSourceProgress は読み込む収集元 1 件の進行である。readBytes だけは読み込みの
// goroutine が lock を取らずに書く。
type loadingSourceProgress struct {
	plan      SourcePlan
	sizeBytes *int64
	readBytes *atomic.Int64
	status    *core.LoadedSourceStatus
}

// processingProgress は処理の段階の状態である。mu が守る。
type processingProgress struct {
	state   core.StageState
	failure *core.StageFailure
	steps   map[core.ProcessingStep]core.StageState
}

// NewStages は、どの段階も始めていない Stages を作る。ctx は段階の実行が従う server の寿命である。
func NewStages(ctx context.Context, config StagesConfig) (*Stages, error) {
	if config.Recorder == nil || config.Clock == nil || config.Layers.Observed == nil ||
		config.Layers.WithCandidates == nil {
		return nil, errors.New("creating stages: recorder, clock and graph layers are required")
	}
	// 番号の払い出しは読み込みのたびに記録先から受け取る (importAndSettle)。
	checked := config.Import
	checked.Ordinals = config.Recorder.Ordinals()
	if _, err := NewRunner(checked); err != nil {
		return nil, fmt.Errorf("creating stages: %w", err)
	}
	stages := &Stages{ctx: ctx, config: config}
	stages.loading.state = core.StageStateNotStarted
	stages.processing = newProcessingProgress(core.StageStateNotStarted)
	return stages, nil
}

func newProcessingProgress(state core.StageState) processingProgress {
	steps := make(map[core.ProcessingStep]core.StageState, len(core.ProcessingSteps()))
	for _, step := range core.ProcessingSteps() {
		steps[step] = core.StageStateNotStarted
	}
	return processingProgress{state: state, steps: steps}
}

// hasControlCharacter は、path が制御文字を持つかを返す。段階の状態は path を制御文字の
// 無い文字列で持つ (core.LoadingSource)。
func hasControlCharacter(path string) bool { return strings.ContainsFunc(path, unicode.IsControl) }

// Load は計画を同期で読み込む。起動引数の収集元と、調査に記録した収集元を読む起動が使う。
// skipped は、呼び出し側が収集の directory を展開したときに取り込まなかった file である
// (ExpandPlans)。
func (s *Stages) Load(plans []SourcePlan, skipped []core.SkippedFile) error {
	for _, plan := range plans {
		if hasControlCharacter(plan.OriginPath) {
			return fmt.Errorf("loading the sources: the path of the source %q has a control character",
				plan.OriginPath)
		}
	}
	if err := s.beginLoading(plans, nil, false); err != nil {
		return err
	}
	defer s.running.Done()
	return s.load(plans, skipped, s.config.Import.Open)
}

// StartRequestedLoading は要求が指す計画を検査し、読み込みを背景で始める。
//
// **検査は読み込みを始める前に同期で行う。** 退ける要求は段階の状態を変えない。
func (s *Stages) StartRequestedLoading(plans []SourcePlan) error {
	return s.startRequestedLoading(plans, false)
}

// StartRequestedLoadingThenProcessing は StartRequestedLoading と同じ検査の後に読み込みを背景で
// 始め、読み込みが完了したら続けて処理を始める。
//
// **読み込みが失敗したときは処理を始めない。** 処理の失敗は処理の段階の状態が持つ。
func (s *Stages) StartRequestedLoadingThenProcessing(plans []SourcePlan) error {
	return s.startRequestedLoading(plans, true)
}

func (s *Stages) startRequestedLoading(plans []SourcePlan, thenProcess bool) error {
	if s.config.Requested == nil {
		return &LoadingRequestError{
			Rejection: core.LoadingRejectionNoBaseDirectory,
			err:       errors.New("the server has no base directory for requested files"),
		}
	}
	expanded, skipped, err := s.expandRequestedPlans(plans)
	if err != nil {
		return err
	}
	checked, sizes, err := s.checkRequestedPlans(expanded)
	if err != nil {
		return err
	}
	if err := s.beginLoading(checked, sizes, thenProcess); err != nil {
		return err
	}
	go func() {
		defer s.running.Done()
		// 失敗は段階の状態と、運用者への log が持つ。
		if err := s.load(checked, skipped, s.config.Requested.Open); err != nil {
			slog.Error("loading the sources failed", "error", err)
			return
		}
		store, started := s.takeFollowingProcessing()
		if !started {
			return
		}
		defer s.running.Done()
		if err := s.process(store); err != nil {
			slog.Error("building the graph failed", "error", err)
		}
	}()
	return nil
}

// takeFollowingProcessing は、読み込みに続けて始めた処理の保存先を返す。started が偽になるのは、
// 読み込みに続けて処理を始めていないときである (load)。
func (s *Stages) takeFollowingProcessing() (store InvestigationStore, started bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	started = s.loading.processingFollowed
	s.loading.processingFollowed = false
	return s.store, started
}

// expandRequestedPlans は、要求の収集の directory を、基準の directory の下で file ごとの計画に
// 展開する (ExpandPlans)。ほかの計画は並びを保って返す。
func (s *Stages) expandRequestedPlans(plans []SourcePlan) ([]SourcePlan, []core.SkippedFile, error) {
	var expanded []SourcePlan
	var skipped []core.SkippedFile
	for _, plan := range plans {
		if plan.FormatKey != FormatKeyWindowsCollection {
			expanded = append(expanded, plan)
			continue
		}
		reject := func(rejection core.LoadingRejection, err error) ([]SourcePlan, []core.SkippedFile, error) {
			return nil, nil, &LoadingRequestError{Rejection: rejection, OriginPath: plan.OriginPath, err: err}
		}
		switch {
		case !isRequestedLocalPath(plan.OriginPath):
			return reject(core.LoadingRejectionPathOutsideBase,
				errors.New("the path must be relative to the base directory"))
		case hasControlCharacter(plan.OriginPath):
			return reject(core.LoadingRejectionPathControlCharacter, errors.New("the path has a control character"))
		case s.config.Requested.FS == nil:
			return reject(core.LoadingRejectionNoBaseDirectory,
				errors.New("the server reads no collection under the base directory"))
		}
		members, rest, err := ExpandPlans(s.config.Requested.FS, []SourcePlan{plan}, s.config.Import.Parsers)
		if err != nil {
			rejection := requestedFileRejection(err)
			if errors.Is(err, errCollectionWithoutSource) {
				rejection = core.LoadingRejectionNoSource
			}
			// 応答には種別だけを載せ、展開の失敗の文字列は原因として残す。
			return nil, nil, &LoadingRequestError{
				Rejection: rejection, OriginPath: plan.OriginPath,
				err:   fmt.Errorf("expanding the requested collection failed as %s", rejection),
				cause: err,
			}
		}
		expanded = append(expanded, members...)
		skipped = append(skipped, rest...)
	}
	return expanded, skipped, nil
}

// isRequestedLocalPath は、要求の path が基準の directory の下に留まる相対 path であるかを返す。
//
// **OS に依らず、`\` とドライブ文字で始まる path を退ける。** POSIX の filepath.IsLocal はこれらを
// 相対 path と数えるが、取り込みの記録と一覧の応答の検査 (core.IsAbsolutePath) は絶対 path として
// 退ける。
func isRequestedLocalPath(originPath string) bool {
	return filepath.IsLocal(originPath) && !core.IsAbsolutePath(originPath)
}

// requestedPlanKey は、要求の中で同じ収集元を指す計画を見分ける鍵である。
type requestedPlanKey struct {
	originPath string
	formatKey  core.FormatKey
}

// checkRequestedPlans は、要求の計画が読み込みを始められる値であることを確かめ、path を
// 整えた計画と収集元の byte 数を返す。
func (s *Stages) checkRequestedPlans(plans []SourcePlan) ([]SourcePlan, []*int64, error) {
	if len(plans) == 0 {
		return nil, nil, &LoadingRequestError{
			Rejection: core.LoadingRejectionNoSource, err: errors.New("the request carries no source"),
		}
	}
	for _, plan := range plans {
		if (plan.CaseId != nil) != (plans[0].CaseId != nil) {
			return nil, nil, &LoadingRequestError{
				Rejection: core.LoadingRejectionPartialCase,
				err:       errors.New("some sources carry a case and others do not"),
			}
		}
	}
	seen := make(map[requestedPlanKey]bool, len(plans))
	checked := make([]SourcePlan, len(plans))
	sizes := make([]*int64, len(plans))
	for index, plan := range plans {
		cleaned, size, err := s.checkRequestedPlan(plan, seen)
		if err != nil {
			return nil, nil, err
		}
		checked[index], sizes[index] = cleaned, &size
	}
	return checked, sizes, nil
}

// checkRequestedPlan は要求の収集元 1 件を確かめ、path を整えた計画と byte 数を返す。seen に
// 確かめた収集元を足す。
func (s *Stages) checkRequestedPlan(plan SourcePlan, seen map[requestedPlanKey]bool) (SourcePlan, int64, error) {
	reject := func(rejection core.LoadingRejection, err error) (SourcePlan, int64, error) {
		return SourcePlan{}, 0, &LoadingRequestError{Rejection: rejection, OriginPath: plan.OriginPath, err: err}
	}
	if !isRequestedLocalPath(plan.OriginPath) {
		return reject(core.LoadingRejectionPathOutsideBase,
			errors.New("the path must be relative to the base directory"))
	}
	if hasControlCharacter(plan.OriginPath) {
		return reject(core.LoadingRejectionPathControlCharacter, errors.New("the path has a control character"))
	}
	if plan.CaseId != nil {
		if err := core.ValidateCaseId("case", *plan.CaseId); err != nil {
			return reject(core.LoadingRejectionCaseInvalid, err)
		}
	}
	factory := s.config.Import.Parsers[plan.FormatKey]
	if factory == nil {
		return reject(core.LoadingRejectionFormatUnknown, fmt.Errorf("no parser reads the format %q", plan.FormatKey))
	}
	if _, err := factory(plan.FormatSpec); err != nil {
		return reject(core.LoadingRejectionFormatSpecInvalid, err)
	}
	if plan.Terminal != nil {
		if err := plan.Terminal.Validate(); err != nil {
			return reject(core.LoadingRejectionTerminalInvalid, err)
		}
	}
	// 要求の文字列の違い (`./a.log` と `a.log`) を、同じ収集元として扱う。
	cleaned := plan
	cleaned.OriginPath = filepath.Clean(plan.OriginPath)
	cleaned.FileName = filepath.Base(cleaned.OriginPath)
	key := requestedPlanKey{originPath: cleaned.OriginPath, formatKey: cleaned.FormatKey}
	if seen[key] {
		return reject(core.LoadingRejectionSourceRepeated, errors.New("the request names the same source twice"))
	}
	seen[key] = true
	size, err := s.config.Requested.Size(cleaned.OriginPath)
	if err != nil {
		rejection := requestedFileRejection(err)
		// 応答には種別だけを載せ、RequestedFiles の失敗の文字列は原因として残す。
		return SourcePlan{}, 0, &LoadingRequestError{
			Rejection: rejection, OriginPath: plan.OriginPath,
			err:   fmt.Errorf("checking the requested file failed as %s", rejection),
			cause: err,
		}
	}
	return cleaned, size, nil
}

// requestedFileRejection は、要求の file を確かめた失敗を退けた理由の種別にする。
func requestedFileRejection(err error) core.LoadingRejection {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return core.LoadingRejectionFileAbsent
	case errors.Is(err, ErrRequestedFileNotRegular):
		return core.LoadingRejectionFileNotRegular
	case errors.Is(err, ErrRequestedPathOutsideBase):
		return core.LoadingRejectionPathOutsideBase
	default:
		return core.LoadingRejectionFileUnreadable
	}
}

// beginLoading は読み込みの段階を実行中にし、実行中の段階に数える。実行中と完了済みの段階と、
// 停止の合図を受けた後は始めない。thenProcess は、読み込みが完了したら続けて処理を始めるかである。
func (s *Stages) beginLoading(plans []SourcePlan, sizes []*int64, thenProcess bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return ErrStagesStopped
	}
	if s.loading.state == core.StageStateRunning || s.loading.state == core.StageStateCompleted {
		return ErrStageAlreadyStarted
	}
	sources := make([]loadingSourceProgress, len(plans))
	for index, plan := range plans {
		sources[index] = loadingSourceProgress{plan: plan, readBytes: new(atomic.Int64)}
		if sizes != nil {
			sources[index].sizeBytes = sizes[index]
		}
	}
	s.loading = loadingProgress{state: core.StageStateRunning, sources: sources, thenProcess: thenProcess}
	s.running.Add(1)
	return nil
}

// load は計画を取り込んで記録し、保存先を公開する。失敗は段階の状態に残し、呼び出し元にも返す。
// skipped は取り込み結果に持たせる、収集の directory の取り込まなかった file である。
func (s *Stages) load(
	plans []SourcePlan, skipped []core.SkippedFile, open func(string) (io.ReadCloser, error),
) error {
	store, err := s.importAndSettle(plans, skipped, open)
	if err != nil {
		failure := s.loadingFailure(err)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.loading.state = core.StageStateFailed
		s.loading.failure = &failure
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, status := range store.ImportResult().Statuses() {
		summary := core.LoadedSourceStatusOf(status)
		s.loading.sources[index].status = &summary
	}
	s.loading.state = core.StageStateCompleted
	s.store = store
	// **続けて行う処理は、読み込みの完了と同じ lock の中で実行中にする。** 段階の状態を読む要求が、
	// 実行中の段階を持たない「読み込みの完了、処理の未開始」の状態を読まない。
	if s.loading.thenProcess && s.ctx.Err() == nil && s.processing.state == core.StageStateNotStarted {
		s.processing = newProcessingProgress(core.StageStateRunning)
		s.running.Add(1)
		s.loading.processingFollowed = true
	}
	return nil
}

// loadingFailure は読み込みの失敗を理由の種別にする。
func (s *Stages) loadingFailure(err error) core.StageFailure {
	var sourceErr *sourceImportError
	switch {
	case s.ctx.Err() != nil:
		return core.StageFailure{Reason: core.StageFailureReasonInterrupted}
	case errors.Is(err, errRecording):
		return core.StageFailure{Reason: core.StageFailureReasonRecordingFailed}
	case errors.As(err, &sourceErr):
		return core.StageFailure{
			Reason: core.StageFailureReasonSourceImportFailed, OriginPath: sourceErr.originPath,
		}
	default:
		return core.StageFailure{Reason: core.StageFailureReasonImportFailed}
	}
}

// importAndSettle は計画を取り込み、記録先に記録した保存先を返す。
//
// **panic を error へ移す。** 要求で始めた読み込みの goroutine の panic は net/http が回復しない
// ため、server の process 全体が止まる。panic は収集元を指さない取り込みの失敗になる (loadingFailure)。
func (s *Stages) importAndSettle(
	plans []SourcePlan, skipped []core.SkippedFile, open func(string) (io.ReadCloser, error),
) (store InvestigationStore, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			store, err = nil, fmt.Errorf("loading the sources panicked: %v", recovered)
		}
	}()
	config := s.config.Import
	config.Open = s.interruptible(open)
	config.Ordinals = s.config.Recorder.Ordinals()
	sources := s.loadingSources()
	config.Progress = func(progress SourceProgress) {
		if progress.Plan < len(sources) {
			sources[progress.Plan].readBytes.Store(progress.ReadBytes)
		}
	}
	runner, err := NewRunner(config)
	if err != nil {
		return nil, fmt.Errorf("creating import runner: %w", err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		return nil, fmt.Errorf("running import: %w", err)
	}
	result = result.WithSkippedFiles(skipped)
	if s.config.Loaded != nil {
		if err := s.config.Loaded(plans, result); err != nil {
			return nil, fmt.Errorf("reporting the import summary: %w", err)
		}
	}
	settled, err := s.config.Recorder.Settle(s.ctx, plans, result, s.config.Clock)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRecording, err)
	}
	return settled, nil
}

// interruptible は、停止の合図を受けた後の読み取りを止める口を返す。
func (s *Stages) interruptible(open func(string) (io.ReadCloser, error)) func(string) (io.ReadCloser, error) {
	return func(originPath string) (io.ReadCloser, error) {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}
		file, err := open(originPath)
		if err != nil {
			return nil, err
		}
		return interruptibleReader{ctx: s.ctx, ReadCloser: file}, nil
	}
}

// interruptibleReader は、ctx が取り消された後の Read を ctx の失敗で止める。
type interruptibleReader struct {
	ctx context.Context
	io.ReadCloser
}

func (r interruptibleReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.ReadCloser.Read(buffer)
}

func (s *Stages) loadingSources() []loadingSourceProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.loading.sources)
}

// StartProcessing は処理を背景で始める。読み込みを終えていない段階では始めない。
func (s *Stages) StartProcessing() error {
	store, err := s.beginProcessing()
	if err != nil {
		return err
	}
	go func() {
		defer s.running.Done()
		// 失敗は段階の状態と、運用者への log が持つ。
		if err := s.process(store); err != nil {
			slog.Error("building the graph failed", "error", err)
		}
	}()
	return nil
}

// Process は処理を同期で行う。test が使う。
func (s *Stages) Process() error {
	store, err := s.beginProcessing()
	if err != nil {
		return err
	}
	defer s.running.Done()
	return s.process(store)
}

// beginProcessing は処理の段階を実行中にし、実行中の段階に数える。
func (s *Stages) beginProcessing() (InvestigationStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return nil, ErrStagesStopped
	}
	if s.loading.state != core.StageStateCompleted {
		return nil, ErrStageNotReady
	}
	if s.processing.state == core.StageStateRunning || s.processing.state == core.StageStateCompleted {
		return nil, ErrStageAlreadyStarted
	}
	s.processing = newProcessingProgress(core.StageStateRunning)
	s.running.Add(1)
	return s.store, nil
}

// process はグラフを組み、処理を終えた Session を公開する。
//
// **試行ごとに新しい catalog を組む。** 失敗した試行の catalog は公開しない。
func (s *Stages) process(store InvestigationStore) (err error) {
	started := time.Now()
	catalog := NewGraphCatalog(store, s.trackedLayers())
	reason := core.StageFailureReasonGraphBuildFailed
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("building the graph panicked: %v", recovered)
			reason = core.StageFailureReasonGraphBuildFailed
		}
		if err == nil {
			s.finishProcessing(catalog, store, nil)
			slog.Info("built the graph", "duration", time.Since(started).Round(time.Millisecond))
			return
		}
		if s.ctx.Err() != nil {
			reason = core.StageFailureReasonInterrupted
		}
		s.finishProcessing(catalog, store, &core.StageFailure{Reason: reason})
	}()
	if err := catalog.Prepare(s.ctx, AllMatchConditions()); err != nil {
		return fmt.Errorf("preparing the default selection: %w", err)
	}
	return nil
}

func (s *Stages) finishProcessing(catalog *GraphCatalog, store InvestigationStore, failure *core.StageFailure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failure != nil {
		s.processing.state = core.StageStateFailed
		s.processing.failure = failure
		for step, state := range s.processing.steps {
			if state == core.StageStateRunning {
				s.processing.steps[step] = core.StageStateFailed
			}
		}
		return
	}
	for step := range s.processing.steps {
		s.processing.steps[step] = core.StageStateCompleted
	}
	s.processing.state = core.StageStateCompleted
	s.session = &Session{Store: store, Catalog: catalog}
	go catalog.Run(s.ctx)
}

// trackedLayers は、処理の段階の間だけ手順の状態を記録する層を返す。
//
// GraphCatalog は観測の層、候補のエッジの順に組む。
func (s *Stages) trackedLayers() GraphLayers {
	layers := s.config.Layers
	return GraphLayers{
		Observed: func(result ImportResult) Graph {
			s.markStep(core.ProcessingStepObservedLayer, core.StageStateRunning)
			graph := layers.Observed(result)
			s.markStep(core.ProcessingStepObservedLayer, core.StageStateCompleted)
			return graph
		},
		WithCandidates: func(observed Graph, result ImportResult, selection MatchConditionSelection) Graph {
			s.markStep(core.ProcessingStepCandidateEdges, core.StageStateRunning)
			graph := layers.WithCandidates(observed, result, selection)
			s.markStep(core.ProcessingStepCandidateEdges, core.StageStateCompleted)
			return graph
		},
	}
}

// markStep は、処理の段階が実行中のときだけ手順の状態を書く。処理を終えた後の組み直し
// (割当の記録で生じる組み直し) を段階の手順として数えない。
func (s *Stages) markStep(step core.ProcessingStep, state core.StageState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processing.state != core.StageStateRunning {
		return
	}
	s.processing.steps[step] = state
}

// Loaded は、読み込みを終えた保存先を返す。ok が偽になるのは、読み込みを終えていないときである。
func (s *Stages) Loaded() (InvestigationStore, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store, s.store != nil
}

// Processed は、処理を終えた Session を返す。ok が偽になるのは、処理を終えていないときである。
func (s *Stages) Processed() (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return Session{}, false
	}
	return *s.session, true
}

// Wait は実行中の段階の終わりを待つ。停止の合図の後に呼ぶと、以後の段階は始まらない。
//
// **mu を 1 度取ってから待つ。** 停止の合図の前に確かめを通った begin* が Add を終えるまで
// 待つためである。mu を取った後の begin* は停止の合図を見て始めない。
func (s *Stages) Wait() {
	s.mu.Lock()
	s.mu.Unlock() //nolint:staticcheck // 空の区間で、確かめを通った begin* の Add を待つ。
	s.running.Wait()
}

// Snapshot は段階の状態と進行を返す。
func (s *Stages) Snapshot() core.InvestigationStages {
	s.mu.Lock()
	defer s.mu.Unlock()
	stages := core.InvestigationStages{
		Loading: core.LoadingStage{
			State: s.loading.state, Sources: make([]core.LoadingSource, 0, len(s.loading.sources)),
			Failure: clonePointer(s.loading.failure),
		},
		Processing: core.ProcessingStage{
			State:   s.processing.state,
			Steps:   make([]core.ProcessingStepProgress, 0, len(core.ProcessingSteps())),
			Failure: clonePointer(s.processing.failure),
		},
		FormatKeys:              slices.Sorted(maps.Keys(s.config.Import.Parsers)),
		AcceptsRequestedLoading: s.config.Requested != nil,
	}
	for _, source := range s.loading.sources {
		loaded := core.LoadingSource{
			OriginPath: source.plan.OriginPath, FormatKey: source.plan.FormatKey,
			ReadBytes: source.readBytes.Load(), SizeBytes: clonePointer(source.sizeBytes),
		}
		if source.status != nil {
			status := *source.status
			loaded.Status = &status
		}
		stages.Loading.Sources = append(stages.Loading.Sources, loaded)
	}
	for _, step := range core.ProcessingSteps() {
		stages.Processing.Steps = append(stages.Processing.Steps,
			core.ProcessingStepProgress{Step: step, State: s.processing.steps[step]})
	}
	return stages
}
