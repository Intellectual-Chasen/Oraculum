import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import { decodeString } from "../contracts/decoding";
import {
  onAuthenticationRequired,
  requestJson,
  requestNdjson,
} from "./httpClient";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => result);
  vi.stubGlobal("fetch", mock);
  return mock;
}

const request = {
  path: "/assertions",
  decode: decodeString,
  failureSummary: "取得できませんでした。",
};

test("本文を持たない要求は GET を送り、Content-Type を付けない", async () => {
  const mock = stubFetch(jsonResponse(200, "ok"));

  await requestJson(request);

  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("GET");
  expect(init?.headers).toEqual({ Accept: "application/json" });
  expect(init?.body).toBeUndefined();
});

test("本文を持つ要求は Content-Type を付け、組を JSON へ直して送る", async () => {
  const mock = stubFetch(jsonResponse(201, "ok"));

  await requestJson({
    ...request,
    method: "POST",
    body: { kind: "technique", author: "analyst-a" },
  });

  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("POST");
  expect(init?.headers).toEqual({
    Accept: "application/json",
    "Content-Type": "application/json",
  });
  expect(JSON.parse(String(init?.body))).toEqual({
    kind: "technique",
    author: "analyst-a",
  });
});

test("204 の応答は本文を読まずに、decode へ undefined を渡す", async () => {
  stubFetch(new Response(null, { status: 204 }));
  const decode = vi.fn((input: unknown) => input);

  const result = await requestJson({ ...request, method: "DELETE", decode });

  expect(decode).toHaveBeenCalledWith(undefined, "response");
  expect(result).toEqual({ ok: true, value: undefined });
});

test("authentication_required の失敗は、登録した listener に同じ失敗を渡し、登録を外した後は渡さない", async () => {
  const listener = vi.fn();
  const unsubscribe = onAuthenticationRequired(listener);
  stubFetch(
    jsonResponse(401, { code: "authentication_required", message: "x" }),
  );

  const first = await requestJson(request);
  unsubscribe();
  stubFetch(
    jsonResponse(401, { code: "authentication_required", message: "x" }),
  );
  await requestJson(request);

  expect(listener).toHaveBeenCalledTimes(1);
  expect(first.ok).toBe(false);
  if (!first.ok) {
    expect(listener).toHaveBeenCalledWith(first.failure);
  }
});

test("本文を持たない PUT も Content-Type を付けない", async () => {
  const mock = stubFetch(jsonResponse(200, "ok"));

  await requestJson({ ...request, method: "PUT" });

  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("PUT");
  expect(init?.headers).toEqual({ Accept: "application/json" });
});

test("接続先を渡した要求は、backend の接続先の代わりにその接続先へ送る", async () => {
  const mock = stubFetch(jsonResponse(200, "ok"));

  await requestJson({ ...request, baseUrl: "/assist" });

  expect(mock.mock.calls[0]?.[0]).toBe("/assist/assertions");
});

/** 渡した文字列の塊を順に流す応答を作る。 */
function streamedResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
    { status: 200 },
  );
}

test("改行で区切った JSON を、塊の境目に依らず 1 行ずつ渡す", async () => {
  const mock = stubFetch(streamedResponse(['"a"\n"b', '"\n\n"c', 'é"']));
  const items: string[] = [];

  const result = await requestNdjson(request, (item) => items.push(item));

  expect(result.ok).toBe(true);
  expect(items).toEqual(["a", "b", "cé"]);
  expect(mock.mock.calls[0]?.[1]?.headers).toEqual({
    Accept: "application/x-ndjson",
  });
});

test("読めない行に達したら、その行より前の行を渡したまま失敗を返す", async () => {
  stubFetch(streamedResponse(['"a"\n{broken\n"c"\n']));
  const items: string[] = [];

  const result = await requestNdjson(request, (item) => items.push(item));

  expect(items).toEqual(["a"]);
  expect(result.ok ? undefined : result.failure.kind).toBe(
    "response_unreadable",
  );
});
