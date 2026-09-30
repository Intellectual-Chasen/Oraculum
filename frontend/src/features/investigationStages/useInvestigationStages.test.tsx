// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, textResponse } from "@/testdata/http";
import {
  loadingCompletedStagesJson,
  loadingRunningStagesJson,
  notStartedStagesJson,
  processingCompletedStagesJson,
  processingRunningStagesJson,
  proxyLogPath,
} from "@/testdata/stages/stagesResponse";
import {
  stagesPollIntervalMs,
  useInvestigationStages,
} from "./useInvestigationStages";

type Reply = Response | Error;

/**
 * 要求の method と path ごとに、応答を並びの順で返す。並びの最後の応答は、それ以降の
 * 要求にも返す。
 */
function stubFetch(replies: Record<string, Reply[]>) {
  const served = new Map<string, number>();
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${input}`;
    const queue = replies[key];
    if (queue === undefined || queue.length === 0) {
      throw new Error(`unexpected request: ${key}`);
    }
    const index = served.get(key) ?? 0;
    served.set(key, index + 1);
    const reply = queue[Math.min(index, queue.length - 1)];
    if (reply === undefined || reply instanceof Error) {
      throw reply ?? new Error("no reply");
    }
    return reply.clone();
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function callsOf(mock: ReturnType<typeof stubFetch>, key: string): number {
  return mock.mock.calls.filter(
    ([input, init]) => `${init?.method ?? "GET"} ${input}` === key,
  ).length;
}

const getStages = "GET /api/v0/stages";

/** 予約した取り直しの時刻まで時計を進め、応答の反映を待つ。 */
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("実行中の段階が無いときは、最初の取得の後に取り直さない", async () => {
  const mock = stubFetch({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);

  expect(result.current.state.status).toBe("loaded");
  await advance(stagesPollIntervalMs * 3);
  expect(callsOf(mock, getStages)).toBe(1);
});

test("実行中の段階があるあいだ間隔ごとに取り直し、段階を終えたら取り直しを止める", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, processingRunningStagesJson()),
      jsonResponse(200, processingRunningStagesJson()),
      jsonResponse(200, processingCompletedStagesJson()),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  expect(callsOf(mock, getStages)).toBe(1);

  await advance(stagesPollIntervalMs - 1);
  expect(callsOf(mock, getStages)).toBe(1);
  await advance(1);
  expect(callsOf(mock, getStages)).toBe(2);

  await advance(stagesPollIntervalMs);
  expect(callsOf(mock, getStages)).toBe(3);
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.processing.state).toBe(
    "completed",
  );

  await advance(stagesPollIntervalMs * 3);
  expect(callsOf(mock, getStages)).toBe(3);
});

test("画面を離れたら、予約した取り直しを消す", async () => {
  const mock = stubFetch({
    [getStages]: [jsonResponse(200, loadingRunningStagesJson())],
  });

  const { unmount } = renderHook(() => useInvestigationStages());
  await advance(0);
  unmount();

  await advance(stagesPollIntervalMs * 3);
  expect(callsOf(mock, getStages)).toBe(1);
});

test("画面を離れたら、実行中の取り直しを打ち切る", async () => {
  let pollSignal: AbortSignal | undefined;
  const mock = vi.fn(async (_input: string, init?: RequestInit) => {
    if (mock.mock.calls.length === 1) {
      return jsonResponse(200, loadingRunningStagesJson());
    }
    pollSignal = init?.signal ?? undefined;
    return new Promise<Response>(() => {});
  });
  vi.stubGlobal("fetch", mock);

  const { unmount } = renderHook(() => useInvestigationStages());
  await advance(0);
  await advance(stagesPollIntervalMs);
  expect(pollSignal?.aborted).toBe(false);

  unmount();

  expect(pollSignal?.aborted).toBe(true);
});

test("取り直しが失敗しても最後に読めた状態を保ち、次の取り直しの成功で失敗を消す", async () => {
  stubFetch({
    [getStages]: [
      jsonResponse(200, loadingRunningStagesJson()),
      new TypeError("offline"),
      jsonResponse(200, loadingCompletedStagesJson()),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await advance(stagesPollIntervalMs);

  expect(result.current.refreshFailure?.kind).toBe("network");
  const kept = result.current.state;
  expect(kept.status === "loaded" && kept.value.loading.state).toBe("running");

  await advance(stagesPollIntervalMs);

  expect(result.current.refreshFailure).toBeUndefined();
  const next = result.current.state;
  expect(next.status === "loaded" && next.value.loading.state).toBe(
    "completed",
  );
});

test("最初の取得の失敗を、失敗の状態にする", async () => {
  stubFetch({
    [getStages]: [jsonResponse(500, { code: "internal_error", message: "x" })],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);

  const { state } = result.current;
  expect(state.status).toBe("failed");
  expect(state.status === "failed" && state.failure.failureCode).toBe(
    "internal_error",
  );
});

test("読み込みを始めた応答を反映し、実行中の読み込みの取り直しを始める", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, notStartedStagesJson()),
      jsonResponse(200, loadingCompletedStagesJson()),
    ],
    "POST /api/v0/stages/loading": [
      jsonResponse(202, loadingRunningStagesJson()),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startLoading(
      [{ originPath: proxyLogPath, formatKey: "squid_combined" }],
      false,
    );
  });

  expect(callsOf(mock, "POST /api/v0/stages/loading")).toBe(1);
  expect(result.current.startFailure).toBeUndefined();
  expect(result.current.isStarting).toBe(false);
  const started = result.current.state;
  expect(started.status === "loaded" && started.value.loading.state).toBe(
    "running",
  );

  await advance(stagesPollIntervalMs);

  expect(callsOf(mock, getStages)).toBe(2);
  const polled = result.current.state;
  expect(polled.status === "loaded" && polled.value.loading.state).toBe(
    "completed",
  );
});

test("段階を始めた後だと退けられたら、失敗を出して段階の状態を取り直す", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      jsonResponse(200, processingCompletedStagesJson()),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });

  expect(result.current.startFailure?.kind).toBe("processing");
  expect(result.current.startFailure?.failure.failureCode).toBe(
    "stage_already_started",
  );
  expect(callsOf(mock, getStages)).toBe(2);
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.processing.state).toBe(
    "completed",
  );
});

test("読み込みを終えていないと退けられたら、失敗を出して段階の状態を取り直す", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      jsonResponse(200, loadingRunningStagesJson()),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_not_ready", message: "x" }),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });

  expect(result.current.startFailure?.failure.failureCode).toBe(
    "stage_not_ready",
  );
  expect(callsOf(mock, getStages)).toBe(2);
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.loading.state).toBe(
    "running",
  );
});

test("退けられた後の取り直しが失敗したら、古い状態を保って取り直しの失敗を出す", async () => {
  stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      new TypeError("offline"),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });

  expect(result.current.startFailure?.failure.failureCode).toBe(
    "stage_already_started",
  );
  expect(result.current.refreshFailure?.kind).toBe("network");
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.processing.state).toBe(
    "not_started",
  );
});

test("開始の要求の応答を待つ間に画面を離れたら、応答を反映しない", async () => {
  let resolveStart: (reply: Response) => void = () => {};
  const mock = vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return new Promise<Response>((resolve) => {
        resolveStart = resolve;
      });
    }
    return jsonResponse(200, loadingCompletedStagesJson());
  });
  vi.stubGlobal("fetch", mock);

  const { result, unmount } = renderHook(() => useInvestigationStages());
  await advance(0);
  let started: Promise<void> = Promise.resolve();
  act(() => {
    started = result.current.startProcessing();
  });
  expect(result.current.isStarting).toBe(true);
  unmount();

  await act(async () => {
    resolveStart(
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
    );
    await started;
  });

  // 画面を離れた後の応答では、段階の状態を取り直さない。要求は最初の取得と開始の 2 つだけである。
  expect(
    mock.mock.calls.map(([input, init]) => `${init?.method ?? "GET"} ${input}`),
  ).toEqual([getStages, "POST /api/v0/stages/processing"]);
});

test("最初の取得に失敗した後、取得し直すと段階の状態を読む", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(500, { code: "internal_error", message: "x" }),
      jsonResponse(200, notStartedStagesJson()),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  expect(result.current.state.status).toBe("failed");

  act(() => {
    result.current.reload();
  });
  expect(result.current.state.status).toBe("loading");
  await advance(0);

  expect(callsOf(mock, getStages)).toBe(2);
  expect(result.current.state.status).toBe("loaded");
});

test("開始の要求が通信の失敗で終わったら、段階の状態を取り直し、実行中の段階の取り直しの予約を始める", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      jsonResponse(200, processingRunningStagesJson()),
      jsonResponse(200, processingCompletedStagesJson()),
    ],
    "POST /api/v0/stages/processing": [new TypeError("offline")],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });

  expect(result.current.startFailure?.kind).toBe("processing");
  expect(result.current.startFailure?.failure.kind).toBe("network");
  expect(result.current.isStarting).toBe(false);
  expect(callsOf(mock, getStages)).toBe(2);
  const refetched = result.current.state;
  expect(
    refetched.status === "loaded" && refetched.value.processing.state,
  ).toBe("running");
  expect(result.current.isPolling).toBe(true);

  await advance(stagesPollIntervalMs);

  expect(callsOf(mock, getStages)).toBe(3);
  const polled = result.current.state;
  expect(polled.status === "loaded" && polled.value.processing.state).toBe(
    "completed",
  );
});

test.each([
  [
    "server の失敗",
    () => jsonResponse(500, { code: "internal_error", message: "x" }),
    "server",
  ],
  [
    "読めない応答",
    () => jsonResponse(202, { loading: { state: "paused" } }),
    "response_unreadable",
  ],
])(
  "開始の要求が %s で終わったら、段階の状態を取り直す",
  async (_name, reply, kind) => {
    const mock = stubFetch({
      [getStages]: [
        jsonResponse(200, loadingCompletedStagesJson()),
        jsonResponse(200, processingRunningStagesJson()),
      ],
      "POST /api/v0/stages/processing": [reply()],
    });

    const { result } = renderHook(() => useInvestigationStages());
    await advance(0);
    await act(async () => {
      await result.current.startProcessing();
    });

    expect(result.current.startFailure?.failure.kind).toBe(kind);
    expect(callsOf(mock, getStages)).toBe(2);
    const { state } = result.current;
    expect(state.status === "loaded" && state.value.processing.state).toBe(
      "running",
    );
  },
);

test.each([
  ["権限の不足", () => textResponse(403, "forbidden"), "authorization"],
  ["要求頻度の制限", () => textResponse(429, "slow down"), "rate_limited"],
])(
  "開始の要求を %s で退けられたら、段階の状態を取り直さない",
  async (_name, reply, kind) => {
    const mock = stubFetch({
      [getStages]: [jsonResponse(200, loadingCompletedStagesJson())],
      "POST /api/v0/stages/processing": [reply()],
    });

    const { result } = renderHook(() => useInvestigationStages());
    await advance(0);
    await act(async () => {
      await result.current.startProcessing();
    });

    expect(result.current.startFailure?.failure.kind).toBe(kind);
    expect(callsOf(mock, getStages)).toBe(1);
  },
);

test("送る前の入力の検査で退けた開始の要求では、要求を送らず、段階の状態を取り直さない", async () => {
  const mock = stubFetch({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startLoading(
      [{ originPath: " ", formatKey: "squid_combined" }],
      false,
    );
  });

  expect(result.current.startFailure?.kind).toBe("loading");
  expect(result.current.startFailure?.failure.summary).toBe(
    "収集元 1: path なし",
  );
  expect(mock).toHaveBeenCalledTimes(1);
});

test("段階を始める要求が成功したら、前の取り直しの失敗を消す", async () => {
  stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      new TypeError("offline"),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
      jsonResponse(202, processingRunningStagesJson()),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });
  expect(result.current.refreshFailure?.kind).toBe("network");

  await act(async () => {
    await result.current.startProcessing();
  });

  expect(result.current.startFailure).toBeUndefined();
  expect(result.current.refreshFailure).toBeUndefined();
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.processing.state).toBe(
    "running",
  );
});

test("取り直しの予約が無いときに取り直すと、応答までは最後に読めた状態を保ち、成功したら取り直しの失敗を消す", async () => {
  const mock = stubFetch({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      new TypeError("offline"),
      jsonResponse(200, processingRunningStagesJson()),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startProcessing();
  });
  expect(result.current.refreshFailure?.kind).toBe("network");
  expect(result.current.isPolling).toBe(false);

  let refreshed: Promise<void> = Promise.resolve();
  act(() => {
    refreshed = result.current.refresh();
  });
  expect(result.current.isRefreshing).toBe(true);
  const kept = result.current.state;
  expect(kept.status === "loaded" && kept.value.processing.state).toBe(
    "not_started",
  );
  await act(async () => {
    await refreshed;
  });

  expect(callsOf(mock, getStages)).toBe(3);
  expect(result.current.isRefreshing).toBe(false);
  expect(result.current.refreshFailure).toBeUndefined();
  const { state } = result.current;
  expect(state.status === "loaded" && state.value.processing.state).toBe(
    "running",
  );
  expect(result.current.isPolling).toBe(true);
});

test("入力の誤りで退けられた開始の要求では、段階の状態を取り直さない", async () => {
  const mock = stubFetch({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    "POST /api/v0/stages/loading": [
      jsonResponse(400, { code: "invalid_request", message: "x" }),
    ],
  });

  const { result } = renderHook(() => useInvestigationStages());
  await advance(0);
  await act(async () => {
    await result.current.startLoading(
      [{ originPath: "../outside.log", formatKey: "squid_combined" }],
      false,
    );
  });

  expect(result.current.startFailure?.kind).toBe("loading");
  expect(result.current.startFailure?.failure.failureCode).toBe(
    "invalid_request",
  );
  expect(callsOf(mock, getStages)).toBe(1);
});
