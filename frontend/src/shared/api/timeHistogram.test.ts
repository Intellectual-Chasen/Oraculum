import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import type { MatchConditionSelection } from "./matchConditions";
import { fetchTimeHistogram, type TimeHistogramRequest } from "./timeHistogram";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => result);
  vi.stubGlobal("fetch", mock);
  return mock;
}

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

const baseRequest = {
  matchConditions,
  columns: 60,
} as const satisfies TimeHistogramRequest;

const emptyResponse = {
  stepMs: 0,
  rows: [],
  localTimeRecordCount: 0,
  undatedRecordCount: 0,
  spanningRecordCount: 0,
};

test("時系列と同じく、検索式を文字列のまま searchExpression に載せる", async () => {
  const expression = "acct == alice or hits > 10";
  const mock = stubFetch(jsonResponse(200, emptyResponse));

  await fetchTimeHistogram({ ...baseRequest, searchExpression: expression });

  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.pathname).toBe("/api/v0/time-histogram");
  expect(url.searchParams.getAll("searchExpression")).toEqual([expression]);
});

test("検索式を持たない要求は searchExpression を載せない", async () => {
  const mock = stubFetch(jsonResponse(200, emptyResponse));

  const result = await fetchTimeHistogram(baseRequest);

  expect(result.ok).toBe(true);
  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.has("searchExpression")).toBe(false);
  expect(url.searchParams.get("columns")).toBe("60");
});
