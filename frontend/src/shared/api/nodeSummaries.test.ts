import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import type { MatchConditionSelection } from "./matchConditions";
import { fetchNodeSummaries, type NodeSummariesRequest } from "./nodeSummaries";

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
  nodeKind: "terminal",
} as const satisfies NodeSummariesRequest;

const emptyResponse = { nodes: [], nodeCount: 0, nodeKind: "terminal" };

test("時系列と同じく、検索式を文字列のまま searchExpression に載せる", async () => {
  const expression = 'acct == "a b" and not hits > 10';
  const mock = stubFetch(jsonResponse(200, emptyResponse));

  await fetchNodeSummaries({ ...baseRequest, searchExpression: expression });

  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.pathname).toBe("/api/v0/node-summaries");
  expect(url.searchParams.getAll("searchExpression")).toEqual([expression]);
});

test.each([
  ["検索式を持たない要求", undefined],
  ["空白だけの検索式", " \t "],
])("%s は searchExpression を載せない", async (_name, expression) => {
  const mock = stubFetch(jsonResponse(200, emptyResponse));

  await fetchNodeSummaries({ ...baseRequest, searchExpression: expression });

  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.has("searchExpression")).toBe(false);
  expect(url.searchParams.get("nodeKind")).toBe("terminal");
});
