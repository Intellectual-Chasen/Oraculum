package core

// StageState は調査の段階 1 つの状態を持つ。
type StageState string

// StageState の値。
const (
	// StageStateNotStarted は段階をまだ始めていない状態である。
	StageStateNotStarted StageState = "not_started"
	// StageStateRunning は段階を実行している状態である。
	StageStateRunning StageState = "running"
	// StageStateCompleted は段階を終えた状態である。完了した段階をもう一度始めない。
	StageStateCompleted StageState = "completed"
	// StageStateFailed は段階が失敗した状態である。同じ段階を始め直せる。
	StageStateFailed StageState = "failed"
)

// IsKnown は StageState が定義の中の値であるかを返す。
func (s StageState) IsKnown() bool {
	switch s {
	case StageStateNotStarted, StageStateRunning, StageStateCompleted, StageStateFailed:
		return true
	default:
		return false
	}
}

// ProcessingStep は処理の段階の中の手順である。
type ProcessingStep string

// ProcessingStep の値。並びは ProcessingSteps が返す実行の順である。
const (
	// ProcessingStepObservedLayer は、レコードから直接作るノードとエッジの層を組む手順である。
	ProcessingStepObservedLayer ProcessingStep = "observed_layer"
	// ProcessingStepCandidateEdges は、収集元をまたいだ関連付けの候補のエッジを足す手順である。
	ProcessingStepCandidateEdges ProcessingStep = "candidate_edges"
)

// ProcessingSteps は処理の手順を実行の順で返す。
func ProcessingSteps() []ProcessingStep {
	return []ProcessingStep{ProcessingStepObservedLayer, ProcessingStepCandidateEdges}
}

// IsKnown は ProcessingStep が定義の中の値であるかを返す。
func (s ProcessingStep) IsKnown() bool {
	for _, known := range ProcessingSteps() {
		if s == known {
			return true
		}
	}
	return false
}

// InvestigationStages は、収集元の読み込みと処理の 2 つの段階の状態と進行を持つ。
// **本型が項目の定義元である。**
type InvestigationStages struct {
	// Loading は収集元の読み込みの段階である。
	Loading LoadingStage `json:"loading"`
	// Processing は処理の段階である。
	Processing ProcessingStage `json:"processing"`
	// FormatKeys は server が読める入力形式である。読み込みを始める要求の選択肢になる。
	FormatKeys []FormatKey `json:"formatKeys"`
	// AcceptsRequestedLoading は、server が要求による読み込みを受け付けるかである。
	// 偽の server は、読み込む file の基準の directory を持たない。
	AcceptsRequestedLoading bool `json:"acceptsRequestedLoading"`
}

// Validate は段階ごとの整合を確かめる。
func (s InvestigationStages) Validate() error {
	if problem := s.Loading.Validate(); problem != nil {
		return itemError("InvestigationStages.loading", problem)
	}
	if problem := s.Processing.Validate(); problem != nil {
		return itemError("InvestigationStages.processing", problem)
	}
	// 処理は読み込みを終えた後にだけ始まる。
	if s.Processing.State != StageStateNotStarted && s.Loading.State != StageStateCompleted {
		return itemError("InvestigationStages.processing started before the loading completed",
			ErrInconsistentValue)
	}
	for index, key := range s.FormatKeys {
		if problem := requirePresent("InvestigationStages.formatKeys at "+formatIndex(index),
			string(key)); problem != nil {
			return problem
		}
	}
	return nil
}

// LoadingStage は収集元の読み込みの段階の状態と、収集元ごとの進行である。
type LoadingStage struct {
	State StageState `json:"state"`
	// Sources は読み込む収集元を、読む順に並べる。段階を始めていないとき要素数 0 である。
	Sources []LoadingSource `json:"sources"`
	// Failure は段階が失敗した理由である。State が failed のときだけ出る。
	Failure *StageFailure `json:"failure,omitempty"`
}

// Validate は状態と項目の対応を確かめる。
func (s LoadingStage) Validate() error {
	if problem := validateStageFailure("LoadingStage", s.State, s.Failure,
		StageFailureReason.IsLoadingReason); problem != nil {
		return problem
	}
	if s.State == StageStateNotStarted && len(s.Sources) != 0 {
		return itemError("LoadingStage.sources before the stage started", ErrUnexpectedItem)
	}
	for index, source := range s.Sources {
		if problem := source.Validate(); problem != nil {
			return itemError("LoadingStage.sources at "+formatIndex(index), problem)
		}
		// 段階を終えた読み込みは、全収集元の取り込みの状態を持つ。
		if s.State == StageStateCompleted && source.Status == nil {
			return itemError("LoadingStage.sources at "+formatIndex(index)+
				" carries no status in a completed stage", ErrMissingRequiredItem)
		}
	}
	return nil
}

// LoadingSource は読み込む収集元 1 件の進行である。
type LoadingSource struct {
	// OriginPath は収集元の取得元である。基準の directory からの相対 path である。
	OriginPath string    `json:"originPath"`
	FormatKey  FormatKey `json:"formatKey"`
	// ReadBytes は読み終えた byte 数である。
	ReadBytes int64 `json:"readBytes"`
	// SizeBytes は収集元の byte 数である。読み込みを始める前に大きさを確かめられなかった
	// 収集元では出ない。
	SizeBytes *int64 `json:"sizeBytes,omitempty"`
	// Status は収集元の取り込みの状態である。読み込みの段階を終えるまで出ない。
	Status *LoadedSourceStatus `json:"status,omitempty"`
}

