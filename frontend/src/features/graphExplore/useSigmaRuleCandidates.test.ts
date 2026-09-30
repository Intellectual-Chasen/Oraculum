// @vitest-environment jsdom
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import { sigmaCandidatesResponse } from "@/testdata/sigmaRuleCandidates/candidateResponse";
import { useSigmaRuleCandidates } from "./useSigmaRuleCandidates";
import type { SubgraphCriteria } from "./useSubgraph";

const criteria: SubgraphCriteria = {
  matchConditions: { conditions: [{ conditionKey: "destination_ip" }] },
  depth: 1,
  valueContains: ["synthetic-user"],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("読み込む間は loading を示し、グラフと同じ条件で取得する", async () => {
  let resolveResponse: ((value: Response) => void) | undefined;
  const fetchMock = vi.fn(
    (_input: string, _init?: RequestInit) =>
      new Promise<Response>((resolve) => {
        resolveResponse = resolve;
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const { result } = renderHook(() => useSigmaRuleCandidates(criteria, 0));

  expect(result.current.status).toBe("loading");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  const [input] = fetchMock.mock.calls[0] ?? [];
  const url = new URL(input, "http://example.test");
  expect(url.pathname).toBe("/api/v0/sigma-rule-candidates");
  expect(url.searchParams.getAll("valueContains")).toEqual(["synthetic-user"]);
  expect(url.searchParams.getAll("matchCondition")).toEqual(["destination_ip"]);

  resolveResponse?.(jsonResponse(200, sigmaCandidatesResponse()));
  await waitFor(() =>
    expect(result.current).toEqual({
      status: "loaded",
      value: sigmaCandidatesResponse(),
    }),
  );
});

test("検索の条件が変わると取得し直す", async () => {
  const fetchMock = vi.fn(async (_input: string, _init?: RequestInit) =>
    jsonResponse(200, sigmaCandidatesResponse()),
  );
  vi.stubGlobal("fetch", fetchMock);
  const { rerender } = renderHook(
    ({ current }) => useSigmaRuleCandidates(current, 0),
    { initialProps: { current: criteria } },
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  rerender({ current: { ...criteria, valueContains: ["other-user"] } });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  const url = new URL(
    fetchMock.mock.calls[1]?.[0] ?? "",
    "http://example.test",
  );
  expect(url.searchParams.getAll("valueContains")).toEqual(["other-user"]);
});

test("server の失敗と読めない応答を failed で返す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(500, {})),
  );
  const failed = renderHook(() => useSigmaRuleCandidates(criteria, 0));
  await waitFor(() => expect(failed.result.current.status).toBe("failed"));
  failed.unmount();

  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(200, { ...sigmaCandidatesResponse(), rules: "synthetic" }),
    ),
  );
  const unreadable = renderHook(() => useSigmaRuleCandidates(criteria, 0));
  await waitFor(() => expect(unreadable.result.current.status).toBe("failed"));
});

test("unmount で要求を中止し、遅れて届く応答を採用しない", async () => {
  const pending: Array<(value: Response) => void> = [];
  const signals: AbortSignal[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: string, init?: RequestInit) => {
      if (init?.signal) signals.push(init.signal);
      return new Promise<Response>((resolve) => pending.push(resolve));
    }),
  );
  const { result, unmount } = renderHook(() =>
    useSigmaRuleCandidates(criteria, 0),
  );
  await waitFor(() => expect(pending).toHaveLength(1));
  unmount();
  expect(signals[0]?.aborted).toBe(true);
  pending[0]?.(jsonResponse(200, sigmaCandidatesResponse()));
  await Promise.resolve();
  expect(result.current.status).toBe("loading");
});
