package pipeline_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// stagesFixture は run の fixture を基準の directory として読む段階の組を返す。
//
// 要求による読み込みの口は、基準の外を指す path の検査を cmd が持つため、ここでは fixture の
// directory の下の file を開くだけにする。
func stagesFixture(t *testing.T, layers pipeline.GraphLayers) (*pipeline.Stages, []pipeline.SourcePlan) {
	t.Helper()
	return stagesFixtureIn(t, context.Background(), layers)
}

// stagesFixtureIn は、ctx を server の寿命とする stagesFixture である。
func stagesFixtureIn(
	t *testing.T, ctx context.Context, layers pipeline.GraphLayers,
) (*pipeline.Stages, []pipeline.SourcePlan) {
	t.Helper()
	return stagesFixtureWith(t, ctx, layers, openRunFixture)
}

// openRunFixture は run の fixture の directory の下の file を開く。
func openRunFixture(name string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(runFixtureDir, name)) // #nosec G304 -- test の fixture を開く。
}

// stagesFixtureWith は、open で収集元を開く stagesFixtureIn である。
func stagesFixtureWith(
	t *testing.T, ctx context.Context, layers pipeline.GraphLayers, openUnder func(string) (io.ReadCloser, error),
) (*pipeline.Stages, []pipeline.SourcePlan) {
	t.Helper()
	fixtures := runFixtures(t)
	plans := make([]pipeline.SourcePlan, len(fixtures))
	for i, fixture := range fixtures {
		plans[i] = pipeline.SourcePlan{OriginPath: fixture.File, FileName: fixture.File, FormatKey: fixture.Format}
	}
	config := runnerConfig("")
	config.Open = openUnder
	stages, err := pipeline.NewStages(ctx, pipeline.StagesConfig{
		Import: config,
		Requested: &pipeline.RequestedFiles{
			Open: openUnder,
			Size: func(name string) (int64, error) {
				info, err := os.Stat(filepath.Join(runFixtureDir, name))
				if err != nil {
					return 0, err
				}
				return info.Size(), nil
			},
		},
		Recorder: pipeline.NewMemoryRecorder(), Layers: layers, Clock: fixedClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stages, plans
}

// fixedClock は分析者の値に同じ時刻を与える。
type fixedClock struct{}

func (fixedClock) Now() core.AssertionTime { return "2026-01-02T03:04:05.000Z" }

// requireValidSnapshot は段階の状態を検査して返す。
func requireValidSnapshot(t *testing.T, stages *pipeline.Stages) core.InvestigationStages {
	t.Helper()
	snapshot := stages.Snapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("the snapshot does not validate: %v (%+v)", err, snapshot)
	}
	return snapshot
}

func TestStagesStartNeitherStage(t *testing.T) {
	stages, _ := stagesFixture(t, pipeline.DefaultGraphLayers())
	snapshot := requireValidSnapshot(t, stages)
	if snapshot.Loading.State != core.StageStateNotStarted ||
		snapshot.Processing.State != core.StageStateNotStarted {
		t.Errorf("the stages are %q and %q, want both not started",
			snapshot.Loading.State, snapshot.Processing.State)
	}
	if !snapshot.AcceptsRequestedLoading || len(snapshot.FormatKeys) == 0 {
		t.Errorf("the snapshot is %+v, want the requested loading and the format keys", snapshot)
	}
	if err := stages.StartProcessing(); !errors.Is(err, pipeline.ErrStageNotReady) {
		t.Errorf("processing before loading returned %v, want %v", err, pipeline.ErrStageNotReady)
	}
	if _, loaded := stages.Loaded(); loaded {
		t.Error("the stages hold a store before loading")
	}
}

func TestStagesLoadAndProcessInOrder(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	if err := stages.StartRequestedLoading(plans); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	snapshot := requireValidSnapshot(t, stages)
	if snapshot.Loading.State != core.StageStateCompleted {
		t.Fatalf("the loading is %q (%+v), want completed", snapshot.Loading.State, snapshot.Loading.Failure)
	}
	for i, fixture := range runFixtures(t) {
		source := snapshot.Loading.Sources[i]
		if source.OriginPath != fixture.File || source.ReadBytes != fixture.Size ||
			source.SizeBytes == nil || *source.SizeBytes != fixture.Size {
			t.Errorf("the source %d is %+v, want %q read to its size %d", i, source, fixture.File, fixture.Size)
		}
		if source.Status.PublicationState != fixture.State {
			t.Errorf("the source %q is %q, want %q", fixture.File, source.Status.PublicationState, fixture.State)
		}
	}
	if err := stages.StartRequestedLoading(plans); !errors.Is(err, pipeline.ErrStageAlreadyStarted) {
		t.Errorf("a second loading returned %v, want %v", err, pipeline.ErrStageAlreadyStarted)
	}

	if err := stages.Process(); err != nil {
		t.Fatal(err)
	}
	snapshot = requireValidSnapshot(t, stages)
	if snapshot.Processing.State != core.StageStateCompleted {
		t.Fatalf("the processing is %q, want completed", snapshot.Processing.State)
	}
	session, processed := stages.Processed()
	if !processed || session.Catalog == nil || session.Store == nil {
		t.Fatalf("the processed session is %+v (%v)", session, processed)
	}
	if err := stages.StartProcessing(); !errors.Is(err, pipeline.ErrStageAlreadyStarted) {
		t.Errorf("a second processing returned %v, want %v", err, pipeline.ErrStageAlreadyStarted)
	}
}

// 失敗した読み込みはやり直せる。やり直した読み込みは、失敗を挟まない読み込みと同じ sourceId を発行する。
func TestStagesRetryAFailedLoading(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	fresh, _ := stagesFixture(t, pipeline.DefaultGraphLayers())
	broken := append([]pipeline.SourcePlan{}, plans...)
	// 大きさを確かめた後に読めなくなる形は作れないため、同期の読み込みで開けない file を渡す。
	broken[len(broken)-1].OriginPath = "missing.log"
	if err := stages.Load(broken, nil); err == nil {
		t.Fatal("loading a missing file succeeded")
	}
	snapshot := requireValidSnapshot(t, stages)
	want := core.StageFailure{Reason: core.StageFailureReasonSourceImportFailed, OriginPath: "missing.log"}
	if snapshot.Loading.State != core.StageStateFailed || snapshot.Loading.Failure == nil ||
		*snapshot.Loading.Failure != want {
		t.Fatalf("the loading is %+v, want failed with %+v", snapshot.Loading, want)
	}
	if err := stages.StartProcessing(); !errors.Is(err, pipeline.ErrStageNotReady) {
		t.Errorf("processing after a failed loading returned %v, want %v", err, pipeline.ErrStageNotReady)
	}

	if err := stages.Load(plans, nil); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Load(plans, nil); err != nil {
		t.Fatal(err)
	}
	retried, _ := stages.Loaded()
	direct, _ := fresh.Loaded()
	retriedStatuses, directStatuses := retried.ImportResult().Statuses(), direct.ImportResult().Statuses()
	if len(retriedStatuses) != len(directStatuses) || len(retriedStatuses) != len(plans) {
		t.Fatalf("the retried loading holds %d sources and the direct one %d, want %d each",
			len(retriedStatuses), len(directStatuses), len(plans))
	}
	for i, status := range retriedStatuses {
		if want := directStatuses[i].SourceId; status.SourceId != want {
			t.Errorf("the retried source %d is %q, want %q", i, status.SourceId, want)
		}
	}
}

// 読み込みに続けて処理する要求は、読み込みを終えると処理まで終える。読み込みが失敗したときは
// 処理を始めない。
func TestStagesProcessAfterTheRequestedLoading(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	if err := stages.StartRequestedLoadingThenProcessing(plans); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	if snapshot := requireValidSnapshot(t, stages); snapshot.Processing.State != core.StageStateCompleted {
		t.Errorf("the processing is %+v, want completed", snapshot.Processing)
	}

	unopenable := errors.New("synthetic open failure")
	failing, plans := stagesFixtureWith(t, context.Background(), pipeline.DefaultGraphLayers(),
		func(name string) (io.ReadCloser, error) {
			if name == plans[len(plans)-1].OriginPath {
				return nil, unopenable
			}
			return openRunFixture(name)
		})
	if err := failing.StartRequestedLoadingThenProcessing(plans); err != nil {
		t.Fatal(err)
	}
	failing.Wait()
	snapshot := requireValidSnapshot(t, failing)
	if snapshot.Loading.State != core.StageStateFailed || snapshot.Processing.State != core.StageStateNotStarted {
		t.Errorf("the stages are %q and %q, want a failed loading and no processing",
			snapshot.Loading.State, snapshot.Processing.State)
	}
}

// 読み込みに続けて処理する要求では、読み込みの完了を読める状態は、処理を実行中として持つ。
// 状態を取り直す画面が、実行中の段階を持たない状態を読んで取り直しを止めない。
func TestStagesShowTheFollowingProcessingWithTheCompletedLoading(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseLayer := func() { releaseOnce.Do(func() { close(release) }) }
	layers := pipeline.DefaultGraphLayers()
	observed := layers.Observed
	layers.Observed = func(result pipeline.ImportResult) pipeline.Graph {
		<-release
		return observed(result)
	}
	stages, plans := stagesFixture(t, layers)
	// 途中で test が止まっても、止めた層の goroutine を残さない。fixture の後に登録し、先に動かす。
	t.Cleanup(releaseLayer)
	if err := stages.StartRequestedLoadingThenProcessing(plans); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		snapshot := requireValidSnapshot(t, stages)
		if snapshot.Loading.State == core.StageStateCompleted {
			if snapshot.Processing.State != core.StageStateRunning {
				t.Errorf("the completed loading shows the processing %q, want running", snapshot.Processing.State)
			}
			break
		}
		if snapshot.Loading.State == core.StageStateFailed || time.Now().After(deadline) {
			t.Fatalf("the loading is %+v, want completed", snapshot.Loading)
		}
		time.Sleep(time.Millisecond)
	}
	releaseLayer()
	stages.Wait()
	if snapshot := requireValidSnapshot(t, stages); snapshot.Processing.State != core.StageStateCompleted {
		t.Errorf("the processing is %+v, want completed", snapshot.Processing)
	}
}

