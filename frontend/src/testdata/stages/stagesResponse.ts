/**
 * `GET /api/v0/stages` の応答の JSON。
 * path・byte 数・件数・識別子は本 fixture が決める値である。
 */

/** 大きさを確かめられた収集元の取得元。 */
export const proxyLogPath = "logs/proxy/access.log";

/** 大きさを確かめられなかった収集元の取得元。 */
export const hostLogPath = "logs/host-a/security.log";

/** `proxyLogPath` の取り込み 1 件。 */
export const proxyLogSourceId = "ingest-proxy-log-1";

/** `hostLogPath` の取り込み 1 件。 */
export const hostLogSourceId = "ingest-host-log-1";

/** server が読める入力形式。 */
export const stageFormatKeys = ["squid_combined", "infotrace_mark_ii"];

/** 読み込みの段階が失敗した理由。収集元 1 件の取り込みの失敗で、その収集元を指す。 */
export function loadingFailureJson() {
  return { reason: "source_import_failed", originPath: proxyLogPath };
}

/** 処理の段階が失敗した理由。 */
export function processingFailureJson() {
  return { reason: "graph_build_failed" };
}

type StepStates = {
  observed_layer: string;
  candidate_edges: string;
};

function processingSteps(states: StepStates) {
  return [
    { step: "observed_layer", state: states.observed_layer },
    { step: "candidate_edges", state: states.candidate_edges },
  ];
}

const notStartedSteps: StepStates = {
  observed_layer: "not_started",
  candidate_edges: "not_started",
};

const completedSteps: StepStates = {
  observed_layer: "completed",
  candidate_edges: "completed",
};

/** 読み込み中の収集元。`hostLogPath` は大きさを持たない。 */
function runningSources() {
  return [
    {
      originPath: proxyLogPath,
      formatKey: "squid_combined",
      readBytes: 2048,
      sizeBytes: 8192,
    },
    {
      originPath: hostLogPath,
      formatKey: "infotrace_mark_ii",
      readBytes: 0,
    },
  ];
}

/** 読み込みを終えた収集元。取り込みの状態を持つ。 */
function loadedSources() {
  return [
    {
      originPath: proxyLogPath,
      formatKey: "squid_combined",
      readBytes: 8192,
      sizeBytes: 8192,
      status: {
        sourceId: proxyLogSourceId,
        publicationState: "published_full",
        counts: [
          { category: "read", count: 40 },
          { category: "succeeded", count: 38 },
          { category: "failed", count: 2 },
        ],
        diagnosisCounts: [{ diagnosisClass: "unsupported_format", count: 2 }],
        failureCount: 2,
      },
    },
    {
      originPath: hostLogPath,
      formatKey: "infotrace_mark_ii",
      readBytes: 5120,
      status: {
        sourceId: hostLogSourceId,
        publicationState: "published_partial",
        counts: [{ category: "read", count: 12 }],
        diagnosisCounts: [],
        failureCount: 0,
      },
    },
  ];
}

/** 段階を 1 つも始めていない server の応答。 */
export function notStartedStagesJson() {
  return {
    loading: { state: "not_started", sources: [] },
    processing: {
      state: "not_started",
      steps: processingSteps(notStartedSteps),
    },
    formatKeys: stageFormatKeys,
    acceptsRequestedLoading: true,
  };
}

/** 要求による読み込みを受け付けない server の応答。 */
export function rejectingRequestedLoadingStagesJson() {
  return { ...notStartedStagesJson(), acceptsRequestedLoading: false };
}

/** 読み込みの段階を実行している応答。 */
export function loadingRunningStagesJson() {
  return {
    ...notStartedStagesJson(),
    loading: { state: "running", sources: runningSources() },
  };
}

/** 読み込みの段階が失敗した応答。 */
export function loadingFailedStagesJson() {
  return {
    ...notStartedStagesJson(),
    loading: {
      state: "failed",
      sources: runningSources(),
      failure: loadingFailureJson(),
    },
  };
}

/** 読み込みを終え、処理を始めていない応答。 */
export function loadingCompletedStagesJson() {
  return {
    ...notStartedStagesJson(),
    loading: { state: "completed", sources: loadedSources() },
  };
}

/** 処理の段階を実行している応答。 */
export function processingRunningStagesJson() {
  return {
    ...loadingCompletedStagesJson(),
    processing: {
      state: "running",
      steps: processingSteps({
        observed_layer: "completed",
        candidate_edges: "running",
      }),
    },
  };
}

/** 処理の段階が失敗した応答。 */
export function processingFailedStagesJson() {
  return {
    ...loadingCompletedStagesJson(),
    processing: {
      state: "failed",
      steps: processingSteps({
        observed_layer: "completed",
        candidate_edges: "failed",
      }),
      failure: processingFailureJson(),
    },
  };
}

/** 2 つの段階を終えた応答。 */
export function processingCompletedStagesJson() {
  return {
    ...loadingCompletedStagesJson(),
    processing: { state: "completed", steps: processingSteps(completedSteps) },
  };
}
