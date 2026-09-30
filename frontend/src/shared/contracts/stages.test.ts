import { expect, test } from "vitest";
import {
  hostLogPath,
  hostLogSourceId,
  loadingCompletedStagesJson,
  loadingFailedStagesJson,
  loadingRunningStagesJson,
  notStartedStagesJson,
  processingCompletedStagesJson,
  processingFailedStagesJson,
  processingRunningStagesJson,
  proxyLogPath,
  proxyLogSourceId,
  stageFormatKeys,
} from "@/testdata/stages/stagesResponse";
import { DecodeFailure } from "./decoding";
import { decodeInvestigationStages, processingSteps } from "./stages";

function decode(input: unknown) {
  return decodeInvestigationStages(input, "response");
}

/** 応答を読まないことと、読めなかった位置と理由を確かめる。 */
function expectRejectedAt(input: unknown, path: string, reason: string) {
  let thrown: unknown;
  try {
    decode(input);
  } catch (error) {
    thrown = error;
  }
  if (!(thrown instanceof DecodeFailure)) {
    throw new Error(`expected a DecodeFailure at ${path}, got ${thrown}`);
  }
  expect(thrown.path).toBe(path);
  expect(thrown.message).toContain(reason);
}

test("段階を始めていない応答から、入力形式と読み込みの受け付けを読む", () => {
  const stages = decode(notStartedStagesJson());

  expect(stages.loading).toEqual({
    state: "not_started",
    sources: [],
    failure: undefined,
  });
  expect(stages.processing.state).toBe("not_started");
  expect(stages.processing.steps.map((progress) => progress.step)).toEqual([
    ...processingSteps,
  ]);
  expect(stages.formatKeys).toEqual(stageFormatKeys);
  expect(stages.acceptsRequestedLoading).toBe(true);
});

test("読み込み中の収集元の読んだ byte 数と大きさを読み、大きさの欠けた収集元を欠測にする", () => {
  const stages = decode(loadingRunningStagesJson());

  const byPath = new Map(
    stages.loading.sources.map((source) => [source.originPath, source]),
  );
  expect(byPath.get(proxyLogPath)).toEqual({
    originPath: proxyLogPath,
    formatKey: "squid_combined",
    readBytes: 2048,
    sizeBytes: 8192,
    status: undefined,
  });
  expect(byPath.get(hostLogPath)?.sizeBytes).toBeUndefined();
  expect(byPath.get(hostLogPath)?.readBytes).toBe(0);
});

test("読み込みを終えた収集元の取り込みの状態を読む", () => {
  const stages = decode(loadingCompletedStagesJson());

  const statuses = new Map(
    stages.loading.sources.map((source) => [source.originPath, source.status]),
  );
  expect(statuses.get(proxyLogPath)).toEqual({
    sourceId: proxyLogSourceId,
    publicationState: "published_full",
    counts: [
      { category: "read", count: 40 },
      { category: "succeeded", count: 38 },
      { category: "failed", count: 2 },
    ],
    diagnosisCounts: [{ diagnosisClass: "unsupported_format", count: 2 }],
    failureCount: 2,
  });
  expect(statuses.get(hostLogPath)?.sourceId).toBe(hostLogSourceId);
});

test("失敗した段階の理由の種別と、失敗した収集元を読む", () => {
  const loading = decode(loadingFailedStagesJson());
  const processing = decode(processingFailedStagesJson());

  expect(loading.loading.failure).toEqual({
    reason: "source_import_failed",
    originPath: proxyLogPath,
  });
  expect(processing.processing.failure).toEqual({
    reason: "graph_build_failed",
  });
  expect(processing.loading.failure).toBeUndefined();
});

test("処理中と処理を終えた応答で、手順ごとの状態を読む", () => {
  const running = decode(processingRunningStagesJson());
  const completed = decode(processingCompletedStagesJson());

  expect(running.processing.steps).toEqual([
    { step: "observed_layer", state: "completed" },
    { step: "candidate_edges", state: "running" },
  ]);
  expect(
    completed.processing.steps.every(
      (progress) => progress.state === "completed",
    ),
  ).toBe(true);
});

test("失敗した段階が理由を欠く応答を読まない", () => {
  const json = loadingFailedStagesJson();
  const { failure: _failure, ...loading } = json.loading;

  expectRejectedAt(
    { ...json, loading },
    "response.loading.failure",
    "expected a JSON object",
  );
});