// 失敗した処理はやり直せる。失敗した処理の手順は失敗として残る。
func TestStagesRetryAFailedProcessing(t *testing.T) {
	var failOnce sync.Once
	layers := pipeline.DefaultGraphLayers()
	observed := layers.Observed
	layers.Observed = func(result pipeline.ImportResult) pipeline.Graph {
		failOnce.Do(func() { panic("the test layer panics once") })
		return observed(result)
	}
	stages, plans := stagesFixture(t, layers)
	if err := stages.Load(plans, nil); err != nil {
		t.Fatal(err)
	}
	if err := stages.Process(); err == nil {
		t.Fatal("the first processing succeeded, want the panic as a failure")
	}
	snapshot := requireValidSnapshot(t, stages)
	if snapshot.Processing.State != core.StageStateFailed || snapshot.Processing.Steps[0].State != core.StageStateFailed {
		t.Fatalf("the processing is %+v, want failed at the observed layer", snapshot.Processing)
	}
	if failure := snapshot.Processing.Failure; failure == nil || failure.Reason != core.StageFailureReasonGraphBuildFailed {
		t.Errorf("the processing failed with %+v, want %q", failure, core.StageFailureReasonGraphBuildFailed)
	}
	if _, processed := stages.Processed(); processed {
		t.Error("a failed processing published its session")
	}
	if err := stages.Process(); err != nil {
		t.Fatal(err)
	}
	if _, processed := stages.Processed(); !processed {
		t.Error("the retried processing did not publish its session")
	}
}

