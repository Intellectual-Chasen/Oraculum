import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import {
  loadingRunningStagesJson,
  notStartedStagesJson,
  processingRunningStagesJson,
  proxyLogPath,
} from "@/testdata/stages/stagesResponse";
import { fetchStages, startLoading, startProcessing } from "./stages";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response | Error) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => {
    if (result instanceof Error) {
      throw result;
    }
    return result;
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function sentBody(mock: ReturnType<typeof stubFetch>): unknown {
  return JSON.parse(String(mock.mock.calls[0]?.[1]?.body));
}

test("段階の状態を GET で取得して読む", async () => {
  const mock = stubFetch(jsonResponse(200, notStartedStagesJson()));

  const result = await fetchStages();

  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/stages");
  expect(mock.mock.calls[0]?.[1]?.method).toBe("GET");
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.loading.state).toBe("not_started");
});

test("段階の状態の形が異なる応答を、読めない応答の失敗にする", async () => {
  stubFetch(jsonResponse(200, { loading: { state: "paused" } }));

  const result = await fetchStages();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
  expect(result.failure.summary).toBe("段階の状態の取得");
});

test("通信が失敗した取得を、ネットワークの失敗にする", async () => {
  stubFetch(new TypeError("offline"));

  const result = await fetchStages();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("network");
  // 段階の状態の取得は server の状態を変えない。取得の案内は、同じ取得をやり直せることを書く。
  expect(result.failure.nextAction).toBe("通信を確認して再実行");
});

test("読み込みを始める要求は、空白だけの省略可の項目を本文に載せない", async () => {
  const mock = stubFetch(jsonResponse(202, loadingRunningStagesJson()));

  const result = await startLoading([
    {
      originPath: proxyLogPath,
      formatKey: "squid_combined",
      formatSpec: "  ",
      caseId: " baseline ",
      terminal: { id: "", hostname: " host-a.example.test ", ip: " " },
    },
    {
      originPath: "logs/host-b/security.log",
      formatKey: "infotrace_mark_ii",
      terminal: { id: " ", hostname: "", ip: "" },
    },
  ]);

  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/stages/loading");
  expect(mock.mock.calls[0]?.[1]?.method).toBe("POST");
  expect(sentBody(mock)).toEqual({
    sources: [
      {
        originPath: proxyLogPath,
        formatKey: "squid_combined",
        caseId: "baseline",
        terminal: { hostname: "host-a.example.test" },
      },
      {
        originPath: "logs/host-b/security.log",
        formatKey: "infotrace_mark_ii",
      },
    ],
  });
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.loading.state).toBe("running");
});

test("読み込みを始める要求は、値を持つ省略可の項目を前後の空白を除いて載せる", async () => {
  const mock = stubFetch(jsonResponse(202, loadingRunningStagesJson()));

  await startLoading([
    {
      originPath: proxyLogPath,
      formatKey: "squid_combined",
      formatSpec: " %>a %ru ",
      terminal: { id: "t-1", hostname: "", ip: "192.0.2.10" },
    },
  ]);

  expect(sentBody(mock)).toEqual({
    sources: [
      {
        originPath: proxyLogPath,
        formatKey: "squid_combined",
        formatSpec: "%>a %ru",
        terminal: { id: "t-1", ip: "192.0.2.10" },
      },
    ],
  });
});

test.each([
  ["収集元が無い入力", [], "収集元なし"],
  [
    "path が空白だけの入力",
    [{ originPath: " ", formatKey: "squid_combined" }],
    "収集元 1: path なし",
  ],
  [
    "2 件目の path が空白だけの入力",
    [
      { originPath: proxyLogPath, formatKey: "squid_combined" },
      { originPath: " ", formatKey: "squid_combined" },
    ],
    "収集元 2: path なし",
  ],
  [
    "入力形式を選んでいない入力",
    [{ originPath: proxyLogPath, formatKey: "" }],
    `${proxyLogPath}: 入力形式なし`,
  ],
  [
    "案件の文字列が規則に合わない入力",
    [{ originPath: proxyLogPath, formatKey: "squid_combined", caseId: "a b" }],
    "案件の形式の誤り",
  ],
])("%s を送らずに退ける", async (_name, drafts, summary) => {
  const mock = stubFetch(jsonResponse(202, loadingRunningStagesJson()));

  const result = await startLoading(drafts);

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.summary).toBe(summary);
});

test("基準の directory の外を指す path を退けた応答の code を失敗に載せる", async () => {
  stubFetch(
    jsonResponse(400, {
      code: "invalid_request",
      message: "originPath escapes the source root",
      originPath: "../outside.log",
    }),
  );

  const result = await startLoading([
    { originPath: "../outside.log", formatKey: "squid_combined" },
  ]);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.failureCode).toBe("invalid_request");
  expect(result.failure.originPath).toBe("../outside.log");
  expect(result.failure.summary).toBe("収集元の読み込みの開始");
});

test("処理を始める要求は本文を送らず、始めた後の段階を読む", async () => {
  const mock = stubFetch(jsonResponse(202, processingRunningStagesJson()));

  const result = await startProcessing();

  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/stages/processing");
  expect(mock.mock.calls[0]?.[1]?.method).toBe("POST");
  expect(mock.mock.calls[0]?.[1]?.body).toBeUndefined();
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.processing.state).toBe("running");
});

test.each([
  ["stage_not_ready", "読み込みか処理が未完了"],
  ["stage_already_started", "実行中か完了済みの段階"],
])(
  "処理を始める要求への 409 %s を、状態の説明と段階の状態を確かめる案内を添えた失敗にする",
  async (code, description) => {
    stubFetch(jsonResponse(409, { code, message: "x" }));

    const result = await startProcessing();

    expect(result.ok).toBe(false);
    if (result.ok) {
      return;
    }
    expect(result.failure.kind).toBe("request_rejected");
    expect(result.failure.failureCode).toBe(code);
    expect(result.failure.failureDescription).toBe(description);
    expect(result.failure.summary).toBe("処理の開始");
    expect(result.failure.nextAction).toBe(
      "段階の状態を確認してから操作を選択",
    );
  },
);

test.each([
  [
    "収集元の読み込みを始める要求",
    () =>
      startLoading([{ originPath: proxyLogPath, formatKey: "squid_combined" }]),
  ],
  ["処理を始める要求", () => startProcessing()],
])(
  "%s が通信の失敗で終わったら、server が要求を受け取ったかを確かめられないことを案内する",
  async (_name, send) => {
    stubFetch(new TypeError("offline"));

    const result = await send();

    expect(result.ok).toBe(false);
    if (result.ok) {
      return;
    }
    expect(result.failure.kind).toBe("network");
    expect(result.failure.nextAction).toBe(
      "段階の状態を確認してから操作を選択",
    );
  },
);
