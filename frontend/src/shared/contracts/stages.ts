import {
  DecodeFailure,
  type Decoder,
  optionalCount,
  optionalMember,
  readObject,
  rejectMember,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import {
  type DiagnosisCount,
  decodeDiagnosisCount,
  decodeImportCount,
  type FormatKey,
  type ImportCount,
  type PublicationState,
  publicationStates,
} from "./sources";

/** 調査の段階 1 つの状態。定義元は `backend/core/stage.go` の `StageState` である。 */
export const stageStates = [
  "not_started",
  "running",
  "completed",
  "failed",
] as const;
export type StageState = (typeof stageStates)[number];

/**
 * 処理の段階の中の手順を、実行の順で並べる。
 * 定義元は `backend/core/stage.go` の `ProcessingSteps` である。
 */
export const processingSteps = ["observed_layer", "candidate_edges"] as const;
export type ProcessingStep = (typeof processingSteps)[number];

/**
 * 読み込みの段階が失敗した理由の種別。
 * 定義元は `backend/core/stage_reason.go` の `StageFailureReason.IsLoadingReason` である。
 */
export const loadingFailureReasons = [
  "source_import_failed",
  "import_failed",
  "recording_failed",
  "interrupted",
] as const;

/**
 * 処理の段階が失敗した理由の種別。
 * 定義元は `backend/core/stage_reason.go` の `StageFailureReason.IsProcessingReason` である。
 */
export const processingFailureReasons = [
  "graph_build_failed",
  "interrupted",
] as const;

/** 段階が失敗した理由の種別。 */
export type StageFailureReason =
  | (typeof loadingFailureReasons)[number]
  | (typeof processingFailureReasons)[number];

/** 段階が失敗した理由。定義元は `backend/core/stage.go` の `StageFailure` である。 */
export type StageFailure = {
  reason: StageFailureReason;
  /** 失敗した収集元の path。`reason` が `source_import_failed` のときだけ出る。 */
  originPath?: string;
};

/** 読み込んだ収集元 1 件の取り込みの状態の要約。定義元は同 file の `LoadedSourceStatus` である。 */
export type LoadedSourceStatus = {
  sourceId: string;
  publicationState: PublicationState;
  counts: ImportCount[];
  diagnosisCounts: DiagnosisCount[];
  failureCount: number;
};

/** 読み込む収集元 1 件の進行。定義元は同 file の `LoadingSource` である。 */
export type LoadingSource = {
  /** 収集元の取得元。server の基準の directory からの相対 path である。 */
  originPath: string;
  formatKey: FormatKey;
  /** 読み終えた byte 数。 */
  readBytes: number;
  /** 収集元の byte 数。出ない場合は、読み込みを始める前に大きさを確かめられなかった。 */
  sizeBytes?: number;
  /** 収集元の取り込みの状態。読み込みの段階を終えるまで出ない。 */
  status?: LoadedSourceStatus;
};

/** 収集元の読み込みの段階。定義元は同 file の `LoadingStage` である。 */
export type LoadingStage = {
  state: StageState;
  /** 読み込む収集元を読む順に並べる。段階を始めていないとき要素数 0 である。 */
  sources: LoadingSource[];
  /** `state` が `failed` のときだけ出る。 */
  failure?: StageFailure;
};

/** 処理の手順 1 つの状態。定義元は同 file の `ProcessingStepProgress` である。 */
export type ProcessingStepProgress = {
  step: ProcessingStep;
  state: StageState;
};

/** 処理の段階。定義元は同 file の `ProcessingStage` である。 */
export type ProcessingStage = {
  state: StageState;
  /** 全手順を `processingSteps` の順で並べる。 */
  steps: ProcessingStepProgress[];
  /** `state` が `failed` のときだけ出る。 */
  failure?: StageFailure;
};

/**
 * 収集元の読み込みと処理の 2 つの段階の状態と進行
 * (`GET /api/v0/stages`)。定義元は `backend/core/stage.go` の `InvestigationStages` である。
 */
export type InvestigationStages = {
  loading: LoadingStage;
  processing: ProcessingStage;
  /** server が読める入力形式。読み込みを始める要求の選択肢になる。 */
  formatKeys: FormatKey[];
  /** server が要求による読み込みを受け付けるか。偽の server は基準の directory を持たない。 */
  acceptsRequestedLoading: boolean;
};

function requireNonEmptyString(
  source: Record<string, unknown>,
  key: string,
  path: string,
): string {
  const value = requireString(source, key, path);
  if (value === "") {
    throw new DecodeFailure(`${path}.${key}`, "expected a non-empty string");
  }
  return value;
}

/** 段階が使う理由の種別の中から、失敗の理由を読む。 */
function stageFailureDecoder(
  reasons: readonly StageFailureReason[],
): Decoder<StageFailure> {
  return (input, path) => {
    const source = readObject(input, path);
    const reason = requireEnum(source, "reason", path, reasons);
    if (reason !== "source_import_failed") {
      rejectMember(
        source,
        "originPath",
        path,
        "expected no originPath for a failure not tied to a source",
      );
      return { reason };
    }
    return {
      reason,
      originPath: requireNonEmptyString(source, "originPath", path),
    };
  };
}

/** 失敗した段階だけが理由を持つことを確かめて、理由を読む。 */
function readStageFailure(
  source: Record<string, unknown>,
  state: StageState,
  path: string,
  reasons: readonly StageFailureReason[],
): StageFailure | undefined {
  if (state === "failed") {
    return requireMember(source, "failure", path, stageFailureDecoder(reasons));
  }
  rejectMember(
    source,
    "failure",
    path,
    "expected no failure while the stage has not failed",
  );
  return undefined;
}

const decodeLoadedSourceStatus: Decoder<LoadedSourceStatus> = (input, path) => {
  const source = readObject(input, path);
  return {
    sourceId: requireNonEmptyString(source, "sourceId", path),
    publicationState: requireEnum(
      source,
      "publicationState",
      path,
      publicationStates,
    ),
    counts: requireArray(source, "counts", path, decodeImportCount),
    diagnosisCounts: requireArray(
      source,
      "diagnosisCounts",
      path,
      decodeDiagnosisCount,
    ),
    failureCount: requireCount(source, "failureCount", path),
  };
};

const decodeLoadingSource: Decoder<LoadingSource> = (input, path) => {
  const source = readObject(input, path);
  return {
    originPath: requireNonEmptyString(source, "originPath", path),
    formatKey: requireNonEmptyString(source, "formatKey", path),
    readBytes: requireCount(source, "readBytes", path),
    sizeBytes: optionalCount(source, "sizeBytes", path),
    status: optionalMember(source, "status", path, decodeLoadedSourceStatus),
  };
};

const decodeLoadingStage: Decoder<LoadingStage> = (input, path) => {
  const source = readObject(input, path);
  const state = requireEnum(source, "state", path, stageStates);
  const failure = readStageFailure(source, state, path, loadingFailureReasons);
  const sources = requireArray(source, "sources", path, decodeLoadingSource);
  if (state === "not_started" && sources.length !== 0) {
    throw new DecodeFailure(
      `${path}.sources`,
      "expected no sources before the stage started",
    );
  }
  // 段階を終えた読み込みは、全収集元の取り込みの状態を持つ。
  if (state === "completed") {
    sources.forEach((loading, index) => {
      if (loading.status === undefined) {
        throw new DecodeFailure(
          `${path}.sources[${index}].status`,
          "expected a status in a completed stage",
        );
      }
    });
  }
  return { state, sources, failure };
};

const decodeProcessingStepProgress: Decoder<ProcessingStepProgress> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    step: requireEnum(source, "step", path, processingSteps),
    state: requireEnum(source, "state", path, stageStates),
  };
};