test("失敗していない段階が理由を持つ応答を読まない", () => {
  const json = processingRunningStagesJson();

  expectRejectedAt(
    {
      ...json,
      processing: {
        ...json.processing,
        failure: { reason: "graph_build_failed" },
      },
    },
    "response.processing.failure",
    "expected no failure",
  );
});

test("段階の使わない理由の種別を持つ応答を読まない", () => {
  const loading = loadingFailedStagesJson();
  const processing = processingFailedStagesJson();

  expectRejectedAt(
    {
      ...loading,
      loading: {
        ...loading.loading,
        failure: { reason: "graph_build_failed" },
      },
    },
    "response.loading.failure.reason",
    "expected one of",
  );
  expectRejectedAt(
    {
      ...processing,
      processing: {
        ...processing.processing,
        failure: { reason: "recording_failed" },
      },
    },
    "response.processing.failure.reason",
    "expected one of",
  );
});

test("収集元 1 件の失敗が収集元を指さない応答と、収集元を指さない失敗が収集元を持つ応答を読まない", () => {
  const json = loadingFailedStagesJson();

  expectRejectedAt(
    {
      ...json,
      loading: { ...json.loading, failure: { reason: "source_import_failed" } },
    },
    "response.loading.failure.originPath",
    "expected a string",
  );
  expectRejectedAt(
    {
      ...json,
      loading: {
        ...json.loading,
        failure: { reason: "recording_failed", originPath: proxyLogPath },
      },
    },
    "response.loading.failure.originPath",
    "expected no originPath",
  );
});

test("読み込みを終えた段階で、取り込みの状態を欠く収集元を持つ応答を読まない", () => {
  const json = loadingCompletedStagesJson();
  const [first, ...rest] = json.loading.sources;
  if (first === undefined) {
    throw new Error("fixture has no source");
  }
  const { status: _status, ...withoutStatus } = first;

  expectRejectedAt(
    {
      ...json,
      loading: { ...json.loading, sources: [withoutStatus, ...rest] },
    },
    "response.loading.sources[0].status",
    "expected a status in a completed stage",
  );
});

test("読み込みを始めていない段階が収集元を持つ応答を読まない", () => {
  const json = notStartedStagesJson();

  expectRejectedAt(
    {
      ...json,
      loading: {
        state: "not_started",
        sources: loadingRunningStagesJson().loading.sources,
      },
    },
    "response.loading.sources",
    "expected no sources before the stage started",
  );
});

test("読み込みを終える前に処理を始めた応答を読まない", () => {
  const json = processingRunningStagesJson();

  expectRejectedAt(
    { ...json, loading: loadingRunningStagesJson().loading },
    "response.processing.state",
    "expected the processing to start after the loading completed",
  );
});

test("手順の並びが実行の順と異なる応答と、手順を欠く応答を読まない", () => {
  const json = processingRunningStagesJson();

  for (const steps of [
    [...json.processing.steps].reverse(),
    json.processing.steps.filter(
      (progress) => progress.step !== "candidate_edges",
    ),
  ]) {
    expectRejectedAt(
      { ...json, processing: { ...json.processing, steps } },
      "response.processing.steps",
      "in this order",
    );
  }
});

test("処理を終えた段階で、終えていない手順を持つ応答を読まない", () => {
  const json = processingCompletedStagesJson();

  expectRejectedAt(
    {
      ...json,
      processing: {
        ...json.processing,
        steps: processingRunningStagesJson().processing.steps,
      },
    },
    "response.processing.steps",
    "expected every step completed in a completed stage",
  );
});

test("定義の外の状態と、空の入力形式を読まない", () => {
  const json = notStartedStagesJson();

  expectRejectedAt(
    { ...json, loading: { state: "paused", sources: [] } },
    "response.loading.state",
    "expected one of",
  );
  expectRejectedAt(
    { ...json, formatKeys: [""] },
    "response.formatKeys[0]",
    "expected a non-empty string",
  );
});

test("負の読んだ byte 数を読まない", () => {
  const json = loadingRunningStagesJson();

  expectRejectedAt(
    {
      ...json,
      loading: {
        ...json.loading,
        sources: json.loading.sources.map((source) => ({
          ...source,
          readBytes: -1,
        })),
      },
    },
    "response.loading.sources[0].readBytes",
    "expected a non-negative safe integer",
  );
});
