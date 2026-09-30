// @vitest-environment jsdom
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { candidateResponse } from "@/testdata/attackCandidates/candidateResponse";
import { jsonResponse } from "@/testdata/http";
import { useAttackCandidates } from "./useAttackCandidates";
import type { SubgraphCriteria } from "./useSubgraph";

const criteria: SubgraphCriteria = {
  matchConditions: { conditions: [{ conditionKey: "destination_ip" }] },
  depth: 1,
  eventCategory: "process",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("候補を読み込む間はloadingを示し、graph条件で取得する", async () => {
  let resolveResponse: ((value: Response) => void) | undefined;
  const fetchMock = vi.fn(
    (_input: string, _init?: RequestInit) =>
      new Promise<Response>((resolve) => {
        resolveResponse = resolve;
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const { result } = renderHook(() => useAttackCandidates(criteria, 0));

  expect(result.current.status).toBe("loading");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  const [input] = fetchMock.mock.calls[0] ?? [];
  const url = new URL(input, "http://example.test");
  expect(url.pathname).toBe("/api/v0/attack-candidates");
  expect(url.searchParams.has("expansion")).toBe(false);
  expect(url.searchParams.get("eventCategory")).toBe("process");
  expect(url.searchParams.getAll("matchCondition")).toEqual(["destination_ip"]);

  resolveResponse?.(jsonResponse(200, candidateResponse()));
  await waitFor(() => expect(result.current.status).toBe("loaded"));
});

test("通信失敗と候補0件を別の状態で返す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(500, {})),
  );
  const failed = renderHook(() => useAttackCandidates(criteria, 0));
  await waitFor(() => expect(failed.result.current.status).toBe("failed"));
  failed.unmount();

  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, candidateResponse([]))),
  );
  const empty = renderHook(() => useAttackCandidates(criteria, 0));
  await waitFor(() => expect(empty.result.current.status).toBe("loaded"));
  expect(empty.result.current).toMatchObject({
    status: "loaded",
    value: { rules: [{ matchCount: 0 }], matches: [] },
  });
});

test("条件変更時に前の要求を中止し、遅れて届く古い応答を採用しない", async () => {
  const pending: Array<(value: Response) => void> = [];
  const signals: AbortSignal[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: string, init?: RequestInit) => {
      if (init?.signal) signals.push(init.signal);
      return new Promise<Response>((resolve) => pending.push(resolve));
    }),
  );
  const firstCriteria = { ...criteria, eventCategory: "first" };
  const secondCriteria = { ...criteria, eventCategory: "second" };
  const { result, rerender } = renderHook(
    ({ requestCriteria, version }) =>
      useAttackCandidates(requestCriteria, version),
    { initialProps: { requestCriteria: firstCriteria, version: 0 } },
  );
  await waitFor(() => expect(pending).toHaveLength(1));
  rerender({ requestCriteria: secondCriteria, version: 0 });
  await waitFor(() => expect(pending).toHaveLength(2));
  expect(signals[0]?.aborted).toBe(true);
  pending[1]?.(jsonResponse(200, candidateResponse([])));
  await waitFor(() =>
    expect(result.current).toMatchObject({
      status: "loaded",
      value: { matches: [] },
    }),
  );
  pending[0]?.(jsonResponse(200, candidateResponse()));
  await waitFor(() =>
    expect(result.current).toMatchObject({
      status: "loaded",
      value: { matches: [] },
    }),
  );

  rerender({ requestCriteria: secondCriteria, version: 1 });
  await waitFor(() => expect(pending).toHaveLength(3));
  expect(signals[1]?.aborted).toBe(true);
  pending[2]?.(jsonResponse(200, candidateResponse()));
  await waitFor(() =>
    expect(result.current).toMatchObject({
      status: "loaded",
      value: { matches: [candidateResponse().matches[0]] },
    }),
  );
});