const decodeProcessingStage: Decoder<ProcessingStage> = (input, path) => {
  const source = readObject(input, path);
  const state = requireEnum(source, "state", path, stageStates);
  const failure = readStageFailure(
    source,
    state,
    path,
    processingFailureReasons,
  );
  const steps = requireArray(
    source,
    "steps",
    path,
    decodeProcessingStepProgress,
  );
  const order = steps.map((progress) => progress.step);
  if (
    order.length !== processingSteps.length ||
    order.some((step, index) => step !== processingSteps[index])
  ) {
    throw new DecodeFailure(
      `${path}.steps`,
      `expected the steps ${processingSteps.join(" / ")} in this order`,
    );
  }
  // 段階を終えた処理は、全手順を終えている。
  if (
    state === "completed" &&
    steps.some((progress) => progress.state !== "completed")
  ) {
    throw new DecodeFailure(
      `${path}.steps`,
      "expected every step completed in a completed stage",
    );
  }
  return { state, steps, failure };
};

/**
 * `InvestigationStages` を検証する。
 * 整合の条件は `backend/core/stage.go` の各型の `Validate` に対応する。
 */
export const decodeInvestigationStages: Decoder<InvestigationStages> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const loading = requireMember(source, "loading", path, decodeLoadingStage);
  const processing = requireMember(
    source,
    "processing",
    path,
    decodeProcessingStage,
  );
  // 処理は読み込みを終えた後にだけ始まる。
  if (processing.state !== "not_started" && loading.state !== "completed") {
    throw new DecodeFailure(
      `${path}.processing.state`,
      "expected the processing to start after the loading completed",
    );
  }
  const formatKeys = requireArray(source, "formatKeys", path, (value, at) => {
    if (typeof value !== "string" || value === "") {
      throw new DecodeFailure(at, "expected a non-empty string");
    }
    return value;
  });
  return {
    loading,
    processing,
    formatKeys,
    acceptsRequestedLoading: requireBoolean(
      source,
      "acceptsRequestedLoading",
      path,
    ),
  };
};