// 同時に始めた読み込みは、片方だけを受け付ける。
func TestStagesAcceptOneOfTwoConcurrentLoadings(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- stages.StartRequestedLoading(plans) }()
	}
	var accepted, rejected int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			accepted++
		case errors.Is(err, pipeline.ErrStageAlreadyStarted):
			rejected++
		default:
			t.Errorf("a loading returned %v", err)
		}
	}
	stages.Wait()
	if accepted != 1 || rejected != 1 {
		t.Errorf("accepted %d and rejected %d loadings, want one of each", accepted, rejected)
	}
}

// 読み込みを始められない要求を、理由の種別と退けた収集元の path で退け、段階の状態を変えない。
func TestStagesRejectAnUnusableLoadingRequest(t *testing.T) {
	spec := "%h %un"
	type rejection struct {
		want core.LoadingRejection
		// originPath は退けた収集元の path の原文である。要求全体を退けるときは空文字列である。
		originPath string
	}
	for name, request := range map[string]struct {
		change func([]pipeline.SourcePlan) []pipeline.SourcePlan
		rejection
	}{
		"収集元が無い": {func([]pipeline.SourcePlan) []pipeline.SourcePlan { return nil },
			rejection{want: core.LoadingRejectionNoSource}},
		"絶対 path": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = "/etc/passwd"
			return p
		}, rejection{core.LoadingRejectionPathOutsideBase, "/etc/passwd"}},
		// POSIX の filepath.IsLocal は、`\` とドライブ文字で始まる path を相対 path と数える。
		"`\\` で始まる path": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = `\run\markii.log`
			return p
		}, rejection{core.LoadingRejectionPathOutsideBase, `\run\markii.log`}},
		"ドライブ文字で始まる path": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = "C:/run/markii.log"
			return p
		}, rejection{core.LoadingRejectionPathOutsideBase, "C:/run/markii.log"}},
		"基準の外へ出る path": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = "../run/" + p[0].OriginPath
			return p
		}, rejection{core.LoadingRejectionPathOutsideBase, "../run/markii.log"}},
		"制御文字を持つ path": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = "a\nb.log"
			return p
		}, rejection{core.LoadingRejectionPathControlCharacter, "a\nb.log"}},
		"読めない入力形式": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].FormatKey = "unknown_format"
			return p
		}, rejection{core.LoadingRejectionFormatUnknown, "markii.log"}},
		"並びを取らない形式への並び": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].FormatSpec = &spec
			return p
		}, rejection{core.LoadingRejectionFormatSpecInvalid, "markii.log"}},
		"案件が一部だけ": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			caseId := "baseline"
			p[0].CaseId = &caseId
			return p
		}, rejection{want: core.LoadingRejectionPartialCase}},
		"規則に合わない案件": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			caseId := "a b"
			for i := range p {
				p[i].CaseId = &caseId
			}
			return p
		}, rejection{core.LoadingRejectionCaseInvalid, "markii.log"}},
		"同じ収集元が 2 回": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			return append(p, p[0])
		}, rejection{core.LoadingRejectionSourceRepeated, "markii.log"}},
		"文字列だけが異なる同じ収集元": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			again := p[0]
			again.OriginPath = "./" + again.OriginPath
			return append(p, again)
		}, rejection{core.LoadingRejectionSourceRepeated, "./markii.log"}},
		"無い file": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].OriginPath = "missing.log"
			return p
		}, rejection{core.LoadingRejectionFileAbsent, "missing.log"}},
		"端末の項目が無い": {func(p []pipeline.SourcePlan) []pipeline.SourcePlan {
			p[0].Terminal = &pipeline.SourceTerminal{}
			return p
		}, rejection{core.LoadingRejectionTerminalInvalid, "markii.log"}},
	} {
		t.Run(name, func(t *testing.T) {
			stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
			if plans[0].OriginPath != "markii.log" {
				t.Fatalf("the first fixture is %q, want markii.log", plans[0].OriginPath)
			}
			err := stages.StartRequestedLoading(request.change(plans))
			if !errors.Is(err, pipeline.ErrLoadingRequestInvalid) {
				t.Fatalf("the request returned %v, want %v", err, pipeline.ErrLoadingRequestInvalid)
			}
			requestError, ok := errors.AsType[*pipeline.LoadingRequestError](err)
			if !ok {
				t.Fatalf("the request returned %T, want a loading request error", err)
			}
			if requestError.Rejection != request.want || requestError.OriginPath != request.originPath {
				t.Errorf("the request was rejected as %q for %q, want %q for %q",
					requestError.Rejection, requestError.OriginPath, request.want, request.originPath)
			}
			base, absErr := filepath.Abs(runFixtureDir)
			if absErr != nil {
				t.Fatal(absErr)
			}
			if strings.Contains(err.Error(), base) {
				t.Errorf("the error %q names the base directory", err)
			}
			if state := requireValidSnapshot(t, stages).Loading.State; state != core.StageStateNotStarted {
				t.Errorf("a rejected request left the loading %q", state)
			}
		})
	}
}

