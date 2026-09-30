import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import {
  accessLogRecordRawText,
  accessLogRecordResponseJson,
  clientTerminalDerivation,
  clientTerminalId,
  clientTerminalName,
  hostALogRecordResponseJson,
  internalErrorJson,
  stoppedRecordResponseJson,
} from "@/testdata/records/recordResponse";
import {
  accessLogSha256,
  accessLogSourceId,
  apiErrorJson,
  hostALogSha256,
  hostALogSourceId,
} from "@/testdata/sources/sourcesResponse";
import { readObject } from "../contracts/decoding";
import { fetchRecord } from "./records";

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

const accessLogPosition = {
  sourceId: accessLogSourceId,
  sourceContentSha256: accessLogSha256,
  lineNumber: 37,
};

const hostALogPosition = {
  sourceId: hostALogSourceId,
  sourceContentSha256: hostALogSha256,
  sequenceNumber: 112,
};

async function failureOf(body: unknown) {
  stubFetch(jsonResponse(200, body));
  const result = await fetchRecord({ record: accessLogPosition });
  expect(result.ok).toBe(false);
  if (result.ok) {
    throw new Error("応答を読めない結果を期待しました");
  }
  return result.failure;
}

test("行番号で指す元レコードを取得し、応答の項目を画面が扱う型へ変換する", async () => {
  const mock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  const result = await fetchRecord({ record: accessLogPosition });

  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/records?sourceId=${accessLogSourceId}` +
      `&sourceContentSha256=${accessLogSha256}&lineNumber=37`,
  );
  expect(mock.mock.calls[0]?.[1]).toEqual({
    method: "GET",
    headers: { Accept: "application/json" },
    signal: undefined,
  });

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  const value = result.value;
  expect(value.rawText).toBe(accessLogRecordRawText);
  expect(value.recordRef.lineNumber).toBe(37);
  expect(value.sourceIdentity.fileName).toBe("access.log");
  expect(value.fields.map((field) => field.name)).toEqual([
    "clientIp",
    "ident",
    "user",
    "requestTime",
    "requestLine",
    "statusCode",
    "replyBytes",
    "referer",
    "userAgent",
    "squidStatus",
    "requestMethod",
    "requestTargetHost",
    "requestTargetPort",
    "clientTerminal",
    "clientTerminalName",
    "clientPort",
    "process",
  ]);
  const terminal = value.fields[13];
  expect(terminal?.kind === "text" ? terminal.text.valueState : undefined).toBe(
    "derived",
  );
  expect(
    terminal?.kind === "text" ? terminal.text.rawText : "",
  ).toBeUndefined();
  expect(terminal?.kind === "text" ? terminal.text.normalized : undefined).toBe(
    clientTerminalId,
  );
  expect(terminal?.kind === "text" ? terminal.text.derivation : undefined).toBe(
    clientTerminalDerivation,
  );
  // 反対側。表示名の項目は同じ導き方を持ち、別の値を持つ。
  const terminalName = value.fields[14];
  expect(
    terminalName?.kind === "text" ? terminalName.text.normalized : undefined,
  ).toBe(clientTerminalName);
  expect(
    terminalName?.kind === "text" ? terminalName.text.derivation : undefined,
  ).toBe(clientTerminalDerivation);
  expect(clientTerminalName).not.toBe(clientTerminalId);
  expect(value.observationKind.raw).toEqual([]);
  expect(value.derivationTrail).toBeUndefined();
});

test("通番で指す元レコードの項目と、到達した経路の 8 段階を含む", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  const result = await fetchRecord({
    record: hostALogPosition,
    origin: {
      record: accessLogPosition,
      matchConditions: {
        conditions: [
          { conditionKey: "destination_port" },
          { conditionKey: "second_of_time", toleranceSeconds: 2 },
        ],
      },
    },
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/records?sourceId=${hostALogSourceId}` +
      `&sourceContentSha256=${hostALogSha256}&sequenceNumber=112` +
      `&originSourceId=${accessLogSourceId}` +
      `&originSourceContentSha256=${accessLogSha256}&originLineNumber=37` +
      "&matchCondition=destination_port&matchCondition=second_of_time%7E2",
  );

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  const value = result.value;
  // 原資料の key をそのまま name にした項目が出る。段階 2 の補いは 1 件も無い。
  expect(value.fields.map((field) => field.name)).toEqual([
    "headerTime",
    "sn",
    "evt",
    "subEvt",
    "com",
    "tmid",
    "csid",
    "ip",
    "psGUID",
    "psPath",
    "srcIP",
    "srcPort",
    "dstIP",
    "dstPort",
    "recv",
    "send",
  ]);
  expect(value.recordRef.sequenceNumber).toBe(112);
  expect(value.observationKind.status).toBe("determined");
  expect(value.derivationTrail?.steps.map((step) => step.stepKey)).toEqual([
    "A1",
    "A2",
    "A3",
    "A4",
    "A5",
    "A6",
    "A7",
    "A8",
  ]);
  expect(value.derivationTrail?.steps[0]?.inputRefs).toBeUndefined();
  expect(value.derivationTrail?.steps[2]?.inputRefs).toHaveLength(5);
  expect(
    value.derivationTrail?.steps[2]?.inputRefs?.map((input) => input.kind),
  ).toEqual(["source", "source", "source", "source", "source"]);
  expect(value.derivationTrail?.steps[6]?.inputRefs).toHaveLength(2);
  expect(value.derivationTrail?.originRef.lineNumber).toBe(37);
  expect(value.derivationTrail?.stoppedAt).toBeUndefined();
});