// Validate は項目の整合を確かめる。
func (s LoadingSource) Validate() error {
	problem := firstProblem(
		requirePresent("LoadingSource.originPath", s.OriginPath),
		requirePresent("LoadingSource.formatKey", string(s.FormatKey)),
		requireSanitized("LoadingSource.originPath", s.OriginPath),
	)
	if problem != nil {
		return problem
	}
	if s.ReadBytes < 0 {
		return itemError("LoadingSource.readBytes", ErrInvalid)
	}
	if s.SizeBytes != nil && *s.SizeBytes < 0 {
		return itemError("LoadingSource.sizeBytes", ErrInvalid)
	}
	if s.Status != nil {
		if problem := s.Status.Validate(); problem != nil {
			return itemError("LoadingSource.status", problem)
		}
	}
	return nil
}

// LoadedSourceStatus は読み込んだ収集元 1 件の取り込みの状態の要約である。
//
// **失敗の一覧を持たない。** 進行を繰り返し読む応答に失敗の全件を載せない。失敗の一覧と
// 位置は `/api/v0/sources` の ImportStatus が持つ。
type LoadedSourceStatus struct {
	SourceId         string           `json:"sourceId"`
	PublicationState PublicationState `json:"publicationState"`
	Counts           ImportCountSet   `json:"counts"`
	DiagnosisCounts  []DiagnosisCount `json:"diagnosisCounts"`
	FailureCount     int64            `json:"failureCount"`
}

// Validate は項目の整合を確かめる。
func (s LoadedSourceStatus) Validate() error {
	problem := firstProblem(
		requirePresent("LoadedSourceStatus.sourceId", s.SourceId),
		requireKnownEnum("LoadedSourceStatus.publicationState", s.PublicationState),
	)
	if problem != nil {
		return problem
	}
	if problem := s.Counts.Validate(); problem != nil {
		return itemError("LoadedSourceStatus.counts", problem)
	}
	if s.FailureCount < 0 {
		return itemError("LoadedSourceStatus.failureCount", ErrInvalid)
	}
	return validateDiagnosisCounts(s.DiagnosisCounts)
}

// LoadedSourceStatusOf は取り込みの状態から要約を組む。
func LoadedSourceStatusOf(status ImportStatus) LoadedSourceStatus {
	diagnosis := make([]DiagnosisCount, len(status.DiagnosisCounts))
	copy(diagnosis, status.DiagnosisCounts)
	return LoadedSourceStatus{
		SourceId: status.SourceId, PublicationState: status.PublicationState,
		Counts: status.Counts, DiagnosisCounts: diagnosis, FailureCount: status.FailureCount,
	}
}

// ProcessingStage は処理の段階の状態と、手順ごとの状態である。
type ProcessingStage struct {
	State StageState `json:"state"`
	// Steps は処理の手順を ProcessingSteps の順で、全手順を並べる。
	Steps []ProcessingStepProgress `json:"steps"`
	// Failure は段階が失敗した理由である。State が failed のときだけ出る。
	Failure *StageFailure `json:"failure,omitempty"`
}

// Validate は状態と手順の対応を確かめる。
func (s ProcessingStage) Validate() error {
	if problem := validateStageFailure("ProcessingStage", s.State, s.Failure,
		StageFailureReason.IsProcessingReason); problem != nil {
		return problem
	}
	steps := ProcessingSteps()
	if len(s.Steps) != len(steps) {
		return itemError("ProcessingStage.steps differs from the processing steps", ErrInconsistentValue)
	}
	for index, step := range s.Steps {
		if step.Step != steps[index] {
			return itemError("ProcessingStage.steps at "+formatIndex(index)+" is out of order",
				ErrInconsistentValue)
		}
		if problem := requireKnownEnum("ProcessingStage.steps.state", step.State); problem != nil {
			return problem
		}
		// 段階を終えた処理は、全手順を終えている。
		if s.State == StageStateCompleted && step.State != StageStateCompleted {
			return itemError("ProcessingStage.steps at "+formatIndex(index)+
				" is not completed in a completed stage", ErrInconsistentValue)
		}
	}
	return nil
}

// ProcessingStepProgress は処理の手順 1 つの状態である。
type ProcessingStepProgress struct {
	Step  ProcessingStep `json:"step"`
	State StageState     `json:"state"`
}

// StageFailure は段階が失敗した理由である。
//
// **理由を種別と要求の相対 path だけで持つ。** 元の error は server の上の保存 path を持つため、
// 運用者の log にだけ出す。
type StageFailure struct {
	Reason StageFailureReason `json:"reason"`
	// OriginPath は失敗した収集元の、読み込みの計画が指した path である。Reason が
	// source_import_failed のとき必須で、それ以外の Reason では出ない。
	OriginPath string `json:"originPath,omitempty"`
}

// validateStageFailure は、失敗した段階だけが理由を持ち、理由がその段階の使う種別であることを
// 確かめる。
func validateStageFailure(
	item string, state StageState, failure *StageFailure, usedBy func(StageFailureReason) bool,
) error {
	if problem := requireKnownEnum(item+".state", state); problem != nil {
		return problem
	}
	if (state == StageStateFailed) != (failure != nil) {
		return itemError(item+".failure does not match the state", ErrInconsistentValue)
	}
	if failure == nil {
		return nil
	}
	if problem := requireKnownEnum(item+".failure.reason", failure.Reason); problem != nil {
		return problem
	}
	if !usedBy(failure.Reason) {
		return itemError(item+".failure.reason is not a reason of the stage", ErrInconsistentValue)
	}
	if (failure.Reason == StageFailureReasonSourceImportFailed) != (failure.OriginPath != "") {
		return itemError(item+".failure.originPath does not match the reason", ErrInconsistentValue)
	}
	return requireSanitized(item+".failure.originPath", failure.OriginPath)
}
