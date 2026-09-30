package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// processingSteps は、全手順が同じ状態の手順の並びを返す。
func processingSteps(state core.StageState) []core.ProcessingStepProgress {
	steps := make([]core.ProcessingStepProgress, 0, len(core.ProcessingSteps()))
	for _, step := range core.ProcessingSteps() {
		steps = append(steps, core.ProcessingStepProgress{Step: step, State: state})
	}
	return steps
}

// loadedSource は読み込みを終えた収集元 1 件である。
func loadedSource() core.LoadingSource {
	size := int64(120)
	return core.LoadingSource{
		OriginPath: "logs/a.log", FormatKey: "linux_auditd", ReadBytes: size, SizeBytes: &size,
		Status: &core.LoadedSourceStatus{
			SourceId: "source-a", PublicationState: core.PublicationStatePublishedFull,
			DiagnosisCounts: []core.DiagnosisCount{},
		},
	}
}

func completedStages() core.InvestigationStages {
	return core.InvestigationStages{
		Loading: core.LoadingStage{
			State: core.StageStateCompleted, Sources: []core.LoadingSource{loadedSource()},
		},
		Processing: core.ProcessingStage{
			State: core.StageStateCompleted, Steps: processingSteps(core.StageStateCompleted),
		},
		FormatKeys: []core.FormatKey{"linux_auditd"},
	}
}

func TestInvestigationStagesAcceptsEveryStageOfTheStateMachine(t *testing.T) {
	notStarted := core.InvestigationStages{
		Loading: core.LoadingStage{State: core.StageStateNotStarted, Sources: []core.LoadingSource{}},
		Processing: core.ProcessingStage{
			State: core.StageStateNotStarted, Steps: processingSteps(core.StageStateNotStarted),
		},
	}
	loading := notStarted
	loading.Loading = core.LoadingStage{
		State:   core.StageStateRunning,
		Sources: []core.LoadingSource{{OriginPath: "logs/a.log", FormatKey: "linux_auditd"}},
	}
	failedProcessing := completedStages()
	failedProcessing.Processing = core.ProcessingStage{
		State: core.StageStateFailed, Steps: processingSteps(core.StageStateFailed),
		Failure: &core.StageFailure{Reason: core.StageFailureReasonGraphBuildFailed},
	}
	failedLoading := notStarted
	failedLoading.Loading = core.LoadingStage{
		State:   core.StageStateFailed,
		Sources: []core.LoadingSource{{OriginPath: "logs/a.log", FormatKey: "linux_auditd"}},
		Failure: &core.StageFailure{
			Reason: core.StageFailureReasonSourceImportFailed, OriginPath: "logs/a.log",
		},
	}
	interruptedLoading := failedLoading
	interruptedLoading.Loading.Failure = &core.StageFailure{Reason: core.StageFailureReasonInterrupted}
	for name, stages := range map[string]core.InvestigationStages{
		"未開始": notStarted, "読み込み中": loading, "完了": completedStages(), "処理の失敗": failedProcessing,
		"収集元 1 件の読み込みの失敗": failedLoading, "停止で打ち切った読み込み": interruptedLoading,
	} {
		t.Run(name, func(t *testing.T) {
			if err := stages.Validate(); err != nil {
				t.Errorf("Validate rejected the stages: %v", err)
			}
		})
	}
}

func TestInvestigationStagesRejectsAnInconsistentState(t *testing.T) {
	for name, broken := range map[string]struct {
		change func(*core.InvestigationStages)
		want   error
	}{
		"失敗した段階が理由を持たない": {func(s *core.InvestigationStages) {
			s.Processing.State = core.StageStateFailed
		}, core.ErrInconsistentValue},
		"失敗していない段階が理由を持つ": {func(s *core.InvestigationStages) {
			s.Loading.Failure = &core.StageFailure{Reason: core.StageFailureReasonImportFailed}
		}, core.ErrInconsistentValue},
		"読み込みの失敗が処理の理由を持つ": {func(s *core.InvestigationStages) {
			failLoading(s, core.StageFailure{Reason: core.StageFailureReasonGraphBuildFailed})
		}, core.ErrInconsistentValue},
		"処理の失敗が読み込みの理由を持つ": {func(s *core.InvestigationStages) {
			s.Processing.State = core.StageStateFailed
			s.Processing.Steps = processingSteps(core.StageStateFailed)
			s.Processing.Failure = &core.StageFailure{Reason: core.StageFailureReasonRecordingFailed}
		}, core.ErrInconsistentValue},
		"収集元 1 件の失敗が収集元を指さない": {func(s *core.InvestigationStages) {
			failLoading(s, core.StageFailure{Reason: core.StageFailureReasonSourceImportFailed})
		}, core.ErrInconsistentValue},
		"収集元を指さない失敗が収集元を持つ": {func(s *core.InvestigationStages) {
			failLoading(s, core.StageFailure{
				Reason: core.StageFailureReasonRecordingFailed, OriginPath: "logs/a.log",
			})
		}, core.ErrInconsistentValue},
		"失敗した収集元の path が制御文字を持つ": {func(s *core.InvestigationStages) {
			failLoading(s, core.StageFailure{
				Reason: core.StageFailureReasonSourceImportFailed, OriginPath: "logs/a\nb.log",
			})
		}, core.ErrControlCharacter},
		"定義に無い失敗の理由": {func(s *core.InvestigationStages) {
			failLoading(s, core.StageFailure{Reason: "disk_full"})
		}, core.ErrUnknownEnumValue},
		"完了した読み込みの収集元が状態を持たない": {func(s *core.InvestigationStages) {
			s.Loading.Sources[0].Status = nil
		}, core.ErrMissingRequiredItem},
		"未開始の読み込みが収集元を持つ": {func(s *core.InvestigationStages) {
			s.Loading.State = core.StageStateNotStarted
			s.Processing = core.ProcessingStage{
				State: core.StageStateNotStarted, Steps: processingSteps(core.StageStateNotStarted),
			}
		}, core.ErrUnexpectedItem},
		"読み込みを終える前に処理が始まる": {func(s *core.InvestigationStages) {
			s.Loading.State = core.StageStateRunning
		}, core.ErrInconsistentValue},
		"手順の並びが異なる": {func(s *core.InvestigationStages) {
			s.Processing.Steps[0], s.Processing.Steps[1] = s.Processing.Steps[1], s.Processing.Steps[0]
		}, core.ErrInconsistentValue},
		"完了した処理に終えていない手順がある": {func(s *core.InvestigationStages) {
			s.Processing.Steps[1].State = core.StageStateRunning
		}, core.ErrInconsistentValue},
		"定義に無い状態": {func(s *core.InvestigationStages) {
			s.Loading.State = "paused"
		}, core.ErrUnknownEnumValue},
		"読んだ byte 数が負": {func(s *core.InvestigationStages) {
			s.Loading.Sources[0].ReadBytes = -1
		}, core.ErrInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			stages := completedStages()
			broken.change(&stages)
			if err := stages.Validate(); !errors.Is(err, broken.want) {
				t.Errorf("error = %v, want one wrapping %v", err, broken.want)
			}
		})
	}
}

