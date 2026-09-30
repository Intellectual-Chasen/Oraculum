import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse, textResponse } from "@/testdata/http";
import {
  accessLogSourceId,
  apiErrorJson,
  countMismatchSourcesResponseJson,
  emptySourcesResponseJson,
  failureCountMismatchSourcesResponseJson,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { readObject } from "../contracts/decoding";
import { fetchSources } from "./sources";

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

test("収集元の一覧を取得し、応答の項目を画面が扱う型へ変換する", async () => {
  const body = sourcesResponseJson();
  const mock = stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/sources");
  expect(mock.mock.calls[0]?.[1]).toEqual({
    method: "GET",
    headers: { Accept: "application/json" },
    signal: undefined,
  });

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.sourceCount).toBe(2);
  expect(result.value.sources.map((item) => item.source.fileName)).toEqual([
    "access.log",
    "host-a.log",
  ]);
  expect(result.value.sources[0]?.source.recordCount).toBe(1200);
  expect(result.value.sources[0]?.source.observedRangeFirst?.rawText).toBe(
    "[08/Oct/2031:10:20:35 +0900]",
  );
  expect(result.value.sources[0]?.source.observedRangeFirst?.normalized).toBe(
    "2031-10-08T10:20:35+09:00",
  );
  expect(result.value.sources[1]?.source.recordCount).toBeUndefined();
  expect(result.value.sources[1]?.importStatus.withheldReason).toBe(
    "identifier_collision",
  );
  expect(result.value.emptyReason).toBeUndefined();
});

test("時刻の解釈で読んだ観測期間を、収集元の組から読む", async () => {
  const body = sourcesResponseJson() as { sources: Record<string, unknown>[] };
  const interpreted = (text: string) => ({
    rawText: text,
    normalized: text,
    normalizedForm: "local_without_offset",
    precision: "second",
    offsetState: "undetermined",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
    interpretation: { offset: "+09:00", assertionId: "as:synthetic" },
  });
  const range = {
    from: interpreted("2031-10-08T01:20:35"),
    to: interpreted("2031-10-08T02:30:45"),
  };
  body.sources[1] = { ...body.sources[1], interpretedObservedRange: range };
  stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.sources[0]?.interpretedObservedRange).toBeUndefined();
  expect(result.value.sources[1]?.interpretedObservedRange).toEqual(range);
});

test("query の項目を 1 つも載せない", async () => {
  const mock = stubFetch(jsonResponse(200, sourcesResponseJson()));

  const result = await fetchSources();

  const requestedUrl = new URL(
    String(mock.mock.calls[0]?.[0]),
    "https://example.test",
  );
  expect([...requestedUrl.searchParams.keys()]).toEqual([]);
  expect(result.ok).toBe(true);
});

test("収集元が 1 件も無い応答は emptyReason を持つ", async () => {
  const mock = stubFetch(jsonResponse(200, emptySourcesResponseJson()));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.sourceCount).toBe(0);
  expect(result.value.sources).toEqual([]);
  expect(result.value.emptyReason).toBe("no_source_ingested");
});

test("sourceCount が 0 で emptyReason を欠く応答を読まない", async () => {
  const body = { sources: [], sourceCount: 0 };
  const mock = stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("sourceCount が 1 以上で emptyReason を出す応答を読まない", async () => {
  const body = {
    ...(sourcesResponseJson() as Record<string, unknown>),
    emptyReason: "no_source_ingested",
  };
  stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("source の組に importStatus が無い応答を読まない", async () => {
  const fixture = sourcesResponseJson() as { sources: unknown[] };
  const body = {
    ...(sourcesResponseJson() as Record<string, unknown>),
    sources: [
      {
        ...(fixture.sources[0] as Record<string, unknown>),
        importStatus: undefined,
      },
    ],
  };
  stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("source と importStatus の sourceId が異なる応答を読まない", async () => {
  const response = readObject(sourcesResponseJson(), "fixture");
  const sources = response.sources;
  if (!Array.isArray(sources)) {
    throw new Error("fixture が sources の集合を持っていません");
  }
  const [first, ...rest] = sources;
  const firstSource = readObject(first, "fixture.sources[0]");
  const firstImportStatus = readObject(
    firstSource.importStatus,
    "fixture.sources[0].importStatus",
  );
  const body = {
    ...response,
    sources: [
      {
        ...firstSource,
        importStatus: { ...firstImportStatus, sourceId: "different-source" },
      },
      ...rest,
    ],
  };
  stubFetch(jsonResponse(200, body));

  const result = await fetchSources();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("sourceCount が sources の要素数と食い違う応答を読まない", async () => {
  const mock = stubFetch(jsonResponse(200, countMismatchSourcesResponseJson()));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("failureCount が failures の要素数と食い違う応答を読まない", async () => {
  const mock = stubFetch(
    jsonResponse(200, failureCountMismatchSourcesResponseJson()),
  );

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("サーバー内の失敗を分類し、応答の code を返す", async () => {
  const mock = stubFetch(jsonResponse(500, apiErrorJson("internal_error")));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("server");
  expect(result.failure.failureCode).toBe("internal_error");
  expect(result.failure.nextAction).toContain("時間を空けて");
});

test("要求の拒否を分類し、失敗に関わる収集元を返す", async () => {
  stubFetch(
    jsonResponse(404, apiErrorJson("source_not_found", accessLogSourceId)),
  );

  const result = await fetchSources();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.failureCode).toBe("source_not_found");
  expect(result.failure.sourceId).toBe(accessLogSourceId);
});

test("失敗の応答が ApiError の形でないときも HTTP の status で分類する", async () => {
  stubFetch(textResponse(503, "service unavailable"));

  const result = await fetchSources();

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("server");
  expect(result.failure.failureCode).toBeUndefined();
});

test("通信そのものが失敗したときにネットワーク断として分類する", async () => {
  const mock = stubFetch(new TypeError("Failed to fetch"));

  const result = await fetchSources();

  expect(mock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("network");
  expect(result.failure.nextAction).toBe("通信を確認して再実行");
});