test("進めなかった段階を含む応答を読む", async () => {
  stubFetch(jsonResponse(200, stoppedRecordResponseJson()));

  const result = await fetchRecord({ record: hostALogPosition });

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.derivationTrail?.stoppedAt?.stepKey).toBe("C5");
});

test("位置を 1 つも与えない要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  const result = await fetchRecord({
    record: {
      sourceId: accessLogSourceId,
      sourceContentSha256: accessLogSha256,
    },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.summary).toBe("レコードの取得");
});

test("起点の位置を 1 つも与えない要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  const result = await fetchRecord({
    record: hostALogPosition,
    origin: {
      record: {
        sourceId: accessLogSourceId,
        sourceContentSha256: accessLogSha256,
      },
      matchConditions: { conditions: [{ conditionKey: "destination_port" }] },
    },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
});

test("符号を持つ位置の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  const result = await fetchRecord({
    record: { ...accessLogPosition, lineNumber: -1 },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
});

test("行番号の下限 1 を要求に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  const result = await fetchRecord({
    record: { ...accessLogPosition, lineNumber: 1 },
  });

  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/records?sourceId=${accessLogSourceId}` +
      `&sourceContentSha256=${accessLogSha256}&lineNumber=1`,
  );
  expect(result.ok).toBe(true);
});

test("行番号 0 の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  const result = await fetchRecord({
    record: { ...accessLogPosition, lineNumber: 0 },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
});

test("通番の下限 0 を要求に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  const result = await fetchRecord({
    record: { ...hostALogPosition, sequenceNumber: 0 },
  });

  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/records?sourceId=${hostALogSourceId}` +
      `&sourceContentSha256=${hostALogSha256}&sequenceNumber=0`,
  );
  expect(result.ok).toBe(true);
});

test("通番 -1 の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  const result = await fetchRecord({
    record: { ...hostALogPosition, sequenceNumber: -1 },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
});

test("起点の行番号 0 の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  const result = await fetchRecord({
    record: hostALogPosition,
    origin: {
      record: { ...accessLogPosition, lineNumber: 0 },
      matchConditions: { conditions: [{ conditionKey: "destination_port" }] },
    },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
});

test("項目が 0 件の fields を含む応答を読まない", async () => {
  const body = {
    ...readObject(accessLogRecordResponseJson(), "fixture"),
    fields: [],
  };

  expect((await failureOf(body)).kind).toBe("response_unreadable");
});

test("レコード全体の原文を欠く応答を読まない", async () => {
  const { rawText: _removed, ...body } = readObject(
    accessLogRecordResponseJson(),
    "fixture",
  );

  expect((await failureOf(body)).kind).toBe("response_unreadable");
});

test("steps の並び順が stepKey の昇順と食い違う応答を読まない", async () => {
  const response = readObject(hostALogRecordResponseJson(), "fixture");
  const trail = readObject(response.derivationTrail, "fixture.derivationTrail");
  const steps = trail.steps;
  if (!Array.isArray(steps)) {
    throw new Error("fixture が steps の集合を持っていません");
  }
  const body = {
    ...response,
    derivationTrail: { ...trail, steps: [...steps].reverse() },
  };

  expect((await failureOf(body)).kind).toBe("response_unreadable");
});

test("kind が source の段階の入力が record を同時に持つ応答を読まない", async () => {
  const response = readObject(hostALogRecordResponseJson(), "fixture");
  const trail = readObject(response.derivationTrail, "fixture.derivationTrail");
  const body = {
    ...response,
    derivationTrail: {
      ...trail,
      steps: [
        {
          stepKey: "A1",
          inputRefs: [
            {
              kind: "source",
              source: {},
              record: {},
            },
          ],
          usedIdentifiers: ["x"],
          output: "y",
        },
      ],
    },
  };

  expect((await failureOf(body)).kind).toBe("response_unreadable");
});

test("用いた識別子が 0 件の段階を含む応答を読まない", async () => {
  const response = readObject(hostALogRecordResponseJson(), "fixture");
  const trail = readObject(response.derivationTrail, "fixture.derivationTrail");
  const body = {
    ...response,
    derivationTrail: {
      ...trail,
      steps: [{ stepKey: "A1", usedIdentifiers: [], output: "y" }],
    },
  };

  expect((await failureOf(body)).kind).toBe("response_unreadable");
});

test("指すレコードが無い失敗を分類し、応答の code を返す", async () => {
  const mock = stubFetch(jsonResponse(404, apiErrorJson("record_not_found")));

  const result = await fetchRecord({ record: accessLogPosition });

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.failureCode).toBe("record_not_found");
});

test("起点を与えない要求は起点の項目と関連付けの条件を送らない", async () => {
  const mock = stubFetch(jsonResponse(200, hostALogRecordResponseJson()));

  await fetchRecord({ record: hostALogPosition });

  expect(mock.mock.calls[0]?.[0]).not.toContain("origin");
  expect(mock.mock.calls[0]?.[0]).not.toContain("matchCondition");
});

// backend の internal_error は code と message だけを載せる。
test("応答の本体を組めなかった失敗を、server の失敗として分類する", async () => {
  const mock = stubFetch(jsonResponse(500, internalErrorJson()));

  const result = await fetchRecord({ record: hostALogPosition });

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("server");
  expect(result.failure.failureCode).toBe("internal_error");
  expect(result.failure.sourceId).toBeUndefined();
  expect(result.failure.sourceContentSha256).toBeUndefined();
});

test("通信そのものが失敗したときにネットワーク断として分類する", async () => {
  stubFetch(new TypeError("Failed to fetch"));

  const result = await fetchRecord({ record: accessLogPosition });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("network");
  expect(result.failure.nextAction).toBe("通信を確認して再実行");
});