// 文字列の違う path は整えてから読み込み、段階の状態は整えた path で指す。
func TestStagesLoadACleanedRequestedPath(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	plans[0].OriginPath = "./" + plans[0].OriginPath
	if err := stages.StartRequestedLoading(plans); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	snapshot := requireValidSnapshot(t, stages)
	if snapshot.Loading.State != core.StageStateCompleted || snapshot.Loading.Sources[0].OriginPath != "markii.log" {
		t.Errorf("the loading is %+v, want completed with markii.log", snapshot.Loading)
	}
}

// 起動引数と記録済みの計画が制御文字を持つ path を指すときは、読み込みを始めない。
func TestStagesLoadRefusesAPathWithAControlCharacter(t *testing.T) {
	stages, plans := stagesFixture(t, pipeline.DefaultGraphLayers())
	plans[0].OriginPath = "a\tb.log"
	if err := stages.Load(plans, nil); err == nil {
		t.Fatal("loading a path with a control character succeeded")
	}
	if state := requireValidSnapshot(t, stages).Loading.State; state != core.StageStateNotStarted {
		t.Errorf("the refused loading left the loading %q", state)
	}
}

// 停止の合図を受けた後は、どの段階も始めない。
func TestStagesStartNothingAfterTheServerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stages, plans := stagesFixtureIn(t, ctx, pipeline.DefaultGraphLayers())
	if err := stages.Load(plans, nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := stages.StartProcessing(); !errors.Is(err, pipeline.ErrStagesStopped) {
		t.Errorf("processing after the stop returned %v, want %v", err, pipeline.ErrStagesStopped)
	}
	stopped, plans := stagesFixtureIn(t, ctx, pipeline.DefaultGraphLayers())
	if err := stopped.StartRequestedLoading(plans); !errors.Is(err, pipeline.ErrStagesStopped) {
		t.Errorf("loading after the stop returned %v, want %v", err, pipeline.ErrStagesStopped)
	}
	stages.Wait()
	stopped.Wait()
}