// failLoading は読み込みの段階を、処理を始めていない失敗の状態にする。
func failLoading(s *core.InvestigationStages, failure core.StageFailure) {
	s.Loading.State = core.StageStateFailed
	s.Loading.Failure = &failure
	s.Processing = core.ProcessingStage{
		State: core.StageStateNotStarted, Steps: processingSteps(core.StageStateNotStarted),
	}
}

func TestLoadedSourceStatusOfCarriesTheImportStatusSummary(t *testing.T) {
	counts, err := core.NewImportCountSet(
		core.ImportCount{Category: core.ImportCategoryRead, Count: 5},
		core.ImportCount{Category: core.ImportCategorySucceeded, Count: 2},
		core.ImportCount{Category: core.ImportCategoryFailed, Count: 3},
	)
	if err != nil {
		t.Fatal(err)
	}
	status := core.ImportStatus{
		SourceId: "source-a", PublicationState: core.PublicationStatePublishedPartial, Counts: counts,
		DiagnosisCounts: []core.DiagnosisCount{{DiagnosisClass: core.DiagnosisClassUndetermined, Count: 3}},
		FailureCount:    3,
	}
	summary := core.LoadedSourceStatusOf(status)
	if summary.SourceId != status.SourceId || summary.PublicationState != status.PublicationState ||
		summary.FailureCount != status.FailureCount {
		t.Errorf("the summary is %+v, want the items of %+v", summary, status)
	}
	for _, category := range []core.ImportCategory{
		core.ImportCategoryRead, core.ImportCategorySucceeded, core.ImportCategoryFailed,
	} {
		got, gotOk := summary.Counts.Count(category)
		want, wantOk := status.Counts.Count(category)
		if got != want || gotOk != wantOk {
			t.Errorf("the %s count is %d (%t), want %d (%t)", category, got, gotOk, want, wantOk)
		}
	}
	if len(summary.DiagnosisCounts) != len(status.DiagnosisCounts) ||
		summary.DiagnosisCounts[0] != status.DiagnosisCounts[0] {
		t.Fatalf("the diagnosis counts are %+v, want %+v", summary.DiagnosisCounts, status.DiagnosisCounts)
	}
	// 要約は取り込みの状態と別の複製を持つ。
	summary.DiagnosisCounts[0].Count = 0
	if status.DiagnosisCounts[0].Count != 3 {
		t.Error("changing the summary changed the import status")
	}
}

func TestApiErrorCarriesALoadingRejectionOnlyForAnInvalidRequest(t *testing.T) {
	rejected := core.ApiError{
		Code: core.ApiErrorCodeInvalidRequest, Message: "x",
		LoadingRejection: core.LoadingRejectionFileAbsent, OriginPath: "logs/a.log",
	}
	if err := rejected.Validate(); err != nil {
		t.Errorf("Validate rejected the loading rejection: %v", err)
	}
	for name, broken := range map[string]struct {
		change func(*core.ApiError)
		want   error
	}{
		"invalid_request でない失敗が理由を持つ": {func(e *core.ApiError) {
			e.Code = core.ApiErrorCodeInternalError
		}, core.ErrUnexpectedItem},
		"定義に無い理由": {func(e *core.ApiError) {
			e.LoadingRejection = "disk_full"
		}, core.ErrUnknownEnumValue},
	} {
		t.Run(name, func(t *testing.T) {
			apiError := rejected
			broken.change(&apiError)
			if err := apiError.Validate(); !errors.Is(err, broken.want) {
				t.Errorf("error = %v, want one wrapping %v", err, broken.want)
			}
		})
	}
}