// stagesReading は、要求の file を open で開き、size で大きさを答える段階を返す。
func stagesReading(
	t *testing.T, open func(string) (io.ReadCloser, error), size func(string) (int64, error),
) *pipeline.Stages {
	t.Helper()
	config := runnerConfig("")
	config.Open = open
	stages, err := pipeline.NewStages(context.Background(), pipeline.StagesConfig{
		Import: config, Requested: &pipeline.RequestedFiles{Open: open, Size: size},
		Recorder: pipeline.NewMemoryRecorder(), Layers: pipeline.DefaultGraphLayers(), Clock: fixedClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stages
}

// requireLoadingFailure は、読み込みが失敗し、want の失敗を残したことを確かめる。
func requireLoadingFailure(t *testing.T, stages *pipeline.Stages, want core.StageFailure) {
	t.Helper()
	loading := requireValidSnapshot(t, stages).Loading
	if loading.State != core.StageStateFailed || loading.Failure == nil || *loading.Failure != want {
		t.Errorf("the loading is %q with %+v, want failed with %+v", loading.State, loading.Failure, want)
	}
}

// untimedProxySpec は、時刻の欄を持たない Squid の logformat である。
const untimedProxySpec = `%6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt`

// 端末を指定した収集元に端末を付けられない読み込みは、その収集元を指す失敗として残る。
func TestStagesPointATerminalFailureAtItsSource(t *testing.T) {
	// 時刻を持たず、観測期間が決まらない。
	const content = "    10 192.0.2.20 TCP_MISS/200 100 GET http://a.example.test/ - " +
		"HIER_DIRECT/198.51.100.9 text/html\n"
	stages := stagesReading(t,
		func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil },
		func(string) (int64, error) { return int64(len(content)), nil })
	spec := untimedProxySpec
	plans := []pipeline.SourcePlan{{
		OriginPath: "logs/local.log", FormatKey: pipeline.SquidLogFormatKey, FormatSpec: &spec,
		Terminal: &pipeline.SourceTerminal{TerminalId: "pc-01"},
	}}
	if err := stages.StartRequestedLoading(plans); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	requireLoadingFailure(t, stages,
		core.StageFailure{Reason: core.StageFailureReasonSourceImportFailed, OriginPath: "logs/local.log"})
}

// 読み込みの途中の panic は server の process を止めず、読み込みの失敗として残る。
func TestStagesKeepAPanicInTheLoadingAsItsFailure(t *testing.T) {
	stages := stagesReading(t,
		func(string) (io.ReadCloser, error) { panic("the test opener panics") },
		func(string) (int64, error) { return 1, nil })
	plans := []pipeline.SourcePlan{{OriginPath: "a.log", FormatKey: "squid_combined"}}
	if err := stages.StartRequestedLoading(plans); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	want := core.StageFailure{Reason: core.StageFailureReasonImportFailed}
	requireLoadingFailure(t, stages, want)
	// 同期の読み込みも panic を失敗として返す。
	if err := stages.Load(plans, nil); err == nil {
		t.Error("the loading that panicked succeeded")
	}
	requireLoadingFailure(t, stages, want)
}

// 読み込みの途中で停止の合図を受けると、読み取りを止め、停止で打ち切った失敗として残す。
func TestStagesInterruptTheLoadingAtTheStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// 最初の収集元を開いた時点で停止の合図を出す。
	stages, plans := stagesFixtureWith(t, ctx, pipeline.DefaultGraphLayers(), func(name string) (io.ReadCloser, error) {
		cancel()
		return openRunFixture(name)
	})
	if err := stages.Load(plans, nil); err == nil {
		t.Fatal("the loading interrupted by the stop succeeded")
	}
	failure := requireValidSnapshot(t, stages).Loading.Failure
	if failure == nil || failure.Reason != core.StageFailureReasonInterrupted {
		t.Errorf("the loading failed with %+v, want %q", failure, core.StageFailureReasonInterrupted)
	}
}
